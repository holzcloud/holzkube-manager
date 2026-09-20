package kube

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// The cluster's networking, as the questions somebody actually has (2026-09-20).
//
// # A Service with no endpoints is the commonest broken thing in Kubernetes
//
// And it looks completely healthy in every list: it has a name, a type, a
// ClusterIP and its ports. Nothing about it says that no pod matches its
// selector, so every request to it is refused instantly and the workload that
// calls it reports a connection error pointing at itself.
//
// So the endpoints are counted per Service, from EndpointSlices, and a Service
// with none is named as such. That is the one fact that makes this screen worth
// more than `kubectl get svc`.
//
// # The other half is that a selector can be right and the pods not ready
//
// EndpointSlices carry `conditions.ready`, and an endpoint that is not ready is
// excluded from load balancing. A Service with three endpoints of which none are
// ready is as broken as one with none at all, and it looks better. Both numbers
// are therefore reported.
//
// # NetworkPolicies: the dangerous default is having none
//
// A namespace with no NetworkPolicy accepts traffic from every pod in the
// cluster. That is Kubernetes's default and it is not obviously wrong -- plenty
// of clusters run that way deliberately -- but it is invisible, and "we have
// policies" is usually believed about a cluster where two namespaces have them
// and eleven do not. So the answer says which namespaces have none.
//
// And a policy whose selector matches no pod is worse than no policy, because
// somebody believes it is protecting something. That is counted too.
//
// # Nothing here writes
//
// A NetworkPolicy applied wrongly cuts a cluster off from itself, including from
// whatever this daemon needs to reach it. The manifest path exists for anybody
// who means to change one, with a plan shown first.

// ServiceSummary is one Service, with whether anything is behind it.
type ServiceSummary struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`

	// Type is ClusterIP, NodePort, LoadBalancer or ExternalName.
	Type      string `json:"type"`
	ClusterIP string `json:"cluster_ip"`

	// External is what reaches it from outside: a LoadBalancer's addresses, an
	// ExternalName's target, or the node ports. Empty for a plain ClusterIP,
	// which is the answer to "why can I not reach this from my laptop".
	External string `json:"external"`

	// Ports in one readable line: "80/TCP -> 8080".
	Ports []string `json:"ports"`

	// Selector as it is written, or empty for a Service whose endpoints are
	// managed by hand. The second kind exists and looks broken to a screen that
	// assumes every Service selects pods.
	Selector string `json:"selector"`

	// Endpoints is how many addresses are behind it, and how many of those are
	// READY. An endpoint that is not ready is excluded from load balancing, so a
	// Service with three of which none are ready is as broken as one with none.
	Endpoints      int `json:"endpoints"`
	ReadyEndpoints int `json:"ready_endpoints"`

	// Healthy is false when nothing usable is behind it. This is the finding the
	// screen exists for.
	Healthy bool `json:"healthy"`

	// Notice says what is wrong, or what kind of Service this is when "no
	// endpoints" is correct for it.
	Notice    string `json:"notice"`
	CreatedAt string `json:"created_at"`
}

// PolicySummary is one NetworkPolicy and what it actually applies to.
type PolicySummary struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`

	// Applies is the pod selector in words: "every pod in the namespace" for an
	// empty selector, which is the one that surprises people.
	Applies string `json:"applies"`

	// Types is Ingress, Egress or both. A policy with Ingress rules only leaves
	// outbound traffic entirely alone, which is not what "we have a policy for
	// that" usually means.
	Types []string `json:"types"`

	// Selects is how many pods its selector currently matches. Zero means
	// somebody believes something is protected and nothing is.
	Selects int `json:"selects"`

	// Isolating says the policy actually denies something: a policy with an
	// empty rule list denies all traffic of its types, and one with rules allows
	// exactly those. Both isolate; a policy selecting no pods does not.
	Isolating bool `json:"isolating"`

	Summary   string `json:"summary"`
	Healthy   bool   `json:"healthy"`
	CreatedAt string `json:"created_at"`
}

// Network is the cluster's networking on one answer.
type Network struct {
	Services []ServiceSummary `json:"services"`
	Policies []PolicySummary  `json:"policies"`

	// Unprotected is every namespace with pods and no NetworkPolicy at all: they
	// accept traffic from every pod in the cluster. Named rather than counted,
	// because "eleven namespaces are open" is not actionable and a list is.
	Unprotected []string `json:"unprotected"`

	// IngressClasses, and which is the default. An Ingress naming no class in a
	// cluster with no default is an Ingress no controller will ever pick up, and
	// it looks exactly like a working one.
	IngressClasses []string `json:"ingress_classes"`
	DefaultClass   string   `json:"default_ingress_class"`

	Notice string `json:"notice"`
}

// Network reads services, endpoints, policies and ingress classes.
func (c *Client) Network(ctx context.Context, namespace string) (Network, error) {
	out := Network{
		Services:       make([]ServiceSummary, 0, 8),
		Policies:       make([]PolicySummary, 0, 4),
		Unprotected:    make([]string, 0, 4),
		IngressClasses: make([]string, 0, 2),
		Notice: "Endpoints are counted from EndpointSlices, which is what the kube-proxy on each " +
			"node actually load-balances to. A Service with none refuses every connection " +
			"instantly, and nothing else about it looks wrong.",
	}

	services, err := c.cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Network{}, fmt.Errorf("kube: listing services: %w", err)
	}

	// One list for the whole set rather than one call per Service: a cluster has
	// more services than it has slices, and asking per service turns a screen
	// into a hundred round trips.
	slices, err := c.cs.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Network{}, fmt.Errorf("kube: listing endpoint slices: %w", err)
	}
	type counts struct{ total, ready int }
	behind := map[string]*counts{}
	for _, slice := range slices.Items {
		owner := slice.Labels[discoveryv1.LabelServiceName]
		if owner == "" {
			continue
		}
		key := slice.Namespace + "/" + owner
		into, ok := behind[key]
		if !ok {
			into = &counts{}
			behind[key] = into
		}
		for _, endpoint := range slice.Endpoints {
			into.total++
			// Absent means ready, per the API's own convention. An endpoint that
			// is not ready is left out of load balancing entirely.
			if endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready {
				into.ready++
			}
		}
	}

	for _, service := range services.Items {
		row := ServiceSummary{
			Namespace: service.Namespace,
			Name:      service.Name,
			Type:      string(service.Spec.Type),
			ClusterIP: service.Spec.ClusterIP,
			Ports:     portLines(service.Spec.Ports),
			Selector:  selectorLine(service.Spec.Selector),
			External:  externalOf(service),
			Healthy:   true,
			CreatedAt: stamp(service.CreationTimestamp),
		}
		if found := behind[service.Namespace+"/"+service.Name]; found != nil {
			row.Endpoints, row.ReadyEndpoints = found.total, found.ready
		}

		switch {
		case service.Spec.Type == corev1.ServiceTypeExternalName:
			// No endpoints is correct: it is a DNS alias and nothing else.
			row.Notice = "A DNS alias for " + service.Spec.ExternalName +
				". It has no endpoints because it forwards nothing."
		case len(service.Spec.Selector) == 0 && row.Endpoints == 0:
			row.Healthy = false
			row.Notice = "It selects no pods and has no endpoints, so its addresses are meant to " +
				"be managed by hand and nobody has. Every connection to it is refused."
		case row.Endpoints == 0:
			row.Healthy = false
			// The finding this screen exists for.
			row.Notice = "Nothing is behind it: no pod matches " + row.Selector +
				". Every connection to it is refused instantly, and the workload that calls it " +
				"reports a connection error that looks like its own fault."
		case row.ReadyEndpoints == 0:
			row.Healthy = false
			row.Notice = fmt.Sprintf("%d pods match it and none are ready, so none are in the "+
				"load balancer. This is as broken as having no endpoints and looks better.",
				row.Endpoints)
		case row.ReadyEndpoints < row.Endpoints:
			row.Notice = fmt.Sprintf("%d of %d endpoints are ready; the rest are not receiving "+
				"traffic.", row.ReadyEndpoints, row.Endpoints)
		}
		out.Services = append(out.Services, row)
	}

	pods, err := c.cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Network{}, fmt.Errorf("kube: listing pods: %w", err)
	}

	policies, err := c.cs.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Network{}, fmt.Errorf("kube: listing network policies: %w", err)
	}
	withPolicy := map[string]bool{}
	for _, policy := range policies.Items {
		withPolicy[policy.Namespace] = true

		selector, selectErr := metav1.LabelSelectorAsSelector(&policy.Spec.PodSelector)
		row := PolicySummary{
			Namespace: policy.Namespace,
			Name:      policy.Name,
			Applies:   podSelectorWords(policy.Spec.PodSelector),
			Types:     policyTypes(policy),
			Healthy:   true,
			CreatedAt: stamp(policy.CreationTimestamp),
		}
		if selectErr == nil {
			for _, pod := range pods.Items {
				if pod.Namespace == policy.Namespace && selector.Matches(labels.Set(pod.Labels)) {
					row.Selects++
				}
			}
		}
		row.Isolating = row.Selects > 0
		switch {
		case row.Selects == 0:
			// Worse than no policy, because somebody believes it is protecting
			// something.
			row.Healthy = false
			row.Summary = "It matches no pod in " + policy.Namespace +
				", so it protects nothing. A policy that selects nothing is worse than none: " +
				"somebody believes this is in force."
		case len(row.Types) == 1 && row.Types[0] == "Ingress":
			row.Summary = fmt.Sprintf("Restricts inbound traffic to %d pods. Outbound traffic from "+
				"them is not restricted at all.", row.Selects)
		default:
			row.Summary = fmt.Sprintf("Applies to %d pods, %s.", row.Selects,
				strings.ToLower(strings.Join(row.Types, " and "))+" traffic")
		}
		out.Policies = append(out.Policies, row)
	}

	// Which namespaces have pods and no policy. The default in Kubernetes is
	// that every pod may reach every other pod, and it is invisible.
	hasPods := map[string]bool{}
	for _, pod := range pods.Items {
		hasPods[pod.Namespace] = true
	}
	for ns := range hasPods {
		if !withPolicy[ns] {
			out.Unprotected = append(out.Unprotected, ns)
		}
	}
	sort.Strings(out.Unprotected)

	classes, err := c.cs.NetworkingV1().IngressClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Network{}, fmt.Errorf("kube: listing ingress classes: %w", err)
	}
	for _, class := range classes.Items {
		out.IngressClasses = append(out.IngressClasses, class.Name)
		if class.Annotations["ingressclass.kubernetes.io/is-default-class"] == "true" {
			out.DefaultClass = class.Name
		}
	}

	return out, nil
}

func portLines(ports []corev1.ServicePort) []string {
	out := make([]string, 0, len(ports))
	for _, port := range ports {
		line := strconv.Itoa(int(port.Port)) + "/" + string(port.Protocol)
		if target := port.TargetPort.String(); target != "" && target != strconv.Itoa(int(port.Port)) {
			line += " → " + target
		}
		if port.NodePort != 0 {
			line += " (node " + strconv.Itoa(int(port.NodePort)) + ")"
		}
		if port.Name != "" {
			line = port.Name + " " + line
		}
		out = append(out, line)
	}
	return out
}

func selectorLine(selector map[string]string) string {
	if len(selector) == 0 {
		return ""
	}
	keys := make([]string, 0, len(selector))
	for key := range selector {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+selector[key])
	}
	return strings.Join(parts, ",")
}

// externalOf is what reaches a Service from outside the cluster.
//
// Empty for a plain ClusterIP, which is the answer to "why can I not reach this
// from my laptop" -- and an answer a list of ports cannot give.
func externalOf(service corev1.Service) string {
	switch service.Spec.Type {
	case corev1.ServiceTypeExternalName:
		return service.Spec.ExternalName
	case corev1.ServiceTypeLoadBalancer:
		addresses := make([]string, 0, 2)
		for _, ingress := range service.Status.LoadBalancer.Ingress {
			if ingress.IP != "" {
				addresses = append(addresses, ingress.IP)
			}
			if ingress.Hostname != "" {
				addresses = append(addresses, ingress.Hostname)
			}
		}
		if len(addresses) == 0 {
			// A LoadBalancer with no address is a Service waiting for a
			// controller that may not exist. Talos ships none by default.
			return "waiting for an address"
		}
		return strings.Join(addresses, ", ")
	case corev1.ServiceTypeNodePort:
		ports := make([]string, 0, len(service.Spec.Ports))
		for _, port := range service.Spec.Ports {
			if port.NodePort != 0 {
				ports = append(ports, "any node:"+strconv.Itoa(int(port.NodePort)))
			}
		}
		return strings.Join(ports, ", ")
	default:
		if len(service.Spec.ExternalIPs) > 0 {
			return strings.Join(service.Spec.ExternalIPs, ", ")
		}
		return ""
	}
}

// podSelectorWords says what a policy applies to.
//
// An EMPTY selector is the one that surprises people: it matches every pod in the
// namespace rather than none, which is the opposite of how an empty filter reads
// everywhere else.
func podSelectorWords(selector metav1.LabelSelector) string {
	if len(selector.MatchLabels) == 0 && len(selector.MatchExpressions) == 0 {
		return "every pod in the namespace"
	}
	return selectorLine(selector.MatchLabels)
}

func policyTypes(policy networkingv1.NetworkPolicy) []string {
	if len(policy.Spec.PolicyTypes) > 0 {
		out := make([]string, 0, len(policy.Spec.PolicyTypes))
		for _, kind := range policy.Spec.PolicyTypes {
			out = append(out, string(kind))
		}
		return out
	}
	// Absent means Ingress, plus Egress if there are egress rules. Deriving it
	// here rather than showing nothing: a policy without the field is ordinary,
	// and an empty types column would read as a policy that does nothing.
	out := []string{"Ingress"}
	if len(policy.Spec.Egress) > 0 {
		out = append(out, "Egress")
	}
	return out
}
