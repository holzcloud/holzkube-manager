---
phase: 17-versionspruefung-zwischen-plan-und-apply
verified: 2026-11-01T12:00:00Z
status: human_needed
score: 14/14 must-haves verified (against the simulator, jsdom and a stubbed API; nothing against a real node or a browser)
covered_files:
  - ".planning/phases/17-versionspruefung-zwischen-plan-und-apply/17-01-PLAN.md"
  - ".planning/phases/17-versionspruefung-zwischen-plan-und-apply/17-01-SUMMARY.md"
  - ".planning/phases/17-versionspruefung-zwischen-plan-und-apply/17-02-PLAN.md"
  - ".planning/phases/17-versionspruefung-zwischen-plan-und-apply/17-02-SUMMARY.md"
  - "README.md"
  - "cmd/holzkube-managerd/budget_test.go"
  - "docs/api-contract.md"
  - "docs/guide.md"
  - "internal/audit/redact.go"
  - "internal/audit/redact_test.go"
  - "internal/configdrift/apply_test.go"
  - "internal/configdrift/callers_test.go"
  - "internal/configdrift/expected_test.go"
  - "internal/configdrift/helpers_test.go"
  - "internal/configdrift/refused_apply_test.go"
  - "internal/httpapi/configapi_test.go"
  - "internal/httpapi/handlers/config.go"
  - "internal/httpapi/problem.go"
  - "internal/machineconfig/changedsinceplan_test.go"
  - "internal/machineconfig/service.go"
  - "internal/machineconfig/sinceplan.go"
  - "internal/machineconfig/sinceplan_test.go"
  - "web/src/api.test.ts"
  - "web/src/api.ts"
  - "web/src/lib/problem.ts"
  - "web/src/routes/config.test.tsx"
  - "web/src/routes/config.tsx"
covered_digest: "v4:sha256:1243af2437532ac5df2510dbe1e7558ceaf03d90e64975a0f365843089040fa7"
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "Against a real node, from the Pi: plan, wait briefly, apply without any change to the node"
    expected: "The apply goes through. A refusal here would refute assumption A3 (two readings of a real Talos node give the same snapshot digest) and would show up as the 'in its form: no setting differs that could be named' sentence"
    why_human: "talossim returns byte-identical readings; only a real Talos node shows whether its re-encoding is stable. Ledger 180 closes on this run"
  - test: "Against a real node: plan, change the node with talosctl in a document other than the first, apply"
    expected: "409 conflict.config-changed-since-plan, a sentence naming the path (for example KubeletConfig.<key> (modified)) and no value; the node keeps the foreign change and does not carry the patch; the page shows the yellow box, Apply is locked, 'See what would change again' plans again"
    why_human: "Needs a real node and a real writer; the simulator only simulates the second client (ledger 180)"
  - test: "Against a real node: plan, apply, then apply the same preview a second time"
    expected: "The second apply is refused (the first changed the digest) and the UI keeps the button locked after success"
    why_human: "Server side proven on talossim only; the real digest change after a real apply is not observed (ledger 180)"
  - test: "In a browser and on a phone: the yellow refusal box, the 'See what would change again' button, the locked Apply button"
    expected: "Readable, usable, no layout break"
    why_human: "No test covers layout; jsdom does not render CSS. No browser was involved in this phase"
---

# Phase 17: Versionspruefung zwischen Plan und Apply Verification Report

**Phase Goal:** Eine Konfiguration, die zwischen Plan und Apply von jemand anderem geaendert wurde, wird nicht still ueberschrieben: das Apply wird verweigert und sagt, was sich seitdem geaendert hat.
**Verified:** 2026-11-01
**Status:** human_needed
**Re-verification:** No, initial verification

## Where this verification ran

Mac (darwin/arm64), go1.27.3, Node with the project's `web/node_modules`. No real node, no browser, no Pi, no `go test -race` (CI only). Everything below that says VERIFIED means: proved by code reading plus tests I ran myself against the simulator (`talossim` with the real Talos client), jsdom, or a stubbed API. It does not mean proved against a real Talos node, and CLAUDE.md is explicit that this distinction has to be said.

## Goal Achievement

The code delivers the goal on the simulated node. A plan carries `base_digest`; an apply requires it, reads the node once, checks strictly before the single send, and on a mismatch answers 409 with a sentence naming changed paths over all documents and sends nothing. The residual claims that only a real node can settle are open by design (ledger 180, 181) and make the status `human_needed`, not `passed`.

### Observable Truths

Source: no `success_criteria` array in ROADMAP for this phase (goal line only), so the must-haves of the two PLAN files are the contract, plus the goal sentence itself.

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 0 | Goal: a foreign change between plan and apply is not silently overwritten, the apply is refused and says what changed | VERIFIED (simulator) | `service.go` `apply()`: Connect, `MachineConfigYAML`, `checkBase`, only then `ApplyPatches` and `ApplyConfigurationWithMode`. `TestAnApplyAfterAForeignChangeIsRefusedOverTheRoute` passes. Own fault injection: `checkBase` made to always return nil in a scratch worktree, `go test ./internal/machineconfig/`: 6 tests red (`TestAnApplyIsRefusedWhenTheNodeChangedSinceThePlan`, `TestTheRefusalSentenceNamesPathsAndNothingElse`, `TestTwoAppliesOnOneNodeDoNotOverwriteEachOther`, `TestARefusalStillHappensWhenThePlanIsNoLongerHeld`, `TestARefusedStagedApplyWithdrawsItsClaim`, `TestAnApplyWithoutADigestIsRefused`); worktree removed afterwards, main tree clean |
| 1 | Plan answers `base_digest` = Snapshot digest over all documents; two plans of an unchanged node equal, a change in a non-first document differs (VERS-01) | VERIFIED (simulator) | `Preview.BaseDigest` set from `Snapshot(base).Digest` in `Plan`; `TestThePlanRouteCarriesTheDigestOfWhatItRead` passes |
| 2 | Apply requires `base_digest`; missing or non-64-lower-hex is 400 `validation.failed` with field `base_digest`; `Service.Apply` itself refuses empty (VERS-02, D-01) | VERIFIED | `baseDigestProblem` in `handlers/config.go`, `ErrBaseDigestRequired` in `Apply`; `TestAnApplyWithoutAUsableDigestIsRefusedOverTheRoute`, `TestAnApplyWithoutADigestIsRefused` pass; field is a plain `string`, no omit path, no override switch anywhere (grep) |
| 3 | Foreign change gives 409 `conflict.config-changed-since-plan`, nothing sent, node keeps the foreign change (VERS-02) | VERIFIED (simulator) | `writeConfigError` maps `ErrChangedSincePlan` to `Conflict(CodeConfigChangedSincePlan, ...)`; code present in `problem.go`; route test asserts the node heard no `ApplyConfiguration` call and the patch did not land |
| 4 | Sentence names changed paths over all documents (incl. `KubeletConfig.*`), kind and list lengths, max eight then "and N more", no value/secret/address; still refuses with a general sentence when the earlier state is not held (VERS-03, D-02) | VERIFIED | `sinceplan.go` `ChangedSincePlanError.Error` uses `Path`, `Kind`, `LenBefore`, `LenAfter` only; `checkBase` uses `CompareDocuments`; `TestTheRefusalSentenceNamesPathsAndNothingElse`, `TestARefusalStillHappensWhenThePlanIsNoLongerHeld` pass |
| 5 | Unchanged node still lands; own apply refreshes the drift baseline via expected digest; refused apply leaves no expectation (VERS-04) | VERIFIED (simulator) | check sits before the observer in `ApplyConfigurationWithMode`; `TestAnApplyOnAnUnchangedNodeStillLandsOverTheRoute`, `TestARefusedApplyLeavesNoExpectationInTheBaseline` and the `configdrift` suite (ran, 25 s, ok) pass. Whether a real node reproduces its digest across two readings is A3, human item 1 |
| 6 | Two concurrent applies on one node do not overwrite each other; the wait ends with the request context (VERS-05) | VERIFIED (simulator) | `lockMachine`: one-slot channel per node, `select` on `ctx.Done()`; `TestLockMachine*`, `TestTwoAppliesOnOneNodeDoNotOverwriteEachOther`, `TestAnApplyWaitingForTheNodeGivesUpWithItsContext` pass. The HTTP mapping of `ErrApplyBusy` to 409 `store.conflict` exists in the handler but has no HTTP-level test (SUMMARY says so; I confirm the branch is only read, not exercised) |
| 7 | Secret-only change between plan and apply is not refused and is carried forward, not reverted (check and patch on the same raw bytes) (D-03) | VERIFIED | `apply()` patches `base` just read; documented as ledger 178 limit; covered by the node test the SUMMARY cites (F10), included in the green `machineconfig` run |
| 8 | Exactly one send site in `service.go`; `Compare`, `ModeFor`, `Snapshot`, `Redact` untouched; ledger 176 stays open | VERIFIED | `TestOnlyKnownCallersSendAConfigurationToAManagedNode` (in `internal/configdrift/callers_guard_test.go`) passes within the green `configdrift` run; `git diff b3c55ef -- diff.go applymode.go snapshot.go redact.go internal/compat internal/changelog web/fixtures docs/screenshots` is empty |
| 9 | UI sends the preview's `base_digest` with every apply; `previewSchema` requires the field (VERS-02) | VERIFIED (jsdom) | `config.tsx:255` `api.config.apply(..., preview.base_digest)`; `api.ts:3174` `base_digest: z.string()` (not optional); `vitest run --project jsdom src/routes/config.test.tsx src/api.test.ts`: 27 passed |
| 10 | On `conflict.config-changed-since-plan` the page shows the server's sentence in a yellow box, locks Apply, offers "See what would change again" which replans with the same patch text and sends the new digest afterwards (VERS-06) | VERIFIED (jsdom) | `config.tsx` lines 265, 341-351, 377; detection by code constant, not status. Own fault injection: Apply's `disabled` reduced to `apply.isPending` in a scratch worktree: 3 tests red (refusal locks Apply, sentence stays until the new plan, no second apply after success) |
| 11 | After a successful apply, Apply stays locked until a new plan | VERIFIED (jsdom) | `applied !== ''` in `disabled`, `key={planSeq}` resets state; "cannot apply the same preview a second time after it landed" passes and went red under my injection |
| 12 | Any other apply error stays a red line without lock or replan offer | VERIFIED (jsdom) | `onError` falls through to `setFailure(e.message)`; test in `config.test.tsx` (10 tests, all pass) |
| 13 | README entry "Configuration changes:", guide section "Applying a configuration patch", API contract section "The apply checks what the plan saw" with limits; changelog, fixtures, screenshots untouched | VERIFIED | Greps found all three texts; limits named with ledger numbers 176, 178, 181 (guide) and 180 referenced; untouched-files diff against BASE empty |
| 14 | Audit allowlist `config.apply` carries `base_digest` and no patch text; budget row describes one client for reading and sending; two open ledger entries 180 (unrun-verify) and 181 (gap) exist | VERIFIED | `redact.go:278` allowlist; `TestConfigApplyKeepsItsBaseDigestAndNothingOfThePatches` passes (audit suite ok); `cmd/holzkube-managerd` suite ok; `.planning/WINDOWS.md` holds entries 180 and 181, both status `open`, `open_count: 28`, `total_count: 181` (was 26/179 per 17-02-SUMMARY) |

**Score:** 14/14 truths verified at simulator/jsdom level (plus the goal row). Not one of them is proved against a real node.

Truths 3, 5 and 6 are state-transition/ordering invariants. They are not left on symbol presence: each has a passing behavioral test, and the check-before-send ordering and the lock were each made red by an injection (the SUMMARY's F1 to F15 list, plus my own `checkBase` injection above). So no truth is `PRESENT_BEHAVIOR_UNVERIFIED`. What stays open is a different thing: the simulator is not a real node.

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/machineconfig/sinceplan.go` | errors, sentence, bounded plan memo | VERIFIED | 3 sentinel errors, `ChangedSincePlanError`, `planMemo` (4 per node, 8 MiB, one hour); substantive, wired from `service.go` |
| `internal/machineconfig/service.go` | `Preview.BaseDigest`, `Apply(...baseDigest)`, `checkBase`, `lockMachine` | VERIFIED | all present and used |
| `internal/httpapi/handlers/config.go` | body field, 400/409 mapping | VERIFIED | `baseDigestProblem`, `writeConfigError` branches |
| `internal/httpapi/problem.go` | `conflict.config-changed-since-plan` | VERIFIED | `TestEveryProblemCodeIsInTheContract` and `...IsEmitted` pass |
| `internal/machineconfig/changedsinceplan_test.go`, `internal/configdrift/refused_apply_test.go` | node proofs, baseline proof | VERIFIED | run, green, seen red under injection |
| `web/src/routes/config.tsx`, `web/src/api.ts`, `web/src/lib/problem.ts`, `web/src/routes/config.test.tsx` | UI state, schema, code constant, proofs | VERIFIED | wired as above |
| `docs/guide.md`, `docs/api-contract.md`, `README.md` | texts in the same change | VERIFIED | contents read |
| `.planning/WINDOWS.md` | two open entries | VERIFIED | 180 and 181 present (the plan's own title string "VERSIONSPRUEFUNG ZWISCHEN PLAN UND APPLY" appears in 180 as "DIE VERSIONSPRUEFUNG ZWISCHEN PLAN UND APPLY IST NUR..."; entry 181 has different wording, which is fine) |

### Key Link Verification

| From | To | Via | Status |
|------|----|-----|--------|
| `handlers/config.go` applyConfig | `Service.Apply` | `d.Config.Apply(ctx, id, patches, mode, body.BaseDigest)` | WIRED |
| `Plan` / `checkBase` | `Snapshot`, `CompareDocuments` | direct calls | WIRED |
| `checkBase` | `ApplyConfigurationWithMode` | strictly before, one client | WIRED (order proven by F4 injection in SUMMARY and by the route test) |
| `writeConfigError` | `CodeConfigChangedSincePlan` | `errors.Is(err, ErrChangedSincePlan)` | WIRED |
| `PreviewCard` | `api.config.apply` | `preview.base_digest` | WIRED |
| `PreviewCard.onError` | `CODE_CONFIG_CHANGED_SINCE_PLAN` | code comparison | WIRED |
| `redact.go` allowlist | `config.apply` | `base_digest` entry | WIRED |

### Data-Flow Trace (Level 4)

The digest flows from a real read: `Plan` -> `read` -> `Snapshot(base).Digest` -> JSON `base_digest` -> `previewSchema` -> `preview.base_digest` -> apply body -> `baseDigestProblem` -> `Service.Apply` -> `checkBase` against a fresh `MachineConfigYAML`. No static or hardcoded digest anywhere in the chain. The refusal sentence flows from `CompareDocuments(before, snap.Text)` to `detail` of the problem to `e.message` to the alert. FLOWING.

### Behavioral Spot-Checks (run by me)

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| machineconfig, configdrift, audit, cmd/holzkube-managerd, handlers suites | `go test ... -count=1` | all `ok` | PASS |
| `internal/talos`, `internal/planning` | `go test -count=1` | `ok` | PASS |
| Route tests `TestThePlanRoute...`, `TestAnApply*`, `TestEveryProblemCode*` | `go test ./internal/httpapi/ -run ... -v` | all PASS, no SKIP | PASS |
| Web config page and api | `vitest run --project jsdom src/routes/config.test.tsx src/api.test.ts` | 27 passed | PASS |
| `go build ./...` | | exit 0 | PASS |
| Full `internal/httpapi` package | `go test ./internal/httpapi/ -count=1` | one FAIL, `TestNoAssetIsInlinedPastTheContentSecurityPolicy` | Pre-existing, see below |
| `internal/publicrepo` | `go test -count=1` | FAIL on `TestNoIdentifierOfTheRealInstallationIsInTheHistory` | Pre-existing, see below |

### Probe Execution

SKIPPED: the phase declares no probe scripts and none are in its PLAN files.

### Requirements Coverage

All six IDs are declared in the PLAN frontmatter (17-01: VERS-01 to 05, 17-02: VERS-02 and VERS-06) and in REQUIREMENTS.md mapped to Phase 17. No orphaned requirement.

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| VERS-01 | 17-01 | Plan carries `base_digest` of the snapshot | SATISFIED (simulator) | truth 1 |
| VERS-02 | 17-01, 17-02 | Apply requires it, rereads, refuses on mismatch, no override, sends nothing | SATISFIED (simulator); real-node behavior human items 1 to 2 | truths 2, 3, 9 |
| VERS-03 | 17-01 | Sentence names changes over all documents, no secrets, values, addresses | SATISFIED | truth 4 |
| VERS-04 | 17-01 | Own apply lands and refreshes drift baseline; refused leaves no expectation | SATISFIED (simulator); A3 open | truth 5 |
| VERS-05 | 17-01 | Per-node lock for concurrent applies from this process | SATISFIED (simulator); HTTP `store.conflict` mapping not exercised | truth 6 |
| VERS-06 | 17-02 | UI shows sentence, offers replan; README, guide, API contract in the same step | SATISFIED in jsdom; layout needs a browser (human item 4) | truths 10, 11, 13 |

Housekeeping, not a goal gap: `REQUIREMENTS.md` still shows all six as `[ ]` / "Pending", and ROADMAP.md still shows "1/2 plans executed" with `17-02-PLAN.md` unchecked, although 17-02 is complete (STATUS: COMPLETE in its SUMMARY). Both plans explicitly leave the ticking to this verification. I did not edit them (instruction: no source edits; these are planning files for the orchestrator). Since the status is `human_needed`, the orchestrator should decide whether to tick them now or after the Pi run.

### Anti-Patterns Found

Scan of the added lines in the phase's non-test source files (`sinceplan.go`, `service.go`, `handlers/config.go`, `problem.go`, `config.tsx`, `api.ts`, `lib/problem.ts`, `redact.go`) for `TBD|FIXME|XXX|TODO|HACK|PLACEHOLDER`: none. No stubs, no static returns, no hollow props found in the chain.

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/httpapi/handlers/config.go` | 412 | `ErrApplyBusy` to 409 `store.conflict` has no HTTP-level test | Warning | Branch is trivially readable and the service-level proof exists; the 30 s route budget cannot be shortened in a test. Admitted in 17-01-SUMMARY |

### Facts measured by the caller, checked

- **17-UAT.md records five checks run against talossim and jsdom only.** Confirmed: all five have `source: automated`; its own closing note says no browser and no real node; its evidence names tests that exist and pass (`TestAnApplyOnAnUnchangedNodeStillLandsOverTheRoute`, `TestAnApplyAfterAForeignChangeIsRefusedOverTheRoute`, `TestTheRefusalSentenceNamesPathsAndNothingElse`, `TestTwoAppliesOnOneNodeDoNotOverwriteEachOther`, `TestConfigApplyKeepsItsBaseDigestAndNothingOfThePatches`, the jsdom tests). The UAT is therefore not evidence of real-node behavior and I did not count it as such.
- **Ledger 180 (unrun-verify) and 181 (gap) open by design.** Confirmed in `.planning/WINDOWS.md`, status `open`, counts 28/181. Their closing conditions are exactly the human items above.
- **`TestNoAssetIsInlinedPastTheContentSecurityPolicy` fails because of a stale git-ignored build.** Confirmed: `internal/httpapi/dist/*` is ignored by `.gitignore:8`, tracked is only `.gitkeep`, the assets in it are dated Sep 3 13:30, and the failing assertion names `assets/index-CYukU3kf.css` inlining a `data:` font. No web file of this phase feeds that build, and the phase's own route tests pass when selected. Not caused by this phase and not counted as a gap. A web rebuild would clear it.
- **`TestNoIdentifierOfTheRealInstallationIsInTheHistory` (additional, not in the caller's list but in both SUMMARYs).** I extracted the commit ids it names (23) and checked each against `HEAD`: none is an ancestor of `HEAD`; the first one I traced lives only on branch `pre-public-rewrite`. The tree half of the guard passes. Not caused by this phase; the operator decides about the local branches (deferred-items.md).

### Public repository check

Phase additions use documentation values only per the SUMMARYs; the `publicrepo` tree half passes. This report names no address, host or mail address.

### Human Verification Required

#### 1. Unchanged real node still applies (ledger 180, A3)

**Test:** On the Pi, against a real node: plan, wait briefly, apply without changing the node.
**Expected:** The apply goes through. A refusal here, with the sentence about "its form: no setting differs that could be named", would refute A3 (stable re-encoding of a real Talos).
**Why human:** talossim is byte-stable; only a real Talos node can show otherwise.

#### 2. Foreign change on a real node is refused (ledger 180)

**Test:** Plan, change a document other than the first with talosctl, apply.
**Expected:** 409 `conflict.config-changed-since-plan`, sentence with the path and no value, node keeps the foreign change and does not carry the patch; yellow box, Apply locked, "See what would change again" replans.
**Why human:** needs a real node and a real second writer.

#### 3. Same preview twice after a success (ledger 180)

**Test:** Plan, apply, apply the same preview again.
**Expected:** Refused; the button is locked after success.
**Why human:** real digest movement after a real apply is not observed.

#### 4. Layout in a browser and on a phone

**Test:** Look at the yellow box, the replan button and the locked Apply button.
**Expected:** Readable and usable.
**Why human:** no test covers layout; no browser was involved.

### Gaps Summary

No gaps. Nothing failed. The phase is code-complete and proved as far as an environment without a real node can prove it, including two fault injections I ran myself (server check skipped: 6 tests red; UI lock removed: 3 tests red, both restored by removing the scratch worktree, main tree untouched). The status is `human_needed` because the claims that matter most operationally, A3 (stable digest across two real readings) and the real-node refusal, are unproven by design and tracked as open ledger entries 180 and 181. Entry 181 (no precondition field in the Talos apply RPC) cannot be closed by this product at all: the window between reading and sending is shortened, not closed, and the documents say so.

---

_Verified: 2026-11-01_
_Verifier: Claude (gsd-verifier)_
