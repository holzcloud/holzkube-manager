package handlers

// Cluster templates (Omni parity phase 6).
//
// Two routes and neither of them applies anything. That is the whole scope and
// it is stated rather than implied: applying a template is provisioning, and
// provisioning is the one part of this product that has never run against real
// hardware. A route that silently drove it from a YAML file would move that gap
// somewhere harder to see.
//
// What is here is the half that is worth having before applying anything and
// that this installation can be certain of: what does this document mean for
// the machines I have, and what does the cluster I have look like written down.

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/holzcloud/holzkube-manager/internal/clustertemplate"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// TemplateRoutes serves the plan and the export.
func TemplateRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			// POST because it carries a document, and a read because it
			// changes nothing -- the same shape, and for the same reason, as
			// the upgrade plan route next door.
			Method:          http.MethodPost,
			Pattern:         "/api/v1/cluster-templates/plan",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Action:          "cluster-template.plan",
			Handler:         handler(planTemplate(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/template",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Handler:         handler(exportTemplate(d)),
		},
	}
}

// TemplateNotice is what a screen says about what these routes do not do.
//
// A constant, because it is a statement about this product's limits rather than
// UI copy, and softening it would be softening the difference between a plan
// and a change.
const TemplateNotice = "Nothing here applies a template. This says what the document would mean " +
	"for the machines this installation knows about right now — which machines each class " +
	"resolves to, and what does not add up. Building or changing a cluster is still the " +
	"provisioning and upgrade screens, one decision at a time."

func planTemplate(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		// The body is YAML rather than JSON, because the document is one an
		// operator wrote in an editor and keeps in a repository. Wrapping it
		// in a JSON envelope to post it would mean escaping every newline in
		// it, which is a file nobody can read in a request log.
		raw, problem := readBody(w, r)
		if problem != nil {
			httpapi.WriteProblem(w, r, problem)
			return
		}

		tpl, err := clustertemplate.Parse(raw)
		if err != nil {
			writeTemplateError(w, r, d, err)
			return
		}

		fleet, err := d.Inventory.Fleet(r.Context())
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}

		plan := clustertemplate.Make(tpl, fleet)
		writeJSON(w, http.StatusOK, map[string]any{
			"template": tpl,
			"plan":     plan,
			"notice":   TemplateNotice,
		})
	}
}

func exportTemplate(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		id := model.ClusterID(r.PathValue("id"))

		cluster, machines, err := d.Inventory.ClusterAndMachines(r.Context(), id)
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}

		raw, err := clustertemplate.FromCluster(cluster, machines).Encode()
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, fmt.Errorf("encode the template: %w", err))
			return
		}

		w.Header().Set("Content-Type", "application/yaml")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename=%q", string(id)+".cluster.yaml"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	}
}

func writeTemplateError(w http.ResponseWriter, r *http.Request, d httpapi.Deps, err error) {
	switch {
	case errors.Is(err, clustertemplate.ErrNotATemplate):
		// Its own code, because the thing to do about it is different: a
		// document of the wrong kind was handed to the wrong route, and an
		// invalid one needs editing.
		httpapi.WriteProblem(w, r, httpapi.Validation(err.Error(),
			httpapi.FieldError{Field: "kind", Reason: "must be ClusterTemplate"}))
	case errors.Is(err, clustertemplate.ErrInvalid):
		httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
	default:
		httpapi.WriteInternal(w, r, d.Logger, err)
	}
}
