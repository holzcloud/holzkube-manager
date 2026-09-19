package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Pod and workload actions (milestone v1.17, slice 4).
//
// Three operations, and the naming is the design: what an operator wants is
// usually "restart this", and Kubernetes has no restart. It has "delete the pod
// and let the controller make another", which is the same thing exactly when a
// controller owns the pod and is data loss when nothing does. So this package
// offers RestartPod, refuses when nothing owns the pod, and says which it is.

// ErrNothingWouldRecreateIt reports a pod no controller owns.
//
// Deleting one is not a restart: nothing brings it back. It is the same refusal
// the drain makes about the same pods, and it is a separate error because the
// repair is different -- a drain wants a flag, and here the answer is usually
// "that is not the pod you meant".
var ErrNothingWouldRecreateIt = errors.New("kube: no controller owns this pod, so deleting it is not a restart")

// ErrNoSuchWorkload reports a deployment or pod the cluster does not have.
var ErrNoSuchWorkload = errors.New("kube: the cluster has no such workload")

// RestartPod deletes one pod so its controller replaces it.
//
// The refusal is the interesting half. A pod with a controller comes back
// within seconds and this is the ordinary way to restart a workload; a pod
// without one is gone, and a product that deleted it because somebody clicked
// "restart" would have destroyed something on the strength of a word.
func (c *Client) RestartPod(ctx context.Context, namespace, name string) error {
	pod, err := c.cs.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: %s/%s", ErrNoSuchWorkload, namespace, name)
		}
		return fmt.Errorf("kube: reading %s/%s: %w", namespace, name, err)
	}

	if metav1.GetControllerOf(pod) == nil {
		return fmt.Errorf("%w: %s/%s would simply be gone. If you meant to remove it, remove it "+
			"deliberately rather than as a restart", ErrNothingWouldRecreateIt, namespace, name)
	}

	if err := c.cs.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			// Gone between the read and the delete, which is the outcome that
			// was being asked for.
			return nil
		}
		return fmt.Errorf("kube: restarting %s/%s: %w", namespace, name, err)
	}
	return nil
}

// Deployment is one deployment, in the numbers somebody scaling it needs.
//
// Desired and Ready are both here for the reason a pod carries phase and
// readiness: "3 desired" is an intention and "1 ready" is the fact, and a
// screen that showed only the first would say a workload is running when it is
// not.
type Deployment struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Desired   int32  `json:"desired"`
	Ready     int32  `json:"ready"`
	Updated   int32  `json:"updated"`
	Available int32  `json:"available"`
	Image     string `json:"image"`
	CreatedAt string `json:"created_at"`
}

// Deployments lists deployments, in one namespace or across all of them.
func (c *Client) Deployments(ctx context.Context, namespace string) ([]Deployment, error) {
	list, err := c.cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing deployments: %w", err)
	}

	out := make([]Deployment, 0, len(list.Items))
	for _, d := range list.Items {
		row := Deployment{
			Namespace: d.Namespace,
			Name:      d.Name,
			Ready:     d.Status.ReadyReplicas,
			Updated:   d.Status.UpdatedReplicas,
			Available: d.Status.AvailableReplicas,
			CreatedAt: d.CreationTimestamp.UTC().Format(time.RFC3339),
		}
		if d.Spec.Replicas != nil {
			row.Desired = *d.Spec.Replicas
		}
		if len(d.Spec.Template.Spec.Containers) > 0 {
			// The first container's image. A deployment with several is
			// unusual and the first is the one an operator means; the rest are
			// on the pods, which this screen also shows.
			row.Image = d.Spec.Template.Spec.Containers[0].Image
		}
		out = append(out, row)
	}
	return out, nil
}

// Scale sets a deployment's replica count.
//
// Through the scale subresource rather than by patching the deployment, which
// is what `kubectl scale` does: the subresource exists so that a client can
// change the count without owning the rest of the spec, and a client that
// patched the whole object could undo a field somebody else had just set.
func (c *Client) Scale(ctx context.Context, namespace, name string, replicas int32) error {
	if replicas < 0 {
		return fmt.Errorf("kube: %d is not a replica count", replicas)
	}

	scale, err := c.cs.AppsV1().Deployments(namespace).GetScale(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: deployment %s/%s", ErrNoSuchWorkload, namespace, name)
		}
		return fmt.Errorf("kube: reading the scale of %s/%s: %w", namespace, name, err)
	}

	scale.Spec.Replicas = replicas
	if _, err := c.cs.AppsV1().Deployments(namespace).UpdateScale(
		ctx, name, scale, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("kube: scaling %s/%s to %d: %w", namespace, name, replicas, err)
	}
	return nil
}

// RolloutRestart restarts every pod of a deployment, the way `kubectl rollout
// restart` does it.
//
// It annotates the pod template with a timestamp, which changes the template
// hash, which makes the deployment controller roll the pods out one at a time
// under its own strategy -- surge, maxUnavailable and readiness probes
// included. That is the difference from deleting the pods: deleting them takes
// the workload down, and this replaces it while keeping it up.
func (c *Client) RolloutRestart(ctx context.Context, namespace, name string, now time.Time) error {
	// Marshalled rather than assembled with fmt.Sprintf, and the reason is not
	// only taste: internal/talos's seam guard reads a format string with a verb,
	// a colon and a verb as an address being built by hand, and it was right to
	// look -- a product that builds JSON by concatenation will eventually build
	// one that is not valid. So the shape is a value and the encoder writes it.
	patch, err := json.Marshal(map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": map[string]string{
						"kubectl.kubernetes.io/restartedAt": now.UTC().Format(time.RFC3339),
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("kube: building the restart patch: %w", err)
	}

	_, err = c.cs.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: deployment %s/%s", ErrNoSuchWorkload, namespace, name)
		}
		return fmt.Errorf("kube: restarting the rollout of %s/%s: %w", namespace, name, err)
	}
	return nil
}
