package httpapi_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube/internal/audit"
	"github.com/holzcloud/holzkube/internal/auth"
	"github.com/holzcloud/holzkube/internal/httpapi"
	"github.com/holzcloud/holzkube/internal/httpapi/handlers"
	"github.com/holzcloud/holzkube/internal/store/fsstore"
)

// TestAuditQueryContract walks GET /api/v1/audit against the shape
// docs/api-contract.md pins, because plan 05 builds its table from that
// document rather than from this code.
func TestAuditQueryContract(t *testing.T) {
	h := newHarness(t)

	// Four records: setup attempt+success, login attempt+success.
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

	// --- limit produces a page and a cursor.
	first := h.auditPage(t, "?limit=2")
	if len(first.Items) != 2 {
		t.Fatalf("first page = %d items, want 2", len(first.Items))
	}
	if first.NextCursor == nil {
		t.Fatal("next_cursor is null although two more records exist")
	}
	if first.Items[0].Seq <= first.Items[1].Seq {
		t.Errorf("page is not newest-first: %d then %d", first.Items[0].Seq, first.Items[1].Seq)
	}

	// --- the cursor continues without overlap and without a gap.
	second := h.auditPage(t, "?limit=2&cursor="+itoa(*first.NextCursor))
	if len(second.Items) != 2 {
		t.Fatalf("second page = %d items, want 2", len(second.Items))
	}
	for _, a := range first.Items {
		for _, b := range second.Items {
			if a.Seq == b.Seq {
				t.Errorf("seq %d appears on both pages", a.Seq)
			}
		}
	}
	if second.NextCursor != nil {
		t.Errorf("next_cursor = %d on the last page, want null", *second.NextCursor)
	}

	// --- exhaustion is null on the wire, not 0 and not an absent field.
	rawLast := h.auditRaw(t, "?limit=2&cursor="+itoa(*first.NextCursor))
	if !strings.Contains(rawLast, `"next_cursor":null`) {
		t.Errorf("last page = %s, want next_cursor null", rawLast)
	}
	if strings.Contains(rawLast, `"next_cursor":0`) {
		t.Errorf("0 was sent as a cursor: %s", rawLast)
	}

	// --- the action filter is exact.
	byAction := h.auditPage(t, "?action=auth.login")
	if len(byAction.Items) != 2 {
		t.Fatalf("action=auth.login returned %d records, want 2", len(byAction.Items))
	}
	for _, rec := range byAction.Items {
		if rec.Action != "auth.login" {
			t.Errorf("filtered page contains %q", rec.Action)
		}
	}

	// --- the time window narrows to what actually happened.
	window := h.auditPage(t, "?from="+rfc3339Param(time.Now().UTC().Add(time.Hour)))
	if len(window.Items) != 0 {
		t.Errorf("a window in the future returned %d records", len(window.Items))
	}

	// --- a malformed parameter is a 400 that names the field.
	resp, raw = h.do(t, http.MethodGet, "/api/v1/audit?from=yesterday", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("from=yesterday: %d, want 400 (%s)", resp.StatusCode, raw)
	}
	p := decodeProblem(t, resp, raw)
	if p.Code != "validation.failed" {
		t.Errorf("code = %q, want validation.failed", p.Code)
	}
	if !strings.Contains(string(raw), `"field":"from"`) {
		t.Errorf("validation problem does not name the field: %s", raw)
	}
}

// TestAuditRequiresASession keeps the log behind authentication: it is the
// record of everything the operator did, and it names actors, addresses and
// parameters.
func TestAuditRequiresASession(t *testing.T) {
	h := newHarness(t)

	anonymous := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // ephemeral test cert
		},
	}
	resp, err := anonymous.Get(h.srv.URL + "/api/v1/audit")
	if err != nil {
		t.Fatalf("GET /api/v1/audit: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (%s)", resp.StatusCode, raw)
	}
	p := decodeProblem(t, resp, raw)
	if p.Code != "auth.unauthenticated" {
		t.Errorf("code = %q, want auth.unauthenticated", p.Code)
	}
}

// TestSystemStatusKeepsAChainBreakVisible is D-15 with teeth: a break found at
// startup is reported identically on every subsequent call, and no request can
// clear it. The only way out is dealing with the file and restarting.
func TestSystemStatusKeepsAChainBreakVisible(t *testing.T) {
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
	seq, err := al.Attempt(context.Background(), audit.Record{Actor: "holz", Action: "auth.login"})
	if err != nil {
		t.Fatalf("Attempt: %v", err)
	}
	if err := al.Outcome(context.Background(), seq, audit.OutcomeSuccess, nil); err != nil {
		t.Fatalf("Outcome: %v", err)
	}
	path := al.CurrentFile()
	if err := al.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	rewriteActor(t, path, "mallory")

	reopened, err := audit.Open(dir)
	if err != nil {
		t.Fatalf("reopen audit: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	ok, file, line, err := reopened.Verify(context.Background())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if ok {
		t.Fatal("startup verification missed a rewritten record")
	}

	au, err := auth.New(st, time.Hour)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	deps := httpapi.Deps{
		Store:      st,
		Audit:      reopened,
		Auth:       au,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		AuditChain: httpapi.ChainStatus{OK: ok, BrokenAtLine: line, File: file},
	}
	deps.Routes = handlers.SystemRoutes(deps)
	srv := httptest.NewTLSServer(httpapi.New(deps))
	t.Cleanup(srv.Close)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // ephemeral test cert
		},
	}

	// Three calls, because a finding that fades after the first read is not a
	// finding. There is no endpoint that acknowledges or recomputes it either.
	for i := range 3 {
		resp, err := client.Get(srv.URL + "/api/v1/system/status")
		if err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var status statusBody
		if err := json.Unmarshal(raw, &status); err != nil {
			t.Fatalf("call %d: decode status: %v (%s)", i+1, err, raw)
		}
		if status.AuditChain.OK {
			t.Fatalf("call %d reports an intact chain after a record was rewritten", i+1)
		}
		if status.AuditChain.BrokenAtLine != 1 {
			t.Errorf("call %d: broken_at_line = %d, want 1", i+1, status.AuditChain.BrokenAtLine)
		}
		// A name, never a path: this endpoint answers before authentication
		// and the data directory is absolute, so a path here would disclose
		// the OS user and their home directory layout to an anonymous caller.
		if want := filepath.Base(path); status.AuditChain.File != want {
			t.Errorf("call %d: file = %q, want %q", i+1, status.AuditChain.File, want)
		}
		if strings.ContainsRune(status.AuditChain.File, filepath.Separator) {
			t.Errorf("call %d: file = %q leaks a filesystem path to an unauthenticated caller",
				i+1, status.AuditChain.File)
		}
	}
}

// rewriteActor performs the edit an attacker with write access would make: it
// changes who did something, leaving the line otherwise intact.
func rewriteActor(t *testing.T, path, actor string) {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // test-owned temp dir
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var rec audit.Record
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("decode line 1: %v", err)
	}
	rec.Actor = actor
	edited, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("encode line 1: %v", err)
	}
	lines[0] = string(edited)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func (h *harness) auditRaw(t *testing.T, query string) string {
	t.Helper()
	resp, raw := h.do(t, http.MethodGet, "/api/v1/audit"+query, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/audit%s: %d (%s)", query, resp.StatusCode, raw)
	}
	return string(raw)
}

func (h *harness) auditPage(t *testing.T, query string) auditBody {
	t.Helper()
	var page auditBody
	if err := json.Unmarshal([]byte(h.auditRaw(t, query)), &page); err != nil {
		t.Fatalf("decode audit page: %v", err)
	}
	return page
}

func itoa(n uint64) string { return strconv.FormatUint(n, 10) }

// rfc3339Param renders a timestamp the way the contract expects it in a query
// string.
func rfc3339Param(ts time.Time) string { return neturl.QueryEscape(ts.Format(time.RFC3339)) }
