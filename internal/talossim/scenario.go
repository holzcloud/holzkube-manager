package talossim

// Fault injection for the simulated node (TRANS-07).
//
// The direction of this file is the whole point, and it is the direction
// internal/audit/redact.go argues for its allowlist: the table is the
// specification, and anything absent from it is a missing specification rather
// than a permitted default. Inject refuses a ScenarioName that Registry does
// not know, so a scenario cannot be exercised before somebody has written down
// what the client is supposed to do about it. A registry that answered
// "unknown scenario? do nothing" would let a test suite grow green assertions
// against a simulator that was not simulating anything.
//
// The second column is what makes this more than a list of faults. ROADMAP
// success criterion 2 is not "the sim can fail" but "der Client verhält sich in
// jedem Fall definiert statt zufällig", and a defined behaviour that is written
// nowhere cannot be asserted. Definition.ExpectedClient is that statement, and
// plan 02-05 asserts the production client against exactly these strings.
// docs/talossim.md carries the same two columns for a human reader, and
// TestDocumentedScenariosMatchRegistry keeps the two from drifting apart.
//
// A caveat that shapes every assertion built on this file: client.EventsWatch
// drops undecodable events and returns nil -- not an error -- on io.EOF,
// codes.Canceled and codes.DeadlineExceeded. go_silent and flap_connection
// therefore look like clean success to a caller that only checks err. Every
// scenario here is consequently observable through something else: a timing
// bound, a sim-side counter, or the connection state.

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"google.golang.org/grpc"
)

// ScenarioName identifies a failure scenario.
//
// The string values are TRANS-07's own tokens, verbatim. Deriving them from
// the requirement rather than paraphrasing them is what makes a rename in the
// requirement and a rename in the code impossible to diverge silently: the
// registry test carries the nine literals and fails on either.
type ScenarioName string

// The nine scenarios TRANS-07 names.
const (
	ScenarioGoSilent                     ScenarioName = "go_silent"
	ScenarioRejectApply                  ScenarioName = "reject_apply"
	ScenarioSecondBootstrapAlreadyExists ScenarioName = "second_bootstrap_returns_AlreadyExists"
	ScenarioFlapConnection               ScenarioName = "flap_connection"
	ScenarioSlowLogConsumer              ScenarioName = "slow_log_consumer"
	ScenarioIPChangesOnReboot            ScenarioName = "ip_changes_on_reboot"
	ScenarioEtcdDown                     ScenarioName = "etcd_down"
	ScenarioK8sDown                      ScenarioName = "k8s_down"
	ScenarioVersionOutOfSupportedRange   ScenarioName = "version_out_of_supported_range"
)

// Scenario is one injectable fault and its parameters.
//
// It is a single struct carrying every scenario's parameters rather than nine
// option types, so that a caller writes
//
//	Inject(Scenario{Name: ScenarioGoSilent, Duration: 90 * time.Second})
//
// and the zero value of a field this scenario does not read is not a trap: an
// unset field is filled from Registry[Name].Defaults, and a field the scenario
// ignores is documented as ignored below.
type Scenario struct {
	// Name selects the scenario. It must be a key of Registry.
	Name ScenarioName

	// Duration is how long the fault lasts where the fault has a length:
	// go_silent only. Ignored by every other scenario.
	Duration time.Duration

	// Cycle is the period of a repeating fault: flap_connection only. The
	// listener is closed for one Cycle and open for the next. Ignored by every
	// other scenario.
	Cycle time.Duration

	// Version is the Talos version the node reports:
	// version_out_of_supported_range only. Ignored by every other scenario.
	Version string
}

// Definition is a scenario's entry in the registry: what the simulator does,
// what the client is then required to do, and the parameters used when the
// caller supplies none.
type Definition struct {
	// Sim is what the simulated node does while the scenario is injected.
	Sim string

	// ExpectedClient is the defined behaviour the production client must show
	// in response. It is normative: plan 02-05 asserts against it, and an
	// empty string here would mean a scenario whose whole purpose -- proving
	// the client is defined rather than random -- has not been stated.
	ExpectedClient string

	// Defaults fills the parameters a caller leaves at their zero value.
	Defaults Scenario
}

// Registry is the scenario catalogue.
//
// It is the code half of the table in docs/talossim.md, and the two are
// asserted equal by TestDocumentedScenariosMatchRegistry so the document
// cannot rot into a lie. Its default direction is refusal: Inject rejects a
// name that is not a key here, because a scenario nobody has specified is not
// a scenario that quietly does nothing.
var Registry = map[ScenarioName]Definition{
	ScenarioGoSilent: {
		Sim: "accepts the TCP connection and completes the TLS handshake, then writes no response byte " +
			"for the configured duration",
		ExpectedClient: "every non-stream call fails at its own class deadline (probe 5 s, fast read 10 s, " +
			"mutation 30 s) and never at 90 s; a concurrent call to a second target is unaffected; " +
			"after the configured consecutive-failure count the per-node breaker opens",
		Defaults: Scenario{Name: ScenarioGoSilent, Duration: 90 * time.Second},
	},
	ScenarioRejectApply: {
		Sim: "ApplyConfiguration returns codes.InvalidArgument with a message",
		ExpectedClient: "the call is not retried -- it is in the mutation class -- and surfaces as a " +
			"distinguishable typed failure carrying the upstream message; the breaker records no " +
			"failure, because an answered call is not a transport failure",
		Defaults: Scenario{Name: ScenarioRejectApply},
	},
	ScenarioSecondBootstrapAlreadyExists: {
		Sim: "the first Bootstrap succeeds; every later one returns codes.AlreadyExists. Injection performs " +
			"that first Bootstrap, so the node is already bootstrapped when the client arrives and the " +
			"client's own first call is a second bootstrap",
		ExpectedClient: "the client surfaces AlreadyExists as its own outcome, never retries it, and never " +
			"reports success; this is the exact case the retry allowlist exists to prevent",
		Defaults: Scenario{Name: ScenarioSecondBootstrapAlreadyExists},
	},
	ScenarioFlapConnection: {
		Sim: "closes and reopens the listener on a configurable cycle",
		ExpectedClient: "a fast-read call retries at most twice (200 ms then 800 ms, full jitter), inside the " +
			"original deadline, and succeeds if a window opens; a mutation fails on the first " +
			"Unavailable; a stream is never restarted",
		Defaults: Scenario{Name: ScenarioFlapConnection, Cycle: 200 * time.Millisecond},
	},
	ScenarioSlowLogConsumer: {
		Sim: "Logs produces faster than the consumer reads until the send blocks",
		ExpectedClient: "the client does not deadlock; the caller's context cancellation tears the stream down " +
			"within 1 s; the 60 s idle timeout applies to no data flowing, not to a blocked send",
		Defaults: Scenario{Name: ScenarioSlowLogConsumer},
	},
	ScenarioIPChangesOnReboot: {
		Sim: "after a simulated reboot the sim listens on a new address and the old one refuses connections",
		ExpectedClient: "Dialer.Resolve is consulted again on the next call, because Target.Addr is a hint and " +
			"MachineID is identity; a stale address yields Unavailable, never an answer attributed to " +
			"the wrong node",
		Defaults: Scenario{Name: ScenarioIPChangesOnReboot},
	},
	ScenarioEtcdDown: {
		Sim: "every Etcd* RPC returns codes.Unavailable; Version and Hostname still answer",
		ExpectedClient: "node-level calls keep working; etcd calls fail distinguishably; the breaker does not " +
			"open, because the node is reachable and only one subsystem is not",
		Defaults: Scenario{Name: ScenarioEtcdDown},
	},
	ScenarioK8sDown: {
		Sim: "the k8s-related COSI resources are absent and the k8s service reports not-running in ServiceList",
		ExpectedClient: "a COSI read for a k8s resource returns a not-found-shaped result rather than an error; " +
			"the node stays reachable and the failure is attributable to Kubernetes, not to the transport",
		Defaults: Scenario{Name: ScenarioK8sDown},
	},
	ScenarioVersionOutOfSupportedRange: {
		Sim: "Version reports a Talos version outside the supported range v1.12-v1.14",
		ExpectedClient: "NewClusterClient refuses with a named error identifying the observed version and the " +
			"supported range, rather than proceeding against an untested API surface",
		Defaults: Scenario{Name: ScenarioVersionOutOfSupportedRange, Version: "v1.15.0"},
	},
}

// withDefaults fills the parameters the caller left at their zero value.
func (d Definition) withDefaults(sc Scenario) Scenario {
	if sc.Duration == 0 {
		sc.Duration = d.Defaults.Duration
	}
	if sc.Cycle == 0 {
		sc.Cycle = d.Defaults.Cycle
	}
	if sc.Version == "" {
		sc.Version = d.Defaults.Version
	}
	return sc
}

// Inject makes the named fault active on a node that is already running, and
// returns the function that removes it again.
//
// Injection is a mutation of a running server: no restart, no re-listen of the
// whole node, and no client reconnect. That is deliberate and it is the
// property TRANS-07 is really asking for -- a test that has to rebuild the
// simulator and redial to see a fault has proven nothing about a fault that
// appears in the middle of a session, which is the only way faults appear on
// hardware.
//
// The returned clear restores the state the server was in before, so scenarios
// compose inside one test binary instead of leaking into the next test. It is
// idempotent: calling it twice is the normal shape of a deferred cleanup plus
// an explicit one on a failure path, and that is not a bug.
//
// No background goroutine owns a scenario's lifetime. A scenario ends when the
// caller clears it or when the server closes, for the reason
// internal/auth.Limiter gives for sweeping from Fail rather than from a
// ticker: a lifetime goroutine needs a shutdown path and somewhere to be
// stopped in every test that builds a server. flap_connection runs a goroutine
// because closing and reopening a listener on a cycle is its mechanism, and
// that goroutine is owned by the injection -- clear and Close both stop it.
func (s *Server) Inject(sc Scenario) (func(), error) {
	def, ok := Registry[sc.Name]
	if !ok {
		return nil, fmt.Errorf(
			"talossim: unknown scenario %q: it is not in Registry, and an unregistered scenario is a "+
				"missing specification rather than a fault that does nothing", sc.Name)
	}

	sc = def.withDefaults(sc)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, fmt.Errorf("talossim: cannot inject %q into a closed node", sc.Name)
	}
	previous, had := s.scenarios[sc.Name]
	s.scenarios[sc.Name] = sc
	s.mu.Unlock()

	restore := func() {
		s.mu.Lock()
		if had {
			s.scenarios[sc.Name] = previous
		} else {
			delete(s.scenarios, sc.Name)
		}
		s.mu.Unlock()
	}

	undo, err := s.startScenario(sc)
	if err != nil {
		restore()
		return nil, err
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			undo()
			restore()
		})
	}, nil
}

// Active reports the scenarios currently injected, ordered by name so that two
// readings of an unchanged server compare equal.
//
// It exists so a test can assert the simulator's own state rather than infer
// it from an RPC that may have failed for an unrelated reason.
func (s *Server) Active() []Scenario {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Scenario, 0, len(s.scenarios))
	for _, sc := range s.scenarios {
		out = append(out, sc)
	}
	slices.SortFunc(out, func(a, b Scenario) int {
		switch {
		case a.Name < b.Name:
			return -1
		case a.Name > b.Name:
			return 1
		default:
			return 0
		}
	})
	return out
}

// activeScenario is the single accessor every service method and every
// interceptor consults before answering.
//
// One function rather than nine scattered conditionals: a scenario that has to
// be checked in nine places is a scenario that will be forgotten in one of
// them, and a forgotten check is exactly the "registered but inert" failure
// TestEveryScenarioIsImplemented exists to catch.
func (s *Server) activeScenario(name ScenarioName) (Scenario, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sc, ok := s.scenarios[name]
	return sc, ok
}

// startScenario performs the per-scenario setup and returns its undo.
//
// The dispatch is exhaustive over Registry by construction: a name that
// reaches here is a registry key, and a key with no case would return a no-op
// undo -- which is why TestEveryScenarioIsImplemented compares injected
// behaviour against an un-injected baseline instead of merely asserting that
// Inject returned no error.
func (s *Server) startScenario(sc Scenario) (func(), error) {
	switch sc.Name {
	case ScenarioGoSilent:
		return s.startGoSilent(sc)
	default:
		return func() {}, nil
	}
}

// recordCall counts one RPC by its short method name.
//
// The counter is what proves a client did not retry: reject_apply is only
// convincing if the simulator can say that ApplyConfiguration arrived once.
// It counts refusals too -- a call the scenario rejected still arrived.
func (s *Server) recordCall(fullMethod string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls[shortMethod(fullMethod)]++
}

// Calls reports how many times the named RPC has been invoked on this node,
// counting calls a scenario refused.
//
// The name is the method's short name -- "ApplyConfiguration", not
// "/machine.MachineService/ApplyConfiguration" -- because a caller asserting
// on a retry knows the method it called and should not have to know the
// service's fully qualified protobuf name to ask about it.
func (s *Server) Calls(method string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.calls[method]
}

// shortMethod is the trailing element of a gRPC full method name.
func shortMethod(fullMethod string) string {
	for i := len(fullMethod) - 1; i >= 0; i-- {
		if fullMethod[i] == '/' {
			return fullMethod[i+1:]
		}
	}
	return fullMethod
}

// scenarioUnary is the interceptor every unary RPC passes through.
func (s *Server) scenarioUnary(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	s.recordCall(info.FullMethod)

	if err := s.scenarioGate(ctx, info.FullMethod); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

// scenarioStream is the interceptor every server stream passes through.
//
// Streams go through the same gate as unary calls because go_silent is a
// property of the connection rather than of a method: a node that has stopped
// answering has stopped answering Logs too, and a gate that covered only unary
// calls would let a stream succeed against a silent node.
func (s *Server) scenarioStream(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	s.recordCall(info.FullMethod)

	if err := s.scenarioGate(ss.Context(), info.FullMethod); err != nil {
		return err
	}
	return handler(srv, ss)
}

// scenarioGate applies the scenarios that answer for a method instead of
// letting the handler answer.
//
// Faults that change what a handler reports rather than whether it runs --
// k8s_down's service list, ip_changes_on_reboot's rebind -- are consulted
// inside the handler through activeScenario instead. The split is between "the
// call does not happen" and "the call happens and says something different".
func (s *Server) scenarioGate(ctx context.Context, _ string) error {
	if sc, ok := s.activeScenario(ScenarioGoSilent); ok {
		return s.goSilent(ctx, sc)
	}
	return nil
}
