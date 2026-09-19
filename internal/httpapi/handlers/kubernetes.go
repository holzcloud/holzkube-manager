package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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
	}
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
