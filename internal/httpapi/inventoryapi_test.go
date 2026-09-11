package httpapi_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/audit"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// inventoryHarness is a logged-in harness with an inventory service pointed at
// one simulated control-plane node of a simulated cluster.
type inventoryHarness struct {
	*harness
	sim     *talossim.Server
	cluster *talossim.Cluster
}

func newInventoryHarness(t *testing.T) *inventoryHarness {
	t.Helper()

	cl, err := talossim.NewCluster("homelab", "https://192.168.1.41:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	sim, err := talossim.New(talossim.Options{
		Hostname:     "cp-1",
		Cluster:      cl,
		ControlPlane: true,
	})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	h := newHarness(t, withInventory(func(st *fsstore.Store) *inventory.Service {
		return inventory.New(inventory.Deps{
			Store:  st,
			Dialer: talos.NewDirectDialer(sim.Port()),
			Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		})
	}))

	resp, raw := h.do(t, http.MethodPost, "/api/v1/setup", map[string]string{
		"username": testUser, "password": testPass,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: %d (%s)", resp.StatusCode, raw)
	}
	resp, raw = h.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": testUser, "password": testPass,
	})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login: %d (%s)", resp.StatusCode, raw)
	}

	return &inventoryHarness{harness: h, sim: sim, cluster: cl}
}

// adopt walks the two-step adoption the UI walks: read the fingerprint, then
// import naming it.
func (h *inventoryHarness) adopt(t *testing.T) map[string]any {
	t.Helper()

	resp, raw := h.do(t, http.MethodPost, "/api/v1/clusters/fingerprint",
		map[string]string{"endpoint": h.sim.Host()})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fingerprint: %d (%s)", resp.StatusCode, raw)
	}
	var fp struct {
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.Unmarshal(raw, &fp); err != nil {
		t.Fatalf("decode fingerprint: %v (%s)", err, raw)
	}

	resp, raw = h.do(t, http.MethodPost, "/api/v1/clusters", map[string]string{
		"name":        "homelab",
		"talosconfig": string(h.cluster.Talosconfig),
		"endpoint":    h.sim.Host(),
		"fingerprint": fp.Fingerprint,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("import: %d (%s)", resp.StatusCode, raw)
	}

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode cluster: %v (%s)", err, raw)
	}
	return out
}

// TestAdoptionOverHTTP walks the whole adoption through the API surface.
func TestAdoptionOverHTTP(t *testing.T) {
	c := newInventoryHarness(t)
	cluster := c.adopt(t)

	if cluster["origin"] != "imported" {
		t.Errorf("origin = %v, want imported", cluster["origin"])
	}
	if locked, _ := cluster["locked"].(bool); !locked {
		t.Error("the adopted cluster is not locked; D-21 adopts read-only")
	}

	resp, raw := c.do(t, http.MethodGet, "/api/v1/machines", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("machines: %d (%s)", resp.StatusCode, raw)
	}
	var list struct {
		Machines []map[string]any `json:"machines"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode machines: %v (%s)", err, raw)
	}
	if len(list.Machines) != 1 {
		t.Fatalf("machines = %d, want the one the cluster was adopted through", len(list.Machines))
	}
}

// TestNoClusterResponseCarriesASecret is the structural claim model.Cluster
// and model.ClusterSecrets are separate entities for.
//
// The five fields PITFALLS names by name -- the CA private key, the cluster
// secret, the two joining tokens and the service-account key -- must not
// appear in any API response, in any error message or in the audit log, and
// this checks the first of those three across every inventory response the
// adoption produces.
func TestNoClusterResponseCarriesASecret(t *testing.T) {
	c := newInventoryHarness(t)
	cluster := c.adopt(t)
	id, _ := cluster["id"].(string)

	secrets := map[string]string{
		"the Talos CA private key":   string(c.cluster.Secrets.Certs.OS.Key),
		"the Kubernetes CA key":      string(c.cluster.Secrets.Certs.K8s.Key),
		"the cluster secret":         c.cluster.Secrets.Cluster.Secret,
		"the bootstrap token":        c.cluster.Secrets.Secrets.BootstrapToken,
		"the machine (trustd) token": c.cluster.Secrets.TrustdInfo.Token,
	}

	for _, path := range []string{
		"/api/v1/clusters",
		"/api/v1/clusters/" + id,
		"/api/v1/machines",
	} {
		resp, raw := c.do(t, http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d (%s)", path, resp.StatusCode, raw)
		}
		for name, secret := range secrets {
			if secret == "" {
				continue
			}
			if strings.Contains(string(raw), secret) {
				t.Errorf("%s leaks %s", path, name)
			}
		}
	}

	// The same five, in the archive that is never deleted. The talosconfig was
	// posted in the adoption body and carries the client private key; the
	// allowlist must have redacted it.
	records := c.auditPage(t, "?limit=100")
	body, err := json.Marshal(records.Items)
	if err != nil {
		t.Fatalf("marshal audit records: %v", err)
	}
	if strings.Contains(string(body), "PRIVATE KEY") {
		t.Error("the audit archive contains a private key; the talosconfig was not redacted")
	}
	for name, secret := range secrets {
		if secret != "" && strings.Contains(string(body), secret) {
			t.Errorf("the audit archive contains %s", name)
		}
	}

	// And the adoption is in the archive at all, with its allowlisted fields
	// readable -- a redaction that swallowed the whole record would pass the
	// checks above and be useless.
	if !hasAction(records.Items, "cluster.import") {
		t.Error("the adoption is not in the audit archive")
	}
}

func hasAction(items []audit.Record, action string) bool {
	for _, r := range items {
		if r.Action == action {
			return true
		}
	}
	return false
}

// TestLockedClusterRefusesMutation is D-22's route half: the lock is enforced
// server-side, declaratively, before the sudo prompt.
func TestLockedClusterRefusesMutation(t *testing.T) {
	c := newInventoryHarness(t)
	cluster := c.adopt(t)
	id, _ := cluster["id"].(string)

	resp, raw := c.do(t, http.MethodPost, "/api/v1/machines", map[string]string{
		"cluster": id,
		"addr":    c.sim.Host(),
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("adding a machine to a locked cluster: %d (%s), want 403", resp.StatusCode, raw)
	}
	p := decodeProblem(t, resp, raw)
	if p.Code != httpapi.CodeClusterLocked {
		t.Errorf("code = %q, want %q", p.Code, httpapi.CodeClusterLocked)
	}

	// Unlocking is itself destructive: it is what makes every other
	// destructive route reachable on this cluster, so it goes through the sudo
	// window like one.
	resp, raw = c.do(t, http.MethodPost, "/api/v1/auth/sudo", map[string]string{"password": testPass})
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Fatalf("open the sudo window: %d (%s)", resp.StatusCode, raw)
	}

	resp, raw = c.do(t, http.MethodPost, "/api/v1/clusters/"+id+"/lock",
		map[string]bool{"locked": false})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unlock: %d (%s)", resp.StatusCode, raw)
	}

	resp, raw = c.do(t, http.MethodPost, "/api/v1/machines", map[string]string{
		"cluster": id,
		"addr":    c.sim.Host(),
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("adding a machine to an unlocked cluster: %d (%s), want 201", resp.StatusCode, raw)
	}
}

// TestAdoptingAWorkerIsNamedNotGeneric pins D-05 at the HTTP boundary: the
// refusal carries its own code, so the screen can say what to do instead of
// showing a request id.
func TestAdoptingAWorkerIsNamedNotGeneric(t *testing.T) {
	cl, err := talossim.NewCluster("homelab", "https://192.168.1.41:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	sim, err := talossim.New(talossim.Options{Hostname: "worker-1", Cluster: cl})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	h := newHarness(t, withInventory(func(st *fsstore.Store) *inventory.Service {
		return inventory.New(inventory.Deps{
			Store:  st,
			Dialer: talos.NewDirectDialer(sim.Port()),
			Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		})
	}))

	resp, raw := h.do(t, http.MethodPost, "/api/v1/setup", map[string]string{
		"username": testUser, "password": testPass,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: %d (%s)", resp.StatusCode, raw)
	}
	resp, raw = h.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": testUser, "password": testPass,
	})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login: %d (%s)", resp.StatusCode, raw)
	}

	resp, raw = h.do(t, http.MethodPost, "/api/v1/clusters/fingerprint",
		map[string]string{"endpoint": sim.Host()})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fingerprint: %d (%s)", resp.StatusCode, raw)
	}
	var fp struct {
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.Unmarshal(raw, &fp); err != nil {
		t.Fatalf("decode fingerprint: %v", err)
	}

	resp, raw = h.do(t, http.MethodPost, "/api/v1/clusters", map[string]string{
		"name":        "homelab",
		"talosconfig": string(cl.Talosconfig),
		"endpoint":    sim.Host(),
		"fingerprint": fp.Fingerprint,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("adopting a worker: %d (%s), want 400", resp.StatusCode, raw)
	}
	p := decodeProblem(t, resp, raw)
	if p.Code != httpapi.CodeNotControlPlane {
		t.Fatalf("code = %q, want %q -- otherwise this arrives as internal.unexpected, "+
			"which carries no detail, and stays that way in an archive with no deletion path",
			p.Code, httpapi.CodeNotControlPlane)
	}
}
