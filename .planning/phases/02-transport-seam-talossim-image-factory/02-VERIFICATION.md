---
phase: 02-transport-seam-talossim-image-factory
verified: 2026-09-04T20:05:54Z
verified_at_commit: 2211c80
status: gaps_found
score: 16/19 must-haves verified
behavior_unverified: 1
overrides_applied: 0
re_verification:
  round: 6
  previous_status: gaps_found
  previous_score: 14/19
  previous_verified_at_commit: d7d07ea
  gaps_closed:
    - "G-02-25 (Runde 5, gaps[0], `G5-1`, 02-REVIEW WR-04/WR-01) — GESCHLOSSEN, in dem Umfang, den Runde 5 selbst ausgeschrieben hat, und jede der drei Korrekturen einzeln fail-first gemessen statt aus 02-27-SUMMARY.md uebernommen. (a) Der Anker: Runde 5s Experiment am echten Baum woertlich wiederholt — `REFUSED_RANGES` durchgaengig in `REFUSED_RANGES_LEGACY` in `web/src/routes/images.tsx`, `go test -run TestBrowserRefusal` -> `--- FAIL: TestBrowserRefusalSetEqualsTheServers` UND `--- FAIL: .../the_real_route`, wo Runde 5 zweimal PASS gemessen hatte. Datei byteweise wiederhergestellt, `git diff` leer. Zusaetzlich den Anker auf die Runde-5-Form `\\s*[^=\\n]*=` zurueckgedreht: die neue Tabellenzeile `the declaration renamed to a prefixed name` wird ROT (`no error; read 2 ranges instead`). (b) Die Bounds: `if to > utf8.MaxRune || from > to` durch `if false` ersetzt -> alle drei Zeilen von `TestBrowserRefusalGuardRefusesABoundItCannotRepresent` ROT mit exakt Runde 5s Werten `[{0 -1}]`, `[{97 -1}]`, `[{31 0}]`. (c) Die Diagnose-Trennung: `if len(matches) == 0 && whole > 0` durch `if false` ersetzt -> die Zeile `the body cut short by a bracket in a comment` ROT. Alle Falsifikationen zurueckgenommen, Arbeitsbaum sauber."
    - "G-02-26 (Runde 5, gaps[1], `G5-2`) — GESCHLOSSEN. Ledger-Eintrag 70 (`unmet-truth`, `open`) eroeffnet mit `SUPERSEDES ENTRY 69, WHICH STAYS FIXED (no amend verb exists)`, benennt den gemessenen Praefix-Fall und die fehlende Bound-Validierung woertlich, trennt ausdruecklich, was an 69 weiterhin gilt, und traegt eine pruefbare Schliessbedingung. Er behauptet nicht, dass die allquantifizierte Wahrheit gilt. `02-26-SUMMARY.md` zieht die `coverage`-D1-Beschreibung auf das zurueck, was 02-26 tatsaechlich geliefert hat (`... oder unter einem Namen steht, der REFUSED_RANGES nicht als Praefix traegt` — fuer den Stand von 02-26 gemessen richtig) und markiert zwei Aussagen unter `## Next Phase Readiness` als WITHDRAWN, durchgestrichen, mit datiertem Korrekturblock, in genau der Form aus `02-21-SUMMARY.md:345-358`. Ledger unabhaengig dreifach geparst: 55 open / 0 waived / 16 fixed / 71 total in Frontmatter, Markdown-Tabelle und JSON-Block, null Status-Abweichungen pro Id, ids 1..71 lueckenlos und doppelfrei."
    - "G-02-27 (Runde 5, gaps[2], `G5-3`, 02-REVIEW WR-07) — GESCHLOSSEN. `internal/httpapi/handlers/schematics.go:458` sagt jetzt `see refreshTheStoredVerdict for the three conditions`; der Diff an dieser Datei ist genau diese eine Kommentarzeile. Sweep unabhaengig nachgefahren: `grep -rn 'refreshTheStoredVerdict'` ueber `internal`, `docs`, `web`, `cmd` mit `.go`/`.md`/`.ts`/`.tsx` -> null Treffer mit dem Zahlwort `two`. Die zwei Reststellen im Baum einzeln beurteilt und richtig: `internal/model/model.go:139` (`Arch's below for the one way the two conditions differ` — vergleicht die Versions- mit der Architektur-Bedingung und zaehlt nicht die Bedingungen des Refresh) und `internal/httpapi/middleware/csrf.go:112` (ohne Bezug zum Refresh)."
  gaps_remaining: []
  regressions:
    - "`.planning/REQUIREMENTS.md` markiert FACT-01, FACT-02 und FACT-06 wieder als `[x]` bzw. `Complete` — dieselbe Aenderung, die der Betreiber am selben Tag um 13:24 mit `2e57ee3 docs(phase-02): revert premature Complete requirements after gaps found` ausdruecklich zurueckgenommen hat. Getrieben von `requirements-completed: [FACT-01, FACT-06]` in `02-27-SUMMARY.md:52` und `[FACT-02, FACT-06]` in `02-28-SUMMARY.md:54`. Siehe gaps[1]."
  adversarial_checks_run:
    - "Die zentrale Frage dieser Runde — ob G-02-25 geschlossen ist — nicht von 02-27-SUMMARY.md und nicht von 02-REVIEW.md uebernommen, sondern an einer eigenen Sonde gegen die reine Funktion gemessen und danach am echten Baum. Die Sonde (`internal/imagefactory/zz_verifier_probe_test.go`) lief mit acht Faellen und wurde danach geloescht; `git status` ist leer."
    - "WR-01 des Runde-6-Reviews REPRODUZIERT und praezisiert. Der Reviewer behauptet, ein `REFUSED_RANGES`, das nur in einem `/* ... */`-Block oder einem Template-Literal ueberlebt, erfuelle den Anker weiterhin. Gemessen: Blockkommentar -> `ranges=[{0 31} {65279 65279}] err=<nil>`; Template-Literal -> `ranges=[{0 31}] err=<nil>`. Die Behauptung haelt. Praezisiert habe ich sie in zwei Richtungen, die der Reviewer nicht misst: eine mit `//` auskommentierte Deklaration erfuellt den Anker NICHT (`^\\s*(?:export\\s+)?const` scheitert an den beiden Schraegstrichen) und liefert korrekt `err != nil`; und ein vollstaendig auskommentiertes `REFUSED_RANGES` in `images.tsx` waere heute ein TypeScript-Fehler, weil `:144` den Bezeichner benutzt — der Schadensfall verlangt also zusaetzlich, dass die echte Tabelle in ein anderes Modul wandert und importiert wird. Drei Konjunktionen, dieselbe Struktur wie beim `_LEGACY`-Fall, den Runde 5 als blockierend gefuehrt hat."
    - "WR-02 des Runde-6-Reviews REPRODUZIERT. Der Spiegel-Defekt ist echt und in dieser Runde neu entstanden: `whole` wird ueber die GANZE Quelle gezaehlt (`:300`), also greift der neue Zweig `len(matches) == 0 && whole > 0` auch fuer eine tatsaechlich leere Deklaration, sobald irgendwo sonst ein eintragsfoermiges Literal steht. Gemessen: `const REFUSED_RANGES: readonly RefusedRange[] = []` plus eine zweite Tabelle -> `the REFUSED_RANGES declaration body was cut short before its first entry, but 1 entries are present in the source as a whole ... Look for that bracket, not for a missing table` — nach einer Klammer, die es nicht gibt. Ohne die zweite Tabelle greift weiterhin die richtige Leer-Meldung. Es ist ein strikter Tausch einer falschen Ursache gegen eine andere, beide fail closed."
    - "WR-04 des Runde-6-Reviews GEGENGEPRUEFT und ABGESCHWAECHT. Der Reviewer nennt die neue Praefix-Tabellenzeile `trivial erfuellt`, weil ihr `wantErr` nur `REFUSED_RANGES` ist und sechs der sieben Fehlermeldungen dieser Funktion diese Zeichenkette tragen. Die Beobachtung stimmt (nur `unreadable range start %q` traegt sie nicht). Sie ist aber nicht `trivial erfuellt`: die Zeile scheitert auch bei `err == nil` mit `t.Fatalf`, und genau dieser Zweig ist es, der ohne den neuen Anker greift — gemessen unter `gaps_closed[0](a)`: `no error; read 2 ranges instead`. Die Zeile traegt also die Eigenschaft und pinnt nur die Diagnose nicht. Warnung, kein Blocker."
    - "WR-03 des Runde-6-Reviews REPRODUZIERT und als klein bewertet. Die Bound-Meldung erklaert beide Faelle mit `a bound above utf8.MaxRune lands on a negative rune`; fuer die invertierte Range `{0x001f, 0x0000}` liegt keine Grenze oberhalb `utf8.MaxRune`. Der erste Satz der Meldung (`the lower bound at most the upper`) deckt den Fall korrekt ab, der erklaerende zweite nicht. Der Grenzfall der echten Datei stimmt: `{ from: 0xfffe, to: 0x10ffff }` in `images.tsx` wird nicht faelschlich abgelehnt — der Test ist gruen."
    - "Die Schliessbedingung, die Ledger-Eintrag 70 sich selbst gibt, WOERTLICH AUSGEFUEHRT: `eine Verifikationsrunde ... benennt REFUSED_RANGES praefixiert um und sieht TestBrowserRefusalSetEqualsTheServers rot, und sie setzt einen Eintrag mit einer Grenze oberhalb utf8.MaxRune und sieht den Waechter rot`. Beide Richtungen gemessen und beide ROT. Der Eintrag steht dennoch weiterhin auf `open` — was korrekt ist, solange gaps[0] offen ist, und was diese Verifikation nicht selbst umschreibt."
    - "Neu gemessen statt uebernommen, dass der Waechter HEUTE etwas misst: `{ from: 0xfeff, to: 0xfeff }` aus `images.tsx` geloescht -> `--- FAIL: TestBrowserRefusalSetEqualsTheServers`, `1 codepoints the server refuses are accepted ...: U+FEFF`. Datei wiederhergestellt. Es ist weiterhin nichts maskiert."
    - "Die Prohibition ueber `02-DECISION-schematic-identity.md` erneut gemessen: `head -c 9137 | shasum -a 256` = `ed2d08742734c2e46977ad4ac3bc3828a7206a7be27cdadb59fda41b26ee77c9`, byteweise identisch mit dem in Runde 5 gemessenen Wert. `:146` traegt unveraendert `This decision is therefore self-resolved, not ratified.`"
    - "`git diff 78b2fe2..HEAD --stat` gelesen, bevor irgendein Regressionsurteil gefaellt wurde: genau ZWEI Quelldateien geaendert (`guard_drift_test.go` +218, `schematics.go` 1 Zeile), keine Datei unter `web/`. SC 1, 2, 3 und 5 liegen ausserhalb dieses Diffs; ihre Belege wurden per `go test -list` neu aufgezaehlt statt abgeschrieben, und der fehlende Produktivaufrufer der Transport-Naht wurde per `grep` ueber `internal/httpapi` und `cmd/` neu bestaetigt (einziger Treffer: ein Kommentar in `router.go:100`)."
    - "Gates selbst gefahren statt vom Orchestrator uebernommen: `go build ./...` Exit 0, `go vet ./...` Exit 0, `gofmt -l internal/ cmd/` leer, `go test ./... -count=1` Exit 0 ueber 15 Testpakete (`internal/httpapi/handlers` 52.740s, `internal/imagefactory` 5.136s)."
gaps:
  - truth: "A guard reports a pass only when it measured the property it is named for — der Browser-Ablehnungs-Waechter meldet keine Zustimmung ueber eine Tabelle, die der Browser nicht ausfuehrt"
    status: partial
    reason: >-
      **Das dritte Loch, und diesmal ist es nicht dasselbe.** Runde 6 hat geliefert, was Runde 5
      ausgeschrieben hat, und ich habe es nicht geglaubt, sondern jede der drei Korrekturen einzeln
      ausgebaut und rot werden sehen. Der Anker bindet jetzt an den Namen: das Experiment, das Runde
      5 am echten Baum gruen gesehen hat, ist heute ROT in beiden Tests. Die Bounds werden vor der
      verlustbehafteten `rune`-Konversion semantisch validiert; die vier Werte aus Runde 5 erreichen
      den Sweep nicht mehr. Die Abschneide-Diagnose ist von der Leer-Diagnose getrennt. Das ist echte
      Arbeit, und es ist die Arbeit, die bestellt war.
      **Was bleibt, ist eine andere Tuer.** Der Anker ist ein regulaerer Ausdruck ueber Quelltext und
      kennt keinen lexikalischen Kontext. Selbst gemessen, nicht vom Code-Review uebernommen: eine
      `REFUSED_RANGES`-Deklaration, die nur noch in einem `/* ... */`-Block lebt, liefert
      `ranges=[{0 31} {65279 65279}] err=<nil>`; in einem Template-Literal `ranges=[{0 31}]
      err=<nil>`. Der Waechter meldet Zustimmung ueber eine Tabelle, die der Browser nicht ausfuehrt
      — woertlich die Eigenschaft, die der Satz in seinem eigenen Fehlertext verspricht
      (`:265-269`: "Renamed, moved or deleted is the same as never having been there").
      **Und ich habe das enger gemacht, als der Reviewer es fuehrt.** Eine mit `//` auskommentierte
      Deklaration erfuellt den Anker NICHT — `^\s*(?:export\s+)?const` scheitert an den beiden
      Schraegstrichen, gemessen `err != nil`. Ein vollstaendig auskommentiertes `REFUSED_RANGES` in
      `images.tsx` waere heute ausserdem ein TypeScript-Fehler, weil `:144` den Bezeichner benutzt.
      Der Schadensfall verlangt also drei Dinge zugleich: die echte Tabelle wandert in ein anderes
      Modul und wird importiert, eine Blockkommentar- oder Template-Kopie der alten Deklaration
      bleibt woertlich stehen, und die beiden Mengen laufen danach auseinander. Das sind exakt so
      viele Konjunktionen wie beim `_LEGACY`-Fall, den Runde 5 als blockierend gefuehrt hat — und
      genau deshalb fuehre ich ihn gleich.
      **Zweitens, und in dieser Runde neu entstanden:** die Diagnose-Trennung hat eine falsche
      Ursache gegen eine andere getauscht statt sie zu beseitigen. `whole` wird ueber die ganze
      Quelle gezaehlt (`:300`), also greift der neue Zweig auch fuer eine *tatsaechlich leere*
      Deklaration, sobald irgendwo sonst ein eintragsfoermiges Literal steht. Gemessen: `const
      REFUSED_RANGES: readonly RefusedRange[] = []` plus eine zweite Tabelle -> "the declaration body
      was cut short before its first entry ... Look for that bracket" — nach einer Klammer, die es
      nicht gibt. Vor dieser Runde war genau dieser Fall richtig diagnostiziert. Fail closed in
      beiden Fassungen; es ist die Diagnose, die wandert, nicht die Sicherheit.
      **Heute ist nichts maskiert**, neu gemessen: das Loeschen des U+FEFF-Eintrags laesst den
      Waechter scheitern und benennt den Codepoint. Es geht, wie in Runde 3, 4 und 5, um das
      kuenftige Verhalten des Waechters.
      **Der ehrliche Zusatz, der in diesen Eintrag gehoert:** drei Runden haben je ein engeres Loch
      in derselben allquantifizierten Eigenschaft gefunden, und ein regulaerer Ausdruck ueber
      TypeScript-Quelltext hat einen Rest, der nicht auf null geht. Die Schliessung muss deshalb
      nicht zwingend `noch ein Loch stopfen` heissen — sie kann auch heissen, die Behauptung des
      Waechters auf das zu verengen, was ein regulaerer Ausdruck messen kann, und den Rest als
      bewusst akzeptierten offenen Ledger-Eintrag zu fuehren. Beide Wege stehen unter `missing`.
    artifacts:
      - path: "internal/imagefactory/guard_drift_test.go:108-110"
        issue: "`refusedRangesDecl` ist ein regulaerer Ausdruck ohne lexikalischen Kontext: eine Deklaration in einem `/* ... */`-Block oder einem Template-Literal erfuellt den Anker und wird gelesen (gemessen `err=<nil>`)"
      - path: "internal/imagefactory/guard_drift_test.go:300, 307-315"
        issue: "`whole` zaehlt ueber die ganze Quelle, also meldet der neue Abschneide-Zweig `cut short ... Look for that bracket` fuer eine tatsaechlich leere Deklaration, sobald die Datei irgendwo sonst ein eintragsfoermiges Literal traegt — in dieser Runde neu entstanden"
      - path: "internal/imagefactory/guard_drift_test.go:462-491"
        issue: "keine Tabellenzeile deckt eine Deklaration ab, die nur in einem Kommentar oder Template-Literal existiert; und drei der sechs Zeilen pruefen mit `wantErr: \"REFUSED_RANGES\"` eine Zeichenkette, die sechs der sieben Fehlermeldungen tragen (02-REVIEW WR-04) — die Zeilen tragen die Eigenschaft, pinnen aber die Diagnose nicht"
      - path: "internal/imagefactory/guard_drift_test.go:370-379"
        issue: "die Bound-Meldung erklaert die invertierte Range mit der Ursache der nicht darstellbaren (02-REVIEW WR-03); der erste Satz deckt beide Faelle, der erklaerende zweite nur einen"
    missing:
      - "Entweder: den Quelltext vor dem Matchen von `/* ... */`-Bloecken und Template-Literalen befreien (eine Hilfsfunktion, die beide Formen durch Leerraum ersetzt und die Zeilenstruktur erhaelt), plus je eine Tabellenzeile fuer den Blockkommentar und das Template-Literal — genau die zwei Faelle, die heute fehlen und die ich gemessen habe"
      - "Oder, falls der Rest bewusst akzeptiert wird: die Behauptung des Waechters in seinem eigenen Fehlertext und in `.planning/WINDOWS.md` Eintrag 70 auf das verengen, was ein regulaerer Ausdruck leisten kann (`Renamed, moved or deleted` -> `unter einem anderen Bezeichner deklariert`), einen eigenen offenen Ledger-Eintrag fuer den lexikalischen Rest anlegen und die Ausnahme hier per `overrides:` ratifizieren. Das ist ein vollwertiger Schliessungsweg und keine Ausrede — er verlangt nur, dass die Verengung dort steht, wo sie gelesen wird"
      - "Den Spiegel-Defekt beseitigen statt ihn zu tauschen: `whole` fuer diese eine Unterscheidung nur ueber die Quelle AUSSERHALB der Deklaration zaehlen, oder den Abschneide-Zweig an das Vorhandensein eines `]` vor dem ersten Eintrag binden statt an `whole > 0`. Plus eine Tabellenzeile `eine wirklich leere Deklaration neben einer zweiten Tabelle`, die heute die falsche Meldung bekommt"
      - "Die drei `wantErr: \"REFUSED_RANGES\"`-Zeilen auf je eine unterscheidende Teilzeichenkette heben (`no REFUSED_RANGES declared`, `carries no entries`, ...) — dieselbe Regel, die `02-27-SUMMARY.md` unter `key-decisions` fuer die Bound-Meldung selbst aufgestellt und hier nicht angewendet hat"
      - "`from > to` in der Bound-Meldung mit seiner eigenen Ursache erklaeren (ein Sweep, der aufwaerts laeuft, betritt eine invertierte Range nie), statt mit der von `to > utf8.MaxRune`"
  - truth: "Das Traceability-Register behauptet nur, was der Phasenstand traegt (die T-02-62 / G-02-1-Linie, angewandt auf `.planning/REQUIREMENTS.md`)"
    status: partial
    reason: >-
      Der Ledger und die SUMMARYs sind in dieser Runde in Ordnung gebracht worden — das ist truths 8,
      14 und 15, und es ist gute Arbeit. Ein Register weiter passiert derselbe Fehler noch einmal.
      `.planning/REQUIREMENTS.md` markiert FACT-01, FACT-02 und FACT-06 wieder als `[x]` und in der
      Traceability-Tabelle als `Complete`. Der Betreiber hat exakt diese drei Zeilen am selben Tag um
      13:24 zurueckgenommen, mit einer Commit-Nachricht, die die Regel ausspricht: `2e57ee3
      docs(phase-02): revert premature Complete requirements after gaps found`.
      Der Ausloeser steht in den Artefakten dieser Runde selbst: `02-27-SUMMARY.md:52`
      `requirements-completed: [FACT-01, FACT-06]` und `02-28-SUMMARY.md:54`
      `requirements-completed: [FACT-02, FACT-06]`. Und darin liegt die eigentliche Spannung: derselbe
      Plan, dessen SUMMARY FACT-06 als abgeschlossen meldet, sagt an zwei Stellen ausdruecklich
      (`02-27-SUMMARY.md:104` und `:262`), dass er G-02-25 NICHT als geschlossen meldet — und G-02-25
      ist die Zuverlaessigkeit des Drift-Waechters ueber genau die Menge, an der FACT-06 haengt. Die
      Disziplin, die dieser Plan an der einen Stelle vorbildlich einhaelt, faellt an der anderen aus.
      Sachlich sind die drei Requirements erfuellt — Runde 5 hat sie einzeln als SATISFIED gefuehrt
      und ich fuehre sie in Abschnitt 6 wieder so. Falsch ist nicht die Aussage ueber die Codebasis,
      sondern der Statuswechsel gegen eine ausdrueckliche, dokumentierte Entscheidung des Betreibers,
      waehrend die Phase `gaps_found` traegt. Deshalb `partial` und nicht `failed`.
      Zwei kleinere Geschwister im selben Diff, die dieselbe Wurzel haben (Werkzeugausgabe, die eine
      handgeschriebene Aussage ueberschreibt, ohne sie zu lesen): `.planning/ROADMAP.md:81` sagt jetzt
      im selben Satz `28/28 plans executed (26/28 ausgefuehrt)` und `— 26/28 ausgefuehrt, 02-27 und
      02-28 geplant`; `.planning/STATE.md` sagt unter `## Current Position` `Plan: 3 of 28` und
      `Status: Ready to execute`, waehrend sein eigenes Frontmatter `completed_plans: 34` traegt.
    artifacts:
      - path: ".planning/REQUIREMENTS.md (FACT-01, FACT-02, FACT-06)"
        issue: "`[x]` und `Complete` wieder gesetzt, nachdem `2e57ee3` dieselben drei Zeilen ausdruecklich als `premature ... after gaps found` zurueckgenommen hat"
      - path: ".planning/ROADMAP.md:81"
        issue: "`28/28 plans executed (26/28 ausgefuehrt) ... — 26/28 ausgefuehrt, 02-27 und 02-28 geplant` — zwei einander widersprechende Zahlen in einem Satz"
      - path: ".planning/STATE.md (## Current Position)"
        issue: "`Plan: 3 of 28` und `Status: Ready to execute` gegen `completed_plans: 34` im eigenen Frontmatter"
    missing:
      - "FACT-01, FACT-02 und FACT-06 auf `[ ]` / `Gaps Found` zuruecknehmen, solange die Phase `gaps_found` traegt — oder der Betreiber ratifiziert ausdruecklich, dass ein einzelnes Requirement `Complete` werden darf, waehrend die Phase Luecken traegt, und `2e57ee3` wird als ueberholt vermerkt. Beides ist vertretbar; unvermerkt beides nochmal zu tun ist es nicht"
      - "`.planning/ROADMAP.md:81` auf eine einzige Zahl bringen"
      - "`.planning/STATE.md` `## Current Position` mit dem eigenen Frontmatter in Einklang bringen (`Plan: 28 of 28`)"
deferred:
  - truth: "Ein nicht erreichbarer Node blockiert die UI nicht (die UI-Haelfte von ROADMAP SC 3 / TRANS-05)"
    addressed_in: "Phase 3"
    evidence: >-
      Bei `2211c80` neu gemessen: `grep` ueber `internal/httpapi` und `cmd/` nach
      `NewClusterClient`, `NewMaintenanceClient`, `FanOut`, `NewBreaker`, `NewDirectDialer`,
      `NewManualSource` findet genau einen Treffer, und der ist ein Kommentar
      (`internal/httpapi/router.go:100`). Es gibt weiterhin keinen Produktivaufrufer der Naht. Die
      Transport-Haelfte ist verhaltensbelegt (`TestFanOutOneSilentNodeCostsOneNode`,
      `TestFanOutCancellationTerminatesEveryInFlightCall`,
      `TestFanOutSkipsAnOpenCircuitWithoutDialing` aufgezaehlt und im gruenen Paket). ROADMAP Phase 3
      besitzt die Inventar-Route.
  - truth: "Dieselbe Contract-Suite laeuft auch gegen echtes Talos (TRANS-08)"
    addressed_in: "Phase 3"
    evidence: "REQUIREMENTS.md bildet TRANS-08 auf Phase 3 (Pending) ab; ROADMAP Phase 3 SC 5 und die `Note` sagen, warum. `internal/talos/contract_test.go:129` parametrisiert den Transport bereits."
  - truth: "G-02-9 — eine Probe, die trotz erhoehtem Budget ausbleibt, ist weiterhin dauerhaft"
    addressed_in: "02-DECISION-probe-budget.md Option 1, bewusst nicht genommen"
    evidence: >-
      Weiterhin ehrlich verbucht, und diese Runde hat den Eintrag erneut nicht verwaessert, obwohl
      sie die Buchhaltung angefasst hat. Ledger-Eintraege 58 und 66 tragen ihn und stehen beide auf
      `open` (unabhaengig aus dem JSON-Block gelesen). Der neue Eintrag 70 schliesst ausdruecklich
      mit `Die Eintraege 58 und 66 tragen G-02-9 und bleiben unberuehrt offen. G-02-9 IST OFFEN.`
      `02-27-SUMMARY.md` und `02-28-SUMMARY.md` nennen ihn ausschliesslich als offen.
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
      Unveraendert und bei `2211c80` erneut gemessen: es gibt weiterhin keinen Produktivaufrufer der
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
      Bei `2211c80` neu gemessen statt abgeschrieben: `web/src/api.ts:475` `REQUEST_CEILING_MS =
      150_000`, `web/src/api.test.ts:99` `expect(call?.[0]).toBeGreaterThan(130_000)`,
      `cmd/holzkube-managerd/main.go:64` `writeTimeout = 130 * time.Second`. Eine zweite
      Handtranskription auf der TypeScript-Seite, die sich weder mit der Go-Konstante noch mit
      `REQUEST_CEILING_MS` bewegt. `writeTimeout` auf 160s zu heben liesse beide gruen, waehrend
      jedes lange Create bei 150s abbraeche. Nur beratend, aendert weder Status noch Score.
human_verification:
  - test: >-
      Entscheiden, welchen der beiden Schliessungswege fuer den Waechter-Rest die Phase nimmt:
      den lexikalischen Kontext beseitigen (Blockkommentare und Template-Literale vor dem Matchen
      ausblenden), oder die Behauptung des Waechters auf das verengen, was ein regulaerer Ausdruck
      leisten kann, und den Rest als offenen Ledger-Eintrag mit `overrides:` ratifizieren.
    expected: >-
      Eine der beiden Optionen ist gewaehlt und in `.planning/WINDOWS.md` bzw. in den `overrides:`
      dieser Datei festgehalten.
    why_human: >-
      Das ist eine Kosten-Nutzen-Entscheidung und keine Messung. Drei Runden haben je ein engeres
      Loch in derselben allquantifizierten Eigenschaft gefunden; ein regulaerer Ausdruck ueber
      TypeScript-Quelltext hat einen Rest, der nicht auf null geht. Ob die Phase eine vierte Runde
      dafuer ausgibt oder die Behauptung ehrlich verengt, gehoert dem Betreiber.
  - test: >-
      Entscheiden, ob FACT-01, FACT-02 und FACT-06 in `.planning/REQUIREMENTS.md` `Complete` bleiben
      duerfen, waehrend die Phase `gaps_found` traegt.
    expected: >-
      Entweder zurueckgenommen wie in `2e57ee3`, oder die Regel wird ausdruecklich geaendert und
      `2e57ee3` als ueberholt vermerkt.
    why_human: >-
      Der Betreiber hat die Regel am 2026-09-04 um 13:24 durch eine Handlung ausgesprochen und die
      Werkzeugkette hat sie um 19:41 wieder ueberschrieben. Welche der beiden gilt, ist eine
      Entscheidung und keine Messung. Siehe gaps[1].
  - test: >-
      Das zusammengebaute Binary durch einen Browser gegen die echte Image Factory fahren: ein
      Schematic von Anfang bis Ende autoren und bestaetigen, dass die /images-Route ausserhalb eines
      Test-Harness funktioniert (02-UAT.md Test 5, Unterpruefungen a-d).
    expected: "Die Route verhaelt sich, wie die Suiten es vorhersagen, mit echter Latenz und echtem Bundle."
    why_human: >-
      Bei `2211c80` weiterhin offen: 02-UAT.md Test 5 traegt `result: issue`.
      `images.browser.test.tsx` oeffnet ImagesView in einem echten Chromium, sagt in seinem eigenen
      Doc-Kommentar aber "no binary runs, no bundle is served, and the embedded UI has still never
      been driven end to end in a browser."
  - test: "Bestaetigen, dass der Browser-Install-Schritt der CI auf ubuntu-latest laeuft."
    expected: "`npm --prefix web exec -- playwright install --with-deps chromium` (.github/workflows/ci.yml:71) gelingt und das Browser-Projekt laeuft in der CI."
    why_human: >-
      WINDOWS-Eintrag 44 ist bei diesem Commit weiterhin offen. Das Browser-Projekt ist der einzige
      Beleg fuer G-02-10 und G-02-19. Es scheitert geschlossen — `requireBrowserBinary()` wirft mit
      dem Kommando, das es richtet —, das Risiko ist also ein roter CI-Lauf und kein stiller Pass.
  - test: "Eine kalte Installer-Aufloesung gegen factory.talos.dev nach dem nebenlaeufigen Fan-out aus Plan 02-23 neu messen."
    expected: "Eine kalte Aufloesung kostet den langsamsten einzelnen Kandidaten, nicht die Summe — die Verbesserung, die 02-23 behauptet."
    why_human: >-
      02-24-SUMMARY.md:232 und WINDOWS-Eintrag 57 sagen es selbst: die Verbesserung ist offline gegen
      Fakes gemessen und gegen factory.talos.dev unmessen.
  - test: >-
      Entscheiden, wo die Nicht-Gap-Befunde der Code-Reviews leben: aus Runde 6 die Reste WR-03
      (Ursache der invertierten Range), WR-05 (Doc-Kommentar am falschen Symbol, in dieser Runde um
      25 Zeilen gewachsen) und IN-01..IN-05 — dazu die aus Runde 5 unveraendert anstehenden WR-06
      (`versionMismatchReason` ohne Leerwert-Zweig), der Surrogate-Punkttest, IN-02..IN-07 und die
      vier Warnungen der Vorvorrunde (SecureBoot-`ParseBool`, Installer-Provenienz, ungewachte
      Ceiling-Transkription, `allowedHosts`/`ssoOnly`).
    expected: "Jeder Punkt ist entweder behoben, mit `gsd-tools windows append` als Window abgelegt oder ausdruecklich abgelehnt."
    why_human: >-
      Eine Policy-Entscheidung, keine Messung. Geprueft: keiner dieser Punkte steht in irgendeiner
      der drei Repraesentationen von `.planning/WINDOWS.md`, und `workflow.windows_enforce` liest
      genau dieses Register beim Ship. Die Liste ist gegenueber Runde 5 wieder laenger geworden.
  - test: "`⚠️ PRESENT_BEHAVIOR_UNVERIFIED` — die UI-Haelfte von SC 3, siehe behavior_unverified_items."
    expected: "Erst nach Phase 3 ausuebbar."
    why_human: "Kein Codepfad vorhanden; die Transport-Haelfte ist belegt."
---

# Phase 2 Verification — Runde 6 (die beiden Gap-Closure-Plaene 02-27 und 02-28)

**Phase Goal:** Jede Talos-Interaktion laeuft durch eine austauschbare Naht und ist ohne Hardware
testbar; Schematics und Image-URLs sind korrekt und nachweislich brauchbar herleitbar.
**Verified:** 2026-09-04T20:05:54Z bei `2211c80` (Branch `main`, Arbeitsbaum sauber; alle
Falsifikationen und die eigene Sonde wurden zurueckgenommen und `git status` ist am Ende leer)
**Status:** gaps_found — alle drei Gaps der Runde 5 geschlossen, ein engeres viertes Loch im selben
Waechter gemessen, ein Buchhaltungsrueckfall
**Re-verification:** Ja — Runde 6 ueber die Plaene 02-27 und 02-28, den Runde-5-Bericht bei
`d7d07ea` ueberschreibend.

Runde 6 hatte drei Auftraege und hat alle drei erledigt. **Ich habe keinen davon geglaubt.** Fuer
G-02-25 habe ich jede der drei Korrekturen einzeln ausgebaut und die zugehoerige Tabellenzeile rot
werden sehen, und danach Runde 5s Experiment am echten Baum woertlich wiederholt: `REFUSED_RANGES`
durchgaengig in `REFUSED_RANGES_LEGACY`, und wo Runde 5 zweimal `PASS` gemessen hat, steht heute
zweimal `FAIL`. Fuer G-02-26 habe ich den Ledger unabhaengig dreifach geparst und den Text von
Eintrag 70 gegen das gelesen, was er ueberholt. Fuer G-02-27 habe ich den Sweep selbst gefahren.

**Zwei Dinge verdienen ausdrueckliches Lob, bevor der Rest kommt.** Erstens: `02-27-SUMMARY.md`
sagt an zwei Stellen ausdruecklich, dass der Plan G-02-25 **nicht** als geschlossen meldet und dass
diese Feststellung einer Verifikationsrunde gehoert. Das ist genau die Disziplin, deren Fehlen der
Befund von Runde 5 an Ledger-Eintrag 69 war, prospektiv angewendet. Zweitens: Ledger-Eintrag 70
gibt sich eine **pruefbare Schliessbedingung** und behauptet nichts darueber hinaus. Ich habe diese
Bedingung woertlich ausgefuehrt, und sie ist in beiden Richtungen erfuellt.

Der Runde-6-Code-Review, der Minuten vor dieser Verifikation lief, hat zwei Reste gemeldet. Ich habe
beide nachgemessen statt uebernommen: **WR-01 haelt** (eine Deklaration in einem Blockkommentar oder
Template-Literal erfuellt den Anker weiterhin — selbst gemessen `err=<nil>`), **WR-02 haelt**
(der neue Abschneide-Zweig meldet fuer eine wirklich leere Deklaration die falsche Ursache), und
**WR-04 habe ich abgeschwaecht** — der Reviewer nennt die neue Praefix-Tabellenzeile "trivial
erfuellt"; sie ist es nicht, denn ihr `err == nil`-Zweig ist genau der, der ohne den neuen Anker
greift, was ich gemessen habe.

Nichts in diesem Bericht ist aus Runde 5 uebernommen, ohne bei `2211c80` neu gemessen worden zu sein.

## 1. Observable truths

| # | Truth | Status | Evidence bei `2211c80` |
|---|---|---|---|
| 1 | SC 1 — der **unveraenderte** Produktions-Client spricht gegen `talossim`: echte Protobufs, echtes mTLS, echter In-Memory-COSI-State, ohne Hardware, ohne `talosctl`, ohne Netz | ✓ VERIFIED | Ausserhalb des Runde-6-Diffs (`git diff 78b2fe2..HEAD --stat`: zwei Quelldateien, keine in `internal/talossim`). Neu aufgezaehlt statt abgeschrieben: `go test ./internal/talossim/ -list '.*'` fuehrt `TestTracerRealClientReachesFakeNode` und `TestDryRunApplyChangesNothing`; das Paket ist im vollen Lauf gruen (3.773s). |
| 2 | SC 2 — der Betreiber schaltet neun Fehlerszenarien und der Client verhaelt sich definiert | ✓ VERIFIED | `internal/talossim/scenario.go:48-57` deklariert alle neun namentlich; neu aufgezaehlt: `TestDocumentedScenariosMatchRegistry`, `TestInjectRefusesAnUnknownScenario` und neun `TestScenario*`-Tests im Simulator-Paket, dazu `TestScenarioContract` (`internal/talos/contract_test.go:129`) und `TestGoSilentFailsAtItsOwnClassDeadline` (`deadline_test.go:250`). Von Runde 6 nicht beruehrt. |
| 3 | SC 3 — ein unerreichbarer Node blockiert weder die UI noch andere Nodes; jeder Aufruf hat ein erzwungenes Deadline; Retries nur fuer eine Lese-Allowlist; Cluster- und Maintenance-Clients sind getrennte Typen | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Transport-Haelfte neu aufgezaehlt und gruen: `TestFanOutOneSilentNodeCostsOneNode`, `TestFanOutCancellationTerminatesEveryInFlightCall`, `TestFanOutSkipsAnOpenCircuitWithoutDialing`, `TestRequireDeadline`, `TestRetryAllowlistIsExactlyTheFastReadClass`, `TestMaintenanceClientMethodSetIsClosed`. Die **UI**-Haelfte hat weiterhin keinen Codepfad: `grep` ueber `internal/httpapi` und `cmd/` findet als einzigen Treffer einen Kommentar (`router.go:100`). Nach Phase 3 verschoben. |
| 4 | SC 4 — der Betreiber stellt ein Schematic aus einem versions-skopierten Katalog zusammen, bekommt die exakten ISO-/Installer-/PXE-URLs, **und das Schematic gilt erst nach einem bestaetigenden Model-Build-Probe als brauchbar**; Kernel-Args oder META loesen die installer/initramfs-Warnung aus | ✓ VERIFIED | Die dritte Bedingung `if stored.TalosVersion != fresh.TalosVersion` steht unveraendert (`schematics.go:579-582`; der einzige Diff an dieser Datei in dieser Runde ist ein Zahlwort in einem Kommentar). `internal/httpapi/handlers` ist im vollen Lauf gruen (52.740s). Runde 5 hat diesen Konjunkt fail-first belegt; er liegt ausserhalb des Runde-6-Diffs. |
| 5 | SC 5 — das ganze Binary laeuft mit `--dry-run` und keine Mutation erreicht einen Node | ✓ VERIFIED | Ausserhalb des Runde-6-Diffs. `TestDryRunRefusesEveryMutationAtTheNode` neu aufgezaehlt und im gruenen Paket; Kompositionswurzel `cmd/holzkube-managerd/main.go:171` `talos.Mode{DryRun: cfg.DryRun}` unveraendert. |
| 6 | R3/R4/R5-Uebertrag — ein Waechter meldet einen Pass nur, wenn er die Eigenschaft gemessen hat, nach der er benannt ist | ✗ FAILED (partial) | Die beiden Loecher der Runde 5 sind **zu** (siehe truths 10 und 11, je fail-first). Ein drittes ist offen und selbst gemessen: eine `REFUSED_RANGES`-Deklaration, die nur in einem `/* ... */`-Block lebt -> `ranges=[{0 31} {65279 65279}] err=<nil>`; in einem Template-Literal -> `ranges=[{0 31}] err=<nil>`. Eine `//`-auskommentierte scheitert korrekt. Siehe gaps[0]. |
| 7 | R3-Uebertrag — ein blockierender Human-Decision-Checkpoint, der ohne Menschen aufgeloest wurde, sagt das dort, wo die Entscheidung gelesen wird | ✓ VERIFIED | `02-DECISION-schematic-identity.md:146` unveraendert ("This decision is therefore self-resolved, not ratified."). Praefix-Hash neu gemessen: `head -c 9137 \| shasum -a 256` = `ed2d0874…ee77c9`, byteweise identisch mit Runde 5. |
| 8 | R5-Uebertrag — ein Abschlussdokument und der Ledger sagen nur, was von der Codebasis wahr ist | ✓ VERIFIED | **Runde 5s gaps[1] ist geschlossen.** Eintrag 70 (`unmet-truth`, `open`) ueberholt 69 ausdruecklich, benennt den gemessenen Praefix-Fall und die fehlenden Bounds, trennt was an 69 gilt von dem, was nicht gilt, und behauptet die allquantifizierte Wahrheit ausdruecklich nicht. `02-26-SUMMARY.md` traegt die korrigierte `coverage`-D1-Zeile und zwei WITHDRAWN-Bloecke in der Form aus `02-21-SUMMARY.md:345-358`. Siehe truths 14 und 15. |
| 9 | R5-Uebertrag — jede Aussage im Baum, die zwei Bedingungen des Refresh nennt, nennt jetzt drei | ✓ VERIFIED | **Runde 5s gaps[2] ist geschlossen.** `schematics.go:458` sagt "the three conditions"; der Diff an dieser Datei ist genau diese Zeile. Sweep selbst gefahren: `grep -rn 'refreshTheStoredVerdict'` ueber `internal`, `docs`, `web`, `cmd` -> null Treffer mit `two`. Die zwei Reststellen im Baum (`model.go:139`, `csrf.go:112`) einzeln gelesen und beide korrekt. |
| 10 | 02-27 — der Anker bindet an den Bezeichner und nicht an sein Praefix | ✓ VERIFIED | Zweifach fail-first. An der reinen Funktion: `REFUSED_RANGES_LEGACY` und `REFUSED_RANGESX` liefern beide `err != nil` (Runde 5: `[{0 0}]` / `[{0 1}]` mit `err=<nil>`). Am echten Baum, als woertliche Wiederholung von Runde 5: Umbenennung in `REFUSED_RANGES_LEGACY` -> `--- FAIL: TestBrowserRefusalSetEqualsTheServers` **und** `--- FAIL: .../the_real_route`. Anker auf die Runde-5-Form zurueckgedreht -> die neue Tabellenzeile ROT (`no error; read 2 ranges instead`). |
| 11 | 02-27 — die gelesenen Grenzen werden semantisch validiert, bevor die verlustbehaftete `rune`-Konversion passiert | ✓ VERIFIED | `if to > utf8.MaxRune \|\| from > to` (:370) steht vor dem `append`. Fail-first: durch `if false` ersetzt -> alle drei Tabellenzeilen ROT mit exakt Runde 5s Werten `[{0 -1}]`, `[{97 -1}]`, `[{31 0}]`. Der Grenzfall der echten Datei (`{ from: 0xfffe, to: 0x10ffff }`) wird nicht faelschlich abgelehnt — der Set-Test ist gruen. |
| 12 | 02-27 — die Abschneide-Diagnose behauptet nicht mehr, die Deklaration sei leer, wenn die Quelle Eintraege traegt | ✓ VERIFIED | `if len(matches) == 0 && whole > 0` (:307) greift vor dem Leer-Zweig; `whole` wird vorher gezaehlt (:300). Fail-first: durch `if false` ersetzt -> die Zeile `the body cut short by a bracket in a comment` ROT. **Aber:** der Zweig zaehlt ueber die ganze Quelle und meldet dieselbe Ursache jetzt fuer eine wirklich leere Deklaration neben einer zweiten Tabelle — ein Tausch, kein Wegfall. Der Beleg ist erbracht, der Spiegel-Defekt steht in gaps[0]. |
| 13 | 02-27 — `TestBrowserRefusalSetEqualsTheServers` bleibt gruen und misst weiterhin dasselbe | ✓ VERIFIED | Neu gemessen: den `{ from: 0xfeff, to: 0xfeff }`-Eintrag aus `images.tsx` geloescht -> `--- FAIL`, `1 codepoints the server refuses are accepted …: U+FEFF`. Datei wiederhergestellt, `git diff` leer. Heute ist nichts maskiert. |
| 14 | 02-28 — der Ledger ueberholt Eintrag 69 mit einem offenen Eintrag statt ihn zu bearbeiten, und der neue Eintrag behauptet nicht mehr als seine Messung | ✓ VERIFIED | Eintrag 70, `unmet-truth`, `open`, eroeffnet mit `SUPERSEDES ENTRY 69, WHICH STAYS FIXED (no amend verb exists)`. Er trennt ausdruecklich `WAS UEBERHOLT IST` von `WAS AN 69 WEITERHIN GILT`, nennt alle sechs Runde-5-Messwerte, gibt die Commits je Korrektur, und sagt `Ob die allquantifizierte Wahrheit nach Plan 02-27 gilt, stellt eine Verifikationsrunde fest; dieser Eintrag behauptet es nicht.` Seine Schliessbedingung habe ich woertlich ausgefuehrt — beide Richtungen ROT. |
| 15 | 02-28 — `02-26-SUMMARY.md` zieht seine ueberbreiten Aussagen in der etablierten Form zurueck | ✓ VERIFIED | `coverage` D1 ist auf den Stand von 02-26 verengt (`... oder unter einem Namen steht, der REFUSED_RANGES nicht als Praefix traegt` — fuer 02-26 gemessen richtig) mit einem `correction:`-Block, der den alten Wortlaut woertlich stehen laesst. Zwei Aussagen unter `## Next Phase Readiness` sind durchgestrichen, mit `— WITHDRAWN`, datiertem Korrekturblock und der Messung; die zweite trennt sauber `G-02-24 IST geschlossen` von `G-02-25 ist es nicht`. Genau die Form aus `02-21-SUMMARY.md:345-358`. |
| 16 | Der Ledger ist in allen drei Repraesentationen selbstkonsistent | ✓ VERIFIED | Unabhaengig geparst: Frontmatter 55 open / 0 waived / 16 fixed / 71 total; Markdown-Tabelle 71 Zeilen mit 55 open + 16 fixed; JSON-Block 71 Eintraege mit denselben Status. Null Status-Abweichungen **pro Id**, ids 1..71 lueckenlos und doppelfrei. |
| 17 | G-02-9 bleibt ehrlich als offen verbucht, auch in einer Runde, die die Buchhaltung anfasst | ✓ VERIFIED | Eintraege 58 und 66 aus dem JSON-Block gelesen: beide `open`. Eintrag 70 schliesst mit `Die Eintraege 58 und 66 tragen G-02-9 und bleiben unberuehrt offen. G-02-9 IST OFFEN.` `02-27-SUMMARY.md` und `02-28-SUMMARY.md` nennen ihn ausschliesslich als offen. Eintrag 56 (die nicht erschoepfend gemessene SERVER-Menge) bleibt daneben `open` und wird von 70 ausdruecklich nicht ueberholt. |
| 18 | Das Traceability-Register behauptet nur, was der Phasenstand traegt | ✗ FAILED (partial) | `.planning/REQUIREMENTS.md` markiert FACT-01, FACT-02 und FACT-06 wieder `[x]` / `Complete` — dieselben drei Zeilen, die der Betreiber am selben Tag um 13:24 mit `2e57ee3 ... revert premature Complete requirements after gaps found` zurueckgenommen hat. Ausloeser: `requirements-completed` in beiden neuen SUMMARYs. Dazu `ROADMAP.md:81` (`28/28 plans executed (26/28 ausgefuehrt) … — 26/28 ausgefuehrt`) und `STATE.md` (`Plan: 3 of 28`, `Status: Ready to execute` gegen `completed_plans: 34`). Siehe gaps[1]. |
| 19 | Die stehende Wahrheit der Phase — kein gespeicherter Zustand und kein Register behauptet mehr, als sein Traeger traegt (T-02-62 / G-02-1 / G-02-8-Linie) | ✓ VERIFIED | Fuer den **Store** seit Runde 5 wiederhergestellt und in dieser Runde unberuehrt. Fuer den **Ledger** und die **SUMMARY** in dieser Runde wiederhergestellt (truths 8, 14, 15) — und zwar prospektiv, weil `02-27-SUMMARY.md:262` die Nicht-Behauptung ausdruecklich als Regel formuliert. Der Rueckfall im Traceability-Register ist truth 18 und eine Ebene darueber. |

**Score:** 16/19 truths verified (1 present, behavior-unverified; 2 failed, in zwei verschiedenen
Registern und ohne gemeinsame Wurzel).

## 2. Deferred items

| # | Item | Addressed In | Evidence |
|---|---|---|---|
| 1 | Die UI-Haelfte von SC 3 / TRANS-05 | Phase 3 | Kein Produktivaufrufer der Naht (neu gemessen: einziger Treffer ein Kommentar in `router.go:100`); ROADMAP Phase 3 besitzt die Inventar-Route. |
| 2 | TRANS-08 — die Contract-Suite gegen echtes Talos | Phase 3 | REQUIREMENTS.md (Pending); ROADMAP Phase 3 SC 5 und die `Note` sagen, warum. `contract_test.go:129` parametrisiert den Transport bereits. |
| 3 | G-02-9 — eine ausgebliebene Probe ist weiterhin dauerhaft | 02-DECISION-probe-budget.md Option 1, nicht genommen | Ledger 58 und 66 beide `open`, aus dem JSON-Block gelesen; Eintrag 70 bekraeftigt es in Grossbuchstaben. **Auch diese Runde hat die Buchhaltung angefasst und den Eintrag nicht verwaessert.** |

## 3. Required artifacts

| Artifact | Expected | Status | Details |
|---|---|---|---|
| `internal/imagefactory/guard_drift_test.go` | ein an seinen Bezeichner gebundener Waechter, dessen Grenzen validiert und dessen Diagnosen getrennt sind | ⚠️ PARTIAL | Alle drei bestellten Korrekturen sind da und je fail-first belegt (:108-110 Anker, :370 Bounds, :307 Diagnose-Trennung), die drei Falsifikationstabellen tragen jetzt 6, 3 und 2 Zeilen und keine bestehende Zeile wurde entfernt. Der lexikalische Rest (Blockkommentar, Template-Literal) und der Spiegel-Defekt der Leer-Diagnose sind gaps[0]. |
| `internal/httpapi/handlers/schematics.go` | der Doc-Verweis nennt drei Bedingungen | ✓ VERIFIED | :458 `for the three conditions`. Der gesamte Diff an dieser Datei in dieser Runde ist diese eine Zeile; die dritte Bedingung selbst (:579-582) ist unberuehrt. |
| `.planning/WINDOWS.md` | ein offener Eintrag, der 69 ueberholt, ohne es zu bearbeiten, mit pruefbarer Schliessbedingung | ✓ VERIFIED | Eintrag 70 (`unmet-truth`, `open`) und 71 (`deviation`, `fixed`, der Zahlwort-Fix mit beiden Sweeps und den vier einzeln beurteilten Reststellen). Drei Repraesentationen unabhaengig geparst: 55/0/16/71, null Abweichungen pro Id. |
| `02-26-SUMMARY.md` | die drei ueberbreiten Aussagen zurueckgezogen, in der Form aus `02-21-SUMMARY.md` | ✓ VERIFIED | `coverage` D1 verengt mit `correction:`-Block; zwei `## Next Phase Readiness`-Punkte durchgestrichen mit `— WITHDRAWN` und datiertem Korrekturblock. Die `verification`-Eintraege und `human_judgment: false` bleiben unveraendert, mit der Begruendung, dass nicht ihr Ergebnis falsch war, sondern die Breite der Beschreibung. |
| `02-27-SUMMARY.md` | eine SUMMARY, die G-02-25 **nicht** als geschlossen meldet | ✓ VERIFIED | `:104` und `:262` sagen es ausdruecklich und nennen den Grund: "Genau diese Reihenfolge zu verletzen war der Befund von Runde 5 an Ledger-Eintrag 69." Die Feststellung ist dieser Verifikation ueberlassen worden. |
| `web/src/routes/images.tsx` | von dieser Runde netto unberuehrt | ✓ VERIFIED | Nicht im `git diff 78b2fe2..HEAD --stat`. Meine beiden Falsifikationen (Praefix-Umbenennung, U+FEFF-Loeschung) sind zurueckgenommen; `git status` ist leer. |
| `.planning/REQUIREMENTS.md` | Status, der den Phasenstand traegt | ✗ REGRESSION | FACT-01, FACT-02, FACT-06 wieder `Complete`, nachdem `2e57ee3` sie zurueckgenommen hat. gaps[1]. |
| `.planning/ROADMAP.md`, `.planning/STATE.md` | Fortschrittsangaben, die einander nicht widersprechen | ⚠️ PARTIAL | `ROADMAP.md:81` traegt zwei einander widersprechende Zahlen in einem Satz; `STATE.md` `## Current Position` widerspricht seinem eigenen Frontmatter. Werkzeugausgabe ueber handgeschriebenem Text. gaps[1]. |

## 4. Key link verification

| From | To | Via | Status | Details |
|---|---|---|---|---|
| `internal/imagefactory/guard_drift_test.go` | `web/src/routes/images.tsx` | der Go-Test liest die **Deklaration** `REFUSED_RANGES` und nicht die Datei | ⚠️ PARTIAL | Er bindet jetzt an den Bezeichner (gemessen: Praefix-Umbenennung am echten Baum ist ROT) und liest nur den herausgeschnittenen Rumpf. Was er nicht kann, ist lexikalischer Kontext: eine Deklaration in einem Blockkommentar oder Template-Literal erfuellt den Anker. |
| `internal/imagefactory/guard_drift_test.go` | `internal/httpapi/handlers/budget_drift_test.go` | die uebernommene Verankerungs-Disziplin | ✓ WIRED | Das Praezedenzmuster `^\s*(?:export\s+)?const\s+NAME\s*=` fordert dort das `=` unmittelbar nach dem Namen — genau die Stelle, an der die Uebernahme in Runde 5 noch aufgeweicht war und die diese Runde nachgezogen hat (`(?::[^=\n]*)?=`, wobei das erzwungene `:` oder `=` die Wortgrenze ist). |
| `internal/imagefactory/guard_drift_test.go` | `internal/imagefactory` (`NotRepresentableReason`) | der Codepoint-Sweep vergleicht Verhalten und nicht zwei Deklarationen | ✓ WIRED | Neu gemessen: U+FEFF-Eintrag geloescht -> `--- FAIL` mit dem benannten Codepoint. Der Sweep laeuft weiterhin und misst weiterhin. |
| `internal/httpapi/handlers/schematics.go:458` | `refreshTheStoredVerdict` (:560-582) | der Doc-Verweis nennt die Zahl der Bedingungen der Funktion, auf die er zeigt | ✓ WIRED | `three` gegen drei `if`-Bedingungen (`fresh.ProbedAt.IsZero()`, `stored.Arch != fresh.Arch`, `stored.TalosVersion != fresh.TalosVersion`). Sweep selbst gefahren, null Resttreffer. |
| `.planning/WINDOWS.md` Eintrag 70 | `internal/imagefactory/guard_drift_test.go` | die Schliessbedingung des Eintrags ist am Code ausfuehrbar | ✓ WIRED | Woertlich ausgefuehrt: Praefix-Umbenennung -> `TestBrowserRefusalSetEqualsTheServers` ROT; Grenze oberhalb `utf8.MaxRune` -> Waechter ROT. Beide Richtungen erfuellt. (Der Eintrag steht dennoch `open` — korrekt, solange gaps[0] laeuft.) |
| `02-27-SUMMARY.md` / `02-28-SUMMARY.md` (`requirements-completed`) | `.planning/REQUIREMENTS.md` | die Werkzeugkette schreibt den Requirement-Status aus der SUMMARY-Frontmatter fort | ⚠️ PARTIAL | Die Verdrahtung funktioniert — und schreibt genau die Aenderung zurueck, die `2e57ee3` von Hand entfernt hat. gaps[1]. |

## 5. Behavioural spot-checks

| Behaviour | Command | Result | Status |
|---|---|---|---|
| Runde 5s Falsifikation, am echten Baum wiederholt | `REFUSED_RANGES` -> `REFUSED_RANGES_LEGACY` in `images.tsx`, `go test -run TestBrowserRefusal` | `--- FAIL: TestBrowserRefusalSetEqualsTheServers` und `--- FAIL: .../the_real_route` (Runde 5: zweimal PASS) | ✓ PASS — die Eigenschaft, die Runde 5 als fehlend gemessen hat, haelt jetzt |
| Fail-first des Ankers | Anker auf die Runde-5-Form `\s*[^=\n]*=` zurueckgedreht | Tabellenzeile `the declaration renamed to a prefixed name` ROT: `no error; read 2 ranges instead` | ✓ PASS — die Zeile beweist, was sie behauptet |
| Fail-first der Bound-Validierung | `if to > utf8.MaxRune \|\| from > to` durch `if false` ersetzt | alle drei Zeilen ROT: `[{0 -1}]`, `[{97 -1}]`, `[{31 0}]` — exakt Runde 5s Werte | ✓ PASS |
| Fail-first der Diagnose-Trennung | `if len(matches) == 0 && whole > 0` durch `if false` ersetzt | Zeile `the body cut short by a bracket in a comment` ROT | ✓ PASS |
| Lexikalischer Kontext (Review WR-01) | eigene Sonde: `REFUSED_RANGES` nur in `/* ... */` bzw. in einem Template-Literal | `ranges=[{0 31} {65279 65279}] err=<nil>` / `ranges=[{0 31}] err=<nil>` | ✗ FAIL — Zustimmung ueber eine Tabelle, die der Browser nicht ausfuehrt |
| Gegenprobe zum Vorigen | `//`-auskommentierte Deklaration; Deklaration ganz weg mit `//`-Rest | beide `err != nil` mit der Praefix-Meldung | ✓ PASS — der Zeilenanfangs-Anker faengt die `//`-Form korrekt ab |
| Spiegel-Defekt der Leer-Diagnose (Review WR-02) | wirklich leeres `= []` plus eine zweite eintragsfoermige Tabelle | `cut short before its first entry, but 1 entries are present … Look for that bracket` | ✗ FAIL — falsche Ursache, nach einer Klammer, die es nicht gibt (fail closed) |
| Gegenprobe zum Vorigen | wirklich leeres `= []` allein | `REFUSED_RANGES is declared but carries no entries this guard can read` | ✓ PASS — ohne die zweite Tabelle greift die richtige Meldung |
| Der Waechter misst weiterhin etwas | `{ from: 0xfeff, to: 0xfeff }` geloescht, Test gefahren | `--- FAIL`, `U+FEFF` benannt | ✓ PASS |
| Zahlwort-Sweep (02-28 Sweep A) | `grep -rn 'refreshTheStoredVerdict'` ueber `internal`/`docs`/`web`/`cmd`, gezaehlt auf `\btwo\b` | 0 | ✓ PASS |
| Entscheidungsdokument unberuehrt | `head -c 9137 … \| shasum -a 256` | `ed2d0874…ee77c9`, byteweise wie Runde 5 | ✓ PASS |
| Ledger-Selbstkonsistenz | unabhaengiger Parse von Frontmatter, Tabelle und JSON, Status **pro Id** verglichen | 55/0/16/71 dreifach, 0 Abweichungen, ids 1..71 | ✓ PASS |
| Kein Produktivaufrufer der Naht | `grep` ueber `internal/httpapi` und `cmd/` nach den sechs Konstruktoren | ein Treffer, ein Kommentar (`router.go:100`) | ✓ PASS (bestaetigt die Verschiebung nach Phase 3) |
| Gates, selbst gefahren | `go build ./...`, `go vet ./...`, `gofmt -l`, `go test ./... -count=1` | Exit 0 / Exit 0 / leer / Exit 0 ueber 15 Testpakete | ✓ PASS |

*Kein `vitest`-Lauf in dieser Runde durchgefuehrt: der Diff beruehrt keine Datei unter `web/`. Der
Orchestrator meldet 9 Dateien / 137 Tests, Exit 0; von nichts hier Gemessenem widersprochen.*

## 6. Requirements coverage

| Requirement | Source plans | Status | Evidence |
|---|---|---|---|
| FOUND-12 | 02-07 | ✓ SATISFIED | `internal/talos/dryrun.go` + Tests; `main.go:171` Kompositionswurzel. Ausserhalb des Runde-6-Diffs. |
| TRANS-01 | 02-01 | ✓ SATISFIED | Echter Machinery-Client ueber die `Dialer`-Naht mit echtem mTLS. |
| TRANS-02 | 02-01 | ✓ SATISFIED | `Dialer` und `DiscoverySource` mit je zweiter Implementierung. |
| TRANS-03 | 02-05 | ✓ SATISFIED | Getrennte Typen; `TestMaintenanceClientMethodSetIsClosed` neu aufgezaehlt. |
| TRANS-04 | 02-05 | ✓ SATISFIED | `TestRequireDeadline`, `TestRetryAllowlistIsExactlyTheFastReadClass` neu aufgezaehlt. |
| TRANS-05 | 02-05 | ⚠️ PARTIAL | Transport-Haelfte belegt; UI-Haelfte ohne Aufrufer — Phase 3. |
| TRANS-06 🚫 | 02-01, 02-08 | ✓ SATISFIED | `internal/talossim` mit In-Memory-COSI, drei Streams, Method-Drift-Guard. |
| TRANS-07 | 02-03 | ✓ SATISFIED | Neun Szenarien deklariert (`scenario.go:48-57`), neun `TestScenario*` plus `TestDocumentedScenariosMatchRegistry` und `TestScenarioContract` neu aufgezaehlt. |
| TRANS-08 | — | ⏭ DEFERRED | Phase 3 (REQUIREMENTS.md, ROADMAP Phase 3 SC 5). |
| FACT-01 | 02-02, 02-06, 02-26 | ✓ SATISFIED | Versions-skopierter Katalog, kein Freitextfeld. Der Drift-Waechter ueber die Browser-Ablehnungsmenge misst heute korrekt (U+FEFF neu gemessen); seine kuenftige Zuverlaessigkeit ist gaps[0] und keine Aussage ueber die heutige Menge. **Die Statuszeile in REQUIREMENTS.md steht dennoch zu Unrecht auf `Complete` — siehe gaps[1].** |
| FACT-02 | 02-02, 02-22, 02-24, 02-25 | ✓ SATISFIED | Vorvalidierung der Extension-Namen; "gilt erst als brauchbar, nachdem ein Model-Build-Probe es bestaetigt hat" gilt auch versionsuebergreifend (Runde 5 fail-first belegt, in dieser Runde unberuehrt). Statuszeile: siehe gaps[1]. |
| FACT-03 | 02-04, 02-09, 02-23 | ✓ SATISFIED | Exakte ISO-/Installer-/PXE-URLs, versionsaufgeloester Repo-Name, Architektur als Parameter. Ausserhalb des Runde-6-Diffs. |
| FACT-04 | 02-04, 02-06 | ✓ SATISFIED | installer/initramfs-Warnung emittiert und in der UI gespiegelt. |
| FACT-05 | 02-04 | ✓ SATISFIED | Pre-Release strukturell gefiltert; kaputte Versionen kuratiert. |
| FACT-06 | 02-02, 02-14, 02-24, 02-26, 02-27 | ✓ SATISFIED | Id lokal vorberechnet und persistiert; `Canonical()` gegen die echte Factory von einem externen Orakel gemessen. Statuszeile: siehe gaps[1]. |

**Orphaned requirements:** keine. Alle vierzehn auf Phase 2 abgebildeten IDs erscheinen in mindestens
einem `requirements`-Frontmatter; die Plan-Frontmatter nennen daneben nur FOUND-11, das auf Phase 1
abgebildet ist. REQUIREMENTS.md bildet keine weitere ID auf Phase 2 ab.

## 7. Anti-patterns

| File | Line | Pattern | Severity | Impact |
|---|---|---|---|---|
| — | — | `TBD` / `FIXME` / `XXX` | — | **Keine** in den beiden von dieser Runde geaenderten Dateien (einzeln geprueft). |
| — | — | `TODO` / `HACK` / `PLACEHOLDER` | — | **Keine** in den beiden geaenderten Dateien. |
| `internal/imagefactory/guard_drift_test.go` | 108-110 | ein Anker ohne lexikalischen Kontext: Blockkommentar und Template-Literal erfuellen ihn | 🛑 Blocker | gaps[0]. Selbst gemessen, nicht uebernommen. |
| `internal/imagefactory/guard_drift_test.go` | 300, 307-315 | `whole` ueber die ganze Quelle: die Abschneide-Meldung greift fuer eine wirklich leere Deklaration | 🛑 Blocker | gaps[0]. **In dieser Runde neu entstanden** — ein Tausch der falschen Ursache, kein Wegfall. |
| `.planning/REQUIREMENTS.md` | FACT-01/02/06 | ein `Complete`, das der Betreiber am selben Tag zurueckgenommen hat | 🛑 Blocker | gaps[1]. `2e57ee3` sagt die Regel in seiner Commit-Nachricht aus. |
| `internal/imagefactory/guard_drift_test.go` | 462-491 | drei Tabellenzeilen pruefen `wantErr: "REFUSED_RANGES"` — eine Zeichenkette, die sechs der sieben Fehlermeldungen tragen | ⚠️ Warning | 02-REVIEW WR-04, von mir abgeschwaecht: die Zeilen tragen die Eigenschaft (der `err == nil`-Zweig greift, gemessen), pinnen aber die Diagnose nicht. Dieselbe Regel, die `02-27-SUMMARY.md` unter `key-decisions` fuer die Bound-Meldung selbst aufstellt. |
| `internal/imagefactory/guard_drift_test.go` | 370-379 | die Bound-Meldung erklaert die invertierte Range mit der Ursache der nicht darstellbaren | ⚠️ Warning | 02-REVIEW WR-03, reproduziert. Der erste Satz deckt beide Faelle, der erklaerende zweite nur einen. |
| `internal/imagefactory/guard_drift_test.go` | 57-105 | der 40-zeilige Doc-Kommentar von `refusedRangesDecl` haengt ohne Leerzeile am Kommentar von `refusedRangesEntry`; `refusedRangesDecl` (:108) hat gar keinen | ⚠️ Warning | 02-REVIEW WR-05, in Runde 5 schon gemeldet und **in dieser Runde um 25 Zeilen gewachsen**. Die Begruendung der Anker-Entscheidung — der inhaltliche Kern der Runde — steht godoc-seitig unter dem falschen Symbol. |
| `.planning/ROADMAP.md` | 81 | `28/28 plans executed (26/28 ausgefuehrt) … — 26/28 ausgefuehrt, 02-27 und 02-28 geplant` | ⚠️ Warning | gaps[1]. Werkzeugpraefix ueber handgeschriebenem Text; zwei einander widersprechende Zahlen in einem Satz. |
| `.planning/STATE.md` | ## Current Position | `Plan: 3 of 28`, `Status: Ready to execute` gegen `completed_plans: 34` im eigenen Frontmatter | ⚠️ Warning | gaps[1]. `/gsd-progress` und die Resume-Pfade lesen genau diesen Abschnitt. |
| `internal/httpapi/handlers/schematics.go` | 628-632 | `versionMismatchReason` ohne Leerwert-Zweig | ⚠️ Warning | 02-REVIEW Runde 5 WR-06, unveraendert weitergetragen; diese Runde hat die Datei nur um ein Zahlwort angefasst. |
| `internal/imagefactory/guard_drift_test.go` | 119-124 | der Surrogate-Bereich wird nur an seinen beiden Endpunkten geprueft | ⚠️ Warning | Vorbestehend, unveraendert weitergetragen. |
| `web/src/api.ts` / `web/src/api.test.ts` | 475 / 99 | `REQUEST_CEILING_MS = 150_000` gegen `toBeGreaterThan(130_000)` gegen `writeTimeout = 130s` | ⚠️ Warning | Bei `2211c80` neu gemessen, drei Handtranskriptionen ohne Waechter. Siehe coincidental_reliance_items. |
| `internal/imagefactory/guard_drift_test.go` | 308-314, 330-335 | zwei Fundorte fuer `%d entries`; die Begruendung fuer `ParseUint(..., 16, 32)` gilt nicht ueber acht Hexziffern; mehrzeilige Annotation und `let` werden als "keine Deklaration" gemeldet; `0x10FFFF` vs. `utf8.MaxRune` | ℹ️ Info | 02-REVIEW IN-01..IN-04. |
| `.planning/WINDOWS.md` | Eintrag 70 | verweist auf `guard_drift_test.go:83`, eine Zeilennummer von vor 02-27 | ℹ️ Info | 02-REVIEW IN-05, bestaetigt: :83 liegt heute im Doc-Kommentar, der Anker steht bei :108-110. |

**Test quality audit.** Kein uebersprungener oder deaktivierter Test ist der alleinige Beleg fuer
eine Anforderung: `grep -n 't.Skip'` ueber die beiden geaenderten Dateien findet nichts; die einzigen
Skips der Phase bleiben die Opt-in-Live-Guards (`live_test.go`, `canonical_live_test.go`), beide im
Ledger (5/64 und 35). Die Assertion-Staerke der drei neuen Tabellen ist Verhaltensebene: sie
scheitern zuerst an `err == nil` und pruefen dann inhaltliche Teilzeichenketten des Fehlertexts. Zwei
Beobachtungen tragen diesen Abschnitt. **(a) Die Fail-first-Reihenfolge ist echt und nicht behauptet
— ich habe sie fuer alle drei Korrekturen selbst hergestellt**, indem ich den jeweiligen Code
ausgebaut habe, und jedes Mal genau die zugehoerige Zeile rot gesehen. Das ist die Disziplin, deren
Fehlen in Runde 4 eine Regression ausgeliefert hat. **(b) Die drei Tabellen bleiben getrennt**, mit
der ausgeschriebenen Begruendung, dass die Akzeptanz jeder Tabelle die Zahl ihrer Zeilen pinnt und
diese Zahl nur haelt, solange die Tabellen getrennt bleiben — das ist derselbe Defekttyp wie ein
Waechter, der nicht findet, was er bewacht, eine Ebene hoeher, und er ist hier vorweggenommen worden.
Die Schwaeche ist die Diagnose-Pinnung dreier Zeilen (`wantErr: "REFUSED_RANGES"`), nicht die
Eigenschaftspruefung. Provenienz der Erwartungswerte fuer den FACT-06-Differential bleibt extern und
nur extern.

## 8. Decision coverage

`02-CONTEXT.md` deklariert D-01 … D-10. Neun sind in mindestens einer SUMMARY benannt. **D-07**
(`talossim` lebt in `internal/talossim`, Modulgrenze erweitert) ist in keiner SUMMARY benannt, wird
aber im Code eingehalten und erzwungen: `internal/depguard_test.go:30` deklariert
`simulatorPackage = rootModule + "/internal/talossim"` und :257 pinnt es in der Grenztabelle; das
Paket `internal` ist im vollen Lauf gruen. Bei `2211c80` erneut geprueft und unveraendert. Nicht
blockierend; nur zur Drift-Erkennung notiert.

## 9. Human verification required

### 1. Welchen Schliessungsweg der Waechter-Rest nimmt

**Test:** Entscheiden zwischen (a) den lexikalischen Kontext beseitigen — Blockkommentare und
Template-Literale vor dem Matchen ausblenden, plus zwei Tabellenzeilen — und (b) die Behauptung des
Waechters ehrlich auf das verengen, was ein regulaerer Ausdruck leisten kann, den Rest als eigenen
offenen Ledger-Eintrag fuehren und die Ausnahme hier per `overrides:` ratifizieren.
**Expected:** Eine der beiden Optionen steht in `.planning/WINDOWS.md` bzw. in den `overrides:`.
**Why human:** Eine Kosten-Nutzen-Entscheidung. Drei Runden haben je ein engeres Loch in derselben
allquantifizierten Eigenschaft gefunden; ein regulaerer Ausdruck ueber TypeScript-Quelltext hat einen
Rest, der nicht auf null geht. **Option (b) ist ein vollwertiger Schliessungsweg und keine Ausrede** —
sie verlangt nur, dass die Verengung dort steht, wo sie gelesen wird.

### 2. Ob FACT-01, FACT-02 und FACT-06 `Complete` bleiben duerfen

**Test:** Entscheiden, ob ein einzelnes Requirement `Complete` werden darf, waehrend die Phase
`gaps_found` traegt.
**Expected:** Entweder zurueckgenommen wie in `2e57ee3`, oder die Regel wird ausdruecklich geaendert
und `2e57ee3` als ueberholt vermerkt.
**Why human:** Der Betreiber hat die Regel am 2026-09-04 um 13:24 durch eine Handlung ausgesprochen
und die Werkzeugkette hat sie um 19:41 ueberschrieben. Welche gilt, ist eine Entscheidung.

### 3. Das zusammengebaute Binary durch einen Browser gegen die echte Image Factory fahren

**Test:** 02-UAT.md Test 5, Unterpruefungen (a)-(d).
**Expected:** Die Route verhaelt sich, wie die Suiten es vorhersagen, mit echter Latenz und echtem Bundle.
**Why human:** 02-UAT.md Test 5 traegt weiterhin `result: issue`; das Browser-Projekt sagt in seinem
eigenen Doc-Kommentar, dass kein Binary laeuft und kein Bundle ausgeliefert wird.

### 4. Bestaetigen, dass der Browser-Install-Schritt der CI auf ubuntu-latest laeuft

**Test:** ein CI-Lauf, der `.github/workflows/ci.yml:71` erreicht.
**Expected:** `playwright install --with-deps chromium` gelingt und das Browser-Projekt laeuft.
**Why human:** WINDOWS-Eintrag 44 ist weiterhin offen. Es scheitert geschlossen, das Risiko ist ein
roter Lauf und kein stiller Pass.

### 5. Eine kalte Installer-Aufloesung gegen factory.talos.dev neu messen

**Test:** eine kalte Assets-Anfrage gegen die echte Factory nach dem Fan-out aus 02-23.
**Expected:** Die kalten Kosten sind der langsamste einzelne Kandidat, nicht die Summe.
**Why human:** 02-24-SUMMARY.md:232 und WINDOWS-Eintrag 57 sagen selbst, dass die Verbesserung
offline gemessen und live unmessen ist.

### 6. Entscheiden, wo die Nicht-Gap-Befunde der Code-Reviews leben

**Test:** Aus Runde 6 WR-03, WR-05 und IN-01..IN-05 triagieren — dazu die aus Runde 5 unveraendert
anstehenden WR-06, den Surrogate-Punkttest, IN-02..IN-07 und die vier Warnungen der Vorvorrunde
(SecureBoot-`ParseBool`, Installer-Provenienz, ungewachte Ceiling-Transkription,
`allowedHosts`/`ssoOnly`).
**Expected:** Jeder Punkt steht in `.planning/WINDOWS.md` oder ist ausdruecklich abgelehnt.
**Why human:** Eine Policy-Entscheidung. Geprueft: keiner dieser Punkte steht in irgendeiner der drei
Repraesentationen des Registers, und `workflow.windows_enforce` liest es beim Ship. Die Liste ist
gegenueber Runde 5 wieder laenger geworden — WR-05 ist zudem in dieser Runde um 25 Zeilen gewachsen.

### 7. `⚠️ PRESENT_BEHAVIOR_UNVERIFIED` — die UI-Haelfte von SC 3

Siehe behavior_unverified_items. Erst nach Phase 3 ausuebbar; die Transport-Haelfte ist belegt.

## 10. Gaps summary

**Runde 6 hat geliefert, was bestellt war — vollstaendig, und mit einer Disziplin, die diese Phase
sich in vier Runden erarbeitet hat.**

Alle drei Gaps der Runde 5 sind geschlossen, und ich habe keinen davon geglaubt. Fuer G-02-25 habe
ich jede Korrektur einzeln ausgebaut und die zugehoerige Tabellenzeile rot werden sehen, und danach
Runde 5s Experiment am echten Baum woertlich wiederholt: dort, wo Runde 5 zweimal `PASS` gemessen
hat, steht heute zweimal `FAIL`. G-02-26 ist mit einem Ledger-Eintrag geschlossen, der besser ist als
das, was er ersetzt — er ueberholt statt zu bearbeiten, trennt was gilt von was nicht gilt, gibt sich
eine **pruefbare** Schliessbedingung und behauptet ausdruecklich nicht, dass die Wahrheit gilt. Ich
habe diese Bedingung woertlich ausgefuehrt; sie ist in beiden Richtungen erfuellt. Und
`02-27-SUMMARY.md` sagt an zwei Stellen von sich aus, dass die Feststellung nicht ihm gehoert. Das
ist die Lehre aus Eintrag 69, prospektiv angewendet, und es ist der Grund, warum diese Runde
qualitativ besser ist als die davor.

**Trotzdem bleibt ein Loch, und es ist ehrlicher, das zu sagen, als es wegzurunden.** Der Anker ist
ein regulaerer Ausdruck ueber Quelltext und kennt keinen lexikalischen Kontext. Selbst gemessen: eine
`REFUSED_RANGES`-Deklaration, die nur in einem `/* ... */`-Block oder einem Template-Literal
ueberlebt, wird gelesen und der Waechter meldet Zustimmung ueber eine Tabelle, die der Browser nicht
ausfuehrt. Ich habe den Fall enger gemacht, als der Code-Review ihn fuehrt — die `//`-Form scheitert
korrekt, und der Schadensfall verlangt drei Konjunktionen —, aber es sind genau so viele
Konjunktionen wie beim `_LEGACY`-Fall, den Runde 5 als blockierend gefuehrt hat. Dieselbe Elle,
dasselbe Urteil. Dazu, in dieser Runde neu entstanden: die Diagnose-Trennung hat eine falsche Ursache
gegen eine andere getauscht — `whole` zaehlt ueber die ganze Quelle, also bekommt eine wirklich leere
Deklaration neben einer zweiten Tabelle jetzt "Look for that bracket" fuer eine Klammer, die es nicht
gibt.

**Und hier gehoert eine Feststellung hin, die keine der fuenf Vorrunden gemacht hat.** Drei Runden
haben je ein engeres Loch in derselben allquantifizierten Eigenschaft gefunden. Das ist kein Zufall
und kein Versagen der ausfuehrenden Plaene — es ist die Eigenschaft eines regulaeren Ausdrucks ueber
TypeScript-Quelltext. Deshalb steht unter `missing` ausdruecklich ein **zweiter** Schliessungsweg:
die Behauptung des Waechters auf das verengen, was er messen kann, den Rest als offenen
Ledger-Eintrag fuehren und die Ausnahme per `overrides:` ratifizieren. Das ist kein Nachgeben,
sondern genau die Operation, die diese Phase seit Runde 3 von jedem anderen Artefakt verlangt: nicht
mehr behaupten, als gemessen wurde. Welcher Weg genommen wird, gehoert dem Betreiber und steht als
Human-Verification-Punkt 1.

**Der zweite Gap ist ein Rueckfall in der Buchhaltung, und er ist unangenehm, weil er einer
ausdruecklichen Handlung des Betreibers widerspricht.** `.planning/REQUIREMENTS.md` markiert FACT-01,
FACT-02 und FACT-06 wieder als `Complete` — dieselben drei Zeilen, die derselbe Tag um 13:24 mit
`2e57ee3 docs(phase-02): revert premature Complete requirements after gaps found` zurueckgenommen
hat. Getrieben wird es von `requirements-completed` in beiden neuen SUMMARYs, und darin liegt die
eigentliche Ironie: derselbe Plan, dessen SUMMARY FACT-06 als abgeschlossen meldet, sagt zwei
Absaetze weiter ausdruecklich, dass er G-02-25 **nicht** als geschlossen meldet — und G-02-25 ist die
Zuverlaessigkeit genau des Drift-Waechters, an dem FACT-06 haengt. Die Disziplin haelt an der
Stelle, die der Plan bewusst adressiert, und faellt an der aus, die eine Werkzeugkette schreibt.
Sachlich sind die drei Requirements erfuellt; falsch ist der Statuswechsel gegen eine dokumentierte
Entscheidung. Zwei kleinere Geschwister im selben Diff — `ROADMAP.md:81` mit zwei einander
widersprechenden Zahlen in einem Satz und `STATE.md` mit `Plan: 3 of 28` gegen sein eigenes
`completed_plans: 34` — haben dieselbe Wurzel.

Keiner der beiden Gaps beruehrt die Transport-Naht, `talossim`, die neun Szenarien, `--dry-run` oder
die URL-Herleitung. Success Criteria 1, 2, 4 und 5 halten; 3 haelt auf seiner Transport-Haelfte, mit
der UI-Haelfte korrekt nach Phase 3 verschoben. Der Score steigt von 14/19 auf 16/19, und die zwei
verbleibenden Fehlschlaege liegen in zwei verschiedenen Registern ohne gemeinsame Wurzel — zum ersten
Mal in dieser Phase.

---

_Verified: 2026-09-04T20:05:54Z bei `2211c80`_
_Verifier: Claude (gsd-verifier) — Runde 6_
