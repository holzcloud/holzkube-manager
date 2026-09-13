package clustertemplate_test

// A template is read once and referred to many times, so everything that can be
// refused is refused at Parse. The tests below are mostly about refusals, and
// each one names a mistake that would otherwise produce a cluster somebody did
// not mean.

import (
	"errors"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/clustertemplate"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

const good = `
kind: ClusterTemplate
name: homelab
talosVersion: v1.13.9
kubernetesVersion: v1.34.0
controlPlane:
  machineClass: rack-b
  count: 3
workers:
  machines:
    - 00000000-0000-4000-8000-000000000009
`

func TestAGoodTemplateParses(t *testing.T) {
	t.Parallel()

	tpl, err := clustertemplate.Parse([]byte(good))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tpl.Name != "homelab" || tpl.ControlPlane.MachineClass != "rack-b" || tpl.ControlPlane.Count != 3 {
		t.Fatalf("parsed as %+v", tpl)
	}
	if len(tpl.Workers.Machines) != 1 {
		t.Errorf("workers = %+v, want the one machine it names", tpl.Workers)
	}
}

func TestATemplateIsRefusedForEveryReasonItShouldBe(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		doc  string
		want string
	}{
		"not a template at all": {
			doc:  "kind: MachineConfig\nname: homelab\n",
			want: "kind",
		},
		"a misspelled field": {
			// Strict decoding, unlike the Factory's responses, and the
			// difference is who wrote the document: an unknown field from a
			// third party is their addition and this one is a typo.
			doc:  "kind: ClusterTemplate\nname: homelab\ncontrolPlain:\n  machines: [x]\n",
			want: "controlPlain",
		},
		"no cluster name": {
			doc:  "kind: ClusterTemplate\ncontrolPlane:\n  machines: [x]\n",
			want: "names no cluster",
		},
		"no control plane": {
			doc:  "kind: ClusterTemplate\nname: homelab\n",
			want: "not a cluster",
		},
		"a class and a list at once": {
			doc: "kind: ClusterTemplate\nname: homelab\ncontrolPlane:\n" +
				"  machineClass: rack-b\n  machines: [x]\n",
			want: "one of them would be ignored",
		},
		"a count with no class": {
			doc:  "kind: ClusterTemplate\nname: homelab\ncontrolPlane:\n  machines: [x]\n  count: 3\n",
			want: "count and names no class",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := clustertemplate.Parse([]byte(tc.doc))
			if err == nil {
				t.Fatal("the template was accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal %q does not mention %q", err, tc.want)
			}
		})
	}
}

func fleet() clustertemplate.Fleet {
	labelled := func(id, rack string) model.Machine {
		return model.Machine{ID: model.MachineID(id), Labels: map[string]string{"rack": rack}}
	}
	return clustertemplate.Fleet{
		Machines: []model.Machine{
			labelled("m1", "b"), labelled("m2", "b"), labelled("m3", "b"), labelled("m9", "c"),
		},
		Classes: []model.MachineClass{{
			ID:       "rack-b",
			Name:     "Rack B",
			Selector: model.LabelSelector{Equals: map[string]string{"rack": "b"}},
		}},
	}
}

func TestAPlanResolvesAClassAgainstTheFleetAsItIs(t *testing.T) {
	t.Parallel()

	tpl, err := clustertemplate.Parse([]byte(
		"kind: ClusterTemplate\nname: homelab\ncontrolPlane:\n  machineClass: rack-b\n  count: 3\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	plan := clustertemplate.Make(tpl, fleet())
	if !plan.Usable() {
		t.Fatalf("the plan reports problems: %v", plan.Problems)
	}
	if len(plan.ControlPlane.Machines) != 3 {
		t.Fatalf("the control plane resolves to %v, want three machines", plan.ControlPlane.Machines)
	}
	// Deterministic, so reading the plan twice gives the same answer.
	again := clustertemplate.Make(tpl, fleet())
	if plan.ControlPlane.Machines[0] != again.ControlPlane.Machines[0] {
		t.Error("two reads of one template picked different machines")
	}
	if !strings.Contains(plan.Sentence, "3 control-plane") {
		t.Errorf("the sentence %q does not say what would happen", plan.Sentence)
	}
}

// TestAClassThatNamesTooFewMachinesIsAProblemAndSaysWhy is the case a template
// exists to catch before anything is applied.
func TestAClassThatNamesTooFewMachinesIsAProblemAndSaysWhy(t *testing.T) {
	t.Parallel()

	tpl, _ := clustertemplate.Parse([]byte(
		"kind: ClusterTemplate\nname: homelab\ncontrolPlane:\n  machineClass: rack-b\n  count: 5\n"))

	plan := clustertemplate.Make(tpl, fleet())
	if plan.Usable() {
		t.Fatal("a template asking for five machines from a class of three was reported as usable")
	}

	joined := strings.Join(plan.Problems, "\n")
	for _, want := range []string{"5", "3", "rack"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the problem %q does not mention %q, so nothing says where to look", joined, want)
		}
	}
}

func TestAMachineNamedOnBothSidesIsRefused(t *testing.T) {
	t.Parallel()

	tpl, _ := clustertemplate.Parse([]byte(
		"kind: ClusterTemplate\nname: homelab\ncontrolPlane:\n  machines: [m1]\nworkers:\n  machines: [m1]\n"))

	plan := clustertemplate.Make(tpl, fleet())
	if plan.Usable() {
		t.Fatal("a machine named as both a control-plane node and a worker was accepted. Two " +
			"lists of UUIDs is exactly where that mistake lives")
	}
	if !strings.Contains(strings.Join(plan.Problems, "\n"), "both") {
		t.Errorf("the problems %v do not say the machine appears twice", plan.Problems)
	}
}

func TestAnEvenControlPlaneIsANoteAndNotARefusal(t *testing.T) {
	t.Parallel()

	tpl, _ := clustertemplate.Parse([]byte(
		"kind: ClusterTemplate\nname: homelab\ncontrolPlane:\n  machines: [m1, m2]\n"))

	plan := clustertemplate.Make(tpl, fleet())
	if !plan.Usable() {
		t.Fatalf("an even control plane was refused: %v. Somebody may be mid-change, and a "+
			"refusal there is a product telling an operator they cannot do what they are doing",
			plan.Problems)
	}
	if !strings.Contains(strings.Join(plan.Notes, "\n"), "tolerates") {
		t.Errorf("the notes %v do not mention what an even control plane costs", plan.Notes)
	}
}

func TestAnExportRoundTrips(t *testing.T) {
	t.Parallel()

	cluster := model.Cluster{ID: "c1", Name: "homelab"}
	machines := []model.Machine{
		{ID: "cp2", Cluster: "c1", Role: model.RoleControlPlane},
		{ID: "cp1", Cluster: "c1", Role: model.RoleControlPlane,
			Snapshot: model.MachineSnapshot{TalosVersion: "v1.13.9", KubernetesVersion: "v1.34.0"}},
		{ID: "w1", Cluster: "c1", Role: model.RoleWorker},
		{ID: "other", Cluster: "c2", Role: model.RoleControlPlane},
	}

	raw, err := clustertemplate.FromCluster(cluster, machines).Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// The export is a template, which is the only claim worth making about it:
	// an export that does not parse is a file that looks like a backup.
	back, err := clustertemplate.Parse(raw)
	if err != nil {
		t.Fatalf("the exported template does not parse: %v\n%s", err, raw)
	}

	if back.Name != "homelab" {
		t.Errorf("name = %q", back.Name)
	}
	if len(back.ControlPlane.Machines) != 2 || len(back.Workers.Machines) != 1 {
		t.Fatalf("the export names %v and %v", back.ControlPlane.Machines, back.Workers.Machines)
	}
	// Sorted, so two exports of one cluster are the same file and a diff of
	// them means something.
	if back.ControlPlane.Machines[0] != "cp1" {
		t.Errorf("the control-plane machines are %v, want them sorted", back.ControlPlane.Machines)
	}
	// The other cluster's node is not in it.
	for _, id := range append(back.ControlPlane.Machines, back.Workers.Machines...) {
		if id == "other" {
			t.Error("the export contains a machine from a different cluster")
		}
	}
}

func TestParseReportsWhichKindOfFailureItWas(t *testing.T) {
	t.Parallel()

	// The two are different things to do about it: a document of the wrong
	// kind was probably handed to the wrong route, and an invalid one needs
	// editing.
	if _, err := clustertemplate.Parse([]byte("kind: MachineConfig\n")); !errors.Is(err, clustertemplate.ErrNotATemplate) {
		t.Errorf("a machine config returned %v, want ErrNotATemplate", err)
	}
	if _, err := clustertemplate.Parse([]byte("kind: ClusterTemplate\nname: x\n")); !errors.Is(err, clustertemplate.ErrInvalid) {
		t.Errorf("a template with no control plane returned %v, want ErrInvalid", err)
	}
}
