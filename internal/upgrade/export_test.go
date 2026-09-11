package upgrade

import "github.com/holzcloud/holzkube-manager/internal/model"

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

// KubernetesPatch exposes the configuration a Kubernetes upgrade writes.
func KubernetesPatch(to Version, controlPlane bool) ([]byte, []string) {
	return kubernetesPatch(to, controlPlane)
}
