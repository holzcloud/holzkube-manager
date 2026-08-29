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
