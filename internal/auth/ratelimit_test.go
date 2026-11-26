package auth

import (
	"testing"
	"time"
)

// frozenLimiter is a limiter whose clock the test moves by hand, so the
// exponential growth can be walked out to a hundred attempts without waiting
// the hours it describes.
func frozenLimiter(t *testing.T) (*Limiter, func(time.Duration)) {
	t.Helper()

	base := time.Now()
	at := base
	l := NewLimiter()
	l.now = func() time.Time { return at }

	return l, func(d time.Duration) { at = base.Add(d) }
}

// TestRateLimitDoesNotDelayTheFirstAttempt keeps the protection off the
// operator's normal path: typing your password correctly costs nothing.
func TestRateLimitDoesNotDelayTheFirstAttempt(t *testing.T) {
	l, _ := frozenLimiter(t)

	if d := l.Delay("192.0.2.1"); d != 0 {
		t.Errorf("first attempt delayed by %v, want 0", d)
	}

	l.Fail("192.0.2.1")
	if d := l.Delay("192.0.2.1"); d <= 0 {
		t.Errorf("second attempt delayed by %v, want more than 0", d)
	}
}

// TestRateLimitGrowsExponentiallyAndStopsAt30Seconds is D-08's shape: fast
// growth, a hard ceiling, and no state beyond the ceiling.
func TestRateLimitGrowsExponentiallyAndStopsAt30Seconds(t *testing.T) {
	l, _ := frozenLimiter(t)
	const ip = "192.0.2.2"

	var previous time.Duration
	for attempt := 1; attempt <= 8; attempt++ {
		l.Fail(ip)
		d := l.Delay(ip)

		if d > MaxLoginDelay {
			t.Fatalf("after %d failures the delay is %v, above the %v ceiling", attempt, d, MaxLoginDelay)
		}
		if d < previous {
			t.Errorf("after %d failures the delay shrank from %v to %v", attempt, previous, d)
		}
		if d != MaxLoginDelay && attempt > 1 && d != previous*2 {
			t.Errorf("after %d failures the delay is %v, want twice the previous %v", attempt, d, previous)
		}
		previous = d
	}

	for range 92 {
		l.Fail(ip)
	}
	if d := l.Delay(ip); d != MaxLoginDelay {
		t.Errorf("after 100 failures the delay is %v, want exactly %v", d, MaxLoginDelay)
	}
}

// TestRateLimitHasNoStateThatOutlastsTheWait is the whole of D-08: however many
// times somebody guesses wrong, waiting is always enough. There is nothing to
// unlock because nothing locks.
func TestRateLimitHasNoStateThatOutlastsTheWait(t *testing.T) {
	l, advance := frozenLimiter(t)
	const ip = "192.0.2.3"

	for range 100 {
		l.Fail(ip)
	}
	if d := l.Delay(ip); d != MaxLoginDelay {
		t.Fatalf("delay = %v, want %v", d, MaxLoginDelay)
	}

	advance(MaxLoginDelay + time.Second)
	if d := l.Delay(ip); d != 0 {
		t.Fatalf("after waiting out the ceiling the delay is still %v; this is a lockout", d)
	}
}

// TestRateLimitForgetsAfterSuccess: a correct password ends the episode, so the
// operator who fat-fingered twice is not still paying for it an hour later.
func TestRateLimitForgetsAfterSuccess(t *testing.T) {
	l, _ := frozenLimiter(t)
	const ip = "192.0.2.4"

	for range 5 {
		l.Fail(ip)
	}
	if l.Delay(ip) == 0 {
		t.Fatal("five failures produced no delay")
	}

	l.Succeed(ip)
	if d := l.Delay(ip); d != 0 {
		t.Errorf("delay after a success = %v, want 0", d)
	}
	if n := l.Len(); n != 0 {
		t.Errorf("%d entries left after a success, want 0", n)
	}
}

// TestRateLimitIsPerSourceIP is the reason the counter is not per account: with
// one operator, a per-account counter is a global one, and a global counter is
// a way for the only operator to shut themselves out, dressed as a defence.
func TestRateLimitIsPerSourceIP(t *testing.T) {
	l, _ := frozenLimiter(t)

	for range 10 {
		l.Fail("192.0.2.5")
	}
	if l.Delay("192.0.2.5") == 0 {
		t.Fatal("the failing source is not delayed")
	}
	if d := l.Delay("198.51.100.7"); d != 0 {
		t.Errorf("an unrelated source is delayed by %v, want 0", d)
	}
}

// TestRateLimitForgetsIdleSources keeps the map from being a slow leak in a
// process meant to run for months (T-01-28).
func TestRateLimitForgetsIdleSources(t *testing.T) {
	l, advance := frozenLimiter(t)

	l.Fail("192.0.2.6")
	l.Fail("198.51.100.8")
	if n := l.Len(); n != 2 {
		t.Fatalf("%d entries, want 2", n)
	}

	advance(2 * time.Hour)
	l.Fail("203.0.113.9")

	if n := l.Len(); n != 1 {
		t.Errorf("%d entries after two hours of idleness, want only the recent one", n)
	}
	if d := l.Delay("192.0.2.6"); d != 0 {
		t.Errorf("a forgotten source is still delayed by %v", d)
	}
}

// TestAReservationIsChargedBeforeThePasswordIsChecked is the parallel-guess
// fault: Delay read before the check and Fail recorded after it let every
// attempt sent at once through. With the charge taken in the same critical
// section as the question, only one of many simultaneous attempts proceeds.
func TestAReservationIsChargedBeforeThePasswordIsChecked(t *testing.T) {
	l, _ := frozenLimiter(t)

	granted := 0
	for range 24 {
		if wait, res := l.Reserve("192.0.2.1"); res != nil {
			granted++
		} else if wait <= 0 {
			t.Fatalf("a refused reservation reported no wait")
		}
	}
	if granted != 1 {
		t.Fatalf("%d of 24 simultaneous attempts were let through, want 1", granted)
	}
}

// TestAReservationCountsAgainstEveryKey: the re-authentication counts per
// address and per account, so a second address does not reset the account.
func TestAReservationCountsAgainstEveryKey(t *testing.T) {
	l, _ := frozenLimiter(t)

	_, res := l.Reserve("192.0.2.1", "sudo-user:a")
	if res == nil {
		t.Fatal("the first attempt was refused")
	}
	res.Fail()

	if wait, res := l.Reserve("192.0.2.2", "sudo-user:a"); res != nil || wait <= 0 {
		t.Fatalf("a second address reset the account's counter (wait %v)", wait)
	}
	if got := l.Failures("sudo-user:a"); got != 1 {
		t.Errorf("Failures = %d, want 1", got)
	}
}

// TestAReservationsOutcomeSettlesTheCharge: success forgets, cancel refunds,
// and failure keeps the charge.
func TestAReservationsOutcomeSettlesTheCharge(t *testing.T) {
	l, advance := frozenLimiter(t)

	_, res := l.Reserve("192.0.2.1")
	res.Cancel()
	if l.Len() != 0 {
		t.Errorf("a cancelled attempt left %d sources behind", l.Len())
	}

	_, res = l.Reserve("192.0.2.1")
	res.Fail()
	res.Succeed() // a second outcome does nothing
	if got := l.Failures("192.0.2.1"); got != 1 {
		t.Fatalf("after one failure Failures = %d, want 1", got)
	}

	advance(time.Minute)
	_, res = l.Reserve("192.0.2.1")
	if res == nil {
		t.Fatal("the wait was served and the attempt still refused")
	}
	res.Succeed()
	if l.Len() != 0 {
		t.Errorf("a successful attempt left %d sources behind", l.Len())
	}
}
