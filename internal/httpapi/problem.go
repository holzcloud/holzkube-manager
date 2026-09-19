package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/holzcloud/holzkube-manager/internal/httpapi/middleware"
)

// ProblemContentType is the media type every error response carries.
const ProblemContentType = "application/problem+json"

// ProblemBaseURI roots the taxonomy. Every type is an absolute URI under it;
// about:blank is never used, because a client that wants to branch on the kind
// of failure needs something stable to branch on.
//
// It is a URN and not a URL, and that is a decision rather than a detail. A
// problem type here is an identifier: nothing dereferences it, the interface
// branches on Code (web/src/lib/problem.ts), and no comparison against a type
// URI exists anywhere in this repository. An https base bought none of that and
// cost two things -- it hardcoded a vendor domain into a contract every AGPL
// redistributor has to ship unchanged, and it read as a promise that a page
// exists at that address. A URN makes the absence of that page obvious.
//
// The base is deployment-independent and deliberately not configurable: no
// flag, no environment variable, no build tag moves it. A per-install base was
// considered and rejected, because two installations emitting different types
// for the same error would force a per-install special case into every
// third-party client -- solving a problem nobody has at the cost of the one
// property the field has.
//
// The namespace identifier is not registered with IANA. RFC 9457 asks for a
// URI, not a registered namespace, and the value is an opaque identifier
// either way.
const ProblemBaseURI = "urn:holzkube-manager:problem:"

// The taxonomy is closed and stable. These URIs and the code tokens below are
// a public contract: clients may match on them, so they never change -- and
// now they name a value no deployment can move.
// Plan 01 task 4 completes the set and pins it in docs/api-contract.md.
//
// Each entry is composed from ProblemBaseURI rather than written out beside it.
// The failure mode of a re-rooting is landing on twelve of thirteen, and a
// constant expression makes that impossible to express. The suffixes are the
// parts clients may already match on and do not move with the base.
const (
	TypeValidation           = ProblemBaseURI + "validation"
	TypeUnauthenticated      = ProblemBaseURI + "unauthenticated"
	TypeCSRF                 = ProblemBaseURI + "csrf"
	TypeForbidden            = ProblemBaseURI + "forbidden"
	TypeNotFound             = ProblemBaseURI + "not-found"
	TypeMethodNotAllowed     = ProblemBaseURI + "method-not-allowed"
	TypeConflict             = ProblemBaseURI + "conflict"
	TypeUnsupportedMediaType = ProblemBaseURI + "unsupported-media-type"
	TypeSudoRequired         = ProblemBaseURI + "sudo-required"
	TypeRateLimited          = ProblemBaseURI + "rate-limited"
	TypeInternal             = ProblemBaseURI + "internal"
	TypeSetupRequired        = ProblemBaseURI + "setup-required"
	TypeUpstream             = ProblemBaseURI + "upstream"
)

// The reserved code tokens of the upstream family, minted in the same commit as
// TypeUpstream so that no later plan invents a divergent spelling of a code the
// contract says never changes. Plan 02-05 emits the node codes and plan 02-06
// the factory codes; both reference these identifiers rather than string
// literals, which is what keeps the two halves of the family in step.
const (
	// CodeUpstreamNodeUnreachable: the Talos node refused or dropped the
	// connection, or does not resolve. The operator can act on this -- it is
	// almost always a wrong address, a firewall or a node that is not up.
	CodeUpstreamNodeUnreachable = "upstream.node-unreachable"

	// CodeUpstreamNodeTimeout: the node accepted the connection and then did not
	// answer in time. Distinct from unreachable because it is retryable and
	// points at load or a wedged service rather than at the address.
	CodeUpstreamNodeTimeout = "upstream.node-timeout"

	// CodeUpstreamFactoryUnavailable: factory.talos.dev did not answer, answered
	// 5xx, or answered something holzkube-manager will not decode. Retryable.
	CodeUpstreamFactoryUnavailable = "upstream.factory-unavailable"

	// CodeUpstreamFactoryRejected: the Factory answered, and the answer was a
	// refusal of what holzkube-manager asked for. Not retryable: retrying an identical
	// rejected request produces an identical rejection.
	CodeUpstreamFactoryRejected = "upstream.factory-rejected"
)

// The codes phase 3 mints, under types the taxonomy already closes over.
//
// They are minted deliberately and in the same commit as the routes that emit
// them, because the alternative is not a missing code: it is every one of
// these failures arriving as internal.unexpected, which by contract carries no
// detail at all, and being kept that way forever in an archive with no
// deletion path.
const (
	// CodeNotControlPlane: the adoption was aimed at a node whose machine
	// configuration carries no control-plane material. The remedy is specific
	// and nothing else implies it -- name a control-plane node -- which is why
	// it is not folded into the generic validation code (D-05).
	CodeNotControlPlane = "validation.node-not-controlplane"

	// CodeTalosconfigInvalid: the uploaded file is not a usable talosconfig.
	// One code for "not YAML", "no context" and "no client certificate": all
	// three mean the wrong file was uploaded, and distinguishing them would
	// narrow a guess for somebody who should not be guessing.
	CodeTalosconfigInvalid = "validation.talosconfig-invalid"

	// CodeFingerprintMismatch: the node presented a certificate other than the
	// one the operator confirmed. It is a validation failure rather than an
	// upstream one because what is wrong is the value that was submitted --
	// or, worse, what is answering at that address.
	//
	// Two paths produce it and they are the same fact at different moments.
	// The adoption path compares before it connects (talos.ErrFingerprintMismatch);
	// the maintenance path compares during the handshake, where the pin is the
	// only trust anchor there is (talos.ErrFingerprintPin, PROV-04). What must
	// never happen is the second arriving as upstream.node-unreachable, which
	// is what it looked like until the pin recorded what it saw: something
	// answered, and telling the operator otherwise sends them to check a cable
	// while somebody else is on the wire.
	CodeFingerprintMismatch = "validation.fingerprint-mismatch"

	// CodeClusterLocked: the cluster was adopted read-only and this request
	// would have changed something (INV-12, D-22).
	CodeClusterLocked = "forbidden.cluster-locked"

	// CodeNoKubernetesAuthority: the cluster's stored bundle carries no
	// Kubernetes authority, so this product cannot mint itself a certificate
	// for that cluster's API server. A conflict rather than an upstream
	// failure: nothing is wrong with the cluster, and the repair is to adopt it
	// again with a talosconfig that carries the authority.
	CodeNoKubernetesAuthority = "conflict.no-kubernetes-authority"

	// CodeNoKubernetesEndpoint: no control-plane node could say where the
	// cluster's Kubernetes API server is. Upstream, because the answer lives on
	// the nodes and they were asked.
	CodeNoKubernetesEndpoint = "upstream.no-kubernetes-endpoint"

	// CodeKubernetesUnreachable: the cluster's Kubernetes API server did not
	// answer. Its own code so that a screen can say "the cluster was not
	// asked" rather than rendering an empty list, which is the claim INV-08
	// forbids one layer down.
	CodeKubernetesUnreachable = "upstream.kubernetes-unreachable"

	// CodeNoMachinesToRotate: a CA rotation was asked for on a cluster with no
	// machines recorded. A conflict rather than a validation error: the request
	// is fine and the inventory is empty.
	CodeNoMachinesToRotate = "conflict.no-machines-to-rotate"

	// CodeAlreadyAdopted: the adoption was aimed at a node another stored
	// cluster already has. A conflict with what is stored rather than a bad
	// value: the same request succeeds once that cluster is forgotten, and the
	// detail names it (ledger 139).
	CodeAlreadyAdopted = "conflict.cluster-already-adopted"

	// CodeClusterBusy: another mutating job holds this cluster's lease
	// (JOB-03). It is a conflict rather than a refusal: the request is fine,
	// the moment is not, and retrying later is the remedy.
	CodeClusterBusy = "store.cluster-busy"

	// CodeJobFinished: a cancel arrived for a job that has already finished.
	CodeJobFinished = "store.job-finished"

	// CodeConfirmationInvalid and CodeConfirmationExpired are the two ways a
	// confirmation fails (JOB-08). They are separate codes because the
	// remedies differ: expired means read the dialog again, invalid means the
	// request is not the one that was confirmed.
	CodeConfirmationInvalid = "forbidden.confirmation-invalid"
	CodeConfirmationExpired = "forbidden.confirmation-expired"

	// CodeStagedPending: a second staged apply arrived while one is already
	// waiting for the node's next boot (CFG-08). Talos would replace the first
	// without saying so, and exactly one of the two changes would happen.
	CodeStagedPending = "store.staged-pending"

	// CodeForbiddenRole: the account is authenticated and does not carry the
	// role this route needs (V2-AUTH-02).
	//
	// It is a 403 and not a 404, and the difference is a judgement rather than
	// an oversight. Hiding the route would hide it from an operator who has an
	// account on this instance and is simply the wrong one for this job --
	// which turns "ask an admin" into "file a bug". Everyone who can receive
	// this has already authenticated against this installation.
	CodeForbiddenRole = "forbidden.role"

	// CodeDryRun: this instance was started with --dry-run and applies
	// nothing. It is a forbidden rather than an internal, because the request
	// is fine and the instance is the reason.
	CodeDryRun = "forbidden.dry-run"

	// CodePatchInvalid and CodePatchNotStrategic are the two ways a patch is
	// refused. They are separate because the remedies differ: invalid means
	// fix the patch, not-strategic means rewrite it in the other form and is
	// a refusal of a whole category.
	CodePatchInvalid      = "validation.patch-invalid"
	CodePatchNotStrategic = "validation.patch-not-strategic"

	// CodeWrongMachine: the machine at the address is not the one the
	// provisioning plan names (PROV-05). It has its own code because it is the
	// only refusal in this product that means "the thing you were about to
	// wipe is somebody else's", and a client showing it as a generic
	// validation failure would show it as a typo.
	CodeWrongMachine = "validation.wrong-machine"

	// CodeNotInMaintenance: the machine answered and already has a
	// configuration. Separate from wrong-machine because the remedy is the
	// opposite: that one means check the address, this one means this machine
	// is in use.
	CodeNotInMaintenance = "validation.not-in-maintenance"

	// CodeBootstrapUnclear: a previous etcd bootstrap has no recorded outcome
	// (PROV-10). It is a conflict rather than a validation failure because
	// nothing about the request is wrong -- the cluster's recorded state is
	// undecided, and the remedy is a person looking at the node, never a
	// retry.
	CodeBootstrapUnclear = "conflict.bootstrap-unclear"

	// CodeBootstrapInProgress and CodeAlreadyBootstrapped are the two
	// bootstrap refusals that are not a fault: one means wait, the other means
	// the cluster is already in the state that was wanted. They are separate
	// codes because a client showing "already bootstrapped" as a failure would
	// send somebody to fix something that works.
	CodeBootstrapInProgress = "conflict.bootstrap-in-progress"
	CodeAlreadyBootstrapped = "conflict.already-bootstrapped"

	// CodeUpgradeBlocked: the plan this run was built from has something
	// standing in the way (UPG-02, UPG-03, UPG-04, UPG-06). It is one code
	// rather than four because the plan document carries which, and a client
	// showing the plan does not need the code to tell it what to render.
	CodeUpgradeBlocked = "conflict.upgrade-blocked"

	// CodeWouldStrand: this upgrade would leave the running Kubernetes version
	// outside what the target Talos supports (UPG-06). It has its own code
	// because it is the one refusal here that is about a state with no good
	// way out -- the upgrade that would fix it is performed by the same Talos
	// that no longer supports the version it is upgrading from.
	CodeWouldStrand = "conflict.would-strand-kubernetes"

	// CodeLastVotingMember: removing this etcd member would leave the cluster
	// without a quorum (UPG-11). Distinct from upgrade-blocked because a
	// removal has no node coming back afterwards.
	CodeLastVotingMember = "conflict.last-voting-member"

	// CodeNoCertificateAuthority: this cluster's stored bundle carries an
	// admin certificate and no authority key, so nothing new can be issued
	// from it (V2-OPS-02). It is a condition with a specific repair rather
	// than a fault: the cluster was adopted from a talosconfig that did not
	// carry the authority.
	CodeNoCertificateAuthority = "conflict.no-certificate-authority"

	// CodeNotAPerson: a password operation aimed at a service account, which
	// has none. The mirror of conflict.not-a-service-account, and it is a
	// named conflict rather than the login path's deliberately blank refusal
	// because this caller has already proven which account it is.
	CodeNotAPerson = "conflict.not-a-person"

	// CodeCertificateRejected: a freshly minted client certificate reached no
	// node, so the old one was kept. Its own code because it is the *safe*
	// outcome of a renewal and not a failure of one -- nothing changed, and a
	// client that treated it as an error would tell an operator their cluster
	// is broken when what happened is that this refused to break it.
	CodeCertificateRejected = "conflict.certificate-rejected"

	// CodeUnknownSchematic: this node's Image Factory schematic could not be
	// read, so an upgrade would install a stock system and silently remove
	// every extension it has (UPG-03).
	CodeUnknownSchematic = "conflict.unknown-schematic"

	// CodeNotUpgraded: the node came back running something other than what
	// was installed (UPG-07). It is the code for "the API said OK and the node
	// disagrees", which is the whole reason that check exists.
	CodeNotUpgraded = "conflict.not-upgraded"

	// CodeNodeRefused: the node answered and the answer was no. It exists
	// because talos.KindRejected deliberately has no upstream code -- "the
	// node refused" is not an availability problem -- and without a code of
	// its own every such refusal arrived as internal.unexpected, which by
	// contract carries no detail. On the etcd routes it almost always means
	// etcd is not running on the node that was asked, and that sentence is
	// worth keeping.
	CodeNodeRefused = "conflict.node-refused"
)

// ClusterLocked reports a mutation refused by a cluster's read-only lock.
//
// It carries a detail, unlike Unauthenticated: there is nothing to conceal
// from a caller who already holds a session, and "this cluster is locked" is
// useless without "and here is how it got that way".
func ClusterLocked(detail string) *Problem {
	return &Problem{
		Type:   TypeForbidden,
		Title:  "The cluster is locked read-only",
		Status: http.StatusForbidden,
		Detail: detail,
		Code:   CodeClusterLocked,
	}
}

// FieldError names one failed field inside a validation problem.
type FieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// Problem is an RFC 9457 problem detail.
//
// Code is holzkube-manager's addition to the standard members: a stable machine token
// that is finer-grained than Type, so a client can distinguish
// setup.already-completed from store.conflict without parsing prose.
type Problem struct {
	Type     string       `json:"type"`
	Title    string       `json:"title"`
	Status   int          `json:"status"`
	Detail   string       `json:"detail,omitempty"`
	Instance string       `json:"instance,omitempty"`
	Code     string       `json:"code"`
	Errors   []FieldError `json:"errors,omitempty"`

	// retryAfter, when set, is emitted as a Retry-After header rather than a
	// body member.
	retryAfter int
}

// Error lets a Problem travel as an error value.
func (p *Problem) Error() string { return p.Code + ": " + p.Title }

// Validation reports a body or query that violates the schema.
func Validation(detail string, errs ...FieldError) *Problem {
	return &Problem{
		Type:   TypeValidation,
		Title:  "Request is not valid",
		Status: http.StatusBadRequest,
		Detail: detail,
		Code:   "validation.failed",
		Errors: errs,
	}
}

// Unauthenticated reports a missing, expired or rejected session.
//
// It takes no arguments on purpose. An unknown username and a wrong password
// must produce a byte-identical response, and the cheapest way to guarantee
// that is to give callers no way to vary it.
func Unauthenticated() *Problem {
	return &Problem{
		Type:   TypeUnauthenticated,
		Title:  "Authentication required",
		Status: http.StatusUnauthorized,
		Detail: "The request has no valid session, or the supplied credentials were rejected.",
		Code:   "auth.unauthenticated",
	}
}

// CSRFFailed reports unmet CSRF preconditions.
func CSRFFailed(detail string) *Problem {
	return &Problem{
		Type:   TypeCSRF,
		Title:  "Cross-site request preconditions not met",
		Status: http.StatusForbidden,
		Detail: detail,
		Code:   "csrf.precondition-unmet",
	}
}

// NotFound reports a resource that does not exist.
func NotFound(code, detail string) *Problem {
	return &Problem{
		Type:   TypeNotFound,
		Title:  "Resource not found",
		Status: http.StatusNotFound,
		Detail: detail,
		Code:   code,
	}
}

// Forbidden reports a valid session that is not allowed to do this.
//
// It is distinct from Unauthenticated on purpose: "who are you" and "you may
// not" are different answers, and collapsing them makes a permissions bug look
// like a login bug.
func Forbidden(code, detail string) *Problem {
	return &Problem{
		Type:   TypeForbidden,
		Title:  "Not permitted",
		Status: http.StatusForbidden,
		Detail: detail,
		Code:   code,
	}
}

// MethodNotAllowed reports a known path reached with the wrong method.
func MethodNotAllowed(detail string) *Problem {
	return &Problem{
		Type:   TypeMethodNotAllowed,
		Title:  "Method not allowed",
		Status: http.StatusMethodNotAllowed,
		Detail: detail,
		Code:   "method.not-allowed",
	}
}

// UnsupportedMediaType reports a body holzkube-manager will not parse.
//
// Reserved rather than dead: in phase 1 the CSRF preconditions reject a
// non-JSON mutating request at 403 before a handler ever inspects the body, so
// nothing emits this yet. It is minted now because the taxonomy is a closed
// contract that wave 2 codes against, and adding an entry later would mean
// changing that contract.
func UnsupportedMediaType(detail string) *Problem {
	return &Problem{
		Type:   TypeUnsupportedMediaType,
		Title:  "Unsupported media type",
		Status: http.StatusUnsupportedMediaType,
		Detail: detail,
		Code:   "media.unsupported",
	}
}

// Conflict reports a revision clash or an operation that is no longer available.
func Conflict(code, detail string) *Problem {
	return &Problem{
		Type:   TypeConflict,
		Title:  "Request conflicts with the current state",
		Status: http.StatusConflict,
		Detail: detail,
		Code:   code,
	}
}

// SudoRequired reports a destructive route reached outside the sudo window.
func SudoRequired() *Problem {
	return &Problem{
		Type:   TypeSudoRequired,
		Title:  "Re-authentication required",
		Status: http.StatusPreconditionRequired,
		Detail: "This action is destructive and requires a recent re-authentication.",
		Code:   "sudo.required",
	}
}

// RateLimited reports a throttled caller. holzkube-manager delays, it never locks out:
// there is exactly one operator and no recovery path by design (D-08).
func RateLimited(retryAfterSeconds int) *Problem {
	return &Problem{
		Type:       TypeRateLimited,
		Title:      "Too many attempts",
		Status:     http.StatusTooManyRequests,
		Detail:     "Further attempts are being delayed. Wait and try again.",
		Code:       "ratelimit.delayed",
		retryAfter: retryAfterSeconds,
	}
}

// Internal reports an unexpected failure.
//
// The error is accepted so callers can pass it, and then deliberately dropped
// from the response: a Go error string routinely carries a filesystem path, a
// store-internal message or a wrapped stack of package names. This binary holds
// cluster PKI, so leaking any of that to a client is free reconnaissance. The
// client gets only the instance; the error itself goes to the log, where the
// same instance identifies it.
func Internal(err error) *Problem {
	_ = err
	return &Problem{
		Type:   TypeInternal,
		Title:  "Internal error",
		Status: http.StatusInternalServerError,
		Detail: "",
		Code:   "internal.unexpected",
	}
}

// Upstream reports that a dependency outside this process did not answer, or
// answered with a refusal.
//
// It exists because Internal discards its error by design. Without this type an
// unreachable Talos node and an unreachable Image Factory both fall through to
// internal.unexpected, which by contract carries instance and nothing else --
// and that detail-free record is what lands, permanently, in an audit archive
// that D-16 gives no deletion path. The operator would be left with a 500 and a
// request id for a failure that was never holzkube-manager's to begin with.
//
// One type, four codes: the type axis stays a taxonomy of failure kinds rather
// than a register of dependencies, and the code stays the fine-grained
// discriminator the contract already promises. A third upstream in a later
// phase costs a code, not a type.
//
// detail is a string and never an error, deliberately. Unlike Internal, this
// type does put its detail on the wire, so a constructor taking an error would
// be a constructor somebody eventually hands a wrapped Go error -- filesystem
// paths, node addresses and package names included. Taking a string means every
// detail a client sees was typed out by somebody who could read it first.
func Upstream(code, detail string) *Problem {
	return &Problem{
		Type:   TypeUpstream,
		Title:  "An upstream dependency did not answer",
		Status: http.StatusBadGateway,
		Detail: detail,
		Code:   code,
	}
}

// SetupRequired reports that no operator account exists yet.
func SetupRequired() *Problem {
	return &Problem{
		Type:   TypeSetupRequired,
		Title:  "Setup required",
		Status: http.StatusServiceUnavailable,
		Detail: "No operator account exists yet. Complete the setup wizard first.",
		Code:   "setup.required",
	}
}

// WriteProblem serialises a problem as the response.
func WriteProblem(w http.ResponseWriter, r *http.Request, p *Problem) {
	if p == nil {
		p = Internal(nil)
	}
	if p.Instance == "" {
		if id := middleware.RequestIDFromContext(r.Context()); id != "" {
			p.Instance = "/requests/" + id
		}
	}
	if p.retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(p.retryAfter))
	}
	w.Header().Set("Content-Type", ProblemContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(p.Status)

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(p)
}

// WriteInternal logs the real cause and returns the redacted problem.
func WriteInternal(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	p := Internal(err)
	if p.Instance == "" {
		if id := middleware.RequestIDFromContext(r.Context()); id != "" {
			p.Instance = "/requests/" + id
		}
	}
	if logger != nil {
		logger.ErrorContext(r.Context(), "request failed",
			slog.String("instance", p.Instance),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Any("error", err),
		)
	}
	WriteProblem(w, r, p)
}
