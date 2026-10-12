---
phase: quick-261012-ppl
plan: 01
subsystem: ci
tags: [lint, revive, errcheck, release-gate]
status: complete
requires: []
provides:
  - "lint-clean tests of internal/k8srelease and internal/upgrade"
key-files:
  modified:
    - internal/k8srelease/k8srelease_test.go
    - internal/upgrade/kubernetes_source_test.go
---

# Quick 261012-ppl: Lint-Funde — Summary

Sechs Parameter `r` zu `_`, die Typzusicherung auf `http.Hijacker` wird geprüft.
Keine Logik geändert.

## Geprüft

- `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...`: "0 issues." (CI-Version, CI-Toolchain). Erstmals lokal gelaufen;
  vorher stand in den Berichten "Linter nicht installiert".
- `go test ./internal/k8srelease ./internal/upgrade`, `go vet`, `gofmt`: grün.

## Nicht geprüft

- Der Linter wurde nicht gegen die 7 Funde rot gesehen (der Fehlerzustand war der
  CI-Lauf selbst, mit denselben 7 Zeilen); der Beweis ist der nächste CI-Lauf.
- Alles andere, was lokal nicht läuft: task ci, Layout, -race über das ganze Repository.
