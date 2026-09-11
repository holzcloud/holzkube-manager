package talos_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// factsClient dials a simulated node over the in-process pipe and returns a
// cluster client plus a context with the phase's usual budget.
func factsClient(t *testing.T, sim *talossim.Server) (context.Context, *talos.ClusterClient) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)

	cc, err := talos.NewClusterClient(ctx, sim.Dialer(), talos.Target{
		Cluster: model.ClusterID("facts"),
		Machine: model.MachineID("00000000-0000-0000-0000-000000000001"),
		Addr:    sim.Host(),
	}, sim.ClientCreds(), talos.Mode{})
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}
	t.Cleanup(func() {
		if err := cc.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return ctx, cc
}

// TestNodeFactsReadsTheInventoryFields is INV-06's read half: every field the
// node detail screen shows comes back from one pass over the resource state.
func TestNodeFactsReadsTheInventoryFields(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{
		Hostname:     "facts-node",
		TalosVersion: "v1.13.9",
		SchematicID:  strings.Repeat("ab", 32),
		CPUs:         2,
		MemoryMiB:    32768,
		Disks: []talossim.DiskFixture{
			{Device: "nvme0n1", Size: 1 << 40, Model: "SIM 1TB", Transport: "nvme"},
			{Device: "sda", Size: 500 << 30, Model: "SIM SATA", Transport: "sata"},
		},
	})

	ctx, cc := factsClient(t, sim)

	facts, err := cc.NodeFacts(ctx)
	if err != nil {
		t.Fatalf("NodeFacts: %v", err)
	}

	if facts.UUID == "" {
		t.Fatal("NodeFacts returned no UUID; the inventory is keyed on it")
	}
	if facts.Hostname != "facts-node" {
		t.Errorf("Hostname = %q, want %q", facts.Hostname, "facts-node")
	}
	if len(facts.CPUs) != 2 {
		t.Errorf("CPUs = %d, want 2", len(facts.CPUs))
	}
	if facts.MemoryMiB != 32768 {
		t.Errorf("MemoryMiB = %d, want 32768", facts.MemoryMiB)
	}
	if len(facts.BlockDevices) != 2 {
		t.Errorf("BlockDevices = %d, want 2", len(facts.BlockDevices))
	}
	if len(facts.Links) == 0 {
		t.Error("Links is empty; INV-06 asks for the node's network interfaces")
	}
	if len(facts.Addresses) == 0 {
		t.Error("Addresses is empty")
	}
	if want := strings.Repeat("ab", 32); facts.SchematicID != want {
		t.Errorf("SchematicID = %q, want %q", facts.SchematicID, want)
	}
	if got := facts.KubernetesVersion(); got != "v"+talossim.DefaultKubernetesVersion {
		t.Errorf("KubernetesVersion() = %q, want %q", got, "v"+talossim.DefaultKubernetesVersion)
	}
}

// TestSchematicIsEmptyWhenTheNodeReportsNone pins D-12: a node not installed
// from a Factory image has no schematic, and holzkube-manager says so rather than
// deriving one from the Talos version.
func TestSchematicIsEmptyWhenTheNodeReportsNone(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "no-factory-image"})
	ctx, cc := factsClient(t, sim)

	facts, err := cc.NodeFacts(ctx)
	if err != nil {
		t.Fatalf("NodeFacts: %v", err)
	}
	if facts.SchematicID != "" {
		t.Fatalf("SchematicID = %q for a node that reports no schematic extension; "+
			"a guessed value here is a phase 9 upgrade that strips the node's extensions and reports success",
			facts.SchematicID)
	}
}

// TestKubernetesVersionComesFromTheKubeletImage pins D-20's mechanism: the
// version is the kubelet's own image tag, read node-side, and a digest
// reference reports nothing rather than a sha256 string pretending to be a
// version.
func TestKubernetesVersionComesFromTheKubeletImage(t *testing.T) {
	t.Parallel()

	rows := []struct {
		image string
		want  string
	}{
		{image: "ghcr.io/siderolabs/kubelet:v1.34.1", want: "v1.34.1"},
		{image: "ghcr.io/siderolabs/kubelet", want: ""},
		{image: "ghcr.io/siderolabs/kubelet@sha256:0123456789abcdef", want: ""},
		{image: "", want: ""},
	}

	for _, row := range rows {
		if got := (talos.NodeFacts{KubeletImage: row.image}).KubernetesVersion(); got != row.want {
			t.Errorf("KubernetesVersion() for %q = %q, want %q", row.image, got, row.want)
		}
	}
}

// TestMembersIsEmptyWithoutDiscovery pins D-08's fallback condition: a cluster
// whose discovery service is switched off has no cluster.Member resources, and
// that is a supported configuration rather than a failure.
func TestMembersIsEmptyWithoutDiscovery(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "no-discovery"})
	ctx, cc := factsClient(t, sim)

	members, err := cc.Members(ctx)
	if err != nil {
		t.Fatalf("Members on a node with no membership resources: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("Members = %v, want none", members)
	}
}

// TestMembersReportsTheCluster is D-08's happy path: after adoption the
// inventory fills itself from the membership the control-plane node knows,
// rather than from five addresses typed by hand.
func TestMembersReportsTheCluster(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{
		Hostname: "cp-1",
		Members: []talossim.MemberFixture{
			{ID: "cp-1", Hostname: "cp-1", Addresses: []string{"192.168.1.41"}, ControlPlane: true},
			{ID: "worker-1", Hostname: "worker-1", Addresses: []string{"192.168.1.42"}},
		},
	})
	ctx, cc := factsClient(t, sim)

	members, err := cc.Members(ctx)
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("Members = %v, want two", members)
	}

	byID := map[string]talos.Member{}
	for _, m := range members {
		byID[m.ID] = m
	}
	if !byID["cp-1"].ControlPlane {
		t.Error("cp-1 is not reported as a control-plane member")
	}
	if byID["worker-1"].ControlPlane {
		t.Error("worker-1 is reported as a control-plane member")
	}
	if got := byID["worker-1"].Addresses; len(got) != 1 || got[0] != "192.168.1.42" {
		t.Errorf("worker-1 addresses = %v", got)
	}
}
