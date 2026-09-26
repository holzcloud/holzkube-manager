package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/power"
)

// PowerRouteBudget bounds one power read or submission (2026-09-26).
//
// Reading what can be done to something means asking it: every node of a
// cluster is probed -- in parallel, one probe budget for all of them -- and a
// control-plane node that could be stopped has the etcd gate asked about it,
// which is three reads against every control-plane node. An app is one or two
// calls to the Kubernetes API. The ceiling is the Kubernetes API's own call
// budget with room above it, so that no single call's constant is a fiction
// under it (R2 in cmd/holzkube-managerd/budget_test.go).
const PowerRouteBudget = 90 * time.Second

// powerConfigured refuses cleanly when this instance has no power model.
func powerConfigured(d httpapi.Deps) *httpapi.Problem {
	if d.Power == nil {
		return httpapi.Upstream("upstream.power-unavailable",
			"This instance was started without the power model.")
	}
	return nil
}

// PowerRoutes serve the one power model for clusters, nodes and apps.
//
// Three reads -- "what can I do to this, and why not" -- and seven actions for
// each of the three targets, one route per action. One route per action rather
// than one with the action as a parameter, for two reasons that are both about
// the route table being the place things are decided:
//
//   - Destructive is per route. The two forced actions, and only those, sit
//     behind the sudo window, and a single parameterised route would have to be
//     all of them or none. The flag is read from power.Action.Sudo -- the same
//     table the report's "sudo" field is read from -- so the route that asks for
//     the password and the screen that says it will are the same decision.
//   - The audit archive records the route's Action and not its path. With one
//     route, every stop, start and force-restart would be archived under one
//     word, and a record that cannot say which of seven things happened is not
//     much of a record.
//
// An action that is not one of the seven has no route, and is answered by the
// router's own 404 problem.
func PowerRoutes(d httpapi.Deps) []httpapi.Route {
	routes := []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/power",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Handler:         handler(clusterPowerReport(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/machines/{id}/power",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Handler:         handler(machinePowerReport(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/kubernetes/apps/{namespace}/{kind}/{name}/power",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Handler:         handler(appPowerReport(d)),
		},
	}

	for _, a := range power.Actions() {
		routes = append(routes,
			httpapi.Route{
				Method:          http.MethodPost,
				Pattern:         "/api/v1/clusters/{id}/power/" + string(a),
				RequiresSession: true,
				MinRole:         model.RoleOperator,
				Destructive:     a.Sudo(),
				ClusterScope:    clusterIDFromPath,
				Action:          string(power.ClusterJobKind(a)),
				Handler:         handler(clusterPowerAction(d, a)),
			},
			httpapi.Route{
				Method:          http.MethodPost,
				Pattern:         "/api/v1/machines/{id}/power/" + string(a),
				RequiresSession: true,
				MinRole:         model.RoleOperator,
				Destructive:     a.Sudo(),
				ClusterScope:    machineClusterFromPath(d),
				Action:          string(power.NodeJobKind(a)),
				Handler:         handler(machinePowerAction(d, a)),
			},
			httpapi.Route{
				Method:          http.MethodPost,
				Pattern:         "/api/v1/clusters/{id}/kubernetes/apps/{namespace}/{kind}/{name}/power/" + string(a),
				RequiresSession: true,
				MinRole:         model.RoleOperator,
				Destructive:     a.Sudo(),
				ClusterScope:    clusterIDFromPath,
				Action:          AppPowerAction(a),
				Handler:         handler(appPowerAction(d, a)),
			},
		)
	}
	return routes
}

// AppPowerAction is the audit token an app power route is archived under.
func AppPowerAction(a power.Action) string { return "cluster.kubernetes-app-" + string(a) }

// machineClusterFromPath is the lock link's way to the cluster a machine route
// acts on.
//
// A machine the inventory does not know is "not cluster-scoped" here rather
// than an error, so that the handler answers the 404 for the machine -- the
// lock link's own answer to a missing record is "no such cluster", which would
// name the wrong thing.
func machineClusterFromPath(d httpapi.Deps) func(*http.Request) (string, error) {
	return func(r *http.Request) (string, error) {
		if d.Inventory == nil {
			return "", nil
		}
		cluster, err := d.Inventory.ClusterOfMachine(r.Context(), model.MachineID(r.PathValue("id")))
		if errors.Is(err, inventory.ErrNotFound) {
			return "", nil
		}
		return string(cluster), err
	}
}

func clusterPowerReport(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := powerConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		ctx, cancel := budgetedContext(r, PowerRouteBudget)
		defer cancel()

		rep, err := d.Power.ClusterReport(ctx, model.ClusterID(r.PathValue("id")))
		if err != nil {
			writePowerError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, rep)
	}
}

func machinePowerReport(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := powerConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		ctx, cancel := budgetedContext(r, PowerRouteBudget)
		defer cancel()

		rep, err := d.Power.MachineReport(ctx, model.MachineID(r.PathValue("id")))
		if err != nil {
			writePowerError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, rep)
	}
}

func appPowerReport(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := powerConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		ref, ok := appRefFromPath(w, r)
		if !ok {
			return
		}
		ctx, cancel := budgetedContext(r, PowerRouteBudget)
		defer cancel()

		rep, err := d.Power.AppReport(ctx, model.ClusterID(r.PathValue("id")), ref)
		if err != nil {
			writeAppPowerError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, rep)
	}
}

// clusterPowerAction and machinePowerAction submit a job and answer 202, as
// every node action does (JOB-09): a cluster start is minutes of waking and
// waiting, and progress belongs on the Jobs screen and the job's topic, not
// behind a request that times out.
func clusterPowerAction(d httpapi.Deps, a power.Action) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := powerConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		ctx, cancel := budgetedContext(r, PowerRouteBudget)
		defer cancel()

		j, err := d.Power.SubmitCluster(ctx, model.ClusterID(r.PathValue("id")), a, actorOf(d, r))
		if err != nil {
			writePowerError(w, r, d, err)
			return
		}
		writeAccepted(w, j)
	}
}

func machinePowerAction(d httpapi.Deps, a power.Action) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := powerConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		ctx, cancel := budgetedContext(r, PowerRouteBudget)
		defer cancel()

		j, err := d.Power.SubmitMachine(ctx, model.MachineID(r.PathValue("id")), a, actorOf(d, r))
		if err != nil {
			writePowerError(w, r, d, err)
			return
		}
		writeAccepted(w, j)
	}
}

// appPowerAction runs an app action and answers 200 with what happened. An app
// action is a scale, a patch or a few deletes: it is over when the API server
// has accepted it, and what the controller does next is visible on the app.
func appPowerAction(d httpapi.Deps, a power.Action) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := powerConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		ref, ok := appRefFromPath(w, r)
		if !ok {
			return
		}
		ctx, cancel := budgetedContext(r, PowerRouteBudget)
		defer cancel()

		msg, err := d.Power.RunApp(ctx, model.ClusterID(r.PathValue("id")), ref, a)
		if err != nil {
			writeAppPowerError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": msg})
	}
}

func appRefFromPath(w http.ResponseWriter, r *http.Request) (power.AppRef, bool) {
	kind, ok := power.ParseAppKind(r.PathValue("kind"))
	if !ok {
		httpapi.WriteProblem(w, r, httpapi.NotFound("notfound.kubernetes-workload",
			"That is not a kind of app. The kinds are Deployment, StatefulSet, DaemonSet, Job, "+
				"CronJob and Pod."))
		return power.AppRef{}, false
	}
	return power.AppRef{Namespace: r.PathValue("namespace"), Kind: kind, Name: r.PathValue("name")}, true
}

func actorOf(d httpapi.Deps, r *http.Request) string {
	if u, ok := d.Auth.CurrentUser(r.Context()); ok {
		return u.Username
	}
	return ""
}

func writeAccepted(w http.ResponseWriter, j model.Job) {
	w.Header().Set("Location", "/api/v1/jobs/"+string(j.ID))
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job":   j,
		"topic": string(jobs.Topic(j.ID)),
	})
}

// powerRefusal answers an action the rules refused, with the rules' sentence.
func powerRefusal(err error) (*httpapi.Problem, bool) {
	var refused *power.UnavailableError
	if !errors.As(err, &refused) {
		return nil, false
	}
	return &httpapi.Problem{
		Type:   httpapi.TypeConflict,
		Title:  "That cannot be done right now",
		Status: http.StatusConflict,
		Detail: refused.Reason,
		Code:   httpapi.CodePowerUnavailable,
	}, true
}

func writePowerError(w http.ResponseWriter, r *http.Request, d httpapi.Deps, err error) {
	if p, ok := powerRefusal(err); ok {
		httpapi.WriteProblem(w, r, p)
		return
	}
	if errors.Is(err, power.ErrNotFound) {
		httpapi.WriteProblem(w, r, httpapi.NotFound("notfound.record", "No such record."))
		return
	}
	writeJobError(w, r, d, err)
}

func writeAppPowerError(w http.ResponseWriter, r *http.Request, err error) {
	if p, ok := powerRefusal(err); ok {
		httpapi.WriteProblem(w, r, p)
		return
	}
	if errors.Is(err, power.ErrNotFound) || errors.Is(err, inventory.ErrNotFound) {
		httpapi.WriteProblem(w, r, httpapi.NotFound("notfound.record", "No such record."))
		return
	}
	writeKubernetesError(w, r, err)
}
