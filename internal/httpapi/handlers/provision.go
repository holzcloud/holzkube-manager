package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/provision"
)

// The provisioning wizard's HTTP surface.
//
// Three of these five routes are reads that change nothing on a machine, and
// the fourth is the only one in this product that can wipe a disk somebody
// else's data is on. The split is deliberate and it is the shape of the
// screens: an operator scans, looks at one machine, reads exactly what would
// be written, and only then submits -- and the submission is a job, so that
// closing the tab loses a viewer rather than a run (PROV-11).
//
// The apply route is the only one marked Destructive. It therefore requires
// the sudo window, a server-issued confirmation bound to the parameters
// actually submitted, and an unlocked cluster -- the same three gates the node
// actions pass, for the same reason.

// ScanRouteBudget bounds a subnet scan.
//
// The arithmetic, because this is the largest number in the route table and a
// number that large is worth showing the working for: the biggest subnet
// provision.expand accepts is a /23, which is 510 addresses. They are probed
// ScanConcurrency at a time and each gets ScanTimeout, so the probing costs
// 510/16 rounded up, times two seconds -- 64 seconds -- and each address that
// answers costs a second handshake, which is local and fast.
//
// 110 seconds is that with room, and it is bounded from above by the server's
// own write timeout: cmd/holzkube-managerd's budget table asserts that this
// plus its slack fits inside it. A larger subnet would not fit, which is why
// expand refuses one rather than this number growing to meet it.
const ScanRouteBudget = 110 * time.Second

// InspectRouteBudget bounds reading one machine: a maintenance connection, the
// facts read, a disk list and a fingerprint handshake.
const InspectRouteBudget = 45 * time.Second

// The plan and the apply declare no budget, and that is a statement rather
// than an omission.
//
// Neither talks to a machine. The plan reads the cluster's control-plane count
// out of the local store and then validates, warns and renders locally; the
// apply does the same and then submits a job. Every node call a provisioning
// run makes happens inside that job, on the engine's own context and not on
// the request's -- which is what makes a closed browser tab lose a viewer
// rather than a run (PROV-11).

func provisionConfigured(d httpapi.Deps) *httpapi.Problem {
	if d.Provision == nil {
		return httpapi.Upstream("upstream.provision-unavailable",
			"This instance was started without the provisioning service.")
	}
	return nil
}

// ProvisionRoutes serves the wizard.
func ProvisionRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			// The notices the wizard opens with. A read of three constants,
			// served rather than written into the browser bundle because each
			// describes how a machine boots rather than anything this product
			// does, and a copy in the bundle is a copy that drifts.
			Method:          http.MethodGet,
			Pattern:         "/api/v1/provision/notices",
			RequiresSession: true,
			Handler:         handler(provisionNotices(d)),
		},
		{
			// A POST because it carries a subnet and a list of addresses, not
			// because it mutates. Nothing on any machine changes.
			Method:          http.MethodPost,
			Pattern:         "/api/v1/provision/scan",
			RequiresSession: true,
			Action:          "provision.scan",
			Handler:         handler(provisionScan(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/provision/inspect",
			RequiresSession: true,
			Action:          "provision.inspect",
			Handler:         handler(provisionInspect(d)),
		},
		{
			// The last screen before the apply. It validates, warns, and says
			// exactly what would be written -- and it writes nothing.
			Method:          http.MethodPost,
			Pattern:         "/api/v1/provision/plan",
			RequiresSession: true,
			Action:          "provision.plan",
			Handler:         handler(provisionPlan(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/provision/apply",
			RequiresSession: true,
			Destructive:     true,
			Action:          "provision.apply",
			ClusterScope:    clusterFromBody,
			Handler:         handler(provisionApply(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/provision/bootstrap-recovery",
			RequiresSession: true,
			Handler:         handler(listBootstrapRecovery(d)),
		},
		{
			// Recording what a person found about a bootstrap is destructive
			// in the sense that matters: it decides whether a later attempt is
			// refused as already-bootstrapped or allowed to run, and getting
			// it wrong bootstraps etcd twice. It is gated like a reset.
			Method:          http.MethodPost,
			Pattern:         "/api/v1/provision/bootstrap-recovery/{cluster}",
			RequiresSession: true,
			Destructive:     true,
			Action:          "provision.bootstrap-resolve",
			ClusterScope:    clusterFromPath,
			Handler:         handler(resolveBootstrapRecovery(d)),
		},
	}
}

func provisionNotices(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := provisionConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"notices": d.Provision.Notices()})
	}
}

func provisionScan(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := provisionConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			CIDR  string   `json:"cidr"`
			Addrs []string `json:"addrs"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}

		ctx, cancel := budgetedContext(r, ScanRouteBudget)
		defer cancel()

		found, err := d.Provision.Scan(ctx, provision.ScanRequest{CIDR: body.CIDR, Addrs: body.Addrs})
		if err != nil {
			writeProvisionError(w, r, d, err)
			return
		}

		// The notices ride along with the result rather than waiting for the
		// operator to have asked for them: this is the screen where "the
		// machine never appeared" is about to be interpreted, and both notices
		// are about exactly that.
		writeJSON(w, http.StatusOK, map[string]any{
			"found":   found,
			"notices": d.Provision.Notices(),
		})
	}
}

func provisionInspect(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := provisionConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			Addr string `json:"addr"`
			// Fingerprint is what the operator read off the machine's console.
			Fingerprint string `json:"fingerprint"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}
		if strings.TrimSpace(body.Addr) == "" {
			httpapi.WriteProblem(w, r, httpapi.Validation("Name the machine's address.",
				httpapi.FieldError{Field: "addr", Reason: "is required"}))
			return
		}

		ctx, cancel := budgetedContext(r, InspectRouteBudget)
		defer cancel()

		c, err := d.Provision.Inspect(ctx, body.Addr, body.Fingerprint)
		if err != nil {
			writeProvisionError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	}
}

// provisionRequestBody is what the plan and the apply both accept.
//
// They take the same body on purpose: the apply must be for the plan that was
// shown, and a body the two routes read differently is a body where the screen
// and the write can disagree.
type provisionRequestBody struct {
	// Cluster is read by the lock middleware on the apply route.
	Cluster string `json:"cluster"`

	Addr         string   `json:"addr"`
	UUID         string   `json:"uuid"`
	ControlPlane bool     `json:"control_plane"`
	InstallDisk  string   `json:"install_disk"`
	SchematicID  string   `json:"schematic_id"`
	TalosVersion string   `json:"talos_version"`
	Fingerprint  string   `json:"fingerprint"`
	Hostname     string   `json:"hostname"`
	PatchIDs     []string `json:"patch_ids"`

	// Confirmation is required on the apply and ignored on the plan.
	Confirmation string `json:"confirmation"`
}

func (b provisionRequestBody) request() provision.Request {
	return provision.Request{
		Addr:         b.Addr,
		UUID:         model.MachineID(b.UUID),
		Cluster:      model.ClusterID(b.Cluster),
		ControlPlane: b.ControlPlane,
		InstallDisk:  b.InstallDisk,
		SchematicID:  b.SchematicID,
		TalosVersion: b.TalosVersion,
		Fingerprint:  b.Fingerprint,
		Hostname:     b.Hostname,
		PatchIDs:     b.PatchIDs,
	}
}

func provisionPlan(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := provisionConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body provisionRequestBody
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}

		preview, err := d.Provision.Plan(r.Context(), body.request())
		if err != nil {
			writeProvisionError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, preview)
	}
}

func provisionApply(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := provisionConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		if p := jobsConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body provisionRequestBody
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}

		req := body.request()

		// Validated here as well as in the job, because a plan that is missing
		// its UUID must be refused before it is accepted rather than after: a
		// 202 for a run that cannot start is a 202 that says the machine is
		// being provisioned.
		if _, err := d.Provision.Plan(r.Context(), req); err != nil {
			writeProvisionError(w, r, d, err)
			return
		}

		params := req.Params()

		// The intent is rebuilt from what was submitted, never read out of the
		// token: a token that carried its own description would authorise
		// whatever it said while the request did something else.
		if err := d.Confirmer.Check(body.Confirmation, jobs.Intent{
			Action:  string(provision.JobKindProvision),
			Machine: string(req.UUID),
			Params:  params,
		}); err != nil {
			writeJobError(w, r, d, err)
			return
		}

		actor := ""
		if u, ok := d.Auth.CurrentUser(r.Context()); ok {
			actor = u.Username
		}

		j, err := d.Jobs.Submit(r.Context(), model.Job{
			Kind:    provision.JobKindProvision,
			Cluster: req.Cluster,
			Machine: req.UUID,
			Params:  params,
			Actor:   actor,
		})
		if err != nil {
			writeJobError(w, r, d, err)
			return
		}

		w.Header().Set("Location", "/api/v1/jobs/"+string(j.ID))
		writeJSON(w, http.StatusAccepted, map[string]any{
			"job":   j,
			"topic": string(jobs.Topic(j.ID)),
		})
	}
}

func listBootstrapRecovery(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := provisionConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		pending, err := d.Provision.PendingBootstraps()
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"pending": pending,
			// Said on the screen rather than only in the package
			// documentation, because the operator reading this is being asked
			// to make the decision the four mechanisms exist to avoid, and the
			// one thing they must not do is guess.
			"guidance": "An attempt with no recorded outcome is one this installation cannot decide. " +
				"Run `talosctl -n <node> etcd members` against the machine named below, or look at " +
				"its console: an etcd with members was bootstrapped, and one that is not running " +
				"was not. Record what you find. Do not retry a bootstrap to find out -- a second " +
				"bootstrap over a running etcd is what destroys a cluster. If a provisioning job " +
				"for this cluster is still running, this is that attempt in flight and there is " +
				"nothing to resolve yet.",
		})
	}
}

func resolveBootstrapRecovery(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := provisionConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			// Bootstrapped is what the operator found, and there is no
			// default: a missing field is a refusal rather than a "no".
			Bootstrapped *bool `json:"bootstrapped"`

			// Note is what they looked at. It is required, because this record
			// is the only account of a bootstrap that anything will ever have.
			Note string `json:"note"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}
		if body.Bootstrapped == nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"Say whether etcd was bootstrapped. There is no default: guessing here is the "+
					"operation the whole bootstrap record exists to prevent.",
				httpapi.FieldError{Field: "bootstrapped", Reason: "is required"}))
			return
		}

		cluster := model.ClusterID(r.PathValue("cluster"))
		if err := d.Provision.ResolveBootstrap(cluster, *body.Bootstrapped, strings.TrimSpace(body.Note)); err != nil {
			writeProvisionError(w, r, d, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"cluster":      string(cluster),
			"bootstrapped": *body.Bootstrapped,
		})
	}
}

// clusterFromPath reads the cluster the lock middleware protects off the path.
func clusterFromPath(r *http.Request) (string, error) {
	return r.PathValue("cluster"), nil
}

// writeProvisionError maps this package's refusals onto the taxonomy.
//
// Each branch is a code minted in this phase. Without them every one of these
// would arrive as internal.unexpected, which by contract carries no detail,
// and would stay that way forever in an archive with no deletion path.
func writeProvisionError(w http.ResponseWriter, r *http.Request, d httpapi.Deps, err error) {
	switch {
	case errors.Is(err, provision.ErrWrongMachine):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeValidation,
			Title:  "That is not the machine this plan is for",
			Status: http.StatusBadRequest,
			Detail: err.Error(),
			Code:   httpapi.CodeWrongMachine,
		})
	case errors.Is(err, provision.ErrNotInMaintenance):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeValidation,
			Title:  "That machine is not waiting for a configuration",
			Status: http.StatusBadRequest,
			Detail: err.Error(),
			Code:   httpapi.CodeNotInMaintenance,
		})
	case errors.Is(err, provision.ErrBootstrapUnclear):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeConflict,
			Title:  "A previous bootstrap has no recorded outcome",
			Status: http.StatusConflict,
			Detail: err.Error(),
			Code:   httpapi.CodeBootstrapUnclear,
		})
	case errors.Is(err, provision.ErrBootstrapInProgress):
		httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeBootstrapInProgress, err.Error()))
	case errors.Is(err, provision.ErrAlreadyBootstrapped):
		httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeAlreadyBootstrapped, err.Error()))
	default:
		if code, ok := upstreamNodeCode(err); ok {
			httpapi.WriteProblem(w, r, httpapi.Upstream(code, err.Error()))
			return
		}
		if strings.HasPrefix(err.Error(), "provision: ") {
			// This package's own validation refusals -- a plan with no UUID, a
			// subnet too large to scan, a resolution with no note -- are the
			// operator's to fix and each carries its own sentence.
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}
		httpapi.WriteInternal(w, r, d.Logger, err)
	}
}
