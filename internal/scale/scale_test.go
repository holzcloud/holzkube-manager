package scale_test

import (
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/health"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/scale"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

func cp(name string) scale.Node {
	return scale.Node{
		Machine: model.Machine{
			ID: model.MachineID(name), Hostname: name,
			Role: model.RoleControlPlane, Cluster: "prod",
		},
		Stage: health.StageWatching,
	}
}

func worker(name string) scale.Node {
	n := cp(name)
	n.Machine.Role = model.RoleWorker
	return n
}

func spare(name string, stage health.Stage) scale.Node {
	return scale.Node{
		Machine: model.Machine{ID: model.MachineID(name), Hostname: name},
		Stage:   stage,
	}
}

// members builds an etcd membership of n voters, with the arithmetic the real
// one does rather than a number typed in beside it.
func members(n int) upgrade.MemberList {
	list := upgrade.MemberList{VotingCount: n}
	for i := range n {
		list.Members = append(list.Members, upgrade.Member{
			ID: string(rune('1' + i)), Name: "cp-" + string(rune('1'+i)), Voting: true,
		})
	}
	list.Tolerates = n - (n/2 + 1)
	if list.Tolerates < 0 {
		list.Tolerates = 0
	}
	return list
}

func plan(nodes []scale.Node, spares []scale.Node, list upgrade.MemberList, problem string) scale.Plan {
	return scale.Make(scale.Input{
		Cluster:        model.Cluster{ID: "prod", Name: "production"},
		Nodes:          nodes,
		Spare:          spares,
		Members:        list,
		MembersProblem: problem,
	})
}

func removalOf(t *testing.T, p scale.Plan, name string) scale.Removal {
	t.Helper()
	for _, r := range p.Removals {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("the plan has no decision for %q", name)
	return scale.Removal{}
}

// TestAThreeMemberControlPlaneCanSpareOne, which is the ordinary case and the
// one every refusal below has to be distinguishable from.
func TestAThreeMemberControlPlaneCanSpareOne(t *testing.T) {
	p := plan([]scale.Node{cp("cp-1"), cp("cp-2"), cp("cp-3"), worker("w-1")}, nil, members(3), "")

	if p.ControlPlane != 3 || p.Workers != 1 {
		t.Errorf("counted %d control plane and %d workers", p.ControlPlane, p.Workers)
	}
	for _, name := range []string{"cp-1", "cp-2", "cp-3", "w-1"} {
		if r := removalOf(t, p, name); !r.Allowed {
			t.Errorf("%s was refused: %s", name, r.Reason)
		}
	}
	if !strings.Contains(p.Sentence, "4 of 4") {
		t.Errorf("the sentence does not say how many may go: %q", p.Sentence)
	}
}

// TestTheScreenAndTheRouteGiveOneRefusal.
//
// The whole design of this package: the sentence shown beside a node is the
// sentence the removal route would produce, because it is produced by the same
// function. A screen with its own account of the rule is a screen that looks
// right until somebody clicks.
func TestTheScreenAndTheRouteGiveOneRefusal(t *testing.T) {
	for _, tc := range []struct{ voting int }{{1}, {2}} {
		list := members(tc.voting)
		p := plan([]scale.Node{cp("cp-1")}, nil, list, "")

		got := removalOf(t, p, "cp-1")
		if got.Allowed {
			t.Fatalf("%d voting member(s): the removal was offered", tc.voting)
		}

		want := upgrade.RefuseIfCannotSpareAVoter(list, "cp-1")
		if want == nil {
			t.Fatalf("%d voting member(s): the route would have allowed it", tc.voting)
		}
		if got.Reason != want.Error() {
			t.Errorf("%d voting member(s):\n screen: %s\n route:  %s", tc.voting, got.Reason, want)
		}
	}
}

// TestAnEvenControlPlaneIsToldItBoughtNothing. This is the arithmetic the
// screen exists for: four voting members feel safer than three and tolerate
// exactly the same single loss.
func TestAnEvenControlPlaneIsToldItBoughtNothing(t *testing.T) {
	p := plan([]scale.Node{cp("cp-1"), cp("cp-2"), cp("cp-3"), cp("cp-4")}, nil, members(4), "")

	if p.Tolerates != 1 {
		t.Fatalf("four voting members tolerate %d", p.Tolerates)
	}

	joined := strings.Join(p.Advice, " ")
	for _, want := range []string{"4 voting members", "3 tolerate", "buying nothing"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the advice does not carry %q: %q", want, joined)
		}
	}
}

// TestAOneMemberClusterIsToldToAddTwo, and told why. An operator given "add
// two" without the arithmetic adds one.
func TestAOneMemberClusterIsToldToAddTwo(t *testing.T) {
	p := plan([]scale.Node{cp("cp-1")}, nil, members(1), "")

	joined := strings.Join(p.Advice, " ")
	if !strings.Contains(joined, "Add two") {
		t.Errorf("a single-member control plane is not told what to do: %q", joined)
	}
	if !strings.Contains(joined, "majority of two is two") {
		t.Errorf("the advice asserts a conclusion without the arithmetic: %q", joined)
	}
}

// TestAnUnreadableMembershipRefusesEveryControlPlaneNodeAndNoWorker.
//
// The important half is the second. A plan computed without a membership is
// not a plan with fewer answers -- it must not say a control-plane node is
// removable -- but a cluster whose etcd cannot be reached must still be a
// cluster whose workers can be removed.
func TestAnUnreadableMembershipRefusesEveryControlPlaneNodeAndNoWorker(t *testing.T) {
	p := plan(
		[]scale.Node{cp("cp-1"), cp("cp-2"), cp("cp-3"), worker("w-1")},
		nil,
		upgrade.MemberList{},
		// The shape the server actually produces: a whole clause, not a
		// fragment. A test that passed "unreachable" here would not have
		// noticed the sentence being introduced twice.
		"etcd's membership could not be read (no control-plane node answered)",
	)

	if p.MembersKnown {
		t.Error("a plan with no membership claims to know one")
	}
	for _, name := range []string{"cp-1", "cp-2", "cp-3"} {
		r := removalOf(t, p, name)
		if r.Allowed {
			t.Errorf("%s was offered with no membership read", name)
		}
		if !strings.Contains(r.Reason, "no control-plane node answered") {
			t.Errorf("%s: the reason does not say what went wrong: %q", name, r.Reason)
		}
	}
	if r := removalOf(t, p, "w-1"); !r.Allowed {
		t.Errorf("a worker was refused because etcd could not be read: %s", r.Reason)
	}

	joined := strings.Join(p.Advice, " ")
	if !strings.Contains(joined, "could not be read") {
		t.Errorf("the advice counts votes it does not have: %q", p.Advice)
	}

	// Once, not twice. The problem is a whole clause written by whoever failed
	// to read the membership, and the advice used to introduce it with its own
	// copy of the same sentence -- which no test saw, because the tests here
	// pass a short made-up problem and only the running server passes the real
	// one. It read: "etcd's membership could not be read, so nothing here
	// counts votes. etcd's membership could not be read (...)".
	if n := strings.Count(joined, "could not be read"); n != 1 {
		t.Errorf("the advice says 'could not be read' %d times:\n%s", n, joined)
	}
}

// TestZeroVotesAndNoMembershipAreDifferentAnswers. Both put 0 in the same
// field, and they send an operator to different places: one is a cluster that
// has lost etcd, the other is a question that was never asked.
func TestZeroVotesAndNoMembershipAreDifferentAnswers(t *testing.T) {
	unread := plan([]scale.Node{cp("cp-1")}, nil, upgrade.MemberList{}, "the cluster is locked")
	none := plan([]scale.Node{cp("cp-1")}, nil, members(0), "")

	if unread.MembersKnown || !none.MembersKnown {
		t.Fatalf("members_known: unread=%v, read-as-empty=%v", unread.MembersKnown, none.MembersKnown)
	}
	if !strings.Contains(none.Sentence, "tolerating") {
		t.Errorf("a read membership of zero reads as unread: %q", none.Sentence)
	}
	if !strings.Contains(unread.Sentence, "not read") {
		t.Errorf("an unread membership reads as a count: %q", unread.Sentence)
	}
}

// TestALockedNodeIsNotRemovable, with the reason the operator wrote.
//
// A lock is somebody saying "not this one" (UPG-14). Rolling operations walk
// past a locked node; a removal cannot walk past anything, so it stops.
func TestALockedNodeIsNotRemovable(t *testing.T) {
	locked := worker("w-1")
	locked.Machine.Locked = true
	locked.Machine.LockReason = "waiting on the replacement PSU"

	p := plan([]scale.Node{cp("cp-1"), cp("cp-2"), cp("cp-3"), locked}, nil, members(3), "")

	r := removalOf(t, p, "w-1")
	if r.Allowed {
		t.Fatal("a locked node was offered for removal")
	}
	if !strings.Contains(r.Reason, "waiting on the replacement PSU") {
		t.Errorf("the refusal drops the reason somebody wrote: %q", r.Reason)
	}
}

// TestAControlPlaneNodeEtcdNeverCountedIsSaidOutLoud.
//
// Four machines the inventory calls control-plane nodes and three voting
// members is not an error and not a refusal. It is one node that is not a
// vote, and an operator counting nodes on the dashboard before deciding they
// can lose one is counting the wrong thing.
func TestAControlPlaneNodeEtcdNeverCountedIsSaidOutLoud(t *testing.T) {
	p := plan([]scale.Node{cp("cp-1"), cp("cp-2"), cp("cp-3"), cp("cp-4")}, nil, members(3), "")

	joined := strings.Join(p.Advice, " ")
	if !strings.Contains(joined, "is not a vote") {
		t.Errorf("a control-plane node etcd does not know about is not mentioned: %q", joined)
	}
}

// TestAMachineNothingIsTalkingToIsNotOnOffer, and is still listed. "There is
// nothing to add" and "there are two machines and both are unreachable" send
// an operator to different places.
func TestAMachineNothingIsTalkingToIsNotOnOffer(t *testing.T) {
	p := plan(
		[]scale.Node{cp("cp-1"), cp("cp-2"), cp("cp-3")},
		[]scale.Node{spare("new-1", health.StageDown), spare("new-2", health.StageWatching)},
		members(3), "",
	)

	if len(p.Additions) != 2 {
		t.Fatalf("the plan lists %d candidates, want both", len(p.Additions))
	}
	for _, c := range p.Additions {
		switch c.Name {
		case "new-1":
			if c.Ready {
				t.Error("a machine that is down was offered")
			}
			if !strings.Contains(c.Reason, "down") {
				t.Errorf("the reason does not say what is wrong: %q", c.Reason)
			}
		case "new-2":
			if !c.Ready {
				t.Errorf("a machine that is answering was not offered: %s", c.Reason)
			}
		}
	}
	if !strings.Contains(p.Sentence, "1 machine(s) could be added") {
		t.Errorf("the sentence counts the wrong candidates: %q", p.Sentence)
	}
}

// TestTheOrderIsStable. Two reads of an unchanged cluster must be the same
// document, or a diff between them means nothing.
func TestTheOrderIsStable(t *testing.T) {
	nodes := []scale.Node{worker("w-2"), cp("cp-3"), worker("w-1"), cp("cp-1")}
	p := plan(nodes, nil, members(3), "")

	var got []string
	for _, r := range p.Removals {
		got = append(got, r.Name)
	}
	want := []string{"cp-1", "cp-3", "w-1", "w-2"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the plan lists %v, want %v", got, want)
	}
}
