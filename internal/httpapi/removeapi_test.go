package httpapi_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// Removing a node from a cluster through the real router: a job, with the
// quorum rule still answered in the request.

type removalHarness struct {
	*inventoryHarness
	cp, worker       model.MachineID
	cpSim, workerSim *talossim.Server
}

func newRemovalHarness(t *testing.T) *removalHarness {
	t.Helper()

	cl, err := talossim.NewCluster("homelab", "https://192.168.1.41:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	cpSim, err := talossim.New(talossim.Options{
		Hostname: "cp-1", Cluster: cl, ControlPlane: true, Bootstrapped: true,
		Members: []talossim.MemberFixture{
			{ID: "cp-1", Hostname: "cp-1", ControlPlane: true, Addresses: []string{"127.0.0.1"}},
		},
	})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = cpSim.Close() })

	// The worker answers on another loopback address at the control plane's
	// port: the direct dialer knows one port and a member carries addresses
	// without one.
	workerSim, err := talossim.New(talossim.Options{
		Cluster: cl, Hostname: "w-1", Bootstrapped: true, NodeIP: "127.0.0.2",
		ListenAddr: net.JoinHostPort("127.0.0.2", strconv.Itoa(cpSim.Port())),
	})
	if err != nil {
		t.Skipf("a second loopback address is not available here: %v", err)
	}
	t.Cleanup(func() { _ = workerSim.Close() })

	if err := cpSim.AddMember(t.Context(), talossim.MemberFixture{
		ID: "w-1", Hostname: "w-1", Addresses: []string{"127.0.0.2"},
	}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	h := newHarness(t,
		withInventory(func(st *fsstore.Store) *inventory.Service {
			return inventory.New(inventory.Deps{
				Store:  st,
				Dialer: talos.NewDirectDialer(cpSim.Port()),
				Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
			})
		}),
		withJobs(),
		withUpgrade(func(h *harness) *upgrade.Service {
			deps := upgrade.Deps{
				Connect: func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
					return h.inv.Connect(ctx, id)
				},
				Machines: h.inv.MachinesOf,
				Forget:   h.inv.ForgetMachine,
				ResolveInstaller: func(context.Context, string, string, bool) (string, error) {
					return "", nil
				},
			}
			upgrade.Register(h.jobs, deps)
			return upgrade.NewService(deps, nil)
		}),
	)

	for _, step := range []struct {
		path string
		body map[string]string
		want int
	}{
		{"/api/v1/setup", map[string]string{"username": testUser, "password": testPass}, http.StatusCreated},
		{"/api/v1/auth/login", map[string]string{"username": testUser, "password": testPass}, http.StatusNoContent},
	} {
		if resp, raw := h.do(t, http.MethodPost, step.path, step.body); resp.StatusCode != step.want {
			t.Fatalf("%s: %d (%s)", step.path, resp.StatusCode, raw)
		}
	}
	resp, raw := h.do(t, http.MethodPost, "/api/v1/auth/sudo", map[string]string{"password": testPass})
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
	}

	ih := &inventoryHarness{harness: h, sim: cpSim, cluster: cl}
	ih.adopt(t)
	ih.unlockEverything(t)

	rh := &removalHarness{inventoryHarness: ih, cpSim: cpSim, workerSim: workerSim}
	rh.cp, rh.worker = rh.machineNamed(t, "cp-1"), rh.machineNamed(t, "w-1")
	return rh
}

func (h *removalHarness) machineNamed(t *testing.T, hostname string) model.MachineID {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	for {
		resp, raw := h.do(t, http.MethodGet, "/api/v1/machines", nil)
		var list struct {
			Machines []struct {
				ID       string `json:"id"`
				Hostname struct {
					Value string `json:"value"`
				} `json:"hostname"`
			} `json:"machines"`
		}
		if resp.StatusCode == http.StatusOK && json.Unmarshal(raw, &list) == nil {
			for _, m := range list.Machines {
				if m.Hostname.Value == hostname {
					return model.MachineID(m.ID)
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no machine named %s after 20s (%s)", hostname, raw)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// remove asks for a confirmation the way the UI does and submits the removal.
func (h *removalHarness) remove(t *testing.T, id model.MachineID, hostname, cluster string) (*http.Response, []byte) {
	t.Helper()
	token := h.confirm(t, id, handlersActionRemove, map[string]string{"cluster": cluster}, hostname)
	return h.do(t, http.MethodPost, "/api/v1/machines/"+string(id)+"/remove-from-cluster",
		map[string]any{"cluster": cluster, "confirmation": token})
}

const handlersActionRemove = "node.remove-from-cluster"

func (h *removalHarness) jobState(t *testing.T, id string) string {
	t.Helper()
	resp, raw := h.do(t, http.MethodGet, "/api/v1/jobs/"+id, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get job: %d (%s)", resp.StatusCode, raw)
	}
	var j struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(raw, &j); err != nil {
		t.Fatal(err)
	}
	return j.State
}

// TestRemovingANodeAnswers202WithAJobAndIsDoneWhenTheJobIs: the request only
// accepts the work. The wipe and the forget happen in the job.
func TestRemovingANodeAnswers202WithAJobAndIsDoneWhenTheJobIs(t *testing.T) {
	h := newRemovalHarness(t)
	cluster := h.adoptedCluster(t)

	resp, raw := h.remove(t, h.worker, "w-1", cluster)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("removing a worker: %d (%s), want 202", resp.StatusCode, raw)
	}
	var body struct {
		Job struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"job"`
		Topic  string `json:"topic"`
		Notice string `json:"notice"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v (%s)", err, raw)
	}
	if body.Job.Kind != "node.remove-from-cluster" || body.Job.ID == "" || body.Topic == "" {
		t.Errorf("the answer does not describe a job: %s", raw)
	}
	if loc := resp.Header.Get("Location"); loc != "/api/v1/jobs/"+body.Job.ID {
		t.Errorf("Location = %q, want the job's URL", loc)
	}
	if body.Notice == "" {
		t.Error("the cordon-and-drain notice is missing from the answer")
	}

	deadline := time.Now().Add(30 * time.Second)
	for h.jobState(t, body.Job.ID) != "succeeded" {
		if time.Now().After(deadline) {
			t.Fatalf("the removal job is %s after 30s", h.jobState(t, body.Job.ID))
		}
		time.Sleep(20 * time.Millisecond)
	}

	if got := h.workerSim.Node().Resets; got != 1 {
		t.Errorf("the worker was wiped %d times, want once", got)
	}
	if n := h.cpSim.Node(); n.EtcdLeaves != 0 || n.Resets != 0 {
		t.Errorf("the control plane was touched by a worker's removal: %+v", n)
	}
	resp, raw = h.do(t, http.MethodGet, "/api/v1/machines/"+string(h.worker), nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("the removed worker is still in the inventory: %d (%s)", resp.StatusCode, raw)
	}
}

// TestTheQuorumRefusalIsImmediateAndCreatesNoJob: the only control-plane node
// cannot be removed, and the answer is a 409 in the request -- not a job that
// fails a moment later.
func TestTheQuorumRefusalIsImmediateAndCreatesNoJob(t *testing.T) {
	h := newRemovalHarness(t)

	resp, raw := h.remove(t, h.cp, "cp-1", h.adoptedCluster(t))
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("removing the only control-plane node: %d (%s), want 409", resp.StatusCode, raw)
	}
	if p := decodeProblem(t, resp, raw); p.Code != httpapi.CodeLastVotingMember {
		t.Errorf("code = %q, want %q", p.Code, httpapi.CodeLastVotingMember)
	}

	resp, raw = h.do(t, http.MethodGet, "/api/v1/jobs", nil)
	var list struct {
		Jobs []json.RawMessage `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &list); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("jobs: %d (%s)", resp.StatusCode, raw)
	}
	if len(list.Jobs) != 0 {
		t.Errorf("a refused removal left %d job(s) behind: %s", len(list.Jobs), raw)
	}
	if n := h.cpSim.Node(); !n.Bootstrapped || n.EtcdLeaves != 0 || n.Resets != 0 {
		t.Errorf("the node was touched despite the refusal: %+v", n)
	}
}

// TestARemovalOnABusyClusterIsRefusedByTheLease is JOB-03 for this route: it
// used to run inside the request and take no lease, so it could run beside an
// upgrade.
func TestARemovalOnABusyClusterIsRefusedByTheLease(t *testing.T) {
	h := newRemovalHarness(t)
	cluster := h.adoptedCluster(t)

	release := make(chan struct{})
	entered := make(chan struct{})
	h.jobs.Register("test.hold", func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{
			Name: "hold the cluster",
			Do: func(ctx context.Context, _ *model.Job) error {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
				}
				return nil
			},
			Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
		}}, nil
	})
	if _, err := h.jobs.Submit(t.Context(), model.Job{
		Kind: "test.hold", Cluster: model.ClusterID(cluster),
	}); err != nil {
		t.Fatalf("Submit the holder: %v", err)
	}
	<-entered
	t.Cleanup(func() { close(release) })

	resp, raw := h.remove(t, h.worker, "w-1", cluster)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("a removal while another job holds the cluster: %d (%s), want 409", resp.StatusCode, raw)
	}
	if p := decodeProblem(t, resp, raw); p.Code != httpapi.CodeClusterBusy {
		t.Errorf("code = %q, want %q", p.Code, httpapi.CodeClusterBusy)
	}
	if got := h.workerSim.Node().Resets; got != 0 {
		t.Errorf("the refused removal wiped the worker %d times", got)
	}
}

// TestARemovalNamingAClusterTheMachineIsNotInIsRefused: the lock middleware checks the
// cluster in the body and the job leases the machine's own, so they have to be
// the same. Note what this test reaches: an unknown cluster is already refused
// by the lock middleware, so it does not by itself fail if the handler's own
// comparison goes (tried, and it stays green). That comparison is only
// reachable with a second real cluster in the inventory.
func TestARemovalNamingAClusterTheMachineIsNotInIsRefused(t *testing.T) {
	h := newRemovalHarness(t)

	token := h.confirm(t, h.worker, handlersActionRemove, map[string]string{"cluster": "some-other-cluster"}, "w-1")
	resp, raw := h.do(t, http.MethodPost, "/api/v1/machines/"+string(h.worker)+"/remove-from-cluster",
		map[string]any{"cluster": "some-other-cluster", "confirmation": token})
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("a removal naming a cluster the machine is not in: %d (%s), want 400", resp.StatusCode, raw)
	}
	if got := h.workerSim.Node().Resets; got != 0 {
		t.Errorf("the worker was wiped %d times", got)
	}
}
