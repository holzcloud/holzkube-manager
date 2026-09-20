package kube_test

import (
	"sort"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// Namespaces, quotas and the cluster's own kinds (2026-09-20).
//
// Two findings here, and both read as a fault of the workload rather than of the
// namespace: a namespace stuck Terminating, and a quota that is full. The pod's
// own event says "exceeded quota" in a namespace whose quota nothing showed.

func inventoryCluster(t *testing.T) (*kubesim.Server, *kube.Client) {
	t.Helper()

	return newCluster(t, kubesim.Options{
		Namespaces: []string{"default", "web", "db", "ci", "monitoring"},
		NamespaceFixtures: []kubesim.NamespaceFixture{
			{
				Name: "old-staging", Terminating: true,
				Holding:    "Some content in the namespace has finalizers remaining: volumes.longhorn.io in 3 resource instances",
				Finalizers: []string{"kubernetes"},
			},
		},
		Quotas: []kubesim.Quota{
			// Full: why the next pod there is refused.
			{Namespace: "ci", Name: "build-quota",
				Hard: map[string]string{"cpu": "4", "memory": "8Gi"},
				Used: map[string]string{"cpu": "4", "memory": "6Gi"}},
			// A compute quota with no LimitRange: every pod without requests is
			// refused, and the error names the pod.
			{Namespace: "web", Name: "web-quota",
				Hard: map[string]string{"requests.cpu": "2"},
				Used: map[string]string{"requests.cpu": "500m"}},
			// Object counts only: no LimitRange warning belongs here.
			{Namespace: "db", Name: "object-quota",
				Hard: map[string]string{"count/pods": "10"},
				Used: map[string]string{"count/pods": "3"}},
		},
		LimitRanges: []kubesim.LimitRange{{Namespace: "ci", Name: "defaults"}},
		Pods: []kubesim.Pod{
			{Namespace: "web", Name: "website-abc", Phase: corev1.PodRunning},
			{Namespace: "ci", Name: "runner-1", Phase: corev1.PodRunning},
			// Finished: it holds nothing and should not be counted.
			{Namespace: "ci", Name: "runner-0", Phase: corev1.PodSucceeded},
		},
		CustomResourceDefinitions: []kubesim.CustomResourceDefinition{
			{
				Group: "longhorn.io", Kind: "Volume", Scope: "Namespaced",
				Versions: []string{"v1beta2"}, Stored: "v1beta2",
				NotServed: []string{"v1beta1"}, Established: true,
			},
			{
				// Not established: every manifest naming it is refused, and
				// nothing else in a cluster says so.
				Group: "example.invalid", Kind: "Widget", Scope: "Cluster",
				Versions: []string{"v1"}, Stored: "v1", Established: false,
			},
		},
	})
}

func namespacesByName(inventory kube.Inventory) map[string]kube.NamespaceSummary {
	out := map[string]kube.NamespaceSummary{}
	for _, namespace := range inventory.Namespaces {
		out[namespace.Name] = namespace
	}
	return out
}

// TestAStuckNamespaceSaysWhatIsHoldingIt.
//
// `kubectl get ns` shows the word Terminating and nothing about which resources
// are keeping it. The namespace hangs for weeks, the name cannot be reused, and
// every attempt to recreate it fails with "already exists".
func TestAStuckNamespaceSaysWhatIsHoldingIt(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := inventoryCluster(t)

	inventory, err := client.Inventory(ctx)
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	stuck := namespacesByName(inventory)["old-staging"]

	if stuck.Phase != "Terminating" {
		t.Fatalf("phase = %q, want Terminating", stuck.Phase)
	}
	if stuck.Healthy {
		t.Error("a namespace that will not go is reported as healthy")
	}
	// The condition message, which is the answer rather than a restatement.
	if !strings.Contains(stuck.Notice, "volumes.longhorn.io") {
		t.Errorf("notice = %q, want it to name what is holding the namespace", stuck.Notice)
	}
	if !strings.Contains(stuck.Notice, "cannot be reused") {
		t.Errorf("notice = %q, want it to say what this prevents", stuck.Notice)
	}
}

// TestAFullQuotaIsNamedAsTheReasonTheNextThingIsRefused.
func TestAFullQuotaIsNamedAsTheReasonTheNextThingIsRefused(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := inventoryCluster(t)

	inventory, err := client.Inventory(ctx)
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	ci := namespacesByName(inventory)["ci"]

	if ci.Healthy {
		t.Error("a namespace whose quota is full is reported as healthy")
	}
	// cpu is at 4 of 4; memory at 6 of 8 is not full and must not be named.
	if !strings.Contains(ci.Notice, "cpu") {
		t.Errorf("notice = %q, want it to name cpu", ci.Notice)
	}
	if strings.Contains(ci.Notice, "memory") {
		t.Errorf("notice = %q, names memory, which is at 6 of 8", ci.Notice)
	}
	// And why the operator has not seen this: the refusal lands on the workload.
	if !strings.Contains(ci.Notice, "exceeded quota") {
		t.Errorf("notice = %q, want it to say where the refusal appears", ci.Notice)
	}
}

// TestAComputeQuotaWithNoLimitRangeIsTheCombinationNobodyExpects.
//
// Every pod that does not set requests is refused -- not for being too big, but
// because the quota cannot account for a pod that asked for nothing. The error
// says "must specify limits", which reads as the pod being wrong.
func TestAComputeQuotaWithNoLimitRangeIsTheCombinationNobodyExpects(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := inventoryCluster(t)

	inventory, err := client.Inventory(ctx)
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	found := namespacesByName(inventory)

	web := found["web"]
	if web.Healthy {
		t.Error("a namespace with a compute quota and no LimitRange is reported as healthy")
	}
	if !strings.Contains(web.Notice, "does not set requests") {
		t.Errorf("notice = %q, want it to say which pods are refused", web.Notice)
	}

	// A quota on OBJECT COUNTS does not do this, and warning about it would be a
	// warning on most namespaces that have a quota at all.
	db := found["db"]
	if !db.Healthy {
		t.Errorf("a namespace whose quota only counts objects is reported as a problem: %q",
			db.Notice)
	}

	// And a compute quota WITH a LimitRange is not this finding -- ci is unhealthy
	// for being full, not for this.
	if strings.Contains(found["ci"].Notice, "does not set requests") {
		t.Error("a namespace with a LimitRange got the no-LimitRange warning")
	}
}

// TestAnEmptyNamespaceIsVisibleAsEmpty, rather than as a name.
func TestAnEmptyNamespaceIsVisibleAsEmpty(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := inventoryCluster(t)

	inventory, err := client.Inventory(ctx)
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	found := namespacesByName(inventory)

	if got := found["monitoring"].PodsRunning; got != 0 {
		t.Errorf("monitoring has %d pods, want none", got)
	}
	// A finished pod holds nothing, so it is not in the count.
	if got := found["ci"].PodsRunning; got != 1 {
		t.Errorf("ci has %d pods running, want 1 -- the Succeeded one is not running", got)
	}
}

// TestTheQuotaLinesComeBackSorted.
//
// Built from a map, whose order Go randomises, so without sorting the screen
// shuffles on every refresh and nothing is findable twice.
//
// Written as "sorted" rather than "the same across several reads", and that was
// measured: the stability version passed with the sort REMOVED. With two resources
// a randomised order repeats across five reads about one time in sixteen, so the
// test was a coin flip pretending to be a guard. Sorted is the property, and one
// read decides it.
func TestTheQuotaLinesComeBackSorted(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{
		Namespaces: []string{"ci"},
		Quotas: []kubesim.Quota{{
			Namespace: "ci", Name: "build-quota",
			// Five resources, deliberately written in a jumbled order: with two
			// the assertion would hold by chance half the time.
			Hard: map[string]string{
				"pods": "20", "cpu": "4", "memory": "8Gi",
				"services": "5", "configmaps": "30",
			},
			Used: map[string]string{
				"pods": "3", "cpu": "1", "memory": "1Gi",
				"services": "2", "configmaps": "4",
			},
		}},
		LimitRanges: []kubesim.LimitRange{{Namespace: "ci", Name: "defaults"}},
	})

	inventory, err := client.Inventory(ctx)
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	lines := namespacesByName(inventory)["ci"].Quotas
	if len(lines) != 5 {
		t.Fatalf("quota lines = %+v, want five", lines)
	}

	order := make([]string, 0, len(lines))
	for _, line := range lines {
		order = append(order, line.Resource)
	}
	if !sort.StringsAreSorted(order) {
		t.Errorf("quota lines come back as %v, which is not sorted -- the screen would shuffle "+
			"on every refresh", order)
	}
}

// TestACustomKindThatIsNotEstablishedIsNamed.
//
// Every manifest naming it is refused, and nothing else in a cluster says so.
func TestACustomKindThatIsNotEstablishedIsNamed(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := inventoryCluster(t)

	inventory, err := client.Inventory(ctx)
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}

	byKind := map[string]kube.CustomKind{}
	for _, kind := range inventory.CustomKinds {
		byKind[kind.Kind] = kind
	}

	widget := byKind["Widget"]
	if widget.Established {
		t.Error("a definition with no Established condition is reported as established")
	}
	if !strings.Contains(widget.Notice, "refused") {
		t.Errorf("notice = %q, want it to say what this prevents", widget.Notice)
	}

	volume := byKind["Volume"]
	if !volume.Established {
		t.Error("an established definition is not reported as established")
	}
	if volume.Notice != "" {
		t.Errorf("notice = %q, want nothing said about a working kind", volume.Notice)
	}
}

// TestAVersionTheApiServerDoesNotServeIsNotOffered.
//
// Listing it would send somebody to a version every request for which is refused.
func TestAVersionTheApiServerDoesNotServeIsNotOffered(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := inventoryCluster(t)

	inventory, err := client.Inventory(ctx)
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	for _, kind := range inventory.CustomKinds {
		if kind.Kind != "Volume" {
			continue
		}
		if len(kind.Versions) != 1 || kind.Versions[0] != "v1beta2" {
			t.Errorf("versions = %v, want only the served one", kind.Versions)
		}
		if kind.Stored != "v1beta2" {
			t.Errorf("stored = %q, want v1beta2", kind.Stored)
		}
		if kind.Group != "longhorn.io" || kind.Scope != "Namespaced" {
			t.Errorf("group/scope = %q/%q, want longhorn.io/Namespaced", kind.Group, kind.Scope)
		}
		return
	}
	t.Fatal("the Volume kind is not in the answer")
}

// TestAClusterWithNoCustomKindsAnswersNoneRatherThanFailing.
//
// A cluster may not serve apiextensions to this identity at all, and that is not a
// failure of the namespace screen.
func TestAClusterWithNoCustomKindsAnswersNoneRatherThanFailing(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{})

	inventory, err := client.Inventory(ctx)
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if len(inventory.CustomKinds) != 0 {
		t.Errorf("custom kinds = %+v, want none", inventory.CustomKinds)
	}
	// And the namespaces every cluster has are still answered.
	if len(inventory.Namespaces) == 0 {
		t.Error("no namespaces at all, which no cluster has")
	}
}
