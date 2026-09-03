---
phase: 02-transport-seam-talossim-image-factory
plan: 23
subsystem: api
tags: [go, imagefactory, concurrency, timeouts, react, abortsignal, drift-guard, talos]

requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "imagefactory.ManifestTimeout, handlers.CreateRouteBudget and handlers.AssetsRouteBudget exported, the per-class composition guard with its R0-R4 ratchets, answerAfter latency injection on both fakes"
provides:
  - "resolveInstallerRepo issues one request per candidate at once and decides in installerCandidates' declared order"
  - "handlers.AssetsRouteBudget tightened 65s -> 35s (one manifest budget plus five seconds)"
  - "The composition table's assets row declares one concurrent manifest call rather than two serial ones"
  - "manifestDelays: per-repository-name manifest latency on both test fakes"
  - "internal/httpapi/handlers/budget_drift_test.go — the Go-reads-TypeScript guard that the UI's stated waits equal the enforced route budgets"
  - "web/src/api.ts REQUEST_CEILING_MS and a fresh AbortSignal per fetch attempt, with its own abort branch"
  - "web/src/routes/images.tsx ASSETS_WAIT_SECONDS and CREATE_WAIT_SECONDS, rendered in both waiting states"
affects: [02-24, phase-03-provisioning, phase-05-deadline-policy]

actuals:
  tokens: 130918
  tasks: 3
  commits: 5

tech-stack:
  added: []
  patterns:
    - "Concurrent requests with a sequential decision: a fixed-length slice indexed by declared position, walked in order, never a shared append or a channel race"
    - "A per-repository-name latency knob beside the per-workload one, because a budget is a workload property and a resolution's shape is a name property"
    - "A per-attempt AbortSignal created at the fetch call rather than stored in a reused RequestInit"
    - "Go-reads-TypeScript drift guard in an external test package, reading an exported constant from the package under test"

key-files:
  created:
    - internal/httpapi/handlers/budget_drift_test.go
    - web/src/api.test.ts
  modified:
    - internal/imagefactory/installer.go
    - internal/imagefactory/installer_test.go
    - internal/imagefactory/fake_test.go
    - internal/httpapi/handlers/schematics.go
    - internal/httpapi/handlers/schematics_test.go
    - cmd/holzkube-managerd/budget_test.go
    - web/src/api.ts
    - web/src/routes/images.tsx
    - web/src/routes/images.test.tsx
    - docs/api-contract.md

key-decisions:
  - "The requests are concurrent and the decision is sequential. A fan-out that returns the first result to arrive would be a silent installer substitution nothing downstream can detect, so the answers land in a slice indexed by candidate position and are read in installerCandidates' declared order."
  - "The short-circuit is taken only where it is free: on the candidate at the current index answering 2xx, never on a later candidate while an earlier one is still outstanding."
  - "cancelFan() before wg.Wait() in one deferred function rather than as two defers, so the goroutines abandoned by a short-circuit are released rather than run to completion."
  - "AssetsRouteBudget tightened to one manifest budget in the same commit that removed the serial walk it was sized for; the clipping ratchet fires on the under-provisioned half-change and, by the arithmetic, tolerates the over-provisioned one."
  - "The browser ceiling is 150s, derived as above writeTimeout (130s) rather than tuned close to it, so the server's own problem+json always wins the race and only a server that never answers is cut."
  - "The abort gets its own error branch and never passes through toProblemError: there is no response to read, and an invented problem document would put words in the server's mouth."
  - "The stated waits are a ceiling and explicitly not the progress indicator PITFALLS:164 requires; that requirement stays open and is recorded as such."

patterns-established:
  - "Concurrent fan-out, sequential decision: preference order survives concurrency because the decision reads a position-indexed slice, not an arrival order"
  - "Drift guards read the UI from Go: three now in this repository, all in the same direction and for the same reason"
  - "A per-attempt ceiling attached at the call site when the RequestInit is deliberately reused"

requirements-completed: [FACT-02, FACT-03]

coverage:
  - id: D1
    description: "A cold installer resolution asks every candidate at the same time, so it costs the slowest single candidate rather than the sum of them all."
    requirement: FACT-03
    verification:
      - kind: unit
        ref: "internal/imagefactory/installer_test.go#TestInstallerImageAsksEveryCandidateAtOnce"
        status: pass
      - kind: manual_procedural
        ref: "serialised the fan-out in place (go func -> func); the test went red at 606.93ms against the 450ms bound; restored"
        status: pass
    human_judgment: false
  - id: D2
    description: "The declared candidate order still decides the winner: the platform-prefixed name keeps its preference and a faster legacy answer cannot displace it."
    requirement: FACT-03
    verification:
      - kind: unit
        ref: "internal/imagefactory/installer_test.go#TestInstallerImagePrefersTheDeclaredOrderOverTheFastestAnswer"
        status: pass
      - kind: manual_procedural
        ref: "replaced the ordered decision walk with a first-to-arrive walk; the test went red naming /installer/ against the wanted /metal-installer/; restored"
        status: pass
    human_judgment: false
  - id: D3
    description: "Every property G-02-3 and plan 02-12 established survives: the unresolved provenance, both fallback warnings, provisional caching and the bounded re-question."
    requirement: FACT-03
    verification:
      - kind: unit
        ref: "internal/imagefactory/installer_test.go — all 22 pre-existing tests pass with no edit to any of them"
        status: pass
      - kind: unit
        ref: "go test ./internal/imagefactory/ -run TestInstaller -count=20 -race"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestAssetsRouteResolvesPastASilentCandidateInOneBudget"
        status: pass
    human_judgment: false
  - id: D4
    description: "The composition table records one concurrent candidate budget rather than two serial ones, and the guard recomputes the route's worst case from that."
    requirement: FACT-03
    verification:
      - kind: unit
        ref: "cmd/holzkube-managerd/budget_test.go#TestRouteBudgetsComposeAgainstWriteTimeout/GET_/api/v1/schematics/{id}/assets"
        status: pass
      - kind: manual_procedural
        ref: "half-change driven both ways: two declared calls against the tightened constant goes red on R4 (60s sum vs 35s deadline); one declared call against the untightened constant stays green (30s vs 65s, uncut vs uncut) exactly as the plan states; restored"
        status: pass
    human_judgment: false
  - id: D5
    description: "The assets route measurably resolves past a silent candidate in about one manifest budget, at the HTTP route and not only inside the package."
    requirement: FACT-03
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestAssetsRouteResolvesPastASilentCandidateInOneBudget"
        status: pass
      - kind: manual_procedural
        ref: "serialised the fan-out; the route test went red at 1.059886208s against the 900ms bound; restored"
        status: pass
    human_judgment: false
  - id: D6
    description: "No request in the browser can pend forever: every fetch carries a fresh abort ceiling above writeTimeout, and a server that never answers ends as a stated failure that claims nothing about the server."
    requirement: FACT-03
    verification:
      - kind: unit
        ref: "web/src/api.test.ts#every request carries a ceiling"
        status: pass
    human_judgment: false
  - id: D7
    description: "Both waiting screens state the ceiling the server actually enforces, held equal to it by a Go drift guard."
    requirement: FACT-03
    verification:
      - kind: unit
        ref: "internal/httpapi/handlers/budget_drift_test.go#TestBudgetWaitsStatedInTheUIMatchTheRouteBudgets"
        status: pass
      - kind: unit
        ref: "web/src/routes/images.test.tsx#ImagesView — the two waits state their ceiling"
        status: pass
      - kind: manual_procedural
        ref: "ASSETS_WAIT_SECONDS 35->36 and CREATE_WAIT_SECONDS 120->119 each driven locally; the guard went red naming both values and the file to change; restored"
        status: pass
    human_judgment: false
  - id: D8
    description: "The contract states the assets route's server-enforced upstream ceiling and its value, and says it is a budget rather than a latency promise."
    requirement: FACT-03
    verification:
      - kind: manual_procedural
        ref: "docs/api-contract.md § GET /api/v1/schematics/{id}/assets — the new 'How long the route may take, and who enforces it' bullet; grep '35-second' returns one hit"
        status: pass
    human_judgment: false
  - id: D9
    description: "The elapsed-time progress indicator PITFALLS:164 requires is NOT built and is NOT claimed. Whether a stated ceiling is an acceptable interim answer for an operator waiting 31 seconds on a create is a judgment about the product, not something a test can assert."
    verification: []
    human_judgment: true
    rationale: "The claim is a negative one -- that this round did not meet a requirement it deliberately did not attempt. No assertion in this repository can establish that a human reading 'This may take up to 120 seconds' finds it sufficient; only a human can, and 02-DECISION-probe-budget.md records it as a risk Option 2 accepts."

duration: 30 min
completed: 2026-09-03
status: complete
---

# Phase 02 Plan 23: Concurrent Candidates, a Tightened Ceiling, and Two Screens That Say It Summary

**`resolveInstallerRepo` asks every candidate repository name at once and still takes the first 2xx in the declared order, `AssetsRouteBudget` falls from two manifest budgets to one in the same change, every browser request carries a fresh abort ceiling above `writeTimeout`, and the two screens where an operator waits state the number the server actually enforces — held equal to it by a Go test that reads the TypeScript.**

## Performance

- **Duration:** ~30 min
- **First task commit:** 2026-09-03T19:01:09Z (`ba81838`)
- **Last task commit:** 2026-09-03T19:19:29Z (`7cb626f`)
- **Tasks:** 3
- **Files modified:** 12 (2 created, 10 modified)

## Accomplishments

- **The cold resolution is one candidate wide.** `resolveInstallerRepo` issues one request per candidate up front, each on a context derived from the caller's, and then walks a fixed-length slice indexed by candidate position. Measured offline: two candidates at 300 ms each resolve in **305.96 ms**, against **606.93 ms** for the same test run against a serialised fan-out. At the HTTP route, a resolution past a candidate that never answers costs **603.57 ms** against a 600 ms scaled manifest budget, versus **1.0599 s** serialised.
- **The decision is still sequential, and there is a test whose only job is to say so.** `TestInstallerImagePrefersTheDeclaredOrderOverTheFastestAnswer` gives the legacy candidate zero latency and the preferred one 300 ms, both answering 2xx, and asserts the preferred name resolves. It passes for free against the serial walk — deliberately, and its comment says so — and it goes red against a first-to-arrive implementation. That is the only thing separating this change from a silent SecureBoot substitution, which `installer.go` refuses precisely because it is undetectable afterwards.
- **`AssetsRouteBudget` fell 65 s → 35 s in the same commit that removed the serial walk it was sized for.** The composition table's assets row now reads one concurrent manifest call: 30 s of declared calls against a 35 s deadline, R1 `40 < 130` ✓, R2 `35 ≥ 30` ✓, `withinBudget`, `uncut`.
- **Every request the browser makes carries a ceiling, and it is created fresh per attempt.** The signal is attached at the `fetch` call rather than stored in `init`, because `init` is deliberately built once and reused so the sudo replay is byte-identical to the refused request — and an `AbortSignal.timeout` that started counting before a password prompt would abort a request that had not begun.
- **Three drift guards now, all in the same direction.** `budget_drift_test.go` joins `TestWarningDetailsMatchTheUI` and the installer-name guard: Go reads TypeScript, because vitest is rooted at `web/` and could not read a Go constant even if it were allowed out.

## Task Commits

1. **Task 1 (TDD) — RED: the candidates are asked at once** — `ba81838` (test)
2. **Task 1 — GREEN: every candidate at once, the declared order still deciding** — `e27ff2c` (feat)
3. **Task 2 — the route's new worst case, where the arithmetic lives** — `b44c189` (feat)
4. **Task 3 (TDD) — RED: the drift guard between stated and enforced wait** — `d3c794b` (test)
5. **Task 3 — GREEN: a browser that cannot wait forever, on screens that say how long** — `7cb626f` (feat)

**Plan metadata:** see the `docs(02-23)` commit.

## The `must_haves.truths`, checked

| # | Truth | Holds | How I know |
|---|---|---|---|
| 1 | The candidates are asked at the same time; a cold resolution costs the slowest single candidate rather than the sum | yes | `TestInstallerImageAsksEveryCandidateAtOnce`: two 300 ms candidates at `installerLegacyVersion` (where the preferred name 404s and is genuinely walked past) resolve in **305.96 ms**, bound 450 ms. Serialising the fan-out in place makes it **606.93 ms** and red; restored. Both names are still asked exactly once, asserted, so the speed-up cannot have come from skipping one |
| 2 | The winner is still the first candidate in the declared order that answered 2xx, not the first to answer | yes | `TestInstallerImagePrefersTheDeclaredOrderOverTheFastestAnswer` gives the legacy candidate 0 ms and the preferred one 300 ms at a version where both answer 200, and asserts `metal-installer`. Driven red against a first-to-arrive decision walk (it returned `/installer/`); restored. Structurally: the answers land in `answers[i]`, indexed by position, and the loop is `for i, repo := range candidates` — there is no channel and no append |
| 3 | The G-02-3 provenance survives: a name reached past an unanswered candidate still carries `unresolved`, still warns on every answer, still caches provisionally | yes | **Every pre-existing test in `installer_test.go` passes with no edit to any of them** — `git diff` on that file adds two tests and one constant and changes no existing line. That includes the provisional-cache group, the re-question narrowing gate, the retention case, the promotion case and both fallback-warning cases. `-count=20 -race` green. And the new route-level test asserts the fallback warning end to end |
| 4 | The errors returned when nothing resolves are byte-identical to the serial walk's | yes | `git diff` on `resolveInstallerRepo`'s three terminal branches touches no format string and no argument order; the only changed characters in the two `fmt.Errorf` calls are `err` → `answer.err` and `status` → `answer.status`. The accumulation the decision walk applies is the serial loop's, in candidate order, so `unresolved`, `unanswered`, `refused` and `lastAttempt` end with the same values. `TestInstallerImageRefusesWhenNeitherAnswers`, `...TreatsAnAllBadRequestCandidateSetAsARefusal` and `...DoesNotCacheAResolutionWithNothingToFallBackOn` assert the message shapes and pass unmodified |
| 5 | The composition table's assets row records one concurrent candidate budget and the guard recomputes from it | yes | The subtest prints `resolveInstallerRepo: slowest concurrent candidate manifest GET [manifest 30s] = 30s sum, largest call 30s, route deadline 35s, slack 5s, writeTimeout 2m10s` and passes `withinBudget`/`uncut`. `upstreamCall.budget()` still reads `imagefactory.ManifestTimeout` rather than restating 30s |
| 6 | `AssetsRouteBudget` tightened in the same task; the ratchet catches the under-provisioned half-change; the over-provisioned one is tolerated | yes, exactly as stated | Both halves are in commit `b44c189`. **Ratcheted direction:** re-adding the second declared call against the 35 s constant gives `sum 60s > 35s`, so R4 computes `clipped` against a declared `uncut` and the guard goes red; restored. **Tolerated direction:** one declared call against the restored 65 s constant gives `sum 30s` vs `deadline 65s` → computed `uncut` against declared `uncut`, R1 `70 < 130` ✓, R2 `65 ≥ 30` ✓, and the row **passes**. Driven locally and confirmed green; restored. The route is left over-provisioned rather than under-provisioned, which is why it is tolerated, and I did not "fix" the guard to catch it |
| 7 | Every request carries a ceiling, and a request whose server never answers ends with a stated failure | yes | `web/src/api.test.ts`: `init.signal` is an `AbortSignal` on every call; the ceiling passed to `AbortSignal.timeout` is asserted `> 130_000`, i.e. above `writeTimeout`, as a relation rather than a value; a fetch that settles only on abort produces an `Error` matching `/did not answer within \d+ seconds/` whose message says "given up on" and "may or may not have been carried out" and which carries **no** `problem` property; and the sudo replay receives a second, distinct signal |
| 8 | The two screens say how long they may wait, and the number is the route budget the server enforces — held equal by a drift guard | yes | `TestBudgetWaitsStatedInTheUIMatchTheRouteBudgets` prints `ASSETS_WAIT_SECONDS = 35 s == handlers.AssetsRouteBudget (35s)` and `CREATE_WAIT_SECONDS = 120 s == handlers.CreateRouteBudget (2m0s)` on pass. Both off-by-one directions driven red locally and restored. `images.test.tsx` asserts each sentence renders while its request is in flight, is absent before it starts and gone after it settles, and that the wording contains no `takes about|estimated|roughly` |

### The prohibitions, checked

- **Which repository name wins is unchanged.** `installerCandidates` is untouched; `grep -c installerCandidates internal/imagefactory/installer.go` reports 6. The decision is a position-ordered walk and truth 2's test is written to fail against the alternative.
- **The `unresolved` provenance, the provisional caching and both fallback warnings are intact.** No pre-existing test in `installer_test.go` was edited. `installerFallbackWarning`, `storeInstallerRepo`, `requestionInstallerRepo` and `installerRepoEntry` are unchanged apart from `installerRepoRetryInterval`'s doc comment.
- **The PITFALLS progress indicator was not built.** No elapsed time, no sub-step, no expected duration. `images.tsx` carries a comment beside the two constants saying exactly that, naming `.planning/research/PITFALLS.md:164` and `02-DECISION-probe-budget.md`, and `images.test.tsx`'s new describe block says it cannot assert progress. See the ledger section.
- **The browser ceiling is not shorter than the server's route budget.** 150 s against `writeTimeout` 130 s and `CreateRouteBudget` 120 s; the relation is asserted in `api.test.ts` rather than the value.
- **`.planning/WINDOWS.md` untouched.** `git log ae4093e..HEAD -- .planning/WINDOWS.md` is empty. Residuals below.

## The measurements this round produced

| what | concurrent | serialised (driven locally, restored) | bound asserted |
|---|---|---|---|
| two 300 ms candidates, package level | **305.96 ms** | 606.93 ms | < 450 ms |
| assets route past a silent candidate, HTTP level (600 ms scaled manifest budget) | **603.57 ms** | 1.0599 s | < 900 ms |
| assets route against a registry that answers nothing (existing ceiling test) | 603.32 ms | — | < 700 ms scaled `AssetsRouteBudget` |

The route-level numbers are at the scaled budgets `ceilingJSONBudget = 600 ms` produces, not at production values. Only the three *client* budgets are scaled in those tests; `AssetsRouteBudget` is applied by the handler at its production 35 s, so the route deadline never binds there and the elapsed time measures the resolution itself.

## Files Created/Modified

- `internal/imagefactory/installer.go` — `resolveInstallerRepo` restructured into a fan-out plus an ordered decision walk, with the "requests are concurrent, the decision is sequential" section spelling out why the second half is load-bearing; `installerRepoRetryInterval`'s cold-path paragraph rewritten with 43.42 s and 60.000 s recorded as measured 2026-08-29 against the serial walk.
- `internal/imagefactory/installer_test.go` — `candidateLatency` with the two failure modes its value avoids, the timing test and the ordering test.
- `internal/imagefactory/fake_test.go` — `manifestDelays` and `answerManifestAfter`, per repository name, applied in `serveManifest` and abandoned on cancellation.
- `internal/httpapi/handlers/schematics.go` — `AssetsRouteBudget` = `imagefactory.ManifestTimeout + 5*time.Second`, with the doc comment recording that the tightening happened here rather than predicting it.
- `internal/httpapi/handlers/schematics_test.go` — the same per-repository latency knob on this package's fake, `TestAssetsRouteResolvesPastASilentCandidateInOneBudget`, and two comments this change falsified.
- `internal/httpapi/handlers/budget_drift_test.go` **(new)** — `package handlers_test`; reads `../../../web/src/routes/images.tsx`, extracts both declared waits anchored on their declarations, compares each against its exported route budget.
- `cmd/holzkube-managerd/budget_test.go` — the assets row's `calls` and `why`.
- `web/src/api.ts` — `REQUEST_CEILING_MS`, `fetchWithCeiling`, `isCeilingAbort`, `ceilingError`, and the abort branch at both `fetch` sites.
- `web/src/api.test.ts` **(new)** — five cases over the ceiling.
- `web/src/routes/images.tsx` — the two exported wait constants, their comment, and the two sentences.
- `web/src/routes/images.test.tsx` — a `hold` option on the stub and the two waiting-screen cases.
- `docs/api-contract.md` — the assets route's server-enforced upstream ceiling.

## Decisions Made

See `key-decisions` in the frontmatter. Three worth reading twice:

**The requests are concurrent; the decision is sequential.** These are two properties and the second is the design. `installerCandidates` declares a *preference*, not a set — for the SecureBoot pair the two names are two different images (G-02-13) — so the answers land in a slice indexed by candidate position and are read in order. A reader who later takes the first result off a channel has changed which image an operator installs, silently, in the one place nobody can check afterwards.

**`cancelFan()` before `wg.Wait()`, in one deferred function.** Two separate `defer`s run LIFO and would wait before cancelling, holding the caller for the slowest candidate it had already decided it did not need. The short-circuit is only free if the abandoned requests are actually released.

**The abort is not a problem response.** `toProblemError` reads a body; an abort has none. Inventing a problem document would attribute a statement to a server that made none, and the message therefore says what this side did — gave up after the ceiling — and explicitly that the request "may or may not have been carried out", because from here that is unknowable.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Bug] Two comments in `internal/httpapi/handlers/schematics_test.go` were falsified by the tightened constant**

- **Found during:** Task 2
- **Issue:** `ceilingJSONBudget`'s comment justified its value from "two manifest budgets against a ceiling of two plus five seconds — so the margin between the two is 8% of the ceiling at every scale". After the tightening it is one against one plus five, and the margin is 14%. `TestAssetsRouteAnswersInsideItsCeiling`'s failure message said "Both candidates are walked in series at this wave". A comment arguing from a premise the code no longer has is worse than no comment.
- **Fix:** Both rewritten. The `ceilingJSONBudget` comment now derives the 14% margin, records the 8% serial shape as history, and says to recheck if either constant moves without the other. The failure message names the concurrency.
- **Files modified:** `internal/httpapi/handlers/schematics_test.go`
- **Verification:** `go test ./internal/httpapi/... -count=1 -race` green; both ceiling tests still pass at the same elapsed times.
- **Committed in:** `b44c189`

**2. [Rule 3 — Blocking] The two UI wait constants had to be exported, and the drift guard's regex with them**

- **Found during:** Task 3
- **Issue:** The plan has `images.test.tsx` assert the rendered number. Transcribing `35` and `120` into the test would create a third copy of each value that nothing checks — the exact failure mode the drift guard exists to prevent, reintroduced in the test that was supposed to help. Exporting the constants and importing them into the test makes the test read the same declaration the Go guard reads. The guard's regex was anchored on `^\s*const`, which an `export const` does not match, so the guard went red the moment the export landed.
- **Fix:** Both constants exported; the regex accepts an optional `export`, and the "declare it as" hint in the failure message was updated to match.
- **Files modified:** `web/src/routes/images.tsx`, `web/src/routes/images.test.tsx`, `internal/httpapi/handlers/budget_drift_test.go`
- **Verification:** The guard passes and both off-by-one directions were re-driven red after the change. That the guard *caught* the export is itself evidence the anchor is doing its job rather than matching a number anywhere in the file.
- **Committed in:** `7cb626f`

**3. [Rule 1 — Bug] Fake timers cannot drive `AbortSignal.timeout`, so the never-settling test was rewritten**

- **Found during:** Task 3
- **Issue:** The first version of the never-settling-fetch case faked the clock and advanced past the 150 s ceiling. `AbortSignal.timeout` is a platform primitive with its own clock rather than a `setTimeout` vitest can advance, so both cases timed out at vitest's own 5 s limit — a test that failed for a reason unrelated to the code under test.
- **Fix:** The signal handed to `fetch` is one the test controls (`vi.spyOn(AbortSignal, 'timeout').mockReturnValue(controller.signal)`), and the test aborts it. Everything downstream is the production path: `fetch` rejects with the `TimeoutError` the platform raises, and the pipeline classifies and reports it. A separate case asserts the real ceiling value's *relation* to `writeTimeout`, so mocking the signal does not lose the number.
- **Files modified:** `web/src/api.test.ts`
- **Verification:** Five cases green in 9 ms rather than timing out at 10 s.
- **Committed in:** `7cb626f`

**4. [Rule 3 — Blocking] `tsc` refused the destructured mock call**

- **Found during:** Task 3 (`npm --prefix web run typecheck`)
- **Issue:** `const [ms] = ceiling.mock.calls[0]` — `noUncheckedIndexedAccess` makes the element possibly `undefined`, which is not iterable (TS2488).
- **Fix:** `const call = ceiling.mock.calls[0]; expect(call).toBeDefined(); expect(call?.[0]).toBeGreaterThan(130_000)`.
- **Files modified:** `web/src/api.test.ts`
- **Verification:** `typecheck` and `lint` both exit 0.
- **Committed in:** `7cb626f`

---

**Total deviations:** 4 auto-fixed (2 bugs, 2 blocking)
**Impact on plan:** No scope creep. Two are comments this plan's own change falsified, one is a toolchain fact about `AbortSignal.timeout` the plan could not have known, and one is a type error in a line the plan asked for. The one judgment call — exporting the two UI constants — is documented above and makes the test read the guarded declaration rather than a fourth copy of the number.

## Answers the plan asked for by name

**Did `requestionInstallerRepo` inherit the concurrency for free?** Structurally yes, materially no, and the reason is worth stating rather than glossing. It calls the same `resolveInstallerRepo` with a shorter candidate list, so it goes through the same fan-out and the same ordered decision walk with no change of its own. It gains nothing today because that list always holds **exactly one** name: an entry is only ever written from a *successful* resolution, and `unresolved` on a success holds the candidates tried before the winner — with two candidates the winner is at index 0 or 1, so `unresolved` has at most one element. A failed first resolution is not cached at all. So a re-question is a one-goroutine fan-out over a one-element slice, which is the serial walk by another name, at exactly the cost it had before. It would inherit a real speed-up if `installerCandidates` ever returned three or more names — which is the reason to leave it routed through one code path rather than special-casing it.

**Which pre-existing `installer_test.go` tests had to change, and why?** None. `git diff ae4093e..HEAD -- internal/imagefactory/installer_test.go` is purely additive: one constant and two tests. That is the strongest available evidence for truth 3, and it is why the plan asked.

## Ledger entries to file

Plan 02-24 owns `.planning/WINDOWS.md` for this round. This plan touched none of it. Four residuals:

**1. The PITFALLS progress indicator is still open, and stating a ceiling is not it.**
`.planning/research/PITFALLS.md:164` requires "Elapsed time, current sub-step, expected duration, and — critically — an explicitly disabled [action] button with the reason"; `:646` names "A spinner during the post-apply install window" as the anti-pattern, with "Named state, elapsed time, expected duration, current sub-step" as the better approach. This plan ships **one static sentence per waiting state** naming the server's ceiling. It has no elapsed time, no sub-step and no expected duration, and the number it shows is a maximum rather than a prediction. `02-DECISION-probe-budget.md` names this as a risk Option 2 accepts ("The create wait stays ~31s with a disabled button as its entire feedback surface, which `.planning/research/PITFALLS.md:164` already rules out"). **File this as an open window.** The comment beside `ASSETS_WAIT_SECONDS`/`CREATE_WAIT_SECONDS` in `images.tsx` says so in the code, so a reader who finds a number there cannot mistake it for the requirement having been met.

**2. Entry 20 (`deviation`, `internal/imagefactory/installer.go`) — its remaining half is now closed.**
02-22's ledger note said the entry was "half wrong": the walk was still serial, but the candidates were bounded by `ManifestTimeout` rather than `DefaultTimeout`. The serial half is closed here. `resolveInstallerRepo` asks every candidate at once, `AssetsRouteBudget` is one manifest budget plus five seconds, and the composition row declares one call. The entry's original composition claim — "so `GET /schematics/{id}/assets` keeps a 2×30 = 60.000s worst case" — has no surviving true reading. **Supersede on that evidence.**

**3. `installerRepoRetryInterval`'s two historical figures now live in a comment rather than in a ledger entry.**
43.42 s (a measured serial success: 30 s of silence plus a 13.4 s answer) and 60.000 s (both candidates silent) are recorded in `installer.go` as measured 2026-08-29 **against the serial walk**, so the five-minute interval's derivation stays legible after the thing it was derived against is gone. Worth a ledger note only so that a future reader who greps for those numbers finds the reason they are still written down rather than assuming they are stale.

**4. The over-provisioned half-change is not ratcheted, and that is deliberate.**
Relisting the assets row to one call while leaving `AssetsRouteBudget` at two manifest budgets computes `uncut` against a declared `uncut` and passes both R1 and R2, so the guard stays green on a route that is over-provisioned by 30 seconds. Driven locally and confirmed green. The plan names this asymmetry and tolerates it because the failure direction it leaves open is a ceiling that is too generous rather than one that cuts a candidate. **File as a known limitation of the guard, not a defect** — teaching R4 to catch it would require it to know how many candidates the handler actually issues, which is exactly the derived count the table's own comment argues against.

## Issues Encountered

- **The route-level assets test's ratchet is not the assertion the plan expected, and it took a measurement to find out which one it is.** The plan's `<action>` says "Assert the elapsed time explicitly; a status-and-body assertion passes just as well against the serial walk." That was true under the 65 s constant. Under the tightened 35 s one, the *production* route deadline would cut a serial walk before it finished — so I first wrote the test expecting the reference-and-warning assertion to be the thing that fires. Driving it red showed the elapsed assertion firing instead, because only the client budgets are scaled in these tests and `AssetsRouteBudget` is applied at its production value, so the route deadline never binds there at all. The test asserts both and the comment now says which is which and why. Recorded because the wrong version of that comment would have sent the next reader looking for a cut that does not happen.
- **The drift guard went red the moment the two UI constants were exported.** Expected in hindsight — the regex was anchored on `^\s*const` — and it is the anchor doing its job. Fixed by accepting an optional `export` rather than by loosening the anchor.
- **No `git checkout`, `git clean` or `git stash` was used for any of the eight deliberate-break checks.** Each one copied the file to the scratchpad, edited in place, ran the test, and copied back — the pattern 02-22's own issues section recommends after a `git checkout` there discarded uncommitted work.

## User Setup Required

None — no external service configuration, no new environment variable, no new dependency. `go.mod` and `web/package.json` are untouched; `AbortSignal.timeout` is a platform API available in both vitest projects and in every browser this product targets.

## Next Phase Readiness

- **Plan 02-24** owns `.planning/WINDOWS.md` for this round and has four residuals above, plus 02-22's four. The first of mine — the PITFALLS progress indicator — is a *new* open window rather than a supersession.
- **Threat register:** T-02-107, T-02-108 and T-02-109 are this plan's. T-02-107 (a first-to-answer race substituting the legacy name) is mitigated by the ordered decision walk and the test written to fail against the alternative. T-02-108 (per-request goroutine fan-out) is accepted: the fan-out is bounded by `installerCandidates`' two names and every candidate is issued on a context derived from the caller's, so the same volume leaves as before, compressed in time. T-02-109 (an unbounded browser request) is mitigated by `REQUEST_CEILING_MS`. `T-02-SC` holds: no Go module and no npm package added. **The next free id is T-02-110**, which plan 02-24 takes.
- **G-02-2 is closed on both halves.** 02-22 made the assets route bounded; this made it fast and gave the operator a stated ceiling on both waiting screens. What remains open from `02-DECISION-probe-budget.md` is G-02-9 (a probe that times out is still permanent) and the progress indicator above — neither is this plan's.

## Self-Check: PASSED

- Both created files present on disk: `internal/httpapi/handlers/budget_drift_test.go`, `web/src/api.test.ts`.
- All five task commits present: `ba81838`, `e27ff2c`, `b44c189`, `d3c794b`, `7cb626f`.
- `go build ./...` clean, `go vet ./...` clean, `gofmt -l ./cmd ./internal` empty.
- `go test ./... -count=1 -race` green across every package, including `./internal/httpapi` and `./cmd/...`.
- `go test ./internal/imagefactory/ -run TestInstaller -count=20 -race` green.
- `task lint:go` (`golangci-lint run`) → 0 issues, exit 0.
- `npm --prefix web test` → 9 files, 133 tests passed across both vitest projects (8 files / 126 tests before this plan).
- `npm --prefix web run typecheck` and `npm --prefix web run lint` exit 0.
- No stubs, no `TODO`/`FIXME`, no skipped tests introduced; every `<verify>` command in the plan was run.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-09-03*
