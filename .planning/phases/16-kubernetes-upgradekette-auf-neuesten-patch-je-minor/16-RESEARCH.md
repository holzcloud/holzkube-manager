# Phase 16: Kubernetes-Upgradekette auf neuesten Patch je Minor - Research

**Researched:** 2026-10-12
**Domain:** `internal/upgrade` (Kubernetes-Upgradeplan), ein neuer schmaler Release-Lookup, Simulator-Tests
**Confidence:** HIGH für Code-Befunde und die live gemessene Quelle; MEDIUM für die Talos-Supportmatrix (Doku und Maschinerie widersprechen sich in einem Punkt, siehe Open Questions)

**Umgebung dieser Recherche:** macOS (darwin/arm64), lokales Go 1.27.2 (Projekt pinnt 1.26.9 über `GOTOOLCHAIN` im Taskfile; hier mit `GOTOOLCHAIN=local` gelaufen). Kein Pi, kein echter Cluster, kein `-race`. Baseline gelaufen: `go test ./internal/upgrade/ ./internal/compat/ -count=1` -> beide `ok`. Netz war erreichbar (dl.k8s.io, ghcr.io, docs.siderolabs.com, kubernetes.io).

<user_constraints>
## User Constraints (from CONTEXT.md)

Es gibt keine CONTEXT.md für diese Phase; der Betreiber hat bewusst ohne geplant. Es gibt daher keine gesperrten Entscheidungen, keine ausdrücklichen Ermessensbereiche und keine zurückgestellten Ideen. Alles, was unten als "Choice" steht, ist offen und wird dem Betreiber als Auswahl vorgelegt (CLAUDE.md: "Every question is a choice, always, and it goes through the choice tool").

Bindend sind stattdessen die Regeln aus `CLAUDE.md` (siehe nächster Abschnitt) und das Phasenziel aus `ROADMAP.md`: "Die Zwischenschritte der Kubernetes-Upgradekette laufen auf dem neuesten Patch je Minor statt über `.0`. Eng begrenzt in `internal/upgrade`; ohne echten Cluster nur gegen die Simulatoren belegbar, und der Bericht sagt das."
</user_constraints>

<phase_requirements>
## Phase Requirements

Keine Requirement-IDs vergeben (ROADMAP: "Requirements: TBD"). Der Planer soll eine Kennung für die Phase anlegen (z.B. `UPG-K8S-PATCH`) oder mit "none" arbeiten. Rückbezug auf bestehende IDs: UPG-05 (Kette statt "latest") und UPG-06 (Strand-Gate) sind die Anforderungen, an die diese Phase anschließt.
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Öffentliches Repository:** Keine LAN-Adresse, kein Host-/öffentlicher Name, keine Mail, keine MAC, kein Home-Verzeichnis der echten Anlage in Tests, Fixtures, Ledger oder Commit-Messages. Dokumentationswerte nutzen (192.168.1.10, homeserver, example.com). `internal/publicrepo` prüft Baum und Historie. Die Recherche hat keinen Betreiberwert in dieses Dokument getragen.
- **Feature nicht fertig ohne README; Release nicht ohne Changelog:** Jede neue Funktion aktualisiert README ("What it does"-Liste; Screenshot nur bei eigenem Screen) im selben Change, Tiefe in `docs/guide.md`. Der Changelog-Eintrag (`internal/changelog/changelog.json`, neueste zuerst, für den Betreiber geschrieben) ist Sache des Releases; der Release-Job verweigert ein Tag ohne Eintrag.
- **Alpha überall gesagt:** unverändert, nichts anzufassen.
- **Verifikationsehrlichkeit:** Bericht sagt, wo die Sitzung lief und was sie nicht prüfen konnte. Hier: nur Simulatoren (talossim, kubesim, httptest), nie ein echter Cluster; arm64 nicht ausgeführt. Die Phase darf nicht als "auf dem Cluster verifiziert" gemeldet werden.
- **linux/arm64 (Raspberry Pi 5) ist das einzige Release-Ziel;** nichts Plattformspezifisches einführen, kein cgo.
- **Fragen an den Betreiber sind immer Auswahl** (`AskUserQuestion`, zwei bis vier Optionen mit Kosten/Nutzen, Empfehlung zuerst). Gilt für die Open Questions unten.
- **Methode:** Ein Wächter zählt erst, wenn er gegen den bewusst zurückgesetzten Fehler rot gesehen wurde. Jede Testaussage im Commit heißt: Fehler wieder eingebaut, Test rot gesehen. Der Plan muss dafür je Wächter eine konkrete Injektion benennen (siehe Validation Architecture).
- **Talos-Pin:** nicht Teil dieser Phase; `machineryversion_test.go` und `talos-upstream.yml` halten ihn.

## Summary

**Was "über `.0`" im Code heißt.** Die Kubernetes-Kette entsteht in genau einer Funktion: `KubernetesChain(from, to Version, talosVersion string)` in `internal/upgrade/plan.go`. Für jede Minor von `from.Minor+1` bis `to.Minor` entsteht ein Schritt; die letzte Minor bekommt die vom Betreiber getippte Zielversion, jede Zwischenminor wird als `Version{Major: to.Major, Minor: m, Patch: 0}` fest auf `.0` gesetzt. Es gibt keine Quelle für Kubernetes-Versionen im Repository: `Service.releases` liefert nur Talos-Versionen (Image Factory), `compat` kennt nur Minor-Fenster, nicht Patches. Das Talos-Gegenstück `Chain(from, to, available)` macht es bereits richtig (neuester stabiler Patch je Minor aus einer Liste, Pre-Releases gefiltert, fehlende Minor = `ErrNoPath`); die K8s-Kette hat diese Liste nie bekommen.

**Was die Kette heute tatsächlich bewirkt, und das ist der wichtigste Planungsbefund.** `PlanKubernetes` hängt die Kette nur an den Plan (`plan.Chain`, Anzeige "This takes more than one run" in `web/src/routes/upgrades.tsx`, nur wenn mehr als ein Schritt). `RequestFor(plan)` baut den Job aber aus `plan.To`, also der getippten Zielversion; `kubernetesSteps` installiert genau `req.To` in einem Lauf. Nichts blockiert ein Ziel, das mehrere Minor entfernt ist. Die `.0`-Schritte sind also ein Text, den der Betreiber abtippen soll, kein Lauf, den der Dienst ausführt. Der Fix der Phase ändert daher, was der Betreiber *sieht und tippt* (`v1.35.0` -> `v1.35.9`). Wer die Kette durchsetzen will, braucht eine eigene Entscheidung (Choice 5).

**Woher "neuester Patch je Minor" kommen kann.** Gemessen in dieser Sitzung: `https://dl.k8s.io/release/stable-1.N.txt` liefert für jede Minor eine Zeile mit dem neuesten *stabilen* Patch (200, `text/plain`, 7 Byte, `cache-control: public, max-age=86400`); für eine Minor ohne stabilen Patch (1.38, nur Alpha) und für eine nicht existierende Minor `404` mit XML-Body (`NoSuchKey`). `latest-1.N.txt` ist dagegen eine Falle: `latest-1.37.txt` lieferte `v1.37.0-rc.1`. Das Talos-Kubelet-Image `ghcr.io/siderolabs/kubelet` trägt dieselben Patch-Tags (1.34.0..1.34.12 und 1.35.0..1.35.9 gemessen), das Prepull-Schritt-Versprechen ("a version that does not exist fails here, before anything has been restarted") bleibt die zweite Absicherung.

**Zweiter Befund, der die Phase im Betrieb erst sichtbar macht:** `internal/compat/compat.go` führt für Talos 1.14 das Fenster `{MinMinor: 32, MaxMinor: 35, DefaultMinor: 35}`. Die Talos-Doku nennt für 1.14 Kubernetes 1.37 bis 1.33, die in `go.mod` gepinnte Maschinerie v1.14.2 hat `DefaultKubernetesVersion = "1.37.1"` und `SupportedKubernetesVersions = 6`. Die Zeilen 12 und 13 sind ebenfalls schmaler als die Doku. `KubernetesChain` lehnt jedes Ziel jenseits des Fensters mit `ErrWouldStrand` ab. Ein Cluster auf Kubernetes 1.34 unter Talos 1.14 kann damit keine Kette mit Zwischenschritten planen (dafür wäre ein Ziel >= 1.36 nötig). **Ohne Korrektur der Matrix ist das Phasenergebnis auf dem Cluster des Betreibers nicht beobachtbar**, auch wenn alle Simulator-Tests grün sind (Choice 3).

**Primary recommendation:** `KubernetesChain` bekommt wie `Chain` eine übergebene Menge neuester Patches (`map[int]Version`), die `Service` über eine neue, schmale Quelle (`WithKubernetesReleases`, ein Lookup von `dl.k8s.io/release/stable-1.N.txt` je benötigter Zwischenminor, nebenläufig unter einem Deadline) füllt; nur bei Ketten mit Zwischenschritten wird gelesen; scheitert die Quelle, ist der Plan *blockiert mit Satz*, nie stillschweigend `.0`. Die Matrix-Korrektur als eigene Vorab-Aufgabe, die Durchsetzung der Kette ausdrücklich nicht in dieser Phase.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Kette berechnen (welche Minor, welcher Patch) | API / Backend (`internal/upgrade`, reine Funktion) | — | Reine Logik, bereits so geschnitten (`Chain`, `KubernetesChain`); testbar ohne Netz |
| Neuesten Patch je Minor beschaffen | API / Backend (kleiner HTTP-Client, verdrahtet in `cmd/holzkube-managerd/main.go`) | — | Wie die Talos-Releases: Quelle ist eine Funktion, die der Compositionsroot einsetzt; `internal/upgrade` kennt den Host nicht |
| Kompatibilitätsfenster Talos <-> Kubernetes | API / Backend (`internal/compat`, kuratierte Tabelle) | CI (wöchentlicher Wächter, falls gewählt) | Bewusst eingebaut, kein Netzlookup (Kommentar in `compat.go`) |
| Anzeige der Kette | Browser / Client (`web/src/routes/upgrades.tsx`) | — | Rendert `plan.chain`; JSON-Form ändert sich nicht |
| Durchsetzen der Reihenfolge (nur falls Choice 5 = B) | API / Backend (`PlanKubernetes`/`RequestFor`) | Browser (Schrittwahl) | Heute nicht durchgesetzt |
| Existenz der Images | Knoten (Prepull-Schritt im Job) | — | Bestehende Absicherung gegen nicht existierende Version |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go-Standardbibliothek (`net/http`, `context`, `io`, `regexp`/`strconv`, `sync`) | Go 1.26.9 (go.mod-Toolchain) | Lookup-Client, nebenläufige Abfrage, Validierung | Keine neue Abhängigkeit nötig; das Repo kapselt Upstream-Clients selbst (`internal/imagefactory/client.go`) und pflegt eigene Versionsvergleiche statt `x/mod/semver` [VERIFIED: internal/imagefactory/versions.go, Kommentar "implemented here rather than with golang.org/x/mod/semver"] |
| `internal/upgrade` `Version`/`ParseVersion`/`Less` | im Repo | Versionsmodell, `Pre`-Feld, Vergleich | Bereits der Typ der Ketten-Schritte (`Step.To`) und der JSON-Form (`{Major,Minor,Patch,Pre}`) |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `net/http/httptest` | stdlib | Fake dl.k8s.io für Client-Tests | Alle Client-Tests (200, 404-XML, Minor-Mismatch, `-rc`, Übergröße, Timeout) |
| `talossim` / `kubesim` / `inventory` / `jobs` | im Repo | Ende-zu-Ende gegen Simulatoren | Plan-Route und Job (`kubernetes_job_test.go`-Muster) |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Live-Lookup `dl.k8s.io/release/stable-1.N.txt` | Kuratierte Patch-Tabelle im Repo (wie `compat`) + wöchentlicher Wächter | Offline-sicher, aber monatlich veraltet (Kubernetes-Patches erscheinen etwa monatlich [CITED: kubernetes.io/releases/patch-releases/]) und braucht je Patch ein Produktrelease; Wächter-Workflow nötig |
| Live-Lookup dl.k8s.io | GitHub Releases API `kubernetes/kubernetes` | Enthält Pre-Releases, unauthentifiziert 60 Anfragen/Stunde [ASSUMED: GitHub-Standardlimit], Paginierung; schlechter als eine Zeile je Minor |
| Live-Lookup dl.k8s.io | Tag-Listen von `registry.k8s.io` / `ghcr.io/siderolabs/kubelet` | Prüft die Existenz der Images direkt, aber große Listen, ghcr braucht Token-Austausch (gemessen: anonymer Token funktioniert), mehrere Hosts |
| `latest-1.N.txt` | — | **Nicht verwenden:** liefert Pre-Releases (`latest-1.37` -> `v1.37.0-rc.1`, gemessen) |

**Installation:** keine. Es wird kein externes Paket installiert.

**Version verification:** Entfällt (stdlib). `go.mod`: `go 1.26.6`, `toolchain go1.26.9`; `github.com/siderolabs/talos/pkg/machinery v1.14.2`; `k8s.io/client-go v0.34.1` (in `internal/depguard_test.go` gepinnt) [VERIFIED: go.mod:12-13 und internal/depguard_test.go: `machineryVersion = "v1.14.2"`, `clientGoVersion = "v0.34.1"`].

## Package Legitimacy Audit

Keine externen Pakete in dieser Phase. Kein `gsd-tools package-legitimacy`-Lauf nötig.

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
Betreiber tippt Ziel (z.B. v1.37.1)
        |
        v
POST /api/v1/clusters/{id}/upgrade/kubernetes/plan  (handlers/upgrade.go: upgradePlan(kubernetes=true))
        |
        v
Service.PlanKubernetes(ctx, cluster, to)                       internal/upgrade/service.go
  |- Machines -> lowestKubernetes (from), lowestTalos
  |- Zwischenminors = from.Minor+1 .. to.Minor-1  --- leer? ----> keine Quelle gelesen
  |       | nicht leer
  |       v
  |   newestPatch(minor) je Zwischenminor, nebenläufig, ein Deadline
  |       |  Quelle: dl.k8s.io/release/stable-1.N.txt   (Funktion, in main.go eingesetzt)
  |       |  Antwort validieren: 200, Body ^v1\.N\.\d+$, N == angefragt, kein "-", <= 64 Byte
  |       |-- Fehler/fehlende Minor --> plan.Blocked + BlockReason (Satz), KEIN .0-Ersatz
  |       v
  |   KubernetesChain(from, to, talosVersion, newest map[int]Version)   reine Funktion
  |       |- to.Less(from) -> ErrNoPath ; to == from -> leer
  |       |- compat.Check(talos, to) -> ErrWouldStrand (Fenster nur fuer `to`)
  |       v
  |   []Step{ 1.35.9 "on the way", 1.36.5 "on the way", to "The version you chose." }
  |- CheckStrand, Knotenliste (walkOrder), Gate-Vorschau
        v
Plan JSON {to, chain[], strand, gate_preview, nodes[], blocked, block_reason}
        v
UI: "This takes more than one run" (nur chain.length > 1) -- Betreiber tippt Schritt 1 als naechstes `to`
        v
confirm -> submit: upgradeParams() baut den Plan NEU (Lookup erneut), RequestFor(plan) -> Request{To: plan.To}
        v
Job cluster.upgrade-kubernetes: kubernetesSteps(req.To)  (unveraendert: Prepull, 3 Static Pods, kube-proxy, Kubelets)
```

### Component Responsibilities

| Datei | Rolle | Änderung in dieser Phase |
|-------|-------|--------------------------|
| `internal/upgrade/plan.go` `KubernetesChain` | wählt Zwischenschritte | `Patch: 0` ersetzen durch übergebenen Patch; neue Fehlerform für fehlende Minor |
| `internal/upgrade/service.go` `PlanKubernetes` | sammelt Eingaben, baut Plan | Zwischenminors bestimmen, Quelle befragen, bei Fehler blockieren |
| `internal/upgrade/service.go` `Service`/`NewService` | hält `releases` (Talos) | neues Feld + Builder `WithKubernetesReleases` nach dem Muster `WithSnapshots` (`safetysnapshot.go:364`) |
| `cmd/holzkube-managerd/main.go` (~Zeile 506) | verdrahtet Quellen | Lookup-Funktion einsetzen |
| neue kleine Quelle (Paket-Ort offen: `internal/upgrade` oder `internal/kube`-fremd; Planer entscheidet, `internal/upgrade` bleibt frei von Host-Wissen) | HTTP-Lookup | neu |
| `cmd/holzkube-managerd/budget_test.go` | Kompositionswächter der Routen-Deadlines | Zeilen für `POST .../upgrade/kubernetes/plan`, `.../upgrade/kubernetes` und `.../upgrade/confirm` (K8s-Fall) um den neuen Aufruf ergänzen |
| `web/src/routes/upgrades.tsx`, `web/src/api.ts` | Anzeige | keine Strukturänderung; JSON-Form von `chain[].to` bleibt `{Major,Minor,Patch,Pre}` |

### Recommended Project Structure
```
internal/upgrade/
├── plan.go              # KubernetesChain(from, to, talos, newest map[int]Version)
├── service.go           # PlanKubernetes: Zwischenminors -> Quelle -> Kette; WithKubernetesReleases
├── k8sreleases.go       # (neu, falls die Quelle hier wohnt) Client + Validierung + nebenlaeufige Abfrage
├── plan_test.go         # reine Kettentests (Patch, fehlende Minor, Pre-Release)
├── k8sreleases_test.go  # (neu) httptest-Client-Tests
└── kubernetes_plan_test.go  # (neu) Service-Ebene: Quelle faellt aus, Einzelschritt liest nichts
```

### Pattern 1: Die Kette ist eine reine Funktion über eine übergebene Liste
**What:** Wie `Chain(from, to, available)`: die Funktion bekommt die Daten, holt keine. Fehlende Minor -> Fehler mit Satz, der `ErrNoPath` wrappt.
**When to use:** Immer; hält die Logik netzfrei testbar.
**Example (Struktur der heutigen Funktion, auf die der Eingriff zielt):**
```go
// Source: internal/upgrade/plan.go:304-314 (gelesen in dieser Sitzung)
	var steps []Step
	for m := from.Minor + 1; m <= to.Minor; m++ {
		step := Version{Major: to.Major, Minor: m, Patch: 0}
		why := fmt.Sprintf("Kubernetes upgrades one minor at a time: a kubelet may be one minor "+
			"behind the API server and no more, so 1.%d has to be passed through.", m)
		if m == to.Minor {
			step = to
			why = "The version you chose."
		}
		steps = append(steps, Step{To: step, Why: why})
	}
```
Zielform (Skizze, kein fertiger Code):
```go
for m := from.Minor + 1; m < to.Minor; m++ {
    v, ok := newest[m]
    if !ok { return nil, fmt.Errorf("%w: ...1.%d ...", ErrNoPath, m) }
    steps = append(steps, Step{To: v, Why: "...on the way..."})
}
steps = append(steps, Step{To: to, Why: "The version you chose."})
```
Achtung: bei `from.Minor == to.Minor` (Patch-Sprung) liefert die heutige Schleife *keinen* Schritt (Kette leer); das ist heute Verhalten und der Test `TestTheKubernetesChainPassesThroughEveryMinor` kennt nur den Mehr-Minor-Fall. Der Planer entscheidet bewusst, ob ein reiner Patch-Sprung weiter eine leere Kette ergibt (empfohlen: ja, unverändert) und sichert das per Test.

### Pattern 2: Quelle als Funktion, verdrahtet im Compositionsroot
**What:** `NewService(d, releases)` nimmt `func(ctx) ([]string, error)` für Talos; Kommentar: "It is a function because the list comes from the Image Factory and this package has no business knowing that" [VERIFIED: internal/upgrade/service.go:24-27]. Dasselbe Muster für Kubernetes, aber als Builder (`WithKubernetesReleases(func(ctx, minor int) (string, error))` o.ä.), damit `NewService`-Aufrufer (viele Tests: `NewService(deps, nil)`) unverändert bleiben.
**Nil-Quelle:** Verhalten explizit festlegen: Ketten mit Zwischenschritten sind dann blockiert mit Satz "this instance has no source of Kubernetes releases", Einzelschritte laufen weiter.

### Pattern 3: Blockierter Plan statt Fehler
`PlanKubernetes` gibt bei `KubernetesChain`-Fehler bewusst `Blocked=true, BlockReason=...` und kein `error` zurück (Kommentar: "A version pair with no legal path is not a failure of this call: it is the answer") [VERIFIED: internal/upgrade/service.go:348-355]. Der Quellenfehler folgt derselben Form.

### Anti-Patterns to Avoid
- **Den Fehler der Quelle schlucken wie in `PlanTalos`:** Dort steht `if rerr == nil { ... }` [VERIFIED: internal/upgrade/service.go:166]; fällt die Release-Quelle aus, hat der Talos-Plan *keine* Kette und ist nicht blockiert, ohne Hinweis. Für die K8s-Kette nicht übernehmen (CLAUDE.md: "never a silent 'no drift' where the reading failed", gleicher Geist in `gatePreview`: "A gate that could not be evaluated is a refusal, not a blank").
- **`latest-1.N.txt` benutzen** (Pre-Releases).
- **Die Kette in die Job-Parameter oder den Store schreiben.** Nichts persistieren: die Kette wird bei jedem Plan neu berechnet (siehe Failure Modes).
- **`.0` als stiller Rückfall bei Lookup-Fehler** (führt den Fehler der Phase wieder ein; nur als ausdrücklich gewählte, sichtbar gekennzeichnete Option 2B).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Pre-Release-Erkennung | eigene Regex für `-rc`/`-beta` | `Version.IsPreRelease()` / `ParseVersion` (`Pre`-Feld) | Existiert, wird von `Chain` und `Releases` genutzt |
| Versionsvergleich "neuester" | String-Vergleich | `Version.Less` | `v1.35.10` > `v1.35.9` nur numerisch richtig |
| Patch-Liste einer Minor | eigener Tag-Scraper | `stable-1.N.txt` (eine Zeile, vom Release-Team gepflegt) | Ein Wert je Minor, stabil per Definition |
| Existenzprüfung der Images | eigener Registry-Client | Prepull-Schritt (`prepullStep` in `job.go`) | Läuft vor jedem Neustart, scheitert früh |
| Skew-/Fensterprüfung | zweite Tabelle | `compat.Check` | Ein Ort (D-25); Tabellenfehler dort beheben, nicht umgehen |

**Key insight:** Die Aufgabe ist klein, weil `Chain` das Modell schon hat. Der Aufwand liegt nicht in der Logik, sondern in der Quelle (Fehlerverhalten, Deadline-Komposition, Validierung des Fremdbodys) und im Beleg.

## Common Pitfalls

### Pitfall 1: Die Kette ist beratend, nicht durchgesetzt
**What goes wrong:** Phase ändert `v1.35.0` -> `v1.35.9` in der Anzeige; der Lauf installiert weiterhin das getippte Ziel in einem Sprung.
**Why it happens:** `RequestFor` nutzt `plan.To` (`service.go:395-397`: `To: plan.To,`), `kubernetesSteps` nutzt `req.To`; `PlanKubernetes` blockiert Mehr-Minor-Ziele nicht. Das Request-Kommentar sagt "It is one version and not a chain" [VERIFIED: internal/upgrade/job.go:132-135]; die Anzeige sagt "several runs".
**How to avoid:** Im Bericht ehrlich benennen; Durchsetzung nur über Choice 5. Kein Test darf "Kette läuft als Läufe" behaupten.
**Warning signs:** Ein Test, der `plan.Chain[0].To` als Jobziel annimmt.

### Pitfall 2: Die Matrix blockiert genau die Ketten, die die Phase verbessert
**What goes wrong:** Unter Talos 1.14 gilt laut `compat` höchstens 1.35; eine Kette mit Zwischenschritten von 1.34 aus braucht ein Ziel >= 1.36 und wird mit `ErrWouldStrand` abgelehnt, bevor die neue Logik läuft.
**How to avoid:** Choice 3 vor oder in der Phase entscheiden; Simulator-Tests so wählen, dass sie die Matrix nicht brauchen (Beispiel: Knoten auf Kubernetes 1.33, Ziel 1.35.x, Zwischenschritt 1.34; passt in das Fenster 32..35 von Talos 1.14).

### Pitfall 3: Fremdbody nicht validiert
**What goes wrong:** `stable-1.99.txt` liefert `404` mit XML-Body; ein Client, der nur den Body liest, parst "<?xml ..." oder gibt einen leeren Patch zurück.
**How to avoid:** Nur `200` akzeptieren; Body trimmen und gegen `^v1\.<N>\.\d+$` mit der *angefragten* Minor N prüfen (verwirft auch `-rc.1`, fremde Minor, Mehrzeiler); Body mit `io.LimitReader` kappen (gemessen sind es 7 Byte). Kein Redirect über Hostgrenzen (Muster `refuseCrossHostRedirect` in `internal/imagefactory/client.go`, ist dort unexportiert).

### Pitfall 4: Deadline-Komposition vergessen
**What goes wrong:** `cmd/holzkube-managerd/budget_test.go` deklariert je Route die Upstream-Aufrufe und rechnet die Summe gegen `routeDeadline` (120 s für die Upgrade-Plan-Routen: `UpgradeReadRouteBudget = 120 * time.Second`, `handlers/upgrade.go:33`). Ein neuer Aufruf ohne Zeile lässt die Route außerhalb der Garantie (bzw. der Vollständigkeitswächter wird rot).
**How to avoid:** Einen Aufruf pro Route deklarieren (nebenläufig unter *einem* Deadline, nicht seriell je Minor), neue `callClass` falls ein eigener Betrag nötig ist (Muster `kubeSummaryCall`: "one call in a row ... under ONE shared deadline"). Die drei Routen, die `PlanKubernetes` aufrufen: `upgrade/kubernetes/plan`, `upgrade/kubernetes` (Submit baut den Plan neu), und `upgrade/confirm` (Kind-abhängig; `upgradeParams`).

### Pitfall 5: Teure Wiederholung
**What goes wrong:** Plan, Confirm und Submit rechnen die Kette je einmal; drei Lookups je Lauf, bei jedem Phone-Tipp neu.
**How to avoid:** Lookup nur wenn Zwischenminors existieren (häufigster Fall: keine); optional kleiner In-Memory-Cache mit kurzer TTL (die Quelle sendet `max-age=86400`). Kein persistenter Cache.

### Pitfall 6: Falsche Begründung im Why-Text
**What goes wrong:** Der bestehende Satz "a kubelet may be one minor behind the API server and no more" ist nach der Skew-Policy falsch (Kubelet darf bis zu drei Minor älter sein). Der echte Grund für die Minor-für-Minor-Kette ist: "Project policies for API deprecation and API change guidelines require `kube-apiserver` to not skip minor versions when upgrading, even in single-instance clusters" und Talos' eigene Pfad-Allowlist (siehe State of the Art). Der Text ist sichtbar für den Betreiber; wenn die Phase den Schritt anfasst, korrigieren.

### Pitfall 7: Betreiberwerte in Fixtures
Nichts aus dem echten Cluster in Tests. Dokumentationswerte (z.B. `https://192.168.1.41:6443` steht in bestehenden Tests; Dokumentations-Adressbereich) wie die vorhandenen Tests.

## Failure Modes (Frage 3, Entscheidungen für den Plan)

| Fall | Heutiges Verhalten | Empfohlene Entscheidung | Beleg |
|------|-------------------|-------------------------|-------|
| Quelle offline/unerreichbar, Kette hat Zwischenschritte | n/a (keine Quelle) | Plan **blockiert** mit Satz (nennt Quelle und Wirkung); Einzelschritt-Ziele nicht betroffen, da dort kein Lookup nötig ist | Choice 2 |
| Quelle offline, Kette ohne Zwischenschritt (Ziel = nächste Minor oder gleiche Minor) | n/a | **Kein Lookup, kein Fehler**; Test zählt Aufrufe = 0 | neu |
| "Neuester" ist Pre-Release | n/a | Nicht möglich über `stable-1.N`; Client verwirft trotzdem jeden Body mit `-` (Gürtel und Hosenträger) | gemessen: `latest-1.37` = `v1.37.0-rc.1`, `stable-1.37` = `v1.37.1` |
| Minor ohne stabilen Patch (z.B. nur Alpha/RC) | n/a | `404` -> Fehler -> Kette mit Satz abgelehnt wie `Chain` bei fehlender Minor ("no 1.N release is available to this installation") | gemessen: `stable-1.38.txt` = 404 (XML) |
| Patch, den Talos nicht unterstützt | n/a | Talos unterstützt Minor-Fenster, nicht Patches (Matrix nennt nur Minors); keine Patch-Sperre nötig. Zwischenminors *unterhalb* des Fensters des laufenden Talos (Start unter Fenster) prüft die Funktion heute nicht (nur `to`); unverändert lassen, im Plan als bekannte Grenze notieren | [CITED: docs.siderolabs.com/talos/v1.14/getting-started/support-matrix] |
| Skew-Regeln | `KubernetesChain` erzwingt eine Minor pro Schritt; sonst nichts | Beibehalten. Eine Minor pro Schritt entspricht kube-apiserver-Regel und Talos' `IsSupported` (nur `N->N` und `N->N+1`) | [CITED: kubernetes.io/releases/version-skew-policy/], [CITED: github.com/siderolabs/go-kubernetes upgrade/path.go] |
| Liste ändert sich zwischen den Läufen | Kette ist nirgends gespeichert | **Nichts speichern.** Jeder Plan rechnet aus `lowestKubernetes` neu; nach Lauf 1 ist `from` = neue Version und Lauf 2 sieht die dann aktuelle Liste. Das Bestätigungstoken bindet Aktion, Cluster und die Job-Params aus `upgradeParams` (`jobs.Intent{Action, Machine, Params}`), also `To` und Knotenliste, nicht die Kette | VERIFIED: handlers/upgrade.go:345-352 (`token, expires := d.Confirmer.Issue(jobs.Intent{ Action: string(kind), Machine: string(cluster), Params: params, })`) |
| Patch zwischen Plan und Lauf erschienen | n/a | Unkritisch: der Lauf installiert `req.To` (vom Betreiber getippt); die Kette ist Hinweis | Pitfall 1 |
| Quelle liefert Müll | n/a | Validierung (Pitfall 3) -> Fehler -> blockiert | neu |
| `from` hat anderes Major | `KubernetesChain` prüft nur Minor | Unverändert lassen; kein Fall in der Praxis (Kubernetes 1.x) | `compat.minor` lehnt Major != 1 ab |
| Betreiber tippt Pre-Release oder nicht existierende Version als Ziel | Plan nicht blockiert; Prepull scheitert früh | Nicht Teil der Phase; als bekannte Grenze notieren | job.go-Doku, guide.md "Upgrading Kubernetes" Schritt 1 |

## Code Examples

### Der Lookup-Vertrag, wie er gemessen wurde
```text
# Gemessen 2026-10-12 (curl) — Quelle: dl.k8s.io, nicht aus dem Training
GET https://dl.k8s.io/release/stable-1.35.txt  -> 200 text/plain, Body "v1.35.9" (7 Byte), cache-control: public, max-age=86400
GET https://dl.k8s.io/release/stable-1.36.txt  -> v1.36.5
GET https://dl.k8s.io/release/stable-1.37.txt  -> v1.37.1
GET https://dl.k8s.io/release/stable-1.34.txt  -> v1.34.12
GET https://dl.k8s.io/release/stable-1.33.txt  -> v1.33.13   (EOL-Minor loest weiter auf)
GET https://dl.k8s.io/release/stable-1.99.txt  -> 404 application/xml  <Error><Code>NoSuchKey</Code>...
GET https://dl.k8s.io/release/stable-1.38.txt  -> 404 XML   (1.38 hat nur Alpha)
GET https://dl.k8s.io/release/latest-1.37.txt  -> v1.37.0-rc.1   <- NICHT verwenden
```

### Testskizze für die Kette (reine Funktion)
```go
// Muster: internal/upgrade/plan_test.go (TestTheKubernetesChainPassesThroughEveryMinor, Z.180-193)
steps, err := upgrade.KubernetesChain(v(t, "v1.33.5"), v(t, "v1.36.5"), "v1.14.0",
    map[int]upgrade.Version{34: v(t, "v1.34.12"), 35: v(t, "v1.35.9")})
// erwartet: 34.12, 35.9, 36.5 -- in dieser Reihenfolge, Patch != 0
```
(Die Signatur ist ein Vorschlag des Researchers; der Planer legt sie fest.)

### Der Job bleibt unverändert und ist bereits belegt
`TestAKubernetesUpgradeMovesEveryPartAndKeepsTheConfiguration` fährt `cluster.upgrade-kubernetes` Ende zu Ende gegen `talossim` mit `to = "v1.35.2"` [VERIFIED: internal/upgrade/kubernetes_job_test.go:107 `const to = "v1.35.2"`]. Diese Phase ändert den Job nicht; die Patch-Stelle der Zwischenversion ist für `kubernetesSteps` irrelevant (es verarbeitet jede `Version`).

## State of the Art

| Alter Ansatz | Aktueller Ansatz | Quelle | Wirkung |
|--------------|-----------------|--------|---------|
| Zwischenschritt `1.N.0` | neuester stabiler Patch je Zwischenminor | Talos-Kette im Repo macht es schon; Kubernetes-Patches erscheinen ~monatlich, aktuell 1.35.9, 1.36.5, 1.37.1 [CITED: kubernetes.io/releases/patch-releases/] | `.0` ist die älteste Version der Minor |
| Talos-Pfadprüfung | `upgrade.Path.IsSupported()` in `siderolabs/go-kubernetes`: erlaubt nur `N->N` und `N->N+1` (Allowlist bis 1.37->1.38) [CITED: github.com/siderolabs/go-kubernetes/kubernetes/upgrade/path.go] | `talosctl upgrade-k8s` lehnt Mehr-Minor-Sprünge selbst ab; dieses Produkt tut es heute nicht |
| `talosctl upgrade-k8s --to` | Default `constants.DefaultKubernetesVersion` (Maschinerie v1.14.2: `"1.37.1"`), `--from` = niedrigste laufende Version | [VERIFIED: gomodcache machinery@v1.14.2 constants/constants.go:381] [CITED: raw.githubusercontent.com/siderolabs/talos/main/cmd/talosctl/cmd/talos/upgrade-k8s.go] | talosctl wählt selbst keine Zwischenversionen |
| Kubelet-Skew | bis zu drei Minor älter als kube-apiserver (>= 1.25), kube-proxy ebenso, controller-manager/scheduler bis eine Minor älter | [CITED: kubernetes.io/releases/version-skew-policy/] | Der Why-Text im Repo ("one minor ... and no more") ist veraltet bzw. falsch |

**Deprecated/outdated:**
- Why-Text in `KubernetesChain` (siehe Pitfall 6).
- `internal/compat` Zeilen 12-14 gegenüber der Doku (siehe Open Questions).

## Runtime State Inventory

Nicht zutreffend (keine Umbenennung/Migration). Nichts wird gespeichert, kein Datenbestand ändert sich.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | GitHub-Unauthentifiziert-Limit 60/h (nur als Ablehnungsgrund einer Alternative genannt) | Alternatives | Gering: Alternative wird ohnehin nicht empfohlen |
| A2 | `stable-1.N.txt` bleibt auch künftig stabil/unverändert im Format (nur Beobachtung heute, nicht als Vertragsdokument gefunden) | Standard Stack | Mittel: Format könnte wechseln; der validierende Client macht daraus einen blockierten Plan statt eines falschen Schritts. Optionaler Live-Test (Muster `HOLZKUBE_MANAGER_FACTORY_LIVE`) merkt es |
| A3 | Docs-Zeile "Talos 1.14: 1.37..1.33" (fünf Minors) ist maßgeblich gegenüber `SupportedKubernetesVersions = 6` (-> 1.32..1.37) in der Maschinerie | Open Questions | Mittel: untere Grenze 32 vs 33 für die korrigierte Matrix |
| A4 | Kubernetes-Patches erscheinen monatlich (Quelle nennt "typical ... monthly"), daher wäre eine kuratierte Tabelle monatlich veraltet | Alternatives | Gering |
| A5 | Die Kette ist heute nirgends durchgesetzt — belegt per Codelektüre (`RequestFor`, `kubernetesSteps`), **nicht** per gelaufenem Test; vor Entscheidung 5 mit einem Test gegen die Plan-Route erhärten | Summary | Mittel: falls ein bisher nicht gelesener Pfad doch begrenzt, entfällt Choice 5 |

## Open Questions (dem Betreiber als Auswahl vorzulegen)

Alle fünf sind Entscheidungen des Betreibers. Der Orchestrator legt sie über `AskUserQuestion` vor, Empfehlung jeweils zuerst.

**Choice 1 — Woher kommt der neueste Patch?**
- **A (empfohlen): Live-Lookup `dl.k8s.io/release/stable-1.N.txt`**, nur für Zwischenminors. Nutzen: immer aktuell, ein Wert je Minor, stabil per Definition, kein Wartungsaufwand. Kosten: neuer ausgehender Host, Validierung/Deadline/Test-Aufwand, braucht Netz am Daemon (Pi) bei Mehr-Minor-Plänen.
- **B: Kuratierte Tabelle im Repo** (wie `compat`) plus wöchentlicher Wächter-Workflow nach `talos-upstream.yml`. Nutzen: offline, kein neuer Host. Kosten: monatlich veraltet, je Patch ein Produktrelease, Wächter und Skript nötig; das ist größer als die Phase ("klein").
- **C: GitHub Releases API.** Nutzen: ein bekannter Host. Kosten: Pre-Releases mitgeliefert, Ratenlimit, Paginierung; nicht empfohlen.

**Choice 2 — Was, wenn der Lookup bei einer Mehr-Minor-Kette fehlschlägt?**
- **A (empfohlen): Plan blockiert mit Satz.** Nutzen: ehrlich, kein `.0` durch die Hintertür (Repo-Regel "refusal, not a blank"); Einzelschritte bleiben möglich. Kosten: ohne Netz am Daemon keine Mehr-Minor-Planung.
- **B: Zurückfallen auf `.0`, sichtbar gekennzeichnet.** Nutzen: Plan immer verfügbar. Kosten: reproduziert den Fehler der Phase im Fehlerfall; der Betreiber sieht den Hinweis, kann ihn aber beim Tippen übersehen.

**Choice 3 — `internal/compat` ist hinter der Doku zurück. Wie umgehen?**
Befund: Zeilen `12: {31,33,33}`, `13: {32,34,34}`, `14: {32,35,35}` gegenüber der Talos-Doku (1.12: 1.30-1.35; 1.13: 1.31-1.36; 1.14: 1.33-1.37) und Maschinerie v1.14.2 (`DefaultKubernetesVersion = "1.37.1"`). Folge: unter Talos 1.14 sind Kubernetes 1.36/1.37 als "newer than supported" blockiert; die Phase wäre im Betrieb nicht sichtbar.
- **A (empfohlen): Eigene kleine Vorab-Aufgabe** (Quick-Task) vor Phase 16: Zeilen 12-14 an die Doku angleichen und einen Wächter gegen die kompilierte Maschinerie ergänzen (Default-Minor von Zeile 14 muss `constants.DefaultKubernetesVersion` entsprechen, analog zu `machineryversion_test.go`). Nutzen: Phase 16 bleibt eng; die Korrektur hat eigenen Beleg und eigene Changelog-Zeile. Kosten: zwei Arbeitspakete.
- **B: Als Wave 0 in Phase 16.** Nutzen: ein Paket, Wirkung sofort sichtbar. Kosten: Phase wächst über "eng begrenzt in `internal/upgrade`" hinaus (`internal/compat`).
- **C: Nicht anfassen.** Nutzen: keine Arbeit. Kosten: Phase 16 bleibt für Talos 1.14 unbeobachtbar; Simulator-Tests wären grün, der Betreiber sähe keine Änderung.
Unklarheit für A/B: untere Grenze von Talos 1.14 ist 32 (Maschinerie, 6 Versionen) oder 33 (Doku, 5 Versionen); die Wahl der Obergrenze 37 ist durch beide gedeckt.

**Choice 4 — Konfigurierbarer Quell-URL (Air-Gap)?**
- **A (empfohlen): Kein Flag**, fester Host `dl.k8s.io`; Fehlerfall durch Choice 2 abgedeckt. Nutzen: keine neue Konfigurationsfläche, kein Doku-/Konfigtest-Aufwand. Kosten: kein Weg für einen abgeschotteten Standort.
- **B: Flag/Env `…_KUBERNETES_RELEASES_URL`** nach `IMAGE_FACTORY_URL`. Nutzen: Air-Gap möglich, Test-Umleitung ohne Code. Kosten: Konfigeintrag, Standardwert-Test, Zeile in `docs/guide.md` Konfigurationstabelle (~Zeile 263), mehr Fläche für Fehlkonfiguration.

**Choice 5 — Kette durchsetzen?**
Befund (Pitfall 1): Die Kette ist beratend; ein Mehr-Minor-Ziel läuft in einem Sprung, obwohl `talosctl upgrade-k8s` genau das mit `IsSupported()` ablehnt.
- **A (empfohlen): Außerhalb der Phase.** Phase 16 ändert nur, was angezeigt und abgetippt wird; der Fund wird als Ledger-Eintrag in `.planning/WINDOWS.md` festgehalten. Nutzen: Phase bleibt klein. Kosten: der Betreiber kann weiter in einem Sprung planen.
- **B: Mehr-Minor-Ziele im Plan blockieren** ("nächster erlaubter Schritt: v1.35.9"), das Ziel muss die erste Kettenstufe sein. Nutzen: schließt die Lücke, macht den Patch wirksam statt nur sichtbar. Kosten: Verhaltensänderung für Betreiber, die heute Sprünge tippen (breaking, Alpha erlaubt es); Änderungen in `PlanKubernetes`, Tests, UI-Text, Guide, Changelog; hängt an Choice 3 (sonst blockiert die Matrix ohnehin).
- **C: Nur warnen** (Plan nicht blockiert, Notice über dem Plan). Nutzen: gering invasiv. Kosten: eine Warnung, die man wegklickt, ändert nichts.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go Toolchain | Tests | ✓ | 1.27.2 lokal (Projekt 1.26.9, `GOTOOLCHAIN` im Taskfile) | `GOTOOLCHAIN=local` für Teilläufe; vollständiges Gate über `./bin/task ci` |
| Netz zu dl.k8s.io | Messung/optionaler Live-Test | ✓ (hier) | — | httptest-Fake für alle Pflicht-Tests |
| Node/npm | Web-Tests/Screenshots | nicht geprüft | — | Phase braucht keinen Web-Code, außer Textänderung |
| Echter Cluster / Pi / arm64-Lauf | End-to-End-Beleg | ✗ in dieser Umgebung | — | nur Simulatoren; Bericht sagt es (Phasenziel) |
| `go test -race` | CI | auf dem Pi nicht möglich (laut CLAUDE.md) | — | CI allein |

**Missing dependencies with no fallback:** keine für die Entwicklung. Der Beleg "läuft auf echtem Cluster" fehlt prinzipiell und wird so berichtet.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` (Standard), Tabellentests, `httptest`; Tests als externes Paket `upgrade_test` mit `export_test.go` für interne Zugänge |
| Config file | keine; Gate: `./bin/task ci` (Taskfile: `go test ./... -count=1 {{.RACE}}`) |
| Quick run command | `GOTOOLCHAIN=local go test ./internal/upgrade/ ./internal/compat/ -count=1` (Baseline: grün, 5,3 s + 0,7 s) |
| Full suite command | `./bin/task ci` (enthält Budget-/Kompositionswächter in `cmd/holzkube-managerd`, `internal/publicrepo`) |

### Phase Requirements → Test Map

Keine IDs vergeben; Zeilen nach Verhalten. Namen sind Vorschläge.

| Verhalten | Test Type | Automated Command | File Exists? |
|-----------|-----------|-------------------|-------------|
| Zwischenschritte tragen den übergebenen neuesten Patch, nicht `.0`; Reihenfolge aufsteigend; Endschritt = Betreiberziel | unit | `go test ./internal/upgrade/ -run TestTheKubernetesChain -count=1` | ❌ Wave 0 (bestehender Test `TestTheKubernetesChainPassesThroughEveryMinor` in `plan_test.go` bleibt Muster, Signatur ändert sich) |
| Fehlende Minor in der übergebenen Menge -> `ErrNoPath` mit Satz, nie `.0` | unit | `-run TestAKubernetesChainWithAMissingMinor` | ❌ Wave 0 |
| Gleiche Minor / eine Minor weiter -> keine Zwischenschritte, `newest` wird nicht gebraucht | unit | `-run TestAKubernetesChainWithoutIntermediates` | ❌ Wave 0 |
| Client: 200 + `v1.35.9\n` -> `v1.35.9`; 404-XML, falsche Minor, `-rc`, Mehrzeiler, Übergröße, Timeout, Cross-Host-Redirect -> Fehler | unit (httptest) | `-run TestKubernetesReleaseLookup` | ❌ Wave 0 |
| Service: Quelle fällt aus bei Mehr-Minor-Ziel -> `Blocked`, `BlockReason` nennt die Quelle, keine Kette mit `.0` | unit | `-run TestAPlanWhoseLookupFailed` | ❌ Wave 0 |
| Service: Einzelschritt-Ziel liest die Quelle nie (Zähler 0), auch wenn sie kaputt wäre | unit | `-run TestASingleHopNeverAsksTheSource` | ❌ Wave 0 |
| Service: nil-Quelle -> Mehr-Minor blockiert mit Satz; Einzelschritt unverändert | unit | `-run TestNoSourceBlocksOnlyMultiMinor` | ❌ Wave 0 |
| Plan-Route über den Router: Knoten auf Kubernetes 1.33 (talossim `KubeletImage`), Ziel `v1.35.x`, Fake-Quelle -> JSON `chain` mit Patch | integration (httpapi-Harness) | `go test ./internal/httpapi/ -run TestTheKubernetesPlan -count=1` | ❌ Wave 0 (Muster `newUpgradeHarness`) |
| Deadline-Komposition der drei Routen mit dem neuen Aufruf | guard | `go test ./cmd/holzkube-managerd/ -run Budget -count=1` | ✅ Datei da, Zeilen anpassen |
| Job unverändert: Kubernetes-Upgrade Ende zu Ende | integration | `go test ./internal/upgrade/ -run TestAKubernetesUpgradeMovesEveryPart -count=1` | ✅ |
| Optional: echter Lookup gegen dl.k8s.io | live (env-gesteuert) | `HOLZKUBE_MANAGER_K8S_LIVE=1 go test ... -run Live` (Namensvorschlag; Muster `HOLZKUBE_MANAGER_FACTORY_LIVE`) | ❌ optional |

### Sampling Rate
- **Per task commit:** Quick run command.
- **Per wave merge:** `go test ./internal/... ./cmd/... -count=1` (ohne Web).
- **Phase gate:** `./bin/task ci` grün, soweit hier lauffähig; andernfalls ausdrücklich benennen, was nicht lief (Muster der Auditberichte: "Nicht gelaufen: `./bin/task ci` ... `go test -race`").

### Rote Injektionen (CLAUDE.md-Methode; der Plan muss sie als Schritt tragen und das Gesehene im Commit nennen)
1. `Patch: 0` in `KubernetesChain` zurücksetzen -> Kettentest muss rot werden (erwartet `v1.35.9`, bekommt `v1.35.0`).
2. Fehlerzweig der Quelle auf "Rückfall `.0`" umbauen -> `TestAPlanWhoseLookupFailed` rot.
3. Lookup auch bei Einzelschritt aufrufen -> `TestASingleHopNeverAsksTheSource` rot.
4. Body-Validierung der Minor entfernen (Antwort `v1.36.0` auf `stable-1.35`) -> Client-Test rot.
5. Pre-Release-Filter entfernen -> Client-Test rot.
6. Deklaration des neuen Aufrufs in `budget_test.go` weglassen -> Vollständigkeitswächter rot (nur gültig, wenn der Wächter das heute tatsächlich prüft; Planer sieht es rot, bevor er es behauptet).
Eine Injektion, die nach Zurücksetzen grün bleibt, ist "keine Messung" (CLAUDE.md) und gehört in den Bericht.

### Was die Simulatoren belegen und was nicht
- **Belegbar:** Kettenlogik (rein), Quellenfehlerverhalten (httptest), Plan-Route mit Fake-Quelle über talossim-Inventar, Deadline-Komposition, dass der Job eine beliebige `Version` verarbeitet (bestehender Test).
- **Nicht belegbar:** dass dl.k8s.io dauerhaft diese Form liefert (nur heute gemessen); dass ein echter Cluster den Zwischenpatch annimmt (Prepull/Kubelet-Image-Existenz nur am echten Knoten; Ledger 85 hält "auf echter Hardware unbelegt" für den Job insgesamt); Verhalten auf arm64 (hier darwin/arm64, Pi nicht ausgeführt). Der Bericht muss das wörtlich sagen.
- **Kein Simulator für die Quelle nötig:** `kubesim` ist die Fake-Kubernetes-API und hat mit der Release-Quelle nichts zu tun; der Fake ist ein `httptest.Server`.

## Security Domain

`security_enforcement` ist nicht abgeschaltet (config.json: `true`, ASVS-Level 1, Blockade ab `high`).

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | nein (neue Route entsteht nicht; bestehende Routen bleiben hinter Sitzung/Sudo/CSRF) | bestehend |
| V3 Session Management | nein | bestehend |
| V4 Access Control | nein (Plan-Routen unverändert; Aktion `upgrade.plan-kubernetes`, Audit-Redaktion `{"to"}`) | bestehend (`internal/audit/redact.go:323`) |
| V5 Input Validation | ja | Fremdbody strikt validieren (Statuscode, Regex mit angefragter Minor, Größenkappe); Betreiber-`to` läuft weiter durch `ParseVersion` |
| V6 Cryptography | nein (TLS über Standard-`net/http`, Zertifikatsprüfung nicht abschalten) | stdlib |
| V12/13 Kommunikation, SSRF | ja | Fester HTTPS-Host oder nur Betreiberkonfiguration (Choice 4); Minor-Zahl als Integer in URL-Pfad einsetzen (kein String aus Nutzereingabe); keine Redirects über Hostgrenzen; Timeout/Deadline gesetzt (`http.DefaultClient` hat keine, Kommentar in `imagefactory/client.go`) |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Manipulierte/ersetzte Antwort der Quelle (falscher Patch) | Tampering | HTTPS mit Prüfung; Minor-Gleichheit und Regex; Prepull des Knotens scheitert vor jeder Änderung an einem nicht existierenden Image; Betreiber bestätigt den Plan |
| Hängende Quelle blockiert den Plan | Denial of Service | Per-Aufruf-Deadline unter Routenbudget, nebenläufig; Plan wird blockiert, nicht hängend |
| Übergroßer Body | DoS | `io.LimitReader` |
| Quellwert als Pfad-/Header-Injektion | Tampering | Antwort wird nie in URL/Header zurückgeschrieben; nur geparst in `Version` |
| Betreiberwerte im Repo | Information Disclosure | Dokumentationswerte in Tests; `internal/publicrepo` im Gate |

## Docs, Contract und Release (Frage 5: sichtbare Oberfläche)

Was sich ändert und was dafür angefasst werden muss:

| Fläche | Änderung | Datei |
|--------|----------|-------|
| Plan-Anzeige | Versionen in der Kettenliste (`v1.35.0` -> neuester Patch); Strukturell keine, Zusatzhinweis bei blockiertem Lookup kommt über `block_reason` (bestehender roter Notice) | `web/src/routes/upgrades.tsx` (Z.~262-285), `web/src/api.ts` (`upgradeStepSchema`, `upgradePlanSchema`, Z.~1716-1800): unverändert |
| API-Vertrag | Abschnitt "The plan is the screen" (`chain`-Tabelle, `docs/api-contract.md` ~Z.4467-4480) spricht nur von Talos ("every intermediate minor"); Absatz ergänzen: bei Kubernetes ist jeder Zwischenschritt der neueste stabile Patch seiner Minor, der Lookup, und was bei Ausfall geschieht (`blocked`/`block_reason`). Die Kubernetes-Plan-Route selbst ist im Vertrag nicht als eigener Abschnitt beschrieben (grep: keine Treffer für `upgrade/kubernetes` in `*.md`); das ist eine Lücke, die der Planer entweder mit schließt oder benennt | `docs/api-contract.md` |
| Guide | Abschnitt "Upgrading Kubernetes" (`docs/guide.md` ~Z.1185-1209): Absatz zur Kette (Zwischenminors über den neuesten Patch, Lookup-Quelle, Verhalten ohne Netz, Hinweis "die Kette ist eine Anleitung, der Lauf installiert, was getippt wird" falls Choice 5 = A). Der Schluss "has run against the simulator and a fake Kubernetes API, not yet against a real cluster" bleibt | `docs/guide.md` |
| Konfiguration (nur Choice 4 = B) | neue Zeile in der Flag-Tabelle (`docs/guide.md` ~Z.263), Eintrag in `internal/config/config.go` mit Standardwert-Test | `internal/config`, `docs/guide.md` |
| README | Lifecycle-Aufzählpunkt "Kubernetes upgrades in `talosctl upgrade-k8s`'s order" (README ~Z.75) um einen Halbsatz ergänzen; kein neuer Screen, also kein Screenshot | `README.md` |
| Changelog | neuer oberster Eintrag für die nächste Version; die neueste Version in der Datei ist `v0.6.0` (2026-10-12), Tag `v0.6.0` existiert. Der Eintrag wird beim Release geschrieben (Skill `release`); Ton wie bestehende Einträge: für den Betreiber, ehrlich "Run against the simulator only, not yet against a real cluster" | `internal/changelog/changelog.json` |
| Ledger | Befunde, die nicht Teil der Phase sind (Kette beratend; Matrix veraltet; Why-Text falsch), als Einträge in `.planning/WINDOWS.md`, je nach Choice 3/5 | `.planning/WINDOWS.md` |
| Audit/Redaktion | `upgrade.plan-kubernetes` redigiert `{"to"}`; keine neuen Felder, nichts zu ändern | `internal/audit/redact.go` |

## Sources

### Primary (HIGH confidence)
- Repository, in dieser Sitzung gelesen: `internal/upgrade/plan.go` (Z.128-190 `Chain`, 284-316 `KubernetesChain`), `internal/upgrade/service.go` (Z.18-56, 128-137, 160-172, 314-388, 390-413, 622-654), `internal/upgrade/job.go` (Z.30-135, 327-363), `internal/upgrade/check.go`, `internal/compat/compat.go` (Z.34-51 Matrix), `internal/compat/compat_test.go`, `internal/httpapi/handlers/upgrade.go`, `cmd/holzkube-managerd/budget_test.go` (Z.100-200, 660-735, 1520-1560), `cmd/holzkube-managerd/main.go` (Z.495-530), `web/src/routes/upgrades.tsx`, `web/src/api.ts` (Z.1716-1810), `docs/guide.md`, `docs/api-contract.md`, `internal/talossim`, `internal/upgrade/kubernetes_job_test.go`, `plan_test.go`, `check_test.go`, `.planning/WINDOWS.md` Einträge 86 und 173, `.planning/AUDIT-2026-10-08.md`, `.planning/ROADMAP.md`.
- Maschinerie im Modul-Cache: `github.com/siderolabs/talos/pkg/machinery@v1.14.2/constants/constants.go` Z.379-384 (`DefaultKubernetesVersion = "1.37.1"`, `SupportedKubernetesVersions = 6`).
- Live-Messung dieser Sitzung (curl, 2026-10-12): dl.k8s.io `stable-1.31..1.37`, `stable-1.38` (404), `stable-1.99` (404 XML), `stable.txt`, `latest-1.37/1.38`; ghcr.io Tag-Liste `siderolabs/kubelet` (anonymer Token), Tags 1.34.0-1.34.12, 1.35.0-1.35.9, 1.36.x, 1.37.0/1.37.1.
- Kubernetes Version-Skew-Policy: https://kubernetes.io/releases/version-skew-policy/
- Kubernetes Patch Releases: https://kubernetes.io/releases/patch-releases/

### Secondary (MEDIUM confidence)
- Talos Support Matrix v1.14 / v1.12: https://docs.siderolabs.com/talos/v1.14/getting-started/support-matrix.md , https://docs.siderolabs.com/talos/v1.12/getting-started/support-matrix.md (v1.14: "1.37, 1.36, 1.35, 1.34, 1.33"; v1.13: "1.36 ... 1.31"; v1.12: "1.35 ... 1.30"); Datumszeile "2026-08-27 (TBD)" markiert als TBD.
- Talos "Upgrading Kubernetes": https://docs.siderolabs.com/kubernetes-guides/advanced-guides/upgrading-kubernetes (Reihenfolge; sagt nichts zu Patch-Wahl oder Mehr-Minor).
- `siderolabs/go-kubernetes` `kubernetes/upgrade/path.go` (`IsSupported`-Allowlist) und `siderolabs/talos` `cmd/talosctl/cmd/talos/upgrade-k8s.go` (`--to` Default, `NewPath`), jeweils über raw.githubusercontent.com gelesen (Zusammenfassung des Abrufwerkzeugs, nicht Volltext geprüft).

### Tertiary (LOW confidence)
- Keine tragenden Aussagen aus reiner Websuche.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — stdlib, Muster im Repo vorhanden.
- Architecture: HIGH — Funktionsgrenzen und Aufrufer im Code gelesen; Pitfall 1 (Kette beratend) per Lektüre, nicht per Test (A5).
- Quelle (dl.k8s.io): HIGH für heutiges Verhalten (gemessen), MEDIUM für Dauerhaftigkeit (A2).
- Matrix-Befund: MEDIUM — Doku und Maschinerie stimmen in der oberen Grenze überein (37), nicht in der unteren (33 vs 32).
- Pitfalls: HIGH.

**Research date:** 2026-10-12
**Valid until:** 2026-11-10 (Kubernetes-Patch- und Talos-Release-Takt monatlich; die gemessenen Patchnummern sind ein Stand, kein Vertrag)
