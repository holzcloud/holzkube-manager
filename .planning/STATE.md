---
gsd_state_version: "1.0"
milestone: v1.18
milestone_name: Host & Telefon
status: planning
last_updated: "2026-09-28T12:45:46.492Z"
last_activity: 2026-09-28
progress:
  total_phases: 0
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

Stand 2026-09-28. Der vorige Bericht (Abschluss v1.15, Übergabe 2026-09-17)
liegt unverändert in `milestones/STATE-2026-09-17.md`.

## Project Reference

See: .planning/PROJECT.md

**Core value:** Eine neue Maschine wird komplett in der UI zum Cluster-Node — ohne `talosctl`, ohne Omni.
**Current focus:** zwischen Milestones. Ausgeliefert ist v0.1.0 und läuft auf dem Pi.

## Current Position

Phase: Not started (defining requirements)
Plan: —
Status: Defining requirements
Last activity: 2026-09-28 — Milestone v1.18 started

## Offen

- **Release-Blocker OPS-05** (Ledger 87) plus 82, 84, 85: ein Durchlauf auf
  echter amd64-Hardware.
- **Nur gegen Fakes bewiesen:** Manifeste (148), Workload-Proxy (151),
  CA-Rotation (103, 143), Contract-Suite real (76), QEMU-Tier (80, 81).
- **Phase-2-Messlücken:** 28, 30, 32, 41, 42, 56, 70, 72. Abweichung 91.
- **Telefon-UI:** 10 Bedienelemente unter 44 px bei 390 px; kein Wächter für
  Tippzielgrößen.
- **Entscheidungen des Betreibers:** Cloud-/Infra-Provider, SAML.
- **Auf echter Hardware nicht geübt:** Live-Sensoren, Wake-on-LAN, erzwungene
  Power-Aktionen.

Maßgeblich für den Ledger ist `WINDOWS.md` (20 offen, 19 verzichtet).

## Accumulated Context

### Decisions

Siehe `PROJECT.md` und die `MILESTONE-*.md`-Dateien.

### Blockers/Concerns

- OPS-05 braucht eine Maschine, keinen Code.
