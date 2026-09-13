# Milestone v1.15 — Abschluss

**Abgeschlossen:** 2026-09-12 (Nachtrag 2026-09-13)
**Status:** alle drei Phasen erfüllt; gebaut, ausgeführt, als Beta ausgeliefert
**Nicht abnahmefähig aus einem Grund, der nicht v1.15 gehört:** OPS-05 🚫

## Was drin ist

| Phase | Requirement | Stand |
|---|---|---|
| 1. Support-Bundle-Export | V2-OPS-01 | 5/5 Kriterien |
| 2. COSI-Watches statt Heartbeat | INV-13, D-19 | 4/4 Kriterien, **Fenster 79 geschlossen** |
| 3. Prometheus-`/metrics` | V2-API-02 | 4/4 Kriterien |

Dazu, außerhalb der Planung und durch den ersten Linter-Lauf ausgelöst:
**Fenster 77 und 89 geschlossen, Fenster 90 gefunden und geschlossen, Fenster 91
eröffnet.**

## Die drei Befunde, die kein Test gefunden hätte

**Der Fingerprint aus dem Maintenance-Modus wurde nie verglichen (Fenster 90).**
Erhoben, angezeigt, bis in `NewMaintenanceClient` weitergereicht, nicht geprüft —
von Phase 8 bis hierher. Auf diesem Pfad gibt es keine Cluster-PKI, also ist der
Fingerprint der einzige Vertrauensanker, und der Aufruf, für den der Pfad
existiert, ist `ApplyConfiguration`. Gefunden hat der Linter nicht das Loch,
sondern sein Fossil: eine ungenutzte `normalise()`, die einen Fingerprint für
einen Vergleich trimmte, den nie jemand geschrieben hatte. Der Kommentar im
Kompositionswurzel behauptete den Pin; der Kommentar in der Naht sagte, es gebe
ihn nicht. Beide standen da, keiner hat den anderen gelesen.

**Ein Watch bemerkt einen verschwundenen Node nicht.** Gemessen, nicht
angenommen: cosi-projects Client baut einen abgerissenen Stream still aus seinem
Bookmark wieder auf und meldet eine Viertelstunde lang nichts. Deshalb ist der
Heartbeat jetzt der Watchdog und nicht nur ein Überbleibsel — ohne ihn kann
dieses Produkt einen stillen Node nicht von einem toten unterscheiden.

**Zwei stille Supervisions-Fehler.** Ein Lesevorgang vor `Start()` legte den
Beobachtungseintrag an, woraufhin `Start()` den Node für überwacht hielt und
nichts startete: eine Instanz, die eine Liste ausgeliefert hatte, bevor sie
startete, überwachte danach gar nichts. Und `Supervise()` nahm den
Request-Context des HTTP-Handlers, also endete der Supervisor eines per API
adoptierten Nodes mit der Antwort.

Alle drei haben gemeinsam, dass sie aussahen wie funktionierender Code. Zwei kamen
vom Ausführen, einer vom Linter.

## Zustand der Verifikation

- **Go-Testsuite: grün**, zum ersten Mal in dieser Umgebung vollständig.
- **`golangci-lint` (2.13.2, go1.26): 0 Befunde.** Jede Ausnahme in
  `.golangci.yml` nennt Regel, Ort und Grund.
- **Frontend:** `tsc --noEmit` sauber, 169 Tests grün, Biome sauber.
- **Binary:** `v1.15.0-beta.1` linux/amd64 gebaut, gestartet, `/metrics` mit
  einem echten Prometheus-Parser gelesen, an den Betreiber übergeben.
- **Container:** gebaut und gelaufen, `/metrics` über TLS aus dem Container
  beantwortet, Datenverzeichnis `drwx------` uid 65532, Zustand überlebt einen
  Neustart.

### Eine Umgebungs-Notiz zum Container-Build

`docker build` scheitert auf diesem Ausführungshost an `go mod download` mit
`x509: certificate signed by unknown authority`: der Agent-Proxy terminiert TLS,
und der Container kennt seine CA nicht. **Das ist kein Fehler im Dockerfile.**
Für die Verifikation wurde eine Kopie des Dockerfiles benutzt, die die CA des
Proxys an `/etc/ssl/certs/ca-certificates.crt` anhängt; das committete Dockerfile
ist unverändert und baut auf einem Host mit direktem Netzzugang ohne das.

## Was offen bleibt, und warum

**OPS-05 🚫 — der Verifikationsdurchlauf auf echter amd64-Hardware.** Der
einzige Release-Blocker, und er gehört v1.14. Fenster 87 ist sein Sammelpunkt;
82, 84 und 85 schließen mit ihm gemeinsam oder gar nicht. **Keine Zeile dieses
Produkts ist je auf einer echten Talos-Maschine gelaufen.** Der Fingerprint-Pin
aus Fenster 90 gehört jetzt in denselben Durchlauf: er ist gegen `talossim`
belegt und gegen echtes Talos im Maintenance-Modus ungeprüft.

**Fenster 91** — der deprecated `ImagePull`. Bewusst nicht getauscht: der
Ersatz ist ein Stream statt eines unären Aufrufs, auf dem einzigen Pfad, der nie
gegen Hardware gelaufen ist. Eine Deprecation-Warnung gegen ein Unbekanntes zu
tauschen ist der falsche Handel.

**Fenster 76, 80, 81** — Tier 1 (Docker) und Tier 2 (QEMU) der Sandbox.
Unverändert: verschachtelte Container können keine sysctls setzen (`runc: unsafe
procfs detected`), und `/dev/kvm` gibt es hier nicht.

**Fenster 83** — die acht auditierten Aktionen ohne Allowlist-Eintrag aus
Phase 6 und 7. Irreparabel: D-16 definiert keinen Löschpfad, und eine Migration
bräche die Hash-Kette.

## Nachtrag, 2026-09-13: was nach dem Abschluss noch kam

Dieser Abschluss wurde geschrieben und danach fünf Commits lang weitergearbeitet.
Der Auslöser war eine einzige Entdeckung: **CI war auf `main` neun Commits lang
rot, und niemand hat hingesehen** — diese Sitzung nicht, die sieben davon selbst
gepusht hat (Fenster 92).

Daraus sind fünf echte Befunde geworden, jeder von einem anderen Mechanismus
gefunden:

| Gefunden durch | Befund |
|---|---|
| CI-Logs lesen | Der Linter lief die ganze Zeit und war rot; Fenster 77 hatte eine falsche Prämisse |
| `-race` plus ein aufgeweitetes Fenster | Der Simulator antwortete an einer Adresse, die der Node aufgegeben hatte — genau der Fehler, gegen den `ip_changes_on_reboot` existiert (Fenster 93) |
| Routen gegen die Oberfläche prüfen | `remove-from-cluster` seit Phase 9 unerreichbar (Fenster 95) |
| Diesen Dialog bauen | Seine **getippte Bestätigung wurde serverseitig nie erzwungen** — der Browser war das einzige Gatter (Fenster 94) |
| Problem-Codes gegen den Vertrag prüfen | Zwei Codes, die ein Client bekommen und nirgends nachschlagen konnte (Fenster 96) |

**Die beiden Prüfungen sind jetzt Tests**, und beide Wächter waren erst falsch,
bevor sie richtig waren — das ist der Teil, der es wert ist, festgehalten zu
werden. Der Routen-Wächter lief grün, während das Support-Bundle gelöscht war,
weil eine Testdatei den href behauptet. Der Tote-Code-Wächter war **völlig
wirkungslos**: jeder Code trägt einen Doku-Kommentar, der mit seinem Namen
beginnt, also erreichte das Textzählen immer zwei. Beides fiel nur auf, weil der
Fehler absichtlich eingebaut und der Test beim Grünbleiben beobachtet wurde.

Eine vierte Prüfung — Job-Arten gegen registrierte Builder — fand nichts: sechs
und sechs. Das ist das Signal, in dieser Richtung aufzuhören.

**Ausgeliefert:** `v1.15.0-beta.2` (linux/amd64) mit all dem darin. Die beta.1
vom Vortag hat den Fingerprint-Pin, das erzwungene Bestätigungs-Gatter und die
drei neuen Oberflächen-Einstiege **nicht** — also genau das, was jemand auf
Blech testen würde.

## Was als Nächstes sinnvoll wäre

Nichts, was ohne Hardware oder ohne eine Entscheidung des Betreibers geht. Der
V2-Rückstau ist bis auf die Punkte leer, die `.planning/MILESTONE-v1.15.md`
ausdrücklich ausschließt, und jeder dieser Gründe steht noch: CA-Rotation
braucht echte Cluster, RBAC braucht einen zweiten Benutzer, PXE braucht eine
Netboot-Umgebung, ARM64 braucht ARM-Hardware, und ein vollständiges CLI ist eine
zweite Oberfläche gegen eine protokollierte Entscheidung.

Der nächste Schritt gehört dem Blech.
