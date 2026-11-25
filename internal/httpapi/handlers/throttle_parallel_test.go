package handlers_test

import (
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi/handlers"
)

// maxVerifiedInParallel is how many of a burst of simultaneous wrong passwords
// may reach the password check. One goes at once; the others wait the inline
// delay and race for the next slot, and the slot after that is a wait longer
// than the inline limit, which is a 429. Four leaves room for scheduling.
const maxVerifiedInParallel = 4

// burst sends n copies of one request at once and counts the answers.
func burst(t *testing.T, n int, send func() int) map[int]int {
	t.Helper()
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		codes = map[int]int{}
		start = make(chan struct{})
	)
	for range n {
		wg.Go(func() {
			<-start
			code := send()
			mu.Lock()
			codes[code]++
			mu.Unlock()
		})
	}
	close(start)
	wg.Wait()
	return codes
}

// TestParallelWrongLoginsAreNotAllChecked is the bypass the audit measured:
// the delay was read before the password check and the failure recorded after
// it, so twenty-four wrong logins sent at once all read "no delay" and all
// reached argon2id.
func TestParallelWrongLoginsAreNotAllChecked(t *testing.T) {
	s := newServer(t, 5*time.Minute)
	owner := s.newClient(t)
	owner.setup()

	guesser := s.newClient(t)
	codes := burst(t, 24, func() int {
		return guesser.login("wrong-password").StatusCode
	})

	if codes[http.StatusUnauthorized] > maxVerifiedInParallel {
		t.Fatalf("%d of 24 parallel wrong logins were checked, want at most %d (answers %v)",
			codes[http.StatusUnauthorized], maxVerifiedInParallel, codes)
	}
	if codes[http.StatusUnauthorized]+codes[http.StatusTooManyRequests] != 24 {
		t.Fatalf("answers other than 401 and 429: %v", codes)
	}
}

// TestParallelWrongReAuthenticationsAreNotAllChecked is the same bypass on
// the route that stands between a session cookie and a destructive action.
// Twenty-four stays under the per-peer bucket in front of the route, so what is
// measured is the password limiter and not the bucket.
func TestParallelWrongReAuthenticationsAreNotAllChecked(t *testing.T) {
	s := newServer(t, 5*time.Minute)
	owner := s.newClient(t)
	owner.setup()

	codes := burst(t, 24, func() int {
		resp, _ := owner.sudo("wrong-password")
		return resp.StatusCode
	})

	if codes[http.StatusUnauthorized] > maxVerifiedInParallel {
		t.Fatalf("%d of 24 parallel wrong re-authentications were checked, want at most %d (answers %v)",
			codes[http.StatusUnauthorized], maxVerifiedInParallel, codes)
	}
	if codes[http.StatusUnauthorized]+codes[http.StatusTooManyRequests] != 24 {
		t.Fatalf("answers other than 401 and 429: %v", codes)
	}
}

// TestTheReAuthenticationHasAPerPeerBucketInFrontOfIt: the route is behind a
// session, and the bucket every anonymous route has did not apply to it. A
// malformed body is answered before the password limiter is consulted, so a
// run of them shows the bucket alone.
func TestTheReAuthenticationHasAPerPeerBucketInFrontOfIt(t *testing.T) {
	s := newServer(t, 5*time.Minute)
	c := s.newClient(t)
	c.setup()

	for i := 1; i <= 40; i++ {
		resp, _ := c.doRaw(http.MethodPost, "/api/v1/auth/sudo", []byte("{"))
		if resp.StatusCode == http.StatusTooManyRequests {
			return
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: got %d, want 401 or 429", i, resp.StatusCode)
		}
	}
	t.Fatal("forty re-authentication requests from one peer never met the per-peer bucket")
}

// TestRepeatedWrongReAuthenticationEndsTheSession: a cookie thief guessing the
// sudo password at the backoff's pace could otherwise guess for as long as the
// session lives. After SudoFailuresBeforeSignOut wrong passwords the session is
// gone; signing in again with the password works at once and clears the
// account's counter.
func TestRepeatedWrongReAuthenticationEndsTheSession(t *testing.T) {
	var at atomic.Int64
	at.Store(time.Now().UnixNano())
	handlers.UseLimiterClock(t, func() time.Time { return time.Unix(0, at.Load()) })

	s := newServer(t, 5*time.Minute)
	c := s.newClient(t)
	c.setup()

	for i := 1; i <= handlers.SudoFailuresBeforeSignOut; i++ {
		if i > 1 {
			// Past the ceiling of the backoff, and short of the hour after
			// which the limiter forgets an idle source.
			at.Add(int64(31 * time.Second))
		}
		if resp, _ := c.do(http.MethodGet, "/api/v1/auth/me", nil); resp.StatusCode != http.StatusOK {
			t.Fatalf("before wrong password %d the session was already gone (%d)", i, resp.StatusCode)
		}
		resp, raw := c.sudo("wrong-password")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("wrong password %d: got %d, want 401 (body %s)", i, resp.StatusCode, raw)
		}
	}

	if resp, _ := c.do(http.MethodGet, "/api/v1/auth/me", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after %d wrong re-authentications the session still answers %d, want 401",
			handlers.SudoFailuresBeforeSignOut, resp.StatusCode)
	}

	// Signing in clears the account's counter: one more wrong password after
	// it is the first of a new run, not the eleventh of the old one.
	at.Add(int64(31 * time.Second))
	c.mustLogin()
	if resp, raw := c.sudo("wrong-password"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a wrong password after signing in again: got %d, want 401 (body %s)", resp.StatusCode, raw)
	}
	if resp, _ := c.do(http.MethodGet, "/api/v1/auth/me", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("one wrong password after signing in again ended the session (%d): "+
			"the sign-in did not clear the account's counter", resp.StatusCode)
	}
	at.Add(int64(time.Second))
	if resp, raw := c.sudo(testPass); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("after signing in again the correct password gives %d, want 204 (body %s)", resp.StatusCode, raw)
	}
}
