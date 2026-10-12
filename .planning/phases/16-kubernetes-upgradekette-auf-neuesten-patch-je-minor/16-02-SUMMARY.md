---
phase: 16-kubernetes-upgradekette-auf-neuesten-patch-je-minor
plan: 02
subsystem: internal/k8srelease
tags: [kubernetes, upgrade-chain, release-source, dl-k8s-io, budget-guard, wiring-guard]
status: complete
requires:
  - "Plan 01: upgrade.KubernetesReleaseSource, Service.WithKubernetesReleases, kubernetesMachine / newKubernetesPlanService"
  - "compat-Matrix-Quick-Task (D-03), BASE aus 16-01-SUMMARY.md"
provides:
  - "internal/k8srelease: Client, New, Newest, DefaultBaseURL, Budget; die streng validierte, gleichzeitige Abfrage von stable-1.N.txt"
  - "Im Daemon fest verdrahtet: k8srelease.New(k8srelease.DefaultBaseURL) und WithKubernetesReleases(client.Newest); kein Flag, keine Variable, kein Konfigurationsfeld"
  - "Budget-Tabelle: die Aufrufklasse kubernetes-releases in den drei Routen, die PlanKubernetes rufen, samt Waechter fuer die Deklaration"
  - "Syntaxbaum-Waechter fuer die Verdrahtung in main.go"
  - "README und docs/guide.md im selben Aenderungsschritt"
  - "Ledger 175 (offen): keine Live-Pruefung des dl.k8s.io-Formats"
affects:
  - "Phasen-Verifikation (K8S-01), Release (Changelog-Eintrag gehoert dem Release)"
tech-stack:
  added: []
  patterns:
    - "Upstream-Client nach dem Muster internal/imagefactory (New, Optionen, Weiterleitungsregel), Frist aus dem Context"
    - "errgroup fuer die gleichzeitige Abfrage, ein gemeinsamer Deadline"
    - "Transport-Fake liest die Frist des Requests: Budget ohne Wartezeit und ohne Naht im Produktionscode belegt"
key-files:
  created:
    - internal/k8srelease/k8srelease.go
    - internal/k8srelease/k8srelease_test.go
    - internal/upgrade/kubernetes_source_test.go
    - cmd/holzkube-managerd/k8sreleases_wiring_test.go
  modified:
    - cmd/holzkube-managerd/main.go
    - cmd/holzkube-managerd/budget_test.go
    - README.md
    - docs/guide.md
    - .planning/WINDOWS.md
key-decisions:
  - "Die Antwortform ist ein einziger regulaerer Ausdruck ^v1\\.N\\.[0-9]+$ mit der angefragten Minor; Pre-Release, andere Minor, zweite Zeile und alles andere fallen aus der Form, nicht aus einer eigenen Pruefung"
  - "Kein Fehlertext nennt eine Adresse oder ein Byte der Antwort: auch die Weiterleitungsfehler sagen nur 'another host' / 'the scheme', ohne Ziel"
  - "Die Weiterleitungsregel steht hier neu, nicht aus imagefactory exportiert (dort unexportiert, die Waechter jenes Pakets bleiben unberuehrt); hoechstens drei Spruenge, gleicher Host, gleiches Schema"
  - "Die Deklaration in der Budget-Tabelle ist ein gemeinsamer Wert kubernetesReleasesLookup, damit die drei Zeilen nicht auseinanderlaufen"
metrics:
  duration: "ca. 1 h 15 min (geschaetzt; die Startzeit wurde nicht aufgenommen)"
  completed: 2026-10-12
  tasks: 3
actuals:
  tokens: 13000
  tasks: 3
  commits: 3
commits: 3
plan_head_before: 215716e2ec7fdb9fd9bb16a7f9e9b742dc959be4
plan_head_after: 6ca21c60f2aae9eb6ccaf1286a44a573c04c0ecf
---

# Phase 16 Plan 02: Die Quelle dl.k8s.io, verdrahtet und bewacht Summary

Der Daemon liest den neuesten stabilen Patch jeder Zwischenminor von `https://dl.k8s.io/release/stable-1.N.txt` (nie `latest-1.N.txt`), streng validiert, gleichzeitig unter einem gemeinsamen Deadline und ohne jede Einstellung; die Budget-Tabelle, ein Syntaxbaum-Waechter, README und Guide sagen es, und ein offener Ledger-Eintrag sagt, dass es fuer das Format keine Live-Pruefung gibt.

## Wo es lief

Mac (darwin/arm64), Go 1.27.2 mit `GOTOOLCHAIN=local` (das Projekt pinnt 1.26.9), kein echter Cluster, kein Pi. Auf `main` gearbeitet, wie der Betreiber es angeordnet hat; die Schutzpruefung "HEAD nicht auf dem Default-Branch" der Commit-Routine wurde deshalb bewusst nicht angewandt (wie in Plan 01). Nicht gepusht, nicht getaggt, nichts veroeffentlicht. `internal/changelog`, `internal/compat` und `internal/config` sind gegen BASE unberuehrt (`git diff --name-only BASE -- ...` leer, nach jedem Task geprueft).

## Was gebaut wurde

- **`internal/k8srelease/k8srelease.go`** (neu): Paketdoc mit dem gemessenen Vertrag (2026-10-12, curl) und der Begruendung, warum nie `latest-1.N.txt` (`latest-1.37.txt` lieferte `v1.37.0-rc.1`). `DefaultBaseURL = "https://dl.k8s.io/release"`, `Budget = 10 * time.Second` (ein Deadline ueber alle gleichzeitigen Anfragen eines Aufrufs; Begruendung aus den fuenf gemessenen Zeiten von 0,06 bis 0,24 s), Deckel von 64 Byte. `New` prueft Schema und Host; `WithHTTPClient` kopiert den Client, leert dessen Timeout und setzt die Weiterleitungsregel. `Newest` loest doppelte Minors auf, lehnt negative ab, fragt je Minor gleichzeitig (errgroup) unter dem frueheren von Aufrufer-Frist und `Budget`, nimmt nur Status 200, liest hoechstens 65 Byte, und akzeptiert nach `TrimSpace` nur `^v1\.N\.[0-9]+$`. Jeder Fehler nennt die Datei (`stable-1.N.txt`) und die Art; der Statuscode zaehlt fuer sich (der Koerper eines 404 wird nicht gelesen); kein Fehlertext gibt ein Byte der Antwort oder eine Adresse aus der Antwort wieder. Das Paket importiert `internal/upgrade` nicht (`go list -deps ./internal/upgrade/ | grep -c k8srelease` ergibt 0).
- **`cmd/holzkube-managerd/main.go`**: vor dem Bau von `upgradeSvc` `k8srelease.New(k8srelease.DefaultBaseURL)` (Fehler mit Hinweis auf den Lookup), an der Kette `.WithKubernetesReleases(k8sReleases.Newest)`. Nichts aus `cfg`; ein Lesezugriff, laeuft also auch in `--dry-run`. Damit ist der in Plan 01 genannte Zustand "Daemon ohne Quelle, jede Mehr-Minor-Kette blockiert" beendet.
- **Tests**: `k8srelease_test.go` (Tabelle der Antworten mit elf Zeilen, Verbindungsabbruch, Marker, Gleichzeitigkeit, eine Minor von zwei scheitert, Frist per Transport-Fake in zwei Faellen, Weiterleitungen in vier Faellen, leere Liste, doppelte Minor, negative Minor, feste Basis), `kubernetes_source_test.go` (echter Client -> Service -> Kette gegen einen Fake-Server: `v1.33.7, v1.34.1`; 404 mit XML blockiert den Plan, `BlockReason` nennt `stable-1.33.txt` und `404`, Kette leer).
- **`cmd/holzkube-managerd/budget_test.go`**: neue Aufrufklasse `kubernetesReleasesCall` ("kubernetes-releases"), gelesen aus `k8srelease.Budget`, als gemeinsamer Wert `kubernetesReleasesLookup` in die drei Zeilen `upgrade/kubernetes/plan`, `upgrade/kubernetes`, `upgrade/confirm`; Pin auf `k8srelease.Budget` bei zehn Sekunden; neuer Test `TestTheKubernetesPlanRoutesDeclareTheReleaseLookup`. Die geloggte Rechnung gelesen, nicht geraten: die zwei Kubernetes-Zeilen kommen auf 45 s gegen 120 s (uncut, wie deklariert), die Confirm-Zeile bleibt clipped; die Urteile mussten nicht geaendert werden.
- **`cmd/holzkube-managerd/k8sreleases_wiring_test.go`** (neu): parst `main.go` mit `go/parser` und prueft am Syntaxbaum (Import da, genau ein `k8srelease.New` mit dem Selektor `k8srelease.DefaultBaseURL` als erstem Argument, genau ein `WithKubernetesReleases` mit einem Argument, das `Newest` selektiert).
- **README** (ein Halbsatz im Lifecycle-Punkt, `newest patch` genau einmal) und **`docs/guide.md`** (drei Absaetze im Abschnitt "Upgrading Kubernetes", vor "This has run against the simulator ..."; `The source has no setting` genau einmal; keine Zeile in der Optionstabelle; der Schlusssatz des Abschnitts unveraendert). Kein Screenshot, keine Fixture, kein Changelog-Eintrag. **Fuer den README-Halbsatz und die Guide-Absaetze gibt es keinen Waechter ausser den Greps der Verifikation.**
- **`.planning/WINDOWS.md`**: Eintrag 175 (offen, `unrun-verify`, beginnt mit `FUER DIE FORM VON dl.k8s.io/release/stable-1.N.txt GIBT ES KEINE LIVE-PRUEFUNG`), beide Haelften, `open_count` 21 auf 22, `total_count` 174 auf 175; `go test ./internal/planning/` gruen. Von Hand statt mit `gsd-tools windows append`: das Werkzeug lehnt wegen der vorhandenen Eintraege mit Arten ausserhalb seines Vokabulars jeden Anhang ab (siehe Plan 01, Abweichung 5). Die Platzhalter `<patch>` des Plantextes stehen als `PATCH` darin, weil `<patch>` in der gerenderten Tabelle als HTML-Tag verschluckt wuerde.

## Rote Laeufe und Injektionen (Exitcode des Befehls selbst)

Verfahren je Injektion: Sicherung ausserhalb des Repos, genau eine Aenderung (ein Hilfsskript bricht ab, wenn der Suchtext nicht genau einmal vorkommt, dann gilt der Versuch als Nicht-Messung), Testbefehl, Exitcode, Wiederherstellung per `cmp` Byte fuer Byte, danach gruen (Exit 0). Alle unten genannten Wiederherstellungen wurden mit `cmp` bestaetigt und liefen danach gruen. **Kein Versuch war ein Nicht-Treffer oder nur ein Build-Fehler**; wo eine Injektion zwei Stellen brauchte, um zu uebersetzen ((iii), (v)), ist das so gemacht (sonst "declared and not used", ein Compile-Rot und keine Messung).

**Task 1, ROT vor dem Eingriff:** `go test ./internal/k8srelease/ ./internal/upgrade/ -run 'TestNewestAsksTheStableList|TestTheRealSource'` Exit 1 (Build-Fehler: das Paket gibt es noch nicht). Das ist ein Compile-Rot, kein Verhaltens-Rot; das Verhalten belegt die Injektion.

| | Eingriff | Exit | scheiternde Zeile |
|---|---|---|---|
| T1 | URL fragt `latest-1.N.txt` statt `stable-1.N.txt` | 1 (beide Pakete) | `k8srelease_test.go:61: Newest: k8srelease: latest-1.34.txt: the source answered 404`; `kubernetes_source_test.go:42: the chain is [], want [v1.33.7 v1.34.1] (blocked=true ... latest-1.33.txt: the answer is not a stable v1.33 release ...)`; `:69: the block reason ... does not mention "stable-1.33.txt"`. Der Pfadvergleich des ersten Tests (`asked()`) wurde dabei nicht erreicht, weil der Test schon am 404 der nicht vorhandenen `latest-1.34.txt` abbrach; rot ist er trotzdem |

**Task 2, ROT gegen den Stand von Task 1:** keiner der neuen Tests war rot (`go test ./internal/k8srelease/ -count=1 -timeout 120s -v` Exit 0). Grund: Task 1 hat die strenge Form, die Statuspruefung, die Gleichzeitigkeit, den Deadline und die Weiterleitungsregel schon als Ganzes gebaut; sie gehoeren in dieselbe Funktion, und Task 2 hatte im Produktionscode nichts mehr zu bauen. Die Aussagekraft der Tests kommt allein aus den Injektionen (Abweichung 1).

| | Eingriff | Exit | scheiternde Zeile |
|---|---|---|---|
| (a) | Form laesst ein Pre-Release-Suffix zu | 1 | `.../a_pre-release: Newest accepted status 200 with body "v1.35.0-rc.1"` |
| (b) | Form nimmt jede Minor | 1 | `.../another_minor: Newest accepted status 200 with body "v1.36.0": map[35:v1.36.0]` |
| (c) | `context.WithTimeout(ctx, Budget)` durch `WithCancel` ersetzt | 1 | `TestNewestAppliesTheBudgetAsADeadline/a_caller_without_a_deadline_gets_the_budget: 2 requests carried no deadline, 0 did (want 0 and 2)`; ohne Wartezeit; der zweite Fall (Aufrufer mit einer Sekunde) blieb gruen, wie er soll |
| (d) | `g.SetLimit(1)`: nacheinander | 1 | `TestNewestAsksEveryMinorAtOnce (6.01s): ... stable-1.33.txt: the source answered 503` (nach der Sperrfrist des Handlers) |
| (e) | Weiterleitungsregel prueft den Host nicht | 1 | `TestNewestRefusesARedirectToAnotherHost: Newest followed a redirect to another host and answered map[35:v1.35.9]` |
| (f) | Fehlertext haengt den Koerper an | 1 | `TestNoErrorRepeatsWhatTheSourceAnswered/200_and_no_version` und `/a_version_that_is_a_lie: the error repeats the answer: ...: "BODY-MARKER-7f3a"` |
| (g) | Statuspruefung entfaellt | 1 | `.../not_found_with_a_plausible_body: Newest accepted status 404 with body "v1.35.9"`; auch `server_error` (500) und, anders als der Plan erwartete, die Zeile `not_found_with_an_XML_body` (die Antwort scheitert dort am Deckel, nennt aber nicht `404`) |
| (h) | `DefaultBaseURL` auf `http://` | 1 | `TestTheDefaultSourceIsTheFixedHttpsHost: DefaultBaseURL is "http://dl.k8s.io/release", want https on dl.k8s.io` |

Dazu sechs Injektionen, die der Plan nicht nannte, fuer Code, den ich gebaut und getestet habe (sonst waere er ungepruefter Produktionscode):

| | Eingriff | Exit | scheiternde Zeile |
|---|---|---|---|
| (i) | Deckel von 64 Byte auf `1 << 20` | 1 | `.../too_long: Newest accepted status 200 with body "v1.35.111...1"` |
| (j) | Sprunggrenze auf 1000 | 1 | `TestNewestRefusesALongChainOfRedirects: four redirects were followed and answered map[35:v1.35.9]` |
| (k) | Schemawechsel erlaubt | 1 | `TestNewestRefusesARedirectFromHttpsToHttp: the transport saw 4 requests, want 1` |
| (l) | negative Minor erlaubt | 1 | `TestNewestRefusesAMinorThatCannotBeOne: the source was asked 2 times before the minors were checked` |
| (m) | Fehler einer Minor verschluckt (`return nil`) | 1 | vier Tests: `TestNewestFailsWhenOneOfTheMinorsFailsAndSaysWhich: Newest answered map[33:v1.33.7] although 1.34 has no list`, Verbindungsabbruch, beide Weiterleitungsfaelle |
| (n) | keine Entdopplung | 1 | `TestNewestAsksOnceForAMinorNamedTwice: the source was asked 2 times for one minor` |

Task 2 traegt damit vierzehn Injektionen statt der acht des Plans.

**Task 3** (Testbefehl `go test ./cmd/holzkube-managerd/ -run 'Budget|TheDaemonWires|TestEveryRouteThatReaches|TestTheKubernetesPlanRoutesDeclare'`):

| | Eingriff | Exit | scheiternde Zeile / was gruen blieb |
|---|---|---|---|
| (i) | Deklaration aus der Zeile `upgrade/kubernetes/plan` entfernt | 1 | `TestTheKubernetesPlanRoutesDeclareTheReleaseLookup` rot. **Gruen blieben `TestEveryRouteThatReachesUpstreamHasABudgetRow`, `TestRouteBudgetsComposeAgainstWriteTimeout` und `TestRouteBudgetTableReadsTheRealConstants`.** |
| (i) | ebenso aus `upgrade/kubernetes` | 1 | derselbe Befund: nur der neue Test rot, die drei anderen gruen |
| (i) | ebenso aus `upgrade/confirm` | 1 | `budget_test.go:1973: POST /api/v1/clusters/{id}/upgrade/confirm builds a Kubernetes plan ... its row declares no call of the class "kubernetes-releases". The table is declared by hand and an omitted call changes no sum ...`; die drei anderen gruen |
| (ii) | `k8srelease.Budget` auf 130 s | 1 | `TestRouteBudgetTableReadsTheRealConstants` rot (der Pin), und `TestRouteBudgetsComposeAgainstWriteTimeout` rot fuer alle drei Zeilen: `... computes to "known over budget" but the row declares "within budget"` (R2: Deadline 120 s unter dem groessten Aufruf) und `... computes to "clipped" but the row declares "uncut": the declared calls sum to 2m45s against a routeDeadline of 2m0s` |
| (iii) | `WithKubernetesReleases` aus `main.go` gestrichen (mit `_ = k8sReleases`, damit es uebersetzt) | 1 | `k8sreleases_wiring_test.go:81: main.go calls WithKubernetesReleases 0 times, want exactly once` |
| (iv) | `k8srelease.New("https://dl.k8s.io/release")` | 1 | `k8sreleases_wiring_test.go:75: the first argument of k8srelease.New at main.go:510:22 is not the constant k8srelease.DefaultBaseURL ...` |
| (iv-b) | `k8srelease.New(cfg.ImageFactoryURL)` (ein Wert aus der Konfiguration) | 1 | dieselbe Zeile |
| (v) | `WithKubernetesReleases(nil)` (mit `_ = k8sReleases`) | 1 | `k8sreleases_wiring_test.go:89: the argument of WithKubernetesReleases at main.go:519:16 is not the Newest method of the client ...` |

**Was bei (i) gruen blieb, und warum die Recherche damit falsch lag:** die Recherche vermutete, das Weglassen des Aufrufs mache den Vollstaendigkeitswaechter rot. Gesehen wurde das Gegenteil: `TestEveryRouteThatReachesUpstreamHasABudgetRow` fragt nur, ob die Route eine Zeile hat, nicht was in ihr steht, und die Rechnung bleibt ohne den Aufruf unter dem Budget (45 s gegen 120 s). Allein der neue Test haelt die Deklaration.

## Verifikation (gelaufen)

- `go test ./internal/k8srelease/ ./internal/upgrade/ -count=1`: Exit 0, `go test ./internal/k8srelease/ -v`: 15 `--- PASS` auf oberster Ebene, kein SKIP, darunter `TestNewestAsksEveryMinorAtOnce`, `TestNewestAppliesTheBudgetAsADeadline`, `TestTheDefaultSourceIsTheFixedHttpsHost`.
- `go test ./cmd/holzkube-managerd/ ./internal/config/ ./internal/host/... ./internal/planning/ ./internal/publicrepo/ ./internal/ -count=1`: Exit 0.
- `go test ./internal/httpapi/ -run 'TestTheKubernetesPlanRoute|TestAKubernetesPlanWhose|TestSubmittingAKubernetesUpgrade|TestAPlan|TestSubmittingABlocked' -count=1`: Exit 0.
- `go build ./... && go vet ./... && test -z "$(gofmt -l .)"`: Exit 0. `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /dev/null ./cmd/holzkube-managerd/`: Exit 0 (belegt die Uebersetzung fuer den Pi, keinen Lauf dort).
- Der volle Lauf `go test ./internal/... ./cmd/... -count=1` war **ohne eingebettetes Web-Bundle** (kein `task build`, kein Node geprueft): rot sind in `internal/httpapi` genau die vier erwarteten Tests (`TestCSPHashMatchesTheServedShell`, `TestFontsAreServedAsFiles`, `TestForeignHostIsRefused`, `TestNoAssetIsInlinedPastTheContentSecurityPolicy`), und einmal `internal/power` `TestADisabledNodeStaysOffUntilItIsEnabled` (`jobs: another job is already running on this cluster`). Letzterer ist ein Zeitverhalten unter der Last des vollen Laufs: allein dreimal Exit 0, das ganze Paket `internal/power` danach Exit 0; das Paket ist von dieser Phase nicht beruehrt. Alle uebrigen 41 Pakete `ok`.
- Zusaetzlich, ohne Beleg fuer CI: `go test -race ./internal/k8srelease/ -count=1` lief hier ueberraschend Exit 0 (darwin/arm64, Race-Detektor lief). Das ersetzt `go test -race` der CI fuer das ganze Repository nicht und wurde fuer kein anderes Paket gemacht.
- Der Linter (`golangci-lint`, `./bin/task lint:go`) ist auf diesem Rechner nicht installiert und lief **nicht**. Gegen errcheck, gosec, bodyclose, contextcheck, errorlint, nilerr und revive ist `k8srelease.go` also nur von Hand geschrieben (Body per `defer func() { _ = ...Close() }()`, kein `http.Get`, Fehler mit `%w`), nicht gemessen.

## Abweichungen vom Plan

1. **[Reihenfolge] Task 2 hatte kein Rot zu bauen.** Wie in Plan 01: die Randfaelle gehoerten in dieselbe Funktion wie der erste Lookup und waren mit Task 1 schon gebaut; alle Task-2-Tests liefen beim ersten Mal gruen. Aussagekraft allein aus den Injektionen.
2. **[Rule 2 - ungepruefter Code] sechs Zusatz-Injektionen (i) bis (n) in Task 2.** Deckel, Sprungzahl, Schemawechsel, negative Minor, verschluckter Fehler und Entdopplung sind Produktionscode dieses Plans; ohne eigenen Test und eigene Injektion waeren sie behauptet und nicht gemessen. Zusatztests dafuer: `TestNewestRefusesALongChainOfRedirects`, `TestNewestRefusesARedirectFromHttpsToHttp`, `TestNewestRefusesAMinorThatCannotBeOne`, `TestNewestAsksOnceForAMinorNamedTwice`, `TestNewestFailsWhenOneOfTheMinorsFailsAndSaysWhich`.
3. **[Erwartung] Injektion (g).** Der Plan erwartete, ohne die Statuszeile bleibe die Zeile mit dem 404 und XML-Koerper gruen. Sie wurde rot (der XML-Koerper ist laenger als 64 Byte, die Meldung sagt "answer too long" statt `404`); der Test haelt also beide, aber die Zeile mit dem plausiblen Koerper (404 und 500) ist die, die die Statuspruefung allein haelt.
4. **[Erwartung] Injektion (i) in Task 3.** Siehe oben: die Recherche irrte, nur der neue Test wird rot.
5. **[Messgrenze] "Eine haengende Quelle" ist nicht als haengender Server gesehen.** Der Plan nennt sie in der Annahmeregel des Clients. Belegt ist, dass die Frist `Budget` auf jeder Anfrage steht (Transport-Fake liest `req.Context().Deadline()`), nicht dass ein wirklich schweigender Server nach zehn Sekunden zu einem Fehler fuehrt. Ein solcher Test haette zehn Sekunden gewartet oder eine Test-Naht im Produktionscode gebraucht, beides ist ausdruecklich ausgeschlossen (Entscheidung des Betreibers, Plan). `context.WithTimeout` und `net/http` tun das, aber das haelt hier kein Test.
6. **[Werkzeug] Ledger von Hand,** und `<patch>` als `PATCH` geschrieben. Siehe oben.
7. **[Shell] zsh kennt `PIPESTATUS` nicht;** der erste ROT-Lauf zeigte deshalb ein leeres `exit=`, wurde wiederholt und mit Exit 1 festgehalten.

## Wichtig fuer das, was als Naechstes kommt

K8S-01 ist nach den `must_haves` dieses Plans erfuellt (der Daemon traegt die feste Quelle, die Quelle ist gegen jede Form einzeln rot gesehen, die drei Routen sind deklariert und bewacht, README und Guide sind im selben Schritt, Ledger 175 steht). Auf Anweisung des Auftraggebers ist es in REQUIREMENTS.md und ROADMAP.md abgehakt; die Phasen-Verifikation steht trotzdem aus. Der Changelog-Eintrag fuer das Release ist nicht geschrieben (er gehoert dem Release, `/release` verweigert ein Tag ohne ihn). Ein Mehr-Minor-Ziel wird weiter nicht blockiert (Ledger 174, D-05): was die Kette zeigt, ist ein Hinweis, der Lauf installiert das getippte Ziel in einem Lauf.

## Was NICHT bewiesen ist

Nur gegen Simulatoren und `httptest` belegt. Nicht belegbar:

- **die Dauerhaftigkeit des Formats von dl.k8s.io**: heute einmal gemessen (2026-10-12, curl), kein Vertragsdokument, keine Live-Pruefung, offener Ledger-Eintrag 175; Entscheidung des Betreibers: nur von Hand. Ein geaendertes Format zeigt sich als blockierter Plan, nie als falscher Schritt, aber der Betreiber merkt es erst, wenn er eine Kette plant.
- **die Annahme des Zwischenpatches durch einen echten Cluster** (das Prepull-Image und das Kubelet kennt nur der echte Knoten);
- **ein Lauf auf dem Pi (linux/arm64)**: gelaufen ist darwin/arm64 gegen Fakes; fuer linux/arm64 ist nur die Uebersetzung belegt;
- **ein wirklich haengender Server** (siehe Abweichung 5) und **TLS gegen den echten Host** (alle Tests laufen ueber `http://127.0.0.1` oder einen Transport-Fake; die Pruefung des Zertifikats ist die der Standardbibliothek und hier nie abgeschaltet, aber nicht gegen `dl.k8s.io` gesehen).

Nicht gelaufen: `go test -race` fuer das ganze Repository (CI allein; fuer `internal/k8srelease` einmal lokal gelaufen, siehe oben), `task ci`, der Layout-Waechter, der Linter, und die vier Web-Bundle-Tests von `internal/httpapi`.

## Known Stubs

Keine.

## Threat Flags

Keine neue Flaeche ausserhalb des Threat-Models. T-16-06 (HTTPS, feste Konstante, strenge Form): Injektionen (a), (b), (h), (iv), (iv-b). T-16-07 (Weiterleitung): (e), (j), (k). T-16-08 (haengende Quelle, Deckel, Budget-Tabelle): (c), (i), (ii) und der Waechter der Deklaration; die haengende Quelle selbst siehe Abweichung 5. T-16-09 (kein Koerper im Fehlertext): (f). T-16-10 und T-16-11 sind akzeptiert und im Guide gesagt.

## Commits

- `1ef9cd0` feat(16-02): the daemon asks dl.k8s.io for the newest stable patch of each minor on the way
- `d483f92` test(16-02): the hostile and the broken answer of the release source, each seen red against its fault
- `6ca21c6` docs(16-02): the budget table and the wiring hold the release lookup, and README and guide say what changes

## Self-Check: PASSED

Alle neun genannten Dateien vorhanden, die drei Commits (`1ef9cd0`, `d483f92`, `6ca21c6`) sind Vorfahren von HEAD.
