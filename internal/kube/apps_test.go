package kube_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// Apps: what runs, and what it is using right now (2026-09-26).
//
// The fixture is one cluster with one of everything the ownership walk has to
// get right, because the claims are about the walk as a whole: a pod that lands
// under the wrong app is not only missing from its own row, it is inflating
// somebody else's.

const mi = 1024 * 1024

// appsCluster is the homelab the apps tests read. Every figure in it is chosen
// so that the likely mistakes produce a DIFFERENT number rather than the same
// one by coincidence -- see each test for which.
func appsCluster(t *testing.T, adjust func(*kubesim.Options)) (*kubesim.Server, *kube.Client) {
	t.Helper()

	jellyfin := func(name, node, owner string, ready int) kubesim.Pod {
		return kubesim.Pod{
			Namespace: "media", Name: name, Node: node,
			OwnerKind: "ReplicaSet", OwnerName: owner,
			ContainerNames: []string{"app"}, Ready: ready,
			Image:      "jellyfin/jellyfin:10.9",
			CPURequest: "100m", MemoryLimit: "256Mi",
			Labels: map[string]string{"app": "jellyfin"},
		}
	}
	withProxy := jellyfin("jellyfin-7d9c-aaaa", "w-1", "jellyfin-7d9c", 1)
	withProxy.Sidecars = []string{"proxy"}
	withProxy.Restarts = 2
	withProxy.IP = "10.244.1.7"
	plain := jellyfin("jellyfin-7d9c-bbbb", "w-2", "jellyfin-7d9c", 0)
	plain.IP = "10.244.2.9"
	previous := jellyfin("jellyfin-5f00-old", "w-1", "jellyfin-5f00", 0)
	previous.Phase = corev1.PodFailed

	opts := kubesim.Options{
		Nodes: []kubesim.Node{{Name: "cp-1"}, {Name: "w-1"}, {Name: "w-2"}},
		Deployments: []kubesim.Deployment{
			{Namespace: "media", Name: "jellyfin", Desired: 2, Ready: 1, Image: "jellyfin/jellyfin:10.9"},
		},
		ReplicaSets: []kubesim.ReplicaSet{
			{Namespace: "media", Name: "jellyfin-7d9c", Owner: "jellyfin", Replicas: 2},
			{Namespace: "media", Name: "jellyfin-5f00", Owner: "jellyfin", Age: time.Hour},
			// Made by hand: no Deployment manages it.
			{Namespace: "kube-system", Name: "handmade", Replicas: 1},
		},
		StatefulSets: []kubesim.StatefulSet{
			{Namespace: "db", Name: "postgres", Desired: 1, Ready: 1, Image: "postgres:16"},
		},
		DaemonSets: []kubesim.DaemonSet{
			{Namespace: "kube-system", Name: "cilium", Scheduled: 3, Ready: 3, Image: "cilium:1.16"},
		},
		Jobs: []kubesim.Job{
			{Namespace: "ops", Name: "backup-29271", Owner: "backup", Active: 1, Image: "restic:0.17"},
			{Namespace: "ops", Name: "backup-29270", Owner: "backup", Succeeded: 1, Image: "restic:0.17"},
			{Namespace: "db", Name: "migrate", Active: 1, Image: "migrate:3"},
		},
		CronJobs: []kubesim.CronJob{
			{Namespace: "ops", Name: "backup", Schedule: "0 * * * *", Image: "restic:0.17"},
			{Namespace: "ops", Name: "report", Schedule: "0 6 * * 1", Image: "report:1"},
		},
		Pods: []kubesim.Pod{
			withProxy, plain, previous,
			{Namespace: "ops", Name: "backup-29271-xyz", Node: "w-1", OwnerKind: "Job",
				OwnerName: "backup-29271", ContainerNames: []string{"restic"}, Ready: 1,
				Image: "restic:0.17"},
			{Namespace: "ops", Name: "backup-29270-abc", Node: "w-1", OwnerKind: "Job",
				OwnerName: "backup-29270", ContainerNames: []string{"restic"},
				Phase: corev1.PodSucceeded, Image: "restic:0.17"},
			{Namespace: "db", Name: "postgres-0", Node: "w-2", OwnerKind: "StatefulSet",
				OwnerName: "postgres", ContainerNames: []string{"postgres"}, Ready: 1,
				Image: "postgres:16"},
			{Namespace: "db", Name: "migrate-abc", Node: "w-2", OwnerKind: "Job",
				OwnerName: "migrate", ContainerNames: []string{"migrate"}, Ready: 1,
				Image: "migrate:3"},
			{Namespace: "kube-system", Name: "cilium-a", Node: "w-1", OwnerKind: "DaemonSet",
				OwnerName: "cilium", Ready: 1, Image: "cilium:1.16"},
			{Namespace: "kube-system", Name: "cilium-b", Node: "w-2", OwnerKind: "DaemonSet",
				OwnerName: "cilium", Ready: 1, Image: "cilium:1.16"},
			{Namespace: "kube-system", Name: "cilium-c", Node: "cp-1", OwnerKind: "DaemonSet",
				OwnerName: "cilium", Ready: 1, Image: "cilium:1.16"},
			// The Talos control plane: a static pod, whose controller reference
			// is the Node it runs on.
			{Namespace: "kube-system", Name: "kube-apiserver-cp-1", Node: "cp-1", Mirror: true,
				OwnerKind: "Node", OwnerName: "cp-1", Ready: 1, Image: "kube-apiserver:v1.34.1"},
			// A StatefulSet by name, from somebody else's group.
			{Namespace: "kube-system", Name: "kruise-web-0", Node: "w-1", OwnerKind: "StatefulSet",
				OwnerName: "kruise-web", OwnerAPIVersion: "apps.kruise.io/v1beta1", Ready: 1},
			{Namespace: "kube-system", Name: "handmade-x", Node: "w-1", OwnerKind: "ReplicaSet",
				OwnerName: "handmade", Ready: 1},
			{Namespace: "default", Name: "debug", Node: "w-1", Ready: 1, Image: "busybox"},
			{Namespace: "default", Name: "done", Node: "w-1", Phase: corev1.PodSucceeded},
		},
		Summary: map[string]map[string]string{
			// 150m + 50m: the sidecar is half of what makes this 200m.
			"media/jellyfin-7d9c-aaaa": {"app": "150m,100Mi", "proxy": "50m,28Mi"},
			// Nanocores that are not a whole millicore, and a pod-level figure
			// (the pod cgroup, overhead included) that disagrees with its one
			// container. The container is what metrics-server reports.
			"media/jellyfin-7d9c-bbbb": {"app": "123456789n,64Mi", "": "999m,1Gi"},
			// Failed, and still in the kubelet's summary for a moment. History.
			"media/jellyfin-5f00-old": {"app": "900m,900Mi"},
			"ops/backup-29271-xyz":    {"restic": "10m,20Mi"},
			"ops/backup-29270-abc":    {"restic": "700m,700Mi"},
			"db/postgres-0":           {"postgres": "250m,512Mi"},
			// A container with no CPU rate yet (it needs two samples), and a
			// pod-level figure that has one. The pod's is then the answer.
			"kube-system/cilium-a": {"container-0": ",30Mi", "": "5m,40Mi"},
			// migrate-abc has no entry: the kubelet has not measured it yet.
		},
	}
	if adjust != nil {
		adjust(&opts)
	}
	return newCluster(t, opts)
}

func appsByKey(apps []kube.App) map[string]kube.App {
	out := map[string]kube.App{}
	for _, app := range apps {
		out[app.Kind+" "+app.Namespace+"/"+app.Name] = app
	}
	return out
}

// TestEveryPodLandsUnderTheAppThatOwnsIt.
//
// Measured against three wrong walks: stopping at the ReplicaSet (every
// Deployment becomes a pile of pods), stopping at the Job (every CronJob run
// becomes an app of its own), and taking ANY controller reference at its word
// -- which lists the Talos API server as an app called "cp-1" and the pods of an
// OpenKruise StatefulSet under the built-in kind. Each fails here.
func TestEveryPodLandsUnderTheAppThatOwnsIt(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := appsCluster(t, nil)

	answer, err := client.Apps(ctx, kube.AppsQuery{}, time.Now())
	if err != nil {
		t.Fatalf("Apps: %v", err)
	}

	got := map[string]int{}
	for key, app := range appsByKey(answer.Apps) {
		got[key] = app.Pods
	}
	want := map[string]int{
		"Deployment media/jellyfin": 2,
		"CronJob ops/backup":        1,
		// Between runs, and still an app: a schedule is not an absence.
		"CronJob ops/report":           0,
		"StatefulSet db/postgres":      1,
		"Job db/migrate":               1,
		"DaemonSet kube-system/cilium": 3,
		// The control plane as what it is, not as an app called cp-1.
		"Pod kube-system/kube-apiserver-cp-1": 1,
		// Controllers this product does not follow: the pod stands for itself.
		"Pod kube-system/kruise-web-0": 1,
		"Pod kube-system/handmade-x":   1,
		"Pod default/debug":            1,
	}

	for key, pods := range want {
		have, ok := got[key]
		if !ok {
			t.Errorf("%s is not in the list", key)
			continue
		}
		if have != pods {
			t.Errorf("%s has %d pods, want %d", key, have, pods)
		}
	}
	for key := range got {
		if _, ok := want[key]; !ok {
			t.Errorf("%s is listed and is not an app of this cluster. Everything listed: %v",
				key, slices.Sorted(func(yield func(string) bool) {
					for k := range got {
						if !yield(k) {
							return
						}
					}
				}))
		}
	}
}

// TestAnAppIsWhatItsRunningPodsUseNow.
//
// Each expected figure fails a specific mistake: 324m and not 323m is the
// rounding kubectl top uses; not 1199m is the pod-level figure winning over the
// containers; not 274m is the sidecar forgotten; one pod and not two for the
// backup is a finished run counted as load. And migrate has no figures at all,
// which must read as unknown -- never as a confident zero.
func TestAnAppIsWhatItsRunningPodsUseNow(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := appsCluster(t, nil)

	answer, err := client.Apps(ctx, kube.AppsQuery{}, time.Now())
	if err != nil {
		t.Fatalf("Apps: %v", err)
	}
	if !answer.UsageAvailable || answer.Notice != "" {
		t.Errorf("usage_available = %v, notice = %q; every kubelet answered",
			answer.UsageAvailable, answer.Notice)
	}
	apps := appsByKey(answer.Apps)

	jellyfin := apps["Deployment media/jellyfin"]
	if !jellyfin.UsageKnown || jellyfin.CPUMillis != 324 || jellyfin.MemoryBytes != 192*mi {
		t.Errorf("jellyfin uses %dm and %d bytes (known %v), want 324m and 192Mi: 200m and 128Mi "+
			"from the pod with the sidecar, 123.456789m rounded up and 64Mi from the other, and "+
			"nothing from the one that failed", jellyfin.CPUMillis, jellyfin.MemoryBytes,
			jellyfin.UsageKnown)
	}
	if jellyfin.Pods != 2 || jellyfin.Ready != 1 || jellyfin.Restarts != 2 {
		t.Errorf("jellyfin: %d pods, %d ready, %d restarts; want 2, 1, 2",
			jellyfin.Pods, jellyfin.Ready, jellyfin.Restarts)
	}
	if !slices.Equal(jellyfin.Nodes, []string{"w-1", "w-2"}) {
		t.Errorf("jellyfin runs on %v", jellyfin.Nodes)
	}
	// Three running containers -- two app, one sidecar -- at 100m and 256Mi each.
	if jellyfin.CPURequestMillis != 300 || jellyfin.MemoryLimitBytes != 3*256*mi ||
		jellyfin.CPULimitMillis != 0 || jellyfin.MemoryRequestBytes != 0 {
		t.Errorf("jellyfin asked for %dm (limit %dm) and %d bytes (limit %d); want 300m, "+
			"unset, unset, 768Mi", jellyfin.CPURequestMillis, jellyfin.CPULimitMillis,
			jellyfin.MemoryRequestBytes, jellyfin.MemoryLimitBytes)
	}
	if !slices.Equal(jellyfin.Images, []string{"jellyfin/jellyfin:10.9"}) {
		t.Errorf("jellyfin images = %v", jellyfin.Images)
	}

	backup := apps["CronJob ops/backup"]
	if backup.Pods != 1 || backup.CPUMillis != 10 || backup.MemoryBytes != 20*mi {
		t.Errorf("backup: %d pods using %dm and %d bytes; want the one running run at 10m and "+
			"20Mi, and not the finished one at 700m", backup.Pods, backup.CPUMillis, backup.MemoryBytes)
	}

	report := apps["CronJob ops/report"]
	if report.UsageKnown || report.CPUMillis != 0 {
		t.Errorf("report runs nothing and reads %+v", report)
	}
	// Read off the template, because there is no pod to read it off.
	if !slices.Equal(report.Images, []string{"report:1"}) {
		t.Errorf("report images = %v, want what it would run", report.Images)
	}

	migrate := apps["Job db/migrate"]
	if migrate.UsageKnown {
		t.Errorf("migrate has no figures from its kubelet and reads as known: %+v", migrate)
	}

	// A container the kubelet has no rate for yet makes the containers an
	// incomplete answer, so the pod's own figure is used rather than the
	// container's memory alone passed off as the pod's.
	cilium := apps["DaemonSet kube-system/cilium"]
	if !cilium.UsageKnown || cilium.CPUMillis != 5 || cilium.MemoryBytes != 40*mi {
		t.Errorf("cilium = %dm, %d bytes, known %v; want the pod-level 5m and 40Mi",
			cilium.CPUMillis, cilium.MemoryBytes, cilium.UsageKnown)
	}

	// Heaviest first, then by namespace and name.
	var order []string
	for _, app := range answer.Apps[:3] {
		order = append(order, app.Name)
	}
	if !slices.Equal(order, []string{"jellyfin", "postgres", "backup"}) {
		t.Errorf("the heaviest three are %v, want jellyfin, postgres, backup", order)
	}
}

// TestANodesPageSeesOnlyWhatRunsThere.
//
// Both halves: membership AND the sums. A DaemonSet's row on one node's page
// counts that node's pod, not all three -- the node page asks what runs here and
// what it uses here. And only that node's kubelet is asked.
func TestANodesPageSeesOnlyWhatRunsThere(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := appsCluster(t, nil)

	answer, err := client.Apps(ctx, kube.AppsQuery{Node: "w-2"}, time.Now())
	if err != nil {
		t.Fatalf("Apps: %v", err)
	}
	apps := appsByKey(answer.Apps)

	listed := slices.Sorted(func(yield func(string) bool) {
		for key := range apps {
			if !yield(key) {
				return
			}
		}
	})
	want := []string{
		"DaemonSet kube-system/cilium", "Deployment media/jellyfin",
		"Job db/migrate", "StatefulSet db/postgres",
	}
	if !slices.Equal(listed, want) {
		t.Errorf("on w-2: %v, want %v -- nothing without a pod there, not even a CronJob "+
			"that is between runs", listed, want)
	}

	jellyfin := apps["Deployment media/jellyfin"]
	if jellyfin.Pods != 1 || jellyfin.CPUMillis != 124 || jellyfin.MemoryBytes != 64*mi ||
		jellyfin.CPURequestMillis != 100 || !slices.Equal(jellyfin.Nodes, []string{"w-2"}) {
		t.Errorf("jellyfin on w-2 = %+v, want its one pod there: 124m, 64Mi, 100m requested", jellyfin)
	}
	if cilium := apps["DaemonSet kube-system/cilium"]; cilium.Pods != 1 {
		t.Errorf("cilium on w-2 has %d pods, want the one that is there", cilium.Pods)
	}

	if got := sim.Calls("GET /api/v1/nodes/w-2/proxy/stats/summary"); got != 1 {
		t.Errorf("w-2's kubelet was asked %d times", got)
	}
	for _, other := range []string{"w-1", "cp-1"} {
		if got := sim.Calls("GET /api/v1/nodes/" + other + "/proxy/stats/summary"); got != 0 {
			t.Errorf("%s's kubelet was asked %d times for a page about w-2", other, got)
		}
	}
}

// TestAKubeletThatDoesNotAnswerCostsOnlyItsOwnPods.
//
// A powered-off node is the ordinary state of a homelab. Its pods read unknown
// -- not zero -- everything else is still live, and the notice names the node
// so "why does postgres show nothing" has an answer on the same screen.
func TestAKubeletThatDoesNotAnswerCostsOnlyItsOwnPods(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := appsCluster(t, func(o *kubesim.Options) { o.SummaryFails = []string{"w-2"} })

	answer, err := client.Apps(ctx, kube.AppsQuery{}, time.Now())
	if err != nil {
		t.Fatalf("Apps: %v", err)
	}
	apps := appsByKey(answer.Apps)

	if !answer.UsageAvailable {
		t.Error("usage_available is false although two of three kubelets answered")
	}
	if !strings.Contains(answer.Notice, "w-2") {
		t.Errorf("notice = %q, want it to name w-2", answer.Notice)
	}
	// And say why, in the API server's words rather than client-go's generic
	// sentence for a 503: "no route to host" is a node that is off.
	if !strings.Contains(answer.Notice, "no route to host") {
		t.Errorf("notice = %q, want the reason the API server gave", answer.Notice)
	}
	if strings.Contains(answer.Notice, "w-1") || strings.Contains(answer.Notice, "cp-1") {
		t.Errorf("notice = %q names a node that answered", answer.Notice)
	}

	if postgres := apps["StatefulSet db/postgres"]; postgres.UsageKnown || postgres.CPUMillis != 0 {
		t.Errorf("postgres runs only on w-2 and reads %dm, known %v; want unknown",
			postgres.CPUMillis, postgres.UsageKnown)
	}
	jellyfin := apps["Deployment media/jellyfin"]
	if !jellyfin.UsageKnown || jellyfin.CPUMillis != 200 || jellyfin.MemoryBytes != 128*mi {
		t.Errorf("jellyfin = %dm, %d bytes, known %v; want its w-1 pod's 200m and 128Mi, still "+
			"counted", jellyfin.CPUMillis, jellyfin.MemoryBytes, jellyfin.UsageKnown)
	}
}

// TestWhenNoKubeletAnswersMetricsServerIsAsked.
//
// The same measurement from somewhere else, and said so. Per pod only: the
// containers are unknown rather than a pod's figure divided among them.
func TestWhenNoKubeletAnswersMetricsServerIsAsked(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := appsCluster(t, func(o *kubesim.Options) {
		o.SummaryFails = []string{"cp-1", "w-1", "w-2"}
		o.Usage = map[string]string{
			"pod/media/jellyfin-7d9c-aaaa": "400m,512Mi",
			"pod/db/postgres-0":            "100m,256Mi",
		}
	})

	answer, err := client.Apps(ctx, kube.AppsQuery{}, time.Now())
	if err != nil {
		t.Fatalf("Apps: %v", err)
	}
	if !answer.UsageAvailable || !strings.Contains(answer.Notice, "metrics-server") {
		t.Errorf("usage_available = %v, notice = %q; want the fallback, named",
			answer.UsageAvailable, answer.Notice)
	}
	apps := appsByKey(answer.Apps)
	if jellyfin := apps["Deployment media/jellyfin"]; !jellyfin.UsageKnown || jellyfin.CPUMillis != 400 {
		t.Errorf("jellyfin = %+v, want metrics-server's 400m", jellyfin)
	}

	detail, err := client.AppDetail(ctx, "media", "Deployment", "jellyfin", time.Now())
	if err != nil {
		t.Fatalf("AppDetail: %v", err)
	}
	for _, pod := range detail.Pods {
		if pod.Name != "jellyfin-7d9c-aaaa" {
			continue
		}
		if !pod.UsageKnown || pod.CPUMillis != 400 {
			t.Errorf("pod = %+v, want 400m from metrics-server", pod)
		}
		for _, c := range pod.Containers {
			if c.UsageKnown {
				t.Errorf("container %s reads as measured, and metrics-server's figure is per pod", c.Name)
			}
		}
	}
}

// TestNobodyMeasuringIsNotAClusterUsingNothing is INV-08 for usage.
func TestNobodyMeasuringIsNotAClusterUsingNothing(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := appsCluster(t, func(o *kubesim.Options) {
		o.SummaryFails = []string{"cp-1", "w-1", "w-2"}
		// And no metrics-server, which is the default and most clusters.
	})

	answer, err := client.Apps(ctx, kube.AppsQuery{}, time.Now())
	if err != nil {
		t.Fatalf("Apps: %v -- the list is true without the numbers", err)
	}
	if answer.UsageAvailable {
		t.Error("usage_available is true with every kubelet down and no metrics-server")
	}
	if !strings.Contains(answer.Notice, "no metrics-server") ||
		!strings.Contains(answer.Notice, "not the same as usage being zero") {
		t.Errorf("notice = %q, want it to say nobody is measuring, and that this is not zero",
			answer.Notice)
	}
	for _, app := range answer.Apps {
		if app.UsageKnown {
			t.Errorf("%s %s reads as measured", app.Kind, app.Name)
		}
	}
	if len(answer.Apps) != 10 {
		t.Errorf("%d apps listed, want all ten regardless of the numbers", len(answer.Apps))
	}
}

// TestAKubeletThatNeverAnswersIsCutOffAndNamed.
//
// Not parallel: it shortens the shared deadline, which is package state (see
// kube.SetSummaryBudget). Without the deadline this waits for the test's own
// thirty seconds, which is what the route would have done to its forty-five.
func TestAKubeletThatNeverAnswersIsCutOffAndNamed(t *testing.T) {
	restore := kube.SetSummaryBudget(300 * time.Millisecond)
	defer restore()

	ctx := testContext(t)
	_, client := appsCluster(t, func(o *kubesim.Options) { o.SummaryHangs = []string{"w-2"} })

	started := time.Now()
	answer, err := client.Apps(ctx, kube.AppsQuery{}, time.Now())
	took := time.Since(started)
	if err != nil {
		t.Fatalf("Apps: %v", err)
	}
	if took > 5*time.Second {
		t.Errorf("the list took %s; a kubelet that never answers must cost the deadline and "+
			"nothing more", took)
	}
	if !strings.Contains(answer.Notice, "w-2") || !strings.Contains(answer.Notice, "no answer within") {
		t.Errorf("notice = %q, want w-2 named as not having answered in time", answer.Notice)
	}
	if jellyfin := appsByKey(answer.Apps)["Deployment media/jellyfin"]; jellyfin.CPUMillis != 200 {
		t.Errorf("jellyfin = %dm, want w-1's 200m still counted", jellyfin.CPUMillis)
	}
}

// TestAnAppsDetailIsItsPodsItsServicesAndWhatHappenedToIt.
func TestAnAppsDetailIsItsPodsItsServicesAndWhatHappenedToIt(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	base := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	_, client := appsCluster(t, func(o *kubesim.Options) {
		o.Services = []kubesim.Service{
			{Namespace: "media", Name: "jellyfin", ClusterIP: "10.96.0.10",
				Selector: map[string]string{"app": "jellyfin"},
				Ports:    []kubesim.ServicePort{{Port: 8096}}},
			{Namespace: "media", Name: "jellyfin-headless", ClusterIP: "None",
				Selector: map[string]string{"app": "jellyfin"}},
			// Selects nothing: its endpoints are kept by hand. An empty
			// selector read as a label selector would match every pod.
			{Namespace: "media", Name: "manual", ClusterIP: "10.96.0.11"},
			{Namespace: "media", Name: "sonarr", ClusterIP: "10.96.0.12",
				Selector: map[string]string{"app": "sonarr"}},
			// The same selector in another namespace selects other pods.
			{Namespace: "ops", Name: "jellyfin", ClusterIP: "10.96.0.13",
				Selector: map[string]string{"app": "jellyfin"}},
		}
		o.Events = []kubesim.Event{
			{Namespace: "media", Type: "Warning", Reason: "OOMKilled", Kind: "Pod",
				Name: "jellyfin-5f00-old", LastSeen: base},
			{Namespace: "media", Type: "Normal", Reason: "ScalingReplicaSet", Kind: "Deployment",
				Name: "jellyfin", LastSeen: base.Add(time.Minute)},
			{Namespace: "media", Type: "Normal", Reason: "SuccessfulCreate", Kind: "ReplicaSet",
				Name: "jellyfin-7d9c", LastSeen: base.Add(2 * time.Minute)},
			{Namespace: "media", Type: "Warning", Reason: "BackOff", Kind: "Pod",
				Name: "jellyfin-7d9c-bbbb", Count: 3, LastSeen: base.Add(3 * time.Minute)},
			// Somebody else's, and newer than all of the above.
			{Namespace: "media", Type: "Warning", Reason: "BackOff", Kind: "Pod",
				Name: "sonarr-1", LastSeen: base.Add(4 * time.Minute)},
			{Namespace: "media", Type: "Normal", Reason: "ScalingReplicaSet", Kind: "Deployment",
				Name: "jellyfin-canary", LastSeen: base.Add(5 * time.Minute)},
		}
	})

	detail, err := client.AppDetail(ctx, "media", "Deployment", "jellyfin", time.Now())
	if err != nil {
		t.Fatalf("AppDetail: %v", err)
	}

	if detail.App.CPUMillis != 324 || detail.App.Pods != 2 {
		t.Errorf("app = %+v, want the same row the list shows", detail.App)
	}
	if detail.CreatedAt == "" {
		t.Error("created_at is empty")
	}

	var services []string
	for _, s := range detail.Services {
		services = append(services, s.Name)
	}
	if !slices.Equal(services, []string{"jellyfin", "jellyfin-headless"}) {
		t.Errorf("services = %v, want the two in media that select its pods", services)
	}
	if len(detail.Services) > 0 && (!slices.Equal(detail.Services[0].Ports, []string{"8096/TCP"}) ||
		detail.Services[0].ClusterIP != "10.96.0.10" || detail.Services[0].Type != "ClusterIP") {
		t.Errorf("service = %+v", detail.Services[0])
	}

	var reasons []string
	for _, e := range detail.Events {
		reasons = append(reasons, e.Object+" "+e.Reason)
	}
	wantEvents := []string{
		"Pod/jellyfin-7d9c-bbbb BackOff",
		"ReplicaSet/jellyfin-7d9c SuccessfulCreate",
		"Deployment/jellyfin ScalingReplicaSet",
		// The failed pod is not counted, and what happened to it is still the
		// app's story.
		"Pod/jellyfin-5f00-old OOMKilled",
	}
	if !slices.Equal(reasons, wantEvents) {
		t.Errorf("events = %v, want %v, newest first", reasons, wantEvents)
	}

	if len(detail.Pods) != 2 {
		t.Fatalf("pods = %+v, want the two running ones", detail.Pods)
	}
	first := detail.Pods[0]
	if first.Name != "jellyfin-7d9c-aaaa" || first.Node != "w-1" || first.IP != "10.244.1.7" ||
		first.Ready != "2/2" || first.Restarts != 2 || first.StartedAt == "" ||
		first.CPUMillis != 200 || first.MemoryBytes != 128*mi || !first.UsageKnown {
		t.Errorf("first pod = %+v", first)
	}
	var containers []string
	for _, c := range first.Containers {
		containers = append(containers, c.Name)
		if !c.UsageKnown || c.CPURequestMillis != 100 || c.MemoryLimitBytes != 256*mi {
			t.Errorf("container %+v", c)
		}
	}
	if !slices.Equal(containers, []string{"proxy", "app"}) {
		t.Errorf("containers = %v, want the sidecar and the app", containers)
	}
	// The sidecar's state is the container detail's own word for it, and the
	// app's figure is its own rather than the pod's.
	if first.Containers[0].State != "running" || first.Containers[0].CPUMillis != 50 {
		t.Errorf("sidecar = %+v", first.Containers[0])
	}
	if first.Containers[1].CPUMillis != 150 || first.Containers[1].MemoryBytes != 100*mi {
		t.Errorf("app container = %+v", first.Containers[1])
	}
	if second := detail.Pods[1]; second.Ready != "0/1" || second.CPUMillis != 124 {
		t.Errorf("second pod = %+v", second)
	}
}

// TestAnAppThatIsNotThereIsNotFound, with the same error every other named
// object's route turns into a 404.
func TestAnAppThatIsNotThereIsNotFound(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := appsCluster(t, nil)

	for _, ask := range []struct{ namespace, kind, name string }{
		{"media", "Deployment", "ghost"},
		{"media", "Frobnicator", "jellyfin"},
		// The right name in the wrong namespace.
		{"ops", "Deployment", "jellyfin"},
		// Finished and owned by nothing: history, not an app.
		{"default", "Pod", "done"},
	} {
		_, err := client.AppDetail(ctx, ask.namespace, ask.kind, ask.name, time.Now())
		if !errors.Is(err, kube.ErrNoSuchWorkload) {
			t.Errorf("%s %s/%s: err = %v, want ErrNoSuchWorkload", ask.kind, ask.namespace,
				ask.name, err)
		}
	}

	// And the kind is forgiven its case: a path somebody typed is not a
	// different app. The kind as the list writes it first, then two a person
	// might type.
	for _, kind := range []string{"CronJob", "cronjob", "CRONJOB"} {
		if _, err := client.AppDetail(ctx, "ops", kind, "report", time.Now()); err != nil {
			t.Errorf("a CronJob between runs, asked as %q: %v", kind, err)
		}
	}
}

// TestNoAppsAnswerContainsANull is ledger 162 for the two new answers: the
// cases where a list is empty are the ones where a nil slice slips through.
func TestNoAppsAnswerContainsANull(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{Name: "w-1"}},
		// No image, no pods, no services, no events.
		CronJobs: []kubesim.CronJob{{Namespace: "ops", Name: "report", Schedule: "0 6 * * 1"}},
		Pods: []kubesim.Pod{
			// Not scheduled: no node to list and nobody to ask.
			{Namespace: "default", Name: "waiting", Phase: corev1.PodPending,
				Labels: map[string]string{"app": "waiting"}},
		},
		Services: []kubesim.Service{
			{Namespace: "default", Name: "portless", Selector: map[string]string{"app": "waiting"}},
		},
	})

	answers := map[string]func() (any, error){
		"apps": func() (any, error) { return client.Apps(ctx, kube.AppsQuery{}, time.Now()) },
		"apps on a node with nothing": func() (any, error) {
			return client.Apps(ctx, kube.AppsQuery{Node: "w-1"}, time.Now())
		},
		"a CronJob between runs": func() (any, error) {
			return client.AppDetail(ctx, "ops", "CronJob", "report", time.Now())
		},
		"a pod that is waiting": func() (any, error) {
			return client.AppDetail(ctx, "default", "Pod", "waiting", time.Now())
		},
	}
	for name, answer := range answers {
		value, err := answer()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshalling %s: %v", name, err)
		}
		if path := findNull(encoded); path != "" {
			t.Errorf("%s answers null at %s:\n%s", name, path, encoded)
		}
	}
}
