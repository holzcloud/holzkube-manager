# Phase 5: Streaming — Summary

**Executed:** 2026-09-11
**Status:** complete

## Der Entry-Blocker, und warum er einer war

Phase 2 hat notiert statt umgangen: keiner der drei
`ResponseWriter`-Wrapper in der Middleware-Kette implementierte `Unwrap`,
und `WriteTimeout` ist prozessweit. Eine SSE-Route auf dieser Kette hätte
**lautlos gepuffert** — dieselben Bytes, dieselbe Reihenfolge, nur am Ende.
Genau deshalb steht im Vermerk „und das scheitert still".

Das hat eine Konsequenz für die Tests dieser Phase, die wichtiger ist als
ihr Umfang: sie fahren durch die **echte** Kette, und die Behauptung ist
nicht „die Frames sind korrekt", sondern „sie kommen an, während der
Handler noch läuft". Ein Test, der den Handler direkt gefahren hätte, wäre
gegen genau den kaputten Zustand grün gewesen.

Gebaut sind zwei Zeilen und eine Regel:

- Alle drei Wrapper implementieren `Unwrap()` — **nur** `Unwrap`, keine
  handgeschriebenen `Flush`- und `Hijack`-Methoden. So ist die Fähigkeit
  exakt die der echten Verbindung, und es gibt keine Nachbildung, die
  subtil falsch sein kann.
- Die Stream-Route löscht den prozessweiten Write-Deadline **für ihre
  Verbindung**. Er ist für argon2id und das Login-Ratelimit bemessen und
  sagt über einen Stream nichts.
- `Route.Streaming` ist deklarativ wie `Destructive`, und `httpapi.New`
  **panickt zur Kompositionszeit**, wenn eine Route beides ist: das
  Sudo-Glied hält eine Antwort zurück, und eine zurückgehaltene Antwort ist
  kein Stream.

Die Sudo-Kante ist benannt statt überlassen: `heldResponse.Unwrap` ist
implementiert, weil die Alternative schlechter ist (eine still verlorene
Fähigkeit statt einer ausgesprochenen Regel), und die Regel wird dort
durchgesetzt, wo sie durchsetzbar ist.

## Der Handel, den ein begrenzter Puffer erzwingt

`streamhub` blockiert beim Fan-out **nie**. Der Publisher ist eine Goroutine,
die von einem Talos-Node liest, und ein Browser-Tab, das aufgehört hat zu
lesen, darf einem Node keinen Gegendruck machen.

Was es dafür verliert, verliert es sichtbar — und die Richtung ist die
nicht-offensichtliche: **bei vollem Puffer gewinnt das neueste Ereignis**,
das älteste in der Schlange fällt weg. Andersherum wäre ein langsames Panel
ein Replay, das nie aufholt, weil jedes neue Ereignis am selben Punkt
weggeworfen wird.

Jedes verlorene Ereignis wird gezählt und als `gap` zugestellt. Ein Client,
der das nicht zeichnet, zeigt ein Log mit einem unsichtbaren Loch — jemandem,
der mit diesem Log einen Ausfall diagnostiziert. Das erzeugt selbstsichere
falsche Schlüsse und ist schlimmer als gar kein Log.

## Eine Verbindung pro Tab

Browser deckeln gleichzeitige Verbindungen pro Origin bei sechs, und eine
Node-Detailseite mit kubelet, etcd, apid und dmesg ist schon vier davon. Das
siebte Panel verbände sich still nie.

Deshalb ist die `id` jedes Frames der **ganze Cursor-Satz** und nicht die
Position eines Topics: SSE gibt dem Client genau einen String zum
Zurückgeben, und eine Per-Topic-id würde einen Stream korrekt fortsetzen und
die übrigen still abschneiden.

## Der Verbindungszustand ist ein Ereignis im Stream

„Es kommt nichts" hat vier Ursachen — der Dienst ist ruhig, wir verbinden
neu, der Node rebootet, der Node ist weg — und jede verlangt etwas anderes
von dem, der liest. Ein Stream aus reinen Logzeilen kann keine davon
unterscheiden. Also sind sie gewöhnliche Ereignisse auf demselben Strom, in
Reihenfolge zu den Zeilen um sie herum. Ein Zustand, der außerhalb des
Stroms reiste, käme an, wann er ankäme, und „reconnecting" stünde über
Zeilen, die älter sind.

## Erfolgskriterien

| # | Stand |
|---|---|
| 1 | **erfüllt** — Logs und `dmesg` live in der UI, Panels pro Dienst aus der eigenen Diensteliste des Nodes |
| 2 | **erfüllt** — eine Verbindung trägt bis zu 12 Topics; der Reconnect spielt über `Last-Event-ID` je Topic korrekt nach, geprüft durch die echte Kette |
| 3 | **erfüllt** — `TestASlowSubscriberNeverBlocksThePublisher` und `TestFallingBehindProducesAVisibleGap`; die Lücke ist im Panel ein Strich mit Zahl |
| 4 | **erfüllt** — vier benannte Zustände, als Ereignisse im Strom, im Panel-Kopf und in der Zeile |

## Was die Schnittlinie bedeutet

Diese Phase ist die designierte Schnittlinie. Beim Kürzen bliebe
`internal/streamhub` erhalten — Topics, Ringpuffer, nicht-blockierendes
Fan-out —, weil JOB-04 in Phase 6 darauf reitet; gestrichen würden
`internal/nodestream`, die SSE-Route und die Panels. Der Schnitt ist sauber:
`streamhub` weiß nicht, was es trägt.
