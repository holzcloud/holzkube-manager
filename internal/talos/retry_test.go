package talos

// This is the one test file in the package that is not an external
// talos_test package. The backoff and the allowlist are unexported by
// intention -- a retry policy a caller can reach is a retry policy a caller can
// disagree with -- and the properties worth asserting about them are properties
// of the functions rather than of an exported surface. The behaviour these
// produce at the wire is asserted from outside, against talossim, in
// deadline_test.go and contract_test.go.

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRetryAllowlistIsExactlyTheFastReadClass is the derivation, not a
// transcription.
//
// internal/audit/redact.go's argument transferred to RPCs: a denylist of
// non-idempotent methods forgets the next mutating RPC a later phase adds, and
// a forgotten entry there is a retried Bootstrap. So the allowlist is checked
// against the class table rather than against a second hand-written list --
// which means a method moved into the fast read class without a decision about
// retrying it fails here.
func TestRetryAllowlistIsExactlyTheFastReadClass(t *testing.T) {
	t.Parallel()

	// The three deliberate exclusions. They are reads, and they are in the
	// fast read class, but each is expensive enough that a retry storm is its
	// own outage -- so they are named here and commented on their allowlist
	// entry rather than being quietly absent.
	excluded := map[string]bool{
		MethodEtcdSnapshot:  true,
		MethodPacketCapture: true,
		MethodDiskUsage:     true,
	}

	for method, class := range deadlineClasses {
		got := retryable[method]

		switch {
		case excluded[method]:
			if got {
				t.Errorf("%s is retryable; it is one of the three deliberate exclusions", method)
			}
		case class == ClassFastRead:
			if !got {
				t.Errorf("%s is in the fast read class and is not retryable; either it belongs on the "+
					"allowlist or it belongs in another class", method)
			}
		default:
			if got {
				t.Errorf("%s is classified %v and is retryable; only the fast read class is retried, "+
					"because a retried mutation is not idempotent and a retried stream silently "+
					"duplicates or drops data", method, class)
			}
		}
	}

	// The exclusions must actually be in the class table, or the loop above
	// would never have looked at them and this test would assert nothing about
	// them at all.
	for method := range excluded {
		if _, ok := deadlineClasses[method]; !ok {
			t.Errorf("%s is named as a deliberate exclusion and is not in the class table", method)
		}
	}

	// And the allowlist must not contain a method the class table has never
	// heard of, which would be an entry no policy covers.
	for method := range retryable {
		if _, ok := deadlineClasses[method]; !ok {
			t.Errorf("%s is on the retry allowlist and is not in the class table", method)
		}
	}
}

// TestRetryBackoffIsFullJitterInsideItsBounds.
//
// Full jitter means the delay is drawn uniformly from [0, base]: two clients
// that failed at the same instant must not come back at the same instant, which
// is the whole reason the jitter is full rather than partial. So the assertion
// has two halves -- every draw is inside its bound, and the draws are not all
// the same value. A fixed backoff would satisfy the first half alone.
func TestRetryBackoffIsFullJitterInsideItsBounds(t *testing.T) {
	t.Parallel()

	const draws = 200

	for attempt, bound := range map[int]time.Duration{
		1: RetryBackoffBase,
		2: 2 * RetryBackoffBase,
	} {
		distinct := map[time.Duration]bool{}

		for range draws {
			d := retryBackoff(attempt)
			if d < 0 || d > bound {
				t.Fatalf("retryBackoff(%d) = %v, outside [0, %v]", attempt, d, bound)
			}
			distinct[d] = true
		}

		if len(distinct) < 2 {
			t.Errorf("retryBackoff(%d) produced one value across %d draws; the jitter is not jitter, "+
				"and a fleet that failed together will come back together", attempt, draws)
		}
	}

	if retryBackoff(0) != 0 {
		t.Errorf("retryBackoff(0) = %v, want 0: there is no delay before the first attempt", retryBackoff(0))
	}
}

// TestRetryBackoffStopsAtItsCeiling pins the ceiling written into the confirmed
// policy.
//
// With the confirmed budget of two retries the two delays drawn are bounded by
// 200 ms and 400 ms, so 800 ms is a ceiling nothing reaches today. It is
// asserted anyway, because the day somebody raises RetryAttempts is the day the
// ceiling starts mattering, and a ceiling that was never tested is a ceiling
// that was never enforced. The doubling is written as a loop rather than a
// shift for the reason internal/auth/ratelimit.go gives: a large attempt count
// must be arithmetic that cannot overflow into a negative duration, which would
// read as "no delay" and undo the mechanism exactly when it matters most.
func TestRetryBackoffStopsAtItsCeiling(t *testing.T) {
	t.Parallel()

	for attempt := 1; attempt <= 64; attempt++ {
		if d := retryBackoff(attempt); d < 0 || d > RetryBackoffCeiling {
			t.Fatalf("retryBackoff(%d) = %v, outside [0, %v]", attempt, d, RetryBackoffCeiling)
		}
	}
}

// TestRetryAttemptsMatchesTheConfirmedPolicy.
func TestRetryAttemptsMatchesTheConfirmedPolicy(t *testing.T) {
	t.Parallel()

	if RetryAttempts != 3 {
		t.Errorf("RetryAttempts = %d, want 3 (at most two retries)", RetryAttempts)
	}
	if RetryBackoffBase != 200*time.Millisecond {
		t.Errorf("RetryBackoffBase = %v, want 200ms", RetryBackoffBase)
	}
	if RetryBackoffCeiling != 800*time.Millisecond {
		t.Errorf("RetryBackoffCeiling = %v, want 800ms", RetryBackoffCeiling)
	}
}

// TestRetryLoopAttemptCounts is the precise half of the retry proof.
//
// The end-to-end tests in deadline_test.go and contract_test.go show the loop
// behaving correctly at the wire, but they cannot count attempts: full jitter
// makes the elapsed time a distribution rather than a number, and gRPC's own
// reconnect backoff means a failed attempt does not necessarily redial. So the
// counts are asserted here, against the loop itself, where they are exact.
//
// Exact, though, only if the draw is pinned. Every row supplies the backoff the
// loop is to use, because a count asserted against a random delay is not a
// count: the budget row below spent a while asserting "50 ms buys no retry"
// against a delay drawn uniformly from [0, 200 ms], which is a statement that
// is true three times in four. The jitter itself is asserted by
// TestRetryBackoffIsFullJitterInsideItsBounds; what is asserted here is what
// the loop does with whatever it drew.
func TestRetryLoopAttemptCounts(t *testing.T) {
	t.Parallel()

	unreachable := &Error{Op: "Version", Kind: KindUnreachable}
	rejected := &Error{Op: "ApplyConfiguration", Kind: KindRejected}

	rows := []struct {
		name    string
		method  string
		budget  time.Duration
		backoff time.Duration
		fail    error
		want    int
		wantErr error

		// within, when set, bounds how long do may take to reach its verdict.
		// It is the half of the budget row that an attempt count cannot carry;
		// see the row for why.
		within time.Duration
	}{
		{
			name:    "an allowlisted read retries to its budget",
			method:  MethodVersion,
			budget:  9 * time.Second,
			backoff: time.Millisecond,
			fail:    unreachable,
			want:    RetryAttempts,
			wantErr: unreachable,
		},
		{
			name:    "a mutation is never retried",
			method:  MethodApplyConfiguration,
			budget:  9 * time.Second,
			backoff: time.Millisecond,
			fail:    unreachable,
			want:    1,
			// Bootstrap is the case this row exists for: a mutation whose
			// success committed and whose reply was lost would, if retried,
			// come back AlreadyExists and be reported as a failure of an
			// operation that in fact succeeded.
			wantErr: unreachable,
		},
		{
			name:    "a stream is never retried",
			method:  MethodLogs,
			budget:  9 * time.Second,
			backoff: time.Millisecond,
			fail:    unreachable,
			want:    1,
			wantErr: unreachable,
		},
		{
			name:    "a deliberate read exclusion is never retried",
			method:  MethodEtcdSnapshot,
			budget:  9 * time.Second,
			backoff: time.Millisecond,
			fail:    unreachable,
			want:    1,
			wantErr: unreachable,
		},
		{
			name:    "an answered refusal is never retried",
			method:  MethodVersion,
			budget:  9 * time.Second,
			backoff: time.Millisecond,
			fail:    rejected,
			want:    1,
			wantErr: rejected,
		},
		{
			// The budget row, and the one that needs both assertions.
			//
			// A count alone does not distinguish budgetAllows from the
			// cancellation escape one line below it: a loop that slept anyway
			// would be woken by ctx.Done and would also report one attempt. The
			// difference is *when* the caller is told. budgetAllows answers at
			// once because it compares the delay against the remaining budget;
			// the cancellation escape answers only when the budget runs out. So
			// the row asserts one attempt and a verdict inside half the budget,
			// and stubbing budgetAllows to true fails it on the second clause.
			name:    "a backoff longer than the remaining budget buys no retry, and says so at once",
			method:  MethodVersion,
			budget:  500 * time.Millisecond,
			backoff: RetryBackoffCeiling,
			fail:    unreachable,
			want:    1,
			wantErr: unreachable,
			within:  250 * time.Millisecond,
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), row.budget)
			defer cancel()

			n := &conn{now: time.Now, backoff: func(int) time.Duration { return row.backoff }}

			attempts := 0
			started := time.Now()
			err := n.do(ctx, row.method, func(context.Context) error {
				attempts++
				return row.fail
			})
			elapsed := time.Since(started)

			if attempts != row.want {
				t.Errorf("the loop made %d attempt(s), want %d", attempts, row.want)
			}
			if !errors.Is(err, row.wantErr) {
				t.Errorf("do returned %v, want the call's own error", err)
			}
			if row.within > 0 && elapsed > row.within {
				t.Errorf("do took %v to give up, want under %v: it waited out the deadline instead of "+
					"noticing that a %v backoff does not fit in a %v budget",
					elapsed, row.within, row.backoff, row.budget)
			}
		})
	}
}

// TestRetryLoopStopsOnSuccess covers the one path the table above cannot: a
// call that fails and then works. Without it the loop could return the first
// error unconditionally and every row above would still pass.
func TestRetryLoopStopsOnSuccess(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 9*time.Second)
	defer cancel()

	n := &conn{now: time.Now, backoff: func(int) time.Duration { return time.Millisecond }}

	attempts := 0
	err := n.do(ctx, MethodVersion, func(context.Context) error {
		attempts++
		if attempts == 1 {
			return &Error{Op: "Version", Kind: KindUnreachable}
		}
		return nil
	})

	if err != nil {
		t.Errorf("do returned %v after the second attempt succeeded", err)
	}
	if attempts != 2 {
		t.Errorf("the loop made %d attempt(s), want 2: it kept going after a success", attempts)
	}
}
