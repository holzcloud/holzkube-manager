package httpapi_test

// The role gate at the HTTP boundary (V2-AUTH-02).
//
// The unit tests in internal/auth cover which refusals the domain makes. What
// these cover is the thing a unit test cannot: that the gate is actually wired
// into the chain, on every route, and that a route which forgot to name a role
// cannot be registered at all.

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// TestARouteWithNoRoleCannotBeRegistered is the composition-time half.
//
// It is the property that makes every other route safe without a test of its
// own: a session route with no MinRole does not serve a 403, it stops the
// process before the listener opens.
func TestARouteWithNoRoleCannotBeRegistered(t *testing.T) {
	t.Parallel()

	for name, rt := range map[string]httpapi.Route{
		"a session route with no role": {
			Method:          http.MethodGet,
			Pattern:         "/api/v1/forgot-the-role",
			RequiresSession: true,
			Handler:         http.NotFoundHandler(),
		},
		"a public route that names one": {
			Method:          http.MethodGet,
			Pattern:         "/api/v1/needs-no-session",
			RequiresSession: false,
			MinRole:         model.RoleAdmin,
			Handler:         http.NotFoundHandler(),
		},
		"a role that is not a role": {
			Method:          http.MethodGet,
			Pattern:         "/api/v1/invented-role",
			RequiresSession: true,
			MinRole:         "superuser",
			Handler:         http.NotFoundHandler(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if recover() == nil {
					t.Error("the route was registered. A permission nobody chose is not a " +
						"permission anybody reviewed, and this is the moment it is cheap to say so")
				}
			}()

			_ = httpapi.New(httpapi.Deps{Routes: []httpapi.Route{rt}})
		})
	}
}

// TestARoleIsEnforcedOnTheWayIn is the runtime half.
//
// The gate is one middleware link and it is easy to wire in the wrong place --
// or to wire and then have something short-circuit before it. These drive a
// real session at each of the three roles against three real routes.
func TestARoleIsEnforcedOnTheWayIn(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)

	// Creating an account is Destructive -- it changes who can reach cluster
	// PKI -- so the window has to be open first. That is the gate doing its
	// job and not scaffolding: an admin adding an account is asked for their
	// password, which is the whole of D-06.
	if resp, raw := h.do(t, http.MethodPost, "/api/v1/auth/sudo",
		map[string]string{"password": testPass}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
	}

	// The account setup created is an admin, so it can make the other two.
	for _, role := range []model.UserRole{model.RoleOperator, model.RoleReader} {
		resp, raw := h.do(t, http.MethodPost, "/api/v1/users", map[string]string{
			"username": string(role) + "-account",
			"password": newAccountPass,
			"role":     string(role),
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("creating the %s account: %d (%s)", role, resp.StatusCode, raw)
		}
	}

	// The gate's verdict is 403 or not-403, and that is deliberately all these
	// assert. This harness has no inventory service, so an allowed request to
	// /machines answers 502 -- which is the route saying "I got past the gate
	// and had nothing to ask". Asserting 200 would be asserting that the
	// harness is fully wired, and a change there would fail this test for a
	// reason that has nothing to do with roles.
	const forbidden = true

	for _, tc := range []struct {
		role     model.UserRole
		username string
		password string

		// A read every role has, a fleet write only operators and admins have,
		// and an account route only admins have.
		machines, lock, users bool
	}{
		{model.RoleAdmin, testUser, testPass, !forbidden, !forbidden, !forbidden},
		{model.RoleOperator, "operator-account", newAccountPass, !forbidden, !forbidden, forbidden},
		{model.RoleReader, "reader-account", newAccountPass, !forbidden, forbidden, forbidden},
	} {
		t.Run(string(tc.role), func(t *testing.T) {
			c := h.asUser(t, tc.username, tc.password)

			for _, call := range []struct {
				what   string
				method string
				path   string
				body   any
				refuse bool
			}{
				{"read the machines", http.MethodGet, "/api/v1/machines", nil, tc.machines},
				{"lock a node", http.MethodPost, "/api/v1/machines/x/lock",
					map[string]any{"locked": true, "reason": "because"}, tc.lock},
				{"read the accounts", http.MethodGet, "/api/v1/users", nil, tc.users},
			} {
				got, raw := c.status(t, call.method, call.path, call.body)
				is403 := got == http.StatusForbidden

				switch {
				case call.refuse && !is403:
					t.Errorf("a %s could %s: %d (%s)", tc.role, call.what, got, raw)
				case !call.refuse && is403:
					t.Errorf("a %s could not %s: %s", tc.role, call.what, raw)
				}
			}
		})
	}
}

// newAccountPass is what the two accounts this test creates sign in with.
const newAccountPass = "a-long-enough-passphrase"

// setupAndLogin runs the wizard and signs in as the account it creates, which
// is an admin.
func (h *harness) setupAndLogin(t *testing.T) {
	t.Helper()

	resp, raw := h.do(t, http.MethodPost, "/api/v1/setup", map[string]string{
		"username": testUser, "password": testPass,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: %d (%s)", resp.StatusCode, raw)
	}
	resp, raw = h.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": testUser, "password": testPass,
	})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login: %d (%s)", resp.StatusCode, raw)
	}
}

// asClient is a second signed-in browser against the same server.
//
// It has its own cookie jar, because the harness's client is already signed in
// as the admin and a shared jar would make every request below that admin's --
// which is a test that passes for the wrong reason.
type asClient struct {
	srv    string
	client *http.Client

	// bearer, when set, is sent instead of relying on a cookie jar. A client
	// with both would be testing neither.
	bearer string
}

func (h *harness) asUser(t *testing.T, username, password string) *asClient {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	c := &asClient{
		srv: h.srv.URL,
		client: &http.Client{
			Jar: jar,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test server uses an ephemeral cert
			},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}

	if got, raw := c.status(t, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"username": username, "password": password}); got != http.StatusNoContent {
		t.Fatalf("login as %s: %d (%s)", username, got, raw)
	}
	return c
}

// status makes a request as this signed-in account.
//
// The options exist for one test: TestASessionCannotBorrowAWallLinkToSkipItsRole
// has to send a session AND an Authorization header together, which is the one
// combination that can reach the role gate with a bearer present.
func (c *asClient) status(t *testing.T, method, path string, body any, opts ...reqOpt) (int, []byte) {
	t.Helper()

	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, c.srv+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.bearer == "" {
		req.Header.Set("X-Holzkube-Manager-CSRF", "1")
	} else {
		// Deliberately no CSRF header. A bearer token is not ambient -- a page
		// on another origin cannot read it or cause it to be sent -- so
		// demanding the header from a machine would be a ritual that protects
		// nothing. If that stops being true, this call starts failing.
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	for _, o := range opts {
		o(req)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, raw
}

// TestAServiceAccountTokenWorksOverHTTP is the wiring the unit tests cannot
// reach: that a bearer token establishes an identity at all, that the role gate
// reads it, and that it satisfies the sudo gate without a password.
func TestAServiceAccountTokenWorksOverHTTP(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)

	if resp, raw := h.do(t, http.MethodPost, "/api/v1/auth/sudo",
		map[string]string{"password": testPass}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
	}

	resp, raw := h.do(t, http.MethodPost, "/api/v1/service-accounts", map[string]string{
		"username": "ci-bot", "role": "operator",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("creating the service account: %d (%s)", resp.StatusCode, raw)
	}

	var created struct {
		Token   string `json:"token"`
		Notice  string `json:"notice"`
		Account struct {
			Kind string `json:"kind"`
			Role string `json:"role"`
		} `json:"account"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatalf("the response is not the shape the contract describes: %v (%s)", err, raw)
	}
	if created.Token == "" {
		t.Fatal("no token was returned, and there is no second chance to ask for one")
	}
	if created.Account.Kind != "service" || created.Account.Role != "operator" {
		t.Errorf("the account came back as %+v, want a service account at operator", created.Account)
	}
	if created.Notice == "" {
		t.Error("nothing told the operator this is the only time the token is shown")
	}

	c := &asClient{srv: h.srv.URL, client: h.srv.Client(), bearer: created.Token}

	// A read the role allows. 502 is the harness having no inventory service,
	// which is this route saying it got past both gates.
	if got, body := c.status(t, http.MethodGet, "/api/v1/machines", nil); got == http.StatusUnauthorized ||
		got == http.StatusForbidden {
		t.Errorf("the token did not authenticate a read its role allows: %d (%s)", got, body)
	}

	// A route the role does not allow. The gate reads the token's identity.
	if got, body := c.status(t, http.MethodGet, "/api/v1/users", nil); got != http.StatusForbidden {
		t.Errorf("an operator token could read the accounts: %d (%s)", got, body)
	}

	// And a genuinely Destructive route, with no sudo window anywhere. This is
	// the decision the phase makes: a bearer token is not ambient, so there is
	// no second secret to ask for.
	//
	// The reboot and not the lock. The lock is deliberately NOT Destructive --
	// it is this installation's own note about what it should not do -- so a
	// test that used it would pass whether or not the sudo gate had been
	// taught about tokens, which is how the first version of this assertion
	// checked nothing.
	got, body := c.status(t, http.MethodPost, "/api/v1/machines/x/reboot", nil)
	if got == http.StatusPreconditionRequired {
		t.Errorf("a token was asked to re-authenticate. There is no password to ask for, so this "+
			"makes every destructive route unreachable by automation: %s", body)
	}
	if got == http.StatusUnauthorized || got == http.StatusForbidden {
		t.Errorf("the token was refused a destructive route its role allows: %d (%s)", got, body)
	}

	// A token that authenticates nothing leaves the request anonymous rather
	// than producing a second vocabulary for "not signed in".
	bad := &asClient{srv: h.srv.URL, client: h.srv.Client(), bearer: "hkm_not-a-real-token"}
	if got, body := bad.status(t, http.MethodGet, "/api/v1/machines", nil); got != http.StatusUnauthorized {
		t.Errorf("an invalid token answered %d, want 401 (%s)", got, body)
	}

	// A service account changing "its" password. It has none -- that is the
	// definition -- and the route is open to it: the password change requires
	// only RoleReader, and a bearer token satisfies both the CSRF check and
	// the sudo window, so every gate in front of this hands the request
	// through.
	//
	// What arrives at the handler is Verify(current, "") against an empty
	// hash, which is not a wrong password but an undecodable one, so it
	// returns an error rather than false -- and an error there is a 500 with a
	// line in the log about an unexpected internal condition. There is nothing
	// unexpected about it. auth.ErrNotAPerson was written for exactly this and
	// was never returned by anything.
	got, body = c.status(t, http.MethodPost, "/api/v1/account/password",
		map[string]string{"current_password": "anything", "new_password": "long-enough-to-pass"})
	if got >= 500 {
		t.Errorf("a service account asking to change a password it does not have got %d, "+
			"which is this product calling a state it can name an internal error: %s", got, body)
	}
	if got != http.StatusConflict {
		t.Errorf("a service account changing a password answered %d, want 409 (%s)", got, body)
	}
	if !strings.Contains(string(body), "service account") {
		t.Errorf("the refusal does not say what is actually wrong: %s", body)
	}

	// And the second route that takes a password, for the same reason. A
	// service account re-authenticating is asking for a window its token
	// already satisfies, with a credential it does not have -- and the same
	// Verify against the same empty hash produced the same 500.
	got, body = c.status(t, http.MethodPost, "/api/v1/auth/sudo",
		map[string]string{"password": "anything"})
	if got >= 500 {
		t.Errorf("a service account re-authenticating got %d: %s", got, body)
	}
	if got != http.StatusConflict {
		t.Errorf("a service account opening a sudo window answered %d, want 409 (%s)", got, body)
	}
}

// TestABogusBearerHeaderDoesNotWaiveSudoForASession pins the hole an
// Authorization header opened: the sudo gate and CSRF asked whether a bearer
// was *presented*, so "Bearer x" on a request that a stolen cookie
// authenticated skipped the password. They now ask whether a token was
// *resolved*.
func TestABogusBearerHeaderDoesNotWaiveSudoForASession(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)
	c := h.asUser(t, testUser, testPass)

	bogus := func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer not-a-token")
	}
	// A Destructive route, and no sudo window has been opened for this session.
	got, body := c.status(t, http.MethodPost, "/api/v1/users", map[string]string{
		"username": "backdoor", "password": newAccountPass, "role": "admin",
	}, bogus)
	if got != http.StatusPreconditionRequired {
		t.Errorf("a session plus a made-up bearer got %d (%s), want 428 sudo.required", got, body)
	}
}

func TestAnAuditedReadWithoutAnActionCannotBeRegistered(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Error("a read that asks to be audited but has no action name was registered")
		}
	}()
	_ = httpapi.New(httpapi.Deps{Routes: []httpapi.Route{{
		Method: http.MethodGet, Pattern: "/api/v1/x", RequiresSession: true,
		MinRole: model.RoleAdmin, AuditRead: true, Handler: http.NotFoundHandler(),
	}}})
}

// TestALoopbackHostNameFromARemotePeerIsSSOOnly: SSO-only was a property of
// the Host header, which is the one thing a remote caller picks.
func TestALoopbackHostNameFromARemotePeerIsSSOOnly(t *testing.T) {
	t.Parallel()

	d := httpapi.Deps{IsSSOOnly: func(string) bool { return false }}
	for _, tc := range []struct {
		host, remote string
		want         bool
	}{
		{"localhost:8443", "203.0.113.9:5555", true},
		{"127.0.0.1:8443", "203.0.113.9:5555", true},
		{"[::1]:8443", "203.0.113.9:5555", true},
		{"localhost:8443", "127.0.0.1:5555", false},
		{"[::1]:8443", "[::1]:5555", false},
		{"homeserver.example.com", "203.0.113.9:5555", false},
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		r.Host, r.RemoteAddr = tc.host, tc.remote
		if got := d.SSOOnly(r); got != tc.want {
			t.Errorf("Host %q from %q: SSOOnly = %v, want %v", tc.host, tc.remote, got, tc.want)
		}
	}
}

// TestAnonymousLoginsCannotFloodTheAuditArchive: every login attempt writes an
// fsync'd, hash-chained intent/outcome pair into an archive that is never
// pruned, and the caller needs no credential. One peer gets a burst, then 429.
func TestAnonymousLoginsCannotFloodTheAuditArchive(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)

	limited := 0
	for range 60 {
		// /setup after setup answers 409 with no throttle of its own, so any
		// 429 here is this limiter and not the login throttle.
		resp, _ := h.do(t, http.MethodPost, "/api/v1/setup",
			map[string]string{"username": "nobody", "password": "wrong-password-xx"})
		if resp.StatusCode == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 {
		t.Error("60 anonymous setup attempts in a row from one peer were all admitted")
	}
}
