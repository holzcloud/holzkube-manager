package power_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
	"github.com/holzcloud/holzkube-manager/internal/power"
)

// One app.

// TestADisabledAppRefusesToStartUntilItIsEnabled: the mark is on the object,
// start refuses while it is there -- on the screen and when pressed -- and
// enable removes it and brings back the three replicas that were running.
func TestADisabledAppRefusesToStartUntilItIsEnabled(t *testing.T) {
	t.Parallel()

	r := newRig(t, []spec{{host: "cp-1", cp: true, endpoint: true}}, kubesim.Options{
		Deployments: []kubesim.Deployment{{Namespace: "default", Name: "web", Desired: 3, Ready: 3}},
	})
	ctx := t.Context()
	web := power.AppRef{Namespace: "default", Kind: power.KindDeployment, Name: "web"}

	msg, err := r.svc.RunApp(ctx, testCluster, web, power.Disable)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !strings.Contains(msg, "stays stopped") {
		t.Errorf("the disable says %q", msg)
	}
	if got := r.kube.Annotations("deployment", "default", "web")[kube.DisabledAnnotation]; got != "true" {
		t.Fatalf("the disabled mark is %q on the object", got)
	}
	if got := r.kube.DesiredReplicas("default", "web"); got != 0 {
		t.Fatalf("a disabled app still wants %d replicas", got)
	}

	rep, err := r.svc.AppReport(ctx, testCluster, web)
	if err != nil {
		t.Fatalf("AppReport: %v", err)
	}
	if rep.State != power.StateDisabled || !rep.Disabled {
		t.Errorf("state = %s disabled = %v", rep.State, rep.Disabled)
	}
	if start := statusOf(t, rep, power.Start); start.Available || start.Reason != power.ReasonDisabled {
		t.Fatalf("start of a disabled app = %+v, want refused with %q", start, power.ReasonDisabled)
	}

	_, err = r.svc.RunApp(ctx, testCluster, web, power.Start)
	var refused *power.UnavailableError
	if !errors.As(err, &refused) || refused.Reason != power.ReasonDisabled {
		t.Fatalf("a start of a disabled app went through: err = %v", err)
	}
	if got := r.kube.DesiredReplicas("default", "web"); got != 0 {
		t.Fatalf("the refused start scaled the app to %d", got)
	}

	if _, err := r.svc.RunApp(ctx, testCluster, web, power.Enable); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if _, left := r.kube.Annotations("deployment", "default", "web")[kube.DisabledAnnotation]; left {
		t.Error("enable left the disabled mark on the object")
	}
	if got := r.kube.DesiredReplicas("default", "web"); got != 3 {
		t.Errorf("after enable the app wants %d replicas, want the 3 it ran", got)
	}
}

// TestADaemonSetAppStopsAndStartsThroughThePowerModel is the DaemonSet round
// trip through the verbs a screen presses.
func TestADaemonSetAppStopsAndStartsThroughThePowerModel(t *testing.T) {
	t.Parallel()

	r := newRig(t, []spec{{host: "cp-1", cp: true, endpoint: true}}, kubesim.Options{
		DaemonSets: []kubesim.DaemonSet{{
			Namespace: "kube-system", Name: "log-shipper", Scheduled: 3, Ready: 3,
			NodeSelector: map[string]string{"kubernetes.io/os": "linux"},
		}},
	})
	ctx := t.Context()
	ds := power.AppRef{Namespace: "kube-system", Kind: power.KindDaemonSet, Name: "log-shipper"}

	if _, err := r.svc.RunApp(ctx, testCluster, ds, power.Stop); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if sel := r.kube.NodeSelector("kube-system", "log-shipper"); sel[kube.StoppedSelectorKey] != "true" ||
		sel["kubernetes.io/os"] != "linux" {
		t.Fatalf("after stop the selector is %v", sel)
	}
	rep, err := r.svc.AppReport(ctx, testCluster, ds)
	if err != nil {
		t.Fatalf("AppReport: %v", err)
	}
	if rep.State != power.StateStopped {
		t.Errorf("a stopped DaemonSet is %s", rep.State)
	}

	if _, err := r.svc.RunApp(ctx, testCluster, ds, power.Start); err != nil {
		t.Fatalf("start: %v", err)
	}
	sel := r.kube.NodeSelector("kube-system", "log-shipper")
	if len(sel) != 1 || sel["kubernetes.io/os"] != "linux" {
		t.Fatalf("after start the selector is %v, want exactly what it had", sel)
	}
}

// TestABarePodHasOnlyAStop: nothing will recreate it, so there is no start, no
// restart, and no disable -- and the screen says so rather than hiding them.
func TestABarePodHasOnlyAStop(t *testing.T) {
	t.Parallel()

	r := newRig(t, []spec{{host: "cp-1", cp: true, endpoint: true}}, kubesim.Options{
		Pods: []kubesim.Pod{{Namespace: "default", Name: "debug", Ready: 1}},
	})
	ctx := t.Context()
	pod := power.AppRef{Namespace: "default", Kind: power.KindPod, Name: "debug"}

	rep, err := r.svc.AppReport(ctx, testCluster, pod)
	if err != nil {
		t.Fatalf("AppReport: %v", err)
	}
	for _, s := range rep.Actions {
		switch s.Action {
		case power.Stop, power.ForceStop:
			if !s.Available {
				t.Errorf("%s of a bare pod = %+v", s.Action, s)
			}
		default:
			if s.Available {
				t.Errorf("%s of a bare pod was offered", s.Action)
			}
		}
	}
	if start := statusOf(t, rep, power.Start); start.Reason != power.ReasonBarePodStart {
		t.Errorf("start says %q, want %q", start.Reason, power.ReasonBarePodStart)
	}

	msg, err := r.svc.RunApp(ctx, testCluster, pod, power.ForceStop)
	if err != nil {
		t.Fatalf("force-stop: %v", err)
	}
	if !strings.Contains(msg, "cannot be started again") {
		t.Errorf("the message %q does not say the pod is gone for good", msg)
	}
	if slices.Contains(r.kube.PodNames(), "default/debug") {
		t.Error("the pod is still there")
	}
	if grace, _ := r.kube.DeleteGrace("default", "debug"); grace == nil || *grace != 0 {
		t.Errorf("force-stop deleted with grace %v, want zero", grace)
	}
}
