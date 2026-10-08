package scale_test

import (
	"context"
	"errors"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/scale"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// The screen must not offer the removal of a healthy control-plane node while
// another one is down, because the route refuses it.
func TestTheScreenDoesNotOfferARemovalWhileAVoterIsDown(t *testing.T) {
	list := members(3)
	for i := range list.Members {
		list.Members[i].Machine = model.MachineID(list.Members[i].Name)
	}
	down := list.Members[2].Name
	list = upgrade.ProbeVoters(t.Context(), list, func(_ context.Context, id model.MachineID) error {
		if string(id) == down {
			return errors.New("no answer")
		}
		return nil
	})

	p := plan([]scale.Node{cp("cp-1"), cp("cp-2"), cp(down)}, nil, list, "")
	healthy := removalOf(t, p, "cp-1")
	if healthy.Allowed {
		t.Fatalf("the removal of a healthy node was offered with another voter down: %+v", healthy)
	}
	if !removalOf(t, p, down).Allowed {
		t.Fatal("the removal of the node that is down was refused")
	}
}
