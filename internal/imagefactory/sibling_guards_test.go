package imagefactory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The written-down remainder of stringArrayLiteral, this package's array-literal
// transcription reader,
// in the form plan 02-30 established for guardBlindSpots: one entry per
// MECHANISM, each carrying the output that evidences it, and a row that
// measures it over the live read path.
//
// Ledger entry 74 is what this closes for this reader. It found that the three
// sibling readers of the browser-refusal guard carry the same genus of hole --
// an anchor bound to a LITERAL rather than to a VALUE -- each in its own extent,
// and it named the two ways out: carry plan 02-27's anchoring discipline, or
// carry a written, measured exclusion. This reader now does the first for the
// prefix hole and the second for what anchoring cannot reach. Its two siblings
// carry the same pair in warnings_blindness_test.go and in
// internal/httpapi/handlers/budget_drift_test.go.
//
// Why a written list and not a sentence: a sentence claiming the remainder is
// zero has infinitely many counterexamples and every one falsifies it. A list
// has finitely many entries and a new shape is an addition.

// siblingBlindSpot is one mechanism a reader in this package does not see.
type siblingBlindSpot struct {
	// reader names the symbol this entry is about, so an entry cannot drift
	// away from the function it describes.
	reader string
	// id is the stable name a siblingBlindnessRow refers to.
	id string
	// mechanism says in one line why the reader is blind here, in terms of what
	// its anchor binds to rather than a syntax it fails to parse.
	mechanism string
	// measured carries the output that evidences the entry. An entry without a
	// measurement is a claim.
	measured string
}

// siblingBlindSpots is the remainder for this reader.
//
// What is NOT in it is as load-bearing as what is: the prefix hole is absent
// because it was CLOSED rather than excluded, and the closure was measured
// against the fault reinstated in the REAL tree, not in a fixture.
//   - the browser suite's declaration renamed to
//     INSTALLER_REPOSITORY_NAMES_LITERAL_LEGACY throughout, so the file stays
//     coherent. The old pattern read members=4 with no error; the anchored one
//     refuses, and TestBrowserInstallerNamesEqualInstallerCandidates goes red.
var siblingBlindSpots = []siblingBlindSpot{
	{
		reader: "stringArrayLiteral",
		id:     "array-literal-text-not-code",
		mechanism: "the line-start anchor asks what precedes `const` on ITS OWN LINE and " +
			"nothing about what encloses that line, so a form whose interior lines still " +
			"begin with `const` satisfies it as well as code does",
		measured: "a declaration living only inside a /* */ block reads members=2, and one " +
			"living only inside a template literal reads members=2, each with the real " +
			"array imported and asserted against beside it; the same declaration in a // " +
			"line comment and in an indented {/* */} JSX comment is REFUSED, because " +
			"neither `//` nor `{/*` is whitespace",
	},
}

// siblingBlindnessRow is one measured shape of one siblingBlindSpot: a synthetic
// source the reader reads although nothing executes what it read.
type siblingBlindnessRow struct {
	name      string
	blindSpot string
	source    string
	// read is what the live reader reports for source today. A row that only
	// demanded "no error" would stay green over a reader that read a different
	// thing, so the OUTPUT is part of the measurement.
	read string
}

// The real declarations, copied verbatim rather than read, because these
// fixtures describe damage cases: a fixture that read the real file would move
// with it and stop describing anything.
const (
	realArrayDecl = `export const INSTALLER_REPOSITORY_NAMES_LITERAL: readonly string[] = [
  'siderolabs/installer',
  'siderolabs/installer-secureboot',
]
`
	// Without the import and the assertion beside it these fixtures would be
	// broken TypeScript rather than damage cases, and a reader passing over a
	// file that does not compile proves nothing.
	liveArrayUse = `import { INSTALLER_REPOSITORY_NAMES } from '../lib/installer'
expect([...INSTALLER_REPOSITORY_NAMES]).toEqual(['siderolabs/installer'])
`
)

func siblingBlindnessRows() []siblingBlindnessRow {
	return []siblingBlindnessRow{
		{
			name:      "an array declaration surviving only in a block comment",
			blindSpot: "array-literal-text-not-code",
			source:    liveArrayUse + "/*\n" + realArrayDecl + "*/\n",
			read:      "members=2",
		},
		{
			name:      "an array declaration surviving only in a template literal",
			blindSpot: "array-literal-text-not-code",
			source:    liveArrayUse + "const doc = `\n" + realArrayDecl + "`\n",
			read:      "members=2",
		},
	}
}

// readSibling drives the LIVE reader for a blind spot and renders what it read.
//
// It calls the shipped functions and not a copy of their patterns. That is the
// load-bearing part: a change to either reader turns these rows red instead of
// slipping past them, which is the whole difference between a measurement and a
// transcription of one.
func readSibling(t *testing.T, blindSpot, source string) string {
	t.Helper()

	switch blindSpot {
	case "array-literal-text-not-code":
		path := filepath.Join(t.TempDir(), "images.browser.test.tsx")
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}
		members := stringArrayLiteral(t, readSource(t, path), "INSTALLER_REPOSITORY_NAMES_LITERAL")
		return fmt.Sprintf("members=%d", len(members))
	default:
		t.Fatalf("no live reader wired for blind spot %q", blindSpot)
		return ""
	}
}

// TestSiblingGuardBlindnessIsMeasured runs every row over the live reader.
//
// Its acceptance is REVERSED, and that is the point: a row is green while the
// reader still reads a source nothing executes. When one of these goes red
// because the reader started refusing, the blindness closed and the entry comes
// out of siblingBlindSpots -- a red here is news, not noise.
func TestSiblingGuardBlindnessIsMeasured(t *testing.T) {
	for _, row := range siblingBlindnessRows() {
		t.Run(row.name, func(t *testing.T) {
			got := readSibling(t, row.blindSpot, row.source)
			if got != row.read {
				t.Fatalf("the live reader reports %s, and this row recorded %s.\n"+
					"This row measures what %q still reads out of a source nothing "+
					"executes. A different answer is a different blindness -- re-measure "+
					"and rewrite the entry rather than the number.", got, row.read, row.blindSpot)
			}
		})
	}
}

// TestSiblingBlindSpotsAreEachMeasured binds the two lists to each other in both
// directions, so neither can grow past the other.
//
// An entry with no row is a claim nothing checks, which is the shape this file
// exists to stop. A row naming no entry measures a blindness the written
// remainder does not mention, which understates the readers -- the same defect
// as overstating them, with the sign flipped.
func TestSiblingBlindSpotsAreEachMeasured(t *testing.T) {
	listed := map[string]string{}
	for _, spot := range siblingBlindSpots {
		if spot.measured == "" {
			t.Errorf("blind spot %q carries no measurement; an entry without one is a claim", spot.id)
		}
		if _, dup := listed[spot.id]; dup {
			t.Errorf("blind spot %q is listed twice", spot.id)
		}
		listed[spot.id] = spot.reader
	}

	measured := map[string][]string{}
	for _, row := range siblingBlindnessRows() {
		measured[row.blindSpot] = append(measured[row.blindSpot], row.name)
	}

	for id := range listed {
		if len(measured[id]) == 0 {
			t.Errorf("blind spot %q (reader %s) is measured by no row.\n"+
				"Every entry needs at least one shape put through the live reader, or the "+
				"written remainder is back to being a sentence about itself.", id, listed[id])
		}
	}
	for id, rows := range measured {
		if _, ok := listed[id]; !ok {
			t.Errorf("rows %v measure the mechanism %q, which siblingBlindSpots does not carry",
				rows, id)
		}
	}
}

// TestSiblingHonestClaimNamesEveryBlindSpot keeps the rendered claim derived from
// the list rather than written beside it: two texts of different lengths would
// themselves be the drift this package guards against.
func TestSiblingHonestClaimNamesEveryBlindSpot(t *testing.T) {
	claim := siblingHonestClaim()
	for _, spot := range siblingBlindSpots {
		if !strings.Contains(claim, spot.id) || !strings.Contains(claim, spot.measured) {
			t.Errorf("the rendered claim does not carry blind spot %q in full", spot.id)
		}
	}
	t.Log("\n" + claim)
}

// siblingHonestClaim renders what these two readers establish OUT OF the list of
// what they do not.
func siblingHonestClaim() string {
	var b strings.Builder
	b.WriteString("What stringArrayLiteral is:\n")
	b.WriteString("It binds to a LITERAL and not to the VALUE an expression around it " +
		"produces, and to TEXT and not to a program. After ledger entry 74 it refuses a " +
		"prefixed extension of the name it was given -- the required `:` or `=` is the " +
		"word boundary that `_` is not. It cannot tell whether anything executes what it " +
		"read.\n")
	b.WriteString("What they therefore do not see:\n")
	for _, spot := range siblingBlindSpots {
		fmt.Fprintf(&b, "  - %s (%s): %s\n    measured: %s\n",
			spot.id, spot.reader, spot.mechanism, spot.measured)
	}
	return b.String()
}
