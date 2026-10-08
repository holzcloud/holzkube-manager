package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// AnonLimit bounds how fast one peer may call routes that need no session and
// leave something behind: an audit pair that is never deleted, or a session
// file. Without it a scanner posting to /auth/login at line speed writes two
// fsync'd, hash-chained records per request into the archive and fills the
// card of a Raspberry Pi.
//
// A token bucket per peer address, keyed on the TCP peer and nothing else for
// the reason the login throttle gives: a forwarded header is chosen by the
// caller. The bucket is generous -- a person mistyping a password does not meet
// it -- and it is separate from the login throttle, which slows guessing but
// sits behind the audit link.
func AnonLimit(burst int, refill time.Duration, deny func(http.ResponseWriter, *http.Request, time.Duration)) Middleware {
	l := &anonBuckets{burst: float64(burst), refill: refill, byPeer: map[string]*bucket{}}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if wait := l.take(ClientIP(r), time.Now()); wait > 0 {
				if deny != nil {
					deny(w, r, wait)
					return
				}
				w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type bucket struct {
	tokens float64
	at     time.Time
}

type anonBuckets struct {
	mu     sync.Mutex
	burst  float64
	refill time.Duration
	byPeer map[string]*bucket
}

// maxAnonPeers caps the table: a scanner rotating source addresses must not
// turn the limiter into the memory problem it exists to prevent.
const maxAnonPeers = 4096

func (l *anonBuckets) take(peer string, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.byPeer[peer]
	if !ok {
		if len(l.byPeer) >= maxAnonPeers {
			for k, v := range l.byPeer {
				if now.Sub(v.at) > l.refill*time.Duration(l.burst) {
					delete(l.byPeer, k)
				}
			}
			if len(l.byPeer) >= maxAnonPeers {
				clear(l.byPeer)
			}
		}
		b = &bucket{tokens: l.burst, at: now}
		l.byPeer[peer] = b
	}
	b.tokens += float64(now.Sub(b.at)) / float64(l.refill)
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.at = now
	if b.tokens < 1 {
		return time.Duration((1 - b.tokens) * float64(l.refill))
	}
	b.tokens--
	return 0
}
