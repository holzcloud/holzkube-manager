# Übergabe — Stand 2026-09-17

Dieses Dokument existiert, weil die Arbeit an einem anderen Ort weitergehen
soll. Es beantwortet die vier Fragen, die eine neue Sitzung sonst aus dem
Transkript der alten holen müsste und dort nicht mehr findet:

1. Wo steht das Projekt gerade?
2. Was ist auf der Anlage des Betreibers **jetzt** offen?
3. Wie prüft man hier etwas nach, ohne einen Browser vor sich zu haben?
4. Was kann diese Umgebung prinzipiell nicht, und was heißt „verifiziert"
   deshalb?

Es ist **datiert, nicht laufend gepflegt** — dieselbe Regel wie für den Rest
von `.planning/` (siehe `README.md` dort). Wo es einer späteren Quelle
widerspricht, gilt die spätere; wo es die einzige Quelle ist, steht es hier,
weil es sonst nirgends steht.

---

## 1. Stand

| | |
|---|---|
| Letztes Release | **v1.16.11**, veröffentlicht, linux/amd64 + linux/arm64 |
| Branch | `main` — der Betreiber hat ausdrücklich erlaubt, direkt dorthin zu arbeiten |
| Milestone | v1.16 („Omni-Parität"), Abschnitt B abgearbeitet; C und D offen |
| Ledger | **54 offen, 82 behoben, 136 gesamt** (`.planning/WINDOWS.md`) |
| Gate | `./bin/task ci` — grün, inklusive `test:layout` |

**Lesereihenfolge für eine neue Sitzung:**

1. `CLAUDE.md` — die vier Regeln, unter denen hier gearbeitet wird. Keine davon
   ist Geschmack; jede steht dort, weil sie gebrochen wurde und das Geld
   gekostet hat.
2. Dieses Dokument.
3. `.planning/MILESTONE-v1.16.md` — was in diesem Milestone gebaut ist, was
   nicht belegbar ist (C), und was eine Entscheidung des Betreibers braucht (D
   und „Was eine Entscheidung braucht").
4. `.planning/WINDOWS.md` — die offenen Einträge. Sie sind die ehrlichste
   Liste, die dieses Projekt über sich selbst führt.
5. `.planning/STATE.md` — ausführlich, aber älter als dieses Dokument.
6. `CONTRIBUTING.md` — die Entwicklerschleife für einen Menschen mit Browser.
   Abschnitt 3 unten ist die Ergänzung für eine Sitzung ohne.

**Was zuletzt gebaut wurde** (Commits `78fe00a` bis `77d7f4b`, 2026-09-17):
die Oberfläche ist telefontauglich (Schublade unterhalb `md`, Tippziele,
wischbare Tabellen), der Layout-Wächter hängt in der Kette, und Fenster 62
(Fortschrittsanzeigen) ist geschlossen — Warten sagt jetzt, was es tut, wie
lange es das schon tut, und wie lange das üblicherweise dauert.

---

## 2. Was auf der Anlage des Betreibers offen ist

Das ist der Teil, der nicht im Repository steht und ohne den die nächste
Sitzung blind anfängt. Alles hier ist vom Betreiber berichtet, nicht hier
gemessen.

### 2.1 Der Cluster ist auf dem Pi noch da

Der Betreiber hat „Forget" zweimal ausgelöst; der Cluster steht weiter im
Dashboard. Die Ursache ist bekannt und im Journal des Betreibers wörtlich
belegt:

```
DELETE /api/v1/clusters/…      status=428
GET  /api/v1/auth/oidc/sudo    status=302
GET  /api/v1/auth/oidc/callback status=403
```

428 ist richtig (die Aktion ist zerstörend und verlangt Sudo), 302 ist richtig
(Rundweg zum Identity Provider), **403 am Callback ist die Wand**: Authentik
liefert im ID-Token keinen `auth_time`-Claim, und `completeSudo` verlangt ihn,
weil ohne ihn nicht zu unterscheiden ist, ob der Provider gerade wirklich neu
authentifiziert hat oder nur eine bestehende Sitzung durchgereicht hat.
`auth_time` ist in OIDC ein **optionaler** Claim — der Provider hat nichts
falsch gemacht, er ist nur nicht konfiguriert, ihn zu senden. Ledger 134.

Seit dieser Sitzung sagen die drei Fehlschläge wenigstens nicht mehr dasselbe:
sie loggen einzeln (`oidc.no-auth-time`, `oidc.not-fresh`,
`oidc.other-identity`) und die Oberfläche zeigt für jeden einen eigenen Text,
statt ein rohes Problem-Dokument als weiße Seite (Ledger 134).

**Der Weg heraus, heute, ohne Codeänderung:** den Manager über die
**LAN-Adresse** aufrufen und mit dem **lokalen Konto** anmelden.
`Config.IsSSOOnly` entscheidet **pro Host** (`--sso-only-hosts`), nicht global
— auf einer Adresse, die dort nicht genannt ist, nimmt der Sudo-Dialog wieder
ein Passwort an. Dann „Forget this cluster" und neu importieren.

**Der Weg heraus dauerhaft:** Authentik dazu bringen, `auth_time`
auszuliefern. Streng zu bleiben statt den Claim optional zu machen, ist eine
bestätigte Entscheidung des Betreibers aus dieser Sitzung — er wollte lieber
erklärt bekommen, warum es klemmt, als eine Sudo-Bestätigung, die nichts
bestätigt.

### 2.2 Zwei Control Planes oder eine doppelt geführte?

Die Cluster-Karte las „2 nodes, 0 healthy, 0 degraded, 2 not answering", und
mindestens ein Teil davon war ein Phantom: die Adoption hielt den
Talos-Mitgliedsnamen (`cluster.Member`-ID = **Hostname**, nicht UUID) für eine
UUID und legte die schon adoptierte Control Plane ein zweites Mal an — als
Datensatz, der per Konstruktion nie antworten konnte (Ledger 131). Behoben.

**Ob der Betreiber zwei echte Control Planes hat oder eine doppelt geführte,
entscheidet erst der nächste Import.** Das ist keine offene Frage an ihn, es
ist eine Messung, die noch aussteht.

### 2.3 Kleinkram, der jemandem gehört

- `tmp-ref-probe` liegt noch auf `origin`. Es ist der Beleg dafür, dass diese
  Sitzung keine Refs löschen kann (403), und genau deshalb kann sie es auch
  nicht wegräumen. Der Betreiber (oder irgendein Token mit Schreibrecht auf
  Refs) löscht es in zwei Sekunden.
- Das Draft-Release **v1.16.0-beta.1** ist durch v1.16.11 überholt und sollte
  weg. Gleiche Begründung.

---

## 3. Wie man hier etwas nachmisst

`CONTRIBUTING.md` beschreibt die Schleife für einen Menschen mit Browser. Was
folgt, ist dieselbe Sache für eine Sitzung ohne — **gemessen am 2026-09-17,
nicht aus dem Quelltext gelesen**.

### 3.1 Daemon starten, Konto anlegen, anmelden — ohne Browser

```sh
D=$(mktemp -d)
./bin/holzkube-managerd --insecure-http --listen 127.0.0.1:18443 \
    --data-dir "$D" --log-level warn &
# Auf /api/v1/system/status pollen, nicht schlafen -- die Route antwortet vor
# jeder Anmeldung. Gemessen: 2181 ms bis zur ersten 200, einmal aber auch nach
# 3 s noch kein Connect. Ein festes sleep ist die Sorte Flake, die man sich
# selbst baut.
until curl -sf -o /dev/null http://127.0.0.1:18443/api/v1/system/status; do sleep 0.1; done

curl -sS -X POST http://127.0.0.1:18443/api/v1/setup \
  -H 'Content-Type: application/json' -H 'X-Holzkube-Manager-CSRF: 1' \
  -d '{"username":"handover-check","password":"a-long-enough-passphrase-for-the-check-1"}'
# -> 201 {"id":"…","username":"handover-check"}

curl -sS -c /tmp/jar -X POST http://127.0.0.1:18443/api/v1/auth/login \
  -H 'Content-Type: application/json' -H 'X-Holzkube-Manager-CSRF: 1' \
  -d '{"username":"handover-check","password":"a-long-enough-passphrase-for-the-check-1"}'
# -> 204, Session im Jar

curl -sS -b /tmp/jar http://127.0.0.1:18443/api/v1/auth/me
# -> {"id":"…","username":"handover-check","role":"admin","dry_run":false,"sso":false}
```

Vier Dinge, an denen es sonst scheitert und die nirgends sonst stehen:

- **`X-Holzkube-Manager-CSRF` muss dran sein**, auch beim Setup und auch über
  `curl`. Der Wert ist egal, die Anwesenheit nicht. Ohne ihn: 403
  `csrf.precondition-unmet`, „missing X-Holzkube-Manager-CSRF header".
- **Benutzername 3–64 Zeichen, Passwort mindestens 12.** Zu kurz gibt 400
  `validation.failed` mit `errors[]` pro Feld — die Feldnamen stehen drin, also
  nachlesen statt raten.
- **Die Sitzung liegt im Cookie**, es gibt keinen Header-Token für diesen Weg.
  Wer einen Bearer-Token will, nimmt einen Service-Account (Phase 4 von v1.16).
- `--insecure-http` loggt eine Warnung, dass Session-Cookies `Secure` sind und
  ein Browser sie über `http` nicht sendet. Für `curl` und für Playwright auf
  `127.0.0.1` ist das folgenlos; der Layout-Wächter fährt genau so.

### 3.2 Die Oberfläche messen

```sh
./bin/task test:layout          # baut das Binary und fährt den Wächter
npm --prefix web run test:layout  # nur der Wächter, Binary muss stehen
```

`web/scripts/layout-audit.mjs` startet den Daemon selbst in einem Wegwerf-Ordner
auf einem vom Kernel gewählten Port, legt ein Konto an, meldet sich an
(`#login-username` / `#login-password`) und fährt zehn Routen bei 390 px und
1280 px ab. Rot wird er, wenn ein Element rechts aus dem Viewport ragt **und
kein scrollbarer Vorfahr es zurückholt**.

Knöpfe: `HOLZKUBE_BINARY` (Standard `../bin/holzkube-managerd`),
`PLAYWRIGHT_CHROMIUM` (sonst `/opt/pw-browsers/chromium`, falls vorhanden,
sonst das, was Playwright selbst installiert hat).

**Was er nicht misst: Tippzielgrößen.** Er misst Erreichbarkeit, nicht
Bedienbarkeit mit einem Daumen. Siehe Abschnitt 5.

### 3.3 Das Tor

`./bin/task ci` fährt die Tore in der Reihenfolge, in der CI sie fährt: Web-Lint,
Web-Tests, Build, Go-Lint, Go-Tests, Layout. **Ein lokal grüner Lauf ist kein
CI-Lauf** — der Data Race aus Fenster 97 ist lokal nie aufgetreten. Das gilt
weiter.

---

## 4. Was diese Umgebung nicht kann

Drei harte Grenzen. Alle drei sind gemessen, keine geraten.

- **Kein arm64.** Der Container ist x86_64, ohne qemu-user und ohne
  binfmt_misc. Ein arm64-Binary lässt sich hier nicht ausführen — nicht
  langsam, gar nicht. Jede End-to-End-Prüfung in der Geschichte dieses
  Repositories ist deshalb am **amd64**-Artefakt gelaufen, während der
  Betreiber auf einem **Pi** startet. „Das Release ist verifiziert" ohne die
  Architektur dazuzusagen, war noch nie eine Aussage über das, was er
  tatsächlich startet. Ehrlich prüfbar für arm64: Archiv vorhanden, Prüfsumme
  stimmt, enthält `holzkube-managerd`, `file` sagt ARM aarch64. Mehr nicht.
- **Kein Tag-Push.** `git push origin refs/tags/v…` antwortet 403, ein
  Branch-Push geht. Deshalb nimmt der Workflow den Tag als Dispatch-Eingabe und
  legt ihn mit seinem eigenen Token an. `.claude/skills/release/SKILL.md` hat
  die vollständige Prozedur.
- **Kein Ref-Löschen.** Gleiches 403. Ein falsch herausgegebener Tag bleibt; die
  Korrektur ist ein neuer Tag, nie ein Untag. `tmp-ref-probe` ist der Beleg.

---

## 5. Wo die offene Arbeit steht

### V2-UI-01 (Telefon)

Gebaut: der Rahmen (Schublade, Griff, Backdrop, Padding), die Tippziele
(`max-md:` auf Knopf, Eingabefeld, Checkbox), wischbare Tabellen mit
Ansage, und der Wächter in der Kette. Abgeschnitten 22 → 0, nutzbar 118 → 358 px.

Offen und benannt:

- **Zehn Bedienelemente liegen bei 390 px noch zwischen 24 und 43 px.** Es sind
  überwiegend Icon-Knöpfe in dichten Listen; 44 px ist die Zielgröße.
- **Der Wächter misst Erreichbarkeit, nicht Größe.** Eine Messung der
  Tippzielgrößen als eigener Wächter fehlt. Ohne sie verschwindet die Größe
  beim nächsten Feature genauso still, wie die Erreichbarkeit es getan hat.
- **Ob eine Tabelle auf dem Telefon zu Karten werden soll**, ist eine
  Gestaltungsfrage. Wischen ist unschön und nicht kaputt.
- **Die Erwartungswerte fürs Warten leben im `localStorage` eines Browsers.**
  Also pro Gerät verschieden, und auf einem frischen Gerät zunächst gar nicht
  da. Das ist der Preis dafür, dass der Server diese Wartezeit nicht kennt —
  eine geschätzte Zahl wäre der Preis dafür gewesen, gar nichts zu sagen.

### Milestone v1.16

Abschnitt C (`.planning/MILESTONE-v1.16.md`) ist gebaut und hier nicht
belegbar: PXE, ARM64/SBC, SideroLink, KMS-Verschlüsselung, Workload-Proxy. Es
fehlt jeweils Hardware oder eine Netzumgebung, kein Code.

Abschnitt D und „Was eine Entscheidung braucht" sind **keine Arbeit, sondern
drei offene Entscheidungen des Betreibers**: Cloud-/Infrastruktur-Provider,
ein Kubernetes-Client (ohne den sind Manifest-Sync und Workload-Proxy nicht
baubar), und SAML. Sie stehen dort ausformuliert mit Kosten und Nutzen — was
noch fehlt, ist die Vorlage als Auswahl, nicht die Analyse.

### Ledger

54 offen. Die vier, die gemeinsam schließen oder gar nicht, beschreiben einen
einzigen Durchlauf auf echter Hardware: **82** (Provisioning-Abnahme), **84**
(die erzeugte MachineConfig als Anweisung), **85** (Upgrade) und **87**
(OPS-05). Das ist der größte einzelne Block und er hängt an einer Maschine,
nicht an einer Tastatur.

---

## 6. Die Regeln, die hier Geld gekostet haben

Sie stehen in `CLAUDE.md` und werden hier nicht wiederholt, sondern nur mit dem
Preis versehen, den sie hatten:

- **Ein Wächter ist nichts wert, bis er am absichtlich wieder eingebauten
  Fehler rot geworden ist.** In dieser Sitzung haben zwei Wächter bestanden,
  während sie nichts prüften: einer akzeptierte den Backtick und übersah
  deshalb einen Wert in einem Doku-Kommentar, einer zählte eine Phrase, die
  dreimal pro Zeile vorkommt. Beide fielen nur auf, weil der Fehler zurückgebaut
  wurde. Und zweimal war „die Injektion hat nichts injiziert" das eigentliche
  Ergebnis — ein grüner Lauf danach ist keine Messung.
- **Entscheidungen gehen als Auswahl an den Betreiber, nie als offene Frage.**
  Der Betreiber hat diese Regel in dieser Sitzung zweimal einfordern müssen,
  beide Male für die letzten zwei Sätze einer langen Nachricht.
- **`declaration-not-use`** ist die Gattung, die hier dreimal zugeschlagen hat:
  eine Deklaration existiert und niemand ruft sie. So fehlten der Löschknopf für
  einen Cluster, der zweite Weg ins Inventar und der Rundweg zum Identity
  Provider — jedes Mal war die API da und kein Bildschirm rief sie.
- **Was der Betreiber auf echter Hardware sieht, findet Fehler, die hier keine
  Suite findet.** Fünf der acht Ledger-Einträge dieser Sitzung stammen aus
  seinen Screenshots und seinem Journal, nicht aus einem Testlauf.
