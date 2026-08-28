package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// csrfCase is one request shape and what the three preconditions should make of
// it.
type csrfCase struct {
	name    string
	method  string
	tls     bool
	headers map[string]string
	pass    bool
}

func runCSRF(t *testing.T, tc csrfCase) (passed bool, denial error) {
	t.Helper()

	var reached bool
	mw := CSRF(func(w http.ResponseWriter, _ *http.Request, err error) {
		denial = err
		w.WriteHeader(http.StatusForbidden)
	})

	req := httptest.NewRequest(tc.method, "https://holzkube.test/api/v1/account/password", nil)
	req.Host = "holzkube.test"
	if tc.tls {
		req.TLS = &tls.ConnectionState{}
	}
	for k, v := range tc.headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })).ServeHTTP(rec, req)
	return reached, denial
}

// TestCSRFPreconditions walks the contract in docs/api-contract.md § CSRF
// Contract one condition at a time. All three must hold simultaneously; failing
// any one of them is a refusal, which is what makes a cross-origin form
// structurally unable to reach a destructive route (T-01-26).
func TestCSRFPreconditions(t *testing.T) {
	full := func(over ...string) map[string]string {
		h := map[string]string{
			"Content-Type":    "application/json",
			"X-Holzkube-CSRF": "1",
			"Sec-Fetch-Site":  "same-origin",
			"Origin":          "https://holzkube.test",
		}
		for i := 0; i+1 < len(over); i += 2 {
			if over[i+1] == "" {
				delete(h, over[i])
				continue
			}
			h[over[i]] = over[i+1]
		}
		return h
	}

	cases := []csrfCase{
		{name: "all three preconditions", method: http.MethodPost, tls: true, headers: full(), pass: true},
		{name: "json with a charset parameter", method: http.MethodPost, tls: true,
			headers: full("Content-Type", "application/json; charset=utf-8"), pass: true},
		{name: "no origin and no fetch metadata (a CLI client)", method: http.MethodPost, tls: true,
			headers: full("Origin", "", "Sec-Fetch-Site", ""), pass: true},

		{name: "missing the custom header", method: http.MethodPost, tls: true,
			headers: full("X-Holzkube-CSRF", ""), pass: false},
		{name: "custom header with the wrong value", method: http.MethodPost, tls: true,
			headers: full("X-Holzkube-CSRF", "0"), pass: false},
		{name: "form content type", method: http.MethodPost, tls: true,
			headers: full("Content-Type", "application/x-www-form-urlencoded"), pass: false},
		{name: "text content type", method: http.MethodPost, tls: true,
			headers: full("Content-Type", "text/plain"), pass: false},
		{name: "no content type at all", method: http.MethodPost, tls: true,
			headers: full("Content-Type", ""), pass: false},
		{name: "foreign origin", method: http.MethodPost, tls: true,
			headers: full("Origin", "https://evil.example"), pass: false},
		{name: "origin null", method: http.MethodPost, tls: true,
			headers: full("Origin", "null"), pass: false},
		{name: "origin on the wrong scheme", method: http.MethodPost, tls: true,
			headers: full("Origin", "http://holzkube.test"), pass: false},
		{name: "cross-site fetch metadata", method: http.MethodPost, tls: true,
			headers: full("Sec-Fetch-Site", "cross-site"), pass: false},
		{name: "same-site fetch metadata", method: http.MethodPost, tls: true,
			headers: full("Sec-Fetch-Site", "same-site"), pass: false},

		{name: "delete is mutating too", method: http.MethodDelete, tls: true,
			headers: full("X-Holzkube-CSRF", ""), pass: false},
		{name: "put is mutating too", method: http.MethodPut, tls: true,
			headers: full("Content-Type", "text/plain"), pass: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			passed, denial := runCSRF(t, tc)
			if passed != tc.pass {
				t.Fatalf("request passed = %v, want %v (denial: %v)", passed, tc.pass, denial)
			}
			if !tc.pass && denial == nil {
				t.Error("the request was refused without a reason")
			}
		})
	}
}

// TestCSRFLetsReadsThrough keeps the preconditions off the paths that cannot
// change anything: a GET carrying none of them is a normal page load.
func TestCSRFLetsReadsThrough(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			passed, denial := runCSRF(t, csrfCase{method: method, tls: true, headers: map[string]string{}})
			if !passed {
				t.Errorf("%s was refused: %v", method, denial)
			}
		})
	}
}
