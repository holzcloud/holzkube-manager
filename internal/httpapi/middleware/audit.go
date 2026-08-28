package middleware

import (
	"context"
	"fmt"
	"net/http"
)

// AuditRecorder is the narrow view of the audit log this link needs. Declaring
// it here rather than importing the audit package keeps the middleware
// testable and the dependency graph flat.
type AuditRecorder interface {
	Attempt(ctx context.Context, action, srcIP string) (uint64, error)
	Outcome(ctx context.Context, seq uint64, outcome string, cause error) error
}

// Audit writes the intent record before the handler runs and the outcome record
// after it returns.
//
// Recording only successes throws away exactly the forensics that matter, so
// both halves are written, and an intent with no outcome is itself a finding.
// If the intent cannot be made durable the request is refused rather than
// performed unlogged: a mutation with no trace is the thing this exists to
// prevent.
func Audit(
	rec AuditRecorder,
	action string,
	mutating bool,
	deny func(http.ResponseWriter, *http.Request, error),
) Middleware {
	return func(next http.Handler) http.Handler {
		if rec == nil || action == "" || !mutating {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seq, err := rec.Attempt(r.Context(), action, ClientIP(r))
			if err != nil {
				if deny != nil {
					deny(w, r, fmt.Errorf("record audit intent: %w", err))
				}
				return
			}

			recorder := WrapResponseWriter(w)
			next.ServeHTTP(recorder, r)

			outcome, cause := "success", error(nil)
			if status := recorder.Status(); status >= http.StatusBadRequest {
				outcome = "error"
				cause = fmt.Errorf("request failed with status %d", status)
			}
			if err := rec.Outcome(r.Context(), seq, outcome, cause); err != nil {
				// The response is already on the wire; the missing outcome is
				// itself detectable in the log, which is the designed signal.
				_ = err
			}
		})
	}
}

// ClientIP reports the peer address. holzkube binds loopback by default and
// sits behind no proxy, so forwarded headers are deliberately not trusted:
// honouring them would let a caller write any address it likes into the log.
func ClientIP(r *http.Request) string {
	host := r.RemoteAddr
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i]
		}
	}
	return host
}
