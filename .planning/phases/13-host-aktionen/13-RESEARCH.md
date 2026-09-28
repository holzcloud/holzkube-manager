# Phase 13: Host-Aktionen über einen root-eigenen Helfer - Research

**Researched:** 2026-09-29
**Domain:** systemd path activation, a root shell script that reads input from an unprivileged directory, a Go order writer and reader beside the store, four destructive routes on the existing D-06 gates, one React action block. No new libraries.
**Confidence:** HIGH for the repository and for systemd semantics measured on this host; MEDIUM for how `systemctl reboot` behaves inside the helper's sandbox (can only be proven at install time, see Assumptions Log).
**Where this ran:** The operator's Raspberry Pi 5, natively on aarch64. systemd 257 (`257.13-1~deb13u1`), GNU coreutils 9.7, Go 1.27.1 from `~/.local/go` (the module pins `toolchain go1.26.7`), `systemd-analyze` and `unshare --user --map-root-user` both present and working. The production service, its units and its data directory were **not** touched; the only live-system reads were `systemctl cat` of the three installed units, `systemd-inhibit --list`, `sysctl fs.protected_*`, `ls -ld` of `/etc/systemd/system/*.wants`, `/usr/local/sbin`, `/var/lib` and `stat` of the data directory. The systemd path-unit behaviour below was measured with **transient user-manager units** in a scratch directory (removed afterwards), not with the system manager.

## Summary

Every building block exists: the D-06 gate chain (`Destructive` → sudo window, `MinRole`, `Action` → audit) is declarative on `httpapi.Route` and enforced at composition; `issueConfirmation`/`typedPhrase` and `jobs.Confirmer` bind a token to an intent rebuilt from the request; `internal/host` reads everything through an `fs.FS` rooted at `/`; the store exports file primitives (`fsstore.WriteFileAtomic`, `fsstore.ReadFile`) that a package beside the store takes by injection (`internal/history`), which keeps the file-access guard intact; the update script shows the house style for a root script and 11-02 shows how to test one without root (a copy against PATH stubs, root cases under `unshare --user --map-root-user`). The phase is mostly wiring — but five findings change how it has to be built.

**(1) D-11 as worded cannot be built.** `go list -deps ./cmd/holzkube-managerd` already contains `os/exec` today, pulled in by four third-party packages (`github.com/pkg/browser` via Talos machinery, `github.com/containernetworking/cni/pkg/invoke`, `github.com/google/gnostic-models/compiler`, `k8s.io/client-go/plugin/pkg/client/auth/exec`). The guard must be "no package **of this module** imports `os/exec`" (checked on the binary's dependency list with `{{.Imports}}`) plus the AST selector scan; and the unit's `SystemCallFilter=@system-service` does **not** block exec (it includes `@process`), so this source guard is the only thing that holds criterion 2. **(2) `systemd-analyze verify` exits 0 on a misspelled or unparseable hardening line** — measured: `ProtectSytem=strict` and `ProtectSystem=stritc` both give rc 0 with a warning. A gate that reads only the exit code would stay green while a typo silently drops a protection; the gate must fail on any output. It also fails on a missing `ExecStart=` binary, so it has to run on a copy whose `ExecStart=` points at an existing executable (`--root=` does not help: it then wants `sysinit.target` inside the root). **(3) `[ -f ] && [ ! -L ]` alone is a TOCTOU hole** for a root reader in a directory the daemon owns; the decisive read must be `dd iflag=nofollow,nonblock bs=65 count=1` (measured: a symlink is refused with ELOOP, a FIFO returns immediately with 0 bytes, a directory errors). **(4) A plain rename cannot give "one slot, 409 if taken"** — `rename(2)` replaces an existing order. Exclusive placement needs `link(2)` (fails with EEXIST) or `renameat2(RENAME_NOREPLACE)` (measured). **(5) `PathExists=` is level-triggered and fires on boot**, so an order left by a power cut in the 10-s window would reboot the host once more after boot; the script needs an age window on the order's mtime.

`PathExists=` is the right trigger, `DirectoryNotEmpty=` is not: the data directory holds the whole store and is never empty. Measured on systemd 257 (user manager): a dot-prefixed temp file does not trigger; the rename/link into `host-order` triggers exactly once; a consuming service does not loop; a file present when the path unit starts triggers immediately; a service that does **not** consume runs 5 times and the path unit then stops with `Result=unit-start-limit-hit` and stays failed.

**Primary recommendation:** Build it in four slices — (A) the root side: `deploy/holzkube-manager-host.{sh,path,service}` + `HOST-HELPER.md`, with a Go test that drives the script under `unshare` against stubs and a Go test that gates the units on `systemd-analyze verify` output; (B) the daemon side: an fsstore placement primitive, `internal/host/hostaction` (place/withdraw/read result/detect helper), the process-start guard; (C) the routes: host confirm + four actions on the existing gates, problem codes, audit allowlist, contract; (D) the page: action block, dialog, status box, helper notice, fixtures, README/guide, re-rendered `host.png`/`wall.png`. Each guard seen red against its reinstated fault (table in Validation Architecture).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

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

### Claude's Discretion

- `PathExists=` gegen `DirectoryNotEmpty=` (Research), Timeout-Wert um 10 s,
  Wortlaut der Seite und der Anleitung.
- Wie der Skript-Test läuft (Go-Test, der `bash` mit Stellvertreter-`systemctl`
  und Test-Verzeichnissen fährt, ist erwartet).
- Ob Problem-Codes in `internal/httpapi/problem.go` einzeln oder als Gruppe
  angelegt werden — `docs/api-contract.md` und `contract_codes_test.go` müssen
  sie führen.

### Deferred Ideas (OUT OF SCOPE)

- Aktion „Rollback auf die vorige Version" (`holzkube-manager-update
  --rollback`) — Future Requirement.
- Den Helfer über das Update-Skript mitaktualisieren — Entscheidung des
  Betreibers.
- Eine reine „nur nachsehen"-Aktion neben „suchen und installieren", falls der
  Betreiber D-06 umwirft.

### UI-SPEC (approved 2026-09-29) — binding checker resolutions

1. Buttons stay off until an order is final or the page is current (extra reasons: waiting to come back, an `update` order started but not finished, last poll failed).
2. Footer labels: "Keep running" instead of "Cancel" in the four dialogs; "Dismiss status" on the status box.
3. Inherited exceptions (weight 500, Button 0.8rem, spacing 12/20/28/44) accepted, none newly authored.
4. Carried from 12-UI-REVIEW: sensor ▲ colour from `SEVERITY_COLOR[severity]`; wall host sentence figure-first; re-render `host.png` and `wall.png`.
</user_constraints>

### Research refinements to the locked decisions (within their intent — the planner applies these)

Each keeps what the decision is for and changes only a mechanism that, as worded, does not work or leaves a hole. Every one is backed by a measurement in this session.

| # | Decision | As worded | Use instead | Why (evidence) |
|---|----------|-----------|-------------|----------------|
| R1 | D-11 | "binary must not import `os/exec`" via `go list -deps` | No package whose import path starts with `github.com/holzcloud/holzkube-manager/` may list `os/exec` in `{{.Imports}}` (on `go list -deps ./cmd/holzkube-managerd`), **plus** an AST scan of all non-test `.go` under `cmd/` and `internal/` for the `"os/exec"` import and the selectors `os.StartProcess`, `syscall.ForkExec`, `syscall.Exec`, `syscall.StartProcess`, `unix.Exec`, and the identifiers `SYS_EXECVE`/`SYS_EXECVEAT` | `os/exec` is already a dependency of the binary through four third-party packages (measured); a membership check is red on day one. No own package imports it today (measured), so the refined guard is green now and red against the injection. |
| R2 | D-02 | temp `.host-order.tmp-<id>`, "atomar umbenennen" | temp `store.TempFilePrefix + "host-order-" + id` (dot-prefixed), fsync, then `os.Link(tmp, host-order)` (EEXIST → 409), then remove tmp | `rename(2)` overwrites a pending order, so "409 if taken" would be a racy check-then-rename. `link(2)` is atomic-exclusive and portable (the daemon also builds for darwin). The store's startup sweep removes only names starting with `.holzkube-manager-tmp-`, so D-02's name would leave orphans nothing sweeps. Still dot-prefixed, so path units ignore it (measured). |
| R3 | D-03 | `PathExists=` or `DirectoryNotEmpty=` | `PathExists=/var/lib/holzkube-manager/host-order` | The data directory is never empty; `DirectoryNotEmpty=` would fire permanently. `PathChanged=`/`PathModified=` are edge-triggered and "Something similar does not apply" at activation (man page). |
| R4 | D-04 | `-f` and `! -L`, then read | keep `-f`/`! -L` as the first classification (for the logged reason), but the **only** read is `dd if="$ORDER" iflag=nofollow,nonblock bs=65 count=1 status=none` into a private temp file | the checks and the read are two syscalls; the daemon can swap the name in between. `O_NOFOLLOW` refuses a symlink at open, `O_NONBLOCK` keeps a FIFO from hanging root (measured). |
| R5 | D-03/D-04 | — | reject a well-formed order whose mtime is older than 60 s or more than 5 s in the future (`rejected`, reason "stale") | `PathExists=` fires at boot (measured: file present at path-unit start → immediate run). A power cut inside the 10-s pickup window would otherwise reboot/power off the host once more after it comes back. |
| R6 | D-05 | `<id> <aktion> <ergebnis> <zeit>` | same four fields; for an order that did not match the pattern, `id` and `aktion` are `-` (`- - rejected <zeit>`) | a malformed order has no trustworthy id or action to echo; writing parsed-but-unvalidated text into a file the daemon shows would be the injection D-04 forbids. |
| R7 | D-17 | `systemd-analyze verify` in `task ci` | a Go test that copies the units to a temp dir, rewrites `ExecStart=` to an existing executable, runs `systemd-analyze verify --man=no`, and fails on **non-zero rc or any output**; it carries its own negative control (a copy with `ProtectHom=true` must produce output) | verify exits 0 on unknown keys and bad values (measured three ways); it exits 1 for a missing `ExecStart=` binary (measured); `--root=` needs a full unit tree (measured: "Unit sysinit.target not found"). |
| R8 | D-13 | "löscht der Daemon ihn selbst" | withdraw by **claiming**: rename `host-order` → `.holzkube-manager-tmp-withdrawn-<id>` in the same dir; success = withdrawn (then remove), `ENOENT` = the helper took it | a stat-then-remove races the helper; rename is atomic and whoever renames first owns the order. |
| R9 | D-13 | timer only | also withdraw at daemon start (any `host-order` present when the process starts was placed by a previous process) | the in-memory 10-s timer dies with the process; D-13's purpose ("never a leftover that a later helper runs") needs a startup sweep too. |

## Phase Requirements

<phase_requirements>

| ID | Description | Research Support |
|----|-------------|------------------|
| HACT-01 | Host neu starten | Order `reboot` → script → `systemctl reboot` (asynchronous, returns after enqueueing — man systemctl). Route `POST /api/v1/host/actions/reboot`. Pattern 1–4 below. |
| HACT-02 | Host herunterfahren | Order `poweroff` → `systemctl poweroff`. Same chain. |
| HACT-03 | Dienst neu starten | Order `restart-service` → `systemctl restart holzkube-manager.service` (blocking; `TimeoutStartSec=3min` on the helper). The daemon's `srv.Shutdown` lets the in-flight `202` and the audit outcome finish (main.go:745-752). |
| HACT-04 | „jetzt nach Updates suchen" | Order `update` → `systemctl start --no-block holzkube-manager-update.service` (the timer's unit, verified installed: `Type=oneshot`, `ExecStart=/usr/local/sbin/holzkube-manager-update`). Outcome visible through the existing `updatestatus` reader (`checked_at` later than the order). Label "Check for updates and install" (CONTEXT flag kept for operator review). |
| HACT-05 | Sudo-Fenster + getippter Hostname + Audit + ab Operator | Route flags `Destructive`, `MinRole: model.RoleOperator`, `Action: "host.*"`; host confirm route with its own all-`true` typed table compared against `Sys.Uname().Nodename`; intent `Machine` mark `@host`; audit `Attempt` is written **before** the handler runs (middleware/audit.go:74-86). Four locks each red (Validation table F1–F6). |
| HACT-06 | Daemon führt nie selbst aus; unbekannter Auftrag verworfen und protokolliert | Refined guard R1; script with anchored ERE under `LC_ALL=C`, nofollow/nonblock read, consume-first, reject log "reason + byte count" only; result `rejected` in `last`. Script test matrix of 21 cases measured with a prototype (Code Examples). |
| HACT-07 | Helfer fehlt → Oberfläche sagt es, kein Auftrag | Detection via `fs.FS` (+ `fs.Lstat` for the wants symlink) of three paths readable under the daemon's `ProtectSystem=strict`; route checks it **before** placing anything; D-13 withdrawal + startup sweep as the second net. |
| HACT-08 | `deploy/` mit Units, Skript, Anleitung | Unit texts below verified with `systemd-analyze verify` (rc 0, empty output) and `systemd-analyze security --offline=true` (exposure 4.9 "OK"); `.goreleaser.yaml` `archives[default].files` gains the four files; install commands kept byte-identical between `HOST-HELPER.md` and the page by a test. |

</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Public repository:** no LAN address, host name, public name, mail address, MAC or home directory of the real setup in code, tests, fixtures, ledger or commit messages. Use `192.168.1.10`, `homeserver`, `example-host`, `example.com`. `internal/publicrepo` fails the gate on known values (stored as hashes). The installed daemon unit carries real identifiers in its `Environment=` lines — **nothing from it may be copied** into deploy files, docs or tests; `HOST-HELPER.md` quotes only the hardening lines.
- **Production target is linux/arm64 (the Pi 5).** Check arm64 when only one can be checked. This session ran on it natively.
- **Feature not done until the README says so:** "What it does" host line + `host.png` re-rendered from `web/fixtures/demo.json` via `task build && node web/scripts/readme-images.mjs`; depth goes to `docs/guide.md`.
- **Release carries its changelog** (`internal/changelog/changelog.json`) — only when a release is cut, which happens only on the operator's request.
- **Alpha** stays said everywhere; `holzkube-managerd --version` stays a bare version.
- **The daemon must never gain root:** `User=holzkube-manager`, `NoNewPrivileges=true`, `CapabilityBoundingSet=` (empty), `RestrictAddressFamilies=AF_INET AF_INET6`, `ProcSubset=pid`, `ProtectSystem=strict`, `StateDirectoryMode=0700`, `UMask=0077` stay (read from the installed unit this session).
- **Replacing the production binary, restarting the service, installing units, or copying `/var/lib/holzkube-manager` is the operator's call, every time.** This phase installs nothing on the host.
- **A guard is worth nothing until it has gone red against the fault deliberately put back**; an injection that does not inject is an unperformed measurement (11-02 row 2a is the precedent).
- **Every question to the operator is a choice** (AskUserQuestion, 2–4 options, recommended first). This research asks nothing; open points are resolved or deferred below.
- **No push CI** (no Actions minutes since 2026-09-28): `./bin/task ci` locally is the gate. **No `-race` on the Pi** (ThreadSanitizer refuses the 47-bit address space). Known flake: `internal/upgrade` `TestANodeSaysHowItBooted` under load.
- **Empty output is not green:** read exit codes from the command itself, never through a pipe.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Deciding an action is allowed (role, sudo, typed hostname, token) | API / Backend (`internal/httpapi` route flags + `handlers/host.go`) | Browser (display only) | The route's `MinRole`/`Destructive`/token are the gates; the browser's disabled buttons are courtesy (D-16). |
| Recording who did it | API / Backend (audit middleware) | — | `Attempt` before the handler, `Outcome` after; the order file carries no user (D-01). |
| Placing / withdrawing an order | Storage (data dir via an fsstore primitive) | API / Backend (`internal/host/hostaction`) | File primitives live in the store package (file-access guard); the policy (id, timeout, one slot) lives in `hostaction`. |
| Carrying an order out | Host OS: systemd path unit → root oneshot → script | — | The only root code; the daemon never executes (R1). |
| Result of an order | Host OS (`/var/lib/holzkube-manager-host/last`, root-owned) | API / Backend (strict reader via `fs.FS`) | Root writes only into its own directory (Phase 11 D-14 rule). |
| Helper installed? | API / Backend (`fs.FS` stat/lstat of three paths) | — | No D-Bus/`AF_UNIX` in the daemon. |
| Order status, waiting, back | Browser (derives phases from polled `GET /api/v1/host` + the `202` id) | API / Backend (exposes order/withdrawn/last/service start/boot time) | UI-SPEC phase table. |

## Standard Stack

### Core (all already in the repository or the base system)

| Component | Version | Purpose | Why Standard |
|-----------|---------|---------|--------------|
| systemd path + oneshot service | 257 on the Pi [VERIFIED: `systemctl --version`] | activate the root helper when the order file exists | Same mechanism family as the existing update timer/service; no daemon, no socket. `TriggerLimit*` since 250 [CITED: local `man systemd.path`]. |
| bash + GNU coreutils (`dd`, `stat`, `mktemp`, `mv`, `rm`, `date`) | bash 5.x, coreutils 9.7 [VERIFIED: `dd --version`] | the helper script | `dd iflag=nofollow,nonblock` is GNU; the update script already requires the GNU userland. No python3 in the helper (not needed; one less interpreter running as root). |
| Go stdlib `os.Link`, `io/fs` (`fs.Stat`, `fs.Lstat`, `fs.ReadLinkFS`) | Go 1.26 module / 1.27 local | place order, detect helper | `os.DirFS` implements `fs.ReadLinkFS` [VERIFIED: `go doc os.DirFS`]; `fstest.MapFS` has `Lstat` [VERIFIED: `go doc testing/fstest.MapFS.Lstat`]. |
| `crypto/rand` | stdlib | 8 random bytes → 16 hex order id | the id must not be guessable or sequential. |
| React + existing `ui/` pieces (`Button`, `Dialog`, `Input`, `Label`), `SudoDialog`, `Problem` | as installed (UI-SPEC inventory) | action block | UI-SPEC: nothing new added. |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `os.Link` for exclusive placement | `unix.Renameat2(..., unix.RENAME_NOREPLACE)` (x/sys v0.47 is already a direct dependency; measured to return "file exists" on the second placement) | Linux-only → build tags for the darwin daemon build; `os.Link` is portable and equally atomic. Use `os.Link`. |
| `dd iflag=nofollow,nonblock` in bash | `python3 -I -c` with `os.open(O_NOFOLLOW|O_NONBLOCK)` + `fstat` | Python gives an fstat on the same fd, but adds an interpreter to the root path for no gain: the regex + exact-size checks reject every non-order content anyway. |
| Moving the order into a root-owned directory before reading | — | Under `ProtectSystem=strict` the data dir (`ReadWritePaths=`) and the helper's `StateDirectory=` are separate bind mounts: `rename(2)` across them fails with `EXDEV` [ASSUMED: kernel mount semantics], and GNU `mv` would silently fall back to copy+unlink. Do not do this. |

**Installation:** none. No package is installed by this phase.

## Package Legitimacy Audit

No external package is added: Go uses the stdlib (and the already-direct `golang.org/x/sys`, only if the planner picks `Renameat2`, which this research recommends against); the web side uses installed components only; the host side uses base-system bash/coreutils/systemd.

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
Browser (/host, operator)
  │ 1. POST /api/v1/host/confirm {action:"host.reboot", typed:"example-host"}
  ▼
Router chain: allowHosts → csrf → authn → audit(Attempt) → role(MinRole operator) → sudo? (not destructive)
  │   handler: container? → 409 ; helper missing? → 409 ; typed == uname nodename? else 422 field "typed"
  │   → Confirmer.Issue(Intent{Action:"host.reboot", Machine:"@host"}) → 200 {token}
  ▼
  │ 2. POST /api/v1/host/actions/reboot {confirmation}
  ▼
Router chain: … audit(Attempt) → role → sudo window open? else 428 sudo.required (SudoDialog replays)
  │   handler: container? → 409 host-in-container
  │            helper files present? → else 409 host-helper-missing (no file written)
  │            Confirmer.Check(token, Intent{Action:"host.reboot", Machine:"@host"}) → else 403
  │            hostaction.Place("reboot") ── fsstore primitive:
  │                 write <datadir>/.holzkube-manager-tmp-host-order-<id> (0600, fsync)
  │                 link → <datadir>/host-order   (EEXIST → 409 host-order-pending)
  │                 remove tmp, fsync dir, arm 10-s withdraw timer
  │            → 202 {order:{id, action, placed_at}}          audit(Outcome)
  ▼
PID 1 inotify on /var/lib/holzkube-manager/host-order  (holzkube-manager-host.path, PathExists=)
  ▼
holzkube-manager-host.service (root oneshot, sandboxed) → /usr/local/sbin/holzkube-manager-host
  │   classify (-L / -f) → read ≤65 B with dd nofollow,nonblock → rm host-order (consume)
  │   reject? (type, size, one line, anchored ERE, mtime window) → log reason+bytes, last="… rejected", exit 2
  │   last="<id> <action> started <time>" → systemctl <fixed argv> → on failure last="… failed", exit 1
  ▼
systemd: reboot.target / poweroff.target / restart holzkube-manager.service / start holzkube-manager-update.service
  ▼
Daemon (GET /api/v1/host every 3 s): order file present? withdrawn? last (strict reader) → page phases
  └─ 10 s after placing, file still there → claim-rename + remove → "withdrawn" (D-13)
```

### Recommended Project Structure

```
deploy/
├── holzkube-manager-update.sh        # unchanged (D-19: must not learn about the helper)
├── holzkube-manager-host.sh          # NEW root script, installed as /usr/local/sbin/holzkube-manager-host
├── holzkube-manager-host.path        # NEW
├── holzkube-manager-host.service     # NEW (no [Install]: started only by the path unit)
└── HOST-HELPER.md                    # NEW install guide; install block between markers
internal/store/fsstore/
└── beside.go (or atomic.go)          # NEW exported PlaceNew / Claim primitives (history-style injection)
internal/host/hostaction/             # NEW package: Action, Order, Box (place/withdraw/timer/sweep),
│                                     #   Result reader (strict), Helper detection, InstallCommands
├── hostaction.go / result.go / helper.go
├── *_test.go
├── script_test.go                    # drives deploy/holzkube-manager-host.sh (non-root + unshare root)
└── units_test.go                     # systemd-analyze verify gate + consistency with Go constants + HOST-HELPER.md
internal/host/                        # View gains `actions` (read via the Box + fs.FS)
internal/httpapi/handlers/host.go     # + confirm route + four action routes (loop like power.go)
internal/httpapi/problem.go           # + CodeHostOrderPending, CodeHostHelperMissing, CodeHostInContainer
internal/audit/redact.go              # + "host.reboot": {}, … (4 entries)
internal/processguard_test.go         # NEW (or in depguard_test.go): R1 guard
web/src/components/HostActions.tsx    # NEW (+ .test.tsx); host.tsx wires it; api.ts schema + calls
```

### Pattern 1: Exclusive placement through a store primitive (keeps the file-access guard intact)

**What:** `TestNoDirectFileAccessOutsideFsstore` forbids `os.WriteFile/OpenFile/Create/Remove/Rename/Link/…` outside `internal/store/…` and a few named files (permissions_test.go forbidden map, read this session). The established way out is not a new exemption but injection: `internal/history.Open(path, read, write, …)` is handed `fsstore.ReadFile, fsstore.WriteFileAtomic` by `main.go:448`. Do the same: add to fsstore two exported functions and hand them to `hostaction.NewBox`.

**Contract:**
- `fsstore.PlaceNew(path string, data []byte) error` — temp in the same dir named `store.TempFilePrefix + "host-order-" + <random>`, mode 0600, write, fsync, `os.Link(tmp, path)`; `errors.Is(err, fs.ErrExist)` when taken; always remove tmp; fsync dir.
- `fsstore.Claim(path, claimName string) ([]byte, error)` — `os.Rename(path, <dir>/claimName)`; `fs.ErrNotExist` when gone; read (bounded) and remove the claimed file.
- `hostaction` owns the policy: one slot, id generation, the 10-s `time.AfterFunc` withdraw, startup sweep (`Claim` at `NewBox`), and what `GET /api/v1/host` shows.

Mode 0600 matters: `fsstore.Guard` walks the **whole** data dir at start and refuses to start if any regular file has group/other bits or if anything is not a regular file or directory (permissions.go:32-73, read this session).

### Pattern 2: The action routes (one route per action, like power.go)

```go
// Source: shape of internal/httpapi/handlers/power.go and jobs.go (read this session)
for _, a := range hostaction.Actions() { // reboot, poweroff, restart-service, update
    routes = append(routes, httpapi.Route{
        Method:          http.MethodPost,
        Pattern:         "/api/v1/host/actions/" + string(a),
        RequiresSession: true,
        MinRole:         model.RoleOperator,
        Destructive:     true,
        Action:          "host." + string(a),
        Handler:         handler(hostAction(d, a)),
    })
}
```

Handler order: `d.HostActions == nil` → 502 `upstream.host-unavailable` (existing code) → container → 409 `conflict.host-in-container` → helper missing → 409 `conflict.host-helper-missing` → `d.Confirmer.Check(body.Confirmation, jobs.Intent{Action: "host." + string(a), Machine: hostIntentTarget})` → `writeJobError` (403 invalid/expired) → `Place` → 409 `conflict.host-order-pending` → `202 {"order": {...}}`. The intent is rebuilt from the route, never from the token (jobs.go:397-404 pattern). `hostIntentTarget = "@host"`: `MachineID` is documented as "a machine's UUID" (model.go:17-18) and the pseudo ids in inventory use `unadopted:`, `adopting:`, `member:`, `endpoint:`, `manual:` prefixes — none contains `@`. The action namespace (`host.*` vs `node.reboot`/`node.shutdown`, model/job.go:88,91) is a second, independent separation.

The confirm route `POST /api/v1/host/confirm`: `RequiresSession`, `MinRole: model.RoleOperator`, `Action: "action.confirm"`, **not** `Destructive` (the node confirm route is not either, jobs.go:71-78). Body `{action, typed}`. Its table is its own:

```go
// hostTypedPhrase: every host action requires typing the hostname (D-09).
var hostTypedPhrase = map[string]bool{
    "host.reboot": true, "host.poweroff": true, "host.restart-service": true, "host.update": true,
}
```

It must **not** be merged into `typedPhrase` — otherwise `POST /api/v1/machines/{id}/confirm` would issue host-action tokens (with `Machine` = a machine id, which the host route then refuses — but the table is the wrong place to rely on that). Compare `strings.TrimSpace(body.Typed)` with a **fresh** `Sys.Uname().Nodename` (the daemon's `ProtectHostname=true` gives it a UTS copy made at service start, so it is the host's name as of start — the same name the page shows). Empty/unreadable hostname → 422 like jobs.go:344-350. Only `d.Confirmer` is needed — do not reuse `jobsConfigured`, which also demands `d.Jobs`.

### Pattern 3: The root script (consume, validate, record, act)

See Code Examples for the full measured skeleton. Rules:
- `set -euo pipefail`, `cd /`, `export LC_ALL=C` (the ERE's `[0-9a-f]` must be ASCII), `umask 022`, fixed `PATH=/usr/sbin:/usr/bin:/sbin:/bin`.
- Overrides only for tests, like the update script: `HOLZKUBE_MANAGER_HOST_ORDER` (default `/var/lib/holzkube-manager/host-order`), `HOLZKUBE_MANAGER_HOST_STATE_DIR` (default `/var/lib/holzkube-manager-host`), `HOLZKUBE_MANAGER_SYSTEMCTL` (default `systemctl`; D-06's "Variable"). The unit sets no `Environment=` (same acceptance as T-11-10). The script test refuses a script copy lacking these names (11-02 precedent: a red run must never address the host's real paths).
- Refuse to run unless `EUID == 0` (tests run it under `unshare --user --map-root-user`).
- Refuse a state dir that is a symlink, not a directory, or not owned by uid 0 (Phase 11 `record_status` precedent); `StateDirectory=` creates it, the check is defence in depth.
- Log lines never contain order bytes: only the reason and the byte count (`${size}`), and the id/action **only after** the line matched the pattern.
- The argv for each action is a fixed array in a `case`; the order text is never passed to a command, never `eval`'d, never unquoted.
- Exit codes: 0 no order / started; 2 rejected; 1 failed. A non-zero exit marks the service failed in `systemctl --failed` (visible to the operator) but does **not** break the path unit (it re-checks after every run, success or failure — man page).

### Pattern 4: Units

Verified texts in Code Examples. Rules:
- Path unit: `PathExists=` only; `Unit=` explicit; `[Install] WantedBy=paths.target` (systemd.special: "It is recommended that path units installed by applications get pulled in via Wants= dependencies from this unit"). `paths.target.wants/` does not exist on the Pi yet (checked); `systemctl enable` creates it. **Never `MakeDirectory=`** (it would create a missing data dir as root 0755, and the store's `Guard` then refuses to start).
- Service: `Type=oneshot`, **no** `RemainAfterExit=yes` (an "active" service would never be started again, so the second order would never run), explicit `TimeoutStartSec=3min` (oneshot has no start timeout by default — man systemd.service; `restart-service` waits for the daemon's stop, default `TimeoutStopSec` 90 s), `ReadWritePaths=-/var/lib/holzkube-manager` (without it `ProtectSystem=strict` makes the `rm` fail with EROFS → no consumption → 5 runs → path unit dead), `StateDirectory=holzkube-manager-host` + `StateDirectoryMode=0755`, `UMask=0022` (so `last` is 0644 for the daemon), `RestrictAddressFamilies=AF_UNIX` (systemctl reaches PID 1 and logind over Unix sockets), **no `CapabilityBoundingSet=`** (see Pitfall 7), no `[Install]`.

### Anti-Patterns to Avoid

- **Membership check on `go list -deps` for `os/exec`** — red on day one (R1).
- **Exit-code-only `systemd-analyze verify`** — green on a typo (R7).
- **`[[ -f ]] && [[ ! -L ]]` followed by `< "$ORDER"` / `cat` / `head`** — follows a swapped-in symlink, blocks on a FIFO (R4).
- **`rename(tmp, host-order)`** — replaces a pending order; "409 pending" becomes a race (R2).
- **Validating after acting / removing after acting** — a reboot would find the order again after boot (D-03).
- **Echoing the order into the journal "for debugging"** — the exfiltration channel D-04 closes.
- **`grep -qx` on the whole file** — matches the first line of a two-line file; check the exact size equals one line plus `\n`.
- **Adding host actions to `typedPhrase`** — see Pattern 2.
- **String-literal problem codes** — `TestEveryProblemCodeIsInTheContract` sees only named constants (12-08 finding); declare the three codes as constants in problem.go.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Exclusive "one slot" file | stat-then-rename, in-process mutex only | `os.Link` (EEXIST) | kernel-atomic; survives two requests and a second daemon process |
| Symlink/FIFO-safe read in bash | `-f`/`-L` tests + redirect | `dd iflag=nofollow,nonblock bs=65 count=1` | single open with `O_NOFOLLOW|O_NONBLOCK` (measured) |
| Loop protection | a counter file in the script | systemd's start limit (5 per 10 s → path unit `unit-start-limit-hit`, measured) and `TriggerLimitBurst` (200 / 2 s) | already there; state in the script would itself be attack surface |
| Waking the helper | polling timer, socket, D-Bus | `PathExists=` inotify in PID 1 | no process runs until there is an order |
| Token binding | a new confirm mechanism | `jobs.Confirmer.Issue/Check` + `jobs.Intent` | HMAC over canonical intent, 10-min TTL, constant-time compare |
| Temp-file orphan cleanup | a sweep in `hostaction` | `store.TempFilePrefix` naming → `fsstore` startup sweep | one prefix for everything the sweep must find (store.go:36-45) |
| Unit validation | parsing unit files in Go | `systemd-analyze verify` (+ output gate) and a small Go consistency check for names/paths | verify knows every key; the Go check covers what verify does not (a path unit whose `Unit=` names a missing service passes verify — measured) |

**Key insight:** every hole in this phase is between two syscalls — check vs. read, stat vs. remove, place vs. place. Use the primitive that is one syscall.

## Runtime State Inventory

Not a rename/refactor phase. For completeness, the runtime state this phase *creates* when the operator installs the helper (nothing is installed by the phase itself): `/usr/local/sbin/holzkube-manager-host`, two unit files in `/etc/systemd/system/`, the `paths.target.wants/` symlink, `/var/lib/holzkube-manager-host/last`, and transiently `/var/lib/holzkube-manager/host-order`. None exists on the Pi today (checked: `/usr/local/sbin` holds only the update script and a dated backup; no `paths.target.wants`). Uninstall steps belong in `HOST-HELPER.md`.

## Common Pitfalls

### Pitfall 1: The process guard that cannot go green (or cannot go red)
**What goes wrong:** D-11's `go list -deps` membership check fails immediately (os/exec via client-go and three others); a naive fix exempts "third-party" and then misses an own package that imports `os/exec`.
**How to avoid:** R1 — filter the deps list to packages under the module path and read their `Imports`; AST-scan non-test sources for the import and the selectors. Guard against a vacuous scan (`scanned < 15 → Fatal`, like the file-access guard).
**Warning signs:** the injection `exec.Command("systemctl", "reboot")` in `internal/host` leaves the test green.

### Pitfall 2: `systemd-analyze verify` is green on a hardening typo
**What goes wrong:** `ProtectSytem=strict` → "Unknown key … ignoring", rc 0. The unit loads without the protection.
**How to avoid:** R7; require empty combined output **and** rc 0; keep the built-in negative control.
**Warning signs:** the gate never produced output in its life.

### Pitfall 3: `ExecStart=` makes verify fail everywhere but on an installed host
**What goes wrong:** "Command /usr/local/sbin/holzkube-manager-host is not executable: No such file or directory", rc 1 (measured on the Pi, where it is not installed).
**How to avoid:** verify a temp copy whose `ExecStart=` points to an executable file in the temp dir; assert separately (Go) that the shipped unit's `ExecStart=` equals the install path the guide and the daemon's detection use.

### Pitfall 4: Consumption fails silently under the sandbox
**What goes wrong:** without `ReadWritePaths=-/var/lib/holzkube-manager`, `ProtectSystem=strict` makes `rm` fail (EROFS); the order stays; the path unit restarts the service 5 times in 10 s and then fails with `unit-start-limit-hit` and **stays failed** (measured with a non-consuming service).
**How to avoid:** the line in the unit; the script treats an `rm` failure as `failed` and does not act; the daemon's 10-s withdrawal clears the file; `HOST-HELPER.md` names the recovery (`systemctl reset-failed holzkube-manager-host.service holzkube-manager-host.path && systemctl start holzkube-manager-host.path`).
**Warning signs:** `systemctl status holzkube-manager-host.path` shows `failed (Result: unit-start-limit-hit)`. The page's D-13 message names exactly this command.

### Pitfall 5: A leftover order runs at boot
**What goes wrong:** `PathExists=` is evaluated when the path unit starts (early, `Before=paths.target`, long before the daemon); a file left by a power cut inside the 10-s window is executed after boot.
**How to avoid:** R5 age window in the script **and** R9 startup withdrawal in the daemon (the latter alone is too late: the helper runs first).

### Pitfall 6: The script and the daemon race at the 10-s mark
**What goes wrong:** daemon removes the file while the helper is between classification and `dd`; the helper then logs a bogus rejection and writes `- - rejected`, and the page shows "rejected" for an order that was withdrawn.
**How to avoid:** R8 claim-rename in the daemon; in the script, if `dd` fails **and** the order path no longer exists, exit 0 quietly ("withdrawn before pickup") without writing `last`.

### Pitfall 7: Over-hardening breaks the reboot
**What goes wrong:** `CapabilityBoundingSet=` restricted (e.g. to `CAP_DAC_OVERRIDE`) — logind authorises `Reboot` through polkit and, where polkit cannot answer, by the caller's `CAP_SYS_BOOT`; systemctl does not fall back to PID 1 on `EACCES`. The helper also needs `CAP_DAC_OVERRIDE` to enter and unlink in the 0700 data dir owned by another user. [ASSUMED: logind/systemctl source behaviour from training; polkit is active on the Pi — checked]
**How to avoid:** do not set `CapabilityBoundingSet=`/`SystemCallFilter=` on the helper; keep D-18's list plus the namespace/kernel protections verified above; prove at install time with the `update` probe (same systemctl → PID 1/logind path).

### Pitfall 8: Inhibitor locks
**What goes wrong:** `systemctl reboot` in `--check-inhibitors=auto` lets logind "respect active inhibitor locks" (man systemctl); a *block*-mode `shutdown` lock refuses the reboot.
**How to avoid:** nothing to force (D-06 says plain `systemctl reboot`); the script records `failed` and the journal names the lock. On the Pi today only `sleep` delay locks and a `handle-power-key` block lock exist (checked) — none blocks shutdown.

### Pitfall 9: The confirm-table guards do not see the host table
**What goes wrong:** `TestEveryConfirmedActionIsInTheTable` scans every `Confirmer.Check` in `handlers/` and demands the action be in `typedPhrase`; its `actionValue` returns `""` for anything but a string literal or `ActionRemoveFromCluster`, so a host check written as `"host." + string(a)` is **silently skipped**, while a literal `"host.reboot"` would make it red.
**How to avoid:** add an explicit exclusion for `host.` in that test **and** a new `TestEveryHostActionRequiresTyping` that walks `hostaction.Actions()` against `hostTypedPhrase` (all `true`, exactly four keys) and asserts none of them is in `typedPhrase`.

### Pitfall 10: Lists that enumerate routes
**What goes wrong:** `cmd/holzkube-managerd/budget_test.go` `TestEveryRouteThatReachesUpstreamHasABudgetRow` fails for every route neither budgeted nor named in `noUpstream`.
**How to avoid:** add `POST /api/v1/host/confirm` and the four `POST /api/v1/host/actions/…` to `noUpstream` with a comment (they write one file and reach nothing).

### Pitfall 11: `restart-service` and `update` kill the process that answered
**What goes wrong:** fear that the `202` or the audit outcome is lost.
**Reality:** audit `Attempt` is written before the handler (middleware/audit.go:77); SIGTERM leads to `srv.Shutdown(shutdownCtx)` (main.go:748-752), which lets the in-flight handler finish. No sleep is needed in the script. The page must treat a failed poll after a `started` order as "waiting" (UI-SPEC).

### Pitfall 12: The fixture/page shows install commands that differ from the guide
**How to avoid:** the commands live once in Go (`hostaction.InstallCommands`, served in the response); `units_test.go` extracts the block between `<!-- install-commands:begin -->` and `<!-- install-commands:end -->` in `deploy/HOST-HELPER.md` and compares byte for byte. Go `embed` cannot reach `deploy/` from `internal/` (patterns may not contain `..`) [ASSUMED: embed rule from training], so a comparison test, not an embed.

## Code Examples

### The helper script (skeleton, measured as a prototype against 21 cases on the Pi)

```bash
#!/usr/bin/env bash
# Fuehrt genau einen Host-Auftrag aus, den holzkube-managerd abgelegt hat.
# (deutsch kommentiert wie deploy/holzkube-manager-update.sh)
set -euo pipefail
cd /
export LC_ALL=C
export PATH=/usr/sbin:/usr/bin:/sbin:/bin
umask 022

ORDER=${HOLZKUBE_MANAGER_HOST_ORDER:-/var/lib/holzkube-manager/host-order}
STATE_DIR=${HOLZKUBE_MANAGER_HOST_STATE_DIR:-/var/lib/holzkube-manager-host}
SYSTEMCTL=${HOLZKUBE_MANAGER_SYSTEMCTL:-systemctl}
MAX_AGE=60   # Sekunden; ein aelterer Auftrag ist ein Rest (Stromausfall), kein Wunsch
MAX_SKEW=5

log() { printf '%s\n' "$*"; }
record() {   # id aktion ergebnis -- atomar, nur im eigenen Verzeichnis
  local tmp
  tmp=$(mktemp "$STATE_DIR/.last.XXXXXX")
  printf '%s %s %s %s\n' "$1" "$2" "$3" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$tmp"
  chmod 0644 "$tmp"
  mv -f -- "$tmp" "$STATE_DIR/last"
}
reject() {   # grund bytes [id aktion] -- nie den Inhalt protokollieren
  log "Auftrag verworfen: $1 (${2:-0} Byte)"
  record "${3:--}" "${4:--}" rejected
  exit 2
}

[[ $EUID -eq 0 ]] || { log "FEHLER: nur als root"; exit 1; }
# + Pruefung STATE_DIR: Verzeichnis, kein Symlink, Eigentuemer uid 0

if [[ ! -e $ORDER && ! -L $ORDER ]]; then log "kein Auftrag"; exit 0; fi
kind=regular
if [[ -L $ORDER ]]; then kind=symlink; elif [[ ! -f $ORDER ]]; then kind=not-regular; fi
mtime=$(stat -c %Y -- "$ORDER" 2>/dev/null || echo 0)

work=$(mktemp -d); trap 'rm -rf -- "$work"' EXIT
# Das einzige Lesen: O_NOFOLLOW gegen einen untergeschobenen Symlink,
# O_NONBLOCK gegen ein FIFO, hoechstens 65 Byte.
if ! dd if="$ORDER" of="$work/o" iflag=nofollow,nonblock bs=65 count=1 status=none 2>/dev/null; then
  [[ -e $ORDER || -L $ORDER ]] || { log "Auftrag vor der Abholung zurueckgezogen"; exit 0; }
fi
# Verbrauchen, bevor irgendetwas geschieht.
rm -f -- "$ORDER" || { log "Auftrag nicht entfernbar"; record - - failed; exit 1; }

[[ $kind == regular ]] || reject "$kind"
size=$(stat -c %s -- "$work/o" 2>/dev/null || echo 0)
(( size <= 64 )) || reject "zu lang" "$size"
line=""
IFS= read -r line < "$work/o" || reject "keine vollstaendige Zeile" "$size"
(( size == ${#line} + 1 )) || reject "nicht genau eine Zeile" "$size"
[[ $line =~ ^(reboot|poweroff|restart-service|update)\ ([0-9a-f]{16})$ ]] || reject "unbekannte Form" "$size"
action=${BASH_REMATCH[1]}; id=${BASH_REMATCH[2]}
now=$(date +%s)
(( mtime <= now + MAX_SKEW && mtime >= now - MAX_AGE )) || reject "veraltet" "$size" "$id" "$action"

case $action in
  reboot)          cmd=(reboot) ;;
  poweroff)        cmd=(poweroff) ;;
  restart-service) cmd=(restart holzkube-manager.service) ;;
  update)          cmd=(start --no-block holzkube-manager-update.service) ;;
esac
record "$id" "$action" started      # vor dem Handeln: ein Reboot kann uns danach beenden
log "Auftrag $id: $action"
if ! "$SYSTEMCTL" "${cmd[@]}"; then record "$id" "$action" failed; exit 1; fi
```

Prototype results (stub `systemctl` appending its argv to a log, run under `timeout 5`): the four valid orders called exactly `reboot` / `poweroff` / `restart holzkube-manager.service` / `start --no-block holzkube-manager-update.service` and left `<id> <action> started`; `halt <id>`, `../etc/passwd`, `reboot; rm -rf /`, `reboot $(…)`, symlink (to a file holding a **valid** order), FIFO (returned immediately), 100-byte file, `reboot` without id, no trailing newline, CRLF, two lines, upper-case hex, NUL byte → rc 2, stub never called, `- - rejected`; mtime −10 min and +10 min → rc 2 with `<id> reboot rejected`; absent order → rc 0, nothing written. A directory named `host-order` → `rm -f` fails, rc 1, order stays (only a compromised daemon can create it; the start limit then stops the loop — document, do not `rm -r` as root).

### The units (verified: `systemd-analyze verify --man=no` rc 0 with **empty** output on the Pi; offline security exposure 4.9 "OK")

```ini
# deploy/holzkube-manager-host.path
[Unit]
Description=holzkube-manager - auf Host-Auftraege warten
Documentation=https://github.com/holzcloud/holzkube-manager/blob/main/deploy/HOST-HELPER.md

[Path]
PathExists=/var/lib/holzkube-manager/host-order
Unit=holzkube-manager-host.service

[Install]
WantedBy=paths.target
```

```ini
# deploy/holzkube-manager-host.service -- kein [Install]: nur die Path-Unit startet ihn
[Unit]
Description=holzkube-manager - einen Host-Auftrag ausfuehren
Documentation=https://github.com/holzcloud/holzkube-manager/blob/main/deploy/HOST-HELPER.md

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/holzkube-manager-host
TimeoutStartSec=3min
StateDirectory=holzkube-manager-host
StateDirectoryMode=0755
ReadWritePaths=-/var/lib/holzkube-manager
UMask=0022
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
PrivateDevices=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectKernelLogs=true
ProtectControlGroups=true
ProtectClock=true
ProtectHostname=true
RestrictAddressFamilies=AF_UNIX
RestrictNamespaces=true
RestrictRealtime=true
RestrictSUIDSGID=true
LockPersonality=true
MemoryDenyWriteExecute=true
SystemCallArchitectures=native
```

### Install block for `deploy/HOST-HELPER.md` (and, byte-identical, the page's `pre`)

```
sudo install -o root -g root -m 0755 deploy/holzkube-manager-host.sh /usr/local/sbin/holzkube-manager-host
sudo install -o root -g root -m 0644 deploy/holzkube-manager-host.path deploy/holzkube-manager-host.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now holzkube-manager-host.path
```

Probe section (not part of the byte-compared block): `sudo systemctl start holzkube-manager-host.service` → journal "kein Auftrag", exit 0 (proves the sandbox starts); `systemctl status holzkube-manager-host.path` → `active (waiting)`; then the page's "Check for updates and install" as the first real order (runs the same systemctl→PID 1 path as the others without taking the host down). Uninstall: `systemctl disable --now holzkube-manager-host.path`, remove the three files, `daemon-reload`. The guide states that the daemon's unit keeps every hardening line and names them (`NoNewPrivileges=true`, `CapabilityBoundingSet=`, `RestrictAddressFamilies=AF_INET AF_INET6`, `ProcSubset=pid`, `ProtectSystem=strict`, …), and that a data directory other than `/var/lib/holzkube-manager` needs a drop-in for `PathExists=` and `Environment=HOLZKUBE_MANAGER_HOST_ORDER=` (otherwise every order is withdrawn after 10 s).

### Helper detection (Go, via the collector's `fs.FS` rooted at "/")

```go
// Paths are relative to the fs.FS root, as elsewhere in internal/host (fsPath trims "/").
const (
    helperScript  = "usr/local/sbin/holzkube-manager-host"
    helperPath    = "etc/systemd/system/holzkube-manager-host.path"
    helperService = "etc/systemd/system/holzkube-manager-host.service"
    helperWants   = "etc/systemd/system/paths.target.wants/holzkube-manager-host.path"
)
// script: fs.Stat → Mode().IsRegular(), Mode()&0o111 != 0, Mode()&0o022 == 0,
//         and uid 0 when info.Sys() is *syscall.Stat_t (fstest.MapFile has a Sys field,
//         so tests can present a foreign uid).
// path unit: both unit files present and regular -> else item "path-unit".
// enabled:   fs.Lstat(helperWants) exists (a symlink after `systemctl enable`) -> else item "not-enabled".
```

`units_test.go` asserts that these constants, the unit's `ExecStart=`/`PathExists=`/`WantedBy=`, the script's `ORDER`/`STATE_DIR` defaults and the install commands agree (the update-status-file precedent: config.go:279-281 "a test holds the two spellings together").

### Process guard (R1)

```go
// internal/processguard_test.go (package internal, beside depguard_test.go)
for _, line := range goList(t, "-deps", "-f", "{{.ImportPath}} {{join .Imports \" \"}}", "./cmd/holzkube-managerd") {
    f := strings.Fields(line)
    if len(f) == 0 || !strings.HasPrefix(f[0], rootModule+"/") { continue }
    for _, imp := range f[1:] {
        if imp == "os/exec" { t.Errorf("%s imports os/exec: the daemon never starts a process", f[0]) }
    }
}
// + AST walk of cmd/ and internal/ (skip *_test.go): ImportSpec "os/exec";
//   SelectorExpr os.StartProcess, syscall.ForkExec, syscall.Exec, syscall.StartProcess, unix.Exec;
//   Ident SYS_EXECVE, SYS_EXECVEAT. Fatal if fewer than 15 files scanned.
```

Note: `goList` skips (visibly) when `go` is not on PATH; the AST half runs regardless, so the guard never goes fully vacuous.

### Unit verify gate (R7)

```go
// internal/host/hostaction/units_test.go (sketch)
bin, err := exec.LookPath("systemd-analyze")
if err != nil {
    if runtime.GOOS != "linux" || os.Getenv("HOLZKUBE_MANAGER_NO_SYSTEMD_ANALYZE") == "1" {
        t.Skipf("SKIPPED, not verified: systemd-analyze is not available (%v)", err)
    }
    t.Fatalf("systemd-analyze is not installed; set HOLZKUBE_MANAGER_NO_SYSTEMD_ANALYZE=1 to skip knowingly")
}
// copy both units into t.TempDir(), write an executable stub, rewrite ExecStart= to it,
// run `systemd-analyze verify --man=no <path> <service>`: want rc 0 AND len(output) == 0.
// Negative control in the same test: replace "ProtectHome=true" with "ProtectHom=true" in a
// second copy -> want len(output) > 0. Without it, a verify that stopped reporting would pass.
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Path units without trigger limits | `TriggerLimitIntervalSec=`/`TriggerLimitBurst=` (2 s / 200) plus the service start limit propagating to the path unit | systemd 250 | a non-consuming helper stops itself instead of spinning (measured: `unit-start-limit-hit` after 5 runs) |
| `fs.FS` without symlink support | `fs.ReadLinkFS`, `fs.Lstat`; `os.DirFS` and `fstest.MapFS` implement it | Go 1.25 | the wants symlink can be checked through the collector's `fs.FS`, with fixture tests |
| Triggered units had no context | `TRIGGER_PATH`/`TRIGGER_UNIT` in the environment (measured: set to the order path) | recent systemd | do **not** use: the script's path is fixed; an environment value is not needed and not trusted |

**Deprecated/outdated:** `parser.ParseDir` (Go 1.26) — walk files one at a time in the new AST guard, as `confirm_test.go` does.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `systemctl reboot/poweroff/restart/start` work from the helper's sandbox (`ProtectSystem=strict`, `NoNewPrivileges`, `RestrictAddressFamilies=AF_UNIX`, `PrivateDevices`, no cap restriction); connecting to a Unix socket on a read-only mount is permitted | Pattern 4, Code Examples | An action fails with `failed` in `last`; caught by the install probe (`update` order) for the PID 1 path; the logind reboot path is only proven by a real reboot. Recovery: drop the offending line; the page and journal say `failed`. |
| A2 | PID 1 (root) can inotify-watch a file inside a 0700 directory owned by another user | R3 | Path unit never fires → every order withdrawn after 10 s (D-13 message). Measured only with the user manager on a user-owned dir; the man page's "not accessible → watch for permission changes" and root's `CAP_DAC_READ_SEARCH` make it near-certain. |
| A3 | logind authorises root's reboot via polkit or `CAP_SYS_BOOT`; `systemctl` does not fall back to PID 1 on `EACCES` | Pitfall 7 | Only matters if someone adds `CapabilityBoundingSet=`; the research says don't. |
| A4 | `rename(2)` between two separate bind mounts of the same fs returns `EXDEV` | Alternatives | Only relevant to the rejected "move into root dir" design. |
| A5 | A 60-s age window is safe against clock jumps on the Pi (NTP step between placement and pickup) | R5 | A legit order rejected as `veraltet` in the rare step moment; operator retries. |
| A6 | Go `embed` patterns cannot contain `..` | Pitfall 12 | None; the comparison test works either way. |
| A7 | HACT-04 "suchen" is satisfied by "search and install" | HACT-04 | Operator may want a check-only action (already in Deferred). |

## Open Questions (RESOLVED)

1. **`PathExists=` or `DirectoryNotEmpty=`?** — RESOLVED: `PathExists=/var/lib/holzkube-manager/host-order` (R3; measured behaviour in Summary).
2. **How is an order carried out exactly once?** — RESOLVED: link-exclusive placement (R2), consume before acting (D-03), level trigger + service start limit against loops (measured), age window against boot replays (R5), claim-rename for the daemon's withdrawal (R8), startup sweep (R9).
3. **How does the daemon know the helper is installed without D-Bus?** — RESOLVED: three paths through `fs.FS`/`fs.Lstat` (D-12), with D-13 as the net for "installed but not running/failed".
4. **Can the daemon see that the path unit is *active* (not just enabled) or has failed (`unit-start-limit-hit`)?** — DEFERRED: not without D-Bus or `/run/systemd` internals; D-13 withdrawal plus its message naming `systemctl status holzkube-manager-host.path` covers it.
5. **Data directory other than `/var/lib/holzkube-manager`?** — DEFERRED: the helper watches the fixed default; `HOST-HELPER.md` explains the drop-in; D-13 makes the mismatch visible. A detection item "watches a different path" would need UI-SPEC copy the approved spec does not have.
6. **D-02's temp name `.host-order.tmp-<id>`** — RESOLVED: use `store.TempFilePrefix`-based naming so the store's startup sweep removes orphans (R2).
7. **`systemd-analyze` locally vs. CI** — RESOLVED: installed on the Pi; the Go test fails on Linux when it is missing unless `HOLZKUBE_MANAGER_NO_SYSTEMD_ANALYZE=1` is set, and then skips with "SKIPPED, not verified" — never silently green (R7).
8. **Should `restart-service` use `--no-block`?** — RESOLVED: no, keep D-06's blocking restart so `failed` means the restart job failed; `TimeoutStartSec=3min` bounds it. `reboot`/`poweroff` are asynchronous by themselves (man systemctl).
9. **HACT-04 wording ("suchen" vs. "suchen und installieren")** — DEFERRED to the operator's review, as CONTEXT already flags; the label says what it does.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | all Go work and tests | ✓ | 1.27.1 local; module toolchain go1.26.7 | — |
| bash, GNU coreutils (`dd` with `iflag=nofollow,nonblock`) | helper script + its test | ✓ | coreutils 9.7 | — |
| `unshare --user --map-root-user` | root cases of the script test | ✓ (`id -u` → 0) | util-linux | visible `t.Skip` naming what unshare said |
| `systemd-analyze` | unit verify gate | ✓ | 257 | `HOLZKUBE_MANAGER_NO_SYSTEMD_ANALYZE=1` → explicit skip |
| systemd user manager (`systemd-run --user --path-property=`) | optional real path-unit probe on the Pi | ✓ (`is-system-running` → running) | 257 | Go tests alone |
| Node + Playwright Chromium | web tests, layout audit, README images | ✓ per Phase 12 (not re-checked this session) | — | — |
| `-race` | — | ✗ on the Pi (TSan vs 47-bit VA) | — | none locally; no push CI currently |
| Root / installing the helper on the Pi | end-to-end with real systemd | ✗ by rule (operator's call) | — | user-manager probe + Go tests; operator installs later via `HOST-HELPER.md` |

**Missing dependencies with no fallback:** none that block the phase.
**Missing with fallback:** real installation — the Pi stays in the "helper not installed" state (criterion 4 describes exactly that state) until the operator installs it.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go `testing` (+ `testing/fstest`), Vitest (jsdom + browser project), `web/scripts/layout-audit.mjs` |
| Config file | none new; `web/vitest.config.*` as existing |
| Quick run command | `go test ./internal/host/hostaction/ ./internal/httpapi/ ./internal/ -count=1 -run 'Host|Process|Units|Script'` |
| Full suite command | `./bin/task ci` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| HACT-01..04 | each route places exactly `<action> <16hex>\n` and answers 202 with that id | integration (httpapi harness, temp data dir) | `go test ./internal/httpapi -run TestHostActions -count=1` | ❌ Wave 0 |
| HACT-01..04 | script maps each action to the fixed argv; order consumed before the stub runs | integration (bash + stub, unshare) | `go test ./internal/host/hostaction -run TestHostScript -count=1 -v` | ❌ Wave 0 |
| HACT-01..04 | writer → script → reader end to end (Go places, script runs, Go parses `last`, ids match) | integration | `go test ./internal/host/hostaction -run TestOrderRoundTrip -count=1` | ❌ Wave 0 |
| HACT-05 | no sudo → 428; reader → 403; wrong/missing typed → no token (422 field `typed`); no/foreign token → 403; node token refused; audit entry `host.<action>` present | integration | `go test ./internal/httpapi -run 'TestHostActions|TestHostConfirm' -count=1` | ❌ Wave 0 |
| HACT-05 | host table all-true, exactly four, disjoint from `typedPhrase` | unit | `go test ./internal/httpapi/handlers -run 'TestEveryHostActionRequiresTyping|TestEveryConfirmedActionIsInTheTable' -count=1` | ❌ Wave 0 (+ edit existing) |
| HACT-06 | daemon starts no process (R1) | guard | `go test ./internal -run TestTheDaemonStartsNoProcess -count=1 -v` | ❌ Wave 0 |
| HACT-06 | 17+ malformed orders rejected, stub never called, journal line has no order bytes, `last` shows `rejected` | integration | `go test ./internal/host/hostaction -run TestHostScript -count=1 -v` | ❌ Wave 0 |
| HACT-06 | concurrent places → one 202, one 409; D-13 withdrawal after timeout; startup sweep | unit | `go test ./internal/host/hostaction -run 'TestPlace|TestWithdraw|TestSweep' -count=1` | ❌ Wave 0 |
| HACT-06 | `last` reader strict (size, 4 fields, patterns, `-` only with `rejected`, RFC 3339) | unit | `go test ./internal/host/hostaction -run TestReadResult -count=1` | ❌ Wave 0 |
| HACT-07 | detection per missing item (script absent / not root / not executable / group-writable; unit absent; not enabled) | unit (fstest.MapFS) | `go test ./internal/host/hostaction -run TestHelper -count=1` | ❌ Wave 0 |
| HACT-07 | helper missing → 409 and the data dir holds no `host-order` and no temp file | integration | `go test ./internal/httpapi -run TestHostActions/helper_missing -count=1` | ❌ Wave 0 |
| HACT-07 | page: buttons off with each reason, helper notice lists only missing items, commands `pre` | component | `npm --prefix web run test -- HostActions host` | ❌ Wave 0 |
| HACT-08 | units verify clean + negative control; constants/paths/commands agree across Go, units, script, HOST-HELPER.md; goreleaser lists the four files; update script never names the helper (D-19) | guard | `go test ./internal/host/hostaction -run TestUnits -count=1 -v` | ❌ Wave 0 |
| all | problem codes in contract; budget rows; publicrepo | existing guards | `go test ./internal/httpapi ./cmd/holzkube-managerd ./internal/publicrepo -count=1` | ✅ (entries to add) |
| UI | 390 px: four buttons ≥ 44 px, no overflow from the helper notice | browser + audit | `npm --prefix web run test:browser` ; `./bin/task test:layout` | ✅ extend `host.browser.test.tsx` |

### Fault-injection table (each: put in, run, seen red, reverted — record the failing line in the SUMMARY)

| # | Injection | Must go red |
|---|-----------|-------------|
| F1 | `Destructive: false` on `host.reboot` | TestHostActions "no sudo window → 428" |
| F2 | host confirm skips the typed comparison | TestHostConfirm "wrong hostname gets no token" |
| F3 | `MinRole: model.RoleReader` on the action routes | TestHostActions "reader → 403" |
| F4 | `Action: ""` on `host.poweroff` | TestHostActions "audit entry host.poweroff present" (the middleware skips routes without an Action, so the test must look for the entry, not for a status) |
| F5 | action handler skips `Confirmer.Check` | TestHostActions "no token → 403" |
| F6 | `hostIntentTarget` replaced by the path's machine id / host actions added to `typedPhrase` | cross-token test; TestEveryHostActionRequiresTyping |
| F7 | `exec.Command("systemctl", "reboot")` in `internal/host` | TestTheDaemonStartsNoProcess (both halves: own-import and AST) |
| F8 | `syscall.ForkExec(...)` in `internal/host` (no os/exec import) | TestTheDaemonStartsNoProcess (AST half alone) |
| F9 | script executes without the pattern (`read -r a i < "$work/o"; "$SYSTEMCTL" "$a"`) | TestHostScript "foreign word" (stub called with `halt`) |
| F10 | `dd … iflag=nonblock` without `nofollow` **and** the `-L` classification removed | TestHostScript "symlink to a valid order" (stub called) — the target must hold a *valid* order or the injection injects nothing |
| F11 | `iflag=nofollow` without `nonblock`, `-f` check removed | TestHostScript "FIFO" fails by the test's own timeout (never by hanging CI: `exec.CommandContext` with 5 s) |
| F12 | `rm` moved after the `systemctl` call | TestHostScript: stub records whether the order file still exists when called → red |
| F13 | age window removed | TestHostScript "stale" and "future" |
| F14 | wants-symlink check removed | TestHelper "not enabled"; TestHostActions/helper_missing (file absent check) |
| F15 | `os.Link` replaced by `os.Rename` in PlaceNew | TestPlace "second place while pending → ErrExist" |
| F16 | withdraw timer not armed | TestWithdraw "not picked up in time → gone and reported" |
| F17 | `ProtectHome=true` → `ProtectHom=true` in the shipped unit | TestUnits verify gate (output non-empty); the built-in control proves the gate reads output |
| F18 | one character changed in HOST-HELPER.md's install block | TestUnits "commands byte-identical" |
| F19 | update script extracts `deploy/holzkube-manager-host.sh` | TestUnits "D-19: update script never names holzkube-manager-host" |
| F20 | `CodeHostOrderPending` row removed from api-contract.md | TestEveryProblemCodeIsInTheContract |
| F21 | HostActions enables buttons for a reader | component test "reader: all off with reason" |

### Sampling Rate

- **Per task commit:** the quick command above for the package touched, plus `bash -n deploy/holzkube-manager-host.sh`.
- **Per wave merge:** `go test ./... -count=1` and `npm --prefix web run test`.
- **Phase gate:** `./bin/task ci` green (exit code read from the command itself), `go test ./internal/publicrepo/ -count=1`, and the fault table above filled in the SUMMARYs. Optional Pi probe: a transient user path unit (`systemd-run --user --path-property=PathExists=<tmp>/host-order …`) running the real script with the stub against a dev daemon's temp data dir — measured workable in this session; never the system manager.

### Wave 0 Gaps

- [ ] `internal/host/hostaction/` package with `hostaction_test.go`, `result_test.go`, `helper_test.go`, `script_test.go`, `units_test.go`
- [ ] `internal/processguard_test.go` (or an addition to `depguard_test.go`)
- [ ] fsstore tests for `PlaceNew`/`Claim` (mode 0600, EEXIST, ENOENT, temp swept)
- [ ] `internal/httpapi/hostapi_test.go` extended (`TestHostActions`, `TestHostConfirm`); `handlers/confirm_test.go` extended
- [ ] `web/src/components/HostActions.test.tsx`; `host.test.tsx`/`host.browser.test.tsx` extended; `web/fixtures/demo.json` gains the helper-missing state
- [ ] no framework install needed

## Security Domain

### Applicable ASVS Categories (L1)

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes (re-auth) | existing sudo window (`middleware.Sudo`, 428 → `SudoDialog`) |
| V3 Session Management | yes | existing session + CSRF chain; no change |
| V4 Access Control | yes | route `MinRole: model.RoleOperator`; token bound to `Intent{Action:"host.*", Machine:"@host"}`; daemon has no privilege, helper accepts four fixed orders |
| V5 Input Validation | yes | typed hostname exact compare; order: size ≤ 64, exactly one line, anchored ERE under `LC_ALL=C`, no eval/unquoted use; result file strict parser |
| V6 Cryptography | yes (minor) | `crypto/rand` ids; HMAC tokens from `jobs.Confirmer` — nothing hand-rolled |
| V7 Error Handling & Logging | yes | audit Attempt before action; journal logs reason + byte count only; problem codes, never order text |
| V12 Files & Resources | yes | `O_NOFOLLOW|O_NONBLOCK` read, consume-before-act, root writes only in its own `StateDirectory`, 0600 order file for `fsstore.Guard` |
| V14 Configuration | yes | helper unit sandbox verified by `systemd-analyze`; daemon unit unchanged |

### Known Threat Patterns for this design

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Compromised daemon injects commands through the order | Tampering / EoP | fixed argv per action; ERE + exact size; no eval (F9) |
| Symlink swap to make root read/log a foreign file | Info Disclosure | `dd iflag=nofollow`; content never logged (F10) |
| FIFO / device to hang root | DoS | `nonblock`; `-f` classification; `PrivateDevices=true`; `TimeoutStartSec=3min` (F11) |
| Non-consumable entry (directory) to spin the helper | DoS | service start limit → path unit fails (measured); daemon D-13 message names the recovery |
| Order replayed after boot | Tampering (unintended action) | consume-first + age window + startup sweep (F12, F13) |
| Token for a node reused for the host | EoP | different Action namespace + `@host` mark; separate table (F6) |
| Reader or no-sudo session triggers a reboot | EoP | MinRole + Destructive, both enforced at composition (F1, F3) |
| Root writes into a directory the daemon controls | EoP | results only in `/var/lib/holzkube-manager-host` (root, 0755, own `StateDirectory`); the only operation in the data dir is `rm` of one name |
| Env overrides redirect the root script | Tampering | unit sets no `Environment=`; accepted as T-11-10 was |
| Compromised daemon reboots the host at will | DoS | **accepted**: a process holding the cluster PKI can do worse; the helper cannot do anything but the four actions |
| Real identifiers leaking into deploy files or docs | Info Disclosure | only documentation values; `internal/publicrepo` gate; nothing copied from the installed unit |

## Sources

### Primary (HIGH confidence)
- Local `man systemd.path`, `systemd.special`, `systemd.exec` (ReadWritePaths, `-` prefix), `systemd.service` (oneshot, TimeoutStartSec, RemainAfterExit), `systemctl` (reboot, `--check-inhibitors`) — systemd 257.13 on the Pi.
- Measurements this session: `dd iflag=nofollow,nonblock` on regular/symlink/FIFO/dir/oversize/missing; `systemd-analyze verify` on draft units (clean, missing ExecStart, `--root`, unknown key, bad value, typo control); `systemd-analyze security --offline=true`; transient user path units (dot temp, rename trigger, consumption, level trigger at start, start-limit failure); `renameat2(RENAME_NOREPLACE)` via x/sys; `go list -deps` import graph; `unshare --user --map-root-user`; a prototype of the helper script against 21 order cases.
- Repository files read this session: `internal/httpapi/router.go` (Route fields, composition panics :325-350, chain :430-492), `handlers/host.go`, `handlers/jobs.go` (:36-111, :270-470), `handlers/power.go`, `handlers/authority.go` (:225-290), `handlers/confirm_test.go`, `internal/jobs/confirm.go`, `internal/audit/redact.go` (:216-230), `internal/httpapi/middleware/audit.go` (:60-107), `internal/httpapi/problem.go` (codes, `Conflict` :460-468), `contract_codes_test.go`, `internal/store/store.go` (:36-45), `internal/store/fsstore/{atomic.go,permissions.go,permissions_test.go}`, `internal/history/persist.go` (:80-107), `internal/host/{host.go,sys.go,collector.go}`, `internal/config/config.go` (:276-291), `internal/depguard_test.go`, `cmd/holzkube-managerd/main.go` (:727-753), `cmd/holzkube-managerd/budget_test.go` (:1740-1830), `deploy/holzkube-manager-update.sh` (header, overrides, self-replace :436-439), `.goreleaser.yaml` (archives), `Taskfile.yml` (ci, test tasks), `internal/model/{model.go,job.go}`.
- Installed units (read-only `systemctl cat`): `holzkube-manager-update.timer`, `holzkube-manager-update.service`, and the daemon unit's hardening lines.

### Secondary (MEDIUM confidence)
- Phase artifacts: 13-CONTEXT.md, 13-UI-SPEC.md, 11-02-SUMMARY.md, 11-SECURITY.md, 12-08-SUMMARY.md.

### Tertiary (LOW confidence)
- Training knowledge for A1–A6 (listed in the Assumptions Log).

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — nothing new; every primitive measured on the target.
- Architecture: HIGH — follows three existing patterns (route loop, fsstore injection, script test under unshare); the refinements R1–R9 are each backed by a measurement.
- Pitfalls: HIGH for 1–6, 9–12 (measured or read); MEDIUM for 7–8 (sandbox × logind behaviour only provable at install).

**Research date:** 2026-09-29
**Valid until:** 2026-10-29 (systemd and Go are stable here; re-check if the Pi's systemd major changes).
