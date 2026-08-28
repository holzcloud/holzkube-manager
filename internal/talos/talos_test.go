package talos_test

import (
	"context"
	"errors"
	"net"
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
func TestProbeReportsNotReachableYet(t *testing.T) {
	t.Parallel()

	port := closedPort(t)
	d := talos.NewDirectDialer(port)

	_, err := d.Probe(t.Context(), talos.Target{Machine: "absent", Addr: "127.0.0.1"})
	if err == nil {
		t.Fatal("probing a closed port succeeded")
	}
	if !errors.Is(err, talos.ErrNotReachableYet) {
		t.Fatalf("Probe of an unreachable target returned %v, which does not satisfy errors.Is(err, ErrNotReachableYet)", err)
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

// closedPort returns a loopback port that nothing is listening on, by opening a
// listener and closing it. Picking a number would be a race against whatever
// else runs on the machine.
func closedPort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("a TCP listener reported a %T address", l.Addr())
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
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
