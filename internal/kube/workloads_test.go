package kube_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// TestRestartingAPodNothingOwnsIsRefused is the refusal this slice exists for.
//
// "Restart" is the word an operator uses and Kubernetes has no such verb: it
// has "delete the pod and let the controller make another". Those are the same
// operation exactly when a controller owns the pod, and when nothing does, the
// second is deletion. A product that deleted a bare pod because somebody
// clicked restart would have destroyed something on the strength of a word.
func TestRestartingAPodNothingOwnsIsRefused(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{Pods: []kubesim.Pod{
		{Namespace: "default", Name: "api-1", Node: "cp-1", OwnerKind: "ReplicaSet"},
		{Namespace: "default", Name: "debug", Node: "cp-1"},
	}})

	err := client.RestartPod(ctx, "default", "debug")
	if !errors.Is(err, kube.ErrNothingWouldRecreateIt) {
		t.Fatalf("restarting a bare pod = %v, want ErrNothingWouldRecreateIt", err)
	}
	if len(sim.Deleted()) != 0 {
		t.Errorf("the pod was deleted anyway: %v", sim.Deleted())
	}

	// And the ordinary case works, or the refusal above would be the only
	// behaviour and this would be a product that cannot restart anything.
	if err := client.RestartPod(ctx, "default", "api-1"); err != nil {
		t.Fatalf("restarting a pod a ReplicaSet owns: %v", err)
	}
	if got := sim.Deleted(); len(got) != 1 || got[0] != "default/api-1" {
		t.Errorf("deleted = %v, want the one pod a controller will replace", got)
	}
	if len(sim.Evicted()) != 0 {
		t.Error("a restart went through the eviction API; a restart is a delete, and a test that " +
			"accepted either would not be able to tell them apart")
	}
}

// TestRestartingAPodThatIsNotThereSaysSo: a pod somebody restarted twice, or
// one a controller already replaced, is not an internal error.
func TestRestartingAPodThatIsNotThereSaysSo(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{})

	err := client.RestartPod(ctx, "default", "ghost")
	if !errors.Is(err, kube.ErrNoSuchWorkload) {
		t.Fatalf("err = %v, want ErrNoSuchWorkload", err)
	}
}

// TestScalingADeploymentChangesWhatTheNextReadSays: through the scale
// subresource, which is what kubectl scale uses -- a client that patched the
// whole deployment could undo a field somebody else had just set.
func TestScalingADeploymentChangesWhatTheNextReadSays(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{Deployments: []kubesim.Deployment{
		{Namespace: "default", Name: "api", Desired: 1, Ready: 1, Image: "example/api:1.4"},
	}})

	before, err := client.Deployments(ctx, "")
	if err != nil {
		t.Fatalf("Deployments: %v", err)
	}
	if len(before) != 1 || before[0].Desired != 1 || before[0].Image != "example/api:1.4" {
		t.Fatalf("deployments = %+v", before)
	}

	if err := client.Scale(ctx, "default", "api", 3); err != nil {
		t.Fatalf("Scale: %v", err)
	}
	if got := sim.DesiredReplicas("default", "api"); got != 3 {
		t.Errorf("the server holds %d replicas, want 3: a scale that did not reach the cluster "+
			"would look exactly like one that did", got)
	}

	after, err := client.Deployments(ctx, "")
	if err != nil {
		t.Fatalf("Deployments: %v", err)
	}
	if after[0].Desired != 3 {
		t.Errorf("desired = %d after scaling to 3", after[0].Desired)
	}
	// Desired and ready are both carried, for the reason a pod carries phase
	// and readiness: three asked for and one running is the state somebody is
	// on this screen about.
	if after[0].Ready != 1 {
		t.Errorf("ready = %d, want the fact rather than the intention", after[0].Ready)
	}
}

// TestScalingSomethingThatIsNotThereSaysSo, and TestANegativeCountIsRefused:
// two refusals that are cheap and stop a confusing answer from the API server.
func TestScalingSomethingThatIsNotThereSaysSo(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{})

	if err := client.Scale(ctx, "default", "ghost", 2); !errors.Is(err, kube.ErrNoSuchWorkload) {
		t.Errorf("err = %v, want ErrNoSuchWorkload", err)
	}
	if err := client.Scale(ctx, "default", "api", -1); err == nil {
		t.Error("a negative replica count was accepted")
	}
}

// TestARolloutRestartRollsRatherThanDeleting is the difference between the two
// restarts, and the reason both exist.
//
// Deleting a deployment's pods takes the workload down. A rollout restart
// annotates the pod template, which changes its hash, which makes the
// deployment controller replace the pods under its own strategy -- surge,
// maxUnavailable and readiness probes included. The annotation is read back
// here, so this proves what was written and not merely that a call was made.
func TestARolloutRestartRollsRatherThanDeleting(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{
		Deployments: []kubesim.Deployment{{Namespace: "default", Name: "api", Desired: 2, Ready: 2}},
		Pods: []kubesim.Pod{
			{Namespace: "default", Name: "api-1", Node: "cp-1", OwnerKind: "ReplicaSet"},
			{Namespace: "default", Name: "api-2", Node: "cp-1", OwnerKind: "ReplicaSet"},
		},
	})

	now := time.Date(2026, 9, 19, 10, 30, 0, 0, time.UTC)
	if err := client.RolloutRestart(ctx, "default", "api", now); err != nil {
		t.Fatalf("RolloutRestart: %v", err)
	}

	if got := sim.RestartedAt("default", "api"); got != now.Format(time.RFC3339) {
		t.Errorf("restartedAt = %q, want the timestamp that changes the template hash", got)
	}
	// And no pod was deleted by this product: the controller does the replacing,
	// which is what keeps the workload up.
	if got := sim.Deleted(); len(got) != 0 {
		t.Errorf("a rollout restart deleted pods itself: %v. That takes the workload down, which "+
			"is the thing a rollout restart exists not to do", got)
	}
	if err := client.RolloutRestart(ctx, "default", "ghost", now); !errors.Is(err, kube.ErrNoSuchWorkload) {
		t.Errorf("restarting a deployment that is not there = %v, want ErrNoSuchWorkload", err)
	}
}

// TestDeploymentsCanBeAskedForOneNamespace: the filter is the server's, for the
// reason the pod list's is.
func TestDeploymentsCanBeAskedForOneNamespace(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{Deployments: []kubesim.Deployment{
		{Namespace: "default", Name: "api", Desired: 1},
		{Namespace: "kube-system", Name: "coredns", Desired: 2},
	}})

	all, err := client.Deployments(ctx, "")
	if err != nil {
		t.Fatalf("Deployments: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("deployments = %d, want both namespaces", len(all))
	}

	one, err := client.Deployments(ctx, "kube-system")
	if err != nil {
		t.Fatalf("Deployments: %v", err)
	}
	names := make([]string, 0, len(one))
	for _, d := range one {
		names = append(names, d.Name)
	}
	if !slices.Equal(names, []string{"coredns"}) {
		t.Errorf("filtered = %v, want only coredns", names)
	}
	if got := sim.Calls("GET /apis/apps/v1/namespaces/kube-system/deployments"); got != 1 {
		t.Errorf("the namespaced path was requested %d times; the filter has to be the server's", got)
	}
}
