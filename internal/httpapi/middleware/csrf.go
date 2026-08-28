package middleware

// CSRF protection here is three preconditions checked together, not a token.
//
// For a pure JSON API that is the stronger of the two designs and the cheaper
// one. A token needs somewhere to live, a way to reach every form, a rotation
// story and a failure mode when the two copies disagree; the combination below
// needs none of that and cannot be forgotten at one call site out of thirty,
// because the browser enforces two thirds of it. `justinas/nosurf` is
// deliberately not a dependency of this project: it would be an unused module
// that `go mod tidy` removes on the next run, and it protects a shape --
// cross-origin form posts against a server that accepts form encodings -- that
// holzkube does not have. If a form-post path is ever added, this file is where
// a double-submit token gets layered on top, not instead.
//
// The three conditions are stated in docs/api-contract.md § CSRF Contract and
// clients are bound by them; web/src/api.ts is the single place the web UI
// speaks HTTP, which is what keeps the client half honest.

import (
	"errors"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

const (
	// CSRFHeader is the custom header every mutating request must carry.
	CSRFHeader = "X-Holzkube-CSRF"

	// CSRFHeaderValue is the only value that header may have. The contract
	// names it literally, and accepting anything non-empty would make the
	// contract a suggestion -- the next client to send "true" would work here
	// and break against a stricter implementation later.
	CSRFHeaderValue = "1"
)

// CSRF enforces three preconditions simultaneously on every mutating request:
//
//  1. Content-Type: application/json
//  2. X-Holzkube-CSRF: 1
//  3. an Origin / Sec-Fetch-Site consistent with our own origin
//
// A cross-origin HTML form can satisfy neither of the first two: a form can
// only send three encodings, none of them JSON, and it cannot set a header at
// all. A cross-origin fetch that sets them is preflighted, and the preflight
// has nowhere to succeed because holzkube sends no CORS headers. So the request
// never arrives rather than arriving and being rejected.
//
// SameSite=Lax on the cookie is necessary but not sufficient on its own: it is
// a browser default that browsers have historically relaxed, and it says
// nothing about a non-browser caller.
func CSRF(deny func(http.ResponseWriter, *http.Request, error)) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !IsMutating(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			if err := checkCSRF(r); err != nil {
				if deny != nil {
					deny(w, r, err)
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// IsMutating reports whether a method changes state.
func IsMutating(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

func checkCSRF(r *http.Request) error {
	if err := checkContentType(r); err != nil {
		return err
	}
	if got := r.Header.Get(CSRFHeader); got != CSRFHeaderValue {
		if got == "" {
			return errors.New("missing " + CSRFHeader + " header")
		}
		return errors.New(CSRFHeader + " must be " + CSRFHeaderValue)
	}
	return checkOrigin(r)
}

// checkContentType requires JSON, tolerating parameters such as a charset.
func checkContentType(r *http.Request) error {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return errors.New("missing Content-Type; mutating requests must send application/json")
	}
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil || mediaType != "application/json" {
		return errors.New("Content-Type must be application/json")
	}
	return nil
}

// checkOrigin compares whatever the caller told us about where it came from
// against where it actually arrived.
//
// Absence of both signals is permitted: a curl or a holzkubectl sends neither,
// and a browser that omits them cannot have satisfied the two conditions above
// from another origin anyway.
func checkOrigin(r *http.Request) error {
	// Sec-Fetch-Site is sent by every current browser and is the strongest of
	// the three signals when present. "same-site" is refused along with
	// "cross-site": a sibling subdomain is not us.
	switch site := r.Header.Get("Sec-Fetch-Site"); site {
	case "", "same-origin", "none":
	default:
		return errors.New("Sec-Fetch-Site is " + site + "; expected same-origin")
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		return nil
	}

	u, err := url.Parse(origin)
	if err != nil {
		return errors.New("unparsable Origin header")
	}
	if !strings.EqualFold(u.Host, r.Host) {
		// An opaque origin ("null" -- a sandboxed frame, a data: document, a
		// redirect chain) parses with an empty host and lands here, which is
		// the right answer for it.
		return errors.New("Origin " + origin + " does not match this host")
	}
	if scheme := requestScheme(r); !strings.EqualFold(u.Scheme, scheme) {
		// holzkube is HTTPS by default (D-04). Accepting an http:// origin on
		// an https:// request would accept a page a network attacker can write,
		// which is most of the reason the transport is encrypted at all.
		return errors.New("Origin " + origin + " does not match this scheme")
	}
	return nil
}

func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
