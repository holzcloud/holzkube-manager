# Phase 8: Provisioning — der Core Value — Summary

**Executed:** 2026-09-11
**Status:** gebaut und gegen `talossim` ausgeführt; Kriterium 1 unverifiziert (Fenster 82)

## Die Eintrittsbedingung ist nicht erfüllt, und das steht hier oben

Der Roadmap-Eintrag nennt zwei nicht verhandelbare Eintrittsbedingungen:
QEMU (Tier 2) nachgewiesen aus Phase 4, und eine Maschine, die blank sein
darf. **Keine von beiden ist erfüllt.** Phase 4 hat das Messgerät gebaut und
nie ausgeführt (Fenster 80 und 81); dieser Ausführungshost hat weder QEMU noch
`/dev/kvm` noch eine Maschine, die man wischen dürfte.

Die Phase ist trotzdem gebaut, weil die Alternative — auf unbestimmte Zeit
nicht zu bauen — den Core Value nicht näher bringt und weil alles außer dem
binären Abnahmetest gegen `talossim` ausführbar ist. Was nicht ausführbar ist,
ist als Fenster 82 eingetragen und **nicht** als erfüllt gemeldet.

Konkret unverifiziert bleibt: ob eine echte Maschine im Maintenance-Mode
dieselben COSI-Ressourcen unauthentifiziert herausgibt wie der Simulator, wie
lange die Installations-Stille wirklich dauert (`ReappearBudget` ist weiterhin
Phase 4s Platzhalter), und ob Talos' eigenes `AlreadyExists` beim zweiten
Bootstrap so aussieht, wie `isAlreadyExists` es erkennt.

## Vier Mechanismen gegen einen doppelten etcd-Bootstrap

Es gibt keinen einzelnen Mechanismus, weil jeder einen Fall hat, den er nicht
sieht:

1. **Pre-Flight `EtcdMemberList`.** Sieht nicht, wenn etcd nicht antwortet.
2. **`O_CREAT|O_EXCL`-Lease.** Atomar im Kernel, und der einzige der vier, der
   zwei Betreiber abdeckt, die im selben Moment klicken.
3. **Fsynced Intent-Record vor dem Aufruf.** Das einzige, was einen Absturz
   mitten im Aufruf überlebt — und der Fall, den nichts rekonstruieren kann.
4. **Talos' eigenes `AlreadyExists`.** Die letzte Linie, und der einzige Fall,
   in dem sie allein greift, ist: Node bereits gebootstrapt *und* etcd
   antwortet nicht. Genau dieser Fall ist als Test geschrieben, mit beiden
   Szenarien gleichzeitig injiziert.

Und die Antwort auf Mechanismus 4 ist **kein Fehler**. Der Cluster ist in dem
Zustand, der gewollt war; ihn als Fehlschlag zu melden schickt jemanden
losreparieren, was funktioniert.

Der unklare Fall — Lease genommen, kein Outcome — geht an einen Menschen und
**nie** in einen Retry. Ein Retry dort ist genau die Operation, die alles
andere verhindert. Der Recovery-Flow verlangt ein Urteil ohne Default und eine
Notiz, weil dieser Record der einzige Bericht ist, den es je über diesen
Bootstrap geben wird.

## Der Job-Schritt ohne `Happened`

Drei der vier Provisioning-Schritte sind verifizierbar: Identität prüfen ist
ein Read, Konfiguration anwenden ist prüfbar (eine Maschine, die ihre Config
genommen hat, ist nicht mehr im Maintenance-Mode), Warten ist ein Read.

Der Bootstrap hat absichtlich **kein** `Happened`. Ein Node, dessen etcd noch
nicht gestartet ist, sieht exakt aus wie einer, der nie gebootstrapt wurde.
Eine Unterbrechung dort parkt den Job. Das ist der Grund, warum die
Park-Maschinerie aus Phase 6 überhaupt existiert.

## Was diese Runde gefunden hat

Vier Befunde, jeder repariert statt umgangen:

**Der Simulator hat gelogen.** `Options.Maintenance` setzte ein Feld in
`Identity()` und servierte weiter ein CA-signiertes Zertifikat. Eine echte
Maschine im Maintenance-Mode hat keine Cluster-PKI, signiert selbst und
verlangt kein Client-Zertifikat — und genau diese Selbstsignatur ist das
einzige, was ein Scan ohne Authentifizierung lesen kann. Ein Test gegen den
alten Simulator hätte einen Pfad geprüft, dem keine echte Maschine ähnelt.

**Der Scan erfand eine UUID.** `Dialer.Probe` gibt die Target-ID zurück, die
man hineingegeben hat; der Scan schrieb sie als `uuid` ins Ergebnis. Eine
Sonde kann ohne Verbindung keine UUID lernen. Das Feld ist weg, `Inspect`
liefert die Identität, und `known` geht jetzt über die Adresse — das, was ein
Scan tatsächlich hat.

**Eine Adresse, die TLS nicht sprach, wurde als „nichts" gemeldet.** Das ist
exakt der Fehler, um den PROV-02 geht, einen Schritt tiefer. Sie ist jetzt ein
eigener Zustand.

**Das Scan-Budget passte nicht in die Antwortfrist.** 180 s gegen 130 s
`writeTimeout` — die Budget-Tabelle aus Phase 2 hat es gefangen. Ein /22 ist
in einer Antwort nicht scanbar, also verweigert `expand` ihn: die größte
Subnetzmaske ist ein /23, und das Routen-Budget ist diese Rechnung statt einer
großzügigen Zahl.

## Der Audit-Befund aus Phase 6 und 7

Acht auditierte Aktionen — die drei Node-Aktionen, die Bestätigung, der
Job-Cancel, `config.plan`, `config.apply`, `patch.create` — hatten **keinen
Allowlist-Eintrag**. Jeder Parameter jedes Reboots, jedes Resets und jedes
Config-Applys wurde als `<redacted>` geschrieben, während `07-SUMMARY.md` und
`docs/api-contract.md` beschrieben, was in diesen Einträgen stehen würde.

Nichts ist fehlgeschlagen, weil nichts hingesehen hat: Die Allowlist scheitert
fail-closed, also sieht eine vergessene Aktion genauso aus wie eine bewusst
leer gelistete — und kein Test *innerhalb* von `internal/audit` kann die beiden
unterscheiden, weil nur die Routen-Tabelle weiß, welche Aktionen es gibt.

Alle acht sind jetzt eingetragen, und `cmd/holzkube-managerd/allowlist_test.go`
hält beide Richtungen: jede Route-Aktion hat einen Eintrag, und jeder Eintrag
wird von einer Route ausgelöst.

Die Allowlist hat dafür einen Mechanismus bekommen: ein Eintrag mit `[]`-Suffix
erlaubt eine Liste von Skalaren. `config.apply` benennt seine Patches über IDs,
und ein Eintrag, dessen IDs `<redacted>` sind, ist ein Eintrag, der sagt, dass
eine Konfiguration angewendet wurde, und nicht sagen kann, welche. Ein Feld
ohne die Markierung lehnt eine Liste weiterhin ab — `username` ist unverändert.

## Die Bestätigung ist eine eigene Route

`POST /api/v1/provision/confirm` existiert nicht aus Symmetrie: die
maschinen-bezogene Bestätigung liest die Maschine aus dem Inventar, um das
Getippte gegen ihren Hostnamen zu prüfen, und **eine Maschine, die gerade
provisioniert wird, ist nicht im Inventar.**

Getippt wird die **Install-Disk**. Ein Reset fragt nach dem Hostnamen, weil der
Hostname das ist, was ein Betreiber an der Maschine vor sich prüfen kann; eine
Maschine im Maintenance-Mode hat keinen prüfenswerten Hostnamen — er ist, was
die ISO entschieden hat — und das, was gleich zerstört wird, ist eine Disk.

## Die Kubernetes-Version kommt aus dem Cluster

Nicht aus einer Konstante in diesem Build. Ein Node, der mit einer anderen
Kubernetes-Version beitritt als der Rest des Clusters, ist ein Node, der
beitritt und dann nicht funktioniert — und das Symptom, Pods die nie
schedulen, sieht so lange nach einem Netzwerkproblem aus, bis jemand zwei
Versions-Strings vergleicht.

Gelesen wird die **niedrigste**, die irgendein Node des Clusters meldet. Ein
Cluster mitten im Upgrade hat zwei, und die höhere würde einen Kubelet vor eine
Control Plane setzen, die noch nicht dort ist. Ein Cluster, dessen Nodes noch
nie eine gemeldet haben, bekommt keine geratene Antwort, sondern den Satz.

## Erfolgskriterien

| # | Stand |
|---|---|
| 1 | **unverifiziert** — kein QEMU, kein `/dev/kvm`, keine blanke Maschine. Fenster 82. Der Pfad ist vollständig gebaut und gegen `talossim` ausgeführt; was fehlt, ist die Maschine. |
| 2 | **erfüllt** — Subnetz-Scan mit `ScanConcurrency` und manuelle Adresse; ein konfigurierter Node wird als solcher gemeldet, eine Adresse, die TLS nicht sprach, ebenfalls; beide Boot-Warnungen kommen vom Server und stehen vor dem Scan |
| 3 | **erfüllt** — UUID, MACs, Disks und Version vor dem Apply; Disk-Picker mit Größe, Modell, Seriennummer, Transport und markierter System-Disk; Quorum-Warnung gegen den Cluster, wie er sein wird; Fingerprint neben jedem Kandidaten mit dem ehrlichen Satz; UUID zweimal erneut geprüft, einmal davon im selben Atemzug wie der Apply |
| 4 | **erfüllt** — alle vier Mechanismen ausgeführt, jeder mit einem Test, der genau seinen Fall isoliert; der unklare Fall hat einen eigenen Recovery-Flow ohne Default und ohne Retry |
| 5 | **erfüllt** — Drei-Wege-Probe mit verstrichener Zeit gegen `ReappearBudget` und einem eigenen Zustand für „noch im Maintenance-Mode, das löst sich durch Warten nie"; der Job trägt den Zustand, nicht der Tab; `.machine.install.image` kommt aus derselben Schematic-ID; der CNI-Satz steht auf dem Apply-Screen, bevor der Zustand eintritt, den er erklärt |

## Was Phase 9 übernimmt

`ReappearBudget` ist weiterhin Phase 4s Platzhalter. Die Zahl zu ersetzen ist
eine Bearbeitung an einer Stelle, und die Messung braucht eine Maschine.
