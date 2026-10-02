---
gsd_state_version: "1.0"
milestone: v1.18
milestone_name: Host & Telefon
current_phase: 14
current_phase_name: telefon-tippziele
status: verifying
stopped_at: Completed 13-17-PLAN.md
last_updated: "2026-10-02T04:57:09.209Z"
last_activity: 2026-09-30
last_activity_desc: Phase 14 execution started
state_head: 735a7680cf981bb719251d03aa937621c4bac26b
progress:
  total_phases: 4
  completed_phases: 2
  total_plans: 39
  completed_plans: 39
  percent: 50
---

# Project State

Stand 2026-09-28. Der vorige Bericht (Abschluss v1.15, Übergabe 2026-09-17)
liegt unverändert in `milestones/STATE-2026-09-17.md`.

## Project Reference

See: .planning/PROJECT.md (updated 2026-09-28)

**Core value:** Eine neue Maschine wird komplett in der UI zum Cluster-Node — ohne `talosctl`, ohne Omni.
**Current focus:** Phase 14 — telefon-tippziele

## Current Position

Phase: 14 (telefon-tippziele) — EXECUTING
Plan: 7 of 7
Status: Phase complete — ready for verification
Last activity: 2026-09-30 — Phase 14 execution started

Progress: [█████░░░░░] 50%

## Offen

- **Release-Blocker OPS-05** (Ledger 87) plus 82, 84, 85: ein Durchlauf auf
  echter amd64-Hardware.
- **Nur gegen Fakes bewiesen:** Manifeste (148), Workload-Proxy (151),
  CA-Rotation (103, 143), Contract-Suite real (76), QEMU-Tier (80, 81).
- **Phase-2-Messlücken:** 28, 30, 32, 41, 42, 56, 70, 72. Abweichung 91.
- **Telefon-UI (Phase 14):** Der Tippziel-Wächter existiert seit e2bd690
  (2026-09-18) in `web/scripts/layout-audit.mjs` und läuft in `task ci` mit;
  laut 400e1d4 bestanden am 2026-09-26 alle Routen. Die frühere Zeile hier
  („10 Bedienelemente unter 44 px, kein Wächter") stammte aus der Übergabe vom
  2026-09-17. Offen ist, was der Wächter nicht öffnet — Dialoge, Menüs, die
  mobile Navigation — und dass seine Routen eine Hand-Liste sind.
- **Phase 13 offen (Stand 2026-09-30):** Verifikation 5/5, `human_needed`.
  Entschieden vom Betreiber: HACT-04 braucht eine **fünfte Aktion „nur nach
  Updates suchen"**, die nichts installiert — „Check for updates and install"
  allein reicht nicht. Noch zu planen (13-12). Außerdem nur auf dem Pi
  prüfbar: Helfer installieren, echter Neustart mit zwei Sitzungen,
  Dienst-Neustart und Herunterfahren je einmal.
- **Entscheidungen des Betreibers:** Cloud-/Infra-Provider, SAML.
- **Auf echter Hardware nicht geübt:** Live-Sensoren, Wake-on-LAN, erzwungene
  Power-Aktionen.

Maßgeblich für den Ledger ist `WINDOWS.md` (20 offen, 19 verzichtet).

## Accumulated Context

### Decisions

Siehe `PROJECT.md`, die `MILESTONE-*.md`-Dateien und für v1.18 den Abschnitt
„Entscheidungen" in `REQUIREMENTS.md`.

- [v1.18 Roadmap]: Senkrechte Scheiben statt „erst Backend, dann Seite" — jede
  Phase endet mit etwas, das der Betreiber im Browser prüfen kann.
- [v1.18 Roadmap]: Telefon zuletzt (Phase 14), weil `/host` seine Knöpfe und
  den Bestätigungsdialog erst mit Phase 13 hat und MOB-03 sie messen soll.
- [Phase 11]: internal/host reads through fs.FS rooted at / plus a four-method Sys seam (uname, CLOCK_BOOTTIME, sysinfo, statfs); no os.ReadFile/os.Open, no exemption from the file-access guard
- [Phase 11]: Reading[T] keeps its value behind a pointer tagged omitempty; health.Field is not reused because omitzero drops a readable 0
- [Phase 11]: os-release escapes are unescaped inside either quote style (Python platform.freedesktop_os_release behaviour)
- [Phase 11]: GET /api/v1/host: session + reader, not audited, not a wall-link route; collector built unconditionally in main
- [Phase 11]: 11-02: update status recorded by root into /var/lib/holzkube-manager-update/status.json from one EXIT trap (set +e, || true); record_status also guards itself; TMP/OUTCOME/INSTALLED/LATEST start empty
- [Phase 11]: 11-02: updatestatus.Read is strict (4 KiB, one object, RFC 3339, five outcomes, version pattern, null only for failed); lives in internal/host/updatestatus
- [Phase 11]: 11-03: hardening named only with proof -- ENOENT plus subset=pid in the topmost /proc super options
- [Phase 11]: 11-03: load falls back to sysinfo(2) on any /proc/loadavg failure
- [Phase 11]: 11-03: rate memo advances only past 0.5 s (or on a negative window); rates_over_seconds reported even while /proc/stat is hidden, for plan 06's network rates
- [Phase 11]: Filesystem Use% on /host is df's: used/(used+available) rounded up; UI-SPEC's used/size and nearest rounding both differ from the shell (23% / 24% vs 25%)
- [Phase 11]: Host filesystems merge only on equal major:minor from mountinfo; without a mount table nothing is merged
- [Phase 11]: Data-dir size walk: 60 s cache counts failed attempts too; a failed re-walk serves the last good value with its measured_at
- [Phase 11]: Host sensors reuse the talos rules (ClassifyChip, ThermalTwinName, Millidegrees) exported, not copied; thermal zones only when no CPU hwmon chip reported; trip points never used as limits
- [Phase 11]: Sensor list, fan rows and temperature limits live in web/src/components/charts/Sensors.tsx, shared by NodeHardware and /host; the sparkline column exists only when a history is passed
- [Phase 11]: 11-06: physical interface = fs.Lstat(<if>/device) succeeds, never a name rule; the address file is never read
- [Phase 11]: 11-06: link rates share the CPU memo's window and advance rule; null (never 0) for first read, a new link, or a backwards counter
- [Phase 11]: 11-07: docs call it a unit with ProcSubset=pid as the reference installation runs it -- the repository ships no systemd unit
- [Phase 11]: 11-07: browser layout tests set the viewport, not only the box, and measure text runs as well as element boxes
- [Phase 12]: 12-01: TestTheHostIsNeverForgotten pins the key host/local; HostSubject=machine/local alone cannot redden Retain since retained compares against HostSubject
- [Phase 12]: 12-01: RangePicker sits under the Live heading in LiveSection; heading/copy rewrite left to plan 05
- [Phase 12]: Temperature lines computed once in Go (inventory.TemperatureLimits) and sent as warn_c/danger_c; drives carry their sensor's pair
- [Phase 12]: Host limits from the twin thermal zone: lowest passive/hot = high, lowest critical = crit, active trips never a line
- [Phase 12]: host.Assess rates temperatures and filesystems only; any unreadable rated value is never ok
- [Phase 12]: 12-03: rx/tx recorded only with a rate window, >=1 physical link and every physical link rated; a host without physical links records no rx/tx
- [Phase 12]: 12-03: the wall reads the sampler's snapshot (Latest), never the host; older than 3 x FineStep is unknown / not readable; host is null before the first sample
- [Phase 12]: 12-04: the browser draws warn_c/danger_c as sent; temperatureSchema requires both with no default, the browser's limit table is deleted
- [Phase 12]: 12-05: one set of chart blocks (charts/HardwareCharts) for a node and /host; a page places them, it does not own a copy
- [Phase 12]: 12-05: a chart slot shows 'Not readable — no history' only when unreadable now (not rate.no-baseline) and no point in the window
- [Phase 12]: 12-05: with usage not readable (no-baseline included) the 1-minute load is the Processor card's figure
- [Phase 12]: 12-06: HostStateMark line form carries no sr-only word (the visible word follows it); sidebar words get their space as a separate text node
- [Phase 12]: 12-px word space: --text-xs--letter-spacing: normal (the body's -0.015em is inherited as -0.24px); held by typography.browser.test.tsx
- [Phase 12]: Layout audit and README renderer serve /api/v1/host/history from demo.json, ending at the fixture's observed_at
- [Phase 12]: 12-08: TestEveryProblemCodeIsInTheContract sees only problem.go constants; string-literal codes (17 undocumented) deferred, not fixed
- [Phase 13]: 13-01: hostaction.ReadResult accepts '- -' with rejected and failed (never started): the helper writes '- - failed' when it cannot remove the order; 13-04's reader table must refuse '-' only with started
- [Phase 13]: 13-01: host token intent is {Action: host.<a>, Machine: @host}, rebuilt from the route; hostTypedPhrase is separate from typedPhrase and all four require typing
- [Phase 13]: Wall host sentences are figure first (Health.Public); page Warnings stay name first
- [Phase 13]: An untyped fallback thermal zone is named in Unread by its directory name
- [Phase 13]: 13-03: the audit redactor keeps a non-allowlisted key with <redacted>; host-action audit tests assert the value, not key absence
- [Phase 13]: 13-04: an oversize order is journalled as 'zu lang (mehr als 64 Byte)' -- the helper reads at most 65 bytes and states no length it did not measure
- [Phase 13]: 13-04: the helper's age window names two reasons, 'veraltet' (>60 s old) and 'aus der Zukunft' (>5 s ahead)
- [Phase 13]: 13-04: ReadResult requires an absolute path; a relative one is refused
- [Phase 13]: 13-05: the process guard's AST half resolves import aliases and refuses dot imports of os/syscall/x/sys/unix
- [Phase 13]: 13-05: withdrawal claims by rename (fsstore.Claim); the pickup timer re-checks id and closed under the Box's mutex
- [Phase 13]: 13-06: helper detection reads the files systemd reads (no D-Bus); both host routes refuse container first, then helper missing, before Check and Place
- [Phase 13]: 13-07: the helper's units are verified by systemd-analyze output (any output fails), with a ProtectHom=true negative control in the same test
- [Phase 13]: 13-07: HOST-HELPER.md's drop-in for another data directory adds ReadWritePaths, or the consuming rm fails
- [Phase 13]: 13-08: HostActions/HostView take sessionRole (biome reads role= as ARIA); the status box follows the held order until dismissed, else the daemon's order or the helper's result within 15 min of observed_at
- [Phase 13]: 13-09: the fixture's helper-missing actions are held to InstallCommands and Detect by TestTheFixtureShowsTheRealInstallCommands
- [Phase 13]: 13-09: readme-images.mjs scrolls to the top before measuring a tall shot
- [Phase 13]: G-13-2 fixed in Detect: not-enabled only asked once both unit files are there, so the page sentence stays exact (D-12)
- [Phase 13]: G-13-3: host action button classes go through cn() as whitespace-bounded literals; a class flush against a template interpolation never reaches Tailwind's output
- [Phase 14]: 14-01: D-05 held on the real path -- the Sudo dialog opener gets the audit daemon's real 428 on POST /api/v1/users from the New account form; no fixture 428
- [Phase 14]: 14-01: route guard uses router.matchRoutes (A1 held); quick guard command must run from web/ (npm --prefix web exec runs vitest in the repo root)
- [Phase 14]: 14-02: focus on cancel is returned from an opener ref, because Radix's modal DialogContent only refocuses a DialogTrigger
- [Phase 14]: 14-02: host-level pair gap is a zero-width spacer (8+0+8=16 px above md, measured 32 before) -- the phase's one deliberate change above md, landed before the D-08 before-dump
- [Phase 14]: 14-03: no new layout-audit exemption; disabled controls are measured, the remaining skips each carry their reason
- [Phase 14]: 14-03: /host helper-installed variant lives in web/fixtures/host-helper-installed.json, merged over the demo host by both the audit and fixtures.test.ts
- [Phase 14]: 14-03: first extended audit run red with 7 findings: primitives dialog-close icon-sm, dropdown-menu-item, select-item (14-05); nav links, WhatsNew version button and chips, Reset disk checkboxes (14-06)
- [Phase 14]: 14-04: the D-08 dump comes out of findSmallTargets' own loop (one CONTROL_SELECTOR, same skips) and records each control's own box; a LAYOUT_DUMP path inside the repository is refused
- [Phase 14]: 14-04: noise floor 0 of 964 controls without any determinism fix or exclusion; before-dump at $HOME/.cache/holzkube-manager-layout/14-before.jsonl, not committed
- [Phase 14]: 14-05: Unused Button sizes (xs, icon-xs, icon-lg) get max-md:size-11 too, so a first use cannot bring back a small target
- [Phase 14]: 14-05: DialogHeader max-md:pr-8 applies also without a close X (SudoDialog)
- [Phase 14]: 14-05: The toast dismiss is held by a Chromium component test, not an audit opener; Tailwind generates max-md:size-11! (A2 confirmed)
- [Phase 14]: 14-06: The phone drawer scrolls (max-md:overflow-y-auto); without it the audit reports CUT OFF, 885px in 844px
- [Phase 14]: 14-06: Reset dialog disk rows are one label each, taking the li's flex classes; wipe-mode radios left unchanged because the audit did not name them
- [Phase 14]: [Phase 14-07]: D-10 red checks (a) (b) (c) each exit 201 from task test:layout; D-08 after-dump compared 964 controls, 0 differ
- [Phase 14]: [Phase 14-07]: D-11 phone hand test not performed; ROADMAP criterion 4 phone half open (human_needed), MOB-03 stays partial; repeat D-08 for /host and D-11 after 13-12
- [Phase 13]: 13-12: Check for updates is a fifth host order, check-update, run by the root helper through holzkube-manager-update-check.service (the update script's --check), behind all four locks
- [Phase 13]: 13-12: a finished check is emerald for current and available, red only for failed; orderPhase compares checked_at with the placement truncated to the second
- [Phase 13]: 13-13: The helper names its orders on one marker line '# holzkube-manager-host orders: ' plus the words; no line means the first four (reboot poweroff restart-service update). Additive: a later order is a new word
- [Phase 13]: 13-13: The check unit is held to the helper service's sandbox except RestrictAddressFamilies (reaches GitHub) and ReadWritePaths (writes only its StateDirectory), with an empty CapabilityBoundingSet and a start limit below the helper's
- [Phase 13]: 13-14: KnownOrders reads the installed helper's marker line only after Detect's ownership rule and bounded at 64 KiB; no marker, two markers or too large counts as the four orders of 13-01
- [Phase 13]: 13-14: actions.outdated (script-outdated, check-unit) is asked only while nothing is missing; available unchanged; both host routes refuse check-update with 409 conflict.host-helper-outdated after container and missing, before token and Place
- [Phase 13]: 13-15: the older-helper notice renders only when nothing is missing, independent of the server keeping outdated empty then; both helper notices share one frame
- [Phase 13]: 13-15: D-08's route/opener/index/tag key turns an index shift across a tag boundary into ONLY BEFORE/AFTER pairs; 'no control disappeared' is judged by re-keying /host by opener, tag and name
- [Phase 13]: 13-16: update is refused with 409 conflict.host-update-unit-missing while holzkube-manager-update.service is not a regular file in /etc/systemd/system; order container, missing, update script, update unit, outdated, busy
- [Phase 13]: 13-16: CapabilityBoundingSet=, SystemCallFilter=, ProtectProc= and ProcSubset= stay out of the shipped update unit until a real install and restart through it is measured as root
- [Phase 13]: 13-17: single-button reasons share one line in button order (at most two sentences); actionReason asks group, update script, update unit, older helper, as the routes do

### Blockers/Concerns

- OPS-05 braucht eine Maschine, keinen Code.
- Die Produktions-Unit auf dem Pi läuft mit `ProcSubset=pid`. Nach Phase 11
  zeigt der Host dort CPU, Load und Speicher deshalb als „nicht lesbar", bis
  der Betreiber die Zeile ändert — so gewollt, und die Seite nennt die Zeile.
- Phase 11 (Update-Status) und Phase 13 (Helfer) liefern Dateien nach
  `deploy/`, installiert wird nichts. Bis der Betreiber installiert, zeigt der
  Pi „nicht hinterlegt" und gesperrte Aktionen; das ist der Soll-Zustand, keine
  Lücke.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 261001-sa9 | Unlink single sign-on (admin, sudo, audit); person accounts only for SSO linking; SSO-only pre-check removed; review fixes CR-01, WR-01..07 | 2026-10-01 | 05f1279 | [261001-sa9-unlink-single-sign-on](./quick/261001-sa9-unlink-single-sign-on/) |

## Session Continuity

Last session: 2026-10-02T04:57:09.026Z
Stopped at: Completed 13-17-PLAN.md
Resume file: None
Next: `/gsd-plan-phase 11`

## Performance Metrics

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 11 P01 | 37min | 3 tasks | 32 files |
| Phase 11 P02 | 9min | 2 tasks | 4 files |
| Phase 11 P03 | 25min | 3 tasks | 22 files |
| Phase 11 P04 | 24min | 3 tasks | 16 files |
| Phase 11 P05 | 60min | 3 tasks | 60 files |
| Phase 11 P06 | 16 min | 2 tasks | 13 files |
| Phase 11 P07 | 28 min | 3 tasks | 7 files |
| Phase 12 P01 | 21min | 2 tasks | 13 files |
| Phase 12 P02 | 18 min | 3 tasks | 17 files |
| Phase 12 P03 | 18 min | 3 tasks | 12 files |
| Phase 12 P04 | 9 min | 2 tasks | 8 files |
| Phase 12 P05 | 14 min | 3 tasks | 6 files |
| Phase 12 P06 | 15 min | 3 tasks | 11 files |
| Phase 12 P07 | 30 min | 3 tasks | 9 files |
| Phase 12 P08 | 42 min | 2 tasks | 3 files |
| Phase 13 P01 | 32min | 2 tasks | 21 files |
| Phase 13 P02 | 13min | 2 tasks | 8 files |
| Phase 13 P03 | 14 min | 2 tasks | 1 files |
| Phase 13 P04 | 9min | 2 tasks | 4 files |
| Phase 13 P05 | 18min | 3 tasks | 6 files |
| Phase 13 P06 | 19min | 3 tasks | 11 files |
| Phase 13 P07 | 14min | 2 tasks | 5 files |
| Phase 13 P08 | 24 min | 3 tasks | 6 files |
| Phase 13 P09 | 45min | 2 tasks | 9 files |
| Phase 13 P10 | 10min | 3 tasks | 12 files |
| Phase 13 P11 | 20min | 2 tasks | 2 files |
| Phase 14 P01 | 21min | 3 tasks | 7 files |
| Phase 14 P02 | 11min | 2 tasks | 5 files |
| Phase 14 P03 | 20min | 2 tasks | 4 files |
| Phase 14 P04 | 11min | 2 tasks | 2 files |
| Phase 14 P05 | 12min | 2 tasks | 6 files |
| Phase 14 P06 | 18min | 2 tasks | 4 files |
| Phase 14 P07 | 50min | 3 tasks | 2 files |
| Phase 13 P12 | 33min | 2 tasks | 16 files |
| Phase 13 P13 | 23min | 2 tasks | 15 files |
| Phase 13 P14 | 20min | 2 tasks | 15 files |
| Phase 13 P15 | 75min | 3 tasks | 11 files |
| Phase 13 P16 | elapsed 2026-10-01 18:10 to 2026-10-02 06:15 CEST with a pause | 3 tasks | 14 files |
| Phase 13 P17 | 41min | 3 tasks | 14 files |
