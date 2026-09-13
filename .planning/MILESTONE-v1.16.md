# Milestone v1.16 — Omni-Parität

**Angelegt:** 2026-09-13
**Angelegt von:** Claude, auf die Anweisung „Behebe alle Fehler, und fahre dann
weiter mit allen offenen Milestones. Bis die Software alle Funktionen welche
talos omni hat auch hat."
**Status:** definiert, nicht vom Betreiber bestätigt
**Fortschritt:** Abschnitt B ist abgearbeitet. Phasen 1 bis 8 gebaut, Phase 9
zur Hälfte und mit dem ausdrücklichen Vermerk, welche Hälfte fehlt und warum.
Offen bleiben nur die Abschnitte C (braucht Hardware, die es hier nicht gibt)
und D (braucht eine Entscheidung des Betreibers, keine Arbeit).

## Was diese Anweisung ändert

Milestone v1.15 hatte eine Auswahlregel: „Alles, was echte Hardware braucht, ist
ausgeschlossen." Sieben V2-Punkte stehen dort mit dieser Begründung in einer
Ausschlusstabelle. Die neue Anweisung hebt diese Regel auf — das Ziel ist jetzt
ein Funktionsumfang, nicht das, was von hier aus belegbar ist.

**Die Begründungen verschwinden dadurch nicht, sie wechseln die Rolle.** Sie
waren Gründe, etwas *nicht zu bauen*; sie sind jetzt Gründe, warum etwas
*gebaut und nicht belegt* sein wird. Das ist eine Aussage über den Zustand
jedes gelieferten Punktes, und sie steht bei jeder Phase unten dabei, weil v1.14
schon genug Fenster hat, die „gebaut, nie ausgeführt" sagen, und keines davon
sagt es freiwillig.

Zwei Punkte sind davon ausdrücklich ausgenommen und stehen weiter unten unter
„Was eine Entscheidung braucht": sie sind nicht schwer zu belegen, sie stehen
quer zu einer Grenze, die dieses Projekt bewusst gezogen hat.

## Womit die Liste abgeglichen ist

Nicht mit Erinnerung. Die Grundlage ist der vollständige Dokumentationsindex von
Omni (`https://docs.siderolabs.com/llms.txt`, abgerufen 2026-09-13), also das,
was Sidero selbst als Funktionsumfang dokumentiert — 97 Seiten. Jede Zeile der
Tabellen unten ist eine Seite oder eine Seitengruppe daraus.

Seiten, die Omnis *eigenen Betrieb* betreffen (Run Omni On-Prem, Back Up Omni
Database, Upgrade Omni, Run Omni on Kubernetes, Expose Omni with Nginx,
Hardware Requirements, Lizenzfragen), sind keine Funktionen des verwalteten
Clusters. Ihr Gegenstück in holzkube ist die README und der Datenverzeichnis-
Abschnitt darin. Sie stehen nicht in den Tabellen.

## A — was holzkube schon hat

| Omni | holzkube |
|---|---|
| Import Talos Clusters | Adoption/Import mit abgeleitetem Secrets-Bundle (D-01 bis D-07) |
| Register a Bare Metal Machine (ISO) | Provisioning-Wizard, Phase 8 |
| Upgrade Omni Clusters | Talos- und Kubernetes-Upgrade, Phase 9 |
| Gate Talos Upgrades with Healthchecks | Health-Gate, INV-07/INV-08 |
| Create Etcd Backups | `EtcdSnapshot`, Phase 9 |
| Audit Logs | Hash-verketteter Audit-Pfad, D-16 |
| Monitor Omni With Prometheus | `/metrics`, v1.15 Phase 3 |
| Support Bundle | `holzkube-managerd support`, v1.15 Phase 1 |
| Install Talos Linux Extensions | Schematics, Phase 2 |
| Modify Kernel Arguments | Schematics, Phase 2 |
| Create a Patch for Cluster Machines | Patch-Domain, Phase 7 |
| Wipe a Machine | Reset mit Tippbestätigung, Phase 6 |
| Image Factory Configuration | `--image-factory`, heute |
| Talos Config Overrides / Override NTP Servers | Patches, Phase 7 |
| Authentication (OIDC) | V2-AUTH-01, in Phase 2 geliefert |
| Break Glass Emergency Access | Talosconfig-Download, Phase 3 |

## B — was fehlt und von hier aus belegbar ist

Das ist der Arbeitsvorrat dieses Milestones. Reihenfolge nach dem, was ein
Betreiber am nächsten Vorfall merkt.

### Phase 1: etcd-Restore (Omni: „Restore Etcd of a Cluster") — GEBAUT

Die README sagt heute ausdrücklich „It takes etcd snapshots and does not restore
them", und das ist die größte einzelne Lücke im Produkt: ein Backup, das nicht
zurückgespielt werden kann, ist eine Datei, kein Wiederanlauf.

Braucht `MachineService/EtcdRecover` (ein Client-Stream — der erste in diesem
Produkt) und `Bootstrap` mit `recover_etcd`. talossim muss beide implementieren,
und zwar so, dass sie den Zustand des simulierten Knotens wirklich ändern, sonst
belegt der Test nur, dass ein RPC abgesetzt wurde (siehe Fenster 3 und 4).

**Belegbar hier:** die Mechanik gegen talossim, die Refusals, der Job-Verlauf.
**Nicht belegbar hier:** dass ein echter Cluster danach wieder läuft.

### Phase 2: kubeconfig (Omni: „Use Kubectl With Omni", „Create a Kubeconfig for a Kubernetes Service Account") — GEBAUT

`MachineService/Kubeconfig` ist ein Server-Stream, der ein tar-Archiv liefert.
Ein Betreiber, der einen Cluster über diese Oberfläche adoptiert hat, hat heute
keinen Weg von hier zu `kubectl`, außer über `talosctl` — also über genau das
Werkzeug, das dieses Produkt ersetzen soll.

**Belegbar hier:** vollständig, gegen talossim.

### Phase 3: Mehrere Benutzer und Rollen (Omni: „Manage Users in Omni", „Access Policies (ACLs)") — GEBAUT

`PROJECT.md` hält die Gegenentscheidung fest: „Ein zweiter echter Benutzer wäre
der Auslöser, nicht Vollständigkeit." Die neue Anweisung ist dieser Auslöser,
und die alte Entscheidung wird damit überschrieben statt vergessen — sie steht
weiter dort, und dieser Absatz ist, was sie ablöst.

Umfang: Benutzerverwaltung, Rollen (mindestens Reader/Operator/Admin), und die
Frage, was eine Rolle auf *Cluster*-Ebene bedeutet. Der Audit-Pfad schreibt
schon heute den Handelnden mit, also gibt es die Hälfte davon bereits.

**Belegbar hier:** vollständig.

### Phase 4: Service-Accounts (Omni: „Create an Omni Service Account") — GEBAUT

Ein nicht-interaktives Zugangsmittel für Automatisierung, mit eigener Identität
im Audit-Pfad und eigener Rolle. Folgt aus Phase 3 und ist ohne sie sinnlos.

**Belegbar hier:** vollständig.

### Phase 5: Maschinen-Labels und Maschinen-Klassen (Omni: „Set Initial Machine Labels", „Create a Machine Class") — GEBAUT

`model.Machine` hat heute kein Label-Feld. Labels sind die Grundlage, auf der
Omni Cluster zusammenstellt: eine Maschinen-Klasse ist ein Label-Selektor, und
eine Cluster-Definition nennt Klassen statt einzelner Maschinen.

**Belegbar hier:** vollständig.

### Phase 6: Cluster-Templates (Omni: „Introduction to Cluster Templates", „Export a Cluster template", Referenz) — GEBAUT

Eine deklarative YAML-Beschreibung eines Clusters, anwendbar und aus einem
bestehenden Cluster exportierbar. Baut auf Phase 5 auf.

**Belegbar hier:** die Übersetzung Template → Aktionen und der Export. Dass ein
so beschriebener Cluster entsteht, hängt an Phase 8 von v1.14 und damit an
Hardware.

### Phase 7: `holzkubectl` (Omni: „omnictl CLI", „Manage Omni Resources with omnictl") — GEBAUT

V2-API-01. Die Ausschlussbegründung von v1.15 war „ein vollständiges CLI ist
eine zweite Oberfläche, und die Entscheidung ‚ein Interface pflegen' steht
noch". Die Entscheidung steht immer noch, und die neue Anweisung überschreibt
sie — mit der Einschränkung, dass das CLI ein Client der bestehenden REST-API
ist und keine zweite Implementierung von irgendetwas. Das ist der einzige
Zuschnitt, in dem „ein Interface pflegen" und „ein CLI haben" beide wahr sind.

**Belegbar hier:** vollständig.

Gebaut als `cmd/holzkubectl`: eigenes Binary, kein eingebettetes Frontend,
Authentifizierung ausschließlich über einen Service-Account-Token. Kein Flag,
das die Zertifikatsprüfung abschaltet — `HOLZKUBE_FINGERPRINT` pinnt wie D-03
es für Knoten tut, und zwar über `VerifyConnection`, weil Go
`VerifyPeerCertificate` bei einer wiederaufgenommenen TLS-Sitzung überspringt
(dieselbe Falle, die `internal/talos/pin.go` beschreibt; ein Test stellt zwei
Anfragen hintereinander, weil die erste allein nichts belegt).

Was dabei gelernt wurde und den Rest des Milestones betrifft: **ein Client, der
einen Schlüssel dekodiert, den die API nicht sendet, fällt nicht auf.**
`encoding/json` lässt das Feld auf dem Nullwert stehen und meldet nichts, also
stand in der Spalte NODES eine 0 und unter STEPS `0/3` — beides sah aus wie eine
Antwort. Zwei von fünf Dekodier-Strukturen waren so falsch. Dagegen steht jetzt
`cmd/holzkubectl/wire_test.go`, das jeden JSON-Namen des CLI gegen den
Servertyp hält, der ihn erzeugt. Derselbe Fehler: das Plan-Route nimmt YAML,
das CLI schickte `application/json`, und es funktionierte nur, weil nichts
davor hinsieht.

### Phase 8: Cluster skalieren (Omni: „Scale a Cluster Up or Down") — GEBAUT

Knoten hinzufügen gibt es (Provisioning); Knoten entfernen gibt es als
`node.remove-from-cluster`. Was fehlt, ist die Sicht darauf als eine Operation
auf dem Cluster statt als zwei Operationen auf Knoten — einschließlich der
Frage, die Omni hier beantwortet und holzkube nicht: wann ein
Control-Plane-Knoten *nicht* entfernt werden darf.

Die Antwort auf genau diese Frage gab es nirgends, und das war schlimmer als
eine fehlende Ansicht: **`node.remove-from-cluster` hat die etcd-Mitgliedschaft
überhaupt nicht gelesen.** Die Weigerung existierte — `ErrLastVotingMember`,
mit Problem-Code und Handler-Mapping — und hing an `RemoveMember`, einer
anderen Funktion. Der Knopf auf der Knotenseite ging direkt zu
`EtcdLeaveCluster` und wischte dann die Systemplatte. Zwischen einem
Ein-Knoten-Cluster und einem Wipe stand nur der Bestätigungsdialog, und ein
Bestätigungsdialog fragt, ob jemand das gemeint hat, was er schon geklickt hat;
er weiß nicht, was es kostet. Dazu kam, dass die Weigerung selbst falsch war:
`Tolerates == 0 && VotingCount > 1` schloss den einen Fall aus, aus dem es
keine Reparatur gibt.

Gebaut: `internal/scale` als reines Lesemodell (erreicht nichts, rechnet nur),
Route `GET /api/v1/clusters/{id}/scale`, ein Panel pro Cluster-Karte, das erst
auf Nachfrage etcd liest, und `holzkubectl scale`. Jede Weigerung kommt aus
`upgrade.RefuseIfCannotSpareAVoter` — derselben Funktion, die auch die Route
ruft — und wird unverändert durchgereicht. Eine Oberfläche mit eigener Lesart
der Regel sieht richtig aus, bis jemand klickt.

Die Arithmetik ist der eigentliche Inhalt: eine Mehrheit von *n* ist *n/2+1*,
also verträgt eine gerade Anzahl stimmberechtigter Mitglieder genau so viel wie
die ungerade darunter. Vier fühlen sich sicherer an als drei und sind es nicht.
Das ist der Fehler, den Betreiber machen, und er sieht aus wie Vorsicht.

Teilweise vorhanden: `upgrade.ErrLastVotingMember` stellt genau diese Frage
schon für Upgrades.

**Belegbar hier:** die Entscheidungslogik. Der Vollzug hängt an Hardware.

### Phase 9: CA-Rotation (Omni: „CA Rotation") — HALB GEBAUT, UND DAS STEHT SO DA

V2-OPS-02. Der V2-Rückstau nennt es selbst „der Radius mit dem breitesten
stillen Schaden im Produkt".

**Belegbar hier:** die Mechanik gegen talossim und jede Ablehnung.
**Nicht belegbar hier:** dass ein echter Cluster die Rotation überlebt. Dieser
Satz ist kein Vorbehalt, sondern die Beschreibung des Auslieferungszustands.

Die Phase zerfällt in zwei Operationen, die in Omni unter einem Namen stehen
und hier nicht dasselbe Risiko haben:

**Erneuern des eigenen Client-Zertifikats — gebaut.** Das ist die Hälfte, an
der die Leiter aus D-23 seit ihrem Bestehen zählt: „The client certificate for
homelab expires within a week. When it does, every node in this cluster becomes
unreachable at once." Es gab nichts zu klicken. Ein Countdown auf eine Tür, die
es nicht gibt, ist schlimmer als kein Countdown — er bringt einem Betreiber
bei, dass die Warnungen auf diesem Bildschirm nichts zum Handeln sind. Das
Erneuern fasst **keinen Knoten an**: ein Knoten vertraut der Autorität, nicht
einem bestimmten daraus ausgestellten Zertifikat. Das neue wird gegen einen
Knoten bewiesen, bevor es gespeichert wird; schlägt das fehl, bleibt das alte
liegen und nichts hat sich geändert. Diese Reihenfolge *ist* die Operation.

**Rotation der Autorität selbst — nicht gebaut, und das steht in der README.**
Sie ändert, wem jeder Knoten vertraut: neue Autorität in die akzeptierte Menge
jedes Knotens, Konfiguration ausrollen, ausstellende Autorität umschalten,
nochmal ausrollen, alte entfernen. Vier Durchläufe über jede Maschine, wobei
ein Abbruch in der Mitte einen Cluster hinterlässt, der zwei Autoritäten
vertraut, und eine falsche Reihenfolge einen, der keiner vertraut. Gegen
talossim ginge das zu bauen; grün wäre es dann auch. Aber grün gegen einen
Simulator und nie gegen Blech ist bei genau dieser Operation keine Aussage,
auf die jemand handeln sollte — die Ausschlussbegründung aus v1.15 („eine
CA-Rotation, die *fast* funktioniert, ist ein Cluster, den niemand mehr
erreicht") bleibt wahr, auch wenn die Entscheidung, sie nicht zu bauen,
überschrieben wurde. Was stattdessen da ist: die Ablehnung benennt den Fall.
Wer extern rotiert hat, bekommt beim Erneuern „the certificate authority in
this installation's store is no longer the one the cluster trusts" statt eines
stillen Fehlschlags.

## C — was gebaut, aber hier nicht belegt werden kann

| Omni | Warum nicht belegbar |
|---|---|
| Register a Bare Metal Machine (PXE/iPXE) | Keine Netboot-Umgebung. Der Bau ist ein URL-Tausch gegen die PXE-Frontend-URL der Factory (V2-PROV-01). |
| SideroLink / Join-Token / Machine Registration über Tunnel | Kein WireGuard-Pfad, keine Knoten außerhalb des LAN (V2-TRANS-01). |
| ARM64/SBC | Keine ARM-Hardware (V2-OPS-03). |
| Omni KMS Disk Encryption | Braucht Knoten mit verschlüsselten Datenträgern. |
| Expose a Workload via Service Proxy | Braucht einen laufenden Cluster mit Workloads. |
| Rotate SideroLink Join Token, Revoke Kubernetes Access Tokens | Folgen aus Tunnel bzw. kubeconfig. |

## D — Infrastruktur-Provider und Cloud

Omni registriert Maschinen bei AWS, Azure, GCP und Hetzner, hat ein
Provider-Interface zum Selberschreiben, und bindet Cluster-Autoscaler und
Karpenter an. Das ist der größte einzelne Block im Index und der einzige, der
gegen `PROJECT.md` läuft: dieses Produkt ist für Blech im eigenen Netz gebaut,
läuft **außerhalb** des Clusters und spricht die Talos machine API direkt.

Ich baue davon nichts, ohne dass der Betreiber es sagt — nicht weil es zu groß
ist, sondern weil „alle Funktionen von Omni" hier zum ersten Mal etwas anderes
heißt als „dasselbe Produkt, mehr können": es hieße Cloud-Anmeldedaten in einem
Prozess, der heute keine hat.

## Was eine Entscheidung braucht

**Sync Kubernetes Manifests** und **Workload-Proxy** brauchen einen
Kubernetes-Client. Fenster 86 hält dieselbe Frage schon für das
Kubernetes-Upgrade fest: holzkube schreibt die Images in die MachineConfig und
spricht nicht mit Kubernetes, weil ein zweiter Transport gegen die Grenze „nur
die Talos machine API" läuft. Diese beiden Punkte sind ohne einen solchen
Client nicht baubar, und ihn zu bauen ist eine Architekturentscheidung und kein
Feature.

**SAML** ist acht Seiten im Omni-Index. holzkube hat OIDC. Ob SAML dazukommt,
ist eine Frage danach, welchen Identitätsanbieter der Betreiber hat, und nicht
danach, was Omni kann.

**Terraform-Provider** ist ein eigenes Repository mit eigenem Lebenszyklus.

## Reihenfolge und Abhängigkeiten

```
Phase 1 (etcd-Restore)     unabhängig
Phase 2 (kubeconfig)       unabhängig
Phase 3 (Benutzer/Rollen)  unabhängig
Phase 4 (Service-Accounts) braucht 3
Phase 5 (Labels/Klassen)   unabhängig
Phase 6 (Templates)        braucht 5
Phase 7 (holzkubectl)      braucht 3 und 4 (ein CLI ohne Token ist ein Cookie-Jar)
Phase 8 (Skalieren)        braucht 5
Phase 9 (CA-Rotation)      unabhängig
```

## Was beim Bauen anders kam als geplant

**Phase 1** hat eine Abweichung von der bestaetigten Deadline-Policy noetig
gemacht (Fenster 98): EtcdRecover ist dort als Mutation gefuehrt und ist jetzt
ClassUpload, weil die Policy nie einen hochladenden Aufruf zu bedenken hatte.
Die Mutations-Klasse begrenzt laut eigenem Doc „the call that *initiates* a
mutation" -- dieser initiiert nichts und passt in kein Paket.

**Phase 2** hat talossim.Options um `Bootstrapped` erweitert. Der Default
modelliert einen frischen Knoten, den der Provisioning-Pfad gerade installiert
hat; das ist nicht der Knoten, den der Adoptions-Pfad trifft. Ein importierter
Cluster laeuft per Definition schon.

**Phase 3** war groesser als geplant, und zwar an der richtigen Stelle: nicht
die drei Rollen, sondern dass jede der 65 Session-Routen ihre Mindestrolle
einzeln nennt und eine Route ohne Rolle den Prozess am Start abbricht. Die
Alternative -- ein Default -- waere genau die Sorte Entscheidung gewesen, die
dieses Repository sonst nirgends trifft.

**Phase 4** hat zwei Tore geoeffnet, die beide begruendet sind und keines davon
bequem: CSRF und das Sudo-Fenster lassen einen Bearer-Token durch. Die erste
Fassung der Sudo-Behauptung im Test hat nichts geprueft (sie benutzte den
per-Node-Lock, der absichtlich NICHT Destructive ist); die CSRF-Ausnahme war
ueberhaupt nicht wirksam, bis sie als Praedikat an die Kompositionswurzel kam.
Beides gefunden, indem der Fehler absichtlich wieder eingebaut wurde.

## Die Regel, unter der das gebaut wird

Sie ist dieselbe wie bisher und steht hier, weil dieser Milestone lang genug
ist, dass sie sonst verrutscht: **ein Wächter ist nichts wert, bis er gegen den
absichtlich wieder eingebauten Fehler rot gewesen ist.** Drei Wächter in der
vorigen Sitzung und zwei in dieser haben bestanden, während sie nichts prüften.
