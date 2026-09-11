// Package middleware holds the HTTP middleware chain, one link per file.
//
// The split is not cosmetic. Each of the five wave-2 plans grows exactly one
// link; keeping them in separate files means none of them has to touch this
// composition file, and none of them collides with another.
//
// No link in this package imports the httpapi package. They take narrow
// callbacks and locally declared interfaces instead, which is what keeps the
// dependency between httpapi and its middleware pointing in one direction.
package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// Middleware wraps a handler.
type Middleware func(http.Handler) http.Handler

// Chain composes middleware so that the first argument is the outermost link.
func Chain(mw ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(mw) - 1; i >= 0; i-- {
			if mw[i] == nil {
				continue
			}
			next = mw[i](next)
		}
		return next
	}
}

// Log emits one structured line per request, carrying the request id so a
// problem response handed to a user can be found in the log by its instance.
func Log(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if logger == nil {
				next.ServeHTTP(w, r)
				return
			}
			start := time.Now()
			rec := WrapResponseWriter(w)
			next.ServeHTTP(rec, r)
			logger.InfoContext(r.Context(), "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.Status()),
				slog.String("request_id", RequestIDFromContext(r.Context())),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}

// responseRecorder remembers the status code so links further out can react to
// what an inner handler decided.
type responseRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

// WrapResponseWriter returns a ResponseWriter that records the status code.
//
// it as an http.ResponseWriter, and exporting it would invite code that depends
// on the recorder's internals.
//
//nolint:revive // the concrete type stays unexported on purpose: links consume
func WrapResponseWriter(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{ResponseWriter: w, status: http.StatusOK}
}

func (r *responseRecorder) WriteHeader(code int) {
	if !r.written {
		r.status = code
		r.written = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.written = true
	return r.ResponseWriter.Write(b)
}

// Status reports the status code the inner handler produced.
func (r *responseRecorder) Status() int { return r.status }

// Unwrap exposes the writer underneath, which is what makes streaming possible
// through this chain at all.
//
// http.ResponseController walks Unwrap() to find a Flusher, a Hijacker or a
// deadline setter. Without it, a wrapper that implements neither silently
// swallows the capability: a streaming handler calling Flush gets
// ErrNotSupported, or -- worse, before ResponseController existed -- a type
// assertion that fails and a handler that buffers the whole stream and looks
// like a hang.
//
// That was the entry blocker phase 2 recorded rather than worked around: none
// of the three wrappers in this chain implemented it, so no SSE route could be
// built on the chain and phase 2 deliberately built none. This is the line
// that lifts it.
//
// Only Unwrap is implemented, not Flush and Hijack directly, and that is the
// smaller and stricter choice. ResponseController reaches through Unwrap to
// the real writer, so the capability is exactly the real writer's -- there is
// no reimplementation here to get subtly wrong, and nothing that claims a
// capability the connection underneath does not have.
func (r *responseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
