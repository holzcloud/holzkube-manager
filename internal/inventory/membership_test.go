package inventory_test

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestANodeThatJoinsLaterAppearsWithoutBeingAdded is the operator's ruling in
// test form: the manager is a view of the cluster, so a node the cluster gains
// after adoption is in the inventory without anybody typing its address.
//
// Before the membership sync the membership was read once, at import, and a
// worker that joined afterwards was in Kubernetes and nowhere here until it
// was added by hand.
func TestANodeThatJoinsLaterAppearsWithoutBeingAdded(t *testing.T) {
	t.Parallel()
	ctx := testContext(t)

	f := newFixtureWith(t, talossim.Options{
		Hostname:     "cp-1",
		ControlPlane: true,
		Bootstrapped: true,
		Members: []talossim.MemberFixture{
			{ID: "cp-1", Hostname: "cp-1", ControlPlane: true, Addresses: []string{"127.0.0.1"}},
		},
	}, 100*time.Millisecond)

	f.importCluster(ctx, t)
	if err := f.svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The joining worker: same cluster PKI, another loopback address, the
	// first node's port -- the direct dialer knows one port and a member
	// carries addresses without one.
	worker, err := talossim.New(talossim.Options{
		Cluster:      f.cluster,
		Hostname:     "w-1",
		Bootstrapped: true,
		NodeIP:       "127.0.0.2",
		ListenAddr:   net.JoinHostPort("127.0.0.2", strconv.Itoa(f.sim.Port())),
	})
	if err != nil {
		t.Skipf("a second loopback address is not available here: %v", err)
	}
	t.Cleanup(func() { _ = worker.Close() })

	if err := f.sim.AddMember(ctx, talossim.MemberFixture{
		ID: "w-1", Hostname: "w-1", Addresses: []string{"127.0.0.2"},
	}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	deadline := time.Now().Add(20 * time.Second)
	for {
		machines, err := f.svc.Machines(ctx)
		if err != nil {
			t.Fatalf("Machines: %v", err)
		}
		for _, m := range machines {
			if m.Hostname.Value == "w-1" {
				if m.Role != model.RoleWorker {
					t.Fatalf("the joined node is filed as %q, want %q", m.Role, model.RoleWorker)
				}
				if !looksLikeUUID(string(m.ID)) {
					t.Fatalf("the joined node is filed under %q, not its UUID (INV-03)", m.ID)
				}
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("a node that joined the cluster after adoption never appeared in the inventory; "+
				"it holds %d machines. The manager is supposed to show what the cluster contains, "+
				"not what it was told about.", len(machines))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestARoleFollowsTheCluster: a record whose role disagrees with the
// membership gets the membership's answer on the next pass.
func TestARoleFollowsTheCluster(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)

	f := newFixture(t, talossim.Options{
		Hostname:     "cp-1",
		ControlPlane: true,
		Bootstrapped: true,
		Members: []talossim.MemberFixture{
			{ID: "cp-1", Hostname: "cp-1", ControlPlane: true, Addresses: []string{"127.0.0.1"}},
		},
	})
	f.importCluster(ctx, t)

	id := f.onlyMachine(ctx, t)
	rec, err := f.store.Machines().Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	rec.Role = model.RoleUnknown
	if _, err := f.store.Machines().Put(ctx, rec); err != nil {
		t.Fatalf("Put: %v", err)
	}

	f.svc.SyncMembership(ctx)

	got, err := f.store.Machines().Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Role != model.RoleControlPlane {
		t.Fatalf("after a membership pass the role is %q; the cluster says %q", got.Role, model.RoleControlPlane)
	}
}

// TestAMachineTheClusterStopsListingIsHiddenAndKept: a machine missing from
// the membership is stamped, hidden once DepartedAfter has passed, keeps its
// labels while hidden, and comes back when the cluster lists it again.
func TestAMachineTheClusterStopsListingIsHiddenAndKept(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)

	f := newFixture(t, talossim.Options{
		Hostname:     "cp-1",
		ControlPlane: true,
		Bootstrapped: true,
		Members: []talossim.MemberFixture{
			{ID: "cp-1", Hostname: "cp-1", ControlPlane: true, Addresses: []string{"127.0.0.1"}},
		},
	})
	c := f.importCluster(ctx, t)
	cp := f.onlyMachine(ctx, t)

	gone, err := f.svc.RecordMachine(ctx, c.ID, talos.NodeFacts{
		UUID: "0badc0de-0000-4000-8000-000000000001", Hostname: "gone",
	}, "", model.RoleWorker)
	if err != nil {
		t.Fatalf("RecordMachine: %v", err)
	}
	if _, err := f.svc.SetLabels(ctx, gone.ID, map[string]string{"keep": "me"}); err != nil {
		t.Fatalf("SetLabels: %v", err)
	}

	f.svc.SyncMembership(ctx)

	rec, err := f.store.Machines().Get(ctx, gone.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.LeftAt.IsZero() {
		t.Fatal("a machine the cluster does not list was not stamped as having left")
	}
	if !visible(ctx, t, f, gone.ID) {
		t.Fatal("a machine was hidden before DepartedAfter passed; one short list must hide nothing")
	}
	if cpRec, _ := f.store.Machines().Get(ctx, cp); !cpRec.LeftAt.IsZero() {
		t.Fatal("a machine the cluster does list was stamped as having left")
	}

	for _, id := range []model.MachineID{gone.ID, cp} {
		r, _ := f.store.Machines().Get(ctx, id)
		r.LeftAt = time.Now().Add(-inventory.DepartedAfter - time.Minute)
		if _, err := f.store.Machines().Put(ctx, r); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	if visible(ctx, t, f, gone.ID) {
		t.Fatal("a machine gone from the membership for longer than DepartedAfter is still shown")
	}
	if of, _ := f.svc.MachinesOf(ctx, c.ID); len(of) != 0 {
		t.Fatalf("MachinesOf still hands %d departed machines to the upgrade and health gates", len(of))
	}
	if kept, _ := f.store.Machines().Get(ctx, gone.ID); kept.Labels["keep"] != "me" {
		t.Fatal("hiding a departed machine lost its labels; the record is supposed to be kept")
	}

	// cp-1 is listed, so the next pass brings it back.
	f.svc.SyncMembership(ctx)
	if !visible(ctx, t, f, cp) {
		t.Fatal("a machine the cluster lists again stayed hidden")
	}
}

// TestAnAddressFollowsTheMembership: a recorded IP the member no longer
// reports is replaced by the reported one that answers as the same machine.
func TestAnAddressFollowsTheMembership(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)

	f := newFixture(t, talossim.Options{
		Hostname:     "cp-1",
		ControlPlane: true,
		Bootstrapped: true,
		Members: []talossim.MemberFixture{
			{ID: "cp-1", Hostname: "cp-1", ControlPlane: true, Addresses: []string{"127.0.0.1"}},
		},
	})
	f.importCluster(ctx, t)
	id := f.onlyMachine(ctx, t)

	rec, _ := f.store.Machines().Get(ctx, id)
	rec.Addr = "127.0.0.3"
	if _, err := f.store.Machines().Put(ctx, rec); err != nil {
		t.Fatalf("Put: %v", err)
	}

	f.svc.SyncMembership(ctx)

	got, _ := f.store.Machines().Get(ctx, id)
	if got.Addr != "127.0.0.1" {
		t.Fatalf("the record still dials %q; the cluster reports the machine at 127.0.0.1", got.Addr)
	}
}

// TestOpeningAViewReadsTheClusterNow: with a heartbeat far away, a live read
// still finds a node that joined a moment ago.
func TestOpeningAViewReadsTheClusterNow(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)

	f := newFixtureWith(t, talossim.Options{
		Hostname:     "cp-1",
		ControlPlane: true,
		Bootstrapped: true,
		Members: []talossim.MemberFixture{
			{ID: "cp-1", Hostname: "cp-1", ControlPlane: true, Addresses: []string{"127.0.0.1"}},
		},
	}, time.Hour)
	f.importCluster(ctx, t)
	if err := f.svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	worker, err := talossim.New(talossim.Options{
		Cluster:      f.cluster,
		Hostname:     "w-1",
		Bootstrapped: true,
		NodeIP:       "127.0.0.4",
		ListenAddr:   net.JoinHostPort("127.0.0.4", strconv.Itoa(f.sim.Port())),
	})
	if err != nil {
		t.Skipf("a second loopback address is not available here: %v", err)
	}
	t.Cleanup(func() { _ = worker.Close() })

	// Let the pass Start runs finish first, so only a live read can see w-1.
	time.Sleep(500 * time.Millisecond)
	if err := f.sim.AddMember(ctx, talossim.MemberFixture{
		ID: "w-1", Hostname: "w-1", Addresses: []string{"127.0.0.4"},
	}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	f.svc.ReadLive(ctx, "", "")

	machines, err := f.svc.Machines(ctx)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}
	for _, m := range machines {
		if m.Hostname.Value == "w-1" {
			return
		}
	}
	t.Fatalf("a view opened after a node joined does not show it (%d machines); "+
		"it showed the last heartbeat, not the cluster", len(machines))
}

func visible(ctx context.Context, t *testing.T, f *fixture, id model.MachineID) bool {
	t.Helper()
	machines, err := f.svc.Machines(ctx)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}
	for _, m := range machines {
		if m.ID == id {
			return true
		}
	}
	return false
}
