// Package clustertemplate turns a written description of a cluster into a
// statement about what it would mean for the fleet as it is now (Omni parity
// phase 6).
//
// The thing this package deliberately does not do is apply one. Applying is
// provisioning, and provisioning is the one part of this product that has never
// run against real hardware; a template that silently drove it would move that
// gap somewhere harder to see. What it does instead is answer the question an
// operator actually has before applying anything: given these machines and
// these labels, what does this document say should happen?
//
// The format is YAML because the document is written by a person and read by a
// person, and it is deliberately small. Every field Omni's template format has
// that this one does not is a field that would need a behaviour behind it in
// this product, and a template that accepts what it cannot do is worse than one
// that refuses.
package clustertemplate

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"gopkg.in/yaml.v3"
)

// Kind is the document's own declaration of what it is.
//
// It is checked rather than assumed so that a file handed to the wrong route
// fails by name. A machine configuration and a cluster template are both YAML
// with a `cluster:` key near the top, and in this product they are two
// documents an operator has open at the same time.
const Kind = "ClusterTemplate"

// Template is a cluster as somebody wrote it down.
type Template struct {
	Kind string `yaml:"kind" json:"kind"`

	// Name is the cluster this describes. It is matched against an existing
	// cluster by name rather than by id, because the id is this product's own
	// and a person writing a file does not have it.
	Name string `yaml:"name" json:"name"`

	// TalosVersion and KubernetesVersion are what the cluster should run.
	// Empty means "whatever it runs now": a template that omits a version is
	// making no claim about it, which is different from claiming the current
	// one and then drifting silently when somebody upgrades.
	TalosVersion      string `yaml:"talosVersion,omitempty" json:"talos_version,omitempty"`
	KubernetesVersion string `yaml:"kubernetesVersion,omitempty" json:"kubernetes_version,omitempty"`

	// ControlPlane and Workers say where the nodes come from.
	ControlPlane NodeSet `yaml:"controlPlane" json:"control_plane"`
	Workers      NodeSet `yaml:"workers,omitempty" json:"workers"`
}

// NodeSet is one side of a cluster: which machines, and how many.
type NodeSet struct {
	// MachineClass names a class by its id. The machines are whatever that
	// class names at the moment the template is read, which is the whole
	// reason a template is worth more than a list of UUIDs.
	MachineClass string `yaml:"machineClass,omitempty" json:"machine_class,omitempty"`

	// Machines names machines directly, by UUID. It is here because a homelab
	// has three nodes and naming them is sometimes simply clearer than
	// inventing a label to select them with.
	Machines []model.MachineID `yaml:"machines,omitempty" json:"machines,omitempty"`

	// Count is how many machines from the class to use. Zero means all of
	// them.
	//
	// It exists because "three control-plane nodes from the class" is the
	// sentence people mean, and "every machine that happens to carry this
	// label" is the sentence a bare class says -- which grows a control plane
	// by one the next time somebody labels a machine.
	Count int `yaml:"count,omitempty" json:"count,omitempty"`
}

var (
	// ErrNotATemplate reports a document that is not one.
	ErrNotATemplate = errors.New("clustertemplate: that is not a cluster template")

	// ErrInvalid reports a template that cannot be acted on. It wraps every
	// reason at once.
	ErrInvalid = errors.New("clustertemplate: that template cannot be used")
)

// Parse reads a template and refuses one that is not usable.
//
// Refusing here rather than at the point of use is deliberate: a template is
// read once and referred to many times, and a problem discovered on the third
// reference is a problem discovered after two things already happened.
func Parse(raw []byte) (Template, error) {
	var t Template

	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	// Strict, so a misspelled field is a refusal rather than a silently
	// ignored line. This is the opposite of the decision internal/imagefactory
	// makes about the Factory's responses, and the difference is who wrote the
	// document: an unknown field from a third party is their addition, and an
	// unknown field here is this operator's typo.
	dec.KnownFields(true)

	if err := dec.Decode(&t); err != nil {
		return Template{}, fmt.Errorf("%w: %w", ErrNotATemplate, err)
	}
	if t.Kind != Kind {
		return Template{}, fmt.Errorf("%w: its kind is %q and this route reads %q",
			ErrNotATemplate, t.Kind, Kind)
	}

	if errs := t.validate(); len(errs) > 0 {
		return Template{}, fmt.Errorf("%w: %w", ErrInvalid, errors.Join(errs...))
	}
	return t, nil
}

func (t Template) validate() []error {
	var errs []error

	if strings.TrimSpace(t.Name) == "" {
		errs = append(errs, errors.New("the template names no cluster"))
	}

	errs = append(errs, t.ControlPlane.validate("controlPlane")...)
	errs = append(errs, t.Workers.validate("workers")...)

	if t.ControlPlane.empty() {
		errs = append(errs, errors.New("controlPlane names no machines and no class; a cluster "+
			"without a control plane is not a cluster"))
	}
	return errs
}

func (n NodeSet) empty() bool {
	return n.MachineClass == "" && len(n.Machines) == 0
}

func (n NodeSet) validate(field string) []error {
	var errs []error

	if n.MachineClass != "" && len(n.Machines) > 0 {
		errs = append(errs, fmt.Errorf("%s names both a class and a list of machines; one of them "+
			"would be ignored and nothing here says which", field))
	}
	if n.Count < 0 {
		errs = append(errs, fmt.Errorf("%s asks for %d machines", field, n.Count))
	}
	if n.Count > 0 && n.MachineClass == "" {
		errs = append(errs, fmt.Errorf("%s sets a count and names no class; a count picks from a "+
			"class, and a list of machines is already the count", field))
	}
	return errs
}

// Encode writes a template back out, for the export route.
func (t Template) Encode() ([]byte, error) {
	t.Kind = Kind

	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(t); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

// FromCluster derives a template from a cluster that already exists.
//
// The export is a list of machines and never a class, and that is a limitation
// stated rather than hidden: this product cannot know which of an operator's
// labels they *meant* as the reason a machine is in this cluster, and guessing
// one would produce a template that silently selects a different set the next
// time somebody labels something. The exported document is a faithful record of
// what is there now; turning it into a class is the operator's edit, and it is
// one line.
func FromCluster(c model.Cluster, machines []model.Machine) Template {
	t := Template{Kind: Kind, Name: c.Name}

	var control, workers []model.MachineID
	for _, m := range machines {
		if m.Cluster != c.ID {
			continue
		}
		if m.Role == model.RoleControlPlane {
			control = append(control, m.ID)
		} else {
			workers = append(workers, m.ID)
		}

		// The versions come from whatever the nodes report, and the first one
		// that reports anything wins. A cluster mid-upgrade has two answers
		// and the document can hold one; the plan below is what says the
		// cluster does not currently match its own export.
		if t.TalosVersion == "" {
			t.TalosVersion = m.Snapshot.TalosVersion
		}
		if t.KubernetesVersion == "" {
			t.KubernetesVersion = m.Snapshot.KubernetesVersion
		}
	}

	sortIDs(control)
	sortIDs(workers)

	t.ControlPlane = NodeSet{Machines: control}
	t.Workers = NodeSet{Machines: workers}
	return t
}

func sortIDs(ids []model.MachineID) {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
}
