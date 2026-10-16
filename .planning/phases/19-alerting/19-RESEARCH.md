# Phase 19: Alerting - Research

**Researched:** 2026-10-16
**Domain:** Go-Daemon (Zustände auswerten, Übergänge erkennen, Webhook ausliefern), Persistenz im Datenverzeichnis, Admin-Route mit Sudo/Audit, React-Einstellungskarte
**Confidence:** HIGH für alles, was im Code gelesen oder gemessen wurde; MEDIUM für das Verhalten fremder Empfänger (ntfy/Gotify/Slack: nur Dokumentation gelesen, nichts davon gegen echte Dienste probiert)
**Umgebung dieser Session:** macOS, arm64 (Mac des Entwicklers), Go 1.27.2 lokal (`go.mod` verlangt `go 1.26.6`, `toolchain go1.26.9`), kein Pi, kein Cluster, kein `task`, kein `golangci-lint`-Binary (der Linter läuft laut CLAUDE.md über `go run ...@v2.13.1`, hier nicht ausgeführt), Node vorhanden. Alles "gemessen" unten heißt: ein Wegwerfprogramm im Scratchpad gegen die Go-Standardbibliothek auf diesem Mac. Es wurde nichts im Repository verändert; `git status` war am Anfang sauber, und es wurde nur diese Datei geschrieben.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

Alle als Auswahl vom Betreiber am 2026-10-16 getroffen, sofern nicht als Ermessen von Claude gekennzeichnet.

- **D-01:** Der Kanal ist ein **Webhook**: eine vom Betreiber konfigurierte URL, an die ein JSON-Dokument geht. Damit lassen sich ntfy, Gotify, Matrix, Slack und eigene Skripte bedienen, ohne dass holzkube-manager einen dieser Dienste kennt. Kein SMTP, keine eingebaute ntfy-Anbindung. — **Reversibility:** costly — die Form des Dokuments ist eine veröffentlichte Schnittstelle für Empfänger.
- **D-02:** Die URL kann ein Geheimnis tragen (Token); sie wird wie ein Geheimnis behandelt (im Datenverzeichnis, nie im Audit-Klartext, nie in der Wand oder in Logs).

### Claude's Discretion
Welche Zustände wie auslösen (nach dem Muster: ein Übergang nach „braucht Aufmerksamkeit", ein Übergang zurück „wieder in Ordnung", keine Wiederholung jeden Takt), Mindestabstand und Zusammenfassen, Form des JSON-Dokuments (Schlüssel, Version), Wiederholung bei Fehlschlag mit Grenze, Zustand über Neustarts (was schon gemeldet wurde, damit ein Neustart nicht alles neu meldet), die Einstellungsseite und ob ein Test-Knopf („Testnachricht senden") dazugehört. Wo eine echte Alternative besteht, geht sie als Auswahl an den Betreiber zurück.

### Deferred Ideas (OUT OF SCOPE)
- ntfy direkt, SMTP-Mail (bewusst nicht gewählt).
- Stille Zeiten, Eskalationsstufen, mehrere Empfänger.
- Alarmregeln pro Knoten.
</user_constraints>

<phase_requirements>
## Phase Requirements

Der Roadmap-Eintrag nennt `Requirements: TBD`. Vorschlag für den Planner (Format `^[A-Z0-9]+-\d+$`, wie `internal/planning` es verlangt):

| ID (Vorschlag) | Beschreibung | Research Support |
|----|-------------|------------------|
| ALRT-01 | Ein Auswerter liest dasselbe Lesemodell wie `/metrics` und die Anzeige und erkennt Übergänge (nach „braucht Aufmerksamkeit", „wieder in Ordnung", „schlimmer"), nicht jeden Takt | Befund 1, 2; Muster 1, 2 |
| ALRT-02 | Gedeckte Zustände: Knoten nicht erreichbar, Knoten-Platte, Knoten-Temperatur, Host (Pi), Snapshot überfällig, Admin-Zertifikat, Zertifikate im Cluster, Konfigurationsdrift; "nicht gelesen" ist nie "wieder in Ordnung" | Befund 1; Pitfall 1 |
| ALRT-03 | Ein Neustart meldet nichts neu; der Erststart legt eine Ausgangslage an, ohne jeden Einzelnen zu melden | Befund 2; Muster 3 |
| ALRT-04 | Webhook-Dokument mit Version, stabilen Schlüsseln, `delivery_id`; ein Dokument je Auswertungsdurchgang | Befund 3; Code Examples |
| ALRT-05 | Auslieferung mit eigener Frist je Versuch, TLS-Prüfung, keine Weiterleitung, Antwortgrenze, begrenzte Wiederholung, sichtbarer Zustand der letzten Auslieferung, kein Geheimnis in Fehlern und Logs | Befund 3, 4; Pitfall 3, 4 |
| ALRT-06 | URL als Einstellung im Datenverzeichnis, schreibend ja, lesend nie; Admin + Sudo; Audit ohne URL; Testnachricht an die gespeicherte URL | Befund 3; Security Domain |
| ALRT-07 | Einstellungskarte: gesetzt/nicht gesetzt, letzte Auslieferung, Liste der letzten Ereignisse | Befund 5 |
| ALRT-08 | README, Leitfaden, API-Vertrag, Metriken; die Sätze "does not alert" werden berichtigt | Befund 5; Pitfall 11 |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Öffentliches Repository:** keine LAN-Adresse, kein Hostname, keine Mail, MAC oder Home-Verzeichnis des echten Aufbaus, nirgends (Tests, Fixtures, Ledger, Commit-Messages). Dokumentationswerte nutzen (`192.168.1.10`, `homeserver`, `example.com`). Das gilt **ausdrücklich auch für das Platzhalterfeld der URL-Eingabe** (ein echter Knoten-Platzhalter stand bis 2026-10-03 in jedem Release). Beispiel-URL überall: `https://ntfy.example.com/holzkube`. `internal/publicrepo` prüft Baum und Historie.
- **README im selben Schritt wie das Feature:** Zeile in "What it does"; Tiefe in `docs/guide.md`; Screenshot nur gerendert aus `web/fixtures/demo.json` (`task build && node web/scripts/readme-images.mjs`).
- **Changelog nur beim Release** (`internal/changelog/changelog.json`); die Phase fasst ihn nicht an.
- **Alpha-Hinweis** bleibt; `holzkube-managerd --version` bleibt eine nackte Version.
- **Produktionsziel linux/arm64 (Pi 5).** Hier weder `go test -race` (nur CI) noch Pi; ein Bericht sagt, wo er lief.
- **Ersetzen der Produktions-Binary, Neustart des Dienstes, Kopieren des Datenverzeichnisses sind Sache des Betreibers.**
- **Entscheidungen immer als Auswahl** (2 bis 4 benannte Optionen mit Kosten/Nutzen, Empfehlung zuerst, über `AskUserQuestion`). Die offenen Auswahlen stehen gesammelt in "Open Questions".
- **Jeder Wächter rot gesehen:** eine Behauptung "der Test deckt das ab" heißt, der Fehler wurde wieder eingesetzt und der Test schlug an. Die Tabelle "Injizierbare Fehler" ist dafür gedacht.
- **Linter lokal:** `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...` (aus CONTEXT.md, Canonical References).
- **Machinery-Pin** wird von dieser Phase nicht berührt.

## Summary

Die Funktion ist im Kern klein, wenn man sie als **reines Abgleichen von Soll und Ist** baut: ein Auswerter macht aus dem vorhandenen Lesemodell eine Liste von "Bedingungen" (Schlüssel, Stufe, Satz, Zustand: feuert / in Ordnung / unbekannt), ein Verfolger vergleicht sie mit dem **persistierten** Stand "schon gemeldet" und erzeugt Ereignisse, ein Sender liefert sie aus. Die Recherche hat **acht Dinge gefunden, die der Plan nicht übersehen darf**:

1. **Nur vier der sechs Zustände stehen in einem Lesemodell, das `/metrics` schon liest.** Knoten-Stufe (`MachineView.Stage`), Snapshot (`ClusterView.Backup`), Admin-Zertifikat, Zertifikate im Cluster (`NodeCertificates`) und Drift (`ConfigDrift`) kommen aus `inv.Machines`/`inv.Clusters`, die der Exporter liest. **Knoten-Temperatur und Knoten-Platte stehen in keinem Lesemodell**: `inventory.Service.Hardware` ist eine Live-Lesung, und die einzige Uhr, die sie regelmäßig ruft, ist der History-Sampler (alle 15 s). Der Exporter exportiert beides nicht. Der Auswerter braucht dafür einen Abgriff am `Hardware`-Dep des Samplers in `main.go`, keinen zweiten Leser.
2. **Die Platten-Schwelle eines Knotens existiert nur im Browser** (`NodeHardware.tsx`: 80 % amber, 90 % rot). Für Temperatur gibt es `inventory.TemperatureLimits` serverseitig, für den Host `filesystemWarnPercent = 80`. Für Knoten-Platten muss die Phase die Regel serverseitig festschreiben (und die Anzeige darf ihr nicht widersprechen), sonst gilt genau der Anti-Pattern "Alarm widerspricht dem Bildschirm".
3. **`url.Error` aus `net/http` und aus `url.Parse` trägt die ganze URL im Text** (gemessen, siehe Pitfall 3). Eine URL mit Token in einem `%w`-Fehler landet im Journal und, über `OutcomeCause`, ohne Weiteres im Audit-Ergebnis. Das Muster im Repo (`k8srelease.go`: `errors.As(err, &ue)` und `ue.Err` nehmen) reicht für `Do` nicht ganz (`dial tcp <ip>:<port>` bleibt drin); der Fehler des Senders muss auf **feste Klassen** abgebildet werden.
4. **Nach einem Neustart ist ein Teil des Lesemodells leer, nicht "in Ordnung".** `NodeCertificates` und `ConfigDrift` sind nil, bis ihre erste Lesung gelaufen ist (certwatch: eine Minute nach Start, `go certWatch.Run(certCtx, time.Minute)`); Knoten stehen in `connecting`/`unknown`, bis der Beobachter zweimal gescheitert ist (`downgradeAfter = 2`). Ein Auswerter, der "nicht da" als "in Ordnung" liest, meldet nach **jedem stündlichen Update-Neustart** "wieder in Ordnung" für alles, und gleich darauf wieder "braucht Aufmerksamkeit". Dreiwertig (feuert / ok / unbekannt) und persistiert löst beides.
5. **`Machine.Disabled` und `Cluster.Disabled` stehen nicht in `MachineView`/`ClusterView`.** Ein vom Betreiber ausgeschalteter Knoten ist `down` und darf nie melden. Der Auswerter braucht deshalb zusätzlich die Records (`st.Machines().List`, `st.Clusters().List`), wie es der Sampler schon über sein `Inventory`-Dep tut.
6. **Das Persistenz-Dilemma ist eine echte Auswahl, kein Detail.** Phase 15 hat Schema 7 gebaut (`const CurrentVersion = 7`) und das als Kosten benannt (ein zurückgerolltes Binary lehnt das Verzeichnis ab, `ErrVersionTooNew`). Für Alerting geht es **ohne** neuen Schema-Sprung: URL und Schalter als additives Feld am `Settings`-Record (Muster `WallLinks`), Zustand und Ereignisliste als **eine** Datei in einem eigenen Verzeichnis über `fsstore.WriteFileAtomic` (Muster `history`). Das ist rückwärtsverträglich; der Preis steht in Open Question 9.
7. **Die Testnachricht darf keine URL aus der Anfrage nehmen.** Sie sendet nur an die gespeicherte URL. Damit ist sie keine SSRF-Sonde, und die Entscheidung "private Ziele sind erlaubt" (ntfy im LAN, ein Skript auf 127.0.0.1) bleibt vertretbar.
8. **ntfy lässt sich mit einem flachen JSON ohne eingebaute Anbindung bedienen** (Template-Parameter in der URL des Betreibers, [CITED: docs.ntfy.sh/publish], Beleg unten). Dafür braucht das Dokument flache Spitzenschlüssel `title`, `message`, `text`; Slack verlangt `text` [CITED: docs.slack.dev].

**Primary recommendation:** neues Paket `internal/alerting` mit **reiner** Funktion `Assess(Input) []Condition` (dreiwertig), einem `Tracker` mit Haltezeit/Hysterese und persistiertem "angekündigt"-Stand (Abgleich Soll gegen Ist statt Warteschlange), einem `Sender` mit festen Fehlerklassen, Lauf auf einem Kontext **ohne** Frist (jede Auslieferung mit eigener Frist), URL am `Settings`-Record, Zustand in `alerting/state.json` über `fsstore.WriteFileAtomic`, drei Routen (`GET`, `PUT` mit Admin+Sudo, `POST …/test`), Karte in den Einstellungen. Die Betreiber-Auswahlen (Zustände, Schwellen, Wiederholung, Erststart, Persistenz) stehen in "Open Questions".

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Zustände zu Bedingungen auswerten | Domain (`internal/alerting`, reine Funktion) | — | Die Regel muss testbar ohne Uhr, Netz und Store sein; sie nutzt die Regeln der Anzeige (Ladder, `TemperatureLimits`, `Health`), erfindet keine zweite |
| Lesemodell liefern | Inventory (`inv.Machines`, `inv.Clusters`) | Sampler-Abgriff (Hardware), `host.Collector.Latest` | Dasselbe Modell wie `/metrics` und die Karten; kein neuer Knotenzugriff |
| Übergänge, Haltezeit, "schon gemeldet" | Domain (`Tracker`) | Store (Datei) | Zustand muss Neustarts überleben (stündlicher Update-Timer) |
| Webhook ausliefern | Domain (`Sender`, `net/http`) | — | Ausgehend, ohne Knotenbezug; eigene Frist, feste Fehlerklassen |
| URL speichern | Store (`Settings`-Record, additiv) | — | D-02: im Datenverzeichnis; Muster `WallLinks` (Begründung am Typ) |
| Zustand und Ereignisliste speichern | Store (`fsstore.WriteFileAtomic`, Datei `alerting/state.json`) | — | Pfade nur über fsstore (`TestNoDirectFileAccessOutsideFsstore`); Muster `history` |
| Einstellen, Testen, Ansehen | API (`handlers/alerting.go`) | Domain | Route-Deklaration trägt Rolle, Sudo, Audit-Action |
| Anzeige | Browser (React, Einstellungskarte) | — | Reine Darstellung; die Sätze kommen vom Server |
| `/metrics` für den Kanal selbst | API (`internal/metrics`) | Domain | Optional: damit ein externes Prometheus den Kanal sehen kann |
| Wand | — | — | Ausdrücklich nicht Teil der Phase; die URL und die Ereignisse dürfen nie in die Wandantwort |

## Standard Stack

### Core

Keine neuen externen Pakete. Alles kommt aus der Go-Standardbibliothek und dem Repo.

| Baustein | Fundort | Zweck |
|---------|---------|-------|
| `net/http`, `net/url`, `encoding/json`, `crypto/rand`, `crypto/sha256` | Standardbibliothek (Go 1.26.x) | Sender, Dokument, `delivery_id`, Digest |
| `internal/store/fsstore` (`WriteFileAtomic`, `ReadFile`) | `internal/store/fsstore/atomic.go:173`, `:363` | Zustandsdatei: eine Datei, atomar, Modi 0700/0600, Temp-Sweep beim Start |
| `model.Settings` (additiv) | `internal/model/model.go:215` | URL, Schalter; Rev-CAS über `settingsStore.Put` |
| `internal/certwatch` / `internal/configdrift` als Gerüst | `Run(ctx, firstAfter)`, `Tick`, `Deps`, `Now` | Schleife, Injektion von Uhr und Quellen |
| `internal/history/persist.go` als Muster für "Datei neben dem Store" | `Open(path, read, write, now, logger)` | Die Funktionen `read`/`write` werden als Deps übergeben, das Paket öffnet keine Dateien selbst |
| `@tanstack/react-query`, shadcn-`Card`/`Badge`/`Button`/`Input`/`Label` | `web/src/...` | Karte wie `WallLinksCard`, `BackupCard` in `web/src/routes/settings.tsx` |

### Alternatives Considered

| Statt | Alternative | Abwägung |
|-------|-------------|----------|
| `Settings`-Feld + eigene Zustandsdatei | Neuer fsstore-Entity `alert-state` plus Schema 7 → 8 | Rev-CAS, `safeKey`, Löschen mit fsync gratis; aber ein zurückgerolltes Binary lehnt das Verzeichnis ab. Open Question 9. |
| Abgleich "Soll gegen Ist" (Zustand je Bedingung) | Warteschlange (Outbox) ausstehender Ereignisse | Outbox wächst bei langem Ausfall, muss gedeckelt und gesichert werden; der Abgleich hat dieses Problem nicht, verliert aber nach dem Aufgeben nichts, wenn die Bedingung weiter feuert (Open Question 4) |
| Eigener Leser für Temperatur/Platte | Abgriff am Sampler-Dep | Ein zweiter Leser wäre ein zweiter Takt und eine zweite Antwort auf "wie warm ist der Knoten" (siehe Kommentar in `history/sampler.go`: "It reuses both reads rather than having its own, and that is a property") |
| Eingebaute ntfy-/Slack-Formate | Ein Dokument, flache Hilfsschlüssel | D-01 schließt eingebaute Anbindungen aus; ntfy-Templates in der URL des Betreibers genügen |

**Installation:** keine. **Version verification:** keine neuen Pakete; `go.mod` (`go 1.26.6`, `toolchain go1.26.9`) gelesen.

## Package Legitimacy Audit

Diese Phase installiert keine externen Pakete (weder Go noch npm). `gsd-tools query package-legitimacy check` wurde nicht aufgerufen, weil es keine Paketnamen gibt.

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
 Lesemodell (nichts davon neu gelesen)                         Takt: alle 30 s (Vorschlag), erste Runde 1 min nach Start
 ─────────────────────────────────────                          │
 inv.Machines ─> MachineView.Stage (down nach 2 Fehlläufen)     ▼
 inv.Clusters ─> ClusterView.{Backup, CertificateUrgency,    alerting.Service.Tick(ctx)   <── Poke (URL gespeichert, Test)
                  NodeCertificates, ConfigDrift}                │
 st.Machines/Clusters.List ─> Disabled (Knoten, Cluster)        │ 1. Input zusammenstellen (nur Speicher, keine Frist nötig)
 Sampler-Abgriff ─> letzte HardwareView je Knoten (Zeit, Temp,  │ 2. Assess(Input) -> []Condition  (rein)
                    Platten) ; Fehlschlag = "unbekannt"         │      Stufe: firing / ok / unknown
 host.Collector.Latest() ─> Snapshot{At, Health}                │      Ursachenunterdrückung (Admin-Zertifikat abgelaufen,
                                                                │      Disabled, laufender Job des Clusters)
                                                                ▼
                                          Tracker.Apply(conditions, now)  gegen persistierten Stand
                                          ┌─ firing, bisher nicht gemeldet, Haltezeit um ──> Ereignis "attention"
                                          ├─ Stufe gestiegen (warning -> critical) ────────> Ereignis "worsened"
                                          ├─ ok, gemeldet, Klärzeit um (Hysterese) ────────> Ereignis "recovered"
                                          ├─ ok, nie gemeldet ────────────────────────────> still weg (kein "recovered")
                                          ├─ unknown ─────────────────────────────────────> nichts ändert sich
                                          └─ Bedingung verschwunden (Knoten vergessen) ───> Eintrag fällt still weg
                                                                │ Ereignisse eines Durchgangs = EIN Dokument
                                                                ▼
                                          Sender.Deliver(ctx mit Frist je Versuch, URL aus Settings)
                                          ┌─ 2xx ─────────────> Ereignisse "zugestellt", Stand "angekündigt" vorrücken
                                          ├─ 408/425/429/5xx/Netz ─> Wiederholung (1 m, 5 m, 30 m), dann "failed"
                                          └─ sonstiges 4xx, 3xx ──> "failed", keine Wiederholung (Korrektur nötig)
                                                                │
                                          alerting/state.json (atomar, nur bei Änderung)
                                          {format, conditions{key->{status,since,announced,level}},
                                           events[ring, 50], last_delivery{at,ok,class,attempts}, first_run_done}
                                                                │
                      ┌─────────────────────────────────────────┼───────────────────────────────┐
                      ▼                                         ▼                               ▼
          GET /api/v1/alerting (Admin)              GET /api/v1/alerting/events (Reader)   /metrics (optional)
          PUT /api/v1/alerting (Admin+Sudo)         POST /api/v1/alerting/test (Admin)
          URL nie zurück: nur "gesetzt" und Ziel (Schema+Host+Port)
```

### Recommended Project Structure

```
internal/alerting/
  conditions.go     # Input, Condition, Kind, Level, Assess (rein), Schwellen, Sätze
  tracker.go        # Tracker: Übergänge, Haltezeit, Hysterese, Zusammenfassen (rein bis auf State)
  state.go          # State (Format 1), Laden/Speichern über übergebene read/write-Funktionen, Prüfung beschädigt
  document.go       # Webhook-Dokument v1 (Typen, Aufbau, Obergrenzen)
  sender.go         # Sender: Client, Fristen, Klassen, Retry-After, Antwortgrenze, keine Weiterleitung
  service.go        # Deps, Run/Tick/Poke/Flush, Testnachricht, View für die API
internal/model/model.go             # Settings.Alerting (additiv, omitempty)
internal/httpapi/handlers/alerting.go  # Routen, Ansicht ohne URL
internal/audit/redact.go            # "alerting.configure", "alerting.test"
internal/metrics/metrics.go         # optional: Kanalgesundheit, Paketkommentar berichtigen
cmd/holzkube-managerd/main.go       # Verdrahtung, Sampler-Abgriff, Lauf auf Kontext ohne Frist
cmd/holzkube-managerd/alerting_wiring_test.go   # wie drift_wiring_test.go (AST-Wächter)
web/src/components/Alerting.tsx     # Karte; eingehängt in web/src/routes/settings.tsx (AdminOnly)
```

### Pattern 1: Dreiwertige reine Auswertung

**What:** `Assess` bekommt einen `Input` (Ansichten, Records, letzte Hardware-Lesungen, Host-Snapshot, `now`) und liefert je Bedingung `State: Firing|OK|Unknown`. Keine Uhr, kein Netz, kein Store. Dieselben Regeln wie die Anzeige: `certwatch.Ladder`/`Urgency`-Felder der Ansichten, `inventory.TemperatureLimits` (über `HardwareTemperature.WarnC/DangerC`), `host.Assess`.
**When to use:** immer; `Unknown` ist ein Pflichtwert, nie ein `false`.
**Example:**
```go
// Source: eigene Skizze nach internal/certwatch/certwatch.go und internal/configdrift/health.go
type State int
const (
	Unknown State = iota // zero value: nobody has looked, never "fine"
	OK
	Firing
)
```

### Pattern 2: Abgleich Soll gegen Ist mit Haltezeit (level-triggered)

**What:** Je Schlüssel persistiert der Tracker `{Firing bool, Level, Since, Announced bool, AnnouncedLevel, PendingSince, ClearSince}`. Ein Durchgang vergleicht Beobachtung und Stand. Was gemeldet werden muss, folgt aus der Differenz (`Firing && !Announced`, `!Firing && Announced`, `Level > AnnouncedLevel`), nicht aus einer Warteschlange. Daraus folgt: ein Neustart meldet nichts neu (der Stand ist da), ein gescheiterter Versand bleibt eine offene Differenz, ein "zugestellt" wird erst nach einem 2xx gesetzt.
**Haltezeiten und Hysterese (Vorschlag, Zahlen sind Auswahl 2 in Open Questions):** Auslösen erst nach `For` ununterbrochen feuernd (Knoten 5 min, Temperatur und Platte länger), Klären erst nach `Clear` ununterbrochen ok (Wert-Hysterese zusätzlich: Temperatur 3 °C unter der Schwelle, Platte 5 Punkte darunter). Die Zertifikatsleiter ist diskret und braucht keine Hysterese.
**Wichtig:** Die Haltezeit läuft **im Speicher und im Stand** (`PendingSince`), und wo die Quelle selbst ein "seit" trägt (Drift: `NodeConfigDrift.Since`, vom Record), zählt dieses, damit ein Neustart die Uhr nicht auf null setzt (Befund 7 aus Phase 15, derselbe Grund).

### Pattern 3: Erststart und beschädigter Stand = Ausgangslage

**What:** Gibt es keinen Stand (`state.json` fehlt) oder ist er beschädigt/unbekanntes Format, gilt "Ausgangslage": der erste Durchgang **nach dem Speichern der URL** schreibt, was gerade feuert, als `Announced`, ohne es einzeln zu senden (Variante A), oder sendet **ein** Sammeldokument (Variante B). Die Wahl ist Open Question 5. In beiden Fällen: eine beschädigte Datei wird **nicht** als leerer Stand gelesen, der alles neu meldet, sondern wie ein Erststart behandelt und im Journal einmal gewarnt (Muster `history.Open`: "logged and set aside").

### Pattern 4: Sender mit festen Fehlerklassen

**What:** Ein `Sender` mit `http.Client{CheckRedirect: …ErrUseLastResponse}`, **ohne** `Client.Timeout`, mit `context.WithTimeout` je Versuch (wie `imagefactory`: "No Timeout. One value cannot express three budgets"). Antwort: höchstens 4 KiB lesen und **verwerfen**, nie speichern, nie loggen. Rückgabe ist eine Klasse aus einer festen Menge (`ok`, `timeout`, `unreachable`, `tls`, `refused-by-endpoint`, `rate-limited`, `server-error`, `redirect`), nie `err.Error()`.

### Anti-Patterns to Avoid

- **Einen Fehler mit `%w` weiterreichen, der eine URL trägt** (`url.Error`, `url.Parse`): gemessen, trägt das Token. Der Sender gibt Klassen zurück.
- **Jeden Takt senden, wenn der Zustand schlecht ist** (kein Stand): die zentrale Behauptung der Phase.
- **"nicht da" als "in Ordnung" lesen** (nil `NodeCertificates`, Knoten `connecting`, Hardware-Lesung gescheitert, Host-Snapshot alt).
- **Einen zweiten Leser für Temperatur/Platte bauen** (Sampler-Abgriff statt dessen).
- **Den Hintergrundlauf auf einem Kontext mit Frist starten** (certwatch tat es und blieb stumm, Quick-Task 261014-gl2; Ledger 137).
- **Die URL in einer Antwort, einem Audit-Eintrag, einem Fehlersatz, einem Log, der Wand, dem Support-Bundle oder dem Platzhalter der Eingabe zeigen.**
- **Wiederholung ohne Grenze oder ohne sichtbaren Zustand.**

## Befunde zu den sechs Fragen

### Befund 1 (Frage 1): Inventar der Aufmerksamkeitszustände und das eine Lesemodell

Das Lesemodell des Exporters ist `Deps.Machines`/`Deps.Clusters`, in `main.go` mit `inv.Machines`/`inv.Clusters` verdrahtet (`Machines: inv.Machines,` `Clusters: inv.Clusters,` in `metrics.New(metrics.Deps{…})`) [VERIFIED: cmd/holzkube-managerd/main.go, Block "The Prometheus export (V2-API-02)"]. Der Kommentar dort nennt den Grund: "a metric that disagreed with the screen would send an operator looking for a fault in the cluster that was actually a fault here". Der Auswerter nimmt **dieselben beiden Funktionen** (nicht eine zweite Zusammenstellung). Zustand für Zustand:

| Zustand | Quelle (gelesen) | Berechnet in | Im Exporter? | Unbekannt wenn |
|---------|------------------|--------------|--------------|----------------|
| Knoten nicht erreichbar | `MachineView.Stage == health.StageDown` | `internal/inventory/observe.go` (Beobachter), `downgradeAfter` | ja (`holzkube_machines{stage}`) | Stufe `unknown`/`connecting`; Knoten oder Cluster `Disabled` ist "nie melden", nicht "unbekannt" |
| Knoten-Temperatur | `HardwareView.Temperatures[].Celsius` gegen `WarnC`/`DangerC` | `inventory.TemperatureLimits` (limits.go) | **nein** | Lesung des Sampler-Durchgangs fehlgeschlagen oder älter als 3 Durchgänge |
| Knoten-Platte | `HardwareView.Filesystems[]` (`UsedBytes`/`SizeBytes`) | Schwelle **nur im Browser** (`NodeHardware.tsx`) | **nein** | wie Temperatur |
| Host (Pi) | `host.Collector.Latest()` → `Snapshot{At, Name, Health}` | `host.Assess(Live)` (rein) | nein | kein Snapshot oder `now - At > 3*history.FineStep` (so macht es die Wand, `wallHostFrom`) |
| Snapshot überfällig | `ClusterView.Backup.Overdue` (nil ohne Snapshot-Store) | `upgrade.healthOf` über `BackupHealthFor` (nur Dateien, keine Knoten) | ja (`holzkube_etcd_snapshot_overdue`) | `Backup == nil` |
| Admin-Zertifikat | `ClusterView.CertificateUrgency` und `ClientCertDaysLeft` | inventory (D-23) | ja (`holzkube_cluster_client_certificate_seconds`) | nie unbekannt (rein aus dem Record) |
| Zertifikate im Cluster | `ClusterView.NodeCertificates` (`Urgency`, `Items[]` mit `Machine`,`Name`,`Kind`,`DaysLeft`) | `certwatch.HealthFor` | ja (`holzkube_node_certificate_seconds`) | `NodeCertificates == nil` (vor der ersten Lesung); Knoten, die nicht antworten, behalten ihre alten Einträge |
| Konfigurationsdrift | `ClusterView.ConfigDrift.Items[]` mit `State`, `Since`; `Banner` | `configdrift.HealthFor`, `Drifting()`, `Judged()` | ja (`holzkube_node_config_drift`) | `ConfigDrift == nil`; Zustände `not-read`, `unreadable`, `baseline-damaged` sind **nicht** "feuert" und **nicht** "ok" |

**Belege (verbatim, Repo gelesen):**
- `const downgradeAfter = 2` [VERIFIED: internal/inventory/inventory.go:240]; `const DefaultHeartbeat = 45 * time.Second` [VERIFIED: internal/inventory/inventory.go:83]. Ein Knoten wird also nach etwa 90 s ohne Antwort `down`; ein Neustart des Knotens (Update) dauert länger. Folge: Haltezeit nötig (Pitfall 5).
- `StageDown` "is not reachable. Facts keep their last value and gain a stale_since" [VERIFIED: internal/health/stage.go:38-40].
- Admin-Leiter: `UrgencyNone = "none"`, `UrgencyBadge = "badge"`, `UrgencyBanner = "banner"`, `UrgencyCritical = "critical"` [VERIFIED: internal/inventory/view.go:194-205]; Kommentare dort: 90/30/7 Tage.
- Zertifikate im Cluster: Authority `left <= 7*day` → Critical, `<= 30*day` → Banner, `<= 90*day` → Badge; ausgestellt `left <= 3*day` → Critical, `<= 14*day` → Banner [VERIFIED: internal/certwatch/certwatch.go:46-58]. Ein ausgestelltes Zertifikat nahe am Ablauf heißt "eine Rotation hängt" (Satz dort).
- Drift: `const BannerAfter = 24 * time.Hour` [VERIFIED: internal/configdrift/health.go:12]; Zustände `ConfigStateDrift = "drift"` und weitere [VERIFIED: internal/model/configdrifthealth.go:14]; `Drifting()` ist `!n.Since.IsZero()` und überlebt einen Lesefehler und einen Neustart (Record).
- Host: `const filesystemWarnPercent = 80` [VERIFIED: internal/host/health.go:63]; `HealthState` `ok`/`warn`/`unknown`; `Health{State, Summary, Warnings, Unreadable, Public}`. **`Health` trägt keine stabilen Schlüssel je Befund** (`finding{sentence, public, critical, excess}` ist unexportiert) [VERIFIED: internal/host/health.go, Typ `finding`]. Empfehlung: ein exportiertes, nicht serialisiertes Feld `Findings []Finding{Key string, Critical bool}` (`json:"-"`) ergänzen, damit der Host je Sensor/Dateisystem eine Bedingung hat; ohne das gibt es genau **eine** Host-Bedingung, und ein zweiter Befund während der erste feuert wird nicht gemeldet.
- Knoten-Platte im Browser: `warn={f.size_bytes * 0.8}` und `danger={f.size_bytes * 0.9}` [VERIFIED: web/src/components/NodeHardware.tsx:276-277]. Die Liste der Dateisysteme filtert Loop-Geräte, Bind-Mounts und Größe 0 (`filesystemsView`, hardware.go: "The root filesystem is a squashfs image on a loop device and is not a disk anybody can fill"), also kein Dauer-Alarm durch das Root-Image.
- Hardware wird zeitgesteuert nur vom Sampler gelesen: `sampleMachine` ruft `s.deps.Hardware(ctx, id)` alle `FineStep` (`FineStep = 15 * time.Second` [VERIFIED: internal/history/history.go:53]); `Hardware: inv.Hardware,` [VERIFIED: cmd/holzkube-managerd/main.go:808]. **Abgriff:** in `main.go` das Dep durch eine Funktion ersetzen, die `inv.Hardware` ruft, das Ergebnis (oder den Fehler) an den Auswerter gibt und unverändert zurückreicht; `history` bleibt unberührt. Der Sampler liest nur Maschinen in einem Cluster.

**Ausgeschaltete Knoten (Befund 5 der Zusammenfassung):** `Disabled bool \`json:"disabled,omitempty"\`` steht an `Cluster` (internal/model/inventory.go:89) und `Machine` (:245) [VERIFIED: grep]; `grep Disabled internal/inventory/*.go` (ohne Tests) findet nichts, also tragen die Ansichten es nicht. Der Auswerter braucht `Inventory(ctx) ([]model.Machine, []model.Cluster, error)` wie das `SamplerDeps.Inventory`.

**Nicht Teil dieser Phase (benennen, nicht bauen):** die Kubernetes-Wand (`kube.wall`, Zustand `warn` bei zu wenigen Replikas, cordoned), `LostAddr`, `UnsupportedVersion`, fehlgeschlagene Jobs. Sie erscheinen nicht in der Zielliste der Phase.

**Zusätzliche Quellenregel (Ursachenunterdrückung):**
- Ist das Admin-Zertifikat eines Clusters abgelaufen (`MachineView.CertificateExpired`, "distinguishes a cluster whose credentials ran out from a node that died" [VERIFIED: internal/inventory/view.go:42-45]), sind alle seine Knoten `down`: **eine** Meldung (Admin-Zertifikat), keine N "Knoten nicht erreichbar".
- Ein Knoten, dessen Cluster einen Job im Zustand `pending` oder `running` hat (`model.Job{Cluster, Machine, State}`, `JobPending`/`JobRunning` [VERIFIED: internal/model/job.go:25,28]), startet vermutlich gerade neu (Upgrade, Reset, Reboot-Aktion). Empfehlung: "Knoten nicht erreichbar" für diese Knoten unterdrücken, solange der Job läuft (Quelle: `engine.List`, wie der Exporter). `JobParked` zählt nicht: ein geparkter Job wartet auf den Betreiber. Das ist Zusatzumfang; Open Question 2 führt es als Option.

### Befund 2 (Frage 2): Übergänge, Neustart, Persistenz, Erststart

- **Was wo steht:** Die Zertifikats- und Drift-Lesungen sind im Speicher (certwatch: "A restart starts with nothing"); die Drift-Zeit `Since` ist im Basis-Record persistiert; Knotenstufen sind im Speicher; `Backup` wird bei jedem Aufruf aus Dateien berechnet. Der Auswerter hat also **keine** Bedingungs-Historie zur Hand und muss seinen Stand selbst halten.
- **Neustart:** der Update-Timer startet den Dienst stündlich neu, wenn ein Release da ist (CLAUDE.md: `holzkube-manager-update.timer` pulling releases hourly). Der Stand "angekündigt" muss deshalb auf Platte. Danach gilt: gleiche Beobachtung wie im Stand → kein Ereignis; `Unknown` → Stand unverändert. Genau das trennt "Neustart" von "wieder in Ordnung".
- **Übergangsregeln:** siehe Pattern 2 und das Diagramm. Zusätzlich: Stufenwechsel eines feuernden Zustands nach oben (Leiter `banner` → `critical`) ist ein eigenes Ereignis `worsened`; nach unten (critical → banner) ändert nur die gespeicherte Stufe still, damit ein flackernder Wert kein Ereignis je Takt erzeugt. Ein `recovered` für etwas, das nie zugestellt wurde, wird **nicht** gesendet (der Empfänger kennt es nicht), steht aber in der Ereignisliste.
- **Zusammenfassen und Mindestabstand:** Alle Ereignisse eines Durchgangs gehen in **ein** Dokument. Zwischen zwei Dokumenten mindestens 30 s (Vorschlag), je Dokument höchstens 20 Ereignisse (`more: n` für den Rest, damit eine Flut aus 40 Knoten kein 40-Ereignis-Dokument wird), und die Ereignisliste im Stand ist ein Ring von 50. Das begrenzt Flut und Dateigröße.
- **Wo persistieren:** eine Datei `alerting/state.json` über `fsstore.WriteFileAtomic` (schreibt `CreateTemp(dir, tempPrefix+"*")`, chmod, fsync, rename, fsync(dir); `Open` räumt Temp-Dateien beim Start und `Guard` verlangt 0700/0600) [VERIFIED: internal/store/fsstore/atomic.go:173 und permissions.go:Guard]. Geschrieben wird **nur bei Änderung** des Stands (Übergang, Zustellung, Versuch), nicht je Takt; das ist ein paar Mal pro Woche. Das Verzeichnis `alerting/` legt der Dienst selbst an (wie `history.DirName = "history"`, [VERIFIED: internal/history/persist.go, `DirName = "history"`]), mit 0700.
- **Schema-Frage:** `const CurrentVersion = 7` [VERIFIED: internal/store/migrate/migrate.go:26]. Jede Entity-Verzeichnis-Einführung hat bisher eine Migration (1→2 bis 6→7). **Keine** Migration ist nötig, wenn Alerting dem `history`-Muster folgt: `history/` ist kein Entity, hat keine Migration, und `Guard` verlangt nur die Modi. Ein **älteres** Binary öffnet ein Verzeichnis mit `alerting/` (Modi stimmen, keine Versionsänderung) und ignoriert es. Zwei Wermutstropfen, beide für Open Question 9 zu benennen: (a) ein älteres Binary, das den `Settings`-Record neu schreibt (Wall-Link anlegen), verliert das unbekannte Feld `alerting` (der Decoder ignoriert unbekannte Felder: `readJSON` ruft `json.Unmarshal` ohne `DisallowUnknownFields` [VERIFIED: internal/store/fsstore/fsstore.go:273-281]); die URL ist dann nach einem Rollback weg, der Betreiber muss sie neu eintragen. (b) Der Stand liegt außerhalb der Rev-Prüfung des Stores; es gibt nur einen Schreiber (den Dienst), das genügt, braucht aber einen Mutex im Dienst. Option B (Entity + Schema 8) vermeidet (b), nicht aber das Rollback-Problem (dort wäre es `ErrVersionTooNew`).
- **Datensicherung und Support-Bundle:** `settings.json` liegt im Datenverzeichnis und damit in der Vorher-Sicherung (`backups/pre-migration-…`) und in jeder Kopie, die der Betreiber macht; das ist bei Cluster-Geheimnissen schon so (CLAUDE.md: "it holds cluster secrets"). `internal/support` liest `Settings()` nicht (`grep "Settings()"` findet nur `auth/walllink.go` und `handlers/setup.go`) [VERIFIED: grep], die URL kommt also nicht ins Bundle; ein Wächter-Test soll das festhalten.
- **Erststart:** drei Möglichkeiten (A still festschreiben, B ein Sammeldokument, C alles einzeln), Open Question 5. Gleiches gilt für einen beschädigten oder unbekannten Stand. Wichtig für den Plan: "Erststart" ist **nicht** "erster Daemon-Start", sondern "Stand fehlt oder URL wurde gerade erstmals gesetzt". Wird die URL später **geändert**, bleibt der Stand (der neue Empfänger bekommt keine Altlast), wird sie **entfernt**, ruht der Auswerter (Bedingungen werden weiter verfolgt, nichts gesendet), damit ein späteres Wiedereinschalten keine Flut auslöst; Wiedereinschalten ist wie Erststart.
- **Verschwundene Bedingungen:** ein Knoten oder Cluster, der aus dem Inventar verschwindet, ist nicht "wieder in Ordnung". Der Eintrag fällt im nächsten Durchgang still weg (Abgleich im `Tick`, Muster `configdrift.reconcile`, `history.Retain`).

### Befund 3 (Frage 3): Webhook, Client, SSRF, Ort der URL, Audit, Test

**Dokument (veröffentlichte Schnittstelle, Form ist "costly", also dem Betreiber zur Bestätigung):** flach, versioniert, mit Hilfsschlüsseln für Fremddienste. Vorschlag `version: 1`:
```json
{
  "version": 1,
  "kind": "attention",
  "delivery_id": "d_3f9c0a6e51b24f08",
  "created_at": "2026-10-16T08:05:00Z",
  "source": "holzkube-manager",
  "title": "2 things need attention",
  "message": "cp-1 (homelab) is not reachable since 08:00.\nThe etcd snapshots of homelab are overdue.",
  "text": "2 things need attention\ncp-1 (homelab) is not reachable since 08:00.\nThe etcd snapshots of homelab are overdue.",
  "events": [
    {
      "id": "e_91b2…", "condition": "node-unreachable", "key": "node-unreachable/<machine-id>",
      "transition": "attention", "severity": "critical",
      "cluster": {"id": "…", "name": "homelab"}, "node": {"id": "…", "name": "cp-1"},
      "since": "2026-10-16T08:00:00Z", "sentence": "cp-1 (homelab) is not reachable since 08:00."
    }
  ],
  "more": 0
}
```
- `kind`: `attention` (mindestens ein Ereignis, auch reine `recovered`-Dokumente) oder `test`. `transition` je Ereignis: `attention`, `worsened`, `recovered`. `severity`: `warning` | `critical`. Die Wörter der `condition`-Werte (`node-unreachable`, `node-temperature`, `node-disk`, `host`, `backup-overdue`, `client-certificate`, `cluster-certificate`, `config-drift`) sind **Entwurf, nicht belegt**; der Planner fixiert sie als Vokabular (wie `health.Stages()`: eine Liste, die ein Test durchläuft).
- `delivery_id` bleibt über alle Wiederholungen gleich und das Dokument bleibt byteweise gleich (`created_at`, nicht `sent_at`): at-least-once, der Empfänger kann entdoppeln.
- **Was nie hineinkommt:** keine URL, kein Fehlertext eines Knotens, keine Zertifikats-`Subject`, keine Adressen, keine Digests, keine Pfade (die Sätze sind die der Karten für Reader: `listSentence` in `configdrift/health.go` zeigt, wie die Drift-Sätze schon absichtlich keine Fehlertexte tragen).
- **Hilfsschlüssel und Fremddienste (belegt):** Slack verlangt `text`: "`no_text` means the `text` attribute was missing from the payload" und "Your webhook URL contains a secret" [CITED: docs.slack.dev/messaging/sending-messages-using-incoming-webhooks]. ntfy: auf `…/topic` ist der Body der Nachrichtentext; JSON-Veröffentlichung braucht `topic` im Body an der Wurzel-URL, **aber** mit Templates in der URL des Betreibers genügt ein beliebiges flaches JSON: `ntfy.sh/mytopic?tpl=yes&t={{.hostname}}:+…&m=…{{.error.desc}}`; Nachrichten bis 4096 Bytes [CITED: docs.ntfy.sh/publish]. Gotify: Endpunkt `/message`, Token als Header `X-Gotify-Key`, Pflichtfeld `message`, optional `title`, `priority`; die Seite zeigt Formularfelder, ob JSON angenommen wird, steht dort nicht [CITED: gotify.net/docs/pushmsg]; JSON-Annahme ist [ASSUMED], A3. Folge für das Dokument: Spitzenschlüssel `title`, `message`, `text` (dreimal derselbe Inhalt in drei Formen) bedienen Slack, Mattermost-artige Dienste, ntfy-Templates und (vermutlich) Gotify. Ein ntfy-Betreiber trägt `?tpl=yes&t={{.title}}&m={{.message}}` in seine URL; **das muss die URL-Prüfung zulassen** (gemessen: `url.Parse` akzeptiert `{{.title}}` im Query, und `http.NewRequest` sendet es unverändert: `/t?tpl=yes&t={{.title}}&token=SECRETTOKEN` stand so auf der Leitung).
- **Obergrenzen:** `message`/`text` höchstens 4000 Zeichen (unter ntfys 4096 Bytes ist knapper nötig; Vorschlag 3500 Zeichen UTF-8, am Zeilenende abgeschnitten, mit "…and N more").

**HTTP-Client (Entscheidungen, jede mit Grund):**
- TLS-Prüfung nie abschaltbar, keine Option (wie `imagefactory.New`: "TLS verification is never disabled and there is no option to disable it"). Folge: ein LAN-Dienst mit selbstsigniertem Zertifikat geht nur über `http://` oder eine vertraute CA. Ein Anheften (Fingerabdruck wie bei `cluster.fingerprint`) ist die spätere Erweiterung, nicht jetzt.
- Klartext-`http://` erlauben (ntfy/Gotify im LAN ist der Normalfall), im UI bei nicht-Loopback-Zielen den Hinweis "the token travels unencrypted" zeigen (Open Question 6).
- **Keine Weiterleitungen folgen:** `CheckRedirect` gibt `http.ErrUseLastResponse` zurück; 3xx zählt als Fehlschlag der Klasse `redirect` ("use the final URL"). Gemessen: ein `307` mit `Location: http://elsewhere.invalid/x?token=SECRET2` kommt als Antwort zurück, ohne gefolgt zu werden. Das Repo-Muster (`refuseCrossHostRedirect`/`refuseForeignRedirect`, `maxRedirects = 3` in k8srelease.go:71) erlaubt Weiterleitungen im selben Host; für einen POST mit Token in der URL gibt es keinen Grund dazu (307/308 würden den Body erneut senden).
- **Frist:** kein `Client.Timeout`, sondern `context.WithTimeout` je Versuch, 10 s (Vorschlag, ntfy im LAN antwortet in Millisekunden, ein Slack-Hook in unter einer Sekunde); Proxy: Standard (`ProxyFromEnvironment`), nichts Eigenes.
- **Antwort:** höchstens 4 KiB lesen und verwerfen (Muster `k8srelease`: `io.LimitReader(resp.Body, maxBodyBytes+1)`, dort `maxBodyBytes = 64` [VERIFIED: internal/k8srelease/k8srelease.go:67]); nie speichern, nie loggen, nie in die Ansicht (Server echoen die URL oder ein Token in Fehlerseiten). Gespeichert wird nur die Klasse und der Statuscode.
- **Statusabbildung:** 2xx → zugestellt; 408, 425, 429, 5xx, Netzfehler, Timeout → wiederholen (`Retry-After` bei 429/503 beachten, auf höchstens 5 min gedeckelt); übrige 4xx → `refused-by-endpoint`, **nicht** wiederholen (Slack: `invalid_payload` "should not be retried without correction" [CITED: docs.slack.dev]); 3xx → `redirect`, nicht wiederholen. 429-Verhalten bei Slack ist in der gelesenen Quelle nicht beschrieben [CITED, Lücke]: `Retry-After` allgemein zu beachten ist [ASSUMED] richtig (A4).
- **Wiederholung:** Versuche nach 0, 1 min, 5 min, 30 min (vier Versuche, dann `failed`), Zahl und Abstände sind Open Question 4. Der Zustand der letzten Auslieferung (`at`, `ok`, `class`, `status`, `attempts`, `next_attempt_at`) steht im Stand und in der Karte.

**SSRF-Entscheidung (mit Begründung):**
- Die URL setzt nur ein **Admin mit Sudo-Fenster** (`PUT`, `Destructive: true`). Die Testnachricht nimmt **keine** URL aus der Anfrage (nur die gespeicherte). Der Daemon liest die Antwort nicht in die Ansicht (nur Klasse und Status). Damit ist der Missbrauch als Sonde in das LAN (Talos-apid `:50000`, die eigene API, Router) auf "ein Admin mit Sudo darf einen festen JSON-POST an eine selbstgewählte Adresse auslösen und erfährt, ob etwas antwortete" begrenzt, was ein Admin über `talosctl`-nahe Funktionen (Cluster importieren, Maschinen scannen: `POST /api/v1/provision/scan`) ohnehin kann.
- **Private Ziele sind erlaubt**, einschließlich Loopback: ntfy/Gotify im LAN und ein Skript auf `127.0.0.1` (der Pi selbst) sind der Normalfall dieses Produkts; eine Sperre privater Netze würde genau die gewählte Anwendung (D-01, "ntfy, Gotify … eigene Skripte") verbieten. Gesperrt wird nur, was nie ein Webhook-Ziel ist: Schema außer `http`/`https`, leerer Host, `user:pass@` im Host ist **erlaubt** (Basic-Auth ist eine übliche Art, das Geheimnis zu tragen; die ganze URL ist Geheimnis), Länge über 2048 Zeichen. Link-Local (`169.254.0.0/16`) zu sperren kostet nichts und hat in einem Heimnetz ohne Cloud-Metadaten keinen Nutzen: Auswahl in Open Question 6, Empfehlung **nicht** sperren.
- Die Prüfung beim Speichern liest **nur** die Syntax. Es gibt keine Namensauflösung und keinen Verbindungsversuch beim Speichern (ein PUT, der ins Netz greift, wäre die Sonde). Ein Fehler der Prüfung nennt den Grund ("must be an http or https address"), nie die Eingabe.

**Ort der Einstellung:** `internal/config` kennt nur Flags und Umgebungsvariablen und sagt es: "Nothing to parse means nothing to migrate … flags and HOLZKUBE_MANAGER_* environment variables only" [VERIFIED: internal/config/config.go, Kopfkommentar]; D-02 verlangt "im Datenverzeichnis" und der Betreiber soll es in der Oberfläche ändern. Es bleibt also ein gespeicherter Wert. Vergleich der vorhandenen Muster:
| Muster | Wo | Für Alerting |
|--------|----|--------------|
| Wall-Links | additives Feld am `Settings`-Record, "Kept on the settings record rather than in a store of their own … an interface, an implementation and a migration for a list that fits on one line" [VERIFIED: internal/model/model.go, Kommentar an `WallLinks`] | **ja** für URL und Schalter: eine URL, eine Handvoll Schalter, zweimal im Jahr geändert; der Preis (Rev-Konkurrenz mit anderen Settings-Schreibern) ist bei dieser Änderungshäufigkeit richtig |
| Backup-Zeitplan | additives Feld am `Cluster` (`BackupSchedule`, "Additive on Cluster and unversioned") | nein: je Cluster; die URL ist instanzweit |
| Flags | `internal/config` | nein (D-02) |
Die Ansicht der Einstellung (`GET`) trägt `configured`, `enabled`, das Ziel `scheme://host[:port]` (Pfad, Query, Userinfo **nie**), die Schalter je Zustand (falls gewählt) und die Zeit der letzten Änderung. Das Ziel ist für einen Admin nützlich ("welcher Dienst ist eingetragen") und ist nicht das Geheimnis, wenn Token in Pfad/Query/Userinfo stehen; ob auch das Ziel verborgen werden soll, ist Auswahl in Open Question 6.

**Route, Sudo, Audit:**
- `PUT /api/v1/alerting` (URL setzen/ändern/leeren, Schalter): `MinRole: model.RoleAdmin` [VERIFIED: internal/model/model.go:154: `RoleAdmin UserRole = "admin"`], `Destructive: true` (Begründung wie `cluster.backup-schedule` und `wall-link.create`: "the other settings that decide what this product does on its own"; und eine falsch gesetzte URL schickt Cluster-Namen an Dritte), `Action: "alerting.configure"`. Das 428/Sudo-Fenster ist im Frontend global (`api.ts`), nur ein Eintrag in `ACTION_LABELS` ist nötig.
- `POST /api/v1/alerting/test`: `MinRole: model.RoleAdmin`, nicht `Destructive` (sendet nur an die gespeicherte URL, ändert nichts; Vorbild `etcd.backup-run`, nicht destruktiv), `Action: "alerting.test"`. Gedrosselt: höchstens eine Testnachricht je 10 s, sonst 429-artiges Problem mit neuem Code `conflict.alerting-test-too-soon` (oder vorhandenem Muster prüfen); Antwort: Klasse und Status, **nie** der Text der Gegenstelle. `kind: "test"` im Dokument, keine Ereignisse, ändert den Stand der Bedingungen nicht, setzt aber `last_test` (getrennt von `last_delivery`, damit ein Test nicht eine echte Störung der Auslieferung überdeckt).
- `GET /api/v1/alerting` (Einstellung und letzte Auslieferung): Admin. `GET /api/v1/alerting/events` (Ereignisliste, Sätze der Karten): Reader (die Sätze sind die der Clusterkarte, die ein Reader schon sieht). Der Planner darf beides zu einer Admin-Route zusammenlegen; die Trennung ist die kleinere Preisgabe.
- **Audit:** der Body des `PUT` trägt die URL; `audit` ist eine Erlaubnisliste mit Standard "redigiert" ("There is no list of exclusions in this package and there is no branch that passes an unrecognised value through"): `RedactedMarker = "<redacted>"` [VERIFIED: internal/audit/redact.go:31]. Einträge: `"alerting.configure": {"enabled"}` und `"alerting.test": {}`. **Die URL fehlt absichtlich in der Liste**, damit der Standardzweig sie als `<redacted>` schreibt; ein Schlüssel `url` darf nie aufgenommen werden. Wer festhalten will, **dass** die URL gesetzt oder geändert wurde, nimmt ein serverseitig geschriebenes Merkmal (Muster `user.identity-link`: `provider`; dort wird der Wert vom Handler gebaut, nicht aus dem Body), etwa `target_host` (nur Host). Die beiden Wächter `TestEveryAuditedActionIsInTheAllowlist` und `TestEveryAllowlistedActionIsAReachableRoute` (`cmd/holzkube-managerd/allowlist_test.go`) erzwingen den Eintrag in beide Richtungen [VERIFIED: allowlist_test.go:31-59].
- **Budget-Wächter:** `TestEveryRouteThatReachesUpstreamHasABudgetRow` (`cmd/holzkube-managerd/budget_test.go`) verlangt für jede Route entweder eine `routeBudgets`-Zeile oder einen Eintrag in `noUpstream`. `GET`/`PUT` und `GET …/events` greifen nichts außerhalb (`noUpstream`, je mit Kommentar im Stil der Nachbarn). **`POST …/test` greift nach draußen** und braucht eine `routeBudgets`-Zeile mit einer **neuen Aufrufklasse** (`webhookCall`, Frist = `alerting.AttemptTimeout`, abgebildet in der `switch` an `budget_test.go:~204-215`) und `routeDeadline = alerting.AttemptTimeout`; `writeTimeout = 130 * time.Second` und `readTimeout = 30 * time.Second` [VERIFIED: cmd/holzkube-managerd/main.go:86-87] liegen weit darüber. Ein Versuch, kein Retry im Anfragepfad (die Testnachricht wiederholt nicht).
- Deps werden **als Wert** in jedes `…Routes(deps)` kopiert (Kommentar in `drift_wiring_test.go`: "a field set after the literal is nil in every handler with no compile error"). Deshalb ein AST-Wächter `alerting_wiring_test.go` nach dem Vorbild von `TestTheDaemonWiresTheDriftWatch` (Service gebaut, Routen mit Deps, Lauf auf Kontext ohne Frist, Abgriff am Sampler gesetzt).

### Befund 4 (Frage 4): Die Frist-Lehre

- Die Hintergrundschleifen starten auf `context.WithCancel(context.Background())` (`certCtx, stopCerts := context.WithCancel(context.Background())`, `go certWatch.Run(certCtx, time.Minute)` [VERIFIED: cmd/holzkube-managerd/main.go:825-827]; `driftCtx … go drift.Run(driftCtx, time.Minute)` [VERIFIED: :836-838]). Der Talos-Client lehnt jeden Aufruf ohne Frist ab (`ErrNoDeadline`, "refusing a call with no deadline"); das war der Grund, dass `certwatch` in v0.6.0 und v0.7.0 nichts las (Kommentar in `certwatch.go` `ReadBudget` und `certwatch/deadline_test.go`).
- **Für Alerting ist die Lage anders und doch dieselbe Falle:** der Auswerter liest nur Speicher und lokale Dateien (`BackupHealthFor` listet Dateien), er ruft **keinen** Talos- oder Kube-Aufruf. Wer ihn dennoch erweitert (ein "Lies jetzt den Knoten"), braucht je Aufruf ein eigenes Budget wie `configdrift.readNode`. Neu ist der **ausgehende HTTP-Aufruf**: `net/http` verlangt keine Frist, hängt aber ohne eine unbegrenzt an einer Gegenstelle, die annimmt und nicht antwortet; der einzige Sender-Goroutine würde dann nie wieder senden. Regel: **jeder Versuch** läuft unter `context.WithTimeout(ctx, AttemptTimeout)`, und der Lauf selbst startet auf einem Kontext **ohne** Frist (`alertCtx, stopAlerts := context.WithCancel(context.Background())`), mit Kommentar und AST-Wächter wie bei Drift.
- Der Wächter-Test muss den Dienst auf einem Kontext ohne Frist starten und einen Sender-Fake sehen lassen, dass `ctx.Deadline()` gesetzt ist (Form wie `certwatch.TestEveryReadCarriesADeadline`, [VERIFIED: internal/certwatch/deadline_test.go]), und einen **echten** `httptest`-Empfänger, der nie antwortet, muss nach `AttemptTimeout` als `timeout` enden, ohne dass der nächste Durchgang blockiert.
- Alle Repo-Tests laufen auf Kontexten mit Frist (`testContext` mit 60 s) und sehen den Fehler deshalb nie (Ledger 137). Der Wächter muss `context.Background()` benutzen.
- Der Sampler-Abgriff läuft im Kontext des Samplers; er darf nichts Blockierendes tun (nur Speicher unter Mutex, kein Senden, keine Datei).

### Befund 5 (Frage 5): Oberfläche, API-Vertrag, Dokumentation

- **Einstellungsseite:** `web/src/routes/settings.tsx` setzt Karten in ein Raster; `AdminOnly` ("presentation and never enforcement") umschließt Admin-Karten (`AccountsCard`, `WallLinksCard`). Neue `AlertingCard` (Datei `web/src/components/Alerting.tsx`, wie `WallLinks.tsx`) in einem `AdminOnly`, über die volle Breite (`xl:col-span-2`) wie Konten/Wall-Links. Inhalt: Zustand "Not set up" / "On" / "Paused", ein Eingabefeld **ohne Vorbelegung** vom Server (`type="password"`-artig oder `autoComplete="off"`, Platzhalter `https://ntfy.example.com/holzkube`), Knöpfe "Save" (Sudo über den globalen 428-Dialog), "Send test message", "Remove"; darunter "Last delivery" (Zeit, `ok`/Klasse, Versuche, nächster Versuch) und "Recent events" (Tabelle: Zeit, Zustand, Satz, Übergang, Zustellung `delivered`/`pending`/`failed`/`not sent`). **Geheimnis nur schreibend:** nach dem Speichern leert sich das Feld, die Karte zeigt "A URL is set (target: ntfy.example.com)". Nie ein Wert im DOM, nie im `queryKey`/Cache der Abfragen (Antwort trägt keine URL, also kann nichts gecacht werden).
- **Wortlaut:** die Sätze der Ereignisse baut der **Server** (wie `certificate_warning`, `bannerSentence`); der Browser druckt sie.
- **`api.ts`:** neue Zod-Schemas für Einstellung, Ereignisse, Auslieferung; `ACTION_LABELS`-Eintrag für `PUT /api/v1/alerting` ("Change where alerts are sent"); `layout-routes.json` ist eine Hand-Liste der **Routen** und braucht keinen Eintrag (kein neuer Bildschirm).
- **`docs/api-contract.md`:** neuer Abschnitt "Alerting" (Muster "Wall links" und "Configuration drift", `### Wall links` bei Zeile 1052), das **Webhook-Dokument als Schnittstelle** mit Version, Schlüsseln, Vokabular der `condition`/`transition`/`severity`-Werte, der Zusage "at-least-once, `delivery_id`", der Klassenliste; neue Problem-Codes in die Tabelle (`TestEveryProblemCodeIsInTheContract` und `TestEveryProblemCodeIsEmitted` in `internal/httpapi/contract_codes_test.go` erzwingen beide Richtungen); die neuen Metriken im Abschnitt "Metrics".
- **`docs/guide.md`:** Abschnitt "Alerts" (Muster "Certificates inside the cluster"/"Configuration drift"): was meldet wann, Haltezeiten, **Grenze: läuft der Pi nicht, geht nichts hinaus** (kein Totmannschalter; Heartbeat-Ereignis ist nicht in dieser Phase), Beispiel-Empfänger (ntfy mit `?tpl=yes…`, Slack `text`), "die URL ist ein Geheimnis", Neustart-Verhalten, Erststart. **Berichtigen:** `docs/guide.md:2411` "**Export, never ingest.** This does not become a monitoring pipeline: it keeps no history and it does not alert." stimmt danach nicht mehr für das Produkt (der Export alarmiert weiter nicht, das Produkt schon); gleiches gilt für den Paketkommentar von `internal/metrics/metrics.go` ("does not become a monitoring pipeline, does not keep a time series, and does not alert") [VERIFIED: beide Stellen gelesen]. Die Datenverzeichnis-Liste im Leitfaden um `alerting/` ergänzen.
- **README:** ein Punkt in "What it does"; **kein** Screenshot nötig (eine Karte in den Einstellungen, kein eigener Bildschirm); falls doch: gerendert aus `web/fixtures/demo.json`, nur Dokumentationswerte, **keine** echte URL.
- **Metriken (optional, klein):** `holzkube_alerting_enabled` (0/1), `holzkube_alerting_conditions_firing{kind}` (Zahl, alle Arten auch mit 0, wie "A series that drops to zero is still written"), `holzkube_alerting_last_delivery_age_seconds` (fehlt, wenn nie), `holzkube_alerting_delivery_failing` (0/1). Kein Knoten als Label (Kopfkommentar des Exporters), keine URL, keine Klassenwörter mit Hostnamen. Der Wert: ein externes Prometheus kann den **Kanal** überwachen. Kosten: ein Block im Exporter, eine neue `Deps`-Funktion, vier Tests. Auswahl in Open Question 10.

### Befund 6 (Frage 6): Testbarkeit

- **Empfänger:** `httptest.NewServer` (Standardbibliothek) zählt Anfragen, speichert Body und Request-URI, antwortet nach Skript (200, 500, 429 mit `Retry-After`, 307, hängt). `httptest.NewTLSServer` für die TLS-Klasse (Test mit **nicht** vertrautem Zertifikat muss `tls` ergeben).
- **Uhr und Quellen als Deps:** `Now`, `Inventory`, `Machines`, `Clusters`, `Hardware`, `Host`, `Jobs`, `Sender` (Fake mit Zähler und Fehlerskript), `ReadState`/`WriteState` (Fake oder Tempdir mit `fsstore.WriteFileAtomic`). `Tick(ctx)` ist deterministisch; `Run` nur Takt.
- **Rote Läufe:** die Tabelle "Injizierbare Fehler" unten; jede Zeile verlangt, dass der Fehler **wirklich eingesetzt** wird und der benannte Test anschlägt.
- **Linter lokal:** `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...`. In dieser Session **nicht** ausgeführt (kein Binary, kein Ziel in der Phase); der Plan muss ihn im Gate nennen. `-race` nur in CI.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Atomare Zustandsdatei | eigenes `os.WriteFile`/Rename | `fsstore.WriteFileAtomic`/`ReadFile` | Temp-Präfix, Modi, zwei fsyncs sind der Absturzvertrag des Stores; `TestNoDirectFileAccessOutsideFsstore` verbietet es ohnehin |
| Zeit/Haltezeit in Tests | `time.Sleep` | injizierte `Now` und `Tick(ctx)` | Tests warten sonst Minuten; das Repo testet überall mit Uhr als Dep |
| Zertifikats-, Temperatur-, Host-Regeln | eigene Schwellen | `certwatch.Ladder`/`Urgency`-Felder, `inventory.TemperatureLimits`, `host.Assess` | eine Regel für Bildschirm und Alarm ("D-06: the one rule") |
| JSON-Schlüsselreihenfolge, Escape | eigene Serialisierung | `encoding/json` mit festen Strukturtypen | Dokument ist Schnittstelle; Strukturtypen halten die Schlüssel stabil, Zeilenumbrüche in Sätzen werden korrekt escaped (keine Einschleusung in Empfänger) |
| Zufällige `delivery_id` | Zeit-/Zählerableitung | `crypto/rand` (16 Byte, Hex) | raten/kollidieren ausgeschlossen; Vorbild `newWallToken` in `internal/auth/walllink.go` |
| Audit-Redaktion der URL | Sonderfall im Handler | Standardzweig der Erlaubnisliste | "no branch that passes an unrecognised value through" |
| Weiterleitungs-/Antwortgrenze | eigene Schleifen | `CheckRedirect`/`io.LimitReader` | Muster `k8srelease`/`imagefactory` |
| Einstellung mit Konkurrenzprüfung | eigene Sperre für die URL | `settingsStore.Put` (Rev-CAS) | existiert, getestet |

**Key insight:** Die Phase erfindet keine Regeln; sie **verfolgt Übergänge** der Regeln, die die Anzeige schon hat. Jede selbstgebaute Schwelle ist ein Alarm, der dem Bildschirm widerspricht.

## Runtime State Inventory

Nicht zutreffend: keine Umbenennung, kein Refactoring, keine Migration (kein Schema-Sprung in der empfohlenen Variante). Für die Variante B (Entity + Schema 8) gälte dieselbe Prüfung wie in Phase 15 (`version-7`-Fixture, Migrationstest).

## Common Pitfalls

### Pitfall 1: "Nicht gelesen" als "wieder in Ordnung"
**What goes wrong:** Nach jedem stündlichen Update-Neustart sind `NodeCertificates` und `ConfigDrift` nil, Knoten `connecting`; ein Auswerter, der nil als "kein Problem" liest, meldet "recovered" für alles und eine Minute später wieder "attention".
**Why it happens:** zweiwertige Logik (`problem := …; if !problem {…}`).
**How to avoid:** `Unknown` ist der Nullwert von `State`; `Unknown` ändert den Stand nie.
**Warning signs:** Ereignisse im Abstand von einer Minute nach jedem Update; Test "erst nil, dann Zertifikate" ergibt Ereignisse.

### Pitfall 2: Neustart meldet alles neu
**What goes wrong:** Stand nur im Speicher; jeder Update-Neustart sendet jede offene Störung erneut.
**How to avoid:** Stand `announced` persistiert; Test: Dienst auf demselben Verzeichnis neu bauen, gleiche Beobachtung, **kein** Ereignis (Fehler "Stand nur im Speicher" muss rot werden).

### Pitfall 3: Das Token steht im Fehlertext
**What goes wrong:** `url.Parse`-Fehler und `Client.Do`-Fehler tragen die ganze URL. **Gemessen** (Go 1.27.2, Mac, Scratchpad): `parse "https://example.com/%zz?token=SECRETTOKEN": invalid URL escape "%zz"` und `Post "http://127.0.0.1:1/hook?token=SECRETTOKEN": dial tcp 127.0.0.1:1: connect: connection refused`; nach `errors.As(err, &ue)` bleibt in `ue.Err` noch `dial tcp 127.0.0.1:1: …` (die Adresse). Dieser Text gelangt über `slog`, über `OutcomeCause` ins Audit-Ergebnis und über einen Problem-Detail in die Antwort.
**How to avoid:** Der Sender gibt nur Klassen zurück (feste Menge, Konstanten); beim Parsen wird der Fehler **verworfen** und durch einen festen Satz ersetzt; der Logger bekommt Klasse, Statuscode, Versuchszahl, nie die URL und nie `err`. Wächter: Test mit einer URL, deren Token ein unverwechselbarer Marker ist (`SECRETTOKEN-…`); der Test fängt `slog`-Ausgabe (Handler auf `bytes.Buffer`), die Antwort des `PUT`/`test`, das Audit (Tempdir-Archiv) und die Zustandsdatei ab und verlangt, dass der Marker in **keinem** vorkommt, außer in `settings.json`.

### Pitfall 4: Keine Frist am Sender, Lauf auf Kontext mit Frist
**What goes wrong:** eine Gegenstelle, die annimmt und schweigt, hält den Sender für immer; oder der Lauf stirbt mit einer Frist still (certwatch, Quick-Task 261014-gl2).
**How to avoid:** Befund 4; zwei Wächter (Fake sieht `Deadline`, echter `httptest`-Hänger endet als `timeout`); AST-Wächter für den Kontext in `main.go`.

### Pitfall 5: Flattern durch Neustarts von Knoten
**What goes wrong:** Ein Talos-Update oder ein Reboot dieses Produkts legt einen Knoten für 2 bis 10 Minuten lahm (Stufe `down` nach etwa 90 s, `downgradeAfter = 2` bei 45 s Takt); der Betreiber bekommt bei jedem Upgrade "down" und "recovered".
**How to avoid:** Haltezeit für `node-unreachable` (Vorschlag 5 min) plus Unterdrückung, solange der Cluster einen `pending`/`running` Job hat; `recovered` erst nach Klärzeit. Ein Zustand, der nie die Haltezeit erreicht, erzeugt nie ein Ereignis (auch kein `recovered`).

### Pitfall 6: Temperatur an der Schwelle flattert
**What goes wrong:** ein Pi-CPU-Wert um 80 °C erzeugt Ereignisse im Minutentakt.
**How to avoid:** Wert-Hysterese (3 °C) **und** Haltezeit; Schwelle ist `WarnC`/`DangerC` des Sensors (nicht eine eigene Zahl). Die Wahl der Auslöser ist Open Question 2.

### Pitfall 7: Ausgeschaltete oder entfernte Knoten melden
**What goes wrong:** `Machine.Disabled` und `Cluster.Disabled` stehen nicht in den Ansichten; ein absichtlich ausgeschalteter Knoten ist `down` und meldet täglich. Ein vergessener Knoten hinterlässt einen ewig offenen Eintrag.
**How to avoid:** Records mitlesen; im `Tick` Einträge verschwundener Knoten/Cluster still räumen.

### Pitfall 8: Ein "zugestellt", das keines war
**What goes wrong:** Stand wird beim Erzeugen des Ereignisses auf `announced` gesetzt, der POST scheitert, die Störung wird nie gemeldet.
**How to avoid:** `announced` rückt erst nach einem 2xx vor (Pattern 2); Test: Empfänger antwortet 500, Stand bleibt "nicht gemeldet", nach Aufgeben `failed` sichtbar, **nicht** `delivered`.

### Pitfall 9: Eine Ursache, N Meldungen
**What goes wrong:** Admin-Zertifikat abgelaufen oder der Pi hat das LAN verloren: alle Knoten `down`.
**How to avoid:** Ursachenunterdrückung (Befund 1); mehrere Ereignisse eines Durchgangs in einem Dokument; höchstens 20 je Dokument.

### Pitfall 10: Host-Befunde ohne Schlüssel
**What goes wrong:** `host.Health` hat nur Sätze mit Zahlen ("82.1 °C ≥ 80 °C"). Als Schlüssel genommen, wechselt er mit jedem Wert, und jede Zehntelgrad-Änderung wäre ein neues Ereignis.
**How to avoid:** stabilen `Key` an den Befund geben (Befund 1) oder genau eine Host-Bedingung mit dem Satz des Zeitpunkts.

### Pitfall 11: Dokumentation widerspricht dem Produkt
**What goes wrong:** `docs/guide.md:2411` und der Paketkommentar der Metriken sagen "does not alert".
**How to avoid:** im selben Schritt berichtigen (Aufgabe im Plan, nicht "später").

### Pitfall 12: Echte Werte im Repository
**What goes wrong:** Platzhalter, Testfixture oder `demo.json` mit einer echten ntfy-/Knotenadresse (Vorfall bis 2026-10-03).
**How to avoid:** nur `https://ntfy.example.com/holzkube`, `homeserver`, `192.168.1.10`; `internal/publicrepo` läuft im Gate.

### Pitfall 13: Rollback verliert die URL
**What goes wrong:** ein zurückgerolltes Binary kennt `Settings.Alerting` nicht und schreibt den Record beim nächsten Wall-Link ohne das Feld neu.
**How to avoid:** im Leitfaden in einem Satz sagen; kein Code nötig (Open Question 9).

## Code Examples

### Dreiwertiger Zustand, Zero Value ist "unbekannt"
```go
// Source: eigene Skizze (Muster: health.Stage, deren Nullwert StageUnknown ist: "so that a freshly loaded
// record reads as 'nobody has looked yet' rather than as a claim", internal/health/stage.go)
type State int

const (
	Unknown State = iota
	OK
	Firing
)
```

### Fehlerklassen statt Fehlertext (der Fehler des Senders trägt nie eine URL)
```go
// Source: eigene Skizze nach internal/k8srelease/k8srelease.go:one (errors.As auf *url.Error) und der Messung
// aus dem Scratchpad (Fehler enthalten die URL).
type Class string

const (
	ClassOK          Class = "ok"
	ClassTimeout     Class = "timeout"
	ClassUnreachable Class = "unreachable"
	ClassTLS         Class = "tls"
	ClassRefused     Class = "refused-by-endpoint" // 4xx except 408, 425, 429
	ClassRateLimited Class = "rate-limited"
	ClassServer      Class = "server-error"
	ClassRedirect    Class = "redirect"
)

func classify(err error) Class {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err // drop the URL, then map to a fixed word; never return err.Error()
	}
	var cert x509.UnknownAuthorityError // plus CertificateInvalidError, HostnameError, tls.RecordHeaderError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return ClassTimeout
	case errors.As(err, &cert):
		return ClassTLS
	default:
		return ClassUnreachable
	}
}
```

### Client ohne Weiterleitung, Frist je Versuch
```go
// Source: eigene Skizze nach internal/imagefactory/client.go (kein Client.Timeout, Frist je Aufruf)
client := &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}
callCtx, cancel := context.WithTimeout(ctx, AttemptTimeout)
defer cancel()
req, _ := http.NewRequestWithContext(callCtx, http.MethodPost, target, bytes.NewReader(doc))
req.Header.Set("Content-Type", "application/json")
resp, err := client.Do(req)
// ... io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10)) ; resp.Body.Close() ; classify by status
```

### Abgriff am Sampler (keine zweite Lesung)
```go
// Source: cmd/holzkube-managerd/main.go:808 setzt heute `Hardware: inv.Hardware,`
Hardware: func(ctx context.Context, id model.MachineID) (inventory.HardwareView, error) {
	v, err := inv.Hardware(ctx, id)
	alerts.ObserveHardware(id, v, err, time.Now()) // nur Speicher unter Mutex; ein Fehler heißt "unbekannt", nie "ok"
	return v, err
},
```

### Route-Deklaration (nach `backups.go` und `walllink.go`)
```go
// Source: internal/httpapi/handlers/backups.go (cluster.backup-schedule) und walllink.go (wall-link.create)
{
	Method:          http.MethodPut,
	Pattern:         "/api/v1/alerting",
	RequiresSession: true,
	MinRole:         model.RoleAdmin,
	Destructive:     true,
	Action:          "alerting.configure",
	Handler:         handler(alertingConfigure(d)),
},
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| nur `/metrics`, Wand, Banner | Webhook mit Übergangsereignissen | diese Phase | Das Produkt alarmiert erstmals; Kommentare "does not alert" sind zu berichtigen |
| `certwatch` auf Kontext ohne Frist, nicht lesend | eigenes Budget je Lesung (configdrift) | v0.8.0 | Muster für Frist je Aufruf, hier für den Sender |
| ein Entity-Ordner je Funktion mit Migration | Datei neben dem Store (`history`) | v0.x | Kein Schema-Sprung nötig, rückwärtsverträglich |

**Deprecated/outdated:** nichts im Repo wird ersetzt; die beiden Sätze "does not alert" werden falsch.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Die `condition`-Wörter (`node-unreachable` …) und Ereignisschlüssel sind ein guter Entwurf für die veröffentlichte Schnittstelle | Befund 3 | Costly to change nach Veröffentlichung (D-01); der Betreiber/Planner muss das Vokabular bestätigen |
| A2 | 30 s Auswertungstakt, 30 s Mindestabstand, 20 Ereignisse je Dokument, 50 im Ring, 10 s je Versuch, 3500 Zeichen sind vernünftige Zahlen | Befund 2, 3 | Zahlen sind Ermessen; zu knapp = verzögerte Meldung, zu weit = Flut |
| A3 | Gotify nimmt ein JSON-Dokument an (die gelesene Seite zeigt nur Formularfelder) und ignoriert unbekannte Schlüssel | Befund 3 | Der Leitfaden darf Gotify erst nennen, wenn es jemand probiert hat; sonst nur Slack und ntfy (belegt) |
| A4 | `Retry-After` bei 429/503 ist allgemein zu beachten (für Slack nicht beschrieben) | Befund 3 | Schlimmstenfalls zu früher Wiederholungsversuch (begrenzt, nur vier Versuche) |
| A5 | Slack und Gotify ignorieren unbekannte Spitzenschlüssel (`title`, `events`, …) | Befund 3 | Ein Dienst, der unbekannte Schlüssel ablehnt, bekäme `refused-by-endpoint`; ein Test bei echten Diensten fehlt |
| A6 | Ein Talos-Reboot beim Update dauert 2 bis 10 Minuten | Pitfall 5 | Zu kurze Haltezeit = Meldung bei jedem Upgrade; nicht auf echter Hardware gemessen |
| A7 | `ProxyFromEnvironment` ist für den Dienst unkritisch (systemd-Dienst ohne Proxy-Umgebung) | Befund 3 | Ein gesetzter Proxy sähe das Token einer `http://`-URL |
| A8 | Ein älteres Binary schreibt den Settings-Record ohne das unbekannte Feld neu | Befund 2, Pitfall 13 | Aus Code gelesen (Decoder ohne `DisallowUnknownFields`, `Put` serialisiert die Struktur), nicht mit einem alten Binary geprüft |
| A9 | Der Sampler ruft `Hardware` nur für Maschinen in einem Cluster, nach Start sofort und dann alle 15 s | Befund 1 | Gelesen im Kommentar und in `pass`; Dauer bis zur ersten Lesung nicht gemessen |

## Open Questions (Auswahl an den Betreiber)

Jede Frage ist eine Auswahl; Empfehlung zuerst. Sie werden dem Betreiber über `AskUserQuestion` gestellt (CLAUDE.md), nicht als offene Frage im Text. Sinnvoll gebündelt in drei Runden: (a) was meldet (1, 2), (b) wie zuverlässig (3, 4, 5), (c) Kanaldetails (6 bis 10).

1. **Welche Zustände melden standardmäßig?**
   - **A (empfohlen): alle acht, je Art ein Schalter in der Karte, Standard an.** Kosten: acht Schalter in Oberfläche und Stand; Nutzen: Drift/Temperatur lassen sich bei Rauschen stummschalten, ohne den Kanal zu entfernen (keine Regeln je Knoten, das bleibt aufgeschoben).
   - B: alle acht an, ohne Schalter. Kosten: Rauschen ist nur durch Entfernen der URL abzustellen. Nutzen: kleinste Oberfläche.
   - C: nur kritische Stufen (Knoten down, Platte/Temperatur rot, Zertifikat kritisch, Snapshot überfällig). Kosten: Drift und die Vorwarnstufen fehlen. Nutzen: ruhigster Kanal.

2. **Schwellen und Haltezeiten.** Vorschlag je Art, jeweils mit Alternative:
   - **Knoten nicht erreichbar:** Haltezeit **5 min** (empfohlen; deckt einen Reboot in 2 bis 5 min) · 2 min (schnell, flattert bei Upgrades) · 15 min (ruhig, spät). Zusatz: Unterdrückung bei laufendem Job des Clusters **ja** (empfohlen; Kosten: Jobliste als Quelle, Nutzen: kein Alarm bei eigenen Upgrades) / nein.
   - **Temperatur:** **A rot sofort nach 5 min, amber nur bei 30 min Dauer** (empfohlen; Pi-Spitzen um 80 °C bleiben still) · B nur rot · C amber sofort (laut).
   - **Platte (Knoten und Host):** **amber 80 %, rot 90 % wie der Bildschirm, Klären unter 75 %, Haltezeit 15 min** (empfohlen) · nur rot ab 90 % (ruhiger, später).
   - **Zertifikate:** ab **`banner`** und `critical` (Admin-Zertifikat 30/7 Tage, Authority 30/7, ausgestellt 14/3) (empfohlen) · zusätzlich `badge` (90 Tage; früh, einmalig). Eine Authority wird einmal je Cluster gemeldet, nicht je Control-Plane-Knoten.
   - **Snapshot überfällig:** wie `Overdue` (zwei verpasste Intervalle), Haltezeit keine.
   - **Drift:** **erst wenn das Banner steht (24 h unbestätigt)** (empfohlen, deckt sich mit D-06 und dem Rauschbedenken) · sofort nach der Lesung (drei Stunden Takt; jede fremde Änderung per `talosctl` wird sofort gemeldet, Kosten: mehr Fehlalarme).

3. **Wiederholung bei bleibender Störung.**
   - **A (empfohlen): keine Erinnerung; die Karte zeigt, was offen ist.** Kosten: eine vergessene Störung bleibt still. Nutzen: kein Zusatzzustand, kein Rauschen; passt zu "keine Wiederholung jeden Takt".
   - B: Erinnerung alle 24 h für kritische Störungen. Kosten: Zustand und Einstellung. Nutzen: vergessene Dinge kommen wieder.
   - C: tägliche Zusammenfassung aller offenen Dinge. Kosten: ein zweiter Dokumenttyp. Nutzen: ein Rhythmus. (Eskalationsstufen sind aufgeschoben.)

4. **Wiederholung bei Fehlschlag (Grenze).**
   - **A: vier Versuche (0, 1, 5, 30 min), dann `failed`, sichtbar; offen gebliebene, noch feuernde Störungen werden einmal je Stunde erneut versucht, solange sie nicht zugestellt sind** (empfohlen, "level-triggered": nichts geht verloren, die Rate bleibt begrenzt). Kosten: etwas mehr Zustand.
   - B: vier Versuche, dann `failed` und Ende. Kosten: ein Ausfall des Empfängers von mehr als einer halben Stunde verschluckt die Meldung, solange die Störung nicht erneut wechselt. Nutzen: einfach.
   - C: Warteschlange bis zugestellt. Kosten: wächst bei langem Ausfall, braucht Deckel und Sicherung. Nutzen: nichts geht verloren.

5. **Erststart (und beschädigter Stand): was passiert mit dem, was schon schlecht ist?**
   - **A (empfohlen, mit Zusatz): still als bekannt festhalten, aber beim Speichern der URL **eine** Bestätigungsmeldung (`kind: test` mit der Zahl offener Dinge) senden.** Kosten: eine Nachricht. Nutzen: der Betreiber sieht Kanal und Ausgangslage, ohne Flut.
   - B: ein Sammeldokument mit allem, was jetzt offen ist. Kosten: ein langes Dokument. Nutzen: nichts Offenes bleibt unbemerkt.
   - C: jedes offene Ding einzeln. Kosten: Flut (bei vielen Knoten). Nutzen: einfachste Regel.

6. **URL-Regeln.**
   - Klartext `http://`: **erlauben, bei Nicht-Loopback mit Warnung in der Karte** (empfohlen; ntfy/Gotify im LAN) · nur `https://` (sicherer, schließt den Normalfall im LAN aus).
   - Link-Local und Private sperren: **nicht sperren** (empfohlen, Begründung in Befund 3) · nur `169.254.0.0/16` sperren (kostet nichts, schützt vor nichts Relevantem) .
   - Ziel `scheme://host:port` in der Karte zeigen: **ja** (empfohlen; Admin-only, sonst weiß der Betreiber nicht, was eingetragen ist) · nur "gesetzt".
   - Selbstsigniertes LAN-HTTPS: **nicht in dieser Phase** (Anheften des Fingerabdrucks wie bei `cluster.fingerprint` wäre die Erweiterung).

7. **Hilfsschlüssel im Dokument.**
   - **A (empfohlen): `title`, `message`, `text` flach neben den strukturierten Feldern.** Kosten: derselbe Inhalt dreimal. Nutzen: Slack, ntfy mit Template und vermutlich Gotify ohne Skript.
   - B: nur `text`. Kosten: ntfy-Templates müssen `{{.text}}` benutzen, Gotify geht nicht. Nutzen: kleiner.
   - C: nur strukturierte Felder. Kosten: jeder Empfänger braucht ein Skript. Nutzen: sauberste Schnittstelle.

8. **`--dry-run`: wird gesendet?**
   - **A (empfohlen): ja; Auswertung und Auslieferung laufen.** Die Phase ändert nichts an einem Knoten; dry-run "refuses every mutating call at the transport" (`internal/config/config.go:702`). Kosten: ein dry-run-Probelauf kann echte Meldungen auslösen. Nutzen: gleiches Verhalten, Test möglich.
   - B: auswerten und anzeigen, aber nichts senden (Ereignis "not sent (dry-run)"). Kosten: Karte und Test-Knopf wären im Probelauf stumm. Nutzen: ein Probelauf stört keinen Dritten.

9. **Persistenz.**
   - **A (empfohlen): additives Feld am `Settings`-Record plus eine Datei `alerting/state.json` über `fsstore.WriteFileAtomic`.** Kosten: nach einem Rollback ist die URL weg, wenn das ältere Binary den Settings-Record neu schreibt; Stand außerhalb der Rev-Prüfung (ein Schreiber, Mutex). Nutzen: kein Schema-Sprung, ein zurückgerolltes Binary läuft weiter.
   - B: neuer Entity `alert-state` und Schema 7 → 8 (Migrationstest, `version-7`-Fixture). Kosten: ein zurückgerolltes Binary lehnt das Verzeichnis ab (`ErrVersionTooNew`), wie bei Phase 15 (dort bewusst gewählt). Nutzen: Rev-CAS, `safeKey`, einheitliches Muster.

10. **Metriken für den Kanal.**
    - **A (empfohlen): vier kleine Serien (`holzkube_alerting_enabled`, `…_conditions_firing{kind}`, `…_last_delivery_age_seconds`, `…_delivery_failing`).** Kosten: ein Block, vier Tests. Nutzen: ein externes Prometheus sieht, ob der Kanal lebt.
    - B: keine. Kosten: der Kanal ist nur in der Karte sichtbar.

Nicht als Auswahl, sondern als **Grenze für den Leitfaden:** läuft der Pi nicht, geht nichts hinaus; ein Heartbeat-Ereignis ("ich lebe") wäre die Gegenmaßnahme, ist hier **nicht** vorgesehen und gehört, wenn gewünscht, in eine eigene Phase.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Bauen, Tests | ✓ (Mac) | go1.27.2 (Repo verlangt `toolchain go1.26.9`; mit `GOTOOLCHAIN=go1.26.9`) | — |
| golangci-lint v2.13.1 | CI-Linter lokal | ✗ (kein Binary) | — | `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...` (braucht Netz beim ersten Mal) |
| `task` | `task ci` | ✗ | — | die darunterliegenden Befehle (`go test ./... -count=1`, `npm --prefix web run test`) |
| Node/npm | Web-Tests | ✓ | (nicht abgefragt) | — |
| Pi (aarch64), echter Cluster, echter Webhook-Empfänger | Handproben | ✗ | — | `httptest`-Empfänger, `talossim`; echte Prüfung bleibt `human_needed` |

**Missing dependencies with no fallback:** keine für die Entwicklung; ein echter Empfänger (ntfy/Slack) und der Pi sind nur für Handproben nötig.
**Missing dependencies with fallback:** golangci-lint, task.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` (+ `net/http/httptest`), Web: vitest (`npm --prefix web run test`) |
| Config file | `Taskfile.yml` (`test`, `ci`); keine neue Konfiguration |
| Quick run command | `go test ./internal/alerting/ ./internal/audit/ ./internal/metrics/ -count=1` |
| Full suite command | `go test ./... -count=1` und `go vet ./...`; Web: `npm --prefix web run test` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ALRT-01 | Übergänge statt jedem Takt; Haltezeit; Hysterese; `worsened` | unit | `go test ./internal/alerting/ -run 'TestTransitions\|TestHold\|TestHysteresis' -count=1` | ❌ Wave 0 |
| ALRT-02 | je Zustand eine Bedingung; `unknown` nie "ok"; Disabled/Job/Admin-Zertifikat unterdrücken | unit | `go test ./internal/alerting/ -run 'TestAssess' -count=1` | ❌ Wave 0 |
| ALRT-03 | Neustart sendet nichts neu; Erststart/beschädigter Stand = Ausgangslage | unit (fsstore im Tempdir) | `go test ./internal/alerting/ -run 'TestRestart\|TestFirstRun\|TestDamagedState' -count=1` | ❌ Wave 0 |
| ALRT-04 | Dokument v1: Schlüssel, `delivery_id` stabil über Wiederholungen, Obergrenzen | unit | `go test ./internal/alerting/ -run 'TestDocument' -count=1` | ❌ Wave 0 |
| ALRT-05 | Frist je Versuch, Klassen, keine Weiterleitung, Antwortgrenze, Retry-After, `failed` sichtbar, kein Geheimnis | unit + `httptest` | `go test ./internal/alerting/ -run 'TestSender\|TestDeadline\|TestNoSecret' -count=1` | ❌ Wave 0 |
| ALRT-06 | Admin+Sudo, Audit ohne URL, Test nur an gespeicherte URL, Drosselung | integration (`internal/httpapi`) | `go test ./internal/httpapi/ ./cmd/holzkube-managerd/ -run 'Alerting\|Allowlist\|Budget' -count=1` | ❌ Wave 0 (Allowlist-/Budget-Wächter ✅ bestehen, brauchen Einträge) |
| ALRT-07 | Karte: URL nie im DOM, Letzte Auslieferung, Ereignisliste | web unit | `npm --prefix web run test -- Alerting` | ❌ Wave 0 |
| ALRT-08 | README/Leitfaden/Vertrag/Metriken; "does not alert" berichtigt | unit (Text) | `go test ./internal/config/ ./internal/httpapi/ -run 'Readme\|ProblemCode' -count=1` | ✅ (bestehende Wächter), neue Prüfung ❌ |

### Injizierbare Fehler (rote Läufe für den Plan)

Jede Zeile: der Fehler, der **wirklich eingesetzt** werden muss, und der Test, der dabei rot werden muss. Ein grüner Lauf nach einer Injektion, die nichts ändert, zählt nicht.

| Kernaussage | Injektion | Erwartet rot |
|-------------|-----------|--------------|
| Ein Zustand erzeugt nicht jeden Takt ein Ereignis | Vergleich mit dem Stand entfernen (immer senden, solange feuernd) | `TestAStandingFaultIsSentOnce` (zehn Takte, ein Ereignis) |
| Ein Neustart meldet nichts neu | Stand nur im Speicher halten (nicht laden) | `TestARestartSendsNothingAgain` (Dienst auf demselben Verzeichnis neu bauen) |
| Nach einem Neustart ist "nicht gelesen" kein "in Ordnung" | `Unknown` wie `OK` behandeln (nil `NodeCertificates` → `recovered`) | `TestUnknownNeverRecovers` |
| Erholung wird gesendet | `recovered`-Zweig entfernen | `TestARecoveryIsSent` |
| Erholung für nie Gemeldetes wird nicht gesendet | `Announced`-Prüfung entfernen | `TestARecoveryOfWhatWasNeverAnnouncedIsNotSent` |
| Haltezeit | `For` auf 0 | `TestAShortBlipIsNotAnAlert` (Knoten 2 min down, dann wieder da: kein Ereignis) |
| Hysterese | Klärschwelle = Auslöseschwelle | `TestATemperatureOnTheLineDoesNotFlap` |
| Ein fehlgeschlagener Versand gilt nicht als zugestellt | `announced` beim Erzeugen setzen statt nach 2xx | `TestAFailedDeliveryIsNotMarkedDelivered` (Empfänger 500; Stand und Ereignisliste prüfen) |
| Wiederholung mit Grenze | Grenze entfernen (unbegrenzt) | `TestRetriesStopAtTheLimit` (Zähler am Empfänger = 4, Zustand `failed`) |
| Kein Retry bei 4xx | 4xx wie 5xx behandeln | `TestARefusalIsNotRetried` |
| Retry-After wird beachtet und gedeckelt | Header ignorieren / nicht deckeln | `TestRetryAfterIsHonouredAndCapped` |
| Kein Geheimnis in Fehler, Log, Antwort, Audit, Zustandsdatei | `err.Error()` des `url.Error` durchreichen / URL ins Log | `TestTheTokenAppearsNowhereButSettings` (Marker `SECRETTOKEN-…`; fängt `slog`, Antwort, Audit-Archiv, `state.json` ab) |
| Parse-Fehler trägt nicht die Eingabe | `url.Parse`-Fehler in den Problem-Detail | `TestARejectedURLIsNotEchoed` |
| Keine Frist im Lauf | `context.WithTimeout` im Sender entfernen | `TestEveryAttemptCarriesADeadline` (Kontext **ohne** Frist; Fake sieht `Deadline`) und `TestAHangingEndpointEndsAsTimeout` (echter `httptest`-Hänger) |
| Lauf auf Kontext ohne Frist | `main.go` startet mit `WithTimeout` | `TestTheDaemonWiresAlerting` (AST, wie `TestTheDaemonWiresTheDriftWatch`) |
| Keine Weiterleitung | `CheckRedirect` entfernen | `TestARedirectIsNotFollowed` (zweiter Server sieht keine Anfrage) |
| Antwortgrenze | `LimitReader` entfernen | `TestAHugeAnswerIsNotReadWhole` |
| TLS wird geprüft | `InsecureSkipVerify` setzen | `TestAnUntrustedCertificateFails` (`httptest.NewTLSServer`, Klasse `tls`) |
| Ausgeschaltete Knoten melden nicht | `Disabled`-Filter entfernen | `TestADisabledNodeNeverAlerts` |
| Eine Ursache, eine Meldung | Unterdrückung (Admin-Zertifikat abgelaufen) entfernen | `TestAnExpiredClientCertificateSilencesTheNodes` |
| Laufender Job unterdrückt "down" | Unterdrückung entfernen | `TestARunningJobSilencesItsNodes` |
| Verschwundener Knoten räumt seinen Eintrag, ohne zu melden | Abgleich entfernen | `TestAForgottenNodeLeavesQuietly` |
| Ein Dokument je Durchgang, höchstens 20 Ereignisse | Obergrenze entfernen / je Ereignis ein POST | `TestOneDocumentPerPassWithAtMostTwentyEvents` |
| Test sendet nur an die gespeicherte URL | `url` aus dem Body nehmen | `TestTheTestMessageIgnoresAnyURLInTheRequest` |
| Testnachricht ändert keinen Zustand | Stand in `test` verändern | `TestATestMessageLeavesTheConditionsAlone` |
| Admin + Sudo für `PUT` | `Destructive` entfernen / `MinRole` senken | bestehender Routen-Wächter plus `TestConfigureNeedsAdminAndSudo` |
| Audit schreibt die URL nie | `"url"` in die Erlaubnisliste | `TestTheAuditRecordNeverHoldsTheURL` und `TestAllowlistHasNoURL` |
| URL wird nie zurückgegeben | `url` in die Ansicht | `TestTheViewNeverCarriesTheURL` (gesamte JSON-Antwort nach Marker durchsucht) |
| Wand und Support-Bundle ohne URL/Ereignisse | — (Beobachtung) | Wand-JSON und Bundle enthalten Marker nicht (Stil `wall_certificates_test`) |
| Rollback-Verträglichkeit (Variante A) | Schema-Sprung einführen | `migrate`-Test: `CurrentVersion` bleibt 7; ein Verzeichnis mit `alerting/` wird von `Guard` und `migrate.Run` akzeptiert |

### Sampling Rate
- **Per task commit:** `go test ./internal/alerting/ -count=1` (Sekunden)
- **Per wave merge:** `go test ./... -count=1` und `go vet ./...`; Web: `npm --prefix web run test`
- **Phase gate:** `go test ./... -count=1` grün, Linter lokal grün; `-race` und `task ci` nur in CI (im Bericht sagen, wo gelaufen)

### Wave 0 Gaps
- [ ] `internal/alerting/*_test.go` mit Fake-Deps (Uhr, Quellen, Sender, Stand) und `httptest`-Empfängern
- [ ] `cmd/holzkube-managerd/alerting_wiring_test.go` (AST-Wächter) und Zeile in `routeBudgets` mit neuer Klasse `webhookCall`; Einträge in `noUpstream`
- [ ] Allowlist-Einträge `alerting.configure`/`alerting.test` (`internal/audit/redact.go`), Problem-Codes in `docs/api-contract.md`
- [ ] Web: `web/src/components/Alerting.test.tsx`, Fixture in `web/fixtures/demo.json` (nur Dokumentationswerte)
- [ ] Falls Open Question 9 B: `version-7`-Fixture und Test in `internal/store/migrate/migrate_test.go`

## Security Domain

`security_enforcement` ist in `.planning/config.json` auf `true`, `security_asvs_level` 1, `security_block_on` `high`.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | nein (nutzt Sitzungen/Sudo des Produkts) | — |
| V3 Session Management | ja, indirekt | bestehende Sitzung + Sudo-Fenster (`Destructive: true`) für das Ändern |
| V4 Access Control | ja | `MinRole: RoleAdmin` für `PUT`/`GET`/`test`, Reader für Ereignisliste; Routen-Tabelle trägt es |
| V5 Input Validation | ja | URL: nur `http`/`https`, nicht leer, höchstens 2048 Zeichen, **nur Syntax**, keine Auflösung; `decode` mit `DisallowUnknownFields` (`handlers.go:34`) |
| V6 Cryptography | nein (keine eigene Kryptografie) | `crypto/rand` für `delivery_id`; TLS-Prüfung der Standardbibliothek, nie abgeschaltet |
| V7 Error Handling and Logging | **ja, zentral** | feste Fehlerklassen; URL nie in Fehler, Log, Audit, Antwort |
| V8 Data Protection | ja | URL in `settings.json` (0600, Verzeichnis 0700, `Guard`); nie in Antworten, Bundle, Wand; Hinweis im Leitfaden zur Sicherung des Verzeichnisses |
| V9 Communications | ja | TLS-Prüfung an; Klartext-`http://` mit sichtbarer Warnung; keine Weiterleitung |
| V13 API | ja | Routen-Tabelle, Allowlist- und Budget-Wächter, Problem-Codes im Vertrag |

### Known Threat Patterns for {Go-Daemon mit ausgehendem Webhook}

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Token der URL in Logs/Fehlern/Audit (gemessen: `url.Error`, `url.Parse`) | Information Disclosure | feste Klassen; Audit-Erlaubnisliste ohne `url`; Wächter-Test mit Marker |
| SSRF über die konfigurierte URL oder die Testnachricht | Information Disclosure / Elevation | nur Admin+Sudo setzt; Test nur an gespeicherte URL; Antwort nie angezeigt; nur Klasse/Status; Entscheidung "private Ziele erlaubt" begründet (Befund 3) |
| Weiterleitung leitet Token/Body an Dritte | Information Disclosure | `ErrUseLastResponse` |
| Gegenstelle hängt/antwortet endlos | Denial of Service | Frist je Versuch, Antwortgrenze 4 KiB, ein Sender, Abstand zwischen Dokumenten |
| Flut (viele Knoten, Flattern) | Denial of Service (Dritter) | Haltezeit, Hysterese, ein Dokument je Durchgang, 20 Ereignisse, Mindestabstand |
| Einschleusen in Empfänger (Markdown, Zeilenumbrüche aus Hostnamen) | Tampering | JSON-Strukturtypen; Sätze vom Server aus festen Mustern; Hostnamen sind Knotenwerte, nicht Betreibereingabe von außen; Länge gedeckelt |
| Gefälschte Meldung an den Empfänger | Spoofing | kein Schutz außer der geheimen URL (Stand der Technik bei Slack/ntfy-URLs); HMAC-Signatur ist aufgeschoben und braucht ein zweites Geheimnis |
| URL im Support-Bundle/Backup | Information Disclosure | Bundle liest `Settings()` nicht (Wächter); Verzeichnissicherung enthält sie wie andere Geheimnisse (Leitfaden) |
| Beschädigter Stand erzeugt Flut | Denial of Service | beschädigt = Ausgangslage, nicht leer |

## Sources

### Primary (HIGH confidence)
- Repository, **gelesen in dieser Session** (Pfad: Inhalt): `.planning/phases/19-alerting/19-CONTEXT.md`, `CLAUDE.md`, `.planning/phases/15-…/15-RESEARCH.md` (Muster), `.planning/AUDIT-2026-10-08.md` (Zeilen zu Alerting), `.planning/ROADMAP.md` (Phase 19), `internal/certwatch/certwatch.go` und `deadline_test.go`, `internal/configdrift/health.go`, `lifecycle.go`, `configdrift.go` (Run/Tick/Flush), `internal/model/configdrifthealth.go`, `backup.go`, `certificates.go`, `job.go`, `model.go` (Settings, Rollen), `internal/inventory/view.go`, `inventory.go` (downgradeAfter), `hardware.go`, `limits.go`, `internal/health/health.go`, `stage.go`, `internal/metrics/metrics.go`, `internal/host/health.go`, `sample.go`, `internal/history/sampler.go`, `persist.go`, `internal/store/store.go`, `fsstore/fsstore.go`, `permissions.go`, `migrate/migrate.go`, `internal/audit/redact.go`, `internal/auth/walllink.go`, `internal/httpapi/handlers/walllink.go`, `backups.go`, `internal/httpapi/router.go` (auditAdapter), `cmd/holzkube-managerd/main.go` (Verdrahtung), `allowlist_test.go`, `budget_test.go`, `drift_wiring_test.go`, `internal/imagefactory/client.go`, `internal/k8srelease/k8srelease.go`, `internal/config/config.go`, `web/src/routes/settings.tsx`, `web/src/components/NodeHardware.tsx`, `docs/guide.md` (Z. 2404-2414), `docs/api-contract.md` (Z. 1052).
- Messung im Scratchpad (Go 1.27.2, Mac): `url.Parse`-Fehler und `http.Client.Do`-Fehler tragen die URL; `{{.title}}` im Query wird akzeptiert und unverändert gesendet; ein 307 wird mit `ErrUseLastResponse` nicht gefolgt.

### Secondary (MEDIUM confidence)
- [CITED: docs.ntfy.sh/publish] JSON-Veröffentlichung an der Wurzel-URL mit `topic`; Body auf `/topic` ist Text; Template-Parameter `?tpl=yes&t=…&m=…`; `auth`-Query-Parameter; 4096 Bytes. (Dokumentation gelesen, gegen einen echten ntfy nicht probiert.)
- [CITED: docs.slack.dev/messaging/sending-messages-using-incoming-webhooks] `text` Pflicht, URL ist Geheimnis, 4xx-Codes, `invalid_payload` "should not be retried without correction".
- [CITED: gotify.net/docs/pushmsg] Endpunkt `/message`, Header `X-Gotify-Key`, `message` Pflicht, `title`/`priority`/`extras`.

### Tertiary (LOW confidence)
- Alles in der Annahmenliste A3 bis A9.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, keine neuen Pakete; alles Standardbibliothek und Repo.
- Architecture: HIGH für Quellen, Lücken (Hardware-Abgriff, Platten-Schwelle, Host-Schlüssel, Disabled) und Persistenzoptionen, aus dem Code gelesen; MEDIUM für das Zusammenspiel der Haltezeiten mit echtem Verhalten (Reboot-Dauer, Pi-Temperatur) und für Fremdempfänger.
- Pitfalls: HIGH für die gemessenen (URL in Fehlern, Weiterleitung) und die aus dem Code gelesenen (nil nach Neustart, Disabled), MEDIUM für Flattern in der Praxis.

**Research date:** 2026-10-16
**Valid until:** 2026-11-15 (der Code ändert sich mit den Phasen 17, 18 und 20; die Fremddienst-Dokumentation ist stabil, aber ein ntfy-/Slack-Verhalten ist nicht gegen echte Dienste geprüft)
