---
phase: quick-261012-l9s
plan: 01
subsystem: compat
tags: [compat, talos, kubernetes, matrix, guard]
status: complete
requires: []
provides:
  - "compat.matrix 12: 30..35, 13: 31..36, 14: 32..37"
  - "TestTheNewestRowMatchesTheCompiledMachinery"
affects: [internal/upgrade (Kettenplanung unter Talos 1.14), phase 16]
key-files:
  modified:
    - internal/compat/compat.go
    - internal/compat/compat_test.go
    - internal/upgrade/plan_test.go
---

# Quick 261012-l9s: compat-Matrix — Summary

Zeilen 12 bis 14 nach der Talos-Doku (1.12: 1.30-1.35, 1.13: 1.31-1.36) und der
Maschinerie (1.14: Default 1.37, sechs Minors zurück, also 32..37). Phase 16
(`16-01` Precondition `14:.*MaxMinor: 37`) ist damit erfüllbar.

## Geprüft

- Rot gesehen, jeweils einzeln: Zeile 14 zurück auf Obergrenze 35, und Zeile 14
  mit Untergrenze 33. Beide lassen `TestTheNewestRowMatchesTheCompiledMachinery`
  scheitern; danach grün.
- `TestCheckClassifiesTheWindow` an die neue Zeile 13 angepasst (Werte, keine
  Logik). `TestAKubernetesChainBeyondWhatTalosSupportsIsRefused` wurde durch die
  Korrektur rot (Ziel v1.35 liegt jetzt in Talos 1.12s Fenster) und prüft nun ein
  Ziel weit über jedem Fenster (v1.99.0), weiter mit `errors.Is(ErrWouldStrand)`.
- Grün: `./internal/compat`, `./internal/upgrade`, `./internal/inventory`,
  `./internal/httpapi/handlers`, `./internal/talos`, `./internal/provision`,
  `./internal/health`; `go vet` der beiden geänderten Pakete.

## Nicht geprüft

- Zeilen 7 bis 11 der Matrix: keine Quelle gelesen. Rot gesehen ist nur Zeile 14;
  die Zeilen 12 und 13 stammen aus der Doku und werden von keinem Test gegen
  eine Quelle gehalten.
- Die Doku führt für 1.14 die Untergrenze 33; der Betreiber hat 32 gewählt.
- `task ci`, Layout, `-race` nicht gelaufen (Mac); CI fährt sie beim nächsten Release.
