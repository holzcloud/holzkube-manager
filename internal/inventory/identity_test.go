package inventory_test

import (
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestTheUUIDWinsAgainstTheAddress is D-10.
//
// The failure it prevents is silent and expensive: with the address as the
// key, a DHCP lease moving between two machines swaps their entire histories,
// and nothing in the product would ever say so.
func TestTheUUIDWinsAgainstTheAddress(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	c := f.importCluster(ctx, t)

	const addr = "192.168.1.50"
	const first = model.MachineID("11111111-1111-4111-8111-111111111111")
	const second = model.MachineID("22222222-2222-4222-8222-222222222222")

	if _, err := f.svc.RecordMachine(ctx, c.ID,
		talos.NodeFacts{UUID: first, Hostname: "node-a"}, addr, model.RoleWorker); err != nil {
		t.Fatalf("record the first machine: %v", err)
	}

	// The same address, a different machine. This is a reboot into a rotated
	// DHCP lease, or a machine replaced in the same slot.
	if _, err := f.svc.RecordMachine(ctx, c.ID,
		talos.NodeFacts{UUID: second, Hostname: "node-b"}, addr, model.RoleWorker); err != nil {
		t.Fatalf("record the second machine: %v", err)
	}

	a, err := f.svc.Machine(ctx, first)
	if err != nil {
		t.Fatalf("the first machine's record is gone: %v", err)
	}
	if a.Hostname.Value != "node-a" {
		t.Errorf("the first machine's hostname is now %q; its record was overwritten by a stranger",
			a.Hostname.Value)
	}
	if !a.LostAddr {
		t.Error("the first machine is not marked as no longer being at that address, " +
			"so the screen cannot explain why it stopped being found")
	}

	b, err := f.svc.Machine(ctx, second)
	if err != nil {
		t.Fatalf("the second machine got no record of its own: %v", err)
	}
	if b.Addr.Value != addr {
		t.Errorf("the second machine's address is %q, want %q", b.Addr.Value, addr)
	}
	if b.LostAddr {
		t.Error("the machine that is actually at the address is marked as having lost it")
	}
}

// TestTheSameMachineAtANewAddressKeepsItsRecord is the other half of D-10: an
// address change is not a new machine.
func TestTheSameMachineAtANewAddressKeepsItsRecord(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	c := f.importCluster(ctx, t)

	const id = model.MachineID("33333333-3333-4333-8333-333333333333")

	if _, err := f.svc.RecordMachine(ctx, c.ID,
		talos.NodeFacts{UUID: id, Hostname: "wanderer"}, "192.168.1.60", model.RoleWorker); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := f.svc.RecordMachine(ctx, c.ID,
		talos.NodeFacts{UUID: id, Hostname: "wanderer"}, "192.168.1.61", model.RoleWorker); err != nil {
		t.Fatalf("re-record at a new address: %v", err)
	}

	all, err := f.svc.Machines(ctx)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}

	seen := 0
	for _, m := range all {
		if m.ID == id {
			seen++
			if m.Addr.Value != "192.168.1.61" {
				t.Errorf("Addr = %q, want the new address", m.Addr.Value)
			}
			if m.LostAddr {
				t.Error("a machine that moved is marked as having lost its address to somebody else")
			}
		}
	}
	if seen != 1 {
		t.Fatalf("the machine has %d records after an address change, want 1", seen)
	}

	_ = inventory.UrgencyNone
}
