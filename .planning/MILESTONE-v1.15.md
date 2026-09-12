# Milestone v1.15 — was ohne Hardware tatsächlich besser wird

**Angelegt:** 2026-09-12
**Angelegt von:** Claude, auf die Anweisung „entwickle weiter ohne mich"
**Status:** definiert, nicht vom Betreiber bestätigt

## Warum dieses Dokument zuerst kommt

Der Betreiber hat gesagt, es seien „noch viele Milestones offen". Das trifft
den Stand nicht, und die Korrektur ist der Anfang dieser Planung: es gibt
**einen** Milestone (v1.14, gebaut bis auf OPS-05) und einen V2-Rückstau von
zehn Punkten — von denen einer, OIDC (V2-AUTH-01), in Phase 2 schon geliefert
wurde, bevor ihn jemand für v2 vorgesehen hatte.

Es gibt also keine Warteschlange, die man abarbeitet. Es gibt eine Auswahl, und
weil ich sie ohne Rückfrage treffe, steht sie hier mit Begründung statt nur im
Code.

## Die Auswahlregel

Alles, was echte Hardware braucht, ist ausgeschlossen — nicht weil es unwichtig
ist, sondern weil es hier nur *geschrieben* und nicht *belegt* werden könnte,
und das Ergebnis wären weitere Fenster wie 82 und 85. Der Milestone v1.14 hat
genug davon.

Was bleibt, ist sortiert nach dem, was ein Betreiber am nächsten Vorfall merkt.

## Phasen

### Phase 1: Support-Bundle-Export (V2-OPS-01)

**Goal:** Wenn etwas kaputt ist, gibt es *eine* Datei, die man sich ansehen
oder weitergeben kann.

Das ist der höchste Wert pro Zeile in diesem Rückstau, und der Grund ist die
Form des Problems: ein Betreiber mit einem kaputten Cluster sammelt heute
Node-Facts, Service-Listen, Logs, `dmesg`, etcd-Status und die Config von Hand
über mehrere Screens, während er unter Druck steht. `talosctl support` macht
genau das für Talos; dieses Produkt hat alle Bausteine und keinen Knopf.

**Die Entscheidung, die es riskant macht, und die Antwort darauf:** ein
Support-Bundle ist per Definition ein Archiv mit allem, und „allem" schließt
Secrets ein. Die Redaction-Maschinerie aus Phase 7 existiert bereits
(`machineconfig.Redact`, zwei Durchgänge, fünf Ausgänge) — das Bundle geht
durch sie, und ein Test, der Entropie über das gesamte Archiv walkt, ist die
Abnahmebedingung. Ein Bundle, das man nicht verschicken kann, ist kein
Support-Bundle.

**Requirements:** V2-OPS-01
**Success Criteria:**

  1. Ein Bundle über einen Cluster enthält pro Node: Facts, Service-Liste,
     Talos- und Kubernetes-Version, Disks, Links, Extensions, etcd-Status und
     die **redigierte** MachineConfig — plus den Audit-Tail und die
     Instanz-Metadaten.
  2. Ein Node, der nicht antwortet, erzeugt einen Eintrag, der sagt *dass* und
     *warum* er fehlt — nie eine leere Datei und nie einen abgebrochenen Lauf.
     Ein Bundle über einen toten Cluster ist genau das Bundle, das jemand
     braucht.
  3. Ein Entropie-Walk über **jede Datei** im Archiv findet keinen privaten
     Schlüssel und kein PEM-Block-Fragment. Getestet mit einem eingeschmuggelten
     Schlüssel in einem Feld, das dieser Build nicht kennt.
  4. Das Bundle ist ein Subkommando **und** eine Route. Das Subkommando läuft
     auf dem Host ohne Session; die Route lädt es herunter, weil der Betreiber
     im Vorfall im Browser ist und nicht auf der Konsole.
  5. Die Größe ist begrenzt und die Grenze ist benannt: Logs werden am Ende
     abgeschnitten, mit einer Zeile, die sagt, wie viel fehlt.

### Phase 2: COSI-Watches statt Heartbeat (schließt Fenster 79)

**Goal:** Eine Änderung an einem Node ist sofort sichtbar, nicht nach ~22
Sekunden.

Fenster 79 hält fest, dass INV-13 und D-19 „Watch primär, Poll als Heartbeat"
verlangen und gebaut ist nur der Heartbeat. Die Schließbedingung, die dort
steht, ist entweder Watches oder eine ausdrückliche Entscheidung, dass der
Heartbeat für 5–20 Nodes die richtige Auflösung ist.

Ich schließe es mit Watches und nicht mit der Entscheidung, weil die
Entscheidung dem Betreiber gehört und der Code sie nicht braucht: die
Antwortform (`health.Field[T]`) ist dieselbe, die ein Watch füllt, also ist der
Tausch clientseitig unsichtbar — was Phase 3 schon als Begründung dafür
angeführt hat, dass der Tausch später möglich bleibt.

**Requirements:** INV-13, D-19
**Success Criteria:**

  1. Eine Änderung an einer beobachteten COSI-Ressource erscheint in der View,
     ohne auf den Heartbeat zu warten; gemessen gegen `talossim`.
  2. Der Heartbeat bleibt und wird **nicht** durch den Watch ersetzt. Ein Watch,
     der stirbt, ohne es zu sagen, ist der Fehler, gegen den der Heartbeat
     existiert — und ein Poller, der wegfällt, sobald ein Watch läuft, macht
     genau diesen Fehler unsichtbar.
  3. Ein abbrechender Watch führt zu einem sichtbaren Zustand und einem
     Wiederaufbau mit Backoff, nicht zu einem stillen Stillstand.
  4. Die Stage-Maschine unterscheidet weiterhin „bestätigt" von „stale", und
     ein Watch, der noch keinen Snapshot geliefert hat, gilt nicht als
     bestätigt.

### Phase 3: Prometheus-`/metrics` (V2-API-02)

**Goal:** Der Betreiber kann die Dinge graphen, die ihn nachts wecken, ohne
dass dieses Produkt selbst eine Monitoring-Pipeline wird.

Die Grenze steht schon im Rückstau: „exportieren, nie ingesten". Was exportiert
wird, ist genau das, was dieses Produkt weiß und niemand sonst hat — die Anzahl
Nodes pro Stage, die Restlaufzeit der Cluster-Zertifikate, die Job-Ergebnisse
nach Art, die etcd-Stimmenzahl, und ob die Audit-Kette hält.

**Requirements:** V2-API-02
**Success Criteria:**

  1. `GET /metrics` liefert Prometheus-Textformat, ohne Session, gebunden an
     die Hosts-Allowlist wie alles andere — und es enthält **keine**
     Node-Namen als Label-Werte, wo eine Kardinalität pro Node vermeidbar ist.
  2. Jede Metrik trägt `HELP` und `TYPE`, und keine Metrik ist ein Gauge, der
     eine Zeitreihe ersetzen soll (kein `last_job_duration`).
  3. Die Zertifikats-Restlaufzeit ist als Sekunden exportiert und kann negativ
     sein — ein abgelaufenes Zertifikat ist ein benannter Zustand und keine
     fehlende Metrik.
  4. Ein Test hält fest, dass die Kardinalität mit der Flottengröße linear und
     nicht multiplikativ wächst.

## Was ausdrücklich NICHT in diesem Milestone ist

| Punkt | Warum nicht |
|---|---|
| **V2-OPS-02** CA-Rotation | Der Rückstau nennt es selbst „der Radius mit dem breitesten stillen Schaden im Produkt". Es ist ohne echte Cluster nicht verifizierbar, und eine CA-Rotation, die *fast* funktioniert, ist ein Cluster, den niemand mehr erreicht. Das gehört an eine Hardware-Sitzung, nicht an eine autonome. |
| **V2-AUTH-02** Mehrere Benutzer / RBAC | Die Entscheidung in `PROJECT.md` sagt: „Ein zweiter echter Benutzer wäre der Auslöser, nicht Vollständigkeit." Es gibt keinen zweiten Benutzer. Das zu bauen wäre Scope gegen eine protokollierte Entscheidung. |
| **V2-TRANS-01** Tunnel-Transport | „Falls Nodes je das LAN verlassen." Sie haben es nicht. Das Interface ist reserviert, und reserviert zu bleiben ist der Zustand. |
| **V2-PROV-01** PXE/netboot | Schreibbar (es ist ein URL-Tausch), aber ohne Netboot-Umgebung nicht belegbar — und v1.14 hat genug Fenster, die „gebaut, nie ausgeführt" sagen. |
| **V2-OPS-03** ARM64/SBC | Braucht ARM-Hardware. |
| **V2-API-01** `holzkubectl` | Phase 10 hat vier Subkommandos gebraucht und geliefert; das deckt die Operationen, die wirklich auf dem Host stattfinden. Ein vollständiges CLI ist eine zweite Oberfläche, und die Entscheidung „ein Interface pflegen" steht noch. |
| **OPS-05** Hardware-Durchlauf | Braucht die Maschine. Fenster 87. |
