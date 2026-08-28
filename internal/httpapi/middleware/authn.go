package middleware

import "net/http"

// Authn rejects a request to a session-protected route when no live session is
// attached. Whether a route needs a session is declared on the route, not
// guessed from its path.
func Authn(required bool, authenticated func(*http.Request) bool, deny func(http.ResponseWriter, *http.Request)) Middleware {
	return func(next http.Handler) http.Handler {
		if !required {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authenticated == nil || !authenticated(r) {
				if deny != nil {
					deny(w, r)
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
