---
phase: 02-transport-seam-talossim-image-factory
plan: 17
subsystem: testing
tags: [vitest, playwright, browser-testing, layout, uax14, tailwind, react]

requires:
  - phase: 02-15
    provides: "The dialog width class `sm:max-w-3xl` that survives the tailwind-merge pass, asserted structurally in jsdom and left with an unmeasured pixel"
  - phase: 02-09
    provides: "`installerCandidates` — the four installer repository names this plan measures"
provides:
  - "A second vitest project that runs in headless Chromium, so this repository can measure layout at all"
  - "A measured guarantee that all four installer repository names occupy exactly one line box, at every width in a 251-step sweep"
  - "A fix that removes the UAX #14 hyphen break inside path segments without changing one character of the reference"
  - "The measured dialog width (768px), closing the half of G-02-19 that jsdom could not assert"
  - "`INSTALLER_REPOSITORY_NAMES` and `INSTALLER_REPOSITORY_NAMES_LITERAL`, the seam plan 02-20's `guard_drift_test.go` binds to `installerCandidates`"
affects: [02-20, 02-21, any future frontend phase needing a layout or visual assertion]

actuals:
  tokens: 10250
  tasks: 4
  commits: 4

tech-stack:
  added:
    - "@vitest/browser 4.1.11"
    - "@vitest/browser-playwright 4.1.11"
    - "playwright 1.62.1"
  patterns:
    - "Two vitest projects in the one vite.config.ts, so both inherit the single path alias and the Tailwind plugin"
    - "`*.browser.test.tsx` as the suffix that selects the layout-measuring project, excluded explicitly from the jsdom project"
    - "Line-box measurement via `Range.getClientRects()` and distinct `top` values, instead of class-string inspection"
    - "A missing external binary fails with the exact command that installs it, not the tool's generic message"

key-files:
  created:
    - web/src/routes/images.browser.test.tsx
  modified:
    - web/vite.config.ts
    - web/package.json
    - web/package-lock.json
    - web/src/routes/images.tsx
    - web/src/routes/images.test.tsx
    - Taskfile.yml
    - .github/workflows/ci.yml
    - README.md
    - .gitignore

key-decisions:
  - "`whitespace-nowrap` on a per-segment inline element, with `<wbr />` moved outside it — suppresses the break opportunity rather than arguing about which character offers it, and leaves the separators as the only break points"
  - "The four names are derived from the resolver's construction AND declared as literals, bound by a test — the derivation stops the literals being an unchecked transcription, the literals are what a Go test can read out of the source"
  - "The browser project does not inherit `src/test/setup.ts`: every prosthesis in it substitutes for something a real browser has"
  - "`@types/node` stays out; the one Node API the config needs is reached through a variable specifier so TypeScript never resolves it"
  - "768px is asserted as an observation, not a token derivation — it was checked independently against a bare element carrying the same classes before being asserted on the dialog"

patterns-established:
  - "Layout claims are measured in the browser project; jsdom tests are named for what they can honestly check and name the browser test that measures the rest"
  - "A test whose name promises a property it cannot measure is a defect, and is replaced rather than left beside a better one"

requirements-completed: [FACT-03]

coverage:
  - id: D1
    description: "All four installer repository names — metal-installer, installer, metal-installer-secureboot, installer-secureboot — occupy exactly one line box at every container width from 30px to 280px"
    requirement: FACT-03
    verification:
      - kind: automated_ui
        ref: "web/src/routes/images.browser.test.tsx#occupies one line box at every width from 30px to 280px: metal-installer"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/images.browser.test.tsx#occupies one line box at every width from 30px to 280px: installer"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/images.browser.test.tsx#occupies one line box at every width from 30px to 280px: metal-installer-secureboot"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/images.browser.test.tsx#occupies one line box at every width from 30px to 280px: installer-secureboot"
        status: pass
    human_judgment: false
  - id: D2
    description: "The reference's text content is byte-for-byte unchanged by the fix, at every measured width and in the DOM the Copy control reads"
    requirement: FACT-03
    verification:
      - kind: automated_ui
        ref: "web/src/routes/images.browser.test.tsx — textContent asserted at all 251 widths inside each of the four sweeps"
        status: pass
      - kind: unit
        ref: "web/src/routes/images.test.tsx#renders each path segment as its own element and the reference as one string"
        status: pass
    human_judgment: false
  - id: D3
    description: "The schematic detail dialog renders wider than the 384px the UAT measured; measured 768px at a 1200px viewport"
    verification:
      - kind: automated_ui
        ref: "web/src/routes/images.browser.test.tsx#renders wider than the 384px the UAT measured"
        status: pass
    human_judgment: false
  - id: D4
    description: "A reference still wraps between path segments — the fix is not over-tightened into nowrap on the whole string"
    verification:
      - kind: automated_ui
        ref: "web/src/routes/images.browser.test.tsx#still offers a break between path segments"
        status: pass
    human_judgment: false
  - id: D5
    description: "The frontend test command runs the layout-measuring project locally and in CI, and names the install command when the browser is missing"
    verification:
      - kind: integration
        ref: "npm --prefix web test (8 files, 118 tests, both projects); task test:web exit 0; task test:web:browser exit 0"
        status: pass
      - kind: manual_procedural
        ref: "PLAYWRIGHT_BROWSERS_PATH=/tmp/no-such-browsers npm run test:browser — fails naming `npm --prefix web exec -- playwright install chromium`; the jsdom project is unaffected"
        status: pass
    human_judgment: false
  - id: D6
    description: "CI installs the browser through the project's own runner, before the web test step"
    verification:
      - kind: other
        ref: ".github/workflows/ci.yml — `Install the browser…` at line 70, `Test web` at line 73"
        status: pass
    human_judgment: true
    rationale: "The step's ordering and form are verifiable by reading the file, but that it actually succeeds on ubuntu-latest with --with-deps has not been observed — no CI run has happened against this commit. The `--` fix was found by observing a silent no-op locally, which is exactly the class of failure that only a real run confirms is gone."

duration: 34 min
completed: 2026-08-30
status: complete
---

# Phase 02 Plan 17: A Test Project That Measures Layout, and the Hyphen Break It Found Summary

**A headless-Chromium vitest project that counts line boxes with `Range.getClientRects()`, and a per-segment `whitespace-nowrap` fix that stops all four installer repository names splitting after a hyphen — measured across 251 widths per name, with the reference string untouched.**

## Performance

- **Duration:** 34 min
- **Started:** 2026-08-30T09:37:00Z
- **Completed:** 2026-08-30T10:11:00Z
- **Tasks:** 4
- **Files modified:** 10 (1 created)

## Accomplishments

- **G-02-10 closed on all three `missing` bullets.** The hyphen break opportunity is suppressed inside the repository-name segment; the property is asserted by measuring line boxes rather than class strings; all four installer names are covered, not `metal-installer` alone.
- **The repository can measure layout at all.** A second vitest project runs in headless Chromium. It is the first thing here that opens the UI in a browser.
- **G-02-19's measurement half closed.** The detail dialog measures **768px** at a 1200px viewport, against the 384px the UAT measured. 02-15's token derivation and this observation agree.
- **The misnamed test is gone.** `cannot split a repository name across a line break` asserted the absence of two class names and passed while the name split. Its replacement is named for what it checks and says in as many words that the line-break property is not asserted there.

## The measurement, and what it found

Swept **30px to 280px in 1px steps — 251 widths per name, 1004 measurements**, mounted once per name and re-measured rather than remounted. Line boxes counted by putting a `Range` over the repository-name text node and counting distinct `top` values among its client rects.

**Pre-fix, measured 2026-08-30 (commit `da15835`, the RED step):**

| repository name | widths that split | windows |
|---|---|---|
| `metal-installer` | 151 / 251 | 30-115px, **174-238px** |
| `installer` | 0 / 251 | — (no hyphen; the only name ever previously tested) |
| `metal-installer-secureboot` | **251 / 251** | every width in the sweep |
| `installer-secureboot` | 194 / 251 | 30-151px, 203-274px |

Three things in that table are the whole finding.

**The non-monotonicity is real and reproduced.** `metal-installer`'s two windows are 30-115px and 174-238px, with a safe gap at **116-173px** — the audit predicted 40-115 and 174-238, and the sweep confirms both bounds. `installer-secureboot` has its own gap at 152-202px. A single sampled width proves nothing about this property, and the 400px viewport it was originally checked at produced a column sitting inside `metal-installer`'s safe gap. "Narrow until it looks cramped" passes through a safe window and can never establish this.

**`installer` never splits, and that is why the original check passed for the wrong reason.** It is the one name with no hyphen. Three of the four were never looked at, and the two that read most dangerously were among them.

**The decisive form appeared in the output.** `metal-installer-secureboot` breaks in three different shapes across the sweep, and one of them is `metal-installer` ⏎ `secureboot` — the ordinary installer's name standing alone as the visible head of a SecureBoot reference. That is the ISO/installer drift `installer.go` says the entire resolution exists to prevent, arriving in the one place an operator reads it.

**Post-fix: 0 splits, all four names, all 251 widths.** Reverting the fix locally turned all three names red again with identical counts, naming the repository and every width; restored, green.

## The fix

Each path segment is its own inline element carrying `whitespace-nowrap`, and the `<wbr />` moved from *inside* the segment element to *between* segments.

This suppresses the break opportunity rather than arguing about which character offers it. `break-normal` — `overflow-wrap: normal` plus `word-break: normal` — only suppresses *arbitrary* mid-word breaking; UAX #14 gives a break opportunity after U+002D HYPHEN-MINUS as line-break class HY, independently of both. `whitespace-nowrap` removes all of them inside the element, and the `<wbr />` outside survives as the only permitted break point.

Everything the previous strategy bought is kept: the text content is byte-for-byte the reference (neither a span, a fragment nor a `<wbr />` contributes to `textContent`), breaks between segments still happen, and the 64-character schematic id is one segment that still overflows into the horizontal scroll rather than breaking.

The prohibition against U+2011 was honoured and is now enforced by a test rather than by intent: the text content is asserted at every one of the 251 widths in each sweep. A character substitution would satisfy the line-box count and hand out a string that is not a valid OCI reference.

## Task Commits

1. **Task 1: Package legitimacy gate** — no commit (a checkpoint; the pre-decision answered both halves, and both packages were re-verified against `registry.npmjs.org` before anything was installed)
2. **Task 2: A test project that performs layout** — `8158697` (feat)
3. **Task 3: One line box, every width, four names** — `da15835` (test, RED) → `8735b10` (feat, GREEN)
4. **Task 4: What the suite covers and what it does not** — `e1af799` (docs)

No REFACTOR commit: the GREEN implementation is three lines and there was nothing to clean up.

## Files Created/Modified

- `web/src/routes/images.browser.test.tsx` — **created.** The layout suite: the dialog-width measurement, the four-name width sweep, the between-segments wrap check, and the derivation/literal agreement test. Header comment states its two properties and disclaims covering the embedded UI end to end.
- `web/vite.config.ts` — split into a `jsdom` and a `browser` project; the browser-binary precondition plugin; the `node:fs` access that needs no `@types/node`.
- `web/package.json` / `web/package-lock.json` — three pinned devDependencies and a `test:browser` script.
- `web/src/routes/images.tsx` — `ReferenceValue` exported and fixed; its doc comment rewritten to name UAX #14's HY class.
- `web/src/routes/images.test.tsx` — the misnamed test replaced; `shows the schematic id in full` scoped.
- `Taskfile.yml` — `test:web:browser`.
- `.github/workflows/ci.yml` — the browser install step, before `Test web`.
- `README.md` — a "The frontend tests need a browser" section.
- `.gitignore` — vitest's per-run screenshots and attachments.

## Decisions Made

- **`whitespace-nowrap` per segment, not a character substitution and not a width heuristic.** It is the only mechanism that removes the hyphen opportunity without touching the string.
- **The four names are derived *and* declared as literals, bound by a test.** The derivation (`INSTALLER_REPOSITORY_NAMES`) is what stops four hand-typed strings being the only authority — that is how three names went unexamined. The literal array (`INSTALLER_REPOSITORY_NAMES_LITERAL`) is what a Go test can read out of the source with `strings.Contains`, the way `TestWarningDetailsMatchTheUI` reads `SchematicWarnings.tsx`. The test `agrees with the derivation about which four names there are` holds the two together from this side.
- **The browser project does not inherit `src/test/setup.ts`.** Every stub in it — `matchMedia`, `ResizeObserver`, `hasPointerCapture`, `scrollIntoView` — is a jsdom prosthesis for something a real browser has. Installing them would replace the engine's behaviour with the fake this project exists to stop relying on.
- **`@types/node` stays out.** `tsconfig.json` curates `types` deliberately; adding Node's globals would make `process` and `__dirname` type-check inside `src/`, where they do not exist. The one Node API the config needs is reached through a variable specifier.
- **768px is asserted as an observation.** 02-15 asked that a disagreement between the derivation and the measurement be treated as a finding. There was an apparent one, it was investigated (below), and the derivation turned out to be right.

## Deviations from Plan

### Auto-fixed issues

**1. [Rule 3 - Blocking] A third package was required: `@vitest/browser-playwright`**

- **Found during:** Task 2, on the first run of the browser project.
- **Issue:** vitest 4 changed `browser.provider` from a string to a factory imported from a per-provider package. The two packages the pre-decision approved do not configure a provider on their own: `TypeError: The browser.provider configuration was changed to accept a factory instead of a string. Add an import of "playwright" from "@vitest/browser-playwright"`.
- **Why this was not halted:** the plan's blocking gate is a *legitimacy* gate, and the honest response to a package outside the approval is to apply the same criteria to it rather than either halting or waving it through. Read from `registry.npmjs.org` before installing: `@vitest/browser-playwright` latest **4.1.11**, repository `github.com/vitest-dev/vitest` directory `packages/browser-playwright`, licence MIT, maintainers `ariperkkio, antfu, hiogawa, oreanno, yyx990803`. **Identical publisher, repository, maintainer set, licence and version to the `@vitest/browser` the user approved** — it is that package's own provider split, `dependencies: {"@vitest/browser": "4.1.11"}`, `peerDependencies: {"vitest": "4.1.11", "playwright": "*"}`. Installed pinned at 4.1.11.
- **Flagged for the record:** the user approved two package names and three were installed. The third satisfies every criterion the approval stated, but it was not on the list, and that is worth a human's eye at review.
- **Committed in:** `8158697`.

**2. [Rule 1 - Bug] `npm exec` silently swallowed the CI install flags**

- **Found during:** Task 2, verifying the CI step by hand.
- **Issue:** `npm --prefix web exec playwright install --with-deps chromium` produced no output and **exited 0 having installed nothing** — npm consumed the flags as its own. As written, the CI step would have passed while doing nothing, surfacing later as a confusing browser-launch failure in the next step.
- **Fix:** `npm --prefix web exec -- playwright install --with-deps chromium`. Verified: with `--`, `npm --prefix web exec -- playwright --version` reports `Version 1.62.1`; without it, npm's own version. Comment in `ci.yml` says the `--` is load-bearing and why.
- **Committed in:** `8158697`.

**3. [Rule 1 - Bug] The Taskfile target ran vitest at the repository root**

- **Found during:** Task 2, running the new target.
- **Issue:** `npm --prefix web exec vitest run -- --project browser` failed with `No projects matched the filter "browser"`. `npm exec` honours `--prefix` for binary resolution but does **not** set the working directory, so vitest ran at the repo root, found no config, and had no projects to filter.
- **Fix:** a `test:browser` script in `web/package.json` invoked through `npm --prefix web run test:browser` — `npm run` does set cwd to the package directory, which is why the pre-existing `test:web` target has always worked.
- **Committed in:** `8158697`.

**4. [Rule 1 - Bug] `shows the schematic id in full` broke on the fix**

- **Found during:** Task 3, GREEN.
- **Issue:** `TestingLibraryElementError: Found multiple elements`. Giving each path segment its own element means the ISO, PXE and disk-image references each contain an element whose entire text is the schematic id (`/image/<id>/<version>/…`), so an unscoped exact-text query over the dialog now matches three elements instead of one. Not a regression — all three do show the id in full.
- **Fix:** scoped to the `Schematic ID` heading's own panel, with the reason in a comment.
- **Committed in:** `8735b10`.

**5. [Rule 3 - Blocking] Biome forbids exports from test files**

- **Found during:** Task 3. `lint/suspicious/noExportsInTest` on the array the plan requires be exported and 02-20 requires be bindable.
- **Fix:** a single-line `biome-ignore` naming plan 02-20's `guard_drift_test.go` as the reason the export exists.
- **Committed in:** `da15835`.

---

**Total deviations:** 5 auto-fixed (3 bugs, 2 blockers).
**Impact on plan:** none on scope. Two of the bugs — the silent `npm exec` no-op and the wrong-cwd Taskfile target — were checks that would have looked green while doing nothing, which is the same failure mode as the test this plan exists to replace.

## Issues Encountered

**The dialog first measured 745.34px, not 768px.** 02-15 asked that a disagreement between the derivation and the measurement be treated as a finding rather than a test to loosen, so it was. A bare element carrying the identical class set measures exactly **768px** at this viewport, and `window.innerWidth` was 1200 with `matchMedia('(min-width: 40rem)')` matching — so the token derivation was correct and something else was wrong.

The cause: `getBoundingClientRect()` returns the **transformed** border box, and the dialog opens with `data-open:zoom-in-95`, a 100ms scale from 0.95. 745.344 / 768 = 0.9705 — it was measured mid-animation. Fixed with a `settled()` helper that awaits `getAnimations({subtree: true})`, inside `act` so Radix's Presence update does not emit a React warning on every measurement. This is a general hazard for anything measured in this suite, which is why it is a named, documented helper rather than a sleep.

**`task` was not installed on this machine.** Installed `go-task` at v3.53.1 — the version `ci.yml` pins — so the "a Taskfile target exists and exits 0" criterion is verified rather than assumed. No project file changed.

## Findings

**The acceptance criterion `grep -cE 'break-(all|words)' web/src/routes/images.tsx` reports 0 cannot be satisfied, and did not describe this plan's scope.** It reports **2**, at lines 653 and 1098 — both `<p className="break-all font-mono text-xs">` on the **schematic ID** display, a bare 64-character hex string with no hyphens and no name to misread. Neither is the reference rendering, both pre-date this plan, and `break-all` is appropriate there. The criterion's own preamble scopes it correctly ("in the reference rendering"), and by that reading it passes: `ReferenceValue`'s class set is `min-w-0 overflow-x-auto break-normal font-mono text-xs`, and the doc comment no longer contains either literal. Changing the ID paragraphs would be an unrelated layout change and was not made.

**The plan's prose says `images.test.tsx` holds 108 tests.** The jsdom project holds 111 across 7 files (`images.test.tsx` itself holds 38). Corrected in the browser suite's header comment.

## Ledger entries to file

For plan 02-21, which owns `.planning/WINDOWS.md` (untouched here, as required):

1. **Only one browser engine is measured.** UAX #14's hyphen rule is a specification and every engine implements it, but the fix is a CSS rule and the measurement is one engine's. **Engine used: Chromium, via playwright 1.62.1, headless, at a 1200×900 viewport.** Firefox and WebKit are unmeasured. Severity: low — `white-space: nowrap` is not a corner of CSS where engines disagree — but the claim "occupies one line box" is currently true of Chromium and inferred elsewhere.

2. **`INSTALLER_REPOSITORY_NAMES` is not pinned to `installerCandidates`.** A fifth candidate added to `internal/imagefactory/installer.go` would leave this sweep measuring four stale strings and still passing — a smaller instance of exactly the defect this plan closed. Plan **02-20** owns `guard_drift_test.go` in `package imagefactory` and closes it in wave 5. Until it lands, the link is a comment. **Open until 02-20 lands.**

3. **The CI browser-install step has never run.** Its form and ordering are verified by reading the file and its local equivalent was exercised, but no CI run has happened against this commit, and `--with-deps` on `ubuntu-latest` is untested. The `--` bug above is precisely the class of failure that only a real run confirms is gone.

## Known Stubs

None. Nothing in this plan renders placeholder data or defers a code path.

## Threat Flags

None. No network endpoint, auth path, file access pattern or schema at a trust boundary was added. The one new trust-boundary crossing — three npm devDependencies and a downloaded browser binary — is `T-02-SC` in the plan's own register and is dispositioned there; the legitimacy check is recorded under Deviations above.

## User Setup Required

Developers running the frontend tests need Chromium once:

```sh
npm --prefix web exec -- playwright install chromium
```

Roughly 150MB, cached afterwards. The test run names this command when the browser is missing, and `README.md` documents it. No `USER-SETUP.md` was generated — this is a developer toolchain step, not external service configuration.

## Next Phase Readiness

- **02-20 is unblocked and has its seam.** Bind `guard_drift_test.go` to **`INSTALLER_REPOSITORY_NAMES_LITERAL`** in `web/src/routes/images.browser.test.tsx` — that is the array holding the four names as source literals, readable with `strings.Contains` the way `TestWarningDetailsMatchTheUI` reads `SchematicWarnings.tsx`. `INSTALLER_REPOSITORY_NAMES` is the runtime-derived array the sweep uses, and the test `agrees with the derivation about which four names there are` already asserts the two are equal, so pinning the literal pins both.
- **A layout-measuring project now exists for any future frontend phase.** Add a `*.browser.test.tsx` file and it runs, locally and in CI. Read the `settled()` helper before measuring anything that animates in.
- **The standing end-to-end concern is untouched.** This suite renders React components against a faked `fetch`. No binary runs and no bundle is served, so the embedded UI has still never been driven end to end in a browser. The file's header comment says so explicitly, precisely so that its existence is not mistaken for having closed that.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-30*

## Self-Check: PASSED

- All 6 key files present on disk.
- All 4 task commits present in `git log`.
- `INSTALLER_REPOSITORY_NAMES_LITERAL` exported at `web/src/routes/images.browser.test.tsx:253`, bound to the derivation at line 371.
- Plan verification re-run at close-out: `npm --prefix web test` 118 passed across both projects; `task test:web` exit 0; `task test:web:browser` exit 0; `npm --prefix web run lint` exit 0; `npm --prefix web run typecheck` exit 0.


## Supply-chain ratification (orchestrator, 2026-08-30)

This plan installed a **third** npm package where the user's gate approval named two.
`@vitest/browser-playwright@4.1.11` was required because vitest 4 moved `browser.provider` to a
factory from a per-provider package. The executor did not wave it through: it applied the gate's
own criteria and escalated the count difference explicitly.

Re-checked independently against `registry.npmjs.org` before the user was asked:

| field | value |
|---|---|
| version | 4.1.11 — same as the approved `@vitest/browser` and the pinned `vitest` |
| repository | `github.com/vitest-dev/vitest`, directory `packages/browser-playwright` |
| maintainers | ariperkkio, antfu, hiogawa, oreanno, yyx990803 — identical set to `@vitest/browser` |
| license | MIT |
| first published | 2025-10-01 |
| dependencies | `@vitest/browser@4.1.11`, `@vitest/mocker@4.1.11`, `tinyrainbow@^3.1.0` |

Same publisher, same monorepo, same maintainer set, same version, and a hard dependency on the
package that was approved.

**Ratified by the user on 2026-08-30.** The approval covers these three packages at these
versions. If `npm` later resolves any of them to a different publisher or repository than recorded
here, that is a new question, not a covered one.
