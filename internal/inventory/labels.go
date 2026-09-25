package inventory

// Machine labels and the named selectors over them (Omni parity phase 5).
//
// A label is the operator's own word about a machine. Nothing observed ever
// touches one -- see model.Machine.Labels for why that separation is what makes
// a label safe to select on -- so the write path here is its own, short, and
// does not go anywhere near the snapshot.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/clustertemplate"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/scale"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// ErrInvalidLabels reports a label set that cannot be stored. It wraps every
// reason at once, so a screen shows them together.
var ErrInvalidLabels = errors.New("inventory: those labels cannot be stored")

// ErrSelectorEmpty reports a machine class that would match nothing.
//
// It is refused rather than stored, and that is the same decision
// LabelSelector.Matches makes from the other side: a class with no conditions
// is almost always a selector somebody cleared by accident, and the operation
// on the other end of one is a cluster.
var ErrSelectorEmpty = errors.New("inventory: a machine class needs at least one condition")

// SetLabels replaces a machine's labels.
//
// Replaces, not merges. A merge cannot remove a label, so an interface built on
// one needs a second operation to delete, and an operator who removed a row
// from a form would find it still there afterwards. The whole set goes in and
// the whole set comes out.
func (s *Service) SetLabels(ctx context.Context, id model.MachineID, labels map[string]string) (MachineView, error) {
	if errs := model.ValidateLabels(labels); len(errs) > 0 {
		return MachineView{}, fmt.Errorf("%w: %w", ErrInvalidLabels, errors.Join(errs...))
	}

	for range writeAttempts {
		rec, err := s.deps.Store.Machines().Get(ctx, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return MachineView{}, ErrNotFound
			}
			return MachineView{}, err
		}

		// Cloned rather than assigned: the caller's map is theirs, and a
		// record that shared it would change when they next touched it.
		rec.Labels = maps.Clone(labels)
		if len(rec.Labels) == 0 {
			// nil rather than an empty map, so the record round-trips through
			// JSON as the absence it is instead of as "{}".
			rec.Labels = nil
		}

		saved, err := s.deps.Store.Machines().Put(ctx, rec)
		switch {
		case err == nil:
			return s.viewOf(saved), nil
		case errors.Is(err, store.ErrConflict):
			continue
		default:
			return MachineView{}, err
		}
	}
	return MachineView{}, fmt.Errorf("%w: the machine record changed under %d successive attempts",
		store.ErrConflict, writeAttempts)
}

// MachineClasses lists the named selectors.
func (s *Service) MachineClasses(ctx context.Context) ([]model.MachineClass, error) {
	return s.deps.Store.MachineClasses().List(ctx)
}

// PutMachineClass creates or replaces a named selector.
func (s *Service) PutMachineClass(ctx context.Context, c model.MachineClass) (model.MachineClass, error) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return model.MachineClass{}, fmt.Errorf("%w: a machine class needs a name", ErrInvalidLabels)
	}
	if c.Selector.Empty() {
		return model.MachineClass{}, ErrSelectorEmpty
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = s.deps.Now().UTC()
	}
	return s.deps.Store.MachineClasses().Put(ctx, c)
}

// DeleteMachineClass removes a named selector.
func (s *Service) DeleteMachineClass(ctx context.Context, id model.MachineClassID) error {
	if err := s.deps.Store.MachineClasses().Delete(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// MachinesMatching answers a selector against the inventory, now.
//
// Now, and not at the moment the class was written. A class is a question and
// not a group: a machine joins it by being labelled and leaves by being
// unlabelled, which is one place to look when the membership is not what
// somebody expected.
func (s *Service) MachinesMatching(ctx context.Context, sel model.LabelSelector) ([]MachineView, error) {
	machines, err := s.presentMachines(ctx)
	if err != nil {
		return nil, err
	}

	views := make([]MachineView, 0, len(machines))
	for _, m := range machines {
		if sel.Matches(m.Labels) {
			views = append(views, s.viewOf(m))
		}
	}
	return views, nil
}

// Fleet is everything a cluster template is planned against.
//
// One method and one read of each entity, because a plan is a statement about
// one moment: three separate calls could see a machine labelled in the second
// and a class rewritten in the third, and produce a plan that was never true of
// anything.
func (s *Service) Fleet(ctx context.Context) (clustertemplate.Fleet, error) {
	clusters, err := s.deps.Store.Clusters().List(ctx)
	if err != nil {
		return clustertemplate.Fleet{}, err
	}
	machines, err := s.presentMachines(ctx)
	if err != nil {
		return clustertemplate.Fleet{}, err
	}
	classes, err := s.deps.Store.MachineClasses().List(ctx)
	if err != nil {
		return clustertemplate.Fleet{}, err
	}
	return clustertemplate.Fleet{Clusters: clusters, Machines: machines, Classes: classes}, nil
}

// ClusterAndMachines reads a cluster and every machine in it, for the export.
func (s *Service) ClusterAndMachines(ctx context.Context, id model.ClusterID) (model.Cluster, []model.Machine, error) {
	cluster, err := s.deps.Store.Clusters().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return model.Cluster{}, nil, ErrNotFound
		}
		return model.Cluster{}, nil, err
	}
	machines, err := s.presentMachines(ctx)
	if err != nil {
		return model.Cluster{}, nil, err
	}
	return cluster, machines, nil
}

// ScaleInput assembles everything a scale plan is computed from except the
// etcd membership.
//
// Except the membership, deliberately: reading it means opening a client to a
// control-plane node, which is the upgrade service's job and not this one's.
// The caller puts the two halves together. That is the composition root doing
// composition rather than this package growing a reason to dial a node.
//
// Stage is carried alongside each record because internal/scale reaches
// nothing and so cannot compute it: it comes from live observation held here.
func (s *Service) ScaleInput(ctx context.Context, id model.ClusterID) (scale.Input, error) {
	cluster, err := s.deps.Store.Clusters().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return scale.Input{}, ErrNotFound
		}
		return scale.Input{}, err
	}

	machines, err := s.presentMachines(ctx)
	if err != nil {
		return scale.Input{}, err
	}

	in := scale.Input{Cluster: cluster}
	for _, rec := range machines {
		stage, _ := s.status(rec.ID, rec.Snapshot.ObservedAt)
		node := scale.Node{Machine: rec, Stage: stage}

		switch rec.Cluster {
		case id:
			in.Nodes = append(in.Nodes, node)
		case "":
			// A machine belonging to no cluster is a candidate for this one.
			// A machine belonging to a *different* cluster is neither, and is
			// left out rather than listed as unavailable: it is not this
			// cluster's business.
			in.Spare = append(in.Spare, node)
		}
	}
	return in, nil
}
