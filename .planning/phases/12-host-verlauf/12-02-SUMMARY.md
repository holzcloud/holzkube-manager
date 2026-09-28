---
phase: 12-host-verlauf
plan: 02
subsystem: api
tags: [go, sysfs, thermal, hwmon, health, limits]
status: complete

requires:
  - phase: 11-host-seite
    provides: "host.readSensors/readChip, Live, Reading/Reason, Filesystem/FSUsage (df's used/available)"
provides:
  - "inventory.TemperatureLimits(kind, high, crit) (warn, danger): the one rule, ported from the browser's temperatureLimits"
  - "warn_c/danger_c on inventory.HardwareTemperature (node and host); temperature_warn_c/temperature_danger_c on inventory.HardwareDisk"
  - "host trip-point limits: twin zone's lowest passive/hot = high, lowest critical = crit; active trips ignored; maxTripPoints = 16"
  - "host.Health, host.HealthState (ok/warn/unknown), host.Assess(Live)"
affects: [12-03, 12-04, 12-05, 12-06, 12-08]

actuals:
  tokens: 13300
  tasks: 3
  commits: 3
plan_head_before: f0b8549cf84890efc497c986050e1a3c024047d5
plan_head_after: a66198fa2edfb92ae2f24687db83d34a8410f469

tech-stack:
  added: []
  patterns:
    - "Temperature lines are computed once, in Go, and sent with every reading; the browser is to print them, not derive them"
    - "Zones are read before the hwmon chips so a chip without limits can take its twin zone's; a zone listing error only matters when the zones are the CPU's temperature"
    - "Filesystem threshold compared in 128-bit integers (bits.Mul64) on used vs used + available, never on the rounded percent"

key-files:
  created:
    - internal/inventory/limits.go
    - internal/inventory/limits_test.go
    - internal/host/health.go
    - internal/host/health_test.go
    - internal/host/testdata/pi5/sys/devices/virtual/thermal/thermal_zone0/trip_point_{1..4}_{type,temp}
  modified:
    - internal/inventory/hardware.go
    - internal/inventory/hardware_test.go
    - internal/httpapi/hardwareapi_test.go
    - internal/host/sensors.go
    - internal/host/sensors_test.go

key-decisions:
  - "A drive's temperature_warn_c/danger_c are TemperatureLimits of the very sensor its figure is taken from (RESEARCH Open Question 1, option 2)"
  - "A trip at or below 0 °C is a disabled trip and sets no line, as a chip limit at or below zero sets none"
  - "Unreadable sentences drop the reason message's own final period before adding theirs, so no sentence ends in '..'"
  - "A filesystem whose used + available is 0 (or wrapped) is not rated rather than divided by"

patterns-established:
  - "health_test builds Live values directly with CPU and memory hidden by ProcSubset=pid, so every case also proves those rate nothing"

requirements-completed: [HMON-07, HOST-04]

coverage:
  - id: D1
    description: "TemperatureLimits reproduces the browser's rule case for case; every node temperature and drive carries its lines"
    requirement: HMON-07
    verification:
      - kind: unit
        ref: "internal/inventory/limits_test.go#TestTemperatureLimits"
        status: pass
      - kind: integration
        ref: "internal/inventory/hardware_test.go#TestHardwareDiscoversClassifiesAndAttachesTheSensors"
        status: pass
  - id: D2
    description: "The Pi 5's CPU warns at 80 and is critical at 110; active trips are never a line; 16-trip cap"
    requirement: HMON-07
    verification:
      - kind: unit
        ref: "internal/host/sensors_test.go#TestTripPoints, #TestSensorsPi5"
        status: pass
  - id: D3
    description: "Assess warns at the lines with its sentences and order, and is never ok when a rated value is unreadable"
    requirement: HOST-04
    verification:
      - kind: unit
        ref: "internal/host/health_test.go#TestAssessWarns, #TestUnreadableIsNeverHealthy"
        status: pass

duration: ~18 min
completed: 2026-09-28
---

# Phase 12 Plan 02: Temperature limits and host.Assess Summary

**Temperature lines are now computed once in Go and sent with every node and host temperature and every node drive. The Pi's CPU gets its red line from its thermal zone's critical trip (110 °C) and never from a fan stage. `host.Assess` decides ok / warn / unknown, and it cannot call a host healthy when something it rates could not be read.**

## Where this ran

The operator's Raspberry Pi 5 (aarch64), Go 1.26.7 via `GOTOOLCHAIN`. There was no `-race` run: ThreadSanitizer does not work on the Pi 5 kernel, so the race detector runs only in CI. The production service, its data directory and `/usr/local/bin` were not touched.

## Accomplishments

- **`inventory.TemperatureLimits`** (`limits.go`). This is the browser's rule, case for case. The chip's own limits win. A high limit at or above danger is not used as a warning line. Warn is never above danger. The per-kind defaults are cpu 80/95, disk 60/70, board 70/85, gpu 80/95 and other 75/90.
- **`warn_c`/`danger_c`** on every `HardwareTemperature`, computed from the rounded limits that the JSON carries. **`temperature_warn_c`/`temperature_danger_c`** on `HardwareDisk`, taken from the sensor the drive's figure comes from. Both are null exactly when `temperature_c` is null. The browser's zod objects are not strict, so it ignores these keys until plan 04 uses them.
- **Host trip points** (`sensors.go`). An hwmon temperature whose chip sets no max or crit takes them from its twin zone: the lowest passive/hot trip becomes high and the lowest critical trip becomes crit. The zone fallback uses its own zone's trips. Active trips and trips without a type are ignored. Each zone is read up to `maxTripPoints = 16`, and every file goes through `readTrimmed`. If `/sys/class/thermal` cannot be listed while a CPU hwmon chip exists, limits fall back instead of the sensors failing. `WarnC`/`DangerC` are set through `inventory.TemperatureLimits` at both construction sites. The comment that said trip points are never used as limits has been rewritten (D-07).
- **Read-only check against the Pi's real sysfs** (a temporary test file, deleted afterwards): `cpu_thermal/temp1 kind=cpu high=<nil> crit=110 warn=80 danger=110` and `rp1_adc/temp1 kind=other warn=75 danger=90`. This matches the fixture.
- **`host.Assess`** (`health.go`, pure).
  - It rates two things: the temperatures, and every filesystem row at 80 % of used + available, using `>=` and integer arithmetic.
  - CPU and memory are not rated.
  - Warning sentences follow the UI-SPEC. Critical warnings come first, then the rest by relative excess, with ties kept in reading order.
  - State: any warning means warn, and the unreadable values are still listed. Otherwise any unreadable rated value means unknown. Otherwise the state is ok, using one of two summaries.
  - `Warnings` and `Unreadable` are never null.

## Task Commits

1. **Task 1: one Go rule for temperature limits** — `63ca4d9` (feat)
2. **Task 2: host limits from trip points** — `e151d89` (feat)
3. **Task 3: host.Assess** — `a66198f` (feat)

## Fault injections (each seen red, then restored)

Before each fault the production file was copied to the scratchpad, and afterwards it was copied back. `cmp` confirmed the restored file was identical to the copy before the next step. Exit codes were read from the `go test` command itself, with its output going to a file.

| # | Fault | Test | Observed (exit 1 each) |
|---|-------|------|------------------------|
| 1a | `high < danger` guard removed from `TemperatureLimits` | TestTemperatureLimits | `TemperatureLimits("cpu", 100, 90) = 100/90, want 80/90`; `("cpu", 90, 90) = 90/90, want 80/90` |
| 1b | `flattenSensors` leaves WarnC/DangerC unset and gives drives a fixed 60/70 (extra; first attempt did not compile and was redone so it did) | TestHardwareDiscoversClassifiesAndAttachesTheSensors | `coretemp "Package id 0" has warn/danger 0/0, want TemperatureLimits' 80/100`, … plus two lines like `nvme0n1 turns amber/red at 60/70, its Composite sensor at 0/0` |
| 2a | `active` counted as a high candidate | TestSensorsPi5, TestTripPoints | `cpu_thermal/temp1 (cpu) 64.4 high 50 crit 110 warn 50 danger 110`, want `… warn 80 danger 110`; also red: "active trips…", "zone fallback…", "at most sixteen…" |
| 2b | `critical` ignored | TestSensorsPi5, TestTripPoints | Pi `warn 80 danger 95`, want `crit 110 warn 80 danger 110`; three TestTripPoints rows red |
| 2c | trip points read without the cap (`maxTripPoints + 1`) | TestTripPoints/at_most_sixteen_trip_points_are_read | `high 60 warn 60 danger 95`, want `warn 80 danger 95` |
| 2d | zone listing error returned even when a CPU chip reported (extra) | TestTripPoints (two unlistable rows) | `readSensors: open sys/class/thermal: permission denied` |
| 3a | temperature check replaced by false | TestAssessWarns | `state = "ok", want "warn"` on "a CPU past its warning line", "past danger is critical…", and others |
| 3b | filesystem check replaced by false | TestAssessWarns | `state = "ok", want "warn"` on "df's percent is rounded up" and "the denominator is used + available" |
| 3c | `used/size` as the denominator (`total := u.SizeBytes`) | TestAssessWarns/the_denominator_is_used_+_available,_not_size | `state = "ok", want "warn"` |
| 3d | the unknown branch returns ok (Assess ok whatever it could not read) | TestUnreadableIsNeverHealthy | every unreadable combination: `{State:ok Summary:Usage of / could not be read…}, want unknown with its reasons`; "the sentences" and "no filesystem row at all" red |
| 3e | unreadable filesystem rows skipped silently (extra) | TestUnreadableIsNeverHealthy | `root not readable, no data row: {State:ok …Unreadable:[]}, want unknown`; `with a full disk: warn lists nothing unreadable` |

## Verification (all on the Pi)

- `go test ./internal/inventory ./internal/host/... ./internal/history ./internal/httpapi/... -count=1`: exit 0. Timings under load: inventory 142 s, httpapi 284 s, handlers 113 s.
- `GOOS=darwin GOARCH=arm64 go build ./...`: exit 0.
- `./bin/task lint:go`: 0 issues. The first run found 1 gofmt `-s` issue and 3 revive comment-form issues in the new file; all four were fixed before the Task 3 commit.
- `go test ./internal/publicrepo/`: ok.
- `go.mod`/`go.sum` unchanged. No package was added.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] The contract-shape test fixed the exact JSON keys**
- **Found during:** Task 1, the plan's second verify command
- **Issue:** `TestHardwareRouteAnswersTheContractShape` asserts the exact key sets of a disk and a temperature, so the new fields turned it red: `a temperature has keys [... danger_c ... warn_c], want exactly [...]`.
- **Fix:** Added `temperature_warn_c`/`temperature_danger_c` to `hardwareDiskKeys`, and `warn_c`/`danger_c` to `hardwareTemperatureKeys`, in `internal/httpapi/hardwareapi_test.go`. The test is still exact, now over the new contract.
- **Commit:** 63ca4d9

**2. [Rule 2 - Robustness] A wrapped or zero `used + available` is not rated**
- `bits.Div64` panics when the quotient overflows. statfs cannot produce such a sum, but the daemon should not be able to panic on one, so `filesystemFinding` returns "no finding" for it.

**3. Extra test rows beyond the plan's list**
- In TestTripPoints: "a disabled trip at zero sets no line", and a second unlistable case (CPU chip without limits gets the kind's defaults). In TestUnreadableIsNeverHealthy: "the lists are never null" (Ledger 162). Extra injections 1b, 2d and 3e.

### Notes

- The reason sentences use the reason message without its final period, then add their own. This applies to the filesystem sentence too (the plan names it only for temperatures), so a message ending in "." does not produce "..".
- `docs/api-contract.md` does not document the new fields yet. That is 12-08's acceptance (`grep -c 'warn_c' docs/api-contract.md ≥ 1`).
- The 12-01 deferred item about `lint:web` was not rechecked here: this plan changed no web file. The orchestrator reports that `lint:web` exits 0.

## Known Stubs

None. `Assess` is not wired into `host.View` or the wall yet; plan 03 does that. The browser keeps its own `temperatureLimits` until plan 04 removes it.

## Threat Flags

None. T-12-05 is mitigated (16 trips, 64 zones, bounded reads, and the 17-trip test was red without the cap). T-12-06 is mitigated (injections 2a, 3a to 3e). No new endpoint and no new file outside `/sys/class/thermal/*/trip_point_*`.

## Self-Check: PASSED

- FOUND: internal/inventory/limits.go, internal/inventory/limits_test.go, internal/host/health.go, internal/host/health_test.go, internal/host/testdata/pi5/sys/devices/virtual/thermal/thermal_zone0/trip_point_4_type
- FOUND: 63ca4d9, e151d89, a66198f
