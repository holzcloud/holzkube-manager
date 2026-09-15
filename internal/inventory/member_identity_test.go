package inventory_test

import (
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestTheAdoptedNodeIsNotAlsoRecordedUnderItsHostname is the regression guard
// for a phantom the operator saw before any test did: a cluster card reading
// "2 nodes, 2 control plane" about a control plane, one of which answered and
// one of which never could.
//
// A Talos cluster.Member resource is named after the node's HOSTNAME.
// m.Metadata().ID() is "holzkube-01", not a UUID -- which is visible in the
// operator's own journal, where one machine is logged as a UUID and the other
// as a hostname. adoptMembers recorded the adopted node from its own facts and
// then walked the membership, skipping the one it had already done with
//
//	if model.MachineID(m.ID) == facts.UUID { continue }
//
// which compares a hostname against a UUID and is therefore never true. The
// adopted node was recorded a second time under its hostname, at whichever
// address the member happened to list first, and that record could not be
// reached under a certificate minted for a different identity.
//
// It is also INV-03 and D-10, which are release-blocking and not style: a
// record keyed by hostname moves when somebody renames a node, and the UUID is
// the one thing about a machine that does not.
func TestTheAdoptedNodeIsNotAlsoRecordedUnderItsHostname(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)

	// The membership a real control plane reports about itself: named by
	// hostname, carrying an address. Without this the simulator reports no
	// members at all and the defect has nowhere to happen -- which is why no
	// test had it.
	f := newFixture(t, talossim.Options{
		Hostname:     "cp-1",
		ControlPlane: true,
		Bootstrapped: true,
		Members: []talossim.MemberFixture{
			{ID: "cp-1", Hostname: "cp-1", ControlPlane: true, Addresses: []string{"127.0.0.1"}},
		},
	})

	f.importCluster(ctx, t)

	machines, err := f.svc.Machines(ctx)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}

	if len(machines) != 1 {
		var got []string
		for _, m := range machines {
			got = append(got, string(m.ID))
		}
		t.Fatalf("the adoption recorded %d machines for a one-node cluster: %s\n"+
			"The second one is the adopted node again, filed under the hostname its own "+
			"membership entry is named after. On the operator's cluster this read as a "+
			"phantom control plane that never answered.", len(machines), strings.Join(got, ", "))
	}

	if id := string(machines[0].ID); !looksLikeUUID(id) {
		t.Errorf("the machine is filed under %q, which is not a UUID.\n"+
			"INV-03 and D-10 want UUID-addressed records, and this is the reason rather "+
			"than the rule: a hostname moves when somebody renames a node and a UUID does not.", id)
	}
}

// looksLikeUUID asks the shape of an identifier, not its validity. A hostname
// is what this has to tell a UUID apart from, and those do not look alike.
func looksLikeUUID(id string) bool {
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		return false
	}
	for _, want := range []int{8, 4, 4, 4, 12} {
		if len(parts[0]) != want {
			return false
		}
		parts = parts[1:]
	}
	return true
}

// TestAMemberThatCannotBeAskedIsNotInvented is the other half of the operator's
// decision: a member that answers at none of its addresses gets no record at
// all, and says so.
//
// The alternative -- recording it under whatever string came to hand -- is what
// produced the phantom above. Missing from the inventory is a visible absence;
// present under a key nothing can reach is a machine that looks adopted and is
// not.
func TestAMemberThatCannotBeAskedIsNotInvented(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)

	f := newFixture(t, talossim.Options{
		Hostname:     "cp-1",
		ControlPlane: true,
		Bootstrapped: true,
		Members: []talossim.MemberFixture{
			{ID: "cp-1", Hostname: "cp-1", ControlPlane: true, Addresses: []string{"127.0.0.1"}},
			// A second member at an address nothing listens on: port 1 is
			// reserved and unbound. This is the operator's open question in
			// miniature -- a member the cluster knows about, reporting an
			// address this host cannot reach.
			{ID: "cp-2", Hostname: "cp-2", ControlPlane: true, Addresses: []string{"127.0.0.1"}},
		},
	})

	f.importCluster(ctx, t)

	machines, err := f.svc.Machines(ctx)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}
	for _, m := range machines {
		if string(m.ID) == "cp-2" {
			t.Fatalf("a member that could not be asked who it is was recorded as %q anyway.\n"+
				"That is the phantom this round removed: a record under a key no "+
				"certificate is minted for and no observation can reach.", m.ID)
		}
	}
}
