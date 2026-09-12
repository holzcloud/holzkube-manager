# v1.15 Phase 2: COSI-Watches statt Heartbeat — Summary

**Executed:** 2026-09-12
**Status:** alle vier Kriterien erfüllt; **Fenster 79 geschlossen**
**Requirements:** INV-13, D-19

## Was gebaut ist

Pro Node laufen jetzt **zwei** Schleifen: die bestehende Refresh-Schleife auf
ihrem eigenen Timer und ein `WatchKind` über fünf Ressourcen-Arten (Hostname,
Adressen, Links, Disks, Extensions).

Ein Watch trägt **keine Daten** nach oben. Er sagt, *dass* sich ein Topic
geändert hat; der Beobachtungsdurchlauf, der ohnehin existiert, liest. Das ist
eine Entscheidung und keine Abkürzung: die Alternative — ein Watch, der den
Snapshot aus seinen Events zusammensetzt — wäre eine zweite Implementierung
jeder Ableitung in `facts.go`, von nichts synchron gehalten, und die erste
Abweichung wäre ein Dashboard, das sich je nach zuletzt schreibendem Pfad selbst
widerspricht.

Der Tausch ist clientseitig unsichtbar, genau wie Phase 3 es vorhergesagt hat.
Die Antwortform (`health.Field[T]`) ist unverändert; dazugekommen ist ein
Objekt `watch` pro Maschine, das sagt, *wie schnell* die nächste Änderung
sichtbar wird — nicht, ob irgendetwas darunter wahr ist.

## Die Deadline-Klasse, die es dafür brauchte

`MethodCOSIWatch` stand als `ClassStream`. Ein Stream hat kein Gesamtlimit, aber
einen `StreamIdleTimeout` von 60 Sekunden — und für einen Watch ist das falsch
herum: bei einem Log-Stream heißt Stille „der Node redet nicht mehr", bei einem
Watch heißt sie „es hat sich nichts geändert", was auf den meisten Nodes die
meiste Zeit der Fall ist. Unter der Stream-Klasse wäre jede Subscription
minütlich abgerissen und neu aufgebaut worden: ein Poller mit Umweg, auf einem
schlechteren Intervall als der Heartbeat, den er verbessern sollte.

`ClassWatch` ist deshalb die einzige Klasse **ohne** Idle-Timeout. Das
First-Byte-Limit bleibt: ein Watch wird mit Bootstrap-Inhalten geöffnet und
schuldet sofort eine Antwort.

## Drei Befunde, die erst das Ausführen geliefert hat

**1. `config.MachineConfig` ist über diesen Transport nicht beobachtbar.**

Das war das wertvollste Topic — ein Config-Apply, der zurückkommt, während der
Betreiber auf den Bildschirm schaut. Es funktioniert nicht: der
Bootstrap-Marker trägt eine *leere* Ressource der Art, und machinerys
`configloader` lehnt die mit `config not found` ab — die ganze Subscription
stirbt an der Nachricht, die sagen sollte, dass sie angefangen hat. Das ist
keine Eigenschaft des Simulators; der fehlschlagende Decode passiert im Client,
an Bytes, die der cosi-project-Server jedem gleich schickt. Ohne
Bootstrap-Inhalte zu beobachten umgeht den Decode und kauft nichts: ein Topic,
das nie einen Snapshot liefert, kann sich nie als gestartet melden, und ein
Stream, der schweigt, läuft in `StreamFirstByteDeadline`.

Die Konfiguration gehört also dem Heartbeat, und der Apply, der sie ändert, hat
seinen eigenen Job zum Zusehen — dorthin schaut ein Betreiber ohnehin.

**2. Ein Watch bemerkt einen verschwundenen Node nicht.**

Gemessen, nicht angenommen: eine Subscription gegen einen simulierten Node, der
dann gestoppt wird, bleibt offen, still und fehlerfrei. cosi-projects
Protobuf-Client baut einen abgerissenen Watch-Stream aus seinem letzten Bookmark
mit eigenem exponentiellem Backoff wieder auf und meldet nach oben **nichts**,
bis das Retry-Budget nach etwa einer Viertelstunde erschöpft ist.

Für einen Watch, der kurz die Verbindung verloren hat, ist das richtig — und
deshalb versucht nichts hier, es auszuhebeln. Für einen Node, der weg ist,
heißt es: die Subscription behauptet fünfzehn Minuten lang, lebendig zu sein,
nachdem es nichts mehr gibt, wogegen sie lebendig sein könnte. Das ist der
stille Stillstand in Reinform.

**Der Heartbeat ist deshalb jetzt der Watchdog.** Erreicht eine Maschine
`StageDown` — zwei aufeinanderfolgende gescheiterte Durchläufe —, wird ihr Watch
geschlossen statt weiter behaupten zu lassen, er sei live; die watchLoop baut
ihn mit Backoff neu auf, und der Neuaufbau scheitert sichtbar, solange der Node
weg ist.

Das ist der **konkrete** Grund, warum der Poller nicht entfernt werden konnte.
Nicht Ordentlichkeit, nicht Gürtel-und-Hosenträger: ohne ihn hat dieses Produkt
keine Möglichkeit, einen stillen Node von einem toten zu unterscheiden. Ein Test
hält die Deficiency fest, samt der Anweisung, bei einem grünen Werden nicht die
Assertion zu löschen, sondern den Entwurf noch einmal zu lesen.

**3. Zwei alte Fehler in der Supervision, beide still.**

`observationFor` legt den Beobachtungseintrag einer Maschine bei Bedarf an, und
`Start` las die Anwesenheit dieses Eintrags als „hat schon einen Supervisor".
Eine Instanz, die **eine Liste ausgeliefert hatte, bevor `Start` lief,
überwachte danach gar nichts** — jeder Node blieb auf dem Stand des letzten
Lesevorgangs, für immer, ohne Fehler irgendwo und mit einem Dashboard, das gut
aussah. Gebissen hat es nicht, weil `holzkube-managerd` `Start` vor dem Listener
aufruft.

Und `Supervise` nahm einen Context — der einzige Aufrufer, der einen hatte, war
ein HTTP-Handler, und der übergab den **des Requests**. Ein über die API
adoptierter Node bekam also einen Supervisor, der mit dem Schreiben der Antwort
gecancelt wurde.

Beide repariert, beide mit Regressionstest. Die Signatur nimmt keinen Context
mehr: die Lebensdauer eines Supervisors ist die des Service.

## Was der Simulator dazulernen musste

`SetHostname` änderte nur `nodeState.hostname` — also nur den Hostnamen im
Antwort-Envelope jedes RPCs, nicht die `network.HostnameStatus`-Ressource, aus
der jeder Leser dieses Produkts den Hostnamen tatsächlich nimmt. Ein Test hätte
damit eine Umbenennung behaupten können, die kein Produktivpfad je gesehen
hätte: der Simulator besteht einen Test, den der echte Node nicht bestehen
würde, wogegen TRANS-06 existiert. Jetzt ändert er beides.

## Erfolgskriterien

| # | Stand |
|---|---|
| 1 | **erfüllt** — `TestAChangeAppearsWithoutWaitingForTheHeartbeat`: Heartbeat auf 15 Minuten, Umbenennung über die Ressource, Ankunft im Read-Model in Sekunden |
| 2 | **erfüllt** — `TestTheHeartbeatStillRunsWhileAWatchIsLive` prüft eine Tatsache, die **kein** Topic trägt (die Talos-Version), also kann sie nur über einen Poll-Durchlauf angekommen sein. Der Heartbeat ist außerdem strukturell unentbehrlich geworden: er ist der Watchdog |
| 3 | **erfüllt** — `TestAWatchThatDiesIsVisibleAndRebuilt`: Node weg → `watch.live: false` mit benanntem Grund, `restarts` bewegt sich, Backoff-Neuaufbau von 2s bis 40s mit Jitter |
| 4 | **erfüllt** — die Initialinhalte werden nicht als Änderungen weitergereicht, der Snapshot-Marker kommt genau einmal und vor jeder Änderung, und `live` wird erst dadurch wahr. Bestätigung kommt weiterhin nur aus einem Durchlauf, der etwas gelesen hat |

## Kosten

Fünf zusätzliche Server-Streams pro Node, plus je eine Goroutine, plus Watchdog
und Closer — grob ein Dutzend Goroutinen pro Node. Bei den fünf bis zwanzig
Nodes, für die Pattern 7 dieses Produkt dimensioniert, sind das einige hundert;
das ist für Go nichts und für einen Talos-Node fünf billige Subscriptions. Einen
Schalter gibt es bewusst nicht: er kaufte eine Flotte, in der zwei Nodes
Verschiedenes bemerken und niemand sagen kann, welcher.
