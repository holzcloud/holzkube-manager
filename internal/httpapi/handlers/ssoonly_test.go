package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// publicHost is the name the instance answers on from the internet in these
// tests; lanHost is the address it answers on locally.
const (
	publicHost = "manager.example.com"
	lanHost    = "192.168.1.10:8443"
)

// depOption mutates the dependency graph before the routes are built.
type depOption func(*httpapi.Deps)

// withSSOOnly marks hosts as accepting the identity provider only.
func withSSOOnly(hosts ...string) depOption {
	return func(d *httpapi.Deps) {
		d.IsSSOOnly = func(host string) bool {
			// The production wiring normalises through config.NormalizeHost.
			// Here the comparison is on the bare name, because these tests set
			// the Host header themselves and a normalisation bug in the harness
			// would be a test that passes for the wrong reason.
			for _, h := range hosts {
				if host == h {
					return true
				}
			}
			return false
		}
	}
}

// asHost sends a request as if it had arrived on the given name. The Host
// allowlist is disabled in the harness -- httptest picks the port -- so this is
// the only thing that decides which policy applies.
//
// The session cookie is attached by hand. Overriding Request.Host takes the
// request out of the cookie jar's addressing, so a jar-managed client sends it
// anonymously: without this, every assertion below would be measuring a missing
// session rather than the policy under test, and the SSO-only cases would pass
// for entirely the wrong reason.
func (c *client) asHost(host string) reqOpt {
	return func(r *http.Request) {
		r.Host = host
		for _, ck := range c.http.Jar.Cookies(r.URL) {
			r.AddCookie(ck)
		}
	}
}

func newSSOOnlyServer(t *testing.T) *server {
	t.Helper()
	return newServerWith(t, 5*time.Minute, nil, talos.Mode{}, withSSOOnly(publicHost))
}

// The local password is refused on the public name and accepted on the LAN
// address, in the same process and against the same account.
//
// This is the whole point of the split: the break-glass credential exists so
// that an identity provider outage does not lock the operator out of the tool
// that repairs the cluster the provider runs on -- and the same credential
// offered to the internet is a password on the public net guarding cluster PKI.
func TestPasswordLoginIsRefusedOnSSOOnlyHost(t *testing.T) {
	t.Parallel()

	s := newSSOOnlyServer(t)
	c := s.newClient(t)
	c.setup()

	// The account exists and the password is right; only the address differs.
	resp, body := c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": testUser,
		"password": testPass,
	}, c.asHost(publicHost))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("login on the public host = %d, want 403 (body %s)", resp.StatusCode, body)
	}

	var p struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("decode problem %s: %v", body, err)
	}
	if p.Code != "auth.sso-only" {
		// A generic 403 would leave the operator guessing whether the password
		// is wrong or the address is.
		t.Errorf("problem code = %q, want auth.sso-only (body %s)", p.Code, body)
	}

	// The same request on the LAN address succeeds.
	lan := s.newClient(t)
	if resp, body := lan.do(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": testUser,
		"password": testPass,
	}, lan.asHost(lanHost)); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login on the LAN host = %d, want 204 (body %s)", resp.StatusCode, body)
	}
}

// The sudo gate is what stands between a session cookie and a destructive
// action. A public address that refuses the password to sign in but accepts it
// to authorise wiping a node would have moved the hole rather than closed it.
func TestSudoByPasswordIsRefusedOnSSOOnlyHost(t *testing.T) {
	t.Parallel()

	s := newSSOOnlyServer(t)
	c := s.newClient(t)
	c.setup() // leaves this client signed in

	resp, body := c.do(http.MethodPost, "/api/v1/auth/sudo", map[string]string{
		"password": testPass,
	}, c.asHost(publicHost))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("sudo on the public host = %d, want 403 (body %s)", resp.StatusCode, body)
	}

	// And still works from the LAN, so the refusal is about the address.
	if resp, body := c.do(http.MethodPost, "/api/v1/auth/sudo", map[string]string{
		"password": testPass,
	}, c.asHost(lanHost)); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sudo on the LAN host = %d, want 204 (body %s)", resp.StatusCode, body)
	}
}

// Setup is reachable before any credential exists. On the public address that
// is a race anybody can enter, and the winner owns an instance the threat model
// calls equivalent to root on every managed node.
func TestSetupIsRefusedOnSSOOnlyHost(t *testing.T) {
	t.Parallel()

	s := newSSOOnlyServer(t)
	c := s.newClient(t)

	resp, body := c.do(http.MethodPost, "/api/v1/setup", map[string]string{
		"username": testUser,
		"password": testPass,
	}, c.asHost(publicHost))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("setup on the public host = %d, want 403 (body %s)", resp.StatusCode, body)
	}

	// No account was created by the refused request.
	var status struct {
		SetupRequired bool `json:"setup_required"`
	}
	_, raw := c.do(http.MethodGet, "/api/v1/system/status", nil, c.asHost(lanHost))
	if err := json.Unmarshal(raw, &status); err != nil {
		t.Fatalf("decode status %s: %v", raw, err)
	}
	if !status.SetupRequired {
		t.Error("the refused setup request created an account anyway")
	}
}

// The sign-in page renders from the status endpoint, so the status endpoint has
// to answer per address: the same instance offers both ways in on the LAN and
// only the provider on the public name.
func TestStatusReportsTheAuthOptionsPerHost(t *testing.T) {
	t.Parallel()

	s := newSSOOnlyServer(t)
	c := s.newClient(t)

	for _, tc := range []struct {
		host          string
		wantPassword  bool
		wantOIDCFalse bool
	}{
		{host: lanHost, wantPassword: true},
		{host: publicHost, wantPassword: false},
	} {
		_, raw := c.do(http.MethodGet, "/api/v1/system/status", nil, c.asHost(tc.host))

		var got struct {
			PasswordLogin bool `json:"password_login"`
			OIDCEnabled   bool `json:"oidc_enabled"`
		}
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("decode status %s: %v", raw, err)
		}
		if got.PasswordLogin != tc.wantPassword {
			t.Errorf("on %s: password_login = %v, want %v", tc.host, got.PasswordLogin, tc.wantPassword)
		}
		// This harness configures no provider, so the field is false on both
		// addresses. Asserting it keeps the pair honest: a page told that
		// neither way in is available is a bug worth failing on later.
		if got.OIDCEnabled {
			t.Errorf("on %s: oidc_enabled is true although no provider is configured", tc.host)
		}
	}
}

// With no SSO-only host configured -- the default -- nothing changes for an
// existing deployment.
func TestPasswordLoginIsUnaffectedWithoutSSOOnlyHosts(t *testing.T) {
	t.Parallel()

	s := newServer(t, 5*time.Minute)
	c := s.newClient(t)
	c.setup()

	if resp := c.login(testPass); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login = %d, want 204", resp.StatusCode)
	}
	if resp, body := c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": testUser,
		"password": testPass,
	}, c.asHost(publicHost)); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login on an arbitrary host = %d, want 204 (body %s)", resp.StatusCode, body)
	}
}

// The OIDC routes exist only when a provider is configured. A route table that
// mirrors the configuration is one an operator can read to find out what this
// instance accepts.
func TestOIDCRoutesAreAbsentWithoutAProvider(t *testing.T) {
	t.Parallel()

	s := newServer(t, 5*time.Minute)
	c := s.newClient(t)
	c.setup()

	for _, path := range []string{
		"/api/v1/auth/oidc/start",
		"/api/v1/auth/oidc/callback",
		"/api/v1/auth/oidc/sudo",
		"/api/v1/auth/oidc/logout",
	} {
		resp, _ := c.do(http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404 with no provider configured", path, resp.StatusCode)
		}
	}
}
