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

func TestUnlinkIdentityRefusesAServiceAccount(t *testing.T) {
	t.Parallel()
	svc, st := newTestService(t, time.Hour)
	ctx := context.Background()

	// A binding on a service account can only have been planted: it signs in
	// with a token and never through the provider.
	u := putAccount(t, st, "u-service", "ci-bot", model.KindService, oldIssuer, oldSubject)

	if _, err := svc.UnlinkIdentity(ctx, u.ID); !errors.Is(err, ErrNotAPerson) {
		t.Fatalf("UnlinkIdentity on a service account: %v, want ErrNotAPerson", err)
	}
	stored, err := st.Users().Get(ctx, u.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.Issuer != oldIssuer || stored.Subject != oldSubject {
		t.Errorf("a refused unlink changed the binding: issuer %q, subject %q", stored.Issuer, stored.Subject)
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
