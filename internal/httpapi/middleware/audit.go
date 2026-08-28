package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/holzcloud/holzkube/internal/audit"
)

const (
	// RequestIDParam carries the request id inside params. The record's field
	// set is closed (D-14) and part of the hash chain, so a new top-level field
	// is not an available move; params is where server-side context belongs.
	RequestIDParam = "request_id"

	// BodyParam stands in for a body that could not be read as a JSON object.
	// Its value goes through the redactor like everything else, so the record
	// says "something was sent" without saying what.
	BodyParam = "_body"

	// maxAuditBody caps how much of a request body is inspected. It matches the
	// handlers' own cap; a larger body is refused downstream anyway.
	maxAuditBody = 64 << 10

	// maxProblemCapture caps the response bytes kept in order to read the
	// taxonomy code back out. A problem response is a few hundred bytes.
	maxProblemCapture = 4 << 10

	maxCodeLen = 64
)

// AuditRecorder is the narrow view of the audit log this link needs. Declaring
// it here rather than depending on the concrete logger keeps the middleware
// testable and the dependency graph flat.
type AuditRecorder interface {
	Attempt(ctx context.Context, action, srcIP string, params map[string]any) (uint64, error)
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
//
// Input parameters are captured, and they pass through audit.Params on the way
// in — always, with no branch around it. The redactor is called here rather
// than being handed in as a dependency precisely so that no wiring mistake
// elsewhere can turn redaction off: a nil callback would fail open, into an
// archive that is never deleted.
func Audit(
	rec AuditRecorder,
	action string,
	mutating bool,
	deny func(http.ResponseWriter, *http.Request, error),
) Middleware {
	return func(next http.Handler) http.Handler {
		// A mutating route with no action token is skipped entirely and
		// therefore executes with no record at all. That is a defect of the
		// route, documented as such in docs/api-contract.md; it is visible here
		// rather than papered over with a guessed token.
		if rec == nil || action == "" || !mutating {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			params := captureParams(r, action)

			seq, err := rec.Attempt(r.Context(), action, ClientIP(r), params)
			if err != nil {
				if deny != nil {
					deny(w, r, fmt.Errorf("record audit intent: %w", err))
				}
				return
			}

			sniffer := &problemSniffer{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sniffer, r)

			// A panic never reaches this line. That is deliberate: the intent
			// stays in the log without an outcome, which is exactly the shape a
			// post-mortem is looking for. Nothing completes the pair after the
			// fact, and the panic travels on to the recover link untouched.
			outcome, cause := audit.OutcomeSuccess, error(nil)
			if sniffer.status >= http.StatusBadRequest {
				outcome = audit.OutcomeError
				// The stable taxonomy code, never the handler's own text: an
				// internal error string can name a path or a store message, and
				// this record is kept forever.
				cause = errors.New(sniffer.code())
			}
			if err := rec.Outcome(r.Context(), seq, outcome, cause); err != nil {
				// The response is already on the wire; the missing outcome is
				// itself detectable in the log, which is the designed signal.
				_ = err
			}
		})
	}
}

// captureParams reads the body, hands it back to the handler untouched, and
// returns what may be written about it.
//
// There is exactly one return path and it goes through the redactor. Anything
// that cannot be parsed becomes a marker rather than raw bytes, because an
// unparsable body is not a safe body: it is a body whose shape is unknown.
func captureParams(r *http.Request, action string) map[string]any {
	params := audit.Params(action, readBody(r))
	params[RequestIDParam] = RequestIDFromContext(r.Context())
	return params
}

func readBody(r *http.Request) map[string]any {
	if r.Body == nil {
		return nil
	}

	head, err := io.ReadAll(io.LimitReader(r.Body, maxAuditBody+1))
	// Whatever happened, the handler must see the stream it would have seen:
	// what was consumed here, followed by whatever is left. Truncating the body
	// would silently accept the prefix of an over-long request instead of
	// letting the handler refuse it.
	r.Body = replayBody{Reader: io.MultiReader(bytes.NewReader(head), r.Body), Closer: r.Body}
	if err != nil {
		return map[string]any{BodyParam: "unreadable"}
	}
	if len(head) > maxAuditBody {
		return map[string]any{BodyParam: "oversized"}
	}

	trimmed := bytes.TrimSpace(head)
	if len(trimmed) == 0 {
		return nil
	}

	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return map[string]any{BodyParam: "unparsable"}
	}
	return raw
}

// replayBody lets the handler read a body the middleware has already consumed.
type replayBody struct {
	io.Reader
	io.Closer
}

// problemSniffer remembers the status and, on a failure, enough of the response
// to recover the stable code the taxonomy put there.
type problemSniffer struct {
	http.ResponseWriter
	status    int
	written   bool
	capturing bool
	body      bytes.Buffer
}

func (s *problemSniffer) WriteHeader(code int) {
	if !s.written {
		s.status = code
		s.written = true
		s.capturing = code >= http.StatusBadRequest &&
			strings.HasPrefix(s.Header().Get("Content-Type"), "application/problem+json")
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *problemSniffer) Write(b []byte) (int, error) {
	if s.capturing {
		if room := maxProblemCapture - s.body.Len(); room > 0 {
			s.body.Write(b[:min(len(b), room)])
		}
	}
	s.written = true
	return s.ResponseWriter.Write(b)
}

// Status reports the status code the inner handler produced.
func (s *problemSniffer) Status() int { return s.status }

// code returns the stable token to record for a failed request.
//
// It reads the code out of the problem response rather than mapping the status,
// because one status covers several codes: 403 is both csrf.precondition-unmet
// and forbidden.*. When there is no usable code the status stands in — never
// free text.
func (s *problemSniffer) code() string {
	if s.body.Len() > 0 {
		var p struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(s.body.Bytes(), &p); err == nil && isCodeToken(p.Code) {
			return p.Code
		}
	}
	return fmt.Sprintf("http.%d", s.status)
}

// isCodeToken checks the shape of a taxonomy code before it is copied into a
// permanent record. Reading a field out of a response body and writing it to
// disk unexamined is how a log ends up carrying somebody else's text.
func isCodeToken(s string) bool {
	if s == "" || len(s) > maxCodeLen {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
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
