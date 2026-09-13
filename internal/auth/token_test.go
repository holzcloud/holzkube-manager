package auth

// Service accounts, and the two asymmetries that make them safe.
//
// A password never signs in a service account, and a token never signs in a
// person. An identity that can be reached two ways is an identity whose weakest
// way in is the one that matters, and these are what keep that from happening
// by accident later.

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

func TestATokenAuthenticatesItsOwnAccountAndNothingElse(t *testing.T) {
	t.Parallel()

	svc, st := usersFixture(t)
	ctx := t.Context()

	seedAccount(t, st, "p1", "a-person", model.RoleAdmin)

	created, token, err := svc.CreateServiceAccount(ctx, "s1", "ci", model.RoleOperator)
	if err != nil {
		t.Fatalf("CreateServiceAccount: %v", err)
	}
	if !strings.HasPrefix(token, TokenPrefix) {
		t.Errorf("the token %q does not carry the prefix that makes it recognisable in a log "+
			"or a secret scanner", token)
	}
	if created.TokenHash != "" {
		t.Error("the created account is returned carrying its token hash")
	}

	got, err := svc.AuthenticateToken(ctx, token)
	if err != nil {
		t.Fatalf("the token that was just minted does not authenticate: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("the token authenticated %s, want %s", got.ID, created.ID)
	}
	if got.Role != model.RoleOperator {
		t.Errorf("the authenticated account has role %q, want operator", got.Role)
	}

	for name, bad := range map[string]string{
		"empty":           "",
		"no prefix":       strings.TrimPrefix(token, TokenPrefix),
		"prefix only":     TokenPrefix,
		"a near miss":     token[:len(token)-1] + "x",
		"somebody's word": "hkm_password",
	} {
		if _, err := svc.AuthenticateToken(ctx, bad); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s authenticated: %v", name, err)
		}
	}
}

// TestATokenIsNeverStored is the property that makes "shown once" true.
func TestATokenIsNeverStored(t *testing.T) {
	t.Parallel()

	svc, st := usersFixture(t)
	ctx := t.Context()

	created, token, err := svc.CreateServiceAccount(ctx, "s1", "ci", model.RoleReader)
	if err != nil {
		t.Fatalf("CreateServiceAccount: %v", err)
	}

	stored, err := st.Users().Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("read the account back: %v", err)
	}
	if stored.TokenHash == "" {
		t.Fatal("nothing was stored, so nothing can ever authenticate")
	}
	if strings.Contains(stored.TokenHash, strings.TrimPrefix(token, TokenPrefix)) {
		t.Error("the stored hash contains the token, which makes it a copy rather than a hash")
	}
	if stored.TokenIssuedAt.IsZero() {
		t.Error("the account records no issue time, so nothing can say how old this credential is")
	}
}

func TestRotatingATokenInvalidatesTheOldOne(t *testing.T) {
	t.Parallel()

	svc, st := usersFixture(t)
	ctx := t.Context()

	created, first, err := svc.CreateServiceAccount(ctx, "s1", "ci", model.RoleReader)
	if err != nil {
		t.Fatalf("CreateServiceAccount: %v", err)
	}

	second, err := svc.RotateToken(ctx, created.ID)
	if err != nil {
		t.Fatalf("RotateToken: %v", err)
	}
	if second == first {
		t.Fatal("rotation produced the same token")
	}

	if _, err := svc.AuthenticateToken(ctx, second); err != nil {
		t.Errorf("the new token does not authenticate: %v", err)
	}
	if _, err := svc.AuthenticateToken(ctx, first); !errors.Is(err, ErrInvalidToken) {
		t.Error("the old token still authenticates. Rotation is the only revocation this " +
			"product has, and a rotation that leaves the old one working revokes nothing")
	}

	// Rotation is refused for a person, because a person has no token to
	// rotate and minting one would be a second way into their account.
	person := seedAccount(t, st, "p1", "a-person", model.RoleAdmin)
	if _, err := svc.RotateToken(ctx, person.ID); !errors.Is(err, ErrNotAServiceAccount) {
		t.Errorf("minting a token for a person returned %v, want ErrNotAServiceAccount", err)
	}
}

// TestAServiceAccountCannotSignInWithAPassword is the other asymmetry.
//
// It has no password hash, so an empty one must not verify -- and the refusal
// is in verifyPassword rather than at the form, so that every path to a
// password check meets it.
func TestAServiceAccountCannotSignInWithAPassword(t *testing.T) {
	t.Parallel()

	svc, _ := usersFixture(t)
	ctx := t.Context()

	if _, _, err := svc.CreateServiceAccount(ctx, "s1", "ci", model.RoleAdmin); err != nil {
		t.Fatalf("CreateServiceAccount: %v", err)
	}

	for name, password := range map[string]string{
		"the empty password":  "",
		"a plausible one":     "a-long-enough-passphrase",
		"the account's name":  "ci",
		"an empty-hash probe": "$argon2id$v=19$m=65536,t=1,p=1$c2FsdA$aGFzaA",
	} {
		if _, err := svc.verifyPassword(ctx, "ci", password); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%s signed in as a service account: %v", name, err)
		}
	}
}

// TestLastUsedIsRecordedAndThrottled covers both halves of a field that is
// written on the read path.
func TestLastUsedIsRecordedAndThrottled(t *testing.T) {
	t.Parallel()

	svc, st := usersFixture(t)
	ctx := t.Context()

	created, token, err := svc.CreateServiceAccount(ctx, "s1", "ci", model.RoleReader)
	if err != nil {
		t.Fatalf("CreateServiceAccount: %v", err)
	}

	at := time.Now().UTC()
	svc.now = func() time.Time { return at }

	if _, err := svc.AuthenticateToken(ctx, token); err != nil {
		t.Fatalf("AuthenticateToken: %v", err)
	}
	after, err := st.Users().Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if after.LastUsedAt.IsZero() {
		t.Fatal("the first use was not recorded, so nothing can say whether a token is still live")
	}
	firstRev := after.Rev

	// A second use inside the throttle window must not write again: this runs
	// on every API call a machine makes, and a store write per call would lose
	// a revision race with whatever the call was about to do.
	svc.now = func() time.Time { return at.Add(tokenUseThrottle / 2) }
	if _, err := svc.AuthenticateToken(ctx, token); err != nil {
		t.Fatalf("AuthenticateToken: %v", err)
	}
	again, err := st.Users().Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if again.Rev != firstRev {
		t.Errorf("a second use inside the throttle window wrote the record again (rev %d -> %d)",
			firstRev, again.Rev)
	}

	// And past the window it does.
	svc.now = func() time.Time { return at.Add(2 * tokenUseThrottle) }
	if _, err := svc.AuthenticateToken(ctx, token); err != nil {
		t.Fatalf("AuthenticateToken: %v", err)
	}
	later, err := st.Users().Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !later.LastUsedAt.After(after.LastUsedAt) {
		t.Error("a use past the throttle window did not move LastUsedAt, so the field freezes " +
			"at whatever the first call set it to")
	}
}
