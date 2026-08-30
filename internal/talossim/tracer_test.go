package talossim_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestTracerRealClientReachesFakeNode is the end-to-end slice this phase is
// built on: the unmodified production machinery client, driven through the
// transport seam, completing a real Version RPC against an in-process Talos
// node over verified mTLS.
//
// Nothing here is forked or patched. talos.NewClusterClient constructs
// machinery's own client.New; the only thing the simulator supplies is a
// grpc.DialOption. If this test passes, hardware is not needed to exercise the
// client, which is what every phase from three onward depends on.
func TestTracerRealClientReachesFakeNode(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{TalosVersion: "v1.13.9", Hostname: "tracer-node"})

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	target := talos.Target{
		Cluster: model.ClusterID("tracer"),
		Machine: model.MachineID("00000000-0000-0000-0000-00000000dead"),
		Addr:    sim.Host(),
	}

	cc, err := talos.NewClusterClient(ctx, sim.Dialer(), target, sim.ClientCreds(), talos.Mode{})
	if err != nil {
		t.Fatalf("NewClusterClient through the in-process dialer: %v", err)
	}
	defer func() {
		if err := cc.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	got, err := cc.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if got != "v1.13.9" {
		t.Errorf("Version() = %q, want the version the simulator was configured with, %q", got, "v1.13.9")
	}

	if cc.Transport() != "fake" {
		t.Errorf("Transport() = %q, want %q", cc.Transport(), "fake")
	}

	// The RPC succeeding is not on its own evidence of mTLS: a server that
	// ignored client certificates would answer just the same. The server-side
	// record of a verified chain is what distinguishes the two.
	verified := sim.VerifiedClients()
	if len(verified) == 0 {
		t.Fatal("the simulator verified no client certificate; the RPC succeeded without mutual TLS")
	}
	if verified[0] != "holzkube-manager" {
		t.Errorf("verified client certificate CN = %q, want %q", verified[0], "holzkube-manager")
	}
}

// TestTracerRefusesUnverifiableClientCertificate is the negative control for the
// claim above.
//
// The credentials here trust the node's server certificate and differ in one
// respect only: their client certificate comes from an authority the node does
// not know. If this test ever passes a connection through, the simulator is not
// requiring client certificates and every mTLS claim made against it is empty.
func TestTracerRefusesUnverifiableClientCertificate(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{})

	creds, err := sim.UntrustedClientCreds()
	if err != nil {
		t.Fatalf("build untrusted credentials: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	target := talos.Target{Machine: model.MachineID("00000000-0000-0000-0000-00000000beef")}

	cc, err := talos.NewClusterClient(ctx, sim.Dialer(), target, creds, talos.Mode{})
	if err == nil {
		_ = cc.Close()
		t.Fatal("a client certificate the node cannot verify was accepted; ClientAuth is not RequireAndVerifyClientCert")
	}
	if len(sim.VerifiedClients()) != 0 {
		t.Errorf("the node recorded a verified client after a refused handshake: %v", sim.VerifiedClients())
	}
	if !strings.Contains(err.Error(), "liveness probe") {
		t.Errorf("the refusal surfaced as %v; the probe at construction is what is meant to catch it (D-05)", err)
	}
}

// TestTracerRefusesMaintenanceCredentialsOnTheClusterPath pins D-06 and T-02-01
// at the constructor: the cluster client is a distinct type from the
// maintenance one, and it will not be talked into being the other by a flag.
func TestTracerRefusesMaintenanceCredentialsOnTheClusterPath(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{})

	creds := sim.ClientCreds()
	creds.Kind = talos.CredMaintenance

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	target := talos.Target{Machine: model.MachineID("00000000-0000-0000-0000-00000000cafe")}

	if cc, err := talos.NewClusterClient(ctx, sim.Dialer(), target, creds, talos.Mode{}); err == nil {
		_ = cc.Close()
		t.Fatal("maintenance credentials built a cluster client")
	}

	insecure := sim.ClientCreds()
	insecure.TLS.InsecureSkipVerify = true
	if cc, err := talos.NewClusterClient(ctx, sim.Dialer(), target, insecure, talos.Mode{}); err == nil {
		_ = cc.Close()
		t.Fatal("InsecureSkipVerify was accepted on the cluster path (T-02-01)")
	}
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
