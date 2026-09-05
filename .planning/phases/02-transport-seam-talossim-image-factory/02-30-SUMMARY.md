---
phase: 02-transport-seam-talossim-image-factory
plan: 30
subsystem: testing
tags: [drift-guard, gap-closure, honest-claim, blind-spots, regexp, go]

# Dependency graph
requires:
  - phase: 02-transport-seam-talossim-image-factory (Plan 02-29)
    provides: "die Rumpfform-Unterscheidung, die getrennten Bound-Zweige, die Duplikatspruefung und die drei unterscheidenden `wantErr`-Teilzeichenketten — alles davon musste das Ersetzen des allquantifizierten Satzes ueberleben, und alles davon hat es"
  - phase: 02-transport-seam-talossim-image-factory (Plan 02-14)
    provides: "die sechs gemessenen Ablehnungsklassen des Servers, gegen die `TestBrowserRefusalSetEqualsTheServers` vergleicht und die dieser Plan auf keiner Seite anfasst"
provides:
  - "Der allquantifizierte Satz `Renamed, moved or deleted is the same as never having been there` ist aus Doc-Kommentar UND Fehlertext verschwunden; `grep -c 'Renamed, moved or deleted'` liefert 0"
  - "`guardBlindSpots` fuehrt die Ausschlussliste als DATEN mit vier Eintraegen, nach MECHANISMUS skopiert: `text-not-code`, `literal-not-value`, `declaration-not-use`, `surrogate-interior` (markiert)"
  - "`honestClaim` rendert die verengte Behauptung aus dieser Liste und haengt sie an genau einen Fehlerausgang — den Kein-Anker-Ausgang"
  - "`browserRefusalRanges` nimmt den Pfad als Parameter; es gibt keinen Wrapper mehr, in dem eine kuenftige Vorverarbeitung an den Blindheitstabellen vorbei sitzen koennte"
  - "Zwei Blindheitstabellen mit UMGEKEHRTER ABNAHME ueber den Live-Lesepfad, sieben Zeilen, jede einzeln gemessen"
  - "`TestGuardBlindSpotsAreEachMeasured` bindet Liste und Zeilen in beide Richtungen und erzwingt genau einen markierten Eintrag"
  - "Punkt 6 der Schliessbedingung von Ledger-Eintrag 72 ist einmal ausgefuehrt, im Code protokolliert und ohne Fund geblieben"
affects: [plan-02-31, verification-runde-7, ship-gate]

# Actuals (#2632) — estimateTokens-Skala (chars/4 ueber den realisierten Diff), keine Harness-Zahl.
actuals:
  tokens: 7740
  tasks: 3
  commits: 4

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Eine Behauptung wird aus der Liste ihrer Ausnahmen GERENDERT und nicht daneben geschrieben: zwei Listen verschiedener Laenge waeren selbst das Drift-Risiko, das die Datei bewacht"
    - "Blindheit wird nach MECHANISMUS gefuehrt und nicht nach FORM. Eine Formenliste war nach drei Runden wieder unvollstaendig; ein Mechanismus deckt seine Auspraegungen mit ab, und der Abdeckungstest verlangt je Mechanismus mindestens eine gemessene Auspraegung"
    - "Umgekehrte Abnahme mit Anti-Rot-Anweisung IM CODE: eine Tabelle, die verlangt, dass der Waechter die Leiche liest, geht rot an dem Tag, an dem die Blindheit endet — und ueber ihr steht, dass die richtige Antwort das Loeschen der Zeile ist und niemals das Aufweichen des Waechters"
    - "Eine Tabelle mit umgekehrter Abnahme braucht ihren eigenen Beleg: `err == nil` ist auch dann gruen, wenn nichts gemessen wird. Deshalb einmal die Fixture zerstoert und einmal eine Vorverarbeitung in den Lesepfad gesetzt, beide Rot-Ausgaben zitiert"
    - "Der Lesepfad ist parametrisiert und wrapper-frei, damit eine kuenftige Vorverarbeitung IN den Tabellen sichtbar wird statt an ihnen vorbei"

key-files:
  created: []
  modified:
    - "internal/imagefactory/guard_drift_test.go — `guardBlindSpot`/`guardBlindSpots`/`honestClaim`, `blindnessRow`/`runBlindnessRows`/`allBlindnessRows`, sieben synthetische Fixtures, zwei Blindheitstabellen, der Abdeckungstest, der parametrisierte Lesepfad und die Entfernung des allquantifizierten Satzes an beiden Stellen (+568/-15)"

key-decisions:
  - "Die Richtungsentscheidung wurde vom Betreiber an den autonomen Lauf DELEGIERT, nicht ratifiziert. Siehe `## Die Provenienz der Richtungsentscheidung` — der Unterschied ist der Gegenstand dieser Phase und keine Formalie"
  - "Die Fixture `a declaration surviving only as JSX text in a pre block` traegt ihre Tabelle in einem Template-Literal INNERHALB des `<pre>`-Blocks und nicht als rohen JSX-Textknoten. In TSX oeffnet ein unmaskiertes `{` in JSX-Kindern einen Ausdrucks-Container, also waere eine Tabelle aus `{ from: … }`-Eintraegen als reiner Textknoten ein Syntaxfehler und kein Schadensfall — und ein Abnahmekriterium desselben Plans verbietet Fixtures, die blosse TypeScript-Fehler sind. Der gemessene Mechanismus ist unveraendert; die Korrektur steht als Kommentar an der Fixture"
  - "`honestClaim` wird an GENAU EINEN Fehlerausgang gehaengt, den Kein-Anker-Ausgang. Er ist der Ausgang, an dem der Waechter sagt, dass er nichts gefunden hat, und damit der einzige Ort, an dem ein Leser wissen muss, was Nichtfinden hier bedeutet und was nicht"
  - "`PLATFORM_RANGES` in der Spread-Fixture ist aus einem Aufruf abgeleitet und nicht als Literale geschrieben. Eintragsfoermige Literale ausserhalb der Deklaration sind ein Fall, den der Waechter bereits meldet; diese Zeile handelt von dem Fall, den er NICHT meldet"
  - "Das Protokoll des Punkt-6-Versuchs steht zusaetzlich im Doc-Kommentar von `guardBlindSpots` und nicht nur in dieser SUMMARY. Ein Versuch, der nur in einem Planungsdokument steht, ist fuer den naechsten Ausfuehrenden von einem unterlassenen nicht zu unterscheiden"

patterns-established:
  - "Sieben Zeilen, sieben zitierte Messungen; drei zitierte Rot-Ausgaben fuer die drei Eigenschaften, die eine umgekehrte Abnahme nicht von selbst hat"
  - "Der Punkt-6-Versuch wird ausgefuehrt UND protokolliert, auch wenn er nichts findet — und sein Nicht-Fund schliesst ausdruecklich nichts"

requirements-completed: []
# ABSICHTLICH LEER, aus demselben Grund wie in 02-29-SUMMARY.md. Die Phase steht auf
# `gaps_found`; ein vorzeitig gefuelltes `requirements-completed` aus zwei SUMMARYs der Runde 6
# hat die Buchhaltungsregression ausgeloest, die der Betreiber mit `b7a1ab9` zuruecknehmen musste.
# FACT-01 und FACT-06 stehen im `requirements`-Feld des Plans, weil dieser Plan an ihrer Naht
# arbeitet — nicht, weil er sie aufloest. `.planning/REQUIREMENTS.md` wurde von diesem Plan nicht
# geschrieben; die Requirement-Korrektur gehoert Plan 02-31.

coverage:
  - id: D1
    description: "Der allquantifizierte Satz ist aus Doc-Kommentar und Fehlertext verschwunden; an seiner Stelle steht eine Aussage ueber den TEXT plus eine gerenderte Ausschlussliste"
    verification:
      - kind: other
        ref: "grep -c 'Renamed, moved or deleted' internal/imagefactory/guard_drift_test.go -> 0"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration (10/10 Untertests)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Die Ausschlussliste ist Daten, nach Mechanismus skopiert, und Liste und Zeilen sind in beide Richtungen aneinander gebunden"
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestGuardBlindSpotsAreEachMeasured"
        status: pass
    human_judgment: false
  - id: D3
    description: "Sieben Blindheitszeilen messen drei Mechanismen einzeln, ueber denselben Lesepfad wie der Live-Waechter"
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardBindsToAnIdentifierAndNotToCode (4 Untertests)"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardBindsToALiteralAndNotToTheValueTheFormUses (3 Untertests)"
        status: pass
    human_judgment: false
  - id: D4
    description: "Die Ablehnungsmenge ist auf keiner Seite angefasst; der Waechter misst weiterhin dasselbe"
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalSetEqualsTheServers"
        status: pass
      - kind: other
        ref: "git diff --quiet HEAD -- web/"
        status: pass
    human_judgment: false
  - id: D5
    description: "Punkt 6 der Schliessbedingung von Ledger-Eintrag 72 wurde einmal ausgefuehrt: sechs Formen gegen den ausgelieferten Waechter, jede Ausgabe protokolliert, kein Fund"
    verification:
      - kind: manual_procedural
        ref: "temporaere Sonde internal/imagefactory/zz_probe_test.go, gefahren und rueckstandslos entfernt; Ausgaben in ## Punkt 6"
        status: pass
    human_judgment: true
    rationale: "Ob der Versuch ERNSTHAFT war — ob die sechs Formen die richtigen waren und nicht nur die bequemen — ist genau die Frage, die kein Test stellen kann. Das Entscheidungsdokument sagt selbst: ein Eintrag, dessen Bedingung nur die schon bekannten Formen abfragt, schliesst sich selbst. Diese Beurteilung gehoert einem Menschen"
  - id: D6
    description: "Die Richtung dieser Runde (Variante B statt A) und die Provenienz dieser Wahl"
    verification: []
    human_judgment: true
    rationale: "Die Wahl wurde vom Betreiber an den autonomen Lauf delegiert und von diesem getroffen. Der Betreiber hat B NICHT ratifiziert. Ob er die Wahl im Nachhinein traegt, ist eine Frage an ihn und an keinen Test"

# Metrics
duration: 19 min
completed: 2026-09-05
status: complete
---

# Phase 02 Plan 30: Der verengte Anspruch des Browser-Ablehnungs-Waechters — Summary

**Der allquantifizierte Satz ist weg; an seiner Stelle stehen vier nach Mechanismus skopierte
Blindheits-Eintraege als Daten, aus denen die Behauptung des Waechters gerendert wird, sieben
einzeln gemessene Tabellenzeilen mit umgekehrter Abnahme ueber den Live-Lesepfad und ein
Abdeckungstest, der Liste und Zeilen in beide Richtungen aneinander bindet.**

## Performance

- **Duration:** 19 min
- **Started:** 2026-09-05T08:44:08Z
- **Completed:** 2026-09-05T09:03:16Z
- **Tasks:** 3 (plus der vorab aufgeloeste Entscheidungs-Checkpoint)
- **Files modified:** 1

## Die Provenienz der Richtungsentscheidung

**Dieser Abschnitt steht vor allem anderen, weil er die Sache dieser Phase ist.**

Die Wahl zwischen B und A wurde vom Betreiber am 2026-09-05 ausdruecklich an den autonomen Lauf
delegiert; der Lauf hat B gewaehlt. Der Betreiber hat B nicht selbst ratifiziert.

Konkret: der Betreiber hat den Checkpoint gelesen und mit *"entscheide du"* geantwortet. Der
autonome Lauf hat daraufhin B gewaehlt, auf diesen Gruenden: es ist die Empfehlung des Panels; die
Plaene 02-30 und 02-31 sind bereits fuer B geschrieben, waehrend A eine Neuplanung verlangte; und
zwei Pruef-Linsen haben Variante A's Lexer gebaut und besiegt — eine Tabelle als JSX-Textknoten in
einem `<pre>`-Block, und der in `02-REVIEW.md` entworfene `stripNonCode` gegen eine
`${…}`-Interpolation.

**Nirgends in diesem Bestand darf stehen, der Betreiber habe B ratifiziert.** Der
Checkpoint-Text von Plan 02-30 sieht eine Ratifikation vor und beauftragt Plan 02-31, sie in den
Statusblock von `02-DECISION-drift-guard-lexik.md` und in den Text von Ledger-Eintrag 72 zu tragen.
**Das ist nicht der eingetretene Fall.** Plan 02-31 traegt statt einer Ratifikation die
Delegations-Formel oben, woertlich. Eine delegierte Wahl als Ratifikation zu verbuchen waere eine
Behauptung, die breiter ist als ihre Messung — genau der Defekt, den diese Runde schliessen soll,
begangen von der Runde, die ihn schliesst.

Ebenfalls festzuhalten, damit ein spaeterer Leser die Beweisdichte dieser Runde richtig einordnet:
**der Betreiber hat fuer den Rest dieser Runde ausdruecklich um schnelleres Vorgehen mit weniger
Verifikation gebeten.** Die Evidenz dieser Runde ist duenner als die der Runden 5 bis 7. Es wurde
genau das gemessen, was die `<verify>`-Bloecke und `<acceptance_criteria>` des Plans verlangen, und
nichts darueber hinaus.

## Accomplishments

- **Der Waechter behauptet nur noch, was er misst.** Der Satz *"Renamed, moved or deleted is the
  same as never having been there"* ist aus dem Doc-Kommentar von `parseBrowserRefusalRanges` UND
  aus dem Fehlertext des Deklarations-Schnitts verschwunden. `grep -c` liefert 0.
- **Die Blindheit ist Daten.** `guardBlindSpots` traegt vier Eintraege mit je Id, Mechanismus,
  gemessener Ausgabe und einem `rowless`-Feld, das genau ein Eintrag traegt.
- **Die Behauptung wird gerendert, nicht danebengeschrieben.** `honestClaim` baut den Text aus der
  Liste; zwei Listen verschiedener Laenge koennen nicht entstehen.
- **Der Lesepfad ist parametrisiert und wrapper-frei.** `browserRefusalRanges(t, path)`; die
  Blindheitstabellen fahren dieselbe Funktion mit demselben `readSource`-Schritt wie
  `TestBrowserRefusalSetEqualsTheServers`.
- **Sieben Zeilen, drei Mechanismen, einzeln gemessen** — mit umgekehrter Abnahme und der
  Anti-Rot-Anweisung ueber beiden Tabellen im Code.
- **Punkt 6 einmal ausgefuehrt und protokolliert**, mit Nicht-Fund, und der Nicht-Fund schliesst
  ausdruecklich nichts.

## Task Commits

1. **Task 1: Ein Mechanismus, einmal ganz durch** — `75bc044` (feat)
2. **Task 2: Die uebrigen Mechanismen und ihre Auspraegungen** — `9e8e430` (feat)
3. **Task 3: Punkt 6, einmal ausgefuehrt und protokolliert** — `b86bafa` (docs)
4. **Lint-Nachzug (golangci-lint QF1012)** — `e040366` (style)

## Die sieben Zeilen und ihre gemessenen Ausgaben

Punkt 5 der Schliessbedingung von Ledger-Eintrag 72. Gemessen mit einer temporaeren Sonde gegen
`parseBrowserRefusalRanges` im ausgelieferten Zustand nach Task 2; die Sonde ist entfernt.

| # | Zeile | Mechanismus | gemessen |
|---|---|---|---|
| 1 | `a declaration surviving only in a block comment` | `text-not-code` | `ranges=6 err=<nil>` |
| 2 | `a declaration surviving only in an indented JSX comment` | `text-not-code` | `ranges=6 err=<nil>` |
| 3 | `a declaration surviving only in a template literal` | `text-not-code` | `ranges=6 err=<nil>` |
| 4 | `a declaration surviving only as JSX text in a pre block` | `text-not-code` | `ranges=6 err=<nil>` |
| 5 | `a declaration whose literal is filtered before it is bound` | `literal-not-value` | `ranges=6 err=<nil>` |
| 6 | `a declaration spread into the table the form really uses` | `literal-not-value` | `ranges=6 err=<nil>` |
| 7 | `an indented leftover declaration beside the real table's import` | `declaration-not-use` | `ranges=2 err=<nil>` |

Zeile 5 ist der gefaehrlichste Rest und der Grund fuer die Skopierung nach Mechanismus: sie ist
**gruen auf echter Drift**, auf lebendem, kompilierendem Code ohne einen Schraegstrich, Backtick oder
Anfuehrungsstrich. Das `].filter((range) => range.class !== 'byte order mark')` hinter dem Literal
laesst den Waechter sechs Bereiche lesen und Uebereinstimmung melden, waehrend das Formular U+FEFF
wieder annimmt und der Server es weiter mit 400 ablehnt. Zeile 7 erwartet **zwei** Bereiche und nicht
sechs — die kuerzere Zahl IST der Schaden.

Der vierte Eintrag, `surrogate-interior`, ist ausdruecklich als nicht zeilenmessbar markiert: der
Codepoint-Sweep ueberspringt U+D800..U+DFFF mit einem `continue` und behauptet die Surrogat-Menge nur
an ihren beiden Endpunkten; ihr Server-Zwilling `rawBodyRefusal` wird von hier nie aufgerufen. **Fuer
2046 innere Codepoints wird nichts verglichen.** Das steht jetzt NEBEN der Behauptung und nicht in
einem anderen Dokument. Keine Fixture kann ihn messen, weil Go keinen unpaarigen Surrogat in einem
String halten kann — `string(rune(0xD800))` ist U+FFFD.

## Die drei Rot-Ausgaben

Eine Tabelle mit umgekehrter Abnahme ist auch dann gruen, wenn sie gar nichts misst. Diese drei
Messungen sind der Beleg, dass sie etwas misst. Jede Aenderung wurde danach zurueckgenommen.

**1. Der Abdeckungstest, Richtung "Eintrag ohne Zeile"** — ein `guardBlindSpots`-Eintrag
`probe-unmeasured` ohne messende Zeile angelegt:

```
--- FAIL: TestGuardBlindSpotsAreEachMeasured (0.00s)
    guard_drift_test.go:1110: guardBlindSpots lists "probe-unmeasured" and no blindness row measures it.
        A listed mechanism without a measured shape is a claim without evidence, which is the thing
        this list replaced. Add a row that measures it, or mark it rowless with the reason no row can.
```

Dieselbe Richtung ein zweites Mal, echt und nicht kuenstlich, zu Beginn von Task 2: die beiden neuen
Eintraege `literal-not-value` und `declaration-not-use` wurden ZUERST angelegt und der Abdeckungstest
gefahren, bevor ihre Zeilen existierten:

```
--- FAIL: TestGuardBlindSpotsAreEachMeasured (0.00s)
    guard_drift_test.go:1124: guardBlindSpots lists "literal-not-value" and no blindness row measures it.
    guard_drift_test.go:1124: guardBlindSpots lists "declaration-not-use" and no blindness row measures it.
```

**2. Die Blindheitszeile gegen eine nicht getroffene Fixture** — die Deklaration in der
Block-Kommentar-Fixture einmal in `REFUSED_RANGES_LEGACY` umbenannt:

```
--- FAIL: TestBrowserRefusalGuardBindsToAnIdentifierAndNotToCode/a_declaration_surviving_only_in_a_block_comment
    guard_drift_test.go:1031: …/001/images.tsx: no REFUSED_RANGES declared as an array literal.
        The browser's set has to be data … A name that merely carries REFUSED_RANGES as a prefix --
        REFUSED_RANGES_LEGACY, REFUSED_RANGESX -- is a different name …

        What this guard is, stated where it matters:
        It is a regular expression over TEXT and not over a program. Its anchor binds to a LITERAL
        and not to the VALUE an expression around it produces, and to an IDENTIFIER and not to CODE
        the browser executes. Not finding the declaration means the anchor did not match this text;
        it does not mean the table is gone. Finding it does not mean the browser runs it.
        What it therefore does not see:
          - text-not-code: …
          - surrogate-interior: …
            no row can measure this: …
```

Diese Ausgabe belegt zwei Dinge in einem: dass die Zeile ueberhaupt etwas misst, und dass
`honestClaim` tatsaechlich im Kein-Anker-Ausgang landet.

**3. Der Live-Pfad-Beleg** — eine Vorverarbeitung in `browserRefusalRanges` gesetzt, die
`/* … */`-Bloecke aus der Quelle entfernt, bevor sie an `parseBrowserRefusalRanges` geht (also genau
die Umgehung, die T-02-131 beschreibt und die `02-DECISION-drift-guard-lexik.md` am Entwurf von B als
Mangel benennt):

```
--- FAIL: TestBrowserRefusalGuardBindsToAnIdentifierAndNotToCode/a_declaration_surviving_only_in_a_block_comment
    guard_drift_test.go:1038: …/001/images.tsx: no REFUSED_RANGES declared as an array literal.
```

Die Zeile geht rot. Ein kuenftiges `stripNonCode` faerbt die Blindheitstabellen also rot, statt sie
zu umgehen — was der Punkt der parametrisierten, wrapper-freien Signatur ist.

## Punkt 6: der Versuch, eine Form zu finden, die die Liste nicht nennt

Ausgefuehrt als Messung gegen den nach Task 2 ausgelieferten Waechter, ueber eine temporaere Sonde
`internal/imagefactory/zz_probe_test.go`, die danach entfernt wurde. Jede Fixture traegt den echten
Import und eine Verwendung, damit sie ein Schadensfall ist und kein TypeScript-Fehler.

| Form | gelesen? | gemessen | deckender Eintrag |
|---|---|---|---|
| `String.raw`-Template | ja | `ranges=6 err=<nil>` | `text-not-code` |
| Regex-Literal | ja | `ranges=6 err=<nil>` | `text-not-code` |
| `${…}`-Interpolation | ja | `ranges=6 err=<nil>` | `text-not-code` |
| Objekt-Property (`cfg = { REFUSED_RANGES: [ … ] }`) | nein | `no REFUSED_RANGES declared as an array literal.` | — (korrekt abgelehnt) |
| Re-Export (`export { REFUSED_RANGES } from …`) | nein | `no REFUSED_RANGES declared as an array literal.` | — (korrekt abgelehnt) |
| zwei Deklarationen, eingerueckte innere zuerst | nein | `2 declarations of REFUSED_RANGES in this source, and this guard reads one.` | — (von Plan 02-29 geschlossen) |

**Ergebnis: kein Fund.** Es wurde keine Form gefunden, die der Waechter liest und deren Mechanismus
die Liste nicht nennt. Die drei gelesenen Formen sind Auspraegungen von `text-not-code` — und genau
das ist der Beleg dafuer, dass die Skopierung nach Mechanismus statt nach Form traegt: eine
Formenliste haette diese drei nicht genannt und waere hier schon wieder unvollstaendig gewesen.

**Punkt 6 hat in dieser Runde etwas gekostet, und das gehoert ins Protokoll:** die sechste Zeile,
zwei Deklarationen mit der eingerueckten inneren zuerst, wurde beim Planen dieser Runde durch genau
diesen Versuch GEFUNDEN. Der Waechter nahm damals schweigend die erste Deklaration und erklaerte den
Fall mit der Ursache eines anderen Defekts (`1 entries inside the REFUSED_RANGES declaration, 7 in
the source as a whole`). Plan 02-29 hat sie **behoben statt gelistet** — die Duplikatspruefung ist
das Ergebnis. Ein Versuch, der einen Fund liefert und eine Reparatur ausloest, ist keine Zeremonie.

**Eintrag 72 bleibt danach offen.** Dieser Task fuehrt Punkt 6 EINMAL aus; die Schliessbedingung
verlangt ihn von der Verifikationsrunde, die den Eintrag schliesst. Ein Eintrag, dessen Bedingung nur
die schon bekannten Formen abfragt, waere ein Eintrag, der sich selbst schliesst.

## Files Created/Modified

- `internal/imagefactory/guard_drift_test.go` — +568/-15. Der parametrisierte Lesepfad, die
  Ausschlussliste als Daten, die gerenderte Behauptung, die Entfernung des allquantifizierten Satzes
  an beiden Stellen, sieben synthetische Fixtures, zwei Blindheitstabellen mit umgekehrter Abnahme
  und Anti-Rot-Anweisung, der Abdeckungstest und das Punkt-6-Protokoll im Doc-Kommentar.

Keine andere Datei wurde beruehrt. `git diff --quiet HEAD -- web/` war ueber den ganzen Plan gruen.

## Decisions Made

Siehe `key-decisions` im Frontmatter. Die tragende Entscheidung ist die Provenienz oben; die
tragende technische ist die Skopierung nach Mechanismus statt nach Form.

## Deviations from Plan

### 1. [Rule 1 - Bug] Die `<pre>`-Fixture haette als roher JSX-Textknoten nicht kompiliert

- **Found during:** Task 2
- **Issue:** Der Plan schreibt fuer Zeile 4 eine Tabelle als reinen JSX-Textknoten in einem
  `<pre>`-Block. In TSX oeffnet ein unmaskiertes `{` in JSX-Kindern einen Ausdrucks-Container;
  `{ from: 0x0000, to: 0x001f, class: 'control character' }` ist dort kein gueltiger Ausdruck. Die
  Fixture waere ein Syntaxfehler gewesen — und ein Abnahmekriterium desselben Plans verlangt
  ausdruecklich, dass **keine** Fixture ein blosser TypeScript-Fehler ist. Der Plan haette sich an
  dieser Stelle selbst widersprochen.
- **Fix:** Die Tabelle liegt in einem Template-Literal INNERHALB des `<pre>`-Blocks. Der gemessene
  Mechanismus ist unveraendert (der Anker sieht Text, den der Browser als Zeichen auf einer Seite
  darstellt); die Zeile heisst weiterhin wie geplant, und die Korrektur samt Begruendung steht als
  Kommentar an der Fixture, nicht in einer Fussnote.
- **Files modified:** `internal/imagefactory/guard_drift_test.go`
- **Verification:** `ranges=6 err=<nil>`, Untertest gruen
- **Committed in:** `9e8e430`

### 2. [Rule 3 - Blocking] golangci-lint QF1012

- **Found during:** Plan-Verifikation
- **Issue:** `honestClaim` benutzte zweimal `b.WriteString(fmt.Sprintf(...))`; staticcheck QF1012
  meldet das, und `golangci-lint run` muss laut `<verification>` mit 0 enden.
- **Fix:** `fmt.Fprintf(&b, …)`. Kein Verhaltensunterschied.
- **Files modified:** `internal/imagefactory/guard_drift_test.go`
- **Verification:** `golangci-lint run` -> `0 issues.`
- **Committed in:** `e040366`

### 3. [Rule 2 - Missing Critical] Das Punkt-6-Protokoll steht zusaetzlich im Code

- **Found during:** Task 3
- **Issue:** Task 3 war als reine Messung geplant und haette bei einem Nicht-Fund **keine einzige
  Zeile** im Baum hinterlassen. Das Protokoll haette dann ausschliesslich in dieser SUMMARY gestanden
  — und der Task begruendet sich selbst damit, dass ein nicht protokollierter Versuch von einem
  unterlassenen nicht zu unterscheiden ist. Fuer den naechsten Ausfuehrenden, der den Code liest und
  nicht den Planungsbestand, waere er genau das gewesen.
- **Fix:** Ein Absatz im Doc-Kommentar von `guardBlindSpots` nennt die sechs Formen, ihre Ausgaben,
  den Nicht-Fund und den Satz, dass ein Versuch, der nur die bekannten Formen abfragt, sich selbst
  schliesst.
- **Files modified:** `internal/imagefactory/guard_drift_test.go`
- **Verification:** `go test ./internal/imagefactory/... -count=1` gruen
- **Committed in:** `b86bafa`

---

**Total deviations:** 3 auto-fixed (1 Bug, 1 Blocking, 1 Missing Critical)
**Impact on plan:** Kein Umfangszuwachs. Deviation 1 haelt ein Abnahmekriterium des Plans gegen einen
anderen Satz desselben Plans ein; Deviation 3 macht das Ergebnis von Task 3 dort sichtbar, wo der
Task selbst sagt, dass es sichtbar sein muss.

## Issues Encountered

**Ein Abnahmekriterium ist nach dem Buchstaben nicht erfuellt, und das gehoert benannt statt
gerundet.** Task 1 verlangt: *"Es gibt genau eine Funktion in dieser Datei, die `readSource` fuer die
Route aufruft und danach `parseBrowserRefusalRanges`"*. Gemessen sind es zwei:
`browserRefusalRanges` (Zeile 335) und `TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration`
(Zeile 563), deren Zeile `the real route` die echte Quelle liest und direkt an
`parseBrowserRefusalRanges` gibt. Diese Zeile stammt aus Runde 4 und ist der Anker, der die uebrigen
Zeilen jener Tabelle an die Wirklichkeit bindet; der Plan verbietet ausdruecklich, eine Tabellenzeile
zu entfernen. Der Halbsatz, den das Kriterium schuetzt — *"kein Wrapper, der einen Pfad hartkodiert"*
— ist erfuellt, und die Eigenschaft, die T-02-131 verlangt, ist gemessen (Rot-Ausgabe 3). Der
Buchstabe des Kriteriums ist es nicht. Es wurde nichts umbenannt, um das zu verdecken.

## Was diese Runde ausdruecklich NICHT geleistet hat

Damit es nicht in dieser SUMMARY untergeht, so wie der Plan es verlangt:

- **G-02-29 ist NICHT geschlossen.** Dieser Plan verengt die Behauptung des Waechters auf null
  Ueberhang; **der Defekt bleibt.** Die Leiche bleibt lesbar, der Waechter bleibt gruen darueber, und
  der Zeitpunkt, an dem das schadet, ist unveraendert. Was sich aendert, ist die Aritaet des Restes:
  von unendlich vielen moeglichen Falsifikationen zu vier gelisteten Mechanismen, deren jeder neue
  Auspraegung eine Ergaenzung und keine Widerlegung ist.
- **G-02-9 bleibt offen.** Keine Zeile dieses Plans sagt etwas anderes.
- **Ledger-Eintrag 70 bleibt offen; 56, 58 und 66 sind unberuehrt.**
  `.planning/WINDOWS.md` wurde von diesem Plan **nicht** geschrieben — gemessen: `windows status`
  meldet weiterhin `"ok": true` und `total_count: 71`. Der amendierende Eintrag 72 gehoert Plan 02-31.
- **Die drei Hinweiszeilen ueber der Deklaration bei `images.tsx:105`, die der Entwurf von Variante B
  vorsieht, wurden NICHT geschrieben.** Damit fehlt B das einzige Geraet, das den Menschen erreicht,
  der die Leiche erzeugt. Das ist eine Umfangsentscheidung dieser Runde und gehoert als benannter
  Rest in Eintrag 72 — Plan 02-31 traegt ihn dort ein.
- **`.planning/REQUIREMENTS.md` wurde nicht geschrieben** und `requirements-completed` ist leer.
- **Der naechste Fund wandert in die Prosa.** Runde 8 findet unter Variante B Saetze statt Muster,
  und Prosa hat keinen Compiler. Der Mechanismus dagegen ist `TestGuardBlindSpotsAreEachMeasured`,
  und er pinnt Mechanismen statt Formen — mehr als der Entwurf vorsah. Ganz weg ist der Einwand
  damit nicht.

## Verifikation

| Kommando | Ergebnis |
|---|---|
| `go build ./... && go vet ./...` | sauber |
| `go test ./internal/imagefactory/... -count=1` | `ok … 2.871s` |
| `go test ./... -count=1 -race` | ueber alle Pakete gruen |
| `golangci-lint run` | `0 issues.` |
| `gofmt -l ./internal` | keine Ausgabe |
| `GOPROXY=off go test ./internal/imagefactory/` | `ok … 2.694s`, Gesamtlaufzeit 3,3 s — offline gruen, keine neue Abhaengigkeit |
| `git diff --quiet HEAD -- web/` | Exit 0 |
| `windows status` | `"ok": true`, `total_count: 71` |

Untertestzahlen, einzeln gefahren mit `-run '^Name$' -v`:

| Tabelle | `--- PASS:` | `--- FAIL:` |
|---|---|---|
| `TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration` | 10 | 0 |
| `TestBrowserRefusalGuardRefusesAnEntryItCannotRead` | 2 | 0 |
| `TestBrowserRefusalGuardRefusesABoundItCannotRepresent` | 3 | 0 |
| `TestBrowserRefusalGuardBindsToAnIdentifierAndNotToCode` | 4 | 0 |
| `TestBrowserRefusalGuardBindsToALiteralAndNotToTheValueTheFormUses` | 3 | 0 |
| `TestGuardBlindSpotsAreEachMeasured` | (kein Untertest) `--- PASS:` | 0 |

`TestBrowserRefusalSetEqualsTheServers` bleibt gruen; sein Sweep wurde nicht veraendert, Rumpf- wie
Dateizahl gegen die echte Route bleiben 6.

Die temporaere Sonde ist rueckstandslos entfernt: `git status --porcelain` zeigt nach dem
Herausfiltern von `internal/imagefactory/guard_drift_test.go`, aller `.planning/`-Pfade und der
SUMMARY keine Zeile.

## Known Stubs

Keine.

## Threat Flags

Keine neue Angriffsflaeche. Dieser Plan aendert eine Testdatei, spricht mit keinem externen Dienst
und fuegt keine Abhaengigkeit hinzu. Die drei Threats des Plans (T-02-129 bis T-02-131) sind
adressiert: T-02-129 durch die Entfernung des allquantifizierten Satzes und die gerenderte Liste,
T-02-130 durch den eigenen Eintrag `literal-not-value` mit zwei messenden Zeilen (**benannt statt
verschwiegen** — behoben ist er nicht), T-02-131 durch den parametrisierten, wrapper-freien Lesepfad
mit gemessenem Beleg.

## User Setup Required

Keine.

## Next Phase Readiness

Welle 3 (Plan 02-31) kann laufen. Sie besitzt `.planning/WINDOWS.md` und `.planning/REQUIREMENTS.md`
fuer diese Runde und legt den amendierenden Eintrag 72 an.

**Zwei Dinge, die Plan 02-31 aus dieser SUMMARY uebernehmen muss und die von seinem Plantext
abweichen:**

1. **Keine Ratifikation eintragen.** Statt der vorgesehenen Ratifikation in den Statusblock von
   `02-DECISION-drift-guard-lexik.md` und in den Text von Eintrag 72 gehoert dort die
   Delegations-Formel: *"Die Wahl zwischen B und A wurde vom Betreiber am 2026-09-05 ausdruecklich an
   den autonomen Lauf delegiert; der Lauf hat B gewaehlt. Der Betreiber hat B nicht selbst
   ratifiziert."* Dazu der Hinweis auf die vom Betreiber gewuenschte reduzierte Verifikationstiefe.
2. **Der Punkt-6-Versuch dieser Runde ist ausgefuehrt und ohne Fund geblieben**, und die
   Doppel-Deklaration ist der Fund, den er beim Planen geliefert hat und den Plan 02-29 behoben hat.
   Beides gehoert in den Text von Eintrag 72, ebenso wie der benannte Rest der nicht geschriebenen
   drei Hinweiszeilen in `images.tsx`.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-09-05*

## Self-Check: PASSED

- `internal/imagefactory/guard_drift_test.go` liegt auf der Platte
- die vier Commits `75bc044`, `9e8e430`, `b86bafa`, `e040366` sind in `git log --oneline --all` auffindbar
- alle `<acceptance_criteria>` und `<verify>`-Kommandos der drei Tasks sind erneut gefahren; das eine
  nach dem Buchstaben nicht erfuellte Kriterium ist unter `## Issues Encountered` benannt und nicht
  weggerundet
