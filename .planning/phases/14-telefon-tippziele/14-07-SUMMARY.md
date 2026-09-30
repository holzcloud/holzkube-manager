---
phase: 14-telefon-tippziele
plan: 07
subsystem: testing
tags: [layout-audit, red-checks, d-08, d-10, d-11, readme, guide, gate]
status: complete

requires:
  - "14-06: ./bin/task test:layout green, 27 routes, 9 opened states"
  - "14-04: the before-dump $HOME/.cache/holzkube-manager-layout/14-before.jsonl (964 controls) and layout-dump-compare.mjs"
provides:
  - "D-10: the audit seen red three times, separately, each with exit 201 read from ./bin/task test:layout"
  - "D-08: the 1280 after-dump compares `compared 964 controls, 0 differ` against the before-dump"
  - "README phone line and guide 'On a phone' describe the route guard, the opened states, disabled controls, nothing carried out, the desk measured once"
  - "./bin/task ci exit 0 on the Pi, production service untouched, fresh arm64 and amd64 builds of the architecture asked for"
affects: [13-12, phase-14-verification]

actuals:
  tokens: 800
  tasks: 3
  commits: 1
plan_head_before: 26b6f0926797520a93c7d13a1ade61e040ea8a0c
plan_head_after: 028b60e962e4cca5b1ac45ee0362b81556e18da1

tech-stack:
  added: []
  patterns:
    - "A red check is: save to the scratchpad, one edit, run, read the exit from the command, restore, cmp against git show HEAD:, one green run before the next"
    - "A re-rendered picture is committed only when its pixels differ from HEAD's (PIL ImageChops difference), not when its bytes do"

key-files:
  created: []
  modified:
    - README.md
    - docs/guide.md

key-decisions:
  - "No README picture is committed: all thirteen files the renderer rewrote are pixel-identical to HEAD, host.png and phone.png included"
  - "The login.test.tsx timeout in the first gate run is logged in deferred-items.md, not fixed and not masked; the whole gate was rerun and passed"
  - "D-11 hand test not performed; ROADMAP criterion 4's phone half stays open (human_needed)"

requirements-completed: [MOB-01, MOB-02]
requirements-partial: [MOB-03]

duration: 50min
completed: 2026-09-30
---

# Phase 14 Plan 07: Red checks, the desk measured, the docs and the gate Summary

**The layout audit was seen red three times, each on its own, each with `./bin/task test:layout` exiting 201: a Button under 44 px, the dialog close X at 28 px, and `/host` taken out of the route list. The 1280 after-dump compares `compared 964 controls, 0 differ` against 14-04's before-dump. The README and the guide now say what the phone check covers. `./bin/task ci` exits 0 on the Pi with the production service untouched. The D-11 hand test was not performed, so ROADMAP criterion 4's phone half is open (human_needed).**

## Where it ran

On the operator's Pi (aarch64). Node came from nvm and Go from `~/.local/go`. No `-race` run: ThreadSanitizer refuses this kernel, so the race verdict belongs to CI. The amd64 binary was compiled here, not executed. Nothing was installed, pushed, released or tagged. Every audit run started its own daemon on 127.0.0.1 with a temporary data directory. Logs and dumps are in `$HOME/.cache/holzkube-manager-layout/14-07*`, outside the repository.

**Production service, before and after** (`systemctl show holzkube-manager.service -p ActiveEnterTimestamp -p NRestarts`):

| When | ActiveEnterTimestamp | NRestarts |
|------|----------------------|-----------|
| plan start, 10:52 UTC | Tue 2026-09-29 20:25:38 CEST | 0 |
| before the gate, 11:15 UTC | Tue 2026-09-29 20:25:38 CEST | 0 |
| after the gate and builds, 11:41 UTC | Tue 2026-09-29 20:25:38 CEST | 0 |

Identical. `/usr/local/sbin/holzkube-manager-host` and `/etc/systemd/system/holzkube-manager-host.path` do not exist (`test ! -e` on both, exit 0).

## Performance

- **Duration:** about 50 min (10:52 to 11:42 UTC). The two full gates took 4 and 20 minutes.
- **Tasks:** 3. Only Task 2 has a commit. Task 1 changed no file, because the D-08 comparison found no difference. Task 3 changed no file, because the gate found nothing in this phase's files.
- **Files:** 2 modified (README.md, docs/guide.md).

## Task 1: three red checks and the desk (no commit)

Precondition met: `14-before.jsonl` exists, 964 lines. Before trusting the comparison, I checked that the instrument had not changed: `git log --oneline 27ace4b..HEAD -- web/scripts/layout-audit.mjs` printed nothing. The before-dump was taken with the same audit, so it was not retaken.

For each check I saved the file to the session scratchpad, made one edit, and ran `./bin/task test:layout > $HOME/.cache/holzkube-manager-layout/14-07-<x>.log 2>&1`. I read the exit code from that command itself. Then I restored the file from the saved copy, compared it with `cmp` against `git show HEAD:<file>`, and ran the audit green once before the next check.

| Check | Fault put in | Exit (read from the command) | Restored | Green run after |
|-------|--------------|------------------------------|----------|-----------------|
| (a) | `max-md:h-11` removed from Button `default` | **201** | cmp=0 | exit 0 |
| (b) | `max-md:size-11` removed from Button `icon-sm` | **201** | cmp=0 | exit 0 |
| (c) | the `/host` line removed from `web/scripts/layout-routes.json` | **201** | cmp=0 | exit 0 |

Each green run ended with `Nothing out of reach at 390px and 1280px, every control is at least 44px at 390px, on 27 routes and in 9 opened states.`

**(a) An element under 44 px.** `15 finding(s) across routes, openers and widths.`, all `SMALL` at 390px. 26 detail lines carry `[button default]`.
```
  SMALL      390px  /setup  (1)
  SMALL      390px  /login  (1)
  SMALL      390px  /clusters  (2)
  ...
  SMALL      390px  /settings · Sudo dialog  (2)
  SMALL      390px  /nodes/m-cp-1 · Power confirmation  (2)
  SMALL      390px  /nodes/m-cp-1 · Reset dialog  (2)
  SMALL      390px  /host · Host action dialog  (2)
              <button> 310x32 [button default] "Create account" ...
              <button> 310x32 [button default] "Sign in" ...
              <button> 117x32 [button default] "Import cluster" ...
```
The 8 route lines not shown: /kubernetes/config, /kubernetes/maintenance, /config, /provision, /upgrades, /images, /audit, /settings.

**(b) A dialog button under 44 px.** `4 finding(s) across routes, openers and widths.` 4 lines of the form `<route> · <opener>` and 4 detail lines with `[dialog-close icon-sm]`:
```
  SMALL      390px  / · What's new  (1)
  SMALL      390px  /nodes/m-cp-1 · Power confirmation  (1)
  SMALL      390px  /nodes/m-cp-1 · Reset dialog  (1)
  SMALL      390px  /host · Host action dialog  (1)
              <button> 28x28 [dialog-close icon-sm] "Close" ...
```
The sudo dialog has no line here. It is measured, but it has no close X, so this fault does not reach it.

**(c) A route taken out of the list.** The route guard failed before any browser started:
```
     × lists every router leaf once, names none that is gone, and gives each parameter an example
+   "/host is in the router but not in web/scripts/layout-routes.json: the layout audit would never open it.",
```
`grep -c '^  ok '` on the log is 0, so the browser run was never reached.

**D-08, the desk.** `LAYOUT_DUMP=$HOME/.cache/holzkube-manager-layout/14-after.jsonl ./bin/task test:layout` gave **exit 0** and `LAYOUT_DUMP: 964 controls at 1280px written to …`. Then `node web/scripts/layout-dump-compare.mjs 14-before.jsonl 14-after.jsonl` gave **exit 0** and printed:
```
compared 964 controls, 0 differ
```
N = 964 > 0. No difference, so button.tsx needed no fix and has no commit. 14-02's 16 px pair spacing on /host, the phase's one deliberate change above md, landed before the before-dump. It is therefore not in this comparison.

## Task 2: README, guide, pictures (028b60e)

- **README.md:** the phone line now reads "**On a phone.** Every screen, one-handed, and every menu and dialog a tap opens: the build fails when a control on any of them is under 44px." There is no new screenshot, and the alpha note is unchanged.
- **docs/guide.md, "On a phone":** six paragraphs were added after the existing ones. They cover:
  - the route list is the router's, `web/scripts/layout-routes.json` held by `src/layoutRoutes.test.ts` before the browser starts (27 routes)
  - the nine opened states, each measured in its own scope, and the first run's seven findings (14-03)
  - disabled controls are measured
  - the audit never carries anything out, and a 2xx to an action fails the run
  - the desk is held by `max-md:` and was measured once (964 controls, 0 differ); the reason a checked-in baseline was rejected is taken from CONTEXT
  - the toast dismiss button is held by a component test

  Every figure comes from a SUMMARY of this phase. "Below 768px a table is not a table" is unchanged.
- **Pictures:** `./bin/task build` (exit 0) and `node web/scripts/readme-images.mjs` (exit 0), rendered from `web/fixtures/demo.json`. The renderer rewrote 13 files. I compared each against HEAD pixel by pixel (PIL `ImageChops.difference`). **All 13 are pixel-identical (bbox None, 0 pixels)**, so none is committed and all were restored with `git checkout -- <file>`:
  - docs/brand/banner.png, docs/brand/social.png
  - docs/screenshots/app-detail.png, apps.png, clusters.png, host.png, kubernetes.png, node-hardware.png, phone.png, wall.png
  - web/public/apple-touch-icon.png, icon-192.png, icon-512.png
- `go test ./internal/publicrepo/ -count=1`: ok, exit 0.
- `grep -c 'layout-routes.json' docs/guide.md` = 1, `grep -c 'layoutRoutes.test.ts' docs/guide.md` = 1. After the commit, `git status --porcelain` was empty.

## Task 3: the gate (no commit)

**First run:** `./bin/task ci` gave **exit 201** (11:15 to 11:19 UTC). One jsdom test failed, `src/routes/login.test.tsx > the sign-in page on an SSO-only address > links to the address the local account works on`, with `Unable to find role="link"` after 1457 ms. testing-library's `findByRole` waits 1000 ms by default. Run alone, the file passed 3 of 3 times (exit 0 each). Neither the test nor `login.tsx` changed in Phase 14. It is logged in `deferred-items.md`, not fixed and not masked.

**Second run, the whole gate:** `./bin/task ci` gave **exit 0**, read from the command itself (11:20 to 11:40 UTC):
- `npm --prefix web ci` and web lint: passed
- vitest, both projects: `Test Files 61 passed (61)`, `Tests 733 passed (733)`
- build: passed
- golangci-lint 2.13.1 (pinned): `0 issues.`
- `go test ./... -count=1`: every package `ok`, no FAIL. The known internal/upgrade flake did not occur (upgrade ok in 76.9 s).
- test:layout: route guard `3 passed (3)`, then `Nothing out of reach at 390px and 1280px, every control is at least 44px at 390px, on 27 routes and in 9 opened states.`
- test:next (go1.27.1, not a gate): no FAIL lines

**Architecture check, fresh builds into the scratchpad.** Both `managerd-*` files were deleted first. The automated check exited 0 (`arch=0`).
- `GOOS=linux GOARCH=arm64 go build -o $SCRATCH/managerd-arm64 ./cmd/holzkube-managerd` → `file -b`: `ELF 64-bit LSB executable, ARM aarch64, version 1 (SYSV), dynamically linked, interpreter /lib/ld-linux-aarch64.so.1, …`
- `GOOS=linux GOARCH=amd64 GOTOOLCHAIN=go1.26.7 go build -o $SCRATCH/managerd-amd64 ./cmd/holzkube-managerd` → `file -b`: `ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked, …`

arm64 is the architecture that runs natively here. The amd64 binary was compiled, not executed.

## D-11 hand test: not performed

The operator has not reported a hand test in this session, and this executor neither performed one nor asked for one. It needs three things, none of which is present:

1. **a phone**, at its native width
2. **a daemon reachable on the LAN over TLS.** A phone does not send the Secure session cookie over http (HANDOVER §3.1). It must never be the production daemon.
3. **a /host whose actions are available.** That requires the holzkube-manager-host helper to be installed on the Pi, which is also an open Phase 13 UAT item.

The steps, once those three exist: open /host without zooming, tap "Restart host", read the dialog, type the host name with the phone keyboard, see the confirm button become enabled, and tap "Keep running". Never submit. For verification, the orchestrator is to put this to the operator as a choice: (A, recommended) install the helper on the Pi and run the hand test against a development daemon with its self-signed TLS on a LAN port, or (B) leave the record at "not performed".

## ROADMAP criteria, Phase 14

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | At 390 px every control on every route is at least 44 × 44 px; at 1280 px nothing changed | met | test:layout exit 0 in the gate: "every control is at least 44px at 390px, on 27 routes and in 9 opened states". D-08: `compared 964 controls, 0 differ` (this plan) |
| 2 | The audit measures what appears after a tap, and a route in the router but not in the audit turns it red | met | 9 opened states (14-03, 14-06). The route guard `layoutRoutes.test.ts` runs before the browser (14-01), and red check (c) here shows it |
| 3 | Seen red three times, separately, exit non-zero read from `task test:layout` | met | (a), (b) and (c) above, exit 201 each, each restored with cmp=0 and followed by a green run |
| 4 | `/host` with its actions and dialog passes at 390 px, **and** on a phone the host can be viewed and an action taken to its typed confirmation without zooming | **measured half met; phone half open -- human_needed (hand test not performed)** | The measured half: `ok 390px /host (14 controls, 27 items)`, plus `Host actions (helper installed)` and `Host action dialog` green at 390 and 1280 (14-06, and the gate here). The phone half has no evidence until D-11 is performed |

MOB-03 therefore stays partial. Its automated part is held, and whether a person can operate /host on a phone is the open hand test.

## Repeat after 13-12

13-12 adds a fifth host action ("only check for updates") and changes /host. Once it lands:

1. **Repeat the D-08 comparison for /host.**
   - Take a before-dump before 13-12's first class change: `LAYOUT_DUMP=<outside the repo>/13-12-before.jsonl ./bin/task test:layout`.
   - Take an after-dump after its last class change.
   - Run `node web/scripts/layout-dump-compare.mjs <before> <after>`.

   The new button will show as `ONLY AFTER`, and the boxes after it will be `MOVED`. Those lines have to be read, not waved through. Nothing else may differ.
2. **Repeat the D-11 hand test** with five actions.

The audit itself needs no change for a fifth button. The "Host actions (helper installed)" state measures the whole group, and 14-02's spacer and gap test are keyed to named buttons, not to a count.

## Deviations from Plan

1. **[Plan expectation not met, recorded] host.png and phone.png did not change.** The plan expected host.png to change because of 14-02's 16 px spacing, and phone.png because the phone routes' controls changed. Both came out pixel-identical to HEAD.
   - host.png: the HEAD picture was rendered at a1f1679 (05:40), before 14-02 (11:43). It already shows 16 px between "Restart service" and "Restart host", measured in the pixel row at y=93: border at x=1140, next border at x=1157. So in the real shell at 1440 px the old `w-4` spacer already drew 16. The 32 px that 14-02 fixed was measured at 1200 px in the component browser test.
   - phone.png: its three screens (/nodes/m-cp-1, /kubernetes/apps, /clusters) show no control this phase resized at the moment the picture is taken.

   So no picture is committed, and the frontmatter `files_modified` entries for the two PNGs do not apply.
2. **[Rule 3] First gate run red on an unrelated timeout.** Covered above. It is logged in deferred-items.md, and the whole gate was rerun, not just the failing file.
3. **Tasks 1 and 3 have no commit of their own.** Their only files were to be committed only if something was found, and nothing was. Their results are in this SUMMARY and in the logs outside the repository. The deferred-items.md entry goes into the SUMMARY commit.
4. **Committed on main**, as the orchestrator directed for this sequential run. The per-commit protected-branch assertion in the executor protocol was overridden by that instruction, as in 14-01 to 14-06.

## Known Stubs

None.

## Threat Flags

None. T-14-13: production service properties identical before and after. T-14-14: the helper is absent. T-14-15: publicrepo ok, and the pictures were rendered only from `web/fixtures/demo.json` (none committed). The hand-test record names no address or host name.

## Self-Check: PASSED

- FOUND: README.md (extended phone line), docs/guide.md (`layout-routes.json`, `layoutRoutes.test.ts`)
- FOUND: commit 028b60e
- FOUND: $HOME/.cache/holzkube-manager-layout/14-07-a.log, 14-07-b.log, 14-07-c.log, 14-after.jsonl, 14-07-compare.txt, 14-07-ci.log, 14-07-ci-2.log
