package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/holzcloud/holzkube/internal/audit"
)

type intent struct {
	action string
	srcIP  string
	params map[string]any
}

type result struct {
	seq     uint64
	outcome string
	cause   error
}

// recorder is the audit log as this middleware sees it: two calls, in order.
type recorder struct {
	mu       sync.Mutex
	intents  []intent
	results  []result
	failWith error
}

func (r *recorder) Attempt(_ context.Context, action, srcIP string, params map[string]any) (uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failWith != nil {
		return 0, r.failWith
	}
	r.intents = append(r.intents, intent{action: action, srcIP: srcIP, params: params})
	return uint64(len(r.intents)), nil
}

func (r *recorder) Outcome(_ context.Context, seq uint64, outcome string, cause error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, result{seq: seq, outcome: outcome, cause: cause})
	return nil
}

// serve runs one request through the audit link wrapped around handler.
func serve(t *testing.T, rec *recorder, action, method, body string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()

	var deny = func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.WriteHeader(http.StatusInternalServerError)
	}
	mw := Chain(
		RequestID(),
		Audit(rec, action, IsMutating(method), deny),
	)

	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	var req *http.Request
	if rdr != nil {
		req = httptest.NewRequest(method, "/api/v1/whatever", rdr)
	} else {
		req = httptest.NewRequest(method, "/api/v1/whatever", nil)
	}
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.0.10:51234"

	w := httptest.NewRecorder()
	mw(handler).ServeHTTP(w, req)
	return w
}

func ok(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }

// TestAuditWritesIntentThenOutcome is FOUND-06 in its simplest form: two
// records, in that order, linked by the intent's sequence number.
func TestAuditWritesIntentThenOutcome(t *testing.T) {
	rec := &recorder{}
	serve(t, rec, "auth.login", http.MethodPost, `{"username":"holz","password":"s"}`, ok)

	if len(rec.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(rec.intents))
	}
	if len(rec.results) != 1 {
		t.Fatalf("outcomes = %d, want 1", len(rec.results))
	}
	if rec.results[0].seq != 1 {
		t.Errorf("outcome refers to seq %d, want the intent's 1", rec.results[0].seq)
	}
	if rec.results[0].outcome != audit.OutcomeSuccess {
		t.Errorf("outcome = %q, want %q", rec.results[0].outcome, audit.OutcomeSuccess)
	}
	if rec.intents[0].action != "auth.login" {
		t.Errorf("action = %q, want auth.login", rec.intents[0].action)
	}
	if rec.intents[0].srcIP != "192.168.0.10" {
		t.Errorf("src_ip = %q, want 192.168.0.10", rec.intents[0].srcIP)
	}
}

// TestAuditRecordsTheStableCodeNotTheGoError holds the line the taxonomy draws:
// the record carries the contract's code, never an internal error string that
// could name a path or a store message.
func TestAuditRecordsTheStableCodeNotTheGoError(t *testing.T) {
	rec := &recorder{}
	// The body is spelled out rather than composed from httpapi.ProblemBaseURI,
	// and that is a decision rather than an oversight. This is the internal test
	// package of middleware, and httpapi imports middleware, so the constant is
	// not reachable from here at all. It is also the right shape for what this
	// test does: the bytes stand in for whatever a handler upstream of the
	// middleware wrote, and the claim under test is that the recorder reads
	// `code` and never `type`. A composed expectation would suggest the
	// middleware cares about the base. It does not.
	serve(t, rec, "auth.login", http.MethodPost, `{"username":"holz"}`, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"urn:holzkube-manager:problem:internal","status":500,` +
			`"instance":"/requests/abc","code":"internal.unexpected"}`))
	})

	if len(rec.results) != 1 {
		t.Fatalf("outcomes = %d, want 1", len(rec.results))
	}
	if rec.results[0].outcome != audit.OutcomeError {
		t.Errorf("outcome = %q, want %q", rec.results[0].outcome, audit.OutcomeError)
	}
	cause := rec.results[0].cause
	if cause == nil {
		t.Fatal("a failed request recorded no reason")
	}
	if cause.Error() != "internal.unexpected" {
		t.Errorf("recorded reason = %q, want the stable code internal.unexpected", cause.Error())
	}
}

// TestAuditFallsBackToTheStatusWhenThereIsNoCode covers a failure produced by
// something other than the taxonomy: still no raw text, just the status.
func TestAuditFallsBackToTheStatusWhenThereIsNoCode(t *testing.T) {
	rec := &recorder{}
	serve(t, rec, "auth.login", http.MethodPost, `{}`, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom: /Users/holz/secret/path", http.StatusBadGateway)
	})

	cause := rec.results[0].cause
	if cause == nil {
		t.Fatal("a failed request recorded no reason")
	}
	if strings.Contains(cause.Error(), "/Users") || strings.Contains(cause.Error(), "boom") {
		t.Errorf("recorded reason leaks the handler's text: %q", cause.Error())
	}
	if cause.Error() != "http.502" {
		t.Errorf("recorded reason = %q, want http.502", cause.Error())
	}
}

// TestAuditLeavesAnIntentWithoutAnOutcomeOnPanic is the finding P16 names: the
// interesting operations are the ones that did not finish. A half pair is the
// signal, and nothing here completes it after the fact.
func TestAuditLeavesAnIntentWithoutAnOutcomeOnPanic(t *testing.T) {
	rec := &recorder{}

	defer func() {
		if r := recover(); r == nil {
			t.Error("the panic was swallowed; it must reach the recover link")
		}
		if len(rec.intents) != 1 {
			t.Errorf("intents = %d, want the intent to stand", len(rec.intents))
		}
		if len(rec.results) != 0 {
			t.Errorf("outcomes = %d, want none: the action never completed", len(rec.results))
		}
	}()

	serve(t, rec, "auth.login", http.MethodPost, `{"username":"holz"}`, func(http.ResponseWriter, *http.Request) {
		panic("the handler died mid-action")
	})
}

// TestAuditIgnoresReads keeps the log about mutations. A GET that changes
// nothing has nothing to repudiate.
func TestAuditIgnoresReads(t *testing.T) {
	rec := &recorder{}
	serve(t, rec, "audit.list", http.MethodGet, "", ok)

	if len(rec.intents) != 0 || len(rec.results) != 0 {
		t.Errorf("a GET produced %d intents and %d outcomes, want none",
			len(rec.intents), len(rec.results))
	}
}

// TestAuditRedactsThePassword is D-14 reaching the wire: the body is captured,
// and the credential in it is not.
func TestAuditRedactsThePassword(t *testing.T) {
	rec := &recorder{}
	serve(t, rec, "auth.login", http.MethodPost,
		`{"username":"holz","password":"correct-horse-battery-staple"}`, ok)

	params := rec.intents[0].params
	if params["username"] != "holz" {
		t.Errorf("username = %v, want it captured", params["username"])
	}
	if params["password"] != audit.RedactedMarker {
		t.Errorf("password = %v, want %q", params["password"], audit.RedactedMarker)
	}
}

// TestAuditRedactsAFieldNobodyListed uses an invented field: the middleware
// must not pass it through just because no rule mentions it.
func TestAuditRedactsAFieldNobodyListed(t *testing.T) {
	rec := &recorder{}
	serve(t, rec, "auth.login", http.MethodPost,
		`{"username":"holz","wibblefrotz":"the-secret-value"}`, ok)

	params := rec.intents[0].params
	if params["wibblefrotz"] != audit.RedactedMarker {
		t.Errorf("wibblefrotz = %v, want %q", params["wibblefrotz"], audit.RedactedMarker)
	}
	for _, v := range params {
		if s, isStr := v.(string); isStr && strings.Contains(s, "the-secret-value") {
			t.Fatalf("the unlisted value reached the record: %v", params)
		}
	}
}

// TestAuditLeavesTheBodyReadableForTheHandler: capturing the body must be
// invisible to everything downstream, or every handler breaks at once.
func TestAuditLeavesTheBodyReadableForTheHandler(t *testing.T) {
	rec := &recorder{}
	const body = `{"username":"holz","password":"s"}`
	var seen string

	serve(t, rec, "auth.login", http.MethodPost, body, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, len(body)+16)
		n, _ := r.Body.Read(buf)
		seen = string(buf[:n])
		w.WriteHeader(http.StatusNoContent)
	})

	if seen != body {
		t.Errorf("handler read %q, want the original body %q", seen, body)
	}
}

// TestAuditRefusesWhenTheIntentCannotBeRecorded is the fail-closed rule from
// the contract: a mutation that cannot be logged is not performed. An unlogged
// mutation is the outcome the whole subsystem exists to prevent.
func TestAuditRefusesWhenTheIntentCannotBeRecorded(t *testing.T) {
	rec := &recorder{failWith: errors.New("disk is full")}
	var reached bool

	w := serve(t, rec, "auth.login", http.MethodPost, `{"username":"holz"}`, func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	})

	if reached {
		t.Error("the handler ran even though the intent was never durable")
	}
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// TestAuditCarriesTheRequestID ties a record to the line in the server log and
// to the instance a user can quote from a problem response.
func TestAuditCarriesTheRequestID(t *testing.T) {
	rec := &recorder{}
	serve(t, rec, "auth.login", http.MethodPost, `{"username":"holz"}`, ok)

	id, present := rec.intents[0].params[RequestIDParam]
	if !present {
		t.Fatalf("params carry no %s: %v", RequestIDParam, rec.intents[0].params)
	}
	if s, isStr := id.(string); !isStr || s == "" {
		t.Errorf("%s = %v, want a non-empty id", RequestIDParam, id)
	}
}

// TestAuditSkipsARouteWithNoAction is the defect the contract calls out: a
// mutating route without an action token executes unlogged. This pins the
// behaviour so it stays a visible, testable hole rather than a surprise.
func TestAuditSkipsARouteWithNoAction(t *testing.T) {
	rec := &recorder{}
	serve(t, rec, "", http.MethodPost, `{}`, ok)

	if len(rec.intents) != 0 {
		t.Errorf("intents = %d for a route with no action", len(rec.intents))
	}
}

// TestAuditHandlesAnUnparsableBody: a body that is not a JSON object still
// produces a record, and still produces no clear text.
func TestAuditHandlesAnUnparsableBody(t *testing.T) {
	rec := &recorder{}
	serve(t, rec, "auth.login", http.MethodPost, `not json at all, password=hunter2`, ok)

	if len(rec.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(rec.intents))
	}
	params := rec.intents[0].params
	if params[BodyParam] != audit.RedactedMarker {
		t.Errorf("params[%s] = %v, want %q", BodyParam, params[BodyParam], audit.RedactedMarker)
	}
	for _, v := range params {
		if s, isStr := v.(string); isStr && strings.Contains(s, "hunter2") {
			t.Fatalf("an unparsable body leaked into the record: %v", params)
		}
	}
}
