package httpapi_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/httpapi/handlers"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

func newJobHarness(t *testing.T) *inventoryHarness {
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
		withJobs(),
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
	// Node actions are destructive, so the sudo window has to be open.
	resp, raw = h.do(t, http.MethodPost, "/api/v1/auth/sudo", map[string]string{"password": testPass})
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
	}

	return &inventoryHarness{harness: h, sim: sim, cluster: cl}
}

// adoptedMachine imports the cluster and returns the one machine's id.
func (h *inventoryHarness) adoptedMachine(t *testing.T) model.MachineID {
	t.Helper()

	h.adopt(t)

	resp, raw := h.do(t, http.MethodGet, "/api/v1/machines", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("machines: %d (%s)", resp.StatusCode, raw)
	}
	var list struct {
		Machines []struct {
			ID string `json:"id"`
		} `json:"machines"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode machines: %v", err)
	}
	if len(list.Machines) == 0 {
		t.Fatal("the adoption recorded no machines")
	}
	return model.MachineID(list.Machines[0].ID)
}

// confirm asks for a token the way the UI does.
func (h *inventoryHarness) confirm(t *testing.T, id model.MachineID, action string, params map[string]string, typed string) string {
	t.Helper()

	resp, raw := h.do(t, http.MethodPost, "/api/v1/machines/"+string(id)+"/confirm", map[string]any{
		"action": action,
		"params": params,
		"typed":  typed,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirm: %d (%s)", resp.StatusCode, raw)
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode confirmation: %v", err)
	}
	return body.Token
}

// TestADestructiveActionAnswers202WithAJobID is JOB-09.
//
// A 200 with a result would be claiming the reboot had happened, and it takes
// a minute. What the operator gets instead is an id and somewhere to watch it.
func TestADestructiveActionAnswers202WithAJobID(t *testing.T) {
	h := newJobHarness(t)
	id := h.adoptedMachine(t)

	// The cluster was adopted read-only, and a reboot is a mutation.
	h.unlockEverything(t)

	token := h.confirm(t, id, string(model.JobReboot), nil, "")

	resp, raw := h.do(t, http.MethodPost, "/api/v1/machines/"+string(id)+"/reboot", map[string]any{
		"confirmation": token,
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("reboot: %d (%s), want 202", resp.StatusCode, raw)
	}
	if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "/api/v1/jobs/") {
		t.Errorf("Location = %q, want a job URL", loc)
	}

	var body struct {
		Job struct {
			ID    string `json:"id"`
			Kind  string `json:"kind"`
			Steps []struct {
				Name       string `json:"name"`
				Verifiable bool   `json:"verifiable"`
			} `json:"steps"`
		} `json:"job"`
		Topic string `json:"topic"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v (%s)", err, raw)
	}
	if body.Job.ID == "" {
		t.Fatal("the response carries no job id")
	}
	if body.Topic == "" {
		t.Error("the response does not say where to watch the job")
	}
	if len(body.Job.Steps) == 0 {
		t.Fatal("the job has no steps")
	}
	// A reboot is checkable after the fact, which is what lets it be resumed
	// rather than parked.
	if !body.Job.Steps[len(body.Job.Steps)-1].Verifiable {
		t.Error("the reboot step is recorded as unverifiable")
	}

	// And the job is readable at its own URL.
	resp, raw = h.do(t, http.MethodGet, "/api/v1/jobs/"+body.Job.ID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get job: %d (%s)", resp.StatusCode, raw)
	}
}

// TestAConfirmationThatOnlyHappenedInTheBrowserIsRefused is JOB-08.
//
// A dialog protects against a misclick and against nothing else: anything that
// can reach the API can skip it. So the server issues the confirmation and
// checks it against what was actually submitted.
func TestAConfirmationThatOnlyHappenedInTheBrowserIsRefused(t *testing.T) {
	h := newJobHarness(t)
	id := h.adoptedMachine(t)
	h.unlockEverything(t)

	// No confirmation at all.
	resp, raw := h.do(t, http.MethodPost, "/api/v1/machines/"+string(id)+"/reboot", map[string]any{})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("an unconfirmed reboot: %d (%s), want 403", resp.StatusCode, raw)
	}
	p := decodeProblem(t, resp, raw)
	if p.Code != httpapi.CodeConfirmationInvalid {
		t.Errorf("code = %q, want %q", p.Code, httpapi.CodeConfirmationInvalid)
	}

	// A confirmation for a *different* action. This is the case that matters:
	// the operator read one dialog and the request does something else.
	hostname := h.hostnameOf(t, id)
	gentle := map[string]string{
		"wipe_mode": "user-disks", "graceful": "true", "reboot": "true", "user_disks": "sdb",
	}
	token := h.confirm(t, id, string(model.JobReset), gentle, hostname)

	resp, raw = h.do(t, http.MethodPost, "/api/v1/machines/"+string(id)+"/reset", map[string]any{
		"confirmation": token,
		"params": map[string]string{
			// Widened behind the operator's back.
			"wipe_mode": "all", "graceful": "false", "reboot": "false",
		},
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a widened reset: %d (%s), want 403 — the confirmation was for the user disks",
			resp.StatusCode, raw)
	}
}

// TestAResetRequiresTypingTheHostname is JOB-07's typing half, enforced where
// the token is issued rather than in the browser.
func TestAResetRequiresTypingTheHostname(t *testing.T) {
	h := newJobHarness(t)
	id := h.adoptedMachine(t)

	params := map[string]string{"wipe_mode": "system-disk", "graceful": "true", "reboot": "true"}

	resp, raw := h.do(t, http.MethodPost, "/api/v1/machines/"+string(id)+"/confirm", map[string]any{
		"action": string(model.JobReset),
		"params": params,
		"typed":  "not-the-hostname",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("confirming a reset with the wrong phrase: %d (%s), want 400", resp.StatusCode, raw)
	}
}

// TestTheResetPreviewShowsTheEffectiveFlags is the rest of JOB-07: the screen
// has to show what will happen before it happens, and it must not present
// talosctl's defaults as this product's.
func TestTheResetPreviewShowsTheEffectiveFlags(t *testing.T) {
	h := newJobHarness(t)
	id := h.adoptedMachine(t)

	resp, raw := h.do(t, http.MethodGet, "/api/v1/machines/"+string(id)+"/reset-preview", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset preview: %d (%s)", resp.StatusCode, raw)
	}

	var body struct {
		Hostname      string `json:"hostname"`
		ConfirmPhrase string `json:"confirm_phrase"`
		Modes         []struct {
			Mode        string `json:"mode"`
			Description string `json:"description"`
		} `json:"modes"`
		Defaults struct {
			Mode     string `json:"mode"`
			Graceful bool   `json:"graceful"`
			Reboot   bool   `json:"reboot"`
		} `json:"defaults"`
		TalosDefaultWarning string `json:"talos_default_warning"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode preview: %v (%s)", err, raw)
	}

	if body.ConfirmPhrase == "" || body.ConfirmPhrase != body.Hostname {
		t.Errorf("the phrase to type is %q and the hostname is %q; "+
			"it has to be the thing the operator can check against the machine",
			body.ConfirmPhrase, body.Hostname)
	}
	if len(body.Modes) != 3 {
		t.Fatalf("the preview offers %d wipe scopes, want 3", len(body.Modes))
	}
	if body.Modes[0].Mode != "user-disks" {
		t.Errorf("the first scope offered is %q; a list whose first option wipes the machine "+
			"is a list somebody will click through", body.Modes[0].Mode)
	}
	for i, m := range body.Modes {
		if m.Description == "" {
			t.Errorf("scope %d (%s) has no description", i, m.Mode)
		}
	}

	// The three pre-selections are each the opposite of what talosctl would do
	// unprompted, and that difference is stated on the screen rather than
	// assumed.
	if body.Defaults.Mode != "user-disks" || !body.Defaults.Graceful || !body.Defaults.Reboot {
		t.Errorf("the pre-selected flags are %+v, want the least destructive combination", body.Defaults)
	}
	if !strings.Contains(body.TalosDefaultWarning, "talosctl") {
		t.Error("the preview does not say how talosctl's own defaults differ")
	}
}

// TestOneMutatingJobPerClusterOverHTTP is JOB-03 at the boundary an operator
// meets it.
func TestOneMutatingJobPerClusterOverHTTP(t *testing.T) {
	h := newJobHarness(t)
	id := h.adoptedMachine(t)
	h.unlockEverything(t)

	token := h.confirm(t, id, string(model.JobReboot), nil, "")
	resp, raw := h.do(t, http.MethodPost, "/api/v1/machines/"+string(id)+"/reboot", map[string]any{
		"confirmation": token,
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("first reboot: %d (%s)", resp.StatusCode, raw)
	}

	// Immediately again. Either it is refused as busy, or the first has
	// already finished — and the simulator's reboot is fast, so the test
	// accepts both rather than racing.
	token = h.confirm(t, id, string(model.JobReboot), nil, "")
	resp, raw = h.do(t, http.MethodPost, "/api/v1/machines/"+string(id)+"/reboot", map[string]any{
		"confirmation": token,
	})
	switch resp.StatusCode {
	case http.StatusConflict:
		p := decodeProblem(t, resp, raw)
		if p.Code != httpapi.CodeClusterBusy {
			t.Errorf("code = %q, want %q", p.Code, httpapi.CodeClusterBusy)
		}
	case http.StatusAccepted:
		// The first finished first. Nothing to assert.
	default:
		t.Fatalf("second reboot: %d (%s)", resp.StatusCode, raw)
	}
}

// unlockEverything opens every cluster's lock, because an imported cluster is
// adopted read-only and a node action is a mutation.
func (h *inventoryHarness) unlockEverything(t *testing.T) {
	t.Helper()

	resp, raw := h.do(t, http.MethodGet, "/api/v1/clusters", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clusters: %d (%s)", resp.StatusCode, raw)
	}
	var list struct {
		Clusters []struct {
			ID string `json:"id"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode clusters: %v", err)
	}
	for _, c := range list.Clusters {
		resp, raw := h.do(t, http.MethodPost, "/api/v1/clusters/"+c.ID+"/lock",
			map[string]bool{"locked": false})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("unlock %s: %d (%s)", c.ID, resp.StatusCode, raw)
		}
	}
}

func (h *inventoryHarness) hostnameOf(t *testing.T, id model.MachineID) string {
	t.Helper()

	resp, raw := h.do(t, http.MethodGet, "/api/v1/machines/"+string(id), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("machine: %d (%s)", resp.StatusCode, raw)
	}
	var body struct {
		Hostname struct {
			Value string `json:"value"`
		} `json:"hostname"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode machine: %v", err)
	}
	return body.Hostname.Value
}

var _ = time.Second

// TestRemovingANodeFromItsClusterRequiresTypingTheHostname is the gap the
// confirmation table closed.
//
// The rule used to be `if action == node.reset`, written when reset was the
// only confirmable action that destroyed anything. Phase 9 added
// node.remove-from-cluster -- the node leaves etcd, forfeits leadership first
// if it holds it, and its record here is forgotten -- and it inherited "no
// typing needed" by not being mentioned. The browser asked for the hostname.
// The server handed a token to anybody who asked without one, which made the
// dialog decoration.
func TestRemovingANodeFromItsClusterRequiresTypingTheHostname(t *testing.T) {
	h := newJobHarness(t)
	id := h.adoptedMachine(t)

	params := map[string]string{"cluster": h.adoptedCluster(t)}

	// Nothing typed at all: the shape a client that skipped the box produces.
	resp, raw := h.do(t, http.MethodPost, "/api/v1/machines/"+string(id)+"/confirm", map[string]any{
		"action": handlers.ActionRemoveFromCluster,
		"params": params,
		"typed":  "",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("confirming a removal with nothing typed: %d (%s), want 400. A confirmation only "+
			"the browser enforces is not a confirmation", resp.StatusCode, raw)
	}

	// And a near miss, because that is what a paste of the wrong hostname
	// looks like.
	resp, raw = h.do(t, http.MethodPost, "/api/v1/machines/"+string(id)+"/confirm", map[string]any{
		"action": handlers.ActionRemoveFromCluster,
		"params": params,
		"typed":  "cp-2",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("confirming a removal with the wrong hostname: %d (%s), want 400",
			resp.StatusCode, raw)
	}

	// The right one is accepted, so the test above is not passing because the
	// route refuses everything.
	if token := h.confirm(t, id, handlers.ActionRemoveFromCluster, params, "cp-1"); token == "" {
		t.Fatal("a correctly typed removal produced no token")
	}
}

// TestAnActionNobodyHasDecidedAboutGetsNoToken is the default this route used
// to have.
//
// Every action fell through to "no typing needed" unless it was reset, so a
// destructive action added later was confirmed by whatever its client chose to
// ask for. A refusal is the safe direction and, unlike a silent default, it is
// visible on the day somebody adds the fifth action.
func TestAnActionNobodyHasDecidedAboutGetsNoToken(t *testing.T) {
	h := newJobHarness(t)
	id := h.adoptedMachine(t)

	resp, raw := h.do(t, http.MethodPost, "/api/v1/machines/"+string(id)+"/confirm", map[string]any{
		"action": "node.something-nobody-wrote-down",
		"params": map[string]string{},
		"typed":  "",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("confirming an unknown action: %d (%s), want 400", resp.StatusCode, raw)
	}
}

// adoptedCluster is the cluster the job harness adopted, read back the way a
// client reads it.
func (h *inventoryHarness) adoptedCluster(t *testing.T) string {
	t.Helper()

	resp, raw := h.do(t, http.MethodGet, "/api/v1/clusters", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clusters: %d (%s)", resp.StatusCode, raw)
	}
	var list struct {
		Clusters []struct {
			ID string `json:"id"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode clusters: %v", err)
	}
	if len(list.Clusters) == 0 {
		t.Fatal("the harness adopted no cluster")
	}
	return list.Clusters[0].ID
}
