package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/holzcloud/holzkube/internal/httpapi/middleware"
)

// ProblemContentType is the media type every error response carries.
const ProblemContentType = "application/problem+json"

// ProblemBaseURI roots the taxonomy. Every type is an absolute URI under it;
// about:blank is never used, because a client that wants to branch on the kind
// of failure needs something stable to branch on.
const ProblemBaseURI = "https://holzkube.dev/problems/"

// The taxonomy is closed and stable. These URIs and the code tokens below are
// a public contract: clients may match on them, so they never change.
// Plan 01 task 4 completes the set and pins it in docs/api-contract.md.
const (
	TypeValidation      = "https://holzkube.dev/problems/validation"
	TypeUnauthenticated = "https://holzkube.dev/problems/unauthenticated"
	TypeCSRF            = "https://holzkube.dev/problems/csrf"
	TypeNotFound        = "https://holzkube.dev/problems/not-found"
	TypeConflict        = "https://holzkube.dev/problems/conflict"
	TypeSudoRequired    = "https://holzkube.dev/problems/sudo-required"
	TypeRateLimited     = "https://holzkube.dev/problems/rate-limited"
	TypeInternal        = "https://holzkube.dev/problems/internal"
	TypeSetupRequired   = "https://holzkube.dev/problems/setup-required"
)

// FieldError names one failed field inside a validation problem.
type FieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// Problem is an RFC 9457 problem detail.
//
// Code is holzkube's addition to the standard members: a stable machine token
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

// RateLimited reports a throttled caller. holzkube delays, it never locks out:
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
