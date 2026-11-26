package upgrade_test

import (
	"context"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// The control-plane parts of a resumed Kubernetes upgrade: a component moved
// past the target by hand is not moved back, and the kube-proxy step, which is
// safe to repeat, is repeated rather than parked.
//
// Sequential: FastPolling changes a package variable.

// withGate gives the rig the etcd gate the component steps ask before every
// control-plane node.
func withGate(r *removalRig) {
	r.deps.Gate = upgrade.NewGate(r.deps.Connect, func(ctx context.Context, c model.ClusterID) ([]model.Machine, error) {
		all, err := r.deps.Machines(ctx, c)
		var cps []model.Machine
		for _, m := range all {
			if m.Role == model.RoleControlPlane {
				cps = append(cps, m)
			}
		}
		return cps, err
	})
}

func (r *removalRig) staticPodImage(t *testing.T, id model.MachineID, component string) string {
	t.Helper()
	cc, err := r.deps.Connect(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer cc.Close() //nolint:errcheck // a test read
	ctx, cancel, err := talos.WithClassDeadline(t.Context(), talos.MethodCOSIList)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	pods, err := cc.StaticPods(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pods {
		if p.Component == component {
			return p.Image
		}
	}
	t.Fatalf("%s reports no %s", id, component)
	return ""
}

// TestAResumedKubernetesUpgradeNeverMovesAComponentBack: cp-1's API server was
// moved to v1.36.0 by hand while the run, confirmed for v1.35.2, sat inside
// its API-server step.
func TestAResumedKubernetesUpgradeNeverMovesAComponentBack(t *testing.T) {
	r := newRemovalRig(t, 1, 0)
	withGate(r)
	upgrade.FastPolling(t)
	m := r.machine("cp-1")

	newer, err := upgrade.ParseVersion("v1.36.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := upgrade.ApplyComponentImage(t.Context(), r.deps, m.ID, "kube-apiserver", newer); err != nil {
		t.Fatalf("moving the API server by hand: %v", err)
	}
	byHand := r.staticPodImage(t, m.ID, "kube-apiserver")
	if !strings.HasSuffix(byHand, ":v1.36.0") {
		t.Fatalf("the API server runs %s after the move by hand", byHand)
	}

	const to = "v1.35.2"
	names := kubernetesStepNames(to, m.ID)
	verifiable := map[string]bool{names[1]: true, names[2]: true, names[3]: true, names[5]: true}
	req := upgrade.Request{To: to, Machines: []model.MachineID{m.ID}}
	id := r.stageUpgradeAt(t, upgrade.JobKindKubernetesUpgrade, req, names, verifiable, 1)

	r.resume(t)
	j := r.await(r.engine, id, model.JobFailed)

	detail := stepNamed(j, names[1]).Detail
	for _, want := range []string{"v1.36.0", "newer", "not moved back"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the failure %q does not say %q", detail, want)
		}
	}
	if got := r.staticPodImage(t, m.ID, "kube-apiserver"); got != byHand {
		t.Errorf("the API server runs %s after the resume, want it left on %s", got, byHand)
	}
}
