package httpapi_test

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

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

// TestCordonAndDrainReachTheCluster walks the two routes slice 3 adds, through
// the same two fakes: the cordon synchronously, and the drain as the job it has
// to be.
//
// The cordon's answer is read BACK from the cluster rather than echoed, which
// this test pins: a cordon that did not take must not be able to look like one
// that did.
func TestCordonAndDrainReachTheCluster(t *testing.T) {
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
		Nodes:        []kubesim.Node{{Name: "cp-1", Ready: corev1.ConditionTrue}},
		Pods: []kubesim.Pod{
			{Namespace: "default", Name: "api-1", Node: "cp-1", OwnerKind: "ReplicaSet"},
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

	h := newHarness(t,
		withInventory(func(st *fsstore.Store) *inventory.Service {
			return inventory.New(inventory.Deps{
				Store:  st,
				Dialer: talos.NewDirectDialer(sim.Port()),
				Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
			})
		}),
		withJobs(),
	)

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
	// Both routes are destructive, so the window has to be open -- which is
	// itself part of what this test checks by not skipping it.
	if resp, raw := h.do(t, http.MethodPost, "/api/v1/auth/sudo",
		map[string]string{"password": testPass}); resp.StatusCode != http.StatusNoContent &&
		resp.StatusCode != http.StatusOK {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
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

	// An imported cluster is read-only, and these two routes are under that
	// lock: they rewrite what a cluster does with its workloads.
	base := "/api/v1/clusters/" + cluster.ID + "/kubernetes/nodes/cp-1"
	resp, raw = h.do(t, http.MethodPost, base+"/cordon", map[string]bool{"unschedulable": true})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cordon on a locked cluster: %d (%s), want 403", resp.StatusCode, raw)
	}

	if resp, raw := h.do(t, http.MethodPost, "/api/v1/clusters/"+cluster.ID+"/lock",
		map[string]bool{"locked": false}); resp.StatusCode != http.StatusOK {
		t.Fatalf("unlock: %d (%s)", resp.StatusCode, raw)
	}

	resp, raw = h.do(t, http.MethodPost, base+"/cordon", map[string]bool{"unschedulable": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cordon: %d (%s)", resp.StatusCode, raw)
	}
	var node struct {
		Name          string `json:"name"`
		Unschedulable bool   `json:"unschedulable"`
	}
	if err := json.Unmarshal(raw, &node); err != nil {
		t.Fatalf("decode node: %v", err)
	}
	if node.Name != "cp-1" || !node.Unschedulable {
		t.Errorf("the answer is %+v; it is read back from the cluster, so a cordon that did not "+
			"take must not look like one that did", node)
	}

	resp, raw = h.do(t, http.MethodPost, base+"/drain",
		map[string]bool{"force": false, "delete_local_data": false})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("drain: %d (%s), want 202 -- a drain is a job", resp.StatusCode, raw)
	}
	var accepted struct {
		Job struct {
			ID    string `json:"id"`
			Kind  string `json:"kind"`
			Steps []struct {
				Name string `json:"name"`
			} `json:"steps"`
		} `json:"job"`
	}
	if err := json.Unmarshal(raw, &accepted); err != nil {
		t.Fatalf("decode job: %v", err)
	}
	if accepted.Job.Kind != "node.drain" {
		t.Errorf("job kind = %q, want node.drain", accepted.Job.Kind)
	}

	// And the job really runs: the pod is gone from the cluster afterwards,
	// which is the assertion that separates submitting a job from doing the
	// work.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if len(api.Evicted()) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := api.Evicted(); len(got) != 1 || got[0] != "default/api-1" {
		t.Errorf("the API server evicted %v, want the one pod on that node", got)
	}
}

// adoptedClusterWithAPI stands up both fakes, a daemon, a session with the sudo
// window open, and one adopted, unlocked cluster.
//
// A helper rather than a fourth copy of eighty lines. The two tests above were
// written before there was a third caller; this exists because the manifest and
// proxy routes need exactly the same ground and a third copy would drift from
// the other two before it drifted from the product.
func adoptedClusterWithAPI(t *testing.T, opts kubesim.Options) (*harness, string, *kubesim.Server) {
	t.Helper()

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

	opts.Listener = ln
	opts.AuthorityCrt = cl.Secrets.Certs.K8s.Crt
	opts.AuthorityKey = cl.Secrets.Certs.K8s.Key
	if opts.Nodes == nil {
		opts.Nodes = []kubesim.Node{{Name: "cp-1", Ready: corev1.ConditionTrue}}
	}
	api, err := kubesim.New(opts)
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

	h := newHarness(t,
		withInventory(func(st *fsstore.Store) *inventory.Service {
			return inventory.New(inventory.Deps{
				Store:  st,
				Dialer: talos.NewDirectDialer(sim.Port()),
				Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
			})
		}),
		withJobs(),
	)

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
	if resp, raw := h.do(t, http.MethodPost, "/api/v1/auth/sudo",
		map[string]string{"password": testPass}); resp.StatusCode != http.StatusNoContent &&
		resp.StatusCode != http.StatusOK {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
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

	// An adoption is read-only until somebody unlocks it (INV-12), and the
	// routes these tests exercise are under that lock.
	if resp, raw := h.do(t, http.MethodPost, "/api/v1/clusters/"+cluster.ID+"/lock",
		map[string]bool{"locked": false}); resp.StatusCode != http.StatusOK {
		t.Fatalf("unlock: %d (%s)", resp.StatusCode, raw)
	}

	return h, cluster.ID, api
}

// TestAManifestIsPlannedThenAppliedThroughTheRoutes, and a conflict comes back
// as a conflict rather than as a success.
//
// The route answers 200 with the per-object result even when something failed,
// which is the part only an HTTP test can check: an apply of two objects where
// one conflicts has changed one thing, and `fully_applied` is what says so.
func TestAManifestIsPlannedThenAppliedThroughTheRoutes(t *testing.T) {
	h, id, api := adoptedClusterWithAPI(t, kubesim.Options{
		Objects:    []string{"configmaps/default/already-there"},
		ConflictOn: []string{"configmaps/default/contested"},
	})

	base := "/api/v1/clusters/" + id + "/kubernetes/manifest"
	manifest := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: already-there\n  " +
		"namespace: default\ndata:\n  a: \"1\"\n---\napiVersion: v1\nkind: ConfigMap\n" +
		"metadata:\n  name: fresh\n  namespace: default\ndata:\n  a: \"1\"\n"

	resp, raw := h.do(t, http.MethodPost, base+"/plan", map[string]string{"manifest": manifest})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("plan: %d (%s)", resp.StatusCode, raw)
	}
	var plan struct {
		Objects []struct {
			Name   string `json:"name"`
			Action string `json:"action"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if len(plan.Objects) != 2 || plan.Objects[0].Action != "update" ||
		plan.Objects[1].Action != "create" {
		t.Fatalf("plan = %+v, want update then create as the CLUSTER reports them", plan.Objects)
	}
	// A plan writes nothing, and this is the route-level version of that claim.
	if got := api.FieldManagers(); len(got) != 0 {
		t.Errorf("the plan route applied something: %v", got)
	}

	resp, raw = h.do(t, http.MethodPost, base+"/apply", map[string]string{"manifest": manifest})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d (%s)", resp.StatusCode, raw)
	}
	var result struct {
		Applied      []struct{ Name string } `json:"applied"`
		Failed       []struct{ Reason string }
		FullyApplied bool `json:"fully_applied"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.FullyApplied || len(result.Applied) != 2 {
		t.Fatalf("result = %+v, want both objects applied", result)
	}
	if api.Applied("configmaps", "default", "fresh") == nil {
		t.Error("the object is not in the cluster")
	}

	// And the conflicting case: 200 with fully_applied false, because one object
	// of the two did change and a single status code cannot say which.
	contested := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: contested\n  " +
		"namespace: default\ndata:\n  a: \"1\"\n---\napiVersion: v1\nkind: ConfigMap\n" +
		"metadata:\n  name: other\n  namespace: default\ndata:\n  a: \"1\"\n"

	resp, raw = h.do(t, http.MethodPost, base+"/apply", map[string]string{"manifest": contested})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apply with a conflict: %d (%s), want 200 with the per-object truth",
			resp.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.FullyApplied {
		t.Error("fully_applied is true although an object conflicted")
	}
	if len(result.Failed) != 1 || len(result.Applied) != 1 {
		t.Errorf("result = %+v, want one applied and one failed", result)
	}
	if got := api.Forced(); len(got) != 0 {
		t.Errorf("the route forced past another field manager: %v", got)
	}

	// A manifest that is not one is refused before anything is attempted.
	resp, raw = h.do(t, http.MethodPost, base+"/apply", map[string]string{"manifest": "   "})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("an empty manifest: %d (%s), want 400", resp.StatusCode, raw)
	}
}

// TestTheProxyRouteNeverServesTheWorkloadsOwnContentType is the security claim of
// slice 6, at the only boundary where it can be measured.
//
// The fake answers `text/html` with a script tag, which is what an ordinary
// workload serves. If this route passed that through, a pod's markup would be
// running in the daemon's origin -- the origin holding the operator's session
// cookie -- and every other test in this repository would still pass.
func TestTheProxyRouteNeverServesTheWorkloadsOwnContentType(t *testing.T) {
	h, id, _ := adoptedClusterWithAPI(t, kubesim.Options{
		Services: []kubesim.Service{{
			Namespace: "default", Name: "api", Type: "ClusterIP",
			Ports: []kubesim.ServicePort{{Name: "http", Port: 8080}},
		}},
		ProxyBodies: map[string]string{
			"default/api:8080/healthz": "<script>alert(document.cookie)</script>",
		},
	})

	route := "/api/v1/clusters/" + id + "/kubernetes/services/default/api/proxy"
	resp, raw := h.do(t, http.MethodPost, route,
		map[string]string{"port": "8080", "path": "/healthz"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("proxy: %d (%s)", resp.StatusCode, raw)
	}

	if got := resp.Header.Get("Content-Type"); !hasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q. The workload said text/html, and a route that repeated "+
			"that would be hosting a pod's markup on this daemon's origin", got)
	}
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q; without it a browser may sniff the body back "+
			"into HTML", got)
	}

	var answer struct {
		Status    int    `json:"status"`
		Body      string `json:"body"`
		Truncated bool   `json:"truncated"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The body is carried verbatim as a JSON string -- the operator sees what the
	// workload said -- and it is a string, not a document.
	if answer.Status != 200 || answer.Body != "<script>alert(document.cookie)</script>" {
		t.Errorf("answer = %+v, want the workload's own bytes as a string", answer)
	}

	// A path that would be rewritten is refused rather than quietly sent.
	resp, raw = h.do(t, http.MethodPost, route,
		map[string]string{"port": "8080", "path": "/healthz/../../secrets"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a traversal: %d (%s), want 400", resp.StatusCode, raw)
	}

	// And a service that is not there is a 404, not a 502 about an unreachable
	// cluster: the cluster answered perfectly well.
	resp, raw = h.do(t, http.MethodPost,
		"/api/v1/clusters/"+id+"/kubernetes/services/default/ghost/proxy",
		map[string]string{"port": "8080", "path": "/healthz"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("a missing service: %d (%s), want 404", resp.StatusCode, raw)
	}
}
