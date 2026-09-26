package kube

import (
	"context"
	"encoding/json"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// What the power model (internal/power) needs from a cluster's Kubernetes API
// to switch one app off and on again (2026-09-26).
//
// The verbs themselves -- stop, start, rollout restart -- already lived here
// and are reused rather than restated. What this file adds is the three things
// they did not cover: a DaemonSet's stop, the disabled mark, and killing an
// app's pods outright for the forced variants.
//
// Everything is addressed by {kind, namespace, name}, and a bare pod by
// {namespace, name}, deliberately: the list of apps a screen shows is being
// built elsewhere, and a power model that depended on that list would be a
// power model that could not act on anything the list had not yet learned to
// show.

// StoppedSelectorKey is the node label a stopped DaemonSet's pods are told to
// need. No node carries it, so the DaemonSet runs nowhere.
const StoppedSelectorKey = "holzkube.io/stopped"

// selectorBeforeStop is where a DaemonSet's selector is written down while it
// is stopped. See startstop.go for why it is recorded even though a start does
// not need it.
const selectorBeforeStop = "holzkube.io/node-selector-before-stop"

// DisabledAnnotation marks an app as "off, and stays off": a start refuses
// while it is there, and only an enable removes it.
//
// On the OBJECT and not in this product's store, for the reason the replica
// count is: the cluster is the thing that knows, this installation can be
// reinstalled, and somebody using kubectl can see why their deployment will
// not come back.
const DisabledAnnotation = "holzkube.io/disabled"

// daemonSetStopped reports whether a DaemonSet carries the selector a stop put
// on it.
func daemonSetStopped(selector map[string]string) bool {
	_, stopped := selector[StoppedSelectorKey]
	return stopped
}

func (c *Client) stopDaemonSet(ctx context.Context, namespace, name string) error {
	set, err := c.cs.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return wrapNotFound(err, KindDaemonSet, namespace, name)
	}
	selector := set.Spec.Template.Spec.NodeSelector
	if daemonSetStopped(selector) {
		return nil
	}
	if selector == nil {
		selector = map[string]string{}
	}
	before, err := json.Marshal(selector)
	if err != nil {
		return fmt.Errorf("kube: recording the selector: %w", err)
	}

	// One patch for both halves, so the record of what the selector was and
	// the selector that stops it arrive together or not at all.
	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]string{selectorBeforeStop: string(before)},
		},
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"nodeSelector": map[string]string{StoppedSelectorKey: "true"},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("kube: building the patch: %w", err)
	}
	return c.patchWorkload(ctx, KindDaemonSet, namespace, name, patch)
}

func (c *Client) startDaemonSet(ctx context.Context, namespace, name string) error {
	set, err := c.cs.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return wrapNotFound(err, KindDaemonSet, namespace, name)
	}
	if !daemonSetStopped(set.Spec.Template.Spec.NodeSelector) {
		return nil
	}

	// A null removes exactly the key the stop added and leaves every other
	// key of the selector as it is now -- including one somebody changed
	// while it was stopped, which a restore from the recorded copy would have
	// silently undone.
	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]any{selectorBeforeStop: nil},
		},
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"nodeSelector": map[string]any{StoppedSelectorKey: nil},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("kube: building the patch: %w", err)
	}
	return c.patchWorkload(ctx, KindDaemonSet, namespace, name, patch)
}

// AppPower is one app's state, in the terms the power model decides in.
type AppPower struct {
	// Disabled is the mark DisabledAnnotation.
	Disabled bool

	// Stopped is "it is meant to run nothing": scaled to zero, suspended, or
	// told to need a node nobody is.
	Stopped bool

	// Finished is a Job that has run to completion or given up. It is not
	// stopped -- nobody stopped it -- and it will not run again, which is what
	// makes a start of it meaningless.
	Finished bool

	// Desired and Ready are the counts, per kind: replicas for a Deployment or
	// a StatefulSet, nodes for a DaemonSet, and for a Job one if it is still
	// running. A CronJob has none between runs, and reports none.
	Desired int32
	Ready   int32

	// WouldStartWith is the count a start of a scaled workload brings back.
	WouldStartWith int32
}

// AppPowerOf reads one app.
func (c *Client) AppPowerOf(ctx context.Context, kind WorkloadKind, namespace, name string) (AppPower, error) {
	switch kind {
	case KindDeployment:
		d, err := c.cs.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return AppPower{}, wrapNotFound(err, kind, namespace, name)
		}
		desired := int32(1)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		return AppPower{
			Disabled: disabledIn(d.Annotations),
			Stopped:  desired == 0,
			Desired:  desired, Ready: d.Status.ReadyReplicas,
			WouldStartWith: rememberedIn(d.Annotations),
		}, nil

	case KindStatefulSet:
		s, err := c.cs.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return AppPower{}, wrapNotFound(err, kind, namespace, name)
		}
		desired := int32(1)
		if s.Spec.Replicas != nil {
			desired = *s.Spec.Replicas
		}
		return AppPower{
			Disabled: disabledIn(s.Annotations),
			Stopped:  desired == 0,
			Desired:  desired, Ready: s.Status.ReadyReplicas,
			WouldStartWith: rememberedIn(s.Annotations),
		}, nil

	case KindDaemonSet:
		d, err := c.cs.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return AppPower{}, wrapNotFound(err, kind, namespace, name)
		}
		return AppPower{
			Disabled: disabledIn(d.Annotations),
			Stopped:  daemonSetStopped(d.Spec.Template.Spec.NodeSelector),
			Desired:  d.Status.DesiredNumberScheduled,
			Ready:    d.Status.NumberReady,
		}, nil

	case KindJob:
		j, err := c.cs.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return AppPower{}, wrapNotFound(err, kind, namespace, name)
		}
		out := AppPower{
			Disabled: disabledIn(j.Annotations),
			Stopped:  j.Spec.Suspend != nil && *j.Spec.Suspend,
			Finished: jobFinished(*j),
		}
		if !out.Stopped && !out.Finished {
			out.Desired = 1
			if j.Status.Active > 0 {
				out.Ready = 1
			}
		}
		return out, nil

	case KindCronJob:
		j, err := c.cs.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return AppPower{}, wrapNotFound(err, kind, namespace, name)
		}
		return AppPower{
			Disabled: disabledIn(j.Annotations),
			Stopped:  j.Spec.Suspend != nil && *j.Spec.Suspend,
		}, nil

	default:
		return AppPower{}, fmt.Errorf("%w: %q is not a kind this product knows", ErrNoSuchWorkload, kind)
	}
}

// jobFinished reports a Job that will not run again on its own.
//
// Its conditions first, because that is the Job controller's own verdict; the
// completion time as the fallback, for a cluster that reports one and not the
// other.
func jobFinished(j batchv1.Job) bool {
	for _, cond := range j.Status.Conditions {
		if (cond.Type == batchv1.JobComplete || cond.Type == batchv1.JobFailed) &&
			cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return j.Status.CompletionTime != nil
}

func disabledIn(annotations map[string]string) bool {
	return annotations[DisabledAnnotation] == "true"
}

// SetAppDisabled writes or removes the disabled mark.
//
// A null in the patch removes the annotation rather than setting it to "false":
// an app that was disabled and enabled again should look like one that never
// was, not carry a flag somebody has to know how to read.
func (c *Client) SetAppDisabled(ctx context.Context, kind WorkloadKind, namespace, name string, disabled bool) error {
	var value any
	if disabled {
		value = "true"
	}
	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]any{DisabledAnnotation: value},
		},
	})
	if err != nil {
		return fmt.Errorf("kube: building the patch: %w", err)
	}
	return c.patchWorkload(ctx, kind, namespace, name, patch)
}

// PodPower is one pod as the power model needs to see it.
type PodPower struct {
	// Owner names the controller that would recreate this pod, "Kind/name",
	// and is empty for a bare pod. A pod with an owner is not an app of its
	// own: stopping it means stopping the owner.
	Owner string

	Phase string

	// Ready is every container reporting ready.
	Ready bool
}

// PodPowerOf reads one pod.
func (c *Client) PodPowerOf(ctx context.Context, namespace, name string) (PodPower, error) {
	pod, err := c.cs.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return PodPower{}, fmt.Errorf("%w: pod %s/%s", ErrNoSuchWorkload, namespace, name)
		}
		return PodPower{}, fmt.Errorf("kube: reading %s/%s: %w", namespace, name, err)
	}

	out := PodPower{Phase: string(pod.Status.Phase), Ready: len(pod.Status.ContainerStatuses) > 0}
	if ref := metav1.GetControllerOf(pod); ref != nil {
		out.Owner = ref.Kind + "/" + ref.Name
	}
	for _, s := range pod.Status.ContainerStatuses {
		if !s.Ready {
			out.Ready = false
		}
	}
	return out, nil
}

// DeleteBarePod removes a pod nothing owns, which is the only stop a bare pod
// has. force deletes it with a grace period of zero.
//
// A pod with a controller is refused: deleting it is a restart, not a stop,
// and that difference is exactly what RestartPod's refusal exists to keep.
func (c *Client) DeleteBarePod(ctx context.Context, namespace, name string, force bool) error {
	pod, err := c.PodPowerOf(ctx, namespace, name)
	if err != nil {
		return err
	}
	if pod.Owner != "" {
		return fmt.Errorf("%w: %s/%s belongs to %s, so deleting it would only make its "+
			"controller start another. Stop the %s instead", ErrCannotStop, namespace, name,
			pod.Owner, pod.Owner)
	}
	return c.deletePod(ctx, namespace, name, force)
}

func (c *Client) deletePod(ctx context.Context, namespace, name string, force bool) error {
	opts := metav1.DeleteOptions{}
	if force {
		zero := int64(0)
		opts.GracePeriodSeconds = &zero
	}
	if err := c.cs.CoreV1().Pods(namespace).Delete(ctx, name, opts); err != nil {
		if apierrors.IsNotFound(err) {
			// Gone between the read and the delete, which is the outcome that
			// was being asked for.
			return nil
		}
		return fmt.Errorf("kube: deleting %s/%s: %w", namespace, name, err)
	}
	return nil
}

// ForceDeleteAppPods deletes every pod of one app with a grace period of zero,
// and returns how many.
//
// The pods are found the way the app's own controller finds them -- by its
// selector -- rather than by walking owner references, because a selector is
// what decides which pods the controller counts, and so which pods it will
// recreate. A CronJob has no selector of its own; its pods are the pods of the
// Jobs it owns.
func (c *Client) ForceDeleteAppPods(ctx context.Context, kind WorkloadKind, namespace, name string) (int, error) {
	selectors, err := c.appSelectors(ctx, kind, namespace, name)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, selector := range selectors {
		pods, err := c.cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return deleted, fmt.Errorf("kube: listing the pods of %s %s/%s: %w", kind, namespace, name, err)
		}
		for _, pod := range pods.Items {
			if err := c.deletePod(ctx, namespace, pod.Name, true); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	return deleted, nil
}

// appSelectors is the label selector, or for a CronJob the selectors, an
// app's pods are matched by.
func (c *Client) appSelectors(ctx context.Context, kind WorkloadKind, namespace, name string) ([]string, error) {
	var selector *metav1.LabelSelector
	switch kind {
	case KindDeployment:
		d, err := c.cs.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, wrapNotFound(err, kind, namespace, name)
		}
		selector = d.Spec.Selector
	case KindStatefulSet:
		s, err := c.cs.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, wrapNotFound(err, kind, namespace, name)
		}
		selector = s.Spec.Selector
	case KindDaemonSet:
		d, err := c.cs.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, wrapNotFound(err, kind, namespace, name)
		}
		selector = d.Spec.Selector
	case KindJob:
		j, err := c.cs.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, wrapNotFound(err, kind, namespace, name)
		}
		selector = j.Spec.Selector
	case KindCronJob:
		return c.cronJobSelectors(ctx, namespace, name)
	default:
		return nil, fmt.Errorf("%w: %q is not a kind this product knows", ErrNoSuchWorkload, kind)
	}

	rendered, err := renderSelector(selector)
	if err != nil {
		return nil, fmt.Errorf("kube: %s %s/%s: %w", kind, namespace, name, err)
	}
	return []string{rendered}, nil
}

func (c *Client) cronJobSelectors(ctx context.Context, namespace, name string) ([]string, error) {
	if _, err := c.cs.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{}); err != nil {
		return nil, wrapNotFound(err, KindCronJob, namespace, name)
	}
	jobs, err := c.cs.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing the jobs of cronjob %s/%s: %w", namespace, name, err)
	}

	var out []string
	for _, j := range jobs.Items {
		owner := metav1.GetControllerOf(&j)
		if owner == nil || owner.Kind != string(KindCronJob) || owner.Name != name {
			continue
		}
		rendered, err := renderSelector(j.Spec.Selector)
		if err != nil {
			return nil, fmt.Errorf("kube: job %s/%s: %w", namespace, j.Name, err)
		}
		out = append(out, rendered)
	}
	return out, nil
}

// renderSelector turns a selector into the query string a pod list takes.
//
// An empty selector is refused rather than rendered: it matches every pod in
// the namespace, and "force-delete this app's pods" must never mean "every pod
// in the namespace" because an object was missing a field.
func renderSelector(selector *metav1.LabelSelector) (string, error) {
	if selector == nil || (len(selector.MatchLabels) == 0 && len(selector.MatchExpressions) == 0) {
		return "", fmt.Errorf("%w: it has no pod selector, and an empty one would match every pod "+
			"in the namespace", ErrCannotStop)
	}
	parsed, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return "", fmt.Errorf("its pod selector cannot be read: %w", err)
	}
	return parsed.String(), nil
}

// Undrained names the pods on a node that a drain with these options would
// still move -- the drain's own classification, asked without moving anything.
//
// It is what makes a drain inside another job resumable, the way the drain job
// is: "is there anything left here that this drain would move" is a read, and
// it is the same question the drain was asked.
func (c *Client) Undrained(ctx context.Context, node string, opts DrainOptions) ([]string, error) {
	pods, err := c.PodsOnNode(ctx, node)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, pod := range pods {
		if pod.wouldMove(opts) {
			out = append(out, pod.Name())
		}
	}
	return out, nil
}
