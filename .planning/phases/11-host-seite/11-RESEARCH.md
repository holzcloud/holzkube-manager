# Phase 11: Host-Seite — Gerät, Dienst, Live-Werte - Research

**Researched:** 2026-09-28
**Where this ran:** on the operator's Pi (aarch64, Raspberry Pi 5, Debian 13, kernel 6.18.50+rpt-rpi-2712). Every `/proc`/`/sys` fact below tagged `[VERIFIED: measured on the Pi]` was read on this machine this session. The production service was **not** touched: its unit was read with `systemctl cat`/`systemctl show` and its mount table with `cat /proc/<pid>/mountinfo`, both read-only.
**Domain:** local Linux host introspection (procfs, sysfs, syscalls) under systemd hardening; Go HTTP read route; React page reusing the node-hardware components; a bash update script that records its outcome.
**Confidence:** HIGH (nearly every load-bearing claim was measured on the target machine or read from the repository)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Datenquelle und Lesbarkeit (HMON-06)

- **D-01:** Neues Paket `internal/host`, das **lokal** liest — über ein
  `fs.FS`, das in Produktion auf `/` zeigt und in Tests auf einen Fixture-Baum
  (`testdata/pi5/`, `testdata/amd64/`, `testdata/procsubset-pid/`). Kein
  Subprozess, kein D-Bus, kein Root. Syscalls nur, wo sie kein `/proc` brauchen
  (`uname`, `statfs`, `clock_gettime`).
- **D-02:** Jeder Abschnitt der Antwort trägt seine Lesbarkeit selbst:
  `readable: bool` plus `reason` (Code + Satz) statt eines Wertes. Das Muster ist
  `health.Field[T]` (available / unavailable_reason); ob `Field[T]` direkt
  verwendet oder ein schmaler Host-Typ gebaut wird, entscheidet der Plan —
  bindend ist: ein nicht lesbarer Wert hat **keinen** Zahlenwert im JSON (kein
  `0`, kein `omitzero`, das zu `0` würde).
- **D-03:** „Durch die Härtung verborgen" wird **deterministisch** erkannt,
  nicht geraten: `/proc/self/mountinfo` (unter `ProcSubset=pid` weiter lesbar)
  nennt für das `/proc`-Mount die Option `subset=pid`. `ENOENT` auf
  `/proc/stat`, `/proc/meminfo`, `/proc/loadavg` **und** `subset=pid` im
  Mount → Ursache `hardening.proc-subset` mit dem Satz, dass die Unit
  `ProcSubset=pid` setzt, und der einen Zeile, die es ändert:
  `ProcSubset=all`. Jeder andere Lesefehler → Ursache `read-failed` mit dem
  Fehler. Nie eine stille 0.
- **D-04:** Was unter `ProcSubset=pid` trotzdem geht, wird **nicht** über `/proc`
  gelesen: Hostname und Kernel über `uname(2)`, Laufzeit seit dem Boot über
  `clock_gettime(CLOCK_BOOTTIME)` (`/proc/uptime` ist verborgen, HOST-01 steht
  aber nicht in der Liste der erlaubt-unlesbaren Werte), Modell über
  `/sys/firmware/devicetree/base/model` (Pi; das abschließende NUL wird
  abgeschnitten) bzw. `/sys/class/dmi/id/{sys_vendor,product_name}` (amd64),
  Betriebssystem aus `/etc/os-release` (`PRETTY_NAME`), Architektur aus
  `runtime.GOARCH` plus `uname -m`, Kernzahl aus
  `/sys/devices/system/cpu/online`.
- **D-05:** CPU-Auslastung (`/proc/stat`), Speicher und Swap (`/proc/meminfo`)
  werden aus `/proc` gelesen und stehen unter der Produktions-Unit als „nicht
  lesbar" da. Load kommt aus `/proc/loadavg` und, wenn das verborgen ist, aus
  `sysinfo(2)` — dieselben drei Werte, exakt (vom Orchestrator nachträglich
  entschieden, Erfolgskriterium 3 entsprechend angepasst). Der Guard-Test läuft gegen den Fixture-Baum `procsubset-pid` und ist
  gegen eine wieder eingesetzte 0 rot zu sehen.

#### Was die Seite zeigt

- **D-06:** Drei Abschnitte, von oben: **Gerät** (Hostname, Modell,
  Architektur, OS, Kernel, Laufzeit seit Boot), **Dienst** (laufende Version,
  Laufzeit des Prozesses, Datenverzeichnis: Größe, Dateisystem, frei,
  Update-Status), **Live** (CPU/Load, Speicher/Swap, Datenträger, Temperaturen
  und Lüfter, Netzwerk). Ein Hinweisblock oben, nur wenn etwas wegen der Härtung
  fehlt: welche Werte, warum, welche Zeile.
- **D-07:** Datenträger: genau `/` und das Dateisystem, auf dem das
  Datenverzeichnis liegt, per `statfs`; welches Mount das ist, aus
  `/proc/self/mountinfo`. Liegen beide auf demselben Dateisystem, eine Zeile mit
  beiden Namen, nicht zwei gleiche Balken. Größe des Datenverzeichnisses per
  Verzeichnislauf, **gecacht für 60 s** (auf einer SD-Karte liest man nicht bei
  jedem Poll den Baum).
- **D-08:** Temperaturen: `/sys/class/thermal/thermal_zone*` und
  `/sys/class/hwmon/hwmon*`, der hwmon-Zwilling einer thermal zone wird wie bei
  den Knoten zusammengelegt (`talos.ClassifyChip`-Logik wiederverwenden, nicht
  kopieren). Nur `temp*_input` und `fan*_input`; Spannungen (`in*_input`,
  z. B. `rp1_adc`) werden nicht gezeigt. Kein Lüfter gemeldet → „kein Lüfter
  gemeldet", nicht eine leere Liste ohne Satz.
- **D-09:** Netzwerk: je Schnittstelle Durchsatz aus
  `/sys/class/net/<if>/statistics/{rx,tx}_bytes`. Standardmäßig nur physische
  Schnittstellen (`/sys/class/net/<if>/device` existiert); `lo`, `veth*`,
  Docker-Brücken sind virtuell und werden eingeklappt als „N virtuelle
  Schnittstellen" gezeigt, nicht verschwiegen.
- **D-10:** Die Oberfläche ist Englisch wie der Rest des Produkts: „Not
  readable", „Not recorded", Seitentitel „Host". Die deutschen Wörter in
  Requirements und Roadmap sind die Anforderung, nicht der UI-Text.

#### API und Live-Aktualisierung

- **D-11:** Eine Leseroute `GET /api/v1/host`, `RequiresSession`,
  `MinRole: RoleReader`, kein `Action` (Lesen wird nicht auditiert). Sie liefert
  Identität, Dienst, Update-Status und Live-Werte in **einer** Antwort mit
  `observed_at` — eine Antwort, eine Uhr, wie `HardwareView`.
- **D-12:** Live heißt Polling wie bei `NodeHardware` (`useQuery` mit
  `refetchInterval: HARDWARE_POLL_INTERVAL_MS`), kein SSE. Raten (CPU %,
  Netzwerk-Bytes/s) rechnet der Server aus der Differenz zum vorigen Zähler,
  den ein Collector im Prozess hält — gleiches Muster wie `hardwareMemo` /
  `usableBaseline` in `internal/inventory/hardware.go`; der erste Aufruf nach
  dem Start hat für Raten noch keine Basis und sagt das („noch keine zweite
  Messung"), statt 0 zu zeigen.
- **D-13:** Die Elementtypen für Temperatur, Lüfter, Schnittstelle und
  Dateisystem übernehmen die JSON-Form von `inventory.HardwareTemperature`,
  `HardwareFan`, `HardwareLink`, `HardwareFilesystem`, damit die Anzeige-Bausteine
  von `NodeHardware.tsx` (Meter, Sensorliste, `temperatureLimits`) geteilt statt
  kopiert werden. Zod-Schema in `web/src/api.ts` mit
  `.nullish().transform(orEmpty)` für Listen (Ledger 162), Go-Seite mit
  `make()` — kein `null` im Dokument.

#### Update-Status (HOST-03)

- **D-14:** `deploy/holzkube-manager-update.sh` schreibt nach jedem Lauf als
  root eine kleine JSON-Datei `/var/lib/holzkube-manager-update/status.json`
  (Verzeichnis root, 0755; Datei 0644; atomar über `mktemp` im selben
  Verzeichnis und `mv`): `checked_at` (RFC 3339, UTC), `installed`, `latest`,
  `outcome` (`current` | `available` | `updated` | `rolled-back` | `failed`).
  **Nicht** ins Datenverzeichnis des Daemons: root schreibt nicht in ein
  Verzeichnis, das dem unprivilegierten Benutzer gehört (Symlink-Falle).
- **D-15:** Das Schreiben ist Nebensache: es darf ein Update nie scheitern
  lassen (`|| true`, eigene Funktion), und `--check` ohne root schreibt nichts,
  wenn das Verzeichnis nicht schreibbar ist. Ein Test fährt das Skript gegen ein
  Test-Verzeichnis mit Stellvertretern für `curl`/`systemctl`.
- **D-16:** Der Daemon liest die Datei (Pfad per Flag/Env überschreibbar,
  Größenlimit, strenges Parsen). Fehlt sie → „Not recorded" mit dem Satz, dass
  das installierte Update-Skript nichts hinterlegt; kaputt → „Not readable" mit
  Ursache. Nie eine erfundene Zeit oder Version.

#### Container-Betrieb (Docker / compose.yaml)

- **D-17:** Der Daemon erkennt, dass er in einem Container läuft
  (`/.dockerenv`, `/run/.containerenv`, oder `container=` in der Umgebung von
  PID 1 soweit lesbar) und sagt es oben auf der Seite: „holzkube-manager runs in
  a container. Kernel, CPU, memory and temperatures are the host's; hostname,
  network and filesystems are the container's." Die Werte werden gezeigt, aber
  Hostname und Netzwerk tragen den Zusatz „container". Phase 13 baut darauf
  (keine Host-Aktionen im Container).

### Claude's Discretion

- Paketaufteilung innerhalb von `internal/host` (Collector, Leser, Typen),
  Namen der Go-Typen, genaue Formulierung der englischen Sätze.
- Ob `/api/v1/host` in `handlers/host.go` neu entsteht (erwartet) und wie die
  Deps-Struktur `httpapi.Deps` den Collector bekommt.
- Aufbau der React-Komponente (`routes/host.tsx` plus geteilte Bausteine aus
  `NodeHardware.tsx` herausgezogen), solange nichts dupliziert wird, was schon
  existiert.

### Ohne Rückfrage entschieden (CONTEXT.md, verbatim summary of rejected alternatives)

- *Speicher und Swap über `sysinfo(2)` lesen* — verworfen (kein `MemAvailable`, kein Page-Cache). Load dagegen exakt über `sysinfo` (D-05).
- *Unlesbarkeit am `ENOENT` allein erkennen* — verworfen; `subset=pid` im Mount ist der Beweis.
- *Update-Status ins Datenverzeichnis des Daemons schreiben* — verworfen (Symlink-Falle).
- *Update-Status aus dem Journal der Update-Unit lesen* — verworfen (kein `AF_UNIX`/D-Bus).
- *SSE statt Polling* — verworfen.
- *Alle Schnittstellen gleichrangig zeigen* — verworfen.
- *Im Container die Seite ganz sperren* — verworfen.
- *UI-Texte deutsch* — verworfen.

### Deferred Ideas (OUT OF SCOPE)

- Eine Referenz-Unit `deploy/holzkube-manager.service` im Repository — nicht
  verlangt; die Anleitung in Phase 13 beschreibt nur die Helfer-Units.
- Spannungen (`in*_input`) und Drosselungs-Flags des Pi (`vcgencmd
  get_throttled` bräuchte einen Subprozess) — nicht in diesem Milestone.
- Wunsch „Hardware-Temperaturen" (2026-09-21): die Knoten sind seit v0.0.1
  über Talos abgedeckt, der Manager-Pi selbst mit dieser Phase; die dritte
  genannte Quelle (Prometheus im Cluster) bleibt ungebaut.

Also out of scope per the phase boundary: history / `internal/history` entries, warning thresholds and a host health state, wall tile, a nav entry *with* state (a plain nav entry is allowed), actions/helper (Phase 13), tap-target work beyond what `ui/` already brings (Phase 14).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| HOST-01 | Gerät: Hostname, Modell, Architektur, OS, Kernel, Laufzeit seit Boot | Measured sources that survive `ProcSubset=pid`: `uname(2)`, `CLOCK_BOOTTIME`, device-tree model (NUL-terminated), `/etc/os-release` (symlink), `/sys/devices/system/cpu/online`. Syscall filter of the production unit allows all of them (§ Measured host facts). |
| HOST-02 | Dienst: Version, Prozesslaufzeit, Größe und freier Platz des Datenverzeichnisses | `httpapi.Deps.Version` already carries the ldflags version; start time captured in `main`; data dir is a **separate bind mount** in the unit's namespace (measured) → dedupe by device number; `statfs` + `du`-compatible walk, cached 60 s with singleflight. |
| HOST-03 | Update-Status soweit hinterlegt | Script design with one EXIT trap, python3 JSON emission, overridable paths for tests; daemon-side strict parser; "Not recorded" when absent (the Pi's installed script records nothing — measured: `/var/lib/holzkube-manager-update` does not exist). |
| HMON-01 | CPU, Load, Speicher, Swap live | `/proc/stat` + `/proc/meminfo` parsers (formula matches `free -b` exactly — measured); load from `/proc/loadavg`, else `sysinfo(2)` rendered with the kernel's own rounding; server-side rates with a baseline memo. |
| HMON-02 | Belegung `/` und Datenverzeichnis-Dateisystem | `statfs` with `f_frsize`; `used = (blocks−bfree)·frsize`, `available = bavail·frsize` reproduce `df -B1` byte-for-byte (measured). |
| HMON-03 | Temperaturen, Lüfter | hwmon + thermal-zone walk reusing `talos.ClassifyChip` and the twin rule; Pi 5 layout measured (cpu_thermal, rp1_adc, rpi_volt, no fan). |
| HMON-04 | Netzwerk-Durchsatz je Schnittstelle | sysfs statistics; physical = `device` exists (measured: eth0, wlan0 yes; lo, docker0, bridge, 2 veth no); `speed` returns EINVAL on a down link (measured). |
| HMON-06 | Härtung → „nicht lesbar" mit Ursache, nie 0 | Real kernel behaviour of `subset=pid` measured in an unprivileged user namespace; production mountinfo measured (`hidepid=invisible,subset=pid` in the **super options** of the single `/proc` entry). Readability type that cannot emit a number when unreadable; guard seen red against a reinstated 0. |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Production target is linux/arm64 on the operator's Pi.** Say where every check ran. This research ran on the Pi; amd64 facts come from fixtures/docs only.
- **Guards must be seen red against the reinstated fault.** Every test the plan adds needs a named fault injection and the observed red (see Validation Architecture → "Fault injections").
- **No `go test -race` on the Pi** (TSan refuses the 47-bit address space); CI runs `-race` on ubuntu and macOS. `./bin/task test` already drops `-race` on aarch64.
- **Replacing the production binary, restarting the service, or copying its data directory is the operator's call.** Verification on the Pi uses a *separate* dev daemon with a temp data dir (HANDOVER §3.1 recipe), never the production service.
- **Decisions put to the operator are choices, never open questions.** Autonomy ordered for this phase: decisions below are made and recorded, not asked.
- **Never write the machine's real hostname, IPs or domains into any file** (`internal/publicrepo` rejects them by hash; MAC addresses and `/home/<user>` shapes too). Fixtures must be hand-written, never copied from `/sys` (the `address` files carry real MACs).
- **Toolchain:** gates build with `GOTOOLCHAIN=go1.26.7` (go.mod `toolchain go1.26.7`); the Pi's installed go1.27.1 is only for `test:next`.
- Memory notes in effect: read exit codes from the command itself (no pipes hiding failure); run the pinned golangci-lint (`./bin/golangci-lint`, v2.13.1) before any push.

## Summary

The phase is well-bounded and almost entirely **measurable on this machine**, which was done. The two facts that shape the design most were not in CONTEXT.md and came out of measurement:

1. **Inside the production unit the data directory is its own mount.** `ProtectSystem=strict` + `StateDirectory=` makes systemd bind-mount `/var/lib/holzkube-manager` over itself; the daemon's `/proc/<pid>/mountinfo` shows `179:2 /var/lib/holzkube-manager /var/lib/holzkube-manager … ext4 /dev/mmcblk0p2` next to `179:2 / / … ext4 /dev/mmcblk0p2`. A longest-prefix mount lookup therefore names the data dir's filesystem `/var/lib/holzkube-manager`, and deduping by mount point draws **two identical bars** — the exact thing D-07 forbids. Dedupe must be by device number (`major:minor`), and the row's mount label must be `/` (the entry whose mountinfo *root* is `/`).
2. **`subset=pid` lives in the super-options field** of mountinfo (after ` - proc proc `), not in the per-mount options, and there can be **two** `/proc` entries (stacked) — measured in a user namespace: the old one first, the new `subset=pid` one last, parented on the old one. In the production unit there is exactly one. The detector must read the super options of the **topmost** `/proc` mount.

Other load-bearing findings: `health.Field[T]` cannot be used as-is — its `Value T \`json:"value,omitzero"\`` drops a *readable* zero (0.0 % CPU, 0 swap) from the JSON, making a real 0 indistinguishable from "no value"; build a narrow `Reading[T]` with a pointer value instead. The existing hardware zod schema uses `.default(0)` on every number, so reusing it for `/host` would turn an absent number back into 0 in the browser — share only the temperature and fan element schemas; links and filesystems need nullable variants. The daemon ships for **darwin/arm64** and CI runs `go test ./...` on macOS, so every syscall lives behind a `linux` build tag with a non-Linux stub. `TestNoDirectFileAccessOutsideFsstore` forbids `os.ReadFile/Open/ReadDir/Readlink` outside exempt dirs — D-01's `fs.FS` design (`fs.ReadFile(os.DirFS("/"), …)`) complies without an exemption. The update script's `set -euo pipefail`, its many `fail … exit 1` paths and its existing `trap … EXIT` mean the status write has to be **one EXIT trap installed once**, which must never change the script's exit code.

**Primary recommendation:** Build `internal/host` as (a) pure parsers over `fs.FS` + a `Sys` interface (linux impl via `golang.org/x/sys/unix`, stub elsewhere), (b) a `Collector` holding the rate baseline and the 60-s data-dir cache, (c) a `Reading[T]` wire type whose value is a pointer (absent when unreadable); detect the hardening from the topmost `/proc` entry's super options; dedupe filesystems by `major:minor`; make the update script record through a single EXIT trap and test it end-to-end against the daemon's own parser.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Reading `/proc`, `/sys`, `/etc/os-release`, syscalls | API / Backend (`internal/host`) | — | Only the daemon's process sees its own namespace; the hardening applies to it, not to the browser. |
| Hardening detection (`subset=pid`) and reason codes | API / Backend | Browser (maps code → copy) | The reason is a fact about the server's mount namespace; the UI only chooses wording (UI-SPEC Copywriting). |
| Rates (CPU %, bytes/s) | API / Backend (Collector memo) | — | D-12: server computes from counter deltas; one clock (`observed_at`). |
| Data-dir size walk + 60-s cache | API / Backend | — | Filesystem access; SD-card cost. |
| Update status recording | Host OS (root, `deploy/holzkube-manager-update.sh`) | API / Backend (reader) | Only the root timer knows what happened; daemon reads a root-owned file. |
| Readability rendering ("Not readable", "Waiting…", "Not recorded") | Browser (`routes/host.tsx`) | — | Presentation; must branch on readability before formatting. |
| Polling every 3 s | Browser (TanStack Query) | — | Same as `NodeHardware`. |
| Access control (reader+, session) | API / Backend (route table) | — | `httpapi.Route` flags. |

## Measured Host Facts (this Pi)

All `[VERIFIED: measured on the Pi]` unless tagged otherwise. Hostname, IPs and domains deliberately omitted.

### Identity sources (HOST-01)

| Value | Source | Measured |
|-------|--------|----------|
| Model | `/sys/firmware/devicetree/base/model`, mode 0444 | `"Raspberry Pi 5 Model B Rev 1.0\x00"` — trailing NUL confirmed with `od -c` and via Go `fs.ReadFile` |
| DMI | `/sys/class/dmi` | **absent** on the Pi (`No such file or directory`) |
| OS | `/etc/os-release` | `PRETTY_NAME="Debian GNU/Linux 13 (trixie)"`; the file is a **symlink** (`stat` shows 777) to `../usr/lib/os-release` |
| Cores | `/sys/devices/system/cpu/online` | `0-3` |
| Kernel | `uname(2)` release | `6.18.50+rpt-rpi-2712`; machine `aarch64`; `runtime.GOARCH` `arm64` |
| Boot uptime | `clock_gettime(CLOCK_BOOTTIME)` | `26090.339277810` vs `/proc/uptime` `26090.33` — matches. `sysinfo(2).uptime` said `26091` (**rounds up**) — do not use sysinfo for uptime. |
| Load | `sysinfo(2).loads[]` | `/proc/loadavg` `7.29 4.90 2.98`; sysinfo rendered with the kernel's formula `7.29 4.90 2.98` (see Code Examples) |

### Memory (HMON-01) — formula check against `free -b` (procps-ng 4.0.4)

`used = MemTotal − MemAvailable`, `buff/cache = Buffers + Cached + SReclaimable`, `swap used = SwapTotal − SwapFree`, all ×1024 from kB: **every column of `free -b` reproduced to the byte** (total 8453947392, used 4093853696, buff/cache 3259285504, available 4360093696, swap used 1498169344). This is exactly `inventory.memoryView` [VERIFIED: internal/inventory/hardware.go:487-496].

### Filesystems (HMON-02) — `statfs` vs `df -B1 /`

`statfs("/")`: `bsize=4096 frsize=4096 blocks=30581165 bfree=23529716 bavail=22268670` → size 125260451840, `(blocks−bfree)·frsize` = 28882735104, `bavail·frsize` = 91212472320. `df -B1 /`: `125260451840 28882735104 91212472320 25%`. **Byte-identical.** Note `df`'s Use% = used/(used+avail) = 24.5 % → 25 %, not used/size (23.1 %): the ext4 root reserve (5 %) is neither used nor available. `statfs` type `0xef53` (ext2/3/4 magic) — take the type **name** from mountinfo (`ext4`), not the magic.

### Sensors (HMON-03)

```
hwmon0 -> ../../devices/virtual/thermal/thermal_zone0/hwmon0   name=cpu_thermal  files: temp1_input (64450)
hwmon1 -> ../../devices/platform/axi/…/1f000c8000.adc/hwmon/hwmon1  name=rp1_adc  temp1_input (55472), temp1_raw, in1..in4_input, in1..in4_raw
hwmon2 -> ../../devices/platform/soc@…/raspberrypi-hwmon/hwmon/hwmon2  name=rpi_volt  in0_lcrit_alarm only
thermal_zone0: type=cpu-thermal temp=65000, contains subdir hwmon0; trip points:
  0 critical 110000, 1 active 50000, 2 active 60000, 3 active 67500, 4 active 75000
/sys/class/thermal: only thermal_zone0 (no cooling_device*)
```
No `temp*_max`/`temp*_crit` on cpu_thermal; no fan hwmon anywhere. `/sys/class/hwmon/*` entries are **symlinks** (`fs.ReadDir` reports `IsDir()==false`, type `L---------`). All `*_input` files mode 0444, trip-point temps 0644 — readable by the unprivileged service user.

With the Talos-path rules applied to this layout (hwmon walk; CPU chip found, so zones are *not* added): temperatures = `cpu_thermal/temp1` (kind cpu) and `rp1_adc/temp1` (kind other); `rpi_volt` dropped (no temps, no fans); voltages never read; fans = `[]` → "no fan" sentence.

### Network (HMON-04)

| iface | `device` | operstate | speed | type |
|-------|----------|-----------|-------|------|
| eth0 | yes | up | 1000 | 1 |
| wlan0 | yes | down | **read → EINVAL** | 1 |
| lo | no | unknown | **read → EINVAL** | 772 |
| docker0, a `br-…` bridge, two `veth…` | no | up | 10000 (fake) | 1 |

Go: `fs.ReadFile(os.DirFS("/"), "sys/class/net/wlan0/speed")` → `read …: invalid argument`, `errors.Is(err, syscall.EINVAL)==true`, `errors.Is(err, fs.ErrNotExist)==false`. A down link's speed is "no speed", not a read failure.

### `ProcSubset=pid` — what actually disappears (HMON-06)

systemd's own man page (systemd 257 on this Pi) says ProcSubset/ProtectProc are "only available to system services" [CITED: `man systemd.exec`, ProcSubset=] — confirmed: `systemd-run --user -p ProcSubset=pid` ran but mounted **no** subset (every file still readable). Real kernel behaviour was measured instead with `unshare --user --map-root-user --mount --pid --fork` + `mount -t proc -o subset=pid,hidepid=invisible proc /proc`:

```
363 328 0:20 / /proc rw,relatime - proc proc rw
573 363 0:46 / /proc rw,relatime - proc proc rw,hidepid=invisible,subset=pid
FAIL /proc/stat, /proc/meminfo, /proc/loadavg, /proc/uptime, /proc/cpuinfo, /proc/mounts, /proc/net,
     /proc/sys/kernel/hostname: No such file or directory
OK   /proc/self/mountinfo, /proc/self/net/dev, /proc/self/stat, /proc/self/environ, /proc/1/environ
top-level of /proc: self thread-self (plus PID dirs)
```

Production daemon's namespace (read-only `cat /proc/<pid>/mountinfo`): **one** `/proc` entry, `337 335 0:53 / /proc rw,nosuid,nodev,noexec,relatime shared:384 - proc proc rw,hidepid=invisible,subset=pid`. Its `/sys` is the host sysfs (`0:19`, rw), `/boot/firmware` visible, `/dev` a private tmpfs, and the data dir a separate bind mount (see Summary).

### Production unit hardening that matters here (read with `systemctl show`)

`User=holzkube-manager`, `ProcSubset=pid`, `ProtectProc=invisible`, `ProtectSystem=strict`, `PrivateDevices=yes`, `PrivateTmp=yes`, `ProtectKernelTunables=yes`, `ProtectControlGroups=yes`, `ProtectClock=yes`, **`ProtectHostname=yes`**, `PrivateNetwork=no`, `RestrictAddressFamilies=AF_INET AF_INET6`, `SystemCallFilter=@system-service`, `SystemCallErrorNumber=EPERM`, `StateDirectory=holzkube-manager` (mode 0700). The **resolved** syscall allow-list contains `sysinfo`, `uname`, `statfs`, `statfs64`, `fstatfs`, `clock_gettime`, `getdents64`, `openat`, `newfstatat`, `statx`, `readlinkat` — every syscall this phase needs is permitted. Consequences:

- `ProtectHostname=yes`: the daemon has its own UTS namespace; `uname` returns the hostname **as of service start**. A later `hostnamectl set-hostname` is not seen until restart [CITED: `man systemd.exec`, ProtectHostname=]. Not a defect; document it as the reason a shell comparison can differ.
- `ProtectProc=invisible`: `/proc/1/environ` is invisible to the daemon on the host → container detection falls back to the marker files (correct: on the host none exist).
- `/var/lib/holzkube-manager-update` does **not** exist yet on the Pi (measured) → `/host` will say "Not recorded" until a release with the new script has run once.

## Standard Stack

### Core (Go)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib `io/fs`, `os.DirFS("/")` | go1.26.7 | All file reads through `fs.FS` (D-01) | `os.DirFS` implements `ReadFileFS`, `ReadDirFS`, `StatFS`, `ReadLinkFS` [VERIFIED: `go doc os.DirFS` with go1.26.7]; not on the file-access guard's forbidden list [VERIFIED: internal/store/fsstore/permissions_test.go:246-251] |
| `golang.org/x/sys/unix` | v0.47.0 (already in go.sum, `// indirect` in go.mod:101) | `Uname`, `Sysinfo`, `Statfs`, `ClockGettime(CLOCK_BOOTTIME)`, `ByteSliceToString`, `Major/Minor` | Official Go sub-repo; stdlib `syscall` lacks `ClockGettime` (the probe needed a raw `Syscall` + `unsafe`). `go mod tidy` promotes it to a direct requirement — no new module enters the graph. |
| `golang.org/x/sync/singleflight` | v0.22.0 (direct, go.mod:15) | One data-dir walk at a time | Already a direct dependency (`errgroup` used in internal/talos). |
| `internal/talos` (`ClassifyChip`, `SensorKind`, `CPUTimes`) | in-repo | Sensor classification, CPU times type | D-08 mandates reuse. Already linked into the binary, so importing it adds no weight. |
| `internal/inventory` (`HardwareTemperature`, `HardwareFan`, rate helpers) | in-repo | Shared element types and rate arithmetic | D-12/D-13. No cycle: inventory does not import host; history imports inventory (Phase 12 can import host). |

### Core (web) — nothing new

zod 4.4.3, @tanstack/react-query 5.102.6, @tanstack/react-router 1.170.32, lucide-react 1.34.0 (`Cpu` icon), shadcn `card`/`badge` already installed [VERIFIED: web/package.json]. UI-SPEC: no shadcn component added.

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `golang.org/x/sys/unix` | stdlib `syscall` only | `syscall.Sysinfo`, `Uname`, `Statfs` exist on linux, but `CLOCK_BOOTTIME` needs `Syscall(SYS_CLOCK_GETTIME, 7, unsafe.Pointer…)` — gosec noise and a hand-rolled constant. Use x/sys. |
| `health.Field[T]` | narrow `host.Reading[T]` | `Field[T]` has `Value T \`json:"value,omitzero"\`` [VERIFIED: internal/health/health.go:104] — a readable 0 vanishes, and `level`/`stale_since` mean nothing for a local read. Use the narrow type. |
| Reading hwmon via a new walker | calling talos's discovery | talos's discovery is bound to `*ClusterClient` gRPC (`listDir`/`readFile`); only the pure helpers are reusable. Export them (see Pattern 4). |

**Installation:** none. After first import of `golang.org/x/sys/unix`: `GOTOOLCHAIN=go1.26.7 go mod tidy` (moves x/sys from indirect to direct).

## Package Legitimacy Audit

No new external package is installed in this phase.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| golang.org/x/sys | Go module proxy | Go project sub-repo | — | go.googlesource.com/sys | already in go.sum (v0.47.0) | Approved — promotion indirect→direct only |
| golang.org/x/sync | Go module proxy | Go project sub-repo | — | go.googlesource.com/sync | already a direct dep (v0.22.0) | Approved — no change |

The gsd `package-legitimacy` seam covers npm/pypi/crates only; it was not run for Go modules. Both modules are already resolved and checksummed in `go.sum` [VERIFIED: go.sum:274-275 for x/sys].
**Packages removed due to [SLOP]:** none. **Packages flagged [SUS]:** none.

## Architecture Patterns

### System Architecture Diagram

```
Browser /host (routes/host.tsx)
   │  useQuery(['host'], api.host, refetchInterval 3000, retry:false)
   ▼
GET /api/v1/host ──► httpapi middleware (host check, session, MinRole reader; no audit)
   │
   ▼
handlers.host(d) ──► d.Host == nil ? 503-style Upstream problem : Collector.Read(ctx)
                         │
                         ├─► Sys (linux: x/sys/unix; other OS: "unsupported" errors)
                         │      uname · clock_gettime(BOOTTIME) · sysinfo · statfs
                         │
                         ├─► fs.FS rooted at "/" (tests: fixture tree)
                         │      proc/self/mountinfo ──► parse ──► procSubsetPid? , mounts by major:minor
                         │      proc/stat, proc/meminfo, proc/loadavg ──► ENOENT && subset=pid ? hardening.proc-subset
                         │                                                : other error ? read-failed : value
                         │      (loadavg hidden) ──► Sys.Sysinfo loads, kernel rounding
                         │      sys/firmware/devicetree/base/model | sys/class/dmi/id/* , etc/os-release,
                         │      sys/devices/system/cpu/online
                         │      sys/class/hwmon/*, sys/class/thermal/* ──► talos.ClassifyChip + twin rule
                         │      sys/class/net/*/{device,operstate,speed,statistics/*}
                         │      .dockerenv, run/.containerenv, proc/1/environ ──► container?
                         │      <update-status path> ──► strict parse ──► recorded | not-recorded | not-readable
                         │
                         ├─► memo (mutex): previous counters (At, CPU times, link bytes) ──► rates or rate.no-baseline
                         └─► data-dir size cache (60 s, singleflight, bounded walk)
                         │
                         ▼
                   host.View JSON (one observed_at) ──► zod hostSchema (discriminated readability) ──► render
```

### Recommended Project Structure

```
internal/host/
├── host.go            # View + section types, Reading[T], Reason, reason codes (package doc: why fs.FS, why not health.Field)
├── collector.go       # Collector{fs, sys, dataDir, statusPath, started, version}; Read(ctx); memo; size cache
├── sys.go             # type Sys interface { Uname; BootTime; Sysinfo; Statfs }
├── sys_linux.go       # //go:build linux — x/sys/unix implementation
├── sys_other.go       # //go:build !linux — every call returns errUnsupported (daemon ships for darwin/arm64)
├── mountinfo.go       # parse, unescape \040, topmost-at-mountpoint, superopts, mountFor(path)
├── proc.go            # /proc/stat, /proc/meminfo, /proc/loadavg parsers + classify(err, subsetPid)
├── identity.go        # model (DT / DMI), os-release, cpu online ranges, container detection
├── sensors.go         # hwmon + thermal walk over fs.FS using talos.ClassifyChip & exported helpers
├── network.go         # sysfs links, physical/virtual split
├── filesystems.go     # statfs rows, dedupe by major:minor, du-compatible size walk
├── updatestatus.go    # ReadUpdateStatus(fs, path) strict parser
├── testdata/pi5/…            # hand-written, documentation values only
├── testdata/amd64/…          # k10temp, nct6798 (fans), nvme, DMI, acpitz zone
├── testdata/procsubset-pid/… # pi5 tree minus proc/stat, proc/meminfo, proc/loadavg; mountinfo with subset=pid
└── *_test.go (incl. updatescript_test.go driving deploy/holzkube-manager-update.sh)
internal/httpapi/handlers/host.go   # HostRoutes(d)
web/src/routes/host.tsx             # page
web/src/components/charts/Sensors.tsx (name per planner) # Sensors, FanRows, temperatureLimits, sensorKey, KIND_LABEL moved out of NodeHardware.tsx
```

### Pattern 1: A readability type that cannot carry a number when unreadable

**What:** Every value that can be hidden is a `Reading[T]` whose value is a pointer; `omitempty` on a pointer omits only nil, so a readable 0 is emitted as `0` and an unreadable value has no `value` key at all.
**When:** every hideable section: CPU usage, per-core, memory (incl. swap), load, each filesystem row, each identity field, rates (no baseline), update status.

```go
// Source: design for this phase; semantics per encoding/json (pointer + omitempty omits only nil)
type Reason struct {
    Code    string `json:"code"`    // "hardening.proc-subset" | "read-failed" | "rate.no-baseline" | "update.not-recorded" | "unsupported"
    Message string `json:"message"` // the server's sentence
}

type Reading[T any] struct {
    Readable bool    `json:"readable"`
    Value    *T      `json:"value,omitempty"`  // present iff Readable
    Reason   *Reason `json:"reason,omitempty"` // present iff !Readable
}

func Read[T any](v T) Reading[T]             { return Reading[T]{Readable: true, Value: &v} }
func Hidden[T any](r Reason) Reading[T]      { return Reading[T]{Reason: &r} }
```

Browser side, a discriminated union forces the renderer to branch before formatting (UI-SPEC "the renderer must branch on readability **before** formatting"):

```ts
// Source: zod 4 discriminatedUnion; mirrors Reading[T]
const reasonSchema = z.object({ code: z.string(), message: z.string().default('') })
const reading = <T extends z.ZodTypeAny>(value: T) =>
  z.discriminatedUnion('readable', [
    z.object({ readable: z.literal(true), value }),
    z.object({ readable: z.literal(false), reason: reasonSchema }),
  ])
```
Do **not** put `.default(0)` anywhere inside a `reading(...)` value.

### Pattern 2: Hardening detection from the topmost `/proc` mount's super options

mountinfo line format (proc(5)): `ID PARENT MAJ:MIN ROOT MOUNTPOINT MOUNTOPTS [OPTIONAL…] - FSTYPE SOURCE SUPEROPTS`. Split on the literal separator field `-`; optional fields vary in count. Unescape `\040` (space), `\011` (tab), `\012` (newline), `\134` (backslash) in ROOT and MOUNTPOINT.

Topmost rule: among entries with `MOUNTPOINT == "/proc"` and `FSTYPE == "proc"`, pick the one whose ID is **not** the PARENT of another `/proc` entry (measured stacked case: `573` parented on `363`). Fall back to the last such line.

`classify(err error, subsetPid bool) Reason`:
- `errors.Is(err, fs.ErrNotExist) && subsetPid` → `hardening.proc-subset`, message "Hidden by the unit's ProcSubset=pid. Set ProcSubset=all in the unit's [Service] section to show it."
- otherwise → `read-failed`, message `"could not read /proc/stat: " + err.Error()`.

### Pattern 3: Filesystems — two paths, dedupe by device number

1. `rootEntry` = topmost mountinfo entry with MOUNTPOINT `/`.
2. `dataEntry` = topmost entry whose MOUNTPOINT is the longest path-prefix of the data dir (cleaned, absolute; prefix at a `/` boundary — `/var/lib/holzkube` must not match `/var/lib/holzkube-manager`).
3. If `rootEntry.MajMin == dataEntry.MajMin` (production: both `179:2`) → **one** row, mount label `/`, roles `["root", "data directory"]`. Otherwise two rows.
4. Values from `Sys.Statfs(path)` with `frsize` (fall back to `bsize` if 0): `size = blocks·frsize`, `used = (blocks−bfree)·frsize`, `available = bavail·frsize`. Device from mountinfo SOURCE (`/dev/mmcblk0p2` — visible even under `PrivateDevices`), type from FSTYPE.

JSON: extends the `HardwareFilesystem` shape (`mount`, `device`, `size_bytes`, `used_bytes`) with `available_bytes`, `fstype`, `roles` — wrapped in the row's readability. The meter percent that matches `df`'s Use% is `used / (used + available)`; UI-SPEC says `used/size` (see Open Questions, decided).

### Pattern 4: Reuse the talos sensor rules without the gRPC walker

`internal/talos/sensors.go` has the rules but binds discovery to `*ClusterClient`. Reusable today: `ClassifyChip` (exported) [VERIFIED: internal/talos/sensors.go:79-103]. Unexported but pure: `inputsNamed` (:636), `numbered` (:658), `millidegrees` (:689), and the twin rule inline in `thermalZones`: `if have[strings.ReplaceAll(typ, "-", "_")] {` (:558), plus "zones only when no CPU hwmon chip reported temps" (`if !cpuTemps {` :497). **Export** them additively (`InputsNamed`, `Numbered`, `Millidegrees`, `ThermalTwinName(zoneType string) string`, and make `thermalZones` call `ThermalTwinName`) so the node path and the host path share one rule. Do not copy.

The host walker then does, over `fs.FS`: list `sys/class/hwmon` (entries are symlinks — do **not** filter on `IsDir()`), read `name`, list the chip dir, `InputsNamed(files,"temp")`/`"fan"`, read `tempN_label/_max/_crit`, `fanN_label`; drop chips with no temps and no fans; zones only if no CPU chip had temps, skipping twins. Emit `inventory.HardwareTemperature` / `inventory.HardwareFan` values (same JSON form, D-13).

### Pattern 5: Collector memo and "no baseline yet"

```go
type counters struct {
    at    time.Time                 // time.Now(): carries the monotonic reading; never serialise this one
    cpu   *talos.CPUTimes           // nil when /proc/stat is unreadable
    cores []talos.CPUTimes
    links map[string]talos.LinkIO
}
```
- On each `Read`: take `cur`; if `prev` exists and `minRateWindow ≤ cur.at.Sub(prev.at) ≤ maxRateWindow` (0.5 s / 5 min — the inventory constants [VERIFIED: internal/inventory/hardware.go:58-59]) compute rates against `prev`; else rates are `Hidden(rate.no-baseline)`.
- Advance `prev = cur` only when the window was ≥ `minRateWindow` (two tabs, or the Phase-12 sampler interleaving, then never erase each other's baseline).
- **Do not** use `usableBaseline`'s `Boot.Equal` check with a boot time derived from `now − CLOCK_BOOTTIME`: nanosecond jitter makes two derivations unequal and no baseline would ever be usable. A process cannot outlive a reboot; drop the boot comparison for the local host.
- A link that appears between readings or whose counter went backwards has no rate this round: `rx/tx` = null, not 0 (the inventory's `perSecond` returns 0 for a backwards counter [VERIFIED: hardware.go:417-422] — acceptable there, not under D-02).
- Reuse the arithmetic: export `inventory.CPUPercent(prev, cur talos.CPUTimes) (busy, iowait float64)` (currently `cpuPercent`, :398) rather than re-deriving it.

### Pattern 6: Data-dir size — `du`-compatible, cached, bounded

`fs.WalkDir(os.DirFS(dataDir), ".")`, sum `st_blocks·512` from `info.Sys().(*syscall.Stat_t)` for every entry including directories, count each `(dev, ino)` once, never follow symlinks (WalkDir does not). That equals `du -s -B1 <dir>`. Guard with `singleflight.Group` + a 60-s cache + a context deadline (recommend 5 s; on timeout the size is `Hidden(read-failed: "the size walk took longer than 5 s")` and the cache keeps the last good value with its `measured_at`). `measured_at` is part of the answer (UI-SPEC "measured {ago}"). `syscall.Stat_t.Blocks` exists on linux and darwin; keep the extraction in the build-tagged file to be safe.

### Pattern 7: Non-Linux build

```go
//go:build !linux
func (osSys) Uname() (Uname, error) { return Uname{}, errUnsupported }
```
`errUnsupported` maps to reason code `unsupported` with "holzkube-manager reads the host on Linux only." On darwin every section is Not readable, the page still renders, `go test ./...` on macos-latest compiles and passes (fixture tests use the fake `Sys`).

### Anti-Patterns to Avoid

- **Deciding "hidden by hardening" on ENOENT alone** — a container without `/proc/stat` would be misreported (D-03), and systemd *silently* skips `subset=` on kernels without it [CITED: man systemd.exec, "gracefully disabled if the used kernel does not support"].
- **Reading the per-mount options field for `subset=pid`** — it is in the super options (measured).
- **Deduping filesystems by mount point** — production has the data dir as its own bind mount (measured).
- **`uptime` from `sysinfo(2)`** — rounds up to the next second (measured 26091 vs 26090.33).
- **Leading `/` in `fs.FS` paths** — `fs.ReadFile(os.DirFS("/"), "/proc/stat")` fails with `invalid argument` (measured). Strip it once when converting an absolute config path.
- **Absolute symlinks in fixture trees read through `os.DirFS`** — they escape to the real `/sys` of the machine running the test and pass for the wrong reason. Open fixtures with `os.OpenRoot(dir)` → `.FS()`: an escaping link fails with `path escapes from parent` (measured).
- **Copying `/sys` into testdata** — carries MACs/serials that `internal/publicrepo` rejects; write fixtures by hand.
- **Using `hardwareSchema`'s element schemas for links/filesystems on `/host`** — `.default(0)` turns a missing number into 0.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CLOCK_BOOTTIME, sysinfo, uname, statfs | raw `syscall.Syscall` + `unsafe` | `golang.org/x/sys/unix` | Correct per-arch structs, `ByteSliceToString`, no gosec `unsafe` findings |
| CPU % arithmetic | new percent code | exported `inventory.cpuPercent` | Already handles iowait-as-idle and the CPU-time denominator |
| Chip classification, twin rule, label/threshold parsing | a second table | exported talos helpers | D-08; one table means the node page and host page agree |
| Concurrent walk suppression | ad-hoc mutex + flag | `golang.org/x/sync/singleflight` | Already a dependency; correct under concurrent polls |
| JSON from bash | `printf '{"latest":"%s"}'` | `python3 -c 'import json,sys; …' "$@"` | Tag names come from GitHub; string interpolation into JSON is an injection/escaping bug; python3 is already a hard prerequisite of the script (line 92) |
| Temperature severity, meters, fan rows | copies on the host page | move `Sensors`, fan row, `temperatureLimits`, `sensorKey`, `KIND_LABEL` out of `NodeHardware.tsx` into one shared module | D-13 + UI-SPEC "moved, not copied" |

**Key insight:** every value on this page has a shell command an operator can run beside it (`uname`, `free -b`, `df -B1`, `du -s -B1`, `cat /proc/uptime`, `cat /sys/...`). Use the formulas those commands use, and the success criteria "stimmen mit der Shell überein" become byte-exact checks instead of judgement calls.

## Common Pitfalls

### Pitfall 1: Two bars for one filesystem
**What goes wrong:** `/` and the data dir listed separately with identical numbers. **Why:** `StateDirectory` + `ProtectSystem=strict` bind-mounts the data dir over itself (measured). **Avoid:** compare `major:minor`; label the merged row with the entry whose mountinfo ROOT is `/`. **Warning sign:** a fixture test that only has one `/` line in its mountinfo — the production fixture must contain the bind-mount line verbatim in shape.

### Pitfall 2: Hardening never detected (or always detected)
**What goes wrong:** reads options from the wrong field or the lower of two stacked `/proc` mounts. **Avoid:** Pattern 2; unit tests with both the single-entry (systemd) and stacked (unshare) shapes; plus a real-kernel test (Validation: `TestRealKernelProcSubset`).

### Pitfall 3: `health.Field` or `omitzero` erases real zeros
`Value T \`json:"value,omitzero"\`` [VERIFIED: internal/health/health.go:104]. A readable CPU at 0.0 % or a machine with 0 B swap loses its value key. **Avoid:** Pattern 1.

### Pitfall 4: zod defaults re-create the forbidden 0
`hardwareSchema` uses `z.number().default(0)` on every numeric field [VERIFIED: web/src/api.ts:1227-1312]. **Avoid:** share only the temperature and fan element schemas (extract them to named consts used by both); host link/filesystem/memory/cpu schemas are readability unions with no defaults on values.

### Pitfall 5: Rates that are never available
**Why:** boot-time equality check with a jittery derived boot time; or two tabs 100 ms apart replacing the baseline each time. **Avoid:** Pattern 5.

### Pitfall 6: The file-access guard
`TestNoDirectFileAccessOutsideFsstore` fails on `os.ReadFile`, `os.Open`, `os.ReadDir`, `os.Readlink`, `os.OpenFile`, `ioutil.*` in any non-exempt production file [VERIFIED: internal/store/fsstore/permissions_test.go:246-251]. `os.DirFS`, `os.Stat`, `os.OpenRoot`, `fs.ReadFile`, `fs.WalkDir` are not on the list. Everything in `internal/host` goes through `fs.FS`; tests are exempt. Do not add an exemption.

### Pitfall 7: Route guards that fail on a new route
- `TestEveryRouteThatReachesUpstreamHasABudgetRow` builds `routeTable(httpapi.Deps{})` and requires every route to have a budget row **or** be named in `noUpstream` [VERIFIED: cmd/holzkube-managerd/budget_test.go:1740-1822]. Add `"GET /api/v1/host"` to `noUpstream` with a reason (it reaches no node and no upstream). `HostRoutes(httpapi.Deps{})` must not dereference a nil collector at construction.
- `TestEveryProblemCodeIsInTheContract` / `…IsEmitted`: any new problem code (e.g. `upstream.host-unavailable` for a nil collector) must appear in `docs/api-contract.md` [VERIFIED: internal/httpapi/contract_codes_test.go:28-62]. Simplest: construct the collector unconditionally in `main` so no new code is needed; if a nil check stays, document the code.
- `TestTheReadmeDocumentsEveryOption` requires every config option in `docs/guide.md`'s table [VERIFIED: internal/config/readme_test.go:26-31]. A new `--update-status-file` / `HOLZKUBE_MANAGER_UPDATE_STATUS_FILE` row goes into the guide.
- Deps literal: `httpapi.Deps` is copied by value into every `…Routes(deps)`; a field assigned after the literal is nil in every handler [VERIFIED: cmd/holzkube-managerd/main.go:554-560 comment]. Put `Host: hostCollector` **inside** the literal.

### Pitfall 8: darwin build and macOS CI
The daemon is built for darwin/arm64 (.goreleaser.yaml:29-39) and CI runs `go test ./... -count=1 -race` on `macos-latest` (.github/workflows/ci.yml:131-162). Any `unix.Sysinfo`/`CLOCK_BOOTTIME` reference outside a `linux` file breaks both. A test that execs bash must also cope with macOS bash 3.2 and BSD `mktemp`/`install`.

### Pitfall 9: The update script's EXIT trap and exit codes
Existing: `set -euo pipefail` (line 30), `fail()` exits 1 from ~15 places, and `trap 'rm -rf "$TMP"' EXIT` at line 211 — a second `trap … EXIT` **replaces** the first. **Avoid:** one `on_exit` function installed once after the rollback block (line 88), doing both cleanup and `record_status || true`, with `set +e` inside it; it must not call `exit`, so the script's original status is preserved. Test asserts exit codes are unchanged with and without a writable status dir. `--rollback`, `--help` and the non-root refusal write nothing.

### Pitfall 10: The self-replacing script under test
After a healthy update the script installs the archive's copy over `readlink -f "$0"` (lines 281-286). A test running the repo file in place would overwrite `deploy/holzkube-manager-update.sh`. **Avoid:** copy the script into `t.TempDir()` and run the copy.

### Pitfall 11: `installed` unknown on early failure
`LOCAL_VERSION` is computed at line 194, after the network calls; a failure at line 152 ("Release-Liste nicht lesbar") would record `failed` without `installed`. **Avoid:** compute `LOCAL_VERSION` before the first network call; `latest` may legitimately be unknown for `failed` → write `null`, and the daemon's parser accepts `latest: null` **only** for `outcome: failed`.

### Pitfall 12: Container differences
In Docker, `/sys/firmware` is masked [ASSUMED: Docker's default masked paths] → Model Not readable (read-failed); the image is `FROM scratch` with no `/etc/os-release` [VERIFIED: Dockerfile] → OS Not readable; the container's `eth0` is a veth with no `device` link → "No physical network interface found" plus a virtual-interface disclosure. All correct under D-17's banner; component test should cover the container shape so nobody "fixes" it into zeros.

### Pitfall 13: gosec G115
`Statfs_t` fields are mixed `int64`/`uint64`; `sysinfo` loads are `uint64`/`uint32` per arch. Conversions trip gosec G115 in production code (no test exemption). Use explicit bounds or line-scoped `//nolint:gosec // <reason>` as `internal/history/persist.go:260` does.

## Code Examples

### Load from sysinfo, rendered exactly like `/proc/loadavg`
```go
// Source: kernel fs/proc/loadavg.c formula (LOAD_INT/LOAD_FRAC with FIXED_1/200 rounding);
// sysinfo loads are avenrun << (SI_LOAD_SHIFT(16) - FSHIFT(11)). Verified on the Pi:
// /proc/loadavg "7.29 4.90 2.98" == this function's output for the same instant.
func loadFromSysinfo(l uint64) float64 {
    x := l >> (16 - 11)       // back to the kernel's 11-bit fixed point
    x += 2048 / 200           // FIXED_1/200: the kernel's rounding step
    whole := x >> 11
    frac := ((x & 2047) * 100) >> 11
    return float64(whole) + float64(frac)/100
}
```
A naive `round(float64(l)/65536, 2)` can differ from `/proc/loadavg` by 0.01 [ASSUMED: arithmetic reasoning; the one live sample matched both ways].

### Boot uptime and uname (linux)
```go
//go:build linux
// Source: golang.org/x/sys/unix
func (linuxSys) BootTime() (time.Duration, error) {
    var ts unix.Timespec
    if err := unix.ClockGettime(unix.CLOCK_BOOTTIME, &ts); err != nil {
        return 0, err
    }
    return time.Duration(ts.Nano()), nil
}

func (linuxSys) Uname() (Uname, error) {
    var u unix.Utsname
    if err := unix.Uname(&u); err != nil {
        return Uname{}, err
    }
    return Uname{
        Nodename: unix.ByteSliceToString(u.Nodename[:]),
        Release:  unix.ByteSliceToString(u.Release[:]),
        Machine:  unix.ByteSliceToString(u.Machine[:]),
    }, nil
}
```

### Model with trailing NUL; cores from a range list
```go
model := strings.TrimRight(string(raw), "\x00\n ") // device-tree strings end in NUL (measured)

// "0-3" -> 4; "0-3,5,7-8" -> 7
func countCPUList(s string) (int, error) { /* split on ',', each "a" or "a-b", b>=a */ }
```

### os-release per os-release(5)
Try `etc/os-release`, then `usr/lib/os-release`; `KEY=value` lines, value optionally in `"…"` or `'…'` with `\"`, `\\`, `\$`, `` \` `` escapes; `PRETTY_NAME` falls back to `NAME` + `VERSION`, then "Linux". [CITED: os-release(5)]

### Update script: record the outcome (sketch)
```bash
STATUS_DIR=${HOLZKUBE_MANAGER_UPDATE_STATUS_DIR:-/var/lib/holzkube-manager-update}
OUTCOME=""        # set at each decisive point: current | available | updated | rolled-back
LATEST=""         # REMOTE_VERSION once known

record_status() {
  local rc=$1 outcome=$OUTCOME
  [[ -z $outcome && $rc -ne 0 ]] && outcome=failed
  [[ -n $outcome ]] || return 0
  if [[ ! -d $STATUS_DIR ]]; then
    [[ $EUID -eq 0 ]] || return 0
    install -d -o root -g root -m 0755 "$STATUS_DIR" || return 0
  fi
  [[ -w $STATUS_DIR ]] || return 0                      # --check without root: nothing
  local tmp; tmp=$(mktemp "$STATUS_DIR/.status.XXXXXX") || return 0
  python3 - "$outcome" "${LOCAL_VERSION:-}" "$LATEST" > "$tmp" <<'PY' || { rm -f "$tmp"; return 0; }
import json, sys, datetime
outcome, installed, latest = sys.argv[1:4]
json.dump({"checked_at": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
           "installed": installed or None, "latest": latest or None, "outcome": outcome}, sys.stdout)
PY
  chmod 0644 "$tmp" && mv -f "$tmp" "$STATUS_DIR/status.json" || rm -f "$tmp"
}

on_exit() {
  local rc=$?
  set +e
  [[ -n ${TMP:-} ]] && rm -rf "$TMP"
  record_status "$rc" >/dev/null 2>&1
}
trap on_exit EXIT     # once, after the --rollback block; replaces the later `trap 'rm -rf "$TMP"' EXIT`
```
Outcome assignment points: `--check` → `current` / `available` (line 199-201); "Bereits aktuell" → `current` (204-206); healthy restart → `updated` (275); unhealthy + previous restored → `rolled-back` (297-300); unhealthy without previous → leave unset (exit 1 → `failed`). Make `BIN` and `CONF` overridable the same way (`BIN=${HOLZKUBE_MANAGER_BIN:-/usr/local/bin/holzkube-managerd}`, `CONF=${HOLZKUBE_MANAGER_UPDATE_CONF:-/etc/holzkube-manager/update.conf}`) so the test never reads the host's real `/etc/holzkube-manager/update.conf` or binary. Since D-14 names the directory root-owned 0755, also skip writing if `STATUS_DIR` exists and is not owned by root when running as root (`[[ $(stat -c %u "$STATUS_DIR") == 0 ]]`; BSD `stat -f %u` on macOS — or do the ownership check in the python snippet with `os.stat`).

### Daemon-side status parser (strict)
- Read through the host `fs.FS` (path from flag/env, leading `/` stripped); `fs.Stat` first: absent → `update.not-recorded`; `Size() > 4096` → not readable ("larger than 4 KiB"); read via `io.LimitReader(f, 4097)`.
- `json.Decoder` with `DisallowUnknownFields` **off** (a later script may add fields) but every required field validated: `checked_at` parses as RFC 3339; `outcome` ∈ the five; `installed` non-empty string; `latest` non-empty string except `failed` where null is allowed; versions match `^v?[0-9A-Za-z.+-]{1,64}$`.
- Anything else → not readable with the parse error in the message. Never a synthesized time or version.
- In a container: absent file → `update.not-recorded` with the container sentence (UI-SPEC: "In a container without the update timer the server's sentence says so instead").

## Wire Shape (recommended)

```json
{
  "observed_at": "2026-09-28T10:00:03Z",
  "container": false,
  "device": {
    "hostname":  {"readable": true, "value": "example-host"},
    "model":     {"readable": true, "value": "Raspberry Pi 5 Model B Rev 1.0"},
    "arch":      {"readable": true, "value": {"goarch": "arm64", "machine": "aarch64"}},
    "cores":     {"readable": true, "value": 4},
    "os":        {"readable": true, "value": "Debian GNU/Linux 13 (trixie)"},
    "kernel":    {"readable": true, "value": "6.18.50+rpt-rpi-2712"},
    "uptime_seconds": {"readable": true, "value": 26090}
  },
  "service": {
    "version": "v0.1.0",
    "started_at": "2026-09-28T08:00:00Z",
    "uptime_seconds": 7203,
    "data_dir": {"path": "/var/lib/holzkube-manager",
                 "size": {"readable": true, "value": {"bytes": 432013312, "measured_at": "2026-09-28T09:59:40Z"}}},
    "update": {"readable": false, "reason": {"code": "update.not-recorded", "message": "The update script installed on this machine does not record its checks. Versions from this release on do; the next update brings it."}}
  },
  "live": {
    "rates_over_seconds": null,
    "cpu": {"cores": 4,
            "usage": {"readable": false, "reason": {"code": "hardening.proc-subset", "message": "…"}},
            "per_core": {"readable": false, "reason": {"code": "hardening.proc-subset", "message": "…"}},
            "load": {"readable": true, "value": {"load1": 0.52, "load5": 0.41, "load15": 0.33, "source": "sysinfo"}}},
    "memory": {"readable": false, "reason": {"code": "hardening.proc-subset", "message": "…"}},
    "filesystems": [{"mount": "/", "device": "/dev/mmcblk0p2", "fstype": "ext4", "roles": ["root", "data directory"],
                     "usage": {"readable": true, "value": {"size_bytes": 125260451840, "used_bytes": 28882735104, "available_bytes": 91212472320}}}],
    "network": {"physical": [{"name": "eth0", "up": true, "speed_mbit": 1000, "rx_bytes_per_sec": null, "tx_bytes_per_sec": null}],
                "virtual":  [{"name": "lo", "up": false, "speed_mbit": 0, "rx_bytes_per_sec": null, "tx_bytes_per_sec": null}],
                "rates": {"readable": false, "reason": {"code": "rate.no-baseline", "message": "Waiting for a second reading"}}},
    "temperatures": [{"chip": "cpu_thermal", "kind": "cpu", "label": "temp1", "celsius": 64.5, "high_c": null, "critical_c": null}],
    "fans": [],
    "sensors_notice": "No fan reported. Nothing under /sys/class/hwmon on this machine has a fan speed input — either there is no fan, or its controller has no driver."
  }
}
```
Values here are documentation values. `rx/tx` null means "no rate this round", never 0; every list is present and never `null` (build with `make`, D-13). The `hardening` notice in the UI is derived from all `reason.code == "hardening.proc-subset"` in the response, as UI-SPEC requires.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `hidepid=` as a global procfs mount option | per-mount `hidepid=` + `subset=pid` on a new procfs instance | Linux 5.8 | systemd can give one unit a reduced `/proc`; detection must read the unit's own mountinfo |
| `used = total − free − buffers − cache` (procps < 4) | `used = total − MemAvailable` | procps-ng 4.0 | This Pi's `free` (4.0.4) matches `inventory.memoryView` exactly |
| `fs.FS` without symlink reads | `io/fs.ReadLinkFS` implemented by `os.DirFS` | Go 1.25 | Twin/`device` checks can use `ReadLink`/`Lstat` through the same FS |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Docker masks `/sys/firmware` by default, so Model is Not readable in a container | Pitfall 12 | Low — only the copy of one row differs; test the container shape with the value both present and absent |
| A2 | Unprivileged `unshare --user --map-root-user` works on GitHub's ubuntu-latest runners | Validation | Medium — if AppArmor restricts user namespaces there, the real-kernel and root-path script tests skip in CI (visibly) and run only on the Pi |
| A3 | A naive `round(loads/65536, 2)` can differ from `/proc/loadavg` by 0.01 | Code Examples | Low — the kernel-formula function is exact either way |
| A4 | `golang.org/x/sys/unix` v0.47.0 exposes `ClockGettime`, `CLOCK_BOOTTIME`, `Sysinfo`, `Statfs`, `Uname`, `ByteSliceToString` on linux/arm64 and linux/amd64 | Standard Stack | Low — compile-time failure, immediately visible |
| A5 | DMI placeholder strings ("To Be Filled By O.E.M.", "System Product Name", "Default string") are common on amd64 desktop boards | Open Questions | Low — cosmetic |
| A6 | GitHub-hosted runners' bash/python3 are available on macOS for the script test | Validation | Low — test skips visibly without them |

## Open Questions (decided without asking, recorded)

1. **Filesystem percent: `used/size` (UI-SPEC) or `used/(used+available)` (`df` Use%)?**
   - Known: `df` shows 25 %, `used/size` 23 % on this Pi (measured). SC2 demands the shell's value.
   - **Decision:** server sends `size_bytes`, `used_bytes`, `available_bytes`; the UI computes the meter percent as `used/(used+available)` so the figure equals `df`'s Use%. Detail line stays "{used} of {size} · {free} free". This is a deliberate one-formula deviation from UI-SPEC's `formatPercent(used/size)`; the planner notes it in the UI task.
2. **Share `inventory.HardwareLink`/`HardwareFilesystem` Go types literally?**
   - **Decision:** No — D-02 is binding and those types carry non-nullable numbers. Share `HardwareTemperature` and `HardwareFan` literally (Go types and zod element schemas); host link/filesystem types keep every JSON key of their inventory counterparts plus nullable rates / readability wrapper, held by a reflection test that asserts the key superset.
3. **Where does the process start time come from?**
   - **Decision:** `started := time.Now()` at the top of `run()` in `cmd/holzkube-managerd/main.go`, passed to the collector. `/proc/self/stat` would also survive `subset=pid` but adds a jiffies-to-time conversion for no gain.
4. **Thermal-zone trip points as `critical_c`?**
   - **Decision:** No. D-08 limits reads to `temp*_input`/`fan*_input` (plus the labels and `_max`/`_crit` the talos path already reads). The Pi's zone says critical 110 °C while the firmware throttles far lower; filling `critical_c` from it would move the danger line to 110 and diverge from the node page's rendering of the same chip.
5. **Where does the script test live?**
   - **Decision:** `internal/host/updatescript_test.go`: it runs the script copy with stubs and then parses the written file with the daemon's own `ReadUpdateStatus` — one test covers writer and reader agreeing.
6. **DMI placeholder names on amd64** — show as read (A5); no filtering in this phase.

## Environment Availability

| Dependency | Required By | Available (on the Pi) | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain go1.26.7 | build/tests | ✓ via `GOTOOLCHAIN=go1.26.7` (downloads the pinned toolchain; installed Go is 1.27.1 at `~/.local/go`) | 1.26.7 | — |
| Node + npm | web tests, layout audit | ✓ | node v22.23.3, npm 10.9.9 | — |
| Playwright Chromium | `test:layout` | ✓ `~/.cache/ms-playwright/chromium-1243` | — | — |
| `./bin/task`, `./bin/golangci-lint` | gates | ✓ | golangci pinned v2.13.1 | — |
| bash, python3 | script test | ✓ | — | test skips visibly without them |
| `unshare` (util-linux), unprivileged userns | real-kernel `subset=pid` test, root-path script test | ✓ measured working | — | skip with a stated reason |
| shellcheck | optional script lint | ✗ | — | `bash -n` syntax check in the test |
| `go test -race` | race detection | ✗ on the Pi (TSan vs 47-bit VA) | — | CI (ubuntu, macOS) |

**Missing with no fallback:** none.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go `testing` (go1.26.7); vitest (jsdom + browser projects); Playwright layout audit |
| Config file | `Taskfile.yml`, `web/vitest.config.*`, `web/scripts/layout-audit.mjs` |
| Quick run command | `GOTOOLCHAIN=go1.26.7 go test ./internal/host/... -count=1` |
| Full suite command | `GOTOOLCHAIN=go1.26.7 go test ./internal/... ./cmd/... -count=1` · `npm --prefix web run test` · `./bin/task test:layout` · finally `./bin/task ci` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| HMON-06 | procsubset-pid fixture: cpu usage, per-core, memory are `readable:false`, code `hardening.proc-subset`, **no `value` key**; load readable from fake sysinfo | unit | `GOTOOLCHAIN=go1.26.7 go test ./internal/host -run TestHardeningHidesCPUAndMemory -count=1` | ❌ Wave 0 |
| HMON-06 | same answer marshalled: no numeric field anywhere under `cpu.usage`, `cpu.per_core`, `memory` (walk the JSON) | unit | `… -run TestHiddenValuesCarryNoNumber` | ❌ |
| HMON-06 | ENOENT without `subset=pid` → `read-failed`; subset=pid but EACCES → `read-failed` | unit | `… -run TestClassifyNeedsTheMountAsProof` | ❌ |
| HMON-06 | mountinfo: single systemd shape, stacked unshare shape, super-options field, `\040` unescape | unit | `… -run TestMountinfo` | ❌ |
| HMON-06 | real kernel: re-exec under `unshare -rmpf` + `mount -t proc -o subset=pid`, collector reports `hardening.proc-subset` for cpu/memory and readable load | integration (linux, skips visibly if userns unavailable) | `… -run TestRealKernelProcSubset` | ❌ |
| HMON-06 (web) | `/host` rendered from the procsubset shape shows "Not readable" + short reason, hardening notice lists "CPU usage, memory and swap" + `ProcSubset=all`, no "0%"/"0 B"/"0.0%" text in the CPU/Memory cards, load line visible | component | `npm --prefix web run test -- src/routes/host.test.tsx` | ❌ |
| HOST-01 | pi5 fixture + fake Sys: model NUL trimmed, os-release PRETTY_NAME, cores 4 from `0-3`, arch `arm64 (aarch64)`; amd64 fixture: DMI "vendor product" | unit | `… -run TestIdentity` | ❌ |
| HOST-01 | live (linux): nodename == `unix.Uname`, boot uptime within 1 s of `/proc/uptime` when readable | integration | `… -run TestLiveHostMatchesTheKernel` | ❌ |
| HOST-02 | data dir size equals `du -s -B1`-rule on a temp tree (hard link counted once, dirs counted); cached 60 s (fake clock); singleflight | unit | `… -run TestDataDirSize` | ❌ |
| HMON-02 | production-shape mountinfo (bind-mounted data dir, same 179:2) → one row, roles root+data directory, mount `/`; different devices → two rows; prefix boundary `/var/lib/holzkube` vs `/var/lib/holzkube-manager` | unit | `… -run TestFilesystemsDedupeByDevice` | ❌ |
| HMON-02 | statfs arithmetic equals df columns for the measured numbers | unit | `… -run TestStatfsMatchesDf` | ❌ |
| HMON-01 | meminfo → used/cache/available/swap equal `free -b` for the measured sample; /proc/stat parse; load kernel formula | unit | `… -run 'TestMeminfo|TestProcStat|TestLoadFromSysinfo'` | ❌ |
| HMON-01/04 | first Read → rates `rate.no-baseline`, rx/tx null; second Read after ≥0.5 s → numbers; second Read after 0.1 s keeps the old baseline; counter backwards → null | unit | `… -run TestRates` | ❌ |
| HMON-03 | pi5 fixture: temps cpu_thermal(cpu)+rp1_adc(other), no rpi_volt, no in*_input, fans `[]`, notice sentence; amd64: k10temp/nct6798/nvme with fans; zone twin not doubled | unit | `… -run TestSensors` | ❌ |
| HMON-03 | talos path unchanged after exporting helpers | unit | `GOTOOLCHAIN=go1.26.7 go test ./internal/talos -count=1` | ✅ |
| HMON-04 | physical (device exists) vs virtual split; `speed` EINVAL → 0/no speed, not an error | unit | `… -run TestNetwork` | ❌ |
| HOST-03 | status: absent → not-recorded; oversized, bad JSON, bad outcome, future-less RFC3339 errors → not-readable; valid → values | unit | `… -run TestUpdateStatus` | ❌ |
| HOST-03 | script (non-root): `--check` current/available/failed write a file the daemon parses; unwritable dir → no file, same exit code; `--rollback`/`--help` write nothing | integration (bash) | `… -run TestUpdateScript` | ❌ |
| HOST-03 | script root paths under `unshare -r` with stubs (`curl`, `systemctl`, `sleep`, fake BIN, tarball+checksums): updated, rolled-back, failed-after-download | integration (skips visibly without userns) | `… -run TestUpdateScriptAsRoot` | ❌ |
| D-17 | `.dockerenv` / `run/.containerenv` / `container=` in `proc/1/environ` → `container:true` | unit | `… -run TestContainer` | ❌ |
| D-13 | no `null` anywhere in the answer over an empty FS; host added to `TestNoReadModelSendsNullForAList` | unit | `… -run TestNoNulls` and `go test ./internal/httpapi -run TestNoReadModelSendsNullForAList` | ❌ / ✅ (extend) |
| D-11 | route: 401 without session, 200 for reader, not audited; listed in `noUpstream` | integration | `go test ./internal/httpapi -run TestHostAPI` · `go test ./cmd/holzkube-managerd -run TestEveryRouteThatReachesUpstreamHasABudgetRow` | ❌ / ✅ (extend) |
| fixture | `/api/v1/host` in `web/fixtures/demo.json` parses with `hostSchema` | unit | `npm --prefix web run test -- src/fixtures.test.ts` | ✅ (extend) |
| layout | `/host` in `ROUTES`, 390 px + 1280 px, tap targets | e2e | `./bin/task test:layout` | ✅ (extend) |
| shared UI | NodeHardware tests stay green after moving Sensors/temperatureLimits | component | `npm --prefix web run test -- src/components/NodeHardware.test.tsx` | ✅ |

### Fault injections (each guard must be seen red — CLAUDE.md)

| Guard | Fault to reinstate | Expected red |
|-------|-------------------|--------------|
| TestHardeningHidesCPUAndMemory / TestHiddenValuesCarryNoNumber | make the hidden branch return `Read(0.0)` (or give `Reading.Value` a non-pointer `omitzero`) | `value` key present / `readable:true` |
| TestClassifyNeedsTheMountAsProof | drop the `subsetPid &&` condition | ENOENT-without-subset case reports hardening |
| TestMountinfo (stacked) | take the first `/proc` entry instead of the topmost | stacked shape reports no hardening |
| TestFilesystemsDedupeByDevice | dedupe by mount point | two rows for the production shape |
| TestRates | advance baseline on every call | 0.1-s second call loses its rates |
| TestUpdateScript | remove `|| true`/`set +e` from the trap, or install a second `trap … EXIT` | exit code changes or no file written |
| host.test.tsx | render `formatPercent(0)` for an unreadable CPU | "0.0%" found |
| budget/noUpstream | omit the `noUpstream` entry | route listed as missing |

### Sampling Rate
- **Per task commit:** `GOTOOLCHAIN=go1.26.7 go test ./internal/host/... -count=1` (+ the touched web test file)
- **Per wave merge:** `GOTOOLCHAIN=go1.26.7 go test ./internal/... ./cmd/... -count=1` and `npm --prefix web run test`
- **Phase gate:** `./bin/task ci` green on the Pi, then a manual check on the Pi with a **dev daemon** (HANDOVER §3.1: temp data dir, `--insecure-http --listen 127.0.0.1:18443`): `curl -b jar …/api/v1/host` compared against `uname -n -r -m`, `cat /proc/uptime`, `free -b`, `df -B1 /`, `cat /sys/class/thermal/thermal_zone0/temp`, two reads of `/sys/class/net/eth0/statistics/rx_bytes`; then the same daemon started inside `unshare --user --map-root-user --mount --pid --fork sh -c 'mount -t proc -o subset=pid,hidepid=invisible proc /proc && exec ./bin/holzkube-managerd …'` to see the hardening answer on the real kernel. The production service is not restarted or replaced; the operator's `/host` shows the production state after the next release through the existing update timer.

### Wave 0 Gaps
- [ ] `internal/host/testdata/{pi5,amd64,procsubset-pid}/` — hand-written trees (documentation values only), opened with `os.OpenRoot(...).FS()` in tests
- [ ] `internal/host/fake_sys_test.go` — fake `Sys` (uname `example-host`, fixed boot time, sysinfo loads, statfs table)
- [ ] `internal/host/*_test.go` per the map above
- [ ] `internal/httpapi/hostapi_test.go`
- [ ] `web/src/routes/host.test.tsx`
- [ ] extend `web/src/fixtures.test.ts`, `web/fixtures/demo.json`, `web/scripts/layout-audit.mjs` `ROUTES`, `cmd/holzkube-managerd/budget_test.go` `noUpstream`, `internal/httpapi/readmodel_test.go`

## Security Domain

`security_enforcement: true`, ASVS level 1.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no (reuses sessions) | existing session middleware |
| V3 Session Management | no | existing |
| V4 Access Control | yes | `RequiresSession: true`, `MinRole: model.RoleReader`, `WallLink: false` (wall is Phase 12) |
| V5 Input Validation | yes | strict status-file parser (size cap 4 KiB, enum, RFC 3339, version regex); bounded reads of `/proc`/`/sys` (cap each read, e.g. 64 KiB; mountinfo 1 MiB) |
| V6 Cryptography | no | — |
| V7 Error/Logging | yes | reasons carry the OS error text for local paths only; no environment dump; `/proc/1/environ` only checked for a `container=` key, never returned |
| V12 Files and Resources | yes | fixed paths only; status path from flag/env (operator-controlled); root-owned 0755 status dir, 0644 file, atomic `mktemp`+`mv`; daemon only reads |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Root writing into a user-writable directory (symlink swap) | Tampering / EoP | D-14 location outside the data dir; script refuses to write if the dir is not root-owned |
| JSON injection via release tag names | Tampering | python3 `json.dump` with argv, never string interpolation |
| Oversized/malicious status file → memory/CPU | DoS | `fs.Stat` size check + `io.LimitReader` |
| Polling-induced disk load (data-dir walk on SD) | DoS | 60-s cache, singleflight, walk deadline |
| Host facts to low-privilege readers (hostname, kernel, data dir path) | Information disclosure | Accepted: reader role already sees the version and cluster topology; no secrets, env, or mount list beyond the two rows are served |
| Status write breaking an update | Availability | EXIT trap with `set +e`, `|| true`, exit code preserved; tested |

## Sources

### Primary (HIGH confidence)
- Measurements on this Pi (aarch64, Debian 13, kernel 6.18.50+rpt-rpi-2712): `/proc`, `/sys`, `uname`, `statfs`, `sysinfo`, `clock_gettime`, `free -b`, `df -B1`, unshare+`subset=pid`, production unit via `systemctl show`, production mountinfo via `/proc/<pid>/mountinfo` (all read-only)
- `man systemd.exec` (systemd 257 on the Pi): ProcSubset=, ProtectProc=, ProtectHostname=
- `man 5 proc`: `subset=pid` (since Linux 5.8)
- `go doc io/fs.ReadLinkFS`, `go doc os.DirFS` with go1.26.7
- Repository: internal/inventory/hardware.go, internal/talos/sensors.go, internal/health/health.go, internal/store/fsstore/permissions_test.go, internal/talos/seam_test.go, internal/depguard_test.go, cmd/holzkube-managerd/{main.go,budget_test.go}, internal/httpapi/{router.go,readmodel_test.go,contract_codes_test.go}, internal/config/{config.go,readme_test.go}, deploy/holzkube-manager-update.sh, web/src/{api.ts,components/NodeHardware.tsx,lib/format.ts,components/HealthField.tsx,App.tsx,components/Sidebar.tsx,fixtures.test.ts}, web/scripts/layout-audit.mjs, Dockerfile, compose.yaml, .goreleaser.yaml, .github/workflows/ci.yml, .golangci.yml

### Secondary (MEDIUM confidence)
- os-release(5) quoting rules [CITED: freedesktop os-release man page — from knowledge, not fetched this session]

### Tertiary (LOW confidence)
- Docker masked paths, GitHub runner userns policy, DMI placeholders (Assumptions A1, A2, A5)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new modules; x/sys and x/sync already checksummed.
- Architecture: HIGH — the two structural surprises (bind-mounted data dir, super-options/stacked `/proc`) were measured on the production namespace and a real `subset=pid` mount.
- Pitfalls: HIGH — each guard cited was read in the repository this session; the script pitfalls come from reading the script.

**Research date:** 2026-09-28
**Valid until:** 2026-10-28 (stable kernel/systemd interfaces; re-check if the operator's unit changes)
