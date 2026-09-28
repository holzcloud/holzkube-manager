---
phase: 11-host-seite
plan: 06
subsystem: api, ui
tags: [go, sysfs, net-class, fs.FS, symlinks, rate-memo, react, zod, details-disclosure]
status: complete

requires:
  - phase: 11-03
    provides: "collector rate memo (counters, minRateWindow/maxRateWindow, advance rule), Reading/Hidden, reasonFor, readBounded"
  - phase: 11-05
    provides: "readTrimmed, the symlinked pi5 sysfs fixture, the Live grid with Sensors"
provides:
  - "internal/host/network.go: Link (inventory.HardwareLink keys, nullable numbers), Network{Physical, Virtual}, readLinks over fs.FS, network()"
  - "counters.links map[string]talos.LinkIO in the collector memo; link rates over the same usable window as CPU"
  - "Live.Network Reading[Network]; listing failure read-failed naming /sys/class/net; absent class = two empty lists"
  - "testdata/pi5/sys/class/net in the kernel's symlinked layout (no address files)"
  - "api.ts: linkSchema, Link, hostSchema.live.network"
  - "Network card on /host with the virtual-interfaces disclosure; complete-Pi-5-page component test"
  - "README: the Host page in What it does"
affects: [11-07, 12]

actuals:
  tokens: 13917
  tasks: 2
  commits: 2
plan_head_before: fa4465d476c9d745e3e3ab6779a2d5142eb0c1fe
plan_head_after: d3eae877a312c9ed4c0b137b9a2c48ff6c24338d

tech-stack:
  added: []
  patterns:
    - "Physical = fs.Lstat(<if>/device) succeeds; never a name rule"
    - "A rate is a pointer: nil for first read, a new link, or a backwards counter -- never inventory's perSecond 0"
    - "UI branches on null before formatRate, since formatRate(0) prints 0 B/s"

key-files:
  created:
    - internal/host/network.go
    - internal/host/network_test.go
    - internal/host/testdata/pi5/sys/class/net/
    - internal/host/testdata/pi5/sys/devices/platform/axi/1000120000.pcie/
    - internal/host/testdata/pi5/sys/devices/platform/axi/1001100000.mmc/
    - internal/host/testdata/pi5/sys/devices/virtual/net/
  modified:
    - internal/host/collector.go
    - internal/host/collector_test.go
    - internal/host/host.go
    - web/src/api.ts
    - web/src/routes/host.tsx
    - web/src/routes/host.test.tsx
    - web/fixtures/demo.json
    - README.md

key-decisions:
  - "Both rates of a link are null when either counter went backwards (the interface was recreated)"
  - "The pi5 net fixture uses the real symlinked layout instead of empty device/ directories (git does not track an empty directory); wlan0's mmc path has its colons replaced by dashes"
  - "readLinks returns a name-sorted slice rather than a map; the memo keeps map[string]talos.LinkIO as planned"
  - "Above 512 interfaces the listing is cut in name order, so veths (sorting last) are the ones not read"

requirements-completed: [HMON-04]

duration: 16 min
completed: 2026-09-28
---

# Phase 11 Plan 06: Host Network Summary

**/host now has a Network card. Each physical interface (one with a `device` link in sysfs) shows up/down, its speed and its in/out throughput. The server computes throughput from its own counter memo. Loopback, bridges and veths are counted behind one 44 px disclosure. A rate that does not exist yet shows "—", never 0 B/s. With this card the Pi 5 page is complete.**

## Where this ran

On the operator's Raspberry Pi 5 (aarch64), natively. Go 1.26.7 from `~/.local/go`, Node via nvm, no `-race` (the kernel refuses it; that is CI's job). The fixture layout was checked against this Pi's real `/sys/class/net` with read-only `readlink`/`cat`. The production service, `/var/lib/holzkube-manager` and `/usr/local/bin` were not touched.

## Tasks

| Task | Name | Commit |
| ---- | ---- | ------ |
| 1 | Interfaces from sysfs, physical/virtual split, rates from the memo | d8a9d61 |
| 2 | Network card with the virtual-interfaces disclosure; complete Pi 5 page | d3eae87 |

## Verification (exit codes read from the command itself)

- `go test ./internal/host -run 'TestNetwork|TestLinkKeysMatchInventory|TestRates|TestNoNulls|TestEveryReadingIsWellFormed' -v`: exit 0. PASS for TestNetworkPi5, TestNetworkSpeedEINVAL, TestNetworkSpeedValues, TestNetworkRates, TestNetworkUnreadable, TestNetworkAbsent, TestLinkKeysMatchInventory, TestRates, TestNoNulls, TestEveryReadingIsWellFormed
- `go test ./internal/host/... ./internal/httpapi ./cmd/holzkube-managerd ./internal/publicrepo ./internal/store/fsstore -count=1`: exit 0
- `./bin/task lint:go`: exit 0, 0 issues (after renaming a test variable that nilerr flagged)
- `npm --prefix web run test`: 54 files, 534 tests, exit 0. `typecheck`: exit 0. `lint`: exit 0 (the 2 warnings and 1 info are older ones outside this plan)
- `./bin/task test:layout`: exit 0. `/host` ok at 390px (4 controls) and 1280px, with no tap-target finding
- Acceptance greps: `statistics/rx_bytes` 1 and `/device` 2 in network.go; `find internal/host/testdata -name address` prints nothing; `min-h-11` 2, `group-open:rotate-90` 1 and the empty sentence 1 in host.tsx; `"physical"` 1 in demo.json
- `go test ./internal/publicrepo`: exit 0 before the last commit

## Fault injections (each seen red, then reverted)

**Task 1:**
1. A backwards counter answered with 0, as inventory's perSecond does. TestNetworkRates went red at "3 s later": got `lo rx=0 tx=166.66666666666666`, want `rx=null tx=null`.
2. Physical decided by name (`lo`/`veth` prefix) instead of the device link. TestNetworkPi5 went red: got physical `[br-0a1b2c3d4e5f][docker0][eth0][wlan0]`. TestNetworkSpeedEINVAL went red the same way.
3. `speed_mbit` removed from `nullableKeys`. TestNoNulls went red: `.live.network.value.physical[].speed_mbit`.
4. Class entries filtered on `IsDir()`. TestNetworkPi5 went red: got `physical  virtual` (empty), and "physical on the wire = []".

**Task 2** (`src/routes/host.test.tsx`):
1. `formatRate(bytesPerSecond ?? 0)` for a null rate. "never 0 B/s" went red: `expected [ 'in 0 B/s · out 0 B/s', …(5) ] to deeply equal []`. On a first run, before the zero guard was moved ahead of the "—" check, the "in — · out —" assertion went red instead.
2. Summary without `min-h-11`. The disclosure test went red on `toHaveClass("min-h-11")`.
3. Disclosure rendered at 0 virtual interfaces. The zero case went red, rendering a `0 virtual interfaces` summary.

After each revert, `cmp` against the saved original matched and the suite was green again.

## Deviations from Plan

**1. [Rule 3 - Blocking] Fixture uses real symlinks, not empty `device/` directories.** Git does not track empty directories, so an empty `device/` would have disappeared on checkout. The fixture copies the Pi 5's real layout instead: class entries are symlinks into `sys/devices`, and `device` is a symlink to the parent device directory. That also covers the "never filter on IsDir" rule. wlan0's real path contains colons (`mmc1:0001:1`); they are dashes here.

**2. [Rule 2 - CLAUDE.md] README "What it does" gains the Host page.** No plan in this phase touched the README, and CLAUDE.md requires it for a new feature. The screenshot was not added. Adding it re-renders every image, so it is logged in `deferred-items.md` for phase end.

**3. [Minor] Extra tests.** TestNetworkSpeedValues (-1, 0, unparsable, empty). The EINVAL test also fails docker0's speed, to prove the value comes from the error and not from the file. Web tests were added for "up" with no speed and for the missing container badge on a host.

**Total deviations:** 1 blocking fix, 1 CLAUDE.md-driven addition, 1 minor. **Impact:** none on the planned behaviour.

## Known Stubs

None.

## Deferred

- README screenshot of /host: see `.planning/phases/11-host-seite/deferred-items.md`.

## Self-Check: PASSED

- FOUND: internal/host/network.go, internal/host/network_test.go, internal/host/testdata/pi5/sys/class/net/eth0/operstate
- FOUND: commits d8a9d61, d3eae87
