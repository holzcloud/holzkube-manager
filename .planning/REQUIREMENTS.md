# Requirements: holzkube

**Defined:** 2026-08-27
**Core Value:** Eine neue Maschine wird komplett in der UI zum Cluster-Node — ohne `talosctl`, ohne Omni.

> **Abnahmetest für v1 (binär):** Ein blanker Mini-PC wird ein gesunder Cluster-Node, ohne dass der Betreiber ein Terminal öffnet.

Die Requirements folgen der Phasenstruktur aus `.planning/research/SUMMARY.md`. Requirements mit **🚫** sind dort als *Release-Blocker* markiert — nicht als Nice-to-have.

---

## v1 Requirements

### Foundation (FOUND)

- [x] **FOUND-01**: Betreiber startet ein einzelnes Binary; die Web-UI ist eingebettet, keine Runtime-Dependencies nötig
- [x] **FOUND-02**: Betreiber meldet sich mit Benutzername und Passwort an (argon2id, Parameter mitgespeichert); die Session wird beim Login rotiert
- [x] **FOUND-03**: Destruktive Aktionen verlangen erneute Passwort-Eingabe (sudo-mode), auch bei bestehender Session
- [x] **FOUND-04**: Login ist gegen Brute-Force ratenbegrenzt
- [x] **FOUND-05**: Server bindet standardmäßig auf `127.0.0.1` und spricht HTTPS mit selbst erzeugtem Zertifikat, wenn keines konfiguriert ist
- [x] **FOUND-06**: Jede Mutation landet im Audit-Log (JSONL, täglich rotiert, Hash-Kette) — Intent vor der Aktion, Outcome danach
- [x] **FOUND-07**: Aller Zustandszugriff läuft über ein Entity-Interface (`store.Machines().Get(...)`); Dateipfade sind oberhalb der Store-Implementierung unsichtbar
- [x] **FOUND-08**: Gleichzeitige Schreibvorgänge korrumpieren keinen Zustand (atomare Writes, `flock`, Per-Entity-Mutex, `rev`-CAS)
- [x] **FOUND-09**: Zustand trägt eine Schema-Version; Upgrades migrieren vorwärts und legen vorher ein Backup an
- [x] **FOUND-10**: Alle Zustandsdateien liegen mit `0600` in einem `0700`-Verzeichnis; falsche Rechte werden beim Start bemängelt
- [x] **FOUND-11**: API-Fehler kommen als RFC-9457 `problem+json` mit einer stabilen Fehlertaxonomie zurück
- [x] **FOUND-12**: Betreiber kann das gesamte Binary mit `--dry-run` fahren; keine Mutation erreicht einen Node

### Transport & Sandbox (TRANS)

- [x] **TRANS-01**: holzkube spricht die Talos machine API direkt per gRPC/mTLS an Node-IPs
- [x] **TRANS-02**: Der Transport liegt hinter **zwei** Interfaces — `Dialer` (Identität → dialbare Adresse) und `DiscoverySource` (Scan / manuell / Tunnel-Registrierung) — damit ein Tunnel-Transport später nachrüstbar ist, obwohl er die Kontaktrichtung umkehrt
- [x] **TRANS-03**: Cluster-Clients (mTLS) und Maintenance-Clients (unauthentifiziert) sind getrennte Typen und nicht verwechselbar
- [x] **TRANS-04**: Jeder Node-Aufruf hat ein erzwungenes Deadline; Retries gibt es nur für eine Allowlist lesender Operationen
- [x] **TRANS-05**: Ein nicht erreichbarer Node blockiert weder die UI noch andere Nodes (Circuit Breaker, Fan-out pro Node)
- [x] **TRANS-06** 🚫: `talossim` — ein In-Process-Fake-Talos-Node mit echten Protobufs, echtem mTLS und echtem In-Memory-COSI-State, gegen den der unveränderte Produktions-Client spricht
- [x] **TRANS-07**: `talossim` kann Fehlerszenarien skripten: `go_silent(90s)`, `reject_apply`, `second_bootstrap_returns_AlreadyExists`, `flap_connection`, `slow_log_consumer`, `ip_changes_on_reboot`, `etcd_down`, `k8s_down`, `version_out_of_supported_range`
- [ ] **TRANS-08**: Contract-Tests laufen gegen Fake und gegen echten Talos, damit Fake-Drift auffällt

### Image Factory (FACT)

- [x] **FACT-01**: Betreiber stellt ein Schematic zusammen — System-Extensions, Kernel-Args, META — über einen versions-skopierten Extension-Katalog, kein Freitextfeld
- [x] **FACT-02**: Extension-Namen werden **vor** dem POST validiert, und das Schematic gilt erst als brauchbar, nachdem ein Model-Build-Probe es bestätigt hat (ein POST liefert `200` auch für nicht existierende Extensions)
- [x] **FACT-03**: Betreiber bekommt die exakten URLs für ISO, Installer und PXE, mit korrekt aufgelöstem, versionsabhängigem Installer-Repo-Namen und ohne hartkodierte Architektur
- [x] **FACT-04**: Beim Autoren eines Schematics mit Kernel-Args oder META warnt die UI, dass `installer`/`initramfs` **nur** System-Extensions ehren — ISO und installiertes System driften sonst auseinander
- [x] **FACT-05**: Pre-Release-Versionen sind herausgefiltert und nur explizit wählbar; bekannt kaputte Versionen sind ausgegraut
- [x] **FACT-06**: Die Schematic-ID wird lokal vorab berechnet und persistiert

### Inventar, Import & Health (INV)

- [ ] **INV-01** 🚫: Betreiber importiert seinen **bestehenden** Cluster — Secrets-Bundle von einem Control-Plane-Node, Fingerprint-Bestätigung, plus Konnektivitätsbeweis mit einem frisch ausgestellten Zertifikat
- [ ] **INV-02**: Betreiber kann alternativ einen neuen Cluster anlegen; `gen secrets` ist ausschließlich aus diesem Pfad erreichbar
- [ ] **INV-03** 🚫: Node-Records sind flach und **UUID-adressiert** (`SystemInformation.UUID`), mit `cluster_id` als nullbarem Feld — DHCP verschiebt IPs
- [ ] **INV-04** 🚫: Jeder Node-Record trägt seine Image-Factory-`schematic_id` ab dem ersten Schema — ohne sie sind Upgrades nicht sicher auslieferbar
- [ ] **INV-05** 🚫: Die Restlaufzeit des talosconfig-Client-Zertifikats ist sichtbar und warnt rechtzeitig (Talos rotiert Client-Certs **nicht**; Ablauf sperrt alle Nodes gleichzeitig aus)
- [ ] **INV-06**: Dashboard zeigt pro Node Health, Talos-Version, Kubernetes-Version, CPU/RAM, Disks, Netzwerk-Interfaces und Service-Status
- [ ] **INV-07** 🚫: Jedes gelesene Feld trägt sein `HealthLevel` und `stale_since`; Daten die nur `:6443` bräuchten, machen NODE-Level-Daten nicht unsichtbar
- [ ] **INV-08**: Bei totem Cluster bleibt die UI ehrlich — Node-Level-Daten weiter sichtbar, veraltete Werte als veraltet markiert, nie eine leere Seite
- [ ] **INV-09**: Ein unerreichbarer Node führt nie dazu, dass sein Inventar-Record gelöscht wird
- [ ] **INV-10**: Cluster-Übersicht zeigt Control-Plane vs Worker, etcd-Member und Cluster-Health
- [ ] **INV-11**: Die Talos↔Kubernetes-Kompatibilitätsmatrix liegt als Daten vor, inklusive "Abstand zum Rand"
- [ ] **INV-12**: Betreiber kann einen Cluster read-only sperren; gesperrte Cluster nehmen keine Mutationen an
- [ ] **INV-13**: Node-Status kommt aus COSI-Watches mit gejittertem Heartbeat, nicht aus blindem Polling

### Jobs & Node-Aktionen (JOB)

- [ ] **JOB-01**: Langlaufende Operationen sind persistierte Jobs; ein Neustart des Prozesses setzt sie fort oder parkt sie für einen Menschen
- [ ] **JOB-02**: Jeder Schritt mit Seiteneffekt hat eine gepaarte Read-only-"ist es passiert?"-Abfrage; Schritte ohne eine solche werden nie automatisch retryt
- [ ] **JOB-03**: Pro Cluster darf immer nur ein mutierender Job laufen (Lease)
- [ ] **JOB-04**: Betreiber kann einen Job an einer Schrittgrenze abbrechen; der Fortschritt ist live sichtbar
- [ ] **JOB-05**: Betreiber rebootet einen Node aus der UI
- [ ] **JOB-06**: Betreiber fährt einen Node herunter
- [ ] **JOB-07** 🚫: Reset zeigt vor dem Ausführen die Disks, die Wipe-Scope-Wahl und die Reboot-Kontrolle, stellt die **effektiven Flags** dar und verlangt das Tippen des Hostnamens — `talosctl reset` ist per Default maximal destruktiv (`--wipe-mode=all`, `--reboot=false`)
- [ ] **JOB-08**: Bestätigungen werden serverseitig durchgesetzt (Confirmation-Token), nicht nur im Browser
- [ ] **JOB-09**: Destruktive Endpoints antworten mit `202 Accepted` und einer Job-ID

### Konfiguration (CFG)

- [ ] **CFG-01**: Betreiber sieht die MachineConfig eines Nodes gerendert und roh
- [ ] **CFG-02** 🚫: Secrets sind serverseitig redigiert, bevor sie das Backend verlassen — dieselbe `redact`-Funktion für Config-View, Raw-Tab, Diff, API-Response und Audit-Log (`.machine.ca.key` steckt in der Antwort)
- [ ] **CFG-03**: Betreiber legt wiederverwendbare Config-Patches an; der Patch-Store ist versioniert und append-only (nur Strategic Merge, kein RFC 6902)
- [ ] **CFG-04**: Vor dem Anwenden zeigt holzkube ein **strukturelles** Diff der tatsächlich gemergten, gerenderten Configs — inklusive Listen-Wachstum und Duplikat-Erkennung
- [ ] **CFG-05**: Ein Patch zweimal angewendet erzeugt dasselbe Ergebnis (Idempotenz-Test pro Patch)
- [ ] **CFG-06**: Der nötige Apply-Modus wird aus der verifizierten Allowlist berechnet und dem Betreiber angezeigt
- [ ] **CFG-07**: Änderungen an `.machine.install` werden als "wirkt erst beim nächsten Install/Upgrade" gekennzeichnet — sie melden Erfolg und ändern nichts
- [ ] **CFG-08**: Ein zweiter `staged`-Apply wird abgelehnt, statt den ersten still zu verwerfen
- [ ] **CFG-09**: Bei Diffs an `.machine.network` empfiehlt die UI `--mode=try` mit sichtbarem 60-Sekunden-Countdown
- [ ] **CFG-10**: Betreiber kann eine Config validieren und im Dry-Run prüfen, bevor er sie anwendet
- [ ] **CFG-11**: Config-Generierung nutzt das importierte Secrets-Bundle und einen pro Cluster gepinnten Talos-Versions-Contract

### Provisioning — der Core Value (PROV)

- [ ] **PROV-01**: Betreiber entdeckt Maschinen per Subnetz-Scan auf `:50000` mit begrenzter Nebenläufigkeit, plus manueller IP-Eingabe
- [ ] **PROV-02**: Ein bereits konfigurierter Node an der Ziel-IP wird als solcher gemeldet — nie als "nichts gefunden"
- [ ] **PROV-03**: Vor dem Anwenden zeigt holzkube UUID, MACs, Disks und Talos-Version der Maschine zur Identifikation
- [ ] **PROV-04**: Optionales `--cert-fingerprint`-Pinning wird angeboten, mit ehrlichem UI-Text dass Maintenance-Mode unauthentifiziert ist und der Fingerprint nur von der physischen Konsole kommt
- [ ] **PROV-05** 🚫: Unmittelbar vor dem Apply wird die UUID erneut verifiziert — die falsche Maschine zu treffen wischt Daten
- [ ] **PROV-06**: Betreiber wählt Rolle, Ziel-Cluster und Install-Disk; der Disk-Picker zeigt Größe, Modell, Seriennummer, Transport und markiert die System-Disk
- [ ] **PROV-07**: Bei geradzahliger Control-Plane-Anzahl warnt die UI (Quorum braucht 1/3/5)
- [ ] **PROV-08**: `.machine.install.image` wird automatisch aus **derselben** Schematic-ID gefüllt wie die ISO — sonst verschwinden die Extensions beim Install
- [ ] **PROV-09** 🚫: Die Wiederauftauch-Prüfung nach dem Disk-Install ist eine Drei-Wege-Probe mit verstrichener Zeit gegen ein erwartetes Budget — nie ein Spinner
- [ ] **PROV-10** 🚫: Doppelter etcd-Bootstrap ist strukturell unmöglich (Pre-Flight `EtcdMemberList`, `O_CREAT|O_EXCL`-Lease, fsynced Intent-Record, Talos' `AlreadyExists`), mit eigenem Recovery-Flow für den unklaren Fall
- [ ] **PROV-11**: Der Provisioning-Zustand wird pro Maschine persistiert; ein geschlossener Browser-Tab verliert den Job nicht
- [ ] **PROV-12**: Beim Wizard-Start warnt die UI, dass eine vorhandene Disk-Installation die ISO überschattet, und dass ohne DHCP kein Zero-Touch-Pfad existiert
- [ ] **PROV-13**: "Talos healthy, Kubernetes NotReady" wird vor der CNI-Installation als normal erklärt, nicht rot dargestellt

### Streaming (STREAM)

- [ ] **STREAM-01**: Betreiber sieht Logs und `dmesg` eines Nodes live in der UI
- [ ] **STREAM-02**: Pro Tab läuft **eine** multiplexte SSE-Verbindung mit `Last-Event-ID`-Replay — nicht eine pro Panel (HTTP/1.1 deckelt bei ~6 Verbindungen pro Origin)
- [ ] **STREAM-03**: Ein langsamer Browser blockiert nie den Upstream-Reader; verworfene Daten werden als sichtbare Lücke markiert
- [ ] **STREAM-04**: Der Stream-Zustand ist explizit sichtbar — live / reconnecting / Node rebootet / getrennt

### Upgrades & etcd (UPG)

- [ ] **UPG-01**: Betreiber fährt ein rollendes Talos-Upgrade über `LifecycleClient.Upgrade` (streaming), Node für Node
- [ ] **UPG-02** 🚫: Das Health-Gate schließt Learner aus, prüft Raft-Konvergenz und Alarme, verweigert bei ≤2 stimmberechtigten Membern, zeigt seine Eingaben an und wird **vor jedem** Node neu bewertet
- [ ] **UPG-03** 🚫: Ein Node mit unbekannter `schematic_id` wird nicht geupgradet; holzkube bietet an, sie vom Node zu lesen
- [ ] **UPG-04**: Kernel-Args-/GRUB-Drift blockiert den Ein-Klick-Pfad — die Upgrade-RPC kann Kernel-Args strukturell nicht tragen, also gibt es zwei Quellen der Wahrheit die synchron bleiben müssen
- [ ] **UPG-05**: Die nötige Kette an Zwischen-Minor-Versionen wird berechnet; es gibt keinen "latest"-Knopf und Pre-Releases sind gefiltert
- [ ] **UPG-06** 🚫: Ein Kubernetes-Forward-Kompatibilitäts-Gate blockiert Talos-Upgrades, die den Cluster stranden würden
- [ ] **UPG-07** 🚫: Nach jedem Upgrade wird deklariert-gegen-beobachtet verifiziert — "die API sagte OK" gilt nicht als Beweis
- [ ] **UPG-08**: Betreiber kann nach dem aktuellen Node stoppen; der UI-Text sagt ehrlich, was schon passiert ist
- [ ] **UPG-09**: Betreiber fährt ein Kubernetes-Upgrade
- [ ] **UPG-10**: Betreiber listet etcd-Member mit Hostname und UUID — nie mit rohen Hex-IDs
- [ ] **UPG-11**: Betreiber entfernt ein etcd-Member
- [ ] **UPG-12**: Betreiber zieht einen etcd-Snapshot, mit dokumentiertem Fallback ohne Quorum
- [ ] **UPG-13**: Betreiber entfernt einen Node aus dem Cluster (cordon/drain → reset → aus dem Inventar)
- [ ] **UPG-14**: Betreiber kann einen Node sperren, damit Upgrades ihn überspringen

### Betrieb & Härtung (OPS)

- [ ] **OPS-01**: Betreiber sichert und restauriert den holzkube-Zustand per Subcommand
- [ ] **OPS-02**: Betreiber verifiziert die Integrität der Audit-Hash-Kette
- [ ] **OPS-03**: Die unterstützte Talos-Versionsrange (v1.12–v1.14) wird als getesteter Check durchgesetzt; RCs sind Opt-in
- [ ] **OPS-04**: holzkube läuft als Docker-/Compose-Setup mit non-root-Container und korrekten Volume-Rechten
- [ ] **OPS-05** 🚫: Ein Verifikationsdurchlauf auf echter amd64-Hardware ist erfolgt

---

## v2 Requirements

Anerkannt, aber verschoben. Nicht in der aktuellen Roadmap.

### Transport

- **V2-TRANS-01**: SideroLink-artiger Tunnel-Transport, falls Nodes je das LAN verlassen (Interface ist reserviert)

### Provisioning

- **V2-PROV-01**: PXE/netboot — die PXE-Frontend-URL der Image Factory macht es zu einem URL-Tausch, wenn Schematics jetzt sauber modelliert sind
- **V2-PROV-02**: BMC/IPMI/Redfish-Power-Control, falls die Hardware je BMCs bekommt

### Zugang

- **V2-AUTH-01**: OIDC/SSO gegen einen bestehenden IdP (Interface ist reserviert)
- **V2-AUTH-02**: Mehrere Benutzer mit Rollen

### Schnittstellen

- **V2-API-01**: `holzkubectl` CLI gegen dieselbe REST-API
- **V2-API-02**: Prometheus-`/metrics`-Endpoint — exportieren, nie ingesten

### Betrieb

- **V2-OPS-01**: Support-Bundle-Export (`talosctl support`-Äquivalent)
- **V2-OPS-02**: CA-Rotation (der Radius mit dem breitesten stillen Schaden im Produkt — Sichtbarkeit des Ablaufs kommt in v1, Rotation nicht)
- **V2-OPS-03**: ARM64/SBC-Support — Schematics sind arch-parametrisiert, aber ungetestet

---

## Out of Scope

| Feature | Grund |
|---------|-------|
| Workload-Management (Deployments, Pods, Helm) | k9s/Lens/Headlamp lösen das. Die Naht ist der Port: `:50000` gehört holzkube, `:6443` den anderen |
| Eigene Metrics-/Monitoring-Pipeline | Keine Prometheus-Konkurrenz. Nur was die Talos-API direkt liefert |
| Betrieb im Cluster selbst | Bewusst außerhalb — ein Management-Tool das mit dem Cluster stirbt ist genau im Fehlerfall nutzlos |
| Managed-Kubernetes-Provider (EKS/GKE/AKS) | Talos-only |
| Generischer YAML-Editor ohne Leitplanken | Der Wert liegt in Diff, Validierung und Apply-Modus-Berechnung, nicht im Textfeld |
| RBAC | Single-Operator-Tool. Ein zweiter echter Benutzer wäre der Auslöser, nicht Vollständigkeit |
| Read-only-Dashboard als v1-Ziel | `talos-pilot` löst das bereits im Terminal. Der verteidigbare Boden sind Provisioning, Upgrades, Inventar und Patches |

---

## Traceability

Gefüllt bei der Roadmap-Erstellung. Quelle: `.planning/ROADMAP.md`.

**Phasen-Legende:**

- Phase 1: Foundation Skeleton
- Phase 2: Transport Seam, `talossim` & Image Factory
- Phase 3: Inventar, Cluster-Import & Health
- Phase 4: Walking Skeleton (Wegwerf) — *trägt absichtlich kein Requirement (Wegwerf-Gerüst)*
- Phase 5: Streaming — *designierte Schnittlinie*
- Phase 6: Jobs-Engine & Node-Aktionen
- Phase 7: Config-Domain
- Phase 8: Provisioning — der Core Value
- Phase 9: Upgrades & etcd-Verwaltung
- Phase 10: Härtung & echter Hardware-Durchlauf

Requirements mit **🚫** sind Release-Blocker.

| Requirement | Phase | Status |
|-------------|-------|--------|
| FOUND-01 | Phase 1 | Complete |
| FOUND-02 | Phase 1 | Complete |
| FOUND-03 | Phase 1 | Complete |
| FOUND-04 | Phase 1 | Complete |
| FOUND-05 | Phase 1 | Complete |
| FOUND-06 | Phase 1 | Complete |
| FOUND-07 | Phase 1 | Complete |
| FOUND-08 | Phase 1 | Complete |
| FOUND-09 | Phase 1 | Complete |
| FOUND-10 | Phase 1 | Complete |
| FOUND-11 | Phase 1 | Complete |
| FOUND-12 | Phase 2 | Complete |
| TRANS-01 | Phase 2 | Complete |
| TRANS-02 | Phase 2 | Complete |
| TRANS-03 | Phase 2 | Complete |
| TRANS-04 | Phase 2 | Complete |
| TRANS-05 | Phase 2 | Complete |
| **TRANS-06** 🚫 | Phase 2 | Complete |
| TRANS-07 | Phase 2 | Complete |
| TRANS-08 | Phase 3 | Pending |
| FACT-01 | Phase 2 | Complete |
| FACT-02 | Phase 2 | Complete |
| FACT-03 | Phase 2 | Complete |
| FACT-04 | Phase 2 | Complete |
| FACT-05 | Phase 2 | Complete |
| FACT-06 | Phase 2 | Complete |
| **INV-01** 🚫 | Phase 3 | Pending |
| INV-02 | Phase 3 | Pending |
| **INV-03** 🚫 | Phase 3 | Pending |
| **INV-04** 🚫 | Phase 3 | Pending |
| **INV-05** 🚫 | Phase 3 | Pending |
| INV-06 | Phase 3 | Pending |
| **INV-07** 🚫 | Phase 3 | Pending |
| INV-08 | Phase 3 | Pending |
| INV-09 | Phase 3 | Pending |
| INV-10 | Phase 3 | Pending |
| INV-11 | Phase 3 | Pending |
| INV-12 | Phase 3 | Pending |
| INV-13 | Phase 3 | Pending |
| STREAM-01 | Phase 5 | Pending |
| STREAM-02 | Phase 5 | Pending |
| STREAM-03 | Phase 5 | Pending |
| STREAM-04 | Phase 5 | Pending |
| JOB-01 | Phase 6 | Pending |
| JOB-02 | Phase 6 | Pending |
| JOB-03 | Phase 6 | Pending |
| JOB-04 | Phase 6 | Pending |
| JOB-05 | Phase 6 | Pending |
| JOB-06 | Phase 6 | Pending |
| **JOB-07** 🚫 | Phase 6 | Pending |
| JOB-08 | Phase 6 | Pending |
| JOB-09 | Phase 6 | Pending |
| CFG-01 | Phase 7 | Pending |
| **CFG-02** 🚫 | Phase 7 | Pending |
| CFG-03 | Phase 7 | Pending |
| CFG-04 | Phase 7 | Pending |
| CFG-05 | Phase 7 | Pending |
| CFG-06 | Phase 7 | Pending |
| CFG-07 | Phase 7 | Pending |
| CFG-08 | Phase 7 | Pending |
| CFG-09 | Phase 7 | Pending |
| CFG-10 | Phase 7 | Pending |
| CFG-11 | Phase 7 | Pending |
| PROV-01 | Phase 8 | Pending |
| PROV-02 | Phase 8 | Pending |
| PROV-03 | Phase 8 | Pending |
| PROV-04 | Phase 8 | Pending |
| **PROV-05** 🚫 | Phase 8 | Pending |
| PROV-06 | Phase 8 | Pending |
| PROV-07 | Phase 8 | Pending |
| PROV-08 | Phase 8 | Pending |
| **PROV-09** 🚫 | Phase 8 | Pending |
| **PROV-10** 🚫 | Phase 8 | Pending |
| PROV-11 | Phase 8 | Pending |
| PROV-12 | Phase 8 | Pending |
| PROV-13 | Phase 8 | Pending |
| UPG-01 | Phase 9 | Pending |
| **UPG-02** 🚫 | Phase 9 | Pending |
| **UPG-03** 🚫 | Phase 9 | Pending |
| UPG-04 | Phase 9 | Pending |
| UPG-05 | Phase 9 | Pending |
| **UPG-06** 🚫 | Phase 9 | Pending |
| **UPG-07** 🚫 | Phase 9 | Pending |
| UPG-08 | Phase 9 | Pending |
| UPG-09 | Phase 9 | Pending |
| UPG-10 | Phase 9 | Pending |
| UPG-11 | Phase 9 | Pending |
| UPG-12 | Phase 9 | Pending |
| UPG-13 | Phase 9 | Pending |
| UPG-14 | Phase 9 | Pending |
| OPS-01 | Phase 10 | Pending |
| OPS-02 | Phase 10 | Pending |
| OPS-03 | Phase 10 | Pending |
| OPS-04 | Phase 10 | Pending |
| **OPS-05** 🚫 | Phase 10 | Pending |

**Verteilung pro Phase:**

| Phase | Requirements | davon Release-Blocker |
|-------|--------------|-----------------------|
| Phase 1 — Foundation Skeleton | 11 | 0 |
| Phase 2 — Transport Seam, `talossim` & Image Factory | 14 | 1 |
| Phase 3 — Inventar, Cluster-Import & Health | 14 | 5 |
| Phase 4 — Walking Skeleton (Wegwerf) | 0 | 0 |
| Phase 5 — Streaming | 4 | 0 |
| Phase 6 — Jobs-Engine & Node-Aktionen | 9 | 1 |
| Phase 7 — Config-Domain | 11 | 1 |
| Phase 8 — Provisioning — der Core Value | 13 | 3 |
| Phase 9 — Upgrades & etcd-Verwaltung | 14 | 4 |
| Phase 10 — Härtung & echter Hardware-Durchlauf | 5 | 1 |
| **Summe** | **95** | **16** |

**Coverage:**

- v1 requirements: 95 total
- Mapped to phases: 95
- Unmapped: 0 ✓
- Doppelt abgebildet: 0 ✓
- Release-Blocker (🚫): 16 von 16 einer Phase zugeordnet ✓

---
*Requirements defined: 2026-08-27*
*Last updated: 2026-08-27 after roadmap creation (Traceability gefüllt)*
