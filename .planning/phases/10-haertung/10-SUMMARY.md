# Phase 10: Härtung & echter Hardware-Durchlauf — Summary

**Executed:** 2026-09-11
**Status:** Kriterien 2–5 erfüllt; **Kriterium 1 nicht erfüllt** (Fenster 87) — und damit auch OPS-05 🚫 nicht

## Die Eintrittsbedingung, noch einmal, und diesmal endgültig

Der Roadmap-Eintrag ist eindeutig: *„Ein Verifikationsdurchlauf auf echter
amd64-Hardware. … Ohne Zugriff auf das Homelab ist diese Phase nicht
abschließbar — das ist die einzige Phase, die das Homelab zwingend braucht."*

Dieser Ausführungshost hat kein Homelab, kein QEMU, kein `/dev/kvm` und keinen
Docker-Daemon. Der Durchlauf hat nicht stattgefunden und findet hier auch nicht
statt. **OPS-05 bleibt offen**, und mit ihm der Release-Blocker, den diese
Phase besitzt. Fenster 87 führt es.

Was gebaut ist, ist alles andere — und es ist so gebaut, dass der Durchlauf,
wenn er kommt, etwas zu prüfen hat statt etwas zu bauen.

## Backup und Restore sind Subkommandos desselben Binaries

Nicht ein zweites Werkzeug. Das Backup-Format, die Rechteregeln und das Hashing
der Audit-Kette leben in diesem Build; ein getrenntes Werkzeug wäre eine zweite
Implementierung von jedem davon, die nichts synchron hält. Ein Backup, das eine
Version schreibt und eine andere ablehnt, ist kein Backup.

**Ein Backup ist während des Betriebs sicher, ein Restore nicht.** Jeder Record
im Store wird atomar geschrieben — temporäre Datei, fsync, rename —, also fängt
ein Tarball mitten im Schreiben entweder den alten Record oder den neuen und nie
einen halben. Die Alternative, das Backup während des Betriebs zu verweigern,
ist ein Backup-Subkommando, das niemand ausführt.

Der Restore nimmt dagegen den Prozess-Lock des Stores, um festzustellen, ob
jemand anderes das Verzeichnis hält: unter einer laufenden Instanz zu
restaurieren würde die Dateien ersetzen, die sie offen hat, durch andere mit
denselben Namen.

## Zwei Verweigerungen vor dem Restore

**Er sichert, was er ersetzen will.** Ein Restore, der schiefgeht, ohne das
getan zu haben, hat eine funktionierende Installation durch eine kaputte
ersetzt und nichts hinterlassen, wohin man zurückkann.

**Er prüft jeden Tar-Eintrag gegen das Ziel.** Ein Pfad in einem Tarball ist,
was der geschrieben hat, der ihn gemacht hat: `../../../etc/shadow` ist ein
gültiger Eintrag, und ein absoluter Pfad auch. Geprüft wird der *aufgelöste*
Pfad und nicht der Text, weil `a/../../b` kein führendes `..` enthält und
trotzdem ausbricht.

Symlinks, Hardlinks und Geräte werden **abgelehnt**, nicht übersprungen. Ein
Symlink in einem Archiv ist ein Pfad, dem der nächste Schreibvorgang folgt, und
ihn still wegzulassen erzeugt ein restauriertes Verzeichnis, dem etwas fehlt,
wovon niemand erfährt.

Die Rechte werden auf `0700` **verengt**, nie erweitert. Ein Restore, der eine
`0644` erzeugt, erzeugt ein Datenverzeichnis, das `fsstore` danach zu öffnen
verweigert — also ein Restore, der funktioniert zu haben scheint.

## Pre-Releases sind Opt-in, und die Absage hat einen eigenen Fehler

Ein Node, der `v1.14.0-rc.2` meldet, ist **innerhalb** des Fensters und wird
trotzdem abgelehnt, solange die Instanz nicht mit `--allow-prerelease` gestartet
wurde.

Die beiden Absagen sind getrennte Fehler, weil die Abhilfen von
entgegengesetzter Art sind: „dieser Node liegt außerhalb des unterstützten
Bereichs" ist eine Aussage über den Node, „diese Instanz nimmt keine
Pre-Releases" ist eine Einstellung. Ein Client, der beide als dieselbe Absage
zeigt, schickt jemanden dazu, einen Node neu zu installieren, auf den er absichtlich
einen Release Candidate gespielt hat.

`v1.14.0+dirty` ist Build-Metadaten und kein Pre-Release — ein Release aus einem
modifizierten Baum, und es als eines abzulehnen wäre falsch über ihn.

## Die Markierung liest den Snapshot, nicht die Verbindung

Ein Node außerhalb des Bereichs wird beim Verbinden **abgelehnt**. Eine
Markierung, die von einer erfolgreichen Verbindung abhinge, wäre also genau bei
den Nodes leer, für die sie existiert. Sie kommt deshalb aus der zuletzt
gemeldeten Version im Snapshot — der überlebt, dass der Node aus irgendeinem
Grund nicht antwortet.

Ein Node, dessen Snapshot gar keine Version trägt, bleibt unmarkiert und bekommt
keinen Satz. „Wir haben von diesem Node nie eine Version gehört" ist nicht
„dieser Node ist in Ordnung".

## Der Container

Non-root (uid 65532) aus `scratch`, mit dem Datenverzeichnis als deklariertem
Volume. holzkube-manager hält Cluster-PKI: ein Container, der als root liefe und
ein world-readable Volume schriebe, legte jedes Geheimnis, das dieses Produkt
schützen soll, einen `docker cp` weit neben jeden auf dem Host.

`scratch` statt alpine oder distroless, weil das Binary statisch ist, seine
Web-Assets einbettet und Talos-Endpunkte gegen die Cluster-PKI verifiziert, die
es hält, statt gegen einen System-Truststore. Eine Shell, ein Paketmanager und
ein CA-Bundle wären je ein Weg hinein, für den es keine Verwendung gibt.

Das Image ist **nicht gebaut**: auf diesem Host läuft kein Docker-Daemon
(dasselbe Fenster 76 wie in Phase 3). Was stattdessen existiert, ist ein Test,
der die beiden Eigenschaften, die eine eilige Änderung entfernt, aus den Dateien
selbst prüft — in derselben Richtung wie `budget_drift_test.go`, das eine
`.tsx`-Datei liest.

## Settings schließt G-01-1

Die Passwort-Route und der 428-Sudo-Fluss stehen seit Phase 1, und es gab keinen
Weg, von der Oberfläche aus an einen von beiden heranzukommen. Der UAT-Bericht
aus Phase 1 hat das als Lücke G-01-1 festgehalten und hierher verschoben, weil
der Einstiegspunkt zu dem Screen gehört, den diese Phase baut.

Es gibt bewusst **keinen Backup-Knopf**. Ein Backup enthält jedes Geheimnis des
Datenverzeichnisses wörtlich, und die Datei landet auf der Platte des *Servers*,
nicht der des Betreibers. Ein Knopf erzeugte also eine Datei, die man danach
ohnehin über SSH suchen muss — und einer, der das Archiv in den Browser
streamte, wäre ein Endpunkt, der den Cluster an jeden übergibt, der eine Session
hat.

## Erfolgskriterien

| # | Stand |
|---|---|
| 1 | **nicht erfüllt** — kein Homelab, kein QEMU, kein `/dev/kvm`. Fenster 87. Dies ist die Eintrittsbedingung der Phase und der Release-Blocker OPS-05. |
| 2 | **erfüllt** — `backup`, `backups`, `restore` und `verify-audit` als Subkommandos; der Restore sichert vorher, prüft jeden Eintrag gegen das Ziel und verifiziert die Kette danach |
| 3 | **teilweise** — Dockerfile und Compose stehen mit non-root, scratch, gedroppten Capabilities und read-only Root; das Image ist nie gebaut worden (Fenster 76 gilt weiter) und „im Dauerbetrieb" ist damit unbelegt |
| 4 | **erfüllt** — der Range-Check läuft auf der Liveness-Probe jedes Client-Konstruktors, Pre-Releases sind Opt-in mit eigenem Fehler, und ein Node außerhalb wird in der Liste markiert; als Test ausgeführt, nicht als README-Satz |
| 5 | **erfüllt** — die Settings-Oberfläche trägt das Passwort-Formular; der Sudo-Dialog rendert darüber statt an seiner Stelle, also bleibt das Formular erhalten, und ein Test hält das fest |

## Was der Hardware-Durchlauf zu prüfen hätte

Nicht diese Phase allein. Die Fenster 82, 84, 85 und 87 schließen gemeinsam oder
gar nicht, und sie beschreiben zusammen einen einzigen Durchlauf: eine blanke
amd64-Maschine wird über den Wizard zu einem Node, bekommt eine Konfiguration,
wird geupgradet, und die gemessene Installations- und Wiederauftauch-Zeit
ersetzt zwei geratene Konstanten.
