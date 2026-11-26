package upgrade_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// TestAResumedKubernetesUpgradeNeverMovesKubeProxyBack: kube-proxy's DaemonSet
// was moved to v1.36.0 by hand while the run, confirmed for v1.35.2, sat
// inside its kube-proxy step. Neither the DaemonSet nor a node's configuration
// is moved back.
//
// Sequential: FastPolling changes a package variable.
func TestAResumedKubernetesUpgradeNeverMovesKubeProxyBack(t *testing.T) {
	const byHand = "registry.k8s.io/kube-proxy:v1.36.0"
	r := newRemovalRigWith(t, 1, 0, removalRigOptions{Sim: func(o *kubesim.Options) {
		o.DaemonSets = append(o.DaemonSets, kubesim.DaemonSet{
			Namespace: "kube-system", Name: "kube-proxy", Scheduled: 1, Ready: 1, Image: byHand,
		})
	}})
	withGate(r)
	// What the composition root wires, without the rollout wait.
	r.deps.KubeProxy = func(ctx context.Context, cluster model.ClusterID, image string) (string, error) {
		kc, err := r.deps.Kube(ctx, cluster)
		if err != nil {
			return "", err
		}
		if err := kc.SetDaemonSetImage(ctx, "kube-system", "kube-proxy", "kube-proxy", image); err != nil {
			if errors.Is(err, kube.ErrNoSuchDaemonSet) {
				return "", upgrade.ErrNoKubeProxy
			}
			return "", err
		}
		return "kube-proxy set to " + image, nil
	}
	upgrade.FastPolling(t)
	m := r.machine("cp-1")

	const to = "v1.35.2"
	names := kubernetesStepNames(to, m.ID)
	verifiable := map[string]bool{}
	for _, n := range names {
		verifiable[n] = true
	}
	req := upgrade.Request{To: to, Machines: []model.MachineID{m.ID}}
	id := r.stageUpgradeAt(t, upgrade.JobKindKubernetesUpgrade, req, names, verifiable, 4)
	applies := r.sim("cp-1").Calls("ApplyConfiguration")

	r.resume(t)
	j := r.await(r.engine, id, model.JobFailed)

	detail := stepNamed(j, names[4]).Detail
	for _, want := range []string{"v1.36.0", "newer", "not moved back"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the failure %q does not say %q", detail, want)
		}
	}
	images, err := r.kubeClient(t).DaemonSetImages(t.Context(), "kube-system", "kube-proxy")
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || images[0] != byHand {
		t.Errorf("kube-proxy's DaemonSet runs %v after the resume, want it left on %s", images, byHand)
	}
	if n := r.sim("cp-1").Calls("ApplyConfiguration") - applies; n != 0 {
		t.Errorf("the configuration was applied %d times by a step that refused", n)
	}
}
