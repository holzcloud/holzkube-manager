package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ErrNoSuchDaemonSet is a DaemonSet that is not there. For kube-proxy that is
// an ordinary cluster rather than a fault: a CNI that replaces kube-proxy
// (Cilium, for one) runs clusters without it.
var ErrNoSuchDaemonSet = errors.New("kube: no such DaemonSet")

// SetDaemonSetImage points one container of a DaemonSet at a new image.
//
// A strategic merge patch naming the container, so nothing else in the
// template is touched and the DaemonSet controller rolls the pods the way it
// always does. A DaemonSet without a container of that name is refused rather
// than given a second container: a patch that adds one would change what the
// workload runs, not which version of it.
func (c *Client) SetDaemonSetImage(ctx context.Context, namespace, name, container, image string) error {
	set, err := c.cs.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("%w: %s/%s", ErrNoSuchDaemonSet, namespace, name)
	}
	if err != nil {
		return err
	}
	found := false
	for _, ct := range set.Spec.Template.Spec.Containers {
		if ct.Name == container {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("kube: %s/%s has no container %q", namespace, name, container)
	}

	patch, err := json.Marshal(map[string]any{
		"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
			"containers": []map[string]string{{"name": container, "image": image}},
		}}},
	})
	if err != nil {
		return err
	}
	_, err = c.cs.AppsV1().DaemonSets(namespace).Patch(ctx, name, types.StrategicMergePatchType, patch,
		metav1.PatchOptions{FieldManager: FieldManager})
	return err
}

// DaemonSetRollout is how far a DaemonSet has got with its current template.
type DaemonSetRollout struct {
	Done      bool
	Updated   int32
	Available int32
	Desired   int32
}

// DaemonSetRolledOut reads the rollout the way `kubectl rollout status` does:
// done when the controller has seen the current generation and every pod it
// schedules runs that template and is available.
func (c *Client) DaemonSetRolledOut(ctx context.Context, namespace, name string) (DaemonSetRollout, error) {
	set, err := c.cs.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return DaemonSetRollout{}, fmt.Errorf("%w: %s/%s", ErrNoSuchDaemonSet, namespace, name)
	}
	if err != nil {
		return DaemonSetRollout{}, err
	}
	st := set.Status
	r := DaemonSetRollout{
		Updated: st.UpdatedNumberScheduled, Available: st.NumberAvailable, Desired: st.DesiredNumberScheduled,
	}
	r.Done = st.ObservedGeneration >= set.Generation &&
		st.UpdatedNumberScheduled == st.DesiredNumberScheduled &&
		st.NumberAvailable == st.DesiredNumberScheduled
	return r, nil
}

// DaemonSetImages is the image of each container in a DaemonSet's template.
func (c *Client) DaemonSetImages(ctx context.Context, namespace, name string) ([]string, error) {
	set, err := c.cs.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("%w: %s/%s", ErrNoSuchDaemonSet, namespace, name)
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, ct := range set.Spec.Template.Spec.Containers {
		out = append(out, ct.Image)
	}
	return out, nil
}
