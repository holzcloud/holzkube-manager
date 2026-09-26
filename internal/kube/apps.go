package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Apps: what runs, and what it is using right now (2026-09-26).
//
// # The question the other screens answer in pieces
//
// "What is running on this cluster and what is it eating" is one question, and
// until now it took four screens to answer: the workload list knows the
// Deployment and not its pods, the pod list knows the pods and not whose they
// are, the usage route knows numbers per pod with a hash in the name, and the
// node detail knows where. An operator adding those up by hand is doing the
// arithmetic this file exists to do once.
//
// So an app here is the TOP-LEVEL controller -- the thing somebody installed --
// with every pod it runs folded into it: a Deployment through its ReplicaSets, a
// CronJob through its Jobs, a StatefulSet and a DaemonSet directly, a Job nobody
// schedules on its own, and a pod nothing owns under its own name. A mirror pod
// is listed the same way: the Talos control plane (kube-apiserver and the rest)
// is static pods the kubelet runs from files, and the controller reference each
// carries points at the Node, which is not an app anybody installed.
//
// # Why the chain stops at the kinds this product knows
//
// Ownership is followed through apps/v1 and batch/v1 and nowhere else. A pod
// whose controller is something else -- an operator's own kind, a ReplicaSet no
// Deployment manages -- is reported as itself, as a Pod, rather than under a
// kind the screens have never heard of. The alternative was inventing a kind per
// CustomResourceDefinition, and a screen that parses a fixed set of kinds would
// then refuse the WHOLE list over one operator's pods: the ledger 162 shape, one
// unexpected value turning a page into a parse error.
//
// The group is checked and not only the kind. OpenKruise ships its own
// `StatefulSet` and `DaemonSet` in apps.kruise.io; reading those as apps/v1
// would name the app correctly and then look for it in the wrong list.
//
// # Where the numbers come from, and why not metrics-server
//
// Talos does not ship metrics-server, so on the operator's cluster the usage
// route has always said "nobody is collecting this". But somebody IS: every
// kubelet measures every container it runs, all the time, and serves it at
// /stats/summary. The API server reaches it through the node proxy with the
// credentials this product already has. That is the same number metrics-server
// would have scraped, read at the source and without installing anything --
// `kubectl get --raw /api/v1/nodes/<n>/proxy/stats/summary` is the manual
// version of it.
//
// One read per node, concurrently and bounded, under ONE shared deadline: a
// powered-off node is the ordinary state of a homelab, and how long the API
// server's own dial to a dead kubelet takes is its setting, not this product's.
// A node that does not answer costs its pods their numbers, which then read as
// unknown and never as zero, and the notice names it (INV-08: a node that was
// not heard from is not a node with nothing on it).
//
// Only when EVERY kubelet fails does this fall back to metrics.k8s.io, which on
// a cluster that has it is the same measurement, averaged over its own window.
// And when that is absent too, the answer says nobody is measuring -- which is a
// different sentence from "these apps use nothing".

// AppKindPod is the kind of an app that is a pod on its own: one nothing owns,
// a static pod, or one whose controller is not a kind this product follows.
const AppKindPod = "Pod"

// SummaryBudget is the shared ceiling on reading every node's usage summary.
//
// Shared, not per node, and that is the property: with a per-node timeout,
// every round of dead nodes beyond the first summaryConcurrency would add
// another timeout, and on a large enough cluster the route's own budget would
// run out before the list is written. With one ceiling for all of them the list
// always arrives, and whatever had not answered by then is named in the notice.
// Exported because cmd/holzkube-managerd/budget_test.go composes route budgets
// out of the constants rather than restating them.
const SummaryBudget = 10 * time.Second

// summaryConcurrency is how many kubelets are asked at once. Enough that a
// homelab's handful of nodes goes in one round, few enough that a large cluster
// does not open a hundred proxied connections through its API server for one
// screen.
const summaryConcurrency = 8

// summaryBudget is SummaryBudget as the reader uses it: a variable only so the
// package's own test can prove a hanging kubelet is cut off without waiting ten
// seconds to do it.
var summaryBudget = SummaryBudget

// App is one thing that runs, with its pods folded in.
//
// Every number is over the pods that are still doing something. A Succeeded or
// Failed pod is history: counting a finished Job's hundred pods as load is how a
// CronJob that runs every minute would come to look like the biggest thing in
// the cluster.
type App struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`

	Pods     int   `json:"pods"`
	Ready    int   `json:"ready"`
	Restarts int32 `json:"restarts"`

	// Nodes it runs on, sorted. The question "what dies with w-2" is this
	// column read the other way round.
	Nodes []string `json:"nodes"`

	// What it is using now, summed over the pods whose usage is known. Read
	// with UsageKnown: zero with UsageKnown false is "nobody said", not idle.
	CPUMillis   int64 `json:"cpu_millis"`
	MemoryBytes int64 `json:"memory_bytes"`

	// What it asked for and may not exceed, summed over its running
	// containers. Zero means unset -- which for a limit means "unbounded", and
	// that is itself worth seeing next to the usage.
	CPURequestMillis   int64 `json:"cpu_request_millis"`
	CPULimitMillis     int64 `json:"cpu_limit_millis"`
	MemoryRequestBytes int64 `json:"memory_request_bytes"`
	MemoryLimitBytes   int64 `json:"memory_limit_bytes"`

	UsageKnown bool `json:"usage_known"`

	Images []string `json:"images"`
}

// Apps is the whole list, with how it was measured.
type Apps struct {
	CollectedAt string `json:"collected_at"`

	// UsageAvailable is false only when nothing at all could be measured: no
	// kubelet answered and there is no metrics-server either. It is the one
	// case where every usage number on the screen means "nobody is
	// collecting", and a screen has to be able to say that instead of drawing
	// a column of zeroes.
	UsageAvailable bool   `json:"usage_available"`
	Notice         string `json:"notice"`

	Apps []App `json:"apps"`
}

// AppsQuery narrows the list.
type AppsQuery struct {
	// Namespace is the API server's own filter, so a namespace view does not
	// fetch the whole cluster to show one corner of it.
	Namespace string

	// Node keeps the apps with a pod on that node, and counts them over those
	// pods only. The node page asks "what runs HERE and what does it use
	// here", and a DaemonSet's cluster-wide total is the wrong answer to it.
	Node string
}

// AppDetail is one app, down to its containers.
type AppDetail struct {
	CollectedAt    string `json:"collected_at"`
	UsageAvailable bool   `json:"usage_available"`
	Notice         string `json:"notice"`

	App       App    `json:"app"`
	CreatedAt string `json:"created_at"`

	Pods     []AppPod     `json:"pods"`
	Services []AppService `json:"services"`

	// Events about the app, its ReplicaSets or Jobs, and its pods, newest
	// first. Finished pods' events are included although the pods are not
	// counted: a Job's failure is told in the events of the pod that failed.
	Events []Event `json:"events"`
}

// AppPod is one pod of an app.
type AppPod struct {
	Name  string `json:"name"`
	Node  string `json:"node"`
	Phase string `json:"phase"`

	// Ready as "1/2", the way kubectl writes it. A string here and a count on
	// the app, because here it is one pod's containers and there it is pods.
	Ready     string `json:"ready"`
	Restarts  int32  `json:"restarts"`
	StartedAt string `json:"started_at"`
	IP        string `json:"ip"`

	CPUMillis   int64 `json:"cpu_millis"`
	MemoryBytes int64 `json:"memory_bytes"`
	UsageKnown  bool  `json:"usage_known"`

	Containers []AppContainer `json:"containers"`
}

// AppContainer is one running container of a pod.
type AppContainer struct {
	Name     string `json:"name"`
	Image    string `json:"image"`
	State    string `json:"state"`
	Restarts int32  `json:"restarts"`

	CPUMillis   int64 `json:"cpu_millis"`
	MemoryBytes int64 `json:"memory_bytes"`

	// UsageKnown is false for every container when the numbers came from
	// metrics-server, whose per-pod answer this product reads as one figure.
	UsageKnown bool `json:"usage_known"`

	CPURequestMillis   int64 `json:"cpu_request_millis"`
	CPULimitMillis     int64 `json:"cpu_limit_millis"`
	MemoryRequestBytes int64 `json:"memory_request_bytes"`
	MemoryLimitBytes   int64 `json:"memory_limit_bytes"`
}

// AppService is a Service whose selector picks one of the app's pods.
type AppService struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	ClusterIP string   `json:"cluster_ip"`
	Ports     []string `json:"ports"`
}

// MaxAppEvents bounds the detail's event list. The thirty-first most recent
// thing that happened to one app is not what anybody opened the page for.
const MaxAppEvents = 30

// Apps lists everything that runs, with what it uses now.
func (c *Client) Apps(ctx context.Context, query AppsQuery, now time.Time) (Apps, error) {
	found, err := c.collectApps(ctx, query.Namespace)
	if err != nil {
		return Apps{}, err
	}

	counted := found.running(query.Node)
	usage := c.readUsage(ctx, nodesOf(counted), query.Namespace)

	out := Apps{
		CollectedAt:    now.UTC().Format(time.RFC3339),
		UsageAvailable: usage.available,
		Notice:         usage.notice,
		Apps:           make([]App, 0, len(found.seeds)+len(counted)),
	}

	keys := map[appKey]bool{}
	for key := range counted {
		keys[key] = true
	}
	// A controller with nothing running is still an app -- a CronJob between
	// runs, a Deployment somebody scaled to zero -- except on a node's page,
	// where the question is what runs HERE and an app with no pod here does
	// not.
	if query.Node == "" {
		for key := range found.seeds {
			keys[key] = true
		}
	}
	for key := range keys {
		out.Apps = append(out.Apps, found.app(key, counted[key], usage))
	}

	// Heaviest first, because that is what the screen is for. The name order
	// under it keeps rows from trading places between refreshes when two apps
	// both use nothing, which is most of them.
	sort.Slice(out.Apps, func(i, j int) bool {
		a, b := out.Apps[i], out.Apps[j]
		if a.CPUMillis != b.CPUMillis {
			return a.CPUMillis > b.CPUMillis
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Kind < b.Kind
	})
	return out, nil
}

// AppDetail reads one app: its pods, their containers, the Services that reach
// it and what the cluster said about it.
//
// kind is the app's kind as the list reports it. The app is computed exactly as
// the list computes it, so the object at the top of the page and the row that
// was clicked cannot disagree.
func (c *Client) AppDetail(ctx context.Context, namespace, kind, name string, now time.Time) (AppDetail, error) {
	canonical, ok := appKinds[strings.ToLower(kind)]
	if !ok {
		return AppDetail{}, fmt.Errorf("%w: %q is not a kind of app. It is one of Deployment, "+
			"StatefulSet, DaemonSet, CronJob, Job or Pod", ErrNoSuchWorkload, kind)
	}
	key := appKey{namespace: namespace, kind: canonical, name: name}

	found, err := c.collectApps(ctx, namespace)
	if err != nil {
		return AppDetail{}, err
	}
	pods := found.running("")[key]
	if _, listed := found.seeds[key]; !listed && len(pods) == 0 {
		return AppDetail{}, fmt.Errorf("%w: %s %s/%s", ErrNoSuchWorkload, canonical, namespace, name)
	}

	usage := c.readUsage(ctx, nodesOf(map[appKey][]*corev1.Pod{key: pods}), namespace)
	out := AppDetail{
		CollectedAt:    now.UTC().Format(time.RFC3339),
		UsageAvailable: usage.available,
		Notice:         usage.notice,
		App:            found.app(key, pods, usage),
		CreatedAt:      found.createdAt(key),
		Pods:           make([]AppPod, 0, len(pods)),
		Services:       make([]AppService, 0, 2),
		Events:         make([]Event, 0, 8),
	}
	for _, pod := range pods {
		out.Pods = append(out.Pods, appPodOf(pod, usage))
	}

	services, err := c.cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return AppDetail{}, fmt.Errorf("kube: listing services: %w", err)
	}
	for _, service := range services.Items {
		// An empty selector selects NOTHING for a Service -- its endpoints are
		// managed by hand -- where the same empty selector in a NetworkPolicy
		// means every pod. labels.SelectorFromSet would read it the second
		// way, so it is refused here before it gets that far.
		if len(service.Spec.Selector) == 0 {
			continue
		}
		selector := labels.SelectorFromSet(service.Spec.Selector)
		for _, pod := range pods {
			if selector.Matches(labels.Set(pod.Labels)) {
				out.Services = append(out.Services, AppService{
					Name:      service.Name,
					Type:      string(service.Spec.Type),
					ClusterIP: service.Spec.ClusterIP,
					Ports:     portLines(service.Spec.Ports),
				})
				break
			}
		}
	}
	sort.Slice(out.Services, func(i, j int) bool { return out.Services[i].Name < out.Services[j].Name })

	// The namespace's events, filtered here: one list rather than one
	// field-selected question per pod, per ReplicaSet and per Job, which for a
	// Deployment with a long rollout history would be dozens of round trips.
	events, err := c.Events(ctx, namespace)
	if err != nil {
		return AppDetail{}, err
	}
	about := found.objectsOf(key)
	// Events answers newest LAST, which is how a log reads; a detail page has
	// room for the latest few, so it walks backwards.
	for i := len(events) - 1; i >= 0 && len(out.Events) < MaxAppEvents; i-- {
		if about[events[i].Object] {
			out.Events = append(out.Events, events[i])
		}
	}
	return out, nil
}

// appKinds maps what a URL may say to the kind as the list reports it. Case is
// forgiven: "deployment" in a path somebody typed is not a different app.
var appKinds = map[string]string{
	"deployment":  string(KindDeployment),
	"statefulset": string(KindStatefulSet),
	"daemonset":   string(KindDaemonSet),
	"cronjob":     string(KindCronJob),
	"job":         string(KindJob),
	"pod":         AppKindPod,
}

type appKey struct{ namespace, kind, name string }

// appSeed is what a listed controller contributes before any pod is counted:
// when it was made, and what it would run.
type appSeed struct {
	created metav1.Time
	images  []string
}

// foundApps is everything collectApps read, resolved to apps.
type foundApps struct {
	// seeds are the controllers that are apps in their own right.
	seeds map[appKey]appSeed

	// pods is every pod with the app it belongs to -- finished ones included,
	// because their events are still the app's.
	pods []resolvedPod

	// children are the intermediate objects an app's events can be about:
	// "ReplicaSet/web-7d9c", "Job/backup-29271".
	children map[appKey][]string
}

type resolvedPod struct {
	key appKey
	pod *corev1.Pod
}

// collectApps reads the controllers, then the pods, and resolves each pod to
// its app.
//
// Controllers FIRST. A pod cannot exist before the ReplicaSet that made it, so a
// pod list taken after the ReplicaSet list finds every pod's ReplicaSet in it --
// the other order would leave a pod created mid-read with an owner this
// function has never heard of, and it would be shown as a stray pod of its own.
func (c *Client) collectApps(ctx context.Context, namespace string) (foundApps, error) {
	out := foundApps{seeds: map[appKey]appSeed{}, children: map[appKey][]string{}}

	sets, err := c.cs.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return foundApps{}, fmt.Errorf("kube: listing replicasets: %w", err)
	}
	deploymentOf := map[string]string{}
	for i := range sets.Items {
		set := &sets.Items[i]
		if ref := controllerIn(set, "apps", "Deployment"); ref != nil {
			deploymentOf[set.Namespace+"/"+set.Name] = ref.Name
			key := appKey{set.Namespace, string(KindDeployment), ref.Name}
			out.children[key] = append(out.children[key], "ReplicaSet/"+set.Name)
		}
	}

	jobs, err := c.cs.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return foundApps{}, fmt.Errorf("kube: listing jobs: %w", err)
	}
	cronJobOf := map[string]string{}
	for i := range jobs.Items {
		job := &jobs.Items[i]
		if ref := controllerIn(job, "batch", "CronJob"); ref != nil {
			cronJobOf[job.Namespace+"/"+job.Name] = ref.Name
			key := appKey{job.Namespace, string(KindCronJob), ref.Name}
			out.children[key] = append(out.children[key], "Job/"+job.Name)
			continue
		}
		// A Job nothing schedules is an app of its own: a migration, a
		// one-off import, a Helm hook.
		out.seeds[appKey{job.Namespace, string(KindJob), job.Name}] = appSeed{
			created: job.CreationTimestamp, images: imagesOf(job.Spec.Template.Spec),
		}
	}

	deployments, err := c.cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return foundApps{}, fmt.Errorf("kube: listing deployments: %w", err)
	}
	for _, d := range deployments.Items {
		out.seeds[appKey{d.Namespace, string(KindDeployment), d.Name}] = appSeed{
			created: d.CreationTimestamp, images: imagesOf(d.Spec.Template.Spec),
		}
	}

	statefulSets, err := c.cs.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return foundApps{}, fmt.Errorf("kube: listing statefulsets: %w", err)
	}
	for _, s := range statefulSets.Items {
		out.seeds[appKey{s.Namespace, string(KindStatefulSet), s.Name}] = appSeed{
			created: s.CreationTimestamp, images: imagesOf(s.Spec.Template.Spec),
		}
	}

	daemonSets, err := c.cs.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return foundApps{}, fmt.Errorf("kube: listing daemonsets: %w", err)
	}
	for _, d := range daemonSets.Items {
		out.seeds[appKey{d.Namespace, string(KindDaemonSet), d.Name}] = appSeed{
			created: d.CreationTimestamp, images: imagesOf(d.Spec.Template.Spec),
		}
	}

	cronJobs, err := c.cs.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return foundApps{}, fmt.Errorf("kube: listing cronjobs: %w", err)
	}
	for _, cj := range cronJobs.Items {
		out.seeds[appKey{cj.Namespace, string(KindCronJob), cj.Name}] = appSeed{
			created: cj.CreationTimestamp, images: imagesOf(cj.Spec.JobTemplate.Spec.Template.Spec),
		}
	}

	pods, err := c.cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return foundApps{}, fmt.Errorf("kube: listing pods: %w", err)
	}
	// By name, so an app's images and pods come out in the same order on every
	// read rather than in whatever order the API server kept them.
	sort.Slice(pods.Items, func(i, j int) bool { return pods.Items[i].Name < pods.Items[j].Name })
	out.pods = make([]resolvedPod, 0, len(pods.Items))
	for i := range pods.Items {
		pod := &pods.Items[i]
		out.pods = append(out.pods, resolvedPod{
			key: appOf(pod, deploymentOf, cronJobOf),
			pod: pod,
		})
	}
	return out, nil
}

// appOf follows a pod's controller up to the app it belongs to.
func appOf(pod *corev1.Pod, deploymentOf, cronJobOf map[string]string) appKey {
	itself := appKey{pod.Namespace, AppKindPod, pod.Name}

	// A mirror pod's controller reference is the Node. That is true and it is
	// not an app anybody installed, so it falls to the default below and the
	// pod stands for itself -- which is how kube-apiserver-cp-1 comes to be
	// listed as what it is rather than as an app called cp-1.
	ref := metav1.GetControllerOf(pod)
	if ref == nil {
		return itself
	}
	switch group := groupOf(ref.APIVersion); {
	case group == "apps" && ref.Kind == "ReplicaSet":
		if deployment, ok := deploymentOf[pod.Namespace+"/"+ref.Name]; ok {
			return appKey{pod.Namespace, string(KindDeployment), deployment}
		}
		// A ReplicaSet no Deployment manages. Rare, and not a kind the screens
		// know, so its pods stand for themselves -- see the file comment.
		return itself
	case group == "apps" && (ref.Kind == string(KindStatefulSet) || ref.Kind == string(KindDaemonSet)):
		return appKey{pod.Namespace, ref.Kind, ref.Name}
	case group == "batch" && ref.Kind == string(KindJob):
		if cronJob, ok := cronJobOf[pod.Namespace+"/"+ref.Name]; ok {
			return appKey{pod.Namespace, string(KindCronJob), cronJob}
		}
		return appKey{pod.Namespace, string(KindJob), ref.Name}
	default:
		return itself
	}
}

// controllerIn returns an object's controller when it is the kind asked for, in
// the group asked for.
func controllerIn(object metav1.Object, group, kind string) *metav1.OwnerReference {
	ref := metav1.GetControllerOf(object)
	if ref == nil || ref.Kind != kind || groupOf(ref.APIVersion) != group {
		return nil
	}
	return ref
}

func groupOf(apiVersion string) string {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return ""
	}
	return gv.Group
}

// running is every pod still doing something, per app -- only those on node
// when one is named.
func (f foundApps) running(node string) map[appKey][]*corev1.Pod {
	out := map[appKey][]*corev1.Pod{}
	for _, resolved := range f.pods {
		if finished(resolved.pod) {
			continue
		}
		if node != "" && resolved.pod.Spec.NodeName != node {
			continue
		}
		out[resolved.key] = append(out[resolved.key], resolved.pod)
	}
	return out
}

// finished is a pod that holds nothing any more. Its numbers are history.
func finished(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}

// nodesOf is every node the given pods are scheduled on, sorted.
func nodesOf(pods map[appKey][]*corev1.Pod) []string {
	seen := map[string]bool{}
	for _, list := range pods {
		for _, pod := range list {
			if pod.Spec.NodeName != "" {
				seen[pod.Spec.NodeName] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for node := range seen {
		out = append(out, node)
	}
	sort.Strings(out)
	return out
}

// createdAt is when the app was made: its controller's timestamp, or for a pod
// on its own -- or a controller the lists missed -- its earliest pod's.
func (f foundApps) createdAt(key appKey) string {
	if seed, ok := f.seeds[key]; ok {
		return stamp(seed.created)
	}
	var earliest metav1.Time
	for _, resolved := range f.pods {
		if resolved.key != key {
			continue
		}
		if earliest.IsZero() || resolved.pod.CreationTimestamp.Before(&earliest) {
			earliest = resolved.pod.CreationTimestamp
		}
	}
	return stamp(earliest)
}

// objectsOf is every "Kind/name" an event about this app can name.
func (f foundApps) objectsOf(key appKey) map[string]bool {
	out := map[string]bool{key.kind + "/" + key.name: true}
	for _, child := range f.children[key] {
		out[child] = true
	}
	for _, resolved := range f.pods {
		if resolved.key == key {
			out["Pod/"+resolved.pod.Name] = true
		}
	}
	return out
}

// app adds up one app over the pods given.
func (f foundApps) app(key appKey, pods []*corev1.Pod, usage usageAnswer) App {
	out := App{
		Namespace: key.namespace, Kind: key.kind, Name: key.name,
		Nodes:  make([]string, 0, 2),
		Images: make([]string, 0, 1),
	}

	nodes := map[string]bool{}
	var cpuNanos, memoryBytes int64
	for _, pod := range pods {
		out.Pods++
		if podReady(pod) {
			out.Ready++
		}
		if pod.Spec.NodeName != "" && !nodes[pod.Spec.NodeName] {
			nodes[pod.Spec.NodeName] = true
			out.Nodes = append(out.Nodes, pod.Spec.NodeName)
		}

		for _, running := range runningContainersOf(pod) {
			if running.status != nil {
				out.Restarts += running.status.RestartCount
			}
			out.CPURequestMillis += milliOf(running.spec.Resources.Requests, corev1.ResourceCPU)
			out.CPULimitMillis += milliOf(running.spec.Resources.Limits, corev1.ResourceCPU)
			out.MemoryRequestBytes += valueOf(running.spec.Resources.Requests, corev1.ResourceMemory)
			out.MemoryLimitBytes += valueOf(running.spec.Resources.Limits, corev1.ResourceMemory)
			out.Images = appendOnce(out.Images, running.spec.Image)
		}

		if measured, ok := usage.podTotal(pod); ok {
			out.UsageKnown = true
			cpuNanos += measured.cpuNanos
			memoryBytes += measured.memoryBytes
		}
	}
	sort.Strings(out.Nodes)

	// Nothing running, so nothing to read the images off: what it WOULD run
	// is the next best answer, and for a CronJob between runs it is the only
	// one.
	if len(pods) == 0 {
		if seed, ok := f.seeds[key]; ok {
			for _, image := range seed.images {
				out.Images = appendOnce(out.Images, image)
			}
		}
	}

	out.CPUMillis = millicores(cpuNanos)
	out.MemoryBytes = memoryBytes
	return out
}

// appPodOf renders one pod of an app, with its containers.
func appPodOf(pod *corev1.Pod, usage usageAnswer) AppPod {
	running := runningContainersOf(pod)
	out := AppPod{
		Name:       pod.Name,
		Node:       pod.Spec.NodeName,
		Phase:      string(pod.Status.Phase),
		IP:         pod.Status.PodIP,
		Containers: make([]AppContainer, 0, len(running)),
	}
	if pod.Status.StartTime != nil {
		out.StartedAt = stamp(*pod.Status.StartTime)
	}
	if total, ok := usage.podTotal(pod); ok {
		out.UsageKnown = true
		out.CPUMillis = millicores(total.cpuNanos)
		out.MemoryBytes = total.memoryBytes
	}

	stats := usage.statsOf(pod)
	ready := 0
	for _, r := range running {
		// The same rendering the container detail uses, so "waiting" and
		// "terminated" mean one thing across the product.
		described := container(r.spec, r.status, r.init)
		row := AppContainer{
			Name:               described.Name,
			Image:              described.Image,
			State:              described.State,
			Restarts:           described.Restarts,
			CPURequestMillis:   milliOf(r.spec.Resources.Requests, corev1.ResourceCPU),
			CPULimitMillis:     milliOf(r.spec.Resources.Limits, corev1.ResourceCPU),
			MemoryRequestBytes: valueOf(r.spec.Resources.Requests, corev1.ResourceMemory),
			MemoryLimitBytes:   valueOf(r.spec.Resources.Limits, corev1.ResourceMemory),
		}
		if described.Ready {
			ready++
		}
		out.Restarts += described.Restarts
		if measured, ok := stats.containers[r.spec.Name]; ok {
			row.UsageKnown = true
			row.CPUMillis = millicores(measured.cpuNanos)
			row.MemoryBytes = measured.memoryBytes
		}
		out.Containers = append(out.Containers, row)
	}
	out.Ready = fmt.Sprintf("%d/%d", ready, len(running))
	return out
}

// runningContainer is a container that runs for the life of the pod.
type runningContainer struct {
	spec   corev1.Container
	status *corev1.ContainerStatus
	init   bool
}

// runningContainersOf is the ordinary containers and the sidecars.
//
// A sidecar is an init container with restartPolicy Always (Kubernetes 1.29 and
// later): declared among the init containers and running beside the others for
// the life of the pod. It uses CPU and memory like any container and the
// kubelet reports it like one, so leaving it out would make a pod's containers
// not add up to the pod. The other init containers ran once and finished, and
// what they asked for is not held while the pod runs.
func runningContainersOf(pod *corev1.Pod) []runningContainer {
	out := make([]runningContainer, 0, len(pod.Spec.Containers)+1)
	for _, spec := range pod.Spec.InitContainers {
		if spec.RestartPolicy != nil && *spec.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			out = append(out, runningContainer{
				spec: spec, status: statusFor(pod.Status.InitContainerStatuses, spec.Name), init: true,
			})
		}
	}
	for _, spec := range pod.Spec.Containers {
		out = append(out, runningContainer{
			spec: spec, status: statusFor(pod.Status.ContainerStatuses, spec.Name),
		})
	}
	return out
}

// podReady is the pod's own Ready condition, and when it has none yet, whether
// every container says ready -- which is what the condition is computed from.
func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	if len(pod.Status.ContainerStatuses) == 0 {
		return false
	}
	for _, status := range pod.Status.ContainerStatuses {
		if !status.Ready {
			return false
		}
	}
	return true
}

// imagesOf is a pod template's images, sidecars included, for an app with no
// pod to read them off.
func imagesOf(spec corev1.PodSpec) []string {
	out := make([]string, 0, len(spec.Containers))
	for _, c := range spec.InitContainers {
		if c.RestartPolicy != nil && *c.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			out = appendOnce(out, c.Image)
		}
	}
	for _, c := range spec.Containers {
		out = appendOnce(out, c.Image)
	}
	return out
}

func appendOnce(list []string, value string) []string {
	if value == "" {
		return list
	}
	for _, have := range list {
		if have == value {
			return list
		}
	}
	return append(list, value)
}

func milliOf(list corev1.ResourceList, name corev1.ResourceName) int64 {
	if q, ok := list[name]; ok {
		return q.MilliValue()
	}
	return 0
}

func valueOf(list corev1.ResourceList, name corev1.ResourceName) int64 {
	if q, ok := list[name]; ok {
		return q.Value()
	}
	return 0
}

// millicores converts what the kubelet reports to what a screen shows.
//
// Rounded UP, which is what resource.Quantity's MilliValue does and therefore
// what `kubectl top` prints: somebody who checks one number against the other
// sees the same figure, and a container using 0.2 of a millicore reads as 1m
// rather than as 0 -- which on this screen would read as idle.
func millicores(nanos int64) int64 {
	if nanos <= 0 {
		return 0
	}
	return (nanos + 999_999) / 1_000_000
}

// Reading usage.

// measured is one CPU and memory figure. cpuNanos is nanocores, the kubelet's
// own unit; memoryBytes is the working set, which is what the kubelet evicts on
// and what `kubectl top` shows -- not RSS, not the page cache.
//
// Signed although the kubelet's own type is unsigned: a figure this screen adds
// up and compares is easier to get right in one integer type, and a node would
// need nine billion cores to overflow it.
type measured struct {
	cpuNanos    int64
	memoryBytes int64
}

// podStats is what one kubelet said about one pod.
type podStats struct {
	// pod is the pod-level figure, when the kubelet gave one.
	pod        *measured
	containers map[string]measured
}

// usageAnswer is everything read about usage for one answer, and how.
type usageAnswer struct {
	// byNode is the kubelets' answers, per node and then per "namespace/pod".
	// Keyed by node so a pod is read from the kubelet that runs it: a pod
	// rescheduled a moment ago can still be in its OLD node's summary, and
	// the stale figure must not win.
	byNode map[string]map[string]podStats

	// fromMetrics is metrics-server's per-pod answer, used only when every
	// kubelet failed. Per pod, not per container: PodUsage sums them.
	fromMetrics map[string]measured

	available bool
	notice    string
}

func podKey(pod *corev1.Pod) string { return pod.Namespace + "/" + pod.Name }

func (u usageAnswer) statsOf(pod *corev1.Pod) podStats {
	return u.byNode[pod.Spec.NodeName][podKey(pod)]
}

// podTotal is what one pod is using, when that is known.
//
// The sum of its containers when every running container reported, because
// that is the figure metrics-server reports and `kubectl top pod` shows -- the
// two sources this screen may be compared against -- and because the pod's row
// then adds up to the container rows under it. The kubelet's pod-level figure
// is the fallback, for a pod whose containers it has not broken down yet.
func (u usageAnswer) podTotal(pod *corev1.Pod) (measured, bool) {
	if u.fromMetrics != nil {
		m, ok := u.fromMetrics[podKey(pod)]
		return m, ok
	}
	stats, ok := u.byNode[pod.Spec.NodeName][podKey(pod)]
	if !ok {
		return measured{}, false
	}

	var sum measured
	complete := true
	for _, running := range runningContainersOf(pod) {
		m, reported := stats.containers[running.spec.Name]
		if !reported {
			complete = false
			break
		}
		sum.cpuNanos += m.cpuNanos
		sum.memoryBytes += m.memoryBytes
	}
	if complete && len(stats.containers) > 0 {
		return sum, true
	}
	if stats.pod != nil {
		return *stats.pod, true
	}
	return measured{}, false
}

// readUsage asks the kubelets on the given nodes, and falls back to
// metrics-server only when none of them answered.
//
// It never fails the answer it is part of. A list of what runs is worth having
// without the numbers, and "the kubelet on w-2 did not answer" is a fact about
// w-2 that belongs in the notice, not a reason to show nothing.
func (c *Client) readUsage(ctx context.Context, nodes []string, namespace string) usageAnswer {
	out := usageAnswer{byNode: map[string]map[string]podStats{}, available: true}
	if len(nodes) == 0 {
		// Nothing is scheduled anywhere, so there is nobody to ask and
		// nothing unmeasured. Not the same as nobody measuring.
		return out
	}

	failed := c.summaries(ctx, nodes, out.byNode)
	if len(failed) == 0 {
		return out
	}

	names := make([]string, 0, len(failed))
	for node := range failed {
		names = append(names, node)
	}
	sort.Strings(names)
	why := names[0] + ": " + shortReason(failed[names[0]])

	if len(failed) < len(nodes) {
		out.notice = fmt.Sprintf("The kubelet on %s did not report usage (%s), so the pods there "+
			"show no usage rather than zero. The numbers from every other node are live.",
			strings.Join(names, ", "), why)
		return out
	}

	// Every kubelet failed. metrics-server, where there is one, measured the
	// same thing on its own schedule.
	pods, err := c.PodUsage(ctx, namespace)
	if err == nil {
		out.fromMetrics = make(map[string]measured, len(pods))
		for _, p := range pods {
			out.fromMetrics[p.Namespace+"/"+p.Name] = measured{
				cpuNanos:    p.CPUMillis * 1_000_000,
				memoryBytes: p.MemoryBytes,
			}
		}
		out.notice = fmt.Sprintf("No kubelet answered for its usage summary (%s), so these numbers "+
			"are metrics-server's instead: the same measurement, averaged over its own window "+
			"rather than read now, and per pod only -- it does not break a pod down by container.",
			why)
		return out
	}

	out.available = false
	if errors.Is(err, ErrNoMetrics) {
		out.notice = fmt.Sprintf("Nothing is measuring usage: no kubelet answered for its usage "+
			"summary (%s), and this cluster has no metrics-server to fall back on. That is not "+
			"the same as usage being zero -- every figure here that says unknown means exactly "+
			"that.", why)
		return out
	}
	out.notice = fmt.Sprintf("Nothing is measuring usage: no kubelet answered for its usage "+
		"summary (%s), and metrics-server did not answer either (%s). That is not the same as "+
		"usage being zero.", why, shortReason(err))
	return out
}

// summaries asks each node's kubelet for its summary, at most
// summaryConcurrency at once and all of them under one deadline. What answered
// lands in into; what did not comes back with its reason.
func (c *Client) summaries(
	ctx context.Context, nodes []string, into map[string]map[string]podStats,
) map[string]error {
	ctx, cancel := context.WithTimeout(ctx, summaryBudget)
	defer cancel()

	type answer struct {
		node string
		pods map[string]podStats
		err  error
	}
	answers := make(chan answer, len(nodes))
	slots := make(chan struct{}, summaryConcurrency)

	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				answers <- answer{node: node, err: ctx.Err()}
				return
			}
			pods, err := c.nodeSummary(ctx, node)
			answers <- answer{node: node, pods: pods, err: err}
		}()
	}
	wg.Wait()
	close(answers)

	failed := map[string]error{}
	for a := range answers {
		if a.err != nil {
			failed[a.node] = a.err
			continue
		}
		into[a.node] = a.pods
	}
	return failed
}

// summaryJSON is the part of the kubelet's summary this reads. The whole
// document carries network, filesystem and volume figures besides; they are
// not decoded, and `only_cpu_and_memory` asks the kubelet not to send them.
type summaryJSON struct {
	Pods []struct {
		PodRef struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"podRef"`
		CPU        *summaryCPU    `json:"cpu"`
		Memory     *summaryMemory `json:"memory"`
		Containers []struct {
			Name   string         `json:"name"`
			CPU    *summaryCPU    `json:"cpu"`
			Memory *summaryMemory `json:"memory"`
		} `json:"containers"`
	} `json:"pods"`
}

// Both fields are pointers because the kubelet leaves them OUT when it has no
// figure yet -- a container that started a second ago has no CPU rate, because
// a rate needs two samples -- and a missing figure read as zero is exactly the
// misreport this screen must not make.
type summaryCPU struct {
	UsageNanoCores *int64 `json:"usageNanoCores"`
}

type summaryMemory struct {
	WorkingSetBytes *int64 `json:"workingSetBytes"`
}

// measure is a figure when both halves of it are there.
func measure(cpu *summaryCPU, memory *summaryMemory) (measured, bool) {
	if cpu == nil || cpu.UsageNanoCores == nil || memory == nil || memory.WorkingSetBytes == nil {
		return measured{}, false
	}
	return measured{cpuNanos: *cpu.UsageNanoCores, memoryBytes: *memory.WorkingSetBytes}, true
}

// nodeSummary reads one kubelet's summary through the API server's node proxy.
//
// The error is taken from Result.Error() and not from DoRaw or Raw, and the
// difference was seen rather than read: those two turn an error answer into
// client-go's generic sentence for the status code -- "the server is currently
// unable to handle the request" -- and drop the Status the API server sent with
// it. Error() decodes that Status, and it is the one line that says WHY: "dial
// tcp 192.168.0.12:10250: connect: no route to host" is a node that is off, and
// it is what the notice carries.
func (c *Client) nodeSummary(ctx context.Context, node string) (map[string]podStats, error) {
	result := c.cs.CoreV1().RESTClient().Get().
		Resource("nodes").Name(node).SubResource("proxy").Suffix("stats", "summary").
		Param("only_cpu_and_memory", "true").
		Do(ctx)
	if err := result.Error(); err != nil {
		return nil, fmt.Errorf("kube: reading the usage summary of %s: %w", node, err)
	}
	raw, err := result.Raw()
	if err != nil {
		return nil, fmt.Errorf("kube: reading the usage summary of %s: %w", node, err)
	}

	var summary summaryJSON
	if err := json.Unmarshal(raw, &summary); err != nil {
		return nil, fmt.Errorf("kube: decoding the usage summary of %s: %w", node, err)
	}

	out := make(map[string]podStats, len(summary.Pods))
	for _, p := range summary.Pods {
		stats := podStats{containers: make(map[string]measured, len(p.Containers))}
		if m, ok := measure(p.CPU, p.Memory); ok {
			stats.pod = &m
		}
		for _, c := range p.Containers {
			if m, ok := measure(c.CPU, c.Memory); ok {
				stats.containers[c.Name] = m
			}
		}
		out[p.PodRef.Namespace+"/"+p.PodRef.Name] = stats
	}
	return out, nil
}

// shortReason is an error as one line a notice can carry.
//
// A deadline is said in words, because "context deadline exceeded" is Go's
// sentence and not the operator's. An answer from the API server is its own
// message without this package's wrapping, which the notice would otherwise
// repeat the node name inside. Anything else is the error as it stands, cut
// short, because a proxied dial error can run to several hundred characters of
// addresses.
func shortReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("no answer within %s", summaryBudget)
	}
	reason := err.Error()
	var status apierrors.APIStatus
	if errors.As(err, &status) && status.Status().Message != "" {
		reason = status.Status().Message
	}
	if runes := []rune(reason); len(runes) > 160 {
		reason = string(runes[:160]) + "…"
	}
	return reason
}
