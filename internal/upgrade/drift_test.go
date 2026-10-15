package upgrade_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/configdrift"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// TestAKubernetesUpgradeIsAnOwnApplyForTheDriftWatch runs a whole Kubernetes
// upgrade job, which applies several configurations per node, against a real
// inventory whose only link to the configuration drift watch is the observer
// Connect hangs on its clients (D-10, DRIFT-04). Afterwards the node has a
// configuration other than the baseline, and the watch calls it what it is: the
// baseline, from an apply, and never a drift.
func TestAKubernetesUpgradeIsAnOwnApplyForTheDriftWatch(t *testing.T) {
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

	// The watch, over the inventory; the observer is made known here and
	// nowhere else.
	reader := machineconfig.New(machineconfig.Deps{Connect: inv.Connect})
	watch := configdrift.New(configdrift.Deps{
		Clusters:    func(ctx context.Context) ([]model.Cluster, error) { return st.Clusters().List(ctx) },
		Machines:    inv.MachinesOf,
		AllMachines: func(ctx context.Context) ([]model.Machine, error) { return st.Machines().List(ctx) },
		Read:        reader.ReadSnapshot,
		Baselines:   st.ConfigBaselines(),
	})
	inv.SetApplyObserver(watch.ApplyObserver())

	deps := upgrade.Deps{
		Connect:  inv.Connect,
		Machines: inv.MachinesOf,
		KubeProxy: func(context.Context, model.ClusterID, string) (string, error) {
			return "kube-proxy rolled out", nil
		},
	}
	deps.Gate = upgrade.NewGate(deps.Connect, inv.ControlPlanesOf)
	upgrade.Register(engine, deps)

	watch.Tick(ctx)
	first, err := st.ConfigBaselines().Get(ctx, node)
	if err != nil {
		t.Fatalf("the first sight laid no baseline: %v", err)
	}
	if first.Source != model.BaselineFirstSight {
		t.Fatalf("precondition: the baseline is %q, want first-sight", first.Source)
	}

	j, err := engine.Submit(ctx, model.Job{
		Kind: upgrade.JobKindKubernetesUpgrade, Cluster: cluster.ID, Actor: "test",
		Params: upgrade.Request{To: "v1.35.2", Machines: []model.MachineID{node}}.Params(),
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

	// The job changed the configuration: without this the test would pass for a
	// job that applied nothing.
	now, err := reader.ReadSnapshot(ctx, node)
	if err != nil {
		t.Fatalf("ReadSnapshot: %v", err)
	}
	if now.Digest == first.Digest {
		t.Fatal("precondition: the node has the configuration of the baseline after the job, so the job changed nothing and the test proves nothing")
	}

	watch.Flush(ctx)
	check := func(when string) {
		t.Helper()
		d, err := watch.Detail(ctx, node)
		if err != nil {
			t.Fatalf("%s: Detail: %v", when, err)
		}
		if d.State != configdrift.StateOK {
			t.Fatalf("%s: the state after a Kubernetes upgrade is %q (%s), want ok", when, d.State, d.Sentence)
		}
		if d.Diff != nil || !d.DriftSince.IsZero() {
			t.Errorf("%s: the upgrade left a diff or a drift start: %+v", when, d)
		}
		rec, err := st.ConfigBaselines().Get(ctx, node)
		if err != nil {
			t.Fatalf("%s: Get: %v", when, err)
		}
		if rec.Source != model.BaselineApply {
			t.Errorf("%s: the baseline source is %q, want apply", when, rec.Source)
		}
		// The job applies several times per node: the last expectation wins,
		// and that is the configuration the node has.
		if rec.Digest != now.Digest {
			t.Errorf("%s: the baseline digest %s is not the node's %s", when, rec.Digest, now.Digest)
		}
	}
	check("after the flush")

	watch.Tick(ctx)
	check("after one more pass")
}
