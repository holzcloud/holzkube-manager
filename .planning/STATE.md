---
gsd_state_version: "1.0"
milestone: v1.18
milestone_name: Host & Telefon
current_phase: 11
current_phase_name: Host-Seite — Gerät, Dienst, Live-Werte
status: executing
stopped_at: Roadmap v1.18 geschrieben (ROADMAP.md, STATE.md, Traceability in REQUIREMENTS.md), nicht committet
last_updated: "2026-09-28T14:04:41.229Z"
last_activity: 2026-09-28
last_activity_desc: "Roadmap für v1.18 angelegt: Phasen 11–14, 22 von 22 Anforderungen zugeordnet"
state_head: 41b7875241067c3a65d081adbe6e082556e02088
progress:
  total_phases: 4
  completed_phases: 0
  total_plans: 7
  completed_plans: 0
  percent: 0
---

# Project State

Stand 2026-09-28. Der vorige Bericht (Abschluss v1.15, Übergabe 2026-09-17)
liegt unverändert in `milestones/STATE-2026-09-17.md`.

## Project Reference

See: .planning/PROJECT.md (updated 2026-09-28)

**Core value:** Eine neue Maschine wird komplett in der UI zum Cluster-Node — ohne `talosctl`, ohne Omni.
**Current focus:** Phase 11 — Host-Seite: Gerät, Dienst, Live-Werte (v1.18 Host & Telefon). Ausgeliefert ist v0.1.0 und läuft auf dem Pi.

## Current Position

Phase: 11 (Host-Seite — Gerät, Dienst, Live-Werte) — READY TO EXECUTE
Plan: —
Status: Ready to execute
Last activity: 2026-09-28 — Roadmap für v1.18 angelegt: Phasen 11–14, 22 von 22 Anforderungen zugeordnet

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

Last session: 2026-09-28
Stopped at: Roadmap v1.18 geschrieben (ROADMAP.md, STATE.md, Traceability in REQUIREMENTS.md), nicht committet
Resume file: None
Next: `/gsd-plan-phase 11`
