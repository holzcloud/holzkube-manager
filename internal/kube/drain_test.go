package kube_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// TestCordonIsVisibleInTheNextRead: a cordon that the next list does not show
// is a button that lies, and this is the property the simulator was built for.
func TestCordonIsVisibleInTheNextRead(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Nodes: []kubesim.Node{{Name: "cp-1"}}})

	if err := client.Cordon(ctx, "cp-1", true); err != nil {
		t.Fatalf("Cordon: %v", err)
	}
	nodes, err := client.Nodes(ctx)
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}
	if !nodes[0].Unschedulable {
		t.Fatal("the node is not unschedulable after a cordon, so the screen would offer to " +
			"schedule onto a node nothing may land on")
	}

	if err := client.Cordon(ctx, "cp-1", false); err != nil {
		t.Fatalf("Uncordon: %v", err)
	}
	nodes, err = client.Nodes(ctx)
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}
	if nodes[0].Unschedulable {
		t.Error("the node is still cordoned after an uncordon")
	}
}

// TestADrainTreatsTheFourSpecialCasesTheWayKubectlDoes is the test this slice
// exists for.
//
// A drain done wrong takes workloads down harder than `kubectl` would, which is
// worse than no drain: it is a tool nobody can trust at the moment they need
// it. Four kinds of pod are not an ordinary eviction, and each one is a decision
// this product must not make silently:
//
//   - a DaemonSet pod is recreated on the same node, so evicting it is a loop;
//   - a static pod belongs to the kubelet and the API server cannot evict it;
//   - a bare pod nothing owns is gone for good if it is evicted;
//   - a pod with an emptyDir loses that data when it moves.
func TestADrainTreatsTheFourSpecialCasesTheWayKubectlDoes(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{Name: "cp-1"}, {Name: "cp-2"}},
		Pods: []kubesim.Pod{
			{Namespace: "default", Name: "api-1", Node: "cp-1", OwnerKind: "ReplicaSet"},
			{Namespace: "kube-system", Name: "proxy-1", Node: "cp-1", OwnerKind: "DaemonSet"},
			{Namespace: "kube-system", Name: "kube-apiserver-cp-1", Node: "cp-1", Mirror: true},
			{Namespace: "default", Name: "debug", Node: "cp-1"},
			{Namespace: "default", Name: "cache-1", Node: "cp-1", OwnerKind: "ReplicaSet", LocalData: true},
			// On the OTHER node: a drain of cp-1 must not touch it.
			{Namespace: "default", Name: "api-2", Node: "cp-2", OwnerKind: "ReplicaSet"},
		},
	})

	result, err := client.Drain(ctx, "cp-1", kube.DrainOptions{})
	if !errors.Is(err, kube.ErrDrainBlocked) {
		t.Fatalf("Drain = %v, want ErrDrainBlocked: a bare pod and a pod with local storage are "+
			"both decisions the operator has to make", err)
	}

	if !slices.Contains(result.Evicted, "default/api-1") {
		t.Errorf("the ordinary pod was not evicted: %+v", result)
	}
	if !slices.Contains(result.SkippedDaemonSet, "kube-system/proxy-1") {
		t.Errorf("the DaemonSet pod was not skipped: evicting it is a loop, because it is "+
			"recreated on the same node. %+v", result)
	}
	if !slices.Contains(result.SkippedMirror, "kube-system/kube-apiserver-cp-1") {
		t.Errorf("the static pod was not skipped: the kubelet owns it and the API server cannot "+
			"evict it. %+v", result)
	}

	blocked := map[string]string{}
	for _, b := range result.Blocked {
		blocked[b.Pod] = b.Reason
	}
	if reason, ok := blocked["default/debug"]; !ok {
		t.Error("the bare pod did not block the drain; nothing would recreate it elsewhere")
	} else if !contains(reason, "nothing will recreate it") {
		t.Errorf("the bare pod's reason does not say what is lost: %q", reason)
	}
	if reason, ok := blocked["default/cache-1"]; !ok {
		t.Error("the pod with local storage did not block the drain")
	} else if !contains(reason, "local storage") {
		t.Errorf("the local-storage reason does not say what goes with the pod: %q", reason)
	}

	// The other node's pod was never touched, which the field selector is
	// responsible for -- and a fake that ignored it would have let this pass
	// while the product evicted a pod on a node nobody drained.
	if slices.Contains(sim.Evicted(), "default/api-2") {
		t.Error("draining cp-1 evicted a pod on cp-2")
	}

	// And the node is cordoned, which is what makes the half-finished state
	// safe: nothing new lands while the operator decides about the two pods.
	nodes, err := client.Nodes(ctx)
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}
	for _, n := range nodes {
		if n.Name == "cp-1" && !n.Unschedulable {
			t.Error("a drain that stopped left the node schedulable, so the scheduler can undo " +
				"what the drain achieved")
		}
	}
}

// TestADrainCanBeToldToTakeTheTwoDecisions: the flags exist because the answer
// is the operator's, and with them the same drain finishes.
func TestADrainCanBeToldToTakeTheTwoDecisions(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{Name: "cp-1"}},
		Pods: []kubesim.Pod{
			{Namespace: "default", Name: "debug", Node: "cp-1"},
			{Namespace: "default", Name: "cache-1", Node: "cp-1", OwnerKind: "ReplicaSet", LocalData: true},
		},
	})

	result, err := client.Drain(ctx, "cp-1", kube.DrainOptions{Force: true, DeleteLocalData: true})
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(result.Blocked) != 0 {
		t.Errorf("blocked = %+v, want none once the operator said so", result.Blocked)
	}
	if len(result.Evicted) != 2 {
		t.Errorf("evicted = %v, want both pods", result.Evicted)
	}

	// Really gone from the cluster, not merely reported: the eviction reached
	// the server and the server removed them.
	if got := sim.Evicted(); len(got) != 2 {
		t.Errorf("the server evicted %v, want both pods", got)
	}
	pods, err := client.Pods(ctx, "")
	if err != nil {
		t.Fatalf("Pods: %v", err)
	}
	if len(pods) != 0 {
		t.Errorf("the node still has %d pods after a finished drain", len(pods))
	}
}

// TestADisruptionBudgetStopsADrainAndIsNotRetriedAway is the reason client-go
// was the right choice for this slice.
//
// An eviction is refused with 429 when it would violate a PodDisruptionBudget;
// a delete is not refused at all. A drain built on delete ignores every promise
// the cluster's owners made about their own availability -- and a drain that
// retried the 429 until it won would be making that decision for them.
func TestADisruptionBudgetStopsADrainAndIsNotRetriedAway(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{Name: "cp-1"}},
		Pods: []kubesim.Pod{
			{
				Namespace: "default", Name: "quorum-1", Node: "cp-1",
				OwnerKind: "StatefulSet", BudgetRefuses: true,
			},
		},
	})

	result, err := client.Drain(ctx, "cp-1", kube.DrainOptions{})
	if !errors.Is(err, kube.ErrDrainBlocked) {
		t.Fatalf("Drain = %v, want ErrDrainBlocked", err)
	}
	if len(result.Blocked) != 1 || !contains(result.Blocked[0].Reason, "PodDisruptionBudget") {
		t.Fatalf("blocked = %+v, want the budget named: the operator's next move is to look at "+
			"the budget, not at this tool", result.Blocked)
	}
	if got := sim.Evicted(); len(got) != 0 {
		t.Errorf("the server evicted %v despite the budget", got)
	}
	if got := sim.Calls("POST /api/v1/namespaces/default/pods/quorum-1/eviction"); got != 1 {
		t.Errorf("the eviction was attempted %d times; a budget's refusal must not be retried "+
			"into submission", got)
	}
}

// TestAFinishedPodIsNotEvicted: Succeeded and Failed pods are not running, so
// there is nothing to move and evicting one only changes a record.
func TestAFinishedPodIsNotEvicted(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{Name: "cp-1"}},
		Pods: []kubesim.Pod{
			{Namespace: "default", Name: "backup-run", Node: "cp-1", Phase: corev1.PodSucceeded},
		},
	})

	result, err := client.Drain(ctx, "cp-1", kube.DrainOptions{})
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(result.Evicted) != 0 || len(result.Blocked) != 0 {
		t.Errorf("result = %+v, want a finished pod neither evicted nor blocking", result)
	}
	if got := sim.Evicted(); len(got) != 0 {
		t.Errorf("a finished pod was evicted: %v", got)
	}
}

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }

// evictionOf is the path a pod's eviction is POSTed to, which is what a test
// counts attempts by.
func evictionOf(namespace, name string) string {
	return "POST /api/v1/namespaces/" + namespace + "/pods/" + name + "/eviction"
}

// budgetPod is a StatefulSet's pod that a PodDisruptionBudget named db-pdb is
// holding.
func budgetPod(node string) kubesim.Pod {
	return kubesim.Pod{
		Namespace: "default", Name: "db-0", Node: node, OwnerKind: "StatefulSet",
		BudgetRefuses: true, BudgetName: "db-pdb",
	}
}

// TestADrainThatMayRetryWaitsOutABudgetAndThenFinishes is what a removal needs
// and a plain drain must not do: the budget belongs to somebody else, and when
// its owner lets it open -- a replacement became ready elsewhere -- the pod
// can go.
func TestADrainThatMayRetryWaitsOutABudgetAndThenFinishes(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{Name: "cp-1"}},
		Pods:  []kubesim.Pod{budgetPod("cp-1")},
	})

	// The budget opens after the third attempt. A drain that does not ask
	// again never gets there.
	go func() {
		for sim.Calls(evictionOf("default", "db-0")) < 3 && ctx.Err() == nil {
			time.Sleep(2 * time.Millisecond)
		}
		_ = sim.SetBudgetRefuses("default", "db-0", false)
	}()

	result, err := client.Drain(ctx, "cp-1", kube.DrainOptions{
		RetryBlocked: 10 * time.Millisecond, Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("Drain: %v (result %+v)", err, result)
	}
	if !slices.Equal(sim.Evicted(), []string{"default/db-0"}) {
		t.Errorf("the server evicted %v, want the pod once the budget opened", sim.Evicted())
	}
	if !slices.Equal(result.Evicted, []string{"default/db-0"}) {
		t.Errorf("the result lists %v as evicted, want the pod once, however many passes it took",
			result.Evicted)
	}
	if got := sim.Calls(evictionOf("default", "db-0")); got < 3 {
		t.Errorf("the eviction was attempted %d times, want at least 3", got)
	}
}

// TestADrainWhoseBudgetNeverOpensEndsAtItsDeadline: a budget with one replica
// never opens, and a loop with no end would run into the thirty seconds of the
// test's own context. The drain ends at its own deadline, with the verdict a
// drain gives -- blocked -- and not with the context's error.
//
// The deadline is the drain's own -- the context its Timeout derives -- but it
// runs out when the test says so: after the third refused eviction. A real
// 200 ms ran out on a loaded machine before the second pass had reached the
// server, and the test then read a drain that never asked again. Ending it on
// the attempt count rather than on the wall clock is what lets "asked again
// until the deadline" be a fact about the drain and not about the machine.
func TestADrainWhoseBudgetNeverOpensEndsAtItsDeadline(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{Name: "cp-1"}},
		Pods:  []kubesim.Pod{budgetPod("cp-1")},
	})

	var asked time.Duration
	expiredAt := make(chan time.Time, 1)
	client.SetDrainDeadline(func(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
		asked = d
		inner, cancel := context.WithCancel(parent)
		go func() {
			for sim.Calls(evictionOf("default", "db-0")) < 3 {
				select {
				case <-inner.Done():
					return
				case <-time.After(2 * time.Millisecond):
				}
			}
			expiredAt <- time.Now()
			cancel()
		}()
		return heldDeadline{Context: inner, parent: parent}, cancel
	})

	result, err := client.Drain(ctx, "cp-1", kube.DrainOptions{
		RetryBlocked: 10 * time.Millisecond, Timeout: 200 * time.Millisecond,
	})
	returned := time.Now()

	if asked != 200*time.Millisecond {
		t.Errorf("the drain's deadline was derived from %s, want its Timeout of 200ms", asked)
	}
	if !errors.Is(err, kube.ErrDrainBlocked) {
		t.Fatalf("Drain = %v, want ErrDrainBlocked", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Drain = %v: a drain that ran out of time says blocked, not that a context expired", err)
	}
	select {
	case at := <-expiredAt:
		if took := returned.Sub(at); took > 5*time.Second {
			t.Errorf("the drain went on for %s after its deadline had passed", took)
		}
	default:
		t.Error("the drain returned before its deadline: it did not ask again until then")
	}
	if got := sim.Evicted(); len(got) != 0 {
		t.Errorf("the server evicted %v despite the budget", got)
	}
	if got := sim.Calls(evictionOf("default", "db-0")); got < 3 {
		t.Errorf("the eviction was attempted %d times, want it asked again until the deadline", got)
	}
	if len(result.Blocked) != 1 || result.Blocked[0].Pod != "default/db-0" ||
		!contains(result.Blocked[0].Reason, "db-pdb") {
		t.Errorf("blocked = %+v, want the pod with the budget named, from the last attempt that "+
			"completed", result.Blocked)
	}
	if got := sim.Deleted(); len(got) != 0 {
		t.Errorf("the drain deleted %v: an eviction that is refused is never turned into a delete", got)
	}
}

// TestADrainNamesTheBudgetTheServerNames: the server writes the budget's name
// into the 429 itself, and the operator's next move is to look at that
// budget. The name is taken as the server wrote it.
func TestADrainNamesTheBudgetTheServerNames(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{Name: "cp-1"}},
		Pods:  []kubesim.Pod{budgetPod("cp-1")},
	})

	result, err := client.Drain(ctx, "cp-1", kube.DrainOptions{})
	if !errors.Is(err, kube.ErrDrainBlocked) {
		t.Fatalf("Drain = %v, want ErrDrainBlocked", err)
	}
	if len(result.Blocked) != 1 {
		t.Fatalf("blocked = %+v", result.Blocked)
	}
	reason := result.Blocked[0].Reason
	if !contains(reason, "db-pdb") || !contains(reason, "a PodDisruptionBudget refuses") {
		t.Errorf("the reason %q must name the budget and say a PodDisruptionBudget refuses", reason)
	}
}

// TestADrainThatWaitsForTheGoneWaitsForTerminatingPods: an accepted eviction is
// not a finished one. The pod is still on the node while its grace period runs,
// and a removal that wiped into that would wipe a node still running pods.
func TestADrainThatWaitsForTheGoneWaitsForTerminatingPods(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{Name: "cp-1"}},
		Pods: []kubesim.Pod{
			{Namespace: "default", Name: "web-1", Node: "cp-1", OwnerKind: "ReplicaSet", LingersAfterEviction: true},
		},
	})

	go finishTerminationAfter(ctx, sim, 150*time.Millisecond, "default", "web-1")

	started := time.Now()
	result, err := client.Drain(ctx, "cp-1", kube.DrainOptions{WaitGone: 10 * time.Millisecond})
	took := time.Since(started)
	if err != nil {
		t.Fatalf("Drain: %v (result %+v)", err, result)
	}
	if took < 150*time.Millisecond {
		t.Errorf("the drain returned after %s, before the pod finished terminating", took)
	}
	if got := sim.PodsOn("cp-1"); len(got) != 0 {
		t.Errorf("the node still lists %v after a drain that waited for the pods to go", got)
	}
}

// finishTerminationAfter ends a lingering pod's termination once d has passed
// and the pod is in fact terminating. It tries until it works or ctx ends: the
// test's drain may not have evicted the pod yet.
func finishTerminationAfter(ctx context.Context, sim *kubesim.Server, d time.Duration, ns, name string) {
	time.Sleep(d)
	for ctx.Err() == nil {
		if sim.FinishTermination(ns, name) == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestADrainThatDoesNotWaitReturnsWhileAPodIsStillTerminating: the option's
// zero value is the drain every caller before it had.
func TestADrainThatDoesNotWaitReturnsWhileAPodIsStillTerminating(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{Name: "cp-1"}},
		Pods: []kubesim.Pod{
			{Namespace: "default", Name: "web-1", Node: "cp-1", OwnerKind: "ReplicaSet", LingersAfterEviction: true},
		},
	})

	started := time.Now()
	if _, err := client.Drain(ctx, "cp-1", kube.DrainOptions{}); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if took := time.Since(started); took > 5*time.Second {
		t.Errorf("a drain that does not wait took %s", took)
	}
	if got := sim.PodsOn("cp-1"); !slices.Equal(got, []string{"default/web-1"}) {
		t.Errorf("the node lists %v, want the pod still there, terminating", got)
	}
	if !slices.Equal(sim.Evicted(), []string{"default/web-1"}) {
		t.Errorf("the server accepted evictions for %v, want the pod", sim.Evicted())
	}
}

// TestADrainThatWaitsForTheGoneEndsAtItsDeadlineNamingTheStragglers: a kubelet
// that never confirms a termination must not hold the job for ever.
func TestADrainThatWaitsForTheGoneEndsAtItsDeadlineNamingTheStragglers(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{Name: "cp-1"}},
		Pods: []kubesim.Pod{
			{Namespace: "default", Name: "web-1", Node: "cp-1", OwnerKind: "ReplicaSet", LingersAfterEviction: true},
		},
	})

	started := time.Now()
	result, err := client.Drain(ctx, "cp-1", kube.DrainOptions{
		WaitGone: 10 * time.Millisecond, Timeout: 300 * time.Millisecond,
	})
	if !errors.Is(err, kube.ErrDrainBlocked) {
		t.Fatalf("Drain = %v, want ErrDrainBlocked", err)
	}
	if took := time.Since(started); took > 5*time.Second {
		t.Errorf("the drain took %s with a 300ms deadline", took)
	}
	if len(result.Blocked) != 1 || result.Blocked[0].Pod != "default/web-1" ||
		!contains(result.Blocked[0].Reason, "still shutting down") {
		t.Errorf("blocked = %+v, want the pod named as evicted and still shutting down", result.Blocked)
	}
}

// TestAPermanentBlockerIsNotWaitedOut: a pod nothing owns is not coming back
// and no amount of waiting changes that, so a drain allowed to retry still
// stops at once.
func TestAPermanentBlockerIsNotWaitedOut(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{Name: "cp-1"}},
		Pods:  []kubesim.Pod{{Namespace: "default", Name: "scratch", Node: "cp-1"}},
	})

	started := time.Now()
	result, err := client.Drain(ctx, "cp-1", kube.DrainOptions{
		RetryBlocked: 5 * time.Second, Timeout: 20 * time.Second,
	})
	if !errors.Is(err, kube.ErrDrainBlocked) {
		t.Fatalf("Drain = %v, want ErrDrainBlocked", err)
	}
	if took := time.Since(started); took > 2*time.Second {
		t.Errorf("the drain waited %s for a pod no waiting can move", took)
	}
	if len(result.Blocked) != 1 {
		t.Errorf("blocked = %+v", result.Blocked)
	}
}

// TestEvictingAPodThatIsAlreadyTerminatingIsAcceptedAgain pins the simulator's
// own behaviour, which the removal's tests lean on: a second eviction of a pod
// that is shutting down is accepted without asking the budget and is not a
// second eviction.
func TestEvictingAPodThatIsAlreadyTerminatingIsAcceptedAgain(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{Name: "cp-1"}},
		Pods: []kubesim.Pod{
			{Namespace: "default", Name: "web-1", Node: "cp-1", OwnerKind: "ReplicaSet", LingersAfterEviction: true},
		},
	})

	for range 2 {
		if _, err := client.Drain(ctx, "cp-1", kube.DrainOptions{}); err != nil {
			t.Fatalf("Drain: %v", err)
		}
	}
	if got := sim.Evicted(); !slices.Equal(got, []string{"default/web-1"}) {
		t.Errorf("the server recorded evictions %v, want the one", got)
	}
	if err := sim.SetBudgetRefuses("default", "no-such-pod", true); err == nil {
		t.Error("SetBudgetRefuses accepted a pod that does not exist")
	}
}

// TestADrainCountsThePodsWhoseLocalDataWentWithThem: with the flag, a pod with
// an emptyDir is evicted and the result says that its contents went with it --
// a drain that moved it quietly would be the thing DeleteLocalData exists to
// make a decision. Without the flag the same pod still blocks.
func TestADrainCountsThePodsWhoseLocalDataWentWithThem(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{Name: "w-1"}},
		Pods: []kubesim.Pod{
			{Namespace: "default", Name: "cache-0", Node: "w-1", OwnerKind: "ReplicaSet", LocalData: true},
			{Namespace: "default", Name: "web-1", Node: "w-1", OwnerKind: "ReplicaSet"},
		},
	})

	result, err := client.Drain(ctx, "w-1", kube.DrainOptions{DeleteLocalData: true})
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if !slices.Equal(result.LocalData, []string{"default/cache-0"}) {
		t.Errorf("LocalData = %v, want only the pod that had an emptyDir", result.LocalData)
	}
	if len(result.Evicted) != 2 {
		t.Errorf("evicted = %v, want both pods", result.Evicted)
	}

	_, other := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{Name: "w-1"}},
		Pods: []kubesim.Pod{
			{Namespace: "default", Name: "cache-0", Node: "w-1", OwnerKind: "ReplicaSet", LocalData: true},
		},
	})
	without, err := other.Drain(ctx, "w-1", kube.DrainOptions{})
	if !errors.Is(err, kube.ErrDrainBlocked) {
		t.Fatalf("a drain without DeleteLocalData returned %v, want ErrDrainBlocked", err)
	}
	if len(without.LocalData) != 0 {
		t.Errorf("LocalData = %v for a pod that was not evicted", without.LocalData)
	}
}

// heldDeadline is a drain's deadline context that runs out when the test
// cancels it: Err answers as an expired deadline does, unless it was the
// caller's context that ended.
type heldDeadline struct {
	context.Context
	parent context.Context
}

func (h heldDeadline) Err() error {
	err := h.Context.Err()
	if err != nil && h.parent.Err() == nil {
		return context.DeadlineExceeded
	}
	return err
}
