---
phase: 02-transport-seam-talossim-image-factory
plan: 18
subsystem: api
tags: [rfc9457, problem-json, urn, taxonomy, contract, agpl]

requires:
  - phase: 01-foundation-skeleton
    provides: the RFC 9457 problem taxonomy, its thirteen type constants and docs/api-contract.md
  - phase: 02-transport-seam-talossim-image-factory
    provides: the upstream type and its four reserved codes (02-02), the schematics handlers whose tests already composed from ProblemBaseURI (02-06)
provides:
  - "The problem-type taxonomy re-rooted at urn:holzkube-manager:problem:, deployment-independent and not configurable"
  - "Thirteen type constants derived from the single base as constant expressions, so a re-rooting cannot land partially"
  - "A source-level closure test that fails when a fourteenth type constant is declared"
  - "A web test proving presentationFor is a function of code alone, which is the premise that made the re-rooting free"
  - "One shared web fixture module holding the base, replacing eight literals across three test files"
  - "A contract paragraph stating that type URIs are identifiers, are not dereferenceable, and why a per-deployment base was rejected"
affects: [the pending repository-wide holzkube to holzkube-manager rename, any future problem-type addition, third-party API clients]

actuals:
  tokens: 5720
  tasks: 3
  commits: 4

tech-stack:
  added: []
  patterns:
    - "Constant-expression derivation for contract value families: one base, N suffixes, no repeated literal"
    - "Source-level closure tests: go/ast parsing of the declaring file, so 'closed' is asserted about the source rather than about a table a new constant can bypass"
    - "One shared fixture module per language for a cross-seam contract string the other language owns"

key-files:
  created:
    - web/src/test/problem-fixtures.ts
  modified:
    - internal/httpapi/problem.go
    - internal/httpapi/problem_test.go
    - internal/httpapi/endtoend_test.go
    - internal/httpapi/handlers/account_test.go
    - internal/httpapi/middleware/audit_test.go
    - web/src/lib/problem.test.ts
    - web/src/components/SudoDialog.test.tsx
    - web/src/routes/images.test.tsx
    - docs/api-contract.md

key-decisions:
  - "The taxonomy is re-rooted at urn:holzkube-manager:problem: and stays closed, stable and identical in every deployment; the per-install base considered in G-02-23 remains rejected and is now argued against in both problem.go and docs/api-contract.md"
  - "The thirteen types are constant expressions over ProblemBaseURI rather than thirteen literals, because the failure mode of a re-rooting is landing on twelve of thirteen"
  - "Closure is asserted by parsing problem.go with go/ast, not by enumerating a table: a table-only test would pass while a fourteenth type was minted beside it"
  - "audit_test.go keeps a literal problem body: it is the internal test package of middleware and httpapi imports middleware, so the constant is unreachable there — and the bytes stand in for an upstream writer whose base the middleware never reads"
  - "The Go fixtures inside internal/httpapi/... were moved in the tracer commit rather than in task 2, because a commit that re-roots the base while leaving same-package fixtures on the old one is a red package"
  - "The code-first-segment fixture convention in the web tests is left unchanged and commented, not refactored: it produces types the taxonomy does not contain, which is itself evidence that nothing reads the field"

patterns-established:
  - "Contract value families derive from one constant; tests pin the base once and the suffixes separately, so re-rooting touches two lines"
  - "A decision recorded in a doc comment carries the rejected alternative, not only the chosen one"

requirements-completed: [FOUND-11, FACT-02]

coverage:
  - id: D1
    description: "Every problem type holzkube emits is a URN under one base, and the thirteen constants derive from that base rather than repeating it"
    requirement: FOUND-11
    verification:
      - kind: unit
        ref: "internal/httpapi/problem_test.go#TestProblemTaxonomyIsRootedAtTheURN"
        status: pass
      - kind: unit
        ref: "internal/httpapi/problem_test.go#TestProblemTaxonomy"
        status: pass
      - kind: unit
        ref: "internal/httpapi/handlers/schematics_test.go (composed ProblemBaseURI+suffix expectations, unchanged and still green)"
        status: pass
    human_judgment: false
  - id: D2
    description: "The taxonomy is closed: a fourteenth type constant fails a test rather than landing quietly"
    requirement: FOUND-11
    verification:
      - kind: unit
        ref: "internal/httpapi/problem_test.go#TestProblemTaxonomyIsClosed"
        status: pass
      - kind: other
        ref: "manual falsification: TypeFourteenth added locally to problem.go -> test red ('problem.go declares TypeFourteenth, which is not in the closed taxonomy'); constant removed, test green"
        status: pass
    human_judgment: false
  - id: D3
    description: "Nothing branches on the type URI: the presentation is a function of code alone"
    requirement: FOUND-11
    verification:
      - kind: unit
        ref: "web/src/lib/problem.test.ts#ignores the type entirely: the presentation is a function of code alone"
        status: pass
    human_judgment: false
  - id: D4
    description: "No test or source file anywhere spells the retired base, and the new base is written once per language"
    requirement: FACT-02
    verification:
      - kind: integration
        ref: "go test ./... -count=1 -race"
        status: pass
      - kind: integration
        ref: "npm --prefix web test (8 files, 119 tests)"
        status: pass
      - kind: other
        ref: "grep -rn 'holzkube.dev/problems' --include='*.go' --include='*.ts' --include='*.tsx' . -> no matches"
        status: pass
      - kind: other
        ref: "grep -rIn 'holzkube-manager' (repo, excluding .git/node_modules/dist/.planning) -> 4 hits, all taxonomy/fixture"
        status: pass
    human_judgment: false
  - id: D5
    description: "The contract states that a type URI is an identifier, is not dereferenceable and is deployment-independent, and records the rejected per-deployment alternative"
    requirement: FOUND-11
    verification:
      - kind: other
        ref: "grep -c 'holzkube.dev' docs/api-contract.md -> 0; grep -c 'urn:holzkube-manager:problem:' docs/api-contract.md -> 2"
        status: pass
    human_judgment: true
    rationale: "The greps prove the paragraph exists and the base moved; whether the prose actually says the decision well enough that the next reader finds the decision rather than relitigating the idea is a judgment about wording, not something a command asserts."

duration: 23 min
completed: 2026-08-30
status: complete
---

# Phase 02 Plan 18: Re-root the problem-type taxonomy at a URN Summary

**The RFC 9457 taxonomy moves from `https://holzkube.dev/problems/` to `urn:holzkube-manager:problem:` — thirteen types derived from one base, a source-level closure test, a web test proving nothing reads the field, and a contract that finally says what a type URI is for.**

## Performance

- **Duration:** 23 min
- **Started:** 2026-08-30T08:09:00Z
- **Completed:** 2026-08-30T08:32:00Z
- **Tasks:** 3
- **Files modified:** 9 modified, 1 created

## Accomplishments

- `ProblemBaseURI` is now `urn:holzkube-manager:problem:`, and each of the thirteen `Type*` constants is the constant expression `ProblemBaseURI + "<suffix>"`. Every suffix is unchanged, so no `code`, status, title or body shape moved — this is a re-rooting of one field and nothing else.
- The doc comment carries the decision rather than the old rationale: why a URN and not a URL, that the base is deployment-independent and deliberately not configurable, the rejected per-install alternative in one sentence, and that the namespace is not IANA-registered.
- `TestProblemTaxonomyIsClosed` parses `problem.go` with `go/ast` and asserts three things about the source: the declared `Type*` set is exactly the thirteen, each is declared as `ProblemBaseURI + literal`, and each literal is the expected suffix. Falsified by hand — adding `TypeFourteenth` turns it red with a message naming the constant.
- `web/src/lib/problem.test.ts` now proves the premise the whole decision rests on: for every code in the taxonomy, `presentationFor` returns the same presentation whether `type` is empty, `about:blank`, a foreign URL or plain nonsense. A comment says what a failure of that test would mean.
- Every hand-written fixture moved. Go tests compose from `httpapi.ProblemBaseURI`; the web suite gained one shared fixture module and dropped eight literals across three files.
- `docs/api-contract.md` gains the section it never had: **What a `type` is, and what it is not**.

## Task Commits

1. **Task 1 (tracer, TDD RED): closure and rooting tests** — `01c59d9` (test)
2. **Task 1 (tracer, TDD GREEN): re-root the taxonomy** — `f1fff00` (feat)
3. **Task 2: move the hand-written fixtures** — `cf89ce9` (test)
4. **Task 3: say what a problem type is** — `0b09503` (docs)

**Plan metadata:** see the `docs(02-18)` commit that carries this SUMMARY.

## Files Created/Modified

- `internal/httpapi/problem.go` — base re-rooted; thirteen types derived; doc comment carries the decision and the rejected alternative
- `internal/httpapi/problem_test.go` — `typeTaxonomy` table, `TestProblemTaxonomyIsRootedAtTheURN`, `TestProblemTaxonomyIsClosed`; existing prefix assertion reworded away from "absolute URI" phrasing that implied http
- `internal/httpapi/endtoend_test.go` — the prefix check in `decodeProblem` and two exact comparisons now compose from `httpapi.ProblemBaseURI`
- `internal/httpapi/handlers/account_test.go` — the sudo-required comparison composes from the base
- `internal/httpapi/middleware/audit_test.go` — fixture body re-rooted, kept literal, with a comment stating why
- `web/src/test/problem-fixtures.ts` (new) — `PROBLEM_BASE_URI` and `problemType(suffix)`; the single place the base is spelled on the web side
- `web/src/lib/problem.test.ts` — uses the shared base; adds the type-independence test
- `web/src/components/SudoDialog.test.tsx` — uses the shared base; comments the fixture convention
- `web/src/routes/images.test.tsx` — five stubbed problem bodies compose via `problemType`; code tokens untouched
- `docs/api-contract.md` — taxonomy rooting sentence and example body re-rooted; new "What a `type` is, and what it is not" section

## Decisions Made

- **Derivation over repetition.** Go constant expressions let each type be `base + suffix`. This is the mechanism that makes a partial re-rooting inexpressible, which is the actual risk the plan named.
- **Closure asserted against the source.** Go constants cannot be enumerated at runtime, so the closure test parses `problem.go`. A table-only test would have passed while a fourteenth constant was minted beside it.
- **`audit_test.go` stays literal, and says so.** It lives in package `middleware` (the *internal* test package), and `httpapi` imports `middleware`, so `httpapi.ProblemBaseURI` is not reachable from there at all. That constraint happens to agree with the test's meaning: the bytes stand in for whatever an upstream handler wrote, and the claim under test is that the audit recorder reads `code` and never `type`.
- **Fixture convention left alone.** `problem.test.ts` and `SudoDialog.test.tsx` build a type from the code's first segment, producing types (`notfound`, `ratelimit`, `store`, `sudo` …) that the taxonomy does not contain. Changing it would mix a fixture refactor into a re-rooting; it is now commented as harmless *and* as evidence that nothing reads the field.
- **The base pinned in one test.** `TestProblemTaxonomyIsRootedAtTheURN` asserts the literal value, so a future re-rooting is a deliberate two-place edit rather than a one-character change nobody notices.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Go fixtures inside `internal/httpapi/...` moved in the tracer commit rather than in task 2**

- **Found during:** Task 1 (tracer GREEN)
- **Issue:** Task 1's acceptance requires `go test ./internal/httpapi/... -count=1 -race` green, but three fixture sites in `endtoend_test.go` (via `decodeProblem`, which also serves `auditapi_test.go`) and one in `handlers/account_test.go` hardcode the old base and live in that same package tree. Re-rooting the base without them leaves the package red at the tracer commit — and the tracer gate exists precisely to refuse to expand onto a red foundation.
- **Fix:** Moved those two files' expectations to `httpapi.ProblemBaseURI + suffix` inside the `feat(02-18)` commit. Task 2 then covered what remained: `middleware/audit_test.go`, the three web test files, the shared web fixture module, and the repository-wide rename guard.
- **Files modified:** `internal/httpapi/endtoend_test.go`, `internal/httpapi/handlers/account_test.go`
- **Verification:** `go test ./internal/httpapi/... -count=1 -race` green at `f1fff00`; `go test ./... -count=1 -race` green at `cf89ce9` and at HEAD
- **Committed in:** `f1fff00`

**2. [Rule 3 - Blocking] Biome import ordering after the new fixture import**

- **Found during:** Task 2
- **Issue:** `npm --prefix web run lint` failed on import order in `problem.test.ts` after adding the `@/test/problem-fixtures` import.
- **Fix:** Ran the repo's own `biome check --write` over `web/src`; one file reordered.
- **Files modified:** `web/src/lib/problem.test.ts`
- **Verification:** `npm --prefix web run lint` → no findings; `npm --prefix web run typecheck` → clean
- **Committed in:** `cf89ce9`

**3. [Documented, not a change] Two plan line references were stale**

- **Found during:** Task 2
- **Issue:** The plan cites `images.test.tsx` lines 197, 509, 584, **1056, 1064**; the last two live at 1224 and 1232 (`NOT_FOUND_PROBLEM`, `STORE_FAILURE_PROBLEM`). All five sites were found and moved.
- **Fix:** None needed — noted so a reader of the plan is not left hunting.

---

**Total deviations:** 2 auto-fixed (both Rule 3 — blocking), 1 documentation note
**Impact on plan:** No scope change. Deviation 1 redistributes work between tasks 1 and 2 to keep every commit green; the set of files touched is exactly the plan's list.

## Prohibitions Honoured

- **Base not configurable.** No flag, environment variable, build tag or config key reads it. `ProblemBaseURI` is a plain `const`; both it and the contract argue explicitly against a per-install base.
- **No stray `holzkube-manager`.** Repository-wide (`grep -rIn 'holzkube-manager'`, excluding `.git`, `node_modules`, `dist`, `.task`, `.planning`) there are exactly **four** hits, all of them the taxonomy or a fixture of it:
  | File | Line | What it is |
  |---|---|---|
  | `internal/httpapi/problem.go` | 37 | the base constant — the one authority |
  | `internal/httpapi/problem_test.go` | 336 | the test that pins the base literal |
  | `internal/httpapi/middleware/audit_test.go` | 126 | the deliberately-literal fixture body |
  | `web/src/test/problem-fixtures.ts` | 15 | the web side's single copy of the base |

  `docs/api-contract.md` adds two documentation occurrences (the rooting sentence and the example body). `.planning/` carries the plan, the UAT gap and the roadmap line, which are documentation of the decision.
- **No `code`, status, title or body-shape change.** The diff touches only `type` values and the prose around them; `problem_test.go`'s status/code assertions are unchanged and green.
- **Untouched:** `internal/imagefactory/`, `internal/httpapi/handlers/schematics.go`, `internal/httpapi/handlers/schematics_test.go` (verified byte-identical since `cc3cbd0`; its two `ProblemBaseURI + suffix` expectations moved with the base and pass unedited, which is the property the plan asked to preserve deliberately), `web/src/routes/images.tsx`, `.planning/WINDOWS.md`.
- **No dependency added.** `go.mod`, `go.sum`, `web/package.json`, `web/package-lock.json` unchanged (T-02-SC).

## Verification

| Check | Result |
|---|---|
| `go test ./... -count=1 -race` | PASS (no FAIL, no panic) |
| `npm --prefix web test` | PASS (8 files, 119 tests) |
| `~/go/bin/golangci-lint run ./...` | PASS (0 issues) |
| `npm --prefix web run lint` | PASS |
| `npm --prefix web run typecheck` | PASS |
| `gofmt -l ./internal` | PASS (empty) |
| `grep -c 'holzkube.dev' internal/httpapi/problem.go` | 0 |
| `grep -c 'urn:holzkube-manager:problem:' internal/httpapi/problem.go` | 1 |
| `grep -c 'holzkube.dev' docs/api-contract.md` | 0 |
| `grep -c 'urn:holzkube-manager:problem:' docs/api-contract.md` | 2 |
| `grep -rn 'holzkube.dev/problems' --include='*.go' --include='*.ts' --include='*.tsx' .` | no matches |
| fourteenth-type falsification | RED as designed, then removed |

Gates were run directly (`go test`, `~/go/bin/golangci-lint`, `npm --prefix web …`) rather than through `Taskfile.yml`, which plan 02-17 rewrote in the previous wave.

## Ledger entries to file

For plan 02-21 (single writer of `.planning/WINDOWS.md`):

1. **Wire-format change to a field the contract calls stable.** Every problem `type` changed from `https://holzkube.dev/problems/<suffix>` to `urn:holzkube-manager:problem:<suffix>` (kind: `deviation`, phase 02, file `internal/httpapi/problem.go`). Any client written against the previous base **that matches on `type` rather than on `code` breaks**. The project has no external clients today, which is why this is a note and not a migration — but the note is the difference between having decided that and not having noticed it. Matches T-02-90 (Repudiation, disposition `accept`).
2. **One fixture will not follow the next re-rooting automatically.** `internal/httpapi/middleware/audit_test.go:126` spells the base literally, by necessity (import cycle) and by intent (it stands in for an upstream writer). A future re-rooting must edit it by hand; nothing fails first if it is forgotten, because the middleware never reads the field (kind: `deviation`, phase 02).
3. **The URN namespace identifier is unregistered with IANA.** Documented in both `problem.go` and `docs/api-contract.md` as acceptable — RFC 9457 asks for a URI, not a registered namespace — and recorded so the decision is visible rather than assumed (kind: `deviation`, phase 02, file `docs/api-contract.md`).

## Known Stubs

None. No placeholder values, skipped tests or unrun `<verify>` commands were introduced; every task's `<verify>` was executed and is recorded above.

## Threat Flags

None. The change removes surface rather than adding it: no new endpoint, auth path, file access or schema. T-02-88 (a type URI that looks like a resolvable page on a domain the operator does not control) and T-02-89 (a partial re-rooting) are both mitigated as planned — by the URN plus the contract paragraph, and by constant-expression derivation plus the closure test respectively.

## Issues Encountered

- The `handlers` package test run takes ~175 s (argon2id calibration), so full-suite runs were dispatched in the background rather than inline. No functional impact.
- An unrelated `docs(02-17)` commit (`e418097`, `.planning/` only) landed on `main` between this plan's RED and GREEN commits. History stayed linear and all four `02-18` commits are reachable from `main`; noted only because it makes the plan's commits non-contiguous in `git log`.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- G-02-23 is closed on all three `missing` bullets: re-rooted, closed and deployment-independent, and the contract now states that type URIs are identifiers rather than addresses.
- The pending repository-wide `holzkube` → `holzkube-manager` rename has **exactly one** string to protect and four occurrences of it, all listed above. No stray occurrence exists that its PROTECTED list would skip and leave stale.
- Plans 02-19 and 02-20 are unaffected: `internal/imagefactory/`, `schematics.go`, `schematics_test.go` and `web/src/routes/images.tsx` were not touched, and the next free threat id remains T-02-91.

## Self-Check: PASSED

- All four created/modified key files exist on disk.
- All four task commits (`01c59d9`, `f1fff00`, `cf89ce9`, `0b09503`) are reachable from `main`.
- Every task's `<acceptance_criteria>` re-run and green (table above); the closure test's falsification was performed and reverted.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-30*
