# v1.15 Phase 1: Support-Bundle-Export — Summary

**Executed:** 2026-09-12
**Status:** alle fünf Kriterien erfüllt
**Requirements:** V2-OPS-01
**Commit:** `3da551b`

## Was gebaut ist

Ein Archiv mit dem, was jemand beim Debuggen dieses Clusters sonst über sechs
Browser-Tabs zusammensammelt, während er unter Druck steht. Pro Node: Facts,
Service-Liste, Talos- und Kubernetes-Version, Disks, Links, Extensions,
etcd-Status, `dmesg`, Logs und die **redigierte** MachineConfig. Dazu der
Audit-Tail, die Instanz-Metadaten und ein Manifest.

`internal/support` ist die Sammlung, `internal/support/tail.go` der Audit-Tail
über `audit.Query` (nicht über die Datei), `internal/httpapi/handlers/support.go`
die Route, `cmd/holzkube-managerd/commands.go` das Subkommando.

## Die zwei Eigenschaften, die die Form bestimmen

**Es muss verschickbar sein.** Ein Bundle ist per Definition ein Archiv mit
allem, und „allem" schließt den privaten Schlüssel der Cluster-CA ein. Jede
Konfiguration geht durch die beiden Durchgänge aus `internal/machineconfig`.

Der Abnahmetest walkt Entropie über **jede Datei im Archiv** statt über die, an
die der Autor gerade dachte: ein Schlüssel, der in `facts.json` oder in einer
Logzeile landet, wäre von einer Prüfung, die nur dorthin sieht, wo sie hinsieht,
nicht gefunden worden. Gesucht wird der *echte* Schlüssel aus dem Secrets-Bundle
des simulierten Clusters — und zusätzlich sein Base64-Körper ohne PEM-Rahmen,
weil ein Schlüssel, den jemand ohne Armour weiterreicht, derselbe Schlüssel ist.
Seine Abwesenheit ist damit ein Beleg und nicht die Abwesenheit eines Markers.

**Und wenn ein Schlüssel beide Durchgänge überlebt, wird die Datei gar nicht
geschrieben**, mit einem Eintrag in `Incomplete`. Ein Bundle, das eine
Zertifizierungsstelle preisgibt, ist schlimmer als eines, dem eine Konfiguration
fehlt.

**Ein Node, der nicht antwortet, ist der Zweck und nicht der Fehler.** Das
Bundle, das jemand wirklich braucht, ist das, das während der Störung gezogen
wird. Ein nicht erreichbarer Node trägt seinen gespeicherten Record bei plus
eine `UNREACHABLE.txt`, die sagt *welcher* Node und *warum*; der Lauf geht
weiter, und das Manifest führt jede Lücke. Eine Sammlung, die am ersten toten
Node abbricht, produziert genau dann nichts, wenn es darauf ankommt.

## Warum Route *und* Subkommando

Sie werden von verschiedenen Orten aus erreicht. Die Route lädt herunter, weil
der Betreiber im Vorfall im Browser ist. Das Subkommando läuft auf dem Host,
wenn die Oberfläche Teil dessen ist, was kaputt ist — und es sagt das in seinem
eigenen Manifest: es nimmt den Store-Lock, also läuft holzkube-manager nicht,
also erreicht es **bewusst keinen Node**, und das Bundle ist die gespeicherte
Hälfte. Ein Subkommando, das behauptet, ein vollständiges Bundle zu liefern,
während es nur den Store gelesen hat, wäre ein Bundle, dem man nicht ansieht,
was fehlt.

## Die Grenzen sind benannt, nicht geraten

`MaxLogBytes = 256 KiB` pro Stream, `PerNodeTimeout = 90s`,
`AuditTailRecords = 500`.

Abgeschnitten wird am **Anfang**, nicht am Ende: die neuesten Bytes bleiben,
weil die letzten Zeilen vor einem Ausfall die sind, wegen derer man das Bundle
zieht. Der Schnitt liegt auf einer Zeilengrenze, und die erste Zeile der Datei
sagt, wie viel fehlt. Eine halbe Zeile am Anfang einer Logdatei ist eine Zeile,
die jemand falsch liest.

## Ein echter Fehler, den erst der Lauf gefunden hat

Ein Node, dessen Cluster keine gespeicherte PKI hat, erzeugte
`store: record not found`. Das sagt einem Betreiber nichts — nicht welcher
Record, nicht welcher Cluster, nicht was zu tun wäre. `inventory.Connect`
benennt jetzt den Cluster und unterscheidet „diese Maschine gehört zu keinem
Cluster" von „dieser Cluster hat keine gespeicherte PKI". Der Satz erscheint
wörtlich in der `UNREACHABLE.txt` im Bundle.

Das ist der Grund, warum das Subkommando gegen einen echten Store ausgeführt
wurde und nicht nur getestet: die Fehlermeldung war in jedem Unit-Test
unsichtbar, weil kein Test sie gelesen hat.

## Erfolgskriterien

| # | Stand |
|---|---|
| 1 | **erfüllt** — Facts, Services, Versionen, Disks, Links, Extensions, etcd-Status, redigierte Config pro Node; Audit-Tail und Instanz-Metadaten im Archiv |
| 2 | **erfüllt** — `UNREACHABLE.txt` pro nicht erreichbarem Node mit benanntem Grund, gespeicherter Record trotzdem dabei, Lauf läuft weiter, Manifest führt jede Lücke |
| 3 | **erfüllt** — `TestNoFileInTheBundleCarriesAPrivateKey` und `TestTheClustersOwnCertificateAuthorityKeyIsNowhereInTheBundle` walken über jede Datei; gesucht wird der echte Schlüssel plus sein Base64-Körper ohne Armour |
| 4 | **erfüllt** — `support`-Subkommando (1,4 KiB Bundle, Manifest mit 2 Nodes und 2 Lücken) und `GET /api/v1/clusters/{id}/support-bundle` (200, `application/gzip`, `content-disposition`) |
| 5 | **erfüllt** — `MaxLogBytes` pro Stream, Schnitt auf Zeilengrenze, Kopfzeile benennt die verworfene Menge |

## Was diese Phase nicht tut

Sie lädt **nichts hoch**. Ein Bundle geht auf die Platte des Betreibers und
sonst nirgendwohin; ein Produkt, das Diagnosen an einen Dienst schickt, ist ein
anderes Produkt, und die Entscheidung dazu gehört nicht in eine autonome
Sitzung.

Sie sammelt **keine Kubernetes-Objekte**. Der Transport-Seam spricht die
Talos-Maschinen-API; ein `kubectl get`-Äquivalent wäre ein zweiter Transport mit
eigener Authentifizierung, und `talosctl support` zieht diese Grenze an
derselben Stelle.
