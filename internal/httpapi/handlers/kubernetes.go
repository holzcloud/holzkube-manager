package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// KubernetesRouteBudget bounds one read of a cluster's Kubernetes API.
//
// It exists for the reason NodeReadRouteBudget does: an inbound request context
// carries no deadline, the server's write timeout is not one, and the walk
// behind this route is a Talos call to find the API server's address followed by
// one or two calls to that API server. Without it a cluster whose API server
// accepts connections and never answers would hold a request open until the
// process gave up.
const KubernetesRouteBudget = 45 * time.Second

// KubernetesRoutes read what Kubernetes knows about a cluster (v1.17 slice 2).
//
// Reads only, and that is this slice's whole scope: the client is proven
// against a cluster before it is allowed to change anything in one. The
// milestone's later slices add cordon and drain, pod actions and manifests, and
// each of those is destructive in the sense D-06 means.
//
// The answer deliberately separates "the cluster said" from "the cluster could
// not be asked". An API server that does not answer is not a cluster with no
// pods, and a screen that rendered an empty list would be making exactly the
// claim the inventory spent two phases learning not to make (INV-08, ledger
// 140).
func KubernetesRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/kubernetes",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Action:          "cluster.kubernetes-overview",
			Handler:         handler(kubernetesOverview(d)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/nodes/{node}/cordon",

			// Destructive, and that is a judgement worth stating rather than
			// assuming. A cordon loses no work: it stops the scheduler placing
			// anything NEW on a node, and uncordoning undoes it completely.
			// What it does do is change where a whole cluster's next workload
			// goes, from a session somebody may have left open -- and D-06's
			// window is cheap here because the operator is already at the
			// screen.
			Destructive:     true,
			ClusterScope:    clusterIDFromPath,
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.kubernetes-cordon",
			Handler:         handler(kubernetesCordon(d)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/nodes/{node}/drain",

			Destructive:     true,
			ClusterScope:    clusterIDFromPath,
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          string(kube.JobKindDrain),
			Handler:         handler(kubernetesDrain(d)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/pods/{namespace}/{pod}/restart",

			// Destructive, and the word is doing work here: what Kubernetes
			// offers is "delete the pod", and that is a restart only because a
			// controller makes another. The route refuses when nothing owns
			// the pod (kube.ErrNothingWouldRecreateIt), which is the case where
			// the word would have been a lie.
			Destructive:     true,
			ClusterScope:    clusterIDFromPath,
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.kubernetes-restart-pod",
			Handler:         handler(kubernetesRestartPod(d)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/deployments/{namespace}/{deployment}/scale",

			Destructive:     true,
			ClusterScope:    clusterIDFromPath,
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.kubernetes-scale",
			Handler:         handler(kubernetesScale(d)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/deployments/{namespace}/{deployment}/restart",

			Destructive:     true,
			ClusterScope:    clusterIDFromPath,
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.kubernetes-rollout-restart",
			Handler:         handler(kubernetesRolloutRestart(d)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/manifest/plan",

			// Not destructive, and that is the entire reason it is a separate
			// route: it asks the cluster what exists and writes nothing. A plan
			// that needed a sudo window would push operators to skip the plan
			// and apply blind, which is the opposite of what it is for.
			//
			// Operator rather than reader, though: a plan asks whether named
			// objects exist in named namespaces, and that is more than the
			// overview shows.
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.kubernetes-manifest-plan",
			Handler:         handler(kubernetesManifestPlan(d)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/manifest/apply",

			// Destructive in the sense D-06 means: it changes objects that are
			// already there, and an apply of the wrong document into the wrong
			// cluster is the mistake this product exists to make harder.
			Destructive:     true,
			ClusterScope:    clusterIDFromPath,
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.kubernetes-manifest-apply",
			Handler:         handler(kubernetesManifestApply(d)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/services/{namespace}/{service}/proxy",

			// A POST for a read, deliberately, and REST taste is not what
			// decided it. Two reasons, both about the record:
			//
			// The audit middleware captures the request BODY and not the query
			// string, so a GET with ?port=&path= would archive "somebody used
			// the proxy" and never say what they read. "Somebody read /healthz"
			// and "somebody read /admin/users" are different events, and this
			// is the one route where the parameters ARE the event.
			//
			// And a path in a query string is also in the daemon's access log
			// and in the browser's history, which is two more places a
			// workload's internal URLs end up for no benefit.
			//
			// What it sends is still only a GET to the workload: see
			// kube.ProxyGet for why forwarding any method would hand this
			// product's identity to whoever holds an operator session.
			//
			// Not Destructive: it cannot write to a workload, so a sudo window
			// would be theatre. Operator rather than reader, though -- reaching
			// an unauthenticated admin endpoint inside the cluster is not a read
			// of this product's own data.
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.kubernetes-service-proxy",
			Handler:         handler(kubernetesServiceProxy(d)),
		},
		{
			Method:  http.MethodGet,
			Pattern: "/api/v1/clusters/{id}/kubernetes/pods/{namespace}/{pod}/containers",

			// Reader, unlike the actions: this is the pod list with one more
			// level of detail, and somebody who may see that a pod is broken may
			// see which of its containers is.
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Action:          "cluster.kubernetes-containers",
			Handler:         handler(kubernetesContainers(d)),
		},
		{
			Method:  http.MethodGet,
			Pattern: "/api/v1/clusters/{id}/kubernetes/pods/{namespace}/{pod}/log",

			// A GET with the container and the flags in the query, unlike the
			// service proxy which had to be a POST. The difference is what the
			// EVENT is: there, the path being fetched was the event, and an
			// archive that could not name it recorded nothing worth having.
			// Here the event is "somebody read the log of this pod", and the pod
			// is in the route's own path, which the archive records. Which
			// container of it is detail, not the event.
			//
			// Reader, and that is a real decision rather than a default: a log
			// carries whatever the workload printed, which is regularly more
			// sensitive than anything else on these screens. It is at reader
			// because somebody who cannot read logs cannot diagnose anything,
			// and a product whose diagnosis needs an operator role pushes people
			// to hand out operator roles.
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Action:          "cluster.kubernetes-logs",
			Handler:         handler(kubernetesLogs(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/kubernetes/events",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Action:          "cluster.kubernetes-events",
			Handler:         handler(kubernetesEvents(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/kubernetes/pods/{namespace}/{pod}/events",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Action:          "cluster.kubernetes-pod-events",
			Handler:         handler(kubernetesPodEvents(d)),
		},
		{
			// A POST for a read, for the reason ledger 150 records about the
			// service proxy: the audit middleware captures the BODY and not the
			// query string, and here nothing else identifies the object -- not
			// even a path segment. A GET would have archived "somebody read an
			// object", which is a record that says nothing.
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/object",

			// Operator rather than reader, and the difference is the whole
			// object: the lists show chosen fields, this shows everything on it
			// -- environment variables, annotations, node selectors. A Secret is
			// refused outright (kube.ErrRefusedKind), but a ConfigMap is not,
			// and plenty of people keep things in one that they should not.
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.kubernetes-object",
			Handler:         handler(kubernetesObject(d)),
		},
		{
			Method:  http.MethodGet,
			Pattern: "/api/v1/clusters/{id}/kubernetes/identity",

			// The preview, and it changes nothing. It asks the CLUSTER what the
			// identity may do, which is the only honest answer: RBAC is the
			// cluster's own arrangement of roles and bindings, and anything
			// computed here would be this product's guess about somebody else's
			// configuration.
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.kubernetes-identity",
			Handler:         handler(kubernetesIdentity(d)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/identity",

			// Destructive, and the word is right even though nothing in the
			// cluster changes: setting the wrong string is a cluster where every
			// Kubernetes screen is refused until somebody works out why. The
			// sudo window costs nothing here because the operator is at the
			// screen, and the cluster lock applies for the same reason it
			// applies to a cordon.
			Destructive:     true,
			ClusterScope:    clusterIDFromPath,
			RequiresSession: true,
			MinRole:         model.RoleAdmin,
			Action:          "cluster.kubernetes-set-identity",
			Handler:         handler(kubernetesSetIdentity(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/kubernetes/workloads",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Action:          "cluster.kubernetes-workloads",
			Handler:         handler(kubernetesWorkloads(d)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/workloads/{kind}/{namespace}/{name}/scale",

			Destructive:     true,
			ClusterScope:    clusterIDFromPath,
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.kubernetes-scale-workload",
			Handler:         handler(kubernetesScaleWorkload(d)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/workloads/{kind}/{namespace}/{name}/restart",

			Destructive:     true,
			ClusterScope:    clusterIDFromPath,
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.kubernetes-restart-workload",
			Handler:         handler(kubernetesRestartWorkload(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/kubernetes/resources",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Action:          "cluster.kubernetes-resources",
			Handler:         handler(kubernetesResources(d)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/delete",

			// A POST rather than a DELETE, for the reason the object read is a
			// POST: the archive captures bodies, and WHAT was deleted is the
			// event. A DELETE with the object in the query would record that
			// somebody deleted something.
			Destructive:     true,
			ClusterScope:    clusterIDFromPath,
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.kubernetes-delete-object",
			Handler:         handler(kubernetesDeleteObject(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/kubernetes/nodes/{node}",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Action:          "cluster.kubernetes-node-detail",
			Handler:         handler(kubernetesNodeDetail(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/kubernetes/usage",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Action:          "cluster.kubernetes-usage",
			Handler:         handler(kubernetesUsage(d)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/pods/{namespace}/{pod}/exec",

			// Admin, not operator, and Destructive although it may only read:
			// this runs a program inside somebody's workload, and what it does
			// there is the workload's business rather than this product's. The
			// sudo window and the cluster lock both apply.
			//
			// There is deliberately no port-forward beside it. The service
			// proxy already reaches a workload for reading, and a forward means
			// this daemon holding a listener whose authentication story is
			// nobody's -- a second network path into the cluster, owned by this
			// process. Somebody who needs one has kubectl.
			Destructive:     true,
			ClusterScope:    clusterIDFromPath,
			RequiresSession: true,
			MinRole:         model.RoleAdmin,
			Action:          "cluster.kubernetes-exec",
			Handler:         handler(kubernetesExec(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/kubernetes/capacity",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Action:          "cluster.kubernetes-capacity",
			Handler:         handler(kubernetesCapacity(d)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/workloads/{kind}/{namespace}/{name}/stop",

			Destructive:     true,
			ClusterScope:    clusterIDFromPath,
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.kubernetes-stop",
			Handler:         handler(kubernetesStartStop(d, false)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/workloads/{kind}/{namespace}/{name}/start",

			// Destructive too, and that is not symmetry for its own sake:
			// starting something somebody stopped deliberately is as much a
			// change to what the cluster runs as stopping it was.
			Destructive:     true,
			ClusterScope:    clusterIDFromPath,
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.kubernetes-start",
			Handler:         handler(kubernetesStartStop(d, true)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/kubernetes/sweep",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.kubernetes-sweep-plan",
			Handler:         handler(kubernetesSweepPlan(d)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/kubernetes/sweep",

			Destructive:     true,
			ClusterScope:    clusterIDFromPath,
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.kubernetes-sweep",
			Handler:         handler(kubernetesSweep(d)),
		},
	}
}

// kubernetesCapacity answers how full the cluster and each node is.
func kubernetesCapacity(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		capacity, err := client.Capacity(ctx)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, capacity)
	}
}

// kubernetesStartStop stops or starts a workload.
//
// One handler for both, because the pair is one decision: a stop that could not
// be undone by the button beside it would be a trap, and writing them apart is
// how the two drift.
func kubernetesStartStop(d httpapi.Deps, start bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		kind := kube.WorkloadKind(r.PathValue("kind"))
		namespace, name := r.PathValue("namespace"), r.PathValue("name")

		var err error
		if start {
			err = client.Start(ctx, kind, namespace, name)
		} else {
			err = client.Stop(ctx, kind, namespace, name)
		}
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}

		state, err := client.StoppedStateOf(ctx, kind, namespace, name)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		// Read back rather than echoed: a stop that did not take must not look
		// like one that did.
		writeJSON(w, http.StatusOK, state)
	}
}

// kubernetesSweepPlan says what clearing out would remove, and removes nothing.
func kubernetesSweepPlan(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		plan, err := client.PlanSweep(ctx, r.URL.Query().Get("namespace"), time.Now())
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, plan)
	}
}

// kubernetesSweep removes exactly what a plan listed.
func kubernetesSweep(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			// The plan comes back rather than being recomputed: between a plan
			// and an apply somebody's CronJob can run, and a sweep that
			// recomputed would remove things nobody saw in the list they
			// approved.
			Items []kube.Sweepable `json:"items"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}
		if len(body.Items) == 0 {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"Say what to remove.",
				httpapi.FieldError{Field: "items", Reason: "required"}))
			return
		}

		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		removed, failed, err := client.Sweep(ctx, body.Items)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Removed int                 `json:"removed"`
			Failed  []kube.FailedObject `json:"failed"`
		}{Removed: removed, Failed: failed})
	}
}

// kubernetesExec runs one command in a container.
func kubernetesExec(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Container string `json:"container"`
			// A list rather than a string, and that is the whole design: a
			// string would have to be split by something, and whatever split it
			// would be a shell. See kube.Exec.
			Command []string `json:"command"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}
		if len(body.Command) == 0 {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"Give the program and its arguments.",
				httpapi.FieldError{Field: "command", Reason: "required"}))
			return
		}

		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		ctx, stop := context.WithTimeout(ctx, kube.ExecBudget)
		defer stop()

		result, err := client.Exec(ctx, r.PathValue("namespace"), r.PathValue("pod"),
			body.Container, body.Command)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// kubernetesUsage answers what nodes and pods are using now.
//
// A cluster without metrics-server is the ordinary case -- Talos does not ship
// one -- so that answers 200 with `collecting: false` rather than an error. It
// is not a failure of anything, and a screen that got a 502 would say the
// cluster is unreachable when it is answering perfectly well.
func kubernetesUsage(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		namespace := r.URL.Query().Get("namespace")

		nodes, err := client.NodeUsage(ctx)
		if errors.Is(err, kube.ErrNoMetrics) {
			writeJSON(w, http.StatusOK, struct {
				Collecting bool   `json:"collecting"`
				Notice     string `json:"notice"`
			}{
				Collecting: false,
				Notice: "No metrics-server is installed, so nothing is collecting usage. That is " +
					"not the same as usage being zero, and Talos does not ship one: it is " +
					"something to install if you want these numbers.",
			})
			return
		}
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}

		pods, err := client.PodUsage(ctx, namespace)
		if err != nil && !errors.Is(err, kube.ErrNoMetrics) {
			writeKubernetesError(w, r, err)
			return
		}

		writeJSON(w, http.StatusOK, struct {
			Collecting bool         `json:"collecting"`
			Nodes      []kube.Usage `json:"nodes"`
			Pods       []kube.Usage `json:"pods"`
			Notice     string       `json:"notice"`
		}{
			Collecting: true,
			Nodes:      nodes,
			Pods:       pods,
			Notice: "This is what they are using. What the scheduler reserves is what they " +
				"REQUESTED, which is on the node detail and is a different number.",
		})
	}
}

// kubernetesNodeDetail answers why nothing will schedule on a node.
func kubernetesNodeDetail(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		detail, err := client.NodeDetail(ctx, r.PathValue("node"))
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}

		// The taints worth pointing at, separated from the ordinary ones here
		// rather than in the browser: every Talos control-plane node carries the
		// control-plane taint, and a screen that presented it as a finding would
		// tell somebody their cluster is misconfigured on their first visit.
		writeJSON(w, http.StatusOK, struct {
			kube.NodeDetail
			Explaining []kube.NodeTaint `json:"explaining_taints"`
		}{
			NodeDetail: detail,
			Explaining: kube.TaintsThatExplainPending(detail.Taints),
		})
	}
}

// kubernetesResources lists the objects beside the workloads.
func kubernetesResources(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		resources, err := client.Resources(ctx, r.URL.Query().Get("namespace"))
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Resources []kube.Resource `json:"resources"`
		}{Resources: resources})
	}
}

// kubernetesDeleteObject removes one object.
func kubernetesDeleteObject(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			APIVersion string `json:"api_version"`
			Kind       string `json:"kind"`
			Namespace  string `json:"namespace"`
			Name       string `json:"name"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}
		if body.APIVersion == "" || body.Kind == "" || body.Name == "" {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"Say which object to remove.",
				httpapi.FieldError{Field: "kind", Reason: "api_version, kind and name are required"}))
			return
		}

		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		if err := client.DeleteObject(ctx, body.APIVersion, body.Kind,
			body.Namespace, body.Name); err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// kubernetesWorkloads lists everything that runs, across the kinds.
func kubernetesWorkloads(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		workloads, err := client.Workloads(ctx, r.URL.Query().Get("namespace"))
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Workloads []kube.Workload `json:"workloads"`
		}{Workloads: workloads})
	}
}

// kubernetesScaleWorkload sets the replica count of a kind that has one.
func kubernetesScaleWorkload(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			// A pointer, so that "scale to 0" is distinguishable from "no
			// number was sent" -- switching a workload off must not happen
			// because a client forgot a key.
			Replicas *int32 `json:"replicas"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}
		if body.Replicas == nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"Say how many replicas you want.",
				httpapi.FieldError{Field: "replicas", Reason: "required"}))
			return
		}

		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		err := client.ScaleWorkload(ctx, kube.WorkloadKind(r.PathValue("kind")),
			r.PathValue("namespace"), r.PathValue("name"), *body.Replicas)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// kubernetesRestartWorkload rolls a workload's pods under its own strategy.
func kubernetesRestartWorkload(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		err := client.RolloutRestartWorkload(ctx, kube.WorkloadKind(r.PathValue("kind")),
			r.PathValue("namespace"), r.PathValue("name"), time.Now())
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// kubernetesIdentity answers who this product acts as, and what that identity
// may do.
func kubernetesIdentity(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		// A candidate lets the screen ask "what WOULD this identity be able to
		// do" before anybody stores it. Without that the only way to find out
		// is to save it and watch the product break.
		identity := client.Identity()
		candidate := r.URL.Query().Get("as")
		if candidate != "" {
			asked, err := client.As(kube.Identity{User: candidate})
			if err != nil {
				writeKubernetesError(w, r, err)
				return
			}
			client, identity = asked, asked.Identity()
		}

		permissions, err := client.WhatMayI(ctx,
			kube.EveryPermissionThisProductUses(r.URL.Query().Get("namespace")))
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}

		missing := 0
		for _, p := range permissions {
			if !p.Allowed {
				missing++
			}
		}

		writeJSON(w, http.StatusOK, struct {
			User        string            `json:"user"`
			Groups      []string          `json:"groups"`
			Describes   string            `json:"describes"`
			Permissions []kube.Permission `json:"permissions"`
			Missing     int               `json:"missing"`
			Notice      string            `json:"notice"`
		}{
			User:        identity.User,
			Groups:      identity.Groups,
			Describes:   identity.String(),
			Permissions: permissions,
			Missing:     missing,
			Notice: "This is what the cluster says, asked as that identity. A refusal here is a " +
				"refusal you would meet on the screens, and this product will not retry it as " +
				"its own certificate.",
		})
	}
}

// kubernetesSetIdentity records whose name this cluster's requests arrive under.
func kubernetesSetIdentity(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			// A pointer so that clearing it is distinguishable from not saying.
			// Clearing is how somebody gets back to the product's own
			// certificate after locking themselves out of their own cluster.
			User   *string  `json:"user"`
			Groups []string `json:"groups"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}
		if body.User == nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"Say who to act as, or an empty name to go back to this product's own certificate.",
				httpapi.FieldError{Field: "user", Reason: "required"}))
			return
		}

		ctx, cancel := budgetedContext(r, KubernetesRouteBudget)
		defer cancel()

		cluster, err := d.Inventory.SetActAs(ctx, model.ClusterID(r.PathValue("id")),
			*body.User, body.Groups)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			User   string   `json:"user"`
			Groups []string `json:"groups"`
		}{User: cluster.ActAs, Groups: cluster.ActAsGroups})
	}
}

// kubernetesObject renders one object as YAML.
func kubernetesObject(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			APIVersion string `json:"api_version"`
			Kind       string `json:"kind"`
			Namespace  string `json:"namespace"`
			Name       string `json:"name"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}
		if body.APIVersion == "" || body.Kind == "" || body.Name == "" {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"Say which object.",
				httpapi.FieldError{Field: "kind", Reason: "api_version, kind and name are required"}))
			return
		}

		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		described, err := client.Describe(ctx, body.APIVersion, body.Kind, body.Namespace, body.Name)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, described)
	}
}

// kubernetesContainers answers which containers a pod has and how each is doing.
func kubernetesContainers(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		containers, err := client.ContainersOf(ctx, r.PathValue("namespace"), r.PathValue("pod"))
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}

		// The explanation is computed here rather than in the browser, so the
		// sentence about an exit code is written once. A screen that built its
		// own would disagree with this one eventually, and quietly.
		type row struct {
			kube.Container
			Explanation string `json:"explanation"`
		}
		out := make([]row, 0, len(containers))
		for _, c := range containers {
			out = append(out, row{Container: c, Explanation: kube.ExplainContainer(c)})
		}
		writeJSON(w, http.StatusOK, struct {
			Containers []row `json:"containers"`
		}{Containers: out})
	}
}

// kubernetesLogs reads one container's output.
func kubernetesLogs(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		opts := kube.LogOptions{
			Container: r.URL.Query().Get("container"),
			Previous:  r.URL.Query().Get("previous") == "true",
		}
		if tail := r.URL.Query().Get("tail"); tail != "" {
			n, err := strconv.ParseInt(tail, 10, 64)
			if err != nil || n <= 0 {
				httpapi.WriteProblem(w, r, httpapi.Validation(
					"Ask for a number of lines.",
					httpapi.FieldError{Field: "tail", Reason: "must be a positive number"}))
				return
			}
			opts.TailLines = n
		}

		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		log, err := client.PodLogs(ctx, r.PathValue("namespace"), r.PathValue("pod"), opts)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, log)
	}
}

// kubernetesEvents answers what the cluster has reported recently.
func kubernetesEvents(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		events, err := client.Events(ctx, r.URL.Query().Get("namespace"))
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		writeEvents(w, events)
	}
}

// kubernetesPodEvents answers what the cluster has reported about one pod.
func kubernetesPodEvents(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		events, err := client.EventsAbout(ctx, r.PathValue("namespace"), "Pod", r.PathValue("pod"))
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		writeEvents(w, events)
	}
}

// writeEvents answers with the window as well as the list.
//
// A cluster keeps events for about an hour, so an empty list means "nothing
// happened recently" OR "it happened before the cluster stopped keeping it".
// Those are different answers and the screen has to be able to say which it
// cannot distinguish -- the same distinction INV-08 makes about a node that was
// not asked.
func writeEvents(w http.ResponseWriter, events []kube.Event) {
	writeJSON(w, http.StatusOK, struct {
		Events []kube.Event `json:"events"`
		Notice string       `json:"notice"`
	}{
		Events: events,
		Notice: "A cluster forgets its events after about an hour. An empty list means nothing " +
			"has been reported recently, not that nothing has happened.",
	})
}

// kubernetesServiceProxy fetches one path from a service in the cluster.
//
// The answer is JSON with the body as a STRING, never the workload's own bytes
// under the workload's own content type. That is the security decision this route
// exists to make: handing back `text/html` from a pod would serve that pod's
// markup from this daemon's origin, which is the origin holding the operator's
// session cookie.
func kubernetesServiceProxy(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Port string `json:"port"`
			Path string `json:"path"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}

		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		// A shorter budget than the route's: behind this is an arbitrary
		// workload rather than the API server answering out of etcd.
		ctx, stop := context.WithTimeout(ctx, kube.ProxyBudget)
		defer stop()

		response, err := client.ProxyGet(ctx, r.PathValue("namespace"), r.PathValue("service"),
			body.Port, body.Path)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	}
}

// maxManifestBytes caps a pasted manifest.
//
// Larger than the 64 KiB every other body gets, because a real manifest is
// larger than every other body: one rendered chart with a CustomResourceDefinition
// in it passes 64 KiB easily, and a product that refused those would be a
// product nobody could apply their actual manifests with. Still bounded, because
// what arrives here is parsed into objects in memory.
const maxManifestBytes = 1 << 20

// kubernetesManifestPlan says what applying a manifest would do, and writes
// nothing.
func kubernetesManifestPlan(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		manifest, ok := manifestFrom(w, r)
		if !ok {
			return
		}

		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		plan, err := client.PlanManifest(ctx, manifest)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, plan)
	}
}

// kubernetesManifestApply applies a manifest with server-side apply.
//
// The answer is 200 with the per-object result even when some objects failed,
// and that is deliberate. An apply of ten objects where the sixth conflicts has
// changed five things; a single status code cannot say which five, and a problem
// document would replace the list with a sentence. So the list is the answer,
// FullyApplied says whether anything failed, and the screen shows both.
func kubernetesManifestApply(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		manifest, ok := manifestFrom(w, r)
		if !ok {
			return
		}

		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		result, err := client.ApplyManifest(ctx, manifest)
		if err != nil && len(result.Applied) == 0 && len(result.Failed) == 0 {
			// Nothing was even attempted: the document itself is unusable, and
			// there is no per-object truth to report.
			writeKubernetesError(w, r, err)
			return
		}

		writeJSON(w, http.StatusOK, struct {
			kube.ApplyResult
			FullyApplied bool `json:"fully_applied"`
		}{ApplyResult: result, FullyApplied: len(result.Failed) == 0})
	}
}

// manifestFrom reads the pasted document out of the request.
func manifestFrom(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxManifestBytes)

	var body struct {
		Manifest string `json:"manifest"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		httpapi.WriteProblem(w, r, decodeProblem(err))
		return nil, false
	}
	if strings.TrimSpace(body.Manifest) == "" {
		httpapi.WriteProblem(w, r, httpapi.Validation(
			"Paste the manifest you want applied.",
			httpapi.FieldError{Field: "manifest", Reason: "required"}))
		return nil, false
	}
	return []byte(body.Manifest), true
}

// kubernetesRestartPod deletes one pod so its controller replaces it.
func kubernetesRestartPod(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		err := client.RestartPod(ctx, r.PathValue("namespace"), r.PathValue("pod"))
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// kubernetesScale sets a deployment's replica count.
func kubernetesScale(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			// A pointer, so that "scale to 0" is distinguishable from "no
			// number was sent". Scaling to zero is a real operation -- it is
			// how a workload is switched off -- and a missing field defaulting
			// to it would switch something off because a client forgot a key.
			Replicas *int32 `json:"replicas"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}
		if body.Replicas == nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"Say how many replicas you want.",
				httpapi.FieldError{Field: "replicas", Reason: "required"}))
			return
		}
		if *body.Replicas < 0 {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"A replica count cannot be negative.",
				httpapi.FieldError{Field: "replicas", Reason: "must be zero or more"}))
			return
		}

		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		if err := client.Scale(ctx, r.PathValue("namespace"), r.PathValue("deployment"),
			*body.Replicas); err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// kubernetesRolloutRestart replaces a deployment's pods under the deployment's
// own strategy, which is what keeps the workload up while it happens.
func kubernetesRolloutRestart(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ctx, cancel, ok := kubeClientFor(d, w, r)
		if !ok {
			return
		}
		defer cancel()

		if err := client.RolloutRestart(ctx, r.PathValue("namespace"),
			r.PathValue("deployment"), time.Now()); err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// kubeClientFor is the four lines every Kubernetes action route starts with.
//
// A helper rather than four copies, because the budget and the two refusals
// are the same for all of them -- and because a route that forgot the budget
// would hold a request open against an API server that accepted the connection
// and never answered.
func kubeClientFor(
	d httpapi.Deps, w http.ResponseWriter, r *http.Request,
) (*kube.Client, context.Context, context.CancelFunc, bool) {
	if p := inventoryConfigured(d); p != nil {
		httpapi.WriteProblem(w, r, p)
		return nil, nil, func() {}, false
	}

	ctx, cancel := budgetedContext(r, KubernetesRouteBudget)

	client, err := d.Inventory.KubeClient(ctx, model.ClusterID(r.PathValue("id")))
	if err != nil {
		cancel()
		writeKubernetesError(w, r, err)
		return nil, nil, func() {}, false
	}
	return client, ctx, cancel, true
}

// kubernetesCordon stops or resumes scheduling onto one node.
func kubernetesCordon(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			// Unschedulable is the state being asked for rather than a verb, so
			// that a repeated request is the same request: "cordon it" twice
			// leaves it cordoned, and a client that lost a response can retry
			// without wondering whether it toggled.
			Unschedulable *bool `json:"unschedulable"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}
		if body.Unschedulable == nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"Say which state you want: cordoned or not.",
				httpapi.FieldError{Field: "unschedulable", Reason: "required"}))
			return
		}

		ctx, cancel := budgetedContext(r, KubernetesRouteBudget)
		defer cancel()

		client, err := d.Inventory.KubeClient(ctx, model.ClusterID(r.PathValue("id")))
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}

		node := r.PathValue("node")
		if err := client.Cordon(ctx, node, *body.Unschedulable); err != nil {
			writeKubernetesError(w, r, err)
			return
		}

		// The node as it now is, read back rather than echoed: the screen's
		// next state comes from the cluster, so a cordon that did not take
		// cannot look like one that did.
		nodes, err := client.Nodes(ctx)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		for _, n := range nodes {
			if n.Name == node {
				writeJSON(w, http.StatusOK, n)
				return
			}
		}
		httpapi.WriteProblem(w, r, httpapi.NotFound("notfound.kubernetes-node",
			"The cluster's API server does not know a node by that name."))
	}
}

// kubernetesDrain submits the drain as a job.
//
// A job because a drain waits out each pod's termination grace period and can
// outlast any response this server holds open -- and because an interrupted
// drain leaves a cordoned node with some pods moved, which is a state somebody
// has to finish rather than guess about.
func kubernetesDrain(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := jobsConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			Force           bool `json:"force"`
			DeleteLocalData bool `json:"delete_local_data"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}

		id := model.ClusterID(r.PathValue("id"))
		req := kube.DrainRequest{
			Node:            r.PathValue("node"),
			Force:           body.Force,
			DeleteLocalData: body.DeleteLocalData,
		}
		params, err := req.Params()
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}

		actor := ""
		if u, ok := d.Auth.CurrentUser(r.Context()); ok {
			actor = u.Username
		}

		j, err := d.Jobs.Submit(r.Context(), model.Job{
			Kind:    kube.JobKindDrain,
			Cluster: id,
			Params:  params,
			Actor:   actor,
		})
		if err != nil {
			writeJobError(w, r, d, err)
			return
		}

		w.Header().Set("Location", "/api/v1/jobs/"+string(j.ID))
		writeJSON(w, http.StatusAccepted, map[string]any{
			"job":   j,
			"topic": string(jobs.Topic(j.ID)),
		})
	}
}

// kubernetesOverviewBody is what the Kubernetes screen is built from.
type kubernetesOverviewBody struct {
	Cluster model.ClusterID `json:"cluster"`

	// ServerVersion is the API server's own version, which is the cheapest
	// proof that this whole path works: address, TLS, certificate, and
	// authorisation.
	ServerVersion string `json:"server_version"`

	Nodes       []kube.Node       `json:"nodes"`
	Pods        []kube.Pod        `json:"pods"`
	Deployments []kube.Deployment `json:"deployments"`
	Services    []kube.Service    `json:"services"`
	Namespaces  []string          `json:"namespaces"`

	// Namespace echoes the filter that was applied, empty meaning every
	// namespace, so a screen cannot show one namespace's pods under another's
	// heading.
	Namespace string `json:"namespace"`
}

func kubernetesOverview(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		ctx, cancel := budgetedContext(r, KubernetesRouteBudget)
		defer cancel()

		id := model.ClusterID(r.PathValue("id"))
		namespace := r.URL.Query().Get("namespace")

		client, err := d.Inventory.KubeClient(ctx, id)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}

		version, err := client.ServerVersion(ctx)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		nodes, err := client.Nodes(ctx)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		pods, err := client.Pods(ctx, namespace)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		deployments, err := client.Deployments(ctx, namespace)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		namespaces, err := client.Namespaces(ctx)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}
		// Services are here because the proxy needs a port, and a screen that
		// made somebody look one up would send them to kubectl for the one thing
		// this screen exists to save them.
		services, err := client.Services(ctx, namespace)
		if err != nil {
			writeKubernetesError(w, r, err)
			return
		}

		writeJSON(w, http.StatusOK, kubernetesOverviewBody{
			Cluster:       id,
			ServerVersion: version,
			Nodes:         nodes,
			Pods:          pods,
			Deployments:   deployments,
			Services:      services,
			Namespaces:    namespaces,
			Namespace:     namespace,
		})
	}
}

// writeKubernetesError keeps the two refusals that are conditions rather than
// bugs out of internal.unexpected.
// The Deps are deliberately absent: every branch below answers from the error
// itself, and a route that needed the logger here would be a route hiding a
// finding in a log line rather than in its answer.
func writeKubernetesError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, kube.ErrNoKubernetesAuthority):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeConflict,
			Title:  "This cluster's stored bundle cannot reach its Kubernetes API",
			Status: http.StatusConflict,
			Detail: err.Error(),
			Code:   httpapi.CodeNoKubernetesAuthority,
		})

	case errors.Is(err, kube.ErrNothingWouldRecreateIt):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeConflict,
			Title:  "Nothing would recreate that pod",
			Status: http.StatusConflict,
			Detail: err.Error(),
			Code:   httpapi.CodeNothingWouldRecreateIt,
		})

	case errors.Is(err, kube.ErrCannotStop):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeConflict,
			Title:  "That cannot be stopped",
			Status: http.StatusConflict,
			Detail: err.Error(),
			Code:   httpapi.CodeCannotStop,
		})

	case errors.Is(err, kube.ErrExecRefused), errors.Is(err, kube.ErrNoIdentityForExec):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeConflict,
			Title:  "That command will not be run",
			Status: http.StatusConflict,
			Detail: err.Error(),
			Code:   httpapi.CodeExecRefused,
		})

	case errors.Is(err, kube.ErrRefusedKind):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeConflict,
			Title:  "That kind is not shown here",
			Status: http.StatusConflict,
			Detail: err.Error(),
			Code:   httpapi.CodeRefusedKind,
		})

	case errors.Is(err, kube.ErrNoSuchContainer):
		httpapi.WriteProblem(w, r, httpapi.Validation(err.Error(),
			httpapi.FieldError{Field: "container", Reason: "required"}))

	case errors.Is(err, kube.ErrProxyPathRefused):
		httpapi.WriteProblem(w, r, httpapi.Validation(err.Error(),
			httpapi.FieldError{Field: "path", Reason: "refused"}))

	case errors.Is(err, kube.ErrManifestInvalid):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeValidation,
			Title:  "That manifest cannot be applied",
			Status: http.StatusBadRequest,
			Detail: err.Error(),
			Code:   httpapi.CodeManifestInvalid,
		})

	case errors.Is(err, kube.ErrNoSuchWorkload):
		httpapi.WriteProblem(w, r, httpapi.NotFound("notfound.kubernetes-workload", err.Error()))

	case errors.Is(err, kube.ErrNoEndpoint):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeUpstream,
			Title:  "Nobody could say where this cluster's Kubernetes API is",
			Status: http.StatusBadGateway,
			Detail: err.Error(),
			Code:   httpapi.CodeNoKubernetesEndpoint,
		})

	default:
		// Everything else is the API server refusing, timing out or being
		// unreachable. It arrives as upstream rather than internal, because
		// nothing in this process is broken: a cluster is not answering, and
		// that is a state the screen has to be able to say out loud.
		if code, ok := upstreamNodeCode(err); ok {
			httpapi.WriteProblem(w, r, httpapi.Upstream(code, err.Error()))
			return
		}
		httpapi.WriteProblem(w, r, httpapi.Upstream(httpapi.CodeKubernetesUnreachable, err.Error()))
	}
}
