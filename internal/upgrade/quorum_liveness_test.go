package upgrade_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// threeVoters is a membership as etcd lists it: three voters, Tolerates 1 on
// paper. Whether any of them is up is a separate question.
func threeVoters() upgrade.MemberList {
	l := upgrade.MemberList{VotingCount: 3, Tolerates: 1}
	for i := 1; i <= 3; i++ {
		l.Members = append(l.Members, upgrade.Member{
			ID:       fmt.Sprintf("%x", i),
			Name:     fmt.Sprintf("cp-%d (id %x)", i, i),
			Hostname: fmt.Sprintf("cp-%d", i),
			Machine:  model.MachineID(fmt.Sprintf("cp-%d", i)),
			Voting:   true,
		})
	}
	return l
}

func probeDown(down ...string) upgrade.StatusProbe {
	return func(_ context.Context, id model.MachineID) error {
		for _, d := range down {
			if string(id) == d {
				return errors.New("no answer")
			}
		}
		return nil
	}
}

// Tolerates counted members, not voters that answer: with one of three already
// down, the route happily removed a second and ended etcd's quorum.
func TestAVoterCannotBeSparedWhileAnotherIsAlreadyDown(t *testing.T) {
	t.Parallel()

	list := upgrade.ProbeVoters(t.Context(), threeVoters(), probeDown("cp-3"))
	if list.Answering != 2 {
		t.Fatalf("Answering = %d, want 2", list.Answering)
	}

	if err := upgrade.RefuseIfCannotSpareAVoter(list, "cp-1"); !errors.Is(err, upgrade.ErrLastVotingMember) {
		t.Fatalf("removing a healthy voter with one already down = %v, want ErrLastVotingMember", err)
	}
	// Same answer under the display name the member route passes.
	if err := upgrade.RefuseIfCannotSpareAVoter(list, "cp-2 (id 2)"); !errors.Is(err, upgrade.ErrLastVotingMember) {
		t.Fatalf("by display name = %v, want ErrLastVotingMember", err)
	}
	// Removing the dead one is what restores the margin.
	if err := upgrade.RefuseIfCannotSpareAVoter(list, "cp-3"); err != nil {
		t.Fatalf("removing the member that is down = %v, want it allowed", err)
	}
}

func TestAHealthyThreeVoterClusterStillSparesOne(t *testing.T) {
	t.Parallel()

	list := upgrade.ProbeVoters(t.Context(), threeVoters(), probeDown())
	if err := upgrade.RefuseIfCannotSpareAVoter(list, "cp-1"); err != nil {
		t.Fatalf("all three answer: %v", err)
	}
}

// A voter nothing in the inventory matches cannot be asked, and an unasked
// voter is not a vote anybody can count on.
func TestAVoterThatCannotBeAskedCountsAsSilent(t *testing.T) {
	t.Parallel()

	l := threeVoters()
	l.Members[2].Machine = ""
	list := upgrade.ProbeVoters(t.Context(), l, probeDown())
	if list.Answering != 2 || !list.Members[2].Silent {
		t.Fatalf("Answering = %d, silent = %v", list.Answering, list.Members[2].Silent)
	}
	if err := upgrade.RefuseIfCannotSpareAVoter(list, "cp-1"); err == nil {
		t.Fatal("an unaskable voter was counted as standing")
	}
}
