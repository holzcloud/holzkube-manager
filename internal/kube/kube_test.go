package kube_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// newCluster starts a simulated API server and returns a client that reached it
// the way the product will: a certificate minted from the cluster's own
// Kubernetes authority, which is what the store holds.
func newCluster(t *testing.T, opts kubesim.Options) (*kubesim.Server, *kube.Client) {
	t.Helper()

	sim, err := kubesim.New(opts)
	if err != nil {
		t.Fatalf("kubesim.New: %v", err)
	}
	t.Cleanup(sim.Close)

	caCrt, caKey := sim.AuthorityPEM()
	creds, err := kube.MintCreds(sim.Endpoint(), caCrt, caKey, time.Now())
	if err != nil {
		t.Fatalf("MintCreds: %v", err)
	}
	client, err := kube.New(creds)
	if err != nil {
		t.Fatalf("kube.New: %v", err)
	}
	return sim, client
}

// TestTheProductReachesAClusterWithACertificateItMintedItself is the whole
// path, and the path is what a fake clientset cannot test: mint from the stored
// authority, verify the API server against that same authority, present the
// certificate, be let in.
//
// It also pins the identity. The certificate says holzkube-manager and not
// admin, because the cluster's own audit log records the common name -- and a
// product that reused the admin kubeconfig Talos hands out would make that log
// say nothing about who acted.
func TestTheProductReachesAClusterWithACertificateItMintedItself(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{Version: "v1.34.1"})

	version, err := client.ServerVersion(ctx)
	if err != nil {
		t.Fatalf("ServerVersion: %v", err)
	}
	if version != "v1.34.1" {
		t.Errorf("version = %q, want v1.34.1", version)
	}

	if got := sim.AnonymousRequests(); got != 0 {
		t.Errorf("%d requests arrived with no client certificate; the product is talking to a "+
			"cluster's API server without saying who it is", got)
	}
	ids := sim.Identities()
	if len(ids) == 0 {
		t.Fatal("the server saw no client identity at all")
	}
	for _, id := range ids {
		if id != kube.ClientName {
			t.Errorf("the server saw %q; every call has to arrive as %q so the cluster's audit log "+
				"records who acted", id, kube.ClientName)
		}
	}
}

// TestACertificateFromAnotherAuthorityIsRefused is the other half of the same
// claim: the server really verifies, so the test above measures something.
//
// Without this, a simulator that accepted any certificate would let the test
// above pass against a product that authenticated with nothing.
func TestACertificateFromAnotherAuthorityIsRefused(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, err := kubesim.New(kubesim.Options{})
	if err != nil {
		t.Fatalf("kubesim.New: %v", err)
	}
	t.Cleanup(sim.Close)

	// A second cluster's authority: a real key pair, and the wrong one.
	other, err := kubesim.New(kubesim.Options{})
	if err != nil {
		t.Fatalf("kubesim.New (other): %v", err)
	}
	t.Cleanup(other.Close)

	caCrt, _ := sim.AuthorityPEM()
	otherCrt, otherKey := other.AuthorityPEM()

	creds, err := kube.MintCreds(sim.Endpoint(), otherCrt, otherKey, time.Now())
	if err != nil {
		t.Fatalf("MintCreds: %v", err)
	}
	// The API server is still verified against the RIGHT authority; only the
	// client certificate comes from the wrong one.
	creds.CACrt = caCrt

	client, err := kube.New(creds)
	if err != nil {
		t.Fatalf("kube.New: %v", err)
	}
	if _, err := client.ServerVersion(ctx); err == nil {
		t.Fatal("a certificate from another cluster's authority was accepted")
	}
}

// TestAClusterWithNoKubernetesAuthorityIsRefusedWithTheRepair: a cluster
// adopted from a talosconfig that carried no Kubernetes authority can be
// managed over the Talos API and cannot be reached here, and saying which is
// the difference between a bug report and a repair.
func TestAClusterWithNoKubernetesAuthorityIsRefusedWithTheRepair(t *testing.T) {
	t.Parallel()

	_, err := kube.MintCreds("https://192.168.0.110:6443", nil, nil, time.Now())
	if !errors.Is(err, kube.ErrNoKubernetesAuthority) {
		t.Fatalf("err = %v, want ErrNoKubernetesAuthority", err)
	}
	if !strings.Contains(err.Error(), "Talos API") {
		t.Errorf("the refusal does not say what the cluster can still be managed with: %v", err)
	}
}

// TestNodesCarryWhatKubernetesKnowsAndTheInventoryDoesNot is the first read,
// and the reason this half of the product exists.
//
// The inventory knows what the machine API says about a machine. This knows
// whether the kubelet registered, what the scheduler will do with it, and
// whether somebody cordoned it -- and those are the facts a person is looking
// for when a workload is not running.
func TestNodesCarryWhatKubernetesKnowsAndTheInventoryDoesNot(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Nodes: []kubesim.Node{
		{
			Name: "holzkube-01", Ready: corev1.ConditionTrue, Roles: []string{"control-plane"},
			KubeletVersion: "v1.34.1", OSImage: "Talos (v1.14.1)",
			InternalAddress: "192.168.0.110", ContainerRuntime: "containerd://2.1.4",
		},
		{Name: "holzkube-02", Ready: corev1.ConditionUnknown, Unschedulable: true},
	}})

	nodes, err := client.Nodes(ctx)
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(nodes))
	}

	first := nodes[0]
	if first.Name != "holzkube-01" || first.Ready != "True" {
		t.Errorf("first node = %+v", first)
	}
	if first.KubeletVersion != "v1.34.1" || first.InternalAddress != "192.168.0.110" {
		t.Errorf("the node's own facts did not survive the round trip: %+v", first)
	}
	if len(first.Roles) != 1 || first.Roles[0] != "control-plane" {
		t.Errorf("roles = %v, want [control-plane] read off the node-role label", first.Roles)
	}
	if first.Unschedulable {
		t.Error("a node nobody cordoned is reported as unschedulable")
	}

	// Unknown is a real answer and not NotReady: a kubelet that stopped
	// reporting is unheard from, which is the same distinction the inventory
	// draws between down and not yet asked (ledger 140).
	second := nodes[1]
	if second.Ready != "Unknown" {
		t.Errorf("a node whose kubelet stopped reporting reads %q, want Unknown", second.Ready)
	}
	if !second.Unschedulable {
		t.Error("a cordoned node is not reported as unschedulable, so the screen would offer to " +
			"schedule onto it")
	}
}

// TestACordonChangesWhatTheNextReadSays is the property this simulator was
// chosen for.
//
// talossim had to be taught exactly this in the same week (ledger 3): a fake
// that records a call and drops its effect lets a test pass that hardware would
// fail. Here the state changes underneath, and the client has to see it.
func TestACordonChangesWhatTheNextReadSays(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{Nodes: []kubesim.Node{{Name: "holzkube-01"}}})

	before, err := client.Nodes(ctx)
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}
	if before[0].Unschedulable {
		t.Fatal("the node started out cordoned")
	}

	if err := sim.SetUnschedulable("holzkube-01", true); err != nil {
		t.Fatalf("SetUnschedulable: %v", err)
	}

	after, err := client.Nodes(ctx)
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}
	if !after[0].Unschedulable {
		t.Error("the node's cordon did not reach the client, so a test of cordoning would pass " +
			"against a server that ignored it")
	}
}

// TestTheEndpointComesFromTheClustersOwnConfiguration: gluing :6443 onto the
// address this product was adopted through is right until the cluster puts its
// API server behind a virtual address -- which is the cluster where being wrong
// costs the most.
func TestTheEndpointComesFromTheClustersOwnConfiguration(t *testing.T) {
	t.Parallel()

	const config = `version: v1alpha1
machine:
  type: controlplane
cluster:
  id: abc
  controlPlane:
    endpoint: https://192.168.0.100:6443
`
	endpoint, err := kube.EndpointFromMachineConfig([]byte(config))
	if err != nil {
		t.Fatalf("EndpointFromMachineConfig: %v", err)
	}
	if endpoint != "https://192.168.0.100:6443" {
		t.Errorf("endpoint = %q, want the one the cluster's own configuration names", endpoint)
	}

	// The shape Talos 1.14 actually writes: the endpoint in a typed document of
	// its own, with no `cluster.controlPlane.endpoint` anywhere. A product that
	// read only the documented v1alpha1 field returned "no endpoint" for every
	// cluster written by the Talos the operator runs -- measured against
	// talossim's generated configuration, not read.
	const typed = `version: v1alpha1
machine:
  type: controlplane
cluster:
  id: abc
---
apiVersion: v1alpha1
kind: KubeClusterConfig
clusterName: homelab
endpoint: https://192.168.0.100:6443
`
	if got, err := kube.EndpointFromMachineConfig([]byte(typed)); err != nil || got != "https://192.168.0.100:6443" {
		t.Errorf("the endpoint in a KubeClusterConfig document gave %q, %v", got, err)
	}

	// A multi-document configuration, as Talos 1.8 and later write it.
	multi := config + "---\napiVersion: v1alpha1\nkind: HostnameConfig\nhostname: cp-1\n"
	if endpoint, err := kube.EndpointFromMachineConfig([]byte(multi)); err != nil || endpoint == "" {
		t.Errorf("a multi-document configuration gave %q, %v", endpoint, err)
	}

	if _, err := kube.EndpointFromMachineConfig([]byte("version: v1alpha1\nmachine:\n  type: worker\n")); !errors.Is(err, kube.ErrNoEndpoint) {
		t.Errorf("a configuration naming no endpoint = %v, want ErrNoEndpoint", err)
	}
}

// TestAPodThatIsRunningAndNotReadyReadsAsBoth is the single most misread state
// in Kubernetes, and the reason the read model carries both numbers.
//
// `Running` is the pod's own claim about its lifecycle. `1/2 ready` is whether
// its containers pass their probes. A screen that showed only the phase would
// call a broken workload healthy -- which is the same failure as calling a node
// that nobody has asked "not answering" (ledger 140), one layer up.
func TestAPodThatIsRunningAndNotReadyReadsAsBoth(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Pods: []kubesim.Pod{
		{
			Namespace: "default", Name: "api-7c9", Node: "holzkube-01",
			Phase: corev1.PodRunning, Ready: 1, Containers: 2, Restarts: 14,
			WaitReason: "CrashLoopBackOff",
		},
		{
			Namespace: "kube-system", Name: "kube-proxy-abc", Node: "holzkube-01",
			Phase: corev1.PodRunning, Ready: 1, Containers: 1,
		},
	}})

	pods, err := client.Pods(ctx, "")
	if err != nil {
		t.Fatalf("Pods: %v", err)
	}
	if len(pods) != 2 {
		t.Fatalf("pods = %d, want 2 across all namespaces", len(pods))
	}

	broken := pods[0]
	if broken.Phase != "Running" {
		t.Errorf("phase = %q, want Running -- which is what the pod claims", broken.Phase)
	}
	if broken.Ready != 1 || broken.Containers != 2 {
		t.Errorf("ready = %d of %d, want 1 of 2: the phase says Running and the workload is not",
			broken.Ready, broken.Containers)
	}
	if broken.Restarts != 14 {
		t.Errorf("restarts = %d, want 14 -- the number that separates broke once from breaking in "+
			"a loop", broken.Restarts)
	}
	if broken.Reason != "CrashLoopBackOff" {
		t.Errorf("reason = %q, want the container's own CrashLoopBackOff rather than the pod's "+
			"empty one", broken.Reason)
	}
	if broken.Node != "holzkube-01" {
		t.Errorf("node = %q, want the node it is on", broken.Node)
	}
}

// TestPodsCanBeAskedForOneNamespace: the screen defaults to every namespace,
// because the question is "what is broken" -- but a namespace filter has to
// reach the server rather than being applied in the browser, or a cluster with
// ten thousand pods sends all of them to filter three.
func TestPodsCanBeAskedForOneNamespace(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{Pods: []kubesim.Pod{
		{Namespace: "default", Name: "api-7c9"},
		{Namespace: "kube-system", Name: "kube-proxy-abc"},
	}})

	pods, err := client.Pods(ctx, "kube-system")
	if err != nil {
		t.Fatalf("Pods: %v", err)
	}
	if len(pods) != 1 || pods[0].Namespace != "kube-system" {
		t.Fatalf("pods = %+v, want only the kube-system one", pods)
	}
	if got := sim.Calls("GET /api/v1/namespaces/kube-system/pods"); got != 1 {
		t.Errorf("the namespaced path was requested %d times; the filter has to be the server's, "+
			"not the browser's", got)
	}

	names, err := client.Namespaces(ctx)
	if err != nil {
		t.Fatalf("Namespaces: %v", err)
	}
	if len(names) < 2 {
		t.Errorf("namespaces = %v, want at least the two every cluster has", names)
	}
}
