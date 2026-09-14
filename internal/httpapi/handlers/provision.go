package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/diskencryption"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/imagefactory"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/provision"
	"github.com/holzcloud/holzkube-manager/internal/talos"
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
			MinRole:         model.RoleReader,
			Handler:         handler(provisionNotices(d)),
		},
		{
			// A POST because it carries a subnet and a list of addresses, not
			// because it mutates. Nothing on any machine changes.
			Method:          http.MethodPost,
			Pattern:         "/api/v1/provision/scan",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "provision.scan",
			Handler:         handler(provisionScan(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/provision/inspect",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "provision.inspect",
			Handler:         handler(provisionInspect(d)),
		},
		{
			// The last screen before the apply. It validates, warns, and says
			// exactly what would be written -- and it writes nothing.
			Method:          http.MethodPost,
			Pattern:         "/api/v1/provision/plan",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "provision.plan",
			Handler:         handler(provisionPlan(d)),
		},
		{
			// Provisioning needs its own confirmation route, and the reason is
			// not symmetry: the machine-scoped one reads the machine out of
			// the inventory to check what was typed against its hostname, and
			// a machine being provisioned is not in the inventory. It cannot
			// be -- it has no UUID recorded, no cluster and no credentials
			// until the run that is being confirmed has finished.
			Method:          http.MethodPost,
			Pattern:         "/api/v1/provision/confirm",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "provision.confirm",
			Handler:         handler(provisionConfirm(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/provision/apply",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Destructive:     true,
			Action:          "provision.apply",
			ClusterScope:    clusterFromBody,
			Handler:         handler(provisionApply(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/provision/bootstrap-recovery",
			RequiresSession: true,
			MinRole:         model.RoleReader,
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
			MinRole:         model.RoleOperator,
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
			httpapi.WriteProblem(w, r, decodeProblem(err))
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
			httpapi.WriteProblem(w, r, decodeProblem(err))
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

	// Encryption asks for the node's system volumes to be encrypted at
	// install.
	//
	Encryption *encryptionBody `json:"encryption,omitempty"`

	// SecureBoot says the machine booted the SecureBoot variant of its
	// schematic.
	//
	// It is stated by the operator rather than derived, and that is not a
	// shortcut: a schematic id does not carry SecureBoot. One id resolves
	// under `metal-installer` and under `metal-installer-secureboot` to two
	// different images, picked by repository name alone, so the only party
	// that knows which image this machine actually booted is whoever wrote the
	// USB stick.
	//
	// It selects the installer, which Talos requires to match -- the ordinary
	// installer does not produce a SecureBoot node -- and it is what the TPM
	// disk-encryption kind is checked against.
	SecureBoot bool `json:"secureboot"`

	// Confirmation is required on the apply and ignored on the plan.
	Confirmation string `json:"confirmation"`
}

// encryptionBody is the wire shape. Kind is a string rather than the domain
// type so that `static` and `kms` arrive here and are answered with the reason
// they are not offered, instead of being rejected as unparsable values.
type encryptionBody struct {
	State     bool   `json:"state"`
	Ephemeral bool   `json:"ephemeral"`
	Kind      string `json:"kind"`
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
		SecureBoot:   b.SecureBoot,
	}
}

// resolveInstaller fills in the exact reference `.machine.install.image` gets.
//
// Resolved here rather than assembled in the provisioning package, and carried
// on the request from this point on, so that the reference shown on the
// confirmation screen is the one the job writes. The previous arrangement
// rebuilt the string at install time from parts, which is how a reference and
// the confirmation of it came to be able to disagree.
//
// A machine with no schematic gets Talos's own published installer: there is
// no Factory repository to resolve for it.
//
// Everything else goes through the Factory, and a failure is a refusal with no
// reference rather than a fallback. That is the decision internal/imagefactory
// already made for the asset panel, for the reason that applies twice as hard
// here: substituting the ordinary installer for the SecureBoot one produces a
// node that installs, joins, and is not SecureBoot, and nothing afterwards
// says so.
func resolveInstaller(
	ctx context.Context, d httpapi.Deps, req provision.Request,
) (provision.Request, []imagefactory.Warning, *httpapi.Problem) {
	none := make([]imagefactory.Warning, 0)

	if req.SchematicID == "" {
		req.InstallerImage = provision.StockInstaller(req.TalosVersion)
		return req, none, nil
	}
	if d.Factory == nil {
		return req, none, httpapi.Validation(
			"This machine was built from an Image Factory schematic, and the installer reference " +
				"for it has to be resolved against the Factory -- which this installation is not " +
				"configured with. Configure --image-factory, or provision a machine that boots " +
				"the stock image.")
	}

	// The architecture comes from the stored schematic and is never a constant
	// here. FACT-03 says so and the reason is concrete: this product is
	// developed on arm64 and its target hardware is amd64, so an architecture
	// baked in is a bug that works perfectly on the machine that wrote it.
	rec, err := d.Store.Schematics().Get(ctx, model.SchematicID(req.SchematicID))
	if err != nil {
		return req, none, httpapi.Validation(
			"This installation does not hold the schematic this machine booted, so the "+
				"architecture to resolve its installer for is unknown. Create or import the "+
				"schematic first.",
			httpapi.FieldError{Field: "schematic_id", Reason: "not found"})
	}

	arch := imagefactory.Arch(rec.Arch)
	if !arch.Valid() {
		return req, none, httpapi.Validation(
			"The stored schematic does not name an architecture this product builds assets for, " +
				"so its installer cannot be resolved.")
	}

	ref, warnings, err := d.Factory.InstallerImage(ctx, imagefactory.AssetRequest{
		SchematicID: req.SchematicID,
		Version:     req.TalosVersion,
		Arch:        arch,
		Platform:    imagefactory.PlatformMetal,
		SecureBoot:  req.SecureBoot,
	})
	if err != nil {
		return req, none, httpapi.Validation(
			"The installer for this schematic could not be resolved: " + err.Error() +
				". Nothing is substituted here, deliberately: installing the ordinary installer " +
				"for a SecureBoot machine produces a node that installs, joins, and is not " +
				"SecureBoot.")
	}

	req.InstallerImage = ref
	return req, warnings, nil
}

// withEncryption resolves the encryption half of a request.
//
// It parses the key kind and reads the schematic's SecureBoot flag out of the
// store. Both refusals it can produce are conditions an operator can act on,
// so they come back as problems rather than as an internal error.
func (b provisionRequestBody) withEncryption(req provision.Request) (provision.Request, *httpapi.Problem) {
	if b.Encryption == nil || (!b.Encryption.State && !b.Encryption.Ephemeral) {
		return req, nil
	}

	kind, err := diskencryption.ParseKind(b.Encryption.Kind)
	if err != nil {
		return req, httpapi.Validation(err.Error(),
			httpapi.FieldError{Field: "encryption.kind", Reason: "not an offered key kind"})
	}

	req.Encryption = &diskencryption.Request{
		State:     b.Encryption.State,
		Ephemeral: b.Encryption.Ephemeral,
		Kind:      kind,
	}
	return req, nil
}

// prepare turns a request body into the request every provisioning route acts
// on: the installer resolved, the encryption parsed, the operator's SecureBoot
// answer carried through.
//
// One function for all three routes -- plan, confirm and apply -- because they
// must agree. A confirmation issued against one reference and an apply that
// rebuilt a different one is a confirmation of something that did not happen,
// which is the failure the confirmation exists to prevent.
func prepare(
	ctx context.Context, d httpapi.Deps, body provisionRequestBody,
) (provision.Request, []imagefactory.Warning, *httpapi.Problem) {
	req, warnings, problem := resolveInstaller(ctx, d, body.request())
	if problem != nil {
		return req, warnings, problem
	}
	req, problem = body.withEncryption(req)
	return req, warnings, problem
}

func provisionPlan(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := provisionConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body provisionRequestBody
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}

		req, warnings, problem := prepare(r.Context(), d, body)
		if problem != nil {
			httpapi.WriteProblem(w, r, problem)
			return
		}

		preview, err := d.Provision.Plan(r.Context(), req)
		if err != nil {
			writeProvisionError(w, r, d, err)
			return
		}

		// The Factory's own warnings about the installer name it resolved --
		// a name reached past a candidate that never answered is usable and
		// provisional, and this is the screen where that matters.
		for _, warning := range warnings {
			preview.Warnings = append(preview.Warnings, warning.Detail)
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
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}

		req, _, problem := prepare(r.Context(), d, body)
		if problem != nil {
			httpapi.WriteProblem(w, r, problem)
			return
		}

		// Validated here as well as in the job, because a plan that is missing
		// its UUID must be refused before it is accepted rather than after: a
		// 202 for a run that cannot start is a 202 that says the machine is
		// being provisioned.
		if _, err := d.Provision.Plan(r.Context(), req); err != nil {
			writeProvisionError(w, r, d, err)
			return
		}

		params, err := req.Params()
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}

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

// provisionConfirm issues a token for exactly the run described.
//
// What the operator types is the **install disk device**, and that is the
// choice worth arguing about. A reset asks for the machine's hostname because
// the hostname is what they can check against the machine in front of them. A
// machine in maintenance mode has no hostname worth checking -- it is whatever
// the ISO decided -- and the thing that is actually about to be destroyed is a
// disk. Typing `/dev/nvme0n1` is the operator saying which device they mean on
// a screen that lists three of them with their sizes, models and serials.
func provisionConfirm(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := provisionConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		if p := jobsConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			provisionRequestBody
			// Typed is what the operator typed into the confirmation box. It
			// is checked here, once, so that a client that skipped the box
			// cannot get a token at all.
			Typed string `json:"typed"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}

		req, _, problem := prepare(r.Context(), d, body.provisionRequestBody)
		if problem != nil {
			httpapi.WriteProblem(w, r, problem)
			return
		}

		// Validated before a token is issued, so that a token cannot exist for
		// a run that would be refused anyway.
		if _, err := d.Provision.Plan(r.Context(), req); err != nil {
			writeProvisionError(w, r, d, err)
			return
		}

		if strings.TrimSpace(body.Typed) != req.InstallDisk {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"Type the device Talos will install to, exactly as it is listed. It is the disk "+
					"that gets written, and typing it is how this screen knows you mean that one.",
				httpapi.FieldError{Field: "typed", Reason: "does not match the install disk"}))
			return
		}

		params, err := req.Params()
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}
		token, expires := d.Confirmer.Issue(jobs.Intent{
			Action:  string(provision.JobKindProvision),
			Machine: string(req.UUID),
			Params:  params,
		})

		writeJSON(w, http.StatusOK, map[string]any{
			"token":   token,
			"expires": expires.Format(time.RFC3339),
			// Echoed back so the client can show exactly what this token
			// authorises rather than what it believes it asked for.
			"action": string(provision.JobKindProvision),
			"params": params,
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
			httpapi.WriteProblem(w, r, decodeProblem(err))
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
	case errors.Is(err, talos.ErrFingerprintPin):
		// The same code the adoption path uses for the same fact, caught at a
		// different moment: there it is compared before connecting, here
		// during the handshake, where it is the only trust anchor there is.
		// What it must not become is an upstream failure -- something
		// answered.
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeValidation,
			Title:  "That machine is not the one you confirmed",
			Status: http.StatusBadRequest,
			Detail: err.Error(),
			Code:   httpapi.CodeFingerprintMismatch,
		})
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
