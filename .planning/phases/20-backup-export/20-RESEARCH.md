# Phase 20: Backup-Export - Research

**Researched:** 2026-10-16
**Domain:** Go-Daemon (etcd-Snapshots zu einem S3-kompatiblen Ziel außerhalb des Geräts exportieren), fsstore-Persistenz mit Geheimnis, httpapi-Routen mit Sudo/Audit, React-Anzeige, Prometheus-Serien
**Confidence:** HIGH für alles, was im Code gelesen oder gegen echte S3-Server gemessen wurde; MEDIUM für Verhalten einzelner Anbieter (AWS, R2, B2), die von hier nicht erreichbar sind
**Umgebung dieser Session:** macOS, arm64 (Mac des Entwicklers), Go 1.27.2 lokal (`go.mod` verlangt `toolchain go1.26.9`), kein Pi, kein Cluster, kein `task`. Docker Desktop war da: SeaweedFS und RustFS liefen als echte S3-kompatible Server in Containern (Container und Images danach wieder entfernt). Alle Messprogramme lagen im Scratchpad außerhalb des Repos; das Repo ist unverändert (nur diese Datei ist neu).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

Alle als Auswahl vom Betreiber am 2026-10-16 getroffen, sofern nicht als Ermessen von Claude gekennzeichnet.

- **D-01:** Das erste Ziel ist **S3-kompatibel** (Bucket mit Zugangsschlüssel, z. B. MinIO im LAN oder ein Cloud-Anbieter). **SFTP folgt später** als eigene Phase; diese Phase baut eine **Austauschnaht** (Schnittstelle „Ziel"), damit das zweite Ziel wenig kostet. — **Reversibility:** costly — Schnittstelle und Einstellungsformat sind veröffentlicht.
- **D-02:** Der Zugangsschlüssel ist ein **Geheimnis** im Datenverzeichnis (nie im Audit-Klartext, nie in Logs, nie in einer Antwort).

### Claude's Discretion
Eigener schmaler S3-Client oder eine Bibliothek (die Abhängigkeit ist zu begründen und gegen die Größe des Releases auf dem Pi abzuwägen), Signaturverfahren, Pfadstil und Region, was bei Fehlschlag passiert (Wiederholung mit Grenze, Zustand „Export fehlgeschlagen" sichtbar, nie still), Aufbewahrung am Ziel (nicht löschen, oder die Aufbewahrungszahl des lokalen Zeitplans spiegeln — eine echte Alternative: dann als Auswahl zurück an den Betreiber), Verschlüsselung der Übertragung (TLS, Zertifikatsprüfung), die Einstellungsseite, ein Test-Knopf („Testobjekt schreiben"). Die Beweisgrenze: nur gegen einen eigenen Fake-Server (`httptest`), kein echter Bucket.

### Deferred Ideas (OUT OF SCOPE)
- SFTP-Ziel (eigene spätere Phase, bewusst als zweites).
- Wiederherstellung aus dem Export (Download vom Ziel in die Oberfläche).
- Verschlüsselung der Snapshots vor dem Export.
</user_constraints>

<phase_requirements>
## Phase Requirements

Es wurden keine Requirement-IDs übergeben (ROADMAP: „Requirements: TBD"). Vorschlag für den Planner (Format `^[A-Z0-9]+-\d+$`, wie `internal/planning` es verlangt):

| ID (Vorschlag) | Beschreibung | Research Support |
|----|-------------|------------------|
| EXPORT-01 | Jeder erfolgreiche Snapshot (geplant und vor Upgrade) wird nach dem Schreiben zu einem konfigurierten Ziel exportiert, mit Prüfsumme; der Export liest die Datei, hält keinen Cluster-Lease | Befund 1, 4; Muster 1 |
| EXPORT-02 | Austauschnaht `Target` (Put, Check, Fingerprint); S3 ist die erste Umsetzung, die Nahtform kennt kein HTTP | Befund 2, 8; Muster 2 |
| EXPORT-03 | Eigener schmaler S3-Client (SigV4, Stdlib), Pfadstil/Virtual-Host, Region, Prüfsummen-Header, Größengrenze 5 GB | Befund 2 |
| EXPORT-04 | TLS-Prüfung bleibt an; eigene CA per PEM für ein LAN-Ziel; nie `InsecureSkipVerify`; kein Redirect | Befund 2, Auswahl C2/C3 |
| EXPORT-05 | Ziel-Einstellungen und Geheimnis im Datenverzeichnis (getrennte Entities), schreibgeschütztes Geheimnis in der UI, Audit-Allowlist, nie in Log/Fehler/Metrik/Antwort | Befund 3 |
| EXPORT-06 | Fehlerverhalten: Wiederholung mit Grenze, Zustand je Snapshot und je Cluster, nie still; Rückstau nach Ausfall; Zielwechsel exportiert neu | Befund 4 |
| EXPORT-07 | Überfällig-Zustand des Exports (Serie, Karte, `BackupHealth`), „nicht konfiguriert" ist ein eigener Zustand und nie „ok" | Befund 4, 6 |
| EXPORT-08 | Einstellungsseite, Export-Zustand auf der Backups-Tafel, „Ziel testen", „jetzt exportieren"; api-contract, Guide, README | Befund 6 |
| EXPORT-09 | Beweis gegen einen eigenen `httptest`-Fake-S3 mit unabhängigem Signaturvektor, je Anspruch ein rot gesehener Fehler | Befund 7 |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Öffentliches Repository:** keine LAN-Adresse, kein Hostname, keine Mail, MAC, kein Home-Verzeichnis des echten Aufbaus in Tests, Fixtures, Ledger, Commit-Messages. Dokumentationswerte nutzen (`192.168.1.10`, `homeserver`, `example.com`; für S3 `minio.example.com`, Schlüssel `AKIAIOSFODNN7EXAMPLE`). `internal/publicrepo` prüft Baum und Historie.
- **README im selben Schritt wie das Feature** („What it does", Tiefe in `docs/guide.md`); Screenshot nur gerendert aus `web/fixtures/demo.json` (`task build && node web/scripts/readme-images.mjs`), nie vom echten Cluster. Das Ziel in `demo.json` heißt `minio.example.com`.
- **Changelog nur beim Release:** `internal/changelog/changelog.json` gehört zum Release, nicht zur Phase.
- **Alpha-Hinweis** bleibt; `holzkube-managerd --version` bleibt eine nackte Version.
- **Produktionsziel linux/arm64 (Raspberry Pi 5), einziges Release-Ziel.** Hier kein `go test -race` (CI), kein Pi; ein Bericht muss sagen, wo er lief. Diese Session: macOS-arm64.
- **Ersetzen der Produktions-Binary, Neustart des Dienstes, Kopieren des Datenverzeichnisses sind Sache des Betreibers.**
- **Entscheidungen immer als Auswahl** (2–4 benannte Optionen mit Kosten/Nutzen, Empfehlung zuerst, über `AskUserQuestion`). Die offenen Auswahlen stehen gesammelt in „Open Questions".
- **Jeder Wächter muss gegen den wieder eingesetzten Fehler rot gesehen werden;** eine Injektion, die nichts injiziert, ist kein Ergebnis. Die Tabelle „Injizierbare Fehler" ist dafür gedacht.
- **Linter lokal:** `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...` [CITED: 20-CONTEXT.md canonical_refs].
- **Frist-Lektion (Quick-Task 261014-gl2, Ledger 137):** Wächter und Leser laufen auf einem Kontext **ohne Frist**; jeder Außenaufruf setzt sein eigenes Budget.

## Summary

Die Funktion ist im Kern klein, aber an drei Stellen heikel. **Erstens der Aufhängepunkt:** Der geplante Snapshot ist ein Job, der den Cluster-Lease hält (`cluster.etcd-backup`); der Snapshot vor einem Upgrade ist **kein** Job, sondern geht über `TakeSafetySnapshot`. Beide laufen aber durch `SnapshotStore.writeKind`. Ein Export als zweiter Schritt im Job würde den Lease während eines Uploads über LAN oder Internet halten, den Upgrade-Snapshot nie erreichen und nach einem Ausfall nichts nachholen. Empfohlen ist deshalb ein **eigener Export-Abgleicher** (Muster `certwatch`/`configdrift`): er vergleicht, was auf der Platte liegt, mit dem, was exportiert ist, und lädt Fehlendes hoch; `writeKind` stößt ihn nach jedem erfolgreichen Schreiben nur an. Das macht Neustart, Ausfall, manuelle und Upgrade-Snapshots und Zielwechsel zu einem einzigen Fall.

**Zweitens der S3-Client:** gemessen (linux/arm64, Go 1.27.2, gestrippt, gzip -9) kostet das volle `aws-sdk-go-v2` rund +0,74 MB, `minio-go` rund +0,72 MB gegenüber einem `net/http`-Programm von 2,24 MB gz (Release-Archiv v0.8.0: 20,3 MB) und bringt 19 bzw. 20 neue Module; nur der Signer des AWS-Kerns kostet +0,07 MB bei 2 Modulen. Das Gewicht entscheidet also nicht. Entscheidend ist: der SigV4-Teil für **einen** Anwendungsfall (PUT, HEAD, GET einer Datei mit bekannter SHA-256, Pfadstil) sind rund 60 Zeilen Stdlib; ich habe ihn als Spike gegen **zwei echte S3-Server** laufen lassen (PUT, Prüfsummen-Ablehnung, falsches Geheimnis, HEAD, DELETE, alles wie erwartet) und seine Signatur gegen den AWS-Signer als unabhängiges Orakel verglichen (bit-identisch für alle Schlüssel, die diese Codebasis erzeugt). Dazu kommt ein belegter Grund gegen das SDK: seine Standardwerte für Prüfsummen wurden 2025 geändert und haben S3-kompatible Fremdserver gebrochen; MinIO selbst ist seit Oktober 2025 ohne Images und seit 2026 archiviert, das Zielfeld „S3-kompatibel" ist also gerade in Bewegung. **Empfehlung: eigener Client**, ohne neue Abhängigkeit.

**Drittens die Integrität:** Der Spike hat etwas gezeigt, das kein Dokument sagt. SeaweedFS akzeptiert einen **falschen** `x-amz-content-sha256` mit 200, prüft aber `x-amz-checksum-sha256`; RustFS prüft beides. Ein Ziel kann den Prüfsummen-Header auch ganz ignorieren. „Hochgeladen" heißt daher nicht „geprüft": der Export-Zustand trägt ein Feld `verified` (`response` = das Ziel hat unsere SHA-256 zurückgegeben, `head` = erst ein HEAD, `size` = nur die Größe stimmt, `none`), und die Oberfläche sagt es. Weitere Befunde mit Folgen für den Plan: eine Datei bis 5 GB geht als ein PUT (etcd-Quota ist 2 GiB, empfohlenes Maximum 8 GiB), Multipart ist für diese Phase nicht nötig, größere Snapshots werden **sichtbar abgelehnt**; die Uhr des Pi entscheidet über SigV4 (`RequestTimeTooSkewed`); S3-Fehlerkörper werden nie weitergereicht; das Ziel-Geheimnis gehört in eine eigene Entity wie `cluster-secrets`, und ob dafür das Schema auf 8 steigt, ist eine echte Auswahl (Rückroll-Falle gegen stilles Aufhören).

**Primary recommendation:** neues Paket `internal/backupexport` mit Schnittstelle `Target` und einem Abgleicher, der auf `context.Background()` ohne Frist läuft und je Aufruf sein eigenes Budget setzt; Unterpaket `internal/backupexport/s3` mit eigenem Stdlib-SigV4-Client (Pfadstil Standard, TLS-Prüfung immer an, optionale CA als PEM, Redirects verweigert); Zustand als Sidecar je Snapshot neben `.sha256` plus `export-status.json` je Cluster; Ziel und Geheimnis als zwei fsstore-Entities; Überfällig wie `healthOf`, aber gemessen am letzten Export-Erfolg.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Snapshot nehmen, ablegen, prüfsummen, aufräumen | API/Backend (`internal/upgrade`, unverändert) | Database/Storage (Snapshot-Verzeichnis im Datenverzeichnis) | Besteht seit v0.5.0; der Export ändert nur den Anstoß am Ende von `writeKind` |
| Abgleich „liegt lokal" gegen „ist exportiert", Wiederholung, Zustand | API/Backend (`internal/backupexport`) | Storage (Sidecars im Snapshot-Verzeichnis) | Level-getriggerter Leser nach Muster `configdrift`; kein Cluster-Zugriff, kein Lease |
| S3-Protokoll (Signatur, PUT/HEAD, Fehlerabbildung) | API/Backend (`internal/backupexport/s3`) | — | Reiner Außenaufruf; hinter `Target` versteckt, damit SFTP nichts davon erbt |
| Ziel-Einstellung und Geheimnis | Database/Storage (`internal/store/fsstore`, zwei Entities) | API (Handler) | Pfade nur in fsstore (`TestNoDirectFileAccessOutsideFsstore`); Geheimnis getrennt wie `cluster-secrets` |
| Überfällig, Zusammenfassung je Cluster | API/Backend (`BackupHealth.Export`) | Metrics, Karte | Abgeleitet, nie gespeichert (wie `healthOf`) |
| Einstellungsmaske, Export-Spalte, Test-Knopf | Browser (React) | — | Nur Darstellung; Sätze und Zustände kommen vom Server |
| Zertifikatsvertrauen des Ziels | API/Backend (`x509.CertPool` im Client) | — | Vertrauen wird im Prozess gebaut; die Oberfläche liefert nur das PEM |

## Standard Stack

### Core

Keine neuen externen Pakete empfohlen. Alles Nötige ist Stdlib oder schon im Repo.

| Baustein | Fundort | Zweck |
|---------|---------|-------|
| `crypto/hmac`, `crypto/sha256`, `crypto/tls`, `crypto/x509`, `net/http` | Go-Stdlib | SigV4, Transport, Vertrauensanker (CA-PEM) |
| `internal/store/fsstore` (`recordStore[K,T]`, `WriteStreamAtomic`, `ReadFile`, `OpenFile`, `ListDir`, `RemoveFile`, `FreeBytes`) | `internal/store/fsstore/inventory.go`, `stream.go` | Entities und Sidecars; atomar, 0600 |
| `internal/upgrade` (`SnapshotStore.List/Open`, `FileOps`) | `internal/upgrade/backups.go`, `safetysnapshot.go` | Quelle der Snapshots und ihrer SHA-256 |
| `@tanstack/react-query`, shadcn-`Card`/`Badge`/`Button`/`Input`/`Select`, `zod` | `web/src/components/Backups.tsx`, `web/src/routes/settings.tsx` | Anzeige |

### Alternatives Considered

Gemessen in einem Scratch-Modul (kein Repo-Eintrag): `go build -trimpath -ldflags="-s -w"`, `CGO_ENABLED=0 GOOS=linux GOARCH=arm64`, Go 1.27.2, Programme rufen je einmal den Client auf.

| Variante | Neue Module | Binary roh | Binary gz -9 | Delta gz zu `net/http`-Basis | Bewertung |
|---|---|---|---|---|---|
| `net/http`-Basis (Referenz) | 0 | 5 439 648 B | 2 240 323 B | — | Referenz |
| **Eigener Client (Stdlib)** | **0** | ≈ Basis | ≈ Basis | **≈ 0** | **Empfohlen** |
| AWS-Signer allein (`aws/signer/v4`) + eigenes HTTP | 2 (`aws-sdk-go-v2`, `smithy-go`) | 5 636 256 B | 2 307 968 B | +67 645 B (+0,07 MB) | Echte Alternative, siehe Auswahl C5 |
| `aws-sdk-go-v2` (`config` + `service/s3`) | 19 | 7 602 336 B | 2 975 324 B | +735 001 B (+0,74 MB) | Nicht empfohlen |
| `minio-go/v7` v7.4.1 | 20 | 7 274 656 B | 2 956 225 B | +715 902 B (+0,72 MB) | Nicht empfohlen |

[VERIFIED: Messung dieser Session, Scratchpad `e_base`, `e_aws`, `e_minio`, `e_signer2`; Zahlen oben aus `stat` und `gzip -9 -c | wc -c`]

Das Release-Archiv `holzkube-manager_0.8.0_linux_arm64.tar.gz` ist 20 325 662 Byte groß [VERIFIED: `gh release view v0.8.0 --json assets`]; das volle SDK wäre rund 3,6 % davon. **Gewicht ist kein Ausschlussgrund, Lieferkette und Verhalten sind es:**

- **Lieferkette:** 19 bzw. 20 neue Module, jedes ein Ziel für `govulncheck` (gated), Dependabot und Pinning; der eigene Client fügt keines hinzu. `go.mod` hat heute 16 direkte Abhängigkeiten, alle bis auf `aws`/`minio` gibt es schon (`golang.org/x/net`, `x/crypto`, `x/sys`, `x/text` teilt `minio-go`). [VERIFIED: internal go.mod gelesen]
- **Verhalten:** Das SDK hat seine Standardwerte für Prüfsummen geändert (`RequestChecksumCalculation`/`ResponseChecksumValidation` von `WhenRequired` auf `WhenSupported`), was bei Drittanbietern zu `SignatureDoesNotMatch` führte; der Gegenschlag ist, beide auf `WhenRequired` zu setzen. [CITED: gitlab.com/gitlab-org/gitlab-runner/-/merge_requests/6691; docs.aws.amazon.com/sdk-for-go/v2/developer-guide/s3-checksums.html] Ein eigener Client entscheidet selbst, welche Header über die Leitung gehen; das ist bei „S3-kompatibel" mit schrumpfendem Referenzserver der wichtigere Vorteil.
- **Zielfeld in Bewegung:** MinIO stellte im Oktober 2025 Images und Binaries ein, die Repository ging im Dezember 2025 in den Wartungsmodus und wurde 2026 archiviert (Datum je nach Quelle Februar oder April). [CITED: itsfoss.com/news/minio-moves-away-from-open-source; blog.vonng.com/en/db/minio-is-dead; sekundäre Quellen, Daten widersprechen sich leicht] Das Bild vom „MinIO im LAN" ist als Beispiel richtig, aber der Client darf sich nicht an einen Server binden. Auf dieser Maschine ließ sich `quay.io/minio/minio` nicht mehr ziehen (401), `dxflrs/garage`, `rustfs/rustfs` und `chrislusf/seaweedfs` schon [VERIFIED: `docker pull` dieser Session].

**Installation:** keine.
**Version verification:** keine neuen Pakete. Die Scratch-Module zogen `aws-sdk-go-v2` v1.47.3, `service/s3` v1.114.3 und `minio-go` v7.4.1; nichts davon kommt in `go.mod`.

## Package Legitimacy Audit

Diese Phase installiert keine externen Pakete. `gsd-tools query package-legitimacy check` kennt nur npm, PyPI und crates, keine Go-Module; die Prüfung für `aws-sdk-go-v2` und `minio-go` (nur zur Messung im Scratchpad gezogen, nicht empfohlen) wurde deshalb nicht ausgeführt. Beide Paketnamen stammen aus Trainingswissen und der Websuche und sind `[ASSUMED]` als Namen; die Messung zeigt nur, dass sie auflösen.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (keine) | — | — | — | — | — | Nichts zu installieren |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

Fällt die Auswahl C5 auf den AWS-Signer, ist vor dem Einchecken ein `checkpoint:human-verify` nötig (Modul `github.com/aws/aws-sdk-go-v2` v1.47.3 plus `github.com/aws/smithy-go` v1.28.5, Messung im Scratchpad).

## Architecture Patterns

### System Architecture Diagram

```
 geplanter Snapshot (Job cluster.etcd-backup, hält den Cluster-Lease)      Snapshot vor Upgrade (TakeSafetySnapshot, kein Job)
                         \                                                   /
                          v                                                 v
                    SnapshotStore.writeKind: Datei + .sha256 atomar, prune, dann  --> onNew(cluster)   [neu: nur ein Anstoß]
                                                                                           |
        Betreiber: "jetzt exportieren" / Ziel gespeichert / Ziel getestet --> Poke --------+
                                                                                           v
                              backupexport.Service.Run (Kontext OHNE Frist, Tick 1 min + Poke), seriell über Cluster
                                                                                           |
                       je Cluster: List(snapshots) minus Sidecar "<name>.export.json" mit gleichem Ziel-Fingerprint
                                                                                           |
                  neueste zuerst, nie vor next_try_at  ------------------------------------+
                                                                                           v
                    Target.Put(ctx mit eigenem Budget)  --> s3.Client: SigV4, PUT <prefix>/<cluster>/<name>
                          |   Header: x-amz-content-sha256 = SHA-256 aus Sidecar, x-amz-checksum-sha256 = base64
                          |   danach PUT <name>.sha256 (sha256sum-Form), dann HEAD (best effort)
                          v
              Receipt{verified: response|head|size|none}  oder  Fehler -> Satz aus fester Tabelle (nie Fremdkörper)
                          |                                         |
                          v                                         v
          Sidecar state=exported (at, key, bytes, sha256,        Sidecar state=failed (attempts, reason, next_try_at)
          verified, target_fingerprint)                          + export-status.json je Cluster (letzter Versuch)
                          \                                        /
                           v                                      v
        BackupHealth.Export (abgeleitet): enabled, overdue, last_success_at, pending, failed, lost, verified
                 |                      |                    |                         |
          Backups-Tafel        Clusterkarte-Zeile      /metrics (je Cluster)     GET /api/v1/clusters/{id}/backups
                                                                         (Phase 19 liest denselben Zustand)
```

### Recommended Project Structure

```
internal/backupexport/             # Service (Run/Tick/Poke/ExportNow), Zustände, Sätze, Health
internal/backupexport/target.go    # Schnittstelle Target, Object, Receipt, Verified-Werte
internal/backupexport/s3/          # Stdlib-SigV4, Client (Put/Head/Check), Fehlerabbildung, Transport
internal/model/backupexport.go     # BackupTarget, BackupTargetSecret, ExportHealth, Format-Konstanten
internal/store/store.go + fsstore/ # BackupTargets(), BackupTargetSecrets() (zwei Entities)
internal/upgrade/                  # SnapshotStore: OnNew-Haken, Sidecar-Zugriff, prune räumt .export.json mit weg
internal/httpapi/handlers/backupexport.go   # GET/PUT/DELETE /api/v1/backup-export, POST .../test, POST .../backups/export
internal/audit/redact.go           # backup-export.configure / .remove / .test, etcd.backup-export-run
internal/metrics/metrics.go        # Export-Serien aus ExportHealth
web/src/components/BackupExport.tsx (Einstellungskarte) + Backups.tsx (Export-Spalte) + api.ts
```

### Pattern 1: Level-getriggerter Abgleicher statt Schritt im Job

**What:** `backupexport.Service` hat `Run(ctx)` (Tick jede Minute), `Poke(cluster)` (nicht blockierender Kanal) und `ExportNow(cluster)`. Pro Durchlauf je Cluster: Snapshots listen (`SnapshotStore.List`, neueste zuerst), je Snapshot den Sidecar lesen, entscheiden (exportiert zum aktuellen Ziel → überspringen; `next_try_at` in der Zukunft → überspringen; sonst hochladen). Seriell, ein Upload zugleich (der Pi hat eine SD-Karte und eine Leitung).
**When to use:** immer; er deckt geplante, manuelle und Upgrade-Snapshots, Neustart, Ausfall und Zielwechsel mit demselben Code ab.
**Warum nicht im Job:** `backupStep()` läuft unter dem Cluster-Lease; `SubmitBackup` meldet `jobs.ErrClusterBusy`, sobald etwas anderes den Cluster hält [VERIFIED: internal/upgrade/backups.go:496-528], und ein Upload von 2 GiB über eine DSL-Leitung dauert Stunden. Ein Upgrade würde so von einem Upload blockiert. Der Upgrade-Snapshot ginge nie durch `backupStep` (`TakeSafetySnapshot` ruft `s.snapshots.write(...)` [VERIFIED: internal/upgrade/safetysnapshot.go:396]).
**Beispiel (Anstoß, Skizze):**
```go
// internal/upgrade/safetysnapshot.go: am Ende von writeKind, nach prune und vor dem Return.
// Quelle: eigener Entwurf; writeKind steht in safetysnapshot.go:271-329.
if st.onNew != nil {
    st.onNew(cluster) // nicht blockierend; der Abgleicher entscheidet, was zu tun ist
}
```

### Pattern 2: Die Naht `Target` kennt kein HTTP

**What:** Die Schnittstelle gehört `internal/backupexport`; `s3.Client` implementiert sie. SFTP braucht später dieselben fünf Dinge.
```go
// Source: eigener Entwurf, geprüft gegen die Anforderungen aus 20-CONTEXT.md D-01.
type Object struct {
    Key    string    // "<prefix>/<cluster>/<name>", nur [A-Za-z0-9._-/] (siehe Pitfall 4)
    Size   int64
    SHA256 string    // hex, kleingeschrieben; leer nur, wenn der Aufrufer ihn erst berechnet
    Body   io.Reader
}

type Receipt struct {
    Bytes    int64
    Verified string // "response" | "head" | "size" | "none"
}

type Target interface {
    Kind() string                                  // "s3"; später "sftp"
    Fingerprint() string                           // identifiziert das Ziel ohne Geheimnis (sha256 über kind|endpoint|bucket|prefix|region)
    Put(ctx context.Context, o Object) (Receipt, error)
    Check(ctx context.Context) error               // "Testobjekt schreiben"
}
```
Eingeführte Werte für `Verified` sind Teil des veröffentlichten Formats (D-01 `costly`): sie stehen im Sidecar und in der API.

### Pattern 3: Zustand als Sidecar neben dem Snapshot

**What:** Je Snapshot eine Datei `<name>.export.json` im Snapshot-Verzeichnis, wie die Prüfsumme als `<name>.sha256` dort liegt (`checksumSuffix = ".sha256"` [VERIFIED: internal/upgrade/safetysnapshot.go:331]); je Cluster eine `export-status.json` wie `backup-status.json` (`statusFile = "backup-status.json"` [VERIFIED: internal/upgrade/backups.go:205]). `prune` entfernt die Sidecars mit dem Snapshot (heute entfernt es `names[0]` und `names[0]+checksumSuffix` [VERIFIED: internal/upgrade/safetysnapshot.go:357-358]).
**Warum nicht in einer Entity:** Der Snapshot und sein Export-Zustand haben dieselbe Lebensdauer; eine zentrale Datei müsste beim Aufräumen nachgeführt werden und könnte verwaisen. Der Aufwand eines weiteren Entity-Ordners ist größer als der Nutzen.
**Format (veröffentlicht, `costly`):**
```json
{"format":1,"state":"exported","at":"2026-10-16T12:05:30Z","target":"<fingerprint>","key":"holzkube-manager/c1/20261016T120000.000Z.scheduled.snapshot","bytes":20971520,"sha256":"…","verified":"response"}
{"format":1,"state":"failed","attempts":3,"last_attempt_at":"…","next_try_at":"…","reason":"the target refused the key (HTTP 403)","target":"<fingerprint>"}
```
Ein Sidecar mit unbekanntem `format` oder beschädigtem JSON ist **nie** „exportiert" und nie „noch nichts": er zählt als `failed` mit dem Satz „the export record could not be read" und wird beim nächsten Erfolg ersetzt (Gegenstück zu Pitfall 5 der Phase 15).

### Pattern 4: Frist-Lektion

**What:** `Run` startet in `main.go` auf `context.WithCancel(context.Background())` wie `backupCtx` [VERIFIED: cmd/holzkube-managerd/main.go:863-866]. Der Abgleicher setzt je Aufruf `context.WithTimeout(ctx, budget)`. Der S3-Client verweigert wie `imagefactory.requireDeadline` jeden Aufruf ohne Frist (`ErrNoDeadline`), und zwar an der **einen** Stelle, an der `http.Client.Do` aufgerufen wird. [Muster: internal/imagefactory/client.go:262-288, gelesen; `ErrNoDeadline = errors.New("imagefactory: refusing a request with no deadline")`]
**Budgets (Empfehlung, Gestaltungsfrage):** kleine Aufrufe (HEAD, Sidecar-PUT, Test) 30 s; Snapshot-PUT `2 min + Size / 1 MiB/s`, gedeckelt bei 6 h; Transport: `DialContext` 10 s, `TLSHandshakeTimeout` 10 s, `ResponseHeaderTimeout` 60 s (nach dem Body), **kein** `http.Client.Timeout`. Die Testroute bekommt eine Zeile in der Budget-Tabelle von `cmd/holzkube-managerd/budget_test.go` (`TestEveryRouteThatReachesUpstreamHasABudgetRow`, `writeTimeout = 130 * time.Second` [VERIFIED: cmd/holzkube-managerd/main.go:87]); 45 s Routenbudget passt.

### Anti-Patterns to Avoid

- **Export als Schritt im Backup-Job:** hält den Lease, erreicht den Upgrade-Snapshot nicht, holt nach Ausfall nichts nach (siehe Muster 1).
- **`InsecureSkipVerify`:** nie, auch nicht „nur für LAN". Der Vertrauensanker ist eine PEM-CA (Befund 2). Die vorhandenen `InsecureSkipVerify`-Stellen gehören zur Maintenance-Mode-Sonderlage [VERIFIED: internal/talos/dial_direct.go:118] und sind kein Vorbild.
- **Fremdkörper weiterreichen:** ein S3-Fehlerkörper darf weder in `reason` noch ins Log noch in die Antwort (siehe Pitfall 3).
- **„Exportiert" ohne Prüfung des Ziels markieren:** nur nach 2xx für **beide** Objekte, und `verified` ehrlich setzen.
- **Rückstau ignorieren oder alles gleichzeitig laden:** seriell, neueste zuerst (Befund 4).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Atomare Dateien, 0600, Rev-CAS für Ziel und Geheimnis | eigene Datei-I/O im neuen Paket | `fsstore.recordStore` und `WriteStreamAtomic` | Pfade nur in fsstore (`TestNoDirectFileAccessOutsideFsstore`) |
| Vertrauensanker für ein selbst signiertes LAN-Ziel | `InsecureSkipVerify` oder eigene Zertifikatsprüfung | `x509.NewCertPool` + `AppendCertsFromPEM` (Stdlib), `tls.Config.RootCAs` | Prüfung bleibt Stdlib; nur der Anker ist konfigurierbar |
| SHA-256 des Snapshots | zweite Prüfsummenlogik | die Sidecar-SHA-256 aus `StoredSnapshot.SHA256` (und `sha256.New()` nur, wenn sie fehlt) | Der Snapshot ist unveränderlich; die Prüfsumme wurde beim Schreiben mitgerechnet (`io.MultiWriter(w, sum)` [VERIFIED: internal/upgrade/safetysnapshot.go:297-300]) |
| Zeit, Frist, „einmal gesetzter Beginn" | eigene Uhrenlogik | Muster `nextDue`/`retryGap` und `healthOf` aus `backups.go` | Überfällig und Wiederholabstand haben dort schon eine bewiesene Form |
| Redaktion von Geheimnissen in Logs | `strings.ReplaceAll` an jeder Fehlerstelle | ein Typ `Secret` mit `String()`, `GoString()`, `MarshalText()` und `LogValue()` (`slog.LogValuer`), der `<redacted>` liefert | Ein Typ, der nicht ausgeben kann, ist ein Wächter, der nicht vergessen werden kann |
| Audit-Redaktion | Feldliste verbotener Felder | die vorhandene Allowlist `internal/audit/redact.go` | Failt geschlossen; `TestEveryAuditedActionIsInTheAllowlist` erzwingt die Einträge |

**Key insight:** SigV4 für einen Fall ist klein; was nicht klein ist, ist das Verhalten der Fremdserver (Prüfsummen, Fehlercodes, Regionen, Uhr). Das lässt sich nur mit echten Servern und einem unabhängigen Signaturvektor belegen, und beides ist in Befund 2 und 7 gemacht.

## Runtime State Inventory

Nicht zutreffend: Greenfield-Funktion, weder Umbenennung noch Migration. Eine Beziehung zur Laufzeit ist trotzdem zu nennen: `internal/store/migrate` ist nur berührt, wenn Auswahl C4 auf „Schema 8" fällt (siehe Befund 3).

## Befunde zu den sieben Fragen

### Befund 1 (Frage 1): Wie Snapshots genommen, abgelegt, gelistet, geprüft, aufbewahrt und gemeldet werden; Aufhängepunkt

Alle Aussagen unten sind gelesen (Read) in `internal/upgrade/backups.go` (665 Zeilen) und `safetysnapshot.go` (426 Zeilen).

- **Ablage:** je Cluster ein Verzeichnis `<DataDir>/upgrade-snapshots/<cluster-id>/` (`backup.UpgradeSnapshotsDirName` ist `upgrade-snapshots` [VERIFIED: internal/store/migrate/backup/backup.go:31], Verwendung in `cmd/holzkube-managerd/main.go:553-555`). Dateiname `<stamp>.snapshot` (vor Upgrade) oder `<stamp>.scheduled.snapshot` (Zeitplan); `snapshotStamp = "20060102T150405.000Z"` [VERIFIED: internal/upgrade/safetysnapshot.go:149]; die Arten heißen `SnapshotKindUpgrade = "upgrade"` und `SnapshotKindScheduled = "scheduled"` [VERIFIED: internal/upgrade/safetysnapshot.go:228-229]. Die Cluster-ID ist auf `[A-Za-z0-9._-]`, höchstens 128 Zeichen, nicht mit Punkt beginnend, beschränkt (`plainClusterID`, Zeilen 166-178), der Dateiname ist durch `parseSnapshotName` auf genau diese Grammatik festgelegt. **Folge:** Jeder Objektschlüssel aus Cluster-ID und Dateiname liegt innerhalb der unreservierten SigV4-Zeichen plus `/`. Nur das vom Betreiber gewählte Präfix muss auf dieselbe Grammatik eingeschränkt werden (Pitfall 4).
- **Prüfsumme:** `writeKind` rechnet SHA-256 beim Schreiben (`io.MultiWriter(w, sum)`) und schreibt `<name>.sha256` im `sha256sum -c`-Format **vor** dem atomaren Umbenennen des Snapshots, sodass ein auflistbarer Snapshot immer seine Prüfsumme hat (Commit e4cf86d, Kommentar Zeilen 307-312). Ein Fehler beim Schreiben der Prüfsumme kostet nur die Prüfsumme, nicht den Snapshot; `StoredSnapshot.SHA256` ist leer für Snapshots aus der Zeit vor den Prüfsummen (backups.go:121-122) und `readChecksum` liefert leer bei fehlender oder kaputter Datei (backups.go:150-163). **Folge:** Der Exporter muss den Fall `SHA256 == ""` behandeln (einmal streamend hashen, bevor das PUT beginnt) und testen.
- **Aufbewahrung:** `prune(dir, kind, keep)` behält die neuesten `keep` einer Art und räumt Temporärdateien; `keepSafetySnapshots = 2` für Upgrade-Snapshots [VERIFIED: internal/upgrade/safetysnapshot.go:43], `DefaultBackupKeep = 7` und `MaxBackupKeep = 60` für den Zeitplan [VERIFIED: internal/model/backup.go:26-27].
- **Zeitplaner und Job:** `JobKindEtcdBackup = model.JobKind("cluster.etcd-backup")` [VERIFIED: internal/upgrade/backups.go:36]; `SubmitBackup` schreibt den Versuch **vor** `submit` (Zeilen 496-528), `finishAttempt` schreibt das Ergebnis (474-489), der Job ist wiederaufnehmbar über `Happened` („ein geplanter Snapshot, der nicht älter ist als der Job, liegt da", 427-438). Beim Daemon-Neustart kann ein hängender Backup-Job wieder aufgenommen werden; ein Export ist davon unberührt, weil er kein Schritt dieses Jobs ist.
- **Gesundheit:** `healthOf(c, snaps, st, now)` leitet `BackupHealth` ab, `Overdue = now.After(base.Add(2 * iv))` mit `base` = letzter geplanter Snapshot oder `Since` (backups.go:268-275). `BackupHealthFor` (287-296) ist, was Übersicht und Metriken lesen; in `main.go` über `inv.SetBackupHealth(...)` (576-579) und `metrics.Deps.Backups` (640-650). Die Serien heißen `holzkube_etcd_snapshot_age_seconds` (fehlt, wenn es keinen Snapshot gibt), `…_schedule_enabled` und `…_overdue` (metrics.go:327-344).

**Aufhängepunkt, Empfehlung:** Der Abgleicher aus Muster 1; Anstoß am Ende von `writeKind`, nach `prune` und vor `return st.Latest(cluster)` (safetysnapshot.go:325-328). Dort laufen **beide** Arten durch. Auch ein Snapshot, den der Betreiber manuell auslöst (`POST /clusters/{id}/backups/run`), geht durch `takeScheduled` → `writeKind(SnapshotKindScheduled)` [VERIFIED: internal/upgrade/backups.go:464].

**Welche Snapshots exportieren? Empfehlung: beide Arten.** Die Phasenzielzeile lautet „jeder erfolgreiche etcd-Snapshot". Der Upgrade-Snapshot ist der Zustand unmittelbar vor der riskantesten Operation des Produkts (safetysnapshot.go:19-33) und deshalb der, den man bei einem Verlust des Pi am dringendsten braucht. Der Schlüssel enthält die Art im Dateinamen, ein Unterschied im Ziel ist nicht nötig. Kosten: höchstens zwei Dateien mehr je Cluster.

**Was ein laufender oder wiederaufgenommener Job und ein Daemon-Neustart für einen Export bedeuten:** Nichts, das persistiert werden müsste. Ein laufender Upload geht beim Neustart verloren, der Sidecar sagt dann (noch) nichts oder `failed`, der nächste Tick lädt die ganze Datei neu hoch (PUT desselben Schlüssels ist idempotent und ersetzt ein altes Objekt). Ein zerbrochener Upload hinterlässt am Ziel keine halbe Datei, weil der Server das Objekt erst mit vollständigem Körper und passender Prüfsumme übernimmt (belegt für den Prüfsummenfall in Befund 2; die Atomarität des PUT selbst ist `[ASSUMED]`, A3).

### Befund 2 (Frage 2): S3-Client, Pfadstil, Region, Größe, Prüfsummen, TLS

**Entscheidung: eigener Stdlib-Client.** Vergleich siehe „Alternatives Considered". Zusätzlich gemessen:

**Spike gegen zwei echte S3-Server (eigener SigV4 in 60 Zeilen, Pfadstil, Region `us-east-1`):**

| Fall | RustFS (aktuell) | SeaweedFS (aktuell) |
|---|---|---|
| PUT mit korrekter Signatur, `x-amz-content-sha256` = SHA-256 des Körpers, `x-amz-checksum-sha256` = base64 | 200, ETag und `x-amz-checksum-sha256` zurück | 200, ETag und `x-amz-checksum-sha256` zurück |
| PUT mit falschem `x-amz-checksum-sha256` | 400 `BadDigest` | 400 `BadDigest` |
| PUT mit **falschem** `x-amz-content-sha256` (Signatur über die gelogene Summe) | 400 `BadDigest` | **200 (akzeptiert!)** |
| PUT mit falschem Geheimnis | 403 `SignatureDoesNotMatch` (Körper ohne Schlüssel, Zugriffsschlüssel oder StringToSign) | 403 `SignatureDoesNotMatch` (Körper mit `Resource`, `RequestId`) |
| HEAD mit `x-amz-checksum-mode: ENABLED` | 200, `x-amz-checksum-sha256` zurück | 200, `x-amz-checksum-sha256` zurück |
| HEAD auf fehlendes Objekt | 404 | 404 |
| DELETE | 204 | 204 |

[VERIFIED: Ausgabe von `go run .` im Scratchpad `e_spike`, beide Server als Docker-Container dieser Session]

**Der Schlüssel zum Verständnis dieser Tabelle:** `x-amz-content-sha256` kann ein Server ungeprüft lassen (SeaweedFS), `x-amz-checksum-sha256` prüft ihn. Ein Ziel, das **beides** ignoriert, ist denkbar (ältere oder einfache Server, `[ASSUMED]`, A4). Deshalb bekommt der Export-Zustand `verified` (siehe Summary); „exportiert" ohne `response` oder `head` heißt „angekommen, vom Ziel nicht bestätigt", und die Oberfläche sagt es.

**Signatur gegen ein unabhängiges Orakel:** Mit festem Zeitpunkt 2026-10-16T12:00:00Z, Region `us-east-1`, Schlüssel `AK`/`SK` und Header `x-amz-checksum-sha256`, `x-amz-content-sha256` ergibt der eigene Signer für `PUT`, `HEAD` und `DELETE` auf `/bucket/cluster-a/20261016T120000.000Z.scheduled.snapshot` und auf `/bucket/` **bit-identische** `Authorization`-Header wie `github.com/aws/aws-sdk-go-v2/aws/signer/v4` mit `DisableURIPathEscaping = true` [VERIFIED: Ausgabe `identical: true` ×6 im Scratchpad `e_signer`]. Für einen **pathologischen** Schlüssel (`a b/ü+x~y=z&.snap`) weichen beide ab: der AWS-Signer signiert `req.URL.EscapedPath()` (lässt `+`, `=`, `&` stehen), der eigene Code kodiert jedes Byte außer `A-Za-z0-9-_.~` und `/` nach der SigV4-Regel. Das ist für diese Codebasis ohne Folge, weil jeder Schlüssel aus validierten Teilen besteht (Pitfall 4), ist aber der Grund, warum der Plan den **AWS-Vektor für die tatsächliche Schlüsselgrammatik** als Test einchecken soll und das Präfix streng beschränkt. Die AWS-Dokumentation hat ein Rechenbeispiel mit Signatur; die Seite war mit WebFetch nicht auslesbar, der Vektor muss daher aus dem Spike (festes Datum, `AK`/`SK`) kommen oder vom Executor aus der AWS-Seite von Hand übernommen werden (`[ASSUMED]`, A5: ob die AWS-Seite denselben Wert ergibt, ist nicht geprüft).

**Pfadstil und Region:** Pfadstil (`https://host/bucket/key`) ist der einzige Stil, der mit einer IP-Adresse und ohne DNS-Wildcard funktioniert, und der, den das LAN-MinIO-Szenario braucht. Virtual-Host (`https://bucket.host/key`) braucht Wildcard-DNS und -Zertifikat und ist nur das Host-Umschreiben im Client; die kanonische URI enthält den Bucket dann nicht. **Empfehlung:** Einstellung `addressing` mit den Werten `path` (Standard) und `virtual`; beides kostet im eigenen Client wenige Zeilen. Ob bestimmte Cloud-Anbieter (AWS, B2, Wasabi, R2) Pfadstil weiterhin annehmen, ist `[ASSUMED]` (A6). Region: Feld `region`, Standard `us-east-1` (beide getesteten Server akzeptierten es); bei AWS muss es die Region des Buckets sein, sonst `AuthorizationHeaderMalformed` (Fehlerabbildung: „the region is wrong, set the bucket's region").

**Größe und Multipart:** Ein einzelner PUT kann bis zu 5 GB [CITED: docs.aws.amazon.com/AmazonS3/latest/userguide/upload-objects.html: „With a single `PUT` operation, you can upload a single object up to 5 GB in size."]; Multipart: 5 MiB bis 5 GiB je Teil, höchstens 10 000 Teile, Objekt bis 48,8 TiB [CITED: docs.aws.amazon.com/AmazonS3/latest/userguide/qfacts.html]. Der etcd-Datenbankspeicher ist standardmäßig auf 2 GiB begrenzt, 8 GiB sind das empfohlene Maximum [CITED: etcd.io/docs/v3.5/dev-guide/limit/; docs.siderolabs.com/talos/v1.14/build-and-extend-talos/cluster-operations-and-maintenance/etcd-maintenance]; ein Snapshot ist so groß wie die Datenbank. **Empfehlung:** ein einziger PUT mit bekannter `Content-Length` bis **5 000 000 000 Byte** (konservativ, `5 GB`); größere Snapshots werden **nicht** hochgeladen, der Zustand lautet `failed` mit dem Satz „this snapshot is larger than one upload can carry (5 GB); multipart is not part of this release", sichtbar und nicht wiederholt (es ist kein vorübergehender Fehler). Multipart ist eine spätere Phase. Gesehen im Betrieb: 20 MiB (Auftrag); 2 GiB ist der Normalfall der Obergrenze.

**Streaming vom Pi:** Der Körper ist der geöffnete Snapshot (`SnapshotStore.Open` liefert `io.ReadCloser` und die Größe, backups.go:166-184), `Content-Length` gesetzt, kein Puffern im Speicher. Weil `x-amz-content-sha256` **vor** dem Körper im Header stehen muss, liefert die Sidecar-SHA-256 die Summe ohne zweite Leserunde. Den Fall `SHA256 == ""` (Alt-Snapshot) deckt ein vorgeschalteter Hash-Durchlauf. `UNSIGNED-PAYLOAD` bewusst nicht benutzt: es ist nur über TLS erlaubt (`[ASSUMED]`, A7) und nimmt dem Ziel die zweite Prüfung.

**Prüfsummen-Header:** `x-amz-checksum-sha256` (base64 der 32 Roh-Bytes) für Einzel-PUT; „If the values don't match, the request is rejected" [CITED: docs.aws.amazon.com/AmazonS3/latest/userguide/checking-object-integrity-upload.html]. Zusätzlich je Snapshot ein Objekt `<name>.sha256` im `sha256sum -c`-Format (derselbe Inhalt wie die lokale Datei), damit ein Mensch eine Kopie mit dem Standardwerkzeug prüfen kann. Reihenfolge: Snapshot zuerst, Sidecar danach; erst beide 2xx → `exported`. Ein Wiederholungsversuch lädt beide neu (idempotent).

**Serverseitige Verschlüsselung:** nicht angefordert (`x-amz-server-side-encryption` nicht gesetzt; mancher Server lehnt es ohne konfigurierten Schlüsseldienst ab, `[ASSUMED]`, A8). Der Hinweis „bucket-seitige Verschlüsselung am Ziel einschalten" gehört in den Guide. Clientseitige Verschlüsselung ist Deferred.

**TLS und eigene CA für ein selbst signiertes LAN-Ziel (der wichtigste Betreiberfall):** Erforderlich ist ein Weg, Vertrauen herzustellen **ohne** die Prüfung abzuschalten.
1. **Standard:** Systemwurzeln (`tls.Config.RootCAs == nil`).
2. **CA-PEM:** Einstellung `ca_pem` (ein oder mehrere Zertifikate, PEM). Der Client baut `x509.NewCertPool()` und hängt es mit `AppendCertsFromPEM` an; `ok == false` oder null Zertifikate ist ein Validierungsfehler beim Speichern. Das PEM ist kein Geheimnis (öffentliches Zertifikat), steht in der Einstellung, nicht im Geheimnis, und darf in der API gezeigt werden. Ob der Pool nur die eigene CA enthält oder Systemwurzeln **plus** eigene CA, ist eine Auslegungsfrage; **Empfehlung: nur die eigene CA, wenn gesetzt** (ein Ziel, das mit einer eigenen CA betrieben wird, soll nicht auch jede Systemwurzel akzeptieren). Ein selbst signiertes Blattzertifikat, das selbst im Pool liegt, ist für Go ein gültiger Anker, solange der Hostname zu den SANs passt; ein Zertifikat ohne passenden SAN (nur Common Name) wird abgelehnt, das ist ein Hinweis für den Guide.
3. **Fingerprint-Pin** (Alternative, Auswahl C3): wie `internal/talos/pin.go` (`VerifyConnection` auf **jeder** Handshake, nicht `VerifyPeerCertificate`, wegen Sitzungswiederaufnahme [VERIFIED: internal/talos/pin.go:99-117 (Kommentar ab 99)]). Bequemer für den Betreiber, bricht aber bei jeder Erneuerung des Blattzertifikats.
4. **Nie:** `InsecureSkipVerify` ohne Pin.

**Klartext-HTTP:** Der Inhalt ist die gesamte etcd-Datenbank des Clusters (der Code selbst sagt „holds every Kubernetes secret", backups.go-Handler `backupsState` „Each one holds every Kubernetes secret of the cluster"). Die Signatur schützt die Schlüssel, nicht den Körper. **Empfehlung:** `https://` verlangen (Auswahl C2).

**Redirects:** verweigern, wie `k8srelease` es mit `refuseForeignRedirect` tut [VERIFIED: internal/k8srelease/k8srelease.go:110, 126]; ein Redirect auf PUT würde den Körper an einen anderen Host schicken. `301/307` von S3 (falsche Region oder falscher Endpunkt) wird als Satz gemeldet („the target redirected to another address; check the endpoint and region").

**SSRF:** Der Endpunkt ist vom Admin konfiguriert (Sudo-Fenster, Audit); das ist ein ausgehender Aufruf an eine vom Admin gewählte Adresse mit dem Snapshot im Körper, was genau das Feature ist. Gegenmaßnahmen: nur Admin, `Destructive`, https, keine Redirects, Endpunkt als reiner `scheme://host[:port]` ohne Pfad, Benutzerinfo oder Query validiert, kein Aufruf auf die eigene Listen-Adresse (`cfg.Listen`) als Ziel.

**Uhr:** SigV4 verlangt eine Uhr innerhalb von 15 Minuten (`RequestTimeTooSkewed` bzw. `x-amz-date` zu weit weg, `[ASSUMED]`, A9: AWS-Frist 15 min). Der Pi hat keine Batteriepufferung der Uhr; fällt NTP aus, scheitert **jeder** Export mit 403. Die Fehlerabbildung braucht den Satz „the target says this device's clock is too far off; check the time on the Pi" (aus `Code == RequestTimeTooSkewed`), und der Test fährt den Client mit einer verschobenen Uhr.

### Befund 3 (Frage 3): Konfiguration, Geheimnisse, Schema, Audit

**Wo liegt was:** Flags oder Entity? D-02 verlangt „Geheimnis im Datenverzeichnis". Die bestehenden Geheimnisse des Produkts gehen über Flags (`oidc-client-secret`, `oidc-client-secret-file`, `internal/config/config.go:462-483` [gelesen]) **oder** über eine Entity mit eigenem Ordner (`cluster-secrets`). Eine in der Oberfläche änderbare Einstellung kann kein Flag sein. **Empfehlung: zwei Entities** nach dem Vorbild `clusters` / `cluster-secrets` [VERIFIED: internal/store/store.go:48-86 (Store-Schnittstelle), internal/store/fsstore/fsstore.go:101-110 (Verdrahtung)]:

- `backup-targets/<id>.json` (`model.BackupTarget`): `Kind` (`"s3"`), `Format`, `Rev`, `Since`, `S3` mit `Endpoint`, `Bucket`, `Region`, `Prefix`, `Addressing`, `CAPEM`. Kein Geheimnis, in API und Oberfläche zeigbar.
- `backup-target-secrets/<id>.json` (`model.BackupTargetSecret`): `AccessKeyID`, `SecretAccessKey`. Lädt nur der Exporter und der Schreibpfad des Handlers; das GET lädt es nicht, womit „der Handler kann nicht ausgeben, was er nie geladen hat" ein Strukturmerkmal ist (so begründet es `model/inventory.go`/`store.go`, Kommentar zu `ClusterSecrets`).
- Die `Id` ist in dieser Phase konstant `default` (ein Ziel). Der Schlüssel ist trotzdem ein Typ (`TargetID`), damit das zweite Ziel (SFTP) kein Umbau des Formats wird (D-01: Format veröffentlicht, `costly`).
- **Der Zugangsschlüssel (Key-ID) ist ebenfalls schreibgeschützt.** D-02 nennt „der Zugangsschlüssel" ein Geheimnis. Die API zeigt `secret_set: true` und **keinen Hinweis** auf Inhalt, auch nicht die letzten vier Zeichen; „nie in einer Antwort" lässt keinen Hinweis zu. Der Betreiber prüft mit „Ziel testen".

**Alternative Einstellung am `Settings`-Singleton:** `model.Settings` trägt heute `WallLinks` (Zugangsdaten) und wird breit geladen; ein Geheimnis dort hätte keine Strukturgrenze. Nicht empfohlen.

**Schema-Version, die echte Frage.** `CurrentVersion = 7` [VERIFIED: internal/store/migrate/migrate.go:26]. Jede Entity der Vergangenheit hat eine eigene Migration, die ihren Ordner anlegt; die letzte (`6 → 7`) legte `config-baselines` an und begründet den Bump ausdrücklich damit, dass „the operator chose it so (D-08, 2026-10-14)" und nennt die Folge: eine Binary von vor diesem Schema weigert sich (`ErrVersionTooNew`), das Update-Skript rollt die Binary zurück, **nicht** das Verzeichnis, der Weg zurück ist das Backup `backups/pre-migration-6-to-7-<time>.tar.gz` [VERIFIED: internal/store/migrate/migrate.go:157-170 (Kommentar der Migration 6→7)]. `fsstore.Open` legt jeden Entity-Ordner auch ohne Migration an (`os.MkdirAll` in der Schleife, fsstore.go:112-120 [gelesen, `for _, sub := range []string{` steht in Zeile 112]), ein Test, der eine Migration je Ordner erzwingt, wurde nicht gefunden (Suche in `internal/store`, `migrate_test.go`, Fixtures: `version-6` ist die letzte, der Fixture-Katalog in `internal/store/testdata/README.md` ist Dokumentation, kein Test). Ein Bump auf 8 braucht also: eine Migration, ein Fixture `version-7/`, den README-Eintrag, `CurrentVersion = 8` und den Leitfaden zum Rückrollen. Das ist eine **echte Auswahl** (siehe C4): ein Bump macht ein Zurückrollen laut (die ältere Binary startet nicht), ohne Bump hört der Export nach einem Zurückrollen **still** auf (die ältere Binary kennt den Ordner nicht und ignoriert ihn; lokale Backups laufen weiter).

**Audit:** Neue Aktionen müssen in `internal/audit/redact.go` stehen, sonst scheitert `TestEveryAuditedActionIsInTheAllowlist` (cmd/holzkube-managerd/allowlist_test.go [gelesen]). Die vorhandenen Einträge der Backups sind `"cluster.backup-schedule": {"interval", "keep"}`, `"etcd.backup-run": {}`, `"etcd.backup-download": {}` [VERIFIED: internal/audit/redact.go:560-562]. Empfehlung:
- `backup-export.configure`: `{"kind", "endpoint", "bucket", "region", "prefix", "addressing"}` (vom Betreiber gewählte Namen, kein Geheimnis). **Nicht** gelistet: `access_key_id`, `secret_access_key`, `ca_pem`; sie schreibt der fail-closed-Standard als `<redacted>` (so steht es in `redact.go` selbst). Endpunkt und Bucket stehen in einem Archiv, das nichts je löscht (D-16); sie identifizieren die Installation, sind aber nur auf dem Pi selbst lesbar. Wer das nicht will, listet nur `kind` und `addressing`; **Empfehlung:** `endpoint` und `bucket` stehen drin, weil ein Audit-Eintrag „das Ziel wurde geändert" ohne das Ziel keine Antwort auf „wohin gingen die Snapshots im März" gibt.
- `backup-export.remove`: `{}`.
- `backup-export.test`: `{}`.
- `etcd.backup-export-run`: `{}`.
Pflicht-Test: ein Body mit `secret_access_key: "zzz"` schreibt `<redacted>` und der Klartext steht **nirgends** im Audit-Archiv (Injektion: `secret_access_key` in die Allowlist aufnehmen → Test rot).

**Nie in Logs, Fehlern, Metriken:** Typ `Secret` (siehe „Don't Hand-Roll"); Metrik-Labels sind nur `cluster`; Fehlersätze kommen aus einer **festen Tabelle** (Befund 4), nie aus dem Fremdkörper; `reason` in `export-status.json` hat denselben Deckel von 300 Zeichen wie `finishAttempt` (backups.go:481-485 [VERIFIED]) und ersetzt das Snapshot-Verzeichnis, den Endpunkt-Host nicht. Die Tests prüfen Log (slog-Handler im Test), Fehlertext, `GET`-Antwort und `/metrics` auf Teilzeichenketten von Schlüssel und Geheimnis.

**Rollen und Sudo:** Ziel setzen, löschen: `MinRole: model.RoleAdmin`, `Destructive: true` (Sudo-Fenster, Begründung wie `backupSetSchedule`: „the other settings that decide what this product does on its own" [VERIFIED: internal/httpapi/handlers/backups.go:38-47 (Kommentar und Route)]). Ziel lesen: `RoleAdmin` (es zeigt Endpunkt und Bucket); „Ziel testen" und „jetzt exportieren": Admin bzw. Operator wie `etcd.backup-run` (`RoleOperator`). Das Ziel „keine `ClusterScope`": der Export ändert nichts an einem Knoten, ein read-only adoptiertes Cluster darf exportiert werden (wie das Backup selbst, handlers/backups.go:49-50).

**Phase 19 teilt sich Teile:** Alerting (D-02) legt ebenfalls ein Geheimnis (Webhook-URL) im Datenverzeichnis ab und braucht ebenfalls einen ausgehenden HTTP-Client mit Budget und Redirect-Verweigerung. Der `Secret`-Typ und der Aufbau „Entity + Entity für das Geheimnis" sollten **einmal** entstehen, nicht zweimal verschieden; Planner prüft die Reihenfolge der Phasen (Open Question 4).

### Befund 4 (Frage 4): Fehlerverhalten, Zustand, Überfällig, Rückstau, Aufbewahrung

**Wiederholung mit Grenze.** Zwei Ebenen:
- **Innerhalb eines Durchlaufs** (für einen Snapshot): bis zu 3 Versuche mit 10 s und 30 s Abstand, **nur** bei vorübergehenden Fehlern (Netz, Zeitüberschreitung, 5xx, 429/503 mit `SlowDown`). Dauerhafte Fehler (403 Zugriff oder Signatur, 404 `NoSuchBucket`, 400, `EntityTooLarge`, Konfiguration ungültig) werden **nicht** wiederholt.
- **Zwischen Durchläufen:** nach einem fehlgeschlagenen Durchlauf schreibt der Sidecar `attempts`, `last_attempt_at` und `next_try_at = last_attempt_at + gap`, `gap = min(max(interval/6, 10min), 1h)` (die Formel von `retryGap`, backups.go:361-363 [VERIFIED]; ohne Zeitplan 1 h). Bis dahin kein weiterer Versuch, auch nicht nach einem Neustart (der Sidecar liegt auf der Platte). Der Abgleicher **hört nicht auf**, solange der Snapshot lokal liegt; er ist aber nie schneller als einmal je `gap`. Ein Klick auf „jetzt exportieren" und das Speichern der Einstellung löschen `next_try_at` (der Betreiber hat etwas geändert).
- Pflicht-Test: bei einem Fake, der immer 500 liefert, gibt es je Durchlauf höchstens 3 Versuche und im Abstand `gap` keinen weiteren; ein Neustart (neuer Service auf derselben Platte) beginnt **nicht** sofort neu (Injektion: Wiederholung ohne Grenze oder `next_try_at` nicht gelesen).

**Sichtbarer Zustand je Snapshot und je Cluster.** `StoredSnapshot` bekommt additiv `Export *SnapshotExport` (`omitempty`) mit `state` (`exported`, `pending`, `failed`, `not-configured`), `at`, `verified`, `reason`, `attempts`, `next_try_at`. Je Cluster `ExportStatus` aus `export-status.json`: letzter Versuch, Ergebnis (`running`, `ok`, `failed`), Grund, aufeinanderfolgende Fehler. Zustände sind Sätze („the target refused the key (HTTP 403)"), nie Roh-Texte. `ExportHealth` am Cluster (abgeleitet, nie gespeichert), additiv an `BackupHealth` als `Export *ExportHealth` mit `omitempty`:
```go
// Source: eigener Entwurf, Form nach model.BackupHealth (internal/model/backup.go).
type ExportHealth struct {
    Enabled       bool       `json:"enabled"`        // ein Ziel ist konfiguriert
    Overdue       bool       `json:"overdue"`
    LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
    Pending       int        `json:"pending"`        // lokal vorhanden, noch nicht (zum aktuellen Ziel) exportiert
    Failed        int        `json:"failed"`
    Lost          int        `json:"lost"`           // vor dem Export lokal aufgeräumt (Auswahl C1)
    LastResult    string     `json:"last_result,omitempty"`
    LastReason    string     `json:"last_reason,omitempty"`
}
```
**„Kein Ziel konfiguriert" ist ein eigener Zustand** (`Enabled=false`), nie „ok" und nie „nicht überfällig im Sinne von sicher"; die Backups-Tafel trägt ohnehin den Hinweis, dass die Snapshots nur auf dem Gerät liegen (handlers `backupsState`, Feld `notice`), und der Export-Block sagt „no export target: these snapshots do not survive the loss of this device".

**Überfällig, Definition:** wie `healthOf`, aber am Export gemessen.
`Overdue = Enabled ∧ Pending > 0 ∧ now − max(LastSuccessAt, target.Since) > 2 × interval(c)`
mit `interval(c)` aus dem Zeitplan des Clusters, ohne Zeitplan 24 h. `target.Since` ist der Zeitpunkt der letzten Änderung des Ziels (wie `BackupSchedule.Since`: ein neu eingerichtetes Ziel ist kein sofortiger Alarm). Gegenstück `Pending == 0` heißt: nichts zu exportieren, das ist Sache des Backup-Überfällig. Eine Serie `holzkube_etcd_snapshot_export_overdue` (0/1 je Cluster, **immer geschrieben**), `holzkube_etcd_snapshot_export_enabled` (0/1), `holzkube_etcd_snapshot_export_age_seconds` (Alter des neuesten **exportierten** Snapshots, fehlt wie `…_age_seconds`, wenn es keinen gibt), `holzkube_etcd_snapshot_export_pending` (Zahl). Nur das Label `cluster`; kein Endpunkt, kein Bucket (Muster `writeBackups`, metrics.go:323-345 [gelesen]; Hilfetext „a series that drops to zero is still written", api-contract). Phase 19 („Snapshot überfällig") liest denselben `ExportHealth`.

**Rückstau nach einem Ausfall.** Optionen: (a) nur der neueste; (b) alle noch lokal liegenden, alt zuerst; (c) alle noch lokal liegenden, **neueste zuerst**. **Empfehlung (c), Ermessen:** Nach einem Ausfall wird zuerst der Wiederherstellungspunkt gesichert, der am meisten wert ist; danach der Rest, solange er lokal liegt (höchstens `keep + 2`, also bei 7 sind es neun Dateien). Kosten: Bandbreite für bis zu neun Dateien nach einem langen Ausfall. Nutzen: jeder Wiederherstellungspunkt, den das Gerät noch hatte, existiert am Ziel. Test: drei Snapshots, Ziel zuerst down, dann up: Reihenfolge der Schlüssel in den Fake-Zugriffen ist neu → alt.

**Zielwechsel:** Ändert sich das Ziel (Endpunkt, Bucket, Präfix, Region), ändert sich `Fingerprint()`. Ein Sidecar mit **anderem** Ziel-Fingerprint gilt als `pending` zum neuen Ziel. **Empfehlung:** so, denn „exportiert" bedeutet „am **aktuellen** Ziel"; ein Betreiber, der von einem LAN-MinIO auf einen Cloud-Bucket wechselt, will die lokal vorhandenen Snapshots dort sehen. Kosten: ein einmaliger Rückstau. Test: Sidecar trägt das alte Ziel → Export am neuen (Injektion: Fingerprint ignorieren → Test rot). Wechsel nur der Zugangsdaten ändert den Fingerprint **nicht**.

**Zusammenspiel mit der lokalen Aufbewahrung (Plattenwächter): echte Auswahl C1.** `prune` kennt den Export nicht und löscht den ältesten Snapshot einer Art, sobald `keep` überschritten ist. Optionen:
- **A (Empfehlung): unabhängig, aber sichtbar.** `prune` bleibt unverändert; ein lokal aufgeräumter, nie exportierter Snapshot wird gezählt (`Lost`, Haken `OnPrune` am `SnapshotStore`) und angezeigt. Kosten: bei langem Ausfall und `keep` klein kann ein Wiederherstellungspunkt nie das Ziel erreichen. Nutzen: der Plattenwächter und das Versprechen „höchstens `keep` Dateien" des Betreibers bleiben wahr; der Pi kann nicht durch einen toten Export volllaufen. Die Warnung kommt früh: Überfällig tritt nach `2 × interval` ein, bei `keep ≥ 3` lange bevor etwas aufgeräumt wird; bei den zwei Upgrade-Snapshots schließt der sofortige Anstoß die Lücke.
- **B: Zurückhalten mit Deckel.** `prune` löscht einen nicht exportierten Snapshot nicht, solange das Ziel konfiguriert ist, aber höchstens bis `keep + 3` Dateien einer Art; darüber löscht es doch. Kosten: Komplexität in `prune`, ein Test je Grenze; Plattenbedarf `keep + 3` statt `keep`. Nutzen: kleinere Lücke.
- **C: unbegrenzt zurückhalten.** Kosten: Der Pi läuft bei totem Ziel voll, der vorhandene Plattenwächter (`guardDisk`) lehnt dann jeden neuen Snapshot ab (`ErrDiskLow`, backups.go:44, 90-110) — ein toter Export kostet das Backup selbst. **Nicht empfohlen.**
Test (A): `keep=2`, drei Snapshots, Ziel aus: der älteste wird lokal gelöscht und `Lost == 1` ist sichtbar (Injektion: `Lost` nicht zählen → rot). Test (B): derselbe Aufbau, der älteste bleibt erhalten, bis `keep+3` überschritten ist.

**Am Ziel gelöscht wird in dieser Phase nichts.** Der Schlüssel braucht dann nur `PutObject` (und `GetObject` für den HEAD), kein `DeleteObject`, kein `ListBucket`. Das Spiegeln von `keep` am Ziel wäre eine echte Alternative und braucht `DeleteObject` plus `ListBucket`, was einem Schlüssel, der einen Verlust des Pi überleben soll, eher nicht zusteht (ein gestohlener Pi trüge sonst den Schlüssel, der die Backups löschen kann). **Empfehlung:** nichts löschen; die Bucket-Lebenszyklusregel (Ablauf nach N Tagen) am Ziel ist der richtige Ort und gehört in den Guide. Als Auswahl C6 geführt, falls der Betreiber das Spiegeln will.

**Fehlerabbildung (feste Tabelle, Satz statt Fremdkörper):**

| Auslöser | Klasse | Satz (Beispiel) |
|---|---|---|
| Netz, Zeitüberschreitung, DNS | vorübergehend | „the target could not be reached" |
| 5xx, 429, `SlowDown`, `InternalError` | vorübergehend | „the target had a problem and will be tried again" |
| 403 `SignatureDoesNotMatch`, `InvalidAccessKeyId`, `AccessDenied` | dauerhaft | „the target refused the key (HTTP 403)" |
| 403 `RequestTimeTooSkewed` | dauerhaft bis Uhr stimmt | „the target says this device's clock is too far off" |
| 404 `NoSuchBucket` | dauerhaft | „the bucket does not exist at the target" |
| 301/307, `PermanentRedirect`, `AuthorizationHeaderMalformed` | dauerhaft | „the endpoint or region is wrong for this bucket" |
| 400 `BadDigest` | dauerhaft (Rückfrage) | „the target received different bytes than were sent" |
| `EntityTooLarge`, lokal > 5 GB | dauerhaft | „this snapshot is larger than one upload can carry" |
| TLS-Prüfung scheitert | dauerhaft | „the target's certificate is not trusted; add its CA in the settings" |

Aus dem Fremdkörper wird höchstens das Element `<Code>` gelesen (Länge begrenzt, nur `[A-Za-z]`), nie `<Message>`, `<StringToSign>`, `<AWSAccessKeyId>` oder `<Resource>`. AWS S3 gibt bei `SignatureDoesNotMatch` den Zugriffsschlüssel und den StringToSign im Körper zurück (`[ASSUMED]`, A10; RustFS und SeaweedFS taten es nicht, SeaweedFS nannte `Resource` und `RequestId`).

### Befund 5 (Frage 5): Die Frist-Lektion

Der Talos-Client verweigert Aufrufe ohne Frist (`ErrNoDeadline`, Ledger 137, Quick-Task 261014-gl2; Phase 15 maß es am Simulator: „refusing a call with no deadline"). Der Export spricht **nicht** mit dem Talos-Client, sondern mit `net/http`; dort verweigert nichts. Die Projektregel lautet trotzdem „jeder Außenaufruf hat sein eigenes Budget", und `imagefactory` setzt sie mit `requireDeadline` an den **zwei** `http.Client.Do`-Stellen um [VERIFIED: internal/imagefactory/client.go:262-288]. Der S3-Client hat genau **eine** Stelle (`do`) mit `requireDeadline` und `ErrNoDeadline`. Der Abgleicher läuft in `main.go` auf `context.WithCancel(context.Background())` (keine Frist) und setzt je Aufruf `WithTimeout` (Muster 4).
**Wächter vom Kontext ohne Frist aus (Form wie Phase 15):** Der Test startet den Service auf `context.Background()` (nicht auf `testContext` mit 60 s, das sähe den Fehler nie), lässt einen Snapshot exportieren und erwartet `exported`. Injektion: `WithTimeout` im Abgleicher entfernen → der Client gibt `ErrNoDeadline`, Test rot. Zweiter Test: der Client mit einem Kontext **ohne** Frist liefert `ErrNoDeadline` und sendet nichts (Fake zählt Anfragen = 0).

### Befund 6 (Frage 6): UI, API, Dokumentation

**API (neu):**
- `GET /api/v1/backup-export` (`RoleAdmin`): `{"target": {kind, endpoint, bucket, region, prefix, addressing, ca_pem, since} | null, "secret_set": bool, "presets": …}`; nie ein Schlüssel.
- `PUT /api/v1/backup-export` (`RoleAdmin`, `Destructive`, Action `backup-export.configure`): Body mit allen Feldern; **Geheimnis weggelassen = vorhandenes behalten**, damit die Oberfläche das Geheimnis nie zurücklesen muss; Validierung am Server (Endpunkt `https://host[:port]`, Bucket-Name nach der S3-Grammatik, Präfix nur `[A-Za-z0-9._-/]` ohne `..`, Region `[a-z0-9-]+`, `addressing ∈ {path, virtual}`, CA-PEM parsbar).
- `DELETE /api/v1/backup-export` (`RoleAdmin`, `Destructive`, `backup-export.remove`): löscht Ziel **und** Geheimnis; lokale Sidecars bleiben (sie nennen das alte Ziel und werden `pending`, sobald ein neues kommt).
- `POST /api/v1/backup-export/test` (`RoleAdmin`, Action `backup-export.test`, **Budget-Zeile nötig**): siehe unten.
- `POST /api/v1/clusters/{id}/backups/export` (`RoleOperator`, Action `etcd.backup-export-run`): löscht `next_try_at` der Cluster-Snapshots, stößt den Abgleicher an, `202`.
- `GET /api/v1/clusters/{id}/backups` bekommt additiv `export` (der `ExportHealth`) und je Snapshot `export` (siehe Befund 4). Das Feld ist additiv; `web/src/api.ts` bekommt die zod-Felder dazu, mit `nullish`-Test wie `api.nulls.test.ts` ihn verlangt.
Die neuen Problem-Codes müssen in `docs/api-contract.md` stehen **und** ausgesendet werden (`TestEveryProblemCodeIsInTheContract`, `TestEveryProblemCodeIsEmitted`, `TestNoStaleCodeExemptions` [gelesen: internal/httpapi/contract_codes_test.go:28, 62, 118]); jede Route braucht ihren Eintrag in `docs/api-contract.md` (Gate `api_coverage_gate` in `.planning/config.json`).

**„Ziel testen": entscheiden. Empfehlung: ein kleines Objekt schreiben und per HEAD prüfen, nichts löschen.** Schreiben: `PUT <prefix>/.holzkube-manager-test` (kleiner fester Inhalt mit Zeitstempel, mit `x-amz-checksum-sha256`), dann `HEAD` auf denselben Schlüssel (Größe, Prüfsummen-Echo). Immer **derselbe** Schlüssel, damit nichts anwächst. **Kein DELETE:** der Schlüssel hat vielleicht kein `DeleteObject` (und soll es nach Befund 4 nicht haben), ein Bucket mit Objektsperre erlaubt es nicht; ein Test, der wegen eines fehlenden Löschrechts rot wird, würde ein korrektes Ziel verwerfen. Der Test legt **keinen Bucket an** und listet nichts (kein `HeadBucket`, kein `ListBucket`, beides sind eigene Berechtigungen). Die Antwort sagt, was geprüft wurde: „write and read-back verified; the object `…/.holzkube-manager-test` stays at the target" und `verified` (`response`/`head`/`size`/`none`). Die Testroute testet das **gespeicherte** Ziel (kein Geheimnis im Testbody; speichern, dann testen); das Speichern verlangt keinen erfolgreichen Test. Alternative (Body mit Entwurfswerten testen) würde das Geheimnis in einen zweiten Body legen und eine zweite Audit-Redaktion brauchen: nicht empfohlen.

**Oberfläche:**
- **Einstellungskarte** („Backup export", neue Datei, in `web/src/routes/settings.tsx` neben `BackupCard`, Admin-sichtbar): Felder Endpunkt, Bucket, Region, Präfix, Adressierung, CA-PEM (Textfeld), Zugangsschlüssel und Geheimnis als `type="password"` mit `autocomplete="off"`; nach dem Speichern leer und mit dem Hinweis „set" (nie zurückgelesen), „replace" statt „edit". Knöpfe „Save" (Sudo-Dialog wie `backupSetSchedule`), „Test the target", „Remove". Vorsicht Namen: `BackupCard` in den Einstellungen ist das Backup des **Datenverzeichnisses**; die neue Karte heißt „etcd backup export" und der Text trennt beides.
- **Backups-Tafel** (`web/src/components/Backups.tsx`, Query `['backups', cluster]`, heute 314 Zeilen): je Snapshot eine Spalte „Export" (Badge `exported`, `pending`, `failed`, `not configured`, Tooltip mit Zeit, `verified` und Grund), im Kopf ein Block mit `Enabled`, `Overdue` (derselbe Alarm-Ton wie „overdue" des Backups), `Pending`, `Failed`, `Lost` und der Hinweis „no export target: …". Knopf „Export now". Zugänglichkeit und Tippziel-Wächter laufen in `task ci` mit (`test:layout`).
- **Clusterkarte:** eine Zeile „Backup export" (Muster `ConfigDriftRow`/„Cluster certificates"); Wand: **nicht** Teil der Phase (die Wand projiziert explizit, ein neues Feld erreicht sie nicht).
- **README:** Eintrag in „What it does" im selben Schritt; ein Bild nur, wenn die Einstellungskarte eine eigene Seite ist (ist sie nicht): kein Screenshot nötig, `web/fixtures/demo.json` bekommt trotzdem den Zustand der Backups-Tafel (`minio.example.com`), falls diese schon in „A look around" steht.
- **Guide:** Abschnitt unter „Scheduled etcd backups" (docs/guide.md:1171 hat ein Gegenstück in `api-contract.md:1171`; die Stellen mit `grep -n "Scheduled etcd" docs/guide.md docs/api-contract.md` neu finden): Einrichtung, minimale Berechtigung (`s3:PutObject` und `s3:GetObject` auf das Präfix, **kein** Delete, **kein** List), Lebenszyklusregel am Ziel, CA-PEM für ein selbst signiertes Ziel, Uhr, die Grenze „5 GB", `verified` und was jede Stufe heißt, Klartext-HTTP wird abgelehnt, **Hinweis: Snapshots werden unverschlüsselt exportiert (Verschlüsselung ist Deferred); bucket-seitige Verschlüsselung einschalten**.
- **Ledger-Ehrlichkeit:** gegen echte Server wurde in dieser Recherche nur der Spike gemessen (RustFS, SeaweedFS); die Phase beweist nur gegen den eigenen Fake, **nie** gegen AWS, B2 oder R2, und gegen **kein** TLS-Ziel mit eigener CA außer dem `httptest`-TLS-Server. Das gehört wörtlich in Ledger und Guide.

### Befund 7 (Frage 7): Testbarkeit

**Fake-S3 mit `httptest`:** `httptest.NewServer` (und `NewTLSServer` für den CA-Pfad). Er muss **prüfen**, nicht nur antworten (sonst beweist der Test nichts):
1. **Signatur** unabhängig nachrechnen — mit dem im Test eingecheckten Vektor, nicht mit der Produktionsfunktion (Zirkelschluss). Zwei Ebenen: (a) ein fester Vektor (Zeitpunkt, `AK`/`SK`, Region, Header, erwartete `Authorization`-Zeichenkette) für die Schlüsselgrammatik der Codebasis, gewonnen aus dem AWS-Signer im Scratchpad dieser Recherche (Ergebnis identisch, siehe Befund 2), und (b) ein Fake-Verifizierer, der die Signatur aus Anfrage und Geheimnis **mit eigener, bewusst anders geschriebener Rechnung** prüft; der AWS-Signer darf **nicht** als Testabhängigkeit in `go.mod`.
2. **Pfad** (`/bucket/prefix/cluster/name` im Pfadstil, Host `bucket.host` im Virtual-Stil) und Methode.
3. **Header:** `x-amz-content-sha256` ist die SHA-256 des Körpers, `x-amz-checksum-sha256` ist ihr base64, `x-amz-date` ist nah an der Uhr, `Host` stimmt, `Content-Length` stimmt.
4. **Körper:** SHA-256 der empfangenen Bytes gleich der Sidecar-Summe; ein Fake-Modus „prüft den Prüfsummen-Header **nicht**" (SeaweedFS-Verhalten für `x-amz-content-sha256`, und ein Server ganz ohne Echo), damit `verified: none/size` und die Anzeige getestet sind.
5. **Fehlerarten** einstellbar: 500, 503 `SlowDown`, 403 mit `SignatureDoesNotMatch` und einem Körper, der **Zugriffsschlüssel und StringToSign enthält** (Test: nichts davon gelangt in Log, Fehler, Antwort), `RequestTimeTooSkewed`, 404 `NoSuchBucket`, 307, abgebrochene Verbindung mitten im Körper, langsame Antwort (Frist), 400 `BadDigest`.
6. **TLS:** `NewTLSServer` mit dem Zertifikat des Servers als CA-PEM im Ziel → Erfolg; ohne CA → Satz „certificate is not trusted"; **kein** Test setzt `InsecureSkipVerify`.

**Injizierbare Fehler (je Anspruch, jeder muss rot gesehen werden):**

| Anspruch | Einzusetzender Fehler | Wächter, der rot werden muss |
|---|---|---|
| Jeder erfolgreiche Snapshot wird exportiert | der Anstoß am Ende von `writeKind` entfällt **und** der Tick listet nicht (Abgleicher überspringt Upgrade-Art) | `TestBothKindsAreExported` (geplant und Upgrade) |
| Export überspringt nichts | Abgleicher exportiert nur den neuesten | `TestBacklogIsExportedNewestFirst` |
| Prüfsumme kommt vor dem Snapshot | Reihenfolge Sidecar/Snapshot im Export vertauscht (oder PUT ohne Prüfsummen-Header) | `TestTheFakeSeesTheChecksumHeaderAndTheBodyDigest` |
| Geheimnis nie in Log/Fehler/Antwort/Metrik | Fehlersatz enthält `err.Error()` des Fremdkörpers; `Secret` ohne `LogValue` | `TestNoSecretInLogErrorResponseOrMetrics` (mit dem AWS-artigen Fehlerkörper) |
| Fehlgeschlagener Upload wird nicht als exportiert markiert | Sidecar wird vor dem PUT geschrieben oder bei 500 auf `exported` gesetzt | `TestAFailedUploadIsNeverMarkedExported` |
| Nur beide Objekte (Snapshot und Sidecar) zählen | Sidecar-PUT-Fehler wird ignoriert | `TestExportedNeedsBothObjects` |
| Wiederholung hat eine Grenze | Schleife ohne Zähler / `next_try_at` ignoriert | `TestRetryHasALimitAndASpacing` (Fake-Uhr) |
| Ein Neustart wiederholt nicht sofort | `next_try_at` nicht aus dem Sidecar gelesen | `TestARestartDoesNotRetryImmediately` |
| Aufbewahrung löscht nichts, was nicht gezählt wird (C1 A) | `Lost` nicht gezählt | `TestPruningAnUnexportedSnapshotIsCountedAndShown` |
| Zielwechsel exportiert neu | Fingerprint nicht verglichen | `TestAChangedTargetIsExportedAgain` |
| Frist-Lektion | `WithTimeout` im Abgleicher entfällt | `TestAnExporterOnAContextWithNoDeadlineStillExports` (auf `context.Background()`), `TestTheClientRefusesACallWithNoDeadline` |
| Überfällig | Basis `LastSuccessAt` falsch oder `Pending` ignoriert | `TestExportOverdueMeansTwoIntervalsWithPendingSnapshots` (wie `TestOverdueMeansTwoIntervalsWithoutAScheduledSnapshot`, backups_test.go:238) |
| „Kein Ziel" ist nie „ok" | `Enabled=false` liefert `Overdue=false` und die Tafel zeigt Grün | `TestNoTargetIsNeverShownAsHealthy` |
| Größengrenze | 6 GB-Snapshot wird begonnen | `TestASnapshotOverTheSinglePutLimitIsRefusedVisibly` |
| Uhr | Fehlerabbildung ohne `RequestTimeTooSkewed` | `TestClockSkewIsSaidInPlainWords` |
| Kein `InsecureSkipVerify`, kein Redirect | Redirect-Folgen eingeschaltet | `TestARedirectIsRefused`, `TestTLSVerificationIsNeverDisabled` (greppt den Quelltext und fährt einen TLS-Fake ohne CA) |
| Audit | `secret_access_key` in der Allowlist | `TestTheSecretIsNotInTheAuditArchive` |
| Alt-Snapshot ohne Prüfsumme | `SHA256 == ""` wird als Leerstring gesendet | `TestASnapshotWithoutAChecksumIsHashedBeforeUpload` |

Jede Zeile heißt: **zuerst Fehler einsetzen, Wächter rot sehen, Fehler zurücknehmen** (Methode des Projekts); der Executor führt es im Plan als eigenen Schritt mit dem Befehl und der roten Ausgabe.

**Linter:** `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...` [CITED: CONTEXT.md]. Die Projekt-Linterregeln zu `gosec` (z. B. `G107`/`G704` für `http.NewRequest` mit variabler URL, `G402` für TLS) greifen im neuen Client; Ausnahmen brauchen einen `//nolint:gosec // Begründung`-Kommentar wie im Bestand (`internal/talos/talosconfig.go:198`).

## Common Pitfalls

### Pitfall 1: Export im Backup-Job
**What goes wrong:** Der Upload hält den Cluster-Lease; ein Upgrade oder ein zweiter Snapshot meldet `ErrClusterBusy`, solange Stunden hochgeladen wird; der Upgrade-Snapshot wird nie exportiert.
**Why it happens:** „Nach dem Snapshot hochladen" liest sich wie ein zweiter Schritt desselben Jobs.
**How to avoid:** Abgleicher (Muster 1).
**Warning signs:** `backupStep` ruft den S3-Client; ein Test, der den Upgrade-Snapshot exportiert haben will, fehlt.

### Pitfall 2: „Hochgeladen" ohne „geprüft"
**What goes wrong:** Der Betreiber glaubt an eine geprüfte Kopie; das Ziel hat `x-amz-content-sha256` ignoriert (SeaweedFS), oder beide Header, und die Datei ist beschädigt.
**How to avoid:** `x-amz-checksum-sha256` immer senden, Antwort auf Echo prüfen, sonst HEAD, sonst nur Größe; `verified` ehrlich anzeigen.
**Warning signs:** `Receipt` ohne `Verified`; UI zeigt nur „exported".

### Pitfall 3: Fremdkörper im Fehler
**What goes wrong:** Ein Fehlertext enthält Zugriffsschlüssel, StringToSign oder interne Pfade, erscheint im Journal, in `reason` und im Browser.
**How to avoid:** feste Satztabelle, nur `<Code>` lesen, nie `err.Error()` des Transports durchreichen (er enthält die URL mit Host und Bucket; für den Host ist das akzeptabel, für Körper nicht).

### Pitfall 4: Schlüssel- und Präfix-Grammatik
**What goes wrong:** Ein Präfix mit Leerzeichen, `+`, `%` oder Unicode wird von manchen Servern anders kanonisiert als vom Client (siehe Befund 2, pathologischer Schlüssel); die Signatur stimmt nicht, der Fehler heißt `SignatureDoesNotMatch` und sieht wie ein falsches Geheimnis aus.
**How to avoid:** Präfix auf `[A-Za-z0-9._-/]` beschränken, kein `//`, kein `..`, kein führendes `/`; Cluster-ID und Dateiname sind durch `plainClusterID` und `parseSnapshotName` schon auf diese Grammatik festgelegt.

### Pitfall 5: Uhr des Pi
**What goes wrong:** Ohne NTP driftet die Uhr; ab etwa 15 Minuten Abstand scheitert jede Signatur mit 403, der Satz sagt „refused the key", der Betreiber tauscht den Schlüssel.
**How to avoid:** `RequestTimeTooSkewed` eigener Satz; Test mit verschobener Uhr.

### Pitfall 6: Nur Standardwerte eines Fremd-SDK
**What goes wrong:** Prüfsummen-Standardwerte ändern sich in einem SDK-Update, Fremdserver lehnen die Anfrage ab (belegt für `aws-sdk-go-v2`).
**How to avoid:** eigener Client (Entscheidung), und der Fake-Server prüft **jeden** Header, den der Client sendet.

### Pitfall 7: Sidecar veraltet oder verwaist
**What goes wrong:** `prune` löscht den Snapshot, der `.export.json` bleibt liegen (Platte füllt sich mit Dateien, die niemand mehr listet) oder verweist auf einen Snapshot, der nicht mehr da ist.
**How to avoid:** `prune` räumt die Sidecars mit; der Abgleicher räumt verwaiste Sidecars bei jedem Durchlauf; Test.

### Pitfall 8: Geheimnis kommt über die Antwort zurück
**What goes wrong:** Die Oberfläche liest die Einstellung zum Bearbeiten und bekommt das Geheimnis.
**How to avoid:** zwei Entities; `GET` lädt das Geheimnis nie; `PUT` ohne Geheimnis behält es.

### Pitfall 9: `go test -race` fehlt auf dem Pi
**What goes wrong:** Der Abgleicher teilt Zustand zwischen Tick, Poke und HTTP-Handlern; ohne `-race` lokal sieht man Datenrennen erst in der CI.
**How to avoid:** alle gemeinsamen Felder hinter einem Mutex, das Poke über einen gepufferten Kanal mit `select`/`default`; ausdrücklich im Bericht sagen, dass `-race` hier nicht lief.

### Pitfall 10: Budget-Tabelle vergessen
**What goes wrong:** Die Testroute macht einen Außenaufruf, hat aber keine Zeile in `cmd/holzkube-managerd/budget_test.go`; `TestEveryRouteThatReachesUpstreamHasABudgetRow` wird rot.
**How to avoid:** Zeile mit `routeDeadline` unterhalb von `writeTimeout − Slack`.

## Code Examples

### SigV4 für einen Anwendungsfall (Stdlib, gegen zwei echte Server und den AWS-Signer geprüft)

```go
// Source: eigener Spike dieser Recherche, gegen RustFS und SeaweedFS gelaufen und gegen
// github.com/aws/aws-sdk-go-v2/aws/signer/v4 (DisableURIPathEscaping) für die Schlüssel dieser
// Codebasis bit-identisch verglichen. In den Produktionscode gehört dieselbe Rechnung als
// kleine, getestete Funktion; die Region und das Dienstkürzel "s3" sind fest.
func hm(k []byte, s string) []byte { h := hmac.New(sha256.New, k); h.Write([]byte(s)); return h.Sum(nil) }

// uriEncode: unreservierte Zeichen bleiben, '/' bleibt im Pfad, alles andere %XX (Großbuchstaben).
func uriEncode(s string, keepSlash bool) string { /* wie im Spike */ }

func sign(req *http.Request, ak, sk, region, payloadHex string, now time.Time) {
    amz := now.UTC().Format("20060102T150405Z"); date := amz[:8]
    req.Header.Set("x-amz-date", amz)
    req.Header.Set("x-amz-content-sha256", payloadHex)
    // signierte Header: host, x-amz-* (und content-type/content-md5, falls gesetzt), sortiert, klein
    // canonical = METHOD \n uriEncode(Pfad) \n sortierte Query \n Header\n \n signedHeaders \n payloadHex
    scope := date + "/" + region + "/s3/aws4_request"
    // sts = "AWS4-HMAC-SHA256\n" + amz + "\n" + scope + "\n" + hex(sha256(canonical))
    // key = hm(hm(hm(hm("AWS4"+sk, date), region), "s3"), "aws4_request")
    // Authorization: AWS4-HMAC-SHA256 Credential=<ak>/<scope>, SignedHeaders=<..>, Signature=hex(hm(key, sts))
}
```

### Der Upload eines Snapshots (Skizze)

```go
// Source: eigener Entwurf nach Muster 2 und Befund 2. Werte: Header-Namen aus
// docs.aws.amazon.com/AmazonS3/latest/userguide/checking-object-integrity-upload.html.
func (c *Client) Put(ctx context.Context, o backupexport.Object) (backupexport.Receipt, error) {
    if o.Size > maxSinglePut { // 5_000_000_000
        return backupexport.Receipt{}, ErrTooLarge
    }
    sum, _ := hex.DecodeString(o.SHA256)                       // 32 Bytes
    req := c.newRequest(ctx, http.MethodPut, o.Key, o.Body, o.Size)
    req.Header.Set("x-amz-checksum-sha256", base64.StdEncoding.EncodeToString(sum))
    c.sign(req, o.SHA256)                                      // x-amz-content-sha256 = hex
    resp, err := c.do(req)                                     // requireDeadline hier, einmal
    // ... Klassifizieren: nur <Code> aus dem Körper, nie weiterreichen
    // verified: resp.Header.Get("x-amz-checksum-sha256") == base64 -> "response", sonst HEAD, sonst Größe
}
```

### Wächter vom Kontext ohne Frist aus (Form wie Phase 15)

```go
// Source: Muster aus .planning/phases/15-*/15-RESEARCH.md ("Wächter ohne Deadline"), hier für den Exporter.
func TestAnExporterOnAContextWithNoDeadlineStillExports(t *testing.T) {
    ctx := context.Background() // die Lage in main.go; ein 60-s-testContext sähe den Fehler nie
    // Fake-S3 starten, einen Snapshot ablegen, svc.Tick(ctx) rufen, Sidecar muss "exported" sagen.
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| MinIO als Referenz-S3 im LAN | MinIO Community: keine Images seit Oktober 2025, Wartungsmodus Dezember 2025, archiviert 2026 | 2025–2026 | Der Client darf sich nicht an MinIO binden; Garage, SeaweedFS, RustFS, Ceph sind die Gegenstücke im Test |
| SDK-Standard „Prüfsumme nur wenn nötig" | SDK-Standard „wenn unterstützt" (CRC32, Trailer) | `service/s3` v1.73/v1.74 (2025) | Fremdserver lehnen ab; ein eigener Client sendet nur, was geprüft ist |
| Prüfung über `Content-MD5` | `x-amz-checksum-sha256` (und CRC-Familie) als Header; S3 setzt sonst CRC-64/NVME | 2025 | Der Einzel-PUT kann SHA-256 selbst prüfen lassen |

**Deprecated/outdated:**
- `quay.io/minio/minio` als Quelle für einen Testserver: nicht mehr abrufbar (401, gemessen).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `aws-sdk-go-v2`, `minio-go/v7` als Paketnamen sind echt (aus Websuche und Wissen, `package-legitimacy` kennt Go nicht) | Standard Stack | Gering: sie werden nicht empfohlen |
| A2 | Die gz-Größen im Produkt-Binary entsprechen den Scratch-Messungen (eine Messung je Variante, kein Produktbuild) | Alternatives Considered | Mittel: das Delta kann kleiner oder größer sein; ändert die Empfehlung nicht |
| A3 | Ein abgebrochener PUT hinterlässt am Ziel kein Teilobjekt (S3-Semantik, nur der Prüfsummenfall wurde gemessen) | Befund 1 | Mittel: ein halbes Objekt unter dem Schlüssel; der Sidecar sagt nicht `exported`, und ein neuer PUT ersetzt es |
| A4 | Es gibt Server, die beide Prüfsummen-Header ignorieren | Befund 2 | Gering: `verified: none/size` fängt es ohnehin |
| A5 | Das Rechenbeispiel der AWS-Seite liefert denselben Wert wie unser Vektor (die Seite war nicht auslesbar) | Befund 2 | Gering: Orakel war der AWS-Signer selbst |
| A6 | AWS, B2, Wasabi, R2 akzeptieren Pfadstil | Befund 2 | Mittel: für ein Cloud-Ziel ist `addressing=virtual` nötig; das Feld deckt es ab |
| A7 | `UNSIGNED-PAYLOAD` ist nur über TLS zulässig | Befund 2 | Gering: wird nicht verwendet |
| A8 | Manche Server lehnen `x-amz-server-side-encryption` ohne Schlüsseldienst ab | Befund 2 | Gering: Header wird nicht gesetzt |
| A9 | SigV4-Zeitfenster 15 Minuten | Befund 2 | Gering: nur ein Satz und ein Test mit verschobener Uhr |
| A10 | AWS S3 liefert Zugriffsschlüssel und StringToSign im Körper von `SignatureDoesNotMatch` | Befund 4 | Gering: die Regel „Körper nie weiterreichen" gilt unabhängig davon |
| A11 | Ein selbst signiertes Blattzertifikat im `RootCAs`-Pool genügt Go als Anker, wenn der SAN passt | Befund 2 | Mittel: ein Test mit `httptest.NewTLSServer` belegt es im Plan; ein Schlüsselfall mit echtem MinIO bleibt unbelegt |
| A12 | Takt 1 min, 3 Versuche je Durchlauf mit 10 s und 30 s, Abstand `retryGap`, 5-GB-Grenze, Budgetformel | Befund 4, Muster 4 | Gestaltungsfragen im Ermessen; leicht änderbar |
| A13 | Der Betreiber nutzt Klartext-HTTP nicht (Auswahl C2) | Open Questions | Mittel: sonst muss die Bestätigungsvariante gebaut werden |

## Open Questions

Alle als **Auswahl** an den Betreiber, jede mit Kosten und Nutzen und einer Empfehlung (CLAUDE.md: Fragen sind Auswahlen, über `AskUserQuestion`). Wo der Betreiber nicht antwortet, gilt die Empfehlung; die nicht markierten Punkte sind Ermessen und brauchen keine Rückfrage.

1. **C1 — Zusammenspiel von lokaler Aufbewahrung und Export** (Befund 4)
   - **A (Empfehlung): unabhängig, Verlust sichtbar.** Kosten: bei langem Ausfall und kleinem `keep` kann ein Wiederherstellungspunkt nie das Ziel erreichen. Nutzen: der Plattenwächter bleibt, der Pi läuft nie durch einen toten Export voll, die Warnung („export overdue") kommt vor dem ersten Verlust.
   - **B: Zurückhalten mit Deckel `keep + 3`.** Kosten: Änderung an `prune`, mehr Plattenbedarf, mehr Tests. Nutzen: kleinere Lücke.
   - **C: unbegrenzt zurückhalten.** Kosten: ein totes Ziel füllt die Platte, bis `guardDisk` Backups ablehnt. Nutzen: keine Lücke, solange Platz ist. **Nicht empfohlen.**

2. **C2 — Klartext-HTTP zum Ziel** (Befund 2)
   - **A (Empfehlung): nur `https://`.** Kosten: ein LAN-MinIO ohne TLS muss TLS bekommen (mit der CA-Option ist ein eigenes Zertifikat genug). Nutzen: der Snapshot, die gesamte etcd-Datenbank, geht nie unverschlüsselt über die Leitung, solange die Snapshot-Verschlüsselung Deferred ist.
   - **B: `http://` mit ausdrücklicher Bestätigung in der Maske und dauerhafter Warnung auf der Backups-Tafel.** Kosten: mehr Oberfläche und eine Lücke, die sich einschleichen kann. Nutzen: ein LAN-Ziel ohne TLS funktioniert sofort.

3. **C3 — Vertrauen für ein selbst signiertes Ziel** (Befund 2)
   - **A (Empfehlung): PEM-CA im Ziel.** Kosten: der Betreiber muss das Zertifikat oder die CA als Text einfügen; das Zertifikat braucht einen passenden SAN. Nutzen: eine Prüfung wie überall, überlebt Erneuerungen unter derselben CA, nichts Neues zu erklären.
   - **B: Fingerprint-Pin wie bei der Cluster-Übernahme.** Kosten: bricht bei jeder Erneuerung des Blattzertifikats und braucht einen Weg „Fingerprint zeigen und bestätigen". Nutzen: kein PEM nötig, passt zum bekannten Ablauf.
   - **C: beide.** Kosten: zwei Wege zu testen und zu erklären. Nutzen: deckt beide Betreiberfälle. Nur wenn der Betreiber es will.

4. **C4 — Schema-Version** (Befund 3)
   - **A (Empfehlung): kein Bump, die neuen Ordner legt `fsstore.Open` an.** Kosten: nach einem Zurückrollen auf eine ältere Binary hört der Export **still** auf (lokale Backups laufen weiter, die neue Binary holt den Rückstau nach). Nutzen: ein Zurückrollen macht den Manager nie unbenutzbar (das Update-Skript rollt die Binary, nicht das Verzeichnis); kein neues Fixture, keine Migration.
   - **B: Bump auf 8 mit Migration und Fixture `version-7/`** (wie Phase 15, D-08). Kosten: die ältere Binary startet nicht mehr, Rückweg ist das Backup-Tarball; ein Eintrag mehr im Leitfaden. Nutzen: ein Zurückrollen ist laut, nichts hört unbemerkt auf; folgt der Konvention „ein Ordner, eine Migration".

5. **C5 — S3-Client** (Befund 2; Empfehlung steht, die Alternative ist echt)
   - **A (Empfehlung): eigener Stdlib-Client.** Kosten: rund 300 Zeilen und ein eigener Beweis (Vektor, Fake, zwei echte Server wurden schon gemessen). Nutzen: keine neue Abhängigkeit, keine SDK-Standardwerte, der Client sendet genau, was geprüft ist.
   - **B: nur der AWS-Signer (`aws/signer/v4`) plus eigenes HTTP.** Kosten: 2 neue Module (`aws-sdk-go-v2`, `smithy-go`), `DisableURIPathEscaping` und die Pfadkodierung sind eine Falle; `+0,07 MB`. Nutzen: die Signatur ist vom AWS-Team gepflegt.
   - **C: volles SDK oder `minio-go`.** Kosten: 19–20 Module, rund +0,7 MB gz, Standardwerte fremder Hand. Nutzen: Multipart und Wiederholung fertig. **Nicht empfohlen.**

6. **C6 — Aufbewahrung am Ziel** (Befund 4)
   - **A (Empfehlung): nichts löschen; Lebenszyklusregel am Bucket.** Kosten: der Bucket wächst, bis der Betreiber eine Regel setzt. Nutzen: der Schlüssel braucht nur Schreiben und Lesen; ein gestohlener Pi kann die Kopien nicht löschen.
   - **B: die Aufbewahrungszahl des lokalen Zeitplans am Ziel spiegeln.** Kosten: `DeleteObject` und `ListBucket` für den Schlüssel, eine Löschlogik, die schiefgehen kann (falsches Präfix löscht fremde Objekte). Nutzen: der Bucket räumt sich selbst auf.

Ermessen ohne Rückfrage (Empfehlung steht in den Befunden): beide Snapshot-Arten exportieren; Rückstau neueste zuerst; Zielwechsel exportiert neu; „Ziel testen" schreibt und liest, löscht nichts; Zustand als Sidecar je Snapshot; kein Job in der Jobliste; Multipart ist nicht Teil der Phase, größere Snapshots werden sichtbar abgelehnt.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Bauen, Tests | ✓ | 1.27.2 lokal; `go.mod` verlangt `toolchain go1.26.9` (`GOTOOLCHAIN=go1.26.9` für den Linter) | — |
| Node/npm | Frontend, `task ci` | ✓ (`node` gefunden) | nicht abgefragt | — |
| `task` | `task ci` | ✗ auf diesem Mac | — | `./bin/task` laut CLAUDE.md; Go- und Web-Befehle einzeln laufen lassen |
| Docker | Messung gegen echte S3-Server (einmalig, nicht Teil der CI) | ✓ | Docker Desktop, Client 29.8.2 | Der Beweis im Repo läuft nur gegen den `httptest`-Fake |
| Echte S3-Server (RustFS, SeaweedFS, Garage) | optionale Handprobe auf dem Pi | ✓ per `docker pull` | — | — |
| `go test -race` | Datenrennen im Abgleicher | ✗ auf dem Pi (ThreadSanitizer lehnt den 47-Bit-Adressraum ab) | — | nur CI; im Bericht sagen |
| GitHub-Zugang für Push/Dispatch | Release | ✗ auf dem Pi | — | Sache des Betreibers |

**Missing dependencies with no fallback:** keine.
**Missing dependencies with fallback:** `task` (siehe oben); echte Ziele (AWS, B2, R2) sind von hier und vom Pi unerreichbar; ein echtes Ziel ist eine Handprobe des Betreibers und bleibt `human_needed`.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` (Go 1.26.9 Toolchain) und `vitest` (Web); Fake-S3 mit `net/http/httptest` |
| Config file | `go.mod`, `web/package.json`, `Taskfile.yml` |
| Quick run command | `go test ./internal/backupexport/... ./internal/upgrade/... ./internal/httpapi/... ./internal/metrics/... -count=1` |
| Full suite command | `./bin/task ci` (laut `Taskfile.yml`: web ci, lint:web, test:web, build, lint:go, test, vuln, test:layout, test:next) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| EXPORT-01 | Beide Snapshot-Arten werden exportiert, nach Neustart, ohne Lease | unit + Fake-S3 | `go test ./internal/backupexport -run 'BothKinds\|Restart'` | ❌ Wave 0 |
| EXPORT-02 | `Target`-Naht, kein HTTP in der Schnittstelle | unit | `go test ./internal/backupexport -run 'Target'` | ❌ Wave 0 |
| EXPORT-03 | Signatur gegen Vektor, Header, Körper, Pfadstil, Region, 5-GB-Grenze | unit + Fake-S3 | `go test ./internal/backupexport/s3 -count=1` | ❌ Wave 0 |
| EXPORT-04 | TLS mit CA, ohne CA abgelehnt, Redirect verweigert, nie `InsecureSkipVerify` | unit (`NewTLSServer`) | `go test ./internal/backupexport/s3 -run 'TLS\|Redirect'` | ❌ Wave 0 |
| EXPORT-05 | Geheimnis nie in Log, Fehler, Antwort, Metrik, Audit; schreibgeschützt | unit + HTTP | `go test ./internal/backupexport ./internal/httpapi ./internal/audit -run 'Secret'` | ❌ Wave 0 |
| EXPORT-06 | Wiederholung mit Grenze, Rückstau neueste zuerst, Zielwechsel, Verlust sichtbar | unit (Fake-Uhr) | `go test ./internal/backupexport -run 'Retry\|Backlog\|ChangedTarget\|Pruning'` | ❌ Wave 0 |
| EXPORT-07 | Überfällig, kein Ziel ist nie „ok", Serien | unit | `go test ./internal/backupexport ./internal/metrics -run 'Overdue\|NoTarget\|Export'` | ❌ Wave 0 |
| EXPORT-08 | Routen, Rollen, Sudo, Budget-Zeile, api-contract, Oberfläche | HTTP + vitest | `go test ./internal/httpapi ./cmd/holzkube-managerd -run 'Export\|Allowlist\|Budget'` und `npm --prefix web test` | ❌ Wave 0 |
| EXPORT-09 | Frist-Wächter auf `context.Background()`, jeder Anspruch rot gesehen | unit | siehe Tabelle „Injizierbare Fehler" | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/backupexport/... -count=1` plus das betroffene Paket.
- **Per wave merge:** die Quick-Run-Zeile.
- **Phase gate:** `./bin/task ci` grün, Linter lokal grün, jeder Wächter der Injektionstabelle rot gesehen und die rote Ausgabe im Bericht.

### Wave 0 Gaps
- [ ] `internal/backupexport/` (Paket, Fake-S3 `fakes3_test.go` mit Signatur-Verifizierer, eingechecktem Vektor und einstellbaren Fehlern) — EXPORT-01..07, 09
- [ ] `internal/backupexport/s3/` (Client, Signatur, Fehlerabbildung) — EXPORT-03, 04
- [ ] Fixture/Test der Entities (`internal/store/fsstore`, Test wie `configbaselines_test.go`) — EXPORT-05
- [ ] bei Auswahl C4 B: `internal/store/testdata/version-7/` und Eintrag in `internal/store/testdata/README.md`
- [ ] `docs/api-contract.md`-Einträge für jede neue Route und jeden neuen Problem-Code; `web/src/api.ts`-Schemata und `api.nulls.test.ts`
- [ ] Zeile in `cmd/holzkube-managerd/budget_test.go` für `POST /api/v1/backup-export/test`
- Framework-Install: keiner.

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no (keine neue Anmeldung; ausgehend authentifiziert SigV4) | Zugangsdaten schreibgeschützt, Admin + Sudo für die Einstellung |
| V3 Session Management | no | bestehende Sitzungen, Sudo-Fenster (`Destructive`) |
| V4 Access Control | yes | `RoleAdmin` für Einstellung, Test und Lesen; `RoleOperator` für „jetzt exportieren"; Audit |
| V5 Input Validation | yes | Server-Validierung von Endpunkt, Bucket, Region, Präfix, `addressing`, CA-PEM (nur Stdlib-Parser) |
| V6 Cryptography | yes | `crypto/hmac`, `crypto/sha256`, `crypto/tls`, `crypto/x509` der Stdlib; nichts selbst entworfen außer der protokollvorgegebenen SigV4-Rechnung (gegen unabhängiges Orakel geprüft) |
| V7 Error Handling and Logging | yes | feste Satztabelle, `Secret`-Typ, kein Fremdkörper in Log/Antwort |
| V8 Data Protection | yes | Geheimnis 0600 im Datenverzeichnis, eigene Entity; Snapshot enthält alle Kubernetes-Secrets (Hinweis; Verschlüsselung Deferred) |
| V9 Communications | yes | TLS-Prüfung immer an, eigene CA per PEM, kein Redirect, https erzwungen (C2) |
| V14 Configuration | yes | kein Standardziel, kein Ziel ohne ausdrückliche Einrichtung |

### Known Threat Patterns for Go + S3-Export

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Geheimnis in Log, Fehler, Antwort, Metrik, Audit | Information Disclosure | `Secret`-Typ mit `LogValue`, Allowlist-Audit, GET lädt das Geheimnis nie, Tests je Kanal |
| Fremdkörper (StringToSign, Zugriffsschlüssel) in Fehlern | Information Disclosure | nur `<Code>` lesen, feste Sätze |
| Mann in der Mitte, Snapshot im Klartext | Information Disclosure, Tampering | `https` erzwungen, Zertifikatsprüfung immer, CA statt Abschalten |
| Redirect schickt den Körper an einen anderen Host | Information Disclosure | Redirect verweigern |
| SSRF über den konfigurierbaren Endpunkt | Elevation, Information Disclosure | Admin + Sudo + Audit, `scheme://host[:port]` ohne Pfad/Userinfo, nicht die eigene Adresse |
| Beschädigte Kopie als „exportiert" gemeldet | Tampering | `x-amz-checksum-sha256`, Echo prüfen, `verified` anzeigen |
| Ausgehender Upload ohne Frist hängt den Abgleicher | Denial of Service | eigenes Budget je Aufruf, `requireDeadline`, Transport-Timeouts |
| Disk-Füllung durch Zurückhalten | Denial of Service | Option C1 A (kein Zurückhalten) oder Deckel; `guardDisk` bleibt |
| Falsche Uhr bricht SigV4 | Denial of Service | eigener Satz, Handprobe im Guide |
| Löschrecht im Schlüssel | Tampering | Minimalberechtigung (nur Put/Get), nichts am Ziel löschen (C6 A) |

## Sources

### Primary (HIGH confidence)
- Code dieses Repos, im Read gelesen: `internal/upgrade/backups.go` (665 Zeilen), `internal/upgrade/safetysnapshot.go` (426 Zeilen), `internal/store/migrate/migrate.go`, `internal/store/store.go`, `internal/store/fsstore/fsstore.go`, `internal/model/backup.go`, `internal/audit/redact.go`, `internal/httpapi/handlers/backups.go`, `internal/metrics/metrics.go`, `internal/talos/pin.go`, `internal/imagefactory/client.go` (Ausschnitte), `internal/k8srelease/k8srelease.go` (Ausschnitte), `cmd/holzkube-managerd/main.go` (Ausschnitte), `cmd/holzkube-managerd/allowlist_test.go`, `cmd/holzkube-managerd/budget_test.go` (Kopf), `Taskfile.yml`, `go.mod`, `.goreleaser.yaml`, `.planning/phases/20-backup-export/20-CONTEXT.md`, `.planning/phases/15-*/15-RESEARCH.md`, `CLAUDE.md`
- Messungen dieser Session: Scratchpad `e_base`, `e_aws`, `e_minio`, `e_signer2` (Binärgrößen), `e_spike` (Signatur und Verhalten gegen RustFS und SeaweedFS), `e_signer` (Orakelvergleich); `gh release view v0.8.0 --json assets`
- docs.aws.amazon.com/AmazonS3/latest/userguide/upload-objects.html (5-GB-Einzel-PUT), `qfacts.html` (Multipart-Grenzen), `checking-object-integrity-upload.html` (Prüfsummen-Header, Ablehnung bei Abweichung)

### Secondary (MEDIUM confidence)
- etcd.io/docs/v3.5/dev-guide/limit/ und docs.siderolabs.com/talos/v1.14/build-and-extend-talos/cluster-operations-and-maintenance/etcd-maintenance (2 GiB Standard, 8 GiB empfohlenes Maximum)
- gitlab.com/gitlab-org/gitlab-runner/-/merge_requests/6691 und docs.aws.amazon.com/sdk-for-go/v2/developer-guide/s3-checksums.html (SDK-Standardwerte für Prüfsummen, Fremdanbieter)
- itsfoss.com/news/minio-moves-away-from-open-source, blog.vonng.com/en/db/minio-is-dead, linuxiac.com/minio-ends-active-development (Stand von MinIO; Daten der Quellen widersprechen sich leicht)

### Tertiary (LOW confidence)
- Trainingswissen, siehe Assumptions Log A1, A3 bis A13

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — Messung, Orakelvergleich und zwei echte Server; Entscheidung hängt nicht an einer Bibliothek
- Architecture: HIGH — Hook, Job- und Lease-Verhalten, Persistenz und Muster im Code gelesen; Abgleicher folgt `configdrift`
- Pitfalls: HIGH für das Gemessene (Prüfsummen-Verhalten, Signatur, Größen), MEDIUM für Anbieterverhalten (AWS, B2, R2) und echte TLS-Ziele

**Research date:** 2026-10-16
**Valid until:** 2026-11-15 (die S3-Fremdserver und MinIOs Lage ändern sich; Machinery-Pin und Repo-Stand ändern sich mit jedem Release)
