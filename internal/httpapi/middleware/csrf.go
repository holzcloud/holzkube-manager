package middleware

import (
	"errors"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// CSRFHeader is the custom header every mutating request must carry.
const CSRFHeader = "X-Holzkube-CSRF"

// CSRF enforces three preconditions simultaneously on every mutating request:
//
//  1. Content-Type: application/json
//  2. the X-Holzkube-CSRF header
//  3. an Origin / Sec-Fetch-Site consistent with our own origin
//
// A cross-origin HTML form can satisfy none of the first two: both sit outside
// the CORS "simple request" envelope, so the browser preflights and the request
// never arrives. This costs nothing and needs no token plumbing. SameSite=Lax
// on the cookie is necessary but not sufficient on its own.
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
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return errors.New("missing Content-Type; mutating requests must send application/json")
	}
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil || mediaType != "application/json" {
		return errors.New("Content-Type must be application/json")
	}

	if r.Header.Get(CSRFHeader) == "" {
		return errors.New("missing " + CSRFHeader + " header")
	}

	// Sec-Fetch-Site is sent by every current browser and is the strongest of
	// the three signals when present.
	switch site := r.Header.Get("Sec-Fetch-Site"); site {
	case "", "same-origin", "none":
	default:
		return errors.New("Sec-Fetch-Site is " + site + "; expected same-origin")
	}

	// Origin is absent on same-origin GETs and on non-browser clients, but when
	// a browser sends it on a mutating request it must be ours.
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil {
			return errors.New("unparsable Origin header")
		}
		if !strings.EqualFold(u.Host, r.Host) {
			return errors.New("Origin " + origin + " does not match this host")
		}
	}
	return nil
}
