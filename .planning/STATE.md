---
gsd_state_version: "1.0"
milestone: v1.18
milestone_name: Host & Telefon
current_phase: 11
current_phase_name: Host-Seite — Gerät, Dienst, Live-Werte
status: executing
stopped_at: Completed 11-02-PLAN.md
last_updated: "2026-09-28T14:57:56.024Z"
last_activity: 2026-09-28
last_activity_desc: Phase 11 execution started
state_head: 76573b2b84acfe99b03fbf68048114567e680cab
progress:
  total_phases: 4
  completed_phases: 0
  total_plans: 7
  completed_plans: 2
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
Plan: 3 of 7
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

Last session: 2026-09-28T14:57:55.846Z
Stopped at: Completed 11-02-PLAN.md
Resume file: None
Next: `/gsd-plan-phase 11`

## Performance Metrics

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 11 P01 | 37min | 3 tasks | 32 files |
| Phase 11 P02 | 9min | 2 tasks | 4 files |
