---
gsd_state_version: 1.0
milestone: v1.15
current_phase: 3
current_phase_name: "Prometheus-/metrics (v1.15)"
status: milestone-closed
stopped_at: v1.15 vollstaendig, CI gruen, Routen-Audit abgearbeitet (Fenster 92-95 zu). OPS-05 bleibt der einzige Release-Blocker (Fenster 87)
last_updated: "2026-09-13T11:00:00.000Z"
last_activity: 2026-09-13
last_activity_desc: CI red for nine commits (fixed); a route audit found a confirmation the server never enforced
state_head: a474d2522823cbfb436ee720dee35494890281a3
progress:
  total_phases: 3
  completed_phases: 3
  total_plans: 0
  completed_plans: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-08-27)

**Core value:** Eine neue Maschine wird komplett in der UI zum Cluster-Node — ohne `talosctl`, ohne Omni.
**Current focus:** v1.15 ist fertig. Offen bleibt OPS-05 🚫 (Hardware) — der einzige Release-Blocker.

## Current Position

Phase: v1.15 abgeschlossen — alle drei Phasen
Plan: `.planning/MILESTONE-v1.15.md`, Abschluss in `.planning/MILESTONE-v1.15-CLOSEOUT.md`
Status: abgeschlossen; `v1.15.0-beta.1` (linux/amd64) gebaut und übergeben
Last activity: 2026-09-12 — v1.15 Phase 3 (Prometheus-`/metrics`) abgeschlossen

Progress: v1.15 [███] 3 of 3 phases · v1.14 [█████████▓] gebaut, OPS-05 offen

**v1.15 Phase 1 (Support-Bundle) ist fertig.** `internal/support` sammelt pro
Node Facts, Services, Versionen, Disks, Links, Extensions, etcd-Status, Logs,
`dmesg` und die redigierte MachineConfig in ein `tar.gz`, plus Audit-Tail,
Instanz-Metadaten und ein Manifest, das jede Lücke führt. Es gibt es als Route
und als Subkommando; das Subkommando sagt in seinem eigenen Manifest, dass es
keinen Node erreicht hat, weil es den Store-Lock hält. Der Abnahmetest walkt
Entropie über **jede** Datei im Archiv und sucht den echten CA-Schlüssel des
simulierten Clusters samt seinem Base64-Körper ohne PEM-Rahmen. Ein Lauf gegen
einen echten Store hat eine nichtssagende Fehlermeldung in `inventory.Connect`
gefunden und ersetzt. Siehe `.planning/phases/v1.15-01-support-bundle/01-SUMMARY.md`.

**v1.15 Phase 2 (COSI-Watches) ist fertig und Fenster 79 ist geschlossen.**
Pro Node laufen zwei Schleifen: der Heartbeat und ein `WatchKind` über fünf
Ressourcen-Arten. Der Watch trägt keine Daten, nur die Aussage, dass sich etwas
geändert hat; gelesen wird weiter im bestehenden Durchlauf. **Der Heartbeat ist
nicht nur geblieben, er ist jetzt der Watchdog**, und das ist gemessen und nicht
vorsichtshalber: eine Subscription gegen einen gestoppten Node bleibt offen,
still und fehlerfrei, weil cosi-projects Client sie fünfzehn Minuten lang
heimlich aus ihrem Bookmark wieder aufbaut. `config.MachineConfig` ist über
diesen Transport gar nicht beobachtbar und deshalb draußen, mit dem Grund im
Code. Nebenbei sind zwei alte, stille Supervisions-Fehler gefallen: ein
Lesevorgang vor `Start()` verhinderte jede Überwachung, und `Supervise()` band
den Supervisor an den Request-Context des HTTP-Handlers. Siehe
`.planning/phases/v1.15-02-cosi-watches/02-SUMMARY.md`.

**Die Routen-Prüfung ist jetzt ein Test und keine Beobachtung mehr.**
`TestEveryRouteIsReachableFromTheInterface` läuft die Routen-Tabelle gegen
`web/src`; eine Route ohne Einstieg ist rot, es sei denn, sie steht mit
Begründung in `notInTheInterface`. Zwei Grenzen stehen im Test selbst, weil sie
gemessen und nicht vermutet sind: ein Pfad, der nur in einem **Kommentar**
vorkommt, gilt als erwähnt (das Passwort-Beispiel fällt deshalb durch), und
bedingt registrierte Routen (die zwei OIDC-Routen) sind unsichtbar, weil die
Tabelle mit leeren `Deps` gelaufen wird. **Testdateien sind ausgenommen**, und
das war der Unterschied zwischen einem Wächter, der funktioniert, und einem, der
nicht funktioniert: mit ihnen drin blieb das entfernte Support-Bundle
unentdeckt, weil `clusters.test.tsx` den href behauptet. Zwei der drei
historischen Auslassungen werden reproduzierbar gefangen.

**Eine systematische Routen-Prüfung nach dem zweiten Fund hat einen dritten
Fall geliefert — und dahinter ein Loch, das kein Einstiegsproblem war.** Alle
61 Routen gegen `web/src` geprüft: drei ohne Einstieg, zwei davon zu Recht (der
OIDC-Callback ist ein Browser-Redirect, `/metrics` ein Scraper-Endpunkt).

Die dritte war `POST /api/v1/machines/{id}/remove-from-cluster` — seit Phase 9
vorhanden, aus der Oberfläche unerreichbar, also die ganze etcd-Arbeit jener
Phase nur per curl bedienbar (Fenster 95). Beim Bauen des Dialogs fiel
**Fenster 94** auf: die Regel im Confirm-Handler lautete
`if action == node.reset` — geschrieben, als Reset die einzige zerstörende
bestätigungspflichtige Aktion war. `node.remove-from-cluster` hat „keine
Eingabe nötig" **geerbt, indem es nicht erwähnt wurde**: der Browser fragte den
Hostnamen ab, der Server gab jedem ein Token, der ohne einen fragte. Die Regel
ist jetzt eine Tabelle, ein fehlender Eintrag eine Verweigerung statt eines
Defaults, und vier Tests halten sie — zwei davon lesen per AST jede
`Confirmer.Check`-Stelle im Paket.

Dazu: `/metrics` war völlig unauffindbar (kein Login nötig, also auch kein
Link), und die Einstellungsseite nennt es jetzt samt Scrape-Config, Grenze und
der Tatsache, dass eine abgelaufene Zertifikatslaufzeit negativ und nicht
fehlend ist.

**CI war neun Commits lang rot, und niemand hat hingesehen — diese Sitzung
nicht, die sieben davon selbst gepusht hat (Fenster 92).** Seit Lauf 12
schlug `Build, lint and test` fehl: Läufe 12–19 an genau den golangci-lint-
Befunden, die Fenster 77 als „nicht ausführbar" führte, Lauf 20 an einem Flake.
Die Zeile *„Required status check 'Build, lint and test' is expected"* stand bei
jedem Push im Ausgang und wurde jedes Mal als Branch-Protection-Rauschen
gelesen. **Ein lokal grüner Lauf ist kein CI-Lauf:** CI fährt `-race` und einen
gepinnten Linter, und beides hat hier Dinge gefunden, die lokal grün waren.

**Der Flake war ein echter Fehler im Simulator (Fenster 93), und zwar genau
der, gegen den `ip_changes_on_reboot` existiert.** `severListener` schloss erst
die Verbindungen und dann den Listener — dazwischen nimmt die Serve-Schleife
eine Verbindung an, die der Kernel eine Mikrosekunde vorher fertig gehandshaked
hat, und bedient sie **auf einer Adresse, die der Node aufgegeben hat**. Genau
dort landet der gRPC-Client, der nach dem Abriss sofort neu wählt. Nachgewiesen
statt vermutet: 50 ms in das Fenster gelegt → fünf von fünf Läufen rot.
Behoben mit einem `severed`-Flag, das aus dem Sever einen Zeitpunkt statt eines
Intervalls macht, plus zwei internen Tests, die ohne den Fix rot sind.

**Der Linter lief zum ersten Mal, und er hat ein Sicherheitsloch gefunden, das
seit Phase 8 offen war.** `golangci-lint` 2.13.2 gegen go1.26 meldete 45
Befunde; heute sind es null, und jede Ausnahme in `.golangci.yml` nennt Regel,
Ort und Grund.

Der wichtigste Befund war **Fenster 90**: der Fingerprint, den ein Betreiber von
der Konsole einer Maschine im Maintenance-Modus abliest, wurde erhoben,
angezeigt, bis in `NewMaintenanceClient` weitergereicht — und **nie verglichen**.
Auf diesem Pfad gibt es keine Cluster-PKI, der Fingerprint ist der einzige
Vertrauensanker, und der Aufruf, für den der Pfad existiert, ist
`ApplyConfiguration`. Gefunden hat der Linter nicht das Loch, sondern sein
Fossil: eine ungenutzte `normalise()`, die einen Fingerprint für einen Vergleich
trimmte, den nie jemand geschrieben hatte. Repariert, mit vier Tests — und
`G123` fand direkt danach den Fehler im frischen Pin: `VerifyPeerCertificate`
wird bei einer **wiederaufgenommenen** TLS-Session gar nicht aufgerufen, was
einen Pin ergibt, der beim ersten Handshake hält und danach nie wieder.

**Fenster 89 ist zu, und der gesamte Go-Testlauf ist erstmals in dieser Umgebung
grün.** Beide dauerhaft roten Tests waren Tests, die eine Eigenschaft der
Maschine prüften statt eine des Codes — ein Hostname, der nicht auflöst, hängt
von der DNS-Auflösung des Hosts ab; eine Kalibrierung unter Last, gemessen
danach, hängt von der Last ab.

**v1.15 Phase 3 (Prometheus-`/metrics`) ist fertig, und damit der ganze
Milestone.** Neun Metrik-Familien, ohne zusätzliche Abhängigkeit, ohne Session
(ein Scraper kann sich nicht anmelden) und an derselben Hosts-Allowlist wie
alles andere. Kein Node wird je ein Label-Wert: die Kardinalität ist eine
Funktion der Cluster-Anzahl und wächst gar nicht mit der Flotte, und ein Test
misst das an 3 gegen 300 Nodes. Die Rest-Laufzeit des Zertifikats ist
vorzeichenbehaftet, weil ein abgelaufenes Zertifikat ein benannter Zustand ist
und keine fehlende Metrik. Gescrapt und **von einem echten Prometheus-Parser**
gelesen, nicht nur getestet. Siehe
`.planning/phases/v1.15-03-metrics/03-SUMMARY.md`.

**v1.15 ist autonom definiert und nicht vom Betreiber bestätigt.**
`.planning/MILESTONE-v1.15.md` nennt die drei Phasen, die Auswahlregel (alles,
was echte Hardware braucht, ist draußen) und sechs Rückstau-Punkte mit je einem
Grund, warum sie es nicht sind. Wenn der Betreiber widerspricht, widerspricht er
einem Dokument und nicht einem Diff.

**Der Milestone ist gebaut und nicht abnahmefähig, und das ist eine Aussage
über die Umgebung statt über den Code.** Jede der zehn Phasen ist gegen
`internal/talossim` ausgeführt; **keine Zeile davon ist je auf einer echten
Maschine gelaufen**. Der Verifikationsdurchlauf auf amd64-Blech, den Phase 10
als nicht verhandelbare Eintrittsbedingung führt, hat nicht stattgefunden: dem
Ausführungshost fehlen Homelab, QEMU und `/dev/kvm`.

**OPS-05 🚫 bleibt offen.** Es ist der einzige offene Release-Blocker des
Milestones. Fenster 87 ist sein Sammelpunkt und schließt gemeinsam mit 82 (die
Provisioning-Abnahme), 84 (die generierte MachineConfig als Anweisung) und 85
(das Upgrade) — die vier beschreiben einen einzigen Durchlauf: eine blanke
amd64-Maschine wird über den Wizard zu einem Node, bekommt eine Konfiguration,
wird geupgradet, und die dabei gemessenen Zeiten ersetzen zwei geratene
Konstanten.

**Was sonst gebaut und nie ausgeführt wurde:** die Installations-Messung und
Tier 2 aus Phase 4 (80, 81), Tier 1 aus Phase 3 (76) und `golangci-lint` gegen
die Ziel-Go-Version (77). Das Container-Image (88) ist inzwischen gebaut und
gelaufen; der Lauf hat drei echte Fehler gefunden.

**Ein Befund aus Phase 8 betrifft Phase 6 und 7 rückwirkend und ist nicht
reparierbar:** acht auditierte Aktionen hatten keinen Allowlist-Eintrag, also
wurde jeder Parameter jedes Reboots, Resets und Config-Applys als `<redacted>`
archiviert — unter anderem der Wipe-Umfang eines Resets. Der Mechanismus ist
repariert und durch einen Test an der Routen-Tabelle gehalten; die bereits
geschriebenen Zeilen bleiben inhaltslos, weil D-16 keinen Löschpfad definiert
und eine Migration die Hash-Kette bräche. Fenster 83.

**Phase 9 ist erfüllt und auf keiner echten Maschine gelaufen.** Alle fünf
Erfolgskriterien sind gegen `talossim` ausgeführt; kein rollendes Upgrade hat je
eine Maschine berührt. Fenster 85 nennt die drei Annahmen, die davon abhängen —
die Form von `LifecycleService.Upgrade` in echtem Talos, die Wiederauftauch-Zeit
(`ReappearBudget` ist wie Phase 8s eine geratene Zahl) und die kuratierte Liste
der Kernel-Argumente, die Talos sich selbst setzt. Fenster 86 hält fest, dass
der Kubernetes-Pfad die Images in die MachineConfig schreibt statt über die
Kubernetes-API zu orchestrieren, was kube-proxy unberührt lässt.

**Phase 8 ist gebaut und Erfolgskriterium 1 ist unverifiziert.** Die beiden
Eintrittsbedingungen der Phase — QEMU aus Phase 4, und eine Maschine, die blank
sein darf — sind unerfüllt geblieben, und die Phase wurde trotzdem gebaut, weil
alles außer dem binären Abnahmetest gegen `talossim` ausführbar ist. Keine
blanke Maschine ist irgendwo zu einem Node geworden. Fenster 82 nennt die drei
Annahmen, die `talossim` bestätigt, weil `talossim` sie eingebaut hat
(Ressourcen-Sensitivität im Maintenance-Mode, die Länge der Installationsstille,
die Form von Talos' `AlreadyExists`); Fenster 84 nennt die Konfiguration, die
als Dokument geprüft und als Anweisung an eine Maschine ungeprüft ist.
`ReappearBudget` ist weiterhin Phase 4s Platzhalter von acht Minuten.

**Ein Befund aus Phase 8 betrifft Phase 6 und 7 rückwirkend:** acht auditierte
Aktionen hatten keinen Allowlist-Eintrag, also wurde jeder Parameter jedes
Reboots, Resets und Config-Applys als `<redacted>` archiviert — unter anderem
der Wipe-Umfang eines Resets. Der Mechanismus ist repariert und durch einen
Test an der Routen-Tabelle gehalten; die bereits geschriebenen Zeilen bleiben
inhaltslos, weil D-16 keinen Löschpfad definiert. Fenster 83.

**Phase 4 ist nicht abgehakt.** Ihr Produkt ist eine Messung, und niemand hat
gemessen: der Ausführungshost hat weder QEMU noch `/dev/kvm` noch verschachtelte
Virtualisierung. Gebaut sind `sandbox/cmd/walking-skeleton` und
`talos-sandbox --provider qemu`; gelaufen sind sie nie. Die vier Unbekannten,
die Phase 4 für Phase 8 entschärfen sollte, sind weiterhin Unbekannte —
insbesondere die Installationsstille, um die herum Phase 8 einen
Fortschrittsindikator entwerfen muss. Fenster 80 und 81; `04-SUMMARY.md` nennt
den Befehl, der beide schließt.

**Phase 3, was offen bleibt:** Erfolgskriterium 5 (TRANS-08) ist **nicht**
erfüllt. Der Tier-1-Provisioner und der reale Contract-Transport existieren und
überspringen sich sichtbar; ausgeführt wurden sie nie, weil in dieser Umgebung
kein Docker-Daemon läuft. Fenster 76 führt das, samt der Einschränkung, dass
sieben der neun Szenarien auf einem echten Node ohnehin nicht induzierbar sind.
Fenster 77 führt, dass `golangci-lint` in dieser Umgebung zu alt für die
Ziel-Go-Version ist und nicht lief.

## Performance Metrics

**Velocity:**

- Total plans completed: 40
- Average duration: —
- Total execution time: 0.0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01 | 6 | - | - |
| 02 | 31 | - | - |

**Recent Trend:**

- Last 5 plans: —
- Trend: —

*Updated after each plan completion*
**Per-Plan Metrics:**

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 01 P01 | 19 min | 4 tasks | 48 files |
| Phase 01 P02 | 18 min | 3 tasks | 20 files |
| Phase 01 P03 | 23 min | 3 tasks | 20 files |
| Phase 01 P04 | 22 min | 3 tasks | 19 files |
| Phase 01 P05 | 27 min | 3 tasks | 47 files |
| Phase 01 P06 | 28 min | 3 tasks | 23 files |
| Phase 02 P01 | 14 min | 2 tasks | 16 files |
| Phase 02 P02 | 31 min | 4 tasks | 17 files |
| Phase 02 P08 | 18 min | 3 tasks | 9 files |
| Phase 02 P04 | 32 min | 3 tasks | 25 files |
| Phase 02 P03 | 38 min | 3 tasks | 12 files |
| Phase 02 P06 | 33 min | 3 tasks | 19 files |
| Phase 02 P05 | 45 min | 4 tasks | 19 files |
| Phase 02 P07 | 28 min | 5 tasks | 26 files |
| Phase 02 P09 | 20 min | 3 tasks | 10 files |
| Phase 02 P10 | 7 min | 2 tasks | 2 files |
| Phase 02 P11 | 11 min | 3 tasks | 7 files |
| Phase 02 P12 | 22 min | 3 tasks | 17 files |
| Phase 02 P13 | 9 min | 2 tasks | 8 files |
| Phase 02 P14 | 45 min | 3 tasks | 5 files |
| Phase 02 P15 | 10 min | 2 tasks | 2 files |
| Phase 02 P16 | 40 min | 3 tasks | 5 files |
| Phase 02 P17 | 34 min | 4 tasks | 10 files |
| Phase 02 P18 | 23 min | 3 tasks | 10 files |
| Phase 02 P19 | 29 min | 4 tasks | 8 files |
| Phase 02 P20 | 33 min | 3 tasks | 8 files |
| Phase 02 P21 | 23 min | 4 tasks | 6 files |
| Phase 02 P22 | 90 min | 3 tasks | 13 files |
| Phase 02 P23 | 30 min | 3 tasks | 12 files |
| Phase 02 P24 | 23 min | 3 tasks | 9 files |
| Phase 02 P25 | 10 min | 3 tasks | 5 files |
| Phase 02 P26 | 10 min | 3 tasks | 2 files |
| Phase 02 P27 | 10 min | 3 tasks | 1 files |
| Phase 02 P28 | 16 min | 3 tasks | 3 files |
| Phase 02 P29 | 9 min | 3 tasks | 1 files |
| Phase 02 P30 | 19 min | 3 tasks | 1 files |
| Phase 02 P31 | 14 min | 3 tasks | 3 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- [Roadmap]: Horizontal Layers **plus** early walking skeleton — Phase 4 fährt den hässlichen End-to-End-Weg gegen QEMU, um vier Unbekannte früh zu entschärfen. Wegwerf-Code, kein PROV-Requirement.
- [Roadmap]: Research-Phasen 1 und 1b zu Phase 2 verschmolzen — GSD parallelisiert Pläne innerhalb einer Phase, nicht Phasen; nur so wird die Parallelität real.
- [Roadmap]: Phase 5 (Streaming) ist die **designierte Schnittlinie** — sie wird gekürzt, wenn v1 in Gefahr gerät, nicht Phase 8.
- [Roadmap]: Persistenz bleibt `0600`-Dateien hinter einem entity-shaped `store`-Interface; kein SQLite in v1 (Research-Ruling §1).
- [Roadmap]: Unterstützte Talos-Range v1.12–v1.14, RCs Opt-in (Research-Ruling §5).
- [Phase 01]: Audit-Hash-Kette über kanonisches JSON des Satzes ohne hash-Feld (Task-2-Gate): Kette überlebt Encoder- und Struct-Änderungen, Pflicht wegen D-16 (unbegrenzte Aufbewahrung)
- [Phase 01]: @biomejs/biome auf 2.5.11 statt 2.5.10 gepinnt — explizite Betreiber-Anweisung am Task-1-Package-Gate
- [Phase 01]: Route-Tabelle wird im Composition Root (cmd/holzkubed/main.go) zusammengesetzt, nicht in router.go: die Plan-Form wäre in Go ein Import-Zyklus. router.go behält Route-Typ, Destructive-Semantik, Mounting und Chain
- [Phase 01]: nosurf nicht aufgenommen — die gewählte CSRF-Kombination braucht keine Token-Bibliothek, und go mod tidy entfernt ein ungenutztes Modul
- [Phase 01]: Audit-params bleiben in Phase 1 leer; Erfassung landet zusammen mit der Allowlist-Redaction in Plan 03, sonst stünden Setup- und Login-Passwörter im dauerhaft aufbewahrten Log
- [Phase 01]: Audit-Middleware ist fail-closed: eine Mutation, deren Intent nicht durable wird, wird abgelehnt statt ungeloggt ausgeführt
- [Phase 01]: Prozess-flock (LOCK_EX|LOCK_NB) auf das Datenverzeichnis: ein zweiter holzkubed startet nicht; der Kernel gibt den Lock bei kill -9 frei, also blockiert eine abgestürzte Instanz das Verzeichnis nicht dauerhaft
- [Phase 01]: Per-Entity-Mutex-Map mit Refcount-Abbau (Schluessel kind/id) statt eines Mutex pro Entity-Store: verschiedene Records blockieren sich nicht, und die Map waechst nicht unbegrenzt ueber Millionen Session-Writes
- [Phase 01]: Der Rechte-Guard verweigert den Start und repariert nie automatisch: ein stilles chmod wuerde verbergen, dass die Dateien lesbar waren. Alle Verstoesse werden gesammelt und mit Reparaturbefehl ausgegeben
- [Phase 01]: backup liegt als eigenes Paket internal/store/migrate/backup, weil Go genau ein Paket pro Verzeichnis erlaubt und die Plan-Aufrufstelle backup.Create(...) lautet
- [Phase 01]: Der Crash-Hook wird ausschliesslich aus einer Testdatei gesetzt, damit atomic.go kein testing importiert; ein injizierter Crash ueberspringt bewusst das defer-Cleanup, weil kill -9 das auch tut
- [Phase 01]: Der Architektur-Guard (FOUND-07) ist ein ausgefuehrter AST-Test statt einer Review-Regel: er nimmt internal/store, internal/tlsx und internal/audit aus, prueft store.go aber separat und behauptet, mindestens 15 Dateien gescannt zu haben
- [Phase 01]: Genesis is sha256 of a domain string, not an empty prev_hash: an empty anchor is indistinguishable from a stripped field
- [Phase 01]: Compression failure is fatal to the audit write, consistent with plan 02 refusing to start on a wrong-permission data directory
- [Phase 01]: Session tokens are truncated inside Logger.append, where the record is sealed, so no caller can forget
- [Phase 01]: The recorded failure reason is the taxonomy code read out of the problem response and shape-checked, never handler text
- [Phase 01]: Startup verification covers the current and the last rotated file only, so its cost does not grow with an archive that is never shortened
- [Phase 01]: A password change invalidates every session except the calling one — The change is nearly always a reaction to a suspicion; the sessions that might not be the operator's are the other ones, and ending their own punishes the correct instinct. Answers the open question in 01-CONTEXT.md Claude's Discretion.
- [Phase 01]: Logging in does not open the sudo window; only POST /api/v1/auth/sudo does — ARCHITECTURE.md:820 phrases the gate as 'session older than N minutes', which would let a fresh login through. The plan's acceptance criteria require a just-logged-in second session to receive 428, and the stricter reading is what makes the window mean anything against a cookie stolen soon after a login.
- [Phase 01]: argon2id calibration raises iterations only, is floored at DefaultParams, and measures the fastest of three runs — A single measurement is inflated by a cold cache and shipped parameters 18 percent below the 250ms target while reporting success. Fastest-of-three errs towards more iterations. Memory is never lowered: it is what makes argon2id expensive to attack with custom hardware.
- [Phase 01]: The sudo gate buffers a destructive route's response until the window has been refreshed — scs v2.9.0 commits the session on the first response write, so a post-handler session mutation is computed and discarded with no visible failure. Buffering is confined to destructive routes, whose responses are a status code and a few hundred bytes.
- [Phase 01]: The dashboard shows only what the System Status Contract defines: setup_required and audit_chain. Version, bind address and data directory do not exist on that endpoint, and inventing them client-side would break against the real server.
- [Phase 01]: The sudo replay reuses the request init object built for the original call, so an unchanged body and unchanged headers are structural rather than asserted.
- [Phase 01]: GET /api/v1/auth/me is exempt from the 401 interceptor because its 401 means not-signed-in, which is an answer rather than an expiry.
- [Phase 01]: next-themes was removed and the copied shadcn sonner component rewired to the project's own theme hook: D-11 specifies one theme with two values and no second theming layer.
- [Phase 01]: shadcn 4.19 ships Radix as the unified radix-ui package, so the plan's literal @radix-ui/ grep does not match while its intent holds.
- [Phase 01]: The theme is recomputed from localStorage and matchMedia on every read rather than cached, so the reload case D-11 requires is proven by storage and not by a cache.
- [Phase 01]: GET /api/v1/system/status was left unextended: version, bind address and data directory are not added to the contract — The endpoint answers before authentication and the data directory path is the most sensitive string the process holds; no consumer remained inside phase 1 to shape or test the fields; docs/api-contract.md is what five parallel plans coded against. Recorded as deferred-items.md item 1.
- [Phase 01]: The self-signed certificate is a leaf, not a CA, and its fingerprint is printed in the browser format — The inherited generator set IsCA true with KeyUsageCertSign, contradicting D-04. Colon-separated upper-case hex is byte-identical to openssl x509 -fingerprint -sha256 and to what browsers show, so the comparison the README asks for needs no conversion step.
- [Phase 01]: The golangci-lint gate landed green: exclusions name a rule and a place, and the eleven residual findings were resolved rather than excluded — A gate that fails on the day it arrives is bypassed within a week. gosec stays fully on for production code; G304 is exempt only in the three packages that own files by design, and the default issue caps are lifted so the count means something.
- [Phase 02]: Die kuratierte Broken-Version-Tabelle bleibt leer — Der einzige Forschungskandidat (v1.9.0, metal-installer loest angeblich nicht auf) wurde waehrend der Ausfuehrung zweimal live nachgeprueft und antwortet heute mit 200. Ein Eintrag waere eine ungepruefte Behauptung in genau der Tabelle, deren einziger Wert die Pruefbarkeit jedes Eintrags ist. Der Lookup ist gegen eine synthetische Tabelle bewiesen, der erste echte Eintrag braucht nichts weiter.
- [Phase 02]: Der Semver-Vergleich ist lokal implementiert statt golang.org/x/mod/semver zu importieren — NewestStable filtert Prereleases vorher heraus, es bleiben drei Ganzzahlen. Der Plan schreibt fest, dass keine neue Modul-Anforderung hinzukommt; go.mod und go.sum sind unveraendert.
- [Phase 02]: Eine Registry, die nicht geantwortet hat, ist kein Schematic, das nicht baubar ist — Nur wenn jede Repository-Kandidatin 404 antwortet, ist das ErrSchematicNotBuildable; Transportfehler, 5xx und Auth-Challenges bleiben ErrUpstreamUnavailable und damit wiederholbar. Sonst schickt ein Ausfall den Betreiber los, etwas zu reparieren, das nicht kaputt ist.
- [Phase 02]: [Phase 02]: model.Schematic gains ProbeReason, gesetzt nur bei ErrSchematicNotBuildable — eine Sonde, die die Factory nicht erreicht hat, sagt nichts ueber das Schematic; ein dort eingetragener Grund laese sich als Aussage. Additiv und ohne Schema-Bump: ein aelterer Satz dekodiert mit leerem Grund, was genau seine Bedeutung ist.
- [Phase 02]: [Phase 02]: Der Live-Warntext ist der Serversatz woertlich, und TestWarningDetailsMatchTheUI liest die Komponente von der Go-Seite — 'die beiden koennen nicht widersprechen' wird erzwungen statt behauptet. vitest darf nicht ausserhalb von web/ lesen, ohne die Dateisystem-Allowlist des Bundlers zu lockern.
- [Phase 02]: [Phase 02]: Die Architektur im Asset-Panel wird in localStorage gemerkt statt vorbelegt — holzkube wird auf arm64 entwickelt und zielt auf amd64, eine feste Vorbelegung ist ein Fehler, der nur auf fremden Maschinen auftritt.
- [Phase 02]: [Phase 02]: Die Phase-1-Behauptung der Sidebar, die Navigation sei vollstaendig, ist korrigiert statt stehengelassen: eine Anforderung, die einen Bildschirm benennt, gewinnt gegen die Behauptung, die Liste sei geschlossen.
- [Phase 02]: [Phase 02]: Die Deadline-Policy ist bestaetigt (option-a): Tabelle unveraendert, Stream/WriteTimeout-Kollision wird Phase-5-Entry-Blocker, keine streamende HTTP-Route in Phase 2. Weil sich nichts geaendert hat, bleiben die ExpectedClient-Strings und docs/talossim.md byte-identisch — 02-03s Doku-Test faengt Zeilendrift, nicht Prosadrift.
- [Phase 02]: [Phase 02]: Ob ein Node geantwortet hat, entscheidet die Praesenz von Response-Trailern, nicht der Statuscode. Empirisch gegen talossim geprueft: etcd_downs serverseitiges Unavailable traegt einen Trailer, eine abgewiesene Verbindung keinen. Der Statuscode ist in beiden Faellen Unavailable, und ein Klassifikator, der ihn allein liest, oeffnet den Breaker eines erreichbaren Nodes.
- [Phase 02]: [Phase 02]: Der Breaker zaehlt KindUnreachable UND KindTimeout als Transportfehler. Der Plantext nannte nur unreachable, aber go_silent erzeugt ein Deadline-Ende und seine Registry-Erwartung ist, dass der Breaker oeffnet. KindRejected laesst den Zaehler unveraendert — das ist etcd_down.
- [Phase 02]: [Phase 02]: Die Policy sitzt auf gRPC-Client-Interceptoren statt auf Wrapper-Methoden: eine Regel, die pro Methode erinnert werden muss, wird in genau einer vergessen — und das ist die, die in Produktion ewig haengt.
- [Phase 02]: [Phase 02]: KindRejected wird auf kein upstream.node-*-Token abgebildet. Die upstream-Familie heisst 'hat nicht geantwortet' und traegt HTTP 502; ein Node, der die Anfrage gehoert und abgelehnt hat, hat geantwortet. Was eine Ablehnung am HTTP-Rand wird, entscheidet Phase 6 mit der ersten Route davor.
- [Phase 02]: A SecureBoot request whose SecureBoot installer names do not resolve is refused with no installer reference; the ordinary installer is never substituted -- substituting reintroduces the ISO/installer drift G-02-4 found, silently, in the one place an operator cannot see it
- [Phase 02]: registryRefused is true for 400 and 404 only; 401, 403, 429 and every 5xx stay retryable -- the probe verdict is written once at creation with no re-probe path, so a throttle filed as not-buildable is a permanent, unclearable accusation
- [Phase 02]: The live installer-name matrix is recorded in live_test.go and the SUMMARY, never transcribed into fake_test.go -- the fixture's v1.9.0 row is the premise of a passing test, and merging the two tables would break it to satisfy a check
- [Phase 02]: DefaultTimeout's doc comment corrected, its value untouched: every budget in internal/imagefactory stays with the open 02-DECISION-probe-budget.md
- [Phase 02]: The asset panel's architecture is seeded from the remembered value and never writes back: reading somebody else's saved schematic at another architecture is a question, and what the operator builds is a preference. Letting the answer to a question rewrite a preference is how an operator probes amd64 hardware against an arm64 image without ever choosing it.
- [Phase 02]: The notfound.schematic list invalidation is exact:true, unlike every other invalidateQueries in images.tsx: TanStack matches keys by prefix, so a bare ['schematics'] would also refetch the dialog's own ['schematics', id] query and put the effect's own cause inside its dependency surface (T-02-50).
- [Phase 02]: Only notfound.schematic drops a row from the saved list. A 500 leaves it, because the stored record is the only recoverable copy of an id the Image Factory will not enumerate, and dropping it on a transport failure would delete the operator's only reference at the moment the server is unwell (T-02-51).
- [Phase 02]: model.Schematic.Arch wird unbedingt gestempelt, anders als ProbedAt: ProbedAt haelt fest, ob eine Antwort kam, Arch haelt fest, worueber die Frage ging. Ein Satz, dessen Sonde nie geantwortet hat, hat trotzdem eine Architektur, nach der gefragt wurde. — Die Symmetrie zur ProbedAt-Bedingung drei Zeilen darueber ist verfuehrerisch und falsch; sie wuerde die Zweideutigkeit, um die es in G-02-8 geht, im Kleinen wiederherstellen.
- [Phase 02]: Die Architektur wird neben dem Urteil gerendert, nicht hineingeschrieben: UsabilityBadge umschliesst die unveraenderten drei Saetze aus 02-12 und haengt einen Qualifikator daneben. — Drei Saetze wuerden sonst sechs; ProbeReason nennt die Architektur bereits im Ablehnungstext, der refused-Zweig saegte sie also zweimal; und Option 1 von 02-DECISION-probe-budget.md wuerde diese Saetze erneut umschreiben — ein danebenstehender Qualifikator ueberlebt das unveraendert.
- [Phase 02]: assetRequest liest rec.Arch bewusst nicht: die gespeicherte Architektur ist ein Befund darueber, was gesondet wurde, keine Vorgabe dafuer, was gebaut werden soll. ?arch= bleibt erforderlich und undefaultet (FACT-03), durch einen Test festgenagelt. — holzkube wird auf arm64 entwickelt und zielt auf amd64; eine defaultete Architektur ist ein Fehler, der nur auf fremden Maschinen auftritt. Der Kommentar allein war eine Bitte, der Test ist die Regel.
- [Phase 02]: [Phase 02]: Der Ablehnungssatz des kanonischen Serialisierers stammt aus einer Messung gegen die Live-Factory, nicht aus der UAT-Prosa: vier Klassen aus 116 gemessenen DIVERGES-Zeilen, jede Klausel zitiert ihre Zeilen — Die neun in 02-UAT.md genannten Codepunkte waeren eine plausible, teilweise falsche Abschrift gewesen: drei davon (U+00A0, U+200B, U+202E) laufen nachweislich unveraendert durch und bleiben akzeptiert. Ein angenommener und ein gemessener Satz sind im Code nicht mehr unterscheidbar.
- [Phase 02]: [Phase 02]: U+2028 und U+2029 werden abgelehnt, obwohl eine von sechs gemessenen Varianten uebereinstimmt — Darstellbarkeit ist eine Eigenschaft des Skalars, nicht des Textes drumherum. Positionsabhaengige Akzeptanz hiesse, derselbe Codepunkt waere je nach Nachbarschaft tragbar oder nicht.
- [Phase 02]: [Phase 02]: Die Obergrenze ist U+FFFD, nicht 'Nicht-Zeichen' — U+FDD0 ist ein Nicht-Zeichen und laeuft unveraendert durch; U+FFFD ist das letzte Zeichen, das die Factory woertlich schreibt. Beide gemessen, die Grenze ist damit exakt statt plausibel.
- [Phase 02]: [Phase 02]: Ein 4xx ist eine Antwort, eine Drosselung nicht -- die Differenzialpruefung liest den Status aus ErrUpstreamUnavailable heraus — Der Client faltet jedes Nicht-2xx in ErrUpstreamUnavailable. Die woertliche Plan-Anweisung haette jedes von der Factory abgelehnte Dokument als Nicht-Beobachtung abgelegt, das Ergebnis geloescht und das Halt-Gate auf einer Unwahrheit ausgeloest.
- [Phase 02]: [Phase 02]: U+1F600 wurde gegen die Erwartung des Plans von der Negativkontrolle zur Divergenz umklassifiziert — Der Plan fuehrte den Emoji als Codepunkt, der unveraendert durchlaufen muss. Die Factory escapt ihn zu \U0001F600 wie U+10FFFF. Die Messung schlaegt die Erwartung.
- [Phase 02]: [Phase 02]: plainAllowed bleibt unveraendert, und der Kommentar haelt fest, warum — Keine gemessene Zeile zeigt die Factory ein Skalar quotieren, das dieser Code plain laesst und das weiterhin akzeptiert wird. Eine Aenderung auf duennerer Grundlage haette die id eines bereits korrekten Skalars verschoben.
- [Phase 02]: A caller class that must beat a shadcn component default carries the default's variant prefix (sm:max-w-3xl), or tailwind-merge keeps both and the default wins
- [Phase 02]: UsabilityVerdict tests for the absence of a verdict before reading usable — a reading of a verdict cannot be correct before its subject exists
- [Phase 02]: Option C on 02-16 task 2: the legacy SecureBoot candidate stays a fallback, labelled by its own warning code installer.secureboot-repo-fallback-unverified, because the two SecureBoot names are different images at v1.13.9 but the same image at v1.12.0 — no single comment can settle it, only a per-resolution label
- [Phase 02]: A live drift guard treats any warning on a resolution as a non-observation rather than a pass, and asserts the resolved repository by exact path segment instead of substring containment
- [Phase 02]: Digests belong in a dated observation a test re-measures, never in a source comment; installer.go's two stale literals were deleted rather than refreshed
- [Phase 02]: The problem-type taxonomy is re-rooted at urn:holzkube-manager:problem: and stays closed, stable and non-configurable; a per-deployment base remains rejected (G-02-23)
- [Phase 02]: Contract value families derive from one constant (thirteen types = base + suffix); closure is asserted by parsing the declaring source with go/ast, not by a table a new constant could bypass
- [Phase 02]: Option A on the assets partial-answer gate: 200 with installer null and an installer_error member carrying the problem code and detail; the null makes the unresolved case a decode-time fact
- [Phase 02]: installer_error is absent rather than null on a resolved answer, so proven and provisional assets responses stay byte-identical
- [Phase 02]: The asset panel derives only the remedy from the problem code; the sentence shown is always the server detail, so one condition is never described as two problems
- [Phase 02]: The warning-code drift guard enumerates exported Warning* constants instead of naming them, so a Go code with no TS mirror cannot ship green
- [Phase 02]: An unpaired surrogate escape and raw invalid UTF-8 in a request body are refused with a 400 on the raw bytes before encoding/json can rewrite either to U+FFFD; no value is ever silently repaired (T-02-67). — Accepting the decoder's repair would store a schematic under a name its author would not recognise, with an id computed over a character they never sent. Checking after the decoder is impossible: the evidence is gone.
- [Phase 02]: The browser refusal set is declared as data and held equal to the server's by a Go drift guard failing in both directions; U+202E is carried rather than refused and isolated at every stored-string render site. — 02-14 measured U+202E round-tripping through the Factory, so a client-side refusal would block a value the API accepts. Rendering it inside a bdi keeps the row honest without making the form the authority instead of the contract.
- [Phase 02]: Schematic identity: Option B (one record, per-architecture verdicts) is the decided direction; Option C (state the constraint) is what plan 02-21 wrote. B's implementation is scoped to the phase that next opens the schematic store.
- [Phase 02]: The architecture-unrecoverability claim is narrowed to the probe's outcome, not the record's age: a refused record carries it verbatim in probe_reason. Narrowed, not inverted -- recovering it means parsing prose and nothing does.
- [Phase 02]: Ledger amendments are filed as superseding entries, not edits: gsd-tools windows has no amend verb and hand-editing WINDOWS.md is prohibited.
- [Phase 02]: Die drei Image-Factory-Budgets liegen als Kontext-Deadlines pro Aufruf am Client, nicht mehr auf http.Client.Timeout. — Ein clientweiter Wert kann drei Budgets nicht ausdruecken -- genau daran war der ISO-Probe an eine gegen JSON-Listen bemessene Zahl gebunden. requireDeadline/ErrNoDeadline halten die Zusage, die das entfernte Feld nur zufaellig hielt.
- [Phase 02]: ProbeTimeout=90s und ManifestTimeout=30s folgen einer genannten Regel: mindestens das Doppelte der langsamsten beobachteten kalten Antwort, aufgerundet auf dreissig Sekunden. — Faktor zwei statt etwas Engerem, weil die gemessene Groesse Arbeit auf fremder Build-Farm ist, fuenfmal beobachtet und nie kontrolliert. Die abgeloeste Konstante lag bei 0,92 des Maximums und wurde in jedem der fuenf Laeufe ueberschritten.
- [Phase 02]: ManifestTimeout ist trotz gleichen Werts kein Alias von DefaultTimeout. — Der gleiche Wert ist das, was die Regel aus einem anderen Input erzeugt hat. Die beiden bewegen sich unabhaengig, und die Kompositionswache liest sie als zwei.
- [Phase 02]: Beide Factory-Routen deklarieren eine geteilte Deadline (CreateRouteBudget=120s, AssetsRouteBudget=65s), writeTimeout steigt auf 130s. — Sobald eine Route eine Deadline deklariert, ist die Deadline der Worst Case und nicht mehr die Summe dessen, was ihre Callees zufaellig tun. writeTimeout ist aus R1 hergeleitet: kleinste Zehnerzahl echt groesser als CreateRouteBudget + budgetSlack.
- [Phase 02]: Clipping ist deklarierte Tabellendaten mit eigener Ratsche in beide Richtungen, keine Doc-Comment-Prosa. — POST /api/v1/schematics deklariert 150s Aufrufbudgets gegen 120s Decke. Als Datum kann die Behauptung veralten und rot werden; als Prosa liest sie niemand nach.
- [Phase 02]: AssetsRouteBudget ist fuer den seriellen Kandidatenlauf dieser Welle bemessen; 02-23 zieht es nach. 02-22 allein liefert G-02-2s Verbesserung an der Assets-Route nicht. — Eine Decke von einem Manifest-Budget wuerde den Legacy-Kandidaten abschneiden, bevor er antworten kann -- der stille Fallback, den G-02-3 diese Phase schon einmal gekostet hat.
- [Phase 02]: resolveInstallerRepo asks every installer candidate at once and decides in installerCandidates' declared order: concurrent requests, sequential decision, so the platform-prefixed name keeps its preference and a faster legacy answer cannot displace it. — A first-to-answer race would be a silent installer substitution that nothing downstream can detect (P9(c): an upgrade that reports success while dropping every system extension). The answers land in a slice indexed by candidate position and are read in order; a test gives the legacy candidate zero latency and the preferred one 300ms and goes red against the alternative.
- [Phase 02]: handlers.AssetsRouteBudget tightened 65s -> 35s (one manifest budget plus five seconds) in the same commit that removed the serial candidate walk it was sized for. — The clipping ratchet in cmd/holzkube-managerd/budget_test.go fires on the under-provisioned half-change (two declared calls against the tightened constant computes clipped against a declared uncut) and, by the arithmetic, tolerates the over-provisioned one. Both directions driven locally.
- [Phase 02]: Every browser request carries a fresh AbortSignal ceiling of 150s, attached at the fetch call rather than stored in the reused RequestInit, with its own abort branch that never passes through toProblemError. — init is built once and reused so the sudo replay is byte-identical to the refused request, and an AbortSignal.timeout starts counting when it is created -- a signal stored there would reach the replay partly spent. The ceiling is above writeTimeout (130s) so the server's own problem+json always wins the race; only a server that never answers is cut.
- [Phase 02]: The two waiting screens state the route budget the server enforces, held equal to it by internal/httpapi/handlers/budget_drift_test.go -- the third Go-reads-TypeScript drift guard in this repository. — vitest is rooted at web/ and could not read a Go constant even if allowed out of it, so the guard reads the TypeScript literal instead. Stating a ceiling is explicitly NOT the elapsed-time progress indicator PITFALLS:164 requires; that stays open and the code comment says so.
- [Phase 02]: Der 409-Pfad schreibt das berechnete Sondierungsurteil in genau drei Felder, unter zwei Bedingungen
- [Phase 02]: Ein verlorenes Compare-and-Swap auf dem 409-Pfad wird nicht wiederholt und ein zwischenzeitlich geloeschter Datensatz nicht neu angelegt: beide antworten den blanken Konflikt
- [Phase 02]: G-02-9 bleibt offen und wird als WINDOWS-Eintrag 58 gefuehrt -- die Erholung ist eine Wiedervorlage, die als Fehler beantwortet wird, keine Nachsondier-Route (Option 1 wurde nicht genommen)
- [Phase 02]: Kein Sonderfall fuer eine leere gespeicherte TalosVersion im Refresh-Guard — Arch wurde additiv nachgeruestet, TalosVersion ist seit dem ersten Schematic-Datensatz Pflichtfeld: eine leere gespeicherte Version ist kein Alt-Datensatz, sondern ein Datensatz, dessen Version nicht bekannt ist — die Ungleichheitspruefung lehnt ihn korrekt ab.
- [Phase 02]: Kein versionsuebergreifender Refresh — er ist eine Schema-Aenderung — model.Schematic hat Platz fuer genau ein Verdikt. Ein Datensatz, der die Verdikte zweier Versionen halten soll, braucht dieselbe Aenderung, die Option B fuer die Architektur beschreibt; argumentiert im angehaengten Vermerk von 02-DECISION-schematic-identity.md, nicht in einem Guard.
- [Phase 02]: Die Leer/Abschneide-Unterscheidung des Browser-Ablehnungs-Waechters laeuft ueber strings.TrimSpace(body); die Abschneide-Meldung zitiert den Rumpf und traegt keine Dateizaehlung mehr — Eine Groesse beantwortet eine Frage. whole beantwortete zwei und war fuer eine wirklich leere Deklaration falsch; IN-01 (zwei Fundorte fuer die Eintragszahl) entfaellt als gemessene Nebenwirkung
- [Phase 02]: Der Drift-Waechter verlangt genau eine Deklaration von REFUSED_RANGES und nennt die Zahl, wenn er mehrere findet; die Pruefung greift vor dem Lesen des Rumpfes — Der Anker erlaubt fuehrenden Leerraum, also erfuellt ihn auch eine eingerueckte Zweitdeklaration; die erste schweigend zu nehmen heisst, ueber die andere Uebereinstimmung zu melden, ohne sie verglichen zu haben
- [Phase 02]: Die Wahl zwischen Variante B und A wurde vom Betreiber am 2026-09-05 ausdruecklich an den autonomen Lauf delegiert; der Lauf hat B gewaehlt. Der Betreiber hat B nicht selbst ratifiziert — Ledger-Eintrag 72 und der Nachtrag an 02-DECISION-drift-guard-lexik.md tragen die Formulierung woertlich, und der Statusblock des Dokuments bleibt "offen — vorgelegt, nicht ratifiziert".
- [Phase 02]: Ledger-Eintrag 70 bleibt open und woertlich unveraendert; Eintrag 72 amendiert ihn als neuer Eintrag. Die Divergenz zwischen 70s geschriebener Schliessbedingung (von Runde 6 erfuellt) und seinem tatsaechlichen Offen-Grund wird durch den Text von 72 behoben, nicht durch ein fixed an 70 — ein fixed waere formal dieselbe Bewegung, die 70 an 69 als Fehler benennt.
- [Phase 02]: Der Werkzeugdefekt an requirements revert-phase wird nicht in der GSD-Laufzeit repariert: ~/.claude/gsd-core/ liegt ausserhalb dieses Repositories. Gemessen an einer Kopie, als Eintrag 73 geledgert, und die eine Statuszelle der TRANS-06-Zeile von Hand korrigiert — die Id-Zelle behaelt ihre Dekoration, weil sie zu entfernen die Release-Blocker-Auszeichnung beschaedigte.

### Pending Todos

Keine.

### Blockers/Concerns

- **Phase 3 Risiko:** Tier-1-Docker-Provisioner auf `darwin/arm64` ist ungetestet. Zuerst spiken — scheitert er, kollabiert Tier 1 auf Tier 0 und das Fake-Drift-Risiko jeder späteren Phase steigt.
- **Phase 4 / 8 Entry Requirement:** QEMU (Tier 2) auf darwin/arm64 ist nur im Quelltext bestätigt, nicht ausgeführt. `sudo` + `vmnet-shared` können schmerzhaft sein; Fallback ist eine verschachtelte Linux-VM (Lima/Colima/UTM).
- **Phase 8 Entry Requirement:** Es braucht eine Maschine, die blank sein darf (dritter Mini-PC oder VM). Ohne sie ist der binäre Abnahmetest nicht durchführbar.
- **Phase 10 Entry Requirement:** Ein Verifikationsdurchlauf auf echter amd64-Hardware. Der Dev-Host ist darwin/arm64 und kann amd64-Installer-Pfade strukturell nicht ausüben. Homelab-Zugriff war beim Projektstart nicht gegeben (Timeout auf `192.168.1.41:6443`).
- **16 Release-Blocker (🚫)** sind in ROADMAP.md § *Release Blockers — Besitzverhältnisse* namentlich einer Phase zugeordnet. Keiner darf still verschoben werden.
- FOUND-06/07/08/09 sind abgehakt. Offen bleiben FOUND-01/02/03/04/05/10/11 — sie werden von den Schwesterplänen 01-04..01-06 mitdeklariert und kippen erst, wenn der letzte deklarierende Plan fertig ist.
- FOUND-07 wird derzeit nur durch Review und grep gehalten, nicht durch eine Lint-Regel. golangci-lint-Konfiguration gehört Plan 06; die Regel müsste internal/audit und internal/tlsx ausnehmen.
- Die eingebettete UI ist im Browser unverifiziert: sie baut, wird eingebettet und ausgeliefert, aber nichts prüft, dass die Seite rendert oder die Formulare absenden. Als D2 mit human_judgment: true markiert.
- Das Audit-Archiv wird beim Start nur zwei Dateien tief geprüft. Eine drei Tage alte Korruption ist unsichtbar, bis jemand weiter zurückliest — und niemand tut das. Ein `holzkubed audit verify` über den ganzen Bestand ist der offensichtliche nächste Schritt und ist nicht eingeplant.
- Der neue Genesis-Anker entwertet lokale Datenverzeichnisse aus den Plänen 01-01/01-02: deren erster Satz trägt `prev_hash: ""` und meldet jetzt einen Bruch in Zeile 1. Keine Produktionsdaten, kein Release — lokales Verzeichnis löschen statt migrieren.
- Die eingebettete UI ist weiterhin nur im Browser unverifiziert: Plan 01-05 hat 60 jsdom-Tests plus einen HTTPS-Durchlauf gegen das laufende Binary hinzugefügt, aber keine Browser-Automatisierung. Concern D2 aus Plan 01-01 ist eingegrenzt, nicht geschlossen.
- GET /api/v1/system/status liefert nur setup_required und audit_chain. Version, Bind-Adresse und Datenverzeichnis, die das Dashboard laut Plan 01-05 zeigen sollte, existieren im Vertrag nicht — eine Erweiterung wäre eine bewusste Vertragsänderung, kein Client-Fix.
- **Aufgelöst (2026-08-29, Nutzerentscheidung):** Der `checkpoint:decision` aus Plan 02-02 Task 1 (RFC-9457-`upstream`-Taxonomie) wurde vom Executor unbeaufsichtigt auf Option A gesetzt und ist nun ausdrücklich vom Nutzer bestätigt: ein `upstream`-Typ, HTTP 502, Codes `upstream.node-unreachable` / `node-timeout` / `factory-unavailable` / `factory-rejected`. Coverage-Item D7 (`human_judgment: true`) gilt damit als eingelöst. Die Pläne 02-05 und 02-06 kodieren gegen genau diese Tokens.
- **Aufgelöst (2026-08-29, Nutzerentscheidung):** Der `checkpoint:decision` aus Plan 02-07 Task 1 (Audit-Actor-Vokabular, D-10) wurde vom Executor unbeaufsichtigt auf Option A gesetzt und ist nun ausdrücklich vom Nutzer bestätigt: die Actors `system` und `job:<id>`, dokumentiert aber vorerst von niemandem geschrieben, und `system` als Operator-Benutzername abgelehnt. `Actor` liegt in `canonicalFields`, und D-16 kennt keinen Löschpfad — die Entscheidung ist einwegig, sobald die Jobs-Engine aus Phase 6 den ersten Satz schreibt. Option B (strukturierter `kind`+`id`) hätte den gehashten Typ von `Record.Actor` geändert und eine Migration des bestehenden Archivs erzwungen.
- **Nicht eskaliert (2026-08-29):** Plan 02-07 Task 2 (`dry_run` auf `GET /api/v1/auth/me` statt auf dem preauthentifizierten `system/status`) wurde ebenfalls unbeaufsichtigt entschieden, ist aber umkehrbar und folgt der in STATE.md verzeichneten Phase-1-Entscheidung zu genau diesem Endpunkt. Bleibt bestehen; eine spätere Verlegung ist eine gewöhnliche Vertragsänderung.
- Die schematic.create-Audit-Allowlist (nur name und talos_version, alles andere redigiert) ist in docs/api-contract.md vertraglich festgelegt, aber noch nicht als Code umgesetzt — sie gehoert Plan 02-06. Ein Fehler dort ist permanent: das Archiv hat keinen Loeschpfad (D-16).
- Das Pre-Migration-Tarball enthaelt ab sofort Schematic-Records und damit Kernel-Args und META-Werte im Klartext in einer einzelnen, portablen 0600-Datei. Nichts warnt einen Betreiber, der sie woanders hin kopiert.
- PITFALLS.md P9(d)s v1.9.0-Befund ist ueberholt: metal-installer loest dort heute auf. Der Eintrag sollte markiert werden, bevor eine spaetere Phase darauf handelt.
- golangci-lint ist auf diesem Host nicht installiert; das Akzeptanzkriterium 'golangci-lint run exits 0' aus Plan 02-06 ist unausgefuehrt. go vet und gofmt sind sauber. Eingetragen in .planning/WINDOWS.md als unrun-verify.
- Der Images-Bildschirm (/images) wurde nie in einem Browser geoeffnet — nur jsdom. Coverage-Item D10 mit human_judgment: true. Erweitert die stehende Phase-1-Sorge um die eingebettete UI auf einen zweiten Bildschirm.
- Phase 5 Entry Blocker: Die HTTP-Kette kann nicht streamen. Keiner der drei ResponseWriter-Wrapper implementiert Flush/Unwrap/Hijack (ein Streaming-Handler puffert lautlos), und WriteTimeout = 60 s ist prozessweit mit argon2id-Begruendung. Beides muss vor der ersten SSE-Route landen. Eingetragen in ROADMAP.md Phase 5 und 02-CONTEXT.md <deadline_policy>.
- Eine Maintenance-Mode-Verbindung verifiziert nichts: Creds.Fingerprint wird von NewMaintenanceClient entgegengenommen und nicht benutzt (Trust-on-first-use). T-02-27 weist das Pinning einer spaeteren Phase zu; die Naht ist bereit, das Pinning ist nicht geschrieben.
- talossims ip_changes_on_reboot kappt beim Rebind bestehende Verbindungen, also erreicht die Reboot-Antwort den Aufrufer moeglicherweise nicht — talosctl reboot antwortet auf echter Hardware. Der Simulator ist damit schwerer zu erfuellen als Hardware (erlaubte Richtung), aber ein Phase-6-Job, der das als Fehlschlag liest, laege falsch. WINDOWS.md Eintrag 7.
- Jeder Schematic-Satz, der vor Plan 02-13 gespeichert wurde, traegt dauerhaft eine leere Architektur und zeigt sein Urteil unqualifiziert — genau die Saetze, die waehrend des G-02-8-Lecks entstanden sind. Aus dem Satz laesst sich nichts rekonstruieren; lesbar werden sie nur durch Loeschen und Neuanlegen. WINDOWS.md Eintrag 21.
- Die Authoring-UI (web/src/routes/images.tsx) lehnt jetzt weniger ab als der Server: ihr hasControlCharacter-Guard deckt nur Runen unter U+0020, U+007F und einzelne Surrogate ab, der Kommentar darueber behauptet aber weiterhin, representable vollstaendig abzuschreiben. Ein Emoji, ein BOM oder U+2028 im Kernel-Argument erfaehrt der Betreiber jetzt erst aus dem 400 des Servers. Verhalten sicher, Datei gehoert Plan 02-20. SUMMARY 02-14 Ledger-Eintrag 2.
- ROADMAP.md Zeile 81 traegt zwei Zahlenangaben fuer die ausgefuehrten Plaene der Phase 2: eine vom Werkzeug gefuehrte und eine von Hand geschriebene, nachgestellte. Die nachgestellte driftet nach jeder Welle erneut (Runde 6 hat dieselbe Form als Regression gemeldet). Plan 02-31 hat sie auf 31/31 nachgezogen; ob sie von Hand gefuehrt oder ganz entfernt werden soll, ist offen.

## Deferred Items

Items acknowledged and deferred at milestone close, most recent first:

| Category | Item | Status | Deferred At | Milestone |
|----------|------|--------|-------------|-----------|
| Release blocker | **OPS-05** 🚫 — Verifikationsdurchlauf auf echter amd64-Hardware | Open, window 87 | 2026-09-11 | v1.14 |
| Verification | Installations-Stille messen; `ReappearBudget` in beiden Domänen ersetzen | Open, windows 80, 85 | 2026-09-11 | v1.14 |
| Verification | Sandbox Tier 1 (Docker) und Tier 2 (QEMU) einmal ausführen | Open, windows 76, 81 — Tier 1 kam bis zum Container-Start, `runc` verweigert sysctls in einem verschachtelten Container | 2026-09-11 | v1.14 |
| Tooling | `golangci-lint` gegen die Ziel-Go-Version laufen lassen | Open, window 77 | 2026-09-11 | v1.14 |
| Tooling | Zwei Tests, die diese Umgebung nicht bestehen kann (blackholte Ports, Kernkonkurrenz) | Open, window 89 | 2026-09-12 | v1.14 |
| Accepted loss | Audit-Parameter aus Phase 6 und 7 bleiben inhaltslos | Open, window 83 | 2026-09-11 | v1.14 |
| Deviation | Kubernetes-Upgrade orchestriert nicht über die Kubernetes-API | Open, window 86 | 2026-09-11 | v1.14 |
| Deviation | Supervisors sind Heartbeat-Poller statt COSI-Watches | Open, window 79 | 2026-09-11 | v1.14 |

## Session Continuity

Last session: 2026-09-11T19:15:00.000Z
Stopped at: Milestone v1.14 built; OPS-05 open
Resume file: .planning/phases/10-haertung/10-SUMMARY.md
