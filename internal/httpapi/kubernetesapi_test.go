package httpapi_test

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

type kubernetesOverview struct {
	Cluster       string `json:"cluster"`
	ServerVersion string `json:"server_version"`
	Namespace     string `json:"namespace"`
	Namespaces    []string
	Nodes         []struct {
		Name          string `json:"name"`
		Ready         string `json:"ready"`
		Unschedulable bool   `json:"unschedulable"`
	} `json:"nodes"`
	Pods []struct {
		Namespace  string `json:"namespace"`
		Name       string `json:"name"`
		Phase      string `json:"phase"`
		Ready      int    `json:"ready"`
		Containers int    `json:"containers"`
		Restarts   int    `json:"restarts"`
		Reason     string `json:"reason"`
	} `json:"pods"`
}

// TestTheKubernetesOverviewAnswersWhatTheClusterSays walks the route the screen
// walks, with both fakes behind it: a simulated Talos node that carries the
// machine configuration, and a simulated API server that carries the cluster.
//
// The route is the first thing in this product that speaks to Kubernetes, so
// what it proves is the whole path: the endpoint out of the node's own
// configuration, a certificate minted from the authority the adoption derived,
// mTLS, and an answer that separates the pod's claim from its readiness.
func TestTheKubernetesOverviewAnswersWhatTheClusterSays(t *testing.T) {
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
		Pods: []kubesim.Pod{
			{
				Namespace: "default", Name: "api-7c9", Node: "cp-1",
				Phase: corev1.PodRunning, Ready: 1, Containers: 2, Restarts: 14,
				WaitReason: "CrashLoopBackOff",
			},
			{Namespace: "kube-system", Name: "kube-proxy-x", Node: "cp-1", Ready: 1, Containers: 1},
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

	h := newHarness(t, withInventory(func(st *fsstore.Store) *inventory.Service {
		return inventory.New(inventory.Deps{
			Store:  st,
			Dialer: talos.NewDirectDialer(sim.Port()),
			Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		})
	}))

	resp, raw := h.do(t, http.MethodPost, "/api/v1/setup", map[string]string{
		"username": testUser, "password": testPass,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: %d (%s)", resp.StatusCode, raw)
	}
	resp, raw = h.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": testUser, "password": testPass,
	})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login: %d (%s)", resp.StatusCode, raw)
	}

	resp, raw = h.do(t, http.MethodPost, "/api/v1/clusters/fingerprint",
		map[string]string{"endpoint": sim.Host()})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fingerprint: %d (%s)", resp.StatusCode, raw)
	}
	var fp struct {
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.Unmarshal(raw, &fp); err != nil {
		t.Fatalf("decode fingerprint: %v", err)
	}

	resp, raw = h.do(t, http.MethodPost, "/api/v1/clusters", map[string]string{
		"name": "homelab", "talosconfig": string(cl.Talosconfig),
		"endpoint": sim.Host(), "fingerprint": fp.Fingerprint,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("import: %d (%s)", resp.StatusCode, raw)
	}
	var cluster struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &cluster); err != nil {
		t.Fatalf("decode cluster: %v", err)
	}

	resp, raw = h.do(t, http.MethodGet, "/api/v1/clusters/"+cluster.ID+"/kubernetes", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("overview: %d (%s)", resp.StatusCode, raw)
	}
	var body kubernetesOverview
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode overview: %v (%s)", err, raw)
	}

	if body.ServerVersion != "v1.34.1" {
		t.Errorf("server_version = %q, want the API server's own answer", body.ServerVersion)
	}
	if len(body.Nodes) != 1 || body.Nodes[0].Ready != "True" {
		t.Errorf("nodes = %+v", body.Nodes)
	}
	if len(body.Pods) != 2 {
		t.Fatalf("pods = %d, want both namespaces when none is named", len(body.Pods))
	}

	var broken bool
	for _, p := range body.Pods {
		if p.Name != "api-7c9" {
			continue
		}
		broken = true
		if p.Phase != "Running" || p.Ready != 1 || p.Containers != 2 {
			t.Errorf("the pod reads %s %d/%d; Running with 1 of 2 ready is the state a screen "+
				"must not round to healthy", p.Phase, p.Ready, p.Containers)
		}
		if p.Restarts != 14 || p.Reason != "CrashLoopBackOff" {
			t.Errorf("restarts = %d, reason = %q; both are what separate broke once from "+
				"breaking in a loop", p.Restarts, p.Reason)
		}
	}
	if !broken {
		t.Error("the pod that is failing is not in the answer")
	}

	// The namespace filter reaches the API server rather than the browser.
	resp, raw = h.do(t, http.MethodGet,
		"/api/v1/clusters/"+cluster.ID+"/kubernetes?namespace=kube-system", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("overview (filtered): %d (%s)", resp.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode filtered: %v", err)
	}
	if body.Namespace != "kube-system" {
		t.Errorf("namespace = %q, want the filter echoed so a screen cannot mislabel the list",
			body.Namespace)
	}
	if len(body.Pods) != 1 || body.Pods[0].Namespace != "kube-system" {
		t.Errorf("filtered pods = %+v", body.Pods)
	}
	if got := api.Calls("GET /api/v1/namespaces/kube-system/pods"); got != 1 {
		t.Errorf("the namespaced path was requested %d times; the filter has to be the server's", got)
	}
}

// TestAClusterWhoseAPIServerIsGoneIsNotAClusterWithNoPods is INV-08 one layer
// up, and it is the reason this route does not answer an empty list.
//
// The API server is closed before the request. An empty list would say "this
// cluster has nothing running", which is a claim, and the operator would go
// looking for a workload rather than for an API server.
func TestAClusterWhoseAPIServerIsGoneIsNotAClusterWithNoPods(t *testing.T) {
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
	})
	if err != nil {
		t.Fatalf("kubesim.New: %v", err)
	}

	sim, err := talossim.New(talossim.Options{
		Hostname: "cp-1", Cluster: cl, ControlPlane: true, Bootstrapped: true,
	})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	h := newHarness(t, withInventory(func(st *fsstore.Store) *inventory.Service {
		return inventory.New(inventory.Deps{
			Store:  st,
			Dialer: talos.NewDirectDialer(sim.Port()),
			Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		})
	}))

	if resp, raw := h.do(t, http.MethodPost, "/api/v1/setup", map[string]string{
		"username": testUser, "password": testPass,
	}); resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: %d (%s)", resp.StatusCode, raw)
	}
	if resp, raw := h.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": testUser, "password": testPass,
	}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login: %d (%s)", resp.StatusCode, raw)
	}

	resp, raw := h.do(t, http.MethodPost, "/api/v1/clusters/fingerprint",
		map[string]string{"endpoint": sim.Host()})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fingerprint: %d (%s)", resp.StatusCode, raw)
	}
	var fp struct {
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.Unmarshal(raw, &fp); err != nil {
		t.Fatalf("decode fingerprint: %v", err)
	}
	resp, raw = h.do(t, http.MethodPost, "/api/v1/clusters", map[string]string{
		"name": "homelab", "talosconfig": string(cl.Talosconfig),
		"endpoint": sim.Host(), "fingerprint": fp.Fingerprint,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("import: %d (%s)", resp.StatusCode, raw)
	}
	var cluster struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &cluster); err != nil {
		t.Fatalf("decode cluster: %v", err)
	}

	// And now the API server is gone, the way it is when somebody is holding
	// this product to find out why.
	api.Close()

	resp, raw = h.do(t, http.MethodGet, "/api/v1/clusters/"+cluster.ID+"/kubernetes", nil)
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("a cluster whose API server is gone answered 200 (%s): an empty list is a claim "+
			"that nothing is running", raw)
	}
	p := decodeProblem(t, resp, raw)
	if !hasPrefix(p.Code, "upstream.") {
		t.Errorf("code = %q, want an upstream.* code: nothing in this process is broken, a "+
			"cluster is not answering", p.Code)
	}
	if p.Status != http.StatusBadGateway && p.Status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 502 or 503", p.Status)
	}
	_ = httpapi.CodeKubernetesUnreachable
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
