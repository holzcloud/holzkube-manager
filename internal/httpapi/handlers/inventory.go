package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// ImportRouteBudget bounds the adoption route.
//
// It is written as a sum of the calls the route actually makes, in the order
// it makes them, rather than as a round number chosen for comfort: a
// fingerprint probe, two connections each proving themselves with a Version
// call, one configuration read, and the membership walk afterwards. The
// alternative -- a route whose worst case is whatever its callees happen to
// do -- is what once flushed a problem document to a socket that had already
// expired.
//
// It is exported so cmd/holzkube-managerd/budget_test.go can read the value that runs,
// for the reason the two Factory budgets state at length: a guard that
// re-declares the number it guards guards nothing.
const ImportRouteBudget = 90 * time.Second

// NodeReadRouteBudget bounds the two routes that read a node's whole resource
// state: adding a machine by address, and refreshing one.
//
// Both make the same walk -- connect, then the fixed series of resource reads
// NodeFacts performs -- so they share one number rather than two that would
// have to be kept equal by hand.
//
// It exists at all because a Talos call on a context with no deadline is
// refused outright (D-04, ErrNoDeadline), and an inbound request context has
// no deadline: the server's write timeout is not one. Without it these routes
// answer 500 internal.unexpected on a call that never left the process.
const NodeReadRouteBudget = 60 * time.Second

// FingerprintRouteBudget bounds POST /api/v1/clusters/fingerprint: one TLS
// handshake against a node nothing trusts yet.
//
// It is the probe class plus room for the route's own work, because the probe
// class is the ceiling the handshake already carries: a route ceiling below it
// would make that constant a number that never applies.
//
// It is a var rather than a const because Deadline() is a function: the number
// is still derived from the class table and not restated next to it, which is
// the property that matters.
var FingerprintRouteBudget = talos.ClassProbe.Deadline() + 5*time.Second

// inventoryConfigured refuses cleanly when this instance has no inventory
// service, rather than panicking on a nil dependency at the first request.
func inventoryConfigured(d httpapi.Deps) *httpapi.Problem {
	if d.Inventory == nil {
		return httpapi.Upstream("upstream.inventory-unavailable",
			"This instance was started without an inventory service.")
	}
	return nil
}

// InventoryRoutes serves the clusters and the machines.
//
// The two resources are flat and separate, and the machines are not nested
// under the clusters: a machine that belongs to no cluster is an ordinary
// state -- it is every machine in maintenance mode -- and nesting would make
// it a special case in the URL as well as in the store (D-10).
//
// Which routes are Destructive is the D-06 marking taken at its word.
// Unlocking a cluster is destructive because it is what makes every other
// destructive route reachable on that cluster. Forgetting a record is
// destructive because D-16 defines no way to get it back. Importing is
// mutating and is not destructive: it creates and destroys nothing.
func InventoryRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters",
			RequiresSession: true,
			Handler:         handler(listClusters(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}",
			RequiresSession: true,
			Handler:         handler(getCluster(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/clusters/fingerprint",
			RequiresSession: true,
			Action:          "cluster.fingerprint",
			Handler:         handler(clusterFingerprint(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/clusters",
			RequiresSession: true,
			Action:          "cluster.import",
			Handler:         handler(importCluster(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/clusters/create",
			RequiresSession: true,
			Action:          "cluster.create",
			Handler:         handler(createCluster(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/talosconfig",
			RequiresSession: true,
			Handler:         handler(clusterTalosconfig(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/clusters/{id}/lock",
			RequiresSession: true,
			Destructive:     true,
			Action:          "cluster.lock",
			Handler:         handler(setClusterLock(d)),
		},
		{
			Method:          http.MethodDelete,
			Pattern:         "/api/v1/clusters/{id}",
			RequiresSession: true,
			Destructive:     true,
			Action:          "cluster.forget",
			Handler:         handler(forgetCluster(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/machines",
			RequiresSession: true,
			Handler:         handler(listMachines(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/machines/{id}",
			RequiresSession: true,
			Handler:         handler(getMachine(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/machines",
			RequiresSession: true,
			Action:          "machine.add",
			// The cluster comes out of the body, because the machine does not
			// exist yet and cannot name it.
			ClusterScope: clusterFromBody,
			Handler:      handler(addMachine(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/machines/{id}/refresh",
			RequiresSession: true,
			Action:          "machine.refresh",
			Handler:         handler(refreshMachine(d)),
		},
		{
			Method:          http.MethodDelete,
			Pattern:         "/api/v1/machines/{id}",
			RequiresSession: true,
			Destructive:     true,
			Action:          "machine.forget",
			Handler:         handler(forgetMachine(d)),
		},
	}
}

// clusterFromBody reads the cluster a request names without consuming the body
// the handler still has to read.
//
// It re-reads rather than parses: the audit middleware already captured the
// body and restored it, and a second reader that consumed it would leave the
// handler with nothing. Keeping this to a string search would be fragile, so
// it decodes into a one-field struct and puts the bytes back.
func clusterFromBody(r *http.Request) (string, error) {
	var body struct {
		Cluster string `json:"cluster"`
	}
	if err := peekJSON(r, &body); err != nil {
		// A body that does not decode is the handler's to refuse, with a
		// validation problem naming the field. Refusing it here would answer
		// "this cluster is locked" to a request that named no cluster.
		return "", nil //nolint:nilerr // see above
	}
	return body.Cluster, nil
}

// budgetedContext applies a route's upstream ceiling to the request context.
//
// It exists because a Talos call on a context with no deadline is refused
// outright, and an inbound request context has no deadline: the server's write
// timeout is not one. Every route in this package that reaches a node goes
// through it.
func budgetedContext(r *http.Request, budget time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), budget)
}

// peekJSON decodes a request body and puts it back.
//
// The lock link runs before the handler and needs one field out of the body;
// the handler then needs the whole of it. A reader that consumed the body
// would leave the handler with nothing, and the failure would look like a
// client that sent no body.
func peekJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return io.EOF
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxTalosconfigBytes))
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(raw))

	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

// maxTalosconfigBytes caps the adoption body.
//
// A talosconfig is a few kilobytes: a CA certificate, a client certificate and
// a key, base64-encoded. Sixteen is room for a file with several contexts and
// is still far below anything that could be used to exhaust memory, which is
// what the smaller general cap in handlers.go exists to prevent for every
// other route.
const maxTalosconfigBytes = 256 << 10

// decodeLargeJSON is decodeJSON with the adoption body's cap.
//
// It is a separate function rather than a parameter on decodeJSON so that the
// larger limit applies to exactly one route and cannot be inherited by
// accident: every other body in this package stays at maxBodyBytes.
func decodeLargeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxTalosconfigBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("body must contain exactly one JSON object")
	}
	return nil
}

func listClusters(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		clusters, err := d.Inventory.Clusters(r.Context())
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}
		// Never null: a null reads to a client as "the server did not check",
		// which is a weaker claim than "there are none".
		writeJSON(w, http.StatusOK, map[string]any{"clusters": clusters})
	}
}

func getCluster(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		c, err := d.Inventory.Cluster(r.Context(), model.ClusterID(r.PathValue("id")))
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	}
}

func clusterFingerprint(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			Endpoint string `json:"endpoint"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}
		if strings.TrimSpace(body.Endpoint) == "" {
			httpapi.WriteProblem(w, r, httpapi.Validation("Name the address of a control-plane node.",
				httpapi.FieldError{Field: "endpoint", Reason: "required"}))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), FingerprintRouteBudget)
		defer cancel()

		fp, err := d.Inventory.Fingerprint(ctx, body.Endpoint)
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		// The fingerprint is shown to the operator to confirm. It is not a
		// secret and it is not a decision: confirming it is.
		writeJSON(w, http.StatusOK, map[string]string{
			"endpoint":    body.Endpoint,
			"fingerprint": fp,
		})
	}
}

func importCluster(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			Name        string `json:"name"`
			Talosconfig string `json:"talosconfig"`
			Endpoint    string `json:"endpoint"`
			Fingerprint string `json:"fingerprint"`
		}
		// The talosconfig arrives in the body, uploaded or pasted, and never
		// as a path on the server: a server-side path would be an arbitrary
		// file read under holzkubed's uid (D-06).
		if err := decodeLargeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}

		var missing []httpapi.FieldError
		if strings.TrimSpace(body.Name) == "" {
			missing = append(missing, httpapi.FieldError{Field: "name", Reason: "required"})
		}
		if strings.TrimSpace(body.Endpoint) == "" {
			missing = append(missing, httpapi.FieldError{Field: "endpoint", Reason: "required"})
		}
		if strings.TrimSpace(body.Talosconfig) == "" {
			missing = append(missing, httpapi.FieldError{Field: "talosconfig", Reason: "required"})
		}
		if strings.TrimSpace(body.Fingerprint) == "" {
			missing = append(missing, httpapi.FieldError{
				Field:  "fingerprint",
				Reason: "confirm the node's certificate fingerprint first",
			})
		}
		if len(missing) > 0 {
			httpapi.WriteProblem(w, r, httpapi.Validation("The adoption is missing required fields.", missing...))
			return
		}

		// One ceiling over every upstream call the adoption makes, applied
		// before the first of them.
		ctx, cancel := context.WithTimeout(r.Context(), ImportRouteBudget)
		defer cancel()

		c, err := d.Inventory.Import(ctx, inventory.ImportRequest{
			Name:        body.Name,
			Talosconfig: []byte(body.Talosconfig),
			Endpoint:    body.Endpoint,
			Fingerprint: body.Fingerprint,
		})
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}

		// The view, not the record: the record and the view happen to carry
		// the same fields today, and going through the view is what keeps that
		// a coincidence rather than a dependency.
		view, err := d.Inventory.Cluster(r.Context(), c.ID)
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}
		writeJSON(w, http.StatusCreated, view)
	}
}

// createCluster is the *other* path onto a cluster record, and the only one
// that mints certificate authorities (INV-02, D-07).
//
// It is a separate route and not a mode of the adoption route on purpose: the
// separation is what a test can check, and what it prevents is an adoption
// that quietly generated its own PKI and produced a cluster that looks right
// and opens nothing.
func createCluster(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			Name     string `json:"name"`
			Endpoint string `json:"endpoint"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}

		var missing []httpapi.FieldError
		if strings.TrimSpace(body.Name) == "" {
			missing = append(missing, httpapi.FieldError{Field: "name", Reason: "required"})
		}
		if strings.TrimSpace(body.Endpoint) == "" {
			missing = append(missing, httpapi.FieldError{
				Field:  "endpoint",
				Reason: "the Kubernetes endpoint this cluster will present, e.g. https://10.0.0.1:6443",
			})
		}
		if len(missing) > 0 {
			httpapi.WriteProblem(w, r, httpapi.Validation("A new cluster needs a name and an endpoint.", missing...))
			return
		}

		// No upstream budget: this route reaches no node. It generates a
		// certificate authority locally and writes two records.
		c, err := d.Inventory.Create(r.Context(), inventory.CreateRequest{
			Name:     body.Name,
			Endpoint: body.Endpoint,
		})
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}

		view, err := d.Inventory.Cluster(r.Context(), c.ID)
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}
		writeJSON(w, http.StatusCreated, view)
	}
}

// clusterTalosconfig hands the operator an admin client configuration.
//
// The certificate in it is minted fresh from the stored bundle and is not the
// one holzkube-manager dials with: two consumers sharing one credential means
// revoking either revokes both, and holzkube-manager's own access is the one that has
// to keep working when everything else has stopped.
//
// It is a read route and not a destructive one, and that is a decision worth
// stating rather than leaving implicit: what it hands out is a *new*
// credential, minted on demand, so there is nothing here to destroy. It is
// still a credential, which is why it requires a session and appears in the
// audit archive like everything else.
func clusterTalosconfig(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		raw, err := d.Inventory.Talosconfig(r.Context(), model.ClusterID(r.PathValue("id")))
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}

		w.Header().Set("Content-Type", "application/yaml")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Disposition", `attachment; filename="talosconfig"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	}
}

func setClusterLock(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			Locked bool `json:"locked"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}

		id := model.ClusterID(r.PathValue("id"))
		if _, err := d.Inventory.SetLock(r.Context(), id, body.Locked); err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		view, err := d.Inventory.Cluster(r.Context(), id)
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func forgetCluster(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		if err := d.Inventory.ForgetCluster(r.Context(), model.ClusterID(r.PathValue("id"))); err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func listMachines(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		machines, err := d.Inventory.Machines(r.Context())
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"machines": machines})
	}
}

func getMachine(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		m, err := d.Inventory.Machine(r.Context(), model.MachineID(r.PathValue("id")))
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, m)
	}
}

func addMachine(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			Cluster string `json:"cluster"`
			Addr    string `json:"addr"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}
		if strings.TrimSpace(body.Addr) == "" {
			httpapi.WriteProblem(w, r, httpapi.Validation("Name the node's address.",
				httpapi.FieldError{Field: "addr", Reason: "required"}))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), NodeReadRouteBudget)
		defer cancel()

		rec, err := d.Inventory.AddManual(ctx, model.ClusterID(body.Cluster), body.Addr)
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		view, err := d.Inventory.Machine(r.Context(), rec.ID)
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}
		d.Inventory.Supervise(rec.ID)
		writeJSON(w, http.StatusCreated, view)
	}
}

func refreshMachine(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		id := model.MachineID(r.PathValue("id"))
		if _, err := d.Inventory.Machine(r.Context(), id); err != nil {
			writeInventoryError(w, r, d, err)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), NodeReadRouteBudget)
		defer cancel()

		// Refresh never fails: an unreachable node is a state the inventory
		// records, not an error the caller has to handle. What comes back is
		// the view afterwards, which is how the screen learns that the node is
		// down rather than that the request was.
		d.Inventory.Refresh(ctx, id)

		view, err := d.Inventory.Machine(r.Context(), id)
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func forgetMachine(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		if err := d.Inventory.ForgetMachine(r.Context(), model.MachineID(r.PathValue("id"))); err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// writeInventoryError maps the inventory's errors onto the taxonomy.
//
// Every branch here is a code minted deliberately in this phase. Without them
// each of these would arrive as internal.unexpected, which by contract carries
// no detail, and would stay that way permanently in an archive with no
// deletion path.
func writeInventoryError(w http.ResponseWriter, r *http.Request, d httpapi.Deps, err error) {
	switch {
	case errors.Is(err, inventory.ErrNotFound), errors.Is(err, store.ErrNotFound):
		httpapi.WriteProblem(w, r, httpapi.NotFound("notfound.record", "No such record."))
	case errors.Is(err, inventory.ErrNotControlPlane):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeValidation,
			Title:  "That node cannot be adopted through",
			Status: http.StatusBadRequest,
			Detail: err.Error(),
			Code:   httpapi.CodeNotControlPlane,
		})
	case errors.Is(err, talos.ErrTalosconfigInvalid):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeValidation,
			Title:  "That is not a usable talosconfig",
			Status: http.StatusBadRequest,
			Detail: err.Error(),
			Code:   httpapi.CodeTalosconfigInvalid,
		})
	case errors.Is(err, talos.ErrFingerprintMismatch):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeValidation,
			Title:  "The node presented a different certificate",
			Status: http.StatusBadRequest,
			Detail: err.Error(),
			Code:   httpapi.CodeFingerprintMismatch,
		})
	case errors.Is(err, inventory.ErrClusterLocked):
		httpapi.WriteProblem(w, r, httpapi.ClusterLocked(err.Error()))
	default:
		if code, ok := upstreamNodeCode(err); ok {
			httpapi.WriteProblem(w, r, httpapi.Upstream(code, err.Error()))
			return
		}
		httpapi.WriteInternal(w, r, d.Logger, err)
	}
}

// upstreamNodeCode maps a transport failure onto the upstream family that
// already exists for it.
func upstreamNodeCode(err error) (string, bool) {
	kind, ok := talos.ErrorKindOf(err)
	if !ok {
		return "", false
	}
	return kind.ProblemCode()
}
