package middleware

import "net/http"

// SecurityHeaders sets the response headers that constrain what a browser will
// do with holzkube-manager's pages, on every response including static assets.
//
// nosniff was previously set on JSON and problem responses but on nothing the
// file server produced, and none of the others were set anywhere.
//
// The framing directives are the ones with teeth here. Without them the console
// is frameable by any page, and SameSite=Lax on the session cookie blunts that
// without eliminating it -- and does nothing at all for an XSS in a page that
// has no policy to contain it, in a binary that holds cluster PKI.
//
// HSTS is keyed off the actual connection rather than configuration: browsers
// ignore it over plain HTTP anyway, and sending it under --insecure-http would
// pin an operator's browser to HTTPS for a host they deliberately chose to
// serve without it.
func SecurityHeaders(csp string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			if csp != "" {
				h.Set("Content-Security-Policy", csp)
			}
			if r.TLS != nil {
				h.Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r)
		})
	}
}
