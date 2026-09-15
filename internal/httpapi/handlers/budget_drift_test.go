package handlers_test

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi/handlers"
)

// This file is the drift guard behind a claim the Images screen makes: the two
// places an operator waits on an upstream say how long the server may take, and
// the number they say is the route budget the server actually enforces.
//
// It is written in Go and reads TypeScript, which is the direction the two
// guards this phase already established go -- internal/imagefactory/warnings_test.go's
// TestWarningDetailsMatchTheUI reads web/src/components/SchematicWarnings.tsx,
// and this reads web/src/routes/images.tsx. The direction is not a preference.
// vitest is rooted at web/ and refuses to read outside it without loosening the
// bundler's filesystem allowlist, and it could not read a Go constant even then;
// a Go test reads a TypeScript literal with the standard library.
//
// package handlers_test rather than package handlers, matching schematics_test.go
// beside it and TestWarningDetailsMatchTheUI's own package imagefactory_test. The
// earlier reason to be inside the package -- that the route budgets were
// unexported -- stopped being true when they were exported so that
// cmd/holzkube-managerd/budget_test.go could read the values that run. An
// external test package reads them for the same reason.
//
// If this fails, the fix is to correct the number in images.tsx, never to edit
// the assertion. The constants in internal/httpapi/handlers/schematics.go are
// what the server enforces; the UI is transcribing them.

// uiPath is the screen that declares the two waits.
//
// Relative to this package's directory, which is where `go test` runs: three
// levels up from internal/httpapi/handlers is the repository root.
const uiPath = "../../../web/src/routes/images.tsx"

// declaredWait is one constant this guard reads out of the UI, and the route
// budget it must equal.
type declaredWait struct {
	// constant is the identifier declared in images.tsx.
	constant string

	// budget is the server-side ceiling it is transcribing.
	budget time.Duration

	// budgetName is what to call that ceiling in a failure message, so the
	// reader is sent to the constant rather than to a duration.
	budgetName string
}

var declaredWaits = []declaredWait{
	{
		constant:   "ASSETS_WAIT_SECONDS",
		budget:     handlers.AssetsRouteBudget,
		budgetName: "handlers.AssetsRouteBudget",
	},
	{
		constant:   "CREATE_WAIT_SECONDS",
		budget:     handlers.CreateRouteBudget,
		budgetName: "handlers.CreateRouteBudget",
	},
}

// TestBudgetWaitsStatedInTheUIMatchTheRouteBudgets compares each wait the two
// waiting screens state against the route budget the server enforces for them.
//
// A stated ceiling that is longer than the enforced one promises an operator a
// patience the server does not have. A stated ceiling that is shorter reads as
// the server having failed while it is still working. Neither is visible from
// either side alone, which is what this test is for.
func TestBudgetWaitsStatedInTheUIMatchTheRouteBudgets(t *testing.T) {
	source, err := os.ReadFile(uiPath)
	if err != nil {
		t.Fatalf("reading %s: %v", uiPath, err)
	}
	ui := string(source)

	for _, w := range declaredWaits {
		match := declaredWaitPattern(w.constant).FindStringSubmatch(ui)
		if match == nil {
			t.Errorf("%s declares no constant %s as a bare integer of seconds.\n"+
				"The waiting state that names %s (%s) reads its number from there, and this "+
				"guard reads the same declaration. Declare it as `export const %s = %d`.",
				uiPath, w.constant, w.budgetName, w.budget, w.constant,
				int(w.budget/time.Second))
			continue
		}

		stated, err := strconv.Atoi(match[1])
		if err != nil {
			t.Errorf("%s: %s = %q, which is not a number: %v", uiPath, w.constant, match[1], err)
			continue
		}

		want := int(w.budget / time.Second)
		if stated != want {
			t.Errorf("%s states %d seconds and %s is %s (%d seconds).\n"+
				"  UI (%s):     %s = %d\n"+
				"  server (%s): %s = %s\n"+
				"The server side is authoritative: fix the number in %s, never this "+
				"assertion. A screen that promises longer than the server will wait tells an "+
				"operator to keep waiting after the answer has already been given up on; one "+
				"that promises shorter reads as a failure while the server is still working.",
				w.constant, stated, w.budgetName, w.budget, want,
				uiPath, w.constant, stated,
				"internal/httpapi/handlers/schematics.go", w.budgetName, w.budget,
				uiPath)
			continue
		}

		// Printed on pass as well as on failure: a comparison that is only
		// visible when it breaks is a comparison nobody checks.
		t.Logf("%s = %d s == %s (%s)", w.constant, stated, w.budgetName, w.budget)
	}
}

// declaredWaitPattern is this file's reader, pulled out of the loop so the
// blindness rows below can drive the LIVE one rather than a copy of it.
//
// Anchored on the declaration so a number that merely appears somewhere in the
// file cannot satisfy it. The literal is required to be a bare integer of
// seconds: an expression would be a second place the value could be computed,
// and this guard would then be checking one of them.
//
// Ledger entry 74 lists this reader as one of the three siblings of the
// browser-refusal guard and asks two questions of each. The first is ALREADY
// ANSWERED here and was answered before the entry existed: it has NO prefix
// hole. What follows the identifier must be whitespace and then `=`, so the
// underscore in CREATE_WAIT_SECONDS_LEGACY breaks the pattern -- re-measured
// while closing entry 74, against a source carrying only that name: REFUSED.
// 02-DECISION-drift-guard-lexik.md says this reader carries round 5's prefix
// hole "ebenso"; it does not, and that sentence is the one thing in that
// document this measurement contradicts.
//
// The second question is the open one, and the rows below answer it. Its
// lexical blindness is NARROWER than that document assumes and for a different
// reason: the line-start anchor allows indentation, but `export` or `const` must
// follow the whitespace immediately, so a line beginning with `//` or with `{/*`
// falls through. It is blind where the declaration line ITSELF still begins with
// `const` -- inside a block comment and inside a template literal.
func declaredWaitPattern(constant string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(
		`(?m)^\s*(?:export\s+)?const\s+%s\s*=\s*([0-9]+)\b`, regexp.QuoteMeta(constant)))
}

// waitReaderBlindSpot is one mechanism declaredWaitPattern does not see, carried
// as DATA in the form plan 02-30 established for guardBlindSpots.
type waitReaderBlindSpot struct {
	id        string
	mechanism string
	measured  string
}

var waitReaderBlindSpots = []waitReaderBlindSpot{
	{
		id: "declaration-line-text-not-code",
		mechanism: "the anchor asks what precedes `const` on ITS OWN LINE and nothing about " +
			"what encloses that line, so a form whose interior lines still begin with " +
			"`const` satisfies it exactly as well as code does",
		measured: "`export const CREATE_WAIT_SECONDS = 45` inside a /* */ block reads 45, " +
			"and inside a template literal reads 45; the same line inside a // line " +
			"comment and inside an indented {/* */} JSX comment is REFUSED, because " +
			"neither `//` nor `{/*` is whitespace",
	},
}

// waitBlindnessRow is one measured shape: a synthetic source the reader reads
// although nothing executes what it read, or refuses although the text is there.
type waitBlindnessRow struct {
	name      string
	blindSpot string
	source    string
	// read is the value the live reader pulls out, or "" for a refusal. The
	// VALUE is part of the measurement: a row demanding only "matched" would
	// stay green over a reader that read a different number.
	read string
}

func waitBlindnessRows() []waitBlindnessRow {
	const decl = "export const CREATE_WAIT_SECONDS = 45"
	return []waitBlindnessRow{
		{
			name:      "a declaration surviving only in a block comment",
			blindSpot: "declaration-line-text-not-code",
			source:    "/*\n" + decl + "\n*/\n",
			read:      "45",
		},
		{
			name:      "a declaration surviving only in a template literal",
			blindSpot: "declaration-line-text-not-code",
			source:    "const doc = `\n" + decl + "\n`\n",
			read:      "45",
		},
		{
			name:      "a declaration in a line comment, which it refuses",
			blindSpot: "declaration-line-text-not-code",
			source:    "// " + decl + "\n",
			read:      "",
		},
		{
			name:      "a declaration in an indented JSX comment, which it refuses",
			blindSpot: "declaration-line-text-not-code",
			source:    "      {/* " + decl + " */}\n",
			read:      "",
		},
		{
			name:      "a prefixed leftover, which it refuses and always did",
			blindSpot: "declaration-line-text-not-code",
			source:    "export const CREATE_WAIT_SECONDS_LEGACY = 45\n",
			read:      "",
		},
	}
}

// TestWaitReaderBlindnessIsMeasured runs every row over the live reader.
//
// Two shapes of row, and the mix is the point. The first two have REVERSED
// acceptance: they are green while the reader still reads a source nothing
// executes, and a red means the blindness closed. The last three pin what it
// REFUSES -- the two comment forms and the prefixed leftover -- so a future
// loosening of the anchor cannot pass unnoticed. Entry 74 asks for the second
// kind explicitly: for this reader the prefix half is already met, and a
// measurement that is not held by a line decays into a sentence.
func TestWaitReaderBlindnessIsMeasured(t *testing.T) {
	for _, row := range waitBlindnessRows() {
		t.Run(row.name, func(t *testing.T) {
			got := ""
			if m := declaredWaitPattern("CREATE_WAIT_SECONDS").FindStringSubmatch(row.source); m != nil {
				got = m[1]
			}
			if got != row.read {
				t.Fatalf("the live reader reports %q, and this row recorded %q.\n"+
					"This row measures what %q does with a source nothing executes. A "+
					"different answer is a different blindness -- re-measure and rewrite "+
					"the entry rather than the number.", got, row.read, row.blindSpot)
			}
		})
	}
}

// TestWaitReaderBlindSpotsAreEachMeasured binds the two lists in both
// directions, so neither can grow past the other.
func TestWaitReaderBlindSpotsAreEachMeasured(t *testing.T) {
	listed := map[string]bool{}
	for _, spot := range waitReaderBlindSpots {
		if spot.measured == "" {
			t.Errorf("blind spot %q carries no measurement; an entry without one is a claim", spot.id)
		}
		listed[spot.id] = true
	}

	measured := map[string][]string{}
	for _, row := range waitBlindnessRows() {
		measured[row.blindSpot] = append(measured[row.blindSpot], row.name)
	}

	for id := range listed {
		if len(measured[id]) == 0 {
			t.Errorf("blind spot %q is measured by no row", id)
		}
	}
	for id, rows := range measured {
		if !listed[id] {
			t.Errorf("rows %v measure the mechanism %q, which waitReaderBlindSpots does not carry",
				rows, id)
		}
	}
}
