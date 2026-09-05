---
phase: 02-transport-seam-talossim-image-factory
plan: 31
subsystem: infra
tags: [windows-ledger, requirements-traceability, gsd-tooling, drift-guard, decision-record]

# Dependency graph
requires:
  - phase: 02-29
    provides: "der beseitigte Spiegel-Defekt (`strings.TrimSpace(body)`), die getrennten Ursachen fuer Ueberlauf und Inversion, die drei unterscheidenden `wantErr`-Teilzeichenketten und die Duplikatspruefung — der Stand, den Eintrag 72 beschreibt"
  - phase: 02-30
    provides: "der parametrisierte, wrapper-freie Lesepfad, `guardBlindSpots`/`honestClaim` nach Mechanismus skopiert, die sieben einzeln gemessenen Blindheitszeilen und der Punkt-6-Lauf ohne Fund — die Messwerte, die Eintrag 72 zitiert"
provides:
  - "Ledger-Eintrag 72: amendiert 70, bleibt `open`, ohne Zeilennummer, zeigt auf `refusedRangesDecl`; traegt die sechspunktige Schliessbedingung, das Ausbleiben der Ratifikation und die fuenf verbleibenden Reste"
  - "Ledger-Eintrag 73: der an der Quelle gelesene und an einer Kopie ausserhalb des Repositories reproduzierte Defekt an `requirements revert-phase`, mit seiner ueber das ganze Register gemessenen Reichweite von 16 dekorierten Release-Blocker-Zeilen"
  - "Ledger-Eintrag 74: die drei Geschwister-Leser, jeder einzeln gemessen, samt der Stelle, an der die Messung dem Entscheidungsdokument widerspricht, und der ausgeschriebenen Messgrenze"
  - "die korrigierte Statuszelle der TRANS-06-Zeile — alle vierzehn Phase-2-Zeilen lesen `Gaps Found`, die Id-Zelle behaelt ihre Dekoration"
  - "der angehaengte, datierte Nachtrag an `02-DECISION-drift-guard-lexik.md`: delegiert statt ratifiziert, Plan-Zuordnung, Byte-Integritaet des entschiedenen Bereichs, Zahlenkorrektur 2048 -> 2046"
affects: [verification-round-8, gsd-runtime-milestone, phase-03-inventory]

# Actuals (#2632) — estimateTokens-Skala (chars/4 ueber den realisierten Diff), keine Harness-Zahl.
actuals:
  tokens: 16363
  tasks: 3
  commits: 3

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Ein Ledger-Eintrag ueber Code zeigt auf einen SYMBOLNAMEN und laesst die Zeilenspalte leer (IN-05); die Absicht steht im Text, damit die leere Spalte nicht wie ein Versehen aussieht"
    - "Ein Werkzeugdefekt wird an einer Kopie AUSSERHALB des Repositories reproduziert, mit einer Kontroll-Id im selben Aufruf, bevor eine Zeile Ledger-Text entsteht"
    - "Eine Aufzaehlung aus einem Planungsdokument wird vor dem Ledgern einzeln nachgemessen; wo die Messung widerspricht, sagt der Eintrag es und benennt die Stelle"

key-files:
  created:
    - .planning/phases/02-transport-seam-talossim-image-factory/02-31-SUMMARY.md
  modified:
    - .planning/WINDOWS.md
    - .planning/REQUIREMENTS.md
    - .planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-drift-guard-lexik.md

key-decisions:
  - "Die Delegation wird als Delegation verbucht, nicht als Ratifikation: die Wahl zwischen B und A wurde vom Betreiber am 2026-09-05 ausdruecklich an den autonomen Lauf delegiert, der Lauf hat B gewaehlt, und der Betreiber hat B nicht selbst ratifiziert. Der Statusblock von 02-DECISION-drift-guard-lexik.md bleibt `offen — vorgelegt, nicht ratifiziert`"
  - "Eintrag 70 bleibt `open` und woertlich unveraendert; die Divergenz zwischen seiner geschriebenen Schliessbedingung und seinem tatsaechlichen Offen-Grund wird durch den TEXT von 72 behoben, nicht durch ein `fixed` an 70"
  - "Die Id-Zelle der TRANS-06-Zeile behaelt ihre Dekoration; korrigiert wird die Statuszelle. Die Dekoration zu entfernen waere die bequeme Reparatur am falschen Ort und beschaedigte die Release-Blocker-Auszeichnung, um einen Werkzeugdefekt zu umgehen"
  - "Der Werkzeugdefekt wird NICHT in der GSD-Laufzeit repariert — `~/.claude/gsd-core/` liegt ausserhalb dieses Repositories und ausserhalb des Umfangs dieser Phase. Gemessen, geledgert, die Zeile korrigiert, mehr nicht"
  - "Kein `windows fixed` in dieser Runde, kein `waive`, kein `overrides:`-Block. Die drei neuen Eintraege stehen bei der Anlage auf `open`, weil `workflow.windows_enforce` einen `fixed`-Eintrag beim Ship nicht sieht"

patterns-established:
  - "Provenienz vor Inhalt: wo eine Entscheidung delegiert und nicht ratifiziert wurde, steht die Delegationsformel woertlich in beiden Registern, bevor irgendetwas auf ihr aufgebaut wird"
  - "Register-Korrektur durch Anhaengen: ein Entscheidungsdokument wird nie eingearbeitet, sondern bekommt einen datierten Nachtrag mit dem Praefix-Hash des entschiedenen Bereichs — dieselbe Mechanik wie der Ledger, der kein Aenderungs-Verb kennt"

requirements-completed: []
# ABSICHTLICH LEER. Der `requirements`-Block dieses Plans fuehrt alle vierzehn Phase-2-Ids, weil
# Task 2 die Statuszeile JEDER dieser vierzehn Zeilen geprueft und die eine falsche korrigiert hat.
# Das ist eine Handlung AN den vierzehn Zeilen, nicht die Behauptung, dieser Plan LIEFERE die
# vierzehn Requirements. Die Phase traegt `gaps_found`; genau die Verwechslung der beiden Felder hat
# in Runde 6 die Buchhaltungsregression ausgeloest, die der Betreiber mit `b7a1ab9` zuruecknehmen
# musste, und dieser Plan hat die Folge davon gerade erst von Hand korrigiert.

coverage:
  - id: D1
    description: "Ledger-Eintrag 72 — amendiert 70, bleibt offen, ohne Zeilennummer, mit der sechspunktigen Schliessbedingung, dem Ausbleiben der Ratifikation und den fuenf verbleibenden Resten"
    verification:
      - kind: other
        ref: "grep -Fc 'AMENDS ENTRY 70, WHICH STAYS OPEN (no amend verb exists)' .planning/WINDOWS.md == 2"
        status: pass
      - kind: other
        ref: "windows status --raw: 56/58/66/70 open, 69 fixed, amendierender Eintrag open mit line=null, Zaehler 56/0/16/72"
        status: pass
    human_judgment: false
  - id: D2
    description: "Der gemessene Werkzeugdefekt an `requirements revert-phase` als offener Ledger-Eintrag 73, mit an der Quelle gelesener Ursache und ueber das ganze Register gemessener Reichweite"
    verification:
      - kind: other
        ref: "gsd-tools requirements revert-phase TRANS-06 FACT-03 --project-dir <Kopie ausserhalb des Repositories> -> reverted=[FACT-03], unchanged=[TRANS-06]"
        status: pass
      - kind: other
        ref: "windows status --raw: Eintrag 73 open, kind=deviation, Zaehler 57/0/16/73, ids 1..73 lueckenlos"
        status: pass
    human_judgment: false
  - id: D3
    description: "Alle vierzehn Phase-2-Zeilen im Traceability-Register lesen `Gaps Found`; keine liest `Complete`; die korrigierte Zeile behaelt ihre Dekoration"
    requirement: "TRANS-06"
    verification:
      - kind: other
        ref: "grep -c '^| .*| Phase 2 | Gaps Found |' .planning/REQUIREMENTS.md == 14"
        status: pass
      - kind: other
        ref: "grep -c '^| .*| Phase 2 | Complete |' .planning/REQUIREMENTS.md == 0"
        status: pass
      - kind: other
        ref: "git diff --numstat .planning/REQUIREMENTS.md == 1 1 (genau eine geaenderte Zeile)"
        status: pass
    human_judgment: false
  - id: D4
    description: "Ledger-Eintrag 74 — die drei Geschwister-Leser, jeder einzeln gemessen, mit dem Widerspruch zum Entscheidungsdokument und der ausgeschriebenen Messgrenze"
    verification:
      - kind: other
        ref: "Sonde ausserhalb des Repositories, die drei Muster woertlich kopiert: Leser 1 beide Loecher JA, Leser 2 Praefix-Loch NEIN und Blindheit begrenzt JA, Leser 3 beide Loecher JA"
        status: pass
      - kind: other
        ref: "windows status --raw: Eintrag 74 open, kind=unmet-truth, Endsignatur 58/0/16/74, ids 1..74 lueckenlos und doppelfrei"
        status: pass
    human_judgment: false
  - id: D5
    description: "Der angehaengte, datierte Nachtrag an `02-DECISION-drift-guard-lexik.md` mit unveraendertem Statusblock, Byte-Integritaet des entschiedenen Bereichs und der Zahlenkorrektur 2048 -> 2046"
    verification:
      - kind: other
        ref: "head -c 34626 02-DECISION-drift-guard-lexik.md | shasum -a 256 == 16d106e88b5e3b3ceafe1c80be6c1cbe89972729fd23951b4fceb6db1a75b5b1 (vor wie nach dem Anhaengen)"
        status: pass
      - kind: other
        ref: "Statusblock Zeile 3 liest unveraendert 'offen — vorgelegt, nicht ratifiziert'"
        status: pass
    human_judgment: false
  - id: D6
    description: "Die Delegations- statt Ratifikationsformulierung ist woertlich in beiden Registern eingetragen und nirgends zu einer Ratifikation umgedeutet"
    verification: []
    human_judgment: true
    rationale: "Ob eine Formulierung die Provenienz einer Entscheidung ehrlich wiedergibt, ist genau die Frage, die kein grep beantwortet. Die Zeichenkette laesst sich pruefen, ihre Angemessenheit nicht — und dieser Plan existiert, weil eine Behauptung, die breiter ist als ihre Messung, maschinell gruen aussieht"
  - id: D7
    description: "Nachpruefung, dass die Buchhaltungsregression der Runde 6 geschlossen geblieben ist"
    verification:
      - kind: other
        ref: "STATE.md: Frontmatter completed_plans 36 / total_plans 37 stimmt mit 'Plan: 30 of 31' unter ## Current Position ueberein (36 - 6 Plaene der Phase 1 = 30)"
        status: pass
      - kind: other
        ref: "ROADMAP.md Zeile 81: ZWEI Zahlenangaben fuer die ausgefuehrten Plaene der Phase, 30/31 und 28/31 — siehe Deviation 1"
        status: fail
    human_judgment: false

# Metrics
duration: 14 min
completed: 2026-09-05
status: complete
---

# Phase 02 Plan 31: Die Register sagen, was Runde 7 getan hat — und was sie nicht getan hat — Summary

**Drei neue offene Ledger-Eintraege (72 amendiert 70 mit einer sechspunktigen Schliessbedingung, 73 der an der Quelle gelesene Defekt an `requirements revert-phase`, 74 die drei einzeln gemessenen Geschwister-Leser), eine von Hand korrigierte Requirement-Zeile, die kein Werkzeug erreichen konnte, und ein angehaengter Nachtrag, der festhaelt, dass die Richtung dieser Runde delegiert und nicht ratifiziert wurde.**

## Performance

- **Duration:** 14 min
- **Started:** 2026-09-05T09:23:39Z
- **Completed:** 2026-09-05T09:38:05Z
- **Tasks:** 3
- **Files modified:** 3 (plus diese SUMMARY)

## Die Provenienz der Richtungsentscheidung

**Dieser Abschnitt steht vor allem anderen, weil er die Sache dieser Phase ist.**

Die Wahl zwischen B und A wurde vom Betreiber am 2026-09-05 ausdruecklich an den autonomen Lauf
delegiert; der Lauf hat B gewaehlt. Der Betreiber hat B nicht selbst ratifiziert.

Der Checkpoint-Text von Plan 02-30 und der Abschnitt "Ratifikation" von
`02-DECISION-drift-guard-lexik.md` sehen beide vor, dass eine Ratifikation in den Statusblock des
Dokuments und in den Text von Ledger-Eintrag 72 wandert. **Das ist nicht der eingetretene Fall.**
Dieser Plan hat statt einer Ratifikation die Formulierung oben woertlich an beide Stellen getragen,
und der Statusblock liest unveraendert `offen — vorgelegt, nicht ratifiziert`. Eine delegierte Wahl
als Ratifikation zu verbuchen waere eine Behauptung, die breiter ist als ihre Messung — genau der
Defekt, den diese Runde schliessen soll, begangen von der Runde, die ihn schliesst.

Ebenfalls festzuhalten, damit ein spaeterer Leser die Beweisdichte einordnen kann: **der Betreiber
hat fuer den Rest dieser Runde ausdruecklich um schnelleres Vorgehen mit weniger Verifikation
gebeten.** Die Evidenz dieser Runde ist duenner als die der Runden 5 bis 7 zuvor. Gemessen wurde
genau das, was die `<verify>`-Bloecke und `<acceptance_criteria>` des Plans verlangen, und nichts
darueber hinaus.

## Accomplishments

- **Eintrag 72 amendiert 70, ohne 70 anzufassen.** Er eroeffnet mit
  `AMENDS ENTRY 70, WHICH STAYS OPEN (no amend verb exists)`, steht auf `open`, traegt **keine**
  Zeilennummer und zeigt stattdessen auf den Symbolnamen `refusedRangesDecl` — womit 70s staler
  Zeiger auf Zeile 83 erledigt ist (02-REVIEW IN-05), ohne dass 70 bearbeitet wird.
- **Was 72 ueberholt, ist eine Divergenz und keine Messung.** 70s geschriebene Schliessbedingung
  wurde von Runde 6 woertlich ausgefuehrt und in beiden Richtungen erfuellt — und 70 steht trotzdem
  offen, weil sein tatsaechlicher Offen-Grund ein anderer ist. Genau das ueberholt 72; 70s Messwerte
  bleiben gueltig und sind als solche benannt.
- **Der Werkzeugdefekt ist gemessen statt geglaubt.** `requirements revert-phase` gegen eine Kopie
  ausserhalb des Repositories, mit einer undekorierten Kontroll-Id im selben Aufruf: die Kontrolle
  erscheint unter `reverted`, die dekorierte Id unter `unchanged`.
- **Seine Ursache steht woertlich in Eintrag 73**, aus `gsd-core/bin/lib/milestone.cjs` gelesen und
  nicht aus der Beobachtung erschlossen.
- **Seine Reichweite ist ueber das ganze Register gemessen:** 16 dekorierte Release-Blocker-Zeilen —
  alle 16 Release-Blocker des Projekts, weil die Dekoration die Release-Blocker-Auszeichnung *ist*.
- **Die eine Zeile, die kein Werkzeug erreicht hat, liest wieder `Gaps Found`.** Genau eine Zelle
  geaendert; die Id-Zelle behaelt ihre Dekoration.
- **Die drei Geschwister-Leser sind einzeln gemessen, und die Messung widerspricht dem
  Entscheidungsdokument an einer Stelle** — das ist der eigentliche Ertrag von Task 3.
- **Der Ledger schliesst die Runde bei 58 open / 0 waived / 16 fixed / 74 total**, in drei
  uebereinstimmenden Repraesentationen, unabhaengig vom Werkzeug nachgezaehlt.

## Task Commits

1. **Task 1 (tracer): Eintrag 72 und der Nachtrag am Entscheidungsdokument** — `890468d` (docs)
2. **Task 2: Eintrag 73 und die eine Requirement-Zeile** — `efb550c` (docs)
3. **Task 3: Eintrag 74 — die drei Geschwister-Leser** — `26c3ddb` (docs)

**Plan metadata:** siehe der `docs(02-31)`-Abschlusscommit.

### Die drei `windows append`-Aufrufe mit ihren zurueckgegebenen Ids

| # | Aufruf | zurueckgegebene id | Zaehler danach |
|---|---|---|---|
| 1 | `windows append --kind unmet-truth --phase 02 --file internal/imagefactory/guard_drift_test.go` (ohne `--line`) | **72** | 56/0/16/72 |
| 2 | `windows append --kind deviation --phase 02 --file .planning/REQUIREMENTS.md` (ohne `--line`) | **73** | 57/0/16/73 |
| 3 | `windows append --kind unmet-truth --phase 02 --file internal/imagefactory/guard_drift_test.go` (ohne `--line`) | **74** | 58/0/16/74 |

**Kein `windows fixed`, kein `waive`, kein `overrides:`-Block.** `waived_count` ist ueber die ganze
Runde 0 geblieben. Der einzige Treffer eines `grep 'overrides:'` ueber den Diff dieser Runde ist die
Tabellenzeile des Nachtrags, die *aussagt*, dass `overrides:` nicht benutzt wird — eine Erwaehnung,
kein Block.

## Der Werkzeugdefekt, gemessen (Task 2)

### Der Lauf gegen die Kopie

Eine Kopie von `.planning/REQUIREMENTS.md` wurde in ein Temp-Projektverzeichnis **ausserhalb dieses
Repositories** gelegt und dort die undekorierte Phase-2-Zeile `FACT-03` als Kontrolle kuenstlich auf
`Complete` gesetzt. Beide Ids gingen dann durch **einen** Aufruf:

```
$ gsd-tools requirements revert-phase TRANS-06 FACT-03 --project-dir <temp>
{
  "reverted": [
    "FACT-03"
  ],
  "unchanged": [
    "TRANS-06"
  ],
  "total": 2
}
```

Zustand der Kopie danach:

```
224:| **TRANS-06** 🚫 | Phase 2 | Complete |      <- unveraendert
229:| FACT-03 | Phase 2 | Gaps Found |            <- zurueckgenommen
```

Beide Checkboxen waren vorher schon leer, beide Ids liefen durch denselben Aufruf, und der einzige
Unterschied zwischen ihnen ist die Dekoration der Id-Zelle. **Das echte
`.planning/REQUIREMENTS.md` wurde von diesem Messlauf nicht beruehrt** —
`git diff --quiet HEAD -- .planning/REQUIREMENTS.md` war unmittelbar danach gruen.

### Die Ursache, an der Quelle gelesen

Datei `gsd-core/bin/lib/milestone.cjs` (GSD-Laufzeit unter `~/.claude/`), Funktion
`cmdRequirementsRevertPhase`. Zwei Wirkflaechen, die sich darueber uneinig sind, wie eine Id-Zelle
aussehen darf.

**Wirkflaeche 1 — die Checkbox, die den `**fett**`-Wrapper ausdruecklich traegt** (Zeile 425):

```js
const checkboxPattern = new RegExp(`(-\\s*\\[)x(\\]\\s*\\*\\*${reqEscaped}\\*\\*)`, 'gi');
```

**Wirkflaeche 2 — das Zeilen-Praedikat des Traceability-Registers, exakte Gleichheit nach Trimmen
und Kleinschreibung** (Zeile 433):

```js
const rowMatch = (row) => (Object.values(row)[0] ?? '').trim().toLowerCase() === reqId.toLowerCase();
```

`**trans-06** 🚫` ist nicht `trans-06`. Das Praedikat findet die Zeile nicht, `tableHit` bleibt
`false`, die Id landet unter `unchanged`, und es wird nichts geschrieben. Eine dekorierte Zeile ist
fuer die eine Haelfte derselben Funktion sichtbar und fuer die andere nicht.

### Reichweite: 16 dekorierte Release-Blocker-Zeilen

Gemessen ueber das ganze Register, nicht ueber diese Phase:

`TRANS-06` (Phase 2), `INV-01`, `INV-03`, `INV-04`, `INV-05`, `INV-07` (Phase 3), `JOB-07`
(Phase 6), `CFG-02` (Phase 7), `PROV-05`, `PROV-09`, `PROV-10` (Phase 8), `UPG-02`, `UPG-03`,
`UPG-06`, `UPG-07` (Phase 9), `OPS-05` (Phase 10).

Das sind **alle 16** Release-Blocker des Projekts (die Summenzeile des Registers nennt dieselbe
Zahl). Die Dekoration *ist* die Release-Blocker-Auszeichnung, also ist jeder einzelne betroffen und
wird beim naechsten `gaps_found` seiner Phase denselben Weg nehmen.

### Die Korrektur und die Nachzaehlung

Genau eine Zelle geaendert:

```
-| **TRANS-06** 🚫 | Phase 2 | Complete |
+| **TRANS-06** 🚫 | Phase 2 | Gaps Found |
```

`git diff --numstat` ueber `.planning/REQUIREMENTS.md`: `1  1` — eine Zeile.

- Phase-2-Zeilen mit `Gaps Found`: **14** (soll 14)
- Phase-2-Zeilen mit `Complete`: **0** (soll 0)
- Phase-2-Checkboxen, die leer sind: **14 von 14**

## Die drei Geschwister-Leser, jeder einzeln gemessen (Task 3)

Gemessen mit einer wegwerfbaren Sonde **ausserhalb dieses Repositories**, die die drei Muster
woertlich kopiert und gegen synthetische Quellen faehrt. Die Sonde ist entfernt;
`git status --porcelain` zeigte danach nur die geplanten `.planning/`-Pfade.

| Leser | Datei / Symbol | Praefix-Loch | lexikalische Blindheit |
|---|---|---|---|
| 1 | `internal/imagefactory/guard_drift_test.go` / `stringArrayLiteral` | **JA** — nur `..._LEGACY` vorhanden: `GELESEN, members=2` (Kontrolle: `GELESEN, members=2`) | **JA** — nur im `//`-Kommentar: `GELESEN, members=2`; nur im Template-Literal: `GELESEN, members=2` |
| 2 | `internal/httpapi/handlers/budget_drift_test.go` / der Anker in `TestBudgetWaitsStatedInTheUIMatchTheRouteBudgets` | **NEIN** — nur `SCHEMATIC_WAIT_SECONDS_LEGACY` vorhanden: `NICHT GELESEN` (Kontrolle: `GELESEN, wert=45`) | **JA, aber begrenzt** — Blockkommentar `GELESEN, wert=45`, Template-Literal `GELESEN, wert=45`; `//`-Kommentar `NICHT GELESEN`, eingerruecktes `{/* */}` `NICHT GELESEN` |
| 3 | `internal/imagefactory/warnings_test.go` / die beiden `strings.Contains`-Sweeps in `TestWarningDetailsMatchTheUI` samt der Schleife ueber `exportedWarningCodes` | **JA** — gesucht `installer.repo-fallback`, vorhanden nur `installer.repo-fallback-unverified`: `GELESEN` | **JA in jeder gepruefen Form** — `//`-Kommentar, Blockkommentar, Template-Literal und JSX-Text jeweils `GELESEN` |

### Wo die Messung dem Entscheidungsdokument widerspricht

`02-DECISION-drift-guard-lexik.md`, Abschnitt *"Was keine der vier Varianten schliesst"*, Unterpunkt
*"Die Geschwister"*, schreibt: *"`stringArrayLiteral` … traegt Runde 5s Praefix-Loch unveraendert
weiter; `budget_drift_test.go:89-90` und `warnings_test.go:281-336` **ebenso**."*

**Fuer den Budget-Anker stimmt das nicht.** Sein Muster verlangt hinter dem Bezeichner unmittelbar
Leerraum und das Gleichheitszeichen (`const\s+%s\s*=\s*([0-9]+)\b`), weshalb der Unterstrich in
`SCHEMATIC_WAIT_SECONDS_LEGACY` das Muster bricht. Er hat das Praefix-Loch **nicht**, und er hatte es
nie.

**Und die Beschreibung seiner lexikalischen Blindheit ist zu weit.** Sein Zeilenanfangs-Anker laesst
Einrueckung zu, aber weil auf den Leerraum unmittelbar `export` oder `const` folgen muss, faellt eine
mit `//` oder mit `{/*` beginnende Zeile durch. Er ist blind gegen einen Blockkommentar und gegen ein
Template-Literal, weil die Deklarationszeile *im Inneren* dieser Formen wieder mit `const` beginnt —
nicht gegen Kommentarzeichen im Allgemeinen.

Die Vermutung des Dokuments trifft also fuer Leser 1 und Leser 3 zu, und fuer Leser 2 nur zur Haelfte
und aus einem anderen Grund als dort angenommen. **Genau deshalb hat dieser Plan gemessen statt
uebernommen:** eine uebernommene Vermutung in einem Ledger waere woertlich die Sorte Behauptung, die
dieser Ledger fuehrt.

## Der Ledger-Stand, unabhaengig vom Werkzeug nachgezaehlt

Frontmatter, Markdown-Tabelle und JSON-Block einzeln geparst, Status pro Id verglichen:

```
fm=58/0/16/74 md=58/16/74 js=58/16/74 ids=74/74
waived: fm=0 md=0 js=0
ids lueckenlos 1..N: true | doppelfrei: true | Status-Abweichungen pro Id: 0
```

`windows status` meldet zusaetzlich `"ok": true`. Maschinell bestaetigt:

| Eintrag | Status | erwartet |
|---|---|---|
| 56 (SERVER-Menge nicht erschoepfend gemessen) | `open` | `open`, von 72 **nicht** ueberholt |
| 58 (G-02-9) | `open` | `open`, unberuehrt |
| 66 (amendiert 58) | `open` | `open`, unberuehrt |
| 69 (der unverankerte Waechter) | `fixed` | `fixed`, nicht angefasst |
| 70 (SUPERSEDES 69) | `open` | `open`, **woertlich unveraendert** |
| 72 (AMENDS 70) | `open`, `line=null` | `open`, ohne Zeilennummer |
| 73 (der Werkzeugdefekt) | `open`, `line=null` | `open` |
| 74 (die Geschwister) | `open`, `line=null` | `open` |

**G-02-9 ist offen.** Die Eintraege 58 und 66 tragen ihn unberuehrt. **G-02-29 ist verengt, nicht
geschlossen** — keine Zeile dieses Plans behauptet etwas anderes; Eintrag 72 existiert genau deshalb,
weil die Luecke offen bleibt.

## Der Nachtrag am Entscheidungsdokument

**Angehaengt, nicht eingearbeitet.** Der Statusblock liest unveraendert:

```
Status: **offen** — vorgelegt, nicht ratifiziert. Empfehlung: Variante B (den Anspruch verengen);
```

**Byte-Integritaet des entschiedenen Bereichs:** die ersten **34626 Bytes** der Datei hashen vor wie
nach dem Anhaengen auf
`16d106e88b5e3b3ceafe1c80be6c1cbe89972729fd23951b4fceb6db1a75b5b1`. Erzeugendes Kommando:

```
head -c 34626 .planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-drift-guard-lexik.md | shasum -a 256
```

Der Nachtrag traegt: die Delegationsformel woertlich; die Tabelle, welcher Plan welchen Teil der
Empfehlung traegt und welcher Teil richtungsunabhaengig ist; den Hinweis, dass ein Wechsel auf
Variante A seit dem 2026-09-05 zusaetzlich einen Plan kostet; die Bitte des Betreibers um reduzierte
Verifikationstiefe; und die **Zahlenkorrektur `2048` -> `2046`**.

### Die Zahlenkorrektur

Der Abschnitt *"Was B in Runde 7 anders bekommen muss"* schreibt *"Fuer 2048 Codepoints wird nichts
verglichen"*. U+D800..U+DFFF **sind** 2048 Codepoints — aber der Waechter behauptet **beide
Endpunkte einzeln**, bevor der Sweep den Bereich mit einem `continue` ueberspringt. Verglichen wird
also nichts fuer die **2046** Codepoints *dazwischen*. **Es gilt 2046.** Der Satz im Koerper des
Dokuments bleibt unveraendert stehen; korrigiert wird durch Anhaengen, genau wie beim Ledger, der
ebenfalls kein Aenderungs-Verb kennt. Ohne diese Korrektur liefe Ledger-Eintrag 72 (der durchgaengig
2046 traegt) gegen das Dokument, auf das er verweist.

## Nachpruefung: ist die Buchhaltungsregression der Runde 6 geschlossen geblieben?

**Geprueft, nicht geaendert** — mit einem Teilbefund, siehe Deviation 1.

- **`.planning/STATE.md`: konsistent.** Frontmatter `completed_plans: 36` / `total_plans: 37`;
  `## Current Position` sagt `Plan: 30 of 31`. Die Phase 1 traegt 6 Plaene, also ist `36 - 6 = 30`
  dieselbe Zahl. Kein Widerspruch.
- **`.planning/ROADMAP.md`: NICHT geschlossen.** Zeile 81 traegt **zwei** Zahlenangaben fuer die
  ausgefuehrten Plaene dieser Phase, und sie widersprechen einander:
  `30/31 plans executed` (vom Werkzeug gefuehrt) und am Ende derselben Zeile `— 28/31 ausgeführt`
  (von Hand geschrieben, stale). Das ist formgleich mit dem Befund der Runde 6
  (`28/28 plans executed (26/28 ausgefuehrt) … — 26/28 ausgefuehrt`): der Betreiber hat mit `77ed0f3`
  die *eingeklammerte* Zahl entfernt, die *nachgestellte* blieb stehen und ist mit der Ausfuehrung
  von 02-29 und 02-30 erneut auseinandergelaufen.

## Files Created/Modified

- `.planning/WINDOWS.md` — drei neue offene Eintraege (72, 73, 74) ueber
  `gsd-tools windows append`; kein Handanlegen an Tabelle, JSON-Block oder Frontmatter-Zaehlern.
- `.planning/REQUIREMENTS.md` — genau eine Zelle: die Statuszelle der TRANS-06-Zeile von `Complete`
  auf `Gaps Found`. Die Dekoration der Id-Zelle bleibt.
- `.planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-drift-guard-lexik.md` —
  ein angehaengter, datierter Nachtrag; alles oberhalb byteweise unveraendert.
- `.planning/ROADMAP.md` — eine stale Zahlenangabe korrigiert (Deviation 1) und die
  Fortschrittstabelle der Phase fortgeschrieben.
- `.planning/STATE.md` — Position, Metrik, Entscheidungen, Sitzung.

**Kein Code.** `git diff --quiet HEAD -- internal/ cmd/ web/` war ueber den ganzen Plan gruen, an
jeder Task-Grenze einzeln geprueft. Es wurde kein Go- und kein npm-Testlauf als Abnahme dieses Plans
gefahren; `go test ./internal/imagefactory/... -count=1` lief einmal als **Vorbedingung** von Task 1
und war gruen (`ok github.com/holzcloud/holzkube-manager/internal/imagefactory 2.741s`).

## Decisions Made

Siehe `key-decisions` im Frontmatter. Die tragende Entscheidung ist die Provenienz oben.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] `.planning/ROADMAP.md` traegt zwei widersprechende Zahlen fuer die ausgefuehrten Plaene der Phase**

- **Found during:** Task 2 (die Nachpruefung, ob die Buchhaltungsregression der Runde 6 geschlossen
  geblieben ist)
- **Issue:** Der Plan nimmt in seiner Abnahmebedingung an, `.planning/ROADMAP.md` trage *"genau eine
  Zahlenangabe fuer die ausgefuehrten Plaene dieser Phase"*. Gemessen sind es **zwei**:
  `30/31 plans executed` am Anfang von Zeile 81 (vom `roadmap update-plan-progress`-Werkzeug
  gefuehrt) und `— 28/31 ausgeführt` am Ende derselben Zeile (von Hand geschrieben). Sie
  widersprechen einander seit der Ausfuehrung von 02-29 und 02-30. Der Betreiber hatte mit `77ed0f3`
  die *eingeklammerte* dritte Zahl entfernt; die nachgestellte blieb und ist wieder gedriftet. Die
  Regression der Runde 6 ist damit in ihrer nachgestellten Haelfte **nicht** geschlossen geblieben.
- **Fix:** Task 2 hat nur **gemessen** und nichts geaendert — die Prohibition des Plans
  (*"prueft nur nach und aendert dort nichts"*) gilt woertlich. Die Korrektur der stalen Zahl ist in
  den ROADMAP-Schreibschritt dieses Plans gelegt, in dem ein ROADMAP-Schreibzugriff ohnehin
  vorgesehen ist. Die Zahl liest danach dieselbe wie die vom Werkzeug gefuehrte.
- **Files modified:** `.planning/ROADMAP.md`
- **Verification:** `sed -n '81p' .planning/ROADMAP.md | grep -oE '[0-9]+/[0-9]+ (plans executed|ausgeführt)'`
  liefert danach zwei uebereinstimmende Zahlen.
- **Committed in:** der `docs(02-31)`-Abschlusscommit
- **Kein Ledger-Eintrag:** ausdruecklich nicht. Die Erfolgskriterien dieses Plans nageln die
  Endsignatur auf `58 open / 0 waived / 16 fixed / 74 total` fest; ein vierter Eintrag haette sie
  gebrochen. Der Befund ist hier aufgezeichnet und gehoert der naechsten Verifikationsrunde, die
  entscheidet, ob die Zahl von Hand oder gar nicht gefuehrt werden soll.

**2. [Rule 2 - Missing Critical] Eintrag 72 nennt die Zahl `2048` gar nicht mehr**

- **Found during:** Task 1 (beim Abgleich des Entwurfs gegen die Abnahmebedingung)
- **Issue:** Der Entwurf von Eintrag 72 nannte beide Zahlen — *"das Dokument schreibt 2048, gemessen
  sind es 2046"*. Die Abnahmebedingung des Plans lautet aber woertlich *"Der Text von Eintrag 72
  traegt `2046` und nicht `2048`"*, und ein maschineller Check auf die Zeichenkette `2048` haette sie
  im Eintrag gefunden.
- **Fix:** Die Zahlenkorrektur steht ausschliesslich im Nachtrag am Entscheidungsdokument, wo der
  Plan sie ausdruecklich mit **beiden** Zahlen verlangt. Eintrag 72 traegt nur `2046` und verweist
  fuer die Korrektur auf den Nachtrag.
- **Files modified:** `.planning/WINDOWS.md`
- **Verification:** `grep -c '2048'` ueber den Text von Eintrag 72: `0`; `2046` kommt dreimal vor.
- **Committed in:** `890468d`

---

**Total deviations:** 2 auto-fixed (1 Bug, 1 Missing Critical)
**Impact on plan:** Beide sind Ehrlichkeitskorrekturen an Registern und beruehren keinen Code und
keine Erfolgsbedingung. Kein Scope Creep: Deviation 1 wird gemeldet und in dem einen Schritt
korrigiert, in dem ROADMAP ohnehin geschrieben wird, und bekommt ausdruecklich **keinen** vierten
Ledger-Eintrag, weil die Endsignatur der Runde festgenagelt ist.

## Issues Encountered

Keine, ausser den beiden oben. Die drei `append`-Aufrufe haben die erwarteten Ids 72, 73 und 74
zurueckgegeben, so dass der Vorwaertsverweis von Eintrag 72 auf Eintrag 74 (*"die Geschwister
bekommen ihren eigenen Eintrag … Eintrag 74"*) beim Schreiben von 72 zwar noch nicht existierte,
beim Abschluss des Plans aber zutrifft und maschinell bestaetigt ist.

## Was diese Runde ausdruecklich NICHT geleistet hat

- **G-02-29 ist nicht geschlossen.** Eintrag 70 bleibt offen, Eintrag 72 amendiert ihn und bleibt
  ebenfalls offen. Der Defekt ist verengt, nicht behoben: eine `REFUSED_RANGES`-Deklaration, die nur
  in einem Kommentar, einem Template-Literal oder hinter einer Ableitung lebt, wird weiterhin
  gelesen.
- **G-02-9 ist offen.** Nichts an den Eintraegen 58 und 66 wurde beruehrt.
- **Der Werkzeugdefekt hinter G-02-30 ist nicht repariert.** `~/.claude/gsd-core/` liegt ausserhalb
  dieses Repositories und ausserhalb des Umfangs dieser Phase. Der naechste `mark-complete`- oder
  `revert-phase`-Zyklus erzeugt ihn erneut, fuer diese Zeile und fuer die fuenfzehn anderen.
- **Die drei Geschwister-Leser sind gemessen, nicht repariert.** Ihre Reparatur ist die
  Schliessbedingung von Eintrag 74 und gehoert einer spaeteren Runde.
- **Die Policy-Frage des Betreibers ist nicht entschieden.** Diese Runde behebt WR-03, WR-04 und
  WR-05 (Plan 02-29) und erledigt IN-05 (Eintrag 72 zeigt auf ein Symbol) und IN-01 (die neue
  Abschneide-Meldung traegt keine Zahl mehr) als Nebenwirkung — **fuenf Punkte verschwinden damit von
  der Liste**. Fuer den Rest (WR-06, IN-02, IN-03, IN-04, der Surrogate-Punkttest und die vier
  Warnungen der Vorvorrunde) wurden **keine** Eintraege angelegt: wo die Nicht-Gap-Befunde der
  Code-Reviews leben, ist die Entscheidung, die dem Betreiber gehoert. Sie faellt jetzt ueber einen
  kuerzeren Rest.
- **Die drei Hinweiszeilen ueber `images.tsx:105`** sind bewusst nicht geschrieben worden, weil sie
  rund ein Dutzend `images.tsx:NNN`-Verweise im Planungsbestand ungueltig machten. Damit fehlt der
  Verengung ihr einziges Geraet, das den Menschen erreicht, der die Leiche erzeugt. Steht als Rest 4
  in Eintrag 72.

## Known Stubs

Keine. Dieser Plan schreibt keinen Code.

## Threat Flags

Keine neue sicherheitsrelevante Oberflaeche. Die beiden Bedrohungen des Plans (T-02-132 Repudiation,
T-02-133 Tampering) sind mitigiert und maschinell nachgewiesen: alle drei neuen Eintraege stehen auf
`open`, `waived_count` ist 0, kein `overrides:`-Block existiert, und die Register-Korrektur ist in
beiden Richtungen gezaehlt (14 Zeilen `Gaps Found`, 0 Zeilen `Complete`).

## Id-Vergabe

**Naechste freie Ledger-Id: 75. Naechste freie Luecken-Id: G-02-31. Naechste freie Threat-Id:
T-02-134.**

## User Setup Required

Keine — keine externe Dienstkonfiguration.

## Next Phase Readiness

Runde 7 ist mit diesem Plan vollstaendig ausgefuehrt (02-29, 02-30, 02-31). Was die naechste
Verifikationsrunde vorfindet:

- **Drei neue offene Ledger-Eintraege mit pruefbaren Schliessbedingungen.** Eintrag 72s Bedingung
  kann diese Runde widerlegen: Punkt 6 verlangt den ausdruecklichen Versuch, eine siebte Form zu
  bauen, die die Ausschlussliste nicht nennt.
- **Ein Release-Blocker-Register, das keine Zeile mehr auf `Complete` fuehrt**, waehrend die Phase
  `gaps_found` traegt — und ein geledgerter Werkzeugdefekt, der erklaert, warum das von Hand
  hergestellt werden musste.
- **Eine offene Richtungsfrage.** `02-DECISION-drift-guard-lexik.md` traegt weiterhin `offen —
  vorgelegt, nicht ratifiziert`. Runde 7 hat auf einer delegierten Wahl ausgefuehrt; der Betreiber
  kann Variante A weiterhin waehlen, was seit dem 2026-09-05 einen Plan statt eines Satzes kostet.
- **Ein Befund fuer die naechste Runde ohne eigenen Ledger-Eintrag:** die nachgestellte Zahlenangabe
  in `ROADMAP.md` Zeile 81 wird von keinem Werkzeug gefuehrt und driftet nach jeder Welle erneut.
  Ob sie von Hand gefuehrt oder ganz entfernt werden soll, ist offen (Deviation 1).

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-09-05*

## Self-Check: PASSED

- Alle vier in `key-files` genannten Dateien existieren auf der Platte (`[ -f ]` je einzeln).
- Alle drei Task-Commit-Hashes (`890468d`, `efb550c`, `26c3ddb`) sind in `git log --oneline --all`
  auffindbar.
- Alle `<acceptance_criteria>` der drei Tasks und die Plan-`<verification>` wurden nach dem letzten
  Task erneut gefahren: Ledger `58/0/16/74` in drei uebereinstimmenden Repraesentationen, `"ok": true`,
  14 Phase-2-Zeilen `Gaps Found` und 0 `Complete`, Statusblock und Praefix-Hash des
  Entscheidungsdokuments unveraendert, `git diff --quiet HEAD -- internal/ cmd/ web/` gruen,
  `git status --porcelain` ohne Pfade ausserhalb von `.planning/`.
- Einzige nicht erfuellte Abnahmebedingung: die ROADMAP-Annahme aus Task 2 — gemeldet, gemessen und
  als Deviation 1 dokumentiert statt still ueberschrieben.
- `requirements-completed` ist leer.
