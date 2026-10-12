---
phase: quick-261012-ppl
plan: 01
type: execute
wave: 1
depends_on: []
autonomous: true
requirements:
  - QUICK-261012-ppl
---

# Lint-Funde im v0.7.0-Gate beheben

Der Release-Lauf 38277179453 (2026-10-12) scheiterte im Schritt "Lint Go" mit
7 Funden in den Testdateien von Phase 16: 6x revive unused-parameter (`r`),
1x errcheck (ungeprüfte Typzusicherung auf http.Hijacker). Kein Release, kein Tag.

## Muss wahr sein

- `golangci-lint` v2.13.1 mit Go 1.26.9 meldet für das ganze Repository 0 Funde.
- Die betroffenen Tests prüfen dasselbe wie vorher.
