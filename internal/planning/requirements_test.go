package planning_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The v1.14 requirements, with their sixteen release blockers, are archived
// since milestone v1.18 started a REQUIREMENTS.md of its own. The archive is
// what the blocker check below reads; the current file is held to the same
// bare-id rule by TestTheCurrentRequirementsAreReachableByTheTool.
const (
	requirementsPath        = "../../.planning/milestones/v1.14-REQUIREMENTS.md"
	currentRequirementsPath = "../../.planning/REQUIREMENTS.md"
)

var (
	checklistBlocker = regexp.MustCompile(`(?m)^- \[[ x]\] \*\*([A-Z0-9]+-\d+)\*\* 🚫`)
	bareRequirement  = regexp.MustCompile(`^[A-Z0-9]+-\d+$`)
)

// Ledger 73: the planning tool's requirements revert-phase finds a
// traceability row by comparing its first cell to the bare id, so a cell that
// carried markup -- "**TRANS-06** 🚫" -- was invisible to it, and the one
// release blocker of its phase went on reading Complete after two reverts had
// reached every undecorated row. The id cell is therefore bare, the blocker
// mark lives in a column of its own, and this test holds both: the tool can
// reach every row, and the table marks exactly the requirements the checklist
// marks.
func TestTheTraceabilityTableIsReachableByTheTool(t *testing.T) {
	raw, err := os.ReadFile(requirementsPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)

	want := map[string]bool{}
	for _, m := range checklistBlocker.FindAllStringSubmatch(text, -1) {
		want[m[1]] = true
	}
	if len(want) == 0 {
		t.Fatal("the checklist marks no release blocker; the pattern no longer reads it")
	}

	start := strings.Index(text, "| Requirement | Phase | Status | Blocker |")
	if start < 0 {
		t.Fatal("no traceability table with a Blocker column")
	}
	rows := 0
	got := map[string]bool{}
	for _, line := range strings.Split(text[start:], "\n")[2:] {
		if !strings.HasPrefix(line, "|") {
			break
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) != 4 {
			t.Errorf("row %q has %d cells, want 4", line, len(cells))
			continue
		}
		id := strings.TrimSpace(cells[0])
		rows++
		if !bareRequirement.MatchString(id) {
			t.Errorf("id cell %q carries more than the id; the planning tool compares the whole "+
				"cell to the id and cannot reach this row", id)
		}
		if strings.TrimSpace(cells[3]) == "🚫" {
			got[strings.Trim(id, "* 🚫")] = true
		}
	}
	if rows < 50 {
		t.Fatalf("read only %d rows; the walk is not reaching the table", rows)
	}
	for id := range want {
		if !got[id] {
			t.Errorf("%s is a release blocker in the checklist and not in the table", id)
		}
	}
	for id := range got {
		if !want[id] {
			t.Errorf("%s is marked a blocker in the table and not in the checklist", id)
		}
	}
}

// The planning tool reaches a row of the current milestone's traceability
// table the same way it reached the v1.14 one (ledger 73), so the id cell of
// every row there is bare as well.
func TestTheCurrentRequirementsAreReachableByTheTool(t *testing.T) {
	raw, err := os.ReadFile(currentRequirementsPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	start := strings.Index(text, "| Requirement | Phase | Status |")
	if start < 0 {
		t.Fatal("no traceability table")
	}
	rows := 0
	for _, line := range strings.Split(text[start:], "\n")[2:] {
		if !strings.HasPrefix(line, "|") {
			break
		}
		id := strings.TrimSpace(strings.Split(strings.Trim(line, "|"), "|")[0])
		rows++
		if !bareRequirement.MatchString(id) {
			t.Errorf("id cell %q carries more than the id; the planning tool cannot reach this row", id)
		}
	}
	if rows == 0 {
		t.Fatal("the traceability table has no rows; the walk is not reaching it")
	}
}
