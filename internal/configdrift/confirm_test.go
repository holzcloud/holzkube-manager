package configdrift_test

import (
	"bytes"
	"context"
	"os"
	"sync/atomic"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/configdrift"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// countingReader is the real reader of machineconfig with a count of how often
// the service called it. Confirming must not add to it.
type countingReader struct {
	n    *node
	read atomic.Int64
}

func (c *countingReader) Read(ctx context.Context, id model.MachineID) (machineconfig.Snap, error) {
	c.read.Add(1)
	return c.n.mc.ReadSnapshot(ctx, id)
}

// TestTracerConfirmingTurnsTheReadingIntoTheBaselineAndTouchesNothing goes from
// a real node that somebody else changed, through the real client, the real
// store and Detail, to "so ist es gewollt" (D-07): the state that was read
// becomes the baseline, and the node is neither read nor written while that
// happens. The two counters are the measurement of "no intervention on the node".
func TestTracerConfirmingTurnsTheReadingIntoTheBaselineAndTouchesNothing(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	cr := &countingReader{n: n}
	svc := n.service(t, withRead(cr.Read))
	bg := context.Background()
	if _, ok := bg.Deadline(); ok {
		t.Fatal("precondition: the context must carry no deadline")
	}

	svc.Tick(bg)
	n.thirdPartyChange(t, kubeletPatch)
	svc.Tick(bg)

	d, err := svc.Detail(bg, "m1")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if d.State != configdrift.StateDrift {
		t.Fatalf("precondition: state is %q (%s), want drift", d.State, d.Sentence)
	}
	if !sha256Hex.MatchString(d.Digest) {
		t.Fatalf("precondition: the digest is %q, want 64 hex characters", d.Digest)
	}
	if d.Diff == nil || d.DriftSince.IsZero() {
		t.Fatalf("precondition: drift without a diff or a start: %+v", d)
	}
	before, err := n.st.ConfigBaselines().Get(bg, "m1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if before.DriftSince.IsZero() {
		t.Fatal("precondition: the record carries no DriftSince")
	}

	reads, applies := cr.read.Load(), n.sim.Calls("ApplyConfiguration")
	if reads != 2 || applies != 1 {
		t.Fatalf("precondition: %d reads and %d applies before confirming, want 2 and 1 (two ticks, one third party)", reads, applies)
	}

	if err := svc.Confirm(bg, "m1", d.Digest); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	// The node was not touched: not read, not written.
	if got := cr.read.Load(); got != reads {
		t.Errorf("Confirm read the node: %d reads before, %d after", reads, got)
	}
	if got := n.sim.Calls("ApplyConfiguration"); got != applies {
		t.Errorf("Confirm wrote to the node: %d applies before, %d after", applies, got)
	}

	// The display no longer shows the old difference.
	after, err := svc.Detail(bg, "m1")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if after.State != configdrift.StateOK {
		t.Errorf("state after confirming is %q (%s), want ok", after.State, after.Sentence)
	}
	if after.Diff != nil {
		t.Errorf("the old difference is still shown: %+v", after.Diff)
	}
	if !after.DriftSince.IsZero() {
		t.Errorf("drift_since is %v after confirming, want empty", after.DriftSince)
	}
	if after.BaselineSource != model.BaselineConfirmed {
		t.Errorf("Detail names baseline source %q, want confirmed", after.BaselineSource)
	}

	// The record is the reading.
	rec, err := n.st.ConfigBaselines().Get(bg, "m1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	switch {
	case rec.Source != model.BaselineConfirmed:
		t.Errorf("source is %q, want confirmed", rec.Source)
	case rec.Digest != d.Digest:
		t.Errorf("the record's digest is %q, want the one the operator saw, %q", rec.Digest, d.Digest)
	case rec.Digest == before.Digest:
		t.Error("the baseline did not move")
	case !rec.DriftSince.IsZero():
		t.Errorf("DriftSince is %v in the record, want empty", rec.DriftSince)
	case !rec.AppliedOnDriftAt.IsZero():
		t.Errorf("AppliedOnDriftAt is %v in the record, want empty", rec.AppliedOnDriftAt)
	case rec.Expected != nil:
		t.Errorf("Expected is %+v in the record, want none", rec.Expected)
	case rec.Rev != before.Rev+1:
		t.Errorf("rev is %d, want %d", rec.Rev, before.Rev+1)
	case rec.Cluster != "c1" || rec.Machine != "m1" || rec.Format != model.BaselineFormat || rec.Snapshot == "":
		t.Errorf("the record is %+v, want cluster c1, machine m1, the format and a snapshot", rec)
	}

	// The node still holds the changed configuration, which is the baseline now.
	onDisk, err := os.ReadFile(n.baselinePath())
	if err != nil {
		t.Fatalf("read the baseline file: %v", err)
	}
	svc.Tick(bg)
	if got := cr.read.Load(); got != reads+1 {
		t.Fatalf("the next tick read the node %d times, want 1", got-reads)
	}
	again, err := svc.Detail(bg, "m1")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if again.State != configdrift.StateOK {
		t.Errorf("state after the next reading is %q (%s), want ok", again.State, again.Sentence)
	}
	afterTick, err := os.ReadFile(n.baselinePath())
	if err != nil {
		t.Fatalf("read the baseline file: %v", err)
	}
	if !bytes.Equal(onDisk, afterTick) {
		t.Errorf("a reading of the confirmed state rewrote the baseline:\n before %s\n after  %s", onDisk, afterTick)
	}
	if got := n.sim.Calls("ApplyConfiguration"); got != applies {
		t.Errorf("a node was written after confirming: %d applies, want %d", got, applies)
	}
}
