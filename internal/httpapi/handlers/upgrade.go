package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// Upgrades and etcd management.
//
// The route that is not here is the one worth naming: there is no
// `POST /api/v1/clusters/{id}/upgrade?version=latest`. "Latest" is a promise
// this server cannot keep -- the newest release may be two minors away, Talos
// does not support skipping a minor, and an endpoint that quietly did two
// upgrades in a row would do the second against a cluster nobody looked at.
// What replaces it is a plan route that returns the chain.

// UpgradeReadRouteBudget bounds the plan.
//
// A plan connects to every node in the cluster and reads its schematic, plus
// the health gate's three reads against every control-plane node. It is the
// most expensive read in this product, and it is a read: nothing it does
// changes a node.
const UpgradeReadRouteBudget = 120 * time.Second

// EtcdRouteBudget bounds the etcd reads and the member removal. Each is one
// connection and one or two fast calls.
const EtcdRouteBudget = 45 * time.Second

func upgradeConfigured(d httpapi.Deps) *httpapi.Problem {
	if d.Upgrade == nil {
		return httpapi.Upstream("upstream.upgrade-unavailable",
			"This instance was started without the upgrade service.")
	}
	return nil
}

// UpgradeRoutes serves the rolling upgrades and etcd management.
func UpgradeRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/upgrade/releases",
			RequiresSession: true,
			Handler:         handler(upgradeReleases(d)),
		},
		{
			// A read that touches every node. POST because it carries the
			// target version and the cluster; nothing on any node changes.
			Method:          http.MethodPost,
			Pattern:         "/api/v1/clusters/{id}/upgrade/plan",
			RequiresSession: true,
			Action:          "upgrade.plan",
			Handler:         handler(upgradePlan(d, false)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/clusters/{id}/upgrade/kubernetes/plan",
			RequiresSession: true,
			Action:          "upgrade.plan-kubernetes",
			Handler:         handler(upgradePlan(d, true)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/clusters/{id}/upgrade",
			RequiresSession: true,
			Destructive:     true,
			Action:          "upgrade.talos",
			ClusterScope:    clusterFromPathID,
			Handler:         handler(submitUpgrade(d, upgrade.JobKindTalosUpgrade)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/clusters/{id}/upgrade/kubernetes",
			RequiresSession: true,
			Destructive:     true,
			Action:          "upgrade.kubernetes",
			ClusterScope:    clusterFromPathID,
			Handler:         handler(submitUpgrade(d, upgrade.JobKindKubernetesUpgrade)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/clusters/{id}/upgrade/confirm",
			RequiresSession: true,
			Action:          "upgrade.confirm",
			Handler:         handler(confirmUpgrade(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/etcd/members",
			RequiresSession: true,
			Handler:         handler(etcdMembers(d)),
		},
		{
			Method:          http.MethodDelete,
			Pattern:         "/api/v1/clusters/{id}/etcd/members/{member}",
			RequiresSession: true,
			Destructive:     true,
			Action:          "etcd.remove-member",
			ClusterScope:    clusterFromPathID,
			Handler:         handler(removeEtcdMember(d)),
		},
		{
			// The snapshot streams bytes rather than JSON, and it is marked
			// Streaming so the middleware chain does not buffer a multi-
			// gigabyte database in memory on the way out.
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/etcd/snapshot",
			RequiresSession: true,
			Streaming:       true,
			Action:          "etcd.snapshot",
			Handler:         handler(etcdSnapshot(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/machines/{id}/lock",
			RequiresSession: true,
			Action:          "machine.lock",
			Handler:         handler(lockMachine(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/machines/{id}/remove-from-cluster",
			RequiresSession: true,
			Destructive:     true,
			Action:          ActionRemoveFromCluster,
			ClusterScope:    clusterFromBody,
			Handler:         handler(removeNodeFromCluster(d)),
		},
	}
}

func upgradeReleases(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := upgradeConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		ctx, cancel := budgetedContext(r, EtcdRouteBudget)
		defer cancel()

		releases, err := d.Upgrade.Releases(ctx)
		if err != nil {
			writeUpgradeError(w, r, d, err)
			return
		}

		out := make([]string, 0, len(releases))
		for _, v := range releases {
			out = append(out, v.String())
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"releases": out,
			// Said here rather than only in the UI, because it is a statement
			// about what this list is: pre-releases are filtered and there is
			// no "latest".
			"notice": "Pre-releases are not offered: an upgrade chain that routed a cluster " +
				"through a release candidate would route it through software its own project " +
				"does not call finished. There is no 'latest' either -- a target more than one " +
				"minor away is several runs, and the plan names each of them.",
		})
	}
}

func upgradePlan(d httpapi.Deps, kubernetes bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := upgradeConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			To string `json:"to"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}
		if strings.TrimSpace(body.To) == "" {
			httpapi.WriteProblem(w, r, httpapi.Validation("Name the version to upgrade to.",
				httpapi.FieldError{Field: "to", Reason: "is required"}))
			return
		}

		ctx, cancel := budgetedContext(r, UpgradeReadRouteBudget)
		defer cancel()

		cluster := model.ClusterID(r.PathValue("id"))

		var (
			plan upgrade.Plan
			err  error
		)
		if kubernetes {
			plan, err = d.Upgrade.PlanKubernetes(ctx, cluster, body.To)
		} else {
			plan, err = d.Upgrade.PlanTalos(ctx, cluster, body.To)
		}
		if err != nil {
			writeUpgradeError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, plan)
	}
}

// confirmUpgrade issues a token for exactly this run.
//
// What is typed is the **cluster's name**. A rolling upgrade is not about one
// machine -- there is no single hostname to type -- and the thing being put at
// risk is the cluster, so the cluster is what the operator names.
func confirmUpgrade(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := upgradeConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		if p := jobsConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			Kind  string `json:"kind"`
			To    string `json:"to"`
			Typed string `json:"typed"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}

		cluster := model.ClusterID(r.PathValue("id"))
		view, err := d.Inventory.Cluster(r.Context(), cluster)
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		if strings.TrimSpace(body.Typed) != view.Name {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"Type the cluster's name exactly to confirm. A rolling upgrade is not about one "+
					"machine, so there is no hostname to type -- what is at risk is the cluster.",
				httpapi.FieldError{Field: "typed", Reason: "does not match the cluster name"}))
			return
		}

		kind, ok := upgradeKind(body.Kind)
		if !ok {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				fmt.Sprintf("%q is not an upgrade kind; it is %q or %q.",
					body.Kind, upgrade.JobKindTalosUpgrade, upgrade.JobKindKubernetesUpgrade)))
			return
		}

		ctx, cancel := budgetedContext(r, UpgradeReadRouteBudget)
		defer cancel()

		params, problem := upgradeParams(ctx, d, cluster, kind, body.To)
		if problem != nil {
			httpapi.WriteProblem(w, r, problem)
			return
		}

		token, expires := d.Confirmer.Issue(jobs.Intent{
			Action:  string(kind),
			Machine: string(cluster),
			Params:  params,
		})

		writeJSON(w, http.StatusOK, map[string]any{
			"token":   token,
			"expires": expires.Format(time.RFC3339),
			"action":  string(kind),
			"params":  params,
		})
	}
}

func submitUpgrade(d httpapi.Deps, kind model.JobKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := upgradeConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		if p := jobsConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			To           string `json:"to"`
			Confirmation string `json:"confirmation"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}

		cluster := model.ClusterID(r.PathValue("id"))

		ctx, cancel := budgetedContext(r, UpgradeReadRouteBudget)
		defer cancel()

		// The plan is rebuilt here rather than taken from the client. What the
		// job walks has to be what the server just checked -- a client that
		// posted its own node list would be posting a list that was true when
		// the screen rendered.
		params, problem := upgradeParams(ctx, d, cluster, kind, body.To)
		if problem != nil {
			httpapi.WriteProblem(w, r, problem)
			return
		}

		if err := d.Confirmer.Check(body.Confirmation, jobs.Intent{
			Action:  string(kind),
			Machine: string(cluster),
			Params:  params,
		}); err != nil {
			writeJobError(w, r, d, err)
			return
		}

		actor := ""
		if u, ok := d.Auth.CurrentUser(r.Context()); ok {
			actor = u.Username
		}

		j, err := d.Jobs.Submit(r.Context(), model.Job{
			Kind:    kind,
			Cluster: cluster,
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

// upgradeParams builds the job parameters from a freshly computed plan, and
// refuses if that plan is blocked.
//
// Both the confirmation route and the submit route go through it, which is
// what makes the token and the run describe the same thing: a token issued
// against one plan and a run built from another would be a confirmation of
// something that did not happen.
func upgradeParams(
	ctx context.Context,
	d httpapi.Deps,
	cluster model.ClusterID,
	kind model.JobKind,
	to string,
) (map[string]string, *httpapi.Problem) {
	var (
		plan upgrade.Plan
		err  error
	)
	if kind == upgrade.JobKindKubernetesUpgrade {
		plan, err = d.Upgrade.PlanKubernetes(ctx, cluster, to)
	} else {
		plan, err = d.Upgrade.PlanTalos(ctx, cluster, to)
	}
	if err != nil {
		return nil, httpapi.Validation(err.Error())
	}
	if plan.Blocked {
		return nil, &httpapi.Problem{
			Type:   httpapi.TypeConflict,
			Title:  "This upgrade is blocked",
			Status: http.StatusConflict,
			Detail: plan.BlockReason,
			Code:   httpapi.CodeUpgradeBlocked,
		}
	}

	req := upgrade.RequestFor(plan)
	if len(req.Machines) == 0 {
		return nil, httpapi.Validation("Every node in this cluster is locked, so this run would " +
			"walk nothing. Unlock at least one node.")
	}
	return req.Params(), nil
}

func etcdMembers(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := upgradeConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		ctx, cancel := budgetedContext(r, EtcdRouteBudget)
		defer cancel()

		list, err := d.Upgrade.EtcdMembers(ctx, model.ClusterID(r.PathValue("id")))
		if err != nil {
			writeUpgradeError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

func removeEtcdMember(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := upgradeConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		ctx, cancel := budgetedContext(r, EtcdRouteBudget)
		defer cancel()

		err := d.Upgrade.RemoveEtcdMember(ctx,
			model.ClusterID(r.PathValue("id")), r.PathValue("member"))
		if err != nil {
			writeUpgradeError(w, r, d, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func etcdSnapshot(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := upgradeConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		cluster := model.ClusterID(r.PathValue("id"))

		// The headers go out before the first byte is read from the node,
		// which means a failure part-way through arrives as a truncated body
		// rather than as a problem document. That is unavoidable for a stream
		// and it is why the byte count matters: a caller must check the length
		// against Content-Length or check the file, and the notice says so.
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename=%q", "etcd-"+string(cluster)+".snapshot"))
		w.Header().Set("X-Holzkube-Snapshot-Notice", "an incomplete download is a truncated file, not an error page")

		if _, err := d.Upgrade.Snapshot(r.Context(), cluster, w); err != nil {
			// Nothing was written yet only if the failure was the call itself.
			// Writing a problem document after bytes have gone out would
			// append JSON to a database file.
			if rw, ok := w.(interface{ Written() bool }); !ok || !rw.Written() {
				writeUpgradeError(w, r, d, err)
			}
			return
		}
	}
}

func lockMachine(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			Locked *bool  `json:"locked"`
			Reason string `json:"reason"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}
		if body.Locked == nil {
			httpapi.WriteProblem(w, r, httpapi.Validation("Say whether to lock or unlock.",
				httpapi.FieldError{Field: "locked", Reason: "is required"}))
			return
		}
		if *body.Locked && strings.TrimSpace(body.Reason) == "" {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"Say why this node is locked. A lock nobody can explain is a lock the next person "+
					"clears because it is in the way.",
				httpapi.FieldError{Field: "reason", Reason: "is required when locking"}))
			return
		}

		m, err := d.Inventory.SetMachineLock(r.Context(),
			model.MachineID(r.PathValue("id")), *body.Locked, strings.TrimSpace(body.Reason))
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, m)
	}
}

func removeNodeFromCluster(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := upgradeConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			Cluster      string `json:"cluster"`
			Confirmation string `json:"confirmation"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}

		id := model.MachineID(r.PathValue("id"))
		m, err := d.Inventory.Machine(r.Context(), id)
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}

		if err := d.Confirmer.Check(body.Confirmation, jobs.Intent{
			Action:  ActionRemoveFromCluster,
			Machine: string(id),
			Params:  map[string]string{"cluster": body.Cluster},
		}); err != nil {
			writeJobError(w, r, d, err)
			return
		}

		ctx, cancel := budgetedContext(r, EtcdRouteBudget)
		defer cancel()

		if err := d.Upgrade.RemoveNodeFromCluster(ctx, id, m.Role == model.RoleControlPlane); err != nil {
			writeUpgradeError(w, r, d, err)
			return
		}

		// Out of the inventory too. A node that was wiped and left in the list
		// shows as a node that is down, which is indistinguishable from one
		// that will come back.
		if err := d.Inventory.ForgetMachine(r.Context(), id); err != nil {
			writeInventoryError(w, r, d, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"machine": string(id),
			"notice":  upgrade.RemoveNodeNotice,
		})
	}
}

// clusterFromPathID reads the cluster the lock middleware protects off the
// path's {id}.
func clusterFromPathID(r *http.Request) (string, error) {
	return r.PathValue("id"), nil
}

func upgradeKind(s string) (model.JobKind, bool) {
	switch model.JobKind(s) {
	case upgrade.JobKindTalosUpgrade:
		return upgrade.JobKindTalosUpgrade, true
	case upgrade.JobKindKubernetesUpgrade:
		return upgrade.JobKindKubernetesUpgrade, true
	}
	return "", false
}

// writeUpgradeError maps this domain's refusals onto the taxonomy.
func writeUpgradeError(w http.ResponseWriter, r *http.Request, d httpapi.Deps, err error) {
	switch {
	case errors.Is(err, upgrade.ErrWouldStrand):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeConflict,
			Title:  "This upgrade would strand the cluster",
			Status: http.StatusConflict,
			Detail: err.Error(),
			Code:   httpapi.CodeWouldStrand,
		})
	case errors.Is(err, upgrade.ErrNoPath):
		httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
	case errors.Is(err, upgrade.ErrLastVotingMember):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeConflict,
			Title:  "That is the last voting member this cluster can spare",
			Status: http.StatusConflict,
			Detail: err.Error(),
			Code:   httpapi.CodeLastVotingMember,
		})
	case errors.Is(err, upgrade.ErrUnknownSchematic):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeConflict,
			Title:  "This node's Image Factory schematic is not known",
			Status: http.StatusConflict,
			Detail: err.Error(),
			Code:   httpapi.CodeUnknownSchematic,
		})
	case errors.Is(err, upgrade.ErrNotUpgraded):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeConflict,
			Title:  "The node is not running what was installed",
			Status: http.StatusConflict,
			Detail: err.Error(),
			Code:   httpapi.CodeNotUpgraded,
		})
	default:
		if code, ok := upstreamNodeCode(err); ok {
			httpapi.WriteProblem(w, r, httpapi.Upstream(code, err.Error()))
			return
		}

		// A node that answered and refused is not an internal error, and it is
		// not an upstream availability problem either -- KindRejected has no
		// upstream code for exactly that reason. On this domain's routes it
		// almost always means one thing: etcd is not running on the node that
		// was asked. Letting it fall through to internal.unexpected would put
		// a refusal with a perfectly good explanation into an archive that
		// keeps it forever with no detail at all.
		if kind, ok := talos.ErrorKindOf(err); ok && kind == talos.KindRejected {
			httpapi.WriteProblem(w, r, &httpapi.Problem{
				Type:   httpapi.TypeConflict,
				Title:  "The node refused this",
				Status: http.StatusConflict,
				Detail: err.Error(),
				Code:   httpapi.CodeNodeRefused,
			})
			return
		}

		if strings.HasPrefix(err.Error(), "upgrade: ") {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}
		httpapi.WriteInternal(w, r, d.Logger, err)
	}
}
