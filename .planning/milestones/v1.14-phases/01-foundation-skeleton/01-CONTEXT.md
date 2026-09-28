# Phase 1: Foundation Skeleton - Context

**Gathered:** 2026-08-27
**Status:** Ready for planning

<domain>
## Phase Boundary

Der Betreiber startet ein einzelnes Binary, meldet sich an, und jede Mutation ist nachvollziehbar und crash-sicher gespeichert — **ohne dass Talos existiert**.

Geliefert wird: Go-Modul (`toolchain go1.26.7` gepinnt), `store` (entity-shaped Interface, `fsstore`, atomare Writes, `flock` + Per-Entity-Mutex + `rev`-CAS, `VERSION` + forward-only Migrationen, Permission-Guard), `audit` (JSONL, täglich rotiert, Hash-Kette, Intent-vor/Outcome-nach), `auth` (argon2id, Session-Rotation beim Login, Sudo-Mode, Rate-Limiting), `httpapi` Middleware-Chain + RFC 9457 `problem+json`, HTTPS-by-default mit Selbsterzeugung, Bind `127.0.0.1`, Vite-Scaffold + `embed.FS`, Taskfile.

**Nicht in dieser Phase:** Jede Talos-Interaktion (Phase 2), `--dry-run` für das Binary (FOUND-12, Phase 2), Inventar (Phase 3).

Requirements: FOUND-01 … FOUND-11.

</domain>

<decisions>
## Implementation Decisions

### Erstlauf & Erreichbarkeit

- **D-01:** Das allererste Betreiber-Konto entsteht über einen **Setup-Wizard in der Web-UI**. Solange kein Konto existiert, leitet jede Route auf `/setup` um; nach Anlage ist `/setup` dauerhaft tot (nicht nur versteckt — der Handler muss aktiv ablehnen). Kein Terminal nötig, passend zum Core Value „ohne `talosctl`". Die Wer-zuerst-kommt-Lücke wird durch den Default-Bind auf `127.0.0.1` entschärft. — **Reversibility:** reversible — betrifft nur zwei Routen und den Startpfad.

- **D-02:** Datenverzeichnis folgt **XDG**: `$XDG_DATA_HOME/holzkube`, Fallback `~/.local/share/holzkube`. Verzeichnis `0700`, Inhalte `0600` (FOUND-10). `--data-dir` / `HOLZKUBE_DATA_DIR` überschreibt — das ist zugleich der Docker-Pfad auf ein gemountetes Volume. Funktioniert auf darwin und linux ohne root. — **Reversibility:** costly — ein späterer Wechsel verlangt einen Migrationspfad für bestehende Installationen samt PKI-Material.

- **D-03:** Konfiguration ausschließlich über **Flags + `HOLZKUBE_*`-ENV, keine Config-Datei**. Präzedenz: Flag > ENV > Default. Kein Parser, kein Suchpfad, kein Schema, nichts zu migrieren; Docker/Compose arbeitet nativ mit ENV. Der effektive Wert jeder Option wird beim Start geloggt. — **Reversibility:** reversible — eine Datei-Ebene lässt sich später unterhalb von ENV einziehen, ohne Bestehendes zu brechen.

- **D-04:** TLS: bei Erstlauf wird ein **langlebiges self-signed Zertifikat** erzeugt (SAN für `127.0.0.1`, `localhost`, Hostname). Der **SHA-256-Fingerprint steht im Startlog**; der Betreiber akzeptiert die Browser-Warnung einmal und vergleicht. Keine eigene CA, kein Trust-Import. Ein bereitgestelltes Zertifikat wird akzeptiert (`--tls-cert` / `--tls-key`). `--insecure-http` nur bei Loopback-Bind (ARCHITECTURE.md:796). — **Reversibility:** reversible.

### Sudo-Mode & Session-Politik

- **D-05:** Sudo-Mode gilt als **Zeitfenster von 5 Minuten** nach erfolgreicher Re-Authentifizierung, und wird bei jeder destruktiven Aktion neu gesetzt. Serien-Operationen (drei Nodes hintereinander resetten) bleiben erträglich, der Schaden einer gestohlenen Session bleibt begrenzt. Ablauf → `428 Precondition Required` (ARCHITECTURE.md:524,820). Das Fenster ist konfigurierbar. — **Reversibility:** reversible.

- **D-06:** Was destruktiv ist, wird **deklarativ am Route-Handler** markiert (z. B. `Destructive: true` in der Route-Registrierung); die Middleware liest die Markierung und gated. Eine einzige Stelle, an der ablesbar ist, was gefährlich ist — eine neue Route ohne Markierung ist ein sichtbares Versehen. **Dieses Muster ist für alle späteren Phasen verbindlich** (Phase 6 Node-Aktionen, Phase 9 etcd). — **Reversibility:** costly — ein Wechsel auf ein anderes Verfahren berührt jede mutierende Route aller Phasen.

- **D-07:** Session-Lebensdauer **24 Stunden**, absolut. *(Der Betreiber wählte bewusst gegen die vorgeschlagenen 30 Tage — kürzere Sessions sind gewünscht.)* Session wird beim Login rotiert (FOUND-02). Server-seitige Sessions über `alexedwards/scs/v2` mit File-Store. — **Reversibility:** reversible — ein Zahlenwert.

- **D-08:** Login-Rate-Limiting ist **reine Verzögerung, niemals eine harte Sperre**: exponentiell wachsende Wartezeit pro Quell-IP, gedeckelt bei ca. 30 Sekunden, plus argon2id mit ≥250 ms Rechenzeit (PITFALLS.md:638). Brute Force wird unmöglich langsam, aber der einzige Operator kann sich nicht aussperren. Es gibt bewusst **keinen Recovery-Pfad**, weil es keinen Zustand gibt, aus dem befreit werden müsste — und damit auch keine Hintertür. — **Reversibility:** reversible.

### UI-Shell & Sprache

- **D-09:** Die Web-UI ist **englisch, ohne i18n-Schicht**. Die gesamte Domäne ist englisch (MachineConfig, Control Plane, etcd Member, Maintenance Mode, Schematic), durchgereichte Talos-API-Fehler ebenso. Das Planungsmaterial (`.planning/`) bleibt deutsch. **Verbindlich für alle späteren Frontend-Phasen.** — **Reversibility:** costly — eine nachträgliche i18n-Einführung berührt jeden String jeder Phase.

- **D-10:** Phase 1 baut **Login + die vollständige dauerhafte App-Shell**: Setup-Wizard, Login, Sidebar mit den späteren Bereichen (Nodes, Clusters, Config, Audit …), Header mit Benutzer und Logout, Toast-System, Fehlerseite, TanStack-Router-Baum. Noch nicht existierende Seiten sind ehrliche „Coming in Phase N"-Platzhalter. Spätere Phasen hängen sich nur noch ein — kein Umbau der Navigation unter Zeitdruck in der Inventar-Phase. — **Reversibility:** reversible.

- **D-11:** Theme ist **dark-first mit Light per Umschalter**. Default folgt `prefers-color-scheme`, die explizite Wahl liegt im `localStorage`. Dichte Statustabellen mit farbigen Health-Indikatoren lesen sich dunkel besser; Tailwind 4 bringt `dark:` ohnehin mit. — **Reversibility:** reversible.

- **D-12:** UI-Bausteine kommen von **shadcn/ui, per CLI in `src/components/ui/` kopiert** (Radix-Primitives + Tailwind), nicht als Abhängigkeit installiert. Kein Versions-Vertrag mit einem Framework, Zugriff auf jede Zeile, Barrierefreiheit und Fokus-Handling von Radix, kleines Bundle — relevant, weil alles ins Binary wandert. Dialog, Command-Palette und dichte Tabellen sind damit für spätere Phasen fertig. — **Reversibility:** costly — ein Wechsel der Komponentenbasis berührt jede gebaute Ansicht.

### Audit-Log — Form & Sichtbarkeit

- **D-13:** Das Audit-Log ist **in Phase 1 bereits in der UI ansehbar**: eine Audit-Seite mit chronologischer Tabelle, Filter nach Datum und Aktion, Detail-Ansicht pro Eintrag. Phase 1 hat sonst keine echten Daten — das Audit-Log ist der einzige Weg, die Kette `store → API → UI` an echten Daten statt an Platzhaltern zu beweisen, und es liefert das erste Tabellen-Muster für alle späteren Phasen. — **Reversibility:** reversible.

- **D-14:** Ein Audit-Eintrag enthält **Akteur, Zeitpunkt, Aktion, Zielobjekt und Eingabeparameter — Secrets redigiert**. Passwörter, Schlüssel und PKI-Material werden über eine **Allowlist** ersetzt (nicht über eine Denylist). Genug, um später zu rekonstruieren, was ein Patch tun sollte; verhindert, dass das Log selbst zum Geheimnisspeicher wird und strenger geschützt werden müsste als alles andere. Kein voller Vorher/Nachher-Diff. — **Reversibility:** one-way — das Feldformat der JSONL-Zeilen ist Teil der Hash-Kette; eine spätere Formatänderung erzwingt entweder einen Kettenbruch an der Nahtstelle oder eine Rückschreib-Migration über den gesamten Bestand.

- **D-15:** Die Hash-Kette wird **automatisch beim Start verifiziert** (aktuelle plus zuletzt rotierte Datei). Ein Bruch erscheint als dauerhaftes Banner in der UI mit Angabe der betroffenen Zeile. Eine Kette ohne Prüfung ist Theater; niemand drückt freiwillig einen Verify-Knopf. — **Reversibility:** reversible.

- **D-16:** Rotierte Audit-Dateien werden **unbegrenzt aufbewahrt, ab dem zweiten Tag gzip-komprimiert**. Bei einem Homelab mit fünf Nodes sind das wenige KB pro Tag. Löschen bräche die Hash-Kette an der Nahtstelle und würde ausgerechnet die Frage „wann fing das an?" unbeantwortbar machen. Keine Retention-Option in v1. — **Reversibility:** reversible.

### Claude's Discretion

Der Betreiber hat bei keiner Frage „Du entscheidest" gewählt — alle sechzehn Entscheidungen sind explizit getroffen. Nicht besprochen und damit im Ermessen von Researcher und Planner:

- Zuschnitt der Store-Entities in Phase 1 (mindestens `Users`, `Sessions`; ob `Settings` als eigene Entity nötig ist)
- Konkrete Fehlertaxonomie hinter RFC 9457 `problem+json` (FOUND-11) — Typ-URIs, Granularität, wie viel Detail an den Client geht
- Port-Default und Verhalten bei falschen Verzeichnisrechten beim Start (FOUND-10 fordert nur „bemängeln")
- Build- und Entwicklungs-Workflow im Detail (Taskfile-Targets, Vite-Proxy im Dev-Modus, Hot-Reload)
- Teststrategie inklusive Crash-Injection auf den Store (ARCHITECTURE.md:965 nennt sie als Phasen-Charakteristikum)
- Navigationsstruktur der Sidebar im Detail; Darstellungsdichte
- Verhalten bei Passwortwechsel (ob alle Sessions invalidiert werden); Darstellung des `428` in der UI (modal vs. inline)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

ROADMAP.md führt für Phase 1 keine `Canonical refs:`-Zeile — die folgende Liste wurde beim Discuss aus PROJECT.md, REQUIREMENTS.md und den Research-Dokumenten zusammengetragen.

### Projekt-Rahmen
- `.planning/PROJECT.md` — Core Value, Constraints (Go-Backend, React+TS+Vite embedded, Betrieb außerhalb des Clusters, Secrets als `0600`-Dateien), Key-Decisions-Tabelle. Out-of-Scope-Liste begrenzt, was Phase 1 nicht vorwegnehmen darf (kein CLI, kein Multi-User/RBAC/OIDC — Auth aber als Interface bauen).
- `.planning/REQUIREMENTS.md` Zeilen 16–26 — FOUND-01 bis FOUND-11 im Wortlaut. FOUND-12 (`--dry-run`) gehört **nicht** hierher, sondern zu Phase 2.
- `.planning/ROADMAP.md` § *Phase 1: Foundation Skeleton* — Goal, fünf Success Criteria, drei parallele Tracks, Research-Flag *skip*.
- `.planning/STATE.md` — akkumulierte Roadmap-Entscheidungen und die offenen Blocker/Concerns (betreffen Phasen 3, 4, 8, 10 — keiner blockiert Phase 1).

### Architektur & Persistenz
- `.planning/research/SUMMARY.md` § *1. Persistence — files win, behind a path-hiding `store` interface* — **Ruling, nicht neu verhandeln.** Alles `0600`-Dateien, kein SQLite in v1. Entity-shaped Interface (`store.Machines().Get(ctx, id)`, **nie** `store.ReadFile(...)`). Jedes `os.ReadFile` außerhalb `internal/store/fsstore/` ist ein Architekturfehler, Lint-Regel wert. Atomare Writes: tmp → `chmod 0600` → `fsync` → `rename` → `fsync(dir)`. Drei Ebenen Nebenläufigkeitskontrolle: Prozess-`flock`, Per-Entity-Mutex, `rev`-CAS → `409 Conflict`. Forward-only Migrationen mit Pre-Migration-Tarball; Start verweigern, wenn `VERSION` neuer als das Binary ist.
- `.planning/research/ARCHITECTURE.md` Zeile 79 — Verantwortungsschnitt des `auth`-Pakets (argon2id-Verifikation, Session-Lebenszyklus, Sudo-Mode-Re-Auth, Login-Rate-Limiting; **kein** Talos, **kein** Plural-Begriff „Users").
- `.planning/research/ARCHITECTURE.md` Zeilen 106–170 — Verzeichnisbaum: `internal/auth/`, `internal/store/`, `internal/talossim/` (in `internal/`, nicht `test/`), `cmd/holzkube-dev/`.
- `.planning/research/ARCHITECTURE.md` Zeile 524 — Middleware-Chain und die Stelle, an der das Sudo-Mode-Gate `428` wirft.
- `.planning/research/ARCHITECTURE.md` Zeile 796 — HTTPS-by-default-Begründung, self-signed bei Erstlauf, bereitgestelltes Zertifikat akzeptieren, `--insecure-http` nur bei Loopback-Bind; HTTP/2 löst nebenbei das SSE-Verbindungslimit.
- `.planning/research/ARCHITECTURE.md` Zeile 820 — Sudo-Mode-Design: destruktive Aktionen bei Session älter als N Minuten → `428`; das einzige Mittel gegen einen gestohlenen Session-Cookie, ca. 40 Zeilen.
- `.planning/research/ARCHITECTURE.md` Zeile 965 — Phasen-Tabelle Zeile „0. Skeleton": vollständige Deliverable-Liste, drei Parallel-Tracks, Charakteristik „**Zero Talos.** Pure unit tests, including crash-injection on the store."

### Stack & Bibliotheken
- `.planning/research/SUMMARY.md` § *Recommended Stack* — Go 1.26.7 (**min 1.26.6**, Dev-Box hat 1.26.4; `toolchain go1.26.7` in `go.mod` pinnen, sonst schlägt CI verwirrend fehl). stdlib `net/http` mit Go-1.22-Method+Wildcard-Patterns deckt das Routing vollständig, `log/slog` für Logging. Auth-Stack: `alexedwards/argon2id` v1.0.0, `alexedwards/scs/v2` v2.9.0 (server-seitige Sessions, File-Store), `justinas/nosurf` v1.2.0. Frontend: React 19.2.8, TypeScript 7.0.2, Vite 8.2.2, Tailwind 4.3.3 über `@tailwindcss/vite` (**nicht** PostCSS), TanStack Router 1.170.32 + Query 5.102.6 (es gibt **kein** v6), zod 4.4.3 an der API-Grenze. Embedding via `//go:embed all:dist` — **das `all:`-Präfix ist zwingend**, sonst überspringt der Embed Vites `_`- und `.`-Dateien still.
- `.planning/research/SUMMARY.md` § *Recommended Stack* → *Module weight rule* — `cmd/holzkubed` hängt **ausschließlich** an `pkg/machinery`. Der Sandbox-Teil (Docker/QEMU-Provisioner, Root-Modul) kommt in ein **eigenes Go-Modul unter `sandbox/`**. Wird das falsch aufgesetzt, entsteht ein ~200-MB-Binary mit ungewollter Supply-Chain-Fläche, schmerzhaft rückgängig zu machen. **Betrifft Phase 1**, weil hier die Modulstruktur entsteht.
- `.planning/research/STACK.md` — Tooling: Taskfile v3 (`build:go` hängt an `build:web`), goreleaser v2.18.0 (**verlangt einen reinen Go-Dependency-Baum**), golangci-lint v2.13.1 (**v2-Config-Schema**, `gosec` aktivieren — dieses Werkzeug hält Cluster-PKI), Biome 2.5.10.

### Sicherheit
- `.planning/research/PITFALLS.md` Zeile 497 und 633 — Bind auf `0.0.0.0` als Default ist ein Anti-Pattern: ein Homelab-Werkzeug an allen Interfaces in einem flachen Netz ist von jedem IoT-Gerät erreichbar. Default `127.0.0.1`; Exponierung ist eine explizite, dokumentierte Entscheidung.
- `.planning/research/PITFALLS.md` Zeile 638 — schwaches oder fehlendes Login-Rate-Limiting ermöglicht Brute Force in Offline-Qualität gegen argon2id. Rate Limit **plus** argon2id-Parameter auf ≥250 ms getunt. *(Der „lockout"-Teil dieser Zeile ist durch D-08 bewusst überstimmt — reine Verzögerung statt Sperre, damit der einzige Operator sich nicht aussperren kann.)*
- `.planning/research/PITFALLS.md` § *P16* — Sicherheitsfundamente, die Phase 1 abräumt.

### Was Phase 1 für spätere Phasen festlegt
- D-06 (deklarative Destruktiv-Markierung) ist das verbindliche Muster für Phase 6 (Node-Aktionen: Reboot, Shutdown, Reset) und Phase 9 (etcd-Member entfernen).
- D-09 (UI englisch, keine i18n) ist verbindlich für jede Frontend-Phase.
- D-10/D-12 (App-Shell + shadcn/ui) sind die Grundlage, in die sich Phase 3, 5, 7, 8 und 9 einhängen, ohne Navigation oder Komponentenbasis neu zu bauen.
- D-14 (Audit-Feldformat) ist Teil der Hash-Kette — jede spätere Phase, die auditiert, schreibt in dieses Format.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

Keine. Das Repository enthält ausschließlich `README.md` (10 Bytes) und `.planning/`. Kein Go-Modul, kein `package.json`, kein Quellcode. Phase 1 ist vollständig greenfield.

### Established Patterns

Keine im Code — alle Muster dieser Phase werden **hier erstmalig etabliert** und binden spätere Phasen:

- **Entity-shaped Store-Zugriff** — `store.Machines().Get(ctx, id)`. Jedes `os.ReadFile` oberhalb von `internal/store/fsstore/` ist ein Architekturfehler; eine Lint-Regel ist dafür vorgesehen (SUMMARY.md § 1).
- **Middleware-Chain mit deklarativem Sudo-Gate** (D-06) — bindet jede mutierende Route aller Folgephasen.
- **RFC 9457 `problem+json` als einzige Fehlerform** (FOUND-11) — jede spätere API-Antwort folgt dieser Taxonomie.
- **Audit als Intent-vor-/Outcome-nach-Paar** (FOUND-06, D-14) — jede spätere Mutation schreibt in dieses Format.
- **App-Shell mit Platzhalter-Routen** (D-10) — spätere Phasen ersetzen Platzhalter, statt Navigation zu bauen.

### Integration Points

Keine bestehenden. Phase 1 schafft die Nähte, an denen Phase 2 andockt:

- `internal/store/` — Phase 3 legt hier die Cluster- und Node-Entities ab (UUID-adressiert, `cluster_id` nullbar, `schematic_id` am Record).
- `internal/audit/` — Phase 6 (Jobs-Engine) und Phase 9 (Upgrades) schreiben jede Mutation hierher.
- `internal/httpapi/` Middleware-Chain — Phase 2 hängt hier die Talos-Routen ein, inklusive des `--dry-run`-Gates (FOUND-12).
- `cmd/holzkubed` — Modulgrenze: hängt später ausschließlich an `pkg/machinery`, **nie** am Root-Modul mit den Provisionern.

**Keine `.planning/codebase/`-Maps vorhanden** — es gibt noch nichts zu mappen.

</code_context>

<specifics>
## Specific Ideas

- **„Ohne Terminal" ist der Maßstab, auch im Fundament.** Die Wahl des Setup-Wizards gegen Bootstrap-Token und CLI-Subcommand (D-01) folgt demselben Prinzip wie der Core Value: Wenn ein Weg einen Terminal-Blick erzwingt, ist er der falsche Weg. Das gilt in Phase 1 sogar dort, wo die CLI-Variante technisch am einfachsten wäre.

- **Kürzere Sessions als vorgeschlagen.** Der Betreiber wählte 24 Stunden gegen die empfohlenen 30 Tage (D-07). Downstream nicht auf einen langlebigen Cookie hin optimieren — die UI muss einen Session-Ablauf sauber und ohne Datenverlust behandeln, weil er täglich vorkommt.

- **Kein Recovery-Pfad ist Absicht, kein Versäumnis.** Bei D-08 wurde die harte Sperre bewusst verworfen, weil jeder Entsperr-Mechanismus selbst zur Angriffsfläche wird und im Notfall Shell-Zugriff voraussetzt — genau das, was dann fehlt. Ein späterer Agent darf keine „Unlock"-Funktion nachrüsten in dem Glauben, hier fehle etwas.

- **Das Audit-Log ist in Phase 1 der einzige echte Datenfluss.** Es dient ausdrücklich als Beweis, dass `store → API → UI` funktioniert, und liefert zugleich das Tabellen-Muster für Phase 3 und danach (D-13). Es nicht als Nebensache behandeln.

- **Allowlist, nicht Denylist, für die Redigierung** (D-14). Eine Denylist vergisst das nächste Geheimnis; die Allowlist zwingt jeden neuen Parameter zu einer bewussten Entscheidung.

- **Kettenprüfung beim Start, nicht auf Knopfdruck.** Begründung wörtlich aus der Diskussion: eine Hash-Kette, die niemand prüft, ist Theater (D-15).

</specifics>

<deferred>
## Deferred Ideas

Keine — die Diskussion blieb durchgehend innerhalb der Phasengrenze. Es kam kein Vorschlag auf, der eine neue Fähigkeit eingeführt hätte.

Zwei Punkte wurden bewusst **nicht** als Deferred markiert, sondern als endgültig entschieden, damit sie nicht später als offene Frage wieder auftauchen:

- **Retention-Option für das Audit-Log** — verworfen (D-16). Nicht „später", sondern nicht.
- **Konto-Entsperrung nach Rate-Limit** — verworfen (D-08). Es gibt keinen Sperrzustand, also nichts zu entsperren.

</deferred>

---

*Phase: 1-foundation-skeleton*
*Context gathered: 2026-08-27*
