---
phase: 02-transport-seam-talossim-image-factory
plan: 26
subsystem: testing
tags: [go, drift-guard, regexp-anchoring, image-factory, typescript-seam, broken-windows]

# Dependency graph
requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "Plan 02-20 baute `browserRefusalRange` und `TestBrowserRefusalSetEqualsTheServers`; dieser Plan verankert den Waechter, den jener Plan unverankert liess"
  - phase: 02-transport-seam-talossim-image-factory
    provides: "Plan 02-25 uebergab in seiner SUMMARY unter `## Ledger entries to file` drei fertig formulierte Ledger-Eintraege; dieser Plan besitzt `.planning/WINDOWS.md` fuer die Runde und legt sie an"
provides:
  - "`parseBrowserRefusalRanges(source string) ([]declaredRange, error)` — die reine, ohne `*testing.T` pruefbare Lesefunktion mit drei Fehlerfaellen statt keinem"
  - "`refusedRangesDecl` — der auf `const REFUSED_RANGES` verankerte Deklarations-Ausdruck (Zeilenanfang, optionales `export`, `regexp.QuoteMeta` um den Bezeichner, Zuweisungs-Verankerung wegen der Typannotation)"
  - "`refusedRangesEntry` — der Ausdruck, der jedes flache `{ … }`-Literal des Rumpfes einsammelt, damit ein unlesbarer Eintrag ein Fehlschlag und kein Ueberspringen ist"
  - "`TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration` — vier Tabellenzeilen, darunter die Falsifikation des Verifiers aus Runde 4 mit umgekehrtem Ergebnis"
  - "`TestBrowserRefusalGuardRefusesAnEntryItCannotRead` — zwei Tabellenzeilen fuer den Dezimal- und den Konstantenfall, beide mit inhaltlicher Erwartung an die Fehlermeldung"
  - "Der Ledger der Runde in `.planning/WINDOWS.md`: Eintraege 66, 67, 68 (die Uebergabe von 02-25) und 69 (der unverankerte Waechter dieser Runde)"
affects: [phase-03-talos-transport, image-factory-ui, drift-guards]

# Actuals (#2632)
actuals:
  tokens: 7650
  tasks: 3
  commits: 5

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Ein Drift-Waechter, der eine Quelldatei liest, verankert auf der Deklaration und nicht auf der Datei: `(?ms)^\\s*(?:export\\s+)?const\\s+` + `regexp.QuoteMeta(NAME)` + `\\s*[^=\\n]*=\\s*\\[(.*?)\\]` — Zeilenanfangs-Anker von `budget_drift_test.go:89-90`, Zuweisungs-Verankerung von `stringArrayLiteral`"
    - "Die Pruefung eines Waechters als reine Funktion mit Fehlerrueckgabe herausloesen, damit ihre Fehlerfaelle selbst Tabellentests bekommen koennen; `t.Fatalf` bleibt an der duennen Aufrufstelle und traegt nur noch den Dateinamen"
    - "Die Trefferzahl im geschnittenen Rumpf gegen die Trefferzahl in der ganzen Quelle vergleichen: eine Abweichung faengt die Verschmutzung (Literal ausserhalb) und das Abschneiden (Klammer in einer Zeichenkette) in einer Pruefung, die beide moeglichen Ursachen nennt"
    - "Zwei getrennte Tabellentests statt eines gemeinsamen, wenn die Abnahme die Zeilenzahl jedes Tests festnagelt — eine Zahl, die nur haltbar ist, solange die Tabellen getrennt bleiben"

key-files:
  created: []
  modified:
    - internal/imagefactory/guard_drift_test.go
    - .planning/WINDOWS.md

key-decisions:
  - "Der `t.Fatalf`-Text von `browserRefusalRanges` ist in den Fehler der reinen Funktion gewandert und wird an der Aufrufstelle nicht wiederholt. Der erste Lebendbeleg-Lauf druckte den Satz zweimal — einmal aus der Funktion, einmal aus dem Wrapper —, und ein Waechter, der seine Begruendung doppelt ausgibt, sagt sie an einer Stelle, an der sie niemand pruefen kann. Sie steht jetzt dort, wo ein Test sie liest."
  - "Der Anker benutzt `[^=\\n]*` und nicht `[^=]*` fuer die Typannotation. Die Annotation steht auf derselben Zeile wie der Name; ein zeilenuebergreifendes `[^=]*` koennte unter `(?s)` an einem spaeteren `=` in der Datei andocken und die Verankerung wieder aufweichen."
  - "Die Reihenfolge in `parseBrowserRefusalRanges` ist: Deklaration schneiden, jedes Literal lesbar, Deklaration nicht leer, Anzahlen vergleichen. Der Lesbarkeits-Test sitzt vor der Zaehlpruefung, weil ein unlesbarer Eintrag *innerhalb* der Deklaration die Zaehlpruefung ebenfalls ausloesen wuerde — mit der falschen Erklaerung, naemlich einem Literal ausserhalb."
  - "Die von 02-25 uebergebene Ergaenzung zu Eintrag 58 und der vom Plan geforderte 'ueberholende Eintrag neben 58' sind **ein** Eintrag (66) und nicht zwei. Zwei Eintraege mit derselben Aussage ueber 58 waeren genau der Schaden, den die Disziplin des Registers verhindert: jede Behauptung steht einmal da. Der Text traegt 02-25s Wortlaut woertlich und bekommt die Grossbuchstaben-Eroeffnung von 51/52/63 davor."

patterns-established:
  - "Verankerung auf der Deklaration: ein Waechter, der eine fremdsprachige Quelle liest, schneidet erst die benannte Deklaration heraus und liest dann nur darin — ein Literal, das irgendwo in der Datei vorkommt, darf ihn nicht erfuellen"
  - "Fehlerfaelle eines Waechters werden selbst getestet: die Pruefung wird pur, die Tabelle laeuft ueber kurze synthetische Quellen, und eine Zeile bindet die Tabelle an die echte Datei, damit die anderen Zeilen nicht nur zueinander konsistent sind"
  - "Eine gemessene Falsifikation wird als Testzeile kodiert und die gemessene Ausgabe im Kommentar der Zeile genannt, damit ihr Zweck beim naechsten Lesen nicht verlorengeht"

requirements-completed: [FACT-01, FACT-06]

# Coverage metadata (#1602)
coverage:
  - id: D1
    description: "Der Waechter liest die Browser-Ablehnungsmenge ausschliesslich aus der `REFUSED_RANGES`-Deklaration und scheitert, wenn sie fehlt, umbenannt oder verschoben ist"
    requirement: "FACT-06"
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/the_declaration_renamed_out_of_existence"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/the_declaration_absent"
        status: pass
      - kind: manual_procedural
        ref: "Lebendbeleg: REFUSED_RANGES im echten Baum umbenannt, TestBrowserRefusalSetEqualsTheServers gefahren, FAIL beobachtet, Datei byteweise wiederhergestellt (sha256 identisch, git diff leer)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Ein eintragsfoermiges Literal ausserhalb der Deklaration wird nicht mehr in die Menge gefaltet und faellt als Abweichung der beiden Anzahlen auf"
    requirement: "FACT-06"
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/an_entry-shaped_literal_outside_the_declaration"
        status: pass
    human_judgment: false
  - id: D3
    description: "Ein Eintrag innerhalb der Deklaration, den der Waechter nicht lesen kann, ist ein benannter Fehlschlag mit dem Literal im Text — kein stilles Ueberspringen"
    requirement: "FACT-06"
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesAnEntryItCannotRead/an_entry_written_in_decimal"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalGuardRefusesAnEntryItCannotRead/an_entry_whose_bounds_are_named_constants"
        status: pass
    human_judgment: false
  - id: D4
    description: "`TestBrowserRefusalSetEqualsTheServers` misst weiterhin dasselbe und bleibt gruen; die Ablehnungsmenge wurde auf keiner der beiden Seiten angefasst"
    requirement: "FACT-01"
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalSetEqualsTheServers"
        status: pass
      - kind: other
        ref: "git diff --stat -- web/ (leer) und der unveraenderte sha256 von web/src/routes/images.tsx"
        status: pass
    human_judgment: false
  - id: D5
    description: "Der Ledger traegt die Eintraege beider Plaene der Runde und seine drei Repraesentationen stimmen ueberein"
    verification:
      - kind: integration
        ref: "gsd-tools windows status -> \"ok\": true"
        status: pass
      - kind: other
        ref: "unabhaengiger Parse der drei Repraesentationen (Frontmatter / Markdown-Tabelle / JSON-Block): 54 open, 0 waived, 15 fixed, 69 total, ids 1..69 zusammenhaengend und eindeutig"
        status: pass
    human_judgment: false

# Metrics
duration: 10 min
completed: 2026-09-04
status: complete
---

# Phase 02 Plan 26: Der verankerte Browser-Ablehnungs-Waechter Summary

**`browserRefusalRange` laeuft nicht mehr ueber die ganze `images.tsx`, sondern nur noch im herausgeschnittenen `REFUSED_RANGES`-Rumpf — und die Falsifikation, mit der der Verifier ihn in Runde 4 als `ok ... 0.546s` gruen gegen eine geloeschte Deklaration gemessen hat, laeuft seither als Tabellenzeile mit umgekehrtem Ergebnis.**

## Performance

- **Duration:** 10 min
- **Started:** 2026-09-04T05:53:19Z
- **Completed:** 2026-09-04T06:03:21Z
- **Tasks:** 3
- **Files modified:** 2

## Accomplishments

- **Die Lesefunktion ist pur und ihre Fehlerfaelle sind selbst getestet.** `parseBrowserRefusalRanges(source string) ([]declaredRange, error)` nimmt kein `*testing.T` mehr entgegen. Das ist der Kern der Luecke: mit `t.Fatalf` im Inneren war das Scheitern des Waechters die einzige Aussage, die nichts prueft — genau die Klasse Defekt, die hier geschlossen wird. `browserRefusalRanges(t)` bleibt bestehen, liest die Datei und ist auf einen `t.Fatalf` mit dem Dateinamen geschrumpft.
- **Der Anker.** `refusedRangesDecl` verankert auf Zeilenanfang, optionalem `export`, `const` und dem mit `regexp.QuoteMeta` gequoteten Bezeichner `REFUSED_RANGES`, laesst eine Typannotation ohne `=` passieren und faengt den Rumpf bis zur schliessenden Klammer nicht-gierig ein. Zwei Vorlagen, eine Eigenschaft je: der Zeilenanfangs-Anker und seine Begruendung von `internal/httpapi/handlers/budget_drift_test.go:89-90` ("so a number that merely appears somewhere in the file cannot satisfy this"), die Zuweisungs-Verankerung von `stringArrayLiteral` — die Typannotation `readonly RefusedRange[]` traegt selbst ein Klammerpaar und wuerde sonst als leeres Array gelesen.
- **Drei Fehlerfaelle statt keinem.** Deklaration fehlt; Deklaration ist da, aber leer; Trefferzahl im Rumpf ungleich Trefferzahl in der ganzen Quelle. Der dritte nennt beide Zahlen und beide Ursachen, zwischen denen der Waechter nicht entscheiden kann — ein eintragsfoermiges Literal ausserhalb der Deklaration, oder ein Rumpf, der vor dem Ende abgeschnitten wurde, weil ein Eintrag eine schliessende Klammer in einer Zeichenkette traegt.
- **Ein unlesbarer Eintrag ist ein Fehlschlag.** `refusedRangesEntry` sammelt jedes flache `{ … }`-Literal des Rumpfes ein; passt `browserRefusalRange` nicht darauf, zitiert der Fehler das Literal woertlich. Dieselbe Strenge, die `stringArrayLiteral` und `exportedWarningCodes` bereits anwenden und die `02-REVIEW.md` WR-04 namentlich als Praezedenz nennt.
- **Der Ledger fuer beide Plaene der Runde.** Vier Eintraege, ausschliesslich ueber `gsd-tools windows`: die drei von Plan 02-25 uebergebenen und der Eintrag dieser Runde ueber den unverankerten Waechter.

## Task Commits

1. **Task 1 RED: der Waechter scheitert noch nicht ohne seine Deklaration** — `c97efbd` (test)
2. **Task 1 GREEN: der Waechter liest nur noch aus der REFUSED_RANGES-Deklaration** — `fe73418` (feat)
3. **Task 2 RED: ein unlesbarer Eintrag wird noch still uebersprungen** — `9a54b5e` (test)
4. **Task 2 GREEN: ein Eintrag, den der Waechter nicht lesen kann, ist ein Fehlschlag** — `8ea6dbd` (feat)
5. **Task 3: der Ledger traegt die Eintraege beider Plaene dieser Runde** — `4adae38` (docs)

Kein REFACTOR-Commit: die GREEN-Fassungen brauchten keine Aufraeumung, die Tests noch bestehen lassen musste.

## Files Created/Modified

- `internal/imagefactory/guard_drift_test.go` — die verankerte Lesefunktion, ihre drei Fehlerfaelle, der Literal-Lesbarkeitstest und die beiden Tabellentests (+278/-14 Zeilen)
- `.planning/WINDOWS.md` — vier Ledger-Eintraege der Runde, ausschliesslich ueber `gsd-tools windows append` / `windows fixed` geschrieben

## Die gemessenen Anzahlen

Der Zaehlvergleich, den Task 1 eingebaut hat, vergleicht die Treffer von `browserRefusalRange` im geschnittenen Deklarationsrumpf mit den Treffern in der ganzen Quelle. **Heute gemessen: sechs und sechs.** Die Deklaration enthaelt sechs flache `{ … }`-Literale, alle sechs sind von `browserRefusalRange` lesbar, und es gibt in `web/src/routes/images.tsx` kein siebtes eintragsfoermiges Literal ausserhalb der Deklaration. Der Waechter faellt also heute in keinen seiner drei neuen Fehlerfaelle — was zu erwarten war, weil `02-VERIFICATION.md` Runde 4 ausdruecklich feststellt, dass nichts maskiert ist. Es ging um sein kuenftiges Verhalten.

## Der Lebendbeleg

Derselbe Versuch, mit dem der Verifier den Waechter falsifiziert hat, am echten Baum wiederholt — mit umgekehrtem Ergebnis.

```
sha256 vorher:   ca13cf5f5bf2abc1f1e4c9376355363f70d232305ef7beaf9716a42f618ee505
perl -pi -e 's/REFUSED_RANGES/RENAMED_BY_VERIFIER/g' web/src/routes/images.tsx
Vorkommen von RENAMED_BY_VERIFIER: 3

go test ./internal/imagefactory -run '^TestBrowserRefusalSetEqualsTheServers$' -count=1
--- FAIL: TestBrowserRefusalSetEqualsTheServers (0.00s)
    guard_drift_test.go:90: ../../web/src/routes/images.tsx: no REFUSED_RANGES declared as an array literal.
        The browser's set has to be data -- a named array of {from: 0x.., to: 0x..} entries -- so that
        this guard can compare it to the server's. A chain of comparisons inside an if is unreadable
        from here, and while it was one, the two sets drifted (G-02-11). Renamed, moved or deleted is
        the same as never having been there, and entry-shaped literals surviving elsewhere in the file
        do not make it better
FAIL	github.com/holzcloud/holzkube-manager/internal/imagefactory	0.355s

sha256 nachher:  ca13cf5f5bf2abc1f1e4c9376355363f70d232305ef7beaf9716a42f618ee505
git diff --stat -- web/src/routes/images.tsx   ->  leer
```

Der Verifier hatte an derselben Stelle `ok github.com/holzcloud/holzkube-manager/internal/imagefactory 0.546s` gemessen. Die Datei ist byteweise wiederhergestellt: identischer sha256, leerer Diff.

## Der Ledger

**Zaehler vor der Runde:** `open 52 / waived 0 / fixed 13 / total 65`
**Zaehler nach der Runde:** `open 54 / waived 0 / fixed 15 / total 69`

Differenz: vier neue Eintraege (65 -> 69), davon zwei bei der Anlage als `fixed` markiert (13 -> 15) und zwei offen (52 -> 54). Die Differenz entspricht der Zahl der angelegten Eintraege.

Jeder abgesetzte Aufruf mit der zurueckgegebenen id:

| # | Aufruf | id | Status | Herkunft |
|---|---|---|---|---|
| 1 | `windows append --kind deviation --phase 02 --file internal/httpapi/handlers/schematics.go` | **66** | open | Uebergabe 02-25 #1, in der ueberholenden Form neben 58 |
| 2 | `windows append --kind deviation --phase 02 --file internal/httpapi/handlers/schematics.go` | **67** | fixed | Uebergabe 02-25 #2 (die Regression selbst) |
| 3 | `windows fixed 67` | 67 | fixed | 02-25 vermerkte `Status: fixed` |
| 4 | `windows append --kind deviation --phase 02 --file .planning/WINDOWS.md` | **68** | open | Uebergabe 02-25 #3 (keine verbleibende Ungenauigkeit gefunden) |
| 5 | `windows append --kind deviation --phase 02 --file internal/imagefactory/guard_drift_test.go --line 49` | **69** | fixed | der unverankerte Waechter dieser Runde |
| 6 | `windows fixed 69` | 69 | fixed | Aufzeichnung eines in derselben Runde geschlossenen Defekts, Form von Eintrag 63 |

`.planning/WINDOWS.md` wurde ausschliesslich ueber diese sechs Aufrufe geschrieben. Weder die Markdown-Tabelle noch der JSON-Block noch die Frontmatter-Zaehler wurden von Hand angefasst.

**Die drei Repraesentationen, unabhaengig geparst** — nicht auf das Wort des Werkzeugs hin, so wie es die adversarische Pruefung der Runde 4 getan hat:

| | Frontmatter | Markdown-Tabelle | JSON-Block |
|---|---|---|---|
| open | 54 | 54 | 54 |
| waived | 0 | 0 | 0 |
| fixed | 15 | 15 | 15 |
| total | 69 | 69 Zeilen | 69 Eintraege |

ids 1..69, zusammenhaengend, ohne Duplikate, und die id-Folge der Tabelle ist identisch mit der des JSON-Blocks. `gsd-tools windows status` meldet zusaetzlich `"ok": true`.

**Eintrag 58 ist unveraendert und weiterhin `open`. Eintrag 56 ist unveraendert und weiterhin `open`.** Der Diff von `.planning/WINDOWS.md` entfernt keine Zeile 56 oder 58; er haengt vier Zeilen an und schreibt die Zaehler fort.

## Decisions Made

- **Ein Eintrag neben 58, nicht zwei.** Der Plan las sich, als seien die von 02-25 uebergebene Ergaenzung zu 58 und der geforderte "ueberholende Eintrag neben 58" zwei verschiedene Dinge. Sie sind dieselbe Aussage. Zwei Ledger-Zeilen ueber dieselbe Sache sind genau der Schaden, gegen den die Disziplin des Registers geschrieben ist — jede Behauptung steht einmal da, und wer sie liest, soll nicht raten muessen, welche der beiden gilt. Eintrag 66 traegt 02-25s Wortlaut woertlich und bekommt die Grossbuchstaben-Eroeffnung von 51/52/63 davor, plus die Nennung von Runde 4 als der Messung, die es festgestellt hat. Siehe Deviation 2.
- **`--file` fuer die uebergebenen Eintraege.** 02-25 nennt fuer alle drei `.planning/WINDOWS.md` als *Zieldatei* (dort liegt der Ledger) und nur fuer #2 einen *Bezug* (`internal/httpapi/handlers/schematics.go`). Fuer #1 wurde die Datei genommen, die Eintrag 58 selbst traegt — das ist die Form, die 51, 52 und 63 vorgeben (der ueberholende Eintrag steht unter der Datei des ueberholten). Fuer #3 gibt es keinen Bezug ausser dem Register selbst, also `.planning/WINDOWS.md`.
- **Der doppelte Fehlersatz wurde entfernt.** Siehe `key-decisions` und Deviation 1.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Der Begruendungssatz des Waechters wurde doppelt ausgegeben**

- **Found during:** Task 1 (Lebendbeleg)
- **Issue:** Die RED-Fassung liess den Satz "The browser's set has to be data -- a named array of {from: 0x.., to: 0x..} entries ..." sowohl im Fehler der reinen Funktion als auch im `t.Fatalf` von `browserRefusalRanges` stehen. Der erste Lebendbeleg-Lauf druckte ihn zweimal untereinander. Ein Waechter, der seine Begruendung an zwei Stellen fuehrt, fuehrt sie an einer Stelle, die kein Test liest — dieselbe Form von Defekt, die dieser Plan schliesst, eine Ebene kleiner.
- **Fix:** Der Text lebt vollstaendig im Fehler der reinen Funktion (dort, wo ihn `TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration` als Teilzeichenkette prueft). `browserRefusalRanges` sagt nur noch, welche Datei gelesen wurde: `t.Fatalf("%s: %v", imagesRoutePath, err)`. Das ist der Plan-Satz "der Text bleibt in der Sache erhalten und wandert nur dorthin, wo er wieder wahr ist", woertlich genommen.
- **Files modified:** `internal/imagefactory/guard_drift_test.go`
- **Verification:** `go test ./internal/imagefactory -count=1` gruen; die Fehlerausgabe des Lebendbelegs oben zeigt den Satz genau einmal.
- **Committed in:** `fe73418` (Task 1 GREEN)

**2. [Rule 1 - Bug] Vier Ledger-Eintraege statt der fuenf, die der Plan-Text nahelegt**

- **Found during:** Task 3
- **Issue:** Der Plan verlangt in Task 3 erst "die drei Eintraege, die Plan 02-25 uebergeben hat" (woertlich) und dann "den ueberholenden Eintrag neben 58". 02-25s uebergebener Eintrag #1 *ist* die Ergaenzung zu 58. Beide Anweisungen woertlich zu befolgen haette zwei Ledger-Zeilen mit derselben Aussage ueber Eintrag 58 erzeugt.
- **Fix:** Ein Eintrag (66), der beides ist: 02-25s Wortlaut woertlich, davor die Grossbuchstaben-Eroeffnung `AMENDS ENTRY 58, WHICH STAYS OPEN (no amend verb exists).` nach dem Muster von 51/52/63, und darin die Nennung von Plan 02-25 sowie der Verifikationsrunde 4, die es gemessen hat. Damit sind beide Abnahmekriterien an einem Eintrag erfuellt — "die drei uebergebenen Eintraege sind woertlich angelegt" und "ein Eintrag eroeffnet in Grossbuchstaben mit der Nennung von Eintrag 58, sagt was weiterhin gilt und was ueberholt ist, und nennt Plan 02-25".
- **Files modified:** `.planning/WINDOWS.md`
- **Verification:** `grep -oE 'G-02-9 [a-zA-Z]+' .planning/WINDOWS.md` liefert nur `G-02-9 REMAINS` (Eintrag 58) und `G-02-9 ist` (Eintrag 66, "**G-02-9 ist offen**"), jeweils zweimal, weil Tabelle und JSON-Block dieselbe Zeile tragen. Keine Formulierung meldet G-02-9 als geschlossen.
- **Committed in:** `4adae38` (Task 3)

---

**Total deviations:** 2 auto-fixed (2 Rule 1)
**Impact on plan:** Beide betreffen die Form, nicht den Umfang. Der Waechter tut genau das, was der Plan verlangt, und der Ledger traegt genau die Aussagen, die er tragen soll — einmal jede.

## Was ausdruecklich NICHT geschehen ist

- **Die Ablehnungsmenge wurde nicht angefasst.** Weder `REFUSED_RANGES` in `web/src/routes/images.tsx` noch `NotRepresentableReason` auf der Go-Seite. Netto ist keine Datei unter `web/` veraendert: `git diff --stat -- web/` ist leer, und der sha256 von `images.tsx` ist derselbe wie vor dem Lauf.
- **`TestBrowserRefusalSetEqualsTheServers` wurde nicht veraendert.** Er misst weiterhin Verhalten und nicht zwei Deklarationen, sweept weiterhin jeden Codepoint durch beide Seiten und scheitert weiterhin in beide Richtungen. Nur die Zeichenkette, aus der er seine Bereiche bekommt, ist jetzt der Deklarationsrumpf statt der ganzen Datei.
- **G-02-9 bleibt offen.** Eintrag 58 bleibt offen und woertlich stehen; Eintrag 66 steht daneben und ueberholt allein die Aufzaehlung der ablehnenden Bedingungen. Die `gaps_mitigated_not_closed`-Buchhaltung von 02-24 ist unberuehrt.
- **Eintrag 56 bleibt offen.** Er zeichnet auf, dass die SERVER-Menge hinter dem Codepoint-Sweep nicht erschoepfend gemessen ist und 02-14s Extrapolation erbt. Das ist eine andere Aussage als die dieser Runde; Eintrag 69 nennt 56 als Nachbarn und ueberholt ihn nicht.
- **`.planning/WINDOWS.md` wurde nicht von Hand bearbeitet.** Sechs Werkzeugaufrufe, sonst nichts.
- **`COVERAGE.md` ist unberuehrt.** Dieser Plan aendert eine Testdatei und den Ledger; die Oberflaeche der Image Factory ist auf beiden Seiten der Naht dieselbe wie vor der Runde.
- **Kein npm-Paket und kein Go-Modul hinzugefuegt** (T-02-SC): es gab keinen Installationsschritt.

## Verification

| Pruefung | Ergebnis |
|---|---|
| `go build ./... && go vet ./...` | Exit 0 |
| `gofmt -l ./internal` | leer |
| `golangci-lint run` (`~/go/bin/golangci-lint`) | `0 issues.` |
| `go test ./... -count=1 -race` | gruen ueber alle Pakete, keine `--- FAIL:`-Zeile |
| `go test ./... -count=1` (Orchestrator, HEAD `4adae38`) | Exit 0, alle 15 Pakete `ok` (`internal/imagefactory 5.473s`, `internal/httpapi/handlers 50.035s`) |
| `go test ./internal/imagefactory -count=1` | `ok ... 2.678s` |
| `TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration` einzeln, `-run '^…$' -v` | `--- PASS:` |
| dessen Untertests, `grep -c '^    --- PASS: '` | **4** |
| `TestBrowserRefusalGuardRefusesAnEntryItCannotRead` einzeln, `-run '^…$' -v` | `--- PASS:` |
| dessen Untertests, `grep -c '^    --- PASS: '` | **2** |
| `TestBrowserRefusalSetEqualsTheServers` einzeln, Test unveraendert | `--- PASS:` |
| `grep -c 'func parseBrowserRefusalRanges' guard_drift_test.go` | 1, und die Funktion nimmt kein `*testing.T` |
| `grep -c 'refusedRangesDecl' guard_drift_test.go` | 3 (>= 2) |
| `regexp.QuoteMeta` um den Bezeichner | `guard_drift_test.go:73`, wie `budget_drift_test.go:90` und `stringArrayLiteral` |
| Lebendbeleg: Umbenennung -> `--- FAIL:` -> byteweise Wiederherstellung | sha256 identisch, `git diff --stat -- web/src/routes/images.tsx` leer |
| Treffer im Deklarationsrumpf / in der ganzen Datei | **6 / 6** |
| `npm --prefix web run typecheck` | Exit 0 |
| `npm --prefix web run test` | 9 Test-Dateien, 137 Tests, alle gruen |
| `gsd-tools windows status` | `"ok": true` |
| drei Repraesentationen unabhaengig geparst | 54 / 0 / 15 / 69 dreifach gleich, ids 1..69 eindeutig |
| Eintrag 56, Eintrag 58 | unveraendert, weiterhin `open` |

Die einzeln gefahrenen Testfaelle wurden auf die `--- PASS:`-Zeile geprueft und nicht auf den Exit-Code, und die Zahl der bestandenen Untertests wurde gezaehlt statt nur der Zustand des Elterntests. Ein `-run`-Filter, der nichts trifft, endet in Go mit 0, und ein Tabellentest mit leerer Tabelle ist gruen — beides ist genau die Eigenschaft, um die es in dieser Luecke geht, und darf in ihrer Verifikation nicht wieder auftreten.

## TDD Gate Compliance

Beide Tasks liefen als RED -> GREEN. Kein REFACTOR-Commit, weil keiner noetig war.

| Task | RED | GREEN | REFACTOR |
|---|---|---|---|
| 1 | `c97efbd` `test(02-26)` — 3 von 4 Tabellenzeilen scheitern; die umbenannte Quelle liefert zwei Bereiche statt eines Fehlers | `fe73418` `feat(02-26)` — alle vier gruen | — |
| 2 | `9a54b5e` `test(02-26)` — beide Zeilen scheitern; der Waechter meldet "die Deklaration ist leer" statt das Literal zu zitieren | `8ea6dbd` `feat(02-26)` — beide gruen | — |

Kein Test lief in der RED-Phase unerwartet gruen.

## Known Stubs

Keine. Der Plan hat keine Platzhalter, keine leeren Rueckgaben und keine uebersprungenen Tests hinterlassen.

## Threat Flags

Keine neue Angriffsflaeche ausserhalb des `<threat_model>` des Plans. Der Plan aendert eine Testdatei und den Ledger; es kommt kein Endpunkt, kein Feld und kein Statuscode hinzu. T-02-118 bis T-02-120 sind mit den im Plan benannten Minderungen belegt (siehe `coverage` D1–D3). T-02-SC ist gegenstandslos: kein Installationsschritt. **Naechste freie Threat-Id: T-02-121.**

## Issues Encountered

Der Lauf wurde vom Stream-Watchdog des Orchestrators unterbrochen, nachdem alle drei Tasks committet und alle Verifikationen gefahren waren. Der Orchestrator hat `go build`, `go vet` und `go test ./... -count=1` auf HEAD `4adae38` unabhaengig wiederholt und gruen bestaetigt; keine Arbeit ging verloren, der Arbeitsbaum war sauber. Diese SUMMARY und die Zustandsdateien wurden danach geschrieben.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Die Luecke **G-02-25** (== `02-VERIFICATION.md` Runde 4, `gaps[1]`, rundenlokal G4-2 == `02-REVIEW.md` WR-04) ist geschlossen. Die Wahrheit "A guard reports a pass only when it measured the property it is named for" gilt jetzt fuer beide Waechter aus jener Runde: die kanonische Haelfte unter `internal/imagefactory/canonical_live_test.go:667-670` und den Browser-Ablehnungs-Waechter hier.
- Zusammen mit Plan 02-25 (G-02-24) sind beide Luecken aus Verifikationsrunde 4 geschlossen. Der Ledger traegt beide Runden.
- **Offen bleibt unveraendert G-02-9** (eine Probe, die trotz erhoehtem Budget ausbleibt, ist weiterhin dauerhaft — es gibt keine Re-Probe-Route). Eintraege 58 und 66 im Ledger; die strukturelle Antwort ist Option 1 aus `02-DECISION-probe-budget.md`, die der Betreiber nicht genommen hat.
- **Offen bleibt Eintrag 56:** die SERVER-Ablehnungsmenge hinter dem Codepoint-Sweep ist nicht erschoepfend gemessen und erbt 02-14s Extrapolation. Der Waechter beweist, dass die beiden Schichten uebereinstimmen; er kann nicht beweisen, dass die Menge richtig ist. Das bleibt eine Frage an eine Runde, die `TestLiveFactory` gegen `factory.talos.dev` fahren kann.
- Naechste freie Threat-Id: **T-02-121**.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-09-04*

## Self-Check: PASSED

- `internal/imagefactory/guard_drift_test.go` — FOUND, enthaelt `parseBrowserRefusalRanges`, `refusedRangesDecl`, `refusedRangesEntry` und beide Tabellentests
- `.planning/WINDOWS.md` — FOUND, 69 Eintraege, drei Repraesentationen konsistent
- `.planning/phases/02-transport-seam-talossim-image-factory/02-26-SUMMARY.md` — FOUND
- Commits `c97efbd`, `fe73418`, `9a54b5e`, `8ea6dbd`, `4adae38`, `78f61c3` — alle im Git-Log vorhanden
