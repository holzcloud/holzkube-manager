# Phase 9: Upgrades & etcd-Verwaltung — Summary

**Executed:** 2026-09-11
**Status:** gebaut und gegen `talossim` ausgeführt; kein Upgrade auf echter Hardware (Fenster 85)

## Die Asymmetrie, aus der alles folgt

Ein etcd mit drei Membern überlebt den Verlust eines. Den Verlust von zweien
überlebt es nicht — und das ist kein langsamer Cluster, sondern ein
stehengebliebener, aus dem kein Befehl ohne Snapshot herausführt. Der zweite
Verlust ist nicht schlimmer als der erste, er ist von anderer Art.

Deshalb **verweigert** das Health-Gate, statt zu warnen, und deshalb läuft es
vor **jedem** Node neu statt einmal pro Lauf: der zweite Node wird in einem
Cluster heruntergefahren, den der erste gerade verändert hat. Ein Gate, das
einmal am Anfang lief, könnte genau das nicht sehen.

Die Reihenfolge der Verweigerungen ist selbst eine Aussage: Alarm, dann
unerreichbarer Member, dann Stimmenzahl, dann Raft-Rückstand, dann fehlender
Leader. Jede ist ein Zustand, in dem der *nächste* Ausfall der letzte wäre.

## Ein Learner ist kein Wähler

Der Befund, der wie ein Erfolg aussieht: ein Cluster mit drei Membern, von
denen einer ein Learner ist, meldet überall drei Member — und hat zwei Stimmen.
Wer den Learner mitzählt, nimmt den zweiten von zwei Wählern herunter und
glaubt dabei, den ersten von dreien zu nehmen.

Die Gegenrichtung steht auch als Test: den Learner selbst zu upgraden ist
erlaubt, weil er das Quorum nicht berührt. Es zu verweigern würde einen
Cluster, der gerade einen Member aufnimmt, für die Dauer des Aufholens
unupgradebar machen.

## „Die API sagte OK" ist kein Beweis

Der Upgrade läuft über `LifecycleService.Upgrade` — den Stream —, nicht über
das deprecated unäre `MachineService.Upgrade`. Der Unterschied ist nicht
kosmetisch: das unäre kehrt zurück, sobald der Node die Anfrage angenommen hat,
also geht alles verloren, was der Installer danach sagt. Und alles, was sagt,
was schiefging, sagt der Installer danach.

Das Ende des Streams ist der Node, der **geht**, nicht der Upgrade, der
gelingt: ein Node, der in das gerade Geschriebene neustartet, und ein Node, der
umgefallen ist, sehen von hier identisch aus. Entschieden wird es durch einen
getrennten Read danach — Version, Schematic und die eigenen Dienste des Nodes.
Ein Node auf der richtigen Version mit dem falschen Image ist genau der Fall,
für den dieser Read existiert.

`talossim` musste das erst verdienen: sein Upgrade ändert, was `Version`
meldet, und zwar erst nach dem Stream. Ein Simulator, der plausible Zeilen
streamt und die Version in Ruhe lässt, ließe eine Verifikation durch, die nie
hinsieht.

## Es gibt kein „latest"

Keine Route nimmt es an, kein Bildschirm bietet es an. „Latest" ist ein
Versprechen, das dieser Server nicht halten kann: die neueste Version kann zwei
Minors entfernt sein, Talos springt keinen Minor, und ein Knopf, der still zwei
Upgrades hintereinander macht, macht das zweite gegen einen Cluster, den
niemand angesehen hat — mit dem Health-Gate dazwischen, das am meisten zählt.

Was an seine Stelle tritt, ist die **Kette**: jeder Zwischen-Minor, benannt,
mit dem Satz, warum er da ist. Für jeden Zwischenschritt der neueste verfügbare
Patch, weil ein in einem Patch behobener Fehler aus einem Grund behoben wurde.

## Zwei Guards hatten Löcher, und beide Male dasselbe

`allowlist_test.go` führte eine **eigene Kopie** der Routen-Tabelle. Die
Phase-9-Routen wurden in `main` eingetragen und nicht im Test — also lief der
Guard über jede Route außer den neuen und war grün. Jetzt gibt es eine
`routeTable`-Funktion, und der Test ruft sie auf. Neun weitere auditierte
Aktionen stellten sich daraufhin als ohne Allowlist-Eintrag heraus.

Die Budget-Tabelle hatte **gar keine** Vollständigkeitsprüfung: eine Route mit
Upstream-Aufrufen und ohne Zeile komponierte zu, was immer sie komponierte. Der
neue Guard fand sofort zwei Routen aus Phase 3 — Cluster anlegen und
Talosconfig —, die nie eine Zeile hatten.

Derselbe Fehler zweimal: ein Guard mit einer eigenen Kopie dessen, was er
bewacht, wird genau dann still, wenn etwas hinzukommt.

Dasselbe galt für `talossims` Coverage-Guard, der auflöste, welche Bezeichner
einen Machinery-Client halten — **pro Datei**. Das Feld ist in `client.go`
deklariert, also fanden zwei neue Dateien voller Aufrufe darüber keine Holder
und meldeten gar keine Call-Sites. Auch der fiel nicht um, er wurde still. Nach
der paketweiten Auflösung: 24 statt 14.

## Drei echte Defekte

**Das Gate-Preview verschluckte seinen Fehler** und ließ das Null-Verdict
stehen, das sich als ein Gate rendert, das weder bestanden noch verweigert hat.
Ein Gate, das nicht ausgewertet werden konnte, ist eine Verweigerung.

**Die eigene Absage eines Nodes** landete auf den etcd-Routen als
`internal.unexpected`, das laut Vertrag kein Detail trägt. Sie bedeutet fast
immer eines — etcd läuft auf dem gefragten Node nicht —, und dieser Satz ist es
wert, in einem Archiv zu stehen, das alles für immer behält.

**Die Kubernetes-Version eines neuen Nodes** kam in Phase 8 aus einer
Konstante. Sie kommt jetzt aus dem Cluster selbst, und zwar die *niedrigste*,
die irgendein Node meldet: ein Cluster mitten im Upgrade hat zwei, und die
höhere setzt ein Kubelet vor eine Control Plane, die noch nicht dort ist.

## Der Snapshot-Fallback steht auf dem Schirm

Ein Snapshot über die etcd-API ist eine konsistente Kopie und braucht ein
Quorum. Ein Cluster ohne Quorum kann das nicht bestätigen, also scheitert der
Aufruf dort. Was bleibt, ist die Datenbankdatei des Members selbst — ein
Snapshot dessen, was *ein* Member glaubte. Das ist, was man zurückspielt, wenn
es nichts Besseres gibt, und es ist nicht dasselbe.

Der Satz steht in der Fehlermeldung beider Pfade, weil bei einem Server-Stream
die Absage beim ersten Read ankommt und nicht beim Öffnen.

## Erfolgskriterien

| # | Stand |
|---|---|
| 1 | **erfüllt** — rollendes Talos-Upgrade über `LifecycleClient.Upgrade` (streaming), ein Job-Schritt pro Node; das Gate läuft in jedem Schritt neu, zeigt seine Eingaben, schließt Learner aus, prüft Raft-Konvergenz und Alarme und verweigert bei ≤2 stimmberechtigten Membern; gesperrte Nodes werden übersprungen statt den Lauf zu beenden |
| 2 | **erfüllt** — das Kubernetes-Gate blockiert mit konkreter Handlungsanweisung statt zu warnen; die Kette der Zwischen-Minors wird berechnet; kein „latest"-Knopf und Pre-Releases sind gefiltert; Kubernetes-Upgrades laufen über denselben Pfad, dieselbe Gate-Auswertung und dieselbe Verifikation |
| 3 | **erfüllt** — unbekannte `schematic_id` blockiert den Node und wird vom Node gelesen statt geraten; Kernel-Args-Drift wird als Mengenvergleich erkannt und blockiert den Ein-Klick-Pfad, mit beiden Listen auf dem Schirm |
| 4 | **erfüllt** — nach jedem Node wird gegen den Node verifiziert (Version, Schematic, eigene Dienste) statt gegen den Rückgabewert; der Betreiber kann nach dem aktuellen Node stoppen, weil jeder Node ein eigener Job-Schritt ist und der Cancel an der Schrittgrenze greift |
| 5 | **erfüllt** — Member mit Hostname und UUID, nie nur mit Hex-ID; Member-Entfernung mit Quorum-Guard davor; Snapshots mit dokumentiertem Fallback ohne Quorum; Node-Entfernung als etcd-leave → reset → forget, mit dem ehrlichen Satz, dass cordon/drain nicht stattfindet |

## Was nicht verifiziert ist

Kein Upgrade ist je auf echter Hardware gelaufen. Fenster 85 führt, was das
konkret offen lässt: ob `LifecycleService.Upgrade` in Talos v1.13 die Form hat,
gegen die hier gebaut wurde, ob ein echter Node nach einem Upgrade in der
erwarteten Zeit zurückkommt (`ReappearBudget` ist wie Phase 8s eine geratene
Zahl), und ob der Kernel-Args-Vergleich gegen eine echte Kommandozeile das
Richtige tut — die Liste der Talos-eigenen Argumente ist kuratiert und nie
gegen einen echten Node gehalten worden.
