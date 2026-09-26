package power_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/kubesim"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/power"
)

// A whole cluster.

// TestAClusterStopsWorkersFirstAndItsOwnEndpointLast.
//
// Workers first, because draining one needs an API server and the control
// plane is where that runs. The control-plane node this daemon reaches the
// cluster through last, because taking it down earlier leaves the rest of the
// walk without a way in. The endpoint node here sorts FIRST by name on
// purpose: an order that was merely alphabetical would pass a test where it
// happened to sort last.
func TestAClusterStopsWorkersFirstAndItsOwnEndpointLast(t *testing.T) {
	t.Parallel()

	r := newRig(t, []spec{
		{host: "cp-a", cp: true, endpoint: true},
		{host: "cp-b", cp: true},
		{host: "w-2"},
		{host: "w-1"},
	}, kubesim.Options{Pods: []kubesim.Pod{
		{Namespace: "default", Name: "web-1", Node: "w-1", OwnerKind: "ReplicaSet"},
	}})

	j, err := r.svc.SubmitCluster(t.Context(), testCluster, power.Stop, "tester")
	if err != nil {
		t.Fatalf("SubmitCluster: %v", err)
	}
	j = r.succeeded(j)

	want := []string{
		"cordon w-1", "drain w-1", "shut down w-1",
		"cordon w-2", "drain w-2", "shut down w-2",
		"cordon cp-b", "force cp-b off",
		"cordon cp-a", "force cp-a off",
	}
	if got := stepNames(j); !slices.Equal(got, want) {
		t.Fatalf("the walk is\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}

	for _, host := range []string{"w-1", "w-2", "cp-a", "cp-b"} {
		node := r.sim(host).Node()
		if !node.PoweredOff {
			t.Errorf("%s is still running after a cluster stop", host)
		}
		// Workers gracefully, after their drain; the control plane without
		// Talos's own drain, which would wait on an API server that is going.
		wantForced := 0
		if strings.HasPrefix(host, "cp-") {
			wantForced = 1
		}
		if node.ForcedShutdowns != wantForced {
			t.Errorf("%s: %d forced shutdowns, want %d", host, node.ForcedShutdowns, wantForced)
		}
	}
	if !slices.Contains(r.kube.Evicted(), "default/web-1") {
		t.Error("the worker was not drained before it was shut down")
	}

	rep, err := r.svc.ClusterReport(t.Context(), testCluster)
	if err != nil {
		t.Fatalf("ClusterReport: %v", err)
	}
	if rep.State != power.StateStopped {
		t.Errorf("after a stop the cluster is %s", rep.State)
	}
}

// TestARollingRestartStopsAtTheFirstNodeThatDoesNotComeBack.
//
// The operator's rule: one node at a time, and stop at the first one that is
// not healthy afterwards. A restart that carried on would take the next node
// down in a cluster already one short -- so the test is not only that the job
// failed, but that the node after the failure was never touched.
func TestARollingRestartStopsAtTheFirstNodeThatDoesNotComeBack(t *testing.T) {
	t.Parallel()

	r := newRig(t, []spec{
		{host: "cp-1", cp: true, endpoint: true},
		{host: "w-1"},
		{host: "w-2"},
	}, kubesim.Options{})
	r.sim("w-2").StayDownOnReboot(true)

	j, err := r.svc.SubmitCluster(t.Context(), testCluster, power.Restart, "tester")
	if err != nil {
		t.Fatalf("SubmitCluster: %v", err)
	}
	j = r.wait(j)

	if j.State != model.JobFailed {
		t.Fatalf("the restart ended %s, want failed:\n%s", j.State, describe(j))
	}
	failed := j.Steps[j.Current]
	if failed.Name != "wait for w-2 to come back" {
		t.Errorf("it failed at %q, want the wait for w-2:\n%s", failed.Name, describe(j))
	}

	if n := r.sim("w-1").Node().Reboots; n != 1 {
		t.Errorf("w-1 rebooted %d times, want once", n)
	}
	if r.kube.Unschedulable("w-1") {
		t.Error("w-1 came back and was left cordoned")
	}
	if n := r.sim("w-2").Node().Reboots; n != 1 {
		t.Errorf("w-2 rebooted %d times, want once", n)
	}
	if n := r.sim("cp-1").Node().Reboots; n != 0 {
		t.Fatalf("cp-1 was rebooted %d time(s) after w-2 did not come back; the restart must stop "+
			"at the first unhealthy node", n)
	}
	if r.kube.Unschedulable("cp-1") {
		t.Error("cp-1 was cordoned although the walk stopped before it")
	}
}

// TestANodeThatStillAnswersAfterAcceptingARebootIsNotBack.
//
// A real node goes on answering for a few seconds after it accepts a reboot.
// A wait that took "it answers" for "it is back" would uncordon it in those
// seconds -- just before it leaves, with the scheduler already placing work on
// it. The evidence is the node's own boot time, the same evidence the reboot
// job's resume uses, and a node whose boot time has not moved is not back
// however promptly it answers.
func TestANodeThatStillAnswersAfterAcceptingARebootIsNotBack(t *testing.T) {
	t.Parallel()

	r := newRig(t, []spec{
		{host: "cp-1", cp: true, endpoint: true},
		{host: "w-1"},
	}, kubesim.Options{})
	r.sim("w-1").StayUpOnReboot(true)

	j, err := r.svc.SubmitMachine(t.Context(), r.ids["w-1"], power.Restart, "tester")
	if err != nil {
		t.Fatalf("SubmitMachine: %v", err)
	}
	j = r.wait(j)

	if j.State != model.JobFailed || j.Steps[j.Current].Name != "wait for w-1 to come back" {
		t.Fatalf("a node that accepted the reboot and never went down was taken for back:\n%s", describe(j))
	}
	if !r.kube.Unschedulable("w-1") {
		t.Error("the node was uncordoned although it never rebooted")
	}
}

// TestAClusterStartWakesTheControlPlaneFirstAndSkipsADisabledNode.
func TestAClusterStartWakesTheControlPlaneFirstAndSkipsADisabledNode(t *testing.T) {
	t.Parallel()

	r := newRig(t, []spec{
		{host: "cp-1", cp: true, endpoint: true, off: true, mac: "02:00:00:00:00:01"},
		{host: "w-1", off: true, mac: "02:00:00:00:00:11"},
		{host: "w-2", off: true, mac: "02:00:00:00:00:12", disabled: true},
	}, kubesim.Options{})
	ctx := t.Context()

	// The stop cordoned w-1 and recorded that it owes it an uncordon.
	rec := r.machine("w-1")
	rec.PowerCordoned = true
	if _, err := r.st.Machines().Put(ctx, rec); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := r.kube.SetUnschedulable("w-1", true); err != nil {
		t.Fatalf("SetUnschedulable: %v", err)
	}

	j, err := r.svc.SubmitCluster(ctx, testCluster, power.Start, "tester")
	if err != nil {
		t.Fatalf("SubmitCluster: %v", err)
	}
	r.succeeded(j)

	if woken := r.waker.woken(); !slices.Equal(woken, []string{"02:00:00:00:00:01", "02:00:00:00:00:11"}) {
		t.Fatalf("Wake-on-LAN went to %v, want the control plane's card and then the worker's -- "+
			"and never the disabled node's", woken)
	}
	if r.sim("w-2").Node().PoweredOff != true {
		t.Error("the disabled node was started")
	}
	if r.kube.Unschedulable("w-1") {
		t.Error("the node the stop cordoned was left cordoned")
	}
	if r.machine("w-1").PowerCordoned {
		t.Error("the owed uncordon is still recorded")
	}
}
