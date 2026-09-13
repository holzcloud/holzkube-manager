package middleware

import (
	"net/http"
	"strings"
)

// BearerToken resolves an Authorization header to an identity.
//
// It sits in the outer chain, beside the session loader and outside CSRF, and
// that placement is the whole design.
//
// **Outside CSRF**, because cross-site request forgery is an attack on
// *ambient* credentials: a cookie the browser attaches to a request the
// operator never made. A bearer token is not ambient -- it is put on the
// request by whatever made it, and a page on another origin cannot read it or
// cause it to be sent. Requiring the CSRF header from a machine would be a
// ritual that protects nothing and that every client has to be told about.
//
// **Beside the session loader**, because the two are alternatives and not
// layers. A request carrying a token gets no session, so nothing writes a
// Set-Cookie to a caller that has no cookie jar, and a stolen token cannot be
// escalated into a session.
//
// A token that does not authenticate is not an error here. It leaves the
// request unauthenticated and lets the route's own gate answer, which keeps one
// sentence for "you are not signed in" rather than two that differ by which
// credential was tried.
func BearerToken(resolve func(*http.Request) (*http.Request, bool)) Middleware {
	return func(next http.Handler) http.Handler {
		if resolve == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if BearerOf(r) == "" {
				next.ServeHTTP(w, r)
				return
			}
			if authenticated, ok := resolve(r); ok {
				r = authenticated
			}
			next.ServeHTTP(w, r)
		})
	}
}

// BearerOf returns the token an Authorization header carries, or "".
//
// The scheme comparison is case-insensitive because RFC 9110 says it is, and a
// client that sends "bearer" is a client that would otherwise be told its token
// is invalid -- which is the least debuggable thing this could say.
func BearerOf(r *http.Request) string {
	header := r.Header.Get("Authorization")
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// IsTokenRequest reports whether a request presented a bearer token at all,
// whether or not it authenticated.
//
// The sudo gate asks this. See the comment there for why a token satisfies it.
func IsTokenRequest(r *http.Request) bool { return BearerOf(r) != "" }
