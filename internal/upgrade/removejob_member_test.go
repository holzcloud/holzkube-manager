package upgrade_test

import (
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// A removal finds the node among etcd's members by more than the inventory's
// hostname. The inventory calls cp-1 "cp-1-old" (a hostname changed by a
// configuration after the machine was recorded); etcd lists it as cp-1. Matched
// by hostname alone, the node was "not a member": the quorum check was skipped,
// the leave said there was nothing to leave, and the wipe took a voter out of
// etcd without etcd being told.

func renamedInInventory(host, as string) removalRigOptions {
	return removalRigOptions{Inventory: func(_ *removalRig, m model.Machine) model.Machine {
		if m.Hostname == host {
			m.Hostname = as
		}
		return m
	}}
}

// TestARemovalFindsItsEtcdMemberWhenTheInventoryCallsItSomethingElse: the node's
// own etcd says which member it is, and the leave happens.
func TestARemovalFindsItsEtcdMemberWhenTheInventoryCallsItSomethingElse(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 3, 0, renamedInInventory("cp-1", "cp-1-old"))

	j := r.await(r.engine, r.submit(r.engine, "cp-1").ID, model.JobSucceeded)

	if got := r.sim("cp-1").Node().EtcdLeaves; got != 1 {
		t.Errorf("cp-1 left etcd %d times, want once: the removal took it for a node that "+
			"had already left (leave: %q)", got, stepNamed(j, "leave etcd").Detail)
	}
}

// TestARemovalThatCannotMatchTheNodeToAMemberRefuses: the names differ and the
// node's own etcd does not answer, while etcd lists a member (cp-1) that no
// other machine accounts for. That member may be this node, so the removal
// refuses before anything changes instead of wiping on a guess.
func TestARemovalThatCannotMatchTheNodeToAMemberRefuses(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 3, 0, renamedInInventory("cp-1", "cp-1-old"))
	restore, err := r.sim("cp-1").Inject(talossim.Scenario{Name: talossim.ScenarioEtcdDown})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)

	j := r.await(r.engine, r.submit(r.engine, "cp-1").ID, model.JobFailed)

	if got := r.sim("cp-1").Node().Resets; got != 0 {
		t.Errorf("cp-1 was wiped %d times although it could not be told whether it is a voter", got)
	}
	var failed model.JobStep
	for _, s := range j.Steps {
		if s.State == model.StepFailed {
			failed = s
		}
	}
	if !strings.Contains(failed.Detail, "matches none of etcd's members") {
		t.Errorf("the failure %q (step %q) does not say the node could not be matched",
			failed.Detail, failed.Name)
	}
}
