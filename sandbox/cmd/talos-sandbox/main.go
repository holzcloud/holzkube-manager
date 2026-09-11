// Command talos-sandbox brings a throwaway Talos cluster up in Docker and
// writes out what is needed to talk to it.
//
// It is the Tier-1 rung of the sandbox ladder: a real Talos node, running the
// real apid, reachable over the real wire. Tier 0 is `internal/talossim`, which
// is fast and always available and is a fake; Tier 2 is real hardware, which is
// slow and scarce. Tier 1 exists because a fake that nobody ever checks against
// the real thing drifts, and every later phase is built on that fake.
//
// It lives in this module and not in the product's, and the reason is
// mechanical rather than stylistic: the provisioner is in the Talos *root*
// module, which pulls in containerd, CNI and a large part of an operating
// system. `internal/depguard_test.go` in the product module asserts that none
// of it is ever in `holzkube-managerd`'s dependency graph, and the split is what makes
// that assertion possible.
//
// The product-module contract suite does not import this command. It reads the
// two files this writes -- an endpoint and a talosconfig -- which is data, not
// a dependency, and which is why the boundary holds in both directions.
//
// Usage:
//
//	talos-sandbox up   [--state DIR] [--name NAME] [--image REF]
//	talos-sandbox down [--state DIR] [--name NAME]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/config/generate"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
	"github.com/siderolabs/talos/pkg/provision"
	"github.com/siderolabs/talos/pkg/provision/providers/docker"
)

// Defaults. The image tag tracks the machinery version the product is pinned
// to: a Tier-1 run against a different Talos minor would be checking the fake
// against something the product does not claim to speak.
const (
	defaultName       = "holzkube-sandbox"
	defaultImage      = "ghcr.io/siderolabs/talos:v1.13.9"
	defaultKubernetes = "1.34.1"

	// The two files the contract suite reads. Their names are part of the
	// interface between the two modules, such as it is.
	endpointFile    = "endpoint"
	talosconfigFile = "talosconfig"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "talos-sandbox:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: talos-sandbox up|down [flags]")
	}

	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	name := fs.String("name", defaultName, "cluster name")
	state := fs.String("state", defaultStateDir(), "state directory")
	image := fs.String("image", defaultImage, "Talos container image")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}

	// No deadline on the outer context: bringing a cluster up pulls an image
	// on a cold machine, and a fixed ceiling here would abort a run that was
	// working. Interrupting it is the operator's job, and Ctrl-C tears down.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch os.Args[1] {
	case "up":
		return up(ctx, *name, *state, *image)
	case "down":
		return down(ctx, *name, *state)
	default:
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}

func defaultStateDir() string {
	if dir := os.Getenv("HOLZKUBE_SANDBOX_STATE"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "holzkube-sandbox")
	}
	return filepath.Join(home, ".holzkube", "sandbox")
}

func up(ctx context.Context, name, stateDir, image string) error {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	p, err := docker.NewProvisioner(ctx)
	if err != nil {
		return fmt.Errorf("docker provisioner: %w", err)
	}
	defer p.Close() //nolint:errcheck // a teardown error does not change the run's verdict

	cidr := netip.MustParsePrefix("10.5.0.0/24")
	gateway := netip.MustParseAddr("10.5.0.1")
	controlPlaneIP := netip.MustParseAddr("10.5.0.2")

	network := provision.NetworkRequest{
		Name:         name,
		CIDRs:        []netip.Prefix{cidr},
		GatewayAddrs: []netip.Addr{gateway},
		MTU:          1500,
	}

	genOpts, _ := p.GenOptions(network, nil)
	endpoint := p.GetInClusterKubernetesControlPlaneEndpoint(network, 6443)

	in, err := generate.NewInput(name, endpoint, defaultKubernetes, genOpts...)
	if err != nil {
		return fmt.Errorf("generate config input: %w", err)
	}

	cpConfig, err := in.Config(machine.TypeControlPlane)
	if err != nil {
		return fmt.Errorf("generate control-plane config: %w", err)
	}

	// One node, not three. Tier 1 exists to check the fake against real Talos,
	// and every assertion the contract suite makes is about one node's
	// behaviour: three would triple the time and the memory for nothing.
	req := provision.ClusterRequest{
		Name:           name,
		Network:        network,
		Image:          image,
		StateDirectory: stateDir,
		Nodes: provision.NodeRequests{
			{
				Name:     name + "-controlplane-1",
				IPs:      []netip.Addr{controlPlaneIP},
				Type:     machine.TypeControlPlane,
				Config:   cpConfig,
				Memory:   2048 * 1024 * 1024,
				NanoCPUs: 2_000_000_000,
			},
		},
	}

	cluster, err := p.Create(ctx, req, provision.WithBootlader(true))
	if err != nil {
		return fmt.Errorf("create cluster: %w", err)
	}

	talosconfig, err := in.Talosconfig()
	if err != nil {
		return fmt.Errorf("generate talosconfig: %w", err)
	}
	talosconfig.Contexts[talosconfig.Context].Endpoints = []string{controlPlaneIP.String()}
	talosconfig.Contexts[talosconfig.Context].Nodes = []string{controlPlaneIP.String()}

	raw, err := talosconfig.Bytes()
	if err != nil {
		return fmt.Errorf("encode talosconfig: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, talosconfigFile), raw, 0o600); err != nil {
		return fmt.Errorf("write talosconfig: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, endpointFile),
		[]byte(controlPlaneIP.String()+"\n"), 0o600); err != nil {
		return fmt.Errorf("write endpoint: %w", err)
	}

	fmt.Printf("cluster %q is up with %d node(s)\n", cluster.Info().ClusterName, len(cluster.Info().Nodes))
	fmt.Println()
	fmt.Println("Run the Tier-1 half of the contract suite with:")
	fmt.Printf("  HOLZKUBE_CONTRACT_ENDPOINT=%s \\\n", controlPlaneIP)
	fmt.Printf("  HOLZKUBE_CONTRACT_TALOSCONFIG=%s \\\n", filepath.Join(stateDir, talosconfigFile))
	fmt.Println("  go test ./internal/talos/ -run TestScenarioContractAgainstRealTalos -count=1")
	fmt.Println()
	fmt.Printf("Tear it down with: talos-sandbox down --name %s --state %s\n", name, stateDir)

	return nil
}

func down(ctx context.Context, name, stateDir string) error {
	p, err := docker.NewProvisioner(ctx)
	if err != nil {
		return fmt.Errorf("docker provisioner: %w", err)
	}
	defer p.Close() //nolint:errcheck // as above

	cluster, err := p.Reflect(ctx, name, stateDir)
	if err != nil {
		return fmt.Errorf("find cluster %q: %w", name, err)
	}

	destroyCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	if err := p.Destroy(destroyCtx, cluster); err != nil {
		return fmt.Errorf("destroy cluster: %w", err)
	}

	// The credentials go with the cluster. Leaving a talosconfig behind that
	// opens nothing is how a later run ends up pointed at a cluster that is not
	// there, with an error about the network rather than about the state.
	for _, f := range []string{talosconfigFile, endpointFile} {
		if err := os.Remove(filepath.Join(stateDir, f)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", f, err)
		}
	}

	fmt.Printf("cluster %q is gone\n", name)
	return nil
}
