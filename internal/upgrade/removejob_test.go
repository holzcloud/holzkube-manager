package upgrade_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// Removing a node from its cluster, as a job (UPG-13).
//
// The rig is a cluster of simulated nodes sharing one etcd ring, a fake
// inventory (the list of machines and the forget), and a real job engine over
// the real file store -- so "the process died" is a second engine over the same
// directory, which is what survives a kill -9.

const removeCluster = model.ClusterID("homelab")

type removalRig struct {
	t      *testing.T
	st     store.Store
	engine *jobs.Engine
	deps   upgrade.Deps

	mu       sync.Mutex
	machines []model.Machine
	sims     map[model.MachineID]*talossim.Server

	// all and simByHost never shrink: the inventory forgets a machine, the
	// test still wants to ask what became of it.
	all       map[string]model.Machine
	simByHost map[string]*talossim.Server

	// waits is every duration the settle step asked to wait, and atWait is what
	// the leaving node looked like at that moment.
	waits  []time.Duration
	atWait []talossim.NodeState
	// blockWait makes the fake Wait hang until its context ends, which is how a
	// test holds a job inside the settle step.
	blockWait chan struct{}
	entered   chan struct{}
	once      sync.Once
}

func newRemovalRig(t *testing.T, controlPlanes int, workers int) *removalRig {
	t.Helper()

	cl, err := talossim.NewCluster("homelab", "https://192.168.1.10:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	ring := talossim.NewEtcdRing()

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	st, err := fsstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	r := &removalRig{
		t:         t,
		st:        st,
		sims:      map[model.MachineID]*talossim.Server{},
		all:       map[string]model.Machine{},
		simByHost: map[string]*talossim.Server{},
		entered:   make(chan struct{}),
	}

	add := func(host string, cp bool, id string) {
		opts := talossim.Options{Hostname: host, Cluster: cl, ControlPlane: cp}
		if cp {
			opts.Bootstrapped = true
			opts.EtcdRing = ring
		}
		sim, err := talossim.New(opts)
		if err != nil {
			t.Fatalf("talossim.New(%s): %v", host, err)
		}
		t.Cleanup(func() { _ = sim.Close() })
		role := model.RoleWorker
		if cp {
			role = model.RoleControlPlane
		}
		mid := model.MachineID(id)
		mach := model.Machine{ID: mid, Cluster: removeCluster, Role: role, Hostname: host}
		r.sims[mid] = sim
		r.all[host] = mach
		r.simByHost[host] = sim
		r.machines = append(r.machines, mach)
	}
	for i := 1; i <= controlPlanes; i++ {
		add("cp-"+string(rune('0'+i)), true, "00000000-0000-4000-8000-00000000000"+string(rune('0'+i)))
	}
	for i := 1; i <= workers; i++ {
		add("w-"+string(rune('0'+i)), false, "00000000-0000-4000-8000-0000000000"+string(rune('1'+i))+"0")
	}

	r.deps = upgrade.Deps{
		ResolveInstaller: func(context.Context, string, string, bool) (string, error) { return "", nil },
		Connect: func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
			r.mu.Lock()
			sim, ok := r.sims[id]
			r.mu.Unlock()
			if !ok {
				return nil, errors.New("no such simulated node")
			}
			return talos.NewClusterClient(ctx, sim.Dialer(), talos.Target{
				Cluster: "c1", Machine: id, Addr: sim.Host(),
			}, sim.ClientCreds(), talos.Mode{})
		},
		Machines: func(context.Context, model.ClusterID) ([]model.Machine, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			return append([]model.Machine(nil), r.machines...), nil
		},
		Forget: func(_ context.Context, id model.MachineID) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			for i, m := range r.machines {
				if m.ID == id {
					r.machines = append(r.machines[:i:i], r.machines[i+1:]...)
					break
				}
			}
			return nil
		},
		Wait: func(ctx context.Context, d time.Duration) error {
			r.mu.Lock()
			r.waits = append(r.waits, d)
			// What the node being removed looked like when the wait began:
			// the settle step must come after the leave and before the wipe.
			r.atWait = append(r.atWait, r.simByHost["cp-1"].Node())
			block := r.blockWait
			r.mu.Unlock()
			r.once.Do(func() { close(r.entered) })
			if block != nil {
				select {
				case <-block:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		},
	}

	r.engine = r.newEngine()
	return r
}

func (r *removalRig) newEngine() *jobs.Engine {
	e := jobs.New(jobs.Deps{
		Store:  r.st,
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	r.t.Cleanup(func() { _ = e.Close() })
	upgrade.Register(e, r.deps)
	return e
}

func (r *removalRig) machine(host string) model.Machine {
	r.t.Helper()
	m, ok := r.all[host]
	if !ok {
		r.t.Fatalf("no machine %s", host)
	}
	return m
}

func (r *removalRig) sim(host string) *talossim.Server { return r.simByHost[host] }

func (r *removalRig) waitList() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]time.Duration(nil), r.waits...)
}

func (r *removalRig) atWaitList() []talossim.NodeState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]talossim.NodeState(nil), r.atWait...)
}

func (r *removalRig) submit(e *jobs.Engine, host string) model.Job {
	r.t.Helper()
	m := r.machine(host)
	j, err := e.Submit(r.t.Context(), model.Job{
		Kind: model.JobRemoveFromCluster, Cluster: m.Cluster, Machine: m.ID,
		Params: upgrade.RemovalParams(m),
	})
	if err != nil {
		r.t.Fatalf("Submit: %v", err)
	}
	return j
}

func (r *removalRig) await(e *jobs.Engine, id model.JobID, want model.JobState) model.Job {
	r.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		j, err := e.Get(r.t.Context(), id)
		if err != nil {
			r.t.Fatalf("Get: %v", err)
		}
		if j.State == want {
			return j
		}
		if j.State.Terminal() || j.State == model.JobParked {
			r.t.Fatalf("job is %s (parked: %q), want %s; steps %+v", j.State, j.ParkedReason, want, j.Steps)
		}
		if time.Now().After(deadline) {
			r.t.Fatalf("job still %s after 30s, want %s; steps %+v", j.State, want, j.Steps)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (r *removalRig) known(host string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.machines {
		if m.Hostname == host {
			return true
		}
	}
	return false
}

// leaveCtx is a context a test may call EtcdLeaveCluster with: the client
// refuses a call that carries no deadline.
func leaveCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel, err := talos.WithClassDeadline(t.Context(), talos.MethodEtcdLeaveCluster)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cancel)
	return ctx
}

func stepNames(j model.Job) []string {
	out := make([]string, 0, len(j.Steps))
	for _, s := range j.Steps {
		out = append(out, s.Name)
	}
	return out
}

// TestRemovingAControlPlaneNodeRunsEveryStepInOrder: leave, settle, wipe,
// forget -- and the wait is EvictionWait, taken after the leave and before the
// wipe.
func TestRemovingAControlPlaneNodeRunsEveryStepInOrder(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 3, 0)

	j := r.await(r.engine, r.submit(r.engine, "cp-1").ID, model.JobSucceeded)

	want := []string{"check the node answers", "leave etcd", "let etcd settle", "wipe the node", "forget the node"}
	if got := stepNames(j); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("steps = %v, want %v", got, want)
	}
	for _, s := range j.Steps {
		if s.State != model.StepDone {
			t.Errorf("step %q is %s, want done", s.Name, s.State)
		}
		// What Resume reads after a crash. Only the wipe cannot be asked
		// about: a record that says otherwise would make a restart inside it
		// wipe the node a second time instead of parking.
		if want := s.Name != "wipe the node"; s.Verifiable != want {
			t.Errorf("step %q is recorded Verifiable=%v, want %v", s.Name, s.Verifiable, want)
		}
	}

	if w := r.waitList(); len(w) != 1 || w[0] != upgrade.EvictionWait {
		t.Fatalf("the settle step waited %v, want exactly [EvictionWait = %s]", w, upgrade.EvictionWait)
	}
	if at := r.atWaitList(); len(at) != 1 || at[0].EtcdLeaves != 1 || at[0].Resets != 0 {
		t.Fatalf("at the moment of the wait the node had %+v; it must have left etcd (1 leave) "+
			"and not yet been wiped (0 resets)", at)
	}

	n := r.sim("cp-2").Node()
	if !n.Bootstrapped {
		t.Error("a peer lost its etcd")
	}
	if r.known("cp-1") {
		t.Error("the node is still in the inventory")
	}
}

// TestRemovingAWorkerLeavesNoEtcdStepsAndWaitsForNothing.
func TestRemovingAWorkerLeavesNoEtcdStepsAndWaitsForNothing(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 1, 1)

	j := r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobSucceeded)
	want := []string{"check the node answers", "wipe the node", "forget the node"}
	if got := stepNames(j); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("steps = %v, want %v", got, want)
	}
	if r.sim("cp-1").Node().EtcdLeaves != 0 {
		t.Error("a worker's removal touched etcd")
	}
	if w := r.waitList(); len(w) != 0 {
		t.Errorf("a worker's removal waited %v", w)
	}
}

// TestTheSettleStepReallyWaits uses the real timer.
func TestTheSettleStepReallyWaits(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 3, 0)
	r.deps.Wait = nil
	r.deps.EvictionWait = 300 * time.Millisecond
	e := r.newEngine()

	j := r.await(e, r.submit(e, "cp-1").ID, model.JobSucceeded)

	settle := j.Steps[2]
	if settle.Name != "let etcd settle" {
		t.Fatalf("step 2 is %q", settle.Name)
	}
	if took := settle.FinishedAt.Sub(settle.StartedAt); took < 300*time.Millisecond {
		t.Fatalf("the settle step took %s, want at least the 300ms it was configured with", took)
	}
}

// TestANodeThatAlreadyLeftEtcdSkipsTheLeave: a resubmitted removal, or a
// removal whose leave went through just before a crash, must not tell the node
// to leave again -- and must not be refused by a quorum rule about a member
// that is no longer there.
func TestANodeThatAlreadyLeftEtcdSkipsTheLeave(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 3, 0)

	// cp-1 leaves behind the job's back. Three voters become two: removing
	// "one more" would be refused for a two-member cluster, if the job asked.
	cc, err := r.deps.Connect(t.Context(), r.machine("cp-1").ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := cc.EtcdLeaveCluster(leaveCtx(t)); err != nil {
		t.Fatalf("leave: %v", err)
	}
	_ = cc.Close()

	j := r.await(r.engine, r.submit(r.engine, "cp-1").ID, model.JobSucceeded)

	if got := r.sim("cp-1").Node().EtcdLeaves; got != 1 {
		t.Fatalf("the node left etcd %d times, want once (the one done by hand)", got)
	}
	if d := j.Steps[1].Detail; !strings.Contains(d, "nothing to leave") {
		t.Errorf("the leave step says %q; it should say there was nothing to leave", d)
	}
	if r.sim("cp-1").Node().Resets != 1 {
		t.Error("the node was not wiped")
	}
}

// TestTheLeaveStepRefusesWhenTheClusterCannotSpareTheVoter: the quorum rule
// runs inside the job too, immediately before the leave, because the job
// starts later than the request that checked it.
func TestTheLeaveStepRefusesWhenTheClusterCannotSpareTheVoter(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 2, 0)

	id := r.submit(r.engine, "cp-1").ID
	deadline := time.Now().Add(30 * time.Second)
	var j model.Job
	for {
		j, _ = r.engine.Get(t.Context(), id)
		if j.State == model.JobFailed || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if j.State != model.JobFailed {
		t.Fatalf("job is %s, want failed", j.State)
	}
	if !strings.Contains(j.Steps[1].Detail, "voting members") {
		t.Errorf("the failure %q does not say why", j.Steps[1].Detail)
	}
	if n := r.sim("cp-1").Node(); n.EtcdLeaves != 0 || n.Resets != 0 {
		t.Errorf("the node was touched despite the refusal: %+v", n)
	}
	if !r.known("cp-1") {
		t.Error("the node was forgotten despite the refusal")
	}
}

// TestPreflightRefusesTheOnlyControlPlaneNode is the rule asked through the
// door the handler uses, before anything is submitted. It is the refusal that
// used to be RemoveNode's, and it must come before the leave and before the
// wipe -- a refusal after either is a refusal in name only.
func TestPreflightRefusesTheOnlyControlPlaneNode(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 1, 0)
	svc := upgrade.NewService(r.deps, nil)

	err := svc.PreflightRemoval(t.Context(), r.machine("cp-1"))
	if !errors.Is(err, upgrade.ErrLastVotingMember) {
		t.Fatalf("removing the only control-plane node returned %v, want ErrLastVotingMember", err)
	}
	if !strings.Contains(err.Error(), "only etcd member") {
		t.Errorf("the refusal gives the wrong cluster's advice: %q", err)
	}
	if n := r.sim("cp-1").Node(); !n.Bootstrapped || n.Resets != 0 {
		t.Errorf("the node was touched before the refusal: %+v", n)
	}
}

// TestPreflightAsksAWorkerNothing: a worker is not a member, and a cluster
// whose etcd cannot be reached must not be a cluster whose workers cannot be
// removed.
func TestPreflightAsksAWorkerNothing(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 1, 1)
	svc := upgrade.NewService(r.deps, nil)

	if err := svc.PreflightRemoval(t.Context(), r.machine("w-1")); err != nil {
		t.Fatalf("preflight for a worker: %v", err)
	}
}

// TestARestartInsideTheSettleWaitContinuesWithTheWipe: the process dies while
// the job is waiting. The restarted engine must not leave etcd a second time,
// and must carry on to the wipe and the forget.
func TestARestartInsideTheSettleWaitContinuesWithTheWipe(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 3, 0)
	r.blockWait = make(chan struct{}) // never released: held until the crash

	j := r.submit(r.engine, "cp-1")
	select {
	case <-r.entered:
	case <-time.After(30 * time.Second):
		t.Fatal("the job never reached the settle step")
	}
	if err := r.engine.Close(); err != nil {
		t.Fatal(err)
	}

	stored, err := r.st.Jobs().Get(t.Context(), j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Steps[2].State != model.StepRunning || stored.Steps[1].State != model.StepDone {
		t.Fatalf("the crash left %+v, want leave done and settle running", stored.Steps)
	}
	if r.sim("cp-1").Node().Resets != 0 {
		t.Fatal("the node was wiped before the crash")
	}

	// The restart. The wait is no longer held.
	r.blockWait = nil
	second := r.newEngine()
	if err := second.Resume(t.Context()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	after := r.await(second, j.ID, model.JobSucceeded)

	n := r.sim("cp-1").Node()
	if n.EtcdLeaves != 1 {
		t.Errorf("the node left etcd %d times across the restart, want once", n.EtcdLeaves)
	}
	if n.Resets != 1 {
		t.Errorf("the node was wiped %d times, want once", n.Resets)
	}
	if r.known("cp-1") {
		t.Error("the node is still in the inventory after the resumed job finished")
	}
	if after.Steps[1].State != model.StepDone {
		t.Errorf("the leave step is %s, want done: it finished before the crash", after.Steps[1].State)
	}
}

// TestARestartAfterTheSettleTimeHasPassedDoesNotWaitAgain.
func TestARestartAfterTheSettleTimeHasPassedDoesNotWaitAgain(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 3, 0)
	r.blockWait = make(chan struct{})

	j := r.submit(r.engine, "cp-1")
	<-r.entered
	if err := r.engine.Close(); err != nil {
		t.Fatal(err)
	}

	waitsBefore := len(r.waitList())
	r.blockWait = nil
	// The daemon was down for an hour.
	r.deps.Now = func() time.Time { return time.Now().Add(time.Hour) }
	second := r.newEngine()
	if err := second.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	after := r.await(second, j.ID, model.JobSucceeded)

	if after.Steps[2].State != model.StepSkipped {
		t.Errorf("the settle step is %s, want skipped: the wait had long passed", after.Steps[2].State)
	}
	if w := r.waitList(); len(w) != waitsBefore {
		t.Errorf("the resumed job waited again (%v)", w)
	}
}

// TestARestartInsideTheLeaveWhenTheNodeAlreadyLeftDoesNotLeaveTwice stages the
// record a crash inside the leave RPC leaves: the step is running, the node
// has in fact left.
func TestARestartInsideTheLeaveWhenTheNodeAlreadyLeftDoesNotLeaveTwice(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 3, 0)

	m := r.machine("cp-1")
	cc, err := r.deps.Connect(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := cc.EtcdLeaveCluster(leaveCtx(t)); err != nil {
		t.Fatal(err)
	}
	_ = cc.Close()

	r.stage(t, m, 1, model.StepRunning)

	second := r.newEngine()
	if err := second.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	jobs, _ := second.List(t.Context())
	after := r.await(second, jobs[0].ID, model.JobSucceeded)

	if after.Steps[1].State != model.StepSkipped {
		t.Errorf("the leave step is %s, want skipped: etcd no longer lists the node", after.Steps[1].State)
	}
	if got := r.sim("cp-1").Node().EtcdLeaves; got != 1 {
		t.Errorf("the node left etcd %d times, want once", got)
	}
}

// TestARestartInsideTheLeaveWhenTheNodeHasNotLeftRunsTheLeave: the other
// answer of the same question.
func TestARestartInsideTheLeaveWhenTheNodeHasNotLeftRunsTheLeave(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 3, 0)
	r.stage(t, r.machine("cp-1"), 1, model.StepRunning)

	second := r.newEngine()
	if err := second.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	jobs, _ := second.List(t.Context())
	r.await(second, jobs[0].ID, model.JobSucceeded)

	if got := r.sim("cp-1").Node().EtcdLeaves; got != 1 {
		t.Errorf("the node left etcd %d times, want once", got)
	}
}

// TestARestartInsideTheWipeParksTheJobAndDoesNotWipeAgain: the one step with
// no way to check, as for node.reset.
func TestARestartInsideTheWipeParksTheJobAndDoesNotWipeAgain(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 3, 0)
	r.stage(t, r.machine("cp-1"), 3, model.StepRunning)

	second := r.newEngine()
	if err := second.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	all, _ := second.List(t.Context())
	if all[0].State != model.JobParked {
		t.Fatalf("the job is %s after a restart inside the wipe, want parked", all[0].State)
	}
	if all[0].ParkedReason == "" {
		t.Error("the parked job does not say what a person must decide")
	}
	if got := r.sim("cp-1").Node().Resets; got != 0 {
		t.Errorf("the node was wiped %d times by the restart", got)
	}
	if !r.known("cp-1") {
		t.Error("a parked removal forgot the node")
	}
}

// TestARestartBeforeTheForgetStillForgets: the wipe is done, the record is
// still there.
func TestARestartBeforeTheForgetStillForgets(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 3, 0)
	r.stage(t, r.machine("cp-1"), 4, model.StepPending)

	second := r.newEngine()
	if err := second.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	all, _ := second.List(t.Context())
	r.await(second, all[0].ID, model.JobSucceeded)

	if r.known("cp-1") {
		t.Error("the node is still in the inventory")
	}
	if got := r.sim("cp-1").Node().Resets; got != 0 {
		t.Errorf("a resume at the forget step wiped the node %d times", got)
	}
}

// TestASecondRemovalOnTheSameClusterIsRefusedByTheLease (JOB-03).
func TestASecondRemovalOnTheSameClusterIsRefusedByTheLease(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 3, 1)
	r.blockWait = make(chan struct{})

	first := r.submit(r.engine, "cp-1")
	<-r.entered

	m := r.machine("w-1")
	_, err := r.engine.Submit(t.Context(), model.Job{
		Kind: model.JobRemoveFromCluster, Cluster: m.Cluster, Machine: m.ID,
		Params: upgrade.RemovalParams(m),
	})
	if !errors.Is(err, jobs.ErrClusterBusy) {
		t.Fatalf("a second removal on a busy cluster returned %v, want ErrClusterBusy", err)
	}
	if r.sim("w-1").Node().Resets != 0 {
		t.Error("the refused removal touched its node")
	}

	close(r.blockWait)
	r.await(r.engine, first.ID, model.JobSucceeded)
}

// stage writes the record a process that died at this point would have left:
// every step before `at` done, step `at` in the given state, the rest pending.
func (r *removalRig) stage(t *testing.T, m model.Machine, at int, state model.StepState) {
	t.Helper()

	build := func() model.Job {
		j := model.Job{
			ID: "staged-removal", Kind: model.JobRemoveFromCluster, Cluster: m.Cluster, Machine: m.ID,
			State: model.JobRunning, Current: at, Params: upgrade.RemovalParams(m),
			CreatedAt: time.Now().Add(-time.Minute), StartedAt: time.Now().Add(-time.Minute),
		}
		return j
	}
	j := build()

	// The control-plane step list, as removeNodeSteps builds it, with the
	// Verifiable flag each had when it was submitted: everything can be asked
	// about except the wipe.
	submitted := []model.JobStep{
		{Name: "check the node answers", Verifiable: true},
		{Name: "leave etcd", Verifiable: true},
		{Name: "let etcd settle", Verifiable: true},
		{Name: "wipe the node", Verifiable: false},
		{Name: "forget the node", Verifiable: true},
	}

	for i, s := range submitted {
		s.State = model.StepPending
		if i < at {
			s.State = model.StepDone
		}
		if i == at {
			s.State = state
			if state == model.StepRunning {
				s.StartedAt = time.Now().Add(-10 * time.Second)
			}
		}
		j.Steps = append(j.Steps, s)
	}
	if _, err := r.st.Jobs().Put(t.Context(), j); err != nil {
		t.Fatal(err)
	}
}
