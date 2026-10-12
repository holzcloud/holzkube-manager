---
phase: 16-kubernetes-upgradekette-auf-neuesten-patch-je-minor
plan: 01
subsystem: internal/upgrade
tags: [kubernetes, upgrade-chain, plan, release-source, simulator]
status: complete
requires:
  - "compat-Matrix-Quick-Task (D-03): Talos 1.14 traegt Kubernetes bis 1.37"
provides:
  - "KubernetesChain(from, to, talosVersion, newest map[int]Version) mit Patch aus der uebergebenen Menge"
  - "upgrade.KubernetesReleaseSource und Service.WithKubernetesReleases (die Naht, die Plan 02 fuellt)"
  - "MaxKubernetesIntermediates = 16 und die gemeinsame Vorpruefung kubernetesChainPrecheck"
  - "Blockade bei Ausfall der Quelle, mit einem Satz; kein Rueckfall auf die erste Version einer Minor"
  - "Hilfen kubernetesMachine / newKubernetesPlanService fuer Plan 02"
affects:
  - "Plan 02: echte Quelle dl.k8s.io, Verdrahtung im Daemon, Budget-Waechter, README und Guide"
tech-stack:
  added: []
  patterns:
    - "Kette als reine Funktion ueber eine uebergebene Menge (wie Chain)"
    - "Quelle als Funktion, Builder nach dem Muster WithSnapshots"
    - "blockierter Plan statt Fehler (nilerr-Muster aus PlanKubernetes)"
key-files:
  created:
    - internal/upgrade/kubernetes_plan_test.go
  modified:
    - internal/upgrade/plan.go
    - internal/upgrade/service.go
    - internal/upgrade/plan_test.go
    - internal/httpapi/upgradeapi_test.go
    - docs/api-contract.md
    - .planning/WINDOWS.md
key-decisions:
  - "Die Kappe und die gemeinsame Vorpruefung sind schon in Task 1 gebaut worden, weil sie in dieselbe Funktion gehoeren; Task 2 hat sie nur noch durch Injektionen belegt"
  - "Die Antwort der Quelle muss genau drei Zahlenteile haben: ParseVersion liest 'v1.33' als 1.33.0 und das waere ein stiller Patch null"
  - "Ohne Quelle (nil) ist eine Kette mit Zwischenschritten blockiert, ein Ziel in der naechsten Minor bleibt planbar"
metrics:
  duration: "ca. 1 h 15 min (Mac, ohne Cluster)"
  completed: 2026-10-12
  tasks: 3
actuals:
  tokens: 9300
  tasks: 3
  commits: 3
commits: 3
plan_head_before: d30fcb58cba394168c01c6d74794b16751baa399
plan_head_after: c38d60bc00c40212370bcdd5592992a9f3af83c7
---

# Phase 16 Plan 01: Kubernetes-Kette auf dem neuesten Patch je Minor Summary

Die Zwischenschritte der Kubernetes-Upgradekette tragen den neuesten stabilen Patch ihrer Minor aus einer uebergebenen Quelle statt `.0`; faellt die Quelle aus, antwortet Muell oder fehlt sie, ist der Plan mit einem Satz blockiert und nie auf die erste Version einer Minor zurueckgefallen.

BASE: d30fcb58cba394168c01c6d74794b16751baa399

(BASE ist der Stand, der den compat-Quick-Task schon enthaelt, genommen vor der ersten Aenderung. Plan 02 liest ihn von hier. Gegen BASE ist `git diff --name-only BASE -- internal/compat internal/changelog internal/config` leer.)

## Wo es lief

Mac (darwin/arm64), Go 1.27.2 mit `GOTOOLCHAIN=local` (das Projekt pinnt 1.26.9), kein echter Cluster, kein Pi, kein `go test -race`. Nicht gepusht, nicht getaggt, nichts veroeffentlicht. Auf `main` gearbeitet, wie der Betreiber es angeordnet hat; die Schutzpruefung "HEAD nicht auf dem Default-Branch" der Commit-Routine wurde deshalb bewusst nicht angewandt.

## Was gebaut wurde

- `internal/upgrade/plan.go`: `KubernetesChain` bekommt `newest map[int]Version`. Abwaerts und Gleichstand wie bisher, danach das Fenster der laufenden Talos-Version fuer das Ziel, danach die Kappe, danach pro Zwischenminor der Eintrag aus `newest`; fehlt er, ist es ein `ErrNoPath` mit Nennung der Minor, und die Funktion setzt nie von sich aus eine Version ein. Der Endschritt ist das Ziel (`The version you chose.`); bei gleicher Minor bleibt die Kette leer. Vorpruefung (`kubernetesChainPrecheck`) und Zwischenminors samt Kappe (`kubernetesIntermediateMinors`, zaehlt vor dem Bauen) sind unexportiert und werden von `PlanKubernetes` genauso gerufen. Der Why-Text nennt jetzt den wirklichen Grund (kube-apiserver darf beim Upgrade keine Minor ueberspringen), sagt "newest stable patch" und enthaelt `on the way`; der falsche Satz zur Skew-Policy ist weg.
- `internal/upgrade/service.go`: `KubernetesReleaseSource func(ctx, minors []int) (map[int]string, error)`, `WithKubernetesReleases`, und in `PlanKubernetes` die Reihenfolge Vorpruefung, Zwischenminors samt Kappe, Quelle (genau ein Aufruf mit allen Zwischenminors, nur wenn es welche gibt), service-seitige Pruefung jeder Antwort (Eintrag da, lesbar, genau drei Zahlenteile, Major wie das Ziel, Minor wie angefragt, kein Pre-Release), dann `KubernetesChain`. Jeder Fehlschlag ist ein blockierter Plan: "The newest patch of 1.N could not be determined: <Ursache>. No step is guessed in its place. A target in v1.M needs no lookup and can still be planned." Ohne Quelle steht in der Ursache `this instance has no source of Kubernetes releases`.
- Tests: `plan_test.go` (fuenf Kettentests, darunter der gegen die Spitze von Talos 1.14, gerechnet aus `compat`), `kubernetes_plan_test.go` (neu: die Hilfen `kubernetesMachine` und `newKubernetesPlanService` plus sieben Service-Tests), `upgradeapi_test.go` (Pruefstand mit `upgradeHarnessOptions`, drei Routentests ueber den echten Router gegen talossim mit Fake-Quelle).
- `docs/api-contract.md`: neuer Abschnitt "The Kubernetes plan: the chain is advice" (die Route war im Vertrag bisher nicht beschrieben).
- `.planning/WINDOWS.md`: Eintrag 174 (offen): die Kette ist ein Hinweis, und die Folge aus D-02. Zaehlung 21 offen / 174 gesamt.

## Rote Laeufe und Injektionen (Exitcode des Befehls selbst)

**Task 1, ROT vor dem Eingriff:** `go test ./internal/upgrade/ -run 'TestTheKubernetesChain|TestAKubernetesChain'` Exit 1 (Build-Fehler: `too many arguments in call to upgrade.KubernetesChain`), `go test ./internal/httpapi/ -run TestTheKubernetesPlanRoute` Exit 1 (Build-Fehler: `undefined: upgrade.KubernetesReleaseSource`). Das ist ein Compile-Rot, wie der Plan es zaehlt, noch kein Verhaltens-Rot; das Verhalten belegen die Injektionen.

**Task 1, Injektionen** (gesichert, eine Aenderung, Befehl, Exitcode, Wiederherstellung per `cmp`, danach gruen):

| | Eingriff | Exit | scheiternde Zeile |
|---|---|---|---|
| (a) | Zwischenschritt aus der Minor ohne den Patch der Quelle gebaut (Patch 0) | upgrade 1, httpapi 1 | `plan_test.go:195: the chain is v1.33.0 then v1.34.1, want v1.33.7 then v1.34.1`; `upgradeapi_test.go:674: the step on the way is v1.33.0, want v1.33.7` |
| (b) | `PlanKubernetes` reicht eine leere Menge an die Kette | httpapi 1 | `upgradeapi_test.go:667: the plan is blocked: ... has to pass through 1.33, and its newest patch is not known` |

Zwei Fehlversuche sind keine Messung und stehen hier, damit sie nicht als Beleg zaehlen: (a) im ersten Anlauf injizierte nichts (die Zeile kam zweimal vor, das Hilfsskript brach ab, beide Pakete blieben gruen); (b) im ersten Anlauf war es ein Build-Fehler (`newest` unbenutzt) und kein Test-Rot. Beide wurden mit genauerem Eingriff wiederholt.

**Task 2, ROT gegen den Stand von Task 1:** keiner der neuen Tests war rot. Grund: die Kappe, die gemeinsame Vorpruefung und die nil-Quelle waren schon in Task 1 gebaut (sie gehoeren in dieselben Funktionen), die plan.go/service.go-Aenderungen, die Task 2 laut Plan noch nennt, entfielen. Alle sieben Service-Tests und die zwei neuen Routentests liefen beim ersten Mal gruen. Ihre Aussagekraft kommt allein aus den Injektionen:

| | Eingriff | Exit | scheiternde Zeile |
|---|---|---|---|
| (a) | Fehlerzweig setzt die erste Version je Minor ein und macht weiter | upgrade 1, httpapi 1 | `kubernetes_plan_test.go:165: a blocked plan still carries a chain: [v1.33.0 v1.34.1]`; `upgradeapi_test.go:713: a plan whose lookup failed was not blocked`; `upgradeapi_test.go:758: submit with a failed lookup answered 403 (...)` |
| (b) | Quelle auch bei Einzelschritt-Zielen gefragt (frueher Rueckkehr bei null Minors entfernt) | upgrade 1 | `kubernetes_plan_test.go:262: the source was asked [[] []] for targets with nothing on the way`; `:235` und `:243` im Nil-Quellen-Test |
| (c) | Quelle vor der gemeinsamen Vorpruefung gefragt | upgrade 1 | `kubernetes_plan_test.go:292: the source was asked [[32 33 34 35 36 37 38 39]] about a target that cannot be taken`; auch `:138: the source was asked 2 times` |
| (d) | Pruefung "Minor wie angefragt" entfaellt | upgrade 1 | `kubernetes_plan_test.go:204: a blocked plan still carries a chain: [v1.34.2 v1.34.1]` (Zeile "another minor") |
| (e) | Kappe von 16 auf `1<<20` gehoben (nicht entfernt, siehe Abweichung 3) | upgrade 1 | `kubernetes_plan_test.go:322: the source was asked [[11 12 ... 29]] before the cap was applied` (Zeile "more minors than the cap") |
| (f) | nil-Quelle wie "keine Zwischenminors noetig" behandelt | upgrade 1 | `kubernetes_plan_test.go:224: the block does not say there is no source`, `:227: ... which target needs no lookup` |

Jede Injektion wurde per `cmp` gegen die Sicherung als wiederhergestellt bestaetigt; danach laufen die Tests gruen.

**Task 3, Injektion des Zaehlers:** `open_count: 21` auf 22 verstellt, `go test ./internal/planning/ -count=1` Exit 1: `TestTheFrontMatterCountsWhatIsThere: windows_test.go:199: front matter says open_count: 22, the ledger holds 21`; zurueckgestellt (`cmp`), `go test ./internal/planning/ ./internal/publicrepo/` Exit 0.

## Verifikation (gelaufen)

- `go test ./internal/upgrade/ ./internal/httpapi/ -run Kubernetes -count=1 -v`: Exit 0, 13 `--- PASS` auf oberster Ebene, kein SKIP, kein FAIL; darunter `TestAKubernetesChainReachesTheNewestKubernetesTalos114Supports` und `TestTheKubernetesPlanRouteNamesTheNewestPatchOfTheMinorOnTheWay`.
- `go test ./internal/upgrade/ ./internal/compat/ ./internal/planning/ ./internal/publicrepo/ -count=1`: Exit 0.
- `go build ./...`, `go vet ./internal/upgrade/ ./internal/httpapi/ ./internal/planning/`, `gofmt -l internal`: sauber.
- Abnahme: in keiner Codezeile von `plan.go`/`service.go` steht `Patch: 0`; `git diff --name-only BASE -- internal/compat internal/changelog internal/config` leer.
- `go test ./internal/httpapi/ -count=1` ganz (143 s): bis auf die vier Tests, die das eingebettete Web-Bundle brauchen (`TestCSPHashMatchesTheServedShell`, `TestForeignHostIsRefused`, `TestNoAssetIsInlinedPastTheContentSecurityPolicy`, `TestFontsAreServedAsFiles`), gruen. Die vier schlagen hier aus diesem Grund fehl und haben mit der Aenderung nichts zu tun.

## Abweichungen vom Plan

1. **[Reihenfolge] Kappe und Vorpruefung schon in Task 1.** Siehe oben: Task 2 hatte in `plan.go`/`service.go` nichts mehr zu bauen, und seine Tests waren beim ersten Lauf gruen statt rot. Die Belege sind die Injektionen (a) bis (f).
2. **[Rule 2 - fehlende Pruefung] Antwort "v1.33" ohne Patch.** `ParseVersion` liest `v1.33` als 1.33.0; eine Quelle, die so antwortet, haette einen stillen Patch null erzeugt. Der Service verlangt deshalb genau drei Zahlenteile; die Zeile "no patch number, which would parse as patch zero" in `TestASourceAnswerThatIsNotAStableReleaseOfThatMinorIsAFailure` haelt es. Eine eigene Injektion dafuer habe ich nicht gemacht: diese Zeile ist nicht rot gesehen.
3. **[Messgrenze] Injektion (e) hat die Kappe nicht entfernt.** Ohne Kappe wuerde die zweite Zeile des Kappentests (`v1.2000000000.0`) rund 2 Mrd. Eintraege anlegen. Statt dessen wurde die Kappe auf `1<<20` gehoben: rot wird nur die Zeile mit 19 Zwischenminors. **Die Zeile `v1.2000000000.0` wurde gegen die Kappe nicht rot gesehen** (sie bleibt unter `1<<20` gruen) und ist deshalb kein Beleg fuer T-16-01 im engeren Sinn; belegt ist, dass eine Kappe existiert und vor jeder Anfrage greift.
4. **[Erwartung] Injektion Task 2 (a), Absenden.** Der Plan erwartet 202; weil die Test-Bestaetigung `anything` nicht zum Request passt, antwortet die Route bei durchgelassenem Fallback 403 (`confirmation does not match`). Der Test wird trotzdem rot (erwartet 409 `upgrade-blocked`), und der Bildschirmtest meldet zusaetzlich `was not blocked`.
5. **[Werkzeug] Ledger von Hand.** `gsd-tools windows append --kind gap` wurde vom Werkzeug abgelehnt (`Ledger entry 147 has invalid kind: "gap"`: die vorhandenen Eintraege tragen Arten, die sein Vokabular nicht kennt). Eintrag 174 steht deshalb von Hand in beiden Haelften im Format von `cell`/`row`; `open_count` 20 auf 21, `total_count` 173 auf 174, `last_updated` auf 2026-10-12. Der Test ist gruen.
6. **[Pruefstand] Zwei meiner ersten Edits am Pruefstand griffen nicht** (falsche Einrueckung im Suchtext), der Routentest zeigte es sofort (Knoten blieb auf 1.34.1, dann "no source of Kubernetes releases"); beide nachgeholt, bevor irgendetwas committet wurde.

## Wichtig fuer das, was als Naechstes kommt

**Bis Plan 02 verdrahtet ist, baut der Daemon die Kette ohne Quelle.** `cmd/holzkube-managerd` ruft `WithKubernetesReleases` noch nicht, also ist in einem Daemon aus diesem Stand jeder Kubernetes-Plan mit Zwischenschritt blockiert (`no source of Kubernetes releases`); Ziele in der laufenden oder der naechsten Minor bleiben planbar. Das ist D-02 wie gewollt, aber es heisst: dieser Stand ist kein Release-Kandidat ohne Plan 02.

## Bekannte Grenzen, unveraendert gelassen

1. Die Kette prueft das Fenster der laufenden Talos-Version nur fuer das Ziel, nicht fuer den Start und nicht fuer die Zwischenminors.
2. Ein getipptes Ziel, das ein Pre-Release ist oder gar nicht existiert, ist im Plan nicht blockiert; der Prepull-Schritt des Jobs scheitert fruehzeitig, bevor etwas neu startet.
3. Ein anderes Major als 1 wird nicht eigens geprueft (der Service vergleicht nur das Major der Antwort mit dem des Ziels).

Dazu eine bewusst akzeptierte Reihenfolge: bei einer Talos-Version ohne Matrixzeile blockiert erst `CheckStrand` nach der Kette; faellt dann die Quelle aus, steht der Quellensatz statt des Strand-Satzes da (beide Plaene sind blockiert).

## Was NICHT bewiesen ist

Nur Simulator und httptest-artige Fakes belegt. Nicht belegt sind:

- die Dauerhaftigkeit des Formats von dl.k8s.io (hier kommt erst in Plan 02 ueberhaupt ein Client; in diesem Plan gibt es nur eine Fake-Quelle),
- die Annahme eines Zwischenpatches durch einen echten Cluster,
- ein Lauf auf dem Pi (linux/arm64); gelaufen ist darwin/arm64 gegen talossim und Fakes.

Nicht gelaufen ist `go test -race` (CI allein), ebenso nicht `task ci`, der Layout-Waechter und die vier Web-Bundle-Tests von `internal/httpapi` (siehe oben). Die Kette bleibt eine Anzeige: der Job installiert weiter das getippte Ziel in einem Lauf (Ledger 174, D-05).

## Offen ausserhalb dieses Plans

README, `docs/guide.md` (Plan 02), Changelog-Eintrag (Release), Budget-/Verdrahtungswaechter im Daemon (Plan 02). `K8S-01` wird hier nicht abgehakt; das tut die Phasen-Verifikation nach Plan 02.

## Known Stubs

Keine.

## Threat Flags

Keine neue Angriffsflaeche ausserhalb des Threat-Models: es gibt in diesem Plan keinen Netzzugriff (die Quelle ist eine Funktion). T-16-01 (Kappe, siehe Abweichung 3 fuer die Grenze des Belegs), T-16-02 (Pruefung der Antwort, Injektion (d)) und T-16-04 (kein stiller Rueckfall, Injektion (a)) sind belegt.

## Commits

- `82292e3` feat(16-01): Kubernetes chain names the newest stable patch of each minor on the way
- `eac4fbc` test(16-01): the edges of the Kubernetes lookup, each seen red against its fault
- `c38d60b` docs(16-01): the Kubernetes plan route is in the contract, and the chain is advice

## Self-Check: PASSED

Alle sieben genannten Dateien vorhanden, die drei Commits (`82292e3`, `eac4fbc`, `c38d60b`) sind Vorfahren von HEAD.
