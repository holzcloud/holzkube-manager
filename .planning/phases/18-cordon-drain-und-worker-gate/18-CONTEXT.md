# Phase 18: Cordon, Drain und Worker-Gate - Context

**Gathered:** 2026-10-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Zwei zusammengehörige Kubernetes-Sicherheitsnetze. (1) Das Entfernen eines Knotens (Job aus v0.5.0, `internal/scale`/Entfernen) markiert den Knoten unschedulbar, drained ihn mit Frist und löscht danach sein Node-Objekt; heute drained es nicht, und das Node-Objekt bleibt zurück. (2) Im Upgrade bekommt jeder Worker vor dem Reboot ein Gate (Readiness, PodDisruptionBudget), so wie die Control Plane heute schon eines hat. Beides braucht den Kubernetes-Client (`internal/kube`), bisher nur gegen `internal/kubesim` bewiesen.

</domain>

<decisions>
## Implementation Decisions

Alle als Auswahl vom Betreiber am 2026-10-16 getroffen, sofern nicht als Ermessen von Claude gekennzeichnet.

- **D-01:** Blockiert beim Entfernen ein PodDisruptionBudget den Drain, läuft der Drain **mit Frist**, danach **bricht der Job vor dem Entfernen ab** und nennt das blockierende Budget. Der Knoten bleibt unverändert, nichts wird halb entfernt. **Kein erzwungenes Löschen von Pods.** — **Reversibility:** reversible.
- **D-02:** Das Worker-Gate im Upgrade prüft vor dem Reboot eines Workers Readiness der Knoten und PodDisruptionBudgets und hält den Upgrade-Schritt mit einem Satz an, wenn das Gate offen ist, so wie das Control-Plane-Gate (`upgrade`, etcd-Gesundheitstor). Es ändert nie Arbeitslasten.

- **D-03:** (Betreiber bestätigt 2026-10-16: alle Empfehlungen der Open Questions in `18-RESEARCH.md`.) Q1 Einzelreplika-PDB beim Worker-Upgrade: **strikt, ohne Ausweg**. Q2 ein eigenes Cordon wird nach einem PDB-Abbruch **zurückgenommen** (Merker in `job.Params`). Q3 das Worker-Gate ist ein **eigener Schritt je Knoten** (Vorbild `power.gate`), mit erneutem Prüfen alle 10 s bis `WorkerGateBudget` 5 min; Readiness heißt alle anderen Knoten Ready; die PDB-Prüfung ist eine **dry-run-Eviction**. Q4 das Node-Objekt wird **nach dem Wipe** gelöscht, ein Fehlschlag ist `failed` mit Satz. Q5 **Bare-Pods** auf dem zu entfernenden Knoten werden verweigert und benannt, emptyDir mit `DeleteLocalData: true` wie `power`. Q6 ein Knoten mit **totem Kubelet**: evicten, aber nicht auf „weg" warten, im Detail sagen. Q7 Kubernetes nicht erreichbar: sichtbarer Skip nur bei `ErrNoKubernetesAuthority`, sonst **vor jeder Änderung verweigern**. Q8 `DrainBudget` 10 min. Neue Option `DrainOptions.RetryBlocked` (0 = bisheriges Verhalten), nur das Entfernen setzt sie; `Drain` wird nicht global auf Retry umgestellt. Die Prüfung steht **vor dem ersten unumkehrbaren Talos-Aufruf**, weil Talos selbst nach 5 min trotz PDB entfernt. Offene Faktenfrage an den Betreiber (Handprobe): ist der Kubernetes-Endpunkt ein VIP oder die Adresse eines einzelnen Control-Plane-Knotens (Pitfall 5)?

### Claude's Discretion
Die Frist des Drains, wie das Gate seine Bedingung formuliert, ob es ein erneutes Prüfen mit Frist oder ein sofortiges Anhalten gibt, Reihenfolge der Schritte im Job (cordon, drain, Entfernen aus etcd, Node-Objekt löschen), der Umgang mit Knoten, die nicht antworten (Drain unmöglich: ehrlich sagen, nie still überspringen). Die Beweisgrenze: nur gegen `kubesim`, ein echter Cluster hat es nicht gesehen — im Bericht und im Ledger zu sagen.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

- `CLAUDE.md` — öffentliches Repository, README im selben Schritt, Changelog erst beim Release, jeder Wächter rot gesehen, Fragen als Auswahl, Linter lokal: `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...`.
- `.planning/AUDIT-2026-10-08.md` — Quelle des Kandidaten.
- `internal/upgrade` (Job, Control-Plane-Gate, `applyImage`), `internal/scale` und das Entfernen-Job aus v0.5.0 (`internal/jobs`, Node-Entfernung), `internal/kube` und `internal/kubesim` (Cordon, Evictions, PodDisruptionBudgets), `docs/api-contract.md`, `docs/guide.md`.
- Muster für Aufbau und Beweis: `internal/certwatch` und `internal/configdrift` (periodischer Leser, „not read yet", Wächter vom Kontext **ohne Frist** aus — der Talos-Client verweigert Aufrufe ohne Frist, siehe Quick-Task 261014-gl2), die Phasen 15 und 16 unter `.planning/phases/`.

</canonical_refs>

<deferred>
## Deferred Ideas

- Erzwungenes Entfernen nach Frist (bewusst nicht gewählt).
- Rückbau von Upgrades (Audit: „groß", bewusst fehlend).
- Cordon/Drain für Reboot und Power-Aktionen außerhalb des Entfernens.

</deferred>

---

*Phase: 18-cordon-drain-und-worker-gate*
*Context gathered: 2026-10-16*
