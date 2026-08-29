package talossim_test

import (
	"context"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/holzcloud/holzkube/internal/talossim"
)

// trans07Scenarios are the nine tokens REQUIREMENTS.md TRANS-07 names, written
// out as literals rather than derived from the package's own constants.
//
// Deriving them would make this test agree with any rename, which is precisely
// what it exists to prevent: the registry's keys are a claim about a
// requirement, and a claim that reads itself is not a check.
var trans07Scenarios = []string{
	"go_silent",
	"reject_apply",
	"second_bootstrap_returns_AlreadyExists",
	"flap_connection",
	"slow_log_consumer",
	"ip_changes_on_reboot",
	"etcd_down",
	"k8s_down",
	"version_out_of_supported_range",
}

// docPath is docs/talossim.md, relative to this package's directory.
const docPath = "../../docs/talossim.md"

// TestRegistryCoversTRANS07 asserts the registry is exactly the requirement.
//
// Both directions matter. A missing entry is a scenario TRANS-07 promises and
// the simulator cannot produce; an extra one is a fault nobody agreed to
// specify, carrying an expected-client string plan 02-05 would then assert
// against.
func TestRegistryCoversTRANS07(t *testing.T) {
	t.Parallel()

	registered := make([]string, 0, len(talossim.Registry))
	for name := range talossim.Registry {
		registered = append(registered, string(name))
	}

	for _, missing := range difference(trans07Scenarios, registered) {
		t.Errorf("scenario %q is named by TRANS-07 and absent from talossim.Registry", missing)
	}
	for _, extra := range difference(registered, trans07Scenarios) {
		t.Errorf("scenario %q is in talossim.Registry and is not named by TRANS-07", extra)
	}
}

// TestRegistryDefinesTheExpectedClientBehaviour asserts the second column
// exists for every scenario.
//
// ROADMAP success criterion 2 is that the client behaves definiert statt
// zufällig. An entry whose ExpectedClient is empty is a fault with no defined
// response, which makes the criterion unassertable and leaves plan 02-05 with
// nothing to code against.
func TestRegistryDefinesTheExpectedClientBehaviour(t *testing.T) {
	t.Parallel()

	for name, def := range talossim.Registry {
		if strings.TrimSpace(def.Sim) == "" {
			t.Errorf("scenario %q has no Sim description", name)
		}
		if strings.TrimSpace(def.ExpectedClient) == "" {
			t.Errorf("scenario %q has no ExpectedClient behaviour: a fault with no defined "+
				"client response cannot satisfy ROADMAP success criterion 2", name)
		}
		if def.Defaults.Name != name {
			t.Errorf("scenario %q carries Defaults.Name = %q; the defaults must name their own scenario",
				name, def.Defaults.Name)
		}
	}
}

// TestDocumentedScenariosMatchRegistry keeps docs/talossim.md and the registry
// from drifting apart.
//
// The document is the operator- and reviewer-facing half of the same table. A
// reviewer who reads a scenario there and cannot inject it, or injects one the
// document never mentions, has been told something untrue by a file whose only
// job is to be true.
func TestDocumentedScenariosMatchRegistry(t *testing.T) {
	t.Parallel()

	documented := documentedScenarios(t)

	registered := make([]string, 0, len(talossim.Registry))
	for name := range talossim.Registry {
		registered = append(registered, string(name))
	}

	for _, missing := range difference(registered, documented) {
		t.Errorf("scenario %q is in talossim.Registry and not in %s", missing, docPath)
	}
	for _, extra := range difference(documented, registered) {
		t.Errorf("scenario %q is documented in %s and is not in talossim.Registry", extra, docPath)
	}
}

// TestDocumentationRecordsTheEventsWatchHazard asserts the document still
// carries the reason no assertion in this phase checks only err.
//
// It is a fact about a third-party client that nothing in this repository
// would otherwise fail on. Losing the paragraph would cost the next author the
// day it takes to rediscover that a silent node reads as a clean success.
func TestDocumentationRecordsTheEventsWatchHazard(t *testing.T) {
	t.Parallel()

	doc := readDoc(t)

	for _, want := range []string{"EventsWatch", "io.EOF", "codes.Canceled", "codes.DeadlineExceeded"} {
		if !strings.Contains(doc, want) {
			t.Errorf("%s no longer mentions %q; the nil-on-EOF hazard is what makes every "+
				"assertion in this phase something other than err != nil", docPath, want)
		}
	}
}

// TestInjectRefusesAnUnknownScenario asserts the registry's default direction.
//
// A simulator that accepted an unknown name and did nothing would let a test
// suite accumulate green assertions against a fault that was never injected.
// Refusal makes the missing specification visible at the call.
func TestInjectRefusesAnUnknownScenario(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "unknown-scenario"})

	restore, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioName("disk_on_fire")})
	if err == nil {
		restore()
		t.Fatal("Inject accepted a scenario that is not in the registry")
	}
	if !strings.Contains(err.Error(), "disk_on_fire") {
		t.Errorf("error %q does not name the unknown scenario", err)
	}
	if len(sim.Active()) != 0 {
		t.Errorf("a refused injection left %d scenarios active", len(sim.Active()))
	}
}

// TestInjectAndClearRestorePreviousBehaviour is the composition property.
//
// Scenarios that leaked would make a test suite order-dependent: the fault
// injected in one test would still be active in the next, and the failure
// would surface as an unrelated flake. The assertion is deliberately made on a
// call's timing and status rather than on Active(), because Active() is the
// simulator agreeing with itself.
func TestInjectAndClearRestorePreviousBehaviour(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "roundtrip-node", TalosVersion: "v1.13.9"})
	cl := newMachineryClient(t, sim)

	if _, err := cl.Version(testContext(t)); err != nil {
		t.Fatalf("Version before any injection: %v", err)
	}

	restore, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioGoSilent, Duration: 5 * time.Second})
	if err != nil {
		t.Fatalf("Inject go_silent: %v", err)
	}

	active := sim.Active()
	if len(active) != 1 || active[0].Name != talossim.ScenarioGoSilent {
		t.Fatalf("Active() = %+v, want exactly go_silent", active)
	}
	if active[0].Duration != 5*time.Second {
		t.Errorf("Active()[0].Duration = %s, want the injected 5s", active[0].Duration)
	}

	silentCtx, cancel := shortContext(t, 300*time.Millisecond)
	defer cancel()

	if _, err := cl.Version(silentCtx); status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("Version against a silent node = %v (code %s), want DeadlineExceeded",
			err, status.Code(err))
	}

	restore()
	restore() // idempotent: a deferred cleanup plus an explicit one is the normal shape

	if len(sim.Active()) != 0 {
		t.Errorf("Active() = %+v after restore, want empty", sim.Active())
	}

	if _, err := cl.Version(testContext(t)); err != nil {
		t.Fatalf("Version after restore: %v; the node did not return to its pre-injection behaviour", err)
	}
}

// TestInjectDefaultsComeFromTheRegistry asserts an unset parameter is filled
// rather than left as a trap.
func TestInjectDefaultsComeFromTheRegistry(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "defaults-node"})

	restore, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioGoSilent})
	if err != nil {
		t.Fatalf("Inject go_silent with no duration: %v", err)
	}
	defer restore()

	active := sim.Active()
	if len(active) != 1 {
		t.Fatalf("Active() = %+v, want one scenario", active)
	}
	want := talossim.Registry[talossim.ScenarioGoSilent].Defaults.Duration
	if active[0].Duration != want {
		t.Errorf("Duration = %s, want the registry default %s", active[0].Duration, want)
	}
}

// docRowPattern matches the first cell of a markdown table row, which is where
// the scenario token lives.
var docRowPattern = regexp.MustCompile("^\\|\\s*`([^`]+)`\\s*\\|")

// documentedScenarios reads the scenario table out of docs/talossim.md.
//
// The table is delimited by HTML comments rather than found by heuristic: the
// document has more than one table, and a parser that guessed would either
// silently miss a renamed heading or start collecting the parameter table's
// field names as scenarios.
func documentedScenarios(t *testing.T) []string {
	t.Helper()

	doc := readDoc(t)

	const begin, end = "<!-- scenario-table:begin -->", "<!-- scenario-table:end -->"

	from := strings.Index(doc, begin)
	to := strings.Index(doc, end)
	if from < 0 || to < 0 || to < from {
		t.Fatalf("%s has no scenario table delimited by %q and %q", docPath, begin, end)
	}

	var found []string
	for _, line := range strings.Split(doc[from+len(begin):to], "\n") {
		if m := docRowPattern.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			found = append(found, m[1])
		}
	}
	if len(found) == 0 {
		t.Fatalf("the scenario table in %s has no rows", docPath)
	}
	return found
}

// shortContext is a context with a deliberately small deadline, for the
// scenarios whose whole point is that the caller's deadline fires first.
func shortContext(t *testing.T, d time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()

	return context.WithTimeout(t.Context(), d)
}

func readDoc(t *testing.T) string {
	t.Helper()

	b, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %s: %v", docPath, err)
	}
	return string(b)
}

// difference returns the elements of a that are not in b.
func difference(a, b []string) []string {
	var out []string
	for _, v := range a {
		if !slices.Contains(b, v) {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out
}
