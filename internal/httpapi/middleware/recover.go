package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recover turns a panic into a 500 response. The stack trace goes to the log
// and never to the client: a Go stack trace names internal paths and package
// layout, which is free reconnaissance for anyone probing a tool that holds
// cluster PKI.
func Recover(logger *slog.Logger, onPanic func(http.ResponseWriter, *http.Request, error)) Middleware {
	return func(next http.Handler) http.Handler {
		//nolint:contextcheck // the deferred closure logs with r.Context() via
		// ErrorContext; contextcheck cannot see through the recover closure.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				// rec is a recovered panic value, not a returned error: it is
				// compared by identity and can never arrive wrapped.
				if rec == http.ErrAbortHandler { //nolint:errorlint
					panic(rec)
				}
				err := fmt.Errorf("panic: %v", rec)
				if logger != nil {
					logger.ErrorContext(r.Context(), "recovered from panic",
						slog.String("request_id", RequestIDFromContext(r.Context())),
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
						slog.Any("panic", rec),
						slog.String("stack", string(debug.Stack())),
					)
				}
				if onPanic != nil {
					onPanic(w, r, err)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
