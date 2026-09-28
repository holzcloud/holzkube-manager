# Phase 13: Host-Aktionen über einen root-eigenen Helfer - Context

**Gathered:** 2026-09-28
**Status:** Ready for planning

<domain>
## Phase Boundary

Auf `/host` stehen vier Aktionen — Host neu starten, Host herunterfahren,
Dienst holzkube-manager neu starten, jetzt nach Updates suchen. Der Daemon führt
keine davon selbst aus: er legt nach Sudo-Fenster, getipptem Hostnamen,
Rollenprüfung und Audit einen Auftrag im Datenverzeichnis ab; eine root-eigene
systemd-Path-Unit startet ein festes Skript, das genau diese vier Aufträge kennt
und alles andere verwirft und protokolliert. Ist der Helfer nicht installiert,
sind die Aktionen gesperrt und die Seite sagt, was aus `deploy/` wohin gehört.
`deploy/` liefert Units, Skript und Anleitung — installiert wird nichts.

Requirements: HACT-01 … HACT-08.

**Nicht diese Phase:** kein Rollback als Aktion (Future Requirement), keine
beliebigen Befehle, kein apt, keine Änderung an der Härtung der Daemon-Unit,
keine automatische Installation oder Aktualisierung des Helfers, keine
Tippziel-Arbeit über `ui/`-Bausteine hinaus (Phase 14 misst den Dialog).

</domain>

<decisions>
## Implementation Decisions

### Auftrag: Form, Ablage, Abholung (HACT-06)

- **D-01:** Ein Auftrag ist **eine Zeile**:
  `<aktion> <id>\n` mit `aktion` ∈ `reboot | poweroff | restart-service |
  update` und `id` = 16 Hex-Zeichen (Zufall aus dem Daemon). Keine JSON, kein
  Benutzername, keine Parameter: das Skript prüft mit einem einzigen
  verankerten Muster `^(reboot|poweroff|restart-service|update) [0-9a-f]{16}$`
  und einer Größengrenze (≤ 64 Byte). Wer die Aktion ausgelöst hat, steht im
  Audit-Log des Daemons, nicht im Auftrag.
- **D-02:** **Ein Platz, ein Auftrag:** die Datei
  `<datenverzeichnis>/host-order`. Der Daemon schreibt nach
  `<datenverzeichnis>/.host-order.tmp-<id>` und benennt atomar um. Liegt schon
  ein Auftrag, antwortet die Route `409` mit `conflict.host-order-pending` —
  keine Warteschlange.
- **D-03:** `holzkube-manager-host.path` beobachtet genau diese Datei
  (pegelgesteuert, `PathExists=`/`DirectoryNotEmpty=` — welches, klärt die
  Research), `holzkube-manager-host.service` (`Type=oneshot`, root) startet
  `/usr/local/sbin/holzkube-manager-host`. Das Skript **löscht den Auftrag, bevor
  es handelt** (konsumieren, dann ausführen): ein Neustart des Hosts darf den
  Auftrag nicht nach dem Boot ein zweites Mal finden, und die Path-Unit darf
  nicht in eine Schleife laufen.
- **D-04:** Das Skript liest aus einem Verzeichnis, das dem unprivilegierten
  Benutzer gehört, also defensiv: nur eine reguläre Datei, kein Symlink
  (`-f` und `! -L`, Lesen mit Größengrenze), Inhalt wird **nie** ungeprüft ins
  Journal geschrieben — ein verworfener Auftrag wird mit Länge und Grund
  protokolliert, nicht mit seinem Text (sonst liest ein kompromittierter Daemon
  über einen Symlink fremde Dateien ins Journal). In das Verzeichnis des Daemons
  schreibt das Skript nichts, es löscht nur (`rm` folgt keinem Symlink).
- **D-05:** Ergebnis: das Skript schreibt `<id> <aktion> <ergebnis> <zeit>` nach
  `/var/lib/holzkube-manager-host/last` (eigenes `StateDirectory` des
  Helfer-Service, 0755, Datei 0644; `ergebnis` ∈ `started | rejected |
  failed`). Der Daemon liest es und zeigt zur zuletzt abgelegten id, was daraus
  wurde. Ein verworfener Auftrag ist damit auf der Seite **und** im Journal des
  Helfers sichtbar.
- **D-06:** Was die vier Aufträge tun, fest im Skript:
  `reboot` → `systemctl reboot`; `poweroff` → `systemctl poweroff`;
  `restart-service` → `systemctl restart holzkube-manager.service`;
  `update` → `systemctl start --no-block holzkube-manager-update.service` —
  derselbe Unit-Lauf wie der stündliche Timer (sucht **und** installiert, wenn
  neuer), damit es genau einen Update-Weg gibt. `systemctl` ist über eine
  Variable ersetzbar, damit der Test einen Stellvertreter einsetzen kann.

### Sperren an der Route (HACT-05)

- **D-07:** Vier Routen `POST /api/v1/host/actions/{reboot|poweroff|restart-service|update}`,
  jede `RequiresSession`, `MinRole: RoleOperator`, `Destructive: true`
  (Sudo-Fenster), mit `Action`-Token `host.reboot`, `host.poweroff`,
  `host.restart-service`, `host.update` — alle vier in der Audit-Allowlist mit
  leerer Parameterliste.
- **D-08:** Getippter Hostname über eine eigene Bestätigungsroute
  `POST /api/v1/host/confirm` (`RoleOperator`, `Action: "action.confirm"` wie
  bei Knoten) nach dem Muster von `issueConfirmation`: eigene Tabelle der vier
  Host-Aktionen, **alle** mit Tipp-Pflicht; verglichen wird mit dem Hostnamen
  aus `uname(2)`. Die Aktionsroute prüft den Token gegen einen neu gebauten
  `jobs.Intent`, dessen Ziel-Feld eine Marke trägt, die keine Maschinen-ID sein
  kann — ein Token für einen Knoten-Reboot öffnet nie den Host-Reboot.
- **D-09:** Abweichung von der Knoten-Regel ausdrücklich: bei Knoten verlangt
  `typedPhrase` für Reboot/Shutdown **kein** Tippen. Beim Host doch (HACT-05 ist
  gesperrt), und es ist begründet: es gibt genau einen Host, und er ist die
  Maschine, auf der diese Oberfläche läuft — nach dem Klick ist sie weg.
- **D-10:** Vier einzeln rot gesehene Sperren (Erfolgskriterium 1): ohne
  Sudo-Fenster → 428; falscher/fehlender Hostname → kein Token; Rolle Reader →
  403; Audit-Eintrag fehlt → Test rot. Dazu der vorhandene Router-Test, der
  `Destructive` ohne `MinRole` ablehnt.

### Der Daemon führt nichts aus (HACT-06, Kriterium 2)

- **D-11:** Der Daemon schreibt eine Datei und sonst nichts. Wächter in
  `internal/depguard_test.go` (oder daneben): das Daemon-Binary darf `os/exec`
  nicht importieren (`go list -deps ./cmd/holzkube-managerd`), und ein
  Quelltext-Scan über `cmd/` und `internal/` (ohne `_test.go`, ohne
  Simulatoren) verbietet `os.StartProcess`, `syscall.ForkExec`,
  `syscall.Exec`, `unix.Exec`. Rot zu sehen gegen ein eingeschmuggeltes
  `exec.Command("systemctl", …)` in `internal/host`.

### Helfer installiert? (HACT-07)

- **D-12:** Ohne D-Bus erkennt der Daemon den Helfer an Dateien, die unter
  `ProtectSystem=strict` lesbar sind: das Skript
  `/usr/local/sbin/holzkube-manager-host` (reguläre Datei, root, ausführbar),
  die Path-Unit unter `/etc/systemd/system/` **und** ihr Aktivierungs-Symlink im
  `*.wants/`-Verzeichnis. Fehlt eines → Aktionen gesperrt, die Route antwortet
  `409 conflict.host-helper-missing` **ohne** einen Auftrag abzulegen, und die
  Seite nennt, was fehlt und die Befehle aus der Anleitung.
- **D-13:** Zweite Sicherung gegen „installiert, aber nicht gestartet": wird ein
  Auftrag nicht binnen 10 s abgeholt (Datei noch da, kein Ergebnis zur id), löscht
  der Daemon ihn selbst und meldet „the helper did not pick up the order" —
  so bleibt nie ein Auftrag liegen, der nach einem späteren Start des Helfers
  überraschend ausgeführt würde.
- **D-14:** Im Container (Phase 11 D-17) gibt es keine Host-Aktionen: die
  Knöpfe sind gesperrt mit „only available with the systemd installation", die
  Route antwortet `409 conflict.host-in-container`.

### Oberfläche

- **D-15:** Ein Aktionsblock auf `/host` im Stil von `PowerMenu`/`NodeActions`:
  vier Knöpfe, ein Bestätigungsdialog (vorhandener `ui/dialog` + `SudoDialog`),
  der sagt, was passiert („The page will lose its connection while the host
  restarts"), und das Tippfeld für den Hostnamen. Nach dem Absenden zeigt die
  Seite den Auftragsstatus (abgelegt → abgeholt → Ergebnis) und bei Verlust der
  Verbindung „Waiting for holzkube-manager to come back", mit erneutem Polling
  statt Fehlerseite.
- **D-16:** Nur ab Rolle Operator sichtbar-aktiv; ein Reader sieht die Knöpfe
  gesperrt mit Grund (Muster der vorhandenen Aktionen).

### Auslieferung (HACT-08)

- **D-17:** `deploy/holzkube-manager-host.path`,
  `deploy/holzkube-manager-host.service`, `deploy/holzkube-manager-host.sh`,
  `deploy/HOST-HELPER.md` (Anleitung: kopieren, `chmod`, `systemctl
  daemon-reload`, `enable --now` der Path-Unit, Probe). Alle vier kommen über
  `.goreleaser` ins Release-Archiv. `systemd-analyze verify` in `task ci`
  (auf dem CI-Runner vorhanden; lokal ohne es: ausdrücklich übersprungen und
  gemeldet, nie stumm grün).
- **D-18:** Der Helfer-Service ist selbst gehärtet, soweit `systemctl reboot`
  es zulässt (`ProtectHome`, `PrivateTmp`, `ProtectSystem=strict` mit eigenem
  `StateDirectory`, `NoNewPrivileges`), und die Anleitung verlangt **keine**
  Zeile weniger an der Daemon-Unit — sie nennt deren Härtung ausdrücklich als
  unverändert.
- **D-19:** Das Update-Skript installiert den Helfer **nicht** mit und
  aktualisiert ihn nicht: neuen Root-Code auf den Host zu bringen ist jedes Mal
  die Entscheidung des Betreibers (CLAUDE.md).

### Claude's Discretion

- `PathExists=` gegen `DirectoryNotEmpty=` (Research), Timeout-Wert um 10 s,
  Wortlaut der Seite und der Anleitung.
- Wie der Skript-Test läuft (Go-Test, der `bash` mit Stellvertreter-`systemctl`
  und Test-Verzeichnissen fährt, ist erwartet).
- Ob Problem-Codes in `internal/httpapi/problem.go` einzeln oder als Gruppe
  angelegt werden — `docs/api-contract.md` und `contract_codes_test.go` müssen
  sie führen.

### Ohne Rückfrage entschieden

Gewählt wurde jeweils die empfohlene Antwort; verworfen:

- *„Updates suchen" nur als `--check` (nichts installieren)* — verworfen: der
  Betreiber aktualisiert, sobald ein Release da ist; ein Knopf, der „verfügbar"
  meldet und dann bis zu einer Stunde auf den Timer wartet, hilft nicht. Der
  Knopf heißt deshalb ehrlich „Check for updates and install" und läuft über
  dieselbe Unit wie der Timer. **Zur Prüfung vorgemerkt**, weil HACT-04 nur
  „suchen" sagt.
- *Auftrag als JSON mit Benutzer und Zeit* — verworfen, jedes Feld, das root
  parst, ist Angriffsfläche; das Audit-Log hat den Rest.
- *Warteschlange mehrerer Aufträge* — verworfen, vier seltene Aktionen brauchen
  keine; ein zweiter Klick bekommt 409.
- *Helfer-Erkennung per Heartbeat-Datei des Helfers* — verworfen, eine
  Path-Unit führt nichts aus, bis ein Auftrag kommt; die Dateiprüfung plus
  Abhol-Timeout deckt beide Fälle.
- *Ergebnis ins Datenverzeichnis des Daemons schreiben* — verworfen,
  Symlink-Falle für root (wie Phase 11 D-14).
- *sudoers-Regel für den Daemon-Benutzer* — verworfen, braucht
  `NoNewPrivileges=false`; die Härtung bleibt (REQUIREMENTS).
- *polkit/D-Bus (`org.freedesktop.login1`)* — verworfen, braucht `AF_UNIX`.
- *Tipp-Pflicht nur für Herunterfahren* — verworfen, HACT-05 verlangt sie für
  jede Host-Aktion.
- *Helfer über das Update-Skript mitverteilen* — verworfen, siehe D-19.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/httpapi/handlers/jobs.go`: `issueConfirmation`, `typedPhrase`
  (Tabelle statt Bedingung, fehlender Eintrag = Ablehnung), `nodeAction`
  (Intent aus dem Request neu gebaut, nie aus dem Token gelesen).
- `internal/jobs/confirm.go`: `Confirmer.Issue/Check`, `Intent`.
- `internal/httpapi/router.go`: `Destructive` → `middleware.Sudo`, `MinRole`,
  `Action` → Audit; Registrierungsprüfungen beim Start.
- `internal/audit/redact.go`: `allowlist` (fail-closed).
- `internal/httpapi/handlers/power.go`: Muster für Aktionsrouten in einer
  Schleife.
- `web/src/components/SudoDialog.tsx`, `PowerMenu.tsx`, `NodeActions.tsx`,
  `ui/dialog.tsx`.
- `deploy/holzkube-manager-update.sh`: Stil des Root-Skripts (deutsch
  kommentiert, `set -euo pipefail`, `log`/`fail`).
- `internal/depguard_test.go`: Abhängigkeitswächter per `go list -deps`.

### Established Patterns
- Destruktiv nach D-06: Sudo-Fenster + an Parameter gebundenes Token + Audit.
- Jede Sperre einzeln rot gesehen (CLAUDE.md).
- `.goreleaser` liefert `deploy/`-Dateien im Archiv aus.

### Integration Points
- `internal/host` (aus Phase 11) bekommt Auftrag schreiben, Helfer prüfen,
  Ergebnis lesen.
- `handlers/host.go`: Bestätigung + vier Aktionsrouten.
- `routes/host.tsx`: Aktionsblock.
- `Taskfile.yml` `ci`: `systemd-analyze verify`.
- `docs/api-contract.md`: Routen und Codes `conflict.host-order-pending`,
  `conflict.host-helper-missing`, `conflict.host-in-container`.

</code_context>

<specifics>
## Specific Ideas

- Die Daemon-Unit des Betreibers setzt `ProtectHostname=true`; der Hostname aus
  `uname(2)` ist trotzdem der des Hosts (eigene UTS-Kopie beim Start), die
  Tipp-Bestätigung vergleicht also mit dem richtigen Namen.
- Dienst-Neustart und Update beenden den Prozess, der gerade die Antwort
  geschrieben hat: die Route antwortet `202` mit der Auftrags-id, **bevor**
  irgendetwas passiert; der Helfer handelt erst danach.
- Erfolgskriterium 3 als Test: das Skript gegen ein Test-Datenverzeichnis mit
  `SYSTEMCTL=<stellvertreter>`; Fälle: die vier gültigen, ein fremdes Wort,
  `../etc/passwd`, `reboot; rm -rf /`, `reboot $(id)`, ein Symlink statt Datei,
  eine Datei > 64 Byte, fehlende id. Rot gesehen gegen ein Skript, das den
  Auftrag ohne Muster ausführt.

</specifics>

<deferred>
## Deferred Ideas

- Aktion „Rollback auf die vorige Version" (`holzkube-manager-update
  --rollback`) — Future Requirement.
- Den Helfer über das Update-Skript mitaktualisieren — Entscheidung des
  Betreibers.
- Eine reine „nur nachsehen"-Aktion neben „suchen und installieren", falls der
  Betreiber D-06 umwirft.

</deferred>
