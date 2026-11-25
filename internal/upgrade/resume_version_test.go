package upgrade_test

import (
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// A rolling upgrade resumed after a restart reads what the node runs before
// it does anything. Happened says "done" only for a node on the target with
// every service running, so "not done" also covers a node on the target whose
// kubelet is still starting, and a node somebody moved past the target while
// the job sat unfinished (a power cut, a data directory restored from a
// backup). Both used to be installed again: the first reinstalled and
// rebooted, the second downgraded, unconfirmed.
//
// Sequential, like the gate's tests: FastPolling changes a package variable.

// stageUpgrade writes the record a crash leaves inside the last step of a
// job: every step before it done, it running.
func (r *removalRig) stageUpgrade(t *testing.T, kind model.JobKind, req upgrade.Request, names []string, verifiable map[string]bool) model.JobID {
	t.Helper()
	j := model.Job{
		ID: "staged-upgrade", Kind: kind, Cluster: removeCluster,
		State: model.JobRunning, Current: len(names) - 1, Params: req.Params(),
		CreatedAt: time.Now().Add(-time.Minute), StartedAt: time.Now().Add(-time.Minute),
	}
	for i, name := range names {
		s := model.JobStep{Name: name, Verifiable: verifiable[name], State: model.StepDone}
		if i == len(names)-1 {
			s.State = model.StepRunning
			s.StartedAt = time.Now().Add(-10 * time.Second)
		}
		j.Steps = append(j.Steps, s)
	}
	if _, err := r.st.Jobs().Put(t.Context(), j); err != nil {
		t.Fatal(err)
	}
	return j.ID
}

func (r *removalRig) stageTalosUpgradeOf(t *testing.T, host string) (model.JobID, string) {
	t.Helper()
	m := r.machine(host)
	req := upgrade.Request{
		To: gateTarget, Machines: []model.MachineID{m.ID},
		Installers: map[model.MachineID]string{m.ID: "ghcr.io/siderolabs/installer:" + gateTarget},
	}
	gate := gateStepPrefix + string(m.ID)
	up := upgradeStepPrefix + string(m.ID) + " to " + gateTarget
	return r.stageUpgrade(t, upgrade.JobKindTalosUpgrade, req, []string{gate, up},
		map[string]bool{gate: true, up: true}), up
}

func (r *removalRig) resume(t *testing.T) {
	t.Helper()
	e := r.newEngine()
	if err := e.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	r.engine = e
}

// TestAResumedUpgradeNeverDowngradesANodeMovedPastTheTarget: w-1 runs v1.14.3
// when the job, confirmed for v1.14.0, is resumed inside its step.
func TestAResumedUpgradeNeverDowngradesANodeMovedPastTheTarget(t *testing.T) {
	r := newRemovalRig(t, 1, 1)
	upgrade.FastPolling(t)
	r.sim("w-1").SetVersion("v1.14.3")

	id, up := r.stageTalosUpgradeOf(t, "w-1")
	r.resume(t)
	j := r.await(r.engine, id, model.JobFailed)

	if n := r.sim("w-1").Calls("Upgrade"); n != 0 {
		t.Errorf("Upgrade was called %d times on a node already past the target: a downgrade", n)
	}
	if n := r.sim("w-1").Calls("ImagePull"); n != 0 {
		t.Errorf("ImagePull was called %d times on a node that is not to be touched", n)
	}
	detail := stepNamed(j, up).Detail
	for _, want := range []string{"v1.14.3", "newer", "not downgraded"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the failure %q does not say %q", detail, want)
		}
	}
}

// TestAResumedUpgradeDoesNotReinstallANodeAlreadyOnTheTarget: w-1 is on
// v1.14.0 and its kubelet has not come up yet when the job is resumed. The
// step waits for it rather than installing and rebooting the node again.
func TestAResumedUpgradeDoesNotReinstallANodeAlreadyOnTheTarget(t *testing.T) {
	r := newRemovalRig(t, 1, 1)
	upgrade.FastPolling(t)
	sim := r.sim("w-1")
	sim.SetVersion(gateTarget)
	restore, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioK8sDown})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		restore()
	}()

	id, up := r.stageTalosUpgradeOf(t, "w-1")
	r.resume(t)
	j := r.await(r.engine, id, model.JobSucceeded)

	if n := sim.Calls("Upgrade"); n != 0 {
		t.Errorf("Upgrade was called %d times on a node already on the target: a second "+
			"install and reboot for a kubelet that was still starting", n)
	}
	if d := stepNamed(j, up).Detail; !strings.Contains(d, "nothing was installed") {
		t.Errorf("the step's detail %q does not say nothing was installed", d)
	}
}

// TestAResumedKubernetesUpgradeNeverMovesAKubeletBack: w-1's kubelet was moved
// to a newer Kubernetes by hand while the run, confirmed for an older one, sat
// inside its last step.
func TestAResumedKubernetesUpgradeNeverMovesAKubeletBack(t *testing.T) {
	r := newRemovalRig(t, 1, 1)
	upgrade.FastPolling(t)
	m := r.machine("w-1")

	to, err := upgrade.ParseVersion("v1.35.2")
	if err != nil {
		t.Fatal(err)
	}
	newer, err := upgrade.ParseVersion("v1.36.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := upgrade.ApplyKubeletImage(t.Context(), r.deps, m.ID, newer); err != nil {
		t.Fatalf("moving the kubelet by hand: %v", err)
	}

	req := upgrade.Request{To: to.String(), Machines: []model.MachineID{m.ID}}
	kubelet := "move " + string(m.ID) + "'s kubelet to Kubernetes " + to.String()
	names := []string{
		"pull the Kubernetes " + to.String() + " images onto every node",
		"move kube-apiserver to " + to.String() + " on every control-plane node",
		"move kube-controller-manager to " + to.String() + " on every control-plane node",
		"move kube-scheduler to " + to.String() + " on every control-plane node",
		"move kube-proxy to " + to.String(),
		kubelet,
	}
	id := r.stageUpgrade(t, upgrade.JobKindKubernetesUpgrade, req, names, map[string]bool{kubelet: true})
	r.resume(t)
	j := r.await(r.engine, id, model.JobFailed)

	detail := stepNamed(j, kubelet).Detail
	for _, want := range []string{"v1.36.0", "newer", "not moved back"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the failure %q does not say %q", detail, want)
		}
	}
	got, err := r.kubeletVersion(t, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != "v1.36.0" {
		t.Errorf("w-1's kubelet is %s after the resume, want it left on v1.36.0", got)
	}
}

func (r *removalRig) kubeletVersion(t *testing.T, id model.MachineID) (string, error) {
	t.Helper()
	cc, err := r.deps.Connect(t.Context(), id)
	if err != nil {
		return "", err
	}
	defer cc.Close() //nolint:errcheck // a test read
	ctx, cancel, err := talos.WithClassDeadline(t.Context(), talos.MethodCOSIList)
	if err != nil {
		return "", err
	}
	defer cancel()
	facts, err := cc.NodeFacts(ctx)
	if err != nil {
		return "", err
	}
	return facts.KubernetesVersion(), nil
}
