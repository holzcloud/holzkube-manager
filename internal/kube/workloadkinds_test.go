package kube_test

import (
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// The other workload kinds (2026-09-19).
//
// Listing Deployments alone describes a cluster nobody runs: the storage layer
// is a DaemonSet, the database is a StatefulSet, the backup is a CronJob. These
// tests are mostly about the differences between them, because flattening those
// into one number is how a screen reports a schedule as an outage.

func mixedCluster(t *testing.T) (*kubesim.Server, *kube.Client) {
	t.Helper()

	return newCluster(t, kubesim.Options{
		Deployments: []kubesim.Deployment{
			{Namespace: "default", Name: "website", Desired: 2, Ready: 2, Image: "web:1"},
		},
		StatefulSets: []kubesim.StatefulSet{
			{Namespace: "db", Name: "postgres", Desired: 3, Ready: 2, Updated: 1, Image: "pg:16"},
		},
		DaemonSets: []kubesim.DaemonSet{
			{Namespace: "longhorn-system", Name: "longhorn-manager", Scheduled: 2, Ready: 1,
				Image: "longhorn:1.7"},
		},
		Jobs: []kubesim.Job{
			{Namespace: "default", Name: "migrate", Succeeded: 1, Image: "migrate:1"},
			{Namespace: "default", Name: "broken-import", Failed: 3, Image: "import:1"},
		},
		CronJobs: []kubesim.CronJob{
			{Namespace: "default", Name: "backup", Schedule: "0 2 * * *",
				LastRun: time.Date(2026, 9, 19, 2, 0, 0, 0, time.UTC), Image: "backup:1"},
			{Namespace: "default", Name: "retired", Schedule: "0 5 * * *", Suspended: true},
		},
	})
}

// TestEverythingThatRunsIsListed, not only the Deployments.
func TestEverythingThatRunsIsListed(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := mixedCluster(t)

	all, err := client.Workloads(ctx, "")
	if err != nil {
		t.Fatalf("Workloads: %v", err)
	}

	kinds := map[kube.WorkloadKind]int{}
	for _, w := range all {
		kinds[w.Kind]++
	}
	for _, want := range []kube.WorkloadKind{
		kube.KindDeployment, kube.KindStatefulSet, kube.KindDaemonSet,
		kube.KindJob, kube.KindCronJob,
	} {
		if kinds[want] == 0 {
			t.Errorf("no %s was listed; a cluster is not only its Deployments", want)
		}
	}
}

// TestTheNumbersMeanWhatTheKindMeans, which is the reason they are not
// flattened into one "ready" column.
func TestTheNumbersMeanWhatTheKindMeans(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := mixedCluster(t)

	all, err := client.Workloads(ctx, "")
	if err != nil {
		t.Fatalf("Workloads: %v", err)
	}
	byName := map[string]kube.Workload{}
	for _, w := range all {
		byName[w.Name] = w
	}

	// A DaemonSet's count is the cluster's shape, so it says nodes and cannot
	// be scaled -- offering that would offer an operation the API server
	// refuses.
	daemon := byName["longhorn-manager"]
	if !strings.Contains(daemon.Summary, "nodes") {
		t.Errorf("daemonset summary = %q, want it to say nodes", daemon.Summary)
	}
	if daemon.Scalable {
		t.Error("a DaemonSet was reported as scalable; its size is how many nodes match")
	}
	if !daemon.Rollable {
		t.Error("a DaemonSet has a pod template and can be rolled")
	}

	// A StatefulSet mid-update is normal, and the summary says which phase
	// rather than implying an outage.
	set := byName["postgres"]
	if !strings.Contains(set.Summary, "one at a time") {
		t.Errorf("statefulset summary = %q, want it to explain the ordered update", set.Summary)
	}

	// A Job's numbers are succeeded and failed, not ready.
	if got := byName["migrate"].Summary; !strings.Contains(got, "succeeded") {
		t.Errorf("job summary = %q", got)
	}
	if got := byName["broken-import"].Summary; !strings.Contains(got, "failed") {
		t.Errorf("failed job summary = %q", got)
	}

	// A CronJob has no pods between runs. "0 ready" would report a schedule as
	// an outage.
	backup := byName["backup"]
	if backup.Schedule != "0 2 * * *" {
		t.Errorf("cronjob schedule = %q", backup.Schedule)
	}
	if strings.Contains(backup.Summary, "ready") {
		t.Errorf("cronjob summary = %q, and a CronJob has nothing running between runs",
			backup.Summary)
	}
	if !strings.Contains(backup.Summary, "last run") {
		t.Errorf("cronjob summary = %q, want when it last ran", backup.Summary)
	}

	// Suspended is not the same as never runs, and it is the finding somebody
	// came for.
	retired := byName["retired"]
	if !retired.Suspended || !strings.Contains(retired.Summary, "suspended") {
		t.Errorf("suspended cronjob = %+v", retired)
	}
}

// TestScalingWhatCannotBeScaledIsRefusedByName.
func TestScalingWhatCannotBeScaledIsRefusedByName(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := mixedCluster(t)

	err := client.ScaleWorkload(ctx, kube.KindDaemonSet, "longhorn-system", "longhorn-manager", 3)
	if err == nil {
		t.Fatal("scaling a DaemonSet was accepted")
	}
	// Named rather than left to the API server's own message: "the server
	// rejected the request" does not tell somebody that the count they wanted
	// is the number of nodes.
	if !strings.Contains(err.Error(), "cluster's shape") {
		t.Errorf("reason = %q, want it to say why a DaemonSet has no replica count", err)
	}

	if err := client.RolloutRestartWorkload(ctx, kube.KindCronJob, "default", "backup",
		time.Now()); err == nil {
		t.Error("rolling a CronJob was accepted; it has no pod template to roll")
	}
}

// TestScalingAStatefulSetGoesThroughItsOwnScaleSubresource.
func TestScalingAStatefulSetGoesThroughItsOwnScaleSubresource(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := mixedCluster(t)

	if err := client.ScaleWorkload(ctx, kube.KindStatefulSet, "db", "postgres", 5); err != nil {
		t.Fatalf("ScaleWorkload: %v", err)
	}
	if got := sim.Calls("PUT /apis/apps/v1/namespaces/db/statefulsets/postgres/scale"); got != 1 {
		t.Errorf("the scale subresource was called %d times; a client that patched the whole "+
			"object could undo a field somebody else had just set", got)
	}
}

// TestTheNamespaceFilterIsTheServers, for every kind.
func TestTheNamespaceFilterIsTheServers(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := mixedCluster(t)

	one, err := client.Workloads(ctx, "db")
	if err != nil {
		t.Fatalf("Workloads: %v", err)
	}
	for _, w := range one {
		if w.Namespace != "db" {
			t.Errorf("%s %s/%s leaked into a filtered list", w.Kind, w.Namespace, w.Name)
		}
	}
	// The filter is the SERVER's, for the reason the pod list's is: a cluster
	// with thousands of workloads must not send all of them so that one can be
	// shown.
	if got := sim.Calls("GET /apis/apps/v1/namespaces/db/statefulsets"); got != 1 {
		t.Errorf("the namespaced statefulset path was requested %d times", got)
	}
	if got := sim.Calls("GET /apis/batch/v1/namespaces/db/cronjobs"); got != 1 {
		t.Errorf("the namespaced cronjob path was requested %d times", got)
	}
}
