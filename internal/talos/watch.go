package talos

// Watching a node's resource state (INV-13, D-19).
//
// The heartbeat in internal/inventory reads COSI resources on a timer, which
// satisfies INV-13's actual prohibition -- the facts come from the state Talos
// already maintains rather than from unary RPCs asked blindly -- and leaves the
// latency. A change made now is visible in about twenty-two seconds on average,
// which is the wrong answer to "did my config apply land?".
//
// What this file adds is the *when*. A watch carries no data upward: it says a
// topic changed, and the observation pass that already exists does the reading.
// That is a deliberate split rather than a shortcut. The alternative -- a watch
// that assembles the snapshot from the events it receives -- would be a second
// implementation of every derivation in facts.go, kept in step with the first
// by nothing, and the first divergence between them would be a dashboard that
// disagrees with itself depending on which path last wrote.
//
// It is also why the heartbeat stays. A watch is a subscription, and the
// characteristic failure of a subscription is that it stops delivering without
// saying so; the poll behind it is what turns that from a silent freeze into a
// stale_since that moves.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/state"

	"github.com/siderolabs/talos/pkg/machinery/resources/block"
	"github.com/siderolabs/talos/pkg/machinery/resources/network"
	runtimeres "github.com/siderolabs/talos/pkg/machinery/resources/runtime"
)

// WatchTopic is a kind of resource, named in holzkube-manager's vocabulary.
//
// The names are the seam's and not machinery's, for the reason facts.go gives:
// a caller that had to say network.NodeAddressType would be a caller that knows
// machinery's resource types, and the seam would be a comment again.
type WatchTopic string

const (
	// TopicHostname is what the node calls itself.
	TopicHostname WatchTopic = "hostname"

	// TopicAddresses is the node's addresses.
	//
	// It is the topic with the sharpest consequence: an address that moved is
	// an address at which this manager can no longer reach the node, and D-10's
	// whole eviction dance hangs off noticing. A DHCP rotation that takes a
	// heartbeat to be seen is a node that is briefly attributed to the wrong
	// record.
	TopicAddresses WatchTopic = "addresses"

	// TopicLinks is the node's network interfaces.
	TopicLinks WatchTopic = "links"

	// TopicDisks is the node's block devices. It changes when somebody plugs
	// something in, which is a thing that happens while somebody is looking at
	// the screen they plugged it in for.
	TopicDisks WatchTopic = "disks"

	// TopicExtensions is the node's system extensions, which is where a
	// Factory image carries its schematic id. It changes on an upgrade.
	TopicExtensions WatchTopic = "extensions"
)

// watchKinds is the translation, in one place.
//
// Two absences are deliberate and both were decided by running the thing.
//
// The service list is absent even though it is the most volatile thing on a
// node: this build reads services through the ServiceList RPC rather than
// through the v1alpha1.Service resource, so a watch on it would be a
// notification that nothing on the read path corresponds to. Adding it means
// moving the read first.
//
// The machine configuration is absent because it cannot be watched over this
// transport. config.MachineConfig's spec is decoded by handing its bytes to
// machinery's configloader, and a WatchKind opened with bootstrap contents ends
// with an envelope carrying an *empty* resource of the kind -- which
// configloader rejects with "config not found", the whole subscription
// failing at the marker that was supposed to say it had started. It is not a
// property of the simulator: the failing decode happens in the client, against
// bytes the cosi-project state server sends the same way to everybody.
// Watching without bootstrap contents avoids the decode and buys nothing, since
// a topic that never delivers a snapshot can never report itself started and a
// stream that stays silent runs into StreamFirstByteDeadline. So the
// configuration is the heartbeat's, and the apply that changes it has its own
// job to watch -- which is where an operator applying a configuration is
// looking anyway.
var watchKinds = map[WatchTopic]resource.Kind{
	TopicHostname:   resource.NewMetadata(network.NamespaceName, network.HostnameStatusType, "", resource.VersionUndefined),
	TopicAddresses:  resource.NewMetadata(network.NamespaceName, network.NodeAddressType, "", resource.VersionUndefined),
	TopicLinks:      resource.NewMetadata(network.NamespaceName, network.LinkStatusType, "", resource.VersionUndefined),
	TopicDisks:      resource.NewMetadata(block.NamespaceName, block.DiskType, "", resource.VersionUndefined),
	TopicExtensions: resource.NewMetadata(runtimeres.NamespaceName, runtimeres.ExtensionStatusType, "", resource.VersionUndefined),
}

// WatchTopics returns every topic a watch can carry, in a stable order.
//
// There is no subset for a caller to choose. Each topic costs one server stream
// per node, and at the five-to-twenty nodes Pattern 7 sizes this product for
// that is a bounded number of cheap subscriptions; a knob would buy a fleet
// where two nodes notice different things and nobody can say which.
func WatchTopics() []WatchTopic {
	topics := make([]WatchTopic, 0, len(watchKinds))
	for topic := range watchKinds {
		topics = append(topics, topic)
	}
	slices.Sort(topics)
	return topics
}

// WatchEvent is one thing that happened, and never what it was.
//
// It deliberately carries no resource: see the package comment above. A caller
// that wants the new value re-reads, because re-reading is the path that is
// already correct.
type WatchEvent struct {
	// Topic is what changed, and is empty on the snapshot event.
	Topic WatchTopic

	// Snapshot marks the single event saying every topic has delivered its
	// current contents, so the watch is now reporting changes rather than
	// catching up.
	//
	// It arrives exactly once per watch, before any change event. Until it
	// does, a caller has a subscription that has not started, which is not the
	// same as a node on which nothing is happening -- and telling those two
	// apart is the entire reason this field exists rather than a caller
	// assuming the watch is live the moment it was constructed.
	Snapshot bool
}

// A watch does not notice a node that vanishes, and that is load-bearing.
//
// It was measured rather than assumed: a subscription against a simulated node
// that is then stopped outright stays open, silent and errorless for minutes.
// The cause is inside cosi-project's protobuf client -- a broken watch stream is
// re-established from the last bookmark on its own exponential backoff, with
// nothing reported upward until the retry budget is exhausted a quarter of an
// hour later.
//
// For a watch that briefly lost its connection that is the right behaviour and
// it is why nothing here tries to defeat it. For a node that is gone it means
// the subscription claims to be live for fifteen minutes after there is
// anything to be live against, which is the silent freeze in its purest form.
//
// The bound is therefore not on this stream. It is the heartbeat: the poller
// keeps reading the node on its own timer, and a caller that holds both takes
// the poller's verdict as the truth about whether the watch still means
// anything. internal/inventory does exactly that, and it is the concrete reason
// the poll could not be removed once the watch existed -- not tidiness, not
// belt and braces. Without it this package has no way to tell a quiet node from
// a dead one.

// Watch is a live subscription to one node's resource state.
type Watch struct {
	events chan WatchEvent
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu   sync.Mutex
	err  error
	done bool
}

// Watch subscribes to every topic on the node this client is connected to.
//
// The returned watch runs until it is closed, its context is cancelled, or the
// node stops it -- and the last of those is a fact the caller has to be able to
// see, so the event channel closes and Err says why.
func (c *ClusterClient) Watch(ctx context.Context) *Watch {
	return startWatch(ctx, c.COSI(), WatchTopics())
}

func startWatch(ctx context.Context, st state.State, topics []WatchTopic) *Watch {
	runCtx, cancel := context.WithCancel(ctx)

	w := &Watch{
		// Buffered, and a send that would block is dropped rather than waited
		// on. An event carries no data, so N pending notices and one pending
		// notice mean the same thing to a consumer that answers by re-reading;
		// what a blocking send would buy is a stalled receive loop, which on a
		// class with no idle timeout is a watch that stops delivering and never
		// says so.
		events: make(chan WatchEvent, 1),
		cancel: cancel,
	}

	// bootstrapped counts topics that have delivered their initial contents.
	// The snapshot event is emitted when the last one does, so a caller is
	// never told the watch is live while one topic is still catching up.
	var (
		mu           sync.Mutex
		bootstrapped int
		announced    bool
	)

	for _, topic := range topics {
		kind, ok := watchKinds[topic]
		if !ok {
			// A topic with no kind behind it is a programming error in this
			// file, and it ends the whole watch rather than silently running
			// with one topic missing: a subscription that is quietly deaf on
			// one kind is worse than one that refuses to start.
			w.finish(fmt.Errorf("talos: watch: no resource kind for topic %q", topic))
			break
		}

		ch := make(chan state.Event)

		if err := st.WatchKind(runCtx, kind, ch, state.WithBootstrapContents(true)); err != nil {
			w.finish(fmt.Errorf("talos: watch %s: %w", topic, err))
			break
		}

		w.wg.Add(1)
		go func() {
			defer w.wg.Done()

			for {
				select {
				case <-runCtx.Done():
					return
				case ev := <-ch:
					switch ev.Type {
					case state.Bootstrapped:
						mu.Lock()
						bootstrapped++
						last := bootstrapped == len(topics) && !announced
						if last {
							announced = true
						}
						mu.Unlock()

						if last {
							w.emit(WatchEvent{Snapshot: true})
						}

					case state.Created, state.Updated, state.Destroyed:
						// Everything a topic already had arrives as Created
						// before its Bootstrapped, so a change is only a change
						// once the snapshot is complete. Forwarding the initial
						// contents would make every node start by announcing
						// six changes that are not changes, and the refresh
						// they triggered would race the one the caller is about
						// to do anyway.
						mu.Lock()
						ready := announced
						mu.Unlock()

						if ready {
							w.emit(WatchEvent{Topic: topic})
						}

					case state.Errored:
						w.finish(fmt.Errorf("talos: watch %s: %w", topic, ev.Error))
						return

					case state.Noop:
						// A bookmark keepalive. It is not a change and it is
						// not a failure; ignoring it is the whole handling.
					}
				}
			}
		}()
	}

	// The closer, started unconditionally -- including on the paths above that
	// gave up before subscribing to every topic. The event channel closing is
	// the caller's only signal that the subscription is over, so a construction
	// failure that returned without arranging for it would hand back a watch
	// that never delivers and never ends, which is precisely the silent freeze
	// this whole design is arranged against.
	go func() {
		w.wg.Wait()
		w.finish(runCtx.Err())
		close(w.events)
	}()

	return w
}

// emit delivers an event, or drops it if the consumer is behind. See the
// comment on the channel in startWatch for why dropping is correct here.
func (w *Watch) emit(ev WatchEvent) {
	select {
	case w.events <- ev:
	default:
	}
}

// finish records the first reason the watch ended and stops the rest of it.
//
// First and not last: the cause is what tore the others down, so a later error
// from a sibling topic is a consequence being reported as a cause.
func (w *Watch) finish(err error) {
	w.mu.Lock()
	if !w.done {
		w.done = true
		w.err = err
	}
	w.mu.Unlock()

	w.cancel()
}

// Events is the subscription. It is closed when the watch ends.
func (w *Watch) Events() <-chan WatchEvent { return w.events }

// Err is why the watch ended, and nil for one that is still running or that was
// closed by its caller.
//
// Cancellation is reported as nil rather than as context.Canceled: a watch the
// caller stopped did not fail, and a caller that has to filter its own
// cancellation out of an error is a caller that will eventually log it.
func (w *Watch) Err() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if errors.Is(w.err, context.Canceled) {
		return nil
	}
	return w.err
}

// Close stops the watch and waits for its goroutines.
func (w *Watch) Close() error {
	w.cancel()
	w.wg.Wait()
	return nil
}
