package kube

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Cordoning and draining a node (milestone v1.17, slice 3).
//
// This is the first thing in this product that CHANGES something through the
// Kubernetes API, and it closes a gap the product has been papering over with a
// sentence: removing a node from a cluster and upgrading one both tell the
// operator to cordon and drain by hand first.
//
// # Why client-go was chosen for exactly this
//
// A drain done wrong is a drain that takes workloads down harder than
// `kubectl` would, and that is worse than no drain at all: it is a tool nobody
// can trust at the moment they need it. Three things make the difference, and
// all three are the reason the operator's choice of client-go was the right
// one:
//
//   - the EVICTION API rather than a delete. An eviction is refused when it
//     would violate a PodDisruptionBudget; a delete is not. A drain built on
//     delete is a drain that ignores every promise the cluster's owners made
//     about their own availability.
//   - DaemonSet pods are left alone. They are recreated on the same node
//     immediately, so evicting them is a loop that never finishes, which is
//     why `kubectl drain` skips them and says so.
//   - a pod with local storage is NAMED rather than silently evicted. Its
//     emptyDir goes with it, and that is data somebody may not know they are
//     losing.
//
// # What is deliberately not here
//
// Waiting for a pod to be rescheduled somewhere else. A drain's promise is that
// the node is empty, not that the cluster absorbed the load: a single-node
// cluster drains successfully and runs nothing afterwards, and a drain that
// blocked on rescheduling would hang for ever there instead of saying what it
// did.

// ErrDrainBlocked reports pods a drain cannot move.
//
// It is one error with a list rather than one per pod, because the operator's
// next decision is about the node and not about the third pod: either they
// force it, or they fix the budget, or they leave the node alone.
var ErrDrainBlocked = errors.New("kube: pods on this node cannot be evicted")

// DrainOptions are the choices a drain offers, and each one is a decision this
// product refuses to make silently.
type DrainOptions struct {
	// Force evicts pods no controller owns.
	//
	// A bare pod is not coming back: nothing will recreate it. kubectl refuses
	// without --force for that reason, and so does this.
	Force bool

	// DeleteLocalData evicts pods with an emptyDir volume.
	//
	// The volume goes with the pod. Refusing by default is the only honest
	// choice: the alternative is a tool that loses data somebody did not know
	// was on that node.
	DeleteLocalData bool

	// GracePeriod overrides each pod's own terminationGracePeriodSeconds. Zero
	// means the pod's own value, which is what an operator means by "drain it"
	// unless they say otherwise.
	GracePeriod time.Duration

	// Timeout bounds the whole drain. A drain that runs until the request
	// context dies leaves the operator with a screen that says nothing about
	// how far it got.
	Timeout time.Duration
}

// DrainResult is what a drain did, in the words the screen needs.
type DrainResult struct {
	// Evicted are the pods this drain moved off the node.
	Evicted []string `json:"evicted"`

	// SkippedDaemonSet are the pods left alone because a DaemonSet owns them:
	// evicting one is a loop, since it is recreated on the same node.
	SkippedDaemonSet []string `json:"skipped_daemonset"`

	// SkippedMirror are static pods -- the control plane's own, on Talos --
	// which the API server cannot evict because the kubelet owns them.
	SkippedMirror []string `json:"skipped_mirror"`

	// Blocked are the pods that stopped the drain, with the reason each one
	// gave. A drain with anything here did not finish.
	Blocked []BlockedPod `json:"blocked"`
}

// BlockedPod is one pod a drain could not move, and why.
type BlockedPod struct {
	Pod    string `json:"pod"`
	Reason string `json:"reason"`
}

// Cordon marks a node unschedulable, or lets it schedule again.
//
// A patch and not a read-modify-write: the field is one boolean and a patch
// cannot lose a concurrent change to the rest of the object. `unschedulable`
// is also exactly what the screen reads back, so the effect is visible in the
// next list rather than being a claim.
func (c *Client) Cordon(ctx context.Context, node string, unschedulable bool) error {
	patch := fmt.Sprintf(`{"spec":{"unschedulable":%t}}`, unschedulable)

	_, err := c.cs.CoreV1().Nodes().Patch(
		ctx, node, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	if err != nil {
		verb := "cordoning"
		if !unschedulable {
			verb = "uncordoning"
		}
		return fmt.Errorf("kube: %s %s: %w", verb, node, err)
	}
	return nil
}

// Drain evicts a node's pods, after cordoning it.
//
// The cordon comes first and is not optional: draining a node that still
// accepts work is a race against the scheduler, and losing it means evicting a
// pod that was placed while the drain was running.
func (c *Client) Drain(ctx context.Context, node string, opts DrainOptions) (DrainResult, error) {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	if err := c.Cordon(ctx, node, true); err != nil {
		return DrainResult{}, err
	}

	pods, err := c.cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + node,
	})
	if err != nil {
		return DrainResult{}, fmt.Errorf("kube: listing the pods on %s: %w", node, err)
	}

	var result DrainResult
	for _, pod := range pods.Items {
		name := pod.Namespace + "/" + pod.Name

		switch kind := classifyPod(pod, opts); kind {
		case podDaemonSet:
			result.SkippedDaemonSet = append(result.SkippedDaemonSet, name)
			continue
		case podMirror:
			result.SkippedMirror = append(result.SkippedMirror, name)
			continue
		case podFinished:
			// Succeeded or Failed: it is not running, so there is nothing to
			// move, and evicting it would only change a record.
			continue
		case podBare:
			result.Blocked = append(result.Blocked, BlockedPod{
				Pod: name,
				Reason: "no controller owns this pod, so nothing will recreate it elsewhere. " +
					"Evicting it means losing it",
			})
			continue
		case podLocalData:
			result.Blocked = append(result.Blocked, BlockedPod{
				Pod: name,
				Reason: "this pod has local storage (emptyDir), and its contents go with it when " +
					"it moves",
			})
			continue
		case podEvictable:
		}

		if err := c.evict(ctx, pod, opts); err != nil {
			result.Blocked = append(result.Blocked, BlockedPod{Pod: name, Reason: err.Error()})
			continue
		}
		result.Evicted = append(result.Evicted, name)
	}

	if len(result.Blocked) > 0 {
		return result, fmt.Errorf("%w: %d of %d pods stayed. The node is cordoned, so nothing new "+
			"is scheduled onto it, and what is left is listed with the reason each one gave",
			ErrDrainBlocked, len(result.Blocked), len(pods.Items))
	}
	return result, nil
}

// evict asks the API server to evict one pod, which is the call that honours a
// PodDisruptionBudget.
func (c *Client) evict(ctx context.Context, pod corev1.Pod, opts DrainOptions) error {
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{Name: pod.Name, Namespace: pod.Namespace},
	}
	if opts.GracePeriod > 0 {
		seconds := int64(opts.GracePeriod.Seconds())
		eviction.DeleteOptions = &metav1.DeleteOptions{GracePeriodSeconds: &seconds}
	}

	err := c.cs.PolicyV1().Evictions(pod.Namespace).Evict(ctx, eviction)
	switch {
	case err == nil:
		return nil
	case apierrors.IsTooManyRequests(err):
		// 429 from an eviction is a PodDisruptionBudget refusing, and it is the
		// one refusal that must not be retried into submission here: the budget
		// is somebody's statement about their own availability, and a drain
		// that waited it out would be making that decision for them.
		return fmt.Errorf("a PodDisruptionBudget refuses to let this pod go right now: %w", err)
	case apierrors.IsNotFound(err):
		// Gone while this was running, which is the outcome the eviction was
		// asking for.
		return nil
	default:
		return err
	}
}

// podKind is how a drain has to treat one pod.
type podKind int

const (
	podEvictable podKind = iota
	podDaemonSet
	podMirror
	podFinished
	podBare
	podLocalData
)

// classifyPod decides what a drain does with one pod, in the order kubectl
// decides it: the pods that are not a decision first.
func classifyPod(pod corev1.Pod, opts DrainOptions) podKind {
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return podFinished
	}
	if _, mirror := pod.Annotations[corev1.MirrorPodAnnotationKey]; mirror {
		return podMirror
	}

	owner := metav1.GetControllerOf(&pod)
	switch {
	case owner == nil && !opts.Force:
		return podBare
	case owner != nil && owner.Kind == "DaemonSet":
		return podDaemonSet
	}

	if !opts.DeleteLocalData && hasLocalData(pod) {
		return podLocalData
	}
	return podEvictable
}

// hasLocalData reports an emptyDir volume, which is the storage that does not
// survive the pod moving.
func hasLocalData(pod corev1.Pod) bool {
	for _, v := range pod.Spec.Volumes {
		if v.EmptyDir != nil {
			return true
		}
	}
	return false
}

// PodsOnNode is the pods the API server places on one node.
//
// It exists for the drain's own verification step: "is there anything left here
// that this drain would move" has to be answerable without moving anything, or
// an interrupted drain has to park for a human instead of resuming.
//
// The filter is a field selector, so the API server does the filtering. A
// client that listed every pod in the cluster and filtered locally would be
// asking a busy cluster to send its whole inventory to answer a question about
// one node.
func (c *Client) PodsOnNode(ctx context.Context, node string) ([]NodePod, error) {
	list, err := c.cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + node,
	})
	if err != nil {
		return nil, fmt.Errorf("kube: listing the pods on %s: %w", node, err)
	}

	out := make([]NodePod, 0, len(list.Items))
	for _, pod := range list.Items {
		out = append(out, NodePod{pod: pod})
	}
	return out, nil
}

// NodePod is one pod on a node, carried as the API server sent it so that the
// drain's own classification can be asked about it.
//
// It wraps rather than flattens, deliberately: the classification reads owner
// references, annotations and volumes, and a read model that dropped them would
// make the verification answer a different question from the step.
type NodePod struct {
	pod corev1.Pod
}

// Name is the pod, as the rest of this product names one.
func (p NodePod) Name() string { return p.pod.Namespace + "/" + p.pod.Name }

// wouldMove reports whether a drain with these options would try to evict this
// pod. It is the same function the drain itself uses, which is the point: a
// verification that classified differently would report a drain complete while
// the drain would still have work.
func (p NodePod) wouldMove(opts DrainOptions) bool {
	return classifyPod(p.pod, opts) == podEvictable
}
