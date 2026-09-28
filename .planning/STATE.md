---
gsd_state_version: "1.0"
milestone: v1.18
milestone_name: Host & Telefon
current_phase: 12
current_phase_name: Host wie ein Knoten — Verlauf, Warnung, Wand
status: executing
stopped_at: Completed 12-06-PLAN.md
last_updated: "2026-09-28T21:26:16.855Z"
last_activity: 2026-09-28
last_activity_desc: Phase 12 execution started
state_head: 1886bc4ed13ffba6c499b1799d466506afaee20c
progress:
  total_phases: 4
  completed_phases: 1
  total_plans: 15
  completed_plans: 13
  percent: 25
---

# Project State

Stand 2026-09-28. Der vorige Bericht (Abschluss v1.15, Übergabe 2026-09-17)
liegt unverändert in `milestones/STATE-2026-09-17.md`.

## Project Reference

See: .planning/PROJECT.md (updated 2026-09-28)

**Core value:** Eine neue Maschine wird komplett in der UI zum Cluster-Node — ohne `talosctl`, ohne Omni.
**Current focus:** Phase 12 — Host wie ein Knoten — Verlauf, Warnung, Wand

## Current Position

Phase: 12 (Host wie ein Knoten — Verlauf, Warnung, Wand) — EXECUTING
Plan: 7 of 8
Status: Ready to execute
Last activity: 2026-09-28 — Phase 12 execution started

Progress: [███░░░░░░░] 25%

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

### Blockers/Concerns

- OPS-05 braucht eine Maschine, keinen Code.
- Die Produktions-Unit auf dem Pi läuft mit `ProcSubset=pid`. Nach Phase 11
  zeigt der Host dort CPU, Load und Speicher deshalb als „nicht lesbar", bis
  der Betreiber die Zeile ändert — so gewollt, und die Seite nennt die Zeile.
- Phase 11 (Update-Status) und Phase 13 (Helfer) liefern Dateien nach
  `deploy/`, installiert wird nichts. Bis der Betreiber installiert, zeigt der
  Pi „nicht hinterlegt" und gesperrte Aktionen; das ist der Soll-Zustand, keine
  Lücke.

## Session Continuity

Last session: 2026-09-28T21:26:16.719Z
Stopped at: Completed 12-06-PLAN.md
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
