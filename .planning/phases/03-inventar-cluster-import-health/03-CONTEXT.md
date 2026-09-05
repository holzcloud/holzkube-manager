# Phase 3: Inventar, Cluster-Import & Health - Context

**Gathered:** 2026-09-05
**Status:** Ready for planning

<autonomy_note>
## Wie dieser Kontext entstanden ist

Der Betreiber hat die Diskussion ausdrücklich delegiert: *„entscheide du alles
selber. ich will keine fragen beantworten."* Jede Entscheidung unten ist
deshalb **Claude's Discretion** — getroffen aus PROJECT.md, ROADMAP.md,
REQUIREMENTS.md, den beiden vorherigen CONTEXT.md, den vier Research-Dokumenten
und dem gelesenen Code, nicht aus einer Betreiber-Antwort.

Das ändert, was dieser Abschnitt leisten muss: **jede Entscheidung trägt ihre
Begründung, damit sie widersprochen werden kann, statt später entdeckt zu
werden.** Eine Entscheidung mit `one-way`-Rating ist eine, bei der ein
Widerspruch *vor* der Ausführung billiger ist als danach — `gsd-planner` trägt
sie als `checkpoint:decision` vor die implementierende Task.

Der vollständige Frage-für-Frage-Verlauf steht in `03-DISCUSSION-LOG.md`.
</autonomy_note>

<domain>
## Phase Boundary

Der bestehende Homelab-Cluster wird importiert, und das Inventar bleibt
ehrlich — auch wenn ein Node oder der ganze Cluster nicht antwortet.

14 Requirements: TRANS-08, INV-01…INV-13. Davon **fünf Release-Blocker**:
INV-01 🚫, INV-03 🚫, INV-04 🚫, INV-05 🚫, INV-07 🚫.

Zwei Tracks, echt parallel, sobald die `Field[T]`-Response-Shape festgezurrt
ist (D-11 tut genau das):

- **(a) Backend** — Import-Pfad, Store-Entities, Health-Supervisors, Read-Model,
  cluster-bezogene Leserouten, Sperr-Middleware, talossim-Ausbau
- **(b) Frontend** — `/`, `/nodes`, `/nodes/$uuid`, `/clusters`

**Nicht diese Phase:** kein Streaming und keine SSE-Route (Phase 5, mit
dokumentiertem Entry-Blocker — siehe D-15); keine Jobs-Engine und keine
Node-Aktionen (Phase 6); kein Ansehen, Diffen oder Anwenden von MachineConfig
(Phase 7) — Phase 3 *liest* die Config genau einmal, im Import, und
ausschließlich um das Secrets-Bundle abzuleiten; kein Provisioning und keine
Maintenance-Mode-Discovery (Phase 8); keine Upgrades und keine
etcd-Verwaltung (Phase 9); keine Zertifikatserneuerung (Phase 10, siehe
`<deferred>`).

</domain>

<decisions>
## Implementation Decisions

### Cluster-Import: was der Betreiber liefert und was holzkube ableitet

- **D-01:** Der Import nimmt **ein `talosconfig` plus die Adresse eines
  Control-Plane-Nodes** entgegen. Das Secrets-Bundle wird **nicht vom Betreiber
  geliefert, sondern von diesem Node gelesen**: die vollständige MachineConfig
  eines Control-Plane-Nodes enthält `.machine.ca` samt privatem Schlüssel,
  `.cluster.secret`, `.cluster.token`, `.machine.token` und
  `cluster.serviceAccount.key` — genau die Felder, die PITFALLS als das Leck der
  „View MachineConfig"-Funktion benennt. Der Betreiber *hat* ein `talosconfig`,
  weil er heute `talosctl` benutzt; eine `secrets.yaml` hat er unter Umständen
  nie gesehen, weil sie beim ursprünglichen `talosctl gen config` entstand und
  seither irgendwo liegt. Einen Import zu bauen, der ein Artefakt verlangt, das
  der einzige Zielnutzer möglicherweise verloren hat, würde den Kernpfad genau
  für den vorgesehenen Cluster unbenutzbar machen. — **Reversibility:** costly —
  die Ableitungsrichtung bestimmt die Form der Import-Route, des Wizards und des
  Cluster-Records; sie umzudrehen heißt, alle drei neu zu schneiden.

- **D-02:** Das persistierte Secrets-Bundle ist eine **harte Vorbedingung der
  Adoption**: schlägt die Ableitung aus D-01 fehl, ist der Cluster **nicht
  importiert** — es gibt keinen halb adoptierten Zustand und keinen
  „später nachreichen"-Pfad. Grund ist P1/P5/P16: das
  talosconfig-Client-Zertifikat läuft nach rund zwölf Monaten ab, Talos rotiert
  Client-Zertifikate **nicht**, und alle Nodes gehen in derselben Sekunde rot.
  Ohne Bundle gibt es **keinen Weg zurück in den Cluster hinein**. Ein Cluster,
  den holzkube nur beobachten, aber nie wieder betreten kann, ist genau das
  Versprechen, das das Werkzeug nicht halten darf. — **Reversibility:** one-way —
  die Zusicherung „jeder adoptierte Cluster hat sein Bundle" ist die Prämisse,
  auf der Phase 7 (Config-Generierung), Phase 8 (Provisioning) und Phase 9
  (Upgrades) je einen Release-Blocker tragen; sie nachträglich aufzuweichen
  hieße, in jeder dieser Phasen einen zweiten, unerprobten Codepfad für
  bundle-lose Cluster zu bauen.

- **D-03:** Vor dem ersten vertrauenden Zugriff zeigt holzkube den
  **SHA-256-Fingerprint des Server-Zertifikats** des genannten Control-Plane-
  Nodes, und der Betreiber bestätigt ihn. Format und Erzeugung kommen von
  `internal/tlsx.Fingerprint` — dieselbe Doppelpunkt-getrennte Großhex-Form, die
  D-04 aus Phase 1 schon im Startlog druckt und die `talos.Creds.Fingerprint`
  erwartet. Zwei Fingerprint-Formate im selben Produkt wären ein Vergleich, den
  niemand durchführt. — **Reversibility:** reversible.

- **D-04:** Der Import endet mit einem **Konnektivitätsbeweis unter einem frisch
  aus dem abgeleiteten Bundle ausgestellten Client-Zertifikat** — nicht unter dem
  eingelieferten `talosconfig`. Erst dieser Beweis schaltet den Cluster scharf.
  Das ist der Unterschied zwischen „die Datei sah gut aus" und „holzkube kann
  sich selbst Zutritt verschaffen": ein `talosconfig`, das heute funktioniert,
  beweist nichts über das Bundle, aus dem morgen ein neues Zertifikat kommen
  muss. Der Beweis ist ein `Version`-Aufruf unter der Probe-Deadline — dieselbe
  Liveness-Prüfung, die D-05 aus Phase 2 begründet, weil sie den vollständigen
  TLS-Handshake und damit den Zertifikatspfad ausübt. — **Reversibility:**
  reversible.

- **D-05:** Zeigt der genannte Node kein Control-Plane-Material (`.machine.ca`
  ohne privaten Schlüssel — der Normalfall auf einem Worker), wird der Import
  **benannt abgelehnt**, mit der Aufforderung, einen Control-Plane-Node zu
  nennen. Kein stiller Import ohne Bundle, kein Weiterprobieren an anderen
  Adressen. Dafür wird ein eigener RFC-9457-Code geprägt; welcher, entscheidet
  der Planner in `docs/api-contract.md` im selben Commit (die Taxonomie ist
  ausdrücklich *closed*, siehe `<code_context>`). — **Reversibility:**
  reversible.

- **D-06:** `talosconfig` und Fingerprint kommen ausschließlich per
  **Datei-Upload oder Einfügen in ein Textfeld** herein — **kein Pfad auf dem
  Server**. Ein serverseitiger Pfad wäre ein beliebiges Dateilesen unter der uid
  von `holzkubed` und verstößt zugleich gegen die Regel in
  `internal/store/store.go:5-7`, dass jedes `os.ReadFile` außerhalb von
  `internal/store/fsstore` ein Architekturfehler ist. Upload und Einfügen sind
  serverseitig derselbe Request-Body. — **Reversibility:** reversible.

- **D-07:** **`gen secrets` ist ausschließlich aus dem Create-Pfad erreichbar**,
  und das wird durch einen Test gehalten, nicht durch Disziplin: der Test weist
  nach, dass der Import-Pfad die Generierungsfunktion nicht aufrufen kann. Es
  gibt **keinen** „generiere, falls fehlt"-Fallback — er würde im
  Fehlerfall eines Imports ein zweites, fremdes CA-Material erzeugen und damit
  einen Cluster erschaffen, der aussieht wie der bestehende und keinen einzigen
  seiner Nodes betreten kann. Import und Create erzeugen denselben
  Record-Typ, unterschieden durch ein `origin`-Feld. — **Reversibility:**
  costly — der Test ist der Nachweis für INV-02; ihn zu entfernen heißt, die
  Trennung nur noch zu behaupten.

### Inventar-Identität und Discovery

- **D-08:** Nach dem Import füllt sich das Inventar **automatisch aus der
  Cluster-Mitgliedschaft**, gelesen vom importierten Control-Plane-Node; die
  **manuelle Adresseingabe bleibt der immer verfügbare Zweitweg**
  (`talos.NewManualSource` existiert bereits). Welche COSI-Ressource die
  Mitgliederliste trägt, wenn der Talos-Discovery-Service abgeschaltet ist, ist
  eine Research-Frage (siehe `<open_questions>`); die *Politik* ist hier
  entschieden. Ein Betreiber, der nach dem Import fünf IP-Adressen abtippen
  muss, hat den Cluster nicht importiert, sondern nur ein Zertifikat
  hinterlegt. — **Reversibility:** reversible.

- **D-09:** **Kein Subnetz-Scan in dieser Phase.** Der Scan ist eine eigene
  `DiscoverySource` und gehört zu Phase 8, wo Maschinen im Maintenance-Mode
  gefunden werden müssen, die per Definition in keinem Cluster stehen. Ihn hier
  zu bauen wäre eine neue Fähigkeit, kein Klarstellen einer bestehenden. Ein
  umgezogener Node wird stattdessen über den lebenden Cluster wiedergefunden
  (D-10); ist der ganze Cluster tot, bleibt die manuelle Eingabe. —
  **Reversibility:** reversible.

- **D-10:** **Die UUID gewinnt immer gegen die Adresse.** Ein Node behält seinen
  Record über IP-Wechsel, Reboot und Unerreichbarkeit hinweg; Schlüssel ist
  `hardware.SystemInformation.UUID`. Antwortet unter einer bekannten Adresse
  eine **fremde** UUID, entsteht ein **neuer** Record, und der alte wird als
  „nicht mehr unter dieser Adresse" markiert — der alte wird **nie**
  überschrieben. `talos.Target` dokumentiert `Addr` bereits als „a hint and
  never identity"; diese Entscheidung ist die Ausformulierung davon auf
  Inventar-Ebene. Die Gegenrichtung — Adresse gewinnt — würde bei einer
  DHCP-Rotation zwischen zwei Nodes die Historie zweier Maschinen vertauschen,
  und zwar still. — **Reversibility:** costly — der Schlüssel ist der Dateiname
  im Store; eine spätere Änderung ist eine Migration des gesamten Inventars.

- **D-11:** Ein Inventar-Record wird **nur auf ausdrückliche Betreiber-Aktion
  gelöscht**, und diese Route ist `Destructive: true` (D-06 aus Phase 1), also
  hinter dem Sudo-Fenster. Kein TTL, kein automatisches Aufräumen, kein
  „unerreichbar seit N Tagen → weg". Das ist INV-09 wörtlich, und es ist die
  Eigenschaft, die das Inventar im Störfall überhaupt erst wertvoll macht. —
  **Reversibility:** reversible.

- **D-12:** Die `schematic_id` (INV-04 🚫) wird **vom Node gelesen** — sie ist
  laut Research über eine virtuelle System-Extension an jedem laufenden Node
  abrufbar — und bleibt **ehrlich leer**, wenn der Node nicht aus einem
  Factory-Image stammt. Sie wird **nie geraten und nie aus der Talos-Version
  abgeleitet**. Phase 9 liefert Upgrades auf Basis dieses Feldes aus; ein
  geratener Wert dort bedeutet einen Node, dem beim Upgrade seine Extensions
  gelöscht werden, mit Erfolgsmeldung (P9/P10 — die „silent-divergence"-Familie).
  Ein leeres Feld ist `Field[T]` mit `Available: false` und einem
  `unavailable_reason`, der sagt, warum. — **Reversibility:** reversible.

### Read-Model: `Field[T]`, Ehrlichkeit und Watches

- **D-13:** Die Response-Shape ist **exakt `Field[T]` aus ARCHITECTURE.md
  Pattern 6**, unverändert übernommen: `{value (omitzero), level, available,
  stale_since, unavailable_reason}`, mit `HealthLevel` als Go-Typ und den vier
  Stufen `LevelNone / LevelNode / LevelEtcd / LevelK8s`. **Auf der Leitung ist
  `level` ein String** (`"none" | "node" | "etcd" | "k8s"`), nicht die
  `iota`-Zahl: die Reihenfolge der Konstanten ist ein Implementierungsdetail,
  und eine Zahl im JSON würde sie zu einem veröffentlichten Vertrag machen.
  **Mit dieser Entscheidung ist der Parallelitäts-Constraint der ROADMAP
  eingelöst** — Backend und Frontend können ab hier getrennt laufen. —
  **Reversibility:** one-way — die Shape steht auf jedem Feld jeder
  Inventar-Antwort; sie zu ändern ist eine Vertragsänderung, die jeden
  gebauten Bildschirm und jeden Test der Phasen 3, 5, 6, 7 und 9 berührt.

- **D-14:** Drei Zustände sind **unterscheidbar, und alle drei sind sichtbar**:

  | `available` | `stale_since` | Bedeutung | Darstellung |
  |---|---|---|---|
  | `true` | `nil` | aktuell bestätigt | normaler Wert |
  | `false` | gesetzt | bekannt, aber alt | letzter Wert gedämpft + „as of 4m ago" |
  | `false` | `nil` | nie gelesen | `—` plus `unavailable_reason` |

  `available` heißt „dieser Wert ist jetzt bestätigt", nicht „dieser Wert
  existiert". Ein veralteter Wert behält seinen `value` — Weglassen wäre in der
  UI nicht von „null" zu unterscheiden, und genau so entsteht das leere
  Dashboard, das INV-08 verbietet. Der prüfbare Satz aus Pattern 6 gilt
  wörtlich als Test: talossim mit `etcd_down` **und** `k8s_down` starten,
  Node-Detail abrufen, und **jedes** `LevelNode`-Feld hat weiterhin
  `available: true`. — **Reversibility:** one-way — dieselbe Vertragsfläche wie
  D-13.

- **D-15:** **`stale_since` wird gesetzt, sobald die Quelle nicht mehr bestätigt
  ist — es gibt keine Karenzzeit im Backend.** Das Alter rechnet der Client aus
  `stale_since`. Ein serverseitiger Schwellwert wäre eine zweite Wahrheit neben
  dem Zeitstempel, und die beiden würden auseinanderlaufen. Die Oberfläche
  markiert ab dem ersten Moment sichtbar, aber gedämpft; **Alarmfarbe ist dem
  Verbindungszustand vorbehalten**, nicht dem Alter eines Wertes. —
  **Reversibility:** reversible.

- **D-16:** Der **letzte bekannte Snapshot wird persistiert**, am Machine-Record
  im Store, und beim Start mit gesetztem `stale_since` geladen, bis ein
  Supervisor ihn bestätigt. Nur im Speicher gehalten würde ein Neustart von
  `holzkubed` eine leere Seite zeigen — und dieser Neustart passiert
  wahrscheinlich *im* Störfall, also genau dann, wenn INV-08 gilt. —
  **Reversibility:** costly — der Snapshot wird Teil des Store-Schemas, eine
  spätere Umformung braucht eine Migration.

- **D-17:** **Supervisors laufen eager**: einer pro Node ab dem Import,
  unabhängig davon, ob jemand hinschaut, und **je Level getrennt**, damit ein
  etcd-Ausfall keinen Node-Level-Read anhält. Heartbeat 30–60 s mit Jitter
  (INV-13). ARCHITECTURE Pattern 7 sagt für 5–20 Nodes ausdrücklich: „Do not
  build a scheduler for this." Lazy zu beobachten hieße, das Dashboard genau
  dann langsam zu machen, wenn es gebraucht wird, und `stale_since` beim ersten
  Öffnen bedeutungslos zu halten. Degradation folgt dem Zustandsautomaten aus
  Pattern 7 (`unknown → connecting → watching → degraded → down`), der bei
  `down` den letzten Snapshot samt `stale_since` behält. — **Reversibility:**
  reversible.

- **D-18:** **Phase 3 baut keine SSE-Route.** Die API liefert
  **Snapshot-Routen**, der Client pollt sie in ruhigem Intervall. Grund ist der
  in ROADMAP § Phase 5 und in `02-CONTEXT.md` § `<deadline_policy>` verzeichnete
  **Entry-Blocker**: keiner der drei `ResponseWriter`-Wrapper implementiert
  `Flush`/`Unwrap`/`Hijack`, und `WriteTimeout = 60 s` ist prozessweit — eine
  streamende Route auf dieser Kette puffert lautlos und stirbt nach 60 s. Phase
  2 hat bewusst keine gebaut, damit die Kollision nicht zuschlägt; Phase 3 zieht
  sie nicht vor. Weil die Response-Shape (D-13) dieselbe bleibt, ist der Tausch
  gegen SSE in Phase 5 ein **client-lokaler** Wechsel. — **Reversibility:**
  reversible.

- **D-19:** Node-Fakten kommen **zuerst aus COSI-Watches**, unary RPCs nur dort,
  wo es keine Ressource gibt. INV-13 schließt blindes Polling aus, und ein
  `Memory()`-Aufruf im Sekundentakt wäre genau das. Statische Fakten
  (CPU-Modell, RAM-Größe, Disks, Interfaces) kommen einmal pro Watch-Bootstrap.
  **Konsequenz, die die Planung tragen muss:** `internal/talos.ClusterClient`
  **und** `internal/talossim` müssen in dieser Phase **synchron wachsen** —
  heute kennt der Client weder `Memory` noch `CPUInfo` noch
  `NetworkDeviceStats`, und talossim seedet nur `hardware.SystemInformation`,
  `network.NodeAddress` und `k8s.Nodename`. Jedes für INV-06 gelesene Feld
  braucht beide Hälften, sonst ist es nicht testbar. — **Reversibility:**
  reversible.

- **D-20:** Die **Kubernetes-Version wird node-seitig gelesen, niemals über
  `:6443`.** Die Naht ist ausdrücklich: `:50000` ist holzkube, `:6443` ist
  k9s/Lens/Headlamp. Ein Kubernetes-Client wäre eine neue Abhängigkeit und ein
  neues Ausfallrisiko genau im Störfall. Ist eine Angabe ausschließlich über
  `:6443` zu bekommen, ist sie ein `LevelK8s`-Feld mit `available: false` — und
  **kein** Grund, einen K8s-Client einzuführen. — **Reversibility:** costly —
  ein später eingeführter K8s-Client wäre eine Architekturänderung, die
  PROJECT.md § Out of Scope widerspricht.

### Sperre, Zertifikatsablauf, Kompatibilitätsmatrix

- **D-21:** Ein **importierter** Cluster ist **standardmäßig read-only
  gesperrt**; ein **angelegter** nicht. P17 verlangt genau das („ship a
  per-cluster read-only mutation lock so the homelab is adopted read-only
  first"): ein importierter Cluster ist per Definition einer, von dem der
  Betreiber bereits abhängt, während ein angelegter vor einer Sekunde noch nicht
  existierte. — **Reversibility:** reversible.

- **D-22:** Die Sperre wirkt **serverseitig, deklarativ an der Route** — dasselbe
  Muster wie `Destructive` aus D-06 (Phase 1), von Middleware gelesen, nichts
  matcht auf URLs. Das Entsperren ist selbst eine `Destructive: true`-Route,
  also Sudo-Fenster plus Audit-Eintrag. Eine Sperre, die nur die Oberfläche
  kennt, ist keine — dasselbe Argument, mit dem D-03 aus Phase 2 den Dry-Run in
  den Transport gelegt hat. Phase 3 hat selbst kaum mutierende cluster-bezogene
  Routen; der Mechanismus entsteht trotzdem hier, weil INV-12 ihn fordert und
  weil die Phasen 6, 7 und 9 Dutzende hinzufügen. — **Reversibility:** costly —
  ein späterer Wechsel des Verfahrens berührt jede mutierende Route aller
  folgenden Phasen.

- **D-23:** Der Ablauf des talosconfig-Client-Zertifikats warnt bei
  **90 / 30 / 7 Tagen**, gestaffelt: bis 90 Tage ein Badge in der
  Cluster-Übersicht, **ab 30 Tagen ein dauerhaftes Banner auf jeder Seite**,
  ab 7 Tagen dringlich. Research nennt 90/30; die 7 kommen dazu, weil der Ausfall
  total und gleichzeitig ist. **Ein abgelaufenes Zertifikat ist ein eigener,
  benannter Zustand** — `x509: certificate has expired` wird ausdrücklich **von
  „Node down" unterschieden** und als **Cluster-Banner** gezeigt, nicht als
  fünf rote Nodes. Andernfalls schließt der Betreiber, sein Cluster sei
  gestorben. — **Reversibility:** reversible.

- **D-24:** Die Talos↔Kubernetes-Kompatibilitätsmatrix ist eine **kuratierte, im
  Binary eingebettete Tabelle**, reviewbar im Git und mit dem Release
  ausgeliefert — dieselbe Bauform und dieselbe Begründung wie D-08 aus Phase 2
  für die bekannt kaputten Versionen: es gibt keine maschinenlesbare
  Upstream-Quelle, und eine Abfrage über das Netz wäre ausgerechnet im Störfall
  nicht verfügbar. „Abstand zum Rand" ist die Zahl der Minor-Versionen bis zum
  Rand des für die laufende Talos-Version unterstützten K8s-Fensters, gezeigt
  **als Zahl und als Satz**. — **Reversibility:** reversible.

- **D-25:** Die Matrix bleibt **getrennt von `MinSupportedVersion` /
  `MaxSupportedVersion`** in `internal/talos/version.go`. Das sind zwei
  verschiedene Aussagen: die Konstanten sagen, welche Talos-Versionen *holzkube*
  getestet hat; die Matrix sagt, welche Kubernetes-Version *Talos* unterstützt.
  In eine Konstante gefaltet wären beide unverständlich und keine für sich
  pflegbar. — **Reversibility:** reversible.

### Contract-Suite und der Tier-1-Provisioner (TRANS-08)

- **D-26:** Die Contract-Suite wird **backend-parametrisiert geschrieben, bevor
  feststeht, welches zweite Backend sie bekommt**: ein Parameter
  (`talossim` | `docker`), derselbe Testkörper. Damit ist der offene
  Research-Flag kein Blocker für den Beginn der Arbeit. — **Reversibility:**
  reversible.

- **D-27:** **Scheitert der Tier-1-Docker-Provisioner auf `darwin/arm64`,
  kollabiert die Suite nicht auf Tier 0.** Die reale Hälfte läuft dann **in CI
  auf `linux/amd64`** und wird lokal **sichtbar übersprungen**, mit Eintrag in
  `.planning/WINDOWS.md`. Ein übersprungener Lauf, der sich als übersprungen
  meldet, ist ehrlicher als eine Suite, die so tut, als gäbe es kein zweites
  Backend — und Fake-Drift ist laut Research (P17) das Risiko, das jede spätere
  Phase trägt. Success Criterion 5 gilt erst als erfüllt, wenn die Suite
  **irgendwo** grün gegen echtes Talos läuft. — **Reversibility:** reversible.

- **D-28:** Der Provisioner-Code lebt in **`sandbox/` als eigenes Modul**. Das
  ist keine neue Entscheidung, sondern D-07 aus Phase 2 an seinem ersten echten
  Nutzer: `pkg/provision` liegt im schweren Talos-Root-Modul, und
  `internal/depguard_test.go` prüft, dass dieses Modul nie in `go list -m all`
  auftaucht. Es steht hier, weil Phase 3 die erste Phase ist, in der jemand in
  Versuchung gerät, es anders zu machen. — **Reversibility:** costly — ein
  späterer Umzug über die Modulgrenze zieht die Test-Imports mit.

### Zuschnitt der Oberfläche

- **D-29:** Die drei Flächen teilen sich so auf: **`/` wird die
  Flotten-Übersicht** (Cluster-Kacheln, Node-Zusammenfassung, die Warnbanner aus
  D-23), **`/nodes` die flache, dichte Tabelle aller Nodes über alle Cluster
  hinweg** — inklusive der Nodes ohne `cluster_id`, was der Grund für das
  flache, UUID-adressierte Datenmodell ist —, **`/clusters` die Cluster-Liste
  samt Detailsicht** (Control-Plane vs. Worker, etcd-Member, Cluster-Health,
  Zertifikatsablauf, Abstand zum Rand). Der bisherige Instanz-Status rutscht auf
  `/` in eine Nebenrolle, **verschwindet aber nicht**: die Kettenbruch-Warnung
  aus D-15 (Phase 1) darf nicht wegfallen. — **Reversibility:** reversible.

- **D-30:** Node-Detail ist eine **eigene Route `/nodes/$uuid`**, kein Panel und
  kein Sheet. Ein Node ist das Objekt, über das im Störfall gesprochen wird —
  eine teilbare URL ist mehr wert als der gesparte Klick. Die Phasen 5 (Logs),
  6 (Aktionen) und 7 (Config) hängen sich alle dort ein; ein Sheet wäre binnen
  drei Phasen zu klein. — **Reversibility:** costly — die Route ist der
  Einhängepunkt dreier späterer Phasen.

- **D-31:** In `web/src/components/Sidebar.tsx` wird bei `/nodes` und
  `/clusters` lediglich `phase: 3` auf `null` gesetzt. Die Platzhalter-Routen
  verschwinden dadurch **automatisch**, weil `placeholders.tsx` sie aus
  `NAV_AREAS` ableitet. **Keine Änderung an der Navigation** — genau der
  Handgriff, für den D-10 aus Phase 1 die Shell gebaut hat. —
  **Reversibility:** reversible.

### Claude's Discretion

**Alle 31 Entscheidungen oben sind Claude's Discretion** — siehe
`<autonomy_note>`. Die folgenden Punkte sind zusätzlich *nicht* entschieden und
liegen im Ermessen von Researcher und Planner:

- Konkreter Zuschnitt der neuen Store-Entities (`Clusters()`, `Machines()`) und
  ob der persistierte Snapshot aus D-16 am Machine-Record hängt oder eine eigene
  Entity ist
- Die genauen RFC-9457-Codes für Import-Fehlschläge (D-05) und für die
  Cluster-Sperre (D-22) — zu prägen in `docs/api-contract.md` im selben Commit
- Polling-Intervall des Clients gegen die Snapshot-Routen (D-18)
- Backoff-Werte und Fehlerzahl-Schwelle des Degradations-Automaten (D-17);
  Pattern 7 gibt die Zustände vor, nicht die Konstanten
- Route-Zuschnitt der Leseflächen (eine Sammelroute pro Cluster vs. mehrere)
- Wie die Import-Ableitung im Audit-Log erscheint, inklusive des
  Allowlist-Eintrags — mit der Warnung aus `<code_context>`, dass ein
  vergessener Eintrag **permanent** ist

</decisions>

<open_questions>
## Was der Researcher klären muss

Diese Punkte sind **Mechanik, nicht Politik** — die Politik ist oben entschieden.
Sie sind hier aufgeführt, damit `gsd-phase-researcher` weiß, wonach zu suchen
ist, statt die Entscheidungen neu zu treffen.

1. **Die tragende Frage des Imports:** Ist die vollständige MachineConfig eines
   Control-Plane-Nodes samt `.machine.ca.key` über COSI mit einem
   Admin-`talosconfig` lesbar, und unter welchem Ressourcentyp und welcher ID?
   D-01 hängt daran. Scheitert das, ist die Rückfallposition ein vom Betreiber
   geliefertes `secrets.yaml` — D-02 bliebe unverändert gültig.
2. Welche COSI-Ressource trägt die Cluster-Mitgliedschaft, wenn der
   Talos-Discovery-Service abgeschaltet ist (`cluster.Member`,
   `cluster.Affiliate`, oder nur `EtcdMemberList` plus manuell)? — D-08
3. Wie ist die `schematic_id` als virtuelle System-Extension abrufbar? — D-12
4. Welche der Felder aus INV-06 (CPU/RAM, Disks, Interfaces, Service-Status,
   Talos-Version, K8s-Version) kommen aus COSI-Ressourcen und welche brauchen
   einen unary RPC? Beide Hälften — `ClusterClient` und `talossim` — müssen
   dann synchron wachsen. — D-19
5. Ist die Kubernetes-Version node-seitig lesbar (z. B. über die
   `k8s`-Ressourcen), ohne `:6443` anzufassen? — D-20
6. Läuft der Tier-1-Docker-Provisioner auf `darwin/arm64`? Offener
   Research-Flag der ROADMAP und Blocker-Eintrag in `STATE.md`. — D-26, D-27

</open_questions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phasenumfang und Requirements
- `.planning/ROADMAP.md` § *Phase 3: Inventar, Cluster-Import & Health* — Goal, 5 Success Criteria, Research-Flag, „Parallel tracks: 2", Release-Blocker-Besitz
- `.planning/ROADMAP.md` § *Phase 5: Streaming* — der Entry-Blocker, den D-18 nicht vorzieht
- `.planning/REQUIREMENTS.md` Zeilen 38, 51–63 — TRANS-08 und INV-01…INV-13 wörtlich
- `.planning/PROJECT.md` § *Out of Scope* — kein Workload-Management, keine eigene Metrics-Pipeline; die Naht `:50000` vs. `:6443` (trägt D-20)

### Vorentscheidungen — geerbt, nicht neu zu verhandeln
- `.planning/phases/01-foundation-skeleton/01-CONTEXT.md` — D-01…D-16; besonders D-06 (deklarative Destructive-Markierung, trägt D-11/D-22), D-05 (Sudo-Fenster), D-09 (UI englisch, keine i18n), D-10 (permanente Shell, trägt D-31), D-11/D-12/D-13 (dark-first, shadcn, Tabellenmuster), D-15 (Kettenbruch-Banner, trägt D-29)
- `.planning/phases/02-transport-seam-talossim-image-factory/02-CONTEXT.md` — D-01…D-10; besonders D-03 (Dry-Run im Transport), D-05 (`Version`-Probe als Liveness, trägt D-04), D-06 (getrennte Client-Typen), D-07 (depguard/Modulgrenze, trägt D-28), D-08 (eingebettete kuratierte Liste, trägt D-24), D-09 (Store-Entity-Naht), D-10 (Audit-Actor-Vokabular)
- `.planning/phases/02-transport-seam-talossim-image-factory/02-CONTEXT.md` § `<deadline_policy>` — Deadline-Klassen und Retry-Allowlist, **bestätigt und geschlossen**; enthält den Phase-5-Entry-Blocker, den D-18 respektiert

### Research — mit den Korrekturen aus `02-CONTEXT.md` lesen
- `.planning/research/ARCHITECTURE.md` § *Pattern 6* — `HealthLevel` und `Field[T]`; die Quelle von D-13/D-14 und der prüfbare Satz
- `.planning/research/ARCHITECTURE.md` § *Pattern 7* — Watch primär, Poll als Heartbeat, Degradations-Automat; trägt D-17
- `.planning/research/ARCHITECTURE.md` § *Pattern 8* und *The sandbox ladder* — Fake-Drift und die drei Tiers; trägt D-26/D-27/D-28
- `.planning/research/ARCHITECTURE.md` § *Data Flow* — Read-Pfad und die Regel „der Browser pollt nie die Talos-API"
- `.planning/research/SUMMARY.md` § *Phase 2: Inventory + cluster import + health* (Zeile 225 ff.) — die Liefer-Liste dieser Phase
- `.planning/research/SUMMARY.md` § *Critical Pitfalls* Punkt 6 (P1/P5/P16) — PKI-Ablauf, Import vom Control-Plane-Node, Konnektivitätsbeweis; **die Quelle von D-01 bis D-05 und D-23**
- `.planning/research/SUMMARY.md` § *Critical Pitfalls* Punkt 2 (P17) — Read-only-Adoption; trägt D-21
- `.planning/research/PITFALLS.md` — P1, P5, P16 (PKI), P17 (Mock-only), P9 (`metal-installer`; der v1.9.0-Befund ist laut STATE.md überholt)
- `.planning/research/STACK.md` — Machinery-Pin. **Achtung: das Connect-Snippet kompiliert nicht und das Image-Factory-Urteil ist von D-01 aus Phase 2 überstimmt** (Begründung in `02-CONTEXT.md` § `<research_flag_resolved>`)

### Bestehende Verträge, die diese Phase nicht brechen darf
- `docs/api-contract.md` § *Error Taxonomy* — RFC 9457, ausdrücklich **closed**; neue Codes werden bewusst geprägt, im selben Commit
- `docs/api-contract.md` § *Route Registration Rule* — eine `…Routes(deps)`-Funktion hinzufügen, `router.go` nie anfassen
- `docs/api-contract.md` § *CSRF Contract*, § *Audit Query Contract*, § *Actor vocabulary*, § *Dry-run*
- `docs/talossim.md` — Szenario-Tabelle; `TestDocumentedScenariosMatchRegistry` hält sie mit `internal/talossim/scenario.go` synchron. Ein Ausbau nach D-19 pflegt beide Seiten im selben Commit
- `internal/store/store.go:5-7` — jedes `os.ReadFile` außerhalb `internal/store/fsstore` ist ein Architekturfehler; trägt D-06
- `internal/talos/talos.go` — `Target` („Addr is a hint and never identity", trägt D-10), `Creds`, `CredKind`, `Dialer`, `DiscoverySource`, `Identity`
- `internal/talos/version.go` — unterstützte Talos-Range v1.12…v1.14; **getrennt von der Kompatibilitätsmatrix** (D-25)
- `internal/depguard_test.go` — Modulgrenze; trägt D-28
- `sandbox/README.md` — was in das separate Modul gehört
- `.planning/WINDOWS.md` — Register unausgeführter Verifikationen; D-27 trägt dort ein

</canonical_refs>

<code_context>
## Existing Code Insights

### Wiederverwendbare Bausteine
- **`internal/talos.ClusterClient`** hat bereits `COSI() state.State`, `Version`,
  `Hostname`, `ServiceList`, `EtcdMemberList`, `Probe`. **`MaintenanceClient`**
  hat `Version`, `Disks`, `COSI`. Beides fertige Einstiegspunkte für das
  Read-Model.
- **`internal/talos.FanOut[T]`** existiert — paralleles Lesen über mehrere
  Nodes ist gebaut, nicht neu zu erfinden.
- **`internal/talos.NewManualSource(targets...)`** ist die bereits vorhandene
  manuelle `DiscoverySource` aus D-08.
- **`internal/talossim`** bringt `etcd_down`, `k8s_down` und
  `ip_changes_on_reboot` bereits als registrierte Szenarien mit — die drei, die
  D-14 und D-10 prüfbar machen. `Server.COSI()` gibt den echten
  COSI-`state.State` heraus, in den weitere Ressourcen geseedet werden können.
- **`model.ClusterID` / `model.MachineID`** existieren mit **null Nutzern** und
  warten genau auf diese Phase; `audit.Record` hat die passenden Spalten
  reserviert.
- **Die Store-Entity-Naht** ist an `Schematics()` / `SchematicStore`
  (`Get/List/Put/Delete` + `Rev`-CAS) exemplarisch vorgeführt — die Vorlage für
  `Clusters()` und `Machines()`.
- **`internal/tlsx.Fingerprint`** liefert exakt das Format, das
  `talos.Creds.Fingerprint` erwartet (Doppelpunkt-getrennte Großhex) — trägt D-03.
- **Die Audit-Seite** ist das gebaute Tabellenmuster; `web/src/components/ui/`
  enthält bereits `table`, `badge`, `card`, `skeleton`, `dialog`, `select`.
- **`NAV_AREAS`** in `web/src/components/Sidebar.tsx` führt `/nodes` und
  `/clusters` bereits mit `phase: 3`, und `web/src/routes/placeholders.tsx`
  leitet die Platzhalter daraus ab — trägt D-31.

### Etablierte Muster, die diese Phase binden
- **`migrate.CurrentVersion = 2`.** Zwei neue Entities (D-16, `Clusters`,
  `Machines`) heben das Schema auf 3 und brauchen eine vorwärtsgerichtete
  Migration. Der Store kennt heute nur `Users`, `Settings`, `Sessions`,
  `Schematics`.
- **Die Audit-Redact-Allowlist ist eine einzige geteilte Tabelle**
  (`internal/audit/redact.go`), die jeder Plan mit einem neuen Action-Token
  bearbeiten muss — ein garantierter Merge-Punkt in einer als zwei parallele
  Tracks geplanten Phase. **Ein vergessener Eintrag ist still**: der Satz wird
  geschrieben, jeder Parameter liest `<redacted>`, und D-16 aus Phase 1 kennt
  keinen Löschpfad. Der Import ist der erste Kandidat dafür.
- **Audit ist fail-closed und fsynct pro Satz unter einem globalen Mutex**,
  dimensioniert auf „a handful per minute". **Der Import ist die erste
  Fan-out-Schreiblast** (ein Satz je entdecktem Node); Contention wird dort zu
  *Mutationsfehler*, nicht bloß zu Latenz.
- **Die RFC-9457-Taxonomie ist geschlossen.** `upstream` mit `502` und den Codes
  `upstream.node-unreachable` / `node-timeout` ist bestätigt und existiert. Wird
  für D-05 und D-22 kein Code bewusst geprägt, landet jeder Fehlschlag als
  `internal.unexpected` — das per Design kein Detail trägt — **permanent** im
  Audit-Archiv.
- **`internal/talossim` implementiert `Memory`, `CPUInfo`, `NetworkDeviceStats`,
  `LoadAvg`, `Mounts` und `Netstat` heute nicht**, und `seedCOSI` legt nur
  `hardware.SystemInformation`, `network.NodeAddress` und `k8s.Nodename` an.
  INV-06 ist ohne Ausbau **nicht testbar** — das ist der Kern von D-19.
- **Fingerprint-Pinning im Maintenance-Mode fehlt** (`Creds.Fingerprint` wird von
  `NewMaintenanceClient` entgegengenommen und nicht benutzt, T-02-27). Für Phase
  3 ohne Folgen — der Import läuft über `CredCluster` —, aber es darf nicht
  versehentlich als erledigt gelten.

### Integrationspunkte
- **`cmd/holzkubed/main.go`, `run()`** — neue Dienste werden als
  Konstruktoraufruf **vor** der `slices.Concat`-Zeile verdrahtet, plus je ein
  `Deps`-Feld und ein `Routes`-Eintrag. **Achtung:** `httpapi.Deps` wird bei
  `main.go:156` **als Wert kopiert** — ein nach dieser Zeile zugewiesenes Feld
  ist in jeder Handler-Closure der Nullwert, **ohne Compile-Fehler**.
- **`internal/httpapi/router.go`** — wird nicht angefasst (Route Registration
  Rule).
- **`internal/httpapi/middleware`** — Ort der Sperr-Prüfung aus D-22, neben der
  bestehenden `Destructive`-Prüfung.
- **`internal/store`** — Schema-Bump plus `Clusters()` / `Machines()`.
- **`docs/api-contract.md`** — neue Routen, neue Problem-Codes, neue
  Audit-Allowlist-Einträge.
- **`web/src/components/Sidebar.tsx` und `web/src/routes/`** — D-29, D-30, D-31.

</code_context>

<specifics>
## Specific Ideas

- **Der prüfbare Satz aus ARCHITECTURE Pattern 6 wird wörtlich ein Test:**
  talossim mit `etcd_down` **und** `k8s_down` starten, Node-Detail abrufen, und
  jedes `LevelNode`-Feld hat weiterhin `available: true`. Dieser eine Test hält
  INV-07 und INV-08 dauerhaft; ohne ihn kriecht die Kubernetes-Abhängigkeit
  laut Research binnen drei Phasen zurück.
- **`ip_changes_on_reboot` ist der Test für D-10.** Das Szenario existiert
  bereits. Zu prüfen ist, dass der Record derselbe bleibt und nur die Adresse
  wechselt. Der WINDOWS.md-Eintrag 7 gilt weiter: das Szenario kappt beim
  Rebind bestehende Verbindungen, ist also strenger als echte Hardware.
- **Der Import ist die erste Stelle, an der holzkube das Bundle anfasst.**
  PITFALLS nennt die fünf Felder namentlich (`.machine.ca.key`,
  `.cluster.secret`, `.cluster.token`, `.machine.token`,
  `cluster.serviceAccount.key`). Sie dürfen im Audit-Log, in jeder API-Antwort
  und in jeder Fehlermeldung nur redigiert auftauchen — auch in dieser Phase,
  in der es noch keine „View MachineConfig"-Funktion gibt.
- **Ein Cluster, dessen Zertifikat abgelaufen ist, sieht aus wie ein toter
  Cluster.** Der Unterschied ist der einzige Grund, warum D-23 einen eigenen
  Zustand fordert: fünf rote Nodes lassen den Betreiber die falsche Reparatur
  beginnen.
- Das Datenmodell ist **flach und UUID-adressiert mit nullbarem `cluster_id`**,
  nicht unter Clustern verschachtelt. Research begründet das dreifach:
  Zuordnung wäre sonst ein Dateiumzug (Rename-Race mitten im Provisioning), ein
  UUID-Lookup müsste jedes Cluster-Verzeichnis scannen, und „nicht zugeordnet"
  wäre ein Sonderfall statt eines Normalzustands.

</specifics>

<deferred>
## Deferred Ideas

- **Ein-Klick-Erneuerung des Client-Zertifikats** — Phase 10 (Härtung). INV-05
  verlangt nur Sichtbarkeit und rechtzeitige Warnung, und Erneuern ist eine
  eigene Fähigkeit. **Sicher aufschiebbar, weil D-02 das Bundle persistiert:**
  die Erneuerung ist danach jederzeit möglich, ohne einen Node anzufassen. Ohne
  D-02 wäre sie unmöglich — deshalb steht die Auslagerung nur unter dieser
  Bedingung.
- **CA-Rotation** — ausdrücklich **nie in v1**. Research P5: der Blast Radius
  ist schlimmer als die Abwesenheit.
- **Subnetz-Scan als `DiscoverySource`** — Phase 8, wo Maschinen im
  Maintenance-Mode gefunden werden müssen (D-09).
- **SSE für Inventar-Deltas** — Phase 5, und erst nach deren Entry-Blocker.
  Die Response-Shape aus D-13 bleibt gleich, der Wechsel ist client-lokal (D-18).
- **Node aus dem Cluster entfernen (cordon/drain → reset → aus dem Inventar)** —
  Phase 6. Phase 3 löscht nur den *Record*, auf ausdrückliche Aktion (D-11), und
  fasst den Node dabei nicht an.
- **Fingerprint-Pinning für Maintenance-Mode-Verbindungen (T-02-27)** — Phase 8,
  wo der Maintenance-Mode zuerst wirklich benutzt wird.
- **`holzkubed audit verify` über den gesamten Bestand** — offener Punkt aus
  Phase 1 (der Start prüft nur zwei Dateien tief), gehört nicht hierher.
- **`ROADMAP.md` Zeile 81 trägt zwei driftende Plan-Zählungen** — offener
  Aufräumpunkt aus Phase 2, keine Phase-3-Arbeit.

</deferred>

---

*Phase: 3-inventar-cluster-import-health*
*Context gathered: 2026-09-05*
