package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"log/slog"
	"os"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// The upgrade surface through the real router.
//
// What these are about is the shape of the answers, not the rolling upgrade
// itself: a plan that is blocked says why on the screen, a member list names
// hostnames, and the submission needs the same three gates every other
// destructive route needs.

type upgradeHarness struct {
	*inventoryHarness
	machine model.MachineID
}

func newUpgradeHarness(t *testing.T) *upgradeHarness {
	t.Helper()
	return newUpgradeHarnessWith(t, false)
}

// newUpgradeHarnessWith is newUpgradeHarness with the node's SecureBoot state
// chosen, because the installer an upgrade writes depends on it.
func newUpgradeHarnessWith(t *testing.T, secureBoot bool) *upgradeHarness {
	t.Helper()

	cl, err := talossim.NewCluster("homelab", "https://192.168.1.41:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	sim, err := talossim.New(talossim.Options{
		Hostname: "cp-1", Cluster: cl, ControlPlane: true, SecureBoot: secureBoot,
	})
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
		// Built inside newHarness, where the inventory already exists: the
		// upgrade service closes over it, and Deps is copied by value into
		// every route closure, so a service assigned afterwards would be nil
		// in every handler with no compile error.
		withUpgrade(func(h *harness) *upgrade.Service {
			deps := upgrade.Deps{
				Connect: func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
					return h.inv.Connect(ctx, id)
				},
				Machines: h.inv.MachinesOf,
				Record: func(ctx context.Context, id model.MachineID) error {
					h.inv.Refresh(ctx, id)
					return nil
				},

				// A stand-in for the Factory resolution. It carries SecureBoot
				// into the name, which is the property these tests care about;
				// whether the real resolver picks the right repository is
				// internal/imagefactory's own test.
				ResolveInstaller: func(_ context.Context, schematicID, version string, secureBoot bool) (string, error) {
					repo := "metal-installer"
					if secureBoot {
						repo += "-secureboot"
					}
					return "factory.example/" + repo + "/" + schematicID + ":" + version, nil
				},
			}
			deps.Gate = upgrade.NewGate(deps.Connect, h.inv.ControlPlanesOf)

			return upgrade.NewService(deps, func(context.Context) ([]string, error) {
				return []string{"v1.13.9", "v1.14.0", "v1.14.1", "v1.15.0-rc.1"}, nil
			})
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

	ih := &inventoryHarness{harness: h, sim: sim, cluster: cl}
	uh := &upgradeHarness{inventoryHarness: ih, machine: ih.adoptedMachine(t)}

	// etcd has to be running for the health gate and the member list to have
	// anything to read. A node that was never bootstrapped refuses every Etcd*
	// RPC, which is a different test.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cc, err := uh.inv.Connect(ctx, uh.machine)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer cc.Close() //nolint:errcheck // the fixture's verdict is the test's

	bootstrapCtx, cancelBootstrap, err := talos.WithClassDeadline(ctx, talos.MethodBootstrap)
	if err != nil {
		t.Fatalf("WithClassDeadline: %v", err)
	}
	defer cancelBootstrap()
	if err := cc.Bootstrap(bootstrapCtx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	return uh
}

// clusterID is the cluster the harness adopted.
func (h *upgradeHarness) clusterID(t *testing.T) model.ClusterID {
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
		t.Fatal("no cluster was adopted")
	}
	return model.ClusterID(list.Clusters[0].ID)
}

// TestTheReleaseListHasNoLatestAndNoPreReleases is UPG-05 at the API edge.
func TestTheReleaseListHasNoLatestAndNoPreReleases(t *testing.T) {
	t.Parallel()

	h := newUpgradeHarness(t)

	resp, raw := h.do(t, http.MethodGet, "/api/v1/upgrade/releases", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("releases: %d (%s)", resp.StatusCode, raw)
	}

	var body struct {
		Releases []string `json:"releases"`
		Notice   string   `json:"notice"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for _, r := range body.Releases {
		if strings.Contains(r, "rc") || strings.Contains(r, "beta") || strings.Contains(r, "alpha") {
			t.Errorf("the release list offers the pre-release %s", r)
		}
		if r == "latest" {
			t.Error("the release list offers 'latest'")
		}
	}
	if len(body.Releases) == 0 {
		t.Fatal("the release list is empty; the filter dropped everything")
	}
	if !strings.Contains(body.Notice, "latest") {
		t.Errorf("the notice does not explain why there is no latest: %q", body.Notice)
	}
}

// TestAPlanSaysWhatItWouldDoToEveryNode.
//
// A plan that only said "upgrade the cluster" would be a plan whose refusals
// arrive one at a time, minutes apart, in the middle of a rolling operation.
func TestAPlanSaysWhatItWouldDoToEveryNode(t *testing.T) {
	t.Parallel()

	h := newUpgradeHarness(t)

	resp, raw := h.do(t, http.MethodPost,
		"/api/v1/clusters/"+string(h.clusterID(t))+"/upgrade/plan",
		map[string]any{"to": "v1.14.0"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("plan: %d (%s)", resp.StatusCode, raw)
	}

	var plan upgrade.Plan
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}

	if len(plan.Nodes) == 0 {
		t.Fatal("the plan walks no nodes")
	}
	for _, n := range plan.Nodes {
		if n.Skipped || n.Blocked {
			continue
		}
		// PROV-08's sibling: the installer this node's upgrade will use, shown
		// per node, because two nodes in one cluster can have been built from
		// different schematics.
		if n.Installer == "" {
			t.Errorf("%s has no installer reference in the plan", n.Machine)
		}
		if n.SchematicSentence == "" {
			t.Errorf("%s carries no statement about its schematic (UPG-03)", n.Machine)
		}
	}

	// UPG-02's preview. The field name says it is a preview, because the gate
	// that decides runs inside each node's step.
	if plan.Gate.Input.Voting == 0 && plan.Gate.Reason == "" {
		t.Error("the plan carries neither a gate verdict nor a reason")
	}

	// UPG-06, evaluated against the version this run installs.
	if plan.Strand.Sentence == "" {
		t.Error("the plan carries no compatibility statement")
	}
}

// TestAPlanThatWouldStrandTheClusterIsBlockedOnTheScreen is UPG-06.
//
// Blocked, and on the screen -- not discovered by a job three nodes in.
func TestAPlanThatWouldStrandTheClusterIsBlockedOnTheScreen(t *testing.T) {
	t.Parallel()

	h := newUpgradeHarness(t)

	// The simulated cluster runs a recent Kubernetes, so upgrading Talos
	// *down* the compatibility table is what strands it. v1.12 supports
	// Kubernetes up to 1.33 and the simulator reports 1.34.
	resp, raw := h.do(t, http.MethodPost,
		"/api/v1/clusters/"+string(h.clusterID(t))+"/upgrade/plan",
		map[string]any{"to": "v1.12.3"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("plan: %d (%s)", resp.StatusCode, raw)
	}

	var plan upgrade.Plan
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if !plan.Blocked {
		t.Fatalf("a plan that would take Kubernetes out of support was not blocked: %+v", plan.Strand)
	}
	if plan.BlockReason == "" {
		t.Error("the block carries no reason")
	}
}

// TestSubmittingABlockedUpgradeIsRefused pins that the screen's block is not
// advisory: the server rebuilds the plan and refuses.
func TestSubmittingABlockedUpgradeIsRefused(t *testing.T) {
	t.Parallel()

	h := newUpgradeHarness(t)

	resp, raw := h.do(t, http.MethodPost, "/api/v1/auth/sudo", map[string]string{"password": testPass})
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
	}

	// An imported cluster is adopted read-only (INV-12), so the lock refuses
	// this before the upgrade's own checks get a look. Opening it is what an
	// operator does, and doing it here is what makes this test about the
	// upgrade's refusal rather than about the lock's.
	cluster := h.clusterID(t)
	resp, raw = h.do(t, http.MethodPost, "/api/v1/clusters/"+string(cluster)+"/lock",
		map[string]any{"locked": false})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unlock: %d (%s)", resp.StatusCode, raw)
	}

	resp, raw = h.do(t, http.MethodPost,
		"/api/v1/clusters/"+string(cluster)+"/upgrade",
		map[string]any{"to": "v1.12.3", "confirmation": "anything"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("a blocked upgrade answered %d (%s)", resp.StatusCode, raw)
	}
	if !strings.Contains(string(raw), "upgrade-blocked") {
		t.Errorf("the refusal does not carry the upgrade-blocked code: %s", raw)
	}
}

// TestTheMemberListNamesHostnames is UPG-10 at the API edge.
func TestTheMemberListNamesHostnames(t *testing.T) {
	t.Parallel()

	h := newUpgradeHarness(t)

	resp, raw := h.do(t, http.MethodGet,
		"/api/v1/clusters/"+string(h.clusterID(t))+"/etcd/members", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("members: %d (%s)", resp.StatusCode, raw)
	}

	var list upgrade.MemberList
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list.Members) == 0 {
		t.Fatal("the membership is empty")
	}
	for _, m := range list.Members {
		if !strings.Contains(m.Name, "cp-1") {
			t.Errorf("member %q is not named by hostname", m.Name)
		}
	}
	if list.Sentence == "" {
		t.Error("the list carries no statement of what its member count means")
	}
}

// TestLockingANodeNeedsAReasonAndSurvivesTheNodeBeingDown is UPG-14.
//
// The lock is holzkube-manager's own note about what it should not do. It has
// to be writable on a node that is not answering, because that is precisely
// when somebody wants to set one.
func TestLockingANodeNeedsAReasonAndSurvivesTheNodeBeingDown(t *testing.T) {
	t.Parallel()

	h := newUpgradeHarness(t)

	// No reason: refused, because a lock nobody can explain is a lock the next
	// person clears because it is in the way.
	resp, raw := h.do(t, http.MethodPost,
		"/api/v1/machines/"+string(h.machine)+"/lock", map[string]any{"locked": true})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a lock with no reason answered %d (%s)", resp.StatusCode, raw)
	}

	// No verdict at all: also refused. A missing field is not "unlock".
	resp, raw = h.do(t, http.MethodPost,
		"/api/v1/machines/"+string(h.machine)+"/lock", map[string]any{"reason": "why"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a lock with no verdict answered %d (%s)", resp.StatusCode, raw)
	}

	// The node is closed, so it is not answering at all -- and the lock still
	// works, which is the point.
	_ = h.sim.Close()

	resp, raw = h.do(t, http.MethodPost,
		"/api/v1/machines/"+string(h.machine)+"/lock",
		map[string]any{"locked": true, "reason": "the database on this one moves first"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("locking a node that is down answered %d (%s)", resp.StatusCode, raw)
	}

	var view struct {
		Locked     bool   `json:"locked"`
		LockReason string `json:"lock_reason"`
	}
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !view.Locked || view.LockReason == "" {
		t.Fatalf("the lock did not take: %+v", view)
	}
}

// TestUpgradingASecureBootNodeDoesNotTakeSecureBootAway.
//
// The defect this replaced: the installer reference an upgrade wrote was
// assembled as "factory.talos.dev/installer/<id>:<version>" for every node,
// with no SecureBoot in it. The ordinary installer does not produce a
// SecureBoot node, so upgrading one with it took SecureBoot away from a
// machine that had it -- on a path an operator runs against a cluster they
// depend on, and with nothing in the result saying so. A fresh provision that
// dropped SecureBoot produced a node that never had it; this took it from a
// node that did.
//
// The plan now reads how each node actually booted, from the node, and carries
// the resolved reference into the job.
func TestUpgradingASecureBootNodeDoesNotTakeSecureBootAway(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		secureBoot bool
	}{
		{"a SecureBoot node", true},
		{"an ordinary node", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newUpgradeHarnessWith(t, tc.secureBoot)

			resp, raw := h.do(t, http.MethodPost,
				"/api/v1/clusters/"+string(h.clusterID(t))+"/upgrade/plan",
				map[string]any{"to": "v1.14.0"})
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("plan: %d (%s)", resp.StatusCode, raw)
			}

			var plan struct {
				Nodes []struct {
					Installer  string `json:"installer"`
					SecureBoot bool   `json:"secureboot"`
					Blocked    bool   `json:"blocked"`
					Reason     string `json:"block_reason"`
				} `json:"nodes"`
			}
			if err := json.Unmarshal(raw, &plan); err != nil {
				t.Fatalf("decode plan: %v", err)
			}
			if len(plan.Nodes) == 0 {
				t.Fatalf("the plan names no nodes: %s", raw)
			}

			for _, n := range plan.Nodes {
				if n.Blocked {
					t.Fatalf("the node is blocked: %s", n.Reason)
				}
				if n.SecureBoot != tc.secureBoot {
					t.Errorf("the plan reports secureboot=%v for a node that booted %v",
						n.SecureBoot, tc.secureBoot)
				}
				if strings.Contains(n.Installer, "secureboot") != tc.secureBoot {
					t.Errorf("a node that booted secureboot=%v is upgraded with %q",
						tc.secureBoot, n.Installer)
				}
			}
		})
	}
}
