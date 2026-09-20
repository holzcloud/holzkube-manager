package kube_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// The resources beside the workloads (2026-09-19).
//
// Each of these exists to answer a question the workload list cannot, and the
// tests are written as those questions.

func resourceCluster(t *testing.T) (*kubesim.Server, *kube.Client) {
	t.Helper()

	return newCluster(t, kubesim.Options{Resources: []kubesim.Resource{
		{Kind: "ConfigMap", Namespace: "default", Name: "website-settings",
			Keys: []string{"theme", "locale"}},
		{Kind: "Secret", Namespace: "default", Name: "registry-pull",
			Keys: []string{".dockerconfigjson"}},
		{Kind: "PersistentVolumeClaim", Namespace: "db", Name: "data-postgres-0",
			Phase: "Pending", Size: "20Gi"},
		{Kind: "PersistentVolumeClaim", Namespace: "db", Name: "data-postgres-1",
			Phase: "Bound", Size: "20Gi"},
		{Kind: "Ingress", Namespace: "default", Name: "website",
			Host: "holzcloud.ch", Address: ""},
		{Kind: "HorizontalPodAutoscaler", Namespace: "default", Name: "website",
			Current: 2, Min: 2, Max: 8, TargetKind: "Deployment", TargetName: "holzcloud-website"},
		{Kind: "PodDisruptionBudget", Namespace: "longhorn-system", Name: "longhorn",
			Healthy: 1, Desired: 2, Allowed: 0},
	}})
}

// TestASecretsValuesNeverLeaveTheCluster.
//
// The fake stores a real-looking value for every key, precisely so this can
// watch it not appear. Names matter -- "does this namespace have the pull
// secret" is a real question -- and the contents are refused everywhere else in
// this product, so a list that carried them would be the one place they leaked.
func TestASecretsValuesNeverLeaveTheCluster(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := resourceCluster(t)

	resources, err := client.Resources(ctx, "")
	if err != nil {
		t.Fatalf("Resources: %v", err)
	}

	var secret kube.Resource
	for _, r := range resources {
		if r.Kind == "Secret" {
			secret = r
		}
		// Nowhere, not merely in the Secret's own row.
		for _, field := range []string{r.Summary, r.Detail, r.Name} {
			if strings.Contains(field, "super-secret-value") {
				t.Fatalf("a secret value appeared in %s %s/%s: %q",
					r.Kind, r.Namespace, r.Name, field)
			}
		}
	}

	if secret.Name != "registry-pull" {
		t.Fatalf("the secret was not listed at all: %+v", secret)
	}
	// The key NAMES are the answer to the question people ask.
	if !strings.Contains(secret.Detail, ".dockerconfigjson") {
		t.Errorf("detail = %q, want the key names", secret.Detail)
	}
}

// TestTheObjectThatIsStuckIsMarkedAsSuch.
func TestTheObjectThatIsStuckIsMarkedAsSuch(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := resourceCluster(t)

	resources, err := client.Resources(ctx, "")
	if err != nil {
		t.Fatalf("Resources: %v", err)
	}
	// Keyed by KIND and name: an Ingress and an autoscaler for the same
	// workload share a name, and keying by name alone silently kept one of
	// them. The test caught it, which is the reason this is spelled out.
	byName := map[string]kube.Resource{}
	for _, r := range resources {
		byName[r.Kind+"/"+r.Name] = r
	}

	// A Pending claim is why a StatefulSet will not start, and the pod's own
	// events only say "unbound claim".
	if byName["PersistentVolumeClaim/data-postgres-0"].Healthy {
		t.Error("a Pending claim was reported as healthy")
	}
	if !byName["PersistentVolumeClaim/data-postgres-1"].Healthy {
		t.Error("a Bound claim was reported as unhealthy")
	}

	// An Ingress with no address is one no controller has claimed, which is the
	// whole "why does this URL not work" case.
	ingress := byName["Ingress/website"]
	if ingress.Healthy || !strings.Contains(ingress.Summary, "no controller") {
		t.Errorf("ingress = %+v, want it named as unclaimed", ingress)
	}

	// A budget allowing zero disruptions is why a drain refuses, and saying so
	// before somebody starts one is the other half of the drain's refusal.
	budget := byName["PodDisruptionBudget/longhorn"]
	if budget.Healthy {
		t.Error("a budget allowing no disruption was reported as healthy")
	}
	if !strings.Contains(budget.Detail, "drain") {
		t.Errorf("budget detail = %q, want it to connect this to a refused drain", budget.Detail)
	}
}

// TestAnAutoscalerNamesWhatItControls, because it is the reason a replica count
// goes back after somebody scales by hand -- which otherwise looks like this
// product ignoring a click.
func TestAnAutoscalerNamesWhatItControls(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := resourceCluster(t)

	resources, err := client.Resources(ctx, "default")
	if err != nil {
		t.Fatalf("Resources: %v", err)
	}
	for _, r := range resources {
		if r.Kind != "HorizontalPodAutoscaler" {
			continue
		}
		if !strings.Contains(r.Detail, "holzcloud-website") {
			t.Errorf("detail = %q, want the workload whose replicas it sets", r.Detail)
		}
		if !strings.Contains(r.Summary, "between 2 and 8") {
			t.Errorf("summary = %q, want the range it keeps", r.Summary)
		}
		return
	}
	t.Fatal("no autoscaler was listed")
}

// TestDeletingANamespaceIsRefused: it removes everything inside it,
// asynchronously, with nothing that stops it once it starts.
func TestDeletingANamespaceIsRefused(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := resourceCluster(t)

	err := client.DeleteObject(ctx, "v1", "Namespace", "", "db")
	if !errors.Is(err, kube.ErrRefusedKind) {
		t.Fatalf("err = %v, want ErrRefusedKind", err)
	}
	if !strings.Contains(err.Error(), "everything inside it") {
		t.Errorf("reason = %q, want it to say what a namespace deletion does", err)
	}
	if len(sim.Deleted()) != 0 {
		t.Errorf("something was deleted anyway: %v", sim.Deleted())
	}
}

// TestDeletingOneObjectDeletesThatObject.
func TestDeletingOneObjectDeletesThatObject(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := resourceCluster(t)

	if err := client.DeleteObject(ctx, "policy/v1", "PodDisruptionBudget",
		"longhorn-system", "longhorn"); err != nil {
		t.Fatalf("DeleteObject: %v", err)
	}
	got := sim.Deleted()
	if len(got) != 1 || got[0] != "poddisruptionbudgets/longhorn-system/longhorn" {
		t.Errorf("deleted = %v, want exactly the one object", got)
	}

	// A kind the cluster does not have is refused by name rather than with a
	// stack of discovery words.
	if err := client.DeleteObject(ctx, "cert-manager.io/v1", "Certificate", "default",
		"tls"); !errors.Is(err, kube.ErrManifestInvalid) {
		t.Errorf("an unknown kind = %v, want ErrManifestInvalid", err)
	}
}

// A single copy is not a finding (2026-09-20).
//
// The operator photographed /kubernetes/config: six PodDisruptionBudgets, every
// one of them amber, every one saying "1 of 1 healthy, 0 may be disrupted", and
// the ConfigMaps and Ingresses the page exists for pushed below them by the
// unhealthy-first sort.
//
// The numbers were right and the verdict was wrong. `DisruptionsAllowed == 0` on
// a workload with ONE copy is the ordinary, correct state of every
// single-replica database in every cluster -- it says a drain cannot move it
// without downtime, which the drain path already says at the moment it matters.
// Flagging it permanently is the failure this codebase names elsewhere in its own
// words: a warning everybody sees is a warning nobody reads. It had already been
// reasoned through correctly for RBAC (wildcards across separate rules are
// ordinary) and got wrong here.
//
// The actual finding is the budget NOT BEING MET: fewer healthy pods than it
// wants, which means one is already missing.

func budgetsNamed(t *testing.T, resources []kube.Resource) map[string]kube.Resource {
	t.Helper()

	out := map[string]kube.Resource{}
	for _, r := range resources {
		if r.Kind == "PodDisruptionBudget" {
			out[r.Name] = r
		}
	}
	return out
}

// TestASingleCopyIsNotAFinding.
func TestASingleCopyIsNotAFinding(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Resources: []kubesim.Resource{
		// The operator's case: one replica, budget met, nothing may be disrupted.
		{Kind: "PodDisruptionBudget", Namespace: "cloud", Name: "nextcloud-db-primary",
			Healthy: 1, Desired: 1, Allowed: 0},
		// Met, with room to spare.
		{Kind: "PodDisruptionBudget", Namespace: "web", Name: "website",
			Healthy: 3, Desired: 2, Allowed: 1},
		// NOT met: a pod is already missing. This is the finding.
		{Kind: "PodDisruptionBudget", Namespace: "db", Name: "postgres",
			Healthy: 1, Desired: 2, Allowed: 0},
	}})

	resources, err := client.Resources(ctx, "")
	if err != nil {
		t.Fatalf("Resources: %v", err)
	}
	found := budgetsNamed(t, resources)

	if !found["nextcloud-db-primary"].Healthy {
		t.Errorf("a single-replica budget that is MET is reported as a problem: %q / %q",
			found["nextcloud-db-primary"].Summary, found["nextcloud-db-primary"].Detail)
	}
	if !found["website"].Healthy {
		t.Error("a budget with room to spare is reported as a problem")
	}
	if found["postgres"].Healthy {
		t.Error("a budget that is not met is reported as healthy")
	}
	if !strings.Contains(found["postgres"].Detail, "already missing") {
		t.Errorf("detail = %q, want it to say a pod is missing", found["postgres"].Detail)
	}
}

// TestASingleCopyStillSaysWhatADrainWillDo.
//
// Not a finding is not the same as not worth saying: somebody about to drain a
// node needs to know that this one cannot be moved without downtime. The
// difference is that it is stated rather than flagged.
func TestASingleCopyStillSaysWhatADrainWillDo(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Resources: []kubesim.Resource{
		{Kind: "PodDisruptionBudget", Namespace: "cloud", Name: "nextcloud-db-primary",
			Healthy: 1, Desired: 1, Allowed: 0},
		{Kind: "PodDisruptionBudget", Namespace: "web", Name: "website",
			Healthy: 3, Desired: 2, Allowed: 1},
	}})

	resources, err := client.Resources(ctx, "")
	if err != nil {
		t.Fatalf("Resources: %v", err)
	}
	found := budgetsNamed(t, resources)

	if !strings.Contains(found["nextcloud-db-primary"].Detail, "drain") {
		t.Errorf("detail = %q, want it to say what a drain would do",
			found["nextcloud-db-primary"].Detail)
	}
	// And nothing said about the one that can be disrupted: there is nothing to
	// warn about, and a line on every row is a line nobody reads.
	if found["website"].Detail != "" {
		t.Errorf("detail = %q, want nothing said about a budget with room",
			found["website"].Detail)
	}
}
