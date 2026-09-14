// Package scale answers one question about a cluster: what would changing its
// size mean, and which of the changes on offer are refused.
//
// It exists because the two halves already existed and nobody could see them
// together. Adding a node is provisioning, removing one is a button on that
// node's page, and an operator deciding whether their three-node control plane
// can lose a machine had to hold etcd's membership, the inventory, and the
// arithmetic of a majority in their head at once. The arithmetic is the part
// that goes wrong, and it goes wrong in a direction that reads as caution:
// four voting members feel safer than three and tolerate exactly the same
// single loss.
//
// Nothing here acts. Every decision below is computed from records this
// installation already holds plus one read of etcd's membership, and the
// operations it describes are the ones that already exist. That is the scope:
// the missing thing was never a button, it was knowing which button is safe.
//
// And nothing here decides for itself whether a node may be removed. The
// refusal comes from upgrade.RefuseIfCannotSpareAVoter, the same function the
// removal route calls, because a screen that reached its own verdict would
// disagree with the route eventually and would do it silently -- it would look
// right until somebody clicked.
package scale

import (
	"fmt"
	"sort"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/health"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// Node is a machine record plus the one thing about it that is not in the
// record: whether anything is currently answering at it.
//
// Stage is computed by the inventory from live observation, so it cannot be
// derived here. It is carried rather than looked up because this package
// reaches nothing -- that is what makes every sentence below testable without
// a node, a clock or a store.
type Node struct {
	Machine model.Machine
	Stage   health.Stage
}

// Input is everything a plan is computed from.
type Input struct {
	Cluster model.Cluster

	// Nodes are the machines in this cluster.
	Nodes []Node

	// Spare are machines this installation knows about that belong to no
	// cluster. They are the candidates for growing it.
	Spare []Node

	// Members is etcd's membership, and MembersProblem is why it is not there.
	//
	// Both, rather than one or the other, because a plan computed without a
	// membership is not a plan with fewer answers -- it is a plan that must
	// not say any control-plane node is removable. The distinction has to
	// survive into the output.
	Members        upgrade.MemberList
	MembersProblem string
}

// Plan is the cluster's size, and what changing it would mean.
type Plan struct {
	Cluster model.ClusterID `json:"cluster"`
	Name    string          `json:"name"`

	ControlPlane int `json:"control_plane"`
	Workers      int `json:"workers"`

	// Voting and Tolerates are etcd's, and MembersKnown says whether they were
	// read at all. A zero that means "none" and a zero that means "not asked"
	// are different answers and a client must be able to tell them apart.
	MembersKnown   bool   `json:"members_known"`
	Voting         int    `json:"voting"`
	Tolerates      int    `json:"tolerates"`
	MembersProblem string `json:"members_problem,omitempty"`

	// Removals is one decision per machine in the cluster, control-plane
	// nodes first, each group by name, so two reads of an unchanged cluster
	// are the same document.
	Removals []Removal `json:"removals"`

	// Additions is one candidate per spare machine.
	Additions []Candidate `json:"additions"`

	// Advice is the arithmetic, in words. It is the reason this screen exists.
	Advice []string `json:"advice"`

	// Sentence is the whole thing in one line.
	Sentence string `json:"sentence"`
}

// Removal is whether one machine may be taken out of the cluster, and why not.
type Removal struct {
	Machine model.MachineID   `json:"machine"`
	Name    string            `json:"name"`
	Role    model.MachineRole `json:"role"`

	Allowed bool `json:"allowed"`

	// Reason is why not, in the words of whatever refused. It is empty when
	// Allowed, and it is never this package's own paraphrase of the removal
	// route's refusal: two accounts of one condition is how an operator comes
	// to believe a screen and a server disagree.
	Reason string `json:"reason,omitempty"`
}

// Candidate is a machine that could join, and whether it is ready to.
type Candidate struct {
	Machine model.MachineID `json:"machine"`
	Name    string          `json:"name"`

	Ready bool `json:"ready"`

	// Reason is why it is not ready. A spare machine that cannot join is more
	// useful on this screen than absent from it: "there is nothing to add" and
	// "there are two machines and both are unreachable" send an operator to
	// different places.
	Reason string `json:"reason,omitempty"`
}

// Make computes the plan. It reaches nothing.
func Make(in Input) Plan {
	// Empty rather than nil, and this is a wire decision rather than a style
	// one. Go marshals a nil slice as null; the browser's schemas declare
	// these as z.array(...).default([]), and a zod default applies to
	// undefined and not to null. A cluster with no machines -- the first one
	// an operator ever looks at -- therefore sent three nulls, the parse
	// threw, the query retried, and the panel sat on its loading line with
	// nothing in the console or the log.
	//
	// The contract states the rule in the inventory handler: "Never null: a
	// null reads to a client as 'the server did not check', which is a weaker
	// claim than 'there are none'."
	p := Plan{
		Cluster:        in.Cluster.ID,
		Name:           in.Cluster.Name,
		MembersProblem: in.MembersProblem,
		MembersKnown:   in.MembersProblem == "",
		Removals:       make([]Removal, 0, len(in.Nodes)),
		Additions:      make([]Candidate, 0, len(in.Spare)),
	}
	if p.MembersKnown {
		p.Voting = in.Members.VotingCount
		p.Tolerates = in.Members.Tolerates
	}

	for _, n := range inOrder(in.Nodes) {
		if n.Machine.Role == model.RoleControlPlane {
			p.ControlPlane++
		} else {
			p.Workers++
		}
		p.Removals = append(p.Removals, decide(n, in))
	}

	for _, n := range inOrder(in.Spare) {
		p.Additions = append(p.Additions, offer(n))
	}

	p.Advice = advise(p)
	p.Sentence = sentence(p)
	return p
}

// decide is one machine's removal verdict.
//
// The control-plane branch delegates, and that is the whole design of this
// package: the sentence an operator reads here is the sentence the route would
// produce, because it is produced by the same function.
func decide(n Node, in Input) Removal {
	m := n.Machine
	out := Removal{Machine: m.ID, Name: nameOf(m), Role: m.Role, Allowed: true}

	switch {
	case m.Locked:
		// A lock is somebody saying "not this one" (UPG-14). Rolling
		// operations walk past a locked node; a removal is not a rolling
		// operation and cannot walk past anything, so it stops instead.
		out.Allowed = false
		out.Reason = "This node is locked" + because(m.LockReason) +
			". Clear the lock if it should be removed."

	case m.Role != model.RoleControlPlane:
		// A worker is not an etcd member. Nothing about the membership is its
		// business, and a cluster whose etcd cannot be reached must not be a
		// cluster whose workers cannot be removed.

	case in.MembersProblem != "":
		out.Allowed = false
		out.Reason = "This is a control-plane node and " + in.MembersProblem +
			", so there is no way to tell whether the cluster survives losing it."

	default:
		if err := upgrade.RefuseIfCannotSpareAVoter(in.Members, nameOf(m)); err != nil {
			out.Allowed = false
			out.Reason = err.Error()
		}
	}
	return out
}

// offer is one spare machine's readiness.
func offer(n Node) Candidate {
	m := n.Machine
	out := Candidate{Machine: m.ID, Name: nameOf(m), Ready: true}

	switch {
	case !n.Stage.Live():
		// Live is watching or degraded -- the two stages in which something is
		// answering. Unknown, connecting and down are all "nothing is there
		// right now", and a machine nothing is talking to cannot be installed.
		out.Ready = false
		out.Reason = "This machine is " + n.Stage.String() +
			", so there is nothing answering to install on."
	case m.Cluster != "":
		// Defensive: Spare is supposed to be machines with no cluster. If one
		// arrives with a cluster anyway, saying so beats offering it.
		out.Ready = false
		out.Reason = "This machine already belongs to " + string(m.Cluster) + "."
	}
	return out
}

// advise is the arithmetic in words, and it is why this screen exists.
//
// The rule that catches people: a majority of n is n/2+1, so an even number of
// voting members tolerates exactly what the odd number below it does. Four
// members feel safer than three and are not. Every sentence here names the
// numbers rather than asserting a conclusion, because an operator who is told
// "add two" and not why will add one.
func advise(p Plan) []string {
	out := make([]string, 0, 3)
	if !p.MembersKnown {
		// MembersProblem is a whole clause written by whoever failed to read
		// the membership, not a fragment to introduce. Prefixing it produced
		// "etcd's membership could not be read, so nothing here counts votes.
		// etcd's membership could not be read (...)" -- which no test caught,
		// because every test here supplies its own short problem string and
		// only the running server supplies the real one.
		return append(out, p.MembersProblem+", so nothing here counts votes.")
	}

	switch {
	case p.Voting == 0:
		out = append(out, "etcd reports no voting members. That is not a cluster this can "+
			"say anything about.")

	case p.Voting == 1:
		out = append(out, "One voting member. It cannot be removed, and losing the machine "+
			"means restoring from a snapshot. Adding one more gives two, which tolerates no "+
			"loss either -- a majority of two is two. Add two to reach a control plane that "+
			"survives losing one.")

	case p.Voting == 2:
		out = append(out, "Two voting members, and a majority of two is two -- so this "+
			"tolerates no loss at all, the same as one member does. Adding a third gives a "+
			"majority of two out of three, which survives losing one.")

	case p.Voting%2 == 0:
		out = append(out, fmt.Sprintf("%d voting members tolerate %d loss(es), which is what "+
			"%d tolerate: a majority of %d is %d and a majority of %d is also %d. The even "+
			"member is paying for itself and buying nothing. Add one more, or take one out.",
			p.Voting, p.Tolerates, p.Voting-1, p.Voting, p.Voting/2+1,
			p.Voting-1, (p.Voting-1)/2+1))

	default:
		out = append(out, fmt.Sprintf("%d voting members, so %d may be lost before the cluster "+
			"stops accepting writes. Adding one gives %d, which tolerates the same %d.",
			p.Voting, p.Tolerates, p.Voting+1, p.Tolerates))
	}

	// The control plane etcd does not know about. Not an error and not a
	// refusal -- it is a node this installation believes is a control-plane
	// node and etcd has never counted, which is worth seeing before somebody
	// relies on it for a vote it does not have.
	if p.MembersKnown && p.ControlPlane > p.Voting {
		out = append(out, fmt.Sprintf("This cluster has %d control-plane node(s) in the "+
			"inventory and %d voting etcd member(s). A control-plane node that is not a "+
			"member is not a vote, whatever the dashboard counts.", p.ControlPlane, p.Voting))
	}

	if ready := readyCount(p); ready == 0 && len(p.Additions) > 0 {
		out = append(out, fmt.Sprintf("%d machine(s) belong to no cluster and none of them "+
			"can be added right now.", len(p.Additions)))
	}
	return out
}

func readyCount(p Plan) int {
	n := 0
	for _, c := range p.Additions {
		if c.Ready {
			n++
		}
	}
	return n
}

func sentence(p Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d control-plane node(s) and %d worker(s)", label(p), p.ControlPlane, p.Workers)
	if p.MembersKnown {
		fmt.Fprintf(&b, ", etcd tolerating the loss of %d", p.Tolerates)
	} else {
		b.WriteString(", etcd not read")
	}

	removable := 0
	for _, r := range p.Removals {
		if r.Allowed {
			removable++
		}
	}
	fmt.Fprintf(&b, ". %d of %d node(s) may be removed; %d machine(s) could be added.",
		removable, len(p.Removals), readyCount(p))
	return b.String()
}

func label(p Plan) string {
	if p.Name != "" {
		return p.Name
	}
	return string(p.Cluster)
}

func because(reason string) string {
	if reason == "" {
		return ""
	}
	return " (" + reason + ")"
}

// inOrder puts control-plane nodes first, then workers, each group by name --
// the same order upgrade walks a cluster in, so the two screens list a cluster
// the same way.
func inOrder(nodes []Node) []Node {
	out := append([]Node(nil), nodes...)
	sort.SliceStable(out, func(i, j int) bool {
		ci := out[i].Machine.Role == model.RoleControlPlane
		cj := out[j].Machine.Role == model.RoleControlPlane
		if ci != cj {
			return ci
		}
		return nameOf(out[i].Machine) < nameOf(out[j].Machine)
	})
	return out
}

func nameOf(m model.Machine) string {
	if m.Hostname != "" {
		return m.Hostname
	}
	return string(m.ID)
}
