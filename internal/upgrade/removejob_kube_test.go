package upgrade_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// The Kubernetes side of a removal: what it gives back when it is stopped, what
// it refuses before it has changed anything, and what it will not do to a pod.
//
// The rig is the one removejob_test.go builds -- talossim nodes, kubesim as the
// cluster's API, a real job engine over the real file store. Nothing here ran
// against a live Kubernetes API server.

// cordonedFor is the job parameter the cordon step leaves when it was the one
// that cordoned the node.
const cordonedFor = "cordoned_by_job"

// nodeNamed reads one node through the daemon's own client.
func nodeNamed(t *testing.T, r *removalRig, name string) kube.Node {
	t.Helper()
	nodes, err := r.kubeClient(t).Nodes(t.Context())
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}
	for _, n := range nodes {
		if n.Name == name {
			return n
		}
	}
	t.Fatalf("the cluster has no node %q any more", name)
	return kube.Node{}
}

// TestAVetoedRemovalGivesBackTheCordonItMade is D-03 Q2: a removal that stops
// at a budget leaves the node as it found it, except for the pods that already
// moved. The node was schedulable when the job started, the job made it
// unschedulable, and the job makes it schedulable again.
func TestAVetoedRemovalGivesBackTheCordonItMade(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{
		Pods:        []kubesim.Pod{budgetedPod("w-1")},
		DrainBudget: 300 * time.Millisecond,
	})

	j := r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobFailed)

	if cur := j.Steps[j.Current].Name; cur != "drain the node" {
		t.Fatalf("the job failed in %q, want the drain", cur)
	}
	if nodeNamed(t, r, "w-1").Unschedulable {
		t.Error("w-1 is still cordoned: the removal left behind a cordon it made itself")
	}
	d := stepNamed(j, "drain the node").Detail
	if !strings.Contains(d, "uncordoned again") {
		t.Errorf("the failure %q does not say the cordon was taken back", d)
	}
	if j.Params[cordonedFor] != "true" {
		t.Errorf("the job's parameters are %v; the cordon step must have recorded that the cordon was its own",
			j.Params)
	}
	if n := r.sim("w-1").Node(); n.Resets != 0 || n.EtcdLeaves != 0 {
		t.Errorf("w-1 was touched: %+v", n)
	}
}

// TestAVetoedRemovalLeavesACordonThatWasNotItsOwn: a node the operator had
// cordoned before they asked for its removal stays cordoned when the removal
// stops. Releasing it would undo a decision somebody else made.
func TestAVetoedRemovalLeavesACordonThatWasNotItsOwn(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{
		Pods:        []kubesim.Pod{budgetedPod("w-1")},
		Nodes:       []kubesim.Node{{Name: "w-1", Unschedulable: true}},
		DrainBudget: 300 * time.Millisecond,
	})

	j := r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobFailed)

	if !nodeNamed(t, r, "w-1").Unschedulable {
		t.Error("w-1 was uncordoned although the job did not cordon it")
	}
	d := stepNamed(j, "drain the node").Detail
	if !strings.Contains(d, "already cordoned before") {
		t.Errorf("the failure %q does not say the cordon was there before the removal", d)
	}
	if v, ok := j.Params[cordonedFor]; ok {
		t.Errorf("the job recorded %s=%q for a cordon it did not make", cordonedFor, v)
	}
}

// TestTheRollbackSaysSoWhenItCannotBeDone: the cluster stops answering while
// the drain waits, so the cordon cannot be taken back. The sentence says the
// node is still cordoned and gives the command, instead of claiming it was
// released.
func TestTheRollbackSaysSoWhenItCannotBeDone(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{
		Pods:        []kubesim.Pod{budgetedPod("w-1")},
		DrainBudget: 5 * time.Second,
	})

	go func() {
		for r.ksim.Calls("POST /api/v1/namespaces/default/pods/db-0/eviction") < 2 && r.t.Context().Err() == nil {
			time.Sleep(2 * time.Millisecond)
		}
		r.ksim.Close()
	}()

	j := r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobFailed)

	d := stepNamed(j, "drain the node").Detail
	for _, want := range []string{"could not be uncordoned", "kubectl uncordon w-1"} {
		if !strings.Contains(d, want) {
			t.Errorf("the failure %q does not say %q", d, want)
		}
	}
	if strings.Contains(d, "has been uncordoned again") {
		t.Errorf("the failure %q claims a cordon was taken back that could not be", d)
	}
	if n := r.sim("w-1").Node(); n.Resets != 0 {
		t.Errorf("w-1 was wiped: %+v", n)
	}
}

// TestAShutdownInsideTheDrainGivesNothingBackAndTheRestartCarriesOn: the
// daemon stops while the drain waits for a budget. That is an interruption, not
// a verdict -- the job is not over -- so the cordon stays and the job stays
// running. After the restart the drain picks up where it was, and the removal
// finishes once the budget opens.
func TestAShutdownInsideTheDrainGivesNothingBackAndTheRestartCarriesOn(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{
		Pods:        []kubesim.Pod{budgetedPod("w-1")},
		DrainBudget: 30 * time.Second,
	})

	j := r.submit(r.engine, "w-1")
	deadline := time.Now().Add(30 * time.Second)
	for r.ksim.Calls("POST /api/v1/namespaces/default/pods/db-0/eviction") < 2 {
		if time.Now().After(deadline) {
			t.Fatal("the drain never asked for the pod a second time")
		}
		time.Sleep(2 * time.Millisecond)
	}
	if err := r.engine.Close(); err != nil {
		t.Fatal(err)
	}

	stored, err := r.st.Jobs().Get(t.Context(), j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != model.JobRunning || stepNamed(stored, "drain the node").State != model.StepRunning {
		t.Fatalf("the shutdown left the job %s with the drain %s, want both running: it was interrupted, "+
			"not ended", stored.State, stepNamed(stored, "drain the node").State)
	}
	if !nodeNamed(t, r, "w-1").Unschedulable {
		t.Error("the cordon was given back on a shutdown")
	}
	if stored.Params[cordonedFor] != "true" {
		t.Errorf("the stored parameters are %v; the note that the cordon was the job's own is gone", stored.Params)
	}

	if err := r.ksim.SetBudgetRefuses("default", "db-0", false); err != nil {
		t.Fatal(err)
	}
	second := r.newEngine()
	if err := second.Resume(t.Context()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	r.await(second, j.ID, model.JobSucceeded)

	if !slices.Equal(r.ksim.Evicted(), []string{"default/db-0"}) {
		t.Errorf("evicted %v, want the pod once the budget opened", r.ksim.Evicted())
	}
	if r.sim("w-1").Node().Resets != 1 {
		t.Error("the resumed removal did not wipe the node")
	}
}

// TestARemovalWithoutPermissionToDeleteNodesIsRefusedBeforeAnythingChanges is
// D-03 Q4: the identity the removal acts as is asked, before the first change,
// whether it may cordon, evict and delete. One that may not delete nodes would
// otherwise find out after the wipe.
func TestARemovalWithoutPermissionToDeleteNodesIsRefusedBeforeAnythingChanges(t *testing.T) {
	t.Parallel()
	const operator = "operator@example.com"
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{
		Pods: []kubesim.Pod{replicaPod("web-1", "w-1")},
		Sim: func(o *kubesim.Options) {
			o.AllowImpersonated = []string{operator}
			o.DenyVerbs = []string{"delete:nodes"}
		},
		Kube: func(r *removalRig) func(context.Context, model.ClusterID) (*kube.Client, error) {
			return func(ctx context.Context, cluster model.ClusterID) (*kube.Client, error) {
				admin, err := r.newKubeClient(ctx, cluster)
				if err != nil {
					return nil, err
				}
				return admin.As(kube.Identity{User: operator})
			}
		},
	})

	j := r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobFailed)

	if cur := j.Steps[j.Current].Name; cur != "cordon the node" {
		t.Fatalf("the job failed in %q, want the cordon, the first step that changes anything", cur)
	}
	d := stepNamed(j, "cordon the node").Detail
	for _, want := range []string{"delete", "nodes", operator, "Nothing has been changed"} {
		if !strings.Contains(d, want) {
			t.Errorf("the failure %q does not say %q", d, want)
		}
	}
	if nodeNamed(t, r, "w-1").Unschedulable {
		t.Error("w-1 was cordoned although the removal was refused")
	}
	if got := r.ksim.Evicted(); len(got) != 0 {
		t.Errorf("evicted %v although the removal was refused", got)
	}
	if n := r.sim("w-1").Node(); n.Resets != 0 || n.EtcdLeaves != 0 {
		t.Errorf("w-1 was touched: %+v", n)
	}
	if !r.known("w-1") {
		t.Error("w-1 was forgotten although the removal was refused")
	}
}

// TestABarePodRefusesTheRemovalAndIsNamed is D-03 Q5: a pod no controller owns
// is not coming back anywhere, so the removal never evicts it. It stops at
// once -- no waiting for something that will not change -- names the pod and
// says what a person has to do, after which the removal can be asked again.
func TestABarePodRefusesTheRemovalAndIsNamed(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{Pods: []kubesim.Pod{
		{Namespace: "default", Name: "scratch", Node: "w-1"},
		replicaPod("web-1", "w-1"),
	}})

	j := r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobFailed)

	if cur := j.Steps[j.Current].Name; cur != "drain the node" {
		t.Fatalf("the job failed in %q, want the drain", cur)
	}
	step := stepNamed(j, "drain the node")
	for _, want := range []string{"default/scratch", "deleted or recreated", "repeated"} {
		if !strings.Contains(step.Detail, want) {
			t.Errorf("the failure %q does not say %q", step.Detail, want)
		}
	}
	if took := step.FinishedAt.Sub(step.StartedAt); took > time.Second {
		t.Errorf("the drain took %s to give up on a pod nothing will ever move", took)
	}
	if got := podNames(t, r.kubeClient(t), "w-1"); !slices.Equal(got, []string{"default/scratch"}) {
		t.Errorf("pods left on w-1: %v, want only the bare one: it is never evicted, the other has moved", got)
	}
	if got := r.ksim.Deleted(); len(got) != 0 {
		t.Errorf("deleted %v: a pod nothing owns is named, never deleted", got)
	}
	if nodeNamed(t, r, "w-1").Unschedulable {
		t.Error("w-1 is still cordoned: the removal left behind a cordon it made itself")
	}
	if n := r.sim("w-1").Node(); n.Resets != 0 {
		t.Errorf("w-1 was wiped: %+v", n)
	}
}

// TestAPodWithLocalDataIsEvictedAndCounted: the node is about to be wiped, so
// the pod's emptyDir goes whatever the drain does. The pod is evicted and the
// screen says how many of them had local data.
func TestAPodWithLocalDataIsEvictedAndCounted(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{Pods: []kubesim.Pod{
		{Namespace: "default", Name: "cache-0", Node: "w-1", OwnerKind: "ReplicaSet", LocalData: true},
		replicaPod("web-1", "w-1"),
	}})

	j := r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobSucceeded)

	d := stepNamed(j, "drain the node").Detail
	if !strings.Contains(d, "2 evicted") || !strings.Contains(d, "1 with local storage") {
		t.Errorf("the drain says %q, want two evicted and one with local storage", d)
	}
	evicted := r.ksim.Evicted()
	slices.Sort(evicted)
	if want := []string{"default/cache-0", "default/web-1"}; !slices.Equal(evicted, want) {
		t.Errorf("evicted %v, want %v", evicted, want)
	}
}

// TestADeadKubeletIsNotWaitedFor is D-03 Q6: a node whose kubelet reports
// nothing cannot finish shutting a pod down, so waiting for the pod to leave
// would be waiting for the deadline. The pods are evicted, the removal goes
// on, and the screen says they stay terminating until the cluster's pod
// garbage collection removes them.
func TestADeadKubeletIsNotWaitedFor(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 1, 1, removalRigOptions{
		Nodes: []kubesim.Node{{Name: "w-1", Ready: corev1.ConditionUnknown}},
		Pods: []kubesim.Pod{{
			Namespace: "default", Name: "web-1", Node: "w-1", OwnerKind: "ReplicaSet",
			LingersAfterEviction: true, // and nothing ever finishes it
		}},
		DrainBudget: 500 * time.Millisecond,
	})

	j := r.await(r.engine, r.submit(r.engine, "w-1").ID, model.JobSucceeded)

	d := stepNamed(j, "drain the node").Detail
	for _, want := range []string{"kubelet", "terminating", "garbage collection"} {
		if !strings.Contains(d, want) {
			t.Errorf("the drain says %q, want it to say %q", d, want)
		}
	}
	if !slices.Equal(r.ksim.Evicted(), []string{"default/web-1"}) {
		t.Errorf("evicted %v, want the pod to have been evicted", r.ksim.Evicted())
	}
	if got := r.ksim.PodsOn("w-1"); !slices.Equal(got, []string{"default/web-1"}) {
		t.Errorf("pods still listed on w-1: %v; the pod is terminating and nothing finishes it", got)
	}
	if r.sim("w-1").Node().Resets != 1 {
		t.Error("the removal did not go on to wipe the node")
	}
}

// TestAControlPlaneRemovalThatEtcdCannotSpareChangesNothingInKubernetes: the
// quorum rule is asked before the cordon, because a rule that refuses only
// after the pods have moved has already cost something. Two control-plane
// nodes cannot spare one.
func TestAControlPlaneRemovalThatEtcdCannotSpareChangesNothingInKubernetes(t *testing.T) {
	t.Parallel()
	r := newRemovalRigWith(t, 2, 0, removalRigOptions{Pods: []kubesim.Pod{replicaPod("web-1", "cp-1")}})

	j := r.await(r.engine, r.submit(r.engine, "cp-1").ID, model.JobFailed)

	if cur := j.Steps[j.Current].Name; cur != "cordon the node" {
		t.Fatalf("the job failed in %q, want the cordon: the quorum rule comes before it", cur)
	}
	if d := stepNamed(j, "cordon the node").Detail; !strings.Contains(d, "voting members") {
		t.Errorf("the failure %q does not say why", d)
	}
	if nodeNamed(t, r, "cp-1").Unschedulable {
		t.Error("cp-1 was cordoned although etcd could not spare it")
	}
	if got := r.ksim.Evicted(); len(got) != 0 {
		t.Errorf("evicted %v although etcd could not spare the node", got)
	}
	if n := r.sim("cp-1").Node(); n.Resets != 0 || n.EtcdLeaves != 0 {
		t.Errorf("cp-1 was touched: %+v", n)
	}
	if !r.known("cp-1") {
		t.Error("cp-1 was forgotten")
	}
}

// TestAJobStoredWithTheOldStepListIsParked is DRAIN-05: a removal that was
// stored before this phase has fewer steps than the build now makes, and
// continuing it would run something other than what was submitted. The engine
// parks it and says so; no node is touched by the restart.
func TestAJobStoredWithTheOldStepListIsParked(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		host     string
		cps, wks int
		old      []string
	}{
		{"cp-1", 3, 0, []string{
			"check the node answers", "leave etcd", "let etcd settle", "wipe the node", "forget the node",
		}},
		{"w-1", 1, 1, []string{"check the node answers", "wipe the node", "forget the node"}},
	} {
		t.Run(tc.host, func(t *testing.T) {
			t.Parallel()
			r := newRemovalRigWith(t, tc.cps, tc.wks, removalRigOptions{
				Pods: []kubesim.Pod{replicaPod("web-1", tc.host)},
			})
			m := r.machine(tc.host)

			j := model.Job{
				ID: "stored-before-the-drain", Kind: model.JobRemoveFromCluster, Cluster: m.Cluster,
				Machine: m.ID, State: model.JobRunning, Current: 1, Params: upgrade.RemovalParams(m),
				CreatedAt: time.Now().Add(-time.Minute), StartedAt: time.Now().Add(-time.Minute),
			}
			for i, name := range tc.old {
				s := model.JobStep{Name: name, Verifiable: name != "wipe the node", State: model.StepPending}
				if i == 0 {
					s.State = model.StepDone
				}
				if i == 1 {
					s.State = model.StepRunning
				}
				j.Steps = append(j.Steps, s)
			}
			if _, err := r.st.Jobs().Put(t.Context(), j); err != nil {
				t.Fatal(err)
			}

			second := r.newEngine()
			if err := second.Resume(t.Context()); err != nil {
				t.Fatalf("Resume: %v", err)
			}
			got, err := second.Get(t.Context(), j.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.State != model.JobParked {
				t.Fatalf("the job is %s, want parked", got.State)
			}
			if !strings.Contains(got.ParkedReason, "could not be rebuilt as they were submitted") {
				t.Errorf("the parked reason is %q, want the engine's sentence about the steps", got.ParkedReason)
			}
			if n := r.sim(tc.host).Node(); n.Resets != 0 || n.EtcdLeaves != 0 {
				t.Errorf("%s was touched by the restart: %+v", tc.host, n)
			}
			if len(r.ksim.Evicted()) != 0 || len(r.ksim.DeletedNodes()) != 0 {
				t.Errorf("the cluster was changed by the restart: evicted %v, deleted %v",
					r.ksim.Evicted(), r.ksim.DeletedNodes())
			}
			if !r.known(tc.host) {
				t.Errorf("%s was forgotten", tc.host)
			}
		})
	}
}
