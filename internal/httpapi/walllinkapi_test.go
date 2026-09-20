package httpapi_test

// What a wall link can reach, measured through the real chain (2026-09-20).
//
// The unit tests in internal/auth cover minting, revoking and the comparison.
// What they cannot cover is the thing that matters once the URL is on a
// television: that the credential opens ONE route and is simply not a credential
// anywhere else.
//
// The route-table guard in cmd/ says one route carries the flag. This says the
// flag is what the middleware chain actually honours -- the two halves of the
// same claim, and neither is worth much alone: a flag nothing reads, or a chain
// reading a flag nobody set.

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func mintWallLink(t *testing.T, h *harness) string {
	t.Helper()

	// Creating one is Destructive, so the window has to be open: handing out a
	// credential that will live on a screen for months is exactly what D-06 is
	// for.
	if resp, raw := h.do(t, http.MethodPost, "/api/v1/auth/sudo",
		map[string]string{"password": testPass}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
	}

	resp, raw := h.do(t, http.MethodPost, "/api/v1/wall-links",
		map[string]string{"label": "The screen in the IT office"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("creating a wall link: %d (%s)", resp.StatusCode, raw)
	}

	var body struct {
		Token  string `json:"token"`
		Notice string `json:"notice"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Token == "" {
		t.Fatal("no token came back, so the link could never be put on a screen")
	}
	if body.Notice == "" {
		t.Error("nothing said this is the only time the link is shown")
	}
	return body.Token
}

// asTheScreen makes a request the way a wall does: the token, and NO session.
//
// A client of its own with no cookie jar, and that is the whole point of the
// helper. The first version of this deleted the Cookie header in a request
// option, which measured nothing at all: net/http's Jar adds the cookie during
// Do, after every option has run. Every route answered 200 as the signed-in
// administrator and the test reported the credential as correctly scoped.
func asTheScreen(t *testing.T, h *harness, method, path, token string, body any) (int, string) {
	t.Helper()

	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rdr = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, h.srv.URL+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Holzkube-Manager-CSRF", "1")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{
		// No Jar. See above.
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test server uses an ephemeral cert
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return resp.StatusCode, string(raw)
}

// refused reports whether the gate stopped a request. The harness has no
// inventory, so an ALLOWED request answers 502 -- which is a route saying "I got
// past the gate and had nothing to ask". 401 and 403 are the gate.
func refused(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}

// TestAWallLinkOpensTheWallAndNothingElse.
func TestAWallLinkOpensTheWallAndNothingElse(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)
	token := mintWallLink(t, h)

	// The wall itself. This harness has no inventory, so the route answers a
	// 502 or a 404 for the cluster -- which is the route saying "I got past the
	// gate and had nothing to ask". A 401 or a 403 would mean the gate stopped
	// it, and that is the whole of what this asserts.
	status, raw := asTheScreen(t, h, http.MethodGet, "/api/v1/clusters/c-1/wall", token, nil)
	if refused(status) {
		t.Errorf("the wall refused its own link: %d (%s)", status, raw)
	}

	// Everything else. These are the routes somebody would reach for if the
	// link were a reader session, and every one of them has to refuse it.
	for _, path := range []string{
		"/api/v1/audit",
		"/api/v1/machines",
		"/api/v1/clusters",
		"/api/v1/users",
		"/api/v1/wall-links",
		"/api/v1/auth/me",
		"/api/v1/clusters/c-1/kubernetes",
		"/api/v1/clusters/c-1/kubernetes/resources",
		"/api/v1/clusters/c-1/kubernetes/access",
	} {
		status, raw := asTheScreen(t, h, http.MethodGet, path, token, nil)
		if !refused(status) {
			t.Errorf("%s answered %d to a wall link (%s).\n\n"+
				"This credential is a URL on a television. It opens the wall and must be no "+
				"credential at all anywhere else.", path, status, raw)
		}
	}
}

// TestAWallLinkChangesNothing.
//
// A GET-only credential is a claim the route table makes; this is it holding at
// the boundary, on the routes that would matter most if it did not.
func TestAWallLinkChangesNothing(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)
	token := mintWallLink(t, h)

	for _, call := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/v1/wall-links", map[string]string{"label": "another"}},
		{http.MethodDelete, "/api/v1/machines/anything", nil},
		{http.MethodPost, "/api/v1/auth/sudo", map[string]string{"password": testPass}},
		{http.MethodPost, "/api/v1/users", map[string]string{
			"username": "smuggled", "password": newAccountPass, "role": "admin",
		}},
	} {
		status, raw := asTheScreen(t, h, call.method, call.path, token, call.body)
		if !refused(status) {
			t.Errorf("%s %s answered %d to a wall link (%s): this credential must never change "+
				"anything", call.method, call.path, status, raw)
		}
	}
}

// TestARevokedLinkStopsWorkingAtTheBoundary.
func TestARevokedLinkStopsWorkingAtTheBoundary(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)
	token := mintWallLink(t, h)

	var listed struct {
		Links []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		} `json:"links"`
	}
	_, listedRaw := h.do(t, http.MethodGet, "/api/v1/wall-links", nil)
	if err := json.Unmarshal(listedRaw, &listed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(listed.Links) != 1 {
		t.Fatalf("listed %+v, want the one that was made", listed.Links)
	}
	// The label is what somebody reads when deciding which to revoke.
	if listed.Links[0].Label != "The screen in the IT office" {
		t.Errorf("label = %q, want the one it was given", listed.Links[0].Label)
	}

	// An empty object as the body: the CSRF contract wants a Content-Type on
	// every mutating request, and the handler reads no body.
	if resp, raw := h.do(t, http.MethodDelete,
		"/api/v1/wall-links/"+listed.Links[0].ID, map[string]any{}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke: %d (%s)", resp.StatusCode, raw)
	}

	status, raw := asTheScreen(t, h, http.MethodGet, "/api/v1/clusters/c-1/wall", token, nil)
	if !refused(status) {
		t.Errorf("a revoked link still opens the wall: %d (%s).\n\n"+
			"Revocation is the only way a screen is turned off from across the building, and a "+
			"credential that outlives it is one nobody can take back.", status, raw)
	}
}

// TestTheListNeverHandsBackTheLink.
func TestTheListNeverHandsBackTheLink(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)
	token := mintWallLink(t, h)

	_, raw := h.do(t, http.MethodGet, "/api/v1/wall-links", nil)
	if string(raw) == "" {
		t.Fatal("no answer")
	}
	// Neither the token nor its hash: only the hash is stored, and a route that
	// returned either would make "shown once" untrue.
	if contains(string(raw), token) {
		t.Error("the list hands back the link itself")
	}
	if contains(string(raw), "token_hash") {
		t.Error("the list hands back the stored hash")
	}
}

// TestASessionCannotBorrowAWallLinkToSkipItsRole.
//
// The one hole the other tests here cannot see. They send a token and no
// session, so Authn refuses first and Authz is never reached -- which means a
// role gate widened to "anything carrying a bearer" stays GREEN in all of them.
// That was measured: the injection passed.
//
// The shape it hides is real. A reader has a session, so Authn lets them in;
// if Authz then waved through anything with an Authorization header, a reader
// could add one and reach an administrator's routes. So this signs in as a
// reader AND sends a token, which is the combination the other tests never make.
func TestASessionCannotBorrowAWallLinkToSkipItsRole(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)
	token := mintWallLink(t, h)

	if resp, raw := h.do(t, http.MethodPost, "/api/v1/users", map[string]string{
		"username": "reader-account", "password": newAccountPass, "role": "reader",
	}); resp.StatusCode != http.StatusCreated {
		t.Fatalf("creating the reader: %d (%s)", resp.StatusCode, raw)
	}

	reader := h.asUser(t, "reader-account", newAccountPass)

	// A route only an administrator may read. With a valid session the request
	// is authenticated, so the role gate is the only thing standing between
	// this reader and it.
	for _, carry := range []struct {
		what  string
		token string
	}{
		{"no token at all", ""},
		{"a valid wall link", token},
	} {
		status, raw := reader.status(t, http.MethodGet, "/api/v1/users", nil,
			func(r *http.Request) {
				if carry.token != "" {
					r.Header.Set("Authorization", "Bearer "+carry.token)
				}
			})
		if status != http.StatusForbidden {
			t.Errorf("a reader carrying %s read /api/v1/users: %d (%s).\n\n"+
				"A wall link opens the wall. It must not lift the role gate on anything else, "+
				"least of all for somebody who already has a session.", carry.what, status, raw)
		}
	}
}

func contains(haystack, needle string) bool {
	return needle != "" && len(haystack) >= len(needle) &&
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}()
}
