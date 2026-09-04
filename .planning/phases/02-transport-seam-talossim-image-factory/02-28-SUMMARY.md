---
phase: 02-transport-seam-talossim-image-factory
plan: 28
subsystem: infra
tags: [windows-ledger, gap-closure, doc-correction, sweep, go, yaml]

# Dependency graph
requires:
  - phase: 02-transport-seam-talossim-image-factory (Plan 02-27)
    provides: "die vier gemessenen Korrekturen am Browser-Ablehnungs-Waechter (gebundener Anker, Bound-Validierung, geteilte Abschneide-Diagnose, drei Falsifikationstabellen), die der ueberholende Ledger-Eintrag zitiert statt paraphrasiert"
  - phase: 02-transport-seam-talossim-image-factory (Plan 02-25)
    provides: "die allquantifizierte Wahrheit 'jede Aussage im Baum, die zwei Bedingungen des Refresh nennt, nennt jetzt drei' — und die Aufzaehlung dreier Stellen, die die vierte uebersehen hat"
provides:
  - "Der letzte Verweis im Baum auf `refreshTheStoredVerdict` nennt die richtige Zahl seiner Bedingungen"
  - "Zwei mechanische Sweeps, die die allquantifizierte Aussage kuenftig pruefen, statt sie durch Aufzaehlung zu behaupten — Sweep A ist als Abnahmebedingung festgenagelt"
  - "`.planning/WINDOWS.md` Eintrag 70: ueberholt Eintrag 69, bleibt `open`, traegt eine pruefbare Schliessbedingung im Text"
  - "`.planning/WINDOWS.md` Eintrag 71: die vierte, uebersehene Zwei-Bedingungen-Aussage, bei der Anlage `fixed`, mit der Grenze der eigenen Messung im Text"
  - "`02-26-SUMMARY.md` sagt nur noch, was gemessen ist — drei WITHDRAWN-Korrekturen in der Form aus `02-21-SUMMARY.md:345-358`"
affects: [verification-runde-6, ship-gate, phase-03]

# Actuals (#2632) — estimateTokens-Skala (chars/4 ueber den realisierten Diff), keine Harness-Zahl.
actuals:
  tokens: 7671
  tasks: 3
  commits: 4

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Sweep statt Aufzaehlung: eine allquantifizierte Aussage wird durch ein wiederholbares Kommando geprueft, dessen Wert vor und nach der Aenderung in der SUMMARY steht"
    - "Ueberholender Ledger-Eintrag: `gsd-tools windows` hat kein Aenderungs-Verb, also bekommt eine verschobene Behauptung einen neuen Eintrag, der in Grossbuchstaben eroeffnet, was er ueberholt"
    - "`correction:` als YAML-Geschwisterschluessel: eine Korrektur im Frontmatter wird als eigener Schluessel angelegt, nicht als Durchstreichung im Klartext, damit der Block YAML bleibt"

key-files:
  created: []
  modified:
    - "internal/httpapi/handlers/schematics.go — das Zahlwort im Doc-Verweis bei :458 (eine Kommentarzeile)"
    - ".planning/WINDOWS.md — die Eintraege 70 (`open`) und 71 (`fixed`), ausschliesslich ueber `gsd-tools windows` geschrieben"
    - ".planning/phases/02-transport-seam-talossim-image-factory/02-26-SUMMARY.md — die `coverage`-D1-Korrektur und zwei WITHDRAWN-Durchstreichungen unter `## Next Phase Readiness`"

key-decisions:
  - "Sweep A wird als Abnahmebedingung festgenagelt, damit die allquantifizierte Aussage kuenftig von einer Suche und nicht von einer Aufzaehlung geprueft wird — die Aufzaehlung aus Plan 02-25 ist genau der Grund, warum diese vierte Stelle vier Runden ueberlebt hat"
  - "Ledger-Eintrag 70 bleibt `open`, obwohl Plan 02-27 in derselben Runde vier Korrekturen geliefert hat: die Eigenschaft ist allquantifiziert ueber kuenftige Umbenennungen, `workflow.windows_enforce` sieht einen `fixed`-Eintrag beim Ship nicht, und ihn bei der Anlage zu schliessen waere woertlich der Fehler von Eintrag 69 ein zweites Mal"
  - "Ledger-Eintrag 71 wird bei der Anlage `fixed` markiert, weil sein Defekt mit einer endlichen, wiederholbaren Messung ueber den ganzen Baum geschlossen ist und nicht mit einer Behauptung ueber kuenftiges Verhalten — der Unterschied zu 70 ist genau diese Endlichkeit"
  - "Die beiden Sweep-Kommandos stehen im Ledger beschrieben statt woertlich zitiert: die Markdown-Repraesentation ist rohrgetrennt und die Konsistenzpruefung liest den Status positionsbasiert als achtes Feld, also darf kein Beschreibungstext ein Rohrzeichen tragen. Die vollstaendige Form mit Rohrzeichen steht hier in der SUMMARY, und der Eintrag sagt ausdruecklich, wo sie steht"
  - "Die D1-Korrektur wird als YAML-Geschwisterschluessel `correction:` angelegt und nicht als Durchstreichung: D1 liegt im Frontmatter, und eine Durchstreichung im Klartext haette den Block als YAML zerschossen"

patterns-established:
  - "Eine Zahl in einer Abnahme wird nicht an den Befund angepasst: Sweep B wurde nach dem Vorliegen von 02-27 neu gemessen, bevor irgendetwas geaendert wurde, und die erwartete Zahl 4 blieb unangetastet"
  - "Ein ueberholender Ledger-Eintrag benennt getrennt, was ueberholt ist und was am ueberholten Eintrag weiterhin gilt (Form aus Eintrag 66)"
  - "Eine WITHDRAWN-Korrektur behaelt den falschen Originaltext durchgestrichen, damit die Korrektur etwas hat, worauf sie zeigen kann (Form aus `02-21-SUMMARY.md:347`)"

requirements-completed: [FACT-02, FACT-06]

# Coverage metadata (#1602)
coverage:
  - id: D1
    description: "Kein Verweis im Baum auf `refreshTheStoredVerdict` nennt mehr eine andere Zahl von Bedingungen als drei"
    requirement: "FACT-02"
    verification:
      - kind: other
        ref: "sed -n '450,465p' internal/httpapi/handlers/schematics.go | sed 's|^[[:space:]]*//[[:space:]]*||' | tr '\\n' ' ' | grep -q 'see refreshTheStoredVerdict for the three conditions and for why a failed refresh is silent'"
        status: pass
      - kind: other
        ref: "Sweep A — grep -rn 'refreshTheStoredVerdict' --include='*.go' --include='*.md' --include='*.ts' --include='*.tsx' internal docs web cmd | grep -c '\\btwo\\b' == 0 (vorher 1)"
        status: pass
      - kind: integration
        ref: "go build ./... && go vet ./... && go test ./internal/httpapi/handlers -count=1"
        status: pass
    human_judgment: false
  - id: D2
    description: "Die Aufzaehlung dreier Stellen ist durch zwei mechanische Sweeps ersetzt, und die vier verbleibenden Fundstellen sind einzeln beurteilt und benannt"
    requirement: "FACT-02"
    verification:
      - kind: other
        ref: "Sweep B — grep -rniE '(refresh|409)' --include='*.go' --include='*.md' --include='*.ts' --include='*.tsx' internal docs web cmd | grep -icE '\\b(two|zwei|both)\\b' == 4 (vorher 5)"
        status: pass
    human_judgment: true
    rationale: "Die Zahl 4 ist mechanisch gemessen, die Beurteilung jeder einzelnen der vier verbleibenden Fundstellen als 'richtig' ist es nicht. Ob `versionMismatchReason` aus einer der drei Bedingungen heraus ueber die beiden anderen spricht — statt sie zu zaehlen — ist eine Lesart, kein Testergebnis. Ein Verifier soll sie nachlesen und nicht uebernehmen; genau das Uebernehmen hat in Runde 5 die vierte Stelle entstehen lassen."
  - id: D3
    description: "`.planning/WINDOWS.md` Eintrag 70 ueberholt Eintrag 69, bleibt `open` und traegt eine pruefbare Schliessbedingung; 69 bleibt woertlich stehen und `fixed`"
    requirement: "FACT-06"
    verification:
      - kind: other
        ref: "unabhaengige Dreifach-Zaehlung (Frontmatter, Markdown-Tabelle, JSON-Block) ergibt woertlich 'fm=55/0/16/71 md=55/16/71 js=55/16/71 ids=71/71'"
        status: pass
      - kind: other
        ref: "gsd-tools windows status --raw | node — 56/58/66 sind `open`, 69 ist `fixed`, der Eintrag mit 'SUPERSEDES ENTRY 69, WHICH STAYS FIXED' ist `open`"
        status: pass
      - kind: other
        ref: "grep -c 'SUPERSEDES ENTRY 69, WHICH STAYS FIXED (no amend verb exists)' .planning/WINDOWS.md == 2 (Tabelle + JSON-Block)"
        status: pass
    human_judgment: false
  - id: D4
    description: "`.planning/WINDOWS.md` Eintrag 71 zeichnet die vierte Zwei-Bedingungen-Aussage auf, nennt beide Sweeps mit ihren Werten und die Grenze seiner eigenen Messung, und ist `fixed`"
    requirement: "FACT-06"
    verification:
      - kind: other
        ref: "gsd-tools windows status meldet '\"ok\": true'; Eintrag 71 hat status `fixed`, kind `deviation`, file `internal/httpapi/handlers/schematics.go`, line 458"
        status: pass
      - kind: other
        ref: "Tabellenblock physisch 73 Zeilen == 71 Eintraege + Kopf + Trenner — kein Rohrzeichen und kein Zeilenumbruch hat die positionsbasierte Lesung zerstoert"
        status: pass
    human_judgment: false
  - id: D5
    description: "`02-26-SUMMARY.md` behauptet an keiner Stelle mehr, der Waechter scheitere bei jeder Umbenennung oder G-02-25 sei geschlossen — und keine Korrektur ersetzt eine Ueberbehauptung durch die naechste"
    requirement: "FACT-06"
    verification:
      - kind: other
        ref: "grep -c 'WITHDRAWN' == 2; der Abschnitt `## Next Phase Readiness` traegt '**CORRECTION (' mit dem Datum 2026-09-04; 'Die Luecke **G-02-25**' steht weiterhin (durchgestrichen) in der Datei"
        status: pass
      - kind: other
        ref: "schluessel-skopierte Pruefung: jedes Vorkommen von 'umbenannt oder verschoben' im Frontmatter haengt unter `    correction:` und keines mehr unter `    description:`"
        status: pass
      - kind: other
        ref: "python3 yaml.safe_load ueber das Frontmatter — parst; D1 hat die Schluessel id/description/requirement/verification/human_judgment/correction, drei unveraenderte `verification`-Eintraege und `human_judgment: false`"
        status: pass
    human_judgment: true
    rationale: "Dass kein Korrekturtext G-02-25 als geschlossen nahelegt und keiner G-02-24 als offen, ist eine Aussage ueber Lesbarkeit und nicht ueber eine Zeichenkette — ein grep kann eine Formulierung finden, aber nicht, wie sie gelesen wird. Genau diese Klasse Fehler (eine Korrektur, die die naechste Ueberbehauptung einfuehrt) ist der Grund, warum diese Phase in Runde 6 ist."

# Metrics
duration: 16 min
completed: 2026-09-04
status: complete
---

# Phase 02 Plan 28: Ledger, Abschlussdokument und der letzte Zahlwort-Verweis Summary

**Der Ledger ueberholt Eintrag 69 mit einem offen bleibenden Eintrag 70 statt ihn zu bearbeiten, `02-26-SUMMARY.md` zieht drei ueberbreite Aussagen als WITHDRAWN zurueck, und der letzte Verweis auf `refreshTheStoredVerdict` nennt drei Bedingungen statt zwei — geprueft von einem Sweep statt von einer Aufzaehlung.**

## Performance

- **Duration:** 16 min
- **Started:** 2026-09-04T19:22:15Z
- **Completed:** 2026-09-04T19:38:30Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments

- **Der letzte Zwei-Bedingungen-Verweis ist weg, und die Aufzaehlung, die ihn uebersehen hat, ist durch eine Suche ersetzt.** `internal/httpapi/handlers/schematics.go:458` sagt jetzt `see refreshTheStoredVerdict for the three conditions and for why a failed refresh is silent`. Sweep A ging von 1 auf 0 und ist ab jetzt Abnahmebedingung — die allquantifizierte Wahrheit aus Plan 02-25 wird kuenftig mechanisch geprueft und nicht mehr durch das Nachziehen dreier namentlich genannter Stellen behauptet.
- **Der Ledger sagt wieder nur, was gemessen ist.** Eintrag 70 (`unmet-truth`, `open`) eroeffnet mit `[from 02-28] SUPERSEDES ENTRY 69, WHICH STAYS FIXED (no amend verb exists)`, trennt sauber, was an 69 ueberholt ist von dem, was weiterhin gilt, zeichnet die vier Korrekturen aus Plan 02-27 mit ihren gemessenen Rot-Ausgaben auf und traegt eine pruefbare Schliessbedingung. Eintrag 69 ist byteweise unveraendert und weiterhin `fixed`.
- **Die vierte Zwei-Bedingungen-Aussage ist als geschlossener Defekt aufgezeichnet.** Eintrag 71 (`deviation`, `fixed`) nennt beide Sweeps mit den Werten vor und nach der Aenderung und benennt ausdruecklich die Grenze seiner eigenen Messung: `internal`, `docs`, `web`, `cmd` ueber `.go`, `.md`, `.ts`, `.tsx` — und nichts darueber hinaus.
- **`02-26-SUMMARY.md` behauptet nicht mehr, was es nicht gemessen hat.** Die `coverage`-Zeile D1 ist auf das Gemessene verengt und traegt einen datierten `correction:`-Block mit dem woertlichen alten Wortlaut; die beiden Saetze unter `## Next Phase Readiness` sind durchgestrichen, mit `— WITHDRAWN` markiert und tragen je einen `**CORRECTION (…, 2026-09-04)**`-Block. Der Korrekturblock zu Punkt 2 sagt ausdruecklich, welche Haelfte welche ist: **G-02-24 ist geschlossen, G-02-25 nicht.**
- **Die drei Repraesentationen des Ledgers stimmen weiterhin ueberein**, unabhaengig vom Werkzeug nachgezaehlt: `fm=54/0/15/69 …` vor der Runde, `fm=55/0/16/71 md=55/16/71 js=55/16/71 ids=71/71` danach.

## Task Commits

Jeder Task wurde atomar committet:

1. **Task 1 (tracer): Der letzte Verweis nennt die richtige Zahl — und ein Sweep ersetzt die Aufzaehlung** — `3a060ef` (docs)
2. **Task 2: Der Ledger ueberholt Eintrag 69, statt ihn zu bearbeiten — und bleibt offen** — `8364a28` (docs)
3. **Task 3: `02-26-SUMMARY.md` sagt nur noch, was gemessen ist** — `3781639` (docs)

**Plan metadata:** siehe Abschluss-Commit dieses Plans (docs: complete plan)

Alle drei Commits tragen den Typ `docs`, weil der Diff dieses Plans aus einer Kommentarzeile und zwei Planungsdokumenten besteht. Kein Verhalten, keine Signatur, keine Bedingung wurde geaendert.

## Die beiden Sweeps

Sie stehen hier in ihrer vollstaendigen Form mit Rohrzeichen; im Ledger sind sie beschrieben statt zitiert, weil dessen Markdown-Repraesentation rohrgetrennt ist.

**Sweep A — die praezise Messung des Defekts.**

```
grep -rn 'refreshTheStoredVerdict' --include='*.go' --include='*.md' --include='*.ts' --include='*.tsx' internal docs web cmd | grep -c '\btwo\b'
```

| Zeitpunkt | Wert |
|---|---|
| beim Planen gemessen | 1 |
| vor der Aenderung neu gemessen (nach Vorliegen von 02-27) | 1 |
| nach der Aenderung | **0** |

Es gibt vier Verweise auf den Bezeichner, alle in `internal/httpapi/handlers/schematics.go` (`:458` der Doc-Verweis, `:461` der Aufruf, `:495` der Doc-Kommentar der Funktion, `:513` ihre Signatur). Genau einer nannte die falsche Zahl.

**Sweep B — die Vollstaendigkeitsaufnahme.**

```
grep -rniE '(refresh|409)' --include='*.go' --include='*.md' --include='*.ts' --include='*.tsx' internal docs web cmd | grep -icE '\b(two|zwei|both)\b'
```

| Zeitpunkt | Wert |
|---|---|
| beim Planen gemessen | 5 |
| **nach dem Vorliegen von 02-27 neu gemessen** (Ausgangswert dieses Laufs) | **5** |
| nach der Aenderung | **4** |

Der neu gemessene Ausgangswert stimmt mit dem beim Planen gemessenen ueberein. Plan 02-27 hat `internal/imagefactory/guard_drift_test.go` angefasst, aber keine Zeile hinterlassen, die Sweep B trifft — er arbeitet an "Refusal", nicht an "refresh". Es gab also keine zusaetzliche Fundstelle zu beurteilen, und **die erwartete Zahl 4 musste nicht angehoben werden.**

### Die vier verbleibenden Fundstellen, einzeln beurteilt

| Datei:Zeile | Wortlaut | Beurteilung |
|---|---|---|
| `internal/httpapi/handlers/schematics.go:628` | "so an operator who meets the declined refresh can tell it from the other two declining conditions" | **richtig, bleibt.** Der Doc-Kommentar von `versionMismatchReason` spricht aus *einer* der drei Bedingungen heraus ueber die *beiden anderen*. Er zaehlt die Bedingungen nicht, er positioniert sich unter ihnen. Nachgelesen an `:626-634`. |
| `internal/httpapi/handlers/schematics_test.go:3143` | "the 409 does not name both architectures" | **richtig, bleibt.** Die Meldung gehoert zu `strings.Contains(detail, ArchAMD64)` und `strings.Contains(detail, ArchARM64)` — "both" bezieht sich auf zwei benannte Werte in einer Fehlermeldung, nicht auf eine Zahl von Bedingungen. |
| `internal/httpapi/handlers/schematics_test.go:3178` | "the 409 does not name both Talos versions" | **richtig, bleibt.** Dieselbe Form ueber `catalogVersion` und `otherCatalogVersion`. |
| `internal/httpapi/handlers/schematics_test.go:3244` | "the 409 does not name both Talos versions" | **richtig, bleibt.** Dieselbe Form, anderer Testfall (die stehengelassene Ablehnung). |

Zwei weitere Stellen, die Plan 02-25 nachgezogen hat, wurden ebenfalls geprueft und **nicht** geaendert: `internal/model/model.go:139` ("Arch's below for the one way the two conditions differ") vergleicht die Versions- mit der Architektur-Bedingung und zaehlt nicht die Bedingungen des Refresh; sie faellt nicht in Sweep B, weil ihre Zeile weder `refresh` noch `409` traegt. `git diff --numstat -- internal/model/model.go` ist leer.

### Warum ein Sweep und keine Aufzaehlung

Das ist der eigentliche Ertrag von Task 1. Plan 02-25 hat seine allquantifizierte Wahrheit ("jede Aussage im Baum") mit einer Aufzaehlung dreier namentlich genannter Stellen eingeloest — `internal/model/model.go` bei `ID` und bei `TalosVersion`, `docs/api-contract.md:620-627`. Die vierte lag im **selben File wie die geaenderte Funktion** und hat vier Runden ueberlebt. Eine allquantifizierte Aussage ist nur so gut wie ihre Suche. Sweep A ist ab jetzt diese Suche und als Abnahmebedingung festgenagelt: er geht rot, sobald irgendwo im Baum wieder eine Zeile entsteht, die auf `refreshTheStoredVerdict` verweist und dabei eine andere Zahl nennt.

## Der Ledger

**Abgesetzte Aufrufe — genau zwei `append`, genau ein `fixed`:**

| # | Aufruf | zurueckgegebene id | Status |
|---|---|---|---|
| 1 | `windows append --kind unmet-truth --phase 02 --file internal/imagefactory/guard_drift_test.go --line 83 --description '<Eintrag A>'` | **70** | `open` |
| 2 | `windows append --kind deviation --phase 02 --file internal/httpapi/handlers/schematics.go --line 458 --description '<Eintrag B>'` | **71** | `open` (bei der Anlage) |
| 3 | `windows fixed 71` | 71 | `fixed`, `resolved_at 2026-09-04T19:29:36.022Z` |

`.planning/WINDOWS.md` wurde ausschliesslich ueber `gsd-tools windows` geschrieben. Weder die Markdown-Tabelle noch der JSON-Block noch die Frontmatter-Zaehler wurden von Hand angefasst; der Diff der Datei entfernt genau vier Zeilen, und alle vier sind Frontmatter-Zaehler bzw. `last_updated`.

**Die Zahlen vor und nach der Runde, unabhaengig vom Werkzeug nachgezaehlt:**

| | Frontmatter (open/waived/fixed/total) | Markdown (open/fixed/rows) | JSON (open/fixed/entries) | ids (eindeutige/hoechste) |
|---|---|---|---|---|
| vor der Runde | 54/0/15/69 | 54/15/69 | 54/15/69 | 69/69 |
| **nach der Runde** | **55/0/16/71** | **55/16/71** | **55/16/71** | **71/71** |

Die Differenz ist nachrechenbar: +2 Eintraege (70 und 71), davon einer `open` (+1 open) und einer `fixed` (+1 fixed). `gsd-tools windows status` bestaetigt zusaetzlich `"ok": true`.

**Die Rohrzeichen-/Zeilenumbruch-Gegenprobe.** Der physische Tabellenblock zaehlt 73 Zeilen von der Kopfzeile bis zur ersten Leerzeile — genau `71 Eintraege + Kopfzeile + Trennzeile`. Waere ein echter Zeilenumbruch in einen Beschreibungstext geraten (den `renderTable`s `cell()` nicht maskiert, #3689), waere die Differenz groesser als 2 gewesen; waere ein Rohrzeichen hineingeraten, haette die positionsbasierte Lesung des achten Feldes einen Status verloren. Beide Beschreibungstexte wurden vor dem `append` geprueft: 0 Rohrzeichen, 0 Backslashes, je genau eine physische Zeile.

**Was unberuehrt geblieben ist:** Eintrag 69 (`fixed`, byteweise unveraendert), Eintrag 56 (`open`, die nicht erschoepfend gemessene SERVER-Menge), die Eintraege 58 und 66 (`open`, beide tragen G-02-9). Maschinell nachgeprueft ueber `windows status --raw`.

## Files Created/Modified

- `internal/httpapi/handlers/schematics.go` — ein Zahlwort im Doc-Verweis bei `:458`. Der Diff ist genau eine Zeile und beruehrt ausschliesslich einen Kommentar (+1/-1).
- `.planning/WINDOWS.md` — die Eintraege 70 und 71, ueber `gsd-tools windows` geschrieben (+30/-4).
- `.planning/phases/02-transport-seam-talossim-image-factory/02-26-SUMMARY.md` — die verengte D1-`description`, der neue `correction:`-Schluessel und zwei WITHDRAWN-Korrekturen unter `## Next Phase Readiness` (+66/-3). Die Diff-Hunks liegen bei `:59`, `:71` und `:316` — D2, D3, die Commit-Liste, der Ledger-Abschnitt und die Self-Check-Liste sind unberuehrt.

## Decisions Made

1. **Sweep A wird als Abnahmebedingung festgenagelt.** Die Aufzaehlung aus Plan 02-25 hat die vierte Stelle nicht gefunden und konnte sie nicht finden. Ab jetzt prueft ein Kommando die allquantifizierte Aussage, und sein Wert steht in dieser SUMMARY.
2. **Eintrag 70 bleibt `open`, obwohl Plan 02-27 in derselben Runde vier Korrekturen geliefert hat.** Die Eigenschaft ist allquantifiziert ueber kuenftige Umbenennungen und kuenftige Eintragsformen; diese Phase hat daran in drei aufeinanderfolgenden Runden je ein engeres Loch gemessen. Ein bei der Anlage geschlossener Eintrag ueber eine solche Eigenschaft war genau der Fehler von 69 — und er ist mechanisch, nicht stilistisch: `workflow.windows_enforce` blockiert `/gsd-ship`, solange `open_count > 0`, und ein `fixed`-Eintrag ist dort unsichtbar.
3. **Eintrag 71 wird bei der Anlage `fixed` markiert.** Der Unterschied zu 70 ist die Endlichkeit der Messung: ein Sweep ueber den ganzen Baum ist wiederholbar und abgeschlossen, keine Behauptung ueber kuenftiges Verhalten. Der Grund steht im Text des Eintrags, weil das `fixed`-Verb keinen traegt (Form aus Eintrag 63).
4. **Die Sweep-Kommandos stehen im Ledger beschrieben statt woertlich zitiert.** Beide enthalten Rohrzeichen — Sweep B zusaetzlich in der Alternation `(refresh|409)` —, und ein Rohrzeichen in der Beschreibung verschiebt den Status aus dem achten Feld der Markdown-Zeile. Der Eintrag benennt beide Kommandos mit Binary, Flags, Pfaden und Endungen und sagt ausdruecklich, dass die vollstaendige Form in `02-28-SUMMARY.md` steht. Backslashes wurden aus demselben Grund vermieden: `cell()` maskiert sie zu `\\` und haette den Text verfaelscht dargestellt.
5. **Die D1-Korrektur ist ein YAML-Geschwisterschluessel `correction:` und keine Durchstreichung.** D1 liegt innerhalb des Frontmatters; eine Durchstreichung im Klartext haette den Block als YAML zerschossen. Der Block-Skalar traegt den woertlichen alten Wortlaut, die Messung, die Ledger-id und die Begruendung, warum der alte Wortlaut nicht geloescht wird.
6. **`human_judgment: true` fuer D2 und D5.** Die Zahlen sind gemessen, die Beurteilungen nicht. Ob `versionMismatchReason` zaehlt oder sich positioniert, und ob ein Korrekturblock als "G-02-25 ist jetzt geschlossen" gelesen werden kann, sind Lesarten. Ein Verifier soll sie nachlesen — genau das Uebernehmen einer fremden Beurteilung hat in Runde 5 die vierte Stelle entstehen lassen.

## Deviations from Plan

None - plan executed exactly as written.

Alle drei Tasks liefen mit den im Plan vorgesehenen Werten. Insbesondere ergab die im Plan verlangte Neumessung von Sweep B nach dem Vorliegen von 02-27 denselben Ausgangswert 5 wie beim Planen, sodass keine zusaetzliche Fundstelle einzeln zu beurteilen war und die erwartete Zahl 4 unangetastet blieb.

## Issues Encountered

- **`task` ist auf diesem Rechner nicht im `PATH`.** Der Plan verlangt unter `<verification>` ein gruenes `task test`. `Taskfile.yml` definiert `test` als genau `go test ./... -count=1 -race`; dieses Kommando wurde stattdessen direkt gefahren und endet mit Exit 0 ueber alle Pakete, einschliesslich der Aenderungen aus Plan 02-27. Die Verifikation ist damit inhaltlich erfuellt, nur ueber einen anderen Einstiegspunkt.

## Verification Results

| Pruefung | Ergebnis |
|---|---|
| `go build ./... && go vet ./...` | Exit 0 |
| `gofmt -l ./internal` | leer |
| `go test ./internal/httpapi/handlers -count=1` | `ok … 59.976s` |
| `go test ./... -count=1 -race` (== `task test`) | Exit 0, 15 Pakete, keine `FAIL`-Zeile |
| `golangci-lint run` (`~/go/bin/golangci-lint`) | `0 issues.` |
| Sweep A | 1 → **0** |
| Sweep B | 5 → **4** |
| Ledger-Signatur, unabhaengig dreifach geparst | `fm=55/0/16/71 md=55/16/71 js=55/16/71 ids=71/71` |
| `gsd-tools windows status` | `"ok": true` |
| `windows status --raw` Maschinenpruefung | 56/58/66 `open`, 69 `fixed`, Eintrag 70 `open` |
| `grep -c 'SUPERSEDES ENTRY 69, WHICH STAYS FIXED (no amend verb exists)'` | 2 (Tabelle + JSON) |
| Frontmatter von `02-26-SUMMARY.md` | parst mit `yaml.safe_load` |
| `grep -c 'WITHDRAWN'` in `02-26-SUMMARY.md` | 2 |
| `npm --prefix web run test` | Exit 0 — 9 Test-Dateien, 137 Tests |
| `npm --prefix web run typecheck` | Exit 0 |
| `git diff --stat -- web/` | leer — dieser Plan aendert keine Datei unter `web/` |

## Known Stubs

Keine. Dieser Plan hat keinen Platzhalter, keinen uebersprungenen Test und kein ungefahrenes `<verify>` hinterlassen. Es wurden deshalb ueber die im Plan vorgesehenen zwei Eintraege hinaus **keine** weiteren Eintraege in `.planning/WINDOWS.md` angelegt — die Abnahme dieses Plans nagelt die Zaehler auf 71 fest.

## Threat Flags

Keine. Dieser Plan fuegt keinen Netzwerk-Endpunkt, keinen Auth-Pfad, kein Dateizugriffsmuster und keine Schema-Aenderung an einer Vertrauensgrenze hinzu. Threat-Ids T-02-124 und T-02-125 sind beide `mitigate` und im Plan-Threat-Register beschrieben; ihre Mitigationen sind ausgefuehrt (der ueberholende, offen bleibende Ledger-Eintrag mit unabhaengiger Dreifach-Zaehlung fuer T-02-124; das korrigierte Zahlwort plus die beiden Sweeps fuer T-02-125).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- **Der Ledger und `02-26-SUMMARY.md` sagen wieder nur, was gemessen ist.** Die Defektklasse, die diese Phase seit fuenf Runden korrigiert — eine Behauptung, die breiter ist als ihre Messung —, ist an den beiden in Runde 5 gefundenen Stellen behoben.
- **G-02-25 ist NICHT als geschlossen gemeldet**, weder hier noch im Ledger noch in den Korrekturbloecken von `02-26-SUMMARY.md`. Die Eigenschaft ist allquantifiziert; Eintrag 70 traegt die pruefbare Schliessbedingung, und eine Verifikationsrunde stellt sie fest: sie benennt `REFUSED_RANGES` praefixiert um und muss `TestBrowserRefusalSetEqualsTheServers` rot sehen, und sie setzt einen Eintrag mit einer Grenze oberhalb `utf8.MaxRune` und muss den Waechter rot sehen. Erst dann wird 70 mit `windows fixed` markiert.
- **G-02-24 ist geschlossen** — von Runde 5 fail-first nachgemessen. Der Korrekturblock unter `## Next Phase Readiness` in `02-26-SUMMARY.md` sagt ausdruecklich, welche Haelfte des zurueckgezogenen Satzes welche ist.
- **Offen bleibt unveraendert G-02-9** (eine Probe, die trotz erhoehtem Budget ausbleibt, ist weiterhin dauerhaft — es gibt keine Re-Probe-Route). Eintraege 58 und 66 im Ledger, beide `open`. Kein Text dieses Plans sagt etwas anderes.
- **Offen bleibt Eintrag 56:** die SERVER-Ablehnungsmenge hinter dem Codepoint-Sweep ist nicht erschoepfend gemessen und erbt 02-14s Extrapolation. Von dieser Runde unberuehrt.
- **Naechste freie Luecken-Id:** G-02-28. **Naechste freie Threat-Id:** T-02-126.
- **Fuer die naechste Verifikationsrunde:** die beiden Sweeps sind wiederholbare Kommandos und stehen oben mit ihren Werten. D2 und D5 tragen `human_judgment: true` — ihre Beurteilungen sind zum Nachlesen gedacht, nicht zum Uebernehmen.

## Self-Check: PASSED

- Alle drei modifizierten Dateien existieren auf der Platte (`internal/httpapi/handlers/schematics.go`, `.planning/WINDOWS.md`, `02-26-SUMMARY.md`) — mit `[ -f ]` geprueft.
- Alle drei Task-Commits sind in `git log --oneline --all` auffindbar: `3a060ef`, `8364a28`, `3781639`.
- Alle `<acceptance_criteria>` der drei Tasks wurden nach Abschluss des jeweiligen Tasks einzeln nachgefahren und sind gruen; die Ergebnisse stehen unter `## Verification Results`.
- Die plan-weite `<verification>` wurde vollstaendig nachgefahren: `go build`/`go vet`/`gofmt`/`golangci-lint`, `go test ./... -count=1 -race`, beide Sweeps, die dreifache Ledger-Zaehlung, `windows status`, der YAML-Parse des korrigierten Frontmatters und die beiden `web`-Kommandos.
- Das Frontmatter dieser SUMMARY parst als YAML; der `coverage`-Kontrakt ist maschinell geprueft (jeder Eintrag mit `human_judgment: false` traegt eine nicht-leere, durchgehend `pass`-Verifikation; jeder mit `true` traegt eine `rationale`).

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-09-04*
