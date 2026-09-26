package power_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/kubesim"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/power"
)

// One node: the careful verbs and the forced ones.

// TestStoppingAControlPlaneNodeEtcdCannotSpareIsRefusedAndForceStopIsNot.
//
// Two control-plane nodes are two etcd voters, and a majority of two is two:
// taking either down stops the cluster accepting writes until it is back. The
// careful stop asks the same gate the rolling upgrade asks, and refuses -- on
// the screen before anybody presses, and at the server when somebody presses
// anyway. The forced one does not ask, which is the whole of what makes it
// forced, and it does not drain either.
func TestStoppingAControlPlaneNodeEtcdCannotSpareIsRefusedAndForceStopIsNot(t *testing.T) {
	t.Parallel()

	r := newRig(t, []spec{
		{host: "cp-1", cp: true, endpoint: true},
		{host: "cp-2", cp: true},
	}, kubesim.Options{Pods: []kubesim.Pod{
		{Namespace: "default", Name: "web-1", Node: "cp-2", OwnerKind: "ReplicaSet"},
	}})
	ctx := t.Context()
	id := r.ids["cp-2"]

	rep, err := r.svc.MachineReport(ctx, id)
	if err != nil {
		t.Fatalf("MachineReport: %v", err)
	}
	if rep.State != power.StateRunning {
		t.Errorf("state = %s, want running", rep.State)
	}
	stop := statusOf(t, rep, power.Stop)
	if stop.Available || stop.Reason != power.ReasonQuorum {
		t.Fatalf("stop = %+v, want unavailable with %q", stop, power.ReasonQuorum)
	}
	if disable := statusOf(t, rep, power.Disable); disable.Available {
		t.Errorf("disable does what stop does and was offered anyway: %+v", disable)
	}
	force := statusOf(t, rep, power.ForceStop)
	if !force.Available || !force.Sudo {
		t.Fatalf("force-stop = %+v, want available behind the password", force)
	}

	_, err = r.svc.SubmitMachine(ctx, id, power.Stop, "tester")
	var refused *power.UnavailableError
	if !errors.As(err, &refused) || refused.Reason != power.ReasonQuorum {
		t.Fatalf("a stop the screen refused was submitted anyway: err = %v", err)
	}
	if r.sim("cp-2").Node().PoweredOff {
		t.Fatal("the refused stop switched the node off")
	}

	j, err := r.svc.SubmitMachine(ctx, id, power.ForceStop, "tester")
	if err != nil {
		t.Fatalf("SubmitMachine force-stop: %v", err)
	}
	r.succeeded(j)

	node := r.sim("cp-2").Node()
	if !node.PoweredOff {
		t.Fatal("force-stop left the node running")
	}
	if node.ForcedShutdowns != 1 {
		t.Errorf("the shutdown was not forced (%d forced of %d)", node.ForcedShutdowns, node.Shutdowns)
	}
	if evicted := r.kube.Evicted(); len(evicted) != 0 {
		t.Errorf("force-stop drained %v; it does not drain", evicted)
	}
	if r.sim("cp-1").Node().PoweredOff {
		t.Error("stopping one node switched another off")
	}
}

// TestStoppingAWorkerCordonsDrainsAndShutsDown: the careful stop, all the way,
// read back from the cluster and the node rather than from the job's word.
func TestStoppingAWorkerCordonsDrainsAndShutsDown(t *testing.T) {
	t.Parallel()

	r := newRig(t, []spec{
		{host: "cp-1", cp: true, endpoint: true},
		{host: "w-1", mac: "02:00:00:00:01:01"},
	}, kubesim.Options{Pods: []kubesim.Pod{
		{Namespace: "default", Name: "web-1", Node: "w-1", OwnerKind: "ReplicaSet"},
		{Namespace: "default", Name: "cache-1", Node: "w-1", OwnerKind: "ReplicaSet", LocalData: true},
		{Namespace: "kube-system", Name: "proxy-w1", Node: "w-1", OwnerKind: "DaemonSet"},
		{Namespace: "default", Name: "api-1", Node: "cp-1", OwnerKind: "ReplicaSet"},
	}})
	ctx := t.Context()

	j, err := r.svc.SubmitMachine(ctx, r.ids["w-1"], power.Stop, "tester")
	if err != nil {
		t.Fatalf("SubmitMachine: %v", err)
	}
	j = r.succeeded(j)

	if !r.kube.Unschedulable("w-1") {
		t.Error("the stopped worker is not cordoned")
	}
	evicted := r.kube.Evicted()
	slices.Sort(evicted)
	if !slices.Equal(evicted, []string{"default/cache-1", "default/web-1"}) {
		t.Errorf("evicted %v, want the worker's two movable pods and nothing else", evicted)
	}
	node := r.sim("w-1").Node()
	if !node.PoweredOff || node.ForcedShutdowns != 0 {
		t.Errorf("the worker: off=%v, forced shutdowns=%d; a careful stop is a graceful shutdown",
			node.PoweredOff, node.ForcedShutdowns)
	}
	if !r.machine("w-1").PowerCordoned {
		t.Error("the stop cordoned the node and did not record that it owes it an uncordon")
	}

	rep, err := r.svc.MachineReport(ctx, r.ids["w-1"])
	if err != nil {
		t.Fatalf("MachineReport: %v", err)
	}
	if rep.State != power.StateStopped {
		t.Errorf("after a stop the state is %s", rep.State)
	}
	if start := statusOf(t, rep, power.Start); !start.Available {
		t.Errorf("a stopped node with a known network card cannot be started: %+v", start)
	}
	_ = j
}

// TestADisabledNodeStaysOffUntilItIsEnabled is the operator's "off and stays
// off", end to end: disable stops it and marks it; start refuses, on the screen
// and at the server; enable clears the mark, wakes it over the network, waits
// for it, and puts it back into the scheduler.
func TestADisabledNodeStaysOffUntilItIsEnabled(t *testing.T) {
	t.Parallel()

	const mac = "02:00:00:00:01:02"
	r := newRig(t, []spec{
		{host: "cp-1", cp: true, endpoint: true},
		{host: "w-1", mac: mac},
	}, kubesim.Options{})
	ctx := t.Context()
	id := r.ids["w-1"]

	j, err := r.svc.SubmitMachine(ctx, id, power.Disable, "tester")
	if err != nil {
		t.Fatalf("SubmitMachine disable: %v", err)
	}
	r.succeeded(j)
	if !r.machine("w-1").Disabled {
		t.Fatal("disable did not record the mark")
	}
	if !r.sim("w-1").Node().PoweredOff {
		t.Fatal("disable did not switch the node off")
	}

	rep, err := r.svc.MachineReport(ctx, id)
	if err != nil {
		t.Fatalf("MachineReport: %v", err)
	}
	if rep.State != power.StateDisabled || !rep.Disabled {
		t.Errorf("state = %s disabled = %v, want disabled", rep.State, rep.Disabled)
	}
	start := statusOf(t, rep, power.Start)
	if start.Available || start.Reason != power.ReasonDisabled {
		t.Fatalf("start of a disabled node = %+v, want refused with %q", start, power.ReasonDisabled)
	}
	if enable := statusOf(t, rep, power.Enable); !enable.Available {
		t.Fatalf("enable of a disabled node = %+v", enable)
	}

	_, err = r.svc.SubmitMachine(ctx, id, power.Start, "tester")
	var refused *power.UnavailableError
	if !errors.As(err, &refused) || refused.Reason != power.ReasonDisabled {
		t.Fatalf("a start of a disabled node was submitted: err = %v", err)
	}
	if woken := r.waker.woken(); len(woken) != 0 {
		t.Fatalf("a refused start sent Wake-on-LAN to %v", woken)
	}

	j, err = r.svc.SubmitMachine(ctx, id, power.Enable, "tester")
	if err != nil {
		t.Fatalf("SubmitMachine enable: %v", err)
	}
	j = r.succeeded(j)

	if r.machine("w-1").Disabled {
		t.Error("enable left the mark")
	}
	if woken := r.waker.woken(); !slices.Equal(woken, []string{mac}) {
		t.Errorf("Wake-on-LAN went to %v, want the node's own card %s and nothing else", woken, mac)
	}
	node := r.sim("w-1").Node()
	if node.PoweredOff || node.PowerOns != 1 {
		t.Errorf("after enable: off=%v power-ons=%d", node.PoweredOff, node.PowerOns)
	}
	if r.kube.Unschedulable("w-1") {
		t.Error("the enabled node is still cordoned")
	}
	if r.machine("w-1").PowerCordoned {
		t.Error("the uncordon left the mark saying one is still owed")
	}
	if !strings.Contains(describe(j), mac) {
		t.Errorf("the job does not say which card it woke:\n%s", describe(j))
	}
}

// TestAStartInterruptedWhileWaitingResumesWithoutWakingTwice.
//
// The daemon is restarted while a start is waiting for the node -- the waits
// are the longest steps, so that is where a restart most often lands. The new
// process rebuilds the job from what it stored, carries on with the wait, and
// does not send the magic packet a second time: the wake step finished before
// the restart, and a finished step is not repeated.
func TestAStartInterruptedWhileWaitingResumesWithoutWakingTwice(t *testing.T) {
	t.Parallel()

	const mac = "02:00:00:00:01:03"
	r := newRig(t, []spec{
		{host: "cp-1", cp: true, endpoint: true},
		{host: "w-1", mac: mac, off: true},
	}, kubesim.Options{})
	r.waker.setHold(true)

	j, err := r.svc.SubmitMachine(t.Context(), r.ids["w-1"], power.Start, "tester")
	if err != nil {
		t.Fatalf("SubmitMachine: %v", err)
	}

	// Wait until it is waiting.
	for {
		got, err := r.engine.Get(t.Context(), j.ID)
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		if got.Current == 2 && got.Steps[2].State == model.StepRunning {
			break
		}
		if got.State.Terminal() {
			t.Fatalf("the start ended before it waited:\n%s", describe(got))
		}
		time.Sleep(10 * time.Millisecond)
	}

	r.restart()
	// The node comes up while the daemon is away, as a real one would.
	r.sim("w-1").PowerOn()

	got := r.succeeded(j)
	if woken := r.waker.woken(); !slices.Equal(woken, []string{mac}) {
		t.Errorf("Wake-on-LAN went out %v; the restart repeated a step that had finished", woken)
	}
	if got.Steps[1].State != model.StepDone {
		t.Errorf("the wake step is %s after the resume:\n%s", got.Steps[1].State, describe(got))
	}
}

// TestADisabledNodeThatComesUpAnywayIsKeptCordoned: somebody pressed its power
// button. It is not switched off behind their back; it is kept out of the
// scheduler, and the log says why.
func TestADisabledNodeThatComesUpAnywayIsKeptCordoned(t *testing.T) {
	t.Parallel()

	r := newRig(t, []spec{
		{host: "cp-1", cp: true, endpoint: true},
		{host: "w-1", disabled: true},
		{host: "w-2"},
	}, kubesim.Options{})

	r.svc.Sweep(t.Context())

	if !r.kube.Unschedulable("w-1") {
		t.Fatal("a disabled node that answers was left schedulable")
	}
	if r.kube.Unschedulable("w-2") {
		t.Error("the sweep cordoned a node nobody disabled")
	}
	if r.sim("w-1").Node().PoweredOff {
		t.Error("the sweep switched the node off; it keeps it cordoned instead")
	}
	logs := r.logs.String()
	if !strings.Contains(logs, "a disabled node is running") || !strings.Contains(logs, "w-1") {
		t.Errorf("the log does not say a disabled node is running and was cordoned:\n%s", logs)
	}
}

// TestTheReportListsAllSevenActionsInOrder: the contract's order, every time,
// with the forced two -- and only those -- behind the password.
func TestTheReportListsAllSevenActionsInOrder(t *testing.T) {
	t.Parallel()

	r := newRig(t, []spec{
		{host: "cp-1", cp: true, endpoint: true},
		{host: "w-1", off: true},
	}, kubesim.Options{})
	ctx := t.Context()

	want := []power.Action{
		power.Stop, power.ForceStop, power.Start, power.Disable, power.Enable, power.Restart, power.ForceRestart,
	}
	machine, err := r.svc.MachineReport(ctx, r.ids["w-1"])
	if err != nil {
		t.Fatalf("MachineReport: %v", err)
	}
	cluster, err := r.svc.ClusterReport(ctx, testCluster)
	if err != nil {
		t.Fatalf("ClusterReport: %v", err)
	}
	for name, rep := range map[string]power.Report{"machine": machine, "cluster": cluster} {
		var got []power.Action
		for _, s := range rep.Actions {
			got = append(got, s.Action)
			if s.Sudo != (s.Action == power.ForceStop || s.Action == power.ForceRestart) {
				t.Errorf("%s: %s sudo = %v", name, s.Action, s.Sudo)
			}
			if s.Available == (s.Reason != "") {
				t.Errorf("%s: %s available=%v with reason %q; an unavailable action says why, "+
					"an available one does not", name, s.Action, s.Available, s.Reason)
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s actions = %v, want %v", name, got, want)
		}
	}

	// The off worker with no known card: start says why it cannot.
	if start := statusOf(t, machine, power.Start); start.Available || start.Reason != power.ReasonNoMAC {
		t.Errorf("start of a node with no known card = %+v", start)
	}
	if cluster.State != power.StatePartial {
		t.Errorf("a cluster with one node up and one off is %s, want partial", cluster.State)
	}
	if restart := statusOf(t, cluster, power.Restart); restart.Available || restart.Reason != power.ReasonPartial {
		t.Errorf("rolling restart of a partial cluster = %+v", restart)
	}
}

// TestANodeOfALockedClusterOffersNothing: the read-only adoption is what stands
// in the way, and saying anything else sends the operator to the wrong screen.
func TestANodeOfALockedClusterOffersNothing(t *testing.T) {
	t.Parallel()

	r := newRig(t, []spec{{host: "cp-1", cp: true, endpoint: true}}, kubesim.Options{})
	ctx := t.Context()
	c, err := r.st.Clusters().Get(ctx, testCluster)
	if err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	c.Locked = true
	if _, err := r.st.Clusters().Put(ctx, c); err != nil {
		t.Fatalf("put cluster: %v", err)
	}

	rep, err := r.svc.MachineReport(ctx, r.ids["cp-1"])
	if err != nil {
		t.Fatalf("MachineReport: %v", err)
	}
	for _, s := range rep.Actions {
		if s.Available || s.Reason != power.ReasonLocked {
			t.Errorf("%s on a locked cluster = %+v", s.Action, s)
		}
	}
}
