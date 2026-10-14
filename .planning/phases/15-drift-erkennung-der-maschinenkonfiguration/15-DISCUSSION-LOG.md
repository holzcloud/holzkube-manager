# Phase 15: Drift-Erkennung der Maschinenkonfiguration - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-10-14
**Phase:** 15-drift-erkennung-der-maschinenkonfiguration
**Areas discussed:** Vergleichsbasis, Speichern und Rauschen, Anzeige/Warnung/Auflösung, Lesen

---

## Vergleichsbasis

| Option | Selected |
|--------|----------|
| Letzter bekannter Stand (nach erstem Sehen und eigenem Apply) | ✓ |
| Gegen Schwesterknoten derselben Rolle | |
| Gegen aus Geheimnissen und Patches erzeugte Konfiguration | |

## Speichern

| Option | Selected |
|--------|----------|
| Redigierter Schnappschuss | ✓ |
| Nur ein Hash | |
| Vollständige Konfiguration mit Geheimnissen | |

## Lesen

| Option | Selected |
|--------|----------|
| Periodisch wie certwatch | ✓ |
| Per COSI-Watch | |

## Rauschen

| Option | Selected |
|--------|----------|
| Feste Ausnahmeliste, per Recherche belegt | ✓ |
| Alles vergleichen | |
| Betreiber kann Pfade ausnehmen | |

## Anzeige (Mehrfachauswahl)

Knotenseite mit Diff ✓ · Zeile auf der Clusterkarte ✓ · Banner auf jeder Seite ✓ · Metrik in /metrics ✓

**Notes:** Für das Banner hatte Claude vor Dauerrauschen gewarnt; der Betreiber blieb dabei und wählte die Abschwächung (siehe Banner).

## Banner

| Option | Selected |
|--------|----------|
| Erst wenn Drift 24 Stunden unbestätigt bleibt | ✓ |
| Sofort bei jeder Drift | |
| Nie auf jeder Seite | |

## Auflösung

| Option | Selected |
|--------|----------|
| Ansehen und „so ist es gewollt" bestätigen | ✓ |
| Nur ansehen | |
| Bestätigen und zurücksetzen | |

## Deferred Ideas

Zurücksetzen, Vergleich gegen Schwesterknoten/Soll-Konfiguration, pflegbare Ausnahmen, Wand-Kachel.
