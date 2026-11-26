package upgrade_test

import (
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// TestAVoterTheInventoryDoesNotKnowCountsAsUnreachable is the voter nobody
// asked.
//
// The gate asks the inventory's control-plane machines and counts etcd's
// voters. cp-3 is a voter the inventory does not list (added by hand, or left
// behind by a removal), and it is down: only cp-1 and cp-2 answered. Three
// voters on paper, two standing -- taking cp-1 leaves one vote of three.
func TestAVoterTheInventoryDoesNotKnowCountsAsUnreachable(t *testing.T) {
	t.Parallel()

	in := healthy()
	delete(in.Statuses, "cp-3")

	v := upgrade.Decide(in, "cp-1", machines(2))
	if v.OK {
		t.Fatal("the gate counted a voter nobody could ask as standing and let a second " +
			"node go from a cluster with one voter already down")
	}
	if !strings.Contains(v.Reason, "cp-3") {
		t.Errorf("the refusal %q does not name the voter that was not asked", v.Reason)
	}

	// The control: the same cluster with cp-3 answering passes.
	if v := upgrade.Decide(healthy(), "cp-1", machines(2)); !v.OK {
		t.Fatalf("all three voters answered and the gate refused: %s", v.Reason)
	}
}
