package inventory_test

// Labels through the service, and the property that makes them worth having:
// nothing observed ever touches one.
//
// Every other field on a machine record is something the node said about
// itself, overwritten by the next observation. A label is something a person
// decided. If a refresh could clear or change one, a selector over labels would
// silently re-form its set whenever a node rebooted — and the thing on the
// other end of a selector is a cluster.

import (
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

func TestALabelSurvivesEveryObservation(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true, Bootstrapped: true})
	f.importCluster(ctx, t)

	machines, err := f.svc.Machines(ctx)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}
	id := machines[0].ID

	view, err := f.svc.SetLabels(ctx, id, map[string]string{"rack": "b3", "storage": "nvme"})
	if err != nil {
		t.Fatalf("SetLabels: %v", err)
	}
	if view.Labels["rack"] != "b3" {
		t.Fatalf("the view came back with labels %v", view.Labels)
	}

	// A full refresh: the widest thing that writes this record.
	f.svc.Refresh(ctx, id)

	after, err := f.svc.Machine(ctx, id)
	if err != nil {
		t.Fatalf("Machine: %v", err)
	}
	if after.Labels["rack"] != "b3" || after.Labels["storage"] != "nvme" {
		t.Errorf("an observation changed the operator's labels: %v. Nothing a node says may "+
			"touch one, or a selector over labels re-forms its set on a reboot", after.Labels)
	}
	// And the observation did happen, so the check above is about a label
	// surviving rather than about nothing having run.
	if after.TalosVersion.Value == "" {
		t.Error("the refresh recorded no Talos version, so this test did not observe anything")
	}
}

// TestSettingLabelsReplacesRatherThanMerges is the other half of the write
// contract, and the half an interface depends on: an operator who removes a row
// from a form must not find it still there afterwards.
func TestSettingLabelsReplacesRatherThanMerges(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true, Bootstrapped: true})
	f.importCluster(ctx, t)

	machines, _ := f.svc.Machines(ctx)
	id := machines[0].ID

	if _, err := f.svc.SetLabels(ctx, id, map[string]string{"rack": "b3", "owner": "ops"}); err != nil {
		t.Fatalf("SetLabels: %v", err)
	}
	view, err := f.svc.SetLabels(ctx, id, map[string]string{"rack": "b4"})
	if err != nil {
		t.Fatalf("SetLabels: %v", err)
	}

	if _, still := view.Labels["owner"]; still {
		t.Error("a label left out of the second write is still there. A merge cannot remove " +
			"anything, so an interface built on one needs a second operation to delete")
	}
	if view.Labels["rack"] != "b4" {
		t.Errorf("labels = %v, want rack b4", view.Labels)
	}

	// And clearing them entirely leaves nothing rather than an empty map that
	// round-trips as "{}".
	view, err = f.svc.SetLabels(ctx, id, nil)
	if err != nil {
		t.Fatalf("SetLabels(nil): %v", err)
	}
	if len(view.Labels) != 0 {
		t.Errorf("labels = %v after clearing them", view.Labels)
	}
}

func TestAMachineClassIsAQuestionAndNotAGroup(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true, Bootstrapped: true})
	f.importCluster(ctx, t)

	machines, _ := f.svc.Machines(ctx)
	id := machines[0].ID

	if _, err := f.svc.PutMachineClass(ctx, model.MachineClass{
		ID:       "rack-b",
		Name:     "Rack B",
		Selector: model.LabelSelector{Equals: map[string]string{"rack": "b3"}},
	}); err != nil {
		t.Fatalf("PutMachineClass: %v", err)
	}

	// Nothing is labelled yet, so the class names nothing. That is the
	// interesting state and it must not be an error.
	classes, err := f.svc.MachineClasses(ctx)
	if err != nil {
		t.Fatalf("MachineClasses: %v", err)
	}
	if len(classes) != 1 {
		t.Fatalf("got %d classes, want the one that was just written", len(classes))
	}
	matched, err := f.svc.MachinesMatching(ctx, classes[0].Selector)
	if err != nil {
		t.Fatalf("MachinesMatching: %v", err)
	}
	if len(matched) != 0 {
		t.Fatalf("the class already names %d machines and nothing is labelled", len(matched))
	}

	// A machine joins by being labelled, without anything touching the class.
	if _, err := f.svc.SetLabels(ctx, id, map[string]string{"rack": "b3"}); err != nil {
		t.Fatalf("SetLabels: %v", err)
	}
	matched, _ = f.svc.MachinesMatching(ctx, classes[0].Selector)
	if len(matched) != 1 || matched[0].ID != id {
		t.Errorf("the class names %v after the machine was labelled, want just %s", matched, id)
	}

	// And leaves by being unlabelled, likewise.
	if _, err := f.svc.SetLabels(ctx, id, map[string]string{"rack": "b4"}); err != nil {
		t.Fatalf("SetLabels: %v", err)
	}
	matched, _ = f.svc.MachinesMatching(ctx, classes[0].Selector)
	if len(matched) != 0 {
		t.Errorf("the class still names %v after the label changed. A class is a question "+
			"re-answered on every read, not a group somebody is added to", matched)
	}
}

func TestAClassWithNoConditionsIsRefused(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true, Bootstrapped: true})

	_, err := f.svc.PutMachineClass(ctx, model.MachineClass{ID: "everything", Name: "Everything"})
	if err == nil {
		t.Fatal("a class with no conditions was stored. Matching nothing makes it useless and " +
			"matching everything makes it dangerous; refusing it is the only answer that is neither")
	}
	if !strings.Contains(err.Error(), "condition") {
		t.Errorf("the refusal %q does not say what is missing", err)
	}
}

func TestALabelThatCannotBeReadIsRefused(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true, Bootstrapped: true})
	f.importCluster(ctx, t)

	machines, _ := f.svc.Machines(ctx)
	id := machines[0].ID

	_, err := f.svc.SetLabels(ctx, id, map[string]string{"rack ": "b3"})
	if err == nil {
		t.Fatal("a label key with a trailing space was stored. It is invisible on a screen, " +
			"which makes two labels that look identical different")
	}
	if !strings.Contains(err.Error(), "space") {
		t.Errorf("the refusal %q does not say what is wrong", err)
	}

	// And the record was not touched.
	after, _ := f.svc.Machine(ctx, id)
	if len(after.Labels) != 0 {
		t.Errorf("a refused write left labels behind: %v", after.Labels)
	}
}
