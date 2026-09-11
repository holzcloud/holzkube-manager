package machineconfig

import (
	"fmt"

	"github.com/siderolabs/crypto/x509"
	talosconfig "github.com/siderolabs/talos/pkg/machinery/config"
	"github.com/siderolabs/talos/pkg/machinery/config/generate"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// Generating a configuration for a node that does not have one yet (CFG-11).
//
// Two inputs, and both are decisions rather than defaults.
//
// The **secrets bundle** is the one adoption derived in phase 3. Generating a
// fresh one here would produce a configuration that is internally consistent
// and that no node of the cluster will accept -- the failure D-07 refuses to
// make reachable from the import path, arriving by a different door.
//
// The **version contract** is pinned per cluster. Talos's generator emits
// different documents for different versions, and the difference is not
// cosmetic: a configuration generated under a newer contract can use fields an
// older node rejects, and one generated under an older contract silently omits
// settings the cluster relies on. Letting it default to "whatever machinery
// was compiled against" would make a node's configuration depend on which
// build of holzkube-manager happened to generate it.

// GenerateInput is what a new node's configuration is built from.
type GenerateInput struct {
	// ClusterName and Endpoint are the cluster's identity and its Kubernetes
	// API address.
	ClusterName string
	Endpoint    string

	// Secrets is the cluster's bundle, as stored at adoption.
	Secrets model.ClusterSecrets

	// ControlPlane selects the role.
	ControlPlane bool

	// TalosVersion is the pinned contract, e.g. "v1.13". It is required: see
	// the note above for why there is no default.
	TalosVersion string

	// KubernetesVersion is the version the generated kubelet runs.
	KubernetesVersion string

	// Patches are applied after generation, in order.
	Patches []string
}

// Generate builds a machine configuration for a new node.
//
// The result is **not** redacted, because it is what gets applied: a redacted
// configuration would hand the node the marker as its certificate authority.
// Everything that shows it to a person goes through Redact separately.
func Generate(in GenerateInput) ([]byte, error) {
	if in.TalosVersion == "" {
		return nil, fmt.Errorf("machineconfig: a generated configuration must name its Talos version " +
			"contract; without one it would depend on which build of holzkube-manager produced it")
	}
	if in.KubernetesVersion == "" {
		return nil, fmt.Errorf("machineconfig: a generated configuration must name its Kubernetes version")
	}

	contract, err := talosconfig.ParseContractFromVersion(in.TalosVersion)
	if err != nil {
		return nil, fmt.Errorf("machineconfig: %q is not a Talos version contract: %w", in.TalosVersion, err)
	}

	bundle, err := bundleFrom(in.Secrets)
	if err != nil {
		return nil, err
	}

	input, err := generate.NewInput(in.ClusterName, in.Endpoint, in.KubernetesVersion,
		generate.WithSecretsBundle(bundle),
		generate.WithVersionContract(contract),
	)
	if err != nil {
		return nil, fmt.Errorf("machineconfig: build the generator input: %w", err)
	}

	role := machine.TypeWorker
	if in.ControlPlane {
		role = machine.TypeControlPlane
	}

	provider, err := input.Config(role)
	if err != nil {
		return nil, fmt.Errorf("machineconfig: generate the %s configuration: %w", role, err)
	}

	raw, err := provider.Bytes()
	if err != nil {
		return nil, fmt.Errorf("machineconfig: encode the generated configuration: %w", err)
	}

	if len(in.Patches) == 0 {
		return raw, nil
	}
	return ApplyPatches(raw, in.Patches)
}

// bundleFrom rebuilds machinery's bundle from the stored record.
//
// The record is holzkube-manager's own flat shape rather than a marshalled bundle, so
// this is where the two meet. A missing certificate authority is refused by
// name rather than passed on as an empty field: machinery would generate a
// configuration with an empty CA, which is a document that looks right and
// opens nothing.
func bundleFrom(s model.ClusterSecrets) (*secrets.Bundle, error) {
	if len(s.OSCACrt) == 0 || len(s.OSCAKey) == 0 {
		return nil, fmt.Errorf("machineconfig: the cluster's stored bundle has no Talos certificate " +
			"authority, so a node generated from it could not join")
	}
	if len(s.K8sCACrt) == 0 || len(s.K8sCAKey) == 0 {
		return nil, fmt.Errorf("machineconfig: the cluster's stored bundle has no Kubernetes " +
			"certificate authority")
	}

	return &secrets.Bundle{
		Clock: secrets.NewClock(),
		Cluster: &secrets.Cluster{
			ID:     s.TalosClusterID,
			Secret: s.ClusterSecret,
		},
		Secrets: &secrets.Secrets{
			BootstrapToken: s.BootstrapToken,
		},
		TrustdInfo: &secrets.TrustdInfo{
			Token: s.MachineToken,
		},
		Certs: &secrets.Certs{
			OS:                &x509.PEMEncodedCertificateAndKey{Crt: s.OSCACrt, Key: s.OSCAKey},
			K8s:               &x509.PEMEncodedCertificateAndKey{Crt: s.K8sCACrt, Key: s.K8sCAKey},
			K8sAggregator:     &x509.PEMEncodedCertificateAndKey{Crt: s.AggregatorCACrt, Key: s.AggregatorCAKey},
			Etcd:              &x509.PEMEncodedCertificateAndKey{Crt: s.EtcdCACrt, Key: s.EtcdCAKey},
			K8sServiceAccount: &x509.PEMEncodedKey{Key: s.ServiceAccountKey},
		},
	}, nil
}
