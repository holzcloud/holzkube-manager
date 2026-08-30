package httpapi_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
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

	// The upstream family. One type, four reserved codes: the type axis stays a
	// taxonomy of failure kinds, and the code stays the fine-grained
	// discriminator the contract already promises.
	{"upstream-node-unreachable", httpapi.Upstream(httpapi.CodeUpstreamNodeUnreachable, "The node did not answer."), httpapi.TypeUpstream, http.StatusBadGateway, "upstream.node-unreachable"},
	{"upstream-node-timeout", httpapi.Upstream(httpapi.CodeUpstreamNodeTimeout, "The node did not answer in time."), httpapi.TypeUpstream, http.StatusBadGateway, "upstream.node-timeout"},
	{"upstream-factory-unavailable", httpapi.Upstream(httpapi.CodeUpstreamFactoryUnavailable, "The Image Factory did not answer."), httpapi.TypeUpstream, http.StatusBadGateway, "upstream.factory-unavailable"},
	{"upstream-factory-rejected", httpapi.Upstream(httpapi.CodeUpstreamFactoryRejected, "The Image Factory refused the schematic."), httpapi.TypeUpstream, http.StatusBadGateway, "upstream.factory-rejected"},
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
				t.Errorf("type %q is not rooted at the taxonomy base %s", gotType, httpapi.ProblemBaseURI)
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

// TestProblemUpstreamCodesAreReserved pins the four code tokens minted with the
// type. They are reserved in the same commit as the type on purpose: plan 02-05
// (transport failures) and plan 02-06 (factory failures) are written later and
// against this list, so neither can invent a divergent spelling of a code the
// contract says never changes.
func TestProblemUpstreamCodesAreReserved(t *testing.T) {
	want := map[string]string{
		"CodeUpstreamNodeUnreachable":    "upstream.node-unreachable",
		"CodeUpstreamNodeTimeout":        "upstream.node-timeout",
		"CodeUpstreamFactoryUnavailable": "upstream.factory-unavailable",
		"CodeUpstreamFactoryRejected":    "upstream.factory-rejected",
	}
	got := map[string]string{
		"CodeUpstreamNodeUnreachable":    httpapi.CodeUpstreamNodeUnreachable,
		"CodeUpstreamNodeTimeout":        httpapi.CodeUpstreamNodeTimeout,
		"CodeUpstreamFactoryUnavailable": httpapi.CodeUpstreamFactoryUnavailable,
		"CodeUpstreamFactoryRejected":    httpapi.CodeUpstreamFactoryRejected,
	}
	for name, wantCode := range want {
		if got[name] != wantCode {
			t.Errorf("%s = %q, want %q", name, got[name], wantCode)
		}
		if !strings.HasPrefix(got[name], "upstream.") {
			t.Errorf("%s = %q, which is outside the reserved upstream.* prefix", name, got[name])
		}
	}
}

// TestProblemUpstreamTakesNoError pins the shape of the constructor rather than
// its output.
//
// Internal accepts an error and deliberately drops it. Upstream must not accept
// one at all: a constructor that takes an error is a constructor somebody
// eventually passes a wrapped Go error to, and unlike Internal this type does
// put its detail on the wire. Forcing a string means every detail that reaches
// a client was typed out by a person who could see what it said.
func TestProblemUpstreamTakesNoError(t *testing.T) {
	fn := reflect.TypeOf(httpapi.Upstream)
	if fn.Kind() != reflect.Func {
		t.Fatalf("httpapi.Upstream is %s, want a function", fn.Kind())
	}
	if fn.NumIn() != 2 {
		t.Fatalf("Upstream takes %d parameters, want 2 (code, detail)", fn.NumIn())
	}

	errType := reflect.TypeOf((*error)(nil)).Elem()
	for i := range fn.NumIn() {
		in := fn.In(i)
		if in == errType || in.Implements(errType) {
			t.Fatalf("Upstream parameter %d is %s; the constructor must never take an error", i, in)
		}
		if in.Kind() != reflect.String {
			t.Errorf("Upstream parameter %d is %s, want string", i, in)
		}
	}
	if fn.NumOut() != 1 || fn.Out(0) != reflect.TypeOf((*httpapi.Problem)(nil)) {
		t.Errorf("Upstream returns %v, want a single *httpapi.Problem", fn.Out(0))
	}
}

// TestProblemUpstreamCarriesADetail is the counterfactual to
// TestProblemInternalLeaksNothing. internal has no detail by contract, which is
// exactly why an unreachable upstream must not fall through to it: the whole
// point of the mint is that the operator is told which dependency failed.
func TestProblemUpstreamCarriesADetail(t *testing.T) {
	const detail = "The Image Factory at factory.talos.dev did not answer within the client timeout."

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/schematics", nil)
	httpapi.WriteProblem(rec, req, httpapi.Upstream(httpapi.CodeUpstreamFactoryUnavailable, detail))

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, rec.Body.String())
	}
	if got, _ := body["detail"].(string); got != detail {
		t.Errorf("detail = %q, want the caller's prose %q", got, detail)
	}
	if got, _ := body["type"].(string); got == httpapi.TypeInternal {
		t.Errorf("an upstream failure serialised as %s; the mint exists to stop exactly that", httpapi.TypeInternal)
	}
}

// TestProblemUpstreamTravelsAsAnError checks that an upstream problem can be
// returned as an error from a service package and reach a handler unchanged.
// Without this, a service would have to invent a parallel error type and the
// handler would have to translate it back, which is where codes drift.
func TestProblemUpstreamTravelsAsAnError(t *testing.T) {
	var err error = httpapi.Upstream(httpapi.CodeUpstreamNodeTimeout, "The node did not answer in time.")

	const want = "upstream.node-timeout: An upstream dependency did not answer"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	var p *httpapi.Problem
	if !errors.As(err, &p) {
		t.Fatalf("an upstream problem does not survive errors.As into *httpapi.Problem")
	}
	if p.Code != httpapi.CodeUpstreamNodeTimeout {
		t.Errorf("recovered code = %q, want %q", p.Code, httpapi.CodeUpstreamNodeTimeout)
	}
}

// typeTaxonomy is the taxonomy spelled out as (constant, suffix) pairs. It is
// the second opinion on problem.go: the constants there are derived from
// ProblemBaseURI, which is what stops a re-rooting from landing on twelve of
// thirteen, and this table is what stops the derivation itself from drifting.
//
// The suffixes are the parts clients may already match on. They do not change
// when the base does -- that is the whole point of separating them.
var typeTaxonomy = []struct {
	name   string
	suffix string
	got    string
}{
	{"TypeValidation", "validation", httpapi.TypeValidation},
	{"TypeUnauthenticated", "unauthenticated", httpapi.TypeUnauthenticated},
	{"TypeCSRF", "csrf", httpapi.TypeCSRF},
	{"TypeForbidden", "forbidden", httpapi.TypeForbidden},
	{"TypeNotFound", "not-found", httpapi.TypeNotFound},
	{"TypeMethodNotAllowed", "method-not-allowed", httpapi.TypeMethodNotAllowed},
	{"TypeConflict", "conflict", httpapi.TypeConflict},
	{"TypeUnsupportedMediaType", "unsupported-media-type", httpapi.TypeUnsupportedMediaType},
	{"TypeSudoRequired", "sudo-required", httpapi.TypeSudoRequired},
	{"TypeRateLimited", "rate-limited", httpapi.TypeRateLimited},
	{"TypeInternal", "internal", httpapi.TypeInternal},
	{"TypeSetupRequired", "setup-required", httpapi.TypeSetupRequired},
	{"TypeUpstream", "upstream", httpapi.TypeUpstream},
}

// TestProblemTaxonomyIsRootedAtTheURN pins the base itself.
//
// The taxonomy is deployment-independent by construction: there is no flag, no
// environment variable and no build tag that moves it, so this literal is the
// value every installation of holzkube emits. Asserting it here means a
// re-rooting is a decision someone takes deliberately in two places, not a
// character someone changes in one.
func TestProblemTaxonomyIsRootedAtTheURN(t *testing.T) {
	const wantBase = "urn:holzkube-manager:problem:"

	if httpapi.ProblemBaseURI != wantBase {
		t.Fatalf("ProblemBaseURI = %q, want %q", httpapi.ProblemBaseURI, wantBase)
	}
	if len(typeTaxonomy) != 13 {
		t.Fatalf("taxonomy has %d entries, want 13", len(typeTaxonomy))
	}

	for _, tc := range typeTaxonomy {
		if want := wantBase + tc.suffix; tc.got != want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, want)
		}
	}
}

// TestProblemTaxonomyIsClosed reads problem.go itself, because "closed" is a
// claim about the source and not about the values a test happens to name.
//
// A test that only walked the table above would pass while a fourteenth type
// was minted quietly beside it; the point of a closed taxonomy is that adding
// to it is a decision rather than an edit. This also asserts the shape of each
// declaration -- base plus suffix, never a repeated literal -- because that
// shape is what makes a partial re-rooting impossible to express.
func TestProblemTaxonomyIsClosed(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "problem.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse problem.go: %v", err)
	}

	declared := map[string]ast.Expr{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range spec.Names {
			if !strings.HasPrefix(name.Name, "Type") || i >= len(spec.Values) {
				continue
			}
			declared[name.Name] = spec.Values[i]
		}
		return true
	})

	want := map[string]string{}
	for _, tc := range typeTaxonomy {
		want[tc.name] = tc.suffix
	}

	for name := range declared {
		if _, ok := want[name]; !ok {
			t.Errorf("problem.go declares %s, which is not in the closed taxonomy; adding a type is a contract decision, not an edit", name)
		}
	}
	for name := range want {
		if _, ok := declared[name]; !ok {
			t.Errorf("%s is in the taxonomy table but no longer declared in problem.go", name)
		}
	}

	for name, expr := range declared {
		suffix, ok := want[name]
		if !ok {
			continue
		}
		bin, ok := expr.(*ast.BinaryExpr)
		if !ok {
			t.Errorf("%s is not declared as ProblemBaseURI + suffix; a repeated literal is how a re-rooting lands on twelve of thirteen", name)
			continue
		}
		base, ok := bin.X.(*ast.Ident)
		if !ok || base.Name != "ProblemBaseURI" {
			t.Errorf("%s does not derive from ProblemBaseURI", name)
			continue
		}
		lit, ok := bin.Y.(*ast.BasicLit)
		if !ok || lit.Value != strconv.Quote(suffix) {
			t.Errorf("%s composes %v, want the suffix %q", name, bin.Y, suffix)
		}
	}
}
