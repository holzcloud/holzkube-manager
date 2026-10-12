---
phase: quick-261012-hxm
plan: 01
subsystem: ci
tags: [github-actions, dependabot, pinned-shas]
status: complete
requires: []
provides:
  - "checkout v7.0.1, setup-go v7.0.0, setup-node v7.0.0, setup-task v2.2.0, goreleaser-action v7.2.3, jeweils auf SHA gepinnt"
affects: [.github/workflows/ci.yml, .github/workflows/factory-drift.yml]
key-files:
  modified:
    - .github/workflows/ci.yml
    - .github/workflows/factory-drift.yml
commits:
  - 064f48d "Merge branch 'deps/actions-bump'"
---

# Quick 261012-hxm: Dependabot-Bumps — Summary

Branch `deps/actions-bump` aus `main` mit den fünf Dependabot-Branches. Ein
Konflikt in `factory-drift.yml` (checkout und setup-go stehen dort in
benachbarten Zeilen) wurde von Hand gelöst, beide Bumps übernommen.

## Geprüft

- CI-Dispatch ohne Tag auf `deps/actions-bump` (Lauf 38237997688, Kopf `c675c79`):
  „Build, lint and test" grün, Release und holzcloud.ch-Job übersprungen.

## Nicht geprüft

- **Release-Job mit den neuen Versionen** (goreleaser-action 7, Verifikation):
  der Lauf ohne Tag überspringt ihn. Ob er mit den neuen Versionen sauber läuft,
  zeigt erst das nächste Release.
- `factory-drift.yml` (wöchentlich, Donnerstag) lief nicht mit den neuen
  Versionen; der nächste Lauf ist der Beleg.
- Die fünf Dependabot-PRs wurden nicht von Hand geschlossen; sie schließen sich,
  wenn GitHub die Commits in `main` erkennt.
