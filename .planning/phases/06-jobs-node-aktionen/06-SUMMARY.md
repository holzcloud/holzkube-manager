# Phase 6: Jobs-Engine & Node-Aktionen — Summary

**Executed:** 2026-09-11
**Status:** complete

## Der eine Satz

**Ein `kill -9` mitten in einem Schritt darf nie einen doppelten
Seiteneffekt erzeugen.** Alles in dieser Phase ist, was dieser Satz kostet.

Ein Schritt ist deshalb keine Funktion, sondern eine Funktion plus eine
gepaarte Nur-Lese-Frage: *ist das schon passiert?* Ein Schritt ohne diese
Frage ist unprüfbar, und eine Unterbrechung in ihm **parkt** den Job.

Geparkt ist kein Fehlermodus dieser Engine. Es ist ihre wichtigste Ausgabe.
Die Maschine kann nicht wissen, was passiert ist, und beide Vermutungen sind
falsch auf eine Weise, die zählt: ein wiederholter Reset ist ein zweites
Wipe; ein als gescheitert gemeldeter ist ein Knoten, der als intakt gilt und
es nicht ist.

Die drei Zweige stehen als Tests und sind grün:

| Fall | Ausgang | Schritt lief |
|---|---|---|
| prüfbar, und die Frage sagt „ja" | übersprungen | 1× |
| prüfbar, und die Frage sagt „nein" | erneut ausgeführt | 2× |
| unprüfbar | **geparkt**, mit einem Satz für einen Menschen | 1× |

Dazu der Aufrüstfall: ein gespeicherter Job, dessen Art dieses Binary nicht
mehr kennt, wird geparkt statt mit fremden Schritten gefahren.

## Was beim Bauen auffiel

Zwei Dinge, beide repariert statt umgangen, und beide erwähnenswert, weil
sie die Sorte Fehler sind, die sonst erst im Betrieb sichtbar wird.

**Der Cancel-Konflikt.** Ein Cancel schreibt aus einem HTTP-Handler in
denselben Record und hebt dessen Revision. Die laufende Kopie war danach
veraltet, ihr nächster `Put` scheiterte am CAS, und der Job hätte nie wieder
einen Zustand geschrieben — ein gecancelter Job, der ewig läuft. `save` ist
jetzt ein Read-Modify-Write mit klarer Eigentumsteilung: die Engine besitzt,
*wo* der Job ist; alles andere kommt aus dem gespeicherten Record.

**Die fehlende Simulator-Methode.** `talossim` implementierte `SystemStat`
nicht, also war die Reboot-Prüfung („ist der Knoten kürzer oben als die
Anfrage alt ist?") gegen den Simulator nicht ausführbar.
`TestMethodCoverage` aus Phase 2 hat das gefangen, wofür es gebaut wurde —
und die Lösung war, den Simulator wachsen zu lassen, nicht die Aufrufstelle
zu entfernen.

## Bestätigung ist serverseitig, und sie ist an die Parameter gebunden

Ein Dialog im Browser schützt gegen einen Fehlklick und gegen sonst nichts:
alles, was die API erreicht, kann ihn überspringen. Also stellt der Server
ein Token aus, das **genau** die Aktion beschreibt, für die es eine
Bestätigung ist, und die destruktive Route baut diese Beschreibung aus dem
tatsächlich Gesendeten neu und vergleicht.

Die Eigenschaft, auf die es ankommt, ist nicht „ein Token lag vor", sondern:
ein Token für `wipe_mode=user-disks` kann `wipe_mode=all` **nicht**
autorisieren. Was der Betreiber gelesen hat und was der Server tut, sind
dieselbe Aktion — oder die Anfrage wird abgelehnt.

Der Schlüssel liegt nur im Speicher. Eine Bestätigung, die einen Neustart
überlebte, wäre eine Entscheidung über eine Flotte, die sich seither
geändert haben kann.

## Der Reset-Dialog

`talosctl reset` wischt per Default jede Disk und lässt die Maschine aus.
Ein Betreiber, der dieses Werkzeug kennt, nimmt dasselbe hier an, wenn ihm
niemand widerspricht — also ist nichts darauf vorausgewählt, und der
Unterschied steht auf dem Schirm.

Die Wipe-Bereiche kommen **am wenigsten destruktiv zuerst**: eine Liste,
deren erste Option die Maschine wischt, ist eine Liste, durch die jemand
klickt. Getippt wird der Hostname, nicht „DELETE" — ein generisches Wort
bestätigt, dass jemand tippen kann, nicht dass er weiß, welche Maschine das
ist. Und getippt wird nur beim Reset: es vor jedem Reboot zu verlangen ist,
wie man jemandem beibringt, es ungelesen einzufügen.

## Erfolgskriterien

| # | Stand |
|---|---|
| 1 | **erfüllt** — Resume-oder-Park in allen drei Zweigen getestet, gegen den echten fsstore; kein Schritt läuft zweimal |
| 2 | **erfüllt** — Reboot und Shutdown aus der UI, Fortschritt über denselben Hub wie die Logs, abbrechbar an der Schrittgrenze |
| 3 | **erfüllt** — Disks, Wipe-Scope, beide Flags mit ihrer Wirkung im Klartext, Hostname tippen |
| 4 | **erfüllt** — Confirmation-Token serverseitig und an die Parameter gebunden; `202` plus Job-ID auf jedem destruktiven Endpoint; Audit-Eintrag mit den Parametern im Klartext und ohne das Token |
| 5 | **erfüllt** — eine Lease je Cluster, geprüft im Engine-Test und über HTTP |

## Der Rest des Handels

Ein fehlgeschlagenes Speichern des Fortschritts ist geloggt und nicht fatal.
Das ist eine bewusste und unbequeme Wahl: der Job läuft bereits, und ihn
wegen eines Buchhaltungsfehlers halb fertig liegen zu lassen wäre schlimmer.
Was es kostet, ist, dass die Crash-Argumentation oben nur so gut ist wie der
letzte erfolgreiche Schreibvorgang — weshalb der Fehlschlag laut geloggt
wird.
