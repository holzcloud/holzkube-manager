package kube_test

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// Networking (2026-09-20).
//
// The claim worth the most is the one about a Service with no endpoints: it is
// the commonest broken thing in Kubernetes and it looks completely healthy in
// every list. Name, type, ClusterIP, ports -- all present, and every connection
// to it refused instantly.

func networkedCluster(t *testing.T) (*kubesim.Server, *kube.Client) {
	t.Helper()

	return newCluster(t, kubesim.Options{
		Services: []kubesim.Service{
			{
				Namespace: "default", Name: "website", Type: "ClusterIP", ClusterIP: "10.96.0.10",
				Selector: map[string]string{"app": "website"},
				Ports:    []kubesim.ServicePort{{Name: "http", Port: 80}},
			},
			{
				// Nothing behind it. The finding.
				Namespace: "default", Name: "api", Type: "ClusterIP", ClusterIP: "10.96.0.11",
				Selector: map[string]string{"app": "api"},
				Ports:    []kubesim.ServicePort{{Port: 8080}},
			},
			{
				// Endpoints, none ready: as broken as having none, and it looks
				// better.
				Namespace: "default", Name: "worker", Type: "ClusterIP", ClusterIP: "10.96.0.12",
				Selector: map[string]string{"app": "worker"},
				Ports:    []kubesim.ServicePort{{Port: 9000}},
			},
			{
				// No endpoints is CORRECT for this kind.
				Namespace: "default", Name: "elsewhere", Type: "ExternalName",
				ExternalName: "db.example.invalid",
			},
		},
		EndpointSlices: []kubesim.EndpointSlice{
			{Namespace: "default", Service: "website", Addresses: 3},
			{Namespace: "default", Service: "worker", Addresses: 2, NotReady: 2},
		},
		Pods: []kubesim.Pod{
			{Namespace: "default", Name: "website-abc", Phase: corev1.PodRunning,
				Labels: map[string]string{"app": "website"}},
			{Namespace: "open", Name: "anything", Phase: corev1.PodRunning},
		},
		NetworkPolicies: []kubesim.NetworkPolicy{
			{Namespace: "default", Name: "website-only", Selector: map[string]string{"app": "website"}},
			// Matches nothing: somebody believes this is in force.
			{Namespace: "default", Name: "api-lockdown", Selector: map[string]string{"app": "api"}},
		},
		IngressClasses: []kubesim.IngressClass{
			{Name: "nginx", Controller: "k8s.io/ingress-nginx", Default: true},
		},
	})
}

func servicesByName(t *testing.T, network kube.Network) map[string]kube.ServiceSummary {
	t.Helper()

	out := map[string]kube.ServiceSummary{}
	for _, service := range network.Services {
		out[service.Name] = service
	}
	return out
}

// TestAServiceWithNothingBehindItIsTheFinding.
func TestAServiceWithNothingBehindItIsTheFinding(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := networkedCluster(t)

	network, err := client.Network(ctx, "")
	if err != nil {
		t.Fatalf("Network: %v", err)
	}
	found := servicesByName(t, network)

	api := found["api"]
	if api.Healthy {
		t.Error("a service with no endpoints is reported as healthy")
	}
	if api.Endpoints != 0 {
		t.Errorf("endpoints = %d, want none", api.Endpoints)
	}
	// It names the selector, because "nothing matches" without saying WHAT does
	// not match leaves somebody exactly where they were.
	if !strings.Contains(api.Notice, "app=api") {
		t.Errorf("notice = %q, want it to name the selector", api.Notice)
	}

	// And the working one is not dragged down with it.
	website := found["website"]
	if !website.Healthy || website.Endpoints != 3 || website.ReadyEndpoints != 3 {
		t.Errorf("website = %+v, want healthy with three ready endpoints", website)
	}
}

// TestEndpointsThatAreNotReadyAreAsBadAsNone, and look better.
//
// An endpoint that is not ready is excluded from load balancing entirely, so a
// Service with two of which none are ready serves nothing -- while its endpoint
// count says two.
func TestEndpointsThatAreNotReadyAreAsBadAsNone(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := networkedCluster(t)

	network, err := client.Network(ctx, "")
	if err != nil {
		t.Fatalf("Network: %v", err)
	}
	worker := servicesByName(t, network)["worker"]

	if worker.Endpoints != 2 {
		t.Errorf("endpoints = %d, want the two that exist", worker.Endpoints)
	}
	if worker.ReadyEndpoints != 0 {
		t.Errorf("ready = %d, want none", worker.ReadyEndpoints)
	}
	if worker.Healthy {
		t.Error("a service whose endpoints are all unready is reported as healthy")
	}
}

// TestAnExternalNameServiceIsNotBrokenForHavingNoEndpoints.
//
// It is a DNS alias and forwards nothing. A screen that flagged it would be
// wrong on every cluster that has one.
func TestAnExternalNameServiceIsNotBrokenForHavingNoEndpoints(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := networkedCluster(t)

	network, err := client.Network(ctx, "")
	if err != nil {
		t.Fatalf("Network: %v", err)
	}
	alias := servicesByName(t, network)["elsewhere"]

	if !alias.Healthy {
		t.Error("an ExternalName service is reported as broken for having no endpoints")
	}
	if alias.External != "db.example.invalid" {
		t.Errorf("external = %q, want the name it aliases", alias.External)
	}
}

// TestAPolicyThatMatchesNoPodIsWorseThanNone, because somebody believes it is
// protecting something.
func TestAPolicyThatMatchesNoPodIsWorseThanNone(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := networkedCluster(t)

	network, err := client.Network(ctx, "")
	if err != nil {
		t.Fatalf("Network: %v", err)
	}

	byName := map[string]kube.PolicySummary{}
	for _, policy := range network.Policies {
		byName[policy.Name] = policy
	}

	empty := byName["api-lockdown"]
	if empty.Selects != 0 {
		t.Errorf("selects = %d, want none", empty.Selects)
	}
	if empty.Healthy {
		t.Error("a policy matching no pod is reported as healthy")
	}
	if !strings.Contains(empty.Summary, "protects nothing") {
		t.Errorf("summary = %q, want it to say the policy is not in force", empty.Summary)
	}

	if byName["website-only"].Selects != 1 {
		t.Errorf("website-only selects %d pods, want 1", byName["website-only"].Selects)
	}
}

// TestANamespaceWithNoPolicyIsNamed.
//
// Kubernetes's default is that every pod may reach every other pod, and it is
// invisible. "We have network policies" is usually believed about a cluster where
// two namespaces have them and eleven do not.
func TestANamespaceWithNoPolicyIsNamed(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := networkedCluster(t)

	network, err := client.Network(ctx, "")
	if err != nil {
		t.Fatalf("Network: %v", err)
	}

	// "open" has a pod and no policy; "default" has policies.
	if len(network.Unprotected) != 1 || network.Unprotected[0] != "open" {
		t.Errorf("unprotected = %v, want exactly the namespace with pods and no policy",
			network.Unprotected)
	}
}

// TestAnEmptyPolicySelectorMeansEveryPod, which is the opposite of how an empty
// filter reads everywhere else in this product.
func TestAnEmptyPolicySelectorMeansEveryPod(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{
		Pods: []kubesim.Pod{
			{Namespace: "default", Name: "one", Phase: corev1.PodRunning},
			{Namespace: "default", Name: "two", Phase: corev1.PodRunning},
		},
		NetworkPolicies: []kubesim.NetworkPolicy{
			{Namespace: "default", Name: "deny-all"},
		},
	})

	network, err := client.Network(ctx, "")
	if err != nil {
		t.Fatalf("Network: %v", err)
	}
	if len(network.Policies) != 1 {
		t.Fatalf("policies = %+v, want one", network.Policies)
	}
	policy := network.Policies[0]
	if policy.Applies != "every pod in the namespace" {
		t.Errorf("applies = %q, want it said in words", policy.Applies)
	}
	if policy.Selects != 2 {
		t.Errorf("selects = %d, want both pods", policy.Selects)
	}
}

// TestAPolicyWithOnlyIngressRulesSaysSoAboutEgress.
//
// "We have a policy for that" rarely means inbound only, and a policy with no
// egress rules leaves outbound traffic entirely alone.
func TestAPolicyWithOnlyIngressRulesSaysSoAboutEgress(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := networkedCluster(t)

	network, err := client.Network(ctx, "")
	if err != nil {
		t.Fatalf("Network: %v", err)
	}
	for _, policy := range network.Policies {
		if policy.Name != "website-only" {
			continue
		}
		if !strings.Contains(policy.Summary, "Outbound traffic") {
			t.Errorf("summary = %q, want it to say egress is unrestricted", policy.Summary)
		}
		return
	}
	t.Fatal("website-only is not in the answer")
}

// TestTheDefaultIngressClassIsReported.
//
// An Ingress naming no class in a cluster with no default is an Ingress no
// controller will ever pick up, and it looks exactly like a working one.
func TestTheDefaultIngressClassIsReported(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := networkedCluster(t)

	network, err := client.Network(ctx, "")
	if err != nil {
		t.Fatalf("Network: %v", err)
	}
	if network.DefaultClass != "nginx" {
		t.Errorf("default class = %q, want nginx", network.DefaultClass)
	}
}

// TestAPlainClusterIPHasNothingExternal, which is the answer to "why can I not
// reach this from my laptop".
func TestAPlainClusterIPHasNothingExternal(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := networkedCluster(t)

	network, err := client.Network(ctx, "")
	if err != nil {
		t.Fatalf("Network: %v", err)
	}
	if got := servicesByName(t, network)["website"].External; got != "" {
		t.Errorf("external = %q, want nothing for a ClusterIP", got)
	}
}

// TestALoadBalancerWithNoAddressSaysItIsWaiting.
//
// Talos ships no load-balancer controller, so this is the ordinary state on this
// product's own target rather than an exotic one.
func TestALoadBalancerWithNoAddressSaysItIsWaiting(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Services: []kubesim.Service{
		{Namespace: "default", Name: "public", Type: "LoadBalancer", ClusterIP: "10.96.0.20"},
	}})

	network, err := client.Network(ctx, "")
	if err != nil {
		t.Fatalf("Network: %v", err)
	}
	if got := network.Services[0].External; got != "waiting for an address" {
		t.Errorf("external = %q, want it to say it is waiting", got)
	}
}
