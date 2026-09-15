package imagefactory_test

import (
	"fmt"
	"strings"
	"testing"
)

// The written-down remainder of carriedAsStringLiteral, the reader behind
// TestWarningDetailsMatchTheUI, in the form plan 02-30 established for
// guardBlindSpots: one entry per MECHANISM, each carrying the output that
// evidences it, and a row that measures it over the live reader.
//
// Ledger entry 74 is what this closes for this reader, the most permeable of
// the three it names: before the change it was a bare strings.Contains with no
// anchor and no word boundary at all. The prefix hole is CLOSED rather than
// excluded -- the entry is absent from the list below for that reason -- and the
// closure was measured against the fault reinstated in the REAL tree:
// web/src/api.ts:507's mirror renamed to 'installer.repo-fallback-unverified-v2'
// while the Go constant kept its name. The old strings.Contains reported true
// over that tree; the anchored reader reports false and
// TestWarningDetailsMatchTheUI goes red.
//
// The measurement also produced a finding against the first version of the fix,
// and it is recorded here because an unrecorded attempt cannot be told apart
// from an omitted one: accepting the BACKTICK as a third delimiter left the
// guard green over that same injection, because api.ts:445 carries the code as a
// markdown code span in a JSDoc comment. The backtick delimits a template
// literal in code and a code span in prose, and this file has far more of the
// second kind.

type warningReaderBlindSpot struct {
	// id is the stable name a row refers to.
	id string
	// mechanism says in one line why the reader is blind here, in terms of what
	// it binds to rather than a syntax it fails to parse.
	mechanism string
	// measured carries the output that evidences the entry. An entry without a
	// measurement is a claim, and claims are what this guard exists to stop.
	measured string
}

var warningReaderBlindSpots = []warningReaderBlindSpot{
	{
		id: "quoted-literal-text-not-code",
		mechanism: "it requires a DELIMITER around the value and asks nothing about line " +
			"context -- a delimiter is a word boundary, not a statement -- so a correctly " +
			"quoted literal anywhere in the text satisfies it",
		measured: "'installer.repo-fallback-unverified' quoted inside a // line comment, " +
			"inside a /* */ block, inside a template literal and as JSX text between " +
			"<pre> tags each read READ; the same code as a markdown code span in a JSDoc " +
			"comment reads REFUSED, which is the one form the delimiter rule does exclude",
	},
}

type warningBlindnessRow struct {
	name      string
	blindSpot string
	source    string
	// read is what the live reader reports today. A row demanding only "no
	// error" would stay green over a reader that read a different thing.
	read string
}

// mirroredCode is one real code, copied verbatim rather than read: these
// fixtures describe damage cases, and a fixture that read the real file would
// move with it and stop describing anything.
const mirroredCode = "installer.repo-fallback-unverified"

func warningBlindnessRows() []warningBlindnessRow {
	quoted := "'" + mirroredCode + "'"
	return []warningBlindnessRow{
		{
			name:      "a quoted code surviving only in a line comment",
			blindSpot: "quoted-literal-text-not-code",
			source:    "// export const WARNING_X = " + quoted + "\n",
			read:      "READ",
		},
		{
			name:      "a quoted code surviving only in a block comment",
			blindSpot: "quoted-literal-text-not-code",
			source:    "/*\nexport const WARNING_X = " + quoted + "\n*/\n",
			read:      "READ",
		},
		{
			name:      "a quoted code surviving only in a template literal",
			blindSpot: "quoted-literal-text-not-code",
			source:    "const doc = `\nexport const WARNING_X = " + quoted + "\n`\n",
			read:      "READ",
		},
		{
			name:      "a quoted code surviving only as JSX text",
			blindSpot: "quoted-literal-text-not-code",
			source:    "<pre>" + quoted + "</pre>\n",
			read:      "READ",
		},
		{
			name:      "the code as a markdown span in a doc comment, which it refuses",
			blindSpot: "quoted-literal-text-not-code",
			source:    " * a `" + mirroredCode + "` entry means the reference is unproven\n",
			read:      "REFUSED",
		},
	}
}

// TestWarningReaderBlindnessIsMeasured runs every row over the live reader.
//
// Acceptance is REVERSED for all but the last row: a row is green while the
// reader still reads a source nothing executes. A red here means the blindness
// closed and the entry comes out of the list -- news, not noise. The last row is
// the opposite shape on purpose: it pins the one form the delimiter rule DOES
// exclude, so a future widening of the delimiter set cannot pass unnoticed.
func TestWarningReaderBlindnessIsMeasured(t *testing.T) {
	for _, row := range warningBlindnessRows() {
		t.Run(row.name, func(t *testing.T) {
			got := "REFUSED"
			if carriedAsStringLiteral(row.source, mirroredCode) {
				got = "READ"
			}
			if got != row.read {
				t.Fatalf("the live reader reports %s, and this row recorded %s.\n"+
					"This row measures what %q still reads out of a source nothing "+
					"executes. A different answer is a different blindness -- re-measure "+
					"and rewrite the entry rather than the number.", got, row.read, row.blindSpot)
			}
		})
	}
}

// TestWarningReaderBlindSpotsAreEachMeasured binds the two lists in both
// directions, so neither can grow past the other.
func TestWarningReaderBlindSpotsAreEachMeasured(t *testing.T) {
	listed := map[string]bool{}
	for _, spot := range warningReaderBlindSpots {
		if spot.measured == "" {
			t.Errorf("blind spot %q carries no measurement; an entry without one is a claim", spot.id)
		}
		if listed[spot.id] {
			t.Errorf("blind spot %q is listed twice", spot.id)
		}
		listed[spot.id] = true
	}

	measured := map[string][]string{}
	for _, row := range warningBlindnessRows() {
		measured[row.blindSpot] = append(measured[row.blindSpot], row.name)
	}

	for id := range listed {
		if len(measured[id]) == 0 {
			t.Errorf("blind spot %q is measured by no row.\nEvery entry needs at least one "+
				"shape put through the live reader, or the written remainder is back to "+
				"being a sentence about itself.", id)
		}
	}
	for id, rows := range measured {
		if !listed[id] {
			t.Errorf("rows %v measure the mechanism %q, which warningReaderBlindSpots does "+
				"not carry. The rendered claim would then understate the reader, which is "+
				"the same defect as overstating it with the sign flipped.", rows, id)
		}
	}
}

// TestWarningReaderHonestClaim keeps the rendered claim derived from the list
// rather than written beside it: two texts of different lengths would themselves
// be the drift this guard is about.
func TestWarningReaderHonestClaim(t *testing.T) {
	var b strings.Builder
	b.WriteString("What carriedAsStringLiteral is:\n")
	b.WriteString("It binds to a quoted LITERAL and not to the VALUE an expression around " +
		"it produces, and to TEXT and not to a program. After ledger entry 74 it refuses " +
		"a prefixed extension of the code it was given -- the closing quote is the word " +
		"boundary that `-` and `.` are not. It cannot tell whether anything executes what " +
		"it read.\n")
	b.WriteString("What it therefore does not see:\n")
	for _, spot := range warningReaderBlindSpots {
		fmt.Fprintf(&b, "  - %s: %s\n    measured: %s\n", spot.id, spot.mechanism, spot.measured)
	}
	t.Log("\n" + b.String())
}
