# Phase 3: Inventar, Cluster-Import & Health - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-09-05
**Phase:** 3-inventar-cluster-import-health
**Areas discussed:** Import-Pfad & Bundle, Node-Discovery & Identität, Ehrlichkeit & Field[T], Sperre & Warnungen, Contract-Suite & Tier 1, Zuschnitt der Oberfläche

---

## Wie diese Sitzung lief

Vier Graubereiche wurden zur Auswahl gestellt. Der Betreiber antwortete:

> „entscheide du alles selber. ich will keine fragen beantworten."

Damit lief der Rest autonom, nach den Regeln von
`workflows/discuss-phase/modes/auto.md`: **alle** Graubereiche ausgewählt, jede
Frage mit der empfohlenen Option beantwortet, keine weitere `AskUserQuestion`.
Zwei Bereiche kamen gegenüber der ursprünglichen Auswahl hinzu — *Contract-Suite
& Tier 1* und *Zuschnitt der Oberfläche* —, weil `--auto` alle Graubereiche
auswählt und nicht nur die vier präsentierten.

Nicht auto-advanced zu `/gsd-plan-phase`: `workflow.auto_advance` ist in
`config.json` `false`, und der Betreiber hat die Diskussion delegiert, nicht die
Planung mitbeauftragt.

---

## Import-Pfad & Bundle

**Q1: Was muss der Betreiber liefern, um einen bestehenden Cluster zu adoptieren?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| talosconfig + CP-Adresse; Bundle wird vom Node abgeleitet | Der Betreiber hat ein talosconfig, weil er talosctl benutzt. Die vollständige MachineConfig eines Control-Plane-Nodes enthält das CA-Material. | ✓ |
| Vollständiges `secrets.yaml` als Pflicht-Upload | Direkter, aber verlangt ein Artefakt, das der einzige Zielnutzer möglicherweise seit dem ursprünglichen `gen config` nicht mehr besitzt. | |
| talosconfig genügt, Bundle optional nachreichbar | Erzeugt einen halb adoptierten Cluster, den holzkube nach Zertifikatsablauf nie wieder betreten kann. | |

**Gewählt:** Ableitung vom Control-Plane-Node (empfohlener Default) → **D-01**
**Notiz:** Fällt die Ableitung in der Research durch, ist die Rückfallposition Option 2; D-02 bliebe unverändert gültig. Als offene Frage 1 vermerkt.

**Q2: Ist das Bundle harte Vorbedingung der Adoption?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Ja — ohne Bundle kein Import | P1/P5/P16: ohne Bundle gibt es nach Ablauf des Client-Zertifikats keinen Weg zurück in den Cluster. | ✓ |
| Nein — read-only-Adoption ohne Bundle erlauben | Würde in Phase 7/8/9 je einen zweiten, unerprobten Codepfad erzwingen. | |

**Gewählt:** harte Vorbedingung → **D-02** (`one-way`)

**Q3: Wie wird Vertrauen vor dem ersten Zugriff hergestellt?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| SHA-256-Fingerprint zeigen, Betreiber bestätigt | Gleiches Format wie `tlsx.Fingerprint` und der Startlog aus Phase 1 D-04. | ✓ |
| Trust-on-first-use ohne Anzeige | Schneller, aber ohne jeden Prüfschritt. | |

**Gewählt:** Fingerprint-Bestätigung → **D-03**

**Q4: Womit wird der Konnektivitätsbeweis geführt?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Mit einem frisch aus dem Bundle gemünzten Zertifikat | Beweist, dass holzkube sich künftig selbst Zutritt verschaffen kann. | ✓ |
| Mit dem eingelieferten talosconfig | Beweist nur, dass die gelieferte Datei heute funktioniert. | |

**Gewählt:** frisch gemünztes Zertifikat → **D-04**

**Q5: Was passiert, wenn der genannte Node ein Worker ist?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Benannte Ablehnung mit eigenem RFC-9457-Code | `.machine.ca` auf einem Worker hat keinen privaten Schlüssel. | ✓ |
| Automatisch weitere Adressen probieren | Undurchsichtig; der Betreiber erfährt nicht, was schiefging. | |

**Gewählt:** benannte Ablehnung → **D-05**

**Q6: Wie kommt das talosconfig herein?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Upload oder Einfügen — serverseitig derselbe Request-Body | Kein beliebiges Dateilesen unter der uid von holzkubed. | ✓ |
| Zusätzlich ein Pfad auf dem Server | Verstößt gegen `internal/store/store.go:5-7`. | |

**Gewählt:** Upload/Einfügen → **D-06**

**Q7: Wie hart ist die Trennung zu `gen secrets`?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Nur im Create-Pfad, durch Test gehalten, kein Fallback | Ein „generiere, falls fehlt"-Fallback erzeugt bei Import-Fehlschlag einen Cluster, der keinen seiner Nodes betreten kann. | ✓ |
| Konvention plus Code-Review | Trennung würde nur behauptet, nicht nachgewiesen. | |

**Gewählt:** Test-gehaltene Trennung → **D-07**

---

## Node-Discovery & Identität

**Q1: Wie füllt sich das Inventar nach dem Import?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Automatisch aus der Cluster-Mitgliedschaft, manuell als Zweitweg | `NewManualSource` existiert bereits als zweite Quelle. | ✓ |
| Nur manuelle IP-Eingabe | Wer nach dem Import fünf Adressen abtippt, hat nichts importiert. | |
| Subnetz-Scan | Neue Fähigkeit, gehört zu Phase 8. | |

**Gewählt:** automatische Enumeration → **D-08**, kein Scan → **D-09**

**Q2: Was gewinnt bei einem Konflikt — UUID oder Adresse?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| UUID gewinnt immer; fremde UUID erzeugt neuen Record | `Target.Addr` ist bereits als „hint, never identity" dokumentiert. | ✓ |
| Adresse gewinnt, Record wird aktualisiert | Würde bei DHCP-Rotation die Historie zweier Maschinen still vertauschen. | |

**Gewählt:** UUID gewinnt → **D-10** (`costly`)

**Q3: Wann verschwindet ein Record?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Nur auf ausdrückliche Aktion, `Destructive: true` | INV-09 wörtlich. | ✓ |
| TTL nach N Tagen Unerreichbarkeit | Löscht Daten genau dann, wenn sie gebraucht werden. | |

**Gewählt:** nur explizit → **D-11**

**Q4: Woher kommt die `schematic_id` bei importierten Nodes?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Vom Node lesen; sonst ehrlich leer | Phase 9 liefert Upgrades darauf aus; ein geratener Wert löscht Extensions mit Erfolgsmeldung. | ✓ |
| Aus der Talos-Version ableiten | Genau der P9/P10-Fehler. | |

**Gewählt:** lesen oder leer → **D-12**

---

## Ehrlichkeit & Field[T]

**Q1: Welche Response-Shape?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| `Field[T]` aus ARCHITECTURE Pattern 6, `level` als String | Löst zugleich den Parallelitäts-Constraint der ROADMAP ein. | ✓ |
| `Field[T]` mit `level` als Zahl | Macht die iota-Reihenfolge zum veröffentlichten Vertrag. | |
| Felder bei Nichtverfügbarkeit weglassen | In der UI nicht von „null" zu unterscheiden — erzeugt das leere Dashboard. | |

**Gewählt:** Pattern 6 mit String-Level → **D-13** (`one-way`)

**Q2: Wie unterscheiden sich „nie gelesen" und „bekannt, aber alt"?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Über die Kombination `available` × `stale_since`, drei sichtbare Zustände | Genau der Grund, warum beide Felder existieren. | ✓ |
| Nur `available` | Verliert den Unterschied, der im Störfall zählt. | |

**Gewählt:** drei Zustände → **D-14** (`one-way`)

**Q3: Ab wann gilt ein Wert als veraltet?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Sofort bei Verlust der Bestätigung, keine Backend-Karenzzeit | Ein Schwellwert wäre eine zweite Wahrheit neben dem Zeitstempel. | ✓ |
| Backend-Schwellwert von N Sekunden | Zwei Wahrheiten, die auseinanderlaufen. | |

**Gewählt:** keine Karenzzeit → **D-15**

**Q4: Wird der letzte Snapshot persistiert?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Ja, am Machine-Record | Ein Neustart passiert wahrscheinlich *im* Störfall; sonst leere Seite. | ✓ |
| Nein, nur im Speicher | Verletzt INV-08 beim ersten Prozess-Neustart. | |

**Gewählt:** persistieren → **D-16** (`costly`)

**Q5: Eager oder lazy beobachten?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Eager, ein Supervisor pro Node, je Level getrennt | Pattern 7: „Do not build a scheduler for this." | ✓ |
| Lazy, erst beim Öffnen einer Seite | Dashboard langsam genau dann, wenn es zählt; `stale_since` beim ersten Öffnen bedeutungslos. | |

**Gewählt:** eager → **D-17**

**Q6: SSE schon in Phase 3?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Nein — nur Snapshot-Routen, Client pollt | Zöge den dokumentierten Phase-5-Entry-Blocker vor; Wechsel bleibt später client-lokal. | ✓ |
| Ja, SSE hier bauen | Streaming-Handler puffert lautlos und stirbt nach 60 s. | |

**Gewählt:** keine SSE-Route → **D-18**

**Q7: COSI oder unary RPC für die Node-Fakten?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| COSI zuerst, unary nur wo nötig | INV-13 schließt blindes Polling aus. Zieht den synchronen Ausbau von `ClusterClient` **und** `talossim` nach sich. | ✓ |
| Unary RPCs im Intervall | Genau das Polling, das INV-13 ausschließt. | |

**Gewählt:** COSI zuerst → **D-19**

**Q8: Woher kommt die Kubernetes-Version?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Node-seitig; sonst `LevelK8s`-Feld mit `available: false` | Die Naht ist `:50000` = holzkube, `:6443` = k9s/Lens. | ✓ |
| Über einen Kubernetes-Client an `:6443` | Neue Abhängigkeit und neues Ausfallrisiko im Störfall; widerspricht PROJECT.md § Out of Scope. | |

**Gewählt:** node-seitig → **D-20** (`costly`)

---

## Sperre & Warnungen

**Q1: Ist ein importierter Cluster standardmäßig gesperrt?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Import ja, Create nein | P17: „adopt the homelab read-only first". | ✓ |
| Beide entsperrt | Erste Mutation trifft einen Cluster, von dem der Betreiber abhängt. | |
| Beide gesperrt | Ein soeben angelegter Cluster hatte noch keine Gelegenheit, wichtig zu werden. | |

**Gewählt:** Import gesperrt → **D-21**

**Q2: Wo greift die Sperre?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Serverseitig, deklarativ an der Route, wie `Destructive` | Gleiches Argument wie Phase 2 D-03 für den Dry-Run. | ✓ |
| In der Oberfläche | Eine Sperre, die nur die UI kennt, ist keine. | |

**Gewählt:** deklarativ serverseitig → **D-22** (`costly`)

**Q3: Wie laut warnt der Zertifikatsablauf?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| 90 / 30 / 7 Tage gestaffelt; abgelaufen als eigener Cluster-Zustand | Der Ausfall ist total und gleichzeitig; „abgelaufen" sieht sonst aus wie fünf tote Nodes. | ✓ |
| Nur 90/30 wie in der Research | Lässt die letzte Woche ohne Dringlichkeit. | |
| Nur ein Badge | Wird übersehen, bis es zu spät ist. | |

**Gewählt:** 90/30/7 plus eigener Zustand → **D-23**

**Q4: Woher kommt die Kompatibilitätsmatrix?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Kuratiert, im Binary eingebettet | Gleiche Bauform wie Phase 2 D-08; keine maschinenlesbare Upstream-Quelle, kein Netz im Störfall nötig. | ✓ |
| Upstream-Abruf zur Laufzeit | Nicht verfügbar, wenn sie gebraucht wird. | |
| Vom Betreiber pflegbar | Strandet operatives Wissen in einer Installation. | |

**Gewählt:** eingebettete Tabelle → **D-24**, getrennt von `MinSupportedVersion` → **D-25**

---

## Contract-Suite & Tier 1

**Q1: Wie umgehen mit dem offenen Research-Flag zum Docker-Provisioner?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Suite backend-parametrisiert schreiben, bevor das Ergebnis feststeht | Der Flag blockiert damit den Arbeitsbeginn nicht. | ✓ |
| Erst spiken, dann schreiben | Serialisiert unnötig. | |

**Gewählt:** parametrisiert → **D-26**

**Q2: Was, wenn Tier 1 auf `darwin/arm64` scheitert?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Reale Hälfte in CI auf linux/amd64; lokal sichtbar übersprungen, WINDOWS.md-Eintrag | Ein übersprungener Lauf, der sich meldet, ist ehrlicher als eine Suite ohne zweites Backend. | ✓ |
| Auf Tier 0 kollabieren | Fake-Drift trägt danach jede spätere Phase. | |

**Gewählt:** CI-Fallback → **D-27**

**Q3: Wo lebt der Provisioner-Code?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| `sandbox/` als eigenes Modul | Phase 2 D-07 an seinem ersten echten Nutzer; depguard hält die Modulgrenze. | ✓ |
| Unter `internal/` | Zieht das Talos-Root-Modul in den Graph. | |

**Gewählt:** `sandbox/` → **D-28**

---

## Zuschnitt der Oberfläche

**Q1: Was zeigt `/` gegenüber `/nodes` und `/clusters`?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| `/` = Flotte, `/nodes` = flache Tabelle, `/clusters` = Liste + Detail | Die flache Tabelle zeigt auch Nodes ohne `cluster_id` — der Grund für das flache Datenmodell. | ✓ |
| `/` bleibt Instanz-Status | Verschenkt die Fläche, auf die der Betreiber im Störfall zuerst schaut. | |

**Gewählt:** Flotten-Übersicht → **D-29**
**Notiz:** Der Instanz-Status verschwindet nicht; die Kettenbruch-Warnung aus Phase 1 D-15 bleibt erhalten.

**Q2: Node-Detail als Route oder Panel?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| Eigene Route `/nodes/$uuid` | Teilbare URL; Einhängepunkt der Phasen 5, 6 und 7. | ✓ |
| Sheet oder Drawer | Binnen drei Phasen zu klein. | |

**Gewählt:** eigene Route → **D-30** (`costly`)

**Q3: Was passiert mit der Navigation?**

| Option | Beschreibung | Gewählt |
|--------|-------------|---------|
| `phase: 3` → `null` in `NAV_AREAS`, sonst nichts | Platzhalter verschwinden automatisch, weil `placeholders.tsx` sie ableitet. | ✓ |
| Navigation umbauen | Genau das, was Phase 1 D-10 verhindern sollte. | |

**Gewählt:** ein Feld umstellen → **D-31**

---

## Claude's Discretion

**Alles.** Der Betreiber hat die gesamte Diskussion delegiert
(„entscheide du alles selber. ich will keine fragen beantworten."). Sämtliche
31 Entscheidungen in CONTEXT.md sind ohne Betreiber-Antwort getroffen, jede mit
Begründung und Reversibilitäts-Rating, damit ihnen vor der Ausführung
widersprochen werden kann.

Zusätzlich ausdrücklich **nicht** entschieden und an Researcher und Planner
weitergereicht — siehe CONTEXT.md § *Claude's Discretion* und
§ *Was der Researcher klären muss*:

- Zuschnitt der Store-Entities `Clusters()` / `Machines()`
- Die konkreten RFC-9457-Codes für Import-Fehlschlag und Cluster-Sperre
- Polling-Intervall gegen die Snapshot-Routen
- Backoff-Werte und Fehlerschwelle des Degradations-Automaten
- Route-Zuschnitt der Leseflächen
- Audit-Darstellung der Import-Ableitung samt Allowlist-Eintrag

## Deferred Ideas

- Ein-Klick-Erneuerung des Client-Zertifikats → Phase 10, sicher aufschiebbar nur wegen D-02
- CA-Rotation → nie in v1 (Research P5)
- Subnetz-Scan als `DiscoverySource` → Phase 8
- SSE für Inventar-Deltas → Phase 5, nach deren Entry-Blocker
- Node aus dem Cluster entfernen (cordon/drain → reset) → Phase 6
- Fingerprint-Pinning im Maintenance-Mode (T-02-27) → Phase 8
- `holzkubed audit verify` über den gesamten Bestand → offen aus Phase 1
- Doppelte Plan-Zählung in `ROADMAP.md` Zeile 81 → offener Aufräumpunkt aus Phase 2
