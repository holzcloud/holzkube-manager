package auth

import (
	"context"
	"testing"
	"time"
)

// TestSudoWindowStartsClosed is the property that makes the gate worth having:
// a session is authenticated and still not authorised.
func TestSudoWindowStartsClosed(t *testing.T) {
	svc, st := newTestService(t, 24*time.Hour)
	useTestParams(t)
	seedCheapUser(t, st)

	token := runInSession(t, svc, func(ctx context.Context) {
		if _, err := svc.Login(ctx, testUsername, testPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
	})

	continueSession(t, svc, token, func(ctx context.Context) {
		if !svc.IsAuthenticated(ctx) {
			t.Fatal("the session is not authenticated")
		}
		if svc.IsSudoOpen(ctx, DefaultSudoWindow) {
			t.Error("logging in opened the sudo window; a stolen cookie would inherit it")
		}
	})
}

// TestSudoWindowOpensAndExpires walks the five minutes of D-05 from both sides.
func TestSudoWindowOpensAndExpires(t *testing.T) {
	svc, st := newTestService(t, 24*time.Hour)
	useTestParams(t)
	seedCheapUser(t, st)

	opened := time.Now()
	svc.now = func() time.Time { return opened }

	token := runInSession(t, svc, func(ctx context.Context) {
		if _, err := svc.Login(ctx, testUsername, testPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
		svc.OpenSudoWindow(ctx)
	})

	for _, tc := range []struct {
		after time.Duration
		want  bool
	}{
		{0, true},
		{4*time.Minute + 59*time.Second, true},
		{5 * time.Minute, false},
		{5*time.Minute + 1*time.Second, false},
		{time.Hour, false},
	} {
		svc.now = func() time.Time { return opened.Add(tc.after) }
		continueSession(t, svc, token, func(ctx context.Context) {
			if got := svc.IsSudoOpen(ctx, 5*time.Minute); got != tc.want {
				t.Errorf("%v after opening: IsSudoOpen = %v, want %v", tc.after, got, tc.want)
			}
		})
	}
}

// TestSudoTouchRestartsTheWindow is what keeps a series of destructive actions
// from asking for the password once per node (D-05).
func TestSudoTouchRestartsTheWindow(t *testing.T) {
	svc, st := newTestService(t, 24*time.Hour)
	useTestParams(t)
	seedCheapUser(t, st)

	opened := time.Now()
	svc.now = func() time.Time { return opened }

	token := runInSession(t, svc, func(ctx context.Context) {
		if _, err := svc.Login(ctx, testUsername, testPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
		svc.OpenSudoWindow(ctx)
	})

	// Four minutes in, a destructive action succeeds and restarts the window.
	svc.now = func() time.Time { return opened.Add(4 * time.Minute) }
	continueSession(t, svc, token, func(ctx context.Context) {
		svc.TouchSudoWindow(ctx)
	})

	// Eight minutes after the original re-authentication -- three minutes past
	// where the first window would have ended -- the operator is still working.
	svc.now = func() time.Time { return opened.Add(8 * time.Minute) }
	continueSession(t, svc, token, func(ctx context.Context) {
		if !svc.IsSudoOpen(ctx, 5*time.Minute) {
			t.Error("the window did not restart after a destructive action")
		}
	})

	// It is still a window, not a switch: five minutes after the last action it
	// is closed again.
	svc.now = func() time.Time { return opened.Add(10 * time.Minute) }
	continueSession(t, svc, token, func(ctx context.Context) {
		if svc.IsSudoOpen(ctx, 5*time.Minute) {
			t.Error("the restarted window never closes")
		}
	})
}

// TestSudoWindowIsPerSession is the reason the state lives in the session and
// not in a process-wide map: a second session must not inherit the first one's
// re-authentication.
func TestSudoWindowIsPerSession(t *testing.T) {
	svc, st := newTestService(t, 24*time.Hour)
	useTestParams(t)
	seedCheapUser(t, st)

	first := runInSession(t, svc, func(ctx context.Context) {
		if _, err := svc.Login(ctx, testUsername, testPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
		svc.OpenSudoWindow(ctx)
	})
	second := runInSession(t, svc, func(ctx context.Context) {
		if _, err := svc.Login(ctx, testUsername, testPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
	})

	continueSession(t, svc, first, func(ctx context.Context) {
		if !svc.IsSudoOpen(ctx, DefaultSudoWindow) {
			t.Error("the re-authenticated session has no open window")
		}
	})
	continueSession(t, svc, second, func(ctx context.Context) {
		if svc.IsSudoOpen(ctx, DefaultSudoWindow) {
			t.Error("a second session of the same operator inherited the window")
		}
	})
}

// TestSudoWindowFallsBackToTheDefault pins the reading of a missing or
// nonsensical configured window: the decided default, not "always open" and not
// "permanently unreachable".
func TestSudoWindowFallsBackToTheDefault(t *testing.T) {
	svc, st := newTestService(t, 24*time.Hour)
	useTestParams(t)
	seedCheapUser(t, st)

	opened := time.Now()
	svc.now = func() time.Time { return opened }

	token := runInSession(t, svc, func(ctx context.Context) {
		if _, err := svc.Login(ctx, testUsername, testPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
		svc.OpenSudoWindow(ctx)
	})

	continueSession(t, svc, token, func(ctx context.Context) {
		if !svc.IsSudoOpen(ctx, 0) {
			t.Error("an unset window is treated as permanently closed")
		}
	})

	svc.now = func() time.Time { return opened.Add(DefaultSudoWindow + time.Second) }
	continueSession(t, svc, token, func(ctx context.Context) {
		if svc.IsSudoOpen(ctx, 0) {
			t.Error("an unset window never expires")
		}
	})
}
