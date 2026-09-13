package handlers

import (
	"net/http"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/scale"
)

// ScaleRoutes serves the question "what would changing this cluster's size
// mean".
//
// One route, and a read. Nothing here scales anything: adding a node is
// provisioning and removing one is that node's own route, and both already
// exist. What did not exist was the answer to the question an operator asks
// before either -- which of these nodes can go, and what does adding one
// actually buy -- and that answer was scattered across etcd's membership, the
// inventory, and arithmetic somebody had to do in their head.
func ScaleRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/scale",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Handler:         handler(clusterScale(d)),
		},
	}
}

// ScaleNotice is what the screen says about its own scope.
//
// Stated on the response rather than left to the UI, for the reason the
// cluster-template notice is: a client rendering its own caveat is a client
// whose caveat drifts from what the server will actually do.
const ScaleNotice = "Nothing here changes the cluster. This says which of its nodes may be " +
	"removed and what adding one would mean; removing a node is that node's own action, and " +
	"adding one is provisioning."

func clusterScale(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		if p := upgradeConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		ctx, cancel := budgetedContext(r, EtcdRouteBudget)
		defer cancel()

		id := model.ClusterID(r.PathValue("id"))

		in, err := d.Inventory.ScaleInput(ctx, id)
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}

		// The membership is read here and its failure is carried into the plan
		// rather than returned. A cluster whose etcd cannot be reached is
		// exactly the cluster somebody is on this screen about: answering the
		// whole request with a 502 would take away the inventory half of the
		// answer -- which machines are in it, which workers can still go --
		// because of a question about the other half.
		list, err := d.Upgrade.EtcdMembers(ctx, id)
		if err != nil {
			in.MembersProblem = "etcd's membership could not be read (" + err.Error() + ")"
		} else {
			in.Members = list
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"plan":   scale.Make(in),
			"notice": ScaleNotice,
		})
	}
}
