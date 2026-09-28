---
phase: 11-host-seite
plan: 03
subsystem: api, ui
tags: [go, procfs, mountinfo, subset-pid, sysinfo, rate-memo, unshare, react, zod]

requires:
  - phase: 11-01
    provides: "Reading[T], Read/Hidden, reason codes, Collector/Config/Sys, readBounded, fsPath, reasonFor, test helpers, /host page with MissingValue"
provides:
  - "internal/host/mountinfo.go: mountEntry, parseMountinfo, topmostAt, procSubsetPid"
  - "internal/host/proc.go: parseProcStat, parseMeminfo, parseLoadavg, loadFromSysinfo, classify"
  - "View.Live {rates_over_seconds, cpu{usage, per_core, load}, memory}; Load{load1,load5,load15,source}"
  - "Collector rate memo (counters, minRateWindow 0.5 s, maxRateWindow 5 min) guarded by a mutex"
  - "exported inventory.CPUPercent and inventory.MemoryView"
  - "testdata/procsubset-pid (production-unit mount shape incl. the data-dir bind mount) and testdata/pi5/proc/*"
  - "TestRealKernelProcSubset (unshare + proc mounted subset=pid)"
  - "/host Live section: Processor and Memory cards, hardening notice, rates clause; hostSchema.live; demo fixture in the production shape"
affects: [11-04, 11-05, 11-06, 11-07, 12]

actuals:
  tokens: 16719
  tasks: 3
  commits: 3
plan_head_before: c3ada26cca41e031fb11a6ab378d967a85c774ef
plan_head_after: 8ab8447b5703bda4d5bf6af4aa0305992218577d

tech-stack:
  added: []
  patterns:
    - "Hardening named only with proof: ENOENT AND subset=pid in the super options of the topmost /proc mount"
    - "Rate memo advanced only past the minimum window, so concurrent pollers never erase each other's baseline"
    - "Real-kernel test by re-executing the test binary inside unshare --user --map-root-user --mount --pid --fork"
    - "Zero guard in component tests checks each element, not the card's glued textContent"

key-files:
  created:
    - internal/host/mountinfo.go
    - internal/host/mountinfo_test.go
    - internal/host/proc.go
    - internal/host/proc_test.go
    - internal/host/live_test.go
    - internal/host/realkernel_test.go
    - internal/host/testdata/pi5/proc/{stat,meminfo,loadavg,self/mountinfo}
    - internal/host/testdata/procsubset-pid/**
  modified:
    - internal/host/host.go
    - internal/host/collector.go
    - internal/host/collector_test.go
    - internal/inventory/hardware.go
    - web/src/api.ts
    - web/src/routes/host.tsx
    - web/src/routes/host.test.tsx
    - web/fixtures/demo.json

key-decisions:
  - "Load falls back to sysinfo(2) on ANY /proc/loadavg failure (not only ENOENT); only when sysinfo also fails does load carry the classified loadavg reason"
  - "The memo also advances on a negative window (a clock that went backwards), so a stepped fake or wall clock cannot freeze the baseline forever"
  - "rates_over_seconds reports the window whenever the baseline is usable, even when /proc/stat is hidden -- plan 06's network rates share the same memo and window"
  - "An unreadable load is drawn as 'Load not readable: {reason}' under the figure, never an empty slot (the UI-SPEC only specified the readable case)"
  - "The hardening notice uses 'is' for a single hidden reading and 'are' for several"

patterns-established:
  - "classify(path, err, subsetPid) is the only way a /proc read error becomes a Reason"
  - "Host-page component tests run every shape through hostSchema.parse before rendering"

requirements-completed: [HMON-01, HMON-06]

coverage:
  - id: D1
    description: "Hardening detected deterministically from the topmost /proc mount's super options; ENOENT without it is read-failed"
    requirement: HMON-06
    verification:
      - kind: unit
        ref: "internal/host/mountinfo_test.go#TestProcSubsetPid; internal/host/proc_test.go#TestClassifyNeedsTheMountAsProof; internal/host/live_test.go#TestMissingProcWithoutProofIsReadFailed"
        status: pass
    human_judgment: false
  - id: D2
    description: "Under ProcSubset=pid CPU usage, per-core and memory carry hardening.proc-subset with no number on the wire; load readable from sysinfo(2)"
    requirement: HMON-06
    verification:
      - kind: unit
        ref: "internal/host/live_test.go#TestHardeningHidesCPUAndMemory, #TestHiddenValuesCarryNoNumber"
        status: pass
      - kind: integration
        ref: "internal/host/realkernel_test.go#TestRealKernelProcSubset (ran, not skipped, on the Pi)"
        status: pass
    human_judgment: false
  - id: D3
    description: "Memory/swap byte-identical to free -b; load identical to /proc/loadavg; CPU rates with first-read no-baseline and a memo that survives a 0.1 s second poll"
    requirement: HMON-01
    verification:
      - kind: unit
        ref: "internal/host/proc_test.go#TestMeminfo, #TestLoadFromSysinfo; internal/host/live_test.go#TestLivePi5FirstRead, #TestRates, #TestCoreCountChange, #TestLoadUnreadable"
        status: pass
    human_judgment: false
  - id: D4
    description: "/host Processor and Memory cards, hardening notice with ProcSubset=all, rates clause; never a 0 for an unread value"
    requirement: HMON-01
    verification:
      - kind: automated_ui
        ref: "web/src/routes/host.test.tsx#the Live section"
        status: pass
      - kind: automated_ui
        ref: "./bin/task test:layout (/host at 390 px and 1280 px, demo fixture in the production shape)"
        status: pass
    human_judgment: false

duration: 25min
completed: 2026-09-28
status: complete
---

# Phase 11 Plan 03: Live CPU, load, memory and swap, and the hardening rule Summary

**/host now shows CPU usage (overall and per core), load, memory and swap every 3 s. Under the production unit's ProcSubset=pid, CPU usage, per-core usage and memory say "Not readable" and point to the one line (`ProcSubset=all`) that brings them back. They are never shown as 0. The hardening is only named when the topmost /proc mount's `subset=pid` super option proves it, and load stays exact from sysinfo(2).**

## Where this ran

All of it ran on the operator's Raspberry Pi 5 (aarch64, Linux 6.18). It used Go 1.26.7 and did not use `-race`, because ThreadSanitizer refuses this kernel's address space. `TestRealKernelProcSubset` **ran and passed** here; it did not skip. In the child's own namespace, where proc was mounted with `subset=pid,hidepid=invisible`, the production Collector (`os.DirFS("/")` plus the real syscalls) reported `hardening.proc-subset` for usage, per-core and memory. It read load from sysinfo(2), for example `4.73 5.54 6.02`.

The production service, its data directory and `/usr/local/bin` were not touched.

## Performance

- **Duration:** about 25 min
- **Started:** 2026-09-28T14:58Z
- **Completed:** 2026-09-28T15:25Z
- **Tasks:** 3
- **Files modified:** 22

## Accomplishments

- `parseMountinfo` and `topmostAt` handle both measured shapes: the production unit's single /proc, and a user namespace with two stacked /proc mounts in either line order. `procSubsetPid` reads only the super options.
- `classify` marks a read as the hardening only when the file is missing (ENOENT) **and** `subset=pid` is in the mount. Every other error is `read-failed` with its text, and a darwin build reports `unsupported`.
- The memory numbers are exactly what `free -b` gave on the measured sample, and they go through `inventory.MemoryView` rather than a copy. The 7.29, 4.90 and 2.98 load values come out of sysinfo(2) through the kernel's own rounding.
- CPU rates use `inventory.CPUPercent`:
  - The first read says "Waiting for a second reading".
  - A poll 0.1 s after the last one neither answers nor replaces the baseline.
  - A 6-minute gap counts as no baseline.
  - A change in core count hides per-core usage only.
- `/host` has:
  - a Live section with Processor and Memory cards
  - a slate hardening notice that lists the hidden readings named in the response ("CPU usage, memory and swap"), with a `ProcSubset=all` code block that scrolls inside itself
  - the sysinfo(2) sentence
  - the "Rates are over the last n s." footer, shown only when there is a baseline

## Task Commits

1. **Task 1: mountinfo and /proc parsers, classification, load from sysinfo, fixtures**: `775b0c8` (feat)
2. **Task 2: Live section in the answer, rate memo, real-kernel test**: `719c203` (feat)
3. **Task 3: Processor and Memory cards, hardening notice, rates clause**: `8ab8447` (feat)

## Fault injections (each seen red, then reverted with `cmp` against a backup)

| # | Injection | Guard | Observed red |
|---|-----------|-------|--------------|
| T1a | `topmostAt` returns `at[0]` (the first /proc) | TestProcSubsetPid | `unshare:_fresh_proc_stacked_on_the_inherited_one: procSubsetPid = false, want true (topmost /proc = {ID:363 … SuperOpts:[rw]})` and `only_the_lower_of_two_carries_it: procSubsetPid = true, want false` |
| T1a' | `topmostAt` returns `at[len-1]` (the last /proc) | TestProcSubsetPid | `unshare,_lines_in_reverse_order: procSubsetPid = false, want true (topmost /proc = {ID:363 … SuperOpts:[rw]})` |
| T1b | `classify` without `&& subsetPid` | TestClassifyNeedsTheMountAsProof | `absent_without_subset=pid: code = "hardening.proc-subset", want "read-failed"` |
| T2a | hidden-/proc/stat branch returns `Read(0.0)` for usage | TestHardeningHidesCPUAndMemory, TestHiddenValuesCarryNoNumber | `round 0 usage: readable=true, value present=true; want not readable and no value` and `live.cpu.usage.value: a number (0) under a reading that was not read` |
| T2b | memo advanced on every call | TestRates | `3 s after read 2: usage = map[readable:true value:0], want readable 25`, `rates_over_seconds = 2.9, want 3` |
| T2c | a readable zero sent without its value key (see Deviations) | TestRates | `idle: usage = map[readable:true], want readable 0` |
| T2d | `procSubsetPid` always false | TestRealKernelProcSubset (child) | `usage: reason = &{Code:read-failed Message:Could not read /proc/stat: open proc/stat: no such file or directory}, want code hardening.proc-subset` |
| T3 | `formatPercent(0)` in place of `MissingValue` for unreadable usage | host.test.tsx | the ProcSubset=pid case fails with `Expected element to have text content: Not readable / Received: Processor4 cores0.0%load 0.52 · 0.41 · 0.33`, and so do the no-baseline and read-failed cases |
| T3' | `formatPercent(0)` rendered **beside** `MissingValue` | host.test.tsx (`expectNoDrawnZero`) | `Expected [] / Received ["0.0%"]` in the ProcSubset=pid and no-baseline cases |

## Verification

- `go test ./internal/host ./internal/inventory ./internal/httpapi -count=1` passed with exit 0. inventory took 100 s and httpapi 265 s because other work was loading the Pi at the same time.
- The plan's Task 1 and Task 2 verify commands passed with exit 0, and every named test showed `--- PASS`.
- `GOOS=darwin GOARCH=arm64 go build ./cmd/holzkube-managerd` passed with exit 0. The first attempt failed only because `test:layout` was rebuilding `internal/httpapi/dist` at the same moment; the rerun passed.
- `golangci-lint run ./internal/host/... ./internal/inventory/...` found 0 issues, and `./bin/task lint:go` (the pinned v2.13.1, whole repository) passed with exit 0 and 0 issues.
- `npm --prefix web run test`: 53 files and 500 tests passed. `typecheck` passed with exit 0. `lint` also passed with exit 0; its 2 warnings and 1 info are pre-existing and sit in `DataTable.tsx` and `wall.test.tsx`.
- `./bin/task test:layout` passed with exit 0, including `ok 390px /host (3 controls, 3 items)` and `ok 1280px /host`.
- `go test ./internal/publicrepo/` passed with exit 0.

## Deviations from Plan

**1. [Rule 3 - Blocking] `Load` type and the inventory rename landed in Task 1, not Task 2**
- `parseLoadavg` returns `Load`, and `TestMeminfo` runs "through inventory's formula", so Task 1 could not compile or test without both.
- The bodies are unchanged. `MemoryView` gained a doc comment, which it lacked before.

**2. [Rule 1 - Test honesty] Fault T2c is simulated rather than done as the literal "non-pointer `omitzero`" edit**
- Changing `Value *T` to `Value T` breaks compilation of every test that dereferences `r.Value`, so the package would never have reached `TestRates`.
- The injection instead made `Read` return `Reading{Readable: true}` without a value whenever `v` is the zero value, which is exactly what `value,omitzero` sends on the wire.
- The plan expected this to show up as a lost per-core 0. It cannot: `omitzero` never drops a non-nil slice's elements. To catch it, `TestRates` gained an idle step (step 5), where overall usage is a real 0; that is the case that went red.

**3. [Rule 1 - Plan text] Fault T1a goes red on the in-order stacked shape, not on the reversed one**
- With the lines in reverse order, the first /proc entry *is* the topmost, so "return the first" still passes there.
- It fails on the in-order shape and on "only the lower carries it". The mirror fault (return the last entry) fails on the reversed shape. Both were run and recorded above.

**4. [Rule 1 - Bug in the test itself] The per-card zero check compares element by element**
- While injecting T3', `card.textContent` read `…cores0.0%…`. The regex needs whitespace before the 0, so a check over the whole card's text passed while the zero was on screen.
- `expectNoDrawnZero` checks each element instead, and that version went red.

**5. [Rule 2] Unreadable load is spelled out**
- The UI-SPEC did not say what the Processor card shows when load itself cannot be read (no loadavg and no sysinfo).
- It now shows "Load not readable: {reason}" rather than leaving the slot empty.

## Next Phase Readiness

- Plan 04 (filesystems) can use `parseMountinfo` and `topmostAt`, and the `procsubset-pid` mountinfo already contains the data-dir bind mount on `179:2`.
- Plan 06 (network) should add its per-link counters to `counters` and use `Live.RatesOverSeconds`, which already reports the window while CPU is hidden.

## Self-Check: PASSED

- The 12 new source, test and fixture files exist on disk, and the procsubset-pid tree has no proc/stat, proc/meminfo or proc/loadavg.
- `git log` has commits 775b0c8, 719c203 and 8ab8447.
