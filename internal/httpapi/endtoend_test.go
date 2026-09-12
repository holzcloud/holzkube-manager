package httpapi_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
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

	"github.com/holzcloud/holzkube-manager/internal/audit"
	"github.com/holzcloud/holzkube-manager/internal/auth"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/httpapi/handlers"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/metrics"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/nodestream"
	"github.com/holzcloud/holzkube-manager/internal/provision"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/streamhub"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
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
	store   *fsstore.Store
	inv     *inventory.Service
	hub     *streamhub.Hub
	streams *nodestream.Manager
	jobs    *jobs.Engine

	// bootstrap is the etcd lease directory, shared by the provisioning
	// service and the job steps. One per harness, for the reason the lease
	// exists at all.
	bootstrap *provision.Bootstrapper
}

// harnessOpt adjusts the object graph before it is served.
//
// It exists so that a test about the inventory can add the inventory service
// and its routes without a second copy of this wiring: the point of the
// harness is that it is the same graph cmd/holzkube-managerd builds, and two copies of
// that claim would drift.
type harnessOpt func(*harnessConfig)

type harnessConfig struct {
	inventory            func(store *fsstore.Store) *inventory.Service
	streaming            bool
	jobs                 bool
	config               bool
	provision            func(*harness) *provision.Service
	registerProvisionJob func(*jobs.Engine, *harness)
	upgrade              func(*harness) *upgrade.Service
	allowedHosts         []string
}

// withAllowedHosts turns the host allowlist on.
//
// It is off by default in the harness because httptest picks the port, and a
// test cannot know it in advance to put it on a list. A test about the
// allowlist itself sets a name and sends the Host header by hand.
func withAllowedHosts(hosts ...string) harnessOpt {
	return func(c *harnessConfig) { c.allowedHosts = hosts }
}

// withInventory adds an inventory service built over the harness's store.
func withInventory(build func(store *fsstore.Store) *inventory.Service) harnessOpt {
	return func(c *harnessConfig) { c.inventory = build }
}

// withStreaming adds the hub, the node-stream manager and the stream route.
func withStreaming() harnessOpt {
	return func(c *harnessConfig) { c.streaming = true }
}

// withConfig adds the machine-configuration service and its routes.
func withConfig() harnessOpt {
	return func(c *harnessConfig) { c.config = true }
}

// withProvision adds the provisioning service and its routes. It implies jobs,
// because an apply is a job submission and nothing else.
func withProvision(build func(*harness) *provision.Service) harnessOpt {
	return func(c *harnessConfig) {
		c.provision = build
		c.jobs = true
		c.streaming = true
	}
}

// withProvisionJob teaches the harness's engine how to run a provisioning job.
// It is separate from withProvision because a test about the routes does not
// need the steps, and registering them would run a machine install.
func withProvisionJob(register func(*jobs.Engine, *harness)) harnessOpt {
	return func(c *harnessConfig) { c.registerProvisionJob = register }
}

// withUpgrade adds the rolling-upgrade and etcd-management service. It implies
// jobs, because an upgrade is a job submission.
func withUpgrade(build func(*harness) *upgrade.Service) harnessOpt {
	return func(c *harnessConfig) {
		c.upgrade = build
		c.jobs = true
		c.streaming = true
	}
}

// withJobs adds the job engine, the confirmer and the node-action routes. It
// implies streaming, because job progress rides on the same hub.
func withJobs() harnessOpt {
	return func(c *harnessConfig) {
		c.jobs = true
		c.streaming = true
	}
}

// newHarness wires the same object graph as cmd/holzkube-managerd against a throwaway
// data directory and serves it over real TLS.
func newHarness(t *testing.T, opts ...harnessOpt) *harness {
	t.Helper()

	var cfg harnessConfig
	for _, o := range opts {
		o(&cfg)
	}

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
		Store:        st,
		Audit:        al,
		Auth:         au,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		SudoWindow:   5 * time.Minute,
		AuditChain:   httpapi.ChainStatus{OK: chainOK, BrokenAtLine: brokenLine, File: chainFile},
		AllowedHosts: cfg.allowedHosts,
	}
	h2 := &harness{}

	var inv *inventory.Service
	if cfg.inventory != nil {
		inv = cfg.inventory(st)
		t.Cleanup(func() { _ = inv.Close() })

		// Inside the struct, deliberately: Deps is copied by value into each
		// …Routes call below, so a field assigned after this point would be
		// nil in every handler closure with no compile error -- and the lock
		// would silently not exist.
		deps.Inventory = inv
		deps.ClusterLocked = func(r *http.Request, cluster string) error {
			return inv.CheckLock(r.Context(), model.ClusterID(cluster))
		}
	}

	if cfg.streaming {
		hub := streamhub.New()
		t.Cleanup(func() { _ = hub.Close() })

		ns := nodestream.New(nodestream.Deps{
			Hub:    hub,
			Logger: deps.Logger,
			Open: func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
				if inv == nil {
					return nil, errors.New("no inventory")
				}
				return inv.Connect(ctx, id)
			},
			// Short, so a test that closes a panel and asserts the reader
			// stopped does not wait half a minute for it.
			Linger:       50 * time.Millisecond,
			RetryBackoff: 50 * time.Millisecond,
		})
		t.Cleanup(func() { _ = ns.Close() })

		deps.Hub = hub
		deps.NodeStreams = ns
		h2.hub = hub
		h2.streams = ns
	}

	if cfg.jobs {
		engine := jobs.New(jobs.Deps{Store: st, Logger: deps.Logger, Hub: deps.Hub})
		t.Cleanup(func() { _ = engine.Close() })

		jobs.RegisterNodeActions(engine, func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
			if inv == nil {
				return nil, errors.New("no inventory")
			}
			return inv.Connect(ctx, id)
		})

		confirmer, err := jobs.NewConfirmer()
		if err != nil {
			t.Fatalf("NewConfirmer: %v", err)
		}

		deps.Jobs = engine
		deps.Confirmer = confirmer
		h2.jobs = engine
	}

	if cfg.config {
		deps.Config = machineconfig.New(machineconfig.Deps{
			Connect: func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
				if inv == nil {
					return nil, errors.New("no inventory")
				}
				return inv.Connect(ctx, id)
			},
		})
	}

	h2.dataDir = dir
	h2.store = st
	h2.inv = inv
	if cfg.provision != nil {
		deps.Provision = cfg.provision(h2)
	}
	if cfg.upgrade != nil {
		deps.Upgrade = cfg.upgrade(h2)
	}
	if cfg.registerProvisionJob != nil && h2.jobs != nil {
		cfg.registerProvisionJob(h2.jobs, h2)
	}

	// The Prometheus exporter, wired the way the composition root wires it:
	// over the same read model the API serves. A second source here would make
	// a metrics test pass against numbers no screen ever shows.
	if inv != nil {
		engine := deps.Jobs
		deps.Metrics = metrics.New(metrics.Deps{
			Machines: inv.Machines,
			Clusters: inv.Clusters,
			Jobs: func(ctx context.Context) ([]model.Job, error) {
				if engine == nil {
					return nil, nil
				}
				return engine.List(ctx)
			},
			AuditChainIntact: func() bool { return chainOK },
		})
	}

	deps.Routes = slices.Concat(
		handlers.SystemRoutes(deps),
		handlers.MetricsRoutes(deps),
		handlers.SetupRoutes(deps),
		handlers.AuthRoutes(deps),
		handlers.AccountRoutes(deps),
		handlers.AuditRoutes(deps),
		handlers.InventoryRoutes(deps),
		handlers.StreamRoutes(deps),
		handlers.JobRoutes(deps),
		handlers.ConfigRoutes(deps),
		handlers.ProvisionRoutes(deps),
		handlers.UpgradeRoutes(deps),
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

	h2.srv = srv
	h2.client = client
	h2.dataDir = dir
	h2.logger = al
	h2.store = st
	h2.inv = inv
	return h2
}

func (h *harness) sessionCookie(t *testing.T) string {
	t.Helper()
	u, err := url.Parse(h.srv.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	for _, c := range h.client.Jar.Cookies(u) {
		if c.Name == "holzkube-manager_session" {
			return c.Value
		}
	}
	return ""
}

type reqOpt func(*http.Request)

func withoutCSRFHeader(r *http.Request) { r.Header.Del("X-Holzkube-Manager-CSRF") }

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
	req.Header.Set("X-Holzkube-Manager-CSRF", "1")
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
	if !strings.HasPrefix(p.Type, httpapi.ProblemBaseURI) {
		t.Errorf("problem type = %q, want a URI rooted at %s", p.Type, httpapi.ProblemBaseURI)
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
		t.Fatalf("setup did not set a %q cookie", "holzkube-manager_session")
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
	if want := httpapi.ProblemBaseURI + "conflict"; p.Type != want {
		t.Errorf("second setup type = %q, want %q", p.Type, want)
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
	if want := httpapi.ProblemBaseURI + "unauthenticated"; badPass.Type != want {
		t.Errorf("bad password type = %q, want %q", badPass.Type, want)
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
			req.Header.Set("X-Holzkube-Manager-CSRF", "1")

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
		if c.Name != "holzkube-manager_session" {
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
	t.Errorf("response set no holzkube-manager_session cookie")
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
