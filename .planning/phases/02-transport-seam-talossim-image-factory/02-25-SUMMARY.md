---
phase: 02-transport-seam-talossim-image-factory
plan: 25
subsystem: api
tags: [go, image-factory, talos, schematics, http-409, compare-and-swap]

# Dependency graph
requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "Plan 02-24 baute `refreshTheStoredVerdict` mit zwei Bedingungen; dieser Plan setzt die dritte daneben"
  - phase: 02-transport-seam-talossim-image-factory
    provides: "02-DECISION-probe-budget.md ratifizierte den Refresh; 02-DECISION-schematic-identity.md haelt die Identitaets-Beschraenkung, die hier auf TalosVersion ausgesprochen wird"
provides:
  - "Dritte Bedingung in `refreshTheStoredVerdict`: der Refresh wird abgelehnt, wenn `stored.TalosVersion` nicht die Version ist, die die frische Probe befragt hat"
  - "`versionMismatchReason(stored, asked string) string` — der ablehnende Satz, der beide Versionen nennt, im Register von `archMismatchReason`"
  - "Ein Fake, der zwei versions-skopierte Extension-Kataloge bedient, sodass ein Test die Talos-Version ueber einen Konflikt hinweg variieren kann"
  - "Zwei Regressionstests, einer je Richtung: `TestConflictAtAnotherTalosVersionDeclinesTheRefresh` und `TestConflictAtAnotherTalosVersionErasesNoStoredRefusal`"
  - "Drei Aussagen im Baum (model.go bei `ID`, `TalosVersion` und `Arch`; docs/api-contract.md), die statt zwei Bedingungen drei nennen"
  - "Ein angehaengter, als nachtraeglich markierter Vermerk in 02-DECISION-schematic-identity.md: die Identitaets-Beschraenkung gilt fuer `TalosVersion` woertlich wie fuer `Arch`, und ein versionsuebergreifender Refresh ist eine Schema-Aenderung"
affects: [phase-03-talos-transport, schematic-store-schema, image-factory-ui]

# Actuals (#2632)
actuals:
  tokens: 5928
  tasks: 3
  commits: 4

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Ein Guard, der ein gemessenes Verdikt nur auf den Datensatz schreibt, dessen Skopus die Messung hatte — dieselbe Form fuer Architektur und Version, damit ein Leser eine Regel zweimal gesagt bekommt statt zwei Regeln"
    - "Byte-Identitaet ueber den ganzen marshallierten Datensatz (`marshalRecord`) statt ueber eine Feldliste, wenn genau die sonst ausgenommenen Probe-Felder die sind, die sich nicht bewegen duerfen"
    - "Ein Fake, der eine zuvor gesetzte Ablehnung wieder aufheben kann (`buildFromNowOn`), um einen versionsabhaengigen Unterschied durch Umschalten zwischen zwei Requests zu erzeugen — weil der Produktionscode die Entscheidung ebenfalls nicht selbst faellt"

key-files:
  created: []
  modified:
    - internal/httpapi/handlers/schematics.go
    - internal/httpapi/handlers/schematics_test.go
    - internal/model/model.go
    - docs/api-contract.md
    - .planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-schematic-identity.md

key-decisions:
  - "Kein Sonderfall fuer eine leere gespeicherte `TalosVersion`. Anders als `Arch`, das additiv nachgeruestet wurde, ist `TalosVersion` seit dem ersten Schematic-Datensatz Pflichtfeld: eine leere gespeicherte Version ist kein Alt-Datensatz, sondern ein Datensatz, dessen Version nicht bekannt ist, und die Ungleichheitspruefung lehnt ihn korrekt ab."
  - "Der ablehnende Satz bekommt eine eigene Funktion `versionMismatchReason` neben `archMismatchReason` statt eines Inline-Strings, damit die aufrufende Zeile dieselbe Gestalt hat wie die der Architektur-Bedingung."
  - "Kein versionsuebergreifender Refresh. Er waere eine Schema-Aenderung — `model.Schematic` hat Platz fuer genau ein Verdikt — und ist im angehaengten Vermerk von 02-DECISION-schematic-identity.md unter dieselbe Aufschiebung wie Option B gestellt, nicht in einem Guard entschieden."
  - "`.planning/WINDOWS.md` bleibt von diesem Plan unberuehrt; die Ledger-Eintraege dieser Runde werden unten fertig formuliert an Plan 02-26 uebergeben."

patterns-established:
  - "Dritte Bedingung in der Form der zweiten: `if <ungleich> { return \" The stored verdict was not refreshed, because \" + <reason>(...) + \".\" }`, mit einem Kommentar, der die Evidenz benennt statt auf den Kommentar darueber zu verweisen"
  - "Cross-Version-Tests im Fake: zwei bediente Katalog-Versionen, jede weitere weiterhin 404 — der Fake bleibt so streng wie die echte Factory"

requirements-completed: [FACT-02]

coverage:
  - id: D1
    description: "Ein zweiter POST derselben Customisation an einer anderen `talos_version` beantwortet 409 und laesst den gespeicherten Datensatz byte-identisch, einschliesslich `usable`, `probed_at` und `probe_reason`"
    requirement: "FACT-02"
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestConflictAtAnotherTalosVersionDeclinesTheRefresh"
        status: pass
    human_judgment: false
  - id: D2
    description: "Die Reproduktion aus 02-VERIFICATION.md Runde 4 laeuft als Test: eine wahre v1.12.0-Ablehnung ueberlebt eine erfolgreiche v1.13.9-Probe, `usable` bleibt false und die `probe_reason` wird nicht geleert"
    requirement: "FACT-02"
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestConflictAtAnotherTalosVersionErasesNoStoredRefusal"
        status: pass
    human_judgment: false
  - id: D3
    description: "Der 409-`detail` nennt beide Versionen und ist von der Architektur-Ablehnung unterscheidbar"
    requirement: "FACT-02"
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestConflictAtAnotherTalosVersionDeclinesTheRefresh (conflictDetailOf enthaelt catalogVersion und otherCatalogVersion)"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestConflictAtAnotherTalosVersionErasesNoStoredRefusal (dieselbe Zusicherung in der anderen Richtung)"
        status: pass
    human_judgment: false
  - id: D4
    description: "Der Refresh bleibt fuer den Fall erhalten, fuer den er gebaut wurde: ein zweiter POST an derselben Version und derselben Architektur schreibt die drei Probe-Felder weiterhin"
    requirement: "FACT-02"
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestConflictRefreshesTheVerdictItJustComputed (unveraendert)"
        status: pass
      - kind: other
        ref: "Bedingung testweise entfernt (`if false && ...`), volles Paket gelaufen: genau ein Test faellt"
        status: pass
    human_judgment: false
  - id: D5
    description: "Der Fake bedient zwei versions-skopierte Kataloge und bleibt fuer jede dritte Version bei 404"
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestFactoryExtensionsForAnUnknownVersionIsUpstream (?version=v1.11.0 -> 502 CodeUpstreamFactoryUnavailable)"
        status: pass
    human_judgment: false
  - id: D6
    description: "`internal/model/model.go` (bei `ID`, `TalosVersion` und `Arch`) und `docs/api-contract.md` nennen drei Bedingungen statt zwei und sagen jeweils auch, was weiterhin gilt"
    verification:
      - kind: other
        ref: "test \"$(sed -n '/^| Condition | Meaning |/,/^$/p' docs/api-contract.md | grep -c '^|')\" -eq 5"
        status: pass
      - kind: other
        ref: "grep -cF 'when **both** conditions hold' docs/api-contract.md == 0 && grep -cF 'conditions hold' docs/api-contract.md == 1"
        status: pass
      - kind: other
        ref: "grep -c 'TalosVersion' internal/model/model.go == 6 (>= 4)"
        status: pass
    human_judgment: true
    rationale: "Die Zaehl- und Wortlaut-Pruefungen beweisen, dass die dritte Bedingung dazugekommen und die Zweizahl ersetzt ist. Ob jeder der drei Absaetze auch wirklich das weiterhin Geltende sagt und nicht nur das Geaenderte, ist eine Leseurteilsfrage, die kein grep entscheidet."
  - id: D7
    description: "`02-DECISION-schematic-identity.md` traegt einen rein angehaengten, als nachtraeglich markierten Vermerk; kein Byte des entschiedenen Textes wurde bewegt"
    verification:
      - kind: other
        ref: "head -c 9137 <pfad> | shasum -a 256 == ed2d08742734c2e46977ad4ac3bc3828a7206a7be27cdadb59fda41b26ee77c9"
        status: pass
      - kind: other
        ref: "wc -c < <pfad> == 11854 (> 9137)"
        status: pass
    human_judgment: false

# Metrics
duration: 10 min
completed: 2026-09-04
status: complete
---

# Phase 02 Plan 25: Der Refresh lehnt quer ueber Talos-Versionen ab Summary

**Dritte Bedingung in `refreshTheStoredVerdict` — ein bei einer Talos-Version gemessenes Verdikt wird nicht mehr auf einen Datensatz geschrieben, der eine andere nennt, und die wahre Ablehnungs-Begruendung wird nicht mehr geloescht.**

## Performance

- **Duration:** 10 min
- **Started:** 2026-09-04T05:35:07Z
- **Completed:** 2026-09-04T05:44:52Z
- **Tasks:** 3
- **Files modified:** 5

## Accomplishments

- **Die Luecke ist geschlossen.** `refreshTheStoredVerdict` prueft jetzt drei Bedingungen statt zwei: die Probe hat geantwortet, die Architektur stimmt, und die gespeicherte `TalosVersion` ist die, die die frische Probe befragt hat. Die dritte tritt neben die zweite und uebernimmt deren Form — eine `if`-Pruefung, ein Kommentar mit der Evidenz, ein Rueckgabesatz, der mit `" The stored verdict was not refreshed, because "` beginnt.
- **`versionMismatchReason(stored, asked string) string`** liegt unmittelbar hinter `archMismatchReason` und nennt beide Versionen, damit ein Betreiber die Versions-Ablehnung von der Architektur-Ablehnung unterscheiden kann (T-02-116).
- **Der Fake bedient zwei versions-skopierte Kataloge.** `otherCatalogVersion = "v1.12.0"` neben `catalogVersion = "v1.13.9"`; jede dritte Version bleibt 404. Das ist die eine Zeile, deren Fehlen der Grund war, dass diese Regression ausgeliefert wurde: `Author` holt den versions-skopierten Katalog, bevor irgendetwas gepostet wird, also konnte kein Test die Version ueber einen Konflikt hinweg ueberhaupt variieren.
- **Zwei Regressionstests, einer je Richtung.** Der eine sichert Byte-Identitaet nach einem POST an einer anderen Version zu; der andere kodiert die Reproduktion des Verifiers und beweist, dass eine wahre Ablehnung eine erfolgreiche Probe an einer anderen Version ueberlebt.
- **Drei Aussagen im Baum nennen drei Bedingungen** und sagen jeweils auch, was weiterhin gilt: `model.go` bei `ID`, bei `TalosVersion` und bei `Arch`, und `docs/api-contract.md` unter `POST /api/v1/schematics`.
- **Der Vermerk im Entscheidungsdokument** spricht die Identitaets-Beschraenkung fuer `TalosVersion` aus und stellt einen versionsuebergreifenden Refresh als Schema-Aenderung unter dieselbe Aufschiebung wie Option B — angehaengt, ohne ein Byte des entschiedenen Textes zu beruehren.

## Task Commits

1. **Task 1 (RED): Fake mit zweitem Katalog + fehlschlagender Test** — `45420ee` (test)
2. **Task 1 (GREEN): dritte Bedingung + `versionMismatchReason`** — `d4ee1f5` (feat)
3. **Task 2: `buildFromNowOn` + die Loesch-Richtung als Test** — `846d540` (test)
4. **Task 3: drei Aussagen und der angehaengte Vermerk** — `061b906` (docs)

**Plan metadata:** siehe letzten `docs(02-25)`-Commit dieses Plans.

## Files Created/Modified

- `internal/httpapi/handlers/schematics.go` — dritte Bedingung `if stored.TalosVersion != fresh.TalosVersion` vor den drei Zuweisungen; `versionMismatchReason` hinter `archMismatchReason`.
- `internal/httpapi/handlers/schematics_test.go` — `otherCatalogVersion`, die geweitete Katalog-Bedingung im `extensions/official`-Zweig, `func (f *fakeFactory) buildFromNowOn(name string)`, `TestConflictAtAnotherTalosVersionDeclinesTheRefresh`, `TestConflictAtAnotherTalosVersionErasesNoStoredRefusal`.
- `internal/model/model.go` — die Doc-Kommentare bei `ID`, `TalosVersion` und `Arch` auf drei Bedingungen verengt.
- `docs/api-contract.md` — dritte Zeile in der Bedingungstabelle, dritte Ursache in der Ergebnistabelle, ein Satz im Architektur-Absatz, Zweizahl durch Dreizahl ersetzt.
- `.planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-schematic-identity.md` — angehaengter Vermerk `## Nachtrag von Plan 02-25 (2026-09-04)`.

## Decisions Made

- **Kein Sonderfall fuer eine leere gespeicherte `TalosVersion`.** `Arch` wurde additiv nachgeruestet und hat deshalb in `archMismatchReason` einen Zweig fuer den Alt-Datensatz. `TalosVersion` ist seit dem ersten Schematic-Datensatz Pflichtfeld, also ist eine leere gespeicherte Version kein Alt-Datensatz, sondern ein Datensatz, dessen Version nicht bekannt ist — die Ungleichheitspruefung lehnt ihn korrekt ab. Der Grund steht als Halbsatz im Kommentar, damit die naechste Leserin nicht nach der fehlenden Sonderbehandlung sucht.
- **Der Rueckgabesatz bekommt eine eigene Funktion.** `versionMismatchReason` statt eines Inline-Strings, damit die aufrufende Zeile dieselbe Gestalt hat wie die der Architektur-Bedingung und ein Leser eine Regel zweimal gesagt bekommt statt zwei Regeln.
- **Kein versionsuebergreifender Refresh.** `model.Schematic` hat Platz fuer genau ein Verdikt. Ein Datensatz, der die Verdikte zweier Versionen halten soll, braucht dieselbe Schema-Aenderung, die Option B fuer die Architektur beschreibt — argumentiert im angehaengten Vermerk, nicht in einem Guard.
- **Der Ledger dieser Runde wird nicht von diesem Plan gefuehrt.** `.planning/WINDOWS.md` ist unberuehrt; die Eintraege gehen unten fertig formuliert an Plan 02-26.

## Der rote Lauf, aufgezeichnet

**Task 1 — `TestConflictAtAnotherTalosVersionDeclinesTheRefresh`, vor der Handler-Aenderung:**

```
--- FAIL: TestConflictAtAnotherTalosVersionDeclinesTheRefresh (1.97s)
 before {... "probe_reason":"", "probed_at":"0001-01-01T00:00:00Z", "rev":1,
         "talos_version":"v1.13.9", "usable":false}
 after  {... "probe_reason":"", "probed_at":"2026-09-04T05:36:06.922669Z", "rev":2,
         "talos_version":"v1.13.9", "usable":true}
 the 409 does not name both Talos versions ...
```

Die Felder, die sich ueber den Konflikt hinweg bewegt hatten: **`usable`** (false -> true) und **`probed_at`** (Nullzeit -> gestempelt), dazu `rev` 1 -> 2 als die Buchhaltung des Schreibvorgangs. `probe_reason` blieb in dieser Richtung leer, weil die frische Probe erfolgreich war.

**Die Gegenprobe (Abnahmekriterium Task 1).** Die dritte Bedingung wurde testweise mit `if false && ...` ausgeschaltet und das ganze Paket gefahren:

```
--- FAIL: TestConflictAtAnotherTalosVersionDeclinesTheRefresh (0.97s)
FAIL	github.com/holzcloud/holzkube-manager/internal/httpapi/handlers	63.142s
```

Genau ein Test faellt und kein anderer — die Bedingung ist nicht zu weit geraten. Der Baum wurde danach byte-identisch wiederhergestellt (`git diff` leer gegen den Stand vor der Gegenprobe).

## Die gemessenen Endzustaende neben denen von 02-VERIFICATION.md

Task 2 wurde zur Kontrolle ebenfalls gegen den ungeschuetzten Handler gefahren. Die Zahlen sind dieselben, die `02-VERIFICATION.md` Runde 4 fuer den unkorrigierten Baum aufgezeichnet hat:

| | 02-VERIFICATION.md, `adversarial_checks_run[0]` | dieser Lauf, Bedingung ausgeschaltet | dieser Lauf, Bedingung aktiv |
|---|---|---|---|
| vor dem zweiten POST | `talos_version v1.12.0, usable=false, probe_reason "... at v1.12.0/amd64 answered HTTP 400"` | `talos_version v1.12.0, usable=false, probe_reason "38f8ee13… at v1.12.0/amd64 answered HTTP 400"`, rev 1 | identisch |
| nach dem zweiten POST bei v1.13.9 | `talos_version: v1.12.0, usable: true, probe_reason: ""`, rev 1 -> 2 | `talos_version: v1.12.0, usable: true, probe_reason: ""`, rev 2 | **unveraendert**: `usable: false`, `probe_reason` erhalten, rev 1 |

Die Reproduktion des Verifiers ist damit als Test kodiert und gruen. Der Schreibvorgang, der die wahre v1.12.0-Ablehnung geloescht hat — die einzige sichtbare Stelle der Meinungsverschiedenheit —, findet nicht mehr statt.

## Deviations from Plan

None - plan executed exactly as written.

**Total deviations:** 0
**Impact on plan:** keiner. Alle drei Tasks liefen wie geschrieben, einschliesslich der Praeconditions-Pruefung in Task 3 (`head -c 9137 | shasum -a 256` = `ed2d0874…`, unveraendert seit der Planung).

## Issues Encountered

None.

## Ledger entries to file

Uebergabe an Plan 02-26, das `.planning/WINDOWS.md` in Welle 2 fuehrt. Dieser Plan hat die Datei nicht angefasst. Drei Eintraege, jeder mit Zieldatei und fertigem Wortlaut:

**1. Ergaenzung zu WINDOWS-Eintrag 58** — Zieldatei `.planning/WINDOWS.md`, Kind `deviation`, Phase 02.

> Eintrag 58 zaehlt die ablehnenden Faelle von `refreshTheStoredVerdict` auf und nennt zwei: die Probe hat nicht geantwortet, und die Architektur stimmt nicht. Seit Plan 02-25 sind es drei — die dritte ist die Talos-Version (`internal/httpapi/handlers/schematics.go`, `if stored.TalosVersion != fresh.TalosVersion`). Was 58 weiterhin korrekt aufzeichnet, bleibt korrekt: **G-02-9 ist offen**, es gibt weiterhin keine Re-Probe-Route, keinen Button und keinen Job, und die Erholung ist eine Wieder-Einreichung, die als Fehler beantwortet wird. Ueberholt ist allein die Aufzaehlung der Bedingungen.

**2. Die Regression selbst** — Zieldatei `.planning/WINDOWS.md`, Kind `deviation`, Phase 02, Bezug `internal/httpapi/handlers/schematics.go`.

> Plan 02-24 hat den von `02-DECISION-probe-budget.md` ratifizierten Refresh mit zwei Bedingungen ausgeliefert. Die dritte fehlte, und keine Zeile in `.planning/WINDOWS.md` hat sie bis Runde 4 getragen. Runde 4 der Verifikation hat sie reproduziert statt sie zu erschliessen (`02-VERIFICATION.md`, `adversarial_checks_run[0]`): `siderolabs/cross-version-ext` bei `v1.12.0` authored, wo der Image-Endpunkt ihn ablehnt — Datensatz `talos_version v1.12.0, usable=false, probe_reason "... at v1.12.0/amd64 answered HTTP 400"` —, dann die Ablehnung aufgehoben und dieselbe Customisation bei `v1.13.9` erneut gepostet. Ergebnis: dieselbe Id, HTTP 409, und der gespeicherte Datensatz wurde zu `talos_version: v1.12.0, usable: true, probe_reason: ""`, rev 1 -> 2. Die wahre v1.12.0-Ablehnung wurde **geloescht**. Ursache: `Schematic.Canonical()` emittiert weder Architektur noch Version, also sind zwei Versuche, die sich nur in `talos_version` unterscheiden, ein Datensatz. Geschlossen von Plan 02-25 (Commits `45420ee`, `d4ee1f5`, `846d540`, `061b906`); die Reproduktion laeuft seither als `TestConflictAtAnotherTalosVersionErasesNoStoredRefusal`. Status: `fixed`.

**3. Verbleibende Ungenauigkeit** — Zieldatei `.planning/WINDOWS.md`, Kind `deviation`, Phase 02.

> **Task 2 hat keine verbleibende Ungenauigkeit gefunden.** Ausdrueckliche Feststellung, kein Ausbleiben einer Pruefung: die Vorpruefung des Tests auf eine echte gespeicherte Ablehnung (`usable: false` und `probe_reason` nennt `otherCatalogVersion`) laeuft und bricht ab, wenn sie nicht zutrifft; die Byte-Identitaet wird mit `marshalRecord` ueber den ganzen Datensatz geprueft und nicht mit `exceptTheProbeFields`; und `web/src/api.ts:203-209` und `:216-219` sowie das Badge `UsabilityVerdict` (`web/src/routes/images.tsx:942-944`) wurden geprueft und sind nach dieser Korrektur weiterhin wahr — `api.ts` nennt die Bedingungen des Refresh gar nicht, sondern verweist auf `docs/api-contract.md`, und das Badge sagt von selbst die Wahrheit, weil der Server keine Unwahrheit mehr speichert. Keine Datei unter `web/` wurde beruehrt.

## Known Stubs

Keine. Der Plan hat keine Platzhalter, keine leeren Rueckgaben und keine uebersprungenen Tests hinterlassen.

## Threat Flags

Keine neue Angriffsflaeche ausserhalb des `<threat_model>` des Plans. Es kommt kein Endpunkt, kein Feld, kein Probe-Zustand und kein Statuscode hinzu; die einzige Verhaltensaenderung ist ein Schreibvorgang, der in einem Fall **weniger** eintritt als bisher. T-02-114 bis T-02-117 sind mit den im Plan benannten Minderungen belegt (siehe `coverage` D1–D4). Die naechste freie Threat-Id bleibt **T-02-118** fuer Plan 02-26.

## Was ausdruecklich NICHT geschehen ist

- **G-02-9 bleibt offen.** Dieser Plan haertet die Minderung, die 02-24 dafuer gebaut hat; er macht kein Verdikt erreichbar. Die Buchhaltung in `gaps_mitigated_not_closed` von 02-24 und WINDOWS-Eintrag 58 bleiben, wie sie sind.
- **Kein versionsuebergreifender Refresh gebaut.** Er ist eine Schema-Aenderung und im angehaengten Vermerk argumentiert, nicht in einem Guard.
- **Option 1 aus `02-DECISION-probe-budget.md` wurde nicht nachtraeglich genommen.**
- **`.planning/WINDOWS.md` unberuehrt**, `web/src/api.ts` und `web/src/routes/images.tsx` unberuehrt, `COVERAGE.md` unberuehrt — die Oberflaeche der Image Factory aendert sich nicht.
- **Der entschiedene Text von `02-DECISION-schematic-identity.md` unberuehrt.** Die ersten 9137 Bytes hashen weiterhin auf `ed2d08742734c2e46977ad4ac3bc3828a7206a7be27cdadb59fda41b26ee77c9`; die Datei ist auf 11854 Bytes gewachsen.

## Verification

| Pruefung | Ergebnis |
|---|---|
| `go build ./... && go vet ./...` | Exit 0 |
| `gofmt -l ./internal ./cmd` | leer |
| `golangci-lint run` | `0 issues.` |
| `go test ./... -count=1 -race` | gruen ueber alle Pakete |
| `go test ./internal/httpapi/handlers ./internal/store/... -count=1 -race` | gruen (76.4s / 1.4s / 4.6s / 2.2s) |
| `TestConflictAtAnotherTalosVersionDeclinesTheRefresh` einzeln, `-run '^…$' -v` | `--- PASS:` |
| `TestConflictAtAnotherTalosVersionErasesNoStoredRefusal` einzeln | `--- PASS:` |
| `TestConflictRefreshesTheVerdictItJustComputed` einzeln, Test unveraendert | `--- PASS:` |
| `TestConflictAtAnotherArchitectureDeclinesTheRefresh` einzeln, Test unveraendert | `--- PASS:` |
| `TestFactoryExtensionsForAnUnknownVersionIsUpstream` einzeln (`?version=v1.11.0` -> 502) | `--- PASS:` |
| `grep -c 'otherCatalogVersion' …_test.go` | 7 (>= 4) |
| `grep -n 'func versionMismatchReason' schematics.go` | eine Zeile, `:630`, hinter `archMismatchReason` (`:617`) |
| `grep -c 'TalosVersion' internal/model/model.go` | 6 (>= 4) |
| Bedingungstabelle in `docs/api-contract.md` | 5 `^|`-Zeilen (Kopf, Trenner, drei Bedingungen) |
| `grep -cF 'when **both** conditions hold'` / `grep -cF 'conditions hold'` | 0 / 1 |
| `head -c 9137 02-DECISION-schematic-identity.md \| shasum -a 256` | `ed2d0874…` |
| `wc -c < 02-DECISION-schematic-identity.md` | 11854 (> 9137) |
| `.planning/WINDOWS.md` in `git diff --name-only` | nicht enthalten |

Die einzeln gefahrenen Testfaelle wurden auf die `--- PASS:`-Zeile geprueft und nicht auf den Exit-Code, weil ein `-run`-Filter, der nichts trifft, in Go mit 0 endet.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Die Luecke G-02-24 (== `02-VERIFICATION.md` Runde 4, `gaps[0]`, rundenlokal G4-1 == `02-REVIEW.md` CR-01) ist geschlossen: die Reproduktion ist als Test kodiert und gruen, und der Datensatz haelt seither nur noch das, was die Probe an seiner eigenen Version gemessen hat.
- **Uebergabe an Plan 02-26 (Welle 2):** die drei Ledger-Eintraege oben unter `## Ledger entries to file` sind fertig formuliert und warten auf `gsd-tools windows append`. Naechste freie Threat-Id: `T-02-118`.
- Offen bleiben, unveraendert: **G-02-9** (eine Probe, die trotz erhoehtem Budget ausbleibt, ist weiterhin dauerhaft — es gibt keine Re-Probe-Route) und die zweite Luecke aus Runde 4, `browserRefusalRange`s Verankerung in `internal/imagefactory/guard_drift_test.go:49-50`, die dieser Plan nicht beruehrt.

## Self-Check: PASSED

- `internal/httpapi/handlers/schematics.go` — FOUND, enthaelt `versionMismatchReason` (`:630`) und die dritte Bedingung (`:579`)
- `internal/httpapi/handlers/schematics_test.go` — FOUND, enthaelt beide neuen Tests, `otherCatalogVersion` und `buildFromNowOn`
- `internal/model/model.go` — FOUND, `grep -c 'TalosVersion'` == 6
- `docs/api-contract.md` — FOUND, Bedingungstabelle mit drei Datenzeilen
- `.planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-schematic-identity.md` — FOUND, 11854 Bytes, erste 9137 hashen auf `ed2d0874…`
- Commits `45420ee`, `d4ee1f5`, `846d540`, `061b906` — alle in `git log --oneline --all` vorhanden

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-09-04*
