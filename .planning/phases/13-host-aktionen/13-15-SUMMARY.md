---
phase: 13-host-aktionen
plan: 15
subsystem: host-actions
tags: [host-actions, update-check, root-helper, web, docs, layout, tdd]
status: complete

requires:
  - phase: 13-host-aktionen
    plan: 14
    provides: actions.outdated in GET /api/v1/host, REASON.helperOutdated, the check's button off while the other four stay on
  - phase: 13-host-aktionen
    plan: 12
    provides: the check-update action and the D-08 before-dump 13-12-before.jsonl
provides:
  - HostHelperOutdatedNotice ("Check for updates needs a newer helper") in the helper notice's place, with what to reinstall and the install commands
  - one shared frame (HelperInstallNotice) for both helper notices; the helper notice says five orders and five buttons
  - "outdated": [] in web/fixtures/host-helper-installed.json, and a raw-JSON check of the key in both /host fixtures
  - README, docs/guide.md, deploy/HOST-HELPER.md and docs/api-contract.md describing five host actions and the check that installs nothing
  - docs/screenshots/host.png with five buttons
  - the D-08 comparison after 13-12, read line by line
affects: [phase-13-verification]

actuals:
  tokens: 4560
  tasks: 3
  commits: 2
plan_head_before: 4e592705c8798316898df20638f22507554c7b5a
plan_head_after: b7ecc83f79548d170e6daf23018215d430d9b28f

tech-stack:
  added: []
  patterns:
    - "Two notices that must never stand together share one frame component, and the route decides which one renders from the same answer"
    - "A fixture key a schema default would supply is checked in the raw JSON, before the schema touches it"

key-files:
  created: []
  modified:
    - web/src/components/HostActions.tsx
    - web/src/routes/host.tsx
    - web/src/routes/host.test.tsx
    - web/src/routes/host.browser.test.tsx
    - web/src/fixtures.test.ts
    - web/fixtures/host-helper-installed.json
    - README.md
    - docs/guide.md
    - deploy/HOST-HELPER.md
    - docs/api-contract.md
    - docs/screenshots/host.png

key-decisions:
  - "The older-helper notice renders only when nothing is missing, even though the server already sends outdated empty while anything is missing: the page does not rely on that to keep the two notices apart"
  - "Both helper notices share one frame (HelperInstallNotice), so the box, the list, the self-scrolling commands and the closing line have one set of classes"
  - "D-08's key (route, opener, index, tag) turns an index shift across a tag boundary into ONLY BEFORE/ONLY AFTER pairs; the verdict on 'no control disappeared' comes from re-keying /host by opener, tag and name, which shows none"
  - "No changelog entry and no release: none was asked for"

requirements-completed: [HACT-04]

coverage:
  - id: D1
    description: "With an installed but outdated helper, /host names what to reinstall with the install commands; never beside the helper notice, never in a container"
    requirement: HACT-04
    verification:
      - kind: unit
        ref: "web/src/routes/host.test.tsx#the older-helper notice"
        status: pass
  - id: D2
    description: "At 390 px the older-helper notice fits, its commands scroll inside their box, the check is off and the other four on"
    requirement: HACT-04
    verification:
      - kind: automated_ui
        ref: "web/src/routes/host.browser.test.tsx#fits the older-helper notice with both pieces"
        status: pass
  - id: D3
    description: "Both /host fixtures carry outdated as an array in their raw JSON"
    requirement: HACT-04
    verification:
      - kind: unit
        ref: "web/src/fixtures.test.ts#carries `outdated` in both /host fixtures as written"
        status: pass
  - id: D4
    description: "README, guide, HOST-HELPER.md, contract and host.png describe five host actions and the check that installs nothing"
    requirement: HACT-04
    verification:
      - kind: other
        ref: "go test ./internal/publicrepo/ ./internal/host/hostaction -run 'TestGuideKeepsTheDaemonsHardening|TestInstallCommandsMatchTheGuide'"
        status: pass
  - id: D5
    description: "D-08 after 13-12: every difference in /host, one new control per actions scope, no control changed size"
    requirement: HACT-04
    verification:
      - kind: automated_ui
        ref: "LAYOUT_DUMP=... ./bin/task test:layout + layout-dump-compare.mjs + re-key script"
        status: pass
  - id: D6
    description: "Phone hand test with five actions (D-11) and installing/probing the helper on the Pi"
    requirement: HACT-04
    verification:
      - kind: manual_procedural
        ref: "human_needed: operator's phone, and the operator's decision to install root code"
        status: unknown

duration: ~75min
completed: 2026-09-30
---

# Phase 13 Plan 15: The older-helper notice, five actions documented, D-08 after 13-12 Summary

**An operator whose helper predates the check now gets a note that names the piece to reinstall and gives the install commands. README, guide, HOST-HELPER.md, the contract and host.png all describe five host actions. The desk at 1280 px differs from the pre-13-12 dump only by the new button in the /host header, and `./bin/task ci` passed (exit 0) on the Pi.**

## Where it ran

On the operator's Raspberry Pi 5 (aarch64). Go came from `~/.local/go` and Node from nvm. There was no `-race`: ThreadSanitizer refuses this kernel, so the race verdict belongs to CI. The amd64 binary was only compiled, not run. Nothing was installed on the host, and nothing was pushed, tagged or released.

## Tasks

1. **Task 1: the older-helper notice, the helper notice's count, the fixtures and the 390 px measurement.** Commit `f5ec24d` (feat).
2. **Task 2: README, guide, HOST-HELPER.md, contract and host.png.** Commit `b7ecc83` (docs).
3. **Task 3: D-08 after 13-12 and the whole gate.** Nothing needed fixing, so this task has no commit.

## What the page does now

- `HostHelperOutdatedNotice` sits in the P11 slate box with no button and no dismiss. Heading: "Check for updates needs a newer helper". The explanation is the planned sentence. Below it:
  - one li per outdated piece, in the server's order, with the path in `font-mono break-all`:
    - `script-outdated`: "the installed helper script does not know the order check-update; the new one is deploy/holzkube-manager-host.sh"
    - `check-unit`: "the unit the check runs, from deploy/holzkube-manager-update-check.service"
  - the install commands in the same `overflow-x-auto` pre
  - the helper notice's closing line
- `HostHelperNotice` now says "knows exactly five orders" and "the five buttons above stay off". Both notices render through one frame, `HelperInstallNotice`.
- `host.tsx` shows the older-helper notice last in the stack, only when the host is not in a container, `missing` is empty and `outdated` is not. The two notices are never shown together.

## Red runs and injections (exit codes read from the command itself)

- **RED** against 13-14's tree, with the new rows only:
  - `npx vitest run --project jsdom src/components/HostActions.test.tsx src/routes/host.test.tsx src/fixtures.test.ts`: **exit 1**, 9 failed.
    - The fixture row failed at `fixtures.test.ts:197`, where host-helper-installed.json has no `outdated`. demo.json already had it.
    - The 4 helper-notice rows failed on "five orders".
    - The 4 older-helper rows failed on "no older-helper notice".
  - `npm --prefix web run test:browser -- src/routes/host.browser.test.tsx`: **exit 1**, 1 failed ("no older-helper notice").
- **GREEN:**
  - jsdom: exit 0, 283 passed.
  - browser: exit 0, 10 passed.
  - `go test ./internal/host/hostaction -run TestTheFixtureShowsTheRealInstallCommands`: exit 0.
  - `npm --prefix web run lint`: exit 0, after `biome format` of the touched files. The first lint run was exit 1 on formatting only. The 2 warnings and 1 info that remain are in DataTable.tsx and wall.test.tsx and were there before.
  - `npm --prefix web run typecheck`: exit 0.
- **Injections**, each checked with `cmp` to confirm it applied, then restored and `cmp`-checked back:
  - (a) Older-helper notice rendered while `missing` is not empty (the `missing.length === 0` condition dropped): host.test.tsx **exit 1**, "never beside the helper notice" failed.
  - (b) `overflow-x-auto` removed from the shared pre: browser **exit 1**, 3 failed (both helper-notice fit rows and the older-helper fit row).
  - (c) `"outdated": []` removed from host-helper-installed.json: fixtures.test.ts **exit 1**.

## Documentation and picture

- **README.md, "What it does":** "check for an update without installing anything, or check and install it". The Host paragraph now says five host actions, the first of which only looks. The alpha statement is unchanged.
- **docs/guide.md, "Host actions":** the section now says:
  - Five buttons, with **Check for updates** first. It runs `holzkube-manager-update --check` through `holzkube-manager-update-check.service`, which the helper starts. It installs nothing and keeps the connection, and the status box and Update check name the newest and the installed version. A failed check points to `journalctl -u holzkube-manager-update-check`.
  - The install button now points to that check for looking only.
  - Every one of the five asks for the hostname, the check included.
  - The audit names include `host.check-update`.
  - The helper has exactly five orders and three units.
  - A new paragraph covers an older helper.
  - "No answer" comes after 3 minutes for a check, and the bullet names the check's journal.
- **deploy/HOST-HELPER.md, "Updating":** the Host page names an outdated helper, and the four older orders keep working meanwhile. Repeating the install commands from the newer archive adds the check. This adds no AF_INET/AF_UNIX line and no ProtectSystem value other than strict.
- **docs/api-contract.md:** the Host actions section already said five. Every remaining "four" there means the four other or older actions, four fields or four command lines, so each is correct. I added one sentence on how the page shows a non-empty `outdated`.
- **Doc checks:**
  - `go test ./internal/publicrepo/ -count=1`: exit 0.
  - `go test ./internal/host/hostaction -run 'TestGuideKeepsTheDaemonsHardening|TestInstallCommandsMatchTheGuide' -v`: exit 0, both PASS, no SKIP.
  - The plan's grep line: exit 0.
  - `go test ./internal/config/` (the README/guide option table): exit 0.
- **Render:**
  - Service state before the render: `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST`, `NRestarts=0`.
  - Commands: `./bin/task build:web` (exit 0), `go build -ldflags "-s -w -X main.version=v0.1.0"` (`--version` gives v0.1.0), `node web/scripts/readme-images.mjs` (exit 0).
  - The renderer rewrote 13 files. I compared each with HEAD using PIL `ImageChops.difference`. 12 were pixel-identical (bbox None, 0 pixels) and were restored with `git checkout --`: banner, social, app-detail, apps, clusters, kubernetes, node-hardware, phone, wall, and the three web/public icons.
  - **host.png differs.** It went from 1440x1767 to 1440x1827, and only it was committed.
  - What host.png shows:
    - the header with five buttons (Check for updates first, all off)
    - the reason line "Host actions need the holzkube-manager-host helper, which is not installed. The note below says what to install."
    - the helper notice saying five orders
    - the four install lines, the second now naming the check unit and scrolling inside its box
  - Why it grew 60 px: at 1440 the five buttons no longer fit beside the title, so the actions row now sits below the header text instead of beside it.
  - Every device value in it comes from demo.json; `manager-01.homelab.example` and `6.18.50+rpt-rpi-2712` are both in the fixture.
  - Afterwards `./bin/task build:go`: exit 0.

## D-08 after 13-12

- `LAYOUT_DUMP=$HOME/.cache/holzkube-manager-layout/13-15-after.jsonl ./bin/task test:layout`: **exit 0**. It reported "Nothing out of reach at 390px and 1280px … on 27 routes and in 9 opened states".
- Before-dump: **964 controls**, 36 on /host. After-dump: **966 controls**, 38 on /host.
- `layout-dump-compare.mjs 13-12-before.jsonl 13-15-after.jsonl > 13-15-compare.txt`: **exit 1**, which was expected. It reports "compared 968 controls, 19 differ".

What I read in the compare file:

- **(i) Every difference is in a /host scope.** The 19 lines are in `/host · (route)` and `/host · Host actions (helper installed)`. None are in `Host action dialog`, and none are on any other route. The re-key script confirms that /host is the only route whose control count changed (36 → 38).
- **(ii) No control is present only before.** The compare file does have two `ONLY BEFORE` lines: `/host · (route) · #25 · <svg>` and `#26 · <summary>`. They are an artifact of the key, not lost controls. The key includes the tag, so when the new button shifts every later index by one, positions #25 and #26 change tag (svg→button, summary→svg), and the old key has no match. The two `ONLY AFTER` lines #25 `<button> "24 h"` and #26 `<svg>` are the same controls at the same boxes under their new index. Re-keyed by opener, tag and name, the ONLY BEFORE list is **empty**.
- **(iii) Exactly one control present only after, per scope that renders the actions group.** Re-keyed, the list is `('(route)', button, 'Check for updates')` and `('Host actions (helper installed)', button, 'Check for updates')`. Each is 152x28 at 248,232. Nothing else.
- **(iv) Counts.** The after-dump has 966 = 964 + 2. The compare script's 968 is the union of keys. It counts the two index-shift pairs above twice, so 964 + 4 only-after keys = 968. With the re-keying the union is 966, which equals before plus the two new buttons.
- **(v) Sizes.** Re-keyed by (opener, tag, name, occurrence), **0 controls changed width or height**. 8 moved at the same size: in both actions scopes, "Check for updates and install", "Restart service", "Restart host" and "Shut down host" each moved +160 px in x at the same y=232. That is the new 152 px button plus the 8 px gap. The 16 px gap between the service pair and the host pair is kept (601→617 before, 761→777 after). The other `MOVED` lines in the compare file (#21–#24 Live/1 h/6 h/24 h, #27) are the index shift only. Their boxes are unchanged under the re-key (Live stays at 248,1202 46x28, and so on).
- **The plan's first automated verify line exits 1.** That line uses `! grep -q '^  ONLY BEFORE '`, and it fails only because of the artifact in (ii). The substantive check is that every difference lies in /host (`grep -E '^  (MOVED|ONLY AFTER|ONLY BEFORE) ' | grep -v ' /host · '` is empty, exit 0) and that the re-keyed ONLY BEFORE list is empty. Both hold. It is recorded as a deviation below.

## The gate

- holzkube-manager.service before the gate (and at the start of Task 1): `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST`, `NRestarts=0`.
- `./bin/task ci`: **exit 0**, read from the command itself (`./bin/task ci > log; echo $?`). This was the first run, so no flake rerun was needed.
  - The stages were lint:web, test:web, build:cli, build:go, lint:go (0 issues), test, test:layout and test:next.
  - Vitest: 61 files, 786 tests passed.
  - The log has no FAIL lines.
  - Out of scope, not touched: `npm ci` prints "5 vulnerabilities (2 moderate, 3 high)". The web lock files are unchanged since 8ba3647, so this predates the plan, and it does not fail the gate.
- Fresh builds into the session scratchpad:
  - `GOOS=linux GOARCH=arm64 go build`: exit 0. `file -b` reports "ELF 64-bit LSB executable, ARM aarch64, version 1 (SYSV), dynamically linked, interpreter /lib/ld-linux-aarch64.so.1, …".
  - `GOOS=linux GOARCH=amd64 GOTOOLCHAIN=go1.26.7 go build`: exit 0. `file -b` reports "ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked, …". It was compiled, not executed.
- holzkube-manager.service after: `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST`, `NRestarts=0`, identical to before.
- The helper is not installed: `/usr/local/sbin/holzkube-manager-host`, `/etc/systemd/system/holzkube-manager-host.path` and `/etc/systemd/system/holzkube-manager-update-check.service` are all absent (exit 0).
- `git diff --quiet 8ba3647 -- go.mod go.sum web/package.json web/package-lock.json deploy/holzkube-manager-update.sh`: exit 0.
- `git status --porcelain` was empty after Task 2 and after the gate.

## Deviations from Plan

**1. [Rule 3 - Blocking] biome formatting.** The first `npm --prefix web run lint` of Task 1 exited 1 on the formatting of HostActions.tsx and host.tsx. I ran `biome format --write` on the five touched files, and lint then exited 0. This happened before the commit.

**2. [Plan verify line inaccurate, recorded] `ONLY BEFORE` lines in the D-08 compare.** The plan's first automated check assumes the compare output has no `ONLY BEFORE` line at all. The compare key includes the tag, so the fifth button's index shift across a tag boundary on /host produces two ONLY BEFORE/ONLY AFTER pairs (#25, #26) for controls whose boxes did not change. The re-key by opener, tag and name in (ii)–(v) shows that no control disappeared and none changed size, and every difference is on /host. I fixed nothing, because there was nothing wrong in the product.

**3. [Noted] api-contract.md needed no "four" → "five" replacement.** Every remaining "four" in its Host actions section is correct as it stands (see above). The only addition is one sentence on how the page shows `outdated`.

**4. [Noted] demo.json already carried `"outdated": []`,** added in 13-14. Only host-helper-installed.json needed the key. The new fixtures test covers both.

## Known Stubs

None.

## HACT-04

Complete. The check that installs nothing is on /host and cannot be sent to an older helper, which is told what to reinstall. It is documented in the README, the guide, HOST-HELPER.md and the contract, and pictured in host.png. The desk was measured against the state before 13-12, and the whole gate is green on the Pi with the production host unchanged. ROADMAP criterion 1 already lists the five actions (48db458).

## human_needed (not this executor's to do)

- The D-11 phone hand test with five actions. It needs a phone, a development daemon with TLS on the LAN and an installed helper (see 14-07).
- Installing the helper on the Pi and probing it, starting with Check for updates. That puts root code on the production host, which is the operator's call (CLAUDE.md).

## Self-Check: PASSED

- The six Task 1 files, the five Task 2 files and this SUMMARY are present.
- Commits `f5ec24d` and `b7ecc83` are in `git log`.
- `13-15-after.jsonl` (966 lines) and `13-15-compare.txt` (46 lines) are in `$HOME/.cache/holzkube-manager-layout/`, outside the repository.
