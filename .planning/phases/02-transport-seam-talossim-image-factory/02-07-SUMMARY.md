---
phase: 02-transport-seam-talossim-image-factory
plan: 07
subsystem: infra
tags: [talos, grpc, interceptor, dry-run, config, audit, react, accessibility]

requires:
  - phase: 02-05
    provides: the single shared connect helper, the deadline class table, the retry allowlist, the breaker and classify
  - phase: 02-06
    provides: the Schematics API, the Deps.Factory precedent and the audit allowlist pattern
  - phase: 02-03
    provides: talossim's per-method call counters, which are what prove nothing reached the listener
provides:
  - "talos.ErrDryRun and the dryRunInterceptor pair, installed in dial so no constructor can forget them"
  - "talos.Mode as a required constructor parameter on both client types"
  - "--dry-run / HOLZKUBE_DRY_RUN, with a startup warning that states the consequence"
  - "httpapi.Deps.TalosMode, the value a Phase 6 handler must pass to NewClusterClient"
  - "dry_run on GET /api/v1/auth/me and the DryRunBanner it feeds"
  - "the permanent audit actor vocabulary for non-HTTP mutations (system, job:<id>) in docs/api-contract.md"
  - "setup refuses `system` as an operator username"
affects: [phase-06-jobs-engine, phase-05-streaming, phase-08-acceptance]

actuals:
  tokens: 18261
  tasks: 5
  commits: 5

tech-stack:
  added: []
  patterns:
    - "A process-level safety mode is enforced as a gRPC client interceptor on the one shared connect path, never per call site"
    - "A mode value is a required constructor parameter, so a call site cannot inherit the answer"
    - "A refusal made before the wire is passed through classify untouched, so the breaker never counts a call that never happened"

key-files:
  created:
    - internal/talos/dryrun.go
    - internal/talos/dryrun_test.go
    - internal/talos/export_test.go
    - internal/httpapi/handlers/auth_test.go
    - web/src/components/DryRunBanner.tsx
    - web/src/components/DryRunBanner.test.tsx
  modified:
    - internal/talos/client.go
    - internal/talos/errors.go
    - internal/talos/seam_test.go
    - internal/config/config.go
    - internal/httpapi/router.go
    - internal/httpapi/handlers/auth.go
    - internal/httpapi/handlers/setup.go
    - cmd/holzkubed/main.go
    - docs/api-contract.md
    - web/src/api.ts
    - web/src/components/AppShell.tsx

key-decisions:
  - "Task-1 checkpoint auto-resolved to option-a under mode: yolo — `system` and `job:<id>`, documented now, unused until Phase 6, with `system` refused as an operator username. NEEDS OPERATOR CONFIRMATION."
  - "Task-2 checkpoint auto-resolved to option-b under mode: yolo — the dry-run field rides on GET /api/v1/auth/me, not on the pre-authentication system/status. NEEDS OPERATOR CONFIRMATION."
  - "talos.Mode is a required fifth constructor parameter rather than a variadic option: a policy that can be omitted is one that will be, and the call site that omits this one mutates a node while the operator believes nothing can"
  - "The dry-run interceptor is chained innermost, after the policy interceptor, so it is genuinely the last frame before the invoker"
  - "classify passes ErrDryRun through untouched: classified it would arrive as KindUnreachable and open the breaker of every node the process declined to mutate"
  - "MaintenanceClient deliberately gets no test-only raw-invoke window, because TestMaintenanceClientMethodSetIsClosed reflects over its method set in the same test binary"

patterns-established:
  - "Bypass proof in two halves: the existing import guard for outside the package, a new AST construction guard for inside it"
  - "A guard reports its inspected count and has a negative control pointed at an empty tree AND at a violating one"

requirements-completed: [FOUND-12]

coverage:
  - id: D1
    description: "With --dry-run set, every RPC the class table marks ClassMutation is refused before the wire and the simulator records zero calls for it"
    requirement: FOUND-12
    verification:
      - kind: integration
        ref: "internal/talos/dryrun_test.go#TestDryRunRefusesEveryMutationAtTheNode"
        status: pass
    human_judgment: false
  - id: D2
    description: "The enumeration is not vacuous: with dry-run off the same 21 calls reach the node and every counter advances"
    requirement: FOUND-12
    verification:
      - kind: integration
        ref: "internal/talos/dryrun_test.go#TestDryRunOffLetsTheSameMutationsThrough"
        status: pass
    human_judgment: false
  - id: D3
    description: "Reads and streams still work under dry-run, and maintenance-mode ApplyConfiguration is refused like everything else"
    requirement: FOUND-12
    verification:
      - kind: integration
        ref: "internal/talos/dryrun_test.go#TestDryRunLeavesReadsAndStreamsAlone"
        status: pass
      - kind: integration
        ref: "internal/talos/dryrun_test.go#TestDryRunRefusesApplyConfigurationInMaintenanceMode"
        status: pass
    human_judgment: false
  - id: D4
    description: "No call site can reach the network another way: one client.New and one literal per client type, all inside internal/talos"
    requirement: FOUND-12
    verification:
      - kind: unit
        ref: "internal/talos/seam_test.go#TestEveryClientConstructionRoutesThroughTheSharedPath"
        status: pass
      - kind: unit
        ref: "internal/talos/seam_test.go#TestConstructionGuardIsNotVacuous"
        status: pass
    human_judgment: false
  - id: D5
    description: "--dry-run is a flag and an environment variable with the documented precedence and default, and the startup log reports the effective value, its origin and a warning naming the consequence"
    requirement: FOUND-12
    verification:
      - kind: unit
        ref: "internal/config/config_test.go#TestPrecedenceFlagBeatsEnvBeatsDefault/dry-run"
        status: pass
      - kind: unit
        ref: "internal/config/config_test.go#TestBindBeyondLoopbackIsWarned"
        status: pass
      - kind: manual_procedural
        ref: "holzkubed --dry-run: WARN 'dry-run is on: every mutating call is refused at the transport and no mutation will reach any node'"
        status: pass
    human_judgment: false
  - id: D6
    description: "The running binary reports the mode it was started in on GET /api/v1/auth/me, in both states"
    requirement: FOUND-12
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/auth_test.go#TestMeReportsTheDryRunMode"
        status: pass
      - kind: manual_procedural
        ref: "built binary on :18445 with --dry-run answered dry_run:true; on :18444 without it answered dry_run:false"
        status: pass
    human_judgment: false
  - id: D7
    description: "The dry-run indicator has no dismiss control and no state that could remember having been seen, and is distinguishable from the chain-break banner when both apply"
    requirement: FOUND-12
    verification:
      - kind: automated_ui
        ref: "web/src/components/DryRunBanner.test.tsx (9 cases: no interactive element, no dismiss affordance, no localStorage/sessionStorage/cookie write, both banners distinguishable by role)"
        status: pass
    human_judgment: false
  - id: D8
    description: "The banner is visible above the whole shell in a browser when the binary is started with --dry-run"
    verification:
      - kind: manual_procedural
        ref: "the embedded bundle served by the running binary contains the banner copy; AppShell mounts the container next to ChainBannerContainer"
        status: pass
    human_judgment: true
    rationale: "Never opened in a real browser — only jsdom plus a bundle-content check. This extends the standing Phase 1 / 02-06 concern about the embedded UI to a third surface."
  - id: D9
    description: "The audit actor vocabulary for non-HTTP mutations is permanently documented, and `system` is refused as an operator username"
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/auth_test.go#TestSetupRefusesTheReservedActorUsername"
        status: pass
      - kind: manual_procedural
        ref: "docs/api-contract.md '### Actor vocabulary'; live binary answered 400 validation.failed for username 'system'"
        status: pass
    human_judgment: true
    rationale: "The tokens themselves were auto-selected at a blocking checkpoint under mode: yolo rather than confirmed by the operator. `actor` is hashed into every subsequent record and the archive has no deletion path (D-16), so the operator should read and confirm the vocabulary before the first non-HTTP writer lands in Phase 6."

duration: 28 min
completed: 2026-08-29
status: complete
---

# Phase 2 Plan 07: Dry-run at the transport Summary

**A `--dry-run` gate as a gRPC client interceptor on internal/talos's one shared connect path, proven at the node by enumerating all 21 mutating RPCs against `talossim` and asserting a zero server-side call counter for each, plus the flag, the `auth/me` field, the non-dismissible banner and the permanent audit actor vocabulary.**

## Performance

- **Duration:** 28 min
- **Started:** 2026-08-29T08:28:00Z
- **Completed:** 2026-08-29T08:56:00Z
- **Tasks:** 5 (2 checkpoints auto-resolved, 3 executed)
- **Files modified:** 26

## Accomplishments

- **The refusal is where FOUND-12's wording requires it.** `dryRunInterceptor` is a unary/stream pair chained *after* `conn.unaryPolicy` and `conn.streamPolicy`, which makes it the innermost frame before gRPC's invoker, installed in `dial` — the one function both `ClusterClient` and `MaintenanceClient` reach a node through. No constructor can forget it and no wrapper method can opt out.
- **Mutation membership is read from the deadline class table, never from a second list.** `refuseIfMutating` calls `ClassOf(method)` and refuses `ClassMutation`. An RPC nobody classified still fails loudly on `ErrUnclassifiedMethod` rather than being waved through as "not a mutation" (T-02-43).
- **The proof is made at the node, in three parts.** The enumeration iterates the class table (21 methods today), calls each through the shared path against a live `talossim`, and asserts both `errors.Is(err, talos.ErrDryRun)` and a zero per-method counter. The negative control runs the same 21 with dry-run off and asserts every counter advances. The bypass guard proves there is exactly one `client.New` call and one composite literal per client type inside `internal/talos`, complementing the existing import guard that already forbids the machinery client outside it.
- **`talos.Mode` is a required constructor parameter.** Not a variadic option, not a package variable. Seventeen call sites were updated by the compiler; a future Phase 6 call site cannot inherit the answer.
- **The mode is visible three ways** — a startup `WARN` that states the consequence rather than the setting, `dry_run` on `GET /api/v1/auth/me`, and a banner above the whole shell with no dismiss control and no persisted state.
- **Both permanent contract additions landed.** The D-10 actor vocabulary (`system`, `job:<id>`) with the reason its format is fixed now, and the dry-run field with its operational meaning and its deliberate scope limit.

## Task Commits

1. **Task 3 RED: the dry-run proof** — `40f2fde` (test)
2. **Task 3 GREEN: refuse every mutation before the wire** — `6970536` (feat)
3. **Task 4 RED: flag and endpoint tests** — `a5f921d` (test)
4. **Task 4 GREEN: flag, composition root, contract** — `d628ed5` (feat)
5. **Task 5: the banner** — `b1e5917` (feat)

Tasks 1 and 2 are `checkpoint:decision` and produced no code of their own; their outcomes are written into commits 4 and 5.

## Files Created/Modified

- `internal/talos/dryrun.go` — `ErrDryRun`, `Mode`, `dryRunInterceptor`, `refuseIfMutating`
- `internal/talos/dryrun_test.go` — the enumeration, the negative control, reads/streams, maintenance mode, the message contract, the classification
- `internal/talos/export_test.go` — test-only `RawInvoke`/`RawStream` on `ClusterClient` so the enumeration can reach the eighteen mutating RPCs that have no wrapper
- `internal/talos/client.go` — `Mode` threaded through `dial` and both constructors; the interceptors installed in the shared path
- `internal/talos/errors.go` — `classify` passes `ErrDryRun` through untouched
- `internal/talos/seam_test.go` — the construction guard and its two negative controls
- `internal/config/config.go` — the `dry-run` option entry and the startup warning
- `internal/httpapi/router.go` — `Deps.TalosMode`
- `internal/httpapi/handlers/auth.go` — `dry_run` on `meResponse`
- `internal/httpapi/handlers/setup.go` — `system` refused as an operator username
- `cmd/holzkubed/main.go` — `talos.Mode{DryRun: cfg.DryRun}` set inside the `httpapi.Deps{}` literal
- `docs/api-contract.md` — `### Actor vocabulary`, `## Dry-run`, the `auth/me` response shape
- `web/src/api.ts`, `web/src/components/DryRunBanner.tsx`, `DryRunBanner.test.tsx`, `AppShell.tsx` — the schema field, the banner, its proof, its mount

## Decisions Made

### The two checkpoints were auto-resolved, and the operator has not confirmed them

`.planning/config.json` has `mode: "yolo"` ("runs autonomously without prompts") while `workflow.auto_advance` and `workflow._auto_chain_active` are both `false`. Both checkpoints carry `gate="blocking"` — the auto-approvable gate — rather than `gate="blocking-human"`, and both recommendations restate decisions already recorded elsewhere in the project. On that reading the recommendations were taken:

- **Task 1 → option-a.** `system` and `job:<id>`, documented now, written by nothing in Phase 2, with the username collision closed by refusing `system` at setup. This is D-10 exactly as already recorded.
- **Task 2 → option-b.** The dry-run field on `GET /api/v1/auth/me` rather than on `GET /api/v1/system/status`, honouring the Phase 1 decision in `STATE.md` about that endpoint answering before authentication. This is a deliberate, documented deviation from `02-PATTERNS.md`, which recommends the other endpoint on convenience grounds without the Phase 1 decision in view.

**Task 1's outcome is one-way and should be read before Phase 6 starts.** `Actor` is in `canonicalFields`, so it is hashed into every subsequent record, and D-16 gives the archive unlimited retention and no deletion path. Changing the vocabulary after the jobs engine is writing costs either a chain break at the seam or an archive-wide rewrite. Nothing writes either token yet, so today the cost of changing it is a documentation edit — that window closes when Phase 6 lands. This is the same shape as the 02-02 taxonomy checkpoint the operator later confirmed.

### `talos.Mode` is a required parameter, not an option

The alternative was `...ClientOption` with `WithDryRun`, which would have touched no existing call site. It was rejected for the reason `deadline.go` gives about default deadlines and `retry.go` gives about its allowlist direction: a policy that can be omitted is a policy that will be omitted, and the call site that omits *this* one mutates a production node while the operator believes nothing can. Seventeen test call sites now pass `talos.Mode{}`; the zero value is the live mode, so dry-run is always something a caller asked for.

### The interceptor is innermost, and `classify` had to learn about it

Chaining the gate after the policy interceptor makes it genuinely the last frame before the invoker — but it also puts `ErrDryRun` inside `conn.do`'s callback, where a failure is handed to `classify`. Unclassified, it would have become `KindUnreachable`: there are no response trailers, because nothing was sent. A process started with `--dry-run` would then have opened the circuit breaker of every node it declined to mutate, on the strength of calls that never happened. `classify` now returns it untouched, alongside the cancellation case it already handled — and `ErrorKindOf`'s existing doc comment already promised exactly this ("a refusal from this package before the wire"), so this made the code match a promise the package had already made.

### `MaintenanceClient` gets no test-only window

`export_test.go` puts `RawInvoke`/`RawStream` on `ClusterClient` only. `TestMaintenanceClientMethodSetIsClosed` reflects over the maintenance client's exported method set in the same test binary, so a test-only method on it is still a method on it — the guard caught this immediately, correctly. It is also unnecessary: the one mutation that path serves is `ApplyConfiguration`, which has a real wrapper, and the gate it hits lives in `dial`, which both paths share.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] `system` refused as an operator username**

- **Found during:** Task 4
- **Issue:** T-02-47 is `mitigate` in the plan's threat register and the recommendation for task 1 names the mitigation ("refusing `system` as an operator username at setup"), but the plan's `<action>` only asked for it to be *documented*. Writing the rule into `docs/api-contract.md` without implementing it would have made the contract a lie about a permanent, unfixable-later property.
- **Fix:** `isReservedActor` in `internal/httpapi/handlers/setup.go`, case-insensitive, returning `400 validation.failed` with a reason that says why. `job:<id>` needs no rule — `:` is not a legal username character.
- **Files modified:** `internal/httpapi/handlers/setup.go`, `internal/httpapi/handlers/auth_test.go`
- **Verification:** `TestSetupRefusesTheReservedActorUsername` (three casings refused, `systems-admin` accepted); confirmed against the running binary.
- **Committed in:** `d628ed5`

**2. [Rule 1 - Bug] `classify` misclassified the dry-run refusal**

- **Found during:** Task 3
- **Issue:** With the interceptor innermost, `ErrDryRun` reached `classify` with no trailers and became `KindUnreachable` — a transport failure the circuit breaker counts.
- **Fix:** Pass it through untouched, matching `ErrorKindOf`'s existing documented contract.
- **Files modified:** `internal/talos/errors.go`
- **Verification:** `TestDryRunRefusalIsNotATransportFailure` asserts `ErrorKindOf` reports `false` for the refusal.
- **Committed in:** `6970536`

**3. [Rule 3 - Blocking] `MaintenanceClient` raw-invoke helpers removed**

- **Found during:** Task 3
- **Issue:** `export_test.go` initially put `RawInvoke`/`RawStream` on both client types, which broke `TestMaintenanceClientMethodSetIsClosed` — the guard 02-05 wrote to hold D-06.
- **Fix:** Removed them from `MaintenanceClient` with a comment saying why, and proved the maintenance path through the real `ApplyConfiguration` wrapper instead.
- **Files modified:** `internal/talos/export_test.go`
- **Verification:** full `go test ./... -race` green.
- **Committed in:** `6970536`

### Scope notes, not deviations

- **`internal/talos/maintenance.go` does not exist.** The plan's `files_modified` lists it; 02-05 put `MaintenanceClient` in `client.go` alongside `ClusterClient` and the shared `conn`. The interceptor install is therefore one edit in `client.go`, which is *more* faithful to "the single shared connect path" than two would have been.
- **`internal/httpapi/handlers/system.go` was not touched,** as the plan instructs when task 2 chooses the authenticated endpoint. `GET /api/v1/system/status` is unchanged.
- **The `## Route Registration Rule` clause was already present.** Plan 02-06 wrote "That field is the third and last permitted `router.go` edit" in wave 3, so `Deps.TalosMode` is a covered edit and nothing had to be added.
- **`internal/httpapi/handlers/account_test.go` was refactored,** not listed in the plan: `newServerWithFactory` became `newServerWith(t, sudoWindow, factory, mode)` with three thin wrappers, so the transport mode is set *inside* the `Deps{}` literal in the harness as well as in production.

---

**Total deviations:** 3 auto-fixed (1 bug, 1 missing critical, 1 blocking)
**Impact on plan:** All three were required for correctness. The reserved-username refusal is the only added surface, and it is the mitigation the plan's own threat register assigned to this plan.

## Issues Encountered

**The bypass-guard threshold observation (acceptance criterion).** `TestEveryClientConstructionRoutesThroughTheSharedPath` logs `inspected 12 production files and 3 construction site(s) in internal/talos` and fails below 8 files. `TestConstructionGuardIsNotVacuous` points the same walk at an empty `t.TempDir()` and asserts it reports 0 files and 0 sites — which is what makes the threshold a real assertion rather than decoration — and then points it at a synthetic file containing a `client.New` call and a `&ClusterClient{}` literal in a function called `somewhereElse`, asserting both are attributed to that function. Recorded here as the plan asks.

**The Phase 5 entry blocker was left alone, as instructed.** `cmd/holzkubed/main.go`'s process-wide `WriteTimeout = 60 s` and the three `ResponseWriter` wrappers that implement none of `Flush`/`Unwrap`/`Hijack` are untouched. This plan permits streams under dry-run (a stream is a read) but adds no streaming HTTP route, so the collision is still not exercised.

**`internal/talossim/scenario.go`'s `ExpectedClient` strings and `docs/talossim.md` are byte-identical to before.** Nothing in the nine scenarios changed: the dry-run gate is off in every one of them, since `talos.Mode{}` is what the contract suite passes.

## User Setup Required

None — no external service configuration required. Two new operator-facing switches exist: `--dry-run` and `HOLZKUBE_DRY_RUN`, both defaulting to off, both listed in `holzkubed --help`.

## Next Phase Readiness

**Phase 2 is complete.** All eight plans have summaries.

Ready for Phase 6:

- `httpapi.Deps.TalosMode` is the value a node-calling handler must pass to `talos.NewClusterClient` — it is already threaded from the composition root into every handler closure.
- The audit actor vocabulary is fixed and documented, so the jobs engine has a token to write on the day it needs one.
- The bypass guard will fail the moment a second `client.New` or a second client literal appears inside `internal/talos`, which is the shape a Phase 6 shortcut would take.

Concerns carried forward:

1. **The two checkpoint decisions are unconfirmed.** Task 1's is one-way and its window closes when Phase 6's jobs engine writes its first record. Task 2's is cheap to revisit.
2. **The dry-run banner has never been opened in a browser** — jsdom plus a bundle-content check only. This is the third surface under the standing Phase 1 concern about the embedded UI (dashboard, `/images`, now the banner).
3. **`--dry-run` gates node mutations, not local state.** A schematic created while dry-run is on still writes to the store and to the audit archive and still contacts the Image Factory. That is FOUND-12's literal scope ("keine Mutation erreicht einen **Node**") and D-03's placement, recorded so a reviewer does not read the narrower scope as an omission.
4. **`golangci-lint` was run and is green** (`0 issues`) with `~/go/bin` on PATH, as is `go test ./... -count=1 -race` and the full web suite (91 tests) and lint.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-29*

## Self-Check: PASSED

- All six created files present on disk (`internal/talos/dryrun.go`, `dryrun_test.go`, `export_test.go`, `internal/httpapi/handlers/auth_test.go`, `web/src/components/DryRunBanner.tsx`, `DryRunBanner.test.tsx`).
- All five task commits found in `git log`: `40f2fde`, `6970536`, `a5f921d`, `d628ed5`, `b1e5917`.
- Plan-level verification re-run at the end: `go build ./...` clean, `go test ./... -count=1 -race` green, `golangci-lint run` `0 issues`, `npm --prefix web run test` 91 passed, `npm --prefix web run lint` clean, `npm --prefix web run build` (which is `tsc --noEmit && vite build`) clean.
- Running-binary checks: with `--dry-run` the log warns and `auth/me` answers `dry_run:true`; without it, no warning and `dry_run:false`; the served bundle contains the banner copy; `username: "system"` is refused `400`.
