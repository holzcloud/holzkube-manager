package kube

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Clearing out what is finished (2026-09-20).
//
// # What accumulates, and why nothing removes it
//
// A CronJob that runs nightly leaves a finished Job and its pod behind every
// night, kept by `successfulJobsHistoryLimit` until somebody notices there are
// four hundred. A rollout leaves the previous ReplicaSet at zero replicas so it
// can be rolled back to, and the one before that, and the one before that. None
// of it runs, none of it reserves anything, and all of it makes every list
// harder to read.
//
// # The plan comes first, and the counts are the plan
//
// This is a deletion, so it follows the same rule as the manifest path: it says
// what it would remove before it removes anything, and the apply is a separate
// request. Nothing here is recoverable and "it deleted more than I expected" is
// the failure to prevent.
//
// # What is deliberately NOT swept
//
//   - A pod that is Pending, Running or Unknown. Unknown especially: it means
//     the node stopped reporting, and the pod may well be running fine.
//   - The most recent ReplicaSet of a Deployment, which is what a rollback
//     would go to.
//   - Anything somebody else still owns.
//
// # Images are not here, and that is a fact about Talos rather than a decision
//
// The Talos machine API can LIST the images on a node and cannot remove one:
// there is no delete in the API. Image removal is the kubelet's own garbage
// collection, which runs when the disk crosses a threshold. So this reports what
// is on the nodes and says who removes it -- a button that claimed to delete
// images would do nothing at all.

// Sweepable is one thing that could be removed.
type Sweepable struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`

	// Why this one qualifies, in words: "finished 12 days ago", "replaced by a
	// newer rollout". A list of names with no reasons is a list nobody can
	// check before pressing the button.
	Reason string `json:"reason"`

	Age string `json:"age"`
}

// SweepPlan is what clearing out would remove.
type SweepPlan struct {
	Items []Sweepable `json:"items"`

	// Notice explains what is not in the list, so an empty plan is not read as
	// "there is nothing to clean up anywhere".
	Notice string `json:"notice"`
}

// MaxSweepItems bounds how much one sweep may remove.
//
// Not tidiness: the route issues one delete per item, so without a cap its cost
// is whatever a plan happened to contain -- and a cluster with four hundred
// leftover pods is exactly the cluster somebody presses this on. The number is
// well above an ordinary tidy-up, so the refusal means "do it in parts and look
// at each" rather than "your cluster is too messy".
const MaxSweepItems = 200

// SweepOlderThan is the default age below which finished things are left alone.
//
// An hour, because a Job that finished a minute ago is one somebody may be
// reading the logs of right now -- and those logs go with the pod.
const SweepOlderThan = time.Hour

// PlanSweep says what clearing out would remove, and removes nothing.
func (c *Client) PlanSweep(ctx context.Context, namespace string, now time.Time) (SweepPlan, error) {
	plan := SweepPlan{
		Notice: "Only things that have finished and are not the way back from a rollout. " +
			"Running, pending and unknown pods are left alone -- unknown especially, because " +
			"that means a node stopped reporting rather than that the pod stopped. Images on " +
			"the nodes are not here: Talos cannot delete one, and the kubelet removes them " +
			"itself when its disk fills.",
	}

	pods, err := c.cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return SweepPlan{}, fmt.Errorf("kube: listing pods: %w", err)
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
			continue
		}
		age := now.Sub(pod.CreationTimestamp.Time)
		if age < SweepOlderThan {
			continue
		}
		reason := "finished"
		if pod.Status.Phase == corev1.PodFailed {
			// A failed pod is kept on purpose in the plan, but named as failed:
			// it is the one whose logs somebody may still want, and the plan is
			// where they get the chance to notice.
			reason = "failed, and its log goes with it"
		}
		plan.Items = append(plan.Items, Sweepable{
			Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name,
			Reason: reason, Age: since(age),
		})
	}

	jobs, err := c.cs.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return SweepPlan{}, fmt.Errorf("kube: listing jobs: %w", err)
	}
	for _, job := range jobs.Items {
		finished := finishedAt(job)
		if finished.IsZero() {
			continue
		}
		age := now.Sub(finished)
		if age < SweepOlderThan {
			continue
		}
		// A Job a CronJob still owns is that CronJob's history, and its own
		// history limit decides when it goes. Removing it here would fight a
		// controller, which is the thing this product does not do.
		if owner := metav1.GetControllerOf(&job); owner != nil && owner.Kind == "CronJob" {
			continue
		}
		plan.Items = append(plan.Items, Sweepable{
			Kind: "Job", Namespace: job.Namespace, Name: job.Name,
			Reason: "finished, and nothing owns it", Age: since(age),
		})
	}

	sets, err := c.cs.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return SweepPlan{}, fmt.Errorf("kube: listing replicasets: %w", err)
	}
	// The newest ReplicaSet per Deployment is the way back from a rollout and
	// is kept. Everything older at zero replicas is a previous rollout nobody
	// will return to without going through the Deployment anyway.
	newest := map[string]time.Time{}
	for _, set := range sets.Items {
		owner := metav1.GetControllerOf(&set)
		if owner == nil || owner.Kind != "Deployment" {
			continue
		}
		key := set.Namespace + "/" + owner.Name
		if set.CreationTimestamp.After(newest[key]) {
			newest[key] = set.CreationTimestamp.Time
		}
	}
	for _, set := range sets.Items {
		owner := metav1.GetControllerOf(&set)
		if owner == nil || owner.Kind != "Deployment" {
			continue
		}
		if set.Status.Replicas > 0 || (set.Spec.Replicas != nil && *set.Spec.Replicas > 0) {
			continue
		}
		key := set.Namespace + "/" + owner.Name
		if !set.CreationTimestamp.Time.Before(newest[key]) {
			continue
		}
		age := now.Sub(set.CreationTimestamp.Time)
		if age < SweepOlderThan {
			continue
		}
		plan.Items = append(plan.Items, Sweepable{
			Kind: "ReplicaSet", Namespace: set.Namespace, Name: set.Name,
			Reason: "an older rollout of " + owner.Name + ", running nothing", Age: since(age),
		})
	}

	return plan, nil
}

// Sweep removes exactly what a plan listed.
//
// The caller passes the plan back, rather than this recomputing one: between a
// plan and an apply somebody's CronJob can run, and a sweep that recomputed
// would remove things nobody saw in the list they approved.
func (c *Client) Sweep(ctx context.Context, items []Sweepable) (removed int, failed []FailedObject, err error) {
	if len(items) > MaxSweepItems {
		return 0, nil, fmt.Errorf("%w: %d things at once is more than this removes in one go "+
			"(%d). Sweep in parts, and look at each list", ErrRefusedKind, len(items), MaxSweepItems)
	}
	for _, item := range items {
		var deleteErr error
		switch item.Kind {
		case "Pod":
			deleteErr = c.cs.CoreV1().Pods(item.Namespace).
				Delete(ctx, item.Name, metav1.DeleteOptions{})
		case "Job":
			// The Job's pods go with it, which is what somebody removing a
			// finished Job means.
			policy := metav1.DeletePropagationBackground
			deleteErr = c.cs.BatchV1().Jobs(item.Namespace).
				Delete(ctx, item.Name, metav1.DeleteOptions{PropagationPolicy: &policy})
		case "ReplicaSet":
			deleteErr = c.cs.AppsV1().ReplicaSets(item.Namespace).
				Delete(ctx, item.Name, metav1.DeleteOptions{})
		default:
			deleteErr = fmt.Errorf("%w: %s", ErrRefusedKind, item.Kind)
		}

		switch {
		case deleteErr == nil:
			removed++
		case isNotFoundLike(deleteErr):
			// Gone between the plan and the sweep, which is the outcome that
			// was being asked for.
			removed++
		default:
			failed = append(failed, FailedObject{
				Object: ManifestObject{
					Kind: item.Kind, Namespace: item.Namespace, Name: item.Name,
				},
				Reason: deleteErr.Error(),
			})
		}
	}
	return removed, failed, nil
}

// finishedAt is when a Job stopped, or the zero time while it is still running.
func finishedAt(job batchv1.Job) time.Time {
	if job.Status.CompletionTime != nil {
		return job.Status.CompletionTime.Time
	}
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			return condition.LastTransitionTime.Time
		}
	}
	return time.Time{}
}

// since is an age somebody can read, rather than a duration with nanoseconds.
func since(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	}
}
