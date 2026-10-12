package talos_test

import (
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

func certsByName(certs []talos.Certificate) map[string]talos.Certificate {
	out := map[string]talos.Certificate{}
	for _, c := range certs {
		out[c.Name] = c
	}
	return out
}

// TestCertificatesOfAControlPlane reads every role a control-plane node holds,
// and the dates come from the certificates themselves, not from a constant.
func TestCertificatesOfAControlPlane(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{
		ControlPlane:              true,
		IssuedCertificateValidity: 40 * 24 * time.Hour,
		AuthorityValidity:         200 * 24 * time.Hour,
	})
	ctx, cc := factsClient(t, sim)

	certs, err := cc.Certificates(ctx)
	if err != nil {
		t.Fatalf("Certificates: %v", err)
	}
	got := certsByName(certs)

	for _, name := range []string{
		"Talos API authority", "Talos API server", "etcd authority", "etcd server",
		"etcd peer", "Kubernetes authority", "Kubernetes API server",
	} {
		if _, ok := got[name]; !ok {
			t.Errorf("no %q among %d certificates", name, len(certs))
		}
	}

	within := func(name string, want time.Duration) {
		t.Helper()
		left := time.Until(got[name].NotAfter)
		if left < want-time.Hour || left > want+time.Hour {
			t.Errorf("%s has %v left, want about %v", name, left, want)
		}
	}
	within("etcd peer", 40*24*time.Hour)
	within("Kubernetes API server", 40*24*time.Hour)
	within("etcd authority", 200*24*time.Hour)

	if got["etcd authority"].Kind != talos.CertificateAuthority || got["etcd peer"].Kind != talos.CertificateIssued {
		t.Errorf("kinds wrong: authority=%q peer=%q", got["etcd authority"].Kind, got["etcd peer"].Kind)
	}
}

// TestCertificatesOfAWorkerHaveNoEtcd: a node without etcd or a Kubernetes API
// server is not a failure and is not reported as holding those.
func TestCertificatesOfAWorkerHaveNoEtcd(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{})
	ctx, cc := factsClient(t, sim)

	certs, err := cc.Certificates(ctx)
	if err != nil {
		t.Fatalf("Certificates: %v", err)
	}
	for _, c := range certs {
		if strings.HasPrefix(c.Name, "etcd") || strings.HasPrefix(c.Name, "Kubernetes") {
			t.Errorf("a worker reports %q", c.Name)
		}
	}
	if _, ok := certsByName(certs)["Talos API server"]; !ok {
		t.Error("a worker has a Talos API server certificate and it was not read")
	}
}
