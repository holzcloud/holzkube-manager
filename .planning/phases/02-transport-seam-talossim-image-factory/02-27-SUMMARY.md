---
phase: 02-transport-seam-talossim-image-factory
plan: 27
subsystem: testing
tags: [go, drift-guard, regexp-anchoring, unicode-bounds, image-factory, typescript-seam]

# Dependency graph
requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "Plan 02-26 loeste die Pruefung als reine Funktion `parseBrowserRefusalRanges` heraus, gab ihren Fehlerfaellen eigene Tabellentests und schnitt den Deklarationsrumpf zuerst heraus; dieser Plan bearbeitet den Rest, den Runde 5 an genau dieser Fassung gemessen hat"
  - phase: 02-transport-seam-talossim-image-factory
    provides: "Plan 02-20 baute `browserRefusalRange` und `TestBrowserRefusalSetEqualsTheServers` — die Ablehnungsmenge, die dieser Plan ausdruecklich nicht anfasst"
provides:
  - "`refusedRangesDecl` mit dem an den genauen Bezeichner gebundenen Anker: hinter dem Namen ist nur noch Leerraum, eine doppelpunkt-gebundene Typannotation oder das Gleichheitszeichen erlaubt"
  - "Die semantische Bound-Validierung in `parseBrowserRefusalRanges`: `to > utf8.MaxRune || from > to` ist ein benannter Fehlschlag mit beiden Grenzen im Text"
  - "Die getrennte Abschneide-Diagnose: `len(matches) == 0 && whole > 0` benennt das Abschneiden, der echte Leer-Fall behaelt seine Meldung woertlich"
  - "`TestBrowserRefusalGuardRefusesABoundItCannotRepresent` — drei Zeilen fuer Ueberlauf, die gefaehrliche Richtung und die invertierte Range"
  - "Zwei weitere Zeilen in `TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration` (jetzt sechs): die praefixierende Umbenennung und der abgeschnittene Rumpf"
affects: [phase-03-talos-transport, image-factory-ui, drift-guards]

# Actuals (#2632)
actuals:
  tokens: 3719
  tasks: 3
  commits: 6

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Ein Anker auf einen Bezeichner in fremder Quelle verlangt das Zeichen, das den Namen syntaktisch beendet (`:` oder `=`), statt eine beliebige Zeichenfolge zuzulassen — das verlangte Zeichen IST die Wortgrenze, denn `_` ist keines von beiden. Ohne es erfuellt jede Praefix-Erweiterung des Namens den Anker"
    - "Eine gelesene Zahl wird nicht nur syntaktisch (matcht das Literal?), sondern semantisch validiert (bedeutet die Zahl etwas im Zielbereich?), bevor sie in eine verlustbehaftete Konvertierung geht"
    - "Der statische Text einer Fehlermeldung enthaelt keine Literale, die eine Tabellenzeile als Teilzeichenkette prueft — sonst ist die Zeile trivial erfuellt und prueft die Meldung nicht mehr"
    - "Eine Diagnose, die zwei Ursachen haben kann, berechnet die unterscheidende Groesse VOR dem Zweig, der sie braucht; sonst greift der allgemeinere Zweig zuerst und meldet die falsche Ursache"

key-files:
  created: []
  modified:
    - internal/imagefactory/guard_drift_test.go

key-decisions:
  - "Die Fehlermeldung der Bound-Validierung traegt KEINE der Beispiel-Hexzahlen (`0xFFFFFFFF`, `0x0061`) in ihrem statischen Text. Die drei Tabellenzeilen pruefen genau diese Zeichenketten als Teilzeichenketten der Meldung; staenden sie fest in der Meldung, waeren die Zeilen erfuellt, ohne dass die Meldung je die gelesenen Grenzen genannt haette — dieselbe Klasse Defekt wie ein Waechter, der gruen ist, ohne gemessen zu haben. Die gefaehrliche Richtung steht deshalb im Kommentar an der Validierung, was das Abnahmekriterium auch genau dort verlangt."
  - "Die Meldung nennt die Grenzen als `from 0x%s to 0x%s` aus den gelesenen Quell-Literalen (`m[1]`, `m[2]`) und nicht formatiert aus den geparsten Werten. `0x%x` von 0 waere `0x0` und nicht `0x0000`; die Meldung soll zitieren, was in der Datei steht, damit der Leser den Eintrag wiederfindet."
  - "Die Bitbreite von `strconv.ParseUint` bleibt bei 32. Bei 21 scheiterte ParseUint selbst, aber mit `value out of range` — einer Meldung, die weder den Eintrag noch die Menge noch den Grund nennt. Der explizite Check nennt alle drei; hier ist die Meldung die Sache und nicht der Abbruch."
  - "Die Berechnung von `whole` steht jetzt vor beiden `len(matches) == 0`-Zweigen und wird von beiden benutzt — sie ist die Groesse, die Abschneiden von Leere unterscheidet. Solange sie danach berechnet wurde, konnte der Leer-Zweig die zweite Ursache gar nicht nennen."

patterns-established:
  - "Bindung an den genauen Namen: ein Waechter, der einen Bezeichner in fremder Quelle sucht, verlangt das Zeichen, das den Bezeichner beendet — sonst bewacht er jede Praefix-Erweiterung mit"
  - "Semantische statt syntaktischer Validierung gelesener Zahlen vor einer verlustbehafteten Konvertierung"
  - "Getrennte Ursachen bekommen getrennte Meldungen, und die Reihenfolge der Zweige wird so gewaehlt, dass die spezifischere Diagnose zuerst greift"

requirements-completed: [FACT-01, FACT-06]

# Coverage metadata (#1602)
coverage:
  - id: D1
    description: "Der Waechter findet seine Deklaration nur unter ihrem genauen Namen; eine praefixierende Umbenennung (`REFUSED_RANGES_LEGACY`, `REFUSED_RANGESX`) erfuellt den Anker nicht mehr"
    requirement: "FACT-06"
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/the_declaration_renamed_to_a_prefixed_name"
        status: pass
      - kind: manual_procedural
        ref: "Lebendbeleg: REFUSED_RANGES im echten Baum durchgaengig in REFUSED_RANGES_LEGACY umbenannt, --- FAIL: TestBrowserRefusalSetEqualsTheServers und --- FAIL: .../the_real_route beobachtet (Runde 5 mass beide PASS), Datei byteweise wiederhergestellt (sha256 identisch, git diff leer)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Eine Grenze ausserhalb des darstellbaren Bereichs und eine invertierte Range sind benannte Fehlschlaege mit beiden Grenzen im Text statt still uebergangener Eintraege"
    requirement: "FACT-06"
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesABoundItCannotRepresent/an_upper_bound_outside_Unicode"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesABoundItCannotRepresent/a_real_lower_bound_with_an_unrepresentable_upper"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesABoundItCannotRepresent/an_inverted_range"
        status: pass
    human_judgment: false
  - id: D3
    description: "Die Leer-Diagnose behauptet nicht mehr, die Deklaration sei leer, wenn die Quelle als Ganzes Eintraege traegt; Abschneiden und Leere sind an ihrem Text unterscheidbar"
    requirement: "FACT-06"
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/the_body_cut_short_by_a_bracket_in_a_comment"
        status: pass
      - kind: other
        ref: "beide Zweige einzeln gemessen: `= []` liefert weiterhin die woertliche Leer-Meldung, der abgeschnittene Rumpf die neue Abschneide-Meldung mit der Zahl der Eintraege"
        status: pass
    human_judgment: false
  - id: D4
    description: "`TestBrowserRefusalSetEqualsTheServers` misst weiterhin dasselbe und bleibt gruen; die Ablehnungsmenge wurde auf keiner der beiden Seiten angefasst und Rumpf- wie Dateizahl sind weiterhin 6"
    requirement: "FACT-01"
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalSetEqualsTheServers"
        status: pass
      - kind: other
        ref: "gemessen mit dem neuen Anker gegen die echte images.tsx: body=6, whole=6 — dieselben Zahlen wie mit dem alten Anker; git diff --stat -- web/ leer, sha256 von images.tsx unveraendert (ca13cf5f...)"
        status: pass
    human_judgment: false
  - id: D5
    description: "Ob die allquantifizierte Wahrheit von G-02-25 damit gilt, ist NICHT von diesem Plan festgestellt — er liefert vier gemessene Korrekturen, die Feststellung gehoert einer Verifikationsrunde"
    verification: []
    human_judgment: true
    rationale: "Eine allquantifizierte Aussage ueber das kuenftige Verhalten des Waechters kann der ausfuehrende Agent nicht messen; genau diese Reihenfolge zu verletzen war der Befund von Runde 5 an Ledger-Eintrag 69. Die Feststellung gehoert der naechsten Verifikationsrunde."

# Metrics
duration: 10 min
completed: 2026-09-04
status: complete
---

# Phase 02 Plan 27: Der auf den Namen gebundene Waechter Summary

**`refusedRangesDecl` verlangt jetzt `:` oder `=` unmittelbar hinter dem Bezeichner, `parseBrowserRefusalRanges` validiert die gelesenen Grenzen gegen `utf8.MaxRune` und `from <= to`, und der Leer-Zweig ist geteilt — die drei Falsifikationen, mit denen Runde 5 den Waechter gruen gegen eine Quelle ohne `REFUSED_RANGES` gemessen hat, laufen seither als Tabellenzeilen mit umgekehrtem Ergebnis.**

## Performance

- **Duration:** 10 min
- **Started:** 2026-09-04T19:03:25Z
- **Completed:** 2026-09-04T19:14:06Z
- **Tasks:** 3
- **Files modified:** 1

## Accomplishments

- **Der Anker bindet an den Namen und nicht an sein Praefix.** Das Fragment hinter `regexp.QuoteMeta(refusedRangesName)` ist von `\s*[^=\n]*=` auf `\s*(?::[^=\n]*)?=` verengt: hinter dem Bezeichner sind nur noch Leerraum, eine Typannotation, die mit einem Doppelpunkt beginnen MUSS, und das Gleichheitszeichen erlaubt. Das verlangte `:` bzw. `=` ist hier die Wortgrenze, denn `_` ist keines von beiden. Die Typannotation bleibt optional und doppelpunkt-gebunden, weil `stringArrayLiteral` die Zuweisungs-Verankerung genau wegen `readonly RefusedRange[]` gewaehlt hat und eine TypeScript-Annotation immer mit einem Doppelpunkt beginnt.
- **Der Fehlertext macht seinen eigenen Satz wahr.** "Renamed, moved or deleted is the same as never having been there" stand schon dort und war nicht wahr. Er hat den Zusatz bekommen, dass ein Name, der den gesuchten als Praefix traegt, ein anderer Name ist — und nennt beide gemessenen Faelle beim Namen.
- **Eine nicht darstellbare Grenze ist ein benannter Fehlschlag.** `to > utf8.MaxRune || from > to` steht vor dem `append`. Der bisherige Unlesbar-Check war syntaktisch (matcht `browserRefusalRange` das Literal?) und nicht semantisch; `strconv.ParseUint(m[1], 16, 32)` akzeptiert bis `0xFFFFFFFF`, und `rune(uint64)` verliert still. Die vier in Runde 5 gemessenen Werte erreichen den Codepoint-Sweep nicht mehr.
- **Die Diagnose ist geteilt: drei Ursachen, drei Meldungen.** Die Lesbarkeitsschleife faengt den unlesbaren Eintrag INNERHALB der Deklaration, der neue Zweig `len(matches) == 0 && whole > 0` das Abschneiden VOR dem ersten Eintrag, die Zaehlpruefung das Literal AUSSERHALB. Keine behauptet die Ursache einer anderen.
- **Drei Falsifikationen als Testzeilen, jede vorher rot gesehen.** Die Tabellen haben jetzt 6, 3 und 2 Zeilen; keine bestehende Zeile wurde entfernt oder umbenannt.

## Task Commits

1. **Task 1 RED: die Praefix-Umbenennung als Tabellenzeile** — `da5c54c` (test)
2. **Task 1 GREEN: der Anker bindet an den Namen und nicht an sein Praefix** — `2ed7f6b` (feat)
3. **Task 2 RED: drei Bound-Faelle als eigene Tabelle** — `e373099` (test)
4. **Task 2 GREEN: eine nicht darstellbare Grenze ist ein Fehlschlag** — `bdaec63` (feat)
5. **Task 3 RED: der abgeschnittene Rumpf als sechste Zeile** — `ecdd083` (test)
6. **Task 3 GREEN: die Leer-Diagnose sagt nicht mehr "leer"** — `6652eab` (feat)

Kein REFACTOR-Commit: keine der drei GREEN-Fassungen brauchte eine Aufraeumung, die die Tests noch bestehen lassen musste.

## Files Created/Modified

- `internal/imagefactory/guard_drift_test.go` — der gebundene Anker, die Bound-Validierung, die geteilte Diagnose, die neue Tabelle und die zwei neuen Zeilen der bestehenden Tabelle (+210/-8 Zeilen)

`web/src/routes/images.tsx` steht bewusst nicht hier: der Lebendbeleg hat die Datei waehrend des Laufs veraendert und danach byteweise wiederhergestellt. Der Netto-Diff ist leer.

## Rot vor gruen — die drei gemessenen Fehlausgaben

Jede der drei Eigenschaften wurde rot gesehen, bevor sie gruen war. Die Zeilen liefen jeweils gegen den unveraenderten Code, bevor die Korrektur eingesetzt wurde.

**1. Der Anker (`da5c54c`, gegen `\s*[^=\n]*=`):**

```
=== RUN   TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/the_declaration_renamed_to_a_prefixed_name
    guard_drift_test.go:402: no error; read 2 ranges instead.
        A guard that reports a pass here reports agreement it never checked -- the property round 4 measured as absent.
--- FAIL: TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/the_declaration_renamed_to_a_prefixed_name
```

Die anderen vier Zeilen waren im selben Lauf gruen — genau das Bild, das Runde 5 beschreibt: alle Negativzeilen bestehen, weil keine von ihnen einen Namen benutzt, der `REFUSED_RANGES` als Praefix traegt.

**2. Die Bounds (`e373099`, ohne Validierung).** Alle drei Zeilen rot, und die gelesenen Werte reproduzieren die Messung aus Runde 5 woertlich:

```
    guard_drift_test.go:559: no error; read 1 ranges instead: [{0 -1}]     (an_upper_bound_outside_Unicode)
    guard_drift_test.go:559: no error; read 1 ranges instead: [{97 -1}]    (a_real_lower_bound_with_an_unrepresentable_upper)
    guard_drift_test.go:559: no error; read 1 ranges instead: [{31 0}]     (an_inverted_range)
```

`{97 -1}` ist die gefaehrliche Richtung: die Form im Browser lehnt damit jeden Kleinbuchstaben ab, waehrend Go einen Bereich liest, der im Sweep ab `rune(0)` keinen einzigen Codepoint abdeckt, und nichts vergleicht.

**3. Die Diagnose (`ecdd083`, ungeteilter Leer-Zweig):**

```
=== RUN   TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/the_body_cut_short_by_a_bracket_in_a_comment
    guard_drift_test.go:481: error does not name "cut short before its first entry, but 2 entries are present in the source as a whole":
        REFUSED_RANGES is declared but carries no entries this guard can read.
        This is the declaration being empty, not the declaration being absent; the two are separate failures because they call for separate fixes
```

Fail closed mit falscher Ursache, an einer Quelle mit zwei Eintraegen — der Befund aus `adversarial_checks_run[2]`, reproduziert.

## Die beiden Anzahlen mit dem neuen Anker

Gegen die echte `web/src/routes/images.tsx` gemessen, nachdem der Anker verengt war:

```
MEASURED body=6 whole=6
MEASURED ranges=[{0 31} {127 159} {55296 57343} {8232 8233} {65279 65279} {65534 1114111}] err=<nil>
```

**Sechs Treffer im Deklarationsrumpf, sechs in der ganzen Datei** — dieselben Zahlen, die Plan 02-26 mit dem alten Anker gemessen hat. Der neue Anker verengt auf den Namen und nicht auf eine Schreibweise: `export const NAME: … = [`, `const NAME: … = [` und `const NAME = [` treffen weiterhin, `const NAME_LEGACY: … = [` und `const NAMEX = [` nicht mehr. Beide Praefix-Varianten wurden gegen die reine Funktion nachgemessen und liefern jetzt `err != nil` statt `ranges=[{0 0}]` bzw. `ranges=[{0 1}]`.

Alle sechs echten Eintraege liegen im darstellbaren Bereich; die neue Bound-Validierung lehnt keinen von ihnen ab.

## Der Lebendbeleg

Die Falsifikation der Runde 5 am echten Baum wiederholt, mit umgekehrtem Ergebnis.

```
sha256 vorher:   ca13cf5f5bf2abc1f1e4c9376355363f70d232305ef7beaf9716a42f618ee505
perl -pi -e 's/REFUSED_RANGES/REFUSED_RANGES_LEGACY/g' web/src/routes/images.tsx
Vorkommen von REFUSED_RANGES_LEGACY: 3

go test ./internal/imagefactory -run TestBrowserRefusal -count=1 -v
    guard_drift_test.go:119: ../../web/src/routes/images.tsx: no REFUSED_RANGES declared as an array literal.
        ... Renamed, moved or deleted is the same as never having been there, and entry-shaped
        literals surviving elsewhere in the file do not make it better. A name that merely carries
        REFUSED_RANGES as a prefix -- REFUSED_RANGES_LEGACY, REFUSED_RANGESX -- is a different name
        and does not satisfy this guard either; that leftover is what stays behind when the real
        table moves away
--- FAIL: TestBrowserRefusalSetEqualsTheServers (0.00s)
--- FAIL: TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration (0.00s)
    --- FAIL: TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/the_real_route (0.00s)
    --- PASS: TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/the_declaration_renamed_out_of_existence
    --- PASS: TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/the_declaration_absent
    --- PASS: TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/an_entry-shaped_literal_outside_the_declaration
    --- PASS: TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/the_declaration_renamed_to_a_prefixed_name

git checkout -- web/src/routes/images.tsx
sha256 nachher:  ca13cf5f5bf2abc1f1e4c9376355363f70d232305ef7beaf9716a42f618ee505
git diff --stat -- web/src/routes/images.tsx   ->  leer
git diff --stat -- web/                        ->  leer
```

**Runde 5 hat an derselben Stelle `--- PASS: TestBrowserRefusalSetEqualsTheServers` UND `--- PASS: TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration` mit allen vier Untertests gemessen.** Beide sind jetzt rot. Die Datei ist byteweise wiederhergestellt: identischer sha256, leerer Diff.

## Die beiden Zweige der Diagnose, einzeln gemessen

```
MEASURED empty:     REFUSED_RANGES is declared but carries no entries this guard can read.
                    This is the declaration being empty, not the declaration being absent; the two
                    are separate failures because they call for separate fixes

MEASURED truncated: the REFUSED_RANGES declaration body was cut short before its first entry, but 1
                    entries are present in the source as a whole.
                    The declaration is NOT empty. The body is captured non-greedily up to the first
                    closing bracket, so a `]` inside a comment or a string before the first entry
                    ends the capture there and nothing readable is left inside it. Look for that
                    bracket, not for a missing table
```

Die Meldung des echten Leer-Falls ist woertlich erhalten und weiterhin erreichbar (gemessen gegen `const REFUSED_RANGES: readonly RefusedRange[] = []`). Beide Faelle sind an ihrem Text unterscheidbar. Die Tabelle bekommt fuer den Leer-Fall keine siebte Zeile, weil die Abnahme dieses Plans die Zeilenzahl bei sechs festnagelt; die Erreichbarkeit wurde stattdessen als Messung belegt.

## Die drei Tabellen

| Test | Zeilen | davon in dieser Runde ergaenzt |
|---|---|---|
| `TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration` | 6 | `the declaration renamed to a prefixed name`, `the body cut short by a bracket in a comment` |
| `TestBrowserRefusalGuardRefusesABoundItCannotRepresent` | 3 | alle drei (neuer Test) |
| `TestBrowserRefusalGuardRefusesAnEntryItCannotRead` | 2 | keine — unveraendert |

Die vier bestehenden Zeilen der ersten Tabelle (`the real route`, `the declaration renamed out of existence`, `the declaration absent`, `an entry-shaped literal outside the declaration`) und die zwei der dritten stehen woertlich unveraendert da. Es wurde keine Zeile entfernt und keine umbenannt.

## Was dieser Plan ausdruecklich NICHT behauptet

- **G-02-25 wird von diesem Plan nicht als geschlossen gemeldet.** Er liefert vier gemessene Korrekturen — den gebundenen Anker, die Bound-Validierung, die getrennte Diagnose und die drei Tabellen, die diese Faelle selbst pruefen. Ob die allquantifizierte Wahrheit "a guard reports a pass only when it measured the property it is named for" damit gilt, misst eine Verifikationsrunde und nicht der ausfuehrende Agent. Genau diese Reihenfolge zu verletzen war der Befund von Runde 5 an Ledger-Eintrag 69.
- **G-02-9 bleibt offen.** Er beruehrt diesen Plan nicht; die Ledger-Eintraege 56, 58 und 66 bleiben unveraendert.
- **Die Ablehnungsmenge wurde auf keiner der beiden Seiten geaendert** — weder `REFUSED_RANGES` in `web/src/routes/images.tsx` noch `NotRepresentableReason` auf der Go-Seite. Diese Luecke betrifft das kuenftige Verhalten des Waechters, nicht seinen heutigen Befund; Runde 5 hat neu gemessen, dass heute nichts maskiert ist.
- **`.planning/WINDOWS.md` wurde von diesem Plan nicht geschrieben.** Der Ledger gehoert in dieser Runde Plan 02-28. `gsd-tools windows status` meldet unveraendert `"ok": true` bei `total_count: 69` (54 open / 0 waived / 15 fixed), und `git diff --stat -- .planning/WINDOWS.md` ist leer.
- **Kein neuer Pfad in `parseBrowserRefusalRanges` ueberspringt einen Eintrag.** Die neue Validierung gibt einen Fehler zurueck; sie enthaelt kein `continue`.

## Verification

| Pruefung | Ergebnis |
|---|---|
| `go build ./... && go vet ./...` | sauber |
| `go test ./internal/imagefactory -count=1` | ok, 2.684s |
| `go test ./... -count=1 -race` | alle Pakete gruen |
| `golangci-lint run` | `0 issues.`, Exit 0 |
| `gofmt -l ./internal` | leer |
| `...RefusesToPassWithoutItsDeclaration` | genau **6** Untertest-`--- PASS:` |
| `...RefusesABoundItCannotRepresent` | genau **3** Untertest-`--- PASS:` |
| `...RefusesAnEntryItCannotRead` | genau **2** Untertest-`--- PASS:` |
| `TestBrowserRefusalSetEqualsTheServers` | `--- PASS:`, Test unveraendert |
| `grep -Fc '(?::[^=\n]*)?='` | 1 |
| `grep -c 'utf8.MaxRune'` | > 0 |
| `grep -c 'whole > 0'` | > 0 |
| `npm --prefix web run typecheck` | sauber |
| `npm --prefix web run test` | 9 Dateien, 137 Tests, alle gruen |
| `git diff --stat -- web/` | leer |
| `gsd-tools windows status` | `"ok": true`, `total_count: 69` |

Die drei Tabellentests wurden einzeln mit `-run '^Name$' -v` gefahren und auf ihre `--- PASS:`-Zeilen sowie auf die **Zahl** der bestandenen Untertests geprueft — nicht nur auf den Exit-Code der Suite. Ein `-run`-Filter, der nichts trifft, endet in Go mit 0, und ein Tabellentest mit leerer Tabelle ist gruen: genau die Eigenschaft, um die es in dieser Luecke geht, und sie darf in ihrer eigenen Verifikation nicht wieder auftreten.

## Decisions Made

- **Keine Beispiel-Hexzahl im statischen Text der Bound-Meldung.** Die drei Tabellenzeilen pruefen `from 0x0000 to 0xFFFFFFFF`, `from 0x0061 to 0xFFFFFFFF` und `from 0x001f to 0x0000` als Teilzeichenketten. Haette die Meldung `0xFFFFFFFF` oder `0x0061` fest im Text, waeren diese Zeilen erfuellt, ohne dass die Meldung je die tatsaechlich gelesenen Grenzen genannt haette — ein Test, der gruen ist, ohne die Eigenschaft zu messen, fuer die er geschrieben ist. Die gefaehrliche Richtung `{ from: 0x0061, to: 0xFFFFFFFF }` mit ihrem gemessenen Wert `{97 -1}` steht deshalb im Kommentar an der Validierung, wo das Abnahmekriterium sie auch verlangt.
- **Die Grenzen werden aus den Quell-Literalen zitiert, nicht aus den geparsten Werten formatiert.** `0x%x` von 0 waere `0x0`; `from 0x%s to 0x%s` mit `m[1]`/`m[2]` gibt `0x0000` und `0xFFFFFFFF` genau so wieder, wie sie in der Datei stehen. Der Leser soll den Eintrag wiederfinden.
- **`whole` wird jetzt einmal vor beiden Leer-Zweigen berechnet und von beiden benutzt.** Es ist die Groesse, die Abschneiden von Leere unterscheidet; solange sie erst danach berechnet wurde, konnte der Leer-Zweig die zweite Ursache strukturell nicht nennen.
- **Die Erreichbarkeit des echten Leer-Falls wurde als Messung belegt statt als siebte Tabellenzeile.** Die Abnahme dieses Plans nagelt die Zeilenzahl der Tabelle bei sechs fest — eine siebte Zeile haette dieses Kriterium verletzt.

## Deviations from Plan

None - plan executed exactly as written.

Die Reihenfolge Rot-vor-gruen, die byteweise Ruecknahme des Lebendbelegs, die Zeilenzahlen 6/3/2 und die Nicht-Behauptung ueber G-02-25 sind wie im Plan beschrieben eingehalten. Es wurde keine bestehende Tabellenzeile entfernt oder umbenannt, kein Import hinzugefuegt, kein Paket installiert und keine Datei unter `web/` netto veraendert.

## Issues Encountered

None.

Zwei temporaere Messdateien (`internal/imagefactory/zz_measure_scratch_test.go`) wurden angelegt, um die Anzahlen `body=6 / whole=6`, das Verhalten der beiden Praefix-Varianten und die Erreichbarkeit beider Diagnose-Zweige an der unexportierten Funktion zu messen. Beide wurden nach der Messung geloescht und nie committet; `git status` ist frei von untracked Dateien.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Die vier von `02-VERIFICATION.md` Runde 5 unter `gaps[0].missing` benannten Korrekturen sind eingebaut und je einzeln rot-vor-gruen gemessen. **Die Feststellung, ob G-02-25 damit geschlossen ist, gehoert der naechsten Verifikationsrunde** — dieser Plan behauptet sie nicht.
- Plan 02-28 dieser Runde besitzt `.planning/WINDOWS.md` und bearbeitet G-02-26 (Ledger-Eintrag 69 und die `coverage`-Zeile D1 von `02-26-SUMMARY.md`, die beide mehr behaupten als gemessen ist) sowie G-02-27 (die vierte Aussage ueber die Refresh-Bedingungen). Der Ledger ist von diesem Plan unberuehrt und steht fuer 02-28 bereit.
- Die naechste freie Threat-Id nach diesem Plan ist **T-02-124** (dieser Plan hat T-02-121, T-02-122, T-02-123 belegt, alle mit Disposition `mitigate` und alle eingeloest).

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-09-04*

## Self-Check: PASSED

Alle in dieser SUMMARY genannten Dateien existieren auf der Platte und alle sechs Task-Commits sind in `git log` auffindbar.
