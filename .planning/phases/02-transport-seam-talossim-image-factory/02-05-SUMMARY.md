---
phase: 02-transport-seam-talossim-image-factory
plan: 05
subsystem: infra
tags: [talos, grpc, deadlines, retry, circuit-breaker, fan-out, contract-suite, rfc9457]

# Dependency graph
requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "plan 02-01's transport seam (Dialer, Target, Creds), ClusterClient over machinery's client.New, and the D-05 liveness probe"
  - phase: 02-transport-seam-talossim-image-factory
    provides: "plan 02-02's RFC 9457 upstream family and the reserved upstream.node-unreachable / upstream.node-timeout tokens"
  - phase: 02-transport-seam-talossim-image-factory
    provides: "plan 02-03's nine injectable scenarios and Definition.ExpectedClient, the normative statement this plan asserts the client against"
  - phase: 02-transport-seam-talossim-image-factory
    provides: "plan 02-08's MachineService/StorageService surface, the in-memory COSI state and the bounded stream emitter"
provides:
  - "talos.MaintenanceClient: a distinct type with a closed five-method surface, so a cluster-only call against a maintenance node is a compile error (D-06, TRANS-03)"
  - "talos.Error / talos.ErrorKind: classified transport failures carrying operation, machine, kind and upstream status, mapped onto the reserved upstream.node-* tokens"
  - "talos deadline policy: the confirmed class table keyed by gRPC full method name, ErrNoDeadline, ErrUnclassifiedMethod, WithClassDeadline (TRANS-04, D-04)"
  - "talos retry policy: a read-only allowlist derived from the fast read class, full-jitter backoff inside the original deadline (TRANS-04, T-02-28)"
  - "talos.Breaker: per-node circuit breaker with Observe, Len and a sweep driven by Fail (TRANS-05, T-02-29, T-02-30)"
  - "talos.FanOut: bounded per-node concurrency with no audit dependency (TRANS-01, TRANS-05, T-02-31)"
  - "TestScenarioContract: the client-side half of TRANS-08, iterating talossim.Registry and parameterised by transport"
affects: [02-07, inventory, health, streaming, jobs-engine, provisioning, upgrade]

actuals:
  tokens: 60988
  tasks: 4
  commits: 7

tech-stack:
  added:
    - golang.org/x/sync/errgroup (already a direct module requirement; first use)
  patterns:
    - "Policy rides on gRPC client interceptors, not on wrapper methods: a rule that has to be remembered once per method is a rule that will be forgotten in one of them."
    - "The trailer signal, not the status code, separates 'the node answered and refused' from 'nothing answered'. codes.Unavailable arrives from both."
    - "A confirmed policy is a reviewable table with a per-entry rationale, and an entry nobody wrote down is refused rather than defaulted."
    - "Allowlist over denylist for retries, for internal/audit/redact.go's reason: a denylist forgets the next mutating RPC, and what it forgets is retried."
    - "A contract suite iterates the registry rather than hand-listing it, and takes its transport as a parameter so the real-hardware half is an addition rather than a rewrite."

key-files:
  created:
    - internal/talos/errors.go
    - internal/talos/errors_test.go
    - internal/talos/maintenance_test.go
    - internal/talos/clusteronly_fixture.go
    - internal/talos/deadline.go
    - internal/talos/deadline_test.go
    - internal/talos/retry.go
    - internal/talos/retry_test.go
    - internal/talos/breaker.go
    - internal/talos/breaker_test.go
    - internal/talos/fanout.go
    - internal/talos/fanout_test.go
    - internal/talos/contract_test.go
  modified:
    - internal/talos/client.go
    - internal/talos/talos.go
    - internal/talossim/machine.go
    - internal/talossim/scenario_conn.go
    - .planning/phases/02-transport-seam-talossim-image-factory/02-CONTEXT.md
    - .planning/ROADMAP.md

key-decisions:
  - "The operator confirmed the deadline policy as option-a: the table stands verbatim, the stream/WriteTimeout collision becomes a Phase 5 entry blocker, and no HTTP streaming endpoint is built here. Nothing changed, so the ExpectedClient strings and docs/talossim.md rows were left byte-identical on purpose."
  - "A node answered iff its RPC produced response trailers. Verified empirically against talossim: etcd_down's server-side Unavailable carries one trailer, a refused connection's Unavailable carries none. The status code cannot make the distinction and reading it alone would open a reachable node's breaker."
  - "MaintenanceClient's method set is pinned whole by a reflection test, not by asserting one absent method: a small type becomes a large one one individually-defensible method at a time."
  - "The compile-time separation is asserted by running `go build -tags talos_compile_fail .` from a test and taking the non-zero exit as the evidence, with a ClusterClient control in the same fixture so a broken fixture cannot masquerade as the assertion passing."
  - "The breaker counts KindUnreachable *and* KindTimeout as transport failures. The plan's prose said unreachable only, but go_silent -- whose registry expectation is that the breaker opens -- produces a timeout. KindRejected leaves the count untouched, which is etcd_down."
  - "The probe class is selected on the context rather than by a second table entry: Version is a fast read at 10s and the D-05 liveness check at 5s, and duplicating the method under a second name would put a lie in the class table."
  - "The stream first-byte and idle timers run only while the caller is blocked in Recv, so the idle timeout means 'the node is sending nothing' and never 'the caller stopped reading'. That is exactly what slow_log_consumer's expectation distinguishes."
  - "StorageService.Disks was classified Fast read although the confirmed policy does not name it: the policy was derived from the MachineService surface, and leaving Disks unclassified would refuse the one call maintenance mode exists to support."
  - "KindRejected maps to no upstream.node-* token, deliberately. The upstream family means 'did not answer' and carries HTTP 502; a node that heard the request and declined it did answer."

patterns-established:
  - "Negative control per claim: the deadline gate, the retry allowlist, the trailer-based classification and the contract suite's registry coverage were each broken once and observed red before being restored."
  - "A guard checked against the generated descriptors rather than a hand-written list: the class table is validated against machine.MachineService_ServiceDesc and storage.StorageService_ServiceDesc, so a misspelt entry cannot look like coverage."
  - "Concurrency measured inside the per-node call rather than around the fan-out, so a serial loop cannot pass the test."

requirements-completed: [TRANS-03, TRANS-04, TRANS-05, TRANS-01]

coverage:
  - id: D1
    description: "Cluster and maintenance clients are distinct types with non-overlapping method sets: a cluster-only call against a MaintenanceClient does not compile, and the maintenance surface is exactly ApplyConfiguration, Version, Disks, COSI and Close."
    requirement: TRANS-03
    verification:
      - kind: integration
        ref: "internal/talos/maintenance_test.go#TestMaintenanceClientRejectsClusterOnlyCall (runs go build -tags talos_compile_fail and asserts the compiler names MaintenanceClient and Bootstrap)"
        status: pass
      - kind: unit
        ref: "internal/talos/maintenance_test.go#TestMaintenanceClientMethodSetIsClosed"
        status: pass
      - kind: integration
        ref: "internal/talos/maintenance_test.go#TestMaintenanceClientAnswersItsMethods"
        status: pass
    human_judgment: false
  - id: D2
    description: "Both constructors refuse the wrong credential kind and name both the kind supplied and the kind required (T-02-26)."
    requirement: TRANS-03
    verification:
      - kind: integration
        ref: "internal/talos/maintenance_test.go#TestConstructorsRefuseTheWrongCredentialKind"
        status: pass
    human_judgment: false
  - id: D3
    description: "A call issued on a context with no deadline is refused before the connection is used: the simulator's request counter records zero arrivals (D-04)."
    requirement: TRANS-04
    verification:
      - kind: integration
        ref: "internal/talos/deadline_test.go#TestRequireDeadline"
        status: pass
      - kind: manual_procedural
        ref: "negative control: requireDeadline stubbed to return nil, test observed red, then restored"
        status: pass
    human_judgment: false
  - id: D4
    description: "The confirmed deadline policy is a reviewable table whose class memberships are pinned, whose entries are all real RPCs, and which refuses an unclassified method rather than defaulting it (T-02-33)."
    requirement: TRANS-04
    verification:
      - kind: unit
        ref: "internal/talos/deadline_test.go#TestClassTable"
        status: pass
      - kind: unit
        ref: "internal/talos/deadline_test.go#TestClassTableNamesOnlyRealRPCs"
        status: pass
      - kind: unit
        ref: "internal/talos/deadline_test.go#TestWithClassDeadlineRefusesAnUnclassifiedMethod"
        status: pass
      - kind: unit
        ref: "internal/talos/deadline_test.go#TestWithClassDeadlineAppliesTheClassBudget"
        status: pass
    human_judgment: false
  - id: D5
    description: "Only the fast-read class retries, at most twice, on a transport failure only, with full jitter, entirely inside the original call deadline (TRANS-04, T-02-28)."
    requirement: TRANS-04
    verification:
      - kind: unit
        ref: "internal/talos/retry_test.go#TestRetryAllowlistIsExactlyTheFastReadClass"
        status: pass
      - kind: unit
        ref: "internal/talos/retry_test.go#TestRetryLoopAttemptCounts (mutation 1, stream 1, excluded read 1, refusal 1, allowlisted read 3)"
        status: pass
      - kind: unit
        ref: "internal/talos/retry_test.go#TestRetryBackoffIsFullJitterInsideItsBounds"
        status: pass
      - kind: integration
        ref: "internal/talos/deadline_test.go#TestRetryStopsAtTheDeadlineRatherThanCompletingItsBudget"
        status: pass
      - kind: manual_procedural
        ref: "negative control: Retryable inverted to a denylist default, mutation and stream rows observed red, then restored"
        status: pass
    human_judgment: false
  - id: D6
    description: "A transport failure carries a classified kind, an operation and a machine id, never the node's address, and each kind maps to exactly one reserved upstream.node-* token with no reserved token left unproduced (T-02-32)."
    verification:
      - kind: unit
        ref: "internal/talos/errors_test.go#TestErrorKindMapping (reads problem.go for the reserved set)"
        status: pass
      - kind: integration
        ref: "internal/talos/errors_test.go#TestErrorNamesTheMachineAndNeverTheAddress"
        status: pass
    human_judgment: false
  - id: D7
    description: "'The node is gone' and 'the node said no' are distinguished by response-trailer presence rather than by the status code, which is identical for both."
    verification:
      - kind: integration
        ref: "internal/talos/errors_test.go#TestClassifyDistinguishesAnsweredFromUnreachable"
        status: pass
      - kind: manual_procedural
        ref: "negative control: the trailer signal hard-coded to false, test observed red, then restored"
        status: pass
    human_judgment: false
  - id: D8
    description: "The per-node breaker opens after three consecutive transport failures, half-opens for exactly one trial after the cooldown, is not advanced by an answered refusal, and does not grow without bound (T-02-29, T-02-30)."
    requirement: TRANS-05
    verification:
      - kind: unit
        ref: "internal/talos/breaker_test.go#TestBreakerOpensAfterConsecutiveTransportFailures"
        status: pass
      - kind: unit
        ref: "internal/talos/breaker_test.go#TestBreakerDistinguishesARefusalFromATransportFailure"
        status: pass
      - kind: unit
        ref: "internal/talos/breaker_test.go#TestBreakerHalfOpensAfterTheCooldown"
        status: pass
      - kind: unit
        ref: "internal/talos/breaker_test.go#TestBreakerDoesNotGrowWithoutBound (500 machines, Len 1 after a sweep)"
        status: pass
    human_judgment: false
  - id: D9
    description: "A fan-out over one silent target and one healthy target returns the healthy result inside the healthy call's own deadline; cancelling the fan-out terminates every in-flight call; an open circuit is skipped without dialing (TRANS-01, TRANS-05)."
    requirement: TRANS-05
    verification:
      - kind: integration
        ref: "internal/talos/fanout_test.go#TestFanOutOneSilentNodeCostsOneNode (healthy answered after 316µs; silent gave up after 3.00s)"
        status: pass
      - kind: integration
        ref: "internal/talos/fanout_test.go#TestFanOutCancellationTerminatesEveryInFlightCall"
        status: pass
      - kind: integration
        ref: "internal/talos/fanout_test.go#TestFanOutSkipsAnOpenCircuitWithoutDialing"
        status: pass
    human_judgment: false
  - id: D10
    description: "The fan-out takes no audit and no HTTP dependency, asserted structurally rather than by review (T-02-31, flagged assumption 5)."
    verification:
      - kind: integration
        ref: "internal/talos/fanout_test.go#TestTransportTakesNoAuditOrHTTPDependency (go list -deps)"
        status: pass
    human_judgment: false
  - id: D11
    description: "Every one of the nine talossim scenarios has a running assertion against its documented ExpectedClient behaviour, and a tenth registry entry without an assertion fails the suite."
    requirement: TRANS-01
    verification:
      - kind: integration
        ref: "internal/talos/contract_test.go#TestScenarioContract (nine subtests, all pass)"
        status: pass
      - kind: manual_procedural
        ref: "negative control: a tenth registry entry added temporarily, suite observed red naming it, then removed"
        status: pass
    human_judgment: false
  - id: D12
    description: "The contract suite is parameterised by transport, so TRANS-08's real-Talos half is an added transport rather than nine rewritten assertions."
    verification: []
    human_judgment: true
    rationale: "The parameterisation compiles and the sim transport runs through it, but nothing here can prove it is sufficient for real hardware -- the sim-side counters (Server.Calls, ListenerTransitions, AddressHistory) have no hardware equivalent, and the cases that use them degrade to a skip or to a weaker assertion. Whether that residue is enough for TRANS-08 is only answerable in Phase 3, against a real node."
  - id: D13
    description: "The stream class carries no total deadline and is bounded by a first-byte deadline and an idle timeout that measure the node sending nothing rather than the caller not reading."
    requirement: TRANS-04
    verification:
      - kind: integration
        ref: "internal/talos/contract_test.go#TestScenarioContract/talossim/slow_log_consumer"
        status: pass
    human_judgment: true
    rationale: "The teardown-within-1s and no-deadlock clauses are asserted directly. That the 60s idle timeout never fires for a *blocked send* is asserted only negatively -- the stream was not torn down by it during the test -- because waiting 60s to watch a timer not fire would breach the plan's 30-second-per-test bound. A reviewer should confirm the reading that a caller who has stopped calling Recv is backpressure rather than a fault."

# Metrics
duration: 45 min
completed: 2026-08-29
status: complete
---

# Phase 2 Plan 05: Deadlines, Retries, Breaker, Fan-out and the Contract Suite Summary

**Every node call now carries an enforced class deadline, only allowlisted reads retry and only inside the original budget, a per-node breaker and per-node fan-out keep one dead node costing one dead node, and all nine `talossim` scenarios have a running assertion against the behaviour plan 02-03 wrote down for them.**

## Performance

- **Duration:** 45 min (across two agents; the first halted at the task-1 checkpoint having written nothing)
- **Started:** 2026-08-29T07:10:00Z
- **Completed:** 2026-08-29T07:55:00Z
- **Tasks:** 4 (one resolved checkpoint, three TDD)
- **Files modified:** 19 (13 created, 6 modified)

## Accomplishments

- **TRANS-01 is proven rather than inherited.** Plan 02-01 recorded it as *abstaining* because it built no fan-out and so nothing there could serialise. `TestFanOutOneSilentNodeCostsOneNode` measures inside each per-node call: the healthy node answered after **316 µs** while the silent one was still burning its own 3-second budget. A serial loop cannot pass that assertion.
- **The nine scenarios have a client-side contract, and it runs.** `TestScenarioContract` iterates `talossim.Registry`, logs each entry's `ExpectedClient` string, and asserts the production client against it. A tenth registry entry without an assertion fails the suite — verified by adding one.
- **The classifier's hard case is solved by evidence, not by a guess.** `etcd_down` returns `codes.Unavailable` from a node that is demonstrably reachable, and so does a refused connection. Probing `talossim` showed the discriminator: a server that answered always sends response trailers (`content-type: application/grpc`), a call that never reached one sends none. The unary interceptor asks for trailers on every attempt and classifies on their presence. Without this the breaker would open a healthy node's circuit whenever its etcd went down.
- **D-04 is structural, not advisory.** The deadline gate is a gRPC client interceptor on the one shared connection both client types are built on, so no method can opt out; a call on a deadline-less context is refused before the wire and `talossim`'s counter records zero arrivals. An RPC absent from the class table is refused by name rather than given a default.
- **D-06 is enforced by the compiler.** `MaintenanceClient` exposes exactly `ApplyConfiguration`, `Version`, `Disks`, `COSI` and `Close`. `TestMaintenanceClientRejectsClusterOnlyCall` runs `go build -tags talos_compile_fail .` against a fixture that calls `Bootstrap` on both client types and takes the compiler's one-sided refusal as the evidence — with the `ClusterClient` line in the same fixture as the control.
- **The retry allowlist is derived, not transcribed.** `TestRetryAllowlistIsExactlyTheFastReadClass` checks the allowlist against the class table, so a method moved into the fast read class without a decision about retrying it turns red. `EtcdSnapshot`, `PacketCapture` and `DiskUsage` are listed as `false` with their reason on the entry rather than being quietly absent.
- **The `WriteTimeout` collision is filed where Phase 5 will read it.** `ROADMAP.md` § *Phase 5* now carries an `ENTRY BLOCKER` naming both concrete fixes, and `02-CONTEXT.md`'s `<deadline_policy>` is marked confirmed so no later plan re-opens it.

## Task Commits

1. **Task 1: the resolved deadline-policy checkpoint** — `4159b7a` (docs)
2. **Task 2 RED: the two client types and classified errors** — `9c057a6` (test)
3. **Task 2 GREEN** — `ea67d63` (feat)
4. **Task 3 RED: the deadline gate and the retry allowlist** — `aa191ac` (test)
5. **Task 3 GREEN** — `a7434e3` (feat)
6. **Task 4 RED: breaker, fan-out and the contract suite** — `f380ece` (test)
7. **Task 4 GREEN** — `1726862` (feat)

## Files Created/Modified

- `internal/talos/deadline.go` — the confirmed class table keyed by gRPC full method name, the four durations with their rationale, `ErrNoDeadline`, `ErrUnclassifiedMethod`, `WithClassDeadline`, and the probe-class context marker.
- `internal/talos/retry.go` — the read-only allowlist, `Retryable`, the full-jitter loop-written backoff, and `conn.do`'s in-budget attempt loop.
- `internal/talos/breaker.go` — `Breaker` with `Allow`/`Success`/`Fail`/`Observe`/`Len`/`Failures` and a sweep driven by `Fail`.
- `internal/talos/fanout.go` — `FanOut`, `Result[T]`, `FanOutConcurrency`, and the written reason there is no audit dependency.
- `internal/talos/errors.go` — `ErrorKind`, `Error`, `classify`, `ErrorKindOf`, `ProblemCode` and the two reserved node tokens.
- `internal/talos/client.go` — the shared `conn`, the unary and stream policy interceptors, both constructors, the `ClusterClient` method surface, `Probe`, `LogStream`, and the `MaintenanceClient` surface.
- `internal/talos/clusteronly_fixture.go` — the build-tag-excluded file that must not compile, plus its control.
- `internal/talos/talos.go` — `ErrNoDeadline` moved out to `deadline.go`, next to the table it refuses against.
- `internal/talossim/scenario_conn.go`, `internal/talossim/machine.go` — two comments corrected (see deviations).
- `.planning/phases/.../02-CONTEXT.md`, `.planning/ROADMAP.md` — the confirmation and the Phase 5 entry blocker.
- Test files: `errors_test.go`, `maintenance_test.go`, `deadline_test.go`, `retry_test.go` (internal), `breaker_test.go` (internal), `fanout_test.go`, `contract_test.go`.

## Decisions Made

See `key-decisions` in the frontmatter. The three a later reader is most likely to want the reasoning for:

**The trailer signal.** The plan's action text says "a dial or connection failure and a transport-level `Unavailable` become `KindUnreachable`" but gives no way to tell a transport-level `Unavailable` from an RPC-level one. They are indistinguishable by status code, and `etcd_down` is exactly the case where getting it wrong is expensive. Before writing the classifier a throwaway probe was run against `talossim` comparing four signals — status code, `ClientConn.GetState()`, `peer.Peer`, and response trailers — across a server-returned `Unavailable`, a server-returned `InvalidArgument`, a connection closed under a live client, and a fresh dial to a dead address. Trailers were the only signal that separated them cleanly and deterministically, and they are available on every unary call because the interceptor appends `grpc.Trailer` to the call options itself.

**The breaker counts timeouts.** The plan's prose says "Only a `KindUnreachable` failure calls `Fail`". Taken literally that contradicts `go_silent`'s registry entry, whose documented client behaviour is *"after the configured consecutive-failure count the per-node breaker opens"* — and `go_silent` produces a deadline expiry, not an unreachable. The plan's own `must_haves.truths` says "consecutive **transport** failures", which covers both. So `Observe` advances on `KindUnreachable` and `KindTimeout` and leaves `KindRejected` alone, which satisfies `go_silent` and `etcd_down` simultaneously. A node that accepts connections and never answers is precisely what a breaker is for: each such call costs a full class deadline of a goroutine and a connection.

**Why `KindRejected` maps to no contract token.** The plan asks for a mapping onto the reserved `upstream.node-*` tokens that is "total and injective". There are three kinds and two reserved node tokens, so a total injective mapping is arithmetically impossible. The resolution keeps the two properties that carry the meaning: every reserved token has exactly one producing kind (checked by reading `problem.go` for `upstream.node-*` literals, so a third token minted later fails the test), and no two kinds share a token. `KindRejected` deliberately produces none — the `upstream` family means "an upstream dependency did not answer" and carries HTTP 502, and a node that heard the request and declined it did answer. Phase 6 decides what a refusal becomes at the HTTP edge, with the route in front of it.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] The `upstream.node-*` mapping could not be both total and injective as written**

- **Found during:** Task 2 (`TestErrorKindMapping`)
- **Issue:** Three `ErrorKind` values, two reserved `upstream.node-*` tokens. "Total and injective" over kinds is impossible; the criterion as literally written cannot be satisfied by any implementation.
- **Fix:** The mapping is total and injective over the *reserved token set* — every declared node token has exactly one producing kind, no token has two — and `KindRejected` maps to none, with the reason written on `ProblemCode`. The test reads `internal/httpapi/problem.go` for the literal token set, so a third node token minted later without a kind fails here.
- **Files modified:** `internal/talos/errors.go`, `internal/talos/errors_test.go`
- **Verification:** `TestErrorKindMapping` passes and fails when a fourth kind is added without a row.
- **Committed in:** `ea67d63`

**2. [Rule 2 - Missing critical] The breaker counts timeouts as transport failures**

- **Found during:** Task 4
- **Issue:** The plan's action text restricts `Fail` to `KindUnreachable`. `go_silent` produces `KindTimeout`, and its registry entry — which this plan is required to assert against — says the breaker opens. Implementing the prose literally would have made the contract suite's own `go_silent` case unsatisfiable, or satisfiable only by not asserting the clause.
- **Fix:** `Breaker.Observe` advances on `KindUnreachable` and `KindTimeout`; `KindRejected` leaves the count untouched. The plan's `must_haves.truths` wording ("consecutive transport failures") is the one implemented.
- **Files modified:** `internal/talos/breaker.go`, `internal/talos/breaker_test.go`, `internal/talos/contract_test.go`
- **Verification:** `TestBreakerDistinguishesARefusalFromATransportFailure` covers all three kinds plus an unclassified error; the `go_silent` and `etcd_down` contract cases assert the opposite outcomes end to end.
- **Committed in:** `1726862`

**3. [Rule 3 - Blocking] `StorageService.Disks` had no entry in the confirmed policy**

- **Found during:** Task 3
- **Issue:** The confirmed deadline policy was derived from the 54-method `MachineServiceServer` surface plus COSI Get/List. `Disks` lives on `StorageService` and is in `MaintenanceClient`'s method set (D-06), so leaving it unclassified would have refused the one call maintenance mode exists to support.
- **Fix:** Classified `ClassFastRead` and retryable, with the omission and the reasoning written on the table entry.
- **Files modified:** `internal/talos/deadline.go`, `internal/talos/retry.go`, `internal/talos/deadline_test.go`
- **Verification:** `TestMaintenanceClientAnswersItsMethods` drives `Disks` end to end; `TestClassTable` accounts for the entry.
- **Committed in:** `a7434e3`

**4. [Rule 1 - Bug] `FanOut` needs the breaker as a parameter**

- **Found during:** Task 4
- **Issue:** The plan specifies `FanOut[T any](ctx, targets, call)` *and* "each consulting the breaker before dialing". The signature as written cannot do the second thing, and Go has no generic methods, so a `(*Breaker)` method was not available either.
- **Fix:** `FanOut(ctx, breaker, targets, call)`, with `breaker` allowed to be nil. Everything else about the signature is unchanged.
- **Files modified:** `internal/talos/fanout.go`, `internal/talos/fanout_test.go`
- **Verification:** `TestFanOutSkipsAnOpenCircuitWithoutDialing` asserts the call never runs and the node's counter does not move.
- **Committed in:** `1726862`

**5. [Rule 1 - Bug] Two `talossim` comments contradicted their own implementation**

- **Found during:** Task 4 (`ip_changes_on_reboot` contract case)
- **Issue:** `rebind()` and `Reboot()` both state that established connections survive the reboot so the `Reboot` reply is delivered over the connection that asked for it. `rebind` calls `closeListener`, which severs them (`trackingListener.closeConns`). Plan 02-03's SUMMARY records the non-severing behaviour as a key decision. The contract case failed with `Reboot: the node is unreachable`, which is what the code does and not what three documents say.
- **Fix:** The comments were corrected, not the behaviour. Severing is the better model: a client whose connection survived the reboot would never observe the address change, which would make the scenario inert against exactly the caller it exists to test. The cost — the `Reboot` reply may not reach the caller — makes the simulator *harder* to satisfy than hardware, which is the permitted direction under `docs/talossim.md` rule 2. The contract assertion accepts either outcome for the `Reboot` call itself and insists on the rebind having happened and on the stale address yielding a transport failure.
- **Files modified:** `internal/talossim/scenario_conn.go`, `internal/talossim/machine.go`, `internal/talos/contract_test.go`
- **Verification:** the `ip_changes_on_reboot` contract case passes; `internal/talossim`'s own suite is unchanged and green.
- **Committed in:** `1726862`; recorded as entry 7 in `.planning/WINDOWS.md`.

**6. [Rule 3 - Blocking] `.planning/WINDOWS.md` carried an invalid status and stale counts**

- **Found during:** SUMMARY close-out
- **Issue:** Entry 6 had `status: "resolved"`, which is not in the ledger's vocabulary, and the frontmatter counts had not been updated. `gsd-tools windows append` refused to write.
- **Fix:** Entry 6's status changed to `fixed` (the correct token for what happened — the verification was run and passed, reason text preserved) and the counts reconciled.
- **Files modified:** `.planning/WINDOWS.md`
- **Verification:** `gsd-tools windows append` now succeeds; entry 7 was written.
- **Committed in:** the plan metadata commit.

### Scope notes (not deviations)

- **Task 2 landed the whole `ClusterClient` method surface** (`Hostname`, `ServiceList`, `EtcdMemberList`, `ApplyConfiguration`, `Bootstrap`, `Reboot`, `COSI`) rather than only the maintenance type. The compile-time separation assertion needs cluster-only methods to *exist* to be meaningful, and tasks 3 and 4 install policy on that path rather than growing it — dribbling the methods across three commits would have been churn without information.
- **`retry_test.go` and `breaker_test.go` are internal (`package talos`)** while every other test file is external. Both assert properties of unexported functions and of an injected clock, and both would otherwise require exporting a knob a caller could use to disable the policy. `internal/auth/ratelimit_test.go` sets the precedent.
- **`Reboot` uses `client.Reboot` rather than `client.RebootWithResponse`.** `talossim`'s `TestMethodCoverage` resolves a call site by the method's name against the generated descriptors, and `RebootWithResponse` resolves to no RPC — the guard logged it as "nothing for the simulator to implement" and stopped covering the one `Reboot` call the product makes.

---

**Total deviations:** 6 auto-fixed (3 blocking, 1 missing critical, 2 bugs)
**Impact on plan:** No scope creep. Three were contradictions inside the plan or the confirmed policy that had to be resolved for the plan's own claims to be assertable; two were bugs (one in another plan's comments, found by this plan's contract case); one was a broken planning artifact. Every resolution is written down where the next reader will hit the same question.

## Negative controls run

The repository's discipline is that a guard which has never fired is a guard that may not. Four were broken deliberately, observed red, and restored:

| Guard | How it was broken | Observed |
|---|---|---|
| `requireDeadline` | stubbed to `return nil` | `TestRequireDeadline`: *"Version succeeded on a context with no deadline"* |
| retry allowlist | `Retryable` defaulted to `true` for unknown methods (a denylist) | `TestRetryLoopAttemptCounts`: *"the loop made 3 attempt(s), want 1"* for the mutation and stream rows |
| trailer classification | `answered` hard-coded to `false` | `TestClassifyDistinguishesAnsweredFromUnreachable`: *"Kind = unreachable, want rejected"* |
| contract-suite registry coverage | a tenth entry `tenth_negative_control` added to `talossim.Registry` | `TestScenarioContract`: *"scenario \"tenth_negative_control\" is in talossim.Registry ... and has no assertion in this suite"* |

## Prohibitions from the plan

All three are **resolved**:

- *"Retry must never be the default."* `retryable` is an allowlist keyed by full method name, derived from and checked against the class table by `TestRetryAllowlistIsExactlyTheFastReadClass`. `Retryable` answers `false` for anything it has not been told about, so an RPC nobody classified is an RPC nobody retries. `TestRetryLoopAttemptCounts` pins one attempt for a mutation, a stream, an excluded read and an answered refusal.
- *"A per-node fan-out must never turn one unreachable node into a whole-inventory outage."* `TestFanOutOneSilentNodeCostsOneNode` measures inside each call: 316 µs for the healthy node against 3 s for the silent one. `TestTransportTakesNoAuditOrHTTPDependency` asserts structurally that `internal/talos` cannot reach the fail-closed audit writer at all.
- *"A transport failure must never reach the operator as an anonymous internal error."* Every failure on either client type is a `*talos.Error` carrying an operation, a machine id, a kind and the upstream status, and `ErrorKind.ProblemCode` binds the two transport kinds to reserved contract tokens that `TestErrorKindMapping` holds equal to `internal/httpapi`'s constants.

## Known Stubs

| File | What | Why, and what closes it |
|---|---|---|
| `internal/talos/client.go` (`NewMaintenanceClient`) | `Creds.Fingerprint` is accepted and not used: an `InsecureSkipVerify` maintenance connection is trust-on-first-use rather than pinned. | T-02-27 in this plan's own threat register assigns the pinning to a later phase and says the field "exists on the seam so a later phase can pin the maintenance certificate without a signature change". Implementing it here would go past the register's own disposition. The hole is real: a maintenance connection today verifies nothing. |
| `internal/talos/client.go` (`ClusterClient`) | Only nine of the 54 `MachineService` RPCs are wrapped. | Deliberate: the class table covers the whole confirmed surface, but a wrapper is written when a caller needs it, and `talossim`'s `TestMethodCoverage` requires the simulator to implement every wrapped method. Phases 3, 6 and 9 add wrappers with the screens that drive them. |

## Threat Flags

None. The surface this plan adds is entirely inside the boundaries the plan's own `<threat_model>` enumerated: no new network endpoint, no new HTTP route, no new trust boundary, no new module requirement (`golang.org/x/sync` was already a direct requirement). `T-02-26`, `T-02-27` (partially — see Known Stubs), `T-02-28`, `T-02-29`, `T-02-30`, `T-02-31`, `T-02-32` and `T-02-33` all have executed mitigations with named tests; `T-02-SC` has nothing to check because `go.mod` and `go.sum` are unchanged.

## Issues Encountered

- **`go-task` is not installed on this host**, so the plan-level verification lines `task test` and `task lint:go` could not be run by name. They were executed as their literal commands: `go test ./... -count=1 -race` (14 packages green) and `golangci-lint run` from `~/go/bin` at the version CI pins (`v2.13.1`, 0 issues).
- **Full jitter makes elapsed time a distribution, and gRPC's own reconnect backoff means a failed attempt does not necessarily redial.** Neither timing nor the simulator's arrival counter can therefore prove an exact retry count end to end. The counts are asserted against `conn.do` directly in `retry_test.go`, where they are exact, and the end-to-end tests assert the bounds that do hold.
- **The `go_silent` class-deadline assertion costs 5 seconds** and is deliberately run against the *probe* class rather than three times against probe, fast read and mutation. Asserting all three end to end would spend 45 seconds proving one mechanism and would breach the plan's own 30-seconds-per-test bound; the other two budgets are pinned by `TestClassTable` from the same constants the gate reads.

## User Setup Required

None — no external service configuration required. This plan adds no module requirement; `go.mod` and `go.sum` are unchanged.

## Next Phase Readiness

- **ROADMAP success criterion 3 is met in full.** An unreachable node is bounded by its own goroutine, its own deadline and its own breaker; cluster and maintenance clients are separate types the compiler keeps apart.
- **ROADMAP success criterion 2 has a running proof.** All nine scenarios are asserted against the `ExpectedClient` strings plan 02-03 wrote down, and the suite fails on a scenario nobody asserted.
- **TRANS-08 is set up for Phase 3.** `runScenarioContract` takes a `contractTransport`; the real half is a second value, not nine rewritten assertions. Coverage item D12 records the caveat: the sim-side counters have no hardware equivalent and those cases will degrade.
- **Phase 5 entry blocker, recorded in `ROADMAP.md` § *Phase 5* and in this plan's decision:** the HTTP chain cannot stream. None of the three `ResponseWriter` wrappers implements `Flush`, `Unwrap` or `Hijack`, so a streaming handler on that chain buffers silently, and `WriteTimeout = 60 s` is process-wide with reasoning scoped to argon2id and the login rate limiter. Both must land before Phase 5's first SSE route. Phase 2 exposes no streaming HTTP route, so nothing here is papered over.
- **Phase 6 owes the HTTP adapter.** `upstream.node-unreachable` and `upstream.node-timeout` are reserved, produced by `ErrorKind.ProblemCode`, and held honest by `TestErrorKindMapping` — but nothing maps a `*talos.Error` onto an RFC 9457 problem yet, and nothing decides what a `KindRejected` becomes. That arrives with the first route that drives a node call.
- **Open concern:** a maintenance-mode connection verifies nothing (see Known Stubs). The seam is ready for pinning; the pinning is not written.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-29*

## Self-Check: PASSED

- **Files claimed created, verified present on disk:** `internal/talos/errors.go`, `errors_test.go`, `maintenance_test.go`, `clusteronly_fixture.go`, `deadline.go`, `deadline_test.go`, `retry.go`, `retry_test.go`, `breaker.go`, `breaker_test.go`, `fanout.go`, `fanout_test.go`, `contract_test.go` — all FOUND.
- **Commits claimed, verified in `git log`:** `4159b7a`, `9c057a6`, `ea67d63`, `aa191ac`, `a7434e3`, `f380ece`, `1726862` — all FOUND.
- **Plan-level verification re-run at close:** `go test ./... -count=1 -race` green across all 14 packages; the slowest single test is `TestGoSilentFailsAtItsOwnClassDeadline` at 5.03 s, well inside the 30 s bound. `golangci-lint run` (v2.13.1, the version CI pins) reports `0 issues.` `go list -deps ./internal/talos` contains no `internal/httpapi` and no `internal/audit`. `TestScenarioContract` covers all nine registry entries and was observed failing against a temporary tenth.
- **Acceptance criteria:** every criterion of tasks 2, 3 and 4 was executed rather than asserted, including the `go doc ./internal/talos MaintenanceClient` surface check, the build-tag compile failure, and `talossim`'s `TestMethodCoverage` against the ten new machinery call sites.
