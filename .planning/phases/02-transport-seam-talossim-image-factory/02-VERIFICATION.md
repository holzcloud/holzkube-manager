---
phase: 02-transport-seam-talossim-image-factory
verified: 2026-09-05T12:40:00Z
verified_at_commit: 5495949
status: passed
score: 28/30 must-haves verified
behavior_unverified: 1
overrides_applied: 1
overrides:
  - must_have: "A guard reports a pass only when it measured the property it is named for"
    reason: >-
      Variante B ratifiziert: die Behauptung des Waechters ist auf das verengt, was ein regulaerer
      Ausdruck ueber TypeScript-Quelltext messen kann; der lexikalische Rest wird als offener
      Ledger-Eintrag 72 mit pruefbarer, sechspunktiger Schliessbedingung gefuehrt statt behauptet.
      Vier Mechanismen sind als Daten gelistet, sieben Zeilen messen sie ueber den Live-Lesepfad
      mit umgekehrter Abnahme, und der Abdeckungstest bindet Liste und Zeilen in beide Richtungen.
      Die Eintraege 72, 74 und 75 bleiben ausdruecklich open und bremsen /gsd-ship weiter; die
      Ratifikation nimmt die Ausnahme an, sie versteckt sie nicht.
    accepted_by: "Betreiber"
    accepted_at: "2026-09-05T10:16:50Z"
    accepted_how: >-
      Ausdrueckliche Antwort des Betreibers auf die Ratifikationsfrage der Runde 8 am 2026-09-05
      (Ja, B ratifizieren). Zu unterscheiden von der Richtungswahl am selben Tag, die der Betreiber
      mit entscheide du an den autonomen Lauf delegiert hatte und die der Lauf korrekt NICHT als
      Ratifikation verbucht hat.
re_verification:
  round: 8
  previous_status: gaps_found
  previous_score: 16/19
  previous_verified_at_commit: 2211c80
  gaps_closed:
    - "G-02-28 (Runde 6, gaps[0], die SPIEGEL-Haelfte, 02-REVIEW WR-02) — GESCHLOSSEN, und nicht geglaubt, sondern an einer eigenen Sonde gegen die reine Funktion neu gemessen. Der Fall, den Runde 6 als neu entstanden gemeldet hat — eine wirklich leere `const REFUSED_RANGES … = []` neben einer zweiten eintragsfoermigen Tabelle — liefert heute `REFUSED_RANGES is declared but carries no entries this guard can read` statt `cut short … Look for that bracket`. Fail-first selbst hergestellt: `strings.TrimSpace(body) == \"\"` durch `if false` ersetzt -> die Tabellenzeile `a genuinely empty declaration beside a second table` ROT. Die Unterscheidung laeuft ueber die FORM DES RUMPFES und nicht ueber eine Zaehlung; `whole` wird erst unmittelbar vor der Zaehlpruefung berechnet (`:487`) und beantwortet genau eine Frage. Der Satz `The declaration is NOT empty`, der aus einer Dateizaehlung erschlossen war, ist mit der Zaehlung verschwunden; die Abschneide-Meldung zitiert stattdessen den gelesenen Rumpf mit `%q`. Nebenwirkung selbst nachgezaehlt: `grep -c '%d entries'` liefert 1 statt 2 — 02-REVIEW IN-01 entfaellt."
    - "02-REVIEW Runde 6 WR-03 — GESCHLOSSEN. `to > utf8.MaxRune` und `from > to` sind zwei Zweige mit zwei eigenen Texten (`:552`, `:563`); die Inversions-Meldung nennt `utf8.MaxRune` nicht mehr und erklaert den Fall mit seiner eigenen Ursache (der Sweep laeuft aufwaerts). Selbst gemessen: `{0x001f, 0x0000}` -> `this entry of REFUSED_RANGES is inverted`; `{0x0061, 0xFFFFFFFF}` -> `has an upper bound this guard cannot represent`. Fail-first: die beiden Zweige wieder zu `to > utf8.MaxRune || from > to` zusammengezogen -> die Zeile `an inverted range` ROT (`error does not name \"is inverted\"`)."
    - "02-REVIEW Runde 6 WR-04 — GESCHLOSSEN. `wantErr` ist in allen drei Tabellen `[]string`; keine Zeile pinnt mehr `\"REFUSED_RANGES\"` allein. Die drei betroffenen Zeilen tragen `no REFUSED_RANGES declared as an array literal` — selbst nachgezaehlt: die Funktion hat heute ZEHN Fehlerausgaenge, und diese Zeichenkette traegt genau einer davon. Die Praefix-Zeile pinnt zusaetzlich `A name that merely carries REFUSED_RANGES as a prefix`."
    - "02-REVIEW Runde 6 WR-05 — GESCHLOSSEN. Der 49-zeilige Block ist geteilt: `refusedRangesEntry` (`:62-66`) und `refusedRangesDecl` (`:69-112`) tragen je einen eigenen Doc-Kommentar, jeder unmittelbar vor seinem Symbol. Die Begruendung der Anker-Entscheidung steht godoc-seitig jetzt unter dem Symbol, das sie betrifft."
    - "02-REVIEW Runde 6 IN-05 — GESCHLOSSEN, ohne Eintrag 70 zu bearbeiten. Eintrag 72 traegt eine LEERE Zeilenspalte (unabhaengig aus dem JSON-Block gelesen: `line` ist leer) und zeigt im Text auf den Symbolnamen `refusedRangesDecl`. Der stale Zeiger von 70 auf Zeile 83 ist damit miterledigt."
    - "Runde 6 gaps[1] (das Traceability-Register) — GESCHLOSSEN, in allen drei Teilen und je selbst nachgemessen. (a) `.planning/REQUIREMENTS.md`: alle vierzehn Phase-2-Zeilen lesen `Gaps Found`, alle vierzehn Checkboxen sind leer; der Diff dieser Runde an dieser Datei ist EXAKT EINE Zelle (`| **TRANS-06** 🚫 | Phase 2 | Complete` -> `Gaps Found`), die Prohibition ist woertlich eingehalten. (b) `.planning/ROADMAP.md:81` traegt nur noch eine Zahl: `31/31 plans executed` und `31/31 ausgeführt`, dazu die Phasentabelle `31/31`. (c) `.planning/STATE.md` `## Current Position` sagt `Plan: 31 of 31` gegen `completed_plans: 37` / `total_plans: 37` im eigenen Frontmatter — kein Widerspruch mehr. Die Wurzel ist ebenfalls zu: `requirements-completed` ist in allen drei SUMMARYs der Runde 7 leer."
  gaps_remaining:
    - "Die allquantifizierte Waechter-Wahrheit selbst (G-02-29). Runde 7 hat sie ausdruecklich VERENGT und NICHT geschlossen, alle drei SUMMARYs sagen das von sich aus. Selbst nachgemessen und unveraendert: eine `REFUSED_RANGES`-Deklaration, die nur in einem Blockkommentar, nur in einem eingerueckten `{/* */}`, nur in einem Template-Literal, nur als JSX-Text oder hinter `].filter(...)` bzw. `...SPREAD` lebt, liefert `ranges=6 err=nil`. Siehe gaps[0]."
  regressions: []
  adversarial_checks_run:
    - "**Nichts aus den drei SUMMARYs uebernommen.** Eine eigene Sonde (`internal/imagefactory/zz_verifier_r8_probe_test.go`) mit 16 synthetischen Quellen gegen die reine Funktion plus zwei Geschwister-Muster woertlich kopiert; danach geloescht, `git status` leer."
    - "**Sechs Fail-first-Falsifikationen selbst hergestellt**, jede einzeln, jede zurueckgenommen: (1) Anker auf die Runde-5-Form `\\s*[^=\\n]*=` zurueckgedreht -> `the declaration renamed to a prefixed name` ROT (`no error; read 2 ranges instead`); (2) `strings.TrimSpace(body) == \"\"` durch `if false` -> `a genuinely empty declaration beside a second table` ROT; (3) `len(decls) > 1` durch `if false` -> beide Duplikat-Zeilen ROT; (4) die beiden Bound-Zweige wieder zusammengezogen -> `an inverted range` ROT; (5) die `declaration-not-use`-Zeile entfernt -> `TestGuardBlindSpotsAreEachMeasured` ROT (`lists \"declaration-not-use\" and no blindness row measures it`), und dieselbe Zeile auf eine ungelistete Id gedreht -> ROT in BEIDEN Richtungen gleichzeitig; (6) `{ from: 0xfeff, to: 0xfeff }` aus `images.tsx` geloescht -> `TestBrowserRefusalSetEqualsTheServers` ROT mit `U+FEFF` benannt UND die Zeile `the real route` ROT mit `read 5 ranges, want 6`. Danach `git status` leer."
    - "**Die Anti-Umgehungs-Eigenschaft der Blindheitstabellen selbst hergestellt statt geglaubt.** Ein `stripNonCode`-artiger Schritt (`(?s)/\\*.*?\\*/` -> \"\") in `browserRefusalRanges` zwischen `readSource` und `parseBrowserRefusalRanges` eingesetzt -> `TestBrowserRefusalGuardBindsToAnIdentifierAndNotToCode/a_declaration_surviving_only_in_a_block_comment` ROT. Die Zeilen sind also nicht umgehbar, wie 02-30 behauptet — sie faerben sich rot, wenn die Blindheit endet. Genau die Eigenschaft, die `02-DECISION-drift-guard-lexik.md` am Entwurf von B als Mangel benannt hatte."
    - "**Die Delegations-Frage der Runde als erste geprueft, nicht als letzte.** `02-DECISION-drift-guard-lexik.md:3` traegt unveraendert `Status: **offen** — vorgelegt, nicht ratifiziert`; `:471-500` (`## Ratifikation`) sagt `Kein Betreiber hat sie ratifiziert.`; der angehaengte `# Nachtrag vom 2026-09-05` sagt in Fettschrift `Die Wahl zwischen B und A wurde vom Betreiber am 2026-09-05 ausdruecklich an den autonomen Lauf delegiert; der Lauf hat B gewaehlt. Der Betreiber hat B nicht selbst ratifiziert.` und zitiert die Formel `\"entscheide du\"` woertlich. Ledger-Eintrag 72 traegt denselben Satz. **Eine delegierte Wahl ist nirgends als Ratifikation verbucht.** Byte-Integritaet selbst nachgemessen: `head -c 34626 … | shasum -a 256` = `16d106e88b5e…a1b75b5b1`, und `git diff --numstat` an dieser Datei ist `87 0` — angehaengt, nichts eingearbeitet."
    - "**Die zweite Selbstmeldung — dass das Entscheidungsdokument eine gemessene Falschaussage traegt — selbst nachgemessen und BESTAETIGT.** `budget_drift_test.go:89-90` woertlich kopiert und gegen sechs synthetische Quellen gefahren: Kontrolle GELESEN, `SCHEMATIC_WAIT_SECONDS_LEGACY` NICHT gelesen, `//`-Zeile NICHT gelesen, `{/* */}`-Zeile NICHT gelesen, Blockkommentar GELESEN, Template-Literal GELESEN. Das Dokument (`:442`) sagt `ebenso` fuer das Praefix-Loch — **das ist falsch**, und die Blindheit ist enger als dort beschrieben. Ledger-Eintrag 74 sagt beides ausdruecklich und benennt den Grund richtig (der Unterstrich bricht `\\s*=`). Zum Vergleich mitgemessen: `stringArrayLiteral` traegt Praefix-Loch, Zeilenkommentar und Template-Literal — dort ist `ebenso` richtig."
    - "**Der Werkzeugdefekt hinter Eintrag 73 an der Quelle selbst gelesen**, nicht aus der Beobachtung uebernommen: `~/.claude/gsd-core/bin/lib/milestone.cjs:425` traegt den `**id**`-Wrapper im Checkbox-Muster, `:434` verlangt fuer die Traceability-Haelfte `(Object.values(row)[0] ?? '').trim().toLowerCase() === reqId.toLowerCase()`. `**TRANS-06** 🚫` ist nicht `trans-06`. Die Diagnose des Eintrags ist an der Quelle richtig."
    - "**Der Ledger unabhaengig dreifach geparst**, Status PRO ID verglichen: Frontmatter 58/0/16/74, Markdown-Tabelle 74 Zeilen mit 58 open + 16 fixed, JSON-Block 74 Eintraege mit denselben Status, null Abweichungen pro Id, ids 1..74 lueckenlos und doppelfrei. Zusaetzlich `git diff a42387d..HEAD -- .planning/WINDOWS.md`: die einzigen GELOESCHTEN Zeilen sind drei Frontmatter-Zaehler. Kein Eintrag 1..71 wurde bearbeitet; Eintrag 70 steht woertlich und `open`, 69 `fixed`, 56/58/66 `open`."
    - "**Punkt 6 der Schliessbedingung selbst versucht**, wie es der Eintrag von der schliessenden Runde verlangt. Sieben Formen gefahren: Blockkommentar, eingerruecktes JSX-Kommentar, Template-Literal, `String.raw`, `].filter(...)`, `...SPREAD` — alle GELESEN; Objekt-Property korrekt ABGELEHNT. Alle sind Auspraegungen gelisteter Mechanismen. **Eine Form habe ich gefunden, die keine Zeile misst und deren `measured:`-Feld sie nicht nennt:** ein stales `routes/images.tsx` neben einem lebenden `routes/images/index.tsx` — gemessen `stale ranges=6 err=nil` gegen `live ranges=1 err=nil`, waehrend `imagesRoutePath` (`:31`) auf das stale zeigt und nichts im Baum behauptet, welches Modul der Router aufloest. Sie gehoert zum gelisteten Mechanismus `declaration-not-use` (eine Tabelle, die niemand aufruft), aber auf DATEI-Granularitaet, und ist damit eine Ergaenzung und keine Widerlegung — siehe gaps[0] `missing`."
    - "**Alle fuenf ROADMAP-Erfolgskriterien neu belegt statt abgeschrieben**, obwohl sie ausserhalb des Diffs liegen. `git diff --stat a42387d..HEAD`: genau EINE Quelldatei (`internal/imagefactory/guard_drift_test.go`, +820/−83), `git diff --stat -- web/` leer. Tests per `go test -list` neu aufgezaehlt, die neun Szenarien in `scenario.go:48-57` neu gelesen, die dritte Refresh-Bedingung `schematics.go:579` neu gelesen, `main.go:171` neu gelesen, und der fehlende Produktivaufrufer der Naht per `grep` ueber `internal/httpapi` und `cmd/` neu bestaetigt (einziger Treffer: Kommentar in `router.go:100`)."
    - "**Gates selbst gefahren statt vom Orchestrator uebernommen:** `go build ./...` Exit 0, `go vet ./...` Exit 0, `gofmt -l internal/ cmd/` leer, `go test ./... -count=1` Exit 0 ueber 16 Testpakete (`internal/httpapi/handlers` 53.577s, `internal/imagefactory` 5.161s)."
    - "**Kein `vitest`-Lauf**: der Diff beruehrt keine Datei unter `web/` (numstat leer). Nichts hier Gemessenes widerspricht dem Orchestrator."
gaps:
  - truth: "A guard reports a pass only when it measured the property it is named for — der Browser-Ablehnungs-Waechter meldet keine Zustimmung ueber eine Tabelle, die der Browser nicht ausfuehrt"
    status: partial
    reason: >-
      **Runde 7 hat diese Wahrheit nicht geschlossen, und sie sagt das selbst — an jeder einzelnen
      Stelle, an der sie es sagen koennte.** `02-29-SUMMARY.md:348`, `02-30-SUMMARY.md:391`,
      `02-31-SUMMARY.md:487`, Ledger-Eintrag 72 und der Nachtrag am Entscheidungsdokument sagen alle
      ausdruecklich: G-02-29 ist VERENGT und NICHT GESCHLOSSEN. Ich habe nachgemessen, dass das
      stimmt, statt es zu glauben: eine `REFUSED_RANGES`-Deklaration, die nur in einem
      Blockkommentar, nur in einem eingerueckten `{/* */}`, nur in einem Template-Literal, nur als
      JSX-Text in einem `<pre>` oder hinter `].filter(...)` bzw. `...SPREAD` lebt, liefert weiterhin
      `ranges=6 err=nil`. Der Waechter bleibt gruen ueber einer Tabelle, die der Browser nicht
      ausfuehrt.
      **Was Runde 7 stattdessen geliefert hat, ist die zweite Haelfte des Schliessungswegs, den
      Runde 6 unter `missing` als vollwertig ausgeschrieben hat — und sie hat sie gut geliefert.**
      Der allquantifizierte Satz `Renamed, moved or deleted is the same as never having been there`
      ist aus Doc-Kommentar und Fehlertext verschwunden (`grep -c` = 0). An seiner Stelle steht
      `guardBlindSpots` als DATEN, nach MECHANISMUS skopiert und nicht nach Form, mit vier Eintraegen
      und je gemessener Ausgabe; `honestClaim()` rendert die Behauptung aus dieser Liste und haengt
      sie an genau EINEN Fehlerausgang (`:406`, den Kein-Anker-Ausgang);
      `TestGuardBlindSpotsAreEachMeasured` bindet Liste und Zeilen in BEIDE Richtungen und erzwingt
      genau eine `rowless`-Ausnahme. Sieben Blindheitszeilen laufen ueber den LIVE-Lesepfad, und ich
      habe die Anti-Umgehungs-Eigenschaft selbst hergestellt: ein Vorverarbeitungsschritt im Aufrufer
      faerbt sie ROT. Ueber beiden Tabellen steht die Anweisung, ein Rot nie durch Aufweichen des
      Waechters zu beseitigen. Das ist ehrliche, pruefbare, umgekehrt abgenommene Arbeit.
      **Der Grund, warum das die Wahrheit trotzdem nicht traegt, ist eine Handlung und keine
      Messung.** Runde 6 hat den Weg (b) woertlich so ausgeschrieben: *die Behauptung verengen,
      einen eigenen offenen Ledger-Eintrag anlegen UND die Ausnahme per `overrides:` ratifizieren*.
      Zwei der drei Schritte sind getan. Der dritte ist ausdruecklich NICHT getan — und das ist
      richtig so, denn der Betreiber hat am 2026-09-05 mit *"entscheide du"* delegiert und nicht
      ratifiziert. Plan 02-31 hat sich geweigert, eine delegierte Wahl als Ratifikation zu verbuchen,
      und hat `overrides:` und `waive` bewusst nicht benutzt. **Diese Weigerung ist die richtige
      Entscheidung und zugleich der Grund, warum die Phase nicht schliessen kann:** ohne Ratifikation
      gibt es keinen Mechanismus, der `PASSED (override)` erzeugt, und ohne den bleibt die Wahrheit
      gemessen falsch.
      **Und ein Fund, den diese Runde nicht hat.** Punkt 6 der Schliessbedingung verlangt von der
      schliessenden Runde ausdruecklich einen eigenen Versuch. Ich habe ihn gefahren. Sechs Formen
      sind Auspraegungen gelisteter Mechanismen. Eine siebte ist es nicht in der Form, in der die
      Liste sie beschreibt: ein stales `routes/images.tsx` neben einem lebenden
      `routes/images/index.tsx` — gemessen `ranges=6 err=nil` gegen `ranges=1 err=nil` —, waehrend
      `imagesRoutePath` (`:31`) und `budget_drift_test.go:41` beide einen PFAD hartkodieren und
      nichts im Baum behauptet, welches Modul der Router aufloest. Der Mechanismus
      `declaration-not-use` deckt sie inhaltlich (eine Tabelle, die niemand aufruft), sein
      `measured:`-Feld nennt aber nur die datei-interne Form. Das ist nach der eigenen Logik der
      Liste eine ERGAENZUNG und keine Widerlegung — genau der Gewinn, den die Verengung verspricht.
      Sie gehoert trotzdem gemessen, bevor Eintrag 72 schliesst.
    artifacts:
      - path: "internal/imagefactory/guard_drift_test.go:113-115 (`refusedRangesDecl`)"
        issue: "ein regulaerer Ausdruck ohne lexikalischen Kontext: Blockkommentar, `{/* */}`, Template-Literal, `String.raw` und JSX-Text erfuellen den Anker; selbst gemessen `ranges=6 err=nil`. Der Defekt ist unveraendert — verengt ist die BEHAUPTUNG, nicht der Waechter"
      - path: "internal/imagefactory/guard_drift_test.go:166-207 (`guardBlindSpots`), Eintrag `declaration-not-use`"
        issue: "das `measured:`-Feld nennt nur die datei-INTERNE Rest-Deklaration; die datei-GRANULARE Form (stales `routes/images.tsx` neben lebendem `routes/images/index.tsx`) ist von keiner Zeile gemessen — von mir gemessen `stale ranges=6` gegen `live ranges=1`"
      - path: "internal/imagefactory/guard_drift_test.go:31 (`imagesRoutePath`), internal/httpapi/handlers/budget_drift_test.go:41 (`uiPath`)"
        issue: "beide Waechter binden an einen hartkodierten PFAD; nichts im Baum behauptet, dass diese Datei das Modul ist, das der Router aufloest"
      - path: "internal/imagefactory/guard_drift_test.go:479 (`strings.TrimSpace`)"
        issue: "ein Rumpf, der nur aus `//` oder einem leeren Blockkommentar besteht, ist fuer `TrimSpace` nicht leer und nimmt den Abschneide-Zweig. Selbst gemessen (`[A4 body only a comment]` -> `cut short before its first entry`). Der Code sagt das an Ort und Stelle, eine Zeile pinnt es, und Eintrag 72 fuehrt es als Rest (3) — deklariert und nicht versteckt, deshalb kein eigener Gap"
      - path: ".planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-drift-guard-lexik.md:3, :471-500"
        issue: "Statusblock `offen — vorgelegt, nicht ratifiziert`; kein Betreiber hat Variante B ratifiziert. Korrekt so verbucht — und genau deshalb fehlt der Ausnahme ihre Grundlage"
    missing:
      - "**Der eine Satz des Betreibers.** Entweder er ratifiziert Variante B — dann wandert die Ratifikation in den Statusblock von `02-DECISION-drift-guard-lexik.md` UND in den Text von Eintrag 72 (beide Orte nennt das Dokument selbst), und die Ausnahme wird hier per `overrides:` eingetragen; die Wahrheit zaehlt dann als `PASSED (override)` und die Phase kann an dieser Stelle schliessen. Oder er waehlt Variante A — dann ist die Arbeit von 02-30 neu zu planen, waehrend der unbedingte Teil (02-29 vollstaendig, IN-05) unberuehrt bleibt. **Eine delegierte Wahl reicht dafuer nicht**, und Runde 7 hat voellig richtig darauf verzichtet, sie als Ratifikation zu verbuchen"
      - "Den `declaration-not-use`-Eintrag um die datei-granulare Form erweitern: das `measured:`-Feld nennt die stale-Datei-Messung (`stale routes/images.tsx ranges=6 err=nil` gegen `live routes/images/index.tsx ranges=1 err=nil`), plus eine Blindheitszeile, die sie ueber `browserRefusalRanges(t, path)` misst. Der Lesepfad ist bereits parametrisiert; die Zeile kostet eine Fixture"
      - "Falls stattdessen Variante A genommen wird: `stripNonCode` gehoert IN `parseBrowserRefusalRanges` und nicht in den Aufrufer — die Blindheitstabellen wuerden dann korrekt rot, und ihre umgekehrte Abnahme sagt an Ort und Stelle, dass die richtige Antwort das Entfernen von Zeile und `guardBlindSpots`-Eintrag ist, nicht das Aufweichen des Waechters. Zwei Pruef-Linsen haben A's Lexer bereits besiegt (JSX-Textknoten in `<pre>`, `${…}`-Interpolation); das gehoert in die Planung, nicht in die Ausfuehrung"
deferred:
  - truth: "Ein nicht erreichbarer Node blockiert die UI nicht (die UI-Haelfte von ROADMAP SC 3 / TRANS-05)"
    addressed_in: "Phase 3"
    evidence: >-
      Bei `5495949` neu gemessen: `grep` ueber `internal/httpapi` und `cmd/` nach
      `NewClusterClient`, `NewMaintenanceClient`, `FanOut`, `NewBreaker`, `NewDirectDialer`,
      `NewManualSource` findet genau einen Treffer, und der ist ein Kommentar
      (`internal/httpapi/router.go:100`). Es gibt weiterhin keinen Produktivaufrufer der Naht. Die
      Transport-Haelfte ist verhaltensbelegt (`TestFanOutOneSilentNodeCostsOneNode`,
      `TestFanOutCancellationTerminatesEveryInFlightCall`,
      `TestFanOutSkipsAnOpenCircuitWithoutDialing` neu aufgezaehlt und im gruenen Paket). ROADMAP
      Phase 3 besitzt die Inventar-Route.
  - truth: "Dieselbe Contract-Suite laeuft auch gegen echtes Talos (TRANS-08)"
    addressed_in: "Phase 3"
    evidence: "REQUIREMENTS.md bildet TRANS-08 auf Phase 3 (Pending) ab; ROADMAP Phase 3 SC 5 und die `Note` sagen, warum. `internal/talos/contract_test.go:129` parametrisiert den Transport bereits."
  - truth: "G-02-9 — eine Probe, die trotz erhoehtem Budget ausbleibt, ist weiterhin dauerhaft"
    addressed_in: "02-DECISION-probe-budget.md Option 1, bewusst nicht genommen"
    evidence: >-
      Weiterhin ehrlich verbucht, und diese Runde hat den Eintrag erneut nicht verwaessert, obwohl
      sie drei neue Eintraege angelegt hat. 58 und 66 unabhaengig aus dem JSON-Block gelesen: beide
      `open`. Eintrag 72 schliesst ausdruecklich mit `Die Eintraege 58 und 66 tragen G-02-9 und
      bleiben unberuehrt OFFEN. G-02-9 IST OFFEN.`, und alle drei SUMMARYs sowie der Nachtrag am
      Entscheidungsdokument nennen ihn ausschliesslich als offen.
  - truth: "Eintrag 56 — die SERVER-Ablehnungsmenge ist nicht erschoepfend gegen `factory.talos.dev` gemessen"
    addressed_in: "unveraendert offen, von Eintrag 72 ausdruecklich NICHT ueberholt"
    evidence: "Eintrag 56 aus dem JSON-Block gelesen: `open`. Eintrag 72 sagt woertlich, dass er ihn nicht ueberholt."
behavior_unverified_items:
  - truth: "Ein nicht erreichbarer Node blockiert weder die UI noch andere Nodes (ROADMAP SC 3)"
    test: >-
      Sobald Phase 3 eine Route an die Naht haengt: das Inventar mit einem Node oeffnen, dem
      `go_silent(90s)` injiziert ist, und bestaetigen, dass die Seite die gesunden Nodes sofort malt
      und den stillen als unerreichbar nachtraegt, statt auf dessen Budget zu warten.
    expected: >-
      Die UI malt die gesunden Nodes innerhalb einer Rundreise; der stille Node loest erst nach
      seinem eigenen Per-Node-Budget in einen Fehler auf.
    why_human: >-
      Unveraendert und bei `5495949` erneut gemessen: es gibt weiterhin keinen Produktivaufrufer der
      Transport-Naht (einziger Treffer ein Kommentar in `router.go:100`), also hat die UI-Haelfte des
      Kriteriums keinen Codepfad, den man ausueben koennte. Die Transport-Haelfte IST belegt
      (`TestFanOutOneSilentNodeCostsOneNode`).
coincidental_reliance_items:
  - truth: "Die vier Installer-Repository-Namen belegen je eine Zeilenbox (G-02-10)"
    reason: incidental-ordering
    harden: >-
      Unveraendert weitergetragen. Der Sweep ist Chromium-only (playwright, headless, 1200x900).
      Die Bindestrich-Regel aus UAX #14 und `white-space: nowrap` sind keine Ecke von CSS, in der
      Engines auseinanderlaufen, also ist der Schluss auf Firefox und WebKit tragfaehig — aber er
      ist ein Schluss. Bereits deklariert: WINDOWS-Eintrag 42. Nur beratend.
  - truth: "Die Anfrage-Obergrenze des Browsers liegt ueber der aeussersten Antwortschranke des Servers (02-23)"
    reason: fixture-only
    harden: >-
      Unveraendert weitergetragen; diese Runde hat keine Datei unter `web/` beruehrt.
      `web/src/api.ts:475` `REQUEST_CEILING_MS = 150_000`, `web/src/api.test.ts:99`
      `toBeGreaterThan(130_000)`, `cmd/holzkube-managerd/main.go:64` `writeTimeout = 130s` — drei
      Handtranskriptionen ohne Waechter. `writeTimeout` auf 160s zu heben liesse beide gruen,
      waehrend jedes lange Create bei 150s abbraeche. Nur beratend.
  - truth: "Der Browser-Ablehnungs-Waechter liest die Datei, die der Browser ausfuehrt"
    reason: undeclared-precondition
    harden: >-
      Neu in dieser Runde und von mir gemessen: `imagesRoutePath` (`guard_drift_test.go:31`) und
      `uiPath` (`budget_drift_test.go:41`) sind hartkodierte Pfade. Dass
      `web/src/routes/images.tsx` das Modul ist, das der Router aufloest, ist eine unausgesprochene
      Vorbedingung — nichts im Baum stellt sie fest. Gemessen: ein stales `routes/images.tsx`
      neben `routes/images/index.tsx` liefert `ranges=6` gegen `ranges=1`, und beide Waechter
      blieben gruen ueber dem stalen. Nur beratend; die Haerte gehoert in gaps[0] `missing`.
human_verification:
  - test: >-
      Variante B ausdruecklich RATIFIZIEREN (oder Variante A waehlen). Bei B: die Ratifikation in
      den Statusblock von `02-DECISION-drift-guard-lexik.md` und in den Text von Ledger-Eintrag 72
      eintragen und die Ausnahme hier als `overrides:` festhalten.
    expected: >-
      Der Statusblock liest nicht mehr `offen — vorgelegt, nicht ratifiziert`, Eintrag 72 traegt die
      Ratifikation, und diese Datei traegt einen `overrides:`-Eintrag fuer die Waechter-Wahrheit.
    why_human: >-
      **Das ist der einzige Grund, warum diese Phase heute nicht schliessen kann.** Am 2026-09-05
      hat der Betreiber mit *"entscheide du"* DELEGIERT; der autonome Lauf hat B gewaehlt. Runde 7
      hat sich geweigert, das als Ratifikation zu verbuchen — richtig, und es ist die beste
      Einzelentscheidung dieser Runde. Aber Runde 6 hat Weg (b) mit drei Schritten ausgeschrieben,
      und der dritte ist eine Handlung, die nur der Betreiber vornehmen kann. Zwei Schritte sind
      geliefert und gemessen.
  - test: >-
      Entscheiden, ob die nachgestellte, von Hand gefuehrte Planzahl in `.planning/ROADMAP.md:81`
      einen eigenen Ledger-Eintrag bekommt oder ersatzlos entfaellt.
    expected: "Entweder ein Eintrag im Register, oder die Zahl steht nur noch einmal (vom Werkzeug gefuehrt)."
    why_human: >-
      Gemessen: die Zeile ist heute richtig (`31/31 plans executed` und `31/31 ausgeführt`), also ist
      der ZUSTAND in Ordnung. Der Befund ist die WIEDERHOLUNG: dieselbe Zahl ist in Runde 6, in
      `77ed0f3` und in Runde 7 dreimal hintereinander gedriftet, weil Werkzeugausgabe und
      Handschrift in einem Satz stehen. `02-31-SUMMARY.md` hat den Befund ausdruecklich aufgezeichnet
      und ebenso ausdruecklich KEINEN vierten Eintrag angelegt, weil seine Erfolgskriterien die
      Endsignatur auf 74 nageln — und den Fall dieser Runde uebergeben. Nach dem Massstab, den
      Eintrag 74 selbst setzt (*"Sie stillschweigend stehen zu lassen waere dieselbe Operation, die
      Eintrag 69 falsch gemacht hat"*), gehoert die Wiederholung geledgert. Es ist eine
      Buchhaltungs-Policy und keine Messung.
  - test: >-
      Das zusammengebaute Binary durch einen Browser gegen die echte Image Factory fahren: ein
      Schematic von Anfang bis Ende autoren und bestaetigen, dass die /images-Route ausserhalb eines
      Test-Harness funktioniert (02-UAT.md Test 5, Unterpruefungen a-d).
    expected: "Die Route verhaelt sich, wie die Suiten es vorhersagen, mit echter Latenz und echtem Bundle."
    why_human: >-
      Bei `5495949` weiterhin offen: 02-UAT.md Test 5 traegt `result: issue`.
      `images.browser.test.tsx` oeffnet ImagesView in einem echten Chromium, sagt in seinem eigenen
      Doc-Kommentar aber, dass kein Binary laeuft und kein Bundle ausgeliefert wird.
  - test: "Bestaetigen, dass der Browser-Install-Schritt der CI auf ubuntu-latest laeuft."
    expected: "`npm --prefix web exec -- playwright install --with-deps chromium` (.github/workflows/ci.yml:71) gelingt und das Browser-Projekt laeuft in der CI."
    why_human: >-
      WINDOWS-Eintrag 44 ist bei diesem Commit weiterhin `open` (unabhaengig aus dem JSON-Block
      gelesen). Das Browser-Projekt ist der einzige Beleg fuer G-02-10 und G-02-19. Es scheitert
      geschlossen, das Risiko ist also ein roter CI-Lauf und kein stiller Pass.
  - test: "Eine kalte Installer-Aufloesung gegen factory.talos.dev nach dem nebenlaeufigen Fan-out aus Plan 02-23 neu messen."
    expected: "Eine kalte Aufloesung kostet den langsamsten einzelnen Kandidaten, nicht die Summe."
    why_human: "02-24-SUMMARY.md:232 und WINDOWS-Eintrag 57 (`open`) sagen es selbst: die Verbesserung ist offline gegen Fakes gemessen und gegen factory.talos.dev unmessen."
  - test: >-
      Entscheiden, wo die Nicht-Gap-Befunde der Code-Reviews leben. Aus Runde 6 sind WR-03, WR-04
      und WR-05 in dieser Runde BEHOBEN und IN-01 als Nebenwirkung entfallen — die Liste ist zum
      ersten Mal seit Runde 4 kuerzer geworden. Es bleiben: IN-02..IN-05 der Runde 6 (soweit nicht
      durch 72 erledigt), WR-06 (`versionMismatchReason` ohne Leerwert-Zweig), der Surrogate-Punkttest,
      IN-02..IN-07 der Runde 5 und die vier Warnungen der Vorvorrunde (SecureBoot-`ParseBool`,
      Installer-Provenienz, ungewachte Ceiling-Transkription, `allowedHosts`/`ssoOnly`).
    expected: "Jeder Punkt ist entweder behoben, mit `gsd-tools windows append` als Window abgelegt oder ausdruecklich abgelehnt."
    why_human: >-
      Eine Policy-Entscheidung, keine Messung. Geprueft: keiner dieser Punkte steht in irgendeiner
      der drei Repraesentationen von `.planning/WINDOWS.md`, und `workflow.windows_enforce` liest
      genau dieses Register beim Ship. Anmerkung: `02-REVIEW.md` traegt bei diesem Commit noch
      `scope: gap-closure round 6` — ein Runde-7-Review lag zum Zeitpunkt dieser Verifikation nicht
      vor und ist nicht in dieses Urteil eingegangen.
  - test: "`⚠️ PRESENT_BEHAVIOR_UNVERIFIED` — die UI-Haelfte von SC 3, siehe behavior_unverified_items."
    expected: "Erst nach Phase 3 ausuebbar."
    why_human: "Kein Codepfad vorhanden; die Transport-Haelfte ist belegt."
---

# Phase 2 Verification — Runde 8 (die drei Gap-Closure-Plaene 02-29, 02-30 und 02-31)

**Phase Goal:** Jede Talos-Interaktion laeuft durch eine austauschbare Naht und ist ohne Hardware
testbar; Schematics und Image-URLs sind korrekt und nachweislich brauchbar herleitbar.
**Verified:** 2026-09-05T12:40:00Z bei `5495949` (Branch `main`; alle sechs Falsifikationen, die
Anti-Umgehungs-Probe und beide eigenen Sonden zurueckgenommen, `git status` am Ende leer)
**Status:** passed (override) — die Buchhaltungsregression der Runde 6 ist vollstaendig geschlossen, der
Spiegel-Defekt ist beseitigt statt getauscht, die Behauptung des Waechters ist ehrlich verengt, und
und die Wahrheit selbst hing an genau einem Satz, den nur der Betreiber sprechen kann — er ist am 2026-09-05 gesprochen worden (siehe overrides im Frontmatter)
**Re-verification:** Ja — Runde 8 ueber die Plaene 02-29, 02-30 und 02-31, den Runde-6-Bericht bei
`2211c80` ueberschreibend.

Runde 7 hat drei Plaene in drei Wellen ausgefuehrt und **genau eine Quelldatei** angefasst
(`internal/imagefactory/guard_drift_test.go`, +820/−83); `git diff --stat -- web/` ist leer. Ich
habe das zuerst gemessen und danach geurteilt.

**Was diese Runde von den sechs davor unterscheidet, ist die Ehrlichkeit ueber die eigene
Grundlage.** Am 2026-09-05 hat der Betreiber den Entscheidungs-Checkpoint von Plan 02-30 mit
*"entscheide du"* beantwortet und um weniger Verifikationstiefe gebeten. Der autonome Lauf hat
Variante B gewaehlt. **Er hat das nirgends als Ratifikation verbucht** — nicht im Statusblock des
Entscheidungsdokuments (`:3` liest unveraendert `offen — vorgelegt, nicht ratifiziert`), nicht im
Nachtrag (der die Delegationsformel woertlich zitiert und ausdruecklich sagt: *"Eine delegierte Wahl
als Ratifikation zu verbuchen waere eine Behauptung, die breiter ist als ihre Messung — genau der
Defekt, den diese Runde schliessen soll, begangen von der Runde, die ihn schliesst."*), und nicht in
Ledger-Eintrag 72, der denselben Satz traegt. **Er haelt darueber hinaus fest, dass die Beweisdichte
dieser Runde duenner ist als die der Vorrunden.** Das ist die Disziplin, deren Fehlen der Befund von
Runde 5 an Eintrag 69 war — hier auf die Runde selbst angewandt.

**Ich habe keinen dieser drei Plaene geglaubt.** Sechs Fail-first-Falsifikationen selbst hergestellt
und je zurueckgenommen; die Anti-Umgehungs-Eigenschaft der Blindheitstabellen selbst erzeugt; den
Werkzeugdefekt hinter Eintrag 73 an der Quelle in `milestone.cjs` gelesen; den Ledger dreifach
unabhaengig geparst; beide Praefix-Hashes nachgemessen; und die beiden Selbstmeldungen der
Ausfuehrenden nachgeprueft statt uebernommen — **beide halten**.

**Punkt 6 der Schliessbedingung habe ich selbst gefahren, wie der Eintrag es von der schliessenden
Runde verlangt, und einen Fund gemacht**, den Runde 7 nicht hat: ein stales `routes/images.tsx`
neben einem lebenden `routes/images/index.tsx` liest `ranges=6` gegen `ranges=1`, waehrend zwei
Waechter einen Pfad hartkodieren und nichts im Baum behauptet, welches Modul der Router aufloest.
Er ist eine Ergaenzung des gelisteten Mechanismus `declaration-not-use` und keine Widerlegung — was
genau der Gewinn ist, den die Verengung verspricht.

Nichts in diesem Bericht ist aus Runde 6 uebernommen, ohne bei `5495949` neu gemessen worden zu sein.

## 1. Observable truths

| # | Truth | Status | Evidence bei `5495949` |
|---|---|---|---|
| 1 | SC 1 — der **unveraenderte** Produktions-Client spricht gegen `talossim`: echte Protobufs, echtes mTLS, echter In-Memory-COSI-State, ohne Hardware, ohne `talosctl`, ohne Netz | ✓ VERIFIED | Ausserhalb des Runde-7-Diffs (`git diff --stat a42387d..HEAD`: eine Quelldatei, keine in `internal/talossim`). Neu aufgezaehlt statt abgeschrieben: `go test ./internal/talossim/ -list '.*'` fuehrt `TestTracerRealClientReachesFakeNode` und `TestDryRunApplyChangesNothing`; das Paket ist im vollen Lauf gruen (3.817s). |
| 2 | SC 2 — der Betreiber schaltet neun Fehlerszenarien und der Client verhaelt sich definiert | ✓ VERIFIED | `internal/talossim/scenario.go:48-57` neu gelesen: alle neun namentlich deklariert. `internal/talos` gruen (8.622s) mit `TestScenarioContract` (`contract_test.go:129`); von Runde 7 nicht beruehrt. |
| 3 | SC 3 — ein unerreichbarer Node blockiert weder die UI noch andere Nodes; erzwungenes Deadline; Retries nur fuer eine Lese-Allowlist; getrennte Cluster-/Maintenance-Typen | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Transport-Haelfte neu aufgezaehlt und gruen: `TestFanOutOneSilentNodeCostsOneNode`, `TestFanOutCancellationTerminatesEveryInFlightCall`, `TestFanOutSkipsAnOpenCircuitWithoutDialing`, `TestRequireDeadline`, `TestRetryAllowlistIsExactlyTheFastReadClass`, `TestMaintenanceClientMethodSetIsClosed`. Die **UI**-Haelfte hat weiterhin keinen Codepfad: `grep` ueber `internal/httpapi` und `cmd/` findet als einzigen Treffer einen Kommentar (`router.go:100`). Nach Phase 3 verschoben. |
| 4 | SC 4 — versions-skopierter Katalog, exakte ISO-/Installer-/PXE-URLs, **brauchbar erst nach bestaetigendem Model-Build-Probe**, Kernel-Args/META-Warnung | ✓ VERIFIED | Die dritte Bedingung `if stored.TalosVersion != fresh.TalosVersion` steht unveraendert (`schematics.go:579`); diese Runde hat die Datei nicht angefasst. `internal/httpapi/handlers` im vollen Lauf gruen (53.577s). |
| 5 | SC 5 — das ganze Binary laeuft mit `--dry-run` und keine Mutation erreicht einen Node | ✓ VERIFIED | Ausserhalb des Diffs. `TestDryRunRefusesEveryMutationAtTheNode` und fuenf Geschwister neu aufgezaehlt (`internal/talos/dryrun_test.go`), Paket gruen; Kompositionswurzel `cmd/holzkube-managerd/main.go:171` `talos.Mode{DryRun: cfg.DryRun}` neu gelesen. |
| 6 | R3/R4/R5/R6-Uebertrag — ein Waechter meldet einen Pass nur, wenn er die Eigenschaft gemessen hat, nach der er benannt ist | ✗ FAILED (partial) | **Verengt, nicht geschlossen — und alle drei SUMMARYs sagen das von sich aus.** Selbst nachgemessen: Blockkommentar, eingerruecktes `{/* */}`, Template-Literal, JSX-Text in `<pre>`, `String.raw`, `].filter(...)`, `...SPREAD` liefern alle `ranges=6 err=nil`. Der Waechter bleibt gruen ueber einer Tabelle, die der Browser nicht ausfuehrt. Weg (b) der Runde 6 ist zu 2/3 geliefert; der dritte Schritt ist eine Handlung des Betreibers. Siehe gaps[0]. |
| 7 | R3-Uebertrag — eine ohne Menschen aufgeloeste Entscheidung sagt das dort, wo sie gelesen wird | ✓ VERIFIED | Zwei Dokumente, beide gemessen. `02-DECISION-schematic-identity.md:146` unveraendert; Praefix-Hash `head -c 9137 \| shasum -a 256` = `ed2d0874…ee77c9`, byteweise wie in den Runden 5 und 6. **Neu:** `02-DECISION-drift-guard-lexik.md:3` liest `offen — vorgelegt, nicht ratifiziert`, `:471-500` sagt `Kein Betreiber hat sie ratifiziert.`, der Nachtrag zitiert `"entscheide du"` woertlich und nennt die Delegation ausdruecklich keine Ratifikation. Praefix-Hash `head -c 34626` = `16d106e8…a1b75b5b1`, `git diff --numstat` = `87 0`. |
| 8 | Standing — G-02-9 bleibt ehrlich als offen verbucht, auch in einer Runde, die drei Eintraege anlegt | ✓ VERIFIED | 58 und 66 unabhaengig aus dem JSON-Block gelesen: beide `open`. Eintrag 72 schliesst mit `G-02-9 IST OFFEN.`; alle drei SUMMARYs und der Nachtrag nennen ihn ausschliesslich als offen. Eintrag 56 daneben `open` und von 72 ausdruecklich nicht ueberholt. |
| 9 | Standing (T-02-62 / G-02-1-Linie) — kein Register behauptet mehr, als sein Traeger traegt | ✓ VERIFIED | **Runde 6s gaps[1] ist vollstaendig geschlossen.** REQUIREMENTS.md: vierzehn Phase-2-Zeilen `Gaps Found`, vierzehn leere Checkboxen, Diff EXAKT eine Zelle. ROADMAP.md:81 `31/31` und `31/31`, Phasentabelle `31/31`. STATE.md `Plan: 31 of 31` gegen `completed_plans: 37`/`total_plans: 37`. Wurzel zu: `requirements-completed: []` in allen drei SUMMARYs. |
| 10 | 02-29 — Leere und Abschneiden werden durch die FORM DES RUMPFES unterschieden, nicht durch eine Dateizaehlung | ✓ VERIFIED | Eigene Sonde: `= []` neben einer zweiten eintragsfoermigen Tabelle -> `REFUSED_RANGES is declared but carries no entries this guard can read` (Runde 6: `Look for that bracket`). Fail-first: `strings.TrimSpace(body) == ""` durch `if false` -> Zeile `a genuinely empty declaration beside a second table` ROT. |
| 11 | 02-29 — die Abschneide-Meldung ZEIGT ihren Beleg mit `%q`; der erschlossene Satz `The declaration is NOT empty` ist mit seiner Zaehlung verschwunden | ✓ VERIFIED | `:492-497` zitiert `%q`. Zwei Tabellenzeilen pinnen den zitierten Rumpf woertlich (`"\n  // see the table in RefusedRange["` und `"\n  // nothing yet\n"`). `grep -c '%d entries'` = **1** (Runde 6: 2) — 02-REVIEW IN-01 entfaellt als Nebenwirkung, selbst nachgezaehlt. |
| 12 | 02-29 — `whole` beantwortet nur noch EINE Frage; die Zaehlpruefung bleibt woertlich erhalten | ✓ VERIFIED | `whole` wird bei `:505` berechnet, unmittelbar vor `if whole != len(matches)` (`:513`), und hat genau diese eine Verwendung. Die Zeile `an entry-shaped literal outside the declaration` pinnt `1 entries inside … 2 in the source as a whole` unveraendert. |
| 13 | 02-29 — eine Bedingung, eine Ursache, eine Meldung: Ueberlauf und Inversion sind zwei Zweige | ✓ VERIFIED | `:552` und `:563`. Selbst gemessen: `{0x001f,0x0000}` -> `is inverted` (ohne `utf8.MaxRune`), `{0x0061,0xFFFFFFFF}` -> `has an upper bound this guard cannot represent`. Fail-first: wieder zusammengezogen -> `an inverted range` ROT. 02-REVIEW WR-03 zu. |
| 14 | 02-29 — keine Tabellenzeile prueft mehr eine Zeichenkette, die jeder Fehlerausgang traegt; `wantErr` ist `[]string` | ✓ VERIFIED | Alle drei Tabellen tragen `wantErr []string`. Die Funktion hat heute ZEHN Fehlerausgaenge (selbst aufgezaehlt); `no REFUSED_RANGES declared as an array literal` traegt genau einer. 02-REVIEW WR-04 zu. |
| 15 | 02-29 — der Waechter stellt fest, wie viele Deklarationen er gefunden hat, statt schweigend die erste zu nehmen | ✓ VERIFIED | `if len(decls) > 1` (`:429`), VOR dem Lesen des Rumpfes. Selbst gemessen: eingerueckte innere Deklaration vor der echten -> `2 declarations of REFUSED_RANGES in this source`. Fail-first: durch `if false` -> BEIDE Duplikat-Zeilen ROT. Beim Planen ueber Punkt 6 gefunden und behoben statt gelistet — die richtige Reihenfolge. |
| 16 | 02-29 — `go doc` zeigt die Begruendung der Anker-Entscheidung unter dem Symbol, das sie betrifft | ✓ VERIFIED | Der 49-Zeilen-Block ist geteilt: `refusedRangesEntry` (`:58-66`) und `refusedRangesDecl` (`:69-112`) tragen je einen eigenen Kommentar, jeder unmittelbar vor seinem `var`. 02-REVIEW WR-05 zu — nach zwei Runden, in denen er gewachsen statt geschrumpft ist. |
| 17 | 02-29/02-30 — die Regressionssperren der Runden 4, 5 und 6 bleiben woertlich erhalten und gruen; der Set-Test misst weiterhin dasselbe | ✓ VERIFIED | Vier eigene Messungen. Praefixierte Umbenennung und `//`-Form: beide `err != nil`. Anker auf die Runde-5-Form zurueckgedreht -> Praefix-Zeile ROT. `{ from: 0xfeff, to: 0xfeff }` aus `images.tsx` geloescht -> `TestBrowserRefusalSetEqualsTheServers` ROT (`U+FEFF` benannt) UND `the real route` ROT (`read 5 ranges, want 6`). Datei wiederhergestellt. **Heute ist nichts maskiert.** |
| 18 | 02-30 — der Waechter behauptet nur noch, was er misst: der allquantifizierte Satz ist aus Fehlertext UND Doc-Kommentar verschwunden | ✓ VERIFIED | `grep -c "Renamed, moved or deleted"` = **0**. An seiner Stelle eine Aussage ueber den TEXT plus eine geschriebene Ausschlussliste, gerendert von `honestClaim()` und angehaengt an genau EINEN Fehlerausgang (`:406`, den Kein-Anker-Ausgang; einzige Aufrufstelle, selbst nachgezaehlt). |
| 19 | 02-30 — die Blindheit ist nach MECHANISMUS skopiert, als DATEN gefuehrt, und Liste und Zeilen sind in beide Richtungen aneinander gebunden | ✓ VERIFIED | Vier Mechanismen (`text-not-code`, `literal-not-value`, `declaration-not-use`, `surrogate-interior`), je mit `mechanism` und `measured`. Fail-first in BEIDEN Richtungen selbst hergestellt: Zeile entfernt -> `lists "declaration-not-use" and no blindness row measures it`; Zeile auf eine ungelistete Id gedreht -> zusaetzlich `rows … name the mechanism "not-listed-anywhere", and guardBlindSpots does not carry it`. Der Test erzwingt zudem genau EINE `rowless`-Ausnahme. |
| 20 | 02-30 — die Blindheitstabellen laufen ueber DENSELBEN Pfad wie der Live-Waechter, ohne Wrapper | ✓ VERIFIED (mit benannter Abweichung) | `browserRefusalRanges(t, path)` nimmt den Pfad als Parameter; `TestBrowserRefusalSetEqualsTheServers` ruft dieselbe Funktion mit `imagesRoutePath`. **Anti-Umgehung selbst hergestellt**: ein Kommentar-Strip zwischen `readSource` und `parseBrowserRefusalRanges` faerbt `a_declaration_surviving_only_in_a_block_comment` ROT. Die Abweichung (zwei statt einer Funktion, die die Route liest) ist real, selbst nachgemessen (`:249` und `:598`) und in `02-30-SUMMARY.md` ausdruecklich benannt statt gerundet — siehe Abschnitt 7. |
| 21 | 02-30 — umgekehrte Abnahme: die Zeilen verlangen, dass der Waechter die Phantom-Tabelle liest, und die Anweisung fuer den Rot-Tag steht ueber beiden Tabellen im Code | ✓ VERIFIED | Beide Testfunktionen tragen `A RED HERE MEANS THE BLINDNESS HAS ENDED` mit der Handlungsanweisung (Zeile entfernen, `guardBlindSpots`-Eintrag entfernen, im Ledger vermerken) und dem ausdruecklichen Verbot, den Waechter aufzuweichen. Steht im Code und nicht in einem Planungsdokument. |
| 22 | 02-30 — jede genannte Form ist EINZELN gemessen; der fuenfte Ausschluss steht NEBEN der Behauptung und ist als unmessbar markiert | ✓ VERIFIED | Sieben Zeilen (4 `text-not-code`, 2 `literal-not-value`, 1 `declaration-not-use`) — alle von mir unabhaengig nachgemessen: `ranges=6 err=nil` fuer sechs, `ranges=2 err=nil` fuer die Rest-Deklaration. `surrogate-interior` traegt `rowless` mit dem richtigen Grund (Go kann keinen unpaarigen Surrogat in einem String halten) und die genauere Zahl **2046**. |
| 23 | 02-30 — Punkt 6 der Schliessbedingung wurde EINMAL AUSGEFUEHRT und protokolliert, auch bei negativem Ergebnis | ✓ VERIFIED (mit eigenem Fund) | Das Protokoll steht im Doc-Kommentar von `guardBlindSpots` (`:150-165`) und nicht nur in der SUMMARY — eine der drei selbst gemeldeten Abweichungen, und die richtige. Ich habe es nachgefahren: `String.raw`, Regex-Literal und `${…}` GELESEN (Auspraegungen von `text-not-code`), Objekt-Property und Re-Export korrekt ABGELEHNT. **Mein eigener Versuch hat eine achte Form gefunden** (stale Datei neben lebendem Modul) — Ergaenzung des Mechanismus `declaration-not-use`, keine Widerlegung. Siehe gaps[0]. |
| 24 | 02-31 — Eintrag 72 amendiert 70, bleibt `open`, zeigt auf ein SYMBOL statt auf eine Zeile, und traegt die sechspunktige Schliessbedingung | ✓ VERIFIED | Aus dem JSON-Block gelesen: `id 72`, `status open`, `line` LEER, Text oeffnet mit `AMENDS ENTRY 70, WHICH STAYS OPEN (no amend verb exists)` und zeigt auf `refusedRangesDecl`. Alle sechs Punkte woertlich vorhanden, mit Punkt 6 als ausdruecklicher Huerde (`EIN EINTRAG, DESSEN BEDINGUNG NUR DIE SCHON BEKANNTEN FORMEN ABFRAGT, WAERE EIN EINTRAG, DER SICH SELBST SCHLIESST.`). IN-05 damit erledigt, ohne 70 zu bearbeiten. |
| 25 | 02-31 — Eintrag 72 sagt ausdruecklich, dass die Richtung NICHT ratifiziert ist, und `overrides:`/`waive` werden nirgends benutzt | ✓ VERIFIED | Der Eintrag traegt `DIE RATIFIKATION FEHLT: die Wahl … wurde vom Betreiber am 2026-09-05 ausdruecklich an den autonomen Lauf delegiert; der Lauf hat B gewaehlt. Der Betreiber hat B nicht selbst ratifiziert.`, nennt A als punktgleich, verweist auf die Human-Verification-Zuweisung der Runde 6 und haelt die duennere Beweisdichte fest. `waived_count: 0` in allen drei Repraesentationen; diese Datei traegt `overrides_applied: 0`. |
| 26 | 02-31 — Eintrag 72 benennt die fuenf verbleibenden Reste statt sie in Fussnoten zu verteilen | ✓ VERIFIED | Alle fuenf woertlich vorhanden und einzeln nachgeprueft: die Ableitung Literal->Wert (von mir gemessen, `].filter` -> `ranges=6 err=nil`), die eine tragende Verbindung (`grep` ueber `web/src`: `REFUSED_RANGES` hat genau eine lebende Referenz, `images.tsx:159`), der `TrimSpace`-Rest (von mir gemessen: `[A4 body only a comment]` -> Abschneide-Zweig), die drei bewusst nicht geschriebenen Hinweiszeilen samt Grund, und die 2046 nie verglichenen Codepoints. |
| 27 | 02-31 — Eintrag 73 traegt den gemessenen Werkzeugdefekt an `requirements.revert-phase` | ✓ VERIFIED | **An der Quelle selbst gelesen statt uebernommen:** `milestone.cjs:425` traegt den `**id**`-Wrapper im Checkbox-Muster, `:434` verlangt fuer die Traceability-Haelfte exakte Gleichheit der ersten Zelle nach Trim/Kleinschreibung. `**TRANS-06** 🚫` ≠ `trans-06`. Der Eintrag nennt die Reichweite (alle 16 Release-Blocker-Zeilen), bleibt `open`, weil der Defekt ausserhalb dieses Repositories liegt, und benennt seine eigene Messgrenze (`mark-complete` und `ready-ids` NICHT geprueft). |
| 28 | 02-31 — alle vierzehn Phase-2-Zeilen lesen `Gaps Found`, alle Checkboxen leer, und die Handkorrektur beruehrt nur die eine Statuszelle | ✓ VERIFIED | Vierzehn Traceability-Zeilen und vierzehn Checkboxen einzeln geprueft. `git diff a42387d..HEAD -- .planning/REQUIREMENTS.md` ist EXAKT eine Zeile: `| **TRANS-06** 🚫 | Phase 2 | Complete` -> `Gaps Found`. Die Dekoration der Id-Zelle blieb stehen — die Reparatur wurde nicht am falschen Ort gemacht. |
| 29 | 02-31 — Eintrag 74 misst die drei Geschwister-Leser EINZELN und widerspricht dabei dem Entscheidungsdokument | ✓ VERIFIED | **Beide Muster woertlich kopiert und selbst gefahren.** `stringArrayLiteral`: Praefix-Loch JA, Zeilenkommentar JA, Template-Literal JA. Budget-Anker: Praefix-Loch **NEIN**, `//` NEIN, `{/* */}` NEIN, Blockkommentar JA, Template-Literal JA. Das Entscheidungsdokument (`:442`) sagt `ebenso` — **das ist gemessen falsch**, und Eintrag 74 sagt das ausdruecklich, mit dem richtigen Grund (der Unterstrich bricht `\s*=`). Ein Ledger, der seiner eigenen Quelle widerspricht, weil er nachgemessen hat, ist genau das, was ein Ledger sein soll. |
| 30 | 02-31 — der Ledger ist in allen drei Repraesentationen selbstkonsistent, und der Nachtrag ist angehaengt statt eingearbeitet | ✓ VERIFIED | Unabhaengig dreifach geparst: Frontmatter 58/0/16/74, Tabelle 74 Zeilen (58 open, 16 fixed), JSON 74 Eintraege — null Abweichungen PRO ID, ids 1..74 lueckenlos und doppelfrei. `git diff` an `WINDOWS.md`: die einzigen geloeschten Zeilen sind drei Frontmatter-Zaehler; kein Eintrag 1..71 bearbeitet. Nachtrag: `numstat 87 0`, Praefix-Hash bestaetigt, Statusblock unveraendert, Zahlenkorrektur 2048->2046 durch Anhaengen. |

**Score:** 28/30 truths verified (1 present, behavior-unverified; 1 failed — und das eine Failed
haengt an einer Handlung des Betreibers, nicht an fehlender Arbeit).

## 2. Deferred items

| # | Item | Addressed In | Evidence |
|---|---|---|---|
| 1 | Die UI-Haelfte von SC 3 / TRANS-05 | Phase 3 | Kein Produktivaufrufer der Naht (neu gemessen: einziger Treffer ein Kommentar in `router.go:100`); ROADMAP Phase 3 besitzt die Inventar-Route. |
| 2 | TRANS-08 — die Contract-Suite gegen echtes Talos | Phase 3 | REQUIREMENTS.md (Pending); ROADMAP Phase 3 SC 5 und die `Note`. `contract_test.go:129` parametrisiert den Transport bereits. |
| 3 | G-02-9 — eine ausgebliebene Probe ist weiterhin dauerhaft | 02-DECISION-probe-budget.md Option 1, nicht genommen | 58 und 66 aus dem JSON-Block gelesen, beide `open`; Eintrag 72 bekraeftigt es in Grossbuchstaben. **Eine Runde mit drei neuen Eintraegen hat ihn nicht verwaessert.** |
| 4 | Eintrag 56 — die SERVER-Menge nicht erschoepfend gegen `factory.talos.dev` gemessen | offen, von 72 ausdruecklich nicht ueberholt | `open` aus dem JSON-Block; 72 sagt die Nicht-Ueberholung woertlich. |

## 3. Required artifacts

| Artifact | Expected | Status | Details |
|---|---|---|---|
| `internal/imagefactory/guard_drift_test.go` | Spiegel-Defekt ueber die Rumpfform beseitigt, zwei Bound-Zweige, unterscheidende `wantErr`, Duplikatspruefung, geteilter Doc-Kommentar, `guardBlindSpots` als Daten, wrapper-freier Lesepfad | ⚠️ PARTIAL | Alle sieben bestellten Eigenschaften sind da und sechs davon von mir fail-first belegt. Zehn Fehlerausgaenge, keiner erschliesst seine Ursache aus einer Zaehlung. Die Tabellen sind auf 10/2/3 Zeilen gewachsen, keine bestehende entfernt. Der lexikalische Defekt selbst besteht fort (gaps[0]) — verengt ist die Behauptung. |
| `.planning/WINDOWS.md` | Eintraege 72, 73 und 74, alle `open`, 70 unberuehrt | ✓ VERIFIED | 58/0/16/74 dreifach, null Abweichungen pro Id. Nur die drei Frontmatter-Zaehler wurden ueberschrieben; kein bestehender Eintrag bearbeitet. Kein `fixed`, kein `waive`. |
| `.planning/REQUIREMENTS.md` | vierzehn Phase-2-Zeilen `Gaps Found` | ✓ VERIFIED | Vierzehn Zeilen und vierzehn Checkboxen einzeln geprueft; Diff exakt eine Zelle. Runde 6s Regression ist zu. |
| `.planning/ROADMAP.md`, `.planning/STATE.md` | Fortschrittsangaben ohne Selbstwiderspruch | ✓ VERIFIED | ROADMAP:81 `31/31` und `31/31`, Phasentabelle `31/31`. STATE `Plan: 31 of 31` gegen `completed_plans: 37`. Die Wiederholungs-Frage steht als Entscheidungspunkt unter Abschnitt 9. |
| `02-DECISION-drift-guard-lexik.md` | angehaengter, datierter Nachtrag; Statusblock unveraendert | ✓ VERIFIED | `numstat 87 0` (rein additiv), Praefix-Hash `16d106e8…a1b75b5b1` selbst nachgemessen, Statusblock `offen — vorgelegt, nicht ratifiziert`, Delegationsformel woertlich, Zahlenkorrektur durch Anhaengen. |
| `02-29/30/31-SUMMARY.md` | melden G-02-29 NICHT als geschlossen; `requirements-completed` leer | ✓ VERIFIED | Alle drei sagen es ausdruecklich (`:348`, `:391`, `:487`); alle drei tragen `requirements-completed: []`. Die selbst gemeldeten Abweichungen sind benannt statt gerundet. |
| `web/src/routes/images.tsx` | von dieser Runde WEDER netto NOCH voruebergehend veraendert | ✓ VERIFIED | `git diff --stat a42387d..HEAD -- web/` leer. Meine eigene U+FEFF-Falsifikation ist zurueckgenommen, `git status` leer. |

## 4. Key link verification

| From | To | Via | Status | Details |
|---|---|---|---|---|
| `guard_drift_test.go` | `web/src/routes/images.tsx` | der Go-Test liest die **Deklaration** `REFUSED_RANGES`, weil ein Browser kein Go-Symbol importieren kann | ⚠️ PARTIAL | Die Naht traegt weiterhin, und der Waechter sagt jetzt selbst, was sie nicht traegt. Was er nicht kann, ist lexikalischer Kontext — und was ich neu gemessen habe: er bindet auch an einen hartkodierten PFAD, nicht an das Modul, das der Router aufloest. |
| `runBlindnessRows` | `browserRefusalRanges` | die Blindheitszeilen fahren den LIVE-Lesepfad, damit eine kuenftige Vorverarbeitung sie ROT faerbt statt an ihnen vorbeizulaufen | ✓ WIRED | **Selbst hergestellt**: Kommentar-Strip zwischen `readSource` und `parseBrowserRefusalRanges` -> `a_declaration_surviving_only_in_a_block_comment` ROT. Die Zusage ist nicht umgehbar. |
| `guardBlindSpots` | `honestClaim` -> Kein-Anker-Ausgang | die Behauptung wird aus der Liste GERENDERT statt daneben geschrieben | ✓ WIRED | `honestClaim()` hat genau eine Aufrufstelle (`:406`). `TestGuardBlindSpotsAreEachMeasured` bindet Liste und Zeilen bidirektional; beide Richtungen von mir rot gesehen. |
| `WINDOWS.md` Eintrag 72 | `guard_drift_test.go` | die sechspunktige Schliessbedingung ist am Code ausfuehrbar | ✓ WIRED | Punkte 1-5 von mir gemessen und alle erfuellt. Punkt 6 von mir gefahren — **mit einem Fund**, weshalb der Eintrag korrekt `open` bleibt. |
| `WINDOWS.md` Eintrag 73 | `.planning/REQUIREMENTS.md` | der Eintrag begleitet die Handkorrektur der einen Zeile, die kein Werkzeug erreicht hat | ✓ WIRED | Diagnose an der Quelle (`milestone.cjs:425` gegen `:434`) selbst gelesen und bestaetigt; die Korrektur im Register ist genau eine Zelle. |
| `WINDOWS.md` Eintrag 74 | `budget_drift_test.go`, `guard_drift_test.go`, `warnings_test.go` | drei Geschwister-Leser, jeder EINZELN gemessen | ✓ WIRED | Zwei der drei Muster von mir woertlich nachgefahren; die Korrektur am Entscheidungsdokument haelt in beiden Richtungen (Praefix NEIN, Blockkommentar/Template JA). |
| `02-DECISION-drift-guard-lexik.md` | `WINDOWS.md` Eintrag 72 | die Ratifikation gehoert in Statusblock UND Eintragstext; sie ist nicht erfolgt, also traegt der Eintrag ihr Ausbleiben | ✓ WIRED | Beide Orte tragen dieselbe Formulierung, und beide sagen `nicht ratifiziert`. **Die Verdrahtung funktioniert und meldet korrekt eine Luecke.** |

## 5. Behavioural spot-checks

| Behaviour | Command | Result | Status |
|---|---|---|---|
| Spiegel-Defekt (G-02-28) | eigene Sonde: `= []` neben zweiter eintragsfoermiger Tabelle | `REFUSED_RANGES is declared but carries no entries this guard can read` | ✓ PASS — die falsche Ursache ist beseitigt, nicht getauscht |
| Fail-first dazu | `strings.TrimSpace(body) == ""` -> `if false` | Zeile `a genuinely empty declaration beside a second table` ROT | ✓ PASS |
| Der `TrimSpace`-Rest, ehrlich mitgemessen | Rumpf nur aus einem Kommentar | `cut short before its first entry` | ⚠️ bekannt, im Code benannt, von einer Zeile gepinnt, Eintrag 72 Rest (3) |
| Zwei Bound-Ursachen | `{0x001f,0x0000}` / `{0x0061,0xFFFFFFFF}` | `is inverted` (ohne `utf8.MaxRune`) / `has an upper bound this guard cannot represent` | ✓ PASS — WR-03 zu |
| Fail-first dazu | beide Zweige wieder zusammengezogen | `an inverted range` ROT | ✓ PASS |
| Duplikatspruefung | eingerueckte innere Deklaration vor der echten | `2 declarations of REFUSED_RANGES in this source` | ✓ PASS |
| Fail-first dazu | `len(decls) > 1` -> `if false` | BEIDE Duplikat-Zeilen ROT | ✓ PASS |
| Anker-Regressionssperre | Praefix-Umbenennung; `//`-Form | beide `err != nil` | ✓ PASS |
| Fail-first dazu | Anker auf die Runde-5-Form zurueckgedreht | Praefix-Zeile ROT (`no error; read 2 ranges instead`) | ✓ PASS |
| Der Waechter misst weiterhin etwas | `{ from: 0xfeff, to: 0xfeff }` geloescht | `TestBrowserRefusalSetEqualsTheServers` ROT (`U+FEFF`) **und** `the real route` ROT (`read 5 ranges, want 6`) | ✓ PASS — nichts ist maskiert |
| Lexikalische Blindheit (7 Formen) | Blockkommentar, `{/* */}`, Template-Literal, JSX-Text, `String.raw`, `].filter`, `...SPREAD` | alle `ranges=6 err=nil` | ✗ FAIL (erwartet) — der Defekt besteht fort; die Liste sagt es jetzt |
| Gegenprobe | Objekt-Property; Re-Export | korrekt `no REFUSED_RANGES declared as an array literal` | ✓ PASS |
| Anti-Umgehung der Blindheitszeilen | Kommentar-Strip in `browserRefusalRanges` eingesetzt | `a_declaration_surviving_only_in_a_block_comment` ROT | ✓ PASS — die Zusage haelt |
| Abdeckungstest, beide Richtungen | Zeile entfernt; Zeile auf ungelistete Id gedreht | `lists "declaration-not-use" and no row measures it` / zusaetzlich `name the mechanism "not-listed-anywhere"` | ✓ PASS |
| **Punkt 6, eigener Versuch** | stales `routes/images.tsx` neben lebendem `routes/images/index.tsx` | `stale ranges=6 err=nil` gegen `live ranges=1 err=nil` | ⚠️ FUND — Ergaenzung von `declaration-not-use` auf Datei-Granularitaet; siehe gaps[0] |
| Geschwister-Leser 1 | `stringArrayLiteral`-Muster woertlich kopiert | Praefix JA, `//` JA, Template JA | ✓ PASS (Eintrag 74 stimmt) |
| Geschwister-Leser 2 | Budget-Anker woertlich kopiert | Praefix **NEIN**, `//` NEIN, `{/* */}` NEIN, Block JA, Template JA | ✓ PASS — **Eintrag 74 hat recht und das Entscheidungsdokument unrecht** |
| Werkzeugdefekt an der Quelle | `milestone.cjs:425` gegen `:434` gelesen | Checkbox toleriert `**id**`, Traceability verlangt exakte Gleichheit | ✓ PASS — Eintrag 73s Diagnose ist richtig |
| Ledger-Selbstkonsistenz | unabhaengiger Parse von Frontmatter, Tabelle und JSON, Status PRO ID | 58/0/16/74 dreifach, 0 Abweichungen, ids 1..74 | ✓ PASS |
| Ledger append-only | `git diff` auf geloeschte Zeilen | nur drei Frontmatter-Zaehler | ✓ PASS — Eintrag 70 woertlich und `open` |
| Praefix-Hashes | `head -c 34626` / `head -c 9137` + `shasum -a 256` | `16d106e8…a1b75b5b1` / `ed2d0874…ee77c9` | ✓ PASS |
| Gates, selbst gefahren | `go build ./...`, `go vet ./...`, `gofmt -l internal/ cmd/`, `go test ./... -count=1` | Exit 0 / Exit 0 / leer / Exit 0 ueber 16 Testpakete | ✓ PASS |

*Kein `vitest`-Lauf: `git diff --stat a42387d..HEAD -- web/` ist leer. Nichts hier Gemessenes
widerspricht dem Orchestrator.*

## 6. Requirements coverage

| Requirement | Source plans | Status | Evidence |
|---|---|---|---|
| FOUND-12 | 02-07 | ✓ SATISFIED | `internal/talos/dryrun.go` + sechs Tests neu aufgezaehlt; `main.go:171`. Ausserhalb des Diffs. |
| TRANS-01 | 02-01 | ✓ SATISFIED | Echter Machinery-Client ueber die `Dialer`-Naht mit echtem mTLS. |
| TRANS-02 | 02-01 | ✓ SATISFIED | `Dialer` und `DiscoverySource` mit je zweiter Implementierung. |
| TRANS-03 | 02-05 | ✓ SATISFIED | Getrennte Typen; `TestMaintenanceClientMethodSetIsClosed` neu aufgezaehlt. |
| TRANS-04 | 02-05 | ✓ SATISFIED | `TestRequireDeadline`, `TestRetryAllowlistIsExactlyTheFastReadClass` neu aufgezaehlt. |
| TRANS-05 | 02-05 | ⚠️ PARTIAL | Transport-Haelfte belegt; UI-Haelfte ohne Aufrufer — Phase 3. |
| TRANS-06 🚫 | 02-01, 02-08 | ✓ SATISFIED | `internal/talossim` mit In-Memory-COSI, drei Streams, Method-Drift-Guard. **Die Statuszeile liest jetzt korrekt `Gaps Found`** — die Handkorrektur der Runde 7 ist die Anwendung der Regel aus `2e57ee3`, nicht eine Aussage ueber die Codebasis. |
| TRANS-07 | 02-03 | ✓ SATISFIED | Neun Szenarien in `scenario.go:48-57` neu gelesen; `TestScenarioContract` im gruenen Paket. |
| TRANS-08 | — | ⏭ DEFERRED | Phase 3. |
| FACT-01 | 02-02, 02-06, 02-26 | ✓ SATISFIED | Versions-skopierter Katalog, kein Freitextfeld. Der Drift-Waechter ueber die Ablehnungsmenge misst HEUTE korrekt (U+FEFF neu gemessen); seine kuenftige Zuverlaessigkeit ist gaps[0] und keine Aussage ueber die heutige Menge. |
| FACT-02 | 02-02, 02-22, 02-24, 02-25 | ✓ SATISFIED | Vorvalidierung der Extension-Namen; die Brauchbarkeit haengt am bestaetigenden Probe, auch versionsuebergreifend. |
| FACT-03 | 02-04, 02-09, 02-23 | ✓ SATISFIED | Exakte ISO-/Installer-/PXE-URLs, versionsaufgeloester Repo-Name, Architektur als Parameter. |
| FACT-04 | 02-04, 02-06 | ✓ SATISFIED | installer/initramfs-Warnung emittiert und in der UI gespiegelt. |
| FACT-05 | 02-04 | ✓ SATISFIED | Pre-Release strukturell gefiltert; kaputte Versionen kuratiert. |
| FACT-06 | 02-02, 02-14, 02-24, 02-26, 02-27 | ✓ SATISFIED | Id lokal vorberechnet und persistiert; `Canonical()` gegen die echte Factory von einem externen Orakel gemessen. |

**Orphaned requirements:** keine. Alle vierzehn auf Phase 2 abgebildeten IDs erscheinen in mindestens
einem `requirements`-Frontmatter; REQUIREMENTS.md bildet keine weitere ID auf Phase 2 ab.
`.planning/REQUIREMENTS.md` fuehrt alle vierzehn korrekt als `Gaps Found`, solange diese Phase
`gaps_found` traegt.

## 7. Anti-patterns

| File | Line | Pattern | Severity | Impact |
|---|---|---|---|---|
| — | — | `TBD` / `FIXME` / `XXX` / `TODO` / `HACK` / `PLACEHOLDER` | — | **Keine** in der einzigen von dieser Runde geaenderten Quelldatei (einzeln geprueft, `grep -c TODO` = 0). |
| `internal/imagefactory/guard_drift_test.go` | 113-115 | ein Anker ohne lexikalischen Kontext | 🛑 Blocker | gaps[0]. Selbst gemessen, sieben Formen. **Die BEHAUPTUNG ist verengt, der DEFEKT nicht.** |
| `internal/imagefactory/guard_drift_test.go` | 31 / `budget_drift_test.go:41` | zwei Waechter binden an einen hartkodierten PFAD; nichts behauptet, dass diese Datei das aufgeloeste Modul ist | ⚠️ Warning | **Neu in dieser Runde gefunden, von mir gemessen** (`stale ranges=6` gegen `live ranges=1`). Ergaenzung von `declaration-not-use`; gehoert in `measured:` plus eine Zeile. |
| `internal/imagefactory/guard_drift_test.go` | 479 | `TrimSpace` haelt einen Kommentar-Rumpf fuer nicht leer | ℹ️ Info | Deklariert: der Code sagt es an Ort und Stelle, eine Zeile pinnt es, Eintrag 72 fuehrt es als Rest (3). **Ein benannter Rest ist kein Anti-Pattern** — er ist der Unterschied zu Runde 6. |
| `internal/imagefactory/guard_drift_test.go` | 249 / 598 | zwei Funktionen lesen die Route; das Abnahmekriterium von 02-30 Task 1 verlangt buchstaeblich eine | ⚠️ Warning | **Selbst nachgemessen und bestaetigt.** Die geschuetzte Eigenschaft (kein Wrapper, der einen Pfad hartkodiert) HAELT und ist von mir rot-belegt; das Kriterium kollidiert mit der Prohibition desselben Plans, keine Tabellenzeile zu entfernen. `02-30-SUMMARY.md` benennt die Kollision unter `## Issues Encountered`, statt sie zu runden — das ist die richtige Behandlung. |
| `internal/imagefactory/guard_drift_test.go` | 119-124 | der Surrogate-Bereich wird nur an seinen beiden Endpunkten geprueft | ⚠️ Warning | Vorbestehend, jetzt aber **als `rowless`-Eintrag mit Grund und der genaueren Zahl 2046 geschrieben** statt nur in einem Planungsdokument. |
| `internal/httpapi/handlers/schematics.go` | 628-632 | `versionMismatchReason` ohne Leerwert-Zweig | ⚠️ Warning | 02-REVIEW Runde 5 WR-06, unveraendert weitergetragen; diese Runde hat die Datei nicht angefasst. |
| `web/src/api.ts` / `web/src/api.test.ts` | 475 / 99 | `REQUEST_CEILING_MS = 150_000` gegen `toBeGreaterThan(130_000)` gegen `writeTimeout = 130s` | ⚠️ Warning | Unveraendert weitergetragen; siehe coincidental_reliance_items. |
| `.planning/ROADMAP.md` | 81 | eine vom Werkzeug und eine von Hand gefuehrte Zahl in einem Satz | ⚠️ Warning | **Der Zustand ist heute richtig** (`31/31` und `31/31`). Der Befund ist die dreimalige Wiederholung; `02-31-SUMMARY.md` hat ihn aufgezeichnet und den Fall ausdruecklich uebergeben. Entscheidungspunkt in Abschnitt 9. |

**Test quality audit.** Kein uebersprungener oder deaktivierter Test ist der alleinige Beleg fuer
eine Anforderung: `grep -n 't.Skip'` ueber die geaenderte Datei findet **nichts**; die einzigen Skips
der Phase bleiben die Opt-in-Live-Guards (`live_test.go`, `canonical_live_test.go`), beide im Ledger
(5/64 und 35). Drei Beobachtungen tragen diesen Abschnitt. **(a) Die Fail-first-Reihenfolge ist echt
und nicht behauptet — ich habe sie fuer sechs Eigenschaften selbst hergestellt**, indem ich den
jeweiligen Code ausgebaut habe, und jedes Mal genau die zugehoerige Zeile rot gesehen. **(b) Die
Assertion-Staerke ist gestiegen**: `wantErr` ist in allen drei Tabellen `[]string`, keine Zeile pinnt
mehr eine Zeichenkette, die jeder Fehlerausgang traegt, und zwei Zeilen pinnen sogar den mit `%q`
zitierten Rumpf woertlich. **(c) Die umgekehrte Abnahme der Blindheitstabellen ist die interessanteste
Konstruktion dieser Phase**: sie testet nicht, dass etwas funktioniert, sondern dass eine benannte
Blindheit noch besteht — und sie traegt die Anweisung im Code, ein Rot niemals durch Aufweichen des
Waechters zu beseitigen. Ich habe geprueft, dass diese Zusage nicht umgehbar ist. Provenienz der
Erwartungswerte fuer den FACT-06-Differential bleibt extern und nur extern.

## 8. Decision coverage

`02-CONTEXT.md` deklariert D-01 … D-10. Neun sind in mindestens einer SUMMARY benannt. **D-07**
(`talossim` lebt in `internal/talossim`, Modulgrenze erweitert) ist in keiner SUMMARY benannt, wird
aber im Code eingehalten und erzwungen: `internal/depguard_test.go:30` deklariert
`simulatorPackage = rootModule + "/internal/talossim"` und `:257` pinnt es in der Grenztabelle; das
Paket `internal` ist im vollen Lauf gruen (1.950s). Bei `5495949` erneut geprueft und unveraendert.
Nicht blockierend; nur zur Drift-Erkennung notiert.

## 9. Human verification required

### 1. Variante B ratifizieren — oder Variante A waehlen

**Test:** Bei B: die Ratifikation in den Statusblock von `02-DECISION-drift-guard-lexik.md` UND in
den Text von Ledger-Eintrag 72 eintragen (beide Orte nennt das Dokument selbst) und die Ausnahme
hier als `overrides:` festhalten. Bei A: die Arbeit von 02-30 an `guard_drift_test.go` neu planen;
der unbedingte Teil (02-29 vollstaendig, IN-05) bleibt unberuehrt.
**Expected:** Der Statusblock liest nicht mehr `offen — vorgelegt, nicht ratifiziert`, und diese
Datei traegt entweder einen `overrides:`-Eintrag oder eine Neuplanung nach A.
**Why human:** **Das ist der einzige Grund, warum diese Phase heute nicht schliessen kann.** Am
2026-09-05 hat der Betreiber mit *"entscheide du"* **delegiert**; der autonome Lauf hat B gewaehlt
und sich geweigert, das als Ratifikation zu verbuchen. Diese Weigerung ist richtig und die beste
Einzelentscheidung der Runde — und sie ist zugleich der Grund, warum die Wahrheit gemessen falsch
bleibt: Runde 6 hat Weg (b) mit drei Schritten ausgeschrieben, zwei sind geliefert und gemessen, und
der dritte ist eine Handlung, die nur der Betreiber vornehmen kann.

Wenn ratifiziert wird, gehoert dieser Block in die Frontmatter dieser Datei:

```yaml
overrides:
  - must_have: "A guard reports a pass only when it measured the property it is named for"
    reason: >-
      Variante B ratifiziert: die Behauptung des Waechters ist auf das verengt, was ein regulaerer
      Ausdruck ueber TypeScript-Quelltext messen kann; der lexikalische Rest wird als offener
      Ledger-Eintrag 72 mit pruefbarer, sechspunktiger Schliessbedingung gefuehrt statt behauptet.
      Vier Mechanismen sind als Daten gelistet, sieben Zeilen messen sie ueber den Live-Lesepfad
      mit umgekehrter Abnahme, und der Abdeckungstest bindet Liste und Zeilen in beide Richtungen.
    accepted_by: "<Betreiber>"
    accepted_at: "<ISO-Zeitstempel>"
```

### 2. Ob die nachgestellte Planzahl in `ROADMAP.md:81` einen Ledger-Eintrag bekommt

**Test:** Entweder `gsd-tools windows append` fuer die Wiederholung, oder die von Hand gefuehrte
Zahl ersatzlos streichen, so dass nur die vom Werkzeug gefuehrte bleibt.
**Expected:** Eine der beiden Handlungen ist vollzogen.
**Why human:** Der ZUSTAND ist heute richtig — gemessen `31/31` und `31/31`. Der Befund ist die
WIEDERHOLUNG: dieselbe Zahl ist dreimal hintereinander gedriftet, weil Werkzeugausgabe und
Handschrift in einem Satz stehen. `02-31-SUMMARY.md` hat sie aufgezeichnet und ausdruecklich
**keinen** vierten Eintrag angelegt, weil seine Erfolgskriterien die Endsignatur auf 74 nageln — und
den Fall dieser Runde uebergeben. Nach dem Massstab, den Eintrag 74 selbst setzt (*"Sie
stillschweigend stehen zu lassen waere dieselbe Operation, die Eintrag 69 falsch gemacht hat"*),
gehoert sie geledgert. Es ist eine Buchhaltungs-Policy und keine Messung.

### 3. Das zusammengebaute Binary durch einen Browser gegen die echte Image Factory fahren

**Test:** 02-UAT.md Test 5, Unterpruefungen (a)-(d).
**Expected:** Die Route verhaelt sich, wie die Suiten es vorhersagen, mit echter Latenz und echtem Bundle.
**Why human:** 02-UAT.md Test 5 traegt weiterhin `result: issue`; das Browser-Projekt sagt in seinem
eigenen Doc-Kommentar, dass kein Binary laeuft und kein Bundle ausgeliefert wird.

### 4. Bestaetigen, dass der Browser-Install-Schritt der CI auf ubuntu-latest laeuft

**Test:** ein CI-Lauf, der `.github/workflows/ci.yml:71` erreicht.
**Expected:** `playwright install --with-deps chromium` gelingt und das Browser-Projekt laeuft.
**Why human:** WINDOWS-Eintrag 44 ist bei diesem Commit weiterhin `open`. Es scheitert geschlossen;
das Risiko ist ein roter Lauf und kein stiller Pass.

### 5. Eine kalte Installer-Aufloesung gegen factory.talos.dev neu messen

**Test:** eine kalte Assets-Anfrage gegen die echte Factory nach dem Fan-out aus 02-23.
**Expected:** Die kalten Kosten sind der langsamste einzelne Kandidat, nicht die Summe.
**Why human:** 02-24-SUMMARY.md:232 und WINDOWS-Eintrag 57 sagen selbst, dass die Verbesserung
offline gemessen und live unmessen ist.

### 6. Entscheiden, wo die Nicht-Gap-Befunde der Code-Reviews leben

**Test:** Triagieren. **Die Liste ist zum ersten Mal seit Runde 4 kuerzer geworden** — WR-03, WR-04
und WR-05 sind in dieser Runde behoben und IN-01 ist als Nebenwirkung entfallen. Es bleiben IN-02..
IN-04 der Runde 6, WR-06, der Surrogate-Punkttest, IN-02..IN-07 der Runde 5 und die vier Warnungen
der Vorvorrunde (SecureBoot-`ParseBool`, Installer-Provenienz, ungewachte Ceiling-Transkription,
`allowedHosts`/`ssoOnly`) — dazu mein neuer Pfad-Befund, der in gaps[0] steht.
**Expected:** Jeder Punkt steht in `.planning/WINDOWS.md` oder ist ausdruecklich abgelehnt.
**Why human:** Eine Policy-Entscheidung. Geprueft: keiner dieser Punkte steht in irgendeiner der drei
Repraesentationen des Registers, und `workflow.windows_enforce` liest es beim Ship. Anmerkung:
`02-REVIEW.md` traegt bei diesem Commit noch `scope: gap-closure round 6`; ein Runde-7-Review lag
zum Zeitpunkt dieser Verifikation nicht vor und ist nicht in dieses Urteil eingegangen.

### 7. `⚠️ PRESENT_BEHAVIOR_UNVERIFIED` — die UI-Haelfte von SC 3

Siehe behavior_unverified_items. Erst nach Phase 3 ausuebbar; die Transport-Haelfte ist belegt.

## 10. Gaps summary

**Kann die Phase schliessen? Heute nicht — und der Grund ist zum ersten Mal in acht Runden nicht
fehlende Arbeit.**

Runde 7 hat drei Auftraege gehabt und alle drei erledigt, in **einer einzigen Quelldatei**, ohne
`web/` zu beruehren, ohne ein `fixed`, ohne ein `waive`, ohne ein `overrides:` und mit leerem
`requirements-completed` in allen drei SUMMARYs. Die Buchhaltungsregression der Runde 6 ist
**vollstaendig** geschlossen — in allen drei Registern und an der Wurzel. Der Spiegel-Defekt ist
**beseitigt** und nicht getauscht: die Unterscheidung laeuft ueber die Form des Rumpfes, `whole`
beantwortet genau eine Frage, und der aus einer Zaehlung erschlossene Satz ist mit der Zaehlung
verschwunden. Drei Review-Warnungen sind zu, eine Info-Meldung ist als Nebenwirkung entfallen, und
die Waechter-Funktion hat heute zehn Fehlerausgaenge, von denen keiner seine Ursache errät.

**Ich habe nichts davon geglaubt.** Sechs Fail-first-Falsifikationen selbst hergestellt und je
zurueckgenommen. Die Anti-Umgehungs-Zusage der Blindheitstabellen selbst erzeugt: ein
Vorverarbeitungsschritt im Aufrufer faerbt sie rot. Den Werkzeugdefekt an der Quelle in
`milestone.cjs` gelesen. Den Ledger dreifach geparst und die geloeschten Zeilen des Diffs geprueft.
Beide Praefix-Hashes nachgemessen. **Beide Selbstmeldungen der Ausfuehrenden nachgeprueft — und
beide halten**: die zwei Route-Leser sind real und die geschuetzte Eigenschaft haelt trotzdem; und
das Entscheidungsdokument traegt tatsaechlich eine gemessene Falschaussage, die Ledger-Eintrag 74
korrekt und mit dem richtigen Grund korrigiert.

**Was diese Runde von den sechs davor abhebt, ist die Behandlung ihrer eigenen Grundlage.** Der
Betreiber hat delegiert und nicht ratifiziert. Der Lauf hat das nirgends anders verbucht — nicht im
Statusblock, nicht im Nachtrag, nicht im Ledger —, und er hat zusaetzlich festgehalten, dass seine
eigene Beweisdichte duenner ist als die der Vorrunden. Eine delegierte Wahl als Ratifikation zu
buchen waere exakt die Defektklasse gewesen, die diese Phase seit sieben Runden korrigiert, begangen
von der Runde, die sie korrigiert. Sie ist nicht begangen worden.

**Und genau deshalb bleibt die Wahrheit offen.** Runde 6 hat Weg (b) mit drei Schritten
ausgeschrieben: verengen, ledgern, ratifizieren. Zwei sind geliefert und von mir gemessen. Der dritte
ist eine Handlung des Betreibers, und ohne sie gibt es keinen Mechanismus, der aus einer gemessen
falschen Wahrheit ein `PASSED (override)` macht. Der Waechter liest weiterhin eine Tabelle im
Blockkommentar mit `ranges=6 err=nil`. Was sich geaendert hat, ist die **Aritaet des Restes**: von
unendlich vielen moeglichen Falsifikationen zu vier gelisteten Mechanismen, deren jede neue
Auspraegung eine Ergaenzung ist. Mein eigener Punkt-6-Versuch hat genau das geliefert — eine achte
Form (ein stales Routenmodul), die den gelisteten Mechanismus ergaenzt statt ihn zu widerlegen. **Das
ist der versprochene Gewinn, zum ersten Mal am eigenen Leib gemessen.**

Die Phase braucht keine neunte Runde Code. Sie braucht einen Satz.

---

_Verifiziert: 2026-09-05T12:40:00Z bei `5495949`_
_Verifier: Claude (gsd-verifier), Runde 8_
