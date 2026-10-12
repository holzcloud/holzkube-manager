---
phase: quick-261012-hxm
plan: 01
type: execute
wave: 1
depends_on: []
autonomous: true
requirements:
  - QUICK-261012-hxm
---

# Dependabot: fünf Actions-Bumps gebündelt

Dependabot hat fünf PRs (#14 setup-task 1→2.2.0, #15 setup-node 4→7.0.0,
#16 goreleaser-action 6→7.2.3, #17 checkout 4→7.0.1, #18 setup-go 5→7.0.0)
geöffnet, alle über mehrere Hauptversionen und ungetestet; die Runner warnen, dass
die bisherigen Versionen Node 20 nutzen. Entscheidung des Betreibers
(2026-10-12): in einem Test-Branch bündeln, ein CI-Lauf, bei Grün nach `main`.

## Muss wahr sein

- Alle fünf Bumps stehen in `.github/workflows`, angepinnt auf Commit-SHAs.
- Ein Gate-Lauf (ohne Tag, also ohne Release) auf dem Branch ist grün.
- Der Release-Job wird beim Lauf übersprungen.
