package inventory_test

import (
	"context"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/health"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// aLongHeartbeat is far longer than any of these tests run.
//
// It is the whole method: with the poller set to fifteen minutes, anything that
// appears within a few seconds appeared because the watch put it there. A test
// using the real forty-five seconds would be a test that waited, and would pass
// just as well with no watch at all.
const aLongHeartbeat = 15 * time.Minute

// TestAChangeAppearsWithoutWaitingForTheHeartbeat is criterion 1.
//
// The node is renamed through the resource a real node's hostname lives in, and
// the rename has to be in the read model long before the poller would next look.
func TestAChangeAppearsWithoutWaitingForTheHeartbeat(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixtureWith(t, talossim.Options{Hostname: "before-1", ControlPlane: true}, aLongHeartbeat)

	f.importCluster(t, ctx)
	id := f.onlyMachine(t, ctx)

	if err := f.svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	f.awaitWatch(t, ctx, id, true)

	if err := f.sim.SetHostname(ctx, "after-1"); err != nil {
		t.Fatalf("SetHostname: %v", err)
	}

	f.await(t, ctx, 20*time.Second, func(v inventory.MachineView) bool {
		return v.Hostname.Value == "after-1"
	}, "the node was renamed and the read model still shows the old name; with the heartbeat at "+
		aLongHeartbeat.String()+" the watch is the only thing that could have brought it, so it brought nothing")
}

// TestTheHeartbeatStillRunsWhileAWatchIsLive is criterion 2.
//
// The requirement is not that the poller exists in the source. It is that it
// keeps reading while a watch is up -- because a poller that stands down once a
// watch is running is a poller that cannot notice the watch dying, which is the
// one failure it was kept for.
//
// The probe is a fact no watched topic carries: the node's Talos version comes
// from a resource nothing subscribes to, so a change to it can only reach the
// read model through a pass the poller made on its own timer.
func TestTheHeartbeatStillRunsWhileAWatchIsLive(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixtureWith(t, talossim.Options{
		Hostname:     "polled-1",
		ControlPlane: true,
		TalosVersion: "v1.12.4",
	}, 900*time.Millisecond)

	f.importCluster(t, ctx)
	id := f.onlyMachine(t, ctx)

	if err := f.svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	f.awaitWatch(t, ctx, id, true)

	f.sim.SetVersion("v1.12.5")

	f.await(t, ctx, 20*time.Second, func(v inventory.MachineView) bool {
		return v.TalosVersion.Value == "v1.12.5"
	}, "with a watch live, a change to a fact no topic carries never arrived -- so the heartbeat "+
		"stopped running once the watch came up, and nothing is left that could notice the watch dying")
}

// TestAWatchThatDiesIsVisibleAndRebuilt is criterion 3, and it is also the
// measured failure from internal/talos/watch.go arriving at the screen.
//
// A node that is stopped does not end its own subscription: the watch stays
// open, silent, for about fifteen minutes. What ends it is the heartbeat
// failing twice, and what the operator has to see is not the absence of updates
// but a sentence.
func TestAWatchThatDiesIsVisibleAndRebuilt(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixtureWith(t, talossim.Options{
		Hostname:     "dying-1",
		ControlPlane: true,
	}, 700*time.Millisecond)

	f.importCluster(t, ctx)
	id := f.onlyMachine(t, ctx)

	if err := f.svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	f.awaitWatch(t, ctx, id, true)

	if err := f.sim.Close(); err != nil {
		t.Fatalf("talossim.Close: %v", err)
	}

	f.await(t, ctx, 30*time.Second, func(v inventory.MachineView) bool {
		return !v.Watch.Live && v.Watch.Reason != ""
	}, "the node was taken away and its watch still reports itself live with no reason given; a "+
		"subscription that keeps claiming freshness after its node is gone is the silent freeze "+
		"this whole pairing exists against")

	v, err := f.svc.Machine(ctx, id)
	if err != nil {
		t.Fatalf("Machine: %v", err)
	}
	if v.Watch.Restarts == 0 {
		t.Error("the watch went from live to dead and the restart count did not move; a watch that " +
			"flaps looks healthy at every instant somebody looks and delivers nothing in between, " +
			"and this counter is the only thing that shows it")
	}
	if v.Watch.Since.IsZero() {
		t.Error("the watch state carries no timestamp, so nothing says how long it has been down")
	}
}

// TestAWatchThatHasNotDeliveredASnapshotIsNotLive is criterion 4.
//
// A subscription that has been constructed is not a subscription that has said
// what the node currently holds, and only the second is a claim about
// freshness. The unreachable node is the sharp case: there is a watch loop
// running for it, forever, and at no point may it report itself live.
//
// The second half is the one that would be easy to get wrong: a live watch is
// never itself a confirmation. Confirmation comes from a pass that read
// something, so a node nothing can read is down whatever its subscription says.
func TestAWatchThatHasNotDeliveredASnapshotIsNotLive(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixtureWith(t, talossim.Options{
		Hostname:     "gone-1",
		ControlPlane: true,
	}, 500*time.Millisecond)

	f.importCluster(t, ctx)
	id := f.onlyMachine(t, ctx)

	// Point the record at an address nothing answers on, so every attempt --
	// the poller's and the watch's alike -- fails from here on.
	rec, err := f.store.Machines().Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	rec.Addr = "203.0.113.9"
	if _, err := f.store.Machines().Put(ctx, rec); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := f.svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	f.await(t, ctx, 40*time.Second, func(v inventory.MachineView) bool {
		return v.Stage == health.StageDown
	}, "an unreachable node never reached the down stage, so this test cannot say anything about "+
		"what its watch claimed while it was there")

	v, err := f.svc.Machine(ctx, id)
	if err != nil {
		t.Fatalf("Machine: %v", err)
	}
	if v.Watch.Live {
		t.Error("a node that cannot be reached reports a live watch; a subscription that has " +
			"delivered no snapshot has not started, and saying otherwise claims a freshness " +
			"nobody established")
	}
	if v.Watch.Reason == "" {
		t.Error("the watch is not live and says nothing about why, which leaves the screen with a " +
			"false flag and no explanation")
	}
}

func (f *fixture) onlyMachine(t *testing.T, ctx context.Context) model.MachineID {
	t.Helper()

	machines, err := f.svc.Machines(ctx)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}
	if len(machines) != 1 {
		t.Fatalf("the fixture holds %d machines, want exactly the one it adopted", len(machines))
	}
	return machines[0].ID
}

// awaitWatch waits for a machine's subscription to reach a state.
func (f *fixture) awaitWatch(t *testing.T, ctx context.Context, id model.MachineID, live bool) {
	t.Helper()

	f.await(t, ctx, 30*time.Second, func(v inventory.MachineView) bool {
		return v.Watch.Live == live
	}, "the watch never became live, so every assertion after this point would be about a "+
		"subscription that was never there")
}

// await polls the read model until a condition holds.
//
// Polling a read model in a test about not polling is not the contradiction it
// looks like: what is being measured is when the fact arrived, and the poll is
// the ruler rather than the thing that put it there.
func (f *fixture) await(
	t *testing.T,
	ctx context.Context,
	within time.Duration,
	ok func(inventory.MachineView) bool,
	failure string,
) {
	t.Helper()

	id := f.onlyMachine(t, ctx)
	deadline := time.Now().Add(within)

	for time.Now().Before(deadline) {
		v, err := f.svc.Machine(ctx, id)
		if err == nil && ok(v) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatal(failure)
}

// TestReadingTheFleetDoesNotStopItFromBeingSupervised is the first of two
// regressions this phase found by accident.
//
// The read model's per-machine state was created on demand by whatever asked
// for it first, and Start took the presence of that state to mean a supervisor
// was already running. So an instance that served one list before Start
// supervised nothing at all: every node sat at whatever the last read had
// recorded, for ever, with no error anywhere and a dashboard that looked fine.
//
// It had not bitten because holzkube-managerd calls Start before it serves.
// The order is now irrelevant, and this test is what says so.
func TestReadingTheFleetDoesNotStopItFromBeingSupervised(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixtureWith(t, talossim.Options{Hostname: "read-first-1", ControlPlane: true}, time.Second)

	f.importCluster(t, ctx)

	// The read that used to be fatal.
	if _, err := f.svc.Machines(ctx); err != nil {
		t.Fatalf("Machines: %v", err)
	}

	if err := f.svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	id := f.onlyMachine(t, ctx)
	f.awaitWatch(t, ctx, id, true)

	f.await(t, ctx, 20*time.Second, func(v inventory.MachineView) bool {
		return v.Stage.Live()
	}, "the fleet was listed before Start and nothing was ever supervised afterwards")
}

// TestAMachineSupervisedAfterStartKeepsRunning is the second.
//
// Supervise used to take a context, and the only caller that had one was an
// HTTP handler -- which passed the request's. A node adopted through the API
// therefore got a supervisor that was cancelled as the response was written,
// and the node it had just adopted was never observed again until a restart.
//
// The signature no longer allows it: a supervisor's lifetime is the service's.
func TestAMachineSupervisedAfterStartKeepsRunning(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixtureWith(t, talossim.Options{Hostname: "late-1", ControlPlane: true}, time.Second)

	f.importCluster(t, ctx)
	if err := f.svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	id := f.onlyMachine(t, ctx)

	// The call the handler makes, with nothing it could bind the supervisor's
	// lifetime to even if it wanted to.
	f.svc.Supervise(id)

	f.awaitWatch(t, ctx, id, true)
}
