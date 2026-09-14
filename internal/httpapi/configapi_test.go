package httpapi_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

func newConfigHarness(t *testing.T) *inventoryHarness {
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
		withConfig(),
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

	return &inventoryHarness{harness: h, sim: sim, cluster: cl}
}

// TestTheConfigViewCarriesNoSecret is CFG-01 and CFG-02 at the boundary they
// meet: the view is the leak, so the redaction has to be in front of it rather
// than applied by whoever remembers.
func TestTheConfigViewCarriesNoSecret(t *testing.T) {
	h := newConfigHarness(t)
	id := h.adoptedMachine(t)

	resp, raw := h.do(t, http.MethodGet, "/api/v1/machines/"+string(id)+"/config", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("config: %d (%s)", resp.StatusCode, raw)
	}

	var view struct {
		Raw      string `json:"raw"`
		Rendered string `json:"rendered"`
	}
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatalf("decode view: %v", err)
	}
	if view.Raw == "" || view.Rendered == "" {
		t.Fatal("the view is empty; CFG-01 asks for both the raw and the rendered form")
	}

	secrets := map[string]string{
		"the Talos CA private key":   string(h.cluster.Secrets.Certs.OS.Key),
		"the cluster secret":         h.cluster.Secrets.Cluster.Secret,
		"the machine (trustd) token": h.cluster.Secrets.TrustdInfo.Token,
	}
	for name, secret := range secrets {
		if secret == "" {
			continue
		}
		if strings.Contains(string(raw), secret) {
			t.Errorf("the config response leaks %s", name)
		}
	}
	if machineconfig.ContainsPrivateKey(raw) {
		t.Error("the config response still contains a PEM private key")
	}
	if !strings.Contains(view.Raw, machineconfig.RedactedMarker) {
		t.Error("nothing in the view is marked as redacted, which suggests nothing was")
	}
}

// TestThePlanShowsTheDiffTheModeAndTheIdempotence is CFG-04, CFG-05, CFG-06
// and CFG-10 in one call, because that is how an operator meets them: one
// screen before the apply.
func TestThePlanShowsTheDiffTheModeAndTheIdempotence(t *testing.T) {
	h := newConfigHarness(t)
	id := h.adoptedMachine(t)

	resp, raw := h.do(t, http.MethodPost, "/api/v1/machines/"+string(id)+"/config/plan",
		map[string]any{
			// Appends to a list, so the plan must report it as not idempotent
			// and as a list that grew.
			"patches": []string{"machine:\n  certSANs:\n    - 10.0.0.9\n"},
		})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("plan: %d (%s)", resp.StatusCode, raw)
	}

	var preview struct {
		Diff struct {
			Changes []struct {
				Path      string `json:"path"`
				Kind      string `json:"kind"`
				LenBefore int    `json:"len_before"`
				LenAfter  int    `json:"len_after"`
			} `json:"changes"`
		} `json:"diff"`
		Verdict struct {
			Mode      string   `json:"mode"`
			Sentences []string `json:"sentences"`
		} `json:"verdict"`
		Idempotent bool   `json:"idempotent"`
		Valid      bool   `json:"valid"`
		Result     string `json:"result"`
	}
	if err := json.Unmarshal(raw, &preview); err != nil {
		t.Fatalf("decode preview: %v (%s)", err, raw)
	}

	if len(preview.Diff.Changes) == 0 {
		t.Fatal("the plan reports no changes for a patch that adds a certSAN")
	}
	if preview.Idempotent {
		t.Error("a patch that appends to a list was reported as idempotent")
	}
	if preview.Verdict.Mode == "" || len(preview.Verdict.Sentences) == 0 {
		t.Error("the plan does not say which apply mode this needs, or why")
	}

	// And the preview's own result is redacted: it is the fourth exit.
	if machineconfig.ContainsPrivateKey([]byte(preview.Result)) {
		t.Error("the plan's result still contains a PEM private key")
	}
}

// TestAJSONPatchIsRefusedWithItsOwnCode pins CFG-03's exclusion at the
// boundary, so a client can tell "fix this patch" from "use the other form".
func TestAJSONPatchIsRefusedWithItsOwnCode(t *testing.T) {
	h := newConfigHarness(t)

	resp, raw := h.do(t, http.MethodPost, "/api/v1/patches", map[string]any{
		"name": "by index",
		"body": "- op: replace\n  path: /machine/certSANs/0\n  value: 10.0.0.9\n",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a JSON patch: %d (%s), want 400", resp.StatusCode, raw)
	}
	p := decodeProblem(t, resp, raw)
	if p.Code != httpapi.CodePatchNotStrategic {
		t.Fatalf("code = %q, want %q", p.Code, httpapi.CodePatchNotStrategic)
	}
}

// TestEditingAPatchWritesANewVersion is CFG-03's append-only half.
//
// The old version stays readable. That is the whole point: "what exactly was
// applied to this node in March" only has an answer if the thing applied still
// exists.
func TestEditingAPatchWritesANewVersion(t *testing.T) {
	h := newConfigHarness(t)

	resp, raw := h.do(t, http.MethodPost, "/api/v1/patches", map[string]any{
		"name": "hostname",
		"body": "machine:\n  network:\n    hostname: node-1\n",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d (%s)", resp.StatusCode, raw)
	}
	var first model.Patch
	if err := json.Unmarshal(raw, &first); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if first.Version != 1 {
		t.Errorf("a new patch is version %d, want 1", first.Version)
	}

	resp, raw = h.do(t, http.MethodPost, "/api/v1/patches", map[string]any{
		"name":   "hostname",
		"body":   "machine:\n  network:\n    hostname: node-2\n",
		"parent": string(first.ID),
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("edit: %d (%s)", resp.StatusCode, raw)
	}
	var second model.Patch
	if err := json.Unmarshal(raw, &second); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if second.Version != 2 || second.Parent != first.ID {
		t.Errorf("the edit is version %d with parent %q", second.Version, second.Parent)
	}
	if second.ID == first.ID {
		t.Fatal("the edit rewrote the first version rather than writing a new one")
	}

	// The old one is still there, marked rather than gone.
	resp, raw = h.do(t, http.MethodGet, "/api/v1/patches/"+string(first.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the superseded version is no longer readable: %d (%s)", resp.StatusCode, raw)
	}
	var stored model.Patch
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !stored.Superseded {
		t.Error("the old version is not marked superseded")
	}
	if stored.Body != "machine:\n  network:\n    hostname: node-1\n" {
		t.Error("the old version's body changed")
	}
}

// TestTheAuditArchiveNeverGetsAPatchBody is the other side of the redaction
// claim, in the one place a leak is permanent.
func TestTheAuditArchiveNeverGetsAPatchBody(t *testing.T) {
	h := newConfigHarness(t)
	id := h.adoptedMachine(t)

	const marker = "a-value-that-should-not-be-archived"
	resp, raw := h.do(t, http.MethodPost, "/api/v1/machines/"+string(id)+"/config/plan",
		map[string]any{
			"patches": []string{"machine:\n  network:\n    hostname: " + marker + "\n"},
		})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("plan: %d (%s)", resp.StatusCode, raw)
	}

	records := h.auditPage(t, "?limit=100")
	body, err := json.Marshal(records.Items)
	if err != nil {
		t.Fatalf("marshal records: %v", err)
	}
	if strings.Contains(string(body), marker) {
		t.Fatal("a patch body reached the audit archive, which has no deletion path")
	}
	if !hasAction(records.Items, "config.plan") {
		t.Error("the plan is not in the archive at all")
	}
}

// supersedeRace is a store that lets somebody else write the parent patch
// between the handler's read of it and its write.
//
// The window is real and narrow: createPatch reads the parent, sets
// Superseded, and writes it back. Two operators editing the same patch at once
// land in it. A real store offers no way to ask for that moment, so the
// decorator chooses it.
type supersedeRace struct {
	store.Store
	patches *racingPatches
}

func (s supersedeRace) Patches() store.PatchStore { return s.patches }

type racingPatches struct {
	store.PatchStore

	mu    sync.Mutex
	armed bool
}

// arm makes the next read of a patch the one that gets overtaken.
//
// Inert until asked for, because the create path writes and reads patches
// before the test has anything to say about it.
func (p *racingPatches) arm() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.armed = true
}

func (p *racingPatches) Get(ctx context.Context, id model.PatchID) (model.Patch, error) {
	rec, err := p.PatchStore.Get(ctx, id)
	if err != nil {
		return rec, err
	}

	p.mu.Lock()
	fire := p.armed
	p.armed = false
	p.mu.Unlock()

	if fire {
		// The competing writer: it touches the same record, which moves its
		// Rev. The handler is now holding the version it read a moment ago,
		// which is exactly the state a second operator's request produces.
		other := rec
		other.Description = "written by somebody else"
		// p.Put and not p.PatchStore.Put: Put is not overridden here, so the
		// qualifier would say something that is not true of this type.
		if _, err := p.Put(ctx, other); err != nil {
			return rec, err
		}
	}
	return rec, nil
}

// TestTwoOperatorsEditingOnePatchGetAConflictAndNotAnInternalError.
//
// The write that supersedes a parent used to map every failure to 500
// internal.unexpected, including a compare-and-swap clash. A 500 tells an
// operator something is broken and to stop; what actually happened is that the
// chain moved under them and the request can be made again against its head.
// The taxonomy already has the answer -- 409 store.conflict -- and this route
// was not using it.
func TestTwoOperatorsEditingOnePatchGetAConflictAndNotAnInternalError(t *testing.T) {
	racing := &racingPatches{}

	h := newHarness(t,
		withConfig(),
		withStore(func(st store.Store) store.Store {
			racing.PatchStore = st.Patches()
			return supersedeRace{Store: st, patches: racing}
		}),
	)
	h.setupAndLogin(t)

	resp, raw := h.do(t, http.MethodPost, "/api/v1/patches", map[string]any{
		"name": "hostname",
		"body": "machine:\n  network:\n    hostname: node-1\n",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d (%s)", resp.StatusCode, raw)
	}
	var first model.Patch
	if err := json.Unmarshal(raw, &first); err != nil {
		t.Fatalf("decode: %v", err)
	}

	racing.arm()

	resp, raw = h.do(t, http.MethodPost, "/api/v1/patches", map[string]any{
		"name":   "hostname",
		"body":   "machine:\n  network:\n    hostname: node-2\n",
		"parent": string(first.ID),
	})
	if resp.StatusCode >= 500 {
		t.Fatalf("a race between two operators answered %d: %s", resp.StatusCode, raw)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("the edit answered %d, want 409 (%s)", resp.StatusCode, raw)
	}

	p := decodeProblem(t, resp, raw)
	if p.Code != "store.conflict" {
		t.Errorf("code = %q, want store.conflict", p.Code)
	}
	if !strings.Contains(p.Detail, "current head") {
		t.Errorf("the refusal does not say what to do about it: %q", p.Detail)
	}
}
