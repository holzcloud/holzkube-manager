package talos_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	mathrand "math/rand/v2"
	"net"
	"strconv"
	"strings"
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

			cc, err := talos.NewClusterClient(ctx, row.dialer, target, sim.ClientCreds(), talos.Mode{})
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

// TestAnAbandonedLogStreamIsReleasedByClose is WR-03.
//
// streamPolicy stores the deadline context's cancel on policyStream and invokes
// it from RecvMsg alone, on io.EOF or on an error. A caller that opens a stream
// and stops reading -- the operator closing the log panel -- reaches neither
// branch, so the context and the gRPC stream were held until the whole
// ClusterClient was closed. LogStream made that unavoidable rather than merely
// possible: it exposed Recv and nothing else, with no Close, no Context and no
// way to say "I am done with this".
//
// The assertion is the simulator's rather than an error return, for the reason
// every case in the contract suite is: a torn-down stream reads as clean
// success at the client. A stalled emitter records a wait when it begins and
// records how long it waited only when it ends, so "parked right now" and "was
// parked and has been released" are two different observations rather than one
// counter read twice.
//
// slow_log_consumer is injected because a bounded emitter finishes into the
// gRPC flow-control window without ever parking, and a stream that ended on its
// own is not an abandoned one.
func TestAnAbandonedLogStreamIsReleasedByClose(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "abandoned"})
	restore, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioSlowLogConsumer})
	if err != nil {
		t.Fatalf("Inject %s: %v", talossim.ScenarioSlowLogConsumer, err)
	}
	t.Cleanup(restore)

	cc := clientFor(t, nodeUnderTest{
		Dialer: talos.NewDirectDialer(sim.Port()),
		Creds:  sim.ClientCreds(),
		Target: talos.Target{
			Machine: model.MachineID("00000000-0000-0000-0000-0000000000e2"),
			Addr:    sim.Host(),
		},
	})

	// The caller's context stays alive for the whole test. Cancelling it is the
	// escape hatch this test exists to show is not the only one.
	stream, err := cc.Logs(t.Context(), "kubelet")
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}

	for range 4 {
		if _, err := stream.Recv(); err != nil {
			t.Fatalf("Recv while the stream should still be producing: %v", err)
		}
	}

	// From here nothing reads. The node fills the flow-control window and then
	// parks on a send that nobody will ever take.
	streams := sim.Streams()
	parked, ok := waitUntilParked(streams)
	if !ok {
		t.Fatal("the node never settled on a parked send after the caller stopped reading, so this " +
			"test is not exercising an abandoned stream")
	}

	if err := stream.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	released := func() bool { return streams.BlockedFor() != parked }
	if !eventually(released) {
		t.Fatal("the node is still parked on a send after the caller closed the stream: Close did " +
			"not reach the gRPC stream, so the context and the stream are held until the whole " +
			"client is closed")
	}

	// Closing twice is what a defer alongside an explicit close does, and it
	// must not be a way to break the client.
	if err := stream.Close(); err != nil {
		t.Fatalf("the second Close returned %v", err)
	}
}

// waitUntilParked waits for the emitter to be blocked on a send and to stay
// blocked, and returns the accumulated wait at that point.
//
// Staying blocked is the load-bearing half. A send that parks and is released a
// moment later is the flow-control window draining, not an abandoned stream,
// and a test that took the first BlockedSends it saw as the signal would be
// measuring the window rather than the leak.
func waitUntilParked(streams *talossim.Streamer) (time.Duration, bool) {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if streams.BlockedSends() == 0 {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		before := streams.BlockedFor()
		time.Sleep(250 * time.Millisecond)
		if streams.BlockedFor() == before {
			return before, true
		}
	}
	return 0, false
}

// eventually polls cond for up to three seconds. The bound is generous because
// a false negative here is a flake, while a false positive is not reachable:
// the counters it reads only ever move forward.
func eventually(cond func() bool) bool {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

// TestProbeDoesNotPropagateAnUnverifiedPeersHostname is WR-04.
//
// directDialer.Probe completes its handshake with InsecureSkipVerify -- correct
// on this path, because maintenance mode has no cluster PKI to verify against
// -- and then reads the hostname out of the peer's leaf certificate.
// Creds.Fingerprint exists as the trust anchor for that read but no pinning is
// performed yet (T-02-27), so until it is, whatever answers on the apid port
// chooses those bytes: their length, their character set and their content.
// Identity.Hostname flows into Candidate and onward to whatever consumes
// discovery, so it has to be bounded and checked where it is harvested.
//
// The 4 KiB name is the reported case. The rest are the shapes that get past a
// length check alone -- a terminal escape, a newline that splits a log line in
// two, a NUL, a path separator -- because the field is a string a UI and a log
// both render.
func TestProbeDoesNotPropagateAnUnverifiedPeersHostname(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		commonName string
		dnsNames   []string
		want       string
	}{
		{
			name:       "an ordinary hostname is kept",
			commonName: "node-01.cluster.example",
			want:       "node-01.cluster.example",
		},
		{
			name:       "a subject alternative name wins, as before",
			commonName: "node-01.cluster.example",
			dnsNames:   []string{"talos-a1b-2c3"},
			want:       "talos-a1b-2c3",
		},
		{
			name:       "four kilobytes of common name",
			commonName: strings.Repeat("A", 4096),
		},
		{
			name:       "a name one byte past the longest a DNS name can be",
			commonName: strings.Repeat("a", 254),
		},
		{
			name:       "a terminal escape",
			commonName: "node\x1b[2Jcluster",
		},
		{
			name:       "a newline, which splits one log line into two",
			commonName: "node\nlevel=INFO msg=\"nothing to see\"",
		},
		{
			name:       "a NUL",
			commonName: "node\x00.cluster.example",
		},
		{
			name:       "a path separator",
			commonName: "../../etc/hostname",
		},
		{
			name:       "a hostile subject alternative name over an innocent common name",
			commonName: "node-01.cluster.example",
			dnsNames:   []string{strings.Repeat("b", 4096)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			port := hostileTLSPort(t, tc.commonName, tc.dnsNames)
			d := talos.NewDirectDialer(port)

			id, err := d.Probe(t.Context(), talos.Target{Machine: "hostile", Addr: "127.0.0.1"})
			if err != nil {
				t.Fatalf("Probe: %v", err)
			}
			if id.Hostname != tc.want {
				t.Errorf("Identity.Hostname = %q (%d bytes), want %q", id.Hostname, len(id.Hostname), tc.want)
			}
			// Machine is holzkube's own identifier and is never the peer's to
			// choose. Stated here because it is the field a hostname that got
			// through would be mistaken for.
			if id.Machine != "hostile" {
				t.Errorf("Identity.Machine = %q, want the probed target's own %q", id.Machine, "hostile")
			}
			// The certificate is self-signed, which is what an unconfigured
			// node looks like -- and, being unverified, what anything can look
			// like. The verdict is unchanged by this commit and is asserted so
			// that a later pinning change has to come past it.
			if !id.Maintenance {
				t.Error("Identity.Maintenance is false for a self-signed peer")
			}
		})
	}
}

// hostileTLSPort serves a real TLS handshake with a self-signed certificate
// carrying exactly the subject the caller asks for.
//
// It is a whole listener rather than a unit test of the sanitiser because the
// question is whether the bytes survive the round trip: a name that crypto/x509
// refuses to marshal, or that the handshake drops, would make a unit test pass
// against a case that cannot happen, and a name that arrives intact is the case
// that can.
func hostileTLSPort(t *testing.T, commonName string, dnsNames []string) int {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
		DNSNames:              dnsNames,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate with common name of %d bytes: %v", len(commonName), err)
	}

	l, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
	})
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
				_, _ = io.Copy(io.Discard, c)
			}()
		}
	}()

	return portOf(t, l)
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
		port := 20000 + mathrand.IntN(10000)

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
