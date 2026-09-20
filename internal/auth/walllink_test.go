package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// The link a screen in a corridor is left open on (2026-09-20).
//
// The credential ends up on a television: bookmarked, photographed, forwarded.
// Every test here is a property that has to hold once it is out of anybody's
// hands.

// TestTheLinkIsShownOnceAndNeverStored.
func TestTheLinkIsShownOnceAndNeverStored(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t, time.Hour)
	ctx := context.Background()

	link, token, err := svc.CreateWallLink(ctx, "The screen in the IT office", "admin")
	if err != nil {
		t.Fatalf("CreateWallLink: %v", err)
	}
	if !strings.HasPrefix(token, WallTokenPrefix) {
		t.Errorf("token = %q, want the %q prefix so a scanner and a person both recognise it",
			token, WallTokenPrefix)
	}
	// The value that comes back from the create call carries no hash, and the
	// list never does either: a route that could return one would be a route
	// that keeps the credential reachable.
	if link.TokenHash != "" {
		t.Error("the created link carries its hash")
	}

	listed, err := svc.WallLinks(ctx)
	if err != nil {
		t.Fatalf("WallLinks: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed %d links, want one", len(listed))
	}
	if listed[0].TokenHash != "" {
		t.Error("the listed link carries its hash; only its own store copy may")
	}
	// And there is no second way to the token: nothing in the listed record
	// could be turned back into it.
	if strings.Contains(token, listed[0].ID) {
		t.Error("the id is a piece of the token, so the list hands out part of the credential")
	}
}

// TestOnlyTheRightTokenOpensIt.
func TestOnlyTheRightTokenOpensIt(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t, time.Hour)
	ctx := context.Background()

	_, token, err := svc.CreateWallLink(ctx, "The office screen", "admin")
	if err != nil {
		t.Fatalf("CreateWallLink: %v", err)
	}

	if _, err := svc.AuthenticateWallLink(ctx, token); err != nil {
		t.Errorf("the token that was just minted does not authenticate: %v", err)
	}
	for _, wrong := range []string{"", "hkw_", token + "x", strings.ToUpper(token)} {
		if _, err := svc.AuthenticateWallLink(ctx, wrong); !errors.Is(err, ErrInvalidWallLink) {
			t.Errorf("%q authenticated, or failed for the wrong reason: %v", wrong, err)
		}
	}
}

// TestATokenOfTheWrongKindNeverReachesTheComparison.
//
// Written against hashWallToken rather than against Authenticate, and that is
// the honest shape: a service-account token put through Authenticate is refused
// anyway, because its hash matches nothing. The prefix check buys something
// different and smaller -- a token of the wrong kind is rejected BY SHAPE, before
// anything is hashed or compared, so a wall link whose stored hash somebody had
// set to a service token's would still not open on one.
//
// The first version of this test claimed the bigger property and measured
// nothing: it passed with the prefix check removed, because the hashes differ
// either way.
func TestATokenOfTheWrongKindNeverReachesTheComparison(t *testing.T) {
	t.Parallel()

	for _, wrong := range []string{
		"hkm_abcdef",    // a service account's
		"abcdef",        // no prefix at all
		WallTokenPrefix, // the prefix and nothing behind it
	} {
		if got := hashWallToken(wrong); got != "" {
			t.Errorf("hashWallToken(%q) = %q, want nothing: a token of the wrong shape must not "+
				"be hashed and compared at all", wrong, got)
		}
	}
	if hashWallToken(WallTokenPrefix+"abcdef") == "" {
		t.Error("a well-shaped token hashes to nothing, so nothing would ever authenticate")
	}
}

// TestRevokingOneStopsTheScreenAtItsNextRefresh.
func TestRevokingOneStopsTheScreenAtItsNextRefresh(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t, time.Hour)
	ctx := context.Background()

	kept, keptToken, err := svc.CreateWallLink(ctx, "Kept", "admin")
	if err != nil {
		t.Fatalf("CreateWallLink: %v", err)
	}
	gone, goneToken, err := svc.CreateWallLink(ctx, "Revoked", "admin")
	if err != nil {
		t.Fatalf("CreateWallLink: %v", err)
	}

	if err := svc.RevokeWallLink(ctx, gone.ID); err != nil {
		t.Fatalf("RevokeWallLink: %v", err)
	}

	if _, err := svc.AuthenticateWallLink(ctx, goneToken); !errors.Is(err, ErrInvalidWallLink) {
		t.Error("a revoked link still authenticates")
	}
	// And revoking one does not touch the others, which is the thing somebody
	// pressing the button in a list of eight is trusting.
	if _, err := svc.AuthenticateWallLink(ctx, keptToken); err != nil {
		t.Errorf("revoking one link broke another: %v", err)
	}
	if err := svc.RevokeWallLink(ctx, gone.ID); !errors.Is(err, ErrNoSuchWallLink) {
		t.Error("revoking something that is not there is not reported as such")
	}
	_ = kept
}

// TestALinkNeedsALabel, because a list of unnamed credentials is a list nobody
// can revoke safely: the whole question at revocation time is WHICH screen.
func TestALinkNeedsALabel(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t, time.Hour)
	ctx := context.Background()

	if _, _, err := svc.CreateWallLink(ctx, "   ", "admin"); !errors.Is(err, ErrInvalidWallLink) {
		t.Errorf("an unlabelled link was accepted: %v", err)
	}
}

// TestThereIsACapOnHowManyExist.
//
// Every request carrying a token is compared against all of them, and a list
// nobody prunes is a list of credentials nobody has looked at.
func TestThereIsACapOnHowManyExist(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t, time.Hour)
	ctx := context.Background()

	for i := 0; i < MaxWallLinks; i++ {
		if _, _, err := svc.CreateWallLink(ctx, "screen", "admin"); err != nil {
			t.Fatalf("CreateWallLink %d: %v", i, err)
		}
	}
	if _, _, err := svc.CreateWallLink(ctx, "one too many", "admin"); !errors.Is(err, ErrTooManyWallLinks) {
		t.Errorf("the cap did not hold: %v", err)
	}
}

// TestUsingOneIsNoted, so the list can answer whether a link is still on a wall.
func TestUsingOneIsNoted(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t, time.Hour)
	ctx := context.Background()

	_, token, err := svc.CreateWallLink(ctx, "The office screen", "admin")
	if err != nil {
		t.Fatalf("CreateWallLink: %v", err)
	}

	before, err := svc.WallLinks(ctx)
	if err != nil {
		t.Fatalf("WallLinks: %v", err)
	}
	if !before[0].LastUsedAt.IsZero() {
		t.Error("a link nothing has used carries a last-used time")
	}

	if _, err := svc.AuthenticateWallLink(ctx, token); err != nil {
		t.Fatalf("AuthenticateWallLink: %v", err)
	}

	after, err := svc.WallLinks(ctx)
	if err != nil {
		t.Fatalf("WallLinks: %v", err)
	}
	if after[0].LastUsedAt.IsZero() {
		t.Error("using a link was not noted, so the list cannot say which are still on a wall")
	}
}
