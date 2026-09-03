---
phase: 02-transport-seam-talossim-image-factory
plan: 09
subsystem: api
tags: [go, image-factory, secureboot, oci-registry, talos, http-taxonomy]

requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "internal/imagefactory: registry-proven installer resolution, ProbeBuildable, the assets route and its contract (plans 02-04, 02-06)"
provides:
  - "SecureBoot reaches the installer repository name: installerCandidates asks only about SecureBoot names when the flag is set, and the cache key carries the selection"
  - "A SecureBoot request refuses with no installer reference rather than substituting the ordinary installer, argued in the code"
  - "registryRefused: one statement of what a registry status means, called from both ProbeBuildable and resolveInstallerRepo"
  - "A shared status table asserting the two classification sites agree answer by answer"
  - "The live four-name installer matrix at v1.12.0 and v1.13.9, recorded as an observation in live_test.go"
  - "docs/api-contract.md states the SecureBoot installer selection and the 400/404-versus-everything-else taxonomy"
affects: [upgrade path, phase 6 jobs engine, probe-budget decision cluster A]

actuals:
  tokens: 12732
  tasks: 3
  commits: 5

tech-stack:
  added: []
  patterns:
    - "One predicate owns a classification the package makes in more than one place; call sites reference it rather than restating it"
    - "A live drift test logs a transport failure as 'not observed' and never as a verdict — the same refused/did-not-answer distinction the production code draws"
    - "Branch-coverage fixtures and registry observations live in separate tables, each labelled as what it is"

key-files:
  created:
    - internal/imagefactory/probe_test.go
  modified:
    - internal/imagefactory/installer.go
    - internal/imagefactory/probe.go
    - internal/imagefactory/urls.go
    - internal/imagefactory/client.go
    - internal/imagefactory/fake_test.go
    - internal/imagefactory/installer_test.go
    - internal/imagefactory/live_test.go
    - internal/httpapi/handlers/schematics_test.go
    - docs/api-contract.md

key-decisions:
  - "A SecureBoot request whose SecureBoot names do not resolve is refused with no installer reference; the ordinary installer is never substituted"
  - "registryRefused is true for 400 and 404 only; 401, 403, 429 and every 5xx are the upstream declining to answer and stay retryable"
  - "The live installer matrix is recorded in live_test.go and here, never transcribed into fake_test.go's branch-coverage fixture"
  - "DefaultTimeout's doc comment was corrected and its value left alone; every budget in this package stays with 02-DECISION-probe-budget.md"

patterns-established:
  - "Shared-taxonomy table: one table declared once, driven through every call site that classifies, so a failure reads as the sites disagreeing"
  - "Tri-state live expectation (unknown / answers / silent): an unchecked cell logs and asserts nothing, which is how it stops being unchecked"

requirements-completed: [FACT-02, FACT-03]

coverage:
  - id: D1
    description: "A SecureBoot asset request resolves to a SecureBoot installer reference, from the query parameter through the cache key to the registry manifest"
    requirement: "FACT-03"
    verification:
      - kind: unit
        ref: "internal/imagefactory/installer_test.go#TestInstallerImageResolvesTheSecureBootName"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/installer_test.go#TestInstallerImageFallsBackToTheLegacySecureBootName"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestAssetsSecureBootSuffixesTheURLs"
        status: pass
      - kind: e2e
        ref: "internal/imagefactory/live_test.go#TestLiveFactory/SecureBoot_resolves_to_a_different_installer_than_the_ordinary_request (HOLZKUBE_FACTORY_LIVE=1, run 2026-08-29)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Two asset requests differing only in SecureBoot resolve to two different references and never share a cache entry"
    requirement: "FACT-03"
    verification:
      - kind: unit
        ref: "internal/imagefactory/installer_test.go#TestInstallerImageCachesSecureBootSeparately"
        status: pass
    human_judgment: false
  - id: D3
    description: "A SecureBoot request at a version where neither SecureBoot name answers refuses with no installer reference rather than substituting a non-SecureBoot one"
    verification:
      - kind: unit
        ref: "internal/imagefactory/installer_test.go#TestInstallerImageRefusesRatherThanSubstitutingTheOrdinaryInstaller"
        status: pass
    human_judgment: true
    rationale: "The offline test proves the branch against a constructed fixture. Whether any shipped Talos version actually behaves that way is a claim about the registry, and the live matrix only covers v1.12.0 and v1.13.9 — see the acceptance under Issues Encountered. A human takes the merge-time acceptance of the new 502 path (T-02-53)."
  - id: D4
    description: "400 and 404 are the registry refusing; 401, 403, 429 and every other non-2xx are the registry not answering — identically in ProbeBuildable and in resolveInstallerRepo"
    requirement: "FACT-02"
    verification:
      - kind: unit
        ref: "internal/imagefactory/probe_test.go#TestProbeBuildableClassifiesEveryRegistryAnswer"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/installer_test.go#TestResolveInstallerRepoClassifiesEveryRegistryAnswer"
        status: pass
    human_judgment: false
  - id: D5
    description: "A 429 from a throttling Factory no longer marks a schematic as not buildable"
    requirement: "FACT-02"
    verification:
      - kind: unit
        ref: "internal/imagefactory/probe_test.go#TestProbeBuildableKeepsAThrottledFactoryRetryable"
        status: pass
    human_judgment: false
  - id: D6
    description: "resolveInstallerRepo can reach ErrSchematicNotBuildable again: an all-400 candidate set is a refusal"
    requirement: "FACT-02"
    verification:
      - kind: unit
        ref: "internal/imagefactory/installer_test.go#TestInstallerImageTreatsAnAllBadRequestCandidateSetAsARefusal"
        status: pass
    human_judgment: false
  - id: D7
    description: "Which of the four repository names answer, per version, is a recorded observation rather than an assumption"
    verification:
      - kind: e2e
        ref: "internal/imagefactory/live_test.go#TestLiveFactory/the_installer-name_matrix_is_what_this_file_records (HOLZKUBE_FACTORY_LIVE=1, run 2026-08-29)"
        status: pass
    human_judgment: false
  - id: D8
    description: "DefaultTimeout's doc comment names the ISO HEAD and the installer manifest GETs it governs, with the value unchanged"
    verification:
      - kind: other
        ref: "git diff internal/imagefactory/client.go — comment lines only; grep -c 'DefaultTimeout = 30 \\* time.Second' == 1"
        status: pass
    human_judgment: false
  - id: D9
    description: "docs/api-contract.md states the SecureBoot installer selection and the status taxonomy"
    verification:
      - kind: manual_procedural
        ref: "grep -c secureboot docs/api-contract.md == 8 (was 2); the status table names 400/404 as upstream.factory-rejected and 429 as upstream.factory-unavailable"
        status: pass
    human_judgment: true
    rationale: "Prose accuracy against the implemented behaviour is a reading, not an assertion. No test binds the contract text to the code."

duration: 20 min
completed: 2026-08-29
status: complete
---

# Phase 02 Plan 09: SecureBoot Installer Selection and One Registry Taxonomy Summary

**SecureBoot now selects the installer repository — `metal-installer-secureboot` before `installer-secureboot`, with the flag in the cache key — and one `registryRefused` predicate decides for both `ProbeBuildable` and `resolveInstallerRepo` what a 400, a 404 or a 429 means.**

## Performance

- **Duration:** 20 min
- **Started:** 2026-08-29T16:21:42Z
- **Completed:** 2026-08-29T16:42:22Z
- **Tasks:** 3
- **Files modified:** 10 (1 created)

## Accomplishments

- **G-02-4 closed.** `installerCandidates` owns the ordered candidate list and appends the `-secureboot` suffix upstream's `getRequestedImage` parses as the switch. The installer cache key gained the selection, formatted (`secureboot=%t`) rather than conditionally concatenated so two requests cannot collide on one entry. Proven against the live registry: at v1.13.9 the same schematic resolves to `factory.talos.dev/metal-installer/…` without the flag and `factory.talos.dev/metal-installer-secureboot/…` with it.
- **G-02-5 closed.** `registryRefused(status)` in `probe.go` is the package's only statement of the taxonomy. `ProbeBuildable` dropped its `status / 100` class test and `resolveInstallerRepo` its `status == http.StatusNotFound` test; both now ask the same function, asserted by one table driven through both call sites.
- **The live installer-name matrix is settled at both ends of the supported range** and recorded as an observation, not transcribed into the fake.
- **The `secureboot=true` assets test asserts the installer reference beside the ISO** — the assertion whose absence let the mismatched pair ship.
- **`DefaultTimeout`'s doc comment now names the ISO HEAD and the manifest GETs it governs** and points at the open probe-budget decision. Comment only; `git diff internal/imagefactory/client.go` contains no non-comment line.

## Live registry observation (task 3)

Run once with `HOLZKUBE_FACTORY_LIVE=1 go test ./internal/imagefactory/ -run TestLiveFactory -count=1 -v`, for the recorded schematic `20e64852…`:

| Talos version | `metal-installer` | `installer` | `metal-installer-secureboot` | `installer-secureboot` |
|---|---|---|---|---|
| v1.13.9 (pinned) | answered | answered | answered | answered |
| v1.12.0 (oldest supported) | answered | answered | answered | answered |

Resolved references at v1.13.9:

```
plain  = factory.talos.dev/metal-installer/20e64852…:v1.13.9
secure = factory.talos.dev/metal-installer-secureboot/20e64852…:v1.13.9
```

**It took two runs, and the first one throttled — recorded verbatim because it is the finding, not noise.** In the first run `metal-installer` and `metal-installer-secureboot` at v1.12.0 both exceeded the 60s client timeout without answering, and the SecureBoot pairing subtest failed the same way (`context deadline exceeded (Client.Timeout exceeded while awaiting headers)`) after 235s. The second run completed in 30.6s with every cell answering. This is exactly the behaviour `.planning/WINDOWS.md` entry 5 records: factory.talos.dev throttles, and it throttles without an HTTP response. **Entry 5 stays open.** No retry was added and no timeout was raised — every budget in this package belongs to `02-DECISION-probe-budget.md`.

What this settles: at both ends of the supported range a SecureBoot request resolves, so the new "502 with no installer reference" path is **not** reachable at v1.12.0 or v1.13.9. What it does not settle: v1.13.x below the pin, all of v1.14, and the fake's v1.9.0 SecureBoot cell, which remains an assumption labelled as one in its own comment.

`fake_test.go`'s `installerRepos` was **not** rewritten to match the matrix. Its `installerLegacyVersion` row still pins v1.9.0 to `installer` alone, which is the premise `TestInstallerImageFallsBackToTheLegacyName` rests on; `git diff` shows the row's non-SecureBoot cell unchanged.

## Task Commits

1. **Task 1 (tracer, TDD): SecureBoot selects the installer** — `025f844` (test, RED) → `d5698ab` (feat, GREEN)
2. **Task 2 (TDD): one taxonomy, both call sites** — `5c56f0b` (test, RED) → `33b13c8` (feat, GREEN)
3. **Task 3: live matrix and the contract** — `9562498` (test)

**Tracer feedback gate:** after `d5698ab` the tracer's `<verify>` (`go test ./internal/imagefactory/ ./internal/httpapi/handlers/ -count=1 -race`) was re-run end to end and passed before any expansion task began.

## Files Created/Modified

- `internal/imagefactory/installer.go` — `installerCandidates` (new, owns ordering + SecureBoot selection + the refusal argument), `secureBootRepoSuffix`, SecureBoot in the cache key, `registryRefused` at the refusal branch
- `internal/imagefactory/probe.go` — `registryRefused` (new, the package's single taxonomy statement); `ProbeBuildable`'s class switch replaced
- `internal/imagefactory/urls.go` — `AssetRequest.SecureBoot`'s "and nothing else" comment corrected; it names `installerCandidates`
- `internal/imagefactory/client.go` — `installerRepos` field comment; `DefaultTimeout` doc comment. **Comments only**
- `internal/imagefactory/probe_test.go` — **new.** The shared `registryAnswerTable` and `ProbeBuildable`'s half of the agreement
- `internal/imagefactory/installer_test.go` — four SecureBoot behaviours, the mirror status table, the all-400 refusal
- `internal/imagefactory/fake_test.go` — SecureBoot names in the fixture; `installerNoSecureBootVersion` (v1.8.0, outside the supported range on purpose); the map's comment now says it is branch coverage and not a registry transcript
- `internal/imagefactory/live_test.go` — the matrix subtests, the expectation table, `liveManifestAnswers`; the skip message names the SecureBoot matrix
- `internal/httpapi/handlers/schematics_test.go` — the manifest fake serves all four names as a whitelist; the `secureboot=true` test asserts the installer and checks the ordinary request is unchanged
- `docs/api-contract.md` — SecureBoot selects the installer repository (with a five-reference example); the status taxonomy table and why a rate limit is retryable

## Decisions Made

1. **Refuse rather than substitute.** When neither SecureBoot name answers, `InstallerImage` returns `ErrSchematicNotBuildable` and no reference. Substituting the ordinary installer would reintroduce the exact ISO/installer drift G-02-4 found, silently. The trade-off is written into `installerCandidates`' doc comment so the next person to see a SecureBoot 502 finds the reasoning rather than a regression.
2. **400 and 404 are answers; everything else is a non-answer.** Argued in `registryRefused`'s comment. The decisive case is 429: the probe verdict is written once at creation with no re-probe path, so a throttle filed as `ErrSchematicNotBuildable` is permanent and unclearable.
3. **Two tables, deliberately apart.** The registry's matrix lives in `live_test.go`; the branch-coverage fixture stays in `fake_test.go`. Both files say so, and both name the other, because the obvious "fix" is to merge them and that would break a passing test.
4. **The value of `DefaultTimeout` was not touched**, and neither was any other budget. The one carve-out the plan allowed was the doc comment, and it is the only thing in `client.go` that changed.
5. **Mid-flight verification was not halted for.** `workflow.human_verify_mode` is `end-of-phase` and `mode` is `yolo`, so the tracer gate was discharged by re-running its `<verify>` rather than by stopping for a human; task 3's `<human-check>` is left for the end-of-phase harvest. Recorded because the executor's default in a non-auto run would have been to stop.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] The live subtest read a throttle as a defect**

- **Found during:** Task 3, on the first opt-in run
- **Issue:** The "half that fails" called `t.Fatalf` on any error from `InstallerImage`, so the first live run failed on `context deadline exceeded` — a transport timeout, which is a non-observation. The plan is explicit that a throttle is a finding and not a verdict, and I had applied that rule to the recording half only. Left as written, the one test that guards against upstream drift would fail on a throttle and be trained out of anyone's attention.
- **Fix:** `errors.Is(err, ErrUpstreamUnavailable)` now skips with a `NOT OBSERVED:` message naming what went unverified; `ErrSchematicNotBuildable` still fails, because that is a real refusal. The distinction is the one task 2 had just made available.
- **Files modified:** `internal/imagefactory/live_test.go`
- **Verification:** the second live run passed in 30.6s with the matrix fully observed
- **Committed in:** `9562498`

**2. [Rule 3 - Blocking] `installerNoSecureBootVersion` moved from v1.12.0 to v1.8.0**

- **Found during:** Task 3
- **Issue:** The fixture version I chose for the non-substitution branch collided with the version the live matrix probes, so a reader would have found the same version string asserting one thing as a constructed fixture and another as an observation — the precise confusion task 3 exists to prevent.
- **Fix:** moved to v1.8.0, outside the supported v1.12–v1.14 range like `installerBrokenVersion`, with a comment saying the cell is constructed and nothing has checked whether any shipped version behaves that way.
- **Files modified:** `internal/imagefactory/fake_test.go`
- **Verification:** `go test ./internal/imagefactory/ -count=1 -race` green
- **Committed in:** `9562498`

**3. [Rule 3 - Blocking] revive `context-as-argument` on the new helper**

- **Found during:** Task 3, at the `golangci-lint` gate
- **Issue:** `installerNameMatrixSubtests(t, client, ctx)` put `context.Context` last.
- **Fix:** reordered to `(ctx, t, client)`.
- **Files modified:** `internal/imagefactory/live_test.go`
- **Verification:** `golangci-lint run` → `0 issues`
- **Committed in:** `9562498` (amended)

### Additions beyond the literal plan text

- **A fourth fixture version.** The plan's three versions could not express "the ordinary names answer and the SecureBoot names do not", which is the only shape that proves the non-substitution clause. `installerNoSecureBootVersion` was added for it. No existing row's meaning changed.
- **The handler test also asserts the ordinary request.** The plan asked for the SecureBoot assertion; asserting that a non-SecureBoot request stays free of the suffix is what makes the two a pair rather than one value that happens to carry a suffix.

---

**Total deviations:** 3 auto-fixed (1 bug, 2 blocking) plus 2 test-coverage additions.
**Impact on plan:** No scope creep. Every prohibition held: no installer reference is assembled or guessed, and no timeout, deadline or budget value in `internal/imagefactory` changed — `git diff internal/imagefactory/client.go` is comment lines only.

## Issues Encountered

**1. `.planning/WINDOWS.md` cannot be appended to — the ledger is malformed for the tool, across the whole project.**

The plan's task-3 fallback asks for a `gsd-tools windows append --kind unrun-verify` entry. Every ledger command now fails:

```
$ gsd-tools windows append --kind unrun-verify --phase 02 …
Error: Ledger entry 7 has invalid kind: "review-finding"
$ gsd-tools windows status
Error: Ledger entry 7 has invalid kind: "review-finding"
```

Entries 8–18 (recorded 2026-08-29T12:40 by the code-review pass) carry `kind: "review-finding"`, which is not in the tool's allowed set (`stub, todo, fixme, skipped-test, lint-warning, unmet-truth, unrun-verify, deviation` — `gsd-core/bin/lib/broken-windows.cjs:89`). One invalid entry rejects the whole file, so `append`, `status`, `waive` and `fixed` are all blocked. This is pre-existing and not caused by this plan; the plan forbids hand-editing the markdown table, the JSON block or the frontmatter counts, so **nothing was written to the file**.

This does not leave an unrecorded assumption. The plan requires *either* the observed matrix in the SUMMARY *or* a ledger entry, and the live run completed, so the observation above is the record. But **plans 02-10 through 02-13 will hit the same wall**, and the ledger is currently unreadable by `/gsd-ship`'s gate. Fixing it means either adding `review-finding` to the tool's vocabulary or rewriting eleven entries' kinds — a decision about a cross-phase register this plan does not own.

**2. The first live run throttled.** Recorded in full above. `WINDOWS.md` entry 5 stays open and is now demonstrated a second time.

## Merge-time acceptance (T-02-53)

This phase ships a new 502 path on `GET /api/v1/schematics/{id}/assets?secureboot=true`: at a Talos version where neither SecureBoot installer name resolves, the route answers with no installer reference rather than substituting the ordinary one. **The live matrix shows this is not reachable at v1.12.0 or v1.13.9**, the two versions probed. It is unknown for v1.13.x below the pin and for v1.14. Refusing is deliberate — substituting reintroduces the drift G-02-4 found — but the acceptance belongs to a human at merge, not to a later UAT.

## Residue filed against deferred work (T-02-45)

Putting SecureBoot in the cache key doubles the key space, so `?secureboot=true` no longer reuses the non-SecureBoot entry and performs its own cold resolution. That walk is two serial candidates at `DefaultTimeout = 30s` each: **2 × 30s = 60.000s against `writeTimeout = 60s`** (`cmd/holzkubed/main.go:44`) — equal, not inside, and precisely the composition G-02-2 measured a 502 at (`duration=1m0.002907792s`). This plan neither worsens that per-request ceiling nor fixes it; it doubles the number of requests that can arrive at it cold. Bounding it needs the per-route deadline `02-DECISION-probe-budget.md` owns, and G-02-2 is deferred to cluster A. Accepted and filed there, not absorbed.

Task 2 also enlarges the unprobed population: 401, 403 and 429 no longer stamp `ProbedAt`, so strictly fewer answers produce a verdict. That is the correct classification and it makes the probe-budget question more pressing rather than answering it.

## User Setup Required

None — no external service configuration required. The live drift check remains opt-in via `HOLZKUBE_FACTORY_LIVE=1`.

## Next Phase Readiness

- G-02-4 and G-02-5 are closed. Wave 1 of the gap-closure set is done; 02-10 through 02-13 follow.
- `02-DECISION-probe-budget.md` remains open and now has one more input: the unprobed population is larger, and the doubled cold-resolution count meets `writeTimeout` exactly.
- **Blocker for the following plans:** `.planning/WINDOWS.md` is not appendable (issue 1 above). Any plan whose acceptance criteria include a `gsd-tools windows append` will fail that criterion until the `review-finding` kind is reconciled.

## Self-Check: PASSED

- `internal/imagefactory/probe_test.go` — FOUND
- Commits `025f844`, `d5698ab`, `5c56f0b`, `33b13c8`, `9562498` — all FOUND in `git log`
- `go test ./... -count=1 -race` — green across all 14 packages
- `golangci-lint run` (`~/go/bin/golangci-lint`) — `0 issues.`
- `gofmt -l ./cmd ./internal` — no output
- `go test ./internal/imagefactory/ -run TestLiveFactory -count=1 -v` — skips loudly, naming the SecureBoot matrix
- `git diff internal/imagefactory/client.go` — comment lines only; no constant's value changed

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-29*
