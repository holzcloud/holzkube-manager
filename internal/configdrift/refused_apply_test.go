package configdrift_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/configdrift"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
)

// TestARefusedApplyLeavesNoExpectationInTheBaseline holds VERS-04 from the
// refusing side: the observer of the drift baseline hangs in the client's send
// method, and a refusal in front of it never reaches it. If it did, the
// expectation would be set, and the foreign change that caused the refusal would
// later be taken for an apply of this installation and adopted without a word.
func TestARefusedApplyLeavesNoExpectationInTheBaseline(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	svc := n.service(t)
	own := n.ownApplies(svc)
	bg := context.Background()
	store := n.st.ConfigBaselines()

	svc.Tick(bg)
	first, err := store.Get(bg, "m1")
	if err != nil {
		t.Fatalf("the first reading stored no baseline: %v", err)
	}
	if first.Expected != nil {
		t.Fatalf("precondition: the baseline already expects %+v", *first.Expected)
	}

	ctx, cancel := context.WithTimeout(bg, 30*time.Second)
	defer cancel()
	planned, err := own.Plan(ctx, "m1", []string{ownPatch})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	// Somebody else, through a client without the observer.
	n.thirdPartyChange(t, kubeletPatch)

	_, err = own.Apply(ctx, "m1", []string{ownPatch}, machineconfig.ModeNoReboot, planned.BaseDigest)
	if !errors.Is(err, machineconfig.ErrChangedSincePlan) {
		t.Fatalf("Apply after a foreign change returned %v, want ErrChangedSincePlan", err)
	}

	after, err := store.Get(bg, "m1")
	if err != nil {
		t.Fatalf("Get after the refusal: %v", err)
	}
	if after.Expected != nil {
		t.Fatalf("a refused apply left the expectation %+v in the baseline", *after.Expected)
	}
	if after.Rev != first.Rev {
		t.Errorf("rev is %d after the refusal, want %d: nothing was written", after.Rev, first.Rev)
	}

	// The foreign change is a drift against the baseline, and only it: the
	// patch of the refused apply is not on the node.
	svc.Flush(bg)
	svc.Tick(bg)
	d, err := svc.Detail(bg, "m1")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if d.State != configdrift.StateDrift {
		t.Fatalf("state after the refusal is %q (%s), want drift: the foreign change was taken for an own apply", d.State, d.Sentence)
	}
	if d.Diff == nil || len(d.Diff.Changes) != 1 || d.Diff.Changes[0].Path != "KubeletConfig.image" {
		t.Fatalf("the diff is %+v, want exactly the third party's change at KubeletConfig.image", d.Diff)
	}
}
