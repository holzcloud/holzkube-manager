---
phase: 02-transport-seam-talossim-image-factory
plan: 22
subsystem: api
tags: [go, imagefactory, timeouts, context-deadline, http-server, talos]

requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "imagefactory.Client, ProbeBuildable's fail-safe tri-state, resolveInstallerRepo's serial candidate walk, cmd/holzkube-managerd/budget_test.go's ratcheting route table"
provides:
  - "imagefactory.ProbeTimeout (90s) and imagefactory.ManifestTimeout (30s), each with the derivation rule and the observations it was applied to in its doc comment"
  - "imagefactory.DefaultTimeout narrowed to the three JSON endpoints its comment was always about"
  - "Per-call context budgets (budgetClass, (*Client).withBudget) replacing http.Client.Timeout"
  - "imagefactory.ErrNoDeadline and requireDeadline at the two call sites that reach http.Client.Do"
  - "imagefactory.WithProbeTimeout and imagefactory.WithManifestTimeout"
  - "handlers.CreateRouteBudget (120s) and handlers.AssetsRouteBudget (65s), one shared deadline per Factory route"
  - "writeTimeout raised 60s -> 130s, derived from CreateRouteBudget + budgetSlack"
  - "A composition guard that sums per-call budgets by class and ratchets R0-R4 in both directions"
  - "An offline reproduction of G-02-1 and a live guard that bounds the probe it runs"
affects: [02-23, 02-24, phase-05-deadline-policy]

actuals:
  tokens: 97796
  tasks: 3
  commits: 6

tech-stack:
  added: []
  patterns:
    - "Per-workload budget classes on the client, applied as context deadlines at the call site (the imagefactory sibling of internal/talos/deadline.go's D-04)"
    - "A structural no-deadline refusal at every site that reaches the wire, with no value that disables it"
    - "One shared route deadline derived before the first upstream call, so the route's worst case is a number it declares"
    - "A composition guard that reads every constant from the package that declares it and ratchets its verdicts in both directions"
    - "A live guard that re-applies a constant's derivation rule to what it measures rather than restating the constant"

key-files:
  created: []
  modified:
    - internal/imagefactory/client.go
    - internal/imagefactory/probe.go
    - internal/imagefactory/installer.go
    - internal/imagefactory/client_test.go
    - internal/imagefactory/fake_test.go
    - internal/imagefactory/tracer_test.go
    - internal/imagefactory/installer_test.go
    - internal/imagefactory/installer_export_test.go
    - internal/imagefactory/live_test.go
    - internal/httpapi/handlers/schematics.go
    - internal/httpapi/handlers/schematics_test.go
    - cmd/holzkube-managerd/main.go
    - cmd/holzkube-managerd/budget_test.go

key-decisions:
  - "The three budgets moved off http.Client.Timeout onto per-call context deadlines: one client-wide value cannot express three budgets, which is the mechanism by which the ISO probe was bounded by a constant sized against a JSON list."
  - "ProbeTimeout = 90s and ManifestTimeout = 30s are derived by one stated rule -- at least twice the slowest observed cold response, rounded up to the next thirty seconds -- applied to recorded observations, not chosen."
  - "ManifestTimeout equals DefaultTimeout by coincidence of that rule and is deliberately a second constant, not an alias."
  - "probeStatus takes its budget class as a parameter because it serves two workloads and must not decide which one it is serving."
  - "The retired sum-versus-writeTimeout comparison is replaced by R1 (routeDeadline + slack < writeTimeout), because once a route declares a deadline the deadline is the worst case and the sum only says whether it clips."
  - "Clipping is declared table data with its own both-directions ratchet rather than a doc comment, so the claim can go stale and go red."
  - "AssetsRouteBudget is sized at two manifest budgets for the serial walk that exists at this wave; plan 02-23 tightens it when the walk becomes concurrent."

patterns-established:
  - "Budget class + withBudget + requireDeadline: the imagefactory statement of 'no call goes to the wire without a deadline', mirroring internal/talos/deadline.go"
  - "Route budgets exported so the guard in package main reads the value that runs rather than restating it"
  - "Scaled-ratio reproduction: a test drives the production *relation* between constants at millisecond scale rather than the production values"

requirements-completed: [FACT-02, FACT-03]

coverage:
  - id: D1
    description: "The ISO probe and the registry manifest GET each have a budget of their own, each carrying the rule that produced it and the observations the rule was applied to; DefaultTimeout governs only the three JSON endpoints."
    requirement: FACT-02
    verification:
      - kind: unit
        ref: "internal/imagefactory/client_test.go#TestEachBudgetBoundsItsOwnWorkloadAndNoOther"
        status: pass
      - kind: unit
        ref: "cmd/holzkube-managerd/budget_test.go#TestRouteBudgetTableReadsTheRealConstants"
        status: pass
    human_judgment: false
  - id: D2
    description: "No request in internal/imagefactory can reach the wire without a deadline; the refusal is a named error and nothing is sent."
    requirement: FACT-02
    verification:
      - kind: unit
        ref: "internal/imagefactory/client_test.go#TestRequestWithNoDeadlineIsRefusedBeforeTheWire"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/client_test.go#TestClientCarriesNoClientWideTimeout"
        status: pass
    human_judgment: false
  - id: D3
    description: "G-02-1's measured failure -- a probe past the JSON budget and inside the probe budget producing no verdict -- is reproducible offline and is green after the change."
    requirement: FACT-02
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestProbeBudgetOutlivesTheJSONBudget"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/client_test.go#TestEachBudgetBoundsItsOwnWorkloadAndNoOther/a_JSON_budget_no_longer_bounds_the_probe"
        status: pass
      - kind: manual_procedural
        ref: "reverted ProbeBuildable's probeStatus calls to classJSON; TestProbeBudgetOutlivesTheJSONBudget went red naming the 165ms answer, TestProbeBudgetStillEndsInNoVerdictWhenItIsExceeded stayed green; restored"
        status: pass
    human_judgment: false
  - id: D4
    description: "The fail-safe tri-state is unchanged: a probe past even the probe budget still yields usable false, probed_at zero and no probe_reason, and a manifest past its budget is an outage rather than a refusal."
    requirement: FACT-02
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestProbeBudgetStillEndsInNoVerdictWhenItIsExceeded"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestProbeBudgetIsNotTheManifestBudget"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/probe_test.go#TestProbeBuildableClassifiesEveryRegistryAnswer"
        status: pass
    human_judgment: false
  - id: D5
    description: "Both Factory routes declare one shared upstream deadline, and each answers inside the scaled equivalent of it against a Factory that never answers."
    requirement: FACT-03
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestCreateRouteAnswersInsideItsCeiling"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestAssetsRouteAnswersInsideItsCeiling"
        status: pass
    human_judgment: false
  - id: D6
    description: "The composition guard sums per-call budgets by class, reads all four constants from the code that runs, and fails in both directions on the verdict and on the clipping."
    requirement: FACT-03
    verification:
      - kind: unit
        ref: "cmd/holzkube-managerd/budget_test.go#TestRouteBudgetsComposeAgainstWriteTimeout"
        status: pass
      - kind: manual_procedural
        ref: "six failure directions driven locally and restored: writeTimeout->60s (R1), CreateRouteBudget<ProbeTimeout (R2), assets routeDeadline removed (R0), store-only row given a deadline (R0 reverse), CreateRouteBudget above the sum (R4 uncut vs declared clipped), assets row declared clipped (R4 + missing rationale)"
        status: pass
    human_judgment: false
  - id: D7
    description: "TestLiveFactory bounds the elapsed time of the probe it runs, measures a genuinely cold one, and re-applies ProbeTimeout's derivation rule to the widened sample."
    requirement: FACT-02
    verification:
      - kind: e2e
        ref: "HOLZKUBE_MANAGER_FACTORY_LIVE=1 go test ./internal/imagefactory/ -run TestLiveFactory -count=1 -v (two runs against factory.talos.dev, 2026-09-03, Talos v1.13.9)"
        status: pass
    human_judgment: false
  - id: D8
    description: "The round's residuals: WINDOWS entries 8, 20 and 48 are moved but not amended here, and AssetsRouteBudget is sized for a serial walk that plan 02-23 replaces. Whether carrying those into 02-23/02-24 in this shape is acceptable is a judgment about sequencing, not something a test can assert."
    verification: []
    human_judgment: true
    rationale: "The claims are about what a *later* plan must do with this plan's output. No assertion in this repository can hold a future plan to them; a human reading the ledger section below is the only mechanism."

duration: 90 min
completed: 2026-09-03
status: complete
---

# Phase 02 Plan 22: Three Upstream Budgets and the Guard That Composes Them Summary

**The ISO probe and the registry manifest GET get budgets of their own with stated derivations, the budgets move from `http.Client.Timeout` onto per-call context deadlines with a structural no-deadline refusal, both Factory routes declare one shared ceiling, and `cmd/holzkube-managerd/budget_test.go` recomputes the whole composition from the four constants that actually run.**

## Performance

- **Duration:** ~90 min
- **First task commit:** 2026-09-03T17:09:42Z (`af8b537`)
- **Last task commit:** 2026-09-03T18:20:55Z (`48cad6c`)
- **Tasks:** 3
- **Files modified:** 13

## Accomplishments

- `imagefactory.ProbeTimeout = 90s` and `imagefactory.ManifestTimeout = 30s` exist as separate constants, each stating the rule that produced it — *at least twice the slowest observed cold response, rounded up to the next thirty seconds* — and naming the observations the rule was applied to. `DefaultTimeout` keeps its value and now governs only the three JSON endpoints its comment was always reasoning about.
- The budgets moved off `http.Client.Timeout` onto per-call context deadlines. One client-wide value cannot express three budgets, and that is precisely the mechanism by which a probe that makes the Factory build a ~335MB image was bounded by a number sized against a list of extensions.
- `ErrNoDeadline` and `requireDeadline` at the two — and only two — places that reach `http.Client.Do`. Removing the client-wide timeout would otherwise have turned a forgotten wrapper into an unbounded goroutine (T-02-105); a test asserts zero requests reach the fake on that path.
- G-02-1's measured failure is reproducible offline. `TestEachBudgetBoundsItsOwnWorkloadAndNoOther/a_JSON_budget_no_longer_bounds_the_probe` reproduces it in **0.21s**; the handler-level `TestProbeBudgetOutlivesTheJSONBudget` reproduces it end to end in 1.64s, of which ~1.4s is the shared handler fixture (argon2id calibration and login) and 165ms is the injected latency.
- `handlers.CreateRouteBudget` (120s) and `handlers.AssetsRouteBudget` (65s) are one deadline each, derived before the first upstream call and shared by all of them. `writeTimeout` rose 60s → 130s, derived from `CreateRouteBudget + budgetSlack = 125s`.
- The composition guard was rewritten from `calls × DefaultTimeout` to a per-class sum with five assertions and five failing directions, and both Factory rows flipped from `knownOverBudget` to `withinBudget`.
- `TestLiveFactory` now bounds how long the probe took, and measures a genuinely cold one for the first time.

## Task Commits

1. **Task 1 (tracer, TDD) — RED: reproduce the probe-budget failure offline** — `af8b537` (test)
2. **Task 1 — GREEN: three budgets, three derivations, no request without a deadline** — `c97b914` (feat)
3. **Task 2 (TDD) — RED: the guard sums budgets instead of multiplying one** — `7f4e3c3` (test)
4. **Task 2 — GREEN: two routes with their own ceiling, a writeTimeout that covers them** — `2e3aa5a` (feat)
5. **Task 3 — the live guard measures the probe instead of restating its budget** — `48cad6c` (test)

**Plan metadata:** see the `docs(02-22)` commit.

## The measurements this round produced

Two live runs against `factory.talos.dev` on **2026-09-03**, Talos **v1.13.9**, `amd64`, `metal`:

| run | cold schematic id | cold probe | warm probe |
|---|---|---|---|
| 1 | `cb87ea331328084882e66b8912fa7f7ba3b670b63fc6023d53a070168f74d4e2` | **28.445563875s** | 3.752628125s |
| 2 | `f99d98ba8f551d00f31443b67a740e98842c51ecba0b403133382f4cce95e6f7` | **31.491771083s** | 2.493417375s |

The two cold ids differ, which is what the nonce is for: each run authored a schematic the Factory had demonstrably never built, and each therefore cost it one ~335MB image build.

Beside the five recorded in `02-DECISION-probe-budget.md` — 30.50, 30.59, 31.18, 31.52, 32.69 — the widened sample of **seven cold observations** is:

> 28.45, 30.50, 30.59, 31.18, 31.52, 31.49, 32.69

**Applying the derivation rule to the widened sample still yields the shipped `ProbeTimeout`.** The slowest observation is unchanged at 32.69s; doubled it is 65.38s; rounded up to the next thirty seconds it is **90s**, which is the constant that shipped. `TestLiveFactory` now re-applies that computation itself, so the next observation that would move it arrives as a red test rather than as nothing.

Two things in this data are worth writing down rather than smoothing over. First, run 1's cold probe at 28.45s is the **first recorded cold observation that would have fit inside the old 30s constant** — the failure G-02-1 measured is not universal, which is exactly why "it worked when I tried it" was never evidence against it. Second, the warm path (2.5–3.8s) is an order of magnitude below the cold one, which is why a warm measurement cannot validate a cold budget and why the warm subtest's own bound is a drift guard rather than a derivation.

## Files Created/Modified

- `internal/imagefactory/client.go` — the three constants and their derivations, `budgetClass`, `(*Client).withBudget`, `requireDeadline`, `ErrNoDeadline`, `WithProbeTimeout`, `WithManifestTimeout`; `http.Client.Timeout` gone from `New` and from `WithHTTPClient`.
- `internal/imagefactory/probe.go` — `probeStatus` takes a budget class and applies it; `requireDeadline` before `http.Client.Do`; `ProbeBuildable` names `classProbe` at both call sites.
- `internal/imagefactory/installer.go` — `resolveInstallerRepo` names `classManifest`; two comments that this plan falsified are corrected.
- `internal/imagefactory/client_test.go` — the no-client-wide-timeout assertion, the no-deadline refusal, the option register, and the per-workload budget matrix.
- `internal/imagefactory/installer_export_test.go` — accessors for the embedded `http.Client`'s `Timeout` and for the two unbudgeted call paths.
- `internal/imagefactory/fake_test.go`, `internal/httpapi/handlers/schematics_test.go` — `answerAfter(shape, d)` latency injection on both fakes, abandoned on caller cancellation.
- `internal/imagefactory/live_test.go` — `probeDerivation`, `assertSatisfiesProbeDerivation`, an elapsed-time bound on the warm subtest, and the cold measurement subtest.
- `internal/httpapi/handlers/schematics.go` — `CreateRouteBudget`, `AssetsRouteBudget`, and the two derived route contexts.
- `internal/httpapi/handlers/schematics_test.go` — the three scaled-ratio reproduction cases and the two elapsed-time route ceiling tests.
- `cmd/holzkube-managerd/main.go` — `writeTimeout` 60s → 130s with its derivation and a pointer to the guard.
- `cmd/holzkube-managerd/budget_test.go` — the guard rewritten around per-class sums, `routeDeadline`, clipping, and R0–R4.

## The `must_haves.truths`, checked

| # | Truth | Holds | How I know |
|---|---|---|---|
| 1 | Probe and manifest each bounded by a constant of their own, neither by the JSON one | yes | `classProbe` at both `ProbeBuildable` sites, `classManifest` at the `resolveInstallerRepo` site; `TestEachBudgetBoundsItsOwnWorkloadAndNoOther` gives each workload a roomy budget and the other two a narrow one and passes for all three |
| 2 | Every request carries a deadline; one without is refused before the wire | yes | `requireDeadline` at `do` and `probeStatus`, the only two `http.Client.Do` call sites (`grep -n 'c.http.Do' internal/imagefactory/*.go` returns exactly two); `TestRequestWithNoDeadlineIsRefusedBeforeTheWire` asserts `ErrNoDeadline` **and** a fake request count of zero on both paths |
| 3 | A probe past the JSON budget and inside the probe budget produces a verdict; reproducible offline in under a second | yes | `TestEachBudgetBoundsItsOwnWorkloadAndNoOther/a_JSON_budget_no_longer_bounds_the_probe` runs in **0.21s**. The handler-level case runs in 1.64s wall, ~1.4s of which is the argon2id fixture, not the reproduction. Reverting `ProbeBuildable` to `classJSON` turns the handler case red naming the 165ms answer; restored |
| 4 | A probe past the probe budget still produces no verdict, no `usable: true`, no invented refusal | yes | `TestProbeBudgetStillEndsInNoVerdictWhenItIsExceeded` asserts 201, `usable` false, `probed_at` zero, `probe_reason` empty. `registryRefused` is byte-identical to its pre-plan form (`git diff 59988a2..HEAD -- internal/imagefactory/probe.go` touches no line containing it) |
| 5 | Both Factory routes carry one route deadline shared by every upstream call | yes | One `context.WithTimeout` in `createSchematic` before `Author` (and the following `Store.Put` runs on it), one in `schematicAssets` before `InstallerImage`; `TestCreateRouteAnswersInsideItsCeiling` and `TestAssetsRouteAnswersInsideItsCeiling` measure elapsed time, not only status |
| 6 | `writeTimeout` covers the largest route budget with slack, and its comment names the upstream budgets alongside the argon2id and rate-limiter reasoning | yes | 130s vs `CreateRouteBudget + budgetSlack = 125s`; R1 asserts it; the old argon2id/`maxInlineDelay` paragraph is preserved verbatim and the new paragraph names both route budgets, their file, the derivation and `cmd/holzkube-managerd/budget_test.go` by path |
| 7 | The guard sums by class, reads every constant from the code that runs, and fails in both directions | yes | `upstreamCall.budget()` reads the three `imagefactory` constants; `routeDeadline` reads the two `handlers` constants; `grep -c` reports ≥1 for each of `imagefactory.ProbeTimeout`, `imagefactory.ManifestTimeout`, `handlers.CreateRouteBudget`, `handlers.AssetsRouteBudget` in the guard, and the guard restates no value. Six failure directions driven locally and restored (listed under D6) |
| 8 | The probe budget's value is derived by a stated rule from measured cold observations, and the live guard re-applies that rule to a widened sample | yes | `ProbeTimeout`'s doc comment states the rule and lists the five observations; `probeDerivation` computes `ceil(2×observed / 30s) × 30s` from the observation and compares against the constant, restating neither the 90 nor the five |
| 9 | `TestLiveFactory` bounds the elapsed time of the probe it runs | yes | Both the warm subtest and the new cold subtest measure and log elapsed time on every outcome and call `assertSatisfiesProbeDerivation`; a cold probe that had drifted past the shipped budget would be red rather than slow-green |

### The prohibitions, checked

- **No new route, probe state, stored field or response field.** `TestSchematicRoutesAreTheSevenContracted` still passes against the same seven routes; `model.Schematic` is untouched; the three probe outcomes (verdict-usable, verdict-refused, no verdict) are unchanged. This is Option 2 of the decision, not Option 1.
- **No timeout value that no measurement supports.** `ProbeTimeout` and `ManifestTimeout` each carry their rule and their inputs; `CreateRouteBudget` is constructed floor-then-headroom with 31.18s named only as a sanity check and no ratio stated; `AssetsRouteBudget` is two manifest budgets plus the resolution's own bookkeeping; `writeTimeout` is derived from R1.
- **`registryRefused` not widened, no timeout reported as a refusal, no `ProbedAt` for a probe that did not answer.** Unchanged, and asserted by the tri-state case above.
- **`.planning/WINDOWS.md` untouched.** `git log -- .planning/WINDOWS.md` shows no commit from this plan; the residuals are below.
- **No `Flush`/`Unwrap`/`Hijack` on middleware `ResponseWriter` wrappers, no `http.ResponseController`.** `grep -rn 'ResponseController\|func.*Hijack\|func.*Unwrap()' internal/httpapi/middleware` returns nothing. Phase 5's decision was not spent here.

## Decisions Made

See `key-decisions` in the frontmatter. The two worth reading twice:

**`probeStatus` takes its class as a parameter.** It is called by the ISO probe and by the installer manifest resolution, two workloads with two derivations. A helper that chose one of them internally would be deciding which workload it was serving, which is the same category of mistake as one constant governing three.

**Clipping is data with a ratchet, not a doc comment.** `POST /api/v1/schematics` declares 150s of per-call budgets against a 120s ceiling. The route can afford *one* of its two JSON calls consuming its whole budget before the probe, not both; on the pathological case where both do, the probe is cut by the route and the record ends unprobed — the existing fail-safe outcome, on a path an operator only reaches through an upstream that is barely answering. Putting that in the table as a declared verdict plus a required rationale makes it a claim that can go stale and go red.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Bug] `scaleBudget` overflowed `int64` in the reproduction helper**

- **Found during:** Task 1 (the reproduction cases)
- **Issue:** `int64(150ms) * int64(90s) / int64(30s)` is 1.8e19 nanoseconds before the division and `int64` stops at 9.2e18, so the scaled probe budget came out negative and `New` refused it (`the probe timeout must be positive, got -14.891469ms`).
- **Fix:** Compute the ratio in `float64` with a comment giving the overflow as the reason. The ratios in play are small whole numbers, so the rounding is exact at every value these constants have held.
- **Files modified:** `internal/httpapi/handlers/schematics_test.go`
- **Verification:** All three reproduction cases pass and print their scaled budgets.
- **Committed in:** `c97b914`

**2. [Rule 3 — Blocking] `contextcheck` refused `requireDeadline(req.Context())` in `probeStatus`**

- **Found during:** Task 1 (`golangci-lint run`)
- **Issue:** `contextcheck` reads the `req.Context()` accessor as a context appearing from nowhere and reported "Non-inherited new context".
- **Fix:** Check `callCtx` instead. `http.NewRequestWithContext` stores exactly what it is given, so the two are the same value; the comment says so.
- **Files modified:** `internal/imagefactory/probe.go`
- **Verification:** `golangci-lint run` → 0 issues; the no-deadline refusal test still passes.
- **Committed in:** `c97b914`

**3. [Rule 1 — Bug] Two comments in `internal/imagefactory/installer.go` were falsified by this plan**

- **Found during:** Tasks 1 and 2
- **Issue:** The `Canceled`-versus-`DeadlineExceeded` reasoning in the re-question branch rested on two claims this plan makes false: "This client's own budget is `http.Client.Timeout`" and "nothing wraps the inbound request context in a deadline — there is no `context.WithTimeout` under `internal/httpapi`". A comment that argues from a premise the code no longer has is worse than no comment, because the next reader will trust it.
- **Fix:** Both rewritten. The budget is now `ManifestTimeout` applied to a *derived* context, which is what leaves `ctx.Err()` untouched here when only the per-call budget expired — the property the branch depends on. The route deadline is named as landed rather than as pending, and the distinction is recorded as load-bearing rather than prospective.
- **Files modified:** `internal/imagefactory/installer.go`
- **Verification:** `go test ./internal/imagefactory/... -race` green; the re-question tests that exercise this branch pass unchanged.
- **Committed in:** `c97b914`, `2e3aa5a`

**4. [Rule 2 — Missing critical] Test helpers were setting only the JSON budget**

- **Found during:** Task 1
- **Issue:** `newClient`, `newClientWithRetryInterval` and the handlers' `f.client(t)` set one timeout. With `ProbeTimeout` defaulting to 90s, a misrouted or deliberately silent image request in any existing test would have hung for a minute and a half instead of failing fast.
- **Fix:** All three name all three budgets. `testBudget = 5s` in the imagefactory package carries the reason.
- **Files modified:** `internal/imagefactory/tracer_test.go`, `internal/imagefactory/installer_test.go`, `internal/httpapi/handlers/schematics_test.go`
- **Verification:** Full suite runtime is unchanged; no test regressed.
- **Committed in:** `af8b537`

---

**Total deviations:** 4 auto-fixed (2 bugs, 1 blocking, 1 missing critical)
**Impact on plan:** No scope creep. Three of the four are consequences of the change the plan asked for — an overflow in the plan's own scaling instruction, a linter reading of the plan's own refusal site, and comments the plan's own change falsified. The fourth is a test-hygiene requirement the plan states in its `<action>`.

## Ledger entries to file

Plan 02-24 owns `.planning/WINDOWS.md` for this round. This plan touched none of it. Four residuals:

**1. Entry 8 ([WR-01], `internal/imagefactory/client.go:103`) — the reason it exists no longer applies.**
The entry says "`WithHTTPClient` silently discards the configured timeout." That was true while the budget lived on `http.Client.Timeout`: the option shallow-copied the caller's client, and the copy's zero `Timeout` overwrote whatever `WithTimeout` had set, silently and order-dependently. The budgets no longer live there. `WithHTTPClient` now clears the copy's `Timeout` explicitly and the three budgets are applied per call from the `Client`, so a caller's own timeout is neither honoured nor discarded — it is not the mechanism. `TestClientCarriesNoClientWideTimeout` asserts the zero duration for a default client *and* for one built through `WithHTTPClient` with a 7s timeout set. **Claim changed:** the discarding is gone, and the order-dependence with it. Supersede on that evidence.

**2. Entry 20 (`deviation`, `internal/imagefactory/installer.go`) — the composition claim changed; the serial walk did not.**
The entry says two things. The first — "a cold `resolveInstallerRepo` still walks two candidates serially at `DefaultTimeout=30s` each" — is now *half* wrong: the walk is still serial (that is plan 02-23's), but the candidates are bounded by `ManifestTimeout`, not by `DefaultTimeout`. The values coincide at 30s each; the constants do not. The second — "so `GET /schematics/{id}/assets` keeps a 2×30 = 60.000s worst case against `writeTimeout=60s`" — is **closed**: `AssetsRouteBudget = 65s` is the route's worst case now, `writeTimeout` is 130s, and R1 asserts `65 + 5 < 130`. The entry's closing sentence, "`cmd/holzkubed/budget_test.go` declares the route as known-over-budget", is stale: the row is `withinBudget` with an empty `deferredTo`, and the guard fails in both directions, so it could not have been left otherwise.

**3. Entry 48 (`deviation`, `internal/imagefactory/installer.go`) — its ceiling claim is closed; its warning is not.**
The entry says "the ceiling against `writeTimeout=60s` is still owned by `02-DECISION-probe-budget.md`". That ownership is discharged: the decision was ratified as Option 2 and this plan implements the budget half of it. What the entry warns against — reading a milder symptom as the timeout having been addressed — still applies verbatim to *this* plan, which is why the next residual is written the way it is.

**4. `AssetsRouteBudget` is sized for a walk plan 02-23 replaces, and 02-22 shipped alone does not deliver G-02-2's improvement.**
`AssetsRouteBudget = 2 × ManifestTimeout + 5s = 65s` because at this wave `resolveInstallerRepo` still asks its candidates one after the other. A ceiling of one manifest budget would cut the legacy candidate before it could answer, which is the silent fallback G-02-3 already cost this phase once. Plan 02-23 makes the walk concurrent and tightens the constant to `ManifestTimeout + 5s`; the clipping ratchet fires if it is tightened without the declared call list changing with it.

State the consequence in the terms the plan asks for: **02-22 shipped alone leaves the assets route bounded and inside the response budget, but it does not deliver G-02-2's improvement.** A cold resolution still costs a silent candidate's full budget before the second question is asked — 43.42s of the measured 43.42s success is still 30s of waiting followed by 13.4s of answer, and the double-silent case still spends both budgets. What changed is that the route now declares a ceiling and the ceiling fits inside `writeTimeout`; what did not change is how long an operator waits for an installer reference on a cold path. Anyone reading "both Factory rows are `withinBudget`" as "the assets route is finished" is making exactly the mistake entry 48 warns about.

## Issues Encountered

- **A `git checkout -- internal/imagefactory/probe.go` during the classJSON revert check discarded uncommitted work.** The file had not yet been committed when I used `git checkout` to restore it after deliberately breaking it, so the restore went back to the pre-plan version rather than to my edited one. Caught immediately by `go build` (`too many arguments in call to c.probeStatus`), reapplied in full, and the full suite re-run green. Recorded because the safe pattern for these deliberate-break checks is an in-place `sed` there-and-back, which is what every subsequent failure-direction check used.
- **A restoring `sed` matched two rows instead of one.** Undoing the R0 failure-direction check replaced `routeDeadline: 0,` on *every* matching line, which handed the store-only route a ceiling it cannot apply. The guard caught it on the next run with R0's reverse message — which is the assertion doing exactly its job, and is now recorded as a seventh verified failure direction.

## User Setup Required

None — no external service configuration. The one environment variable involved, `HOLZKUBE_MANAGER_FACTORY_LIVE=1`, is an opt-in test flag rather than a credential, and it was exported for the two measurement runs recorded above.

## Next Phase Readiness

- **Plan 02-23** reads `imagefactory.ManifestTimeout`, `handlers.AssetsRouteBudget` and the guard's declared call list. Making the candidate walk concurrent means changing the assets row's `calls` from two manifest entries to one and tightening `AssetsRouteBudget` to `imagefactory.ManifestTimeout + 5*time.Second` in the same commit — the clipping ratchet fires if only one of the two moves.
- **Plan 02-24** owns the ledger for this round. The four residuals above are written to be superseded on evidence rather than edited.
- **Threat register:** T-02-103 … T-02-106 are the ids this plan assigned. The next free id is **T-02-107**, which plan 02-23 takes.
- **No new threat surface** beyond the register: no route added, no dependency added, `go.mod` and `web/package.json` untouched.

## Self-Check: PASSED

- All 13 modified files present on disk (`git diff --stat 59988a2..HEAD` lists exactly them).
- All five task commits present: `af8b537`, `c97b914`, `7f4e3c3`, `2e3aa5a`, `48cad6c`.
- `go build ./...` clean, `go vet ./...` clean, `gofmt -l ./cmd ./internal` empty.
- `go test ./... -count=1 -race` green across every package.
- `golangci-lint run` → 0 issues.
- `npm --prefix web test` → 8 files, 126 tests passed (this plan changes no web source).
- `go test ./internal/imagefactory/ -count=1` green with the live env var unset, and the skip message still names what went unverified.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-09-03*
