package middleware

import (
	"net"
	"net/http"
	"strings"
)

// AllowHosts refuses a request whose Host header names an address this instance
// does not answer to.
//
// This closes DNS rebinding, the standard attack against a loopback-bound admin
// tool. The three CSRF preconditions are all self-referential under it: a
// victim's browser resolving evil.example to 127.0.0.1 sends
// Host: evil.example, Origin: https://evil.example and
// Sec-Fetch-Site: same-origin, because from the browser's point of view it *is*
// same-origin. checkOrigin compares the attacker-supplied Origin against the
// attacker-supplied Host, so all three pass and the request reaches the handler
// with the victim's cookie. Nothing else validates that Host is one of ours.
//
// TLS mitigates this by default -- the browser shows a certificate error for
// evil.example, since the generated SANs are localhost, the hostname and the
// loopback addresses -- but under --insecure-http there is no mitigation at
// all, and --insecure-http on loopback is exactly the configuration rebinding
// targets.
//
// It runs in the outer chain rather than inside the CSRF link, which only
// covers mutating requests. A rebound GET of the audit log is a leak of the
// archive, so reads need the same check.
//
// An empty allowed list disables the check. The composition root always
// populates it; the escape hatch exists so a test can construct a handler
// without knowing what port httptest will pick.
func AllowHosts(allowed []string, deny func(http.ResponseWriter, *http.Request, error)) Middleware {
	if len(allowed) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}

	set := make(map[string]struct{}, len(allowed))
	for _, h := range allowed {
		if h = normalizeHost(h); h != "" {
			set[h] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := set[normalizeHost(r.Host)]; !ok {
				if deny != nil {
					deny(w, r, errHost{r.Host})
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// normalizeHost reduces a Host header or a configured address to a comparable
// host: no port, no brackets, lowercase.
func normalizeHost(host string) string {
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(strings.Trim(host, "[]"))
}

type errHost struct{ host string }

func (e errHost) Error() string {
	return "Host " + e.host + " is not an address this instance serves"
}
