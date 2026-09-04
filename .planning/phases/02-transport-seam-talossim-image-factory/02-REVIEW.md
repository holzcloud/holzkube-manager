---
phase: 02-transport-seam-talossim-image-factory
reviewed: 2026-09-04T20:15:00Z
depth: standard
scope: gap-closure round 6 (Plaene 02-27 und 02-28, diff base 78b2fe2)
files_reviewed: 2
files_reviewed_list:
  - internal/imagefactory/guard_drift_test.go
  - internal/httpapi/handlers/schematics.go
findings:
  critical: 0
  warning: 5
  info: 5
  total: 10
status: issues_found
---

# Phase 02: Code Review Report (Runde 6, Plaene 02-27 und 02-28)

**Reviewed:** 2026-09-04
**Depth:** standard
**Diff base:** `78b2fe2` (+211/−9 ueber zwei Dateien)
**Files Reviewed:** 2
**Status:** issues_found

## Summary

### Was die vier Befunde der Runde 5 betrifft — einzeln beurteilt

**WR-01 (Anker bindet an ein Namenspraefix): fuer Praefix-Erweiterungen
geschlossen, als allquantifizierte Eigenschaft nicht.** Ich habe den neuen Anker
gegen eine nachgebaute Fassung von `parseBrowserRefusalRanges` gefahren.
`REFUSED_RANGES_LEGACY` liefert jetzt `err != nil` statt `ranges=[{0 0}]`; die
Verengung wirkt genau dort, wo Runde 5 gemessen hat. Sie wirkt aber nur gegen
*diese eine* Form von Leftover. Ein `REFUSED_RANGES`, das nur noch in einem
`/* ... */`-Block oder in einem Template-Literal steht, erfuellt den Anker
weiterhin und liefert `ranges=[{0 31}] err=<nil>` (WR-01, gemessen). Der Satz,
den der Plan seiner eigenen Fehlermeldung neu hinzugefuegt hat — "that leftover
is what stays behind when the real table moves away" — beschreibt damit
weiterhin nicht alle Leftovers, die stehenbleiben.

**WR-02 (`]` im Kommentar erzeugt die falsche Diagnose): geschlossen, mit einem
neuen Spiegel-Defekt.** Die gemessene Fehlausgabe der Runde 5 ist weg; der
`truncated`-Testfall belegt sie rot-vor-gruen. Der Preis ist, dass der
Leer-Zweig jetzt in der gespiegelten Lage falsch meldet: eine *tatsaechlich
leere* Deklaration in einer Datei, die irgendwo sonst ein eintragsfoermiges
Literal traegt, bekommt "the declaration body was cut short before its first
entry" und die Anweisung "Look for that bracket" — nach einer Klammer, die es
nicht gibt (WR-02, gemessen). Eine falsche Ursache ist gegen eine andere
getauscht, nicht beseitigt.

**WR-03 (Bounds werden nicht validiert): geschlossen.** `to > utf8.MaxRune ||
from > to` steht vor dem `append`, alle drei in Runde 5 gemessenen Werte
erreichen den Sweep nicht mehr, und die drei Tabellenzeilen pruefen die
gelesenen Grenzen als Teilzeichenketten der Meldung statt nur den Fehlschlag.
Der Grenzfall stimmt: `{ from: 0xfffe, to: 0x10ffff }` aus der echten
`images.tsx` wird nicht faelschlich abgelehnt. Zwei Reste: die Meldung nennt
fuer die invertierte Range die falsche Ursache (WR-03), und die Begruendung
fuer `ParseUint(..., 16, 32)` gilt nicht fuer Literale mit mehr als acht
Hexziffern (IN-02).

**WR-07 (`createSchematic` sagt "the two conditions"): geschlossen.**
`schematics.go:458` sagt "the three conditions". Der Sweep aus 02-28
(`grep -rn 'refreshTheStoredVerdict' ... | grep -c '\btwo\b'`) ergibt bei mir
`0`, nachgemessen. Das ist die gesamte Aenderung an dieser Datei in dieser
Runde, und sie ist korrekt.

### Was diese Runde neu eingebracht hat

Der Schwerpunkt bleibt der Waechter. Die zentrale Eigenschaft der Phase — *ein
Waechter meldet einen Durchlauf nur, wenn er die Eigenschaft, nach der er
benannt ist, wirklich gemessen hat* — ist nach dieser Runde enger, aber nicht
hergestellt. Drei der fuenf Warnings betreffen sie unmittelbar: der
Kommentar-Leftover kommt weiterhin durch (WR-01), die neue Testzeile fuer die
praefixierende Umbenennung ist trivial erfuellbar, weil **jede** der sechs
Fehlermeldungen dieser Funktion die Zeichenkette `REFUSED_RANGES` traegt und die
Zeile genau diese als `wantErr` prueft (WR-04) — das ist woertlich die Regel,
die `02-27-SUMMARY.md` unter `key-decisions` fuer die Bound-Meldung selbst
aufgestellt und hier nicht angewendet hat — und die Bound-Meldung erklaert einen
ihrer beiden Faelle mit der Ursache des anderen (WR-03).

Dazu WR-05: der Doc-Kommentar, der die ganze inhaltliche Begruendung dieser
Runde traegt, haengt weiterhin am falschen Symbol — und diese Runde hat ihm 25
Zeilen hinzugefuegt, ohne die fehlende Leerzeile zu setzen. Runde 5 hat das als
WR-05 festgehalten; der Plan hat in genau diesen Block hineingeschrieben.

**Kein Critical.** Die Aenderungen dieser Runde beruehren den Produktivpfad mit
einer einzigen Kommentarzeile. Es gibt kein falsches Laufzeitverhalten, keine
Datenverlustgefahr und keine Sicherheitsluecke. Die Warnings sind
Waechter-Zuverlaessigkeit und Diagnose-Wahrheit, was in dieser Phase
ausdruecklich in Scope ist.

### Nicht erneut verhandelt

WR-04 (Surrogate-Bereich nur an seinen Endpunkten geprueft), WR-06
(`versionMismatchReason` ohne Leerwert-Zweig) und IN-01…IN-04, IN-05, IN-07 der
Runde 5 sind vom Diff nicht beruehrt und bleiben unveraendert offen. IN-06 der
Runde 5 ("1 entries") ist beruehrt: die neue Abschneide-Meldung erzeugt einen
zweiten Fundort desselben Fehlers, deshalb steht er hier als IN-01.

Eine Beobachtung ausserhalb der Datei-Scope: `.planning/WINDOWS.md` Eintrag 70,
von 02-28 angelegt, verweist auf `internal/imagefactory/guard_drift_test.go`
Zeile **83**. Der Anker, den der Eintrag beschreibt, steht nach 02-27 in den
Zeilen 108–110; Zeile 83 liegt mitten in einem Kommentarabsatz. Der Eintrag
wurde nach 02-27 geschrieben und traegt die Zeilennummer von davor.

## Narrative Findings (AI reviewer)

## Warnings

### WR-01: Ein `REFUSED_RANGES` in einem Blockkommentar oder Template-Literal erfuellt den Anker weiterhin — der Waechter meldet Zustimmung ueber eine Tabelle, die der Browser nie ausfuehrt

**File:** `internal/imagefactory/guard_drift_test.go:108-110`, Anspruch bei `265-270`

**Issue:** Der neue Anker ist

```go
`(?ms)^\s*(?:export\s+)?const\s+` + regexp.QuoteMeta(refusedRangesName) +
	`\s*(?::[^=\n]*)?=\s*\[(.*?)\]`
```

Er verlangt `:` oder `=` hinter dem Namen — das schliesst die praefixierende
Umbenennung aus, und das wirkt. Was er nach wie vor nicht verlangt, ist, dass
die getroffene Zeile ausfuehrbarer TypeScript-Code ist. `^\s*` steht mit `(?m)`
am Zeilenanfang und kennt weder Blockkommentare noch Zeichenketten. Gemessen
gegen eine byteweise nachgebaute Fassung der Funktion:

```
A1 block-commented-out decl only        ranges=[{0 31}] err=<nil>
A10 decl inside a template literal      ranges=[{0 31}] err=<nil>
```

Die Quelle von A1 war:

```ts
/*
const REFUSED_RANGES: readonly RefusedRange[] = [
  { from: 0x0000, to: 0x001f, class: 'control character' },
]
*/
export function hasControlCharacter(v: string) { return false }
```

Es gibt in dieser Datei keine lebende Ablehnungstabelle. Der Waechter liest die
auskommentierte, findet Rumpf- und Dateizahl gleich (beide 1), vergleicht die
Server-Menge gegen eine Menge, die der Browser nie benutzt, und meldet einen
Durchlauf. Das ist derselbe Schaden, den WR-01 der Runde 5 beschrieben hat, in
der Leftover-Form, die beim Verschieben einer Tabelle mindestens so haeufig
entsteht wie die Umbenennung: man kommentiert das Alte aus, statt es zu
loeschen. Der Fehlertext, den dieser Plan neu geschrieben hat, behauptet die
Gegenrichtung ausdruecklich ("that leftover is what stays behind when the real
table moves away") und wird in diesem Fall nie erreicht.

Der Pollution-Check faengt es nicht: `whole` zaehlt ueber dieselbe Quelle, in
der nur die auskommentierten Eintraege stehen, also sind beide Zahlen gleich.
Erst wenn eine echte Tabelle *zusaetzlich* existiert, wird es laut — gemessen
als `COUNT: 1 inside, 3 whole` — und dann nennt die Meldung "Pollution oder
Abschneiden" und nicht die tatsaechliche Ursache.

**Fix:** Kommentare und Zeichenketten vor dem Anker aus der Quelle entfernen,
statt den Anker weiter zu verfeinern — der Anker kann diese Unterscheidung
strukturell nicht treffen. Eine billige, in dieser Datei ausreichende Fassung:

```go
// Kommentare und Template-Literale sind kein Code. Ein REFUSED_RANGES, das nur
// noch in einem /* ... */ steht, ist ein Leftover und keine Deklaration -- und
// der Fehlertext unten behauptet genau das bereits.
var tsNonCode = regexp.MustCompile("(?s)/\\*.*?\\*/|//[^\n]*|`[^`]*`")

func stripNonCode(source string) string {
	return tsNonCode.ReplaceAllStringFunc(source, func(s string) string {
		return strings.Repeat("\n", strings.Count(s, "\n"))
	})
}
```

und `parseBrowserRefusalRanges` auf `stripNonCode(source)` laufen lassen — fuer
**beide** Zaehlungen, sonst kippt das Verhaeltnis `whole != len(matches)`. Dazu
eine siebte Tabellenzeile mit der A1-Quelle und einem `wantErr`, das nicht bloss
`REFUSED_RANGES` ist (siehe WR-04).

Falls das als zu teuer gilt, ist die ehrliche Alternative, den Satz "Renamed,
moved or deleted is the same as never having been there" in der Fehlermeldung
und in `.planning/WINDOWS.md` Eintrag 70 auf das einzuschraenken, was gemessen
ist — sonst behauptet die Meldung weiterhin eine Eigenschaft, die der Waechter
nicht hat.

### WR-02: Die neue Abschneide-Diagnose meldet Abschneiden fuer eine tatsaechlich leere Deklaration

**File:** `internal/imagefactory/guard_drift_test.go:300-321`

**Issue:** Die Reihenfolge ist jetzt

```go
whole := len(browserRefusalRange.FindAllString(source, -1))

if len(matches) == 0 && whole > 0 { /* "cut short before its first entry" */ }
if len(matches) == 0             { /* "declared but carries no entries"  */ }
```

`whole` zaehlt ueber die **ganze Datei** und nicht ueber das, was ausserhalb der
Deklaration liegt. Damit entscheidet der erste Zweig nicht "abgeschnitten oder
leer", sondern "traegt die Datei irgendwo ein eintragsfoermiges Literal".
Gemessen:

```
A3 genuinely empty + foreign literal
ranges=[] err=CUT-SHORT: body cut short before its first entry, but 1 entries in source as a whole
```

Die Quelle war:

```ts
const REFUSED_RANGES: readonly RefusedRange[] = []

const SOME_OTHER_TABLE: readonly RefusedRange[] = [
  { from: 0x2028, to: 0x2029, class: 'line separator' },
]
```

Die Deklaration ist woertlich leer. Nichts ist abgeschnitten. Die Meldung sagt
"The declaration is NOT empty" — sie ist es — und schickt den Leser mit "Look
for that bracket, not for a missing table" nach einer Klammer, die nicht
existiert, waehrend die eigentliche Ursache (jemand hat die Tabelle geleert) im
Text nicht vorkommt. Das ist dieselbe Defektklasse wie WR-02 der Runde 5, nur
gespiegelt; `SOME_OTHER_TABLE` ist ausserdem genau die Quelle, die die
`polluted`-Zeile derselben Tabelle bereits als realistisch fuehrt.

Nebenwirkung: der woertlich erhaltene Leer-Zweig ist ab sofort nur noch in einer
Datei erreichbar, die **kein einziges** `{ from: 0x.., to: 0x.. }` mehr traegt.
Die Messung in `02-27-SUMMARY.md`, die seine Erreichbarkeit belegt, wurde genau
gegen eine solche Ein-Zeilen-Quelle gefahren und deckt den realistischen Fall
nicht ab.

**Fix:** Die unterscheidende Groesse ist nicht "Eintraege in der Datei", sondern
"Eintraege hinter dem Ende des gekappten Rumpfes, aber vor dem Ende der
Deklaration". Billiger und ausreichend: die Entscheidung an der Form des Rumpfes
festmachen statt an einer Zaehlung ueber die ganze Datei.

```go
// Ein Rumpf, der nach dem Entfernen von Leerraum leer ist, ist eine leere
// Deklaration. Ein Rumpf, der Text traegt, aus dem dieser Waechter keinen
// Eintrag lesen kann, wurde abgeschnitten -- die beiden sind an dieser
// Eigenschaft unterscheidbar und nicht an einer Zaehlung ueber die ganze Datei.
if len(matches) == 0 {
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("%s is declared but carries no entries this guard can read.\n"+
			"This is the declaration being empty, not the declaration being absent; "+
			"the two are separate failures because they call for separate fixes",
			refusedRangesName)
	}
	return nil, fmt.Errorf("the %s declaration body was cut short before its first entry.\n"+
		"The captured body is %q, which carries no entry this guard can read while the "+
		"source as a whole carries %d. The body is captured non-greedily up to the first "+
		"closing bracket, so a `]` inside a comment or a string before the first entry "+
		"ends the capture there",
		refusedRangesName, body, whole)
}
```

Der gekappte Rumpf `"\n  // see the table in RefusedRange["` erfuellt den
zweiten Zweig, `""` den ersten, und die A3-Quelle landet wieder korrekt im
Leer-Zweig. Die bestehende `truncated`-Zeile bleibt gruen, wenn ihr `wantErr`
auf `"cut short before its first entry"` gekuerzt wird; die Zahl gehoert dann
nicht mehr in die Zusicherung.

### WR-03: Die Bound-Meldung erklaert die invertierte Range mit der Ursache der nicht darstellbaren

**File:** `internal/imagefactory/guard_drift_test.go:370-380`, gepinnt bei `634-638`

**Issue:** Eine Bedingung, zwei Ursachen, eine Meldung:

```go
if to > utf8.MaxRune || from > to {
	return nil, fmt.Errorf("this entry of %s has bounds this guard cannot represent: ...
		"The upper bound has to be at most utf8.MaxRune and the lower bound at most the "+
		"upper. This guard compares SETS by sweeping every codepoint upwards from "+
		"rune(0), and rune(uint64) is lossy: a bound above utf8.MaxRune lands on a "+
		"negative rune, ...
```

Fuer `{ from: 0x001f, to: 0x0000 }` sind beide Grenzen einwandfrei
darstellbar. Der Kopfsatz ("bounds this guard cannot represent") ist falsch, und
der gesamte erklaerende Absatz beschreibt eine verlustbehaftete Konvertierung,
die hier nicht stattfindet; die zutreffende Ursache steht in einem Nebensatz
("and the lower bound at most the upper"). Ein Operator liest, sein Wert liege
oberhalb `utf8.MaxRune`, und sucht dort.

Das ist bemerkenswert, weil derselbe Plan drei Zeilen weiter oben genau das
Gegenteil als Prinzip formuliert und umsetzt: "With the count in hand the three
causes are three messages, and none of them claims another's"
(guard_drift_test.go:301-306). Die Bound-Pruefung ist der eine Ort, an dem
dieselbe Runde zwei Ursachen in einer Meldung zusammenlegt. Die Testzeile
`an inverted range` (Zeile 634-638) nagelt die unzutreffende Formulierung mit
`wantErr: []string{"from 0x001f to 0x0000", "cannot represent"}` fest.

**Fix:** Zwei Zweige, wie an der Abschneide-Diagnose vorgemacht, und die
Testzeile auf das umstellen, was die Meldung dann sagt:

```go
if to > utf8.MaxRune {
	return nil, fmt.Errorf("this entry of %s has an upper bound this guard cannot "+
		"represent: from 0x%s to 0x%s.\n"+
		"The upper bound has to be at most utf8.MaxRune. rune(uint64) is lossy -- a bound "+
		"above utf8.MaxRune lands on a negative rune -- so such an entry covers no "+
		"codepoint in a sweep from rune(0) upwards and is passed over. An entry it passes "+
		"over is a codepoint it reports agreement about without having compared it",
		refusedRangesName, m[1], m[2])
}
if from > to {
	return nil, fmt.Errorf("this entry of %s is inverted: from 0x%s to 0x%s.\n"+
		"Both bounds are representable and the range still covers nothing, because the "+
		"sweep runs upwards from rune(0). An entry that covers nothing is a codepoint set "+
		"this guard reports agreement about without having compared it",
		refusedRangesName, m[1], m[2])
}
```

### WR-04: Die neue Testzeile fuer die praefixierende Umbenennung ist trivial erfuellt — sie unterscheidet den erwarteten Fehlschlag von keinem anderen

**File:** `internal/imagefactory/guard_drift_test.go:483-487`

**Issue:**

```go
{
	name:    "the declaration renamed to a prefixed name",
	source:  prefixed,
	wantErr: "REFUSED_RANGES",
},
```

`parseBrowserRefusalRanges` hat sechs Fehlerausgaenge (Zeilen 261, 283, 308,
317, 330, 371). **Jeder** von ihnen formatiert `refusedRangesName` in seinen
Text; nachgezaehlt an den Aufrufstellen 270, 288, 314, 320, 335, 379. Damit ist
`strings.Contains(err.Error(), "REFUSED_RANGES")` fuer jeden moeglichen
Fehlschlag dieser Funktion wahr, und die Zeile prueft nach `err != nil` nichts
weiter. Ein kuenftiger Anker, der `REFUSED_RANGES_LEGACY` wieder trifft, dann
aber an der Zaehlpruefung oder an der Bound-Validierung scheitert, laesst diese
Zeile gruen — waehrend die Eigenschaft, nach der die Zeile benannt ist ("der
Waechter findet seine Deklaration nur unter ihrem genauen Namen"), verletzt ist.

Das ist woertlich die Regel, die derselbe Plan unter `key-decisions` fuer die
Bound-Meldung aufgestellt hat: "Der statische Text einer Fehlermeldung enthaelt
keine Literale, die eine Tabellenzeile als Teilzeichenkette prueft — sonst ist
die Zeile trivial erfuellt und prueft die Meldung nicht mehr." Fuer die drei
Bound-Zeilen wurde sie eingehalten, fuer die neue Praefix-Zeile nicht. Die
beiden Altzeilen `renamed` und `absent` teilen den Mangel, wurden von dieser
Runde aber nicht angefasst; die Praefix-Zeile ist neu und traegt die
Kernbehauptung dieses Plans.

**Fix:** Auf den Satz pruefen, den nur der Kein-Anker-Ausgang traegt, und die
neue Praefix-Formulierung mitnehmen:

```go
{
	name:   "the declaration renamed to a prefixed name",
	source: prefixed,
	wantErr: "A name that merely carries REFUSED_RANGES as a prefix",
},
```

Sinnvollerweise gleich fuer `renamed` und `absent` auf
`"no REFUSED_RANGES declared as an array literal"` umstellen — dieser Praefix
ist dem Kein-Anker-Ausgang eigen.

### WR-05: Der Doc-Kommentar von `refusedRangesDecl` haengt weiterhin an `refusedRangesEntry` — und diese Runde hat ihm 25 Zeilen hinzugefuegt

**File:** `internal/imagefactory/guard_drift_test.go:57-110`

**Issue:** Runde 5 hat das als WR-05 festgehalten. Der Plan hat in genau diesen
Block hineingeschrieben, ohne ihn zu trennen. Der Bruch steht jetzt bei Zeile
96/97:

```go
// cut-short case carries its own message and the count check keeps the one it
// can still explain.
// refusedRangesEntry matches one flat object literal inside the declaration
// body, whatever it is made of.
```

Ohne Leerzeile laeuft der Block von Zeile 57 durch bis Zeile 105 und
dokumentiert `var refusedRangesEntry` (Zeile 106). `var refusedRangesDecl`
(Zeile 108-110) hat gar keinen Doc-Kommentar. Damit steht die gesamte
Begruendung dieser Runde — der `:`-gebundene Anker, die gemessenen
Praefix-Werte, die `budget_drift_test.go`-Praezedenz, die Erklaerung der
geteilten Diagnose — unter dem Symbol, das sie nicht betrifft, waehrend das
Symbol, um das es geht, unkommentiert daneben steht. `go doc` zeigt es
entsprechend. Der Befund ist damit nicht nur offen, sondern in dieser Runde
gewachsen: 25 der 49 Kommentarzeilen sind neu.

**Fix:** Eine Leerzeile bei Zeile 96/97 und die beiden Haelften vor ihre
jeweiligen Deklarationen ziehen:

```go
// refusedRangesEntry matches one flat object literal inside the declaration
// body, whatever it is made of.
//
// Deliberately without nesting: ...
var refusedRangesEntry = regexp.MustCompile(`\{[^{}]*\}`)

// refusedRangesDecl cuts the declaration out before anything is read from it.
//
// Two precedents, and this takes one thing from each. ...
var refusedRangesDecl = regexp.MustCompile(
	`(?ms)^\s*(?:export\s+)?const\s+` + regexp.QuoteMeta(refusedRangesName) +
		`\s*(?::[^=\n]*)?=\s*\[(.*?)\]`)
```

## Info

### IN-01: Die neue Abschneide-Meldung erzeugt einen zweiten Fundort von "1 entries"

**File:** `internal/imagefactory/guard_drift_test.go:308-314`

**Issue:** `"but %d entries are present in the source as a whole"` liefert bei
eins "but 1 entries are present". Gemessen:

```
A9 cut short, exactly one whole entry
CUT-SHORT: body cut short before its first entry, but 1 entries in source as a whole
```

`02-27-SUMMARY.md` zeigt dieselbe Ausgabe in ihrem eigenen Messprotokoll.
IN-06 der Runde 5 hat den Fehler an der Zaehlpruefung (Zeile 330) festgehalten;
diese Runde hat einen zweiten angelegt, und die `truncated`-Testzeile pinnt ihn
wie schon die `polluted`-Zeile woertlich als `wantErr`.

**Fix:** Ein Plural-Helfer fuer beide Stellen, oder die Zahl aus der Zusicherung
nehmen (siehe WR-02) und neutral formulieren: "entry count in the source as a
whole: %d".

### IN-02: Die Begruendung fuer `ParseUint(..., 16, 32)` gilt nicht fuer Literale mit mehr als acht Hexziffern

**File:** `internal/imagefactory/guard_drift_test.go:365-369`, Ausgang bei `340-347`

**Issue:** Der neue Kommentar begruendet die Bitbreite so: "At 21 ParseUint
would reject these itself, but with `value out of range`, which names neither
the entry, nor the set, nor the reason. Here the message is the point and not
the abort." Das gilt bis `0xFFFFFFFF`. Darueber tut `ParseUint` bei 32 genau
das, wogegen der Absatz argumentiert. Gemessen:

```
A7 nine-hex-digit bound
PARSE-END "1FFFFFFFF": strconv.ParseUint: parsing "1FFFFFFFF": value out of range
```

`browserRefusalRange` erlaubt `0x([0-9a-fA-F]+)` ohne Laengengrenze, also ist
der Fall erreichbar, und der Leser bekommt genau die Meldung, die der Kommentar
als unbrauchbar bezeichnet.

**Fix:** Den `ParseUint`-Fehlerausgang auf dieselbe Meldung leiten wie die
Bound-Pruefung, statt `%w` durchzureichen — dann stimmt die Begruendung fuer
jeden Eingabewert:

```go
from, err := strconv.ParseUint(m[1], 16, 32)
if err != nil {
	return nil, boundsError(m[1], m[2])
}
```

### IN-03: Eine mehrzeilige Typannotation oder eine `let`-Deklaration wird als "keine Deklaration vorhanden" gemeldet

**File:** `internal/imagefactory/guard_drift_test.go:108-110`

**Issue:** `(?::[^=\n]*)?` schliesst den Zeilenumbruch aus, `const` ist fest
verlangt. Gemessen:

```
A4 multi-line type annotation   err=NO-DECL
A6 let instead of const         err=NO-DECL
```

Die Quelle von A4 war eine vollstaendige, korrekte Deklaration, deren
Annotation umgebrochen war. Der Waechter meldet daraufhin "no REFUSED_RANGES
declared as an array literal ... Renamed, moved or deleted is the same as never
having been there" ueber eine Deklaration, die unveraendert dasteht — fail
closed mit falscher Ursache, dieselbe Klasse wie WR-02.

Die Erreichbarkeit ist heute gering und deshalb Info und nicht Warning: `biome`
formatiert mit `lineWidth: 100` (biome.json), und `images.tsx:105` ist 49
Zeichen lang. Sie waechst, sobald die Annotation waechst (etwa
`readonly (RefusedRange & { readonly class: RefusalClass })[]`).

**Fix:** `[^=\n]*` auf `[^=]*` zuruecknehmen — die Wortgrenze traegt hier der
verlangte Doppelpunkt und nicht das Newline-Verbot — und im Kommentar
festhalten, dass `const` verlangt ist und warum.

### IN-04: Der Sweep nennt `0x10FFFF`, die neue Validierung `utf8.MaxRune`

**File:** `internal/imagefactory/guard_drift_test.go:153` gegen `370`

**Issue:**

```go
for r := rune(0); r <= 0x10FFFF; r++ {      // Zeile 153
...
if to > utf8.MaxRune || from > to {          // Zeile 370
```

Zwei Schreibweisen derselben Obergrenze in einer Datei, deren erklaerter Zweck
es ist, zwei Schreibungen einer Menge aneinander zu binden. Sie stimmen heute
ueberein; die Datei selbst ist der Beleg dafuer, dass das kein Argument ist.

**Fix:** `for r := rune(0); r <= utf8.MaxRune; r++` — `utf8` ist importiert, und
die Bound-Validierung ist damit sichtbar dieselbe Grenze wie der Sweep, den sie
schuetzt.

### IN-05: `.planning/WINDOWS.md` Eintrag 70 verweist auf eine Zeilennummer von vor 02-27

**File:** `.planning/WINDOWS.md:87` (ausserhalb der Datei-Scope dieses Reviews)

**Issue:** Der Eintrag zeigt auf
`internal/imagefactory/guard_drift_test.go` Zeile **83** und beschreibt den
Anker. Nach 02-27 steht `refusedRangesDecl` in den Zeilen 108-110; Zeile 83
liegt im Kommentarabsatz ueber die doppelpunkt-gebundene Annotation. 02-28 wurde
nach 02-27 ausgefuehrt und haette die verschobene Nummer sehen koennen.

**Fix:** Die Zeilennummer auf 108 ziehen, oder — haltbarer — auf den
Symbolnamen `refusedRangesDecl` statt auf eine Zeile verweisen.

---

_Reviewed: 2026-09-04T20:15:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
_Scope: Runde 6, Plaene 02-27 und 02-28 (diff base `78b2fe2`)_
