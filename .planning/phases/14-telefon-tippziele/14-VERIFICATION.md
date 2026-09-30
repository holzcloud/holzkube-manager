---
phase: 14-telefon-tippziele
verified: 2026-09-30T12:59:34Z
status: human_needed
score: 15/16 must-haves verified
covered_files:
  - .planning/phases/14-telefon-tippziele/14-01-PLAN.md
  - .planning/phases/14-telefon-tippziele/14-01-SUMMARY.md
  - .planning/phases/14-telefon-tippziele/14-02-PLAN.md
  - .planning/phases/14-telefon-tippziele/14-02-SUMMARY.md
  - .planning/phases/14-telefon-tippziele/14-03-PLAN.md
  - .planning/phases/14-telefon-tippziele/14-03-SUMMARY.md
  - .planning/phases/14-telefon-tippziele/14-04-PLAN.md
  - .planning/phases/14-telefon-tippziele/14-04-SUMMARY.md
  - .planning/phases/14-telefon-tippziele/14-05-PLAN.md
  - .planning/phases/14-telefon-tippziele/14-05-SUMMARY.md
  - .planning/phases/14-telefon-tippziele/14-06-PLAN.md
  - .planning/phases/14-telefon-tippziele/14-06-SUMMARY.md
  - .planning/phases/14-telefon-tippziele/14-07-PLAN.md
  - .planning/phases/14-telefon-tippziele/14-07-SUMMARY.md
  - README.md
  - docs/guide.md
  - web/fixtures/demo.json
  - web/fixtures/host-helper-installed.json
  - web/package.json
  - web/scripts/layout-audit.mjs
  - web/scripts/layout-dump-compare.mjs
  - web/scripts/layout-routes.json
  - web/src/App.tsx
  - web/src/components/HostActions.test.tsx
  - web/src/components/HostActions.tsx
  - web/src/components/NodeActions.tsx
  - web/src/components/NodeSchedulingActions.tsx
  - web/src/components/Sidebar.tsx
  - web/src/components/WhatsNew.tsx
  - web/src/components/ui/button.tsx
  - web/src/components/ui/dialog.tsx
  - web/src/components/ui/dropdown-menu.tsx
  - web/src/components/ui/select.tsx
  - web/src/components/ui/sonner.browser.test.tsx
  - web/src/components/ui/sonner.tsx
  - web/src/fixtures.test.ts
  - web/src/layoutRoutes.test.ts
  - web/src/routeTree.ts
  - web/src/routes/host.browser.test.tsx
  - web/src/routes/host.test.tsx
  - web/src/routes/host.tsx
covered_digest: "v2:sha256:5630ee77f6d9df92f1ab4391597c1e537baec0899ce8072fdbe16cde009d2234"
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "D-11 hand test (ROADMAP criterion 4, phone half). On a phone at its native width, against a development daemon (never production) reachable over TLS on the LAN, with the holzkube-manager-host helper installed so /host offers its actions: open /host without zooming, tap Restart host, read the dialog, type the host name with the phone keyboard, see the confirm button become enabled, tap Keep running. Never submit."
    expected: "Everything readable and tappable without pinch-zoom; typing into the hostname field does not zoom the page (the Input primitive is text-base below md); the confirm enables only on the exact name; Keep running closes the dialog and focus returns to Restart host."
    why_human: "Needs a real phone, TLS (the Secure session cookie is not sent over http) and the installed helper; the audit measures sizes at 390 px in Chromium, not a thumb on a device. Truth is tagged verification: backstop in 14-07-PLAN, so presence plus the audit cannot close it."
  - test: "CR-01 dialog lock on a phone. Same setup as above, but let the order be placed against the development daemon's helper: tap confirm and, while the button reads Working... (or while a 428 sudo prompt sits on top), try Keep running, a tap outside and the close X."
    expected: "The dialog stays open, Keep running stays disabled until the confirm and placement have answered, and nothing is placed after an attempted close."
    why_human: "The jsdom test (HostActions.test.tsx, the three ways to close while pending) passes, but a touch tap outside, the real sudo prompt stacking and the in-flight timing on a device are not what jsdom simulates; 14-REVIEW-FIX marks CR-01 'fixed: requires human verification'."
  - test: "Repeat after 13-12 lands (fifth host action)."
    expected: "(1) D-08 for /host: a before-dump taken before 13-12's first class change and an after-dump after its last, compared with web/scripts/layout-dump-compare.mjs -- the new button reads ONLY AFTER and the boxes after it MOVED, and every such line is read; nothing else differs. (2) The D-11 hand test with five actions."
    why_human: "13-12 has not landed; this is a scheduled re-check, not something the current tree can show."
prohibitions_judged:
  - statement: "The layout audit never carries anything out (no confirm pressed, sudo dialog never typed into or submitted, no typed-confirmation field filled); a 2xx to a non-GET action request fails the run"
    tier: judgment
    llm_verdict: holds
    flag: "unverified-prohibition -- human review recommended"
    evidence: "OPENERS fill only #new-username and the New account password; no opener addresses #reset-confirm, #host-action-confirm or a confirm button; watchRequests allows only paths ending in /confirm (WR-02). This session's green run printed exactly two non-GET lines, POST /api/v1/users -> 428 at 390 and 1280. The WR-02 red log shows EXECUTED POST /api/v1/auth/sudo -> 204 against the deliberate fault."
  - statement: "No opener confirms anything: Power confirmation, Reset dialog and host action dialog are opened, measured and closed"
    tier: judgment
    llm_verdict: holds
    flag: "unverified-prohibition -- human review recommended"
    evidence: "Same code and run as above; the Host action dialog is measured with its confirm disabled (nothing typed), closed with Escape."
  - statement: "No tap-target change of this phase alters a control's size or position at 1280 px"
    tier: judgment
    llm_verdict: holds
    flag: "unverified-prohibition -- human review recommended"
    evidence: "Every added size class is max-md: prefixed (diff 567cd4c..HEAD). This session's LAYOUT_DUMP after-dump against 14-04's before-dump: compared 964 controls, 0 differ, exit 0. Not covered by the dump: the 14-02 pair spacing (a deliberate 13-UI-REVIEW fix, landed before the before-dump) and IN-06 (a desk click on a Reset disk row's text now toggles it -- behaviour, not size or position)."
  - statement: "No identifier of the operator's installation enters a fixture, picture, SUMMARY or commit message"
    tier: judgment
    llm_verdict: holds
    flag: "unverified-prohibition -- human review recommended"
    evidence: "internal/publicrepo ok in this session's go test; a pattern scan of the phase diff's added lines and of the phase's commit messages for IPv4, /home/<user>, mail addresses and MACs finds only 127.0.0.1; no README picture was committed."
---

# Phase 14: Telefon -- Tippziele, die ein Daumen trifft -- Verification Report

**Phase Goal:** Bei 390 px Breite trifft ein Daumen jedes Bedienelement -- auf jeder Route, auch in dem, was erst nach einem Tipp erscheint, und auf der neuen Host-Seite --, und die Layout-Prüfung in `task ci` wird rot, sobald eines darunter fällt. Oberhalb `md` bleibt alles, wie es ist.
**Verified:** 2026-09-30T12:59:34Z
**Status:** human_needed
**Re-verification:** No -- initial verification
**Where it ran:** the operator's Pi (aarch64), main checkout at 97137ec. Node from nvm, Go from `~/.local/go`, no `-race`. `holzkube-manager.service` untouched: `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST`, `NRestarts=0` before and after. Nothing installed. Logs in the session scratchpad, outside the repository.

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SC1: at 390 px every control on every route is at least 44 x 44 px; at 1280 px the interface is unchanged | VERIFIED | This session, `LAYOUT_DUMP=<scratch> ./bin/task test:layout` exit 0: "every control is at least 44px at 390px, on 27 routes and in 9 opened states", 71 `ok` lines (54 route passes + 17 opener passes). `layout-dump-compare.mjs 14-before.jsonl <after>` exit 0: `compared 964 controls, 0 differ` |
| 2 | SC2: the audit measures what appears after a tap (mobile navigation, Power menu, confirmation and sudo dialog), and a router route missing from the audit turns it red | VERIFIED | `OPENERS` in layout-audit.mjs: Sudo dialog, Navigation, What's new, Power menu, Power confirmation, Reset dialog, Select list, Host actions (helper installed), Host action dialog -- all `ok` at 390 in this run. Guard: `web/package.json` `test:layout` = `vitest run --project jsdom src/layoutRoutes.test.ts && node scripts/layout-audit.mjs`; Taskfile `test:layout` and ci.yml:141 both run that script |
| 3 | SC3: seen red three times separately (element < 44 px, dialog button < 44 px, route removed), each with a non-zero exit read from the command | VERIFIED | Reproduced here: (b) `max-md:size-11` removed from icon-sm -> exit 201, 4 SMALL lines `28x28 [dialog-close icon-sm]` (What's new, Power confirmation, Reset dialog, Host action dialog); (c) `/host` line removed from layout-routes.json -> exit 201, guard line "/host is in the router but not in web/scripts/layout-routes.json", 0 `ok` lines (no browser). Both restored, `git show HEAD:<file> \| cmp -` = 0. (a) not re-run; its log `$HOME/.cache/holzkube-manager-layout/14-07-a.log` holds 15 findings and 26 `button default` lines as claimed |
| 4 | SC4 measured half: /host with its actions and the confirmation dialog passes the audit at 390 px | VERIFIED | `ok 390px /host (14 controls, 27 items)` (helper missing, four buttons measured disabled), `ok 390px /host · Host actions (helper installed) (4 controls)`, `ok 390px /host · Host action dialog (4 controls)`; the helper-installed variant is parsed with hostSchema in fixtures.test.ts |
| 5 | SC4 phone half: on a phone the host can be viewed and an action taken to its typed confirmation without zooming | ? UNCERTAIN (human) | Backstop truth; D-11 hand test not performed (14-07-SUMMARY says so explicitly). Supporting code evidence only: viewport meta `width=device-width`, Input is `text-base` below md (no iOS focus zoom) |
| 6 | Route list is the router's leaves (27), the hand list is gone; guard fails on missing/extra/duplicate/parameterless entries and on a vacuous tree (>= 20 leaves, no 'undefined' key) | VERIFIED | `web/src/routeTree.ts` exports `routeTree`, imported by the guard; `layoutRoutes.test.ts` three tests; `ROUTE_ENTRIES` read from JSON in the audit; guard 3/3 passed in each rerun |
| 7 | An opener that cannot open/close (OPENER), measures nothing (EMPTY) or triggers a 2xx action (EXECUTED) fails the run; the sudo grant counts as EXECUTED (WR-02) | VERIFIED | `openMeasureClose` / `runOpener` / `watchRequests` code; allowlist is only `path.endsWith('/confirm')`; WR-02 logs: gap run `POST /api/v1/auth/sudo -> 204` green, red run `EXECUTED POST /api/v1/auth/sudo -> 204` |
| 8 | Disabled controls are measured; each SMALL line names the primitive (data-slot, data-size) | VERIFIED | `findSmallTargets` has no disabled skip; red (b) output carries `[dialog-close icon-sm]` |
| 9 | Primitives at 44 px below md: every Button size, the dialog close X (icon stays 22 px from the corner, header `max-md:pr-8`), one-line menu/select items | VERIFIED | button.tsx, dialog.tsx, dropdown-menu.tsx, select.tsx diffs, all `max-md:`; green audit; red (b) |
| 10 | Toast dismiss >= 44 x 44 at 390 px (backstop), held by a Chromium test that also holds the 24 px chip on the desk | VERIFIED | `sonner.browser.test.tsx` (390 and 1280 cases, requires at least one `[data-close-button]`) passed inside this session's `test:web` (61 files, 741 tests); its red log `14-05-toast-red.log` exists |
| 11 | Mobile navigation: 44 px links, the drawer scrolls, no CUT OFF; What's new version button and chips 44 px | VERIFIED | Sidebar.tsx `max-md:min-h-11`, `max-md:overflow-y-auto`; WhatsNew.tsx `max-md:min-h-11`; `ok 390px / · Navigation (15 controls)` with `checkDrawer` (slid in, scrolls) |
| 12 | Reset dialog: checkboxes and radios tapped through 44 px labels; each disk row one label | VERIFIED | NodeActions.tsx label per disk row `max-md:min-h-11`; `ok 390px /nodes/m-cp-1 · Reset dialog (11 controls)`; demo.json reset preview parsed with resetPreviewSchema |
| 13 | Host action dialog behaviour: focus back to the opener on cancel (or to the reason line when the opener was turned off, WR-01); cannot close while an order is in flight (CR-01); follows a reason arriving while open; Problem clears on typing | VERIFIED | HostActions.tsx `onCloseAutoFocus`, `onOpenChange` guarded by `run.isPending`, Keep running `disabled={run.isPending}`; tests in HostActions.test.tsx (three-ways-to-close while pending, focus with and without reason, reason above footer) passed in this session's `test:web`. Phone check of CR-01 kept as a human item (below) |
| 14 | /host copy and layout: "back" without "up since .", the waiting notice in an always-mounted polite region (WR-03), the pair 16 px apart above md, the "no answer" and "recorded {time}" copy asserted | VERIFIED | host.tsx `<div aria-live="polite">` always mounted; HostActions.test.tsx:1426-1436 (no "up since"), :1345 ("no answer ..."), :1415-1419 ("recorded {time}"); host.test.tsx:1651 (region there, empty, before the sentence); host.browser.test.tsx 16 px gap -- all passed in `test:web` |
| 15 | D-08 instrument: LAYOUT_DUMP (refused inside the repo), comparer that fails on difference, one-sided control and empty dump; noise floor 0; no dump committed | VERIFIED | layout-dump-compare.mjs `load`/differences code; `14-04-noise-compare.txt` exists; this session's compare 964/0; `git status --porcelain` clean after the runs |
| 16 | README and guide say what the phone check covers; the gate passes on the Pi | VERIFIED | README.md:110-111 (after WR-04: "the menus and dialogs behind its main actions"); guide "On a phone" adds route guard, opened states, disabled measured, nothing carried out, desk measured once. Gate: see "The gate" below |

**Score:** 15/16 truths verified (0 present, behavior-unverified; 1 uncertain, routed to human verification)

### The gate (`./bin/task ci`, run once here after the review fixes)

`./bin/task ci` exited **201**, read from the command itself. Every step before the last passed in that run: `npm ci`, web lint, `test:web` (61 files, 741 tests passed, both projects), build, golangci-lint (`0 issues.`), `go test ./...` (every package `ok`, internal/upgrade included). It failed in `test:layout` before anything was measured: vitest could not start the fork worker for `layoutRoutes.test.ts` ("Timeout waiting for worker to respond", 101 s). At that moment the Pi's load average was 25 on four cores, driven by another project's WebKit UAT run (not this repository). This is not one of the two known flakes, and not a test failure: no test ran. `test:next` (not a gate) was not reached.

`./bin/task test:layout` then ran three more times in this session on the same HEAD: the guard passed 3/3 in the (b) run and in the green run, and the green run exited 0. So every gate step has an exit 0 on 97137ec in this session, but not in one uninterrupted `task ci` run. Recorded as Info; a clean single run when the Pi is idle would remove the caveat.

The arm64 daemon the gate built: `file bin/holzkube-managerd` reports ARM aarch64. No amd64 build was made here.

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `web/src/routeTree.ts` | route tree as a side-effect-free module | VERIFIED | `export const routeTree`, imported by App.tsx and the guard |
| `web/scripts/layout-routes.json` | 27 router leaves, parameter examples | VERIFIED | 27 entries, `/nodes/m-cp-1`, `when` on /setup and /login |
| `web/src/layoutRoutes.test.ts` | route guard | VERIFIED | 3 tests, red (c) reproduced |
| `web/scripts/layout-audit.mjs` | JSON routes, 9 openers, request monitor, dump | VERIFIED | 1365 lines, all passes run in this session |
| `web/scripts/layout-dump-compare.mjs` | D-08 comparer | VERIFIED | used in this session, 964/0 |
| `web/fixtures/host-helper-installed.json` | helper-installed actions | VERIFIED | `"available": true`, parsed with hostSchema |
| `web/fixtures/demo.json` | m-cp-1 reset preview | VERIFIED | parsed with resetPreviewSchema |
| `web/src/components/ui/{button,dialog,dropdown-menu,select,sonner}.tsx` | 44 px below md | VERIFIED | `max-md:` classes as planned |
| `web/src/components/ui/sonner.browser.test.tsx` | toast backstop | VERIFIED | passes in the browser project |
| `web/src/components/{Sidebar,WhatsNew,NodeActions,NodeSchedulingActions}.tsx` | single places | VERIFIED | `max-md:` classes as planned |
| `web/src/components/HostActions.tsx`, `web/src/routes/host.tsx` | 13-UI-REVIEW items, CR-01, WR-01, WR-03 | VERIFIED | code and tests as above |
| `README.md`, `docs/guide.md` | phone line and "On a phone" | VERIFIED | IN-01 inaccuracy remains (below) |

### Key Link Verification

| From | To | Via | Status |
|------|----|-----|--------|
| web/package.json | layoutRoutes.test.ts | `layoutRoutes.test.ts && node scripts/layout-audit.mjs` | WIRED |
| Taskfile `ci` | `test:layout` | `- task: test:layout` in the ci chain; ci.yml:141 runs the same npm script | WIRED |
| layoutRoutes.test.ts | routeTree.ts | `from '@/routeTree'`, `createRouter` with memory history | WIRED |
| layout-audit.mjs | layout-routes.json | `ROUTE_ENTRIES` read at start, `when` selects /setup and /login | WIRED |
| layout-audit.mjs | /api/v1/users sudo gate | Sudo dialog opener -> `REQUEST POST /api/v1/users -> 428` in this run | WIRED |
| layout-audit.mjs | host-helper-installed.json | per-page `page.route` override for the two /host openers | WIRED |
| fixtures.test.ts | host-helper-installed.json, demo reset preview | hostSchema / resetPreviewSchema parse | WIRED |
| dialog.tsx | button.tsx | close X is `size="icon-sm"` | WIRED |
| sonner.browser.test.tsx | Toaster.tsx | `notify.info` on the app's Toaster | WIRED |
| HostActions | HostActionDialog | `reason`, `opener`, `reasonLine` props | WIRED |

### Data-Flow Trace (Level 4)

Not applicable in the usual sense: the phase adds measurement and CSS classes, not data-rendering components. The audit's own data path was checked: the helper-installed override reaches the page (the `enabled` check would print OPENER otherwise; the state printed `ok ... (4 controls)`), and the Reset dialog renders its disk rows from the fixture (`expect` requires a checkbox; 11 controls measured).

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Audit green on HEAD with desk dump | `LAYOUT_DUMP=<scratch>/verify-after.jsonl ./bin/task test:layout` | exit 0; 27 routes, 9 opened states; only non-GET lines `POST /api/v1/users -> 428` x2 | PASS |
| Desk unchanged | `node web/scripts/layout-dump-compare.mjs 14-before.jsonl verify-after.jsonl` | exit 0, `compared 964 controls, 0 differ` | PASS |
| Red (b): dialog X under 44 | icon-sm without `max-md:size-11`, `./bin/task test:layout` | exit 201, 4 findings `28x28 [dialog-close icon-sm]`; restored, cmp 0 | PASS (went red) |
| Red (c): route removed | `/host` line dropped, `./bin/task test:layout` | exit 201, guard line, no browser; restored, cmp 0 | PASS (went red) |
| Full gate | `./bin/task ci` | exit 201, worker-start timeout under foreign load; all earlier steps green | see "The gate" |

### Probe Execution

Step 7c: no `scripts/*/tests/probe-*.sh` declared or present for this phase; SKIPPED.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| MOB-01 | 14-04, 14-05, 14-06, 14-07 | 44 x 44 at 390 px for every control, desktop unchanged | SATISFIED | Truths 1, 9-12, 15; REQUIREMENTS.md marks it complete |
| MOB-02 | 14-01, 14-03, 14-07 | a guard in the layout check measures tap targets on all routes and goes red below | SATISFIED | Truths 2, 3, 6-8; REQUIREMENTS.md marks it complete |
| MOB-03 | 14-02, 14-03, 14-06, 14-07 | the host page meets MOB-01 and is operable on a phone | NEEDS HUMAN | Measured half verified (truth 4, 13, 14); phone operability open (truth 5). REQUIREMENTS.md correctly still shows it Pending |

No orphaned requirements: REQUIREMENTS.md maps exactly MOB-01..03 to Phase 14, and every one is claimed by at least one plan.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (phase diff) | -- | TBD/FIXME/XXX/TODO/HACK | none found | -- |
| docs/guide.md | 1669-1670 | "measures each on its own, at both widths" while the Navigation drawer is 390 only (14-REVIEW IN-01, not fixed) | Info | Guide slightly overstates; no behaviour impact |
| web/src/components/NodeActions.tsx | 501-516 | Disk-row `<label>` not breakpoint-scoped: a desk click on the text now toggles the checkbox (IN-06) | Info | Behaviour change above md; size/position unchanged (dump 0 differ); still gated by the typed phrase and token |
| web/src/components/NodeActions.tsx | 402-422 | Reset flag checkboxes sit ~14 px below their heading line on the phone (IN-03) | Info | Visual alignment only |
| web/scripts/layout-audit.mjs | 164-165 | LAYOUT_DUMP inside-repo test accepts a `..name` folder and a symlinked file (IN-02) | Info | Only matters for a deliberately odd dump path |

IN-04 (hard-coded ids) and IN-05 (redundant assertion) also remain; both Info, out of the fix scope by 14-REVIEW-FIX.

### Human Verification Required

### 1. D-11 hand test (ROADMAP criterion 4, phone half)

**Test:** On a phone at its native width, against a development daemon (never production) reachable over TLS on the LAN with the host helper installed: open /host without zooming, tap Restart host, read the dialog, type the host name with the phone keyboard, watch the confirm enable, tap Keep running. Never submit.
**Expected:** Readable and tappable without zoom; no zoom on focusing the hostname field; confirm enables only on the exact name; Keep running closes and focus returns to Restart host.
**Why human:** Needs a phone, TLS and the installed helper; a backstop truth that the 390-px Chromium audit cannot close.

### 2. CR-01 dialog lock on a phone

**Test:** Same setup; place the order against the development daemon and, while it is in flight (Working..., or with a 428 sudo prompt on top), try Keep running, a tap outside and the close X.
**Expected:** The dialog stays open, Keep running stays disabled, nothing happens after an attempted close.
**Why human:** Touch, the real sudo prompt and device timing are outside what the jsdom test simulates; 14-REVIEW-FIX marks it "fixed: requires human verification".

### 3. Repeat after 13-12

**Test:** Once the fifth host action lands: D-08 before/after dumps around 13-12's class changes, compared, every ONLY AFTER / MOVED line read; then the D-11 hand test with five actions.
**Expected:** Only the new button and the boxes after it differ; the hand test passes with five actions.
**Why human:** 13-12 has not landed.

### Gaps Summary

No gaps. The measured goal holds on the current HEAD, checked in this session rather than taken from the SUMMARYs: 27 router routes and 9 opened states pass at 390 px, the desk dump compares 964 controls with 0 differences, and two of the three red checks were reproduced here with exit 201 and byte-identical restores. What remains open is what a phone and a person must show: the D-11 hand test (criterion 4's phone half, so MOB-03 stays pending), a phone check of the CR-01 lock, and the re-checks owed after 13-12. The one full `task ci` run here was interrupted by a vitest worker-start timeout under another project's load; every step has an exit 0 on this HEAD, but not in one uninterrupted run.

---

_Verified: 2026-09-30T12:59:34Z_
_Verifier: Claude (gsd-verifier)_
