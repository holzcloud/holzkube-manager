// Package planning holds no code. It exists for this one test.
//
// .planning/WINDOWS.md is the broken-windows ledger, and it carries every entry
// twice: once as a row in a markdown table, which is what a person reads, and
// once in a JSON block, which is what `gsd-tools windows` reads and writes. Two
// copies of the same record is a design that only works while something checks
// that they agree, and for a long time nothing did.
//
// They had drifted, in a way that is worse than a contradiction because it does
// not look like one: 23 entries had their description cut short in the markdown
// half -- at 400, then 350, then 260 characters, a shrinking budget nobody
// wrote down -- and four also had their resolution cut. Every cut was a clean
// prefix with no ellipsis and no marker, so a sentence ending mid-word read as
// a sentence that had finished. The JSON kept the whole text. A reader of the
// ledger, which is the human half, was reading a summary and could not tell.
//
// The counts in the front matter are the third copy, and `/gsd-ship` blocks on
// one of them.
package planning_test

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const ledgerPath = "../../.planning/WINDOWS.md"

// window is one entry as the JSON block spells it.
type window struct {
	ID          int    `json:"id"`
	Kind        string `json:"kind"`
	Phase       string `json:"phase"`
	File        string `json:"file"`
	Line        *int   `json:"line"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Reason      string `json:"reason"`
	RecordedAt  string `json:"recorded_at"`
	ResolvedAt  string `json:"resolved_at"`
}

// cell is the ONE rule for putting a value in a markdown table cell, and the
// reason this test can compare exactly rather than approximately.
//
// A pipe would end the cell and a newline would end the row, so those two are
// handled and nothing else is. An earlier version of the file also escaped
// backslashes and asterisks in two entries and not in the rest, which is the
// kind of near-agreement that forces a guard to compare loosely -- and a guard
// that compares loosely has a hole exactly the size of its tolerance.
func cell(v string) string {
	return strings.NewReplacer("|", `\|`, "\n", " ").Replace(v)
}

func (w window) line() string {
	if w.Line == nil {
		return ""
	}
	return strconv.Itoa(*w.Line)
}

func (w window) row() string {
	return "| " + strings.Join([]string{
		strconv.Itoa(w.ID), w.Phase, w.Kind, cell(w.File), w.line(),
		cell(w.Description), w.Status, cell(w.Reason),
		cell(w.RecordedAt), cell(w.ResolvedAt),
	}, " | ") + " |"
}

var (
	jsonBlock = regexp.MustCompile("(?s)````json\n(.*?)\n````")
	rowStart  = regexp.MustCompile(`^\| (\d+) \|`)
	frontKey  = regexp.MustCompile(`(?m)^(\w+): (.+)$`)
)

// ledger reads the file once and returns its three halves.
func ledger(t *testing.T) (front map[string]string, rows map[int]string, order []int, entries []window) {
	t.Helper()

	raw, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatalf("read the ledger: %v", err)
	}
	text := string(raw)

	parts := strings.SplitN(text, "---\n", 3)
	if len(parts) < 3 {
		t.Fatal("the ledger has no YAML front matter")
	}
	front = map[string]string{}
	for _, m := range frontKey.FindAllStringSubmatch(parts[1], -1) {
		front[m[1]] = strings.TrimSpace(m[2])
	}

	block := jsonBlock.FindStringSubmatch(text)
	if block == nil {
		t.Fatal("the ledger has no ````json block")
	}
	if err := json.Unmarshal([]byte(block[1]), &entries); err != nil {
		t.Fatalf("decode the JSON block: %v", err)
	}

	rows = map[int]string{}
	for _, line := range strings.Split(text, "\n") {
		m := rowStart.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		id, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("row id %q: %v", m[1], err)
		}
		if _, seen := rows[id]; seen {
			t.Errorf("entry %d appears in the table twice", id)
		}
		rows[id] = line
		order = append(order, id)
	}
	return front, rows, order, entries
}

// TestTheLedgersTwoHalvesSayTheSameThing compares the whole row, not a chosen
// set of fields.
//
// That choice is the point of the test. An earlier check of mine compared
// status, file, kind and phase -- and passed, on a file where 23 descriptions
// were truncated, because description was not one of the four. A guard that
// looks at some fields reports on some drift; the fields it does not read are
// exactly where drift accumulates unseen.
func TestTheLedgersTwoHalvesSayTheSameThing(t *testing.T) {
	t.Parallel()

	_, rows, _, entries := ledger(t)

	if len(rows) != len(entries) {
		t.Errorf("the table has %d rows and the JSON block has %d entries", len(rows), len(entries))
	}

	seen := map[int]bool{}
	for _, w := range entries {
		if seen[w.ID] {
			t.Errorf("entry %d appears in the JSON block twice", w.ID)
		}
		seen[w.ID] = true

		row, ok := rows[w.ID]
		if !ok {
			t.Errorf("entry %d is in the JSON block and not in the table", w.ID)
			continue
		}
		if row != w.row() {
			t.Errorf("entry %d differs between the table and the JSON block:\n  table: %s\n   json: %s",
				w.ID, truncate(row), truncate(w.row()))
		}
	}
	for id := range rows {
		if !seen[id] {
			t.Errorf("entry %d is in the table and not in the JSON block", id)
		}
	}
}

// TestTheFrontMatterCountsWhatIsThere.
//
// `/gsd-ship` blocks while open_count is above zero, so this number decides
// whether the project believes it can release. A hand-maintained count is a
// claim about a list, and this is where it is held against the list.
func TestTheFrontMatterCountsWhatIsThere(t *testing.T) {
	t.Parallel()

	front, _, _, entries := ledger(t)

	counts := map[string]int{"open": 0, "waived": 0, "fixed": 0}
	for _, w := range entries {
		if _, known := counts[w.Status]; !known {
			t.Errorf("entry %d has status %q, which is not one of open, waived or fixed",
				w.ID, w.Status)
			continue
		}
		counts[w.Status]++
	}

	for key, want := range map[string]int{
		"open_count":   counts["open"],
		"waived_count": counts["waived"],
		"fixed_count":  counts["fixed"],
		"total_count":  len(entries),
	} {
		got, err := strconv.Atoi(front[key])
		if err != nil {
			t.Errorf("front matter %s = %q: %v", key, front[key], err)
			continue
		}
		if got != want {
			t.Errorf("front matter says %s: %d, the ledger holds %d", key, got, want)
		}
	}
}

// TestEveryEntryIsResolvedExactlyWhenItSaysItIs.
//
// The two fields disagree in one direction silently: an entry marked fixed with
// no resolved_at reads, in every listing that sorts or filters by date, as an
// entry nobody has closed -- and the status column says otherwise on the same
// row.
func TestEveryEntryIsResolvedExactlyWhenItSaysItIs(t *testing.T) {
	t.Parallel()

	_, _, _, entries := ledger(t)

	for _, w := range entries {
		switch w.Status {
		case "open":
			if w.ResolvedAt != "" {
				t.Errorf("entry %d is open and carries resolved_at %q", w.ID, w.ResolvedAt)
			}
		case "fixed", "waived":
			if w.ResolvedAt == "" {
				t.Errorf("entry %d is %s and carries no resolved_at", w.ID, w.Status)
			}
			if w.Status == "waived" && strings.TrimSpace(w.Reason) == "" {
				t.Errorf("entry %d is waived with no reason; a waiver without one is a deletion",
					w.ID)
			}
		}
		if strings.TrimSpace(w.Description) == "" {
			t.Errorf("entry %d has no description", w.ID)
		}
		if w.RecordedAt == "" {
			t.Errorf("entry %d has no recorded_at", w.ID)
		}
	}
}

// TestTheTableIsInTheOrderTheIdsWereMinted keeps the two halves readable side
// by side: an entry appended to one and inserted into the other still compares
// equal field by field, and a reader following both loses their place.
func TestTheTableIsInTheOrderTheIdsWereMinted(t *testing.T) {
	t.Parallel()

	_, _, order, entries := ledger(t)

	fromJSON := make([]int, 0, len(entries))
	for _, w := range entries {
		fromJSON = append(fromJSON, w.ID)
	}
	if fmt.Sprint(order) != fmt.Sprint(fromJSON) {
		t.Errorf("the table and the JSON block list entries in different orders:\n  table: %v\n   json: %v",
			order, fromJSON)
	}
	for i := 1; i < len(order); i++ {
		if order[i] <= order[i-1] {
			t.Errorf("entry %d follows %d; the ledger is append-only and ids only go up",
				order[i], order[i-1])
		}
	}
}

// truncate keeps a failure message readable. Entries run to several thousand
// characters, and the difference is almost always near the end.
func truncate(s string) string {
	const keep = 160
	if len(s) <= keep*2 {
		return s
	}
	return s[:keep] + " …[" + strconv.Itoa(len(s)-keep*2) + " more]… " + s[len(s)-keep:]
}
