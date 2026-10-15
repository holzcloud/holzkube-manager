package handlers

import (
	"errors"
	"net/http"
	"regexp"

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
		{
			// Destructive for its sudo window, because it overwrites the one
			// record of what the node was meant to look like and with it the
			// proof of what changed. Admin, like the other settings that
			// decide what this product holds against a node.
			//
			// No ClusterScope: it changes nothing on any node -- it is local
			// state -- so a cluster adopted read-only can still be confirmed,
			// as it can still be backed up (cluster.backup-schedule).
			//
			// The audit record names the node from the path and the digest in
			// clear (internal/audit: MachineFromPath, the allowlist): the
			// route's own words, "this state is intended", mean nothing in an
			// archive without the node they are about.
			Method:          http.MethodPost,
			Pattern:         "/api/v1/machines/{id}/config/drift/confirm",
			RequiresSession: true,
			MinRole:         model.RoleAdmin,
			Destructive:     true,
			Action:          "config.drift-confirm",
			Handler:         handler(confirmConfigDrift(d)),
		},
	}
}

// driftDigestShape is a sha256 as lower-case hex, the digest Detail hands out.
var driftDigestShape = regexp.MustCompile(`^[0-9a-f]{64}$`)

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

// confirmConfigDrift makes the state the operator looked at the new baseline.
// The digest in the body is the one of the reading they were shown: it is the
// compare-and-swap, so a change after the screen is not confirmed with it.
func confirmConfigDrift(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := driftConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		var body struct {
			Digest string `json:"digest"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}
		if !driftDigestShape.MatchString(body.Digest) {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"Send the digest of the reading you looked at: 64 hexadecimal characters, as the difference gave it.",
				httpapi.FieldError{Field: "digest", Reason: "must be 64 lower-case hexadecimal characters"}))
			return
		}

		machine := model.MachineID(r.PathValue("id"))
		if err := d.Drift.Confirm(r.Context(), machine, body.Digest); err != nil {
			writeDriftError(w, r, d, err)
			return
		}
		detail, err := d.Drift.Detail(r.Context(), machine)
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
	switch {
	case errors.Is(err, configdrift.ErrNotRead):
		httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeDriftNotRead, err.Error()))
	case errors.Is(err, configdrift.ErrNoDrift):
		httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeNoDrift, err.Error()))
	case errors.Is(err, configdrift.ErrDigestChanged):
		httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeDriftChanged, err.Error()))
	default:
		httpapi.WriteInternal(w, r, d.Logger, err)
	}
}
