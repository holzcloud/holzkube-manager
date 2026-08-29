package talossim

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/siderolabs/talos/pkg/machinery/api/common"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
)

// DefaultStreamMessages is how many messages each server stream emits when
// Options does not say otherwise.
//
// It is small on purpose. The number exists so that a test can consume a
// stream to completion and say what completion was; a large default would only
// make every streaming test slower without making any of them stronger.
const DefaultStreamMessages = 8

// Streamer is the per-server emitter behind Logs, Dmesg and Events.
//
// The emission goes through a channel rather than a direct write loop inside
// each handler, and that indirection is the point rather than an accident: the
// slow_log_consumer scenario in plan 02-03 stalls a stream, and stalling a
// channel is something a scenario can do from outside without reshaping this
// file. Collapsing Open into a loop over stream.Send would remove the only
// place that stall can be applied.
//
// The channel is unbounded in neither direction: it is unbuffered, so the
// producer makes no progress at all while the consumer is not reading. That is
// the mitigation for T-02-08-01 -- an emitter that buffered ahead of a stalled
// consumer would grow until the test process ran out of memory, and every
// scenario test built on it would be a flake rather than a failure.
type Streamer struct {
	mu       sync.Mutex
	messages int
	produced int

	// unbounded is set by the slow_log_consumer scenario. The emitter already
	// blocks rather than buffers unconditionally; what the scenario adds is a
	// sequence that never ends, so a consumer that stops reading is stalling a
	// producer that would otherwise have finished and closed the stream.
	unbounded bool

	// blocked counts sends that had to wait for a consumer, and blockedFor
	// accumulates how long they waited. They are recorded when the wait
	// begins, not when it ends, because the assertion slow_log_consumer needs
	// is about a send that is blocked right now -- a record written only on
	// release would read zero for exactly as long as the stall lasted.
	blocked    int
	blockedFor time.Duration
}

func newStreamer(count int) *Streamer {
	if count <= 0 {
		count = DefaultStreamMessages
	}
	return &Streamer{messages: count}
}

// Streams returns the node's stream emitter, so that a test or a scenario can
// change how much a stream emits before opening one.
func (s *Server) Streams() *Streamer { return s.streams }

// Messages reports how many messages each stream emits.
func (e *Streamer) Messages() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.messages
}

// SetMessages changes how many messages each subsequently opened stream emits.
func (e *Streamer) SetMessages(n int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.messages = n
}

// setUnbounded switches the emitter between the bounded default and the
// endless production slow_log_consumer needs.
func (e *Streamer) setUnbounded(v bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.unbounded = v
}

// BlockedSends reports how many sends have had to wait for a consumer.
//
// It is the observation slow_log_consumer is proven by. client.EventsWatch
// returns nil on Canceled and DeadlineExceeded, so a stalled stream torn down
// by its caller reads as clean success at the client; the simulator saying "I
// was blocked" is the evidence that survives that.
func (e *Streamer) BlockedSends() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.blocked
}

// BlockedFor is how long sends have waited for a consumer in total, counting
// only waits that have ended.
func (e *Streamer) BlockedFor() time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.blockedFor
}

func (e *Streamer) beginBlocked() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.blocked++
}

func (e *Streamer) endBlocked(waited time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.blockedFor += waited
}

// Produced reports how many messages the emitter has taken off its generator
// across every stream opened so far, counting the one currently offered to a
// consumer that has not taken it yet.
//
// It is what a test asserts on to show the emitter blocks rather than buffers:
// against a consumer that reads nothing this number stops at one, however many
// messages the stream was configured to carry.
func (e *Streamer) Produced() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.produced
}

// Open starts one stream's emission and returns the channel carrying it.
//
// The channel is closed when the sequence is exhausted or the context is done,
// so a caller ranges over it and needs no separate termination signal. The
// emission is bounded by Messages: a Follow request does not make it endless,
// because a simulated node that streamed forever could only be consumed by a
// test that already knew when to stop, and that test would be asserting on its
// own timeout rather than on the node.
//
// The slow_log_consumer scenario is the single, deliberate exception, and it
// is what makes that scenario distinguishable from the baseline at all: a
// bounded stream against a stalled consumer also blocks, but it blocks having
// promised to end.
func (e *Streamer) Open(ctx context.Context, prefix string) <-chan []byte {
	e.mu.Lock()
	count := e.messages
	if e.unbounded {
		// Zero means no bound. The loop below reads it that way rather than
		// counting to a large number, so "endless" is a property of the code
		// and not of an arithmetic limit a slow test could reach.
		count = 0
	}
	e.mu.Unlock()

	// Unbuffered. A buffered channel here would let the producer run ahead of
	// a stalled consumer, which is exactly the unbounded growth this type
	// exists to refuse.
	ch := make(chan []byte)

	go func() {
		defer close(ch)

		for i := 1; count == 0 || i <= count; i++ {
			// Checked before the send as well as inside it. Once the context
			// is done both arms of the select below are ready and Go picks
			// between them at random, so without this the emitter could keep
			// delivering after cancellation for as long as the coin kept
			// landing the same way. Here the loop stops at the next iteration,
			// which makes "at most one message in flight" a fact a test can
			// assert rather than a probability.
			select {
			case <-ctx.Done():
				return
			default:
			}

			total := strconv.Itoa(count)
			if count == 0 {
				total = "unbounded"
			}
			msg := []byte(fmt.Sprintf("%s: message %d of %s\n", prefix, i, total))

			e.mu.Lock()
			e.produced++
			e.mu.Unlock()

			// The non-blocking attempt first, so that a send which did not
			// have to wait is not recorded as a stall. Only when it fails is
			// the wait entered, and the record is written before the wait
			// rather than after it -- a stalled stream has to be observable
			// while it is stalled.
			select {
			case ch <- msg:
				continue
			default:
			}

			e.beginBlocked()
			started := time.Now()

			select {
			case <-ctx.Done():
				e.endBlocked(time.Since(started))
				return
			case ch <- msg:
				e.endBlocked(time.Since(started))
			}
		}
	}()

	return ch
}

// Logs streams container logs.
//
// The signature is grpc.ServerStreamingServer[common.Data] rather than a named
// interface because machinery declares MachineService_LogsServer as a type
// alias onto the gRPC generic: there is no interface to implement, only the
// generic itself.
func (m *machineService) Logs(req *machine.LogsRequest, stream grpc.ServerStreamingServer[common.Data]) error {
	if err := m.server.node.up(); err != nil {
		return err
	}

	prefix := fmt.Sprintf("logs %s/%s", req.GetNamespace(), req.GetId())

	return m.server.emitData(stream, prefix)
}

// Dmesg streams the kernel ring buffer.
func (m *machineService) Dmesg(_ *machine.DmesgRequest, stream grpc.ServerStreamingServer[common.Data]) error {
	if err := m.server.node.up(); err != nil {
		return err
	}

	return m.server.emitData(stream, "dmesg")
}

// Events streams the node's event history.
//
// Unlike Logs and Dmesg the payload is a typed message rather than opaque
// bytes, so each message carries a MachineStatusEvent in its Any -- the shape
// a caller has to unmarshal on a real node, which means a test written against
// this one exercises that unmarshalling rather than skipping it.
func (m *machineService) Events(_ *machine.EventsRequest, stream grpc.ServerStreamingServer[machine.Event]) error {
	if err := m.server.node.up(); err != nil {
		return err
	}

	metadata := m.server.node.metadata()

	i := 0
	for range m.server.streams.Open(stream.Context(), "event") {
		i++

		payload, err := anypb.New(&machine.MachineStatusEvent{
			Stage: machine.MachineStatusEvent_RUNNING,
		})
		if err != nil {
			return fmt.Errorf("talossim: build event payload: %w", err)
		}

		if err := stream.Send(&machine.Event{
			Metadata: metadata,
			Data:     payload,
			Id:       fmt.Sprintf("%s-%d", metadata.GetHostname(), i),
			ActorId:  actorID(),
		}); err != nil {
			return err
		}
	}

	return nil
}

// emitData is the body Logs and Dmesg share: pull from the per-server emitter,
// send each message with the answering node's identity attached, and stop when
// the emitter closes -- which it does on exhaustion and on the caller's
// cancellation alike.
func (s *Server) emitData(stream grpc.ServerStreamingServer[common.Data], prefix string) error {
	metadata := s.node.metadata()

	for msg := range s.streams.Open(stream.Context(), prefix) {
		if err := stream.Send(&common.Data{Metadata: metadata, Bytes: msg}); err != nil {
			return err
		}
	}

	return nil
}
