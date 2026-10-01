package handlers

// The rules the OIDC handlers apply, tested as functions.
//
// First-use binding is decided before anything reaches the store, and the end
// to end harness cannot complete a callback against a made-up provider -- so
// the decision is asked of bindFirstIdentity directly.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/auth"
	"github.com/holzcloud/holzkube-manager/internal/auth/oidc"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
)

// Documentation values only: the repository is public.
const (
	rulesIssuer      = "https://idp.example.com/application/o/holzkube-manager/"
	rulesOtherIssuer = "https://auth.example.com/application/o/holzkube-manager/"
)

// plantedHash stands in for a password hash. Its shape does not matter: what
// is asserted is only that a bind leaves it where it was.
const plantedHash = "$argon2id$v=19$m=65536,t=1,p=4$c2FsdA$aGFzaA"

func rulesDeps(t *testing.T) (httpapi.Deps, *fsstore.Store) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod data dir: %v", err)
	}
	st, err := fsstore.Open(dir)
	if err != nil {
		t.Fatalf("fsstore.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	au, err := auth.New(st, time.Hour)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	// No IsSSOOnly: every request is from the local network, which is where
	// binding is allowed at all.
	return httpapi.Deps{
		Store:  st,
		Auth:   au,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, st
}

func TestFirstUseBindCountsOnlyPeople(t *testing.T) {
	t.Parallel()

	type account struct {
		id   string
		kind model.UserKind
	}
	cases := []struct {
		name     string
		accounts []account
		wantErr  error
		wantBind string
	}{
		{
			name:     "one person and a service account binds the person",
			accounts: []account{{"u-person", model.KindPerson}, {"u-service", model.KindService}},
			wantBind: "u-person",
		},
		{
			name:     "two people bind nobody",
			accounts: []account{{"u-one", model.KindPerson}, {"u-two", model.KindPerson}},
			wantErr:  errBindAmbiguous,
		},
		{
			name:    "no account at all is setup",
			wantErr: errBindBeforeSetup,
		},
		{
			name:     "only a service account is setup too",
			accounts: []account{{"u-service", model.KindService}},
			wantErr:  errBindBeforeSetup,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, st := rulesDeps(t)
			ctx := context.Background()
			for _, a := range tc.accounts {
				if _, err := st.Users().Put(ctx, model.User{
					ID: model.UserID(a.id), Username: a.id, Role: model.RoleAdmin, Kind: a.kind,
					PasswordHash: plantedHash, CreatedAt: time.Now(),
				}); err != nil {
					t.Fatalf("put %s: %v", a.id, err)
				}
			}

			r := httptest.NewRequest("GET", "https://192.168.1.10:8443/api/v1/auth/oidc/callback", nil)
			bound, err := bindFirstIdentity(d, r, rulesIssuer, oidc.Identity{Subject: "subject-0f3a9c"})

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("bindFirstIdentity: %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("bindFirstIdentity: %v", err)
			}
			if string(bound.ID) != tc.wantBind {
				t.Errorf("bound %q, want %q", bound.ID, tc.wantBind)
			}
			for _, a := range tc.accounts {
				stored, err := st.Users().Get(ctx, model.UserID(a.id))
				if err != nil {
					t.Fatalf("get %s: %v", a.id, err)
				}
				if want := a.id == tc.wantBind; stored.HasIdentityBinding() != want {
					t.Errorf("%s carries a binding: %v, want %v", a.id, stored.HasIdentityBinding(), want)
				}
				// The account the person looked up is the one with its hash
				// stripped; binding must not write that copy back. A first
				// single sign-on that erased the password would remove the
				// break-glass way in at the moment the provider became one.
				if stored.PasswordHash != plantedHash {
					t.Errorf("%s lost its password hash to the bind: %q", a.id, stored.PasswordHash)
				}
			}
		})
	}
}

func TestSudoRefusalNamesAnUnlinkedAccount(t *testing.T) {
	t.Parallel()

	bound := model.User{Username: "somebody", Issuer: rulesIssuer, Subject: "subject-0f3a9c"}
	cases := []struct {
		name    string
		account model.User
		issuer  string
		subject string
		want    string
	}{
		{"an unlinked account", model.User{Username: "somebody"}, rulesIssuer, "subject-0f3a9c", "oidc.not-linked"},
		{"the same identity", bound, rulesIssuer, "subject-0f3a9c", ""},
		{"a different subject", bound, rulesIssuer, "subject-7d21e4", "oidc.other-identity"},
		{"a different issuer", bound, rulesOtherIssuer, "subject-0f3a9c", "oidc.other-identity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := sudoIdentityRefusal(tc.account, tc.issuer, tc.subject); got != tc.want {
				t.Errorf("sudoIdentityRefusal = %q, want %q", got, tc.want)
			}
		})
	}
}

// callbackDeps is rulesDeps with a provider -- built from a made-up issuer,
// which oidc.New never contacts -- and every host SSO-only when ssoOnly is set.
func callbackDeps(t *testing.T, ssoOnly bool) (httpapi.Deps, *fsstore.Store) {
	t.Helper()
	d, st := rulesDeps(t)
	p, err := oidc.New(rulesIssuer, "holzkube-manager", "s3cret")
	if err != nil {
		t.Fatalf("oidc.New: %v", err)
	}
	d.OIDC = p
	if ssoOnly {
		d.IsSSOOnly = func(string) bool { return true }
	}
	return d, st
}

// completeLoginAs runs the end of a callback for the given subject, as the
// session middleware would, and answers the redirect and the session cookie.
func completeLoginAs(t *testing.T, d httpapi.Deps, subject string) (*httptest.ResponseRecorder, []*http.Cookie) {
	t.Helper()
	h := d.Auth.Sessions().LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		completeLogin(d, w, r, oidc.Identity{Subject: subject, RawIDToken: "id-token"})
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "https://192.168.1.10:8443/api/v1/auth/oidc/callback", nil))
	return rec, rec.Result().Cookies()
}

// signedInAs answers which account the cookies' session is signed in as, and
// "" for none.
func signedInAs(t *testing.T, d httpapi.Deps, cookies []*http.Cookie) model.UserID {
	t.Helper()
	var who model.UserID
	h := d.Auth.Sessions().LoadAndSave(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if u, ok := d.Auth.CurrentUser(r.Context()); ok {
			who = u.ID
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "https://192.168.1.10:8443/api/v1/auth/me", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	return who
}

// A binding on a service account -- one an earlier release made -- never
// signs anybody in as that service account. On an SSO-only address the
// identity is then simply not linked: bind-host, and no session.
func TestCallbackNeverSignsInAsAServiceAccount(t *testing.T) {
	t.Parallel()
	d, st := callbackDeps(t, true)
	ctx := context.Background()

	if _, err := st.Users().Put(ctx, model.User{
		ID: "u-person", Username: "somebody", Role: model.RoleAdmin, Kind: model.KindPerson,
		PasswordHash: plantedHash, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("put person: %v", err)
	}
	if _, err := st.Users().Put(ctx, model.User{
		ID: "u-break-glass", Username: "break-glass", Role: model.RoleAdmin, Kind: model.KindService,
		Issuer: rulesIssuer, Subject: "subject-0f3a9c", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("put service account: %v", err)
	}

	rec, cookies := completeLoginAs(t, d, "subject-0f3a9c")
	if loc := rec.Header().Get("Location"); loc != "/login?sso_error=bind-host" {
		t.Errorf("redirect = %d %q, want /login?sso_error=bind-host", rec.Code, loc)
	}
	if who := signedInAs(t, d, cookies); who != "" {
		t.Fatalf("the callback signed in as %q through a service account's binding", who)
	}
}

// conflictingStore makes every user write lose a revision race: before each
// one, a concurrent writer changes the account's role, so the write that
// follows carries a stale revision and the store answers ErrConflict.
type conflictingStore struct{ *fsstore.Store }

func (c conflictingStore) Users() store.UserStore { return conflictingUsers{c.Store.Users()} }

type conflictingUsers struct{ store.UserStore }

func (c conflictingUsers) Put(ctx context.Context, rec model.User) (model.User, error) {
	current, err := c.UserStore.Get(ctx, rec.ID)
	if err == nil {
		current.Role = model.RoleOperator
		if _, err := c.UserStore.Put(ctx, current); err != nil {
			return model.User{}, err
		}
	}
	return c.UserStore.Put(ctx, rec)
}

// A first-use bind whose account keeps changing under it ends on the sign-in
// page with a code the page explains -- not as a problem document in the
// address bar, which is what a 500 on this navigation renders as.
func TestABindThatKeepsLosingARaceSaysSo(t *testing.T) {
	t.Parallel()
	d, st := callbackDeps(t, false)
	if _, err := st.Users().Put(context.Background(), model.User{
		ID: "u-person", Username: "somebody", Role: model.RoleAdmin, Kind: model.KindPerson,
		PasswordHash: plantedHash, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("put person: %v", err)
	}
	au, err := auth.New(conflictingStore{st}, time.Hour)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	d.Auth = au

	rec, cookies := completeLoginAs(t, d, "subject-0f3a9c")
	if loc := rec.Header().Get("Location"); loc != "/login?sso_error=account-changed" {
		t.Errorf("answer = %d %q, want a redirect to /login?sso_error=account-changed", rec.Code, loc)
	}
	if who := signedInAs(t, d, cookies); who != "" {
		t.Errorf("signed in as %q although nothing was linked", who)
	}
}

// An account change that loses a revision race -- an unlink against a role
// change, say -- is a named conflict, not an internal error.
func TestAnAccountChangeLosingARaceIsAConflict(t *testing.T) {
	t.Parallel()
	d, _ := rulesDeps(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "https://192.168.1.10:8443/api/v1/users/u-person/identity", nil)
	writeUserError(rec, req, d, fmt.Errorf("auth: store unlinked account: %w", store.ErrConflict))

	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"store.conflict"`) {
		t.Errorf("answer = %d %s, want 409 store.conflict", rec.Code, rec.Body.String())
	}
}

// An admin who signed in through the provider and unlinks their own account
// ends the session they did it from: it came in through the link that is now
// gone. The unlink still answers with the account, marked as theirs, and the
// session record is destroyed rather than left to answer 401 later.
func TestUnlinkingYourOwnLinkEndsTheSessionThatCameThroughIt(t *testing.T) {
	t.Parallel()
	d, st := callbackDeps(t, false)
	ctx := context.Background()
	if _, err := st.Users().Put(ctx, model.User{
		ID: "u-person", Username: "somebody", Role: model.RoleAdmin, Kind: model.KindPerson,
		PasswordHash: plantedHash, Issuer: rulesIssuer, Subject: "subject-0f3a9c", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("put person: %v", err)
	}

	_, cookies := completeLoginAs(t, d, "subject-0f3a9c")
	if who := signedInAs(t, d, cookies); who != "u-person" {
		t.Fatalf("the provider sign-in is signed in as %q, want u-person", who)
	}

	h := d.Auth.Sessions().LoadAndSave(unlinkUserIdentity(d))
	req := httptest.NewRequest(http.MethodDelete, "https://192.168.1.10:8443/api/v1/users/u-person/identity", nil)
	req.SetPathValue("id", "u-person")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unlink: %d %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, `"self":true`) || !strings.Contains(body, `"linked_identity":false`) {
		t.Errorf("the answer does not describe the caller's own, now unlinked account: %s", body)
	}
	if who := signedInAs(t, d, cookies); who != "" {
		t.Errorf("the session that came in through the removed link is still signed in as %q", who)
	}
	sessions, err := st.Sessions().List(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("%d session records survive; the ended one was not destroyed", len(sessions))
	}
}
