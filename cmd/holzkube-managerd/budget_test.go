package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/httpapi/handlers"
	"github.com/holzcloud/holzkube-manager/internal/imagefactory"
	"github.com/holzcloud/holzkube-manager/internal/kube"
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

	// The Kubernetes API's class, added with milestone v1.17. A fourth family:
	// the three above are the Image Factory's and the Talos node's, and this is
	// a cluster's own API server, reached with client-go.
	//
	// It is one number rather than a table of verbs, and that is the honest
	// description of what the client does today: internal/kube sets a single
	// ceiling on its rest configuration, and every call this product makes to
	// Kubernetes is a list or a get. When the later slices add evictions and
	// applies, the number they need is a decision, and a second constant here
	// is where that decision will be visible.
	kubeCall callClass = "kube"
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
	case kubeCall:
		return kube.CallBudget, true
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
		route: "POST /api/v1/clusters/{id}/upgrade/plan",
		calls: append([]upstreamCall{
			{name: "NewClusterClient: Version (per node)", class: nodeProbeCall},
		}, append(nodeFactsCalls(),
			upstreamCall{name: "SecurityState (per node -- which installer it needs)", class: nodeFastReadCall},
			upstreamCall{name: "KernelCmdline + MachineConfig (per node -- UPG-04 drift)", class: nodeFastReadCall},
			upstreamCall{name: "gate: EtcdStatus (per control-plane node)", class: nodeFastReadCall},
			upstreamCall{name: "gate: EtcdMemberList (per control-plane node)", class: nodeFastReadCall},
			upstreamCall{name: "gate: EtcdAlarmList (per control-plane node)", class: nodeFastReadCall},
		)...),
		routeDeadline:     handlers.UpgradeReadRouteBudget,
		verdict:           withinBudget,
		clipping:          clipped,
		clippingRationale: nodeReadClippingRationale,
		why: "The most expensive read in this product, and still a read: it connects to every " +
			"node to read the schematic Talos recorded at install time and how that node " +
			"booted -- the second decides whether it needs the SecureBoot installer -- then " +
			"runs the health " +
			"gate's three etcd reads against every control-plane node. The declared calls are " +
			"what *one* node costs in series; the ceiling covers a homelab-sized cluster of " +
			"them, which is the size this product is for. A cluster large enough to exceed it " +
			"is a cluster whose plan should be built a few nodes at a time, and the ceiling " +
			"saying so is better than a request that never ends.",
	},
	{
		route: "POST /api/v1/clusters/{id}/upgrade/kubernetes/plan",
		calls: []upstreamCall{
			{name: "gate: NewClusterClient Version (per control-plane node)", class: nodeProbeCall},
			{name: "gate: EtcdStatus", class: nodeFastReadCall},
			{name: "gate: EtcdMemberList", class: nodeFastReadCall},
			{name: "gate: EtcdAlarmList", class: nodeFastReadCall},
		},
		routeDeadline: handlers.UpgradeReadRouteBudget,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "Shorter than the Talos plan by exactly the reads that do not apply: a Kubernetes " +
			"upgrade writes no installer image, so there is no schematic to read off each node. " +
			"What is left is the health gate and the versions already in the store. It shares " +
			"the Talos plan's ceiling because it is the same screen.",
	},
	{
		route: "POST /api/v1/clusters/{id}/upgrade",
		calls: append([]upstreamCall{
			{name: "NewClusterClient: Version (per node)", class: nodeProbeCall},
		}, append(nodeFactsCalls(),
			upstreamCall{name: "SecurityState (per node -- which installer it needs)", class: nodeFastReadCall},
			upstreamCall{name: "KernelCmdline + MachineConfig (per node -- UPG-04 drift)", class: nodeFastReadCall},
			upstreamCall{name: "gate: EtcdStatus (per control-plane node)", class: nodeFastReadCall},
			upstreamCall{name: "gate: EtcdMemberList (per control-plane node)", class: nodeFastReadCall},
			upstreamCall{name: "gate: EtcdAlarmList (per control-plane node)", class: nodeFastReadCall},
		)...),
		routeDeadline:     handlers.UpgradeReadRouteBudget,
		verdict:           withinBudget,
		clipping:          clipped,
		clippingRationale: nodeReadClippingRationale,
		why: "The submission rebuilds the plan before accepting it -- what the job walks has to " +
			"be what the server just checked -- so it pays the plan route's reads and then " +
			"submits a job. Every call the upgrade itself makes happens inside that job, on " +
			"the engine's context and not on this request's: the ImagePull, the streaming " +
			"upgrade and the verification are all after this response has been written.",
	},
	{
		route: "POST /api/v1/clusters/{id}/upgrade/kubernetes",
		calls: []upstreamCall{
			{name: "gate: NewClusterClient Version (per control-plane node)", class: nodeProbeCall},
			{name: "gate: EtcdStatus", class: nodeFastReadCall},
			{name: "gate: EtcdMemberList", class: nodeFastReadCall},
			{name: "gate: EtcdAlarmList", class: nodeFastReadCall},
		},
		routeDeadline: handlers.UpgradeReadRouteBudget,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "As the Talos submission, against the shorter plan: rebuild, check the " +
			"confirmation, submit. The rolling apply happens in the job.",
	},
	{
		route: "POST /api/v1/clusters/{id}/upgrade/confirm",
		calls: append([]upstreamCall{
			{name: "NewClusterClient: Version (per node)", class: nodeProbeCall},
		}, append(nodeFactsCalls(),
			upstreamCall{name: "SecurityState (per node -- which installer it needs)", class: nodeFastReadCall},
			upstreamCall{name: "KernelCmdline + MachineConfig (per node -- UPG-04 drift)", class: nodeFastReadCall},
			upstreamCall{name: "gate: EtcdStatus (per control-plane node)", class: nodeFastReadCall},
			upstreamCall{name: "gate: EtcdMemberList (per control-plane node)", class: nodeFastReadCall},
			upstreamCall{name: "gate: EtcdAlarmList (per control-plane node)", class: nodeFastReadCall},
		)...),
		routeDeadline:     handlers.UpgradeReadRouteBudget,
		verdict:           withinBudget,
		clipping:          clipped,
		clippingRationale: nodeReadClippingRationale,
		why: "It builds the same plan the submission will, and that is the point rather than an " +
			"inefficiency: a token issued against one plan and a run built from another would " +
			"be a confirmation of something that did not happen.",
	},
	{
		route: "GET /api/v1/clusters/{id}/etcd/members",
		calls: []upstreamCall{
			{name: "NewClusterClient: Version", class: nodeProbeCall},
			{name: "EtcdMemberList", class: nodeFastReadCall},
		},
		routeDeadline: handlers.EtcdRouteBudget,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "One connection to the first control-plane node that answers, and one read. It " +
			"tries the next node when one does not answer, which is why the ceiling has room " +
			"for more than one attempt.",
	},
	{
		route: "DELETE /api/v1/clusters/{id}/etcd/members/{member}",
		calls: []upstreamCall{
			{name: "NewClusterClient: Version", class: nodeProbeCall},
			{name: "EtcdMemberList (to decide whether the quorum survives)", class: nodeFastReadCall},
			{name: "NewClusterClient: Version (a different node -- a member cannot remove itself)", class: nodeProbeCall},
			{name: "EtcdRemoveMemberByID", class: nodeMutationCall},
		},
		routeDeadline: handlers.EtcdRouteBudget,
		verdict:       withinBudget,
		clipping:      clipped,
		clippingRationale: "The mutation's thirty-second budget is a ceiling for a node that is " +
			"barely answering, and the two probes and the read in front of it take milliseconds " +
			"on a node that is well. Forty-five seconds covers the realistic series; a run that " +
			"reaches the ceiling is a control plane that cannot answer a membership read, and " +
			"being cut there is right -- what the operator needs then is to know the cluster is " +
			"not well, not a removal that eventually goes through.",
		why: "The membership is read before the removal because the decision -- would this leave " +
			"the cluster without a quorum -- is about the membership as it is now. The second " +
			"connection is to a different node, because a member cannot remove itself.",
	},
	{
		route:         "GET /api/v1/clusters/{id}/etcd/snapshot",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "A stream, and the only row here that declares no ceiling for a route that does " +
			"reach a node. That is deliberate: EtcdSnapshot is in the stream deadline class, " +
			"which carries a first-byte deadline and an idle timeout rather than a total one, " +
			"and a multi-gigabyte etcd on a slow disk takes as long as it takes. A total " +
			"deadline here would fail the backups that most need to succeed. The route is " +
			"marked Streaming, so the write timeout does not apply to it either.",
	},
	{
		route:         "POST /api/v1/machines/{id}/etcd/restore",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "The third row that reaches a node and declares no ceiling, and the first that is a " +
			"route receiving rather than sending. The upload is EtcdRecover, which is in the " +
			"upload deadline class: no total deadline, because a total deadline on it is a bound " +
			"on how large somebody's etcd is allowed to be. The route is not marked Streaming -- " +
			"that flag is about the response, and this response is a small JSON body -- so what " +
			"the handler clears instead is the server's READ deadline, with " +
			"http.ResponseController, for the same reason and on the other side. The bootstrap " +
			"that follows the upload does carry its class deadline; it is one call and it is " +
			"bounded.",
	},
	{
		route:         "POST /api/v1/machines/{id}/lock",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "store-only: a lock is holzkube-manager's own note about what it should not do, and " +
			"writing it reaches no node. That is why it works on a node that is down, which is " +
			"precisely when somebody wants to set one.",
	},
	{
		route: "POST /api/v1/machines/{id}/remove-from-cluster",
		calls: []upstreamCall{
			{name: "NewClusterClient: Version", class: nodeProbeCall},
			{name: "EtcdMemberList (control-plane nodes only -- does the cluster survive this)", class: nodeFastReadCall},
			{name: "EtcdLeaveCluster (control-plane nodes only)", class: nodeMutationCall},
			{name: "Reset", class: nodeMutationCall},
		},
		routeDeadline: handlers.EtcdRouteBudget,
		verdict:       withinBudget,
		clipping:      clipped,
		clippingRationale: "Two mutations in series, each with a thirty-second ceiling, and the " +
			"route gives them forty-five between them. That is not a route hoping they are " +
			"quick: both are calls that *initiate* -- the etcd leave returns when the member " +
			"has been removed from the membership, and the reset returns when the node has " +
			"accepted it, neither waits for the work -- so the ceilings are for a node that is " +
			"barely answering. A removal cut at forty-five seconds is a node that could not be " +
			"told to leave etcd, and stopping there is better than wiping it anyway.",
		why: "The node leaves etcd and is then wiped, in that order and on one connection. " +
			"Doing the removal from another node while this one still runs leaves a member " +
			"that believes it is in a cluster that has forgotten it. The membership read in " +
			"front of both is what decides whether the cluster survives losing this node, and " +
			"it is first because a refusal after the etcd leave would be a refusal in name.",
	},
	{
		route: "POST /api/v1/clusters/{id}/client-certificate",
		calls: []upstreamCall{
			{name: "NewClusterClient: Version (proving the new certificate, first node that answers)", class: nodeProbeCall},
		},
		routeDeadline: handlers.EtcdRouteBudget,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "One connection, and it is the whole point of the route rather than an " +
			"incidental read: the certificate is minted locally and written only after a node " +
			"has answered through it. Every machine in the cluster is tried in turn, which is " +
			"why the ceiling has room for more than one attempt -- a cluster where the first " +
			"node happens to be switched off must not conclude that its certificate is bad.",
	},
	{
		route: "GET /api/v1/clusters/{id}/kubernetes",
		calls: []upstreamCall{
			{name: "NewClusterClient: Version (finding a control-plane node)", class: nodeProbeCall},
			{name: "COSI Get: the machine configuration, for the API server's address", class: nodeFastReadCall},
			{name: "Kubernetes: /version", class: kubeCall},
			{name: "Kubernetes: list nodes", class: kubeCall},
			{name: "Kubernetes: list pods", class: kubeCall},
			{name: "Kubernetes: list namespaces", class: kubeCall},
		},
		routeDeadline: handlers.KubernetesRouteBudget,
		verdict:       knownOverBudget,
		clipping:      clipped,
		deferredTo: "milestone v1.17 slice 2's own follow-up: the four Kubernetes reads are " +
			"issued one after another and could be one call each against a paginated list, " +
			"or concurrent. Until they are, the sum is larger than the ceiling and this row " +
			"says so rather than a number being quietly chosen.",
		clippingRationale: "The ceiling is sized for an API server that answers, which is what a " +
			"list call against Kubernetes does in milliseconds. Summing six per-call budgets " +
			"describes a case that would mean every one of them timed out in turn -- at which " +
			"point the answer an operator needs is 'this cluster is not answering', and " +
			"forty-five seconds is long enough to establish that and short enough that the " +
			"screen says it rather than hanging.",
		why: "The first route in this product that speaks to two upstreams in series, and the " +
			"one row in this table whose worst case is deliberately larger than its ceiling. " +
			"Two Talos calls find the API server's address -- the endpoint is read from the " +
			"node's own configuration rather than assembled, because a cluster behind a " +
			"virtual address is where assembling is wrong -- and then four reads go to the " +
			"cluster. Summing every per-call budget gives a number no answer ever takes: " +
			"these are list calls against an API server that either answers in milliseconds " +
			"or is not answering at all, which is what the ceiling is sized for. The clipping " +
			"is the point rather than an oversight: a screen that waited out the full sum " +
			"would be a screen nobody keeps open, and a cluster that needs longer than " +
			"forty-five seconds to list its pods has a finding of its own.",
	},
	{
		route: "POST /api/v1/clusters/{id}/kubernetes/nodes/{node}/cordon",
		calls: []upstreamCall{
			{name: "NewClusterClient: Version (finding a control-plane node)", class: nodeProbeCall},
			{name: "COSI Get: the machine configuration, for the API server's address", class: nodeFastReadCall},
			{name: "Kubernetes: patch the node", class: kubeCall},
			{name: "Kubernetes: list nodes (reading the result back)", class: kubeCall},
		},
		routeDeadline: handlers.KubernetesRouteBudget,
		verdict:       knownOverBudget,
		clipping:      clipped,
		deferredTo: "the same follow-up as the overview row: the two Talos calls that find the " +
			"API server's address are repeated per request and could be a short-lived cache, " +
			"which is a decision about staleness rather than a tidy-up.",
		clippingRationale: "A patch and a list against an API server that answers are " +
			"milliseconds. The sum describes every call timing out in turn, and at that point " +
			"the answer is that the cluster is not reachable -- which forty-five seconds " +
			"establishes.",
		why: "The node is read back after the patch rather than echoed, so a cordon that did " +
			"not take cannot look like one that did. That is the second Kubernetes call and it " +
			"is deliberate.",
	},
	{
		route: "GET /api/v1/clusters/{id}/scale",
		calls: []upstreamCall{
			{name: "NewClusterClient: Version", class: nodeProbeCall},
			{name: "EtcdMemberList", class: nodeFastReadCall},
		},
		routeDeadline: handlers.EtcdRouteBudget,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "The same pair as the member list, because the membership is the only thing on " +
			"this route that leaves the process -- everything else is records already held. " +
			"A membership that cannot be read does not fail the request: it is carried into " +
			"the plan as the reason no control-plane node is offered for removal, because a " +
			"cluster whose etcd is unreachable is exactly the cluster somebody is on this " +
			"screen about.",
	},
	{
		route:         "GET /api/v1/upgrade/releases",
		calls:         []upstreamCall{{name: "Factory: versions", class: jsonCall}},
		routeDeadline: handlers.EtcdRouteBudget,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "One Image Factory call. It shares the etcd ceiling rather than declaring a third " +
			"number, because both are 'one upstream call plus room for the handler'.",
	},
	{
		route: "POST /api/v1/clusters/create",
		calls: []upstreamCall{
			{name: "NewClusterClient: Version", class: nodeProbeCall},
		},
		routeDeadline: handlers.ImportRouteBudget,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "Creating a cluster mints its PKI locally and then proves the credentials reach " +
			"something: the connectivity check D-04 asks for is one connection, whose Version " +
			"probe is the proof. It shares the import route's ceiling because it is the second " +
			"half of the same operation -- this row exists because the completeness guard found " +
			"that it never had one.",
	},
	{
		route:         "GET /api/v1/users",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "store-only: accounts live in this installation's own store and nothing about them " +
			"is on a node. The four rows below are the same, and they are listed one by one " +
			"rather than as a prefix because the guard matches whole routes -- a prefix rule " +
			"here would silently cover a future /api/v1/users/{id}/something that does reach " +
			"one.",
	},
	{
		route:         "POST /api/v1/users",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why:           "store-only, and one argon2id hash, which is bounded by its own calibration.",
	},
	{
		route:         "POST /api/v1/users/{id}/role",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why:           "store-only.",
	},
	{
		route:         "POST /api/v1/users/{id}/password",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why:           "store-only, and one argon2id hash.",
	},
	{
		route:         "DELETE /api/v1/users/{id}",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why:           "store-only.",
	},
	{
		route:         "POST /api/v1/service-accounts",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "store-only. Note what it deliberately does NOT do: minting a token is one read " +
			"from crypto/rand and one SHA-256, not an argon2id hash. A token is 256 bits and " +
			"stretching it improves nothing, while the stretch would be paid on every API call " +
			"a machine makes.",
	},
	{
		route:         "POST /api/v1/service-accounts/{id}/token",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why:           "store-only.",
	},
	{
		route:         "PUT /api/v1/machines/{id}/labels",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "store-only, and deliberately so: a label is the operator's word about a machine " +
			"and nothing about it is on the machine. That is the same property that makes a " +
			"label safe to select on -- no observation ever overwrites one.",
	},
	{
		route:         "GET /api/v1/machine-classes",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "store-only. It answers each class's selector against the stored inventory rather " +
			"than asking any node, which is what makes the membership a question re-answered on " +
			"every read instead of a second thing to keep in step with the labels.",
	},
	{
		route:         "PUT /api/v1/machine-classes/{id}",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why:           "store-only.",
	},
	{
		route:         "DELETE /api/v1/machine-classes/{id}",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why:           "store-only.",
	},
	{
		route:         "POST /api/v1/cluster-templates/plan",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "store-only, and that is the scope of the feature rather than an accident of this " +
			"route: a plan says what a template would mean for the machines this installation " +
			"already knows about. Applying one is provisioning, and there is no route here that " +
			"does it.",
	},
	{
		route:         "GET /api/v1/clusters/{id}/template",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why:           "store-only: the export is written from the stored records.",
	},
	{
		route:         "GET /api/v1/clusters/{id}/talosconfig",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "store-only: the talosconfig is rendered from the stored bundle and reaches no " +
			"node. Found by the completeness guard along with the row above.",
	},
	{
		route: "GET /api/v1/clusters/{id}/kubeconfig",
		// Five nodes' worth, because this route is a loop and the table has to
		// say how long a loop it is willing to describe. Five is the largest
		// control plane anybody runs; a cluster with more is not a case this
		// ceiling was sized for and the ceiling is what stops it.
		calls:         kubeconfigAttempts(5),
		routeDeadline: handlers.KubeconfigRouteBudget,
		verdict:       withinBudget,
		clipping:      clipped,
		clippingRationale: "The route tries control-plane nodes in turn and stops at the first " +
			"that answers, so the calls above are one node's pair repeated. Sixty seconds is " +
			"four of those pairs at their ceilings, which covers what the fallback exists for: " +
			"a cluster whose first node or two are down. It deliberately does not cover five " +
			"nodes all timing out, because at that point the operator has a cluster-wide " +
			"problem that every other screen is already telling them about, and a longer wait " +
			"adds nothing but the wait. A ceiling that cannot be reached is not a ceiling.",
	},
	{
		route:         "GET /api/v1/clusters/{id}/support-bundle",
		calls:         nil,
		routeDeadline: 0,
		verdict:       withinBudget,
		clipping:      uncut,
		why: "A stream, and the second row here that declares no ceiling for a route that does " +
			"reach nodes -- the etcd snapshot is the other. The reason is the same and it is " +
			"stronger here: a bundle over a fleet connects to every node in turn, and the whole " +
			"point of it is being taken while things are broken, which is when every one of " +
			"those connections is slow. A route ceiling would cut exactly the bundle somebody " +
			"needs. What bounds it instead is per node: support.PerNodeTimeout, so one node " +
			"answering slowly cannot eat the budget of the nodes after it, and each read inside " +
			"carries its own deadline class. The route is marked Streaming, so the server's " +
			"write timeout does not apply either.",
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

// TestEveryRouteThatReachesUpstreamHasABudgetRow closes the same hole
// allowlist_test.go closed.
//
// The table above is hand-declared, and it has to be: no static analysis
// available here can count sequential upstream calls through a handler. What
// *can* be derived is the set of routes, and a row that nobody wrote for a
// route that exists is the failure mode of every hand-maintained list -- the
// table keeps passing while the route it does not know about composes to
// whatever it composes to.
//
// It does not demand a row for every route. A read that touches nothing has
// nothing to budget, and demanding a row for each of them would make the table
// a list of the whole API and stop anybody reading it. What it demands is that
// a route either has a row or is named below as one that needs none.
func TestEveryRouteThatReachesUpstreamHasABudgetRow(t *testing.T) {
	t.Parallel()

	declared := map[string]bool{}
	for _, row := range routeBudgets {
		declared[row.route] = true
	}

	// Routes that reach no node and no upstream service: authentication, the
	// store-backed reads and writes, the stream fan-out. Each is named rather
	// than matched by prefix, so that adding one is a deliberate edit here.
	noUpstream := map[string]bool{}
	for _, r := range []string{
		"GET /api/v1/system/status", "GET /api/v1/system/dry-run",
		"GET /api/v1/system/version",
		"POST /api/v1/setup", "GET /api/v1/setup",
		"POST /api/v1/auth/login", "POST /api/v1/auth/logout", "POST /api/v1/auth/sudo",
		"GET /api/v1/auth/me",
		"GET /api/v1/auth/oidc/start", "GET /api/v1/auth/oidc/callback",
		"POST /api/v1/account/password",
		"GET /api/v1/audit",
		"GET /api/v1/schematics/{id}", "DELETE /api/v1/schematics/{id}",
		"GET /api/v1/machines", "GET /api/v1/machines/{id}",
		"GET /api/v1/clusters", "GET /api/v1/clusters/{id}",
		"POST /api/v1/clusters/{id}/lock",
		"DELETE /api/v1/clusters/{id}", "DELETE /api/v1/machines/{id}",
		"GET /api/v1/stream",
		"GET /api/v1/jobs", "GET /api/v1/jobs/{id}", "POST /api/v1/jobs/{id}/cancel",
		"GET /api/v1/machines/{id}/reset-preview", "POST /api/v1/machines/{id}/confirm",
		"POST /api/v1/machines/{id}/reboot", "POST /api/v1/machines/{id}/shutdown",
		"POST /api/v1/machines/{id}/reset",
		// Draining a node submits a job and answers 202. Nothing upstream is
		// called while the request is open, which is the whole reason a drain
		// is a job: it waits out each pod's termination grace period and would
		// outlast any response this server is willing to hold open. Its
		// ceiling is kube.DrainBudget and belongs to the job.
		"POST /api/v1/clusters/{id}/kubernetes/nodes/{node}/drain",
		"GET /api/v1/patches", "POST /api/v1/patches", "GET /api/v1/patches/{id}",
		"GET /api/v1/provision/notices", "POST /api/v1/provision/scan",
		"POST /api/v1/provision/inspect", "POST /api/v1/provision/plan",
		"POST /api/v1/provision/confirm", "POST /api/v1/provision/apply",
		"GET /api/v1/provision/bootstrap-recovery",
		"POST /api/v1/provision/bootstrap-recovery/{cluster}",
		"GET /api/v1/factory/versions", "GET /api/v1/factory/extensions",
		// The Prometheus export reads the read model the supervisors already
		// filled and the job records in the store. It reaches no node, which
		// is the point: a scrape that dialled the fleet would put a request
		// every fifteen seconds against machines that may be the reason
		// somebody is looking.
		"GET /metrics",
	} {
		noUpstream[r] = true
	}

	var missing []string
	for _, route := range routeTable(httpapi.Deps{}) {
		name := route.Method + " " + route.Pattern
		if declared[name] || noUpstream[name] {
			continue
		}
		missing = append(missing, name)
	}

	if len(missing) > 0 {
		t.Fatalf("these routes have no row in the budget table and are not named as reaching "+
			"nothing:\n  %s\n\nEither add a row -- with its sequential upstream calls and the "+
			"ceiling it declares -- or name it above as a route that reaches no node. A route "+
			"the table does not know about composes to whatever it composes to, which is the "+
			"failure this guard exists to catch.", strings.Join(missing, "\n  "))
	}
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

// kubeconfigAttempts is n rounds of the pair the kubeconfig route makes per
// control-plane node it tries.
func kubeconfigAttempts(n int) []upstreamCall {
	calls := make([]upstreamCall, 0, n*2)
	for i := range n {
		calls = append(calls,
			upstreamCall{
				name:  fmt.Sprintf("control-plane node %d: Connect (Version)", i+1),
				class: nodeProbeCall,
			},
			upstreamCall{
				name:  fmt.Sprintf("control-plane node %d: Kubeconfig", i+1),
				class: nodeFastReadCall,
			},
		)
	}
	return calls
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
