package upgrade_test

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// The Kubernetes upgrade run end to end, against a node that carries a real
// configuration (ledger 86, and the defect under it).
//
// Until this test the job had never run at all: only the function that built
// its patch was tested, and the simulator it would have run against stored the
// patch as the node's whole configuration. On a real node the first apply
// would have been refused. So this runs the job the way the composition root
// wires it and then asks the node what it now is.
func TestAKubernetesUpgradeMovesEveryPartAndKeepsTheConfiguration(t *testing.T) {
	upgrade.FastPolling(t)

	cl, err := talossim.NewCluster("homelab", "https://192.168.1.41:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	sim, err := talossim.New(talossim.Options{
		Hostname: "cp-1", Cluster: cl, ControlPlane: true, Bootstrapped: true,
		KubeletImage: "ghcr.io/siderolabs/kubelet:v" + talossim.DefaultKubernetesVersion,
	})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	st, err := fsstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	inv := inventory.New(inventory.Deps{Store: st, Dialer: talos.NewDirectDialer(sim.Port()), Logger: logger})
	t.Cleanup(func() { _ = inv.Close() })
	engine := jobs.New(jobs.Deps{Store: st, Logger: logger})
	t.Cleanup(func() { _ = engine.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

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
	machines, err := inv.MachinesOf(ctx, cluster.ID)
	if err != nil || len(machines) != 1 {
		t.Fatalf("machines = %v, %v; want the one node", machines, err)
	}
	node := machines[0].ID

	var (
		mu         sync.Mutex
		proxyImage string
	)
	deps := upgrade.Deps{
		Connect:  inv.Connect,
		Machines: inv.MachinesOf,
		KubeProxy: func(_ context.Context, _ model.ClusterID, image string) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			proxyImage = image
			return "kube-proxy rolled out", nil
		},
	}
	deps.Gate = upgrade.NewGate(deps.Connect, inv.ControlPlanesOf)
	upgrade.Register(engine, deps)

	readConfig := func() string {
		t.Helper()
		cc, err := inv.Connect(ctx, node)
		if err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer cc.Close() //nolint:errcheck // a test read
		rctx, rcancel, err := talos.WithClassDeadline(ctx, talos.MethodCOSIList)
		if err != nil {
			t.Fatal(err)
		}
		defer rcancel()
		raw, err := cc.MachineConfigYAML(rctx)
		if err != nil {
			t.Fatalf("MachineConfigYAML: %v", err)
		}
		return string(raw)
	}
	before := readConfig()

	const to = "v1.35.2"
	params := upgrade.Request{To: to, Machines: []model.MachineID{node}}.Params()
	j, err := engine.Submit(ctx, model.Job{
		Kind: upgrade.JobKindKubernetesUpgrade, Cluster: cluster.ID, Params: params, Actor: "test",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	for {
		cur, err := engine.Get(ctx, j.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if cur.State.Terminal() {
			j = cur
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("the job did not finish: %+v", cur)
		case <-time.After(20 * time.Millisecond):
		}
	}
	if j.State != model.JobSucceeded {
		for _, s := range j.Steps {
			t.Logf("step %q: %s %s", s.Name, s.State, s.Detail)
		}
		t.Fatalf("the Kubernetes upgrade ended %s", j.State)
	}

	after := readConfig()
	// The configuration is the node's whole configuration still, with the
	// images moved -- not the image lines alone.
	if len(after) < len(before)/2 {
		t.Fatalf("the configuration shrank from %d to %d bytes; the run replaced it with its patch",
			len(before), len(after))
	}
	for _, want := range []string{
		"kubelet:" + to, "kube-apiserver:" + to, "kube-controller-manager:" + to,
		"kube-scheduler:" + to, "kube-proxy:" + to,
	} {
		if !strings.Contains(after, want) {
			t.Errorf("the configuration after the run does not carry %s", want)
		}
	}
	if !strings.Contains(after, "machine:") || !strings.Contains(after, "token:") {
		t.Error("the configuration after the run lost its machine section or its tokens")
	}

	// And the node reports it: the kubelet on the new version, each static
	// pod back and ready on its new image.
	cc, err := inv.Connect(ctx, node)
	if err != nil {
		t.Fatal(err)
	}
	defer cc.Close() //nolint:errcheck // a test read
	rctx, rcancel, err := talos.WithClassDeadline(ctx, talos.MethodCOSIList)
	if err != nil {
		t.Fatal(err)
	}
	defer rcancel()
	facts, err := cc.NodeFacts(rctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := facts.KubernetesVersion(); got != strings.TrimPrefix(to, "v") && got != to {
		t.Errorf("the node reports Kubernetes %q after the run, want %s", got, to)
	}
	pods, err := cc.StaticPods(rctx)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, p := range pods {
		if p.Component != "" {
			seen[p.Component] = true
			if !strings.HasSuffix(p.Image, ":"+to) || !p.Ready {
				t.Errorf("%s reports %s ready=%v, want the new image and ready", p.Component, p.Image, p.Ready)
			}
		}
	}
	if len(seen) != 3 {
		t.Errorf("the node reports %d control-plane static pods, want 3: %v", len(seen), pods)
	}

	mu.Lock()
	defer mu.Unlock()
	if proxyImage != "registry.k8s.io/kube-proxy:"+to {
		t.Errorf("kube-proxy's DaemonSet was moved to %q, want registry.k8s.io/kube-proxy:%s", proxyImage, to)
	}
}

// The patch follows the configuration the node has: a v1.14 configuration
// carries each component as a document of its own and refuses the old field
// beside it, one generated before v1.14 has only the old field.
func TestTheImagePatchTakesTheShapeOfTheNodesConfiguration(t *testing.T) {
	t.Parallel()

	cl, err := talossim.NewCluster("homelab", "https://192.168.1.41:6443")
	if err != nil {
		t.Fatal(err)
	}
	withDoc := cl.Config(true)
	to, err := upgrade.ParseVersion("v1.35.2")
	if err != nil {
		t.Fatal(err)
	}

	got, err := upgrade.ComponentPatchFor("kube-apiserver", to, withDoc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "kind: KubeAPIServerConfig") || strings.Contains(got, "cluster:") {
		t.Errorf("against a configuration with a KubeAPIServerConfig document the patch is:\n%s\nwant that document", got)
	}

	// The same configuration without the document is the pre-v1.14 shape.
	var kept []string
	for _, doc := range strings.Split(string(withDoc), "\n---\n") {
		if !strings.Contains(doc, "kind: KubeAPIServerConfig") {
			kept = append(kept, doc)
		}
	}
	legacy := []byte(strings.Join(kept, "\n---\n"))
	got, err = upgrade.ComponentPatchFor("kube-apiserver", to, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "cluster:") || !strings.Contains(got, "apiServer:") || strings.Contains(got, "kind:") {
		t.Errorf("against a configuration without the document the patch is:\n%s\nwant the v1alpha1 field", got)
	}
}
