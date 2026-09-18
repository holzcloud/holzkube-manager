package inventory_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// A refresh takes seconds and holds a revision the whole time.
//
// Refresh reads the machine record, spends the width of the network talking to
// the node, and writes the record back. Anything that writes the same record in
// that window -- the other half of an overlapping heartbeat and manual refresh,
// or recordMachine filing a moved address -- takes the revision with it, and
// the refresh arrives at a Put it can no longer make. What used to happen then
// was a log line and a lost round of observation.
//
// The two tests below pin both halves of the answer: the observation survives
// the collision, and the writer that caused it is not undone by the retry.

// interposingStore hands out a MachineStore that can run something of the
// test's choosing at the moment a Put is about to be made.
type interposingStore struct {
	store.Store
	machines *interposingMachines
}

func (s interposingStore) Machines() store.MachineStore { return s.machines }

type interposingMachines struct {
	store.MachineStore

	mu     sync.Mutex
	armed  bool
	fired  bool
	puts   int
	before func(store.MachineStore)
}

// arm makes the next Put the one that gets interfered with.
//
// The adoption path writes the record several times before the test has
// anything to say about it, so the hook stays inert until the test asks for it.
func (m *interposingMachines) arm() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.armed = true
}

// collided reports whether the interposed writer actually ran, which is the
// difference between a test that observed the race and a test that did not.
func (m *interposingMachines) collided() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.fired
}

func (m *interposingMachines) attempts() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.puts
}

func (m *interposingMachines) Put(ctx context.Context, rec model.Machine) (model.Machine, error) {
	m.mu.Lock()
	fire := m.armed && !m.fired
	if m.armed {
		m.puts++
	}
	if fire {
		m.fired = true
	}
	m.mu.Unlock()

	if fire && m.before != nil {
		m.before(m.MachineStore)
	}
	return m.MachineStore.Put(ctx, rec)
}

func interposed(before func(store.MachineStore)) (func(store.Store) store.Store, *interposingMachines) {
	hook := &interposingMachines{before: before}
	return func(st store.Store) store.Store {
		hook.MachineStore = st.Machines()
		return interposingStore{Store: st, machines: hook}
	}, hook
}

// TestARefreshThatLosesTheRevisionRaceStillLandsItsSnapshot is the defect
// itself: the write that arrives second must not be the write that is thrown
// away.
func TestARefreshThatLosesTheRevisionRaceStillLandsItsSnapshot(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)

	var f *fixture
	wrap, hook := interposed(func(ms store.MachineStore) {
		// The competing writer, running in the window a refresh holds open.
		// It reads the current revision, so it wins and the refresh does not.
		machines, err := ms.List(ctx)
		if err != nil || len(machines) == 0 {
			t.Errorf("the interposed writer could not read the inventory: %v", err)
			return
		}
		rec := machines[0]
		rec.Locked = true
		rec.LockReason = "written while a refresh was in flight"
		if _, err := ms.Put(ctx, rec); err != nil {
			t.Errorf("the interposed writer could not write: %v", err)
		}
	})
	f = newFixtureWrapped(t, talossim.Options{ControlPlane: true}, 0, wrap)

	f.importCluster(ctx, t)

	machines, err := f.svc.Machines(ctx)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}
	if len(machines) != 1 {
		t.Fatalf("the adoption recorded %d machines, want one", len(machines))
	}
	id := machines[0].ID

	hook.arm()
	f.svc.Refresh(ctx, id)

	if !hook.collided() {
		t.Fatal("the interposed writer never ran, so no revision race happened and this test " +
			"proves nothing")
	}

	rec, err := f.store.Machines().Get(ctx, id)
	if err != nil {
		t.Fatalf("read the machine back: %v", err)
	}

	if rec.Snapshot.ObservedAt.IsZero() {
		t.Error("the record carries no observation time: the refresh lost the revision race and " +
			"dropped its snapshot, which is the defect")
	}
	if rec.Snapshot.TalosVersion == "" {
		t.Error("the record carries no Talos version, so the snapshot the refresh took was not stored")
	}
	if rec.SeenAt.IsZero() {
		t.Error("the record was never marked as seen, so the refresh's write did not land")
	}
	if got := hook.attempts(); got < 2 {
		t.Errorf("the refresh made %d write attempts; it cannot have re-read the revision it lost", got)
	}
}

// TestARetriedRefreshDoesNotUndoTheWriterItLostTo is the other half, and the
// one a careless fix gets wrong.
//
// Retrying with the record read before the network round trip would put the
// address, role, cluster and lock back the way they were, which is a silent
// revert wearing a heartbeat's clothes. The retry re-reads, so only the three
// fields an observation owns may differ afterwards.
func TestARetriedRefreshDoesNotUndoTheWriterItLostTo(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)

	wrap, hook := interposed(func(ms store.MachineStore) {
		machines, err := ms.List(ctx)
		if err != nil || len(machines) == 0 {
			t.Errorf("the interposed writer could not read the inventory: %v", err)
			return
		}
		rec := machines[0]
		rec.Locked = true
		rec.LockReason = "written while a refresh was in flight"
		if _, err := ms.Put(ctx, rec); err != nil {
			t.Errorf("the interposed writer could not write: %v", err)
		}
	})
	f := newFixtureWrapped(t, talossim.Options{ControlPlane: true}, 0, wrap)

	f.importCluster(ctx, t)

	machines, err := f.svc.Machines(ctx)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}
	id := machines[0].ID

	hook.arm()
	f.svc.Refresh(ctx, id)

	rec, err := f.store.Machines().Get(ctx, id)
	if err != nil {
		t.Fatalf("read the machine back: %v", err)
	}

	if !rec.Locked {
		t.Error("the lock the interposed writer set is gone: the refresh's retry wrote back the " +
			"record it had read before the collision and undid somebody else's write")
	}
	if rec.LockReason != "written while a refresh was in flight" {
		t.Errorf("LockReason = %q, want the interposed writer's reason; the retry overwrote it",
			rec.LockReason)
	}
}

// TestFilingAMachineSurvivesLosingTheRevisionRace is the same defect on the
// other side of the seam.
//
// recordMachine reads a record, decides what to change about it, and writes it
// back. Before persistSnapshot retried, a refresh landing in that window would
// simply lose; now it wins, and the writer that loses is this one -- so a scan
// or an adoption would have reported a bare "revision conflict" to the
// operator for a collision the product caused itself. The retry re-reads,
// which is why the decisions are made inside the loop and not above it.
func TestFilingAMachineSurvivesLosingTheRevisionRace(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)

	wrap, hook := interposed(func(ms store.MachineStore) {
		machines, err := ms.List(ctx)
		if err != nil || len(machines) == 0 {
			t.Errorf("the interposed writer could not read the inventory: %v", err)
			return
		}
		rec := machines[0]
		rec.Locked = true
		rec.LockReason = "written while the machine was being filed"
		if _, err := ms.Put(ctx, rec); err != nil {
			t.Errorf("the interposed writer could not write: %v", err)
		}
	})
	f := newFixtureWrapped(t, talossim.Options{ControlPlane: true}, 0, wrap)

	c := f.importCluster(ctx, t)

	machines, err := f.svc.Machines(ctx)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}
	id := machines[0].ID

	hook.arm()
	saved, err := f.svc.RecordMachine(ctx, c.ID, talos.NodeFacts{
		UUID:     id,
		Hostname: "renamed-after-the-race",
	}, "", model.RoleControlPlane)
	if err != nil {
		t.Fatalf("filing the machine failed with %v; the collision it lost is one this product "+
			"caused itself and can simply redo", err)
	}

	if !hook.collided() {
		t.Fatal("the interposed writer never ran, so no revision race happened and this test " +
			"proves nothing")
	}
	if saved.Hostname != "renamed-after-the-race" {
		t.Errorf("Hostname = %q, want the one that was just filed", saved.Hostname)
	}
	if !saved.Locked || saved.LockReason != "written while the machine was being filed" {
		t.Error("the lock the interposed writer set is gone: the retry wrote back the record it " +
			"had read before the collision")
	}
}

// TestForgettingAClusterSurvivesAConcurrentWrite is the defect CI found and
// this Pi did not: ForgetCluster read every machine, cleared its cluster and
// wrote it back with no retry, while a supervisor was observing every one of
// them the whole time. A heartbeat landing in that window made the write carry
// a revision that no longer existed, and the operator saw "revision conflict"
// and a cluster that stayed.
//
// The race is forced here rather than waited for: a load-dependent test that
// passes on a quiet machine and fails on a busy one is how this got to CI in
// the first place.
func TestForgettingAClusterSurvivesAConcurrentWrite(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)

	wrap, hook := interposed(func(ms store.MachineStore) {
		// Exactly what a heartbeat does in that window: read the record and
		// write a snapshot onto it, taking the revision with it.
		machines, err := ms.List(ctx)
		if err != nil || len(machines) == 0 {
			t.Errorf("the interposed writer could not read the inventory: %v", err)
			return
		}
		rec := machines[0]
		rec.SeenAt = rec.SeenAt.Add(time.Second)
		if _, err := ms.Put(ctx, rec); err != nil {
			t.Errorf("the interposed writer could not write: %v", err)
		}
	})
	f := newFixtureWrapped(t, talossim.Options{ControlPlane: true}, 0, wrap)

	cluster := f.importCluster(ctx, t)
	hook.arm()

	if err := f.svc.ForgetCluster(ctx, cluster.ID); err != nil {
		t.Fatalf("ForgetCluster lost a revision race and gave up: %v. The operator sees the "+
			"cluster stay, with an error naming a revision they have no way to act on", err)
	}

	machines, err := f.svc.Machines(ctx)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}
	for _, m := range machines {
		if m.Cluster != "" {
			t.Errorf("machine %s still belongs to %s after the cluster was forgotten", m.ID, m.Cluster)
		}
	}
	if !hook.collided() {
		t.Error("the competing write never happened, so this test proved nothing about the race")
	}
}
