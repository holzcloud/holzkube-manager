package auth

// The lockout guard, which is the only part of account management that is hard.
//
// Everything else here is a store write. What matters is that no single
// legitimate-looking action can leave an instance with nobody able to manage
// it, because the repair for that is a shell on the host -- and somebody who
// reached this product through a browser may not have one.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

func usersFixture(t *testing.T) (*Service, store.Store) {
	t.Helper()
	return newTestService(t, time.Hour)
}

// seedAccount writes one account at a named role, with a hash nobody verifies.
func seedAccount(t *testing.T, st store.Store, id, username string, role model.UserRole) model.User {
	t.Helper()

	u, err := st.Users().Put(context.Background(), model.User{
		ID:           model.UserID(id),
		Username:     username,
		Role:         role,
		PasswordHash: "not-verified-by-these-tests",
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed %s: %v", username, err)
	}
	return u
}

func TestTheLastAdminCannotBeDeletedOrDemoted(t *testing.T) {
	t.Parallel()

	svc, st := usersFixture(t)
	ctx := t.Context()

	only := seedAccount(t, st, "a1", "only-admin", model.RoleAdmin)
	seedAccount(t, st, "o1", "an-operator", model.RoleOperator)
	seedAccount(t, st, "r1", "a-reader", model.RoleReader)

	if err := svc.DeleteUser(ctx, only.ID); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("deleting the only admin returned %v, want ErrLastAdmin -- two operators and a "+
			"reader are not somebody who can undo it", err)
	}

	// Demoted by somebody else, so this is the last-admin rule rather than the
	// self-demotion one.
	other := seedAccount(t, st, "a2", "another", model.RoleOperator)
	if _, err := svc.SetRole(ctx, other.ID, only.ID, model.RoleOperator); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("demoting the only admin returned %v, want ErrLastAdmin", err)
	}

	// And with a second admin it is allowed, which is what makes the refusals
	// above a rule rather than a blanket refusal.
	seedAccount(t, st, "a3", "second-admin", model.RoleAdmin)
	if err := svc.DeleteUser(ctx, only.ID); err != nil {
		t.Errorf("deleting an admin while another exists returned %v, want success", err)
	}
}

// TestAnAdminCannotDemoteItself is a separate rule from the one above, and the
// separation is the point: the remedy differs.
func TestAnAdminCannotDemoteItself(t *testing.T) {
	t.Parallel()

	svc, st := usersFixture(t)
	ctx := t.Context()

	me := seedAccount(t, st, "a1", "me", model.RoleAdmin)
	them := seedAccount(t, st, "a2", "them", model.RoleAdmin)

	// Not the last admin -- there are two -- so a refusal here can only be the
	// self-demotion rule.
	if _, err := svc.SetRole(ctx, me.ID, me.ID, model.RoleReader); !errors.Is(err, ErrSelfDemotion) {
		t.Errorf("demoting oneself returned %v, want ErrSelfDemotion", err)
	}

	// Another admin may do it. That is the remedy the error names.
	if _, err := svc.SetRole(ctx, them.ID, me.ID, model.RoleReader); err != nil {
		t.Errorf("another admin demoting this one returned %v, want success", err)
	}
}

// TestAnAccountMayDeleteItself is the case that must NOT be refused.
//
// Somebody leaving should not have to ask a colleague to remove them, and a
// rule that refused it would be a rule people work around by sharing an
// account.
func TestAnAccountMayDeleteItself(t *testing.T) {
	t.Parallel()

	svc, st := usersFixture(t)
	ctx := t.Context()

	seedAccount(t, st, "a1", "stays", model.RoleAdmin)
	leaving := seedAccount(t, st, "a2", "leaving", model.RoleAdmin)

	if err := svc.DeleteUser(ctx, leaving.ID); err != nil {
		t.Errorf("an admin deleting itself while another admin exists returned %v, want success", err)
	}
}

// TestAnAccountWrittenBeforeRolesExistedIsAnAdmin is the upgrade path.
//
// Every account on an installation that predates roles has an empty one.
// Reading that as the least privilege would lock the operator out of their own
// instance on the upgrade that introduced the feature.
func TestAnAccountWrittenBeforeRolesExistedIsAnAdmin(t *testing.T) {
	t.Parallel()

	svc, st := usersFixture(t)
	ctx := t.Context()

	old := seedAccount(t, st, "a1", "from-before", "")

	users, err := svc.Users(ctx)
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	if len(users) != 1 || users[0].Role != model.RoleAdmin {
		t.Fatalf("an account with no stored role is reported as %+v, want admin", users)
	}

	// And it counts as the last admin, so nothing can delete it.
	if err := svc.DeleteUser(ctx, old.ID); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("deleting the sole role-less account returned %v, want ErrLastAdmin", err)
	}
}

func TestAPasswordHashNeverLeavesThisPackage(t *testing.T) {
	t.Parallel()

	svc, st := usersFixture(t)
	ctx := t.Context()

	seedAccount(t, st, "a1", "somebody", model.RoleAdmin)

	users, err := svc.Users(ctx)
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	for _, u := range users {
		if u.PasswordHash != "" {
			t.Errorf("%s is reported with its password hash. A hash is not a password and is "+
				"also not nothing: it is the input to an offline attack", u.Username)
		}
	}

	created, err := svc.CreateUser(ctx, "a2", "another", "a-long-enough-passphrase", model.RoleReader)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if created.PasswordHash != "" {
		t.Error("a created account is returned with its password hash")
	}

	// And the hash really was stored, so the check above is about what is
	// reported rather than about there being nothing to report.
	stored, err := st.Users().Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("read the created account back: %v", err)
	}
	if stored.PasswordHash == "" {
		t.Fatal("the account was stored with no password hash, so nothing can ever sign in as it")
	}
}

func TestARoleThatIsNotARoleIsRefused(t *testing.T) {
	t.Parallel()

	svc, st := usersFixture(t)
	ctx := t.Context()

	admin := seedAccount(t, st, "a1", "admin", model.RoleAdmin)
	target := seedAccount(t, st, "o1", "target", model.RoleOperator)

	if _, err := svc.CreateUser(ctx, "x", "new", "a-long-enough-passphrase", "superuser"); !errors.Is(err, ErrInvalidRole) {
		t.Errorf("creating an account with an invented role returned %v, want ErrInvalidRole", err)
	}
	if _, err := svc.SetRole(ctx, admin.ID, target.ID, "superuser"); !errors.Is(err, ErrInvalidRole) {
		t.Errorf("setting an invented role returned %v, want ErrInvalidRole", err)
	}
	if _, err := svc.CreateUser(ctx, "y", "new2", "a-long-enough-passphrase", ""); !errors.Is(err, ErrInvalidRole) {
		t.Errorf("creating an account with no role returned %v, want ErrInvalidRole -- the empty "+
			"role means 'written before roles existed' and must not be writable deliberately", err)
	}
}

func TestAUsernameCannotBeTakenTwice(t *testing.T) {
	t.Parallel()

	svc, st := usersFixture(t)
	ctx := t.Context()

	seedAccount(t, st, "a1", "taken", model.RoleAdmin)

	if _, err := svc.CreateUser(ctx, "a2", "taken", "a-long-enough-passphrase", model.RoleReader); !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("creating a second account with one username returned %v, want ErrUsernameTaken", err)
	}

	// Case and surrounding space are not a different username: FindByUsername
	// normalises, so sign-in would find whichever record it reached first.
	if _, err := svc.CreateUser(ctx, "a3", "  TAKEN  ", "a-long-enough-passphrase", model.RoleReader); !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("creating an account whose username differs only in case and space returned %v, "+
			"want ErrUsernameTaken -- sign-in normalises, so these are one account", err)
	}
}

// TestAResetPasswordSignsIn closes the loop: the admin reset is only worth
// anything if the account can then actually use it.
func TestAResetPasswordSignsIn(t *testing.T) {
	t.Parallel()

	svc, st := usersFixture(t)
	ctx := t.Context()

	u := seedAccount(t, st, "a1", "somebody", model.RoleOperator)

	const fresh = "a-brand-new-passphrase"
	if err := svc.SetPassword(ctx, u.ID, fresh); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	if _, err := svc.verifyPassword(ctx, "somebody", fresh); err != nil {
		t.Errorf("the account cannot sign in with the password that was just set for it: %v", err)
	}
	if _, err := svc.verifyPassword(ctx, "somebody", "not-the-new-one"); !errors.Is(err, ErrInvalidCredentials) {
		t.Error("the old password still works after a reset")
	}
}
