package power

import (
	"github.com/holzcloud/holzkube-manager/internal/kube"
)

// The rules: from what was read, which of the seven may be pressed and why not.
//
// They are pure functions of facts gathered elsewhere, for the reason
// upgrade.decide is: every branch is about a state that is expensive to produce
// against a simulator -- a disabled node somebody powered on, a control-plane
// node etcd cannot spare, a Job that finished -- and a rule that is a function
// of its inputs is a rule a table can walk. The same functions answer the GET
// and gate the POST, so a button the screen showed as available is the button
// the server accepts, and the sentence it showed is the 409's detail.

// answer is whether a node answered its liveness probe.
type answer int

const (
	// answerUnknown is a node nobody could ask: no credentials, a cluster whose
	// secrets are gone, a version this build refuses. It is not "off", and
	// treating it as off would offer to wake a machine that is running.
	answerUnknown answer = iota
	answerYes
	answerNo
)

// nodeFacts is everything a node's verdicts are computed from.
type nodeFacts struct {
	Answer answer

	// Unknown is why the node could not be asked, when Answer is unknown.
	Unknown string

	Disabled        bool
	ClusterDisabled bool
	ClusterLocked   bool
	PowerCordoned   bool
	HasMACs         bool

	// GateRefusal is the etcd gate's sentence when this is a running
	// control-plane node that the cluster cannot spare, and empty otherwise --
	// including for every worker, which is not an etcd member.
	GateRefusal string
}

func (f nodeFacts) state() State {
	switch {
	case f.Disabled:
		return StateDisabled
	case f.Answer == answerYes:
		return StateRunning
	case f.Answer == answerNo:
		return StateStopped
	default:
		return StateUnknown
	}
}

// nodeReport is one node's seven verdicts.
func nodeReport(f nodeFacts) Report {
	return report(f.state(), f.Disabled, func(a Action) string {
		// The lock first: an operator told "etcd cannot spare it" goes looking
		// at etcd, when what actually stands in the way is a read-only
		// adoption they have to lift before anything here is possible.
		if f.ClusterLocked {
			return ReasonLocked
		}

		switch a {
		case Stop:
			// A disabled node that answers is one somebody powered on; taking
			// it back down is exactly what the mark asks for, so stop stays
			// available -- through the same gate as any other stop.
			if why := f.needsAnswer(); why != "" {
				return why
			}
			return f.GateRefusal

		case ForceStop:
			return f.needsAnswer()

		case Start:
			switch {
			case f.Disabled:
				return ReasonDisabled
			case f.ClusterDisabled:
				return ReasonClusterDisabled
			case f.Answer == answerUnknown:
				return f.Unknown
			case f.Answer == answerYes && !f.PowerCordoned:
				return ReasonRunning
			case f.Answer == answerNo && !f.HasMACs:
				return ReasonNoMAC
			}
			// Off, or running and still cordoned from a power action: a start
			// of a node that answers is its uncordon.
			return ""

		case Disable:
			switch {
			case f.Disabled:
				return ReasonAlreadyDisabled
			case f.Answer == answerUnknown:
				return f.Unknown
			case f.Answer == answerYes:
				// Disable does what stop does, so it refuses what stop refuses.
				return f.GateRefusal
			}
			// A node that is already off is disabled by the mark alone.
			return ""

		case Enable:
			switch {
			case !f.Disabled:
				return ReasonNotDisabled
			case f.ClusterDisabled:
				return ReasonClusterDisabled
			}
			return ""

		case Restart, ForceRestart:
			switch {
			case f.Disabled:
				return ReasonDisabled
			case f.Answer == answerUnknown:
				return f.Unknown
			case f.Answer == answerNo:
				return ReasonNotRunning
			}
			return ""
		}
		return "That is not an action."
	})
}

// needsAnswer is the refusal of an action that has to reach the node.
func (f nodeFacts) needsAnswer() string {
	switch f.Answer {
	case answerUnknown:
		return f.Unknown
	case answerNo:
		return ReasonAlreadyStopped
	default:
		return ""
	}
}

// memberFacts is one node as its cluster's verdicts need it.
type memberFacts struct {
	Answer        answer
	Disabled      bool
	PowerCordoned bool
}

// clusterFacts is everything a cluster's verdicts are computed from.
type clusterFacts struct {
	Disabled bool
	Locked   bool
	Members  []memberFacts
}

// underlying is what the cluster is doing, disabled or not.
//
// Disabled nodes are left out of it, because they are off on purpose: a
// cluster whose every enabled node answers is running, and calling it
// "partial" because the node somebody disabled is off would make the word mean
// nothing.
func (f clusterFacts) underlying() (State, string) {
	if len(f.Members) == 0 {
		return StateUnknown, ReasonNoMachines
	}
	var active, yes, no int
	for _, m := range f.Members {
		if m.Disabled {
			continue
		}
		active++
		switch m.Answer {
		case answerYes:
			yes++
		case answerNo:
			no++
		}
	}
	switch {
	case active == 0:
		return StateStopped, ""
	case yes == active:
		return StateRunning, ""
	case no == active:
		return StateStopped, ""
	case yes == 0 && no == 0:
		return StateUnknown, ReasonNoneAnswer
	default:
		return StatePartial, ""
	}
}

func (f clusterFacts) anyAnswering() bool {
	for _, m := range f.Members {
		if m.Answer == answerYes {
			return true
		}
	}
	return false
}

func (f clusterFacts) anyMarked() bool {
	for _, m := range f.Members {
		if !m.Disabled && m.PowerCordoned {
			return true
		}
	}
	return false
}

// clusterReport is one cluster's seven verdicts.
func clusterReport(f clusterFacts) Report {
	under, unknown := f.underlying()
	state := under
	if f.Disabled {
		state = StateDisabled
	}

	return report(state, f.Disabled, func(a Action) string {
		if f.Locked {
			return ReasonLocked
		}

		switch a {
		case Stop, ForceStop:
			// Anything that answers can be stopped, a disabled node somebody
			// powered on included.
			if f.anyAnswering() {
				return ""
			}
			if under == StateUnknown {
				return unknown
			}
			return ReasonAlreadyStopped

		case Start:
			switch {
			case f.Disabled:
				return ReasonDisabled
			case under == StateUnknown:
				return unknown
			case under == StateRunning && !f.anyMarked():
				return ReasonRunning
			}
			return ""

		case Disable:
			switch {
			case f.Disabled:
				return ReasonAlreadyDisabled
			case len(f.Members) == 0:
				return ReasonNoMachines
			}
			return ""

		case Enable:
			if !f.Disabled {
				return ReasonNotDisabled
			}
			return ""

		case Restart, ForceRestart:
			if f.Disabled {
				return ReasonDisabled
			}
			switch under {
			case StateRunning:
				return ""
			case StateStopped:
				return ReasonNotRunning
			case StatePartial:
				// A rolling restart of a cluster with nodes already down walks
				// onto the first one of them and stops there, having restarted
				// half the cluster for nothing. Starting it first is the
				// sentence, not a half-finished job.
				return ReasonPartial
			default:
				return unknown
			}
		}
		return "That is not an action."
	})
}

// appFacts is everything an app's verdicts are computed from.
type appFacts struct {
	Kind   AppKind
	Locked bool

	// Power is a workload's state; Pod a bare pod's.
	Power kube.AppPower
	Pod   kube.PodPower
}

func (f appFacts) state() State {
	if f.Kind == KindPod {
		switch {
		case f.Pod.Phase == "Succeeded" || f.Pod.Phase == "Failed":
			return StateStopped
		case f.Pod.Phase == "Running" && f.Pod.Ready:
			return StateRunning
		default:
			return StatePartial
		}
	}
	switch {
	case f.Power.Disabled:
		return StateDisabled
	case f.Power.Stopped || f.Power.Finished:
		return StateStopped
	case f.Power.Ready >= f.Power.Desired:
		return StateRunning
	default:
		return StatePartial
	}
}

// appReport is one app's seven verdicts.
func appReport(f appFacts) Report {
	return report(f.state(), f.Power.Disabled, func(a Action) string {
		if f.Locked {
			return ReasonLocked
		}
		if f.Kind == KindPod {
			return barePodWhyNot(f.Pod, a)
		}

		p := f.Power
		switch a {
		case Stop, ForceStop:
			switch {
			case p.Finished:
				return ReasonJobFinished
			case p.Stopped:
				return ReasonAlreadyStopped
			}
			return ""

		case Start:
			switch {
			case p.Disabled:
				return ReasonDisabled
			case p.Finished:
				return ReasonJobRunsOnce
			case !p.Stopped:
				return ReasonRunning
			}
			return ""

		case Disable:
			if p.Disabled {
				return ReasonAlreadyDisabled
			}
			return ""

		case Enable:
			if !p.Disabled {
				return ReasonNotDisabled
			}
			return ""

		case Restart:
			switch {
			case f.Kind == KindCronJob || f.Kind == KindJob:
				return ReasonNoRollout
			case p.Disabled:
				return ReasonDisabled
			case p.Stopped:
				return ReasonNotRunning
			}
			return ""

		case ForceRestart:
			switch {
			case p.Disabled:
				return ReasonDisabled
			case p.Finished:
				return ReasonJobFinished
			case p.Stopped:
				return ReasonNotRunning
			}
			return ""
		}
		return "That is not an action."
	})
}

// barePodWhyNot is the one kind with only a stop.
//
// A pod its controller owns is not an app of its own: stopping it means the
// controller, and deleting it is a restart. It is refused by name rather than
// acted on, the way RestartPod refuses the opposite case.
func barePodWhyNot(p kube.PodPower, a Action) string {
	if p.Owner != "" {
		return "This pod belongs to " + p.Owner + "; stop or start that instead."
	}
	switch a {
	case Stop, ForceStop:
		return ""
	case Start:
		return ReasonBarePodStart
	case Disable:
		return ReasonBarePodDisable
	case Enable:
		return ReasonNotDisabled
	case Restart:
		return ReasonBarePodRestart
	case ForceRestart:
		return ReasonBarePodRecreate
	}
	return "That is not an action."
}
