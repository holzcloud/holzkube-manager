package configdrift_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/configdrift"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// TestTracerAFirstReadLaysABaselineAndAThirdPartyChangeIsDrift goes through
// every layer of the feature: a real node (the simulator), the real Talos
// client, a real store on disk, and a context WITHOUT a deadline, which is what
// main.go hands every background loop and what certwatch failed on (ledger 137).
func TestTracerAFirstReadLaysABaselineAndAThirdPartyChangeIsDrift(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	svc := n.service(t)
	bg := context.Background()
	if _, ok := bg.Deadline(); ok {
		t.Fatal("precondition: the context must carry no deadline")
	}

	// 1 and 2: the first reading is the baseline, and it says so.
	svc.Tick(bg)
	d, err := svc.Detail(bg, "m1")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if d.State != configdrift.StateBaselineCreated {
		t.Fatalf("state after the first reading is %q (%s), want baseline-created", d.State, d.Sentence)
	}
	if !strings.Contains(d.Sentence, "baseline") {
		t.Errorf("the sentence %q does not name the baseline", d.Sentence)
	}
	if !sha256Hex.MatchString(d.Digest) {
		t.Errorf("the digest is %q, want 64 hex characters", d.Digest)
	}
	if d.Diff != nil {
		t.Errorf("a baseline just laid has a diff: %+v", d.Diff)
	}
	rec, err := n.st.ConfigBaselines().Get(bg, "m1")
	if err != nil {
		t.Fatalf("the first reading stored no baseline: %v", err)
	}
	if rec.Source != model.BaselineFirstSight || rec.Rev != 1 || rec.Cluster != "c1" || rec.Snapshot == "" {
		t.Fatalf("the baseline is %+v, want source first-sight, rev 1, cluster c1 and a snapshot", rec)
	}
	if !rec.DriftSince.IsZero() {
		t.Errorf("DriftSince is %v on a fresh baseline", rec.DriftSince)
	}
	onDisk, err := os.ReadFile(n.baselinePath())
	if err != nil {
		t.Fatalf("read the baseline file: %v", err)
	}

	// 3: the second reading of the same node leaves the record alone.
	svc.Tick(bg)
	d, err = svc.Detail(bg, "m1")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if d.State != configdrift.StateOK {
		t.Fatalf("state after the second equal reading is %q (%s), want ok", d.State, d.Sentence)
	}
	again, err := os.ReadFile(n.baselinePath())
	if err != nil {
		t.Fatalf("read the baseline file: %v", err)
	}
	if !bytes.Equal(onDisk, again) {
		t.Errorf("a reading that found nothing new rewrote the baseline:\n before %s\n after  %s", onDisk, again)
	}
	if rec2, _ := n.st.ConfigBaselines().Get(bg, "m1"); rec2.Rev != 1 {
		t.Errorf("rev is %d after an equal reading, want 1", rec2.Rev)
	}

	// 4: a third party changes a document that is not the first.
	n.thirdPartyChange(t, "apiVersion: v1alpha1\nkind: KubeletConfig\nimage: ghcr.io/siderolabs/kubelet:v1.34.2\n")
	svc.Tick(bg)
	d, err = svc.Detail(bg, "m1")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if d.State != configdrift.StateDrift {
		t.Fatalf("state after a third party's change is %q (%s), want drift", d.State, d.Sentence)
	}
	if d.Diff == nil || len(d.Diff.Changes) != 1 || d.Diff.Changes[0].Path != "KubeletConfig.image" {
		t.Fatalf("the diff is %+v, want exactly one change at KubeletConfig.image", d.Diff)
	}
	if d.DriftSince.IsZero() {
		t.Error("Detail.DriftSince is not set while the node drifts")
	}
	drifted, err := n.st.ConfigBaselines().Get(bg, "m1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if drifted.DriftSince.IsZero() || drifted.Rev != 2 {
		t.Fatalf("the record is rev %d with DriftSince %v, want rev 2 and a set start", drifted.Rev, drifted.DriftSince)
	}
	if drifted.Snapshot != rec.Snapshot || drifted.Digest != rec.Digest {
		t.Error("the baseline moved with the drift; it must stay what it was")
	}

	// 5: a third reading: still drift, the start is written once.
	svc.Tick(bg)
	d, err = svc.Detail(bg, "m1")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if d.State != configdrift.StateDrift {
		t.Fatalf("state after a third reading is %q, want drift", d.State)
	}
	third, err := n.st.ConfigBaselines().Get(bg, "m1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !third.DriftSince.Equal(drifted.DriftSince) || third.Rev != 2 {
		t.Errorf("DriftSince moved or was rewritten: rev %d, %v then %v", third.Rev, drifted.DriftSince, third.DriftSince)
	}
}

// TestEveryChangeCountsNoPathIsExcepted: there is no list of paths that are
// allowed to change, and no mechanism for one (D-03, D-12). Each change on a
// fresh node after a first sight is drift, in the v1alpha1 document and in
// another.
func TestEveryChangeCountsNoPathIsExcepted(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, patch string
	}{
		{"v1alpha1 certSANs", "machine:\n  certSANs:\n    - 192.168.1.50\n"},
		{"v1alpha1 install.image", "machine:\n  install:\n    image: ghcr.io/siderolabs/installer:v1.14.1\n"},
		{"KubeletConfig image", "apiVersion: v1alpha1\nkind: KubeletConfig\nimage: ghcr.io/siderolabs/kubelet:v1.34.2\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			n := newNode(t)
			svc := n.service(t)
			bg := context.Background()

			svc.Tick(bg)
			d, err := svc.Detail(bg, "m1")
			if err != nil || d.State != configdrift.StateBaselineCreated {
				t.Fatalf("precondition: first sight gave %q (%v), want baseline-created", d.State, err)
			}

			n.thirdPartyChange(t, tc.patch)
			svc.Tick(bg)

			d, err = svc.Detail(bg, "m1")
			if err != nil {
				t.Fatalf("Detail: %v", err)
			}
			if d.State != configdrift.StateDrift {
				t.Fatalf("%s: state is %q (%s), want drift", tc.name, d.State, d.Sentence)
			}
			if d.Diff == nil || len(d.Diff.Changes) == 0 {
				t.Errorf("%s: drift without a diff: %+v", tc.name, d.Diff)
			}
		})
	}
}
