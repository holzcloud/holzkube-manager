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
		// Anchored on the declaration so a number that merely appears somewhere
		// in the file cannot satisfy this. The literal is required to be a bare
		// integer of seconds: an expression would be a second place the value
		// could be computed, and this guard would then be checking one of them.
		pattern := regexp.MustCompile(fmt.Sprintf(
			`(?m)^\s*(?:export\s+)?const\s+%s\s*=\s*([0-9]+)\b`, regexp.QuoteMeta(w.constant)))

		match := pattern.FindStringSubmatch(ui)
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
