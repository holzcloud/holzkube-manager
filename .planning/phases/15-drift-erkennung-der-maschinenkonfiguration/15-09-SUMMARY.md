---
phase: 15-drift-erkennung-der-maschinenkonfiguration
plan: 09
subsystem: web
tags: [drift, ui, node-page, cluster-card, banner, sudo, zod]
requires: [15-08]
provides:
  - "api.ts: configDriftStateSchema, configDriftDetailSchema (changes, paths, items lesen null als leere Liste), das Feld config_drift am Cluster, api.configDrift.get und confirm, ein Eintrag in ACTION_LABELS für das Sudo-Fenster"
  - "NodeConfigDrift: der Abschnitt Configuration der Knotenseite mit Zustand, Diff über alle Dokumente, Beginn, Basis, fester Auskunft zu Geheimniswerten und Bestätigen nur bei Drift"
  - "ConfigDriftRow: die Zeile Configuration der Clusterkarte"
  - "ConfigDriftBanner: das Banner auf jeder Seite, nur bei banner vom Server, nicht wegklickbar, geteilte Abfrage"
affects: [15-10]
actuals:
  tokens: 13800
  tasks: 3
  commits: 3
plan_head_before: b82d14f2890fdb8c7b4621b128fc04e402649777
plan_head_after: cba79f3443aff9d6c45d3f79841c08cf99e5f805
tech-stack:
  added: []
  patterns: ["Zeile und Banner nach NodeCertificatesRow und CertificateBanner", "Bestätigen über die globale 428-Behandlung, nur ein Name der Aktion", "Injektionsskript: genau ein Treffer, Typcheck vor dem Test, cmp danach"]
key-files:
  created:
    - web/src/components/NodeConfigDrift.tsx
    - web/src/components/NodeConfigDrift.test.tsx
    - web/src/components/ConfigDriftRow.tsx
    - web/src/components/ConfigDriftRow.test.tsx
    - web/src/components/ConfigDriftBanner.tsx
    - web/src/components/ConfigDriftBanner.test.tsx
  modified:
    - web/src/api.ts
    - web/src/api.test.ts
    - web/src/api.nulls.test.ts
    - web/src/routes/node-detail.tsx
    - web/src/routes/clusters.tsx
    - web/src/routes/clusters.test.tsx
    - web/src/components/AppShell.tsx
    - cmd/holzkube-managerd/reachable_test.go
key-decisions:
  - "Die Oberfläche rechnet nichts: Zustand, Sätze, 24-Stunden-Schwelle und Beginn kommen vom Server; die Zeile urteilt über abweichend nach dem Feld since, nicht nach dem Zustand drift"
  - "Als passend zählt nur ein Knoten im Zustand ok; baseline-created, settling, not-read, unreadable und baseline-damaged haben je einen eigenen Satz"
  - "Der Knopf Confirm as intended sendet den Digest der angezeigten Antwort; nach Erfolg und nach Fehler werden die Abfrage des Knotens und ['clusters'] neu geladen (onSettled)"
  - "Kein Knopf zum Zurücksetzen, kein Schließen-Knopf am Banner, kein README-Bild, die Wand unberührt"
requirements-completed: []
status: complete
---

# Phase 15 Plan 09: Oberfläche Summary

Die Drift ist für den Betreiber sichtbar: ein Abschnitt auf der Knotenseite mit Zustand, Diff und dem Bestätigen mit genau dem angezeigten Digest, eine Zeile Configuration auf der Clusterkarte, die einen nicht beurteilten Knoten nie als in Ordnung zählt, und ein Banner auf jeder Seite, das nur der Server einschaltet und das sich nicht wegklicken lässt.

BASE: b82d14f2890fdb8c7b4621b128fc04e402649777

`plan_head_before` ist derselbe Wert. `git rev-list --count BASE..HEAD` vor diesem SUMMARY-Commit: 3 (7f8b3b4, e5f8cc3, cba79f3).

## Wo es lief

Mac (darwin/arm64), Node v26.11.1, jsdom-Projekt von vitest (`NODE_OPTIONS=--no-experimental-webstorage npx vitest run --project jsdom`), Go aus dem Prüfstand des Rechners. **Kein Browser** (das Browser-Projekt von vitest, `*.browser.test.*` und der Layout- und Tippziel-Wächter liefen hier nicht und werden nicht behauptet), kein echter Cluster, kein Pi, kein `task`, kein Daemon-Prozess. Nichts installiert (`web/package.json` und `package-lock.json` unberührt), nichts gepusht, nichts getaggt, kein Changelog, kein README-Bild. Dokumentationswerte in allen Beispielen (`example.com`, `cp-1`, `m-cp-2`, `10.0.0.1`).

Am Ende, ganze Suite: `npx vitest run --project jsdom` 70 Dateien, 927 Tests grün; `npm --prefix web run typecheck` Exit 0; `npm --prefix web run lint` Exit 0 (die zwei Warnungen und die eine Info sind die alten, nicht von dieser Phase); `go test ./cmd/holzkube-managerd/ ./internal/publicrepo/ ./internal/httpapi/ -count=1`: `cmd` und `publicrepo` ok, `internal/httpapi` rot nur in den vier Tests, die `web/dist` brauchen (`TestCSPHashMatchesTheServedShell`, `TestForeignHostIsRefused`, `TestNoAssetIsInlinedPastTheContentSecurityPolicy`, `TestFontsAreServedAsFiles`; nicht Gegenstand); `GOTOOLCHAIN=go1.26.9 go run .../golangci-lint@v2.13.1 run ./...`: `0 issues.`

## Was gebaut wurde

- **Task 1, Tracer (7f8b3b4).** `api.ts`: `configDriftStateSchema` (sieben Zustände), `configDriftDetailSchema` (Zeitfelder und `diff` nullish, `changes` und `paths` `.nullish().transform(orEmpty)`, `duplicates` mit Vorgabe false, `notice`), `api.configDrift.get` und `confirm` (die ID mit `encodeURIComponent`), ein verankerter Eintrag in `ACTION_LABELS` (`Confirm this configuration as the new baseline`, mit einem `because`, das sagt, dass es die zuletzt bekannte Konfiguration überschreibt und das Banner abschaltet, am Knoten selbst nichts ändert). `NodeConfigDrift`: Zustands-Badge (kein Häkchen, `ok` heißt „As last known“, `not-read` heißt „not read yet“, `unreadable` „cannot be read“, `baseline-damaged` „baseline unusable“), der Satz des Servers, Basis (wann, woher), letzte Lesung, bei Drift der Diff in der Form von `config.tsx` mit Beginn, ein Absatz, was Confirm tut und was nicht, der Knopf mit `max-md:min-h-11`, der `notice` in jedem Zustand. Kein Knopf zum Zurücksetzen. Eingebunden in `node-detail.tsx`.
- **Task 2 (e5f8cc3).** `config_drift` am `clusterSchema` (nullish; `items` liest `null` als leere Liste). `ConfigDriftRow`: `not read yet` ohne Meldung; „abweicht“ folgt `since` (ein nicht antwortender Knoten mit Beginn zählt weiter, mit dem Zusatz `(last known)`); als passend zählt nur `ok`; Knoten ohne Urteil stehen in einem Satz „N nodes cannot be judged: …“ und mit ihrem Zustand in der Liste „All nodes“ (details, `max-md:min-h-11`); die Teilantwort wird gesagt; `baseline-created` und `settling` haben eigene Sätze. In `clusters.tsx` ein `dt` Configuration nach „Cluster certificates“.
- **Task 3 (cba79f3).** `ConfigDriftBanner` nach `CertificateBanner`: gleiche Abfrage `['clusters']`, filtert auf `config_drift?.banner === true`, je Cluster ein `role="status"` mit dem Satz des Servers, dem Clusternamen und einem Link „Open the node“ zum Knoten mit dem ältesten `since`. In `AppShell.tsx` direkt nach `CertificateBanner`, mit Kommentar. Die Gruppe `ConfigDriftRoutes` in `interfaceInFlight()` von `reachable_test.go` ist gelöscht (Aufräumen aus Plan 08).
- Gegen BASE unberührt: `git diff --name-only b82d14f -- web/src/routes/wall.tsx web/src/routes/wall.test.tsx web/fixtures internal/changelog internal` ist leer (0 Zeilen, gemessen nach jeder Aufgabe und am Ende); `web/package.json` unberührt.

## Rot gesehen

Verfahren: ein Skript (`inj.py`, im Scratchpad, nicht im Repo) sichert die Datei, ersetzt genau eine Stelle (ein Treffer, sonst „NON-MEASUREMENT“), führt `npm --prefix web run typecheck` aus (muss Exit 0 sein, sonst „NON-MEASUREMENT“), dann vitest (Exit des Befehls selbst), stellt per `cp` wieder her, `cmp` (Exit 0 in jedem Lauf ausgegeben) und läuft danach grün (Exit 0). Die Testdateien der Aufgabe: Task 1 `NodeConfigDrift.test.tsx`, `api.test.ts`, `api.nulls.test.ts`; Task 2 `ConfigDriftRow.test.tsx`, `clusters.test.tsx`, `NodeCertificates.test.tsx`, `api.nulls.test.ts`; Task 3 `ConfigDriftBanner.test.tsx`, `NodeCertificates.test.tsx`, `Sidebar.test.tsx`.

### Task 1

RED gegen den Stand vor dem Eingriff: Exit 1 (Schemas und Komponente fehlten). Danach grün: 28 Tests in den drei Dateien (`NodeConfigDrift.test.tsx` später 8 Tests).

| Injektion | Test | Exit | scheiternde Zeile |
|---|---|---|---|
| (a) der Knopf sendet `''` statt `data.digest` | `confirms with exactly the digest it shows …` | 1 | `AssertionError: expected "confirm" to be called with arguments: [ 'm-1', …(1) ]` |
| (b) der Knopf auch bei `ok` (Bedingung `drift` oder `ok`) | `confirms with exactly …` und `offers to confirm only while there is a drift` | 1 | `expected <button …> to be null` und `ok: expected <button …> to be null` |
| (c) `not-read` mit dem Text „As last known“ | `never shows a state without a judgement as if it were fine` | 1 | `expected 'ConfigurationAs last knownNothing was…' not to match /as last known/i` |
| (d) der `notice` wird nicht gezeigt | `says that a changed secret value is invisible, in every state` | 1 | `Unable to find an element with the text: A change of nothing but a secret value …` |
| (e) `['clusters']` wird nach dem Bestätigen nicht invalidiert | `confirms with exactly …` | 1 | `expected "invalidateQueries" to be called with arguments: [ { queryKey: [ 'clusters' ] } ]` |
| (f) der Diff zeigt nichts (`slice(0, 0)`) | `shows a drift with its path, its values and since when` | 1 | `Unable to find an element with the text: KubeletConfig.image` |
| (g) `changes` im Schema verliert `.transform(orEmpty)` (Typ per Cast gehalten) | `api.nulls.test.ts` `reads a drift detail with null or missing diff lists as empty lists` | 1 | `AssertionError: expected null to deeply equal []` |

(b) wurde zweimal gelaufen: der erste Lauf traf nur den Bestätigen-Test; der Test `offers to confirm only while there is a drift` kam danach dazu, damit der Knopf in jedem der sieben Zustände einzeln gehalten wird, und (b) wurde wiederholt (beide Zeilen oben).

### Task 2

RED: Exit 1 (Komponente fehlte; der Test in `api.nulls.test.ts` war an dieser Stelle schon grün, weil das Schema im selben Schritt entstand; er ist mit (e) rot gesehen).

| Injektion | Test | Exit | scheiternde Zeile |
|---|---|---|---|
| (a) fehlende `config_drift` heißt „no drift“ | `says "not read yet" when the server sends no config_drift …` und der Test in `clusters.test.tsx` | 1 | `Unable to find an element with the text: not read yet` |
| (b) `unreadable` mit `since` zählt nicht als abweichend | `still counts a node that stopped answering while it drifts …` | 1 | `Unable to find an element with the text: 1 of 3 nodes differs from its baseline: cp-2 (last known)` |
| (c) `not-read` zählt als passend | `lists a node without a judgement … never counts it as matching` | 1 | `Unable to find an element with the text: 1 of 4 nodes match their baseline` |
| (d) der Satz zur Teilantwort entfällt (`partial = false`) | `says how many nodes answered, and that the rest show their last reading` | 1 | `Unable to find an element with the text: 2 of 3 nodes answered; the rest show their last reading.` |
| (e) `items` im Cluster-Schema verliert `.transform(orEmpty)` | `api.nulls.test.ts` `reads a cluster config_drift with null or missing items as an empty list` | 1 | `AssertionError: expected null to deeply equal []` |

(d) wurde wiederholt: der erste Versuch (`{partial && false && (`) brach den Typcheck (Exit 1, `health` möglicherweise null): „NON-MEASUREMENT“, nicht gezählt; die Wiederholung über die Definition von `partial` zählt.

### Task 3

RED: Exit 1 (Komponente fehlte).

| Injektion | Test | Exit | scheiternde Zeile |
|---|---|---|---|
| (a) Banner bei `drifted > 0`, ohne `banner` zu prüfen | `comes only from the server …`, `renders nothing for a cluster without config_drift or with banner false`, `shows one banner for each cluster that has one` | 1 | `expected <div role="status" …> to be null` (und `to have a length of 2 but got 3`) |
| (b) ein Schließen-Knopf mit lokalem Zustand | `cannot be clicked away: the only thing to click is the link to the node` | 1 | `expected [ Array(1) ] to have a length of +0 but got 1` |
| (c) ein eigener Text statt des Satzes des Servers | `shows the sentence of the server, names the cluster and leads to …` | 1 | `Unable to find an element with the text: cp-2 has differed from its baseline for more than a day …` |
| (d) `queryKey: ['clusters', 'config-drift-banner']` | `shares the cluster query with the certificate banner and asks the server once` | 1 | `expected "list" to be called 1 times, but got 2 times` |
| (e, zusätzlich) das Banner führt zum neuesten statt zum ältesten Knoten | derselbe Test wie (c) | 1 | `expect(element).toHaveAttribute("href", "/nodes/m-cp-2")` |
| (f, zusätzlich) `/config/drift/confirm` in `api.ts` heißt `/config/drift-confirmation`; Typcheck Exit 0; `go test ./cmd/holzkube-managerd/ -run TestEveryRouteIsReachableFromTheInterface` | `TestEveryRouteIsReachableFromTheInterface` | 1 | `no file under web/src mentions these routes: POST /api/v1/machines/{id}/config/drift/confirm` (`api.ts` danach mit `cmp` Exit 0 wiederhergestellt, `go test ./cmd/holzkube-managerd/ -count=1` ok) |

(f) ist der Beleg, den der Plan verlangt: ohne die Gruppe in `interfaceInFlight()` meldet der Wächter eine nicht genannte Route, das Löschen weicht die Prüfung also nicht auf.

### Eine Panne des Messverfahrens

Der erste Lauf von `inj.py` (1a) gab die Testdateien in zsh als ein einziges Argument durch (`$T` wird in zsh nicht zerlegt); vitest fand keine Datei und lieferte Exit 1, auch beim „grün nach der Wiederherstellung“. Das ist keine Messung und wurde verworfen; alle Zeilen oben stammen aus Läufen mit einem Array. `NodeConfigDrift.tsx` war nach diesem Lauf `cmp`-gleich wiederhergestellt.

## Abweichungen vom Plan

**1. [Rule 1 - Layout] Der Abschnitt steht zwischen Hardware und der Live-Ausgabe, nicht gleich nach „Version compatibility“.** „Version compatibility“ und „Hardware“ sind im Zweispalten-Raster ein Paar; eine Karte über zwei Spalten dazwischen ließe eine Lücke. Ein Kommentar in `node-detail.tsx` sagt es. Gesehen habe ich das nicht im Browser, nur aus dem Raster gelesen.

**2. [Rule 3] `configDriftStateSchema` steht vor `clusterSchema`, nicht bei den Detail-Schemas.** Das Cluster-Schema braucht es zur Definitionszeit; weiter unten wäre es ein Zugriff vor der Initialisierung. `STATE_LABEL` ist aus `NodeConfigDrift.tsx` exportiert und wird von der Zeile benutzt, damit beide dieselben Worte sagen.

**3. Mehr Tests als der Plan nennt.** `offers to confirm only while there is a drift` (der Knopf in jedem der sieben Zustände), ein Test in `clusters.test.tsx` (die Karte hat die Zeile; die Datei steht nicht in `files` des Plans, ist aber die Stelle der Einbindung), Injektionen (b) wiederholt, 3(e) und 3(f) zusätzlich.

**4. `onSettled` statt `onSuccess` und `onError`.** Gleiche Wirkung, eine Stelle; nach einem 409 zeigt die Seite damit den aktuellen Stand.

**5. Der Banner nennt den Cluster in einer eigenen Zeile** („Cluster homelab.“) unter dem Satz des Servers, damit der Satz unverändert als eigener Absatz steht und exakt prüfbar ist.

Sonst: der Plan wurde ausgeführt wie geschrieben. Das STATE-Kopfstück (Phase 14) ist nicht angefasst.

## Nicht bewiesen

- **Das Aussehen im Browser.** Nichts von dem, was hier gebaut wurde, wurde von einem Browser gerendert: Farben, Umbruch, das Raster der Knotenseite (Abweichung 1), die Schrift.
- **Die Tippziele auf dem Telefon.** Der Layout-Wächter und das Browser-Projekt von vitest liefen hier nicht (kein Chromium). Die Klassen `max-md:min-h-11` stehen am Knopf, an der `summary` der Liste und am Link im Banner, nach dem Muster der Zertifikatszeile; dass die 44 px erreicht werden, hat niemand gemessen. Der jsdom-Test prüft nur, dass die Klasse da ist.
- **Der Weg gegen einen echten Daemon und einen echten Cluster.** Alle Antworten sind Beispiele nach dem Vertrag in `docs/api-contract.md` (Zeitfelder fehlen, wenn leer; kein `diff` außer bei Drift). Dass der Server genau diese Form schickt, halten Pläne 06 und 08, nicht dieser; hier ist es nur gegen die Schemas geparst. Das Sudo-Fenster ist nur über den Namen der Aktion (`challengeRaisedBy`) gesehen, nicht als Dialog in einem Lauf.
- **Die Einbindung in `AppShell`.** Es gibt keinen Test der Hülle (auch für `CertificateBanner` nicht); gehalten wird sie nur durch den Quelltext-Treffer der Abnahme, nicht durch einen Wächter. Entfiele die Zeile, bliebe die Suite grün.
- **Die Einbindung des Abschnitts in `node-detail.tsx`** ist ebenso nur an der Stelle gelesen und vom Typcheck gehalten; kein Test rendert die Knotenseite mit ihm.
- **Der Zweig „Banner an, aber kein Element mit `since`“.** Der Banner zeigt dann den Satz ohne Link; das ist nicht gemessen und kommt vom Server nicht vor.
- **Die Aktualisierung im Hintergrund:** `refetchInterval` (eine Minute am Abschnitt, fünf Minuten am Banner) ist im Code, aber in keinem Test mit abgelaufener Zeit gesehen.
- **`-race`, Pi:** nicht Gegenstand dieses Plans (nur Web-Dateien und eine Go-Testdatei).

## Auth gates

Keine.

## Known Stubs

Keine.

## Threat Flags

Keine über den Plan hinaus. T-15-32 (falsche Zusicherung auf dem Schirm): rot mit 1(b), 1(c), 2(a), 2(c), 2(d); T-15-33 (falscher Stand bestätigt): rot mit 1(a); T-15-34 (Werte im Diff): akzeptiert, Werte werden als Text gesetzt; T-15-SC: nichts installiert.

## Self-Check: PASSED

Dateien vorhanden (`NodeConfigDrift.tsx` und Test, `ConfigDriftRow.tsx` und Test, `ConfigDriftBanner.tsx` und Test; geändert `api.ts`, `api.test.ts`, `api.nulls.test.ts`, `node-detail.tsx`, `clusters.tsx`, `clusters.test.tsx`, `AppShell.tsx`, `reachable_test.go`); Commits 7f8b3b4, e5f8cc3 und cba79f3 sind Vorfahren von HEAD.
