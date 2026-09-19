package kube_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// Applying manifests (milestone v1.17, slice 5).
//
// The tests here are about the two claims the screen makes: that the plan says
// what an apply would do, and that an apply which hit somebody else's field
// manager says so instead of winning.

// TestThePlanAsksTheClusterWhatExists is why there is a plan at all.
//
// "Create" and "update" cannot be read off a manifest -- the manifest looks
// identical either way -- so the product asks the cluster, one object at a time.
// A plan that guessed would tell an operator they are creating something while
// it overwrites what is there.
func TestThePlanAsksTheClusterWhatExists(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{
		Objects: []string{"configmaps/default/already-there"},
	})

	manifest := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: already-there
  namespace: default
data:
  tuned: "yes"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: brand-new
  namespace: default
data:
  tuned: "yes"
`)

	plan, err := client.PlanManifest(ctx, manifest)
	if err != nil {
		t.Fatalf("PlanManifest: %v", err)
	}
	if len(plan.Objects) != 2 {
		t.Fatalf("plan = %+v, want both documents", plan.Objects)
	}
	if plan.Objects[0].Action != "update" {
		t.Errorf("an object the cluster holds planned as %q, want update", plan.Objects[0].Action)
	}
	if plan.Objects[1].Action != "create" {
		t.Errorf("an object the cluster does not hold planned as %q, want create",
			plan.Objects[1].Action)
	}

	// And the plan changed nothing: it read, and it did not apply. A preview that
	// wrote would be the worst kind of preview.
	if got := sim.FieldManagers(); len(got) != 0 {
		t.Errorf("the plan applied something: %v", got)
	}
}

// TestAnObjectWithNoNamespaceIsWarnedAbout: a namespaced object that names no
// namespace lands in "default", and finding that out afterwards is how a
// manifest meant for one cluster's namespace ends up in another.
func TestAnObjectWithNoNamespaceIsWarnedAbout(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{})

	plan, err := client.PlanManifest(ctx, []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: settings
`))
	if err != nil {
		t.Fatalf("PlanManifest: %v", err)
	}
	if len(plan.Objects) != 1 || plan.Objects[0].Namespace != "default" {
		t.Fatalf("plan = %+v, want the namespace it would land in", plan.Objects)
	}
	if !plan.Objects[0].Namespaced {
		t.Error("a ConfigMap was planned as cluster-scoped; the scope comes from discovery")
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "default") {
		t.Errorf("warnings = %v, want one that names the namespace it would land in", plan.Warnings)
	}
}

// TestAClusterScopedObjectGoesToTheClusterScopedPath: the scope comes from the
// cluster's discovery, and getting it wrong sends the object to a path that
// answers 404.
func TestAClusterScopedObjectGoesToTheClusterScopedPath(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{})

	manifest := []byte(`apiVersion: v1
kind: Namespace
metadata:
  name: web
`)

	plan, err := client.PlanManifest(ctx, manifest)
	if err != nil {
		t.Fatalf("PlanManifest: %v", err)
	}
	if len(plan.Objects) != 1 || plan.Objects[0].Namespaced {
		t.Fatalf("plan = %+v, want one cluster-scoped object", plan.Objects)
	}
	if plan.Objects[0].Namespace != "" {
		t.Errorf("namespace = %q for a cluster-scoped object", plan.Objects[0].Namespace)
	}
	if len(plan.Warnings) != 0 {
		t.Errorf("warnings = %v; a cluster-scoped object needs no namespace", plan.Warnings)
	}

	if _, err := client.ApplyManifest(ctx, manifest); err != nil {
		t.Fatalf("ApplyManifest: %v", err)
	}
	if sim.Applied("namespaces", "", "web") == nil {
		t.Error("the namespace did not reach the cluster-scoped path")
	}
}

// TestAnApplyIsProvenByWhatTheClusterThenHolds, and by the name it acted under.
//
// The field manager is the point of server-side apply: the cluster records who
// set each field, so `kubectl get -o yaml --show-managed-fields` can say this
// product did. An apply under client-go's default name would make that log say
// nothing.
func TestAnApplyIsProvenByWhatTheClusterThenHolds(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{})

	result, err := client.ApplyManifest(ctx, []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: settings
  namespace: default
data:
  tuned: "yes"
`))
	if err != nil {
		t.Fatalf("ApplyManifest: %v", err)
	}
	if len(result.Applied) != 1 || len(result.Failed) != 0 {
		t.Fatalf("result = %+v", result)
	}

	object := sim.Applied("configmaps", "default", "settings")
	if object == nil {
		t.Fatal("the object is not in the cluster: an apply that did not arrive looks exactly " +
			"like one that did, from the client's side")
	}
	data, _ := object["data"].(map[string]any)
	if data["tuned"] != "yes" {
		t.Errorf("the cluster holds data %v, want the manifest's own", object["data"])
	}

	// The literal name, not the constant: comparing the constant with itself
	// would pass whatever it was renamed to, and the point is that a cluster's
	// audit log says this product acted rather than "kubectl".
	if got := sim.FieldManagers(); !slices.Equal(got, []string{"holzkube-manager"}) {
		t.Errorf("field managers = %v, want holzkube-manager: the cluster's record of who set a "+
			"field is the whole reason this is server-side apply", got)
	}
	if kube.FieldManager != "holzkube-manager" {
		t.Errorf("FieldManager = %q; renaming it renames what every cluster's managedFields "+
			"already says, so it is pinned here", kube.FieldManager)
	}
}

// TestAConflictIsReportedRatherThanForced is the refusal this slice exists for.
//
// A conflict means another field manager -- usually a controller -- owns a field
// this apply sets. Forcing takes the value away from whatever is managing it,
// which will set it back: a fight this product must not start on its own. So the
// conflict comes back as something to read, with the other objects still
// applied, and `force` is never sent.
func TestAConflictIsReportedRatherThanForced(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{
		ConflictOn: []string{"configmaps/default/contested"},
	})

	result, err := client.ApplyManifest(ctx, []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: contested
  namespace: default
data:
  tuned: "yes"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: uncontested
  namespace: default
data:
  tuned: "yes"
`))
	if !errors.Is(err, kube.ErrManifestInvalid) {
		t.Fatalf("err = %v, want the apply to report that something did not apply", err)
	}
	if len(result.Failed) != 1 || result.Failed[0].Object.Name != "contested" {
		t.Fatalf("failed = %+v, want the contested object", result.Failed)
	}
	if !strings.Contains(result.Failed[0].Reason, "field manager") {
		t.Errorf("reason = %q, want it to say what a conflict is", result.Failed[0].Reason)
	}

	// The other object was applied: an apply of ten that gave up on the sixth
	// would leave five written and say nothing about which.
	if len(result.Applied) != 1 || result.Applied[0].Name != "uncontested" {
		t.Errorf("applied = %+v, want the object that had no conflict", result.Applied)
	}
	if sim.Applied("configmaps", "default", "uncontested") == nil {
		t.Error("the uncontested object did not reach the cluster")
	}

	if got := sim.Forced(); len(got) != 0 {
		t.Errorf("the apply forced its way past another field manager: %v", got)
	}
	if sim.Applied("configmaps", "default", "contested") != nil {
		t.Error("the contested object was written anyway")
	}
}

// TestAKindTheClusterDoesNotHaveIsRefusedByName: the mapping comes from the
// cluster, so a manifest for a CustomResourceDefinition this cluster has not
// installed is refused with the kind in the message rather than with a stack of
// discovery words.
func TestAKindTheClusterDoesNotHaveIsRefusedByName(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{})

	plan, err := client.PlanManifest(ctx, []byte(`apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: tls
  namespace: default
`))
	if err != nil {
		t.Fatalf("PlanManifest: %v", err)
	}
	if len(plan.Objects) != 0 {
		t.Errorf("objects = %+v, want none: the cluster cannot take this kind", plan.Objects)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "Certificate") {
		t.Fatalf("warnings = %v, want one that names the kind", plan.Warnings)
	}
}

// TestAManifestThatIsNotOneIsRefusedBeforeAnythingIsApplied: an empty paste, a
// document with no name, YAML that is not YAML. All three are mistakes somebody
// makes on this screen, and none of them may half-apply.
func TestAManifestThatIsNotOneIsRefusedBeforeAnythingIsApplied(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{})

	for _, tc := range []struct {
		name     string
		manifest string
	}{
		{"empty", "   \n"},
		{"only separators", "---\n---\n"},
		{"not yaml", "kind: ConfigMap\n\tname: broken\n"},
	} {
		if _, err := client.PlanManifest(ctx, []byte(tc.manifest)); !errors.Is(err, kube.ErrManifestInvalid) {
			t.Errorf("%s: err = %v, want ErrManifestInvalid", tc.name, err)
		}
		if _, err := client.ApplyManifest(ctx, []byte(tc.manifest)); !errors.Is(err, kube.ErrManifestInvalid) {
			t.Errorf("%s: apply err = %v, want ErrManifestInvalid", tc.name, err)
		}
	}

	// A document with a kind and no name is refused per object, because the rest
	// of the manifest is still applicable and stopping would be worse.
	result, err := client.ApplyManifest(ctx, []byte(`apiVersion: v1
kind: ConfigMap
data:
  tuned: "yes"
`))
	if !errors.Is(err, kube.ErrManifestInvalid) {
		t.Fatalf("err = %v, want a refusal for a document with no name", err)
	}
	if len(result.Failed) != 1 {
		t.Errorf("failed = %+v", result.Failed)
	}
	if got := sim.FieldManagers(); len(got) != 0 {
		t.Errorf("something was applied from an invalid manifest: %v", got)
	}
}

// TestAManifestWithMoreObjectsThanTheCapIsRefusedBeforeAnyCall: the plan asks
// the cluster about every object and the apply writes every one, both serially,
// so the number of documents decides how much upstream work one request does.
// The refusal has to come before the first call, or the cap would not be a cap.
func TestAManifestWithMoreObjectsThanTheCapIsRefusedBeforeAnyCall(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{})

	var b strings.Builder
	for i := 0; i <= kube.MaxManifestObjects; i++ {
		fmt.Fprintf(&b, "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: c%d\n  namespace: default\n---\n", i)
	}

	_, err := client.PlanManifest(ctx, []byte(b.String()))
	if !errors.Is(err, kube.ErrManifestInvalid) {
		t.Fatalf("err = %v, want ErrManifestInvalid", err)
	}
	if !strings.Contains(err.Error(), "at most") {
		t.Errorf("reason = %q, want it to say what the limit is", err)
	}
	if got := sim.Calls("GET /api"); got != 0 {
		t.Errorf("the cluster was asked %d times before the refusal; a cap that spends the "+
			"request's budget first is not a cap", got)
	}
}

// TestPlanningManyObjectsIsNotThrottledByThisProcess.
//
// client-go rate-limits itself: 5 requests a second by default, burst 10. A plan
// asks the cluster about every object in turn, so that default turns a manifest
// of the size this route accepts into a request that spends its whole budget
// waiting in a queue INSIDE this process -- and then reports the cluster as
// slow. Measured before it was fixed: 20 objects took 2 seconds against a fake
// on localhost.
//
// The bound here is loose on purpose. It is not a performance target; it is the
// difference between "the calls are made" and "the calls are queued behind a
// limiter nobody set deliberately".
func TestPlanningManyObjectsIsNotThrottledByThisProcess(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{})

	const objects = 60
	var b strings.Builder
	for i := range objects {
		fmt.Fprintf(&b, "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: c%d\n  namespace: default\n---\n", i)
	}

	start := time.Now()
	plan, err := client.PlanManifest(ctx, []byte(b.String()))
	took := time.Since(start)
	if err != nil {
		t.Fatalf("PlanManifest: %v", err)
	}
	if len(plan.Objects) != objects {
		t.Fatalf("planned %d objects, want %d", len(plan.Objects), objects)
	}

	// The default limiter would need about ten seconds for these: 60 requests at
	// 5 a second, less the burst.
	if took > 5*time.Second {
		t.Errorf("planning %d objects took %v. The default client-go rate limit would explain "+
			"exactly this, and a cluster would be blamed for it", objects, took)
	}
}
