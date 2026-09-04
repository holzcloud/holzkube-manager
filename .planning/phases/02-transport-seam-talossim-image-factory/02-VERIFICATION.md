---
phase: 02-transport-seam-talossim-image-factory
verified: 2026-09-04T11:17:28Z
verified_at_commit: d7d07ea
status: gaps_found
score: 14/19 must-haves verified
behavior_unverified: 1
overrides_applied: 0
re_verification:
  round: 5
  previous_status: gaps_found
  previous_score: 15/19
  previous_verified_at_commit: b6e954b
  gaps_closed:
    - "G-02-24 (Runde 4, gaps[0], `G4-1`, 02-REVIEW.md CR-01) — GESCHLOSSEN, fail-first gemessen und nicht erschlossen. `refreshTheStoredVerdict` (schematics.go:579-582) traegt die dritte Bedingung `if stored.TalosVersion != fresh.TalosVersion`. Beweis: die Bedingung entfernt, `TestConflictAtAnotherTalosVersionDeclinesTheRefresh` und `TestConflictAtAnotherTalosVersionErasesNoStoredRefusal` einzeln gefahren — beide ROT, und die Ausgabe reproduziert Runde 4 woertlich (`talos_version: v1.12.0, usable: true, probe_reason: \"\"`, rev 1 -> 2, die wahre v1.12.0-Ablehnung geloescht). Bedingung wieder eingesetzt: alle zehn `TestConflict*` gruen. Der Arbeitsbaum ist danach byteweise identisch mit HEAD."
  gaps_remaining:
    - "G-02-25 (Runde 4, gaps[1], `G4-2`, 02-REVIEW.md WR-04) — VERENGT, NICHT GESCHLOSSEN. Am echten Baum gemessen, nicht vom Code-Review uebernommen: siehe gaps[0] und adversarial_checks_run[0]."
  regressions: []
  adversarial_checks_run:
    - "WR-01 des Code-Reviews REPRODUZIERT und darueber hinaus am echten Baum bestaetigt. Erst die reine Funktion befragt: `parseBrowserRefusalRanges` liefert gegen eine Quelle, die nur `REFUSED_RANGES_LEGACY` enthaelt, `ranges=[{0 0}] err=<nil>`; gegen `REFUSED_RANGESX` `ranges=[{0 1}] err=<nil>`. Dann die Falsifikation aus Runde 4 mit dem Namen wiederholt, den der Waechter nicht abfaengt: `REFUSED_RANGES` -> `REFUSED_RANGES_LEGACY` durchgaengig in `web/src/routes/images.tsx`, `go test -run TestBrowserRefusal -v` -> `--- PASS: TestBrowserRefusalSetEqualsTheServers` UND `--- PASS: TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration` (alle vier Untertests). Der Waechter ist gruen gegen eine Quelle, in der `REFUSED_RANGES` nicht vorkommt — dieselbe Eigenschaft, die Runde 4 gemessen hat, nur durch eine engere Tuer. Datei byteweise wiederhergestellt, `git diff` leer."
    - "WR-03 des Code-Reviews REPRODUZIERT: `{ from: 0x0000, to: 0x001f }` plus `{ from: 0xFFFFFFFF, to: 0xFFFFFFFF }` -> `ranges=[{0 31} {-1 -1}] err=<nil>`; `{ from: 0x001f, to: 0x0000 }` -> `ranges=[{31 0}] err=<nil>`; `{ from: 0x10FFFF, to: 0xFFFFFFFF }` -> `ranges=[{1114111 -1}] err=<nil>`. `rune(0xFFFFFFFF)` ist -1, der Sweep laeuft ab `rune(0)`, also deckt ein solcher Eintrag keinen einzigen Codepoint ab und wird still uebergangen."
    - "WR-02 des Code-Reviews REPRODUZIERT: eine vollstaendige, korrekte Deklaration mit `// see the table in RefusedRange[] above` als erster Zeile im Rumpf -> `err=REFUSED_RANGES is declared but carries no entries this guard can read. This is the declaration being empty, not the declaration being absent`. Der Waechter scheitert also (fail closed), aber mit der falschen Diagnose ueber eine Deklaration mit sechs Eintraegen."
    - "Gegenprobe zur Behauptung des Code-Reviews, CR-01 sei strukturell vollstaendig: `ProbeBuildable` (probe.go:23-38) nimmt `(id, talosVersion, arch)` und nagelt `Platform: PlatformMetal` fest, `SecureBoot` bleibt der Nullwert. Jede weitere Achse von `model.Schematic` liegt entweder im kanonischen Dokument (Extensions, KernelArgs, Meta -> gleiche Id -> gleich) oder beruehrt die Probe nicht (Cluster, Name). Der Definitionsbereich des Urteils ist damit genau (id, version, arch), und die drei Vorbedingungen decken ihn ab. Das Argument des Reviews haelt der Nachpruefung stand."
    - "Der U+FEFF-Beleg neu gemessen statt uebernommen: den Eintrag `{ from: 0xfeff, to: 0xfeff }` aus `images.tsx` geloescht -> `--- FAIL: TestBrowserRefusalSetEqualsTheServers`, `1 codepoints the server refuses are accepted ...: U+FEFF`. Datei wiederhergestellt. Heute ist also weiterhin nichts maskiert."
    - "Die Prohibition von Plan 02-25 ueber `02-DECISION-schematic-identity.md` gemessen: `head -c 9137 | shasum -a 256` = `ed2d08742734c2e46977ad4ac3bc3828a7206a7be27cdadb59fda41b26ee77c9`, exakt der im Plan genannte Wert. Der entschiedene Text ist unberuehrt, der Nachtrag ist reines Anhaengen."
    - "`.planning/WINDOWS.md` erneut unabhaengig dreifach geparst: Frontmatter 54 open / 0 waived / 15 fixed / 69 total, Markdown-Tabelle 69 Zeilen mit 54 open + 15 fixed, JSON-Block 69 Eintraege mit denselben Status. Null Status-Abweichungen, ids 1..69 lueckenlos und doppelfrei."
    - "`git diff b6e954b..HEAD --stat` gelesen, bevor irgendein Regressionsurteil gefaellt wurde: genau fuenf Quelldateien haben sich seit Runde 4 geaendert (schematics.go, schematics_test.go, guard_drift_test.go, model.go, api-contract.md). SC 1, 2, 3 und 5 liegen ausserhalb dieses Diffs; ihre Belege wurden per `go test -list` neu aufgezaehlt statt aus Runde 4 abgeschrieben."
gaps:
  - truth: "A guard reports a pass only when it measured the property it is named for — der Browser-Ablehnungs-Waechter scheitert, wenn er `REFUSED_RANGES` nicht findet, und ueberspringt keinen Eintrag still"
    status: partial
    reason: >-
      Plan 02-26 hat den Waechter deutlich verengt und dabei echte Arbeit geleistet: die Pruefung ist
      als reine Funktion `parseBrowserRefusalRanges` herausgeloest, ihre Fehlerfaelle haben eigene
      Tabellentests, ein eintragsfoermiges Literal ausserhalb der Deklaration wird nicht mehr
      eingefaltet, und der Rumpf wird tatsaechlich zuerst herausgeschnitten (heute gemessen: 6
      Treffer im Rumpf, 6 in der ganzen Datei). Zwei Loecher bleiben, beide am echten Baum gemessen.
      **Erstens der Anker.** `refusedRangesDecl` (guard_drift_test.go:83-85) setzt hinter den Namen
      `\s*[^=\n]*`, und `[^=\n]*` frisst die Fortsetzung eines laengeren Identifiers. Es gibt keine
      Wortgrenze und keine Forderung nach `:` oder `=` direkt hinter dem Namen. Gemessen:
      `REFUSED_RANGES_LEGACY` allein -> `ranges=[{0 0}] err=<nil>`, `REFUSED_RANGESX` ->
      `ranges=[{0 1}] err=<nil>`. Und am echten Baum, als Wiederholung der Falsifikation aus Runde 4
      mit dem Namen, den der Waechter nicht abfaengt: `REFUSED_RANGES` durchgaengig in
      `REFUSED_RANGES_LEGACY` umbenannt, `TestBrowserRefusalSetEqualsTheServers` gruen — und
      `TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration` mit allen vier Untertests
      ebenfalls gruen, denn dessen drei Negativzeilen benutzen `RENAMED_BY_VERIFIER`,
      `hasControlCharacter` und `SOME_OTHER_TABLE` — kein Name, der `REFUSED_RANGES` als Praefix
      traegt. Der Satz, den der Waechter in seinem eigenen Fehlertext trifft ("Renamed, moved or
      deleted is the same as never having been there", :240-241), ist damit weiterhin falsch. Der
      stille Schadensfall ist derselbe wie in Runde 4, nur enger: `REFUSED_RANGES` wandert in ein
      anderes Modul, eine praefixierte Resttabelle bleibt in `images.tsx` stehen, der Waechter liest
      die Resttabelle und meldet Gleichheit ueber eine Menge, die die Form nicht mehr benutzt.
      **Zweitens die Bounds.** Der in dieser Runde ergaenzte Unlesbar-Eintrag-Check ist rein
      syntaktisch — er fragt, ob `browserRefusalRange` das Literal matcht — und nicht semantisch.
      `strconv.ParseUint(m[1], 16, 32)` akzeptiert bis `0xFFFFFFFF`, `rune(...)` ist verlustbehaftet,
      und es gibt weder `from <= to` noch `to <= utf8.MaxRune`. Gemessen:
      `{0xFFFFFFFF, 0xFFFFFFFF}` -> `{-1 -1}`, `{0x10FFFF, 0xFFFFFFFF}` -> `{1114111 -1}`,
      `{0x001f, 0x0000}` -> `{31 0}` — jeder dieser Eintraege deckt im Sweep (`for r := rune(0)`)
      keinen einzigen Codepoint ab und wird still ignoriert. Das ist woertlich die Eigenschaft, die
      der neue Check beseitigen sollte, sein eigener Text sagt es (:255-259): "an entry it skips is a
      codepoint it reports agreement about without having compared it". Die gefaehrliche Richtung ist
      die, die der Waechter selbst als die schlimmere bezeichnet: ein `{ from: 0x0061, to:
      0xFFFFFFFF }` laesst die Form im Browser jeden Kleinbuchstaben ablehnen, waehrend Go dort
      `{97 -1}` liest und nichts vergleicht. **Heute ist nichts maskiert** — die beiden Mengen
      stimmen ueberein, und das Loeschen des U+FEFF-Eintrags laesst den Waechter weiterhin scheitern
      (neu gemessen). Es geht, wie schon in Runde 3 und 4, um das kuenftige Verhalten des Waechters,
      und das ist derselbe Massstab, den diese Phase der kanonischen Haelfte angelegt hat.
    artifacts:
      - path: "internal/imagefactory/guard_drift_test.go:83-85"
        issue: "`\\s*[^=\\n]*` hinter dem Bezeichner erlaubt jede Praefix-Erweiterung des Namens; es fehlt die Wortgrenze bzw. die Forderung nach `:` oder `=` direkt hinter dem Namen"
      - path: "internal/imagefactory/guard_drift_test.go:286-299"
        issue: "die gelesenen Bounds werden nicht validiert: kein `to <= utf8.MaxRune`, kein `from <= to`; `rune(uint64)` verliert still"
      - path: "internal/imagefactory/guard_drift_test.go:344-369"
        issue: "alle drei Negativzeilen der Falsifikationstabelle benutzen Namen, die kein Praefix von `REFUSED_RANGES` sind — genau die Umbenennung, die niemand vornimmt, statt der, die jemand vornimmt"
      - path: "internal/imagefactory/guard_drift_test.go:263-269"
        issue: "ein `]` in einem Kommentar vor dem ersten Eintrag schneidet den Rumpf ab; der `len(matches) == 0`-Zweig greift vor der Zaehlpruefung und meldet 'die Deklaration ist leer' ueber eine Deklaration mit sechs Eintraegen (fail closed, aber falsche Diagnose)"
    missing:
      - "Den Anker an den Namen binden statt an sein Praefix: `regexp.QuoteMeta(refusedRangesName) + \\`\\\\s*(?::[^=\\\\n]*)?=\\\\s*\\\\[(.*?)\\\\]\\`` — und eine fuenfte Tabellenzeile, die auf einen praefixierten Namen umbenennt (`REFUSED_RANGES_LEGACY`), weil genau diese Zeile heute fehlt"
      - "Nach dem Parsen validieren, mit derselben Begruendung, die der Unlesbar-Check bereits traegt: `if to > utf8.MaxRune || from > to { return nil, fmt.Errorf(...) }`; `utf8` ist bereits importiert. Plus je eine Tabellenzeile fuer den Ueberlauf und fuer die invertierte Range"
      - "Die Leer-Diagnose nicht behaupten lassen, die Deklaration sei leer, wenn im Gesamt-Source Eintraege gefunden werden — `if len(matches) == 0 && whole > 0` benennt die Ursache (Abschneiden an einem `]`) statt sie zu verschleiern"
      - "Oder, falls die verbleibende Verengung bewusst akzeptiert wird: das gehoert als eigener Eintrag in `.planning/WINDOWS.md` und Eintrag 69 darf dann nicht `fixed` bleiben (siehe gaps[1])"
  - truth: "Ein Abschlussdokument und der Ledger sagen nur, was von der Codebasis wahr ist (T-02-62 / G-02-1-Linie, angewandt auf den Ledger statt auf den Store)"
    status: failed
    reason: >-
      Der Store sagt seit Plan 02-25 die Wahrheit — das ist der geschlossene Teil dieser Runde und in
      truths 4 und 19 belegt. Der Ledger tut es an einer Stelle nicht. `.planning/WINDOWS.md`
      Eintrag 69 ist bei der Anlage als `fixed` markiert und sagt im Text "Plan 02-26 hat es
      geschlossen: `refusedRangesDecl` schneidet die Deklaration zuerst heraus ..., ihr Fehlen ist
      ein Fehler statt eines kuerzeren Ergebnisses". Gemessen ist ihr Fehlen kein Fehler, sobald der
      Name praefixiert weiterlebt (siehe gaps[0]). Dieselbe Behauptung steht in der
      `coverage`-Metadatenzeile D1 von `02-26-SUMMARY.md`: "scheitert, wenn sie fehlt, umbenannt
      oder verschoben ist". Der Unterschied zu einer blossen Ungenauigkeit ist mechanisch:
      `workflow.windows_enforce` liest genau dieses Register beim Ship, und ein `fixed`-Eintrag ist
      dort unsichtbar. Der Ledger fuehrt sonst in dieser Runde vorbildlich Buch — 66 amendiert 58
      ausdruecklich, 67 zeichnet den geschlossenen Defekt mit den Commits auf, 68 sagt ausdruecklich,
      dass eine Pruefung nichts gefunden hat, und alle drei Repraesentationen stimmen ueberein
      (54/0/15/69, unabhaengig geparst). Genau deshalb faellt der eine Eintrag auf, der mehr
      behauptet, als gemessen wurde.
    artifacts:
      - path: ".planning/WINDOWS.md (Eintrag 69)"
        issue: "`status: fixed` und 'Plan 02-26 hat es geschlossen' fuer eine Eigenschaft, die verengt und nicht geschlossen ist"
      - path: ".planning/phases/02-transport-seam-talossim-image-factory/02-26-SUMMARY.md (coverage D1)"
        issue: "'scheitert, wenn sie fehlt, umbenannt oder verschoben ist' — gemessen falsch fuer eine praefixierende Umbenennung"
    missing:
      - "Ein neuer Ledger-Eintrag (es gibt kein Aenderungs-Verb) der in Grossbuchstaben eroeffnet, was er an Eintrag 69 ueberholt, den gemessenen Praefix-Fall und die fehlende Bound-Validierung benennt und `open` bleibt, bis gaps[0] geschlossen ist"
      - "Die Korrektur in `02-26-SUMMARY.md` in der Form, die `02-21-SUMMARY.md:345-358` in dieser Phase bereits etabliert hat: durchgestrichen, als WITHDRAWN markiert, mit datiertem Korrekturblock und der Messung"
  - truth: "Jede Aussage im Baum, die bisher zwei Bedingungen des Refresh nennt, nennt jetzt drei (Plan 02-25, must_haves.truths[5])"
    status: partial
    reason: >-
      Die drei im Plan namentlich aufgezaehlten Stellen sind nachgezogen und wurden geprueft:
      `internal/model/model.go` bei `ID` (:104-119) und bei `TalosVersion` (:133-139) sowie
      `docs/api-contract.md:620-627` mit einer dritten Tabellenzeile und einem erweiterten
      Outcome-Satz. Die Aussage der Wahrheit ist aber allquantifiziert, und eine vierte Stelle steht
      im selben File wie die geaenderte Funktion und zeigt direkt auf sie:
      `internal/httpapi/handlers/schematics.go:457-459` sagt weiterhin "see refreshTheStoredVerdict
      for the two conditions and for why a failed refresh is silent", 54 Zeilen ueber einer Funktion,
      die seit `d4ee1f5` drei prueft. Ein Leser, der dem Verweis folgt, faengt mit einer falschen
      Erwartung an — dieselbe Klasse Aussage, deren Pflege Task 3 von Plan 02-25 zum Gate gemacht
      hat. Klein, eindeutig, ein Wort.
    artifacts:
      - path: "internal/httpapi/handlers/schematics.go:457-459"
        issue: "'the two conditions' verweist auf eine Funktion mit drei Bedingungen"
    missing:
      - "`for the two` -> `for the three` in schematics.go:458"
deferred:
  - truth: "Ein nicht erreichbarer Node blockiert die UI nicht (die UI-Haelfte von ROADMAP SC 3 / TRANS-05)"
    addressed_in: "Phase 3"
    evidence: >-
      Bei `d7d07ea` erneut geprueft. Es gibt weiterhin keinen Produktivaufrufer von
      `NewClusterClient`, `NewMaintenanceClient`, `FanOut`, `NewBreaker`, `NewDirectDialer` oder
      `NewManualSource` ausserhalb von `internal/talos` und dessen Tests. Die Transport-Haelfte ist
      verhaltensbelegt (`TestFanOutOneSilentNodeCostsOneNode`,
      `TestFanOutCancellationTerminatesEveryInFlightCall`,
      `TestFanOutSkipsAnOpenCircuitWithoutDialing` aufgezaehlt und im gruenen Paket). ROADMAP Phase 3
      besitzt die Inventar-Route.
  - truth: "Dieselbe Contract-Suite laeuft auch gegen echtes Talos (TRANS-08)"
    addressed_in: "Phase 3"
    evidence: "REQUIREMENTS.md:226 bildet TRANS-08 auf Phase 3 (Pending) ab; ROADMAP.md Phase 3, Success Criterion 5 und die `Note` sagen, warum. `internal/talos/contract_test.go` parametrisiert den Transport bereits."
  - truth: "G-02-9 — eine Probe, die trotz erhoehtem Budget ausbleibt, ist weiterhin dauerhaft"
    addressed_in: "02-DECISION-probe-budget.md Option 1, bewusst nicht genommen"
    evidence: >-
      Kein Gap, sondern ehrlich verbucht — und diese Runde hat es nicht verwaessert, obwohl sie die
      Minderung angefasst hat. Gemessen: `grep -rn 'G-02-9'` ueber beide SUMMARYs, `.planning/WINDOWS.md`,
      `internal/` und `docs/` findet ausschliesslich Formulierungen, die ihn offen nennen — WINDOWS 58
      ("G-02-9 REMAINS OPEN AFTER ROUND 4", weiterhin `open`), WINDOWS 66 ("AMENDS ENTRY 58, WHICH
      STAYS OPEN ... **G-02-9 ist offen**"), 02-25-SUMMARY.md:251 und :290, 02-26-SUMMARY.md:241,
      :253 und :318, sowie schematics_test.go:3382 ("the whole of why G-02-9 stays open"). Kein
      Artefakt dieser Runde behauptet das Gegenteil.
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
      Unveraendert und bei `d7d07ea` erneut geprueft: es gibt weiterhin keinen Produktivaufrufer der
      Transport-Naht, also hat die UI-Haelfte des Kriteriums keinen Codepfad, den man ausueben
      koennte. Die Transport-Haelfte IST belegt (`TestFanOutOneSilentNodeCostsOneNode`).
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
      Bei `d7d07ea` neu gemessen statt abgeschrieben: `web/src/api.ts:475` `REQUEST_CEILING_MS =
      150_000`, `web/src/api.test.ts:99` `expect(call?.[0]).toBeGreaterThan(130_000)` — eine zweite
      Handtranskription auf der TypeScript-Seite, die sich weder mit der Go-Konstante noch mit
      `REQUEST_CEILING_MS` bewegt. `writeTimeout` auf 160s zu heben liesse beide gruen, waehrend
      jedes lange Create bei 150s abbraeche. Nur beratend, aendert weder Status noch Score.
human_verification:
  - test: >-
      Das zusammengebaute Binary durch einen Browser gegen die echte Image Factory fahren: ein
      Schematic von Anfang bis Ende autoren und bestaetigen, dass die /images-Route ausserhalb eines
      Test-Harness funktioniert (02-UAT.md Test 5, Unterpruefungen a-d).
    expected: "Die Route verhaelt sich, wie die Suiten es vorhersagen, mit echter Latenz und echtem Bundle."
    why_human: >-
      Bei `d7d07ea` weiterhin offen und korrekt so verbucht: 02-UAT.md Test 5 traegt `result: issue`.
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
      Fakes gemessen und gegen factory.talos.dev unmessen. Die zwei Live-Laeufe aus 02-22 haben die
      ISO-Probe gemessen, nicht die Installer-Aufloesung.
  - test: >-
      Entscheiden, wo die Befunde aus 02-REVIEW.md landen, die diese Verifikation nicht als Gap
      fuehrt: WR-05 (Doc-Kommentar am falschen Symbol), WR-06 (`versionMismatchReason` ohne
      Leerwert-Zweig), der vorbestehende Surrogate-Punkttest (Review WR-04) sowie IN-01 bis IN-07 —
      dazu die vier Warnungen der Vorrunde (WR-01 SecureBoot-ParseBool, WR-02 Installer-Provenienz,
      WR-03 ungewachte Ceiling-Transkription, WR-05 allowedHosts/ssoOnly), die in Runde 4 schon
      einmal zur Triage anstanden.
    expected: "Jeder Punkt ist entweder behoben, mit `gsd-tools windows append` als Window abgelegt oder ausdruecklich abgelehnt."
    why_human: >-
      Eine Policy-Entscheidung, keine Messung. Geprueft: keiner dieser Punkte steht in irgendeiner
      Repraesentation von `.planning/WINDOWS.md`, und `workflow.windows_enforce` liest genau dieses
      Register beim Ship. Die Liste ist gegenueber Runde 4 laenger geworden, nicht kuerzer.
---

# Phase 2 Verification — Runde 5 (die beiden Gap-Closure-Plaene 02-25 und 02-26)

**Phase Goal:** Jede Talos-Interaktion laeuft durch eine austauschbare Naht und ist ohne Hardware
testbar; Schematics und Image-URLs sind korrekt und nachweislich brauchbar herleitbar.
**Verified:** 2026-09-04T11:17:28Z bei `d7d07ea` (Branch `main`, Arbeitsbaum sauber; alle
Falsifikationen wurden byteweise zurueckgenommen und `git diff` ist am Ende leer)
**Status:** gaps_found — ein Gap geschlossen, eines verengt statt geschlossen, zwei kleine neu
**Re-verification:** Ja — Runde 5 ueber die Plaene 02-25 und 02-26, den Runde-4-Bericht bei
`b6e954b` ueberschreibend.

Runde 5 hatte zwei Auftraege. **Der erste ist erledigt, und zwar sauber.** G-02-24 — der
versionsuebergreifende Refresh, den Runde 4 als Regression reproduziert hat — ist geschlossen, und
ich habe es nicht geglaubt, sondern die Bedingung entfernt und zugesehen, wie die beiden neuen Tests
rot werden und dabei Runde 4s Ausgabe woertlich reproduzieren. **Der zweite ist zur Haelfte
erledigt.** G-02-25 ist deutlich verengt: der Waechter schneidet die Deklaration jetzt wirklich
heraus, die Pruefung ist pur und ihre Fehlerfaelle haben eigene Tests. Aber die Eigenschaft, um die
es geht, ist weiterhin falsifizierbar — ich habe Runde 4s Experiment mit einem praefixierten Namen
wiederholt und den Waechter wieder gruen gegen eine Quelle ohne `REFUSED_RANGES` gesehen.

Der Code-Review, der unmittelbar vor dieser Verifikation lief, hat beides richtig gesehen. Ich habe
seine beiden zentralen Behauptungen nicht uebernommen, sondern nachgemessen: CR-01 haelt auch der
Gegenprobe stand (der Definitionsbereich des Urteils ist wirklich (id, version, arch) und nichts
sonst), und WR-01 ist am echten Baum reproduzierbar, nicht nur an einer synthetischen Quelle.

Nichts in diesem Bericht ist aus Runde 4 uebernommen, ohne bei `d7d07ea` neu gemessen worden zu sein.

## 1. Observable truths

| # | Truth | Status | Evidence bei `d7d07ea` |
|---|---|---|---|
| 1 | SC 1 — der **unveraenderte** Produktions-Client spricht gegen `talossim`: echte Protobufs, echtes mTLS, echter In-Memory-COSI-State, ohne Hardware, ohne `talosctl`, ohne Netz | ✓ VERIFIED | Ausserhalb des Runde-5-Diffs (`git diff b6e954b..HEAD --stat`: fuenf Quelldateien, keine davon in `internal/talossim`). Neu aufgezaehlt statt abgeschrieben: `go test ./internal/talossim/ -list '.*'` fuehrt `TestTracerRealClientReachesFakeNode` und `TestDryRunApplyChangesNothing`. `internal/depguard_test.go:30,257` pinnt die Modulgrenze aus D-07 unveraendert. |
| 2 | SC 2 — der Betreiber schaltet neun Fehlerszenarien und der Client verhaelt sich definiert | ✓ VERIFIED | `internal/talossim/scenario.go:48-57` deklariert alle neun namentlich (`go_silent`, `reject_apply`, `second_bootstrap_returns_AlreadyExists`, `flap_connection`, `slow_log_consumer`, `ip_changes_on_reboot`, `etcd_down`, `k8s_down`, `version_out_of_supported_range`); `TestScenarioContract` und `TestGoSilentFailsAtItsOwnClassDeadline` aufgezaehlt und im gruenen Paket. Von Runde 5 nicht beruehrt. |
| 3 | SC 3 — ein unerreichbarer Node blockiert weder die UI noch andere Nodes; jeder Aufruf hat ein erzwungenes Deadline; Retries nur fuer eine Lese-Allowlist; Cluster- und Maintenance-Clients sind getrennte Typen | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Die Transport-Haelfte ist verhaltensbelegt und wurde neu aufgezaehlt: `TestFanOutOneSilentNodeCostsOneNode`, `TestRequireDeadline`, `TestRetryAllowlistIsExactlyTheFastReadClass`, `TestMaintenanceClientMethodSetIsClosed`. Die **UI**-Haelfte hat weiterhin keinen Codepfad: kein Produktivaufrufer der Naht ausserhalb `internal/talos` und dessen Tests. Nach Phase 3 verschoben; siehe behavior_unverified_items. |
| 4 | SC 4 — der Betreiber stellt ein Schematic aus einem versions-skopierten Katalog zusammen, bekommt die exakten ISO-/Installer-/PXE-URLs (versionsabhaengiger Repo-Name, keine hartkodierte Architektur), **und das Schematic gilt erst nach einem bestaetigenden Model-Build-Probe als brauchbar**; Kernel-Args oder META loesen die installer/initramfs-Warnung aus | ✓ VERIFIED | Der Konjunkt, der in Runde 4 als einziger fiel, ist wiederhergestellt und fail-first belegt: dritte Bedingung `if stored.TalosVersion != fresh.TalosVersion` (schematics.go:579-582). Bedingung entfernt -> beide neuen Tests ROT mit Runde 4s Ausgabe (`talos_version: v1.12.0, usable: true, probe_reason: ""`, rev 1->2); Bedingung zurueck -> 10/10 `TestConflict*` gruen. Die uebrigen Konjunkte liegen ausserhalb des Runde-5-Diffs und wurden in Runde 4 belegt. |
| 5 | SC 5 — das ganze Binary laeuft mit `--dry-run` und keine Mutation erreicht einen Node | ✓ VERIFIED | Ausserhalb des Runde-5-Diffs. `TestDryRunRefusesEveryMutationAtTheNode` aufgezaehlt und gruen; Kompositionswurzel `cmd/holzkube-managerd/main.go:171` `talos.Mode{DryRun: cfg.DryRun}` unveraendert. |
| 6 | R3/R4-Uebertrag — ein Waechter meldet einen Pass nur, wenn er die Eigenschaft gemessen hat, nach der er benannt ist | ✗ FAILED (partial) | Deutlich verengt und weiterhin falsifizierbar. Am echten Baum gemessen: `REFUSED_RANGES` -> `REFUSED_RANGES_LEGACY`, `TestBrowserRefusalSetEqualsTheServers` **gruen**, und der neue Falsifikationstest mit allen vier Untertests ebenfalls gruen. Dazu die still uebergangenen Bounds (`{0xFFFFFFFF, 0xFFFFFFFF}` -> `{-1 -1}`, `err=<nil>`). Siehe gaps[0]. |
| 7 | R3-Uebertrag — ein blockierender Human-Decision-Checkpoint, der ohne Menschen aufgeloest wurde, sagt das dort, wo die Entscheidung gelesen wird | ✓ VERIFIED | `02-DECISION-schematic-identity.md:130-148` unveraendert ("This decision is therefore self-resolved, not ratified."). Gemessen, dass Plan 02-25 sie nicht angetastet hat: `head -c 9137 \| shasum -a 256` = `ed2d0874…ee77c9`, exakt der in der Prohibition genannte Wert; der Nachtrag ist reines Anhaengen. |
| 8 | Ein Abschlussdokument und der Ledger sagen nur, was von der Codebasis wahr ist | ✗ FAILED | WINDOWS-Eintrag 69 ist bei der Anlage `fixed` und sagt "Plan 02-26 hat es geschlossen … ihr Fehlen ist ein Fehler"; `02-26-SUMMARY.md` coverage D1 sagt "scheitert, wenn sie fehlt, umbenannt oder verschoben ist". Beides ist fuer eine praefixierende Umbenennung gemessen falsch. Siehe gaps[1]. |
| 9 | 02-25 — die dritte Bedingung existiert und ist fail-first belegt, nicht nur vorhanden | ✓ VERIFIED | Bedingung ausgebaut, `TestConflictAtAnotherTalosVersionDeclinesTheRefresh` und `TestConflictAtAnotherTalosVersionErasesNoStoredRefusal` einzeln gefahren: beide ROT, mit vollstaendiger Vorher-/Nachher-Zeile des Datensatzes im Fehlertext. Baum danach byteweise wiederhergestellt. |
| 10 | 02-25 — die drei Vorbedingungen decken den Definitionsbereich des Urteils vollstaendig ab | ✓ VERIFIED | Gegen die Behauptung des Code-Reviews nachgeprueft, nicht uebernommen: `ProbeBuildable` (probe.go:23-38) nimmt `(id, talosVersion, arch)` und nagelt `Platform: PlatformMetal` fest; `SecureBoot` bleibt Nullwert. Extensions/KernelArgs/Meta liegen im kanonischen Dokument (gleiche Id -> gleich), Cluster und Name beruehren die Probe nicht. Es bleibt keine vierte Achse. |
| 11 | 02-25 — der `409`-`detail` nennt beide Versionen, und der Datensatz bleibt in beiden Richtungen byteweise unveraendert | ✓ VERIFIED | Beide Tests pruefen `marshalRecord(after) == marshalRecord(before)` ueber den **ganzen** Datensatz (nicht `exceptTheProbeFields`) und verlangen beide Versionen im `detail`. `TestConflictAtAnotherTalosVersionErasesNoStoredRefusal` hat zusaetzlich eine Vorpruefung, die abbricht, wenn die erste Ablehnung keine echte war — der Test kann nicht versehentlich nichts beweisen. |
| 12 | 02-25 — der Refresh bleibt fuer den Fall erhalten, fuer den er gebaut wurde (die Bedingung ist nicht zu eng) | ✓ VERIFIED | Alle zehn `TestConflict*`-Faelle einzeln gefahren und gruen, darunter `TestConflictRefreshesTheVerdictItJustComputed`, `…StoresTheFactorysRefusal` und `…TouchesNothingButTheThreeProbeFields` (Ganz-Record-Vergleich plus `rev == before+1`). |
| 13 | 02-25 — jede Aussage im Baum, die zwei Bedingungen nannte, nennt jetzt drei | ✗ FAILED (partial) | Die drei namentlich aufgezaehlten Stellen sind nachgezogen (`model.go:104-119`, `model.go:133-139`, `api-contract.md:620-627`). Die vierte steht im selben File wie die geaenderte Funktion: `schematics.go:458` "for the two conditions". Siehe gaps[2]. |
| 14 | 02-26 — der Waechter liest die Eintraege nur noch aus dem herausgeschnittenen `REFUSED_RANGES`-Rumpf | ✓ VERIFIED | `refusedRangesDecl` (:83-85) schneidet zuerst, `parseBrowserRefusalRanges` liest nur noch `decl[1]`. Heute gemessen: 6 Treffer im Rumpf, 6 in der ganzen Datei. Die Aussage stimmt — was sie nicht leistet, ist die Bindung an den *Namen*, siehe truth 6. |
| 15 | 02-26 — ein eintragsfoermiges Literal ausserhalb der Deklaration wird nicht mehr eingefaltet | ✓ VERIFIED | Der Zaehlvergleich `whole != len(matches)` (:276-284) faengt es und nennt beide moeglichen Ursachen; die Tabellenzeile "an entry-shaped literal outside the declaration" pinnt die Meldung inhaltlich. |
| 16 | 02-26 — ein Eintrag, den der Waechter nicht lesen kann, ist ein Fehlschlag und kein stilles Ueberspringen | ✗ FAILED (partial) | Syntaktisch ja: Dezimalliteral und benannte Konstante scheitern mit dem Literal im Text (beide Tabellenzeilen gruen). Semantisch nein: `{0xFFFFFFFF, 0xFFFFFFFF}` -> `{-1 -1}`, `{0x10FFFF, 0xFFFFFFFF}` -> `{1114111 -1}`, `{0x001f, 0x0000}` -> `{31 0}`, jeweils `err=<nil>` — Eintraege, die im Sweep keinen Codepoint abdecken und still uebergangen werden. Siehe gaps[0]. |
| 17 | 02-26 — `TestBrowserRefusalSetEqualsTheServers` bleibt gruen und misst weiterhin dasselbe | ✓ VERIFIED | Neu gemessen statt abgeschrieben: den `{ from: 0xfeff, to: 0xfeff }`-Eintrag aus `images.tsx` geloescht -> `--- FAIL`, `1 codepoints the server refuses are accepted …: U+FEFF`. Datei wiederhergestellt. Heute ist nichts maskiert. |
| 18 | 02-26 — die Pruefung ist eine reine Funktion, deren Fehlerfaelle selbst geprueft werden | ✓ VERIFIED | `parseBrowserRefusalRanges(source string) ([]declaredRange, error)` nimmt kein `*testing.T`; `browserRefusalRanges` haelt nur noch Dateilesen und `t.Fatalf` und wiederholt die Begruendung nicht. Zwei Tabellentests mit 4 + 2 Zeilen pruefen die Fehlerfaelle. Das ist der strukturell wertvollste Teil dieser Runde — er ist der Grund, warum gaps[0] mit einer Tabellenzeile und drei Zeilen Code zu schliessen ist. |
| 19 | Die stehende Wahrheit der Phase — kein gespeicherter Zustand behauptet mehr, als der Datensatz traegt (T-02-62 / G-02-1 / G-02-8-Linie) | ✓ VERIFIED | Fuer den **Store** wiederhergestellt: die Reproduktion aus Runde 4 laeuft seither als Test und ist rot, sobald man die Bedingung entfernt. Fuer den **Ledger** nicht — das ist truth 8 und derselbe Defekttyp, eine Ebene hoeher. |

**Score:** 14/19 truths verified (1 present, behavior-unverified; 4 failed, davon zwei in derselben
Datei und mit demselben Wurzelgrund).

## 2. Deferred items

| # | Item | Addressed In | Evidence |
|---|---|---|---|
| 1 | Die UI-Haelfte von SC 3 / TRANS-05 | Phase 3 | Kein Produktivaufrufer der Naht; ROADMAP Phase 3 besitzt die Inventar-Route. |
| 2 | TRANS-08 — die Contract-Suite gegen echtes Talos | Phase 3 | REQUIREMENTS.md:226 (Pending); ROADMAP Phase 3 SC 5 und die `Note` sagen, warum. |
| 3 | G-02-9 — eine ausgebliebene Probe ist weiterhin dauerhaft | 02-DECISION-probe-budget.md Option 1, nicht genommen | An sieben unabhaengigen Stellen als offen verbucht, in beiden SUMMARYs, im Ledger (58 und 66, beide `open`) und im Testkommentar. **Diese Runde hat die Minderung angefasst und den Eintrag nicht verwaessert.** |

## 3. Required artifacts

| Artifact | Expected | Status | Details |
|---|---|---|---|
| `internal/httpapi/handlers/schematics.go` | die dritte Bedingung in `refreshTheStoredVerdict` und der ablehnende Satz, der beide Versionen nennt | ✓ VERIFIED | :579-582 plus `versionMismatchReason` :628-632. Fail-first belegt. Ein residualer Doc-Kommentar bei :458 ist gaps[2]. |
| `internal/httpapi/handlers/schematics_test.go` | die zweite Katalog-Version des Fakes und zwei Regressionstests, einer je Richtung | ✓ VERIFIED | `otherCatalogVersion = "v1.12.0"` (:45), Gate bei :333; `TestConflictAtAnotherTalosVersionDeclinesTheRefresh` (:3152) und `…ErasesNoStoredRefusal` (:3193). Beide vergleichen den ganzen Datensatz; der zweite bricht ab, wenn die Vorbedingung nicht steht. |
| `internal/model/model.go` | die auf drei Bedingungen verengten Doc-Kommentare bei `ID` und `TalosVersion` | ✓ VERIFIED | :104-119 und :133-139. Der Zweig "gleiche Version" zaehlt die Talos-Version noch unter den Refusal-Zielen auf (Review IN-02) — eine Ungenauigkeit im Nebensatz, kein Truth-Fehler. |
| `docs/api-contract.md` | die dritte Zeile der Bedingungstabelle und der angepasste Outcome-Satz | ✓ VERIFIED | :620-627 und :651-659. Die Ueberladung von "three" (Review IN-03) ist kosmetisch. |
| `02-DECISION-schematic-identity.md` | ein als nachtraeglich markierter Anhang, ohne den entschiedenen Text zu beruehren | ✓ VERIFIED | Praefix-Hash gemessen: `ed2d0874…ee77c9` fuer die ersten 9137 Bytes, exakt wie in der Prohibition gefordert. |
| `internal/imagefactory/guard_drift_test.go` | ein an seine Deklaration verankerter Waechter, dessen Fehlerfaelle selbst geprueft werden | ⚠️ PARTIAL | Die Herausloesung (:233-299) und die beiden Tabellentests sind echte, gute Arbeit. Der Anker bindet an ein Namenspraefix und die Bounds werden nicht validiert — als Waechter also weiterhin falsifizierbar. gaps[0]. |
| `.planning/WINDOWS.md` | der Ledger der Runde: 66, 67, 68 (Uebergabe von 02-25) und 69 | ⚠️ PARTIAL | Struktur einwandfrei: drei Repraesentationen unabhaengig geparst, 54/0/15/69 dreifach gleich, ids 1..69 lueckenlos und doppelfrei, `windows status` gruen. Eintrag 69 ist als `fixed` angelegt fuer eine Eigenschaft, die verengt und nicht geschlossen ist. gaps[1]. |
| `web/src/routes/images.tsx` | von dieser Runde netto unberuehrt | ✓ VERIFIED | Nicht im `git diff b6e954b..HEAD --stat`. Meine eigenen zwei Falsifikationen (Umbenennung, U+FEFF-Loeschung) sind byteweise zurueckgenommen; `git diff` ist leer. |

## 4. Key link verification

| From | To | Via | Status | Details |
|---|---|---|---|---|
| `internal/httpapi/handlers/schematics.go` | `internal/imagefactory/probe.go` | die drei Vorbedingungen des Refresh decken den Definitionsbereich von `ProbeBuildable` ab | ✓ WIRED | `(id, talosVersion, arch)`, `Platform` festgenagelt. Gegengeprueft, nicht uebernommen. |
| `internal/httpapi/handlers/schematics.go` | `internal/imagefactory/schematicid.go` | `Canonical()` emittiert weder Architektur noch Version, deshalb sind zwei Versuche ein Datensatz | ✓ WIRED | Das ist der Grund, warum die Bedingung eine Ablehnung sein muss und kein zweiter Datensatz. |
| `internal/httpapi/handlers/schematics_test.go` | `internal/imagefactory/author.go` | der Fake bedient zwei versions-skopierte Kataloge, sonst kann kein Test die Version ueber einen Konflikt hinweg variieren | ✓ WIRED | `:333` `parts[1] != catalogVersion && parts[1] != otherCatalogVersion`. Genau die eine Zeile, deren Fehlen der Grund war, dass die Regression ausgeliefert wurde. |
| `internal/httpapi/handlers/schematics.go` | `internal/store/store.go` | der Refresh liest fuer `Rev` und schreibt damit | ✓ WIRED | CAS-Verlust und geloeschter Datensatz beide getestet und gruen. |
| `internal/imagefactory/guard_drift_test.go` | `web/src/routes/images.tsx` | der Go-Test liest die **Deklaration** `REFUSED_RANGES` | ⚠️ PARTIAL | Er liest jetzt einen herausgeschnittenen Rumpf statt der ganzen Datei — aber er bindet an ein Praefix des Namens, nicht an den Namen. Gemessen. |
| `internal/imagefactory/guard_drift_test.go` | `internal/httpapi/handlers/budget_drift_test.go` | die uebernommene Verankerungs-Disziplin | ⚠️ PARTIAL | Der Zeilenanfangs-Anker und `regexp.QuoteMeta` sind uebernommen; das Praezedenzmuster `^\s*(?:export\s+)?const\s+NAME\s*=` fordert dort das `=` unmittelbar nach dem Namen — genau die Stelle, an der die Uebernahme aufgeweicht wurde. |
| `02-25-SUMMARY.md` | `.planning/WINDOWS.md` | `## Ledger entries to file` — die Uebergabe an den Plan, der die Datei besitzt | ✓ WIRED | Eintraege 66, 67, 68 tragen `[from 02-26, uebergeben von Plan 02-25]` und den Wortlaut aus 02-25. |

## 5. Behavioural spot-checks

| Behaviour | Command | Result | Status |
|---|---|---|---|
| Fail-first der dritten Bedingung | dritte Bedingung entfernt, `go test -run TestConflictAtAnotherTalosVersion -v` | beide ROT; Ausgabe reproduziert Runde 4 woertlich (`usable: true`, `probe_reason: ""`, rev 1->2) | ✓ PASS — die Tests beweisen, was sie behaupten |
| Der Refresh nach dem Fix | `go test ./internal/httpapi/handlers/ -run TestConflict -v` | 10/10 PASS, 12.666s | ✓ PASS |
| Praefix-Umbenennung am echten Baum (Review WR-01) | `REFUSED_RANGES` -> `REFUSED_RANGES_LEGACY` in `images.tsx`, `go test -run TestBrowserRefusal -v` | `--- PASS` fuer beide Tests, alle vier Untertests gruen | ✗ FAIL — Waechter gruen gegen eine Quelle ohne `REFUSED_RANGES` |
| Praefix-Fall an der reinen Funktion | `parseBrowserRefusalRanges` mit `REFUSED_RANGES_LEGACY` / `REFUSED_RANGESX` | `ranges=[{0 0}] err=<nil>` / `ranges=[{0 1}] err=<nil>` | ✗ FAIL |
| Bound-Validierung (Review WR-03) | `{0xFFFFFFFF,…}`, `{0x10FFFF,0xFFFFFFFF}`, `{0x001f,0x0000}` | `{-1 -1}` / `{1114111 -1}` / `{31 0}`, jeweils `err=<nil>` | ✗ FAIL — Eintraege, die keinen Codepoint abdecken, werden still uebergangen |
| Klammer im Kommentar (Review WR-02) | vollstaendige Deklaration, `// see the table in RefusedRange[] above` als erste Rumpfzeile | `err=… is declared but carries no entries this guard can read` | ⚠️ fail closed, aber falsche Diagnose |
| Der Waechter misst weiterhin etwas | `{ from: 0xfeff, to: 0xfeff }` geloescht, Test gefahren | `--- FAIL`, `U+FEFF` benannt | ✓ PASS |
| Entscheidungsdokument unberuehrt | `head -c 9137 … \| shasum -a 256` | `ed2d0874…ee77c9`, exakt wie gefordert | ✓ PASS |
| Ledger-Selbstkonsistenz | unabhaengiger Parse von Frontmatter, Tabelle und JSON | 54/0/15/69 dreifach, 0 Abweichungen, ids 1..69 | ✓ PASS |
| G-02-9-Ehrlichkeit | `grep -rn 'G-02-9'` ueber SUMMARYs, Ledger, `internal/`, `docs/` | ausschliesslich Formulierungen, die ihn offen nennen | ✓ PASS |
| Regression SC 1/2/3/5 | `go test ./internal/talossim/ ./internal/talos/ -list '.*'` plus `git diff b6e954b..HEAD --stat` | alle neun genannten Tests vorhanden; keine der vier Bereiche im Runde-5-Diff | ✓ PASS |
| Gesamtsuite | vom Orchestrator bei HEAD | `go test ./... -count=1 -race` Exit 0 (15 Pakete), `npm test` Exit 0 (9 Dateien / 137 Tests) | ✓ PASS — von nichts hier Gemessenem widersprochen; die beiden Pakete dieser Runde nach allen Wiederherstellungen erneut gruen |

## 6. Requirements coverage

| Requirement | Source plans | Status | Evidence |
|---|---|---|---|
| FOUND-12 | 02-07 | ✓ SATISFIED | `internal/talos/dryrun.go` + Tests; `main.go:171` Kompositionswurzel. Ausserhalb des Runde-5-Diffs. |
| TRANS-01 | 02-01 | ✓ SATISFIED | Echter Machinery-Client ueber die `Dialer`-Naht mit echtem mTLS. |
| TRANS-02 | 02-01 | ✓ SATISFIED | `Dialer` und `DiscoverySource` mit je zweiter Implementierung. |
| TRANS-03 | 02-05 | ✓ SATISFIED | Getrennte Typen; `TestMaintenanceClientMethodSetIsClosed`. |
| TRANS-04 | 02-05 | ✓ SATISFIED | `TestRequireDeadline`, `TestRetryAllowlistIsExactlyTheFastReadClass`. |
| TRANS-05 | 02-05 | ⚠️ PARTIAL | Transport-Haelfte belegt; UI-Haelfte ohne Aufrufer — Phase 3. |
| TRANS-06 🚫 | 02-01, 02-08 | ✓ SATISFIED | `internal/talossim` mit In-Memory-COSI, drei Streams, Method-Drift-Guard. |
| TRANS-07 | 02-03 | ✓ SATISFIED | Neun Szenarien deklariert (`scenario.go:48-57`) und contract-getestet. |
| TRANS-08 | — | ⏭ DEFERRED | Phase 3 (REQUIREMENTS.md:226, ROADMAP Phase 3 SC 5). |
| FACT-01 | 02-02, 02-06, 02-26 | ✓ SATISFIED | Versions-skopierter Katalog, kein Freitextfeld. Der Drift-Waechter ueber die Browser-Ablehnungsmenge ist heute korrekt und misst; seine kuenftige Zuverlaessigkeit ist gaps[0] und keine Aussage ueber die heutige Menge. |
| FACT-02 | 02-02, 02-22, 02-24, 02-25 | ✓ SATISFIED | **Von BLOCKED in Runde 4 auf SATISFIED gedreht.** Vorvalidierung der Extension-Namen unveraendert; "gilt erst als brauchbar, nachdem ein Model-Build-Probe es bestaetigt hat" gilt jetzt auch versionsuebergreifend, fail-first belegt. |
| FACT-03 | 02-04, 02-09, 02-23 | ✓ SATISFIED | Exakte ISO-/Installer-/PXE-URLs, versionsaufgeloester Repo-Name, Architektur als Parameter. Ausserhalb des Runde-5-Diffs. |
| FACT-04 | 02-04, 02-06 | ✓ SATISFIED | installer/initramfs-Warnung emittiert und in der UI gespiegelt. |
| FACT-05 | 02-04 | ✓ SATISFIED | Pre-Release strukturell gefiltert; kaputte Versionen kuratiert. |
| FACT-06 | 02-02, 02-14, 02-24, 02-26 | ✓ SATISFIED | Id lokal vorberechnet und persistiert; `Canonical()` gegen die echte Factory von einem externen Orakel gemessen. |

**Orphaned requirements:** keine. Alle vierzehn IDs der Phase erscheinen in mindestens einem
`requirements`-Frontmatter, und REQUIREMENTS.md:218-232 bildet keine weitere ID auf Phase 2 ab.

## 7. Anti-patterns

| File | Line | Pattern | Severity | Impact |
|---|---|---|---|---|
| — | — | `TBD` / `FIXME` / `XXX` | — | **Keine** in den fuenf von dieser Runde geaenderten Dateien. Der einzige `XXX`-Treffer ist `\uXXXX` in `hexQuad` (schematics.go:1169), kein Schuldenmarker. |
| — | — | `TODO` / `HACK` / `PLACEHOLDER` | — | **Keine** in den fuenf geaenderten Dateien. |
| `internal/imagefactory/guard_drift_test.go` | 83-85 | ein Anker, der an ein Namenspraefix bindet | 🛑 Blocker | gaps[0]. |
| `internal/imagefactory/guard_drift_test.go` | 286-299 | ungepruefte Bounds; `rune(uint64)` verliert still | 🛑 Blocker | gaps[0]. |
| `.planning/WINDOWS.md` | Eintrag 69 | ein `fixed`-Eintrag fuer eine verengte, nicht geschlossene Eigenschaft | 🛑 Blocker | gaps[1]. `workflow.windows_enforce` liest genau dieses Register. |
| `internal/httpapi/handlers/schematics.go` | 457-459 | "the two conditions" ueber einer Funktion mit drei | ⚠️ Warning | gaps[2] (02-REVIEW WR-07). |
| `internal/imagefactory/guard_drift_test.go` | 57-85 | der Doc-Kommentar von `refusedRangesDecl` haengt an `refusedRangesEntry` | ⚠️ Warning | 02-REVIEW WR-05. Die Begruendung der Anker-Entscheidung — der inhaltliche Kern dieser Runde — steht unter dem falschen Symbol. Derselbe Defekt, den die Vorrunde fuer `allowedHosts`/`ssoOnly` festgehalten hat. |
| `internal/httpapi/handlers/schematics.go` | 628-632 | `versionMismatchReason` ohne Leerwert-Zweig, obwohl der Kommentar bei :575-578 den Leerwert als erreichbar beschreibt | ⚠️ Warning | 02-REVIEW WR-06. `archMismatchReason` hat den Zweig; die Zwillingsfunktion nicht, und der Satz liest sich dann mit leerem Subjekt. |
| `internal/imagefactory/guard_drift_test.go` | 119-124 | der Surrogate-Bereich wird nur an seinen beiden Endpunkten geprueft | ⚠️ Warning | 02-REVIEW WR-04 (dort als vorbestehend gefuehrt, korrekt). Ein Waechter fuer Mengengleichheit prueft hier ein Intervall an zwei Punkten. |
| `internal/imagefactory/guard_drift_test.go` | 263-269 | "die Deklaration ist leer" ueber eine Deklaration mit sechs Eintraegen | ⚠️ Warning | 02-REVIEW WR-02, hier reproduziert. Fail closed, aber die Diagnose schickt den Leser in die falsche Richtung. |
| `internal/model/model.go` | 104-107 | der Zweig "gleiche Version" zaehlt die Talos-Version weiterhin unter den Refusal-Zielen | ℹ️ Info | 02-REVIEW IN-02. In diesem Zweig ist sie definitionsgemaess Vorbedingung. |
| `docs/api-contract.md` | 620-621 | "all three conditions … which of the three outcomes" in einem Satz | ℹ️ Info | 02-REVIEW IN-03. |
| `internal/model/model.go` / `docs/api-contract.md` | 181-199 / 617-634 | Datensaetze, die zwischen `efb31da` und `d4ee1f5` verfaelscht wurden, werden weder erkannt noch benannt | ℹ️ Info | 02-REVIEW IN-04. Der Erholungsweg existiert (ein POST an *der* Version, die der Datensatz nennt) und ist nirgends ausgesprochen. |
| `internal/imagefactory/guard_drift_test.go` | 340-353, 368 | `wantRanges: 6` pinnt die Eintraege der echten Datei, die Begruendung spricht von Tabellenzeilen; `%d entries` erzeugt "1 entries" und ist als `wantErr` woertlich gepinnt | ℹ️ Info | 02-REVIEW IN-05, IN-06. |
| `internal/httpapi/handlers/schematics.go` | 584-604 | keine Monotoniebedingung ueber `ProbedAt`; das CAS schuetzt nur den Fall, dass A vor B gelesen hat | ℹ️ Info | 02-REVIEW IN-07. Beide Urteile beantworten seit dieser Runde dieselbe Frage `(id, version, arch)`, deshalb Info. |
| `web/src/api.ts` / `web/src/api.test.ts` | 475 / 99 | `REQUEST_CEILING_MS = 150_000` gegen `toBeGreaterThan(130_000)` — zwei ungewachte Handtranskriptionen | ⚠️ Warning | Aus Runde 4 unveraendert weitergetragen und bei `d7d07ea` neu gemessen. Siehe coincidental_reliance_items. |

**Test quality audit.** Kein uebersprungener oder deaktivierter Test ist der alleinige Beleg fuer
eine Anforderung: `grep -rn "t.Skip"` ueber die beiden von dieser Runde geaenderten Testdateien
findet nichts; die einzigen Skips der Phase bleiben die Opt-in-Live-Guards (`live_test.go`,
`canonical_live_test.go`), beide im Ledger (5/64 und 35). Die Assertion-Staerke der neuen Tests ist
Wert- bzw. Verhaltensebene: `marshalRecord` ueber den ganzen Datensatz, `rev`-Arithmetik, inhaltliche
Erwartungen an den Fehlertext statt blosser Nicht-Nullheit. Zwei Beobachtungen, die diesen Abschnitt
tragen: (a) `TestConflictAtAnotherTalosVersionErasesNoStoredRefusal` hat eine **Vorpruefung**, die
abbricht, wenn die erste Ablehnung keine echte war — der Test kann nicht versehentlich nichts
beweisen, und das ist die Disziplin, die dieser Phase in Runde 4 gefehlt hat; (b)
`TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration` prueft die Fehlerfaelle des Waechters
ueberhaupt erst pruefbar — der Fortschritt ist echt, und die fehlende fuenfte Tabellenzeile ist
genau deshalb eine Zeile und keine Umarbeitung. Provenienz der Erwartungswerte fuer den
FACT-06-Differential bleibt extern und nur extern.

## 8. Decision coverage

`02-CONTEXT.md` deklariert D-01 … D-10. Neun sind in mindestens einer SUMMARY benannt. **D-07**
(`talossim` lebt in `internal/talossim`, Modulgrenze erweitert) ist in keiner SUMMARY benannt, wird
aber im Code eingehalten und erzwungen: `internal/depguard_test.go:30` deklariert
`simulatorPackage = rootModule + "/internal/talossim"` und :257 pinnt es in der Grenztabelle. Bei
`d7d07ea` erneut geprueft und unveraendert. Nicht blockierend; nur zur Drift-Erkennung notiert.

## 9. Human verification required

### 1. Das zusammengebaute Binary durch einen Browser gegen die echte Image Factory fahren

**Test:** 02-UAT.md Test 5, Unterpruefungen (a)-(d).
**Expected:** Die Route verhaelt sich, wie die Suiten es vorhersagen, mit echter Latenz und echtem Bundle.
**Why human:** 02-UAT.md Test 5 traegt weiterhin `result: issue`. Das Browser-Projekt oeffnet
ImagesView in Chromium, sagt in seinem eigenen Doc-Kommentar aber, dass kein Binary laeuft und kein
Bundle ausgeliefert wird.

### 2. Bestaetigen, dass der Browser-Install-Schritt der CI auf ubuntu-latest laeuft

**Test:** ein CI-Lauf, der `.github/workflows/ci.yml:71` erreicht.
**Expected:** `playwright install --with-deps chromium` gelingt und das Browser-Projekt laeuft.
**Why human:** WINDOWS-Eintrag 44 ist weiterhin offen. Es scheitert geschlossen, das Risiko ist ein
roter Lauf und kein stiller Pass.

### 3. Eine kalte Installer-Aufloesung gegen factory.talos.dev neu messen

**Test:** eine kalte Assets-Anfrage gegen die echte Factory nach dem Fan-out aus 02-23.
**Expected:** Die kalten Kosten sind der langsamste einzelne Kandidat, nicht die Summe.
**Why human:** 02-24-SUMMARY.md:232 und WINDOWS-Eintrag 57 sagen selbst, dass die Verbesserung
offline gemessen und live unmessen ist.

### 4. Entscheiden, wo die Nicht-Gap-Befunde der Code-Reviews leben

**Test:** WR-05, WR-06 und der Surrogate-Punkttest aus Runde 5 sowie IN-01..IN-07 in
behoben / abgelegt / abgelehnt triagieren — dazu die vier Warnungen der Vorrunde (SecureBoot-`ParseBool`,
Installer-Provenienz, ungewachte Ceiling-Transkription, `allowedHosts`/`ssoOnly`), die seit Runde 4
unveraendert anstehen.
**Expected:** Jeder Punkt steht in `.planning/WINDOWS.md` oder ist ausdruecklich abgelehnt.
**Why human:** Eine Policy-Entscheidung. Geprueft: keiner dieser Punkte steht in irgendeiner der drei
Repraesentationen des Registers, und `workflow.windows_enforce` liest es beim Ship. Die Liste ist
gegenueber Runde 4 laenger geworden.

### 5. `⚠️ PRESENT_BEHAVIOR_UNVERIFIED` — die UI-Haelfte von SC 3

Siehe behavior_unverified_items. Erst nach Phase 3 ausuebbar; die Transport-Haelfte ist belegt.

## 10. Gaps summary

**Runde 5 hat ihren schwereren Auftrag erledigt und ihren leichteren zur Haelfte.**

**G-02-24 ist geschlossen, und die Art der Schliessung ist gut.** Die dritte Bedingung steht neben
der zweiten statt an ihrer Stelle, uebernimmt deren Form und deren Begruendungsschema, und der
Kommentar sagt den entscheidenden Satz explizit: die Version ist auf *derselben* Evidenz skopiert,
die der Architektur-Guard fuer sich selbst anfuehrt. Ich habe die Bedingung ausgebaut und beide neuen
Tests rot werden sehen, mit einer Ausgabe, die Runde 4s Reproduktion woertlich wiederholt — die
Tests beweisen also, was sie behaupten. Das strukturelle Argument des Code-Reviews habe ich
gegengeprueft statt uebernommen: `ProbeBuildable` haengt an `(id, talosVersion, arch)` mit
festgenagelter Plattform, jede andere Achse liegt entweder im kanonischen Dokument oder beruehrt die
Probe nicht. Es bleibt keine vierte Achse offen. FACT-02 dreht damit von BLOCKED auf SATISFIED und
SC 4 haelt in allen Konjunkten.

**G-02-25 ist verengt und nicht geschlossen**, und das ist keine Formulierungsfrage, sondern
gemessen. Ich habe Runde 4s Experiment wiederholt — mit dem einen Unterschied, dass der neue Name
`REFUSED_RANGES` als Praefix traegt. `TestBrowserRefusalSetEqualsTheServers` ist gruen gegen eine
`images.tsx`, in der `REFUSED_RANGES` nicht vorkommt, und der in dieser Runde eigens geschriebene
Falsifikationstest ist es mit allen vier Untertests ebenfalls, weil seine drei Negativzeilen
ausschliesslich Namen benutzen, die kein Praefix sind. Dazu kommt die zweite Haelfte: der neue
Unlesbar-Check ist syntaktisch und nicht semantisch, also faellt ein Eintrag mit einem Bound
ausserhalb des Unicode-Bereichs zu `{-1 -1}` zusammen, deckt keinen Codepoint ab und wird still
uebergangen — woertlich die Eigenschaft, die der Check beseitigen sollte.

Es waere unfair, das als reinen Fehlschlag zu lesen. Der wertvollste Teil des Plans ist gelungen:
die Pruefung ist eine reine Funktion, ihre Fehlerfaelle haben zum ersten Mal eigene Tests, und die
Deklaration wird tatsaechlich zuerst herausgeschnitten. Genau deshalb ist der Rest billig — eine
Wortgrenze im Anker, drei Zeilen Bound-Validierung, zwei Tabellenzeilen. Vor dieser Runde waere
dieselbe Korrektur eine Umarbeitung gewesen.

**Der dritte Gap ist der unangenehmste, weil er die Buchhaltung betrifft.** Der Ledger dieser Runde
ist sonst vorbildlich — 66 amendiert 58 ausdruecklich, 68 sagt ausdruecklich, dass eine Pruefung
nichts gefunden hat, alle drei Repraesentationen stimmen. Genau vor diesem Hintergrund faellt
Eintrag 69 auf: er ist bei der Anlage `fixed` und sagt "Plan 02-26 hat es geschlossen". Das ist
dieselbe Defektklasse, die diese Phase seit vier Runden korrigiert — eine Behauptung, die breiter
ist als ihre Messung —, diesmal nicht im Store, sondern im Register, das `workflow.windows_enforce`
beim Ship liest. Der vierte Gap ist ein Wort: `schematics.go:458` verweist auf "the two conditions"
einer Funktion, die drei prueft, 54 Zeilen tiefer im selben File.

Keiner der vier Gaps beruehrt die Transport-Naht, `talossim`, die neun Szenarien, `--dry-run` oder
die URL-Herleitung. Success Criteria 1, 2, 4 und 5 halten; 3 haelt auf seiner Transport-Haelfte, mit
der UI-Haelfte korrekt nach Phase 3 verschoben.

---

_Verified: 2026-09-04T11:17:28Z bei `d7d07ea`_
_Verifier: Claude (gsd-verifier) — Runde 5_
