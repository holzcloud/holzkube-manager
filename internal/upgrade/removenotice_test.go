package upgrade_test

import (
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// The route test of the same notice (internal/httpapi, the 202 answer) needs a
// second loopback address and skips on macOS. This one runs everywhere, so the
// text is held on every machine a session may run on, not only in CI.
//
// What it holds is what the notice says a stopped removal leaves behind:
// the drain evicts everything it may, pass by pass, before it reports the pods
// a budget holds, so "nothing on the node has changed" was never true. The
// removal stops before etcd and the wipe, evicted pods stay evicted, the
// cordon comes back or the job says it did not, and only a cluster that never
// had Kubernetes access skips the Kubernetes half; one that lost it fails
// (D-01 and D-03 Q7 as amended on 2026-11-21).
func TestTheRemovalNoticeSaysWhatAStoppedRemovalLeaves(t *testing.T) {
	t.Parallel()

	notice := upgrade.RemoveNodeNotice
	for _, want := range []string{
		"PodDisruptionBudget",
		"node object",
		"before the node leaves etcd or is wiped",
		"already evicted",
		"no Kubernetes access to from the start is removed without the cordon, the drain and the deletion of the node object",
		"loses it on the way fails instead",
	} {
		if !strings.Contains(notice, want) {
			t.Errorf("the removal notice does not say %q: %q", want, notice)
		}
	}
	for _, not := range []string{
		"before anything on the node",
		"kubectl drain",
	} {
		if strings.Contains(notice, not) {
			t.Errorf("the removal notice still says %q: %q", not, notice)
		}
	}
}
