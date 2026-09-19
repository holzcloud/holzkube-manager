package kube_test

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// A node, as Kubernetes sees it (2026-09-19).
//
// The node list says Ready and cordoned. These are about the fields that say WHY
// nothing will schedule there, which is the question that brings somebody to a
// node in the first place.

// TestAPressureConditionIsBadWhenItIsTrue.
//
// The inversion is the whole test. Ready is bad when it is False; DiskPressure
// is bad when it is TRUE, and a screen that treated every condition the same
// would paint a healthy node red and a full disk green.
func TestAPressureConditionIsBadWhenItIsTrue(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Nodes: []kubesim.Node{{
		Name:  "cp-1",
		Ready: corev1.ConditionTrue,
		Conditions: []string{
			"DiskPressure=True,KubeletHasDiskPressure",
			"MemoryPressure=False,KubeletHasSufficientMemory",
		},
	}}})

	detail, err := client.NodeDetail(ctx, "cp-1")
	if err != nil {
		t.Fatalf("NodeDetail: %v", err)
	}

	byType := map[string]kube.NodeCondition{}
	for _, c := range detail.Conditions {
		byType[c.Type] = c
	}

	if !byType["DiskPressure"].Bad {
		t.Error("DiskPressure=True was not marked bad; that is the condition that stops work")
	}
	if byType["MemoryPressure"].Bad {
		t.Error("MemoryPressure=False was marked bad; that node has enough memory")
	}
	// Ready is on the node list already, and carrying it here too would put one
	// fact in two places that can disagree.
	if _, ok := byType["Ready"]; ok {
		t.Error("Ready is repeated in the detail; it belongs to the list")
	}
	if byType["DiskPressure"].Reason != "KubeletHasDiskPressure" {
		t.Errorf("reason = %q, want the kubelet's own", byType["DiskPressure"].Reason)
	}
}

// TestNoExecuteIsSaidToBeDifferentFromNoSchedule.
//
// One stops new pods arriving; the other removes the ones already there. The
// effect string alone leaves that to whoever remembers, and they are different
// days of work.
func TestNoExecuteIsSaidToBeDifferentFromNoSchedule(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Nodes: []kubesim.Node{{
		Name: "worker-2",
		Taints: []string{
			"node.kubernetes.io/disk-pressure:NoSchedule",
			"maintenance=true:NoExecute",
		},
	}}})

	detail, err := client.NodeDetail(ctx, "worker-2")
	if err != nil {
		t.Fatalf("NodeDetail: %v", err)
	}
	if len(detail.Taints) != 2 {
		t.Fatalf("taints = %+v", detail.Taints)
	}

	byKey := map[string]kube.NodeTaint{}
	for _, taint := range detail.Taints {
		byKey[taint.Key] = taint
	}

	if !strings.Contains(byKey["node.kubernetes.io/disk-pressure"].Explanation, "nothing new") {
		t.Errorf("NoSchedule explanation = %q", byKey["node.kubernetes.io/disk-pressure"].Explanation)
	}
	if !strings.Contains(byKey["maintenance"].Explanation, "EVICTED") {
		t.Errorf("NoExecute explanation = %q, want it to say pods are removed rather than kept off",
			byKey["maintenance"].Explanation)
	}
}

// TestRoomLeftIsWhatWasREQUESTED, and the answer says so.
//
// The most misread number in Kubernetes: the scheduler reserves what pods asked
// for, not what they consume, so a node at 5% CPU can have no room. Finished
// pods hold no reservation and are not counted -- otherwise a CronJob that ran a
// hundred times would report a node as full.
func TestRoomLeftIsWhatWasRequested(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{
			Name: "cp-1", Ready: corev1.ConditionTrue,
			CPUAllocatable: "4", MemoryAllocatable: "8Gi", PodCapacity: "110",
		}},
		Pods: []kubesim.Pod{
			{Namespace: "default", Name: "running-1", Node: "cp-1", Phase: corev1.PodRunning},
			{Namespace: "default", Name: "running-2", Node: "cp-1", Phase: corev1.PodRunning},
			// A CronJob that already finished. It reserves nothing.
			{Namespace: "default", Name: "backup-29271", Node: "cp-1", Phase: corev1.PodSucceeded},
			// Somebody else's node.
			{Namespace: "default", Name: "elsewhere", Node: "worker-2", Phase: corev1.PodRunning},
		},
	})

	detail, err := client.NodeDetail(ctx, "cp-1")
	if err != nil {
		t.Fatalf("NodeDetail: %v", err)
	}

	if detail.CPUAllocatable != "4" || detail.MemoryAllocatable != "8Gi" {
		t.Errorf("allocatable = %s CPU, %s memory", detail.CPUAllocatable, detail.MemoryAllocatable)
	}
	if detail.PodCapacity != 110 {
		t.Errorf("pod capacity = %d", detail.PodCapacity)
	}
	// Two running, not three: the finished one holds nothing. And not four: the
	// other node's pod is another node's business, filtered by the SERVER.
	if detail.PodsRunning != 2 {
		t.Errorf("pods running = %d, want the two that still hold a reservation", detail.PodsRunning)
	}

	// The sentence, because this is the misreading the type exists to prevent.
	if !strings.Contains(detail.Notice, "not what they use") {
		t.Errorf("notice = %q, want it to separate requested from used", detail.Notice)
	}
}

// TestTheControlPlaneTaintIsNotPresentedAsAFinding: every Talos control-plane
// node has it, and somebody reading a node for the first time should not be told
// their cluster is misconfigured.
func TestTheControlPlaneTaintIsNotPresentedAsAFinding(t *testing.T) {
	t.Parallel()

	taints := []kube.NodeTaint{
		{Key: "node-role.kubernetes.io/control-plane", Effect: "NoSchedule"},
		{Key: "maintenance", Effect: "NoExecute"},
		{Key: "spot", Effect: "PreferNoSchedule"},
	}

	got := kube.TaintsThatExplainPending(taints)
	if len(got) != 1 || got[0].Key != "maintenance" {
		t.Errorf("explaining taints = %+v, want only the one that is not ordinary", got)
	}
}
