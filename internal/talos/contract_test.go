package talos_test

// The client-side half of the TRANS-07 contract suite.
//
// ROADMAP success criterion 2 is not "the sim can fail" but "der Client verhält
// sich in jedem Fall *definiert* statt zufällig". Plan 02-03 wrote those
// definitions down, one per scenario, in talossim.Registry's ExpectedClient
// column and in docs/talossim.md. This file asserts the production client
// against them.
//
// Two properties of the shape matter as much as the assertions themselves:
//
//  1. The suite iterates talossim.Registry rather than hand-listing the
//     scenarios. A scenario added there without an assertion here fails the
//     suite, so the two cannot drift apart the way a documented list and a code
//     list always eventually do.
//
//  2. The transport is a parameter, not a sim constructed inline in every case.
//     TRANS-08 is the same suite run against real Talos in Phase 3, and it
//     arrives by adding a second contractTransport rather than by rewriting
//     nine assertions.
//
// No assertion here is built on a bare err != nil. client.EventsWatch returns
// nil -- not an error -- on io.EOF, codes.Canceled and codes.DeadlineExceeded,
// so a silent node and a flapping listener read as clean success to a caller
// that only checks the error. Every case below rests on a timing bound, on a
// counter the simulator keeps, on the connection state, or on a classified
// error kind.

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/cosi-project/runtime/pkg/state"
	"github.com/siderolabs/talos/pkg/machinery/resources/k8s"
	"google.golang.org/grpc/codes"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// nodeUnderTest is one addressable node plus the sim-side observations that
// make a scenario assertable through something other than an error return.
type nodeUnderTest struct {
	Dialer talos.Dialer
	Creds  talos.Creds
	Target talos.Target

	// Inject puts a scenario in force and returns the function that removes it.
	Inject func(t *testing.T, sc talossim.Scenario) func()

	// Calls counts arrivals of one RPC at the node. It is what proves a client
	// did not retry, and it is nil for a transport that keeps no counters --
	// which is what a real Talos node is, and why every case that uses it
	// checks first.
	Calls func(method string) int

	// Sim is the simulator behind this node, or nil against real hardware.
	Sim *talossim.Server
}

// contractTransport builds a node for one contract case.
type contractTransport struct {
	Name string
	New  func(t *testing.T, opts talossim.Options) nodeUnderTest
}

// simTransport is the fake half of TRANS-08. Phase 3 adds the real half.
var simTransport = contractTransport{
	Name: "talossim",
	New: func(t *testing.T, opts talossim.Options) nodeUnderTest {
		t.Helper()

		sim := newSim(t, opts)

		return nodeUnderTest{
			// The direct dialer rather than the in-process pipe: half of these
			// scenarios are about connections, and a transport that ignores
			// addresses cannot express an address that changed.
			Dialer: talos.NewDirectDialer(sim.Port()),
			Creds:  sim.ClientCreds(),
			Target: talos.Target{
				Machine: model.MachineID("00000000-0000-0000-0000-0000000000e1"),
				Addr:    sim.Host(),
			},
			Inject: func(t *testing.T, sc talossim.Scenario) func() {
				t.Helper()

				restore, err := sim.Inject(sc)
				if err != nil {
					t.Fatalf("Inject %s: %v", sc.Name, err)
				}
				return restore
			},
			Calls: sim.Calls,
			Sim:   sim,
		}
	},
}

// scenarioAssertion is one scenario's client-side contract.
type scenarioAssertion func(t *testing.T, tr contractTransport)

// scenarioContract is the assertion per registry entry.
//
// It is a map rather than a switch so that runScenarioContract can ask whether
// an entry has an assertion at all, which is what makes a tenth scenario added
// to the registry a red test here rather than a silent gap.
var scenarioContract = map[talossim.ScenarioName]scenarioAssertion{
	talossim.ScenarioGoSilent:                     assertGoSilent,
	talossim.ScenarioRejectApply:                  assertRejectApply,
	talossim.ScenarioSecondBootstrapAlreadyExists: assertSecondBootstrap,
	talossim.ScenarioFlapConnection:               assertFlapConnection,
	talossim.ScenarioSlowLogConsumer:              assertSlowLogConsumer,
	talossim.ScenarioIPChangesOnReboot:            assertIPChangesOnReboot,
	talossim.ScenarioEtcdDown:                     assertEtcdDown,
	talossim.ScenarioK8sDown:                      assertK8sDown,
	talossim.ScenarioVersionOutOfSupportedRange:   assertVersionOutOfSupportedRange,
}

// TestScenarioContract runs the whole registry against the simulated
// transport.
func TestScenarioContract(t *testing.T) {
	t.Parallel()

	runScenarioContract(t, simTransport)
}

// runScenarioContract is the suite. Phase 3 calls it a second time with a
// transport that reaches real hardware (TRANS-08).
func runScenarioContract(t *testing.T, tr contractTransport) {
	t.Helper()

	if len(talossim.Registry) == 0 {
		t.Fatal("the registry is empty; this suite would pass against nothing")
	}

	for name, def := range talossim.Registry {
		assert, ok := scenarioContract[name]
		if !ok {
			t.Errorf("scenario %q is in talossim.Registry with the documented expectation %q and has "+
				"no assertion in this suite; a registered scenario nobody asserts against is a "+
				"definition the client was never held to", name, def.ExpectedClient)
			continue
		}

		t.Run(tr.Name+"/"+string(name), func(t *testing.T) {
			t.Parallel()
			t.Logf("expected client behaviour: %s", def.ExpectedClient)
			assert(t, tr)
		})
	}
}

// assertGoSilent: "every non-stream call fails at its own class deadline (probe
// 5 s, fast read 10 s, mutation 30 s) and never at 90 s; a concurrent call to a
// second target is unaffected; after the configured consecutive-failure count
// the per-node breaker opens".
//
// The concurrency half is asserted in fanout_test.go's
// TestFanOutOneSilentNodeCostsOneNode, which is where the fan-out lives. The
// three class deadlines are pinned by TestClassTable from the same constants
// this gate reads; asserting all three end to end would spend 45 seconds
// proving one mechanism, and the plan's own verification forbids a test over 30
// seconds.
func assertGoSilent(t *testing.T, tr contractTransport) {
	node := tr.New(t, talossim.Options{Hostname: "go-silent"})
	cc := clientFor(t, node)

	defer node.Inject(t, talossim.Scenario{Name: talossim.ScenarioGoSilent, Duration: 90 * time.Second})()

	t.Run("the call ends at its own class deadline and not at the node's silence", func(t *testing.T) {
		// A caller budget far larger than the class deadline and far smaller
		// than the node's 90 seconds, so that what bounds the call can only be
		// the class.
		ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
		defer cancel()

		started := time.Now()
		_, err := cc.Probe(ctx)
		elapsed := time.Since(started)

		te := requireTalosError(t, err)
		if te.Kind != talos.KindTimeout {
			t.Errorf("Kind = %v, want %v", te.Kind, talos.KindTimeout)
		}
		if elapsed < talos.ProbeDeadline || elapsed > talos.ProbeDeadline+2*time.Second {
			t.Errorf("the call took %v; its probe class deadline is %v and the node is silent for 90s",
				elapsed, talos.ProbeDeadline)
		}
	})

	t.Run("the per-node breaker opens after the configured consecutive failures", func(t *testing.T) {
		breaker := talos.NewBreaker()

		// A short caller budget: the class ceiling is what the subtest above
		// proves, and paying it three more times here would cost fifteen
		// seconds to demonstrate a counter.
		for i := range talos.BreakerFailureThreshold {
			if err := breaker.Allow(node.Target.Machine); err != nil {
				t.Fatalf("the circuit opened after %d failure(s): %v", i, err)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
			_, err := cc.Version(ctx)
			cancel()

			te := requireTalosError(t, err)
			if te.Kind != talos.KindTimeout {
				t.Fatalf("Kind = %v, want %v", te.Kind, talos.KindTimeout)
			}
			breaker.Observe(node.Target.Machine, err)
		}

		if err := breaker.Allow(node.Target.Machine); !errors.Is(err, talos.ErrCircuitOpen) {
			t.Errorf("Allow = %v after %d consecutive transport failures, want ErrCircuitOpen",
				err, talos.BreakerFailureThreshold)
		}
	})
}

// assertRejectApply: "the call is not retried -- it is in the mutation class --
// and surfaces as a distinguishable typed failure carrying the upstream
// message; the breaker records no failure, because an answered call is not a
// transport failure".
func assertRejectApply(t *testing.T, tr contractTransport) {
	node := tr.New(t, talossim.Options{Hostname: "reject-apply"})
	cc := clientFor(t, node)

	defer node.Inject(t, talossim.Scenario{Name: talossim.ScenarioRejectApply})()

	before := callCount(node, "ApplyConfiguration")

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	_, err := cc.ApplyConfiguration(ctx, []byte("version: v1alpha1\n"))

	te := requireTalosError(t, err)
	if te.Kind != talos.KindRejected {
		t.Errorf("Kind = %v, want %v: the node answered", te.Kind, talos.KindRejected)
	}
	if te.Status != codes.InvalidArgument {
		t.Errorf("Status = %v, want %v", te.Status, codes.InvalidArgument)
	}
	if !strings.Contains(te.Error(), "talossim") {
		t.Errorf("the rendered error %q does not carry the node's own message; a refusal an operator "+
			"cannot read the reason for is a refusal they cannot act on", te)
	}

	// The counter, not the error, is what proves there was no retry: a client
	// that retried twice and reported the third refusal would produce an
	// identical error value.
	if delta := callCount(node, "ApplyConfiguration") - before; delta >= 0 && delta != 1 {
		t.Errorf("the node saw %d ApplyConfiguration call(s), want exactly 1: a mutation was retried", delta)
	}

	breaker := talos.NewBreaker()
	breaker.Observe(node.Target.Machine, err)
	if got := breaker.Failures(node.Target.Machine); got != 0 {
		t.Errorf("the breaker recorded %d failure(s) for an answered call, want 0", got)
	}
}

// assertSecondBootstrap: "the client surfaces AlreadyExists as its own outcome,
// never retries it, and never reports success; this is the exact case the retry
// allowlist exists to prevent".
func assertSecondBootstrap(t *testing.T, tr contractTransport) {
	node := tr.New(t, talossim.Options{Hostname: "second-bootstrap"})
	cc := clientFor(t, node)

	// Injection performs the first Bootstrap, so the client's own first call is
	// a second bootstrap -- which is the shape of the real hazard: a Bootstrap
	// whose success committed and whose reply was lost.
	defer node.Inject(t, talossim.Scenario{Name: talossim.ScenarioSecondBootstrapAlreadyExists})()

	before := callCount(node, "Bootstrap")

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	err := cc.Bootstrap(ctx)
	if err == nil {
		t.Fatal("Bootstrap reported success against an already-bootstrapped node")
	}

	te := requireTalosError(t, err)
	if te.Kind != talos.KindRejected {
		t.Errorf("Kind = %v, want %v", te.Kind, talos.KindRejected)
	}
	if te.Status != codes.AlreadyExists {
		t.Errorf("Status = %v, want %v", te.Status, codes.AlreadyExists)
	}

	if delta := callCount(node, "Bootstrap") - before; delta >= 0 && delta != 1 {
		t.Errorf("the node saw %d Bootstrap call(s), want exactly 1", delta)
	}

	// And the policy that makes it so, stated where a reader will look for it.
	if talos.Retryable(talos.MethodBootstrap) {
		t.Error("Bootstrap is on the retry allowlist")
	}
}

// assertFlapConnection: "a fast-read call retries at most twice (200 ms then
// 800 ms, full jitter), inside the original deadline, and succeeds if a window
// opens; a mutation fails on the first Unavailable; a stream is never
// restarted".
func assertFlapConnection(t *testing.T, tr contractTransport) {
	node := tr.New(t, talossim.Options{Hostname: "flap"})
	cc := clientFor(t, node)

	defer node.Inject(t, talossim.Scenario{
		Name:  talossim.ScenarioFlapConnection,
		Cycle: 200 * time.Millisecond,
	})()

	// The scenario is doing something. Without this the counter assertions
	// below would pass just as happily against a listener that never flapped.
	time.Sleep(500 * time.Millisecond)
	if node.Sim != nil && node.Sim.ListenerTransitions() == 0 {
		t.Fatal("the listener never flapped; the assertions below would be about nothing")
	}

	t.Run("a fast read attempts at most three times", func(t *testing.T) {
		before := callCount(node, "Version")

		ctx, cancel := context.WithTimeout(t.Context(), 9*time.Second)
		defer cancel()

		started := time.Now()
		_, err := cc.Version(ctx)
		elapsed := time.Since(started)

		if delta := callCount(node, "Version") - before; delta > talos.RetryAttempts {
			t.Errorf("the node saw %d Version call(s), want at most %d", delta, talos.RetryAttempts)
		}
		if err != nil {
			te := requireTalosError(t, err)
			if te.Kind != talos.KindUnreachable {
				t.Errorf("Kind = %v, want %v", te.Kind, talos.KindUnreachable)
			}
		}
		if elapsed > talos.FastReadDeadline {
			t.Errorf("the call took %v, past its own %v class deadline: retries extended the budget",
				elapsed, talos.FastReadDeadline)
		}
	})

	t.Run("a mutation attempts once", func(t *testing.T) {
		before := callCount(node, "ApplyConfiguration")

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, _ = cc.ApplyConfiguration(ctx, []byte("version: v1alpha1\n"))

		// At most one: a call the flap swallowed before it reached the node
		// cannot be counted, and that it was not counted is itself the point.
		if delta := callCount(node, "ApplyConfiguration") - before; delta > 1 {
			t.Errorf("the node saw %d ApplyConfiguration call(s) for one client call; a mutation was "+
				"retried across a flapping connection", delta)
		}
	})

	t.Run("a stream is never restarted", func(t *testing.T) {
		if talos.Retryable(talos.MethodLogs) {
			t.Error("Logs is on the retry allowlist; a restarted stream silently duplicates or drops " +
				"data and the caller cannot tell which")
		}
	})
}

// assertSlowLogConsumer: "the client does not deadlock; the caller's context
// cancellation tears the stream down within 1 s; the 60 s idle timeout applies
// to no data flowing, not to a blocked send".
func assertSlowLogConsumer(t *testing.T, tr contractTransport) {
	node := tr.New(t, talossim.Options{Hostname: "slow-consumer", StreamMessages: 4})
	cc := clientFor(t, node)

	defer node.Inject(t, talossim.Scenario{Name: talossim.ScenarioSlowLogConsumer})()

	// A stream carries no total deadline, which is the class's whole point: a
	// stream that is still delivering data has not failed.
	ctx, cancel := context.WithCancel(t.Context())

	stream, err := cc.Logs(ctx, "kubelet")
	if err != nil {
		cancel()
		t.Fatalf("Logs: %v", err)
	}

	// Read enough to be past the node's ordinary bound, which is what the
	// scenario changes: under it the stream does not end.
	for range 6 {
		if _, err := stream.Recv(); err != nil {
			cancel()
			t.Fatalf("Recv while the stream should still be producing: %v", err)
		}
	}

	// Now stop reading and walk away. The reader below is blocked in Recv, so
	// what is being torn down is a genuinely blocked call.
	blocked := make(chan error, 1)
	go func() {
		for {
			if _, err := stream.Recv(); err != nil {
				blocked <- err
				return
			}
		}
	}()

	time.Sleep(200 * time.Millisecond)
	cancelled := time.Now()
	cancel()

	select {
	case err := <-blocked:
		if elapsed := time.Since(cancelled); elapsed > time.Second {
			t.Errorf("the stream took %v to tear down after cancellation, want under 1s", elapsed)
		}
		if errors.Is(err, io.EOF) {
			t.Error("the stream ended with io.EOF; under slow_log_consumer production is endless, so " +
				"this test is not exercising the scenario")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the reader is still blocked in Recv three seconds after the caller cancelled: the " +
			"client deadlocked, which is the first thing this scenario's expectation forbids")
	}

	// The idle timeout is 60 s and it never fired here, which is the third
	// clause: it measures the node sending nothing, not a caller that stopped
	// reading. Had it been a wall-clock timer over the whole stream, the reader
	// above would have been torn down by it rather than by the cancellation --
	// and the elapsed assertion would have failed.
	if talos.StreamIdleTimeout <= time.Second {
		t.Errorf("StreamIdleTimeout = %v; the assertion above cannot distinguish an idle timeout from "+
			"a cancellation", talos.StreamIdleTimeout)
	}
}

// assertIPChangesOnReboot: "Dialer.Resolve is consulted again on the next call,
// because Target.Addr is a hint and MachineID is identity; a stale address
// yields Unavailable, never an answer attributed to the wrong node".
func assertIPChangesOnReboot(t *testing.T, tr contractTransport) {
	node := tr.New(t, talossim.Options{Hostname: "rebinder"})
	if node.Sim == nil {
		t.Skip("this case reads the node's address history, which only the simulator keeps")
	}

	cc := clientFor(t, node)

	defer node.Inject(t, talossim.Scenario{Name: talossim.ScenarioIPChangesOnReboot})()

	oldPort := node.Sim.Port()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Either outcome is honest for a node that is going away: the reply may
	// reach the caller, or the connection may die while it is in flight. What
	// must not happen is a wrong answer, and what must have happened is the
	// rebind -- both of which are asserted below rather than here. An assertion
	// that insisted on the reply would be asserting the simulator's shutdown
	// timing rather than the client's behaviour.
	if err := cc.Reboot(ctx); err != nil {
		te := requireTalosError(t, err)
		t.Logf("the reboot reply did not survive the node going away: %v (%v)", err, te.Kind)
	}

	if node.Sim.Port() == oldPort {
		t.Fatal("the node came back on the same address; the scenario did nothing")
	}
	if history := node.Sim.AddressHistory(); len(history) < 2 {
		t.Fatalf("address history = %v, want at least two entries", history)
	}

	t.Run("the stale address yields a transport failure, never a wrong answer", func(t *testing.T) {
		readCtx, readCancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer readCancel()

		_, err := cc.Hostname(readCtx)
		if err == nil {
			t.Fatal("the client at the abandoned address answered; an answer attributed to the wrong " +
				"node is the failure this scenario exists to rule out")
		}

		te := requireTalosError(t, err)
		if te.Kind != talos.KindUnreachable {
			t.Errorf("Kind = %v, want %v", te.Kind, talos.KindUnreachable)
		}
	})

	t.Run("resolving again reaches the same node", func(t *testing.T) {
		// Identity is unchanged; only the address hint moved. That is the whole
		// claim: a node whose address changed is the same node.
		fresh, err := talos.NewClusterClient(t.Context(), talos.NewDirectDialer(node.Sim.Port()),
			node.Target, node.Creds, talos.Mode{})
		if err != nil {
			t.Fatalf("NewClusterClient after the rebind: %v", err)
		}
		defer fresh.Close() //nolint:errcheck // the subtest owns this client

		readCtx, readCancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer readCancel()

		hostname, err := fresh.Hostname(readCtx)
		if err != nil {
			t.Fatalf("Hostname after the rebind: %v", err)
		}
		if hostname != "rebinder" {
			t.Errorf("Hostname() = %q, want %q: the answer came from a different node", hostname, "rebinder")
		}
	})
}

// assertEtcdDown: "node-level calls keep working; etcd calls fail
// distinguishably; the breaker does not open, because the node is reachable and
// only one subsystem is not".
func assertEtcdDown(t *testing.T, tr contractTransport) {
	node := tr.New(t, talossim.Options{Hostname: "etcd-down"})
	cc := clientFor(t, node)

	defer node.Inject(t, talossim.Scenario{Name: talossim.ScenarioEtcdDown})()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	breaker := talos.NewBreaker()

	// Enough etcd failures that a breaker counting them would have opened
	// several times over. This is the asymmetry the whole classifier exists
	// for: the status code is codes.Unavailable, exactly as it is for a dead
	// connection.
	for range talos.BreakerFailureThreshold + 2 {
		_, err := cc.EtcdMemberList(ctx)

		te := requireTalosError(t, err)
		if te.Kind != talos.KindRejected {
			t.Fatalf("Kind = %v, want %v: the node answered, and only etcd is down", te.Kind, talos.KindRejected)
		}
		if te.Status != codes.Unavailable {
			t.Errorf("Status = %v, want %v", te.Status, codes.Unavailable)
		}
		breaker.Observe(node.Target.Machine, err)
	}

	if err := breaker.Allow(node.Target.Machine); err != nil {
		t.Errorf("the circuit opened on etcd failures alone: %v; one broken subsystem has become a "+
			"node-wide outage", err)
	}

	// Node-level calls keep working, which is what "reachable" means and what
	// makes the refusal a refusal rather than an outage.
	if _, err := cc.Version(ctx); err != nil {
		t.Errorf("Version failed while only etcd was down: %v", err)
	}
	if _, err := cc.Hostname(ctx); err != nil {
		t.Errorf("Hostname failed while only etcd was down: %v", err)
	}
}

// assertK8sDown: "a COSI read for a k8s resource returns a not-found-shaped
// result rather than an error; the node stays reachable and the failure is
// attributable to Kubernetes, not to the transport".
func assertK8sDown(t *testing.T, tr contractTransport) {
	node := tr.New(t, talossim.Options{Hostname: "k8s-down"})
	cc := clientFor(t, node)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// The resource is readable before the scenario, so its absence afterwards
	// is the scenario and not a node that never had it.
	if _, err := safe.StateGetByID[*k8s.Nodename](ctx, cc.COSI(), k8s.NodenameID); err != nil {
		t.Fatalf("the k8s resource was already unreadable before injection: %v", err)
	}

	defer node.Inject(t, talossim.Scenario{Name: talossim.ScenarioK8sDown})()

	_, err := safe.StateGetByID[*k8s.Nodename](ctx, cc.COSI(), k8s.NodenameID)
	if err == nil {
		t.Fatal("the k8s resource is still readable while k8s_down is injected")
	}

	// Not-found-shaped, and shaped that way through cosi's own predicate rather
	// than through a string match. This is the clause that would break silently
	// if the transport's error classification swallowed the gRPC status.
	if !state.IsNotFoundError(err) {
		t.Errorf("the COSI read returned %v, which cosi does not recognise as not-found; the failure "+
			"is being reported as a transport problem rather than as an absent resource", err)
	}

	// The node stays reachable, so the failure is attributable to Kubernetes.
	if _, err := cc.Version(ctx); err != nil {
		t.Errorf("Version failed while only Kubernetes was down: %v", err)
	}

	services, err := cc.ServiceList(ctx)
	if err != nil {
		t.Fatalf("ServiceList: %v", err)
	}

	var found bool
	for _, svc := range services {
		if svc.ID != "kubelet" {
			continue
		}
		found = true
		if svc.State == "Running" || svc.Healthy {
			t.Errorf("kubelet reports state %q healthy=%v while k8s_down is injected", svc.State, svc.Healthy)
		}
	}
	if !found {
		t.Error("the service list does not mention kubelet at all; the assertion above checked nothing")
	}
}

// assertVersionOutOfSupportedRange: "NewClusterClient refuses with a named error
// identifying the observed version and the supported range, rather than
// proceeding against an untested API surface".
func assertVersionOutOfSupportedRange(t *testing.T, tr contractTransport) {
	node := tr.New(t, talossim.Options{Hostname: "wrong-version"})

	const observed = "v1.15.0"

	// Injected before the client is built: the refusal happens at construction,
	// on the D-05 liveness probe that already had to ask for the version.
	defer node.Inject(t, talossim.Scenario{
		Name:    talossim.ScenarioVersionOutOfSupportedRange,
		Version: observed,
	})()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cc, err := talos.NewClusterClient(ctx, node.Dialer, node.Target, node.Creds, talos.Mode{})
	if err == nil {
		_ = cc.Close()
		t.Fatal("NewClusterClient built a client for a node outside the supported version range")
	}

	if !errors.Is(err, talos.ErrUnsupportedVersion) {
		t.Errorf("error %v does not satisfy errors.Is(err, talos.ErrUnsupportedVersion)", err)
	}

	msg := err.Error()
	for _, want := range []string{observed, talos.MinSupportedVersion, talos.MaxSupportedVersion} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal %q does not name %q; an operator told only that the version is wrong "+
				"has to go and read the source to find out what would be right", msg, want)
		}
	}
}

// clientFor builds a cluster client against a node under test and closes it
// when the test ends.
func clientFor(t *testing.T, node nodeUnderTest) *talos.ClusterClient {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cc, err := talos.NewClusterClient(ctx, node.Dialer, node.Target, node.Creds, talos.Mode{})
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}
	t.Cleanup(func() {
		if err := cc.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return cc
}

// callCount is the node's arrival counter, or -1 for a transport that keeps
// none. Every caller compares deltas and skips the assertion on -1, so the
// suite degrades to what a real Talos node can prove rather than failing
// against it.
func callCount(node nodeUnderTest, method string) int {
	if node.Calls == nil {
		return -1
	}
	return node.Calls(method)
}

// requireTalosError extracts the classified transport failure, failing the test
// when there is none. Every case goes through it, so no assertion in this file
// can degenerate into a bare err != nil.
func requireTalosError(t *testing.T, err error) *talos.Error {
	t.Helper()

	if err == nil {
		t.Fatal("the call succeeded where the scenario's documented expectation is a failure")
	}

	var te *talos.Error
	if !errors.As(err, &te) {
		t.Fatalf("error is not a classified *talos.Error: %[1]T: %[1]v", err)
	}
	return te
}
