package auth

import (
	"context"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// linkedPerson seeds the one operator account with a cheap password and a
// binding to the old provider identity.
func linkedPerson(t *testing.T) (*Service, model.User) {
	t.Helper()
	svc, st := newTestService(t, 24*time.Hour)
	useTestParams(t)
	u := seedCheapUser(t, st)
	u.Issuer, u.Subject, u.Role, u.Kind = oldIssuer, oldSubject, model.RoleAdmin, model.KindPerson
	u, err := st.Users().Put(context.Background(), u)
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	return svc, u
}

func providerSession(t *testing.T, svc *Service, u model.User, issuer, subject string) string {
	t.Helper()
	return runInSession(t, svc, func(ctx context.Context) {
		if err := svc.StartProviderSession(ctx, u, issuer, subject); err != nil {
			t.Fatalf("StartProviderSession: %v", err)
		}
	})
}

func passwordSession(t *testing.T, svc *Service) string {
	t.Helper()
	return runInSession(t, svc, func(ctx context.Context) {
		if _, err := svc.Login(ctx, testUsername, testPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
	})
}

// A session that came in through a provider identity lives only as long as
// that identity is linked to its account. Unlinking ends it -- the session of
// somebody who should not have been linked included -- and leaves the
// account's password sessions alone.
func TestASessionThroughTheProviderEndsWithItsLink(t *testing.T) {
	t.Parallel()
	svc, u := linkedPerson(t)
	ctx := context.Background()

	viaProvider := providerSession(t, svc, u, oldIssuer, oldSubject)
	viaPassword := passwordSession(t, svc)
	if !authenticatedIn(t, svc, viaProvider) || !authenticatedIn(t, svc, viaPassword) {
		t.Fatalf("a fresh session is not signed in")
	}

	if _, err := svc.UnlinkIdentity(ctx, u.ID); err != nil {
		t.Fatalf("UnlinkIdentity: %v", err)
	}
	if authenticatedIn(t, svc, viaProvider) {
		t.Errorf("the session that came in through the removed link is still signed in")
	}
	if !authenticatedIn(t, svc, viaPassword) {
		t.Errorf("the unlink ended a password session")
	}
}

// Linking the account to somebody else does not bring an ended session back:
// the session is tied to the identity it came in through, not to "linked".
func TestASessionThroughTheProviderDoesNotOutliveARelink(t *testing.T) {
	t.Parallel()
	svc, u := linkedPerson(t)
	ctx := context.Background()

	viaOld := providerSession(t, svc, u, oldIssuer, oldSubject)
	if _, err := svc.UnlinkIdentity(ctx, u.ID); err != nil {
		t.Fatalf("UnlinkIdentity: %v", err)
	}
	if _, err := svc.BindIdentity(ctx, u, newIssuer, newSubject); err != nil {
		t.Fatalf("BindIdentity: %v", err)
	}
	if authenticatedIn(t, svc, viaOld) {
		t.Errorf("a session through the old identity is signed in again after a relink")
	}
	if viaNew := providerSession(t, svc, u, newIssuer, newSubject); !authenticatedIn(t, svc, viaNew) {
		t.Errorf("a session through the new identity is not signed in")
	}
}

// A provider session from before this release carries the flag and no
// fingerprint. What can still be checked of it is that its account is linked
// at all.
func TestAProviderSessionFromAnEarlierReleaseEndsWithTheLink(t *testing.T) {
	t.Parallel()
	svc, u := linkedPerson(t)

	legacy := runInSession(t, svc, func(ctx context.Context) {
		if err := svc.StartSession(ctx, u); err != nil {
			t.Fatalf("StartSession: %v", err)
		}
		svc.sm.Put(ctx, sessionKeyProviderSignIn, true)
	})
	if !authenticatedIn(t, svc, legacy) {
		t.Fatalf("an earlier release's provider session is not signed in while linked")
	}
	if _, err := svc.UnlinkIdentity(context.Background(), u.ID); err != nil {
		t.Fatalf("UnlinkIdentity: %v", err)
	}
	if authenticatedIn(t, svc, legacy) {
		t.Errorf("an earlier release's provider session outlived the unlink")
	}
}

// Signing in with the password in a browser whose old session came through the
// provider makes a password session. The session id rotates but its data is
// kept, so without clearing the marks the new session would inherit the old
// one's tie to the link -- and an unlink would sign out somebody who never used
// single sign-on this time.
func TestAPasswordSignInDropsTheProviderMarks(t *testing.T) {
	t.Parallel()
	svc, u := linkedPerson(t)

	tok := providerSession(t, svc, u, oldIssuer, oldSubject)
	tok = continueSession(t, svc, tok, func(ctx context.Context) {
		if _, err := svc.Login(ctx, testUsername, testPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
		if svc.SignedInThroughProvider(ctx) {
			t.Errorf("a password sign-in still says it came through the provider")
		}
	})
	if _, err := svc.UnlinkIdentity(context.Background(), u.ID); err != nil {
		t.Fatalf("UnlinkIdentity: %v", err)
	}
	if !authenticatedIn(t, svc, tok) {
		t.Errorf("the unlink ended a password session that inherited a provider session's marks")
	}
}
