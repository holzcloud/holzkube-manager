---
phase: 02-transport-seam-talossim-image-factory
plan: 03
subsystem: testing
tags: [talos, grpc, fault-injection, test-fake, cosi, streaming, semver]

# Dependency graph
requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "plan 02-01's transport seam, talossim's mTLS listeners, in-process dialer and gRPC server"
  - phase: 02-transport-seam-talossim-image-factory
    provides: "plan 02-08's MachineService/StorageService method bodies, per-server node state, in-memory COSI state and the bounded stream emitter"
  - phase: 01-foundation
    provides: "internal/audit/redact.go's reviewable-table discipline and internal/auth/ratelimit.go's per-key-state-under-one-mutex, no-lifetime-goroutine shape"
provides:
  - "talossim.Registry: the nine TRANS-07 scenarios with a written, normative expected-client behaviour per scenario"
  - "Server.Inject/Active: runtime fault injection on an already-running node, with an idempotent restore so scenarios compose"
  - "Sim-side observations that survive client.EventsWatch's nil-on-EOF: Server.Calls, Server.ListenerTransitions, Server.AddressHistory, Streamer.BlockedSends/BlockedFor"
  - "talos.MinSupportedVersion/MaxSupportedVersion and talos.CheckSupportedVersion, enforced by NewClusterClient's liveness probe"
  - "docs/talossim.md: the operator- and reviewer-facing scenario catalogue, held equal to the registry by a test"
  - "TestEveryScenarioIsImplemented: a baseline-comparing gate that fails on a registered-but-inert scenario"
affects: [02-05, 02-06, provisioning, upgrade, streaming, dashboard]

actuals:
  tokens: 34373
  tasks: 3
  commits: 4

tech-stack:
  added: []
  patterns:
    - "The registry is the specification: an unregistered scenario is a missing specification, not a fault that does nothing, so Inject refuses it (the direction internal/audit/redact.go's allowlist argues for)."
    - "Every fault is observable through something other than an error return, because client.EventsWatch turns io.EOF, Canceled and DeadlineExceeded into nil."
    - "A scenario supplies a precondition rather than a special case where it can: second_bootstrap injects the state that makes the client's next Bootstrap a second one, and the node's ordinary refusal answers it."
    - "The completeness gate compares injected behaviour against an un-injected baseline, not against 'no panic'."
    - "Documentation held by an executed test: TestDocumentedScenariosMatchRegistry parses the markdown table and asserts row-set equality with the code."

key-files:
  created:
    - internal/talossim/scenario.go
    - internal/talossim/scenario_conn.go
    - internal/talossim/scenario_rpc.go
    - internal/talossim/scenario_test.go
    - internal/talos/version.go
    - internal/talos/version_test.go
    - docs/talossim.md
  modified:
    - internal/talossim/talossim.go
    - internal/talossim/machine.go
    - internal/talossim/stream.go
    - internal/talossim/cosi.go
    - internal/talos/client.go

key-decisions:
  - "second_bootstrap_returns_AlreadyExists injects the precondition, not the refusal: injection performs the first Bootstrap, so the client's own first call is the second one. Implementing it as a handler special case would have been byte-identical to the node's uninjected behaviour and the completeness gate would have caught it as inert."
  - "The scenario gate is a chained gRPC interceptor keyed on the method name, not nine conditionals in nine handlers. Only the two faults that change what a handler says rather than whether it runs -- k8s_down's ServiceList, ip_changes_on_reboot's rebind -- are consulted inside a handler."
  - "flap_connection severs established connections through a tracking listener. net.Listener.Close only stops new accepts, so a client that had already dialled would never observe the flap -- the scenario would be inert against exactly the caller it exists to test."
  - "ip_changes_on_reboot deliberately does NOT sever established connections: closing the listener leaves the Reboot reply to be delivered over the connection that asked for it, which is the sequence a client observes on hardware."
  - "Server.Close closes a done channel and drains scenario goroutines before grpc.Server.Stop. Stop blocks until pending RPCs return, so a go_silent-parked handler without a shutdown signal would deadlock Close and hang the whole test binary (T-02-15)."
  - "The supported-version range is a constant pair in internal/talos with a locally implemented major.minor comparison, not a golang.org/x/mod/semver import -- the same argument plan 02-04 made for its semver comparison. go.mod and go.sum are unchanged by this plan."
  - "Prereleases inside the window are accepted (v1.14.0-rc.2 passes): opting into a release candidate is a separate decision from being inside the supported window, and conflating them would refuse a node the operator deliberately chose."

patterns-established:
  - "Negative control per claim: every scenario implementation was deliberately disabled once and the corresponding test observed red before being restored. This is what caught the first slow_log_consumer test, which was green with the implementation switched off."
  - "Coarse observation in the gate, precise assertion in the scenario test: the gate asks only whether anything changed; what changed is the individual test's business."

requirements-completed: [TRANS-07, TRANS-06]

coverage:
  - id: D1
    description: "Each of the nine named scenarios is injectable at runtime on an already-running sim, without restarting it and without reconnecting the client."
    requirement: TRANS-07
    verification:
      - kind: integration
        ref: "internal/talossim/scenario_test.go#TestEveryScenarioIsImplemented"
        status: pass
      - kind: integration
        ref: "internal/talossim/scenario_test.go#TestInjectAndClearRestorePreviousBehaviour"
        status: pass
    human_judgment: false
  - id: D2
    description: "Every scenario is observable at the wire through something other than a bare error return, because Client.EventsWatch returns nil on io.EOF, Canceled and DeadlineExceeded."
    requirement: TRANS-07
    verification:
      - kind: integration
        ref: "internal/talossim/scenario_test.go#TestScenarioGoSilent (timing bound: returns at its own 400ms deadline, strictly before the node's 8s silence)"
        status: pass
      - kind: integration
        ref: "internal/talossim/scenario_test.go#TestScenarioFlapConnection (Server.ListenerTransitions)"
        status: pass
      - kind: integration
        ref: "internal/talossim/scenario_test.go#TestScenarioSlowLogConsumer (Streamer.BlockedSends plus production past the configured bound)"
        status: pass
      - kind: integration
        ref: "internal/talossim/scenario_test.go#TestScenarioIPChangesOnReboot (Server.AddressHistory plus a refused dial to the abandoned address)"
        status: pass
      - kind: integration
        ref: "internal/talossim/scenario_test.go#TestScenarioRejectApply (Server.Calls(\"ApplyConfiguration\") == 1)"
        status: pass
    human_judgment: false
  - id: D3
    description: "The scenario registry carries a defined expected client behaviour for every scenario, so 'der Client verhält sich definiert statt zufällig' has a written referent."
    requirement: TRANS-07
    verification:
      - kind: unit
        ref: "internal/talossim/scenario_test.go#TestRegistryDefinesTheExpectedClientBehaviour"
        status: pass
    human_judgment: false
  - id: D4
    description: "A scenario named in TRANS-07 but absent from the registry fails a test, and the documented table cannot drift from the registry."
    requirement: TRANS-07
    verification:
      - kind: unit
        ref: "internal/talossim/scenario_test.go#TestRegistryCoversTRANS07"
        status: pass
      - kind: unit
        ref: "internal/talossim/scenario_test.go#TestDocumentedScenariosMatchRegistry"
        status: pass
      - kind: unit
        ref: "internal/talossim/scenario_test.go#TestDocumentationRecordsTheEventsWatchHazard"
        status: pass
    human_judgment: false
  - id: D5
    description: "Injecting and then clearing a scenario returns the sim to its prior behaviour, so scenarios compose in a test suite rather than leaking into the next test."
    requirement: TRANS-07
    verification:
      - kind: integration
        ref: "internal/talossim/scenario_test.go#TestInjectAndClearRestorePreviousBehaviour"
        status: pass
      - kind: integration
        ref: "internal/talossim/scenario_test.go#TestScenarioK8sDown (the removed COSI resource is readable again after restore)"
        status: pass
      - kind: unit
        ref: "internal/talossim/scenario_test.go#TestInjectRefusesAnUnknownScenario"
        status: pass
    human_judgment: false
  - id: D6
    description: "A registered scenario that does nothing fails a running test rather than silently making plan 02-05's contract suite pass against nothing."
    requirement: TRANS-07
    verification:
      - kind: integration
        ref: "internal/talossim/scenario_test.go#TestEveryScenarioIsImplemented"
        status: pass
      - kind: manual_procedural
        ref: "negative control: etcd_down's gate condition disabled, gate observed red, then restored"
        status: pass
    human_judgment: false
  - id: D7
    description: "NewClusterClient refuses a node reporting a Talos version outside v1.12-v1.14, naming both the observed version and the supported range."
    requirement: TRANS-06
    verification:
      - kind: integration
        ref: "internal/talossim/scenario_test.go#TestScenarioVersionOutOfRange"
        status: pass
      - kind: unit
        ref: "internal/talos/version_test.go#TestCheckSupportedVersion"
        status: pass
    human_judgment: false
  - id: D8
    description: "talossim reproduces the documented failure shape rather than a convenient approximation, so it is never easier to satisfy than a real node."
    verification: []
    human_judgment: true
    rationale: "Fidelity to hardware is not assertable from inside the simulator. The status codes, the asymmetry of etcd_down, and the refusal of a repeated Bootstrap are derived from PITFALLS.md and the machinery surface, and each was written against a documented shape -- but only a run against a real Talos node can settle whether the shape is right. Phase 4's QEMU walking skeleton is the first place that becomes checkable."
  - id: D9
    description: "ip_changes_on_reboot rebinds to a new port on 127.0.0.1 rather than to a different loopback IP."
    verification:
      - kind: integration
        ref: "internal/talossim/scenario_test.go#TestScenarioIPChangesOnReboot"
        status: pass
    human_judgment: true
    rationale: "The property under test -- Target.Addr is a hint and MachineID is identity -- is preserved, and the abandoned address refuses rather than times out. But macOS assigns only 127.0.0.1 to lo0 by default, so binding 127.0.0.2 fails on the dev host; the address change is therefore a port change. A reviewer should confirm this is close enough to a DHCP lease change for what plan 02-05 asserts."

# Metrics
duration: 38 min
completed: 2026-08-29
status: complete
---

# Phase 2 Plan 03: `talossim` Fault Injection Summary

**Nine named failure scenarios injectable at runtime on a running simulated Talos node, each carrying a written, normative expected-client behaviour, each provable by a sim-side counter, a timing bound or the connection state rather than by an error return, and each held against an un-injected baseline by a gate that fails on a scenario which does nothing.**

## Performance

- **Duration:** 38 min
- **Started:** 2026-08-29T05:31:00Z
- **Completed:** 2026-08-29T06:09:14Z
- **Tasks:** 3
- **Files modified:** 12 (7 created, 5 modified)

## Accomplishments

- **The expectation table this plan owed exists in three places that a test keeps equal.** `02-CONTEXT.md` recorded that the client's defined behaviour per scenario was specified nowhere; `talossim.Registry` now carries it as `Definition.ExpectedClient`, `docs/talossim.md` carries the same nine rows for a human reader, and `TestDocumentedScenariosMatchRegistry` parses the markdown and asserts row-set equality. Plan 02-05 has something normative to code against.
- **Faults are injected into a node that is already running.** `Server.Inject` mutates a live server under its existing mutex and returns an idempotent restore. No restart, no re-listen, no client reconnect — a test that had to redial to see a fault would prove nothing about a fault appearing mid-session, which is the only way faults appear on hardware.
- **`Inject` refuses a scenario the registry does not know.** The default direction is `internal/audit/redact.go`'s: an unregistered name is a missing specification, not a no-op, so a test suite cannot accumulate green assertions against a fault nobody wrote down.
- **Every assertion in this plan is built on something other than `err != nil`.** `client.EventsWatch` returns nil on `io.EOF`, `codes.Canceled` and `codes.DeadlineExceeded`, so a silent node and a flapping listener read as clean success. The simulator therefore exposes `Server.Calls`, `Server.ListenerTransitions`, `Server.AddressHistory`, `Streamer.BlockedSends` and `Streamer.BlockedFor`, and the tests read those and the clock.
- **`NewClusterClient` now refuses an unsupported Talos version.** The range check rides on the D-05 liveness probe, which already had to ask for the version. `talos.MinSupportedVersion`/`MaxSupportedVersion` are the single place the window is written down, referenced by the check and by its test.
- **A registered-but-inert scenario fails a test.** `TestEveryScenarioIsImplemented` injects each of the nine against a fresh node, exercises what its `Sim` column names, and compares against the un-injected baseline.

## Task Commits

1. **Task 1: The scenario mechanism, the registry, and the documented table** — `fce1947` (feat)
2. **Task 2: The connection- and timing-shaped scenarios** — `036c359` (feat)
3. **Task 3: The RPC-semantics scenarios and the completeness gate** — `f9ffd7c` (feat)

## Files Created/Modified

- `internal/talossim/scenario.go` — `ScenarioName`, `Scenario`, `Definition`, `Registry`, `Inject`, `Active`, `activeScenario`, the chained unary and stream interceptors, and the per-method call counters.
- `internal/talossim/scenario_conn.go` — `go_silent`, `flap_connection`, `slow_log_consumer`, `ip_changes_on_reboot`'s rebind and `version_out_of_supported_range`, plus the tracking listener that lets a flap sever established connections.
- `internal/talossim/scenario_rpc.go` — `reject_apply`, `second_bootstrap_returns_AlreadyExists`, `etcd_down`, `k8s_down`, and the method-keyed RPC gate.
- `internal/talossim/scenario_test.go` — twelve named tests plus the nine-way completeness gate.
- `internal/talos/version.go` — `MinSupportedVersion`, `MaxSupportedVersion`, `ErrUnsupportedVersion`, `CheckSupportedVersion`.
- `internal/talos/version_test.go` — the window and its edges, with the bounds read from the constants.
- `docs/talossim.md` — the catalogue, the two rules the fake lives by, the `EventsWatch` hazard, and the parameter table.
- `internal/talossim/talossim.go` — the `done` channel, the scenario-goroutine `WaitGroup`, listener bookkeeping (`Addr`, `AddressHistory`, `ListenerTransitions`), and the interceptor chain.
- `internal/talossim/machine.go` — `setBootstrapped`, the `ip_changes_on_reboot` hook in `Reboot`, and the `k8s_down` branch in `ServiceList`.
- `internal/talossim/stream.go` — unbounded emission for `slow_log_consumer` and blocked-send accounting recorded at the moment the send blocks.
- `internal/talossim/cosi.go` — `seedKubernetes`, which is also `k8s_down`'s restore path.
- `internal/talos/client.go` — the supported-range check on the liveness probe.

## Decisions Made

See `key-decisions` in the frontmatter. The two that a later reader is most likely to want the reasoning for:

**`second_bootstrap_returns_AlreadyExists` injects the precondition, not the refusal.** Plan 02-08 already made a repeated `Bootstrap` return `codes.AlreadyExists` — that is the node's ordinary behaviour, and rightly so. A scenario implemented as a handler special case would therefore have been byte-identical to the baseline, and `TestEveryScenarioIsImplemented` would have reported it inert. Injection instead marks the node bootstrapped, so the client's own first `Bootstrap` is a second bootstrap. That is also the case the retry allowlist actually exists for: a `Bootstrap` whose success committed and whose reply was lost, which a client that retried would see refused. The registry's `Sim` column and `docs/talossim.md` both say this in full.

**The scenario gate is an interceptor.** One function consulted by every unary call and every server stream, keyed on the method name, rather than nine conditionals spread across nine handlers. A fault that has to be remembered in nine places is a fault that will be forgotten in one — which is precisely the "registered but inert" failure the gate exists to catch. Only two scenarios are consulted inside a handler, and both change *what a handler says* rather than *whether it runs*.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] The first `TestScenarioSlowLogConsumer` was inert and passed against a disabled implementation**

- **Found during:** Task 2, negative-control pass
- **Issue:** The test asserted `Streamer.BlockedSends() > 0` after the consumer stopped reading. That is true with or without the scenario: the emitter is unbuffered by design (02-08's T-02-08-01 mitigation), so a *bounded* stream against a stalled consumer blocks too. Running the test with `setUnbounded(true)` commented out produced a green suite — exactly the failure `TestEveryScenarioIsImplemented` exists to prevent, in the test that was supposed to be proving the scenario.
- **Fix:** The test now also reads past the node's configured `StreamMessages` bound. Under the baseline the stream ends at message four with `io.EOF`; under the scenario it does not end at all. The blocked-send assertion stays, because the criterion asks for it, but it is no longer the load-bearing one.
- **Files modified:** `internal/talossim/scenario_test.go`
- **Verification:** Re-ran the same negative control — the test now fails with "the stream ended after 4 messages (EOF); slow_log_consumer did not make production endless, so it is indistinguishable from an ordinary bounded stream" — then restored and confirmed green.
- **Committed in:** `036c359`

**2. [Rule 3 - Blocking] `k8s_down` had no Kubernetes resource to remove**

- **Found during:** Task 3
- **Issue:** Plan 02-08 seeds `hardware.SystemInformation` and `network.NodeAddress`. Neither is Kubernetes-related, so `k8s_down`'s "the k8s-related COSI resources are absent" had nothing to make absent, and the scenario would have been inert.
- **Fix:** `seedKubernetes` in `internal/talossim/cosi.go` seeds `k8s.Nodename` in the `k8s` namespace at construction. It is factored out rather than inlined because it is also `k8s_down`'s restore path — a scenario that removed a resource nothing could put back would be a one-way door, leaving every later test on that node looking at a cluster with no Kubernetes.
- **Files modified:** `internal/talossim/cosi.go`
- **Verification:** `TestScenarioK8sDown` asserts the resource is readable before injection, not-found-shaped during, and readable again after restore.
- **Committed in:** `f9ffd7c`
- **Note:** `cosi.go` is not in the plan's `files_modified`. The plan's `read_first` for task 3 names it, and the scenario is unimplementable without the seed.

**3. [Rule 2 - Missing critical] `Server.Close` would have deadlocked behind a `go_silent`-parked handler**

- **Found during:** Task 1
- **Issue:** `grpc.Server.Stop` blocks until every pending RPC returns. A handler parked by `go_silent` for its default 90 seconds would therefore hang `Close`, and one injected scenario would hang the whole test binary. This is threat T-02-15; the plan named the mitigation ("the blocking handler also selects on the server's shutdown channel") but the `Server` had no shutdown channel.
- **Fix:** Added `Server.done`, closed by `Close` *before* `srv.Stop()`, and `Server.sg` — a second `WaitGroup` for scenario-owned goroutines, drained between the two so a flapper cannot re-listen behind `Close`'s back (T-02-16). `Close` also closes the current listener at the end, so a listener opened after the gRPC server stopped serving it does not hold a port for the rest of the test binary.
- **Files modified:** `internal/talossim/talossim.go`, `internal/talossim/scenario_conn.go`
- **Verification:** `TestFlappingNodeClosesCleanly` closes a node mid-flap without clearing the scenario first — the case that matters, because a test that fails mid-flap runs only its deferred `Close` — and asserts the address then refuses.
- **Committed in:** `fce1947` and `036c359`

**4. [Sequencing] `go_silent` landed in task 1 rather than task 2**

- **Found during:** Task 1
- **Issue:** Task 1's acceptance criteria require a test that "injects a scenario, calls the returned `clear`, and asserts a subsequent call behaves as it did before injection". No scenario is purely mechanism-level, so the criterion cannot be met without one of them being implemented.
- **Fix:** `go_silent` was implemented in task 1, in `scenario_conn.go` (task 2's file, created early). It is the scenario that exercises the single accessor for *every* method, so it is the mechanism's natural demonstrator, and its T-02-15 mitigation had to land with `Close`'s ordering anyway. Task 2 added its deep test (`TestScenarioGoSilent`, the deadline-class timing bound) as planned.
- **Files modified:** `internal/talossim/scenario_conn.go`
- **Verification:** `TestScenarioGoSilent` and `TestInjectAndClearRestorePreviousBehaviour` both pass; the gate disabled produces a red `TestScenarioGoSilent`.
- **Committed in:** `fce1947`

**5. [Scope] `internal/talos/version.go` and `version_test.go` created**

- **Found during:** Task 2
- **Issue:** The plan asks for "a single exported constant pair in `internal/talos`, referenced by both the check and its test", and lists only `internal/talos/client.go` under `files_modified`. It names no file for the constants.
- **Fix:** A dedicated `version.go` rather than more surface on `client.go`, which is already the constructor plus the client type. The test reads the bounds from the exported constants, so widening the range stays one edit.
- **Files modified:** `internal/talos/version.go`, `internal/talos/version_test.go`
- **Verification:** `TestCheckSupportedVersion` covers both edges, both sides, prereleases and unparseable input.
- **Committed in:** `036c359`

---

**Total deviations:** 5 (1 bug, 1 blocking, 1 missing critical, 2 scope/sequencing)
**Impact on plan:** No scope creep. Deviations 1–3 were necessary for the plan's own claims to be true — deviation 1 in particular was found by the negative-control discipline the plan asked for, and it was a live instance of the exact failure mode the completeness gate exists to catch. Deviations 4 and 5 are file-placement choices inside the plan's stated intent.

## The completeness gate's negative control

The plan requires this to be recorded. `TestEveryScenarioIsImplemented` was run with `etcd_down`'s gate condition deliberately short-circuited to `false`:

```
--- FAIL: TestEveryScenarioIsImplemented/etcd_down
    scenario_test.go:903: scenario "etcd_down" is indistinguishable from the un-injected
    baseline (both observed "OK"): it is registered, documented and inert
```

The implementation was then restored and the gate observed green across all nine entries. The same discipline was applied to `go_silent` (gate disabled), `flap_connection` (dispatch disabled), `ip_changes_on_reboot` (rebind disabled), `version_out_of_supported_range` (range check disabled) and `slow_log_consumer` (unbounded switch disabled) — each red, each restored. `slow_log_consumer` was the one that stayed green, which is how deviation 1 was found.

## Prohibitions from the plan

Both are now **resolved**:

- *"A scenario must never be assertable only through the error return of a machinery client call."* Every scenario's test asserts on a timing bound (`go_silent`), a sim-side counter (`reject_apply`, `flap_connection`, `slow_log_consumer`), the connection state (`ip_changes_on_reboot`), a resource read shape (`k8s_down`), or an asymmetry across two calls on one connection (`etcd_down`). Where a status code is asserted it is never the only assertion.
- *"talossim must not become easier to satisfy than a real node."* Each scenario reproduces the documented shape: `InvalidArgument` for a rejected apply (PITFALLS.md P11 — Talos rejects before writing, the node is unaffected), `AlreadyExists` for a repeated bootstrap, `Unavailable` scoped to `Etcd*` only, a not-found rather than a transport error for a missing Kubernetes resource, and an abandoned address that refuses rather than times out. The one place this is an approximation is recorded as coverage item D9.

## Issues Encountered

- **`Streamer` blocking is unconditional, so "the send is blocked" is not a scenario signature.** Resolved by making unbounded production the scenario's distinguishing property (deviation 1).
- **macOS assigns only `127.0.0.1` to `lo0`.** `ip_changes_on_reboot` therefore rebinds to a new *port* on `127.0.0.1` rather than to `127.0.0.2`. The plan's flagged assumption 4 already anticipated an approximation here; this is its portable form. Recorded as coverage item D9 with `human_judgment: true`.
- **`ip_changes_on_reboot` does not sever established connections**, deliberately: closing a listener stops new accepts and leaves open connections alone, which is what lets the `Reboot` reply be delivered before the node comes back elsewhere. `flap_connection` does sever, through a tracking listener, because a flap an already-connected client cannot observe is not a flap.

## User Setup Required

None — no external service configuration required. This plan adds no module requirement of any kind; `go.mod` and `go.sum` are unchanged (threat T-02-SC has nothing to check).

## Next Phase Readiness

- **Plan 02-05 can start.** The nine scenarios are injectable, and `Definition.ExpectedClient` is the normative statement it asserts the production client against. `internal/talos/client.go` is free — this plan's edit landed in wave 3, 02-05's lands in wave 4, so they do not race.
- **One dependency to watch:** the `ExpectedClient` column is derived from `02-CONTEXT.md`'s `<deadline_policy>`, which is still marked **PROPOSED, awaiting confirmation**. Plan 02-05 carries the checkpoint that confirms it. If that checkpoint changes a deadline class or the retry allowlist, `Registry`'s `ExpectedClient` strings and the matching rows in `docs/talossim.md` must change in the same commit — `TestDocumentedScenariosMatchRegistry` will not catch a prose change, only a row-set change.
- **TRANS-06 closes here**, as the second and last of its two declaring plans. TRANS-07 closes with it.
- **D8 is the standing caveat:** nothing inside the simulator can prove the simulator is faithful to hardware. Phase 4's QEMU walking skeleton is the first place that becomes checkable, and it is where a wrong status code or a wrong timing shape will surface.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-29*

## Self-Check: PASSED

- Files claimed created, verified present on disk: `internal/talossim/scenario.go`, `internal/talossim/scenario_conn.go`, `internal/talossim/scenario_rpc.go`, `internal/talossim/scenario_test.go`, `internal/talos/version.go`, `internal/talos/version_test.go`, `docs/talossim.md` — all FOUND.
- Commits claimed, verified in `git log`: `fce1947`, `036c359`, `f9ffd7c` — all FOUND.
- Plan-level verification re-run at close: `go test ./... -count=1 -race` green across every package; `golangci-lint run` reports 0 issues; `internal/talossim` has no non-test file importing `testing`; all nine TRANS-07 tokens appear in both `internal/talossim/scenario.go` and `docs/talossim.md`; the slowest single test in the package is `TestScenarioGoSilent` at 0.42 s, well inside the 30 s bound.
