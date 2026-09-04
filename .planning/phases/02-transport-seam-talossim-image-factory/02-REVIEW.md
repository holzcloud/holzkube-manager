---
phase: 02-transport-seam-talossim-image-factory
reviewed: 2026-09-04T13:10:00Z
depth: standard
scope: gap-closure round 5 (Plaene 02-25 und 02-26, diff base 9eeadcd)
files_reviewed: 5
files_reviewed_list:
  - docs/api-contract.md
  - internal/httpapi/handlers/schematics.go
  - internal/httpapi/handlers/schematics_test.go
  - internal/imagefactory/guard_drift_test.go
  - internal/model/model.go
findings:
  critical: 0
  warning: 7
  info: 7
  total: 14
status: issues_found
---

# Phase 02: Code Review Report (Runde 5, Plaene 02-25 und 02-26)

**Reviewed:** 2026-09-04
**Depth:** standard
**Diff base:** `9eeadcd` (Commits `45420ee` .. `4adae38`)
**Files Reviewed:** 5
**Status:** issues_found

## Summary

**CR-01 ist geschlossen, und zwar vollstaendig.** Ich habe versucht, die dritte
Bedingung zu umgehen, und bin nicht durchgekommen. Der Grund ist strukturell und
nicht nur empirisch: `ProbeBuildable` (internal/imagefactory/probe.go:23-38)
haengt an genau drei Groessen — der Schematic-ID, `talosVersion` und `arch` —
und `Platform` ist dort auf `PlatformMetal` festgenagelt. Die ID ist der
Konfliktschluessel, also decken die drei Vorbedingungen in
`refreshTheStoredVerdict` (schematics.go:532, 554, 579) den Definitionsbereich
des Urteils exakt ab. Es bleibt keine vierte Achse offen, an der ein Urteil
ueber eine andere Frage auf diesen Record geschrieben werden koennte. Die beiden
neuen Tests sind ausserdem trennscharf: beide POSTs laufen unter derselben
`arch` (`createBody` setzt amd64 fest, schematics_test.go:517), also faellt ohne
die Versionsbedingung die Arch-Bedingung nicht ein und der Record aendert sich —
die Tests sind ohne den Fix rot. `go test ./internal/httpapi/handlers/
./internal/imagefactory/` laeuft gruen.

**WR-04 ist verengt, nicht geschlossen.** Das ist der Schwerpunkt dieses
Reports. Der neue Anker `refusedRangesDecl` (guard_drift_test.go:83-85) bindet
an ein *Namenspraefix* statt an den Namen: `parseBrowserRefusalRanges` liefert
gegen eine Quelle, in der `REFUSED_RANGES` gar nicht vorkommt, sondern nur
`REFUSED_RANGES_LEGACY`, fehlerfrei eine Range-Liste zurueck (gemessen, siehe
WR-01). Genau die Aussage, die der Waechter in seinem eigenen Fehlertext trifft
— "Renamed, moved or deleted is the same as never having been there",
guard_drift_test.go:240-241 — ist damit falsch, und der neue
Falsifikationstest deckt den Fall nicht ab, weil der Verifier auf
`RENAMED_BY_VERIFIER` umbenannt hat statt auf einen praefixierten Namen. Dazu
kommen drei weitere Stellen, an denen derselbe Waechter Zustimmung meldet, die
er nie geprueft hat: ein `]` in einem Kommentar innerhalb der Deklaration
(WR-02), Bounds ausserhalb des Unicode-Bereichs (WR-03), und der
Surrogate-Bereich, der vom Sweep komplett uebersprungen und nur an seinen beiden
Endpunkten geprueft wird (WR-04, vorbestehend).

Auf der Handler-Seite bleiben zwei kleinere, aber konkrete Maengel: die neue
`versionMismatchReason` hat keinen Leerwert-Zweig, obwohl der Kommentar zwei
Zeilen darueber den Leerwert-Fall ausdruecklich als erreichbar beschreibt
(WR-06), und der Kommentar in `createSchematic` verweist 54 Zeilen ueber der
Funktion, die jetzt drei Bedingungen prueft, weiterhin auf "the two conditions"
(WR-07) — die vierte Aussage im Baum, die der Plan uebersehen hat.

Kein Critical: die Aenderungen dieser Runde erzeugen kein falsches Verhalten im
Produktivpfad, keine Datenverlustgefahr und keine Sicherheitsluecke. Die sieben
Warnings sind ueberwiegend Waechter-Zuverlaessigkeit, was hier ausdruecklich in
Scope ist — ein gruener Waechter, der nichts prueft, ist die Defektklasse, die
diese Runde schliessen wollte.

Bereits in `.planning/WINDOWS.md` erfasste Punkte (Audit-Ausgang eines
auffrischenden 409, Nummer 61; prozessweiter `writeTimeout`, 59;
`CreateRouteBudget`-Clipping, 60) sind hier bewusst nicht erneut aufgefuehrt.

## Narrative Findings (AI reviewer)

## Warnings

### WR-01: `refusedRangesDecl` bindet an ein Namenspraefix statt an den Namen — WR-04 bleibt in verengter Form offen

**File:** `internal/imagefactory/guard_drift_test.go:83-85`, Anspruch bei `228-231` und `240-241`

**Issue:** Der Anker ist

```go
var refusedRangesDecl = regexp.MustCompile(
	`(?ms)^\s*(?:export\s+)?const\s+` + regexp.QuoteMeta(refusedRangesName) +
		`\s*[^=\n]*=\s*\[(.*?)\]`)
```

Nach dem eingesetzten Namen folgt `\s*[^=\n]*`, und `[^=\n]*` frisst jedes
Zeichen ausser `=` und Zeilenumbruch — also auch die Fortsetzung eines laengeren
Identifiers. Es gibt keine Wortgrenze und keine Forderung nach `:` oder `=`
direkt hinter dem Namen. Gemessen gegen die echte Funktion:

```
REFUSED_RANGES absent, REFUSED_RANGES_LEGACY present -> ranges=[{0 0}] err=<nil>
```

`parseBrowserRefusalRanges` liefert also einen Erfolg fuer eine Quelle, in der
`REFUSED_RANGES` nicht existiert. Der Pollution-Check (`whole != len(matches)`,
Zeile 276-283) faengt das nicht ab, weil beide Zaehlungen dieselbe eine
Fremd-Tabelle sehen. Der Fehlertext, der genau diesen Fall ausschliessen soll —
"Renamed, moved or deleted is the same as never having been there, and
entry-shaped literals surviving elsewhere in the file do not make it better" —
wird nie erreicht.

Der stille Schadensfall ist: `REFUSED_RANGES` wird in ein anderes Modul
verschoben oder auf einen nicht praefixierten Namen umbenannt, waehrend eine
praefixierte Rest-Tabelle in `images.tsx` stehenbleibt. Der Waechter liest dann
die Rest-Tabelle, meldet Gleichheit, und die tatsaechlich verwendete Menge kann
frei driften. `TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration` deckt
das nicht ab, weil alle drei Negativ-Zeilen (`renamed`, `absent`, `polluted`)
Namen verwenden, die kein Praefix von `REFUSED_RANGES` sind.

**Fix:** Wortgrenze bzw. das Zeichen erzwingen, das auf den Namen folgen muss,
und eine Zeile in die bestehende Tabelle nachziehen:

```go
var refusedRangesDecl = regexp.MustCompile(
	`(?ms)^\s*(?:export\s+)?const\s+` + regexp.QuoteMeta(refusedRangesName) +
		`\s*(?::[^=\n]*)?=\s*\[(.*?)\]`)
```

```go
// Umbenannt auf einen Namen, der REFUSED_RANGES als Praefix enthaelt. Der
// Verifier hat RENAMED_BY_VERIFIER gewaehlt; das ist die Umbenennung, die
// niemand vornimmt.
const prefixed = `const REFUSED_RANGES_LEGACY: readonly RefusedRange[] = [
  { from: 0x0000, to: 0x0000, class: 'nul' },
]
`
// ... { name: "die Deklaration auf einen praefixierten Namen umbenannt",
//       source: prefixed, wantErr: "REFUSED_RANGES" },
```

### WR-02: Ein `]` in einem Kommentar innerhalb der Deklaration schneidet den Body ab und erzeugt die falsche Diagnose

**File:** `internal/imagefactory/guard_drift_test.go:83-85` (der `(.*?)\]`-Body), Meldung bei `265-269`, Anspruch bei `68-71`

**Issue:** Der Body wird non-greedy bis zur ersten `]` gelesen. Der Kommentar
oberhalb behauptet, das sei ungefaehrlich, "because the count check below
compares the entries found in the body against the entries found in the whole
source and fails on any difference". Das gilt nur, wenn der Schnitt *nach*
mindestens einem Eintrag faellt. Faellt er davor, wird der Count-Check nie
erreicht — der `len(matches) == 0`-Zweig greift vorher. Gemessen:

```
comment carrying a bracket -> ranges=[] err=REFUSED_RANGES is declared but
carries no entries this guard can read. This is the declaration being empty,
not the declaration being absent; ...
```

Die Quelle dieses Laufs war eine vollstaendige, korrekte Deklaration mit einem
Kommentar `// see the table in RefusedRange[] above` als erster Zeile im Body.
Der Waechter meldet "die Deklaration ist leer" ueber eine Deklaration mit sechs
Eintraegen und schickt den Leser damit gezielt in die falsche Richtung — und der
Typname `RefusedRange[]` ist in genau dieser Datei der wahrscheinlichste
Kommentarinhalt ueberhaupt.

**Fix:** Entweder den Body bis zur *letzten* `]` vor dem naechsten
Top-Level-Statement lesen, oder — billiger und ehrlicher — die Meldung nicht
mehr behaupten lassen, die Deklaration sei leer, wenn im Gesamt-Source Eintraege
gefunden werden:

```go
matches := browserRefusalRange.FindAllStringSubmatch(body, -1)
whole := len(browserRefusalRange.FindAllString(source, -1))
if len(matches) == 0 && whole > 0 {
	return nil, fmt.Errorf("the %s declaration body was read as empty while the source "+
		"carries %d entries: the body was almost certainly cut short at a `]` inside a "+
		"comment or a string before the first entry", refusedRangesName, whole)
}
```

### WR-03: Die gelesenen Bounds werden nicht validiert — ein syntaktisch lesbarer Eintrag kann an keinem Vergleich teilnehmen

**File:** `internal/imagefactory/guard_drift_test.go:286-299`

**Issue:** `strconv.ParseUint(m[1], 16, 32)` akzeptiert alles bis
`0xFFFFFFFF`, und `rune(from)` ist eine verlustbehaftete Konvertierung nach
`int32`. Es gibt weder eine Pruefung `from <= to` noch `to <= utf8.MaxRune`.
Gemessen:

```
out-of-range bound -> ranges=[{0 31} {-1 -1}] err=<nil>
inverted bounds    -> ranges=[{31 0}]         err=<nil>
```

Der zweite Eintrag `{-1 -1}` deckt keinen einzigen Codepoint des Sweeps ab
(`for r := rune(0); r <= 0x10FFFF`, Zeile 128) und wird still ignoriert. Das ist
wortwoertlich die Eigenschaft, die der in dieser Runde ergaenzte
Unlesbar-Eintrag-Check beseitigen sollte: "an entry it skips is a codepoint it
reports agreement about without having compared it" (Zeile 253-256). Der neue
Check ist rein syntaktisch — er fragt, ob `browserRefusalRange` das Literal
matcht — und nicht semantisch. Die invertierte Variante wird durch den
Set-Vergleich zufaellig noch laut (sie erzeugt `underRefused`), die
Ueberlauf-Variante nicht.

**Fix:** Nach dem Parsen pruefen, mit derselben Begruendung, die der
Unlesbar-Check bereits traegt:

```go
if to > utf8.MaxRune || from > to {
	return nil, fmt.Errorf("entry %d of %s spans 0x%X..0x%X, which is not a codepoint "+
		"range this guard can compare: an entry that covers nothing is a codepoint set "+
		"this guard reports agreement about without having compared it",
		len(out), refusedRangesName, from, to)
}
```

(`utf8` ist in der Datei bereits importiert.)

### WR-04: Der Surrogate-Bereich wird nur an seinen beiden Endpunkten geprueft

**File:** `internal/imagefactory/guard_drift_test.go:119-124` und `128-131`

**Issue:** Der Sweep ueberspringt `0xD800..0xDFFF` vollstaendig (Zeile 129-131,
korrekt begruendet: Go kann keinen unpaired surrogate in einem String halten).
Als Ersatz steht genau eine Zusicherung:

```go
if !refusedByBrowser(surrogateLow) || !refusedByBrowser(surrogateHigh) {
```

Das prueft `0xD800` und `0xDFFF` und sonst nichts. Eine `REFUSED_RANGES`, die
`{ from: 0xd800, to: 0xd800 }` und `{ from: 0xdfff, to: 0xdfff }` enthaelt —
etwa nach einem verunglueckten Split der Range — ist gruen, waehrend der Browser
2046 Codepoints akzeptiert, die `rawBodyRefusal` mit 400 beantwortet. Der
Waechter, dessen erklaerte Aufgabe Mengengleichheit ist, prueft hier ein
Intervall an zwei Punkten. Vorbestehend, aber in derselben Datei und derselben
Defektklasse wie die uebrigen Befunde dieser Runde.

**Fix:** Die Abdeckung des Intervalls pruefen statt seiner Raender:

```go
for r := rune(surrogateLow); r <= surrogateHigh; r++ {
	if !refusedByBrowser(r) {
		t.Fatalf("%s does not refuse %U. The surrogate range is the one range this "+
			"guard cannot sweep behaviourally -- Go cannot hold an unpaired surrogate "+
			"in a string -- so it has to be asserted whole rather than at its ends.",
			imagesRoutePath, r)
	}
}
```

### WR-05: Der Doc-Kommentar von `refusedRangesDecl` haengt an `refusedRangesEntry`

**File:** `internal/imagefactory/guard_drift_test.go:57-85`

**Issue:** Zeile 57 beginnt `// refusedRangesDecl cuts the declaration out
before anything is read from it.` Der Block laeuft ohne Leerzeile und ohne
dazwischenliegende Deklaration bis Zeile 80 durch und endet auf `var
refusedRangesEntry` (Zeile 81). `refusedRangesDecl` (Zeile 83-85) hat gar keinen
Doc-Kommentar. Der Bruch ist an Zeile 71/72 sichtbar:

```go
// the entries found in the whole source and fails on any difference.
// refusedRangesEntry matches one flat object literal inside the declaration
```

Damit haengt ausgerechnet die Begruendung der Anker-Entscheidung — der ganze
inhaltliche Kern dieser Runde, inklusive der `budget_drift_test.go`-Praezedenz —
am falschen Symbol, und `go doc` zeigt sie unter `refusedRangesEntry`. Das ist
derselbe Defekt, den WR-05 der Vorrunde fuer `allowedHosts`/`ssoOnly` in
`cmd/holzkube-managerd/main.go` festgehalten hat, in einer Datei, die diese
Runde neu geschrieben hat.

**Fix:** Den Block an Zeile 71 trennen und die beiden Haelften vor ihre
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
var refusedRangesDecl = regexp.MustCompile(...)
```

### WR-06: `versionMismatchReason` hat keinen Leerwert-Zweig, obwohl der Kommentar daneben den Leerwert-Fall als erreichbar beschreibt

**File:** `internal/httpapi/handlers/schematics.go:630-634`, Begruendung bei `575-578`

**Issue:** Der Kommentar direkt ueber der Bedingung sagt ausdruecklich: "an
empty stored version is not a record written before the field existed, it is a
record whose version is not known; the inequality declines it, which is
correct." Das Ablehnen ist korrekt. Der Satz, den der Operator daraufhin liest,
ist es nicht:

```
... this record holds the verdict for Talos  and this submission asked about
Talos v1.13.9, and one stored customisation holds exactly one version's verdict.
```

Ein leeres Subjekt und ein doppeltes Leerzeichen. `archMismatchReason`
(schematics.go:617-625) hat fuer genau diesen Fall einen eigenen Zweig und
formuliert ihn aus; die neue Zwillingsfunktion hat ihn nicht. Erreichbar ist der
Leerwert ueber ein von Hand editiertes oder beschaedigtes Record-JSON im
`fsstore` — der einzige Schreiber ist `createSchematic`, der `talos_version`
validiert, aber der Store ist ein Verzeichnis mit JSON-Dateien und der Kommentar
rechnet selbst mit dem Fall.

**Fix:**

```go
func versionMismatchReason(stored, asked string) string {
	if stored == "" {
		return "this record does not name the Talos version its verdict is about, so there " +
			"is nothing to match against, and this submission asked about Talos " + asked
	}
	return "this record holds the verdict for Talos " + stored + " and this submission asked " +
		"about Talos " + asked + ", and one stored customisation holds exactly one version's " +
		"verdict"
}
```

### WR-07: Der Kommentar in `createSchematic` nennt weiterhin "the two conditions"

**File:** `internal/httpapi/handlers/schematics.go:457-459`

**Issue:**

```go
// Usable, ProbedAt and ProbeReason are therefore written and
// nothing else is; see refreshTheStoredVerdict for the two
// conditions and for why a failed refresh is silent.
```

Diese Aussage verweist auf eine Funktion 54 Zeilen weiter unten, die seit
`d4ee1f5` drei Bedingungen prueft. Der Plan hat `docs/api-contract.md`,
`model.go` (ID) und `model.go` (Arch) nachgezogen und die vierte Stelle
uebersehen — die, die im selben File steht und direkt auf die geaenderte
Funktion zeigt. Ein Leser, der dem Verweis folgt, faengt mit einer falschen
Erwartung an; das ist genau die Klasse von Aussage, deren Pflege der Plan
02-25 Task 3 zum Gate gemacht hat.

**Fix:**

```go
// nothing else is; see refreshTheStoredVerdict for the three
// conditions and for why a failed refresh is silent.
```

## Info

### IN-01: Bei gleichzeitigem Arch- und Version-Mismatch nennt das `detail` nur die Architektur

**File:** `internal/httpapi/handlers/schematics.go:554-582`, `docs/api-contract.md:620`

**Issue:** Die drei Bedingungen werden sequenziell geprueft und die erste
fehlschlagende gewinnt. Ein zweiter POST, der sowohl die Architektur als auch
die Talos-Version aendert, bekommt ausschliesslich den Architektur-Satz. Der
Contract verspricht, das `detail` sage, "why". Ein Operator, der daraufhin die
Architektur korrigiert, laeuft in eine zweite Ablehnung mit einem neuen Grund.

**Fix:** Entweder beide Gruende sammeln und mit " und " verbinden, oder im
Contract festhalten, dass das `detail` die erste fehlschlagende Bedingung nennt
und nicht alle.

### IN-02: `model.go` sagt im Zweig "gleiche Version" weiterhin, die Talos-Version werde zurueckgewiesen

**File:** `internal/model/model.go:104-107`

**Issue:** "At the same architecture *and the same Talos version* the 409 now
refreshes Usable, ProbedAt and ProbeReason in place ..., and refuses the label,
the cluster and the Talos version exactly as before." In dem Zweig, den der Satz
beschreibt, ist die Talos-Version definitionsgemaess dieselbe; es gibt dort
nichts zurueckzuweisen. Der Rest der Aufzaehlung (label, cluster) bleibt richtig.

**Fix:** "and refuses the label and the cluster exactly as before" — die
Talos-Version ist in diesem Zweig eine Vorbedingung und kein Refusal-Ziel mehr.

### IN-03: `docs/api-contract.md:620` ueberlaedt "three"

**File:** `docs/api-contract.md:620-621`

**Issue:** "The refresh happens only when **all three** conditions hold, and the
`409` `detail` says which of the three outcomes the caller got." Die beiden
Dreien bezeichnen Verschiedenes (drei Bedingungen, drei Ergebnis-Zeilen) und
stehen in einem Satz. Vor dieser Runde stand dort "both conditions ... which of
the three outcomes", was die Kollision nicht hatte.

**Fix:** "... says which of the three *outcomes* the caller got" umformulieren zu
"... and the `409` `detail` reports the outcome: refreshed and usable, refreshed
and refused, or not refreshed with the reason."

### IN-04: Records, die der Defekt vor 02-25 bereits verfaelscht hat, werden weder erkannt noch benannt

**File:** `internal/model/model.go:181-199`, `docs/api-contract.md:617-634`

**Issue:** Zwischen `efb31da` (02-24, Einfuehrung des Refresh) und `d4ee1f5`
(02-25, Versionsbedingung) konnte ein zweiter POST an einer anderen Version ein
Urteil auf einen Record schreiben, dessen `talos_version` eine andere ist. Solche
Records existieren nach dem Update weiter und tragen ein Urteil, dessen Subjekt
nicht im Record steht. Der Erholungsweg existiert — ein POST an *der* Version,
die der Record nennt, frischt korrekt auf — aber weder `model.go` noch der
Contract sagen das, und es gibt keine Erkennung.

**Fix:** Einen Satz in `TalosVersion`s Kommentar aufnehmen, der den Zeitraum
nennt und den Erholungsweg ausspricht; eine Migration ist hier nicht angebracht,
weil nichts im Record entscheidet, welches der beiden Urteile das gespeicherte
ist.

### IN-05: `wantRanges: 6` pinnt die Eintragszahl der echten Datei, die Begruendung spricht aber von den Zeilen der Tabelle

**File:** `internal/imagefactory/guard_drift_test.go:340-353`

**Issue:** Die Begruendung im Doc-Kommentar lautet "Each table's acceptance pins
the number of its rows, and that number only holds while the tables stay apart."
`wantRanges: 6` pinnt aber nicht die Zeilen der Testtabelle, sondern die Anzahl
der Eintraege in `web/src/routes/images.tsx`. Ein legitimer siebter Range macht
diesen Test rot, obwohl der eigentliche Drift-Waechter gruen bleibt — was
vertretbar ist, aber nicht das ist, was der Kommentar behauptet.

**Fix:** Die Begruendung an das anpassen, was die Zahl tatsaechlich festhaelt,
oder gegen `len(REFUSED_RANGES) > 0` plus einen Vergleich mit dem
Nicht-Synthetik-Lauf pruefen.

### IN-06: "1 entries inside the ..."

**File:** `internal/imagefactory/guard_drift_test.go:278-283`, gepinnt bei `368`

**Issue:** `%d entries` erzeugt bei eins "1 entries". Der Testfall pinnt den
Grammatikfehler wortwoertlich als `wantErr`, was ihn schwerer zu korrigieren
macht als noetig.

**Fix:** `%d entr%s` mit einem Plural-Helfer, oder neutral "entry count inside
the %s declaration: %d, in the source as a whole: %d" — dann bleibt die
`wantErr`-Zeile ein Substring ohne Zahlwort-Grammatik.

### IN-07: Die Auffrischung hat keine Monotonie-Bedingung ueber `ProbedAt`

**File:** `internal/httpapi/handlers/schematics.go:584-604`, Anspruch bei `450-455`

**Issue:** Der Kommentar begruendet den Schreibvorgang damit, die Anfrage halte
"a strictly better-informed answer to the same question than the stored one".
Bei zwei nebenlaeufigen POSTs derselben Customisation gilt das nur, solange die
Reihenfolge der `Put`-Aufrufe der Reihenfolge der `ProbedAt`-Stempel folgt. Wird
Request A zwischen seinem `Get` und seinem `Put` verdraengt, nachdem B bereits
geschrieben hat, ueberschreibt A ein neueres Urteil mit seinem aelteren, und
`ProbedAt` laeuft rueckwaerts. Das CAS schuetzt nur den Fall, dass A *vor* B
gelesen hat. Das Fenster ist sehr klein und beide Urteile beantworten dieselbe
Frage `(id, version, arch)`, deshalb Info und nicht Warning.

**Fix:** Eine Zeile vor dem Schreiben, die den Anspruch des Kommentars zur
Bedingung macht:

```go
if !stored.ProbedAt.Before(fresh.ProbedAt) {
	return ""
}
```

---

_Reviewed: 2026-09-04T13:10:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
_Scope: Runde 5, Plaene 02-25 und 02-26 (diff base `9eeadcd`)_
