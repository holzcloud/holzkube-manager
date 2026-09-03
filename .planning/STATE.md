---
gsd_state_version: 1.0
milestone: v1.14
current_phase: 02
current_phase_name: Transport Seam, `talossim` & Image Factory
status: executing
stopped_at: Completed 02-21-PLAN.md
last_updated: "2026-08-30T10:08:41.928Z"
last_activity: 2026-08-30
last_activity_desc: Phase 02 execution started
state_head: 162fe0fd8478e61d6e2f620b0ff7fb8c9702124c
progress:
  total_phases: 10
  completed_phases: 1
  total_plans: 27
  completed_plans: 27
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-08-27)

**Core value:** Eine neue Maschine wird komplett in der UI zum Cluster-Node — ohne `talosctl`, ohne Omni.
**Current focus:** Phase 02 — Transport Seam, `talossim` & Image Factory

## Current Position

Phase: 02 (Transport Seam, `talossim` & Image Factory) — EXECUTING
Plan: 9 of 21
Status: Ready to execute
Last activity: 2026-08-30 — Phase 02 execution started

Progress: [░░░░░░░░░░] 0%

## Performance Metrics

**Velocity:**

- Total plans completed: 6
- Average duration: —
- Total execution time: 0.0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01 | 6 | - | - |

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

## Deferred Items

Items acknowledged and deferred at milestone close, most recent first:

| Category | Item | Status | Deferred At | Milestone |
|----------|------|--------|-------------|-----------|
| *(none)* | | | | |

## Session Continuity

Last session: 2026-08-30T10:08:31.162Z
Stopped at: Completed 02-21-PLAN.md
Resume file: None
