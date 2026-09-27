package talos

import (
	"context"
	"strings"

	"github.com/siderolabs/talos/pkg/machinery/resources/k8s"
)

// ControlPlaneComponents are the three static pods a control-plane node runs,
// in the order a Kubernetes upgrade moves them -- the order talosctl
// upgrade-k8s uses: the API server first, because the other two talk to it and
// a newer client against an older server is the skew Kubernetes forbids.
var ControlPlaneComponents = []string{"kube-apiserver", "kube-controller-manager", "kube-scheduler"}

// StaticPod is what the kubelet reports back about one static pod.
type StaticPod struct {
	// Component is the static pod's name without the node suffix, one of
	// ControlPlaneComponents, or "" for a static pod this product does not
	// manage.
	Component string
	// Image is the image the pod's first container runs. The static pods
	// Talos renders have one container each.
	Image string
	// Ready is the pod's Ready condition.
	Ready bool
}

// StaticPods reads the static pods the node's kubelet reports.
//
// It reads Talos's StaticPodStatus resources rather than asking the
// Kubernetes API for the mirror pods: the question is what this node runs, the
// node answers it, and the answer does not depend on the API server that the
// same upgrade is in the middle of replacing.
func (c *ClusterClient) StaticPods(ctx context.Context) ([]StaticPod, error) {
	var out []StaticPod
	err := eachResource(ctx, c.COSI(), k8s.NamespaceName, k8s.StaticPodStatusType,
		func(r *k8s.StaticPodStatus) {
			out = append(out, staticPodFrom(r.Metadata().ID(), r.TypedSpec().PodStatus))
		})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// staticPodFrom reads one status. The ID is "<namespace>/<pod name>" and the
// pod name is "<component>-<nodename>"; the status is a Kubernetes PodStatus
// as a map. Anything missing reads as not ready rather than failing, because a
// pod that has just been restarted reports a partial status for a moment, and
// that moment is exactly what a caller waiting on it polls through.
func staticPodFrom(id string, status map[string]any) StaticPod {
	name := id
	if i := strings.LastIndex(id, "/"); i >= 0 {
		name = id[i+1:]
	}
	p := StaticPod{}
	for _, c := range ControlPlaneComponents {
		if strings.HasPrefix(name, c+"-") {
			p.Component = c
			break
		}
	}

	if conditions, ok := status["conditions"].([]any); ok {
		for _, c := range conditions {
			m, _ := c.(map[string]any)
			if m["type"] == "Ready" && m["status"] == "True" {
				p.Ready = true
			}
		}
	}
	if containers, ok := status["containerStatuses"].([]any); ok && len(containers) > 0 {
		if m, ok := containers[0].(map[string]any); ok {
			p.Image, _ = m["image"].(string)
			if ready, ok := m["ready"].(bool); ok && !ready {
				p.Ready = false
			}
		}
	}
	return p
}
