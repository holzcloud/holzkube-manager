package kube_test

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// Clearing out what is finished (2026-09-20).
//
// This is a deletion, so the tests are about what is NOT swept. Everything in
// the plan is gone for good afterwards, and "it removed more than I expected" is
// the failure worth preventing.

func litteredCluster(t *testing.T) (*kubesim.Server, *kube.Client) {
	t.Helper()

	return newCluster(t, kubesim.Options{
		Pods: []kubesim.Pod{
			// Finished, and old enough.
			{Namespace: "default", Name: "backup-29271", Phase: corev1.PodSucceeded},
			{Namespace: "default", Name: "import-failed", Phase: corev1.PodFailed},
			// Still doing something. None of these may be touched.
			{Namespace: "default", Name: "website-abc", Phase: corev1.PodRunning},
			{Namespace: "default", Name: "waiting", Phase: corev1.PodPending},
			{Namespace: "default", Name: "lost-contact", Phase: corev1.PodUnknown},
		},
		ReplicaSets: []kubesim.ReplicaSet{
			// The current one, running nothing -- which is what a STOPPED
			// deployment looks like, and the case the "keep the newest" rule
			// exists for. With replicas on it the earlier filter would protect
			// it and this test would prove nothing: measured, it passed against
			// a version with the rule removed.
			{Namespace: "default", Name: "website-newest", Owner: "website",
				Replicas: 0, Age: 2 * time.Hour},
			// Older rollouts, running nothing.
			{Namespace: "default", Name: "website-old", Owner: "website",
				Replicas: 0, Age: 48 * time.Hour},
			{Namespace: "default", Name: "website-older", Owner: "website",
				Replicas: 0, Age: 96 * time.Hour},
		},
	})
}

// TestNothingThatIsStillDoingSomethingIsSwept.
//
// Unknown especially: it means the node stopped reporting, not that the pod
// stopped. Sweeping one would delete a pod that is very likely running fine, on
// the strength of a word that means "we cannot see it".
func TestNothingThatIsStillDoingSomethingIsSwept(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := litteredCluster(t)

	plan, err := client.PlanSweep(ctx, "", time.Now().Add(2*time.Hour))
	if err != nil {
		t.Fatalf("PlanSweep: %v", err)
	}

	swept := map[string]bool{}
	for _, item := range plan.Items {
		swept[item.Name] = true
	}
	for _, safe := range []string{"website-abc", "waiting", "lost-contact"} {
		if swept[safe] {
			t.Errorf("%s is in the plan and it is still doing something", safe)
		}
	}
	for _, gone := range []string{"backup-29271", "import-failed"} {
		if !swept[gone] {
			t.Errorf("%s finished and is not in the plan", gone)
		}
	}
}

// TestTheNewestRolloutIsKept, because it is the way back.
func TestTheNewestRolloutIsKept(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := litteredCluster(t)

	plan, err := client.PlanSweep(ctx, "", time.Now().Add(2*time.Hour))
	if err != nil {
		t.Fatalf("PlanSweep: %v", err)
	}

	swept := map[string]bool{}
	for _, item := range plan.Items {
		swept[item.Name] = true
	}
	if swept["website-newest"] {
		t.Error("the current ReplicaSet is in the plan; it is what a rollback returns to")
	}
	if !swept["website-old"] || !swept["website-older"] {
		t.Error("older rollouts running nothing are not in the plan")
	}

	// Every row says WHY, because a list of names with no reasons is a list
	// nobody can check before pressing the button.
	for _, item := range plan.Items {
		if item.Reason == "" {
			t.Errorf("%s %s has no reason", item.Kind, item.Name)
		}
	}
}

// TestSomethingThatJustFinishedIsLeftAlone: its logs go with it, and somebody
// may be reading them right now.
func TestSomethingThatJustFinishedIsLeftAlone(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := litteredCluster(t)

	// Now, rather than two hours from now: everything here is minutes old.
	plan, err := client.PlanSweep(ctx, "", time.Now())
	if err != nil {
		t.Fatalf("PlanSweep: %v", err)
	}
	for _, item := range plan.Items {
		if item.Kind == "Pod" {
			t.Errorf("%s finished a moment ago and is already in the plan", item.Name)
		}
	}
}

// TestASweepRemovesExactlyWhatWasApproved.
//
// The plan is passed back rather than recomputed: between a plan and an apply
// somebody's CronJob can run, and a sweep that recomputed would remove things
// nobody saw in the list they approved.
func TestASweepRemovesExactlyWhatWasApproved(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := litteredCluster(t)

	removed, failed, err := client.Sweep(ctx, []kube.Sweepable{
		{Kind: "ReplicaSet", Namespace: "default", Name: "website-old"},
	})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if removed != 1 || len(failed) != 0 {
		t.Fatalf("removed=%d failed=%+v", removed, failed)
	}

	got := sim.Deleted()
	if len(got) != 1 || got[0] != "replicasets/default/website-old" {
		t.Errorf("deleted = %v, want exactly the one approved", got)
	}
}

// TestTheNoticeSaysWhatIsNotInTheList, so an empty plan is not read as "there
// is nothing to clean up anywhere" -- images especially, which Talos cannot
// delete at all.
func TestTheNoticeSaysWhatIsNotInTheList(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{})

	plan, err := client.PlanSweep(ctx, "", time.Now())
	if err != nil {
		t.Fatalf("PlanSweep: %v", err)
	}
	if len(plan.Items) != 0 {
		t.Fatalf("plan = %+v, want nothing on an empty cluster", plan.Items)
	}
	if !strings.Contains(plan.Notice, "kubelet") {
		t.Errorf("notice = %q, want it to say who removes images", plan.Notice)
	}
}
