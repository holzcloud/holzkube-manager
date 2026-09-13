package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// Node actions and the jobs they become.
//
// Every destructive endpoint here answers **202 Accepted with a job id**
// (JOB-09), never a 200 with a result. That is not a style choice: a reboot
// takes a minute and a reset takes several, and an endpoint that waits for
// them is an endpoint whose progress nobody can watch and whose failure
// arrives as a timeout with no record of how far it got.
//
// Every one of them also requires a confirmation token that the server issues
// and the server checks against the parameters actually submitted (JOB-08). A
// dialog in the browser protects against a misclick and against nothing else.

// jobsConfigured refuses cleanly when this instance has no job engine.
func jobsConfigured(d httpapi.Deps) *httpapi.Problem {
	if d.Jobs == nil || d.Confirmer == nil {
		return httpapi.Upstream("upstream.jobs-unavailable",
			"This instance was started without a job engine.")
	}
	return nil
}

// JobRoutes serves the jobs and the node actions.
func JobRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/jobs",
			RequiresSession: true,
			Handler:         handler(listJobs(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/jobs/{id}",
			RequiresSession: true,
			Handler:         handler(getJob(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/jobs/{id}/cancel",
			RequiresSession: true,
			Action:          "job.cancel",
			Handler:         handler(cancelJob(d)),
		},
		{
			// The reset preview: what the machine has, what each choice would
			// do, and what has to be typed. It is a read, so it is neither
			// destructive nor confirmed -- and it is the screen the
			// confirmation is issued from.
			Method:          http.MethodGet,
			Pattern:         "/api/v1/machines/{id}/reset-preview",
			RequiresSession: true,
			Handler:         handler(resetPreview(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/machines/{id}/confirm",
			RequiresSession: true,
			Action:          "action.confirm",
			Handler:         handler(issueConfirmation(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/machines/{id}/reboot",
			RequiresSession: true,
			Destructive:     true,
			Action:          "node.reboot",
			ClusterScope:    clusterFromBody,
			Handler:         handler(nodeAction(d, model.JobReboot)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/machines/{id}/shutdown",
			RequiresSession: true,
			Destructive:     true,
			Action:          "node.shutdown",
			ClusterScope:    clusterFromBody,
			Handler:         handler(nodeAction(d, model.JobShutdown)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/machines/{id}/reset",
			RequiresSession: true,
			Destructive:     true,
			Action:          "node.reset",
			ClusterScope:    clusterFromBody,
			Handler:         handler(nodeAction(d, model.JobReset)),
		},
	}
}

func listJobs(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := jobsConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		all, err := d.Jobs.List(r.Context())
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"jobs": all})
	}
}

func getJob(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := jobsConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		j, err := d.Jobs.Get(r.Context(), model.JobID(r.PathValue("id")))
		if err != nil {
			writeJobError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, j)
	}
}

func cancelJob(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := jobsConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		// Cancelling is mutating and is deliberately *not* destructive: it
		// stops something from happening. Gating it behind the sudo window
		// would mean an operator watching something go wrong has to find their
		// password before they can stop it.
		j, err := d.Jobs.Cancel(r.Context(), model.JobID(r.PathValue("id")))
		if err != nil {
			writeJobError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusAccepted, j)
	}
}

// resetPreviewBody is what the reset screen is built from (JOB-07).
type resetPreviewBody struct {
	Machine  model.MachineID `json:"machine"`
	Hostname string          `json:"hostname"`

	// ConfirmPhrase is what the operator has to type. It is the hostname,
	// because the hostname is the thing they can check against the machine in
	// front of them -- a generic "DELETE" confirms that somebody can type, not
	// that they know which machine this is.
	ConfirmPhrase string `json:"confirm_phrase"`

	Disks []model.Disk `json:"disks"`

	// Modes are the wipe scopes, least destructive first, each with the
	// sentence describing what it does.
	Modes []resetMode `json:"modes"`

	// Defaults are the flags holzkube-manager pre-selects, and they are deliberately
	// NOT Talos's. `talosctl reset` defaults to wiping everything and not
	// rebooting; those defaults are pre-selected nowhere in this product.
	Defaults resetDefaults `json:"defaults"`

	// TalosDefaultWarning states the difference out loud, because an operator
	// who knows talosctl will otherwise assume the flags mean what they mean
	// there.
	TalosDefaultWarning string `json:"talos_default_warning"`
}

type resetMode struct {
	Mode        string `json:"mode"`
	Label       string `json:"label"`
	Description string `json:"description"`

	// NeedsDisks marks the mode that cannot be run without naming devices.
	NeedsDisks bool `json:"needs_disks"`
}

type resetDefaults struct {
	Mode     string `json:"mode"`
	Graceful bool   `json:"graceful"`
	Reboot   bool   `json:"reboot"`
}

func resetPreview(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		m, err := d.Inventory.Machine(r.Context(), model.MachineID(r.PathValue("id")))
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}

		// The disks come from the last snapshot rather than from a fresh read.
		// That is honest and it is also what the field's provenance already
		// says: the machine view carries them with their own staleness, so a
		// screen built from this can tell the operator that the disk list is
		// four minutes old rather than pretending it is now.
		writeJSON(w, http.StatusOK, resetPreviewBody{
			Machine:       m.ID,
			Hostname:      m.Hostname.Value,
			ConfirmPhrase: m.Hostname.Value,
			Disks:         m.Disks.Value,
			Modes: []resetMode{
				{
					Mode:        "user-disks",
					Label:       "User disks only",
					Description: "Wipes the block devices you name. Talos itself stays installed.",
					NeedsDisks:  true,
				},
				{
					Mode:  "system-disk",
					Label: "System disk",
					Description: "Wipes the disk Talos is installed on. The machine comes back in " +
						"maintenance mode and can be provisioned again.",
				},
				{
					Mode:        "all",
					Label:       "Everything",
					Description: "Wipes every disk on the machine. Nothing on it survives.",
				},
			},
			Defaults: resetDefaults{
				// The least destructive scope, etcd left gracefully, and the
				// machine brought back up. Every one of the three is the
				// opposite of what talosctl would do unprompted.
				Mode:     "user-disks",
				Graceful: true,
				Reboot:   true,
			},
			TalosDefaultWarning: "talosctl reset defaults to wiping every disk and leaving the machine " +
				"powered off. holzkube-manager does not pre-select either: choose the scope and both flags.",
		})
	}
}

// ActionRemoveFromCluster is the confirmable action phase 9 added.
//
// It is a constant here rather than a literal at its two use sites because
// those sites are in different files -- the token is issued in this one and
// checked in upgrade.go -- and a typo in either would produce a token that
// never validates, which reads to an operator as a confirmation dialog that
// simply does not work.
const ActionRemoveFromCluster = "node.remove-from-cluster"

// typedPhrase says, for every action this route will issue a token for,
// whether the operator has to type the machine's hostname first.
//
// It is a table and not a condition, and the reason is what it replaced. The
// rule used to be `if action == node.reset`, written when reset was the only
// confirmable action that destroyed anything. Phase 9 then added
// node.remove-from-cluster -- a node taken out of etcd and out of the
// inventory, which on a three-member control plane is a third of the quorum --
// and it inherited "no typing needed" by not being mentioned. The browser
// asked for the hostname; the server issued a token to anybody who asked
// without one.
//
// A missing entry is now a refusal rather than a silence, and
// TestEveryConfirmableActionDecidesOnTypedPhrase walks the map so that the
// next action cannot be added without somebody answering this question.
var typedPhrase = map[string]bool{
	// Reboot and shutdown: no. Asking somebody to type a hostname before every
	// reboot is how they learn to paste it without reading, and then the
	// typing means nothing on the screen where it matters.
	string(model.JobReboot):   false,
	string(model.JobShutdown): false,

	// Reset wipes disks on a real machine (JOB-07).
	string(model.JobReset): true,

	// Removing from the cluster is irreversible in the way that matters: the
	// node leaves etcd and its record here is forgotten, so what is lost is
	// the cluster's memory of it rather than its disks.
	ActionRemoveFromCluster: true,
}

// issueConfirmation hands out a token for exactly the action described.
func issueConfirmation(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := jobsConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			Action string            `json:"action"`
			Params map[string]string `json:"params"`
			// Typed is what the operator typed into the confirmation box. It
			// is checked here, once, against the machine's hostname -- so that
			// a client that skipped the box cannot get a token at all.
			Typed string `json:"typed"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}

		id := model.MachineID(r.PathValue("id"))
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		m, err := d.Inventory.Machine(r.Context(), id)
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}

		// An action nobody has decided about gets no token. A default of "no
		// typing needed" is how node.remove-from-cluster went two phases with
		// a confirmation box the server did not enforce.
		needsPhrase, known := typedPhrase[body.Action]
		if !known {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"This instance issues confirmations for a fixed set of actions and that is not one "+
					"of them.",
				httpapi.FieldError{Field: "action", Reason: "not a confirmable action"}))
			return
		}

		if needsPhrase {
			if m.Hostname.Value == "" {
				httpapi.WriteProblem(w, r, httpapi.Validation(
					"This machine has no known hostname, so there is nothing to type to confirm "+
						"this. Refresh it first."))
				return
			}
			if strings.TrimSpace(body.Typed) != m.Hostname.Value {
				httpapi.WriteProblem(w, r, httpapi.Validation(
					"Type the machine's hostname exactly to confirm.",
					httpapi.FieldError{Field: "typed", Reason: "does not match the hostname"}))
				return
			}
		}

		intent := jobs.Intent{Action: body.Action, Machine: string(id), Params: body.Params}
		token, expires := d.Confirmer.Issue(intent)

		writeJSON(w, http.StatusOK, map[string]any{
			"token":   token,
			"expires": expires.Format(time.RFC3339),
			// Echoed back so the client can show exactly what this token
			// authorises rather than what it believes it asked for.
			"action": body.Action,
			"params": body.Params,
		})
	}
}

// nodeAction submits one of the three node jobs.
func nodeAction(d httpapi.Deps, kind model.JobKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := jobsConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			Cluster      string            `json:"cluster"`
			Confirmation string            `json:"confirmation"`
			Params       map[string]string `json:"params"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}

		id := model.MachineID(r.PathValue("id"))

		// The intent is rebuilt from what was submitted, never read out of the
		// token. A token that carried its own description would authorise
		// whatever it said while the request did something else.
		if err := d.Confirmer.Check(body.Confirmation, jobs.Intent{
			Action:  string(kind),
			Machine: string(id),
			Params:  body.Params,
		}); err != nil {
			writeJobError(w, r, d, err)
			return
		}

		cluster, err := d.Inventory.ClusterOfMachine(r.Context(), id)
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}

		actor := ""
		if u, ok := d.Auth.CurrentUser(r.Context()); ok {
			actor = u.Username
		}

		j, err := d.Jobs.Submit(r.Context(), model.Job{
			Kind:    kind,
			Cluster: cluster,
			Machine: id,
			Params:  body.Params,
			Actor:   actor,
		})
		if err != nil {
			writeJobError(w, r, d, err)
			return
		}

		// 202, with the job id and where to watch it. The operation has been
		// accepted and has not happened yet, and saying 200 would be claiming
		// otherwise.
		w.Header().Set("Location", "/api/v1/jobs/"+string(j.ID))
		writeJSON(w, http.StatusAccepted, map[string]any{
			"job":   j,
			"topic": string(jobs.Topic(j.ID)),
		})
	}
}

// writeJobError maps the engine's errors onto the taxonomy.
func writeJobError(w http.ResponseWriter, r *http.Request, d httpapi.Deps, err error) {
	switch {
	case errors.Is(err, jobs.ErrNotFound):
		httpapi.WriteProblem(w, r, httpapi.NotFound("notfound.job", "No such job."))
	case errors.Is(err, jobs.ErrClusterBusy):
		httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeClusterBusy, err.Error()))
	case errors.Is(err, jobs.ErrNotCancellable):
		httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeJobFinished,
			"This job has already finished; there is nothing to cancel."))
	case errors.Is(err, jobs.ErrConfirmationExpired):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeForbidden,
			Title:  "The confirmation has expired",
			Status: http.StatusForbidden,
			Detail: "Confirmations are good for a few minutes. Read the dialog again and confirm.",
			Code:   httpapi.CodeConfirmationExpired,
		})
	case errors.Is(err, jobs.ErrConfirmationInvalid):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeForbidden,
			Title:  "This is not a confirmation for this request",
			Status: http.StatusForbidden,
			Detail: "The confirmation does not match the action and parameters that were submitted.",
			Code:   httpapi.CodeConfirmationInvalid,
		})
	case errors.Is(err, jobs.ErrUnknownKind):
		httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
	default:
		if strings.HasPrefix(err.Error(), "jobs: ") {
			// The engine's own validation refusals -- a reset with no wipe
			// mode, a user-disks reset naming no disks -- are the operator's
			// to fix and carry their own sentence.
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}
		httpapi.WriteInternal(w, r, d.Logger, err)
	}
}
