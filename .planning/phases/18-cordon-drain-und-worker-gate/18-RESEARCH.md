# Phase 18: Cordon, Drain und Worker-Gate - Research

**Researched:** 2026-10-16
**Domain:** `internal/upgrade` (Entfernen-Job, Talos-Upgrade-Job, neues Worker-Gate), `internal/kube` (Drain, Cordon, Node-Löschen), `internal/kubesim` (Simulator), Doku und UI-Text
**Confidence:** HIGH für alle Code-Befunde (Dateien dieser Sitzung gelesen) und für das Verhalten der Kubernetes-Eviction und des Talos-Drains (Quelltext gelesen); MEDIUM für das Verhalten eines echten Clusters (nicht beweisbar, siehe Beweisgrenze)

**Umgebung dieser Recherche:** macOS (darwin/arm64), lokales Go 1.27.2 (Projekt pinnt 1.26.9 über `GOTOOLCHAIN`; hier `GOTOOLCHAIN=local`). Kein Pi, kein echter Cluster, kein `-race`. Baseline gelaufen: `go test ./internal/upgrade/ ./internal/kube/ ./internal/kubesim/ ./internal/power/ ./internal/jobs/ -count=1` -> alle `ok` (kubesim hat keine eigenen Tests). Netz erreichbar (raw.githubusercontent.com, Websuche).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Blockiert beim Entfernen ein PodDisruptionBudget den Drain, läuft der Drain **mit Frist**, danach **bricht der Job vor dem Entfernen ab** und nennt das blockierende Budget. Der Knoten bleibt unverändert, nichts wird halb entfernt. **Kein erzwungenes Löschen von Pods.** — **Reversibility:** reversible.
- **D-02:** Das Worker-Gate im Upgrade prüft vor dem Reboot eines Workers Readiness der Knoten und PodDisruptionBudgets und hält den Upgrade-Schritt mit einem Satz an, wenn das Gate offen ist, so wie das Control-Plane-Gate (`upgrade`, etcd-Gesundheitstor). Es ändert nie Arbeitslasten.

### Claude's Discretion
Die Frist des Drains, wie das Gate seine Bedingung formuliert, ob es ein erneutes Prüfen mit Frist oder ein sofortiges Anhalten gibt, Reihenfolge der Schritte im Job (cordon, drain, Entfernen aus etcd, Node-Objekt löschen), der Umgang mit Knoten, die nicht antworten (Drain unmöglich: ehrlich sagen, nie still überspringen). Die Beweisgrenze: nur gegen `kubesim`, ein echter Cluster hat es nicht gesehen — im Bericht und im Ledger zu sagen.

### Deferred Ideas (OUT OF SCOPE)
- Erzwungenes Entfernen nach Frist (bewusst nicht gewählt).
- Rückbau von Upgrades (Audit: „groß", bewusst fehlend).
- Cordon/Drain für Reboot und Power-Aktionen außerhalb des Entfernens.
</user_constraints>

<phase_requirements>
## Phase Requirements

Keine Requirement-IDs vergeben (ROADMAP: "Requirements: TBD"). Der Planer legt eine Kennung an (Vorschlag: `UPG-DRAIN` für Entfernen mit Cordon/Drain/Node-Objekt, `UPG-WGATE` für das Worker-Gate) oder arbeitet ohne. Anschluss an bestehende IDs: UPG-13 (Entfernen als Job), UPG-02 (Gate vor jedem Knoten).
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Öffentliches Repository:** keine LAN-Adresse, kein Host-/öffentlicher Name, keine Mail, MAC oder Home-Verzeichnis der echten Anlage in Tests, Fixtures, Ledger, Commit-Messages. Dokumentationswerte (192.168.1.10, homeserver, example.com). `internal/publicrepo` prüft Baum und Historie. Diese Recherche trägt keinen Betreiberwert ein.
- **Feature nicht fertig ohne README; Release nicht ohne Changelog:** README im selben Change ("What it does"; Screenshot nur bei eigenem Screen, gerendert aus `web/fixtures/demo.json`), Tiefe in `docs/guide.md`. Changelog-Eintrag ist Sache des Releases (CONTEXT: "Changelog erst beim Release").
- **Alpha überall gesagt:** nichts anzufassen. `holzkube-managerd --version` bleibt eine nackte Version.
- **Verifikationsehrlichkeit:** der Bericht sagt, wo die Sitzung lief. Hier nur Simulatoren (talossim, kubesim, httptest); arm64 nicht ausgeführt; kein echter Cluster hat Cordon, Eviction, PDB oder Node-Löschen dieser Phase gesehen. Nie als "auf dem Cluster verifiziert" melden.
- **Ziel linux/arm64 (Pi 5)**: nichts Plattformspezifisches, kein cgo.
- **Fragen an den Betreiber sind immer Auswahl** (`AskUserQuestion`, zwei bis vier Optionen, Empfehlung zuerst). Gilt für die Open Questions unten.
- **Methode:** ein Wächter zählt erst, wenn er gegen den bewusst wieder eingebauten Fehler rot gesehen wurde. Der Plan nennt je Wächter eine konkrete Injektion (Abschnitt Validation Architecture).
- **Linter lokal:** `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...`. Gate: `./bin/task ci`.

## Summary

**Der Kern in drei Sätzen.** Die Bausteine sind zu 80 Prozent da, sie hängen nur nicht am Entfernen-Job und nicht am Upgrade: `internal/kube` hat `Cordon`, `Drain` (Eviction-API, DaemonSets und Mirror-Pods übersprungen, Bare-Pods und emptyDir benannt statt geräumt), `PodsOnNode`/`Undrained` für Resume und einen `node.drain`-Job; `internal/power` hat dieselbe Schrittfolge (cordon, drain, reboot, uncordon) samt Resume-Mustern, `findNode`, `PowerCordoned`. Es fehlen vier Dinge: (1) ein **Drain mit Frist, der ein PDB-Veto abwartet** (heute genau ein Durchgang, 429 wird sofort zu "Blocked", und ein Test sichert genau das ab), (2) das **Benennen des Budgets** (die Doku behauptet es schon, der Code nennt es nicht), (3) **Node-Objekt löschen** (kein `Nodes().Delete` im Repo, kein DELETE in kubesim) und (4) ein **Worker-Gate** samt Kubernetes-Zugang im `upgrade.Deps`.

**Der wichtigste Befund über Talos selbst.** Talos drained auf dem Knoten bereits (Reset mit `graceful`, Upgrade, Shutdown), aber seine Drain-Schleife **gibt nach fünf Minuten auf und macht trotzdem weiter**: ein PDB-geblockter Pod wird alle fünf Sekunden neu evictet, nach `DrainTimeout = 5 * time.Minute` wird der Fehler nur als `WARNING` geloggt, danach `StopAllPods`/`RemoveAllPods`. Das heutige Entfernen (`Graceful: true`) und jedes Worker-Upgrade **töten also einen PDB-geschützten Pod nach fünf Minuten doch**. Das ist die Lücke, die D-01 und D-02 schließen, und es heißt: die Prüfung muss **vor** dem ersten unumkehrbaren Talos-Aufruf stehen, und sie kann nur prüfen, nicht das Talos-Verhalten ändern. CONTEXT sagt "heute drained es nicht"; genauer: Talos drained, aber ohne Veto-Wirkung.

**Primäre Empfehlung:** Entfernen bekommt vier neue Schritte ("cordon the node", "drain the node" vor dem Entfernen aus etcd; "delete the Kubernetes node object" **nach** dem Wipe); der Drain läuft mit `RetryBlocked` bis `kube.DrainBudget` und bricht bei PDB-Veto ab, nennt Budget **und** Pod aus `Status.Details.Causes[DisruptionBudget]` und macht ein eigenes Cordon rückgängig. Das Worker-Gate ist ein eigener Schritt je Knoten vor dem Talos-Upgrade (Vorbild `power.gate`), prüft Readiness aller anderen Knoten und einen **dry-run-Eviction** je räumbarem Pod des Workers, wartet bis `WorkerGateBudget` (5 min) erneut und verweigert dann mit einem Satz. Beweis nur gegen kubesim, der dafür vier Fähigkeiten braucht (Abschnitt kubesim).

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Cordon, Drain mit Frist, Node löschen | `internal/kube` (Client) | — | Einziger Ort, der die Kubernetes-API spricht; Jobs/Gate rufen nur auf (Muster `power`, `kube/job.go`) |
| Schrittliste des Entfernens, Resume-Prüfungen | `internal/upgrade` (`removejob.go`) | `internal/jobs` (Engine) | Die Reihenfolge ist der Job; die Engine entscheidet Resume anhand `Verifiable`/`Happened` |
| Worker-Gate (Readiness, PDB) | `internal/upgrade` (neue Datei `workergate.go`, Gegenstück zu `gate.go`) | `internal/kube` (liest) | Gate-Logik wohnt neben dem etcd-Gate; die reine Entscheidungsfunktion ist testbar ohne Cluster |
| Zugang zur Kubernetes-API aus dem Job | Composition Root (`cmd/holzkube-managerd/main.go`, `inv.KubeClient`) | `internal/inventory` | `upgrade` darf kein Inventar-/Store-Wissen über Zertifikate haben; Funktion in `Deps` (Muster `KubeProxy`) |
| Sätze, die der Betreiber liest (Refusal, Hold) | `internal/upgrade` / `internal/kube` (Fehlertext) | `web/src` (zeigt nur an) | Job-Schritt `Detail` ist der Kanal; UI rendert generisch |
| Simulation von PDB-Veto, Cordon, Node-Löschen | `internal/kubesim` | — | Einzige Beweisbasis ohne echten Cluster |
| Erzwingen, dass PDBs nicht umgangen werden | `internal/kube` (Eviction statt Delete) | — | Eviction ist die Aufrufart, die PDBs prüft; Delete nicht |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `k8s.io/client-go` | v0.34.1 (gepinnt in `go.mod`, per `depguard_test.go` abgesichert) | Eviction (`PolicyV1().Evictions().Evict`), Node patch/delete, Pod-Liste mit Feldselektor | Bereits Abhängigkeit; `drain.go` nutzt sie. [VERIFIED: go.mod:22 `k8s.io/client-go v0.34.1`] |
| `k8s.io/api/policy/v1` | v0.34.1 | `Eviction`, `PodDisruptionBudget`, `DisruptionBudgetCause` | [VERIFIED: ~/go/pkg/mod/k8s.io/api@v0.34.1/policy/v1/types.go:25 `const DisruptionBudgetCause metav1.CauseType = "DisruptionBudget"`] |
| `internal/kube`, `internal/kubesim`, `internal/jobs`, `internal/upgrade`, `internal/power` | Repo | siehe Architektur | Alles vorhanden |

### Supporting / Alternatives
Keine neuen Pakete. Kein `kubectl`-drain-Helfer (`k8s.io/kubectl/pkg/drain`) hinzufügen: er brächte eine neue Abhängigkeit, und `internal/kube/drain.go` bildet dieselben Regeln bereits ab (Reihenfolge wie kubectl, DaemonSet, Mirror, finished, bare, local data). Talos' eigener `nodedrain` liegt im schweren Root-Modul und ist durch `TestBinaryDependencyWeight` ausgeschlossen. [VERIFIED: internal/depguard_test.go:60-80 verbietet Pakete des Talos-Root-Moduls außer `pkg/machinery`]

**Installation:** keine.

## Package Legitimacy Audit

Diese Phase installiert **keine** externen Pakete (`go.mod` bleibt unverändert; `policyv1`, `apierrors`, `metav1`, `types` sind in `drain.go` bereits importiert). Audit entfällt.

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
POST /machines/{id}/remove-from-cluster (202, Job)
        |
        v
 Request: PreflightRemoval (Quorum, nur CP)  --refused--> 409, kein Job
        |
        v  Submit -> Cluster-Lease -> Engine.run (Context.Background-Kind, kein Frist)
 +--------------------------------------------------------------------+
 | 1 check the node answers        (Talos Probe, lesen)               |
 | 2 cordon the node               (K8s: Preflight + Cordon)          |-- K8s nicht erreichbar / 403 / Node unbekannt
 |     CP: Quorumregel zuerst (nichts geändert, falls Verweigerung)   |      -> Job failed HIER, nichts geändert
 | 3 drain the node                (Eviction, 429 -> erneut bis Frist)|-- PDB blockiert bis Frist
 |                                                                    |      -> eigenes Cordon zurück, Job failed,
 | 4 leave etcd        (CP)  5 let etcd settle (CP)                   |         Satz nennt Budget + Pod
 | 6 wipe the node                 (Talos Reset, UNVERIFIZIERBAR)     |
 | 7 delete the Kubernetes node    (K8s DELETE; NotFound = fertig)    |-- API weg/403 hier: Knoten ist schon gewipt!
 | 8 forget the node               (Inventar)                         |      -> Job failed, Satz nennt "kubectl delete node X"
 +--------------------------------------------------------------------+

Upgrade (Talos), je Knoten in req.Machines:
   [gate step "check Kubernetes can spare <id>"]  --> Worker: Readiness aller anderen Knoten
        |                                              + dry-run-Eviction je räumbarem Pod (Budget-Name vom Server)
        | offen -> alle 10 s neu prüfen bis 5 min -> dann Schritt FAILED mit EINEM Satz (wie CP-Gate)
        v
   [upgrade <id> to <v>]  (unverändert; CP: etcd-Gate im Schritt; Talos drained danach selbst)
```

### Recommended Project Structure
```
internal/kube/drain.go            # DrainOptions + RetryBlocked/Interval; Budgetname aus Causes; DeleteNode; FindNodeFor
internal/kube/drain_test.go       # bestehender Test "attempted 1 time" bleibt (Frist 0 = ein Durchgang)
internal/kube/evictioncheck.go    # neu: WouldEvict / dry-run (für das Gate)
internal/kubesim/kubesim.go       # Fähigkeiten K1-K5 (siehe kubesim)
internal/upgrade/removejob.go     # 3 neue Schritte, Reihenfolge, Resume
internal/upgrade/workergate.go    # neu: WorkerGate, decide(), Verdict-Text
internal/upgrade/job.go           # Deps.Kube, Deps.WorkerGate; talosSteps fügt Gate-Schritt je Knoten ein
cmd/holzkube-managerd/main.go     # Deps.Kube = inv.KubeClient
```

### Pattern 1: Die Entfernen-Schrittfolge (Ist und Soll)

**Ist** [VERIFIED: internal/upgrade/removejob.go:83-87]:
```go
steps := []jobs.Step{jobs.ReachableStep(d.Connect, j.Machine)}
if role == model.RoleControlPlane {
	steps = append(steps, d.leaveEtcdStep(j), d.settleStep())
}
return append(steps, d.wipeStep(j), d.forgetStep(j)), nil
```
Worker heute: `check the node answers`, `wipe the node`, `forget the node`. CP heute: `check the node answers`, `leave etcd`, `let etcd settle`, `wipe the node`, `forget the node` (getestet in `removejob_test.go:268-285` per exakter Namensliste).

**Soll:** nach dem Probe-Schritt `cordon the node`, `drain the node`; nach `wipe the node` `delete the Kubernetes node object`; dann `forget the node`. Worker: 6 Schritte, CP: 8. Schrittnamen bleiben generisch (kein Hostname), wie die vorhandenen.

**Warum Cordon/Drain vor `leave etcd`:** Pods müssen weg sein, bevor der Knoten die Membership verlässt; und alles, was verweigern kann (K8s-Zugang, PDB), muss **vor** dem ersten Schritt stehen, der den Knoten verändert, sonst gilt "nichts wird halb entfernt" nicht. Gleiche Begründung, warum der Probe-Schritt zuerst steht.

**Warum Node-Objekt nach dem Wipe:** Solange der Kubelet läuft, kann er sein Node-Objekt neu anlegen (siehe Pitfall 4, `[ASSUMED]`). Nach dem Wipe ist der Kubelet weg. Preis: ein Fehlschlag steht an der spätesten Stelle (Open Question Q4).

### Pattern 2: Resume je Schritt (Idempotenz, kein doppelt zerstörerischer Drain)

Die Engine entscheidet (`jobs.go`, `Resume`): unterbrochen im Schritt + `Verifiable` + `Happened` true -> übersprungen; false -> Schritt läuft neu; nicht verifizierbar -> `parked`. Außerdem [VERIFIED: internal/jobs/jobs.go:`steps, err := build(j); if err != nil || len(steps) != len(j.Steps) {` -> `park`]: **ein Job, der mit der alten Schrittzahl gespeichert wurde, wird nach dem Update geparkt** ("the job's steps could not be rebuilt as they were submitted"). Das ist korrekt und gewollt, gehört aber in die Release-Notiz (Entfernen-Jobs und Talos-Upgrades, die über das stündliche Update-Timer-Update hinweg laufen).

| Schritt | `Do` wiederholt | `Happened` | Begründung |
|---|---|---|---|
| `cordon the node` | harmlos (Patch `unschedulable:true`, idempotent) | `node.Unschedulable` lesen; API-Fehler -> `(false, nil)` (Muster `power.cordon`, steps.go:368-378) | Wiederholen kostet einen Aufruf |
| `drain the node` | harmlos: `Drain` cordont erneut (drain.go:141) und räumt nur, was noch auf dem Knoten steht; ein schon terminierender Pod wird von der Eviction ohne PDB-Prüfung angenommen (`canIgnorePDB`) | `Undrained(...)` leer; API-Fehler -> `(false, nil)` wie `power.drain` (steps.go:413-430), **nicht** `(false, err)` wie `kube/job.go:126-141` (das würde parken, obwohl Wiederholen sicher ist) | "Doppelt destruktiv" kann nur entstehen, wenn zwischenzeitlich jemand uncordont und Pods zurückkehren; dann ist ein zweiter Drain genau das, was der Betreiber verlangt hat |
| `leave etcd`, `let etcd settle` | unverändert | unverändert | nicht anfassen |
| `wipe the node` | — | **`nil` bleibt** (parkt) | unverändert |
| `delete the Kubernetes node object` | NotFound = Erfolg | Node nicht mehr in `Nodes()`; API-Fehler -> `(false, nil)` | Wiederholen sicher |
| `forget the node` | unverändert | unverändert | |

### Pattern 3: Drain mit Frist, der ein Veto abwartet (D-01)

`kube.Drain` ist heute ein Durchgang: jeder 429 wird sofort `Blocked`. Das ist **gewollt und per Test gesichert** [VERIFIED: internal/kube/drain_test.go:176-205, Aussage `"the eviction was attempted %d times; a budget's refusal must not be retried into submission"`, erwartet `Calls(...eviction) == 1`], und `power`/`node.drain` hängen daran. D-01 verlangt das Gegenteil für das Entfernen (mit Frist). Darum:

- **Nicht** `Drain`s Standard ändern. `DrainOptions` bekommt `RetryBlocked time.Duration` (Pause zwischen Durchgängen; Null = heutiges Verhalten, der Bestandstest bleibt grün). Nur das Entfernen setzt es (Vorschlag 5 s, wie Talos' eigene Schleife; `[VERIFIED: talos@v1.14.2/pkg/kubernetes/kubernetes.go:305-306 `case apierrors.IsTooManyRequests(err): time.Sleep(5 * time.Second)`]`).
- Wiederholt werden **nur** PDB-Vetos (Cause `DisruptionBudget`). Dauerhafte Blocker (Bare-Pod, emptyDir ohne Erlaubnis) brechen sofort ab: Warten ändert nichts.
- Frist = `Timeout` (heute `kube.DrainBudget = 10 * time.Minute`, `[VERIFIED: internal/kube/job.go:86]`). Empfehlung: diesen Wert wiederverwenden (ein Drain-Budget im Produkt). Talos nimmt 5 min.
- **Warten bis die Pods wirklich weg sind.** `Evict` kehrt zurück, sobald der Server die Löschung angenommen hat; der Pod steht danach noch terminierend auf dem Knoten (Grace Period, PreStop). `Drain` wartet darauf bewusst nicht (drain.go:44-48, nur für Umplatzierung). Für das Entfernen ist "aus etcd austreten, wipen" auf einen Knoten mit noch terminierenden Pods genau die Unordnung, die der Drain verhindern soll. Empfehlung: im Entfernen-Drain nach dem letzten Evict `PodsOnNode` abfragen, bis `wouldMove` für keinen Pod mehr gilt oder die Frist endet (derselbe `Undrained`, den `Happened` benutzt: Schritt und Prüfung beantworten dieselbe Frage).
- **Budget benennen** (siehe Code Examples): die Antwort trägt `Details.Causes[{Type: "DisruptionBudget", Message: "The disruption budget NAME needs N healthy pods and has M currently"}]`. Heute wird nur `err` (der Status-`Message`-Satz ohne Causes) eingebaut, drain.go:219 [VERIFIED: `return fmt.Errorf("a PodDisruptionBudget refuses to let this pod go right now: %w", err)`]. **Doku und Code widersprechen sich hier schon:** `docs/guide.md:1736-1738` und `docs/api-contract.md:2649` sagen "the answer names the pod and the budget", der Code nennt nur den Pod. D-01 verlangt das Budget; die Phase liefert, was die Doku schon verspricht.

### Pattern 4: Das Worker-Gate (D-02), Form und Verhalten

**Die Form des Control-Plane-Gates (zum Spiegeln)** [VERIFIED: internal/upgrade/gate.go, internal/upgrade/job.go:216-227]: `Gate.Evaluate(ctx, cluster, taking) (Verdict, error)`; `Verdict{OK, Reason, Input}` zeigt immer die Eingaben (UPG-02); die reine Funktion `decide(in, taking, machines)` entscheidet ohne Cluster; **es gibt kein Anhalten im Sinn eines Pausierens**: ein offenes Gate macht den Schritt `failed`:
```go
if !verdict.OK {
	return fmt.Errorf("upgrade: the health gate refused before %s: %s",
		nameOf(me), verdict.Reason)
}
```
Der Job steht dann `failed`; der Betreiber startet das Upgrade neu, fertige Knoten erkennt `Happened` an der Version. "Anhalten mit einem Satz" (D-02) heißt also: **Schritt schlägt fehl, ein Satz**.

**Warum ein sofortiges Spiegeln hier falsch wäre (Ermessen: erneutes Prüfen mit Frist).** Der Talos-Upgrade-Schritt wartet nur auf Version und Dienste des Knotens (`waitForUpgrade`, job.go:806-846), **nicht** darauf, dass der Kubelet Ready ist und die Pods zurück sind. Das Gate vor Worker 2 läuft also unmittelbar nach Worker 1; Readiness und PDB-Allowance sind dann typischerweise noch ein paar zehn Sekunden offen. Ein sofortiges Verweigern ließe jedes mehrknotige Worker-Rolling-Upgrade am zweiten Knoten scheitern. Empfehlung: erneut prüfen alle `pollInterval` (10 s, `[VERIFIED: internal/upgrade/job.go:638 `var pollInterval = 10 * time.Second`]`) bis `WorkerGateBudget = 5 * time.Minute` (dieselbe Größenordnung wie `ComponentBudget = 5 * time.Minute`, job.go:634, und Talos' Drain), dann verweigern mit dem letzten Befund. Preis: ein Budget, das nie frei wird (Einzelreplika), kostet fünf Minuten bis zur Verweigerung.

**Wo das Gate steht: eigener Schritt je Knoten.** Der Schritt-`Detail` wird erst beim Speichern nach `Do` veröffentlicht [VERIFIED: internal/jobs/jobs.go `run`: `err := steps[i].Do(ctx, &j)` ... `j = e.save(ctx, j)` / `e.publish(j, steps[i].Name, model.StepDone, j.Steps[i].Detail)`]; während einer fünfminütigen Wartezeit im Upgrade-Schritt sähe der Betreiber nur "running" ohne Text. Ein eigener Schritt `check Kubernetes can spare <id>` zeigt wenigstens seinen Namen. Präzedenzfall ist `power.gate` (steps.go:262-288), der für Worker "is a worker and not an etcd member; nothing to check" meldet und `Happened: never` hat. Der Gate-Schritt ist für Control-Plane-Knoten ein erklärter No-op (deren Gate bleibt im Upgrade-Schritt), für gesperrte Knoten (`me.Locked`) ebenfalls (`SkipReason`). Rollen sind beim Bauen der Schritte nicht bekannt (`talosSteps` kennt nur IDs, Rolle wird in `Do` aus dem Inventar gelesen), darum ein Schritt für **jeden** Knoten. Nachteil: die Schrittzahl ändert sich (Resume-Parken, siehe oben) und `live_test.go`/`kubernetes_job_test.go`-Erwartungen der Schrittzahl ändern sich. Alternative (Q3): im Upgrade-Schritt einbetten, keine Schrittänderung, kein sichtbarer Wartetext.

**Was das Gate prüft (präzise):**
1. **Zugang.** Nur für `ErrNoKubernetesAuthority` (Konfigurationstatsache: dieses Cluster wurde aus einer talosconfig ohne Kubernetes-Autorität adoptiert, `[VERIFIED: internal/inventory/rotate.go:216-243, kube.go:68]`) ist ein **sichtbarer** Skip erlaubt: Detail "no Kubernetes access to this cluster; the gate did not run". Jeder andere Fehler (Timeout, 5xx, TLS, 403 `ErrImpersonationRefused`) **verweigert** (fail closed): ein Cluster, dessen API nicht antwortet, ist nicht "in Ordnung".
2. **Readiness.** `Nodes()` (liefert `Ready` als `"True"|"False"|"Unknown"`, kube.go:440-470): jeder **andere** Knoten muss `Ready == "True"` sein; sonst verweigern mit Namen ("X is not Ready, so this cluster is already one node short..."). Der Knoten selbst: nicht Ready/Unknown verweigert **nicht** (ein Knoten, der schon steht, verliert durch den Reboot nichts mehr) und wird im Detail gesagt. Unbekannter Knoten im Cluster (nie beigetreten): Skip mit Detail.
3. **PDB.** Je Pod auf dem Worker, den ein Drain räumen würde (`classifyPod == podEvictable`: Phase nicht beendet, kein Mirror-Pod, kein DaemonSet, Controller vorhanden, kein ungeklärtes emptyDir; DaemonSets und Mirror-Pods ignoriert), ein **dry-run-Eviction**. 429 mit Cause `DisruptionBudget` = blockiert, Budget und Pod nennen. Alles andere (403, 500 "more than one PodDisruptionBudget", Netz) = nicht feststellbar = verweigern mit dem Grund. Der Server entscheidet damit selbst (selector, `maxUnavailable`, `unhealthyPodEvictionPolicy`, Observed-Generation); das Gate rechnet nichts nach. Siehe Don't Hand-Roll.
4. **Nie Arbeitslasten ändern.** Dry-run-Eviction ändert nichts: `[VERIFIED: kubernetes release-1.34 eviction.go:447-449 `// If this is a dry-run, we don't need to go any further than that. if dryRun { return nil }` vor dem `UpdateStatus` der PDB, und :318 `if !dryrun.IsDryRun(options.DryRun) {` vor dem Anlegen der Bedingung]`. Das Gate legt kein Cordon, keine Annotation, nichts an.
5. **Informativ, nicht verweigernd** (Ermessen, in `Detail`): Bare-Pods und Pods mit emptyDir auf dem Worker. Talos tötet sie beim Reboot ohnehin (Bare-Pod: übersprungen, dann `StopAllPods`); der Betreiber hat mit dem Upgrade den Reboot gewählt. Das Gate prüft laut D-02 nur Readiness und PDB.

**Beschränkung des dry-run:** er prüft jeden Pod einzeln gegen den aktuellen Stand. Zwei Pods desselben Budgets auf dem Worker bestehen beide, obwohl `disruptionsAllowed == 1` ist. Das ist unkritisch, weil Talos' Drain parallel evictet und den zweiten per 429-Schleife warten lässt, bis der Ersatz des ersten bereit ist. Gefährlich ist nur ein Budget mit Allowance 0, und das findet der dry-run.

**Kubernetes-Upgrade (Kubelet-Wechsel):** **kein** Worker-Gate. `kubernetesNodeStep` ändert die Kubelet-Konfiguration ohne Reboot (`applyImage` rechnet den Modus über `machineconfig.ModeFor`, job.go:492; "It is no-reboot for every path here"); ein Kubelet-Neustart räumt keine Pods (`[ASSUMED]`, siehe Assumptions). D-02 sagt "vor dem Reboot".

### Pattern 5: Zugang zum Cluster aus den Jobs

`upgrade.Deps` hat heute keinen Kubernetes-Zugang außer `KubeProxy func(ctx, cluster, image)`. Neu: `Kube func(ctx, cluster) (*kube.Client, error)`, verdrahtet in `main.go` mit `inv.KubeClient(ctx, cluster)` (genau wie `power.Deps.Kube`, main.go:507-509 [VERIFIED]). **Pro Schritt frisch holen, nie cachen:** das Zertifikat gilt eine Stunde (`certValidity = time.Hour`, identity.go/kube.go) und der Endpunkt kann sich ändern (Kommentar `KubeClient`: "Nothing is cached"). Kein Importzyklus: `internal/kube` importiert intern nur `jobs` und `model` [VERIFIED: grep der Importe], `upgrade` importiert `kube` damit zulässig.

**Fristregel:** Der Kubernetes-Client **verweigert Aufrufe ohne Frist nicht** (anders als der Talos-Client mit `WithClassDeadline`). Er hat nur eine Obergrenze je Aufruf: `Timeout: CallBudget` (60 s) in der `rest.Config` [VERIFIED: internal/kube/kube.go:109 `const CallBudget = 60 * time.Second` und :356 `Timeout: CallBudget,`; Kommentar: "A ceiling, not the deadline callers rely on: every call in this package takes its budget from the context"]. Hintergrund-Jobs laufen auf `context.WithCancel(e.shutdown)` (`jobs.go start`), also ohne Frist. Folge: jede **Schleife** (Drain-Retry, Warten auf Pods, Gate-Wiederholung) braucht eine **eigene** Frist (`context.WithTimeout`), einzelne Aufrufe sind je 60 s gedeckelt. Eine Schleife ohne Frist würde nie enden, wenn ein PDB nie frei wird.

### Anti-Patterns to Avoid
- **Pods löschen statt evicten** oder `--force` nach Frist: umgeht jedes PDB, ausdrücklich nicht gewählt (D-01, Deferred).
- **Talos' eigenen Drain für die Prüfung halten.** Er hat keine Veto-Wirkung (siehe Summary); die Phase darf `Graceful: true` im Wipe nicht als "Drain erledigt" lesen.
- **`kube.Drain`-Standard ändern.** Bricht den Test `TestADisruptionBudgetStopsADrainAndIsNotRetriedAway` und das Verhalten von `power`/`node.drain`.
- **Das Node-Objekt vor dem Wipe löschen** (Pitfall 4).
- **Fehlende Kubernetes-Erreichbarkeit still überspringen.** Nur der konfigurationsbedingte Fall (`ErrNoKubernetesAuthority`) darf überspringen, sichtbar im `Detail`.
- **Gate rechnet PDB selbst aus** (Selektoren matchen, `desiredHealthy` vergleichen): dupliziert Serverlogik und weicht von ihr ab.
- **Cluster-Zugang in `upgrade` selbst bauen** (Zertifikate, Store): gehört in die Composition Root.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| "Würde ein Budget diesen Pod jetzt halten?" | Selektor-Matching und `DisruptionsAllowed`-Rechnung | dry-run-Eviction (`DeleteOptions{DryRun: []string{metav1.DryRunAll}}`) | Der Server kennt `unhealthyPodEvictionPolicy`, Generation, mehrere Budgets (Antwort 500), Phase/Terminating-Ausnahmen (`canIgnorePDB`) |
| Budget-Name in der Meldung | Namen aus der Nachricht parsen oder PDBs listen und matchen | `Status.Details.Causes` mit Typ `policyv1.DisruptionBudgetCause`, `Message` ungeparst in den Satz | Der Server schreibt den Namen selbst hinein; Parsen bricht bei Formulierungsänderung |
| Pod-Klassifikation für den Drain | zweite Klassifikation im Gate/Job | `classifyPod`/`NodePod.wouldMove`/`Undrained` | Zweite Wahrheit würde "fertig" melden, wo der Drain noch Arbeit hätte (drain.go:310-318 sagt es selbst) |
| Kubernetes-Node zu einer Machine finden | Kopie von `findNode` | `findNode` aus `internal/power/steps.go:302-318` nach `internal/kube` heben (`FindNodeFor`), `power` ruft es auf | Zwei Kopien (Hostname zuerst, dann InternalIP) würden auseinanderlaufen |
| Rückgängig machen eines eigenen Cordons | neues Mark-Feld im Inventar | Muster aus Q2 | `PowerCordoned` bedeutet "Power-Modell schuldet ein Uncordon" und steuert dessen Regeln (`power/rules.go:92,212`) |
| Zugang zur K8s-API | eigener rest.Config/Zertifikat | `inv.KubeClient` über `Deps.Kube` | Zertifikatsprägung, Endpunktsuche, Impersonation sind dort entschieden |
| Warten mit Frist | `time.Sleep`-Schleife ohne Abbruch | `Deps.Wait`-Seam (existiert, `removejob.go:105`) bzw. `pollInterval`-Variable; `ctx.Done()` bedienen | Tests warten in Millisekunden; Abbruch/Shutdown erreichen die Schleife |

**Key insight:** Die Eviction-API ist die einzige Aufrufart, die PDBs achtet; alles, was "ungefähr dasselbe" ohne sie tut, wird beim ersten echten Budget zum erzwungenen Löschen, genau das, was D-01 ausschließt.

## Runtime State Inventory

Keine Umbenennung/Migration. Dennoch relevante Laufzeitzustände dieser Phase:

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | Gespeicherte, nicht beendete Jobs `node.remove-from-cluster` und `talos.upgrade` mit der **alten Schrittzahl** (`len(steps) != len(j.Steps)`) | Code: keiner. Verhalten: `Resume` parkt sie mit Satz. In die Release-Notiz/Guide aufnehmen. Kein Datenmigrationsschritt nötig |
| Live service config | Cluster-seitig: Cordon-Zustand (`spec.unschedulable`) und Node-Objekt eines entfernten Knotens | Durch die Schritte selbst; bei Abbruch nach Q2 |
| OS-registered state | Keiner (None, verifiziert: der Daemon registriert nichts neu) | none |
| Secrets/env vars | Keine neuen. Kubernetes-Zugang kommt aus dem gespeicherten Bündel (`ClusterSecrets`) | none |
| Build artifacts | Keine | none |

## Common Pitfalls

### Pitfall 1: Talos' Drain als Sicherheitsnetz missverstehen
**What goes wrong:** Man lässt `Graceful: true` stehen und nimmt an, ein PDB-Veto verhindere das Entfernen oder das Upgrade. **Why:** Talos' Drain loggt Fehler nur und geht weiter. **How to avoid:** Eigener Drain/Gate **vor** dem unumkehrbaren Schritt, Fehler dort. **Warning signs:** Der Test "PDB blockiert, Job läuft trotzdem durch". [VERIFIED: talos@v1.14.2 `v1alpha1_sequencer.go:311-314` Reset `taskErrorHandler(logError, CordonAndDrainNode)`; `pkg/kubernetes/kubernetes.go:285-287` `log.Printf("WARNING: failed to evict pod: %v", err)` `return nil`; `:41` `DrainTimeout = 5 * time.Minute`; Upgrade-Sequenz `v1alpha1_sequencer.go:455` ruft `CordonAndDrainNode` ohne Fehlerbehandler, die Funktion selbst gibt aber nach Warnungen `nil` zurück]. Gilt für Talos v1.14.2 (die gepinnte machinery); ältere/jüngere Knoten-Versionen `[ASSUMED]` gleich.

### Pitfall 2: 429 ist nicht immer ein PDB
**What goes wrong:** Jeder 429 wird als Budget gemeldet. **Why:** Der Server liefert 429 auch bei Konflikten ("createTooManyRequestsError", Wiederhol-Hinweis 10 s) und bei einem noch nicht verarbeiteten Budget (`ObservedGeneration < Generation`). **How to avoid:** Budget nur melden, wenn ein Cause vom Typ `DisruptionBudget` da ist; sonst "kurz zurückgewiesen, Grund: <Server-Nachricht>". Mehrere Budgets auf einem Pod liefern **500** (kein 429) und sind nie räumbar; benennen, nicht wiederholen. [VERIFIED: eviction.go:226 `Message: "This pod has more than one PodDisruptionBudget, which the eviction subresource does not support."`, :418-419, :425-427, :436-437]

### Pitfall 3: "Entfernt" ohne Warten auf terminierende Pods
**What goes wrong:** `Drain` meldet "N evicted", der Job tritt aus etcd aus und wiped, während Pods noch in ihrer Grace Period laufen. **How to avoid:** Pattern 3 (bis `Undrained` leer). **Sonderfall NotReady/Unknown-Knoten:** dessen Kubelet bestätigt keine Terminierung; die Pods stehen bis zur Pod-GC terminierend. Dann nicht bis zur Frist warten (Q6), sondern das im `Detail` sagen.

### Pitfall 4: Node-Objekt zu früh löschen
**What goes wrong:** Das Objekt wird gelöscht, solange der Kubelet noch läuft, und kommt zurück. **Why:** `[ASSUMED]` ein laufender Kubelet legt sein Node-Objekt neu an (Websuche: nicht belegt, "no source that directly documents the exact re-creation timing for a running kubelet"). **How to avoid:** nach dem Wipe löschen; `Happened` = nicht in `Nodes()`.

### Pitfall 5: Der Kubernetes-Endpunkt ist der entfernte Control-Plane-Knoten
**What goes wrong:** `inv.KubeClient` liest den Endpunkt aus der Maschinenkonfiguration des ersten antwortenden Control-Plane-Knotens (`kubernetesEndpoint`, rotate.go:290-306, `kube.EndpointFromMachineConfig`). Zeigt `cluster.controlPlane.endpoint` auf **genau die Adresse des entfernten Knotens** (kein VIP/Load-Balancer), ist die API nach dem Wipe nicht mehr erreichbar, und `delete the Kubernetes node object` scheitert nach dem Wipe. **How to avoid:** nicht aus dem Repository entscheidbar (Endpunkt ist Betreiberkonfiguration). Siehe Q4 und Beweisgrenze.

### Pitfall 6: Drain nimmt fremdes Cordon mit zurück
**What goes wrong:** Beim Abbruch uncordont der Job einen Knoten, den der Betreiber vorher selbst cordoned hatte. **How to avoid:** Nur uncordonen, wenn der Job selbst cordoned hat (Q2).

### Pitfall 7: Der Probe-Schritt macht tote Knoten schon heute unentfernbar
**What goes wrong:** Frage 5 der Aufgabe ("Entfernen eines toten Knotens muss möglich bleiben?"): es ist heute **nicht** möglich. `ReachableStep` ist Schritt 1 und schlägt für einen Knoten fehl, der Talos nicht antwortet ("A node that is down fails the job here", removejob.go:25-26); `leave etcd` und `wipe` brauchen den Knoten. Die Phase verschlechtert das nicht; ein Knoten, der Talos beantwortet, dessen **Kubelet** aber tot ist (NotReady/Unknown), ist der neue Fall (Q6).

### Pitfall 8: Das 202-Notice-Feld und der UI-Text sagen das Gegenteil
`RemoveNodeNotice` (etcd.go:354-357) sagt "so it cannot cordon or drain this node ... Run `kubectl drain <node>` first". Der Handler schickt ihn als `notice` im 202 (upgrade.go:857); die UI hat denselben Satz hart codiert (`web/src/components/NodeActions.tsx:115` Kommentar und `:202-206` Absatz); `removeapi_test.go:200-201` verlangt nur "nicht leer"; `NodeActions.test.tsx:203` nutzt `'Nothing was drained.'`. Alle vier ändern sich, `docs/api-contract.md` ("**Cordon and drain are not performed**", Schritttabelle ab Zeile 4967) und `docs/guide.md:1514-1517` ebenso.

## Code Examples

### Budget-Name aus der 429-Antwort (Quelle: Server und client-go)
```go
// Quelle: k8s.io/api policy/v1 types.go:25 (DisruptionBudgetCause = "DisruptionBudget"),
// kubernetes release-1.34 eviction.go:436-437 (Message-Format)
var status apierrors.APIStatus
if errors.As(err, &status) && apierrors.IsTooManyRequests(err) {
	if d := status.Status().Details; d != nil {
		for _, c := range d.Causes {
			if c.Type == policyv1.DisruptionBudgetCause {
				// c.Message: "The disruption budget NAME needs N healthy pods and has M currently"
				return fmt.Sprintf("%s (namespace %s): %s", podName, pod.Namespace, c.Message)
			}
		}
	}
}
```

### Dry-run-Eviction für das Worker-Gate
```go
// Quelle: eviction.go:110-125 (propagateDryRun nimmt DryRun aus eviction.DeleteOptions),
// :447-449 (Rückkehr vor dem PDB-UpdateStatus), metav1.DryRunAll = "All"
eviction := &policyv1.Eviction{
	ObjectMeta:    metav1.ObjectMeta{Name: pod.Name, Namespace: pod.Namespace},
	DeleteOptions: &metav1.DeleteOptions{DryRun: []string{metav1.DryRunAll}},
}
err := c.cs.PolicyV1().Evictions(pod.Namespace).Evict(ctx, eviction)
```

### Node löschen (nicht vorhanden, Muster Cordon)
```go
// Muster: internal/kube/drain.go Cordon (Fehler mit Verb eingepackt)
func (c *Client) DeleteNode(ctx context.Context, node string) error {
	err := c.cs.CoreV1().Nodes().Delete(ctx, node, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("kube: deleting node %s: %w", node, err)
	}
	return nil
}
```

### Abbruch mit eigenem Cordon rückgängig (Skizze)
```go
// Im Schritt "drain the node": der Bestand (internal/kube/drain.go Drain) cordont selbst.
result, err := client.Drain(ctx, node.Name, kube.DrainOptions{
	DeleteLocalData: true,              // wie power.drainOptions: der Knoten wird gewipt, emptyDir geht mit
	RetryBlocked:    kube.DrainRetry,   // NEU, 0 = heutiges Verhalten
	Timeout:         kube.DrainBudget,  // 10 min, [VERIFIED: kube/job.go:86]
})
if errors.Is(err, kube.ErrDrainBlocked) && cordonedByThisJob(job) {
	_ = client.Cordon(context.WithoutCancel(ctx), node.Name, false) // best effort, eigene kurze Frist
}
```
(Alle Namen mit `NEU` sind Vorschläge; `kube.DrainOptions`, `kube.ErrDrainBlocked`, `kube.DrainBudget`, `Cordon`, `Undrained`, `PodsOnNode` existieren [VERIFIED: drain.go:55-82,114,134; apppower.go:454; job.go:86].)

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Entfernen: nur Talos `Graceful: true` (Talos-Drain ohne Veto, Abbruch nach 5 min wird geschluckt) | Eigener Drain vor dem Entfernen mit Veto und Frist | diese Phase | PDB-Veto wird wirksam |
| Upgrade Worker: nur Talos' Drain während des Upgrade-RPC | Gate vor dem RPC (dry-run-Eviction, Readiness) | diese Phase | Kein stilles Töten nach 5 min |
| Node-Objekt bleibt nach Entfernen | `DELETE node` nach dem Wipe | diese Phase | Dashboard/`kubectl get nodes` ohne Geisterknoten |

**Deprecated/outdated:** `RemoveNodeNotice` (Text), der UI-Warnabsatz im Entfernen-Dialog, Absatz "Cordon and drain are not performed" in `docs/api-contract.md`.

## kubesim: was belegt werden kann, was fehlt

**Was kubesim heute modelliert** [VERIFIED: internal/kubesim/kubesim.go gelesen]: Cordon als Patch von `spec.unschedulable` (`serveNode`, 1303-1350; andere Patches `400`), Node-Bereitschaft (`Node.Ready`, `renderNode` 1265-1300), Pod-Liste mit Feldselektor `spec.nodeName` (1219-1262), Eviction mit `Pod.BudgetRefuses` -> 429 `TooManyRequests` (1379-1428), Eviction entfernt den Pod wirklich und merkt ihn in `Evicted()`, PDB als `Resource{Kind: "PodDisruptionBudget"}` mit `Healthy/Desired/Allowed` (2973-2989), Zugriffsprüfung (`DenyVerbs`), Impersonation, `SetUnschedulable`.

**Lücken, mit Mindestumfang für diese Phase:**

| # | Fehlt | Mindestumfang | Gebraucht für |
|---|---|---|---|
| K1 | 429 ohne `Details.Causes` und ohne Budget-Namen (Nachricht: "Cannot evict pod as it would violate the pod's disruption budget: the budget allows no further disruption", kubesim.go:1411-1418) | `Pod.BudgetName string`; 429-Body mit `Details.Causes=[{Type:"DisruptionBudget", Message:"The disruption budget <name> needs ... "}]` | Namen im Satz beweisen; Test "Name steht im Satz" ist sonst ein Test gegen Eigenerfindung |
| K2 | Budget öffnet sich nie während eines Tests | `Server.SetBudgetRefuses(namespace, name string, refuses bool) error` | Beweis "Retry wartet ein Veto ab und räumt dann" und "Frist endet" |
| K3 | Eviction ignoriert den Body, kennt kein dry-run | Body lesen; bei `deleteOptions.dryRun` ["All"] dieselben Prüfungen, **kein** Pod entfernen, **kein** `evicted`; `Server.DryRunEvictions() []string` | Gate: "ändert nie Arbeitslasten" beweisbar (Pods unverändert, `Evicted()` leer) |
| K4 | Node-DELETE (`serveNode`: alles außer GET/PATCH ist 404) | `DELETE /api/v1/nodes/{name}` entfernt Node, `Server.DeletedNodes() []string`; NotFound beim zweiten Mal | Schritt "delete the Kubernetes node object", Resume-Idempotenz |
| K5 | Readiness nicht zur Laufzeit änderbar | `Server.SetReady(name string, status corev1.ConditionStatus) error` | Gate-Wartebeweis (Knoten wird nach n Prüfungen Ready) |
| K6 (empfohlen) | terminierende Pods | `Pod.LingersAfterEviction bool`: Eviction setzt `deletionTimestamp`, Pod bleibt gelistet bis `Server.FinishTermination(ns, name)` | "Drain wartet bis weg" (Pattern 3) beweisbar |
| K7 (optional) | hängende Eviction | `Pod.EvictionHangs bool` (Muster `SummaryHangs`) | Beweis, dass die Frist greift statt zu hängen |
| K8 (optional) | 500 für "mehrere Budgets" | `Pod.BudgetsConflict bool` -> 500-Status | Gate-Satz für "nicht feststellbar" |

Mit `Server.Close()` (existiert) lässt sich "API unerreichbar" ohne neue Fähigkeit herstellen. Kubesim hat **keine eigenen Tests** (`?  ... [no test files]`); jede Erweiterung wird über die Aufrufer (`internal/kube`, `internal/upgrade`) bewiesen, und der Plan sollte je Fähigkeit mindestens einen Test mit nachgewiesener Rot-Injektion nennen.

**Was nicht beweisbar ist (Beweisgrenze, im Bericht und Ledger zu sagen):** ob ein echter API-Server dasselbe 429/500 und dieselben Causes liefert wie kubesim (nur aus dem Quelltext von release-1.34 belegt, nicht gegen einen laufenden Server); die echte PDB-Controller-Rechnung (`DisruptionsAllowed`, `unhealthyPodEvictionPolicy`); ob ein laufender Kubelet sein Node-Objekt neu anlegt; das Zusammenspiel mit Talos' eigenem Drain danach; RBAC jenseits von `SelfSubjectAccessReview`; ob der Kubernetes-Endpunkt nach dem Wipe des Control-Plane-Knotens noch antwortet (Pitfall 5); Zeitverhalten (Terminierung, Grace Periods); arm64.

## Open Questions (als Auswahl an den Betreiber)

Alle Fragen sind Auswahl; die Empfehlung steht zuerst. Sie gehören an `AskUserQuestion`, nicht in Fließtext.

**Q1. Einzelreplika-Workload (PDB mit Allowance 0) auf dem Worker beim Upgrade: Gate strikt?** D-02 nennt keinen Ausweg. Ein Homelab mit einem Worker und einem Single-Replica-Dienst mit `minAvailable: 1` kann so nie per Dienst aktualisiert werden; Talos würde ihn nach 5 min trotzdem töten.
- **(a) Strikt, ohne Ausweg (Empfehlung).** Entspricht D-01/D-02 wörtlich. Der Satz sagt, was zu tun ist (Replika erhöhen oder Budget lockern). Kosten: solche Cluster blockieren, bis der Betreiber handelt. Nutzen: kein stilles Töten, einfach, eine Regel.
- (b) Pro Upgrade eine ausdrückliche Bestätigung "Ausfall dieser Workloads akzeptieren" (Flag am Request, im Job gespeichert). Kosten: neue Request-Form, API/UI/Doku wachsen, widerspricht dem "nie Arbeitslasten ändern"-Geist nicht, öffnet aber einen Pfad, den D-02 nicht wollte. Nutzen: Single-Replica-Cluster upgradebar.
- (c) Verweigern nur bei **ungedecktem** Budget (`currentHealthy < desiredHealthy`), Allowance 0 nur als Hinweis. Kosten: schwächeres Gate, Pod wird nach 5 min doch getötet. Nutzen: kein Block für Einzelreplikas.

**Q2. Nach Abbruch des Entfernens durch ein PDB: Cordon zurücknehmen?** D-01 sagt "Der Knoten bleibt unverändert".
- **(a) Zurücknehmen, wenn der Job selbst cordoned hat (Empfehlung).** Der Cordon-Schritt merkt sich "von mir" im Job (`job.Params`, vom Schritt geschrieben und mit dem Schrittende gespeichert; ein Absturz zwischen Patch und Speichern lässt es konservativ cordoned). Kosten: neuer Mechanismus (Schreiben in `Params` aus einem Schritt ist neu; alternativ `PowerCordoned` wiederverwenden: Präzedenzfall, aber koppelt an das Power-Modell). Nutzen: erfüllt "unverändert" bis auf bereits geräumte Pods.
- (b) Cordoned lassen und im Satz sagen. Kosten: Betreiber muss manuell uncordonen; verletzt "unverändert". Nutzen: nichts zu speichern, kein Risiko, einen fremden Cordon zu lösen.

**Q3. Wo steht das Worker-Gate?**
- **(a) Eigener Schritt je Knoten (Empfehlung).** Sichtbar benannt, auch beim Warten; Muster `power.gate`. Kosten: Schrittzahl ändert sich (laufende Jobs über das Update hinweg werden geparkt, Tests der Schrittzahl ändern sich).
- (b) Im Upgrade-Schritt eingebettet. Kosten: kein Text während der bis zu 5 min Wartezeit (Detail erscheint erst am Schrittende). Nutzen: Schrittzahl unverändert.

**Q4. Node-Objekt löschen: wann, und was wenn die API danach weg ist?**
- **(a) Nach dem Wipe, Fehlschlag als `failed` mit Satz (Empfehlung).** Kubelet ist sicher weg; ein Fehlschlag nennt `kubectl delete node <name>` und sagt ehrlich, dass der Knoten schon gewipt und aus etcd ist. Davor prüft der Cordon-Schritt per `WhatMayI`, ob `delete nodes`, `patch nodes` und `create pods/eviction` erlaubt sind (Verweigerung vor jeder Änderung). Kosten: Fehlschlag steht an der spätesten Stelle; bei Pitfall 5 (Endpunkt = entfernter Knoten) scheitert **jedes** Entfernen eines solchen Control-Plane-Knotens hier. Der Inventareintrag bleibt, bis der Betreiber ihn von Hand vergisst.
- (b) Vor dem Wipe löschen (nach etcd-Austritt und Settle). Kosten: der Kubelet könnte das Objekt neu anlegen (`[ASSUMED]`), dann bleibt der Geisterknoten wie heute. Nutzen: alle Kubernetes-Arbeit vor dem unumkehrbaren Schritt, kein Endpunktproblem.
- (c) Nach dem Wipe löschen, aber ein Fehlschlag macht den Job **nicht** `failed`, sondern `Detail` sagt es, `forget the node` läuft. Kosten: leicht zu übersehen, Job "succeeded" mit Rest. Nutzen: Inventar immer aufgeräumt.

Hinweis an den Betreiber (Faktenfrage, keine Wahl): ist der Kubernetes-Endpunkt seines Clusters ein VIP/Load-Balancer oder die Adresse eines einzelnen Control-Plane-Knotens? Davon hängt Q4(a) für Control-Plane-Entfernungen ab.

**Q5. Bare-Pods (kein Controller) auf dem zu entfernenden Knoten.**
- **(a) Verweigern und benennen (Empfehlung).** Wie `node.drain` und `power`: "not coming back anywhere, the operator should decide". Kosten: Entfernen blockiert bis der Betreiber den Pod gelöscht hat. Nutzen: kein stilles Zerstören.
- (b) `Force`: Bare-Pods evicten (zerstört sie). Kosten: widerspricht dem Geist von D-01. (c) Nur melden und fortfahren; der Wipe tötet sie. Kosten: still zerstörerisch. emptyDir: Empfehlung `DeleteLocalData: true` wie `power.drainOptions` (der Knoten wird ohnehin gewipt), Anzahl im `Detail`.

**Q6. Knoten, dessen Kubelet nicht antwortet (Talos antwortet, Kubernetes-Node NotReady/Unknown)?** Evictions werden angenommen, aber nicht bestätigt; Pods bleiben terminierend.
- **(a) Evicten (PDB gilt weiter), nicht bis zur Frist auf "weg" warten, im Detail sagen "kubelet reports nothing; pods left terminating for the pod garbage collector" (Empfehlung).** Kosten: Entfernen läuft mit Pods weiter, die der Cluster später räumt. Nutzen: ein Knoten mit totem Kubelet bleibt entfernbar.
- (b) Verweigern ("drain impossible"). Kosten: genau der Fall, in dem man entfernen will, ist blockiert. Nutzen: strengste Lesart von D-01.

**Q7. Kubernetes für das Cluster grundsätzlich nicht erreichbar (Entfernen und Gate).**
- **(a) `ErrNoKubernetesAuthority` (Konfigurationstatsache): sichtbarer Skip im `Detail`; jede andere Nicht-Erreichbarkeit (Timeout, 5xx, TLS, 403): Verweigern vor jeder Änderung (Empfehlung).** Kosten: Clusters ohne K8s-Autorität bekommen weder Drain noch Gate (sagt der Satz ehrlich); API-Ausfall blockiert Entfernen/Worker-Upgrade bis sie zurück ist. Nutzen: kein stilles Überspringen, keine Regression für Altadoptionen.
- (b) Immer verweigern. Kosten: macht Entfernen/Upgrade für Cluster ohne K8s-Autorität unmöglich (Regression). (c) Immer überspringen mit Detail. Kosten: ein ausgefallener API-Server entfernt das Sicherheitsnetz genau dann, wenn es nötig ist.

**Q8. Drain-Frist.** (a) `kube.DrainBudget` (10 min) wiederverwenden (Empfehlung: ein Budget im Produkt, `power` und `node.drain` nutzen es). (b) 5 min wie Talos: kürzer, nähert sich dem Talos-Verhalten an. Reine Zahlenwahl, in Claudes Ermessen laut CONTEXT; hier nur der Vollständigkeit halber.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Ein laufender Kubelet legt sein gelöschtes Node-Objekt neu an | Pitfall 4, Q4 | Wenn nicht: Löschen vor dem Wipe (Q4b) wäre sicher und besser |
| A2 | Ein Kubelet-Neustart (Config-Apply `no-reboot`) räumt keine Pods; darum kein Worker-Gate im Kubernetes-Upgrade | Pattern 4 | Wenn doch: Gate auch dort nötig |
| A3 | Talos-Knoten anderer Versionen als v1.14.2 drainen gleich (Fehler nur geloggt, 5 min) | Pitfall 1 | Ältere Versionen könnten härter oder weicher sein; Wirkung auf Begründung, nicht auf Design |
| A4 | Pods auf einem NotReady/Unknown-Knoten bleiben terminierend, bis die Pod-GC sie räumt | Pitfall 3, Q6 | Kurze Wartezeit könnte reichen; Wirkung auf Q6 |
| A5 | Der Kubernetes-Eviction-Server-Code von release-1.34 entspricht dem der Cluster-Version des Betreibers (Kubernetes 1.34/1.35) | kubesim, Pattern 3/4 | Causes-Format oder dry-run-Verhalten weichen ab; nur ein echter Cluster zeigt es |
| A6 | Die Talos-Upgrade-Sequenz cordont und drained auch bei Workern ohne `skipNodeRegistration` | Pitfall 1 | Mit gesetztem `skipNodeRegistration` drainet Talos nicht: Gate bliebe sinnvoll, die Begründung "Talos tötet nach 5 min" träfe nicht zu |

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Build/Tests | ja | 1.27.2 lokal (Projekt 1.26.9 per `GOTOOLCHAIN`) | `GOTOOLCHAIN=local` bis zum Gate |
| Node/npm | UI-Tests (`NodeActions.test.tsx`) | nicht geprüft | — | `web/`-Tests laufen in CI; Textänderung in der UI ist klein |
| echter Cluster / Pi | Beweis der Beweisgrenze | nein | — | nur Simulatoren; im Bericht sagen |
| Netz | Quelltext-Abruf | ja | — | — |

**Missing dependencies with no fallback:** echter Cluster (bewusst; nicht blockierend, aber Beweisgrenze).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` (stdlib), `httptest`-basierte `kubesim`/`talossim`, echte `jobs.Engine` über `fsstore` in Temp-Verzeichnis |
| Config file | keine (`./bin/task ci` ist das Gate) |
| Quick run command | `GOTOOLCHAIN=local go test ./internal/upgrade/ ./internal/kube/ ./internal/power/ ./internal/jobs/ -count=1` (Baseline: alle ok, ca. 20 s) |
| Full suite command | `./bin/task ci` (enthält Linter, `internal/publicrepo`, `internal/depguard_test.go`, Web) |

### Phase Requirements → Test Map (Verhalten, benannte Injektion je Kernaussage)

Jede Zeile braucht den Beleg, dass der Test **rot** wird, wenn der Fehler eingebaut ist (Projektmethode). Die Injektionsspalte ist der Fehler, den der Plan wieder einbauen und rot sehen muss.

| Kernaussage | Test (Typ) | Injektion (muss rot werden) | File Exists? |
|---|---|---|---|
| Entfernen cordont und drained **vor** `leave etcd` und `wipe` (Reihenfolge, `atWait`-Muster aus `TestRemovingAControlPlaneNodeRunsEveryStepInOrder`) | `removejob_test.go` (Integration, kubesim+talossim) | Schritt "drain the node" aus der Liste nehmen / hinter `wipe` stellen | ❌ Wave 0 (Rig braucht kubesim + `Deps.Kube`) |
| Drain übersprungen -> Pods noch auf dem Knoten beim Wipe | derselbe: beim `wipe`-Zeitpunkt `sim.Pods` auf dem Knoten leer | `drain`-`Do` zum No-op machen | ❌ Wave 0 |
| PDB-Veto bricht den Job **vor** dem Entfernen ab, nennt Budget und Pod | `removejob_test.go`: Job `failed`, `talossim` zeigt 0 Leaves/0 Resets, Satz enthält `BudgetName` und Pod | Veto ignorieren (`Blocked` verwerfen) / Budgetnamen weglassen | ❌ Wave 0 (braucht K1) |
| Kein erzwungenes Löschen: nach Veto `Deleted()` und `Evicted()` leer | gleicher Test | Fallback auf Pod-DELETE nach Frist einbauen | ❌ Wave 0 |
| Retry wartet ein Veto ab: Budget öffnet sich (K2) -> Drain gelingt, Eviction wurde >1 mal versucht | `kube/drain_test.go` | `RetryBlocked` ignorieren (ein Durchgang) |  ❌ Wave 0 (K2) |
| Frist endet: Veto bleibt -> `ErrDrainBlocked` nach Frist, nicht Hänger | `kube/drain_test.go` mit kurzem `Timeout`/Intervall | Frist entfernen (Endlosschleife; Test läuft in Timeout) | ❌ Wave 0 |
| Bestehendes Verhalten unverändert: `RetryBlocked == 0` = genau ein Eviction-Aufruf | bestehend `TestADisruptionBudgetStopsADrainAndIsNotRetriedAway` | Standard auf Retry ändern -> dieser Test rot | ✅ |
| Eigenes Cordon wird nach Abbruch zurückgenommen, fremdes nicht | `removejob_test.go` (zwei Fälle: vorher schedulable / vorher cordoned) | Uncordon immer / nie ausführen | ❌ Wave 0 |
| Node-Objekt wird gelöscht, **nach** dem Wipe, nicht davor | `removejob_test.go`: `DeletedNodes()==[name]`, Zeitpunkt über `atWait`-Muster; Resume-Fall "Neustart vor dem Löschen" löscht noch, "nach dem Löschen" fehlerfrei | Schritt weglassen / vor `wipe` stellen | ❌ Wave 0 (K4) |
| Neustart in `drain` räumt weiter, parkt nicht; Neustart in `wipe` parkt weiterhin | `removejob_test.go` im Stil `TestARestartInsideTheWipeParksTheJobAndDoesNotWipeAgain` (removejob_test.go:578) und `crash_test.go`-Rig | `Happened` des Drains auf `nil` setzen (würde parken); `Happened` des Wipes auf etwas setzen (würde doppelt wipen) | ❌ Wave 0 |
| Kubernetes nicht erreichbar -> Job scheitert im Cordon-Schritt, **nichts geändert** (talossim 0/0) | `removejob_test.go` mit `ksim.Close()` | Fehler schlucken ("skip") | ❌ Wave 0 |
| `ErrNoKubernetesAuthority` -> sichtbarer Skip im `Detail`, Job läuft | `removejob_test.go` | Skip auch bei Verbindungsfehler | ❌ Wave 0 |
| Berechtigung fehlt (`DenyVerbs: delete:nodes`) -> Verweigerung vor jeder Änderung | `removejob_test.go` | Prüfung entfernen | ❌ Wave 0 |
| Worker-Gate hält bei NotReady-Nachbarn mit Satz, der den Knoten nennt | `workergate_test.go` (reine `decide`-Tests, Muster `gate_test.go`) | Readiness-Prüfung entfernen | ❌ Wave 0 |
| Worker-Gate hält bei PDB-Veto (dry-run), nennt Budget; **ändert nichts** (`Evicted()` leer, `DryRunEvictions()` gefüllt) | `workergate_test.go` (Integration, K3) | Echte statt dry-run-Eviction schicken -> `Evicted()` nicht leer, rot | ❌ Wave 0 |
| Gate wartet: Knoten wird nach n Runden Ready -> OK; sonst nach `WorkerGateBudget` Verweigerung | `workergate_test.go` mit `Deps.Wait`/`pollInterval` in ms und K5 | Wiederholung entfernen (sofort verweigern) | ❌ Wave 0 |
| Gate umgangen: Verweigerung stoppt den Upgrade-Schritt, `ImagePull`/`Upgrade` wird **nicht** aufgerufen | `live_test.go`-Stil (talossim zählt Calls) | Gate-Schritt aus `talosSteps` entfernen / Ergebnis ignorieren | ❌ Wave 0 |
| Gate ist für Control-Plane und gesperrte Knoten ein erklärter No-op | `workergate_test.go` | Gate auch für CP verweigern lassen | ❌ Wave 0 |
| Kubernetes-Upgrade bekommt **kein** Worker-Gate | `kubernetes_job_test.go` (Schrittliste unverändert) | Gate-Schritt in `kubernetesSteps` einbauen | ✅ erweitern |
| Doku und UI-Text sagen nicht mehr "cannot cordon or drain" | grep-artiger Test oder bestehender `web`-Test `NodeActions.test.tsx` angepasst; `removeapi_test.go` prüft Notice-Inhalt statt "nicht leer" | alten Text wieder einsetzen | ✅ anpassen |

### Sampling Rate
- **Per task commit:** `GOTOOLCHAIN=local go test ./internal/<betroffenes Paket>/ -count=1`
- **Per wave merge:** Quick-Befehl oben plus `go vet ./...`
- **Phase gate:** `./bin/task ci` grün **und** Linter `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...` vor `/gsd-verify-work`. Auf dem Pi läuft kein `go test -race` (nur CI).

### Wave 0 Gaps
- [ ] `internal/kubesim/kubesim.go`: K1 bis K5 (K6 empfohlen), je mit Test über den Aufrufer
- [ ] `internal/upgrade/removejob_test.go`: Rig um kubesim erweitern (Muster `internal/power/rig_test.go:145-206`: `kubesim.New` mit `AuthorityCrt/Key` des talossim-Clusters, `kube.MintCreds`, `kube.New`) und `Deps.Kube` setzen
- [ ] `internal/upgrade/workergate_test.go` (neu)
- [ ] `internal/kube/drain_test.go`: Fälle für `RetryBlocked`, Budgetname, DeleteNode
- [ ] Bestehende Schrittlisten-Tests anpassen: `removejob_test.go:268-285`, `:308-322`, `live_test.go`/`kubernetes_job_test.go` je nach Q3

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | nein (keine neue Anmeldung) | unverändert |
| V3 Session Management | nein | unverändert |
| V4 Access Control | **ja** | Route bleibt `destructive` (Sudo), `operator`, auditiert als `node.remove-from-cluster`; Kubernetes-Seite: Client handelt als `ActAs`-Identität des Clusters, **eine Verweigerung wird nie als Admin wiederholt** (identity.go); `EveryPermissionThisProductUses` um `delete nodes` ergänzen und das Budget-Zeilen-Guard in `budget_test.go:273-279` mitziehen |
| V5 Input Validation | ja (klein) | Knotenname kommt aus der Inventar-/Kubernetes-Liste, nicht aus dem Request, und geht in einen Feldselektor `spec.nodeName=`+Name (drain.go:146); bei `FindNodeFor` nur Namen aus `Nodes()` weitergeben, nie rohen Request-Text |
| V6 Cryptography | nein | Zertifikatsprägung unverändert (`MintCreds`) |

### Known Threat Patterns for diese Phase

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Datenverlust durch erzwungenes Löschen von Pods | Tampering | Eviction-API statt Delete, kein Force nach Frist (D-01) |
| Entfernen mit einer Identität, die `delete nodes` nicht darf, schlägt erst nach dem Wipe fehl | Denial of Service (halb entfernter Knoten) | `WhatMayI` vor dem ersten Schritt, der etwas ändert |
| Fehlender Audit für neue Kubernetes-Änderungen | Repudiation | Die Route ist bereits auditiert; Schritte stehen im Job (`Detail`); Cluster-Audit sieht die `ActAs`-Identität |
| Betreiberdaten im Repository (Hostnamen in Tests/Fixtures) | Information Disclosure | Dokumentationswerte (192.168.1.10, homeserver) in kubesim-Fixtures und Doku; `internal/publicrepo` |

## Sources

### Primary (HIGH confidence)
- Repository, in dieser Sitzung gelesen: `internal/upgrade/removejob.go`, `gate.go`, `job.go`, `etcd.go:330-387`, `service.go:656-669`; `internal/kube/drain.go`, `job.go`, `kube.go`, `identity.go`, `resources.go:160-225`; `internal/power/steps.go`; `internal/jobs/jobs.go`; `internal/kubesim/kubesim.go` (Optionen, Nodes, Eviction, Node-Handler, PDB-Liste); `internal/depguard_test.go`; `internal/model/inventory.go:247-255`, `job.go:95-126`; `internal/inventory/rotate.go:205-330`; `cmd/holzkube-managerd/main.go:395-520`; `docs/guide.md`, `docs/api-contract.md`; `web/src/components/NodeActions.tsx`; `internal/httpapi/handlers/upgrade.go:780-960`.
- Kubernetes `release-1.34` `pkg/registry/core/pod/storage/eviction.go` (komplett heruntergeladen und per `grep`/`sed` gelesen: dry-run, 429-Causes, 500 für mehrere Budgets, `canIgnorePDB`).
- `k8s.io/api@v0.34.1/policy/v1/types.go` im Modulcache (`DisruptionBudgetCause`).
- Talos `v1.14.2` im Modulcache: `internal/app/machined/pkg/runtime/v1alpha1/v1alpha1_sequencer.go` (Reset 311-314, Shutdown, Upgrade 455), `v1alpha1_sequencer_tasks.go:753-790` (`CordonAndDrainNode`), `pkg/kubernetes/kubernetes.go:41,242-312` (Drain, Evict, Timeout), `internal/app/machined/pkg/controllers/k8s/node_apply.go:466-486` (Uncordon nur für von Talos gesetztes Cordon).

### Secondary (MEDIUM confidence)
- Websuche zu Talos-Upgrade/Cordon/Drain (Drittguides und Talos-Dokumentation `docs.siderolabs.com/talos/v1.12/.../upgrading-talos`): bestätigt, dass Talos beim Upgrade selbst cordont und drained; ob dabei PDBs geachtet werden, dokumentiert keine gefundene Seite, daher der Quelltext oben.
- Zusammenfassende Wiedergabe von `eviction.go` (WebFetch) deckt sich mit der Originaldatei, die zusätzlich gelesen wurde.

### Tertiary (LOW confidence)
- Websuche zu "Node-Objekt nach `kubectl delete node` bei laufendem Kubelet": nicht belegt (A1).

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, keine neuen Pakete, alle Bausteine im Repo gelesen.
- Architecture: HIGH für Schrittfolge/Resume (Engine-Semantik gelesen); MEDIUM für Gate-Schrittform (Q3, Q1 sind Betreiberentscheidungen).
- Pitfalls: HIGH für Talos-/Kubernetes-Verhalten aus Quelltext; LOW für A1/A4.

**Research date:** 2026-10-16
**Valid until:** 2026-11-16 (Kubernetes-/Talos-Quellen stabil; neu prüfen, wenn der Talos-Pin angehoben wird)
