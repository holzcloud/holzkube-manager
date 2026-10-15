package configdrift_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/configdrift"
	invsvc "github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/metrics"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// adopted is a real inventory over one real node (the simulator): the cluster
// was imported the way an operator imports it, so the machine id, the hostname
// and the connector all come from the inventory and not from a fixture. The
// tests that follow (the metrics here, the HTTP routes of the later plans) build
// on it.
type adopted struct {
	cl      *talossim.Cluster
	sim     *talossim.Server
	st      *fsstore.Store
	inv     *invsvc.Service
	cluster model.Cluster
	machine model.Machine
	mc      *machineconfig.Service
}

func newAdoptedNode(t *testing.T) *adopted {
	t.Helper()

	cl, err := talossim.NewCluster("homelab", "https://192.168.1.41:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	sim, err := talossim.New(talossim.Options{Hostname: "cp-1", Cluster: cl, ControlPlane: true})
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
	inv := invsvc.New(invsvc.Deps{Store: st, Dialer: talos.NewDirectDialer(sim.Port()), Logger: logger})
	t.Cleanup(func() { _ = inv.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	fp, err := inv.Fingerprint(ctx, sim.Host())
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	rec, err := inv.Import(ctx, invsvc.ImportRequest{
		Name: "homelab", Talosconfig: cl.Talosconfig, Endpoint: sim.Host(), Fingerprint: fp,
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	machines, err := inv.MachinesOf(ctx, rec.ID)
	if err != nil || len(machines) != 1 {
		t.Fatalf("machines = %v, %v; want the one node", machines, err)
	}
	return &adopted{
		cl: cl, sim: sim, st: st, inv: inv, cluster: rec, machine: machines[0],
		mc: machineconfig.New(machineconfig.Deps{Connect: inv.Connect}),
	}
}

// deps is what the daemon hands the service, built from the inventory and the
// store and nothing else.
func (a *adopted) deps() configdrift.Deps {
	return configdrift.Deps{
		Clusters: func(ctx context.Context) ([]model.Cluster, error) {
			return a.st.Clusters().List(ctx)
		},
		Machines:    a.inv.MachinesOf,
		AllMachines: func(ctx context.Context) ([]model.Machine, error) { return a.st.Machines().List(ctx) },
		Read:        a.mc.ReadSnapshot,
		Baselines:   a.st.ConfigBaselines(),
	}
}

// service builds the service and gives the inventory its HealthFor.
func (a *adopted) service() *configdrift.Service {
	svc := configdrift.New(a.deps())
	a.inv.SetConfigDrift(svc.HealthFor)
	return svc
}

// thirdPartyChange is somebody else changing the node through a client of the
// inventory that has no observer: talosctl, another operator.
func (a *adopted) thirdPartyChange(t *testing.T, patch string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cc, err := a.inv.Connect(ctx, a.machine.ID)
	if err != nil {
		t.Fatalf("a third party cannot connect: %v", err)
	}
	defer func() { _ = cc.Close() }()
	active, err := cc.MachineConfigYAML(ctx)
	if err != nil {
		t.Fatalf("a third party cannot read the active configuration: %v", err)
	}
	patched, err := machineconfig.ApplyPatches(active, []string{patch})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	if _, err := cc.ApplyConfigurationWithMode(ctx, patched, "no-reboot"); err != nil {
		t.Fatalf("ApplyConfigurationWithMode: %v", err)
	}
}

// viewJSON is the cluster card as the API would serve it.
func viewJSON(t *testing.T, v invsvc.ClusterView) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return string(raw)
}

const (
	// tracerImage is what a third party sets the kubelet image to; the diff of
	// that change is the configuration content the cluster list must not carry.
	tracerImage = "ghcr.io/siderolabs/kubelet:v1.34.2"
	tracerPatch = "apiVersion: v1alpha1\nkind: KubeletConfig\nimage: " + tracerImage + "\n"
)

// TestTracerAReadingReachesTheClusterView goes from a reading through the real
// Talos client, the real store and the real inventory to the JSON of the
// cluster card: "not read yet" before the first pass (no config_drift at all),
// the numbers and the state after it, drift with a start once a third party
// changed the node, and never a word of the difference itself, because the
// cluster list is readable by a Reader (D-05, D-01).
func TestTracerAReadingReachesTheClusterView(t *testing.T) {
	t.Parallel()

	a := newAdoptedNode(t)
	svc := a.service()
	bg := context.Background()

	// Not read yet: no field at all, which the card says in words.
	cards, err := a.inv.Clusters(bg)
	if err != nil || len(cards) != 1 {
		t.Fatalf("Clusters = %v, %v; want the one cluster", cards, err)
	}
	if cards[0].ConfigDrift != nil {
		t.Fatalf("before the first pass ConfigDrift is %+v, want nil (\"not read yet\")", cards[0].ConfigDrift)
	}
	if strings.Contains(viewJSON(t, cards[0]), `"config_drift"`) {
		t.Fatal("before the first pass the cluster JSON carries a config_drift key")
	}

	// The first pass lays the baseline.
	svc.Tick(bg)
	for name, get := range map[string]func() (invsvc.ClusterView, error){
		"Clusters": func() (invsvc.ClusterView, error) {
			all, err := a.inv.Clusters(bg)
			if err != nil || len(all) != 1 {
				return invsvc.ClusterView{}, err
			}
			return all[0], nil
		},
		"Cluster": func() (invsvc.ClusterView, error) { return a.inv.Cluster(bg, a.cluster.ID) },
	} {
		v, err := get()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		h := v.ConfigDrift
		if h == nil {
			t.Fatalf("%s: after the first pass ConfigDrift is nil", name)
		}
		if h.Nodes != 1 || h.ReadNodes != 1 || h.Drifted != 0 {
			t.Errorf("%s: nodes %d, read %d, drifted %d; want 1, 1, 0", name, h.Nodes, h.ReadNodes, h.Drifted)
		}
		if h.CheckedAt.IsZero() {
			t.Errorf("%s: CheckedAt is not set", name)
		}
		if len(h.Items) != 1 {
			t.Fatalf("%s: %d items, want 1: %+v", name, len(h.Items), h.Items)
		}
		it := h.Items[0]
		if it.Node != "cp-1" || it.Machine != a.machine.ID || it.State != string(configdrift.StateBaselineCreated) {
			t.Errorf("%s: item %+v, want node cp-1, machine %s, state baseline-created", name, it, a.machine.ID)
		}
		if !it.Since.IsZero() || it.ReadAt.IsZero() || it.Sentence == "" {
			t.Errorf("%s: item %+v, want no start, a last good reading and a sentence", name, it)
		}
	}

	// The baseline holds the kubelet image the third party is about to change,
	// so that the check below for its absence says something.
	base, err := a.st.ConfigBaselines().Get(bg, a.machine.ID)
	if err != nil {
		t.Fatalf("the first pass laid no baseline: %v", err)
	}
	if !strings.Contains(base.Snapshot, "ghcr.io/siderolabs/kubelet") {
		t.Fatal("precondition: the snapshot does not name a kubelet image, so its absence from the view proves nothing")
	}

	// A third party changes the node.
	a.thirdPartyChange(t, tracerPatch)
	svc.Tick(bg)

	v, err := a.inv.Cluster(bg, a.cluster.ID)
	if err != nil {
		t.Fatalf("Cluster: %v", err)
	}
	if v.ConfigDrift == nil || v.ConfigDrift.Drifted != 1 {
		t.Fatalf("after a third party's change ConfigDrift is %+v, want one drifted node", v.ConfigDrift)
	}
	it := v.ConfigDrift.Items[0]
	if it.State != string(configdrift.StateDrift) || it.Since.IsZero() {
		t.Errorf("item %+v, want state drift with a start", it)
	}
	if v.ConfigDrift.Banner {
		t.Error("a drift seconds old brings the banner")
	}

	// And not a word of the difference.
	raw := viewJSON(t, v)
	for _, leak := range []string{"KubeletConfig", "kubelet:v1.34.2", tracerImage, "ghcr.io/siderolabs/kubelet", "snapshot"} {
		if strings.Contains(raw, leak) {
			t.Errorf("the cluster JSON carries %q; the list is readable by a Reader and the difference is configuration content:\n%s", leak, raw)
		}
	}
	if !strings.Contains(raw, `"config_drift"`) {
		t.Fatal("precondition: the cluster JSON carries no config_drift, so the checks above proved nothing")
	}
}

// TestAnEmptyPassSerialisesItsItemsAsAList: a cluster whose pass found no node
// is a cluster that was read and has nothing to say, and its items are an empty
// list. The interface reads null as the failure of a page.
func TestAnEmptyPassSerialisesItsItemsAsAList(t *testing.T) {
	t.Parallel()

	a := newAdoptedNode(t)
	d := a.deps()
	d.Machines = func(context.Context, model.ClusterID) ([]model.Machine, error) { return nil, nil }
	svc := configdrift.New(d)
	a.inv.SetConfigDrift(svc.HealthFor)
	bg := context.Background()

	svc.Tick(bg)

	v, err := a.inv.Cluster(bg, a.cluster.ID)
	if err != nil {
		t.Fatalf("Cluster: %v", err)
	}
	if v.ConfigDrift == nil {
		t.Fatal("a pass over a cluster with no node left ConfigDrift nil; the cluster was read")
	}
	if v.ConfigDrift.Nodes != 0 || v.ConfigDrift.ReadNodes != 0 || v.ConfigDrift.Drifted != 0 {
		t.Errorf("counts %+v, want zeros", v.ConfigDrift)
	}
	raw := viewJSON(t, v)
	if !strings.Contains(raw, `"items":[]`) {
		t.Errorf("an empty pass does not serialise its items as an empty list:\n%s", raw)
	}
	if strings.Contains(raw, `"items":null`) {
		t.Errorf("an empty pass serialises its items as null:\n%s", raw)
	}
}

// TestTheCountsFollowAPokedReadingBeforeThePassComes: a node that is poked is
// read between two passes, and the card says what that reading found. Numbers
// the pass kept would say "fine" until the next pass, three hours on.
func TestTheCountsFollowAPokedReadingBeforeThePassComes(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	fake := &fakeReader{}
	before, after := twoSnaps(t, n)
	clk := newClock()
	svc := n.service(t, withRead(fake.Read), withClock(clk))
	bg := context.Background()
	cluster := model.Cluster{ID: "c1", Name: "homelab"}

	fake.set(before, nil)
	svc.Tick(bg)
	h := svc.HealthFor(cluster, clk.Now())
	if h == nil || h.Nodes != 1 || h.Drifted != 0 {
		t.Fatalf("precondition: after the pass the view is %+v, want one node, no drift", h)
	}

	clk.Advance(time.Minute)
	fake.set(after, nil)
	svc.Poke("m1")
	svc.Flush(bg)

	h = svc.HealthFor(cluster, clk.Now())
	if h == nil || h.Drifted != 1 || h.ReadNodes != 1 || h.Nodes != 1 {
		t.Fatalf("after a poked reading the view is %+v, want 1 node, 1 read, 1 drifted", h)
	}
	if h.Items[0].State != string(configdrift.StateDrift) {
		t.Errorf("the item is %q, want drift", h.Items[0].State)
	}
	if !h.CheckedAt.Equal(clk.Now()) {
		t.Errorf("CheckedAt is %v, want the moment of the poked reading %v", h.CheckedAt, clk.Now())
	}
}

// TestAnUnreadableNodeCarriesNoErrorTextInTheList: the error of a failed reading
// can name an address or a path, and the cluster list is readable by a Reader.
// The operator's Detail keeps the text.
func TestAnUnreadableNodeCarriesNoErrorTextInTheList(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	fake := &fakeReader{}
	const secretish = "192.168.1.10:50000 /var/lib/holzkube-manager/config-baselines"
	fake.set(machineconfig.Snap{}, errors.New("dial tcp "+secretish+": connection refused"))
	svc := n.service(t, withRead(fake.Read))
	bg := context.Background()

	svc.Tick(bg)

	d := detail(t, svc)
	if d.State != configdrift.StateUnreadable || !strings.Contains(d.Sentence, "192.168.1.10") {
		t.Fatalf("precondition: Detail says %q: %q, want unreadable with the error in it", d.State, d.Sentence)
	}
	h := svc.HealthFor(model.Cluster{ID: "c1", Name: "homelab"}, time.Now())
	if h == nil || len(h.Items) != 1 {
		t.Fatalf("the view is %+v, want one item", h)
	}
	it := h.Items[0]
	if it.State != string(configdrift.StateUnreadable) {
		t.Fatalf("precondition: the item is %q, want unreadable", it.State)
	}
	for _, leak := range []string{"192.168.1.10", "/var/lib", "connection refused"} {
		if strings.Contains(it.Sentence, leak) {
			t.Errorf("the sentence in the cluster list carries %q: %q", leak, it.Sentence)
		}
	}
	if it.Sentence == "" {
		t.Error("the item has no sentence")
	}
}

// TestTheStatesOfTheViewAreTheStatesOfThePackage: model repeats the state strings
// because it imports nothing of the domain; this holds the two together.
func TestTheStatesOfTheViewAreTheStatesOfThePackage(t *testing.T) {
	t.Parallel()

	pairs := []struct {
		name     string
		pkg, mod string
	}{
		{"not-read", string(configdrift.StateNotRead), model.ConfigStateNotRead},
		{"baseline-created", string(configdrift.StateBaselineCreated), model.ConfigStateBaselineCreated},
		{"ok", string(configdrift.StateOK), model.ConfigStateOK},
		{"settling", string(configdrift.StateSettling), model.ConfigStateSettling},
		{"drift", string(configdrift.StateDrift), model.ConfigStateDrift},
		{"unreadable", string(configdrift.StateUnreadable), model.ConfigStateUnreadable},
		{"baseline-damaged", string(configdrift.StateBaselineDamaged), model.ConfigStateBaselineDamaged},
	}
	for _, p := range pairs {
		if p.pkg != p.name || p.mod != p.name {
			t.Errorf("state %q: configdrift says %q, model says %q", p.name, p.pkg, p.mod)
		}
	}
}

// TestTracerAReadingReachesTheMetrics extends the tracer to /metrics: the same
// real inventory, the exporter over its two lists, and no node asked at scrape
// time. A reading that matched is 0, a third party's change is 1.
func TestTracerAReadingReachesTheMetrics(t *testing.T) {
	t.Parallel()

	a := newAdoptedNode(t)
	svc := a.service()
	bg := context.Background()

	exp := metrics.New(metrics.Deps{
		Machines: a.inv.Machines,
		Clusters: a.inv.Clusters,
		Jobs:     func(context.Context) ([]model.Job, error) { return nil, nil },
	})
	scrape := func() string {
		t.Helper()
		var b strings.Builder
		if err := exp.Write(bg, &b); err != nil {
			t.Fatalf("Write: %v", err)
		}
		return b.String()
	}
	nodeSeries := `holzkube_node_config_drift{cluster="` + string(a.cluster.ID) + `",node="cp-1"}`
	clusterSeries := `holzkube_cluster_config_drift{cluster="` + string(a.cluster.ID) + `"}`

	if out := scrape(); strings.Contains(out, nodeSeries) || strings.Contains(out, clusterSeries) {
		t.Fatalf("before the first pass the export carries a drift series:\n%s", out)
	}

	svc.Tick(bg)
	out := scrape()
	if got, ok := sample(out, nodeSeries); !ok || got != "0" {
		t.Fatalf("after a reading that founded the baseline the node is %q (present %v), want 0", got, ok)
	}
	if got, ok := sample(out, clusterSeries); !ok || got != "0" {
		t.Errorf("the cluster is %q (present %v), want 0: its one node has a verdict", got, ok)
	}
	if got, ok := sample(out, `holzkube_cluster_config_nodes_judged{cluster="`+string(a.cluster.ID)+`"}`); !ok || got != "1" {
		t.Errorf("nodes judged is %q (present %v), want 1", got, ok)
	}
	if strings.Contains(out, string(a.machine.ID)) {
		t.Errorf("the machine id %s is in the export", a.machine.ID)
	}

	a.thirdPartyChange(t, tracerPatch)
	svc.Tick(bg)
	out = scrape()
	if got, ok := sample(out, nodeSeries); !ok || got != "1" {
		t.Errorf("after a third party's change the node is %q (present %v), want 1", got, ok)
	}
	if got, ok := sample(out, clusterSeries); !ok || got != "1" {
		t.Errorf("after a third party's change the cluster is %q (present %v), want 1", got, ok)
	}
}

// sample is the value text of one series, found by its name and labels.
func sample(out, series string) (string, bool) {
	for _, line := range strings.Split(out, "\n") {
		if rest, found := strings.CutPrefix(line, series+" "); found {
			return rest, true
		}
	}
	return "", false
}
