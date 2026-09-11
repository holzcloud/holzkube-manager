# Phase 3: Inventar, Cluster-Import & Health — Summary

**Executed:** 2026-09-11
**Status:** complete, mit zwei protokollierten offenen Fenstern (76, 77)

## Was gebaut wurde

### Der Lesekern

`internal/health` trägt das Vokabular, in dem jede Antwort dieser und jeder
späteren Phase spricht: `Level` (auf der Leitung ein String, nie die
`iota`-Zahl — D-13), `Field[T]` mit den drei unterscheidbaren Zuständen aus
D-14, und der Degradations-Automat aus ARCHITECTURE Pattern 7.

Der mittlere der drei Zustände ist der ganze Punkt und der einzige, den man
falsch bauen kann, ohne es zu merken: **ein veralteter Wert behält seinen
Wert.** Ihn wegzulassen ist in der Oberfläche von `null` nicht zu
unterscheiden, und genau so entsteht die leere Seite, die INV-08 verbietet —
auf dem Bildschirm, der für den Störfall existiert, während des Störfalls.

Der prüfbare Satz aus Pattern 6 steht wörtlich als Test
(`TestNodeLevelFactsSurviveEtcdAndKubernetesBeingDown`): talossim mit
`etcd_down` **und** `k8s_down`, Node-Detail abrufen, und jedes
`LevelNode`-Feld ist weiterhin `available: true`.

### Identität

Das Datenmodell ist flach und UUID-adressiert mit nullbarem `cluster_id`
(D-10). Antwortet unter einer bekannten Adresse eine fremde UUID, bekommt die
Fremde einen eigenen Record und die alte wird als „nicht mehr unter dieser
Adresse" markiert — **nie überschrieben**. Die Gegenrichtung würde bei einer
DHCP-Rotation die Historie zweier Maschinen vertauschen, und zwar still.

### Adoption

`POST /api/v1/clusters/fingerprint` liest das Zertifikat, dem noch nichts
traut. `POST /api/v1/clusters` schickt talosconfig, Adresse und den bestätigten
Fingerprint zusammen; der Server liest das Zertifikat erneut und lehnt ab, wenn
es nicht mehr passt. Die Bestätigung ist damit ein Tor und keine Zeremonie.

Danach, in dieser Reihenfolge: verbinden, die MachineConfig des Nodes lesen,
das Bundle daraus ableiten, sich selbst ein Zertifikat daraus ausstellen, unter
diesem wieder verbinden und `Version` rufen. **Vor Schritt fünf wird nichts
geschrieben.** Ein halb adoptierter Cluster ist ein Zustand, den dieses Produkt
nicht hat.

Ein Worker wird benannt abgelehnt (`validation.node-not-controlplane`): seine
MachineConfig trägt die CA ohne privaten Schlüssel. Der Adoptionspfad kann kein
CA-Material erzeugen, und das hält ein Test, der den Quelltext liest, nicht die
Disziplin (D-07). Der Erzeugungspfad existiert daneben als eigene Route.

### Die Sperre

Deklarativ an der Route, gelesen von Middleware, und niemand matcht auf URLs
(D-22). Sie sitzt **innerhalb** des Audit-Glieds — ein Änderungsversuch an
einem read-only adoptierten Cluster gehört ins Archiv — und **außerhalb** des
Sudo-Fensters, weil ein Passwort-Prompt vor einer sicheren Ablehnung dem
Betreiber beibringt, dass der Prompt nichts bedeutet.

### talossim wuchs mit

Ohne Ausbau wäre INV-06 nicht testbar gewesen (D-19). Der Simulator generiert
jetzt eine echte Cluster-PKI mit machinerys eigenem Generator, leitet daraus
beide MachineConfigs ab und stellt seine Zertifikate **aus der OS-CA dieses
Clusters** aus — erst das macht den Konnektivitätsbeweis aus D-04 überhaupt
prüfbar. Dazu Processor, MemoryModule, block.Disk, LinkStatus,
HostnameStatus, ExtensionStatus (die virtuelle Schematic-Extension) und
KubeletSpec. `k8s_down` räumt den KubeletSpec mit ab, sonst meldete ein Node
mit totem Kubernetes weiterhin selbstbewusst eine Kubernetes-Version.

## Was die Budget-Tabelle gefangen hat

Sofort, beim ersten Lauf: eine Talos-RPC auf einem Request-Context hat keine
Deadline — der Write-Timeout des Servers ist keine —, und `ErrNoDeadline` ist
eine Verweigerung ohne Umweg. Die drei knotenlesenden Routen antworteten
`500 internal.unexpected` auf einen Aufruf, der den Prozess nie verlassen hat.
Drei neue Zeilen in `cmd/holzkube-managerd/budget_test.go` und zwei
Knoten-Klassen, die ihre Zahlen aus `internal/talos` lesen statt sie zu
wiederholen.

## Erfolgskriterien

| # | Stand |
|---|---|
| 1 | **erfüllt** — Import mit Fingerprint-Bestätigung und Konnektivitätsbeweis; Erzeugung als getrennter Pfad, die Trennung von einem Test gehalten |
| 2 | **erfüllt** — jedes Feld trägt Level und `stale_since`; gelesen aus COSI-Ressourcen mit gejittertem Heartbeat. Siehe Fenster 79 zur Abweichung Watch ↔ Heartbeat |
| 3 | **erfüllt** — UUID gewinnt gegen die Adresse, Records überleben alles, veraltete Werte bleiben sichtbar und markiert |
| 4 | **erfüllt** — Control-Plane vs. Worker, etcd-Member, Zertifikatsleiter 90/30/7/abgelaufen, Abstand zum Rand als Zahl **und** als Satz, Sperre serverseitig |
| 5 | **nicht erfüllt** — die Suite läuft grün gegen talossim; der Tier-1-Provisioner und der reale Transport existieren, wurden hier aber nie ausgeführt (kein laufender Docker-Daemon). Fenster 76, mit der zweiten Einschränkung: selbst ein grüner Lauf deckt nur zwei der neun Szenarien ab |

## Offene Fenster aus dieser Phase

- **76** — Tier 1 nie ausgeführt, und sieben von neun Szenarien auf einem echten Node nicht induzierbar
- **77** — `golangci-lint` ist zu alt für die Ziel-Go-Version und lief nicht
- **78** — `Refresh` behandelt die Rev-Kollision nicht; ein Snapshot kann verloren gehen
- **79** — Supervisors sind Heartbeat-Poller über COSI-Reads, keine Watches

## Was diese Phase bewusst nicht tut

Kein Streaming und keine SSE-Route (Phase 5, mit dokumentiertem Entry-Blocker),
keine Jobs und keine Node-Aktionen (Phase 6), kein Ansehen oder Anwenden von
MachineConfig (Phase 7 — Phase 3 liest die Config genau einmal, im Import, und
ausschließlich zur Ableitung), kein Subnetz-Scan (Phase 8), keine
Zertifikatserneuerung (Phase 10).
