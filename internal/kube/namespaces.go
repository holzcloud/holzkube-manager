package kube

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Namespaces, quotas, and the kinds this cluster has that Kubernetes does not
// (2026-09-20).
//
// # Why a namespace needs a screen and not a dropdown
//
// It has been a filter everywhere in this product, which is what a namespace is
// for most questions. Two things about one are findings in their own right, and
// neither is visible anywhere else:
//
//   - A namespace stuck in `Terminating`. Something in it has a finalizer nothing
//     will clear, so the namespace hangs -- for weeks, in practice -- and every
//     attempt to recreate it fails with "already exists" while every attempt to
//     use it fails too. `kubectl get ns` shows the word and nothing about which
//     resources are holding it.
//   - A ResourceQuota that is full. That is why the next pod is refused, and the
//     pod's own event says "exceeded quota" in a namespace whose quota nothing on
//     any screen showed. It is the single most confusing refusal in Kubernetes
//     after an unbound claim.
//
// # The combination nobody expects
//
// A namespace with a ResourceQuota on cpu or memory and NO LimitRange refuses
// every pod that does not set requests -- not because the pod is too big, but
// because the quota cannot account for a pod that asked for nothing. The error
// says "must specify limits.cpu", which reads like the pod is wrong. It is the
// namespace that is half-configured, and this is said in words.
//
// # Custom resource definitions
//
// The object route already reads any kind through the cluster's own discovery, so
// a CustomResourceDefinition this build has never heard of works. What was missing
// is finding out which ones exist: a cluster's own kinds are where its operators
// keep their state, and "what is a Longhorn Volume called here" is not answerable
// from a list of pods.
//
// Objects are NOT counted per kind. That would be one list call per definition, on
// a cluster that can have two hundred of them, and a screen's worth of numbers
// nobody asked for is not worth a request storm.

// NamespaceSummary is one namespace and what is true of it.
type NamespaceSummary struct {
	Name string `json:"name"`

	// Phase is Active or Terminating.
	Phase string `json:"phase"`

	// PodsRunning counts what is in it, so an empty namespace is visible as empty
	// rather than as a name.
	PodsRunning int `json:"pods_running"`

	// Quotas are the ResourceQuotas in it, each as "used of hard".
	Quotas []QuotaLine `json:"quotas"`

	// HasLimitRange says whether pods that set no requests get defaults. With a
	// quota and without this, every such pod is refused.
	HasLimitRange bool `json:"has_limit_range"`

	// Healthy is false when the namespace itself is the problem: terminating, or
	// holding a full quota.
	Healthy bool `json:"healthy"`

	// Notice explains what is wrong, in words, because every one of these reads
	// as a fault of the workload rather than of the namespace.
	Notice    string `json:"notice"`
	CreatedAt string `json:"created_at"`
}

// QuotaLine is one limit and how much of it is spoken for.
type QuotaLine struct {
	Quota    string `json:"quota"`
	Resource string `json:"resource"`
	Used     string `json:"used"`
	Hard     string `json:"hard"`

	// Percent of hard, rounded; -1 when hard cannot be read as a number.
	Percent int `json:"percent"`
}

// CustomKind is one CustomResourceDefinition, in what it is rather than its
// hundred fields.
type CustomKind struct {
	// Group and Kind, as somebody would write them in a manifest.
	Group string `json:"group"`
	Kind  string `json:"kind"`

	// Versions the cluster serves, and which one it stores.
	Versions []string `json:"versions"`
	Stored   string   `json:"stored"`

	// Namespaced or Cluster.
	Scope string `json:"scope"`

	// Established says the API server is actually serving it. A definition that
	// is not established is a kind every manifest naming it will be refused for,
	// and nothing else says so.
	Established bool `json:"established"`

	Notice    string `json:"notice"`
	CreatedAt string `json:"created_at"`
}

// Inventory is the namespace answer, with the cluster's own kinds beside it.
//
// Not called Namespaces, because Client.Namespaces already answers the list of
// names every screen's dropdown is built from. Two questions, two names.
type Inventory struct {
	Namespaces  []NamespaceSummary `json:"namespaces"`
	CustomKinds []CustomKind       `json:"custom_kinds"`
	Notice      string             `json:"notice"`
}

// Inventory reads every namespace, its quotas, and the cluster's custom kinds.
func (c *Client) Inventory(ctx context.Context) (Inventory, error) {
	out := Inventory{
		Namespaces:  make([]NamespaceSummary, 0, 8),
		CustomKinds: make([]CustomKind, 0, 8),
		Notice: "A quota's used figure is what the namespace has reserved, and a full quota is why " +
			"the next pod is refused -- the pod's own event says \"exceeded quota\" and nothing " +
			"else shows the quota. Objects of the cluster's own kinds are not counted here: that " +
			"would be one list call per kind, and a cluster can have two hundred.",
	}

	namespaces, err := c.cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Inventory{}, fmt.Errorf("kube: listing namespaces: %w", err)
	}

	pods, err := c.cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return Inventory{}, fmt.Errorf("kube: listing pods: %w", err)
	}
	running := map[string]int{}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		running[pod.Namespace]++
	}

	quotas, err := c.cs.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return Inventory{}, fmt.Errorf("kube: listing resource quotas: %w", err)
	}
	byNamespace := map[string][]QuotaLine{}
	full := map[string][]string{}
	for _, quota := range quotas.Items {
		names := make([]string, 0, len(quota.Status.Hard))
		for name := range quota.Status.Hard {
			names = append(names, string(name))
		}
		// Sorted, because a map's order would make the screen shuffle between
		// refreshes and nothing would be findable twice.
		sort.Strings(names)
		for _, name := range names {
			hard := quota.Status.Hard[corev1.ResourceName(name)]
			used := quota.Status.Used[corev1.ResourceName(name)]
			line := QuotaLine{
				Quota: quota.Name, Resource: name,
				Used: used.String(), Hard: hard.String(),
				Percent: percent(used.MilliValue(), hard.MilliValue()),
			}
			byNamespace[quota.Namespace] = append(byNamespace[quota.Namespace], line)
			if line.Percent >= 100 {
				full[quota.Namespace] = append(full[quota.Namespace], name)
			}
		}
	}

	ranges, err := c.cs.CoreV1().LimitRanges("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return Inventory{}, fmt.Errorf("kube: listing limit ranges: %w", err)
	}
	hasRange := map[string]bool{}
	for _, limit := range ranges.Items {
		hasRange[limit.Namespace] = true
	}

	for _, namespace := range namespaces.Items {
		row := NamespaceSummary{
			Name:          namespace.Name,
			Phase:         string(namespace.Status.Phase),
			PodsRunning:   running[namespace.Name],
			Quotas:        listOrEmpty(byNamespace[namespace.Name]),
			HasLimitRange: hasRange[namespace.Name],
			Healthy:       true,
			CreatedAt:     stamp(namespace.CreationTimestamp),
		}

		switch {
		case namespace.Status.Phase == corev1.NamespaceTerminating:
			row.Healthy = false
			row.Notice = "It has been deleted and something in it will not go. " +
				terminatingReason(namespace) +
				" Until that clears, the name cannot be reused and nothing new can be created here."
		case len(full[namespace.Name]) > 0:
			row.Healthy = false
			row.Notice = "Its quota is full on " + strings.Join(full[namespace.Name], ", ") +
				", so the next thing asking for any of those is refused. The refusal appears on " +
				"the workload as \"exceeded quota\", which looks like the workload's fault."
		case len(row.Quotas) > 0 && !row.HasLimitRange && quotaCoversCompute(row.Quotas):
			// The combination nobody expects, and the error it produces names the
			// pod rather than the namespace.
			row.Healthy = false
			row.Notice = "It has a quota on cpu or memory and no LimitRange, so every pod that " +
				"does not set requests itself is refused -- not for being too big, but because the " +
				"quota cannot account for a pod that asked for nothing. The error says \"must " +
				"specify limits\", which reads as the pod being wrong."
		}
		out.Namespaces = append(out.Namespaces, row)
	}

	kinds, err := c.customKinds(ctx)
	if err != nil {
		return Inventory{}, err
	}
	out.CustomKinds = kinds

	return out, nil
}

// stringAt reads a string at a path in an unstructured object, or "".
//
// A CustomResourceDefinition's own shape is guaranteed by the API server, so a
// missing field here means a cluster answering something this product cannot read
// -- and an empty string in a table is a better outcome than refusing the screen.
func stringAt(object map[string]any, path ...string) string {
	value, found, err := unstructured.NestedString(object, path...)
	if err != nil || !found {
		return ""
	}
	return value
}

func nestedSlice(object map[string]any, path ...string) ([]any, bool, error) {
	return unstructured.NestedSlice(object, path...)
}

// quotaCoversCompute reports whether a quota constrains cpu or memory.
//
// Only those two make a pod without requests unschedulable. A quota on
// `persistentvolumeclaims` or `count/pods` does not, and warning about it would
// be a warning on every namespace that limits object counts -- which is most of
// them that have a quota at all.
func quotaCoversCompute(lines []QuotaLine) bool {
	for _, line := range lines {
		switch line.Resource {
		case "cpu", "memory", "requests.cpu", "requests.memory",
			"limits.cpu", "limits.memory":
			return true
		}
	}
	return false
}

// terminatingReason says what is holding a namespace, rather than that one is
// held.
//
// The namespace's own status carries the conditions Kubernetes writes when it
// cannot finish: which resources remain, and which finalizers are unmet. That is
// the whole answer, and `kubectl get ns` shows none of it.
func terminatingReason(namespace corev1.Namespace) string {
	for _, condition := range namespace.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		case corev1.NamespaceDeletionContentFailure,
			corev1.NamespaceContentRemaining,
			corev1.NamespaceFinalizersRemaining:
			if condition.Message != "" {
				return condition.Message
			}
		}
	}
	if len(namespace.Spec.Finalizers) > 0 {
		names := make([]string, 0, len(namespace.Spec.Finalizers))
		for _, finalizer := range namespace.Spec.Finalizers {
			names = append(names, string(finalizer))
		}
		return "Finalizers still on it: " + strings.Join(names, ", ") + "."
	}
	// Said rather than left blank: the API server has not written a condition
	// yet, which happens in the first seconds and is not the same as nothing
	// holding it.
	return "The API server has not said yet what is holding it."
}

// customKinds lists the cluster's own kinds.
//
// Read through the dynamic client rather than an apiextensions clientset: this
// product already carries the dynamic client and the RESTMapper for the object
// route, and adding a second typed clientset for one list would be a dependency
// for nothing.
func (c *Client) customKinds(ctx context.Context) ([]CustomKind, error) {
	definitions := schema.GroupVersionResource{
		Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions",
	}
	list, err := c.dyn.Resource(definitions).List(ctx, metav1.ListOptions{})
	if err != nil {
		// A cluster may not serve apiextensions to this identity, and a cluster
		// with no custom kinds at all answers 404 on the resource. Neither is a
		// failure of the namespace screen.
		if isNotFoundLike(err) || apierrors.IsForbidden(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("kube: listing custom resource definitions: %w", err)
	}

	out := make([]CustomKind, 0, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		row := CustomKind{
			Group:     stringAt(item.Object, "spec", "group"),
			Kind:      stringAt(item.Object, "spec", "names", "kind"),
			Scope:     stringAt(item.Object, "spec", "scope"),
			Versions:  make([]string, 0, 2),
			CreatedAt: stamp(item.GetCreationTimestamp()),
		}

		versions, _, _ := nestedSlice(item.Object, "spec", "versions")
		for _, entry := range versions {
			version, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			name, _ := version["name"].(string)
			if name == "" {
				continue
			}
			if served, ok := version["served"].(bool); ok && !served {
				// Present in the definition and not answered by the API server.
				// Listing it as available would send somebody to a version that
				// is refused.
				continue
			}
			row.Versions = append(row.Versions, name)
			if stored, ok := version["storage"].(bool); ok && stored {
				row.Stored = name
			}
		}

		conditions, _, _ := nestedSlice(item.Object, "status", "conditions")
		for _, entry := range conditions {
			condition, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if condition["type"] == "Established" && condition["status"] == "True" {
				row.Established = true
			}
		}
		if !row.Established {
			row.Notice = "The API server is not serving this kind, so every manifest naming it is " +
				"refused. A definition waits like this while its conversion webhook is unreachable."
		}

		out = append(out, row)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].Kind < out[j].Kind
	})
	return out, nil
}
