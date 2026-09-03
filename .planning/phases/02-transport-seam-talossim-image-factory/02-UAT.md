---
status: complete
phase: 02-transport-seam-talossim-image-factory
source: [02-VERIFICATION.md]
round: 2
started: 2026-08-29T22:10:00Z
updated: 2026-08-30T12:15:00Z
previous_round:
  status: complete
  total: 2
  passed: 1
  issues: 1
  gaps_diagnosed: 9
  outcome: >-
    Nine gaps in three clusters. B and C (G-02-3 through G-02-8) were closed by plans 02-09
    through 02-13. Cluster A (G-02-1, G-02-2, G-02-9) is deferred to the still-open
    02-DECISION-probe-budget.md. Round 1's own test records are in git history at commit
    47ca6e5.
---

## Current Test

number: 4
name: Accept or refuse the new SecureBoot 502 path (T-02-53)
expected: |
  A recorded merge-time acceptance, or an instruction to substitute after all.
awaiting: user response

## Tests

### 1. The installer row and the Usability badge at a narrow window width

deferred_from: 02-12-PLAN.md task 3 `<human-check>`

steps: |
  Start the binary, open Images, open a saved schematic, and narrow the window until the asset
  panel is cramped. Read the installer reference row and the Usability badge.
expected: |
  The repository name is unbroken — `metal-installer` never wraps into something readable as
  `installer` — and the badge claims only what the record supports ("Not verified — the build
  probe has no verdict", never "the build probe did not run").
why_human: |
  Line breaking is a rendering fact. The 108 web tests run under jsdom, which lays nothing out.
  ReferenceValue's `<wbr />`-only strategy is asserted structurally, but the property it buys is
  visual.
result: issue
reported: "Adversarial re-check (2026-08-30): refuted at high confidence."
severity: major
correction: |
  This was recorded as `pass` earlier in this same session on reasoning that was wrong. The
  claim was that computed `overflow-wrap: normal` and `word-break: normal` prevent the
  repository name from splitting. Those two properties suppress *arbitrary* mid-word breaking
  only. UAX #14 gives a break opportunity after U+002D HYPHEN-MINUS (class HY) independently of
  both, and `break-normal` does not remove it. The evidence gathered proved a different
  proposition than the one the test asserts.
finding: |
  The decisive case needs no narrowing at all. At the default viewport
  `metal-installer-secureboot` breaks after a hyphen, so the first line reads `metal-installer`
  — the ordinary installer name standing as the visible head of a SecureBoot reference. That is
  the ISO/installer drift `installer.go:174-195` says the refusal exists to prevent, arriving in
  the one place an operator reads it.

  Only `metal-installer` was tested. The four-name set was already enumerated in this phase's own
  matrix; the other three were never looked at.

  The method was also unsound independent of the result: break windows are non-monotonic in width
  (columns 40-115px and 174-238px both split), and the 400px viewport tested yielded a 152-168px
  column sitting in the gap between them. "Narrow until cramped" passes through a safe window and
  can never establish this property.

  `images.test.tsx:849` is named `cannot split a repository name across a line break` but asserts
  only that className lacks break-all/break-words and that textContent is intact. It passes while
  the name is split.
badge_half: |
  The badge half of the test does survive: "the build probe did not run" has zero occurrences in
  the shipped bundle and no Go string produces it. But see gap G-02-14 — the *usable* branch
  returns before `isProbed` is ever consulted.

### 2. The architecture beside the verdict, across three record kinds

deferred_from: 02-13-PLAN.md task 2 `<human-check>`

steps: |
  Have one amd64 schematic, one arm64 schematic, and one record authored before plan 02-13 (an
  existing record from an earlier session). Open the saved list, then each record's detail dialog.
expected: |
  Each verdict names the architecture it is about; the pre-02-13 record reads as unqualified
  rather than as amd64.
why_human: |
  The point is what an operator reads beside a badge. jsdom asserts the text nodes exist, not that
  the pairing is legible.
result: pass
source: automated
evidence: |
  All three record kinds observed together in the saved list and in their detail dialogs:

  | record | badge | qualifier |
  |---|---|---|
  | uat amd64 microcode (probed) | Usable — the build probe confirmed it | architecture: amd64 |
  | legacy record authored before 02-13 (`arch: ""`) | Usable — the build probe confirmed it | none |
  | uat unprobed record | Not verified — the build probe has no verdict | architecture: amd64 |
  | uat arm64 iscsi (probed) | Usable — the build probe confirmed it | architecture: arm64 |

  The pre-02-13 record reads unqualified, not as amd64 — in the list and in the detail dialog.
  Switching the detail dialog's asset-panel architecture to arm64 left that record's verdict
  still unqualified, so the panel's architecture does not retroactively qualify a past probe.

  correction: an earlier version of this entry said the switch "re-resolved every asset URL to
  `metal-arm64`". That is false. `installer` is byte-identical at both architectures, and the
  installer is the one reference this phase treats as dangerous. Only the other four moved.

  The fabricated fixture was confirmed representative by the adversarial re-check: the pre-02-13
  struct genuinely had no `Arch` field, no migration backfills one, and `CurrentVersion` is 2 at
  both ends, so a real old record does decode to `""`. `arch: z.string()` is identical across the
  list, detail and 201 paths with no optional, default or coercion.

  caveat: |
    No genuine pre-02-13 record existed — the data directory was empty. The unqualified record was
    made by writing a store file with no `arch` key, which the Go store decodes to `""`, the same
    value a genuinely older record produces. That is the input the UI branches on
    (`images.tsx:695`, `arch === undefined || arch === ''`), so the rendering path exercised is the
    real one, but the record's provenance is synthetic.

### 3. Is 02-09's recorded live installer matrix honest

deferred_from: 02-09-PLAN.md task 3 `<human-check>`

steps: |
  Read 02-09-SUMMARY.md's recorded live installer matrix. Compare it against the expectation table
  in `internal/imagefactory/live_test.go`. Check the throttled first run is stated rather than
  quietly dropped.
expected: |
  The matrix in the SUMMARY, the table in live_test.go and the live registry agree, and the
  throttling is on the record.
why_human: |
  A judgement about whether a recorded observation is honest. The verifier reproduced the matrix
  live at commit ad786b5 and it agreed — all four names answered at both v1.12.0 and v1.13.9 — and
  the account of the first, throttled run is at 02-09-SUMMARY.md:182. What remains is your call on
  whether that record is candid enough.
result: issue
reported: "Adversarial audit (2026-08-30): the observation is candid; what is built on it is not."
severity: minor
finding: |
  The matrix itself is honest and the softening accusation is refuted outright. All eight cells
  reproduced in a fourth independent run. 02-09-SUMMARY.md has exactly one revision in its
  history, so the candid "It took two runs, and the first one throttled" paragraph was never
  revised. `liveInstallerMatrix` was born all-`liveAnswers`. WINDOWS entry 5 is byte-identical
  across all eleven revisions of that file.

  What fails is everything built on top of the observation:

  - **This UAT swapped the plan's own check, and did not say so.** 02-09-PLAN.md:459 asks whether
    the matrix agrees with the table in `fake_test.go` — which has no v1.12.0 row at all.
    02-UAT.md:113 restates it as `live_test.go`, which was written in the same commit by the same
    author from the same run. That makes it a transcription check that cannot fail. I ran it as
    the UAT stated it and reported agreement as if it meant something.
  - "settled at both ends of the supported range" is false: `MaxSupportedVersion = "v1.14"` was
    never probed. `installer.go:197` escalates this to "is settled by", and that sentence is what
    T-02-53's merge acceptance rests on.
  - `fake_test.go:69-71` claims the matrix settles a v1.9.0 SecureBoot cell the subtest can never
    reach — it loops only over v1.13.9 and v1.12.0.
  - "pins v1.9.0 to `installer` alone" is false in both files; the row is
    `{"installer": true, "installer-secureboot": true}`.

  Verdict: candid about what happened, not candid about what it proves.### 4. Accept or refuse the new SecureBoot 502 path (T-02-53)

steps: |
  Decide, do not test. A SecureBoot asset request at a Talos version where neither SecureBoot
  installer name resolves now answers with no installer reference rather than substituting the
  ordinary installer.
expected: |
  A recorded merge-time acceptance, or an instruction to substitute after all.
why_human: |
  02-09-SUMMARY.md:279 raises it explicitly and hands it to a human. The live matrix shows the path
  is unreachable at v1.12.0 and v1.13.9, the two versions probed; it is unknown for v1.13.x below
  the pin and for v1.14. No offline test can settle it. Substituting a non-SecureBoot installer for
  a SecureBoot request is the failure mode the refusal exists to prevent.
result: pass
ratified_by: user
ratified_at: 2026-08-30
decision: |
  Split ratification, as recommended by the adversarial audit.

  RATIFIED: the "never substitute" rule, merged unchanged. A substituted installer would be
  undetectable forever by construction. docs/api-contract.md:661-663 defines `warnings: []` as
  meaning the installer repository name was *proven*, and warnings.go declares three codes, none
  of which means "substituted" — so the response would carry an affirmative claim of proof.
  SecureBoot is a per-request query parameter (schematics.go:592) absent from the stored record,
  so once the reference is copied no later code, log or audit entry can re-derive that a
  substitution occurred. G-02-3 was findable only because two processes disagreed; a substitution
  produces no disagreement to find.

  NOT RATIFIED: the coupling of "never substitute" with "answer with nothing at all". Both
  adversaries converged on this independently and it is confirmed in source: ISO, PXE, disk-image
  and cmdline are built and validated at schematics.go:400-420 by pure string assembly that never
  touches the registry, and `if err != nil { return }` at :432-436 discards all four. Filed as
  G-02-15.

  REQUIRED BEFORE SHIP: the assets-route atomicity is written into docs/api-contract.md one way
  or the other. Today :655 describes "no installer reference" while the code returns no references
  at all — the description shown to the ratifying human is materially milder than the behaviour.

### 5. Drive the assembled binary through a browser against the live Image Factory

steps: |
  With the binary running against the real Image Factory, in a browser:
  a. Author a schematic end to end.
  b. Type a control character into a kernel argument and try to create.
  c. Delete a schematic in a second tab, then click its still-listed row in the first.
  d. Switch the asset panel's architecture, then open the creation form.
expected: |
  b. The refusal is a 400 naming the field, not "The Image Factory did not answer usably".
  c. The dialog says the schematic is gone, and the row disappears from the saved list.
  d. The creation form's Architecture select is unchanged by what the asset panel was set to.
why_human: |
  The /images route has never been opened outside jsdom since the gap-closure round. Every
  behaviour above has a passing test, but round 1's UAT found six defects the same suites could not
  reach, because the suites use zero-latency fakes and the failures were latency- and
  layout-shaped.
result: issue
reported: "Adversarial re-check (2026-08-30): sub-check (b) refuted at high confidence."
severity: major
correction: |
  Recorded as `pass` earlier in this session. Sub-checks (a) and (d) hold outright and (c) holds
  on its primary claim. Sub-check (b) does not, and (b) was tested with exactly one codepoint.
finding: |
  Only U+0007 was ever tried. Fired against the live API, five control characters do produce the
  claimed field-named 400 — but:

  - **U+2028 produces HTTP 502 `upstream.factory-unavailable`, detail "The Image Factory did not
    answer usably".** That is verbatim the sentence sub-check (b) exists to exclude. Root cause
    proved by posting holzkube's own canonical document to factory.talos.dev directly and getting
    a YAML scan error: the document holzkube emits is invalid YAML. See gap G-02-16.
  - U+FEFF, U+0085 and U+009F produce 502 `upstream.factory-rejected` — a FACT-06 divergence
    triggered by operator input.
  - Unpaired surrogates and raw invalid UTF-8 return **201** with silent U+FFFD rewriting, so
    "refused twice over" does not hold for that class either.
  - `name` and `cluster` are not validated at all. A payload with NUL and RLO in the name returns
    201 and renders raw in the saved table. See gap G-02-17.
  - Client-side `hasControlCharacter` tests only <0x20, 0x7F and D800-DFFF, so U+2028, U+FEFF,
    U+0085, U+200B, U+202E and U+00A0 all pass the form.
sub_check_c: |
  Holds, and the 404-invalidation is causally attributable rather than routine refetching. But
  narrower than the record implied: a record deleted while its dialog is already open leaves the
  dialog showing a copyable installer reference and a "Usable" badge, with the row still present.

## Summary

total: 5
passed: 2
issues: 3
pending: 0
skipped: 0
blocked: 0

note: |
  Tests 1 and 5 were recorded as `pass` earlier in this same session and are corrected above.
  An adversarial re-check refuted both at high confidence. The audit also surfaced nine defects
  that no UAT test covers; they are in the Gaps section below as G-02-14 through G-02-22.

## Discharged before UAT

The verifier listed six human items. One was settled during this run and is not a test here:

- **Where the seven scoped-out round-2 review findings are recorded.** A policy call, resolved the
  same way round 1's was: entered in `.planning/WINDOWS.md` as ids 22-28 with an `R2-` prefix, plus
  id 29 for a residual risk neither review round filed. Commit `3ac3299`.

## Gaps

<!-- Round 2. Round 1 used G-02-1 .. G-02-9. G-02-10 .. G-02-12 come from failed UAT tests;
     G-02-13 .. G-02-22 were surfaced by the adversarial audit and no UAT test covers them. -->

- gap_id: G-02-10
  truth: "The repository name is unbroken; metal-installer never wraps into something readable as installer"
  status: resolved
  resolved_by: 02-17-PLAN.md
  resolved_at: 2026-08-30
  reason: "UAX #14 gives a break opportunity after U+002D (class HY) that break-normal does not remove. At the default viewport metal-installer-secureboot breaks after a hyphen and the first line reads metal-installer."
  severity: major
  test: 1
  artifacts:
    - path: "web/src/routes/images.tsx"
      issue: "ReferenceValue inserts <wbr> on path separators only; nothing suppresses the hyphen break"
    - path: "web/src/routes/images.test.tsx:849"
      issue: "test named 'cannot split a repository name across a line break' asserts only className and textContent; passes while the name is split"
  missing:
    - "Suppress the hyphen break opportunity in the repository-name segment (word-break/line-break control or U+2011, not <wbr>)"
    - "Assert the property the test is named for: measure line boxes, not class strings"
    - "Cover all four installer names, not metal-installer alone"

- gap_id: G-02-11
  truth: "A control character in a kernel argument is refused with a field-named 400, never as 'The Image Factory did not answer usably'"
  status: resolved
  resolved_by: 02-20-PLAN.md
  resolved_at: 2026-08-30
  reason: "U+2028 returns 502 upstream.factory-unavailable with exactly that sentence. U+FEFF/U+0085/U+009F return 502 upstream.factory-rejected. Unpaired surrogates and invalid UTF-8 return 201 with silent U+FFFD rewriting."
  severity: major
  test: 5
  artifacts:
    - path: "web/src/routes/images.tsx"
      issue: "hasControlCharacter tests only <0x20, 0x7F and D800-DFFF"
  missing:
    - "Extend the guard to YAML's line-break and non-printable sets: U+0085, U+2028, U+2029, C1, U+FEFF"
    - "Decide and test the unpaired-surrogate contract; today it silently rewrites to U+FFFD and returns 201"
  depends_on: G-02-16

- gap_id: G-02-12
  truth: "02-09's recorded live installer matrix is honest"
  status: resolved
  resolved_by: 02-16-PLAN.md
  resolved_at: 2026-08-30
  reason: "The observation is candid and reproduced a fourth time; the claims built on it overclaim. 'Settled at both ends of the supported range' is false with MaxSupportedVersion v1.14 never probed, and installer.go:197 escalates it to 'is settled by' — the sentence T-02-53's acceptance rests on."
  severity: minor
  test: 3
  artifacts:
    - path: "internal/imagefactory/installer.go:197"
      issue: "'is settled by' overstates a matrix that never probed v1.14 or v1.13.x below the pin"
    - path: "internal/imagefactory/fake_test.go:69-71"
      issue: "claims the matrix settles a v1.9.0 SecureBoot cell the subtest cannot reach; 'pins v1.9.0 to installer alone' is false in both files"
  missing:
    - "Restate what the matrix settles to match what it probed"
    - "Correct the two false statements about the fake's v1.9.0 row"
    - "Record that 02-UAT.md:113 substituted live_test.go for the plan's fake_test.go, making the check unfalsifiable"

- gap_id: G-02-13
  truth: "installer-secureboot is a legacy alias of metal-installer-secureboot"
  status: resolved
  resolved_by: 02-16-PLAN.md
  resolved_at: 2026-08-30
  reason: "Measured live twice at v1.13.9: metal-installer-secureboot is sha256:dd035140, installer-secureboot is sha256:eb5cfa79 — different images. Round 1's G-02-4 evidence recorded them as identical, and that is the basis for treating the SecureBoot legacy fallback as harmless."
  severity: major
  artifacts:
    - path: "internal/imagefactory/installer.go:180-181"
      issue: "hardcoded digests f960382f/878b171c no longer reproduce"
  missing:
    - "Correct the recorded alias claim and the stale digests"
    - "Decide whether the SecureBoot legacy fallback is still acceptable now that the images differ"

- gap_id: G-02-14
  truth: "No state makes a stronger claim than the record supports (T-02-62)"
  status: resolved
  resolved_by: 02-15-PLAN.md
  resolved_at: 2026-08-30
  reason: "UsabilityVerdict tests `if (usable)` and returns 'Usable — the build probe confirmed it' before it ever reaches `if (!isProbed(probedAt))`. A record with usable:true and probed_at zero renders the affirmative verdict. Reproduced live from a planted fixture in both list and dialog."
  severity: major
  artifacts:
    - path: "web/src/routes/images.tsx"
      issue: "isProbed is never consulted on the usable branch — the branch where the overclaim is dangerous"
  missing:
    - "Reorder the conditional so the no-verdict test precedes the usable test"
  note: "Latent: not reachable through today's POST path, but any migration, import, re-probe or hand-edited record produces it."

- gap_id: G-02-15
  truth: "A SecureBoot request that cannot resolve an installer answers with no installer reference"
  status: resolved
  resolved_by: 02-19-PLAN.md
  resolved_at: 2026-08-30
  reason: "The route answers with no references at all. ISO, PXE, disk-image and cmdline are built by pure string assembly that never touches the registry at schematics.go:400-420, then discarded by `if err != nil { return }` at :432-436. docs/api-contract.md:655 describes the milder behaviour, so the human ratifying T-02-53 is shown a description the code does not match."
  severity: major
  artifacts:
    - path: "internal/httpapi/handlers/schematics.go:432-436"
      issue: "discards four already-validated URLs on any installer error"
    - path: "web/src/routes/images.tsx:1165-1170"
      issue: "one hardcoded role=alert sentence that never reads assets.error, never names SecureBoot, never suggests unticking the box; no test covers the branch"
  missing:
    - "Return the four registry-free references and mark the installer alone as unresolved"
    - "Surface the server's detail; separate 'wait and retry' from 'this version has no SecureBoot installer'"
  reachable_today: "Yes, with no SecureBoot name failing: a cold ?secureboot=true request measured 49.8s of a 60s writeTimeout, and 60.002s at v1.13.8 returning HTTP 000 with the connection dropped."

- gap_id: G-02-16
  truth: "FACT-06: holzkube's computed schematic id equals the Factory's"
  status: resolved
  resolved_by: 02-14-PLAN.md
  resolved_at: 2026-08-30
  reason: "representable refuses only r < 0x20 || r == 0x7F, but YAML's line-break set also contains U+0085 and U+2028/U+2029 and its printable set excludes C1 and U+FEFF; plainAllowed inspects only ASCII bytes. Nine codepoints sampled, four broke it in three distinct ways."
  severity: major
  artifacts:
    - path: "internal/imagefactory/schematicid.go"
      issue: "transcribed YAML emitter diverges from the real one above U+007F"
  missing:
    - "Extend representable and plainAllowed to YAML's actual line-break and printable sets"
    - "Differential fuzz of Schematic.ID() against the canonical the live Factory returns"
  worst_case: "With U+0085 the Factory silently eats the character, so the stored record's kernel_args and canonical disagree about what the image will be built with. A trailing-U+0085 variant collapsed onto an existing canonical and returned a false 409."

- gap_id: G-02-17
  truth: "Operator input that cannot be carried is refused before it reaches the Factory"
  status: resolved
  resolved_by: 02-20-PLAN.md
  resolved_at: 2026-08-30
  reason: "schematicInput.validate checks only that name is non-empty plus talos_version and arch. Neither name nor cluster passes through representable or any control-character check. A POST with NUL and RLO in the name returned 201, stored verbatim, and rendered raw in the saved table including the bidi override."
  severity: major
  artifacts:
    - path: "internal/httpapi/handlers/schematics.go:487"
      issue: "two unguarded doors in the same request body whose kernel_args and meta siblings are guarded"
  missing:
    - "Route name and cluster through the same guard"
    - "Decide the rendering contract for bidi controls in the saved table"

- gap_id: G-02-18
  truth: "The live drift guard fails when the installer name drifts"
  status: resolved
  resolved_by: 02-16-PLAN.md
  resolved_at: 2026-08-30
  reason: "resolveInstallerRepo returns ErrUpstreamUnavailable only when NO candidate answered 2xx. On a partial throttle the preferred name times out, the legacy one answers, and a nil error comes back — so errors.Is at live_test.go:208/:217 never fires and both assertions pass on the fallback. Nothing asserts which name resolved. This is the exact shape 02-09's first run hit."
  severity: major
  artifacts:
    - path: "internal/imagefactory/live_test.go:208-217"
      issue: "plain != secure and Contains(secure, '-secureboot/') are both satisfied by the legacy fallback"
    - path: "cffa851"
      issue: "sole edit to the drift guard discarded the Warning that would close this: `plain, err :=` became `plain, _, err :=`; two hunks, no comment, in no record"
  missing:
    - "Assert len(warnings) == 0 in the drift guard"
    - "Assert which repository name resolved, not merely that the two differ"

- gap_id: G-02-19
  truth: "The asset dialog widens on large screens"
  status: resolved
  resolved_by: 02-15-PLAN.md
  resolved_at: 2026-08-30
  reason: "DialogContent carries sm:max-w-sm and the caller adds max-w-3xl. tailwind-merge treats them as different groups because of the variant prefix, so both survive and the sm: rule wins by source order. Measured dialogWidth 384 at a 1200px viewport."
  severity: minor
  artifacts:
    - path: "web/src/routes/images.tsx:958"
      issue: "max-w-3xl is dead code; the panel is permanently cramped at every viewport"
  missing:
    - "Remove or correct the conflicting width class"
  blocks: "G-02-10 — any width-based mitigation would be tuned against this bug rather than the intended layout."

- gap_id: G-02-20
  truth: "A schematic record carries the architecture its probe used"
  status: resolved
  resolved_by: 02-21-PLAN.md
  resolved_at: 2026-08-30
  reason: "The id is the SHA-256 of the canonical document, which excludes the architecture, so POSTing the same customisation at a second arch returns 409 store.conflict — verified live. model.Schematic.Arch is a field whose value the record's own identity cannot vary."
  severity: minor
  artifacts:
    - path: "internal/model/model.go"
      issue: "one customisation holds exactly one architecture's verdict; obtaining the other means destroying the first"
  missing:
    - "Decide whether arch belongs in the identity, or whether one record should carry per-arch verdicts"
  note: "On no plan and not in WINDOWS.md; entry 21 covers only the narrower empty-arch legacy case."

- gap_id: G-02-21
  truth: "On a throttle, an unrun-verify ledger entry naming the unverified SecureBoot matrix is appended"
  status: resolved
  resolved_by: 02-21-PLAN.md
  resolved_at: 2026-08-30
  reason: "02-09-PLAN.md:447 requires this conjunctively with the two things that were done. It throttled; the entry was never filed. The SUMMARY at :271 reframes the conjunctive criterion as disjunctive and cites the ledger being unappendable — real for seven minutes (9562498 at 18:41:05, e1a5c6c at 18:48:09), after which plans 02-11 through 02-13 appended entries 19-29 without trouble."
  severity: minor
  artifacts:
    - path: ".planning/WINDOWS.md"
      issue: "no entry records the plan-09 throttle, the unprobed v1.14, or the unprobed v1.13.x below the pin; a ship gate reading the register sees none of it"
    - path: "02-VERIFICATION.md:233"
      issue: "marks the criterion VERIFIED on the softened reading"
  missing:
    - "File the entry"

- gap_id: G-02-22
  truth: "The architecture is not recoverable from a pre-02-13 record"
  status: resolved
  resolved_by: 02-21-PLAN.md
  resolved_at: 2026-08-30
  reason: "True for the usable subset, false for the refused one. A record whose probe refused carries the architecture verbatim inside probe_reason (pre-02-13 format '%s at %s/%s answered HTTP %d', stored by probeDetail at schematics.go:609), so it is machine-parseable."
  severity: minor
  artifacts:
    - path: "internal/model/model.go:96"
      issue: "overbroad rationale statement"
    - path: "web/src/api.ts:198"
      issue: "overbroad rationale statement"
    - path: ".planning/WINDOWS.md"
      issue: "entry 21 overbroad"
  missing:
    - "Narrow the three statements to the usable subset"

- gap_id: G-02-23
  truth: "The problem-type taxonomy is a stable public contract that no deployment has to host"
  status: resolved
  resolved_by: 02-18-PLAN.md
  resolved_at: 2026-08-30
  reason: "User decision 2026-08-30. The type URIs hardcode a vendor domain into a contract that AGPL redistributors must ship unchanged, and they look like a promise that a page exists at holzkube.dev. Nothing dereferences them — the UI branches on `code` (problem.ts:7, :98) and no comparison against the type URI exists anywhere — so the domain buys nothing and costs the redistribution story."
  severity: minor
  origin: user-decision
  artifacts:
    - path: "internal/httpapi/problem.go:18-40"
      issue: "ProblemBaseURI and all thirteen Type* constants are rooted at https://holzkube.dev/problems/"
    - path: "docs/api-contract.md"
      issue: "documents the https taxonomy"
    - path: "web/src/lib/problem.test.ts, web/src/routes/images.test.tsx, web/src/components/SudoDialog.test.tsx"
      issue: "fixtures build type strings from the https base"
  missing:
    - "Re-root the taxonomy at urn:holzkube-manager:problem: (RFC 9457 permits non-HTTP URIs)"
    - "Keep the taxonomy closed and stable — the URN is deployment-independent, NOT configurable per install; problem.go:20's 'clients may match on them, so they never change' still holds"
    - "State in docs/api-contract.md that type URIs are identifiers and are not dereferenceable"
  rejected_alternative: |
    A per-deployment configurable base URI was considered and rejected: two installations would
    emit different `type` values for the same error, so every third-party client would need a
    per-install special case. It solves a problem that does not exist (nothing fetches the URI)
    at the cost of the one property the field has.
  sequencing: |
    Lands BEFORE the holzkube -> holzkube-manager rename and already carries the new name. The
    rename script's PROTECTED list has been updated to guard `holzkube-manager` first, so the
    URN is not double-renamed into holzkube-manager-manager.

## Residual Risk

The breadth of the canonical-serialiser divergence class (G-02-16) is unbounded, and it sits on
the one property this phase exists to guarantee. Nine codepoints were sampled and four broke it.
Every non-ASCII rune whose quoting differs between the transcribed emitter and the real one is an
untested candidate, and nothing in the repo tests any codepoint above U+007F. Ranked above the
drift-guard gap deliberately: a detector's blind spot is worth less than a corruption in the thing
being detected.
