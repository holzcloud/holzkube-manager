package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type requestIDKey struct{}

// RequestIDHeader carries the id back to the client so an operator can quote it
// from a problem response.
const RequestIDHeader = "X-Request-Id"

// RequestID assigns every request a fresh identifier and puts it in the context
// and the response headers.
//
// The id is generated per request and never reused, which is what keeps two
// concurrent failures of the same kind distinguishable: each problem response
// carries its own instance.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := newRequestID()
			w.Header().Set(RequestIDHeader, id)
			ctx := context.WithValue(r.Context(), requestIDKey{}, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequestIDFromContext returns the request id, or the empty string.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

func newRequestID() string {
	var b [16]byte
	// rand.Read from crypto/rand cannot fail in Go 1.24+; it panics internally
	// on a broken entropy source rather than returning an error.
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
