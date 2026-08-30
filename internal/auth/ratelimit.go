package auth

import (
	"sync"
	"time"
)

// The login delay is holzkube-manager's only answer to guessing, and it is deliberately
// the weaker-sounding of the two available answers.
//
// D-08: repeated failures make the next attempt wait, the wait doubles, and it
// stops growing at half a minute. There is no state that survives the wait --
// no counter that has to be cleared by hand, no flag that stops a login from
// being possible, no administrative path back. That is not an omission and it
// must not be "fixed" later: holzkube-manager has exactly one operator, so any state
// that can refuse them is a state they can be stranded in, and every mechanism
// for getting out of it is a second way in for somebody else. The attacker's
// cost comes from here plus argon2id's quarter second per attempt; the
// operator's cost is zero, because their first attempt is never delayed.
//
// The counter is keyed by source IP and never by account name. With one
// account, a per-account counter is a global one: a stranger hammering the
// login from anywhere would slow the operator down from everywhere.
const (
	// BaseLoginDelay is the wait after the first failure. Each further failure
	// doubles it.
	BaseLoginDelay = 250 * time.Millisecond

	// MaxLoginDelay is the ceiling (D-08). Eight wrong guesses reach it; the
	// hundredth is no worse, which is what keeps a mistyped password from
	// turning into an outage.
	MaxLoginDelay = 30 * time.Second

	// idleForget is how long an untouched source is remembered. Anything older
	// is indistinguishable from a stranger, and keeping it is a slow leak in a
	// process meant to run for months (T-01-28).
	idleForget = time.Hour

	// sweepInterval bounds how often the map is walked.
	sweepInterval = time.Minute
)

// Limiter tracks recent failures per source address.
//
// The zero value is not usable; call NewLimiter.
type Limiter struct {
	mu      sync.Mutex
	sources map[string]*source

	lastSweep time.Time

	// now is the clock, so the growth curve can be walked out to a hundred
	// attempts in a test without waiting the hours it describes.
	now func() time.Time
}

// source is one address's recent history.
type source struct {
	failures    int
	nextAllowed time.Time
	touched     time.Time
}

// NewLimiter returns an empty limiter.
func NewLimiter() *Limiter {
	return &Limiter{
		sources: make(map[string]*source),
		now:     time.Now,
	}
}

// Delay reports how long this source must still wait before its next attempt is
// worth making. It is zero for a source with no recent failures, and zero again
// once the wait has been served.
func (l *Limiter) Delay(ip string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	s, ok := l.sources[ip]
	if !ok {
		return 0
	}
	remaining := s.nextAllowed.Sub(l.now())
	if remaining <= 0 {
		return 0
	}
	return remaining
}

// Fail records a rejected credential and schedules the next attempt.
func (l *Limiter) Fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	s, ok := l.sources[ip]
	if !ok {
		s = &source{}
		l.sources[ip] = s
	}
	s.failures++
	s.touched = now
	s.nextAllowed = now.Add(backoff(s.failures))
}

// Succeed forgets a source. Proving you know the password ends the episode: the
// operator who mistyped twice is not still paying for it an hour later.
func (l *Limiter) Succeed(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.sources, ip)
}

// Len reports how many sources are being remembered. It exists so a test can
// assert the map does not grow without bound.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return len(l.sources)
}

// sweep drops sources nobody has heard from in a while. It runs from Fail
// because Fail is the only thing that adds an entry, so the map cannot grow
// between sweeps. A background goroutine would do the same work while also
// needing a lifetime, a shutdown path and somewhere to be stopped in every test
// that builds a server.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < sweepInterval {
		return
	}
	l.lastSweep = now

	for ip, s := range l.sources {
		if now.Sub(s.touched) >= idleForget {
			delete(l.sources, ip)
		}
	}
}

// backoff is the delay after n consecutive failures: doubling from
// BaseLoginDelay, stopping at MaxLoginDelay.
//
// It is written as a loop rather than a shift so that a hundred failures is
// arithmetic that cannot overflow into a negative duration -- which would read
// as "no delay" and quietly undo the whole mechanism at exactly the point it
// matters most.
func backoff(failures int) time.Duration {
	if failures <= 0 {
		return 0
	}
	d := BaseLoginDelay
	for range failures - 1 {
		if d >= MaxLoginDelay {
			return MaxLoginDelay
		}
		d *= 2
	}
	if d > MaxLoginDelay {
		return MaxLoginDelay
	}
	return d
}
