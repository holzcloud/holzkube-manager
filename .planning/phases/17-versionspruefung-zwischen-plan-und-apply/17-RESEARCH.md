# Phase 17: Versionsprüfung zwischen Plan und Apply - Research

**Researched:** 2026-10-16
**Domain:** Go-Daemon (`internal/machineconfig`, `internal/httpapi`), Talos-Machinery v1.14.2, React-Oberfläche (`web/src/routes/config.tsx`)
**Confidence:** HIGH für Ablauf, Digest-Wahl, Fehlerform und Beweisbarkeit (alles im Code gelesen); MEDIUM für das Verhalten eines echten Knotens (Re-Encoding der Konfiguration, Dauer des Fensters), das hier nicht geprüft werden kann.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Bei Abweichung zwischen Plan und Apply wird das Apply **verweigert**, der Betreiber plant neu. **Kein Überschreib-Schalter.** — **Reversibility:** reversible — ein späterer „trotzdem anwenden"-Weg wäre additiv.
- **D-02:** Der Satz der Verweigerung nennt, **was sich geändert hat**, über alle Dokumente der Konfiguration (also mit `machineconfig.CompareDocuments` aus Phase 15, nicht mit dem Ein-Dokument-`Compare`), redigiert wie die Konfigurationsansicht.

### Claude's Discretion
Wie der Digest durch Plan und Bestätigung getragen wird (Token der Bestätigung, Feld der Vorschau), Fehlercode und HTTP-Status der Verweigerung (409 mit eigenem Problemcode, im API-Vertrag), Wortlaut. Das Verhältnis zur Drift-Basis aus Phase 15 (ein eigenes Apply frischt die Basis über den erwarteten Digest auf) darf sich nicht verändern.

### Deferred Ideas (OUT OF SCOPE)
- Ein „trotzdem anwenden"-Weg mit Sudo (bewusst nicht gewählt).
- Dasselbe Prinzip für andere Aktionen (Upgrade, Reboot), die auf einem Plan beruhen.
</user_constraints>

<phase_requirements>
## Phase Requirements

ROADMAP und REQUIREMENTS führen für Phase 17 noch keine IDs (`**Requirements**: TBD`). Vorschlag für den Planer (eigene Namen, nicht vom Betreiber bestätigt, `[ASSUMED]`):

| ID | Beschreibung | Research-Grundlage |
|----|--------------|--------------------|
| VERS-01 | Die Vorschau (Plan) trägt `base_digest`, den Digest des Snapshots der gelesenen aktiven Konfiguration | Frage 1, 2: `Snapshot(raw).Digest` |
| VERS-02 | Das Apply verlangt `base_digest`, liest erneut und verweigert bei Abweichung, ohne Überschreib-Schalter, und sendet dann nichts | Frage 1, 3: Prüfung vor `ApplyConfigurationWithMode`; Feld ist Pflicht |
| VERS-03 | Der Satz der Verweigerung nennt die Änderungen über alle Dokumente (`CompareDocuments`), ohne Geheimnisse, Werte und Adressen | Frage 4 |
| VERS-04 | Ein eigenes Apply landet wie bisher und frischt die Drift-Basis über den erwarteten Digest auf; ein verweigertes Apply hinterlässt keine Erwartung | Frage 3 |
| VERS-05 | Zwei gleichzeitige Applies auf einen Knoten aus diesem Prozess überschreiben einander nicht still (Sperre je Knoten) | Frage 3 |
| VERS-06 | Die Oberfläche zeigt die Verweigerung als Satz und bietet „neu planen" an; README, Anleitung und API-Vertrag sind im selben Schritt nachgezogen | Frage 6 |
</phase_requirements>

## Summary

**Die Annahme „Token der Bestätigung" aus CONTEXT.md trifft auf die Konfigurationsroute nicht zu.** `POST /api/v1/machines/{id}/config/apply` benutzt den `jobs.Confirmer` nicht. Die Route ist zustandslos: die Oberfläche schickt die Patches ein zweites Mal, der Server liest den Knoten erneut und patcht dessen *dann aktuelle* Rohkonfiguration. Die einzige Bestätigung ist das Sudo-Fenster (`Destructive: true`) und ein Klick. Es gibt deshalb kein Token, das man erweitern könnte; der Digest reist als gewöhnliches Feld: `base_digest` in der Antwort des Plans und im Body des Applys. [VERIFIED: internal/httpapi/handlers/config.go:63-72, 168-208 — die Apply-Route nennt `Destructive: true`, `ClusterScope: clusterFromBody`, `Action: "config.apply"` und ruft `d.Config.Apply(ctx, ..., patches, machineconfig.Mode(body.Mode))`; `Confirmer` kommt nur in upgrade.go, jobs.go, host.go, authority.go, provision.go vor]

**Empfehlung zur Digest-Frage:** Der Digest ist `Snapshot(raw).Digest` (Phase 15, normalisiert und redigiert, alle Dokumente) und reicht aus, **weil das Apply die frisch gelesene Rohkonfiguration patcht und nicht die des Plans**: eine fremd rotierte Geheimnisfarbe wird vom Apply mitgenommen und nie zurückgedreht. Was der Digest schützen muss, ist die Übereinstimmung zwischen dem, was der Betreiber gesehen hat (redigierter Diff), und dem, was geschieht. Geheimnisfarben sind in dieser Ansicht nie sichtbar gewesen. Die Grenze (reine Geheimnisänderung wird nicht verweigert) ist dieselbe wie Ledger 178 und gehört in Anleitung und Wächtertest, nicht in eine Sonderlösung.

**Zwei Dinge sind notwendig, ohne die der Digest eine Scheinsicherheit wäre:** (a) die Prüfung muss gegen **dieselben Rohbytes** laufen, die anschließend gepatcht und gesendet werden (kein zweites Lesen zwischen Prüfung und Patch), und (b) eine **Sperre je Knoten** im `Service` von der Prüfung bis zur Rückkehr des RPC. Ohne (b) lesen zwei gleichzeitige Applies beide D0, bestehen beide die Prüfung und der zweite überschreibt den ersten still: genau der Fehler, den die Phase schließen soll, innerhalb eines Prozesses. Ein Schreiber außerhalb (talosctl) im Fenster zwischen Lesen und RPC ist nicht schließbar, weil `ApplyConfigurationRequest` keine Vorbedingung kennt; das Fenster lässt sich nur verkürzen (ein Client für Lesen und Senden).

**Primary recommendation:** `Preview.BaseDigest` aus `Snapshot(base)`; `Apply` bekommt `baseDigest` als Pflichtparameter (leer = Verweigerung, damit kein Weglassen zum Überschreib-Schalter wird); Prüfung und Patch auf denselben Rohbytes, unter einer Sperre je Knoten, vor dem einzigen `ApplyConfigurationWithMode`; Abweichung = `409 conflict.config-changed-since-plan` mit einem Satz aus `CompareDocuments(planText, jetztText)` (nur Pfade und Art, keine Werte); der Plan-Text kommt aus einem kleinen begrenzten Speicher im Prozess und fehlt er, wird trotzdem verweigert, nur mit allgemeinerem Satz.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Digest der gelesenen Konfiguration bilden | API / Backend (`internal/machineconfig`) | — | Nur dort liegen die Rohbytes; sie verlassen das Paket nie (`Snapshot` ist der einzige Ausgang) |
| Digest zwischen Plan und Apply tragen | Browser / Client (Feld in Vorschau, Feld im Apply-Body) | API / Backend | Die Route ist zustandslos; der Client hält den Wert zwischen den beiden Aufrufen |
| Prüfen und Verweigern | API / Backend (`Service.Apply`) | — | Muss vor dem RPC und unter der Knotensperre passieren; ein Client kann das nicht erzwingen |
| Sperre gleichzeitiger Applies | API / Backend (`Service`) | — | Gleiche Begründung wie `reserveStaged`: Prüfung und Marke in einem Schritt |
| Satz „was hat sich geändert" | API / Backend (`machineconfig`) | Browser (zeigt `detail`) | `CompareDocuments` und Redaktion sind Backend; der Client zeigt nur den Satz |
| Verweigerung zeigen, neu planen anbieten | Browser / Client (`config.tsx`) | — | Reine Darstellung und ein erneuter Plan-Aufruf |
| Drift-Basis nicht verändern | API / Backend (`configdrift.ApplyObserver` am Talos-Client) | — | Der Beobachter hängt am RPC; eine Verweigerung davor lässt ihn unberührt |

## Standard Stack

Keine neuen Pakete. Alles, was die Phase braucht, liegt im Repository und in der bereits gepinnten Machinery.

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `internal/machineconfig` (`Snapshot`, `CompareDocuments`, `Redact`, `ApplyPatches`) | Repo | Digest, Satz, Patchen | Phase 15 hat genau diese Funktionen für „alle Dokumente, redigiert, normalisiert" gebaut |
| `github.com/siderolabs/talos/pkg/machinery` | v1.14.2 | Konfiguration laden, `ApplyConfigurationRequest` | [VERIFIED: go.mod:13 `github.com/siderolabs/talos/pkg/machinery v1.14.2`] |
| `crypto/sha256`, `encoding/hex`, `sync` | Go-Standard | Digest steht schon in `Snapshot`; Sperre je Knoten | nichts Eigenes bauen |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `internal/talossim` | Repo | Echter Talos-Client gegen simulierten Knoten (`setMachineConfig` ersetzt die aktive Konfiguration wirklich) | alle Beweise auf dem Mac; Dritter = zweiter Client mit `ApplyConfigurationWithMode` |
| Vitest (jsdom-Projekt) | Repo | UI-Beweis | `NODE_OPTIONS=--no-experimental-webstorage npx vitest run --project jsdom` |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Digest über redigierten Snapshot | Hash der Rohbytes | fängt Geheimnisänderungen, aber: erste Hash-Ausgabe unredigierter Konfiguration aus dem Paket, und jede Re-Encodierung durch einen Talos-Upgrade ergäbe Verweigerung **ohne** nennbare Änderung (leerer Satz, gegen D-02) |
| Plan-Text im Prozessspeicher für den Satz | Snapshot-Text im Apply-Body zurückschicken | zustandslos und restart-fest, aber API-Form „ein Feld" wird „zehn KB zurückschicken" (siehe Auswahl 2) |

**Installation:** keine. `go get` entfällt. **Package Legitimacy Audit:** entfällt, es werden keine externen Pakete installiert.

## Architecture Patterns

### System Architecture Diagram

```
 Browser (config.tsx)                     Daemon                                   Knoten (Talos)
 ────────────────────                     ──────                                   ──────────────
 "See what would change"
   POST .../config/plan {patches}  ──►  planConfig ──► Service.Plan
                                          read(raw0) ─────────────────────────────► COSI Get (aktive Konfig)
                                          snap0 = Snapshot(raw0)  (Digest D0, Text T0)
                                          memo[machine][D0] = T0  (begrenzt, nur Prozessspeicher)
                                          patched = ApplyPatches(raw0, patches); Compare; ModeFor; validate
   Preview{..., base_digest: D0}  ◄──────  
 Betreiber liest, wählt Modus
   POST .../config/apply            ──►  [Sudo-Fenster, Cluster-Sperre] ──► applyConfig
     {patches, mode, cluster,              base_digest-Form prüfen (64 Hex), Pflicht   ──► 400 validation.failed
      base_digest: D0}                     Service.Apply:
                                             DryRun?                                   ──► 403 forbidden.dry-run
                                             Sperre je Knoten holen (ctx-fähig)
                                             staged? reserveStaged                     ──► 409 store.staged-pending
                                             cc = Connect (EIN Client)
                                             raw1 = cc.MachineConfigYAML ────────────► COSI Get
                                             snap1 = Snapshot(raw1)
                                             snap1.Digest != D0 ?  ──ja──► CompareDocuments(memo[D0] ?? "", T1)
                                                                          abort(); Sperre frei; KEIN RPC, KEIN Beobachter
                                                                          ──► 409 conflict.config-changed-since-plan + Satz
                                             patched = ApplyPatches(raw1, patches)    (dieselben Rohbytes)
                                             cc.ApplyConfigurationWithMode(patched) ──► Beobachter (Phase 15: erwarteter Digest)
                                                                                    ──► ApplyConfiguration RPC
                                             commit/abort; Sperre frei
   200 {mode, details}  oder  Satz im Fehlerfeld ("failure") + "neu planen"
```

### Recommended Project Structure
```
internal/machineconfig/
├── service.go        # Plan trägt BaseDigest; Apply(ctx,id,patches,mode,baseDigest); Knotensperre
├── baseline.go       # NEU: ErrChangedSincePlan, ChangedSincePlanError + Satz, Plan-Speicher (begrenzt)
└── *_test.go         # NEU: changedsinceplan_test.go (echter Client gegen talossim)
internal/httpapi/
├── problem.go        # CodeConfigChangedSincePlan
└── handlers/config.go# planBody.BaseDigest, Pflicht + Form im Apply, writeConfigError-Zweig
docs/api-contract.md  # Zeile in der Code-Tabelle + Abschnitt im Config-Kapitel
internal/audit/redact.go # "config.apply" erlaubt zusätzlich "base_digest" (Hash, kein Geheimnis)
web/src/api.ts, web/src/routes/config.tsx, web/src/routes/config.test.tsx (NEU)
```

### Pattern 1: Prüfung und Patch auf denselben Rohbytes (TOCTOU-minimal)
**What:** `apply` liest **einmal**, bildet daraus den Snapshot zur Prüfung **und** patcht dieselben Bytes. Kein zweites Lesen zwischen Prüfung und Senden.
**When to use:** immer; ein zweites Lesen wäre die Variante „Digest über das Falsche".
**Example (Skelett, Entwurf; Namen aus service.go:223-282):**
```go
// Entwurf [ASSUMED]; ErrStagedAlreadyPending, reserveStaged, ApplyPatches,
// ApplyConfigurationWithMode und Snapshot sind im Repository gelesen.
func (s *Service) Apply(ctx context.Context, id model.MachineID, patches []string, mode Mode, baseDigest string) (talos.ApplyResult, error) {
	if s.deps.Mode.DryRun {
		return talos.ApplyResult{}, ErrDryRun
	}
	if baseDigest == "" { // Pflicht: Weglassen darf kein Überschreib-Schalter sein (D-01)
		return talos.ApplyResult{}, ErrBaseDigestRequired
	}
	release, err := s.lockMachine(ctx, id) // je Knoten, ctx-fähig
	if err != nil {
		return talos.ApplyResult{}, err
	}
	defer release()
	// ... reserveStaged wie bisher, danach apply(ctx, id, patches, mode, baseDigest)
}

// im RPC-Teil, mit EINEM Client:
//   raw1 := cc.MachineConfigYAML(ctx)
//   snap1, err := Snapshot(raw1)
//   if snap1.Digest != baseDigest { return s.changedSincePlan(id, baseDigest, snap1) }
//   patched := ApplyPatches(raw1, patches)
//   return cc.ApplyConfigurationWithMode(ctx, patched, string(mode))
```

### Pattern 2: Verweigerung als typisierter Fehler mit festem Satz
**What:** `ChangedSincePlanError` mit `Paths []string` und `Error()` als fertigem Satz; `errors.Is(err, ErrChangedSincePlan)`. Der Handler mappt in `writeConfigError` wie schon `ErrStagedAlreadyPending` (`httpapi.Conflict(code, err.Error())`). [VERIFIED: internal/httpapi/handlers/config.go:380-383 `case errors.Is(err, machineconfig.ErrStagedAlreadyPending): httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeStagedPending, err.Error()))`]
**Why:** `Problem.Detail` ist die einzige Stelle, an der der Satz die Oberfläche erreicht (`ProblemError` setzt `super(problem.detail ...)`, `config.tsx` zeigt `e.message`). [VERIFIED: web/src/lib/problem.ts — `super(problem.detail && problem.detail.length > 0 ? problem.detail : problem.title)`]

### Pattern 3: Der Digest als Vorbedingung, nicht als Berechtigung
Der Wert ist eine Vorbedingung (If-Match): wer ihn hat, darf *neu planen* und bekommt einen frischen. Er ist deshalb kein Geheimnis und keine Autorisierung; Fälschbarkeit ist unkritisch (siehe Sicherheit). Die Form ist dieselbe wie beim Drift-Bestätigen: 64 Kleinbuchstaben-Hex. [VERIFIED: internal/httpapi/handlers/configdrift.go:70 `var driftDigestShape = regexp.MustCompile(`^[0-9a-f]{64}$`)`] Die Variable kann wiederverwendet (umbenannt oder aliasiert) werden.

### Anti-Patterns to Avoid
- **Optionales `base_digest`:** ein fehlendes Feld würde zum Überschreib-Schalter, den D-01 ausschließt. Pflichtfeld, `400` mit `FieldError{Field: "base_digest", Reason: "required"}`.
- **Eine zweite Methode `ApplyUnchecked`/Fallback im Service:** zweite Sendestelle und Hintertür. Der Wächter `TestOnlyKnownCallersSendAConfigurationToAManagedNode` hält `internal/machineconfig/service.go` bei genau einem Aufruf. [VERIFIED: internal/configdrift/callers_guard_test.go — `filepath.Join("internal", "machineconfig", "service.go"): {1,`]
- **Digest aus einem zweiten `read` nach der Prüfung:** Prüfung und Patch lägen auf verschiedenen Bytes.
- **Satz aus `Compare`:** liest nur das erste Dokument (Ledger 176); eine Änderung an `KubeletConfig.image` ergäbe einen leeren Satz.
- **Werte in den Satz schreiben (`Change.Before/After`):** siehe Frage 4.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Normalisierter, redigierter Digest über alle Dokumente | eigenen Hash/YAML-Normalisierer | `machineconfig.Snapshot` | Reihenfolge der Dokumente, Kommentare, Redaktion und der PEM-Gürtel sind dort gelöst und getestet |
| Diff über alle Dokumente | Zeilendiff oder `Compare` | `machineconfig.CompareDocuments(before, after string)` | Paare `Dokument.pfad`, erkennt hinzugekommene/entfallene Dokumente, nie `nil` |
| Redaktion | Regex auf Konfigurationstext | `Redact` (nur über `Snapshot`/`CompareDocuments`) | Fünf Ausgänge, ein Gürtel |
| Atomares Prüfen-und-Markieren | `if pending {…}; mark()` | Muster `reserveStaged` (Prüfung und Marke unter einem Lock) | Der Fehler „beide sehen nichts Offenes" ist dort dokumentiert |
| Problemform | eigenes JSON | `httpapi.Conflict(code, detail)` | 409, `TypeConflict`, Taxonomie geschlossen |
| Digest-Formprüfung | neue Regex | `driftDigestShape` | gleiche Form, gleiche Meldungsart |

**Key insight:** Die Phase besteht aus Verdrahtung vorhandener, bewiesener Teile an der richtigen Stelle (vor dem RPC, auf denselben Bytes, unter der Sperre). Jede eigene Normalisierung oder jeder eigene Vergleich wäre eine zweite Wahrheit über „gleiche Konfiguration".

## Frage-für-Frage-Antworten

### (1) Der Ablauf heute, und wo der Digest reist

**Service.Plan** (`service.go:177-216`): `s.read` → `ApplyPatches(base, patches)` → `Compare(base, patched)` (Ein-Dokument) → `Redact(patched)` → `ModeFor(diff.Paths)` → `idempotentSet` → `validate`. Antwort `Preview{Machine, Diff, Verdict, Result, Idempotent, Valid, Validation}`. Nichts davon trägt heute einen Bezug zum gelesenen Stand. [VERIFIED: internal/machineconfig/service.go:152-216]

**Service.Apply** (`service.go:223-262`): DryRun → `ErrDryRun`; bei `ModeStaged` `reserveStaged` (Prüfung und Marke unter `s.mu`); `s.apply` (**liest erneut**, `ApplyPatches`, `Connect` ein *zweites* Mal, `ApplyConfigurationWithMode`); bei Fehler `abort()`, sonst `commit(result.Details)`; bei `ModeReboot`/`ModeTry` wird der Staged-Eintrag gelöscht. [VERIFIED: internal/machineconfig/service.go:223-282] Das Apply wendet also **die Patches** auf den Stand *zum Zeitpunkt des Applys* an, nicht das Ergebnis des Plans.

**HTTP:** `POST /api/v1/machines/{id}/config/plan` (Operator, Aktion `config.plan`) und `.../config/apply` (Operator, `Destructive: true`, `Action: "config.apply"`, `ClusterScope: clusterFromBody`). Beide dekodieren denselben `planBody{Cluster, Patches, PatchIDs, Mode}` mit `DisallowUnknownFields`; `Mode` ist auf dem Apply Pflicht (`strings.TrimSpace(body.Mode) == ""` → `Validation(... FieldError{Field: "mode", Reason: "required"})`). [VERIFIED: internal/httpapi/handlers/config.go:118-135, 180-186; internal/httpapi/handlers/inventory.go:280-283 `dec.DisallowUnknownFields()`]

**Bestätigung:** Es gibt **keinen** Confirmer-Token auf dieser Route (siehe Summary). Der Klick „Apply" ruft `api.config.apply(machineID, [patch], mode, cluster)` und schickt den **Patch-Text erneut**, nicht den Plan. Ein 428 (`sudo.required`) öffnet den Sudo-Prompt und wiederholt die Anfrage byte-genau. [VERIFIED: web/src/routes/config.tsx:230-231 `mutationFn: () => api.config.apply(machineID, [patch], mode, cluster),`; web/src/api.ts:3965-3972; Replay: web/src/api.ts:819 „so the sudo replay is byte for byte the request that was refused"] Ein im Body mitgeschicktes `base_digest` überlebt den Replay unverändert.

**Folgen für die Gestaltung:**
- Digest-Träger: `Preview.base_digest` (Plan-Antwort) → `base_digest` im Apply-Body. Der Zod-Schema `previewSchema` und `api.config.apply` bekommen das Feld; `ConfigPreview` wird um `base_digest` ergänzt.
- „Was muss der Token zusätzlich binden?" entfällt. Die Frage „bindet der Digest die Patches?" ist die nach einem **Ergebnis-Digest**: Ein Apply mit *anderen* Patches als geplant fiele durch einen reinen Basis-Digest. Das ist für das Ziel der Phase („jemand anderes hat die Konfiguration geändert") nicht nötig und gehört, wenn überhaupt, zu den zurückgestellten Ideen (Prinzip für andere Aktionen). Empfehlung: nicht binden.
- Bonus: `Snapshot.Digest` ist derselbe Wert, den die Drift-Ansicht zeigt und den `POST .../config/drift/confirm` als Compare-and-Swap nimmt. Ein Betreiber kann beide vergleichen; ein gemeinsamer Begriff, keine zweite Hash-Art.

### (2) Worüber der Digest läuft, und reicht der redigierte?

**Optionen:**
| | Digest über | Fängt | Kostet |
|---|---|---|---|
| **A (empfohlen)** | `Snapshot(raw).Digest` (redigiert, normalisiert, alle Dokumente) | jede sichtbare Änderung in jedem Dokument | reine Geheimnisänderung bleibt unsichtbar (Ledger 178) |
| B | Hash der Rohbytes | auch Geheimnisänderungen | erste Hash-Ausgabe unredigierter Bytes aus dem Paket; jede Re-Encodierung auf dem Knoten (Talos-Upgrade, andere Maschinery-Version) verweigert **ohne** nennbare Änderung → leerer Satz, gegen D-02 |
| C | A plus Digest je Geheimnisfeld | auch Geheimnisänderungen, mit Namen | Eingriff in `Redact`/Snapshot, entscheidet Ledger 178 nebenbei; mehr Umfang als „klein" |

**Warum A genügt (die eigentliche Frage):** Das Apply patcht die **frisch gelesene Rohkonfiguration** (`base` in `apply()` kommt aus `s.read`, nicht aus dem Plan). Rotiert jemand zwischen Plan und Apply ein Geheimnis (Token, Cluster-Secret, Schlüssel), dann trägt das Apply den neuen Wert unverändert weiter: es wird nichts zurückgedreht, solange der Patch das Feld nicht selbst setzt. Die Verweigerung soll verhindern, dass der Betreiber etwas bestätigt, das er so nicht gesehen hat. Geheimnisfarben waren in der Vorschau nie sichtbar (`Redact` ersetzt jede durch `RedactedMarker`). Eine Verweigerung wegen einer Änderung, die er nie hätte beurteilen können, wäre kein Gewinn. [VERIFIED: internal/machineconfig/snapshot.go:33-36 „Redact replaces every secret by the same constant, so a change of nothing but a secret value, a rotated token, is invisible in a snapshot. That is wanted (D-02), and a test holds it"] [VERIFIED: internal/machineconfig/redact.go:28 `const RedactedMarker = "<redacted by holzkube-manager>"`]

**Was dabei wahr sein muss, und deshalb einen Wächter braucht:** das Apply darf die Rohbytes des *Plans* nie wiederverwenden (z. B. aus dem Speicher des Plans patchen, um das Fenster zu verkürzen). Dann würde eine fremd rotierte Farbe still zurückgedreht, genau der Schaden, den A nicht bemerkt. Test T8 unten beweist es und ist mit der Einspritzung „patcht Plan-Rohbytes" rot zu sehen.

**Zwei Fälle, in denen Digest-Abweichung und leerer Diff zusammenfallen können** (selten, aber der Planer braucht eine Regel): (i) Snapshot-Text unterscheidet sich, `walk` nennt aber keine Änderung (`3` gegen `"3"`, Schlüsselreihenfolge): dann **trotzdem verweigern** (fail-closed) mit allgemeinem Satz („…in der Form, ohne dass sich eine Einstellung unterscheidet; neu planen"). (ii) Gespeicherter Plan-Text fehlt: siehe Frage 4. Beides ist im Satzbauer als eigener Zweig zu testen.

**Grenzen von A, die in die Anleitung gehören (Zeile im Abschnitt Configuration drift, „Known limits"):** Reine Geheimnisänderungen zwischen Plan und Apply werden nicht verweigert (Ledger 178, gleiche Entscheidung D-02 Phase 15). Die Zeile über `Compare` und das Ein-Dokument-Plan (Ledger 176) bleibt bestehen; **die Phase schließt 176 nicht**, weil D-02 es nicht verlangt (siehe Auswahl 3). [VERIFIED: docs/guide.md:1450-1452 „The comparison reads every document, but the Config view's **Plan** still only reads the first one, so a patch to another document shows an empty diff there (ledger 176)."]

### (3) Das Rennen, Staged, Gleichzeitigkeit, Drift-Basis

| Fenster | Wirkung heute | Mit Phase 17 |
|---|---|---|
| Plan → Apply (Minuten) | stilles Überschreiben/Stützen auf alten Diff | **verweigert** (Digest) |
| Prüfung → RPC im selben Apply (ms bis s) | n/a | Rest-Fenster: Talos kennt keine Vorbedingung. [VERIFIED: ~/go/pkg/mod/github.com/siderolabs/talos/pkg/machinery@v1.14.2/api/machine/machine.pb.go:843-851 — `type ApplyConfigurationRequest struct` hat nur `Data`, `Mode`, `DryRun`, `TryModeTimeout`] Verkürzen durch **einen** Client für Lesen und Senden (heute: Lesen schließt, `Connect` öffnet neu, mit TLS und Version-Probe); schließen lässt es sich nicht |
| Zwei Applies aus diesem Daemon, gleichzeitig | beide lesen D0, beide senden, der zweite überschreibt still | **Sperre je Knoten** von der Prüfung bis zum RPC-Ende: der zweite liest D1 und wird verweigert |
| Schreiber außerhalb (talosctl) im Rest-Fenster | still überschrieben | unverändert möglich; **ehrlich dokumentieren** |

**Was der Planer aus der Sperre machen muss:** Eine Map von Knoten zu einem Slot (z. B. Kanal der Größe 1), geholt mit `select` auf `ctx.Done()`, damit die Wartezeit im 30-s-Budget der Route bleibt (`ConfigRouteBudget`) und kein Request hängt. Die Sperre wird **vor** `reserveStaged` geholt und mit `defer` freigegeben. Sie gilt nur für `Service.Apply`; `Plan` und `Get` blockieren nicht. Sie serialisiert nicht gegen Job-Applies (Upgrade, CA-Rotation); die lesen und schreiben in Millisekunden und würden einen vorher geplanten Apply durch den Digest ohnehin verweigern lassen.

**Staged / Reboot (`reserveStaged`):** `MachineConfigYAML` liefert die **aktive** Konfiguration; ein wartendes Staged ändert sie nicht. Ein Plan, ein Staged-Apply und ein weiterer Plan haben deshalb denselben Digest, es gibt keine falsche Verweigerung. Landet das Staged beim Reboot, ändert sich die aktive Konfiguration, ein davor erstellter Plan wird zu Recht verweigert. Die Verweigerung muss durch den **bestehenden** Fehlerpfad laufen (`abort()` in `Apply`), damit eine verweigerte Staged-Anfrage keine Marke hinterlässt; Test T6. Die Reihenfolge der Fehler: DryRun, fehlender Digest, Sperre, `ErrStagedAlreadyPending` (billig, ohne Knotenlesen), dann Digest-Prüfung. `try`: ein Plan während des Rückrollfensters wird nach dem Rollback verweigert und der Satz nennt die zurückgekehrten Pfade; das ist richtig und braucht nur einen Hinweis im Satz („kann auch ein Apply von dir selbst sein").

**Verhältnis zur Drift-Basis (Phase 15), das sich nicht ändern darf:** Der Beobachter hängt am Talos-Client und wird **innerhalb** von `ApplyConfigurationWithMode` vor dem RPC gerufen. [VERIFIED: internal/talos/client.go:1055-1056 `done := c.observeApply(ctx, cfg, mode)` / `defer func() { done(err) }()`] Eine Verweigerung **davor** erreicht den Beobachter nie, hinterlässt also keine Erwartung (`Expected`). Ein durchgelassenes Apply läuft wie bisher: der Beobachter bildet `machineconfig.Snapshot(a.Config)` der gesendeten Bytes und legt `model.ExpectedConfig{Digest, Mode, At}` ab. [VERIFIED: internal/configdrift/apply.go:59, 72 `snap, err := machineconfig.Snapshot(a.Config)` … `exp := model.ExpectedConfig{Digest: snap.Digest, Mode: a.Mode, At: s.deps.Now()}`] Damit das so bleibt: genau **ein** `ApplyConfigurationWithMode` in `service.go` (Wächter `{1,`), die Prüfung strikt vor ihm, der Client weiterhin aus `Connect` (der den Beobachter hängt: `inventory/mutate.go` `cc.SetApplyObserver(observer)`).

**Neu beobachtbare Nebenwirkung, die beabsichtigt ist:** Ein zweiter Klick auf „Apply" derselben Vorschau nach einem erfolgreichen Apply wird verweigert (der Digest hat sich durch das eigene Apply geändert). Heute würde er den Patch ein zweites Mal anwenden, bei einem Listen-Append mit Duplikat (CFG-05). Die Oberfläche soll die Vorschau nach Erfolg ohnehin sperren.

### (4) Fehlerform

- **Code:** `conflict.config-changed-since-plan`, Konstante `CodeConfigChangedSincePlan` in `problem.go`. **Status:** 409 über `httpapi.Conflict(code, detail)` (Type `TypeConflict`). Vergleichbare Namen: `conflict.drift-changed` (anderer Sinn: Digest des Bestätigens), `store.staged-pending`. Eigener Code, weil der Weg heraus (neu planen) ein anderer ist als bei allen bestehenden. [VERIFIED: internal/httpapi/problem.go:345 `CodeDriftChanged = "conflict.drift-changed"` und :555-563 `func Conflict(code, detail string) *Problem` mit `Status: http.StatusConflict`]
- **Fehlendes oder falsch geformtes `base_digest`:** `400` über `httpapi.Validation(...)` mit `FieldError{Field: "base_digest", Reason: "required"}` bzw. „must be 64 lower-case hexadecimal characters" (Code `validation.failed`). Keine neue Code-Konstante.
- **Dazu nötig, sonst rot:** `TestEveryProblemCodeIsInTheContract` verlangt den Code-Text irgendwo in `docs/api-contract.md`; `TestEveryProblemCodeIsEmitted` verlangt eine Verwendung des Bezeichners im Code. [VERIFIED: internal/httpapi/contract_codes_test.go:14-60 `strings.Contains(doc, value)`] Einzutragen: eine Zeile in der Tabelle „Codes minted here" (api-contract.md um Zeile 5043-5057, dort stehen `conflict.drift-changed` u. a.) und der Satz im Kapitel „Machine configuration" (um Zeile 4232-4270: Routen-Tabelle `apply` und Abschnitt zum zweiten `staged`-Apply). Außerdem die Routenzeile: `apply` verlangt `base_digest`, `plan` liefert es.
- **Satz (Entwurf, englisch, wie die UI):** „Nothing was applied: the configuration of this node changed after the plan was made. Changed since the plan: KubeletConfig.image (modified), v1alpha1.machine.certSANs (list changed, 3 entries became 4), and 2 more. That can be another operator, talosctl, or an apply of your own. Plan again to see what your patch does on top of the current configuration."
- **Inhalt des Satzes:** Pfade mit Dokumentpräfix (aus `Diff.Changes[].Path`), Art (`added`/`removed`/`modified`/`list-changed`) und bei Listen die Längen; höchstens ~8 Pfade, dann „and N more". **Keine Werte** (`Change.Before/After`). Begründung: Die Werte sind redigiert, aber Redaktion kennt nur Geheimnisse; Hostnamen, Adressen und SANs bleiben darin. Pfade genügen für „was hat sich geändert"; die Werte sieht der Betreiber im neuen Plan und auf der Knotenseite. Das ist eine Ermessensentscheidung innerhalb von D-02 („redigiert wie die Konfigurationsansicht", die Werte *zeigt*); wenn der Betreiber Werte im Satz will, ist es eine Zeile Änderung, kostet aber den Wächter „keine Adresse im Satz". `[ASSUMED]`: dass „keine Adressen" aus dem Auftrag den Satz meint und nicht nur das Repository.
- **Geheimnisse:** nicht möglich, wenn der Satz nur aus `CompareDocuments` kommt (beide Seiten gehen erneut durch `Snapshot`). Der Audit protokolliert nur den Code, nie den Text: [VERIFIED: internal/httpapi/middleware/audit.go:93-98 „The stable taxonomy code, never the handler's own text" / `cause = errors.New(sniffer.code())`].
- **Plan-Text für den Satz (Auswahl 2):** `CompareDocuments` braucht den *Vorher*-Text. Empfehlung: kleiner, begrenzter Speicher im `Service` (`digest → Snapshot.Text` je Knoten, z. B. 4 Einträge je Knoten, Gesamtgrenze, Lebensdauer ~1 h; nur Prozessspeicher, redigiert wie jede Ansicht). Fehlt der Eintrag (Neustart des Daemons durch das stündliche Update, Verdrängung), wird **trotzdem verweigert**, mit dem allgemeineren Satz: „…changed after the plan was made, and the earlier state is no longer held, so the difference cannot be named. Plan again." Sicherheit hängt am Digest, nicht am Speicher; der Speicher ist nur Wortlaut. Dieser Zweig braucht einen eigenen Test (Speicher geleert, Apply verweigert trotzdem).
- **Fehler beim Snapshot (`ErrSnapshotUnreadable`/`TooLarge`/`Unsafe`):** `Plan` scheitert bereits heute früh in `Compare→Redact` (Klartext-Fehler, im Handler ungemappt, also `500 internal.unexpected` über `writeInventoryError`). Neu hinzu kommt nur die Obergrenze `MaxSnapshotBytes` (1 MiB). Der Planer soll die drei Snapshot-Fehler im Handler auf einen bestehenden 409/Upstream-Code mappen oder ausdrücklich als 500 belassen; keine neue Konstante nötig.

### (5) Andere Aufrufer

- `Service.Plan` und `Service.Apply` haben **genau einen** Produktionsaufrufer: die beiden HTTP-Handler in `handlers/config.go:159, 197`. [VERIFIED: grep nach `Config.Apply`/`Config.Plan`/`.Plan(` über `internal`, `cmd`: nur `internal/httpapi/handlers/config.go:159` und `:197`]
- **`holzkubectl` ist nicht betroffen:** es hat keine Konfigurations-Plan/Apply-Befehle (nur `kubeconfig` und `talosconfig`). [VERIFIED: cmd/holzkubectl/commands.go:36-37, 106-109]
- **Jobs, Upgrade, CA-Rotation:** rufen `Service.Apply` nicht. `upgrade.applyImage` (lesen → patchen → senden, `job.go:455-500`) und `rotateca.Deps.rewrite` (`job.go:~421-480`) machen Lesen-Ändern-Schreiben innerhalb einer Funktion mit `ApplyConfigurationWithMode` direkt; sie haben keinen Plan, auf den ein Digest passt. Das ist die zurückgestellte Idee („dasselbe Prinzip für andere Aktionen"), nicht Teil dieser Phase. Der Wächter `knownSenders` führt sie als eigene Sendestellen; sie bleiben unverändert.
- **Zu aktualisierende Testaufrufer** (Signaturwechsel von `Apply`): `internal/configdrift/apply_test.go:63`, `callers_test.go:42`, `expected_test.go:545` und `:694`. Sie rufen `own.Apply(ctx, id, []string{patch}, Mode…)` ohne vorherigen Plan. Sie werden mit einem Hilfsaufruf „Plan, dann Apply mit dessen `BaseDigest`" angepasst; das macht sie realistischer. Sie beweisen weiter, dass ein eigenes Apply die Basis auffrischt (VERS-04). `internal/httpapi/configdriftapi_test.go:657` schickt einen Apply-Body ohne `base_digest` gegen einen **gesperrten** Cluster und erwartet 403 `forbidden.cluster-locked`; die Cluster-Sperre liegt vor dem Handler (`middleware.ClusterLock` ist „outside the sudo gate", vor `next.ServeHTTP`), der Test bleibt grün; das ist als Nebenbeobachtung wert, dass die Sperre vor der Validierung des Digests antwortet. [VERIFIED: internal/httpapi/configdriftapi_test.go:657-659; internal/httpapi/middleware/clusterlock.go:38-57]

### (6) Oberfläche

- **Wo sie erscheint:** `PreviewCard` in `web/src/routes/config.tsx` zeigt Fehler des Applys bereits in `failure` (`<p className="text-destructive">`). Der Satz erscheint dort ohne Zutun, weil `ProblemError.message` das `detail` ist. Ohne weitere Arbeit ist das aber ein roter Absatz unter einem noch klickbaren „Apply".
- **Was der Betreiber tun soll:** neu planen. Empfehlung: bei Code `conflict.config-changed-since-plan` die Vorschau als **veraltet** markieren (amberfarbener Kasten wie die vorhandenen Hinweise), „Apply" sperren und einen Knopf „See what would change again" anbieten, der denselben Plan-Aufruf mit dem **unveränderten Patch-Text** wiederholt. Der Satz bleibt dabei lesbar stehen, bis der neue Plan da ist (Zustand in `NodeConfig`, nicht in `PreviewCard`, sonst verschwindet er mit der Vorschau).
- **Nach Erfolg:** Vorschau sperren oder schließen (`applied` ist schon gesetzt); ein zweiter Klick würde ohnehin verweigert.
- **Das Digest-Feld:** `previewSchema` bekommt `base_digest: z.string()`; `PreviewCard` reicht `preview.base_digest` an `api.config.apply` durch. `api.config.plan` schickt weiter `cluster: ''`/`mode: ''`; `planBody` darf `base_digest` mitführen und beim Plan ignorieren (ein Struct, `DisallowUnknownFields`).
- **Presentation:** `conflict.`-Codes stehen in `PRESENTATION_BY_CODE` nicht; sie fallen auf `toast`. [VERIFIED: web/src/lib/problem.ts, Liste `['validation.', 'field-errors'], … ['internal.', 'toast']` ohne `conflict.`] Für diese Phase genügt die lokale Behandlung per `error.code`; die Tabelle muss nicht erweitert werden.
- **README / Anleitung (Pflicht nach CLAUDE.md, im selben Schritt):** README „What it does": ein Halbsatz (am besten an einem vorhandenen Eintrag zur Konfiguration, die einzige Konfigurationsnennung dort ist der Drift-Eintrag, Zeile 54); Tiefe in `docs/guide.md` (neuer Unterabschnitt „Applying a patch" oder im Abschnitt Configuration drift; Grenzen: Rest-Fenster, Geheimnisfarben, Ledger 176). Ein Screenshot ist nur nötig, wenn ein eigener Bildschirm entsteht; die Verweigerung ist ein Zustand der bestehenden Seite, Phase 15 hat ohne Bild entschieden (`STATE.md`: „README ohne Bild (Betreiber 2026-10-14)"). Empfehlung: kein Bild, außer der Betreiber verlangt es; dann Daten in `web/fixtures/demo.json` und `task build && node web/scripts/readme-images.mjs`. Der Changelog wird **nicht** angefasst (gehört zum Release).

### (7) Beweisbarkeit auf dem Mac

Alles Folgende läuft gegen den **echten Talos-Client** und den simulierten Knoten; `talossim` ersetzt bei `ApplyConfiguration` die aktive Konfiguration wirklich (`setMachineConfig`: Destroy und Create der MachineConfig-Ressource), und ein Dritter ist ein zweiter Client mit `ApplyConfigurationWithMode`. [VERIFIED: internal/talossim/machine.go:858-876 `m.server.node.applyConfig()` … `if m.server.opts.Cluster != nil { if err := m.server.setMachineConfig(ctx, req.GetData()); …` und internal/talossim/cosi.go:93-125 „It is what makes ApplyConfiguration an operation rather than a counter"] Das Muster für „Dritter ändert ein Dokument, das nicht das erste ist" steht in `snapshot_node_test.go` (zweiter Client, `ApplyPatches` mit `kind: KubeletConfig`, `no-reboot`) und `configdrift/helpers_test.go:215-236` (`thirdPartyChange`).

**Das Rennen im Rest-Fenster ist nicht deterministisch beweisbar und wird nicht behauptet.** Beweisbar ist: die Prüfung läuft auf denselben Bytes (T8/T2), die Sperre serialisiert (T4), der Beobachter hört eine verweigerte Anfrage nie (T5). Für T4 gibt es keinen Haken im Simulator zwischen Lesen und RPC; der Dienst nimmt aber einen `Connect`, und `ClusterClient.SetApplyObserver` ist in `machineconfig`-Tests am eigenen Client setzbar (Beobachter ist der Haken „kurz vor dem RPC"). [VERIFIED: internal/talos/applyobserver.go:46 `func (c *ClusterClient) SetApplyObserver(o ApplyObserver) { c.observer = o }`]

| # | Behauptung | Test (Name als Vorschlag) | Eingespritzter Fehler, der rot werden muss |
|---|---|---|---|
| T1 | Ein Apply nach Fremdänderung wird verweigert, nichts wird gesendet, der Knoten behält den Fremdstand, der Satz nennt den Pfad | `TestAnApplyIsRefusedWhenTheNodeChangedSinceThePlan` (Dritter ändert `KubeletConfig.image` **und** getrennt ein Feld im ersten Dokument) | **F1** Prüfung entfernt (immer weiter); **F3** Digest-Vergleich ignoriert (`!=` zu `false`) |
| T1b | „Verweigerung, die trotzdem anwendet": Fehler zurück, aber RPC lief | derselbe Test prüft zusätzlich, dass der Knoten **nicht** den Patch trägt | **F4** Prüfung nach `ApplyConfigurationWithMode` |
| T2 | Ein Apply auf unverändertem Knoten landet | `TestAnApplyOnAnUnchangedNodeStillLands` | **F2** Digest über das Falsche (`Snapshot(patched)` statt `Snapshot(raw1)`; oder Plan-Digest über `patched`): verweigert immer → rot |
| T3 | Kein Weglassen als Überschreib-Schalter | `TestAnApplyWithoutADigestIsRefused` (leer, kurz, Großbuchstaben, sowohl Service als auch HTTP `400`) | **F5** leerer Digest = „nicht prüfen" |
| T4 | Zwei gleichzeitige Applies auf einen Knoten: einer landet, einer wird verweigert; die zweite Änderung überschreibt die erste nicht | `TestTwoAppliesOnOneNodeDoNotOverwriteEachOther` | **F6** Sperre je Knoten entfernt. Hinweis: zeitgestützt (Beobachter des ersten Applys hält begrenzt, z. B. bis ein Kanal oder 200 ms); die rote Sicht muss mit entfernter Sperre gesehen werden, das Ergebnis mit Sperre ist unabhängig von der Zeit |
| T5 | Verweigerung hinterlässt **keine** Erwartung in der Drift-Basis; die Fremdänderung gilt später als Drift, nicht als „eigen" | `TestARefusedApplyLeavesNoExpectationInTheBaseline` (`configdrift`-Paket, Beobachter am Client wie `apply_test.go`) | **F4** (RPC und damit Beobachter laufen), **F1** |
| T5b | Ein durchgelassenes eigenes Apply frischt die Basis weiter auf (VERS-04) | die vier umgestellten Bestandstests (`apply_test.go`, `callers_test.go`, `expected_test.go`) | Beobachter entfernt aus `Connect` (Bestandsfehler, bleibt rot) |
| T6 | Verweigertes Staged-Apply gibt seine Marke frei; danach geht ein frisches Staged | `TestARefusedStagedApplyWithdrawsItsClaim` | **F7** `abort()` auf dem Prüfpfad entfernt (zweites Staged: `ErrStagedAlreadyPending`) |
| T7 | Satz: Pfade aus allen Dokumenten, gekappt, ohne Wert, ohne Geheimnis, ohne Adresse; Zweige „Speicher leer" und „Digest anders, Diff leer" | `TestTheRefusalSentenceNamesPathsAndNothingElse` (Werte `192.168.1.10`, Geheimnis aus `secretsOf`, `RedactedMarker`) | **F8** Satz aus `Compare` statt `CompareDocuments` (Änderung in `KubeletConfig` → leer); **F9** `Change.After` im Satz (Adresse taucht auf) |
| T8 | Digest über den redigierten Snapshot genügt: Eine reine Geheimnisänderung wird **nicht** verweigert, und das Apply **trägt den neuen Wert weiter** | `TestASecretRotatedSincethePlanIsCarriedForwardNotReverted` (Dritter ersetzt `machine.token` o. ä.; danach liest der Test die Rohkonfiguration vom Knoten: neuer Wert vorhanden, alter nicht) | **F10** Apply patcht die beim Plan gelesenen Rohbytes (etwa aus einem Merker) → alter Wert wieder drin → rot. Das ist der Beweis zu Frage 2 |
| T9 | HTTP-Grenze: `plan` liefert 64 Hex; `apply` ohne Feld `400` `base_digest`; veraltet `409` mit Code und Pfad im `detail`, ohne Geheimnisse; Erfolg `200`; Audit zeigt den Code | in `internal/httpapi/configapi_test.go` (Harness `newConfigHarness` existiert) | Handler ohne Pflichtprüfung; Handler mappt auf `500`/falschen Code |
| T10 | Vertragswächter | `TestEveryProblemCodeIsInTheContract`, `TestEveryProblemCodeIsEmitted` | Zeile im Vertrag weglassen → rot |
| T11 | UI: Apply schickt `preview.base_digest`; Verweigerung zeigt den Satz, sperrt „Apply" und bietet erneutes Planen mit demselben Patch | neue Datei `web/src/routes/config.test.tsx` (jsdom; `api.config.*` gemockt) | **F11** UI schickt leeren/alten Digest; UI zeigt nur den Toast ohne Sperre |
| T12 | Genau eine Sendestelle bleibt | `TestOnlyKnownCallersSendAConfigurationToAManagedNode` (unverändert grün) | zweite Sendestelle (Fallback) → rot |
| T13 | Audit-Erlaubnisliste | vorhandener Test über `internal/audit/redact.go` | `base_digest` fehlt in der Liste: das Feld wird verworfen (kein Fehler); Entscheidung, ob erlaubt (Empfehlung ja: Hash des redigierten Textes, sagt, welchen Stand der Betreiber bestätigte) |

CLAUDE.md verlangt „ein Wächter ist erst etwas wert, wenn er gegen den bewusst zurückgesetzten Fehler rot gesehen wurde". Die Spalte F1 bis F11 ist als **verbindliche Abnahme je Plan** gedacht: der Plan nennt die Einspritzung, führt sie aus, nennt den roten Test und stellt den Fehler wieder her.

## Runtime State Inventory

Nicht zutreffend: Es ist keine Umbenennung, Refaktorierung oder Migration. Vorsorglich: Es wird **kein** Zustand auf Platte hinzugefügt (Plan-Speicher nur im Prozess, Sperre nur im Prozess); kein Datenverzeichnis-Schema ändert sich; `ConfigBaseline` und `ExpectedConfig` bleiben unberührt.

## Common Pitfalls

### Pitfall 1: Digest optional gelassen
**What goes wrong:** Ältere Clients/Skripte lassen das Feld weg und überschreiben weiter still.
**Why:** „Abwärtskompatibel" wirkt freundlich.
**How to avoid:** Pflichtfeld (T3); das Produkt ist Alpha, Brüche sind erlaubt und angekündigt; der einzige Client ist die Oberfläche.
**Warning signs:** `if body.BaseDigest != ""` im Handler oder Service.

### Pitfall 2: Prüfung und Patch auf verschiedenen Lesungen
**What goes wrong:** Prüfung grün, dann ein zweites `s.read`, dann Patch: Fremdänderung im Zwischenraum.
**How to avoid:** Eine Lesung, `raw1` für Snapshot und `ApplyPatches` (Muster 1); T2/T8.
**Warning signs:** zwei Aufrufe von `read`/`MachineConfigYAML` in `apply`.

### Pitfall 3: Die Sperre fehlt oder hält zu lange
**What goes wrong:** Ohne Sperre: stilles Überschreiben im Prozess. Mit Sperre ohne `ctx`: hängender Request, wenn ein Apply am Knoten hängt.
**How to avoid:** Slot je Knoten mit `select` auf `ctx.Done()`; Wartezeit im 30-s-Budget. Freigabe mit `defer`, auch auf jedem Fehlerpfad.
**Warning signs:** `sync.Mutex` je Knoten ohne Abbruchweg; Sperre erst nach dem Lesen geholt.

### Pitfall 4: Der Beobachter hört eine verweigerte Anfrage
**What goes wrong:** Prüfung nach dem RPC oder innerhalb von `ApplyConfigurationWithMode`: Erwartung (`Expected`) wird gesetzt, und eine fremde Änderung wird später als „eigenes Apply" angenommen (Phase-15-Regel 4).
**How to avoid:** Prüfung strikt davor (T5, F4).

### Pitfall 5: Satz aus dem Ein-Dokument-`Compare`
Siehe Ledger 176. Eine Änderung an `KubeletConfig` ergibt `changes=0`, also einen Satz ohne Inhalt, der D-02 verletzt. **How to avoid:** `CompareDocuments`; T7/F8.

### Pitfall 6: Werte im Satz
`Problem.Detail` geht über die Leitung; Redaktion fängt Geheimnisse, nicht Adressen. Nur Pfade und Art. T7/F9.

### Pitfall 7: Der Plan-Speicher wird zur Wahrheit
**What goes wrong:** Jemand lässt die Verweigerung davon abhängen, ob der Plan-Text noch da ist (nach Neustart: kein Digest-Vergleich, „kein Plan bekannt → weiter").
**How to avoid:** Die Sicherheit liegt am Digest; fehlt der Text, wird dennoch verweigert (T7, Zweig „Speicher leer").

### Pitfall 8: Bestandstests rot durch Signaturwechsel
Vier Aufrufer in `internal/configdrift` (siehe Frage 5) und das Budget-Dokument. Früh umstellen, bevor die eigentliche Arbeit beginnt (Wave 0).

### Pitfall 9: Budget-Tabelle widerspricht dem Code
`cmd/holzkube-managerd/budget_test.go` beschreibt die Apply-Route mit vier Upstream-Aufrufen, darunter „apply: NewClusterClient Version", mit dem Text, der Client des Lesens werde vor dem Merge geschlossen. Mit **einem** Client für Lesen und Senden entfällt der zweite Probe-Aufruf. Die Zeile ist deklarativ (kein Abgleich mit dem Code), aber falsch zu lassen wäre eine Lüge im Wächter; sie ist zu ändern, wenn der Client geteilt wird. [VERIFIED: cmd/holzkube-managerd/budget_test.go:522-545]

## Code Examples

### Plan trägt den Digest (Entwurf `[ASSUMED]`)
```go
// Preview bekommt:
//   // BaseDigest is the digest of the snapshot of the configuration this plan
//   // was made from. Apply must be sent it back.
//   BaseDigest string `json:"base_digest"`
//
// In Plan, nach read(): snap, err := Snapshot(base); p.BaseDigest = snap.Digest
// und s.remember(id, snap) für den Satz. Die Rohbytes (base) gehen weiter nur
// in ApplyPatches/Compare/Redact, nie in den Speicher.
```

### Verweigerung im Handler (Muster aus config.go:380-383, Entwurf)
```go
case errors.Is(err, machineconfig.ErrChangedSincePlan):
	httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeConfigChangedSincePlan, err.Error()))
```

### Pflichtfeld und Form im Apply (Muster aus config.go:180-186 und configdrift.go:70)
```go
if !driftDigestShape.MatchString(body.BaseDigest) {
	httpapi.WriteProblem(w, r, httpapi.Validation(
		"Send the base_digest the plan gave you: 64 hexadecimal characters. Plan first, then apply.",
		httpapi.FieldError{Field: "base_digest", Reason: "must be 64 lower-case hexadecimal characters"}))
	return
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Apply patcht, was der Knoten gerade hat | Apply patcht denselben Stand, gegen den der Betreiber geplant hat, sonst Verweigerung | Phase 17 | Zwei Operatoren überschreiben einander nicht mehr still |
| Ein-Dokument-`Compare` für alles | `CompareDocuments` für den Satz | Phase 15 (gebaut), Phase 17 (benutzt) | Änderungen in `KubeletConfig` u. a. werden benannt |

**Deprecated/outdated:** nichts in dieser Phase.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Die Requirement-IDs VERS-01..06 sind frei gewählt, nicht vom Betreiber bestätigt | phase_requirements | Namensänderung; kein Verhalten |
| A2 | „keine Adressen" im Auftrag meint den Satz der Verweigerung (Werte weglassen), nicht nur das Repository | Frage 4 | Betreiber will Werte im Satz: eine Zeile, aber der Wächter T7/F9 ändert sich |
| A3 | `MachineConfigYAML` liefert auf einem echten Knoten bei zwei aufeinanderfolgenden Lesungen ohne Änderung denselben Snapshot-Digest (die Normalisierung deckt Kommentare und Reihenfolge ab) | Frage 2/3 | Falsche Verweigerung auf echter Hardware; im Simulator nicht prüfbar. Als Pi-Prüfung in die Verifikation (`human_needed`) |
| A4 | Das Rest-Fenster (Lesen bis RPC mit einem Client) ist in der Praxis klein (Millisekunden) | Frage 3 | Fremdänderung im Fenster bleibt unbemerkt; nicht messbar auf dem Mac |
| A5 | Plan-Speicher: 4 Einträge je Knoten, Lebensdauer ~1 h, Gesamtgrenze sind Vorschläge, nicht aus dem Repository | Frage 4 | Zahlen sind Planerentscheidung; wirken nur auf Wortlaut |
| A6 | Die Verweigerung eines zweiten Klicks auf dieselbe Vorschau nach erfolgreichem Apply ist erwünscht | Frage 3 | Betreiber empfindet es als Störung; es ist die Folge von D-01 und schützt vor doppelten Listeneinträgen |
| A7 | Vorschlag `base_digest` im Audit zu erlauben (Hash, kein Geheimnis) | T13 | Wenn nicht: Feld wird still verworfen, nichts bricht |

## Open Questions

1. **Echter Knoten:** Liefert das Re-Encoding eines echten Talos-Knotens bei jedem Lesen denselben redigierten Snapshot? (A3.) Nicht auf dem Mac prüfbar; eine Pi-Prüfung gehört in `human_needed` der Verifikation: Plan, kurz warten, Apply ohne Änderung muss durchgehen.
2. **Fehlermapping der Snapshot-Fehler im Plan** (`ErrSnapshotUnreadable`, `TooLarge`, `Unsafe`) heute `500`. Planer entscheidet: belassen oder auf `409`/Upstream mappen. Keine Auswirkung auf VERS-01..06.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Tests, Linter | ✓ | go1.27.2 (lokal); CI-Linter braucht `GOTOOLCHAIN=go1.26.9` | — |
| Node | Web-Tests | ✓ | v26.11.1; `web/node_modules` vorhanden | — |
| talossim | alle Knotenbeweise | ✓ | im Repository | — |
| Browser / echter Knoten / `go test -race` | Layout, A3, Rennen | ✗ | — | Pi-Prüfung durch den Betreiber (`human_needed`); Race-Detektor nur in CI |

**Missing dependencies with no fallback:** keine für den Bau. **Missing with fallback:** echter Knoten und Browser, siehe Open Question 1.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` (talossim, echter Talos-Client) und Vitest (jsdom) |
| Config file | `web/vitest.config.*` vorhanden; Go ohne Konfiguration |
| Quick run command | `go test ./internal/machineconfig/... ./internal/configdrift/... ./internal/httpapi/...` |
| Full suite command | `./bin/task ci`; Linter lokal: `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...`; Web: `NODE_OPTIONS=--no-experimental-webstorage npx vitest run --project jsdom` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| VERS-01 | `base_digest` im Plan, 64 Hex, gleich `Snapshot(raw).Digest` | Integration (talossim) | `go test ./internal/machineconfig -run TestThePlanCarriesTheDigestOfTheConfigurationItRead` | ❌ Wave 0 |
| VERS-02 | Verweigerung, nichts gesendet; Pflichtfeld; kein Schalter | Integration + HTTP | `go test ./internal/machineconfig -run 'TestAnApplyIsRefused\|TestAnApplyWithoutADigest'` und `go test ./internal/httpapi -run TestConfig` | ❌ Wave 0 |
| VERS-03 | Satz: Pfade aller Dokumente, keine Werte/Geheimnisse | Unit + Integration | `go test ./internal/machineconfig -run TestTheRefusalSentence` | ❌ Wave 0 |
| VERS-04 | Eigenes Apply frischt Basis auf; Verweigerung hinterlässt nichts | Integration | `go test ./internal/configdrift -run 'TestARefusedApply\|Apply'` | teils ✅ (anzupassen), teils ❌ |
| VERS-05 | Sperre je Knoten | Integration (zeitgestützt) | `go test ./internal/machineconfig -run TestTwoAppliesOnOneNode -count=20` | ❌ Wave 0 |
| VERS-06 | UI: Digest durchgereicht, Satz, Sperre, Neu-Planen; Doku | Vitest + Go-Vertragswächter | `NODE_OPTIONS=--no-experimental-webstorage npx vitest run --project jsdom src/routes/config.test.tsx` und `go test ./internal/httpapi -run TestEveryProblemCode` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** Quick run des jeweiligen Pakets
- **Per wave merge:** `go test ./...` plus Vitest jsdom
- **Phase gate:** `./bin/task ci` grün (auf dem Mac teils nicht lauffähig: dann benennen, was gelaufen ist), Linter lokal, vor `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `internal/machineconfig/changedsinceplan_test.go` — T1 bis T4, T6, T7, T8 mit dem Muster aus `snapshot_node_test.go`
- [ ] Hilfsfunktion „Plan, dann Apply" und Umstellung der vier Aufrufer in `internal/configdrift` (`apply_test.go:63`, `callers_test.go:42`, `expected_test.go:545`, `:694`)
- [ ] `internal/configdrift/…` T5 (Beobachter + Verweigerung, Baseline prüfen)
- [ ] `internal/httpapi/configapi_test.go` — T9
- [ ] `web/src/routes/config.test.tsx` — T11 (bislang gibt es keinen Test für `config.tsx`; vorhandene Tests: `api.test.ts`, `NodeConfigDrift.test.tsx`, `drift-wiring.test.tsx`)
- [ ] Budget-Zeile in `cmd/holzkube-managerd/budget_test.go` anpassen, falls ein Client geteilt wird

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | nein (unverändert) | Sitzung, Sudo-Fenster (Route bleibt `Destructive`) |
| V3 Session Management | nein | — |
| V4 Access Control | ja, unverändert | `MinRole: RoleOperator`, Cluster-Sperre vor dem Handler; **Wächter**: der Digest ersetzt keine Berechtigung |
| V5 Input Validation | ja | `base_digest` Pflicht und 64 Hex (`driftDigestShape`), `DisallowUnknownFields` |
| V6 Cryptography | nein | SHA-256 aus `Snapshot` (Standardbibliothek), kein eigener Primitiv |
| V7 Logging / Error Handling | ja | Detail ohne Geheimnisse und Werte; Audit speichert nur den Code |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Informationsabfluss über den Verweigerungssatz | Information Disclosure | nur Pfade und Art aus `CompareDocuments` (beide Seiten über `Snapshot`), keine Werte; T7 |
| Gefälschter oder wiederverwendeter Digest | Tampering | kein Sicherheitsmerkmal: Vorbedingung; wer ihn fälscht, kann ebenso neu planen; Rolle und Sudo bleiben das Tor |
| Hash unredigierter Konfiguration als Orakel | Information Disclosure | vermieden: Digest nur über den redigierten Snapshot (Wahl A) |
| Hängender Request durch wartende Sperre | Denial of Service | `ctx`-fähige Sperre, 30-s-Budget der Route |
| Unbegrenzter Plan-Speicher | Denial of Service | Grenzen je Knoten und gesamt, nur redigierter Text, `MaxSnapshotBytes` je Eintrag |
| Fremdschreiber im Rest-Fenster | Tampering | nicht schließbar (kein Vorbedingungsfeld im RPC); dokumentiert |

## Auswahl für den Betreiber

Nur dort, wo echte Alternativen mit unterschiedlichen Kosten bestehen. Alles andere in diesem Dokument ist von Claude entschieden (Ermessen aus CONTEXT.md).

**Auswahl 1: Worüber läuft der Digest?**
1. **Redigierter Snapshot (empfohlen).** Kostet: eine reine Geheimnisänderung zwischen Plan und Apply wird nicht verweigert (Ledger 178, gleiche Entscheidung wie Phase 15). Bringt: Satz hat immer Inhalt, keine Hash-Ausgabe unredigierter Bytes, keine falschen Verweigerungen durch Re-Encoding; das Apply trägt Fremdgeheimnisse ohnehin mit.
2. **Snapshot plus Digest je Geheimnisfeld.** Kostet: Eingriff in `Redact`/`Snapshot`, entscheidet Ledger 178 nebenbei, deutlich mehr als „klein". Bringt: auch Geheimnisänderungen werden benannt.
3. **Hash der Rohbytes.** Kostet: erste Hash-Ausgabe unredigierter Konfiguration; Verweigerung ohne nennbare Änderung möglich. Bringt: fängt jede Änderung. Nicht empfohlen.

**Auswahl 2: Woher kommt der Vorher-Text für den Satz?**
1. **Begrenzter Speicher im Prozess (empfohlen).** Kostet: nach Neustart des Daemons (stündliches Update) oder Verdrängung nennt der Satz die Pfade nicht („der frühere Stand liegt nicht mehr vor"), die Verweigerung bleibt. Bringt: das API bleibt ein Feld `base_digest`, wie beim Drift-Bestätigen.
2. **Redigierten Snapshot im Apply-Body zurückschicken.** Kostet: einige KB (erzeugte Control-Plane-Konfiguration 6,7 KB, laut Kommentar in `snapshot.go`) mehr je Anfrage, API-Form schwerer für Skripte, Text vom Client fälschbar (nur Wortlaut betroffen). Bringt: Satz immer vollständig, auch nach Neustart, kein Zustand im Daemon.

**Auswahl 3: Ledger 176 (Plan sieht nur das erste Dokument) in dieser Phase schließen?**
1. **Nein, offen lassen (empfohlen).** Kostet: eine Vorschau für einen Patch auf ein anderes Dokument zeigt weiter einen leeren Diff; der Digest schützt trotzdem. Bringt: Phase bleibt „klein"; D-09 aus Phase 15 gilt weiter.
2. **Ja, `Compare`/`Plan` mehrdokumentfähig und `ModeFor` mit Dokumentpräfix.** Kostet: Eingriff in die Modus-Whitelist (unbekannt = Neustart), mehr Tests, anderes Risiko als diese Phase. Bringt: schließt 176, Vorschau und Satz benutzen dieselbe Sicht.
3. **Nur die Anzeige der Vorschau auf `CompareDocuments` umstellen, `ModeFor` unverändert.** Kostet: Vorschau zeigt Pfade, für die `ModeFor` keine Pfade bekommt; irreführend. Nicht empfohlen.

## Sources

### Primary (HIGH confidence)
- Im Repository gelesen (diese Sitzung): `internal/machineconfig/service.go`, `snapshot.go`, `redact.go`, `diff.go`; `internal/httpapi/handlers/config.go`, `configdrift.go`, `inventory.go:240-292`; `internal/httpapi/problem.go`; `internal/httpapi/contract_codes_test.go`; `internal/talos/client.go:1033-1099`, `applyobserver.go`; `internal/configdrift/apply.go`, `decide.go:1-120`, `callers_guard_test.go`; `internal/talossim/machine.go:841-886`, `cosi.go:93-145`, `scenario_rpc.go`; `internal/audit/redact.go:255-290`; `internal/httpapi/middleware/audit.go`, `clusterlock.go`; `web/src/routes/config.tsx`, `web/src/api.ts:3130-3175, 3940-3990`, `web/src/lib/problem.ts`; `cmd/holzkube-managerd/budget_test.go:505-545`; `docs/api-contract.md:4220-4300, 5040-5060`; `docs/guide.md:1440-1460`; `internal/upgrade/job.go:455-500`, `internal/rotateca/job.go:405-495`; `.planning/phases/17-…/17-CONTEXT.md`, `.planning/AUDIT-2026-10-08.md`, `.planning/WINDOWS.md` (Ledger 176 und 178), `CLAUDE.md`.
- Talos Machinery v1.14.2 im Modulcache: `api/machine/machine.pb.go:843-851` (`ApplyConfigurationRequest` ohne Vorbedingung).

### Secondary (MEDIUM confidence)
- Keine Websuche nötig; die Phase berührt keine externe Bibliothek außer der bereits gepinnten Machinery.

### Tertiary (LOW confidence)
- Verhalten eines echten Knotens (A3, A4): nicht geprüft.

## Project Constraints (from CLAUDE.md)

- **Öffentliches Repository:** in Tests und Fixtures nur Dokumentationswerte (`192.168.1.10`, `homeserver`, `example.com`); keine Werte der echten Installation; neue echte Werte sofort als Hash in `internal/publicrepo`. Nicht gegen eine echte Konfiguration testen und deren Werte einchecken.
- **README im selben Schritt:** Eintrag in „What it does", Tiefe in `docs/guide.md`; Screenshots nur gerendert aus `web/fixtures/demo.json` (`task build && node web/scripts/readme-images.mjs`).
- **Changelog nur beim Release:** `internal/changelog/changelog.json` nicht anfassen.
- **Alpha-Hinweis** bleibt; `holzkube-managerd --version` bleibt eine nackte Version.
- **Jeder Wächter rot gesehen:** Einspritzungen F1 bis F11 sind Abnahmebedingungen; ein Test, der nach einer Einspritzung grün bleibt, die nichts einspritzte, ist keine Messung.
- **Fragen als Auswahl** (`AskUserQuestion`, zwei bis vier Optionen, Empfehlung zuerst): im Abschnitt „Auswahl für den Betreiber" so vorbereitet.
- **Ziel ist `linux/arm64`** (Raspberry Pi 5); der Mac beweist Logik, nicht die Hardware; was nur der Pi prüfen kann (A3), gehört in `human_needed`.
- **Talos-Pin:** unverändert (Machinery v1.14.2); keine neue Abhängigkeit.
- **Linter lokal:** `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...`.
- **Wächter vom Kontext ohne Frist aus:** Der Talos-Client verweigert Aufrufe ohne Frist (Quick-Task 261014-gl2, Ledger 137). Für diese Phase relevant, weil `Service.apply` über `s.read(ctx, id)` ohne eigene `WithClassDeadline` liest und sich auf `budgetedContext` (30 s) der Route verlässt. Tests, die `Service.Apply` direkt aufrufen, müssen ihrerseits ein `context.WithTimeout` mitgeben (Muster `thirdPartyChange`), sonst verweigert der Client; der Planer entscheidet, ob `apply` die Klassenfrist selbst setzt wie `ReadSnapshot`.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — keine neuen Pakete, alles im Repository gelesen
- Architecture: HIGH — Ablauf, Beobachterreihenfolge und Wächter im Code gelesen; MEDIUM nur für Rest-Fenster und echtes Re-Encoding
- Pitfalls: HIGH — aus gelesenem Code und den Ledger-Einträgen 176 und 178 abgeleitet

**Research date:** 2026-10-16
**Valid until:** 2026-11-15 (stabiler Code; früher ungültig, wenn die Machinery-Pin angehoben wird oder `service.go` umgebaut wird)
