package talossim

// The connection- and timing-shaped scenarios.
//
// What these five have in common is that the fault is not in what an RPC says
// but in whether, when and over what connection it says anything. That is also
// why none of them can be asserted through an error return alone:
// client.EventsWatch turns io.EOF, Canceled and DeadlineExceeded into a nil
// error, so a caller checking err reads a silent node and a flapping listener
// as clean success. Each scenario here therefore exposes a sim-side
// observation -- a transition counter, an address history, a blocked-send
// record -- and the tests assert on those and on timing bounds.

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// silence is one injection of go_silent.
//
// stop is closed by the injection's clear, so a handler parked in goSilent
// wakes as soon as the scenario is removed rather than serving out a duration
// nobody is waiting for any more.
type silence struct {
	stop chan struct{}
}

// startGoSilent arms the silence.
//
// The listener stays open and the TLS handshake still completes: a refused
// connection and a silent one produce different client behaviour, and the
// scenario TRANS-07 names is the second. Only the response is withheld.
func (s *Server) startGoSilent(_ Scenario) (func(), error) {
	sil := &silence{stop: make(chan struct{})}

	s.mu.Lock()
	previous := s.silence
	s.silence = sil
	s.mu.Unlock()

	return func() {
		close(sil.stop)

		s.mu.Lock()
		if s.silence == sil {
			s.silence = previous
		}
		s.mu.Unlock()
	}, nil
}

// goSilent parks a call until the silence ends.
//
// It waits on four things, and three of them are escapes rather than the
// nominal path. The caller's context is what the scenario is really about: the
// property under test is that the client's own deadline fires, at its own
// class, long before the node's 90 s of silence elapses. The server's done
// channel is the T-02-15 mitigation -- without it Close would block forever
// behind a parked handler and one injected scenario would hang the whole test
// binary. The injection's stop channel releases the call when the scenario is
// cleared.
func (s *Server) goSilent(ctx context.Context, sc Scenario) error {
	s.mu.Lock()
	sil := s.silence
	s.mu.Unlock()

	var stop <-chan struct{}
	if sil != nil {
		stop = sil.stop
	}

	timer := time.NewTimer(sc.Duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		// The client gave up first, which is the outcome this scenario exists
		// to produce. Reporting it as a status rather than as ctx.Err() keeps
		// the wire shape identical to a real deadline.
		return status.FromContextError(ctx.Err()).Err()
	case <-s.done:
		return status.Error(codes.Unavailable, "talossim: node is shutting down")
	case <-stop:
		return nil
	case <-timer.C:
		// The silence elapsed before the caller's deadline did. On a real node
		// the call would then be answered; here the handler runs as usual.
		return nil
	}
}

// startFlapConnection closes and reopens the listener on a cycle.
//
// The port is held explicitly -- the listener comes back on the address the
// node already had, not on a fresh :0 -- because a flap that moved the node is
// ip_changes_on_reboot, which is a different scenario with a different expected
// client behaviour. Re-listening on the same address is what makes the
// distinction real rather than nominal.
//
// This is the one scenario that owns a goroutine, and the exception is
// deliberate: closing and reopening on a cycle is the fault itself rather than
// a way of expiring it. The goroutine is owned by the injection and is stopped
// by both the returned restore and by Close, which is why Server.sg exists --
// a flapper that outlived Close could re-listen after the gRPC server was
// stopped and leak the listener (T-02-16).
func (s *Server) startFlapConnection(sc Scenario) (func(), error) {
	stop := make(chan struct{})
	finished := make(chan struct{})

	s.sg.Add(1)
	go func() {
		defer s.sg.Done()
		defer close(finished)

		for {
			if !s.waitCycle(sc.Cycle, stop) {
				return
			}
			s.closeListener()

			if !s.waitCycle(sc.Cycle, stop) {
				return
			}
			if err := s.openListener(s.Addr()); err != nil {
				// The address could not be taken back. Continuing would leave
				// the node permanently down while still reporting a flap, so
				// the flapper stops and the node stays closed; the restore
				// below reopens it.
				return
			}
		}
	}()

	return func() {
		close(stop)
		<-finished
		s.ensureListening()
	}, nil
}

// waitCycle sleeps for one cycle and reports whether the flapper should keep
// going.
func (s *Server) waitCycle(cycle time.Duration, stop <-chan struct{}) bool {
	timer := time.NewTimer(cycle)
	defer timer.Stop()

	select {
	case <-stop:
		return false
	case <-s.done:
		return false
	case <-timer.C:
		return true
	}
}

// startSlowLogConsumer makes the log emitter produce without bound.
//
// The emitter already blocks rather than buffers -- that is the T-02-08-01
// mitigation and it is unconditional. What this scenario adds is that the
// sequence never ends, so a consumer that stops reading is stalling a producer
// that would otherwise have finished and closed the stream. Without it the
// scenario would be indistinguishable from the baseline, since a bounded
// stream against a stalled consumer also blocks -- for a while.
func (s *Server) startSlowLogConsumer(_ Scenario) (func(), error) {
	s.streams.setUnbounded(true)

	return func() { s.streams.setUnbounded(false) }, nil
}

// startVersionOutOfSupportedRange makes the node report an unsupported version.
//
// It is a state mutation rather than a handler special case, because Version
// already answers from live node state: an upgrade changes that answer, and a
// scenario that bypassed the state would be testing a code path the product
// never takes.
func (s *Server) startVersionOutOfSupportedRange(sc Scenario) (func(), error) {
	previous := s.node.snapshot().Version
	s.node.setVersion(sc.Version)

	return func() { s.node.setVersion(previous) }, nil
}

// rebind moves the node to a fresh loopback address and leaves the old one
// refusing connections.
//
// It goes through closeListener, so the connections already established on the
// old address are severed along with it. A machine that is rebooting has gone
// away, and a client holding a connection that kept working across the reboot
// would never observe the address change at all -- the scenario would be inert
// against exactly the caller it exists to test.
//
// The cost is that the Reboot RPC's own reply may not reach the caller: the
// node disappears while the reply is in flight. That makes the simulator harder
// to satisfy than hardware rather than easier, which is the permitted direction
// (docs/talossim.md rule 2), and it is why plan 02-05's contract assertion
// accepts either a delivered reply or a transport failure for the Reboot call
// itself while insisting on the rebind having happened.
//
// The old address refuses rather than times out, which is the client-observable
// difference between a released port and a black hole -- and only the refusing
// one models a machine that gave its address back.
func (s *Server) rebind() error {
	s.closeListener()

	return s.openListener("127.0.0.1:0")
}

// closeListener stops the node accepting, and severs the connections already
// open on it.
//
// Severing is what makes flap_connection a fault at all: a client that had
// already dialled would otherwise keep using its established connection and
// never observe the flap, which would make the scenario inert against exactly
// the caller it exists to test.
func (s *Server) closeListener() {
	s.lmu.Lock()
	l := s.tcp
	s.tcp = nil
	if l != nil {
		s.transitions++
	}
	s.lmu.Unlock()

	if l == nil {
		return
	}

	// Errors are dropped on purpose: Close on a listener the gRPC server has
	// already stopped returns net.ErrClosed, and there is nothing a scenario
	// could do about it.
	if tl, ok := l.(*trackingListener); ok {
		tl.closeConns()
	}
	_ = l.Close()
}

// openListener puts the node back on the wire at addr.
func (s *Server) openListener(addr string) error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()

	if closed {
		return fmt.Errorf("talossim: cannot listen on %s: the node is closed", addr)
	}

	raw, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("talossim: re-listen on %s: %w", addr, err)
	}
	l := newTrackingListener(raw)

	s.lmu.Lock()
	s.tcp = l
	s.transitions++
	if got := l.Addr().String(); got != s.tcpAddr {
		s.tcpAddr = got
		s.addrHistory = append(s.addrHistory, got)
	}
	s.lmu.Unlock()

	s.serve(l)
	return nil
}

// ensureListening reopens the listener if a scenario left it closed.
func (s *Server) ensureListening() {
	s.lmu.Lock()
	open := s.tcp != nil
	addr := s.tcpAddr
	s.lmu.Unlock()

	if open {
		return
	}
	_ = s.openListener(addr)
}

// trackingListener remembers the connections it accepted so that a scenario
// can sever them.
//
// net.Listener.Close only stops new accepts, and a flap that a
// already-connected client cannot observe is not a flap.
type trackingListener struct {
	net.Listener

	mu    sync.Mutex
	conns map[net.Conn]struct{}
}

func newTrackingListener(l net.Listener) *trackingListener {
	return &trackingListener{Listener: l, conns: make(map[net.Conn]struct{})}
}

func (t *trackingListener) Accept() (net.Conn, error) {
	c, err := t.Listener.Accept()
	if err != nil {
		return nil, err
	}

	tc := &trackedConn{Conn: c, owner: t}

	t.mu.Lock()
	t.conns[c] = struct{}{}
	t.mu.Unlock()

	return tc, nil
}

// closeConns severs every connection accepted on this listener.
func (t *trackingListener) closeConns() {
	t.mu.Lock()
	conns := make([]net.Conn, 0, len(t.conns))
	for c := range t.conns {
		conns = append(conns, c)
	}
	t.conns = make(map[net.Conn]struct{})
	t.mu.Unlock()

	for _, c := range conns {
		_ = c.Close()
	}
}

func (t *trackingListener) forget(c net.Conn) {
	t.mu.Lock()
	delete(t.conns, c)
	t.mu.Unlock()
}

// trackedConn removes itself from its listener when it closes, so a long-lived
// node does not accumulate a set of dead connections.
type trackedConn struct {
	net.Conn

	owner *trackingListener
}

func (c *trackedConn) Close() error {
	c.owner.forget(c.Conn)

	return c.Conn.Close()
}
