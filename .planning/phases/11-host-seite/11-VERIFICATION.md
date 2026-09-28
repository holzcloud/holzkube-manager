---
phase: 11-host-seite
verified: 2026-09-28T18:43:25Z
status: passed
score: 21/22 must-haves verified
covered_files:
  - .planning/phases/11-host-seite/11-01-PLAN.md
  - .planning/phases/11-host-seite/11-01-SUMMARY.md
  - .planning/phases/11-host-seite/11-02-PLAN.md
  - .planning/phases/11-host-seite/11-02-SUMMARY.md
  - .planning/phases/11-host-seite/11-03-PLAN.md
  - .planning/phases/11-host-seite/11-03-SUMMARY.md
  - .planning/phases/11-host-seite/11-04-PLAN.md
  - .planning/phases/11-host-seite/11-04-SUMMARY.md
  - .planning/phases/11-host-seite/11-05-PLAN.md
  - .planning/phases/11-host-seite/11-05-SUMMARY.md
  - .planning/phases/11-host-seite/11-06-PLAN.md
  - .planning/phases/11-host-seite/11-06-SUMMARY.md
  - .planning/phases/11-host-seite/11-07-PLAN.md
  - .planning/phases/11-host-seite/11-07-SUMMARY.md
  - README.md
  - cmd/holzkube-managerd/budget_test.go
  - cmd/holzkube-managerd/main.go
  - deploy/holzkube-manager-update.sh
  - docs/api-contract.md
  - docs/guide.md
  - go.mod
  - internal/config/config.go
  - internal/config/config_test.go
  - internal/host/collector.go
  - internal/host/collector_test.go
  - internal/host/filesystems.go
  - internal/host/filesystems_test.go
  - internal/host/helpers_test.go
  - internal/host/host.go
  - internal/host/identity.go
  - internal/host/identity_test.go
  - internal/host/live_test.go
  - internal/host/mountinfo.go
  - internal/host/mountinfo_test.go
  - internal/host/network.go
  - internal/host/network_test.go
  - internal/host/proc.go
  - internal/host/proc_test.go
  - internal/host/realkernel_test.go
  - internal/host/sensors.go
  - internal/host/sensors_test.go
  - internal/host/service_test.go
  - internal/host/sys.go
  - internal/host/sys_linux.go
  - internal/host/sys_other.go
  - internal/host/updatestatus/script_test.go
  - internal/host/updatestatus/status.go
  - internal/host/updatestatus/status_test.go
  - internal/httpapi/endtoend_test.go
  - internal/httpapi/handlers/host.go
  - internal/httpapi/hostapi_test.go
  - internal/httpapi/router.go
  - internal/inventory/hardware.go
  - internal/talos/sensors.go
  - internal/talos/sensors_test.go
  - web/fixtures/demo.json
  - web/scripts/layout-audit.mjs
  - web/scripts/readme-images.mjs
  - web/src/App.tsx
  - web/src/api.ts
  - web/src/components/NodeHardware.test.tsx
  - web/src/components/NodeHardware.tsx
  - web/src/components/Sidebar.tsx
  - web/src/components/charts/Sensors.test.tsx
  - web/src/components/charts/Sensors.tsx
  - web/src/fixtures.test.ts
  - web/src/routes/host.browser.test.tsx
  - web/src/routes/host.test.tsx
  - web/src/routes/host.tsx
covered_digest: "v2:sha256:7bb5b1059d7efbfbe055ca49f8d767a24729d7e9441ec3bbb41344248ec234b8"
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "After the next release has installed itself through the update timer, open /host on the production service, signed in"
    expected: "Device and Service show the Pi; the slate notice names CPU usage, memory and swap and shows ProcSubset=all; load, the merged 'root · data directory' filesystem, cpu_thermal/rp1_adc, 'No fan reported' and eth0/wlan0 show; Update check says 'Not recorded' until the new script's first run, then the recorded outcome"
    why_human: "The production binary is v0.1.0 without this phase. Replacing it or restarting the service is the operator's call. Everything checkable without doing that was checked against a dev daemon on the same Pi, including under a real subset=pid /proc"
  - test: "Decide how to treat the full gate: ./bin/task ci went red once in this verification, in internal/upgrade TestANodeSaysHowItBooted"
    expected: "Either accept it as a pre-existing load-dependent flake outside this phase and push after a green rerun, or schedule a fix for the timeout before relying on the local gate again"
    why_human: "The failure is 'talos: Get … timed out' in a package this phase did not touch. The test passes alone in 2.6 s, and 11-07 saw the same test time out under full-suite load. Every other step was green, including the layout audit run separately. Whether an intermittently red local gate is acceptable is the operator's decision, since no push CI runs"
---

# Phase 11: Host-Seite — Gerät, Dienst, Live-Werte Verification Report

**Phase goal:** The operator opens `/host` and sees the device holzkube-manager runs on, the service itself and its live values: CPU, memory, disk, temperature and network. Where the unit's hardening hides a value, the page says "not readable" with the cause, never 0.
**Verified:** 2026-09-28T18:43:25Z
**Status:** human_needed
**Re-verification:** No. This is the initial verification. No earlier VERIFICATION.md existed.
**Where this ran:** On the operator's Raspberry Pi 5, natively on aarch64. Go was 1.26.7 from the user's local Go, with no `-race`. Node 22 came from nvm, and Chromium from Playwright. `holzkube-manager.service`, its binary and `/var/lib/holzkube-manager` were not touched: `ActiveEnterTimestamp` was still the 18:06 CEST start that 11-07 recorded. Every dev daemon ran on 127.0.0.1 with a data directory from `mktemp -d`. All of them were stopped and their directories removed.

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SC1: `/host` shows hostname, model, architecture, OS, kernel and uptime since boot, and the shell on the same machine names the same values. Beside them: the service version, the process uptime, and the data directory's size and free space | ✓ VERIFIED | A dev daemon was built from HEAD and run on the Pi. A script compared its `/api/v1/host` with the shell over two fresh fetches 3.1 s apart: 57 checks, 0 failed. It checked `uname -n/-r/-m`, `/proc/uptime` (±10 s), the device-tree model with the NUL removed, `PRETTY_NAME`, the online CPU count, `version == v0.0.0-verify` (the ldflags value), `data_dir.path`, and `du -s -B1` of the data directory. Chromium drove the real page at 390 px and it showed the Device and Service cards. Only field names and pass/fail were printed; no identifiers are recorded here |
| 2 | SC2: live without reloading: CPU usage, load, memory and swap; `/` and the data directory's filesystem; temperatures from thermal/hwmon and fans where present; throughput per interface. On the Pi the values agree with `/proc` and `/sys` | ✓ VERIFIED | The same comparison run checked MemTotal/SwapTotal exactly and used/cache/swap-used within 2 %. Load matched `/proc/loadavg` with source `loadavg`. There was one row `root · data directory` (the data directory is on `/`), with size equal to `df -B1` and used/avail within 16 MiB. `cpu_thermal` was within 3 °C of `thermal_zone0`, and no voltage chip appeared. eth0 was physical with its state equal to operstate, and lo was virtual. First poll: `rate.no-baseline` and null rates. Second poll: readable usage, one per-core value per core, and a non-null eth0 rate. Live in a real browser: over 7 s the page made 3 `/api/v1/host` requests on its own, the footer time changed, the "Rates are over …" clause appeared, and "Waiting for a second reading" went away. Chromium at 390 px showed CPU 0–3, load, Swap, `root · data directory`, °C values, "No fan reported", eth0 and the virtual-interfaces disclosure, with scrollWidth equal to innerWidth (390) |
| 3 | SC3: under `ProcSubset=pid`, CPU and memory show "not readable" with the cause and the `ProcSubset=all` line, never 0; load comes from sysinfo(2) and stays exact. The guard has been seen red against a reinstated 0 | ✓ VERIFIED | The same binary ran under `unshare … mount -t proc -o subset=pid,hidepid=invisible`. In both rounds, usage, per_core and memory were `readable:false`, code `hardening.proc-subset`, with no `value` key. Load was readable with `source: sysinfo`, and the difference from the outer `/proc/loadavg` was 0.00/0.00/0.00. Device, filesystems and network stayed readable, and round 2 had `rates_over_seconds`. **Two faults were reinstated in this verification.** (a) `collector.go` returned `Read(0.0)` for hidden usage: `TestHardeningHidesCPUAndMemory` went red, and so did `TestRealKernelProcSubset` on the real kernel ("usage: readable=true, value present=true"). (b) `host.tsx` rendered `formatPercent(0)` in place of MissingValue: 3 tests in host.test.tsx went red, among them "…never 0". Both files were restored, and `git status` was clean afterwards |
| 4 | SC4: when the update mechanism records the last check and the available version, the page shows both; when it records nothing (the script installed on the Pi today), it says "not recorded". The future script is in `deploy/` and is not installed | ✓ VERIFIED | On the Pi, `/var/lib/holzkube-manager-update` does not exist. The installed `/usr/local/sbin/holzkube-manager-update` contains no `status.json` or `record_status`. The dev daemon answered `update.not-recorded`, with no value key. With `--update-status-file` pointing at a valid file, the answer was readable with checked_at, installed and latest exactly as written. With `checked_at: "yesterday"` it was `read-failed`, "…not an RFC 3339 time", with nothing invented. `deploy/holzkube-manager-update.sh` has `trap on_exit EXIT` and `record_status`. The installed script already self-replaces from the release archive (`cmp -s … install`), so the server's sentence "the next update brings it" holds |
| 5 | GET /api/v1/host: 401 without a session, 200 for reader and admin, no audit record, one observed_at | ✓ VERIFIED | Against the dev daemon: 401 unauthenticated and 200 after login. The audit record count was 4 before and 4 after three host reads. The route has `RequiresSession`, `MinRole: RoleReader` and no Action (handlers/host.go). hostapi_test and endtoend_test pass in the gate |
| 6 | Every value is a Reading: a value, or a reason with no value key, never a 0 standing in for an unread value (D-02) | ✓ VERIFIED | `Reading[T]` keeps its value behind a pointer tagged `omitempty` (host.go). The zod `reading()` union has no `.default(0)` (api.ts). The injections under #3 show both sides are guarded |
| 7 | The Pi 5 and amd64 fixture identities: device-tree model with the NUL removed, PRETTY_NAME, cores from "0-3", DMI "vendor product" | ✓ VERIFIED | `go test ./internal/host` passes (identity_test). The real Pi model matched the device tree exactly (#1) |
| 8 | Container detection (/.dockerenv, /run/.containerenv, container= in PID 1's environment) gives the D-17 sentence, the hostname and Network badges, and the container sentence for update status | ✓ VERIFIED | identity.go reads all three markers. collector.go uses `notRecordedContainer`. host.tsx renders the sentence and both badges. host.test covers them. On the Pi, `container:false` |
| 9 | UI states: loading "Reading the host…", first-poll error card, stale notice with opacity-60, partial "Not readable" row | ✓ VERIFIED | HostPage tests cover loading and error. In a real browser the dev daemon was stopped mid-session, and within 4.5 s the page showed "did not answer the latest request". Device stayed on screen and one `.opacity-60` element was present, so the success-to-failure transition was exercised live |
| 10 | The navigation has "Host" with the Cpu icon after Nodes and no state dot; `/host` is in the layout audit ROUTES; demo.json carries a host answer that parses | ✓ VERIFIED | Sidebar.tsx lines 76–82. `hostRoute` is in App.tsx. layout-audit.mjs line 77. fixtures.test.ts passes |
| 11 | (backstop) Long host identifiers wrap and never widen the page at 390 px | ✓ VERIFIED | The explicit evidence is host.browser.test.tsx, which passed in Chromium. **It was seen red in this verification:** with `break-words` removed from Row's `dd`, the long hostname ended at 504.9 px and the kernel at 652.3 px, both past 390.5 px. The file was restored |
| 12 | (backstop) The amd64 fixture (k10temp, nct6798, fans) renders without horizontal overflow | ✓ VERIFIED | The same browser test's amd64 case passed in Chromium. The layout audit reported `/host` ok at 390 px (4 controls, 7 items) and 1280 px against the built product |
| 13 | Rate semantics: the first read has no baseline ("Waiting for a second reading", no 0 %), and a read under 0.5 s later keeps the older baseline. Network rates are null on the first read, for a new interface, or for a counter that went backwards, and render as "—" | ✓ VERIFIED | Shown live (#2), and covered by live_test/network_test and by the host.test case "never 0 B/s". collector.go keeps the memo on a window under `minRateWindow` |
| 14 | The update script: one EXIT trap that never changes the exit code; nothing written for --help, --rollback, non-root, or an unwritable directory; refusal of a symlinked, non-root or group/other-writable status directory; tags reach the JSON only through python3 argv; the strict reader | ✓ VERIFIED | `TestUpdateScript` and `TestUpdateScriptAsRoot` both ran for real, the root cases through `unshare --user --map-root-user`. There were 14 root subtests, including symlink, other uid and four modes, with nothing skipped. `TestReadStatus` covers absent, oversize, malformed, unknown outcome, bad time, missing key and null outside failed |
| 15 | Filesystems: merged by major:minor into one meter "root · data directory"; df-exact size; data directory size equal to `du -s -B1`, cached 60 s, singleflight, 5 s deadline | ✓ VERIFIED | Confirmed live on the Pi (#2). filesystems_test covers dedupe, two devices, a symlinked data directory, the prefix boundary, statfs failure, the cache, and vanished entries |
| 16 | Sensors use one set of rules with the node path (talos.ClassifyChip, InputsNamed, Numbered, Millidegrees, ThermalTwinName) and one shared UI module. On the Pi 5: cpu_thermal and rp1_adc; rpi_volt dropped; no voltages; no thermal twin; the no-fan sentence | ✓ VERIFIED | host/sensors.go calls all five talos functions. NodeHardware.tsx and host.tsx both import `@/components/charts/Sensors`. The live Pi gave exactly `cpu_thermal` and `rp1_adc` with 0 fans, and the browser showed "No fan reported" |
| 17 | Network: physical means a `device` link exists; virtual interfaces are collapsed into "N virtual interfaces" (min-h-11); speed on EINVAL is null; the empty-state sentence | ✓ VERIFIED | network.go and TestNetworkSpeedEINVAL. host.tsx `<summary className="flex min-h-11 …">`. The layout audit found every control at least 44 px at 390 px |
| 18 | `--update-status-file` / `HOLZKUBE_MANAGER_UPDATE_STATUS_FILE`, defaulting to `/var/lib/holzkube-manager-update/status.json`, documented in the guide, and held equal to the script's default by a test | ✓ VERIFIED | config.go:281. guide.md lines 98, 118 and 189. config_test.go:756 reads the script's `STATUS_DIR` default. main.go passes `UpdateStatusPath: cfg.UpdateStatusFile` |
| 19 | docs/api-contract.md describes GET /api/v1/host completely. The guide explains Not readable under ProcSubset=pid, the one line that changes it, and ProtectHostname | ✓ VERIFIED | The contract section is present, and the reason codes and option are mentioned 10 times. The guide has "### The Host page", `ProcSubset=all` at line 175 and `ProtectHostname` at line 183. `TestEveryProblemCodeIsInTheContract` passes in the gate |
| 20 | `./bin/task ci` is green on the Pi | ? UNCERTAIN | This verification ran the chain once, and it exited **201**. The only failure was `internal/upgrade TestANodeSaysHowItBooted` ("talos: Get … timed out", 31.5 s), in a package this phase did not touch. Run alone it passed in 2.6 s, and 11-07 recorded the same test timing out under load. Everything else was green: web lint, web tests (55 files, 537 tests), build, golangci-lint 2.13.1 (0 issues), and every other Go package. `task test:layout` was run separately because the chain stopped before it, and it was green. See Human Verification #2 |
| 21 | The production service, its binary and its data directory were not touched | ✓ VERIFIED | `ActiveEnterTimestamp` was unchanged (18:06 CEST). This verification also left them alone |
| 22 | The README carries the feature (project rule) | ✓ VERIFIED | README "What it does" line 43 ("The machine it runs on — the Host page") and "A look around" with `docs/screenshots/host.png` |

**Score:** 21/22 truths verified (0 present-but-behavior-unverified, 1 uncertain).

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/host/host.go` | Reading[T], reason codes, View types | ✓ VERIFIED | 216 lines, `type Reading[T any] struct` |
| `internal/host/sys*.go` | Sys seam, linux impl, non-linux stub | ✓ VERIFIED | `//go:build linux` / `!linux`. The darwin build is covered by the WR-03 unsupported gate |
| `internal/host/collector.go` | Collector, Config, New, Read | ✓ VERIFIED | 443 lines. Wired from main.go |
| `internal/host/identity.go`, `mountinfo.go`, `proc.go`, `filesystems.go`, `sensors.go`, `network.go` | readers | ✓ VERIFIED | Substantive and exercised live on the Pi |
| `internal/host/realkernel_test.go` | subset=pid on the real kernel | ✓ VERIFIED | Runs (not skipped) and was seen red under injection |
| `internal/host/updatestatus/status.go` | strict reader | ✓ VERIFIED | Used by collector.go and script_test |
| `deploy/holzkube-manager-update.sh` | record_status + EXIT trap | ✓ VERIFIED | Lines 124 and 223–231. Not installed on the Pi |
| `internal/httpapi/handlers/host.go` | HostRoutes | ✓ VERIFIED | Registered in main.go's route table |
| `web/src/routes/host.tsx` | page | ✓ VERIFIED | 779 lines. Rendered in a real browser against a real daemon |
| `web/src/components/charts/Sensors.tsx` | shared sensor module | ✓ VERIFIED | Imported by NodeHardware and host |
| `web/src/routes/host.browser.test.tsx` | 390-px measurement | ✓ VERIFIED | Passes, and went red under the break-words injection |
| `docs/api-contract.md`, `docs/guide.md` | contract, operator text | ✓ VERIFIED | See #19 |

### Key Link Verification

| From | To | Via | Status |
|------|----|-----|--------|
| main.go | host.New | `Host: hostCollector` inside the Deps literal | ✓ WIRED |
| main.go | handlers.HostRoutes | route table, line 878 | ✓ WIRED |
| main.go | collector | `UpdateStatusPath: cfg.UpdateStatusFile`, `DataDir: dataDirAbs`, `Version: version`, `Started: started` | ✓ WIRED |
| host.tsx | api.host | `useQuery(['host'], api.host, refetchInterval 3000)` | ✓ WIRED (3 requests in 7 s observed) |
| App.tsx | hostRoute | addChildren | ✓ WIRED (`GET /host` returns the SPA, 200) |
| collector.go | inventory.CPUPercent / MemoryView | direct calls | ✓ WIRED |
| collector.go | classify(…, procSubsetPid) | /proc/stat, meminfo, loadavg | ✓ WIRED |
| collector.go | updatestatus.Read | readService | ✓ WIRED (both branches shown live) |
| host/sensors.go | talos.ClassifyChip/InputsNamed/Numbered/Millidegrees/ThermalTwinName | direct calls | ✓ WIRED |
| NodeHardware.tsx, host.tsx | charts/Sensors | import | ✓ WIRED |
| script_test.go | deploy script + updatestatus.Read | copy + run + parse | ✓ WIRED |

### Data-Flow Trace (Level 4)

| Artifact | Data | Source | Real data | Status |
|----------|------|--------|-----------|--------|
| host.tsx Device/Service | `host.device`, `host.service` | uname(2), CLOCK_BOOTTIME, /sys, os-release, ldflags version, du walk | Yes: matched the shell | ✓ FLOWING |
| host.tsx Live | `host.live.*` | /proc/stat, meminfo, loadavg or sysinfo, statfs, hwmon/thermal, /sys/class/net | Yes: matched the shell, and changed between polls | ✓ FLOWING |
| Update check | `service.update` | status.json via updatestatus.Read | Yes: all three branches seen | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| host packages and related | `go test -count=1 ./internal/host/... ./internal/config ./internal/publicrepo ./internal/talos ./internal/inventory` | rc=0 | ✓ PASS |
| Hardening guard vs a reinstated 0 (Go) | inject `Read(0.0)`, `go test -run 'TestHardeningHidesCPUAndMemory\|TestRealKernelProcSubset'` | rc=1, both red | ✓ PASS (guard live) |
| Hardening guard vs a rendered 0 % (UI) | inject `formatPercent(0)`, `vitest run src/routes/host.test.tsx` | rc=1, 3 red | ✓ PASS (guard live) |
| 390-px wrap guard | inject: drop `break-words`, `npm run test:browser -- host.browser.test.tsx` | rc=1, red | ✓ PASS (guard live) |
| host UI tests | `vitest run host.test.tsx NodeHardware.test.tsx fixtures.test.ts` | 87 passed | ✓ PASS |
| Dev daemon vs shell | compare script, 2 fetches | 57/57 | ✓ PASS |
| Dev daemon under subset=pid | unshare + subset=pid proc | hardening.proc-subset ×3, load from sysinfo exact | ✓ PASS |
| Update status readable / malformed | `--update-status-file` to a scratch file | readable, then read-failed | ✓ PASS |
| Live polling + stale in a real browser | Playwright against the dev daemon, daemon killed mid-session | 3 polls in 7 s; stale notice + opacity-60 after the kill | ✓ PASS |
| Full gate | `./bin/task ci` | rc=201 (internal/upgrade timeout only) | ? see truth #20 |
| Layout audit | `./bin/task test:layout` | rc=0, `/host` ok at 390 and 1280 | ✓ PASS |

### Probe Execution

Step 7c: SKIPPED. The phase declares no `probe-*.sh` scripts, and there are none under `scripts/*/tests/`.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| HOST-01 | 11-01, 11-07 | Device: hostname, model, arch, OS, kernel, uptime | ✓ SATISFIED | Truth #1 |
| HOST-02 | 11-04, 11-07 | Service: version, process uptime, data directory size and free space | ✓ SATISFIED | Truths #1 and #15 |
| HOST-03 | 11-02, 11-04, 11-07 | Last update check and available version, as far as recorded | ✓ SATISFIED | Truths #4 and #14 |
| HMON-01 | 11-03, 11-07 | CPU, load, memory, swap live | ✓ SATISFIED | Truths #2 and #13 |
| HMON-02 | 11-04, 11-07 | Disk usage of the data directory's filesystem and `/` | ✓ SATISFIED | Truths #2 and #15 |
| HMON-03 | 11-05, 11-07 | Temperatures (thermal/hwmon) and fans | ✓ SATISFIED | Truths #2 and #16 |
| HMON-04 | 11-06, 11-07 | Network throughput | ✓ SATISFIED | Truths #2 and #17 |
| HMON-06 | 11-03, 11-07 | Hidden by hardening shows "not readable" with the cause, never 0 | ✓ SATISFIED | Truth #3 |

REQUIREMENTS.md maps no other ID to Phase 11. HOST-04, HMON-05 and HMON-07 belong to Phase 12. None is orphaned.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (phase files) | — | TBD/FIXME/XXX | — | None. The only hits are mktemp templates (`.status.XXXXXX`) in the update script |
| internal/upgrade/live_test.go | 842 | Load-dependent timeout in `TestANodeSaysHowItBooted` | ⚠️ Warning | Not phase code. It turns the local full gate red intermittently (truth #20) |
| internal/host/updatestatus/script_test.go | 486 | The SIGINT signal case sometimes waits out the stub's `sleep 60` (took 60.07 s once under parallel load; 0.06–0.11 s in 4 other runs). It still passes | ℹ️ Info | Slow and intermittent, not wrong. It probably comes from a signal landing between `touch ready` and `exec sleep` |
| internal/host/host.go, web/src/routes/host.tsx | doc / notice | "The systemd unit this product ships" / "The systemd unit sets": the repository ships no unit | ℹ️ Info | Already noted in 11-07's deviation 3 for Phase 12/13. On screen the text only appears when subset=pid was actually detected |
| web/src/components/charts/Sensors.tsx | 55–57 | The React key `chip/label` collides for two chips of the same name (IN-05, skipped) | ℹ️ Info | Duplicate key and shared history for two NVMe drives. It does not affect the Pi |
| docs/screenshots (others) | — | Only host.png was re-rendered, so the other screenshots show the navigation without Host | ℹ️ Info | Cosmetic, recorded in deferred-items.md |

### Human Verification Required

#### 1. /host on the production service after the next release

**Test:** Once the update timer has installed the next release, open `/host` signed in.
**Expected:** Device and Service show the Pi. The slate notice names CPU usage, memory and swap, and shows `ProcSubset=all` (the unit has `ProcSubset=pid`). Load, the merged `root · data directory` meter, cpu_thermal and rp1_adc, "No fan reported" and eth0/wlan0 all show. Update check says "Not recorded" until the new script's first run, then the recorded outcome.
**Why human:** The production binary predates this phase, and replacing or restarting it is the operator's call. Every part of this was already exercised against a dev daemon on the same Pi, both plain and under a real subset=pid /proc.

#### 2. The intermittently red full gate

**Test:** Decide how to handle `internal/upgrade TestANodeSaysHowItBooted` timing out under full-suite load.
**Expected:** One of: accept it as a pre-existing flake outside Phase 11 and rerun until green before a push; or schedule a fix (a longer deadline, or less parallelism for that test).
**Why human:** No push CI runs, so the local gate is the only gate. Whether it may be intermittently red is a policy choice, not a code fact.

### Gaps Summary

None found. The phase goal is achieved in the codebase and on the Pi. The API matches the shell. The subset=pid path reports `hardening.proc-subset` and never a 0. That guard was seen red against a reinstated 0 in Go, on the real kernel and in the UI. The page polls live and degrades to a stale view. The update status reads `update.not-recorded` today and shows a recorded status verbatim. Two items remain for the operator: looking at the page on the production service after a release, and deciding on the unrelated flaky test that turned the full gate red once.

---

_Verified: 2026-09-28T18:43:25Z_
_Verifier: Claude (gsd-verifier)_

## Orchestrator disposition (2026-09-28, unattended run)

The operator ordered this milestone to run without questions, so the two
human items above were not put to them mid-run. They are carried in
`11-UAT.md` as pending and will be listed in the milestone's closing report:

1. `/host` on the production service — possible only after a release, which
   the operator cuts. Everything checkable before that was checked on the Pi
   against a dev daemon (57/57 against the shell, and under a real subset=pid).
2. `internal/upgrade TestANodeSaysHowItBooted` times out under load in the full
   local gate; it passes alone. Not touched by this phase; recorded for a fix.

The status is set to `passed` for routing on that basis, not because the
human items were done.
