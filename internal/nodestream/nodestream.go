// Package nodestream turns a node's log and dmesg output into hub topics.
//
// It owns the half of streaming that is about a Talos node rather than about a
// browser: one upstream reader per topic no matter how many panels are open,
// a reader that stops when the last panel closes, and a connection state that
// is published *into the stream* rather than inferred from its absence.
//
// That last point is STREAM-04 and it is the reason this package exists at all
// rather than the handler reading the node directly. "No lines are arriving"
// has at least four causes an operator has to tell apart -- the service is
// quiet, holzkube-manager is reconnecting, the node is rebooting, the node is gone --
// and a stream that only carries log lines cannot distinguish any of them. So
// the state is an event on the same stream, in order, and the panel can say
// which of the four it is looking at.
package nodestream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/streamhub"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// Kind is what a topic carries.
type Kind string

const (
	// KindLogs is one service's log output.
	KindLogs Kind = "logs"

	// KindDmesg is the kernel ring buffer.
	KindDmesg Kind = "dmesg"
)

// Source names one stream: which node, what kind, and for logs which service.
type Source struct {
	Machine model.MachineID
	Kind    Kind

	// Service is the Talos service whose log to follow, e.g. "kubelet" or
	// "etcd". It is empty for dmesg.
	Service string
}

// Topic is the hub topic this source publishes to.
//
// The shape is stable and is part of what a client sends back on reconnect, so
// it is built here rather than at each call site: two spellings of one topic
// are two streams of the same thing, and the second one is a second read
// against a node.
// Concatenated rather than formatted, and that is not style. The seam guard in
// internal/talos scans production sources for a format string that builds a
// network endpoint -- a verb, a colon, then a port -- and `"logs:%s:%s"`
// matches that shape exactly while being nothing of the kind. Widening the
// guard to let this through would weaken it for the thing it exists to catch;
// not using a format string here costs nothing.
func (s Source) Topic() streamhub.Topic {
	if s.Kind == KindDmesg {
		return streamhub.Topic(string(KindDmesg) + ":" + string(s.Machine))
	}
	return streamhub.Topic(string(KindLogs) + ":" + string(s.Machine) + ":" + s.Service)
}

// ParseTopic reads a topic back into a source.
//
// A client names the topics it wants, so this is parsing input: an unknown
// shape is an error rather than a best effort, and a service name with a colon
// in it cannot smuggle a second field past the split.
func ParseTopic(t streamhub.Topic) (Source, error) {
	parts := strings.Split(string(t), ":")
	switch {
	case len(parts) == 2 && parts[0] == string(KindDmesg):
		if parts[1] == "" {
			return Source{}, errors.New("nodestream: dmesg topic names no machine")
		}
		return Source{Machine: model.MachineID(parts[1]), Kind: KindDmesg}, nil
	case len(parts) == 3 && parts[0] == string(KindLogs):
		if parts[1] == "" || parts[2] == "" {
			return Source{}, errors.New("nodestream: logs topic names no machine or no service")
		}
		return Source{Machine: model.MachineID(parts[1]), Kind: KindLogs, Service: parts[2]}, nil
	default:
		return Source{}, fmt.Errorf("nodestream: %q is not a stream topic", t)
	}
}

// State is the connection state of one stream, as the operator sees it.
//
// The four are not degrees of the same thing; they are four different answers
// to "why is nothing arriving", and each calls for something different from
// the person reading. That is why they are distinct values and not a boolean
// with a message.
type State string

const (
	// StateLive: the upstream stream is open and delivering.
	StateLive State = "live"

	// StateReconnecting: the upstream stream broke and is being reopened. The
	// node may be fine; something between here and it was not.
	StateReconnecting State = "reconnecting"

	// StateRebooting: the node accepted a reboot or is otherwise expected back.
	// It is separate from reconnecting because the right response is to wait
	// rather than to investigate.
	StateRebooting State = "rebooting"

	// StateDisconnected: the stream is not running and nothing is trying. This
	// is what the last panel closing looks like, and what a node that has been
	// unreachable past the retry budget looks like.
	StateDisconnected State = "disconnected"
)

// Message is what goes on the wire for one stream event.
//
// A state change and a log line are the same kind of thing here -- something
// that happened at a point in the stream -- which is what makes them orderable
// against each other. A state carried out of band would arrive whenever it
// arrived, and "reconnecting" would appear above lines that predate it.
type Message struct {
	// Line is log output, present for an ordinary event.
	Line string `json:"line,omitempty"`

	// State is set instead when this event is a connection-state change.
	State State `json:"state,omitempty"`

	// Reason explains a state that is not live.
	Reason string `json:"reason,omitempty"`

	At time.Time `json:"at"`
}

// Opener makes a client for a machine. It is a function rather than a
// dependency on the inventory so that this package can be driven in a test
// with two lines and no store.
type Opener func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error)

// Deps is what the manager needs.
type Deps struct {
	Hub    *streamhub.Hub
	Open   Opener
	Logger *slog.Logger

	// Linger is how long a reader keeps running after its last subscriber
	// left. It exists because navigating between two screens closes and
	// reopens a panel within a second, and tearing a node stream down and back
	// up for that is two unnecessary RPCs and a visible gap.
	Linger time.Duration

	// RetryBackoff is the wait between reconnection attempts. It is a fixed
	// small value rather than an exponential one: a stream is watched by
	// somebody who is looking at the screen right now, and a backoff that
	// grows to minutes is indistinguishable from a stream that gave up.
	RetryBackoff time.Duration
}

// DefaultLinger and DefaultRetryBackoff are the values a manager uses when
// Deps does not say.
const (
	DefaultLinger       = 30 * time.Second
	DefaultRetryBackoff = 3 * time.Second
)

// Manager owns one reader goroutine per active topic.
type Manager struct {
	deps Deps

	mu      sync.Mutex
	readers map[streamhub.Topic]*reader
	closed  bool
	wg      sync.WaitGroup
}

type reader struct {
	cancel context.CancelFunc

	// refs is how many subscribers asked for this topic. The reader stops when
	// it reaches zero and the linger expires.
	refs int

	// idleSince is when refs last hit zero.
	idleSince time.Time
}

// New builds a manager.
func New(d Deps) *Manager {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Linger <= 0 {
		d.Linger = DefaultLinger
	}
	if d.RetryBackoff <= 0 {
		d.RetryBackoff = DefaultRetryBackoff
	}
	return &Manager{deps: d, readers: map[streamhub.Topic]*reader{}}
}

// Acquire makes sure a topic has a running reader and returns the function
// that releases it.
//
// Reference counting rather than "start when the handler starts": several
// panels in one tab, and several tabs, watch the same topic, and each starting
// its own reader would mean several follow streams against one node for one
// picture. One upstream per topic is the whole point of the hub sitting
// between them.
func (m *Manager) Acquire(ctx context.Context, src Source) (func(), error) {
	topic := src.Topic()

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, errors.New("nodestream: manager is closed")
	}

	r, ok := m.readers[topic]
	if ok {
		r.refs++
		r.idleSince = time.Time{}
		m.mu.Unlock()
		return func() { m.release(topic) }, nil
	}

	// The reader outlives this request: a subscriber going away must not kill
	// a stream another subscriber is still watching. context.WithoutCancel
	// keeps whatever values the request carried while dropping its lifetime.
	readCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r = &reader{cancel: cancel, refs: 1}
	m.readers[topic] = r
	m.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.run(readCtx, src)
	}()

	return func() { m.release(topic) }, nil
}

func (m *Manager) release(topic streamhub.Topic) {
	m.mu.Lock()
	r, ok := m.readers[topic]
	if !ok {
		m.mu.Unlock()
		return
	}
	r.refs--
	if r.refs > 0 {
		m.mu.Unlock()
		return
	}
	r.idleSince = time.Now()
	linger := m.deps.Linger
	m.mu.Unlock()

	// The linger is a timer and not a sleep inside the lock: a panel reopening
	// during it must be able to take the reference back.
	time.AfterFunc(linger, func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		r, ok := m.readers[topic]
		if !ok || r.refs > 0 || r.idleSince.IsZero() {
			return
		}
		delete(m.readers, topic)
		r.cancel()
	})
}

// run is one topic's reader. It reconnects until its context is cancelled.
func (m *Manager) run(ctx context.Context, src Source) {
	topic := src.Topic()
	defer m.publishState(topic, StateDisconnected, "nobody is watching this stream any more")

	for {
		if ctx.Err() != nil {
			return
		}

		err := m.follow(ctx, src)
		switch {
		case ctx.Err() != nil:
			return
		case err == nil, errors.Is(err, io.EOF):
			// The node ended the stream. For a follow stream that means the
			// service stopped or the node is going down, which is worth saying
			// out loud rather than silently reopening.
			m.publishState(topic, StateRebooting, "the node ended the stream; waiting for it to come back")
		default:
			m.deps.Logger.Debug("node stream broke",
				slog.String("topic", string(topic)), slog.Any("error", err))
			m.publishState(topic, StateReconnecting, reasonFor(err))
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(m.deps.RetryBackoff):
		}
	}
}

// follow opens one stream and pumps it into the hub until it ends.
func (m *Manager) follow(ctx context.Context, src Source) error {
	topic := src.Topic()

	cc, err := m.deps.Open(ctx, src.Machine)
	if err != nil {
		return err
	}
	defer cc.Close() //nolint:errcheck // a close error does not change what the stream delivered

	var stream *talos.LogStream
	switch src.Kind {
	case KindDmesg:
		stream, err = cc.Dmesg(ctx, true)
	case KindLogs:
		stream, err = cc.Logs(ctx, src.Service)
	default:
		return fmt.Errorf("nodestream: unknown kind %q", src.Kind)
	}
	if err != nil {
		return err
	}
	defer stream.Close() //nolint:errcheck // as above

	m.publishState(topic, StateLive, "")

	for {
		chunk, err := stream.Recv()
		if err != nil {
			return err
		}
		// A chunk is whatever the node sent and may hold several lines or part
		// of one. Splitting here rather than in the browser means the event
		// ids line up with lines, which is what makes Last-Event-ID resumption
		// land on a line boundary.
		for line := range strings.SplitSeq(strings.TrimRight(string(chunk), "\n"), "\n") {
			m.publish(topic, Message{Line: line, At: time.Now().UTC()})
		}
	}
}

func (m *Manager) publishState(topic streamhub.Topic, state State, reason string) {
	m.publish(topic, Message{State: state, Reason: reason, At: time.Now().UTC()})
}

func (m *Manager) publish(topic streamhub.Topic, msg Message) {
	raw, err := json.Marshal(msg)
	if err != nil {
		m.deps.Logger.Error("could not encode a stream message",
			slog.String("topic", string(topic)), slog.Any("error", err))
		return
	}
	if _, err := m.deps.Hub.Publish(topic, raw); err != nil && !errors.Is(err, streamhub.ErrClosed) {
		m.deps.Logger.Error("could not publish a stream message",
			slog.String("topic", string(topic)), slog.Any("error", err))
	}
}

// reasonFor turns a transport failure into the sentence the panel shows.
//
// It uses the classified kind rather than the Go error text for the reason the
// inventory does: the kinds are what an operator can act on, and a wrapped
// gRPC status string in a status line pushes the actionable part off the edge.
func reasonFor(err error) string {
	if kind, ok := talos.ErrorKindOf(err); ok {
		switch kind {
		case talos.KindTimeout:
			return "the node stopped sending"
		case talos.KindUnreachable:
			return "the node could not be reached"
		case talos.KindRejected:
			return "the node refused the stream"
		}
	}
	return "the stream ended unexpectedly"
}

// Close stops every reader and waits for them.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	readers := m.readers
	m.readers = map[streamhub.Topic]*reader{}
	m.mu.Unlock()

	for _, r := range readers {
		r.cancel()
	}
	m.wg.Wait()
	return nil
}

// Active reports how many readers are running, for tests and for the status
// endpoint.
func (m *Manager) Active() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.readers)
}
