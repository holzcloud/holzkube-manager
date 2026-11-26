package upgrade_test

import (
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

func kubernetesStepNames(to string, id model.MachineID) []string {
	return []string{
		"pull the Kubernetes " + to + " images onto every node",
		"move kube-apiserver to " + to + " on every control-plane node",
		"move kube-controller-manager to " + to + " on every control-plane node",
		"move kube-scheduler to " + to + " on every control-plane node",
		"move kube-proxy to " + to,
		"move " + string(id) + "'s kubelet to Kubernetes " + to,
	}
}

// TestARestartInsideThePrepullPullsAgainInsteadOfParking: pulling an image
// that is already there is harmless, which is what the step's comment always
// said -- "a resumed run simply does it again". With no Happened it was
// unverifiable, and a Kubernetes upgrade interrupted in its first, harmless
// step was parked for a person.
func TestARestartInsideThePrepullPullsAgainInsteadOfParking(t *testing.T) {
	r := newRemovalRig(t, 1, 1)
	upgrade.FastPolling(t)
	m := r.machine("w-1")

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
	names := kubernetesStepNames(first, m.ID)
	for i, s := range submitted.Steps {
		if s.Name != names[i] {
			t.Fatalf("step %d is %q, want %q", i, s.Name, names[i])
		}
	}

	// The crash, inside the prepull of the next run.
	const to = "v1.35.3"
	verifiable := map[string]bool{}
	for i, name := range kubernetesStepNames(to, m.ID) {
		verifiable[name] = submitted.Steps[i].Verifiable
	}
	pullsBefore := r.sim("w-1").Calls("ImagePull")
	req.To = to
	id := r.stageUpgradeAt(t, upgrade.JobKindKubernetesUpgrade, req,
		kubernetesStepNames(to, m.ID), verifiable, 0)

	r.resume(t)
	r.await(r.engine, id, model.JobSucceeded)

	if r.sim("w-1").Calls("ImagePull") <= pullsBefore {
		t.Error("the resumed prepull pulled nothing")
	}
	got, err := r.kubeletVersion(t, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != to {
		t.Errorf("w-1's kubelet is %s after the resumed run, want %s", got, to)
	}
}
