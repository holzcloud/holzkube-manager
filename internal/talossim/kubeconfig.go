package talossim

// The Kubeconfig RPC (Omni parity phase 2).
//
// What is simulated here is both halves of the real thing, because a stub that
// streamed a hand-written string would let two different bugs pass. The RPC's
// shape is a server stream carrying a **gzipped tar** with one file in it --
// machinery unpacks it, and a client that got the framing wrong would look
// identical to one that got it right until it met a real node. And the content
// is a genuine kubeconfig, with a client certificate actually signed by this
// cluster's Kubernetes CA, so a test can check what the operator is handed
// rather than that some bytes arrived.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"time"

	cryptox509 "github.com/siderolabs/crypto/x509"
	"github.com/siderolabs/talos/pkg/machinery/api/common"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v3"
)

// kubeconfigCertTTL is how long the admin certificate in a rendered kubeconfig
// is good for. Talos issues a day; the number matters here only in that it is
// finite and a test can read it back.
const kubeconfigCertTTL = 24 * time.Hour

// Kubeconfig streams the cluster's admin kubeconfig.
//
// A node that has not been bootstrapped refuses, because there is no
// Kubernetes to configure access to yet -- and answering with a usable-looking
// file for a cluster that does not exist is the kind of simulator behaviour
// that moves a surprise from the test suite to the cluster.
func (m *machineService) Kubeconfig(_ *emptypb.Empty, stream machine.MachineService_KubeconfigServer) error {
	m.server.recordCall("Kubeconfig")

	if err := m.server.node.up(); err != nil {
		return err
	}

	n := m.server.node.snapshot()
	if !n.Bootstrapped {
		return status.Errorf(codes.FailedPrecondition,
			"talossim: %s has no Kubernetes yet: the node has not been bootstrapped", n.Hostname)
	}

	raw, err := m.server.renderKubeconfig()
	if err != nil {
		return status.Errorf(codes.Internal, "talossim: render a kubeconfig: %v", err)
	}

	packed, err := gzippedTar("kubeconfig", raw)
	if err != nil {
		return status.Errorf(codes.Internal, "talossim: pack the kubeconfig: %v", err)
	}

	// Two chunks, so a client that reads only the first message fails here
	// rather than against a real node. Talos streams whatever the window gives
	// it, which is never guaranteed to be one message.
	half := len(packed) / 2
	for _, chunk := range [][]byte{packed[:half], packed[half:]} {
		if err := stream.Send(&common.Data{Bytes: chunk}); err != nil {
			return err
		}
	}
	return nil
}

// renderKubeconfig builds an admin kubeconfig for the simulated cluster.
//
// The client certificate is minted from the cluster's Kubernetes CA rather
// than being a fixture, which is what makes the test that checks the chain a
// test of something.
func (s *Server) renderKubeconfig() ([]byte, error) {
	cl := s.opts.Cluster
	if cl == nil || cl.Secrets == nil || cl.Secrets.Certs == nil || cl.Secrets.Certs.K8s == nil {
		return nil, fmt.Errorf("talossim: this node belongs to no cluster with a Kubernetes CA")
	}
	ca := cl.Secrets.Certs.K8s

	authority, err := cryptox509.NewCertificateAuthorityFromCertificateAndKey(ca)
	if err != nil {
		return nil, fmt.Errorf("load the Kubernetes CA: %w", err)
	}

	pair, err := cryptox509.NewKeyPair(authority,
		cryptox509.CommonName("admin"),
		cryptox509.Organization("system:masters"),
		cryptox509.NotAfter(time.Now().Add(kubeconfigCertTTL)),
	)
	if err != nil {
		return nil, fmt.Errorf("issue an admin certificate: %w", err)
	}
	client := cryptox509.NewCertificateAndKeyFromKeyPair(pair)

	name := "admin@" + cl.Name
	doc := kubeconfigDoc{
		APIVersion:     "v1",
		Kind:           "Config",
		CurrentContext: name,
		Clusters: []namedCluster{{
			Name: cl.Name,
			Cluster: clusterEntry{
				Server:                   cl.Endpoint,
				CertificateAuthorityData: base64.StdEncoding.EncodeToString(ca.Crt),
			},
		}},
		Contexts: []namedContext{{
			Name:    name,
			Context: contextEntry{Cluster: cl.Name, User: name},
		}},
		Users: []namedUser{{
			Name: name,
			User: userEntry{
				ClientCertificateData: base64.StdEncoding.EncodeToString(client.Crt),
				ClientKeyData:         base64.StdEncoding.EncodeToString(client.Key),
			},
		}},
	}

	return yaml.Marshal(doc)
}

// The kubeconfig document, spelled out rather than imported.
//
// k8s.io/client-go would model this, and importing it to write four maps would
// put a Kubernetes client in the dependency graph of a product whose stated
// boundary is that it speaks the Talos machine API and nothing else. These are
// the fields a kubeconfig needs and no more.
type kubeconfigDoc struct {
	APIVersion     string         `yaml:"apiVersion"`
	Kind           string         `yaml:"kind"`
	CurrentContext string         `yaml:"current-context"`
	Clusters       []namedCluster `yaml:"clusters"`
	Contexts       []namedContext `yaml:"contexts"`
	Users          []namedUser    `yaml:"users"`
}

type namedCluster struct {
	Name    string       `yaml:"name"`
	Cluster clusterEntry `yaml:"cluster"`
}

type clusterEntry struct {
	Server                   string `yaml:"server"`
	CertificateAuthorityData string `yaml:"certificate-authority-data"`
}

type namedContext struct {
	Name    string       `yaml:"name"`
	Context contextEntry `yaml:"context"`
}

type contextEntry struct {
	Cluster string `yaml:"cluster"`
	User    string `yaml:"user"`
}

type namedUser struct {
	Name string    `yaml:"name"`
	User userEntry `yaml:"user"`
}

type userEntry struct {
	ClientCertificateData string `yaml:"client-certificate-data"`
	ClientKeyData         string `yaml:"client-key-data"`
}

// gzippedTar wraps one file the way the Kubeconfig RPC does.
func gzippedTar(name string, body []byte) ([]byte, error) {
	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o600,
		Size: int64(len(body)),
	}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(body); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
