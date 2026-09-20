package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Stopping and starting a workload (2026-09-20).
//
// # "Stop this pod" is not a thing Kubernetes has
//
// Deleting a pod does not stop it: its controller makes another within seconds.
// That is the whole point of a controller, and it is why the restart button
// works at all. Stopping means telling the CONTROLLER to want none, and what an
// operator means by "stop this service" is always the controller.
//
// So this takes a workload, and a pod is resolved to its owner first. A pod that
// nothing owns has no stop -- deleting it is the only way to make it go, and
// that is a deletion rather than a stop, which the restart path already refuses
// to blur.
//
// # Starting again needs a number nobody wrote down
//
// Scaling to zero throws away the replica count, and "start it again" then has
// no answer but 1. For a three-replica service that silently halves capacity at
// the moment somebody is restoring it.
//
// So the count is written onto the workload as an annotation before it goes to
// zero, and read back when it starts. On the OBJECT rather than in this
// product's store, deliberately: the cluster is the thing that knows, this
// installation can be reinstalled, and somebody using kubectl can see why their
// deployment has a strange annotation.
//
// # A CronJob stops differently, and that is the right difference
//
// It has no replicas. Suspending it is exactly "stop": it keeps its schedule and
// runs nothing. Scaling something to zero and suspending something are the same
// intention expressed in the two ways Kubernetes offers, so both are here under
// one verb.

// ErrCannotStop reports a workload that has no stop.
var ErrCannotStop = errors.New("kube: that cannot be stopped")

// replicasBeforeStop is where the count waits while a workload is stopped.
const replicasBeforeStop = "holzkube.holzcloud.ch/replicas-before-stop"

// Stop makes a workload run nothing, remembering how much it ran.
func (c *Client) Stop(ctx context.Context, kind WorkloadKind, namespace, name string) error {
	switch kind {
	case KindCronJob, KindJob:
		// Suspending IS stopping for these: they have no replicas, and a
		// suspended CronJob keeps its schedule and runs nothing.
		return c.suspend(ctx, kind, namespace, name, true)

	case KindDeployment, KindStatefulSet:
		current, err := c.replicasOf(ctx, kind, namespace, name)
		if err != nil {
			return err
		}
		if current == 0 {
			return nil
		}
		// Written BEFORE the scale, so an interrupted stop leaves the count
		// recoverable rather than lost. An annotation that outlives a stop that
		// did not happen is harmless; a stop whose count went missing is not.
		if err := c.remember(ctx, kind, namespace, name, current); err != nil {
			return err
		}
		return c.ScaleWorkload(ctx, kind, namespace, name, 0)

	default:
		return fmt.Errorf("%w: a %s runs on every matching node, so there is no count to set "+
			"to zero. Cordon or drain the nodes instead, or remove the %s", ErrCannotStop, kind, kind)
	}
}

// Start makes a stopped workload run again, at the count it ran at.
func (c *Client) Start(ctx context.Context, kind WorkloadKind, namespace, name string) error {
	switch kind {
	case KindCronJob, KindJob:
		return c.suspend(ctx, kind, namespace, name, false)

	case KindDeployment, KindStatefulSet:
		remembered, err := c.remembered(ctx, kind, namespace, name)
		if err != nil {
			return err
		}
		if remembered <= 0 {
			// Nothing was written down, so this was scaled to zero by something
			// else -- a kubectl, an autoscaler, a manifest. One is the only
			// honest answer and the screen says where it came from.
			remembered = 1
		}
		return c.ScaleWorkload(ctx, kind, namespace, name, remembered)

	default:
		return fmt.Errorf("%w: a %s is not something that was stopped", ErrCannotStop, kind)
	}
}

// StoppedState is what a screen needs to offer the right button.
type StoppedState struct {
	Stopped bool `json:"stopped"`

	// WouldStartWith is the count a start would restore. Zero means nothing was
	// written down and a start would use one.
	WouldStartWith int32 `json:"would_start_with"`
}

// StoppedStateOf reports whether a workload is stopped and what starting it
// would do.
func (c *Client) StoppedStateOf(
	ctx context.Context, kind WorkloadKind, namespace, name string,
) (StoppedState, error) {
	switch kind {
	case KindCronJob:
		job, err := c.cs.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return StoppedState{}, wrapNotFound(err, kind, namespace, name)
		}
		return StoppedState{Stopped: job.Spec.Suspend != nil && *job.Spec.Suspend}, nil

	case KindDeployment, KindStatefulSet:
		current, err := c.replicasOf(ctx, kind, namespace, name)
		if err != nil {
			return StoppedState{}, err
		}
		remembered, err := c.remembered(ctx, kind, namespace, name)
		if err != nil {
			return StoppedState{}, err
		}
		return StoppedState{Stopped: current == 0, WouldStartWith: remembered}, nil

	default:
		return StoppedState{}, nil
	}
}

func (c *Client) replicasOf(ctx context.Context, kind WorkloadKind, namespace, name string) (int32, error) {
	switch kind {
	case KindDeployment:
		scale, err := c.cs.AppsV1().Deployments(namespace).GetScale(ctx, name, metav1.GetOptions{})
		if err != nil {
			return 0, wrapNotFound(err, kind, namespace, name)
		}
		return scale.Spec.Replicas, nil
	case KindStatefulSet:
		scale, err := c.cs.AppsV1().StatefulSets(namespace).GetScale(ctx, name, metav1.GetOptions{})
		if err != nil {
			return 0, wrapNotFound(err, kind, namespace, name)
		}
		return scale.Spec.Replicas, nil
	default:
		return 0, fmt.Errorf("%w: a %s has no replica count", ErrCannotStop, kind)
	}
}

// remember writes the count onto the object, so starting again restores what
// was running rather than guessing one.
func (c *Client) remember(ctx context.Context, kind WorkloadKind, namespace, name string, replicas int32) error {
	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]string{
				replicasBeforeStop: strconv.Itoa(int(replicas)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("kube: building the annotation: %w", err)
	}
	return c.patchWorkload(ctx, kind, namespace, name, patch)
}

func (c *Client) remembered(ctx context.Context, kind WorkloadKind, namespace, name string) (int32, error) {
	meta, err := c.metaOf(ctx, kind, namespace, name)
	if err != nil {
		return 0, err
	}
	return rememberedIn(meta.Annotations), nil
}

func (c *Client) metaOf(ctx context.Context, kind WorkloadKind, namespace, name string) (metav1.ObjectMeta, error) {
	switch kind {
	case KindDeployment:
		object, err := c.cs.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return metav1.ObjectMeta{}, wrapNotFound(err, kind, namespace, name)
		}
		return object.ObjectMeta, nil
	case KindStatefulSet:
		object, err := c.cs.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return metav1.ObjectMeta{}, wrapNotFound(err, kind, namespace, name)
		}
		return object.ObjectMeta, nil
	default:
		return metav1.ObjectMeta{}, fmt.Errorf("%w: a %s keeps no count", ErrCannotStop, kind)
	}
}

func (c *Client) patchWorkload(ctx context.Context, kind WorkloadKind, namespace, name string, patch []byte) error {
	var err error
	switch kind {
	case KindDeployment:
		_, err = c.cs.AppsV1().Deployments(namespace).Patch(
			ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	case KindStatefulSet:
		_, err = c.cs.AppsV1().StatefulSets(namespace).Patch(
			ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	case KindCronJob:
		_, err = c.cs.BatchV1().CronJobs(namespace).Patch(
			ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	case KindJob:
		_, err = c.cs.BatchV1().Jobs(namespace).Patch(
			ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	default:
		return fmt.Errorf("%w: a %s cannot be patched this way", ErrCannotStop, kind)
	}
	if err != nil {
		return wrapNotFound(err, kind, namespace, name)
	}
	return nil
}

func (c *Client) suspend(ctx context.Context, kind WorkloadKind, namespace, name string, suspended bool) error {
	patch, err := json.Marshal(map[string]any{"spec": map[string]any{"suspend": suspended}})
	if err != nil {
		return fmt.Errorf("kube: building the patch: %w", err)
	}
	return c.patchWorkload(ctx, kind, namespace, name, patch)
}

func wrapNotFound(err error, kind WorkloadKind, namespace, name string) error {
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("%w: %s %s/%s", ErrNoSuchWorkload, kind, namespace, name)
	}
	return fmt.Errorf("kube: %s %s/%s: %w", kind, namespace, name, err)
}

// OwnerOfPod finds the workload a pod belongs to.
//
// "Stop this pod" always means its controller: deleting a pod makes another
// appear. A pod nothing owns has no stop, and saying that is better than
// deleting it and calling the deletion a stop.
func (c *Client) OwnerOfPod(
	ctx context.Context, namespace, name string,
) (kind WorkloadKind, owner string, err error) {
	pod, err := c.cs.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", "", fmt.Errorf("%w: %s/%s", ErrNoSuchWorkload, namespace, name)
		}
		return "", "", fmt.Errorf("kube: reading %s/%s: %w", namespace, name, err)
	}

	ref := metav1.GetControllerOf(pod)
	if ref == nil {
		return "", "", fmt.Errorf("%w: nothing owns %s/%s, so there is no controller to tell. "+
			"Removing it is a deletion rather than a stop", ErrCannotStop, namespace, name)
	}

	switch ref.Kind {
	case "ReplicaSet":
		// A ReplicaSet is itself owned by the Deployment, and the Deployment is
		// what somebody means. Scaling the ReplicaSet would be undone by its
		// Deployment within seconds, which looks exactly like the button not
		// working.
		set, err := c.cs.AppsV1().ReplicaSets(namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			return "", "", fmt.Errorf("kube: reading the replicaset of %s/%s: %w", namespace, name, err)
		}
		if owner := metav1.GetControllerOf(set); owner != nil && owner.Kind == "Deployment" {
			return KindDeployment, owner.Name, nil
		}
		return "", "", fmt.Errorf("%w: %s/%s belongs to a ReplicaSet that no Deployment owns",
			ErrCannotStop, namespace, name)

	case "StatefulSet":
		return KindStatefulSet, ref.Name, nil
	case "DaemonSet":
		return KindDaemonSet, ref.Name, nil
	case "Job":
		return KindJob, ref.Name, nil
	default:
		return "", "", fmt.Errorf("%w: %s/%s is owned by a %s, which this product does not manage",
			ErrCannotStop, namespace, name, ref.Kind)
	}
}

// rememberedIn reads the count a stop wrote onto an object.
//
// Used by the workload listing as well, so the number a start would restore is
// on the screen before anybody presses it without a second route to ask for it.
//
// Anything that is not a plain count reads as nothing written down -- a hand
// edit, a negative, or a number too large for a replica count. That is the same
// answer as an absent annotation and leads to the same honest fallback of one;
// refusing to start would be worse than starting small. Parsed into 32 bits
// explicitly, because a replica count IS an int32 and a value that does not fit
// one is not a count that was ever running.
func rememberedIn(annotations map[string]string) int32 {
	n, err := strconv.ParseInt(annotations[replicasBeforeStop], 10, 32)
	if err != nil || n < 0 {
		return 0
	}
	return int32(n)
}
