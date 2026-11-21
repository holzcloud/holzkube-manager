package upgrade_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
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

	// ksim is the cluster's Kubernetes API, and hostOf names the host behind
	// each machine id so that a Connect can be recorded against its target.
	ksim   *kubesim.Server
	hostOf map[model.MachineID]string

	// snaps is one rigSnapshot per Connect, in order, and forgot one per call
	// of the inventory's Forget.
	snaps  []rigSnapshot
	forgot []rigSnapshot

	// waits is every duration the settle step asked to wait, and atWait is what
	// the leaving node and the cluster looked like at that moment.
	waits  []time.Duration
	atWait []rigSnapshot
	// blockWait makes the fake Wait hang until its context ends, which is how a
	// test holds a job inside the settle step.
	blockWait chan struct{}
	entered   chan struct{}
	once      sync.Once
}

// removalRigOptions are the choices a test makes about the Kubernetes side of
// a rig and the drain's pace. The zero value is the rig every older test uses:
// one node per host, no pods, a drain that retries every 10 ms for up to 5 s.
type removalRigOptions struct {
	// Pods are the cluster's pods, in kubesim's words.
	Pods []kubesim.Pod

	// Nodes override the node of the same name; a name no host has is added.
	Nodes []kubesim.Node

	// Sim edits kubesim's options last, so a test can take the default nodes
	// away ("Kubernetes has no node for this machine") or add a denial.
	Sim func(*kubesim.Options)

	// Kube replaces Deps.Kube. It receives the rig so that it can build a
	// client from the rig's own cluster, or ignore it and fail.
	Kube func(r *removalRig) func(ctx context.Context, cluster model.ClusterID) (*kube.Client, error)

	// DrainBudget and DrainPace are the drain's deadline and its pause between
	// attempts. Zero is 5 s and 10 ms here; production runs with the
	// constants of internal/kube.
	DrainBudget, DrainPace time.Duration

	// KubeDownAfterWipe makes Deps.Kube answer with a connection error as soon
	// as any simulated node has been wiped. It is the cluster whose Kubernetes
	// endpoint is the address of the node being removed: the API is there
	// until the node is wiped and not after.
	KubeDownAfterWipe bool

	// Inventory is what the inventory says about a machine at the moment it is
	// asked. A refresh after the wipe can give a wiped machine, which boots
	// into maintenance mode, another hostname and another address: this is how
	// a test says so. Nil is the machine as it was created. The function is
	// called under the rig's lock and must not take it.
	Inventory func(r *removalRig, m model.Machine) model.Machine
}

// rigSnapshot is what the rig can see at one moment: the node a Connect
// targets, and the cluster around it.
type rigSnapshot struct {
	Node talossim.NodeState

	// Evicted is what kubesim has accepted evictions for. PodsOnNode is the
	// pods kubesim still lists on that node, as "namespace/name".
	// DeletedNodes is the Node objects kubesim no longer has.
	Evicted, PodsOnNode, DeletedNodes []string
}

func newRemovalRig(t *testing.T, controlPlanes int, workers int) *removalRig {
	t.Helper()
	return newRemovalRigWith(t, controlPlanes, workers, removalRigOptions{})
}

func newRemovalRigWith(t *testing.T, controlPlanes int, workers int, opts removalRigOptions) *removalRig {
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
		hostOf:    map[model.MachineID]string{},
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
		r.hostOf[mid] = host
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

	// The cluster's Kubernetes API: one node per host, then what the test says.
	kopts := kubesim.Options{Pods: opts.Pods}
	for _, m := range r.machines {
		kopts.Nodes = append(kopts.Nodes, kubesim.Node{Name: m.Hostname})
	}
	for _, n := range opts.Nodes {
		i := slices.IndexFunc(kopts.Nodes, func(have kubesim.Node) bool { return have.Name == n.Name })
		if i >= 0 {
			kopts.Nodes[i] = n
		} else {
			kopts.Nodes = append(kopts.Nodes, n)
		}
	}
	if opts.Sim != nil {
		opts.Sim(&kopts)
	}
	ksim, err := kubesim.New(kopts)
	if err != nil {
		t.Fatalf("kubesim.New: %v", err)
	}
	t.Cleanup(ksim.Close)
	r.ksim = ksim

	drainBudget, drainPace := opts.DrainBudget, opts.DrainPace
	if drainBudget == 0 {
		drainBudget = 5 * time.Second
	}
	if drainPace == 0 {
		drainPace = 10 * time.Millisecond
	}

	r.deps = upgrade.Deps{
		DrainBudget:      drainBudget,
		DrainPace:        drainPace,
		Kube:             r.newKubeClient,
		ResolveInstaller: func(context.Context, string, string, bool) (string, error) { return "", nil },
		Connect: func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
			r.mu.Lock()
			sim, ok := r.sims[id]
			r.mu.Unlock()
			if !ok {
				return nil, errors.New("no such simulated node")
			}
			r.mu.Lock()
			r.snaps = append(r.snaps, r.snapshotOf(r.hostOf[id]))
			r.mu.Unlock()
			return talos.NewClusterClient(ctx, sim.Dialer(), talos.Target{
				Cluster: "c1", Machine: id, Addr: sim.Host(),
			}, sim.ClientCreds(), talos.Mode{})
		},
		Machines: func(context.Context, model.ClusterID) ([]model.Machine, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			out := append([]model.Machine(nil), r.machines...)
			if opts.Inventory != nil {
				for i := range out {
					out[i] = opts.Inventory(r, out[i])
				}
			}
			return out, nil
		},
		Forget: func(_ context.Context, id model.MachineID) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.forgot = append(r.forgot, r.snapshotOf(r.hostOf[id]))
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
			r.atWait = append(r.atWait, r.snapshotOf("cp-1"))
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

	if opts.Kube != nil {
		r.deps.Kube = opts.Kube(r)
	}
	if opts.KubeDownAfterWipe {
		up := r.deps.Kube
		r.deps.Kube = func(ctx context.Context, cluster model.ClusterID) (*kube.Client, error) {
			r.mu.Lock()
			wiped := false
			for _, sim := range r.sims {
				if sim.Node().Resets > 0 {
					wiped = true
				}
			}
			r.mu.Unlock()
			if wiped {
				return nil, errors.New("dial tcp 192.168.1.10:6443: connect: connection refused")
			}
			return up(ctx, cluster)
		}
	}

	r.engine = r.newEngine()
	return r
}

// newKubeClient is how the daemon reaches the cluster: a certificate minted
// from the cluster's own Kubernetes authority, fresh every time.
func (r *removalRig) newKubeClient(context.Context, model.ClusterID) (*kube.Client, error) {
	caCrt, caKey := r.ksim.AuthorityPEM()
	creds, err := kube.MintCreds(r.ksim.Endpoint(), caCrt, caKey, time.Now())
	if err != nil {
		return nil, err
	}
	return kube.New(creds)
}

// kubeClient is a client for a test's own questions to the cluster.
func (r *removalRig) kubeClient(t *testing.T) *kube.Client {
	t.Helper()
	c, err := r.newKubeClient(t.Context(), removeCluster)
	if err != nil {
		t.Fatalf("a client for the simulated cluster: %v", err)
	}
	return c
}

// snapshotOf reads what the rig can see about one host right now.
func (r *removalRig) snapshotOf(host string) rigSnapshot {
	return rigSnapshot{
		Node:         r.simByHost[host].Node(),
		Evicted:      r.ksim.Evicted(),
		PodsOnNode:   r.ksim.PodsOn(host),
		DeletedNodes: r.ksim.DeletedNodes(),
	}
}

// forgets is one rigSnapshot per call of the inventory's Forget, taken at that
// moment, for the host being forgotten.
func (r *removalRig) forgets() []rigSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.forgot)
}

// connects is one rigSnapshot per Connect, read from the Connect's target at
// the moment it was opened. The last one of a removal that ran to the end is
// the wipe's: it shows what the cluster looked like when the node was reset.
func (r *removalRig) connects() []rigSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.snaps)
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

func (r *removalRig) atWaitList() []rigSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.atWait)
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

// removalStepNames is the step list a removal of this role is submitted with.
// A test asks for steps by these names, never by position: a step inserted
// later moves every index and no name.
func removalStepNames(role model.MachineRole) []string {
	if role == model.RoleControlPlane {
		return []string{
			"check the node answers", "cordon the node", "drain the node",
			"leave etcd", "let etcd settle", "wipe the node",
			"delete the Kubernetes node object", "forget the node",
		}
	}
	return []string{
		"check the node answers", "cordon the node", "drain the node",
		"wipe the node", "delete the Kubernetes node object", "forget the node",
	}
}

// stepNamed is the job's step of that name. A name the job does not have is
// the zero step, whose state and detail match no expectation.
func stepNamed(j model.Job, name string) model.JobStep {
	for _, s := range j.Steps {
		if s.Name == name {
			return s
		}
	}
	return model.JobStep{}
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

	want := removalStepNames(model.RoleControlPlane)
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
	if at := r.atWaitList(); len(at) != 1 || at[0].Node.EtcdLeaves != 1 || at[0].Node.Resets != 0 {
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
	want := removalStepNames(model.RoleWorker)
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

	settle := stepNamed(j, "let etcd settle")
	if settle.Name == "" {
		t.Fatalf("the job has no settle step: %v", stepNames(j))
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
	if d := stepNamed(j, "leave etcd").Detail; !strings.Contains(d, "nothing to leave") {
		t.Errorf("the leave step says %q; it should say there was nothing to leave", d)
	}
	if r.sim("cp-1").Node().Resets != 1 {
		t.Error("the node was not wiped")
	}
}

// TestTheLeaveStepRefusesWhenTheClusterCannotSpareTheVoter: the quorum rule
// runs inside the job too, immediately before the leave, because the job
// starts later than the request that checked it. The cordon step asks it
// first now (see removejob_kube_test.go), so a refusal at the leave is
// staged: the job resumes at that step, as it does when the cluster lost a
// voter while the drain was moving pods.
func TestTheLeaveStepRefusesWhenTheClusterCannotSpareTheVoter(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 2, 0)
	r.stage(t, r.machine("cp-1"), "leave etcd", model.StepPending)

	second := r.newEngine()
	if err := second.Resume(t.Context()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	all, _ := second.List(t.Context())
	id := all[0].ID
	deadline := time.Now().Add(30 * time.Second)
	var j model.Job
	for {
		j, _ = second.Get(t.Context(), id)
		if j.State == model.JobFailed || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if j.State != model.JobFailed {
		t.Fatalf("job is %s, want failed", j.State)
	}
	if d := stepNamed(j, "leave etcd").Detail; !strings.Contains(d, "voting members") {
		t.Errorf("the failure %q does not say why", d)
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
	if stepNamed(stored, "let etcd settle").State != model.StepRunning ||
		stepNamed(stored, "leave etcd").State != model.StepDone {
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
	if st := stepNamed(after, "leave etcd").State; st != model.StepDone {
		t.Errorf("the leave step is %s, want done: it finished before the crash", st)
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

	if st := stepNamed(after, "let etcd settle").State; st != model.StepSkipped {
		t.Errorf("the settle step is %s, want skipped: the wait had long passed", st)
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

	r.stage(t, m, "leave etcd", model.StepRunning)

	second := r.newEngine()
	if err := second.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	jobs, _ := second.List(t.Context())
	after := r.await(second, jobs[0].ID, model.JobSucceeded)

	if st := stepNamed(after, "leave etcd").State; st != model.StepSkipped {
		t.Errorf("the leave step is %s, want skipped: etcd no longer lists the node", st)
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
	r.stage(t, r.machine("cp-1"), "leave etcd", model.StepRunning)

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
	r.stage(t, r.machine("cp-1"), "wipe the node", model.StepRunning)

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
	r.stage(t, r.machine("cp-1"), "forget the node", model.StepPending)

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

// stage writes the record a process that died at this step would have left:
// every step before it done, the named step in the given state, the rest
// pending. The step list is the one a removal of the machine's role is
// submitted with, and the step is found by name.
func (r *removalRig) stage(t *testing.T, m model.Machine, step string, state model.StepState) {
	t.Helper()

	names := removalStepNames(m.Role)
	at := slices.Index(names, step)
	if at < 0 {
		t.Fatalf("a %s's removal has no step %q (it has %v)", m.Role, step, names)
	}

	j := model.Job{
		ID: "staged-removal", Kind: model.JobRemoveFromCluster, Cluster: m.Cluster, Machine: m.ID,
		State: model.JobRunning, Current: at, Params: upgrade.RemovalParams(m),
		CreatedAt: time.Now().Add(-time.Minute), StartedAt: time.Now().Add(-time.Minute),
	}

	for i, name := range names {
		// What each step could be asked about when it was submitted:
		// everything except the wipe.
		s := model.JobStep{Name: name, Verifiable: name != "wipe the node", State: model.StepPending}
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

// replicaPod is a pod a ReplicaSet owns: the ordinary kind a drain moves.
func replicaPod(name, node string) kubesim.Pod {
	return kubesim.Pod{Namespace: "default", Name: name, Node: node, OwnerKind: "ReplicaSet"}
}

// podNames is the pods kubesim still lists on a node, through the client the
// daemon uses.
func podNames(t *testing.T, c *kube.Client, node string) []string {
	t.Helper()
	pods, err := c.PodsOnNode(t.Context(), node)
	if err != nil {
		t.Fatalf("PodsOnNode(%s): %v", node, err)
	}
	out := make([]string, 0, len(pods))
	for _, p := range pods {
		out = append(out, p.Name())
	}
	slices.Sort(out)
	return out
}

// TestRemovingAWorkerCordonsAndDrainsItBeforeTheWipe is D-01 for a worker: by
// the time the wipe opens its connection, the node takes no new work and every
// pod that can be evicted is gone through the eviction API. A DaemonSet's pod
// stays, and nothing on another node is touched.
func TestRemovingAWorkerCordonsAndDrainsItBeforeTheWipe(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{Pods: []kubesim.Pod{
		replicaPod("web-1", "w-1"),
		replicaPod("web-2", "w-1"),
		{Namespace: "kube-system", Name: "agent", Node: "w-1", OwnerKind: "DaemonSet"},
		replicaPod("elsewhere", "cp-1"),
	}})

	j := r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobSucceeded)

	want := removalStepNames(model.RoleWorker)
	if got := stepNames(j); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("steps = %v, want %v", got, want)
	}
	for _, s := range j.Steps {
		if s.State != model.StepDone {
			t.Errorf("step %q is %s, want done", s.Name, s.State)
		}
		if want := s.Name != "wipe the node"; s.Verifiable != want {
			t.Errorf("step %q is recorded Verifiable=%v, want %v", s.Name, s.Verifiable, want)
		}
	}

	evicted := r.ksim.Evicted()
	slices.Sort(evicted)
	if want := []string{"default/web-1", "default/web-2"}; !slices.Equal(evicted, want) {
		t.Fatalf("evicted %v, want exactly the worker's two ReplicaSet pods %v", evicted, want)
	}

	c := r.kubeClient(t)
	if got := podNames(t, c, "w-1"); !slices.Equal(got, []string{"kube-system/agent"}) {
		t.Errorf("pods left on the worker: %v, want only the DaemonSet's", got)
	}
	if got := podNames(t, c, "cp-1"); !slices.Equal(got, []string{"default/elsewhere"}) {
		t.Errorf("pods on the control plane: %v, a worker's drain touched them", got)
	}
	nodes, err := c.Nodes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range nodes {
		if want := n.Name == "w-1"; n.Unschedulable != want {
			t.Errorf("node %s unschedulable=%v, want %v", n.Name, n.Unschedulable, want)
		}
	}

	// The moment that matters: the wipe's own connection. Everything above is
	// also true of a drain that ran after the wipe.
	conns := r.connects()
	if len(conns) == 0 {
		t.Fatal("no connection was recorded")
	}
	atWipe := conns[len(conns)-1]
	if len(atWipe.Evicted) != 2 || atWipe.Node.Resets != 0 {
		t.Errorf("when the wipe connected, evicted=%v and resets=%d; the pods must already be gone "+
			"and the node not yet wiped", atWipe.Evicted, atWipe.Node.Resets)
	}
}

// TestRemovingAControlPlaneNodeDrainsItBeforeItLeavesEtcd: leaving the
// membership is the first step that cannot be taken back, so the pods have to
// be gone before it. The settle wait comes after the leave, which makes it the
// vantage point from which the order can be read.
func TestRemovingAControlPlaneNodeDrainsItBeforeItLeavesEtcd(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 3, 0, removalRigOptions{Pods: []kubesim.Pod{replicaPod("web-1", "cp-1")}})

	j := r.await(r.engine, r.submit(r.engine, "cp-1").ID, model.JobSucceeded)

	want := removalStepNames(model.RoleControlPlane)
	if got := stepNames(j); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("steps = %v, want %v", got, want)
	}
	for _, s := range j.Steps {
		if wantV := s.Name != "wipe the node"; s.Verifiable != wantV {
			t.Errorf("step %q is recorded Verifiable=%v, want %v: a restart inside it would "+
				"otherwise park or repeat", s.Name, s.Verifiable, wantV)
		}
	}

	at := r.atWaitList()
	if len(at) != 1 {
		t.Fatalf("the settle step waited %d times, want once", len(at))
	}
	if at[0].Node.EtcdLeaves != 1 || at[0].Node.Resets != 0 {
		t.Errorf("at the settle wait the node had %+v; it must have left etcd and not been wiped",
			at[0].Node)
	}
	if !slices.Equal(at[0].Evicted, []string{"default/web-1"}) {
		t.Errorf("at the settle wait the cluster had evicted %v; the pod must be gone by then "+
			"(the drain comes before the leave)", at[0].Evicted)
	}
}

// TestRemovingANodeOfAClusterWithoutKubernetesAuthoritySaysItDidNotDrain: a
// cluster whose stored bundle has no Kubernetes authority cannot be reached at
// all, and that is a fact about the configuration. It is a visible skip, not a
// silent one: the screen says nothing was drained.
func TestRemovingANodeOfAClusterWithoutKubernetesAuthoritySaysItDidNotDrain(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{
		Pods: []kubesim.Pod{replicaPod("web-1", "w-1")},
		Kube: func(*removalRig) func(context.Context, model.ClusterID) (*kube.Client, error) {
			return func(context.Context, model.ClusterID) (*kube.Client, error) {
				return nil, fmt.Errorf("inventory: cluster homelab: %w", kube.ErrNoKubernetesAuthority)
			}
		},
	})

	j := r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobSucceeded)

	for _, name := range []string{"cordon the node", "drain the node"} {
		d := stepNamed(j, name).Detail
		if !strings.Contains(d, "no Kubernetes access") || !strings.Contains(d, "not drained") {
			t.Errorf("step %q says %q; it must say there is no Kubernetes access and that the "+
				"node was not drained", name, d)
		}
	}
	if got := r.ksim.Evicted(); len(got) != 0 {
		t.Errorf("evicted %v with no Kubernetes access", got)
	}
	if r.sim("w-1").Node().Resets != 1 {
		t.Error("the removal did not go on to wipe the node")
	}
}

// TestRemovingANodeWhenKubernetesDoesNotAnswerChangesNothing is D-03 Q7: a
// Kubernetes that does not answer is not a cluster without Kubernetes. Treating
// it as a skip would remove the safety net exactly when an API server is
// down, so the job refuses before it has changed anything.
func TestRemovingANodeWhenKubernetesDoesNotAnswerChangesNothing(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 3, 1, removalRigOptions{
		Pods: []kubesim.Pod{replicaPod("web-1", "w-1")},
		Kube: func(*removalRig) func(context.Context, model.ClusterID) (*kube.Client, error) {
			return func(context.Context, model.ClusterID) (*kube.Client, error) {
				return nil, errors.New("dial tcp 192.168.1.10:6443: connect: connection refused")
			}
		},
	})

	for _, host := range []string{"w-1", "cp-1"} {
		e := r.newEngine()
		j := r.await(e, r.submit(e, host).ID, model.JobFailed)
		if cur := j.Steps[j.Current].Name; cur != "cordon the node" {
			t.Fatalf("%s: the job failed in %q, want the first Kubernetes step", host, cur)
		}
		if d := stepNamed(j, "cordon the node").Detail; !strings.Contains(d, "Nothing has been changed") {
			t.Errorf("%s: the failure %q does not say nothing was changed", host, d)
		}
		if n := r.sim(host).Node(); n.Resets != 0 || n.EtcdLeaves != 0 {
			t.Errorf("%s was touched although Kubernetes did not answer: %+v", host, n)
		}
		if !r.known(host) {
			t.Errorf("%s was forgotten", host)
		}
		// Only one removal may run on the cluster at a time, and this one has
		// ended, so the next host can be tried.
	}
	if got := r.ksim.Evicted(); len(got) != 0 {
		t.Errorf("evicted %v while Kubernetes did not answer", got)
	}
}

// TestRemovingAMachineKubernetesHasNoNodeForSaysSo: a machine that never
// joined, or one the API server has forgotten, has nothing to drain. That is
// said, and the removal goes on.
func TestRemovingAMachineKubernetesHasNoNodeForSaysSo(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{
		Sim: func(o *kubesim.Options) {
			o.Nodes = slices.DeleteFunc(o.Nodes, func(n kubesim.Node) bool { return n.Name == "w-1" })
		},
	})

	j := r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobSucceeded)

	for _, name := range []string{"cordon the node", "drain the node", "delete the Kubernetes node object"} {
		if d := stepNamed(j, name).Detail; !strings.Contains(d, "no node") {
			t.Errorf("step %q says %q; it must say Kubernetes knows no node for this machine", name, d)
		}
	}
	if d := stepNamed(j, "delete the Kubernetes node object").Detail; !strings.Contains(d, "nothing to delete") {
		t.Errorf("the deletion step says %q; it must say there was nothing to delete", d)
	}
	if got := r.ksim.DeletedNodes(); len(got) != 0 {
		t.Errorf("deleted nodes %v for a machine Kubernetes has no node for", got)
	}
	if r.sim("w-1").Node().Resets != 1 {
		t.Error("the removal did not go on to wipe the node")
	}
}

// TestRemovingWithoutAKubernetesClientWiredIsRefusedBeforeAnythingChanges: a
// build that was wired without Kubernetes must refuse, not skip. A skip is a
// claim that there was nothing to drain; this build cannot know.
func TestRemovingWithoutAKubernetesClientWiredIsRefusedBeforeAnythingChanges(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 1, 1)
	r.deps.Kube = nil
	e := r.newEngine()

	j := r.await(e, r.submit(e, "w-1").ID, model.JobFailed)

	if cur := j.Steps[j.Current].Name; cur != "cordon the node" {
		t.Fatalf("the job failed in %q, want cordon the node", cur)
	}
	if d := stepNamed(j, "cordon the node").Detail; !strings.Contains(d, "without a Kubernetes client") {
		t.Errorf("the failure %q does not say this build has no Kubernetes client", d)
	}
	if r.sim("w-1").Node().Resets != 0 || !r.known("w-1") {
		t.Error("the node was touched although the build could not drain it")
	}
}

// TestARestartInsideTheDrainKeepsDrainingAndDoesNotPark: the drain is the one
// new step that moves things, and it can be asked "is anything left that you
// would move", so a restart inside it carries on rather than parking for a
// person. A restart inside the wipe still parks (above).
func TestARestartInsideTheDrainKeepsDrainingAndDoesNotPark(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{Pods: []kubesim.Pod{replicaPod("web-1", "w-1")}})
	r.stage(t, r.machine("w-1"), "drain the node", model.StepRunning)

	second := r.newEngine()
	if err := second.Resume(t.Context()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	all, _ := second.List(t.Context())
	after := r.await(second, all[0].ID, model.JobSucceeded)

	if !slices.Equal(r.ksim.Evicted(), []string{"default/web-1"}) {
		t.Errorf("the resumed drain evicted %v, want the pod that was still there", r.ksim.Evicted())
	}
	if st := stepNamed(after, "drain the node").State; st != model.StepDone {
		t.Errorf("the drain step is %s, want done: pods were left to move", st)
	}
	if r.sim("w-1").Node().Resets != 1 {
		t.Error("the resumed removal did not wipe the node")
	}
}

// TestARestartInsideTheDrainWhenNothingIsLeftSkipsIt: the other answer of the
// same question.
func TestARestartInsideTheDrainWhenNothingIsLeftSkipsIt(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 1, 1)
	r.stage(t, r.machine("w-1"), "drain the node", model.StepRunning)

	second := r.newEngine()
	if err := second.Resume(t.Context()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	all, _ := second.List(t.Context())
	after := r.await(second, all[0].ID, model.JobSucceeded)

	if st := stepNamed(after, "drain the node").State; st != model.StepSkipped {
		t.Errorf("the drain step is %s, want skipped: nothing was left to move", st)
	}
}

// budgetedPod is a StatefulSet's pod a PodDisruptionBudget named db-pdb holds.
func budgetedPod(node string) kubesim.Pod {
	return kubesim.Pod{
		Namespace: "default", Name: "db-0", Node: node, OwnerKind: "StatefulSet",
		BudgetRefuses: true, BudgetName: "db-pdb",
	}
}

// TestABudgetThatNeverOpensStopsTheRemovalBeforeAnythingIsIrreversible is the
// reason the drain is a step of its own and comes first: a pod a budget
// protects is a veto, and a removal that honours it ends with the node still
// in etcd and still wiped of nothing. Talos' own drain inside the reset gives
// up after five minutes and goes on; this one does not.
func TestABudgetThatNeverOpensStopsTheRemovalBeforeAnythingIsIrreversible(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		host string
		cps  int
		wks  int
	}{
		{"w-1", 1, 1},
		{"cp-1", 3, 0},
	} {
		t.Run(tc.host, func(t *testing.T) {
			t.Parallel()
			r := newRemovalRigWith(t, tc.cps, tc.wks, removalRigOptions{
				Pods:        []kubesim.Pod{budgetedPod(tc.host)},
				DrainBudget: 400 * time.Millisecond,
			})

			j := r.await(r.engine, r.submit(r.engine, tc.host).ID, model.JobFailed)

			if cur := j.Steps[j.Current].Name; cur != "drain the node" {
				t.Fatalf("the job failed in %q, want the drain", cur)
			}
			d := stepNamed(j, "drain the node").Detail
			for _, want := range []string{"db-pdb", "default/db-0", "Nothing has been wiped", "has not left etcd"} {
				if !strings.Contains(d, want) {
					t.Errorf("the failure %q does not say %q", d, want)
				}
			}
			if n := r.sim(tc.host).Node(); n.Resets != 0 || n.EtcdLeaves != 0 {
				t.Errorf("%s was touched although a budget held a pod: %+v", tc.host, n)
			}
			if !r.known(tc.host) {
				t.Errorf("%s was forgotten", tc.host)
			}
			if got := r.ksim.Evicted(); len(got) != 0 {
				t.Errorf("evicted %v despite the budget", got)
			}
			if got := r.ksim.Deleted(); len(got) != 0 {
				t.Errorf("deleted %v: a refused eviction is never turned into a delete", got)
			}
			if got := r.ksim.Calls("POST /api/v1/namespaces/default/pods/db-0/eviction"); got < 2 {
				t.Errorf("the eviction was attempted %d times, want it asked again until the deadline", got)
			}
		})
	}
}

// TestABudgetThatOpensLetsTheRemovalFinish: the same cluster, the budget opens
// while the drain waits, and the removal runs to the end.
func TestABudgetThatOpensLetsTheRemovalFinish(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{
		Pods:        []kubesim.Pod{budgetedPod("w-1")},
		DrainBudget: 10 * time.Second,
	})

	go func() {
		for r.ksim.Calls("POST /api/v1/namespaces/default/pods/db-0/eviction") < 2 && r.t.Context().Err() == nil {
			time.Sleep(2 * time.Millisecond)
		}
		_ = r.ksim.SetBudgetRefuses("default", "db-0", false)
	}()

	r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobSucceeded)

	if !slices.Equal(r.ksim.Evicted(), []string{"default/db-0"}) {
		t.Errorf("evicted %v, want the pod once the budget opened", r.ksim.Evicted())
	}
	if r.sim("w-1").Node().Resets != 1 {
		t.Error("the removal did not go on to wipe the node")
	}
}

// TestARemovalWaitsForTerminatingPodsBeforeTheWipe: an eviction the server
// accepted is not a pod that is gone. The wipe must connect after the pods have
// left the node.
func TestARemovalWaitsForTerminatingPodsBeforeTheWipe(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{Pods: []kubesim.Pod{
		{Namespace: "default", Name: "web-1", Node: "w-1", OwnerKind: "ReplicaSet", LingersAfterEviction: true},
		{Namespace: "default", Name: "web-2", Node: "w-1", OwnerKind: "ReplicaSet", LingersAfterEviction: true},
	}})

	// The pods finish shutting down 300 ms after their evictions were accepted,
	// not after the test started: the steps before the drain take about as long
	// as a fixed delay would, and a wipe that connected after a delay that was
	// already over would prove nothing.
	go func() {
		for len(r.ksim.Evicted()) < 2 && r.t.Context().Err() == nil {
			time.Sleep(2 * time.Millisecond)
		}
		time.Sleep(300 * time.Millisecond)
		for _, name := range []string{"web-1", "web-2"} {
			for r.t.Context().Err() == nil {
				if r.ksim.FinishTermination("default", name) == nil {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()

	r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobSucceeded)

	conns := r.connects()
	atWipe := conns[len(conns)-1]
	if len(atWipe.PodsOnNode) != 0 {
		t.Errorf("when the wipe connected the node still listed %v; the drain must wait for the "+
			"pods to be gone", atWipe.PodsOnNode)
	}
	if len(atWipe.Evicted) != 2 {
		t.Errorf("when the wipe connected, evicted=%v, want both pods", atWipe.Evicted)
	}
}

// TestTheKubernetesNodeObjectIsDeletedAfterTheWipeAndBeforeTheForget is D-03
// Q4: once the machine is wiped its Node object is a promise nobody keeps, and
// it is deleted -- after the wipe, because a kubelet that is still running
// could put its object back, and before the forget, because forgetting is what
// makes the removal look finished.
func TestTheKubernetesNodeObjectIsDeletedAfterTheWipeAndBeforeTheForget(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{Pods: []kubesim.Pod{
		replicaPod("web-1", "w-1"), replicaPod("web-2", "w-1"),
	}})

	j := r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobSucceeded)

	// Errorf and not Fatalf: a step list in the wrong order is one failure,
	// what the cluster looked like at the wipe is another, and the second is
	// the one that says why the order matters.
	want := removalStepNames(model.RoleWorker)
	if got := stepNames(j); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("steps = %v, want %v", got, want)
	}
	for _, s := range j.Steps {
		if wantV := s.Name != "wipe the node"; s.Verifiable != wantV {
			t.Errorf("step %q is recorded Verifiable=%v, want %v", s.Name, s.Verifiable, wantV)
		}
		if s.State != model.StepDone {
			t.Errorf("step %q is %s, want done", s.Name, s.State)
		}
	}

	if got := r.ksim.DeletedNodes(); !slices.Equal(got, []string{"w-1"}) {
		t.Fatalf("deleted nodes %v, want exactly [w-1]", got)
	}
	nodes, err := r.kubeClient(t).Nodes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range nodes {
		if n.Name == "w-1" {
			t.Error("the Node object is still listed after the removal")
		}
	}

	conns := r.connects()
	atWipe := conns[len(conns)-1]
	if len(atWipe.DeletedNodes) != 0 || atWipe.Node.Resets != 0 {
		t.Errorf("when the wipe connected, deleted=%v resets=%d; the object must still be there "+
			"(the kubelet could put it back before the wipe)", atWipe.DeletedNodes, atWipe.Node.Resets)
	}
	forgets := r.forgets()
	if len(forgets) != 1 {
		t.Fatalf("the inventory forgot %d machines, want 1", len(forgets))
	}
	if !slices.Equal(forgets[0].DeletedNodes, []string{"w-1"}) || forgets[0].Node.Resets != 1 {
		t.Errorf("when the machine was forgotten, deleted=%v resets=%d; the object must be gone "+
			"and the node wiped by then", forgets[0].DeletedNodes, forgets[0].Node.Resets)
	}
}

// TestAKubernetesThatIsGoneAfterTheWipeFailsTheRemovalWithTheCommandToRun: the
// Kubernetes endpoint of a control-plane node's configuration may be that
// node's own address. After the wipe nothing answers there, the Node object
// cannot be deleted, and the job says so -- with the command that finishes it
// -- instead of passing over it.
func TestAKubernetesThatIsGoneAfterTheWipeFailsTheRemovalWithTheCommandToRun(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		host     string
		cps, wks int
		says     []string
	}{
		{"w-1", 1, 1, []string{"kubectl delete node w-1", "has been wiped", "stays listed here"}},
		// A control-plane node has also left etcd, and the sentence says so; a
		// worker never was a member and the sentence does not claim it.
		{"cp-1", 3, 0, []string{"kubectl delete node cp-1", "has been wiped and has left etcd", "stays listed here"}},
	} {
		t.Run(tc.host, func(t *testing.T) {
			t.Parallel()
			r := newRemovalRigWith(t, tc.cps, tc.wks, removalRigOptions{KubeDownAfterWipe: true})

			j := r.await(r.engine, r.submit(r.engine, tc.host).ID, model.JobFailed)

			if cur := j.Steps[j.Current].Name; cur != "delete the Kubernetes node object" {
				t.Fatalf("the job failed in %q, want the deletion of the Node object", cur)
			}
			d := stepNamed(j, "delete the Kubernetes node object").Detail
			for _, want := range tc.says {
				if !strings.Contains(d, want) {
					t.Errorf("the failure %q does not say %q", d, want)
				}
			}
			if tc.host == "w-1" && strings.Contains(d, "left etcd") {
				t.Errorf("the failure %q claims a worker left etcd", d)
			}
			if got := r.sim(tc.host).Node().Resets; got != 1 {
				t.Errorf("the node was wiped %d times, want once", got)
			}
			if !r.known(tc.host) {
				t.Error("the machine was forgotten although the Node object could not be deleted")
			}
			if st := stepNamed(j, "forget the node").State; st != model.StepPending {
				t.Errorf("the forget step is %s, want pending: it did not run", st)
			}
		})
	}
}

// TestAFailedNodeObjectDeletionFailsTheRemovalAndNamesTheNodeTheClusterCalledIt:
// the API answers and refuses. The name in the command is the one the cluster
// listed, not text from the request.
func TestAFailedNodeObjectDeletionFailsTheRemovalAndNamesTheNodeTheClusterCalledIt(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{
		// The cluster knows the kubelet as W-1; the inventory says w-1.
		Nodes: []kubesim.Node{{Name: "W-1"}},
		Sim: func(o *kubesim.Options) {
			o.Nodes = slices.DeleteFunc(o.Nodes, func(n kubesim.Node) bool { return n.Name == "w-1" })
		},
	})
	r.ksim.FailNodeDeletes(true)

	j := r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobFailed)

	if cur := j.Steps[j.Current].Name; cur != "delete the Kubernetes node object" {
		t.Fatalf("the job failed in %q, want the deletion of the Node object", cur)
	}
	d := stepNamed(j, "delete the Kubernetes node object").Detail
	for _, want := range []string{"kubectl delete node W-1", "has been wiped", "stays listed here"} {
		if !strings.Contains(d, want) {
			t.Errorf("the failure %q does not say %q", d, want)
		}
	}
	if !r.known("w-1") || r.sim("w-1").Node().Resets != 1 {
		t.Error("the node must be wiped and still in the inventory")
	}
}

// TestARestartBeforeTheNodeObjectDeletionStillDeletesIt: the wipe is done, the
// object is still there.
func TestARestartBeforeTheNodeObjectDeletionStillDeletesIt(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 1, 1)
	r.stage(t, r.machine("w-1"), "delete the Kubernetes node object", model.StepPending)

	second := r.newEngine()
	if err := second.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	all, _ := second.List(t.Context())
	r.await(second, all[0].ID, model.JobSucceeded)

	if got := r.ksim.DeletedNodes(); !slices.Equal(got, []string{"w-1"}) {
		t.Errorf("deleted nodes %v, want [w-1]", got)
	}
	if got := r.sim("w-1").Node().Resets; got != 0 {
		t.Errorf("a resume at the deletion wiped the node %d times", got)
	}
	if r.known("w-1") {
		t.Error("the machine is still in the inventory")
	}
}

// TestARestartInsideTheNodeObjectDeletionDoesNotFailOnANodeThatIsAlreadyGone:
// the DELETE went through just before the crash. The step asks, finds no node
// and is skipped; it does not fail on the second DELETE.
func TestARestartInsideTheNodeObjectDeletionDoesNotFailOnANodeThatIsAlreadyGone(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 1, 1)
	if err := r.kubeClient(t).DeleteNode(t.Context(), "w-1"); err != nil {
		t.Fatalf("DeleteNode: %v", err)
	}
	r.stage(t, r.machine("w-1"), "delete the Kubernetes node object", model.StepRunning)

	second := r.newEngine()
	if err := second.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	all, _ := second.List(t.Context())
	after := r.await(second, all[0].ID, model.JobSucceeded)

	if st := stepNamed(after, "delete the Kubernetes node object").State; st != model.StepSkipped {
		t.Errorf("the deletion step is %s, want skipped: the object was already gone", st)
	}
	if r.known("w-1") {
		t.Error("the machine is still in the inventory")
	}
}

// renamedAfterTheWipe is an inventory that has refreshed a machine after its
// wipe: a machine in maintenance mode asks DHCP again and comes back under
// another hostname and another address. Whether a real inventory does that
// between the wipe and the deletion of the Node object is not known; the
// review could not rule it out, so the rig makes it so.
func renamedAfterTheWipe(host string) func(*removalRig, model.Machine) model.Machine {
	return func(r *removalRig, m model.Machine) model.Machine {
		if m.Hostname == host && r.sim(host).Node().Resets > 0 {
			m.Hostname = "talos-2xk-9qa"
			m.Addr = "192.168.1.99"
		}
		return m
	}
}

// renamedFromTheStart is the same inventory without the wait for the wipe, for
// a test that stages a job at the deletion step.
func renamedFromTheStart(host string) func(*removalRig, model.Machine) model.Machine {
	return func(_ *removalRig, m model.Machine) model.Machine {
		if m.Hostname == host {
			m.Hostname = "talos-2xk-9qa"
			m.Addr = "192.168.1.99"
		}
		return m
	}
}

// recordNodeName writes the parameter the cordon step leaves into a staged job,
// the way a process that died after that step would have stored it.
func recordNodeName(t *testing.T, r *removalRig, name string) {
	t.Helper()
	j, err := r.st.Jobs().Get(t.Context(), "staged-removal")
	if err != nil {
		t.Fatal(err)
	}
	if j.Params == nil {
		j.Params = map[string]string{}
	}
	j.Params["kubernetes_node"] = name
	if _, err := r.st.Jobs().Put(t.Context(), j); err != nil {
		t.Fatal(err)
	}
}

// TestTheNodeObjectIsDeletedByTheNameTheCordonFoundWhenTheInventoryChangedAfterTheWipe
// is WR-02: the deletion used to look the machine up again by hostname and
// address, and a machine the inventory calls something else after its wipe was
// found nowhere, which the step reported as "nothing to delete": success, with
// the Node object still listed. The cordon step now keeps the name the cluster
// gave the node, and that name is what is deleted.
func TestTheNodeObjectIsDeletedByTheNameTheCordonFoundWhenTheInventoryChangedAfterTheWipe(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		nodes []kubesim.Node
	}{
		{"schedulable", nil},
		// The name is held when the node was cordoned before the job as well:
		// the deletion needs it either way.
		{"already-cordoned", []kubesim.Node{{Name: "w-1", Unschedulable: true}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newRemovalRigWith(t, 1, 1, removalRigOptions{
				Nodes:     tc.nodes,
				Inventory: renamedAfterTheWipe("w-1"),
			})

			j := r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobSucceeded)

			if got := r.ksim.DeletedNodes(); !slices.Equal(got, []string{"w-1"}) {
				t.Fatalf("deleted nodes %v, want exactly [w-1]", got)
			}
			nodes, err := r.kubeClient(t).Nodes(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			for _, n := range nodes {
				if n.Name == "w-1" {
					t.Error("the Node object is still listed after the removal")
				}
			}
			d := stepNamed(j, "delete the Kubernetes node object").Detail
			if !strings.Contains(d, "w-1") || strings.Contains(d, "nothing to delete") {
				t.Errorf("the deletion step says %q, want it to name w-1 and not say there was nothing to delete", d)
			}
			if got := j.Params["kubernetes_node"]; got != "w-1" {
				t.Errorf("the job's parameters are %v, want kubernetes_node=w-1", j.Params)
			}
		})
	}
}

// TestARecordedNodeObjectIsNeverReportedAsNothingToDelete: once the cordon step
// has kept a name there is a Node object to delete, and the deletion either
// deletes it or fails with the command to run. Kubernetes that does not answer,
// a configuration that has lost its Kubernetes authority since, and a DELETE the
// API refuses are the three ways it cannot; none is a skip, and none names the
// machine by the hostname the inventory now has.
func TestARecordedNodeObjectIsNeverReportedAsNothingToDelete(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		opts func(*removalRigOptions)
		pre  func(*removalRig)
	}{
		{"kubernetes-gone", func(o *removalRigOptions) { o.KubeDownAfterWipe = true }, nil},
		{"authority-gone", func(o *removalRigOptions) {
			o.Kube = func(r *removalRig) func(context.Context, model.ClusterID) (*kube.Client, error) {
				return func(ctx context.Context, cluster model.ClusterID) (*kube.Client, error) {
					if r.sim("w-1").Node().Resets > 0 {
						return nil, fmt.Errorf("inventory: cluster homelab: %w", kube.ErrNoKubernetesAuthority)
					}
					return r.newKubeClient(ctx, cluster)
				}
			}
		}, nil},
		{"delete-refused", func(*removalRigOptions) {}, func(r *removalRig) { r.ksim.FailNodeDeletes(true) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := removalRigOptions{Inventory: renamedAfterTheWipe("w-1")}
			tc.opts(&opts)
			r := newRemovalRigWith(t, 1, 1, opts)
			if tc.pre != nil {
				tc.pre(r)
			}

			j := r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobFailed)

			if cur := j.Steps[j.Current].Name; cur != "delete the Kubernetes node object" {
				t.Fatalf("the job failed in %q, want the deletion of the Node object", cur)
			}
			d := stepNamed(j, "delete the Kubernetes node object").Detail
			for _, want := range []string{"kubectl delete node w-1", "has been wiped"} {
				if !strings.Contains(d, want) {
					t.Errorf("the failure %q does not say %q", d, want)
				}
			}
			for _, not := range []string{
				"kubectl delete node talos-2xk-9qa", "nothing to delete", "no Kubernetes access configured",
			} {
				if strings.Contains(d, not) {
					t.Errorf("the failure %q says %q", d, not)
				}
			}
			if !r.known("w-1") {
				t.Error("the machine was forgotten although the Node object could not be deleted")
			}
			if st := stepNamed(j, "forget the node").State; st != model.StepPending {
				t.Errorf("the forget step is %s, want pending: it did not run", st)
			}
		})
	}
}

// TestARestartInsideTheNodeObjectDeletionAsksForTheRecordedName: after a
// restart the step asks about the name that was kept, not about the machine's
// hostname, which the inventory has since changed. A node that is listed is
// deleted; a Kubernetes that does not answer fails the step with the command
// and is not taken for "already gone".
func TestARestartInsideTheNodeObjectDeletionAsksForTheRecordedName(t *testing.T) {
	t.Parallel()

	t.Run("kubernetes-answers", func(t *testing.T) {
		t.Parallel()
		r := newRemovalRigWith(t, 1, 1, removalRigOptions{Inventory: renamedFromTheStart("w-1")})
		r.stage(t, r.machine("w-1"), "delete the Kubernetes node object", model.StepRunning)
		recordNodeName(t, r, "w-1")

		second := r.newEngine()
		if err := second.Resume(t.Context()); err != nil {
			t.Fatal(err)
		}
		after := r.await(second, "staged-removal", model.JobSucceeded)

		if st := stepNamed(after, "delete the Kubernetes node object").State; st != model.StepDone {
			t.Errorf("the deletion step is %s, want done: the node was still listed", st)
		}
		if got := r.ksim.DeletedNodes(); !slices.Equal(got, []string{"w-1"}) {
			t.Errorf("deleted nodes %v, want [w-1]", got)
		}
	})

	t.Run("kubernetes-silent", func(t *testing.T) {
		t.Parallel()
		r := newRemovalRigWith(t, 1, 1, removalRigOptions{
			Inventory: renamedFromTheStart("w-1"),
			Kube: func(*removalRig) func(context.Context, model.ClusterID) (*kube.Client, error) {
				return func(context.Context, model.ClusterID) (*kube.Client, error) {
					return nil, errors.New("dial tcp 192.168.1.10:6443: connect: connection refused")
				}
			},
		})
		r.stage(t, r.machine("w-1"), "delete the Kubernetes node object", model.StepRunning)
		recordNodeName(t, r, "w-1")

		second := r.newEngine()
		if err := second.Resume(t.Context()); err != nil {
			t.Fatal(err)
		}
		after := r.await(second, "staged-removal", model.JobFailed)

		if cur := after.Steps[after.Current].Name; cur != "delete the Kubernetes node object" {
			t.Fatalf("the job failed in %q, want the deletion of the Node object", cur)
		}
		if d := stepNamed(after, "delete the Kubernetes node object").Detail; !strings.Contains(d, "kubectl delete node w-1") {
			t.Errorf("the failure %q does not name the command for w-1", d)
		}
		if !r.known("w-1") {
			t.Error("the machine was forgotten although the Node object was not deleted")
		}
	})
}

// TestAStaleRecordedNodeNameIsNeverDeleted: a name that stood in the job's
// parameters before the cordon step ran is not the cordon step's. When that
// step finds no node for the machine it takes the parameter away, and the
// deletion deletes nothing -- least of all the other node the stale name is.
func TestAStaleRecordedNodeNameIsNeverDeleted(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{
		Nodes: []kubesim.Node{{Name: "w-9"}},
		Sim: func(o *kubesim.Options) {
			o.Nodes = slices.DeleteFunc(o.Nodes, func(n kubesim.Node) bool { return n.Name == "w-1" })
		},
	})
	r.stage(t, r.machine("w-1"), "cordon the node", model.StepPending)
	recordNodeName(t, r, "w-9")

	second := r.newEngine()
	if err := second.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	after := r.await(second, "staged-removal", model.JobSucceeded)

	if slices.Contains(r.ksim.DeletedNodes(), "w-9") {
		t.Error("the stale name was deleted: a Node object that is not the machine's is gone")
	}
	nodes, err := r.kubeClient(t).Nodes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(nodes, func(n kube.Node) bool { return n.Name == "w-9" }) {
		t.Error("w-9 is no longer listed")
	}
	if v, ok := after.Params["kubernetes_node"]; ok {
		t.Errorf("the job still carries kubernetes_node=%q after a cordon step that found no node", v)
	}
}
