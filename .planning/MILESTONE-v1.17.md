# Milestone v1.17 — Kubernetes und Pods

**Angelegt:** 2026-09-19
**Angelegt von:** Claude, auf die Anweisung „ich will noch zusätzliche features:
der manager soll auch kubernetes selber und die pods managen können."
**Status:** die zwei Entscheidungen sind vom Betreiber getroffen, der Umfang
folgt daraus

## Was diese Anweisung ändert, und sie ändert etwas Grundsätzliches

`PROJECT.md` zieht eine Grenze, und sie ist die Begründung für ziemlich viel in
diesem Repository: **dieses Produkt spricht die Talos machine API und sonst
nichts.** Aus ihr folgt, dass der Daemon außerhalb des Clusters läuft, dass er
ohne Kubernetes auskommt, wenn Kubernetes kaputt ist, und dass es genau einen
Upstream gibt, dessen Versionen gepflegt werden müssen.

Fenster 86 hält den Preis dieser Grenze fest: **das Kubernetes-Upgrade
schreibt die Images in die MachineConfig und spricht nicht mit Kubernetes.**
`talosctl upgrade-k8s` orchestriert über die Kubernetes-API — prepull, dann die
Static Pods einzeln, dann kube-proxy —, und dieses Produkt kann das nicht, weil
es den Transport nicht hat. Dieselbe Grenze macht Omnis „Sync Kubernetes
Manifests" und den Workload-Proxy unbaubar.

**Die Anweisung hebt die Grenze auf, und diese Datei ist die Stelle, an der das
festgehalten wird statt vergessen.** `PROJECT.md` bleibt stehen; dieser Absatz
ist, was sie an diesem Punkt ablöst. Was NICHT abgelöst wird: der Daemon läuft
weiter außerhalb des Clusters, und alles, was heute ohne Kubernetes
funktioniert, muss ohne Kubernetes weiter funktionieren. Ein Cluster, dessen
API-Server weg ist, ist genau der Cluster, den ein Betreiber mit diesem Produkt
reparieren will — eine Oberfläche, die dann leer wird, hätte die Grenze zu Recht
gehabt.

## Die zwei Entscheidungen des Betreibers

**client-go, nicht ein eigener schlanker Client.** Vorgelegt wurden beide mit
ihren Kosten; die Wahl ist die kanonische Bibliothek. Was sie bringt: getippte
Typen, Watches und Informer, und — der eigentliche Grund, warum sie hier
richtig ist — `drain` mit derselben Semantik wie `kubectl`, also Eviction-API
und PodDisruptionBudgets. Ein selbstgebautes Drain, das Pods härter abschießt
als `kubectl`, wäre ein Werkzeug, dem man genau im Ernstfall nicht trauen kann.

Was sie kostet, und das steht hier, weil es sonst später als Überraschung
auftritt: ein großer Abhängigkeitsbaum (vorher **null** Module aus `k8s.io`),
ein deutlich größeres Binary, und ein **zweiter Upstream**, dessen Version eine
Produktentscheidung ist. `internal/depguard_test.go` pinnt heute machinery und
das COSI-Runtime mit der Begründung, dass ein `go get -u` keine Entscheidung
sein darf. client-go gehört in dieselbe Liste, mit derselben Begründung.

**Voller Umfang, nicht erst die Leseseite.** Also: Lesen, Node-Aktionen,
Pod-Aktionen, Manifeste, Workload-Proxy. Das sind mehrere Runden, und die
Reihenfolge unten ist die, in der jede Scheibe die nächste belegbar macht.

## Die Version, und warum diese

client-go v0.34, weil der Cluster des Betreibers Kubernetes v1.34 fährt und
`internal/compat` v1.34.1 als obere Grenze führt. Die Regel aus `CLAUDE.md`
über den machinery-Pin gilt hier genauso: ein Client, der älter ist als die API,
die er bedient, beschreibt Felder nicht, die der Betreiber im Cluster sieht.

## Die Reihenfolge

```
1  Fundament      client-go, Zugangsweg, internal/kube, der Fake        FERTIG
2  Lesen          Nodes, Namespaces, Pods, Events, Logs + Bildschirm    FERTIG
3  Node-Aktionen  cordon, uncordon, drain (Eviction + PDB)              FERTIG
4  Pod-Aktionen   Pod löschen/neu starten, Deployment skalieren         FERTIG
5  Manifeste      anwenden und synchronisieren, mit Plan vorher         FERTIG
6  Proxy          einen Service erreichbar machen                       FERTIG
```

**Stand 2026-09-19, alle sechs Scheiben gebaut.** Was dabei gefunden und im Ledger
festgehalten wurde, gehört zur Reihenfolge und nicht in eine Fußnote:

- Der Fake musste dreimal erweitert werden, weil er sonst Tests bestehen ließe,
  die Hardware fallen lässt: erst echtes mTLS, dann Zustand, der sich ändert
  (ein cordon verändert die nächste Liste), dann Discovery und Server-Side
  Apply. Die Regel unten hat das jedes Mal erzwungen.
- Ledger 147: client-go drosselt sich selbst auf fünf Anfragen pro Sekunde. Der
  Manifest-Plan fragt pro Objekt, also hätte eine Anfrage ihr ganzes Budget in
  einer Warteschlange im eigenen Prozess verbracht und danach den Cluster für
  langsam erklärt. Gemessen: 60 Objekte in 10,0 Sekunden, nach der Korrektur
  0,02 Sekunden.
- Daraus entstand `MaxManifestObjects = 256`. Ohne Deckel ist die Zahl der
  Upstream-Calls das, was jemand eingefügt hat — und die Budgetzeile wäre nicht
  schreibbar gewesen, was das ehrliche Signal ist, dass die Route dann keinen
  schlechtesten Fall hat.
- Ledger 148 bleibt offen: der ganze Manifest-Pfad ist gegen den Fake bewiesen
  und nie gegen einen echten Cluster gelaufen. Dieselbe Unterscheidung wie 143
  für die CA-Rotation.
- Ledger 149: der Layout-Wächter misst nur den leeren Zustand. /kubernetes
  meldete „3 Bedienelemente", obwohl die Seite inzwischen ein Dutzend trägt —
  alles hinter Daten wird nie gerendert, weil das Skript einen frischen Daemon
  ohne Cluster startet.
- Ledger 150: die Proxy-Route war zuerst ein GET mit `?port=&path=`, und der
  Allowlist-Eintrag dafür war eine leere Behauptung — die Audit-Middleware liest
  nur Bodies. Als POST beschreibt er etwas, das wirklich ankommt. Die allgemeine
  Falle steht im Eintrag: eine Aktion, deren Parameter zum Ereignis gehören,
  darf sie nicht in der URL tragen.
- Ledger 151 bleibt offen: der Proxy ist nur gegen den Fake bewiesen. Seine
  entscheidende Zusage — der Content-Type des Workloads wird nicht
  weitergetragen — ist auf beiden Seiten rot geprüft.

### Scheibe 1 — Fundament

**Der Zugangsweg ist schon da und braucht keinen neuen.** Der Daemon hält die
Kubernetes-CA samt Schlüssel jedes adoptierten Clusters (`ClusterSecrets.K8sCACrt`
/ `K8sCAKey`, abgeleitet beim Import), und er holt heute auf Wunsch eine
kubeconfig über `MachineService/Kubeconfig`. Er kann sich also ein eigenes
Client-Zertifikat ausstellen — dasselbe Muster wie für Talos, inklusive der
Reihenfolge, die dort gilt: erst beweisen, dann speichern.

**Eine eigene Identität, nicht die Admin-kubeconfig.** Das Zertifikat wird auf
`holzkube-manager` ausgestellt, nicht auf `admin`, damit im Audit-Log des
Clusters steht, wer gehandelt hat. Dass es in `system:masters` liegt, ist eine
eigene Entscheidung und wird in Scheibe 3 zur Frage, ob eine eigene Rolle mit
weniger Rechten reicht.

**Was der Daemon damit kann, ausgesprochen:** alles, was cluster-admin kann. Das
ist nicht mehr als er heute schon hat — er hält die Talos-PKI, und damit kann er
jeden Knoten neu installieren —, aber es ist ein zweiter Weg zu demselben
Schaden, und ein Audit-Eintrag pro Aufruf gehört dazu wie bei den
Talos-Aktionen.

**Der Fake ist die eigentliche Arbeit dieser Scheibe.** `talossim` existiert,
weil ein Test gegen einen Simulator, der seinen Zustand nicht wirklich ändert,
nur belegt, dass ein RPC abgesetzt wurde (Fenster 3, und diese Woche die
CA-Rotation). Für Kubernetes gibt es drei Möglichkeiten und die Wahl ist
begründet:

- `client-go`s `fake.NewSimpleClientset` — kein HTTP, kein Transport, keine
  Authentifizierung. Prüft die Aufrufe des Produkts gegen eine Attrappe, nicht
  den Weg dorthin.
- `envtest` — ein echter `kube-apiserver` plus `etcd` als Binärdateien. Das ist
  die Wahrheit und es bedeutet, dass die CI zwei Binärdateien herunterlädt und
  der Pi sie für `arm64` bräuchte.
- **ein eigener `internal/kubesim`**, nach dem Muster von `talossim`: ein
  `httptest`-Server über TLS mit Client-Zertifikatsprüfung, der die benutzte
  API-Teilmenge wirklich bedient und seinen Zustand ändert — ein `cordon`
  verändert, was ein `get node` danach zurückgibt.

Gewählt ist der dritte, mit demselben Argument, mit dem `talossim` gewählt
wurde: der Transport und die Authentifizierung sind Teil dessen, was schiefgeht,
und ein Fake ohne sie lässt Tests bestehen, die gegen einen echten API-Server
fallen. `envtest` bleibt als späterer Zusatz sinnvoll, genau wie die Tier-1- und
Tier-2-Stufen bei Talos.

**Was diese Scheibe nicht belegt:** dass es gegen einen echten API-Server läuft.
Dafür gibt es den Cluster des Betreibers, und der Nachweis gehört dazu — anders
als bei den Talos-Pfaden ist er hier billig, weil ein `get nodes` nichts
verändert.

### Scheibe 2 — Lesen

Nodes mit ihren Conditions, Namespaces, Pods mit Status, Neustarts, Knoten und
Alter, die Events des Clusters, und Pod-Logs. Ein eigener Bildschirm, und die
Zahlen kommen aus dem Cluster statt aus dem Inventar — die beiden dürfen sich
widersprechen, und wo sie es tun, ist das die Information.

Die Ehrlichkeitsregel des Inventars gilt hier genauso: ein Cluster, dessen
API-Server nicht antwortet, ist nicht „keine Pods", sondern „nicht gefragt", und
der Bildschirm muss das sagen können. Das ist dieselbe Unterscheidung wie
`checking` gegen `down` (Fenster 140).

### Scheibe 3 — Node-Aktionen

`cordon`, `uncordon`, `drain`. Damit schließt sich eine Lücke, die das Produkt
heute mit einem Hinweis überbrückt: beim Entfernen eines Knotens und beim
Upgrade sagt es dem Betreiber, er solle selbst cordon und drain machen.

Drain ist der Teil, der richtig gemacht werden muss: Eviction-API statt
`delete`, PodDisruptionBudgets respektieren, DaemonSet-Pods auslassen,
`emptyDir` benennen statt stillschweigend löschen. Genau dafür wurde client-go
gewählt.

### Scheibe 4 — Pod-Aktionen

Pod löschen (was ihn bei einem Controller neu starten lässt), Deployment
skalieren, `rollout restart`. Jede davon zerstörend im Sinne von D-06, also
hinter dem Sudo-Fenster und im Audit-Pfad.

### Scheibe 5 — Manifeste

Anwenden und synchronisieren, mit einem Plan davor — was würde sich ändern —
weil ein `apply`, das ohne Vorschau Dinge löscht, dieselbe Gattung ist wie ein
Reset ohne Bestätigung. Server-Side Apply, damit der Besitz der Felder
nachvollziehbar bleibt.

### Scheibe 6 — Workload-Proxy

Einen Service im Cluster über diesen Daemon erreichbar machen. Zuletzt, weil er
einen Port-Forward-Pfad durch den Daemon braucht und damit die einzige Scheibe
ist, die eine neue Angriffsfläche nach außen aufmacht.

## Was dadurch an Fenstern aufgeht oder zugeht

- **Fenster 86** (Kubernetes-Upgrade schreibt nur Images) bekommt mit Scheibe 3
  zum ersten Mal die Möglichkeit, das Richtige zu tun. Es wird nicht
  automatisch geschlossen: das Upgrade umzubauen ist eine eigene Entscheidung,
  und sie gehört nach Scheibe 3.
- Die Ausschlussbegründungen in `MILESTONE-v1.16.md` unter „Was eine
  Entscheidung braucht" sind mit dieser Datei beantwortet, für Manifest-Sync
  und Workload-Proxy.
- Neu und gleich mitgeschrieben: der zweite Upstream. Ein Wächter, der fragt,
  ob client-go noch zur Kubernetes-Version des Clusters passt, gehört in
  dieselbe Kategorie wie `talos-upstream.yml`.

## Die Regel, unter der auch das gebaut wird

Unverändert: **ein Wächter ist nichts wert, bis er gegen den absichtlich wieder
eingebauten Fehler rot gewesen ist.** Für diesen Milestone hat sie eine zweite
Hälfte, die aus dieser Woche kommt: **ein Fake, der den Zustand nicht wirklich
ändert, lässt Tests bestehen, die gegen echte Hardware fallen.** Die CA-Rotation
wäre daran vorbeigelaufen, wenn `talossim` die angewandte Konfiguration weiter
nur gezählt hätte.
