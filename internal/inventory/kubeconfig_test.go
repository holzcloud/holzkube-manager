package inventory_test

// The kubeconfig fetch (Omni parity phase 2).
//
// It is the one credential this product hands over without minting it: a
// talosconfig is rendered here from the stored bundle, and a kubeconfig is
// rendered by the node from the machine configuration it is running. So what
// these tests check is not "did some bytes arrive" but that the bytes are a
// kubeconfig for *this* cluster, with a client certificate the cluster's own
// Kubernetes CA actually signed.
//
// That is only checkable because talossim renders a real one rather than
// serving a fixture -- see internal/talossim/kubeconfig.go.

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/talossim"
	"gopkg.in/yaml.v3"
)

// kubeconfigShape is the part of a kubeconfig these tests read.
type kubeconfigShape struct {
	CurrentContext string `yaml:"current-context"`
	Clusters       []struct {
		Name    string `yaml:"name"`
		Cluster struct {
			Server                   string `yaml:"server"`
			CertificateAuthorityData string `yaml:"certificate-authority-data"`
		} `yaml:"cluster"`
	} `yaml:"clusters"`
	Users []struct {
		Name string `yaml:"name"`
		User struct {
			ClientCertificateData string `yaml:"client-certificate-data"`
			ClientKeyData         string `yaml:"client-key-data"`
		} `yaml:"user"`
	} `yaml:"users"`
}

func TestTheKubeconfigIsForThisClusterAndSignedByIt(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	// Bootstrapped, because an imported cluster is one that is already
	// running: a control-plane node that has never been bootstrapped has no
	// Kubernetes to hand out access to, and the simulator says so rather than
	// producing a usable-looking file for a cluster that does not exist.
	f := newFixture(t, talossim.Options{ControlPlane: true, Bootstrapped: true})
	c := f.importCluster(ctx, t)

	raw, err := f.svc.Kubeconfig(ctx, c.ID)
	if err != nil {
		t.Fatalf("Kubeconfig: %v", err)
	}

	var cfg kubeconfigShape
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("what came back is not a kubeconfig: %v\n%s", err, raw)
	}

	if len(cfg.Clusters) != 1 || len(cfg.Users) != 1 {
		t.Fatalf("the kubeconfig names %d clusters and %d users, want one of each",
			len(cfg.Clusters), len(cfg.Users))
	}
	if cfg.CurrentContext == "" {
		t.Error("the kubeconfig selects no context, so kubectl would need one named on every call")
	}

	// The endpoint, not some endpoint. A kubeconfig pointing at the wrong
	// cluster is the failure that looks like success until somebody applies
	// something.
	if got := cfg.Clusters[0].Cluster.Server; got != f.cluster.Endpoint {
		t.Errorf("the kubeconfig points at %q, want this cluster's endpoint %q",
			got, f.cluster.Endpoint)
	}

	caPEM := decodeField(t, cfg.Clusters[0].Cluster.CertificateAuthorityData)
	if string(caPEM) != string(f.cluster.Secrets.Certs.K8s.Crt) {
		t.Error("the kubeconfig carries a Kubernetes CA that is not this cluster's")
	}

	// And the client certificate chains to it. A self-signed certificate
	// pasted into the right field would satisfy every check above.
	clientPEM := decodeField(t, cfg.Users[0].User.ClientCertificateData)
	client := parseCert(t, clientPEM)
	ca := parseCert(t, caPEM)

	pool := x509.NewCertPool()
	pool.AddCert(ca)
	if _, err := client.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		t.Errorf("the client certificate is not signed by the cluster's Kubernetes CA: %v", err)
	}

	if len(decodeField(t, cfg.Users[0].User.ClientKeyData)) == 0 {
		t.Error("the kubeconfig carries no client key, so nothing can authenticate with it")
	}
}

// TestAKubeconfigIsRefusedForAClusterWithNoControlPlane is the refusal that
// happens before anything is dialled.
//
// A kubeconfig is rendered by a control-plane node. Asking a cluster that has
// none on record produces a sentence about that rather than a connection
// error, because the two send an operator to different places.
func TestAKubeconfigIsRefusedForAClusterWithNoControlPlane(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})

	_, err := f.svc.Kubeconfig(ctx, "a-cluster-nobody-adopted")
	if err == nil {
		t.Fatal("a kubeconfig was produced for a cluster this installation has no record of")
	}
	if !strings.Contains(err.Error(), "control-plane") {
		t.Errorf("the refusal %q does not say a control-plane node is what renders one", err)
	}
}

func decodeField(t *testing.T, b64 string) []byte {
	t.Helper()

	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("a kubeconfig field is not base64: %v", err)
	}
	return raw
}

func parseCert(t *testing.T, raw []byte) *x509.Certificate {
	t.Helper()

	block, _ := pem.Decode(raw)
	if block == nil {
		t.Fatalf("not PEM: %q", raw)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert
}
