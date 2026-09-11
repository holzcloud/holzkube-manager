package upgrade_test

import (
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// The health gate, which is UPG-02 and a release blocker.
//
// Every test here is about the same asymmetry: a three-member etcd survives
// losing one and does not survive losing two, and the second loss is not a
// degraded cluster but a stopped one that needs a restore. So the gate refuses
// rather than warns, and these pin the cases it must refuse.

func members(n int, learners int) []talos.EtcdMemberDetail {
	out := make([]talos.EtcdMemberDetail, 0, n)
	for i := range n {
		out = append(out, talos.EtcdMemberDetail{
			ID:        uint64(i + 1),
			Hostname:  hostnames[i],
			IsLearner: i >= n-learners,
		})
	}
	return out
}

var hostnames = []string{"cp-1", "cp-2", "cp-3", "cp-4", "cp-5"}

func machines(n int) []model.Machine {
	out := make([]model.Machine, 0, n)
	for i := range n {
		out = append(out, model.Machine{
			ID:       model.MachineID(hostnames[i]),
			Hostname: hostnames[i],
			Role:     model.RoleControlPlane,
		})
	}
	return out
}

// healthy is a three-member cluster with nothing wrong with it.
func healthy() upgrade.GateInput {
	in := upgrade.GateInput{
		Members:  members(3, 0),
		Voting:   3,
		Statuses: map[string]talos.EtcdStatus{},
	}
	for i, h := range hostnames[:3] {
		in.Statuses[h] = talos.EtcdStatus{
			MemberID:         uint64(i + 1),
			Leader:           1,
			RaftIndex:        1000,
			RaftAppliedIndex: 1000,
		}
	}
	return in
}

// TestAHealthyThreeNodeClusterPasses is the control. Without it every other
// test here could pass against a gate that refuses everything.
func TestAHealthyThreeNodeClusterPasses(t *testing.T) {
	t.Parallel()

	v := upgrade.Decide(healthy(), "cp-1", machines(3))
	if !v.OK {
		t.Fatalf("a healthy three-member cluster was refused: %s", v.Reason)
	}
	if v.Input.Voting != 3 {
		t.Errorf("the verdict reports %d voting members", v.Input.Voting)
	}
}

// TestTwoVotingMembersIsRefused is the whole point.
//
// Two voters tolerate zero failures: the one that remains holds one vote out
// of two, which is not a majority, and the cluster stops accepting writes
// until the other comes back. An upgrade is exactly that absence.
func TestTwoVotingMembersIsRefused(t *testing.T) {
	t.Parallel()

	in := healthy()
	in.Members = members(2, 0)
	in.Voting = 2

	v := upgrade.Decide(in, "cp-1", machines(2))
	if v.OK {
		t.Fatal("the gate allowed a node to be taken out of a two-member etcd; that is the " +
			"cluster-stopping case this gate exists for")
	}
	for _, want := range []string{"2 voting", "majority"} {
		if !strings.Contains(v.Reason, want) {
			t.Errorf("the refusal %q does not mention %q", v.Reason, want)
		}
	}
}

// TestALearnerIsNotCountedAsAVoter is the failure that looks like success.
//
// A cluster with three members, one of them a learner, reports three members
// everywhere a member count is shown -- and has two votes. Counting the
// learner is how a rolling upgrade takes down the second of two voters while
// believing it is taking down the first of three.
func TestALearnerIsNotCountedAsAVoter(t *testing.T) {
	t.Parallel()

	in := healthy()
	in.Members = members(3, 1)
	in.Voting = talos.VotingMembers(in.Members)

	if in.Voting != 2 {
		t.Fatalf("VotingMembers counted %d of three members with one learner, want 2", in.Voting)
	}

	v := upgrade.Decide(in, "cp-1", machines(3))
	if v.OK {
		t.Fatal("the gate counted a learner as a voter and allowed the second of two voters to " +
			"be taken down")
	}
}

// TestUpgradingTheLearnerItselfIsAllowed is the other half.
//
// A learner does not vote, so taking it down does not change the quorum at
// all. Refusing it would make a cluster that is adding a member unupgradeable
// for as long as the member is catching up.
func TestUpgradingTheLearnerItselfIsAllowed(t *testing.T) {
	t.Parallel()

	in := healthy()
	in.Members = members(4, 1) // three voters plus a learner
	in.Voting = talos.VotingMembers(in.Members)
	in.Statuses["cp-4"] = talos.EtcdStatus{MemberID: 4, Leader: 1, RaftIndex: 1000, RaftAppliedIndex: 1000}

	v := upgrade.Decide(in, "cp-4", machines(4))
	if !v.OK {
		t.Fatalf("the gate refused to upgrade a learner, which cannot affect quorum: %s", v.Reason)
	}
}

// TestAnAlarmRefusesEverything pins that NOSPACE and CORRUPT stop the run.
//
// Both mean a member cannot be relied on to hold what it is given. Taking
// another node down on top of that turns a problem that is still recoverable
// into a restore from backup.
func TestAnAlarmRefusesEverything(t *testing.T) {
	t.Parallel()

	for _, alarm := range []string{"NOSPACE", "CORRUPT"} {
		in := healthy()
		in.Alarms = []talos.EtcdAlarm{{MemberID: 2, Type: alarm}}

		v := upgrade.Decide(in, "cp-1", machines(3))
		if v.OK {
			t.Fatalf("the gate allowed an upgrade with a %s alarm raised", alarm)
		}
		if !strings.Contains(v.Reason, alarm) {
			t.Errorf("the refusal for %s does not name the alarm: %q", alarm, v.Reason)
		}
	}
}

// TestAnUnreachableMemberIsAlreadyTheFirstFailure.
//
// A cluster where one member does not answer has already spent its tolerance,
// whether or not anybody noticed. The node about to be upgraded would be the
// second.
func TestAnUnreachableMemberIsAlreadyTheFirstFailure(t *testing.T) {
	t.Parallel()

	in := healthy()
	in.Unreachable = []string{"cp-3"}

	v := upgrade.Decide(in, "cp-1", machines(3))
	if v.OK {
		t.Fatal("the gate allowed a second node to be taken down while a first was already gone")
	}
	if !strings.Contains(v.Reason, "cp-3") {
		t.Errorf("the refusal does not name the member that did not answer: %q", v.Reason)
	}
}

// TestAMemberThatHasNotCaughtUpIsRefused is the one that catches a rolling
// upgrade outrunning its own cluster.
//
// The previous node's changes are replicated but not yet applied everywhere. A
// member still replaying will not survive losing a peer, and the gate running
// once at the start of the run could not see this at all -- which is why
// UPG-02 asks for it before every node.
func TestAMemberThatHasNotCaughtUpIsRefused(t *testing.T) {
	t.Parallel()

	in := healthy()
	in.MaxRaftLag = upgrade.MaxRaftLag + 1

	v := upgrade.Decide(in, "cp-1", machines(3))
	if v.OK {
		t.Fatal("the gate allowed a node to be taken down while another was still replaying")
	}
	if !strings.Contains(v.Reason, "behind") {
		t.Errorf("the refusal %q does not say what is wrong", v.Reason)
	}

	// And a few entries of lag is normal, not a fault: entries are applied
	// after they are replicated, so a healthy raft is always slightly apart.
	in.MaxRaftLag = upgrade.MaxRaftLag - 1
	if v := upgrade.Decide(in, "cp-1", machines(3)); !v.OK {
		t.Errorf("the gate refused a normal amount of raft lag: %s", v.Reason)
	}
}

// TestNoLeaderIsAClusterMidElection.
func TestNoLeaderIsAClusterMidElection(t *testing.T) {
	t.Parallel()

	in := healthy()
	s := in.Statuses["cp-2"]
	s.Leader = 0
	in.Statuses["cp-2"] = s

	v := upgrade.Decide(in, "cp-1", machines(3))
	if v.OK {
		t.Fatal("the gate allowed an upgrade against a cluster with no etcd leader")
	}
	if !strings.Contains(v.Reason, "election") {
		t.Errorf("the refusal %q does not say what the state is", v.Reason)
	}
}

// TestASingleNodeClusterIsUpgradeable is the deliberate exception.
//
// One member has no quorum to lose. Refusing it would make the smallest
// homelab -- which is most of this product's audience -- unupgradeable, and
// taking its one node down is an outage the operator is already asking for.
func TestASingleNodeClusterIsUpgradeable(t *testing.T) {
	t.Parallel()

	in := upgrade.GateInput{
		Members: members(1, 0),
		Voting:  1,
		Statuses: map[string]talos.EtcdStatus{
			"cp-1": {MemberID: 1, Leader: 1, RaftIndex: 10, RaftAppliedIndex: 10},
		},
	}

	v := upgrade.Decide(in, "cp-1", machines(1))
	if !v.OK {
		t.Fatalf("the gate refused to upgrade a single-node cluster: %s", v.Reason)
	}
}

// TestTheVerdictAlwaysCarriesItsInputs is UPG-02's "shows its inputs".
//
// A gate that only says no is a gate an operator works around. The numbers it
// decided on have to travel with the decision, including when it says yes:
// "why did it let that through" deserves the same answer as "why did it not".
func TestTheVerdictAlwaysCarriesItsInputs(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		in   upgrade.GateInput
	}{
		{name: "allowed", in: healthy()},
		{name: "refused", in: func() upgrade.GateInput {
			in := healthy()
			in.Alarms = []talos.EtcdAlarm{{MemberID: 1, Type: "NOSPACE"}}
			return in
		}()},
	} {
		v := upgrade.Decide(tc.in, "cp-1", machines(3))
		if len(v.Input.Members) == 0 {
			t.Errorf("the %s verdict carries no membership", tc.name)
		}
		if len(v.Input.Statuses) == 0 {
			t.Errorf("the %s verdict carries no raft statuses", tc.name)
		}
		if v.Input.Voting == 0 {
			t.Errorf("the %s verdict carries no voting count", tc.name)
		}
	}
}
