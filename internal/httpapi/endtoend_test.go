package httpapi_test

import (
	"bytes"
	"context"
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
	"strings"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/holzcloud/holzkube/internal/audit"
	"github.com/holzcloud/holzkube/internal/auth"
	"github.com/holzcloud/holzkube/internal/httpapi"
	"github.com/holzcloud/holzkube/internal/httpapi/handlers"
	"github.com/holzcloud/holzkube/internal/store/fsstore"
)

const (
	testUser = "operator"
	testPass = "correct-horse-battery-staple"
)

type harness struct {
	srv     *httptest.Server
	client  *http.Client
	dataDir string
	logger  *audit.Logger
}

// newHarness wires the same object graph as cmd/holzkubed against a throwaway
// data directory and serves it over real TLS.
func newHarness(t *testing.T) *harness {
	t.Helper()

	// t.TempDir creates its numbered subdirectory with 0777&^umask, which is
	// 0755 on a normal host. The store's permission guard refuses to open a
	// data directory that group or other can read (FOUND-10), so the fixture
	// has to be as tight as a real data directory is.
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

	chainOK, chainFile, brokenLine, err := al.Verify(context.Background())
	if err != nil {
		t.Fatalf("audit.Verify: %v", err)
	}
	if chainOK {
		chainFile = al.CurrentFile()
	}

	deps := httpapi.Deps{
		Store:      st,
		Audit:      al,
		Auth:       au,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		SudoWindow: 5 * time.Minute,
		AuditChain: httpapi.ChainStatus{OK: chainOK, BrokenAtLine: brokenLine, File: chainFile},
	}
	deps.Routes = slices.Concat(
		handlers.SystemRoutes(deps),
		handlers.SetupRoutes(deps),
		handlers.AuthRoutes(deps),
		handlers.AccountRoutes(deps),
		handlers.AuditRoutes(deps),
	)

	srv := httptest.NewTLSServer(httpapi.New(deps))
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := &http.Client{
		Jar: jar,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test server uses an ephemeral cert
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	return &harness{srv: srv, client: client, dataDir: dir, logger: al}
}

func (h *harness) sessionCookie(t *testing.T) string {
	t.Helper()
	u, err := url.Parse(h.srv.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	for _, c := range h.client.Jar.Cookies(u) {
		if c.Name == "holzkube_session" {
			return c.Value
		}
	}
	return ""
}

type reqOpt func(*http.Request)

func withoutCSRFHeader(r *http.Request) { r.Header.Del("X-Holzkube-CSRF") }

func withTextContentType(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }

func (h *harness) do(t *testing.T, method, path string, body any, opts ...reqOpt) (*http.Response, []byte) {
	t.Helper()

	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, h.srv.URL+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Holzkube-CSRF", "1")
	for _, o := range opts {
		o(req)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, raw
}

type problemBody struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance"`
	Code     string `json:"code"`
}

func decodeProblem(t *testing.T, resp *http.Response, raw []byte) problemBody {
	t.Helper()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("content-type = %q, want application/problem+json (body: %s)", ct, raw)
	}
	var p problemBody
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("decode problem: %v (body: %s)", err, raw)
	}
	if !strings.HasPrefix(p.Type, "https://holzkube.dev/problems/") {
		t.Errorf("problem type = %q, want an absolute holzkube.dev URI", p.Type)
	}
	if p.Code == "" {
		t.Errorf("problem code is empty (body: %s)", raw)
	}
	return p
}

type statusBody struct {
	SetupRequired bool `json:"setup_required"`
	AuditChain    struct {
		OK           bool   `json:"ok"`
		BrokenAtLine int    `json:"broken_at_line"`
		File         string `json:"file"`
	} `json:"audit_chain"`
}

type auditBody struct {
	Items      []audit.Record `json:"items"`
	NextCursor *uint64        `json:"next_cursor"`
}

// TestEndToEndSetupLoginAudit walks the single path this plan exists to prove:
// fresh data dir -> HTTPS -> setup wizard -> first account -> login with session
// rotation -> intent/outcome audit pairs with an intact hash chain.
func TestEndToEndSetupLoginAudit(t *testing.T) {
	h := newHarness(t)

	// --- 1. A fresh install demands setup and starts with an intact (empty) chain.
	resp, raw := h.do(t, http.MethodGet, "/api/v1/system/status", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", resp.StatusCode, raw)
	}
	var st statusBody
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatalf("decode status: %v (body: %s)", err, raw)
	}
	if !st.SetupRequired {
		t.Fatalf("setup_required = false on a fresh data directory")
	}
	if !st.AuditChain.OK {
		t.Fatalf("audit_chain.ok = false on a fresh data directory")
	}

	// --- 2. Setup creates exactly one account and logs it in.
	resp, raw = h.do(t, http.MethodPost, "/api/v1/setup", map[string]string{
		"username": testUser,
		"password": testPass,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: got %d, want 201 (body: %s)", resp.StatusCode, raw)
	}
	afterSetup := h.sessionCookie(t)
	if afterSetup == "" {
		t.Fatalf("setup did not set a %q cookie", "holzkube_session")
	}
	assertSessionCookieFlags(t, resp)

	// --- 3. Login rotates the session id.
	resp, raw = h.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": testUser,
		"password": testPass,
	})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login: got %d, want 204 (body: %s)", resp.StatusCode, raw)
	}
	afterLogin := h.sessionCookie(t)
	if afterLogin == "" {
		t.Fatalf("login cleared the session cookie")
	}
	if afterLogin == afterSetup {
		t.Fatalf("session id was not rotated on login (%q before and after)", afterSetup)
	}

	// --- 4. Both operations are on record as intent/outcome pairs.
	resp, raw = h.do(t, http.MethodGet, "/api/v1/audit", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("audit: got %d, want 200 (body: %s)", resp.StatusCode, raw)
	}
	var ab auditBody
	if err := json.Unmarshal(raw, &ab); err != nil {
		t.Fatalf("decode audit: %v (body: %s)", err, raw)
	}
	if len(ab.Items) != 4 {
		t.Fatalf("audit records = %d, want 4 (setup attempt+success, login attempt+success)", len(ab.Items))
	}
	if _, ok := findRawField(t, raw, "next_cursor"); !ok {
		t.Errorf("audit response omits next_cursor; the contract requires it always be present")
	}
	for i := 1; i < len(ab.Items); i++ {
		if ab.Items[i-1].Seq <= ab.Items[i].Seq {
			t.Fatalf("audit records are not newest-first: seq %d followed by %d", ab.Items[i-1].Seq, ab.Items[i].Seq)
		}
	}
	wantOutcomes := []string{"success", "attempt", "success", "attempt"}
	for i, want := range wantOutcomes {
		if ab.Items[i].Outcome != want {
			t.Errorf("record %d outcome = %q, want %q", i, ab.Items[i].Outcome, want)
		}
	}
	if got := ab.Items[3].Action; got != "setup.create" {
		t.Errorf("oldest record action = %q, want setup.create", got)
	}
	if got := ab.Items[0].Action; got != "auth.login" {
		t.Errorf("newest record action = %q, want auth.login", got)
	}

	// --- 4b. The one thing that must never be in the file is not in the file.
	// This reads the raw bytes rather than the decoded records: a leak that got
	// past redaction would still be a leak if it were nested somewhere the
	// struct does not model.
	assertAuditFileHoldsNoSecret(t, h, afterLogin)

	// --- 5. The chain over what was just written verifies, both via the API and directly.
	_, raw = h.do(t, http.MethodGet, "/api/v1/system/status", nil)
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatalf("decode status: %v (body: %s)", err, raw)
	}
	if st.SetupRequired {
		t.Errorf("setup_required is still true after the first account was created")
	}
	if !st.AuditChain.OK {
		t.Fatalf("audit_chain.ok = false after setup+login (broken at line %d in %s)",
			st.AuditChain.BrokenAtLine, st.AuditChain.File)
	}

	verifier, err := audit.Open(h.dataDir)
	if err != nil {
		t.Fatalf("reopen audit log: %v", err)
	}
	defer verifier.Close()
	ok, _, brokenLine, err := verifier.Verify(context.Background())
	if err != nil {
		t.Fatalf("verify chain: %v", err)
	}
	if !ok {
		t.Fatalf("hash chain broken at line %d", brokenLine)
	}

	// --- 6. Setup is actively dead, not merely hidden.
	resp, raw = h.do(t, http.MethodPost, "/api/v1/setup", map[string]string{
		"username": "second",
		"password": testPass,
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second setup: got %d, want 409 (body: %s)", resp.StatusCode, raw)
	}
	p := decodeProblem(t, resp, raw)
	if p.Type != "https://holzkube.dev/problems/conflict" {
		t.Errorf("second setup type = %q, want .../problems/conflict", p.Type)
	}
	if p.Code != "setup.already-completed" {
		t.Errorf("second setup code = %q, want setup.already-completed", p.Code)
	}

	// --- 7. A wrong password and an unknown user are indistinguishable.
	resp, raw = h.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": testUser,
		"password": "not-the-password",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad password: got %d, want 401 (body: %s)", resp.StatusCode, raw)
	}
	badPass := decodeProblem(t, resp, raw)
	if badPass.Type != "https://holzkube.dev/problems/unauthenticated" {
		t.Errorf("bad password type = %q, want .../problems/unauthenticated", badPass.Type)
	}
	if strings.Contains(strings.ToLower(badPass.Detail+badPass.Title), "password") &&
		strings.Contains(badPass.Detail, testUser) {
		t.Errorf("401 body leaks whether the username exists: %s", raw)
	}

	resp, raw = h.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": "no-such-operator",
		"password": testPass,
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown user: got %d, want 401 (body: %s)", resp.StatusCode, raw)
	}
	unknownUser := decodeProblem(t, resp, raw)
	if unknownUser.Title != badPass.Title || unknownUser.Code != badPass.Code {
		t.Errorf("unknown user (%s/%s) is distinguishable from a wrong password (%s/%s)",
			unknownUser.Title, unknownUser.Code, badPass.Title, badPass.Code)
	}

	// --- 8. CSRF preconditions are all required, simultaneously.
	resp, raw = h.do(t, http.MethodPost, "/api/v1/setup",
		map[string]string{"username": "x", "password": testPass}, withoutCSRFHeader)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("missing CSRF header: got %d, want 403 (body: %s)", resp.StatusCode, raw)
	}
	if got := decodeProblem(t, resp, raw).Code; got != "csrf.precondition-unmet" {
		t.Errorf("missing CSRF header code = %q, want csrf.precondition-unmet", got)
	}

	resp, raw = h.do(t, http.MethodPost, "/api/v1/setup",
		map[string]string{"username": "x", "password": testPass}, withTextContentType)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-JSON content type: got %d, want 403 (body: %s)", resp.StatusCode, raw)
	}
	if got := decodeProblem(t, resp, raw).Code; got != "csrf.precondition-unmet" {
		t.Errorf("non-JSON content type code = %q, want csrf.precondition-unmet", got)
	}

	// --- 9. Concurrent failures do not share state: each gets its own instance.
	// The requests run in parallel; every assertion happens back on the test
	// goroutine, because t.Fatalf outside it is undefined behaviour.
	type concurrentResult struct {
		status int
		body   []byte
		ctype  string
	}
	results := make([]concurrentResult, 2)
	var g errgroup.Group
	for i := range results {
		g.Go(func() error {
			body, err := json.Marshal(map[string]string{
				"username": testUser,
				"password": "still-not-the-password",
			})
			if err != nil {
				return err
			}
			req, err := http.NewRequest(http.MethodPost, h.srv.URL+"/api/v1/auth/login", bytes.NewReader(body))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Holzkube-CSRF", "1")

			resp, err := h.client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			results[i] = concurrentResult{status: resp.StatusCode, body: raw, ctype: resp.Header.Get("Content-Type")}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("concurrent logins: %v", err)
	}

	instances := make([]string, len(results))
	for i, res := range results {
		if res.status != http.StatusUnauthorized {
			t.Fatalf("concurrent login %d: got %d, want 401 (body: %s)", i, res.status, res.body)
		}
		if !strings.HasPrefix(res.ctype, "application/problem+json") {
			t.Fatalf("concurrent login %d content-type = %q, want application/problem+json", i, res.ctype)
		}
		var p problemBody
		if err := json.Unmarshal(res.body, &p); err != nil {
			t.Fatalf("concurrent login %d: decode problem: %v (body: %s)", i, err, res.body)
		}
		if p.Instance == "" {
			t.Fatalf("concurrent login %d has an empty instance (body: %s)", i, res.body)
		}
		instances[i] = p.Instance
	}
	if instances[0] == instances[1] {
		t.Fatalf("concurrent problems share the instance %q; request ids leaked between requests", instances[0])
	}
}

func assertSessionCookieFlags(t *testing.T, resp *http.Response) {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name != "holzkube_session" {
			continue
		}
		if !c.HttpOnly {
			t.Errorf("session cookie is not HttpOnly")
		}
		if !c.Secure {
			t.Errorf("session cookie is not Secure")
		}
		if c.SameSite != http.SameSiteLaxMode {
			t.Errorf("session cookie SameSite = %v, want Lax", c.SameSite)
		}
		return
	}
	t.Errorf("response set no holzkube_session cookie")
}

func findRawField(t *testing.T, raw []byte, field string) (json.RawMessage, bool) {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, false
	}
	v, ok := m[field]
	return v, ok
}

// assertAuditFileHoldsNoSecret reads the day file as bytes and fails if the
// password, or a usable session token, survived into it. D-16 keeps the file
// forever, so anything written here is written permanently (T-01-16).
func assertAuditFileHoldsNoSecret(t *testing.T, h *harness, sessionToken string) {
	t.Helper()

	raw, err := os.ReadFile(h.logger.CurrentFile()) //nolint:gosec // test-owned temp dir
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	body := string(raw)

	if strings.Contains(body, testPass) {
		t.Fatalf("the operator password is in the audit log")
	}
	if sessionToken != "" && strings.Contains(body, sessionToken) {
		t.Fatalf("a live session token is in the audit log")
	}
	if !strings.Contains(body, `"password":"<redacted>"`) {
		t.Errorf("no redaction marker in the log; the body was not captured at all:\n%s", body)
	}
	if !strings.Contains(body, `"username":"`+testUser+`"`) {
		t.Errorf("the allowlisted username did not survive into the log:\n%s", body)
	}
	// JSONL: every line is a whole record on its own.
	for i, line := range strings.Split(strings.TrimSpace(body), "\n") {
		var rec audit.Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Errorf("line %d is not a complete record: %v", i+1, err)
		}
	}
}
