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
