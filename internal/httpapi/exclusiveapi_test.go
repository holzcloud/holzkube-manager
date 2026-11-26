package httpapi_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The routes that act on a cluster inside their own request -- a
// configuration apply, an etcd restore, an etcd member removal -- used to run
// beside the jobs engine unseen. A Talos upgrade with one control-plane node
// rebooting, and an apply in reboot mode on a second, was two of three down.

const blockingKind model.JobKind = "test.blocking"

// occupy submits a job on cluster that runs until the test ends, the way an
// upgrade walking its nodes holds the cluster.
func occupy(t *testing.T, e *jobs.Engine, cluster model.ClusterID) {
	t.Helper()
	release := make(chan struct{})
	e.Register(blockingKind, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{Name: "hold", Do: func(context.Context, *model.Job) error {
			<-release
			return nil
		}}}, nil
	})
	if _, err := e.Submit(t.Context(), model.Job{Kind: blockingKind, Cluster: cluster}); err != nil {
		t.Fatalf("submit the occupying job: %v", err)
	}
	t.Cleanup(func() { close(release) })
}

func newExclusiveHarness(t *testing.T) (*upgradeHarness, model.ClusterID) {
	t.Helper()
	h := newUpgradeHarnessOptions(t, upgradeHarnessOptions{Config: true})
	h.openSudo(t)
	h.unlockEverything(t)
	return h, h.clusterID(t)
}

func wantCode(t *testing.T, what string, resp *http.Response, raw []byte, status int, code string) {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("%s: %d (%s), want %d", what, resp.StatusCode, raw, status)
	}
	if p := decodeProblem(t, resp, raw); p.Code != code {
		t.Fatalf("%s: code = %q, want %q", what, p.Code, code)
	}
}

// TestTheRoutesThatActInTheirRequestAreRefusedWhileAJobHoldsTheCluster: the
// same 409 a second job gets.
func TestTheRoutesThatActInTheirRequestAreRefusedWhileAJobHoldsTheCluster(t *testing.T) {
	h, cluster := newExclusiveHarness(t)
	digest := h.planDigest(t, h.machine, certSANPatch)
	occupy(t, h.jobs, cluster)

	resp, raw := h.applyBody(t, h.machine, map[string]any{
		"patches": []string{certSANPatch}, "mode": "no-reboot", "base_digest": digest,
	})
	wantCode(t, "a config apply while a job holds the cluster", resp, raw,
		http.StatusConflict, httpapi.CodeClusterBusy)

	resp, raw = h.do(t, http.MethodPost,
		"/api/v1/machines/"+string(h.machine)+"/etcd/restore?machine="+string(h.machine), map[string]any{})
	wantCode(t, "an etcd restore while a job holds the cluster", resp, raw,
		http.StatusConflict, httpapi.CodeClusterBusy)

	resp, raw = h.do(t, http.MethodDelete, "/api/v1/clusters/"+string(cluster)+"/etcd/members/x", map[string]any{})
	wantCode(t, "an etcd member removal while a job holds the cluster", resp, raw,
		http.StatusConflict, httpapi.CodeClusterBusy)
}

// TestAConfigApplyHoldsTheClusterWhileItRuns: the apply takes its cluster's lease
// through a hold job on the record, and leaves the cluster free when it is done.
// That a job is refused while the hold exists is TestAHoldRefusesAJob (handlers).
func TestAConfigApplyHoldsTheClusterWhileItRuns(t *testing.T) {
	h, cluster := newExclusiveHarness(t)
	digest := h.planDigest(t, h.machine, certSANPatch)

	h.jobs.Register(blockingKind, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{Name: "noop", Do: func(context.Context, *model.Job) error { return nil }}}, nil
	})

	resp, raw := h.applyBody(t, h.machine, map[string]any{
		"patches": []string{certSANPatch}, "mode": "no-reboot", "base_digest": digest,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d (%s)", resp.StatusCode, raw)
	}

	var seenHold bool
	list, err := h.jobs.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range list {
		if j.Kind == "cluster.exclusive" && j.Cluster == cluster && j.Machine == h.machine {
			seenHold = true
		}
	}
	if !seenHold {
		t.Fatal("the apply took no hold on its cluster")
	}

	// And the cluster is free the moment the apply answered -- not a moment
	// later. A client that applies and then starts a job, or applies again,
	// must not be told the cluster is busy by its own finished request. This
	// polled for five seconds until the release waited for the hold job's end,
	// and TestARebootApplyOnAControlPlaneNodeAsksTheEtcdGate, which sends two
	// applies back to back, failed on "store.cluster-busy" now and then.
	if _, err := h.jobs.Submit(t.Context(), model.Job{Kind: blockingKind, Cluster: cluster}); err != nil {
		t.Fatalf("the cluster is still held after the apply answered: %v", err)
	}
}

// TestARebootApplyOnAControlPlaneNodeAsksTheEtcdGate: an apply that reboots a
// control-plane node takes an etcd member down, as an upgrade step does, and
// it asks the same gate first. One that does not reboot does not ask.
func TestARebootApplyOnAControlPlaneNodeAsksTheEtcdGate(t *testing.T) {
	h, _ := newExclusiveHarness(t)
	digest := h.planDigest(t, h.machine, certSANPatch)

	// A cluster of one is the gate's deliberate exception, so a second
	// control-plane record makes it a cluster the gate has an opinion about,
	// and the alarm is the opinion: etcd in NOSPACE spares nobody.
	rec, err := h.store.Machines().Get(t.Context(), h.machine)
	if err != nil {
		t.Fatalf("read the machine: %v", err)
	}
	rec.ID, rec.Rev = "m-second-control-plane", 0
	if _, err := h.store.Machines().Put(t.Context(), rec); err != nil {
		t.Fatalf("add a second control-plane record: %v", err)
	}
	h.sim.SetEtcdAlarm("NOSPACE")

	for _, mode := range []string{"reboot", "auto"} {
		resp, raw := h.applyBody(t, h.machine, map[string]any{
			"patches": []string{certSANPatch}, "mode": mode, "base_digest": digest,
		})
		wantCode(t, "a "+mode+" apply on a control-plane node with etcd in alarm", resp, raw,
			http.StatusConflict, httpapi.CodeEtcdGateRefused)
	}

	resp, raw := h.applyBody(t, h.machine, map[string]any{
		"patches": []string{certSANPatch}, "mode": "no-reboot", "base_digest": digest,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("a no-reboot apply with etcd in alarm: %d (%s), want 200", resp.StatusCode, raw)
	}
}
