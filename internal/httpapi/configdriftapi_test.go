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

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/audit"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
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

// ---- confirming ------------------------------------------------------------

// confirmPath is the route that makes the state an operator looked at the new
// baseline.
func confirmPath(id model.MachineID) string {
	return "/api/v1/machines/" + string(id) + "/config/drift/confirm"
}

// baselineRecord is the stored baseline of the node, as the bytes the store
// would write, so that "left as it was" is a byte comparison.
func (h *inventoryHarness) baselineRecord(t *testing.T, id model.MachineID) string {
	t.Helper()

	rec, err := h.store.ConfigBaselines().Get(context.Background(), id)
	if err != nil {
		t.Fatalf("the baseline of %s: %v", id, err)
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal the baseline: %v", err)
	}
	return string(raw)
}

// accountWithSudo creates an account of the role and signs it in with an open
// sudo window of its own, so that what refuses it is its role and not the
// window.
func (h *inventoryHarness) accountWithSudo(t *testing.T, role model.UserRole) *asClient {
	t.Helper()

	h.openSudo(t)
	name := string(role) + "-account"
	resp, raw := h.do(t, http.MethodPost, "/api/v1/users", map[string]string{
		"username": name, "password": newAccountPass, "role": string(role),
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("creating the %s account: %d (%s)", role, resp.StatusCode, raw)
	}
	c := h.asUser(t, name, newAccountPass)
	if got, raw := c.status(t, http.MethodPost, "/api/v1/auth/sudo",
		map[string]string{"password": newAccountPass}); got != http.StatusNoContent {
		t.Fatalf("sudo as the %s: %d (%s)", role, got, raw)
	}
	return c
}

func TestConfirmingNeedsTheSudoWindow(t *testing.T) {
	h := newDriftHarness(t)
	id := h.drifting(t)
	v, _ := h.getDrift(t, id)
	before := h.baselineRecord(t, id)

	resp, raw := h.do(t, http.MethodPost, confirmPath(id), map[string]string{"digest": v.Digest})
	if resp.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("confirming without an open sudo window: %d, want 428 (%s)", resp.StatusCode, raw)
	}
	if after := h.baselineRecord(t, id); after != before {
		t.Errorf("a refused confirmation changed the baseline:\n before %s\n after  %s", before, after)
	}
	if again, _ := h.getDrift(t, id); again.State != "drift" {
		t.Errorf("the node is %q after a confirmation the gate refused, want drift", again.State)
	}
}

func TestConfirmingIsForAdminsOnly(t *testing.T) {
	h := newDriftHarness(t)
	id := h.drifting(t)
	v, _ := h.getDrift(t, id)
	before := h.baselineRecord(t, id)

	for _, role := range []model.UserRole{model.RoleOperator, model.RoleReader} {
		c := h.accountWithSudo(t, role)
		if got, raw := c.status(t, http.MethodPost, confirmPath(id), map[string]string{"digest": v.Digest}); got != http.StatusForbidden {
			t.Errorf("a %s confirmed the drift: %d, want 403 (%s)", role, got, raw)
		}
	}
	if after := h.baselineRecord(t, id); after != before {
		t.Errorf("a refused confirmation changed the baseline:\n before %s\n after  %s", before, after)
	}
}

func TestConfirmingTurnsTheDriftIntoTheBaseline(t *testing.T) {
	h := newDriftHarness(t)
	id := h.drifting(t)
	seen, _ := h.getDrift(t, id)
	h.openSudo(t)

	resp, raw := h.do(t, http.MethodPost, confirmPath(id), map[string]string{"digest": seen.Digest})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirming: %d (%s)", resp.StatusCode, raw)
	}
	var v driftView
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode the answer: %v (%s)", err, raw)
	}
	if v.State != "ok" || v.Diff != nil || v.DriftSince != "" {
		t.Errorf("the answer of a confirmation is %s, want the new view: ok, no diff, no start", raw)
	}
	if v.BaselineSource != "confirmed" {
		t.Errorf("the baseline source is %q, want confirmed", v.BaselineSource)
	}

	if again, raw := h.getDrift(t, id); again.State != "ok" || again.Diff != nil {
		t.Errorf("GET after confirming: %s, want ok and no diff", raw)
	}
	resp, list := h.do(t, http.MethodGet, "/api/v1/clusters", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clusters: %d (%s)", resp.StatusCode, list)
	}
	var clusters struct {
		Clusters []struct {
			ConfigDrift *struct {
				Drifted int `json:"drifted"`
			} `json:"config_drift"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal(list, &clusters); err != nil {
		t.Fatalf("decode the cluster list: %v", err)
	}
	if len(clusters.Clusters) != 1 || clusters.Clusters[0].ConfigDrift == nil || clusters.Clusters[0].ConfigDrift.Drifted != 0 {
		t.Errorf("the cluster card still counts the drift after it was confirmed: %s", list)
	}

	// A reading that follows is compared with what was confirmed.
	h.drift.Tick(context.Background())
	if after, raw := h.getDrift(t, id); after.State != "ok" {
		t.Errorf("the next reading of an unchanged node is %q, want ok: %s", after.State, raw)
	}
}

func TestConfirmingRefusesABadDigest(t *testing.T) {
	h := newDriftHarness(t)
	id := h.drifting(t)
	h.openSudo(t)
	before := h.baselineRecord(t, id)

	for name, body := range map[string]any{
		"not hex":          map[string]string{"digest": strings.Repeat("z", 64)},
		"too short":        map[string]string{"digest": strings.Repeat("a", 63)},
		"too long":         map[string]string{"digest": strings.Repeat("a", 65)},
		"upper case":       map[string]string{"digest": strings.Repeat("A", 64)},
		"empty":            map[string]string{"digest": ""},
		"no digest at all": map[string]string{},
		"no body":          nil,
	} {
		// An empty body still says what it is: the CSRF rule wants a content
		// type on every mutating request.
		resp, raw := h.do(t, http.MethodPost, confirmPath(id), body, func(r *http.Request) {
			r.Header.Set("Content-Type", "application/json")
		})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400 (%s)", name, resp.StatusCode, raw)
			continue
		}
		if name != "no body" && !strings.Contains(string(raw), `"digest"`) {
			t.Errorf("%s: the 400 does not name the field digest: %s", name, raw)
		}
	}
	if after := h.baselineRecord(t, id); after != before {
		t.Errorf("a refused confirmation changed the baseline:\n before %s\n after  %s", before, after)
	}
}

func TestConfirmingRefusesAChangedReading(t *testing.T) {
	h := newDriftHarness(t)
	id := h.drifting(t)
	seen, _ := h.getDrift(t, id)

	// Between the screen and the click somebody changes it again.
	h.thirdParty(t, id, "apiVersion: v1alpha1\nkind: KubeletConfig\nimage: ghcr.io/siderolabs/kubelet:v1.34.3\n")
	h.drift.Tick(context.Background())
	if now, _ := h.getDrift(t, id); now.Digest == seen.Digest {
		t.Fatal("precondition: the second change did not change the digest")
	}
	before := h.baselineRecord(t, id)

	h.openSudo(t)
	resp, raw := h.do(t, http.MethodPost, confirmPath(id), map[string]string{"digest": seen.Digest})
	if resp.StatusCode != http.StatusConflict || problemCode(raw) != "conflict.drift-changed" {
		t.Fatalf("confirming what is not the last reading: %d (%s), want 409 conflict.drift-changed", resp.StatusCode, raw)
	}
	if after := h.baselineRecord(t, id); after != before {
		t.Errorf("a refused confirmation changed the baseline:\n before %s\n after  %s", before, after)
	}
	if now, _ := h.getDrift(t, id); now.State != "drift" {
		t.Errorf("the node is %q after a refused confirmation, want drift", now.State)
	}
}

func TestConfirmingRefusesWhatHasNoDrift(t *testing.T) {
	h := newDriftHarness(t)
	id := h.adoptedMachine(t)
	h.drift.Tick(context.Background())
	h.drift.Tick(context.Background())
	v, raw := h.getDrift(t, id)
	if v.State != "ok" {
		t.Fatalf("precondition: the node is %q (%s), want ok", v.State, raw)
	}
	before := h.baselineRecord(t, id)

	h.openSudo(t)
	resp, raw := h.do(t, http.MethodPost, confirmPath(id), map[string]string{"digest": v.Digest})
	if resp.StatusCode != http.StatusConflict || problemCode(raw) != "conflict.no-drift" {
		t.Fatalf("confirming a node without drift: %d (%s), want 409 conflict.no-drift", resp.StatusCode, raw)
	}
	if after := h.baselineRecord(t, id); after != before {
		t.Errorf("a refused confirmation changed the baseline:\n before %s\n after  %s", before, after)
	}
}

func TestConfirmingRefusesWhatWasNeverRead(t *testing.T) {
	h := newDriftHarness(t)
	id := h.adoptedMachine(t)
	h.openSudo(t)

	resp, raw := h.do(t, http.MethodPost, confirmPath(id), map[string]string{"digest": strings.Repeat("a", 64)})
	if resp.StatusCode != http.StatusConflict || problemCode(raw) != "conflict.drift-not-read" {
		t.Fatalf("confirming a node nobody has read: %d (%s), want 409 conflict.drift-not-read", resp.StatusCode, raw)
	}
	if _, err := h.store.ConfigBaselines().Get(context.Background(), id); err == nil {
		t.Error("a refused confirmation laid a baseline")
	}
}

// driftConfirmRecords are the audit records of the confirm action.
func (h *inventoryHarness) driftConfirmRecords(t *testing.T) (attempts []audit.Record, successes int) {
	t.Helper()

	page, err := h.logger.Query(context.Background(), audit.Filter{Action: "config.drift-confirm"})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	for _, rec := range page.Items {
		switch rec.Outcome {
		case audit.OutcomeAttempt:
			attempts = append(attempts, rec)
		case audit.OutcomeSuccess:
			successes++
		}
	}
	return attempts, successes
}

func TestTheConfirmationNamesTheNodeAndTheDigestInTheAuditRecord(t *testing.T) {
	h := newDriftHarness(t)
	id := h.drifting(t)
	seen, _ := h.getDrift(t, id)
	h.openSudo(t)

	if resp, raw := h.do(t, http.MethodPost, confirmPath(id), map[string]string{"digest": seen.Digest}); resp.StatusCode != http.StatusOK {
		t.Fatalf("confirming: %d (%s)", resp.StatusCode, raw)
	}

	attempts, successes := h.driftConfirmRecords(t)
	if len(attempts) != 1 || successes != 1 {
		t.Fatalf("the archive holds %d attempts and %d successes of config.drift-confirm, want one of each", len(attempts), successes)
	}
	rec := attempts[0]
	if rec.Actor != testUser {
		t.Errorf("the record actor is %q, want %q", rec.Actor, testUser)
	}
	if got := rec.Params["machine"]; got != string(id) {
		t.Errorf("the record names the node %v, want %q: an admin who confirmed a drift without a node is no record", got, id)
	}
	if got := rec.Params["digest"]; got != seen.Digest {
		t.Errorf("the record names the digest %v, want %q: it proves which state was confirmed", got, seen.Digest)
	}
}

func TestTheBodyCannotNameTheNodeInTheAuditRecord(t *testing.T) {
	h := newDriftHarness(t)
	id := h.drifting(t)
	seen, _ := h.getDrift(t, id)
	h.openSudo(t)

	// The route reads no key but the digest and refuses this body, and the
	// attempt is on the record before the handler looks at it: the record is
	// what a client that lies about the node would have to fool.
	resp, raw := h.do(t, http.MethodPost, confirmPath(id), map[string]string{
		"digest": seen.Digest, "machine": "somebody-elses-node", "note": "free text a person might type",
	})
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("a body with keys the route does not know was accepted: %s", raw)
	}

	attempts, _ := h.driftConfirmRecords(t)
	if len(attempts) != 1 {
		t.Fatalf("the archive holds %d attempts, want 1", len(attempts))
	}
	rec := attempts[0]
	if got := rec.Params["machine"]; got != string(id) {
		t.Errorf("the record names the node %v, want the one in the path %q", got, id)
	}
	if got := rec.Params["note"]; got != audit.RedactedMarker {
		t.Errorf("a free-text key of the body is %v in the record, want %s", got, audit.RedactedMarker)
	}
	encoded, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal the record: %v", err)
	}
	for _, planted := range []string{"somebody-elses-node", "free text a person might type"} {
		if strings.Contains(string(encoded), planted) {
			t.Errorf("the record carries %q, which nothing ever removes from the archive: %s", planted, encoded)
		}
	}
}

func TestConfirmingTouchesNothingAtTheNode(t *testing.T) {
	h := newDriftHarness(t)
	id := h.drifting(t)
	seen, _ := h.getDrift(t, id)
	h.openSudo(t)

	// What a read costs the node, and what a write does: the calls the reader
	// and the apply path make.
	calls := func() map[string]int {
		out := map[string]int{}
		for _, m := range []string{"ApplyConfiguration", "Get", "Version", "List", "Reboot", "Upgrade"} {
			out[m] = h.sim.Calls(m)
		}
		return out
	}
	before := calls()

	if resp, raw := h.do(t, http.MethodPost, confirmPath(id), map[string]string{"digest": seen.Digest}); resp.StatusCode != http.StatusOK {
		t.Fatalf("confirming: %d (%s)", resp.StatusCode, raw)
	}
	after := calls()
	for m, n := range before {
		if after[m] != n {
			t.Errorf("confirming made the node hear %s %d times, want %d: it changes local state only", m, after[m]-n, 0)
		}
	}
}

func TestConfirmingWorksOnAReadOnlyAdoptedCluster(t *testing.T) {
	h := newDriftHarness(t)
	id := h.drifting(t)
	seen, _ := h.getDrift(t, id)

	// The cluster was imported, which adopts it read-only; say it again with
	// the route so the test does not lean on a default, and prove the lock
	// refuses what it is for.
	resp, list := h.do(t, http.MethodGet, "/api/v1/clusters", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clusters: %d (%s)", resp.StatusCode, list)
	}
	var clusters struct {
		Clusters []struct {
			ID string `json:"id"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal(list, &clusters); err != nil || len(clusters.Clusters) != 1 {
		t.Fatalf("decode the cluster list: %v (%s)", err, list)
	}
	cluster := clusters.Clusters[0].ID
	h.openSudo(t)
	if resp, raw := h.do(t, http.MethodPost, "/api/v1/clusters/"+cluster+"/lock",
		map[string]any{"locked": true}); resp.StatusCode != http.StatusOK {
		t.Fatalf("lock: %d (%s)", resp.StatusCode, raw)
	}
	resp, raw := h.do(t, http.MethodPost, "/api/v1/machines/"+string(id)+"/config/apply",
		map[string]any{"cluster": cluster, "patches": []string{"machine:\n  certSANs:\n    - 10.0.0.9\n"}})
	if resp.StatusCode != http.StatusForbidden || problemCode(raw) != "forbidden.cluster-locked" {
		t.Fatalf("precondition: the lock does not refuse an apply: %d (%s)", resp.StatusCode, raw)
	}

	resp, raw = h.do(t, http.MethodPost, confirmPath(id), map[string]string{"digest": seen.Digest})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirming on a cluster that is locked: %d (%s), want 200: it writes local state only", resp.StatusCode, raw)
	}
	if v, raw := h.getDrift(t, id); v.State != "ok" {
		t.Errorf("the node is %q after the confirmation, want ok: %s", v.State, raw)
	}
}

// TestTheWallCarriesNoConfigDrift is an observation, not a guard: the wall is
// not part of this phase, and a fault put back into this code would not turn it
// red. It says what the wall answers today, for a cluster whose card does carry
// config_drift.
func TestTheWallCarriesNoConfigDrift(t *testing.T) {
	nodes := []kubesim.Node{{Name: "cp-1", Ready: corev1.ConditionTrue, Roles: []string{"control-plane"}}}
	h, id, _ := adoptedClusterWithAPI(t, kubesim.Options{Nodes: nodes}, withDrift())
	if h.drift == nil {
		t.Fatal("the harness built no drift watch")
	}
	h.drift.Tick(context.Background())

	resp, list := h.do(t, http.MethodGet, "/api/v1/clusters", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(list), `"config_drift"`) {
		t.Fatalf("precondition: the cluster card does not carry config_drift: %d (%s)", resp.StatusCode, list)
	}

	path := "/api/v1/clusters/" + id + "/wall"
	token := mintWallLink(t, h)
	status, raw := asTheScreen(t, h, http.MethodGet, path, token, nil)
	if status != http.StatusOK {
		t.Fatalf("the wall answered %d (%s); the observation needs the real answer", status, raw)
	}
	if strings.Contains(strings.ToLower(raw), "drift") {
		t.Errorf("the wall answer mentions drift: %s", raw)
	}
	resp, session := h.do(t, http.MethodGet, path, nil)
	if resp.StatusCode != http.StatusOK || strings.Contains(strings.ToLower(string(session)), "drift") {
		t.Errorf("the wall answer to a session: %d, mentions drift: %s", resp.StatusCode, session)
	}
}
