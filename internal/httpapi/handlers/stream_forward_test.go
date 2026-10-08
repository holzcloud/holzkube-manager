package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/streamhub"
)

// A forwarding goroutine whose reader has gone away must end. The handler stops
// draining the merged channel when it returns; a forwarder blocked in a bare
// send stays parked for the life of the process, once per abandoned stream.
func TestForwardFeedEndsWhenTheReaderIsGone(t *testing.T) {
	t.Parallel()

	items := make(chan streamhub.Item, 8)
	for range 8 {
		items <- streamhub.Item{}
	}
	merged := make(chan taggedItem, 1) // never drained

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		forwardFeed(ctx, "t", items, merged)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond) // let it fill merged and block
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the forwarder is still blocked after the request context ended")
	}
}
