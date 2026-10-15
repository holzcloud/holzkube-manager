package rotateca_test

import (
	"context"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/configdrift"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// TestACARotationIsAnOwnApplyForTheDriftWatch runs a whole CA rotation (four
// passes, an apply each) against a real inventory whose only link to the
// configuration drift watch is the observer Connect hangs on its clients (D-10,
// DRIFT-04). The authority the node trusts has changed, and so have the
// credentials this installation holds: the watch reads with the new ones and
// finds exactly the configuration the last pass sent, which is the baseline and
// never a drift.
func TestACARotationIsAnOwnApplyForTheDriftWatch(t *testing.T) {
	t.Parallel()

	ctx := testCtx(t)
	f := newRotationFixture(t)

	machines, err := f.inv.MachinesOf(ctx, f.cluster.ID)
	if err != nil || len(machines) != 1 {
		t.Fatalf("machines = %v, %v; want the one node", machines, err)
	}
	id := machines[0].ID

	reader := machineconfig.New(machineconfig.Deps{Connect: f.inv.Connect})
	watch := configdrift.New(configdrift.Deps{
		Clusters:    func(ctx context.Context) ([]model.Cluster, error) { return f.store.Clusters().List(ctx) },
		Machines:    f.inv.MachinesOf,
		AllMachines: func(ctx context.Context) ([]model.Machine, error) { return f.store.Machines().List(ctx) },
		Read:        reader.ReadSnapshot,
		Baselines:   f.store.ConfigBaselines(),
	})
	// The one place the test makes the observer known.
	f.inv.SetApplyObserver(watch.ApplyObserver())

	watch.Tick(ctx)
	first, err := f.store.ConfigBaselines().Get(ctx, id)
	if err != nil {
		t.Fatalf("the first sight laid no baseline: %v", err)
	}
	if first.Source != model.BaselineFirstSight {
		t.Fatalf("precondition: the baseline is %q, want first-sight", first.Source)
	}
	before := f.nodeState(ctx, t, id)

	job := f.run(ctx, t, []model.MachineID{id})
	if job.State != model.JobSucceeded {
		t.Fatalf("the rotation ended %s: %s", job.State, jobText(job))
	}

	// The rotation changed what the node trusts: without this the test would
	// pass for a rotation that did nothing.
	after := f.nodeState(ctx, t, id)
	if after.IssuesFrom(before.IssuingCrt) {
		t.Fatal("precondition: the node still issues from the old authority, so nothing was rotated and the test proves nothing")
	}
	now, err := reader.ReadSnapshot(ctx, id)
	if err != nil {
		t.Fatalf("after the rotation the node cannot be read with the credentials the inventory holds: %v", err)
	}
	if now.Digest == first.Digest {
		t.Fatal("precondition: the node has the configuration of the baseline after the rotation, so the test proves nothing")
	}

	watch.Flush(ctx)
	check := func(when string) {
		t.Helper()
		d, err := watch.Detail(ctx, id)
		if err != nil {
			t.Fatalf("%s: Detail: %v", when, err)
		}
		if d.State != configdrift.StateOK {
			t.Fatalf("%s: the state after a CA rotation is %q (%s), want ok", when, d.State, d.Sentence)
		}
		if d.Diff != nil || !d.DriftSince.IsZero() {
			t.Errorf("%s: the rotation left a diff or a drift start: %+v", when, d)
		}
		rec, err := f.store.ConfigBaselines().Get(ctx, id)
		if err != nil {
			t.Fatalf("%s: Get: %v", when, err)
		}
		if rec.Source != model.BaselineApply {
			t.Errorf("%s: the baseline source is %q, want apply", when, rec.Source)
		}
		// Four applies: the expectation of the fourth wins, and it is what the
		// node has.
		if rec.Digest != now.Digest {
			t.Errorf("%s: the baseline digest %s is not the node's %s", when, rec.Digest, now.Digest)
		}
	}
	check("after the flush")

	watch.Tick(ctx)
	check("after one more pass")
}
