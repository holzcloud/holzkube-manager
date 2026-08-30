package talos

// The stream half of the deadline policy, tested against a scripted
// grpc.ClientStream rather than against a node.
//
// These are internal tests for two reasons. policyStream is unexported and is
// reachable from outside only through a live gRPC stream, and the property
// under test is what happens in the window between a receive completing and its
// timer being stopped -- a window that is nanoseconds wide against a real node
// and cannot be aimed at. The scripted stream widens it on purpose, so the
// outcome is decided rather than drawn.
//
// internal/talossim imports this package, so a test in package talos cannot use
// the simulator. Everything here is built by hand.

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// scriptedStream answers a fixed sequence of receives and lets a test put a
// delay where the code under test reads from it.
type scriptedStream struct {
	grpc.ClientStream

	ctx   context.Context //nolint:containedctx // this is what ClientStream.Context returns
	steps []func() error
	n     int

	// trailerDelay is how long Trailer blocks. RecvMsg reads the trailer after
	// the receive has settled and before the deferred timer.Stop, which makes
	// it the one place a test can hold the code inside that window long enough
	// for a timer to fire in it.
	trailerDelay time.Duration
}

func (s *scriptedStream) RecvMsg(any) error {
	if s.n >= len(s.steps) {
		return errors.New("scriptedStream: the script ran out")
	}
	step := s.steps[s.n]
	s.n++
	return step()
}

func (s *scriptedStream) Context() context.Context { return s.ctx }

func (s *scriptedStream) Trailer() metadata.MD {
	time.Sleep(s.trailerDelay)
	return nil
}

// unavailable is an ordinary transport failure: something this stream can fail
// with that is not a timeout and must never be reported as one.
func unavailable() error { return status.Error(codes.Unavailable, "connection closed") }

// newTestStream builds an unbounded policyStream over a script, at a window
// short enough for a test. The steps are filled in by the caller so that a step
// can close over the stream's own context.
func newTestStream(t *testing.T, window time.Duration, steps int) *policyStream {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	return &policyStream{
		ClientStream: &scriptedStream{ctx: ctx, steps: make([]func() error, steps)},
		conn:         &conn{target: Target{Machine: model.MachineID("stream-under-test")}},
		op:           "Logs",
		cancel:       cancel,
		unbounded:    true,
		firstByte:    window,
		idle:         window,
	}
}

// script is the scripted stream behind a policyStream built by newTestStream.
func script(t *testing.T, ps *policyStream) *scriptedStream {
	t.Helper()

	s, ok := ps.ClientStream.(*scriptedStream)
	if !ok {
		t.Fatalf("ClientStream is a %T", ps.ClientStream)
	}
	return s
}

// assertNotReportedAsStarvation fails when err is the idle timer's verdict.
//
// The kind and the wrapped cause are checked separately because they are two
// claims: KindTimeout is what a caller branches on, and the context.
// DeadlineExceeded underneath it is the "no data for 60s" sentence an operator
// reads. Neither is true of a stream whose receive returned on time.
func assertNotReportedAsStarvation(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("the receive succeeded where the script fails it")
	}
	if kind, ok := ErrorKindOf(err); ok && kind == KindTimeout {
		t.Errorf("an ordinary transport failure is classified as a timeout: %v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a stream that produced is reported as having produced nothing: %v", err)
	}
}

// TestStarvationIsNotLatchedPastADeliveredMessage is the reported half of
// WR-02.
//
// starved is set by the idle timer and was never cleared, so once it had fired
// every later failure on that stream was reported as KindTimeout with "no data
// for 60s" -- including a failure that followed a message the stream had
// actually delivered. The three scripted receives are that sequence: the window
// expires with nothing on the wire, then the transport hands over a message it
// had already buffered, then the connection goes away for an unrelated reason.
// Only the first of those is a timeout.
func TestStarvationIsNotLatchedPastADeliveredMessage(t *testing.T) {
	t.Parallel()

	const window = 100 * time.Millisecond

	ps := newTestStream(t, window, 3)
	s := script(t, ps)

	// Nothing arrives, so the receive stays blocked until the idle timer
	// cancels the stream. That is what a starved stream is.
	s.steps[0] = func() error {
		<-s.ctx.Done()
		return s.ctx.Err()
	}
	// A message the transport had already buffered.
	s.steps[1] = func() error { return nil }
	// And an unrelated failure afterwards.
	s.steps[2] = unavailable

	err := ps.RecvMsg(nil)
	kind, ok := ErrorKindOf(err)
	if !ok || kind != KindTimeout {
		t.Fatalf("a stream that received nothing for its whole window reported %v, want a timeout", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("the starvation error does not carry a deadline: %v", err)
	}

	if err := ps.RecvMsg(nil); err != nil {
		t.Fatalf("a delivered message was reported as an error: %v", err)
	}

	assertNotReportedAsStarvation(t, ps.RecvMsg(nil))
}

// TestATimerFiringAfterTheReceiveSettlesDoesNotStarveTheStream is the racing
// half of WR-02.
//
// time.AfterFunc's callback can already be running when RecvMsg returns, so the
// deferred timer.Stop() does not prevent it: the stream is cancelled although
// data flowed on time, and starved is set although nothing starved. Against a
// node that window is nanoseconds wide and the report it produces is
// unreproducible. Here the scripted stream holds RecvMsg inside it -- the
// trailer is read after the receive has settled and before the deferred Stop --
// so the timer fires in the window on every run rather than on one stream in a
// million.
func TestATimerFiringAfterTheReceiveSettlesDoesNotStarveTheStream(t *testing.T) {
	t.Parallel()

	const window = 50 * time.Millisecond

	ps := newTestStream(t, window, 2)
	s := script(t, ps)
	s.steps[0] = unavailable
	s.steps[1] = unavailable

	// The receive returns at once, five windows before the timer is due; the
	// call then stays inside RecvMsg for five of them.
	s.trailerDelay = 5 * window

	// This call is unaffected either way -- starved is read before the trailer
	// is -- so it is asserted only to show the stream was not starved when the
	// timer fired.
	assertNotReportedAsStarvation(t, ps.RecvMsg(nil))

	// This one is the assertion. Without the guard the timer that fired while
	// the call above was unwinding has set starved, and this second ordinary
	// transport failure comes back as "no data for 50ms" on a stream whose
	// every receive returned immediately.
	s.trailerDelay = 0
	assertNotReportedAsStarvation(t, ps.RecvMsg(nil))
}

// TestStreamWindowsComeFromTheDeadlinePolicy keeps the two window fields
// honest.
//
// policyStream carries its window so that a test can shorten it. The
// constructor is the only thing that sets it and it has to set it from the
// constants the deadline policy is written in; otherwise the fields are a
// second place the policy is stated, which is how two statements of it end up
// disagreeing.
func TestStreamWindowsComeFromTheDeadlinePolicy(t *testing.T) {
	t.Parallel()

	n := &conn{target: Target{Machine: model.MachineID("wiring")}}

	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string,
		_ ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		return &scriptedStream{ctx: ctx}, nil
	}

	cs, err := n.streamPolicy(t.Context(), &grpc.StreamDesc{}, nil, MethodLogs, streamer)
	if err != nil {
		t.Fatalf("streamPolicy: %v", err)
	}
	ps, ok := cs.(*policyStream)
	if !ok {
		t.Fatalf("streamPolicy returned a %T", cs)
	}
	t.Cleanup(ps.cancel)

	if !ps.unbounded {
		t.Error("Logs is not treated as an unbounded stream, although it carries no total deadline")
	}
	if ps.firstByte != StreamFirstByteDeadline {
		t.Errorf("firstByte = %v, want StreamFirstByteDeadline (%v)", ps.firstByte, StreamFirstByteDeadline)
	}
	if ps.idle != StreamIdleTimeout {
		t.Errorf("idle = %v, want StreamIdleTimeout (%v)", ps.idle, StreamIdleTimeout)
	}
	if got := ps.window(); got != StreamFirstByteDeadline {
		t.Errorf("the first window is %v, want the first-byte deadline %v", got, StreamFirstByteDeadline)
	}
	ps.gotData = true
	if got := ps.window(); got != StreamIdleTimeout {
		t.Errorf("the window after data is %v, want the idle timeout %v", got, StreamIdleTimeout)
	}
}
