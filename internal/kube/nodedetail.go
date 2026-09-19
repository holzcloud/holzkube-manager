package kube

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

// A node, as Kubernetes sees it (2026-09-19).
//
// The node list already says Ready and cordoned. What it cannot say is WHY a pod
// will not go there, and that is the question that brings somebody to a node:
//
//   - A taint keeps pods off unless they tolerate it. A control-plane node has
//     one by default, and a node with a disk problem gets one added by the
//     kubelet. From the pod's side this shows as "0/2 nodes are available",
//     which names no node at all.
//   - A condition says the kubelet's own verdict: DiskPressure, MemoryPressure,
//     PIDPressure, NetworkUnavailable. Ready is the one everybody looks at and
//     the other four are the ones that explain it.
//   - Allocatable minus what is already requested is how much room is left for
//     the scheduler. It is NOT how much is being used -- a node at 5% CPU can
//     still be full, because the scheduler reserves what pods asked for rather
//     than what they consume. That distinction is the single most misread
//     number in Kubernetes and it is written on the screen rather than implied.

// NodeDetail is one node in the fields that answer "why will nothing schedule
// here".
type NodeDetail struct {
	Name string `json:"name"`

	// Conditions other than Ready, each with the kubelet's own reason. Ready is
	// on the node list already.
	Conditions []NodeCondition `json:"conditions"`

	// Taints, with what each one does, in words.
	Taints []NodeTaint `json:"taints"`

	// What the scheduler has to work with. Requested is what the pods on this
	// node ASKED for, not what they use.
	CPUAllocatable    string `json:"cpu_allocatable"`
	CPURequested      string `json:"cpu_requested"`
	MemoryAllocatable string `json:"memory_allocatable"`
	MemoryRequested   string `json:"memory_requested"`

	// PodsRunning and PodCapacity: a node can be full of pods while its CPU is
	// idle, and the limit is a real one.
	PodsRunning int   `json:"pods_running"`
	PodCapacity int64 `json:"pod_capacity"`

	// Notice is the sentence about requests versus usage. It is carried rather
	// than left to the screen because it is the misreading this whole type
	// exists to prevent.
	Notice string `json:"notice"`

	Labels map[string]string `json:"labels"`
}

// NodeCondition is one of the kubelet's verdicts.
type NodeCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`

	// Bad is true when this condition is the one stopping work. For Ready that
	// is status != True; for the pressures it is status == True, which is the
	// inversion that makes a naive screen show a healthy node in red.
	Bad bool `json:"bad"`
}

// NodeTaint is one taint and what it does.
type NodeTaint struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Effect string `json:"effect"`

	// Explains the effect in a sentence, because NoSchedule and NoExecute are
	// different days: one stops new pods, the other evicts the ones already
	// there.
	Explanation string `json:"explanation"`
}

// NodeDetail reads one node and the pods on it.
func (c *Client) NodeDetail(ctx context.Context, name string) (NodeDetail, error) {
	node, err := c.cs.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return NodeDetail{}, fmt.Errorf("%w: node %s", ErrNoSuchWorkload, name)
		}
		return NodeDetail{}, fmt.Errorf("kube: reading node %s: %w", name, err)
	}

	// The pods on this node, asked for by the SERVER with a field selector: a
	// cluster with ten thousand pods must not send all of them so that one
	// node's can be added up.
	pods, err := c.cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", name).String(),
	})
	if err != nil {
		return NodeDetail{}, fmt.Errorf("kube: listing the pods on %s: %w", name, err)
	}

	cpu := resource.NewQuantity(0, resource.DecimalSI)
	memory := resource.NewQuantity(0, resource.BinarySI)
	running := 0
	for _, pod := range pods.Items {
		// Only the pods the scheduler still counts. A Succeeded or Failed pod
		// holds no reservation, and counting them would report a node as full
		// because a CronJob ran a hundred times.
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		running++
		for _, container := range pod.Spec.Containers {
			if q, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
				cpu.Add(q)
			}
			if q, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
				memory.Add(q)
			}
		}
	}

	out := NodeDetail{
		Name:            node.Name,
		Labels:          node.Labels,
		CPURequested:    cpu.String(),
		MemoryRequested: memory.String(),
		PodsRunning:     running,
		Notice: "Requested is what the pods here asked for, which is what the scheduler " +
			"reserves. It is not what they use: a node at 5% CPU can still have no room left.",
	}
	if q, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
		out.CPUAllocatable = q.String()
	}
	if q, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
		out.MemoryAllocatable = q.String()
	}
	if q, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
		out.PodCapacity = q.Value()
	}

	for _, condition := range node.Status.Conditions {
		// Ready is on the node list already, and repeating it here would put
		// the same fact in two places that could disagree.
		if condition.Type == corev1.NodeReady {
			continue
		}
		out.Conditions = append(out.Conditions, NodeCondition{
			Type:    string(condition.Type),
			Status:  string(condition.Status),
			Reason:  condition.Reason,
			Message: condition.Message,
			// A pressure condition is bad when it is TRUE, which is the
			// inversion of Ready -- and a screen that treated every condition
			// the same would paint a healthy node red.
			Bad: condition.Status == corev1.ConditionTrue,
		})
	}

	for _, taint := range node.Spec.Taints {
		out.Taints = append(out.Taints, NodeTaint{
			Key:         taint.Key,
			Value:       taint.Value,
			Effect:      string(taint.Effect),
			Explanation: explainTaint(taint),
		})
	}

	return out, nil
}

// explainTaint says what a taint does, in words.
//
// NoSchedule and NoExecute are different days of work: one stops new pods
// arriving, the other removes the ones already there. A screen showing the
// effect string alone leaves that to whoever remembers.
func explainTaint(taint corev1.Taint) string {
	subject := taint.Key
	if taint.Value != "" {
		subject += "=" + taint.Value
	}
	switch taint.Effect {
	case corev1.TaintEffectNoSchedule:
		return fmt.Sprintf("nothing new is scheduled here unless it tolerates %s", subject)
	case corev1.TaintEffectPreferNoSchedule:
		return fmt.Sprintf("the scheduler avoids this node unless it has to, because of %s", subject)
	case corev1.TaintEffectNoExecute:
		return fmt.Sprintf("pods that do not tolerate %s are EVICTED from this node, "+
			"not merely kept off it", subject)
	default:
		return ""
	}
}

// TaintsThatExplainPending picks the taints a pending pod would trip over.
//
// The control-plane taint is the usual answer and the least interesting one, so
// it is named as ordinary rather than presented as a finding: every Talos
// control-plane node has it, and somebody reading a node for the first time
// should not be told their cluster is misconfigured.
func TaintsThatExplainPending(taints []NodeTaint) []NodeTaint {
	out := make([]NodeTaint, 0, len(taints))
	for _, taint := range taints {
		if strings.HasPrefix(taint.Key, "node-role.kubernetes.io/") {
			continue
		}
		if taint.Effect == string(corev1.TaintEffectPreferNoSchedule) {
			continue
		}
		out = append(out, taint)
	}
	return out
}
