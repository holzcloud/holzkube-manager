package provision

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// This file is an internal test because steps is not exported, and the thing to
// measure is where inside one step the hook sits: after the machine is
// confirmed, before it is written to.

const (
	forgetStepApply   = "apply the configuration"
	forgetClusterName = "homelab"
)

// forgetFixture is a blank machine in maintenance mode and the deps of the
// provisioning steps over it.
type forgetFixture struct {
	blank *talossim.Server
	deps  Deps
	req   Request

	mu        sync.Mutex
	forgotten []model.MachineID
	applies   []int // what the simulator had heard of ApplyConfiguration at each forget
}

// storedSecrets is the stored bundle of a simulated cluster (copied from the
// machineconfig tests: it lives in a _test package there).
func storedSecrets(cl *talossim.Cluster) model.ClusterSecrets {
	s := cl.Secrets
	return model.ClusterSecrets{
		OSCACrt:           s.Certs.OS.Crt,
		OSCAKey:           s.Certs.OS.Key,
		K8sCACrt:          s.Certs.K8s.Crt,
		K8sCAKey:          s.Certs.K8s.Key,
		EtcdCACrt:         s.Certs.Etcd.Crt,
		EtcdCAKey:         s.Certs.Etcd.Key,
		AggregatorCACrt:   s.Certs.K8sAggregator.Crt,
		AggregatorCAKey:   s.Certs.K8sAggregator.Key,
		ServiceAccountKey: s.Certs.K8sServiceAccount.Key,
		TalosClusterID:    s.Cluster.ID,
		ClusterSecret:     s.Cluster.Secret,
		BootstrapToken:    s.Secrets.BootstrapToken,
		MachineToken:      s.TrustdInfo.Token,
	}
}

func newForgetFixture(t *testing.T, withHook bool) *forgetFixture {
	t.Helper()

	blank, err := talossim.New(talossim.Options{
		Hostname:    "blank-1",
		Maintenance: true,
		Disks: []talossim.DiskFixture{
			{Device: "nvme0n1", Size: 512 << 30, Model: "SIMULATED NVMe", Serial: "SN-1", Transport: "nvme"},
		},
	})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = blank.Close() })

	cl, err := talossim.NewCluster(forgetClusterName, "https://192.168.1.10:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	secrets := storedSecrets(cl)

	f := &forgetFixture{blank: blank}
	dialer := talos.NewDirectDialer(blank.Port())
	creds := func(string) talos.Creds { return blank.MaintenanceCreds() }

	// The UUID the simulator reports, asked the way the product asks it.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mc, err := talos.NewMaintenanceClient(ctx, dialer,
		talos.Target{Machine: "probe", Addr: blank.Host()}, creds(""), talos.Mode{})
	if err != nil {
		t.Fatalf("NewMaintenanceClient: %v", err)
	}
	facts, err := mc.NodeFacts(ctx)
	_ = mc.Close()
	if err != nil {
		t.Fatalf("NodeFacts: %v", err)
	}
	if facts.UUID == "" {
		t.Fatal("precondition: the simulated machine reports no UUID")
	}

	f.deps = Deps{
		Dialer:           dialer,
		MaintenanceCreds: creds,
		Secrets:          func(context.Context, model.ClusterID) (model.ClusterSecrets, error) { return secrets, nil },
		Cluster: func(context.Context, model.ClusterID) (model.Cluster, error) {
			return model.Cluster{Name: forgetClusterName, Endpoint: "https://192.168.1.10:6443"}, nil
		},
		PatchBodies:       func(context.Context, []string) ([]string, error) { return nil, nil },
		KubernetesVersion: func(context.Context, model.ClusterID) (string, error) { return talossim.DefaultKubernetesVersion, nil },
	}
	if withHook {
		f.deps.ForgetConfig = func(_ context.Context, id model.MachineID) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.forgotten = append(f.forgotten, id)
			f.applies = append(f.applies, blank.Calls("ApplyConfiguration"))
		}
	}
	f.req = Request{
		Addr:         blank.Host(),
		UUID:         facts.UUID,
		Cluster:      "homelab-id",
		ControlPlane: true,
		InstallDisk:  "/dev/nvme0n1",
		TalosVersion: "v1.13.9",
		InstallerImage: "factory.talos.dev/metal-installer/" +
			"376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba:v1.13.9",
	}
	return f
}

// runApplyStep runs the step called "apply the configuration" the way the job
// engine would.
func (f *forgetFixture) runApplyStep(t *testing.T) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var step *jobs.Step
	for _, s := range steps(f.deps, f.req) {
		if s.Name == forgetStepApply {
			step = &s
			break
		}
	}
	if step == nil {
		t.Fatalf("there is no step called %q any more; an older job record would not find it", forgetStepApply)
	}
	job := &model.Job{Steps: make([]model.JobStep, 4)}
	return step.Do(ctx, job)
}

// TestTheApplyStepForgetsTheOldBaselineBeforeItWrites: a machine that is set up
// again under the same UUID has no use for what was known of its old self, and
// a baseline that survived would show the whole new configuration as drift. The
// hook sits after the machine is confirmed and before the write, so that "this
// is this machine, and it is about to change" is known and the write has not
// happened (D-04).
func TestTheApplyStepForgetsTheOldBaselineBeforeItWrites(t *testing.T) {
	t.Parallel()

	f := newForgetFixture(t, true)
	if err := f.runApplyStep(t); err != nil {
		t.Fatalf("the step failed: %v", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.forgotten) != 1 {
		t.Fatalf("ForgetConfig was called %d times, want exactly once", len(f.forgotten))
	}
	if f.forgotten[0] != f.req.UUID {
		t.Errorf("ForgetConfig was called for %q, want the machine %q", f.forgotten[0], f.req.UUID)
	}
	if f.applies[0] != 0 {
		t.Errorf("the machine had heard %d configurations when ForgetConfig was called, want 0: "+
			"the old baseline must go before the write and not after", f.applies[0])
	}
	if got := f.blank.Calls("ApplyConfiguration"); got != 1 {
		t.Errorf("the machine heard %d configurations in the end, want 1: the step has to have written, "+
			"or the test proves nothing about 'before'", got)
	}
}

// TestAWrongMachineForgetsNothing: a check of the machine that fails has not
// found this machine, so it has no business with its baseline. Nothing is
// forgotten and nothing is written.
func TestAWrongMachineForgetsNothing(t *testing.T) {
	t.Parallel()

	f := newForgetFixture(t, true)
	f.req.UUID = "00000000-0000-4000-8000-000000000000"

	err := f.runApplyStep(t)
	if !errors.Is(err, ErrWrongMachine) {
		t.Fatalf("the step returned %v, want ErrWrongMachine", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.forgotten) != 0 {
		t.Errorf("ForgetConfig was called for %v although the machine was not the one asked for", f.forgotten)
	}
	if got := f.blank.Calls("ApplyConfiguration"); got != 0 {
		t.Errorf("the machine heard %d configurations, want 0: nothing is written to a machine that is not the one", got)
	}
}

// TestWithoutAForgetHookTheApplyStepIsUnchanged: the hook is optional; a
// daemon without the drift watch provisions as it always did.
func TestWithoutAForgetHookTheApplyStepIsUnchanged(t *testing.T) {
	t.Parallel()

	f := newForgetFixture(t, false)
	if f.deps.ForgetConfig != nil {
		t.Fatal("precondition: the fixture has a hook")
	}
	if err := f.runApplyStep(t); err != nil {
		t.Fatalf("the step failed without a hook: %v", err)
	}
	if got := f.blank.Calls("ApplyConfiguration"); got != 1 {
		t.Errorf("the machine heard %d configurations, want 1", got)
	}
}

// TestAFailedWriteStillForgetsTheOldBaseline: the old state of a machine that
// is being set up again is void from the moment the check of it passed, whether
// or not the write then goes through; a retry of the step forgets once more and
// costs nothing.
func TestAFailedWriteStillForgetsTheOldBaseline(t *testing.T) {
	t.Parallel()

	f := newForgetFixture(t, true)
	restore, err := f.blank.Inject(talossim.Scenario{Name: talossim.ScenarioRejectApply})
	if err != nil {
		t.Fatalf("Inject: %v", err)
	}
	defer restore()

	if err := f.runApplyStep(t); err == nil {
		t.Fatal("the step succeeded although the machine rejects the configuration; the scenario did not take")
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.forgotten) != 1 || f.forgotten[0] != f.req.UUID {
		t.Errorf("ForgetConfig was called for %v, want once for %q: a failed write changes nothing about the old baseline being void",
			f.forgotten, f.req.UUID)
	}
}
