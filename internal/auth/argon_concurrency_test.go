package auth

import (
	"testing"
	"time"
)

// TestVerifyWaitsForAFreeSlot: with every slot taken, Verify must queue rather
// than allocate another 64 MiB. Not parallel: it occupies the package's slots.
func TestVerifyWaitsForAFreeSlot(t *testing.T) {
	for range cap(hashSlots) {
		hashSlots <- struct{}{}
	}
	done := make(chan struct{})
	go func() {
		_, _ = Verify("x", "not-a-hash")
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Verify ran with every slot taken")
	case <-time.After(150 * time.Millisecond):
	}
	for range cap(hashSlots) {
		<-hashSlots
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Verify never ran after the slots were freed")
	}
}
