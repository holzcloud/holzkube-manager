package inventory_test

import (
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/health"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// A forgotten machine's observer used to run until the process ended: two
// goroutines per forgotten machine, each still asking a node that is no longer
// in the inventory, for ever.
func TestForgettingAMachineStopsItsObservers(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixtureWith(t, talossim.Options{Hostname: "cp-1", ControlPlane: true, Bootstrapped: true}, 250*time.Millisecond)
	if err := f.svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	f.importCluster(ctx, t)
	f.await(ctx, t, 30*time.Second, func(v inventory.MachineView) bool {
		return v.Stage == health.StageWatching
	}, "the node never reached watching")

	id := f.onlyMachine(ctx, t)
	done := f.svc.SupervisorDone(id)
	if done == nil {
		t.Fatal("the machine is not supervised")
	}

	if err := f.svc.ForgetMachine(ctx, id); err != nil {
		t.Fatalf("ForgetMachine: %v", err)
	}

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("the forgotten machine's observer loops are still running")
	}
	if f.svc.SupervisorDone(id) != nil {
		t.Fatal("the forgotten machine is still listed as supervised, so re-adopting it would start nothing")
	}
}
