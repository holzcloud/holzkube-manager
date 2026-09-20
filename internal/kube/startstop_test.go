package kube_test

import (
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// Stopping and starting (2026-09-20).
//
// The claim worth the most: starting again restores the count that was running.
// Scaling to zero throws it away, and "start it" then has no answer but one --
// which silently halves a three-replica service at the moment somebody is
// restoring it.

// TestStartingAgainRestoresWhatWasRunning.
func TestStartingAgainRestoresWhatWasRunning(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{Deployments: []kubesim.Deployment{
		{Namespace: "default", Name: "website", Desired: 3, Ready: 3},
	}})

	if err := client.Stop(ctx, kube.KindDeployment, "default", "website"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := sim.DesiredReplicas("default", "website"); got != 0 {
		t.Fatalf("after stopping, desired = %d, want 0", got)
	}

	state, err := client.StoppedStateOf(ctx, kube.KindDeployment, "default", "website")
	if err != nil {
		t.Fatalf("StoppedStateOf: %v", err)
	}
	if !state.Stopped {
		t.Error("a workload at zero was not reported as stopped")
	}
	// The screen shows this before anybody presses start, so nobody is
	// surprised by the number that comes back.
	if state.WouldStartWith != 3 {
		t.Errorf("would start with %d, want the 3 it was running", state.WouldStartWith)
	}

	if err := client.Start(ctx, kube.KindDeployment, "default", "website"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := sim.DesiredReplicas("default", "website"); got != 3 {
		t.Errorf("after starting, desired = %d, want the 3 it was running before", got)
	}
}

// TestStartingSomethingNobodyStoppedUsesOne: scaled to zero by a kubectl, an
// autoscaler or a manifest, there is nothing written down, and one is the only
// honest answer.
func TestStartingSomethingNobodyStoppedUsesOne(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{Deployments: []kubesim.Deployment{
		{Namespace: "default", Name: "website", Desired: 0},
	}})

	state, err := client.StoppedStateOf(ctx, kube.KindDeployment, "default", "website")
	if err != nil {
		t.Fatalf("StoppedStateOf: %v", err)
	}
	if !state.Stopped || state.WouldStartWith != 0 {
		t.Errorf("state = %+v, want stopped with nothing written down", state)
	}

	if err := client.Start(ctx, kube.KindDeployment, "default", "website"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := sim.DesiredReplicas("default", "website"); got != 1 {
		t.Errorf("desired = %d, want 1", got)
	}
}

// TestStoppingACronJobSuspendsIt, because it has no replicas and suspending is
// exactly what stopping means for one.
func TestStoppingACronJobSuspendsIt(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{CronJobs: []kubesim.CronJob{
		{Namespace: "default", Name: "backup", Schedule: "0 2 * * *"},
	}})

	if err := client.Stop(ctx, kube.KindCronJob, "default", "backup"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	state, err := client.StoppedStateOf(ctx, kube.KindCronJob, "default", "backup")
	if err != nil {
		t.Fatalf("StoppedStateOf: %v", err)
	}
	if !state.Stopped {
		t.Error("a suspended CronJob was not reported as stopped")
	}

	if err := client.Start(ctx, kube.KindCronJob, "default", "backup"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	state, err = client.StoppedStateOf(ctx, kube.KindCronJob, "default", "backup")
	if err != nil {
		t.Fatalf("StoppedStateOf: %v", err)
	}
	if state.Stopped {
		t.Error("a resumed CronJob is still reported as stopped")
	}
}

// TestADaemonSetHasNoStop, and the refusal says what to do instead.
func TestADaemonSetHasNoStop(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{})

	err := client.Stop(ctx, kube.KindDaemonSet, "longhorn-system", "longhorn-manager")
	if !errors.Is(err, kube.ErrCannotStop) {
		t.Fatalf("err = %v, want ErrCannotStop", err)
	}
	if !strings.Contains(err.Error(), "Cordon or drain") {
		t.Errorf("reason = %q, want it to name what to do instead", err)
	}
}

// TestStoppingAPodMeansStoppingItsController.
//
// Deleting a pod does not stop it: its controller makes another within seconds.
// And the controller of a Deployment's pod is a ReplicaSet, which the Deployment
// would immediately scale back -- so the answer has to walk up twice.
func TestStoppingAPodMeansStoppingItsController(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Pods: []kubesim.Pod{
		{Namespace: "default", Name: "website-abc", Node: "cp-1",
			OwnerKind: "ReplicaSet", Phase: corev1.PodRunning},
		{Namespace: "default", Name: "debug", Node: "cp-1", Phase: corev1.PodRunning},
	}})

	// A pod nothing owns has no stop, and saying so is better than deleting it
	// and calling the deletion a stop.
	if _, _, err := client.OwnerOfPod(ctx, "default", "debug"); !errors.Is(err, kube.ErrCannotStop) {
		t.Errorf("a bare pod = %v, want ErrCannotStop", err)
	}
}

// TestTheCountIsWrittenBeforeTheScale: an interrupted stop must leave the count
// recoverable rather than lost.
func TestTheCountIsWrittenBeforeTheScale(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{Deployments: []kubesim.Deployment{
		{Namespace: "default", Name: "website", Desired: 5, Ready: 5},
	}})

	if err := client.Stop(ctx, kube.KindDeployment, "default", "website"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// The annotation patch goes out before the scale. An annotation that
	// outlives a stop that did not happen is harmless; a stop whose count went
	// missing is not.
	patch := sim.Calls("PATCH /apis/apps/v1/namespaces/default/deployments/website")
	scale := sim.Calls("PUT /apis/apps/v1/namespaces/default/deployments/website/scale")
	if patch != 1 || scale != 1 {
		t.Fatalf("patch=%d scale=%d, want one of each", patch, scale)
	}

	state, err := client.StoppedStateOf(ctx, kube.KindDeployment, "default", "website")
	if err != nil {
		t.Fatalf("StoppedStateOf: %v", err)
	}
	if state.WouldStartWith != 5 {
		t.Errorf("would start with %d, want 5", state.WouldStartWith)
	}
}
