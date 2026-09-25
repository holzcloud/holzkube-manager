package inventory_test

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
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
