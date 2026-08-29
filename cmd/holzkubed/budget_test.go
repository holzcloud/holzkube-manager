package main

import (
	"testing"
	"time"

	"github.com/holzcloud/holzkube/internal/imagefactory"
)

// This file is the composition guard 02-DECISION-probe-budget.md asks for under
// "What is not in question either way": "(max sequential upstream calls per
// route) x (client timeout) + slack < writeTimeout, as a table over routes". Its
// absence is what let 2 x 30 == 60 ship -- writeTimeout was reasoned about
// argon2id and the login rate limiter (main.go:33-42) and never about the
// upstream budgets the Factory routes consume, and nothing anywhere multiplied
// the two numbers together.
//
// It lives in package main because writeTimeout is an unexported constant here.
// A guard that re-declares the number it is guarding guards nothing: it would
// keep passing while the real constant moved underneath it. It imports
// internal/imagefactory for DefaultTimeout for the same reason, and it changes
// neither value -- what those values should be is the open question the
// decision document owns, not this test's to answer.
//
// The table ratchets in both directions, which is the property that makes it
// worth writing rather than merely writing down:
//
//   - A route declared withinBudget that gains a fourth sequential upstream call
//     computes to over budget and this test goes red.
//   - A route declared knownOverBudget that is *fixed* also goes red, because its
//     declaration still says it is over budget. Whoever lands the per-route
//     deadline has to come back here, flip the row, and close the matching
//     .planning/WINDOWS.md entry rather than leaving a stale claim behind.
//
// A knownOverBudget row is a deferred defect, not a permission. Both rows below
// name G-02-2 as the work that owns the fix.

// budgetVerdict is whether a route's worst-case upstream composition fits
// inside the server's response budget.
type budgetVerdict string

const (
	withinBudget    budgetVerdict = "within budget"
	knownOverBudget budgetVerdict = "known over budget"
)

// budgetSlack is the headroom the composition must clear the response budget
// by. It exists because a route has to do its own work -- decode, load the
// record, assemble four URLs, encode -- and then write the response, all inside
// writeTimeout and all after the last upstream call returns. A composition that
// merely ties the budget has none of that left, which is how a 60.000s worst
// case produced a problem+json flushed to an already-expired socket.
const budgetSlack = 5 * time.Second

// routeBudget is one row: what the route is, how many upstream calls it makes
// in series at worst, and what that is declared to compose to.
//
// The call counts are declared by hand and not derived. No static analysis
// available here can be trusted to count sequential upstream calls through a
// handler -- they are spread across helper functions, some are conditional, and
// a cache can remove one at runtime without removing it from the worst case. A
// hand-declared number that a human must revisit when they touch the handler is
// more honest than a derived one that is quietly wrong, and this comment is the
// instruction to revisit it.
type routeBudget struct {
	route string

	// calls is the route's *maximum* number of upstream calls issued in series,
	// each independently bounded by imagefactory.DefaultTimeout.
	calls int

	// verdict is what the row claims the composition comes to.
	verdict budgetVerdict

	// deferredTo names the work that owns the fix. Required on a
	// knownOverBudget row and empty on a withinBudget one.
	deferredTo string

	// why records where the call count comes from, so the next reader can check
	// the declaration against the handler rather than trusting it.
	why string
}

var routeBudgets = []routeBudget{
	{
		route:      "GET /api/v1/schematics/{id}/assets",
		calls:      2,
		verdict:    knownOverBudget,
		deferredTo: "G-02-2; per-route deadline owned by 02-DECISION-probe-budget.md",
		why: "schematicAssets resolves the installer reference, and a cold resolveInstallerRepo " +
			"walks both candidate repository names serially. The four other references are " +
			"assembled locally and cost nothing. Measured: status=502 duration=1m0.002907792s. " +
			"The bounded re-question added by plan 02-12 is one call on the warm path, not an " +
			"additional call on the cold path, so it does not raise this number.",
	},
	{
		route:      "POST /api/v1/schematics",
		calls:      3,
		verdict:    knownOverBudget,
		deferredTo: "G-02-2; probe budget owned by 02-DECISION-probe-budget.md",
		why: "imagefactory.Author issues Extensions, CreateSchematic and ProbeBuildable in " +
			"series, and ProbeBuildable makes the Factory build a ~335MB image synchronously.",
	},
	{
		route:   "GET /api/v1/schematics",
		calls:   0,
		verdict: withinBudget,
		why: "store-only: the list is read from fsstore and no Factory call is made. It is in " +
			"the table so the table demonstrably distinguishes a route that talks upstream " +
			"from one that does not.",
	},
}

// TestRouteBudgetsComposeAgainstWriteTimeout is the assertion itself: for each
// route, compute the worst-case composition from the two constants that
// actually govern it and check the computed verdict against the declared one.
func TestRouteBudgetsComposeAgainstWriteTimeout(t *testing.T) {
	for _, row := range routeBudgets {
		t.Run(row.route, func(t *testing.T) {
			worst := time.Duration(row.calls)*imagefactory.DefaultTimeout + budgetSlack

			computed := knownOverBudget
			if worst < writeTimeout {
				computed = withinBudget
			}

			if computed != row.verdict {
				t.Errorf("%s: %d sequential upstream calls x %s + %s slack = %s against "+
					"writeTimeout = %s, which computes to %q, but the row declares %q.\n"+
					"Declaration: %s\n"+
					"If the route changed, fix the row. If the budget changed, this table is "+
					"where that has to be argued -- and a knownOverBudget row that has become "+
					"within budget must be flipped and its .planning/WINDOWS.md entry closed.",
					row.route, row.calls, imagefactory.DefaultTimeout, budgetSlack, worst,
					writeTimeout, computed, row.verdict, row.why)
			}

			if row.verdict == knownOverBudget && row.deferredTo == "" {
				t.Errorf("%s is declared over budget but names no deferred work that owns the "+
					"fix; an undated defect with no owner is the silence this table exists to end",
					row.route)
			}
			if row.verdict == withinBudget && row.deferredTo != "" {
				t.Errorf("%s fits the budget but still points at deferred work (%q); a stale "+
					"reference here is how a closed defect stays open on paper",
					row.route, row.deferredTo)
			}
		})
	}
}

// TestRouteBudgetTableReadsTheRealConstants stops the table drifting into a
// self-consistent fiction. It asserts the two values it multiplies are the ones
// the server and the client actually run with, so a change to either shows up
// here as a changed verdict rather than as nothing at all.
func TestRouteBudgetTableReadsTheRealConstants(t *testing.T) {
	if writeTimeout != 60*time.Second {
		t.Errorf("writeTimeout = %s, and the declarations in this table were computed against "+
			"60s. Recheck every row rather than editing this line", writeTimeout)
	}
	if imagefactory.DefaultTimeout != 30*time.Second {
		t.Errorf("imagefactory.DefaultTimeout = %s, and the declarations in this table were "+
			"computed against 30s. Recheck every row rather than editing this line",
			imagefactory.DefaultTimeout)
	}
}
