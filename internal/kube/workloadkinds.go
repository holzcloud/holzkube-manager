package kube

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// The other workload kinds (2026-09-19).
//
// # Why a Deployment alone is not a cluster
//
// Until now this product listed Deployments and nothing else, which describes a
// cluster nobody runs. The storage layer is a DaemonSet, the CNI is a DaemonSet,
// the database is a StatefulSet, the backup is a CronJob. An operator whose
// Longhorn is broken looked at the Kubernetes screen and saw a workload list
// with Longhorn absent from it -- which is worse than an empty screen, because
// an empty screen does not imply the thing is not there.
//
// # One shape, four kinds, and the differences kept rather than flattened
//
// They are reported through one type, because a screen listing "what runs here"
// should not make somebody visit four tabs. But the fields that differ are the
// ones that matter, so they are carried rather than averaged into a percentage:
//
//   - A DaemonSet's "desired" is however many nodes match it, which changes when
//     a node is added. Comparing it to a Deployment's replica count is comparing
//     an intention to a consequence.
//   - A StatefulSet replaces pods in order and waits for each, so "2 of 3
//     ready" during an update is normal rather than broken.
//   - A Job's numbers are succeeded and failed, not ready: a Job with zero
//     running pods has usually finished.
//   - A CronJob has no pods at all between runs. Reporting "0 ready" for one
//     would be reporting a schedule as an outage.

// WorkloadKind is which of the four a row is.
type WorkloadKind string

// The five kinds this product knows. A cluster runs all of them.
const (
	KindDeployment  WorkloadKind = "Deployment"
	KindStatefulSet WorkloadKind = "StatefulSet"
	KindDaemonSet   WorkloadKind = "DaemonSet"
	KindJob         WorkloadKind = "Job"
	KindCronJob     WorkloadKind = "CronJob"
)

// Workload is one thing that runs, whatever kind it is.
type Workload struct {
	Kind      WorkloadKind `json:"kind"`
	Namespace string       `json:"namespace"`
	Name      string       `json:"name"`

	// Desired and Ready mean different things per kind, which is why Summary
	// exists beside them rather than instead of them.
	Desired int32 `json:"desired"`
	Ready   int32 `json:"ready"`

	// Summary is the honest one-line state, written per kind. "3/3 ready" is
	// wrong for a CronJob and misleading for a Job.
	Summary string `json:"summary"`

	Image     string `json:"image"`
	CreatedAt string `json:"created_at"`

	// Schedule is a CronJob's, empty otherwise. Suspended says a CronJob or Job
	// is switched off, which looks exactly like "never runs" and is not.
	Schedule  string `json:"schedule"`
	Suspended bool   `json:"suspended"`

	// LastRun is a CronJob's last scheduled time, empty otherwise. A CronJob
	// whose last run was in March is the finding somebody came for.
	LastRun string `json:"last_run"`

	// Scalable says whether this kind takes a replica count at all. A DaemonSet
	// does not -- its count is the cluster's shape -- and a screen offering to
	// scale one would offer an operation the API server refuses.
	Scalable bool `json:"scalable"`

	// Rollable says whether `rollout restart` applies. Deployments,
	// StatefulSets and DaemonSets have a pod template; Jobs do not.
	Rollable bool `json:"rollable"`

	// Stoppable says whether this kind has a stop at all. A DaemonSet does not:
	// its size is how many nodes match, so there is no count to set to zero.
	// The screen shows only what this says, for the same reason Scalable exists
	// -- a button the API server would refuse teaches that the buttons here are
	// suggestions.
	Stoppable bool `json:"stoppable"`

	// Stopped, and what a start would bring back.
	//
	// Read from the annotation the stop wrote, on the object this listing has
	// already fetched -- so the number is on the screen BEFORE anybody presses
	// start, without a second route to ask for it. Zero means nothing was
	// written down and a start would use one, which for a three-replica service
	// is the difference between restoring it and silently running a third of it.
	Stopped        bool  `json:"stopped"`
	WouldStartWith int32 `json:"would_start_with"`
}

// Workloads lists everything that runs, across the kinds this product knows.
//
// One call per kind, in series. They could be concurrent and are not yet: five
// small list calls against an API server that answers are milliseconds, and the
// budget row says so rather than a number being quietly chosen.
func (c *Client) Workloads(ctx context.Context, namespace string) ([]Workload, error) {
	out := make([]Workload, 0, 32)

	deployments, err := c.cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing deployments: %w", err)
	}
	for _, d := range deployments.Items {
		desired := int32(0)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		out = append(out, Workload{
			Kind: KindDeployment, Namespace: d.Namespace, Name: d.Name,
			Desired: desired, Ready: d.Status.ReadyReplicas,
			Summary:   fmt.Sprintf("%d of %d ready", d.Status.ReadyReplicas, desired),
			Image:     firstImage(d.Spec.Template.Spec.Containers),
			CreatedAt: stamp(d.CreationTimestamp),
			Scalable:  true, Rollable: true,
			Stoppable: true, Stopped: desired == 0,
			WouldStartWith: rememberedIn(d.Annotations),
		})
	}

	sets, err := c.cs.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing statefulsets: %w", err)
	}
	for _, s := range sets.Items {
		desired := int32(0)
		if s.Spec.Replicas != nil {
			desired = *s.Spec.Replicas
		}
		// An ordered update is the normal state of a StatefulSet mid-rollout,
		// so the summary says which phase rather than implying an outage.
		summary := fmt.Sprintf("%d of %d ready", s.Status.ReadyReplicas, desired)
		if s.Status.UpdatedReplicas != desired && desired > 0 {
			summary += fmt.Sprintf(", %d updated (it replaces them one at a time)",
				s.Status.UpdatedReplicas)
		}
		out = append(out, Workload{
			Kind: KindStatefulSet, Namespace: s.Namespace, Name: s.Name,
			Desired: desired, Ready: s.Status.ReadyReplicas, Summary: summary,
			Image:     firstImage(s.Spec.Template.Spec.Containers),
			CreatedAt: stamp(s.CreationTimestamp),
			Scalable:  true, Rollable: true,
			Stoppable: true, Stopped: desired == 0,
			WouldStartWith: rememberedIn(s.Annotations),
		})
	}

	daemons, err := c.cs.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing daemonsets: %w", err)
	}
	for _, d := range daemons.Items {
		// Desired here is how many nodes match, which is the cluster's shape
		// rather than somebody's intention -- so it is not scalable, and saying
		// "on N nodes" keeps that distinction on the screen. It is stoppable
		// since 2026-09-26, by a selector no node matches (see startstop.go).
		stopped := daemonSetStopped(d.Spec.Template.Spec.NodeSelector)
		summary := fmt.Sprintf("%d of %d nodes ready", d.Status.NumberReady,
			d.Status.DesiredNumberScheduled)
		if stopped {
			summary = "stopped — it runs on no node"
		}
		out = append(out, Workload{
			Kind: KindDaemonSet, Namespace: d.Namespace, Name: d.Name,
			Desired: d.Status.DesiredNumberScheduled, Ready: d.Status.NumberReady,
			Summary:   summary,
			Image:     firstImage(d.Spec.Template.Spec.Containers),
			CreatedAt: stamp(d.CreationTimestamp),
			Scalable:  false, Rollable: true,
			Stoppable: true, Stopped: stopped,
		})
	}

	jobs, err := c.cs.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing jobs: %w", err)
	}
	for _, j := range jobs.Items {
		out = append(out, Workload{
			Kind: KindJob, Namespace: j.Namespace, Name: j.Name,
			Desired: 1, Ready: j.Status.Succeeded,
			Summary:   jobSummary(j),
			Image:     firstImage(j.Spec.Template.Spec.Containers),
			CreatedAt: stamp(j.CreationTimestamp),
			Suspended: j.Spec.Suspend != nil && *j.Spec.Suspend,
			Scalable:  false, Rollable: false,
			Stoppable: true,
			Stopped:   j.Spec.Suspend != nil && *j.Spec.Suspend,
		})
	}

	crons, err := c.cs.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing cronjobs: %w", err)
	}
	for _, c := range crons.Items {
		row := Workload{
			Kind: KindCronJob, Namespace: c.Namespace, Name: c.Name,
			Image:     firstImage(c.Spec.JobTemplate.Spec.Template.Spec.Containers),
			CreatedAt: stamp(c.CreationTimestamp),
			Schedule:  c.Spec.Schedule,
			Suspended: c.Spec.Suspend != nil && *c.Spec.Suspend,
			Scalable:  false, Rollable: false,
			Stoppable: true,
		}
		row.Stopped = row.Suspended
		if c.Status.LastScheduleTime != nil {
			row.LastRun = stamp(*c.Status.LastScheduleTime)
		}
		// A CronJob has no pods between runs, so "0 ready" would report a
		// schedule as an outage.
		switch {
		case row.Suspended:
			row.Summary = "suspended — it will not run"
		case row.LastRun == "":
			row.Summary = "has never run"
		default:
			row.Summary = "last run " + row.LastRun
		}
		out = append(out, row)
	}

	return out, nil
}

func jobSummary(j batchv1.Job) string {
	switch {
	case j.Spec.Suspend != nil && *j.Spec.Suspend:
		return "suspended"
	case j.Status.Failed > 0 && j.Status.Succeeded == 0:
		return fmt.Sprintf("failed %d time(s)", j.Status.Failed)
	case j.Status.Succeeded > 0:
		return fmt.Sprintf("finished, %d succeeded", j.Status.Succeeded)
	case j.Status.Active > 0:
		return fmt.Sprintf("running, %d active", j.Status.Active)
	default:
		return "waiting to start"
	}
}

func firstImage(containers []corev1.Container) string {
	if len(containers) == 0 {
		return ""
	}
	// The first container's image. A workload with several is unusual and the
	// first is the one an operator means; the rest are on the pods.
	return containers[0].Image
}

func stamp(t metav1.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// ScaleWorkload sets the replica count of a kind that has one.
//
// A DaemonSet is refused by name rather than by the API server's own error,
// because "the server rejected the request" does not tell somebody that the
// count they wanted is the number of nodes.
func (c *Client) ScaleWorkload(
	ctx context.Context, kind WorkloadKind, namespace, name string, replicas int32,
) error {
	if replicas < 0 {
		return fmt.Errorf("kube: %d is not a replica count", replicas)
	}

	switch kind {
	case KindDeployment:
		return c.Scale(ctx, namespace, name, replicas)
	case KindStatefulSet:
		scale, err := c.cs.AppsV1().StatefulSets(namespace).GetScale(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("%w: statefulset %s/%s", ErrNoSuchWorkload, namespace, name)
			}
			return fmt.Errorf("kube: reading the scale of %s/%s: %w", namespace, name, err)
		}
		scale.Spec.Replicas = replicas
		if _, err := c.cs.AppsV1().StatefulSets(namespace).UpdateScale(
			ctx, name, scale, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("kube: scaling %s/%s to %d: %w", namespace, name, replicas, err)
		}
		return nil
	default:
		return fmt.Errorf("kube: a %s has no replica count to set. Its size is the cluster's "+
			"shape rather than a number somebody chooses", kind)
	}
}

// RolloutRestartWorkload replaces a workload's pods under its own strategy.
func (c *Client) RolloutRestartWorkload(
	ctx context.Context, kind WorkloadKind, namespace, name string, now time.Time,
) error {
	patch, err := restartPatch(now)
	if err != nil {
		return err
	}

	switch kind {
	case KindDeployment:
		return c.RolloutRestart(ctx, namespace, name, now)
	case KindStatefulSet:
		_, err = c.cs.AppsV1().StatefulSets(namespace).Patch(
			ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	case KindDaemonSet:
		_, err = c.cs.AppsV1().DaemonSets(namespace).Patch(
			ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	default:
		return fmt.Errorf("kube: a %s has no pod template to roll. It runs to completion rather "+
			"than being kept running", kind)
	}

	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: %s %s/%s", ErrNoSuchWorkload, kind, namespace, name)
		}
		return fmt.Errorf("kube: restarting the rollout of %s/%s: %w", namespace, name, err)
	}
	return nil
}
