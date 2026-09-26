package handlers

import (
	"net/http"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/history"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// historyConfigured refuses cleanly when this instance keeps no history.
func historyConfigured(d httpapi.Deps) *httpapi.Problem {
	if d.History == nil {
		return httpapi.Upstream("upstream.history-unavailable",
			"This instance was started without a metrics history.")
	}
	return nil
}

// HistoryRoutes serves the last day of the charts (2026-09-26).
//
// Both are reads of memory: the sampler (internal/history) did the asking, on
// its own timer and under its own budgets, and these routes reach no node and
// no API server. That is why they have no budget row -- they are in
// budget_test.go's list of routes that reach nothing -- and why a chart of a
// node that is down still draws the hour before it went.
//
// Reader, like the live views whose numbers they keep, and not audited, like
// them: a chart asks every time a page opens, and none of it changes anything.
func HistoryRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/machines/{id}/hardware/history",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Handler:         handler(machineHistory(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/kubernetes/apps/{namespace}/{kind}/{name}/history",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Handler:         handler(appHistory(d)),
		},
	}
}

// historyRange reads ?range=, answering the refusal itself when it is not one
// of the three.
func historyRange(w http.ResponseWriter, r *http.Request) (history.Range, bool) {
	rg, ok := history.ParseRange(r.URL.Query().Get("range"))
	if !ok {
		httpapi.WriteProblem(w, r, httpapi.Validation("The range is one of 1h, 6h or 24h.",
			httpapi.FieldError{Field: "range", Reason: "invalid"}))
		return "", false
	}
	return rg, true
}

// machineHistory is one node's hardware history.
//
// A machine the inventory does not hold is the usual 404. One it holds and has
// never sampled -- a machine in no cluster, or one that has not answered since
// the daemon started -- is 200 with no series: the machine exists, and its
// charts are empty.
func machineHistory(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		if p := historyConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		rg, ok := historyRange(w, r)
		if !ok {
			return
		}

		id := model.MachineID(r.PathValue("id"))
		if _, err := d.Inventory.MachineRecord(r.Context(), id); err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, d.History.Query(history.MachineSubject(id), rg, time.Now()))
	}
}

// appHistory is one app's CPU and memory history.
//
// Whether the app exists is answered without asking the cluster, which is the
// point of a route that reads memory. An app with history exists; so does one
// the sampler's last listing named, measured or not. Only when the sampler HAS
// listed the cluster and neither holds is it 404 -- before the first listing,
// with the cluster down or the daemon just started, nobody has asked, and an
// app with no history is answered as one with an empty chart rather than as
// one that does not exist.
func appHistory(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		if p := historyConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		rg, ok := historyRange(w, r)
		if !ok {
			return
		}

		cluster := model.ClusterID(r.PathValue("id"))
		if _, err := d.Inventory.Cluster(r.Context(), cluster); err != nil {
			writeInventoryError(w, r, d, err)
			return
		}

		namespace, name := r.PathValue("namespace"), r.PathValue("name")
		kind, ok := kube.CanonicalAppKind(r.PathValue("kind"))
		if !ok {
			writeKubernetesError(w, r, kube.ErrNoSuchWorkload)
			return
		}
		if known, certain := d.History.AppKnown(cluster, namespace, kind, name); certain && !known {
			httpapi.WriteProblem(w, r, httpapi.NotFound("notfound.kubernetes-workload",
				"No such app: "+kind+" "+namespace+"/"+name+"."))
			return
		}

		writeJSON(w, http.StatusOK,
			d.History.Query(history.AppSubject(cluster, namespace, kind, name), rg, time.Now()))
	}
}
