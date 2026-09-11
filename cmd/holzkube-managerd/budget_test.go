package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi/handlers"
	"github.com/holzcloud/holzkube-manager/internal/imagefactory"
	"github.com/holzcloud/holzkube-manager/internal/talos"
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

	// The two node classes, added in phase 3. They are a different family from
	// the three above -- those are the Image Factory's, these are a Talos
	// node's -- and they are read out of internal/talos's own class table for
	// the reason this whole file exists: a guard that restates the number it
	// guards guards nothing.
	nodeProbeCall    callClass = "node-probe"
	nodeFastReadCall callClass = "node-fast-read"
	nodeMutationCall callClass = "node-mutation"
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
	case nodeProbeCall:
		return talos.ClassProbe.Deadline(), true
	case nodeFastReadCall:
		return talos.ClassFastRead.Deadline(), true
	case nodeMutationCall:
		return talos.ClassMutation.Deadline(), true
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

// nodeFactsCalls is the fixed series of resource reads talos.NodeFacts
// performs. It is a function because three rows make the same walk, and three
// hand-copied lists would drift from each other before they drifted from the
// code.
func nodeFactsCalls() []upstreamCall {
	return []upstreamCall{
		{name: "NodeFacts: SystemInformation Get", class: nodeFastReadCall},
		{name: "NodeFacts: NodeAddress Get (filtered)", class: nodeFastReadCall},
		{name: "NodeFacts: NodeAddress Get (unfiltered fallback)", class: nodeFastReadCall},
		{name: "NodeFacts: Processor List", class: nodeFastReadCall},
		{name: "NodeFacts: MemoryModule List", class: nodeFastReadCall},
		{name: "NodeFacts: Disk List", class: nodeFastReadCall},
		{name: "NodeFacts: LinkStatus List", class: nodeFastReadCall},
		{name: "NodeFacts: ExtensionStatus List", class: nodeFastReadCall},
		{name: "NodeFacts: KubeletSpec Get", class: nodeFastReadCall},
		{name: "NodeFacts: Nodename Get", class: nodeFastReadCall},
		{name: "NodeFacts: HostnameStatus Get", class: nodeFastReadCall},
	}
}

// nodeReadClippingRationale is the argument the two node-read rows owe.
const nodeReadClippingRationale = "Every fast read here is a node answering out of state it already holds, which " +
	"takes milliseconds; the ten-second class deadline is a ceiling for a node that " +
	"is barely answering, not an expectation. A dozen of them at their ceiling is a " +
	"node that cannot serve its own resource state, and being cut there is the right " +
	"outcome: what the operator needs then is the record marked unconfirmed, which is " +
	"exactly what a cut produces."

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
		route: "POST /api/v1/clusters",
		calls: append([]upstreamCall{
			{name: "ServerFingerprint: pre-trust TLS handshake", class: nodeProbeCall},
			{name: "NewClusterClient under the uploaded talosconfig: Version", class: nodeProbeCall},
			{name: "MachineConfigYAML: COSI Get", class: nodeFastReadCall},
			{name: "NewClusterClient under the minted certificate: Version", class: nodeProbeCall},
			{name: "connectivity proof: Version", class: nodeProbeCall},
		}, append(nodeFactsCalls(),
			upstreamCall{name: "adoptMembers/Members: cluster.Member List", class: nodeFastReadCall})...),
		routeDeadline: handlers.ImportRouteBudget,
		verdict:       withinBudget,
		clipping:      clipped,
		clippingRationale: "Every fast read here is a node answering out of state it already " +
			"holds, which takes milliseconds; the ten-second class deadline is a ceiling for " +
			"a node that is barely answering, not an expectation. Twelve of them at their " +
			"ceiling is a node that cannot serve its own resource state, and an adoption " +
			"that is still reading hardware facts ninety seconds in has not adopted " +
			"anything -- being cut there is the correct outcome and not a lost one. What " +
			"the clipping must not cut is the part that writes, and it cannot: the store " +
			"writes happen before adoptMembers runs, so a cut during the membership walk " +
			"leaves an adopted cluster whose inventory the supervisors fill in on their " +
			"next pass.",
		why: "inventory.Import makes these in series: the fingerprint probe, one connection " +
			"under the uploaded talosconfig (whose constructor proves itself with Version), " +
			"the single machine-configuration read this phase performs, a second connection " +
			"under the certificate minted from the derived bundle, that connection's own " +
			"Version as the D-04 connectivity proof, and then adoptMembers -- which is " +
			"NodeFacts, itself a fixed walk over the resource state, followed by the " +
			"membership list. The two NodeAddress reads are declared as two because the " +
			"unfiltered one is a fallback that runs when the filtered one came back empty, " +
			"which is the worst case this list is about. Whoever changes readNodeFacts has " +
			"to come back and revisit this count; nothing derives it.",
	},
	{
		route: "GET /api/v1/machines/{id}/config",
		calls: []upstreamCall{
			{name: "NewClusterClient: Version", class: nodeProbeCall},
			{name: "MachineConfigYAML: COSI Get", class: nodeFastReadCall},
		},
		routeDeadline: handlers.ConfigRouteBudget,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "machineconfig.Service.Get connects and reads the active MachineConfig resource, " +
			"then redacts and re-renders locally. The redaction and the rendering are CPU and " +
			"touch no node, so they are not upstream calls -- which is why this row is two " +
			"entries and not four.",
	},
	{
		route: "POST /api/v1/machines/{id}/config/plan",
		calls: []upstreamCall{
			{name: "NewClusterClient: Version", class: nodeProbeCall},
			{name: "MachineConfigYAML: COSI Get", class: nodeFastReadCall},
		},
		routeDeadline: handlers.ConfigRouteBudget,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "A plan is the same single read as the view. Everything after it -- merging the " +
			"patches, diffing, computing the apply mode, validating, checking idempotence by " +
			"merging a second time -- happens in this process against bytes it already has.",
	},
	{
		route: "POST /api/v1/machines/{id}/config/apply",
		calls: []upstreamCall{
			{name: "read: NewClusterClient Version", class: nodeProbeCall},
			{name: "read: MachineConfigYAML COSI Get", class: nodeFastReadCall},
			{name: "apply: NewClusterClient Version", class: nodeProbeCall},
			{name: "ApplyConfiguration", class: nodeMutationCall},
		},
		routeDeadline: handlers.ConfigRouteBudget,
		verdict:       withinBudget,
		clipping:      clipped,
		clippingRationale: "The mutation class is thirty seconds to *initiate*, and this ceiling is " +
			"thirty seconds for the whole route -- so the apply is clipped by whatever the read " +
			"before it consumed. That is the right direction for this route and not a compromise: " +
			"an apply whose read took twenty-nine seconds is an apply against a node that is " +
			"barely answering, and initiating a configuration change on such a node is the thing " +
			"to avoid rather than the thing to make more time for. The read happens first and " +
			"fails first.",
		why: "Service.Apply reads the current configuration, merges the patches locally, connects " +
			"again and applies. The second connection is a second Version probe because " +
			"NewClusterClient proves itself; it is not shared with the read's, because the read " +
			"closes its client before the merge.",
	},
	{
		route: "POST /api/v1/clusters/fingerprint",
		calls: []upstreamCall{
			{name: "ServerFingerprint: pre-trust TLS handshake", class: nodeProbeCall},
		},
		routeDeadline: handlers.FingerprintRouteBudget,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "clusterFingerprint opens one TLS connection to read the certificate the " +
			"operator is about to confirm, and reads nothing else. ServerFingerprint " +
			"carries the probe class itself, so the route ceiling is that plus room for " +
			"the handler's own work rather than a number chosen independently.",
	},
	{
		route: "POST /api/v1/machines",
		calls: append([]upstreamCall{
			{name: "NewClusterClient: Version", class: nodeProbeCall},
		}, append(nodeFactsCalls(), upstreamCall{name: "Members: cluster.Member List", class: nodeFastReadCall})...),
		routeDeadline:     handlers.NodeReadRouteBudget,
		verdict:           withinBudget,
		clipping:          clipped,
		clippingRationale: nodeReadClippingRationale,
		why: "inventory.AddManual connects, reads the node's facts and then asks for the " +
			"membership so the machine's role is read rather than guessed. The two " +
			"NodeAddress reads are declared as two because the unfiltered one runs when " +
			"the filtered one came back empty, which is the worst case. Whoever changes " +
			"readNodeFacts has to come back and revisit this count; nothing derives it.",
	},
	{
		route: "POST /api/v1/machines/{id}/refresh",
		calls: append([]upstreamCall{
			{name: "NewClusterClient: Version", class: nodeProbeCall},
		}, append(nodeFactsCalls(),
			upstreamCall{name: "Version", class: nodeFastReadCall},
			upstreamCall{name: "ServiceList", class: nodeFastReadCall},
			upstreamCall{name: "EtcdMemberList (control-plane nodes only)", class: nodeFastReadCall},
		)...),
		routeDeadline:     handlers.NodeReadRouteBudget,
		verdict:           withinBudget,
		clipping:          clipped,
		clippingRationale: nodeReadClippingRationale,
		why: "inventory.Refresh connects, walks the resource state, and then asks the three " +
			"typed RPCs that have no resource behind them. EtcdMemberList is in the list " +
			"because a control-plane node is the worst case; a worker skips it.",
	},
	{
		route: "POST /api/v1/provision/scan",
		calls: []upstreamCall{
			{name: "Dialer.Probe: TLS handshake (per address, bounded by ScanTimeout)", class: nodeProbeCall},
			{name: "ServerFingerprint: second handshake, per address that answered", class: nodeProbeCall},
		},
		routeDeadline: handlers.ScanRouteBudget,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "A scan is the one row here whose worst case is not the sum of its calls: the " +
			"addresses are probed ScanConcurrency at a time, each bounded by " +
			"provision.ScanTimeout, which is far shorter than the probe class. The two " +
			"declared calls are what *one* address costs in series -- a handshake to see " +
			"what is there and a second to read the certificate the operator will compare " +
			"against the console. The route ceiling covers a full /22 at that concurrency, " +
			"which is why it is the largest number in this table and why it is declared " +
			"here rather than inferred from the two classes.",
	},
	{
		route: "POST /api/v1/provision/inspect",
		calls: append([]upstreamCall{
			{name: "NewMaintenanceClient: Version", class: nodeProbeCall},
		}, append(nodeFactsCalls(),
			upstreamCall{name: "Version", class: nodeFastReadCall},
			upstreamCall{name: "Disks", class: nodeFastReadCall},
			upstreamCall{name: "ServerFingerprint: pre-trust TLS handshake", class: nodeProbeCall},
		)...),
		routeDeadline:     handlers.InspectRouteBudget,
		verdict:           withinBudget,
		clipping:          clipped,
		clippingRationale: nodeReadClippingRationale,
		why: "provision.Inspect connects to a machine in maintenance mode, walks the same " +
			"resource state an adopted node's read walks, asks for the disk list the picker " +
			"is built from, and reads the certificate fingerprint on its own connection. " +
			"The fingerprint is a separate handshake on purpose: a value the operator " +
			"confirms has to come from the connection they are about to trust.",
	},
	{
		route:         "POST /api/v1/provision/plan",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "The plan reads the cluster's control-plane count out of fsstore and then works " +
			"locally: it validates, warns and renders the install image. No ceiling, because " +
			"there is no upstream call to put one over -- the same shape as the schematic " +
			"list below.",
	},
	{
		route:         "POST /api/v1/provision/apply",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "The apply validates the same plan, checks the confirmation and submits a job, and " +
			"then answers. Every node call provisioning makes happens inside that job, on " +
			"the engine's own context and not on this request's -- which is the whole " +
			"reason a provisioning run survives a closed tab (PROV-11). This row is the " +
			"one place that is written down.",
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
