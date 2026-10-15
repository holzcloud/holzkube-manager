package handlers

import (
	"errors"
	"net/http"

	"github.com/holzcloud/holzkube-manager/internal/configdrift"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The configuration drift routes: where a node stands against the
// configuration it had when it was last known to be right, and the
// confirmation that makes a new state the right one.
//
// Neither route asks a node anything while it is served. The watch reads the
// nodes on its own schedule and keeps what it read in memory; these routes
// work out the difference from that and from the store, which is why neither
// has a budget of its own and why both are in noUpstream of the budget guard
// in cmd/holzkube-managerd.

func driftConfigured(d httpapi.Deps) *httpapi.Problem {
	if d.Drift == nil {
		return httpapi.Upstream("upstream.drift-unavailable",
			"This instance was started without the configuration drift watch.")
	}
	return nil
}

// ConfigDriftRoutes serves the drift state and diff of one node.
func ConfigDriftRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			// The difference is configuration content, so the role is the one
			// of GET .../config and not the Reader's: the cluster list, which a
			// Reader sees, carries states only.
			Method:          http.MethodGet,
			Pattern:         "/api/v1/machines/{id}/config/drift",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Handler:         handler(getConfigDrift(d)),
		},
	}
}

func getConfigDrift(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := driftConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		// No budgetedContext: Detail reads the memory of the watch and the
		// store and asks no node.
		detail, err := d.Drift.Detail(r.Context(), model.MachineID(r.PathValue("id")))
		if err != nil {
			writeDriftError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, detail)
	}
}

// writeDriftError maps what the watch refuses onto the problems the API has.
func writeDriftError(w http.ResponseWriter, r *http.Request, d httpapi.Deps, err error) {
	if errors.Is(err, configdrift.ErrUnknownMachine) {
		httpapi.WriteProblem(w, r, httpapi.NotFound("notfound.record", "No such record."))
		return
	}
	httpapi.WriteInternal(w, r, d.Logger, err)
}
