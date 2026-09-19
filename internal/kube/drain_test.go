package kube_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

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
