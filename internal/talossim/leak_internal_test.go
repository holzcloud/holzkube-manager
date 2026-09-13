package talossim

// New's doc makes a promise: "a running server or an error, never a half-built
// value: a simulator that is listening on one of its two listeners is worse
// than one that failed."
//
// It was not kept. Both listeners are open before the COSI state is seeded, and
// a seeding failure returned without closing either -- so every failed
// construction left a loopback port held for the life of the process.
//
// This is an internal test because the promise is about the listeners, and
// nothing outside this package can see them: New opens them itself. serveOn
// takes them as arguments for exactly this reason.

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/test/bufconn"
)

// closeWatcher reports whether the listener under it was closed.
type closeWatcher struct {
	net.Listener
	closed atomic.Bool
}

func (c *closeWatcher) Close() error {
	c.closed.Store(true)
	return c.Listener.Close()
}

// duplicateDisks cannot be seeded, and it is a mistake somebody can make rather
// than an injected fault: two disks with one device name are two COSI resources
// with one id, and the second Create refuses.
func duplicateDisks() Options {
	return Options{
		Hostname: "leaky",
		Disks: []DiskFixture{
			{Device: "nvme0n1", Size: 1 << 30, Model: "SIMULATED", Transport: "nvme"},
			{Device: "nvme0n1", Size: 2 << 30, Model: "SIMULATED", Transport: "nvme"},
		},
	}
}

func TestAFailedConstructionClosesBothListeners(t *testing.T) {
	t.Parallel()

	// The precondition. A fixture that quietly started seeding would make
	// everything below vacuous, and this is the second guard in this session
	// that would have passed while checking nothing.
	if sim, err := New(duplicateDisks()); err == nil {
		_ = sim.Close()
		t.Fatal("the fixture no longer fails to seed, so this test proves nothing")
	}

	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	tcp := &closeWatcher{Listener: raw}
	pipe := bufconn.Listen(bufSize)

	s, err := newServer(duplicateDisks())
	if err != nil {
		t.Fatalf("build the server value: %v", err)
	}

	if _, err := s.serveOn(tcp, pipe); err == nil {
		_ = s.Close()
		t.Fatal("a fixture that cannot be seeded constructed a server")
	}

	if !tcp.closed.Load() {
		t.Error("the loopback listener is still open after a failed construction; it holds its " +
			"port for the life of the process and New promises not to leave one behind")
	}
	// A bufconn that is still open blocks a dial forever waiting for an accept
	// that will never come, and a closed one refuses immediately. So the
	// question has to carry its own deadline: without one this test states the
	// finding by hanging, which is the worst way to state it.
	dialCtx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if _, err := pipe.DialContext(dialCtx); err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("the in-process pipe is still accepting after a failed construction (dial: %v)", err)
	}
}
