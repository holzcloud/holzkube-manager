package upgrade_test

import (
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// TestARestartInsideTheKubeProxyStepMovesItAgainInsteadOfParking: setting the
// same image in the configuration and in the DaemonSet again changes nothing,
// so an interrupted kube-proxy step is repeated, as the prepull is.
func TestARestartInsideTheKubeProxyStepMovesItAgainInsteadOfParking(t *testing.T) {
	r := newRemovalRig(t, 1, 0)
	withGate(r)
	upgrade.FastPolling(t)
	r.engine = r.newEngine()
	m := r.machine("cp-1")

	// What a submission records for the step, read from a real one.
	const first = "v1.35.2"
	req := upgrade.Request{To: first, Machines: []model.MachineID{m.ID}}
	submitted, err := r.engine.Submit(t.Context(), model.Job{
		Kind: upgrade.JobKindKubernetesUpgrade, Cluster: removeCluster, Params: req.Params(), Actor: "test",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	r.await(r.engine, submitted.ID, model.JobSucceeded)

	const to = "v1.35.3"
	names := kubernetesStepNames(to, m.ID)
	verifiable := map[string]bool{}
	for i, name := range names {
		verifiable[name] = submitted.Steps[i].Verifiable
	}
	req.To = to
	id := r.stageUpgradeAt(t, upgrade.JobKindKubernetesUpgrade, req, names, verifiable, 4)

	r.resume(t)
	j := r.await(r.engine, id, model.JobSucceeded)

	if st := stepNamed(j, names[4]).State; st != model.StepDone {
		t.Errorf("the kube-proxy step is %s, want done: it ran again", st)
	}
}
