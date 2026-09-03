---
phase: 02-transport-seam-talossim-image-factory
verified: 2026-08-30T12:25:00Z
verified_at_commit: fb16546
status: gaps_found
score: 18/19 must-haves verified
behavior_unverified: 1
overrides_applied: 0
re_verification:
  round: 3
  previous_status: human_needed
  previous_score: 38/39
  previous_verified_at_commit: ad786b59c058f741a2d5681c106e9f69d966ad74
  gaps_closed:
    - "G-02-10 — a repository name occupies one line box; the hyphen break opportunity is suppressed and the property is now measured in a real layout engine across all four installer names"
    - "G-02-11 — the browser's refusal set is the server's, held equal by an exhaustive codepoint sweep that fails in both directions"
    - "G-02-12 — installer.go no longer claims the matrix settles the supported range; the unprobed regions are named in place"
    - "G-02-13 — the two SecureBoot names are recorded as different images, the expired digests are gone, and the legacy fallback is labelled rather than passed off as equivalent"
    - "G-02-14 — UsabilityVerdict asks whether a verdict exists before reading it, so usable:true with a zero probed_at can no longer render an affirmative badge"
    - "G-02-15 — an unresolvable installer returns the four registry-free references plus installer_error, and the panel says which of the two things happened"
    - "G-02-16 — representable covers the four measured divergence classes; the differential against the live Factory exists and its floor-not-ceiling limits are in the contract"
    - "G-02-17 — name, cluster, kernel_args, extensions and meta all pass the one refusal predicate; a stored bidi control is isolated at render"
    - "G-02-18 — the drift guard consults the warnings, compares the repository segment exactly, and reports a warning-bearing answer as NOT OBSERVED rather than as a pass"
    - "G-02-19 — the caller's width rule is sm:max-w-3xl and the dialog measures 768px in a real browser"
    - "G-02-20 — the identity constraint is written into model.go, docs/api-contract.md and the ledger; a decision document records the reasoning"
    - "G-02-21 — WINDOWS entry 30 names the plan-09 throttle, the unprobed v1.14 and the unprobed v1.13.x below the pin; VERIFICATION row 7 is corrected in place"
    - "G-02-22 — the recoverability claim is narrowed to the probe's outcome in model.go and api.ts, and entry 31 supersedes entry 21's clause"
    - "G-02-23 — the problem taxonomy is re-rooted at urn:holzkube-manager:problem:, closed by an AST test, and documented as non-dereferenceable"
  gaps_remaining: []
  regressions: []
  adversarial_checks_run:
    - "Removed `whitespace-nowrap` from ReferenceValue's segment span and re-ran the browser project: 3 of 4 name sweeps FAILED with the split widths and line contents printed. metal-installer alone still passed — which is the original G-02-10 finding reproduced. File restored."
    - "Deleted the U+FEFF entry from REFUSED_RANGES and re-ran TestBrowserRefusalSetEqualsTheServers: FAILED naming U+FEFF as under-refused. File restored."
    - "Ran `npm --prefix web test` with a JSON reporter: 8 files / 126 tests, images.browser.test.tsx among them; `--project browser` alone is 7 tests. Both vitest projects execute under the bare command."
    - "Parsed .planning/WINDOWS.md's three representations independently: frontmatter 47/0/9/56, table 47 open + 9 fixed over 56 rows, JSON block 56 entries with the same statuses. Zero status mismatches, ids 1..56 with no duplicates."
gaps:
  - truth: "A guard reports a pass only when it measured the property it is named for"
    status: partial
    reason: >-
      TestLiveCanonical classifies a throttled or transport-failed batch as `notObserved`, and
      reportCanonResults handles that verdict with `t.Logf` alone — only `diverges` and an
      over-refused negative control produce `t.Errorf`. A live run in which every batch is
      throttled therefore records zero DIVERGES rows and reports PASS, with the NOT OBSERVED
      marker visible only under `-v`. This is the same shape the round fixed in the sibling
      guard: `live_test.go:425-431` turns a warning-bearing installer answer into `t.Skipf`
      precisely so it "is no longer reported as a pass". The principle was applied in one file
      of this round and not in the other, and nothing records the difference — WINDOWS entry 35
      says the differential is opt-in and unrun in CI, not that a fully-throttled run reports
      green. Nothing was masked in practice: 02-14's recorded runs carry 0 NOT OBSERVED rows,
      so this is about the guard's future behaviour rather than about the current measurement.
    artifacts:
      - path: "internal/imagefactory/canonical_live_test.go:644-651"
        issue: "the `notObserved` case is `t.Logf`; the test exits 0 having compared nothing against factory.talos.dev"
    missing:
      - "Make a run that observed nothing report as something other than a pass — `t.Skipf` when every row is notObserved, or a failure when the observed fraction falls below a stated floor — matching live_test.go's own rule"
      - "Or, if the current behaviour is deliberate, record it in WINDOWS entry 35 so the register carries it"
  - truth: "A blocking human decision checkpoint that was resolved without a human says so where the decision is read"
    status: partial
    reason: >-
      `02-21-PLAN.md:113` declares task 1 as `<task type="checkpoint:decision" gate="blocking">`.
      It was resolved by the executor without stopping. 02-21-SUMMARY.md:165-181 records that
      candidly and at length, including the explicit note that it is "a non-standard checkpoint
      resolution rather than an approval". The artifact a later reader consults does not:
      `02-DECISION-schematic-identity.md` opens `Status: **decided**` / `Decided in:
      02-21-PLAN.md task 1` and contains no occurrence of "checkpoint", "ratified", "user" or
      any equivalent. The contrast is inside this same phase — 02-UAT.md test 4 carries
      `ratified_by: user` / `ratified_at`, and 02-DECISION-probe-budget.md is `open` and was
      respected as such. This is the round's own failure shape: candid in the SUMMARY, silent
      where a reader would look.
    artifacts:
      - path: ".planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-schematic-identity.md:3-5"
        issue: "`Status: decided` with no indication that the blocking human gate was self-resolved"
    missing:
      - "One line in the decision document's header stating that the blocking checkpoint was resolved by the executor on the dispatcher's pre-answer and the plan's own recommendation, not ratified by the user — with a pointer to 02-21-SUMMARY.md's account"
  - truth: "A completion record states only what is true of the repository"
    status: partial
    reason: >-
      02-21-SUMMARY.md's Issues Encountered #2 reads "`npm --prefix web test` runs one vitest
      project, not two" and "No second project configuration exists in `web/` today, so the
      criterion is met as far as the repository can express it." Both sentences are false.
      `web/vite.config.ts` declares two named projects (`jsdom` and `browser`) inside one
      config, and `web/package.json` declares `test:browser`. Measured at this commit with a
      JSON reporter: the bare `npm --prefix web test` runs 8 files / 126 tests including
      `images.browser.test.tsx`, and `--project browser` alone is 7 of those tests. The plan's
      `<verification>` criterion "green across both projects" is therefore literally met, and
      the SUMMARY records it as unmeetable for a reason that is not true. The risk is concrete:
      a reader acting on that sentence would take `test:browser` and the browser project for
      redundant configuration and remove the only layout evidence G-02-10 and G-02-19 have.
    artifacts:
      - path: ".planning/phases/02-transport-seam-talossim-image-factory/02-21-SUMMARY.md"
        issue: "Issues Encountered #2 contradicts web/vite.config.ts and web/package.json, and contradicts a measured run"
    missing:
      - "Correct the two sentences — the criterion was met, both projects run under the bare command — in the same corrected-in-place register 02-21 used for VERIFICATION row 7"
deferred:
  - truth: "Ein nicht erreichbarer Node blockiert die UI nicht (the UI half of ROADMAP SC 3 / TRANS-05)"
    addressed_in: "Phase 3"
    evidence: >-
      Unchanged at fb16546. `git diff ad786b5..fb16546 -- internal/talos internal/talossim
      cmd/holzkubed` is empty, so the previous report's finding stands: no production caller of
      NewClusterClient, NewMaintenanceClient, FanOut, NewBreaker, NewDirectDialer or
      NewManualSource exists outside internal/talos and its tests. The transport half is proven
      behaviourally.
  - truth: "Dieselbe Contract-Suite läuft auch gegen echtes Talos (TRANS-08)"
    addressed_in: "Phase 3"
    evidence: "REQUIREMENTS.md maps TRANS-08 to Phase 3 (Pending); internal/talos/contract_test.go already parameterises the transport."
  - truth: "G-02-1 / G-02-2 / G-02-9 — the probe-budget cluster"
    addressed_in: "Open decision — 02-DECISION-probe-budget.md"
    evidence: >-
      Re-confirmed at fb16546 rather than carried over: `DefaultTimeout = 30 * time.Second`
      (client.go:43) and `writeTimeout != 60*time.Second` is a hard failure in
      cmd/holzkubed/budget_test.go:157. Neither moved in this round. WINDOWS entry 20 is still
      open and entry 48 stands beside it saying in capitals that plan 02-19 changed what an
      unresolved installer returns and not the cold 2 x 30s walk that produces it.
behavior_unverified_items:
  - truth: "Ein nicht erreichbarer Node blockiert weder die UI noch andere Nodes (ROADMAP SC 3)"
    test: >-
      Once Phase 3 wires a route to the transport: open the inventory with one node injected
      with go_silent(90s) and confirm the page renders the healthy nodes immediately and the
      silent one as unreachable, rather than the page waiting on the silent node's budget.
    expected: >-
      The UI paints healthy nodes within one round trip; the silent node resolves to an error
      after its own per-node budget only.
    why_human: >-
      Unchanged and re-checked at this commit: there is still no production caller of the
      transport seam, so the UI half of the criterion has no code path to exercise. The
      transport half IS proven (TestFanOutOneSilentNodeCostsOneNode).
coincidental_reliance_items:
  - truth: "Das Schematic gilt erst nach einem bestätigenden Model-Build-Probe als brauchbar (part of ROADMAP SC 4)"
    reason: undeclared-precondition
    harden: >-
      Carried forward unchanged. Every passing observation of the Usable=false -> true
      transition rests on the Factory's ISO being already built; for a genuinely novel tuple the
      probe measures 30.5-32.7s against a 30s budget and the verdict is absent. This is G-02-1
      and it belongs to the open 02-DECISION-probe-budget.md. Advisory only.
  - truth: "The four installer repository names occupy one line box (G-02-10)"
    reason: incidental-ordering
    harden: >-
      The sweep is Chromium-only (playwright 1.62.1, headless, 1200x900). UAX #14's hyphen rule
      and `white-space: nowrap` are not a corner of CSS where engines disagree, so the
      inference to Firefox and WebKit is sound — but it is an inference. Already declared:
      WINDOWS entry 42 records exactly this. Advisory only; it changes no status and no score.
human_verification:
  - test: >-
      Drive the assembled binary through a browser against the live Image Factory: author a
      schematic end to end, and confirm the /images route works outside a test harness.
    expected: "The route behaves as the suites predict, with real latency and a real bundle."
    why_human: >-
      Still outstanding, and correctly scoped by the round rather than claimed closed.
      `images.browser.test.tsx` opens ImagesView in a real Chromium, but its own doc comment
      says plainly: "this is not a general browser suite and it is not an end-to-end test... no
      binary runs, no bundle is served, and the embedded UI has still never been driven end to
      end in a browser." The two properties it does measure — the dialog width and the four
      line-box sweeps — are genuinely closed and need no human.
  - test: >-
      Confirm the CI browser-install step runs on ubuntu-latest.
    expected: "`npm --prefix web exec -- playwright install --with-deps chromium` succeeds and the browser project runs in CI."
    why_human: >-
      WINDOWS entry 44 records this: no CI run has happened against this commit, and the
      browser project is now the sole evidence for G-02-10 and G-02-19. It fails closed —
      requireBrowserBinary() throws with the fixing command — so the risk is a red CI run, not
      a silent pass.
---

# Phase 2 Verification — round 3 (the round-2 gap-closure round)

**Phase Goal:** Jede Talos-Interaktion läuft durch eine austauschbare Naht und ist ohne Hardware
testbar; Schematics und Image-URLs sind korrekt und nachweislich brauchbar herleitbar.
**Verified:** 2026-08-30T12:25:00Z at `fb16546` (working tree clean)
**Status:** gaps_found — 14/14 gap truths hold; three record- and guard-integrity defects introduced
by the round itself
**Re-verification:** Yes — round 3, over plans 02-14 … 02-21.

This round exists because two UAT checks were recorded PASS on reasoning that proved a different
proposition than the one asserted. Verification was therefore run against that failure mode
specifically, and against the code rather than against the eight SUMMARYs. Where a claim was
falsifiable, it was falsified: two guards were deliberately broken and re-run, and the ledger's
three representations were parsed independently rather than read.

## 1. The fourteen closed gaps, checked against the tree

| Gap | Truth | Status | Evidence at `fb16546` |
|---|---|---|---|
| G-02-10 | The repository name is unbroken | ✓ VERIFIED | `ReferenceValue` wraps each `/`-separated segment in `whitespace-nowrap` (`images.tsx:1513`), which suppresses the UAX #14 HY break the previous `break-normal` did not. Proven by measurement, not by class string: `images.browser.test.tsx` sweeps all four names at every width 30→280px in Chromium, counting line boxes via `Range.getClientRects()`, and asserts `textContent` is byte-identical at every step so a look-alike hyphen cannot satisfy it. **Falsified:** removing `whitespace-nowrap` fails 3 of the 4 sweeps with the split widths and line contents printed — and `metal-installer` alone still passes, reproducing the original finding that testing one name could never have caught this. |
| G-02-11 | A control character is refused with a field-named 400 | ✓ VERIFIED | `REFUSED_RANGES` (`images.tsx:105-132`) is declared as data covering C0/DEL/C1, surrogates, U+2028-9, U+FEFF and everything above U+FFFD. `NotRepresentableReason` is the server's single statement of the same rule and `refuseUnrepresentable` routes every operator scalar through it. `TestBrowserRefusalSetEqualsTheServers` sweeps all of Unicode through both sides and fails in **both** directions. **Falsified:** deleting the U+FEFF entry fails the guard naming U+FEFF. The unpaired-surrogate half is answered before decoding by `rawBodyRefusal` (`schematics.go:271`, `:830`), which is the only way to see what `encoding/json` would have rewritten. |
| G-02-12 | 02-09's matrix claims match what it probed | ✓ VERIFIED | `installer.go:220-231` replaces "is settled by" with a paragraph naming what is unprobed: `talos.MaxSupportedVersion` v1.14 is a range bound and not a tag, and no v1.13.x below the pin has been probed. `fake_test.go:73-81` retracts both false statements about the v1.9.0 row by name. The UAT's own substitution of `live_test.go` for the plan's `fake_test.go` is recorded in 02-UAT.md test 3. |
| G-02-13 | The two SecureBoot names are not interchangeable | ✓ VERIFIED | `installer.go:211-219` states the measured difference and says why the digests were removed — "a digest in a comment is a fact with an expiry date that nothing in the build checks". The fallback is kept and labelled: `WarningInstallerSecureBootRepoFallbackUnverified` is declared (`warnings.go:76`) **and emitted** (`installer.go:521`), mirrored in `web/src/api.ts:345` and given a row in `docs/api-contract.md:639`. |
| G-02-14 | No state claims more than the record supports | ✓ VERIFIED | `UsabilityVerdict` tests `!isProbed(probedAt)` first (`images.tsx:825`), with the ordering argued in place rather than left as an accident of writing order. |
| G-02-15 | A SecureBoot request that cannot resolve an installer still answers | ✓ VERIFIED | `schematics.go:508-537` returns the four registry-free references with `installer: null` plus `installer_error{code, detail}`. `UnresolvedInstallerRow` (`images.tsx:1407`) renders the server's own detail plus a per-code remedy that is *opposite* for the two codes, and names SecureBoot only when it was ticked. Both branches are tested (`images.test.tsx:1184`, `:1203`, `:1207`). The contract carries the new shape and its five-outcome table (`docs/api-contract.md:701-792`). |
| G-02-16 | FACT-06 holds above U+007F | ✓ VERIFIED | `NotRepresentableReason` (`schematicid.go:321-380`) covers the four measured classes, each clause citing the rows that proved it. The oracle is external and only external: `canonical_live_test.go` compares the local document and id against `Created.Canonical` and `Created.ID` from factory.talos.dev — not against a second local YAML library, which the file says would "replace one transcription with a different transcription". `plainAllowed` was deliberately left unchanged and the measurement is given as the reason. See gap 1 below for the one integrity defect in this file. |
| G-02-17 | Unusable operator input is refused before the Factory | ✓ VERIFIED | `refuseUnrepresentable` (`schematics.go:624-647`) walks `name`, `cluster`, `kernel_args`, `extensions` and `meta` through the one predicate and appends every error so one round trip reports all of them. The rendering contract is `StoredText`/`<bdi>`, applied at all four stored-operator-text render sites. |
| G-02-18 | The drift guard fails when the installer name drifts | ✓ VERIFIED | `checkInstallerName` (`live_test.go:291`) inspects the warnings first — because `resolveInstallerRepo` returns `ErrUpstreamUnavailable` only when *no* candidate answered, so a partial throttle yields a nil error and a usable reference — and reports such an answer as `installerNameNotObserved`, which the caller turns into `t.Skipf`, not a pass (`live_test.go:425-431`). Comparison is by exact repository segment, replacing `strings.Contains(secure, "-secureboot/")` which the legacy name also satisfied. Exercisable offline and exercised: `installer_test.go:1009`, `:1042`, `:1068`. |
| G-02-19 | The asset dialog widens on large screens | ✓ VERIFIED | The caller's class is now `sm:max-w-3xl` (`images.tsx:1091`), same tailwind-merge group as `DialogContent`'s `sm:max-w-sm`, so it wins instead of coexisting. Measured, not merged-and-asserted: the browser project reads `getBoundingClientRect().width` after awaiting the entry animation and asserts both `> 384` (the regression guard, which also fails loudly if the project is ever misconfigured back to jsdom) and `=== 768`. |
| G-02-20 | A record carries the architecture its probe used | ✓ VERIFIED | The constraint is stated in `model.go:105-115`, in `docs/api-contract.md` beside the existing 409 section, and in WINDOWS. `02-DECISION-schematic-identity.md` records A/B/C with B as the direction and C as this round's action. See gap 2 below for how the decision was reached. |
| G-02-21 | The unrun-verify ledger entry is filed | ✓ VERIFIED | WINDOWS entry 30 names all four things: the two-version matrix, the unprobeable v1.14, the never-probed v1.13.x below the pin, and the fake's v1.9.0 assumption. Entry 5 stays open independently, as the entry itself says. VERIFICATION row 7 is corrected **in place** with the original assessment kept verbatim — which is the right form, since "the criterion was reread rather than met" is the whole of the gap and a silent rewrite would repeat it. |
| G-02-22 | The architecture is not recoverable from a pre-02-13 record | ✓ VERIFIED | Narrowed in both places the gap named (`model.go`, `web/src/api.ts:190-215`) to "depends on the probe's outcome, not on the record's age", with the refused-probe format pinned by `TestRefusalReasonNamesTheArchitectureItAskedAbout`. Entry 31 supersedes entry 21's recoverability clause while leaving 21 open for the window itself. |
| G-02-23 | The problem taxonomy is deployment-independent | ✓ VERIFIED | `ProblemBaseURI = "urn:holzkube-manager:problem:"` with all thirteen types composed from it. No occurrence of `holzkube.dev/problems` survives anywhere outside `.planning/`. `TestProblemTaxonomyIsClosed` parses `problem.go` with `go/ast` and asserts each `Type*` is a `BinaryExpr` of `ProblemBaseURI + "<literal suffix>"` — so a repeated literal, which is "how a re-rooting lands on twelve of thirteen", is a failure and not a style question. |

**Score on the round's own work: 14/14.**

## 2. Would any of the new guards go green while its property is false?

This is the G-02-10 shape, asked of every guard the round added.

| Guard | Could it pass while the property is broken? | Evidence |
|---|---|---|
| Browser line-box sweep | **No.** | Falsified live. Also fails closed in the wrong environment: in jsdom every rect is zero, `lineBoxCount` returns 0, and `0 !== 1` pushes every width into `split`. The vite config excludes the file from the jsdom project by name and says why the exclusion is explicit rather than a narrower include. |
| Dialog width | **No.** | `> 384` is stated as the regression guard *and* as the project-misconfiguration guard; `=== 768` is asserted separately as an observation. |
| Codepoint drift guard | **No.** | Falsified live. Fails closed on its own reading: `browserRefusalRanges` calls `t.Fatalf` when the regex matches nothing, on the stated ground that "a guard that silently passes when it can no longer find what it guards is worse than no guard". Only one `{from: 0x…}` table exists in `images.tsx`, so no conflation. |
| Installer-name binding | **No.** | The Go side pins the TypeScript literal array; the browser side asserts its own derivation equals that literal. A fifth Go candidate breaks the Go guard, and a drifted derivation breaks the browser one. Closes the residual 02-17 filed against itself (WINDOWS 43). |
| Warning-code drift guard | **No.** | `TestWarningDetailsMatchTheUI` now loops `exportedWarningCodes(t)`, an AST walk of every non-test file in the package, instead of naming the codes it knows — which is exactly how 02-16 shipped a Go warning with no TS mirror and stayed green. `TestWarningsCodesAreNamespaced` additionally asserts three known codes are in the scan's output so that a scan reading nothing cannot pass. *Minor:* the failure message asks for a `docs/api-contract.md` row and nothing asserts one; the row does exist today. |
| AST taxonomy closure | **No** for the declared surface. | Both directions asserted, plus declaration shape. *Minor:* it collects identifiers prefixed `Type`, so a fourteenth type named otherwise would escape. |
| Canonical differential | **Yes — see gap 1.** | The oracle is genuinely external and the corpus is real, but a run in which every batch throttles reports PASS. |

## 3. The three reported deviations

| Plan | Deviation | Sound? | Honestly recorded? |
|---|---|---|---|
| 02-19 | Extended the warning drift guard beyond "run the guard that checks the two sides agree" | **Yes.** No such guard existed for the new code; adding the mirror while leaving the guard blind would have left the next code in the same position. Verified in `warnings_test.go:329-334`. | Yes — filed as Rule 2, with the drift-and-restore verification recorded. |
| 02-20 | Widened `validate` from the two named fields to five scalars | **Yes.** The plan's own behaviour row required "a POST with bad values in `name`, `cluster` and `kernel_args` reports all three field errors in one response", and `kernel_args` was only refused downstream in `Schematic.ID()`. Checking two fields would have shipped that row false. Verified in `schematics.go:624-647`. | Yes — and the accompanying limitation is stated rather than glossed: the form has no `cluster` input, so the client cannot guard it and only the server does (WINDOWS 54). |
| 02-21 | `gsd-tools windows` has no amend verb, so entries 13/20/21/27 are superseded rather than reworded | **Yes.** The tool exposes `status`/`append`/`waive`/`fixed` only, and hand-editing would desynchronise the three representations the tool keeps in step. Each superseding entry opens in capitals naming what it supersedes, so a linear reader cannot meet the old text without the new. | Yes, and unusually well: the SUMMARY states the acceptance criterion is **"not literally met"** rather than reinterpreting it, on the explicit ground that reinterpreting a criterion inside the plan closing G-02-21 "would be a joke at the register's expense". |
| 02-21 | A `gate="blocking"` `checkpoint:decision` resolved without stopping | **Defensible** — the dispatcher pre-answered it, the plan's own `<recommendation>` said the same independently, and the project runs `mode: yolo`. The answer taken was the recommendation, not "the first option". | **In the SUMMARY, yes. In the decision document, no** — see gap 2. |

## 4. What the round did NOT fix

Checked for closure claims that outrun their evidence. The ledger is candid on every one of these, which is the notable finding:

- **WINDOWS entry 20 is unchanged and correctly stays open.** Re-measured rather than trusted: `DefaultTimeout = 30 * time.Second` and `budget_test.go` hard-fails if `writeTimeout != 60s`. Entry 48 stands beside 20 saying in capitals that 02-19 changed what an unresolved installer *returns*, not the cold 2 × 30s walk — and warning specifically that "the panel now shows four references on a timeout" must not be read as the timeout having been addressed. The milder symptom does make `02-DECISION-probe-budget.md` easier to defer; the ledger says so.
- **The refusal set is a floor, not a ceiling.** Six unmeasured regions are named in `docs/api-contract.md:924-937` — where an API reader looks, not only in a SUMMARY — with the explicit instruction to read the section as "these classes were measured to diverge and are refused", not as "every divergent codepoint is refused". Also WINDOWS 32 and 56.
- **The differential never runs in CI.** WINDOWS 35.
- **Only one browser engine is measured.** WINDOWS 42.
- **Pre-02-14 and pre-02-20 records are unmigratable.** WINDOWS 37, 53.
- **Entries 25 and 26 were deliberately not fixed** even though 02-19 rewrote the section they sit in, on the ground that "a scoping decision is not an executor's to overturn". WINDOWS 50.
- **Entry 13's `never sent` half is still true**; only the `unvalidated` half closed. WINDOWS 52 says exactly that.
- **The CI browser-install step has never run.** WINDOWS 44. It fails closed.

## 5. Ledger integrity

Parsed independently rather than read.

| Representation | open | waived | fixed | total |
|---|---|---|---|---|
| Frontmatter | 47 | 0 | 9 | 56 |
| Markdown table | 47 | 0 | 9 | 56 rows |
| JSON block | 47 | 0 | 9 | 56 entries |

Ids 1…56, no duplicates, no status mismatch between table and JSON on any entry. The six entries 02-21 marked fixed were checked in the tree, not on the SUMMARY's say-so:

| Entry | Claim | Verified |
|---|---|---|
| 23 | The provisional-warning branch of GET /assets had no handler-level test | `schematics_test.go:1584` `t.Run("provisional", …)` asserts the reference is non-null, exactly one fallback warning, and a non-empty detail |
| 29 | A lone surrogate from an API client is rewritten before the id is computed | `rawBodyRefusal` at `schematics.go:830`, called at `:271` on the raw bytes before `decodeJSON` |
| 33 | `images.tsx` under-refuses relative to the server | `REFUSED_RANGES` + `TestBrowserRefusalSetEqualsTheServers`, falsified live |
| 38 | QF1012 in `canonical_live_test.go:627` | no `WriteString(fmt.Sprintf` remains in the file |
| 40 | The new warning code had no TS mirror and no contract row | `api.ts:345` and `docs/api-contract.md:639` |
| 43 | `INSTALLER_REPOSITORY_NAMES` was not pinned to `installerCandidates` | `TestBrowserInstallerNamesEqualInstallerCandidates`, passing |

## 6. The unprobed regions

| Region | Recorded where a reader would look? |
|---|---|
| `talos.MaxSupportedVersion` v1.14 never probed | Yes — `installer.go:220-231` (the function whose refusal it governs), `live_test.go:495-517` (with the reason it is unprobeable: a range bound is not a tag), WINDOWS 30 and 41 |
| No v1.13.x below the pin probed | Yes — same three places. `liveNewestSupportedRow` asks the version list at run time and starts probing v1.14.x on its own once upstream ships it, so this half closes itself |
| The fake's v1.9.0 rows are constructed, not observed | Yes — `fake_test.go:73-81`, in the register "do not fix this map against the registry" |
| The canonical differential's six unmeasured regions | Yes — `docs/api-contract.md:924-937` names both what the sweep reached and what it did not, plus WINDOWS 32 and 56 |

## 7. Regression check on the ROADMAP success criteria

`git diff ad786b5..fb16546 -- internal/talos internal/talossim cmd/holzkubed` is **empty**, so SC 1, 2, 3 and 5 carry from the round-2 report unchanged, including SC 3's UI clause remaining ⚠️ PRESENT_BEHAVIOR_UNVERIFIED and deferred to Phase 3. SC 4 is materially strengthened by this round (the installer answer, the canonical serialiser and the browser refusal set all moved toward it) and nothing in the round weakened it.

**ROADMAP score: 4/5 (1 present, behaviour-unverified). Round-2 gap truths: 14/14. Combined: 18/19.**

## Gaps Summary

All fourteen gap truths hold in the tree, and the two guards it was possible to falsify were
falsified and fail closed. The three gaps are not in what the round built; they are in what the
round *said*, and in one guard it built to a standard it applied elsewhere in the same round.

1. `TestLiveCanonical` reports a pass on a run that measured nothing. The sibling guard in the
   same round was changed for precisely this reason, and the divergence between the two files is
   unrecorded.
2. `02-DECISION-schematic-identity.md` reads `decided` without saying that a blocking human
   checkpoint was resolved by the executor. The candid account exists — in the SUMMARY only.
3. `02-21-SUMMARY.md` states two things about `web/`'s test configuration that are false, and
   declares an acceptance criterion unmeetable that is in fact met.

Each is a one-line to one-paragraph fix and none blocks the fourteen closures. They are filed as
gaps rather than notes because this phase has twice now paid for the same thing: a record that is
candid in one place and silent in the place a reader actually consults.

---

_Verified: 2026-08-30T12:25:00Z at `fb16546`_
_Verifier: Claude (gsd-verifier), round 3_

---

## Appendix — round 2 report, retained verbatim


# Phase 2: Transport Seam, `talossim` & Image Factory — Verification Report (round 2)

**Phase Goal:** Jede Talos-Interaktion läuft durch eine austauschbare Naht und ist ohne Hardware
testbar; Schematics und Image-URLs sind korrekt und nachweislich brauchbar herleitbar.
**Verified:** 2026-08-29T21:35:00Z at `ad786b5`
**Status:** human_needed
**Re-verification:** Yes — after the gap-closure round (plans 02-09 … 02-13) and the review-fix
pass (`49be02f`, `be75d5e`, `2ee117b`, `ec10e08`, `37d40b2`).

Everything below was re-run in this session against the working tree. No claim is carried over
from a SUMMARY, a PLAN or the previous report — including the four criteria that already passed,
which were re-checked for regression.

## Goal Achievement

### A. Observable Truths — ROADMAP Success Criteria

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Der unveränderte Produktions-Client spricht gegen `talossim` — echte Protobufs, echtes mTLS, echter In-Memory-COSI-State | ✓ VERIFIED (regression) | `go test ./... -count=1` green across all 18 packages, including `TestTracerRealClientReachesFakeNode`. `go list -deps ./cmd/holzkubed \| grep -c talossim` = **0**. `TestSimulatorIsNotInTheProduct` present and passing. Untouched by the gap-closure round: `git diff c1c65a2..HEAD` names no file under `internal/talos*`. |
| 2 | Die neun TRANS-07-Fehlerszenarien sind injizierbar und der Client verhält sich definiert | ✓ VERIFIED (regression) | `TestScenarioContract` and `TestRegistryCoversTRANS07` enumerate and pass; the suite still iterates `talossim.Registry`, so a tenth scenario without an assertion is a red test. Untouched by this round. |
| 3 | Ein nicht erreichbarer Node blockiert weder die UI noch andere Nodes; erzwungenes Deadline; Retries nur für eine Read-Allowlist; getrennte Client-Typen | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Three of four clauses behaviourally proven and unchanged. The **UI clause still has no code path**: `grep` over `cmd` and `internal` (excluding `internal/talos` and `_test.go`) finds no production caller of `NewClusterClient`, `NewMaintenanceClient`, `FanOut(`, `NewBreaker`, `NewDirectDialer` or `NewManualSource` — only a comment in `router.go:99` and a scenario string. Deferred to Phase 3. |
| 4 | Schematic aus versions-skopiertem Katalog; exakte ISO-/Installer-/PXE-URLs mit versionsabhängigem Repo-Namen, ohne hartkodierte Architektur; usable erst nach Model-Build-Probe; Kernel-Args/META warnen | ✓ VERIFIED (coincidental-reliance) | Materially **stronger** than at `575da7a`: the installer reference is now SecureBoot-correct, which it was not. Proven live at this commit — see the SC 4 breakdown. The reliance flag is on the probe clause only and is advisory; see `coincidental_reliance_items`. |
| 5 | Das gesamte Binary läuft mit `--dry-run`, und keine Mutation erreicht dabei einen Node | ✓ VERIFIED (regression) | `go run ./cmd/holzkubed --help` prints `--dry-run … refuse every mutating node call at the transport (env HOLZKUBE_DRY_RUN)`. `TestDryRunRefusesEveryMutationAtTheNode` and `TestDryRunRefusesApplyConfigurationInMaintenanceMode` present and passing. Untouched by this round. |

**ROADMAP score: 4/5 truths verified (1 present, behaviour-unverified).**

#### SC 3 breakdown (unchanged from `575da7a`, re-checked)

| Clause | Status | Evidence at `ad786b5` |
|--------|--------|-----------------------|
| …blockiert nicht **andere Nodes** | ✓ VERIFIED | `TestFanOutOneSilentNodeCostsOneNode`, `TestFanOutSkipsAnOpenCircuitWithoutDialing`, `TestFanOutCancellationTerminatesEveryInFlightCall` all present and green. |
| …**erzwungenes Deadline** | ✓ VERIFIED | `TestRequireDeadline` green; `requireDeadline(ctx)` in both interceptors. |
| …**Retries nur für eine Allowlist** | ✓ VERIFIED | `TestRetryAllowlistIsExactlyTheFastReadClass` green. |
| Client-Typen **nicht verwechselbar** | ✓ VERIFIED | `TestMaintenanceClientRejectsClusterOnlyCall` green — a real `go build -tags talos_compile_fail` that must fail. |
| …blockiert nicht **die UI** | ⚠️ UNVERIFIED | No production caller. Deferred to Phase 3. |

#### SC 4 breakdown — live evidence gathered in this session

`HOLZKUBE_FACTORY_LIVE=1 go test ./internal/imagefactory -run TestLiveFactory` — **PASS in 32.1 s,
7/7 subtests**, against the public `factory.talos.dev`:

| Check | Result |
|-------|--------|
| Pinned Talos release still listed | PASS (0.49 s) |
| Version-scoped catalog still lists the recorded extension | PASS (0.30 s) — no free-text field exists client-side either (`images.tsx` renders a `<fieldset>` over `catalog.data.extensions`) |
| Locally precomputed id still agrees with upstream (**FACT-06**) | PASS (0.15 s) — the canonical serialiser has not drifted |
| Creation is still not validation (**FACT-02**) | PASS (0.30 s) |
| A good schematic still builds | PASS (3.52 s) — warm; see the reliance note |
| **SecureBoot resolves to a different installer than the ordinary request** | PASS (5.73 s). Logged verbatim: `plain = factory.talos.dev/metal-installer/20e64852…:v1.13.9`, `secure = factory.talos.dev/metal-installer-secureboot/20e64852…:v1.13.9`. This is the G-02-4 defect, gone. |
| The installer-name matrix is what the file records | PASS (21.62 s). Observed: at **v1.13.9** and **v1.12.0**, all four of `metal-installer`, `installer`, `metal-installer-secureboot`, `installer-secureboot` answered. Agrees with 02-09-SUMMARY.md:170-173. |

Additional SC 4 checks, offline:

| Check | Result |
|-------|--------|
| No hardcoded architecture (**FACT-03**) | `urls.go` contains the two `Arch` enum constants and no architecture literal inside a URL. `TestAssetURLsDifferOnlyInTheArchitecture` and the web test "changes every asset URL when the architecture control changes" pass. `TestTheStoredArchitectureDoesNotDefaultTheAssetsQuery` pins that `?arch=` stays required even now that the record carries one. |
| usable only after the probe | `Usable` is written only from `ProbeBuildable`. `TestCreateReturns201WithWarningsAndAProbedVerdict`, `TestCreateStoresTheArchitectureTheProbeUsed`, `TestUnansweredProbeStillStoresTheArchitecture` pass. |
| Kernel-args / META warning (**FACT-04**) | `TestWarningsForKernelArgs`, `TestWarningsForMeta`, `TestWarningsNameInstallerAndInitramfs`, `TestWarningDetailsMatchTheUI` pass; web tests "warns … while they are being typed, before any create request" and "stays quiet about a row that has just been added and holds nothing" pass. |
| Prereleases / broken versions (**FACT-05**) | Web tests "hides prereleases until they are explicitly asked for" and "renders a broken version disabled and says why it is listed" pass. `brokenversions.go` is still deliberately empty, documented in-file. |

### B. Observable Truths — gap-closure must_haves (plans 02-09 … 02-13)

All 34 `must_haves.truths` across the five gap-closure plans, checked against code and tests
rather than against the SUMMARYs.

#### 02-09 — G-02-4 (SecureBoot) and G-02-5 (registry taxonomy)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SecureBoot request ⇒ SecureBoot installer repository | ✓ VERIFIED | `installerCandidates` appends `secureBootRepoSuffix` (`installer.go:206-215`). `TestInstallerImageResolvesTheSecureBootName`, `TestInstallerImageFallsBackToTheLegacySecureBootName` pass; confirmed live. |
| 2 | Two requests differing only in SecureBoot never share a cache entry | ✓ VERIFIED | `installerRepoKey` = `"%s/%s/secureboot=%t"` — formatted, not concatenated. `TestInstallerImageCachesSecureBootSeparately` passes under `-race -count=3`. |
| 3 | 400/404 = refusal, everything else = no answer, **in both sites** | ✓ VERIFIED | One predicate: `registryRefused(status)` (`probe.go:95`) is called by `ProbeBuildable` (`probe.go:57`) *and* by `resolveInstallerRepo` (`installer.go:498`). The old `status == http.StatusNotFound` test is gone. |
| 4 | A 429 no longer marks a schematic not buildable | ✓ VERIFIED | 429 falls into `default` ⇒ `ErrUpstreamUnavailable`. `TestInstallerImageSeparatesAnUnreachableRegistryFromARefusal` and the probe status table pass. |
| 5 | `?secureboot=true` returns a SecureBoot ISO **and** a SecureBoot installer in one answer | ✓ VERIFIED | `TestAssetsSecureBootSuffixesTheURLs` asserts `metal-amd64-secureboot` in the ISO **and** `/metal-installer-secureboot/` in the installer, with a negative control that an ordinary request contains `secureboot` in neither. |
| 6 | Where neither SecureBoot name answers, refuse rather than substitute | ✓ VERIFIED | `TestInstallerImageRefusesRatherThanSubstitutingTheOrdinaryInstaller` passes; the candidate list structurally cannot contain a non-SecureBoot name when the flag is set. Merge-time acceptance of the resulting 502 is a human item. |
| 7 | The four-name matrix is a recorded observation or a ledger entry | ⚠ CORRECTED 2026-08-30 — was ✓ VERIFIED | **See the correction note below the table.** Original assessment, kept verbatim: *"Recorded in `02-09-SUMMARY.md:170-173` and in `live_test.go`'s expectation table; **independently reproduced live in this session**, agreeing cell for cell. The throttled first run is stated (`02-09-SUMMARY.md:182`), and `.planning/WINDOWS.md` entry 5 stays open."* Every fact in it is true. The reading it rests on is not: the criterion was conjunctive, and its third conjunct — the ledger entry — went unfiled. Filed 2026-08-30 as **WINDOWS entry 30**. |
| 8 | `DefaultTimeout`'s comment names every call it governs; value unchanged | ✓ VERIFIED | `client.go:26-42` now names the ISO HEAD and both installer manifest GETs, states the 30.5-32.7 s measured band, and points at `02-DECISION-probe-budget.md`. `git show 7f87cb1:…` vs now: `DefaultTimeout = 30 * time.Second` unchanged. |

> **Correction, 2026-08-30 (plan 02-21, closing G-02-21).** Row 7's truth was restated here as a
> disjunction — *"a recorded observation **or** a ledger entry"* — and marked verified because the
> observation exists. `02-09-PLAN.md:445-452` does not say *or*. On a throttle it requires three
> things **conjunctively**: that the SUMMARY record the throttle verbatim, **and** that WINDOWS
> entry 5 stay open, **and** that *"a new `unrun-verify` entry naming the unverified SecureBoot
> matrix is appended with `gsd-tools windows append --kind unrun-verify --phase 02`"*.
>
> The run throttled. **Two of the three were done. The third was not.**
>
> `02-09-SUMMARY.md:271` gave the reason: every ledger command was failing project-wide
> (`Ledger entry 7 has invalid kind: "review-finding"`), and the plan forbade hand-editing the
> file. That blockage was real — and it lasted seven minutes, between `9562498` (18:41:05) and
> `e1a5c6c` (18:48:09). Plans 02-11 through 02-13 appended entries 19–29 afterwards without
> trouble. So the obligation survived the blockage and was simply never picked back up, and this
> row then recorded the criterion as met on the softened reading rather than recording that it was
> outstanding. **A ship gate reading the register between 2026-08-29 and today saw nothing about
> the plan-09 throttle, nothing about the unprobed `talos.MaxSupportedVersion`, and nothing about
> the unprobed v1.13.x below the pin.**
>
> **Filed 2026-08-30 as WINDOWS entry 30** (`unrun-verify`, `internal/imagefactory/live_test.go`),
> naming all three. WINDOWS entry 5 — the factory.talos.dev throttle itself — remains open and is
> not closed by this. The original assessment above is corrected in place rather than deleted: that
> the criterion was *reread* rather than *met* is the whole of G-02-21, and a silent rewrite would
> repeat it.

#### 02-10 — G-02-7, and the first half of G-02-8

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Inspecting arm64 asset URLs does not change what the next schematic is created against | ✓ VERIFIED | `AssetPanel` holds `useState<Architecture>(archSeed)` (`images.tsx:1123`); only the form uses `useRememberedArch()` (`:135`). Web tests "does not let the asset panel rewrite the remembered architecture" and "creates the next schematic for the architecture the form shows, not the one just inspected" pass. |
| 2 | The panel's architecture is never written to localStorage | ✓ VERIFIED | The `localStorage` write lives only in `useRememberedArch`; the panel's state is seeded and discarded on unmount. Test "remembers the architecture rather than defaulting to the developer machine" pins the form half. |
| 3 | A schematic that is no longer stored shows a dialog that says so | ✓ VERIFIED | `record.isError` branch at `images.tsx:968` → `SchematicDetailUnavailable`. Test "says a schematic is no longer stored instead of opening an empty dialog" passes. |
| 4 | A detail fetch answering 404 removes the row | ✓ VERIFIED | `images.tsx:942` invalidates `['schematics']` guarded on `failure.code === 'notfound.schematic'` and a one-shot ref. |
| 5 | A store/transport failure is presented differently from a deletion | ✓ VERIFIED | `gone` is computed from the problem **code**, not the message (`:994`). Test "distinguishes a failed fetch from a deleted schematic and keeps the row" passes. |
| 6 | The second half of G-02-8 is 02-13's, not a silence | ✓ VERIFIED | 02-13 exists, carries `gap_ids: [G-02-8]`, and has executed. |

#### 02-11 — G-02-6

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | A locally refused value is a 400 naming the field, not a 502 | ✓ VERIFIED | `createProblem` does `errors.As(err, &refused)` on `*imagefactory.NotRepresentableError` **before** falling through to `factoryProblem` (`schematics.go:667-681`). `TestCreateRefusesALocallyUnrenderableValueAsAnInputProblem` and `TestCreateSplitsALocalRefusalFromAFactoryOutage` pass. |
| 2 | The refusal names which field and which entry | ✓ VERIFIED | `refusalReason` renders a one-based entry index; `requestFieldForPath` is an explicit table. `TestSchematicRefusalNamesTheFieldAndEntry`, `TestSchematicRefusalReportsTheFirstBadValueInDocumentOrder` pass. |
| 3 | A control character is refused at the input, before Create | ✓ VERIFIED | `hasControlCharacter` at `images.tsx:117-119`. Tests "refuses a kernel argument carrying a control character before any request", "…a META value…", "re-enables Create once the control character is removed" pass. |
| 4 | A create error disappears when the form changes | ✓ VERIFIED | Test "clears a create error as soon as the form it belongs to changes" passes. |

#### 02-12 — G-02-3 (plus the unconditional badge-copy and `break-all` items from G-02-1)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | A reference reached past an unanswered candidate carries a warning naming repo, version and transport error | ✓ VERIFIED | `installerFallbackWarning` (`installer.go:433-446`) interpolates all three. `TestInstallerImageWarnsWhenThePreferredNameWasNeverRuledOut` passes. |
| 2 | Cached only provisionally; warning on every answer including cached; re-questioned after the interval | ✓ VERIFIED | `installerRepoEntry.proven()`, `installerRepoRetryInterval = 5m`, and the `installerRepo` branch at `:246-251`. `TestInstallerImageServesAProvisionalAnswerFromTheCacheWithinTheInterval` and `TestInstallerImageReQuestionsAProvisionalAnswer` pass under `-race -count=3`. |
| 3 | A re-question asks only the never-ruled-out candidate | ✓ VERIFIED | `requestionInstallerRepo` passes `entry.unresolved...` to `resolveInstallerRepo`, never the full list. Asserted by a request counter in the concurrency test ("`installer` asked exactly once"). |
| 4 | The silent candidate's timeout is paid at most once per interval | ✓ VERIFIED | Follows from truths 2 and 3 and is what `budget_test.go`'s slack computation is against; the re-stamp on failure (`:417`) is what enforces the cadence. |
| 5 | A proven name carries no warning and never expires | ✓ VERIFIED | `TestInstallerImageProvenNameCarriesNoWarning` passes; `installerRepo` returns early on `entry.proven()`. |
| 6 | A failed re-question keeps and re-stamps the provisional entry | ✓ VERIFIED | `TestInstallerImageRetainsAProvisionalAnswerWhenTheReQuestionFails` passes. |
| 7 | A re-question whose preferred candidate refuses promotes the entry to proven | ✓ VERIFIED | The `errors.Is(err, ErrSchematicNotBuildable)` branch drops `unresolved` and the warning. `TestInstallerImagePromotesAProvisionalAnswerWhenTheReQuestionIsRefused` passes. |
| 8 | Budget composition is asserted by a route table, with the two over-budget routes declared | ✓ VERIFIED | `cmd/holzkubed/budget_test.go`: `TestRouteBudgetsComposeAgainstWriteTimeout` computes `calls × DefaultTimeout + slack` and fails a row whose declaration disagrees **in either direction**, and `TestRouteBudgetTableReadsTheRealConstants` fails if either constant moves. Both pass with no budget raised. |
| 9 | The operator sees the warning on the panel that shows the reference | ✓ VERIFIED | `assetReferences.Warnings` (`schematics.go:47`, populated at `:451`, `nil`-normalised to `[]`); `web/src/api.ts:245` requires the array. Tests "shows the operator what was not proven about the installer reference" and "renders no warning box when the installer repository name was proven" pass. |
| 10 | The repository name cannot be split across a line break | ✓ VERIFIED | `ReferenceValue` replaces `break-all` with `break-normal` plus `<wbr />` at `/` only (`images.tsx:1249-1265`); text content is unchanged. Test "cannot split a repository name across a line break" passes. |
| 11 | The badge no longer claims the probe did not run | ✓ VERIFIED | `images.tsx:747` reads "Not verified — the build probe has no verdict", with "The probe either did not run or did not answer in time." Test "renders three distinguishable usability states and the reason for a refusal" passes. |

#### 02-13 — G-02-8, second half

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | The record stores the architecture it was authored and probed against | ✓ VERIFIED | `model.Schematic.Arch string \`json:"arch"\`` (`model.go:98`); `createSchematic` stamps `Arch: in.Arch` (`schematics.go:278`) unconditionally. `TestCreateStoresTheArchitectureTheProbeUsed` and `TestUnansweredProbeStillStoresTheArchitecture` pass. |
| 2 | The verdict is shown with that architecture beside it, in all three places | ✓ VERIFIED | `UsabilityBadge` renders `architecture: {arch}` (`:701`) and is called with `arch` at the create result (`:650`), the saved list (`:877`) and the detail dialog (`:1053`). Tests at all three sites pass. |
| 3 | A pre-field record renders exactly 02-12's sentence, unqualified | ✓ VERIFIED | Test "leaves a record written before the architecture existed unqualified" passes; the Go drift guard `TestWarningDetailsMatchTheUI` still pins the transcribed sentences. |
| 4 | The stored architecture never defaults the assets query | ✓ VERIFIED | `TestTheStoredArchitectureDoesNotDefaultTheAssetsQuery` passes; `web/src/api.ts:434` always sends `arch`. |
| 5 | The records that can never be qualified are an open ledger entry | ✓ VERIFIED | `.planning/WINDOWS.md` entry 21, open, on `internal/model/model.go`. |

**Gap-closure score: 34/34 truths verified.**
**Combined score: 38/39 must-haves verified (5 ROADMAP SCs + 34 plan truths; 1 present,
behaviour-unverified).**

### Prohibition Checks (must-NOTs)

Each gap-closure plan's `must_haves.prohibitions` verified against the diff, not against prose.
All are judgment-tier; none is silently passed.

| Prohibition (plan) | Status | Evidence |
|--------------------|--------|----------|
| No installer repository name may be assembled or guessed (02-09) | ✓ HELD | Every returned name comes from a candidate that answered 2xx; `resolveInstallerRepo` returns an error rather than a name when none does. `TestInstallerImageRefusesWhenNeitherAnswers` passes. |
| No timeout, deadline or budget changed anywhere (02-09, 02-12, 02-13) | ✓ HELD | `git show 7f87cb1:internal/imagefactory/client.go` and `:cmd/holzkubed/main.go` vs the tree: `DefaultTimeout = 30 * time.Second` and `writeTimeout = 60 * time.Second`, both unchanged. 02-REVIEW-FIX.md's constraint audit over `e2db392..HEAD` reports `cmd/holzkubed/main.go` byte-identical; independently re-checked here. |
| Carve-out: `DefaultTimeout`'s doc comment only (02-09) | ✓ HELD | Comment rewritten, value untouched (above). |
| No probe state, no re-probe path, no change to `Usable`/`ProbedAt`/`ProbeReason` semantics (02-10, 02-12, 02-13) | ✓ HELD | The three fields' declarations and comments are unchanged from `7f87cb1`; `Arch` is additive and descriptive. `TestSchematicRoutesAreTheSevenContracted` passes, so no re-probe route was slipped in. |
| No store schema bump or migration (02-13) | ✓ HELD | `migrate.CurrentVersion = 2`, unchanged. |
| No architecture may be persisted in 02-10 (it is 02-13's) | ✓ HELD | `Arch` first appears in the 02-13 commits (`193b05e`), not in 02-10's two commits. |
| The refusal must never echo the offending value (02-11) | ✓ HELD | `refusalReason` emits an index and a class only. `TestSchematicRefusalDoesNotEchoTheValue` and `TestCreateRefusalDoesNotEchoTheOffendingValue` pass. |
| The canonical serialiser's refused set must not move (02-11, and again under WR-04) | ✓ HELD | WR-04 was fixed on the **client** side only; `representable`'s Go rule is untouched, which is what FACT-06 rests on — and FACT-06 is re-proven live ("the recorded payload still produces the recorded id"). |
| The fallback must not become an error (02-12) | ✓ HELD | The provisional path returns a reference plus a warning; only "no candidate answered at all" is an error. |
| The three badge sentences must not be rewritten (02-13) | ✓ HELD | `TestWarningDetailsMatchTheUI` passes; the architecture is rendered beside, not spliced in. |
| The stored architecture must not default anything (02-13) | ✓ HELD | Test named above. |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/imagefactory/installer.go` | SecureBoot-aware candidates and cache key; provisional caching with a bounded re-question; a never-demote-proven write | ✓ VERIFIED | 519 lines. `installerCandidates`, `installerRepoKey`, `installerRepoEntry.proven`, `storeInstallerRepo`, `requestionInstallerRepo`, `installerFallbackWarning` all present and substantive; wired from `InstallerImage`, which is called from `schematics.go:432`. |
| `internal/imagefactory/probe.go` | The package's single registry-answer taxonomy | ✓ VERIFIED | `registryRefused` defined here and called from both classification sites. |
| `internal/imagefactory/probe_test.go` | A status table asserting both functions agree | ✓ VERIFIED | Table tests present; `TestInstallerImageTreatsAnAllBadRequestCandidateSetAsARefusal` covers the 400 case the old code could not reach. |
| `internal/imagefactory/warnings.go` | `installer.repo-fallback-unverified` | ✓ VERIFIED | Constant present; `TestWarningsCodesAreNamespaced` now enumerates the package's exported codes by AST (WR-05 fix) instead of restating three names. |
| `internal/imagefactory/schematicid.go` | A typed refusal naming path and index | ✓ VERIFIED | `NotRepresentableError{Path, Index, Reason}` with `Unwrap` to `ErrSchematicNotRepresentable`. |
| `internal/httpapi/handlers/schematics.go` | `createProblem` split; assets response carrying `warnings`; `Arch` stamped | ✓ VERIFIED | All three present and wired; 7 contracted routes unchanged. |
| `cmd/holzkubed/budget_test.go` | The composition guard | ✓ VERIFIED | Present, reads the real constants, declares two `knownOverBudget` rows with `deferredTo` set, and fails a stale declaration in either direction. |
| `internal/model/model.go` | `Arch` on `Schematic` | ✓ VERIFIED | Additive, unversioned, documented; ledger entry 21 records the consequence. |
| `web/src/api.ts` | `arch` required on the record; `warnings` required on the assets response | ✓ VERIFIED | `arch: z.string()` (`:202`), `warnings: z.array(schematicWarningSchema)` on both the 201 body (`:218`) and the assets response (`:245`). |
| `web/src/routes/images.tsx` | Panel-owned arch, detail error branch, `ReferenceValue`, badge copy, control-char/surrogate refusal | ✓ VERIFIED | All present; 1 300+ lines; 36 tests in `images.test.tsx`, all passing. |
| `web/src/components/SchematicWarnings.tsx` | `predictWarnings` = the server's predicate | ✓ VERIFIED | WR-03 fix: the blank-row filter moved to `LiveSchematicWarnings`; the Go drift guard still passes. |
| `.planning/WINDOWS.md` | Entries for what this round could not close | ✓ VERIFIED | 21 entries, 19 open. Entry 20 (cold-path composition), entry 21 (unqualifiable legacy records) were added by this round. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `handlers/schematics.go` | `imagefactory/installer.go` | `AssetRequest.SecureBoot` reaches the repository name | ✓ WIRED | `?secureboot=` → `AssetRequest` → `installerCandidates`/`installerRepoKey`. End-to-end proof: `TestAssetsSecureBootSuffixesTheURLs` asserts the installer, not only the ISO. |
| `imagefactory/installer.go` | `imagefactory/probe.go` | one classifier for both | ✓ WIRED | `registryRefused` call sites: `probe.go:57`, `installer.go:498`. |
| `handlers/schematics.go` | `imagefactory/schematicid.go` | `errors.As` on the typed refusal | ✓ WIRED | `schematics.go:667`. |
| `web/src/routes/images.tsx` | `web/src/lib/problem.ts` | detail dialog discriminates on the problem **code** | ✓ WIRED | `images.tsx:942`, `:994` — `notfound.schematic`, never message text. |
| `imagefactory/installer.go` | `handlers/schematics.go` | `InstallerImage` returns warnings alongside the reference | ✓ WIRED | `schematics.go:432` destructures `(installer, warnings, err)`; `:451` puts them on the response. |
| `web/src/routes/images.tsx` | `web/src/api.ts` | the assets schema requires `warnings`; the badge reads `record.arch` | ✓ WIRED | A response without either field is a decode failure, not an empty render. |
| `handlers/schematics.go` | `internal/model/model.go` | `createSchematic` stamps `in.Arch` beside `ProbedAt` | ✓ WIRED | `schematics.go:278`, unconditional on the probe outcome. |

### Data-Flow Trace (Level 4)

| Artifact | Data variable | Source | Produces real data | Status |
|----------|---------------|--------|--------------------|--------|
| `images.tsx` asset panel | `assets.data` | `api.schematics.assets` → `GET /api/v1/schematics/{id}/assets` → `imagefactory` URL derivations + live registry resolution | Yes | ✓ FLOWING |
| `images.tsx` asset panel | `assets.data.warnings` | `InstallerImage`'s second return, `nil`-normalised server-side | Yes | ✓ FLOWING |
| `images.tsx` saved list / detail | `record.arch` | `store.Schematics()` record, stamped at create | Yes | ✓ FLOWING |
| `images.tsx` detail dialog | `record.error` | `ProblemError` from the real fetch | Yes | ✓ FLOWING |
| `images.tsx` create form | `catalog.data.extensions` | version-scoped Factory catalog; no fallback path | Yes | ✓ FLOWING |

No static returns, hardcoded literals or mock-terminated chains found in the rendered values of
this round.

### Behavioural Spot-Checks

| Behaviour | Command | Result | Status |
|-----------|---------|--------|--------|
| Whole tree compiles | `go build ./...` | exit 0 | ✓ PASS |
| Go suite | `go test ./... -count=1` | 18 packages ok, 0 failures | ✓ PASS |
| The five state-transition/concurrency invariants of this round | `go test ./internal/imagefactory -race -count=3 -run 'NeverRevertsAProven\|DoesNotReStamp\|ReQuestions\|Promotes\|Retains\|CachesSecureBoot\|RefusesRatherThanSubstituting'` | all PASS, 3/3 iterations, no race | ✓ PASS |
| Live Factory drift + SecureBoot split + name matrix | `HOLZKUBE_FACTORY_LIVE=1 go test ./internal/imagefactory -run TestLiveFactory` | PASS 32.1 s, 7/7 subtests | ✓ PASS |
| Web suite | `npm --prefix web run test -- --run` | 108 passed, 7 files | ✓ PASS |
| `--dry-run` reaches the binary | `go run ./cmd/holzkubed --help` | flag documented with its env var | ✓ PASS |
| Go lint | `golangci-lint run ./...` | **0 issues** | ✓ PASS |
| Web lint | `npm --prefix web run lint` | 47 files checked, clean | ✓ PASS |
| Typecheck | `tsc -p web/tsconfig.json --noEmit` | exit 0 | ✓ PASS |
| Route contract unchanged | `go test ./internal/httpapi/handlers -run TestSchematicRoutesAreTheSevenContracted` | ok | ✓ PASS |
| Budget composition | `go test ./cmd/holzkubed -run Budget` | both tests pass with no budget raised | ✓ PASS |

Note on `golangci-lint`: `.planning/WINDOWS.md` entries 6 and 19 record earlier runs that could
not execute it. It ran clean here with `~/go/bin` on PATH; both entries are already marked fixed.

### Probe Execution

No `scripts/*/tests/probe-*.sh` exist in this repository and no plan declares one. This project's
equivalent is the opt-in live drift guard, which was executed above rather than quoted.

### Requirements Coverage

Every ID in the ROADMAP's Phase 2 requirement list, cross-referenced against
`.planning/REQUIREMENTS.md` and against the plans that claim it.

| Requirement | Claimed by | Description | Status | Evidence |
|-------------|-----------|-------------|--------|----------|
| FOUND-12 | 02-07 | `--dry-run` for the whole binary; no mutation reaches a node | ✓ SATISFIED | `TestDryRunRefusesEveryMutationAtTheNode` (21 RPCs, sim counter 0); flag present in `--help`. Caveat on the record: the transport has no production caller yet, so the gate currently gates nothing (02-UAT.md, "Notes recorded, not gaps"). |
| TRANS-01 | 02-01, 02-05 | gRPC/mTLS directly to node IPs | ✓ SATISFIED | `TestTracerRealClientReachesFakeNode`. Product-facing half is Phase 3's. |
| TRANS-02 | 02-01 | `Dialer` + `DiscoverySource` seam | ✓ SATISFIED | `talos.go` interfaces; two implementations each; `TestSimulatorIsNotInTheProduct`. |
| TRANS-03 | 02-05 | Cluster vs maintenance clients are distinct types | ✓ SATISFIED | `TestMaintenanceClientRejectsClusterOnlyCall` — a real compile failure with a negative control. |
| TRANS-04 | 02-05 | Forced deadline; retries only for a read allowlist | ✓ SATISFIED | `TestRequireDeadline`, `TestRetryAllowlistIsExactlyTheFastReadClass`. |
| TRANS-05 | 02-05 | An unreachable node blocks neither the UI nor other nodes | ⚠️ PARTIAL | Transport half proven (`TestFanOutOneSilentNodeCostsOneNode`). UI half has no code path — deferred to Phase 3. REQUIREMENTS.md marks it `Complete`, which is ahead of observable behaviour. |
| TRANS-06 🚫 | 02-01, 02-03, 02-08 | `talossim` with real protobufs, mTLS, in-memory COSI | ✓ SATISFIED | Tracer test plus `coverage_test.go`'s AST-driven method-drift guard. Release blocker discharged. |
| TRANS-07 | 02-03 | Nine scriptable failure scenarios | ✓ SATISFIED | `TestScenarioContract` (9 subtests), `TestRegistryCoversTRANS07`. |
| TRANS-08 | — | Contract tests against fake **and** real Talos | ➖ NOT THIS PHASE | REQUIREMENTS.md maps it to Phase 3 (Pending). Not orphaned — deliberately relocated, with the reason in the ROADMAP Note. |
| FACT-01 | 02-02, 02-06, 02-11 | Version-scoped catalog, no free-text field | ✓ SATISFIED | Checkbox `<fieldset>` over the fetched catalog; web test "offers extensions only from the catalog and has no field to type one into". |
| FACT-02 | 02-02, 02-06, 02-09, 02-10, 02-13 | Names validated before the POST; usable only after the probe | ✓ SATISFIED | `ValidateExtensions` before the POST; `Usable` written only from `ProbeBuildable`; live "creation is still not validation" passes. Reachability of a *green* verdict for a novel tuple is G-02-1 (deferred). |
| FACT-03 | 02-04, 02-06, 02-09, 02-10, 02-12, 02-13 | Exact ISO/installer/PXE URLs, version-resolved repo name, no hardcoded arch | ✓ SATISFIED | Live URL derivations verified at `575da7a` and the installer half re-proven live here, now SecureBoot-correct. `?arch=` stays required and undefaulted. |
| FACT-04 | 02-04, 02-06, 02-12 | Kernel-args/META warning about `installer`/`initramfs` | ✓ SATISFIED | `Warnings()` plus the live-typing web tests; drift guard pins the three sentences. |
| FACT-05 | 02-04, 02-06 | Prereleases filtered and opt-in; broken versions greyed out | ✓ SATISFIED | Both web tests pass. `brokenVersions` is deliberately empty and documented as a finding; the greyed-out rendering has never run against a non-empty real list (recorded in 02-UAT.md). |
| FACT-06 | 02-02, 02-04, 02-11 | Schematic id precomputed locally and persisted | ✓ SATISFIED | Live "the recorded payload still produces the recorded id" passed in this session — the strongest available form of this claim. WR-04 was fixed client-side precisely so the serialiser's refused set stayed put. |

**Orphaned requirements: none.** All 14 IDs the ROADMAP assigns to Phase 2 are claimed by at least
one plan and accounted for above. The only Phase-2-adjacent ID not covered here is TRANS-08, which
`REQUIREMENTS.md` assigns to Phase 3.

### Anti-Patterns Found

Scanned the 22 non-planning files changed by `c1c65a2..HEAD`.

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `web/src/routes/images.tsx` | 81 | `XXX` | ℹ️ Info | Not a debt marker: the literal `\udXXX` in a comment explaining the surrogate mechanism. |
| `web/src/routes/images.test.tsx` | 551 | `XXX` | ℹ️ Info | Same, in the test's comment. |
| `.planning/ROADMAP.md` | 123 | Stale checkbox | ⚠️ Warning | `02-13-PLAN.md` is still `- [ ]` and the phase header still reads "12/13 plans executed", while `02-13-SUMMARY.md` exists, four commits landed (`f4d172b`…`0872da9`) and `STATE.md` records `stopped_at: Completed 02-13-PLAN.md` with 18/19 plans. Bookkeeping only — no code consequence — but the roadmap now understates what shipped. |

No `TBD`, `FIXME`, `TODO`, `HACK`, `PLACEHOLDER`, "not yet implemented" or "coming soon" markers
in any file this round touched. No empty-implementation or hollow-prop patterns found: every
rendered value traces to a real query (see the Level 4 trace).

### Human Verification Required

Six items, in `human_verification` above. Three are the deferred `<human-check>` blocks the
planner moved out of the executor's hands (02-09 task 3, 02-12 task 3, 02-13 task 2); one is the
merge-time acceptance 02-09-SUMMARY.md:279 hands to a human; one is the ledger policy call; one is
that **the /images route has never been opened outside jsdom since the gap-closure round**.

That last one deserves its weight. The previous UAT drove a real browser against the live Factory
and found seven defects in a phase whose suites were entirely green — because the suites use
zero-latency fakes and the failures were latency- and layout-shaped. Every fix in this round has a
test that fails against the pre-fix code, which is much stronger than the round that preceded it,
but the same structural blind spot is still there.

### Gaps Summary

**No gaps.** Every truth from every gap-closure plan is verified in the code, and the four ROADMAP
criteria that passed at `575da7a` still pass. This report is `human_needed` rather than `passed`
for three reasons, none of which is a defect in the delivered work:

1. **SC 3's UI clause remains behaviour-unverified.** Unchanged and unchangeable inside this phase:
   there is still no production caller of the transport seam. Deferred to Phase 3.
2. **Six human items are outstanding**, of which three are `<human-check>` blocks the plans
   deliberately deferred to end-of-phase, and one is a merge-time acceptance of a new 502 path.
3. **Cluster A is deliberately open.** G-02-1, G-02-2 and G-02-9 belong to
   `02-DECISION-probe-budget.md`, which is still open. This round was forbidden from touching any
   budget and did not: `DefaultTimeout` and `writeTimeout` are byte-identical to their pre-round
   values, no probe state was added, and no re-probe route was slipped in. The cost is declared in
   two places rather than hidden — `budget_test.go`'s `knownOverBudget` rows and `.planning/WINDOWS.md`
   entry 20.

Two things worth carrying forward, neither counted against the phase:

- **`REQUIREMENTS.md` is ahead of observable behaviour for TRANS-01 and TRANS-05**, both marked
  `Complete` while their product-facing half belongs to Phase 3. Same finding as the previous
  report; the table has not moved.
- **Seven review findings from the gap-closure round are unfixed and off the ledger** — WR-02,
  WR-06 and IN-01…IN-05 of `02-REVIEW.md`, left alone by explicit user scope choice. Leaving them
  unfixed is a decision; where they are recorded is not one yet. The identical situation for
  round 1 was resolved by entering eleven findings as ledger ids 8-18, and the ship gate cannot see
  what is not on the ledger.

What actually improved between `575da7a` and `ad786b5`: an operator asking for a SecureBoot ISO
used to be handed the ordinary installer — proven live here to be a genuinely different image —
and now is not. A registry rate-limit used to become a permanent accusation against a schematic,
and now does not. A value holzkube itself refused used to be blamed on the Image Factory with an
invitation to retry, and now names the field. A deleted schematic used to open a dialog whose
entire text was "Close". Reading one schematic's arm64 URLs used to silently change what every
future schematic was probed against, forever. And a proven installer repository could, until
`49be02f`, be reverted to an unproven one by a slower goroutine — a defect nothing in the phase
would have surfaced, found by review and now pinned by a test that forces the interleaving rather
than hoping for it.

---

_Verified: 2026-08-29T21:35:00Z_
_Verifier: Claude (gsd-verifier)_
