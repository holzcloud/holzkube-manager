package kube

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
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
// # What a drain does and does not decide for its caller
//
// A drain without RetryBlocked and WaitGone is one pass. It does not wait for a
// pod to be rescheduled somewhere else, nor for an evicted pod to finish
// shutting down: its promise is that the node was asked to give its pods up,
// not that the cluster absorbed the load. A single-node cluster drains
// successfully and runs nothing afterwards, and a drain that blocked on
// rescheduling would hang for ever there instead of saying what it did. And it
// never decides for the owner of a PodDisruptionBudget: a refusal is reported
// the moment it comes.
//
// The node-removal job sets both options on purpose. It is about to take the
// node out of etcd and wipe it, so it waits -- up to the drain's deadline -- for
// a budget to open and for the evicted pods to be gone. What it does not do,
// ever, is treat a veto as something it can overcome by deleting: after the
// deadline the drain ends blocked, with the budget named.

// ErrDrainBlocked reports pods a drain cannot move.
//
// It is one error with a list rather than one per pod, because the operator's
// next decision is about the node and not about the third pod: either they
// force it, or they fix the budget, or they leave the node alone.
var ErrDrainBlocked = errors.New("kube: pods on this node cannot be evicted")

// DrainRetry is how long a drain that may retry waits before it asks again for
// a pod a PodDisruptionBudget refused. Five seconds is the pause Talos' own
// drain loop takes between attempts, so a removal that drains first is no more
// insistent on the API server than the reset that follows it.
const DrainRetry = 5 * time.Second

// DrainGonePoll is how often a drain that waits looks again at pods it has
// evicted and that are still shutting down. A look is one list call filtered
// to the node, and a grace period is counted in seconds, so two seconds is
// quick enough to notice and slow enough not to matter to the API server.
const DrainGonePoll = 2 * time.Second

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

	// RetryBlocked is the pause between passes that failed only because a
	// PodDisruptionBudget refused a pod. Zero is one pass and no retry, which
	// is what a drain was before this existed. A pod no waiting can move -- one
	// nothing owns, one with local data it may not lose -- ends the drain at
	// once whatever this says. Ends with Timeout, with ErrDrainBlocked and the
	// last state, not with the context's error.
	RetryBlocked time.Duration

	// WaitGone is how often to look again, after the last eviction, until the
	// evicted pods have left the node. Zero returns as soon as the server has
	// accepted the evictions. Ends with Timeout, with the pods still there named
	// in Blocked.
	WaitGone time.Duration
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

// Summary is the sentence a job screen shows for a drain: what moved, what was
// left on purpose, and each pod that stopped it with the reason it gave.
func (r DrainResult) Summary() string {
	parts := []string{strconv.Itoa(len(r.Evicted)) + " evicted"}
	if n := len(r.SkippedDaemonSet); n > 0 {
		parts = append(parts, strconv.Itoa(n)+" DaemonSet pods left in place")
	}
	if n := len(r.SkippedMirror); n > 0 {
		parts = append(parts, strconv.Itoa(n)+" static pods left in place")
	}
	for _, b := range r.Blocked {
		parts = append(parts, b.Pod+": "+b.Reason)
	}
	return strings.Join(parts, "; ")
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
	// parent is the caller's context. Timeout ends a child of it, and the two
	// ending are different things: a drain that ran out of time is blocked, a
	// caller that went away is cancelled.
	parent := ctx
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	if err := c.Cordon(ctx, node, true); err != nil {
		return DrainResult{}, err
	}

	waits := opts.RetryBlocked > 0 || opts.WaitGone > 0

	var result DrainResult
	var lastBlocked []BlockedPod // from the last pass that ran to the end
	for {
		pass, err := c.drainPass(ctx, node, opts)
		if err != nil {
			if waits && parent.Err() != nil {
				return result, parent.Err()
			}
			if expired(ctx) && lastBlocked != nil {
				// The deadline came while the pass was asking the server. Out
				// of time is a verdict, not a failure: the last complete pass
				// is what is reported.
				result.Blocked = lastBlocked
				return result, blockedError(result, len(lastBlocked))
			}
			return result, err
		}
		result.Evicted = unionOf(result.Evicted, pass.result.Evicted)
		result.SkippedDaemonSet = pass.result.SkippedDaemonSet
		result.SkippedMirror = pass.result.SkippedMirror
		result.Blocked = pass.result.Blocked

		if waits && parent.Err() != nil {
			return result, parent.Err()
		}
		if expired(ctx) && lastBlocked != nil {
			// The deadline came in the middle of a pass. That pass is cut short
			// and says so with errors about a context; the state worth
			// reporting is the last one that was complete.
			result.Blocked = lastBlocked
			return result, blockedError(result, pass.total)
		}
		if len(result.Blocked) == 0 {
			break
		}
		if opts.RetryBlocked <= 0 || pass.vetoes != len(result.Blocked) {
			return result, blockedError(result, pass.total)
		}

		lastBlocked = slices.Clone(result.Blocked)
		if !pause(ctx, opts.RetryBlocked) {
			if parent.Err() != nil {
				return result, parent.Err()
			}
			return result, blockedError(result, pass.total)
		}
	}

	if opts.WaitGone > 0 {
		return c.waitGone(ctx, parent, node, opts, result)
	}
	return result, nil
}

// blockedError is the verdict of a drain that left pods behind.
func blockedError(result DrainResult, total int) error {
	return fmt.Errorf("%w: %d of %d pods stayed. The node is cordoned, so nothing new "+
		"is scheduled onto it, and what is left is listed with the reason each one gave",
		ErrDrainBlocked, len(result.Blocked), total)
}

// expired reports whether the context's deadline has passed, whether or not
// its Done channel has been closed yet. The difference is not academic: the
// client's rate limiter refuses a call that cannot start before the deadline
// with "would exceed context deadline" in the instant after the deadline and
// before the context notices, and a drain that asked ctx.Err() would take that
// for a failure of the server instead of for running out of time.
func expired(ctx context.Context) bool {
	if ctx.Err() != nil {
		return true
	}
	deadline, ok := ctx.Deadline()
	return ok && !time.Now().Before(deadline)
}

// pause waits d, or until ctx ends. It reports whether the time passed.
func pause(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// unionOf appends what is not yet there, in order.
func unionOf(have, more []string) []string {
	for _, m := range more {
		if !slices.Contains(have, m) {
			have = append(have, m)
		}
	}
	return have
}

// waitGone returns once the pods this drain evicted have left the node, or
// ends blocked at the deadline naming the ones that have not.
//
// An eviction the server accepted is not a pod that is gone: it is on the node
// until its grace period is over. Asking Undrained is asking the question the
// drain was asked, so the wait and the check that follows a restart cannot
// disagree.
func (c *Client) waitGone(
	ctx, parent context.Context,
	node string,
	opts DrainOptions,
	result DrainResult,
) (DrainResult, error) {
	var left []string
	for {
		got, err := c.Undrained(ctx, node, opts)
		switch {
		case err == nil:
			left = got
		case parent.Err() != nil:
			return result, parent.Err()
		case expired(ctx):
			// Out of time in the middle of a look: the last complete look
			// stands. With none, the pods this drain evicted are the ones it
			// could not see leave.
			if left == nil {
				left = result.Evicted
			}
		default:
			return result, fmt.Errorf("kube: waiting for the pods on %s to leave: %w", node, err)
		}
		if err == nil && len(left) == 0 {
			return result, nil
		}
		if err == nil && pause(ctx, opts.WaitGone) {
			continue
		}
		if parent.Err() != nil {
			return result, parent.Err()
		}

		for _, name := range left {
			result.Blocked = append(result.Blocked, BlockedPod{
				Pod: name,
				Reason: "still on the node when the drain's time ran out; it was evicted and " +
					"is still shutting down",
			})
		}
		return result, fmt.Errorf("%w: %d pods were evicted and are still shutting down. The node is "+
			"cordoned, so nothing new is scheduled onto it, and the pods still there are listed",
			ErrDrainBlocked, len(left))
	}
}

// drainPassResult is one pass over a node's pods.
type drainPassResult struct {
	result DrainResult

	// total is the pods the node had, vetoes the blocked ones that were a
	// PodDisruptionBudget's refusal -- the only kind waiting can change.
	total, vetoes int
}

// drainPass lists a node's pods once and evicts what a drain moves.
func (c *Client) drainPass(ctx context.Context, node string, opts DrainOptions) (drainPassResult, error) {
	pods, err := c.cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + node,
	})
	if err != nil {
		return drainPassResult{}, fmt.Errorf("kube: listing the pods on %s: %w", node, err)
	}

	pass := drainPassResult{total: len(pods.Items)}
	result := &pass.result
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
			var veto *vetoError
			if errors.As(err, &veto) {
				pass.vetoes++
			}
			result.Blocked = append(result.Blocked, BlockedPod{Pod: name, Reason: err.Error()})
			continue
		}
		result.Evicted = append(result.Evicted, name)
	}
	return pass, nil
}

// vetoError is an eviction a PodDisruptionBudget refused. It is a type of its
// own because the drain loop treats it differently from every other failure:
// it is the one refusal that waiting can change.
type vetoError struct {
	reason string
	err    error
}

func (e *vetoError) Error() string { return e.reason }
func (e *vetoError) Unwrap() error { return e.err }

// vetoReason is the one sentence for a pod a budget refuses to let go, and
// whether err is such a refusal at all.
//
// A 429 is taken as a veto even without a cause, because the server answers
// 429 for a budget it has not finished counting, too, and then waiting is
// right. Only when the server names the budget -- a cause of the type
// policy/v1 defines -- is the budget in the sentence, and its message is taken
// as the server wrote it: the name is in it, and parsing it would break at the
// first rewording.
func vetoReason(err error) (reason string, isVeto bool) {
	if !apierrors.IsTooManyRequests(err) {
		return "", false
	}
	reason = "a PodDisruptionBudget refuses to let this pod go right now: " + err.Error()

	var status apierrors.APIStatus
	if errors.As(err, &status) {
		if details := status.Status().Details; details != nil {
			for _, cause := range details.Causes {
				if cause.Type == policyv1.DisruptionBudgetCause {
					reason += " (" + cause.Message + ")"
					break
				}
			}
		}
	}
	return reason, true
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
	if err == nil {
		return nil
	}
	if reason, ok := vetoReason(err); ok {
		// 429 from an eviction is a PodDisruptionBudget refusing, and it is the
		// one refusal that is never overridden: the budget is somebody's
		// statement about their own availability. A drain that was told to
		// retry asks again, up to its deadline; it never turns a refusal into a
		// delete.
		return &vetoError{reason: reason, err: err}
	}
	if apierrors.IsNotFound(err) {
		// Gone while this was running, which is the outcome the eviction was
		// asking for.
		return nil
	}
	return err
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
