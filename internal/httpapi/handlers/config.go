package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// The config domain's HTTP surface.
//
// Every route that returns configuration bytes returns them **already
// redacted**, and it does so by never holding unredacted bytes in the first
// place: internal/machineconfig hands back redacted views and the handlers
// have nothing else to serialise. That is the shape CFG-02 asks for -- the
// view is the leak, so the redaction is in front of the view rather than
// applied by whoever remembers.

// ConfigRouteBudget bounds the routes that read a node.
//
// A configuration read is one COSI Get plus the connection's own Version
// probe. The plan routes do the same read and then work locally, so the
// upstream half is identical and the number is shared.
const ConfigRouteBudget = 30 * time.Second

func configConfigured(d httpapi.Deps) *httpapi.Problem {
	if d.Config == nil {
		return httpapi.Upstream("upstream.config-unavailable",
			"This instance was started without the configuration service.")
	}
	return nil
}

// ConfigRoutes serves a node's configuration and the reusable patches.
func ConfigRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/machines/{id}/config",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Handler:         handler(getConfig(d)),
		},
		{
			// A plan changes nothing on the node: it reads, merges locally and
			// answers with the diff, the mode and the validation. It is a POST
			// because it carries the patches, not because it mutates.
			Method:          http.MethodPost,
			Pattern:         "/api/v1/machines/{id}/config/plan",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "config.plan",
			Handler:         handler(planConfig(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/machines/{id}/config/apply",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Destructive:     true,
			Action:          "config.apply",
			ClusterScope:    clusterFromBody,
			Handler:         handler(applyConfig(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/patches",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Handler:         handler(listPatches(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/patches",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "patch.create",
			Handler:         handler(createPatch(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/patches/{id}",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Handler:         handler(getPatch(d)),
		},
	}
}

func getConfig(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := configConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		ctx, cancel := budgetedContext(r, ConfigRouteBudget)
		defer cancel()

		view, err := d.Config.Get(ctx, model.MachineID(r.PathValue("id")))
		if err != nil {
			writeConfigError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

// planBody is what both the plan and the apply route accept.
type planBody struct {
	// Cluster is read by the lock middleware on the apply route.
	Cluster string `json:"cluster"`

	// Patches are inline patch bodies, applied in order after the stored ones.
	Patches []string `json:"patches"`

	// PatchIDs name stored patches, applied in the order given. They are
	// resolved to their bodies here rather than by the client, so that what is
	// applied is the version the store holds and not a copy a browser has been
	// carrying since yesterday.
	PatchIDs []string `json:"patch_ids"`

	// Mode is the apply mode, on the apply route only. It is required: there
	// is no default, because the default that would suggest itself -- `auto` --
	// usually means a reboot.
	Mode string `json:"mode"`
}

func planConfig(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := configConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body planBody
		if err := decodeLargeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}

		patches, problem := resolvePatches(r, d, body)
		if problem != nil {
			httpapi.WriteProblem(w, r, problem)
			return
		}

		ctx, cancel := budgetedContext(r, ConfigRouteBudget)
		defer cancel()

		preview, err := d.Config.Plan(ctx, model.MachineID(r.PathValue("id")), patches)
		if err != nil {
			writeConfigError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, preview)
	}
}

func applyConfig(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := configConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body planBody
		if err := decodeLargeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}
		if strings.TrimSpace(body.Mode) == "" {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"Name the apply mode. There is no default: the one that would suggest itself, "+
					"auto, usually means a reboot.",
				httpapi.FieldError{Field: "mode", Reason: "required"}))
			return
		}

		patches, problem := resolvePatches(r, d, body)
		if problem != nil {
			httpapi.WriteProblem(w, r, problem)
			return
		}

		ctx, cancel := budgetedContext(r, ConfigRouteBudget)
		defer cancel()

		result, err := d.Config.Apply(ctx, model.MachineID(r.PathValue("id")),
			patches, machineconfig.Mode(body.Mode))
		if err != nil {
			writeConfigError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"mode":    result.Mode,
			"details": result.Details,
		})
	}
}

// resolvePatches turns stored patch ids and inline bodies into one ordered
// list.
//
// Stored patches come first, in the order named, then the inline ones. The
// order is stated rather than left to whichever the client happened to send,
// because a strategic merge is order-dependent and a set of patches that
// applies differently depending on transport ordering would be a set nobody
// can reason about.
func resolvePatches(r *http.Request, d httpapi.Deps, body planBody) ([]string, *httpapi.Problem) {
	out := make([]string, 0, len(body.PatchIDs)+len(body.Patches))

	for _, id := range body.PatchIDs {
		rec, err := d.Store.Patches().Get(r.Context(), model.PatchID(id))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpapi.NotFound("notfound.patch", "No such patch: "+id)
			}
			return nil, httpapi.Internal(err)
		}
		if rec.Superseded {
			return nil, httpapi.Validation(
				"Patch " + rec.Name + " version " + itoa(rec.Version) + " has been superseded. " +
					"Use the current version, or say explicitly that you mean this one by naming " +
					"its successor's id.")
		}
		out = append(out, rec.Body)
	}

	out = append(out, body.Patches...)

	if len(out) == 0 {
		return nil, httpapi.Validation("Name at least one patch.",
			httpapi.FieldError{Field: "patches", Reason: "required"})
	}
	return out, nil
}

func listPatches(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all, err := d.Store.Patches().List(r.Context())
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}
		// Newest version of each lineage first, then by name. The superseded
		// ones stay in the list -- that is what append-only is for -- and are
		// marked rather than hidden.
		sort.Slice(all, func(i, j int) bool {
			if all[i].Name == all[j].Name {
				return all[i].Version > all[j].Version
			}
			return all[i].Name < all[j].Name
		})
		writeJSON(w, http.StatusOK, map[string]any{"patches": all})
	}
}

func getPatch(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec, err := d.Store.Patches().Get(r.Context(), model.PatchID(r.PathValue("id")))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				httpapi.WriteProblem(w, r, httpapi.NotFound("notfound.patch", "No such patch."))
				return
			}
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}
		writeJSON(w, http.StatusOK, rec)
	}
}

// createPatch stores a patch, or a new version of one.
//
// An edit never rewrites the old record: it writes a new one and marks the
// predecessor superseded. What that buys is the ability to answer "what
// exactly was applied to this node in March", which only has an answer if the
// thing applied still exists.
func createPatch(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name        string `json:"name"`
			Cluster     string `json:"cluster"`
			Body        string `json:"body"`
			Description string `json:"description"`

			// Parent names the version this edits. Absent for a new patch.
			Parent string `json:"parent"`
		}
		if err := decodeLargeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}

		if strings.TrimSpace(body.Name) == "" {
			httpapi.WriteProblem(w, r, httpapi.Validation("A patch needs a name.",
				httpapi.FieldError{Field: "name", Reason: "required"}))
			return
		}
		if err := machineconfig.ValidatePatch(body.Body); err != nil {
			httpapi.WriteProblem(w, r, patchProblem(err))
			return
		}

		version := 1
		var parent model.PatchID
		if body.Parent != "" {
			prev, err := d.Store.Patches().Get(r.Context(), model.PatchID(body.Parent))
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					httpapi.WriteProblem(w, r, httpapi.NotFound("notfound.patch",
						"No such patch to edit."))
					return
				}
				httpapi.WriteInternal(w, r, d.Logger, err)
				return
			}
			version = prev.Version + 1
			parent = prev.ID

			prev.Superseded = true
			if _, err := d.Store.Patches().Put(r.Context(), prev); err != nil {
				httpapi.WriteInternal(w, r, d.Logger, err)
				return
			}
		}

		id, err := newPatchID()
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}

		actor := ""
		if u, ok := d.Auth.CurrentUser(r.Context()); ok {
			actor = u.Username
		}

		rec, err := d.Store.Patches().Put(r.Context(), model.Patch{
			ID:          id,
			Name:        body.Name,
			Cluster:     model.ClusterID(body.Cluster),
			Version:     version,
			Parent:      parent,
			Body:        body.Body,
			Description: body.Description,
			Author:      actor,
			CreatedAt:   time.Now().UTC(),
		})
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}
		writeJSON(w, http.StatusCreated, rec)
	}
}

func writeConfigError(w http.ResponseWriter, r *http.Request, d httpapi.Deps, err error) {
	switch {
	case errors.Is(err, machineconfig.ErrStagedAlreadyPending):
		httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeStagedPending, err.Error()))
	case errors.Is(err, machineconfig.ErrDryRun):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeForbidden,
			Title:  "This instance applies nothing",
			Status: http.StatusForbidden,
			Detail: err.Error(),
			Code:   httpapi.CodeDryRun,
		})
	case errors.Is(err, machineconfig.ErrPatchNotStrategic), errors.Is(err, machineconfig.ErrPatchInvalid):
		httpapi.WriteProblem(w, r, patchProblem(err))
	default:
		writeInventoryError(w, r, d, err)
	}
}

func patchProblem(err error) *httpapi.Problem {
	code := httpapi.CodePatchInvalid
	if errors.Is(err, machineconfig.ErrPatchNotStrategic) {
		code = httpapi.CodePatchNotStrategic
	}
	return &httpapi.Problem{
		Type:   httpapi.TypeValidation,
		Title:  "The patch cannot be used",
		Status: http.StatusBadRequest,
		Detail: err.Error(),
		Code:   code,
	}
}

func newPatchID() (model.PatchID, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return model.PatchID(hex.EncodeToString(b[:])), nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}
