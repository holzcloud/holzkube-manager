package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// Documentation values only: the repository is public.
const (
	oldIssuer  = "https://idp.example.com/application/o/holzkube-manager/"
	oldSubject = "subject-0f3a9c"
	newIssuer  = "https://auth.example.com"
	newSubject = "subject-7d21e4"
)

// putAccount writes an account straight into the store, with no password
// hash: these tests are about bindings, and an argon2id hash per account would
// be time spent on nothing they look at.
func putAccount(t *testing.T, st store.Store, id, username string, kind model.UserKind, issuer, subject string) model.User {
	t.Helper()
	u, err := st.Users().Put(context.Background(), model.User{
		ID:        model.UserID(id),
		Username:  username,
		Role:      model.RoleAdmin,
		Kind:      kind,
		Issuer:    issuer,
		Subject:   subject,
		CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("put %s: %v", username, err)
	}
	return u
}

func TestUnlinkIdentityLetsTheNextSignInBindANewProvider(t *testing.T) {
	t.Parallel()
	svc, st := newTestService(t, time.Hour)
	ctx := context.Background()

	u := putAccount(t, st, "u-person", "somebody", model.KindPerson, oldIssuer, oldSubject)

	// The reason the operation exists: a bound account refuses a new provider.
	if _, err := svc.BindIdentity(ctx, u, newIssuer, newSubject); !errors.Is(err, ErrAlreadyBound) {
		t.Fatalf("binding a new provider before the unlink: %v, want ErrAlreadyBound", err)
	}

	unlinked, err := svc.UnlinkIdentity(ctx, u.ID)
	if err != nil {
		t.Fatalf("UnlinkIdentity: %v", err)
	}
	if unlinked.HasIdentityBinding() {
		t.Errorf("the returned account still carries a binding")
	}

	stored, err := st.Users().Get(ctx, u.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.Issuer != "" || stored.Subject != "" {
		t.Errorf("the store still holds half a binding: issuer %q, subject %q", stored.Issuer, stored.Subject)
	}
	if _, err := svc.FindByIdentity(ctx, oldIssuer, oldSubject); !errors.Is(err, ErrNoIdentityBinding) {
		t.Errorf("the old identity still resolves: %v, want ErrNoIdentityBinding", err)
	}

	if _, err := svc.BindIdentity(ctx, stored, newIssuer, newSubject); err != nil {
		t.Errorf("binding the new provider after the unlink: %v", err)
	}
	found, err := svc.FindByIdentity(ctx, newIssuer, newSubject)
	if err != nil || found.ID != u.ID {
		t.Errorf("the new identity resolves to %q (%v), want %q", found.ID, err, u.ID)
	}
}

// A binding on a service account is not hypothetical: the first-use rule before
// person accounts counted every account, so an instance whose only account was
// a service account bound the provider's identity to it. The unlink is how such
// a binding goes, so it clears it rather than refusing.
func TestUnlinkIdentityClearsAServiceAccountsBinding(t *testing.T) {
	t.Parallel()
	svc, st := newTestService(t, time.Hour)
	ctx := context.Background()

	u := putAccount(t, st, "u-service", "ci-bot", model.KindService, oldIssuer, oldSubject)

	unlinked, err := svc.UnlinkIdentity(ctx, u.ID)
	if err != nil {
		t.Fatalf("UnlinkIdentity on a service account with a binding: %v", err)
	}
	if unlinked.HasIdentityBinding() {
		t.Errorf("the returned account still carries a binding")
	}
	stored, err := st.Users().Get(ctx, u.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.Issuer != "" || stored.Subject != "" {
		t.Errorf("the service account still holds a binding: issuer %q, subject %q", stored.Issuer, stored.Subject)
	}
	if stored.Kind != model.KindService {
		t.Errorf("the unlink changed the account's kind to %q", stored.Kind)
	}
}

// A provider identity never resolves to a service account, whatever the store
// holds: a service account signs in with a token, and a browser session for
// one -- the break-glass account included -- would skip everything a token is
// checked for, its expiry first.
func TestFindByIdentityNeverAnswersAServiceAccount(t *testing.T) {
	t.Parallel()
	svc, st := newTestService(t, time.Hour)

	putAccount(t, st, "u-service", "ci-bot", model.KindService, oldIssuer, oldSubject)

	if u, err := svc.FindByIdentity(context.Background(), oldIssuer, oldSubject); !errors.Is(err, ErrNoIdentityBinding) {
		t.Fatalf("FindByIdentity resolved a service account's binding: %q (%v), want ErrNoIdentityBinding", u.ID, err)
	}
}

// BindIdentity guards the kind itself rather than trusting its caller to have
// asked SinglePersonAccount: the record it writes is the one it re-reads, and
// that is the one whose kind counts.
func TestBindIdentityRefusesAServiceAccount(t *testing.T) {
	t.Parallel()
	svc, st := newTestService(t, time.Hour)
	ctx := context.Background()

	u := putAccount(t, st, "u-service", "ci-bot", model.KindService, "", "")

	if _, err := svc.BindIdentity(ctx, u, newIssuer, newSubject); !errors.Is(err, ErrNotAPerson) {
		t.Fatalf("BindIdentity on a service account: %v, want ErrNotAPerson", err)
	}
	stored, err := st.Users().Get(ctx, u.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.HasIdentityBinding() {
		t.Errorf("a refused bind left a binding on the service account")
	}
}

// No session ever carries a service account. StartSession refuses one, and a
// session record that names one -- written by a release whose sign-in through
// the provider honoured a binding on a service account -- is not a session.
func TestASessionNeverCarriesAServiceAccount(t *testing.T) {
	t.Parallel()
	svc, st := newTestService(t, time.Hour)

	u := putAccount(t, st, "u-service", "ci-bot", model.KindService, oldIssuer, oldSubject)

	var startErr error
	token := runInSession(t, svc, func(ctx context.Context) {
		startErr = svc.StartSession(ctx, u)
	})
	if !errors.Is(startErr, ErrNotAPerson) {
		t.Errorf("StartSession for a service account: %v, want ErrNotAPerson", startErr)
	}
	if token != "" && authenticatedIn(t, svc, token) {
		t.Errorf("StartSession left a session signed in as a service account")
	}

	// The record an older release could have left behind.
	legacy := runInSession(t, svc, func(ctx context.Context) {
		if err := svc.sm.RenewToken(ctx); err != nil {
			t.Fatalf("renew: %v", err)
		}
		svc.markAuthenticated(ctx, string(u.ID))
	})
	if authenticatedIn(t, svc, legacy) {
		t.Errorf("a session naming a service account is treated as signed in")
	}
}

func TestUnlinkIdentityRefusesAnUnlinkedAccount(t *testing.T) {
	t.Parallel()
	svc, st := newTestService(t, time.Hour)

	u := putAccount(t, st, "u-person", "somebody", model.KindPerson, "", "")

	if _, err := svc.UnlinkIdentity(context.Background(), u.ID); !errors.Is(err, ErrNotLinked) {
		t.Fatalf("UnlinkIdentity on an unlinked account: %v, want ErrNotLinked", err)
	}
}

func TestSinglePersonAccountIgnoresServiceAccounts(t *testing.T) {
	t.Parallel()
	svc, st := newTestService(t, time.Hour)

	person := putAccount(t, st, "u-person", "somebody", model.KindPerson, "", "")
	putAccount(t, st, "u-service", "ci-bot", model.KindService, "", "")

	got, err := svc.SinglePersonAccount(context.Background())
	if err != nil {
		t.Fatalf("SinglePersonAccount with one person and a service account: %v", err)
	}
	if got.ID != person.ID {
		t.Errorf("SinglePersonAccount = %q, want the person %q", got.ID, person.ID)
	}
	if got.PasswordHash != "" {
		t.Errorf("SinglePersonAccount returned a password hash")
	}
}

func TestSinglePersonAccountRefusesSeveralPeople(t *testing.T) {
	t.Parallel()
	svc, st := newTestService(t, time.Hour)

	putAccount(t, st, "u-one", "first", model.KindPerson, "", "")
	// An account from before service accounts existed has no kind at all, and
	// is a person.
	putAccount(t, st, "u-two", "second", "", "", "")

	if _, err := svc.SinglePersonAccount(context.Background()); !errors.Is(err, ErrSeveralPersonAccounts) {
		t.Errorf("SinglePersonAccount with two people: %v, want ErrSeveralPersonAccounts", err)
	}
}

func TestSinglePersonAccountWithNobody(t *testing.T) {
	t.Parallel()
	svc, st := newTestService(t, time.Hour)

	if _, err := svc.SinglePersonAccount(context.Background()); !errors.Is(err, ErrNoPersonAccount) {
		t.Errorf("SinglePersonAccount with no account: %v, want ErrNoPersonAccount", err)
	}

	// Service accounts alone are still nobody an identity could belong to.
	putAccount(t, st, "u-service", "ci-bot", model.KindService, "", "")
	if _, err := svc.SinglePersonAccount(context.Background()); !errors.Is(err, ErrNoPersonAccount) {
		t.Errorf("SinglePersonAccount with only a service account: %v, want ErrNoPersonAccount", err)
	}
}

// racingStore makes the first `races` user writes lose a revision race: just
// before each one, it lets a concurrent writer change the stored record, so the
// write that follows carries a stale revision and the store answers
// ErrConflict -- the real conflict, not a stand-in for one.
type racingStore struct {
	store.Store
	races   int
	meddle  func(u *model.User)
	written int
}

func (r *racingStore) Users() store.UserStore { return &racingUsers{UserStore: r.Store.Users(), r: r} }

type racingUsers struct {
	store.UserStore
	r *racingStore
}

func (u *racingUsers) Put(ctx context.Context, rec model.User) (model.User, error) {
	if u.r.written < u.r.races {
		u.r.written++
		current, err := u.UserStore.Get(ctx, rec.ID)
		if err != nil {
			return model.User{}, err
		}
		u.r.meddle(&current)
		if _, err := u.UserStore.Put(ctx, current); err != nil {
			return model.User{}, err
		}
	}
	return u.UserStore.Put(ctx, rec)
}

func racingService(t *testing.T, races int, meddle func(u *model.User)) (*Service, store.Store) {
	t.Helper()
	_, st := newTestService(t, time.Hour)
	racing := &racingStore{Store: st, races: races, meddle: meddle}
	svc, err := New(racing, time.Hour)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	return svc, st
}

// Two first-use binds at once: one wins, and the other must not become a 500.
// Re-read once: the same identity is the success it would have been, a
// different one is ErrAlreadyBound, and an unrelated change (a role) is simply
// retried.
func TestBindIdentityLosingARevisionRace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		meddle  func(u *model.User)
		wantErr error
		wantSub string
	}{
		{"the same identity won", func(u *model.User) { u.Issuer, u.Subject = newIssuer, newSubject }, nil, newSubject},
		{"a different identity won", func(u *model.User) { u.Issuer, u.Subject = newIssuer, oldSubject }, ErrAlreadyBound, oldSubject},
		{"a role changed meanwhile", func(u *model.User) { u.Role = model.RoleOperator }, nil, newSubject},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, st := racingService(t, 1, tc.meddle)
			ctx := context.Background()
			u := putAccount(t, st, "u-person", "somebody", model.KindPerson, "", "")

			bound, err := svc.BindIdentity(ctx, u, newIssuer, newSubject)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("BindIdentity after losing the race: %v, want %v", err, tc.wantErr)
			}
			if err == nil && bound.Subject != newSubject {
				t.Errorf("returned subject %q, want %q", bound.Subject, newSubject)
			}
			stored, err := st.Users().Get(ctx, u.ID)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if stored.Subject != tc.wantSub {
				t.Errorf("stored subject %q, want %q", stored.Subject, tc.wantSub)
			}
		})
	}

	// A record that keeps changing under it is a conflict the caller can name,
	// not an internal error: store.ErrConflict, after one retry.
	t.Run("it keeps changing", func(t *testing.T) {
		t.Parallel()
		svc, st := racingService(t, 2, func(u *model.User) { u.Role = model.RoleOperator })
		u := putAccount(t, st, "u-person", "somebody", model.KindPerson, "", "")
		if _, err := svc.BindIdentity(context.Background(), u, newIssuer, newSubject); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("BindIdentity losing twice: %v, want store.ErrConflict", err)
		}
	})
}

// The bind answers the account the way every other read here does: without its
// password hash.
func TestBindIdentityReturnsNoPasswordHash(t *testing.T) {
	t.Parallel()
	svc, st := newTestService(t, time.Hour)
	ctx := context.Background()

	u, err := st.Users().Put(ctx, model.User{
		ID: "u-person", Username: "somebody", Role: model.RoleAdmin, Kind: model.KindPerson,
		PasswordHash: "$argon2id$v=19$m=65536,t=1,p=4$c2FsdA$aGFzaA", CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	bound, err := svc.BindIdentity(ctx, u, newIssuer, newSubject)
	if err != nil {
		t.Fatalf("BindIdentity: %v", err)
	}
	if bound.PasswordHash != "" {
		t.Errorf("the first bind returned the password hash")
	}
	again, err := svc.BindIdentity(ctx, u, newIssuer, newSubject)
	if err != nil {
		t.Fatalf("BindIdentity again: %v", err)
	}
	if again.PasswordHash != "" {
		t.Errorf("binding the same identity again returned the password hash")
	}
}
