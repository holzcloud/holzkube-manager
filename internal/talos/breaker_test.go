package talos

// Internal, like retry_test.go, and for the same reason: the breaker carries an
// injected clock so that a thirty-second cooldown and a one-hour retention
// window can be walked out without waiting them, and that clock is unexported
// because a caller who can move it is a caller who can disable the breaker.
// internal/auth/ratelimit_test.go tests its Limiter the same way.

import (
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

const testMachine = model.MachineID("00000000-0000-0000-0000-0000000000b1")

// newTestBreaker returns a breaker whose clock the test drives.
func newTestBreaker() (*Breaker, *time.Time) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	b := NewBreaker()
	b.now = func() time.Time { return now }
	return b, &now
}

// TestBreakerOpensAfterConsecutiveTransportFailures.
func TestBreakerOpensAfterConsecutiveTransportFailures(t *testing.T) {
	t.Parallel()

	b, _ := newTestBreaker()

	for i := 1; i < BreakerFailureThreshold; i++ {
		b.Fail(testMachine)
		if err := b.Allow(testMachine); err != nil {
			t.Fatalf("the circuit opened after %d failure(s): %v; one dropped packet must not take a "+
				"node out of the inventory", i, err)
		}
	}

	b.Fail(testMachine)

	err := b.Allow(testMachine)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("Allow after %d failures = %v, want ErrCircuitOpen", BreakerFailureThreshold, err)
	}
	if !strings.Contains(err.Error(), string(testMachine)) {
		t.Errorf("the refusal %q does not name the machine being skipped", err)
	}
}

// TestBreakerDistinguishesARefusalFromATransportFailure is the etcd_down case,
// and it is the reason the classifier exists.
//
// A node answering codes.Unavailable for one subsystem is reachable. Counting
// that against its circuit would turn one broken subsystem into a node-wide
// outage, which is a strictly worse failure than the one being reported.
func TestBreakerDistinguishesARefusalFromATransportFailure(t *testing.T) {
	t.Parallel()

	b, _ := newTestBreaker()

	rejected := &Error{Op: "EtcdMemberList", Machine: testMachine, Kind: KindRejected, Status: codes.Unavailable}
	unreachable := &Error{Op: "Version", Machine: testMachine, Kind: KindUnreachable, Status: codes.Unavailable}
	timedOut := &Error{Op: "Version", Machine: testMachine, Kind: KindTimeout}

	// Both errors carry the same gRPC status code. Only the classification
	// tells them apart, which is the whole point.
	for range 10 {
		b.Observe(testMachine, rejected)
	}
	if got := b.Failures(testMachine); got != 0 {
		t.Errorf("ten refusals advanced the failure count to %d, want 0", got)
	}
	if err := b.Allow(testMachine); err != nil {
		t.Errorf("the circuit opened on refusals alone: %v", err)
	}

	// A refusal in the middle of a run of transport failures leaves the count
	// where it was: it is not evidence that the calls which were timing out
	// have started working.
	b.Observe(testMachine, unreachable)
	b.Observe(testMachine, rejected)
	if got := b.Failures(testMachine); got != 1 {
		t.Errorf("failure count = %d after one transport failure and one refusal, want 1", got)
	}

	// A timeout is a transport failure: the node did not answer. go_silent is
	// exactly this shape, and its documented expectation is that the breaker
	// opens.
	b.Observe(testMachine, timedOut)
	b.Observe(testMachine, timedOut)
	if err := b.Allow(testMachine); !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("Allow = %v after %d transport failures, want ErrCircuitOpen", err, BreakerFailureThreshold)
	}

	// And a cancellation is not a failure of the node at all.
	b.Success(testMachine)
	b.Observe(testMachine, errors.New("plain error, never classified"))
	if got := b.Failures(testMachine); got != 0 {
		t.Errorf("an unclassified error advanced the failure count to %d, want 0", got)
	}
}

// TestBreakerHalfOpensAfterTheCooldown.
func TestBreakerHalfOpensAfterTheCooldown(t *testing.T) {
	t.Parallel()

	b, clock := newTestBreaker()

	for range BreakerFailureThreshold {
		b.Fail(testMachine)
	}
	if err := b.Allow(testMachine); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("Allow = %v, want ErrCircuitOpen", err)
	}

	*clock = clock.Add(BreakerCooldown)

	if err := b.Allow(testMachine); err != nil {
		t.Fatalf("Allow after the cooldown = %v, want one trial call through", err)
	}

	// One trial, not one per goroutine. A second caller arriving in the same
	// instant is still refused, because the trial has restarted the cooldown.
	if err := b.Allow(testMachine); !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("Allow = %v for the second caller in the half-open window, want ErrCircuitOpen: "+
			"a window that admits every waiting goroutine is not a trial", err)
	}

	t.Run("a failing trial re-opens the circuit", func(t *testing.T) {
		b.Fail(testMachine)
		if err := b.Allow(testMachine); !errors.Is(err, ErrCircuitOpen) {
			t.Errorf("Allow = %v after a failed trial, want ErrCircuitOpen", err)
		}
	})

	t.Run("a succeeding trial resets it", func(t *testing.T) {
		b.Success(testMachine)
		if err := b.Allow(testMachine); err != nil {
			t.Errorf("Allow = %v after a successful trial, want nil", err)
		}
		if got := b.Failures(testMachine); got != 0 {
			t.Errorf("failure count = %d after a success, want 0", got)
		}
	})
}

// TestBreakerDoesNotGrowWithoutBound is T-02-30.
//
// The sweep runs from Fail because Fail is the only method that adds an entry,
// so the map cannot grow between sweeps. Len exists for exactly this assertion,
// in the same words internal/auth.Limiter.Len uses.
func TestBreakerDoesNotGrowWithoutBound(t *testing.T) {
	t.Parallel()

	b, clock := newTestBreaker()

	const machines = 500
	for i := range machines {
		b.Fail(model.MachineID(machineID(i)))
	}
	if got := b.Len(); got != machines {
		t.Fatalf("Len = %d after %d machines failed, want %d", got, machines, machines)
	}

	// Move past the retention window and touch one machine, which is what runs
	// the sweep.
	*clock = clock.Add(breakerRetention + time.Minute)
	b.Fail(testMachine)

	if got := b.Len(); got != 1 {
		t.Errorf("Len = %d after a sweep, want 1: %d machines nobody had heard from in over an hour "+
			"were still being remembered", got, machines)
	}
}

func machineID(i int) string {
	const digits = "0123456789abcdef"
	out := []byte("00000000-0000-0000-0000-000000000000")
	for pos := len(out) - 1; pos >= 0 && i > 0; pos-- {
		if out[pos] == '-' {
			continue
		}
		out[pos] = digits[i%16]
		i /= 16
	}
	return string(out)
}
