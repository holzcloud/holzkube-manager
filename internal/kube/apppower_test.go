package kube_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// The app half of the power model (2026-09-26), at the Kubernetes API.

// TestADaemonSetStopsAndStartsAgain is the round trip, read back from the
// cluster rather than from what the product said it did.
//
// What it guards is the pair of promises the stop makes: the DaemonSet runs
// nowhere while stopped, and starting gives it back EXACTLY the selector it had
// -- including a key it had before, which a start that cleared the selector
// would silently widen onto every node in the cluster.
func TestADaemonSetStopsAndStartsAgain(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{DaemonSets: []kubesim.DaemonSet{{
		Namespace: "longhorn-system", Name: "longhorn-manager", Scheduled: 3, Ready: 3,
		NodeSelector: map[string]string{"storage": "ssd"},
	}}})

	if err := client.Stop(ctx, kube.KindDaemonSet, "longhorn-system", "longhorn-manager"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	selector := sim.NodeSelector("longhorn-system", "longhorn-manager")
	if selector[kube.StoppedSelectorKey] != "true" {
		t.Fatalf("after a stop the selector is %v; it must require %s=true, which no node carries",
			selector, kube.StoppedSelectorKey)
	}
	if selector["storage"] != "ssd" {
		t.Errorf("the stop replaced the selector (%v) instead of adding to it", selector)
	}
	if got := sim.Annotations("daemonset", "longhorn-system", "longhorn-manager")["holzkube.io/node-selector-before-stop"]; got != `{"storage":"ssd"}` {
		t.Errorf("the selector it had is recorded as %q, want the JSON of what it had", got)
	}

	state, err := client.StoppedStateOf(ctx, kube.KindDaemonSet, "longhorn-system", "longhorn-manager")
	if err != nil {
		t.Fatalf("StoppedStateOf: %v", err)
	}
	if !state.Stopped {
		t.Error("a stopped DaemonSet reads as running")
	}

	rows, err := client.Workloads(ctx, "longhorn-system")
	if err != nil {
		t.Fatalf("Workloads: %v", err)
	}
	for _, row := range rows {
		if row.Kind == kube.KindDaemonSet && (!row.Stoppable || !row.Stopped) {
			t.Errorf("the workload row says stoppable=%v stopped=%v; a screen reading it would "+
				"offer the wrong button", row.Stoppable, row.Stopped)
		}
	}

	// A second stop is the same stop, not a second key or a lost record.
	if err := client.Stop(ctx, kube.KindDaemonSet, "longhorn-system", "longhorn-manager"); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	if got := sim.Annotations("daemonset", "longhorn-system", "longhorn-manager")["holzkube.io/node-selector-before-stop"]; got != `{"storage":"ssd"}` {
		t.Errorf("a second stop overwrote the record of the original selector with %q", got)
	}

	if err := client.Start(ctx, kube.KindDaemonSet, "longhorn-system", "longhorn-manager"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	selector = sim.NodeSelector("longhorn-system", "longhorn-manager")
	if len(selector) != 1 || selector["storage"] != "ssd" {
		t.Fatalf("after starting again the selector is %v, want exactly the {storage: ssd} it had", selector)
	}
	if _, left := sim.Annotations("daemonset", "longhorn-system", "longhorn-manager")["holzkube.io/node-selector-before-stop"]; left {
		t.Error("the record of the stopped selector outlived the start")
	}
	state, err = client.StoppedStateOf(ctx, kube.KindDaemonSet, "longhorn-system", "longhorn-manager")
	if err != nil {
		t.Fatalf("StoppedStateOf: %v", err)
	}
	if state.Stopped {
		t.Error("a started DaemonSet still reads as stopped")
	}
}

// TestDisabledIsAMarkThatComesOffAgain: the annotation is written, read, and
// REMOVED -- an enable that left "holzkube.io/disabled" behind with some other
// value would be a flag somebody has to know how to read.
func TestDisabledIsAMarkThatComesOffAgain(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{
		Deployments: []kubesim.Deployment{{Namespace: "default", Name: "web", Desired: 2, Ready: 2}},
		CronJobs:    []kubesim.CronJob{{Namespace: "default", Name: "backup", Schedule: "0 3 * * *"}},
	})

	for _, target := range []struct {
		kind kube.WorkloadKind
		key  string
		name string
	}{
		{kube.KindDeployment, "deployment", "web"},
		{kube.KindCronJob, "cronjob", "backup"},
	} {
		if err := client.SetAppDisabled(ctx, target.kind, "default", target.name, true); err != nil {
			t.Fatalf("disable %s: %v", target.kind, err)
		}
		if got := sim.Annotations(target.key, "default", target.name)[kube.DisabledAnnotation]; got != "true" {
			t.Fatalf("%s carries %s=%q after a disable", target.kind, kube.DisabledAnnotation, got)
		}
		power, err := client.AppPowerOf(ctx, target.kind, "default", target.name)
		if err != nil {
			t.Fatalf("AppPowerOf %s: %v", target.kind, err)
		}
		if !power.Disabled {
			t.Errorf("a disabled %s reads as not disabled", target.kind)
		}

		if err := client.SetAppDisabled(ctx, target.kind, "default", target.name, false); err != nil {
			t.Fatalf("enable %s: %v", target.kind, err)
		}
		if _, left := sim.Annotations(target.key, "default", target.name)[kube.DisabledAnnotation]; left {
			t.Errorf("the disabled mark is still on the %s after it was removed", target.kind)
		}
	}
}

// TestForceDeletingAnAppsPodsTouchesOnlyThatApp and uses no grace period.
//
// The pods are found by the app's own selector. A selector ignored -- or an
// empty one -- is "every pod in the namespace", which is the failure worth a
// test: a force-stop of one app that killed its neighbour's pods.
func TestForceDeletingAnAppsPodsTouchesOnlyThatApp(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{
		Deployments: []kubesim.Deployment{
			{Namespace: "default", Name: "web", Desired: 2, Ready: 2},
			{Namespace: "default", Name: "db", Desired: 1, Ready: 1},
		},
		Pods: []kubesim.Pod{
			{Namespace: "default", Name: "web-1", OwnerKind: "ReplicaSet", Labels: map[string]string{"app": "web"}},
			{Namespace: "default", Name: "web-2", OwnerKind: "ReplicaSet", Labels: map[string]string{"app": "web"}},
			{Namespace: "default", Name: "db-1", OwnerKind: "ReplicaSet", Labels: map[string]string{"app": "db"}},
		},
	})

	n, err := client.ForceDeleteAppPods(ctx, kube.KindDeployment, "default", "web")
	if err != nil {
		t.Fatalf("ForceDeleteAppPods: %v", err)
	}
	if n != 2 {
		t.Errorf("deleted %d pods, want the 2 web has", n)
	}
	left := sim.PodNames()
	if !slices.Equal(left, []string{"default/db-1"}) {
		t.Fatalf("pods left: %v, want only db's", left)
	}
	for _, pod := range []string{"web-1", "web-2"} {
		grace, deleted := sim.DeleteGrace("default", pod)
		if !deleted || grace == nil || *grace != 0 {
			t.Errorf("%s: deleted=%v grace=%v, want deleted with a grace period of zero", pod, deleted, grace)
		}
	}
}

// TestABarePodIsDeletedAndAnOwnedOneIsNot.
//
// A bare pod's only stop is a delete. A pod with a controller is refused,
// because deleting it is a restart -- its controller makes another -- and
// calling that a stop is the blur RestartPod was written to refuse.
func TestABarePodIsDeletedAndAnOwnedOneIsNot(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{Pods: []kubesim.Pod{
		{Namespace: "default", Name: "debug"},
		{Namespace: "default", Name: "scratch"},
		{Namespace: "default", Name: "web-1", OwnerKind: "ReplicaSet"},
	}})

	if err := client.DeleteBarePod(ctx, "default", "web-1", false); !errors.Is(err, kube.ErrCannotStop) {
		t.Fatalf("deleting an owned pod as a bare one: err = %v, want ErrCannotStop", err)
	}
	if err := client.DeleteBarePod(ctx, "default", "debug", false); err != nil {
		t.Fatalf("DeleteBarePod: %v", err)
	}
	if grace, deleted := sim.DeleteGrace("default", "debug"); !deleted || grace != nil {
		t.Errorf("an ordinary stop of a bare pod: deleted=%v grace=%v, want its own grace period", deleted, grace)
	}
	if err := client.DeleteBarePod(ctx, "default", "scratch", true); err != nil {
		t.Fatalf("DeleteBarePod force: %v", err)
	}
	if grace, deleted := sim.DeleteGrace("default", "scratch"); !deleted || grace == nil || *grace != 0 {
		t.Errorf("a forced stop of a bare pod: deleted=%v grace=%v, want zero", deleted, grace)
	}
	if left := sim.PodNames(); !slices.Equal(left, []string{"default/web-1"}) {
		t.Errorf("pods left: %v", left)
	}
}

// TestAFinishedJobIsNotAStoppedOne: nobody stopped it, and it will not run
// again. The two read differently so that a start of a finished Job can be
// refused with the right sentence rather than accepted and do nothing.
func TestAFinishedJobIsNotAStoppedOne(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Jobs: []kubesim.Job{
		{Namespace: "default", Name: "migrate", Succeeded: 1},
		{Namespace: "default", Name: "import", Active: 1},
	}})

	done, err := client.AppPowerOf(ctx, kube.KindJob, "default", "migrate")
	if err != nil {
		t.Fatalf("AppPowerOf: %v", err)
	}
	if !done.Finished || done.Stopped {
		t.Errorf("a finished Job reads as finished=%v stopped=%v", done.Finished, done.Stopped)
	}
	running, err := client.AppPowerOf(ctx, kube.KindJob, "default", "import")
	if err != nil {
		t.Fatalf("AppPowerOf: %v", err)
	}
	if running.Finished || running.Stopped || running.Ready != 1 {
		t.Errorf("a running Job reads as %+v", running)
	}
}
