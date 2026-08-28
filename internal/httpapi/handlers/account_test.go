package handlers_test

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/holzcloud/holzkube/internal/audit"
	"github.com/holzcloud/holzkube/internal/auth"
	"github.com/holzcloud/holzkube/internal/httpapi"
	"github.com/holzcloud/holzkube/internal/httpapi/handlers"
	"github.com/holzcloud/holzkube/internal/store/fsstore"
)

const (
	testUser = "operator"
	testPass = "correct-horse-battery-staple"
	newPass  = "a-completely-different-passphrase"
)

// server is the whole object graph cmd/holzkubed builds, over a throwaway data
// directory. Password change is only observable through the full chain -- the
// sudo gate lives in middleware, not in the handler -- so the test has to drive
// the real thing rather than a handler in isolation.
type server struct {
	srv        *httptest.Server
	deps       httpapi.Deps
	sudoWindow time.Duration
}

func newServer(t *testing.T, sudoWindow time.Duration) *server {
	t.Helper()

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod data dir: %v", err)
	}

	st, err := fsstore.Open(dir)
	if err != nil {
		t.Fatalf("fsstore.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	al, err := audit.Open(dir)
	if err != nil {
		t.Fatalf("audit.Open: %v", err)
	}
	t.Cleanup(func() { _ = al.Close() })

	au, err := auth.New(st, 24*time.Hour)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}

	deps := httpapi.Deps{
		Store:      st,
		Audit:      al,
		Auth:       au,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		SudoWindow: sudoWindow,
		AuditChain: httpapi.ChainStatus{OK: true},
	}
	deps.Routes = slices.Concat(
		handlers.SystemRoutes(deps),
		handlers.SetupRoutes(deps),
		handlers.AuthRoutes(deps),
		handlers.AccountRoutes(deps),
		handlers.AuditRoutes(deps),
	)

	ts := httptest.NewTLSServer(httpapi.New(deps))
	t.Cleanup(ts.Close)

	return &server{srv: ts, deps: deps, sudoWindow: sudoWindow}
}

// client is one browser: its own cookie jar, and therefore its own session.
type client struct {
	t    *testing.T
	s    *server
	http *http.Client
}

func (s *server) newClient(t *testing.T) *client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	return &client{
		t: t,
		s: s,
		http: &http.Client{
			Jar: jar,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // ephemeral test cert
			},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

type reqOpt func(*http.Request)

func (c *client) do(method, path string, body any, opts ...reqOpt) (*http.Response, []byte) {
	c.t.Helper()

	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.s.srv.URL+path, rdr)
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Holzkube-CSRF", "1")
	for _, o := range opts {
		o(req)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("read body: %v", err)
	}
	return resp, raw
}

func (c *client) sessionCookie() string {
	c.t.Helper()
	u, err := url.Parse(c.s.srv.URL)
	if err != nil {
		c.t.Fatalf("parse url: %v", err)
	}
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == auth.CookieName {
			return ck.Value
		}
	}
	return ""
}

// setup runs the first-run wizard, which leaves this client logged in.
func (c *client) setup() {
	c.t.Helper()
	resp, raw := c.do(http.MethodPost, "/api/v1/setup", map[string]string{
		"username": testUser,
		"password": testPass,
	})
	if resp.StatusCode != http.StatusCreated {
		c.t.Fatalf("setup: got %d, want 201 (body: %s)", resp.StatusCode, raw)
	}
}

func (c *client) login(password string) *http.Response {
	c.t.Helper()
	resp, _ := c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": testUser,
		"password": password,
	})
	return resp
}

func (c *client) mustLogin() {
	c.t.Helper()
	if resp := c.login(testPass); resp.StatusCode != http.StatusNoContent {
		c.t.Fatalf("login: got %d, want 204", resp.StatusCode)
	}
}

func (c *client) sudo(password string) (*http.Response, []byte) {
	c.t.Helper()
	return c.do(http.MethodPost, "/api/v1/auth/sudo", map[string]string{"password": password})
}

func (c *client) changePassword(current, next string) (*http.Response, []byte) {
	c.t.Helper()
	return c.do(http.MethodPost, "/api/v1/account/password", map[string]string{
		"current_password": current,
		"new_password":     next,
	})
}

type problemBody struct {
	Type   string `json:"type"`
	Status int    `json:"status"`
	Code   string `json:"code"`
}

func decodeProblem(t *testing.T, raw []byte) problemBody {
	t.Helper()
	var p problemBody
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("decode problem: %v (body: %s)", err, raw)
	}
	return p
}

// TestAccountPasswordRouteIsMarkedDestructive reads the route table itself.
// D-06 makes the marking the single readable statement of what is dangerous, so
// the marking is worth asserting directly and not only through its effect.
func TestAccountPasswordRouteIsMarkedDestructive(t *testing.T) {
	s := newServer(t, 5*time.Minute)

	routes := handlers.AccountRoutes(s.deps)
	idx := slices.IndexFunc(routes, func(r httpapi.Route) bool {
		return r.Method == http.MethodPost && r.Pattern == "/api/v1/account/password"
	})
	if idx < 0 {
		t.Fatalf("POST /api/v1/account/password is not registered (routes: %d)", len(routes))
	}
	rt := routes[idx]
	if !rt.Destructive {
		t.Error("the password change is not marked Destructive")
	}
	if !rt.RequiresSession {
		t.Error("the password change does not require a session")
	}
	if rt.Action == "" {
		t.Error("the password change has no audit action token; it would execute unlogged")
	}
}

// TestAccountPasswordWithoutSudoIs428 is the gate doing its job: a valid session
// is not enough for a destructive route.
func TestAccountPasswordWithoutSudoIs428(t *testing.T) {
	s := newServer(t, 5*time.Minute)
	c := s.newClient(t)
	c.setup()

	resp, raw := c.changePassword(testPass, newPass)
	if resp.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("got %d, want 428 (body: %s)", resp.StatusCode, raw)
	}
	p := decodeProblem(t, raw)
	if p.Code != "sudo.required" {
		t.Errorf("code = %q, want sudo.required", p.Code)
	}
	if p.Type != "https://holzkube.dev/problems/sudo-required" {
		t.Errorf("type = %q, want the sudo-required problem type", p.Type)
	}
}

// TestAccountPasswordWithoutSessionIs401 keeps the chain order honest: an
// anonymous caller is told to log in, not asked to re-enter a password they do
// not have a session for.
func TestAccountPasswordWithoutSessionIs401(t *testing.T) {
	s := newServer(t, 5*time.Minute)
	owner := s.newClient(t)
	owner.setup()

	anon := s.newClient(t)
	resp, raw := anon.changePassword(testPass, newPass)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401 (body: %s)", resp.StatusCode, raw)
	}
	if code := decodeProblem(t, raw).Code; code != "auth.unauthenticated" {
		t.Errorf("code = %q, want auth.unauthenticated", code)
	}
}

// TestSudoOpensTheWindowAndTheChangeGoesThrough is the loop the operator
// actually walks: refused, re-authenticate, done.
func TestSudoOpensTheWindowAndTheChangeGoesThrough(t *testing.T) {
	s := newServer(t, 5*time.Minute)
	owner := s.newClient(t)
	owner.setup()

	// A second browser, logged in and idle, which the change must throw out.
	other := s.newClient(t)
	other.mustLogin()

	if resp, raw := owner.changePassword(testPass, newPass); resp.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("before sudo: got %d, want 428 (body: %s)", resp.StatusCode, raw)
	}

	resp, raw := owner.sudo(testPass)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sudo: got %d, want 204 (body: %s)", resp.StatusCode, raw)
	}

	resp, raw = owner.changePassword(testPass, newPass)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("after sudo: got %d, want 204 (body: %s)", resp.StatusCode, raw)
	}

	// The caller keeps working; the parallel session does not.
	if resp, _ := owner.do(http.MethodGet, "/api/v1/auth/me", nil); resp.StatusCode != http.StatusOK {
		t.Errorf("the session that changed the password was logged out: %d", resp.StatusCode)
	}
	if resp, _ := other.do(http.MethodGet, "/api/v1/auth/me", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a parallel session survived the password change: %d", resp.StatusCode)
	}

	// The credential really moved.
	fresh := s.newClient(t)
	if resp := fresh.login(testPass); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the old password still logs in: %d", resp.StatusCode)
	}
	if resp := fresh.login(newPass); resp.StatusCode != http.StatusNoContent {
		t.Errorf("the new password does not log in: %d", resp.StatusCode)
	}
}

// TestSudoWithAWrongPasswordLeavesTheWindowClosed keeps the re-authentication
// from being a formality.
func TestSudoWithAWrongPasswordLeavesTheWindowClosed(t *testing.T) {
	s := newServer(t, 5*time.Minute)
	c := s.newClient(t)
	c.setup()

	resp, raw := c.sudo("not-the-password")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("sudo with a wrong password: got %d, want 401 (body: %s)", resp.StatusCode, raw)
	}
	if code := decodeProblem(t, raw).Code; code != "auth.unauthenticated" {
		t.Errorf("code = %q, want auth.unauthenticated", code)
	}

	if resp, raw := c.changePassword(testPass, newPass); resp.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("after a failed sudo: got %d, want 428 (body: %s)", resp.StatusCode, raw)
	}
}

// TestSudoWithoutASessionIs401 keeps the chain order right: someone who is not
// logged in is told to log in, not asked to re-enter a password.
func TestSudoWithoutASessionIs401(t *testing.T) {
	s := newServer(t, 5*time.Minute)
	owner := s.newClient(t)
	owner.setup()

	anon := s.newClient(t)
	resp, raw := anon.sudo(testPass)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401 (body: %s)", resp.StatusCode, raw)
	}
	if code := decodeProblem(t, raw).Code; code != "auth.unauthenticated" {
		t.Errorf("code = %q, want auth.unauthenticated", code)
	}
}

// TestSudoRouteIsNotItselfDestructive: gating the re-authentication behind an
// open window would be a lock whose key is inside it.
func TestSudoRouteIsNotItselfDestructive(t *testing.T) {
	s := newServer(t, 5*time.Minute)

	routes := handlers.AuthRoutes(s.deps)
	idx := slices.IndexFunc(routes, func(r httpapi.Route) bool {
		return r.Method == http.MethodPost && r.Pattern == "/api/v1/auth/sudo"
	})
	if idx < 0 {
		t.Fatalf("POST /api/v1/auth/sudo is not registered")
	}
	if routes[idx].Destructive {
		t.Error("the re-authentication route is marked Destructive; it could never be reached")
	}
	if !routes[idx].RequiresSession {
		t.Error("the re-authentication route does not require a session")
	}
	if routes[idx].Action == "" {
		t.Error("the re-authentication route has no audit action token")
	}
}

// TestSudoWindowExpires proves the window is a window. The server runs with a
// short one so the test measures the real expiry path rather than a mocked one.
func TestSudoWindowExpires(t *testing.T) {
	const window = 150 * time.Millisecond

	s := newServer(t, window)
	c := s.newClient(t)
	c.setup()

	if resp, raw := c.sudo(testPass); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sudo: got %d, want 204 (body: %s)", resp.StatusCode, raw)
	}
	time.Sleep(3 * window)

	resp, raw := c.changePassword(testPass, newPass)
	if resp.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("after the window closed: got %d, want 428 (body: %s)", resp.StatusCode, raw)
	}
	if code := decodeProblem(t, raw).Code; code != "sudo.required" {
		t.Errorf("code = %q, want sudo.required", code)
	}
}

// TestDestructiveActionRestartsTheWindow is D-05's usability half: a series of
// destructive actions asks for the password once. The second change happens
// after the original window would have closed and is carried by the refresh the
// first one earned.
func TestDestructiveActionRestartsTheWindow(t *testing.T) {
	const window = 400 * time.Millisecond

	s := newServer(t, window)
	c := s.newClient(t)
	c.setup()

	if resp, raw := c.sudo(testPass); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sudo: got %d, want 204 (body: %s)", resp.StatusCode, raw)
	}

	// Late in the first window, still inside it.
	time.Sleep(window * 5 / 8)
	if resp, raw := c.changePassword(testPass, newPass); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("first change: got %d, want 204 (body: %s)", resp.StatusCode, raw)
	}

	// Past where the original window ended, inside the one the first change
	// restarted.
	time.Sleep(window * 5 / 8)
	const thirdPass = "yet-another-long-passphrase"
	if resp, raw := c.changePassword(newPass, thirdPass); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("second change: got %d, want 204 -- the window was not restarted (body: %s)", resp.StatusCode, raw)
	}
}

// TestSudoWindowIsNotSharedBetweenSessions is T-01-25 at the HTTP boundary: the
// window a stolen cookie would need cannot be borrowed from the real operator.
func TestSudoWindowIsNotSharedBetweenSessions(t *testing.T) {
	s := newServer(t, 5*time.Minute)
	owner := s.newClient(t)
	owner.setup()

	if resp, raw := owner.sudo(testPass); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sudo: got %d, want 204 (body: %s)", resp.StatusCode, raw)
	}

	other := s.newClient(t)
	other.mustLogin()

	resp, raw := other.changePassword(testPass, newPass)
	if resp.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("second session: got %d, want 428 (body: %s)", resp.StatusCode, raw)
	}
	if code := decodeProblem(t, raw).Code; code != "sudo.required" {
		t.Errorf("code = %q, want sudo.required", code)
	}
}
