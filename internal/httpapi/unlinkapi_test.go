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

	"github.com/holzcloud/holzkube-manager/internal/audit"
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

func TestUnlinkSingleSignOnNeedsAnAdmin(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)
	h.openSudo(t)
	if resp, raw := h.do(t, http.MethodPost, "/api/v1/users", map[string]string{
		"username": "operator-account", "password": newAccountPass, "role": string(model.RoleOperator),
	}); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create operator: %d (%s)", resp.StatusCode, raw)
	}

	me := h.self(t)
	h.plantBinding(t, model.UserID(me.ID))

	op := h.asUser(t, "operator-account", newAccountPass)
	// The operator's own window is open, so the refusal below is the role and
	// not the sudo gate standing in for it.
	if got, raw := op.status(t, http.MethodPost, "/api/v1/auth/sudo",
		map[string]string{"password": newAccountPass}); got != http.StatusNoContent {
		t.Fatalf("operator sudo: %d (%s)", got, raw)
	}

	got, raw := op.status(t, http.MethodDelete, "/api/v1/users/"+me.ID+"/identity", map[string]any{})
	if got != http.StatusForbidden || !strings.Contains(string(raw), "forbidden.role") {
		t.Fatalf("an operator unlinking the admin: %d (%s), want 403 forbidden.role", got, raw)
	}
	stored, err := h.store.Users().Get(context.Background(), model.UserID(me.ID))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !stored.HasIdentityBinding() {
		t.Errorf("a refused unlink removed the binding")
	}
}

func TestUnlinkSingleSignOnNeedsTheSudoWindow(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)
	me := h.self(t)
	h.plantBinding(t, model.UserID(me.ID))

	resp, raw := h.unlink(t, me.ID)
	if resp.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("unlink with no sudo window: %d (%s), want 428", resp.StatusCode, raw)
	}
	if p := decodeProblem(t, resp, raw); p.Code != "sudo.required" {
		t.Errorf("code = %q, want sudo.required", p.Code)
	}
	stored, err := h.store.Users().Get(context.Background(), model.UserID(me.ID))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !stored.HasIdentityBinding() {
		t.Errorf("a refused unlink removed the binding")
	}
}

// A service account never signs in through the provider, but an earlier
// release could bind one -- first-use linking used to count every account. The
// unlink is how that binding goes.
func TestUnlinkSingleSignOnClearsAServiceAccountsBinding(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)
	h.openSudo(t)

	resp, raw := h.do(t, http.MethodPost, "/api/v1/service-accounts", map[string]string{
		"username": "ci-bot", "role": string(model.RoleReader),
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create service account: %d (%s)", resp.StatusCode, raw)
	}
	var created struct {
		Account listedUser `json:"account"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatalf("decode: %v (%s)", err, raw)
	}
	h.plantBinding(t, model.UserID(created.Account.ID))

	// The list shows it, so that an admin can see there is something to remove.
	users, _ := h.listUsers(t)
	for _, u := range users {
		if u.ID == created.Account.ID && (!u.LinkedIdentity || u.LinkedProvider != "idp.example.com") {
			t.Errorf("the list hides the service account's binding: %+v", u)
		}
	}

	resp, raw = h.unlink(t, created.Account.ID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unlink a service account's binding: %d (%s), want 200", resp.StatusCode, raw)
	}
	stored, err := h.store.Users().Get(context.Background(), model.UserID(created.Account.ID))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.Issuer != "" || stored.Subject != "" {
		t.Errorf("the service account still holds a binding: issuer %q, subject %q",
			stored.Issuer, stored.Subject)
	}
}

func TestUnlinkSingleSignOnRefusesAnUnlinkedAccount(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)
	h.openSudo(t)
	me := h.self(t)

	resp, raw := h.unlink(t, me.ID)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("unlink an unlinked account: %d (%s), want 409", resp.StatusCode, raw)
	}
	if p := decodeProblem(t, resp, raw); p.Code != "conflict.not-linked" {
		t.Errorf("code = %q, want conflict.not-linked", p.Code)
	}

	resp, raw = h.unlink(t, "no-such-account")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unlink an unknown account: %d (%s), want 404", resp.StatusCode, raw)
	}
	if p := decodeProblem(t, resp, raw); p.Code != "notfound.user" {
		t.Errorf("code = %q, want notfound.user", p.Code)
	}
}

// An admin unlinking their own account stays signed in: the session belongs to
// the account, and the account -- its ID, role and password -- is unchanged.
func TestUnlinkSingleSignOnOfYourOwnAccountKeepsTheSession(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)
	me := h.self(t)
	h.plantBinding(t, model.UserID(me.ID))
	h.openSudo(t)

	if resp, raw := h.unlink(t, me.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("unlink: %d (%s)", resp.StatusCode, raw)
	}

	resp, raw := h.do(t, http.MethodGet, "/api/v1/auth/me", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the session that unlinked its own account was ended: GET /auth/me = %d (%s)",
			resp.StatusCode, raw)
	}
	var who struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(raw, &who); err != nil {
		t.Fatalf("decode me: %v (%s)", err, raw)
	}
	if who.Username != testUser {
		t.Errorf("signed in as %q after the unlink, want %q", who.Username, testUser)
	}
}

func TestUnlinkSingleSignOnAuditRecordCarriesNoIdentity(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)
	me := h.self(t)
	h.plantBinding(t, model.UserID(me.ID))
	h.openSudo(t)

	// A client that sends the identity anyway: the route reads no body, and the
	// archive must not keep what it was handed either.
	resp, raw := h.do(t, http.MethodDelete, "/api/v1/users/"+me.ID+"/identity", map[string]string{
		"issuer": plantedIssuer, "subject": plantedSubject,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unlink: %d (%s)", resp.StatusCode, raw)
	}

	page, err := h.logger.Query(context.Background(), audit.Filter{Action: "user.identity-unlink"})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	var attempt, outcome bool
	for _, rec := range page.Items {
		if rec.Actor != testUser {
			t.Errorf("record actor = %q, want %q", rec.Actor, testUser)
		}
		switch rec.Outcome {
		case audit.OutcomeAttempt:
			attempt = true
		case audit.OutcomeSuccess:
			outcome = true
		}
		encoded, err := json.Marshal(rec)
		if err != nil {
			t.Fatalf("marshal record: %v", err)
		}
		for _, secret := range []string{plantedIssuer, plantedSubject} {
			if strings.Contains(string(encoded), secret) {
				t.Errorf("an audit record carries %q, which nothing ever removes: %s", secret, encoded)
			}
		}
	}
	if !attempt || !outcome {
		t.Fatalf("no record of the unlink: attempt %v, outcome %v (%d records)", attempt, outcome, len(page.Items))
	}
}
