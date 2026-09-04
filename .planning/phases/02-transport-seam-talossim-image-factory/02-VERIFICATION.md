---
phase: 02-transport-seam-talossim-image-factory
verified: 2026-09-04T04:23:14Z
verified_at_commit: b6e954b
status: gaps_found
score: 15/19 must-haves verified
behavior_unverified: 1
overrides_applied: 0
re_verification:
  round: 4
  previous_status: gaps_found
  previous_score: 18/19
  previous_verified_at_commit: fb16546
  gaps_closed:
    - "R3-3 — 02-21-SUMMARY.md's Issues Encountered #2 is struck through and marked WITHDRAWN with a dated correction block; web/vite.config.ts:113 still declares both projects and web/package.json still declares test:browser, so the correction is accurate at HEAD"
    - "R3-1 (one half of two) — canonical_live_test.go's reportCanonResults now ends in `if !t.Failed() && counts[notObserved] > 0 { t.Skipf(...) }` (canonical_live_test.go:667-670), placed after the error loop so a real DIVERGES row still fails. The half that closed is the one round 3 named."
  gaps_remaining:
    - "R3-1 generalised — the truth round 3 stated was about guards, not about one file. Falsified at HEAD in a different guard: see gap G4-2."
  regressions:
    - "SC 4 / FACT-02 — plan 02-24's `refreshTheStoredVerdict` writes a probe verdict across Talos versions. Reproduced at HEAD, not inferred: see gap G4-1. This is new in round 4 and did not exist at fb16546."
  adversarial_checks_run:
    - "CR-01 REPRODUCED, not adjudicated on the reviewer's word. Widened the fake's catalog gate to two versions, authored `siderolabs/cross-version-ext` at v1.12.0 where the image endpoint refuses it (record: talos_version v1.12.0, usable=false, probe_reason `... at v1.12.0/amd64 answered HTTP 400`), stopped the refusal and re-POSTed the identical customisation at v1.13.9. Result: same id, HTTP 409, and the stored record became `talos_version: v1.12.0, usable: true, probe_reason: \"\"`, rev 1 -> 2. The true v1.12.0 refusal was ERASED, which is worse than 02-REVIEW.md described. Tree restored byte-for-byte (git status clean apart from the pre-existing STATE.md edit)."
    - "WR-04 FALSIFIED. Renamed `REFUSED_RANGES` to `RENAMED_BY_VERIFIER` throughout web/src/routes/images.tsx and re-ran TestBrowserRefusalSetEqualsTheServers: `ok github.com/holzcloud/holzkube-manager/internal/imagefactory 0.546s`. The guard passed green against a declaration that no longer exists, which is precisely the property its own t.Fatalf comment claims it has (\"A guard that silently passes when it can no longer find what it guards is worse than no guard\"). File restored."
    - "Parsed .planning/WINDOWS.md's three representations independently: frontmatter 52 open / 0 waived / 13 fixed / 65 total; markdown table 65 rows, 52 open + 13 fixed; JSON block 65 entries with the same statuses. Zero status mismatches, ids 1..65 with no duplicates."
    - "Ran the eight TestConflict* cases, TestRouteBudget*, TestProbeBudget*, TestBudgetWaitsStatedInTheUIMatchTheRouteBudgets and TestInstallerImageNeverRevertsAProvenNameUnderConcurrentResolution individually rather than trusting the suite-level exit code. All pass."
    - "Enumerated the handlers package's test list: there is no TestConflictAtAnotherTalosVersion* of any spelling, and createBody hardcodes catalogVersion. The reviewer's \"nothing catches this\" is measured, not asserted."
gaps:
  - truth: "Das Schematic gilt erst nach einem bestätigenden Model-Build-Probe als brauchbar (ROADMAP SC 4 / FACT-02) — and no stored verdict claims more than the probe measured"
    status: failed
    reason: >-
      `refreshTheStoredVerdict` (schematics.go:513-587) guards two things and needs three. It
      returns early when the fresh probe did not answer, and when `stored.Arch != fresh.Arch`.
      There is no condition on `TalosVersion`, and there has to be, because `Schematic.Canonical()`
      (schematicid.go:125-178) emits `owner`, `overlay` and `customization` and nothing else — no
      architecture AND no Talos version — so two authoring attempts differing only in
      `talos_version` are one record and collide on `store.ErrConflict`. The handler's own comment
      at :433-436 says so ("regardless of name, cluster or the version they were authored
      against"). The verdict, meanwhile, IS version-scoped on exactly the evidence the Arch guard
      cites for itself: `ProbeBuildable(ctx, id, talosVersion, arch)` probes that version's ISO
      URL, the extension catalog is version-scoped, and `probe.go:71` writes
      `<id> at <version>/<arch> answered HTTP <status>` — version and architecture in one sentence.
      `isTalosVersion` is a shape check only, so any well-formed version reaches this path.
      Reproduced at HEAD rather than inferred (see adversarial_checks_run): the record ends as
      `talos_version: v1.12.0, usable: true, probe_reason: ""` after a probe that succeeded only at
      v1.13.9 — and the correct v1.12.0 refusal sentence, the single place the discrepancy was
      visible, is erased by the same write. `UsabilityVerdict` (images.tsx:942) then renders
      **"Usable — the build probe confirmed it"** about a version at which the probe did nothing of
      the kind. That is the claim T-02-62 and G-02-1 exist to prevent, arriving through the
      mitigation built for G-02-9, and it is producible through the supported UI in two POSTs.
      Nothing in .planning/WINDOWS.md records it: entry 58 enumerates the two declining cases and
      does not mention the version.
    artifacts:
      - path: "internal/httpapi/handlers/schematics.go:513-587"
        issue: "the guard beside line 554 covers Arch and not TalosVersion; the write at :551-553 proceeds"
      - path: "internal/httpapi/handlers/schematics_test.go:483-494"
        issue: "createBody hardcodes catalogVersion, so no test varies the version across a conflict; TestConflictRefreshTouchesNothingButTheThreeProbeFields deliberately excludes the three fields at issue"
      - path: "internal/model/model.go:105-108"
        issue: "the doc comment says a second POST 'refuses the label, the cluster and the Talos version exactly as before' — true of the stored FIELD, false of the verdict now written onto it"
    missing:
      - "A third condition in refreshTheStoredVerdict: `if stored.TalosVersion != fresh.TalosVersion` returns the declining clause naming both versions, in the same register archMismatchReason already uses"
      - "TestConflictAtAnotherTalosVersionDeclinesTheRefresh beside TestConflictAtAnotherArchitectureDeclinesTheRefresh, asserting marshalRecord(after) == marshalRecord(before) and that the 409 detail names both versions. It needs the fake's catalog gate to serve a second version — one line, and its absence is exactly why this shipped"
      - "If a cross-version refresh is instead wanted, that is a schema change (model.Schematic has room for one verdict) and belongs in 02-DECISION-schematic-identity.md, not in a guard"
  - truth: "A guard reports a pass only when it measured the property it is named for"
    status: partial
    reason: >-
      The half round 3 named is closed: canonical_live_test.go:667-670 now skips a run in which any
      row went unmeasured, after the error loop, so a fully-throttled differential is no longer a
      pass. The truth as round 3 stated it was general, and it is falsified at HEAD in the sibling
      guard written in that same round. `browserRefusalRange` (guard_drift_test.go:49-50) is
      `\{\s*from:\s*0x([0-9a-fA-F]+),\s*to:\s*0x([0-9a-fA-F]+)` run over the whole of images.tsx,
      anchored to no identifier. Measured: renaming `REFUSED_RANGES` out of existence leaves
      TestBrowserRefusalSetEqualsTheServers green (`ok ... 0.546s`), while `browserRefusalRanges`'s
      own t.Fatalf comment promises "A guard that silently passes when it can no longer find what
      it guards is worse than no guard". The pollution direction is open too — any other
      `{from: 0x.., to: 0x..}` literal added to images.tsx is folded into the set this test
      believes the browser refuses, which masks an under-refusal rather than reporting it. The
      discipline exists three files away: budget_drift_test.go:89-90 anchors on
      `^\s*(?:export\s+)?const\s+NAME\s*=` and says why. Nothing is currently masked — the two
      sets do agree, and deleting the U+FEFF entry still fails the guard — so this is about the
      guard's future behaviour, which is the same standing round 3 gave the canonical half.
    artifacts:
      - path: "internal/imagefactory/guard_drift_test.go:49-50, used at :170-196"
        issue: "the entry regex scans the whole file instead of the REFUSED_RANGES declaration; the empty-result Fatalf cannot fire while any such literal survives anywhere"
    missing:
      - "Cut the declaration out first (`(?s)const\\s+REFUSED_RANGES\\s*:[^=]*=\\s*\\[(.*?)\\]`), Fatalf when it is absent, and scan only inside it — the shape stringArrayLiteral and exportedWarningCodes already use"
      - "Fail on an entry inside the declaration that this regex cannot parse (a decimal literal or a named constant), rather than skipping it silently"
      - "Or, if the current anchoring is deliberate, record it in .planning/WINDOWS.md beside entry 56 so the register carries it"
deferred:
  - truth: "Ein nicht erreichbarer Node blockiert die UI nicht (the UI half of ROADMAP SC 3 / TRANS-05)"
    addressed_in: "Phase 3"
    evidence: >-
      Re-checked at b6e954b. There is still no production caller of NewClusterClient,
      NewMaintenanceClient, FanOut, NewBreaker, NewDirectDialer or NewManualSource outside
      internal/talos and its tests. The transport half is proven behaviourally
      (TestFanOutOneSilentNodeCostsOneNode, TestFanOutCancellationTerminatesEveryInFlightCall,
      TestFanOutSkipsAnOpenCircuitWithoutDialing all present and passing).
  - truth: "Dieselbe Contract-Suite läuft auch gegen echtes Talos (TRANS-08)"
    addressed_in: "Phase 3"
    evidence: "REQUIREMENTS.md:226 maps TRANS-08 to Phase 3 (Pending); ROADMAP.md:181 states why. internal/talos/contract_test.go already parameterises the transport."
  - truth: "G-02-9 — a probe that times out despite the raised budget is still permanent"
    addressed_in: "02-DECISION-probe-budget.md Option 1, deliberately not taken"
    evidence: >-
      Not a gap: it is recorded honestly rather than claimed closed, which is what round 4 was
      asked to verify. 02-24-PLAN.md's frontmatter carries `gap_ids: [G-02-1]` with G-02-9 under a
      separate `gaps_mitigated_not_closed` key so a machine reading frontmatter gets the same
      answer as a prose reader; WINDOWS entry 58 opens "G-02-9 REMAINS OPEN AFTER ROUND 4" and
      names the three missing things (no route, no button, no job); the decision document's own
      status header says "G-02-9 is **not** closed by this choice"; 02-24-SUMMARY.md:236 has a
      section titled "G-02-9 is not closed". No artifact of the round claims otherwise.
behavior_unverified_items:
  - truth: "Ein nicht erreichbarer Node blockiert weder die UI noch andere Nodes (ROADMAP SC 3)"
    test: >-
      Once Phase 3 wires a route to the transport: open the inventory with one node injected with
      go_silent(90s) and confirm the page renders the healthy nodes immediately and the silent one
      as unreachable, rather than the page waiting on the silent node's budget.
    expected: >-
      The UI paints healthy nodes within one round trip; the silent node resolves to an error after
      its own per-node budget only.
    why_human: >-
      Unchanged and re-checked at b6e954b: there is still no production caller of the transport
      seam, so the UI half of the criterion has no code path to exercise. The transport half IS
      proven (TestFanOutOneSilentNodeCostsOneNode).
coincidental_reliance_items:
  - truth: "The four installer repository names occupy one line box (G-02-10)"
    reason: incidental-ordering
    harden: >-
      Carried forward unchanged. The sweep is Chromium-only (playwright, headless, 1200x900).
      UAX #14's hyphen rule and `white-space: nowrap` are not a corner of CSS where engines
      disagree, so the inference to Firefox and WebKit is sound — but it is an inference. Already
      declared: WINDOWS entry 42. Advisory only.
  - truth: "The browser's request ceiling sits above the server's outermost response bound (02-23)"
    reason: fixture-only
    harden: >-
      web/src/api.test.ts:87-101 asserts the ceiling is `> 130_000` — a second hand-transcription
      of `writeTimeout` on the TypeScript side, so the test moves with neither the Go constant nor
      REQUEST_CEILING_MS. Raising writeTimeout to 160s leaves both the constant and this assertion
      green while every long create aborts at 150s. See warning WR-03 below; advisory only, it
      changes no status and no score.
human_verification:
  - test: >-
      Drive the assembled binary through a browser against the live Image Factory: author a
      schematic end to end, and confirm the /images route works outside a test harness
      (02-UAT.md test 5, sub-checks a-d).
    expected: "The route behaves as the suites predict, with real latency and a real bundle."
    why_human: >-
      Still outstanding at b6e954b and correctly scoped rather than claimed closed. 02-UAT.md test
      5 carries `result: issue`. `images.browser.test.tsx` opens ImagesView in a real Chromium but
      its own doc comment says "no binary runs, no bundle is served, and the embedded UI has still
      never been driven end to end in a browser."
  - test: >-
      Confirm the CI browser-install step runs on ubuntu-latest.
    expected: "`npm --prefix web exec -- playwright install --with-deps chromium` (.github/workflows/ci.yml:71) succeeds and the browser project runs in CI."
    why_human: >-
      WINDOWS entry 44 is still open at this commit. The browser project is the sole evidence for
      G-02-10 and G-02-19. It fails closed — requireBrowserBinary() throws with the fixing command
      — so the risk is a red CI run, not a silent pass.
  - test: >-
      Re-measure a cold installer resolution against factory.talos.dev after plan 02-23's
      concurrent fan-out.
    expected: "A cold resolution costs the slowest single candidate, not the sum — the improvement 02-23 claims."
    why_human: >-
      02-24-SUMMARY.md:232 says so itself: the improvement is measured offline against fakes and
      unmeasured against factory.talos.dev. 02-22's two live runs measured the ISO probe, not the
      installer resolution. Nothing here claims otherwise; the item exists because the claim is
      offline-only and the phase's own standard is that a live number is a live number.
  - test: >-
      Decide whether the eleven findings of 02-REVIEW.md that are not gaps (WR-01, WR-02, WR-03,
      WR-05, IN-01..IN-05) should be filed in .planning/WINDOWS.md.
    expected: "Each is either fixed, filed as a window with `gsd-tools windows append`, or explicitly declined."
    why_human: >-
      A policy call, not a measurement. The review's own scope note lists the findings it did NOT
      re-file because they are already in the ledger; the eleven above are new and are in no
      representation of it. This phase's convention is that an accepted defect lives in the
      register, and `workflow.windows_enforce` reads that register at ship time.
---

# Phase 2 Verification — round 4 (the probe-budget cluster)

**Phase Goal:** Jede Talos-Interaktion läuft durch eine austauschbare Naht und ist ohne Hardware
testbar; Schematics und Image-URLs sind korrekt und nachweislich brauchbar herleitbar.
**Verified:** 2026-09-04T04:23:14Z at `b6e954b` (branch `main`; working tree carries one
uncommitted bookkeeping edit to `.planning/STATE.md` and no source change)
**Status:** gaps_found — two gaps, one of them a regression introduced by this round
**Re-verification:** Yes — round 4, over plans 02-22, 02-23, 02-24, superseding the round-3 report
written at `fb16546`.

Round 3 left three `partial` gaps and a deferred probe-budget cluster. Two of the three are closed.
The third is closed in the file round 3 named and false in a sibling guard, measured here rather
than read. The ratified decision was implemented faithfully and G-02-9 is recorded honestly
throughout — and the mitigation built for it introduced a new instance of the exact claim this
phase has spent four rounds correcting: a badge that asserts more than was measured.

Nothing in this report is carried over from round 3 without being re-measured at `b6e954b`.

## 1. Observable truths

| # | Truth | Status | Evidence at `b6e954b` |
|---|---|---|---|
| 1 | SC 1 — the **unchanged** production client speaks to `talossim`: real protobufs, real mTLS, real in-memory COSI, no hardware, no `talosctl`, no network | ✓ VERIFIED | `internal/talossim/` intact (16 files); `TestTracerRealClientReachesFakeNode`, `TestTracerRefusesUnverifiableClientCertificate`, `TestTracerRefusesMaintenanceCredentialsOnTheClusterPath` all present and in a passing package. `internal/depguard_test.go:30,102` still pins the module boundary D-07 declares. Untouched by round 4 apart from the repo rename. |
| 2 | SC 2 — the operator switches on nine failure scenarios and the client behaves definitely | ✓ VERIFIED | `internal/talossim/scenario.go:49-57` declares all nine by name (`GoSilent`, `RejectApply`, `SecondBootstrap`, `FlapConnection`, `SlowLogConsumer`, `IPChangesOnReboot`, `EtcdDown`, `K8sDown`, `VersionOutOfSupportedRange`), each with a documented expectation; `TestScenarioContract` and `TestGoSilentFailsAtItsOwnClassDeadline` present and passing. Unchanged this round. |
| 3 | SC 3 — an unreachable node blocks neither the UI nor other nodes; every call has a forced deadline; retries only for a read allowlist; cluster and maintenance clients are distinct types | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | The transport half is behaviourally proven and re-checked: `TestFanOutOneSilentNodeCostsOneNode`, `TestFanOutCancellationTerminatesEveryInFlightCall`, `TestFanOutSkipsAnOpenCircuitWithoutDialing`, `TestRequireDeadline`, `TestWithClassDeadlineRefusesAnUnclassifiedMethod`, `TestRetryAllowlistIsExactlyTheFastReadClass`, `TestMaintenanceClientRejectsClusterOnlyCall`, `TestMaintenanceClientMethodSetIsClosed`. The **UI** half has no code path: no production caller of the seam exists outside `internal/talos` and its tests. Deferred to Phase 3; see behavior_unverified_items. |
| 4 | SC 4 — the operator assembles a schematic from a version-scoped catalog, gets the exact ISO/installer/PXE URLs (version-dependent repo name, no hardcoded architecture), **and the schematic counts as usable only after a confirming model-build probe**; kernel args or META raise the installer/initramfs warning | ✗ FAILED | Every conjunct but one holds and was re-checked: the catalog is version-scoped and there is no free-text field; `ISOURL`/`InstallerImage`/PXE derive from `AssetRequest` with the architecture as a parameter; `resolveInstallerRepo` resolves the version-dependent repo name and now asks all candidates at once while still deciding in declared order; the kernel-args/META warning is emitted and mirrored in the UI. The last conjunct is false at HEAD — **reproduced**, see gap G4-1: a re-POST at a second Talos version writes `usable: true` onto a record that names the first, and erases the refusal that named it. |
| 5 | SC 5 — the whole binary runs with `--dry-run` and no mutation reaches a node | ✓ VERIFIED | `internal/talos/dryrun.go` with six named tests present and passing (`TestDryRunRefusesEveryMutationAtTheNode`, `TestDryRunOffLetsTheSameMutationsThrough`, `TestDryRunLeavesReadsAndStreamsAlone`, `TestDryRunRefusesApplyConfigurationInMaintenanceMode`, `TestDryRunRefusalNamesTheRPCAndTheWayOut`, `TestDryRunRefusalIsNotATransportFailure`), plus `TestDryRunApplyChangesNothing` in talossim. Wired at the composition root: `cmd/holzkube-managerd/main.go:171` `talos.Mode{DryRun: cfg.DryRun}`. Unchanged this round. |
| 6 | R3 carry-over — a guard reports a pass only when it measured the property it is named for | ✗ FAILED (partial) | The named half is closed: `reportCanonResults` ends in `if !t.Failed() && counts[notObserved] > 0 { t.Skipf(...) }` (`canonical_live_test.go:667-670`), placed after the error loop so a DIVERGES row still fails, with the reasoning and the `live_test.go:425-431` precedent written in place. The truth as stated is general, and **falsified at HEAD in the other guard of that round**: renaming `REFUSED_RANGES` away leaves `TestBrowserRefusalSetEqualsTheServers` green. See gap G4-2. |
| 7 | R3 carry-over — a blocking human decision checkpoint resolved without a human says so where the decision is read | ✓ VERIFIED | `02-DECISION-schematic-identity.md:130-148` now carries a titled section, *How this decision was taken*, opening "Recorded for provenance, because the status line alone would misrepresent it" and ending **"This decision is therefore self-resolved, not ratified."** with a pointer to `02-21-SUMMARY.md:165-181` and the in-phase contrast (`02-UAT.md` test 4's `ratified_by: user`). Round 3's exact finding — the artifact "contains no occurrence of 'checkpoint', 'ratified', 'user' or any equivalent" — is measurably false now (grep hits at :134, :142, :146). One residual observation, not a gap: the header at :3 still reads `Status: **decided**` unqualified, while the sibling `02-DECISION-probe-budget.md:3-6` adopted the header convention in the same round *and cross-references this document by name*. The fix landed one line lower than asked for. |
| 8 | R3 carry-over — a completion record states only what is true of the repository | ✓ VERIFIED | `02-21-SUMMARY.md:345-358`: the claim is struck through, headed "**— WITHDRAWN, this was false**", and followed by a dated correction block that names `web/vite.config.ts:113`, both project names, `web/package.json`'s `test:browser`, the measurement, and why it mattered ("a reader acting on the original sentence would … delete the only layout evidence G-02-10 and G-02-19 have"). Verified accurate at HEAD, not just present: `vite.config.ts:113-138` still declares `projects:` with `jsdom` and `browser`, `package.json:11-12` still declares both scripts. The given HEAD measurement (9 files / 137 tests under the bare command) is consistent with the correction and larger than round 3's 8/126 because 02-23 added `api.test.ts`. |
| 9 | 02-22 — the ISO probe and the registry manifest GET each have a constant of their own, and no request reaches the wire on a context with no deadline | ✓ VERIFIED | `client.go:36,62,78` — `DefaultTimeout = 30s`, `ProbeTimeout = 90s`, `ManifestTimeout = 30s`, each with its derivation in place. `client.go:254-262` refuses before the wire: `if _, ok := ctx.Deadline(); !ok { return ErrNoDeadline }`, with `ErrNoDeadline` documented as "Nothing was sent." `TestEachBudgetBoundsItsOwnWorkloadAndNoOther` and `TestRequestWithNoDeadlineIsRefusedBeforeTheWire` present and passing. |
| 10 | 02-22 — a probe past the JSON budget and inside the probe budget produces a verdict; one past the probe budget still produces none, and no invented refusal | ✓ VERIFIED | Run individually: `TestProbeBudgetOutlivesTheJSONBudget` PASS (1.68s), `TestProbeBudgetStillEndsInNoVerdictWhenItIsExceeded` PASS (0.95s), `TestProbeBudgetIsNotTheManifestBudget` PASS (0.65s). The fail-safe tri-state in `probe.go:65-77` is intact — `registryRefused` still gates `ErrSchematicNotBuildable` and everything else is `ErrUpstreamUnavailable`. |
| 11 | 02-22 — both Factory routes carry one route deadline, `writeTimeout` covers the largest with slack, and the composition guard sums per-call budgets by class from the code that runs | ✓ VERIFIED | `schematics.go:58` `CreateRouteBudget = ProbeTimeout + DefaultTimeout` (120s) applied at :366; `:83` `AssetsRouteBudget = ManifestTimeout + 5s` (35s) applied at :720; `main.go:64` `writeTimeout = 130s`. `TestRouteBudgetsComposeAgainstWriteTimeout` and `TestRouteBudgetTableReadsTheRealConstants` PASS; the latter fails if `writeTimeout != 130s` (`budget_test.go:377`). Every shipped value matches the decision document's own "constants as shipped" table exactly. |
| 12 | 02-22 — the probe budget is derived by a stated rule from measured cold observations, and `TestLiveFactory` bounds the elapsed time of the probe it runs | ✓ VERIFIED | The derivation and its observations are in `client.go:62`'s comment; the live guard re-applies the rule rather than restating the number. Opt-in and unrun in CI, which is recorded rather than hidden: WINDOWS entry 64 supersedes entry 5 and says which three of entry 5's claims are unchanged and which two moved. |
| 13 | 02-23 — the installer candidates are asked at the same time, the first candidate in **declared** order that answered 2xx still wins, the G-02-3 provenance survives, and the errors are byte-identical to the serial walk's | ✓ VERIFIED | `installer.go:613-707`: one slot per candidate indexed by declared position, `close(done[i])` as the happens-before, cancel-then-wait so no goroutine outlives the call, and the return taken on the first 2xx **in the iteration order of `candidates`**, not on arrival. `unresolved` and `unanswered` are still accumulated and still drive the provisional caching and the fallback warning. `TestInstallerImageAsksEveryCandidateAtOnce` and `TestInstallerImageNeverRevertsAProvenNameUnderConcurrentResolution` (cold cache and stale provisional entry) PASS under `-race`. |
| 14 | 02-23 — the composition table's assets row records one concurrent candidate budget, and the clipping ratchet goes red if the constant is tightened without the declared call list changing | ✓ VERIFIED | `AssetsRouteBudget` is one `ManifestTimeout` plus 5s and the guard recomputes from it. The half-change the truth explicitly declines to claim (relisting to one call while the constant stays at two budgets) is recorded as a known limitation in WINDOWS entry 65, in the same words the plan's own must-have used. A truth that states only the direction its arithmetic delivers is the correct shape and it does deliver it. |
| 15 | 02-23 — every browser request carries a ceiling, and the two waits the UI names equal the route budgets the server enforces, held equal by a drift guard | ✓ VERIFIED | `api.ts:509` hands `AbortSignal.timeout(REQUEST_CEILING_MS)` to every `fetch`; `api.test.ts` has five cases including a fresh ceiling for the sudo replay and "does not dress the abort up as a server problem". `budget_drift_test.go` reads `CREATE_WAIT_SECONDS` and `ASSETS_WAIT_SECONDS` out of `images.tsx` by anchored regex and compares them to the Go constants; `TestBudgetWaitsStatedInTheUIMatchTheRouteBudgets` PASS. (The third constant, `REQUEST_CEILING_MS`, is unguarded — warning WR-03, not a truth failure.) |
| 16 | 02-24 — the 409 refresh writes `Usable`, `ProbedAt`, `ProbeReason` and nothing else, only when the fresh probe answered and only when the stored architecture matches, under a compare-and-swap that never retries and never recreates | ✓ VERIFIED | All eight cases run individually and PASS: `TestConflictRefreshesTheVerdictItJustComputed`, `…StoresTheFactorysRefusal`, `TestConflictWithNoAnswerFromTheProbeChangesNothing`, `…TouchesNothingButTheThreeProbeFields` (whole-record comparison plus `rev == before+1`), `TestConflictAtAnotherArchitectureDeclinesTheRefresh`, `…LosingTheCompareAndSwapAnswersThePlainConflict`, `…AgainstADeletedRecordDoesNotRecreateIt`, `TestConflictDetailNamesWhichRefreshOutcomeHappened`. The truth as the plan stated it is exactly true. It is also exactly as wide as the plan stated it, which is the whole of gap G4-1. |
| 17 | 02-24 — the 409 body says which outcome happened, the saved list refetches after a failed create, and the no-verdict badge names the recovery without promising a verdict | ✓ VERIFIED | Three distinct clauses in `refreshTheStoredVerdict` (:530, :551, :575, :578) plus `archMismatchReason` naming both architectures. `images.tsx:302-311` invalidates `['schematics']` on create failure with the reason written in place ("un-refetched would show the operator the stale badge"). `images.tsx:932-938` reads *Not verified — the build probe has no verdict* with the muted line naming the re-submission, that it updates the verdict in place, and that it is still answered as a conflict — and the comment at :925-929 says why it stops there. |
| 18 | 02-24 — the ledger says precisely what this round moved and that G-02-9 stays open | ✓ VERIFIED | WINDOWS entry 57 opens "SUPERSEDES ENTRIES 20 AND 48, BOTH NOW MARKED FIXED (no amend verb exists)" and enumerates what moved; entry 58 opens "G-02-9 REMAINS OPEN AFTER ROUND 4" and names the three missing things; entries 59-65 carry the residuals including the ones that cut against the round (process-wide `writeTimeout`, `CreateRouteBudget` clipping, the un-surfaced audit outcome of a refreshing 409, the still-missing progress indicator, the guard's tolerated half-change). Entry 63 supersedes entry 8 and carries the reason the `fixed` verb cannot. Three representations parsed independently: 52/0/13/65 in all three, zero mismatches, ids 1..65 with no duplicates. |
| 19 | The phase's own standing truth — no stored state claims more than the record supports (T-02-62 / G-02-1 / G-02-8 lineage) | ✗ FAILED | Reproduced at HEAD: `talos_version: v1.12.0, usable: true, probe_reason: ""` after a probe that succeeded only at v1.13.9, rendering **"Usable — the build probe confirmed it"**. Same root cause as truth 4; see gap G4-1. |

**Score:** 15/19 truths verified (1 present, behavior-unverified; 3 failed, of which two share one root cause).

## 2. Deferred items

| # | Item | Addressed In | Evidence |
|---|---|---|---|
| 1 | The UI half of SC 3 / TRANS-05 | Phase 3 | No production caller of the seam exists; ROADMAP Phase 3 owns the inventory route. |
| 2 | TRANS-08 — the contract suite against real Talos | Phase 3 | REQUIREMENTS.md:226 (Pending); ROADMAP.md:181 states why. |
| 3 | G-02-9 — a timed-out probe is still permanent | 02-DECISION-probe-budget.md Option 1, not taken | Recorded open in four independent places; no artifact claims it closed. **This is the item round 4 was asked to check for dishonesty and it is honest.** |

## 3. Required artifacts

| Artifact | Expected | Status | Details |
|---|---|---|---|
| `internal/imagefactory/client.go` | three budgets, the classes, the no-deadline refusal | ✓ VERIFIED | :36, :62, :78, :228-262. Each constant carries the rule that produced it. |
| `internal/imagefactory/probe.go` | version- and arch-scoped probe, fail-safe tri-state | ✓ VERIFIED | :23 signature takes `talosVersion`; :71 writes `<id> at <version>/<arch>`; :65-77 tri-state intact. |
| `internal/imagefactory/installer.go` | concurrent fan-out deciding in declared order | ✓ VERIFIED | :613-707. Race-clean, ordering-correct. |
| `internal/httpapi/handlers/schematics.go` | route deadlines and the conflict refresh | ⚠️ HOLLOW | :366 and :720 wire the deadlines correctly; :513-587 refreshes a record on a guard set that is one condition short. Present, substantive, wired — and writes a claim the data does not support. |
| `internal/httpapi/handlers/budget_drift_test.go` | Go-reads-TypeScript wait guard | ✓ VERIFIED | :41 `uiPath`, anchored constant regex, both waits compared to the Go constants. |
| `cmd/holzkube-managerd/budget_test.go` | composition guard over the route table | ✓ VERIFIED | Sums by class from the real constants; :377 pins `writeTimeout`. |
| `cmd/holzkube-managerd/main.go` | `writeTimeout` covering the largest route budget | ✓ VERIFIED | :64 = 130s. |
| `web/src/api.ts` | a ceiling on every request | ✓ VERIFIED | :475, :509, :530. |
| `web/src/routes/images.tsx` | recovery copy and post-failure refetch | ✓ VERIFIED | :302-311, :932-938. |
| `internal/model/model.go` | the narrowed statements about a second POST | ⚠️ PARTIAL | :105-115 is accurate about the *fields* and reads as reassurance about the *verdict*. It should name the version condition once it exists. |
| `.planning/WINDOWS.md` | entries 57-65 | ✓ VERIFIED | Three representations in agreement; supersessions explicit; nothing claimed closed that is not. |
| `02-DECISION-probe-budget.md` | ratified text unaltered, implementation record appended | ✓ VERIFIED | The append is marked as written after ratification and states the hash of the preserved prefix. Every constant in its "as shipped" table matches the code. |
| `internal/imagefactory/canonical_live_test.go` | a guard that does not pass on nothing | ✓ VERIFIED | :667-670. |
| `internal/imagefactory/guard_drift_test.go` | a guard anchored to what it guards | ✗ STUB (as a guard) | :49-50 is unanchored; falsified by renaming the declaration. The comparison it performs is correct; the anchoring it claims is not. |

## 4. Key link verification

| From | To | Via | Status | Details |
|---|---|---|---|---|
| `cmd/holzkube-managerd/budget_test.go` | `internal/imagefactory/client.go` | imports the budget constants rather than re-declaring them | ✓ WIRED | `imagefactory.ProbeTimeout` read directly; moving one changes the verdict. |
| `internal/httpapi/handlers/schematics.go` | `internal/imagefactory/client.go` | route deadline wraps `r.Context()` before `Author` | ✓ WIRED | :366, and `context.WithTimeout` takes the earlier of the two, so it is a ceiling and never a floor. |
| `internal/imagefactory/installer.go` | `internal/imagefactory/client.go` | every concurrent candidate issued under `classManifest` and the caller's deadline | ✓ WIRED | `fanCtx` derived from `ctx`; `probeStatus(fanCtx, …, classManifest)`. |
| `internal/httpapi/handlers/budget_drift_test.go` | `web/src/routes/images.tsx` | Go test reads the TypeScript literal, anchored on the declaration | ✓ WIRED | Guard passes and is anchored. |
| `web/src/api.ts` | `cmd/holzkube-managerd/main.go` | `REQUEST_CEILING_MS` transcribes `writeTimeout` | ✗ NOT WIRED | Third transcription of the phase and the only one with no guard. Warning WR-03. |
| `internal/httpapi/handlers/schematics.go` | `internal/store/store.go` | refresh reads for `Rev` and writes with it | ✓ WIRED | `Schematics().Get` then `Put`; CAS loss and deleted-record cases both tested. |
| `web/src/routes/images.tsx` | `internal/httpapi/handlers/schematics.go` | the operator reads the server's own conflict detail | ✓ WIRED | `create.error` surfaces `detail` verbatim, which is how my reproduction read the refresh sentence off the wire. |
| `internal/imagefactory/guard_drift_test.go` | `web/src/routes/images.tsx` | Go test reads `REFUSED_RANGES` | ✗ NOT WIRED | It reads the *file*, not the declaration. Measured. |

## 5. Behavioural spot-checks

| Behaviour | Command | Result | Status |
|---|---|---|---|
| Cross-version conflict refresh (CR-01) | temporary test in `handlers_test` + widened fake catalog gate | record became `talos_version: v1.12.0, usable: true, probe_reason: ""` (rev 1→2); 409 detail read "this schematic builds, so the stored verdict was refreshed" | ✗ FAIL — defect reproduced |
| Refusal-set guard anchoring (WR-04) | rename `REFUSED_RANGES` → `RENAMED_BY_VERIFIER`, `go test -run TestBrowserRefusalSetEqualsTheServers` | `ok … 0.546s` | ✗ FAIL — guard green against a declaration that does not exist |
| The eight conflict-refresh cases | `go test ./internal/httpapi/handlers/ -run TestConflict -v` | 8/8 PASS, 11.755s | ✓ PASS |
| Composition guard | `go test ./cmd/holzkube-managerd/ -run TestRouteBudget -v` | 2/2 PASS | ✓ PASS |
| Probe-budget separation | `go test ./internal/httpapi/handlers/ -run 'TestBudgetWaits…\|TestProbeBudget' -v` | 4/4 PASS | ✓ PASS |
| Concurrent installer resolution | `go test ./internal/imagefactory/ -run 'TestResolveInstaller\|Concurrent' -v` | PASS incl. both subtests | ✓ PASS |
| Test existence: a cross-version conflict test | `go test ./internal/httpapi/handlers/ -list '.*' \| grep -i version` | no match; `createBody` hardcodes `catalogVersion` | ✗ FAIL — no such test exists |
| Ledger self-consistency | independent parse of frontmatter, table and JSON | 52/0/13/65 in all three, 0 mismatches | ✓ PASS |
| Whole Go suite / web suite | given at HEAD | `go test ./... -count=1 -race` exit 0 (19 packages); `npm --prefix web run test` exit 0, 9 files / 137 tests | ✓ PASS — not contradicted by anything measured here |

## 6. Requirements coverage

| Requirement | Source plans | Status | Evidence |
|---|---|---|---|
| FOUND-12 | 02-07 | ✓ SATISFIED | `internal/talos/dryrun.go` + six tests; `main.go:171` composition root; banner in the UI. |
| TRANS-01 | 02-01 | ✓ SATISFIED | Real machinery client over the `Dialer` seam with real mTLS; tracer test. |
| TRANS-02 | 02-01 | ✓ SATISFIED | `Dialer` and `DiscoverySource` each with a second implementation (`dial_direct.go`, `discovery_manual.go`, `talossim/dialer.go`, `talossim/discovery.go`). |
| TRANS-03 | 02-05 | ✓ SATISFIED | Distinct types; `TestMaintenanceClientRejectsClusterOnlyCall`, `TestMaintenanceClientMethodSetIsClosed`, `clusteronly_fixture.go`. |
| TRANS-04 | 02-05 | ✓ SATISFIED | `TestRequireDeadline`, `TestWithClassDeadlineRefusesAnUnclassifiedMethod`, `TestRetryAllowlistIsExactlyTheFastReadClass`. |
| TRANS-05 | 02-05 | ⚠️ PARTIAL | Transport half proven; UI half has no caller — Phase 3. |
| TRANS-06 🚫 | 02-01, 02-08 | ✓ SATISFIED | `internal/talossim` with in-memory COSI, the three streams and a method-drift guard. |
| TRANS-07 | 02-03 | ✓ SATISFIED | Nine scenarios declared and contract-tested. |
| TRANS-08 | — | ⏭ DEFERRED | Phase 3 (REQUIREMENTS.md:226, ROADMAP.md:181). |
| FACT-01 | 02-02, 02-06 | ✓ SATISFIED | Version-scoped catalog, no free-text field. |
| FACT-02 | 02-02, 02-22, 02-24 | ✗ BLOCKED | Pre-POST validation holds. "gilt erst als brauchbar, nachdem ein Model-Build-Probe es bestätigt hat" does not: gap G4-1 produces `usable: true` for a version no probe confirmed. |
| FACT-03 | 02-04, 02-09, 02-23 | ✓ SATISFIED | Exact ISO/installer/PXE URLs, version-resolved repo name, architecture a parameter. |
| FACT-04 | 02-04, 02-06 | ✓ SATISFIED | The installer/initramfs warning is emitted, mirrored in `api.ts` and contracted. |
| FACT-05 | 02-04 | ✓ SATISFIED | Prerelease filtered structurally; broken versions curated. |
| FACT-06 | 02-02, 02-14, 02-24 | ✓ SATISFIED | Id precomputed locally and persisted; `Canonical()` measured against the live Factory by an external oracle. |

**Orphaned requirements:** none. All fourteen IDs the phase declares appear in at least one plan's `requirements` frontmatter, and REQUIREMENTS.md maps no additional ID to Phase 2.

## 7. Anti-patterns

| File | Line | Pattern | Severity | Impact |
|---|---|---|---|---|
| — | — | `TBD` / `FIXME` / `XXX` | — | **None** in any file this round modified. |
| — | — | `TODO` / `HACK` / `PLACEHOLDER` | — | **None** in any file this round modified. |
| `internal/httpapi/handlers/schematics.go` | 513-587 | a guard set that is one condition short of the claim it writes | 🛑 Blocker | Gap G4-1. |
| `internal/imagefactory/guard_drift_test.go` | 49-50 | a drift guard not anchored to its subject | ⚠️ Warning | Gap G4-2. |
| `internal/httpapi/handlers/schematics.go` | 942-944 | `strconv.ParseBool` error discarded — `secureboot=yes` served as an ordinary request | ⚠️ Warning | 02-REVIEW WR-01. The one route whose own comments (`:691-695`, `docs/api-contract.md:735-741`) call a SecureBoot substitution undetectable forever; `arch`, `version` and `platform` all 400 on a value they do not understand. Not filed in WINDOWS. |
| `internal/imagefactory/installer.go` | 420-431 | the `ErrSchematicNotBuildable` branch mints a **proven** entry from a negative observation, with the warning cleared and no expiry | ⚠️ Warning | 02-REVIEW WR-02. Pre-existing (plan 02-12), made more reachable by 02-23's concurrency. WINDOWS entry 22 records the single-flight half; the negative-observation half is not in the ledger. |
| `web/src/api.ts` | 453-475 | `REQUEST_CEILING_MS` transcribes `writeTimeout` with no guard, while its two siblings have one | ⚠️ Warning | 02-REVIEW WR-03, and worse than reported: `api.test.ts:98` asserts `> 130_000`, a *second* hand-transcription, so raising `writeTimeout` to 160s leaves both green. |
| `cmd/holzkube-managerd/main.go` | 288-306 | `allowedHosts`'s DNS-rebinding rationale is attached to `ssoOnly` | ⚠️ Warning | 02-REVIEW WR-05. Security-relevant justification on the wrong symbol; `allowedHosts` left undocumented. Introduced by the out-of-phase SSO work, not by 02-22/23/24. |
| `web/src/lib/problem.ts` | 89-109 | a list that calls itself "the full closed taxonomy" and omits the `upstream.` family | ℹ️ Info | 02-REVIEW IN-01. Behaviour correct; the claim is not. |
| `web/src/api.ts` | 512-517 | any `AbortError` reported as the 150-second ceiling firing | ℹ️ Info | 02-REVIEW IN-02. |
| `web/src/test/problem-fixtures.ts` | 15 | a second unguarded transcription of `ProblemBaseURI`, producing no red test when stale | ℹ️ Info | 02-REVIEW IN-03; WINDOWS entry 46 records the sibling and not this file. |
| `web/src/routes/images.tsx` / `schematics.go` | 379-393, 795-799 | a whitespace-only schematic name passes both guards | ℹ️ Info | 02-REVIEW IN-04. |
| `internal/httpapi/handlers/schematics.go` | 713-719 | a budget comment still describing the serial candidate walk in the present tense, contradicting `AssetsRouteBudget`'s own comment one screen above | ℹ️ Info | 02-REVIEW IN-05. Exactly the "fact with an expiry date that nothing in the build checks" this file deleted its digest literals over. |

**Test quality audit.** No skipped or disabled test is the sole evidence for any requirement: `grep -rn "t.Skip\|it.skip\|describe.skip\|test.todo"` over the phase's suites finds only the opt-in live guards (`live_test.go`, `canonical_live_test.go`), each recorded in WINDOWS (entries 5/64 and 35). Expected-value provenance for the FACT-06 differential is **external and only external** — `canonical_live_test.go` compares against `Created.Canonical` and `Created.ID` from factory.talos.dev, and the file says why a second local YAML library would not do. Assertion strength is value- or behaviour-level throughout the round-4 tests (whole-record marshalling, `rev` arithmetic, elapsed-time bounds, race-detected ordering). One coverage hole, and it is the gap: the conflict-refresh table has seven cases and no eighth for the version.

## 8. Decision coverage

`02-CONTEXT.md` declares D-01 … D-10. Nine are named in at least one SUMMARY. **D-07** (`talossim` lives in `internal/talossim`, module boundary extended) is named in no SUMMARY but is honoured in code and enforced: `internal/depguard_test.go:30` declares `simulatorPackage = rootModule + "/internal/talossim"` and `:102` pins it in the boundary table. Non-blocking; recorded for drift-spotting only.

## 9. Human verification required

### 1. Drive the assembled binary through a browser against the live Image Factory

**Test:** 02-UAT.md test 5, sub-checks (a)-(d).
**Expected:** the route behaves as the suites predict, with real latency and a real bundle.
**Why human:** 02-UAT.md test 5 still carries `result: issue`. The browser project opens ImagesView in Chromium but its own doc comment says no binary runs and no bundle is served.

### 2. Confirm the CI browser-install step runs on ubuntu-latest

**Test:** a CI run reaching `.github/workflows/ci.yml:71`.
**Expected:** `playwright install --with-deps chromium` succeeds and the browser project runs.
**Why human:** WINDOWS entry 44 is still open. Fails closed, so the risk is a red run, not a silent pass.

### 3. Re-measure a cold installer resolution against factory.talos.dev

**Test:** one cold assets request against the live Factory after 02-23's fan-out.
**Expected:** the cold cost is the slowest single candidate, not the sum.
**Why human:** 02-24-SUMMARY.md:232 states plainly that the improvement is measured offline and unmeasured live. Believed, not doubted — but it is the phase's own standard that a live number must be a live number.

### 4. Decide where 02-REVIEW.md's eleven non-gap findings live

**Test:** triage WR-01, WR-02, WR-03, WR-05 and IN-01..IN-05 into fixed / filed / declined.
**Expected:** each is in `.planning/WINDOWS.md` or explicitly declined.
**Why human:** a policy call. None of the eleven appears in any representation of the ledger, and `workflow.windows_enforce` reads that ledger at ship time.

## 10. Gaps summary

**Round 4 delivered what it was asked to deliver, and broke one thing doing it.**

The ratified decision was implemented faithfully. Every constant in `02-DECISION-probe-budget.md`'s
"constants as shipped" table matches the code exactly; the composition guard sums by class from the
constants that run rather than multiplying one by a call count; the fan-out is genuinely
concurrent, genuinely order-preserving and race-clean; the browser has a ceiling; and G-02-9 is
recorded as open in the plan frontmatter, the ledger, the decision document and the SUMMARY, in
four consistent voices with no artifact dissenting. Two of round 3's three record-integrity gaps
are closed on evidence measured here.

The two gaps are both of the same species, which is why this phase keeps finding them: **a claim
that is wider than the measurement behind it.**

**G4-1** is the serious one, and it is a regression the round introduced. The 409 refresh — built
to stop discarding a verdict — writes a verdict across Talos versions, because the identity of a
schematic record cannot vary by version any more than it can by architecture, and only the
architecture was guarded. I reproduced it rather than reasoning about it, and it is worse than the
code review described: the refresh does not merely add a wrong `usable: true`, it *erases* the
correct refusal sentence that was the only evidence of the disagreement. An operator sees
"Usable — the build probe confirmed it" for a version at which the Factory answered HTTP 400
seconds earlier. Two POSTs through the supported UI produce it. The fix is fifteen lines beside a
guard that already exists and already argues the case for itself; the test is a copy of
`TestConflictAtAnotherArchitectureDeclinesTheRefresh` plus one line in the fake.

**G4-2** is smaller and older. Round 3's truth was "a guard reports a pass only when it measured the
property it is named for", and the file it named was fixed — but the sibling guard from that same
round scans `images.tsx` for `{from: 0x.., to: 0x..}` with no anchor to `REFUSED_RANGES`, so it
passes green against a declaration that has been renamed away. Its own `t.Fatalf` comment states
the property it does not have. Nothing is masked today; this is about what the guard will notice
tomorrow, which is the same standing round 3 gave the canonical half and the same fix shape:
anchor on the declaration, the way `budget_drift_test.go` already does three files away.

Neither gap touches the transport seam, `talossim`, the nine scenarios, `--dry-run` or the URL
derivation. Success criteria 1, 2 and 5 hold; 3 holds on its transport half with its UI half
correctly deferred to Phase 3; 4 holds on every conjunct but the one G4-1 breaks.

---

_Verified: 2026-09-04T04:23:14Z at `b6e954b`_
_Verifier: Claude (gsd-verifier) — round 4_
