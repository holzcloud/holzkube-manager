package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi/handlers"
	"github.com/holzcloud/holzkube-manager/internal/imagefactory"
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
// internal/imagefactory for the three client budgets and
// internal/httpapi/handlers for the two route budgets for the same reason, and
// it restates none of their values.
//
// # What changed, and why the old expression is gone
//
// The retired assertion was one call count multiplied by one constant:
//
//	worst := time.Duration(row.calls)*imagefactory.DefaultTimeout + budgetSlack
//	if worst < writeTimeout { ... }
//
// It was right while there was one client constant and no route declared a
// deadline: the sum of a route's per-call budgets *was* the route's worst case,
// so comparing it against the response budget was comparing the right two
// numbers. Both halves of that stopped being true. There are now three client
// constants -- DefaultTimeout, ProbeTimeout and ManifestTimeout -- so a product
// cannot express a route's sum; and both Factory routes now declare a shared
// deadline of their own, so the deadline is the worst case and the sum only
// says whether the deadline clips it. Keeping the old comparison would fail a
// correctly bounded route, and satisfying it would mean inflating writeTimeout
// to a number no route can reach. It is not weakened here; it is replaced by
// R1, which compares the thing that is now the worst case.
//
// # The rule, in five assertions with five failing directions
//
// sum is the total of a row's declared upstream call budgets; maxCall is the
// largest of them.
//
//   - R0, declaration. A row with at least one declared upstream call must
//     declare a non-zero routeDeadline; a row with none must declare a zero one.
//   - R1, the response-budget composition. routeDeadline + budgetSlack <
//     writeTimeout, vacuously true at a zero deadline. This is the 2 x 30 == 60
//     shape, stated once.
//   - R2, no constant is a fiction. routeDeadline >= maxCall, vacuously true
//     with no calls. A route whose ceiling is shorter than one of its own
//     calls' budgets makes that call's constant a number that never applies.
//   - R3, the verdict ratchet. The computed verdict is withinBudget exactly
//     when R1 and R2 both hold. Compared against the declared one and failing
//     in both directions: a row that becomes over budget goes red, and a row
//     still declared over budget after it has been fixed goes red.
//   - R4, the clipping ratchet. The computed clipping is clipped exactly when
//     sum > routeDeadline. Compared against the declared one, failing in both
//     directions. Clipping is not a failure: it is a declared, argued property
//     of a route whose callees could together ask for more than the route will
//     give them.
//
// A knownOverBudget row is a deferred defect, not a permission, and it must
// name the work that owns the fix. Whoever lands that fix has to come back
// here, flip the row, and close the matching .planning/WINDOWS.md entry rather
// than leaving a stale claim behind. Both Factory rows were knownOverBudget and
// named G-02-2; this is the round that flipped them.

// budgetVerdict is whether a route's declared ceiling fits inside the server's
// response budget with room for the route's own work.
type budgetVerdict string

const (
	withinBudget    budgetVerdict = "within budget"
	knownOverBudget budgetVerdict = "known over budget"
)

// clippingVerdict is whether a route's shared deadline is shorter than the sum
// of the per-call budgets its callees could ask for.
//
// It is declared data rather than a doc comment because a claim in the table
// can go stale and go red, and prose nobody re-reads cannot.
type clippingVerdict string

const (
	uncut   clippingVerdict = "uncut"
	clipped clippingVerdict = "clipped"
)

// budgetSlack is the headroom the composition must clear the response budget
// by. It exists because a route has to do its own work -- decode, load the
// record, assemble four URLs, encode -- and then write the response, all inside
// writeTimeout and all after the last upstream call returns. A composition that
// merely ties the budget has none of that left, which is how a 60.000s worst
// case produced a problem+json flushed to an already-expired socket.
const budgetSlack = 5 * time.Second

// callClass is which of the three client budgets governs one upstream call.
// Three constants, so a route's worst case is a sum of different numbers rather
// than a product of one.
type callClass string

const (
	jsonCall     callClass = "json"
	probeCall    callClass = "probe"
	manifestCall callClass = "manifest"
)

// upstreamCall is one call a route makes in series: what it is, and which
// budget class governs it.
type upstreamCall struct {
	name  string
	class callClass
}

// budget reads the class's constant out of internal/imagefactory rather than
// restating it, so moving one of the three shows up here as a changed verdict
// instead of as nothing at all.
func (u upstreamCall) budget() (time.Duration, bool) {
	switch u.class {
	case jsonCall:
		return imagefactory.DefaultTimeout, true
	case probeCall:
		return imagefactory.ProbeTimeout, true
	case manifestCall:
		return imagefactory.ManifestTimeout, true
	default:
		return 0, false
	}
}

// routeBudget is one row: what the route is, which upstream calls it makes in
// series at worst, what ceiling it declares over all of them, and what that is
// declared to compose to.
//
// The calls are declared by hand and not derived. No static analysis available
// here can be trusted to count sequential upstream calls through a handler --
// they are spread across helper functions, some are conditional, and a cache
// can remove one at runtime without removing it from the worst case. A
// hand-declared list that a human must revisit when they touch the handler is
// more honest than a derived one that is quietly wrong, and this comment is the
// instruction to revisit it.
type routeBudget struct {
	route string

	// calls is the route's *maximum* series of upstream calls, each named and
	// each carrying the budget class that governs it.
	calls []upstreamCall

	// routeDeadline is the one ceiling the route puts over all of them. Zero
	// for a route that makes no upstream call, and required otherwise (R0).
	routeDeadline time.Duration

	// verdict is what the row claims the composition comes to (R3).
	verdict budgetVerdict

	// clipping is whether the row claims its ceiling is shorter than the sum of
	// its calls' budgets (R4).
	clipping clippingVerdict

	// clippingRationale is the argument a clipped row owes and an uncut row
	// must not carry. Required on clipped, empty on uncut.
	clippingRationale string

	// deferredTo names the work that owns the fix. Required on a
	// knownOverBudget row and empty on a withinBudget one.
	deferredTo string

	// why records where the declaration comes from, so the next reader can
	// check it against the handler rather than trusting it.
	why string
}

var routeBudgets = []routeBudget{
	{
		route: "GET /api/v1/schematics/{id}/assets",
		calls: []upstreamCall{
			{name: "resolveInstallerRepo: slowest concurrent candidate manifest GET", class: manifestCall},
		},
		routeDeadline: handlers.AssetsRouteBudget,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "schematicAssets resolves the installer reference, and resolveInstallerRepo asks " +
			"every candidate repository name at the same time rather than one after the other " +
			"-- so the route's worst case is the slowest single candidate, one manifest " +
			"budget, and not the sum of the candidates. Check that against the function: it " +
			"issues one request per candidate up front and only then walks the answers in " +
			"declared order, so the number of candidates does not enter this row's arithmetic. " +
			"The count is one for that reason and not because there is one candidate -- " +
			"installerCandidates still returns two. A concurrent fan-out is exactly the shape " +
			"a derived count would miscount, which is why this list is declared by hand and " +
			"why whoever changes the handler has to come back and revisit this number. The " +
			"four other references are assembled locally and cost nothing. The bounded " +
			"re-question added by plan 02-12 is one call on the warm path, not an additional " +
			"call on the cold path, so it does not lengthen this list. Measured against the " +
			"serial walk before the ceiling existed: status=502 duration=1m0.002907792s, the " +
			"60.000s serial sum arriving against a 60s writeTimeout -- history, not the " +
			"current composition. AssetsRouteBudget is the worst case and the sum only says " +
			"whether it clips.",
	},
	{
		route: "POST /api/v1/schematics",
		calls: []upstreamCall{
			{name: "imagefactory.Author: Extensions", class: jsonCall},
			{name: "imagefactory.Author: CreateSchematic", class: jsonCall},
			{name: "imagefactory.Author: ProbeBuildable", class: probeCall},
		},
		routeDeadline: handlers.CreateRouteBudget,
		verdict:       withinBudget,
		clipping:      clipped,
		clippingRationale: "The route can afford one of its two JSON calls consuming its whole " +
			"budget before the probe, not both. On the pathological case where both do, the " +
			"probe is cut by the route and the record ends unprobed -- the existing fail-safe " +
			"outcome, on a path an operator only reaches through an upstream that is barely " +
			"answering. The clipping is exactly one JSON budget wide, which is the headroom " +
			"CreateRouteBudget adds above its R2 floor.",
		why: "imagefactory.Author issues Extensions, CreateSchematic and ProbeBuildable in " +
			"series, and ProbeBuildable makes the Factory build a ~335MB image synchronously " +
			"-- which is why the third call is the probe class and not the JSON one. The " +
			"ranged-GET fallback inside ProbeBuildable is not a fourth call in the worst " +
			"case: it is reached only on a 405 or a 501, which is an answer arriving quickly, " +
			"never a budget expiring. CreateRouteBudget is the ceiling over all three and the " +
			"composition is computed here rather than asserted.",
	},
	{
		route:         "GET /api/v1/schematics",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "store-only: the list is read from fsstore and no Factory call is made. It is in " +
			"the table so the table demonstrably distinguishes a route that talks upstream " +
			"from one that does not -- and so R0 has a row that must *not* declare a ceiling.",
	},
}

// decompose is the per-call arithmetic a failure message prints, so a reader can
// check it rather than trust it.
func (row routeBudget) decompose(t *testing.T) (sum, maxCall time.Duration, detail string) {
	t.Helper()

	var parts []string
	for _, call := range row.calls {
		d, ok := call.budget()
		if !ok {
			t.Fatalf("%s declares the call %q with the unknown budget class %q; every call "+
				"must name one of the three constants that actually govern requests",
				row.route, call.name, call.class)
		}
		sum += d
		if d > maxCall {
			maxCall = d
		}
		parts = append(parts, fmt.Sprintf("%s [%s %s]", call.name, call.class, d))
	}
	if len(parts) == 0 {
		return 0, 0, "no upstream calls"
	}
	return sum, maxCall, strings.Join(parts, " + ")
}

// TestRouteBudgetsComposeAgainstWriteTimeout is the assertion itself: for each
// route, recompute the composition from the constants that actually govern it
// and check the computed verdicts against the declared ones.
func TestRouteBudgetsComposeAgainstWriteTimeout(t *testing.T) {
	for _, row := range routeBudgets {
		t.Run(row.route, func(t *testing.T) {
			sum, maxCall, detail := row.decompose(t)

			// Printed on pass as well as on failure. A table whose arithmetic
			// is only visible when it breaks is a table nobody checks.
			t.Logf("%s: %s = %s sum, largest call %s, route deadline %s, slack %s, writeTimeout %s",
				row.route, detail, sum, maxCall, row.routeDeadline, budgetSlack, writeTimeout)

			// R0, declaration.
			switch {
			case len(row.calls) > 0 && row.routeDeadline == 0:
				t.Errorf("%s makes %d upstream call(s) (%s) and declares no routeDeadline. A "+
					"route that talks upstream without a ceiling has a worst case equal to "+
					"the sum of whatever its callees happen to do, which is the shape this "+
					"table exists to end.\nDeclaration: %s",
					row.route, len(row.calls), detail, row.why)
			case len(row.calls) == 0 && row.routeDeadline != 0:
				t.Errorf("%s makes no upstream call and declares a routeDeadline of %s, which "+
					"it cannot apply to anything.\nDeclaration: %s",
					row.route, row.routeDeadline, row.why)
			}

			// R1, the response-budget composition. Vacuous at a zero deadline.
			r1 := row.routeDeadline == 0 || row.routeDeadline+budgetSlack < writeTimeout

			// R2, no constant is a fiction. Vacuous with no calls.
			r2 := len(row.calls) == 0 || row.routeDeadline >= maxCall

			// R3, the verdict ratchet, failing in both directions.
			computed := knownOverBudget
			if r1 && r2 {
				computed = withinBudget
			}
			if computed != row.verdict {
				t.Errorf("%s computes to %q but the row declares %q.\n"+
					"  calls:          %s\n"+
					"  sum:            %s\n"+
					"  largest call:   %s\n"+
					"  routeDeadline:  %s\n"+
					"  R1 (deadline + %s slack < writeTimeout = %s): %t\n"+
					"  R2 (deadline >= largest call): %t\n"+
					"Declaration: %s\n"+
					"If the route changed, fix the row. If a budget changed, this table is "+
					"where that has to be argued -- and a knownOverBudget row that has become "+
					"within budget must be flipped and its .planning/WINDOWS.md entry closed.",
					row.route, computed, row.verdict,
					detail, sum, maxCall, row.routeDeadline,
					budgetSlack, writeTimeout, r1, r2, row.why)
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

			// R4, the clipping ratchet, failing in both directions. Clipping is
			// not a failure; an undeclared or misdeclared clipping is.
			computedClipping := uncut
			if sum > row.routeDeadline {
				computedClipping = clipped
			}
			if computedClipping != row.clipping {
				t.Errorf("%s computes to %q but the row declares %q: the declared calls sum to "+
					"%s against a routeDeadline of %s.\n"+
					"  calls: %s\n"+
					"Declaration: %s\n"+
					"A route whose callees can together ask for more than the route will give "+
					"them is clipped, and clipping has to be argued rather than discovered.",
					row.route, computedClipping, row.clipping, sum, row.routeDeadline,
					detail, row.why)
			}
			if row.clipping == clipped && row.clippingRationale == "" {
				t.Errorf("%s is declared clipped and carries no rationale. What gets cut, on "+
					"which path, and with what outcome for the operator is the whole content "+
					"of the claim; without it the row records that something is lost and not "+
					"what", row.route)
			}
			if row.clipping == uncut && row.clippingRationale != "" {
				t.Errorf("%s is declared uncut and carries a clipping rationale (%q); a "+
					"rationale for a cut that does not happen is a claim nothing holds to "+
					"account", row.route, row.clippingRationale)
			}
		})
	}
}

// TestRouteBudgetTableReadsTheRealConstants stops the table drifting into a
// self-consistent fiction. It asserts the values it composes are the ones the
// server and the client actually run with, so a change to any of them shows up
// here as a changed verdict rather than as nothing at all.
//
// It pinned two constants before this round and pins four now, because a table
// that sums three different client budgets can be made to agree with itself
// about numbers nothing runs just as easily as one that multiplied a single
// one. Widened, never deleted: raising writeTimeout is exactly the change that
// should turn this red first.
func TestRouteBudgetTableReadsTheRealConstants(t *testing.T) {
	if writeTimeout != 130*time.Second {
		t.Errorf("writeTimeout = %s, and the declarations in this table were computed against "+
			"130s. Recheck every row rather than editing this line", writeTimeout)
	}
	if imagefactory.DefaultTimeout != 30*time.Second {
		t.Errorf("imagefactory.DefaultTimeout = %s, and the declarations in this table were "+
			"computed against 30s. Recheck every row rather than editing this line",
			imagefactory.DefaultTimeout)
	}
	if imagefactory.ProbeTimeout != 90*time.Second {
		t.Errorf("imagefactory.ProbeTimeout = %s, and the declarations in this table were "+
			"computed against 90s. Recheck every row rather than editing this line",
			imagefactory.ProbeTimeout)
	}
	if imagefactory.ManifestTimeout != 30*time.Second {
		t.Errorf("imagefactory.ManifestTimeout = %s, and the declarations in this table were "+
			"computed against 30s. Recheck every row rather than editing this line",
			imagefactory.ManifestTimeout)
	}
}
