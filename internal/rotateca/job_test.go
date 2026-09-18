package rotateca_test

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/rotateca"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// testCtx carries a deadline for the same reason the fixture does.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// rotationFixture is a cluster this product has adopted, with the rotation
// wired the way the composition root wires it.
type rotationFixture struct {
	sim     *talossim.Server
	store   store.Store
	inv     *inventory.Service
	engine  *jobs.Engine
	cluster model.Cluster
}

func newRotationFixture(t *testing.T) *rotationFixture {
	t.Helper()

	cl, err := talossim.NewCluster("homelab", "https://192.168.1.41:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
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

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	inv := inventory.New(inventory.Deps{
		Store:  st,
		Dialer: talos.NewDirectDialer(sim.Port()),
		Logger: logger,
	})
	t.Cleanup(func() { _ = inv.Close() })

	engine := jobs.New(jobs.Deps{Store: st, Logger: logger})
	t.Cleanup(func() { _ = engine.Close() })

	rotateca.Register(engine, rotateca.Deps{
		Store:    st,
		Logger:   logger,
		Connect:  inv.Connect,
		Machines: inv.MachinesOf,
		AdoptAuthority: func(ctx context.Context, cluster model.ClusterID, a rotateca.Authority) error {
			return inv.AdoptAuthority(ctx, cluster, a.Crt, a.Key)
		},
		Now: time.Now,
	})

	// A deadline, because D-04 refuses a Talos call without one and the HTTP
	// routes are what supply it in production. A fixture on
	// context.Background() would be measuring that refusal -- which is exactly
	// how the heartbeat shipped broken (ledger 137).
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

	return &rotationFixture{sim: sim, store: st, inv: inv, engine: engine, cluster: cluster}
}

// run submits the rotation and waits for the engine to finish with it.
func (f *rotationFixture) run(ctx context.Context, t *testing.T, machines []model.MachineID) model.Job {
	t.Helper()

	params, err := rotateca.Request{Machines: machines}.Params()
	if err != nil {
		t.Fatalf("Params: %v", err)
	}
	j, err := f.engine.Submit(ctx, model.Job{
		Kind: rotateca.JobKindRotateCA, Cluster: f.cluster.ID, Params: params, Actor: "test",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		cur, err := f.engine.Get(ctx, j.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		switch cur.State {
		case model.JobSucceeded, model.JobFailed, model.JobParked, model.JobCancelled:
			return cur
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the rotation never finished")
	return model.Job{}
}

// nodeState is what the simulated node says it trusts, read through the same
// resource every reader in this product reads.
func (f *rotationFixture) nodeState(ctx context.Context, t *testing.T, id model.MachineID) rotateca.State {
	t.Helper()

	cc, err := f.inv.Connect(ctx, id)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer cc.Close() //nolint:errcheck // a close error is not this assertion's business

	cfg, err := cc.MachineConfigYAML(ctx)
	if err != nil {
		t.Fatalf("MachineConfigYAML: %v", err)
	}
	state, err := rotateca.Inspect(cfg)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	return state
}

// TestARotationLeavesTheClusterTrustingOnlyTheNewAuthority is the whole
// operation against the simulator: four passes, and afterwards the node trusts
// the new authority and nothing else, this installation dials with a
// certificate from it, and the record no longer says a rotation is in progress.
//
// It asserts against the node's OWN machine configuration rather than against
// a count of applies, which is what ledger 3 made impossible until this round:
// the simulator now adopts an applied configuration as its active one. Without
// that, every assertion below would have been about an RPC having been issued.
func TestARotationLeavesTheClusterTrustingOnlyTheNewAuthority(t *testing.T) {
	t.Parallel()

	ctx := testCtx(t)
	f := newRotationFixture(t)

	machines, err := f.inv.MachinesOf(ctx, f.cluster.ID)
	if err != nil {
		t.Fatalf("MachinesOf: %v", err)
	}
	if len(machines) != 1 {
		t.Fatalf("the fixture adopted %d machines, want 1", len(machines))
	}
	id := machines[0].ID

	before := f.nodeState(ctx, t, id)
	secBefore, err := f.store.ClusterSecrets().Get(ctx, f.cluster.ID)
	if err != nil {
		t.Fatalf("ClusterSecrets: %v", err)
	}

	job := f.run(ctx, t, []model.MachineID{id})
	if job.State != model.JobSucceeded {
		t.Fatalf("the rotation ended %s: %s", job.State, jobText(job))
	}

	after := f.nodeState(ctx, t, id)
	if after.IssuesFrom(before.IssuingCrt) {
		t.Error("the node still issues from the old authority, so nothing was rotated")
	}
	if after.Accepts(before.IssuingCrt) {
		t.Error("the node still accepts the old authority: pass 4 did not happen, and a rotation " +
			"that leaves the old authority trusted has not changed who can enter the cluster")
	}
	if len(after.AcceptedCrts) != 1 || !after.Accepts(after.IssuingCrt) {
		t.Errorf("the node's accepted set is %d entries; after a finished rotation it is exactly "+
			"the new authority", len(after.AcceptedCrts))
	}

	secAfter, err := f.store.ClusterSecrets().Get(ctx, f.cluster.ID)
	if err != nil {
		t.Fatalf("ClusterSecrets: %v", err)
	}
	if string(secAfter.OSCACrt) == string(secBefore.OSCACrt) {
		t.Error("the stored authority did not move, so this installation is holding the authority " +
			"the cluster no longer uses")
	}
	if string(secAfter.ClientCrt) == string(secBefore.ClientCrt) {
		t.Error("the stored client certificate was not replaced; it was issued by an authority " +
			"no node accepts any more")
	}
	if len(secAfter.NextOSCACrt) != 0 || len(secAfter.NextOSCAKey) != 0 {
		t.Error("the record still says a rotation is in progress, so the next rotation would " +
			"resume this one instead of starting")
	}

	// The product can still reach the node -- with the new credential, because
	// that is the only one the node accepts now. This is the assertion the
	// whole ordering exists for.
	if _, err := f.inv.Connect(ctx, id); err != nil {
		t.Fatalf("after the rotation the node cannot be reached at all: %v", err)
	}
}

// TestARotationRefusesWhileANodeIsSilent is the refusal, and it is checked
// where it decides something: after pass 1, before pass 2.
//
// The node is switched off between the two. What must NOT happen is pass 2 on
// the nodes that did answer -- that is the half-rotated cluster nobody can
// finish. What must happen is a job that stops and says which node was silent.
func TestARotationRefusesWhileANodeIsSilent(t *testing.T) {
	t.Parallel()

	ctx := testCtx(t)
	f := newRotationFixture(t)

	machines, err := f.inv.MachinesOf(ctx, f.cluster.ID)
	if err != nil {
		t.Fatalf("MachinesOf: %v", err)
	}
	id := machines[0].ID

	// A node in the inventory that no longer exists anywhere: the rotation
	// must not walk past it. It is filed by hand rather than by adopting a
	// second simulator, because what is being tested is the refusal and not
	// the discovery.
	if _, err := f.store.Machines().Put(ctx, model.Machine{
		ID: "ghost-node", Cluster: f.cluster.ID, Role: model.RoleWorker, Addr: "203.0.113.7",
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	before := f.nodeState(ctx, t, id)
	job := f.run(ctx, t, []model.MachineID{id, "ghost-node"})

	if job.State == model.JobSucceeded {
		t.Fatal("a rotation succeeded while a node in the cluster never answered")
	}
	if !containsText(job, "ghost-node") {
		t.Errorf("the failure does not name the node that did not answer: %s", jobText(job))
	}

	after := f.nodeState(ctx, t, id)
	if !after.IssuesFrom(before.IssuingCrt) {
		t.Error("the node that DID answer was switched to the new authority anyway, which is the " +
			"half-rotated cluster this refusal exists to prevent")
	}
	if !after.Accepts(before.IssuingCrt) {
		t.Error("the node that answered no longer accepts the old authority")
	}
}

// jobText is everything the job says about itself, which is where a refusal
// has to be readable: the step details, the parked reason, the failure.
func jobText(j model.Job) string {
	parts := []string{string(j.State), j.ParkedReason}
	for _, s := range j.Steps {
		parts = append(parts, s.Name+": "+s.Detail)
	}
	return strings.Join(parts, " | ")
}

func containsText(j model.Job, want string) bool {
	return strings.Contains(jobText(j), want)
}
