---
phase: 12-host-verlauf
fixed_at: 2026-09-29T01:25:00Z
review_path: .planning/phases/12-host-verlauf/12-REVIEW.md
iteration: 1
findings_in_scope: 8
fixed: 7
skipped: 1
status: partial
---

# Phase 12: Code Review Fix Report

**Fixed at:** 2026-09-29T01:25:00Z
**Source review:** .planning/phases/12-host-verlauf/12-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 8 (CR-01, WR-01..WR-04, and the Info items asked for: IN-01, IN-05, IN-07)
- Fixed: 7
- Skipped: 1 (IN-01)

**Where verification ran:** in the main checkout (`workflow.use_worktrees` is
false, so no worktree was used), on the operator's Pi 5 (aarch64), with
Go 1.26.7 and without `-race`. Every fix got a regression test, and each test
was seen failing against the reinstated fault before it passed. IN-07 is a
build script with no test harness, so its fault was measured in a scratch copy
instead. The closing gates were:
`go test ./internal/host/... ./internal/history/... ./internal/httpapi/... ./internal/inventory/ ./internal/publicrepo/`
(exit 0), `./bin/task lint:go` (0 issues), `npm --prefix web run typecheck`
(exit 0) and `npm --prefix web run test`. The web suite passed all 594 tests
after WR-04. The final full run had one failure,
`login.test.tsx > links to the address the local account works on`. That test
passes alone (4/4) and in the earlier full run, and its page was not touched.
It is a flake under load, not a regression.

## Fixed Issues

### CR-01: Temperature sensors that exist but cannot be read make the host "Healthy"

**Files modified:** `internal/host/sensors.go`, `internal/host/health.go`, `internal/host/sensors_test.go`, `internal/host/health_test.go`, `docs/api-contract.md`, `docs/guide.md`
**Commit:** 901f479
**Status:** fixed: requires human verification (logic)
**Applied fix:** `Sensors` now has `Unread []string` (`json:"-"`, so it is not
sent). It names each temperature that exists but was not read:
- an input that failed or did not parse (as "chip label");
- a chip whose directory could not be listed (as the chip name);
- a fallback zone whose `temp` failed.

`Assess` adds "Temperature <name> could not be read." for each one. It sets
`noSensors` only when there are no temperatures and nothing is unread.

The tests went red against two faults: `Assess` ignoring `Unread`, and
`readChip` dropping failures as before. `TestSensorsThatFailAreNotNone`
includes the review's probe as one of its cases.

Two things to confirm:
- A sensor input that fails permanently (for example a disk-temperature input
  that errors while the disk sleeps) now keeps the host "Not readable" for as
  long as it fails. That is what D-12 asks for, but it pulls against D-11's
  "a grey that never goes away".
- All inputs on this Pi read cleanly (cpu_thermal, rp1_adc, thermal_zone0), so
  it is not affected.

### WR-01: A filesystem with zero capacity is rated "below 80%"

**Files modified:** `internal/host/health.go`, `internal/host/health_test.go`, `docs/api-contract.md`
**Commit:** 78f4166
**Status:** fixed: requires human verification (logic)
**Applied fix:** A row whose used + available is 0, or whose sum wrapped, is now
listed in `Assess` as "Usage of <mount> reports no capacity." (unknown, not
ok). `filesystemFinding` keeps its own guard so `bits.Div64` cannot panic. The
new case went red for both `{0,0}` and a wrapped sum.

### WR-02: The wall's host reason can show a filesystem mount path

**Files modified:** `internal/host/health.go`, `internal/httpapi/handlers/wall_host.go`, `internal/httpapi/handlers/wall_host_test.go`, `docs/api-contract.md`, `docs/guide.md`
**Commit:** f49b6c1
**Status:** fixed
**Applied fix:**
- `Assess` now also builds `Health.Public` (`json:"-"`): the same warnings in
  the same order, but a filesystem is named by its role, "/" or "data
  directory".
- `wallHostFrom` reads only `Public`. When `Public` is empty it falls back to
  the path-free summary, never to `Warnings`.
- `/host` keeps the full sentence.

The tests use a data directory on `/mnt/ssd`, and one whose path stands in for
the mount, both through `Assess`. They went red with the wall reading
`Warnings` again.

### WR-03: The sampler's host read has no deadline

**Files modified:** `internal/history/sampler.go`, `internal/history/host_test.go`, `internal/host/sample.go`, `internal/host/sample_test.go`, `internal/host/collector.go`, `docs/api-contract.md`
**Commit:** 82aa866
**Status:** fixed
**Applied fix:**
- `sampleHost` runs the read in a goroutine and waits at most `HostBudget`
  (5 s; overridable in tests through `SamplerDeps.HostBudget`). If the read
  does not finish, it records a gap and the pass goes on.
- `Collector.Sample` refuses to start a second read while one is stuck
  (`ErrSampleBusy`, an atomic flag), so there is at most one hung goroutine.

Two faults went red:
- The read inline again: neither the pass nor `Run` returned after cancel.
- The guard removed: the second `Sample` hung beside the first.

### WR-04: Short gaps are drawn as a line across them

**Files modified:** `web/src/hooks/useChartRange.ts`, `web/src/hooks/useLiveSeries.ts`, `web/src/components/charts/LiveChart.tsx`, `web/src/components/charts/Sparkline.tsx`, `web/src/components/charts/Sensors.tsx`, `web/src/hooks/useChartRange.test.ts`, `web/src/components/charts/Sensors.test.tsx`, `web/src/routes/host.test.tsx`, `docs/guide.md`
**Commit:** 8cff078
**Status:** fixed: requires human verification (logic)
**Applied fix:**
- `merge` now knows the history's `step_seconds`. It marks a recorded point
  with `gap: true` when it comes more than 1.5 steps after the previous point.
- `segments` starts a new run at every marked point. It keeps the old
  45-s / 3x tolerance only for the page's own 3-s readings, which have no
  fixed slots.
- `Sparkline` now breaks through `segments` too. This applies to both the node
  page and `/host`.
- `Sensors`' `history` prop is now typed as `Series`.
- The `append` comment and the guide now say what the live tail tolerates.

Three faults were each seen red: `merge` not marking, `segments` ignoring the
mark, and `Sparkline` drawing a single path. `web/src/hooks/useChartRange.test.ts`
(where most of the new cases are added) also covers the node page's merge.

### IN-05: The warning notice's explanation leaves out trip points

**Files modified:** `web/src/routes/host.tsx`, `web/src/routes/host.test.tsx`, `.planning/phases/12-host-verlauf/12-UI-SPEC.md`
**Commit:** 41350bd
**Status:** fixed
**Applied fix:** The notice now reads "...its chip's own limit, or at its
thermal zone's trip point, or at a default for its kind (80 °C for a processor)
when neither names one...". The test's sentence was changed first and went red
against the old UI. The README host screenshot was not re-rendered. If the
notice footer shows in it, it still has the old sentence until
`task build && node web/scripts/readme-images.mjs` is run.

### IN-07: readme-images waits the full 10 s if the daemon has already exited

**Files modified:** `web/scripts/readme-images.mjs`
**Commit:** 0991ab8
**Status:** fixed
**Applied fix:** It now uses the same `exitCode`/`signalCode` guard as
`layout-audit.mjs`, and unrefs the bound's timer so the timer does not hold the
process open. No tracked test covers a build script, so the fault was measured
in a scratch copy of the pattern with a child that had already exited:
- Without the guard, the race settled only at its bound (3001 ms), and the
  process took 3347 ms.
- With the guard, it settled in 1 ms, and the process took 344 ms.

## Skipped Issues

### IN-01: The wall's staleness limit is hard-coded, and `now` is taken before the cluster call

**File:** `internal/httpapi/handlers/wall_host.go:60`, `internal/httpapi/handlers/kubernetes.go:519-527`
**Reason:** It is not small once the project's method is applied. Moving
`time.Now()` after `ForTheWall` is a one-line change. But proving it red needs
a cluster call that takes measurable time: `kubesim` has no latency option and
`httpapi.Deps` has no clock. Passing the sampler interval into the handler is
more wiring on top of that. The fix was deferred rather than committed
untested.
**Original issue:** The limit `3*history.FineStep` does not follow
`SamplerDeps.Interval`. `now` is taken before `ForTheWall`, which can use up
its whole budget, so the tile's age is understated by that time.

---

_Fixed: 2026-09-29T01:25:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
