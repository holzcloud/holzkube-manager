---
phase: 17-versionspruefung-zwischen-plan-und-apply
plan: 02
subsystem: ui
tags: [react, vitest, audit, docs, optimistic-concurrency]
status: complete

requires:
  - phase: 17-01
    provides: base_digest in the plan answer, required on apply, 409 conflict.config-changed-since-plan
provides:
  - the Config page sends the digest of its plan and shows a refusal as the server's sentence with a way to plan again
  - audit allowlist for config.apply carries base_digest, never patch text
  - budget row of the apply route describes one client for reading and sending
  - README, guide and API contract say what the check does and cannot do
affects: [phase 17 verification]

actuals:
  tokens: 8700
  tasks: 3
  commits: 3
plan_head_before: 70e89dd334d05d6d70c2c9a2febcf96d57ec8d51
plan_head_after: 80c5c12ebf6562e837d96a6b46dcf3ae35b12f32
commits: 3

key-files:
  created:
    - web/src/routes/config.test.tsx
  modified:
    - web/src/api.ts
    - web/src/api.test.ts
    - web/src/lib/problem.ts
    - web/src/routes/config.tsx
    - internal/audit/redact.go
    - internal/audit/redact_test.go
    - cmd/holzkube-managerd/budget_test.go
    - docs/api-contract.md
    - docs/guide.md
    - README.md

requirements-addressed: [VERS-02, VERS-06]
requirements-completed: []  # not ticked here by the plan's own instruction; the phase verification does it

completed: 2026-10-20
---

# Phase 17 Plan 02: Version check between plan and apply, interface, tables, texts

**STATUS: COMPLETE (2026-11-01, entries 180 and 181 written by hand on the operator's choice; the verb cannot read the ledger). Earlier: PARTIAL. One part of Task 2 is NOT done: the two open ledger entries (A3/A4 and the read-to-send window) are not in `.planning/WINDOWS.md`. See "Blocked" below. Everything else ran.**

The Config page now sends the digest of the plan it applied from, shows a refusal as the server's sentence in a yellow alert with Apply locked and a "See what would change again" button that repeats the plan with the same patch text, and locks a preview after it landed.

## Where it ran

Mac (darwin/arm64), go1.27.3 (linter under `GOTOOLCHAIN=go1.26.9`), Node from Homebrew with the project's `web/node_modules`, vitest jsdom project. No real cluster, no browser, no `go test -race` (CI only), no Pi (linux/arm64). `./bin/task` is not present, so the gate ran as single commands. Nothing installed, pushed, tagged or published.

## Commits

| Task | Commit | What |
|------|--------|------|
| 1 | e9adcc6 | `previewSchema.base_digest` required, `api.config.apply(..., baseDigest, patchIDs)`, `CODE_CONFIG_CHANGED_SINCE_PLAN`, refusal state in `NodeConfig`, alert box and button in `PreviewCard`, `key={planSeq}`, Apply locked after refusal and after success; `config.test.tsx` (10 tests) and an `api.test.ts` case |
| 2 (part) | 87c8aad | audit allowlist `base_digest` + `TestConfigApplyKeepsItsBaseDigestAndNothingOfThePatches`; budget row of the apply route (three calls, one Version probe) |
| 3 | 80c5c12 | `docs/api-contract.md` (routes table, "The apply checks what the plan saw", audit), `docs/guide.md` ("Applying a configuration patch"), README entry "Configuration changes:" |

## Blocked: the ledger entries (Task 2 step 3), not done

The plan's step 3 says to add two entries (kind `unrun-verify` for A3/A4 being proved only on the simulator, kind `gap` for the read-to-send window with no precondition field in the RPC) by hand, because `gsd-tools windows append --kind gap` was refused in plan 15-10. The instruction for this run was the opposite: do not hand-edit `.planning/WINDOWS.md`, use the `gsd-tools windows` verbs.

The verbs cannot be used on this ledger at all. Measured:

- `gsd-tools windows append --kind gap --phase 17 --file x --description probe` exits non-zero with `Error: Ledger entry 147 has invalid kind: "gap"`.
- `gsd-tools windows status` fails with the same message. The verb's `KINDS` are `stub, todo, fixme, skipped-test, lint-warning, unmet-truth, unrun-verify, deviation`; the ledger already holds entries of kind `gap` (147, 178, 179) and `defect` (176), so the verb refuses to read the file, and therefore any kind, including the accepted `unrun-verify`, fails for append.

Nothing was written to WINDOWS.md (`git status` clean for it; counts unchanged: open 26, total 179, last_updated 2026-10-15). I did not work around it with a temporary normalised copy or a hand edit, since either would be hand-editing under another name.

Consequences, stated plainly:

- `open_count`/`total_count` did not move by two, and the injection of the counter in step 3 was not run (it is meaningless without the entries).
- The docs name the two new limits by content and say "an open ledger entry of phase 17", but carry no number for them. They do carry 176 and 178. Once the entries exist, the numbers can be added to `docs/guide.md` and `docs/api-contract.md` (optional).
- The two entries and their wording are in the plan (Task 2 step 3, bullets "Eintrag 1" and "Eintrag 2"). A decision is needed: allow the hand edit as in 15-10, or extend the verb's kind vocabulary (or tolerate unknown kinds on read) so the verb works.

## ROT runs (exit code of the command itself)

- Task 1: `vitest run --project jsdom src/routes/config.test.tsx src/api.test.ts` before the change: exit 1, 9 failed of 27 (the digest not sent, no `alert`, Apply not locked, schema accepts a missing digest, `api.config.apply` body without `base_digest`); `tsc --noEmit` exit 1 (a 5-argument call did not type-check against the old signature). A first version of the test helper failed for the wrong reason (the first textbox on the page is the saved-patches Name field, which stands before a node is chosen); fixed by selecting the field by its placeholder, then the red was for the right reasons.
- Task 2 audit: `go test ./internal/audit/ -run TestConfigApplyKeepsItsBaseDigest`: exit 1, `redact_test.go:318: base_digest = <redacted>, want the digest in clear`.

## Injections (file backed up outside the repo, one change, exit code of the vitest/go command itself, restored byte-identical with `cmp`, green run after)

Task 1 (all exit 1):
- (a) F11, the digest of the first preview of the page kept for later applies (a never-updated holder): `sends the new digest after planning again, not the old one` (config.test.tsx:183) and `starts from the new verdict once the operator plans again` (:245) red. A first attempt at (a) referenced an undefined name and made everything red, so it was discarded and redone as a real stale digest.
- (b) the refusal handled like any other error (`setFailure`): 4 red, e.g. `Unable to find role="alert"` (:146, :168, :200).
- (c) Apply stays enabled after a refusal: `expect(element).toBeDisabled()` at :148 and :205.
- (d) Apply stays enabled after success: `cannot apply the same preview a second time after it landed`, :224.
- (e) `base_digest: z.string().optional()`: `refuses an answer of the plan that has no base_digest`, :117 (`expected true to be false`).
- (f) `PreviewCard` without `key={planSeq}`: red at :183 (old mode and digest carried) and :242 (the "Applied" line stays). It did not stay green, so no gap had to be closed.

Task 2:
- F16 (`base_digest` removed from the `config.apply` allowlist): exit 1, `redact_test.go:318: base_digest = <redacted>, want the digest in clear`; restored, `cmp` identical, `go test ./internal/audit/` ok.
- Counter injection (open_count off by one): NOT run, see Blocked.

## Gates

| Gate | Result |
|------|--------|
| `vitest run --project jsdom src/routes/config.test.tsx src/api.test.ts` | exit 0, 27 passed, 0 skipped |
| `vitest run --project jsdom` (whole web suite) | exit 0, 72 files, 940 passed |
| `tsc --noEmit` | exit 0 |
| `npm --prefix web run lint` | exit 0 (two pre-existing warnings in `wall.test.tsx`) |
| `golangci-lint v2.13.1 run ./...` (go1.26.9) | exit 0, 0 issues |
| `go test ./cmd/holzkube-managerd/ ./internal/planning/ ./internal/audit/` | ok |
| `go test ./... -count=1` | exit 1: only the two known pre-existing failures below; everything else ok |
| Untouched against BASE: changelog, web/fixtures, docs/screenshots, diff.go, applymode.go, snapshot.go, redact.go (machineconfig), compat, wall.tsx | empty diff |

Pre-existing failures, not caused by this plan (logged in `deferred-items.md` by plan 01, not fixed, not claimed to pass):
- `internal/httpapi` `TestNoAssetIsInlinedPastTheContentSecurityPolicy` (embedded built CSS inlines a `data:` font).
- `internal/publicrepo` `TestNoIdentifierOfTheRealInstallationIsInTheHistory`: it names 19 commits in this run, all on the local branches `pre-public-rewrite` (and similar), none in `70e89dd..HEAD`; the tree half passes.

## Deviations from Plan

**1. [Rule 3 - blocker, ledger] The two ledger entries were not written.** See Blocked. This is the only part of the plan not carried out.

**2. Docs carry no ledger numbers for the two new entries.** Follows from 1.

Otherwise the plan ran as written. The yellow box uses `role="alert"`, the button is the existing outline `Button`; layout is not covered by any test.

## Known Stubs

None.

## Threat Flags

None beyond the plan's register. T-17-10 (digest not sent or stale) has tests and injections (a), (c), (e); T-17-12 (digest in the audit archive) has the test and F16; T-17-13 (operator misled about limits) is only partly mitigated until the two ledger entries exist; T-17-11 accepted as planned (React escapes the sentence; the page adds nothing to it).

## Limits of what is proved (verbatim from the plan)

Proved only against the simulator, jsdom and a stubbed API. Not proved: the layout in a browser (box, button, phone); the re-encoding of a real Talos node (two readings without a change giving the same digest, A3); the window between reading and sending (A4); a run on the Pi (linux/arm64). `go test -race` did not run (CI only).

## Open: the `human-check` (belongs to the phase verification as `human_needed`)

1. Against a real node: plan, wait briefly, apply without a change: must go through (A3).
2. Plan, change the node with talosctl in another document, apply: refused with the sentence and the path; the page shows the yellow box, Apply locked, "See what would change again" plans again.
3. After a success, the same preview cannot be applied a second time: the button is locked.
4. The yellow box and the button are readable and usable on a phone.

## Self-Check

Files exist (`web/src/routes/config.test.tsx` and the modified files); commits e9adcc6, 87c8aad, 80c5c12 are ancestors of HEAD. Self-check passes for what was done; the plan as a whole is NOT complete (ledger entries missing).
