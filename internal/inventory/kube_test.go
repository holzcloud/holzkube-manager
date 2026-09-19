package inventory_test

import (
	"errors"
	"log/slog"
	"net"
	"os"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestTheKubernetesClientIsAssembledFromWhatTheClusterSays is milestone v1.17's
// first slice, end to end and with nothing made up on the way.
//
// Three separate facts have to line up, and each one is a place this could have
// been written wrong without any test noticing:
//
//   - the Kubernetes authority comes out of the bundle this product DERIVED at
//     adoption, so the simulated API server is given that same authority. A
//     fake with an authority of its own would refuse a correctly working
//     product, and a product that minted from somewhere else would be accepted
//     by a fake that verified nothing.
//   - the endpoint comes out of a control-plane node's own machine
//     configuration, not from the address the cluster was adopted through. The
//     simulated cluster is built with the API server's URL as its endpoint, so
//     a product that glued :6443 onto the Talos address would reach nothing.
//   - the certificate says holzkube-manager, which is what the cluster's audit
//     log records.
func TestTheKubernetesClientIsAssembledFromWhatTheClusterSays(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)

	// The address first, so the order comes out right: the product reads the
	// API server's URL out of the machine configuration, so the cluster has to
	// be generated against a URL that already exists -- and this server then
	// has to verify certificates against that cluster's own authority.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	endpoint := "https://" + ln.Addr().String()

	cl, err := talossim.NewCluster("homelab", endpoint)
	if err != nil {
		_ = ln.Close()
		t.Fatalf("NewCluster: %v", err)
	}

	api, err := kubesim.New(kubesim.Options{
		Listener:     ln,
		AuthorityCrt: cl.Secrets.Certs.K8s.Crt,
		AuthorityKey: cl.Secrets.Certs.K8s.Key,
		Version:      "v1.34.1",
		Nodes: []kubesim.Node{
			{Name: "cp-1", Ready: corev1.ConditionTrue, Roles: []string{"control-plane"}},
		},
	})
	if err != nil {
		t.Fatalf("kubesim.New: %v", err)
	}
	t.Cleanup(api.Close)

	sim, err := talossim.New(talossim.Options{
		Hostname: "cp-1", Cluster: cl, ControlPlane: true, Bootstrapped: true,
	})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	st, err := fsstore.Open(dir)
	if err != nil {
		t.Fatalf("fsstore.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	inv := inventory.New(inventory.Deps{
		Store:  st,
		Dialer: talos.NewDirectDialer(sim.Port()),
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	t.Cleanup(func() { _ = inv.Close() })

	fp, err := inv.Fingerprint(ctx, sim.Host())
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	cluster, err := inv.Import(ctx, inventory.ImportRequest{
		Name: "homelab", Talosconfig: cl.Talosconfig, Endpoint: sim.Host(), Fingerprint: fp,
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	client, err := inv.KubeClient(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("KubeClient: %v", err)
	}

	version, err := client.ServerVersion(ctx)
	if err != nil {
		t.Fatalf("ServerVersion: %v. The product reached no Kubernetes API server with the "+
			"credentials it derived from the cluster's own configuration", err)
	}
	if version != "v1.34.1" {
		t.Errorf("version = %q, want v1.34.1", version)
	}

	nodes, err := client.Nodes(ctx)
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Name != "cp-1" {
		t.Fatalf("nodes = %+v, want the one this cluster has", nodes)
	}

	if got := api.AnonymousRequests(); got != 0 {
		t.Errorf("%d requests reached the API server with no client certificate", got)
	}
	for _, id := range api.Identities() {
		if id != kube.ClientName {
			t.Errorf("the API server saw %q, want %q -- the cluster's audit log records this name",
				id, kube.ClientName)
		}
	}
}

// TestAClusterWithNoKubernetesAuthorityCannotReachKubernetes: the refusal has
// to name what the cluster can still be managed with, because the answer is
// "everything else in this product".
func TestAClusterWithNoKubernetesAuthorityCannotReachKubernetes(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	cluster := f.importCluster(ctx, t)

	// The bundle without its Kubernetes half, which is what a talosconfig-only
	// adoption leaves behind.
	sec, err := f.store.ClusterSecrets().Get(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("ClusterSecrets: %v", err)
	}
	sec.K8sCACrt, sec.K8sCAKey = nil, nil
	if _, err := f.store.ClusterSecrets().Put(ctx, sec); err != nil {
		t.Fatalf("Put: %v", err)
	}

	_, err = f.svc.KubeClient(ctx, cluster.ID)
	if !errors.Is(err, kube.ErrNoKubernetesAuthority) {
		t.Fatalf("KubeClient = %v, want ErrNoKubernetesAuthority", err)
	}
	if !strings.Contains(err.Error(), "Talos API") {
		t.Errorf("the refusal does not say the cluster can still be managed over the Talos API: %v", err)
	}
}

// TestAClusterWithNoControlPlaneHasNobodyToAsk pins the other refusal: the
// endpoint is a question put to the cluster, and a cluster with no
// control-plane node in the inventory cannot answer it.
func TestAClusterWithNoControlPlaneHasNobodyToAsk(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	cluster := f.importCluster(ctx, t)

	machines, err := f.svc.Machines(ctx)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}
	for _, m := range machines {
		rec, err := f.store.Machines().Get(ctx, m.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		rec.Role = model.RoleWorker
		if _, err := f.store.Machines().Put(ctx, rec); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	_, err = f.svc.KubeClient(ctx, cluster.ID)
	if !errors.Is(err, kube.ErrNoEndpoint) {
		t.Fatalf("KubeClient = %v, want ErrNoEndpoint", err)
	}
	if !strings.Contains(err.Error(), "control-plane") {
		t.Errorf("the refusal does not say what is missing: %v", err)
	}
}
