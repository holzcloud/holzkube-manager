package clustertemplate

// What a template means for the fleet as it is now.
//
// This is the half of a template that is worth having before anything is
// applied, and it is the half this product can be sure of: every question it
// answers is answered from records this installation already holds. Nothing
// here reaches a node.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// Fleet is what a plan is computed against.
type Fleet struct {
	Clusters []model.Cluster
	Machines []model.Machine
	Classes  []model.MachineClass
}

// Plan is what a template would mean, said in full.
//
// It is deliberately not a yes/no. A template that cannot be applied and a
// template that can are two ends of a range, and most of the useful answers are
// in between -- "this class names two machines and you asked for three" is a
// thing to go and fix, not an error to retry.
type Plan struct {
	// Cluster is the name the template names, and Exists says whether this
	// installation has one.
	Cluster string `json:"cluster"`
	Exists  bool   `json:"exists"`

	// ControlPlane and Workers are what each side resolves to right now.
	ControlPlane NodeSetPlan `json:"control_plane"`
	Workers      NodeSetPlan `json:"workers"`

	// Problems are the reasons this template cannot be applied as written.
	// Empty means it can.
	Problems []string `json:"problems"`

	// Notes are things worth knowing that are not problems -- a cluster that
	// already exists, a version the nodes are not on yet.
	Notes []string `json:"notes"`

	// Sentence is the whole plan in one line, for a screen and for a CLI.
	Sentence string `json:"sentence"`
}

// NodeSetPlan is one side of the cluster, resolved.
type NodeSetPlan struct {
	// Source says where these came from: a class id, or "named".
	Source string `json:"source"`

	// Machines are the ids this side resolves to, in the order they would be
	// used.
	Machines []model.MachineID `json:"machines"`

	// Wanted is the count the template asked for, and 0 when it asked for all
	// of them.
	Wanted int `json:"wanted"`
}

// Usable reports a plan with no problems.
func (p Plan) Usable() bool { return len(p.Problems) == 0 }

// Make computes what a template would do against a fleet.
func Make(t Template, f Fleet) Plan {
	// Empty rather than nil. Go marshals a nil slice as null, the browser's
	// schemas declare these as z.array(...).default([]), and a zod default
	// applies to undefined and not to null -- so a template that resolves
	// cleanly, which is the one an operator most wants to see, sent nulls and
	// the panel never rendered. The contract's rule, from the inventory
	// handler: "Never null: a null reads to a client as 'the server did not
	// check', which is a weaker claim than 'there are none'."
	p := Plan{
		Cluster:  t.Name,
		Problems: make([]string, 0),
		Notes:    make([]string, 0),
	}

	for _, c := range f.Clusters {
		if strings.EqualFold(c.Name, t.Name) {
			p.Exists = true
			p.Notes = append(p.Notes, fmt.Sprintf(
				"A cluster named %q already exists here. Applying this template would change it "+
					"rather than build a new one.", c.Name))
			break
		}
	}

	byClass := map[string]model.MachineClass{}
	for _, c := range f.Classes {
		byClass[string(c.ID)] = c
	}

	p.ControlPlane, p.Problems = resolve(t.ControlPlane, "controlPlane", f, byClass, p.Problems)
	p.Workers, p.Problems = resolve(t.Workers, "workers", f, byClass, p.Problems)

	// The one structural rule about a control plane that holds regardless of
	// anything else: an even number of voting members tolerates no more
	// failures than the odd number below it, so it is a note rather than a
	// refusal -- somebody may be mid-change.
	if n := len(p.ControlPlane.Machines); n > 0 && n%2 == 0 {
		p.Notes = append(p.Notes, fmt.Sprintf(
			"This control plane has %d nodes. An even number tolerates no more failures than %d "+
				"does, because a quorum of %d is still %d.", n, n-1, n, n/2+1))
	}

	p.Problems = append(p.Problems, versionProblems(t)...)
	p.Sentence = sentence(p)
	return p
}

// resolve turns one side of a template into the machines it names now.
func resolve(
	set NodeSet,
	field string,
	f Fleet,
	byClass map[string]model.MachineClass,
	problems []string,
) (NodeSetPlan, []string) {
	// Machines likewise: the panel joins this list, and joining null throws
	// before anything is drawn.
	plan := NodeSetPlan{Wanted: set.Count, Machines: make([]model.MachineID, 0)}

	switch {
	case set.MachineClass != "":
		plan.Source = set.MachineClass

		class, ok := byClass[set.MachineClass]
		if !ok {
			return plan, append(problems, fmt.Sprintf(
				"%s names the machine class %q and this installation has no class by that id",
				field, set.MachineClass))
		}

		for _, m := range f.Machines {
			if class.Selector.Matches(m.Labels) {
				plan.Machines = append(plan.Machines, m.ID)
			}
		}
		sortIDs(plan.Machines)

		if set.Count > 0 {
			if len(plan.Machines) < set.Count {
				problems = append(problems, fmt.Sprintf(
					"%s asks for %d machines from the class %q and it names %d right now (%s)",
					field, set.Count, class.Name, len(plan.Machines), class.Selector.Sentence()))
				return plan, problems
			}
			// Taking the first n by id rather than at random, so that reading
			// the plan twice gives the same answer. Which n is a decision this
			// product has no basis for making differently.
			plan.Machines = plan.Machines[:set.Count]
		}

	case len(set.Machines) > 0:
		plan.Source = "named"
		known := map[model.MachineID]bool{}
		for _, m := range f.Machines {
			known[m.ID] = true
		}
		for _, id := range set.Machines {
			if !known[id] {
				problems = append(problems, fmt.Sprintf(
					"%s names the machine %s and this installation has no record of it", field, id))
				continue
			}
			plan.Machines = append(plan.Machines, id)
		}
	}

	return plan, problems
}

// versionProblems checks what the template says against what it can mean on
// its own.
//
// It takes no fleet, deliberately. Checking a template's versions against what
// the nodes currently run would turn "this cluster is mid-upgrade" into a
// problem with the document, and the document is not wrong -- it is the
// destination. What is checkable here is only whether the two strings are
// shaped like versions, plus the one structural mistake this format makes easy.
func versionProblems(t Template) []string {
	var problems []string

	if t.TalosVersion != "" && !strings.HasPrefix(t.TalosVersion, "v") {
		problems = append(problems, fmt.Sprintf(
			"talosVersion is %q and a Talos version starts with a v", t.TalosVersion))
	}
	if t.KubernetesVersion != "" && !strings.HasPrefix(t.KubernetesVersion, "v") {
		problems = append(problems, fmt.Sprintf(
			"kubernetesVersion is %q and a Kubernetes version starts with a v", t.KubernetesVersion))
	}

	// A machine named on both sides is the mistake that produces a node which
	// is a control-plane node and a worker at once, and the template format
	// makes it easy: two lists, and a UUID looks like every other UUID.
	seen := map[model.MachineID]string{}
	for _, pair := range []struct {
		field string
		set   NodeSet
	}{{"controlPlane", t.ControlPlane}, {"workers", t.Workers}} {
		for _, id := range pair.set.Machines {
			if where, already := seen[id]; already {
				problems = append(problems, fmt.Sprintf(
					"the machine %s is named in both %s and %s", id, where, pair.field))
				continue
			}
			seen[id] = pair.field
		}
	}

	sort.Strings(problems)
	return problems
}

func sentence(p Plan) string {
	if !p.Usable() {
		return fmt.Sprintf("This template cannot be applied as written: %d problem(s).", len(p.Problems))
	}

	control := len(p.ControlPlane.Machines)
	workers := len(p.Workers.Machines)

	verb := "build"
	if p.Exists {
		verb = "change"
	}
	return fmt.Sprintf("This template would %s the cluster %q with %d control-plane node(s) and %d worker(s).",
		verb, p.Cluster, control, workers)
}
