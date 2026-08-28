package auth

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/alexedwards/scs/v2"

	"github.com/holzcloud/holzkube/internal/model"
	"github.com/holzcloud/holzkube/internal/store"
	"github.com/holzcloud/holzkube/internal/store/fsstore"
)

const (
	testUsername = "operator"
	testPassword = "correct-horse-battery-staple"
)

// newTestService wires a Service over a real fsstore in a throwaway directory.
//
// The directory is tightened to 0700 because the store's permission guard
// refuses to open anything group or other can read (FOUND-10), and Go's
// t.TempDir hands out 0755.
func newTestService(t *testing.T, lifetime time.Duration) (*Service, store.Store) {
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

	svc, err := New(st, lifetime)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	return svc, st
}

// seedUser writes the one operator account with a caller-chosen hash, so a test
// can decide how much a verification is going to cost it.
func seedUser(t *testing.T, st store.Store, hash string) model.User {
	t.Helper()
	u, err := st.Users().Put(context.Background(), model.User{
		ID:           model.UserID("0123456789abcdef0123456789abcdef"),
		Username:     testUsername,
		PasswordHash: hash,
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

func seedCheapUser(t *testing.T, st store.Store) model.User {
	t.Helper()
	h, err := argon2id.CreateHash(testPassword, weakParams)
	if err != nil {
		t.Fatalf("CreateHash: %v", err)
	}
	return seedUser(t, st, h)
}

// runInSession loads the session behind token (or a fresh one for ""), runs fn
// inside it and persists whatever fn changed, returning the token the next
// "request" would present. It is the non-HTTP equivalent of one round trip
// through the session middleware.
func runInSession(t *testing.T, svc *Service, fn func(ctx context.Context)) string {
	t.Helper()
	return continueSession(t, svc, "", fn)
}

func continueSession(t *testing.T, svc *Service, token string, fn func(ctx context.Context)) string {
	t.Helper()

	sm := svc.Sessions()
	ctx, err := sm.Load(context.Background(), token)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	fn(ctx)

	switch sm.Status(ctx) {
	case scs.Destroyed:
		return ""
	case scs.Modified:
		tok, _, err := sm.Commit(ctx)
		if err != nil {
			t.Fatalf("commit session: %v", err)
		}
		return tok
	default:
		return sm.Token(ctx)
	}
}

func authenticatedIn(t *testing.T, svc *Service, token string) bool {
	t.Helper()
	var ok bool
	continueSession(t, svc, token, func(ctx context.Context) {
		ok = svc.IsAuthenticated(ctx)
	})
	return ok
}

// TestSessionRotatesOnLogin closes the session-fixation hole (T-01-24): the id
// a client held before authenticating must be worthless afterwards.
func TestSessionRotatesOnLogin(t *testing.T) {
	svc, st := newTestService(t, 24*time.Hour)
	useTestParams(t)
	seedCheapUser(t, st)

	// An anonymous visitor gets a session before ever logging in.
	before := runInSession(t, svc, func(ctx context.Context) {
		svc.Sessions().Put(ctx, "anonymous_marker", "1")
	})
	if before == "" {
		t.Fatal("no pre-authentication session token")
	}

	after := continueSession(t, svc, before, func(ctx context.Context) {
		if _, err := svc.Login(ctx, testUsername, testPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
	})
	if after == "" {
		t.Fatal("no post-authentication session token")
	}
	if after == before {
		t.Fatalf("session id did not rotate on login: %q", after)
	}

	if !authenticatedIn(t, svc, after) {
		t.Error("the rotated session is not authenticated")
	}
	if authenticatedIn(t, svc, before) {
		t.Error("the pre-authentication session id still authenticates")
	}
	if _, err := st.Sessions().Get(context.Background(), before); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the old session record survived rotation: err = %v", err)
	}
}

// TestSessionAbsoluteLimitIgnoresActivity is D-07 stated as a property rather
// than a configuration value: 24 hours from issue, no matter how busy the
// operator was in between.
func TestSessionAbsoluteLimitIgnoresActivity(t *testing.T) {
	svc, st := newTestService(t, 24*time.Hour)
	useTestParams(t)
	seedCheapUser(t, st)

	issued := time.Now()
	svc.now = func() time.Time { return issued }

	token := runInSession(t, svc, func(ctx context.Context) {
		if _, err := svc.Login(ctx, testUsername, testPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
	})

	first, err := st.Sessions().Get(context.Background(), token)
	if err != nil {
		t.Fatalf("read session: %v", err)
	}

	// Constant traffic across most of the day.
	for hour := 1; hour <= 20; hour++ {
		svc.now = func() time.Time { return issued.Add(time.Duration(hour) * time.Hour) }
		if !authenticatedIn(t, svc, token) {
			t.Fatalf("session expired after %d hours of activity", hour)
		}
		continueSession(t, svc, token, func(ctx context.Context) {
			svc.Sessions().Put(ctx, "touch", hour)
		})
	}

	later, err := st.Sessions().Get(context.Background(), token)
	if err != nil {
		t.Fatalf("read session: %v", err)
	}
	if !later.ExpiresAt.Equal(first.ExpiresAt) {
		t.Errorf("activity moved the expiry from %v to %v; the limit is absolute",
			first.ExpiresAt, later.ExpiresAt)
	}

	svc.now = func() time.Time { return issued.Add(25 * time.Hour) }
	if authenticatedIn(t, svc, token) {
		t.Error("a 25 hour old session still authenticates")
	}
}

// TestLogoutInvalidatesTheSessionServerSide proves logout is not merely a
// cleared cookie: the record is gone, so a stolen copy of the id is worthless.
func TestLogoutInvalidatesTheSessionServerSide(t *testing.T) {
	svc, st := newTestService(t, 24*time.Hour)
	useTestParams(t)
	seedCheapUser(t, st)

	token := runInSession(t, svc, func(ctx context.Context) {
		if _, err := svc.Login(ctx, testUsername, testPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
	})

	continueSession(t, svc, token, func(ctx context.Context) {
		if err := svc.Logout(ctx); err != nil {
			t.Fatalf("Logout: %v", err)
		}
	})

	if authenticatedIn(t, svc, token) {
		t.Error("the session still authenticates after logout")
	}
	if _, err := st.Sessions().Get(context.Background(), token); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the session record survived logout: err = %v", err)
	}
}

// TestChangePasswordInvalidatesEveryOtherSession records the decision the plan
// left to discretion: a password change is nearly always a reaction to a
// suspicion, so it throws out every other session and keeps the one doing the
// throwing.
func TestChangePasswordInvalidatesEveryOtherSession(t *testing.T) {
	svc, st := newTestService(t, 24*time.Hour)
	useTestParams(t)
	seedCheapUser(t, st)

	first := runInSession(t, svc, func(ctx context.Context) {
		if _, err := svc.Login(ctx, testUsername, testPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
	})
	second := runInSession(t, svc, func(ctx context.Context) {
		if _, err := svc.Login(ctx, testUsername, testPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
	})
	if first == second {
		t.Fatal("the two sessions share a token")
	}

	const newPassword = "a-completely-different-passphrase"
	continueSession(t, svc, first, func(ctx context.Context) {
		if err := svc.ChangePassword(ctx, testPassword, newPassword); err != nil {
			t.Fatalf("ChangePassword: %v", err)
		}
	})

	if !authenticatedIn(t, svc, first) {
		t.Error("the session that changed the password was logged out")
	}
	if authenticatedIn(t, svc, second) {
		t.Error("a parallel session survived the password change")
	}

	users, err := st.Users().List(context.Background())
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	ok, err := Verify(newPassword, users[0].PasswordHash)
	if err != nil || !ok {
		t.Fatalf("the new password does not verify: ok=%v err=%v", ok, err)
	}
	ok, err = Verify(testPassword, users[0].PasswordHash)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if ok {
		t.Error("the old password still verifies")
	}
}

// TestChangePasswordRejectsAWrongCurrentPassword pins the half of the operation
// that must change nothing at all.
func TestChangePasswordRejectsAWrongCurrentPassword(t *testing.T) {
	svc, st := newTestService(t, 24*time.Hour)
	useTestParams(t)
	seeded := seedCheapUser(t, st)

	first := runInSession(t, svc, func(ctx context.Context) {
		if _, err := svc.Login(ctx, testUsername, testPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
	})
	second := runInSession(t, svc, func(ctx context.Context) {
		if _, err := svc.Login(ctx, testUsername, testPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
	})

	continueSession(t, svc, first, func(ctx context.Context) {
		err := svc.ChangePassword(ctx, "not-the-current-password", "a-completely-different-passphrase")
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("ChangePassword err = %v, want ErrInvalidCredentials", err)
		}
	})

	users, err := st.Users().List(context.Background())
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if users[0].PasswordHash != seeded.PasswordHash {
		t.Error("a rejected password change rewrote the stored hash")
	}
	if !authenticatedIn(t, svc, second) {
		t.Error("a rejected password change invalidated another session")
	}
}

// TestSessionCookieFollowsTheTransportContract keeps the cookie attributes from
// drifting: every one of them is load bearing, and none of them is visible in a
// test that only checks whether login works.
func TestSessionCookieFollowsTheTransportContract(t *testing.T) {
	svc, _ := newTestService(t, 24*time.Hour)
	sm := svc.Sessions()

	if sm.Lifetime != 24*time.Hour {
		t.Errorf("lifetime = %v, want 24h", sm.Lifetime)
	}
	if sm.Cookie.Name != CookieName {
		t.Errorf("cookie name = %q, want %q", sm.Cookie.Name, CookieName)
	}
	if !sm.Cookie.HttpOnly {
		t.Error("cookie is readable from JavaScript")
	}
	if !sm.Cookie.Secure {
		t.Error("cookie is not restricted to TLS")
	}
	if sm.Cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite = %v, want Lax", sm.Cookie.SameSite)
	}
	if sm.Cookie.Path != "/" {
		t.Errorf("cookie path = %q, want /", sm.Cookie.Path)
	}
	if !sm.Cookie.Persist {
		t.Error("cookie does not survive a browser restart")
	}
}
