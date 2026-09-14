package httpapi_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
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
// testSchematic is a well-formed schematic id for the tests below.
const testSchematic = "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"

// fakeFactory answers the one request the installer resolver makes: a registry
// manifest GET at /v2/<repo>/<schematic>/manifests/<version>.
//
// answered records every repository name it was asked about, in order, which
// is what lets a test say *which* installer was resolved rather than only that
// one was. only, when non-empty, is the set of repository names that answer;
// everything else is a 404, which is how the resolver's candidate order and
// its SecureBoot naming become observable.
type fakeFactory struct {
	mu       sync.Mutex
	answered []string
	only     map[string]bool
}

func (f *fakeFactory) start(t *testing.T) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		// /v2/<repo>/<schematic>/manifests/<version>
		if len(parts) < 5 || parts[0] != "v2" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		repo := parts[1]

		f.mu.Lock()
		f.answered = append(f.answered, repo)
		allowed := len(f.only) == 0 || f.only[repo]
		f.mu.Unlock()

		if !allowed {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		_, _ = w.Write([]byte(`{"schemaVersion":2}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func (f *fakeFactory) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.answered...)
}

func newProvisionHarness(t *testing.T) *provisionHarness {
	t.Helper()
	return newProvisionHarnessWith(t, nil)
}

// newProvisionHarnessWith is newProvisionHarness with an Image Factory.
//
// A harness with no Factory is the honest default: most of these tests are
// about routes and jobs, and giving every one of them a registry to talk to
// would hide which ones actually depend on it.
func newProvisionHarnessWith(t *testing.T, factory *fakeFactory) *provisionHarness {
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
		// The kind is registered so that the engine accepts a submission. The
		// steps are the real ones; the simulated machine is what they act on.
		withProvisionJob(func(e *jobs.Engine, h *harness) {
			provision.Register(e, provision.Deps{
				Dialer:           dialer,
				MaintenanceCreds: func(string) talos.Creds { return blank.MaintenanceCreds() },
				ClusterCreds:     h.inv.ClusterCreds,
				Secrets: func(ctx context.Context, id model.ClusterID) (model.ClusterSecrets, error) {
					return h.store.ClusterSecrets().Get(ctx, id)
				},
				Cluster: func(ctx context.Context, id model.ClusterID) (model.Cluster, error) {
					return h.store.Clusters().Get(ctx, id)
				},
				PatchBodies:       func(context.Context, []string) ([]string, error) { return nil, nil },
				Record:            func(context.Context, model.ClusterID, string, bool) error { return nil },
				Bootstrapper:      bootstrapper(t, h),
				KubernetesVersion: h.inv.KubernetesVersion,
			})
		}),
		withProvision(func(h *harness) *provision.Service {
			return provision.NewService(dialer,
				func(string) talos.Creds { return blank.MaintenanceCreds() },
				h.inv.KnownAt, bootstrapper(t, h), h.inv.ControlPlaneCount)
		}),
		factoryOpt(t, factory),
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

	// The schematic record the installer resolution reads its architecture
	// from. It is seeded here rather than in each test because a machine that
	// booted a schematic this installation does not hold is its own refusal,
	// with its own test.
	if _, err := h.store.Schematics().Put(context.Background(), model.Schematic{
		ID:           testSchematic,
		Name:         "homelab",
		TalosVersion: "v1.13.9",
		Arch:         "amd64",
		Usable:       true,
	}); err != nil {
		t.Fatalf("seed the schematic: %v", err)
	}

	return &provisionHarness{harness: h, blank: blank}
}

// factoryOpt is withFactory when there is one and a no-op when there is not,
// so the harness builder stays one expression.
func factoryOpt(t *testing.T, f *fakeFactory) harnessOpt {
	if f == nil {
		return func(*harnessConfig) {}
	}
	return withFactory(f.start(t))
}

// bootstrapper opens the harness's lease directory, once per harness: the
// service and the job steps must share it, because a second Bootstrapper over
// the same directory would be a second lease-holder and the lease is the
// mechanism.
func bootstrapper(t *testing.T, h *harness) *provision.Bootstrapper {
	t.Helper()

	if h.bootstrap == nil {
		b, err := provision.NewBootstrapper(h.dataDir + "/bootstrap")
		if err != nil {
			t.Fatalf("NewBootstrapper: %v", err)
		}
		h.bootstrap = b
	}
	return h.bootstrap
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

	h := newProvisionHarnessWith(t, &fakeFactory{})

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
		"schematic_id":  testSchematic,
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

// TestAConfirmedApplyIsAcceptedAsAJob is JOB-09 for the provisioning path: the
// operation has been accepted and has not happened yet, and answering 200
// would be claiming otherwise.
//
// It also pins the confirmation this path uses, which is deliberately not the
// machine-scoped one: a machine being provisioned is not in the inventory, so
// there is no hostname to type. What is typed is the disk that gets written.
func TestAConfirmedApplyIsAcceptedAsAJob(t *testing.T) {
	t.Parallel()

	h := newProvisionHarness(t)

	resp, raw := h.do(t, http.MethodPost, "/api/v1/provision/inspect",
		map[string]any{"addr": h.blank.Host()})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("inspect: %d (%s)", resp.StatusCode, raw)
	}
	var candidate provision.Candidate
	if err := json.Unmarshal(raw, &candidate); err != nil {
		t.Fatalf("decode candidate: %v", err)
	}

	req := map[string]any{
		"cluster":       string(testProvisionCluster),
		"addr":          h.blank.Host(),
		"uuid":          string(candidate.UUID),
		"control_plane": true,
		"install_disk":  candidate.Disks[0].Device,
		"talos_version": "v1.13.9",
	}

	resp, raw = h.do(t, http.MethodPost, "/api/v1/auth/sudo", map[string]string{"password": testPass})
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
	}

	// The wrong device: refused, so a client that skipped the box cannot get a
	// token at all.
	wrong := map[string]any{"typed": "/dev/sdz"}
	for k, v := range req {
		wrong[k] = v
	}
	resp, raw = h.do(t, http.MethodPost, "/api/v1/provision/confirm", wrong)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a confirmation naming the wrong disk answered %d (%s)", resp.StatusCode, raw)
	}

	body := map[string]any{"typed": candidate.Disks[0].Device}
	for k, v := range req {
		body[k] = v
	}
	resp, raw = h.do(t, http.MethodPost, "/api/v1/provision/confirm", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirm: %d (%s)", resp.StatusCode, raw)
	}
	var confirmation struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &confirmation); err != nil {
		t.Fatalf("decode confirmation: %v", err)
	}

	// A token is bound to the parameters it was issued for. Submitting a
	// different disk with it is refused, which is what makes the dialog a gate
	// rather than decoration.
	tampered := map[string]any{"confirmation": confirmation.Token}
	for k, v := range req {
		tampered[k] = v
	}
	tampered["install_disk"] = "/dev/sdz"
	resp, raw = h.do(t, http.MethodPost, "/api/v1/provision/apply", tampered)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("an apply with a token for a different disk answered %d (%s)", resp.StatusCode, raw)
	}

	apply := map[string]any{"confirmation": confirmation.Token}
	for k, v := range req {
		apply[k] = v
	}
	resp, raw = h.do(t, http.MethodPost, "/api/v1/provision/apply", apply)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("apply: %d (%s)", resp.StatusCode, raw)
	}
	if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "/api/v1/jobs/") {
		t.Errorf("the acceptance does not say where to watch the run: Location = %q", loc)
	}

	var accepted struct {
		Job struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"job"`
		Topic string `json:"topic"`
	}
	if err := json.Unmarshal(raw, &accepted); err != nil {
		t.Fatalf("decode acceptance: %v", err)
	}
	if accepted.Job.ID == "" || accepted.Topic == "" {
		t.Fatalf("the acceptance carries no job to watch: %s", raw)
	}
	if accepted.Job.Kind != string(provision.JobKindProvision) {
		t.Errorf("the accepted job is a %q", accepted.Job.Kind)
	}
}

// TestAStockMachineGetsTalosOwnInstaller.
//
// A machine that was not built from an Image Factory schematic has no Factory
// repository to resolve, and Talos's published installer is the answer. It is
// the one case where a reference is built rather than resolved, and it is
// worth pinning so that "there is nothing to resolve" does not quietly become
// "resolve it anyway and fail".
func TestAStockMachineGetsTalosOwnInstaller(t *testing.T) {
	t.Parallel()

	h := newProvisionHarness(t)

	resp, raw := h.do(t, http.MethodPost, "/api/v1/provision/plan", map[string]any{
		"cluster":       string(testProvisionCluster),
		"addr":          h.blank.Host(),
		"uuid":          "00000000-0000-4000-8000-0000000000cc",
		"install_disk":  "/dev/nvme0n1",
		"talos_version": "v1.13.9",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("plan: %d (%s)", resp.StatusCode, raw)
	}

	var preview provision.Preview
	if err := json.Unmarshal(raw, &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if preview.InstallImage != "ghcr.io/siderolabs/installer:v1.13.9" {
		t.Errorf("a stock machine installs %q", preview.InstallImage)
	}
}

// TestASchematicWithNoFactoryIsRefusedRatherThanGuessed.
//
// This is the decision the change made, stated as a test because it is a
// trade-off and not an obvious win: a plan naming a schematic now needs a
// Factory to resolve its installer against, and without one it is refused.
//
// The alternative is what shipped before -- assemble
// "factory.talos.dev/installer/<id>:<version>" by hand -- and it was wrong
// three ways at once. It never carried SecureBoot, so a machine booted from a
// SecureBoot ISO installed a system that is not SecureBoot. It assumed the
// legacy repository name rather than asking which one answers. And it named
// the public Factory, so an installation pointed at a private one with
// --image-factory provisioned nodes that pulled their installer from
// somewhere the operator never configured. None of the three reported
// anything.
func TestASchematicWithNoFactoryIsRefusedRatherThanGuessed(t *testing.T) {
	t.Parallel()

	h := newProvisionHarness(t)

	resp, raw := h.do(t, http.MethodPost, "/api/v1/provision/plan", map[string]any{
		"cluster":       string(testProvisionCluster),
		"addr":          h.blank.Host(),
		"uuid":          "00000000-0000-4000-8000-0000000000dd",
		"install_disk":  "/dev/nvme0n1",
		"talos_version": "v1.13.9",
		"schematic_id":  "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("plan: %d (%s), want 400", resp.StatusCode, raw)
	}
	if !strings.Contains(string(raw), "image-factory") {
		t.Errorf("the refusal does not say what to configure: %s", raw)
	}

	// And nothing was substituted. A refusal that still handed back a
	// reference would be one a client can ignore by not reading the status.
	if strings.Contains(string(raw), "factory.talos.dev/installer/") {
		t.Errorf("an installer reference was produced for a refused plan: %s", raw)
	}
}

// TestASecureBootMachineGetsTheSecureBootInstaller.
//
// The defect this replaced: the installer reference was assembled by hand as
// "factory.talos.dev/installer/<id>:<version>" for every machine, so a machine
// booted from a SecureBoot ISO installed a system that is not SecureBoot. The
// install succeeds, the node joins, and the property the image was built for
// is gone, with nothing reporting it. internal/imagefactory names this exact
// pairing as "the ISO/installer drift this file's own comments warn about,
// arriving from the one direction nothing checked" -- and provisioning was
// that direction.
func TestASecureBootMachineGetsTheSecureBootInstaller(t *testing.T) {
	t.Parallel()

	f := &fakeFactory{}
	h := newProvisionHarnessWith(t, f)

	plan := func(secureBoot bool) provision.Preview {
		t.Helper()
		resp, raw := h.do(t, http.MethodPost, "/api/v1/provision/plan", map[string]any{
			"cluster":       string(testProvisionCluster),
			"addr":          h.blank.Host(),
			"uuid":          "00000000-0000-4000-8000-0000000000ee",
			"install_disk":  "/dev/nvme0n1",
			"talos_version": "v1.13.9",
			"schematic_id":  testSchematic,
			"secureboot":    secureBoot,
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("plan: %d (%s)", resp.StatusCode, raw)
		}
		var p provision.Preview
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatalf("decode preview: %v", err)
		}
		return p
	}

	secure := plan(true)
	if !strings.Contains(secure.InstallImage, "secureboot") {
		t.Errorf("a SecureBoot machine installs %q", secure.InstallImage)
	}

	ordinary := plan(false)
	if strings.Contains(ordinary.InstallImage, "secureboot") {
		t.Errorf("an ordinary machine installs %q", ordinary.InstallImage)
	}
	if secure.InstallImage == ordinary.InstallImage {
		t.Fatalf("both resolve to the same installer: %q", secure.InstallImage)
	}

	// The reference points at the Factory this installation was configured
	// with, not at the public one. That was the third way the hand-built
	// string was wrong: an installation pointed at a private Factory with
	// --image-factory provisioned nodes that pulled their installer from
	// factory.talos.dev.
	if strings.Contains(secure.InstallImage, "factory.talos.dev") {
		t.Errorf("the installer is pulled from the public Factory: %q", secure.InstallImage)
	}
	for _, want := range []string{testSchematic, "v1.13.9"} {
		if !strings.Contains(secure.InstallImage, want) {
			t.Errorf("the installer reference lost %q: %q", want, secure.InstallImage)
		}
	}
}

// TestTheResolverIsAskedForThePreferredRepositoryFirst.
//
// The name is resolved rather than assumed, and the old code assumed the
// *legacy* one. internal/imagefactory keeps an ordered candidate list because
// which name answers varies by version, and asks for the platform-prefixed
// name first.
func TestTheResolverIsAskedForThePreferredRepositoryFirst(t *testing.T) {
	t.Parallel()

	// Only the preferred name answers, so a resolver that asked for the legacy
	// one and took it would produce nothing here.
	f := &fakeFactory{only: map[string]bool{"metal-installer": true}}
	h := newProvisionHarnessWith(t, f)

	resp, raw := h.do(t, http.MethodPost, "/api/v1/provision/plan", map[string]any{
		"cluster":       string(testProvisionCluster),
		"addr":          h.blank.Host(),
		"uuid":          "00000000-0000-4000-8000-0000000000ff",
		"install_disk":  "/dev/nvme0n1",
		"talos_version": "v1.13.9",
		"schematic_id":  testSchematic,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("plan: %d (%s)", resp.StatusCode, raw)
	}

	var p provision.Preview
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if !strings.Contains(p.InstallImage, "/metal-installer/") {
		t.Errorf("the resolved reference is %q, want the preferred repository", p.InstallImage)
	}

	asked := f.asked()
	if len(asked) == 0 {
		t.Fatal("the Factory was never asked which repository answers")
	}
}
