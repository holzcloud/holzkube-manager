# Phase 7: Config-Domain — Summary

**Executed:** 2026-09-11
**Status:** complete

## Warum Redaction und View in einer Phase liegen

Constraint #6 sagt es, und das Ergebnis dieser Phase gibt ihm recht: **die
View ist das Leck.** Eine MachineConfig trägt den privaten Schlüssel der
Cluster-CA, also ist „den Config eines Nodes ansehen" dieselbe Fähigkeit wie
„den Cluster übergeben" — es sei denn, jeder Ausgang geht durch eine
Redaction-Funktion.

Gebaut ist es so, dass die Redaction **vor** der View steht statt hinter
demjenigen, der daran denkt: `internal/machineconfig` gibt redigierte
Ansichten heraus, und die Handler haben nichts anderes zu serialisieren. Es
gibt kein `?raw=true`, keinen Schalter und keinen Endpunkt dahinter.

Zwei Durchgänge, weil sie in verschiedene Richtungen versagen:

- machinerys eigenes `RedactSecrets` kennt das Schema, gegen das dieser Build
  übersetzt wurde. Ein künftiges Talos-Release mit einem neuen
  geheimnistragenden Feld würde davon nichts entfernen.
- Ein Sweep nach PEM-Privatschlüsselblöcken kennt kein Schema und fängt einen
  Schlüssel in einem Feld, das dieser Build nie gesehen hat.

Ein Test schmuggelt genau das ein — ein zusätzliches Dokument mit einem
Schlüssel in einem erfundenen Feld — und prüft, dass er trotzdem verschwindet.
Ein weiterer prüft den Fall, der sonst ein als Fehler verkleidetes Leck wäre:
eine Config, die *nicht parst*, bekommt trotzdem den zweiten Durchgang und
wird mit einem Fehler daneben zurückgegeben. Sie zu verweigern würde das
Problem verstecken; sie roh zu zeigen würde den Schlüssel herausgeben.

Der Entropie-Walk aus CFG-02 läuft über alle fünf Ausgänge. Ein Test über
einen davon wäre ein Test der Funktion, nicht der Behauptung — und der
Fehler, um den es geht, ist genau der fünfte Ausgang, den jemand hinzufügt,
ohne durch sie zu gehen.

## Das Diff ist strukturell

Ein Text-Diff zweier YAML-Dokumente meldet Umsortierung und Einrückung als
Unterschiede, und eine Liste, die von drei auf vier Einträge wächst, als
„eine Zeile mehr". Das ist genau die Form, die ein zweimal angewandter
Strategic-Merge-Patch erzeugt — und das zweite Anwenden ist das, das niemand
wollte.

Also sind zwei Befunde benannt statt im Rauschen: **Listen-Wachstum** mit
beiden Längen, und **Duplikate** nach dem Merge. Der Plan sagt dasselbe einen
Schritt früher: `idempotent: false` heißt, dass zweimal anwenden anders ist
als einmal.

Listen werden als Ganzes verglichen und nicht elementweise. Elementweise
würde „Element 2 hat sich geändert" melden, wenn vorne eines eingefügt wurde
— das Irreführendste, was ein Config-Diff sagen kann.

## Der Apply-Modus wird berechnet

Aus den geänderten Pfaden gegen eine kuratierte Whitelist. Was nicht darauf
steht, gilt als reboot-pflichtig: die konservative Richtung, weil die andere
eine Änderung ist, die Erfolg meldet und nichts tut.

Drei Fälle sind namentlich herausgehoben:

- **`.machine.install`** steht bewusst *nicht* auf der No-Reboot-Liste,
  obwohl das Anwenden nichts neustartet. „Kein Reboot nötig" läse sich als
  „wirkt jetzt", und es wirkt erst beim nächsten Install oder Upgrade.
- **`.machine.network`** bekommt `try` und den sichtbaren Countdown. Eine
  falsche Netzwerkänderung macht den Node unerreichbar, und einem
  unerreichbaren Node kann man nicht sagen, dass er sie rückgängig machen
  soll. Die Zahl auf dem Schirm ist dieselbe Konstante wie die auf dem Node.
- **Ein zweiter `staged`-Apply** wird abgelehnt. Talos nimmt ihn an und
  ersetzt den ersten still — der Betreiber hat zwei Änderungen gestaged und
  genau eine passiert, ohne dass irgendwo steht, welche.

Die Staged-Sperre ist prozesslokal und im Speicher, und das steht so in der
Antwort statt versteckt zu sein: ein gespeichertes Flag wäre eine Behauptung
über den Node, die nur der Node beantworten kann, und sie würde in dem Moment
falsch, in dem jemand den Node außerhalb von holzkube neustartet.

## Patches

Ausschließlich Strategic Merge. RFC 6902 adressiert Listeneinträge über den
Index, und ein Index ist eine Behauptung über eine Liste, wie sie zufällig
war, als der Patch geschrieben wurde — angewandt auf einen Node mit einer um
eins längeren Liste ändert er den falschen Eintrag und meldet Erfolg. Die
Ablehnung hat ihren eigenen Code, damit ein Client „repariere diesen Patch"
von „nimm die andere Form" unterscheiden kann.

Append-only: ein Edit schreibt eine neue Version und markiert die alte als
abgelöst. Der alte Body bleibt lesbar, weil „was genau wurde im März auf
diesen Node angewendet" nur dann eine Antwort hat, wenn das Angewandte noch
existiert.

## Erfolgskriterien

| # | Stand |
|---|---|
| 1 | **erfüllt** — gerendert und roh, und der Entropie-Walk über alle fünf Ausgänge ist grün, einschließlich des eingeschmuggelten Schlüssels in einem unbekannten Feld |
| 2 | **erfüllt** — strukturelles Diff der tatsächlich gemergten Dokumente, mit Listen-Wachstum und Duplikat-Erkennung als benannten Befunden |
| 3 | **erfüllt** — Modus aus der Whitelist berechnet; `.machine.install` als „wirkt erst beim nächsten Install" gekennzeichnet; zweiter `staged`-Apply mit `409` abgelehnt; `.machine.network` empfiehlt `try` mit sichtbarem 60-Sekunden-Countdown |
| 4 | **erfüllt** — versioniert, append-only, nur Strategic Merge; Idempotenz vorher geprüft; Validierung im Plan |
| 5 | **erfüllt** — Generierung aus dem in Phase 3 abgeleiteten Bundle, mit pro Cluster gepinntem Versions-Contract, und beide Auslassungen werden benannt abgelehnt |

## Der Audit-Eintrag, und was bewusst fehlt

`config.apply` protokolliert Cluster, Modus und die **Patch-IDs**. Die
Patch-*Bodies* stehen absichtlich nicht auf der Allowlist: ein Body ist
beliebige Konfiguration, Konfiguration ist der Ort der Secrets, und das
Archiv hat keinen Löschpfad. Ein gespeicherter Patch ist unter seiner ID
lesbar, solange er existiert — also für immer —, sodass der Eintrag
vollständig ist, ohne dass das Archiv die Bytes hält.
