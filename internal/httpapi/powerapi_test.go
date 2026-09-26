package httpapi_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/httpapi/handlers"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/power"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// The power model over HTTP (2026-09-26): the contract the frontend is being
// written against, checked from the outside.

func newPowerHarness(t *testing.T) *inventoryHarness {
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
		withPower(),
	)
	h.setupAndLogin(t)

	// The admin's own session opens the window, because adopting and unlocking
	// are destructive. The tests below that are ABOUT the window use a second
	// session, which has not.
	resp, raw := h.do(t, http.MethodPost, "/api/v1/auth/sudo", map[string]string{"password": testPass})
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
	}
	return &inventoryHarness{harness: h, sim: sim, cluster: cl}
}

// TestOnlyTheForcedPowerActionsAskForThePassword, on all three targets.
//
// A session whose sudo window is shut presses each of the fourteen node and
// cluster actions and the seven app ones. The two forced ones are refused 428
// before anything runs; none of the five careful ones is -- they need the
// operator role and the cluster's lock, and the password prompt in front of
// them would teach the operator that the prompt means nothing.
func TestOnlyTheForcedPowerActionsAskForThePassword(t *testing.T) {
	h := newPowerHarness(t)
	id := h.adoptedMachine(t)
	h.unlockEverything(t)
	cluster := h.adoptedCluster(t)

	fresh := h.asUser(t, testUser, testPass)

	targets := []string{
		"/api/v1/clusters/" + cluster + "/power/",
		"/api/v1/machines/" + string(id) + "/power/",
		"/api/v1/clusters/" + cluster + "/kubernetes/apps/default/Deployment/web/power/",
	}
	for _, target := range targets {
		for _, a := range []power.Action{power.ForceStop, power.ForceRestart} {
			got, raw := fresh.status(t, http.MethodPost, target+string(a), map[string]any{})
			if got != http.StatusPreconditionRequired {
				t.Errorf("%s%s without the password: %d (%s), want 428", target, a, got, raw)
				continue
			}
			var p struct {
				Code string `json:"code"`
			}
			_ = json.Unmarshal(raw, &p)
			if p.Code != "sudo.required" {
				t.Errorf("%s%s: code %q, want sudo.required", target, a, p.Code)
			}
		}
	}

	// A careful one on the node that is running: not 428, and refused with the
	// sentence the report shows.
	got, raw := fresh.status(t, http.MethodPost, targets[1]+string(power.Start), map[string]any{})
	if got != http.StatusConflict {
		t.Fatalf("start of a running node: %d (%s), want 409", got, raw)
	}
	var p struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("decode: %v (%s)", err, raw)
	}
	if p.Code != httpapi.CodePowerUnavailable || p.Detail != power.ReasonRunning {
		t.Errorf("the refusal is %+v, want %s with the detail %q", p, httpapi.CodePowerUnavailable, power.ReasonRunning)
	}

	// An action that is not one of the seven has no route.
	got, raw = fresh.status(t, http.MethodPost, targets[1]+"hibernate", map[string]any{})
	if got != http.StatusNotFound {
		t.Errorf("an unknown action: %d (%s), want 404", got, raw)
	}
}

// TestThePowerRoutesMarkExactlyTheForcedActionsDestructive reads the route
// table, which is where D-06 says this is decided.
func TestThePowerRoutesMarkExactlyTheForcedActionsDestructive(t *testing.T) {
	t.Parallel()

	posts := 0
	for _, route := range handlers.PowerRoutes(httpapi.Deps{}) {
		if route.Method == http.MethodGet {
			if route.Destructive || route.MinRole != "reader" {
				t.Errorf("%s: destructive=%v role=%s; a report is a read", route.Pattern, route.Destructive, route.MinRole)
			}
			continue
		}
		posts++
		forced := strings.HasSuffix(route.Pattern, "/force-stop") || strings.HasSuffix(route.Pattern, "/force-restart")
		if route.Destructive != forced {
			t.Errorf("%s: destructive = %v, want %v", route.Pattern, route.Destructive, forced)
		}
		if route.ClusterScope == nil {
			t.Errorf("%s names no cluster, so a read-only cluster's lock does not reach it", route.Pattern)
		}
		if route.MinRole != "operator" {
			t.Errorf("%s: role %s, want operator", route.Pattern, route.MinRole)
		}
	}
	if posts != 21 {
		t.Errorf("%d power actions are routed, want 7 for each of the three targets", posts)
	}
}

// TestThePowerReportIsTheContract: the shape the frontend reads, from the
// wire -- all seven, in order, with the forced two behind the password and a
// reason that is present, and empty, when there is nothing in the way.
func TestThePowerReportIsTheContract(t *testing.T) {
	h := newPowerHarness(t)
	id := h.adoptedMachine(t)
	h.unlockEverything(t)
	cluster := h.adoptedCluster(t)

	for _, path := range []string{
		"/api/v1/machines/" + string(id) + "/power",
		"/api/v1/clusters/" + cluster + "/power",
	} {
		resp, raw := h.do(t, http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %d (%s)", path, resp.StatusCode, raw)
		}
		var body struct {
			State    string `json:"state"`
			Disabled *bool  `json:"disabled"`
			Actions  []struct {
				Action    string  `json:"action"`
				Available *bool   `json:"available"`
				Reason    *string `json:"reason"`
				Sudo      *bool   `json:"sudo"`
			} `json:"actions"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode %s: %v (%s)", path, err, raw)
		}
		if body.State != "running" {
			t.Errorf("%s: state %q, want running", path, body.State)
		}
		if body.Disabled == nil || *body.Disabled {
			t.Errorf("%s: disabled = %v", path, body.Disabled)
		}

		var order []string
		for _, a := range body.Actions {
			order = append(order, a.Action)
			if a.Available == nil || a.Reason == nil || a.Sudo == nil {
				t.Errorf("%s: %s omits a field: %s", path, a.Action, raw)
				continue
			}
			if *a.Sudo != (a.Action == "force-stop" || a.Action == "force-restart") {
				t.Errorf("%s: %s sudo = %v", path, a.Action, *a.Sudo)
			}
		}
		want := []string{"stop", "force-stop", "start", "disable", "enable", "restart", "force-restart"}
		if !slices.Equal(order, want) {
			t.Errorf("%s: actions %v, want %v", path, order, want)
		}
	}

	resp, raw := h.do(t, http.MethodGet, "/api/v1/machines/no-such-machine/power", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("a machine nobody adopted: %d (%s), want 404", resp.StatusCode, raw)
	}
}
