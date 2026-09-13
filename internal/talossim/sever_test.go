package talossim

// An internal test, unlike the rest of this package's tests, because what it
// asserts is the property of an unexported type that the observable behaviour
// above it depends on -- and the failure it guards is one that reproduced
// roughly one full-suite run in three while every test of the layer above
// passed.

import (
	"errors"
	"net"
	"testing"
	"time"
)

// TestASeveredListenerRefusesAConnectionTheKernelAlreadyCompleted is the fix
// for the flake that kept CI red.
//
// Closing a listening socket stops the kernel completing new handshakes. It
// says nothing about one completed a microsecond earlier, which is sitting in
// the accept queue: the serve loop takes it and serves it, on an address the
// node has already given up. A machine that has handed back its DHCP lease
// cannot do that, so the simulator must not either -- and when it did, the
// symptom was ip_changes_on_reboot failing with "the client at the abandoned
// address answered", which is the simulator producing the exact fault that
// scenario exists to rule out rather than catching it.
//
// The test stages it deterministically rather than hoping for the race: the
// connection is established before the sever and taken out of the queue after,
// which is the same order the failing run had and a window a timing test could
// only meet by accident.
func TestASeveredListenerRefusesAConnectionTheKernelAlreadyCompleted(t *testing.T) {
	t.Parallel()

	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	l := newTrackingListener(raw)
	defer l.Close() //nolint:errcheck // the listener is the subject, not the assertion

	// Completed by the kernel and not yet accepted -- the backlog is the whole
	// point.
	client, err := net.DialTimeout("tcp", l.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close() //nolint:errcheck // ditto

	l.closeConns()

	conn, err := l.Accept()
	if err == nil {
		_ = conn.Close()
		t.Fatal("a severed listener handed out a connection. The node has given this address up, " +
			"so anything served on it is an answer from a machine that is not there -- which is " +
			"the failure ip_changes_on_reboot exists to rule out")
	}
	if !errors.Is(err, net.ErrClosed) {
		t.Errorf("Accept after a sever returned %v, want net.ErrClosed; the serve loop treats a "+
			"non-closed error as worth retrying and would spin", err)
	}
}

// TestASeveredListenerClosesWhatItRefuses is the other half.
//
// Refusing to hand the connection upward is not enough: a socket nobody closes
// stays open, and the client then sees a connection that completed and then
// said nothing -- which is a silent node, the one state this whole simulator
// is careful to distinguish from a node that is gone.
func TestASeveredListenerClosesWhatItRefuses(t *testing.T) {
	t.Parallel()

	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	l := newTrackingListener(raw)
	defer l.Close() //nolint:errcheck // the listener is the subject

	client, err := net.DialTimeout("tcp", l.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close() //nolint:errcheck // ditto

	l.closeConns()
	if _, err := l.Accept(); err == nil {
		t.Fatal("the severed listener accepted")
	}

	// The peer has to notice. A read on a connection the far end closed returns
	// promptly; one nobody closed sits there until the deadline.
	if err := client.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("the client read from a connection the severed listener should have closed")
	} else if errors.Is(err, net.ErrClosed) || isTimeout(err) {
		t.Errorf("the connection was not closed by the node; the client sees a silent node rather "+
			"than one that has gone: %v", err)
	}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
