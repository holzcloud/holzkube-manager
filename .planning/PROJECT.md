# holzkube

## What This Is

holzkube ist eine self-hosted Management-Oberfläche für Kubernetes-Cluster auf Talos Linux — eine Alternative zu Sidero Omni, ohne Vendor-Bindung und ohne SaaS. Ein einzelnes Go-Binary läuft **außerhalb** des Clusters, spricht die Talos machine API direkt per gRPC an und stellt eine Web-UI bereit, mit der Nodes überwacht, konfiguriert, geupgradet und von blankem Blech aus neu provisioniert werden.

Gebaut für Homelab-/Lab-Betreiber, die Talos fahren und die volle Kontrolle über ihr Management-Tooling behalten wollen. Primärer Nutzer: der Autor selbst.

## Core Value

**Eine neue Maschine wird komplett in der UI zum Cluster-Node — ohne `talosctl`, ohne Omni.**

Wenn alles andere scheitert: der Weg blanke Maschine → Image → Boot → Discovery → Config → Cluster-Join muss in der Oberfläche funktionieren.

## Requirements

### Validated

Milestone v1.14, 2026-09-11. Every line below is built and executed against
`internal/talossim`; what is *not* validated is named in the section after the
list, and in `.planning/WINDOWS.md`.

**Inventar & Sichtbarkeit** *(Phase 3, Phase 5)*
- [x] Nodes und Cluster werden als persistentes Inventar geführt (multi-cluster-fähiges Datenmodell)
- [x] Dashboard zeigt pro Node: Health, Talos-Version, Kubernetes-Version, CPU/RAM, Disks, Netzwerk-Interfaces
- [x] Service-Status pro Node sichtbar (etcd, kubelet, apid, machined, trustd)
- [x] Live-Streaming von Logs und `dmesg` pro Node in der UI
- [x] Cluster-Übersicht: Control-Plane vs Worker, etcd-Member, Cluster-Health

**Konfiguration** *(Phase 7)*
- [x] MachineConfig eines Nodes in der UI ansehen (rendered + raw YAML) - **immer redigiert**, ohne Schalter dahinter
- [x] Config-Patches erstellen, versionieren und wiederverwenden - append-only, nur Strategic Merge
- [x] Diff-Ansicht vor dem Anwenden - strukturell, über die tatsächlich gemergten Dokumente
- [x] Patch anwenden mit Wahl des Apply-Modus - der Modus wird aus den geänderten Pfaden **berechnet**, nicht frei gewählt
- [x] Dry-Run vor dem Anwenden - als Transport-Interceptor, nicht als UI-Zustand

**Lifecycle** *(Phase 6, Phase 8, Phase 9)*
- [x] Node-Aktionen: Reboot, Shutdown, Reset - jede als Job mit server-seitiger, an die Parameter gebundener Bestätigung
- [x] Cluster bootstrappen - mit vier unabhängigen Mechanismen gegen einen doppelten etcd-Bootstrap
- [x] etcd-Member verwalten: auflisten (nach Hostname, nie nur Hex-ID), entfernen, Snapshot ziehen
- [x] Node aus Cluster entfernen - etcd-leave, reset, aus dem Inventar; cordon/drain findet nicht statt und der Screen sagt es

**Upgrades** *(Phase 9)*
- [x] Talos-Version rollend upgraden, Node für Node
- [x] Kubernetes-Version upgraden
- [x] Health-Gate zwischen Nodes - vor **jedem** Node neu bewertet, mit seinen Eingaben auf dem Schirm
- [x] Upgrade-Fortschritt live in der UI, abbrechbar - ein Job-Schritt pro Node, also greift ein Abbruch an der Node-Grenze

**Provisioning (der Kernwert)** *(Phase 2, Phase 8)*
- [x] Talos Image Factory Integration: Schematic bauen (System Extensions, Kernel-Args, Meta)
- [x] ISO/Installer-Image für ein Schematic erzeugen bzw. Download-Link bereitstellen
- [x] Maschinen im Maintenance-Mode entdecken (Netz-Scan / manuelle IP-Eingabe)
- [x] MachineConfig für neuen Node generieren (aus Cluster-Secrets + Rolle + Patches)
- [x] Config auf Maintenance-Node anwenden, Node joint Cluster, erscheint im Dashboard

**Zugang & Sicherheit** *(Phase 1)*
- [x] Lokaler Login: Benutzername + Passwort (argon2id), Session-Cookie
- [x] talosconfig / Cluster-PKI als Dateien auf Disk mit `0600`
- [x] Destruktive Aktionen erfordern explizite Bestätigung - Sudo-Fenster plus ein an die Parameter gebundenes Token
- [x] Audit-Log: wer hat wann was an welchem Node gemacht - JSONL mit durchgehender Hash-Kette

**Betrieb** *(Phase 1, Phase 2, Phase 10)*
- [x] Single Binary mit embedded Web-UI - keine Runtime-Dependencies
- [x] Docker-Image / Compose-Setup für Dauerbetrieb - geschrieben, **nie gebaut** (Fenster 88)
- [x] Lokale Talos-Sandbox für Entwicklung und Tests ohne echte Hardware

### Was nicht validiert ist

**Nichts davon ist je auf einer echten Maschine gelaufen.** Der
Verifikationsdurchlauf auf amd64-Blech, den Phase 10 als nicht verhandelbare
Eintrittsbedingung führt, hat nicht stattgefunden: dem Ausführungshost fehlen
Homelab, QEMU, `/dev/kvm` und ein Docker-Daemon. Das ist **OPS-05**, der eine
offene Release-Blocker dieses Milestones.

Vier Fenster schließen gemeinsam oder gar nicht, und sie beschreiben zusammen
einen einzigen Durchlauf - eine blanke Maschine wird ein Node, bekommt eine
Konfiguration, wird geupgradet:

| Fenster | Was unbelegt bleibt |
|---|---|
| 82 | Der binäre Abnahmetest von Phase 8, und drei Annahmen, die `talossim` bestätigt, weil `talossim` sie eingebaut hat |
| 84 | Die generierte MachineConfig ist als Dokument geprüft und als Anweisung an eine Maschine nicht |
| 85 | Kein Upgrade auf echter Hardware; `ReappearBudget` ist in beiden Domänen eine geratene Zahl |
| 87 | OPS-05 selbst, plus: der amd64-Installer-Pfad ist auf arm64-QEMU **strukturell** nicht ausübbar |

Drei weitere halten fest, was gebaut und nie ausgeführt wurde: 88 (das
Container-Image), 80 und 81 (die Messung und Tier 2 aus Phase 4) und 76 (Tier 1,
Docker, aus Phase 3).

### Out of Scope

- **SideroLink / WireGuard-Tunnel** — Nodes sind im selben Netz erreichbar; direkter gRPC reicht. Architektur bleibt aber transport-abstrahiert, damit ein Tunnel später nachrüstbar ist
- **CLI (`holzkubectl`)** — Fokus liegt auf der WebUI; die REST-API bleibt aber sauber, damit ein CLI später möglich ist
- **Multi-User / RBAC / OIDC** — Single-Operator-Tool. Auth-Layer wird als Interface gebaut, damit OIDC nachrüstbar ist
- **PXE / netboot-Infrastruktur** — Boot läuft heute über ISO/USB. netboot ist ein späterer Schritt, kein v1
- **ARM64 / SBC-Support** — Hardware ist amd64 Bare Metal / Mini-PC. Image-Factory-Schematics sind arch-parametrisiert, aber ungetestet auf ARM
- **Betrieb im Cluster selbst** — bewusst außerhalb, damit holzkube genau dann erreichbar ist wenn der Cluster kaputt ist
- **Managed-Kubernetes-Provider (EKS/GKE/AKS)** — Talos-only
- **Eigene Monitoring-/Metrics-Pipeline** — keine Prometheus-Konkurrenz. Nur was die Talos API direkt liefert
- **Workload-Management (Deployments, Pods, Helm)** — dafür gibt es k9s/Lens/Headlamp. holzkube ist Node- und Cluster-Ebene

## Context

**Bestehende Umgebung**
- Ein Talos-Cluster im Homelab, API-Server `https://192.168.1.41:6443`
- Hardware: Bare Metal / Mini-PCs, amd64
- Neue Nodes werden heute manuell aufgesetzt: ISO ziehen → USB-Stick → booten → `talosctl apply-config`
- kubeconfig liegt unter `~/.kube/config` (Context `default`) sowie `~/.kube/config-homelab`

**Entwicklungsmaschine (macOS, darwin/arm64)**
- go 1.26.4, node + npm, kubectl v1.34.1, helm v4.2.2, docker vorhanden
- `talosctl` ist **nicht** installiert, `~/.talos/config` existiert **nicht**
- Der Cluster war beim Projektstart vom Dev-Rechner aus nicht erreichbar (Timeout auf `192.168.1.41:6443`) — anderes Netz / VPN aus

**Konsequenz daraus:** Entwicklung darf nicht vom Zugriff auf den echten Cluster abhängen. Es braucht früh eine lokale Talos-Sandbox (Docker- oder QEMU-Provisioner) plus Fakes für die Talos-API, sonst ist jeder Schritt blind. Das gehört in eine frühe Phase, nicht ans Ende.

**Warum kein Omni**
Omni ist proprietär und an Sidero als Anbieter gebunden. Gesucht ist etwas, das dem Betreiber gehört, das er versteht und anpassen kann.

**Technische Landschaft**
- Talos machine API: gRPC auf Port `:50000`, mTLS über talosconfig-Credentials
- Go-Client verfügbar über `siderolabs/talos/pkg/machinery` — kein `talosctl`-Binary als Subprozess nötig
- Talos Image Factory (`factory.talos.dev`): Schematics werden als YAML gepostet, zurück kommt eine Schematic-ID; daraus lassen sich ISO-, Installer- und Disk-Image-URLs bilden
- Maschinen im Maintenance-Mode sprechen dieselbe API, aber ohne Cluster-PKI

## Constraints

- **Tech stack**: Go Backend — die offizielle Talos machinery ist Go, alles andere hieße Protobuf/mTLS von Hand nachbauen
- **Tech stack**: Frontend React + TypeScript + Vite, eingebettet via `embed.FS` — Live-Log-Streams, dichte Dashboards und Terminal-Emulation (xterm.js) sind dort am besten abgedeckt. Ergebnis ist ein einzelnes Binary ohne Runtime-Dependencies
- **Deployment**: Läuft außerhalb des Clusters — ein Management-Tool, das mit dem Cluster stirbt, ist genau im Fehlerfall nutzlos
- **Transport**: Direkter gRPC an Node-IPs. Nodes müssen vom holzkube-Host erreichbar sein
- **Security**: Das Tool hält Cluster-PKI und kann Maschinen wipen. Secrets nur als `0600`-Dateien, destruktive Aktionen nur mit Bestätigung, Audit-Log ab v1
- **Dependencies**: Talos machine API ist versioniert und ändert sich zwischen Talos-Releases — die unterstützte Talos-Version-Range muss explizit sein
- **Testing**: Kein verlässlicher Zugriff auf echte Hardware während der Entwicklung. Lokale Sandbox ist Voraussetzung, nicht Kür
- **Scale**: Heute ein Cluster. Datenmodell muss mehrere Cluster tragen, ohne dass später alles umgebaut wird

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Eigenbau statt Omni | Omni proprietär / an Sidero gebunden; volle Kontrolle und Anpassbarkeit gewünscht | **Bestätigt.** Der Wizard, das Health-Gate und der Audit-Pfad sind drei Entscheidungen, die Omni anders trifft; keine davon wäre in einem Fork anpassbar gewesen. |
| Direkter gRPC statt SideroLink-Tunnel | Nodes sind im selben Netz erreichbar; Tunnel wäre massiver Zusatzaufwand ohne Nutzen | **Bestätigt, und die Naht hat sich bezahlt gemacht.** Kein Aufruf oberhalb von `internal/talos` baut eine Adresse, und ein AST-Guard hält das - ein Tunnel-Transport ist ein zweiter `Dialer`. |
| Transport hinter Interface abstrahiert | Tunnel-Transport soll später nachrüstbar sein, ohne die Codebasis aufzureißen | **Bestätigt.** `talossim` ist derselbe `Dialer` wie das echte Netz, und deshalb läuft jede Phase dieses Milestones gegen einen simulierten Node statt gegen gar nichts. |
| Betrieb außerhalb des Clusters | Kein Henne-Ei-Problem; erreichbar wenn der Cluster down ist | **Bestätigt.** Phase 9s Health-Gate liest etcd von außen; im Cluster wäre es beim Verlust des Quorums selbst weg. |
| Go Backend | Offizielle `siderolabs/talos/pkg/machinery` ist Go; kein `talosctl`-Subprozess nötig | **Bestätigt.** `pkg/machinery` trägt alles außer `pkg/provision`, das bewusst in einem zweiten Modul liegt und durch `TestModuleGraphExcludesTalosRoot` dort gehalten wird. |
| React + TypeScript + Vite, embedded | Live-Streams, dichte Dashboards, xterm.js; ein Binary ohne Runtime-Deps | **Bestätigt.** Ein Binary, und die SSE-Streams tragen Logs und Job-Fortschritt über eine Verbindung pro Tab statt über eine pro Panel. |
| Secrets als Dateien auf Disk (`0600`) | Menschlich lesbar, mit vorhandenen Mitteln sicherbar, kein Krypto-Eigenbau | **Bestätigt.** Der Restore verengt die Rechte beim Auspacken, und der Permission-Guard weist ein Verzeichnis ab, das Gruppe oder andere lesen können. |
| Lokaler Login (argon2id) + Session-Cookie | Von überall erreichbar, ohne IdP-Abhängigkeit. Auth als Interface für späteres OIDC | **Bestätigt und erweitert.** OIDC kam in Phase 2 dazu, als Interface neben dem lokalen Passwort statt an dessen Stelle. |
| Nur WebUI, kein CLI | Ein Interface pflegen. REST-API bleibt sauber für ein späteres CLI | **Teilweise revidiert.** Phase 10 hat vier Subkommandos gebraucht: Backup, Restore und Kettenprüfung sind Operationen auf dem Host, und ein Knopf dafür wäre ein Endpunkt, der den Cluster an jede Session übergibt. |
| Multi-cluster-fähiges Datenmodell ab Tag 1 | Ein Cluster heute, aber ein Umbau bei Cluster #2 wäre teuer | **Bestätigt.** `Machine.Cluster` ist ein nullbares Feld statt eines Verzeichnisses, und genau das macht eine Maschine ohne Cluster zum Normalfall statt zum Sonderfall. |
| Provisioning inkl. Image Factory ist v1-Scope | Es ist der Core Value — ohne das ist holzkube nur ein weiteres Dashboard | **Bestätigt und unbewiesen.** Der Pfad ist vollständig; die Abnahme fehlt (Fenster 82, 87). |
| Lokale Talos-Sandbox als frühe Phase | Kein verlässlicher Hardware-Zugriff beim Entwickeln; sonst wird blind gebaut | **Die folgenreichste Entscheidung des Milestones.** Ohne `talossim` wäre von zehn Phasen keine ausführbar gewesen - und die vier offenen Fenster sind genau die Stellen, an denen der Simulator eine Annahme bestätigt, die er selbst eingebaut hat. |

## Evolution

Dieses Dokument entwickelt sich an Phasenübergängen und Milestone-Grenzen.

**Nach jedem Phasenübergang** (via `/gsd-transition`):
1. Requirements widerlegt? → nach Out of Scope, mit Begründung
2. Requirements validiert? → nach Validated, mit Phasen-Referenz
3. Neue Requirements aufgetaucht? → zu Active
4. Entscheidungen zu protokollieren? → zu Key Decisions
5. "What This Is" noch korrekt? → anpassen falls abgedriftet

**Nach jedem Milestone** (via `/gsd-complete-milestone`):
1. Vollständiger Review aller Abschnitte
2. Core-Value-Check — noch die richtige Priorität?
3. Out of Scope prüfen — Begründungen noch gültig?
4. Context auf aktuellen Stand bringen

---
*Last updated: 2026-09-11 at the close of milestone v1.14.*
