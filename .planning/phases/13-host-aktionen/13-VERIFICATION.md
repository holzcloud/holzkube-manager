---
phase: 13-host-aktionen
verified: 2026-09-30T04:14:06Z
status: human_needed
score: 5/5 must-haves verified
covered_files:
  - ".github/workflows/ci.yml"
  - ".goreleaser.yaml"
  - ".planning/phases/13-host-aktionen/13-01-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-01-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-02-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-02-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-03-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-03-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-04-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-04-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-05-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-05-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-06-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-06-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-07-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-07-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-08-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-08-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-09-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-09-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-10-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-10-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-11-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-11-SUMMARY.md"
  - "README.md"
  - "cmd/holzkube-managerd/budget_test.go"
  - "cmd/holzkube-managerd/main.go"
  - "deploy/HOST-HELPER.md"
  - "deploy/holzkube-manager-host.path"
  - "deploy/holzkube-manager-host.service"
  - "deploy/holzkube-manager-host.sh"
  - "docs/api-contract.md"
  - "docs/guide.md"
  - "docs/screenshots/host.png"
  - "internal/audit/redact.go"
  - "internal/host/actions_test.go"
  - "internal/host/collector.go"
  - "internal/host/collector_test.go"
  - "internal/host/health.go"
  - "internal/host/health_test.go"
  - "internal/host/host.go"
  - "internal/host/hostaction/helper.go"
  - "internal/host/hostaction/helper_owner_other.go"
  - "internal/host/hostaction/helper_owner_unix.go"
  - "internal/host/hostaction/helper_test.go"
  - "internal/host/hostaction/hostaction.go"
  - "internal/host/hostaction/hostaction_test.go"
  - "internal/host/hostaction/result.go"
  - "internal/host/hostaction/result_test.go"
  - "internal/host/hostaction/script_test.go"
  - "internal/host/hostaction/units_test.go"
  - "internal/host/sensors.go"
  - "internal/host/sensors_test.go"
  - "internal/httpapi/endtoend_test.go"
  - "internal/httpapi/handlers/confirm_test.go"
  - "internal/httpapi/handlers/host.go"
  - "internal/httpapi/handlers/wall_host_test.go"
  - "internal/httpapi/hostactionsapi_test.go"
  - "internal/httpapi/hostgatesapi_test.go"
  - "internal/httpapi/hosthelperapi_test.go"
  - "internal/httpapi/problem.go"
  - "internal/httpapi/router.go"
  - "internal/jobs/confirm.go"
  - "internal/jobs/confirm_test.go"
  - "internal/processguard_test.go"
  - "internal/store/fsstore/atomic.go"
  - "internal/store/fsstore/place_test.go"
  - "web/fixtures/demo.json"
  - "web/scripts/readme-images.mjs"
  - "web/src/api.ts"
  - "web/src/components/HostActions.test.tsx"
  - "web/src/components/HostActions.tsx"
  - "web/src/components/charts/Sensors.test.tsx"
  - "web/src/components/charts/Sensors.tsx"
  - "web/src/fixtures.test.ts"
  - "web/src/routes/host.browser.test.tsx"
  - "web/src/routes/host.test.tsx"
  - "web/src/routes/host.tsx"
  - "web/src/routes/wall.test.tsx"
covered_digest: "v2:sha256:1e68bdfb8e993c6f3bb1b56c81233c2a21558ad86b003109e1344612d0708a54"
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 4/5
  gaps_closed:
    - "SC4: With the helper not installed, the actions are locked, the page names what from deploy/ goes where, and no order is created (G-13-2: Detect no longer lists not-enabled beside path-unit)"
    - "No host action label runs past its button at 390 px (G-13-3, previously deferred to Phase 14, closed in 13-11)"
  gaps_remaining: []
  regressions: []
human_verification:
  - test: "Install the helper per deploy/HOST-HELPER.md on the Pi (operator's call), then run the three-step probe: start holzkube-manager-host.service with no order, check the path unit is 'active (waiting)', then press 'Check for updates and install' on /host."
    expected: "Journal says 'kein Auftrag' and exit 0; path unit active (waiting); the order is picked up once, /var/lib/holzkube-manager-host/last reads '<id> update started <time>', the status box walks placed -> picked up -> started -> update finished, the path unit does not loop."
    why_human: "Needs root-installed units under the real system manager and its sandbox (ProtectSystem=strict, ReadWritePaths into a 0700 dir of another user, NoNewPrivileges). Tests ran the script under unshare with a stand-in systemctl, and 13-09 probed it under the user manager only."
  - test: "With the helper installed, restart the host from /host with two sessions open, only one of which placed the order."
    expected: "Both pages show 'Waiting for holzkube-manager to come back' (not the amber stale notice) while the host is down, then the status reaches 'back'; the order is not run a second time after boot (last shows one started line for that id)."
    why_human: "Only a real reboot can show the connection loss, the PathExists-at-boot behaviour and the 60-s age window against the Pi's restored clock; host.test.tsx holds only the derivation from a reading."
  - test: "With the helper installed, 'Restart service' and 'Shut down host' once each (operator's choice when)."
    expected: "restart-service: the daemon comes back and the page shows back; poweroff: the Pi powers off via the root oneshot under NoNewPrivileges/RestrictAddressFamilies=AF_UNIX."
    why_human: "systemctl reboot/poweroff/restart from inside the helper's sandbox is not exercised by any test; the stand-in systemctl only records argv."
  - test: "Decide whether HACT-04 ('jetzt nach Updates suchen') is met by 'Check for updates and install' (search AND install through holzkube-manager-update.service)."
    expected: "Operator accepts the broader action, or asks for a check-only variant."
    why_human: "13-CONTEXT marks this deviation from the requirement's wording 'Zur Prüfung vorgemerkt'; it is a product decision, not a code fact."
---

# Phase 13: Host-Aktionen über einen root-eigenen Helfer -- Verification Report

**Phase Goal:** Der Betreiber kann den Host neu starten, herunterfahren, den Dienst holzkube-manager neu starten und „jetzt nach Updates suchen" auslösen -- ohne dass der Daemon Root, D-Bus oder eine Capability bekommt. Er legt einen Auftrag im Datenverzeichnis ab; eine root-eigene Path-Unit holt ihn ab und ein festes Skript führt ihn aus einer festen Liste aus.
**Verified:** 2026-09-30T04:14:06Z
**Status:** human_needed (all five success criteria verified; what remains needs an installed helper, a real reboot, or an operator decision)
**Re-verification:** Yes -- after gap closure (13-10 G-13-2, 13-11 G-13-3), previous report 2026-09-29 at a45015c

**Where this ran:** the operator's Raspberry Pi 5 (aarch64), Go 1.27.1 from `~/.local/go`, Node via nvm, the browser project's Chromium. No `-race` (ThreadSanitizer refuses this kernel; the race verdict is CI's). `unshare --user --map-root-user id -u` printed 0, so the root-namespace script tests ran (28 `TestHostScriptAsRoot` subtests PASS, 0 SKIP). Nothing installed: `/usr/local/sbin/holzkube-manager-host` and `/etc/systemd/system/holzkube-manager-host.path` are still absent. The production service and its data directory were not addressed. `git status --porcelain` was empty after every injection and after the run.

## Re-verification scope

The commits since a45015c (`51631f8`, `a1f1679`, `dd670b3`, `c265f42`, plus planning docs) touch `helper.go`, the Detect/fixture/page tests, `demo.json`, `api-contract.md`, `api.ts`, `host.png`, `HostActions.tsx` and `host.browser.test.tsx`. The two gaps were verified in full. SC1, SC2, SC3 and SC5 got a regression check: their suites were re-run green (below), and none of their implementation files changed.

## Goal Achievement

### Observable Truths (ROADMAP success criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Four actions on /host; each needs sudo window + typed hostname, refused below operator, audited; each lock removed singly and its test seen red | VERIFIED (regression) | `handlers/host.go`, `redact.go` unchanged since a45015c; `go test ./internal/httpapi/` rc 0 (includes TestHostActionGates, TestHostConfirmGates, TestHostTokenOpensOneOrderInItsOwnSession). Red runs from the initial verification stand. |
| 2 | Daemon never executes a host action itself; guard seen red against a smuggled process call | VERIFIED (regression) | `go test ./internal/ ./cmd/...` rc 0 (TestTheDaemonStartsNoProcess). `main.go` unchanged. |
| 3 | Helper script runs exactly the four orders against a test dir and stand-in systemctl; unknown/malformed input discarded and logged; test seen red | VERIFIED (regression) | `deploy/holzkube-manager-host.sh` unchanged; `TestHostScriptAsRoot` PASS with 28/28 subtests run as root in a user namespace, 0 SKIP. |
| 4 | Helper not installed -> actions locked, page names what from deploy/ goes where, no order left in the data dir | VERIFIED (gap G-13-2 closed) | `helper.go:96-115`: the wants entry is only checked when `unitsInstalled`, so an empty machine gives `[script, path-unit]`. `TestHelperEveryCombination` walks all 16 presence states and holds that path-unit and not-enabled never appear together, that not-enabled appears exactly when both units are present and wants is absent, and that the list is empty exactly when everything is installed. The routes' 409 for a missing helper is unchanged: `TestHostActionsNeedTheHelper` has new rows for "nothing installed" and "script installed, no unit files" and passes. `demo.json`, `fixtures.test.ts`, `api-contract.md:1865-1872` and the example at `:1888-1894` show the two-item list. The contract now defines not-enabled as "both unit files are there". **host.png viewed by this verifier:** the notice lists only the script path and the .path unit path, and does not contain "installed but not enabled". The sentence "is installed but not enabled" (`HostActions.tsx:784`) is now only reachable when it is true. **Reinstated by this verifier:** `if unitsInstalled {` -> `if unitsInstalled \|\| !unitsInstalled {`, which compiles, gave rc 1 across TestHelperEveryCombination (e.g. "script=no .path=no .service=no wants=no ... lists path-unit and not-enabled together"), TestHelperBoxMissing, TestHelper (3 rows), TestTheFixtureShowsTheRealInstallCommands, TestReadCarriesActions/nothing_installed and TestHostActionsNeedTheHelper (2 rows). Restored with `git checkout`; tree clean. |
| 5 | deploy/ has path unit, service unit, script, install guide; `systemd-analyze verify` accepts them; guide asks for no less hardening | VERIFIED (regression) | deploy/ unchanged since a45015c; TestUnitsVerify, TestUnitsAgree, TestGuideKeepsTheDaemonsHardening, TestInstallCommandsMatchTheGuide in the `hostaction` package run: rc 0. |

**Score:** 5/5 truths verified (0 present-but-behavior-unverified)

### Plan-level and UAT truths

| Truth | Status | Evidence |
|---|---|---|
| No host action label runs past its button at 390 px (UAT G-13-3, previously deferred to Phase 14) | VERIFIED (closed in 13-11) | Root cause confirmed in code: the old template put `max-md:whitespace-normal${` right before the interpolation, so Tailwind's scanner never saw the class as a token. It is now `cn('... max-md:whitespace-normal', spec.destructive && 'text-destructive')` (`HostActions.tsx:251-254`). **Built CSS:** `./bin/task build:web --force` rc 0, and `index-*.css` has exactly one `.max-md\:whitespace-normal{white-space:normal}`, inside `@media not all and (width>=48rem)`. HostActions.tsx is the only source file that uses the class. **Browser:** `host.browser.test.tsx` 8/8 PASS. The tests include per-button `spillsPastItsButton`, `lineBoxes(update) == 2` at 390 px, both helper-notice shapes with disabled buttons, and the 1200-px desktop shape (28 px tall, one line each). **Reinstated by this verifier:** the original template literal put back gave rc 1, 3 failed / 5 passed, with `"Check for updates and install" scrolls: scrollWidth 191 > clientWidth 189`, the icon at -0.5..15.5 and the text line at 19.5..191.5 outside 0..191. This matches 13-11's claimed red run exactly. Restored; tree clean. |
| 40-character hostname wraps inside the dialog at 390 px | VERIFIED | browser test PASS; the dialog footer buttons now also pass `spillsPastItsButton` |
| A /host page that did not place the reboot order shows the waiting notice | VERIFIED (derivation) + human | `host.test.tsx` PASS; the real reboot is a human item |
| Nothing installed on the operator's host, production never restarted; helper ships in deploy/ only | VERIFIED | Helper files absent from the host; this verifier did not touch the service |

### Required Artifacts (changed since a45015c)

| Artifact | Expected | Status | Details |
|---|---|---|---|
| `internal/host/hostaction/helper.go` | Detect: not-enabled only with both units | VERIFIED | its doc comment and code agree; red against the reinstated fault |
| `internal/host/hostaction/helper_test.go` | 16-combination invariant plus example rows | VERIFIED | goes red on the fault |
| `web/fixtures/demo.json`, `docs/api-contract.md`, `docs/screenshots/host.png` | two-item empty-machine list | VERIFIED | Go and web fixture guards both pin the list; png inspected |
| `web/src/components/HostActions.tsx` | class list through `cn()` | VERIFIED | rule is in the built CSS |
| `web/src/routes/host.browser.test.tsx` | per-button fit check and desktop shape | VERIFIED | red against the reinstated template |

### Key Link Verification

| From | To | Via | Status |
|---|---|---|---|
| `Detect` | page notice + route 409 | `Box.Missing()` -> `actions.missing` / `available` | WIRED (TestReadCarriesActions, TestHostActionsNeedTheHelper) |
| `demo.json` | `host.png` / README | `web/scripts/readme-images.mjs` | WIRED (png shows the fixture's two items) |
| `HostActions.tsx` class | built stylesheet | Tailwind source scan -> `dist/assets/index-*.css` | WIRED (rule present, one source) |

Other links are unchanged from the initial report and still pass: buttons -> confirm/actions routes -> `Box.Place` -> order file -> PathExists -> script -> `last` -> `ReadResult`.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|---|---|---|---|
| Host, hostaction, httpapi, fsstore, jobs, publicrepo | `go test ./internal/host/... ./internal/httpapi/ ./internal/store/fsstore/ ./internal/jobs/ ./internal/publicrepo/ -count=1` | rc 0, 7 packages ok | PASS |
| Named G-13-2 tests + root script | `go test -v -run 'TestHelper\|TestTheFixtureShowsTheRealInstallCommands\|TestReadCarriesActions\|TestHostActionsNeedTheHelper\|TestHostScriptAsRoot'` | rc 0, 64 PASS, 0 SKIP, 0 FAIL | PASS |
| Process guard + cmd guards | `go test ./internal/ ./cmd/... -count=1` | rc 0 | PASS |
| Web jsdom | `vitest run --project jsdom HostActions.test.tsx host.test.tsx fixtures.test.ts` | rc 0, 221/221 | PASS |
| Web browser | `npm run test:browser -- src/routes/host.browser.test.tsx` | rc 0, 8/8 | PASS |
| Built CSS carries the wrap rule | `./bin/task build:web --force`; grep the css | rc 0; 1 rule inside the max-md media query | PASS |
| Web typecheck / lint | `npm run typecheck`, `npm run lint` | rc 0 / rc 0 (2 warnings, 1 info, pre-existing per 13-10) | PASS |

### Fault injections by this verifier (each restored; `git status --porcelain` empty after)

| Injection | Test | rc |
|---|---|---|
| `helper.go`: wants check made unconditional (`if unitsInstalled \|\| !unitsInstalled`) | TestHelper*, TestTheFixtureShowsTheRealInstallCommands, TestReadCarriesActions, TestHostActionsNeedTheHelper | 1 |
| `HostActions.tsx`: original `...whitespace-normal${...}` template put back | host.browser.test.tsx | 1 (3 failed) |

### Requirements Coverage

| Requirement | Description | Status | Evidence |
|---|---|---|---|
| HACT-01 | Restart host | SATISFIED (automated) / real run human | unchanged |
| HACT-02 | Shut down host | SATISFIED (automated) / real run human | unchanged |
| HACT-03 | Restart service | SATISFIED (automated) / real run human | unchanged |
| HACT-04 | "jetzt nach Updates suchen" | SATISFIED as "check and install" -- decision flagged | unchanged; label now fits at 390 px |
| HACT-05 | sudo + typed hostname + audit + operator | SATISFIED | SC1 |
| HACT-06 | daemon never executes; fixed list; unknown discarded and logged | SATISFIED | SC2, SC3 |
| HACT-07 | helper missing -> UI says so and names what to install, no order | SATISFIED | SC4, G-13-2 closed |
| HACT-08 | deploy/ units, script, guide | SATISFIED | SC5 |

No orphaned requirements.

### Anti-Patterns Found

The files changed since a45015c contain no `TBD`, `FIXME`, `XXX`, `TODO` or `HACK`. Both warnings from the previous report (helper.go:102-104 and HostActions.tsx:777) are resolved.

| File | Line | Pattern | Severity | Impact |
|---|---|---|---|---|
| .github/workflows/ci.yml | 121-122 | userns sysctl step never executed (no CI minutes) | Info | the root matrix is proven on the Pi only |

### Human Verification Required

1. **Install and probe the helper.** Follow HOST-HELPER.md, run the three-step probe, then press "Check for updates and install". Expected: `kein Auftrag` on the dry start, the path unit `active (waiting)`, one pickup, `last` reads `<id> update started`, and no loop. Why human: it needs the real system manager and sandbox, and installing is the operator's call.
2. **Real reboot with two sessions.** Expected: both pages show the waiting notice, then "back", and the order does not run again after boot. Why human: it needs a real reboot and the Pi's clock at boot.
3. **Restart service and shut down, once each.** Expected: both work from inside the helper's sandbox. Why human: the stand-in systemctl only records argv.
4. **HACT-04 wording.** Either accept "check for updates and install" as meeting "nach Updates suchen", or ask for a check-only variant. Why human: this is a product decision flagged in 13-CONTEXT.

### Gaps Summary

Both gaps are closed in the code, not only in the summaries. On an empty machine, `Detect` now reports `[script, path-unit]`. That state is pinned by a 16-state invariant, by the Go and web fixture guards, by the contract and by the re-rendered host.png. Putting the old unconditional check back turns five test functions red. The update button's wrap class now reaches the built stylesheet. Chromium measures every host button against its own border box at 390 px and checks the desktop shape at 1200 px. Putting the old template back reproduces the 191/189 overflow. No regressions were found in SC1-3 or SC5. What remains cannot be checked here: an installed helper under the real system manager, a real reboot, restart-service and poweroff from the sandbox, and the HACT-04 wording decision.

---

_Verified: 2026-09-30T04:14:06Z_
_Verifier: Claude (gsd-verifier)_
