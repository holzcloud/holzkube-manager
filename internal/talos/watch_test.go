package talos_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestAWatchSaysItHasStartedBeforeItSaysAnythingElse is the property criterion
// 4 of this phase turns on.
//
// A COSI watch opened with bootstrap contents delivers everything the node
// already holds as Created events before it delivers the Bootstrapped marker.
// Forwarding those upward would mean every node begins its subscription by
// announcing six changes that are not changes -- and the caller, which answers
// a change by re-reading the node, would answer each of them with a full read.
//
// The stronger half is what it means for freshness: a watch that has not yet
// said "this is what the node has now" is a subscription that has not started,
// which is a different thing from a node on which nothing is happening. A
// caller that could not tell those apart would report a node as watched from
// the moment the stream was constructed.
func TestAWatchSaysItHasStartedBeforeItSaysAnythingElse(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "watched-1", ControlPlane: true})
	cc := newClusterClient(t, sim)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	w := cc.Watch(ctx)
	defer w.Close() //nolint:errcheck // Close only cancels and waits

	first, ok := recvEvent(t, w, 15*time.Second)
	if !ok {
		t.Fatal("the watch delivered nothing; a subscription opened with bootstrap contents owes an " +
			"answer immediately, and silence here is a watch that never started")
	}
	if !first.Snapshot {
		t.Fatalf("the first event was a change on %q rather than the snapshot marker; the node's "+
			"existing resources are being forwarded as if somebody had just changed them", first.Topic)
	}

	// And now a real change, which must arrive and must name the topic it
	// happened on.
	if err := sim.SetHostname(ctx, "renamed-1"); err != nil {
		t.Fatalf("SetHostname: %v", err)
	}

	ev, ok := recvEvent(t, w, 15*time.Second)
	if !ok {
		t.Fatal("a hostname change produced no watch event, so the subscription is open and deaf")
	}
	if ev.Topic != talos.TopicHostname {
		t.Errorf("the hostname change arrived as topic %q, want %q", ev.Topic, talos.TopicHostname)
	}
	if ev.Snapshot {
		t.Error("a change arrived marked as the snapshot; the marker is emitted once and a second " +
			"one would tell a caller the watch had restarted when it had not")
	}
}

// TestAWatchDoesNotNoticeANodeThatVanishes pins the assumption the whole
// watch/heartbeat pairing rests on.
//
// It asserts a *deficiency*, on purpose. A subscription whose node is stopped
// outright stays open, silent and errorless: cosi-project's protobuf client
// re-establishes a broken watch stream from its last bookmark on an internal
// exponential backoff and reports nothing upward until that budget runs out
// about fifteen minutes later. ClassWatch removes the idle timeout that would
// otherwise have caught it, and removing it was right -- silence on a watch is
// a node on which nothing happened -- but it leaves this package with no way at
// all to tell a quiet node from a dead one.
//
// That is why internal/inventory keeps the heartbeat and closes a watch the
// poller has stopped believing in. If a future machinery or cosi-project
// release starts surfacing the failure here, this test goes red, and whoever
// sees it should re-read that design rather than delete the assertion: the
// watchdog would then be redundant, which is a thing worth knowing.
func TestAWatchDoesNotNoticeANodeThatVanishes(t *testing.T) {
	t.Parallel()

	sim, err := talossim.New(talossim.Options{Hostname: "vanishing-1", ControlPlane: true})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	cc, err := talos.NewClusterClient(ctx, sim.Dialer(),
		simTarget(sim, "00000000-0000-0000-0000-0000000000dd"), sim.ClientCreds(), talos.Mode{})
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}
	defer cc.Close() //nolint:errcheck // the node is about to be gone anyway

	w := cc.Watch(ctx)
	defer w.Close() //nolint:errcheck // Close only cancels and waits

	if first, ok := recvEvent(t, w, 15*time.Second); !ok || !first.Snapshot {
		t.Fatal("the watch did not start, so what this test goes on to observe would be nothing")
	}

	if err := sim.Close(); err != nil {
		t.Fatalf("talossim.Close: %v", err)
	}

	// Long enough that a transport-level notice would have arrived -- the
	// first-byte deadline and the idle timeout are both ten seconds and a
	// minute -- and far short of the fifteen minutes the retry budget runs for.
	select {
	case _, open := <-w.Events():
		if !open {
			t.Fatal("the watch ended by itself when its node was stopped. That is better than what " +
				"was measured when this pairing was designed, and it means internal/inventory's " +
				"heartbeat watchdog is now belt and braces rather than the only liveness signal -- " +
				"re-read the note at the top of watch.go before changing either")
		}
		t.Fatal("a stopped node delivered a watch event")
	case <-time.After(15 * time.Second):
	}

	if err := w.Err(); err != nil {
		t.Fatalf("the watch reported %v for a node that was stopped; see the paragraph above", err)
	}
}

// TestTheTopicVocabularyIsWellFormed checks the set itself.
//
// That every topic names a kind a node will actually serve is proved by the
// test above, which cannot see its snapshot marker until all of them have
// delivered -- the machine configuration was dropped from the table because
// exactly that assertion caught it.
func TestTheTopicVocabularyIsWellFormed(t *testing.T) {
	t.Parallel()

	topics := talos.WatchTopics()
	if len(topics) == 0 {
		t.Fatal("there are no watch topics, so every node's subscription carries nothing")
	}
	if !slices.IsSorted(topics) {
		t.Error("WatchTopics is not sorted; a set that comes back in a different order each run " +
			"makes a fleet where two nodes subscribe in a different sequence and nobody can say why")
	}
	if slices.Contains(topics, talos.WatchTopic("")) {
		t.Error("the topic set contains the empty topic, which is the value WatchEvent uses to mean " +
			"'this is the snapshot marker'")
	}
}

func recvEvent(t *testing.T, w *talos.Watch, within time.Duration) (talos.WatchEvent, bool) {
	t.Helper()

	select {
	case ev, ok := <-w.Events():
		if !ok {
			t.Fatalf("the watch ended while a test was waiting for an event: %v", w.Err())
		}
		return ev, true
	case <-time.After(within):
		return talos.WatchEvent{}, false
	}
}
