package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/holzcloud/holzkube/internal/httpapi"
	"github.com/holzcloud/holzkube/internal/httpapi/middleware"
)

// taxonomy is the closed set of problems holzkube can produce. Every entry is
// exercised by TestProblemTaxonomy below; an entry added to problem.go without
// a line here is an entry nobody checked.
var taxonomy = []struct {
	name       string
	problem    *httpapi.Problem
	wantType   string
	wantStatus int
	wantCode   string
}{
	{"validation", httpapi.Validation("bad body"), httpapi.TypeValidation, http.StatusBadRequest, "validation.failed"},
	{"unauthenticated", httpapi.Unauthenticated(), httpapi.TypeUnauthenticated, http.StatusUnauthorized, "auth.unauthenticated"},
	{"csrf", httpapi.CSRFFailed("missing header"), httpapi.TypeCSRF, http.StatusForbidden, "csrf.precondition-unmet"},
	{"forbidden", httpapi.Forbidden("forbidden.action", "nope"), httpapi.TypeForbidden, http.StatusForbidden, "forbidden.action"},
	{"not-found", httpapi.NotFound("notfound.route", "no such endpoint"), httpapi.TypeNotFound, http.StatusNotFound, "notfound.route"},
	{"method-not-allowed", httpapi.MethodNotAllowed("wrong method"), httpapi.TypeMethodNotAllowed, http.StatusMethodNotAllowed, "method.not-allowed"},
	{"conflict-store", httpapi.Conflict("store.conflict", "rev mismatch"), httpapi.TypeConflict, http.StatusConflict, "store.conflict"},
	{"conflict-setup", httpapi.Conflict("setup.already-completed", "done"), httpapi.TypeConflict, http.StatusConflict, "setup.already-completed"},
	{"unsupported-media-type", httpapi.UnsupportedMediaType("send json"), httpapi.TypeUnsupportedMediaType, http.StatusUnsupportedMediaType, "media.unsupported"},
	{"sudo-required", httpapi.SudoRequired(), httpapi.TypeSudoRequired, http.StatusPreconditionRequired, "sudo.required"},
	{"rate-limited", httpapi.RateLimited(30), httpapi.TypeRateLimited, http.StatusTooManyRequests, "ratelimit.delayed"},
	{"internal", httpapi.Internal(fmt.Errorf("boom")), httpapi.TypeInternal, http.StatusInternalServerError, "internal.unexpected"},
	{"setup-required", httpapi.SetupRequired(), httpapi.TypeSetupRequired, http.StatusServiceUnavailable, "setup.required"},
}

// TestProblemTaxonomy checks the invariants every entry must hold: the media
// type, an absolute type URI, a status, a non-empty title and a stable code.
func TestProblemTaxonomy(t *testing.T) {
	for _, tc := range taxonomy {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
			httpapi.WriteProblem(rec, req, tc.problem)

			if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
				t.Errorf("Content-Type = %q, want application/problem+json", got)
			}
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}

			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v (body: %s)", err, rec.Body.String())
			}

			gotType, _ := body["type"].(string)
			if gotType != tc.wantType {
				t.Errorf("type = %q, want %q", gotType, tc.wantType)
			}
			if !strings.HasPrefix(gotType, httpapi.ProblemBaseURI) {
				t.Errorf("type %q is not an absolute URI under %s", gotType, httpapi.ProblemBaseURI)
			}
			if gotType == "about:blank" {
				t.Errorf("about:blank is never a valid holzkube problem type")
			}

			if title, _ := body["title"].(string); strings.TrimSpace(title) == "" {
				t.Errorf("title is empty")
			}
			if status, ok := body["status"].(float64); !ok || int(status) != tc.wantStatus {
				t.Errorf("status member = %v, want %d", body["status"], tc.wantStatus)
			}
			if code, _ := body["code"].(string); code != tc.wantCode {
				t.Errorf("code = %q, want %q", code, tc.wantCode)
			}
		})
	}
}

// TestProblemRateLimitedSetsRetryAfter checks the one entry with a header
// obligation. Telling a client to back off without saying for how long makes
// the delay unusable.
func TestProblemRateLimitedSetsRetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	httpapi.WriteProblem(rec, req, httpapi.RateLimited(30))

	if got := rec.Header().Get("Retry-After"); got != "30" {
		t.Errorf("Retry-After = %q, want 30", got)
	}
}

// TestProblemInternalLeaksNothing is the test behind the prohibition: an
// internal error must never hand a client a filesystem path, a Go error string
// or a stack trace. This binary holds cluster PKI, so any of those is free
// reconnaissance for whoever is probing it.
func TestProblemInternalLeaksNothing(t *testing.T) {
	const (
		secretPath = "/Users/holz/.local/share/holzkube/users/a1b2c3.json"
		goErrText  = "open " + secretPath + ": permission denied"
	)
	cause := fmt.Errorf("load user record: %w", fmt.Errorf("%s", goErrText))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	httpapi.WriteProblem(rec, req, httpapi.Internal(cause))

	body := rec.Body.String()
	for _, forbidden := range []string{
		secretPath,
		goErrText,
		"permission denied",
		"load user record",
		".local/share/holzkube",
		"users/",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("internal problem leaked %q:\n%s", forbidden, body)
		}
	}

	var p map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail, ok := p["detail"]; ok && detail != "" {
		t.Errorf("internal problem carries a detail member (%v); only instance may be exposed", detail)
	}
}

// TestProblemInstancesAreDistinctUnderConcurrency runs two failing requests at once
// through the real request-id middleware and checks that neither borrows the
// other's identity. A shared instance would make the log useless exactly when
// two things go wrong at the same time.
func TestProblemInstancesAreDistinctUnderConcurrency(t *testing.T) {
	const workers = 32

	handler := middleware.RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
	}))

	var wg sync.WaitGroup
	instances := make([]string, workers)

	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil))
			var p struct {
				Instance string `json:"instance"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err == nil {
				instances[i] = p.Instance
			}
		}()
	}
	wg.Wait()

	seen := make(map[string]int, workers)
	for i, inst := range instances {
		if inst == "" {
			t.Fatalf("worker %d produced a problem with no instance", i)
		}
		if !strings.HasPrefix(inst, "/requests/") {
			t.Errorf("instance %q is not of the form /requests/<request-id>", inst)
		}
		seen[inst]++
	}
	if len(seen) != workers {
		t.Errorf("got %d distinct instances across %d concurrent requests; ids leaked between requests",
			len(seen), workers)
	}
}
