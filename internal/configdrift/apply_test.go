package configdrift_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/configdrift"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// ownApplies is the way this installation changes a node: a machineconfig
// service whose every client carries the drift service's observer. In the
// daemon the observer is hung on the client where the client is built (plan
// 07); here the test does it by hand.
func (n *node) ownApplies(svc *configdrift.Service) *machineconfig.Service {
	return machineconfig.New(machineconfig.Deps{
		Connect: func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
			cc, err := n.connect(ctx, id)
			if err != nil {
				return nil, err
			}
			cc.SetApplyObserver(svc.ApplyObserver())
			return cc, nil
		},
	})
}

// TestTracerAnOwnApplyMovesTheBaselineAndAThirdPartyChangeDoesNot goes through
// every layer: the real node (the simulator), the real Talos client with the
// observer at the one place every apply passes, a real store on disk, the real
// reader. An apply from this installation leaves its expected digest before the
// RPC, the node is read out of turn, the landed configuration becomes the
// baseline and there was never a drift; a change by somebody else afterwards is
// one (D-10, D-04).
func TestTracerAnOwnApplyMovesTheBaselineAndAThirdPartyChangeDoesNot(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	logs := &logSink{}
	svc := n.service(t, withLog(logs))
	own := n.ownApplies(svc)
	bg := context.Background()
	store := n.st.ConfigBaselines()

	svc.Tick(bg)
	first, err := store.Get(bg, "m1")
	if err != nil {
		t.Fatalf("the first reading stored no baseline: %v", err)
	}
	if first.Rev != 1 || first.Source != model.BaselineFirstSight || first.Expected != nil {
		t.Fatalf("precondition: the baseline is %+v, want rev 1, first-sight and nothing expected", first)
	}

	// The apply, under a context with a deadline as the client requires.
	const patch = "machine:\n  certSANs:\n    - 10.0.0.9\n"
	ctx, cancel := context.WithTimeout(bg, 30*time.Second)
	defer cancel()
	if _, err := own.Apply(ctx, "m1", []string{patch}, machineconfig.ModeNoReboot); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// What the observer left before the RPC is still there right after it: only
	// a reading turns it into the baseline.
	pending, err := store.Get(bg, "m1")
	if err != nil {
		t.Fatalf("Get after the apply: %v", err)
	}
	if pending.Expected == nil {
		t.Fatal("an own apply left no expected digest in the baseline")
	}
	if pending.Expected.Mode != "no-reboot" || !sha256Hex.MatchString(pending.Expected.Digest) || pending.Expected.At.IsZero() {
		t.Fatalf("the expectation is %+v, want mode no-reboot, a sha256 digest and a time", *pending.Expected)
	}
	if pending.Expected.Digest == first.Digest {
		t.Fatal("the expected digest is the baseline's: the apply changed nothing, so the test proves nothing")
	}
	if pending.Snapshot != first.Snapshot || pending.Digest != first.Digest || pending.Source != first.Source {
		t.Error("the observer moved the baseline; only a reading of the landed configuration may")
	}
	if pending.Rev != 2 {
		t.Errorf("rev is %d after the observer, want 2: one write", pending.Rev)
	}
	file, err := os.ReadFile(n.baselinePath())
	if err != nil {
		t.Fatalf("read the baseline file: %v", err)
	}
	if strings.Contains(string(file), "10.0.0.9") {
		t.Error("the file holds the content of the apply that has not landed; an expectation is a digest, a mode and a time")
	}

	// The apply's own result poked the node: Flush is what Run does with it.
	svc.Flush(bg)

	d, err := svc.Detail(bg, "m1")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if d.State != configdrift.StateOK {
		t.Fatalf("state after an own apply landed is %q (%s), want ok", d.State, d.Sentence)
	}
	if d.Diff != nil || !d.DriftSince.IsZero() {
		t.Errorf("an own apply left a diff or a drift start: %+v", d)
	}
	landed, err := store.Get(bg, "m1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if landed.Source != model.BaselineApply || landed.Digest != pending.Expected.Digest {
		t.Fatalf("the baseline is source %q digest %s, want apply and the expected %s",
			landed.Source, landed.Digest, pending.Expected.Digest)
	}
	if landed.Expected != nil || !landed.DriftSince.IsZero() || !landed.AppliedOnDriftAt.IsZero() {
		t.Errorf("the landed baseline still carries %+v", landed)
	}
	if landed.Rev != 3 {
		t.Errorf("rev is %d after the landing, want 3: the baseline was written once for it", landed.Rev)
	}
	// Never a drift, not even for one reading: a drift would have been logged
	// as a change of state.
	for _, line := range logs.Lines() {
		if strings.Contains(line, "state=drift") {
			t.Errorf("an own apply made the node drift: %q", line)
		}
	}

	// Somebody else, through a client without the observer.
	n.thirdPartyChange(t, "apiVersion: v1alpha1\nkind: KubeletConfig\nimage: ghcr.io/siderolabs/kubelet:v1.34.2\n")
	svc.Tick(bg)

	d, err = svc.Detail(bg, "m1")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if d.State != configdrift.StateDrift {
		t.Fatalf("state after a third party's change is %q (%s), want drift: an own apply swallowed it", d.State, d.Sentence)
	}
	if d.Diff == nil || len(d.Diff.Changes) != 1 || d.Diff.Changes[0].Path != "KubeletConfig.image" {
		t.Fatalf("the diff is %+v, want exactly the third party's change at KubeletConfig.image", d.Diff)
	}
	if d.BaselineSource != model.BaselineApply {
		t.Errorf("the drift is compared with a baseline from %q, want the one the apply made", d.BaselineSource)
	}
}
