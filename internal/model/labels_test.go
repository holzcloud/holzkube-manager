package model_test

// Selectors, and the one decision in them that can cost a cluster.
//
// An empty selector matches nothing. Every other reading of "no conditions" is
// "everything", which is the reading a query language usually takes -- and the
// operation on the other end of a machine class is a cluster, so a selector
// that was cleared by accident must not quietly become the whole fleet.

import (
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

func TestAnEmptySelectorMatchesNothing(t *testing.T) {
	t.Parallel()

	var empty model.LabelSelector
	if !empty.Empty() {
		t.Fatal("a zero selector does not report itself as empty")
	}

	for name, labels := range map[string]map[string]string{
		"a labelled machine": {"rack": "b3"},
		"a bare machine":     {},
		"nil labels":         nil,
	} {
		if empty.Matches(labels) {
			t.Errorf("an empty selector matched %s. Every reading of 'no conditions' other than "+
				"'nothing' turns a cleared selector into the whole fleet", name)
		}
	}

	if !strings.Contains(empty.Sentence(), "no machines") {
		t.Errorf("the sentence for an empty selector is %q, which does not say what it matches",
			empty.Sentence())
	}
}

func TestASelectorIsAllOfItsConditions(t *testing.T) {
	t.Parallel()

	sel := model.LabelSelector{
		Equals:  map[string]string{"rack": "b3", "storage": "nvme"},
		Present: []string{"owner"},
		Absent:  []string{"decommissioned"},
	}

	for name, tc := range map[string]struct {
		labels map[string]string
		match  bool
	}{
		"everything holds": {
			map[string]string{"rack": "b3", "storage": "nvme", "owner": "ops"}, true,
		},
		"a value differs": {
			map[string]string{"rack": "b4", "storage": "nvme", "owner": "ops"}, false,
		},
		"a required label is missing": {
			map[string]string{"rack": "b3", "storage": "nvme"}, false,
		},
		"a forbidden label is set": {
			map[string]string{"rack": "b3", "storage": "nvme", "owner": "ops", "decommissioned": "yes"}, false,
		},
		"a forbidden label set to empty is still set": {
			map[string]string{"rack": "b3", "storage": "nvme", "owner": "ops", "decommissioned": ""}, false,
		},
		"presence does not care about the value": {
			map[string]string{"rack": "b3", "storage": "nvme", "owner": ""}, true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := sel.Matches(tc.labels); got != tc.match {
				t.Errorf("Matches(%v) = %v, want %v", tc.labels, got, tc.match)
			}
		})
	}
}

// TestTheSentenceNamesEveryCondition keeps the description from falling behind
// the selector, which is how a screen comes to describe a set it is not
// selecting.
func TestTheSentenceNamesEveryCondition(t *testing.T) {
	t.Parallel()

	sel := model.LabelSelector{
		Equals:  map[string]string{"rack": "b3"},
		Present: []string{"owner"},
		Absent:  []string{"decommissioned"},
	}

	got := sel.Sentence()
	for _, want := range []string{"rack", "b3", "owner", "is set", "decommissioned", "is not set"} {
		if !strings.Contains(got, want) {
			t.Errorf("the sentence %q does not mention %q", got, want)
		}
	}
}

func TestALabelSetIsRefusedForEveryReasonAtOnce(t *testing.T) {
	t.Parallel()

	// Two problems in one set, so a screen shows both rather than teaching the
	// operator one round trip at a time. The second value carries a bell
	// character, built rather than typed so this file stays readable.
	errs := model.ValidateLabels(map[string]string{
		" leading-space": "fine",
		"bell":           "ring" + string(rune(7)),
	})
	if len(errs) != 2 {
		t.Fatalf("got %d errors, want one for each problem: %v", len(errs), errs)
	}

	joined := ""
	for _, err := range errs {
		joined += err.Error() + "\n"
	}
	if !strings.Contains(joined, "space") {
		t.Error("the refusals do not mention the leading space, which is invisible on a screen " +
			"and is the reason two identical-looking labels are not")
	}
	if !strings.Contains(joined, "printable") {
		t.Error("the refusals do not mention the control character")
	}

	if errs := model.ValidateLabels(map[string]string{"rack": "b3"}); len(errs) != 0 {
		t.Errorf("an ordinary label was refused: %v", errs)
	}

	tooMany := map[string]string{}
	for i := range model.MaxLabels + 1 {
		tooMany[string(rune('a'+i%26))+string(rune('0'+i/26))] = "x"
	}
	if errs := model.ValidateLabels(tooMany); len(errs) == 0 {
		t.Errorf("%d labels were accepted and the limit is %d", len(tooMany), model.MaxLabels)
	}
}
