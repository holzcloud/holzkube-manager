// Package streamhub is the fan-out substrate between something that produces a
// stream and the browsers watching it.
//
// It exists because the naive shape -- one upstream reader per browser panel,
// writing straight to the socket -- fails in three ways that all look like
// something else:
//
//   - A slow browser blocks the upstream reader, so one stalled tab stops the
//     log stream for everybody and, worse, applies backpressure to a Talos node.
//   - A reconnecting browser silently loses whatever arrived while it was away,
//     and the gap is invisible: the log simply does not contain those lines.
//   - Two panels on one node open two upstream streams, which doubles the load
//     on a node for a picture that is identical.
//
// The answer is the one ARCHITECTURE Pattern 5 specifies and it is not novel:
// one reader per topic, a bounded ring buffer per topic, and a non-blocking
// fan-out to subscribers. What is worth being precise about is the failure
// mode that remains, because a bounded buffer must drop something eventually:
//
//	**A subscriber that falls behind loses events, and is told so.**
//
// It is never silently truncated. The subscriber receives a Gap carrying how
// many events it missed, and the UI renders that as a visible break in the
// log. An invisible omission in a log an operator is using to diagnose an
// outage is worse than no log at all: it produces confident wrong conclusions.
package streamhub

import (
	"errors"
	"sync"
)

// Topic names one stream. It is opaque here -- "logs:<uuid>:kubelet",
// "dmesg:<uuid>", "job:<id>" -- because the hub has no business knowing what
// it is carrying; that is what lets phase 6 hang job progress on it without
// touching this package.
type Topic string

// Event is one item in a stream.
//
// ID is monotonic within a topic and starts at 1, so that 0 is always "before
// the first event" and a client that sends Last-Event-ID: 0 gets everything
// still buffered. Zero being a value that cannot be an event id is what lets
// the replay logic have no special case.
type Event struct {
	ID   uint64
	Data []byte
}

// Gap reports that a subscriber fell behind and lost events.
//
// It is a value on the channel and not a log line, because the operator
// watching the stream is the one who needs to know. Missed is exact: it is the
// difference between the id the subscriber last received and the id of the
// next event it will.
type Gap struct {
	Missed uint64

	// Through is the id of the last event that was dropped, so a UI can say
	// "lines 400-612 were dropped" rather than "some lines were dropped".
	Through uint64
}

// Item is what a subscriber receives: an event, or the news that some are
// missing. Exactly one of the two is set.
type Item struct {
	Event *Event
	Gap   *Gap
}

// ErrClosed is returned by Publish after the hub is closed.
var ErrClosed = errors.New("streamhub: hub is closed")

// DefaultRingSize is how many events a topic remembers.
//
// It is a count and not a byte budget, deliberately: the thing a reconnecting
// client needs is "the last N lines", and a byte budget makes N depend on how
// chatty the log happened to be. Five hundred lines is roughly a screenful at
// any sensible font size times ten, which is enough to cover a reconnect and
// far too little to be a memory concern at a few dozen topics.
const DefaultRingSize = 500

// DefaultSubscriberBuffer is how far one subscriber may fall behind before it
// starts losing events.
//
// Small on purpose. A large per-subscriber buffer does not prevent the loss,
// it delays it and then loses more at once; and every byte of it is held per
// browser tab. What actually protects the reader is that the fan-out never
// blocks, which this number does not affect.
const DefaultSubscriberBuffer = 64

// Hub is the fan-out.
type Hub struct {
	ringSize int
	subBuf   int

	mu     sync.Mutex
	topics map[Topic]*topic
	closed bool
}

// New builds a hub.
func New() *Hub {
	return &Hub{
		ringSize: DefaultRingSize,
		subBuf:   DefaultSubscriberBuffer,
		topics:   map[Topic]*topic{},
	}
}

// NewWithSizes builds a hub with explicit buffer sizes. It exists for tests
// that want to provoke an overflow in ten events rather than in five hundred.
func NewWithSizes(ringSize, subscriberBuffer int) *Hub {
	if ringSize < 1 {
		ringSize = 1
	}
	if subscriberBuffer < 1 {
		subscriberBuffer = 1
	}
	return &Hub{ringSize: ringSize, subBuf: subscriberBuffer, topics: map[Topic]*topic{}}
}

// topic is one stream's buffer and its subscribers.
type topic struct {
	mu sync.Mutex

	// ring holds the last len(ring) events, oldest first after wrap. It is a
	// slice used as a circular buffer rather than a linked list because every
	// access is either "append one" or "walk everything after id N", and both
	// are what a slice is good at.
	ring  []Event
	next  int
	count int

	lastID uint64

	subs map[*subscription]struct{}
}

type subscription struct {
	ch chan Item

	// lastSent is the id of the last event this subscriber actually received.
	// It is what makes a Gap exact rather than approximate.
	lastSent uint64

	// dropped counts events lost since the last Gap was delivered. It is
	// carried rather than reported immediately because a subscriber that is
	// behind is, by definition, not reading -- so the Gap is sent with the
	// next event that fits, when there is somebody to read it.
	dropped        uint64
	droppedThrough uint64

	closed bool
}

// Publish appends an event to a topic and fans it out.
//
// It never blocks on a subscriber, and that is the single most important
// property in this package: the caller is a goroutine reading from a Talos
// node, and a browser tab that stopped reading must not be able to apply
// backpressure to a node.
func (h *Hub) Publish(name Topic, data []byte) (uint64, error) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return 0, ErrClosed
	}
	t := h.topicLocked(name)
	h.mu.Unlock()

	t.mu.Lock()
	defer t.mu.Unlock()

	t.lastID++
	ev := Event{ID: t.lastID, Data: data}

	t.ring[t.next] = ev
	t.next = (t.next + 1) % len(t.ring)
	if t.count < len(t.ring) {
		t.count++
	}

	for sub := range t.subs {
		sub.deliver(ev)
	}
	return ev.ID, nil
}

// deliver hands one event to a subscriber.
//
// The caller holds the topic lock, so exactly one deliver runs per subscriber
// at a time and the accounting below cannot interleave with itself.
//
// What it does when the subscriber is full is the design decision, and it is
// the opposite of the obvious one: **the newest event wins and the oldest
// queued one is discarded**, rather than the newest being dropped on the
// floor. A live log is a tail. A subscriber that fell behind and is then shown
// the events from thirty seconds ago, while the ones from now are thrown away,
// is watching a stream that has quietly become a replay -- and will never
// catch up, because every new event is discarded at the same point.
//
// The discarded events are counted and reported as a Gap, which is the whole
// bargain: a bounded buffer must lose something eventually, and what it must
// never do is lose it silently.
func (s *subscription) deliver(ev Event) {
	if s.closed {
		return
	}

	// The gap goes in before the event it precedes, so the order on the
	// channel is the order on the screen: "…, 212 lines dropped, line 613".
	if s.dropped > 0 {
		gap := Gap{Missed: s.dropped, Through: s.droppedThrough}
		s.dropped = 0
		s.droppedThrough = 0
		s.push(Item{Gap: &gap})
	}
	s.push(Item{Event: &ev})
}

// push enqueues an item, evicting the oldest queued one if there is no room.
//
// It never blocks, which is the property the publisher depends on: it is a
// goroutine reading from a Talos node, and a browser tab that stopped reading
// must not be able to apply backpressure to a node.
//
// The loop terminates: the topic lock means one push at a time per
// subscriber, so after a successful eviction there is room, and a concurrent
// reader can only make room rather than take it.
func (s *subscription) push(item Item) {
	for {
		select {
		case s.ch <- item:
			if item.Event != nil {
				s.lastSent = item.Event.ID
			}
			return
		default:
		}

		select {
		case old := <-s.ch:
			s.account(old)
		default:
			// The reader emptied it between the two selects. Round again.
		}
	}
}

// account records an evicted item as lost.
//
// An evicted Gap folds into the pending one rather than disappearing: two
// holes with the events between them also gone is one bigger hole, and
// reporting the smaller of the two would be an undercount in the direction
// that misleads.
func (s *subscription) account(old Item) {
	switch {
	case old.Event != nil:
		s.dropped++
		if old.Event.ID > s.droppedThrough {
			s.droppedThrough = old.Event.ID
		}
	case old.Gap != nil:
		s.dropped += old.Gap.Missed
		if old.Gap.Through > s.droppedThrough {
			s.droppedThrough = old.Gap.Through
		}
	}
}

// Subscribe returns a channel of items for a topic, replaying what the ring
// still holds after afterID.
//
// afterID is the client's Last-Event-ID. Zero means "everything you still
// have", which is what a fresh connection wants: a log panel that opens to an
// empty box while the node is quiet is indistinguishable from a broken one.
//
// If the ring no longer reaches back to afterID, the first item is a Gap. A
// client that was away longer than the buffer is told exactly that, rather
// than being handed a continuation that quietly skips.
func (h *Hub) Subscribe(name Topic, afterID uint64) (<-chan Item, func(), error) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, nil, ErrClosed
	}
	t := h.topicLocked(name)
	h.mu.Unlock()

	t.mu.Lock()
	defer t.mu.Unlock()

	sub := &subscription{
		// Sized to hold the replay plus the live buffer, so that a subscriber
		// is not already behind at the moment it connects.
		ch:       make(chan Item, h.subBuf+len(t.ring)),
		lastSent: afterID,
	}

	backlog := t.since(afterID)
	if missed := t.missedBefore(afterID, backlog); missed > 0 {
		through := t.oldestID() - 1
		sub.ch <- Item{Gap: &Gap{Missed: missed, Through: through}}
	}
	for _, ev := range backlog {
		sub.ch <- Item{Event: &ev} //nolint:exportloopref // ev is copied into the Item
		sub.lastSent = ev.ID
	}

	t.subs[sub] = struct{}{}

	cancel := func() {
		t.mu.Lock()
		defer t.mu.Unlock()

		if sub.closed {
			return
		}
		sub.closed = true
		delete(t.subs, sub)
		close(sub.ch)
	}
	return sub.ch, cancel, nil
}

// since returns the buffered events after id, oldest first.
func (t *topic) since(id uint64) []Event {
	out := make([]Event, 0, t.count)
	for i := range t.count {
		idx := (t.next - t.count + i + len(t.ring)*2) % len(t.ring)
		if t.ring[idx].ID > id {
			out = append(out, t.ring[idx])
		}
	}
	return out
}

// oldestID is the id of the oldest event still buffered, or lastID+1 when the
// buffer is empty.
func (t *topic) oldestID() uint64 {
	if t.count == 0 {
		return t.lastID + 1
	}
	idx := (t.next - t.count + len(t.ring)*2) % len(t.ring)
	return t.ring[idx].ID
}

// missedBefore is how many events between afterID and the start of the backlog
// the buffer no longer holds.
func (t *topic) missedBefore(afterID uint64, backlog []Event) uint64 {
	if t.count == 0 || len(backlog) == 0 {
		return 0
	}
	first := backlog[0].ID
	if first <= afterID+1 {
		return 0
	}
	return first - afterID - 1
}

// LastID reports the newest event id on a topic, or 0 for a topic nothing has
// published to.
func (h *Hub) LastID(name Topic) uint64 {
	h.mu.Lock()
	t, ok := h.topics[name]
	h.mu.Unlock()
	if !ok {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastID
}

// Subscribers reports how many subscribers a topic has. It is what lets a
// producer stop reading from a node nobody is watching.
func (h *Hub) Subscribers(name Topic) int {
	h.mu.Lock()
	t, ok := h.topics[name]
	h.mu.Unlock()
	if !ok {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.subs)
}

// Topics lists every topic the hub currently holds.
func (h *Hub) Topics() []Topic {
	h.mu.Lock()
	defer h.mu.Unlock()

	out := make([]Topic, 0, len(h.topics))
	for name := range h.topics {
		out = append(out, name)
	}
	return out
}

// Drop forgets a topic entirely, closing every subscriber.
//
// It is what a producer calls when its upstream ended for good -- a node that
// was reset, a job that finished. Keeping the buffer around after that would
// serve a log that can never gain another line to a panel that looks live.
func (h *Hub) Drop(name Topic) {
	h.mu.Lock()
	t, ok := h.topics[name]
	delete(h.topics, name)
	h.mu.Unlock()
	if !ok {
		return
	}
	t.closeAll()
}

// Close shuts the hub down and closes every subscriber channel.
func (h *Hub) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	topics := make([]*topic, 0, len(h.topics))
	for _, t := range h.topics {
		topics = append(topics, t)
	}
	h.topics = map[Topic]*topic{}
	h.mu.Unlock()

	for _, t := range topics {
		t.closeAll()
	}
	return nil
}

func (t *topic) closeAll() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for sub := range t.subs {
		if !sub.closed {
			sub.closed = true
			close(sub.ch)
		}
	}
	t.subs = map[*subscription]struct{}{}
}

// topicLocked returns a topic, creating it if it does not exist. The caller
// holds h.mu.
func (h *Hub) topicLocked(name Topic) *topic {
	t, ok := h.topics[name]
	if !ok {
		t = &topic{
			ring: make([]Event, h.ringSize),
			subs: map[*subscription]struct{}{},
		}
		h.topics[name] = t
	}
	return t
}
