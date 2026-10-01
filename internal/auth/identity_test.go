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
