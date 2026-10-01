package httpapi_test

// Unlinking an account's single sign-on, over the real route graph.
//
// The binding is planted through the store rather than made by a sign-in,
// because the harness has no identity provider to sign in through -- and what
// is under test is what the route does to a binding, not how one came to be.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The values a binding is planted with. Documentation names only: the
// repository is public.
const (
	plantedIssuer  = "https://idp.example.com/application/o/holzkube-manager/"
	plantedSubject = "subject-0f3a9c"
)

// plantBinding links an account to plantedIssuer and plantedSubject directly in
// the store.
func (h *harness) plantBinding(t *testing.T, id model.UserID) {
	t.Helper()
	u, err := h.store.Users().Get(context.Background(), id)
	if err != nil {
		t.Fatalf("get account %s: %v", id, err)
	}
	u.Issuer = plantedIssuer
	u.Subject = plantedSubject
	if _, err := h.store.Users().Put(context.Background(), u); err != nil {
		t.Fatalf("plant binding on %s: %v", id, err)
	}
}

// openSudo opens the re-authentication window for the harness's own session.
func (h *harness) openSudo(t *testing.T) {
	t.Helper()
	if resp, raw := h.do(t, http.MethodPost, "/api/v1/auth/sudo",
		map[string]string{"password": testPass}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
	}
}

// unlink sends the DELETE with an empty object as its body: the CSRF contract
// wants a Content-Type on every mutating request, and the handler reads no body.
func (h *harness) unlink(t *testing.T, id string) (*http.Response, []byte) {
	t.Helper()
	return h.do(t, http.MethodDelete, "/api/v1/users/"+id+"/identity", map[string]any{})
}

// listedUser is the part of the account view these tests read.
type listedUser struct {
	ID             string `json:"id"`
	Username       string `json:"username"`
	Kind           string `json:"kind"`
	LinkedIdentity bool   `json:"linked_identity"`
	LinkedProvider string `json:"linked_provider"`
	Self           bool   `json:"self"`
}

// listUsers answers GET /api/v1/users, decoded, and the raw body beside it.
func (h *harness) listUsers(t *testing.T) ([]listedUser, []byte) {
	t.Helper()
	resp, raw := h.do(t, http.MethodGet, "/api/v1/users", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list users: %d (%s)", resp.StatusCode, raw)
	}
	var body struct {
		Users []listedUser `json:"users"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode users: %v (%s)", err, raw)
	}
	return body.Users, raw
}

// self is the signed-in admin's own row.
func (h *harness) self(t *testing.T) listedUser {
	t.Helper()
	users, _ := h.listUsers(t)
	for _, u := range users {
		if u.Self {
			return u
		}
	}
	t.Fatalf("no account in the list is marked self")
	return listedUser{}
}

func TestUnlinkSingleSignOnClearsTheBinding(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)
	me := h.self(t)
	h.plantBinding(t, model.UserID(me.ID))
	h.openSudo(t)

	users, raw := h.listUsers(t)
	var listed listedUser
	for _, u := range users {
		if u.ID == me.ID {
			listed = u
		}
	}
	if !listed.LinkedIdentity {
		t.Errorf("linked_identity = false for an account with a binding")
	}
	if listed.LinkedProvider != "idp.example.com" {
		t.Errorf("linked_provider = %q, want the issuer's host idp.example.com", listed.LinkedProvider)
	}
	// The subject is somebody's identifier at a third party, and the issuer's
	// path says nothing an admin needs: neither is anywhere in the answer.
	if strings.Contains(string(raw), plantedSubject) {
		t.Errorf("the account list carries the subject: %s", raw)
	}
	if strings.Contains(string(raw), "/application/o/") {
		t.Errorf("the account list carries the issuer's path: %s", raw)
	}

	resp, raw := h.unlink(t, me.ID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unlink: %d (%s)", resp.StatusCode, raw)
	}
	var view listedUser
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatalf("decode unlink answer: %v (%s)", err, raw)
	}
	if view.LinkedIdentity || view.LinkedProvider != "" {
		t.Errorf("the answer still says linked: %s", raw)
	}

	stored, err := h.store.Users().Get(context.Background(), model.UserID(me.ID))
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if stored.Issuer != "" || stored.Subject != "" {
		t.Errorf("the store still holds a binding: issuer %q, subject %q", stored.Issuer, stored.Subject)
	}
}
