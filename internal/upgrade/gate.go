// Package upgrade rolls a cluster forward, one node at a time, behind a gate
// that would rather refuse than strand it.
//
// The asymmetry that shapes everything here: a three-member etcd tolerates one
// member being away and does not tolerate two. Taking the second one down is
// not a slow operation or a degraded one -- it is a cluster that has stopped,
// and there is no command that brings it back without a snapshot. So the gate
// **refuses** rather than warns, and it is re-evaluated before every node
// rather than once at the start (UPG-02): a rolling upgrade's second node is
// being taken down in a cluster that the first node just changed.
package upgrade

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// MinVotingMembers is the smallest etcd a node may be taken out of.
//
// Three. Not two, and the difference is the whole gate: three voters tolerate
// one being away, two tolerate none. A cluster of two that loses a member has
// one member left and one vote out of two, which is not a majority -- it stops
// accepting writes, and the Kubernetes API stops with it.
//
// A cluster of exactly one is the deliberate exception below: it has nothing
// to lose quorum against, and refusing to upgrade a single-node cluster would
// make the smallest homelab unupgradeable.
const MinVotingMembers = 3

// GateInput is everything the gate read, kept so it can be shown.
//
// UPG-02 asks that the gate "show its inputs", and this type is that promise
// made structural: a verdict is never returned without the numbers it was
// computed from. A gate that only says no is a gate an operator works around.
type GateInput struct {
	// Members is the etcd membership as the cluster reports it, learners
	// included and marked.
	Members []talos.EtcdMemberDetail `json:"members"`

	// Voting is how many of them can vote. Learners replicate and do not vote,
	// and counting one is how a "three-member" cluster turns out to have two
	// votes at exactly the moment one of them is being rebooted.
	Voting int `json:"voting"`

	// Statuses is each reachable member's own account of the raft. They are
	// read per member rather than once, because "the raft has converged" is a
	// statement about every member and a single member cannot make it.
	Statuses map[string]talos.EtcdStatus `json:"statuses"`

	// Unreachable names the members that did not answer. A member that cannot
	// be asked is not a member that is fine.
	Unreachable []string `json:"unreachable,omitempty"`

	// Alarms are etcd's own. NOSPACE and CORRUPT are both conditions under
	// which nothing should be taken down.
	Alarms []talos.EtcdAlarm `json:"alarms,omitempty"`

	// MaxRaftLag is the largest gap between any member's applied index and the
	// highest index seen. A member that is behind is a member that has not
	// caught up with what the last node did, and taking the next one down
	// while it is still catching up is how a rolling upgrade outruns its own
	// cluster.
	MaxRaftLag uint64 `json:"max_raft_lag"`
}

// MaxRaftLag is how far behind a member may be and still count as converged.
//
// Zero would be wrong: a healthy raft is always a few entries apart between
// the leader's index and a follower's applied index, because entries are
// applied after they are replicated. What is not healthy is a member that is
// thousands behind -- that is one still replaying, and it will not survive
// losing a peer.
const MaxRaftLag = 128

// Verdict is the gate's answer.
type Verdict struct {
	// OK is whether the next node may be taken down.
	OK bool `json:"ok"`

	// Reason is why not, in a sentence naming the number that decided it. It
	// is empty when OK.
	Reason string `json:"reason,omitempty"`

	// Input is what was read. Always present, including when OK: an operator
	// asking "why did it let that through" deserves the same answer as one
	// asking why it did not.
	Input GateInput `json:"input"`
}

// Gate reads a cluster's etcd and decides whether a node may be taken down.
//
// connect opens a client to one machine. It is a function rather than a client
// because the gate has to ask **every** member, and a gate that asked only the
// node it was about to upgrade would be asking the one member whose answer
// does not matter.
type Gate struct {
	connect func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error)

	// controlPlanes lists the control-plane machines of a cluster, which is
	// who the gate asks.
	controlPlanes func(ctx context.Context, id model.ClusterID) ([]model.Machine, error)
}

// NewGate builds one.
func NewGate(
	connect func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error),
	controlPlanes func(ctx context.Context, id model.ClusterID) ([]model.Machine, error),
) *Gate {
	return &Gate{connect: connect, controlPlanes: controlPlanes}
}

// Evaluate reads the cluster and decides.
//
// taking is the machine about to be taken down. It is a parameter because the
// question is not "is this cluster healthy" but "does this cluster survive
// losing that node", and those differ for exactly the member that is about to
// go: a cluster of three where one is already unreachable is fine, unless the
// one about to go is a different one.
func (g *Gate) Evaluate(ctx context.Context, cluster model.ClusterID, taking model.MachineID) (Verdict, error) {
	machines, err := g.controlPlanes(ctx, cluster)
	if err != nil {
		return Verdict{}, err
	}
	if len(machines) == 0 {
		return Verdict{Reason: "This cluster has no control-plane node in the inventory, so there " +
			"is no etcd to ask. Refresh the cluster before upgrading anything in it."}, nil
	}

	in := GateInput{Statuses: make(map[string]talos.EtcdStatus, len(machines))}

	var highestIndex uint64
	for _, m := range machines {
		status, members, alarms, err := g.read(ctx, m.ID)
		if err != nil {
			in.Unreachable = append(in.Unreachable, nameOf(m))
			continue
		}

		in.Statuses[nameOf(m)] = status
		if status.RaftIndex > highestIndex {
			highestIndex = status.RaftIndex
		}
		if len(members) > len(in.Members) {
			in.Members = members
		}
		in.Alarms = append(in.Alarms, alarms...)
	}

	in.Voting = talos.VotingMembers(in.Members)
	for _, s := range in.Statuses {
		if lag := highestIndex - min(s.RaftAppliedIndex, highestIndex); lag > in.MaxRaftLag {
			in.MaxRaftLag = lag
		}
	}
	in.Alarms = dedupeAlarms(in.Alarms)
	sort.Strings(in.Unreachable)

	return decide(in, taking, machines), nil
}

// decide is the gate's rule, separated from the reading so that it is a
// function of its inputs and nothing else -- which is what makes every branch
// below testable without a cluster.
func decide(in GateInput, taking model.MachineID, machines []model.Machine) Verdict {
	v := Verdict{Input: in}

	// A single-node cluster has no quorum to lose. Refusing it would make the
	// smallest homelab unupgradeable, and taking its one node down is an
	// outage the operator already knows they are asking for.
	if len(machines) == 1 && in.Voting <= 1 {
		v.OK = true
		return v
	}

	if len(in.Alarms) > 0 {
		v.Reason = fmt.Sprintf(
			"etcd has raised %s. An alarm means a member cannot write or cannot be trusted to "+
				"replicate what it has; taking a node down on top of that is how a recoverable "+
				"problem becomes a restore from backup. Clear the alarm first.",
			describeAlarms(in.Alarms))
		return v
	}

	if len(in.Unreachable) > 0 {
		v.Reason = fmt.Sprintf(
			"%s did not answer, so this cluster is already one member short of what it thinks it "+
				"has. Taking another node down now is the second failure, not the first.",
			strings.Join(in.Unreachable, ", "))
		return v
	}

	if in.Voting < MinVotingMembers {
		v.Reason = fmt.Sprintf(
			"etcd has %d voting member(s). Three is the smallest number that survives losing one: "+
				"with two, the remaining member holds one vote out of two, which is not a majority, "+
				"and the cluster stops accepting writes until the other comes back. Add a "+
				"control-plane node, or accept the outage and do this by hand.",
			in.Voting)
		return v
	}

	// A learner cannot vote, so upgrading one is not a quorum question at all
	// -- but the *cluster* still has to be healthy, which the checks above
	// already established. What is refused here is a learner being counted as
	// one of the three.
	for _, m := range in.Members {
		if m.IsLearner && strings.EqualFold(m.Hostname, hostnameOf(taking, machines)) {
			v.OK = true
			return v
		}
	}

	if in.MaxRaftLag > MaxRaftLag {
		v.Reason = fmt.Sprintf(
			"One member is %d raft entries behind the leader (the limit is %d). It has not caught "+
				"up with what the previous node did, and a member that is still replaying will not "+
				"survive losing a peer. Wait, then look again.",
			in.MaxRaftLag, MaxRaftLag)
		return v
	}

	for name, s := range in.Statuses {
		if s.Leader == 0 {
			v.Reason = fmt.Sprintf(
				"%s reports no etcd leader, which is a cluster mid-election. Nothing should be "+
					"taken down until one is chosen.", name)
			return v
		}
		if len(s.Errors) > 0 {
			v.Reason = fmt.Sprintf("%s reports: %s", name, strings.Join(s.Errors, "; "))
			return v
		}
	}

	v.OK = true
	return v
}

// read asks one member everything the gate needs, on one connection.
func (g *Gate) read(ctx context.Context, id model.MachineID) (
	talos.EtcdStatus, []talos.EtcdMemberDetail, []talos.EtcdAlarm, error,
) {
	cc, err := g.connect(ctx, id)
	if err != nil {
		return talos.EtcdStatus{}, nil, nil, err
	}
	defer cc.Close() //nolint:errcheck // the gate's verdict is not a close error's to change

	statusCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodEtcdStatus)
	if err != nil {
		return talos.EtcdStatus{}, nil, nil, err
	}
	status, err := cc.EtcdStatus(statusCtx)
	cancel()
	if err != nil {
		return talos.EtcdStatus{}, nil, nil, err
	}

	membersCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodEtcdMemberList)
	if err != nil {
		return talos.EtcdStatus{}, nil, nil, err
	}
	members, err := cc.EtcdMembers(membersCtx)
	cancel()
	if err != nil {
		return talos.EtcdStatus{}, nil, nil, err
	}

	alarmCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodEtcdAlarmList)
	if err != nil {
		return talos.EtcdStatus{}, nil, nil, err
	}
	alarms, err := cc.EtcdAlarmList(alarmCtx)
	cancel()
	if err != nil {
		// An alarm list that cannot be read is not "no alarms". It is reported
		// as the member being unreachable for the gate's purposes, which is
		// the conservative reading and the only honest one.
		return talos.EtcdStatus{}, nil, nil, err
	}

	return status, members, alarms, nil
}

func nameOf(m model.Machine) string {
	if m.Hostname != "" {
		return m.Hostname
	}
	return string(m.ID)
}

func hostnameOf(id model.MachineID, machines []model.Machine) string {
	for _, m := range machines {
		if m.ID == id {
			return m.Hostname
		}
	}
	return ""
}

func dedupeAlarms(alarms []talos.EtcdAlarm) []talos.EtcdAlarm {
	seen := map[talos.EtcdAlarm]bool{}
	out := alarms[:0]
	for _, a := range alarms {
		if seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	return out
}

func describeAlarms(alarms []talos.EtcdAlarm) string {
	parts := make([]string, 0, len(alarms))
	for _, a := range alarms {
		parts = append(parts, fmt.Sprintf("%s on member %x", a.Type, a.MemberID))
	}
	sort.Strings(parts)
	return strings.Join(parts, " and ")
}
