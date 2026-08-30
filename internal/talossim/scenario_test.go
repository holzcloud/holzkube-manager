package talossim_test

import (
	"context"
	"errors"
	"net"
	"os"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/cosi-project/runtime/pkg/state"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/siderolabs/talos/pkg/machinery/api/common"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/resources/k8s"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
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

// TestScenarioGoSilent asserts the client's own deadline is what fires.
//
// The property is not "the call fails" -- a call against a silent node fails
// either way, eventually. It is that the failure arrives at the caller's
// deadline and not at the node's silence duration, which is the difference
// between a bounded transport and one that inherits whatever the far end feels
// like. The assertion is therefore a timing bound, not an error check, which is
// also the only kind of assertion the EventsWatch nil-on-EOF behaviour leaves
// available.
func TestScenarioGoSilent(t *testing.T) {
	t.Parallel()

	const silence = 8 * time.Second
	const callDeadline = 400 * time.Millisecond

	sim := newSim(t, talossim.Options{Hostname: "silent-node"})
	cl := newMachineryClient(t, sim)

	// A second node, never injected. The expected-client column says a
	// concurrent call to another target is unaffected; a silent node that took
	// the whole process down with it would satisfy every other assertion here.
	other := newSim(t, talossim.Options{Hostname: "talkative-node"})
	otherClient := newMachineryClient(t, other)

	restore, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioGoSilent, Duration: silence})
	if err != nil {
		t.Fatalf("Inject go_silent: %v", err)
	}
	defer restore()

	ctx, cancel := shortContext(t, callDeadline)
	defer cancel()

	started := time.Now()
	_, err = cl.Version(ctx)
	elapsed := time.Since(started)

	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("Version against a silent node = %v (code %s), want DeadlineExceeded", err, status.Code(err))
	}
	if elapsed >= silence {
		t.Errorf("the call took %s: it waited out the node's %s of silence instead of its own deadline",
			elapsed, silence)
	}
	if elapsed > callDeadline+2*time.Second {
		t.Errorf("the call took %s, want its own %s deadline plus a tolerance", elapsed, callDeadline)
	}

	if _, err := otherClient.Version(testContext(t)); err != nil {
		t.Errorf("a concurrent call to a second, uninjected node failed: %v", err)
	}
}

// TestScenarioFlapConnection asserts on the simulator's transition counter.
//
// Asserting that a call failed would be asserting on a race between the
// client's timing and the flap cycle, and it would pass just as happily
// against a scenario that did nothing but was slow. The counter is a fact
// about the listener.
func TestScenarioFlapConnection(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "flapping-node"})

	if got := sim.ListenerTransitions(); got != 0 {
		t.Fatalf("ListenerTransitions() = %d before injection, want 0", got)
	}

	before := sim.Addr()

	restore, err := sim.Inject(talossim.Scenario{
		Name:  talossim.ScenarioFlapConnection,
		Cycle: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Inject flap_connection: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for sim.ListenerTransitions() < 4 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}

	if got := sim.ListenerTransitions(); got < 4 {
		t.Errorf("ListenerTransitions() = %d after two cycles, want at least 4 "+
			"(two closes and two reopens)", got)
	}

	restore()

	// The node comes back on the address it had. A flap that moved the node
	// would be ip_changes_on_reboot, which is a different scenario with a
	// different expected client behaviour.
	if got := sim.Addr(); got != before {
		t.Errorf("Addr() = %q after flapping, want the unchanged %q", got, before)
	}
	if _, err := newMachineryClient(t, sim).Version(testContext(t)); err != nil {
		t.Errorf("Version after the flap was cleared: %v; the node did not come back", err)
	}
}

// TestFlappingNodeClosesCleanly is the T-02-16 half: a flapper must not
// outlive the node it flaps.
//
// The listener is opened and closed from a goroutine the injection owns. If
// Close did not drain it, the goroutine could re-listen after the gRPC server
// had stopped and hold the port for the rest of the test binary -- a leak that
// only shows up as an unrelated test failing to bind.
func TestFlappingNodeClosesCleanly(t *testing.T) {
	t.Parallel()

	sim, err := talossim.New(talossim.Options{Hostname: "leaky-node"})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}

	if _, err := sim.Inject(talossim.Scenario{
		Name:  talossim.ScenarioFlapConnection,
		Cycle: 20 * time.Millisecond,
	}); err != nil {
		t.Fatalf("Inject flap_connection: %v", err)
	}

	addr := sim.Addr()
	time.Sleep(100 * time.Millisecond)

	// Closed without clearing the scenario first, which is the case that
	// matters: a test that fails mid-flap runs its deferred Close and nothing
	// else.
	if err := sim.Close(); err != nil {
		t.Fatalf("Close with a flap still injected: %v", err)
	}

	// Whatever the flapper was doing, nothing is listening now. Given time for
	// any in-flight reopen, the address must refuse.
	time.Sleep(100 * time.Millisecond)
	assertRefused(t, addr)
}

// TestScenarioSlowLogConsumer asserts the simulator observed a blocked send,
// that the production it is blocking never ends, and that cancelling the
// caller tears the stream down promptly.
//
// The blocked-send record alone is not enough, and finding that out was the
// point of running this test against a deliberately disabled implementation: a
// bounded stream against a consumer that stops reading blocks too, so the
// record is green with or without the scenario. What the scenario adds is that
// the sequence has no end -- the consumer is not merely slow, it is behind a
// producer that will never finish -- and that is what the message count below
// pins. The client's error is deliberately not the assertion: the stream ends
// in Canceled, which machinery's own EventsWatch reports as a nil error.
func TestScenarioSlowLogConsumer(t *testing.T) {
	t.Parallel()

	const configured = 4

	sim := newSim(t, talossim.Options{Hostname: "slow-consumer-node", StreamMessages: configured})
	cl := newMachineryClient(t, sim)

	restore, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioSlowLogConsumer})
	if err != nil {
		t.Fatalf("Inject slow_log_consumer: %v", err)
	}
	defer restore()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	stream, err := cl.Logs(ctx, "system", common.ContainerDriver_CONTAINERD, "apid", true, -1)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}

	// Consume a little, then stop. The emitter keeps producing, so the next
	// send has nobody to hand its message to.
	for i := range 2 {
		if _, err := stream.Recv(); err != nil {
			t.Fatalf("Recv %d: %v", i, err)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	for sim.Streams().BlockedSends() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := sim.Streams().BlockedSends(); got == 0 {
		t.Fatal("the simulator never observed a blocked send against a consumer that stopped reading")
	}

	// Past the configured bound. Without the scenario the stream would have
	// ended at message four; a Recv that returns io.EOF here is a
	// slow_log_consumer that is registered, documented and inert.
	for received := 2; received < configured+4; received++ {
		if _, err := stream.Recv(); err != nil {
			t.Fatalf("the stream ended after %d messages (%v); slow_log_consumer did not make "+
				"production endless, so it is indistinguishable from an ordinary bounded stream",
				received, err)
		}
	}

	// Teardown within a second of the caller giving up. A stream that outlived
	// its caller would hold the emitter goroutine for the life of the process.
	torn := make(chan error, 1)
	go func() {
		for {
			if _, err := stream.Recv(); err != nil {
				torn <- err
				return
			}
		}
	}()

	cancel()

	select {
	case <-torn:
	case <-time.After(time.Second):
		t.Fatal("the stream was still open a second after the caller cancelled")
	}
}

// TestScenarioIPChangesOnReboot asserts the node moved and left the old
// address refusing.
//
// Refusing rather than timing out is the whole point: they are different
// client-observable outcomes, and only the refusing one models a machine that
// gave its address back. A black hole would be go_silent, which has a
// different expected client behaviour.
func TestScenarioIPChangesOnReboot(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "moving-node"})
	cl := newMachineryClient(t, sim)

	restore, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioIPChangesOnReboot})
	if err != nil {
		t.Fatalf("Inject ip_changes_on_reboot: %v", err)
	}
	defer restore()

	before := sim.Addr()

	if _, err := cl.RebootWithResponse(testContext(t)); err != nil {
		t.Fatalf("Reboot: %v", err)
	}

	after := sim.Addr()
	if after == before {
		t.Fatalf("Addr() is still %q after a reboot under ip_changes_on_reboot", after)
	}

	history := sim.AddressHistory()
	if len(history) < 2 || history[0] != before || history[len(history)-1] != after {
		t.Errorf("AddressHistory() = %v, want it to record %q then %q", history, before, after)
	}

	assertRefused(t, before)

	// The node is listening at its new address, so the change is a move and
	// not an outage. The assertion is a TCP accept rather than an RPC because
	// the in-process dialer these tests use bypasses the loopback socket
	// entirely -- asking it for a Version would answer from the pipe and prove
	// nothing about where the node is bound.
	assertAccepts(t, after)
}

// TestScenarioVersionOutOfRange asserts NewClusterClient refuses a node
// outside the supported window, and names both halves of the reason.
//
// An error that says only "unsupported" leaves an operator with a node that
// will not connect and no way to tell whether to upgrade it, downgrade it or
// upgrade holzkube-manager.
func TestScenarioVersionOutOfRange(t *testing.T) {
	t.Parallel()

	const reported = "v1.15.0"

	sim := newSim(t, talossim.Options{Hostname: "future-node", TalosVersion: "v1.13.9"})

	restore, err := sim.Inject(talossim.Scenario{
		Name:    talossim.ScenarioVersionOutOfSupportedRange,
		Version: reported,
	})
	if err != nil {
		t.Fatalf("Inject version_out_of_supported_range: %v", err)
	}

	target := talos.Target{
		Cluster: model.ClusterID("sim"),
		Machine: model.MachineID("00000000-0000-0000-0000-0000000000cc"),
		Addr:    sim.Host(),
	}

	cc, err := talos.NewClusterClient(testContext(t), sim.Dialer(), target, sim.ClientCreds(), talos.Mode{})
	if err == nil {
		_ = cc.Close()
		t.Fatal("NewClusterClient accepted a node reporting a version outside the supported range")
	}
	if !errors.Is(err, talos.ErrUnsupportedVersion) {
		t.Errorf("error %v does not satisfy errors.Is(err, talos.ErrUnsupportedVersion)", err)
	}
	for _, want := range []string{reported, talos.MinSupportedVersion, talos.MaxSupportedVersion} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}

	// Cleared, the same node is acceptable again -- the refusal was about the
	// version and not about the node.
	restore()

	cc, err = talos.NewClusterClient(testContext(t), sim.Dialer(), target, sim.ClientCreds(), talos.Mode{})
	if err != nil {
		t.Fatalf("NewClusterClient after the scenario was cleared: %v", err)
	}
	if err := cc.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// assertAccepts asserts that something is listening at addr.
func assertAccepts(t *testing.T, addr string) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		t.Fatalf("dial to %s: %v; the node is not listening where it says it is", addr, err)
	}
	if err := conn.Close(); err != nil {
		t.Errorf("close the probe connection: %v", err)
	}
}

// assertRefused asserts that a dial to addr is actively refused rather than
// hanging until a timeout.
func assertRefused(t *testing.T, addr string) {
	t.Helper()

	started := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err == nil {
		_ = conn.Close()
		t.Fatalf("a dial to the abandoned address %s succeeded", addr)
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		t.Errorf("dial to %s failed with %v after %s, want a connection refused: a released port "+
			"refuses, and only a black hole times out", addr, err, time.Since(started))
	}
}

// TestScenarioRejectApply asserts the refusal and, more importantly, the call
// count.
//
// The status code alone would be satisfied by a client that retried three
// times and reported the last refusal. ApplyConfiguration is in the mutation
// class and is never retried, and the only place that is observable is at the
// node.
func TestScenarioRejectApply(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "rejecting-node"})
	cl := newMachineryClient(t, sim)

	restore, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioRejectApply})
	if err != nil {
		t.Fatalf("Inject reject_apply: %v", err)
	}
	defer restore()

	before := sim.Calls("ApplyConfiguration")

	_, err = cl.ApplyConfiguration(testContext(t), &machine.ApplyConfigurationRequest{
		Data: []byte("version: v1alpha1\n"),
		Mode: machine.ApplyConfigurationRequest_AUTO,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("ApplyConfiguration = %v (code %s), want InvalidArgument", err, status.Code(err))
	}
	if !strings.Contains(err.Error(), "block device") {
		t.Errorf("error %q does not carry the upstream validation message", err)
	}

	if got := sim.Calls("ApplyConfiguration") - before; got != 1 {
		t.Errorf("the node saw %d ApplyConfiguration calls for one client call: a mutation was retried", got)
	}

	// Refused, not applied. PITFALLS.md P11 records that Talos rejects an
	// invalid configuration before writing anything, and a simulator that
	// counted the refusal as an apply would let the product ship a UI saying
	// the opposite.
	if got := sim.Node().AppliedConfigs; got != 0 {
		t.Errorf("AppliedConfigs = %d after a refused apply, want 0", got)
	}
}

// TestScenarioSecondBootstrap asserts both halves: the node's own refusal of a
// repeated bootstrap, and the scenario's precondition.
func TestScenarioSecondBootstrap(t *testing.T) {
	t.Parallel()

	t.Run("uninjected, the first succeeds and the second is refused", func(t *testing.T) {
		t.Parallel()

		sim := newSim(t, talossim.Options{Hostname: "bootstrap-node"})
		cl := newMachineryClient(t, sim)
		ctx := testContext(t)

		if err := cl.Bootstrap(ctx, &machine.BootstrapRequest{}); err != nil {
			t.Fatalf("the first Bootstrap: %v", err)
		}
		if err := cl.Bootstrap(ctx, &machine.BootstrapRequest{}); status.Code(err) != codes.AlreadyExists {
			t.Fatalf("the second Bootstrap = %v (code %s), want AlreadyExists", err, status.Code(err))
		}
		if got := sim.Node().BootstrapCalls; got != 2 {
			t.Errorf("BootstrapCalls = %d, want 2: a refusal is still a call", got)
		}
	})

	t.Run("injected, the client's own first call is already the second", func(t *testing.T) {
		t.Parallel()

		sim := newSim(t, talossim.Options{Hostname: "prebootstrapped-node"})
		cl := newMachineryClient(t, sim)

		restore, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioSecondBootstrapAlreadyExists})
		if err != nil {
			t.Fatalf("Inject second_bootstrap_returns_AlreadyExists: %v", err)
		}
		defer restore()

		if !sim.Node().Bootstrapped {
			t.Fatal("the scenario did not leave the node bootstrapped")
		}
		if got := sim.Node().BootstrapCalls; got != 0 {
			t.Errorf("BootstrapCalls = %d after injection alone, want 0: injection is not an RPC", got)
		}

		err = cl.Bootstrap(testContext(t), &machine.BootstrapRequest{})
		if status.Code(err) != codes.AlreadyExists {
			t.Fatalf("Bootstrap against an already-bootstrapped node = %v (code %s), want AlreadyExists",
				err, status.Code(err))
		}
	})
}

// TestScenarioEtcdDown asserts the asymmetry.
//
// A test that only showed the etcd call failing would pass against a node that
// had gone away entirely, which is the opposite diagnosis and the opposite fix.
// The Version call on the same connection is what makes the scenario mean "one
// subsystem is down".
func TestScenarioEtcdDown(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "etcd-down-node"})
	cl := newMachineryClient(t, sim)
	ctx := testContext(t)

	// Bootstrapped first, so that the failure below is the scenario and not
	// the FailedPrecondition an un-bootstrapped node answers with.
	if err := cl.Bootstrap(ctx, &machine.BootstrapRequest{}); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if _, err := cl.EtcdMemberList(ctx, &machine.EtcdMemberListRequest{}); err != nil {
		t.Fatalf("EtcdMemberList before injection: %v", err)
	}

	restore, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioEtcdDown})
	if err != nil {
		t.Fatalf("Inject etcd_down: %v", err)
	}
	defer restore()

	if _, err := cl.EtcdMemberList(ctx, &machine.EtcdMemberListRequest{}); status.Code(err) != codes.Unavailable {
		t.Errorf("EtcdMemberList = %v (code %s), want Unavailable", err, status.Code(err))
	}
	if _, err := cl.EtcdStatus(ctx); status.Code(err) != codes.Unavailable {
		t.Errorf("EtcdStatus = %v (code %s), want Unavailable", err, status.Code(err))
	}

	// The same connection, unchanged. The node is reachable; only etcd is not.
	if _, err := cl.Version(ctx); err != nil {
		t.Errorf("Version on the same connection: %v; etcd_down took the whole node down", err)
	}
	if _, err := cl.ServiceList(ctx); err != nil {
		t.Errorf("ServiceList on the same connection: %v", err)
	}
}

// TestScenarioK8sDown asserts a not-found-shaped result rather than an error,
// and a node that is still answering.
//
// Those two together are the diagnosis the scenario exists to make possible:
// Kubernetes is not running on a machine that is otherwise fine. A transport
// error would send an operator to look at the network.
func TestScenarioK8sDown(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "k8s-down-node"})
	cl := newMachineryClient(t, sim)
	ctx := testContext(t)

	if _, err := safe.StateGetByID[*k8s.Nodename](ctx, cl.COSI, k8s.NodenameID); err != nil {
		t.Fatalf("read %s before injection: %v", k8s.NodenameType, err)
	}

	restore, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioK8sDown})
	if err != nil {
		t.Fatalf("Inject k8s_down: %v", err)
	}

	_, err = safe.StateGetByID[*k8s.Nodename](ctx, cl.COSI, k8s.NodenameID)
	if err == nil {
		t.Fatal("the k8s resource is still readable with k8s_down injected")
	}
	if !state.IsNotFoundError(err) {
		t.Errorf("reading %s returned %v, want a not-found: a transport-shaped failure would "+
			"attribute a Kubernetes outage to the network", k8s.NodenameType, err)
	}

	// The connection is still usable, and the service view agrees with the
	// resource view.
	if _, err := cl.Version(ctx); err != nil {
		t.Errorf("Version with k8s_down injected: %v; the node is supposed to stay reachable", err)
	}

	services, err := cl.ServiceList(ctx)
	if err != nil {
		t.Fatalf("ServiceList: %v", err)
	}
	if kubelet := serviceByID(services.GetMessages()[0].GetServices(), "kubelet"); kubelet == nil {
		t.Error("ServiceList reports no kubelet at all")
	} else if kubelet.GetState() == "Running" {
		t.Error("ServiceList still reports the kubelet as Running while its resources are gone")
	}

	restore()

	if _, err := safe.StateGetByID[*k8s.Nodename](ctx, cl.COSI, k8s.NodenameID); err != nil {
		t.Errorf("read %s after the scenario was cleared: %v; k8s_down is a one-way door",
			k8s.NodenameType, err)
	}
}

func serviceByID(services []*machine.ServiceInfo, id string) *machine.ServiceInfo {
	for _, s := range services {
		if s.GetId() == id {
			return s
		}
	}
	return nil
}

// gateProbe exercises the RPC or the connection one scenario's Sim column
// names, and reduces what it saw to a comparable string.
type gateProbe struct {
	inject  talossim.Scenario
	options talossim.Options
	observe func(t *testing.T, sim *talossim.Server) string
}

// TestEveryScenarioIsImplemented is the completeness gate.
//
// It compares each scenario's injected behaviour against the same node's
// un-injected baseline, and fails when the two are indistinguishable. Asserting
// that Inject returned no error, or that nothing panicked, would not do: a
// scenario that is registered, documented and inert is the worst outcome
// available here, because it makes plan 02-05's contract suite pass against
// nothing at all while looking exactly like a suite that passed against
// something.
//
// The gate is also what keeps the registry from growing entries nobody
// implemented: a registry key with no probe below is a failure, not a skip.
func TestEveryScenarioIsImplemented(t *testing.T) {
	t.Parallel()

	probes := gateProbes()

	for name := range talossim.Registry {
		probe, ok := probes[name]
		if !ok {
			t.Errorf("scenario %q is in the registry and has no probe in the completeness gate; "+
				"an unprobed scenario is one nothing would notice had stopped working", name)
			continue
		}

		t.Run(string(name), func(t *testing.T) {
			t.Parallel()

			baseline := probe.observe(t, newSim(t, probe.options))

			sim := newSim(t, probe.options)
			restore, err := sim.Inject(probe.inject)
			if err != nil {
				t.Fatalf("Inject %s: %v", name, err)
			}
			defer restore()

			injected := probe.observe(t, sim)

			if injected == baseline {
				t.Errorf("scenario %q is indistinguishable from the un-injected baseline "+
					"(both observed %q): it is registered, documented and inert",
					name, injected)
			}
		})
	}

	for name := range probes {
		if _, ok := talossim.Registry[name]; !ok {
			t.Errorf("the completeness gate probes %q, which is not in the registry", name)
		}
	}
}

// gateProbes is one probe per registry entry.
//
// Each observation is deliberately coarse -- a status code, "moved" against
// "stable" -- because the gate's question is only whether the scenario changed
// anything. What it changed is the individual scenario tests' business.
func gateProbes() map[talossim.ScenarioName]gateProbe {
	return map[talossim.ScenarioName]gateProbe{
		talossim.ScenarioGoSilent: {
			inject: talossim.Scenario{Name: talossim.ScenarioGoSilent, Duration: 5 * time.Second},
			observe: func(t *testing.T, sim *talossim.Server) string {
				t.Helper()

				ctx, cancel := shortContext(t, 300*time.Millisecond)
				defer cancel()

				_, err := newMachineryClient(t, sim).Version(ctx)
				return status.Code(err).String()
			},
		},

		talossim.ScenarioRejectApply: {
			inject: talossim.Scenario{Name: talossim.ScenarioRejectApply},
			observe: func(t *testing.T, sim *talossim.Server) string {
				t.Helper()

				_, err := newMachineryClient(t, sim).ApplyConfiguration(testContext(t),
					&machine.ApplyConfigurationRequest{Data: []byte("version: v1alpha1\n")})
				return status.Code(err).String()
			},
		},

		talossim.ScenarioSecondBootstrapAlreadyExists: {
			inject: talossim.Scenario{Name: talossim.ScenarioSecondBootstrapAlreadyExists},
			observe: func(t *testing.T, sim *talossim.Server) string {
				t.Helper()

				err := newMachineryClient(t, sim).Bootstrap(testContext(t), &machine.BootstrapRequest{})
				return status.Code(err).String()
			},
		},

		talossim.ScenarioFlapConnection: {
			inject: talossim.Scenario{Name: talossim.ScenarioFlapConnection, Cycle: 30 * time.Millisecond},
			observe: func(t *testing.T, sim *talossim.Server) string {
				t.Helper()

				// Short: the injected cycle is 30 ms, so a listener that is
				// going to flap has flapped many times over by now, and the
				// baseline pays this wait in full on every run.
				deadline := time.Now().Add(time.Second)
				for sim.ListenerTransitions() == 0 && time.Now().Before(deadline) {
					time.Sleep(20 * time.Millisecond)
				}
				if sim.ListenerTransitions() > 0 {
					return "listener flapped"
				}
				return "listener stable"
			},
		},

		talossim.ScenarioSlowLogConsumer: {
			options: talossim.Options{StreamMessages: 3},
			inject:  talossim.Scenario{Name: talossim.ScenarioSlowLogConsumer},
			observe: func(t *testing.T, sim *talossim.Server) string {
				t.Helper()

				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()

				stream, err := newMachineryClient(t, sim).Logs(ctx, "system",
					common.ContainerDriver_CONTAINERD, "apid", true, -1)
				if err != nil {
					t.Fatalf("Logs: %v", err)
				}

				for range 6 {
					if _, err := stream.Recv(); err != nil {
						return "production is bounded"
					}
				}
				return "production is endless"
			},
		},

		talossim.ScenarioIPChangesOnReboot: {
			inject: talossim.Scenario{Name: talossim.ScenarioIPChangesOnReboot},
			observe: func(t *testing.T, sim *talossim.Server) string {
				t.Helper()

				before := sim.Addr()
				if _, err := newMachineryClient(t, sim).RebootWithResponse(testContext(t)); err != nil {
					t.Fatalf("Reboot: %v", err)
				}
				if sim.Addr() != before {
					return "the node moved"
				}
				return "the node stayed"
			},
		},

		talossim.ScenarioEtcdDown: {
			inject: talossim.Scenario{Name: talossim.ScenarioEtcdDown},
			observe: func(t *testing.T, sim *talossim.Server) string {
				t.Helper()

				cl := newMachineryClient(t, sim)
				ctx := testContext(t)

				// Bootstrapped first: an un-bootstrapped node answers
				// FailedPrecondition, and a baseline that already failed would
				// hide a scenario that changed nothing.
				if err := cl.Bootstrap(ctx, &machine.BootstrapRequest{}); err != nil {
					t.Fatalf("Bootstrap: %v", err)
				}
				_, err := cl.EtcdMemberList(ctx, &machine.EtcdMemberListRequest{})
				return status.Code(err).String()
			},
		},

		talossim.ScenarioK8sDown: {
			inject: talossim.Scenario{Name: talossim.ScenarioK8sDown},
			observe: func(t *testing.T, sim *talossim.Server) string {
				t.Helper()

				cl := newMachineryClient(t, sim)
				if _, err := safe.StateGetByID[*k8s.Nodename](testContext(t), cl.COSI, k8s.NodenameID); err != nil {
					return "kubernetes resources absent"
				}
				return "kubernetes resources present"
			},
		},

		talossim.ScenarioVersionOutOfSupportedRange: {
			inject: talossim.Scenario{
				Name:    talossim.ScenarioVersionOutOfSupportedRange,
				Version: "v1.15.0",
			},
			observe: func(t *testing.T, sim *talossim.Server) string {
				t.Helper()

				resp, err := newMachineryClient(t, sim).Version(testContext(t))
				if err != nil {
					t.Fatalf("Version: %v", err)
				}
				tag := resp.GetMessages()[0].GetVersion().GetTag()
				if err := talos.CheckSupportedVersion(tag); err != nil {
					return "outside the supported range"
				}
				return "inside the supported range"
			},
		},
	}
}
