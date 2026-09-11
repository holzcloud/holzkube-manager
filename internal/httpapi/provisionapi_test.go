package httpapi_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/provision"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// The provisioning surface, through the real router: the middleware chain, the
// audit capture, the sudo gate and the per-cluster lock all run.

// testProvisionCluster is the cluster the fixture's machine joins.
const testProvisionCluster = model.ClusterID("c1")

type provisionHarness struct {
	*harness
	blank *talossim.Server
}

// newProvisionHarness serves a wizard against one simulated machine that is
// waiting for a configuration.
func newProvisionHarness(t *testing.T) *provisionHarness {
	t.Helper()

	blank, err := talossim.New(talossim.Options{
		Hostname:    "blank-1",
		Maintenance: true,
		Disks: []talossim.DiskFixture{
			{Device: "nvme0n1", Size: 512 << 30, Model: "SIMULATED NVMe", Serial: "SN-1", Transport: "nvme"},
		},
	})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = blank.Close() })

	dialer := talos.NewDirectDialer(blank.Port())

	h := newHarness(t,
		withInventory(func(st *fsstore.Store) *inventory.Service {
			return inventory.New(inventory.Deps{
				Store:  st,
				Dialer: dialer,
				Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
			})
		}),
		withProvision(func(h *harness) *provision.Service {
			b, err := provision.NewBootstrapper(h.dataDir + "/bootstrap")
			if err != nil {
				t.Fatalf("NewBootstrapper: %v", err)
			}
			return provision.NewService(dialer,
				func(string) talos.Creds { return blank.MaintenanceCreds() },
				h.inv.KnownAt, b, h.inv.ControlPlaneCount)
		}),
	)

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

	// The cluster the machine joins. A provisioning run is always into a
	// cluster that already exists -- it is created, or adopted, before there
	// is anything to provision into -- and the cluster-scoped routes are
	// checked against its lock, so the fixture has to have one.
	if _, err := h.store.Clusters().Put(context.Background(), model.Cluster{
		ID: testProvisionCluster, Name: "homelab", Endpoint: "https://10.0.0.10:6443",
	}); err != nil {
		t.Fatalf("seed the cluster: %v", err)
	}

	return &provisionHarness{harness: h, blank: blank}
}

// TestTheWizardFindsAMachineAndReadsIt walks the two read steps an operator
// takes before anything can be written.
func TestTheWizardFindsAMachineAndReadsIt(t *testing.T) {
	t.Parallel()

	h := newProvisionHarness(t)

	resp, raw := h.do(t, http.MethodPost, "/api/v1/provision/scan",
		map[string]any{"addrs": []string{h.blank.Host()}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("scan: %d (%s)", resp.StatusCode, raw)
	}

	var scan struct {
		Found []struct {
			Addr  string `json:"addr"`
			State string `json:"state"`
			Known bool   `json:"known"`
		} `json:"found"`
		Notices []string `json:"notices"`
	}
	if err := json.Unmarshal(raw, &scan); err != nil {
		t.Fatalf("decode scan: %v", err)
	}
	if len(scan.Found) != 1 || scan.Found[0].State != string(provision.StateMaintenance) {
		t.Fatalf("the scan reported %+v", scan.Found)
	}
	if scan.Found[0].Known {
		t.Error("a machine the inventory has never heard of was reported as known")
	}

	// PROV-12: both boot warnings, on the screen where "the machine never
	// appeared" is about to be interpreted.
	joined := strings.Join(scan.Notices, "\n")
	for _, want := range []string{"boot from the disk", "no DHCP", "maintenance mode"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the scan carries no notice mentioning %q: %v", want, scan.Notices)
		}
	}

	resp, raw = h.do(t, http.MethodPost, "/api/v1/provision/inspect",
		map[string]any{"addr": h.blank.Host()})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("inspect: %d (%s)", resp.StatusCode, raw)
	}

	var candidate provision.Candidate
	if err := json.Unmarshal(raw, &candidate); err != nil {
		t.Fatalf("decode candidate: %v", err)
	}
	if candidate.UUID == "" || len(candidate.Disks) == 0 || candidate.Fingerprint == "" {
		t.Fatalf("the candidate is missing what identifies the machine: %+v", candidate)
	}
}

// TestAPlanNamingNoMachineIsRefusedBeforeAnythingIsAccepted is PROV-05 at the
// edge of the API: a 202 for a run that cannot start is a 202 that says the
// machine is being provisioned.
func TestAPlanNamingNoMachineIsRefusedBeforeAnythingIsAccepted(t *testing.T) {
	t.Parallel()

	h := newProvisionHarness(t)

	resp, raw := h.do(t, http.MethodPost, "/api/v1/provision/plan", map[string]any{
		"cluster":       string(testProvisionCluster),
		"addr":          h.blank.Host(),
		"install_disk":  "/dev/nvme0n1",
		"talos_version": "v1.13.9",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a plan with no UUID answered %d (%s)", resp.StatusCode, raw)
	}
	if !strings.Contains(string(raw), "UUID") {
		t.Errorf("the refusal does not say what is missing: %s", raw)
	}
}

// TestTheEvenControlPlaneWarningReachesTheScreen is PROV-07, end to end: the
// warning is computed against the cluster as it will be, and it is on the last
// screen before the apply rather than in a log.
func TestTheEvenControlPlaneWarningReachesTheScreen(t *testing.T) {
	t.Parallel()

	h := newProvisionHarness(t)

	// One control-plane node already in the inventory, so this plan would make
	// two.
	if _, err := h.store.Machines().Put(context.Background(), model.Machine{
		ID:      model.MachineID("00000000-0000-4000-8000-0000000000aa"),
		Cluster: testProvisionCluster,
		Role:    model.RoleControlPlane,
		Addr:    "10.0.0.10",
	}); err != nil {
		t.Fatalf("seed a control-plane node: %v", err)
	}

	resp, raw := h.do(t, http.MethodPost, "/api/v1/provision/plan", map[string]any{
		"cluster":       string(testProvisionCluster),
		"addr":          h.blank.Host(),
		"uuid":          "00000000-0000-4000-8000-0000000000bb",
		"control_plane": true,
		"install_disk":  "/dev/nvme0n1",
		"talos_version": "v1.13.9",
		"schematic_id":  "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("plan: %d (%s)", resp.StatusCode, raw)
	}

	var preview provision.Preview
	if err := json.Unmarshal(raw, &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if preview.ControlPlaneAfter != 2 {
		t.Errorf("the preview says the cluster will have %d control-plane nodes, want 2",
			preview.ControlPlaneAfter)
	}
	if preview.Bootstrap {
		t.Error("a second control-plane node is planned to bootstrap etcd, which would destroy the cluster")
	}

	var warned bool
	for _, w := range preview.Warnings {
		if strings.Contains(w, "quorum") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("no quorum warning on a plan that makes two control-plane nodes: %v", preview.Warnings)
	}

	// PROV-08: the install image names the same schematic the ISO was built
	// from, on the screen, before the apply.
	if !strings.Contains(preview.InstallImage, "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba") {
		t.Errorf("the preview's install image %q does not name the schematic", preview.InstallImage)
	}
}

// TestTheApplyIsGatedLikeEveryOtherDestructiveRoute pins that provisioning did
// not quietly get its own weaker door. A machine being wiped is a machine
// being wiped, whether the request calls it a reset or a provision.
func TestTheApplyIsGatedLikeEveryOtherDestructiveRoute(t *testing.T) {
	t.Parallel()

	h := newProvisionHarness(t)

	body := map[string]any{
		"cluster":       string(testProvisionCluster),
		"addr":          h.blank.Host(),
		"uuid":          "00000000-0000-4000-8000-0000000000bb",
		"install_disk":  "/dev/nvme0n1",
		"talos_version": "v1.13.9",
	}

	// No sudo window: refused before the body is even considered, with the
	// status that tells a client to ask for the password rather than to give
	// up.
	resp, raw := h.do(t, http.MethodPost, "/api/v1/provision/apply", body)
	if resp.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("an apply outside the sudo window answered %d (%s)", resp.StatusCode, raw)
	}

	resp, raw = h.do(t, http.MethodPost, "/api/v1/auth/sudo", map[string]string{"password": testPass})
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
	}

	// Inside the window, and still refused: there is no confirmation, and the
	// confirmation is issued by the server against the parameters actually
	// submitted.
	resp, raw = h.do(t, http.MethodPost, "/api/v1/provision/apply", body)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("an apply with no confirmation answered %d (%s)", resp.StatusCode, raw)
	}
	if !strings.Contains(string(raw), "confirmation") {
		t.Errorf("the refusal does not name what is missing: %s", raw)
	}
}

// TestTheBootstrapRecoveryScreenSaysWhatToLookAt is PROV-10's recovery half.
//
// The operator arriving here is being asked to make the decision the four
// mechanisms exist to avoid making automatically, and the one thing they must
// not do is guess -- so the screen has to say what to look at.
func TestTheBootstrapRecoveryScreenSaysWhatToLookAt(t *testing.T) {
	t.Parallel()

	h := newProvisionHarness(t)

	// The shape a crash mid-call leaves: the lease taken, no outcome.
	intent := `{"cluster":"c1","machine":"00000000-0000-4000-8000-0000000000bb",` +
		`"addr":"10.0.0.10","started_at":"2026-09-11T12:00:00Z"}`
	if err := os.WriteFile(h.dataDir+"/bootstrap/c1.json", []byte(intent), 0o600); err != nil {
		t.Fatalf("write the interrupted record: %v", err)
	}

	resp, raw := h.do(t, http.MethodGet, "/api/v1/provision/bootstrap-recovery", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap-recovery: %d (%s)", resp.StatusCode, raw)
	}

	var body struct {
		Pending []struct {
			Cluster string `json:"cluster"`
			Addr    string `json:"addr"`
		} `json:"pending"`
		Guidance string `json:"guidance"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Pending) != 1 || body.Pending[0].Addr != "10.0.0.10" {
		t.Fatalf("the unclear attempt is not offered for recovery: %+v", body.Pending)
	}
	for _, want := range []string{"etcd members", "Do not retry"} {
		if !strings.Contains(body.Guidance, want) {
			t.Errorf("the guidance does not mention %q: %q", want, body.Guidance)
		}
	}

	// Resolving needs the sudo window, like every other destructive route.
	resolve := map[string]any{"bootstrapped": true, "note": "etcd has three members"}
	resp, raw = h.do(t, http.MethodPost, "/api/v1/provision/bootstrap-recovery/c1", resolve)
	if resp.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("a resolution outside the sudo window answered %d (%s)", resp.StatusCode, raw)
	}

	resp, raw = h.do(t, http.MethodPost, "/api/v1/auth/sudo", map[string]string{"password": testPass})
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
	}

	// A resolution with no verdict is refused: there is no default, because
	// guessing here is the operation the record exists to prevent.
	resp, raw = h.do(t, http.MethodPost, "/api/v1/provision/bootstrap-recovery/c1",
		map[string]any{"note": "I did not look"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a resolution with no verdict answered %d (%s)", resp.StatusCode, raw)
	}

	resp, raw = h.do(t, http.MethodPost, "/api/v1/provision/bootstrap-recovery/c1", resolve)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resolve: %d (%s)", resp.StatusCode, raw)
	}

	resp, raw = h.do(t, http.MethodGet, "/api/v1/provision/bootstrap-recovery", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap-recovery: %d (%s)", resp.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Pending) != 0 {
		t.Errorf("the attempt is still pending after it was resolved: %+v", body.Pending)
	}
}

// TestTheAuditRecordOfAScanNamesTheSubnet is the audit half.
//
// "A scan happened" and "this installation scanned 10.0.0.0/24" are different
// events, and the second is the one somebody reviewing an unexpected
// connection on their own network is looking for.
func TestTheAuditRecordOfAScanNamesTheSubnet(t *testing.T) {
	t.Parallel()

	h := newProvisionHarness(t)

	resp, raw := h.do(t, http.MethodPost, "/api/v1/provision/scan",
		map[string]any{"addrs": []string{h.blank.Host()}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("scan: %d (%s)", resp.StatusCode, raw)
	}

	resp, raw = h.do(t, http.MethodGet, "/api/v1/audit?action=provision.scan", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("audit: %d (%s)", resp.StatusCode, raw)
	}
	if !strings.Contains(string(raw), h.blank.Host()) {
		t.Errorf("the audit record of a scan does not name what was scanned: %s", raw)
	}
}
