---
phase: 11-host-seite
plan: 07
subsystem: docs, ui, verification
tags: [api-contract, guide, vitest-browser, playwright, layout, proc-subset, unshare, readme]
status: complete

requires:
  - phase: 11-01..11-06
    provides: "internal/host, GET /api/v1/host, the recording update script, the /host page with all its cards"
provides:
  - "docs/api-contract.md: the complete GET /api/v1/host contract"
  - "docs/guide.md: 'The Host page' for the operator"
  - "web/src/routes/host.browser.test.tsx: /host at 390 px in a real browser, amd64 shape and long identifiers"
  - "docs/screenshots/host.png and README 'A look around' entry"
  - "the Pi comparison, the subset=pid run and the gate, recorded here"
affects: [12, 13, 14]

actuals:
  tokens: 7850
  tasks: 3
  commits: 3
plan_head_before: 08710ddd2ecb2f69e3913d106753c7eccd2caf0e
plan_head_after: c2bd7a763cafe906bd0874fd84c8b6213836f06e

tech-stack:
  added: []
  patterns:
    - "Browser layout tests set the viewport as well as the box: media queries read the viewport"
    - "Overflow is measured on text runs (Range.getClientRects) as well as element boxes"

key-files:
  created:
    - web/src/routes/host.browser.test.tsx
    - docs/screenshots/host.png
  modified:
    - docs/api-contract.md
    - docs/guide.md
    - README.md
    - web/scripts/readme-images.mjs
    - .planning/phases/11-host-seite/deferred-items.md

key-decisions:
  - "The docs speak of 'a unit with ProcSubset=pid, as the reference installation runs it', not 'the shipped unit': the repository ships no systemd unit"
  - "host.browser.test.tsx sets the viewport to 390 px; a 390-px box inside the project's 1200-px viewport measured a desktop grid no phone renders"
  - "The overflow check measures text runs too; an element-only check passed a whitespace-nowrap notice"
  - "host.png was rendered by a binary built with version v0.1.0 (the newest release tag, and the fixture's service version), because git describe on this checkout gives v0.0.1-58-g... and the build was dirty"
  - "Only host.png was committed; the other re-rendered pictures were restored"

requirements-completed: [HOST-01, HOST-02, HOST-03, HMON-01, HMON-02, HMON-03, HMON-04, HMON-06]

duration: 28 min
completed: 2026-09-28
---

# Phase 11 Plan 07: Contract, 390-px measurement, and Pi verification Summary

**The /api/v1/host contract is now written out in full, and the guide explains why CPU and memory say "Not readable" and which line changes that. /host is measured at 390 px in a real Chromium, for the amd64 desktop shape and for identifiers longer than a phone is wide. On the Pi, a dev daemon's answer matched the shell on 66 checks. The same binary under a real `subset=pid` /proc said `hardening.proc-subset` and gave load from sysinfo. `./bin/task ci` exited 0.**

## Where this ran

On the operator's Raspberry Pi 5, natively on aarch64 (`uname -m` = aarch64, `file bin/holzkube-managerd` = ARM aarch64). Go 1.26.7 from `~/.local/go`, Node via nvm, Chromium from Playwright. There was no `-race` run: ThreadSanitizer refuses the Pi 5 kernel's 47-bit address space, so the race verdict belongs to CI. Nothing touched `holzkube-manager.service`, `/usr/local/bin` or `/var/lib/holzkube-manager`. `ActiveEnterTimestamp` read `Mon 2026-09-28 18:06:00 CEST` before Task 1 and again after the gate.

## Tasks

| Task | Name | Commit |
| ---- | ---- | ------ |
| 1 | The finished contract and the operator's paragraph | 2655a61 |
| 2 | 390-px browser measurement of the amd64 and long-identifier shapes | 11c105d |
| 3 | On the Pi: page against shell, against subset=pid, full gate (record only) | (this SUMMARY) |
| + | README screenshot of /host (deferred from 11-06) | c2bd7a7 |

## Task 1: contract and guide

- `docs/api-contract.md` has a new subsection, "GET /api/v1/host: read on every call, kept nowhere". It covers:
  - a full JSON example with documentation values only
  - the reading rule
  - a table of all five reason codes and when each occurs
  - a table of every nullable key
  - that lists are never null
  - what each section reads from
  - how subset=pid is detected (the super options of the topmost /proc mount)
  - the filesystem dedupe by `st_dev`
  - polling every 3 s, no audit, no upstream
  - the status-file format (path, owner, mode, fields, five outcomes, strict reader) and `--update-status-file`
  - `502 upstream.host-unavailable`
- `docs/guide.md` has a new section, "The Host page". It covers what the page reads, why CPU, memory and swap are "Not readable" under `ProcSubset=pid` (load stays, from sysinfo), the `ProcSubset=all` drop-in with daemon-reload and restart, that `ProtectProc=invisible` can stay, `ProtectHostname`, and when "Not recorded" goes away.
- Verify: `TestEveryProblemCodeIsInTheContract|TestEveryProblemCodeIsEmitted` PASS. `go test ./internal/config ./internal/publicrepo` exit 0. Acceptance greps: proc-subset 5, read-failed 5, rate.no-baseline 2, update.not-recorded 3, unsupported 7, upstream.host-unavailable 2, rx_bytes_per_sec 5, the status path 2. In the guide: ProcSubset=all 1, ProtectHostname 1.

## Task 2: 390 px in Chromium

`npm --prefix web run test:browser -- src/routes/host.browser.test.tsx --reporter=verbose` exited 0:
- ✓ fits the amd64 desktop shape: notices, many sensors and fans, twelve virtual interfaces
- ✓ wraps long identifiers instead of widening the page

**Fault injections.** 1 and 2 were seen red; 3 and 4 were tried and stayed green, for the reasons given. After each one `host.tsx` was restored from a saved copy, and `git diff` was empty afterwards.
1. **`break-words` removed from the Device card's `dd`** (`Row`). The long-identifiers case went red, naming `<span class="font-mono"> "examplehost…exampleh" ends at 504.9 > 390.5` (the hostname) and `<span class="font-mono"> "6.12.47-kkk…" ends at 652.3 > 390.5` (the kernel). An earlier hostname with hyphens (`hhh-ooo-sss`) broke at its hyphens and so was not tested by this fault. It is now a 63-character label with no hyphen.
2. **`whitespace-nowrap` on the container notice.** The amd64 case went red: `text of <p class="whitespace-nowrap rounded-md …"> "holzkube-manager runs in a container. …"`. On the first attempt this injection stayed **green**. The notice's `<p>` keeps its 390-px box while its text runs past it, and the check measured only element boxes. The check now also measures text runs through a Range, and it was seen red after that change.
3. **`min-w-0` removed from the `dd` (keeping `break-words`): stays green.** This is a measurement, not a hole: at 390 px the grid is `grid-cols-1`, whose track is already `minmax(0, 1fr)`. `min-w-0` only matters for the md two-column grid.
4. **LinkRow without `flex-wrap`, with `whitespace-nowrap`: stays green.** The longest row (`br-0a1b2c3d4e5f · up · 10000 Mbit/s` plus rates) is 369 px wide and ends at 385 px. It still fits. The debug dump showed the Card itself is `overflow: hidden`, which is why clipped content must fail this check rather than be exempted. It does fail: only `auto` and `scroll` are exempt.

The first run of the test failed for a reason of its own. With the project's 1200-px viewport, the cards' `md:` two-column grid applied inside the 390-px box, and the Service card's `dd`s ended at 395 px. That layout never occurs on a phone. The test now calls `page.viewport(390, 900)` and resets it to 1200 afterwards.

## Task 3: on the Pi

**Build:** `./bin/task build` exit 0. `GOOS=linux GOARCH=amd64 go build` exit 0. `GOOS=darwin GOARCH=arm64 go build` exit 0.

**Dev daemon, run 1** (`--insecure-http --listen 127.0.0.1:18443 --data-dir $(mktemp -d)`). Setup returned 201 and login 204. `compare.py` (in the session scratchpad; it prints field names and pass/fail only) exited 0 with **66 checks, all PASS**, over two fetches 3.2 s apart:

| Field | Compared with | Tolerance | Result |
|---|---|---|---|
| device.hostname | `uname -n` | exact | pass ×2 |
| device.kernel | `uname -r` | exact | pass ×2 |
| device.arch.machine / goarch | `uname -m` / arm64 | exact | pass ×2 |
| device.uptime_seconds | `/proc/uptime` | 3 s | pass ×2 |
| device.model | device-tree model, NUL removed | exact | pass ×2 |
| device.os | PRETTY_NAME | exact | pass ×2 |
| device.cores | `/sys/devices/system/cpu/online` count | exact | pass ×2 |
| memory.total_bytes / swap_total_bytes | MemTotal·1024 / SwapTotal·1024 | exact | pass ×2 |
| memory.used / available / swap_used | `/proc/meminfo` | 2 % of total | pass ×2 |
| cpu.load | `/proc/loadavg` | 0.1 | pass ×2 (source `loadavg`, unhardened) |
| cpu.usage | first: `rate.no-baseline`; second: readable, per_core one per core, rates_over_seconds set | exact | pass |
| filesystems row count | `stat -c %d /` vs data dir | exact | pass ×2 (2 rows: /tmp is its own filesystem) |
| filesystem size (each row) | `df -B1` size | exact | pass ×2 |
| filesystem used / available | `df -B1` | 16 MiB | pass ×2 |
| service.data_dir.size | `du -s -B1 $D` | 64 KiB | pass ×2 |
| service.data_dir.path | `$D` | exact | pass ×2 |
| sensors cpu_thermal | `thermal_zone0/temp`/1000 | 3 °C | pass ×2 |
| network eth0 | physical; up = operstate; first rx/tx null, second not null | exact | pass |
| service.update | status file absent, so `update.not-recorded` | exact | pass ×2 |
| container | false | exact | pass ×2 |

**Dev daemon, run 2** (data dir from `mktemp -d` under the user's cache directory, on the root filesystem). This exercised the merged row that /tmp could not: **60 checks, all PASS**, including `filesystems rows = 1`, `filesystem[root+data directory].size_bytes = df -B1 size` and used/available within 16 MiB. As a sanity check, the script was pointed at the wrong data directory against run 1's daemon. It exited 1. The FAIL came at the first-fetch baseline check, because that daemon already had a baseline, so the path/du checks were never reached. This shows only that the script fails loudly. It is not a targeted injection.

**Success criterion 4:** `/var/lib/holzkube-manager-update/status.json` does not exist on the Pi, and the answer says `update.not-recorded`. The recording script is in `deploy/` and this phase does not install it.

**Hardening run:** `unshare --user --map-root-user --mount --pid --fork sh -c 'mount -t proc -o subset=pid,hidepid=invisible proc /proc && exec ./bin/holzkube-managerd … --listen 127.0.0.1:18444 …'`. The daemon **started as uid 0 inside the namespace; there was no refusal**. `harden.py` exited 0, all PASS, over two rounds:
- `live.cpu.usage`, `live.cpu.per_core`, `live.memory`: `readable: false`, code `hardening.proc-subset`, no `value` key (both rounds)
- `live.cpu.load`: readable, `source: sysinfo`, within 0.1 of `/proc/loadavg` read outside (both rounds)
- network still read from sysfs, and round 2 had `rates_over_seconds` and an eth0 rate under subset=pid

It was stopped with SIGTERM to the daemon (PID 1 of its namespace, found as the unshare process's child) and exited within 10 s. No SIGKILL was needed, and unshare exited with it. Both temp data dirs were removed. Nothing listens on 18443 or 18444 any more.

**Gate:** `./bin/task ci` **exit 0**, read from the command itself. Results:
- web lint
- web tests: 55 files, 536 tests, the new browser file included
- build
- golangci-lint 2.13.1: 0 issues
- `go test ./...`: all ok, internal/upgrade 45.9 s, internal/host 0.24 s
- layout audit: `/host` ok at 390 px (4 controls, 7 items) and 1280 px

`test:next` ran under the forced `GOTOOLCHAIN=go1.26.7`, not a newer Go. It had one timeout, `TestANodeSaysHowItBooted` (`talos: Get … timed out`, internal/upgrade), with the full suite loading the Pi. That step cannot fail the chain. Rerun alone, `go test ./internal/upgrade -run TestANodeSaysHowItBooted -count=1` gave ok in 8.75 s, exit 0.

## README screenshot (deferred from 11-06, closed)

`web/scripts/readme-images.mjs` now shoots `/host` from `web/fixtures/demo.json`, which already had the full host shape. The README's "A look around" has a **Host** entry with `docs/screenshots/host.png`. The script re-renders every picture. The other eleven (`docs/brand/*`, the other screenshots, `web/public/*` icons) were restored with `git checkout --`, and only host.png was committed. The rendered image was checked: fixture values only (`manager-01.homelab.example`), sidebar `v0.1.0`. `deferred-items.md` marks the item closed.

## Deviations from Plan

**1. [Rule 1 - Bug in the new test] The test sets the viewport to 390 px, not just the box.** Found during Task 2. Media queries follow the viewport, so the first version measured a desktop grid squeezed into 390 px. Fixed in 11c105d.

**2. [Rule 2 - Guard gap] Text runs are measured as well as element boxes.** Found during Task 2 injection 2. Without this the amd64 case could not go red for a non-wrapping notice. `sr-only` text (the sensors' "critical" label, laid out past a 1-px clipped box) is exempt.

**3. [Rule 1 - Accuracy] The docs don't call it "the shipped unit".** The repository contains no systemd unit; the one with `ProcSubset=pid` is the reference installation's. `internal/host/host.go` and the page's notice still say "the systemd unit this product ships" and "the systemd unit sets". They were not changed here (out of scope). Noted for Phase 12/13.

**4. [Additional work, from the dispatch] README screenshot.** The dispatch asked for it and CLAUDE.md requires it. The render binary was built with `-X main.version=v0.1.0`, because `git describe --tags` on this checkout gives `v0.0.1-58-g…` (not `v0.1.0`) and was `-dirty` at render time; the fixture's service version is `0.1.0`. `./bin/task ci` rebuilt `bin/holzkube-managerd` normally afterwards.

**5. [Additional check] A second comparison run with the data dir on the root filesystem.** On the Pi /tmp is a separate filesystem, so the plan's `mktemp -d` alone never exercises the merged row.

## Known Stubs

None.

## Threat Flags

None. No new surface: docs, one test file, one screenshot, one line in a render script.

## Human check still open (the operator's machine)

After the next release installs itself through the update timer, open `/host` signed in:
- Device and Service show the Pi.
- The slate notice names CPU usage, memory and swap and shows `ProcSubset=all`.
- Load, filesystems, cpu_thermal/rp1_adc, "No fan reported" and eth0/wlan0 show.
- Update check says "Not recorded" until the first run of the recording script, then shows the recorded outcome.

## Self-Check: PASSED

- FOUND: docs/api-contract.md (section present), docs/guide.md ("### The Host page"), web/src/routes/host.browser.test.tsx, docs/screenshots/host.png
- FOUND: commits 2655a61, 11c105d, c2bd7a7
- `go test ./internal/publicrepo -count=1` exit 0 after this SUMMARY was written (see final commit)
