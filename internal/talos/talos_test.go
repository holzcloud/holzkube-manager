package talos_test

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/holzcloud/holzkube/internal/model"
	"github.com/holzcloud/holzkube/internal/talos"
	"github.com/holzcloud/holzkube/internal/talossim"
)

// TestDialerSwap is the proof that the seam is a seam.
//
// Two transports that share nothing below the interface -- a real TCP socket
// and an in-process pipe that never touches the network stack -- are driven
// through one call site, and the call site is written once rather than twice.
// The identity of the body is the evidence: if swapping the Dialer required
// even one different line above it, this test would not compile as written, and
// the claim that a SideroLink tunnel is retrofittable without touching callers
// would be untested opinion (TRANS-02).
func TestDialerSwap(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{TalosVersion: "v1.12.4", Hostname: "swap-node"})

	rows := []struct {
		name   string
		dialer talos.Dialer
		kind   string
	}{
		{name: "direct over TCP", dialer: talos.NewDirectDialer(sim.Port()), kind: "direct"},
		{name: "in-process pipe", dialer: sim.Dialer(), kind: "fake"},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()

			// Everything from here down is the call site, and it is byte
			// identical between the two rows. Only row.dialer differs.
			target := talos.Target{
				Cluster: model.ClusterID("swap"),
				Machine: model.MachineID("00000000-0000-0000-0000-000000000001"),
				Addr:    sim.Host(),
			}

			cc, err := talos.NewClusterClient(ctx, row.dialer, target, sim.ClientCreds())
			if err != nil {
				t.Fatalf("NewClusterClient: %v", err)
			}
			defer func() {
				if err := cc.Close(); err != nil {
					t.Errorf("Close: %v", err)
				}
			}()

			version, err := cc.Version(ctx)
			if err != nil {
				t.Fatalf("Version: %v", err)
			}
			if version != "v1.12.4" {
				t.Errorf("Version() = %q, want %q", version, "v1.12.4")
			}
			if cc.Transport() != row.kind {
				t.Errorf("Transport() = %q, want %q", cc.Transport(), row.kind)
			}
		})
	}
}

// TestDirectDialerRefusesATargetWithNoAddress pins the refusal message's
// contract: a Target is identity, and the address is a hint that may be absent.
func TestDirectDialerRefusesATargetWithNoAddress(t *testing.T) {
	t.Parallel()

	d := talos.NewDirectDialer(0)
	if _, err := d.Resolve(t.Context(), talos.Target{Machine: "m"}); err == nil {
		t.Fatal("a target with no address hint resolved to an address")
	}
}

// TestProbeReportsNotReachableYet is the state-versus-failure distinction
// TRANS-02 rests on.
//
// A tunnel transport cannot probe a node that has not dialled in, and a scan
// cannot probe a node that is still booting. Both must be representable as a
// state rather than as an error, or the contact-direction reversal shows up in
// the UI as a fleet of broken machines.
//
// The second row is the booting node, and it is not a hypothetical: the TCP
// accept comes out of the kernel's listen backlog whether or not apid is
// serving behind it, so a node coming up completes the TCP handshake and then
// drops the connection before a byte of TLS. It reaches the probe as a broken
// pipe or a reset, and it is "not yet" rather than "broken".
//
// The row this test deliberately does not contain is a peer that spoke TLS and
// spoke it wrongly; that one is an error, and it is asserted as an error by
// TestProbeReportsATLSFaultAsAFailure. Without that companion this test would
// pass just as happily against a probe that called every failure a state.
func TestProbeReportsNotReachableYet(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name string
		port func(*testing.T) int
	}{
		{name: "nothing is listening", port: closedPort},
		{name: "the connection dies before the TLS handshake", port: acceptThenClosePort},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			d := talos.NewDirectDialer(row.port(t))

			_, err := d.Probe(t.Context(), talos.Target{Machine: "absent", Addr: "127.0.0.1"})
			if err == nil {
				t.Fatal("probing a target nothing is serving succeeded")
			}
			if !errors.Is(err, talos.ErrNotReachableYet) {
				t.Fatalf("Probe of an unreachable target returned %v, which does not satisfy errors.Is(err, ErrNotReachableYet)", err)
			}
		})
	}
}

// TestProbeReportsATLSFaultAsAFailure is the other side of the same line, and
// it is what keeps the test above from being satisfiable by a probe that
// reports everything as a state.
//
// A peer that answered the ClientHello with bytes that are not TLS has been
// heard from. Something is serving that port and it is not a Talos node, which
// an operator needs told -- reporting it as "not reachable yet" would leave a
// machine sitting in a pending state forever with nothing to look at.
func TestProbeReportsATLSFaultAsAFailure(t *testing.T) {
	t.Parallel()

	d := talos.NewDirectDialer(garbageTLSPort(t))

	_, err := d.Probe(t.Context(), talos.Target{Machine: "impostor", Addr: "127.0.0.1"})
	if err == nil {
		t.Fatal("probing a port serving something that is not TLS succeeded")
	}
	if errors.Is(err, talos.ErrNotReachableYet) {
		t.Fatalf("Probe of a peer that answered with non-TLS bytes returned %v, which satisfies "+
			"errors.Is(err, ErrNotReachableYet): a node that answered is reachable, and calling it a "+
			"state leaves it pending forever", err)
	}
}

// TestProbeReadsIdentityWithoutClusterPKI checks the other half of the Probe
// contract: it works with no credentials at all, which is what maintenance mode
// looks like before a node has been configured.
func TestProbeReadsIdentityWithoutClusterPKI(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "probe-node"})
	d := talos.NewDirectDialer(sim.Port())

	id, err := d.Probe(t.Context(), talos.Target{Machine: "probe", Addr: sim.Host()})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if id.Machine != "probe" {
		t.Errorf("Identity.Machine = %q, want the probed target's identity %q", id.Machine, "probe")
	}
	if id.Hostname != talossim.ServerName {
		t.Errorf("Identity.Hostname = %q, want %q from the presented certificate", id.Hostname, talossim.ServerName)
	}
	if id.Maintenance {
		t.Error("Identity.Maintenance is true for a node whose certificate is signed by an authority")
	}
}

// TestDiscoverySourcesShareOneCallSite is the DiscoverySource half of the same
// argument as TestDialerSwap: an outward-pushing source and an inward-pushing
// one feed one fan-in, and the fan-in is written once.
func TestDiscoverySourcesShareOneCallSite(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "found-node"})

	sources := []talos.DiscoverySource{
		talos.NewManualSource(
			talos.Target{Machine: "manual-a", Addr: "192.0.2.10"},
			talos.Target{Machine: "manual-b", Addr: "192.0.2.11"},
		),
		sim.DiscoverySource(),
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// One fan-in for every source, which is the point.
	out := make(chan talos.Candidate, 8)
	for _, src := range sources {
		if err := src.Run(ctx, out); err != nil {
			t.Fatalf("%s source: %v", src.Kind(), err)
		}
	}
	close(out)

	bySource := map[string]int{}
	for c := range out {
		bySource[c.Source]++
		if c.Target.Machine == "" {
			t.Errorf("%s emitted a candidate with no machine identity", c.Source)
		}
	}

	if bySource["manual"] != 2 {
		t.Errorf("manual source emitted %d candidates, want 2", bySource["manual"])
	}
	if bySource["fake"] != 1 {
		t.Errorf("fake source emitted %d candidates, want 1", bySource["fake"])
	}
}

// closedPort returns a loopback port that nothing is listening on and that
// nothing in this test binary will start listening on either.
//
// The obvious implementation -- listen on :0, read the port back, close --
// returns a port the kernel has just put back in the ephemeral pool, and this
// package opens dozens of :0 listeners in parallel: every talossim node, and
// every rebind and flap a scenario performs. One of them takes the returned
// port often enough to matter. It was the cause of a probe test that failed
// roughly one run in eight with a connection that was accepted and then died,
// which is a squatter's listener closing and not the absence this helper is
// asked for.
//
// So the port is drawn from below the ephemeral range instead -- which starts
// at 49152 on macOS and 32768 on Linux -- where a :0 bind never lands. It is
// still bound and released rather than merely picked, so the number returned is
// one the kernel agreed to hand out and not one an unrelated process is already
// using.
func closedPort(t *testing.T) int {
	t.Helper()

	for range 64 {
		port := 20000 + rand.IntN(10000)

		l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			continue
		}
		if err := l.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		return port
	}

	t.Fatal("no free loopback port below the ephemeral range after 64 attempts")
	return 0
}

// acceptThenClosePort serves a port that completes the TCP handshake and then
// drops the connection without speaking TLS.
//
// That is a node whose kernel has the socket and whose apid does not yet: the
// accept comes out of the listen backlog, and the client's ClientHello meets a
// closed socket. It reaches the probe as a broken pipe, a reset or an EOF
// depending on which side moved first, and all three are the same event.
func acceptThenClosePort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	return portOf(t, l)
}

// garbageTLSPort serves a port that answers the ClientHello with bytes that are
// not a TLS record. It is the peer that did speak, and spoke wrongly.
func garbageTLSPort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close() //nolint:errcheck // the fixture owns this connection

				// Read the ClientHello off the wire first, so the answer is an
				// answer rather than a race with the client's own write.
				_, _ = c.Read(make([]byte, 1024))
				_, _ = c.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))

				// Hold the connection open until the client hangs up, so the
				// probe classifies the reply and not the close that follows it.
				_, _ = io.Copy(io.Discard, c)
			}()
		}
	}()

	return portOf(t, l)
}

func portOf(t *testing.T, l net.Listener) int {
	t.Helper()

	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("a TCP listener reported a %T address", l.Addr())
	}
	return addr.Port
}

func newSim(t *testing.T, opts talossim.Options) *talossim.Server {
	t.Helper()

	sim, err := talossim.New(opts)
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() {
		if err := sim.Close(); err != nil {
			t.Errorf("talossim.Close: %v", err)
		}
	})
	return sim
}
