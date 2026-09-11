# Phase 4: Walking Skeleton (Wegwerf) — Summary

**Executed:** 2026-09-11
**Status:** Instrument gebaut, **Messung nicht durchgeführt** — siehe unten

## Was diese Phase ist, und was sie nicht ist

Sie trägt absichtlich kein Requirement. Sie ist Wegwerf-Gerüst zur
Risikoentschärfung, und ihr Produkt ist **eine Messung**: vier Unbekannte, die
in Phase 8 sonst als offene Fragen auftauchen, sollen vorher bekannte Probleme
sein.

Das hat eine Konsequenz, die diese Zusammenfassung nicht verstecken darf: eine
Phase, deren Ergebnis eine Messung ist, ist nicht abgeschlossen, solange
niemand gemessen hat. Man kann den Weg nicht durch Lesen entschärfen.

## Was gebaut wurde

### `sandbox/cmd/walking-skeleton`

Der Weg einmal von Hand, hartkodiert und echt: eine MachineConfig aus einem
hartkodierten Schematic wird auf eine von Hand genannte Maintenance-Mode-IP
angewendet, die Stille danach wird **gemessen** statt geschätzt, dann wird
zweimal gebootstrappt und einmal auf eine falsche Adresse appliziert. Alles,
was dabei beobachtet wird, landet in einem Markdown-Protokoll — eine Messung,
die niemand aufgeschrieben hat, ist eine Anekdote.

Die Stille wird unter **Cluster**-Zugangsdaten gemessen, nicht unter den
Maintenance-Daten. Gemessen wird nicht „lauscht da etwas", sondern „antwortet
der Knoten, den diese Konfiguration gemacht hat, als Mitglied dieses Clusters".
Ein Probe, der den Maintenance-API akzeptierte, würde die Uhr anhalten, während
die Maschine noch die alte war.

### `sandbox/cmd/talos-sandbox --provider qemu`

Tier 2 zieht hierher vor. Derselbe Provisioner-Aufruf trägt jetzt beide
Provider: Docker (Tier 1, ein Container, der nie installiert und nie rebootet)
und QEMU (Tier 2, eine echte VM, die ein ISO bootet, auf eine Disk installiert
und wiederkommt). Nur auf Tier 2 sind die vier Unbekannten überhaupt messbar —
ein Container hat keine Installationsstille, weil er nichts installiert.

Beide leben im `sandbox/`-Modul. `TestBinaryDependencyWeight`,
`TestSandboxIsASeparateModule` und `TestModuleGraphExcludesTalosRoot` laufen
weiterhin grün: vom Talos-Root-Modul erreicht nichts das Produkt-Binary.

## Erfolgskriterien

| # | Stand |
|---|---|
| 1 | **nicht erfüllt** — QEMU (Tier 2) wurde nicht ausgeführt. Auf diesem Host gibt es weder `qemu-system-*` noch `/dev/kvm` noch `vmx`; es ist ein Container ohne verschachtelte Virtualisierung. Der `--provider qemu`-Pfad ist gebaut und übersetzt, aber nie gelaufen. |
| 2 | **nicht erfüllt** — folgt aus 1: ohne Maschine keine Maschine, die die Config annimmt |
| 3 | **nicht erfüllt** — die Installationsstille ist **ungemessen**. Das Messgerät steht, die Zahl fehlt. |
| 4 | **nicht erfüllt** — doppelter Bootstrap und falsche Ziel-IP sind im Programm ausgelöst, aber nie gegen echte Hardware. `talossim` *behauptet* `AlreadyExists`; ob echtes Talos das tut, hat diese Phase nicht festgestellt. |
| 5 | **erfüllt** — der Skeleton ist Wegwerf-Code, liegt im getrennten Modul, wird von nichts im Produkt importiert, und kein PROV-Requirement gilt dadurch als erfüllt. Die Modulgrenze macht das strukturell statt als Zusage. |

Vier von fünf offen. Die Fenster 80 und 81 führen sie.

## Was das für Phase 8 bedeutet

Die vier Unbekannten sind **weiterhin Unbekannte**. Phase 8 muss sie tragen
oder vorher messen — und das ist die einzige ehrliche Aussage, die diese Phase
über sich machen kann. Was sie geliefert hat, ist ein Messgerät, das auf dem
Rechner des Betreibers in einem Durchlauf vier Zahlen produziert; was sie nicht
geliefert hat, sind die Zahlen.

Konkret für die Fortsetzung: **vor** dem Bau des Provisioning-Fortschritts in
Phase 8 einmal

```
cd sandbox
go run ./cmd/talos-sandbox up --provider qemu --iso <talos.iso>
go run ./cmd/walking-skeleton --node 10.5.0.2 \
  --wrong-node 10.5.0.99 \
  --out ../.planning/phases/04-walking-skeleton/04-MEASUREMENTS.md
```

ausführen und das entstehende `04-MEASUREMENTS.md` committen. Danach sind die
Fenster 80 und 81 schließbar und Kriterium 3 hat eine Zahl.
