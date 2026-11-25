package upgrade_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// The gate before a worker's reboot (milestone v1.17, phase 18, D-02).
//
// Every test of this file is sequential: they call upgrade.FastPolling, which
// changes a package variable, and Go runs a parallel test only after every
// sequential one has finished, which is what makes the change safe.

const gateTarget = "v1.14.0"

// gateStepPrefix and upgradeStepPrefix are how the step list of a Talos upgrade
// is read: by name, never by position.
const (
	gateStepPrefix    = "check Kubernetes can spare "
	upgradeStepPrefix = "upgrade "
)

// upgradeWorkers submits a Talos upgrade of the named hosts to the rig's engine
// and returns the job as submitted. The request names workers only: the rig has
// no etcd gate, and a control-plane node would ask for one.
func upgradeWorkers(t *testing.T, r *removalRig, e *jobs.Engine, hosts ...string) model.Job {
	t.Helper()
	req := upgrade.Request{To: gateTarget, Installers: map[model.MachineID]string{}}
	for _, h := range hosts {
		m := r.machine(h)
		req.Machines = append(req.Machines, m.ID)
		req.Installers[m.ID] = "ghcr.io/siderolabs/installer:" + gateTarget
	}
	j, err := e.Submit(t.Context(), model.Job{
		Kind: upgrade.JobKindTalosUpgrade, Cluster: removeCluster, Params: req.Params(), Actor: "test",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	return j
}

// dryRunCount is how many dry-run evictions kubesim has been asked for.
func dryRunCount(r *removalRig) int { return len(r.ksim.DryRunEvictions()) }

// TestAWorkerUpgradeStopsAtTheGateWhenABudgetWouldHoldAPod is the point of the
// gate: Talos drains a worker in its own upgrade and gives up after five
// minutes, so a budget that holds a pod would not protect it. The step before
// the upgrade says so, and nothing has been called on the node.
func TestAWorkerUpgradeStopsAtTheGateWhenABudgetWouldHoldAPod(t *testing.T) {
	r := newRemovalRigWith(t, 1, 2, removalRigOptions{Pods: []kubesim.Pod{budgetedPod("w-1")}})
	r.deps.WorkerGateBudget = 300 * time.Millisecond
	e := r.newEngine()
	upgrade.FastPolling(t)

	j := r.await(e, upgradeWorkers(t, r, e, "w-1").ID, model.JobFailed)

	id := string(r.machine("w-1").ID)
	gate := stepNamed(j, gateStepPrefix+id)
	if gate.Name == "" {
		t.Fatalf("the job has no gate step for %s: %v", id, stepNames(j))
	}
	if gate.State != model.StepFailed {
		t.Fatalf("the gate step is %s, want failed", gate.State)
	}
	for _, want := range []string{"db-pdb", "default/db-0", "w-1", "replica", "budget"} {
		if !strings.Contains(gate.Detail, want) {
			t.Errorf("the failure %q does not say %q", gate.Detail, want)
		}
	}
	if up := stepNamed(j, upgradeStepPrefix+id+" to "+gateTarget); up.State != model.StepPending {
		t.Errorf("the upgrade step of the held node is %s, want pending", up.State)
	}
	if n := r.sim("w-1").Calls("ImagePull"); n != 0 {
		t.Errorf("ImagePull was called %d times on a node the gate had refused", n)
	}
	if n := r.sim("w-1").Calls("Upgrade"); n != 0 {
		t.Errorf("Upgrade was called %d times on a node the gate had refused", n)
	}
	if got := r.ksim.Evicted(); len(got) != 0 {
		t.Errorf("the gate evicted %v: it asks, it never changes a workload", got)
	}
	if n := dryRunCount(r); n < 2 {
		t.Errorf("the gate asked %d times, want it asked again while it waited", n)
	}
}

// TestTheGateChangesNothingWhenItLetsTheUpgradeThrough: D-02, the gate never
// changes a workload. A dry-run eviction is the question, and kubesim must have
// removed nothing.
func TestTheGateChangesNothingWhenItLetsTheUpgradeThrough(t *testing.T) {
	r := newRemovalRigWith(t, 1, 2, removalRigOptions{Pods: []kubesim.Pod{
		replicaPod("web-1", "w-1"), replicaPod("web-2", "w-1"),
	}})
	e := r.newEngine()
	upgrade.FastPolling(t)

	j := r.await(e, upgradeWorkers(t, r, e, "w-1").ID, model.JobSucceeded)

	if got := r.ksim.Evicted(); len(got) != 0 {
		t.Errorf("evicted %v: the gate's question is a dry run", got)
	}
	if got := r.ksim.PodsOn("w-1"); !slices.Equal(got, []string{"default/web-1", "default/web-2"}) {
		t.Errorf("the pods on w-1 are %v after the gate, want both still there", got)
	}
	got := r.ksim.DryRunEvictions()
	for _, want := range []string{"default/web-1", "default/web-2"} {
		if !slices.Contains(got, want) {
			t.Errorf("the gate never asked about %s (asked about %v)", want, got)
		}
	}
	if n := r.sim("w-1").Calls("Upgrade"); n != 1 {
		t.Errorf("Upgrade was called %d times, want once", n)
	}
	gate := stepNamed(j, gateStepPrefix+string(r.machine("w-1").ID))
	if gate.State != model.StepDone {
		t.Fatalf("the gate step is %s, want done", gate.State)
	}
	if !strings.Contains(gate.Detail, "can spare w-1") || !strings.Contains(gate.Detail, "2 pods") {
		t.Errorf("the gate's detail %q does not say it ran and how many pods it asked about", gate.Detail)
	}
}

// TestTheGateWaitsForABudgetToOpen: the second worker of a rolling upgrade
// meets the gate while the first one's pods are still coming back, so a gate
// that refused at once would fail every rolling upgrade at its second node.
func TestTheGateWaitsForABudgetToOpen(t *testing.T) {
	r := newRemovalRigWith(t, 1, 2, removalRigOptions{Pods: []kubesim.Pod{budgetedPod("w-1")}})
	r.deps.WorkerGateBudget = 5 * time.Second
	e := r.newEngine()
	upgrade.FastPolling(t)

	go func() {
		for dryRunCount(r) < 2 && r.t.Context().Err() == nil {
			time.Sleep(2 * time.Millisecond)
		}
		_ = r.ksim.SetBudgetRefuses("default", "db-0", false)
	}()

	r.await(e, upgradeWorkers(t, r, e, "w-1").ID, model.JobSucceeded)

	if n := dryRunCount(r); n < 3 {
		t.Errorf("the gate asked %d times, want at least three (held twice, then open)", n)
	}
	if n := r.sim("w-1").Calls("Upgrade"); n != 1 {
		t.Errorf("Upgrade was called %d times, want once", n)
	}
}

// TestTheGateStepComesBeforeTheUpgradeStepOfEveryNode: gate, upgrade, gate,
// upgrade. Each gate reads the cluster as the node before it left it.
func TestTheGateStepComesBeforeTheUpgradeStepOfEveryNode(t *testing.T) {
	r := newRemovalRig(t, 1, 2)
	e := r.newEngine()
	upgrade.FastPolling(t)

	j := r.await(e, upgradeWorkers(t, r, e, "w-1", "w-2").ID, model.JobSucceeded)

	names := stepNames(j)
	if len(names) != 4 {
		t.Fatalf("steps = %v, want four", names)
	}
	for i, n := range names {
		want := gateStepPrefix
		if i%2 == 1 {
			want = upgradeStepPrefix
		}
		if !strings.HasPrefix(n, want) {
			t.Errorf("step %d is %q, want one starting with %q (steps %v)", i, n, want, names)
		}
	}
	for _, s := range j.Steps {
		if s.State != model.StepDone {
			t.Errorf("step %q is %s, want done", s.Name, s.State)
		}
	}
}

// TestTheGateEndsAtItsBudgetEvenWhenKubernetesNeverAnswers: the engine puts no
// time limit on a step, so the gate's own budget is the only thing that ends it
// -- also when one call to Kubernetes hangs, which is an API server that takes
// the connection and never answers. A loop without a deadline on each pass
// would hang until the rig gives up after thirty seconds.
func TestTheGateEndsAtItsBudgetEvenWhenKubernetesNeverAnswers(t *testing.T) {
	r := newRemovalRigWith(t, 1, 2, removalRigOptions{
		Kube: func(*removalRig) func(context.Context, model.ClusterID) (*kube.Client, error) {
			return func(ctx context.Context, _ model.ClusterID) (*kube.Client, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}
		},
	})
	r.deps.WorkerGateBudget = 300 * time.Millisecond
	e := r.newEngine()
	upgrade.FastPolling(t)

	started := time.Now()
	j := r.await(e, upgradeWorkers(t, r, e, "w-1").ID, model.JobFailed)
	if took := time.Since(started); took > 5*time.Second {
		t.Errorf("the gate took %s to end on a budget of 300ms", took)
	}

	gate := stepNamed(j, gateStepPrefix+string(r.machine("w-1").ID))
	if gate.State != model.StepFailed {
		t.Fatalf("the gate step is %s, want failed", gate.State)
	}
	for _, want := range []string{"w-1", "did not answer"} {
		if !strings.Contains(gate.Detail, want) {
			t.Errorf("the failure %q does not say %q", gate.Detail, want)
		}
	}
	for _, bad := range []string{"context deadline exceeded", "context canceled"} {
		if strings.Contains(gate.Detail, bad) {
			t.Errorf("the failure %q carries a context error (%s) where a sentence belongs", gate.Detail, bad)
		}
	}
	if n := r.sim("w-1").Calls("ImagePull"); n != 0 {
		t.Errorf("ImagePull was called %d times on a node whose gate never got an answer", n)
	}
}

// TestTheGateWaitEndsWhenTheJobIsCancelled: the context bounds the loop from
// outside. A cancel ends the wait at once and reaches the engine as a
// cancellation -- not as the gate's own sentence, which would make it a failure.
func TestTheGateWaitEndsWhenTheJobIsCancelled(t *testing.T) {
	r := newRemovalRigWith(t, 1, 2, removalRigOptions{Pods: []kubesim.Pod{budgetedPod("w-1")}})
	r.deps.WorkerGateBudget = 60 * time.Second
	e := r.newEngine()
	upgrade.FastPolling(t)

	job := upgradeWorkers(t, r, e, "w-1")
	go func() {
		for dryRunCount(r) < 2 && r.t.Context().Err() == nil {
			time.Sleep(2 * time.Millisecond)
		}
		_, _ = e.Cancel(r.t.Context(), job.ID)
	}()

	started := time.Now()
	j := r.await(e, job.ID, model.JobCancelled)
	if took := time.Since(started); took > 5*time.Second {
		t.Errorf("the cancel took %s to end the wait, want it at once and not after the budget", took)
	}

	gate := stepNamed(j, gateStepPrefix+string(r.machine("w-1").ID))
	if !strings.Contains(gate.Detail, "cancelled while this step was running") {
		t.Errorf("the gate step says %q, want the engine's words for a cancel", gate.Detail)
	}
	if strings.Contains(gate.Detail, "db-pdb") {
		t.Errorf("the gate step says %q: a cancel must not become the gate's sentence", gate.Detail)
	}
	if n := r.sim("w-1").Calls("ImagePull"); n != 0 {
		t.Errorf("ImagePull was called %d times on a cancelled gate", n)
	}
}

// gateStepOf is the gate step of one host in a finished job.
func gateStepOf(r *removalRig, j model.Job, host string) model.JobStep {
	return stepNamed(j, gateStepPrefix+string(r.machine(host).ID))
}

// TestTheGateRefusesWhileAnotherNodeIsNotReady: losing a worker when another
// one is already out is the second failure. The node is named and nothing was
// called on the one being upgraded.
func TestTheGateRefusesWhileAnotherNodeIsNotReady(t *testing.T) {
	r := newRemovalRigWith(t, 1, 2, removalRigOptions{
		Nodes: []kubesim.Node{{Name: "w-2", Ready: corev1.ConditionFalse}},
	})
	r.deps.WorkerGateBudget = 300 * time.Millisecond
	e := r.newEngine()
	upgrade.FastPolling(t)

	j := r.await(e, upgradeWorkers(t, r, e, "w-1").ID, model.JobFailed)

	gate := gateStepOf(r, j, "w-1")
	if gate.State != model.StepFailed {
		t.Fatalf("the gate step is %s, want failed", gate.State)
	}
	if !strings.Contains(gate.Detail, "w-2") || !strings.Contains(gate.Detail, "Ready=False") {
		t.Errorf("the failure %q does not name w-2 and its state", gate.Detail)
	}
	if n := r.sim("w-1").Calls("ImagePull"); n != 0 {
		t.Errorf("ImagePull was called %d times on a node the gate had refused", n)
	}
	if n := r.sim("w-1").Calls("Upgrade"); n != 0 {
		t.Errorf("Upgrade was called %d times on a node the gate had refused", n)
	}
}

// TestTheGateWaitsForTheNodeToComeBack: the second worker of a rolling upgrade
// meets the gate while the first one is still coming back.
func TestTheGateWaitsForTheNodeToComeBack(t *testing.T) {
	r := newRemovalRigWith(t, 1, 2, removalRigOptions{
		Nodes: []kubesim.Node{{Name: "w-2", Ready: corev1.ConditionFalse}},
	})
	r.deps.WorkerGateBudget = 5 * time.Second
	e := r.newEngine()
	upgrade.FastPolling(t)

	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = r.ksim.SetReady("w-2", corev1.ConditionTrue)
	}()

	j := r.await(e, upgradeWorkers(t, r, e, "w-1").ID, model.JobSucceeded)
	if gate := gateStepOf(r, j, "w-1"); gate.State != model.StepDone {
		t.Errorf("the gate step is %s, want done", gate.State)
	}
	if n := r.sim("w-1").Calls("Upgrade"); n != 1 {
		t.Errorf("Upgrade was called %d times, want once", n)
	}
}

// TestADisabledNodeDoesNotCloseTheGate: a node that is off on purpose is not a
// node that failed, and without this exception a gate would refuse every
// upgrade for as long as the operator had switched a node off.
func TestADisabledNodeDoesNotCloseTheGate(t *testing.T) {
	r := newRemovalRigWith(t, 1, 2, removalRigOptions{
		Nodes: []kubesim.Node{{Name: "w-2", Ready: corev1.ConditionFalse}},
	})
	r.mu.Lock()
	for i := range r.machines {
		if r.machines[i].Hostname == "w-2" {
			r.machines[i].Disabled = true
		}
	}
	r.mu.Unlock()
	e := r.newEngine()
	upgrade.FastPolling(t)

	j := r.await(e, upgradeWorkers(t, r, e, "w-1").ID, model.JobSucceeded)

	if d := gateStepOf(r, j, "w-1").Detail; !strings.Contains(d, "w-2") || !strings.Contains(d, "off on purpose") {
		t.Errorf("the gate's detail %q does not name w-2 as left out because it is switched off", d)
	}
}

// TestTheNodeBeingUpgradedMayItselfBeNotReady: its reboot takes nothing from a
// node that is not serving, and the detail says that the gate let it through
// for that reason.
func TestTheNodeBeingUpgradedMayItselfBeNotReady(t *testing.T) {
	r := newRemovalRigWith(t, 1, 2, removalRigOptions{
		Nodes: []kubesim.Node{{Name: "w-1", Ready: corev1.ConditionFalse}},
	})
	e := r.newEngine()
	upgrade.FastPolling(t)

	j := r.await(e, upgradeWorkers(t, r, e, "w-1").ID, model.JobSucceeded)

	if d := gateStepOf(r, j, "w-1").Detail; !strings.Contains(d, "not Ready itself") {
		t.Errorf("the gate's detail %q does not say the node being upgraded is not Ready", d)
	}
}

// TestTheGateRefusesWhenTheClusterCannotSayWhetherAPodMayGo: two budgets on one
// pod make the server answer 500, which is neither a veto nor consent.
func TestTheGateRefusesWhenTheClusterCannotSayWhetherAPodMayGo(t *testing.T) {
	r := newRemovalRigWith(t, 1, 2, removalRigOptions{Pods: []kubesim.Pod{{
		Namespace: "default", Name: "api-0", Node: "w-1", OwnerKind: "StatefulSet", BudgetsConflict: true,
	}}})
	r.deps.WorkerGateBudget = 300 * time.Millisecond
	e := r.newEngine()
	upgrade.FastPolling(t)

	j := r.await(e, upgradeWorkers(t, r, e, "w-1").ID, model.JobFailed)

	gate := gateStepOf(r, j, "w-1")
	if !strings.Contains(gate.Detail, "could not be asked") || !strings.Contains(gate.Detail, "default/api-0") {
		t.Errorf("the failure %q does not say that the cluster could not be asked about default/api-0", gate.Detail)
	}
	if n := r.sim("w-1").Calls("ImagePull"); n != 0 {
		t.Errorf("ImagePull was called %d times on a node the gate had refused", n)
	}
}

// TestAnUnknownKubernetesNodeIsASkipThatSaysSo: a machine that never joined has
// no pods to protect, and the gate says that it looked and found nothing.
func TestAnUnknownKubernetesNodeIsASkipThatSaysSo(t *testing.T) {
	r := newRemovalRigWith(t, 1, 2, removalRigOptions{
		Sim: func(o *kubesim.Options) {
			o.Nodes = slices.DeleteFunc(o.Nodes, func(n kubesim.Node) bool { return n.Name == "w-1" })
		},
	})
	e := r.newEngine()
	upgrade.FastPolling(t)

	j := r.await(e, upgradeWorkers(t, r, e, "w-1").ID, model.JobSucceeded)

	if d := gateStepOf(r, j, "w-1").Detail; !strings.Contains(d, "no node") {
		t.Errorf("the gate's detail %q does not say that Kubernetes knows no node for the machine", d)
	}
}

// TestTheGateNamesPodsNothingOwnsWithoutRefusing: no budget can protect a pod no
// controller owns, and Talos stops it at the reboot. The gate asks readiness and
// budgets of the cluster; it says what it saw and does not refuse for this.
func TestTheGateNamesPodsNothingOwnsWithoutRefusing(t *testing.T) {
	r := newRemovalRigWith(t, 1, 2, removalRigOptions{Pods: []kubesim.Pod{
		{Namespace: "default", Name: "scratch", Node: "w-1"},
		replicaPod("web-1", "w-1"),
	}})
	e := r.newEngine()
	upgrade.FastPolling(t)

	j := r.await(e, upgradeWorkers(t, r, e, "w-1").ID, model.JobSucceeded)

	d := gateStepOf(r, j, "w-1").Detail
	if !strings.Contains(d, "default/scratch") || !strings.Contains(d, "Talos stops") {
		t.Errorf("the gate's detail %q does not name the pod nothing owns and say that Talos stops it", d)
	}
	if got := r.ksim.Evicted(); len(got) != 0 {
		t.Errorf("evicted %v: the gate never changes a workload", got)
	}
}
