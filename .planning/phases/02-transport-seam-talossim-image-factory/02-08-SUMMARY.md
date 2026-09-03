---
phase: 02-transport-seam-talossim-image-factory
plan: 08
subsystem: infra
tags: [talos, grpc, cosi, machinery, streaming, test-fake, ast-guard, protobuf]

# Dependency graph
requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "plan 02-01's internal/talos seam, internal/talossim's mTLS listeners and gRPC server, the machinery and cosi-project pins"
  - phase: 01-foundation
    provides: "internal/auth's per-key-state-under-one-mutex shape, internal/store/fsstore's AST-walk guard model, internal/depguard_test.go's loud-skip idiom"
provides:
  - "internal/talossim: the MachineService subset the product reaches (Version, Hostname, ServiceList, ApplyConfiguration, Bootstrap, Reboot, Shutdown, Reset, EtcdMemberList, EtcdStatus) plus StorageService.Disks"
  - "internal/talossim: per-server mutable node state (bootstrapped, version, hostname, last boot, reboots, resets, powered-off, applied configs) reachable as Server.Node()"
  - "internal/talossim: in-memory COSI state served on the same gRPC server as MachineService, with Server.COSI() and two seeded resources"
  - "internal/talossim: Logs, Dmesg and Events as bounded, cancellable server streams fed from a per-server emission channel, with Server.Streams()"
  - "TestMethodCoverage: an executed drift guard between the production client's call sites and the simulator's method surface"
affects: [02-03, 02-05, provisioning, discovery, dashboard, sidero-link-retrofit]

actuals:
  tokens: 25892
  tasks: 3
  commits: 3

tech-stack:
  added:
    - github.com/cosi-project/runtime v1.14.1 (promoted from indirect to direct; same pin)
  patterns:
    - "Mutation observability: a state change the simulator makes must be readable back through an RPC, not only through a private field. A Bootstrap changes what EtcdMemberList and ServiceList report."
    - "The simulator is never easier to satisfy than the hardware: a second Bootstrap is refused, an un-bootstrapped node has no etcd member list, and a powered-off node answers Unavailable."
    - "Bounded, unbuffered emission: a server stream's producer blocks on its consumer, so a stalled scenario cannot exhaust memory."
    - "Derived enumeration over hand-maintained lists: the drift guard reads the generated gRPC service descriptors and the production AST, so it cannot disagree with either."
    - "Behavioural rather than structural detection: an unimplemented method is found by calling it and reading codes.Unimplemented, not by reflecting over the server type."

key-files:
  created:
    - internal/talossim/cosi.go
    - internal/talossim/stream.go
    - internal/talossim/coverage_test.go
    - internal/talossim/node_test.go
    - internal/talossim/stream_test.go
  modified:
    - internal/talossim/machine.go
    - internal/talossim/talossim.go
    - go.mod
    - go.sum

key-decisions:
  - "Disks is implemented on StorageService, not MachineService: that is where the resolved machinery module puts it and where client.Client.Disks looks. Both services are registered on the one gRPC server, because the production client builds both stubs from one *grpc.ClientConn."
  - "A second Bootstrap returns codes.AlreadyExists rather than succeeding again, and an un-bootstrapped node answers EtcdMemberList with FailedPrecondition rather than an empty list -- an empty list is a cluster with no members, which is a different fact."
  - "Shutdown and a non-rebooting Reset leave the node answering Unavailable. Unavailable, never Unimplemented: the drift guard reads Unimplemented as 'the simulator has fallen behind', and a machine that is off is a state rather than a drift."
  - "The stream emission channel is unbuffered, and the producer checks its context before the send as well as inside it. Without the pre-check both select arms are ready after cancellation and Go picks at random, so 'at most one message in flight' would have been a probability instead of a fact a test can assert."
  - "The drift guard probes with an empty request and discards the reply. An empty proto3 message encodes to zero bytes, which every request type accepts, so one call shape serves every method the walk can turn up -- including methods added in phases that will never edit this file."
  - "The guard resolves method names against machine.MachineService_ServiceDesc and storage.StorageService_ServiceDesc rather than against a written list, so a method renamed upstream disappears from the guard instead of sitting in a list nobody updated."
  - "Call-site detection is a narrow AST resolution (struct fields, declarations, parameters, client.New assignments, plus the MachineClient/StorageClient accessors) rather than a full type check: golang.org/x/tools would be a module added for the sake of a guard whose point is that the module graph stays small."

patterns-established:
  - "Negative controls for every guard: each of the four claims this plan makes about the simulator (blocks rather than buffers, stops on cancellation, fires on a removed implementation, fires on a new call site) was proven by breaking the implementation once and observing the red."
  - "Probe self-control: a guard whose failure path has never run is tested against deliberately chosen implemented and inherited methods in both call shapes, so its streaming branch is not dead code until the first streaming call site appears."

requirements-completed: []  # TRANS-06 is co-owned with plan 02-03, which has no SUMMARY yet; requirements.ready-ids reports 0/1 ready.

coverage:
  - id: D1
    description: "talossim answers real protobufs across the MachineService/StorageService subset the product reaches this milestone, and every response carries the answering node's identity."
    requirement: TRANS-06
    verification:
      - kind: integration
        ref: "internal/talossim/node_test.go#TestNodeAnswersTheImplementedSurface"
        status: pass
      - kind: integration
        ref: "internal/talossim/node_test.go#TestTwoNodesAreDistinguishable"
        status: pass
    human_judgment: false
  - id: D2
    description: "Node state a mutation changes is observable through a later read: a Bootstrap changes what EtcdMemberList and ServiceList report, and a second Bootstrap is refused as a real node refuses it."
    requirement: TRANS-06
    verification:
      - kind: integration
        ref: "internal/talossim/node_test.go#TestBootstrapChangesWhatALaterReadReports"
        status: pass
      - kind: integration
        ref: "internal/talossim/node_test.go#TestRebootAndShutdownAreObservable"
        status: pass
      - kind: integration
        ref: "internal/talossim/node_test.go#TestResetWipesTheNode"
        status: pass
      - kind: integration
        ref: "internal/talossim/node_test.go#TestDryRunApplyChangesNothing"
        status: pass
    human_judgment: false
  - id: D3
    description: "A method the simulator does not implement answers codes.Unimplemented rather than a zero-valued success -- the inherited path is proven to be the inherited path."
    requirement: TRANS-06
    verification:
      - kind: integration
        ref: "internal/talossim/node_test.go#TestUnimplementedMethodIsUnimplemented"
        status: pass
    human_judgment: false
  - id: D4
    description: "The unmodified production client's COSI accessor reads a resource seeded into the simulator's in-memory state, over the same connection it uses for MachineService."
    requirement: TRANS-06
    verification:
      - kind: integration
        ref: "internal/talossim/stream_test.go#TestProductionClientReadsSeededCOSIState"
        status: pass
      - kind: integration
        ref: "internal/talossim/stream_test.go#TestCOSIIsRegisteredOnTheMachineServiceConnection"
        status: pass
      - kind: integration
        ref: "internal/talossim/stream_test.go#TestScenarioMutationOfCOSIStateIsVisibleToTheClient"
        status: pass
    human_judgment: false
  - id: D5
    description: "Logs, Dmesg and Events stream real messages and complete after the configured number."
    requirement: TRANS-06
    verification:
      - kind: integration
        ref: "internal/talossim/stream_test.go#TestLogsStreamsToCompletion"
        status: pass
      - kind: integration
        ref: "internal/talossim/stream_test.go#TestDmesgAndEventsStream"
        status: pass
    human_judgment: false
  - id: D6
    description: "A stream tears down within a second of the caller cancelling, and the producer goroutine behind it stops rather than parking forever (T-02-08-01)."
    verification:
      - kind: integration
        ref: "internal/talossim/stream_test.go#TestCancelledStreamTearsDownPromptly"
        status: pass
      - kind: unit
        ref: "internal/talossim/stream_test.go#TestCancellingAStreamStopsTheEmitter"
        status: pass
    human_judgment: false
  - id: D7
    description: "The emitter blocks on a stalled consumer rather than buffering without bound, so no stream can run the test process out of memory (T-02-08-01)."
    verification:
      - kind: unit
        ref: "internal/talossim/stream_test.go#TestStalledConsumerBlocksTheEmitter"
        status: pass
      - kind: integration
        ref: "internal/talossim/stream_test.go#TestStreamsRefuseOnAPoweredOffNode"
        status: pass
    human_judgment: false
  - id: D8
    description: "A running test -- not a document -- fails on method drift in either direction, naming the method and stating that the fix is to implement it rather than to remove the call site (T-02-08-02)."
    requirement: TRANS-06
    verification:
      - kind: unit
        ref: "internal/talossim/coverage_test.go#TestMethodCoverage"
        status: pass
      - kind: unit
        ref: "internal/talossim/coverage_test.go#TestMethodCoverageProbeRecognisesBothCallShapes"
        status: pass
    human_judgment: false
  - id: D9
    description: "The module boundary survives the new imports: the Talos root module is absent from the graph, the pins are unchanged, the simulator is unreachable from the product binary, and the product still cross-compiles CGO-free for linux/amd64 and linux/arm64."
    verification:
      - kind: integration
        ref: "internal/depguard_test.go#TestModuleGraphExcludesTalosRoot"
        status: pass
      - kind: integration
        ref: "internal/depguard_test.go#TestPinnedUpstreamVersions"
        status: pass
      - kind: integration
        ref: "internal/depguard_test.go#TestSimulatorIsNotInTheProduct"
        status: pass
      - kind: other
        ref: "CGO_ENABLED=0 GOOS=linux GOARCH={amd64,arm64} go build ./cmd/holzkubed"
        status: pass
    human_judgment: false
  - id: D10
    description: "Plan 02-03 can start from this plan's output alone: the methods its scenarios intercept exist, the streams its slow_log_consumer stalls exist, and the COSI resources its k8s_down removes are seeded."
    verification: []
    human_judgment: true
    rationale: "This is a readiness judgement about a plan that has not been written yet. Every named attachment point exists and is exercised by a test here, but whether the shapes fit 02-03's nine scenarios is a claim only 02-03 can settle. Asserting it green from this side would be the simulator marking its own homework."

# Metrics
duration: 18 min
completed: 2026-08-29
status: complete
---

# Phase 2 Plan 08: talossim as a Credible Oracle Summary

**The unmodified machinery client now reads seeded COSI state and drives eleven real MachineService/StorageService RPCs against an in-process Talos node whose mutations are observable, whose three server streams are bounded and cancellable, and whose method surface is held to the production client's call sites by an executed AST guard.**

## Performance

- **Duration:** 18 min
- **Started:** 2026-08-29T04:39:39Z
- **Completed:** 2026-08-29T04:58:15Z
- **Tasks:** 3
- **Files modified:** 9 (5 created, 4 modified)

## Accomplishments

- `internal/talossim/machine.go` implements the subset the product reaches -- `Version`, `Hostname`, `ServiceList`, `ApplyConfiguration`, `Bootstrap`, `Reboot`, `Shutdown`, `Reset`, `EtcdMemberList`, `EtcdStatus` on `MachineService` and `Disks` on `StorageService` -- on top of the still-embedded `UnimplementedMachineServiceServer`, with every reply carrying the answering node's identity in its `common.Metadata` (T-02-08-03).
- Node state lives on the `Server` behind one mutex in the shape of `internal/auth`'s `Limiter`, and every mutation is readable back through an RPC: a `Bootstrap` moves etcd from `Preparing` to `Running` and gives `EtcdMemberList` a member, a `Reboot` moves every service's last-change timestamp, a `Reset` un-bootstraps the node, and a `Shutdown` leaves it answering `Unavailable`.
- `internal/talossim/cosi.go` serves `state.WrapCore(namespaced.NewState(inmem.Build))` through `protobuf/server.NewState` and `cosiapi.RegisterStateServer` **on the same gRPC server as `MachineService`**, seeds a `SystemInformation` and a `NodeAddress`, and exposes `Server.COSI()` -- the accessor seeding, scenarios and the test all share.
- `internal/talossim/stream.go` implements `Logs`, `Dmesg` and `Events` directly against the `grpc.ServerStreamingServer[T]` generics (machinery declares the stream server types as aliases, so there is no interface to implement), fed from a per-server unbuffered emission channel that blocks rather than buffers and stops at the next iteration after its context is done.
- `TestMethodCoverage` walks `internal/talos`, resolves each discovered method against the generated gRPC service descriptors, and calls it against a live simulator -- failing on `codes.Unimplemented` with a message that names the method and says the fix is to implement it, not to delete the call.
- Every claim this plan makes was proven by breaking the implementation once: the buffering guard, the cancellation guard and both directions of the drift guard were each observed red before being left green.

## Task Commits

1. **Task 1: the MachineService subset with mutable node state** - `ba354c5` (feat)
2. **Task 2: in-memory COSI state and the three server streams** - `fa65427` (feat)
3. **Task 3: the method-drift guard** - `c4bc443` (test)

## Files Created/Modified

- `internal/talossim/machine.go` - The node's mutable state (`nodeState`, the exported `NodeState` snapshot, `Server.Node`/`SetHostname`/`SetVersion`) plus the `MachineService` and `StorageService` implementations and their registration.
- `internal/talossim/cosi.go` - The in-memory COSI state, its registration on the shared gRPC server, `Server.COSI()`, the two seeded resources, and the stable SMBIOS-shaped node UUID.
- `internal/talossim/stream.go` - `Streamer` (the per-server bounded emitter), `Server.Streams()`, and `Logs`, `Dmesg`, `Events`.
- `internal/talossim/talossim.go` - `Options.Now` and `Options.StreamMessages`; the `node`, `cosi` and `streams` fields; construction, registration and seeding before the listeners serve; `Identity()` now reads live state.
- `internal/talossim/node_test.go` - The method-surface, mutation-observability, inherited-path and node-identity tests, driven through the unmodified machinery client.
- `internal/talossim/stream_test.go` - The COSI round-trip through `client.Client.COSI`, the three streams, and the two T-02-08-01 controls.
- `internal/talossim/coverage_test.go` - `TestMethodCoverage` and the probe's own control.
- `go.mod`, `go.sum` - `cosi-project/runtime` promoted from indirect to direct at the same pinned `v1.14.1`.

## Decisions Made

- **`Disks` is a `StorageService` method.** The plan listed it among the `MachineService` subset; the resolved module puts it on `StorageService`, and `client.Client.Disks` reaches it through `c.StorageClient`. The plan itself says to shape against the module rather than from memory, so the module won. Both services are registered on the one gRPC server, which is what the production client's single `*grpc.ClientConn` requires.
- **The simulator refuses what a real node refuses.** A second `Bootstrap` is `AlreadyExists`; `EtcdMemberList` and `EtcdStatus` on an un-bootstrapped node are `FailedPrecondition` rather than an empty success; an empty `ApplyConfiguration` is `InvalidArgument`. Each of these is a place where returning a plausible success would have been easier and would have moved the surprise from this test suite to the hardware.
- **A powered-off node answers `Unavailable`, never `Unimplemented`.** The drift guard reads `Unimplemented` as "the simulator has fallen behind the client". Reusing that code for a node that is switched off would make a state indistinguishable from a defect.
- **The emission channel is unbuffered and the producer checks its context twice.** The pre-check is not redundant: once the context is done, both arms of the `select` are ready and Go chooses between them at random, so without it "at most one message in flight after cancellation" would have been a coin flip rather than something a test could assert.
- **The drift guard derives its enumeration from two sources it cannot contradict** -- the production AST for *what is called*, and the generated `ServiceDesc`s for *what exists on the wire*. The only hand-written name in the file is the machinery client's import path, which is the same string `internal/talos/seam_test.go` writes out for the same reason.
- **The guard probes with an empty request.** An empty proto3 message encodes to zero bytes, which any request type unmarshals, so one call shape covers every method the walk can produce -- including methods added by phases that will never edit `coverage_test.go`. That is what keeps this from becoming the hand-maintained list it replaces.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] `Disks` is not a `MachineService` method**
- **Found during:** Task 1
- **Issue:** The plan lists `Disks` among the nine `MachineService` methods to add. In the resolved `pkg/machinery@v1.13.9` it is declared on `StorageService` (`api/storage/storage_grpc.pb.go:77`), and `client.Client.Disks` calls it via `c.StorageClient`. Implementing it on `machineService` would not have compiled against the interface, and skipping it would have left a method the production client calls unimplemented.
- **Fix:** Added a `storageService` embedding `storage.UnimplementedStorageServiceServer` and registered it on the same `*grpc.Server` as `MachineService`, matching how the production client builds both stubs from one connection.
- **Files modified:** `internal/talossim/machine.go`, `internal/talossim/talossim.go`
- **Verification:** `TestNodeAnswersTheImplementedSurface` calls `cl.Disks(ctx)` through the unmodified client and asserts a populated, node-identified reply.
- **Committed in:** `ba354c5`

**2. [Rule 3 - Blocking] `internal/talossim/talossim.go` had to be modified, and it is not in the plan's `files_modified`**
- **Found during:** Tasks 1 and 2
- **Issue:** The plan's `files_modified` names only `machine.go`, `cosi.go`, `stream.go` and `coverage_test.go`, but the `Server` struct, its constructor and its option set all live in `talossim.go`. Task 2's own text requires registering the COSI state server on "the gRPC server built in `talossim.go`", which cannot be done without touching that file.
- **Fix:** Added `Options.Now` and `Options.StreamMessages`, the `node`, `cosi` and `streams` fields, the registration and seeding calls, and made `Identity()` read live state. The edits are confined to construction and wiring; no existing behaviour changed.
- **Files modified:** `internal/talossim/talossim.go`
- **Verification:** Plan 02-01's whole tracer suite still passes unchanged.
- **Committed in:** `ba354c5`, `fa65427`

**3. [Rule 2 - Missing Critical] The plan named no test files, but every acceptance criterion is a test**
- **Found during:** Tasks 1 and 2
- **Issue:** Both tasks' acceptance criteria are phrased as "a test calls X and observes Y", yet `files_modified` lists only source files plus `coverage_test.go`. The criteria cannot be met without somewhere to put those tests.
- **Fix:** Added `internal/talossim/node_test.go` (method surface, mutation observability, the inherited path, node identity) and `internal/talossim/stream_test.go` (the COSI round-trip through the production client, the three streams, and both T-02-08-01 controls).
- **Files modified:** `internal/talossim/node_test.go`, `internal/talossim/stream_test.go`
- **Verification:** `go test ./internal/talossim/ -count=1 -race` green; each guard was also observed red against a deliberately broken implementation (see Negative Controls below).
- **Committed in:** `ba354c5`, `fa65427`

**4. [Rule 2 - Missing Critical] The emitter's cancellation had a race that made the assertion probabilistic**
- **Found during:** Task 2
- **Issue:** With the context checked only inside the send `select`, both arms are ready once the context is done and Go picks between them at random. A test asserting "the emitter stops after cancellation" could therefore see an unbounded run of further messages with probability 2^-k, which is a flake rather than a guard.
- **Fix:** Added a non-blocking context pre-check at the top of each loop iteration, which bounds post-cancellation delivery to the single message already offered.
- **Files modified:** `internal/talossim/stream.go`
- **Verification:** `TestCancellingAStreamStopsTheEmitter` passes 20 consecutive `-race` runs and fails deterministically when the context handling is removed.
- **Committed in:** `fa65427`

**5. [Rule 2 - Missing Critical] The drift guard's own failure path and streaming branch had never run**
- **Found during:** Task 3
- **Issue:** `TestMethodCoverage` passes because every discovered call site is implemented, so its failure path is untested. Worse, no production call site is streaming yet, so `probeStream` would first execute on the day someone added one -- and a broken branch would then report a working method as missing, or the reverse.
- **Fix:** Added `TestMethodCoverageProbeRecognisesBothCallShapes`, a four-case control over deliberately chosen implemented (`Version`, `Logs`) and inherited (`Containers`, `EtcdSnapshot`) methods across both call shapes, including an assertion that each case is the shape it claims to be.
- **Files modified:** `internal/talossim/coverage_test.go`
- **Verification:** All four cases pass; the control is independent of what `internal/talos` happens to call.
- **Committed in:** `c4bc443`

**6. [Rule 3 - Blocking] `cosi-project/runtime` moved from indirect to direct, and the indirect set grew**
- **Found during:** Task 2
- **Issue:** `go build` failed with `updates to go.mod needed`. `internal/talossim/cosi.go` is the first package in the repository to import `cosi-project/runtime` directly, and importing `pkg/machinery/resources/network` pulls its transitive dependencies (`jsimonetti/rtnetlink`, the `mdlayher` netlink stack, `google/cel-go`, `siderolabs/net`) into the graph as indirect requirements.
- **Fix:** Ran `go mod tidy`. The pinned version is unchanged at `v1.14.1`; no module was chosen or added, only reclassified, and the new indirect entries arrive with `pkg/machinery` packages the plan's own artifact list requires.
- **Files modified:** `go.mod`, `go.sum`
- **Verification:** `TestPinnedUpstreamVersions` and `TestModuleGraphExcludesTalosRoot` still pass; `go list -deps ./cmd/holzkubed` reports zero packages of the Talos root module and zero of `internal/talossim`; both CGO-free cross-compilations succeed.
- **Committed in:** `fa65427`

**7. [Rule 3 - Blocking] `golangci-lint` findings on the new code**
- **Found during:** Tasks 1 and 2
- **Issue:** Three `gosec` G115 integer-overflow findings (`int -> uint64` on the raft index, `uint64 -> uint16` in the UUID formatting) and one `revive` `context-as-argument` finding in a test helper.
- **Fix:** Replaced the narrowing conversions with masks, which are visibly total; moved the `context.Context` parameter to the front of `etcdState`.
- **Files modified:** `internal/talossim/machine.go`, `internal/talossim/cosi.go`, `internal/talossim/node_test.go`
- **Verification:** `golangci-lint run` reports `0 issues.`
- **Committed in:** `ba354c5`, `fa65427`

---

**Total deviations:** 7 auto-fixed (3 blocking, 3 missing critical, 1 bug)
**Impact on plan:** No scope creep. One was a factual correction the plan itself asked for (follow the module, not the plan's method list); two were files the plan's acceptance criteria required but its file list omitted; the rest were an environment blocker, a lint pass and two guards that had been asserted but not tested. Every added test pins a claim the plan's own `must_haves` already made.

## Negative Controls Performed

Each control was applied once as a throwaway edit and reverted. The observed failure messages, as the plan's acceptance criteria require:

**1. An implemented method removed from the simulator.** Deleting `machineService.Version`:

```
--- FAIL: TestMethodCoverage (0.01s)
    coverage_test.go:78: scanned 4 production file(s) across 1 package(s); found 2 machinery-client call site(s): Close, Version
    coverage_test.go:92: Close is not an RPC on either service; nothing for the simulator to implement
    coverage_test.go:108: holzkube calls 1 MachineService/StorageService method(s) that talossim does not implement:
          Version (machine.MachineService): the simulator answered Unimplemented; the method is inherited from UnimplementedMachineServiceServer rather than written

        The simulator has drifted behind the production client. The fix is to implement the method in internal/talossim -- not to remove the call site. Deleting the call is the cheaper way to make this test green and it inverts the guard: the product would keep needing the method and the simulator would stop being an honest stand-in for a node.
```

**2. A production call site added for a method the simulator does not implement.** Adding a throwaway `ClusterClient.Containers` calling `c.c.Containers(ctx, "system", 0)` in `internal/talos/client.go`:

```
--- FAIL: TestMethodCoverage (0.01s)
    coverage_test.go:78: scanned 4 production file(s) across 1 package(s); found 3 machinery-client call site(s): Close, Containers, Version
    coverage_test.go:92: Close is not an RPC on either service; nothing for the simulator to implement
    coverage_test.go:108: holzkube calls 1 MachineService/StorageService method(s) that talossim does not implement:
          Containers (machine.MachineService): the simulator answered Unimplemented; the method is inherited from UnimplementedMachineServiceServer rather than written
        [... same fix guidance ...]
```

**3. The emitter buffering instead of blocking.** Changing `make(chan []byte)` to `make(chan []byte, 1<<20)`:

```
--- FAIL: TestStalledConsumerBlocksTheEmitter (0.10s)
    stream_test.go:232: the emitter produced 100000 messages against a consumer that read none; it is buffering rather than blocking, and a stalled scenario would exhaust memory
```

**4. The producer ignoring its context.** Replacing both context checks with a bare `ch <- msg`:

```
--- FAIL: TestCancellingAStreamStopsTheEmitter (0.00s)
    stream_test.go:246: the emitter delivered 2 further messages after cancellation; it is not reading the stream context and its goroutine outlives the stream
```

## Issues Encountered

- **`task` (go-task) is not installed on this host,** so the plan-level verification lines `task build`, `task test` and `task lint:go` were executed as their literal commands: `go build ./cmd/holzkubed` (the binary starts and prints its option table), `go test ./... -count=1 -race` (17 packages green) and `golangci-lint run` (0 issues, using the version pinned in `.github/workflows/ci.yml`). This plan touches nothing under `web/`, so `build:web` had unchanged inputs. Same finding as plan 02-01.
- **The drift guard currently sees only two call sites** (`Close`, which is not an RPC, and `Version`). That is the truthful state of `internal/talos` at wave 2 -- `dial_direct.go`'s `Probe` deliberately reads a TLS certificate rather than making an RPC, so it contributes no machinery-client call. The guard's value is prospective, and its detection is proven by control 2 above rather than by the size of today's result.

## Known Stubs

| File | What | Why, and what closes it |
|---|---|---|
| `internal/talossim/machine.go` | 11 of the 54 `MachineService` RPCs plus 1 `StorageService` RPC are written; the rest inherit `Unimplemented`. | Deliberate and bounded by design, per the plan's flagged assumption 3: the implemented subset is what the product reaches this milestone, not all 54. This is not an omission that can rot -- `TestMethodCoverage` turns any future call site against an unwritten method into a red test in this package rather than a runtime surprise in a later phase. |
| `internal/talossim/machine.go` (`ApplyConfiguration`) | An applied configuration is counted, not parsed: applying a config that sets a hostname does not change what `Hostname` reports. | Parsing machine configuration would mean pulling `pkg/machinery/config` and its provider machinery into the simulator for a fidelity nothing yet tests. `Server.SetHostname` and `Server.SetVersion` give a scenario the same effect explicitly, which is what plan 02-03's `ip_changes_on_reboot` needs. Closing this properly belongs with whichever phase first drives a real config through the seam. |
| `internal/talossim/stream.go` (`Events`) | Every event carries the same `MachineStatusEvent{Stage: RUNNING}` payload; the event stream is not driven by the node's actual state transitions. | The three streams owed by this plan are "emit, bound and cancel", and all three are proven. Correlating events with the `Bootstrap`/`Reboot`/`Reset` mutations is a scenario-engine concern and is plan 02-03's to shape, since it is that plan that decides which transitions a scenario has to observe. |

## Threat Flags

None. Every trust boundary this plan touches was enumerated in its own `<threat_model>`. `T-02-08-01` (unbounded stream emission), `T-02-08-02` (method drift), `T-02-08-03` (unidentified responses) and `T-02-08-05` (responses shaped to pass rather than to match) all have executed mitigations, each with a negative control or a module-derived shape behind it. `T-02-08-04` was accepted in the plan and remains accepted: the seeded `SystemInformation` and `NodeAddress` are synthetic values created per `talossim.New`, held only in process memory, and never written to disk. `T-02-08-SC` holds -- no module was added, the pins are unchanged, and `TestPinnedUpstreamVersions` and `TestModuleGraphExcludesTalosRoot` both pass.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- **Plan 02-03 has its attachment points.** The methods its scenarios intercept exist and are exercised; `Server.Streams()` exposes the per-server emitter its `slow_log_consumer` stalls, and stalling it is already demonstrated by `TestStalledConsumerBlocksTheEmitter`; `Server.COSI()` is the accessor its `k8s_down` removes resources through, and a removal made that way is already proven visible to the production client. `second_bootstrap_returns_AlreadyExists` is answered honestly by the node today, so that scenario has behaviour to assert rather than behaviour to add.
- **TRANS-06 is not complete and must not be marked so.** It is authored across three plans: 02-01 (listener, mTLS, seam), this one (method surface, COSI state, drift guard) and 02-03 (the scenario engine). `requirements.ready-ids` reports 0 of 1 ready because 02-03 has no SUMMARY yet, and `requirements-completed` here is deliberately empty. The release blocker closes when 02-03 does.
- **Open concern carried forward from 02-01 (TRANS-01):** the concurrency truth is still unproven and still abstains. Nothing in this plan fans out, so nothing here could serialise; plan 02-05 owns the proof.
- **Open concern:** the drift guard's AST detection resolves client-valued identifiers rather than type-checking them. It handles the shapes `internal/talos` uses today (struct fields, declarations, parameters, `client.New` results, and the `MachineClient`/`StorageClient` accessors). A call routed through an interface value or a returned closure would slip past it. That is a deliberate trade against adding `golang.org/x/tools` to a module whose smallness is itself a guarded property -- but it is a limit a later phase should revisit if the call sites get less direct.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-29*

## Self-Check: PASSED

All five created source files and both modified ones exist on disk, the SUMMARY exists, and all three task commits (`ba354c5`, `fa65427`, `c4bc443`) are reachable in `git log`. Every acceptance criterion of all three tasks was executed rather than asserted: `go build ./...`, both CGO-free cross-compilations, `go test ./... -count=1 -race` (17 packages green), `golangci-lint run` at 0 issues, and `go list -deps ./cmd/holzkubed` reporting zero Talos-root and zero `internal/talossim` packages. The four negative controls above were each observed red before being reverted.
