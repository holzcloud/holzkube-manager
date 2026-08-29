package talos_test

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube/internal/model"
	"github.com/holzcloud/holzkube/internal/talos"
	"github.com/holzcloud/holzkube/internal/talossim"
)

// TestFanOutOneSilentNodeCostsOneNode is TRANS-05 and the explicit half of
// TRANS-01, which plan 02-01 recorded as abstaining because it had no fan-out
// to prove it against.
//
// The claim is not "the fan-out eventually finishes". A serial loop finishes
// too. The claim is that the healthy node's answer arrives inside the healthy
// node's own deadline while the silent node is still pending -- so the
// measurement is taken inside each call, not around the fan-out.
func TestFanOutOneSilentNodeCostsOneNode(t *testing.T) {
	t.Parallel()

	silent := newSim(t, talossim.Options{Hostname: "silent"})
	healthy := newSim(t, talossim.Options{Hostname: "healthy"})

	restore, err := silent.Inject(talossim.Scenario{Name: talossim.ScenarioGoSilent, Duration: 90 * time.Second})
	if err != nil {
		t.Fatalf("Inject: %v", err)
	}
	defer restore()

	silentTarget := simTarget(silent, "00000000-0000-0000-0000-0000000000f1")
	healthyTarget := simTarget(healthy, "00000000-0000-0000-0000-0000000000f2")

	// Each node has its own dialer because each listens on its own port. The
	// fan-out is over identities; the transport per identity is the Dialer's
	// business, which is the seam working as designed.
	dialers := map[model.MachineID]talos.Dialer{
		silentTarget.Machine:  talos.NewDirectDialer(silent.Port()),
		healthyTarget.Machine: talos.NewDirectDialer(healthy.Port()),
	}
	creds := map[model.MachineID]talos.Creds{
		silentTarget.Machine:  silent.ClientCreds(),
		healthyTarget.Machine: healthy.ClientCreds(),
	}

	// Clients are built before the measurement so that the construction probe
	// is not what is being timed. The silent node was injected before its
	// client was built, so it gets its client the only way it can: it does not.
	healthyClient, err := talos.NewClusterClient(t.Context(), dialers[healthyTarget.Machine],
		healthyTarget, creds[healthyTarget.Machine], talos.Mode{})
	if err != nil {
		t.Fatalf("NewClusterClient for the healthy node: %v", err)
	}
	defer healthyClient.Close() //nolint:errcheck // the test owns this client

	// A per-node budget well under the silent node's 90 seconds and well over
	// what a loopback round trip costs.
	const perNode = 3 * time.Second

	var mu sync.Mutex
	elapsed := map[model.MachineID]time.Duration{}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	started := time.Now()

	results := talos.FanOut(ctx, nil, []talos.Target{silentTarget, healthyTarget},
		func(ctx context.Context, target talos.Target) (string, error) {
			callCtx, callCancel := context.WithTimeout(ctx, perNode)
			defer callCancel()

			var (
				version string
				err     error
			)

			if target.Machine == healthyTarget.Machine {
				version, err = healthyClient.Version(callCtx)
			} else {
				// The silent node cannot even be constructed: the D-05 probe is
				// the first thing that goes silent. That is the realistic shape
				// and it is still bounded by this target's own budget.
				_, err = talos.NewClusterClient(callCtx, dialers[target.Machine], target, creds[target.Machine], talos.Mode{})
			}

			mu.Lock()
			elapsed[target.Machine] = time.Since(started)
			mu.Unlock()

			return version, err
		})

	if len(results) != 2 {
		t.Fatalf("FanOut returned %d result(s), want 2", len(results))
	}

	byMachine := map[model.MachineID]talos.Result[string]{}
	for _, r := range results {
		byMachine[r.Target.Machine] = r
	}

	healthyResult := byMachine[healthyTarget.Machine]
	if healthyResult.Err != nil {
		t.Fatalf("the healthy node failed: %v", healthyResult.Err)
	}
	if healthyResult.Value == "" {
		t.Error("the healthy node returned an empty version")
	}

	silentResult := byMachine[silentTarget.Machine]
	if silentResult.Err == nil {
		t.Fatal("the silent node answered")
	}

	mu.Lock()
	healthyElapsed, silentElapsed := elapsed[healthyTarget.Machine], elapsed[silentTarget.Machine]
	mu.Unlock()

	// The assertion. The healthy node's result exists long before the silent
	// node's budget runs out, which is only possible if the two calls ran
	// concurrently and neither queued behind a shared lock.
	if healthyElapsed >= perNode {
		t.Errorf("the healthy node's result took %v, which is its whole %v budget: it waited for the "+
			"silent node, and one unreachable node has become an inventory-wide outage",
			healthyElapsed, perNode)
	}
	if silentElapsed < perNode {
		t.Errorf("the silent node failed after %v, before its own %v budget; the test is not "+
			"exercising the case it describes", silentElapsed, perNode)
	}

	t.Logf("healthy answered after %v; silent gave up after %v", healthyElapsed, silentElapsed)
}

// TestFanOutCancellationTerminatesEveryInFlightCall.
//
// A fan-out whose caller has walked away must not keep two dozen goroutines
// each holding a connection for the rest of its class deadline.
func TestFanOutCancellationTerminatesEveryInFlightCall(t *testing.T) {
	t.Parallel()

	const nodes = 4

	targets := make([]talos.Target, 0, nodes)
	for i := range nodes {
		sim := newSim(t, talossim.Options{Hostname: "silent"})
		restore, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioGoSilent, Duration: 90 * time.Second})
		if err != nil {
			t.Fatalf("Inject: %v", err)
		}
		t.Cleanup(restore)

		targets = append(targets, simTarget(sim, fanoutMachineID(i)))
	}

	ctx, cancel := context.WithCancel(t.Context())

	var running sync.WaitGroup
	running.Add(nodes)

	done := make(chan []talos.Result[struct{}], 1)
	go func() {
		done <- talos.FanOut(ctx, nil, targets,
			func(ctx context.Context, _ talos.Target) (struct{}, error) {
				running.Done()
				<-ctx.Done()
				return struct{}{}, ctx.Err()
			})
	}()

	// Every per-node call has started before the cancellation, so what is being
	// torn down is genuinely in flight.
	running.Wait()
	cancel()

	select {
	case results := <-done:
		if len(results) != nodes {
			t.Errorf("FanOut returned %d result(s), want %d: a cancelled fan-out still owes an "+
				"outcome per target", len(results), nodes)
		}
		for _, r := range results {
			if !errors.Is(r.Err, context.Canceled) {
				t.Errorf("%s reported %v, want context.Canceled", r.Target.Machine, r.Err)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("FanOut did not return within 3s of its context being cancelled; the per-node calls " +
			"are running to their own deadlines instead of to the caller's")
	}
}

// TestFanOutSkipsAnOpenCircuitWithoutDialing.
func TestFanOutSkipsAnOpenCircuitWithoutDialing(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "skipped"})
	target := simTarget(sim, "00000000-0000-0000-0000-0000000000f9")

	breaker := talos.NewBreaker()
	for range talos.BreakerFailureThreshold {
		breaker.Fail(target.Machine)
	}

	before := sim.Calls("Version")

	dialed := false
	results := talos.FanOut(t.Context(), breaker, []talos.Target{target},
		func(context.Context, talos.Target) (string, error) {
			dialed = true
			return "", nil
		})

	if dialed {
		t.Error("the call ran against a node whose circuit is open; short-circuiting means not dialing")
	}
	if len(results) != 1 || !errors.Is(results[0].Err, talos.ErrCircuitOpen) {
		t.Errorf("results = %+v, want one ErrCircuitOpen", results)
	}
	if after := sim.Calls("Version"); after != before {
		t.Errorf("the node saw %d call(s) while its circuit was open", after-before)
	}
}

// TestTransportTakesNoAuditOrHTTPDependency is T-02-31 and flagged assumption 5,
// asserted structurally rather than by review.
//
// The audit writer is fail-closed and fsyncs every record under one global
// mutex, sized on a handful of records per minute. A fan-out that recorded per
// node would turn contention into *mutation failure*, so the dependency must
// not exist at all -- not merely be unused today. internal/httpapi is the same
// argument in the other direction: the transport sits below HTTP, and an import
// upward is the shape that later becomes a cycle.
func TestTransportTakesNoAuditOrHTTPDependency(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	out, err := exec.CommandContext(ctx, "go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}

	deps := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(deps) < 10 {
		t.Fatalf("go list reported %d dependencies; the detection is broken and this guard is vacuous", len(deps))
	}

	for _, forbidden := range []string{
		"github.com/holzcloud/holzkube/internal/audit",
		"github.com/holzcloud/holzkube/internal/httpapi",
	} {
		for _, dep := range deps {
			if dep == forbidden {
				t.Errorf("internal/talos depends on %s", forbidden)
			}
		}
	}
}

// fanoutMachineID is a distinct identity per simulated node. Identity is what a
// fan-out is keyed on, so two nodes sharing one would make the whole test
// vacuous rather than merely wrong.
func fanoutMachineID(i int) string {
	return "00000000-0000-0000-0000-00000000000" + string(rune('a'+i))
}
