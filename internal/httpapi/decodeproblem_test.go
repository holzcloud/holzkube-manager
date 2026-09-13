package httpapi_test

// A bad request body is a contracted response, not a place to paste Go's
// internal vocabulary.
//
// Eighteen routes answered with the decoder's own error text, which is how a
// sentence like "json: cannot unmarshal string into Go struct field
// schematicInput.meta.key of type uint8" came to be a documented API response.
// It names types and struct fields that are not in any contract, it is not
// actionable by anybody outside this repository, and it puts the field name in
// prose when the problem document has a member for exactly that.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (h *harness) badBody(t *testing.T, path, raw string) (int, problemBody) {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, h.srv.URL+path, bytes.NewReader([]byte(raw)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Holzkube-Manager-CSRF", "1")

	resp, err := h.client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var p problemBody
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("the response is not a problem document: %v (%s)", err, body)
	}
	return resp.StatusCode, p
}

// goVocabulary is what must never reach a client.
//
// "json:" catches the decoder's own prefix, and the rest are the Go type names
// that appeared in the messages this replaced. A test that only asserted the
// new wording would still pass if a route were added that echoed err.Error().
var goVocabulary = []string{"json:", "Go struct field", "uint8", "int64", "unmarshal"}

func TestABadlyTypedFieldIsNamedRatherThanDescribedInGo(t *testing.T) {
	t.Parallel()

	h := newInventoryHarness(t)

	status, p := h.badBody(t, "/api/v1/clusters/fingerprint", `{"endpoint": 42}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if p.Code != "validation.failed" {
		t.Errorf("code = %q, want validation.failed", p.Code)
	}

	for _, word := range goVocabulary {
		if strings.Contains(p.Detail, word) {
			t.Errorf("the detail contains %q, which is Go's vocabulary and not this API's: %q",
				word, p.Detail)
		}
	}

	if len(p.Errors) != 1 {
		t.Fatalf("errors = %+v, want the one field that was wrong; the problem document has a "+
			"member for this and prose is not it", p.Errors)
	}
	if p.Errors[0].Field != "endpoint" {
		t.Errorf("the wrong field is reported as %q, want endpoint", p.Errors[0].Field)
	}
	if !strings.Contains(p.Errors[0].Reason, "string") {
		t.Errorf("reason = %q, want it to say what the field should have been", p.Errors[0].Reason)
	}
}

func TestAnUnknownFieldIsNamedAsAField(t *testing.T) {
	t.Parallel()

	h := newInventoryHarness(t)

	status, p := h.badBody(t, "/api/v1/clusters/fingerprint", `{"endpoint":"x","endpiont":"y"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	for _, word := range goVocabulary {
		if strings.Contains(p.Detail, word) {
			t.Errorf("the detail contains %q: %q", word, p.Detail)
		}
	}
	if len(p.Errors) != 1 || p.Errors[0].Field != "endpiont" {
		t.Fatalf("errors = %+v, want the misspelled field named; naming it is the whole value of "+
			"refusing unknown fields rather than ignoring them", p.Errors)
	}
}

func TestAMalformedBodyDoesNotQuoteTheDecoder(t *testing.T) {
	t.Parallel()

	h := newInventoryHarness(t)

	for name, raw := range map[string]string{
		"truncated": `{"endpoint":`,
		"not json":  `endpoint=x`,
		"empty":     ``,
	} {
		t.Run(name, func(t *testing.T) {
			status, p := h.badBody(t, "/api/v1/clusters/fingerprint", raw)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", status)
			}
			if p.Detail == "" {
				t.Error("the problem carries no detail, so the client is told a body was wrong and " +
					"nothing about how")
			}
			for _, word := range goVocabulary {
				if strings.Contains(p.Detail, word) {
					t.Errorf("the detail contains %q: %q", word, p.Detail)
				}
			}
		})
	}
}

// TestNoRouteEchoesTheDecodersOwnError is the guard, and the two tests above
// are the demonstration.
//
// Three routes cannot be covered by a request-shaped test: login answers a
// malformed body with the authentication problem on purpose, and setup and the
// password change each say their own sentence. A source scan covers all of
// them, and it covers the route somebody adds next -- which is how eighteen of
// these accumulated in the first place.
func TestNoRouteEchoesTheDecodersOwnError(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("handlers")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read the handlers directory: %v", err)
	}

	scanned := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // a directory this test named itself
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		scanned++

		lines := strings.Split(string(raw), "\n")
		for n, line := range lines {
			// Comments are skipped because decodeProblem's own doc quotes the
			// pattern it replaced, and a guard that cannot be explained in
			// prose is a guard somebody deletes the explanation of.
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if !strings.Contains(line, "Validation(err.Error())") {
				continue
			}
			// Only the decode sites. The same shape is right elsewhere:
			// internal/upgrade's ErrNoPath carries a sentence written for an
			// operator, and rewording it here would replace the only account of
			// the refusal with a vaguer one.
			if n == 0 || !strings.Contains(lines[n-1], "decodeJSON(") {
				continue
			}
			t.Errorf("%s:%d hands the client encoding/json's own error text. Use decodeProblem, "+
				"which says which field was wrong without naming a Go type.", e.Name(), n+1)
		}
	}
	if scanned == 0 {
		t.Fatal("no handler files were scanned, so this test proves nothing")
	}
}
