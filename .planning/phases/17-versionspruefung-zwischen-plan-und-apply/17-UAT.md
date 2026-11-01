---
status: complete
phase: 17-versionspruefung-zwischen-plan-und-apply
source: 17-01-SUMMARY.md, 17-02-SUMMARY.md
started: 2026-11-01T11:15:00Z
updated: 2026-11-01T11:15:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Ein unveraenderter Knoten wird angewendet
expected: Plan, kurz warten, Apply ohne Aenderung am Knoten geht durch (Ledger 180, A3).
result: pass
source: automated
evidence: TestAnApplyOnAnUnchangedNodeStillLandsOverTheRoute (HTTP-Route, echter Talos-Client, talossim); Zwei Plaene eines unveraenderten Knotens tragen denselben Digest. NUR SIMULATOR, echter Knoten bleibt Ledger 180.

### 2. Fremde Aenderung zwischen Plan und Apply wird verweigert
expected: Plan, dann den Knoten mit talosctl in einem anderen Dokument aendern, dann Apply: gelbe Meldung mit dem Pfad der Aenderung, Apply gesperrt, Knoten behaelt die fremde Aenderung.
result: pass
source: automated
evidence: TestAnApplyAfterAForeignChangeIsRefusedOverTheRoute und TestTheRefusalSentenceNamesPathsAndNothingElse. Fehler eingebaut (checkBase uebersprungen): 4 Tests rot (machineconfig 3, httpapi 1), danach zurueckgestellt.

### 3. "Erneut planen" setzt auf dem neuen Stand auf
expected: Button "See what would change again" wiederholt den Plan mit demselben Patch-Text; danach ist Apply wieder moeglich.
result: pass
source: automated
evidence: config.test.tsx: Alert mit Satz des Servers, Apply gesperrt, neuer Plan sendet neuen Digest, Satz bleibt bis der neue Plan da ist (jsdom, nicht im Browser).

### 4. Dieselbe Vorschau laesst sich nicht zweimal anwenden
expected: Nach erfolgreichem Apply ist die Vorschau gesperrt; ein zweiter Apply derselben Vorschau ist nicht moeglich (UI) bzw. wird mit 409 verweigert.
result: pass
source: automated
evidence: UI: cannot apply the same preview a second time after it landed; Server: TestTwoAppliesOnOneNodeDoNotOverwriteEachOther (zweiter Apply auf altem Stand wird verweigert).

### 5. Audit-Log traegt den Digest, nie den Patch-Text
expected: Eintrag config.apply im Audit zeigt base_digest und keinen Patch-Inhalt.
result: pass
source: automated
evidence: TestConfigApplyKeepsItsBaseDigestAndNothingOfThePatches.

## Summary

total: 5
passed: 5
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

[none yet]

Hinweis: alle fuenf Pruefungen sind automatisiert (Simulator, jsdom). Kein Browser, kein echter Knoten. Handprobe auf dem Pi: Ledger 180. Ein roter Test ausserhalb der Phase: TestNoAssetIsInlinedPastTheContentSecurityPolicy prueft ein veraltetes, ignoriertes Build vom 2026-09-03 in internal/httpapi/dist.
