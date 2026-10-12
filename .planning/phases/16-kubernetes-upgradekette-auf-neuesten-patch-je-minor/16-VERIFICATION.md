---
phase: 16-kubernetes-upgradekette-auf-neuesten-patch-je-minor
verified: 2026-10-12T16:30:00Z
status: human_needed
score: 6/6 must-haves verified
covered_files:
  - ".planning/phases/16-kubernetes-upgradekette-auf-neuesten-patch-je-minor/16-01-PLAN.md"
  - ".planning/phases/16-kubernetes-upgradekette-auf-neuesten-patch-je-minor/16-01-SUMMARY.md"
  - ".planning/phases/16-kubernetes-upgradekette-auf-neuesten-patch-je-minor/16-02-PLAN.md"
  - ".planning/phases/16-kubernetes-upgradekette-auf-neuesten-patch-je-minor/16-02-SUMMARY.md"
  - "cmd/holzkube-managerd/main.go"
  - "internal/k8srelease/k8srelease.go"
  - "internal/upgrade/plan.go"
  - "internal/upgrade/service.go"
covered_digest: "v3:sha256:c23e368219d01352c21259fa8935956d9068003cc3f6025e7a16b83a3530e5b1"
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "curl https://dl.k8s.io/release/stable-1.35.txt (und eine Minor ohne stabilen Patch), danach eine echte Kette gegen den Betreiber-Cluster planen"
    expected: "Eine Zeile v1.35.PATCH ohne Pre-Release; der Plan zeigt fuer jede Zwischenminor diesen Patch, nie .0; bei gekappter Verbindung ein blockierter Plan mit Satz"
    why_human: "Die Dauerhaftigkeit des dl.k8s.io-Formats (einmal am 2026-10-12 gemessen, kein Vertragsdokument), TLS gegen den echten Host, ein echter haengender Server und die Annahme eines Zwischenpatches durch einen echten Cluster sind ohne Netz/Cluster nicht belegbar. Bewusst offen als Ledger 175 (nur von Hand, Betreiberentscheidung)."
---

# Phase 16: Kubernetes-Upgradekette auf neuesten Patch je Minor - Verifikationsbericht

**Phase-Ziel:** Die Zwischenschritte der Kubernetes-Upgradekette laufen auf dem neuesten Patch je Minor statt ueber `.0`. Eng begrenzt in `internal/upgrade`; ohne echten Cluster nur gegen die Simulatoren belegbar, und der Bericht sagt das.
**Verifiziert:** 2026-10-12, Mac (darwin/arm64), Go 1.27.2 lokal
**Status:** human_needed (alle automatisch pruefbaren Wahrheiten verifiziert; nur die benannten, bewusst offenen Grenzen bleiben)
**Re-Verifikation:** Nein, Erstverifikation

## Was hier belegt ist und was nicht

Belegt gegen `httptest`, Fakes und talossim auf darwin/arm64. **Nicht belegt:** Dauerhaftigkeit des `dl.k8s.io`-Formats, TLS gegen den echten Host, ein wirklich haengender Server, die Annahme eines Zwischenpatches durch einen echten Cluster, ein Lauf auf linux/arm64 (nur die Uebersetzung ist von den SUMMARYs belegt, von mir nicht erneut), `go test -race` (nur CI), `task ci`, der Layout-Waechter, der Linter (nicht installiert) und die vier Web-Bundle-Tests von `internal/httpapi`. Nichts davon habe ich hier ausgefuehrt oder behauptet.

## Observable Truths

| # | Wahrheit | Status | Beleg |
| - | -------- | ------ | ----- |
| 1 | Der Plan-Weg erzeugt fuer Zwischenschritte den neuesten stabilen Patch, nirgends ein stilles `.0` | VERIFIED | `plan.go` `KubernetesChain`: Zwischenschritte nur aus der uebergebenen Map `newest`, fehlende Minor ist ein `ErrNoPath` mit Namen der Minor, keine Eigenerzeugung von `Patch: 0`. Einzige Aufrufer von `KubernetesChain`: `service.go:398`. Handler `upgrade.go:271` und `:565` rufen `PlanKubernetes` (kein zweiter Kettenbauer; `grep` ueber alle nicht-Test-`.go`). Injektion A rot (siehe unten). |
| 2 | Lookup-Fehler bei Mehr-Minor-Kette blockiert den Plan mit einem Satz; Einzelschritt-Ziele fragen nie | VERIFIED | `newestKubernetesPatches`: `len(minors)==0` kehrt vor jeder Quellenfrage zurueck; nil-Quelle, Fehler, fehlende Minor, Pre-Release, falsche Minor, Antwort ohne Patch ("v1.33") -> `Blocked` + Satz "The newest patch of ... could not be determined". `kubernetesChainPrecheck` (Downgrade, Strand) laeuft vor der Quelle. Tests `TestASingleHopNeverAsksTheSource`, `TestAStrandedTargetNeverAsksTheSource`, `TestNoSourceBlocksOnlyMultiMinorChains`, `TestAPlanWhoseLookupFailedIsBlockedWithASentence`, httpapi `TestSubmittingAKubernetesUpgradeWhoseLookupFailedIsRefused` (409 `conflict.upgrade-blocked`) gruen. Verhalten ist testgedeckt, nicht nur praesent. |
| 3 | D-01: `stable-1.N.txt`, nie `latest-`, strikte Validierung | VERIFIED | `k8srelease.go`: Datei `"stable-1."+minor+".txt"`; `latest-` kommt in nicht-Test-Code nur im Kommentar vor. Nur Status 200, <= 64 Byte, Regex `^v1\.<minor>\.[0-9]+$` (Go-`$` ohne `(?m)`: zweite Zeile faellt durch), Redirect nur im selben Host/Schema, max. 3, Fehlertexte geben keine Antwortbytes wieder. Injektion C rot. |
| 4 | D-02..D-05: blockiert statt Rueckfall; `internal/compat` und `internal/changelog` unberuehrt; kein Flag, fest verdrahtet; Kette nur Hinweis + Ledger 174; Budget-Tabelle | VERIFIED | `git diff --name-only d30fcb5..HEAD` enthaelt weder `internal/compat` noch `internal/changelog` noch `internal/config`. `k8srelease.New(k8srelease.DefaultBaseURL)` ohne Config-Eingang; AST-Waechter haelt das. `RequestFor` unveraendert (Kette bleibt Anzeige), Ledger 174 (open, gap) benennt das und die Folge aus D-02. Budget: neue Klasse `kubernetesReleasesCall` liest `k8srelease.Budget`, drei Routenzeilen (`kubernetes/plan`, `kubernetes`, `confirm`) tragen `kubernetesReleasesLookup`, Test `TestTheKubernetesPlanRoutesDeclareTheReleaseLookup`. |
| 5 | Der Daemon verdrahtet die Quelle und erreicht sie (Deklaration != Benutzung) | VERIFIED | `main.go:510` `k8srelease.New(DefaultBaseURL)`, `:523` `.WithKubernetesReleases(k8sReleases.Newest)` an genau dem `upgradeSvc`, der in `main.go:711` als `Upgrade:` in die Deps geht; keine weitere `upgrade.NewService`-Stelle. Injektion B rot. |
| 6 | Doku, README, API-Vertrag, Ledger 174/175 vorhanden; Changelog nicht angefasst; keine Betreiberkennungen | VERIFIED | README (Feature-Liste), `docs/guide.md`, `docs/api-contract.md` ("The Kubernetes plan: the chain is advice") beschreiben Verhalten inkl. ehrlicher Grenze; `WINDOWS.md` 174 (gap, open) und 175 (unrun-verify, open), Zaehler 22/175; `internal/planning` und `internal/publicrepo` gruen. |

**Score:** 6/6 Wahrheiten verifiziert (0 present-behavior-unverified).

## Gelaufene Pruefungen (Exitcodes aus dem Testbefehl selbst)

| Befehl | Ergebnis |
| ------ | -------- |
| `go test ./internal/k8srelease ./internal/upgrade ./internal/compat ./internal/planning ./internal/publicrepo ./cmd/holzkube-managerd -count=1` | alle ok, Exit 0 |
| `go test ./internal/httpapi -run Kubernetes -count=1` | ok, Exit 0 |
| `go vet` derselben Pakete plus `internal/httpapi` | Exit 0 |
| `gofmt -l internal cmd` | leer |
| `verify.artifacts` / `verify.key-links` beider Plaene | 6/6 + 2/2 bzw. 6/6 + 3/3 |

## Eigene Fehlerinjektionen (Original per `cp` gesichert, danach `cmp` byte-gleich, `git status` sauber, danach wieder gruen)

| # | Eingriff | Ergebnis |
| - | -------- | -------- |
| A | In `KubernetesChain` den Zwischenschritt auf `Patch: 0` gesetzt (nur diese Stelle; ein erster Versuch traf versehentlich auch die Talos-`Chain`, `TestCheckTalos` wurde mit rot, das zaehle ich nicht) | Exit 1: `TestAKubernetesChainReachesTheNewestKubernetesTalos114Supports`, `TestTheKubernetesChainPassesThroughEveryMinor`, `TestAPlanWithIntermediatesAsksTheSourceOnceForExactlyTheMinorsBetween`, `TestTheRealSourceFeedsTheChain` (upgrade) und `TestTheKubernetesPlanRouteNamesTheNewestPatchOfTheMinorOnTheWay` (httpapi) |
| B | `.WithKubernetesReleases(...)` aus `main.go` entfernt. Erster Versuch war nur ein Compile-Fehler (`declared and not used: k8sReleases`), also keine Messung; zweiter Versuch mit `_ = k8sReleases` | Exit 1: `TestTheDaemonWiresTheKubernetesReleaseSourceWithTheFixedHost: main.go calls WithKubernetesReleases 0 times, want exactly once` |
| C | In `k8srelease.go` `"stable-1."` durch `"latest-1."` ersetzt | Exit 1: vier `k8srelease`-Tests (`TestNewestAsksTheStableList...`, `...AtOnce`, `...FailsWhenOneOfTheMinorsFails...`, `...RefusesWhatIsNotAStable...`) und `TestTheRealSourceFeedsTheChain`, `TestTheRealSourceAnsweringNotFoundBlocksThePlan` |

## Anti-Pattern

Keine `TBD/FIXME/XXX/TODO/HACK` in den geaenderten Produktionsdateien. Keine Stubs: Datenfluss `main.go` -> `Service.kubernetesReleases` -> `newestKubernetesPatches` -> `KubernetesChain` -> `Plan.Chain` -> JSON der Plan-Route ist durch den httpapi-Test Ende zu Ende gegen die echte Route belegt.

## Ehrlichkeit der SUMMARYs

Beide SUMMARYs benennen die Grenzen wortwoertlich: kein echter Cluster, kein Pi-Lauf (nur `GOARCH=arm64`-Uebersetzung), kein `-race`/`task ci`/Linter/Layout, die vier Web-Bundle-Tests, `dl.k8s.io`-Format einmal gemessen, kein echter haengender Server, kein TLS gegen den echten Host. Sie legen auch Nicht-Messungen offen (Injektion der Kappe `v1.2000000000.0` nicht rot gesehen; Patch-loser Fall "v1.33" ohne eigene Injektion; Build-Fehler-Injektionen verworfen; Behauptung der Recherche zum Vollstaendigkeitswaechter widerlegt). Kein Overclaim gefunden. Ein Hinweis: SUMMARY 16-02 meldet fuer den vollen Lauf einmal `internal/power` rot (Zeitverhalten unter Last, danach allein gruen); dieses Paket habe ich nicht erneut gefahren, es ist von der Phase nicht beruehrt.

## Warnungen (keine Blocker)

1. **Bewusst akzeptierte Grenze, kein Mangel:** Die Kette ist Hinweis, kein Schloss (D-05, Ledger 174): der Job installiert das getippte Ziel in einem Lauf. Folge aus D-02: faellt dl.k8s.io aus, ist ein Mehr-Minor-Ziel blockiert, das ohne Lookup in einem Lauf liefe. In Ledger, API-Vertrag und Guide benannt.
2. **Offen und sichtbar:** Ledger 175, das Format von `stable-1.N.txt` hat keine Live-Pruefung (Betreiberentscheidung: nur von Hand). Daher `human_needed` und nicht `passed`: ein einziger manueller `curl` und eine echte Kettenplanung gegen den Betreiber-Cluster stehen aus.
3. Vor dem Release: Changelog-Eintrag fehlt absichtlich (`/release` verweigert das Tag ohne ihn); `task ci` und CI-`-race` muessen auf dem Betreiber-Rechner/CI laufen.

## Gaps

Keine.

---

_Verifiziert: 2026-10-12_
_Verifier: Claude (gsd-verifier)_
