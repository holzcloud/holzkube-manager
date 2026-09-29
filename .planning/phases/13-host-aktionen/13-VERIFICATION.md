---
phase: 13-host-aktionen
verified: 2026-09-29T19:37:10Z
status: gaps_found
score: 4/5 must-haves verified
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
  - "README.md"
  - "cmd/holzkube-managerd/budget_test.go"
  - "cmd/holzkube-managerd/main.go"
  - "deploy/HOST-HELPER.md"
  - "deploy/holzkube-manager-host.path"
  - "deploy/holzkube-manager-host.service"
  - "deploy/holzkube-manager-host.sh"
  - "docs/api-contract.md"
  - "docs/guide.md"
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
covered_digest: "v2:sha256:046803a731b409afb95cb776692277a33b5b7a3521a4190ea9e3f89309d862eb"
behavior_unverified: 0
overrides_applied: 0
gaps:
  - truth: "SC4: With the helper not installed, the actions are locked, the page names what from deploy/ goes where, and no order is created"
    status: partial
    reason: >-
      Locked and no-order hold (tests pass; seen red by this verifier). But in exactly the
      state SC4 names -- nothing installed, the Pi's state -- the helper notice states a
      falsehood: hostaction.Detect appends MissingNotEnabled whenever the
      paths.target.wants entry is absent, also when the unit files themselves are missing
      (helper.go:102-104, contradicting its own doc at :55-57), and HostActions.tsx:777
      renders that item as "holzkube-manager-host.path is installed but not enabled",
      directly under the line saying the path unit is missing. The API contract
      (api-contract.md:1867, "units are there but the path unit is not enabled"), the demo
      fixture and README's host.png carry the same contradiction. Independently confirmed
      by code reading; also reported by 13-UAT.md as G-13-2.
    artifacts:
      - path: "internal/host/hostaction/helper.go"
        issue: "not-enabled reported alongside path-unit when no unit file exists"
      - path: "web/src/components/HostActions.tsx"
        issue: "line 777 asserts the path unit is installed"
      - path: "docs/api-contract.md"
        issue: "line 1867 defines not-enabled as 'units are there', which Detect does not honour"
      - path: "web/fixtures/demo.json"
        issue: "fixture shows all three items, so docs/screenshots/host.png shows the false sentence"
    missing:
      - "Report not-enabled only when both unit files are present (or word the item so it is true in both states), with a Detect/TestHelper row seen red against the current behaviour"
      - "Fixture, TestTheFixtureShowsTheRealInstallCommands expectation, contract text and host.png follow the corrected list"
deferred:
  - truth: "No host action label runs past its button at 390 px (13-UAT G-13-3)"
    addressed_in: "Phase 14"
    evidence: "Phase 14 SC4: '/host mit seinen Aktionen und dem Bestätigungsdialog besteht die Prüfung bei 390 px, und auf einem Telefon lässt sich der Host ansehen und eine Aktion bis zur getippten Bestätigung führen'. Confirmed here: a temporary assertion in host.browser.test.tsx measured 'Check for updates and install' at scrollWidth 191 / clientWidth 189 in the 2 x 2 grid (Chromium, 390 px); reverted."
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
**Verified:** 2026-09-29T19:37:10Z
**Status:** gaps_found (one partial gap; plus human items that only an installed helper and a real reboot can show)
**Re-verification:** No -- initial verification

**Where this ran:** the operator's Raspberry Pi 5 (aarch64), Go 1.26.7 via GOTOOLCHAIN, Node v22 via nvm, Playwright Chromium, systemd 257. No `-race` (ThreadSanitizer refuses this kernel). `unshare --user --map-root-user id -u` printed 0, so every root-namespace test ran; 0 SKIP lines in any run below. Nothing installed; the production service, its data directory and `/etc/systemd` were not touched (only read: `systemctl cat`/`list-unit-files`). `/usr/local/sbin/holzkube-manager-host` and the path unit do not exist on this host. During verification a concurrent session committed `3e08d11` (13-UAT.md); its two issues were checked independently below, not taken on trust.

## Goal Achievement

### Observable Truths (ROADMAP success criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Four actions on /host; each needs sudo window + typed hostname, refused below operator, audited; each of the four locks removed singly and its test seen red | VERIFIED | Routes in `handlers/host.go` `HostRoutes`: `RequiresSession`, `MinRole: RoleOperator`, `Destructive: true`, `Action: host.<a>`; confirm route compares `TrimSpace(typed)` with uname hostname; `hostTypedPhrase` all four true; token intent `{host.<a>, "@host", session digest}` checked by `CheckOnce`. Audit allowlist `redact.go:238-241`. `TestHostActionGates` (8 subtests, each looping all four actions), `TestHostConfirmGates` (loops all four), `TestHostTokenOpensOneOrderInItsOwnSession`: PASS. 13-03 SUMMARY records F1 sudo / F2 typed / F3 role / F4 audit each red. **Spot-checked by this verifier:** typed-hostname comparison disabled -> `TestHostConfirmGates` rc 1 (5 subtests red, "the answer carries a token"); `Action: ""` on the action routes -> `TestHostActionGates/every_action_is_audited` rc 1. Four buttons: `HostActions.tsx` `HOST_ACTIONS`, jsdom + browser tests pass. |
| 2 | Daemon never executes a host action itself; guard seen red against a smuggled process call | VERIFIED | `main.go:575-583` wires only `fsstore.PlaceNew`/`Claim` into the Box; `internal/processguard_test.go` (go-list half + alias-aware AST half + linkname/cgo/raw-syscall checks): PASS. **Spot-checked:** added `internal/host/smuggle.go` with `exec.Command("systemctl","reboot")` -> build ok, `TestTheDaemonStartsNoProcess` rc 1, both halves red (`smuggle.go:3: imports "os/exec"`); file removed. 13-05 records F7/F8 and 13-REVIEW-FIX IN-01 more. |
| 3 | Helper script against a test data dir and stand-in systemctl runs exactly the four orders; unknown/malformed (foreign word, path, shell metachar) discarded and logged; test seen red against a script that runs it | VERIFIED | `deploy/holzkube-manager-host.sh`: claim by rename, `dd iflag=nofollow,nonblock` 65 B, anchored ERE under `LC_ALL=C`, exact-size check, fixed argv per action, content never logged. `TestHostScriptAsRoot` (28 subtests incl. foreign word, path, `;`, `$()`, backticks, NUL, CRLF, symlink, FIFO, stale, future) and `TestHostScriptClaimRace` (8): PASS, run as root in a user namespace. **Spot-checked:** pattern relaxed to any two fields and `*)` branch made to run `$action` -> `TestHostScriptAsRoot` rc 1 (`rejects_foreign_word`: "exit = 0, want 2", "systemctl was called, want never"); restored, `git status` clean. 13-04 records F9-F13. |
| 4 | Helper not installed -> actions locked, page names what from deploy/ goes where, no order left in the data dir | PARTIAL (gap) | Locked + no order: both routes check `InContainer()` then `Missing()` before body/token/Place; `TestHostActionsNeedTheHelper` / `TestHostActionsInAContainer`: PASS. **Spot-checked:** helper check removed from the action route -> rc 1, "202, want 409 ... a refused request placed an order". Page: `disabledReason` puts the helper reason first after container; `HostHelperNotice` lists items and `install_commands`. **But** in the nothing-installed state the notice says "holzkube-manager-host.path is installed but not enabled" right after "path unit missing" (`helper.go:102-104` + `HostActions.tsx:777`); contract, fixture and host.png repeat it. See Gaps. |
| 5 | deploy/ has path unit, service unit, script, install guide; `systemd-analyze verify` accepts the units; guide asks for no less hardening on the daemon unit | VERIFIED | All four files present and in `.goreleaser.yaml:85-88`. **Run by this verifier:** `systemd-analyze verify --man=no` on copies with ExecStart pointed at `/bin/true`: rc 0, no output (on the shipped files the only complaint is that `/usr/local/sbin/holzkube-manager-host` does not exist, i.e. not installed). `TestUnitsVerify` (output-gated, negative control), `TestUnitsAgree`, `TestGuideKeepsTheDaemonsHardening`, `TestInstallCommandsMatchTheGuide`, `TestTheArchiveCarriesTheHelper`, `TestTheUpdateScriptDoesNotShipTheHelper`: PASS. HOST-HELPER.md "The service's own unit stays as it is" names NoNewPrivileges, empty CapabilityBoundingSet, AF_INET AF_INET6, ProcSubset=pid and four more as unchanged; the production unit (read only) still carries all of them. |

**Score:** 4/5 truths verified (0 present-but-behavior-unverified)

### Plan-level truths (cross-cutting, from ROADMAP)

| Truth | Status | Evidence |
|---|---|---|
| 40-character hostname wraps inside the dialog at 390 px | VERIFIED | `host.browser.test.tsx` "wraps a 40-character hostname inside the Restart host dialog": PASS (Chromium, 6/6) |
| A /host page that did not place the reboot order shows the waiting notice | VERIFIED (derivation) + human | `host.test.tsx` "says waiting on a page that did not place the order" PASS; real two-session reboot is a human item |
| Nothing installed on the operator's host, production never restarted; helper ships in deploy/ only | VERIFIED | helper files absent from `/usr/local/sbin` and `/etc/systemd/system`; update script never names the helper (test); archive carries it |

### Deferred Items

| # | Item | Addressed In | Evidence |
|---|------|-------------|----------|
| 1 | "Check for updates and install" label overflows its button at 390 px (scrollWidth 191 / clientWidth 189 in the browser harness; UAT saw 180/173 in the app) | Phase 14 | SC4: "/host mit seinen Aktionen ... besteht die Prüfung bei 390 px" |

### Required Artifacts

| Artifact | Expected | Status | Details |
|---|---|---|---|
| `deploy/holzkube-manager-host.sh` | root helper, four fixed orders | VERIFIED | substantive (claim, validate, record, act); exercised by 36 root-namespace subtests |
| `deploy/holzkube-manager-host.path` / `.service` | PathExists on host-order; hardened oneshot | VERIFIED | verify clean; no [Install] on service, no RemainAfterExit |
| `deploy/HOST-HELPER.md` | install, probe, hardening unchanged, drop-in, recovery, uninstall | VERIFIED | install block == `hostaction.InstallCommands` (test) |
| `internal/host/hostaction/*` | Box, Place/Claim, pickup timer, sweep, Close withdrawal, Detect, ReadResult | VERIFIED (Detect wording gap above) | wired in `main.go`, `host.Collector`, handlers |
| `internal/httpapi/handlers/host.go` | confirm + four action routes | VERIFIED | registered via `handlers.HostRoutes(deps)` in `main.go:904` |
| `internal/processguard_test.go` | no process start in the daemon | VERIFIED | red against injection |
| `web/src/components/HostActions.tsx`, `routes/host.tsx` | four buttons, dialog, reason, status, waiting, helper notice | VERIFIED except line 777 | 219 jsdom tests + 6 browser tests pass |

### Key Link Verification

| From | To | Via | Status |
|---|---|---|---|
| `/host` buttons | `POST /api/v1/host/confirm` then `/actions/<a>` | `api.hostActions`, `HOST_ACTION_PATHS` | WIRED (cmd route-reachability guards pass) |
| action route | order file | `Box.Place` -> `fsstore.PlaceNew` (link, EEXIST -> 409) | WIRED (`TestHostActionRoundTrip` PASS, root helper consumed the real order) |
| order file | root script | `PathExists=/var/lib/holzkube-manager/host-order` | WIRED on paper (units agree with script default and Go constant); real firing = human item |
| script | page | `/var/lib/holzkube-manager-host/last` -> `ReadResult` -> `actions.result` | WIRED (round trip reads the script's own line) |
| helper detection | routes + page | `Box.Missing()` used by both routes and `actions.available` | WIRED |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|---|---|---|---|
| Host/hostaction/fsstore/jobs suites | `go test ./internal/host/... ./internal/store/fsstore/ ./internal/jobs/ -count=1 -v` | rc 0, 423 PASS, 0 SKIP | PASS |
| Host API gates, helper, container, round trip, contract codes | `go test ./internal/httpapi/ -run '...'` (8 tests) + `TestHostTokenOpensOneOrderInItsOwnSession` | rc 0, 0 SKIP | PASS |
| Process guard | `go test ./internal/ -run 'TestTheDaemonStartsNoProcess\|TestProcessGuardRecognisesAProcessStart'` | rc 0 | PASS |
| cmd guards (budget, allowlist, reachability) | `go test ./cmd/holzkube-managerd/ -count=1 -v` | rc 0, 140 PASS | PASS |
| Full Go suite, once | `go test ./internal/... ./cmd/... -count=1` | rc 0, 41 packages ok (incl. publicrepo) | PASS |
| Web jsdom | `vitest run --project jsdom HostActions.test.tsx host.test.tsx fixtures.test.ts` | rc 0, 219/219 | PASS |
| Web browser | `npm run test:browser -- src/routes/host.browser.test.tsx` | rc 0, 6/6 | PASS |
| Units | `systemd-analyze verify --man=no` (ExecStart stubbed) | rc 0, empty output | PASS |

### Fault injections re-run by this verifier (each restored; `git status --porcelain` empty after every one)

| Injection | Test | rc |
|---|---|---|
| `internal/host/smuggle.go` with `exec.Command("systemctl","reboot")` | TestTheDaemonStartsNoProcess | 1 |
| script accepts any two-field line and runs `$action` | TestHostScriptAsRoot | 1 |
| typed-hostname comparison `if false && ...` | TestHostConfirmGates | 1 |
| helper check off in the action route | TestHostActionsNeedTheHelper | 1 |
| `Action: ""` on the four action routes | TestHostActionGates/every_action_is_audited | 1 |
| (measurement, not a fix) scrollWidth/clientWidth of the four buttons at 390 px | host.browser.test.tsx | 1 -- confirms deferred G-13-3 |

### Requirements Coverage

| Requirement | Description | Status | Evidence |
|---|---|---|---|
| HACT-01 | Restart host | SATISFIED (automated) / real run human | route, script `systemctl reboot`, button |
| HACT-02 | Shut down host | SATISFIED (automated) / real run human | route, script `systemctl poweroff`, button |
| HACT-03 | Restart service | SATISFIED (automated) / real run human | route, script `systemctl restart holzkube-manager.service` |
| HACT-04 | "jetzt nach Updates suchen" | SATISFIED as "check and install" -- decision flagged | `systemctl start --no-block holzkube-manager-update.service`; unit exists on the Pi |
| HACT-05 | sudo + typed hostname + audit + operator | SATISFIED | SC1 |
| HACT-06 | daemon never executes; fixed list; unknown discarded and logged | SATISFIED | SC2, SC3 |
| HACT-07 | helper missing -> UI says so and names what to install, no order | PARTIAL | SC4 gap (false "installed but not enabled") |
| HACT-08 | deploy/ units, script, guide | SATISFIED | SC5 |

No orphaned requirements: REQUIREMENTS.md maps exactly HACT-01..08 to Phase 13, all claimed by plans.

### Anti-Patterns Found

No `TBD`/`FIXME`/`XXX`/`TODO`/`HACK` in the 56 non-planning files changed since `ef4648a`.

| File | Line | Pattern | Severity | Impact |
|---|---|---|---|---|
| internal/host/hostaction/helper.go | 102-104 | item reported contrary to its own documented meaning | Warning | false sentence on the page in the production state (gap) |
| web/src/components/HostActions.tsx | 777 | copy asserts a fact the data does not carry | Warning | same gap |
| .github/workflows/ci.yml | 121-122 | userns sysctl step never executed (no CI minutes) | Info | root matrix is proven on the Pi only; CI run unperformed |

### Human Verification Required

1. **Install and probe the helper** -- follow HOST-HELPER.md, run the three-step probe, press "Check for updates and install". Expected: `kein Auftrag` on the dry start, path unit `active (waiting)`, one pickup, `last` = `<id> update started`, no loop. Why human: real system manager and sandbox; installing is the operator's call.
2. **Real reboot, two sessions** -- expected waiting notice on both pages, then "back", order not re-run after boot. Why human: needs a real reboot and the Pi's clock at boot.
3. **Restart service and shut down once each** -- expected both work from inside the helper's sandbox. Why human: the stand-in systemctl only records argv.
4. **HACT-04 wording** -- accept "check for updates and install" as meeting "nach Updates suchen", or ask for check-only. Why human: product decision flagged in 13-CONTEXT.

### Gaps Summary

The phase goal is structurally achieved: the daemon places one validated line through fsstore and nothing else, the guard is red against smuggled process starts, the root script carries out exactly four fixed commands and was red against a script that runs anything else, the four locks and the host/session-bound single-use token are each held by tests seen red, and deploy/ ships verified units and a guide that loosens nothing.

One gap remains, and it sits in the state the Pi is actually in: with nothing installed, the helper notice (and the contract, fixture and README picture) says the path unit "is installed but not enabled" one line after saying it is missing. `Detect` should report `not-enabled` only when both unit files are present. The 390-px label overflow is real but belongs to Phase 14's /host layout criterion. Everything a real reboot or a root-installed helper would show is left to the operator.

---

_Verified: 2026-09-29T19:37:10Z_
_Verifier: Claude (gsd-verifier)_
