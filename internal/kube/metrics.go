package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// What is actually being used (2026-09-19).
//
// # The number the node detail deliberately does not have
//
// NodeDetail reports what pods REQUESTED, because that is what the scheduler
// reserves. This is the other half: what they are using now. Both are needed and
// neither substitutes for the other -- a node with no room left and 5% usage is
// over-reserved, a node with room left and 95% usage is about to fall over, and
// those are opposite repairs.
//
// # Why this can be absent, and why that is said out loud
//
// The numbers come from metrics-server, which is a separate thing somebody
// installs. Talos does not ship it. So the honest answer on most clusters is
// "nobody is collecting this", and it has to be distinguishable from "usage is
// zero" -- the same distinction the whole product makes about a node that was
// not asked (INV-08).
//
// # Read through the raw client
//
// The typed metrics client lives in k8s.io/metrics, a separate module and a
// third upstream to pin and keep in step with the server. Two JSON shapes read
// through the REST client is a smaller commitment than a module, and the shapes
// are stable API -- they are what `kubectl top` reads.

// ErrNoMetrics reports a cluster with nobody collecting usage.
var ErrNoMetrics = errors.New("kube: this cluster has no metrics-server, so nothing is collecting usage")

// Usage is what one thing is using now.
type Usage struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`

	// CPUMillis and MemoryBytes as numbers, so a screen can compare them
	// against allocatable without parsing quantities twice.
	CPUMillis   int64 `json:"cpu_millis"`
	MemoryBytes int64 `json:"memory_bytes"`

	// CPU and Memory as Kubernetes writes them, for showing.
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

// NodeUsage reads what each node is using.
func (c *Client) NodeUsage(ctx context.Context) ([]Usage, error) {
	return c.usage(ctx, "/apis/metrics.k8s.io/v1beta1/nodes")
}

// PodUsage reads what each pod is using, in one namespace or across all.
func (c *Client) PodUsage(ctx context.Context, namespace string) ([]Usage, error) {
	path := "/apis/metrics.k8s.io/v1beta1/pods"
	if namespace != "" {
		path = "/apis/metrics.k8s.io/v1beta1/namespaces/" + namespace + "/pods"
	}
	return c.usage(ctx, path)
}

// usage reads and adds up one of the two metrics shapes.
func (c *Client) usage(ctx context.Context, path string) ([]Usage, error) {
	raw, err := c.cs.CoreV1().RESTClient().Get().AbsPath(path).DoRaw(ctx)
	if err != nil {
		// A cluster with no metrics-server answers 404 on the whole API group,
		// and that is a fact about the cluster rather than a failure of this
		// product. It is a named error so the screen can say "nobody is
		// collecting this" instead of showing zeroes.
		if isNotFoundLike(err) {
			return nil, ErrNoMetrics
		}
		return nil, fmt.Errorf("kube: reading usage: %w", err)
	}

	var body struct {
		Items []struct {
			Metadata metav1.ObjectMeta `json:"metadata"`
			// A node carries usage directly; a pod carries it per container.
			Usage      map[string]string `json:"usage"`
			Containers []struct {
				Usage map[string]string `json:"usage"`
			} `json:"containers"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("kube: reading usage: %w", err)
	}

	out := make([]Usage, 0, len(body.Items))
	for _, item := range body.Items {
		row := Usage{Namespace: item.Metadata.Namespace, Name: item.Metadata.Name}

		add := func(usage map[string]string) {
			if q, err := resource.ParseQuantity(usage["cpu"]); err == nil {
				row.CPUMillis += q.MilliValue()
			}
			if q, err := resource.ParseQuantity(usage["memory"]); err == nil {
				row.MemoryBytes += q.Value()
			}
		}

		if len(item.Containers) > 0 {
			// Summed across the containers, the way `kubectl top pod` shows it:
			// a pod's usage is what its containers use together, and reporting
			// only the first would understate every sidecar.
			for _, container := range item.Containers {
				add(container.Usage)
			}
		} else {
			add(item.Usage)
		}

		row.CPU = fmt.Sprintf("%dm", row.CPUMillis)
		row.Memory = resource.NewQuantity(row.MemoryBytes, resource.BinarySI).String()
		out = append(out, row)
	}
	return out, nil
}

// isNotFoundLike reports an answer that means "this API is not installed".
//
// Both shapes are checked because they both happen: a cluster with no
// metrics-server has no API group at all (404 from the aggregator), and one
// whose metrics-server is starting answers 503 through the aggregation layer.
// Neither is a defect in this product and both mean the same thing to a screen.
func isNotFoundLike(err error) bool {
	var status interface{ Status() metav1.Status }
	if !errors.As(err, &status) {
		return false
	}
	code := status.Status().Code
	return code == 404 || code == 503
}
