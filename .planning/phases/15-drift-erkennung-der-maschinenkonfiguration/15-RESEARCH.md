# Phase 15: Drift-Erkennung der Maschinenkonfiguration - Research

**Researched:** 2026-10-14
**Domain:** Go-Daemon (Talos-Maschinenkonfiguration lesen, redigiert speichern, vergleichen), fsstore-Persistenz, httpapi-Route mit Sudo/Audit, React-Anzeige, Prometheus-Serien
**Confidence:** HIGH für alles, was im Code gelesen oder am Simulator gemessen wurde; MEDIUM für das Verhalten echter Talos-Knoten (nicht messbar von hier, siehe Assumptions Log)
**Umgebung dieser Session:** macOS, arm64 (Mac des Entwicklers), Go 1.27.2 lokal (go.mod verlangt `toolchain go1.26.9`), kein Pi, kein Cluster, kein `task`. Alles "gemessen" unten heißt: gegen `internal/talossim` und die Machinery-Bibliothek v1.14.2 auf diesem Mac. Die Messprogramme waren temporäre `_test.go`-Dateien und sind wieder gelöscht (`git status` sauber).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

Alle als Auswahl vom Betreiber am 2026-10-14 getroffen.

#### Vergleichsbasis
- **D-01:** Basis ist der **letzte bekannte Stand**: die redigierte aktive Konfiguration, die holzkube-manager nach dem ersten Sehen eines Knotens und nach jedem eigenen Apply festhält. Drift heißt: der Knoten hat sich seitdem ohne uns verändert (talosctl, anderer Operator). Ein Knoten ohne Basis hat keine Abweichung; seine erste Lesung legt die Basis an und sagt das. — **Reversibility:** costly — die Basis ist persistierter Zustand im Datenverzeichnis, ein Format und ein Aktualisierungsweg, die später nur mit Migration zu ändern sind.
- **D-02:** Gespeichert wird ein **redigierter Schnappschuss** (Geheimnisse unkenntlich über `machineconfig.Redact`), kein Hash und keine vollständige Konfiguration. Damit zeigt die Seite, **was** sich geändert hat (Diff wie `machineconfig.Compare`). Eine Änderung nur eines Geheimniswerts bleibt unsichtbar; das ist zu benennen. Es kommt nie ein unredigierter Schlüssel in das Verzeichnis. — **Reversibility:** costly — Schnappschussformat im Datenverzeichnis.

#### Rauschen
- **D-03:** Felder, die sich ohne Zutun ändern könnten, stehen in einer **kleinen festen Ausnahmeliste im Code, per Recherche belegt** (die Recherche misst, welche Pfade das sind). Alles andere zählt. Kein Bedienelement, kein gespeicherter Zustand beim Betreiber. Eine vergessene Ausnahme zeigt sich als Fehlalarm, nicht als übersehene Drift.

#### Lesen
- **D-04:** Das Lesen ist **periodisch nach dem Muster `internal/certwatch`**: alle paar Stunden jeder erreichbare Knoten, dazu nach jedem eigenen Apply. Kein COSI-Watch. Ein Knoten, der nicht antwortet, behält seine letzte Lesung und wird als nicht gelesen gezählt; ein nie gelesener Knoten sagt „not read yet", nie „keine Drift".

#### Anzeige
- **D-05:** Drift erscheint auf der **Knotenseite mit Diff**, als **Zeile auf der Clusterkarte** (wie „Cluster certificates"), als **Metrik in `/metrics`** (abgewichen 0/1 je Cluster und Knoten) und als **Banner auf jeder Seite**.
- **D-06:** Das Banner erscheint **erst, wenn Drift 24 Stunden unbestätigt bleibt**. Karte, Knotenseite und Metrik zeigen sie sofort. Der Betreiber hat das Banner trotz des Hinweises auf Dauerrauschen gewählt; die 24-Stunden-Schwelle ist die vereinbarte Abschwächung.

#### Auflösung
- **D-07:** Der Betreiber kann das Diff ansehen und **„so ist es gewollt" bestätigen**: der aktuelle Stand wird die neue Basis. Aktion mit Admin-Rolle, Sudo-Fenster falls die Projektregeln es für Aktionen dieser Art verlangen, und Audit-Eintrag; kein Eingriff am Knoten. **Kein Zurücksetzen** in dieser Phase.

### Claude's Discretion
- Name und Ort des Pakets (nach dem Vorbild `certwatch`), Lesetakt in Stunden, Form der Persistenz (Datei je Knoten über `fsstore`), Wortlaut der Sätze. Alle Gestaltungsfragen, die ein Betreiber nicht entscheiden muss; wo doch eine echte Alternative besteht, geht sie als Auswahl zurück an den Betreiber.

### Deferred Ideas (OUT OF SCOPE)
- Zurücksetzen auf die Basis (Schreibaktion auf den Cluster, mit Sudo, Bestätigung und Reboot-Frage) — eigene Phase.
- Vergleich gegen Schwesterknoten oder gegen eine aus Geheimnissen und Patches erzeugte Soll-Konfiguration.
- Vom Betreiber pflegbare Ausnahmen für Pfade.
- Wand-Kachel für Drift.
</user_constraints>

<phase_requirements>
## Phase Requirements

Es wurden keine Requirement-IDs übergeben. Vorschlag für den Planner (Format `^[A-Z0-9]+-\d+$`, wie `internal/planning` es verlangt); die Zuordnung zur Forschung steht in der rechten Spalte.

| ID (Vorschlag) | Beschreibung | Research Support |
|----|-------------|------------------|
| DRIFT-01 | Erste Lesung eines Knotens legt die Basis an und sagt das; ein Knoten ohne Basis hat keine Abweichung | Befund 3, Muster 2 (Zustandsmaschine) |
| DRIFT-02 | Basis = redigierter, normalisierter Schnappschuss je Knoten in `config-baselines/`, nie unredigiert, beschädigte Datei wird gemeldet statt als "keine Drift" gelesen | Befund 2, 3; Muster 1, 2 |
| DRIFT-03 | Periodische Lesung (Muster certwatch) mit eigenem Deadline-Budget; nicht erreichbarer Knoten behält Lesung und zählt als nicht gelesen; nie gelesen = "not read yet" | Befund 5, Pitfall 1 |
| DRIFT-04 | Eigene Applies (Service.Apply, Kubernetes-Upgrade, CA-Rotation) verschieben die Basis, staged/try/reboot korrekt | Befund 4, Muster 3 |
| DRIFT-05 | Knotenseite zeigt Zustand und Diff über alle Dokumente der Konfiguration (nicht nur v1alpha1) | Befund 1, Muster 4 |
| DRIFT-06 | Clusterkarte: Zeile "Configuration" in `ClusterView`; `/metrics`: 0/1 je Cluster und Knoten, nie 0 für nicht gelesen | Befund 8 |
| DRIFT-07 | Banner nach 24 h unbestätigter Drift; "seit wann" überlebt einen Neustart | Befund 7 |
| DRIFT-08 | "So ist es gewollt": Admin, Sudo-Fenster, Audit mit Knoten und Digest, neuer Stand wird Basis, nichts am Knoten | Befund 6 |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Öffentliches Repository:** keine LAN-Adresse, kein Hostname, keine Mail, MAC oder Home-Verzeichnis des echten Aufbaus in Tests, Fixtures, Ledger, Commit-Messages. Dokumentationswerte nutzen (`192.168.1.10`, `homeserver`, `example.com`). `internal/publicrepo` prüft Baum und Historie. Nicht gegen die echte Konfiguration eines Knotens testen und deren Werte einchecken.
- **README im selben Schritt wie das Feature:** Eintrag in "What it does", Tiefe in `docs/guide.md`; Screenshot nur gerendert aus `web/fixtures/demo.json` (`task build && node web/scripts/readme-images.mjs`), nie vom echten Cluster.
- **Changelog nur beim Release:** `internal/changelog/changelog.json` gehört zum Release, nicht zur Phase. Die Phase fasst ihn nicht an.
- **Alpha-Hinweis** bleibt; `holzkube-managerd --version` bleibt eine nackte Version.
- **Produktionsziel ist linux/arm64 (Raspberry Pi 5).** Hier kein `go test -race` (nur CI), kein Pi. Ein Bericht muss sagen, wo er lief.
- **Ersetzen der Produktions-Binary, Neustart des Dienstes, Kopieren des Datenverzeichnisses sind Sache des Betreibers.**
- **Machinery-Pin:** `internal/talos/machineryversion_test.go` und `talos-upstream.yml` halten den Pin; diese Phase hebt ihn nicht.
- **Entscheidungen an den Betreiber immer als Auswahl** (2–4 benannte Optionen mit Kosten/Nutzen, Empfehlung zuerst). Die offenen Auswahlen stehen gesammelt in "Open Questions".
- **Jeder Wächter muss gegen den wieder eingesetzten Fehler rot gesehen werden;** eine Injektion, die nichts injiziert, ist kein Ergebnis. Die Tabelle "Injizierbare Fehler" unten ist dafür gedacht.

## Summary

Die Funktion ist im Kern klein: lesen (`Service.read` gibt es), redigieren (`Redact` gibt es), speichern (ein neuer fsstore-Entity), vergleichen, anzeigen. Die Recherche hat aber **sechs Dinge gefunden, die der Plan nicht übersehen darf**, drei davon durch Messung am Simulator:

1. **`machineconfig.Compare` sieht nur das erste YAML-Dokument.** Seit Talos v1.14 liegt der größte Teil einer Konfiguration in Dokumenten neben `v1alpha1` (die generierte Control-Plane-Konfiguration hat 28). Gemessen: eine Änderung an `KubeletConfig.image` ergibt `changes=0`, eine Änderung an `.machine.certSANs` (erstes Dokument) ergibt `changes=1`. Mit `Compare` als Drift-Vergleich wäre jede Änderung außerhalb von `v1alpha1` unsichtbar, also genau das stille "keine Drift", das die Phase ausschließen soll. Es braucht einen Mehrdokument-Vergleich.
2. **Redact ersetzt Geheimniswerte durch eine Konstante** und lässt leere Felder leer. Eine reine Geheimnisänderung ist unsichtbar (gemessen: Token, Cluster-Secret und Bootstrap-Token ersetzt, `changes=0`, redigierte Bytes identisch). Sichtbar bleibt: gesetzt ↔ leer. Nicht redigiert werden u. a. `machine.files[].content`, `machine.env`, `cluster.inlineManifests[].contents` und `EnvironmentConfig`/`EtcFileConfig`/`ExtensionServiceConfig`; sie landen im Schnappschuss genauso wie sie heute schon in der Config-Ansicht stehen.
3. **Kein Rauschpfad belegbar.** Talos schreibt die aktive Konfiguration nach Dokumentation und Code dieses Repos nicht selbst um; am Simulator ist Lesen, Wiederholen und Re-Applizieren identischer Bytes stabil. Die D-03-Ausnahmeliste ist nach heutigem Wissen **leer**. Was wie Rauschen aussieht (Kommentare, Dokumentreihenfolge, Machinery-Version) ist Normalisierung, keine Ausnahme.
4. **Eigene Applies laufen über genau einen Engpass:** `ClusterClient.ApplyConfigurationWithMode`. Drei Aufrufer (`machineconfig.Service.Apply`, `upgrade.applyImage`, `rotateca.Deps.rewrite`) plus Provisionierung über `MaintenanceClient`. `staged` und `try` ändern die aktive Konfiguration nicht dauerhaft bzw. nicht sofort; `auto`/`reboot` ändern sie erst nach dem Neustart. Ein "Basis neu lesen nach Apply" allein ist dafür falsch.
5. **`certwatch` hat sehr wahrscheinlich den Fehler aus Ledger 137.** Der Lauf bekommt `context.WithCancel(context.Background())`, jeder Talos-Aufruf ohne Deadline wird mit `ErrNoDeadline` abgelehnt. Gemessen am Simulator: `read node certificates: talos: Get on m1: talos: refusing a call with no deadline`. Der neue Leser muss sein Budget selbst setzen, und für certwatch ist eine Auswahl an den Betreiber fällig.
6. **Das Audit-Record hat für HTTP-Routen nie ein `machine_id`.** `auditAdapter.Attempt` setzt weder `MachineID` noch `ClusterID`; der Knoten steht nur in der URL, die nicht gespeichert wird. Ein "Drift bestätigt"-Eintrag ohne Knoten wäre wertlos; es braucht einen vom Server geschriebenen Parameter nach dem Vorbild von `AccountParam`.

**Primary recommendation:** neues Paket `internal/configdrift` (Muster certwatch, aber mit eigenem Deadline-Budget je Knoten), ein neuer fsstore-Entity `config-baselines` (Schlüssel `MachineID`, `recordStore` mit Rev-CAS), Schnappschuss = **normalisierter** redigierter Text (Dokumente nach `kind[/name]` sortiert, Kommentare weg), Vergleich über eine **neue** Mehrdokument-Funktion neben `Compare`, eigene Applies über einen Haken am `ClusterClient` mit **erwartetem Digest** statt "danach neu lesen", `DriftSince` im Basis-Datensatz für das 24-h-Banner, Bestätigung als `config.drift-confirm` (Admin, `Destructive`, Digest-CAS, Knoten als serverseitiger Audit-Parameter).

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Aktive Konfiguration lesen | Node (COSI `MachineConfigs.config.talos.dev/v1alpha1`) über `internal/talos` | — | Nur der Knoten weiß, was aktiv ist; Zugriff nur über `inv.Connect` |
| Redigieren und Normalisieren | Domain (`internal/machineconfig`) | — | Geheimnisse verlassen `internal/talos`/`machineconfig` nur redigiert (CLAUDE.md, CONTEXT) |
| Basis speichern | Store (`internal/store/fsstore`) | — | Pfade nur in fsstore (`TestNoDirectFileAccessOutsideFsstore`) |
| Periodisches Lesen, Zustand, 24-h-Uhr | Domain (`internal/configdrift`) | Store (Persistenz von `DriftSince`) | Muster certwatch; Zustand zum Lesezeitpunkt im Speicher, Basis und Drift-Beginn persistiert |
| Haken bei eigenen Applies | Transport (`internal/talos.ClusterClient`) | Inventory (`Connect` installiert den Haken) | Einziger Engpass aller Schreibpfade auf verwaltete Knoten |
| Cluster-Zusammenfassung | Domain → `inventory.ClusterView` | Metrics | Wie `SetCertificateHealth`: Setter am Inventory, Funktion vom Paket |
| Diff und Bestätigen | API/Backend (`internal/httpapi/handlers`) | Domain | Route-Deklaration trägt Rolle, Sudo, Audit-Action |
| Anzeige (Knotenseite, Karte, Banner) | Browser (React) | — | Reine Darstellung; die 24-h-Schwelle und die Sätze kommen vom Server |
| `/metrics` | API (`internal/metrics`) | Domain | Serien aus `ClusterView`, kein Knotenzugriff beim Scrape |
| Wand | — | — | Ausdrücklich nicht Teil der Phase; die Wand projiziert explizit (`wallCertificatesFrom`), ein neues `ClusterView`-Feld erreicht sie nicht |

## Standard Stack

### Core

Keine neuen externen Pakete. Alles, was gebraucht wird, ist im Repo oder schon in `go.mod`/`web/package.json`.

| Baustein | Fundort | Zweck |
|---------|---------|-------|
| `github.com/siderolabs/talos/pkg/machinery` v1.14.2 | `go.mod`, `depguard_test.go` pinnt `machineryVersion = "v1.14.2"` | Laden, Redigieren (`RedactSecrets`), Encodieren |
| `gopkg.in/yaml.v3` | schon importiert in `internal/machineconfig/diff.go` | Mehrdokument-Zerlegung für Normalisierung und Vergleich |
| `internal/store/fsstore` (`recordStore[K,T]`) | `internal/store/fsstore/inventory.go` | Atomare, per-Entity gesperrte, Rev-geprüfte Datensätze |
| `internal/certwatch` als Gerüst | `internal/certwatch/certwatch.go` | Deps-Funktionen, `Run`/`Tick`/`Refresh`, letzte Lesung behalten |
| `@tanstack/react-query`, shadcn-`Card`/`Badge`/`Button`, `zod` | `web/src/...` | Wie `NodeCertificates.tsx`, `Backups.tsx`, `api.ts` |

### Alternatives Considered
| Statt | Alternative | Abwägung |
|-------|-------------|----------|
| Neuer fsstore-Entity | `fsstore.WriteFileAtomic` mit eigener Datei (so macht es `internal/history`) | Verliert Rev-CAS, per-Entity-Sperre, `safeKey`, Löschen mit fsync. `history` rechtfertigt es mit "eine Datei, einmal pro Minute"; hier sind es N Datensätze mit konkurrierenden Schreibern. Nicht empfohlen. |
| Mehrdokument-Vergleich als neue Funktion | `Compare` selbst reparieren | Ändert `Plan`, `ModeFor` und dessen Tests (Pfade bekämen ein Dokument-Präfix); siehe Open Question 2 |

**Installation:** keine. **Version verification:** keine neuen Pakete. `go.mod` Zeile `github.com/siderolabs/talos/pkg/machinery v1.14.2` gelesen.

## Package Legitimacy Audit

Diese Phase installiert keine externen Pakete. Es gibt nichts zu prüfen; `gsd-tools query package-legitimacy check` wurde nicht aufgerufen, weil es keine Paketnamen gibt.

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
                 eigene Applies                                   periodisch (3 h) + 1 min nach Start
 config.apply ─┐                                                              │
 k8s-Upgrade  ─┼─> inv.Connect ─> ClusterClient.ApplyConfigurationWithMode    ▼
 CA-Rotation  ─┘        │              │ vorher: Haken schreibt "erwartet"   configdrift.Service.Tick
                        │              │ nachher: Haken (Modus, Fehler)        │ je Cluster, je Knoten (seriell,
                        ▼              ▼                                       │ eigenes Budget ~20 s)
                  Talos-Knoten  <── COSI Get MachineConfigs/v1alpha1 ──────────┤
                                                                               ▼
                                         Redact ──> Normalize (Dokumente sortiert, Kommentare weg)
                                                         │ digest = sha256(snapshot)
                                                         ▼
                              ┌── Basis fehlt ─────────> Basis anlegen (first-sight), Zustand "baseline-created"
   fsstore                    │
   config-baselines/<id>.json ├── digest == basis.digest ──> ok   (DriftSince löschen, falls gesetzt)
   {snapshot,digest,rev,      ├── digest == basis.erwartet ─> Basis übernehmen (eigener Apply gelandet)
    drift_since,erwartet}     ├── erwartet gesetzt, jung ───> "settling" (weder ok noch Drift)
                              └── sonst ──────────────────> Drift (DriftSince einmalig setzen)
                                                         │
                                  letzte Lesung im Speicher (Zustand, Zeit, Snapshot, Fehlersatz)
                                                         │
        ┌────────────────────────────┬───────────────────┴───────────┬─────────────────────────┐
        ▼                            ▼                               ▼                         ▼
 ClusterView.ConfigDrift      GET /machines/{id}/config/drift   /metrics (aus ClusterView)   Banner (>= 24 h seit
 (Karte, Zeile)               (Diff Basis↔Lesung, Operator+)    nur Knoten mit Lesung        DriftSince), jede Seite
                                           │
                       POST .../config/drift/confirm  {digest}  Admin + Sudo + Audit
                       (Digest muss zur aktuellen Lesung passen; Basis := Lesung; nichts am Knoten)
```

### Recommended Project Structure
```
internal/configdrift/        # Service (Deps, Run/Tick/Refresh, HealthFor, Confirm, Detail), Zustände, Sätze
internal/model/configdrift.go    # ConfigBaseline (persistiert), ConfigDriftHealth/NodeConfigDrift (Sicht)
internal/machineconfig/snapshot.go  # Normalize, SnapshotDigest, CompareDocuments (neu, neben diff.go)
internal/store/store.go + fsstore/  # ConfigBaselines() Entity "config-baselines"
internal/talos/client.go     # ApplyObserver am ClusterClient (Haken)
internal/inventory/          # SetApplyObserver(...), SetConfigDrift(...) (Setter-Muster wie SetCertificateHealth)
internal/httpapi/handlers/configdrift.go   # GET + POST confirm, Routen-Deklaration
internal/audit/redact.go     # "config.drift-confirm" + serverseitiger Knoten-Parameter
internal/metrics/metrics.go  # Serien aus ClusterView
web/src/components/NodeConfigDrift.tsx, ConfigDriftRow.tsx, DriftBanner (in CertificateBanner-Nähe), api.ts
```

### Befunde zu den neun Fragen

**Befund 1 (Frage 2): Was `read`/`Redact`/`Compare` liefern.**
- `MachineConfigYAML` liest die Ressource mit der ID `v1alpha1` (aktive Konfiguration): `safe.StateGetByID[*configres.MachineConfig](ctx, c.COSI(), configres.ActiveID)` [VERIFIED: internal/talos/facts.go:467] und `const ActiveID = resource.ID("v1alpha1")` [VERIFIED: machinery@v1.14.2/resources/config/machine_config.go:22].
- Der Knoten sendet seine Bytes (`YamlMarshalled`); der Client lädt sie mit `configloader.NewFromBytes` zu einem **read-only Container, der die Byte-Darstellung erhält** (`NewReadonly ... preserves byte representation`, `Bytes()` gibt `container.bytes` zurück) [VERIFIED: machinery container.go:131, 1110-1120; resources/config/machine_config.go MarshalProto/UnmarshalProto]. Roh-Bytes enthalten also Kommentare und die Dokumentreihenfolge des Knotens (gemessen: 18586 Bytes roh, 6721 redigiert, 28 Dokumente).
- `Redact` = `provider.RedactSecrets(RedactedMarker)` + `EncodeBytes(CommentsDisabled)` + PEM-Sweep; `RedactedMarker = "<redacted by holzkube-manager>"` [VERIFIED: internal/machineconfig/redact.go:28, 58-79]. **Parst die Konfiguration nicht, liefert `Redact` Bytes UND einen Fehler** (nur PEM-Sweep, Zeilen 63-69). Diese Bytes sind kein Schnappschuss und dürfen nie gespeichert werden.
- Gemessen: `Redact(Redact(x)) == Redact(x)`; `Redact` zweimal auf derselben Eingabe: gleiche Bytes; das redigierte Ergebnis lässt sich mit `NewFromBytes` wieder laden (`REPARSE err=nil`) und durch `Compare` schicken.
- Gemessen: Vertauschen zweier Dokumente ändert die redigierten Bytes (`equal=false`), `Compare` meldet 0 Änderungen. **Byte-Gleichheit taugt deshalb nur nach Normalisierung** (Dokumente sortieren), strukturell muss über Dokumente verglichen werden.
- **Compare-Blindheit (gemessen):** `diff.go:97-98` ruft `yaml.Unmarshal(redactedBefore, &a)` auf; yaml.v3 liest damit nur das erste Dokument. Änderung `KubeletConfig.image` → `NON-FIRST DOC CHANGE: changes=0`; Änderung `certSANs` im ersten Dokument → `changes=1 paths=[.machine.certSANs]`. Das gilt auch für das bestehende `Plan`/`ModeFor` (Vorbefund, nicht Teil dieser Phase).
- **Redact-Umfang (gelesen, `v1alpha1_provider.go:109-205`):** ersetzt nicht-leere `machine.token`, `machine.ca.key`, Registry-Passwort/Auth/Identity-Token/TLS-Client-Key, WireGuard-Private-Key, VIP-API-Token (Equinix/Hetzner), statische Festplatten-Schlüssel, `cluster.secret`, `cluster.token`, AES-CBC-/Secretbox-Secret, Service-Account-Key, CA-Keys (Kubernetes, Aggregator, etcd); außerhalb von v1alpha1 implementieren Redact: `DiscoveryIdentityConfig`, `KubeAggregatorCAConfig`, `KubeAPIServerCAConfig`, `KubeServiceAccountConfig`, `KubeEtcdEncryptionConfig`, `WireguardConfig`, `HCloudVIPConfig`, `RegistryTLSConfig`, `RegistryAuthConfig`, siderolink `Config`, `User/Swap/Raw/VolumeConfig`, `EncryptionSpec` [VERIFIED: grep `func.*Redact(` in machinery@v1.14.2/config]. **Ersetzt wird nur, wenn das Feld nicht leer ist** (`redactBytes`: `if len(b) == 0 { return b }`).
- Nicht von Redact erfasst (kein `Redact`-Eintrag gefunden): `machine.files[].content`, `machine.env`, `machine.install.extraKernelArgs`, `cluster.inlineManifests[].contents`, `cluster.extraManifests` (URLs), `*.extraArgs`, `EnvironmentConfig`, `EtcFileConfig`, `ExtensionServiceConfig` [CITED: machinery-Quelltext, nur Abwesenheit eines Redact-Eintrags; ob diese Felder beim Betreiber Geheimnisse tragen, ist [ASSUMED]]. Wichtig: dieselben Werte zeigt die Config-Ansicht (Operator) heute schon. Neu ist, dass sie **dauerhaft in `config-baselines/`** liegen.
- Gemessen: sieben Redact-Marker in einer generierten Control-Plane-Konfiguration (`machine.token`, `cluster.token`, `clusterSecret`, vier Schlüssel); die CA-`key`-Felder erscheinen als Base64 des Markers (`PHJlZGFjdGVkIGJ5...`), nicht als Klartext-Marker. Diff-Ausgabe zeigt deshalb für Zertifikate große Base64-Blöcke (öffentliche `crt`-Felder werden nicht redigiert).

**Befund 2 (Frage 1): Rauschen (D-03). Gemessen vs. dokumentiert.**

| Kandidat | Ergebnis | Beleg |
|----------|----------|-------|
| Zwei Lesungen hintereinander | identisch (roh und redigiert) | gemessen am Simulator |
| Identische Bytes erneut applizieren, danach lesen | identisch | gemessen am Simulator (`SIM after apply-same ... equal=true`) |
| Kommentare, Schlüsselreihenfolge im Dokument | verschwinden durch `CommentsDisabled`-Encoding | gemessen indirekt: 18586 → 6721 Bytes; Schlüsselreihenfolge folgt der Struct-Reihenfolge des Encoders |
| Reihenfolge der Dokumente | **ändert Bytes, nicht Bedeutung** | gemessen; Normalisierung nötig (Muster 1) |
| Machinery-Version des Daemons (Encoding) | Ausgabe identisch zwischen v1.13.9 und v1.14.2 für eine generierte Standardkonfiguration (2 Dokumente, 10916 Bytes beide) | gemessen mit zwei Scratch-Modulen |
| Talos-OS-Upgrade ändert `machine.install.image` | **nein**: der Upgrade-Aufruf trägt das Image; der Upgrade-Job dieses Repos schreibt keine Konfiguration (einziger `ApplyConfigurationWithMode`-Aufruf im Paket ist `applyImage` für Kubernetes-Images) | Code: [VERIFIED: grep `ApplyConfiguration` in internal/upgrade → job.go:499 nur]; Doku: "The upgrade API call passes a node the installer image to use to perform the upgrade." [CITED: docs.siderolabs.com/talos/v1.14/.../upgrading-talos] |
| Kubernetes-Upgrade ändert Image-Felder | ja, **aber das ist ein Apply** (eigener: Basis nachziehen; `talosctl upgrade-k8s` eines anderen: echte Drift) | Code `internal/upgrade/job.go:459-500` |
| Talos schreibt die aktive Konfiguration selbst um | Doku nennt keinen Fall; "The supported configuration may change between versions ... The upgrade process should handle such changes transparently" ohne Details [CITED: upgrading-talos v1.14] | **Abwesenheit** von Evidenz, daher [ASSUMED] (Assumptions Log A1) |
| Staged-Apply | `Staging leaves the running configuration unchanged` ("doesn't modify current node configuration") [CITED: editing-machine-configuration v1.14] | siehe Befund 4 |

**Ergebnis für D-03: Es gibt keinen Pfad, für den ein "ändert sich ohne Zutun" belegt ist. Die Ausnahmeliste ist leer.** Empfehlung: **keinen** Ausnahme-Mechanismus bauen (toter Code mit null Einträgen), den Satz "holzkube-manager ignoriert keinen Pfad" in den Leitfaden schreiben und die Messung als Test festhalten (Lesen, Re-Apply identischer Bytes, Lesen: kein Drift). Das ist eine Auswahl für den Betreiber (Open Question 6), weil D-03 "kleine feste Liste" erwartete. Bei einem Fehlalarm im Betrieb ist das Aufnehmen eines Pfades eine Ein-Zeilen-Änderung.

**Befund 3 (Frage 3): Basis-Persistenz.**
- fsstore-Muster [VERIFIED: internal/store/fsstore/atomic.go:12-16, fsstore.go]: `dirPerm = 0o700`, `filePerm = 0o600`, Schreiben `CreateTemp(dir, tempPrefix+"*")` → chmod → write → fsync → rename → fsync(dir); `tempPrefix = store.TempFilePrefix` mit `const TempFilePrefix = ".holzkube-manager-tmp-"` [VERIFIED: internal/store/store.go:45]; `Open` räumt Temp-Dateien beim Start (`sweepTempFiles`) und lehnt zu weite Rechte ab (`Guard`). Entity-Verzeichnisse stehen in `fsstore.go:223-224` (`kindPatches = "patches"`, `kindMachineClasses = "machine-classes"`); ein neuer Entity heißt `kindConfigBaselines = "config-baselines"`.
- **Empfehlung: generischer `recordStore[model.MachineID, model.ConfigBaseline]`** (wie `machineStore`/`patchStore`): `Get`/`List`/`Put` (Rev-CAS, Konflikt bei falscher Rev)/`Delete`. Schlüssel = MachineID (UUID, `safeKey` erlaubt `[A-Za-z0-9_-]`). Keine neue Schnittstelle "mit Pfaden": `store.Store` bekommt `ConfigBaselines() ConfigBaselineStore` mit denselben vier Methoden wie `PatchStore`.
- Format (Vorschlag, **costly to change**, deshalb dem Planner/Betreiber ausdrücklich zur Bestätigung):
  ```go
  type ConfigBaseline struct {
      Machine   MachineID `json:"machine"`
      Rev       uint64    `json:"rev"`
      Format    int       `json:"format"`              // 1; erlaubt Migration im Code statt im Verzeichnis
      Cluster   ClusterID `json:"cluster"`             // unter dem die Basis genommen wurde
      Snapshot  string    `json:"snapshot"`            // normalisiert, redigiert (Muster 1)
      Digest    string    `json:"digest"`              // sha256 hex von Snapshot
      TakenAt   time.Time `json:"taken_at"`
      Source    string    `json:"source"`              // first-sight | apply | confirmed
      Machinery string    `json:"machinery"`           // gendata.VersionTag zum Zeitpunkt
      DriftSince time.Time `json:"drift_since,omitzero"`
      Expected  *ExpectedConfig `json:"expected,omitempty"` // Muster 3
  }
  ```
- Größe: ~6,7 KB für die minimale generierte Control-Plane-Konfiguration (gemessen), echte Knoten mit mehr Dokumenten rechnerisch im zweistelligen KB-Bereich [ASSUMED]; ein Deckel (z. B. 1 MiB, darüber "unreadable: too large") schützt vor einem entarteten Knoten.
- **Beschädigte Datei:** `readJSON` liefert bei kaputtem JSON einen Unmarshal-Fehler (nicht `ErrNotFound`). Der Leser muss das **von "keine Basis" unterscheiden** und als eigenen Zustand zeigen ("baseline unreadable"), nie als first-sight überschreiben und nie als "keine Drift". Ebenso: `Format` unbekannt, `Snapshot` leer oder `Digest != sha256(Snapshot)` ist beschädigt.
- **Vor jedem Schreiben `machineconfig.ContainsPrivateKey(snapshot)` prüfen** und ablehnen (Gürtel zu Redact, wie `support.go:342`).
- **Löschen bei Forget:** `inventory.Service.ForgetMachine` (`internal/inventory/mutate.go:179`) löscht nur den Machine-Record und die Observer; `ForgetCluster` (`mutate.go:205`) macht die Maschinen zu unzugeordneten Records (`unassign`), löscht sie nicht. Es gibt keinen Haken. Das Vorbild für "Zustand zu verschwundenen Maschinen räumen" ist **kein Haken, sondern ein Abgleich im periodischen Lauf**: `history.Retain(known, live)` im Sampler (`internal/history/sampler.go:205`). Empfehlung: im `Tick` Basen löschen, deren Maschine fehlt oder deren `Cluster` nicht mehr zum Machine-Record passt. Das deckt `ForgetMachine`, `ForgetCluster`, Umhängen in einen anderen Cluster und eine wiederhergestellte Sicherung ab.
- **Reset und Neuprovisionierung** derselben Maschine (gleiche UUID, gleicher Cluster) hinterlassen eine veraltete Basis, die der Abgleich nicht sieht. Folge ohne Gegenmittel: riesige Fehl-Drift nach jeder Neuinstallation. Haken: der `Record`-Schritt des Provisionierungs-Jobs (`provision/job.go` ruft `d.Record(ctx, req.Cluster, req.Addr, req.ControlPlane)`; die Maschine ist dort `req.UUID`) löscht die Basis; Signatur im Planner prüfen.
- **Wiederherstellung aus Datenverzeichnis-Sicherung:** ein Tarball enthält `config-baselines/` mit (`backup/backup.go` schließt nur `backups/`, `upgrade-snapshots/`, Lockdatei und Temp-Dateien aus). Eine ältere Sicherung bringt ältere Basen; Änderungen seither erscheinen als Drift und werden bestätigt. Das ist ehrlich und braucht keine Sonderbehandlung; der Leitfaden sagt es in einem Satz.
- **Schema-Version:** `const CurrentVersion = 6` [VERIFIED: internal/store/migrate/migrate.go:26]; jeder bisherige Entity-Ordner hat eine eigene Migration (zuletzt `5 → 6` für `machine-classes`). Es gibt keinen Test, der eine Migration je Ordner erzwingt (gesucht, keiner gefunden). Ein Bump auf 7 hat eine Folge: **ein zurückgerolltes Binary lehnt das Verzeichnis ab** (`ErrVersionTooNew`); `deploy/holzkube-manager-update.sh` rollt bei ungesundem Update auf die vorige Version zurück. Auswahl für den Betreiber (Open Question 1).

**Befund 4 (Frage 4): Wann die Basis geschrieben wird.**
- Erste Lesung eines Knotens (Zustand `baseline-created`, mit Satz). Nur für Maschinen mit Cluster, über `inv.MachinesOf` (liefert nur "present" Maschinen); `Disabled` (vom Betreiber ausgeschaltet) überspringen und nicht in den Nenner "nicht gelesen" zählen.
- Schreibpfade auf die Konfiguration eines **verwalteten** Knotens [VERIFIED: grep `ApplyConfiguration` über internal/ und cmd/]:
  1. `machineconfig.Service.Apply` (`service.go:223`, Route `config.apply`), Modi frei wählbar.
  2. `upgrade.applyImage` (`upgrade/job.go:459-500`, Kubernetes-Upgrade, mehrere Applies je Knoten und Lauf, Modus aus `ModeFor([]string{change.path})`).
  3. `rotateca.Deps.rewrite` (`rotateca/job.go:429-480`, `ModeNoReboot`, mehrere Durchgänge je Knoten).
  4. Provisionierung über `MaintenanceClient.ApplyConfiguration` (`provision/job.go:139`): neuer Knoten, noch ohne Basis; erste Lesung danach.
  Die Pfade 1-3 laufen alle über `ClusterClient.ApplyConfigurationWithMode` (`talos/client.go:1036`). `jobs.AfterReboot(configSvc.ClearStaged)` (`main.go:284`) ist **kein** Schreibpfad, nur ein Staged-Marker.
- Aktive vs. gestagte Konfiguration: `PersistentID = resource.ID("persistent")` mit dem Kommentar "PersistentID might be ahead of the "current" ID if the config was submitted in e.g. "staged" mode" und "might be behind ... "try" mode" [VERIFIED: machinery resources/config/machine_config.go:26-28]. Doku: staged ändert die laufende Konfiguration nicht [CITED]. Das bestehende `stagedRecord` ist nur im Speicher und geht beim Neustart verloren (`service.go:53-62`).
- **Simulator-Lücke (gemessen):** `talossim` übernimmt auch eine `staged`-Konfiguration sofort als aktive (`SIM STAGED apply: active changed=true`); `talossim/machine.go:857-877` setzt für jeden Nicht-Dry-Run-Apply die Konfiguration. Der Simulator kann die Staged-Semantik also nicht beweisen; dafür braucht es einen Fake oder eine Simulator-Erweiterung (Wave 0, Validation Architecture).
- Modus-Wirkung auf die aktive Konfiguration:

| angefragter Modus | aktiv nach Rückkehr des RPC | Basis |
|---|---|---|
| `no-reboot` | neu, sofort | nach Rückkehr: Lesung = Basis |
| `reboot` / `auto` (mit Neustart) | alt bis zum Neustart, dann neu; Knoten kurz nicht lesbar | erst übernehmen, wenn die neue Fassung gelesen wird |
| `staged` | **unverändert** bis zum nächsten Neustart (auch durch Dritte) | nicht sofort; übernehmen, wenn die erwartete Fassung aktiv wird |
| `try` | neu für ~1 Minute, dann zurück | **nicht** übernehmen (Basis bleibt gültig) |

  (`ModeNoReboot`, `ModeReboot`, `ModeStaged`, `ModeTry` = `"no-reboot"`, `"reboot"`, `"staged"`, `"try"` [VERIFIED: internal/machineconfig/applymode.go:33-43]; der Handler akzeptiert den Modus als freie Zeichenkette, `auto` ist im Client abgebildet, `client.go:1079`.)
- Konsequenz: **"nach Apply neu lesen und das Gelesene zur Basis machen" ist für `reboot`/`auto`/`staged` falsch** (liest die alte Fassung, danach landet die neue als Fehl-Drift) und für `try` ebenfalls. Empfehlung "erwarteter Digest" in Muster 3.
- Race "Apply im Flug während periodischer Lesung": mit dem erwarteten Digest verschwindet sie, weil jede Zwischenlesung entweder der alten Basis (ok) oder der erwarteten Fassung (übernehmen) gleicht; es gibt keine Interleaving-Reihenfolge, die Fehl-Drift erzeugt. Ohne den Digest braucht es eine In-Flight-Markierung plus Generationszähler.

**Befund 5 (Frage 5): Periodischer Leser.**
- Gerüst: `certwatch.Deps{Clusters, ControlPlanes, Read, Every, Now, Log}`, `Run(ctx, firstAfter)` mit `time.NewTimer`, `Tick` ruft `Refresh` je Cluster, ein nicht antwortender Knoten behält seine alten Einträge, `HealthFor(cluster, now)` liefert `nil` bis zur ersten Lesung [VERIFIED: internal/certwatch/certwatch.go:97-248]. Verdrahtung in `main.go:555-568` (`inv.SetCertificateHealth(certWatch.HealthFor)`) und `main.go:796` (`go certWatch.Run(certCtx, time.Minute)`).
- Unterschiede für Drift: **alle** Knoten des Clusters (`inv.MachinesOf`), nicht nur Control Planes; zusätzlich ein Schreibpfad (Basis) und ein persistierter Zustand (`DriftSince`).
- **Deadline (kritisch):** `requireDeadline` lehnt jeden Aufruf ohne Deadline ab (`talos/deadline.go:401-406`, `ErrNoDeadline`); der Konstruktor setzt nur für den Probe-Aufruf ein eigenes Budget (`proveAnswering`: `WithTimeout(withProbeClass(ctx), ProbeDeadline)`, `client.go:539`). Das COSI-Get danach nicht. `certWatch.Run(certCtx...)` läuft auf `context.WithCancel(context.Background())`. Gemessen mit demselben Kontext: `CONNECT without deadline: err=<nil>`, `CERTIFICATES without deadline: ... talos: refusing a call with no deadline`, `MACHINECONFIG without deadline: ... refusing a call with no deadline`. Das ist derselbe Fehler wie Ledger 137 (Heartbeat, `.planning/WINDOWS.md:162`: "refusing a call with no deadline"; Wächter dort: `TestASupervisorOnALifetimeWithNoDeadlineStillReadsItsNode` startet ohne Deadline). Folge für diese Phase: Der Leser setzt je Knoten `context.WithTimeout(ctx, readBudget)` um Connect **und** Lesen und ruft `talos.WithClassDeadline(ctx, talos.MethodCOSIGet)` wie `rotateca.configOf` (`rotateca/job.go`). `ProbeDeadline = 5 * time.Second`, `FastReadDeadline = 10 * time.Second` [VERIFIED: internal/talos/deadline.go], also Budget ~20 s je Knoten. Der Test **muss** den Dienst auf einem Kontext ohne Deadline starten (die Fixtures der Repo-Tests nutzen `testContext` mit 60 s und sehen den Fehler deshalb nie, Ledger 137).
- Takt: Empfehlung **3 Stunden**, erste Lesung 1 Minute nach Start (wie certwatch), dazu sofort nach eigenem Apply und einmal nach `AfterReboot`. Begründung: Lesen ist ein Connect plus ein COSI-Get je Knoten (Größenordnung Sekunden bei wenigen Knoten); "alle paar Stunden" (D-04); die Banner-Schwelle ist 24 h, die Erkennungslatenz liegt also bei Takt + 24 h. Der Takt ist Ermessen (Claude's Discretion).
- Nebenläufigkeit: seriell je Cluster und Knoten genügt (Größenordnung 1-10 Knoten, jedes Budget ~20 s); der Sampler nutzt `hardwareConcurrency = 8` mit Semaphor [VERIFIED: internal/history/sampler.go:91] falls später nötig. `Tick` bricht bei `ctx.Err()` ab.
- Leases/Sperren: Lesen läuft über `inv.Connect` und nimmt weder Job-Lease noch Cluster-Sperre. Die "Cluster ist read-only gesperrt"-Prüfung (`ClusterLocked`) hängt an **Routen** (`Route.ClusterScope`), nicht am Connector; ein Lesen wartet auf nichts. Der Bootstrap-Lease liegt im Provisionierungspfad.
- Logging: certwatch loggt jeden Fehlschlag in jedem Lauf (`certwatch.go:194`). Der Sampler loggt nur Übergänge (`failed`/`recovered`, `sampler.go:353-377`) und ist damit das bessere Vorbild; ein Knoten, der drei Stunden aus ist, soll nicht jede Stunde eine Zeile erzeugen.

**Befund 6 (Frage 6): Bestätigen. Route, Sudo, Audit.**
- `Route`-Felder [VERIFIED: internal/httpapi/router.go:54, 86, 90, 120]: `Destructive bool`, `MinRole model.UserRole`, `Action string`, `ClusterScope func(*http.Request) (string, error)`. `New` verweigert eine Session-Route ohne `MinRole`.
- Vorbild `cluster.backup-schedule` [VERIFIED: internal/httpapi/handlers/backups.go:42-50]: `Method: http.MethodPut`, `Pattern: "/api/v1/clusters/{id}/backups/schedule"`, `RequiresSession: true`, `MinRole: model.RoleAdmin`, `Destructive: true`, `Action: "cluster.backup-schedule"`; kein `ClusterScope`, weil "it changes nothing on any node, so a cluster adopted read-only can still be backed up". Allowlist-Eintrag `"cluster.backup-schedule": {"interval", "keep"},` [VERIFIED: internal/audit/redact.go:560]. Vorbild `user.identity-unlink`: `Destructive: true`, Allowlist `{}` (redact.go:145), Konto als serverseitiger Parameter (`accountPath`, `AccountParam`).
- `model.RoleAdmin UserRole = "admin"` [VERIFIED: internal/model/model.go:154].
- **Sudo: ja, empfohlen.** Die Projektregel steht an `Destructive` und an den Präzedenzfällen: `backup-schedule` ist `Destructive`, obwohl nichts zerstört wird, "as the other settings that decide what this product does on its own"; `wall-link.create` "D-06 exists for acts whose consequences outlast the click". Gegenbeispiel `kubernetes.go:169`: nicht `Destructive`, wo die Route "cannot write ... so a sudo window would be theatre". Eine Bestätigung überschreibt die Basis, also den einzigen Beleg dessen, was sich geändert hat, und schaltet das Banner ab. Das ist eine Sicherheitsaussage, kein Komfort. Frontend: kein eigener Dialog nötig, `api.ts` behandelt 428 global; nur ein Eintrag in `ACTION_LABELS` (`api.ts:~655`), z. B. `{ match: /^\/api\/v1\/machines\/[^/]+\/config\/drift\/confirm$/, action: 'Confirm this configuration as the new baseline' }`. Auswahl in Open Question 4 (Empfehlung: Sudo).
- **Audit-Lücke (gemessen/gelesen):** `auditAdapter.Attempt` (`router.go:664-689`) füllt `Actor`, `Session`, `SrcIP`, `Action`, `Params`; `MachineID`/`ClusterID` bleiben leer. Weitere Aufrufer von `audit.Record{...}`: `oidc.go:453`, `commands.go:493`. Der Knoten steht nirgends. Es braucht (a) im Body `digest` (Allowlist `{"digest"}`, ein Hash ist unbedenklich und beweist, **welcher** Stand bestätigt wurde) und (b) einen serverseitig aus dem Pfad geschriebenen Parameter `machine` nach dem Muster `accountPath`/`AccountFromPath` (`audit/redact.go`, `middleware/audit.go:119-122`), geschrieben **nach** der Redaktion, damit ein Body-Schlüssel `machine` ihn nicht bestimmt. `TestEveryAuditedActionIsInTheAllowlist` und `TestEveryAllowlistedActionIsAReachableRoute` (`cmd/holzkube-managerd/allowlist_test.go`) erzwingen den Allowlist-Eintrag in beide Richtungen.
- Neue Fehlercodes im Muster `conflict.*` (`problem.go`): z. B. `conflict.drift-changed` (Lesung hat sich seit der Anzeige geändert, Digest passt nicht), `conflict.no-drift` (nichts zu bestätigen), `conflict.not-read-yet`. Jeder gehört in die Tabelle in `docs/api-contract.md`.
- **Digest-CAS:** Bestätigt wird "der Stand, den ich gesehen habe": `{"digest": "<sha256 der Lesung>"}`; weicht der Digest der aktuellen Lesung davon ab, 409. Sonst könnte zwischen Anzeige und Klick ein dritter Eingriff mitbestätigt werden. Bestätigen nutzt die zuletzt **gelesene** Lesung im Speicher (kein Knotenzugriff, Budget-Test `noUpstream`); nach einem Daemon-Neustart ohne Lesung: `conflict.not-read-yet`.
- Dry-run: Routen werden nicht per Methode gesperrt; `--dry-run` greift am Talos-`Mode` und im `machineconfig.Service` (`ErrDryRun`). Das Bestätigen schreibt nur lokalen Zustand und funktioniert in dry-run wie `backup-schedule` (zu bestätigen im Test, Planner).

**Befund 7 (Frage 7): Banner und "seit wann".**
- Ohne Persistenz würde jeder Neustart die 24-h-Uhr auf null setzen; der stündliche Update-Timer (`holzkube-manager-update.timer`, CLAUDE.md) startet den Dienst bei jedem installierten Release neu. In einer Phase mit vielen Releases würde das Banner nie erscheinen. **Empfehlung: `DriftSince` im Basis-Datensatz.** Gesetzt, wenn eine Lesung erstmals abweicht (`DriftSince.IsZero()`), gelöscht, wenn eine Lesung wieder der Basis gleicht oder bestätigt wird. Das sind zwei Schreibvorgänge je Drift-Episode, nicht je Lesung.
- Nach einem Neustart und vor der ersten Lesung sagt der Knoten aus dem Datensatz "Drift seit X (noch nicht gelesen seit dem Start)" statt "not read yet" zu verschweigen; das Banner zählt weiter.
- Banner-Regel serverseitig beim Bauen der Sicht (`now` wird mitgegeben, wie bei `HealthFor(c, now)`): `now - DriftSince >= 24h`, Zustand Drift (gelesen oder aus Datensatz). Der Satz kommt vom Server (wie `certificate_warning`), z. B. "The configuration of cp-1 (homelab) has differed from what holzkube-manager last knew since 2026-10-14 08:00. Open the node to see what changed and confirm it, or find out who changed it." Wortlaut ist Ermessen.
- `CertificateBanner` fragt `api.clusters.list()` alle 5 Minuten (`refetchInterval: 5 * 60 * 1000`) und filtert `c.node_certificates?.urgency === 'banner' || ... 'critical'`; eingehängt in `AppShell.tsx:79`. Die Drift-Banner dürfen dieselbe Abfrage (`queryKey: ['clusters']`) mitbenutzen, also keine zusätzliche Anfrage; ein Zustandsfeld wie `config_drift.banner: boolean` plus `warning: string` genügt. Die Komponente liegt am besten als eigene Komponente neben `CertificateBanner`, in `AppShell` daneben eingehängt (das Banner ist keine Zertifikatsfrage).

**Befund 8 (Frage 8): Anzeige, API, Metriken, Budget, Wand.**
- `ClusterView` bekommt `ConfigDrift *model.ConfigDriftHealth` (`json:"config_drift,omitempty"`), gesetzt über `inv.SetConfigDrift(fn)` nach dem Muster `SetCertificateHealth` (`inventory/view.go:426-440`, aufgerufen in `Clusters()` und `Cluster()` neben `certificateHealthOf`). Inhalt: `CheckedAt`, `ReadNodes`, `Nodes`, `Drifted`, `Banner`, `Warning`, `Items[]{machine,node,state,since,read_at}`. Kein Diff-Inhalt in der Cluster-Liste (die ist Reader-lesbar; der Diff ist Konfigurationsinhalt wie die Config-Ansicht, **Operator**).
- Knotenroute: `GET /api/v1/machines/{id}/config/drift` (`MinRole: model.RoleOperator`, wie `GET .../config`), Antwort: Zustand, Zeiten, Basis-Quelle, `drift_since`, Diff (Mehrdokument-`Diff`), `digest` der Lesung. `POST /api/v1/machines/{id}/config/drift/confirm` (Befund 6).
- **Budget-Wächter:** `TestEveryRouteThatReachesUpstreamHasABudgetRow` (`cmd/holzkube-managerd/budget_test.go:1795`) verlangt für **jede** Route der Tabelle entweder eine `routeBudgets`-Zeile oder einen Eintrag in `noUpstream`. Beide neuen Routen reichen keinen Knoten an (der Leser läuft nicht über eine Route, Bestätigen nutzt die gespeicherte Lesung) und gehören in `noUpstream`, je mit Kommentar im Stil der Nachbarn. Fehlt der Eintrag, wird der Test rot.
- Metriken (`internal/metrics/metrics.go`; vorhandene Namen `holzkube_cluster_client_certificate_seconds` :184, `holzkube_node_certificate_seconds` :197, `holzkube_cluster_locked`): Vorschlag `holzkube_cluster_config_drift{cluster}` (0/1, 1 wenn irgendein Knoten abweicht) und `holzkube_node_config_drift{cluster,node}` (0/1) **nur für Knoten mit Lesung oder gespeichertem Drift**, nie 0 für "nicht gelesen"; dazu `holzkube_node_config_read_age_seconds{cluster,node}` (Alter der letzten Lesung, fehlend wenn nie gelesen), damit eine Alarmregel Veraltung erkennt. Der Exporter liest nur `ClusterView`, kein Knotenzugriff beim Scrape.
- Dokumentationsstellen: `docs/api-contract.md` (Abschnitt "Certificates inside a cluster" um Z. 3330-3370 als Vorlage; neuer Abschnitt, Fehlercodes, Audit-Action, Metriken Z. ~4700), `docs/guide.md` (Abschnitt "Certificates inside the cluster" Z. ~1268 als Vorlage; Datenverzeichnis-Liste Z. ~832-849 um `config-baselines/` ergänzen), `README.md` ("What it does": ein Stichpunkt; Screenshot in "A look around" nur wenn ein Bildschirm für sich entsteht, hier ist es eine Zeile und ein Abschnitt, daher Screenshot optional), `web/fixtures/demo.json` für gerenderte Bilder.
- Wand: nicht Teil der Phase. Die einzige Wand-Route projiziert explizit (`wallCertificatesFrom`, `handlers/wall_certificates.go`); `TestOnlyTheWallAcceptsAWallLink` hält `WallLink` auf eine Route. Ein neues `ClusterView`-Feld gelangt nicht auf die Wand.

**Befund 9 (Frage 9):** siehe Validation Architecture ("Injizierbare Fehler").

### Pattern 1: Normalisierter, redigierter Schnappschuss
**What:** `Snapshot(raw)` = `Redact(raw)` (bei Fehler abbrechen, die Teil-Bytes verwerfen), dann Dokumente an `---` zerlegen (yaml.v3 `Decoder`), nach Schlüssel sortieren, wieder zu einem Text zusammensetzen. Schlüssel: `v1alpha1` für das Dokument mit `version: v1alpha1`, sonst `kind` und bei benannten Dokumenten `kind/name` (gemessen existieren zwei `KubeAuthorizerConfig` mit den Namen `node` und `rbac`, ein `VolumeConfig` mit `name: EPHEMERAL`, ein `DiscoveryServiceConfig` mit `name: default`). Doppelte Schlüssel mit laufender Nummer unterscheiden. `Digest = sha256(Snapshot)`.
**When to use:** vor jedem Vergleich und vor jedem Speichern, **auch auf der gespeicherten Basis** (`Redact` ist idempotent, gemessen), damit eine geänderte Machinery-Version des Daemons keine Fehl-Drift erzeugt.
**Example:**
```go
// Skizze. Werte aus dem gelesenen Code: Redact gibt bei Parsefehler Bytes UND error zurück
// (internal/machineconfig/redact.go:63-69); diese Bytes sind kein Schnappschuss.
func Snapshot(raw []byte) (Snap, error) {
    red, err := Redact(raw)
    if err != nil {
        return Snap{}, fmt.Errorf("configdrift: unreadable configuration: %w", err) // nie red speichern
    }
    if ContainsPrivateKey(red) {
        return Snap{}, errors.New("configdrift: a private key survived redaction") // nie persistieren
    }
    docs, err := splitDocuments(red) // yaml.v3 Decoder; Schlüssel = v1alpha1 | kind[/name]
    ...
    sort.Slice(docs, ...)
    text := joinDocuments(docs)
    sum := sha256.Sum256([]byte(text))
    return Snap{Text: text, Digest: hex.EncodeToString(sum[:])}, nil
}
```

### Pattern 2: Zustandsmaschine der Lesung (je Knoten)
Zustände (Server-Vokabular, englisch in der UI): `not-read` (seit Start keine Lesung, auch keine gespeicherte Drift), `baseline-created` (erste Lesung, Basis angelegt, wird zu `ok`), `ok`, `settling` (erwartete Fassung steht aus, jünger als Fenster), `drift`, `unreadable` (letzte Lesung scheiterte; mit Grund und Zeit der letzten guten), `baseline-damaged`. Regel je Lesung, in dieser Reihenfolge:
1. Lesen scheitert (Verbindung, Deadline, `Redact`-Fehler, "kind not registered" = Knoten neuer als der Daemon): Zustand `unreadable` mit Satz; **alte Lesung und `DriftSince` bleiben**; Knoten zählt als nicht gelesen. Den "not registered"-Fall wie `unknownKindPattern` (`inventory/secrets.go:83`) in einen Satz "holzkube-manager is older than this node's Talos; update it" übersetzen.
2. Basis fehlt (`store.ErrNotFound`): anlegen (`Source: first-sight`), `baseline-created`.
3. Basis beschädigt: `baseline-damaged`, nichts überschreiben.
4. `digest == base.Digest`: `ok`; war `DriftSince` gesetzt, löschen.
5. `base.Expected != nil && digest == base.Expected.Digest`: Basis := Lesung (`Source: apply`), `Expected` löschen.
6. `base.Expected != nil` und jünger als das Fenster (Vorschlag 15 min für `no-reboot`/`reboot`/`auto`; für `staged` ohne Ablauf, solange `Expected` steht): `settling`.
7. Sonst `drift`; `DriftSince` einmalig setzen (CAS-Put).
Nach jeder Entscheidung die Lesung im Speicher ablegen (Zustand, `at`, Snapshot-Text für den Diff, Digest, Fehlersatz). Schreiben der Basis nur in den Fällen 2, 4 (nur Löschen von `DriftSince`), 5, 7 (nur Setzen von `DriftSince`) und beim Bestätigen.

### Pattern 3: Eigener Apply über einen Haken am Engpass, mit erwartetem Digest
**What:** `ClusterClient` bekommt einen optionalen `ApplyObserver` (`Before(machine, cfg, mode) token`, `After(token, result, err)`), den `inventory.Connect` einsetzt, wenn das Inventory einen kennt (Setter-Muster wie `SetCertificateHealth`). `ApplyConfigurationWithMode` ruft `Before` vor dem RPC und `After` danach. Damit sind `Service.Apply`, `applyImage` und `rotateca.rewrite` und jeder künftige Aufrufer abgedeckt; ein neuer Aufrufer kann den Haken nicht vergessen.
- `Before`: für Modus ≠ `try`: `Expected := Snapshot(cfg).Digest` mit `Mode` und Zeit in den Basis-Datensatz schreiben **vor** dem RPC. (Existiert keine Basis, nichts tun: der Knoten hat keine Basis, die erste Lesung legt sie an.)
- `After(err != nil)`: `Expected` löschen, Basis unverändert (ein gescheiterter oder zeitüberschrittener Apply, der doch landete, erscheint als Drift; das ist die sichere Richtung: nie verbergen).
- `After(ok)` und angefragter Modus `no-reboot`: sofort lesen und die Lesung als Basis übernehmen (`Source: apply`), unabhängig vom Digest-Vergleich (Rückfall, falls ein echter Knoten Bytes anders ausliefert als gesendet, Assumption A2).
- `After(ok)` bei `reboot`/`auto`/`staged`: nichts weiter; der Leser übernimmt die erwartete Fassung, sobald er sie liest (Regel 5). Ein `AfterReboot`-Anstoß (`Poke`) beschleunigt das, ist aber nicht nötig.
- `try`: weder `Expected` noch Basisänderung (Basis bleibt gültig, die Rücknahme nach etwa einer Minute stellt sie wieder her).
- Dry-run (`talos.Mode.DryRun`): der Aufruf kommt gar nicht bis zum Knoten; der Haken darf dann keinen `Expected` hinterlassen (After mit Fehler räumt auf).
**Warum nicht je Aufrufer:** drei Pakete, vier Aufrufer, jede neue Schreib-Funktion wäre eine neue Vergessensstelle; der Engpass ist eine Funktion.

### Pattern 4: Mehrdokument-Vergleich
`machineconfig.CompareDocuments(beforeSnapshot, afterSnapshot string) (Diff, error)`: beide Seiten über denselben Dokumentschlüssel zerlegen, je Dokument `walk(prefix, a, b, &d)` (die vorhandene Funktion in `diff.go` wiederverwenden), Pfade mit Dokumentpräfix (`KubeletConfig.image`, `v1alpha1.machine.certSANs`); fehlendes/hinzugekommenes Dokument als `ChangeAdded`/`ChangeRemoved` mit Pfad = Schlüssel. Nicht `Compare` ändern (Open Question 2).

### Pattern 5: Bestätigen
`Confirm(ctx, machine, digest, actor)`: unter der Maschinen-Sperre des Dienstes: letzte Lesung holen (fehlt: `not-read-yet`), `reading.Digest != digest` → `drift-changed`, Zustand ≠ `drift` → `no-drift`; Basis := Lesung (`Source: confirmed`), `DriftSince` und `Expected` löschen, Rev-CAS-Put; Lesung im Speicher auf `ok` setzen. Kein Talos-Aufruf.

### Anti-Patterns to Avoid
- **`Compare` als Drift-Vergleich** (sieht nur Dokument 1, gemessen).
- **Byte-Gleichheit der redigierten Texte ohne Normalisierung** (Dokumentreihenfolge, gemessen).
- **Teil-Bytes von `Redact` (Fehlerfall) speichern oder diffen.**
- **Eine fehlgeschlagene Lesung als "keine Drift" oder als "ok" behandeln;** auch eine fehlgeschlagene Lesung darf eine gespeicherte Drift nicht löschen.
- **Basis nach einem Apply aus den eigenen gesendeten Bytes bilden** (Encoding-Unterschiede zur Lesung); immer aus der Lesung oder über den normalisierten Digest.
- **Den Leser auf dem Lauf-Kontext ohne Deadline starten** (Ledger 137).
- **Ein Mal pro Lesung die Basis neu schreiben** (Schreibverstärkung und Rev-Konflikte); nur bei Zustandswechsel.
- **Pfad-Ausnahmen "auf Verdacht"** (D-03 verlangt Beleg; Befund 2).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Atomare, gesperrte Datensätze | eigene Datei mit `os.WriteFile` | `fsstore` `recordStore` (Rev-CAS, `writeAtomic`, `safeKey`, `removeAndSync`) | Crash-Vertrag (Temp-Präfix, fsyncs, 0600), `TestNoDirectFileAccessOutsideFsstore` |
| Geheimnisse entfernen | eigener Regex/Feldliste | `machineconfig.Redact` + `ContainsPrivateKey` | Kennt das Schema der gepinnten Machinery; PEM-Sweep als zweite Lage |
| Struktureller Vergleich einzelner Dokumente | eigener YAML-Differ | `walk`/`diffList` aus `diff.go` (nur der Dokument-Rahmen ist neu) | Listenwachstum und Duplikate sind dort schon behandelt |
| Periodischer Lauf | eigener Scheduler | `Run(ctx, firstAfter)`/`Tick`/`Refresh` wie certwatch | Gleiches Start- und Fehlerverhalten, gleiche Tests |
| Rolle, Sudo, Audit einer Route | eigene Prüfung im Handler | `Route{MinRole, Destructive, Action}` | Deklarativ, von Wächtern geprüft |
| Aufräumen verschwundener Maschinen | Haken in `ForgetMachine` | Abgleich im `Tick` (`history.Retain`-Muster) | Deckt Forget, ForgetCluster, Umhängen, Restore mit einer Regel ab |
| Deadline | eigene Timeout-Logik | `talos.WithClassDeadline(ctx, talos.MethodCOSIGet)` | Klassenbudget zentral, `requireDeadline` verlangt es |

**Key insight:** In dieser Domäne ist nicht das Lesen oder Speichern schwer, sondern die **Ehrlichkeit an den Rändern**: was als "gleich" zählt (Normalisierung), wann ein eigener Apply die Basis verschieben darf (Modi), und dass jeder Fehlerpfad "unbekannt" sagt statt "in Ordnung".

## Common Pitfalls

### Pitfall 1: Leser ohne Deadline
**What goes wrong:** Jede Lesung scheitert mit "refusing a call with no deadline"; die Karte zeigt dauerhaft "0 of N read" oder (bei falscher Fehlerbehandlung) "keine Drift".
**Why it happens:** `Run`-Kontext ohne Deadline (`context.WithCancel(context.Background())`); Tests nutzen 60-s-Kontext (Ledger 137). Gemessen für `certwatch` und `MachineConfigYAML`.
**How to avoid:** Budget je Knoten im Leser (Befund 5); Wächter startet den Dienst auf `context.Background()`.
**Warning signs:** Journal "talos: Get on <node>: talos: refusing a call with no deadline".

### Pitfall 2: Nur das erste Dokument verglichen
**What goes wrong:** Änderungen an `KubeletConfig`, `DiscoveryServiceConfig`, Volume-, Netzwerk- und Kubernetes-Dokumenten sind unsichtbar.
**How to avoid:** Mehrdokument-Vergleich (Muster 4); Test mit Änderung in einem Nicht-Erst-Dokument (rot gegen `Compare`).

### Pitfall 3: Basis nach Apply falsch gesetzt (staged/try/reboot/auto)
**What goes wrong:** Fehl-Drift nach einem Neustart oder nach der Rücknahme von `try`; oder eine echte Änderung wird verschluckt.
**How to avoid:** Muster 3 (erwarteter Digest); `try` nie übernehmen; `staged` erst bei Landung.
**Warning signs:** Drift erscheint genau nach einem eigenen Apply, "seit" ≈ Zeit des Applys.

### Pitfall 4: `Redact`-Teilergebnis gespeichert
**What goes wrong:** Bei nicht ladbarer (z. B. zu neuer) Konfiguration liefert `Redact` Bytes plus Fehler; wer nur die Bytes nimmt, speichert eine ungeprüfte Fassung, die Geheimnisse außerhalb PEM enthalten kann.
**How to avoid:** Fehler zuerst prüfen; Test mit zu neuem Dokumentkind.

### Pitfall 5: Beschädigte Basis wird zu first-sight
**What goes wrong:** Eine kaputte JSON-Datei wird als "keine Basis" behandelt, überschrieben, die Drift ist weg.
**How to avoid:** `ErrNotFound` vs. Parsefehler trennen; `baseline-damaged` zeigen.

### Pitfall 6: Veraltete Basis nach Reset/Neuprovisionierung
**What goes wrong:** Dieselbe UUID, neue Konfiguration: riesiger Fehl-Diff.
**How to avoid:** Basis im Provisionierungs-`Record`-Schritt löschen (Befund 3).

### Pitfall 7: Banner-Uhr im Speicher
**What goes wrong:** Jeder (stündlich mögliche) Neustart setzt die 24 h zurück.
**How to avoid:** `DriftSince` im Datensatz (Befund 7).

### Pitfall 8: Audit-Eintrag ohne Knoten
**What goes wrong:** "admin hat Drift bestätigt", aber nicht auf welchem Knoten (gelesen: `machine_id` bleibt leer).
**How to avoid:** serverseitiger Parameter aus dem Pfad plus `digest` (Befund 6); Test, dass ein Body-Schlüssel `machine` ihn nicht bestimmt.

### Pitfall 9: Reine Geheimnisänderung unbemerkt
**What goes wrong:** Rotiertes Token/Secret ohne Spur. **How to avoid:** nicht verhindern, sondern **benennen** (D-02): ein Satz auf Knotenseite und im Leitfaden. Eine Sicherung über einen Digest der unredigierten Fassung wäre ein Rückfall in "Hash gespeichert", den D-02 ausschließt.

### Pitfall 10: Metrik `0` für "nicht gelesen"
**What goes wrong:** Prometheus-Alarm "kein Drift" über einen Knoten, der nie gelesen wurde. **How to avoid:** Serie nur mit Lesung; `read_age_seconds` fehlt bei nie gelesen; Test.

### Pitfall 11: Veraltete Lesung im Speicher nach Bestätigen/Löschen
**What goes wrong:** Die Anzeige zeigt nach Bestätigen weiter den alten Diff. **How to avoid:** Bestätigen setzt die Lesung im Speicher; Test über `HealthFor` nach `Confirm`.

### Pitfall 12: Nenner "nicht gelesen" mit ausgeschalteten Knoten
**What goes wrong:** Vom Betreiber ausgeschaltete (`Disabled`) Knoten erzeugen Dauer-"nicht gelesen". **How to avoid:** überspringen (Befund 4).

## Code Examples

### Routen-Deklaration (nach `backups.go:42-50`)
```go
// Skizze; Muster und Feldnamen aus handlers/backups.go:42-50 und router.go:45-121 gelesen.
{
    Method: http.MethodGet, Pattern: "/api/v1/machines/{id}/config/drift",
    RequiresSession: true, MinRole: model.RoleOperator,
    Handler: handler(getConfigDrift(d)),
},
{
    // Admin und Destructive wie cluster.backup-schedule: nichts am Knoten, aber die Aktion
    // überschreibt den Beleg und schaltet das Banner ab. Kein ClusterScope (ändert keinen Knoten).
    Method: http.MethodPost, Pattern: "/api/v1/machines/{id}/config/drift/confirm",
    RequiresSession: true, MinRole: model.RoleAdmin, Destructive: true,
    Action: "config.drift-confirm", Handler: handler(confirmConfigDrift(d)),
},
```
Allowlist (`audit/redact.go`): `"config.drift-confirm": {"digest"},` plus serverseitiger Parameter `machine` aus dem Pfad.

### Leser mit eigenem Budget (Pflicht, Ledger 137)
```go
// readBudget deckt Probe (5 s) und COSI-Get (10 s) aus talos/deadline.go plus Reserve.
const readBudget = 20 * time.Second

read := func(ctx context.Context, id model.MachineID) ([]byte, error) {
    ctx, cancel := context.WithTimeout(ctx, readBudget) // ohne dies lehnt requireDeadline ab
    defer cancel()
    cc, err := inv.Connect(ctx, id)
    if err != nil { return nil, err }
    defer func() { _ = cc.Close() }()
    rctx, c2, err := talos.WithClassDeadline(ctx, talos.MethodCOSIGet)
    if err != nil { return nil, err }
    defer c2()
    return cc.MachineConfigYAML(rctx)
}
```

### Wächter ohne Deadline (Form des Wächters aus Ledger 137)
```go
// Start auf einem Kontext ohne Deadline: genau die Lage in main.go. Ein 60-s-testContext
// würde den Fehler verdecken.
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
svc.Tick(ctx)
// erwartet: Knoten gelesen (state ok/baseline-created), nicht unreadable
```

## State of the Art

| Alt | Aktuell | Seit | Wirkung |
|-----|---------|------|---------|
| Eine v1alpha1-Konfiguration | v1alpha1 plus typisierte Dokumente (28 in der generierten CP-Konfiguration) | Talos v1.14 | Dokumentweiser Vergleich nötig; Daemon muss die Kinds kennen (Pin) |
| `talosctl ... --mode=reboot` | in v1.14 entfernt; API-Enum `REBOOT` "Deprecated: use AUTO or NO_REBOOT instead" | Talos v1.14 | Betrifft `ModeReboot` des Repos; nicht Phase 15, aber im Modus-Handling erwähnt (A5) |
| Installer von ghcr.io | Image Factory | Talos v1.14 | Keine Rauschquelle; der Upgrade-Aufruf trägt das Image |

**Deprecated/outdated:** nichts, was die Phase einsetzt.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Talos schreibt die aktive Konfiguration nicht selbst um (Doku schweigt, kein Gegenbeispiel gefunden; kein Echt-Knoten-Test möglich) | Befund 2, D-03 | Fehlalarme; Abhilfe: Pfad in eine Ausnahmeliste, Ein-Zeilen-Änderung |
| A2 | Ein echter Knoten liefert nach einem Apply semantisch das Gesendete zurück (der Simulator liefert bytegleich); Rückfall: bei `no-reboot` Lesung unbedingt übernehmen | Muster 3 | Fehl-Drift nach `reboot`/`auto`/`staged`-Applies; Betreiber bestätigt, nichts wird verborgen |
| A3 | `certwatch` ist in Produktion vom Deadline-Fehler betroffen (am Simulator gemessen, Journal des Pi nicht gelesen) | Befund 5 | Wenn doch ein Deadline gesetzt wird (nicht gefunden), entfällt Open Question 5 |
| A4 | Echte Konfigurationen liegen im zweistelligen KB-Bereich | Befund 3 | Größe/Deckel anpassen |
| A5 | `ModeReboot` (`REBOOT` deprecated) funktioniert auf v1.14-Knoten wie bisher | State of the Art | Betrifft das Apply-Feature, nicht die Drift; hier nur vermerkt |
| A6 | Takt 3 h, Fenster 15 min für erwartete Fassung nach `no-reboot`/`reboot`/`auto` | Befund 5, Muster 2 | Gestaltungsfragen (Ermessen), leicht änderbar |
| A7 | Felder wie `machine.files`, `env`, `inlineManifests` tragen beim Betreiber ggf. Geheimnisse und landen im Schnappschuss | Befund 1 | Datenschutz der Basis-Dateien; Dateirechte 0600 und derselbe Inhalt wie die Config-Ansicht mindern es |
| A8 | `gendata.VersionTag` ist die richtige Quelle für das Feld `machinery` (so benennt es CLAUDE.md, Datei nicht geöffnet) | Befund 3 | Anderes Feld nutzen |
| A9 | `provision/job.go`-`Record` ist der passende Haken für das Löschen der Basis; Signatur nicht geprüft | Befund 3 | Anderer Haken |

## Open Questions (RESOLVED)

Alle sind **Auswahlen für den Betreiber** (Empfehlung zuerst). Der Orchestrator stellt sie über das Auswahlwerkzeug.

1. **Schema-Version und Rückrollen (Format der Basis ist teuer, D-01)**
   - A) **Migration `6 → 7` (Empfehlung, Konvention)**: neuer Ordner `config-baselines/` per Migration, Fixture `version-6/`, Test. Kosten: ein vom Update-Timer zurückgerolltes Binary lehnt das Verzeichnis ab (`ErrVersionTooNew`), Wiederherstellung nur aus dem Pre-Migration-Tarball. Nutzen: einheitlich, Layout versioniert.
   - B) **Keine Versionserhöhung**, `Open` legt den Ordner an (`MkdirAll` tut das für alle Entity-Ordner). Kosten: weicht von der Konvention ab (kein Test erzwingt sie). Nutzen: das automatische Rückrollen funktioniert weiter, ein altes Binary ignoriert den Ordner.
   - Fragen: wie oft rollt der Update-Timer in der Praxis zurück? Das ist Betreiberwissen.
   - **RESOLVED:** Option A, Migration 6 auf 7 (CONTEXT D-08); die Folge für ein zurückgerolltes Binary steht im Leitfaden (Plan 15-02).
2. **Mehrdokument-Vergleich**
   - A) **Neue Funktion `CompareDocuments` (Empfehlung)**, `Compare` bleibt. Kosten: zwei Vergleichsfunktionen, `Plan` bleibt blind für Nicht-Erst-Dokumente. Nutzen: kein Eingriff in `Plan`/`ModeFor`.
   - B) **`Compare` selbst mehrdokumentfähig machen.** Nutzen: behebt den Vorbefund in `Plan` (ein Patch auf `KubeletConfig` zeigt heute einen leeren Diff). Kosten: `ModeFor` bekommt unbekannte Pfade (konservativ: unbekannt = Reboot), bestehende Tests ändern sich; größerer Radius für eine Drift-Phase.
   - Unabhängig davon sollte der Vorbefund als eigene Aufgabe vermerkt werden.
   - **RESOLVED:** Option A, neue Funktion `CompareDocuments`, `Compare` bleibt (CONTEXT D-09); der Vorbefund ist ein offener Ledger-Eintrag (Plan 15-10).
3. **Wie eigene Applies die Basis führen**
   - A) **Erwarteter Digest (Empfehlung)**: einheitlich für alle Modi, Race entfällt, überlebt Neustart. Kosten: ein Feld im Datensatz (`expected`), Annahme A2 (Knoten liefert Gesendetes) ungeprüft; Fehlschlag zeigt Drift statt zu verbergen.
   - B) **Nur "nach Apply neu lesen"**: einfacher, kein Zusatzfeld. Kosten: falsch für `staged`, `try`, `reboot`, `auto`; braucht Sonderfälle und In-Flight-Markierung.
   - **RESOLVED:** Option A, erwarteter Digest am Engpass (CONTEXT D-10); Annahme A2 bleibt ungeprüft und steht im Ledger (Plan 15-10). Eine offene Drift überlebt die Landung eines eigenen Applys (CONTEXT D-13).
4. **Sudo für "so ist es gewollt"**
   - A) **`Destructive: true` (Empfehlung)**: überschreibt den Beleg und schaltet das Banner ab; Präzedenz `backup-schedule`, `wall-link.*`. Kosten: ein Klick mehr.
   - B) **Nicht `Destructive`**, nur Admin und Audit. Nutzen: weniger Reibung; Kosten: eine offene Admin-Sitzung kann ein sicherheitsrelevantes Banner stillschweigend wegbestätigen.
   - **RESOLVED:** Option A, `Destructive` (CONTEXT D-11).
5. **`certwatch` ohne Deadline (außerhalb von Phase 15, aber gleicher Fehler)**
   - A) **Eigene Quick-Aufgabe vor Phase 15 (Empfehlung)**: Budget im `Read`, Wächter auf `context.Background()`; vorher im Journal des Pi nach `certificate watch: read a node` suchen (das kann nur der Betreiber).
   - B) **Als Aufgabe 0 in den Plan von Phase 15 aufnehmen.** Nutzen: ein Durchgang; Kosten: vermischt zwei Themen.
   - C) Nicht anfassen. Kosten: ein ausgeliefertes Feature (v0.6.0) bleibt möglicherweise wirkungslos.
   - **RESOLVED:** eigene Quick-Aufgabe vor Phase 15 (Quick-Task 261014-gl2, `Deps.ReadBudget` in `internal/certwatch`); der neue Leser hat sein eigenes Budget (Plan 15-03).
6. **D-03 Ausnahmeliste**
   - A) **Keinen Mechanismus bauen (Empfehlung)**: Messung und Dokumentation sagen "kein Rauschpfad"; ein Pfad wird im Fehlalarmfall in einer Zeile ergänzt.
   - B) **Leere, typisierte Liste mit Test "jeder Eintrag hat Beleg"**: Gerüst steht bereit; Kosten: toter Code.
   - **RESOLVED:** Option A, kein Mechanismus (CONTEXT D-12, vom Betreiber bestätigt). Die rohen, nicht redigierten Felder (Annahme A7) stehen so im Schnappschuss, wie die Konfigurationsansicht sie zeigt (CONTEXT D-14).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Backend, Tests | ✓ | 1.27.2 lokal (go.mod: `toolchain go1.26.9`) | `GOTOOLCHAIN=go1.26.9` wie in der Vorgabe |
| Node / npm | Web-Tests, Build | ✓ | v26.11.1 / 11.20.0 (CI nutzt 22) | — |
| `vitest` (jsdom-Projekt) | Web-Tests | ✓ mit Einschränkung | Lauf nur mit `NODE_OPTIONS=--no-experimental-webstorage npx vitest run --project jsdom <datei>` (Node 26 + jsdom: sonst `localStorage.clear` schlägt in allen Tests fehl; gemessen: 6/6 grün mit Flag); Browser-Projekt braucht Browser-Binary | in CI (Node 22) |
| `task` / `./bin/task ci` | Gesamtgate | ✗ | — | `go test ./...`, `go vet`, Linter über `go run` |
| `golangci-lint` v2.13.1 | Lint | per `go run` (laut Vorgabe) | nicht ausgeführt | — |
| `go test -race` | Nebenläufigkeit | ✓ auf dem Mac (nicht auf dem Pi, nur CI) | nicht ausgeführt | CI |
| Pi / echter Cluster | echtes Knotenverhalten (A1, A2), Journal (A3) | ✗ | — | `[ASSUMED]` markieren; Betreiber prüft |

**Missing dependencies with no fallback:** Echt-Knoten-Verhalten (A1, A2) und das Journal des Pi (A3); beides kann nur der Betreiber liefern.
**Missing dependencies with fallback:** `task` (direkte Go-/npm-Befehle).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` (Backend, `talossim` als Knoten), Vitest 4.1.11 + Testing Library (Web) |
| Config file | `web/vite.config.ts` (Projekte `jsdom`, `browser`); Go: keine |
| Quick run command | `go test ./internal/configdrift/ ./internal/machineconfig/ ./internal/store/... ./internal/metrics/ -count=1` |
| Full suite command | `go test ./... -count=1` (plus `cd web && NODE_OPTIONS=--no-experimental-webstorage npx vitest run --project jsdom`) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DRIFT-01 | erste Lesung legt Basis an, Zustand `baseline-created`, kein Drift | unit (Fake-Reader + fsstore im Tempdir) | `go test ./internal/configdrift -run TestFirstSight` | ❌ Wave 0 |
| DRIFT-02 | Schnappschuss ohne Geheimnis; beschädigte Datei ≠ first-sight; Redact-Fehler speichert nichts | unit | `go test ./internal/configdrift ./internal/machineconfig -run 'Snapshot\|Damaged'` | ❌ Wave 0 |
| DRIFT-03 | Lauf auf Kontext ohne Deadline liest; Fehlschlag behält Lesung; nie gelesen = `not-read` | unit + talossim | `go test ./internal/configdrift -run 'NoDeadline\|KeepsLast\|NotRead'` | ❌ Wave 0 |
| DRIFT-04 | Apply über alle drei Pfade (Service.Apply, Kubernetes-Upgrade, CA-Rotation) zeigt keine Drift; `try` verschiebt die Basis nicht; `staged` landet ohne Fehl-Drift | integration (talossim + Fake für staged) | `go test ./internal/configdrift ./internal/upgrade ./internal/rotateca -run Drift` | ❌ Wave 0 |
| DRIFT-05 | Änderung in Nicht-Erst-Dokument erscheint im Diff | unit | `go test ./internal/machineconfig -run CompareDocuments` | ❌ Wave 0 |
| DRIFT-06 | `ClusterView.config_drift`; Metriken nur mit Lesung | unit | `go test ./internal/inventory ./internal/metrics -run Drift` | ❌ Wave 0 |
| DRIFT-07 | Banner nach 24 h; `DriftSince` überlebt Neustart (neuer Service auf demselben Verzeichnis) | unit | `go test ./internal/configdrift -run 'Banner\|Restart'` | ❌ Wave 0 |
| DRIFT-08 | Confirm: Rolle, Sudo (428 bei geschlossenem Fenster), Audit-Eintrag mit Knoten und Digest, Digest-CAS 409, kein Talos-Aufruf | HTTP-Test (`httpapi_test`-Harness) | `go test ./internal/httpapi -run DriftConfirm` | ❌ Wave 0 |
| (alle) | Routen in `noUpstream`; Action in Allowlist | bestehende Wächter | `go test ./cmd/holzkube-managerd -run 'Budget\|Allowlist'` | ✅ bestehend |
| (Web) | Karte "not read yet"/Drift, Banner nur ab Server-Flag, Knotenseite mit Diff, Bestätigen | vitest | `NODE_OPTIONS=--no-experimental-webstorage npx vitest run --project jsdom src/components/NodeConfigDrift.test.tsx` | ❌ Wave 0 |

### Injizierbare Fehler (rote Läufe für den Plan)

Jede Zeile: der Fehler, der **wirklich eingesetzt** werden muss, und der Test, der dabei rot werden muss. Ein grüner Lauf nach einer Injektion, die nichts ändert, zählt nicht.

| Kernaussage | Injektion | Erwartet rot |
|-------------|-----------|--------------|
| Nie ein stilles "keine Drift" bei Lesefehler | Leserfehler wie `ok` behandeln (Fehlerzweig gibt `StateOK`) | `TestAFailedReadIsNeverNoDrift`, Zustand muss `unreadable` und `ReadNodes` kleiner sein |
| Gespeicherte Drift überlebt Lesefehler | im Fehlerzweig `DriftSince` löschen | Test "Drift bleibt bei unlesbarem Knoten" |
| Basis wird nicht bei jeder Lesung überschrieben | Basis in jedem Lauf neu schreiben | `TestADriftingNodeStaysDrifting` |
| Kein Geheimnis in der Datei | Schnappschuss aus `raw` statt aus `Redact(raw)` bilden | `TestNoSecretReachesTheBaselineFile` (liest die Datei vom Tempdir-fsstore, sucht die echten Secrets aus `talossim.Cluster.Secrets` wie `redact_test.go`; ohne Fixture-Prüfung "enthält das Roh-Dokument sie") |
| Redact-Fehler speichert nichts | Fehler von `Redact` ignorieren, Teil-Bytes speichern | Test mit zu neuem Kind (`unknownKindPattern`-Muster, `inventory/multidoc_test.go`) |
| Nicht-Erst-Dokument zählt | `CompareDocuments` auf Dokument 1 beschränken (oder `Compare` verwenden) | Test mit geänderter `KubeletConfig.image` |
| Dokumentreihenfolge ist kein Drift | Normalisierung (Sortierung) entfernen | Test "vertauschte Dokumente sind ok" |
| Eigener Apply verschiebt die Basis | Haken nicht aufrufen (oder in einem der drei Pfade umgehen) | je Pfad ein Test; **alle drei** rot einzeln |
| `try` verschiebt die Basis nicht | `try` wie `no-reboot` behandeln | Test `try` ändert die Basis nicht |
| `staged` landet ohne Fehl-Drift, davor kein Drift | `Expected` beim Staged-Apply nicht setzen | Test mit Fake-Knoten (Wave 0: der Simulator übernimmt Staged sofort, gemessen) |
| Fehlgeschlagener Apply lässt keinen `Expected` zurück | `After(err)` ohne Aufräumen | Test "abgelehnter Apply, danach echte Änderung = Drift" |
| Digest-CAS beim Bestätigen | Digest-Prüfung entfernen | `TestConfirmRefusesAChangedReading` (409) |
| Bestätigen ändert nichts am Knoten | Confirm ruft Talos | Test mit zählendem Simulator (`sim`-Anfragezähler wie `scenario.go`), Zähler muss 0 sein |
| Audit nennt den Knoten, Body kann ihn nicht bestimmen | Parameter aus dem Body statt aus dem Pfad | Test mit Body `{"machine":"anderer"}` |
| Banner erst nach 24 h, nicht vorher | Schwelle 0 | Test bei 23 h 59 min ohne, bei 24 h mit Banner |
| `DriftSince` überlebt Neustart | `DriftSince` nur im Speicher halten | Test: Service neu bauen auf demselben fsstore |
| Leser ohne Deadline | `WithTimeout` im Leser entfernen | `TestAReaderOnALifetimeWithNoDeadlineStillReadsItsNode` (Kontext ohne Deadline) |
| Metrik 0 nur mit Lesung | Serie für jeden Knoten schreiben | `metrics_test`: nie gelesener Knoten hat keine `holzkube_node_config_drift`-Serie |
| Verwaiste Basis wird geräumt | Abgleich im Tick entfernen | Test: Maschine gelöscht, Basisdatei verschwindet nach `Tick` |
| Wand unberührt | — (Beobachtung) | Wand-JSON enthält kein `config_drift` (bestehender `wall_certificates_test`-Stil) |

### Sampling Rate
- **Per task commit:** `go test ./internal/configdrift/ ./internal/machineconfig/ -count=1` (Sekunden)
- **Per wave merge:** `go test ./... -count=1` und `go vet ./...`, Web: `vitest --project jsdom`
- **Phase gate:** `go test ./... -count=1` grün, Linter über `go run ...golangci-lint@v2.13.1 run ./...` grün; `-race` und `task ci` nur in CI (hier nicht möglich, im Bericht sagen)

### Wave 0 Gaps
- [ ] `internal/configdrift/*_test.go` mit Fake-Deps (wie `certwatch_test.go`) und fsstore im Tempdir
- [ ] Simulator-Erweiterung oder Fake für **echte Staged-Semantik** (aktiv bleibt alt, `persistent` voraus) und für `try` (Rücknahme nach Zeit); `talossim/machine.go:857-877` setzt heute jede Konfiguration sofort
- [ ] Fixture mit zwei Dokumenten, die sich unabhängig ändern lassen (`talossim.NewCluster(...)` liefert 28)
- [ ] `internal/store/fsstore`-Tests für den neuen Entity (Muster `fsstore_test.go`), bei Option A in Frage 1: `version-6/`-Fixture und Test in `migrate_test.go`
- [ ] Web: Test-Dateien für die neuen Komponenten; `web/fixtures/demo.json` um Drift-Daten ergänzen (nur Dokumentationswerte)

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | nein (bestehende Sitzung) | `RequiresSession` |
| V3 Session Management | nein | bestehend; Sudo-Fenster für `Destructive` |
| V4 Access Control | **ja** | `MinRole`: Operator für den Diff (Konfigurationsinhalt), Admin für Bestätigen; Cluster-Liste (Reader) ohne Diff-Inhalt |
| V5 Input Validation | **ja** | `digest` als 64 Hex-Zeichen validieren; Maschinen-ID über `safeKey`/Inventory-Prüfung; Konfigurationsgröße deckeln |
| V6 Cryptography | gering | nur `crypto/sha256` der Stdlib für Digests; keine eigene Kryptografie |
| V7 Logging | **ja** | Audit `config.drift-confirm` mit Knoten und Digest; Logzeilen nur bei Übergängen, ohne Konfigurationsinhalt |
| V8 Data Protection | **ja** | Basis redigiert (Redact + `ContainsPrivateKey`), 0600/0700 über `Guard`; Hinweis auf nicht redigierte Felder (`files`, `env`, `inlineManifests`) |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Geheimnis im Basis-Datei-Inhalt | Information Disclosure | `Redact` + PEM-Sweep + `ContainsPrivateKey` vor dem Schreiben; roter Lauf "Schnappschuss aus Roh-Bytes" |
| Diff-Inhalt für Reader sichtbar | Information Disclosure | Cluster-Liste ohne Pfade/Werte; Diff-Route `RoleOperator` |
| Basis ohne Knotenspur bestätigt | Repudiation | Audit mit serverseitigem Knoten-Parameter und Digest |
| Unbemerktes Mitbestätigen eines dritten Eingriffs | Tampering | Digest-CAS beim Bestätigen |
| Offene Admin-Sitzung bestätigt ein Banner weg | Elevation/Tampering | `Destructive` (Sudo-Fenster), Open Question 4 |
| Entarteter Knoten liefert riesige Konfiguration | Denial of Service | Größen-Deckel, Lesebudget, seriell |
| Manipulierte Basis-Datei | Tampering | Dateirechte 0600 via `Guard`; `Digest != sha256(Snapshot)` ⇒ `baseline-damaged` |
| Echte Werte im öffentlichen Repo | Information Disclosure | nur Dokumentationswerte in Fixtures; `internal/publicrepo` |

## Sources

### Primary (HIGH confidence)
- Repo-Code gelesen: `internal/machineconfig/{service,redact,diff,applymode}.go`, `internal/certwatch/certwatch.go`, `internal/talos/{facts,client,deadline,certs}.go`, `internal/store/{store.go,fsstore/atomic.go,fsstore/fsstore.go,fsstore/inventory.go,fsstore/permissions.go,migrate/migrate.go,migrate/backup/*.go}`, `internal/httpapi/{router.go,handlers/config.go,handlers/backups.go,handlers/wall_certificates.go,middleware/audit.go}`, `internal/audit/{redact.go,record.go}`, `internal/inventory/{view.go,mutate.go,lookup.go}`, `internal/metrics/metrics.go`, `internal/talossim/{cosi.go,cluster.go,machine.go}`, `internal/upgrade/job.go`, `internal/rotateca/job.go`, `internal/provision/job.go`, `cmd/holzkube-managerd/{main.go,allowlist_test.go,budget_test.go}`, `web/src/{components/CertificateBanner.tsx,components/NodeCertificates.tsx,routes/clusters.tsx,api.ts}`
- Machinery v1.14.2 im Modulcache: `resources/config/machine_config.go`, `config/container/container.go`, `config/configloader/configloader.go`, `config/types/v1alpha1/v1alpha1_provider.go`, `api/machine/machine.pb.go`
- Messungen am Simulator und an Machinery (temporäre Tests und zwei Scratch-Module, gelöscht)
- `.planning/WINDOWS.md` Ledger 137 (Heartbeat ohne Deadline)

### Secondary (MEDIUM confidence)
- https://docs.siderolabs.com/talos/v1.14/configure-your-talos-cluster/system-configuration/editing-machine-configuration (Apply-Modi, Staged ändert die laufende Konfiguration nicht; `--mode=reboot` in 1.14 entfernt)
- https://docs.siderolabs.com/talos/v1.14/configure-your-talos-cluster/lifecycle-management/upgrading-talos (Upgrade-Aufruf trägt das Installer-Image; "The supported configuration may change between versions")

### Tertiary (LOW confidence)
- Drittanbieter-Blog (oneuptime.com, März 2026) zur Unverändertheit von `machine.install.image` bei `talosctl upgrade`: nur Hinweis, nicht als Beleg verwendet.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, keine neuen Pakete, alles im Repo gelesen
- Architecture: HIGH für Repo-Integration und Persistenz; MEDIUM für das Apply-Verhalten echter Knoten (A1, A2)
- Pitfalls: HIGH für die gemessenen (Compare-Blindheit, Deadline, Redact-Verhalten, Reihenfolge), MEDIUM für Staged/Reboot-Randfälle (Simulator kann sie nicht zeigen)

**Research date:** 2026-10-14
**Valid until:** 2026-11-13 (Machinery-Pin und Talos-Releases bewegen sich schnell; bei einer Anhebung des Pins die Messung "Encoding v1.13.9 = v1.14.2" und die Dokumentzahl wiederholen)
