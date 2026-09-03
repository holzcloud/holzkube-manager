# holzkube

## What This Is

holzkube ist eine self-hosted Management-Oberfläche für Kubernetes-Cluster auf Talos Linux — eine Alternative zu Sidero Omni, ohne Vendor-Bindung und ohne SaaS. Ein einzelnes Go-Binary läuft **außerhalb** des Clusters, spricht die Talos machine API direkt per gRPC an und stellt eine Web-UI bereit, mit der Nodes überwacht, konfiguriert, geupgradet und von blankem Blech aus neu provisioniert werden.

Gebaut für Homelab-/Lab-Betreiber, die Talos fahren und die volle Kontrolle über ihr Management-Tooling behalten wollen. Primärer Nutzer: der Autor selbst.

## Core Value

**Eine neue Maschine wird komplett in der UI zum Cluster-Node — ohne `talosctl`, ohne Omni.**

Wenn alles andere scheitert: der Weg blanke Maschine → Image → Boot → Discovery → Config → Cluster-Join muss in der Oberfläche funktionieren.

## Requirements

### Validated

(Noch nichts — ship to validate)

### Active

**Inventar & Sichtbarkeit**
- [ ] Nodes und Cluster werden als persistentes Inventar geführt (multi-cluster-fähiges Datenmodell)
- [ ] Dashboard zeigt pro Node: Health, Talos-Version, Kubernetes-Version, CPU/RAM, Disks, Netzwerk-Interfaces
- [ ] Service-Status pro Node sichtbar (etcd, kubelet, apid, machined, trustd …)
- [ ] Live-Streaming von Logs und `dmesg` pro Node in der UI
- [ ] Cluster-Übersicht: Control-Plane vs Worker, etcd-Member, Cluster-Health

**Konfiguration**
- [ ] MachineConfig eines Nodes in der UI ansehen (rendered + raw YAML)
- [ ] Config-Patches erstellen, versionieren und wiederverwenden
- [ ] Diff-Ansicht vor dem Anwenden (was ändert sich konkret)
- [ ] Patch anwenden mit Wahl des Apply-Modus (auto / no-reboot / staged / reboot)
- [ ] Dry-Run vor dem Anwenden

**Lifecycle**
- [ ] Node-Aktionen: Reboot, Shutdown, Reset — mit expliziter Bestätigung
- [ ] Cluster bootstrappen (erster Control-Plane-Node)
- [ ] etcd-Member verwalten: auflisten, entfernen, Snapshot ziehen
- [ ] Node aus Cluster entfernen (cordon/drain → reset → aus Inventar)

**Upgrades**
- [ ] Talos-Version rollend upgraden, Node für Node
- [ ] Kubernetes-Version upgraden
- [ ] Health-Gate zwischen Nodes: nächster Node erst wenn vorheriger wieder healthy ist
- [ ] Upgrade-Fortschritt live in der UI, abbrechbar

**Provisioning (der Kernwert)**
- [ ] Talos Image Factory Integration: Schematic bauen (System Extensions, Kernel-Args, Meta)
- [ ] ISO/Installer-Image für ein Schematic erzeugen bzw. Download-Link bereitstellen
- [ ] Maschinen im Maintenance-Mode entdecken (Netz-Scan / manuelle IP-Eingabe)
- [ ] MachineConfig für neuen Node generieren (aus Cluster-Secrets + Rolle + Patches)
- [ ] Config auf Maintenance-Node anwenden → Node joint Cluster → erscheint im Dashboard

**Zugang & Sicherheit**
- [ ] Lokaler Login: Benutzername + Passwort (argon2id), Session-Cookie
- [ ] talosconfig / Cluster-PKI als Dateien auf Disk mit `0600`
- [ ] Destruktive Aktionen (Reset, Wipe, etcd-Member entfernen) erfordern explizite Bestätigung
- [ ] Audit-Log: wer hat wann was an welchem Node gemacht

**Betrieb**
- [ ] Single Binary mit embedded Web-UI — keine Runtime-Dependencies
- [ ] Docker-Image / Compose-Setup für Dauerbetrieb
- [ ] Lokale Talos-Sandbox für Entwicklung und Tests ohne echte Hardware

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
| Eigenbau statt Omni | Omni proprietär / an Sidero gebunden; volle Kontrolle und Anpassbarkeit gewünscht | — Pending |
| Direkter gRPC statt SideroLink-Tunnel | Nodes sind im selben Netz erreichbar; Tunnel wäre massiver Zusatzaufwand ohne Nutzen | — Pending |
| Transport hinter Interface abstrahiert | Tunnel-Transport soll später nachrüstbar sein, ohne die Codebasis aufzureißen | — Pending |
| Betrieb außerhalb des Clusters | Kein Henne-Ei-Problem; erreichbar wenn der Cluster down ist | — Pending |
| Go Backend | Offizielle `siderolabs/talos/pkg/machinery` ist Go; kein `talosctl`-Subprozess nötig | — Pending |
| React + TypeScript + Vite, embedded | Live-Streams, dichte Dashboards, xterm.js; ein Binary ohne Runtime-Deps | — Pending |
| Secrets als Dateien auf Disk (`0600`) | Menschlich lesbar, mit vorhandenen Mitteln sicherbar, kein Krypto-Eigenbau | — Pending |
| Lokaler Login (argon2id) + Session-Cookie | Von überall erreichbar, ohne IdP-Abhängigkeit. Auth als Interface für späteres OIDC | — Pending |
| Nur WebUI, kein CLI | Ein Interface pflegen. REST-API bleibt sauber für ein späteres CLI | — Pending |
| Multi-cluster-fähiges Datenmodell ab Tag 1 | Ein Cluster heute, aber ein Umbau bei Cluster #2 wäre teuer | — Pending |
| Provisioning inkl. Image Factory ist v1-Scope | Es ist der Core Value — ohne das ist holzkube nur ein weiteres Dashboard | — Pending |
| Lokale Talos-Sandbox als frühe Phase | Kein verlässlicher Hardware-Zugriff beim Entwickeln; sonst wird blind gebaut | — Pending |

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
*Last updated: 2026-08-27 after initialization*
