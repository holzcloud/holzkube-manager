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

// NewLimiterWithClock returns an empty limiter reading the time from now, so a
// test outside this package can walk the backoff out without waiting it.
func NewLimiterWithClock(now func() time.Time) *Limiter {
	l := NewLimiter()
	l.now = now
	return l
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

// Reserve is the check and the charge of one password attempt, in one critical
// section.
//
// Delay followed by Fail had a gap the length of a password verification
// between the question and the answer, and nothing counted the attempts inside
// it: twenty-four wrong logins sent at once all read "no delay" and all reached
// argon2id, and the backoff only began after the last of them came back. So an
// attempt is now charged as a failure before the password is looked at -- the
// pessimistic bookkeeping -- and refunded by the outcome: Succeed forgets the
// keys, Cancel takes the charge back when no password was checked at all.
//
// keys are every counter this attempt counts against; the attempt waits for the
// slowest of them and charges all of them. When any key must still wait, Reserve
// charges nothing and reports how long, and the Reservation is nil.
func (l *Limiter) Reserve(keys ...string) (time.Duration, *Reservation) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	var wait time.Duration
	for _, k := range keys {
		if s, ok := l.sources[k]; ok {
			wait = max(wait, s.nextAllowed.Sub(now))
		}
	}
	if wait > 0 {
		return wait, nil
	}

	l.sweep(now)
	for _, k := range keys {
		s, ok := l.sources[k]
		if !ok {
			s = &source{}
			l.sources[k] = s
		}
		s.failures++
		s.touched = now
		s.nextAllowed = now.Add(backoff(s.failures))
	}
	return 0, &Reservation{l: l, keys: keys}
}

// Failures reports how many failures in a row a key has on record, in-flight
// reservations included.
func (l *Limiter) Failures(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	if s, ok := l.sources[key]; ok {
		return s.failures
	}
	return 0
}

// Reservation is one attempt Reserve let through. Exactly one of Fail, Succeed
// or Cancel ends it; the later calls do nothing.
type Reservation struct {
	l    *Limiter
	keys []string
	once sync.Once
}

// Fail keeps the charge, and restarts the wait from now: the delay a failure
// earns is counted from when the answer was given, as Fail always did, and not
// from when the attempt began.
func (r *Reservation) Fail() {
	r.once.Do(func() {
		r.l.mu.Lock()
		defer r.l.mu.Unlock()

		now := r.l.now()
		for _, k := range r.keys {
			if s, ok := r.l.sources[k]; ok {
				s.touched = now
				if next := now.Add(backoff(s.failures)); next.After(s.nextAllowed) {
					s.nextAllowed = next
				}
			}
		}
	})
}

// Succeed forgets every key of the attempt, as Limiter.Succeed does.
func (r *Reservation) Succeed() {
	r.once.Do(func() {
		r.l.mu.Lock()
		defer r.l.mu.Unlock()

		for _, k := range r.keys {
			delete(r.l.sources, k)
		}
	})
}

// Cancel takes the charge back for an attempt that checked no password -- an
// internal error, a client gone away. The wait it scheduled stays: undoing it
// exactly would need the state from before, and a few hundred milliseconds too
// many after an error is not worth that.
func (r *Reservation) Cancel() {
	r.once.Do(func() {
		r.l.mu.Lock()
		defer r.l.mu.Unlock()

		for _, k := range r.keys {
			s, ok := r.l.sources[k]
			if !ok {
				continue
			}
			s.failures--
			if s.failures <= 0 {
				delete(r.l.sources, k)
			}
		}
	})
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
