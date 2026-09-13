package middleware

import "net/http"

// Authz refuses a request whose account does not carry the role a route asks
// for (V2-AUTH-02).
//
// It sits directly inside Authn and outside Audit, and both sides of that are
// deliberate.
//
// Inside Authn, because "who are you" has to be answered before "may you": a
// 403 to an anonymous caller tells them a route exists and that they would need
// a role for it, which is a fact about this installation that a 401 does not
// disclose.
//
// Outside Audit, because a refusal here is exactly the kind of event the
// archive exists for -- somebody with a valid session reached for something
// their account is not allowed to do -- and the argument is the same one that
// put Audit outside the sudo gate. The difference from that case is only which
// gate refused.
//
// allowed is a predicate over the request rather than a role value, because the
// role lives in the session and this package does not know what a session is.
func Authz(required bool, allowed func(*http.Request) bool, deny func(http.ResponseWriter, *http.Request)) Middleware {
	return func(next http.Handler) http.Handler {
		if !required {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// A nil predicate denies. A route that asked for a role against an
			// instance that cannot evaluate one is a route nobody should reach:
			// the safe reading of "I cannot tell" is "no".
			if allowed == nil || !allowed(r) {
				if deny != nil {
					deny(w, r)
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
