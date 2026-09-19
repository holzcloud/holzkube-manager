package kube

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// The resources beside the workloads (2026-09-19).
//
// # Why these five
//
// They are the ones an operator reaches for while something is wrong, and each
// answers a question the workload list cannot:
//
//   - A ConfigMap is where the setting somebody changed lives.
//   - An Ingress is why a URL does or does not reach the cluster.
//   - A PersistentVolumeClaim that is Pending is why a StatefulSet will not
//     start, and the pod's own events only say "unbound claim".
//   - A HorizontalPodAutoscaler is why a replica count keeps changing back
//     after somebody scales it by hand -- which otherwise looks like this
//     product ignoring a click.
//   - A PodDisruptionBudget is why a drain refuses, and the drain already says
//     so; seeing them before starting is the other half.
//
// # Secrets are listed and never read
//
// Their NAMES matter -- "does this namespace have the pull secret" is a real
// question -- and their contents are refused everywhere (see Describe). So they
// appear here with keys and sizes and no values, which answers the question
// without putting a credential on a screen.

// Resource is one non-workload object, in the few fields that answer the
// question it is looked at for.
type Resource struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`

	// Summary is the answer this kind exists to give, written per kind for the
	// reason a workload's is: a percentage of a PersistentVolumeClaim is not a
	// thing.
	Summary string `json:"summary"`

	// Detail is the longer line, when the kind has one: an Ingress's hosts, a
	// ConfigMap's keys.
	Detail string `json:"detail"`

	// Healthy is false when this object is the reason something is stuck. A
	// Pending claim and a budget that currently allows no disruption are the
	// two that matter.
	Healthy bool `json:"healthy"`

	CreatedAt string `json:"created_at"`
}

// Resources lists the non-workload objects, in one namespace or across all.
func (c *Client) Resources(ctx context.Context, namespace string) ([]Resource, error) {
	out := make([]Resource, 0, 32)

	configs, err := c.cs.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing configmaps: %w", err)
	}
	for _, m := range configs.Items {
		out = append(out, Resource{
			Kind: "ConfigMap", Namespace: m.Namespace, Name: m.Name,
			Summary:   fmt.Sprintf("%d keys", len(m.Data)+len(m.BinaryData)),
			Detail:    strings.Join(keysOf(m.Data), ", "),
			Healthy:   true,
			CreatedAt: stamp(m.CreationTimestamp),
		})
	}

	// Names and key names only. The values are refused everywhere in this
	// product, and a list that carried them would be the one place they leaked.
	secrets, err := c.cs.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing secrets: %w", err)
	}
	for _, s := range secrets.Items {
		out = append(out, Resource{
			Kind: "Secret", Namespace: s.Namespace, Name: s.Name,
			Summary:   fmt.Sprintf("%s, %d keys", s.Type, len(s.Data)),
			Detail:    strings.Join(keysOfBytes(s.Data), ", "),
			Healthy:   true,
			CreatedAt: stamp(s.CreationTimestamp),
		})
	}

	claims, err := c.cs.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing persistentvolumeclaims: %w", err)
	}
	for _, p := range claims.Items {
		size := ""
		if q, ok := p.Status.Capacity[corev1.ResourceStorage]; ok {
			size = q.String()
		} else if q, ok := p.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			size = q.String() + " requested"
		}
		summary := string(p.Status.Phase)
		if size != "" {
			summary += ", " + size
		}
		detail := ""
		if p.Spec.StorageClassName != nil {
			detail = "class " + *p.Spec.StorageClassName
		}
		out = append(out, Resource{
			Kind: "PersistentVolumeClaim", Namespace: p.Namespace, Name: p.Name,
			Summary: summary, Detail: detail,
			// A Pending claim is the reason a StatefulSet will not start, and
			// the pod's own events only say "unbound claim".
			Healthy:   p.Status.Phase == corev1.ClaimBound,
			CreatedAt: stamp(p.CreationTimestamp),
		})
	}

	ingresses, err := c.cs.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing ingresses: %w", err)
	}
	for _, i := range ingresses.Items {
		hosts := make([]string, 0, len(i.Spec.Rules))
		for _, rule := range i.Spec.Rules {
			if rule.Host != "" {
				hosts = append(hosts, rule.Host)
			}
		}
		// An Ingress with no address has not been picked up by a controller,
		// which is exactly the "why does this URL not work" case.
		address := addressOf(i)
		out = append(out, Resource{
			Kind: "Ingress", Namespace: i.Namespace, Name: i.Name,
			Summary:   summarise(address, "no address yet — no controller has claimed it"),
			Detail:    strings.Join(hosts, ", "),
			Healthy:   address != "",
			CreatedAt: stamp(i.CreationTimestamp),
		})
	}

	scalers, err := c.cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing horizontalpodautoscalers: %w", err)
	}
	for _, h := range scalers.Items {
		minimum := int32(1)
		if h.Spec.MinReplicas != nil {
			minimum = *h.Spec.MinReplicas
		}
		out = append(out, Resource{
			Kind: "HorizontalPodAutoscaler", Namespace: h.Namespace, Name: h.Name,
			Summary: fmt.Sprintf("%d now, between %d and %d",
				h.Status.CurrentReplicas, minimum, h.Spec.MaxReplicas),
			// Named, because this is the object that puts a replica count back
			// after somebody scales by hand -- which otherwise looks like this
			// product ignoring a click.
			Detail: fmt.Sprintf("it sets the replicas of %s %s",
				h.Spec.ScaleTargetRef.Kind, h.Spec.ScaleTargetRef.Name),
			Healthy:   true,
			CreatedAt: stamp(h.CreationTimestamp),
		})
	}

	budgets, err := c.cs.PolicyV1().PodDisruptionBudgets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing poddisruptionbudgets: %w", err)
	}
	for _, b := range budgets.Items {
		out = append(out, Resource{
			Kind: "PodDisruptionBudget", Namespace: b.Namespace, Name: b.Name,
			Summary: fmt.Sprintf("%d of %d healthy, %d may be disrupted",
				b.Status.CurrentHealthy, b.Status.DesiredHealthy, b.Status.DisruptionsAllowed),
			// Zero allowed is why a drain refuses, and knowing that before
			// starting one is the other half of the drain's own refusal.
			Detail:    disruptionDetail(b),
			Healthy:   b.Status.DisruptionsAllowed > 0,
			CreatedAt: stamp(b.CreationTimestamp),
		})
	}

	return out, nil
}

func disruptionDetail(b policyv1.PodDisruptionBudget) string {
	if b.Status.DisruptionsAllowed > 0 {
		return ""
	}
	return "a drain of a node carrying these pods will be refused until another becomes healthy"
}

func addressOf(i networkingv1.Ingress) string {
	for _, in := range i.Status.LoadBalancer.Ingress {
		if in.IP != "" {
			return in.IP
		}
		if in.Hostname != "" {
			return in.Hostname
		}
	}
	return ""
}

func summarise(value, whenEmpty string) string {
	if value == "" {
		return whenEmpty
	}
	return value
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return sorted(out)
}

func keysOfBytes(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return sorted(out)
}

func sorted(in []string) []string {
	// Map iteration is random, and a list that reordered itself between two
	// reads would look like the object changed.
	for i := 1; i < len(in); i++ {
		for j := i; j > 0 && in[j] < in[j-1]; j-- {
			in[j], in[j-1] = in[j-1], in[j]
		}
	}
	return in
}

// DeleteObject removes one object.
//
// # Why there is a delete at all, and why it is this shape
//
// Until now this product could create and change and never remove, which sounds
// safe and is not: an operator who cannot delete a stuck Job or a leftover
// ConfigMap from here goes to kubectl, and the audit archive loses the whole
// session rather than one line of it.
//
// It takes a kind and a name and nothing else. There is no label selector and no
// "delete all in namespace": the operations that remove many things at once are
// the ones where a mistake is unbounded, and `--prune` was already refused for
// the same reason on the manifest path.
func (c *Client) DeleteObject(ctx context.Context, apiVersion, kind, namespace, name string) error {
	if strings.EqualFold(kind, "Namespace") {
		// Deleting a namespace deletes everything in it, asynchronously, with
		// no way to stop it once it starts. That is a different operation from
		// removing an object and it is not offered here.
		return fmt.Errorf("%w: deleting a namespace removes everything inside it, and nothing "+
			"stops it once it starts. That is not an object deletion", ErrRefusedKind)
	}

	object, err := c.resolveObject(apiVersion, kind)
	if err != nil {
		return err
	}
	if object.Scope.Name() != "namespace" {
		namespace = ""
	}

	if err := c.dyn.Resource(object.Resource).Namespace(namespace).
		Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: %s %s/%s", ErrNoSuchWorkload, kind, namespace, name)
		}
		return fmt.Errorf("kube: deleting %s %s/%s: %w", kind, namespace, name, err)
	}
	return nil
}

// resolveObject maps an apiVersion and kind to the resource that holds it,
// through the cluster's own discovery.
//
// Shared by Describe and DeleteObject so the two cannot disagree about where a
// kind lives -- a disagreement that would show one object and delete another.
func (c *Client) resolveObject(apiVersion, kind string) (*meta.RESTMapping, error) {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return nil, fmt.Errorf("%w: %q is not an apiVersion", ErrManifestInvalid, apiVersion)
	}
	mapper, err := c.restMapper()
	if err != nil {
		return nil, err
	}
	mapping, err := mapper.RESTMapping(schema.GroupKind{Group: gv.Group, Kind: kind}, gv.Version)
	if err != nil {
		return nil, fmt.Errorf("%w: this cluster does not have %s %s: %w",
			ErrManifestInvalid, apiVersion, kind, err)
	}
	return mapping, nil
}
