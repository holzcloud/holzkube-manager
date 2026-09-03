# Roadmap: holzkube-manager

## Overview

holzkube wächst von unten nach oben, aber mit einem frühen Loch durch alle Schichten. Zuerst entsteht ein Fundament ohne jede Talos-Abhängigkeit (Store, Audit, Auth, HTTPS, embedded UI). Danach die Naht, an der alles hängt: der gRPC-Transport plus `talossim` — ein In-Process-Fake-Talos-Node, ohne den jede spätere Phase blind gebaut würde. Auf dieser Naht entsteht das Inventar mit dem **Import des bereits existierenden Homelab-Clusters** — nicht mit `gen secrets`, denn der Cluster ist schon da. An dieser Stelle wird bewusst ein **Walking Skeleton** eingeschoben: der hässliche, hartkodierte End-to-End-Weg blanke Maschine → Config → Apply → Cluster-Join gegen die QEMU-Sandbox, damit die vier gefährlichsten Unbekannten in Woche drei bekannt sind statt in Monat sechs. Erst danach kommen Streaming, die Jobs-Engine, die Config-Domain — jede davon existiert, um die eigentliche Phase zu ermöglichen und abzusichern: **Provisioning**, den Core Value. Upgrades und etcd-Verwaltung folgen, weil sie Health-Gate, Jobs-Engine und `schematic_id` voraussetzen. Am Ende steht die einzige Phase, die echte Hardware zwingend braucht.

**Struktur-Herkunft:** Die Phasenstruktur stammt aus `.planning/research/SUMMARY.md` § *Suggested phase structure* und respektiert alle zehn harten Ordering-Constraints aus § *Hard ordering constraints*. Zwei bewusste Abweichungen sind unten unter [Abweichungen von der Research-Struktur](#abweichungen-von-der-research-struktur) begründet.

## Phases

**Phase Numbering:**

- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Foundation Skeleton** - Store, Audit, Auth, HTTPS, Single Binary mit embedded UI — ohne jede Talos-Abhängigkeit (completed 2026-08-28)
- [ ] **Phase 2: Transport Seam, `talossim` & Image Factory** - Die Naht zu Talos plus der Fake, gegen den alles Weitere getestet wird (BLOCKING)
- [ ] **Phase 3: Inventar, Cluster-Import & Health** - Der bestehende Cluster wird importiert; das Inventar bleibt ehrlich, auch wenn alles tot ist
- [ ] **Phase 4: Walking Skeleton (Wegwerf)** - Der hässliche End-to-End-Weg gegen QEMU, um die vier gefährlichsten Unbekannten zu entschärfen
- [ ] **Phase 5: Streaming** - Live-Logs und `dmesg` über eine multiplexte SSE-Verbindung (DESIGNIERTE SCHNITTLINIE)
- [ ] **Phase 6: Jobs-Engine & Node-Aktionen** - Persistierte, crash-feste Jobs; Reboot, Shutdown und das Reset-Dialog
- [ ] **Phase 7: Config-Domain** - Ansehen, redigieren, patchen, diffen, anwenden — mit berechnetem Apply-Modus
- [ ] **Phase 8: Provisioning — der Core Value** - Blanke Maschine wird gesunder Cluster-Node, ohne Terminal
- [ ] **Phase 9: Upgrades & etcd-Verwaltung** - Rollende Talos-/K8s-Upgrades hinter einem Gate, das lieber blockiert als strandet
- [ ] **Phase 10: Härtung & echter Hardware-Durchlauf** - Backup/Restore, Docker/Compose, Versionsrange — und ein Durchlauf auf echtem amd64

## Phase Details

### Phase 1: Foundation Skeleton

**Goal**: Der Betreiber startet ein einzelnes Binary, meldet sich an, und jede Mutation ist nachvollziehbar und crash-sicher gespeichert — alles ohne dass Talos existiert.
**Depends on**: Nothing (first phase)
**Requirements**: FOUND-01, FOUND-02, FOUND-03, FOUND-04, FOUND-05, FOUND-06, FOUND-07, FOUND-08, FOUND-09, FOUND-10, FOUND-11
**Success Criteria** (what must be TRUE):

  1. Betreiber startet ein einzelnes Binary ohne Runtime-Dependencies und erreicht die eingebettete Web-UI über HTTPS auf `127.0.0.1`; API-Fehler kommen einheitlich als RFC-9457 `problem+json` mit stabiler Taxonomie zurück.
  2. Betreiber meldet sich mit Benutzername und Passwort an (argon2id, Parameter mitgespeichert); die Session wird beim Login rotiert und wiederholte Fehlversuche werden ratenbegrenzt.
  3. Eine als destruktiv markierte Aktion verlangt trotz gültiger Session erneut das Passwort (sudo-mode).
  4. Jede Mutation erscheint im Audit-Log als Intent-vor-/Outcome-nach-Paar; das Log ist JSONL, täglich rotiert und eine durchgehende Hash-Kette.
  5. Ein `kill -9` mitten im Schreiben hinterlässt keinen korrupten Zustand; aller Zugriff läuft über `store.Machines().Get(...)` statt über Dateipfade, alle Zustandsdateien liegen `0600` in einem `0700`-Verzeichnis, und ein Schema-Upgrade legt vorher ein Backup an.

**Research**: Nicht nötig — Standard-Patterns (argon2id, Sessions, CSRF, atomare Writes, JSONL-Audit). Research-Flag: *skip*.
**Parallel tracks**: 3 — (a) `store` / `audit` / `auth`, (b) Vite-Scaffold + `embed.FS`, (c) TLS + Config-Loading.
**Release blockers owned**: keine
**Plans**: 6/6 plans executed (Wave 1: 1 Tracer; Wave 2: 5 parallele Pläne)

Plans:
**Wave 1**

- [x] 01-01-PLAN.md — Tracer: Binary → HTTPS → UI → Setup → Login → Audit-Eintrag; legt zugleich alle Schnittstellen und `docs/api-contract.md` fest

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 01-02-PLAN.md — `store`: Prozess-flock, Per-Entity-Mutex, `rev`-CAS, forward-only Migrationen mit Backup, Rechte-Guard, Crash-Injection
- [x] 01-03-PLAN.md — `audit`: tägliche Rotation mit durchlaufender Hash-Kette, gzip ab Tag 2, Startverifikation, Allowlist-Redaktion, Query-API
- [x] 01-04-PLAN.md — `auth`: argon2id kalibriert, Session-Rotation und 24-h-Grenze, Sudo-Mode mit `428`, Rate-Limiting ohne Sperrzustand, CSRF
- [x] 01-05-PLAN.md — Web-App-Shell: TanStack Router, shadcn/ui, dark-first Theme, Setup/Login/Sudo-Dialog, Audit-Ansicht, Ketten-Banner
- [x] 01-06-PLAN.md — Config (Flags + `HOLZKUBE_*`), TLS mit Fingerprint und Loopback-Guard, Bau-/Lint-/Release-Kette, Modulgrenze `sandbox/`

**UI hint**: yes

### Phase 2: Transport Seam, `talossim` & Image Factory

**Goal**: Jede Talos-Interaktion läuft durch eine austauschbare Naht und ist ohne Hardware testbar; Schematics und Image-URLs sind korrekt und nachweislich brauchbar herleitbar.
**Depends on**: Phase 1
**Requirements**: FOUND-12, TRANS-01, TRANS-02, TRANS-03, TRANS-04, TRANS-05, TRANS-06, TRANS-07, FACT-01, FACT-02, FACT-03, FACT-04, FACT-05, FACT-06
**Success Criteria** (what must be TRUE):

  1. Der **unveränderte** Produktions-Client spricht gegen `talossim` — echte Protobufs, echtes mTLS, echter In-Memory-COSI-State — ohne Hardware, ohne `talosctl`, ohne Netzwerk.
  2. Betreiber schaltet in `talossim` Fehlerszenarien (`go_silent(90s)`, `reject_apply`, `second_bootstrap_returns_AlreadyExists`, `flap_connection`, `slow_log_consumer`, `ip_changes_on_reboot`, `etcd_down`, `k8s_down`, `version_out_of_supported_range`) und der Client verhält sich in jedem Fall definiert statt zufällig.
  3. Ein nicht erreichbarer Node blockiert weder die UI noch andere Nodes; jeder Aufruf hat ein erzwungenes Deadline und Retries gibt es nur für eine Allowlist lesender Operationen. Cluster- und Maintenance-Clients sind getrennte Typen und lassen sich nicht verwechseln.
  4. Betreiber stellt ein Schematic aus einem versions-skopierten Extension-Katalog zusammen, bekommt die exakten ISO-, Installer- und PXE-URLs (versionsabhängiger Repo-Name, keine hartkodierte Architektur) und das Schematic gilt erst nach einem bestätigenden Model-Build-Probe als brauchbar. Kernel-Args oder META lösen die Warnung aus, dass `installer`/`initramfs` nur System-Extensions ehren.
  5. Das gesamte Binary läuft mit `--dry-run`, und keine Mutation erreicht dabei einen Node.

**Research**: **Ja** (Research-Flag Phase 1) — `talossim`-Machbarkeit ruht auf zwei unverifizierten Annahmen (`cosi-project/runtime/.../inmem` nicht direkt verifiziert; `MachineService`-Streaming-Signaturen für `Logs`/`Dmesg`/`Events` nicht enumeriert). Die `machinery`-API-Oberfläche gegen die gepinnte Version prüfen.
**Parallel tracks**: 2 — (a) Transport-Naht + `pool` + `talossim`, (b) Image-Factory-Client. Track (b) hat **null** Talos-Abhängigkeit; das ist die Research-Parallelität *Phase 1 ∥ Phase 1b*, hier innerhalb einer Phase realisiert.
**Release blockers owned**: TRANS-06 🚫
**Note**: FOUND-12 (`--dry-run` für das ganze Binary) liegt hier statt in Phase 1, weil die Research es explizit in den Deliverables der Transport-Phase führt — ein Dry-Run ist erst sinnvoll, wenn Mutationen einen Node erreichen könnten.
**Plans**: 21/21 plans executed in 5 waves (Wave 1: 2 parallele Tracer — Track (a) Transport-Naht, Track (b) Image Factory) + 5 Gap-Closure-Pläne der Runde 1 aus 02-UAT.md in 4 Wellen (Cluster B und C; Cluster A ist auf `02-DECISION-probe-budget.md` blockiert) + 8 Gap-Closure-Pläne der Runde 2 in 6 Wellen (alle 14 Lücken G-02-10..G-02-23, voller Umfang in einer Runde)
**UI hint**: yes

Plans:
**Wave 1**

- [x] 02-01-PLAN.md — Tracer: der **unveränderte** machinery-Client spricht über die `Dialer`-Naht per echtem mTLS mit `talossim`; zweite `Dialer`- und `DiscoverySource`-Implementierung an einer Aufrufstelle, Modulgrenzen-Guard (D-07)
- [x] 02-02-PLAN.md — Tracer Track (b): handgeschriebener Image-Factory-Client (D-01), lokal vorberechnete Schematic-ID, Katalog-Validierung vor dem POST, Model-Build-Probe; mintet die `upstream`-Problem-Familie

**Wave 2** *(blocked on Wave 1)*

- [x] 02-08-PLAN.md — `talossim` als glaubwürdiges Orakel: `MachineService`-Umfang mit echtem Node-State, In-Memory-COSI am selben gRPC-Server, die drei Streams, und ein laufender Drift-Guard (Abspaltung von 02-01, W-2)
- [x] 02-04-PLAN.md — Asset-URLs mit versions-aufgelöstem Installer-Repo (D-02), Prerelease-Filter und kuratierte Broken-Liste (D-08), `store.Schematics()` und Schema-Version 2 (D-09)

**Wave 3** *(blocked on Wave 2)*

- [x] 02-03-PLAN.md — `talossim`-Szenario-Engine: die neun TRANS-07-Fehlerfälle zur Laufzeit injizierbar, jeder mit dokumentiertem erwartetem Client-Verhalten
- [x] 02-06-PLAN.md — Schematics-API und Images-Screen: versions-skopierter Katalog ohne Freitextfeld, Kernel-Args-/META-Warnung, exakte ISO-/PXE-/Installer-URLs, Audit-Allowlist-Einträge

**Wave 4** *(blocked on Wave 3)*

- [x] 02-05-PLAN.md — Transport-Policy: getrennte Cluster-/Maintenance-Typen (D-06), erzwungene Deadlines und Read-Allowlist-Retries (D-04), Per-Node-Breaker und Fan-out, Contract-Suite über alle neun Szenarien

**Wave 5** *(blocked on Wave 4)*

- [x] 02-07-PLAN.md — `--dry-run` als vertikale Scheibe: Verweigerung im Transport vor der Leitung (D-03), Flag und Composition Root, sichtbares Banner, Audit-Actor-Vokabular (D-10)

**Gap-Closure Wave 1** *(aus 02-UAT.md; Cluster B und C)*

- [x] 02-09-PLAN.md — G-02-4, G-02-5: SecureBoot erreicht Installer-Repo-Namen und Cache-Key; **eine** Taxonomie für Registry-Antworten (400/404 = Absage, 401/403/429/5xx = keine Antwort) in `resolveInstallerRepo` **und** `ProbeBuildable`
- [x] 02-10-PLAN.md — G-02-7 (und die erste Hälfte von G-02-8): eigener Arch-State für das Asset-Panel (der gemerkte Wert gehört dem Erstellungsformular); Fehlerzweig im Schematic-Detail-Dialog statt eines Modals, dessen einziger Text „Close" ist

**Gap-Closure Wave 2** *(blocked on Gap-Closure Wave 1)*

- [x] 02-11-PLAN.md — G-02-6: ein lokal verweigerter Wert wird 400 mit Feldnamen statt 502 „Image Factory did not answer"; Steuerzeichen werden am Eingabefeld abgelehnt, der Create-Fehler wird beim Formularwechsel gelöscht

**Gap-Closure Wave 3** *(blocked on Gap-Closure Wave 2)*

- [x] 02-12-PLAN.md — G-02-3: ein an einem stummen Kandidaten vorbei erreichter Repo-Name warnt (`installer.repo-fallback-unverified`), wird nur **provisorisch** gecacht (mit Warnung bei jeder Antwort, Neu-Frage nach Ablauf des Intervalls) und erscheint im Asset-Panel; dazu die unbedingte Badge-Kopie aus G-02-1 und der `break-all`-Fix

**Gap-Closure Wave 4** *(blocked on Gap-Closure Wave 3)*

- [x] 02-13-PLAN.md — G-02-8 (zweite Hälfte, schließt die Lücke): `Arch` auf `model.Schematic`, gestempelt wie `ProbedAt`, und das Usability-Verdikt wird neben der Architektur gezeigt, gegen die geprüft wurde; Altbestände bleiben unqualifiziert und stehen als Fenster im Ledger

**Runde-2-Gap-Closure Wave 1** *(aus 02-UAT.md Runde 2; alle 14 Lücken, voller Umfang)*

- [x] 02-14-PLAN.md — G-02-16: die Divergenz des kanonischen Serialisierers wird gegen das Canonical der echten Factory **gemessen** statt erneut abgeschrieben; `representable`/`plainAllowed` verweigern anschließend genau die gemessenen Klassen, und der Differential-Test bleibt als Wächter im Baum
- [x] 02-15-PLAN.md — G-02-19, G-02-14: der Detail-Dialog überlebt den tailwind-merge und ist nicht mehr auf 384px geklemmt; das Verdikt fragt erst, **ob** es eines gibt, bevor es behauptet, welches

**Runde-2-Gap-Closure Wave 2** *(blocked on Wave 1)*

- [x] 02-16-PLAN.md — G-02-18, G-02-13, G-02-12: der Drift-Wächter kann wieder fehlschlagen (Warnungen und aufgelöster Repo-Name werden geprüft, offline bewiesen); `installer-secureboot` ist **kein** Alias von `metal-installer-secureboot`, und jede Aussage über die Matrix behauptet nur noch, was gemessen wurde
- [x] 02-17-PLAN.md — G-02-10: ein Repository-Name belegt genau **eine** Zeilenbox, gemessen in einer echten Layout-Engine über einen Breiten-Sweep und alle vier Installer-Namen; der Referenztext bleibt Zeichen für Zeichen derselbe

**Runde-2-Gap-Closure Wave 3** *(blocked on Wave 2)*

- [x] 02-18-PLAN.md — G-02-23 (Nutzerentscheidung): die Problem-Taxonomie wird auf `urn:holzkube-manager:problem:` umgewurzelt, bleibt geschlossen und **nicht** pro Deployment konfigurierbar; der Vertrag sagt, dass Type-URIs Bezeichner und nicht dereferenzierbar sind

**Runde-2-Gap-Closure Wave 4** *(blocked on Wave 3)*

- [x] 02-19-PLAN.md — G-02-15: die vier registry-freien Referenzen werden nicht mehr weggeworfen, weil eine fünfte nicht auflösbar war; die Meldung trennt „warten und erneut fragen" von „diese Version hat keinen SecureBoot-Installer", und die Atomarität steht im Vertrag

**Runde-2-Gap-Closure Wave 5** *(blocked on Wave 4)*

- [x] 02-20-PLAN.md — G-02-17, G-02-11: `name` und `cluster` laufen durch dieselbe Prüfung wie ihre Geschwister, der einsame Surrogat-Halbwert bekommt einen entschiedenen Vertrag, und der Browser verweigert genau die Menge, die der Server verweigert — von einem Drift-Wächter festgenagelt

**Runde-2-Gap-Closure Wave 6** *(blocked on Wave 5)*

- [x] 02-21-PLAN.md — G-02-20, G-02-21, G-02-22: die Identitätsfrage (Architektur gehört nicht in den Hash) wird als Entscheidung festgehalten, die drei zu breiten Aussagen werden auf die Sätze verengt, für die sie gelten, und das Ledger dieser Runde wird vollständig eingetragen

**Blockiert** *(nicht geplant)*: G-02-1, G-02-2, G-02-9 — der Probe-Budget-Knoten. Offene Entscheidung in `.planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-probe-budget.md`.

### Phase 3: Inventar, Cluster-Import & Health

**Goal**: Der bestehende Homelab-Cluster ist importiert, und das Inventar bleibt ehrlich — auch wenn ein Node oder der ganze Cluster nicht antwortet.
**Depends on**: Phase 2
**Requirements**: TRANS-08, INV-01, INV-02, INV-03, INV-04, INV-05, INV-06, INV-07, INV-08, INV-09, INV-10, INV-11, INV-12, INV-13
**Success Criteria** (what must be TRUE):

  1. Betreiber importiert seinen **bestehenden** Cluster über ein Secrets-Bundle von einem Control-Plane-Node, bestätigt den Fingerprint und sieht einen Konnektivitätsbeweis mit einem frisch ausgestellten Zertifikat. Alternativ legt er einen neuen Cluster an; `gen secrets` ist ausschließlich aus diesem zweiten Pfad erreichbar.
  2. Dashboard zeigt pro Node Health, Talos-Version, Kubernetes-Version, CPU/RAM, Disks, Netzwerk-Interfaces und Service-Status; jedes gelesene Feld trägt sein `HealthLevel` und `stale_since`, und die Daten kommen aus COSI-Watches mit gejittertem Heartbeat statt aus blindem Polling.
  3. Ein Node behält seinen Record über IP-Wechsel, Reboot und Unerreichbarkeit hinweg (UUID-adressiert, `cluster_id` nullbar, `schematic_id` am Record). Bei totem Cluster bleiben Node-Level-Daten sichtbar und veraltete Werte sind als veraltet markiert — nie eine leere Seite, nie ein gelöschter Record.
  4. Cluster-Übersicht zeigt Control-Plane vs Worker, etcd-Member, Cluster-Health, die Restlaufzeit des talosconfig-Client-Zertifikats mit rechtzeitiger Warnung und den "Abstand zum Rand" der Talos↔Kubernetes-Kompatibilitätsmatrix. Ein read-only gesperrter Cluster nimmt keine Mutation an.
  5. **Dieselbe** Contract-Test-Suite läuft grün gegen `talossim` und gegen echtes Talos (Tier-1-Docker-Provisioner); eine Divergenz ist ein Testfehler, keine Produktionsüberraschung.

**Research**: **Ja** (Research-Flag Phase 2) — der Tier-1-Docker-Provisioner auf `darwin/arm64` wurde **nicht getestet**. Zuerst spiken; scheitert `pkg/provision`, kollabiert Tier 1 auf Tier 0 und das Fake-Drift-Risiko jeder späteren Phase steigt.
**Parallel tracks**: 2 — Backend ∥ Frontend, sobald die `Field[T]`-Response-Shape festgezurrt ist.
**Release blockers owned**: INV-01 🚫, INV-03 🚫, INV-04 🚫, INV-05 🚫, INV-07 🚫
**Note**: TRANS-08 liegt hier statt in Phase 2, weil "gegen echtes Talos" erst ausführbar ist, sobald Tier 1 existiert. Die Suite selbst und ihre Fake-Hälfte entstehen in Phase 2.
**Plans**: TBD
**UI hint**: yes

### Phase 4: Walking Skeleton (Wegwerf)

**Goal**: Der Weg blanke Maschine → Config → Apply → Cluster-Join läuft **einmal** end-to-end gegen QEMU — hartkodiert, hässlich und echt — damit die vier gefährlichsten Unbekannten bekannte Probleme werden statt Risiken.
**Depends on**: Phase 3
**Requirements**: keine — bewusst (siehe Note)
**Success Criteria** (what must be TRUE):

  1. Die QEMU-Sandbox (Tier 2) läuft reproduzierbar auf dem `darwin/arm64`-Dev-Host — oder der dokumentierte Fallback über eine verschachtelte Linux-VM (Lima/Colima/UTM) tut es, mit festgehaltener Begründung.
  2. Eine **von Hand eingegebene** Maintenance-Mode-IP nimmt eine aus einem hartkodierten Schematic generierte MachineConfig an; die Maschine installiert, rebootet und erscheint als Node im Inventar — ohne dass ein Terminal geöffnet wird.
  3. Die Installations-Stille nach dem Apply ist **gemessen** und als Zeitbudget notiert, statt als offene Frage in Phase 8 aufzutauchen.
  4. Doppelter etcd-Bootstrap und ein Apply auf die falsche Ziel-IP sind beide einmal ausgelöst und ihr beobachtetes Verhalten ist dokumentiert (`AlreadyExists`, Fehlerbild, Zeitverlauf).
  5. Der Skeleton ist als Wegwerf-Code markiert und ausdrücklich nicht in einen Produktionspfad eingebaut; **kein** PROV-Requirement gilt dadurch als erfüllt.

**Research**: **Ja** — QEMU auf `darwin/arm64` ist nur im Quelltext bestätigt (`preflight_darwin.go`), **nicht ausgeführt**; `sudo` + `vmnet-shared` können schmerzhaft sein. Ebenso offen: welche Ressourcen im Maintenance-Mode unauthentifiziert lesbar sind (`sensitivity`-Feld auf den Resource-*Definitionen*) — das ist nur an einem echten Node feststellbar und Phase 8 darf nicht auf Ressourcen gebaut werden, die dort nicht existieren.
**Release blockers owned**: keine — aber diese Phase entschärft PROV-05 🚫, PROV-09 🚫 und PROV-10 🚫, bevor sie in Phase 8 gebaut werden.
**Note**: Diese Phase trägt **absichtlich kein Requirement**. Sie ist Wegwerf-Gerüst zur Risikoentschärfung, nicht die vorgezogene Erfüllung des Core Value. Jedes Requirement, das sie berührt, wird in Phase 6, 7 oder 8 ordentlich gebaut. Sie ist **nicht** die Provisioning-Phase, und ein grüner Skeleton ist **kein** Grund, Phase 8 zu kürzen. Abweichung von der Research-Struktur auf ausdrückliche Entscheidung des Betreibers: "Horizontal Layers *plus* early walking skeleton".
**Note**: Der QEMU-Aufbau (Tier 2) zieht hierher vor. Research verlangt ihn "vor Phase 6 declared done" — früher ist strikt stärker und verletzt Constraint #10 nicht.
**Plans**: TBD

### Phase 5: Streaming

**Goal**: Betreiber sieht Logs und `dmesg` eines Nodes live, und der Verbindungszustand lügt nie.
**Depends on**: Phase 3
**Requirements**: STREAM-01, STREAM-02, STREAM-03, STREAM-04
**Success Criteria** (what must be TRUE):

  1. Betreiber öffnet Logs und `dmesg` eines Nodes und sieht sie live in der UI.
  2. Mehrere Panels in einem Tab teilen sich **eine** SSE-Verbindung; ein Reconnect spielt über `Last-Event-ID` nach, statt Lücken zu hinterlassen.
  3. Ein langsamer Browser blockiert nie den Upstream-Reader; verworfene Daten erscheinen als sichtbare Lücke statt als stille Auslassung.
  4. Der Stream-Zustand ist jederzeit sichtbar: live / reconnecting / Node rebootet / getrennt.

**Research**: Nicht nötig — SSE + Ring-Buffer + Fan-out ist eine gelöste Form, vollständig spezifiziert in ARCHITECTURE Pattern 5. Research-Flag: *skip*.
**Release blockers owned**: keine
**🚧 ENTRY BLOCKER (aus Phase 2, Plan 02-05, Nutzerentscheidung `option-a` vom 2026-08-29)**: Die HTTP-Kette kann heute **nicht streamen**, und das scheitert still. Bevor die erste SSE-Route gebaut wird, müssen zwei Dinge landen: (1) alle drei `ResponseWriter`-Wrapper in der Middleware-Kette implementieren `Flush`, `Unwrap` und `Hijack` — heute tut es keiner, also puffert ein Streaming-Handler auf dieser Kette lautlos; (2) `WriteTimeout = 60 s` in `cmd/holzkubed/main.go` ist prozessweit und mit argon2id plus Login-Ratelimit begründet, nicht mit Upstream-Calls — es muss pro Route ausgenommen oder über `http.ResponseController.SetWriteDeadline` überschrieben werden, sonst stirbt jeder Stream nach 60 s. Phase 2 hat die Stream-Deadline-Policy (kein Gesamt-Deadline, 10 s First-Byte, 60 s Idle) bewusst nur in der Transportschicht umgesetzt und **keine** streamende HTTP-Route gebaut, damit die Kollision hier nicht zuschlägt. Details: `.planning/phases/02-transport-seam-talossim-image-factory/02-CONTEXT.md` § `<deadline_policy>`.
**⚠️ DESIGNIERTE SCHNITTLINIE**: Diese Phase ist echt wertvoll und liegt echt **neben** dem Core-Value-Pfad. Wenn v1 in Gefahr gerät, wird **diese** Phase gekürzt oder gestrichen — nicht Phase 8. Beim Kürzen bleibt der `streamhub`-Kern (Topics, Ring-Buffer, non-blocking Fan-out) erhalten, weil JOB-04 in Phase 6 darauf reitet; gestrichen werden dann Log-/`dmesg`-Viewer und xterm.js.
**Note**: Research führt Streaming ∥ Jobs. Als sequenzielle GSD-Phasen ist diese Parallelität bewusst aufgegeben, um die Kürzbarkeit zu erhalten — eine mit der Jobs-Engine verschmolzene Streaming-Phase wäre nicht mehr sauber herauszuschneiden. Streaming steht vorn, weil `streamhub` das Fan-out-Substrat für den Job-Fortschritt ist.
**Plans**: TBD
**UI hint**: yes

### Phase 6: Jobs-Engine & Node-Aktionen

**Goal**: Langlaufende, gefährliche Operationen überleben einen Prozess-Neustart und sind nie versehentlich auslösbar.
**Depends on**: Phase 5
**Requirements**: JOB-01, JOB-02, JOB-03, JOB-04, JOB-05, JOB-06, JOB-07, JOB-08, JOB-09
**Success Criteria** (what must be TRUE):

  1. Ein `kill -9` mitten in einem Job führt beim Neustart zu korrektem Fortsetzen **oder** zu einem für einen Menschen geparkten Job — nie zu einem doppelt ausgeführten Seiteneffekt. Schritte ohne gepaarte Read-only-"ist es passiert?"-Abfrage werden nie automatisch retryt.
  2. Betreiber rebootet und fährt Nodes aus der UI herunter; der Fortschritt ist live sichtbar und an einer Schrittgrenze abbrechbar.
  3. Reset zeigt **vor** dem Ausführen die Disks, die Wipe-Scope-Wahl und die Reboot-Kontrolle, stellt die effektiven Flags dar und verlangt das Tippen des Hostnamens.
  4. Eine Bestätigung, die nur im Browser stattfand, wird vom Server abgelehnt (Confirmation-Token); jeder destruktive Endpoint antwortet mit `202 Accepted` und einer Job-ID und schreibt ins Audit-Log.
  5. Pro Cluster läuft immer nur **ein** mutierender Job; ein zweiter wartet oder wird abgelehnt, statt sich mit dem ersten zu überlagern.

**Research**: Nicht geflaggt.
**Release blockers owned**: JOB-07 🚫
**Note**: Constraint #2 — die Engine muss vor dem schwersten Job auf ihr existieren. Der wertvollste Test des Projekts liegt hier: Prozess mitten im Schritt töten, korrektes Resume-oder-Park behaupten.
**Plans**: TBD
**UI hint**: yes

### Phase 7: Config-Domain

**Goal**: Betreiber sieht, versioniert, diffed und wendet MachineConfigs an — ohne je ein Secret zu leaken und ohne Überraschung beim Apply-Modus.
**Depends on**: Phase 6
**Requirements**: CFG-01, CFG-02, CFG-03, CFG-04, CFG-05, CFG-06, CFG-07, CFG-08, CFG-09, CFG-10, CFG-11
**Success Criteria** (what must be TRUE):

  1. Betreiber sieht die MachineConfig eines Nodes gerendert und roh — und in **keinem** Ausgabepfad (Config-View, Raw-Tab, Diff, API-Response, Audit-Log) erscheint ein Secret; bewiesen durch einen Entropie-Walk-Test über alle fünf Aufrufstellen derselben `redact`-Funktion.
  2. Vor dem Anwenden zeigt holzkube ein **strukturelles** Diff der tatsächlich gemergten, gerenderten Configs, inklusive Listen-Wachstum und Duplikat-Erkennung.
  3. Der nötige Apply-Modus wird aus der verifizierten Allowlist berechnet und angezeigt; `.machine.install`-Änderungen sind als "wirkt erst beim nächsten Install/Upgrade" gekennzeichnet, ein zweiter `staged`-Apply wird abgelehnt statt still verworfen, und `.machine.network`-Diffs empfehlen `--mode=try` mit sichtbarem 60-Sekunden-Countdown.
  4. Betreiber legt wiederverwendbare Patches an (versioniert, append-only, ausschließlich Strategic Merge); derselbe Patch zweimal angewendet erzeugt dasselbe Ergebnis, und vorher lässt sich validieren und im Dry-Run prüfen.
  5. Config-Generierung für einen Node nutzt das in Phase 3 importierte Secrets-Bundle und einen pro Cluster gepinnten Talos-Versions-Contract.

**Research**: Nicht nötig — `machinery` liefert Merge, Diff und Validierung. Die Arbeit ist Verdrahtung plus Redaction-Test, keine Entdeckung. Research-Flag: *skip*.
**Release blockers owned**: CFG-02 🚫
**Note**: Constraint #6 — CFG-02 (Redaction) liegt zwingend in **derselben** Phase wie CFG-01 (Config-View). Die View *ist* das Leck; Redaction danach zu sequenzieren heißt, das Leck einmal auszuliefern.
**Plans**: TBD
**UI hint**: yes

### Phase 8: Provisioning — der Core Value

**Goal**: Eine blanke Maschine wird in der Oberfläche zum gesunden Cluster-Node.
**Depends on**: Phase 7 (und Phase 4 für die QEMU-Sandbox)
**Requirements**: PROV-01, PROV-02, PROV-03, PROV-04, PROV-05, PROV-06, PROV-07, PROV-08, PROV-09, PROV-10, PROV-11, PROV-12, PROV-13
**Success Criteria** (what must be TRUE):

  1. **Binärer Abnahmetest:** Eine blanke Maschine wird ein gesunder Cluster-Node, **ohne dass der Betreiber ein Terminal öffnet.**
  2. Betreiber entdeckt Maschinen per Subnetz-Scan auf `:50000` mit begrenzter Nebenläufigkeit oder per manueller IP-Eingabe; ein bereits konfigurierter Node an der Ziel-IP wird als solcher gemeldet, **nie** als "nichts gefunden". Beim Wizard-Start warnt die UI, dass eine vorhandene Disk-Installation die ISO überschattet und dass ohne DHCP kein Zero-Touch-Pfad existiert.
  3. Vor dem Apply zeigt holzkube UUID, MACs, Disks und Talos-Version, lässt Rolle, Ziel-Cluster und Install-Disk wählen (Größe, Modell, Seriennummer, Transport, System-Disk markiert), warnt bei geradzahliger Control-Plane-Anzahl, bietet optionales `--cert-fingerprint`-Pinning mit ehrlichem Text zur Unauthentifiziertheit des Maintenance-Mode — und **verifiziert unmittelbar vor dem Schreiben die UUID erneut**.
  4. Ein doppelter etcd-Bootstrap ist strukturell unmöglich (Pre-Flight `EtcdMemberList`, `O_CREAT|O_EXCL`-Lease, fsynced Intent-Record, Talos' `AlreadyExists`); der unklare Fall landet in einem eigenen Recovery-Flow statt in einem Retry.
  5. Die Wiederauftauch-Prüfung nach dem Disk-Install ist eine Drei-Wege-Probe mit verstrichener Zeit gegen ein erwartetes Budget — **nie ein Spinner**; ein geschlossener Browser-Tab verliert den Job nicht; `.machine.install.image` stammt aus **derselben** Schematic-ID wie die ISO; und "Talos healthy, Kubernetes NotReady" wird vor der CNI-Installation als normal erklärt statt rot dargestellt.

**Research**: **Ja** (Research-Flag Phase 6) — QEMU-Ausführung und Maintenance-Mode-Ressourcen-Sensitivität sind nur an echter Hardware/VM feststellbar. Teile davon werden bereits in Phase 4 beantwortet; was dort offen bleibt, gehört in die Planung dieser Phase.
**⚠️ ENTRY REQUIREMENTS (nicht verhandelbar, keine Stretch Goals)**:

  - **QEMU (Tier 2) funktioniert** — nachgewiesen in Phase 4. Docker kann Maintenance-Mode, Install und Upgrade strukturell nicht ausüben.
  - **Eine Maschine, die blank sein darf** — ein dritter Mini-PC oder eine VM. Ohne sie ist der Abnahmetest nicht durchführbar.
  Diese Phase wird nicht begonnen, solange beides nicht steht.
**Release blockers owned**: PROV-05 🚫, PROV-09 🚫, PROV-10 🚫
**Note**: Constraint #3 — der Cluster-Import gehört **nicht** hierher, sondern liegt bereits in Phase 3. Diese Phase konsumiert das importierte Bundle, sie erzeugt keins.
**Plans**: TBD
**UI hint**: yes

### Phase 9: Upgrades & etcd-Verwaltung

**Goal**: Betreiber fährt rollende Talos- und Kubernetes-Upgrades hinter einem Gate, das lieber blockiert als den Cluster zu stranden.
**Depends on**: Phase 8
**Requirements**: UPG-01, UPG-02, UPG-03, UPG-04, UPG-05, UPG-06, UPG-07, UPG-08, UPG-09, UPG-10, UPG-11, UPG-12, UPG-13, UPG-14
**Success Criteria** (what must be TRUE):

  1. Betreiber startet ein rollendes Talos-Upgrade über `LifecycleClient.Upgrade` (streaming), Node für Node; **vor jedem** Node wird das Health-Gate neu bewertet, zeigt seine Eingaben an, schließt Learner aus, prüft Raft-Konvergenz und Alarme und verweigert bei ≤2 stimmberechtigten Membern. Gesperrte Nodes werden übersprungen.
  2. Ein Talos-Upgrade, das die laufende Kubernetes-Version aus der Unterstützung fallen ließe, wird **blockiert** — nicht gewarnt — mit konkreter Handlungsanweisung. Die nötige Kette an Zwischen-Minor-Versionen wird berechnet und ausgeführt; es gibt keinen "latest"-Knopf und Pre-Releases sind gefiltert. Kubernetes-Upgrades laufen über denselben Pfad.
  3. Ein Node mit unbekannter `schematic_id` wird **nicht** geupgradet; holzkube bietet an, sie vom Node zu lesen. Kernel-Args-/GRUB-Drift blockiert den Ein-Klick-Pfad, weil die Upgrade-RPC Kernel-Args strukturell nicht tragen kann.
  4. Nach jedem Node wird deklariert-gegen-beobachtet verifiziert — "die API sagte OK" gilt nicht als Beweis. Betreiber kann nach dem aktuellen Node stoppen, und der UI-Text sagt ehrlich, was bereits passiert ist.
  5. Betreiber listet etcd-Member mit Hostname und UUID (nie mit rohen Hex-IDs), entfernt Member, zieht Snapshots (mit dokumentiertem Fallback ohne Quorum) und entfernt einen Node sauber aus dem Cluster: cordon/drain → reset → aus dem Inventar.

**Research**: **Ja** (Research-Flag Phase 7) — Kubernetes-Upgrade-Orchestrierung (prepull → static pods → kube-proxy → kubelet) wurde nicht vertieft; die Kompatibilitätsmatrix braucht eine gepflegte Datenquelle; Talos v1.14's Upgrade-Oberfläche ist ungeprüft.
**Parallel tracks**: 2 — etcd-Verwaltung (UPG-10 … UPG-13) ∥ Upgrade-Orchestrierung (UPG-01 … UPG-09, UPG-14).
**Release blockers owned**: UPG-02 🚫, UPG-03 🚫, UPG-06 🚫, UPG-07 🚫
**Note**: Constraints #4 und #5 — Cluster-Übersicht/etcd-Health und die Talos↔K8s-Matrix liegen bereits in Phase 3; ohne sie könnte das Health-Gate hier nicht existieren und die Reihenfolge würde siderolabs/talos#12398 selbst herstellen.
**Plans**: TBD
**UI hint**: yes

### Phase 10: Härtung & echter Hardware-Durchlauf

**Goal**: holzkube ist als Dauerbetrieb sicherbar, versionsehrlich — und einmal auf echtem amd64-Blech bewiesen.
**Depends on**: Phase 9
**Requirements**: OPS-01, OPS-02, OPS-03, OPS-04, OPS-05
**Success Criteria** (what must be TRUE):

  1. Ein Verifikationsdurchlauf auf **echter amd64-Hardware** ist erfolgt: Provisioning, Apply und Upgrade laufen dort, nicht nur gegen QEMU auf arm64.
  2. Betreiber sichert und restauriert den holzkube-Zustand per Subcommand und verifiziert die Integrität der Audit-Hash-Kette.
  3. holzkube läuft als Docker-/Compose-Setup mit non-root-Container und korrekten Volume-Rechten im Dauerbetrieb.
  4. Ein Node außerhalb der unterstützten Talos-Range (v1.12–v1.14) wird sichtbar markiert und blockiert versionsabhängige Aktionen; RCs sind ausdrücklich Opt-in. Der Check ist getestet, kein README-Satz.
  5. Der Betreiber ändert sein Passwort über die Settings-Oberfläche; der Sudo-Dialog erscheint und das darunterliegende Formular geht dabei nicht verloren. *(Aus Phase 1 hierher verschoben, 2026-08-28: der 428-Mechanismus und `POST /api/v1/account/password` stehen seit Phase 1, aber der Einstiegspunkt gehört zu dem Screen, den diese Phase baut. Siehe `phases/01-foundation-skeleton/01-UAT.md` Gap G-01-1.)*

**Research**: Nicht geflaggt.
**⚠️ ENTRY REQUIREMENT (nicht verhandelbar, kein Stretch Goal)**: **Ein Verifikationsdurchlauf auf echter amd64-Hardware.** Der Dev-Host ist `darwin/arm64`; QEMU läuft dort arm64-Talos nativ und kann amd64-Installer-Pfade **strukturell nicht** ausüben. Ohne Zugriff auf das Homelab ist diese Phase nicht abschließbar — das ist die einzige Phase, die das Homelab zwingend braucht.
**Release blockers owned**: OPS-05 🚫
**Plans**: TBD

## Abweichungen von der Research-Struktur

Zwei bewusste Abweichungen von `.planning/research/SUMMARY.md` § *Suggested phase structure*:

1. **Research-Phase 1 und 1b sind zu einer Phase verschmolzen (hier Phase 2).**
   *Grund:* GSD führt Phasen sequenziell aus und parallelisiert Pläne **innerhalb** einer Phase. Research markiert 1 ∥ 1b als echte Parallelität; als zwei getrennte GSD-Phasen wäre genau diese Parallelität verloren. Als zwei Tracks einer Phase wird sie tatsächlich realisiert. Der Image-Factory-Track hat null Talos-Abhängigkeit, also ist die Verschmelzung risikofrei.

2. **Eine neue Phase 4 (Walking Skeleton) ist eingeschoben.**
   *Grund:* ausdrückliche Entscheidung des Betreibers ("Horizontal Layers *plus* early walking skeleton"), gestützt von Research § *Phase Ordering Rationale*: "A walking skeleton should precede the polished Phase 6." Sie zieht den QEMU-Aufbau von "vor Phase 6 declared done" nach vorn — strikt stärker als Constraint #10, nicht dagegen. Sie trägt absichtlich kein Requirement und ist Wegwerf-Code.

*Nicht* abgewichen wurde bei der Streaming-Phase: Research markiert 3 ∥ 4, aber Streaming bleibt eine eigenständige, sequenzielle Phase, weil ihre Kürzbarkeit (designierte Schnittlinie) ein Entwurfsmerkmal ist, das eine Verschmelzung mit der Jobs-Engine zerstören würde.

## Harte Ordering-Constraints — Nachweis

| # | Constraint | Erfüllt durch |
|---|---|---|
| 1 | `talossim` vor jeder Feature-Arbeit | TRANS-06 in Phase 2; alle Feature-Phasen ≥ 3 |
| 2 | Jobs-Engine vor Provisioning | Phase 6 vor Phase 8 |
| 3 | Cluster-Import in der Inventar-Phase, nicht der Provisioning-Phase | INV-01 in Phase 3, nicht Phase 8 |
| 4 | Cluster-Übersicht / etcd-Health vor rollenden Upgrades | INV-10 in Phase 3, UPG-02 in Phase 9 |
| 5 | Talos↔K8s-Matrix vor der Talos-Upgrade-Phase; Forward-Gate ist Release-Blocker | INV-11 in Phase 3, UPG-06 🚫 in Phase 9 |
| 6 | Redaction in derselben Phase wie die Config-View | CFG-01 **und** CFG-02 🚫 beide in Phase 7 |
| 7 | Secrets-Bundle + Cert-Ablauf mit dem PKI-Store | INV-01 🚫 und INV-05 🚫 beide in Phase 3 |
| 8 | `schematic_id` am Node-Record ab dem ersten Schema | INV-04 🚫 in Phase 3, dem ersten Schema-Träger |
| 9 | UUID-Inventar, Multi-Cluster-Scoping, Deadlines, Fehlertaxonomie, `--dry-run`, Read-only-Lock: alles Fundament | INV-03/INV-12 (Phase 3), TRANS-04/FOUND-12 (Phase 2), FOUND-11 (Phase 1) |
| 10 | QEMU vor "Provisioning fertig"; ein echter amd64-Durchlauf vor v1 | QEMU in Phase 4 + Entry Requirement Phase 8; OPS-05 🚫 in Phase 10 |

## Release Blockers — Besitzverhältnisse

Die 16 mit **🚫** markierten Requirements sind Release-Blocker, keine Nice-to-haves. Keiner darf still verschoben werden.

| Blocker | Phase | Kurz |
|---------|-------|------|
| TRANS-06 🚫 | 2 | `talossim` — ohne ihn ist nichts verifizierbar |
| INV-01 🚫 | 3 | Import des bestehenden Clusters mit Fingerprint + Konnektivitätsbeweis |
| INV-03 🚫 | 3 | UUID-adressierte, flache Node-Records |
| INV-04 🚫 | 3 | `schematic_id` am Node-Record ab Schema 1 |
| INV-05 🚫 | 3 | Restlaufzeit des Client-Zertifikats sichtbar |
| INV-07 🚫 | 3 | `HealthLevel` + `stale_since` pro gelesenem Feld |
| JOB-07 🚫 | 6 | Reset-Dialog mit Wipe-Scope, Reboot-Kontrolle, Disk-Enumeration |
| CFG-02 🚫 | 7 | Serverseitige Redaction an allen fünf Aufrufstellen |
| PROV-05 🚫 | 8 | UUID-Re-Verifikation unmittelbar vor dem Apply |
| PROV-09 🚫 | 8 | Drei-Wege-Wiederauftauch-Probe mit Zeitbudget |
| PROV-10 🚫 | 8 | Vier-Schichten-Bootstrap-Guard |
| UPG-02 🚫 | 9 | Volles Health-Gate vor jedem Node |
| UPG-03 🚫 | 9 | Kein Upgrade bei unbekannter `schematic_id` |
| UPG-06 🚫 | 9 | Kubernetes-Forward-Kompatibilitäts-Gate |
| UPG-07 🚫 | 9 | Deklariert-gegen-beobachtet nach jedem Upgrade |
| OPS-05 🚫 | 10 | Ein Verifikationsdurchlauf auf echter amd64-Hardware |

## Research-Flags

| Phase | Research bei der Planung? | Herkunft |
|-------|---------------------------|----------|
| 1 — Foundation | Nein (Standard-Patterns) | Research-Flag *Phase 0: skip* |
| 2 — Transport / `talossim` / Factory | **Ja** | Research-Flag *Phase 1: needs research* |
| 3 — Inventar / Import / Health | **Ja** | Research-Flag *Phase 2: needs research* |
| 4 — Walking Skeleton | **Ja** | QEMU-Ausführung + Maintenance-Mode-Sensitivität (aus Research-Flag *Phase 6* vorgezogen) |
| 5 — Streaming | Nein (Standard-Patterns) | Research-Flag *Phase 3: skip* |
| 6 — Jobs-Engine | Nicht geflaggt | — |
| 7 — Config-Domain | Nein (Standard-Patterns) | Research-Flag *Phase 5: skip* |
| 8 — Provisioning | **Ja** | Research-Flag *Phase 6: needs research* |
| 9 — Upgrades / etcd | **Ja** | Research-Flag *Phase 7: needs research* |
| 10 — Härtung | Nicht geflaggt | — |

## Echte Parallelität

`parallelization: true`. Diese Tracks sind real, nicht aspirational:

| Phase | Parallele Tracks |
|-------|------------------|
| 1 | `store`/`audit`/`auth` ∥ Vite-Scaffold + `embed.FS` ∥ TLS + Config-Loading |
| 2 | Transport-Naht + `talossim` ∥ Image-Factory-Client (die Research-Parallelität *1 ∥ 1b*) |
| 3 | Backend ∥ Frontend, sobald die `Field[T]`-Response-Shape fixiert ist |
| 9 | etcd-Verwaltung ∥ Upgrade-Orchestrierung |

Bewusst **nicht** parallelisiert: Research führt Streaming ∥ Jobs (3 ∥ 4). Hier bleiben sie getrennt und sequenziell, damit Streaming als Schnittlinie kürzbar bleibt.

## Progress

**Execution Order:**
Phasen laufen in numerischer Reihenfolge: 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9 → 10

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation Skeleton | 6/6 | Complete    | 2026-08-28 |
| 2. Transport Seam, `talossim` & Image Factory | 21/21 | In Progress|  |
| 3. Inventar, Cluster-Import & Health | 0/TBD | Not started | - |
| 4. Walking Skeleton (Wegwerf) | 0/TBD | Not started | - |
| 5. Streaming | 0/TBD | Not started | - |
| 6. Jobs-Engine & Node-Aktionen | 0/TBD | Not started | - |
| 7. Config-Domain | 0/TBD | Not started | - |
| 8. Provisioning — der Core Value | 0/TBD | Not started | - |
| 9. Upgrades & etcd-Verwaltung | 0/TBD | Not started | - |
| 10. Härtung & echter Hardware-Durchlauf | 0/TBD | Not started | - |

## Coverage

- v1-Requirements gesamt: **95**
- Auf Phasen abgebildet: **95**
- Ohne Phase: **0** ✓
- Doppelt abgebildet: **0** ✓

Vollständige Zuordnung: `.planning/REQUIREMENTS.md` § Traceability.

---
*Roadmap erstellt: 2026-08-27*
