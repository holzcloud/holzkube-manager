package inventory_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/health"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// fixture is one simulated cluster, one node in it, and an inventory service
// wired to reach that node the way the product would.
type fixture struct {
	svc     *inventory.Service
	store   store.Store
	sim     *talossim.Server
	cluster *talossim.Cluster
}

func newFixture(t *testing.T, opts talossim.Options) *fixture {
	t.Helper()

	return newFixtureWith(t, opts, 0)
}

// newFixtureWith is newFixture with the heartbeat named.
//
// The interval is a parameter rather than a field on talossim.Options because
// it is not a property of the simulated node: how often this product looks is
// this product's business, and putting it on the simulator would invite a test
// to configure a node by configuring its observer. Zero takes the default.
func newFixtureWith(t *testing.T, opts talossim.Options, heartbeat time.Duration) *fixture {
	t.Helper()

	return newFixtureWrapped(t, opts, heartbeat, nil)
}

// newFixtureWrapped is newFixtureWith with the store handed through a
// decorator before the service sees it.
//
// It exists for the one class of test that cannot be written against the
// persistence seam from outside: a test about what happens when two writers
// race for the same record needs to choose the moment the second one arrives,
// and a real store offers no way to ask for that moment. wrap may be nil.
func newFixtureWrapped(
	t *testing.T,
	opts talossim.Options,
	heartbeat time.Duration,
	wrap func(store.Store) store.Store,
) *fixture {
	t.Helper()

	cl, err := talossim.NewCluster("homelab", "https://192.168.1.41:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}

	opts.Cluster = cl
	if opts.Hostname == "" {
		opts.Hostname = "cp-1"
	}
	sim, err := talossim.New(opts)
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() {
		if err := sim.Close(); err != nil {
			t.Errorf("talossim.Close: %v", err)
		}
	})

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	st, err := fsstore.Open(dir)
	if err != nil {
		t.Fatalf("fsstore.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("store.Close: %v", err)
		}
	})

	seen := store.Store(st)
	if wrap != nil {
		seen = wrap(st)
	}

	svc := inventory.New(inventory.Deps{
		Store:     seen,
		Heartbeat: heartbeat,
		// The direct dialer against the simulator's real loopback listener:
		// the fingerprint probe opens its own TLS connection, so the
		// in-process pipe would have nothing to read a certificate from.
		Dialer: talos.NewDirectDialer(sim.Port()),
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	t.Cleanup(func() {
		if err := svc.Close(); err != nil {
			t.Errorf("inventory.Close: %v", err)
		}
	})

	return &fixture{svc: svc, store: st, sim: sim, cluster: cl}
}

func (f *fixture) importCluster(ctx context.Context, t *testing.T) model.Cluster {
	t.Helper()

	fp, err := f.svc.Fingerprint(ctx, f.sim.Host())
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}

	c, err := f.svc.Import(ctx, inventory.ImportRequest{
		Name:        "homelab",
		Talosconfig: f.cluster.Talosconfig,
		Endpoint:    f.sim.Host(),
		Fingerprint: fp,
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	return c
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestImportAdoptsAControlPlaneNode is the adoption path end to end (D-01
// through D-07): a talosconfig and an address go in, and what comes out is a
// cluster whose secrets bundle was derived from the node's own machine
// configuration and proven by reconnecting under a certificate minted from it.
func TestImportAdoptsAControlPlaneNode(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})

	c := f.importCluster(ctx, t)

	if c.Origin != model.OriginImported {
		t.Errorf("Origin = %q, want %q", c.Origin, model.OriginImported)
	}
	if !c.Locked {
		t.Error("an imported cluster is not locked; D-21 adopts a cluster the operator already depends on read-only")
	}
	if c.ClientCertNotAfter.IsZero() {
		t.Error("the cluster records no client-certificate expiry, so D-23 has nothing to warn about")
	}

	// D-02: the bundle is a hard precondition, so it must be on disk.
	sec, err := f.store.ClusterSecrets().Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("the adopted cluster has no stored secrets bundle: %v", err)
	}
	if len(sec.OSCAKey) == 0 {
		t.Error("the stored bundle has no Talos CA private key, which is the one thing it exists to hold")
	}
	if len(sec.ClientCrt) == 0 || len(sec.ClientKey) == 0 {
		t.Error("the stored bundle has no client certificate, so the connectivity proof used something else")
	}

	// D-01: the derived bundle is the cluster's own, not a fresh one.
	if string(sec.OSCACrt) != string(f.cluster.Secrets.Certs.OS.Crt) {
		t.Error("the derived Talos CA is not the cluster's own; a generated one would look right and open nothing")
	}
	if sec.ClusterSecret != f.cluster.Secrets.Cluster.Secret {
		t.Error("the derived cluster secret does not match the cluster's own")
	}

	// D-08: the inventory filled itself.
	machines, err := f.svc.Machines(ctx)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}
	if len(machines) != 1 {
		t.Fatalf("the adoption recorded %d machines, want the node it was adopted through", len(machines))
	}
	if machines[0].Role != model.RoleControlPlane {
		t.Errorf("the adopted node's role is %q, want controlplane", machines[0].Role)
	}
}

// TestImportRefusesAWorker pins D-05. A worker's machine configuration carries
// the Talos CA certificate without its private key, so no bundle can be
// derived -- and a half-adopted cluster is a state this product refuses to
// have.
func TestImportRefusesAWorker(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{Hostname: "worker-1", ControlPlane: false})

	fp, err := f.svc.Fingerprint(ctx, f.sim.Host())
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}

	_, err = f.svc.Import(ctx, inventory.ImportRequest{
		Name:        "homelab",
		Talosconfig: f.cluster.Talosconfig,
		Endpoint:    f.sim.Host(),
		Fingerprint: fp,
	})
	if !errors.Is(err, inventory.ErrNotControlPlane) {
		t.Fatalf("Import against a worker returned %v, want ErrNotControlPlane", err)
	}
	if !strings.Contains(err.Error(), "worker") {
		t.Errorf("the refusal reads %q, which does not tell the operator to name a control-plane node", err)
	}

	clusters, err := f.store.Clusters().List(ctx)
	if err != nil {
		t.Fatalf("list clusters: %v", err)
	}
	if len(clusters) != 0 {
		t.Fatalf("a refused import left %d cluster(s) behind", len(clusters))
	}
}

// TestImportRefusesAnUnconfirmedFingerprint pins D-03: nothing trusts a node
// before the operator has looked at its certificate.
func TestImportRefusesAnUnconfirmedFingerprint(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})

	_, err := f.svc.Import(ctx, inventory.ImportRequest{
		Name:        "homelab",
		Talosconfig: f.cluster.Talosconfig,
		Endpoint:    f.sim.Host(),
		Fingerprint: strings.Repeat("AA:", 31) + "AA",
	})
	if !errors.Is(err, talos.ErrFingerprintMismatch) {
		t.Fatalf("Import with a wrong fingerprint returned %v, want ErrFingerprintMismatch", err)
	}
}

// TestNodeLevelFactsSurviveEtcdAndKubernetesBeingDown is ARCHITECTURE
// Pattern 6's checkable sentence, taken literally (D-14).
//
// Without this test the Kubernetes dependency creeps back within three phases
// and the dashboard goes blank in exactly the outage it exists for.
func TestNodeLevelFactsSurviveEtcdAndKubernetesBeingDown(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	c := f.importCluster(ctx, t)

	machines, err := f.svc.Machines(ctx)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}
	id := machines[0].ID

	// A first pass with everything healthy, so there is something to lose.
	f.svc.Refresh(ctx, id)

	restoreEtcd, err := f.sim.Inject(talossim.Scenario{Name: talossim.ScenarioEtcdDown})
	if err != nil {
		t.Fatalf("inject etcd_down: %v", err)
	}
	defer restoreEtcd()

	restoreK8s, err := f.sim.Inject(talossim.Scenario{Name: talossim.ScenarioK8sDown})
	if err != nil {
		t.Fatalf("inject k8s_down: %v", err)
	}
	defer restoreK8s()

	f.svc.Refresh(ctx, id)

	v, err := f.svc.Machine(ctx, id)
	if err != nil {
		t.Fatalf("Machine: %v", err)
	}

	nodeLevel := map[string]health.Field[string]{
		"talos_version": v.TalosVersion,
		"manufacturer":  v.Manufacturer,
		"product_name":  v.ProductName,
	}
	for name, f := range nodeLevel {
		if f.Level != health.LevelNode {
			t.Errorf("%s is at level %v, want node", name, f.Level)
		}
		if !f.Available {
			t.Errorf("%s went unavailable when etcd and Kubernetes did; every LevelNode fact must survive both", name)
		}
	}
	if !v.MemoryMiB.Available || !v.Disks.Available || !v.Interfaces.Available {
		t.Error("hardware facts went unavailable when etcd and Kubernetes did")
	}

	if v.KubernetesVersion.Available {
		t.Error("the Kubernetes version is still reported as confirmed with Kubernetes down")
	}
	if v.KubernetesVersion.StaleSince == nil {
		t.Error("the Kubernetes version lost its stale_since, so the UI cannot say how old it is")
	}
	if v.KubernetesVersion.Value == "" {
		t.Error("the Kubernetes version dropped its last known value; that is indistinguishable from null in the UI")
	}
	if v.Stage != health.StageDegraded {
		t.Errorf("Stage = %v with the node answering and its upper layers down, want degraded", v.Stage)
	}

	_ = c
}

// TestForgettingAMachineNeverTouchesTheNode pins D-11 and INV-09: the record
// goes only on an explicit action, and nothing is done to the machine.
func TestForgettingAMachineNeverTouchesTheNode(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	f.importCluster(ctx, t)

	machines, err := f.svc.Machines(ctx)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}
	id := machines[0].ID

	before := f.sim.Calls("Reset") + f.sim.Calls("Reboot") + f.sim.Calls("Shutdown")

	if err := f.svc.ForgetMachine(ctx, id); err != nil {
		t.Fatalf("ForgetMachine: %v", err)
	}

	if _, err := f.svc.Machine(ctx, id); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("the machine is still in the inventory: %v", err)
	}
	if after := f.sim.Calls("Reset") + f.sim.Calls("Reboot") + f.sim.Calls("Shutdown"); after != before {
		t.Fatal("forgetting a record reached the node; phase 3 deletes the record and touches nothing")
	}
}

// TestAnImportedClusterIsLockedAndCanBeUnlocked covers INV-12 and D-21/D-22's
// service half: the lock is state on the cluster, and CheckLock is what a
// route asks.
func TestAnImportedClusterIsLockedAndCanBeUnlocked(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	c := f.importCluster(ctx, t)

	if err := f.svc.CheckLock(ctx, c.ID); !errors.Is(err, inventory.ErrClusterLocked) {
		t.Fatalf("CheckLock on a freshly imported cluster returned %v, want ErrClusterLocked", err)
	}

	if _, err := f.svc.SetLock(ctx, c.ID, false); err != nil {
		t.Fatalf("SetLock: %v", err)
	}
	if err := f.svc.CheckLock(ctx, c.ID); err != nil {
		t.Fatalf("CheckLock after unlocking: %v", err)
	}

	// A machine in no cluster is not locked: there is no cluster to have
	// adopted it read-only.
	if err := f.svc.CheckLock(ctx, ""); err != nil {
		t.Fatalf("CheckLock on an unassigned machine: %v", err)
	}
}

// TestCertificateLadderNamesTheState pins D-23's rungs, including the one the
// research did not name: an expired certificate is its own state and is not a
// dead cluster.
func TestCertificateLadderNamesTheState(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	c := f.importCluster(ctx, t)

	rows := []struct {
		left    time.Duration
		urgency string
	}{
		{left: 200 * 24 * time.Hour, urgency: inventory.UrgencyNone},
		{left: 60 * 24 * time.Hour, urgency: inventory.UrgencyBadge},
		{left: 20 * 24 * time.Hour, urgency: inventory.UrgencyBanner},
		{left: 3 * 24 * time.Hour, urgency: inventory.UrgencyCritical},
		{left: -time.Hour, urgency: inventory.UrgencyCritical},
	}

	for _, row := range rows {
		rec, err := f.store.Clusters().Get(ctx, c.ID)
		if err != nil {
			t.Fatalf("get cluster: %v", err)
		}
		rec.ClientCertNotAfter = time.Now().Add(row.left)
		if _, err := f.store.Clusters().Put(ctx, rec); err != nil {
			t.Fatalf("put cluster: %v", err)
		}

		v, err := f.svc.Cluster(ctx, c.ID)
		if err != nil {
			t.Fatalf("Cluster: %v", err)
		}
		if v.CertificateUrgency != row.urgency {
			t.Errorf("with %v left the urgency is %q, want %q", row.left, v.CertificateUrgency, row.urgency)
		}
		if row.urgency != inventory.UrgencyNone && v.CertificateWarning == "" {
			t.Errorf("with %v left there is no warning text", row.left)
		}
	}
}

// TestAddManualRecordsANodeByAddress is D-08's second way in: after the
// membership has filled the inventory, an operator can still name an address,
// and that path must work on a cluster that is not locked.
func TestAddManualRecordsANodeByAddress(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	c := f.importCluster(ctx, t)

	rec, err := f.svc.AddManual(ctx, c.ID, f.sim.Host())
	if err != nil {
		t.Fatalf("AddManual: %v", err)
	}
	if rec.Addr != f.sim.Host() {
		t.Errorf("Addr = %q, want %q", rec.Addr, f.sim.Host())
	}
	if rec.Cluster != c.ID {
		t.Errorf("Cluster = %q, want %q", rec.Cluster, c.ID)
	}
}

// TestCreateMintsItsOwnAuthorityAndIsNotLocked is the other half of INV-02 and
// the asymmetry D-21 is about: a created cluster did not exist a second ago, so
// there is nothing yet to protect from a mistake.
func TestCreateMintsItsOwnAuthorityAndIsNotLocked(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})

	c, err := f.svc.Create(ctx, inventory.CreateRequest{
		Name:     "fresh",
		Endpoint: "https://10.0.0.1:6443",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if c.Origin != model.OriginCreated {
		t.Errorf("Origin = %q, want %q", c.Origin, model.OriginCreated)
	}
	if c.Locked {
		t.Error("a created cluster is locked; the lock exists for clusters the operator already depends on")
	}

	sec, err := f.store.ClusterSecrets().Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("the created cluster has no secrets bundle: %v", err)
	}
	if len(sec.OSCAKey) == 0 || len(sec.K8sCAKey) == 0 {
		t.Error("the generated bundle is missing a certificate authority private key")
	}

	// And it is genuinely its own authority, not the simulated cluster's.
	if string(sec.OSCACrt) == string(f.cluster.Secrets.Certs.OS.Crt) {
		t.Fatal("the created cluster reused an existing certificate authority")
	}

	raw, err := f.svc.Talosconfig(ctx, c.ID)
	if err != nil {
		t.Fatalf("Talosconfig: %v", err)
	}
	tc, err := talos.ParseTalosconfig(raw)
	if err != nil {
		t.Fatalf("the rendered talosconfig does not parse: %v", err)
	}
	if tc.NotAfter.IsZero() {
		t.Error("the rendered talosconfig carries no expiry")
	}
}

// TestANodeThatAnswersAndCannotBeReadIsNotTheSameAsAbsent.
//
// The distinction the whole product is built on, arriving at the one record
// that did not make it. A machine whose connection succeeds and whose facts
// read fails used to persist nothing at all: its SeenAt stayed at the last
// time a *complete* observation worked, which is not what the field says it is
// -- "when the machine last answered anything at all" -- so a node that is
// present and unreadable looked exactly like one that is switched off.
//
// They lead to different repairs. The first sends an operator to the node; the
// second to the cable or the power.
func TestANodeThatAnswersAndCannotBeReadIsNotTheSameAsAbsent(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	c := f.importCluster(ctx, t)

	machines, err := f.svc.MachinesOf(ctx, c.ID)
	if err != nil || len(machines) == 0 {
		t.Fatalf("MachinesOf: %v (%d machines)", err, len(machines))
	}
	id := machines[0].ID

	// One complete observation first: adoption writes the record, and SeenAt
	// is the observer's field.
	f.svc.Refresh(ctx, id)

	before, err := f.svc.MachineRecord(ctx, id)
	if err != nil {
		t.Fatalf("MachineRecord: %v", err)
	}
	if before.SeenAt.IsZero() {
		t.Fatal("a machine that was just observed has never been seen")
	}
	if before.Snapshot.TalosVersion == "" {
		t.Fatal("the observation read no version, so there is no snapshot to protect")
	}

	// Take the hardware information away: the node still answers, and the
	// facts read behind the observation now fails. This is the shape of a read
	// that times out against a node that is otherwise up.
	if err := f.sim.SetFactsUnavailable(ctx, true); err != nil {
		t.Fatalf("SetFactsUnavailable: %v", err)
	}

	f.svc.Refresh(ctx, id)

	after, err := f.svc.MachineRecord(ctx, id)
	if err != nil {
		t.Fatalf("MachineRecord: %v", err)
	}

	if !after.SeenAt.After(before.SeenAt) {
		t.Errorf("the machine answered and SeenAt did not move: %s then %s",
			before.SeenAt, after.SeenAt)
	}

	// And the snapshot is untouched. Nothing was read, so there is nothing to
	// write, and re-stamping the old reading with a new time would turn a
	// stale fact into one that looks current -- which is what the Field[T]
	// read model exists against.
	if !after.Snapshot.ObservedAt.Equal(before.Snapshot.ObservedAt) {
		t.Errorf("the snapshot was re-stamped without anything being read: %s then %s",
			before.Snapshot.ObservedAt, after.Snapshot.ObservedAt)
	}
	if after.Snapshot.TalosVersion != before.Snapshot.TalosVersion {
		t.Errorf("the snapshot changed: %q then %q",
			before.Snapshot.TalosVersion, after.Snapshot.TalosVersion)
	}
}
