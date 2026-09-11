package talossim

import (
	"fmt"

	"github.com/siderolabs/talos/pkg/machinery/config/generate"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
)

// DefaultKubernetesVersion is what a simulated cluster's kubelet reports.
//
// It is a real Kubernetes version rather than a placeholder because the
// compatibility matrix is consulted against it: a made-up version would make
// every matrix test pass by not matching anything.
const DefaultKubernetesVersion = "1.34.1"

// Cluster is a simulated cluster's shared material: one secrets bundle and the
// two machine configurations derived from it.
//
// It exists because the thing holzkube-manager's import path actually does is read a
// control-plane node's machine configuration and derive the bundle back out of
// it. A simulator whose nodes serve a hand-written stub would let that code
// pass a test it could not pass against Talos; this generates the real
// documents with machinery's own generator, so the derivation runs against
// what a real node would serve.
//
// The worker configuration is not decoration either: D-05 refuses an import
// aimed at a worker, and that refusal is only testable if a node can actually
// serve a worker's config -- which is the same document minus the CA private
// key.
type Cluster struct {
	Name     string
	Endpoint string

	// Secrets is the bundle both configurations were generated from. A test
	// asserts against it that the bundle holzkube-manager derived at import is the same
	// one, field for field.
	Secrets *secrets.Bundle

	// Talosconfig is the admin client configuration for this cluster, in the
	// bytes an operator would hold in ~/.talos/config. It is what the adoption
	// path is handed, so a test drives the real parser over a real file rather
	// than over a fixture somebody wrote by hand.
	Talosconfig []byte

	// ControlPlaneConfig and WorkerConfig are the YAML a node of each type
	// serves as its active MachineConfig.
	ControlPlaneConfig []byte
	WorkerConfig       []byte
}

// NewCluster generates a simulated cluster.
//
// endpoint is the Kubernetes API endpoint, in the https://host:port form the
// generator expects; it never has to be reachable.
func NewCluster(name, endpoint string) (*Cluster, error) {
	// The system clock, not a fixed one: the CA validity window is computed
	// from it, and a bundle minted at a frozen past date would serve
	// certificates that are already expired -- which would make every
	// connection in every test fail for a reason that has nothing to do with
	// what the test is about.
	bundle, err := secrets.NewBundle(secrets.NewClock(), nil)
	if err != nil {
		return nil, fmt.Errorf("talossim: generate secrets bundle: %w", err)
	}

	in, err := generate.NewInput(name, endpoint, DefaultKubernetesVersion,
		generate.WithSecretsBundle(bundle))
	if err != nil {
		return nil, fmt.Errorf("talossim: generate input: %w", err)
	}

	cp, err := configBytes(in, machine.TypeControlPlane)
	if err != nil {
		return nil, err
	}
	worker, err := configBytes(in, machine.TypeWorker)
	if err != nil {
		return nil, err
	}

	tc, err := in.Talosconfig()
	if err != nil {
		return nil, fmt.Errorf("talossim: generate talosconfig: %w", err)
	}
	tcBytes, err := tc.Bytes()
	if err != nil {
		return nil, fmt.Errorf("talossim: encode talosconfig: %w", err)
	}

	return &Cluster{
		Name:               name,
		Endpoint:           endpoint,
		Secrets:            bundle,
		Talosconfig:        tcBytes,
		ControlPlaneConfig: cp,
		WorkerConfig:       worker,
	}, nil
}

// Config returns the machine configuration for one role.
func (c *Cluster) Config(controlPlane bool) []byte {
	if controlPlane {
		return c.ControlPlaneConfig
	}
	return c.WorkerConfig
}

func configBytes(in *generate.Input, t machine.Type) ([]byte, error) {
	provider, err := in.Config(t)
	if err != nil {
		return nil, fmt.Errorf("talossim: generate %s config: %w", t, err)
	}
	raw, err := provider.Bytes()
	if err != nil {
		return nil, fmt.Errorf("talossim: encode %s config: %w", t, err)
	}
	return raw, nil
}

// MemberFixture is one entry of the cluster membership a node reports.
type MemberFixture struct {
	ID           string
	Hostname     string
	Addresses    []string
	ControlPlane bool
}

// DiskFixture is one block device a node reports.
type DiskFixture struct {
	Device    string
	Size      uint64
	Model     string
	Serial    string
	Transport string
}

// clusterOSCA returns the cluster's Talos OS certificate authority, or nil for
// a node that belongs to no simulated cluster.
func clusterOSCA(c *Cluster) *pemPair {
	if c == nil || c.Secrets == nil || c.Secrets.Certs == nil || c.Secrets.Certs.OS == nil {
		return nil
	}
	return &pemPair{Crt: c.Secrets.Certs.OS.Crt, Key: c.Secrets.Certs.OS.Key}
}

// CACertPEM is the authority a client must trust to reach this node.
//
// It exists because a node built on a cluster's OS CA is reachable with
// credentials derived from that cluster's bundle, and a test proving the
// adoption path has to be able to build exactly those credentials.
func (s *Server) CACertPEM() []byte { return s.pki.caPEM() }
