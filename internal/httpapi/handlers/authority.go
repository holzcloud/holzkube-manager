package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/rotateca"
)

// ActionRotateAuthority is the audited action name, and the action a
// confirmation token is issued for.
const ActionRotateAuthority = string(rotateca.JobKindRotateCA)

// AuthorityRoutes rotate a cluster's Talos certificate authority (V2-OPS-02's
// second half, ledger 103).
//
// Three routes and not one, in the shape the reset already has: a preview the
// screen is built from, a confirmation that has to be typed, and the submission
// that carries the token. The reason is the same one the reset states -- a
// dialog in a browser protects against a misclick and against nothing else --
// and it is sharper here, because this operation changes what every node in
// the cluster trusts.
//
// What this product will NOT do here is offer the operation on a cluster it
// cannot see whole. That refusal lives in the job, between pass 1 and pass 2,
// where it is a statement about now rather than about the moment somebody
// clicked.
func AuthorityRoutes(d httpapi.Deps) []httpapi.Route {
	if d.Jobs == nil || d.Confirmer == nil {
		return nil
	}

	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/authority",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Action:          "cluster.authority-preview",
			Handler:         handler(authorityPreview(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/clusters/{id}/authority/confirm",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "cluster.authority-confirm",
			Handler:         handler(confirmAuthorityRotation(d)),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/authority",

			// Destructive, and behind the cluster lock -- both, and the second
			// is the difference from renewing this installation's own
			// certificate. That one touches no node, so putting it behind the
			// lock would make an imported cluster unreachable the day its
			// certificate expires. This one rewrites the configuration of
			// every node, which is exactly what INV-12's read-only adoption
			// says must not happen until the operator unlocks it.
			Destructive:     true,
			ClusterScope:    clusterIDFromPath,
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          ActionRotateAuthority,
			Handler:         handler(startAuthorityRotation(d)),
		},
	}
}

// clusterIDFromPath reads the cluster the lock middleware protects out of an
// {id} path parameter.
//
// It exists beside provision.go's clusterFromPath because the two routes spell
// the parameter differently -- {id} on the cluster routes, {cluster} on the
// provisioning ones -- and a scope function that reads the wrong name returns
// the empty string, which the middleware reads as "not cluster-scoped" and
// skips. That is not a theory: this route shipped its first version with the
// other helper, and the test asserting a locked cluster refuses the rotation
// got a 202.
func clusterIDFromPath(r *http.Request) (string, error) {
	return r.PathValue("id"), nil
}

// AuthorityPreview is what the rotation screen is built from.
//
// Exported for one reason: cmd/holzkubectl decodes it, and
// cmd/holzkubectl/wire_test.go holds every name that tool decodes against the
// server type that produces it. That guard exists because a client decoding a
// key the API never sends does not fail -- encoding/json leaves the field at
// its zero value and says nothing, which is how the CLI once printed 0 nodes
// and 0/3 steps and both looked like answers.
type AuthorityPreview struct {
	Cluster model.ClusterID `json:"cluster"`
	Name    string          `json:"name"`

	// ConfirmPhrase is what the operator has to type: the cluster's name. The
	// reset asks for a hostname for the reason this asks for a cluster name --
	// it is the thing they can check against what they meant, where a generic
	// word confirms only that somebody can type.
	ConfirmPhrase string `json:"confirm_phrase"`

	// Nodes are every machine the rotation will reach, and it will refuse to
	// continue past pass 1 unless all of them answer.
	Nodes []AuthorityNode `json:"nodes"`

	// InProgress reports that this cluster already holds an authority it is
	// moving to: a rotation was started and did not finish. Submitting again
	// continues that one rather than starting a second, which is what the
	// stored authority is for.
	InProgress bool `json:"in_progress"`

	// Locked reports the read-only adoption, because the route will refuse
	// while it holds and a screen that offered the button anyway would be
	// sending the operator into a 403.
	Locked bool `json:"locked"`

	// Passes are the four passes in the order they run, so the dialog can say
	// what is about to happen rather than "are you sure".
	Passes []string `json:"passes"`

	// Warnings are the things about this operation that are true and not
	// reassuring. They are server-side text rather than words in the browser
	// because they are statements about what this build can and cannot prove.
	Warnings []string `json:"warnings"`
}

// AuthorityNode is a node the rotation will reach.
//
// It carries the record's fields and not the health view's: what this screen
// needs is who will be written to, and the stage belongs to the node list,
// which already shows it with its own staleness. Restating it here would be a
// second copy to disagree with the first.
type AuthorityNode struct {
	ID       model.MachineID   `json:"id"`
	Hostname string            `json:"hostname"`
	Role     model.MachineRole `json:"role"`
}

func authorityPreview(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		id := model.ClusterID(r.PathValue("id"))
		cluster, err := d.Inventory.Cluster(r.Context(), id)
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		machines, err := d.Inventory.MachinesOf(r.Context(), id)
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}

		body := AuthorityPreview{
			Cluster:       id,
			Name:          cluster.Name,
			ConfirmPhrase: cluster.Name,
			Locked:        cluster.Locked,
			Passes: []string{
				"every node accepts the new authority as well as the one it uses now",
				"every node starts issuing from the new authority",
				"holzkube-manager takes a certificate from the new authority and proves it against a node",
				"every node stops accepting the old authority",
			},
			Warnings: []string{
				"Every node in this cluster has to answer. A node that misses the first pass " +
					"refuses the certificate every other node accepts after the second, and " +
					"nothing here can repair it afterwards.",
				"Between the passes the cluster trusts two authorities, which is a working state. " +
					"An interrupted rotation can be continued rather than restarted.",
				"This has never been run against real hardware by anybody. It is measured against " +
					"the simulator, which models what a node accepts and from whom -- not whether " +
					"your cluster survives it.",
				"The Kubernetes certificate authority is not touched. This is the Talos API " +
					"authority only.",
			},
		}
		for _, m := range machines {
			body.Nodes = append(body.Nodes, AuthorityNode{
				ID:       m.ID,
				Hostname: m.Hostname,
				Role:     m.Role,
			})
		}

		if sec, err := d.Store.ClusterSecrets().Get(r.Context(), id); err == nil {
			body.InProgress = len(sec.NextOSCACrt) > 0
		}

		writeJSON(w, http.StatusOK, body)
	}
}

// confirmAuthorityRotation issues the token, once the operator has typed the
// cluster's name.
func confirmAuthorityRotation(d httpapi.Deps) http.HandlerFunc {
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
			Typed string `json:"typed"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}

		id := model.ClusterID(r.PathValue("id"))
		cluster, err := d.Inventory.Cluster(r.Context(), id)
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		if strings.TrimSpace(body.Typed) != cluster.Name {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"Type the cluster's name exactly to confirm.",
				httpapi.FieldError{Field: "typed", Reason: "does not match the cluster name"}))
			return
		}

		// The machine field carries the CLUSTER, because that is what this
		// action is scoped to, and the token has to describe the thing it
		// authorises: a confirmation for one cluster must not start a rotation
		// on another.
		token, expires := d.Confirmer.Issue(jobs.Intent{
			Action:  ActionRotateAuthority,
			Machine: string(id),
		})

		writeJSON(w, http.StatusOK, map[string]any{
			"token":   token,
			"expires": expires.Format(time.RFC3339),
			"action":  ActionRotateAuthority,
			"cluster": id,
		})
	}
}

func startAuthorityRotation(d httpapi.Deps) http.HandlerFunc {
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
			Confirmation string `json:"confirmation"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}

		id := model.ClusterID(r.PathValue("id"))
		if err := d.Confirmer.Check(body.Confirmation, jobs.Intent{
			Action:  ActionRotateAuthority,
			Machine: string(id),
		}); err != nil {
			writeJobError(w, r, d, err)
			return
		}

		machines, err := d.Inventory.MachinesOf(r.Context(), id)
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		if len(machines) == 0 {
			httpapi.WriteProblem(w, r, &httpapi.Problem{
				Type:   httpapi.TypeConflict,
				Title:  "This cluster has no machines in the inventory",
				Status: http.StatusConflict,
				Detail: "A rotation writes the configuration of every node in the cluster, and this " +
					"cluster has none recorded. Refresh it first.",
				Code: httpapi.CodeNoMachinesToRotate,
			})
			return
		}

		req := rotateca.Request{}
		for _, m := range machines {
			req.Machines = append(req.Machines, m.ID)
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
			Kind:    rotateca.JobKindRotateCA,
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
