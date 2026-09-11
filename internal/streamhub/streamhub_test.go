package streamhub_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/streamhub"
)

const topic = streamhub.Topic("logs:node-1:kubelet")

// drain reads everything currently available without blocking.
func drain(ch <-chan streamhub.Item) []streamhub.Item {
	var out []streamhub.Item
	for {
		select {
		case item, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, item)
		default:
			return out
		}
	}
}

func TestSubscriberReceivesWhatIsPublished(t *testing.T) {
	t.Parallel()

	h := streamhub.New()
	defer h.Close() //nolint:errcheck // test cleanup

	ch, cancel, err := h.Subscribe(topic, 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	for i := range 3 {
		if _, err := h.Publish(topic, []byte(fmt.Sprintf("line %d", i))); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	items := drain(ch)
	if len(items) != 3 {
		t.Fatalf("received %d items, want 3", len(items))
	}
	for i, item := range items {
		if item.Event == nil {
			t.Fatalf("item %d is a gap, want an event", i)
		}
		if item.Event.ID != uint64(i+1) {
			t.Errorf("item %d has id %d, want %d", i, item.Event.ID, i+1)
		}
	}
}

// TestASlowSubscriberNeverBlocksThePublisher is the property the whole package
// exists for: the publisher is a goroutine reading from a Talos node, and a
// browser tab that stopped reading must not be able to apply backpressure to
// a node.
func TestASlowSubscriberNeverBlocksThePublisher(t *testing.T) {
	t.Parallel()

	h := streamhub.NewWithSizes(4, 2)
	defer h.Close() //nolint:errcheck // test cleanup

	_, cancel, err := h.Subscribe(topic, 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	// Nobody reads that channel. Publishing far more than it can hold must
	// still return promptly.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 10_000 {
			if _, err := h.Publish(topic, []byte(fmt.Sprintf("line %d", i))); err != nil {
				t.Errorf("Publish: %v", err)
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Publish blocked on a subscriber that is not reading; " +
			"one stalled browser tab would apply backpressure to a Talos node")
	}
}

// TestFallingBehindProducesAVisibleGap is the other half of the bargain. A
// bounded buffer must drop something eventually; what it must never do is drop
// it silently. An invisible omission in a log somebody is using to diagnose an
// outage produces confident wrong conclusions.
func TestFallingBehindProducesAVisibleGap(t *testing.T) {
	t.Parallel()

	h := streamhub.NewWithSizes(4, 2)
	defer h.Close() //nolint:errcheck // test cleanup

	ch, cancel, err := h.Subscribe(topic, 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	for i := range 200 {
		if _, err := h.Publish(topic, []byte(fmt.Sprintf("line %d", i))); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	items := drain(ch)
	if len(items) == 0 {
		t.Fatal("the subscriber received nothing at all")
	}

	var gaps, events int
	var missed uint64
	for _, item := range items {
		switch {
		case item.Gap != nil:
			gaps++
			missed += item.Gap.Missed
		case item.Event != nil:
			events++
		}
	}

	if gaps == 0 {
		t.Fatal("200 events into a buffer that holds a handful produced no gap; " +
			"the subscriber was silently truncated")
	}
	if missed == 0 {
		t.Error("the gap reports zero missed events")
	}
}

// TestReplayAfterLastEventID is the reconnect: a browser that dropped its
// connection asks for what it missed and gets it, rather than resuming with a
// hole it cannot see.
func TestReplayAfterLastEventID(t *testing.T) {
	t.Parallel()

	h := streamhub.New()
	defer h.Close() //nolint:errcheck // test cleanup

	for i := range 10 {
		if _, err := h.Publish(topic, []byte(fmt.Sprintf("line %d", i))); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	// The client saw up to id 4 before its connection dropped.
	ch, cancel, err := h.Subscribe(topic, 4)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	items := drain(ch)
	if len(items) != 6 {
		t.Fatalf("replay delivered %d items, want ids 5..10", len(items))
	}
	for i, item := range items {
		if item.Gap != nil {
			t.Fatalf("item %d is a gap, but the buffer still held everything the client missed", i)
		}
		if want := uint64(i + 5); item.Event.ID != want {
			t.Errorf("item %d has id %d, want %d", i, item.Event.ID, want)
		}
	}
}

// TestReplayBeyondTheBufferIsAGapAndNotAContinuation is the case that is easy
// to get wrong and expensive to get wrong: a client away longer than the ring
// must be told, not handed a continuation that quietly skips.
func TestReplayBeyondTheBufferIsAGapAndNotAContinuation(t *testing.T) {
	t.Parallel()

	h := streamhub.NewWithSizes(5, 8)
	defer h.Close() //nolint:errcheck // test cleanup

	for i := range 100 {
		if _, err := h.Publish(topic, []byte(fmt.Sprintf("line %d", i))); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	// The client last saw id 2; the buffer now starts at 96.
	ch, cancel, err := h.Subscribe(topic, 2)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	items := drain(ch)
	if len(items) == 0 {
		t.Fatal("nothing was delivered")
	}
	if items[0].Gap == nil {
		t.Fatal("the first item is not a gap; the client was handed a continuation " +
			"that silently skips 93 events")
	}
	if items[0].Gap.Missed != 93 {
		t.Errorf("the gap reports %d missed, want 93 (ids 3..95)", items[0].Gap.Missed)
	}
}

// TestOneUpstreamManyPanels is the multiplexing claim: several subscribers on
// one topic all see the same events, and the producer publishes once.
func TestOneUpstreamManyPanels(t *testing.T) {
	t.Parallel()

	h := streamhub.New()
	defer h.Close() //nolint:errcheck // test cleanup

	const panels = 4
	chans := make([]<-chan streamhub.Item, panels)
	for i := range panels {
		ch, cancel, err := h.Subscribe(topic, 0)
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		defer cancel()
		chans[i] = ch
	}

	if got := h.Subscribers(topic); got != panels {
		t.Fatalf("Subscribers = %d, want %d", got, panels)
	}

	if _, err := h.Publish(topic, []byte("one line")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	for i, ch := range chans {
		items := drain(ch)
		if len(items) != 1 || items[0].Event == nil {
			t.Fatalf("panel %d received %v, want one event", i, items)
		}
		if string(items[0].Event.Data) != "one line" {
			t.Errorf("panel %d received %q", i, items[0].Event.Data)
		}
	}
}

func TestCancelClosesTheChannelAndUnsubscribes(t *testing.T) {
	t.Parallel()

	h := streamhub.New()
	defer h.Close() //nolint:errcheck // test cleanup

	ch, cancel, err := h.Subscribe(topic, 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	cancel()
	// Idempotent: an HTTP handler's defer and an explicit close can both run.
	cancel()

	if _, ok := <-ch; ok {
		t.Error("the channel is still open after cancel")
	}
	if got := h.Subscribers(topic); got != 0 {
		t.Errorf("Subscribers = %d after cancel, want 0", got)
	}
}

// TestConcurrentPublishAndSubscribe is the race detector's business. The hub
// is driven from a reader goroutine per topic and from one HTTP handler per
// browser tab, all at once.
func TestConcurrentPublishAndSubscribe(t *testing.T) {
	t.Parallel()

	h := streamhub.New()
	defer h.Close() //nolint:errcheck // test cleanup

	var wg sync.WaitGroup
	for p := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 200 {
				if _, err := h.Publish(streamhub.Topic(fmt.Sprintf("t%d", p)), []byte(fmt.Sprint(i))); err != nil {
					return
				}
			}
		}()
	}
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range 4 {
				ch, cancel, err := h.Subscribe(streamhub.Topic(fmt.Sprintf("t%d", p)), 0)
				if err != nil {
					return
				}
				drain(ch)
				cancel()
			}
		}()
	}
	wg.Wait()
}
