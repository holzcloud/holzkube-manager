---
phase: 02-transport-seam-talossim-image-factory
plan: 29
subsystem: testing
tags: [drift-guard, gap-closure, error-messages, regexp, go, tdd]

# Dependency graph
requires:
  - phase: 02-transport-seam-talossim-image-factory (Plan 02-27)
    provides: "die geteilte Abschneide-Diagnose, die `whole` als Unterscheider eingefuehrt hat — und damit der Defekt, den dieser Plan schliesst; ausserdem die drei Falsifikationstabellen, deren Zeilen hier erweitert und deren `wantErr`-Form hier vereinheitlicht wird"
  - phase: 02-transport-seam-talossim-image-factory (Plan 02-14)
    provides: "die sechs gemessenen Ablehnungsklassen des Servers, gegen die `TestBrowserRefusalSetEqualsTheServers` vergleicht und die dieser Plan auf keiner Seite anfasst"
provides:
  - "Die Unterscheidung zwischen leerer und abgeschnittener Deklaration laeuft ueber die Form des Rumpfes (`strings.TrimSpace(body)`) und nicht mehr ueber eine Zaehlung ueber die ganze Datei"
  - "Die Abschneide-Meldung zeigt ihren Beleg — sie zitiert den gelesenen Rumpf mit `%q` — statt zu behaupten, die Deklaration sei nicht leer"
  - "`whole` beantwortet genau eine Frage: steht ein eintragsfoermiges Literal ausserhalb der Deklaration"
  - "Ueberlauf (`to > utf8.MaxRune`) und Inversion (`from > to`) sind zwei Zweige mit zwei Ursachen und zwei Texten"
  - "Der Waechter verlangt genau eine Deklaration des bewachten Bezeichners und nennt die Zahl, wenn er mehrere findet"
  - "Keine Tabellenzeile prueft mehr eine Zeichenkette, die jeder Fehlerausgang der Funktion traegt; `wantErr` ist in allen drei Tabellen `[]string`"
  - "`var refusedRangesEntry` und `var refusedRangesDecl` tragen je einen eigenen Doc-Kommentar"
affects: [verification-runde-7, plan-02-30, plan-02-31, ship-gate]

# Actuals (#2632) — estimateTokens-Skala (chars/4 ueber den realisierten Diff), keine Harness-Zahl.
actuals:
  tokens: 5473
  tasks: 3
  commits: 4

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Eine Groesse beantwortet eine Frage: wo eine Zahl zwei Fragen beantwortet, beantwortet sie mindestens eine davon aus Zufall. Die Unterscheidung wird an einer Eigenschaft des Gegenstands festgemacht, nicht an einer Zaehlung ueber seine Umgebung"
    - "Eine Meldung zeigt ihren Beleg, statt ihn zu behaupten: der gelesene Rumpf wird mit `%q` zitiert, damit der Leser sieht, was der Waechter gesehen hat"
    - "Fail-first-Ersatz: laesst sich eine Zeile nicht rot-vor-gruen fahren, weil ihre Eigenschaft heute schon gilt, wird die unterscheidende Teilzeichenkette einmal aus der Meldung entfernt, die Zeile rot gesehen und die Aenderung zurueckgenommen — die Rot-Ausgabe steht in der SUMMARY"
    - "Der Waechter waehlt nicht, er verlangt Eindeutigkeit: mehr als eine Deklaration des bewachten Bezeichners ist ein eigener Fehlerausgang, und er greift vor jeder Pruefung, die seine Ursache ueberschreiben koennte"

key-files:
  created: []
  modified:
    - "internal/imagefactory/guard_drift_test.go — die Rumpfform-Unterscheidung, der geteilte Bound-Zweig, die Duplikatspruefung, `wantErr []string` mit unterscheidenden Teilzeichenketten, vier neue synthetische Quellen mit vier neuen Tabellenzeilen und der geteilte Doc-Kommentarblock"

key-decisions:
  - "Die Abschneide-Meldung traegt KEINE Zahl mehr. `02-REVIEW.md` WR-02 schlaegt eine Fassung vor, die `whole` weiterhin zitiert; sie wurde nicht uebernommen, weil `whole` dann zwei Verwendungsstellen behielte und die Meldung wieder eine Groesse naehme, die eine andere Frage beantwortet. Der zitierte Rumpf ist der bessere Beleg, weil er die Eigenschaft ZEIGT, die die Meldung behauptet. Nebenwirkung: `IN-01` (zwei Fundorte fuer `%d entries`) entfaellt — gemessen, `grep -c '%d entries'` liefert jetzt 1 statt 2"
  - "Der Rest, den `strings.TrimSpace` uebriglaesst — ein Rumpf aus reinem Kommentar nimmt den Abschneide-Zweig, obwohl nichts abgeschnitten wurde —, steht nicht in einer Fussnote, sondern als eigene Tabellenzeile `a body carrying only a comment` und als eigener Absatz im Kommentar an der Verzweigung. Er ist eine bewusste Verhaltensaenderung gegenueber HEAD und gehoert nach Ledger-Eintrag 72, den Plan 02-31 besitzt"
  - "Die Duplikatspruefung greift VOR dem Lesen des Rumpfes. Steht sie danach, ueberschreibt die Rumpfform-Verzweigung ihre Ursache: gemessen liefert eine leere innere Deklaration vor der echten sechs-Eintraege-Tabelle sonst `is declared but carries no entries this guard can read` ueber eine Datei, deren echte Tabelle sechs Eintraege traegt"
  - "Die Inversions-Meldung erwaehnt `utf8.MaxRune` nicht und sagt ausdruecklich `Nothing was lost in conversion here; the two numbers are in the wrong order`. Fuer `{0x001f, 0x0000}` sind beide Grenzen darstellbar; die alte Meldung erklaerte dem Operator eine verlustbehaftete Konvertierung, die dort nicht stattfindet"
  - "Die synthetischen Quellen der Duplikatspruefung tragen die SECHS Bereiche der echten Route in ihrer aeusseren Deklaration und nicht zwei. Der Plan sagt an einer Stelle `zwei Eintraege` und nagelt an anderer Stelle die Rot-Ausgabe `7 in the source as a whole` fest; die zweite Angabe ist die maschinenpruefbare und setzt sechs aeussere Eintraege voraus. Gemessen wurde daraufhin woertlich `1 entries inside the REFUSED_RANGES declaration, 7 in the source as a whole`"

patterns-established:
  - "Ein Doc-Kommentar, den eine Aenderung falsch macht, wird in derselben Aenderung korrigiert und nicht in der naechsten verschoben (siehe Deviation 2)"
  - "Vier neue Tabellenzeilen, vier zitierte Rot-Ausgaben; drei umgestellte `wantErr`-Zeilen, eine zitierte Fail-first-Rot-Ausgabe. Keine Zeile dieses Plans behauptet eine Wirkung, die nicht vorher gemessen wurde"
  - "Die Zahl der bestandenen Untertests wird geprueft (10 / 2 / 3) und nicht nur der Zustand des Elterntests, weil ein Tabellentest mit leerer Tabelle gruen ist"

requirements-completed: []
# ABSICHTLICH LEER, und das ist keine Auslassung. `02-VERIFICATION.md` Runde 6 fuehrt die Phase auf
# `gaps_found`; genau ein vorzeitig gefuelltes `requirements-completed` aus zwei SUMMARYs der Runde 6
# hat die Buchhaltungsregression ausgeloest, die dieselbe Runde als `gaps[1]` gefuehrt hat und die der
# Betreiber mit `b7a1ab9` zuruecknehmen musste. FACT-01 und FACT-06 stehen im `requirements`-Feld des
# Plans, weil dieser Plan an ihrer Naht arbeitet — nicht, weil er sie aufloest. Ob sie aufgeloest
# sind, stellt eine Verifikationsrunde fest, nicht der Ausfuehrende.

# Coverage metadata (#1602)
coverage:
  - id: D1
    description: "Eine wirklich leere `const REFUSED_RANGES ... = []` bekommt die Leer-Meldung, auch wenn ein zweites eintragsfoermiges Literal daneben in der Datei steht"
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/a genuinely empty declaration beside a second table"
        status: pass
      - kind: other
        ref: "grep -Fc 'strings.TrimSpace(body)' internal/imagefactory/guard_drift_test.go != 0"
        status: pass
    human_judgment: false
  - id: D2
    description: "Die Abschneide-Meldung zitiert den gelesenen Rumpf mit `%q` und traegt keine Dateizaehlung mehr; der Rest, den `TrimSpace` uebriglaesst, ist als eigene Zeile festgenagelt"
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/a body carrying only a comment"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/the body cut short by a bracket in a comment"
        status: pass
    human_judgment: false
  - id: D3
    description: "`whole` beantwortet genau eine Frage; die Zaehlpruefung `whole != len(matches)` ist woertlich erhalten und bleibt gruen"
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/an entry-shaped literal outside the declaration"
        status: pass
      - kind: other
        ref: "grep -c '%d entries' internal/imagefactory/guard_drift_test.go == 1 (vorher 2)"
        status: pass
    human_judgment: false
  - id: D4
    description: "Ueberlauf und Inversion sind zwei Zweige mit zwei Ursachen; die Inversions-Meldung erwaehnt `utf8.MaxRune` nicht"
    verification:
      - kind: unit
        ref: "go test ./internal/imagefactory -run '^TestBrowserRefusalGuardRefusesABoundItCannotRepresent$' -count=1 -v | grep -c '^    --- PASS: ' == 3"
        status: pass
      - kind: other
        ref: "grep -n 'utf8.MaxRune || from > to' internal/imagefactory/guard_drift_test.go — keine Treffer"
        status: pass
    human_judgment: false
  - id: D5
    description: "Keine Tabellenzeile prueft mehr eine Zeichenkette, die jeder Fehlerausgang der Funktion traegt; `wantErr` ist in allen drei Tabellen `[]string`"
    verification:
      - kind: other
        ref: "grep -n 'wantErr: []string{\"REFUSED_RANGES\"}' internal/imagefactory/guard_drift_test.go — keine Treffer"
        status: pass
      - kind: unit
        ref: "Fail-first-Ersatz: unterscheidende Teilzeichenkette einmal aus dem Kein-Anker-Ausgang entfernt -> genau die drei Zeilen renamed/absent/prefixed rot, alle uebrigen sieben gruen; Aenderung zurueckgenommen"
        status: pass
    human_judgment: false
  - id: D6
    description: "Der Waechter verlangt genau eine Deklaration des bewachten Bezeichners und nennt die Zahl, wenn er mehrere findet; die echte `images.tsx` traegt genau eine und wird nicht falsch-rot"
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/two declarations, an indented inner one first"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/an empty inner declaration before the real one"
        status: pass
      - kind: integration
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalSetEqualsTheServers"
        status: pass
    human_judgment: false
  - id: D7
    description: "`go doc` zeigt die Begruendung der Anker-Entscheidung unter dem Symbol, das sie betrifft: `var refusedRangesDecl` und `var refusedRangesEntry` tragen je einen eigenen Doc-Kommentar"
    verification:
      - kind: other
        ref: "awk '/^var refusedRangesDecl = regexp.MustCompile\\(/{print prev} {prev=$0}' internal/imagefactory/guard_drift_test.go | grep -q '^//'"
        status: pass
      - kind: other
        ref: "awk '/^var refusedRangesEntry = regexp.MustCompile\\(/{print prev} {prev=$0}' internal/imagefactory/guard_drift_test.go | grep -q '^//'"
        status: pass
    human_judgment: true
    rationale: "Die beiden Pruefungen stellen fest, dass VOR jeder Deklaration eine Kommentarzeile steht — nicht, dass die richtige Haelfte des geteilten Blocks vor der richtigen Deklaration gelandet ist. Ob die Begruendung des `:`-gebundenen Ankers wirklich unter `refusedRangesDecl` steht und die Begruendung des flachen Eintrags-Literals unter `refusedRangesEntry`, ist eine Lesart und kein Testergebnis. Ein Verifier soll `go doc` bzw. die beiden Bloecke nachlesen; genau das Uebernehmen hat WR-05 zwei Runden ueberleben lassen."

# Metrics
duration: 9 min
completed: 2026-09-05
status: complete
---

# Phase 02 Plan 29: Der Waechter nennt die Ursache, die er festgestellt hat Summary

**Die Unterscheidung zwischen leerer und abgeschnittener `REFUSED_RANGES`-Deklaration laeuft jetzt ueber `strings.TrimSpace(body)` statt ueber eine Zaehlung ueber die ganze Datei; Ueberlauf und Inversion sind zwei Bound-Zweige mit zwei Ursachen; und der Waechter verlangt genau eine Deklaration, statt schweigend die erste zu nehmen.**

## Performance

- **Duration:** 9 min
- **Started:** 2026-09-05T08:30:17Z
- **Completed:** 2026-09-05T08:39:44Z
- **Tasks:** 3
- **Files modified:** 1

## Accomplishments

- **G-02-28, der Spiegel-Defekt der Runde 6, ist geschlossen.** `whole` beantwortet nur noch die Frage nach dem Literal ausserhalb der Deklaration; die Leer/Abschneide-Unterscheidung haengt an einer Eigenschaft der Deklaration selbst. Der Satz *"The declaration is NOT empty"*, der aus einer Dateizaehlung erschlossen war, ist mit der Zaehlung verschwunden, aus der er erschlossen war.
- Die Abschneide-Meldung **zeigt** ihren Beleg: sie zitiert den gelesenen Rumpf mit `%q`. Nebenwirkung, gemessen: `IN-01` (zwei Fundorte fuer `%d entries`) entfaellt — `grep -c '%d entries'` liefert 1 statt 2.
- **WR-03:** `to > utf8.MaxRune` und `from > to` sind zwei Zweige. Die Inversions-Meldung erwaehnt `utf8.MaxRune` nicht und sagt ausdruecklich, dass nichts in der Konvertierung verlorenging.
- **WR-04:** die drei Zeilen mit `wantErr: "REFUSED_RANGES"` pinnen je eine unterscheidende Teilzeichenkette des Kein-Anker-Ausgangs; `wantErr` ist in allen drei Tabellen `[]string`.
- **WR-05:** der 49-zeilige Kommentarblock ist geteilt; beide Regexp-Deklarationen tragen ihren eigenen Doc-Kommentar.
- **Ein siebter Fall, beim Planen gefunden:** der Waechter nahm die erste von mehreren Deklarationen ohne zu fragen, ob es eine zweite gibt. Er stellt die Zahl jetzt fest und macht mehr als eine zu einem eigenen Fehlerausgang, der vor dem Lesen des Rumpfes greift.
- Die Funktion hat jetzt **zehn** Fehlerausgaenge, und jeder nennt die Ursache, die er festgestellt hat. Keine wird aus einer Zaehlung erschlossen.

## Task Commits

1. **Task 1: Die Form des Rumpfes entscheidet, ob die Deklaration leer ist** — `de04482` (fix, tracer)
2. **Task 2: Eine Bedingung, eine Ursache, eine Meldung** — `58dd247` (fix)
3. **Task 3: Der Waechter zaehlt die Deklarationen, und der Doc-Kommentar steht unter seinem Symbol** — `abdd918` (fix)

**Plan metadata:** siehe `docs(02-29)`-Commit.

## Files Created/Modified

- `internal/imagefactory/guard_drift_test.go` — 252 Einfuegungen, 68 Loeschungen. Rumpfform-Unterscheidung, geteilter Bound-Zweig, Duplikatspruefung, `wantErr []string`, vier neue synthetische Quellen (`emptyBesideASecondTable`, `commentOnlyBody`, `shadowedByAnInnerTable`, `shadowedByAnEmptyInnerTable`), vier neue Tabellenzeilen, geteilter Doc-Kommentarblock.

Unter `web/` wurde nichts geaendert — weder netto noch voruebergehend. `git diff --stat -- web/` war ueber den ganzen Plan leer und ist es am Ende.

## Messungen

### Rot-vor-gruen: die vier neuen Tabellenzeilen

**`a genuinely empty declaration beside a second table`** (gegen den unveraenderten Kontrollfluss von HEAD `a42387d`):

```
error does not name "REFUSED_RANGES is declared but carries no entries this guard can read":
    the REFUSED_RANGES declaration body was cut short before its first entry, but 1 entries
    are present in the source as a whole.
    The declaration is NOT empty. The body is captured non-greedily up to the first closing
    bracket, so a `]` inside a comment or a string before the first entry ends the capture
    there and nothing readable is left inside it. Look for that bracket, not for a missing table
```

Die Deklaration ist woertlich `= []`. Nichts ist abgeschnitten. Die Meldung behauptet, sie sei nicht leer, und schickt den Leser nach einer Klammer, die es nicht gibt.

**`a body carrying only a comment`** (gegen denselben Stand):

```
error does not name "cut short before its first entry":
    REFUSED_RANGES is declared but carries no entries this guard can read.
    This is the declaration being empty, not the declaration being absent; the two are
    separate failures because they call for separate fixes
error does not name "\"\\n  // nothing yet\\n\"":
    (dieselbe Meldung)
```

**`two declarations, an indented inner one first`** (gegen den Stand nach Task 1 und 2, also mit Rumpfform-Unterscheidung, aber ohne Duplikatspruefung):

```
error does not name "2 declarations of REFUSED_RANGES in this source":
    1 entries inside the REFUSED_RANGES declaration, 7 in the source as a whole.
    Either an entry-shaped literal sits outside the declaration -- this guard used to fold
    those into the browser's set and now ignores them, which is correct and still worth a
    look -- or the declaration body was cut short because an entry carries a closing bracket
    inside a string
```

Fail closed, aber mit der Ursache eines anderen Defekts: der Leser sucht ein Streu-Literal ausserhalb der Deklaration, waehrend die echte Tabelle ungelesen darunter steht.

**`an empty inner declaration before the real one`** (gegen denselben Stand):

```
error does not name "2 declarations of REFUSED_RANGES in this source":
    REFUSED_RANGES is declared but carries no entries this guard can read.
    This is the declaration being empty, not the declaration being absent; the two are
    separate failures because they call for separate fixes
```

Diese falsche Ursache ist **erst durch Task 1 dieses Plans entstanden** — vorher lieferte dieselbe Quelle die Abschneide-Meldung. Das ist der Grund, warum die Duplikatspruefung in dieselbe Runde gehoert wie die Rumpfform-Korrektur und nicht in die naechste.

### Fail-first-Ersatz fuer die drei umgestellten `wantErr`-Zeilen

Die Zeilen `the declaration renamed out of existence`, `the declaration absent` und `the declaration renamed to a prefixed name` pinnen eine Eigenschaft, die heute schon gilt; sie koennen nicht rot-vor-gruen laufen. Vorgeschriebener Ersatz: die unterscheidende Teilzeichenkette einmal aus dem Kein-Anker-Ausgang entfernen (`"no %s declared as an array literal"` -> `"%s is missing"`, `"A name that merely carries %s as a prefix"` -> `"A prefixed leftover of %s"`), fahren, zurueckziehen.

```
--- FAIL: .../the_declaration_renamed_out_of_existence
    error does not name "no REFUSED_RANGES declared as an array literal":
        REFUSED_RANGES is missing.
--- FAIL: .../the_declaration_absent
    error does not name "no REFUSED_RANGES declared as an array literal":
        REFUSED_RANGES is missing.
--- FAIL: .../the_declaration_renamed_to_a_prefixed_name
    error does not name "no REFUSED_RANGES declared as an array literal": ...
    error does not name "A name that merely carries REFUSED_RANGES as a prefix": ...
```

Genau diese drei Zeilen wurden rot, die uebrigen fuenf der damals achtzeiligen Tabelle blieben gruen. **Die Aenderung wurde zurueckgenommen** — belegt durch `grep -c 'no %s declared as an array literal'` == 1 und `grep -c 'A name that merely carries %s as a prefix'` == 1 im Endstand, und durch `grep -c 'is missing'` == 0.

### Rot-vor-gruen: die Inversions-Zeile (WR-03)

Gegen die ungeteilte Bedingung `to > utf8.MaxRune || from > to`:

```
--- FAIL: .../an_inverted_range
    error does not name "is inverted":
        this entry of REFUSED_RANGES has bounds this guard cannot represent: from 0x001f to 0x0000.
        The upper bound has to be at most utf8.MaxRune and the lower bound at most the upper.
        ... rune(uint64) is lossy: a bound above utf8.MaxRune lands on a negative rune ...
```

Beide Grenzen von `{0x001f, 0x0000}` sind darstellbar. Die alte Meldung erklaerte eine verlustbehaftete Konvertierung, die dort nicht stattfindet. Die beiden Ueberlauf-Zeilen liefen im selben Lauf ebenfalls rot, weil ihre Erwartung von `"cannot represent"` auf die praezisere Formulierung `"has an upper bound this guard cannot represent"` gehoben wurde.

### Gegen die echte `web/src/routes/images.tsx`

Gemessen mit einem eigenstaendigen Go-Programm, das die drei Ausdruecke (`refusedRangesDecl`, `refusedRangesEntry`, `browserRefusalRange`) woertlich aus der Testdatei uebernimmt — vor und nach den Aenderungen identisch:

| Groesse | Wert |
|---|---|
| Deklarationen, die `refusedRangesDecl` findet | **1** |
| Treffer im Deklarationsrumpf | **6** |
| Treffer in der ganzen Datei | **6** |

Die Duplikatspruefung ist an der echten Datei also nicht falsch-rot, und Rumpf- wie Dateizahl sind unveraendert 6.

### Zustand der drei Tabellen

| Tabelle | Untertests | `--- FAIL:` |
|---|---|---|
| `TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration` | **10** PASS | 0 |
| `TestBrowserRefusalGuardRefusesAnEntryItCannotRead` | **2** PASS | 0 |
| `TestBrowserRefusalGuardRefusesABoundItCannotRepresent` | **3** PASS | 0 |

Keine bestehende Zeile wurde entfernt oder umbenannt. Die drei Regressionssperren der Vorrunden sind woertlich erhalten und weiterhin gruen: `the declaration renamed to a prefixed name` (praefixierte Umbenennung), `the body cut short by a bracket in a comment` (Deklaration hinter `//`), `an upper bound outside Unicode` (Grenze oberhalb `utf8.MaxRune`).

`TestBrowserRefusalSetEqualsTheServers` bleibt gruen; die Ablehnungsmenge wurde auf keiner der beiden Seiten angefasst.

### Verifikation auf Plan-Ebene

| Kommando | Ergebnis |
|---|---|
| `go build ./... && go vet ./...` | sauber |
| `go test ./internal/imagefactory/... -count=1` | `ok ... 2.698s` |
| `go test ./... -count=1 -race` | alle Pakete `ok`, kein `FAIL` |
| `~/go/bin/golangci-lint run` | `0 issues.`, Exit 0 |
| `gofmt -l ./internal` | keine Ausgabe |
| `git diff --stat -- web/` | leer |
| `gsd-tools windows status` | `"ok": true`, `total_count: 71` — unveraendert |

## Decisions Made

Siehe `key-decisions` im Frontmatter. Der Kern in einem Satz: die unterscheidende Groesse ist immer eine Eigenschaft des Gegenstands, ueber den die Meldung spricht, und nie eine Zaehlung ueber seine Umgebung.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Die synthetischen Quellen der Duplikatspruefung tragen sechs statt zwei aeussere Eintraege**

- **Found during:** Task 3
- **Issue:** Der Plan widerspricht sich. Der `<action>`-Text sagt "darunter die echte, aeussere Deklaration mit zwei Eintraegen"; das `<acceptance_criteria>` desselben Tasks verlangt, dass die SUMMARY die Rot-Ausgabe `1 entries inside the REFUSED_RANGES declaration, 7 in the source as a whole` zitiert. Mit zwei aeusseren Eintraegen betraegt die Zahl 3, nicht 7 — die geforderte Ausgabe ist mit der geforderten Quelle nicht herstellbar. Die 7 stammt aus der Planungsmessung, die eine innere Deklaration vor die **echte** sechs-Eintraege-Tabelle gestellt hat.
- **Fix:** Die aeussere Deklaration beider neuer Quellen traegt die sechs Bereiche der echten Route. Damit ist die zitierte Rot-Ausgabe woertlich reproduziert und die Quelle entspricht zugleich der Formulierung der Planungsmessung ("eine eingerueckte innere Deklaration vor der echten").
- **Files modified:** `internal/imagefactory/guard_drift_test.go`
- **Verification:** Die gemessene Rot-Ausgabe lautet woertlich `1 entries inside the REFUSED_RANGES declaration, 7 in the source as a whole.` (siehe oben unter Messungen).
- **Committed in:** `abdd918`

**2. [Rule 1 - Bug] Ein Doc-Kommentar-Absatz wurde durch Task 1 falsch und in Task 1 korrigiert statt in Task 3 unveraendert verschoben**

- **Found during:** Task 1
- **Issue:** Der Absatz im Doc-Block, der mit *"That sentence was not true for a body cut short BEFORE its first entry"* beginnt, schloss mit *"The whole-source count is now taken before that branch, so the cut-short case carries its own message and the count check keeps the one it can still explain."* Nach der Rumpfform-Korrektur wird die Zaehlung nicht mehr vor dem Zweig genommen; der Satz beschrieb einen Kontrollfluss, den es nicht mehr gibt. Task 3 sieht vor, beide Haelften des Blocks "inhaltlich unveraendert" zu verschieben — das haette einen falschen Satz konserviert und woertlich die Gattung Defekt reproduziert, die dieser Plan beseitigt.
- **Fix:** Der Absatz nennt jetzt die Rumpfform als unterscheidende Groesse und sagt ausdruecklich, dass Runde 6 dafuer die Dateizaehlung geliehen hat und warum das der Defekt war. Task 3 hat die Haelften danach unveraendert verschoben.
- **Files modified:** `internal/imagefactory/guard_drift_test.go`
- **Verification:** `go build`, `go vet`, `golangci-lint run` sauber; der Absatz steht im Endstand vor `var refusedRangesDecl` und beschreibt den tatsaechlichen Kontrollfluss.
- **Committed in:** `de04482`

**3. [Rule 2 - Missing Critical] Die Abschneide-Meldung von WR-02 wurde ohne die Dateizaehlung uebernommen**

- **Found during:** Task 1
- **Issue:** Die in `02-REVIEW.md` WR-02 vorgeschlagene Fassung der Abschneide-Meldung zitiert `whole` weiterhin (`"while the source as a whole carries %d"`). Damit haette `whole` zwei Verwendungsstellen behalten und die Meldung erneut eine Groesse gefuehrt, die eine andere Frage beantwortet — genau die Eigenschaft, die dieser Plan entfernt. Die `must_haves`-Wahrheit des Plans verlangt dagegen, dass `whole` nur noch EINE Frage beantwortet.
- **Fix:** Die Abschneide-Meldung traegt keine Zahl. Sie zitiert den Rumpf mit `%q` und sagt, dass er Text traegt, aus dem kein Eintrag lesbar ist. `whole` wird erst unmittelbar vor der Zaehlpruefung berechnet.
- **Files modified:** `internal/imagefactory/guard_drift_test.go`
- **Verification:** `grep -c '%d entries'` liefert 1 (vorher 2) — die Nebenwirkung, die `IN-01` erledigt.
- **Committed in:** `de04482`

---

**Total deviations:** 3 auto-fixed (2 Bug, 1 Missing Critical)
**Impact on plan:** Alle drei dienen der Zusicherung, die der Plan selbst aufstellt — eine Meldung nennt die Ursache, die sie festgestellt hat. Kein Scope Creep: keine Datei ausserhalb `internal/imagefactory/guard_drift_test.go` wurde angefasst.

## Issues Encountered

Keine. Alle drei Tasks liefen rot-vor-gruen wie geplant, und jede vorhergesagte Rot-Ausgabe trat woertlich so ein, wie der Plan sie beim Planen gemessen hatte.

## Was dieser Plan ausdruecklich NICHT behauptet

- **G-02-29 (die Anker-Blindheit) ist NICHT geschlossen** und wurde von diesem Plan nicht beruehrt. Der Anker bindet weiterhin an ein Literal statt an einen Wert; `].filter(...)`, `...PLATFORM_RANGES` und `].slice(...)` bleiben gruen auf echter Drift. Das gehoert Plan 02-30 und 02-31 und haengt an der noch nicht ratifizierten Richtungsentscheidung.
- **G-02-9 bleibt offen.** Keine Zeile dieses Plans sagt etwas anderes.
- **Ob G-02-28 geschlossen ist, stellt eine Verifikationsrunde fest**, nicht diese SUMMARY. Hier steht, was gemessen wurde.
- `.planning/WINDOWS.md` und `.planning/REQUIREMENTS.md` wurden **nicht** geschrieben; beide gehoeren Plan 02-31. Der Ledger steht unveraendert bei `total_count: 71`, die Eintraege 56, 58, 66 und 70 sind unberuehrt.
- `IN-02`, `IN-03` und `IN-04` aus dem Runde-6-Review wurden **nicht** angefasst; sie stehen unter der Policy-Entscheidung des Betreibers (`02-VERIFICATION.md` Human-Verification-Punkt 6). `IN-01` entfaellt als gemessene Nebenwirkung, was die Liste des Betreibers um einen Punkt verkuerzt.

## Known Stubs

Keine. Dieser Plan legt keinen Platzhalter, keinen `TODO`, keinen `t.Skip` und keinen unausgefuehrten `<verify>` an.

**Uebergabe an Plan 02-31 fuer Ledger-Eintrag 72:** der Rest, den `strings.TrimSpace` uebriglaesst — ein Rumpf aus reinem Kommentar oder Leerraum-mit-Zeichen nimmt den Abschneide-Zweig, obwohl nichts abgeschnitten wurde. Er ist kein Stub, sondern eine bewusst in Kauf genommene, von einer eigenen Tabellenzeile festgenagelte Verhaltensaenderung, und er wird hier genannt, damit Plan 02-31 ihn nicht suchen muss.

## User Setup Required

Keine — dieser Plan aendert eine Testdatei und spricht mit keinem externen Dienst.

## Next Phase Readiness

- **Bereit fuer Plan 02-30.** Dieser Plan war die richtungsunabhaengige Welle 1 der Runde 7; er haelt unveraendert, gleich ob der Betreiber Variante B (Empfehlung) oder die punktgleiche Variante A ratifiziert. Plan 02-30 oeffnet mit dem blockierenden Entscheidungs-Checkpoint und ist `autonomous: false`.
- **Bereit fuer Plan 02-31**, der die Register fuehrt: `.planning/WINDOWS.md` (Eintrag 72, samt der oben uebergebenen `TrimSpace`-Restmenge) und `.planning/REQUIREMENTS.md` (Statuszeile aller vierzehn Phasen-Requirements, darunter die falsche von TRANS-06).
- **Offen und diesem Plan nicht zugeordnet:** die noch nicht ratifizierte Richtungsentscheidung in `02-DECISION-drift-guard-lexik.md`. Der Betreiber entscheidet sie mit einem Satz; nur Plan 02-30 haengt daran.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-09-05*
