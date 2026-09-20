package auth

// The break-glass token.
//
// It is an admin credential handed out without anybody signing in, so the thing
// worth testing is not that it works -- it is everything that keeps it from
// outliving the errand it was minted for.

import (
	"errors"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// TestABreakGlassTokenStopsWorkingWhenItExpires.
//
// This is the single claim the whole feature rests on. A token minted with no
// decision behind it and no expiry is a forgotten admin credential in somebody's
// shell history.
func TestABreakGlassTokenStopsWorkingWhenItExpires(t *testing.T) {
	t.Parallel()

	svc, _ := usersFixture(t)
	ctx := t.Context()

	minted := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return minted }

	user, token, err := svc.MintBreakGlass(ctx, 15*time.Minute)
	if err != nil {
		t.Fatalf("MintBreakGlass: %v", err)
	}
	if user.Role != model.RoleAdmin {
		t.Errorf("the account is %q, want admin -- a break-glass credential that cannot do "+
			"the thing somebody broke the glass for invites hand-editing the store instead",
			user.Role)
	}

	// A minute before: still the credential it was minted as.
	svc.now = func() time.Time { return minted.Add(14 * time.Minute) }
	if _, err := svc.AuthenticateToken(ctx, token); err != nil {
		t.Fatalf("before the expiry the token does not authenticate: %v", err)
	}

	// Exactly AT the expiry, and after. At, because an expiry that is still
	// valid in the instant it names is an expiry nobody can reason about.
	for _, when := range []time.Duration{15 * time.Minute, time.Hour} {
		svc.now = func() time.Time { return minted.Add(when) }
		_, err := svc.AuthenticateToken(ctx, token)
		if !errors.Is(err, ErrTokenExpired) {
			t.Errorf("%s after minting the token gives %v, want ErrTokenExpired", when, err)
		}
	}
}

// TestAnOrdinaryServiceAccountNeverExpires.
//
// The expiry is for the one account minted without a sign-in. A service account
// runs a backup at three in the morning, and an expiry nobody is awake to renew
// is an outage rather than a safeguard.
func TestAnOrdinaryServiceAccountNeverExpires(t *testing.T) {
	t.Parallel()

	svc, _ := usersFixture(t)
	ctx := t.Context()

	created := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return created }

	account, token, err := svc.CreateServiceAccount(ctx, "s1", "nightly-backup", model.RoleOperator)
	if err != nil {
		t.Fatalf("CreateServiceAccount: %v", err)
	}
	if !account.TokenExpiresAt.IsZero() {
		t.Fatalf("a service account was given an expiry of %s", account.TokenExpiresAt)
	}

	svc.now = func() time.Time { return created.Add(5 * 365 * 24 * time.Hour) }
	if _, err := svc.AuthenticateToken(ctx, token); err != nil {
		t.Errorf("five years on, the nightly backup's token does not authenticate: %v", err)
	}
}

// TestMintingAgainRotatesTheSameAccount, rather than leaving a trail of admin
// accounts nobody remembers creating -- and the previous token stops working at
// that moment, which is what somebody expects of a credential they re-issued.
func TestMintingAgainRotatesTheSameAccount(t *testing.T) {
	t.Parallel()

	svc, st := usersFixture(t)
	ctx := t.Context()

	first, firstToken, err := svc.MintBreakGlass(ctx, time.Hour)
	if err != nil {
		t.Fatalf("MintBreakGlass: %v", err)
	}
	second, secondToken, err := svc.MintBreakGlass(ctx, time.Hour)
	if err != nil {
		t.Fatalf("MintBreakGlass again: %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("minting twice made two accounts, %q and %q", first.ID, second.ID)
	}
	users, err := st.Users().List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(users) != 1 {
		t.Errorf("the store holds %d accounts, want one", len(users))
	}

	if _, err := svc.AuthenticateToken(ctx, secondToken); err != nil {
		t.Errorf("the freshly minted token does not authenticate: %v", err)
	}
	if _, err := svc.AuthenticateToken(ctx, firstToken); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("the superseded token gives %v, want ErrInvalidToken -- re-issuing a "+
			"credential has to take the old one out of circulation", err)
	}
}

// TestABreakGlassTokenCannotBeAskedToLastForever.
func TestABreakGlassTokenCannotBeAskedToLastForever(t *testing.T) {
	t.Parallel()

	svc, _ := usersFixture(t)
	ctx := t.Context()

	if _, _, err := svc.MintBreakGlass(ctx, MaxBreakGlassTTL+time.Minute); !errors.Is(err, ErrTTLTooLong) {
		t.Errorf("a ttl past the maximum gives %v, want ErrTTLTooLong -- past a day the honest "+
			"thing to ask for is a service account created with somebody signed in", err)
	}

	// And zero means the default rather than "no expiry", which is the value a
	// caller that forgot the flag would otherwise get.
	minted := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return minted }
	user, _, err := svc.MintBreakGlass(ctx, 0)
	if err != nil {
		t.Fatalf("MintBreakGlass: %v", err)
	}
	if want := minted.Add(DefaultBreakGlassTTL); !user.TokenExpiresAt.Equal(want) {
		t.Errorf("a zero ttl expires at %s, want %s", user.TokenExpiresAt, want)
	}
}

// TestBreakGlassRefusesToConvertAPerson.
//
// Turning an account that has a password into a token account in place is the
// kind of surprise that ends with a person locked out of their own installation.
func TestBreakGlassRefusesToConvertAPerson(t *testing.T) {
	t.Parallel()

	svc, st := usersFixture(t)
	ctx := t.Context()

	seedAccount(t, st, BreakGlassID, BreakGlassUsername, model.RoleAdmin)

	if _, _, err := svc.MintBreakGlass(ctx, time.Hour); !errors.Is(err, ErrNotAServiceAccount) {
		t.Errorf("minting over a person gives %v, want ErrNotAServiceAccount", err)
	}

	// And the person is untouched: still signs in, still has no token.
	after, err := st.Users().Get(ctx, model.UserID(BreakGlassID))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if after.TokenHash != "" || after.PasswordHash == "" {
		t.Errorf("the person was converted anyway: token=%q password=%q",
			after.TokenHash, after.PasswordHash)
	}
}
