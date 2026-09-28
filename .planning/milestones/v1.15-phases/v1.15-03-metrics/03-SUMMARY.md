# v1.15 Phase 3: Prometheus-`/metrics` — Summary

**Executed:** 2026-09-12
**Status:** alle vier Kriterien erfüllt
**Requirements:** V2-API-02

## Die Grenze, die der Rückstau gezogen hat

**Exportieren, nie ingesten.** Dieses Produkt wird keine Monitoring-Pipeline,
hält keine Zeitreihe und alarmiert nicht. Es veröffentlicht die Handvoll Zahlen,
die es als einziges kennt — wie viele Nodes in welcher Stage stehen, wie lange
das Client-Zertifikat eines Clusters noch hat, wie viele Jobs wie geendet sind,
wie viele etcd-Stimmen bestätigt sind, und ob die Audit-Kette hält — und
irgendwessen Prometheus macht den Rest.

Neun Metrik-Familien, keine Abhängigkeit dazugekommen: das Textformat ist
zwanzig Zeilen Schreiben, und selbst zu schreiben ist der Grund, warum jede
Zeile `HELP` und `TYPE` trägt statt sie von einer Registry zu erben.

## Drei Regeln, die die Ausgabe formen

**Kein Node wird je ein Label-Wert.** Ein Label pro Node multipliziert jede
Serie mit der Flotte, und die Flotte ist die Achse, die wächst. Es ist außerdem
die falsche Frage: gegraphed wird „wie viele Nodes sind unten", und „welcher"
beantwortet das Dashboard. Die Kardinalität hier ist eine Funktion der
**Cluster**-Anzahl und **gar nicht** der Node-Anzahl — der Test misst genau das:
ein Cluster mit 3 Nodes und einer mit 300 exportieren dieselbe Anzahl Serien,
und jeder weitere Cluster kostet dieselbe feste Zahl.

**Eine Serie, die auf null fällt, wird trotzdem geschrieben.** Prometheus kann
eine Serie, die aufgehört hat zu existieren, nicht von einem Target
unterscheiden, das weg ist. Ein Graph „Nodes unten", der einfach endet, wenn die
Zahl null erreicht, zeigt den letzten Wert ungleich null, solange irgendjemand
hinsieht — auf dem Dashboard, das für den Ausfall existiert, nach dem Ausfall.
Also wird jede Stage für jeden Cluster geschrieben, Nullen inklusive, und jeder
Job-Zustand für jede Art, die überhaupt Jobs hat.

**Nichts hier ist ein Gauge, der eine Zeitreihe ersetzt.** Kein
`last_job_duration`, kein `last_scrape_seconds`. Eine einzelne Zahl, die bei
jedem Scrape überschrieben wird, beantwortet „was war es, als zuletzt jemand
hingesehen hat" — genau die Frage, die Prometheus abschaffen soll. Ein Test
lehnt Namen mit `last_`, `_latest` und `_duration_seconds` ab.

## Der Cluster-Name steht in genau einer Serie

`holzkube_cluster_info{cluster,name,origin} 1` ist das idiomatische Info-Metric:
der Wert ist immer 1, die Labels tragen die Beschreibung, und eine Query joint
dagegen, wenn sie den menschenlesbaren Namen will. Ohne das wäre die Wahl
zwischen einer Cluster-ID, die niemand wiedererkennt, und einem Namen, der auf
**jeder** Serie wiederholt wird.

Ein Cluster-Name ist Freitext eines Betreibers, also wird er escaped. Ein nicht
escaptes Anführungszeichen darin würde nicht nur diese Serie beschädigen: es
beendet die Zeile früh, und alles, was der Scraper danach liest, ist Unsinn —
ein schlecht benannter Cluster nimmt den ganzen Export mit. Ein Test schickt
`he said "hello"\nand C:\went\home` durch.

## Zwei Entscheidungen an der Route

**Keine Session.** Prometheus schickt ein nacktes GET auf einem Timer und kann
sich nicht anmelden. Ein Metrics-Endpunkt hinter dem Session-Cookie ist einer,
den niemand scrapen kann — und die übliche Folge ist ein **zweiter, komplett
unauthentifizierter Listener auf einem anderen Port**, was strikt schlechter
ist. Was ihn schützt, ist, was alles schützt: die Hosts-Allowlist in der äußeren
Kette und eine Bind-Adresse, die Loopback ist, wenn der Betreiber nichts anderes
gesagt hat. Ein Test schickt einen fremden `Host` und erwartet 403.

**Kein Audit-Action.** Die Route ändert nichts und wird alle fünfzehn Sekunden
für immer angefragt; sie zu protokollieren hieße, ein Archiv, das D-16 ewig
aufhebt, mit der Tatsache zu füllen, dass ein Scraper gescrapt hat.

Und: nur eine **bestätigte** etcd-Mitgliedschaft zählt. Ein Node, dessen
etcd-Level stale ist, ist ein Node, dessen *letzte bekannte* Antwort
„Mitglied" war. Ihn mitzuzählen meldet ein Quorum, das diese Instanz gerade
nicht sieht — und das ist die eine Zahl, bei der eine hoffnungsvolle Antwort
schlechter ist als keine, weil sie jemanden dazu bringt, das Mitglied zu
entfernen, das den Cluster getragen hat.

## Ausgeführt, nicht nur getestet

Der Daemon wurde gestartet und `/metrics` gescrapt. Die Antwort ist
`200 text/plain; version=0.0.4`, und **ein echter Prometheus-Parser** hat sie
gelesen: neun Familien, jede mit Typ und Dokumentation. Was ein
handgeschriebener Writer falsch macht — eine Familie zweimal deklariert, ein
Sample vor seiner `TYPE`-Zeile — hält jetzt eine Grammatik-Assertion im Go-Test
fest, weil der Parser dort nicht zur Verfügung steht.

## Erfolgskriterien

| # | Stand |
|---|---|
| 1 | **erfüllt** — `GET /metrics`, Textformat, ohne Session, an der Hosts-Allowlist (403 bei fremdem Host, 200 bei erlaubtem); keine Node-Namen in Label-Werten, geprüft gegen die echten IDs der Fixture |
| 2 | **erfüllt** — `HELP` und `TYPE` aus der Ausgabe abgeleitet statt aus einer Liste, plus Grammatik-Prüfung; verbotene Namensmuster abgelehnt |
| 3 | **erfüllt** — `holzkube_cluster_client_certificate_seconds` ist vorzeichenbehaftet; ein abgelaufenes Zertifikat liefert −5400 statt gar nichts |
| 4 | **erfüllt** — `TestCardinalityDoesNotGrowWithTheFleet`: 3 Nodes und 300 Nodes exportieren dieselbe Serienzahl, und der zweite wie der dritte Cluster kosten dieselbe Differenz |

## Was diese Phase nicht tut

Sie exportiert **keine Prozess-Metriken** (Heap, Goroutinen, GC). Die kämen aus
einer Registry, die diese Zeilen nicht braucht, und sie beantworten Fragen über
dieses Binary statt über die Flotte — und für „läuft der Prozess" gibt es `up`,
das der Scraper selbst schreibt.

Sie **alarmiert nicht** und liefert keine Regeln mit. Eine Alarmregel ist eine
Aussage darüber, was einen Betreiber nachts wecken darf, und die gehört dem
Betreiber.
