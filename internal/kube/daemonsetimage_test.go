package kube_test

import (
	"errors"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// A Kubernetes upgrade moves kube-proxy by pointing its DaemonSet at the new
// image and waiting for the rollout (ledger 86). The image has to be read back
// from the DaemonSet, not taken on the patch's word.
func TestADaemonSetMovesToANewImageAndReportsItsRollout(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{DaemonSets: []kubesim.DaemonSet{{
		Namespace: "kube-system", Name: "kube-proxy", Scheduled: 3, Ready: 3,
		Image: "registry.k8s.io/kube-proxy:v1.34.1",
	}}})

	const next = "registry.k8s.io/kube-proxy:v1.35.2"
	if err := client.SetDaemonSetImage(ctx, "kube-system", "kube-proxy", "kube-proxy", next); err != nil {
		t.Fatalf("SetDaemonSetImage: %v", err)
	}
	images, err := client.DaemonSetImages(ctx, "kube-system", "kube-proxy")
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || images[0] != next {
		t.Errorf("the DaemonSet runs %v after the change, want [%s]", images, next)
	}

	rollout, err := client.DaemonSetRolledOut(ctx, "kube-system", "kube-proxy")
	if err != nil {
		t.Fatal(err)
	}
	if !rollout.Done || rollout.Desired != 3 || rollout.Available != 3 {
		t.Errorf("rollout = %+v, want done with 3 of 3", rollout)
	}
}

// A cluster whose CNI replaces kube-proxy has no DaemonSet to move, and that is
// said as such rather than as a failure.
func TestNoKubeProxyIsSaidAsSuch(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{})

	err := client.SetDaemonSetImage(ctx, "kube-system", "kube-proxy", "kube-proxy", "x:v1")
	if !errors.Is(err, kube.ErrNoSuchDaemonSet) {
		t.Errorf("err = %v, want ErrNoSuchDaemonSet", err)
	}
}
