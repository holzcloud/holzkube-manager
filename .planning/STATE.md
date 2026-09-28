---
gsd_state_version: "1.0"
milestone: v1.18
milestone_name: Host & Telefon
current_phase: 11
current_phase_name: Host-Seite — Gerät, Dienst, Live-Werte
status: executing
stopped_at: Completed 11-06-PLAN.md
last_updated: "2026-09-28T17:09:14.343Z"
last_activity: 2026-09-28
last_activity_desc: Phase 11 execution started
state_head: d3eae877a312c9ed4c0b137b9a2c48ff6c24338d
progress:
  total_phases: 4
  completed_phases: 0
  total_plans: 7
  completed_plans: 6
  percent: 0
---

# Project State

Stand 2026-09-28. Der vorige Bericht (Abschluss v1.15, Übergabe 2026-09-17)
liegt unverändert in `milestones/STATE-2026-09-17.md`.

## Project Reference

See: .planning/PROJECT.md (updated 2026-09-28)

**Core value:** Eine neue Maschine wird komplett in der UI zum Cluster-Node — ohne `talosctl`, ohne Omni.
**Current focus:** Phase 11 — Host-Seite — Gerät, Dienst, Live-Werte

## Current Position

Phase: 11 (Host-Seite — Gerät, Dienst, Live-Werte) — EXECUTING
Plan: 7 of 7
Status: Ready to execute
Last activity: 2026-09-28 — Phase 11 execution started

Progress: [░░░░░░░░░░] 0%

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

Last session: 2026-09-28T17:09:14.282Z
Stopped at: Completed 11-06-PLAN.md
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
