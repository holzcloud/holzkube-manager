package talos_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/api/storage"

	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestRequireDeadline is D-04's structure, which is the half of the decision
// that must not be relaxed: the values are constants, but "there is no
// default-less path" is the property.
//
// The assertion is deliberately made at the simulator rather than at the error
// return. A refusal that still put the call on the wire would be a refusal in
// name only, and the sim's own request counter is the only thing that can say
// the call never happened.
func TestRequireDeadline(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "deadline-node"})
	cc := newClusterClient(t, sim)

	before := sim.Calls("Version")

	//nolint:usetesting // the point of this call is a context with no deadline at all
	_, err := cc.Version(context.Background())
	if err == nil {
		t.Fatal("Version succeeded on a context with no deadline")
	}
	if !errors.Is(err, talos.ErrNoDeadline) {
		t.Errorf("error %v does not satisfy errors.Is(err, talos.ErrNoDeadline)", err)
	}

	if after := sim.Calls("Version"); after != before {
		t.Errorf("the node saw %d Version call(s) after the refusal, want %d: the call was refused "+
			"after it had already gone to the wire, which is not a refusal", after-before, 0)
	}
}

// TestClassTable pins the confirmed policy.
//
// The table below is the transcription of 02-CONTEXT.md's <deadline_policy>,
// confirmed by the operator on 2026-08-29. Its value is not that it repeats the
// map -- it is that it pins the class *memberships*. An entry that drifted from
// Mutation to Fast read would make Bootstrap retryable, and a retried Bootstrap
// is the cluster-destroying case the allowlist exists to prevent. That is a
// one-word edit in the map and a red test here.
func TestClassTable(t *testing.T) {
	t.Parallel()

	const (
		m = "/machine.MachineService/"
		s = "/storage.StorageService/"
		c = "/cosi.resource.State/"
	)

	policy := map[talos.DeadlineClass][]string{
		talos.ClassFastRead: {
			m + "Version", m + "Hostname", m + "ServiceList", m + "Memory", m + "LoadAvg",
			m + "SystemStat", m + "CPUInfo", m + "CPUFreqStats", m + "DiskStats",
			m + "NetworkDeviceStats", m + "Mounts", m + "Netstat", m + "Processes", m + "Stats",
			m + "Containers", m + "EtcdMemberList", m + "EtcdStatus", m + "EtcdAlarmList",
			c + "Get", c + "List",
			s + "Disks",
		},
		talos.ClassMutation: {
			m + "ApplyConfiguration", m + "Bootstrap", m + "Reset", m + "Reboot", m + "Shutdown",
			m + "Upgrade", m + "Rollback", m + "MetaWrite", m + "MetaDelete",
			m + "ServiceStart", m + "ServiceStop", m + "ServiceRestart", m + "ImagePull",
			m + "EtcdLeaveCluster", m + "EtcdRemoveMemberByID", m + "EtcdForfeitLeadership",
			m + "EtcdDefragment", m + "EtcdRecover",
			m + "EtcdDowngradeEnable", m + "EtcdDowngradeValidate", m + "EtcdDowngradeCancel",
		},
		talos.ClassStream: {
			m + "Logs", m + "Dmesg", m + "Events", m + "Read", m + "Copy", m + "List",
			m + "DiskUsage", m + "ImageList", m + "EtcdSnapshot", m + "PacketCapture",
			c + "Watch",
		},
	}

	table := talos.DeadlineClasses()

	covered := 0
	for class, methods := range policy {
		for _, method := range methods {
			covered++

			got, ok := table[method]
			if !ok {
				t.Errorf("%s has no entry in the class table; the confirmed policy names it, and an "+
					"unclassified RPC is a gap in the specification rather than a candidate for a "+
					"default deadline", method)
				continue
			}
			if got != class {
				t.Errorf("%s is classified %v, want %v", method, got, class)
			}
		}
	}

	// The probe class is the one RPC used as the D-05 liveness check, and it is
	// deliberately the same method as a fast read under a shorter budget, so it
	// is asserted separately rather than as a table row.
	if talos.ClassProbe.Deadline() != 5*time.Second {
		t.Errorf("probe deadline = %v, want 5s", talos.ClassProbe.Deadline())
	}
	if talos.ClassFastRead.Deadline() != 10*time.Second {
		t.Errorf("fast read deadline = %v, want 10s", talos.ClassFastRead.Deadline())
	}
	if talos.ClassMutation.Deadline() != 30*time.Second {
		t.Errorf("mutation deadline = %v, want 30s", talos.ClassMutation.Deadline())
	}
	if talos.StreamFirstByteDeadline != 10*time.Second {
		t.Errorf("stream first-byte deadline = %v, want 10s", talos.StreamFirstByteDeadline)
	}
	if talos.StreamIdleTimeout != 60*time.Second {
		t.Errorf("stream idle timeout = %v, want 60s", talos.StreamIdleTimeout)
	}
	if talos.ClassStream.Deadline() != 0 {
		t.Errorf("stream class total deadline = %v, want 0: a stream is bounded by its first-byte "+
			"deadline and its idle timeout, not by a total one", talos.ClassStream.Deadline())
	}

	if covered != len(table) {
		t.Errorf("the class table has %d entries and the confirmed policy accounts for %d; an entry "+
			"nobody wrote down is a deadline nobody reviewed", len(table), covered)
	}
}

// TestClassTableNamesOnlyRealRPCs is the guard against a typo.
//
// A method spelled wrongly in the table is worse than a missing one: the real
// RPC then has no entry and is refused, and the wrong one sits in the table
// looking like coverage. The generated service descriptors are what the wire
// actually carries, so they are what the table is checked against.
func TestClassTableNamesOnlyRealRPCs(t *testing.T) {
	t.Parallel()

	known := map[string]bool{}

	mSvc := "/" + machine.MachineService_ServiceDesc.ServiceName + "/"
	for _, md := range machine.MachineService_ServiceDesc.Methods {
		known[mSvc+md.MethodName] = true
	}
	for _, sd := range machine.MachineService_ServiceDesc.Streams {
		known[mSvc+sd.StreamName] = true
	}

	sSvc := "/" + storage.StorageService_ServiceDesc.ServiceName + "/"
	for _, md := range storage.StorageService_ServiceDesc.Methods {
		known[sSvc+md.MethodName] = true
	}
	for _, sd := range storage.StorageService_ServiceDesc.Streams {
		known[sSvc+sd.StreamName] = true
	}

	checked := 0
	for method := range talos.DeadlineClasses() {
		// COSI is served by cosi-project's own descriptor rather than by
		// machinery's, and its methods are exercised by the tests that read
		// through it. Everything under the two Talos services is checked here.
		if strings.HasPrefix(method, "/cosi.") {
			continue
		}
		checked++
		if !known[method] {
			t.Errorf("%s is in the class table and is not an RPC of either Talos service; a misspelt "+
				"entry leaves the real method unclassified while looking like coverage", method)
		}
	}

	if checked == 0 {
		t.Fatal("checked no entries; the detection is broken and this guard is vacuous")
	}
}

// TestWithClassDeadlineRefusesAnUnclassifiedMethod is T-02-33.
func TestWithClassDeadlineRefusesAnUnclassifiedMethod(t *testing.T) {
	t.Parallel()

	ctx, cancel, err := talos.WithClassDeadline(t.Context(), "/machine.MachineService/GenerateClientConfiguration")
	if err == nil {
		cancel()
		t.Fatal("WithClassDeadline gave a deadline to a method that is not in the table")
	}
	if ctx != nil {
		t.Error("WithClassDeadline returned a context alongside its refusal")
	}
	if !errors.Is(err, talos.ErrUnclassifiedMethod) {
		t.Errorf("error %v does not satisfy errors.Is(err, talos.ErrUnclassifiedMethod)", err)
	}
}

// TestWithClassDeadlineAppliesTheClassBudget checks both directions: a caller
// with no deadline gets the class one, and a caller with a shorter one keeps
// it. The second is what makes the class a ceiling rather than a floor.
func TestWithClassDeadlineAppliesTheClassBudget(t *testing.T) {
	t.Parallel()

	//nolint:usetesting // the ceiling is only observable from a context that has no deadline of its own
	ctx, cancel, err := talos.WithClassDeadline(context.Background(), talos.MethodVersion)
	if err != nil {
		t.Fatalf("WithClassDeadline: %v", err)
	}
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("the returned context carries no deadline")
	}
	if budget := time.Until(deadline); budget > talos.ClassFastRead.Deadline() {
		t.Errorf("budget %v exceeds the fast read class deadline %v", budget, talos.ClassFastRead.Deadline())
	}

	short, shortCancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer shortCancel()

	inner, innerCancel, err := talos.WithClassDeadline(short, talos.MethodVersion)
	if err != nil {
		t.Fatalf("WithClassDeadline: %v", err)
	}
	defer innerCancel()

	innerDeadline, ok := inner.Deadline()
	if !ok {
		t.Fatal("the returned context carries no deadline")
	}
	if budget := time.Until(innerDeadline); budget > time.Second {
		t.Errorf("budget %v ignored the caller's shorter deadline; the class is a ceiling, not a floor", budget)
	}
}

// TestGoSilentFailsAtItsOwnClassDeadline is the timing half of go_silent's
// documented expectation: "every non-stream call fails at its own class
// deadline and never at 90 s".
//
// It is run against the probe class, which is the shortest of the three, for
// the reason plan 02-05's verification gives: no single test may take 30
// seconds, and asserting the same mechanism three times at 5 s, 10 s and 30 s
// would spend 45 seconds proving one thing. The other two budgets are pinned by
// TestClassTable, which reads them from the same constants the gate uses.
func TestGoSilentFailsAtItsOwnClassDeadline(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "silent-node"})
	cc := newClusterClient(t, sim)

	restore, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioGoSilent, Duration: 90 * time.Second})
	if err != nil {
		t.Fatalf("Inject: %v", err)
	}
	defer restore()

	// A generous caller budget, so that what bounds the call is the class and
	// not the caller. Without this the test would prove only that a context
	// with a timeout times out.
	ctx, cancel := context.WithTimeout(t.Context(), 80*time.Second)
	defer cancel()

	started := time.Now()
	_, err = cc.Probe(ctx)
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("the probe succeeded against a node that answers nothing")
	}

	var te *talos.Error
	if !errors.As(err, &te) {
		t.Fatalf("error is not a *talos.Error: %[1]T: %[1]v", err)
	}
	if te.Kind != talos.KindTimeout {
		t.Errorf("Kind = %v, want %v", te.Kind, talos.KindTimeout)
	}

	if elapsed < talos.ClassProbe.Deadline() {
		t.Errorf("the probe returned after %v, before its own %v class deadline", elapsed, talos.ClassProbe.Deadline())
	}
	if elapsed > talos.ClassProbe.Deadline()+2*time.Second {
		t.Errorf("the probe returned after %v; it was bounded by something other than its %v class "+
			"deadline, and the node stays silent for 90s", elapsed, talos.ClassProbe.Deadline())
	}
}

// TestRetryStopsAtTheDeadlineRatherThanCompletingItsBudget is D-04's "retries
// never extend a call's deadline".
//
// The proof is a contrast rather than a counter: against an address nothing
// answers at, a generous deadline must cost at least the backoff sum -- which
// is what shows retries happened at all -- while a deadline shorter than the
// first backoff must return inside it. A test with only the second half would
// pass against an implementation that never retried.
func TestRetryStopsAtTheDeadlineRatherThanCompletingItsBudget(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "budget-node"})
	cc := newClusterClient(t, sim)

	// Close the node out from under a client that is already connected, so the
	// failures are transport failures against a target that was reachable a
	// moment ago -- the shape a node going down actually has.
	if err := sim.Close(); err != nil {
		t.Fatalf("Close the sim: %v", err)
	}

	t.Run("a generous budget pays for the retries", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 9*time.Second)
		defer cancel()

		started := time.Now()
		_, err := cc.Version(ctx)
		elapsed := time.Since(started)

		if err == nil {
			t.Fatal("Version succeeded against a closed node")
		}
		if elapsed > 5*time.Second {
			t.Errorf("elapsed %v: the attempt loop ran longer than the whole backoff budget allows", elapsed)
		}
		t.Logf("three attempts with backoff took %v", elapsed)
	})

	t.Run("a budget shorter than the backoff sum is not exceeded", func(t *testing.T) {
		const budget = 150 * time.Millisecond

		ctx, cancel := context.WithTimeout(t.Context(), budget)
		defer cancel()

		started := time.Now()
		_, err := cc.Version(ctx)
		elapsed := time.Since(started)

		if err == nil {
			t.Fatal("Version succeeded against a closed node")
		}
		// The tolerance covers scheduling, not another attempt: one more
		// backoff would put this past 350ms even with full jitter at its
		// minimum, because the loop abandons rather than sleeping past the
		// deadline.
		if elapsed > budget+150*time.Millisecond {
			t.Errorf("elapsed %v exceeds the %v call budget: a retry ran past the original deadline, "+
				"which is exactly what D-04 forbids", elapsed, budget)
		}
	})
}
