package audit

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestPendingDoesNotGrowForever covers the panic path. The audit middleware
// deliberately does not defer the outcome write, so an attempt whose handler
// panicked never gets one and its correlation entry stays behind holding a
// whole Record. Every panic-triggering input is then a repeatable allocation a
// caller with a session can drive.
func TestPendingDoesNotGrowForever(t *testing.T) {
	dir := t.TempDir()
	clk := newClock(t)
	l := openAt(t, dir, clk)
	defer l.Close()

	// Ten attempts nobody ever completes, as ten panicking requests would
	// leave behind.
	for i := range 10 {
		if _, err := l.Attempt(context.Background(), Record{
			Actor:  "holz",
			Action: fmt.Sprintf("account.password%d", i),
		}); err != nil {
			t.Fatalf("attempt: %v", err)
		}
	}
	if got := len(l.pending); got != 10 {
		t.Fatalf("pending = %d, want the 10 un-outcomed attempts", got)
	}

	// Well past the TTL, one more request arrives.
	clk.t = clk.t.Add(pendingTTL + time.Minute)
	seq, err := l.Attempt(context.Background(), Record{Actor: "holz", Action: "auth.login"})
	if err != nil {
		t.Fatalf("attempt: %v", err)
	}

	if got := len(l.pending); got != 1 {
		t.Errorf("pending = %d after the TTL elapsed, want only the live attempt", got)
	}
	if _, ok := l.pending[seq]; !ok {
		t.Error("the live attempt was evicted; its outcome can no longer be correlated")
	}

	// The live one still pairs up.
	if err := l.Outcome(context.Background(), seq, OutcomeSuccess, nil); err != nil {
		t.Errorf("Outcome on the live attempt: %v", err)
	}
	if got := len(l.pending); got != 0 {
		t.Errorf("pending = %d after the outcome, want 0", got)
	}
}
