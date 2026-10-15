package configdrift_test

import (
	"context"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/configdrift"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// TestTracerAnApplyThroughTheInventoryIsAnOwnApply is the tracer of the callers
// (D-10): the real inventory, the real client Connect gives out, the real
// configuration service and route (config.apply), the real drift service and
// store. The only place the test makes the observer known is
// inv.SetApplyObserver; every client below it gets the observer from Connect.
// An apply from this installation is never drift, and one from somebody else
// afterwards still is.
func TestTracerAnApplyThroughTheInventoryIsAnOwnApply(t *testing.T) {
	t.Parallel()

	a := newAdoptedNode(t)
	svc := a.service()
	a.inv.SetApplyObserver(svc.ApplyObserver())
	bg := context.Background()

	svc.Tick(bg)
	first, err := a.st.ConfigBaselines().Get(bg, a.machine.ID)
	if err != nil {
		t.Fatalf("the first sight laid no baseline: %v", err)
	}
	if first.Source != model.BaselineFirstSight {
		t.Fatalf("precondition: the baseline is %q, want first-sight", first.Source)
	}

	// The route config.apply: the machineconfig service over inv.Connect.
	ctx, cancel := context.WithTimeout(bg, 30*time.Second)
	defer cancel()
	own := machineconfig.New(machineconfig.Deps{Connect: a.inv.Connect})
	const patch = "machine:\n  certSANs:\n    - 10.0.0.9\n"
	if _, err := own.Apply(ctx, a.machine.ID, []string{patch}, machineconfig.ModeNoReboot); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// The node's configuration is no longer the baseline's: without this the
	// test would pass for a service that changed nothing.
	now, err := a.mc.ReadSnapshot(ctx, a.machine.ID)
	if err != nil {
		t.Fatalf("ReadSnapshot: %v", err)
	}
	if now.Digest == first.Digest {
		t.Fatal("precondition: the node has the configuration of the baseline after the apply, so the test proves nothing")
	}

	svc.Flush(bg)

	d, err := svc.Detail(bg, a.machine.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if d.State != configdrift.StateOK {
		t.Fatalf("state after an own apply through the inventory is %q (%s), want ok", d.State, d.Sentence)
	}
	landed, err := a.st.ConfigBaselines().Get(bg, a.machine.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if landed.Source != model.BaselineApply || landed.Digest != now.Digest {
		t.Errorf("the baseline is source %q digest %s, want apply and the node's %s", landed.Source, landed.Digest, now.Digest)
	}
	if landed.Expected != nil || !landed.DriftSince.IsZero() {
		t.Errorf("the landed baseline still carries %+v", landed)
	}
	if d.Diff != nil {
		t.Errorf("an own apply left a diff: %+v", d.Diff)
	}

	// Somebody else. thirdPartyChange connects through the same inventory, so
	// the observer is taken away first: a third party is a client the watch
	// does not hear.
	a.inv.SetApplyObserver(nil)
	a.thirdPartyChange(t, tracerPatch)
	svc.Tick(bg)
	d, err = svc.Detail(bg, a.machine.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if d.State != configdrift.StateDrift {
		t.Fatalf("state after a third party's change is %q (%s), want drift: an own apply swallowed it", d.State, d.Sentence)
	}
	if d.DriftSince.IsZero() {
		t.Errorf("drift without a start: %+v", d)
	}
}
