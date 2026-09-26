package kube_test

import (
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// Acting as the operator (2026-09-19).
//
// The claim: a cluster's own audit log should record WHO acted, and its RBAC
// should be able to decide what they may do. Both are impossible while every
// request arrives as this product's certificate in system:masters.
//
// The hard part to test is that it is really happening, because a permissive
// cluster answers both ways. So the fake records the identity of every request.

// TestEveryCallArrivesAsTheOperatorWhenActingAsOne.
func TestEveryCallArrivesAsTheOperatorWhenActingAsOne(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, admin := newCluster(t, kubesim.Options{
		AllowImpersonated: []string{"admin@example.com"},
		Nodes:             []kubesim.Node{{Name: "cp-1", Ready: corev1.ConditionTrue}},
		Pods:              []kubesim.Pod{{Namespace: "default", Name: "api-1", Node: "cp-1"}},
	})

	// Before: the product's own certificate.
	if _, err := admin.Nodes(ctx); err != nil {
		t.Fatalf("Nodes as the product: %v", err)
	}
	if got := sim.Impersonated(); got[len(got)-1] != "" {
		t.Errorf("the plain client impersonated %q", got[len(got)-1])
	}

	as, err := admin.As(kube.Identity{User: "admin@example.com", Groups: []string{"platform"}})
	if err != nil {
		t.Fatalf("As: %v", err)
	}
	if as.Identity().User != "admin@example.com" {
		t.Errorf("identity = %+v", as.Identity())
	}

	before := len(sim.Impersonated())
	if _, err := as.Nodes(ctx); err != nil {
		t.Fatalf("Nodes as the operator: %v", err)
	}
	if _, err := as.Pods(ctx, "default"); err != nil {
		t.Fatalf("Pods as the operator: %v", err)
	}

	// Every request after the switch, not merely the first: a client that
	// impersonated some of its calls would produce an audit log that is right
	// about the reads and wrong about the writes.
	for i, who := range sim.Impersonated()[before:] {
		if who != "admin@example.com" {
			t.Errorf("request %d arrived as %q, want the operator", i, who)
		}
	}

	// And the original client is untouched, because a rest.Config is shared:
	// mutating it would change the identity of calls already in flight.
	if !admin.Identity().Empty() {
		t.Error("switching identity changed the client it was derived from")
	}
}

// TestARefusalIsNotRetriedAsTheAdmin is the rule the whole feature rests on.
//
// A fallback would make impersonation decorative: every request would succeed
// either way, the cluster's RBAC would decide nothing, and its audit log would
// show an admin action whenever somebody lacked a role. So a 403 comes back as
// a 403.
func TestARefusalIsNotRetriedAsTheAdmin(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, admin := newCluster(t, kubesim.Options{
		// The cluster knows nobody: every impersonated request is refused.
		AllowImpersonated: nil,
		Nodes:             []kubesim.Node{{Name: "cp-1", Ready: corev1.ConditionTrue}},
	})

	as, err := admin.As(kube.Identity{User: "stranger@example.com"})
	if err != nil {
		t.Fatalf("As: %v", err)
	}

	before := len(sim.Impersonated())
	if _, err := as.Nodes(ctx); err == nil {
		t.Fatal("a cluster that refused the identity answered anyway")
	}

	// Exactly one request, and it carried the identity. A retry would show up
	// here as a second entry with an empty user -- the admin sneaking in behind
	// a refusal.
	attempts := sim.Impersonated()[before:]
	if len(attempts) != 1 {
		t.Fatalf("attempts = %v, want one: a refusal must not be retried", attempts)
	}
	if attempts[0] != "stranger@example.com" {
		t.Errorf("attempt arrived as %q", attempts[0])
	}
	if slices.Contains(attempts, "") {
		t.Error("a request went out as this product's own certificate after a refusal")
	}
}

// TestTheClusterIsAskedWhatTheIdentityMayDo, so nobody turns this on and finds
// out during an incident.
func TestTheClusterIsAskedWhatTheIdentityMayDo(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, admin := newCluster(t, kubesim.Options{
		AllowImpersonated: []string{"reader@example.com"},
		DenyVerbs:         []string{"delete:pods", "patch:nodes"},
	})

	as, err := admin.As(kube.Identity{User: "reader@example.com"})
	if err != nil {
		t.Fatalf("As: %v", err)
	}

	answers, err := as.WhatMayI(ctx, kube.EveryPermissionThisProductUses("default"))
	if err != nil {
		t.Fatalf("WhatMayI: %v", err)
	}
	if len(answers) == 0 {
		t.Fatal("no permissions were checked")
	}

	byName := map[string]kube.Permission{}
	for _, a := range answers {
		byName[a.Verb+":"+a.Resource] = a
	}
	if byName["list:pods"].Allowed != true {
		t.Error("listing pods was refused, and this identity is allowed to")
	}
	if byName["delete:pods"].Allowed != false {
		t.Error("deleting pods was allowed; the cluster denies it")
	}
	// The authoriser's own sentence, rather than this product's guess about
	// somebody else's RBAC.
	if !strings.Contains(byName["delete:pods"].Reason, "reader@example.com") {
		t.Errorf("reason = %q, want the cluster's own", byName["delete:pods"].Reason)
	}

	// Asked as the PRODUCT, everything is allowed -- it is in system:masters.
	// A preview that said otherwise would lie about the state the cluster is in
	// before anybody switches impersonation on.
	plain, err := admin.WhatMayI(ctx, []kube.Permission{{Verb: "delete", Resource: "pods"}})
	if err != nil {
		t.Fatalf("WhatMayI as the product: %v", err)
	}
	if !plain[0].Allowed {
		t.Error("the product's own certificate was reported as unable to delete a pod")
	}
}

// TestAnEmptyIdentityChangesNothing: the zero value is what every call did
// before this existed, and a cluster that has not been set up for impersonation
// still needs it.
func TestAnEmptyIdentityChangesNothing(t *testing.T) {
	t.Parallel()

	_, admin := newCluster(t, kubesim.Options{})

	same, err := admin.As(kube.Identity{})
	if err != nil {
		t.Fatalf("As: %v", err)
	}
	if same != admin {
		t.Error("an empty identity built a new client; it has to be the same one")
	}
}

// TestTheManifestPathActsAsTheOperatorToo.
//
// The manifest path uses a different client -- the dynamic one, plus discovery
// -- so impersonating only the typed calls is an easy mistake to make and an
// invisible one to have: reads would be attributed to the person and WRITES to
// the product, which is the wrong half of an audit log to get right.
//
// Measured, not assumed: this test stayed green against exactly that fault
// until it was written.
func TestTheManifestPathActsAsTheOperatorToo(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, admin := newCluster(t, kubesim.Options{
		AllowImpersonated: []string{"admin@example.com"},
	})

	as, err := admin.As(kube.Identity{User: "admin@example.com"})
	if err != nil {
		t.Fatalf("As: %v", err)
	}

	manifest := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: settings\n  " +
		"namespace: default\ndata:\n  a: \"1\"\n")

	before := len(sim.Impersonated())
	if _, err := as.PlanManifest(ctx, manifest); err != nil {
		t.Fatalf("PlanManifest: %v", err)
	}
	if _, err := as.ApplyManifest(ctx, manifest); err != nil {
		t.Fatalf("ApplyManifest: %v", err)
	}

	after := sim.Impersonated()[before:]
	if len(after) == 0 {
		t.Fatal("the manifest path made no requests")
	}
	for i, who := range after {
		if who != "admin@example.com" {
			t.Errorf("manifest request %d arrived as %q -- discovery and the dynamic client have "+
				"to carry the identity as well", i, who)
		}
	}
}
