package handlers

// The rules the OIDC handlers apply, tested as functions.
//
// First-use binding is decided before anything reaches the store, and the end
// to end harness cannot complete a callback against a made-up provider -- so
// the decision is asked of bindFirstIdentity directly.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/auth"
	"github.com/holzcloud/holzkube-manager/internal/auth/oidc"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
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
