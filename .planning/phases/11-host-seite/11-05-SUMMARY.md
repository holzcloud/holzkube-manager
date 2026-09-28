---
phase: 11-host-seite
plan: 05
subsystem: api, ui
tags: [go, hwmon, thermal, fs.FS, symlinks, react, zod, shared-components]
status: complete

requires:
  - phase: 11-03
    provides: "Reading, Hidden, reasonFor, readBounded, fixtureFS, nullableKeys, the /host Live section"
  - phase: 11-04
    provides: "Live.Filesystems, the Live grid's left column (Processor, Memory, Filesystems)"
provides:
  - "talos: exported InputsNamed, Numbered, Millidegrees, ThermalTwinName; thermalZones calls ThermalTwinName"
  - "internal/host/sensors.go: Sensors{Temperatures, Fans} in inventory element types, readSensors over fs.FS"
  - "Live.Sensors Reading[Sensors]; listing failure is read-failed, a missing class directory is two empty lists"
  - "testdata/pi5 hwmon/thermal trees with the Pi 5's real symlinked layout; testdata/amd64 hwmon/thermal trees"
  - "web/src/components/charts/Sensors.tsx: TEMPERATURE_DEFAULTS, temperatureLimits, KIND_LABEL, SEVERITY_COLOR, sensorKey, Sensors, FanList -- shared by NodeHardware and /host"
  - "api.ts: temperatureSchema, fanSchema, Temperature, Fan; hostSchema.live.sensors"
  - "the Sensors card on /host; demo fixture in the Pi 5 layout"
affects: [11-06, 11-07, 12]

actuals:
  tokens: 18278
  tasks: 3
  commits: 3
plan_head_before: 34e6c5e5f2de645b9946c64133a9e5ddd198a7d1
plan_head_after: 7933fe94c7cddf962c4665224bb8a73c07a8607b

tech-stack:
  added: []
  patterns:
    - "A /sys/class listing is never filtered on IsDir: the entries are symlinks, and reading through the path lets the FS follow them"
    - "Thermal zones are a fallback only when no CPU-kind hwmon chip reported a temperature, and a zone whose hwmon twin is already listed is skipped"
    - "One display module, two pages: the sparkline column exists only when a history is passed, so /host shows no column of empty curves"

key-files:
  created:
    - internal/host/sensors.go
    - internal/host/sensors_test.go
    - internal/host/testdata/pi5/sys/class/hwmon/
    - internal/host/testdata/pi5/sys/class/thermal/
    - internal/host/testdata/pi5/sys/devices/
    - internal/host/testdata/amd64/sys/class/hwmon/
    - internal/host/testdata/amd64/sys/class/thermal/
    - web/src/components/charts/Sensors.tsx
    - web/src/components/charts/Sensors.test.tsx
  modified:
    - internal/talos/sensors.go
    - internal/talos/sensors_test.go
    - internal/host/host.go
    - internal/host/collector.go
    - internal/host/collector_test.go
    - web/src/components/NodeHardware.tsx
    - web/src/components/NodeHardware.test.tsx
    - web/src/api.ts
    - web/src/routes/host.tsx
    - web/src/routes/host.test.tsx
    - web/fixtures/demo.json

key-decisions:
  - "Chip classification, the twin rule and the threshold parsing are the talos package's own functions, exported -- not copied into internal/host"
  - "Thermal-zone trip points are never used as limits: a critical trip of 110 C would put the danger line far above where the firmware throttles"
  - "Only temp*_input and fan*_input are read on the host path; the Pi 5's rp1_adc voltages and rpi_volt never reach the answer"
  - "The pi5 fixture keeps real relative symlinks inside the fixture root; no repository guard refused them, so the MapFS fallback was not needed"

duration: ~60 min (across an interrupted run and its resume)
completed: 2026-09-28
---

# Phase 11 Plan 05: Host Sensors Summary

**/host now has a Sensors card. It reads hwmon and thermal zones through the node page's own talos rules and shows them with the node page's own list and fan rows, which now live in one shared module. On the Pi 5 that means cpu_thermal (Processor) and rp1_adc (Other). Where no fan is reported, the card says so and why that can happen.**

## Where this ran

On the operator's Pi (aarch64), natively: Go 1.26.7 from `~/.local/go`, Node v22.23.3. The race detector was not used (it cannot run on this kernel; that is CI's job). The production service was not touched.

## Tasks

| Task | Name | Commit |
| ---- | ---- | ------ |
| 1 | hwmon and thermal-zone walk over fs.FS with the node path's rules | 751db6b |
| 2 | sensor list, fan rows and temperature limits move to one shared module | 2c5cec8 |
| 3 | Sensors card on /host | 7933fe9 |

Tasks 1 and 2 were committed by an executor that was then interrupted. Task 3 was left uncommitted in the working tree. This run checked that diff, kept it unchanged (it was complete and correct), ran its verification and fault injections, and committed it.

## Verification (all exit codes read from the command itself)

- `GOTOOLCHAIN=go1.26.7 go test ./internal/host ./internal/talos ./internal/inventory ./internal/publicrepo -count=1`: exit 0
- `-run 'TestSensors|TestThermalTwinRule|TestNoNulls|TestEveryReadingIsWellFormed' -v`: PASS for TestSensorsPi5, TestSensorsAMD64, TestThermalTwinRule, TestSensorsNone, TestSensorsListFails, TestSensorsSkipsBadInput, TestNoNulls, TestEveryReadingIsWellFormed
- `npm --prefix web run test`: 54 files, 524 tests, exit 0
- `npm --prefix web run typecheck`: exit 0. `npm --prefix web run lint`: exit 0. Its 2 warnings and 1 info are older and sit in DataTable.tsx and wall.test.tsx, outside this plan.
- `./bin/task lint:go`: exit 0, 0 issues
- `./bin/task test:layout`: exit 0, /host ok at 390px and at 1280px
- NodeHardware.test.tsx has 8 `it(` cases, the same count as before the move (34e6c5e)
- Acceptance greps: four exported talos helpers. `talos.ClassifyChip` and `talos.ThermalTwinName` are used in host/sensors.go. `temperatureLimits` and `TEMPERATURE_DEFAULTS` are defined only in charts/Sensors.tsx. The no-fan and no-sensor sentences and the `@/components/charts/Sensors` import are in host.tsx. `"rp1_adc"` is in demo.json.

## Fault injections (each seen red, then reverted)

**Task 1** (re-run in this session so the literal output is on record; the first run's results are in 751db6b's message):

1. Drop the CPU-chip guard (`if true || !cpuTemps`). Red in `TestZonesOnlyWithoutCPUChip`: got `[coretemp/Package id 0 (cpu) 52, x86_pkg_temp/thermal_zone0 (cpu) 52]`, want only the coretemp entry. On the Pi 5 fixture, the twin rule on its own still keeps cpu-thermal out. That is why the plan's expected red for this injection lands in the dedicated test and not in TestSensorsPi5.
2. Drop the guard and the twin rule together. Red in TestSensorsPi5: got `[cpu_thermal/temp1 (cpu) 64.4, rp1_adc/temp1 (other) 55.4, cpu-thermal/thermal_zone0 (cpu) 65]`. Also red in TestThermalTwinRule (`acpitz/thermal_zone0` listed a second time), TestZonesOnlyWithoutCPUChip and TestSensorsAMD64.
3. Filter class entries on `IsDir()`. Red in TestSensorsPi5: got `[]`, want the two Pi 5 entries (every entry is a symlink).
4. From the first run (751db6b): dropping only the twin rule reddens TestThermalTwinRule. Leaving `high_c`/`critical_c` out of `nullableKeys` reddens TestNoNulls.

**Task 2** (first run, 2c5cec8): Sensors.test.tsx went red when the node page stopped passing `history` (no "Package id 0 temperature" img). It also went red when the sparkline column was drawn without a history.

**Task 3** (this session, against `src/routes/host.test.tsx`):

1. `FanList empty={null}`. 2 failed: the Pi 5 case and the no-sensor case both report "Unable to find an element with the text: No fan reported. Nothing under /sys/class/hwmon ...".
2. `emptyText=""`. 1 failed: "says the machine reports no temperature sensors" reports "Unable to find an element with the text: This machine reports no temperature sensors.".
3. `history={{}}` passed on /host. 1 failed: the Pi 5 case, where `expect(card.querySelector('svg')).toBeNull()` received an svg.

After each revert, `git diff --stat` matched the pre-injection state. The tree was clean after the task 1 re-runs.

## Deviations from Plan

**1. [Process] Resume of an interrupted run.** The task 3 work was already in the tree when this run started. It was verified, fault-injected and committed as it stood; nothing was rewritten.

**2. [Process] Commits on `main`.** `gsd_run query git.base-branch --is-protected main` returns true. The repository runs `branching_strategy: none`, not in a worktree, and every plan in this phase, including tasks 1 and 2 of this one, was committed on main. Task 3 and the docs commit follow the same practice.

**3. [Process] Dispatch file missing.** The orchestrator's `exec-prompt.md` was not in the scratchpad. This run followed the plan, the resume instructions in the spawn message, and the executor protocol.

Otherwise the plan was executed as written. The pi5 fixture uses real relative symlinks, so the plan's MapFS fallback was not needed.

## Known Stubs

None. The sparkline column is left out on purpose because the plan says no curves in this phase; curves are Phase 12's.

## Self-Check: PASSED

- FOUND: internal/host/sensors.go, internal/host/sensors_test.go, web/src/components/charts/Sensors.tsx, web/src/components/charts/Sensors.test.tsx
- FOUND: commits 751db6b, 2c5cec8, 7933fe9
