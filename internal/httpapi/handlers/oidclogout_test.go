package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/auth/oidc"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// TestTheProviderSignOutCannotBeTriggeredCrossSite: as a GET, the provider
// sign-out ended the session on any top-level navigation to it, and SameSite=Lax
// sends the cookie on exactly that -- a link or a redirect on any page signed
// the operator out. It is a POST under the CSRF preconditions now, and answers
// where to go instead of redirecting there.
func TestTheProviderSignOutCannotBeTriggeredCrossSite(t *testing.T) {
	t.Parallel()

	// An issuer nothing listens on: discovery fails at once, and the route
	// falls back to the local landing page without the test touching a network.
	p, err := oidc.New("https://127.0.0.1:1/", "holzkube-manager", "s3cret")
	if err != nil {
		t.Fatalf("oidc.New: %v", err)
	}
	s := newServerWith(t, 5*time.Minute, nil, talos.Mode{}, func(d *httpapi.Deps) { d.OIDC = p })
	c := s.newClient(t)
	c.setup()

	const path = "/api/v1/auth/oidc/logout"
	signedIn := func() bool {
		resp, _ := c.do(http.MethodGet, "/api/v1/auth/me", nil)
		return resp.StatusCode == http.StatusOK
	}

	// A navigation: GET, no CSRF header, no JSON.
	resp, raw := c.do(http.MethodGet, path, nil, func(r *http.Request) {
		r.Header.Del("X-Holzkube-Manager-CSRF")
		r.Header.Del("Content-Type")
	})
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusFound {
		t.Fatalf("a GET still signs out: %d (%s)", resp.StatusCode, raw)
	}
	if !signedIn() {
		t.Fatal("a GET to the provider sign-out ended the session")
	}

	// A cross-site form post: POST without the header.
	resp, raw = c.do(http.MethodPost, path, map[string]string{},
		func(r *http.Request) { r.Header.Del("X-Holzkube-Manager-CSRF") })
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a POST without the CSRF header: %d (%s), want 403", resp.StatusCode, raw)
	}
	if !signedIn() {
		t.Fatal("a POST without the CSRF header ended the session")
	}

	// The client's own call.
	resp, raw = c.do(http.MethodPost, path, map[string]string{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the sign-out: %d (%s), want 200", resp.StatusCode, raw)
	}
	var body struct {
		Redirect string `json:"redirect"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || body.Redirect != "/" {
		t.Fatalf("the sign-out answered %s, want {\"redirect\": \"/\"} without a provider endpoint", raw)
	}
	if signedIn() {
		t.Fatal("the sign-out left the session alive")
	}
}
