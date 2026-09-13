package handlers

import (
	"errors"
	"net/http"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// ActionRenewClientCertificate is the audited action name.
const ActionRenewClientCertificate = "cluster.renew-client-certificate"

// RotateRoutes renews the certificate this installation dials a cluster with
// (V2-OPS-02).
//
// The cluster card has warned about this certificate since D-23 shipped, on an
// escalating ladder ending in "every node in this cluster becomes unreachable
// at once", and until now there was nothing to click. A countdown to a door
// that does not exist is worse than no countdown: it teaches an operator that
// the warnings on this screen are not actionable.
func RotateRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/clusters/{id}/client-certificate",

			// Destructive, for the same reason changing the account password
			// is (D-06): it replaces the credential that guards this
			// installation's access to a cluster's PKI. A stolen session that
			// could swap it is a stolen session that could be pointed at a
			// cluster the operator cannot then get back into.
			Destructive: true,

			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          ActionRenewClientCertificate,

			// Deliberately *not* ClusterScope. The cluster lock (INV-12) means
			// "nothing may change this cluster", and this changes nothing on
			// the cluster: the certificate is minted here, from an authority
			// held here, and no node is touched or even written to. Putting it
			// behind the lock would mean an imported cluster kept read-only on
			// purpose becomes permanently unreachable the day its certificate
			// expires -- the lock turning into the thing it was meant to
			// protect against.

			Handler: handler(renewClientCertificate(d)),
		},
	}
}

func renewClientCertificate(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		ctx, cancel := budgetedContext(r, EtcdRouteBudget)
		defer cancel()

		id := model.ClusterID(r.PathValue("id"))

		if _, err := d.Inventory.RenewClientCertificate(ctx, id); err != nil {
			writeRotateError(w, r, d, err)
			return
		}

		// The view, and not the record the renewal returns. Every other route
		// that answers with a cluster answers with this shape, and the record
		// is a different one -- no node counts, no days-left, no certificate
		// urgency. A client parsing one response against the other's schema is
		// a client that either throws or quietly fills in defaults, and the
		// field it would default here is the countdown this route exists to
		// move.
		view, err := d.Inventory.Cluster(r.Context(), id)
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

// writeRotateError maps the two refusals that are conditions rather than bugs.
func writeRotateError(w http.ResponseWriter, r *http.Request, d httpapi.Deps, err error) {
	switch {
	case errors.Is(err, inventory.ErrNoCertificateAuthority):
		// A conflict rather than a validation error: the request was
		// well-formed and the installation is in a state that cannot answer
		// it. The detail carries the repair, which is the only useful part.
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeConflict,
			Title:  "This cluster's stored bundle cannot issue a certificate",
			Status: http.StatusConflict,
			Detail: err.Error(),
			Code:   httpapi.CodeNoCertificateAuthority,
		})

	case errors.Is(err, inventory.ErrCertificateRejected):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeConflict,
			Title:  "No node accepted the new certificate",
			Status: http.StatusConflict,
			Detail: err.Error(),
			Code:   httpapi.CodeCertificateRejected,
		})

	default:
		writeInventoryError(w, r, d, err)
	}
}
