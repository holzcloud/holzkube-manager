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
