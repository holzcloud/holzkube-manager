package httpapi_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// The drift routes over the real router, the real inventory, the real Talos
// client against the simulator and a real store on disk.

var driftDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

// kubeletImagePatch is a change in a document that is not the first one: the
// kind of thing a third party does with talosctl and nobody notices.
const kubeletImagePatch = "apiVersion: v1alpha1\nkind: KubeletConfig\nimage: ghcr.io/siderolabs/kubelet:v1.34.2\n"

// newDriftHarness is newConfigHarness with the drift watch added: a logged-in
// admin, one simulated control-plane node, the inventory and the config domain.
func newDriftHarness(t *testing.T) *inventoryHarness {
	t.Helper()

	cl, err := talossim.NewCluster("homelab", "https://192.168.1.41:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	sim, err := talossim.New(talossim.Options{Hostname: "cp-1", Cluster: cl, ControlPlane: true})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	h := newHarness(t,
		withInventory(func(st *fsstore.Store) *inventory.Service {
			return inventory.New(inventory.Deps{
				Store:  st,
				Dialer: talos.NewDirectDialer(sim.Port()),
				Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
			})
		}),
		withDrift(),
	)
	h.setupAndLogin(t)
	return &inventoryHarness{harness: h, sim: sim, cluster: cl}
}

// driftView is what the tests read of the drift route's answer.
type driftView struct {
	Machine         string `json:"machine"`
	State           string `json:"state"`
	Sentence        string `json:"sentence"`
	Digest          string `json:"digest"`
	DriftSince      string `json:"drift_since"`
	BaselineSource  string `json:"baseline_source"`
	BaselineTakenAt string `json:"baseline_taken_at"`
	Notice          string `json:"notice"`
	Diff            *struct {
		Changes []struct {
			Path string `json:"path"`
		} `json:"changes"`
		Paths []string `json:"paths"`
	} `json:"diff"`
}

// getDrift reads the drift route as the harness's admin.
func (h *inventoryHarness) getDrift(t *testing.T, id model.MachineID) (driftView, []byte) {
	t.Helper()

	resp, raw := h.do(t, http.MethodGet, "/api/v1/machines/"+string(id)+"/config/drift", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET config/drift: %d (%s)", resp.StatusCode, raw)
	}
	var v driftView
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode the drift view: %v (%s)", err, raw)
	}
	return v, raw
}

// thirdParty changes the node behind the product's back: through a client that
// carries no observer, which is what talosctl, another operator or another
// tool is.
func (h *inventoryHarness) thirdParty(t *testing.T, id model.MachineID, patch string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cc, err := talos.NewClusterClient(ctx, h.sim.Dialer(), talos.Target{
		Cluster: "c1", Machine: id, Addr: h.sim.Host(),
	}, h.sim.ClientCreds(), talos.Mode{})
	if err != nil {
		t.Fatalf("a third party cannot connect: %v", err)
	}
	defer func() { _ = cc.Close() }()

	active, err := cc.MachineConfigYAML(ctx)
	if err != nil {
		t.Fatalf("a third party cannot read the active configuration: %v", err)
	}
	patched, err := machineconfig.ApplyPatches(active, []string{patch})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	if _, err := cc.ApplyConfigurationWithMode(ctx, patched, "no-reboot"); err != nil {
		t.Fatalf("a third party cannot apply: %v", err)
	}
}

// drifting adopts the node, lays the baseline, lets a third party change it and
// reads it again: the state the confirmation is for.
func (h *inventoryHarness) drifting(t *testing.T) model.MachineID {
	t.Helper()

	id := h.adoptedMachine(t)
	ctx := context.Background()
	h.drift.Tick(ctx)
	h.thirdParty(t, id, kubeletImagePatch)
	h.drift.Tick(ctx)
	if v, raw := h.getDrift(t, id); v.State != "drift" {
		t.Fatalf("precondition: the node is %q after a third party's change, want drift (%s)", v.State, raw)
	}
	return id
}

// TestTracerTheDriftRouteShowsTheStateAndTheDiffOfARealNode goes from the real
// router through the inventory, the Talos client and the store to a node.
func TestTracerTheDriftRouteShowsTheStateAndTheDiffOfARealNode(t *testing.T) {
	h := newDriftHarness(t)
	id := h.adoptedMachine(t)
	ctx := context.Background()

	h.drift.Tick(ctx)
	v, raw := h.getDrift(t, id)
	if v.State != "baseline-created" {
		t.Fatalf("state after the first reading is %q (%s), want baseline-created", v.State, v.Sentence)
	}
	if v.Sentence == "" || v.Notice == "" {
		t.Errorf("the answer lacks its sentence or the standing notice: %s", raw)
	}
	if !driftDigest.MatchString(v.Digest) {
		t.Errorf("the digest is %q, want 64 hex characters", v.Digest)
	}
	if v.Diff != nil {
		t.Errorf("a baseline just laid has a diff: %s", raw)
	}
	if v.BaselineSource == "" {
		t.Errorf("the answer does not say where the baseline came from: %s", raw)
	}

	h.thirdParty(t, id, kubeletImagePatch)
	h.drift.Tick(ctx)
	v, raw = h.getDrift(t, id)
	if v.State != "drift" {
		t.Fatalf("state after a third party's change is %q (%s), want drift", v.State, v.Sentence)
	}
	if v.DriftSince == "" {
		t.Errorf("drift_since is not set while the node drifts: %s", raw)
	}
	if v.Diff == nil {
		t.Fatalf("a drifting node's answer carries no diff: %s", raw)
	}
	found := false
	for _, c := range v.Diff.Changes {
		if c.Path == "KubeletConfig.image" {
			found = true
		}
	}
	if !found {
		t.Errorf("the diff does not name KubeletConfig.image, the document a third party changed: %s", raw)
	}

	// The diff is configuration content, and no secret of the cluster is in it.
	secrets := map[string]string{
		"the Talos CA private key":   string(h.cluster.Secrets.Certs.OS.Key),
		"the Kubernetes CA key":      string(h.cluster.Secrets.Certs.K8s.Key),
		"the etcd CA private key":    string(h.cluster.Secrets.Certs.Etcd.Key),
		"the cluster secret":         h.cluster.Secrets.Cluster.Secret,
		"the bootstrap token":        h.cluster.Secrets.Secrets.BootstrapToken,
		"the machine (trustd) token": h.cluster.Secrets.TrustdInfo.Token,
	}
	for name, secret := range secrets {
		if strings.TrimSpace(secret) == "" {
			t.Fatalf("%s is empty in the fixture, so its absence proves nothing", name)
		}
		if strings.Contains(string(raw), secret) {
			t.Errorf("the drift response leaks %s", name)
		}
	}
	if machineconfig.ContainsPrivateKey(raw) {
		t.Error("the drift response still contains a PEM private key")
	}

	// A list is never null: the interface reads null as an error of a page.
	if strings.Contains(string(raw), `"changes":null`) || strings.Contains(string(raw), `"paths":null`) {
		t.Errorf("the diff serialises a list as null: %s", raw)
	}
	if len(v.Diff.Paths) == 0 {
		t.Errorf("the diff names no path: %s", raw)
	}

	// The cluster list carries the state, as a list that is a list.
	resp, list := h.do(t, http.MethodGet, "/api/v1/clusters", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clusters: %d (%s)", resp.StatusCode, list)
	}
	var clusters struct {
		Clusters []struct {
			ConfigDrift *struct {
				Drifted int               `json:"drifted"`
				Items   []json.RawMessage `json:"items"`
			} `json:"config_drift"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal(list, &clusters); err != nil {
		t.Fatalf("decode the cluster list: %v", err)
	}
	if len(clusters.Clusters) != 1 || clusters.Clusters[0].ConfigDrift == nil {
		t.Fatalf("the cluster list carries no config_drift: %s", list)
	}
	if got := clusters.Clusters[0].ConfigDrift; got.Items == nil || len(got.Items) != 1 || got.Drifted != 1 {
		t.Errorf("config_drift is %+v, want one item and one drifted node: %s", got, list)
	}
	if strings.Contains(string(list), "KubeletConfig") {
		t.Errorf("the cluster list, which a Reader may read, carries configuration content: %s", list)
	}
}

// TestTheDriftRouteIsForOperatorsAndUp: the difference is configuration
// content, so a Reader does not see it; the cluster list, which a Reader does
// see, carries states only.
func TestTheDriftRouteIsForOperatorsAndUp(t *testing.T) {
	h := newDriftHarness(t)
	id := h.adoptedMachine(t)
	h.drift.Tick(context.Background())

	h.openSudo(t)
	for _, role := range []model.UserRole{model.RoleOperator, model.RoleReader} {
		resp, raw := h.do(t, http.MethodPost, "/api/v1/users", map[string]string{
			"username": string(role) + "-account", "password": newAccountPass, "role": string(role),
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("creating the %s account: %d (%s)", role, resp.StatusCode, raw)
		}
	}
	path := "/api/v1/machines/" + string(id) + "/config/drift"

	operator := h.asUser(t, "operator-account", newAccountPass)
	if got, raw := operator.status(t, http.MethodGet, path, nil); got != http.StatusOK {
		t.Errorf("an operator reads the drift: %d, want 200 (%s)", got, raw)
	}
	reader := h.asUser(t, "reader-account", newAccountPass)
	if got, raw := reader.status(t, http.MethodGet, path, nil); got != http.StatusForbidden {
		t.Errorf("a reader reads the drift: %d, want 403: the diff is configuration content (%s)", got, raw)
	}

	// An unknown machine is a 404 with the code every record uses.
	resp, raw := h.do(t, http.MethodGet, "/api/v1/machines/no-such-machine/config/drift", nil)
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(string(raw), "notfound.record") {
		t.Errorf("an unknown machine: %d (%s), want 404 notfound.record", resp.StatusCode, raw)
	}
}

// TestADriftRouteWithoutTheWatchSaysSo: an instance started without the watch
// answers 502 with a sentence, not a panic and not an empty 200.
func TestADriftRouteWithoutTheWatchSaysSo(t *testing.T) {
	h := newConfigHarness(t)
	id := h.adoptedMachine(t)

	resp, raw := h.do(t, http.MethodGet, "/api/v1/machines/"+string(id)+"/config/drift", nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("a harness without the watch: %d (%s), want 502", resp.StatusCode, raw)
	}
	if !strings.Contains(string(raw), "upstream.drift-unavailable") {
		t.Errorf("the 502 does not carry its code: %s", raw)
	}
}
