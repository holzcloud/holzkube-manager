---
phase: quick-261014-gl2
plan: 01
subsystem: certwatch
tags: [certwatch, deadline, bugfix, v0.7.1]
status: complete
requires: []
provides:
  - "certwatch.DefaultReadBudget, Deps.ReadBudget; every Read has a deadline"
key-files:
  modified:
    - internal/certwatch/certwatch.go
  created:
    - internal/certwatch/deadline_test.go
---

# Quick 261014-gl2: certwatch-Frist — Summary

Korrektur des Fehlers aus v0.6.0/v0.7.0: `Refresh` legt um jeden `Read` eine Frist
von 20 s.

## Geprüft

- Selbst nachgemessen vor der Korrektur: `Certificates(context.Background())` gegen
  talossim liefert `talos: refusing a call with no deadline`, mit Frist 7 Zertifikate.
- Rot gesehen gegen die wieder eingebaute Fehlerform (Frist durch `WithCancel`
  ersetzt): `TestEveryReadCarriesADeadline` und
  `TestARunningDaemonsContextReadsARealNode` scheitern; mit Korrektur grün
  (Exit 0 aus dem Testbefehl, Datei `cmp`-gleich zurückgebaut).
- `golangci-lint` v2.13.1 (Go 1.26.9) über das ganze Repository: 0 Funde. `go vet`,
  `gofmt`, `./internal/certwatch`, `./cmd/holzkube-managerd`, `./internal/publicrepo`: grün.

## Nicht geprüft

- Auf dem Pi und gegen einen echten Cluster: erst der Daemon aus dem nächsten Release
  zeigt, ob die Karte Zertifikate füllt. Das Journal des Pi sollte bis dahin
  `certificate watch: read a node` mit der Frist-Meldung zeigen (nicht gelesen).
- `task ci`, `-race` über das ganze Repository, Layout (Mac).
