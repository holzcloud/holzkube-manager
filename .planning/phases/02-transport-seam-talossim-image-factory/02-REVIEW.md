---
phase: 02-transport-seam-talossim-image-factory
reviewed: 2026-09-05T12:00:00Z
depth: standard
scope: gap-closure round 7 (Plaene 02-29, 02-30 und 02-31, diff base a42387d)
files_reviewed: 1
files_reviewed_list:
  - internal/imagefactory/guard_drift_test.go
findings:
  critical: 0
  warning: 12
  info: 7
  total: 19
status: issues_found
---

# Phase 02: Code Review Report (Runde 7, Plaene 02-29 / 02-30 / 02-31)

**Reviewed:** 2026-09-05
**Depth:** standard
**Diff base:** `a42387d` (+820/−83 in einer Datei)
**Files Reviewed:** 1
**Status:** issues_found

## Methode

Alle Aussagen unten sind gemessen, nicht erschlossen. Ausserhalb des Repositories
liegt ein eigenstaendiges Go-Programm, das `browserRefusalRange`,
`refusedRangesName`, `refusedRangesEntry`, `refusedRangesDecl`, `guardBlindSpots`,
`honestClaim` und `parseBrowserRefusalRanges` **zeilenweise** aus
`guard_drift_test.go` uebernimmt (`sed -n`-Auszuege, kein Nachbau). Kontrolle: gegen
die echte `web/src/routes/images.tsx` liefert es

```
ranges=6 [{0 31} {127 159} {55296 57343} {8232 8233} {65279 65279} {65534 1114111}] err=nil
```

also genau das, was 02-29-SUMMARY als Messung gegen die echte Route fuehrt.
23 synthetische Quellen wurden dagegen gefahren. `git status --porcelain` ist leer;
`web/src/routes/images.tsx` wurde nicht angefasst.

## Summary

### Die fuenf Befunde der Runde 6 — einzeln beurteilt

**WR-01 (Anker bindet an ein Namenspraefix / lexikalischer Rest): als Anspruch
geschlossen, als Defekt bewusst nicht.** `grep -c 'Renamed, moved or deleted'`
liefert 0. Der allquantifizierte Satz ist aus Doc-Kommentar und Fehlertext
verschwunden und durch `guardBlindSpots` + `honestClaim` ersetzt. Das ist die
richtige Antwort auf den Befund, und sie ist sauber gebaut: Liste als Daten, Text
gerendert statt danebengeschrieben, Zeilen mit umgekehrter Abnahme, Abdeckungstest
in beide Richtungen. **Die Frage dieser Runde ist damit eine neue: deckt sich die
geschriebene Blindheit mit der tatsaechlichen?** Sie tut es nicht, in beide
Richtungen (WR-01 bis WR-05 unten).

**WR-02 (Spiegel-Defekt: `whole` ueber die ganze Datei): geschlossen, gemessen.**
Der Runde-6-Fall — `= []` neben einer zweiten eintragsfoermigen Tabelle — liefert
jetzt

```
ERR: REFUSED_RANGES is declared but carries no entries this guard can read.
This is the declaration being empty, not the declaration being absent; ...
```

also die Leer-Diagnose und nicht mehr die Klammer-Anweisung. `whole` wird erst
unmittelbar vor seiner einen Verwendung berechnet (Zeile 498) und beantwortet eine
Frage. Der von 02-29 selbst benannte Rest (`commentOnlyBody`) ist gepinnt — aber der
Kommentar an genau dieser Stelle widerspricht sich selbst (WR-11).

**WR-03 (eine Meldung fuer zwei Ursachen): geschlossen.** Zwei Zweige (Zeile 558
und 568), `grep MaxRune` im Inversions-Zweig liefert 0, die drei Zeilen der
Bound-Tabelle pinnen unterscheidende Teilzeichenketten. Die echte Route mit
`{0xfffe, 0x10ffff}` wird nicht falsch-rot.

**WR-04 (`prefixed`-Zeile trivial erfuellt): geschlossen.**
`grep 'wantErr: []string{"REFUSED_RANGES"}'` liefert 0 Treffer; alle drei Zeilen
pinnen jetzt `"no REFUSED_RANGES declared as an array literal"` bzw.
`"A name that merely carries REFUSED_RANGES as a prefix"`. Die Fail-first-Ersatz-
Messung in 02-29-SUMMARY ist plausibel und passt zum Endstand.

**WR-05 (Doc-Block am falschen Symbol): geschlossen.** Zeile 58 dokumentiert
`refusedRangesEntry`, Zeile 69 `refusedRangesDecl`, mit Leerzeile dazwischen.

### Was diese Runde neu eingebracht hat, und wo sie sich verrechnet

Die zentrale Eigenschaft der Phase heisst jetzt: *die geschriebene Blindheitsliste
deckt sich mit der tatsaechlichen Blindheit.* Sie ist verletzt, und zwar in beiden
Vorzeichen — das eine davon ist die Defektklasse dieser Phase mit umgedrehtem
Vorzeichen und in dieser Runde neu.

**Unterbehauptung (der Waechter ist blind, die Liste nennt es nicht):** Gemessen,
`ranges=6 err=nil` auf lebendem, referenziertem Code —

- ein **auskommentierter Eintrag INNERHALB einer lebenden Deklaration** (WR-01).
  Der Waechter liest sechs Bereiche, das Formular wendet fuenf an. Das ist woertlich
  der G-02-11-Schaden, auf dem realistischsten Weg, den es gibt: jemand nimmt einen
  Bereich voruebergehend heraus.
- **das Zugehoerigkeitspraedikat und jedes Entry-Feld ausser `from`/`to`** (WR-02).
  `code > r.from && code < r.to` liest `ranges=6 err=nil`; `!REFUSED_RANGES.some(...)`
  ebenso; ein `enabled: false`-Feld, das das Formular auswertet, ebenso.

Beide sind weder `text-not-code` (der Mechanismus spricht ausdruecklich vom Anker
und vom Bezeichner, und alle vier gemessenen Auspraegungen sind Deklarationen, die
ganz in einem Kommentar stehen) noch `literal-not-value` (zwischen Literal und
Bindung steht nichts) noch `declaration-not-use` (die Tabelle wird benutzt).

**Ueberbehauptung (die Liste nennt eine Blindheit, die der Waechter nicht hat):**
`text-not-code` sagt *"the anchor matches the identifier wherever it stands IN THE
TEXT"*. Gemessen: eine Deklaration in einem `//`-Zeilenkommentar und eine in einem
einzeiligen String werden **korrekt abgelehnt** (WR-03). `honestClaim` traegt
denselben zu breiten Satz in jede Kein-Anker-Meldung.

**Belege, die etwas anderes belegen als sie sagen:** `guardBlindSpots[0].measured`
nennt *"only as JSX text inside a <pre> block"*; die Fixture traegt ein
Template-Literal in einem `<pre>` (WR-04) — 02-30 hat das im Fixture-Kommentar
korrekt festgehalten und im `measured`-Text nicht nachgezogen. Zwei weitere
Belegsaetze werden von ihren eigenen Fixtures widerlegt: *"with no slash, backtick
or quote anywhere in the shape"* ueber eine Fixture, die `from '../lib/refusal-table'`
enthaelt, und *"living, compiling, referenced code, with no comment, string or
template anywhere in it"* ueber eine Fixture mit `class: 'control character'`, die
ausserdem nicht kompiliert (WR-05, WR-06).

**Kein Critical.** Diese Runde aendert eine Testdatei; kein Produktivpfad, kein
Laufzeitverhalten, keine Datenverlustgefahr, keine Angriffsflaeche. Die Warnings
sind Waechter-Zuverlaessigkeit und Wahrhaftigkeit der Diagnose, was in dieser Phase
ausdruecklich in Scope ist. Das ist dieselbe Einordnung wie in Runde 6.

### Die zwei konkret nachgeprueften Punkte aus dem Auftrag

**Duplikatspruefung:** greift, mit eigener Ursache, vor jeder Rumpfpruefung. Ein
eingerueckter innerer `const REFUSED_RANGES` vor der echten Tabelle liefert
`2 declarations of REFUSED_RANGES in this source`. Die echte `images.tsx` hat genau
einen Ankertreffer (gemessen: `ranges=6 err=nil`, also `len(decls) == 1`), die
Pruefung kann dort nicht falsch-rot werden. **Nebenwirkung, ungenannt:** dieselbe
Pruefung wird jetzt rot, wenn die alte Tabelle als Blockkommentar stehenbleibt und
die neue daneben steht (WR-08).

**02-30s selbstgemeldete Abweichung (eine statt zwei Funktionen, die die Route
lesen):** die geschuetzte Eigenschaft haelt. `browserRefusalRanges(t, path)` nimmt
den Pfad als Parameter, es gibt keinen Wrapper mit hartkodiertem Pfad, und die
02-30 gemessene Rot-Ausgabe zeigt, dass eine Vorverarbeitung in diesem Pfad die
Blindheitszeilen rot faerbt. Der zweite Leser ist die Runde-4-Zeile `the real
route`, die `readSource` direkt aufruft; sie kann keine Vorverarbeitung verbergen,
weil sie keine Zwischenschicht ist (Rest als IN-07).

### Nicht erneut verhandelt

`IN-02` (Begruendung fuer `ParseUint(..., 16, 32)`), `IN-03` (mehrzeilige
Annotation und `let` -> "keine Deklaration") und `IN-04` (`0x10FFFF` gegen
`utf8.MaxRune`) der Runde 6 sind unveraendert und stehen laut 02-29-SUMMARY unter
einer Policy-Entscheidung des Betreibers. Ich habe sie nachgemessen und fuehre sie
als IN-05 zusammengefasst weiter, ohne neue Argumente. `IN-01` der Runde 6 ist
tatsaechlich entfallen. `IN-05` der Runde 6 (`WINDOWS.md` Zeilennummer) ist laut
02-31 behoben; `.planning/` liegt ausserhalb des Datei-Scope dieses Reviews.

## Narrative Findings (AI reviewer)

## Warnings

### WR-01: Ein auskommentierter Eintrag INNERHALB einer lebenden Deklaration wird als lebend gelesen — und kein Mechanismus der Liste nennt das

**File:** `internal/imagefactory/guard_drift_test.go:167-176` (der Mechanismus), Ausgang bei `498-513`

**Issue:** Gemessen gegen den ausgelieferten Leser:

```ts
const REFUSED_RANGES: readonly RefusedRange[] = [
  { from: 0x0000, to: 0x001f, class: 'control character' },
  { from: 0x007f, to: 0x009f, class: 'control character' },
  { from: 0xd800, to: 0xdfff, class: 'unpaired surrogate' },
  { from: 0x2028, to: 0x2029, class: 'line separator' },
  // Temporarily relaxed while the Factory ticket is open:
  // { from: 0xfeff, to: 0xfeff, class: 'byte order mark' },
  { from: 0xfffe, to: 0x10ffff, class: 'above U+FFFD' },
]
```

```
ranges=6 [{0 31} {127 159} {55296 57343} {8232 8233} {65279 65279} {65534 1114111}] err=nil
```

Der Waechter liest **sechs** Bereiche, das Formular wendet **fuenf** an. U+FEFF wird
vom Browser wieder angenommen, der Server antwortet weiter 400 — der Schaden, fuer
den `TestBrowserRefusalSetEqualsTheServers` geschrieben wurde, und der Waechter
bleibt gruen. Der Zaehlcheck faengt es nicht: der auskommentierte Eintrag liegt
INNERHALB des Rumpfes, also sind `matches` und `whole` beide 6.

Kein Eintrag der Liste nennt diesen Mechanismus. `text-not-code` ist ausdruecklich
in Begriffen des Ankers und des Bezeichners formuliert (*"the anchor matches the
identifier"*), und alle vier gemessenen Auspraegungen sind **Deklarationen, die
ganz in einem Kommentar stehen** — die Fixtures heissen woertlich
`declarationOnlyInABlockComment`, `declarationOnlyInAnIndentedJSXComment`,
`declarationOnlyInATemplateLiteral`, `declarationOnlyAsJSXTextInAPreBlock`. Eine
lebende Deklaration mit einem toten Eintrag ist keine Auspraegung davon.
`literal-not-value` trifft nicht (zwischen Literal und Bindung steht nichts),
`declaration-not-use` trifft nicht (die Tabelle wird benutzt).

Der Doc-Kommentar von `guardBlindSpots` (Zeile 152-165) protokolliert einen
Punkt-6-Versuch mit sechs Formen. Alle sechs fragen nach der **Deklaration**; keine
nach dem **Eintrag**. Der Versuch hat die Ebene nicht gewechselt, auf der der Anker
seine zweite Bindung hat.

**Fix:** Entweder einen fuenften Eintrag mit eigenem Mechanismus aufnehmen, etwa

```go
{
	id: "entry-text-not-entry",
	mechanism: "the entry pattern reads every {from: 0x.., to: 0x..} SHAPE inside the " +
		"captured body; whether that shape is an entry of the array or a line the " +
		"browser never evaluates is the same question one level down from text-not-code",
	measured: "a live declaration with one entry commented out inside it reads " +
		"ranges=6 err=nil while the form applies five -- the count check cannot see " +
		"it because the dead entry lies inside the body and is counted in both numbers",
},
```

samt einer messenden Zeile in `textNotCodeRows` (die Fixture oben, `wantRanges: 6`)
— oder den Mechanismus von `text-not-code` so umformulieren, dass er die
Eintragsebene mitnimmt, und eine Zeile dieser Form ergaenzen. Ohne eine der beiden
Aenderungen behauptet `honestClaim` eine Vollstaendigkeit, die nicht besteht.

### WR-02: Das Zugehoerigkeitspraedikat und jedes Entry-Feld ausser `from`/`to` werden angenommen, nie gelesen — und stehen in keinem Mechanismus

**File:** `internal/imagefactory/guard_drift_test.go:251-258` (die Annahme), `166-209` (die Liste)

**Issue:** `refusedByBrowser` in `TestBrowserRefusalSetEqualsTheServers` ist

```go
if r >= each.from && r <= each.to {
```

Das ist eine **Annahme** darueber, wie das Formular seine Tabelle auswertet. Nichts
liest sie aus der Quelle. Drei Messungen, alle `err=nil`, alle auf lebendem,
referenziertem Code:

| Quelle | gemessen |
|---|---|
| `REFUSED_RANGES.some((r) => code > r.from && code < r.to)` (exklusiv) | `ranges=6 err=nil` |
| `!REFUSED_RANGES.some((r) => code >= r.from && code <= r.to)` (negiert) | `ranges=1 err=nil` |
| Entry mit `enabled: false`, Formular prueft `r.enabled && ...` | `ranges=6 err=nil` |

Im ersten Fall lehnt der Browser U+0000 und U+001F nicht mehr ab, obwohl die Tabelle
sie fuehrt; der Server antwortet 400. Im dritten laesst das Formular U+FEFF durch.
Der Waechter meldet in beiden Faellen Uebereinstimmung mit der Servermenge.

Der Test heisst `TestBrowserRefusalSetEqualsTheServers`. Was er vergleicht, ist die
Servermenge gegen eine Menge, die er aus zwei Feldern **rekonstruiert**, unter einer
Semantik, die er selbst mitbringt. `honestClaim` sagt dazu nichts. Die naechste
Formulierung von `literal-not-value` — *"a filter, a slice or a spread between the
two"* — beschreibt Ausdruecke **um** das Literal herum, nicht die Auswertung der
Elemente an der Verwendungsstelle.

**Fix:** Ein eigener Eintrag, mit einer messenden Zeile pro Auspraegung:

```go
{
	id: "fields-not-predicate",
	mechanism: "the reader takes from and to out of each entry and the sweep supplies " +
		"the membership test itself (r >= from && r <= to); the comparison the form " +
		"actually performs, and every entry field the form consults besides those two, " +
		"are never read",
	measured: "code > r.from && code < r.to reads ranges=6 err=nil while the browser " +
		"stops refusing both endpoints of every range; an entry field enabled: false " +
		"that the form honours reads ranges=6 err=nil over a set the form applies with five",
},
```

Die billige Teilverschaerfung — die eine Zeile, an der das Formular vergleicht, per
Ausdruck an die erwartete Form binden — schliesst den Fall nicht, macht ihn aber
teurer; sie gehoert dann ihrerseits gemessen und nicht behauptet.

### WR-03: `text-not-code` behauptet eine Blindheit, die der Waechter nicht hat — `//`-Zeilenkommentar und einzeiliger String werden korrekt abgelehnt

**File:** `internal/imagefactory/guard_drift_test.go:169-171`, gerendert bei `220-224`

**Issue:** Der Mechanismus sagt

```
"the anchor matches the identifier wherever it stands IN THE TEXT"
```

Das ist zu breit. `refusedRangesDecl` verlangt `(?m)^\s*(?:export\s+)?const\s+` —
also einen Zeilenanfang, gefolgt nur von Leerraum. Gemessen:

```ts
import { REFUSED_RANGES } from '../lib/refusal-table'

// const REFUSED_RANGES: readonly RefusedRange[] = [{ from: 0x0000, to: 0x001f, class: 'c' }]
```

```
ERR: no REFUSED_RANGES declared as an array literal.
```

Ebenso eine Deklaration in einem einzeiligen String-Literal
(`const DOC = 'see: const REFUSED_RANGES ... = [{ ... }]'`) — abgelehnt. Der
Waechter ist in beiden Faellen **nicht** blind; die Liste fuehrt sie trotzdem als
Loch, und `honestClaim` schreibt denselben zu breiten Satz in jede
Kein-Anker-Meldung: *"Its anchor binds ... to an IDENTIFIER and not to CODE the
browser executes"*.

Das ist genau die Defektklasse, die diese Runde beseitigen soll, mit umgedrehtem
Vorzeichen: ein Satz, der breiter ist als die Messung. Er wirkt in die andere
Richtung — er macht den Waechter schlechter, als er ist — aber die Regel dieser
Datei lautet nicht "nicht ueberschaetzen", sondern "nicht behaupten, was nicht
gemessen ist".

**Fix:** Den Mechanismus an die Ankerform binden, statt an "den Text":

```go
mechanism: "the anchor matches an identifier that OPENS A LINE, whatever encloses " +
	"that line; a block comment, a JSX comment and a template literal satisfy it as " +
	"well as code does. Whether the browser executes that line is not a question a " +
	"regular expression can ask. A // line comment does NOT satisfy it, because the " +
	"slashes stand where the anchor requires whitespace -- that is a limit of the " +
	"anchor and not a second blindness",
```

und den Satz in `honestClaim` entsprechend verengen. Alternativ eine Zeile mit
umgekehrter Abnahme fuer den `//`-Fall aufnehmen — dann waere gemessen, dass er
abgelehnt wird, und die Grenze der Blindheit stuende in einer Tabelle statt in Prosa.

### WR-04: `measured` nennt eine Auspraegung, die die Fixture nicht traegt — "JSX text inside a `<pre>` block" ist ein Template-Literal

**File:** `internal/imagefactory/guard_drift_test.go:172-176` gegen `1155-1184`, Zeilenname bei `1052-1055`

**Issue:** Der Belegtext sagt

```
"a declaration living only in a /* */ block, only in an indented {/* */} JSX comment,
 only in a template literal, or only as JSX text inside a <pre> block each read ranges=6"
```

Die vierte Fixture ist

```
"    <pre>{`\n" +
"const REFUSED_RANGES: readonly RefusedRange[] = [\n" +
...
"`}</pre>\n" +
```

also ein **Template-Literal in einem Ausdrucks-Container**, nicht JSX-Text. Der
Fixture-Kommentar (Zeile 1157-1164) sagt das ausdruecklich und begruendet es
richtig: eine Tabelle aus `{ from: … }`-Eintraegen als roher JSX-Textknoten ist in
TSX ein Syntaxfehler, weil `{` einen Ausdrucks-Container oeffnet. 02-30 hat das als
Deviation 1 protokolliert — und den `measured`-Text und den Zeilennamen
(`a declaration surviving only as JSX text in a pre block`) nicht nachgezogen.

Folge: von den vier aufgezaehlten Auspraegungen sind drei und vier **dieselbe
Form** (Deklaration in einem Template-Literal). Die Liste zaehlt vier Belege und
traegt drei. Und der einzige Ort, an dem der Widerspruch aufgeloest ist, ist ein
Kommentar 900 Zeilen weiter unten — der Text, den `honestClaim` einem Leser in die
Fehlermeldung schreibt, traegt die falsche Angabe.

**Fix:** Beides an die Fixture anpassen:

```go
measured: "a declaration living only in a /* */ block, only in an indented {/* */} " +
	"JSX comment, or only in a template literal -- including one rendered inside a " +
	"<pre> block -- each read ranges=6 err=nil with the real import standing beside " +
	"it; String.raw, a regex literal and a ${...} interpolation measured the same. A " +
	"table as raw JSX text is not among them: in TSX an unescaped { opens an " +
	"expression container, so that shape does not compile and is no damage case",
```

und den Zeilennamen auf `a declaration surviving only in a template literal inside a
pre block` ziehen. Der `blindnessRow`-Doc sagt selbst: *"name is the subtest name.
It describes the shape"*.

### WR-05: Zwei Belegsaetze werden von ihren eigenen Fixtures widerlegt

**File:** `internal/imagefactory/guard_drift_test.go:193-194` gegen `1253-1272`, und `1191-1193` gegen `1194-1212`

**Issue:** Erstens, `declaration-not-use`:

```
"reads ranges=2 err=nil -- with no slash, backtick or quote anywhere in the shape"
```

Die Fixture `indentedLeftoverDeclaration` beginnt mit

```ts
import { REFUSED_RANGES } from '../lib/refusal-table'
```

— zwei Schraegstriche und zwei Anfuehrungszeichen in der ersten Zeile, dazu
`class: 'control character'` in jedem Eintrag.

Zweitens, der Kommentar an `literalFilteredBeforeItIsBound`:

```
// it is GREEN ON REAL DRIFT, on living, compiling, referenced code,
// with no comment, string or template anywhere in it.
```

Die Fixture traegt `class: 'control character'`, `class: 'byte order mark'` und
`range.class !== 'byte order mark'` — drei String-Literale — und sie kompiliert
nicht (siehe WR-06).

Die Absicht ist in beiden Faellen erkennbar: gemeint ist "kein Kommentar, String
oder Template, **der die Deklaration traegt**". Geschrieben steht etwas anderes, und
zwar in einer Datei, deren erklaerter Zweck es ist, dass ein Satz nicht breiter ist
als seine Messung. Der erste der beiden steht ausserdem in `guardBlindSpots` und
wird damit von `honestClaim` in Fehlermeldungen ausgegeben.

**Fix:** Beide Saetze auf das einschraenken, was gilt: `-- and no comment, string or
template CARRIES the declaration here; the shape is ordinary code` bzw.
`with nothing that carries the table standing in a comment, a string or a template`.

### WR-06: Drei der sieben Blindheits-Fixtures sind blosse TypeScript-Fehler — genau das Kriterium, mit dem 02-30 die `<pre>`-Fixture umgeschrieben hat

**File:** `internal/imagefactory/guard_drift_test.go:1194`, `1221`, `1253`

**Issue:** 02-30 Deviation 1 begruendet die Aenderung der `<pre>`-Fixture damit,
dass *"ein Abnahmekriterium desselben Plans verlangt, dass keine Fixture ein blosser
TypeScript-Fehler ist"* — *"a fixture that does not compile proves nothing about a
guard"*. Dieselbe Regel ist bei drei anderen Fixtures nicht angewandt:

| Fixture | undefinierte Bezeichner |
|---|---|
| `literalFilteredBeforeItIsBound` (1194) | `RefusedRange` |
| `literalSpreadIntoTheTableTheFormUses` (1221) | `RefusedRange`, `platformRefusals` |
| `indentedLeftoverDeclaration` (1253) | `RefusedRange` |

Keine der drei deklariert oder importiert `type RefusedRange`. Die vier
`text-not-code`-Fixtures sind sauber (dort steht `RefusedRange` nur innerhalb des
Kommentars bzw. des Template-Literals). Betroffen sind ausgerechnet die drei Zeilen,
die das Argument dieser Runde tragen — `literalFilteredBeforeItIsBound` ist im
Kommentar als *"the dangerous direction"* und als *"GREEN ON REAL DRIFT, on living,
**compiling**, referenced code"* bezeichnet.

Erschwerend: nichts im Repository typprueft diese Fixtures. Sie sind Go-Strings; die
Regel ist unerzwungen und wird deshalb weiter driften.

**Fix:** Jeder Fixture, die eine Typannotation benutzt, die Typzeile voranstellen
(so wie `renamed` bei Zeile 602 es bereits tut) und `platformRefusals` importieren
oder deklarieren:

```go
const literalFilteredBeforeItIsBound = `type RefusedRange = { from: number; to: number; class: string }

const REFUSED_RANGES: readonly RefusedRange[] = [
...
```

Und, weil eine unerzwungene Regel wieder bricht: entweder die Fixtures unter einen
`tsc --noEmit`-Lauf stellen (teuer, aber der einzige Weg, der die Regel misst), oder
den Anspruch in den Kommentaren auf das reduzieren, was nachgehalten wird.

### WR-07: Der Doc-Kommentar verspricht, dass ein abgeschnittener Rumpf "nicht still" ist — gemessen ist er still

**File:** `internal/imagefactory/guard_drift_test.go:98-101`, Zaehlcheck bei `498-513`

**Issue:** Der Kommentar an `refusedRangesDecl` sagt:

```
// The body is captured non-greedily up to the first closing bracket. An entry
// carrying a bracket inside a string would cut it short -- which is not silent,
// because the count check below compares the entries found in the body against
// the entries found in the whole source and fails on any difference.
```

Gemessen mit einer Quelle, deren zweiter Eintrag eine Klammer im String vor `from:`
traegt:

```ts
const REFUSED_RANGES: readonly RefusedRange[] = [
  { from: 0x0000, to: 0x001f, class: 'control character' },
  { class: 'line separator ] pair', from: 0x2028, to: 0x2029 },
]
```

```
ranges=1 [{0 31}] err=nil
```

Kein Fehler. Der Rumpf wurde nach dem ersten Eintrag gekappt, der Waechter liest
**einen** von **zwei** deklarierten Bereichen und meldet keinen Fehlschlag. Der
Grund: `browserRefusalRange` zaehlt in beiden Zahlen dasselbe. Der abgeschnittene
zweite Eintrag beginnt mit `{ class:` und wird von `browserRefusalRange` weder im
Rumpf noch in der ganzen Quelle gezaehlt — `matches == whole == 1`, der Vergleich
ist strukturell nicht in der Lage, diesen Abschnitt zu sehen.

Die Gegenprobe zeigt dieselbe Blindheit von der anderen Seite: steht die Klammer im
String **hinter** `from`/`to` (`class: 'bom ] mark'` am Ende des Eintrags), zaehlt
`browserRefusalRange` den halben Eintrag in beiden Zahlen mit und der Waechter liest
zufaellig richtig (`ranges=2 err=nil`). In beiden Faellen misst der Zaehlcheck nicht,
was der Kommentar ihm zuschreibt.

Der Fehlertext bei Zeile 507-512 nennt dieselbe Ursache (*"or the declaration body
was cut short because an entry carries a closing bracket inside a string"*), also
zweimal derselbe unbelegte Anspruch.

**Fix:** Den Satz auf das einschraenken, was der Vergleich kann — `an entry carrying
a bracket inside a string can cut it short, and the count check below sees that only
when the truncation changes the number of {from: 0x.., to: 0x..} shapes` — und den
Fall als eigene Zeile in `TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration`
messen. Wer es schliessen statt beschreiben will: `matches` gegen
`refusedRangesEntry.FindAllString(body, -1)` vergleichen; ein halber Eintrag hat
keine schliessende Klammer und faellt dann auf.

### WR-08: Zwei Pruefungen lesen Kommentare als Code und gehen mit falscher Ursache rot — die Liste nennt nur die gruene Richtung dieser Blindheit

**File:** `internal/imagefactory/guard_drift_test.go:426-432` und `498-513`

**Issue:** `guardBlindSpots` fuehrt die Unfaehigkeit, Text von Code zu
unterscheiden, ausschliesslich als **gruene** Blindheit. Dieselbe Unfaehigkeit macht
zwei andere Pruefungen **falsch rot**, mit einer Ursache, die es nicht gibt.
Gemessen:

```ts
/**
 * The first range is { from: 0x0000, to: 0x001f, class: 'control character' }.
 */
const REFUSED_RANGES: readonly RefusedRange[] = [
  { from: 0x0000, to: 0x001f, class: 'control character' },
]
```

```
ERR: 1 entries inside the REFUSED_RANGES declaration, 2 in the source as a whole.
Either an entry-shaped literal sits outside the declaration ... or the declaration
body was cut short because an entry carries a closing bracket inside a string
```

Weder das eine noch das andere ist der Fall. Die Datei ist korrekt; ein Doc-Kommentar
zitiert einen Eintrag. Das ist keine Theorie: die echte `images.tsx` traegt
unmittelbar ueber der Tabelle einen langen Doc-Block (Zeilen ~85-104), der die
Bereiche in Prosa beschreibt. Ein einziges hinzugefuegtes Beispiel legt den Waechter
lahm, und die Meldung schickt den Leser nach einem Streu-Literal oder einer
Klammer.

Zweiter Fall, dieselbe Ursache: die alte Tabelle bleibt als Blockkommentar stehen,
die neue steht daneben.

```
ERR: 2 declarations of REFUSED_RANGES in this source, and this guard reads one.
```

Es gibt genau eine Deklaration; die zweite ist ein Kommentar. Die Meldung sagt
*"Until exactly one declaration of this name is left, there is no set to compare"* —
der Leser soll etwas entfernen, das kein Code ist.

Beide Faelle sind fail-closed und daher kein Critical. Sie sind trotzdem genau die
Sorte falscher Ursache, die diese Datei seit Runde 3 verfolgt, und keiner der vier
Mechanismen nennt sie: die Liste beschreibt nur, was der Waechter faelschlich
**liest**, nicht, was er faelschlich **anzeigt**.

**Fix:** Kurzfristig beide Meldungen um die dritte moegliche Ursache ergaenzen
(*"...or the shape you see stands in a comment, which this guard cannot tell from
code -- see the blind spots below"*) und `honestClaim` an den Duplikat- und den
Zaehl-Ausgang haengen, nicht nur an den Kein-Anker-Ausgang. Sauberer: einen fuenften
Eintrag `text-not-code, red direction` mit einer Zeile mit **normaler** Abnahme
(`wantErr`) in `TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration`, damit
der Fall gemessen dasteht statt unerwaehnt.

### WR-09: Nichts pinnt, dass `honestClaim` in der Meldung landet oder dass ein Blindheits-Eintrag ueberhaupt einen Beleg traegt

**File:** `internal/imagefactory/guard_drift_test.go:406`, `1334-1394`, Vertrag bei `132-140`

**Issue:** Zwei Luecken derselben Art:

1. `honestClaim()` hat genau eine Aufrufstelle (Zeile 406). Kein Test prueft, dass
   die Kein-Anker-Meldung sie traegt. Die drei `wantErr`-Zeilen `renamed`, `absent`
   und `prefixed` pinnen ausschliesslich Teilzeichenketten des statischen
   Meldungstextes. Loescht jemand `honestClaim()` aus dem `Errorf`-Aufruf oder laesst
   die Funktion `""` zurueckgeben, bleibt die gesamte Suite gruen. Der Kernbeleg von
   02-30 fuer diese Bindung ist eine einmalige, wieder zurueckgenommene Rot-Messung —
   also genau die Art Beleg, die diese Datei bei anderen als "Behauptung" verwirft.

2. `TestGuardBlindSpotsAreEachMeasured` prueft Id-Eindeutigkeit, die
   `rowless`-Ausnahme und die Abdeckung in beide Richtungen. Es prueft **nicht**,
   dass `mechanism` und `measured` nichtleer sind. Der Vertrag des Feldes sagt
   woertlich:

   ```go
   // measured carries the output that evidences the entry. An entry without a
   // measurement is a claim, and claims are what this file is here to stop.
   ```

   Ein Eintrag mit `measured: ""` besteht den Test, sobald irgendeine Zeile seine Id
   nennt, und `honestClaim` rendert dafuer `measured: ` mit nichts dahinter.

**Fix:** Beides in `TestGuardBlindSpotsAreEachMeasured`:

```go
claim := honestClaim()
for _, spot := range guardBlindSpots {
	if strings.TrimSpace(spot.mechanism) == "" || strings.TrimSpace(spot.measured) == "" {
		t.Errorf("guardBlindSpots %q carries an empty mechanism or measurement; "+
			"an entry without a measurement is the claim this list replaced", spot.id)
	}
	if !strings.Contains(claim, spot.id) {
		t.Errorf("honestClaim does not render %q", spot.id)
	}
}
```

und eine Zeile in der Falsifikationstabelle, deren `wantErr` einen Satz aus
`honestClaim` fuehrt (etwa `"Finding it does not mean the browser runs it"`), damit
die Anbindung an den Kein-Anker-Ausgang gemessen ist statt erinnert.

### WR-10: Der Abdeckungstest zaehlt Zeilen, die kein Test ausfuehren muss

**File:** `internal/imagefactory/guard_drift_test.go:1059-1065`, `1301-1331`, `1334-1394`

**Issue:** `TestGuardBlindSpotsAreEachMeasured` baut `measuredBy` aus
`allBlindnessRows()` und **fuehrt keine einzige Zeile aus**. Ausgefuehrt werden sie
von zwei separaten Funktionen, die die Slices direkt bei Namen nehmen:

```go
func allBlindnessRows() [][]blindnessRow {
	return [][]blindnessRow{textNotCodeRows, literalNotValueRows}
}
...
func TestBrowserRefusalGuardBindsToALiteralAndNotToTheValueTheFormUses(t *testing.T) {
	runBlindnessRows(t, literalNotValueRows)
}
```

Loescht jemand eine der beiden `Test…`-Funktionen, bleibt der Abdeckungstest gruen:
die Ids sind weiterhin "gemessen", weil die Zeilen weiterhin in der Liste stehen —
nur laeuft niemand mehr ueber sie. Das ist woertlich die Eigenschaft, die diese Datei
seit sieben Runden verfolgt, eine Ebene ueber dem Waechter: eine Zusicherung, die
einen Durchlauf meldet, ohne die Eigenschaft gemessen zu haben, nach der sie benannt
ist. Der Kommentar ueber `allBlindnessRows` benennt nur die andere Haelfte des
Risikos (*"A table that is not listed here is a table the coverage test cannot
see"*).

**Fix:** Die Ausfuehrung an dieselbe Liste haengen, aus der gezaehlt wird — dann ist
"gelistet" und "gefahren" dieselbe Aussage:

```go
func TestBlindnessRowsAllRun(t *testing.T) {
	for _, rows := range allBlindnessRows() {
		runBlindnessRows(t, rows)
	}
}
```

Die beiden benannten Tests koennen bleiben (ihre Namen tragen die Anti-Rot-
Anweisung), oder ihre Doc-Kommentare wandern an die Slice-Deklarationen und die
beiden Funktionen entfallen. Ohne die Bindung ist `allBlindnessRows` ein zweites
Register, das mit dem ersten driften kann.

### WR-11: Der Kommentar zu `commentOnlyBody` widerspricht seiner eigenen Testzeile und der Messung

**File:** `internal/imagefactory/guard_drift_test.go:671-683`

**Issue:** Derselbe Kommentarblock sagt zweimal Verschiedenes ueber denselben Zweig:

```go
// A body made of nothing but a comment is not
// empty to strings.TrimSpace, so it takes the cut-short branch although
// nothing was cut ...
// ... Today this source reaches the empty
// branch instead, because no entry-shaped literal exists anywhere in it.
```

Gemessen gegen den ausgelieferten Stand:

```
ERR: the REFUSED_RANGES declaration body was cut short before its first entry.
The captured body is "\n  // nothing yet\n" -- it carries text, and no entry this
guard can read. ... Look for that bracket in the body quoted above, not for a
missing table
```

Also der **Abschneide-Zweig**, wie die Testzeile darunter (`wantErr:
"cut short before its first entry"`) es auch verlangt. Der letzte Satz beschreibt
das Verhalten von HEAD `a42387d`, das Task 1 dieses Plans gerade ersetzt hat; er ist
mit der Aenderung nicht mitgezogen worden. 02-29 hat einen anderen Absatz genau
dieser Sorte als Deviation 2 selbst gefunden und korrigiert — diesen nicht.

Nebenbei bestaetigt die Messung den vom Plan benannten Rest: die Anweisung *"Look
for that bracket in the body quoted above"* steht ueber einem Rumpf ohne Klammer.
Das ist der Runde-6-Spiegel-Defekt, verkleinert und offengelegt, aber nicht weg.

**Fix:** Den letzten Satz streichen und durch die Messung ersetzen: `Measured on the
shipped reader, this source takes the cut-short branch and the message quotes
"\n  // nothing yet\n" -- which is why the row pins the quoted body and not the
diagnosis word.`

### WR-12: Die einzige Ausnahme von der Messpflicht ist mit einem Grund begruendet, der nicht traegt

**File:** `internal/imagefactory/guard_drift_test.go:205-207`, Assertion bei `1387-1391`

**Issue:** `surrogate-interior` ist der eine Eintrag, der von der Belegpflicht
befreit ist. Die Begruendung:

```
rowless: "it is a property of the SWEEP and not of the reader, and the blindness
tables drive the reader. Go cannot hold an unpaired surrogate in a string at all --
string(rune(0xD800)) is U+FFFD -- so no fixture can make a row measure it",
```

Der erste Halbsatz traegt. Der zweite nicht, und er ist der, der Unmoeglichkeit
behauptet. Die Blindheit ist eine Eigenschaft der **Browser-Seite** und laesst sich
mit einer gewoehnlichen Fixture zeigen, ohne einen Surrogat in einem Go-String zu
halten:

```ts
const REFUSED_RANGES: readonly RefusedRange[] = [
  { from: 0xd800, to: 0xd800, class: 'unpaired surrogate' },
  { from: 0xdfff, to: 0xdfff, class: 'unpaired surrogate' },
  ...
]
```

Diese Tabelle lehnt 2 von 2048 Surrogaten ab. Die Zusicherung in Zeile 267 prueft
`refusedByBrowser(surrogateLow) && refusedByBrowser(surrogateHigh)` — beide sind
Mitglieder — und der Sweep ueberspringt den Rest. Der Waechter bliebe gruen.

Warum keine Zeile das misst, ist folglich kein Naturgesetz, sondern eine
Entwurfsentscheidung dieser Runde: `browserRefusalRanges` wurde parametrisiert und
wrapper-frei gemacht, damit die Tabellen den Live-Pfad fahren — der **Sweep**
dagegen steht mit `imagesRoutePath` fest in `TestBrowserRefusalSetEqualsTheServers`
(Zeile 249) und ist nicht ueber eine Quelle fahrbar. Genau die Behandlung, die der
Leser bekommen hat, hat der Sweep nicht bekommen, und der `rowless`-Text verbucht
das Ergebnis als Unmoeglichkeit.

Dazu, kleiner: die Assertion nagelt die Id woertlich fest.

```go
if len(rowless) != 1 || rowless[0] != "surrogate-interior" {
```

Wird die Blindheit eines Tages geschlossen und der Eintrag korrekt geloescht, geht
dieser Test rot und seine Meldung liest sich wie ein Verstoss, waehrend ueber beiden
Blindheitstabellen die Anweisung steht, dass Rot bei einer geschlossenen Blindheit
das Loeschen der Zeile bedeutet und nicht das Aufweichen. Diese Anweisung fehlt hier.

**Fix:** Den `rowless`-Text auf den ersten Halbsatz kuerzen und die
Entwurfsentscheidung benennen (`the sweep hardcodes imagesRoutePath and is not
driveable over a source the way the reader is; parameterising it would make this
measurable`). Die Assertion um dieselbe Anti-Rot-Anweisung ergaenzen, die die
Tabellen tragen, und den Sonderfall `len(rowless) == 0` mit einer eigenen Meldung
begruessen (*"a rowless entry disappeared -- if the blindness ended, this is the
right red; delete this expectation with it"*).

## Info

### IN-01: `wantRanges` pinnt nur eine Zahl, und dieselben sechs Bereiche stehen achtmal in der Datei

**File:** `internal/imagefactory/guard_drift_test.go:986-990`, Kopien bei `1020`, `1115`, `1140`, `1170`, `1194`, `1221`, `702`, `725`

**Issue:** Der Doc von `wantRanges` sagt: *"a row that only demanded err == nil
would stay green over a guard that read a different set"*. Eine Anzahl ist nur
geringfuegig staerker: ein Waechter, der sechs **andere** Bereiche liest, bleibt
ebenfalls gruen. Gleichzeitig sind die sechs Bereiche der echten Route in acht
Fixtures woertlich kopiert, und keine Zusicherung vergleicht den Inhalt — die Kopien
koennen von `images.tsx` wegdriften, ohne dass eine Zeile rot wird. Die Begruendung
*"so the measured range count is the one the real route produces"* verlangt nur die
Zahl.

**Fix:** Entweder `wantRanges []declaredRange` statt `wantRanges int`, oder einen
Konstanten-String `realSixEntries` einmal definieren und in die Fixtures einsetzen —
dann ist die Kopie einmal vorhanden und einmal zu pflegen.

### IN-02: Review-Ids der Form `WR-04` werden als dauerhafte Codereferenz benutzt und sind rundenabhaengig

**File:** `internal/imagefactory/guard_drift_test.go:66`, `834-836`

**Issue:** Zwei Stellen verweisen auf `WR-04`. In Runde 5 war WR-04 der
Surrogat-Endpunkt-Befund, in Runde 6 die trivial erfuellte `prefixed`-Testzeile,
gemeint ist offenbar ein aelterer Befund ueber unlesbare Eintraege. Ohne
Rundenangabe ist die Referenz nicht aufloesbar — in einer Datei, deren Thema die
Aufloesbarkeit von Behauptungen ist.

**Fix:** `WR-04 (Runde N)` schreiben, oder auf die stabile G-Nummer bzw. den
Ledger-Eintrag verweisen.

### IN-03: `surrogate-interior` verletzt den Vertrag des Feldes, in dem es steht

**File:** `internal/imagefactory/guard_drift_test.go:129-131`, `143-144`, `196-208`, gerendert bei `406`

**Issue:** Der Doc von `guardBlindSpots` sagt *"what a regular expression over
TypeScript source does not establish"*, und `mechanism` verlangt eine Begruendung
*"in terms of what its anchor binds to"*. `surrogate-interior` ist weder das eine
noch das andere — es ist eine Eigenschaft des Sweeps. Praktische Folge: `honestClaim`
haengt am Kein-Anker-Ausgang von `parseBrowserRefusalRanges`, einer Funktion, die
mit dem Sweep nichts zu tun hat, und erzaehlt dem Leser dort von 2046 nicht
verglichenen Codepunkten.

**Fix:** Entweder den Vertrag oeffnen (`mechanism` beschreibt, woran der Waechter
bindet **oder** was seine Vergleichsschleife auslaesst) oder die Sweep-Blindheit in
eine zweite, eigene Liste ziehen, die an der Sweep-Fehlermeldung gerendert wird.

### IN-04: Ein unicode-escapter Bezeichner ist derselbe Name und bekommt die Praefix-Erklaerung

**File:** `internal/imagefactory/guard_drift_test.go:398-406`

**Issue:** Gemessen mit `const REFUSED_RANGES: readonly RefusedRange[] = [...]`
— fuer den Compiler ist das `REFUSED_RANGES`:

```
ERR: no REFUSED_RANGES declared as an array literal. ... A name that merely carries
REFUSED_RANGES as a prefix -- REFUSED_RANGES_LEGACY, REFUSED_RANGESX -- is a
different name ...
```

Fail closed, mit einer Ursache, die nicht zutrifft: der Name ist weder praefixiert
noch verschieden. Erreichbarkeit gering (biome schreibt so nichts), deshalb Info.

**Fix:** Keine Codeaenderung noetig; `honestClaim` deckt es bereits mit *"Not
finding the declaration means the anchor did not match this text"* ab. Wenn es
genannt werden soll, gehoert es zu dem Mechanismus, der die Ankerform beschreibt
(siehe WR-03).

### IN-05: Die drei Info-Befunde der Runde 6 sind unveraendert, nachgemessen

**File:** `internal/imagefactory/guard_drift_test.go:113-115`, `276`, `517-524`

**Issue:** Gemessen: `let REFUSED_RANGES = [...]` und eine ueber zwei Zeilen
umgebrochene Typannotation liefern beide `no REFUSED_RANGES declared as an array
literal` ueber eine unveraendert dastehende Deklaration (Runde 6 IN-03). `0x10FFFF`
steht in Zeile 276 gegen `utf8.MaxRune` in Zeile 558 (IN-04). `ParseUint(..., 16,
32)` liefert fuer neun Hexziffern weiterhin `value out of range` (IN-02), also genau
die Meldung, die der Kommentar bei Zeile 542-546 als unbrauchbar bezeichnet.

Neu ist nur, dass die Kein-Anker-Meldung jetzt `honestClaim` traegt und damit
wenigstens sagt, dass Nichtfinden nicht Nichtdasein heisst. Die falsche Ursache
bleibt.

**Fix:** Siehe Runde-6-Review IN-02, IN-03, IN-04. Sie stehen laut 02-29-SUMMARY
unter einer Policy-Entscheidung des Betreibers; ich fuehre sie hier nur als
nachgemessen weiter.

### IN-06: Die Eigenschaftsreihenfolge im Eintrag ist tragend

**File:** `internal/imagefactory/guard_drift_test.go:50-51`

**Issue:** `browserRefusalRange` verlangt `{` unmittelbar gefolgt von `from:`.
Gemessen mit `{ class: 'control character', from: 0x0000, to: 0x001f }`:

```
ERR: this entry of REFUSED_RANGES cannot be read by this guard: { class: ... }
```

Fail closed mit korrekter Ursache — aber eine reine Umsortierung der Felder, die in
TypeScript nichts bedeutet, legt den Waechter lahm. Das ist heute unwahrscheinlich
(biome sortiert keine Objektschluessel) und deshalb Info.

**Fix:** `\{[^{}]*?from:\s*0x([0-9a-fA-F]+)[^{}]*?to:\s*0x([0-9a-fA-F]+)` — oder
die Reihenfolge als bewusste Anforderung im Kommentar festhalten, damit sie eine
Entscheidung ist und kein Zufall.

### IN-07: Der zweite Leser der echten Route umgeht `browserRefusalRanges`

**File:** `internal/imagefactory/guard_drift_test.go:598` gegen `367-377`

**Issue:** 02-30 meldet selbst, dass das Kriterium *"genau eine Funktion liest die
Route"* nicht buchstaeblich erfuellt ist. Die geschuetzte Eigenschaft haelt — es gibt
keinen Wrapper mit hartkodiertem Pfad, und eine Vorverarbeitung in
`browserRefusalRanges` faerbt die Blindheitszeilen rot (von 02-30 gemessen). Der
Rest ist umgekehrt: die Zeile `the real route` ruft `readSource` direkt und geht an
`parseBrowserRefusalRanges`, also **an `browserRefusalRanges` vorbei**. Sie ist
damit der einzige Leser, der eine Regression in `browserRefusalRanges` selbst nicht
bemerken wuerde. Praktisch folgenlos, weil `TestBrowserRefusalSetEqualsTheServers`
und alle sieben Blindheitszeilen ueber den Wrapper laufen.

**Fix:** Kein Handlungsbedarf. Wenn das Kriterium buchstaeblich erfuellt werden
soll, `the real route` auf `browserRefusalRanges(t, imagesRoutePath)` umstellen —
dann verliert die Zeile aber ihre Faehigkeit, den Fehlertext zu pruefen, weshalb sie
so bleiben sollte, wie sie ist. Der bessere Zug ist, das Kriterium zu korrigieren.

---

_Reviewed: 2026-09-05T12:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
_Scope: Runde 7, Plaene 02-29, 02-30 und 02-31 (diff base `a42387d`)_
