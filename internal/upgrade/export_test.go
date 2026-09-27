package upgrade

import (
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// Decide exposes the gate's rule to its own test.
//
// The rule is deliberately a pure function of what was read, separated from
// the reading, and this is why: every branch of it is about a cluster in a
// state that is either expensive or impossible to produce -- a raft mid-
// election, a member thousands of entries behind, a NOSPACE alarm. A test that
// had to produce those states against a simulator would test the simulator.
//
// The reading half is covered separately, against a live node, by
// TestTheGateReadsARealClusterAndShowsWhatItRead.
func Decide(in GateInput, taking model.MachineID, machines []model.Machine) Verdict {
	return decide(in, taking, machines)
}

// ComponentPatch, KubeletPatch and ProxyPatch expose the v1alpha1 patch and
// path of each change a Kubernetes upgrade writes; ComponentPatchFor shows the
// patch a given configuration gets.
func ComponentPatch(component string, to Version) (string, []string) {
	c := componentChange(component, to)
	return c.legacy, []string{c.path}
}

func KubeletPatch(to Version) (string, []string) {
	c := kubeletChange(to)
	return c.legacy, []string{c.path}
}

func ProxyPatch(to Version) (string, []string) {
	c := proxyChange(to)
	return c.legacy, []string{c.path}
}

func ComponentPatchFor(component string, to Version, base []byte) (string, error) {
	return componentChange(component, to).patchFor(base)
}

// FastPolling makes the waiting steps poll in milliseconds for the length of a
// test, and puts the interval back afterwards.
func FastPolling(t interface{ Cleanup(func()) }) {
	was := pollInterval
	pollInterval = 20 * time.Millisecond
	t.Cleanup(func() { pollInterval = was })
}
