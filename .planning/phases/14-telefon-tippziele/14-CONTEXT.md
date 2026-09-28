# Phase 14: Telefon — Tippziele und ihr Wächter - Context

**Gathered:** 2026-09-28
**Status:** Ready for planning

<domain>
## Phase Boundary

Bei 390 px Breite hat jedes Bedienelement eine Tippfläche von mindestens
44 × 44 px — auf jeder Route, auch in dem, was erst nach einem Tipp erscheint
(mobile Navigation, Menüs wie das Power-Menü, Bestätigungs- und Sudo-Dialog),
und auf der neuen Host-Seite mit ihren Aktionen. Die Layout-Prüfung in
`task ci` wird rot, sobald ein Element darunter fällt, und ebenso, sobald eine
Route im Router steht, die sie nicht misst. Oberhalb `md` bleibt alles, wie es
ist.

Requirements: MOB-01, MOB-02, MOB-03.

**Ausgangslage:** `web/scripts/layout-audit.mjs` misst Tippziele unter 44 px
bei 390 px seit e2bd690 und läuft in `task ci` mit; am 2026-09-26 bestanden
alle Routen. Es fehlen: geöffnete Zustände (nach dem Login klickt der Wächter
nichts an) und eine Routenliste, die nicht von Hand gepflegt wird.

**Nicht diese Phase:** Tabellen bleiben wischbar (Betreiber, 2026-09-17,
V2-UI-01) — keine Karten-Darstellung; keine Änderung an Desktop-Größen; keine
neuen Breiten außer 390 und 1280; kein Umbau von Seiten über die Tippziele
hinaus.

</domain>

<decisions>
## Implementation Decisions

### Routen vollständig (MOB-02, Kriterium 2)

- **D-01:** Die Routenliste zieht aus `layout-audit.mjs` in eine geteilte Datei
  `web/scripts/layout-routes.json`: jede Route des Routers mit einem konkreten
  Pfad; Parameter-Routen (`/nodes/$uuid`, `apps/$namespace/$kind/$name`) mit
  einem Beispiel aus `web/fixtures/demo.json`.
- **D-02:** Ein vitest-Test (`web/src/layoutRoutes.test.ts`) läuft den
  **echten** `routeTree` ab (dafür wird der Baum aus `App.tsx` in ein
  seiteneffektfreies Modul gezogen) und schlägt fehl, wenn eine Router-Route
  in der JSON fehlt, eine JSON-Route im Router nicht existiert oder eine
  Parameter-Route kein Beispiel hat. `task test:layout` führt diesen Test
  **vor** dem Browser-Lauf aus, sodass Kriterium 3 („Route aus der Liste
  genommen → `task test:layout` Exit ≠ 0") am Befehl selbst abgelesen wird.
  Platzhalter-Routen aus `NAV_AREAS` zählen mit; `/setup`, `/login` und
  `/wall` werden wie heute gesondert gemessen und in der JSON als solche
  markiert.

### Geöffnete Zustände (MOB-02, Kriterium 2)

- **D-03:** Eine ausdrückliche Liste von **Öffnern** im Audit:
  `{ route, name, open: <Schritte>, expect: <Selektor des Geöffneten> }`.
  Mindestens: mobile Navigation („Open the navigation", auf einer Route
  genügt — die Navigation ist überall dieselbe), das Power-Menü auf
  `/nodes/<beispiel>`, der Bestätigungsdialog einer Knotenaktion, der
  Sudo-Dialog, auf `/host` der Aktionsdialog mit Tippfeld (Phase 13). Nach dem
  Öffnen wird dieselbe Tippziel-Messung auf das Geöffnete angewendet.
- **D-04:** Ein Öffner, dessen Auslöser oder dessen `expect` nicht gefunden
  wird, ist **rot**, nie übersprungen („Empty output is not green"). Die
  Anzahl gemessener Elemente je Öffner wird ausgegeben, damit ein leerer Dialog
  auffällt.
- **D-05:** Den Sudo-Dialog öffnet der Wächter über den echten Weg: das
  Audit-Konto hat nach dem Login kein offenes Sudo-Fenster bzw. es wird
  geschlossen, und eine destruktive Aktion gegen den echten Daemon antwortet
  `428 sudo.required` — der Dialog erscheint, ohne dass ein POST je etwas
  ausführt (der Wächter bestätigt nichts, er misst und schließt mit Escape).
  Lässt sich das Fenster nicht zuverlässig schließen, darf eine
  Audit-Fixture-Antwort `428` liefern; das entscheidet der Plan nach einem
  Versuch.

### Größen setzen, Desktop unverändert (MOB-01)

- **D-06:** Größen wachsen **nur** über `max-md:`-Varianten, wie es
  `ui/button.tsx` schon tut (`h-8 max-md:h-11`, `size-8 max-md:size-11`).
  Korrekturen zuerst in den geteilten Bausteinen (`ui/button`,
  `ui/dropdown-menu`-Items, `ui/dialog`-Schließen, `ui/select`-Trigger,
  `ui/input`), erst dann an einzelnen Stellen.
- **D-07:** Kleine Symbolknöpfe, die optisch klein bleiben sollen, bekommen die
  Tippfläche über Innenabstand oder ein unsichtbares Trefferfeld
  (`max-md:` Pseudo-Element/Padding) — nicht über ein größeres Symbol.
- **D-08:** „Desktop unverändert" wird einmal in der Phase gemessen: der
  Wächter schreibt bei 1280 px Größe und Lage jedes Bedienelements; der Lauf
  vor der ersten Änderung und der nach der letzten werden verglichen und das
  Ergebnis in die SUMMARY geschrieben. Dauerhaft hält es die
  `max-md:`-Disziplin, keine eingecheckte Baseline.
- **D-09:** Die vorhandenen Ausnahmen des Wächters bleiben, wie sie begründet
  sind (verstecktes natives `<select>` hinter Radix, Link im Fließtext nach
  WCAG 2.5.8); neue Ausnahmen nur mit Begründung im Code und einzeln
  aufgezählt.

### Beweis (Kriterium 3, 4)

- **D-10:** Drei Rot-Checks, einzeln, Exit-Code vom Befehl selbst gelesen (nicht
  durch eine Pipe): (a) ein Element auf unter 44 px zurückgesetzt (z. B.
  `max-md:h-11` am Button entfernt), (b) ein Dialogknopf unter 44 px, (c) eine
  Route aus `layout-routes.json` genommen.
- **D-11:** `/host` mit Aktionen, Bestätigungsdialog und Tippfeld besteht bei
  390 px; dazu ein Handtest auf einem Telefon (Aktion bis zur getippten
  Bestätigung, **ohne** abzusenden), dessen Ergebnis in der SUMMARY steht — ist
  kein Telefon erreichbar, steht das dort als nicht ausgeführt, nicht als
  bestanden.

### Claude's Discretion

- Die genaue Form der Öffner-Liste und der Ausgabe.
- Welche Bausteine zuerst angepasst werden — Reihenfolge ergibt sich aus der
  ersten roten Messung des erweiterten Wächters.
- Ob `task test:layout` die vitest-Prüfung als eigenen Task-Schritt oder über
  ein npm-Skript aufruft.

### Ohne Rückfrage entschieden

Gewählt wurde jeweils die empfohlene Antwort; verworfen:

- *Routen per Regex aus den Quelldateien lesen* — verworfen, eine Liste aus
  `path: '…'`-Treffern sieht Kind-Routen und `NAV_AREAS`-Platzhalter falsch
  und scheitert still; der echte `routeTree` ist die Wahrheit.
- *Router zur Laufzeit über ein `window`-Global auslesen* — verworfen, das
  hieße ein Debug-Global im ausgelieferten Bundle.
- *Automatisch jeden Knopf anklicken, um Menüs/Dialoge zu finden* — verworfen,
  klickt auch destruktive Aktionen an und ist nicht deterministisch; eine
  ausdrückliche Öffner-Liste, die rot wird, wenn ein Öffner nichts findet, ist
  prüfbar.
- *Eingecheckte Desktop-Baseline als Dauer-Wächter* — verworfen, jede
  gewollte Desktop-Änderung müsste sie neu schreiben, und eine Baseline, die
  man ohne Lesen neu erzeugt, bewacht nichts.
- *Tabellen auf dem Telefon als Karten* — verworfen vom Betreiber (2026-09-17).
- *Größere Tippziele auch auf dem Desktop* — verworfen, Entscheidung des
  Betreibers vom 2026-09-17 (kompakter Desktop).

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `web/scripts/layout-audit.mjs`: `WIDTHS = [390, 1280]`, `TOUCH_MIN = 44`,
  Tippziel-Messung mit Ausnahmen, Clipping-Prüfung, Fixture-Server aus
  `web/fixtures/demo.json`, `ROUTES` (Hand-Liste), Login-Ablauf,
  `LAYOUT_SHOTS`-Bilder.
- `web/src/components/ui/button.tsx`: `max-md:`-Größen als Vorbild.
- `web/src/App.tsx`: `routeTree` (`rootRoute.addChildren(...)`),
  `placeholderRoutes` aus `NAV_AREAS`.
- `Header.tsx` („Open the navigation", `md:hidden`), `AppShell.tsx`
  („Close the navigation"), `PowerMenu.tsx`, `SudoDialog.tsx`, `ui/dialog.tsx`.
- `web/src/fixtures.test.ts`: hält die Fixtures an den zod-Schemas.

### Established Patterns
- Operator-Entscheidung 2026-09-17: 44 px unter `md`, Desktop kompakt; zwei
  Breiten.
- Guards laufen in der Kette (`task ci`), nicht opt-in (Ledger 5, 64).
- Exit-Codes vom Befehl selbst lesen, nie durch eine Pipe (Memory).

### Integration Points
- `Taskfile.yml` `test:layout` (Zeile ~116) und `ci`.
- `web/fixtures/demo.json`: `/host`-Antworten aus Phase 11–13, damit die Seite
  mit Daten und mit freigeschalteten Aktionen gemessen wird.

</code_context>

<specifics>
## Specific Ideas

- Der Aktionsdialog auf `/host` wird in der Fixture im Zustand „Helfer
  installiert" gemessen; im Zustand „nicht installiert" sind die Knöpfe
  gesperrt, aber weiterhin gemessen (gesperrt heißt nicht unsichtbar).
- Die erste Messung des erweiterten Wächters wird rot erwartet — das ist der
  Befund, mit dem die Arbeit beginnt, und wird so im Plan geführt.

</specifics>

<deferred>
## Deferred Ideas

- Weitere Breiten (z. B. 360 px, Tablet 768 px).
- Messung auf der Wand bei Fernseh-Auflösung als Wächter (heute nur Bild).
- Tabellen als Karten — vom Betreiber verworfen, nur der Vollständigkeit halber.

</deferred>
