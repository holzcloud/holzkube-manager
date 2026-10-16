# Phase 17: Versionsprüfung zwischen Plan und Apply - Context

**Gathered:** 2026-10-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Eine Konfiguration, die zwischen Plan und Apply von jemand anderem geändert wurde, wird nicht still überschrieben. Heute überschreiben zwei Operatoren einander still, wenn sie dieselbe Konfiguration planen und anwenden (`machineconfig.Service.Plan` / `Apply`, `Compare`). Geliefert wird: der Plan trägt einen Digest der gelesenen aktiven Konfiguration; das Apply liest erneut und verweigert bei Abweichung mit einem Satz, der sagt, was sich seitdem geändert hat.

</domain>

<decisions>
## Implementation Decisions

Alle als Auswahl vom Betreiber am 2026-10-16 getroffen, sofern nicht als Ermessen von Claude gekennzeichnet.

- **D-01:** Bei Abweichung zwischen Plan und Apply wird das Apply **verweigert**, der Betreiber plant neu. **Kein Überschreib-Schalter.** — **Reversibility:** reversible — ein späterer „trotzdem anwenden"-Weg wäre additiv.
- **D-02:** Der Satz der Verweigerung nennt, **was sich geändert hat**, über alle Dokumente der Konfiguration (also mit `machineconfig.CompareDocuments` aus Phase 15, nicht mit dem Ein-Dokument-`Compare`), redigiert wie die Konfigurationsansicht.

### Claude's Discretion
Wie der Digest durch Plan und Bestätigung getragen wird (Token der Bestätigung, Feld der Vorschau), Fehlercode und HTTP-Status der Verweigerung (409 mit eigenem Problemcode, im API-Vertrag), Wortlaut. Das Verhältnis zur Drift-Basis aus Phase 15 (ein eigenes Apply frischt die Basis über den erwarteten Digest auf) darf sich nicht verändern.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

- `CLAUDE.md` — öffentliches Repository, README im selben Schritt, Changelog erst beim Release, jeder Wächter rot gesehen, Fragen als Auswahl, Linter lokal: `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...`.
- `.planning/AUDIT-2026-10-08.md` — Quelle des Kandidaten.
- `internal/machineconfig/service.go` (`Plan`, `Apply`, `reserveStaged`), `diff.go`, `snapshot.go` (Phase 15: `CompareDocuments`, `ReadSnapshot`), `internal/httpapi/handlers` (Konfigurationsrouten), `web/src` (Konfigurationsseite, Vorschau und Bestätigung), `docs/api-contract.md`, `docs/guide.md`.
- Muster für Aufbau und Beweis: `internal/certwatch` und `internal/configdrift` (periodischer Leser, „not read yet", Wächter vom Kontext **ohne Frist** aus — der Talos-Client verweigert Aufrufe ohne Frist, siehe Quick-Task 261014-gl2), die Phasen 15 und 16 unter `.planning/phases/`.

</canonical_refs>

<deferred>
## Deferred Ideas

- Ein „trotzdem anwenden"-Weg mit Sudo (bewusst nicht gewählt).
- Dasselbe Prinzip für andere Aktionen (Upgrade, Reboot), die auf einem Plan beruhen.

</deferred>

---

*Phase: 17-versionspruefung-zwischen-plan-und-apply*
*Context gathered: 2026-10-16*
