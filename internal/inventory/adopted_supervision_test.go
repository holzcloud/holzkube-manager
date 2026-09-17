package inventory_test

import (
	"context"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/health"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestAnAdoptedNodeIsObservedWithoutARestart is the regression guard for the
// first thing an operator saw on real hardware after adoption succeeded: a
// cluster card reading "2 nodes, 0 healthy, 0 degraded, 2 not answering",
// moments after the import had PROVEN it could reach the control plane.
//
// The order here is the operator's and it is the whole point. The daemon is
// already running -- Start has already walked the store and supervised what was
// in it -- and the cluster arrives afterwards. Every machine the adoption
// records is therefore a machine Start never saw, and nothing else was asking
// anyone to look at them: Supervise had exactly one production caller, the
// manual "add a node" handler, so an adopted node sat at StageUnknown until the
// next restart happened to pick it up from the store.
//
// A node the product has just authenticated to, reported as not answering, is
// worse than a wrong colour. It is the screen teaching an operator that its
// health column is noise -- and this is the product whose entire claim is an
// inventory that stays honest.
//
// The assertion is deliberately about the READ MODEL and not about an internal
// map. What was broken is what the operator sees, and a test that reached into
// the service's supervised set would pass again the moment somebody satisfied
// the set without the observation arriving.
func TestAnAdoptedNodeIsObservedWithoutARestart(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixtureWith(t, talossim.Options{Hostname: "cp-1", ControlPlane: true, Bootstrapped: true}, time.Second)

	// The running daemon: Start walks a store that does not yet hold this
	// cluster, which is exactly what makes the adoption path responsible for
	// what it records afterwards.
	if err := f.svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	f.importCluster(ctx, t)

	// No Supervise call here on purpose. watch_test.go makes that call itself,
	// which is why the gap survived: the guard for the supervisor's LIFETIME
	// proved the handler's call outlives the request, and nothing asked whether
	// the adoption path makes a call at all.
	f.await(ctx, t, 30*time.Second, func(v inventory.MachineView) bool {
		return v.Stage == health.StageWatching
	}, "the adopted node never left StageUnknown, so nothing was observing it. "+
		"This is the cluster card reading 'not answering' about a node the import "+
		"had just authenticated to -- the adoption path recorded the machine and "+
		"asked nobody to look at it")
}

// TestASupervisorOnALifetimeWithNoDeadlineStillReadsItsNode is the regression
// guard for what the operator's journal showed on the Pi, every forty-five
// seconds, against a node that was up:
//
//	a node answered the connection and not the question
//	  error="talos: Get on holzkube-01: talos: refusing a call with no deadline"
//	closing a resource watch the heartbeat no longer believes in
//
// The daemon starts the inventory on context.Background(). The supervisors
// inherit that lifetime and handed it to Refresh unchanged, so every read of
// every heartbeat pass was refused by D-04 before it left the process, the
// node went to StageDown after two passes, and the watchdog closed its watch.
//
// Every other test in this package starts the service on testContext, which
// carries a sixty-second deadline. That deadline flowed into the supervisors
// and made the refused call a permitted one -- the fixture was supplying the
// one thing production did not. This test starts on a context that can only be
// cancelled, which is the context main.go actually passes.
func TestASupervisorOnALifetimeWithNoDeadlineStillReadsItsNode(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	heartbeat := 250 * time.Millisecond
	f := newFixtureWith(t, talossim.Options{Hostname: "cp-1", ControlPlane: true, Bootstrapped: true}, heartbeat)

	lifetime, cancel := context.WithCancel(context.WithoutCancel(ctx))
	t.Cleanup(cancel)
	if _, has := lifetime.Deadline(); has {
		t.Fatal("the lifetime carries a deadline, so this test would measure the fixture again")
	}
	if err := f.svc.Start(lifetime); err != nil {
		t.Fatalf("Start: %v", err)
	}

	f.importCluster(ctx, t)

	f.await(ctx, t, 30*time.Second, func(v inventory.MachineView) bool {
		return v.Stage == health.StageWatching
	}, "the node never reached watching")

	// Long enough for the heartbeat to have made several passes of its own,
	// and for downgradeAfter consecutive refusals to have turned into StageDown
	// if the passes are still being refused.
	id := f.onlyMachine(ctx, t)
	until := time.Now().Add(12 * heartbeat)
	for time.Now().Before(until) {
		v, err := f.svc.Machine(ctx, id)
		if err != nil {
			t.Fatalf("Machine: %v", err)
		}
		if v.Stage == health.StageDown {
			t.Fatal("a reachable node went to down under a supervisor whose lifetime has no " +
				"deadline: the heartbeat's reads are being refused before they leave the process, " +
				"which is the Pi's journal -- 'refusing a call with no deadline' every pass")
		}
		time.Sleep(25 * time.Millisecond)
	}
}
