// Package power is one model for switching three kinds of thing off and on
// again: a whole cluster, one node, and one app (2026-09-26).
//
// # Seven verbs, the same seven everywhere
//
// The operator asked for "a simple way" to stop, force-stop, start, disable,
// enable, restart and force-restart all three. Simple is the constraint that
// shapes the package: the same seven verbs mean the same thing whatever they are
// aimed at, a screen asks one question ("what can I do to this, and why not")
// and gets the same seven answers in the same order, and the reason an action is
// unavailable is a sentence rather than a missing button. A button that is
// silently absent teaches an operator nothing; one that is present and says why
// it cannot be pressed teaches them what state the thing is in.
//
// What the verbs mean, decided by the operator on 2026-09-26:
//
//   - stop takes it down carefully: drain first, honour every PodDisruption
//     Budget, and refuse on a control-plane node whose loss would cost etcd its
//     quorum.
//   - force-stop takes it down now and checks nothing. It is the only thing the
//     sudo window guards here; the careful verbs need the ordinary operator role
//     and the cluster's mutation lock, like every other change.
//   - start brings it back. For a node or a cluster that is off, that is
//     Wake-on-LAN from this daemon -- the daemon runs on the same LAN, and it is
//     the one thing that is still up when everything else is off.
//   - disable is stop plus a mark that keeps it off: a start refuses, a cluster
//     start skips it, and a node that comes up anyway is kept cordoned.
//   - enable removes the mark and starts it.
//   - restart is stop-and-start without leaving the cluster short: one node at a
//     time, drained, and only onto the next once the last one is back.
//   - force-restart reboots now, everything at once, and checks nothing.
//
// # Why the forced verbs are the only ones behind the password
//
// A careful stop refuses the damaging cases itself -- a drain that would break
// a budget, a control-plane node etcd cannot spare -- so the check is in the
// verb. A forced one has no such check by definition, and the password window
// is what stands in for it. Putting the careful ones behind the window too would
// teach the operator that the password prompt is just the thing in front of
// every button, which is how it stops meaning anything.
package power

import "strings"

// Action is one of the seven verbs.
type Action string

// The seven, in the order every answer lists them.
const (
	Stop         Action = "stop"
	ForceStop    Action = "force-stop"
	Start        Action = "start"
	Disable      Action = "disable"
	Enable       Action = "enable"
	Restart      Action = "restart"
	ForceRestart Action = "force-restart"
)

// Actions is every action, in the order the contract promises the frontend.
//
// The order is part of the contract rather than a presentation detail: a
// screen that renders them as a row of buttons renders them in this order, and
// a second client must not have to sort them to agree with the first.
func Actions() []Action {
	return []Action{Stop, ForceStop, Start, Disable, Enable, Restart, ForceRestart}
}

// Sudo reports whether an action needs the password window.
//
// It is the one table the route and the answer both read: the route table marks
// exactly these routes Destructive, and the "sudo" field a screen shows before
// anybody presses comes from here too. Two copies of this list would be two
// answers to "will this ask for my password", and the first place they
// disagreed would be a dialog that asked when it said it would not.
func (a Action) Sudo() bool { return a == ForceStop || a == ForceRestart }

// ParseAction reads an action from a path segment.
func ParseAction(s string) (Action, bool) {
	for _, a := range Actions() {
		if strings.EqualFold(s, string(a)) {
			return a, true
		}
	}
	return "", false
}

// State is what the thing is doing, in one word.
type State string

const (
	// StateRunning is up: every node answers, every replica is wanted.
	StateRunning State = "running"

	// StateStopped is meant to run nothing, or does not answer at all.
	StateStopped State = "stopped"

	// StatePartial is somewhere between: some nodes answer and some do not, or
	// an app wants more than it has ready.
	StatePartial State = "partial"

	// StateDisabled is off because somebody said it should stay off. It wins
	// over what the thing is actually doing, because it is the one state an
	// observation cannot produce and the one a screen must never hide.
	StateDisabled State = "disabled"

	// StateUnknown is the honest answer when nothing could be asked.
	StateUnknown State = "unknown"
)

// ActionStatus is one action's verdict.
type ActionStatus struct {
	Action    Action `json:"action"`
	Available bool   `json:"available"`

	// Reason is one plain sentence when the action is unavailable, and empty
	// when it is available. Never omitted: a client reads "" as "nothing
	// stands in the way", and a missing field as a server that did not say.
	Reason string `json:"reason"`

	// Sudo is whether pressing it asks for the password.
	Sudo bool `json:"sudo"`
}

// Report is the answer to "what can I do to this, and why not".
type Report struct {
	State    State          `json:"state"`
	Disabled bool           `json:"disabled"`
	Actions  []ActionStatus `json:"actions"`
}

// Available reports whether one action is available, and the sentence if not.
func (r Report) Available(a Action) (bool, string) {
	for _, s := range r.Actions {
		if s.Action == a {
			return s.Available, s.Reason
		}
	}
	return false, "That is not an action."
}

// report assembles the seven verdicts in order from one function that answers
// "why not" -- empty meaning "it can".
//
// One function per target rather than a table of flags, because the reasons are
// the product here: each unavailable action has to say, in a sentence, what
// state makes it so, and a sentence is a decision about that target and not a
// cell in a grid.
func report(state State, disabled bool, whyNot func(Action) string) Report {
	out := Report{State: state, Disabled: disabled, Actions: make([]ActionStatus, 0, len(Actions()))}
	for _, a := range Actions() {
		reason := whyNot(a)
		out.Actions = append(out.Actions, ActionStatus{
			Action:    a,
			Available: reason == "",
			Reason:    reason,
			Sudo:      a.Sudo(),
		})
	}
	return out
}

// The sentences. Named, because the same condition is the reason for several
// actions on several targets, and a screen that says it three different ways
// reads like three different conditions.
const (
	ReasonRunning         = "It is running."
	ReasonAlreadyStopped  = "It is already stopped."
	ReasonNotRunning      = "It is not running; start it instead."
	ReasonDisabled        = "Disabled — enable it first."
	ReasonAlreadyDisabled = "It is already disabled."
	ReasonNotDisabled     = "It is not disabled."
	ReasonQuorum          = "Stopping this control-plane node would lose etcd quorum; force-stop does not check."
	ReasonEtcdUnhealthy   = "etcd is not healthy enough to lose this control-plane node right now; force-stop does not check."
	ReasonClusterDisabled = "Its cluster is disabled — enable the cluster first."
	ReasonLocked          = "This cluster was adopted read-only; unlock it before changing anything on it."
	ReasonNoMAC           = "No network card address is known for it, so it cannot be woken over the network; power it on by hand."
	ReasonPartial         = "Some of its nodes are not running; start it first."
	ReasonNoMachines      = "No machine of this cluster is in the inventory, so there is nothing to switch."
	ReasonNoneAnswer      = "None of its nodes can be asked whether they are running, so its state is unknown."

	ReasonBarePodStart    = "A bare pod cannot be started again once deleted."
	ReasonBarePodDisable  = "A bare pod cannot be disabled: stopping it deletes it, so there is nothing left to mark."
	ReasonBarePodRestart  = "A bare pod has no controller to restart it."
	ReasonBarePodRecreate = "A bare pod has no controller to recreate it."
	ReasonJobFinished     = "It has already finished."
	ReasonJobRunsOnce     = "It has finished, and a Job does not run twice."
	ReasonNoRollout       = "It runs to completion rather than being kept running, so it has no rollout to restart."
)
