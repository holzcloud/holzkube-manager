---
phase: 12-host-verlauf
reviewed: 2026-09-28T22:46:26Z
depth: standard
files_reviewed: 52
files_reviewed_list:
  - README.md
  - cmd/holzkube-managerd/budget_test.go
  - cmd/holzkube-managerd/main.go
  - docs/api-contract.md
  - docs/guide.md
  - internal/history/history.go
  - internal/history/host_test.go
  - internal/history/sampler.go
  - internal/host/collector.go
  - internal/host/health.go
  - internal/host/health_test.go
  - internal/host/host.go
  - internal/host/sample.go
  - internal/host/sample_test.go
  - internal/host/sensors.go
  - internal/host/sensors_test.go
  - internal/httpapi/handlers/history.go
  - internal/httpapi/handlers/kubernetes.go
  - internal/httpapi/handlers/wall_host.go
  - internal/httpapi/handlers/wall_host_test.go
  - internal/httpapi/hardwareapi_test.go
  - internal/httpapi/historyapi_test.go
  - internal/httpapi/hostapi_test.go
  - internal/httpapi/kubernetesapi_test.go
  - internal/httpapi/wallhostapi_test.go
  - internal/httpapi/walllinkapi_test.go
  - internal/inventory/hardware.go
  - internal/inventory/hardware_test.go
  - internal/inventory/limits.go
  - internal/inventory/limits_test.go
  - web/fixtures/demo.json
  - web/scripts/layout-audit.mjs
  - web/scripts/readme-images.mjs
  - web/src/api.ts
  - web/src/components/HostState.tsx
  - web/src/components/NodeHardware.test.tsx
  - web/src/components/NodeHardware.tsx
  - web/src/components/Sidebar.test.tsx
  - web/src/components/Sidebar.tsx
  - web/src/components/charts/HardwareCharts.tsx
  - web/src/components/charts/Sensors.test.tsx
  - web/src/components/charts/Sensors.tsx
  - web/src/fixtures.test.ts
  - web/src/hooks/useLiveSeries.test.ts
  - web/src/hooks/useLiveSeries.ts
  - web/src/index.css
  - web/src/routes/host.browser.test.tsx
  - web/src/routes/host.test.tsx
  - web/src/routes/host.tsx
  - web/src/routes/wall.test.tsx
  - web/src/routes/wall.tsx
  - web/src/typography.browser.test.tsx
findings:
  critical: 1
  warning: 4
  info: 7
  total: 12
status: issues_found
---

# Phase 12: Code Review Report

**Reviewed:** 2026-09-28T22:46:26Z
**Depth:** standard
**Files Reviewed:** 52
**Status:** issues_found

## Summary

I reviewed the host history (the sampler hook, `host.Collector.Sample`, the separate
rate baseline, persistence), `host.Assess` and its thresholds, the move of the
temperature-limit rule from the browser to Go (including trip-point parsing), the
wall's `host` field, the sidebar's react-query polling, and the new schemas.

These parts are sound: the sampler's and the page's rate baselines are really
separate and both are touched only under `c.mu`. The host subject survives
`Retain` and the file reload. The filesystem check is done in integers with `>=`
over df's denominator (used + available), and `dfPercent` cannot panic. The Go
`TemperatureLimits` matches the former browser `temperatureLimits` case for case.
The trip-point walk is bounded (at most 16 trips, `readTrimmed` caps each file,
values at or below zero are dropped). An unknown host on the wall always shows
the fixed "not readable". Both `health.state` and `warn_c`/`danger_c` are required
in the schema.

The main defect is that the D-12 guard ("a host whose rated values cannot be read
is never ok") has a hole. Temperature inputs that exist but fail to read are
dropped without a trace, so the host is reported **healthy** as "a machine with
no temperature sensors". I reproduced this with an overlay probe test against the
real `readSensors` + `Assess` (nothing in the tree was changed). A zero-capacity
filesystem has the same shape. There are also three other warnings:
- The wall reason can show a mount path to a wall link.
- The sampler now makes synchronous local reads that have no deadline.
- The charts join short gaps with a line, although the docs promise a gap.

## Critical Issues

### CR-01: Temperature sensors that exist but cannot be read make the host "Healthy" (D-12 broken)

**File:** `internal/host/sensors.go:275-279`, `internal/host/sensors.go:263-266`, `internal/host/sensors.go:94-99`, `internal/host/sensors.go:120-123`, `internal/host/health.go:84-85,129-132`

**Issue:** `readChip` drops, without a trace:
- any `temp*_input` that fails to read or parse (`if !ok { continue }`);
- a whole chip whose directory cannot be listed (`return nil, nil, name`).

`readSensors` then leaves out a chip with no temps and no fans. In the zone
fallback it also skips a zone whose `temp` fails. If every input failed, the
result is `Sensors{Temperatures: []}`, marked **readable**. `Assess` treats that
as `noSensors` and returns `HealthOK` with "This machine reports no temperature
sensors, so it is judged by its filesystems alone."

I confirmed this with a probe test run through `go test -overlay`. The input was
hwmon0 `cpu_thermal` with an empty `temp1_input` and zone0 `cpu-thermal` with a
garbage `temp`. The result was `temps=0 state=ok summary="Filesystems are below
80%. This machine reports no temperature sensors, ..."`.

This is the exact case D-12 forbids. The sensor the host is rated on could not
be read, and the host shows green in the navigation, on the page and on the wall
tile. It also flaps: a Pi whose `cpu_thermal` read fails on one poll goes from
"Warning" (hot) to "Healthy" on that poll.

`TestUnreadableIsNeverHealthy` does not catch it. It only covers readable-with-a-
sensor versus the whole `Sensors` reading being hidden. It never covers readable
but empty because the reads failed.

**Fix:** Keep "no sensor exists" apart from "a sensor exists but could not be
read". For example, count the inputs that were attempted and return the failures:
```go
type Sensors struct {
    Temperatures []inventory.HardwareTemperature `json:"temperatures"`
    Fans         []inventory.HardwareFan         `json:"fans"`
    // Unread names every temperature input that exists but could not be read.
    Unread []string `json:"unread"`
}
// readChip: on !ok -> unread = append(unread, name+" "+label)
// a chip whose ReadDir fails -> unread = append(unread, chip.dir)
```
Then in `Assess`, add each `Unread` entry to `unreadable`. Set `noSensors` only
when there are no temperatures **and** `Unread` is empty. Add the case
"readable, all inputs failed" to the table test and watch it go red against the
current code.

## Warnings

### WR-01: A filesystem with zero capacity is rated as neither read nor crossed, so the host is "ok" with a false sentence

**File:** `internal/host/health.go:183-188`, `internal/host/health.go:94-102,129-135`

**Issue:** `filesystemFinding` returns `(finding{}, false)` when
`used + available == 0` or the sum wrapped. `Assess` then counts the row as read
and below its line. It never adds the row to `unreadable`. The probe test with
`FSUsage{0,0,0}` on `/` returned `state=ok` and "Filesystems are below 80%". A
statfs that reports zero blocks gives no usage figure at all: some FUSE
filesystems do this, and so does a pseudo-fs bind-mounted over the data
directory. Calling that "below 80%" is the green that D-12 rules out.

**Fix:** Report it as unreadable instead of passing it over:
```go
if total == 0 || total < u.UsedBytes {
    unreadable = append(unreadable, "Usage of "+row.Mount+" reports no capacity.")
    continue
}
```
(Move the check from `filesystemFinding` into the loop in `Assess`, or return a
third state from it.)

### WR-02: The wall's host reason can show a filesystem mount path to wall links

**File:** `internal/httpapi/handlers/wall_host.go:69-79`, `internal/host/health.go:99,197`, `internal/host/filesystems.go:67-69,88`

**Issue:** For `warn`, the tile's reason is `s.Health.Warnings[0]`. A filesystem
warning is built as `row.Mount + " 91% used ≥ 80%"`. For the data directory's
row, `Mount` is:
- the mount point of the data directory's own filesystem (for example
  `/mnt/ssd`); or
- when mountinfo cannot be read, the resolved data-directory path itself, which
  after a symlink can be a home directory.

The wall answer is public to anyone with a wall link. `wall_host.go`'s own
disclosure comment leaves out the Unreadable sentences *because they name paths*,
and `docs/guide.md` says "The wall shows the name and state only". The
Warnings list names paths just the same.

**Fix:** Build the wall's short reason from something that is not a path. For
example, `finding` could carry a `public` sentence that uses the row's role
("data directory 91% used ≥ 80%", "/ 91% used ≥ 80%" only for the root). Store
it in `Health` next to `Warnings` (or in the Snapshot), have `wallHostFrom` use
it, and add a wall test with a data-dir mount such as `/mnt/ssd` that asserts
the path is not in the answer.

### WR-03: The sampler now makes synchronous local reads with no deadline, so a hung statfs stops all history and shutdown

**File:** `internal/history/sampler.go:161-165,226-236`, `internal/host/sample.go:21-37`, `internal/host/collector.go:257`

**Issue:** `Sample` checks `ctx.Err()` once and then does, inline and with no
deadline:
- statfs of `/` and of the data directory;
- the sysfs walks (hwmon, thermal zones with trip points, net);
- `uname`.

It does this at the start of **every** sampler pass, before the inventory.
Before Phase 12 the sampler never touched local filesystems; only a page request
could block on them. Now a data directory on a hard-mounted NFS or SMB share
that stops answering, or a USB SSD that is failing, blocks `pass()` forever.
Every node's and cluster's history then stops, not only the host's.

Because `Run` never comes back to its `select`, the shutdown path also hangs:
`main.go` waits on `<-sampled` in a defer. The final `Flush` is never reached, so
the minute before an update restart is lost, and the process does not exit until
systemd kills it. The design note in `sampler.go` ("a store that cannot list the
machines must not cost the host its history") guards one direction only.

**Fix:** Bound the host read and keep it from holding up the pass. For example,
run it in a goroutine that has a deadline (such as `HardwareBudget`), give up on
it after that, and record nothing (a gap):
```go
func (s *Sampler) sampleHost(ctx context.Context, at time.Time) {
    ctx, cancel := context.WithTimeout(ctx, hostBudget)
    defer cancel()
    type res struct{ v map[string]float64; err error }
    ch := make(chan res, 1)
    go func() { v, err := s.deps.Host(ctx); ch <- res{v, err} }()
    select {
    case r := <-ch: /* record / failed as now */
    case <-ctx.Done(): s.failed(HostSubject(), "the host read did not finish", ctx.Err())
    }
}
```
(`Collector.Sample` would also need to guard against an earlier read that is
still running, so that goroutines do not pile up.)

### WR-04: Short gaps are drawn as a line across them, which contradicts the phase's "gap, never a line" claims

**File:** `web/src/components/charts/LiveChart.tsx:296-311` (used by `web/src/components/charts/HardwareCharts.tsx`), `web/src/hooks/useLiveSeries.ts:53-62`, `docs/guide.md` (host history paragraph), `README.md` (host screenshot caption)

**Issue:** `segments()` breaks a curve only when two points are more than
`max(45 s, 3 × median step)` apart. So:
- On 1 h / Live, up to two missing 15-s samples, or up to 14 missing 3-s live
  readings, are bridged with a straight line.
- On 24 h (60-s step), up to two missing minutes are bridged.

The guide says "the minutes the service was not running are a gap in them" and
"A value that could not be read at a sample is a gap too". The new comment in
`append()` says "the chart shows the gap because no point exists for that time".
The hourly update restart takes seconds and a single failed sensor read is one
slot, so neither ever appears as a gap. They appear as an interpolated line,
which is the invented reading HMON-06 forbids. (The phase's own README picture
uses a 40-minute gap, so it cannot show this.)

**Fix:** Give `segments` the known step (the history's `step_seconds`, and 3 s
for the live tail) and break at, for example, `1.5 × step`, rather than at a
45-s floor. Or have `merge` insert an explicit break where a known slot has no
point. At least correct the guide sentence and the `append` comment so they
state the 45-s / 3-step tolerance.

## Info

### IN-01: The wall's staleness limit is a hard-coded constant, measured from a `now` taken before the cluster call

**File:** `internal/httpapi/handlers/wall_host.go:60`, `internal/httpapi/handlers/kubernetes.go:519-527`

**Issue:** The limit `3*history.FineStep` does not follow `SamplerDeps.Interval`.
`now` is taken before `ForTheWall`, which can use up its whole budget, so the
tile's age is understated by that time. A pass that runs longer than 45 s (many
unreachable nodes at `HardwareBudget` 10 s per round of 8) turns a healthy host
grey.

**Fix:** Give the collector (or Deps) the sampler interval. Take `time.Now()`
right after `ForTheWall` for the host tile.

### IN-02: Twin limits are keyed by chip name, and the first zone wins

**File:** `internal/host/sensors.go:81-87`

**Issue:** Several zones of the same type (for example two `acpitz` zones with
different trips) collapse into one entry. Every hwmon chip with that name gets
zone 0's trips.

**Fix:** Match twins by device link, or at least leave a comment that this is
ambiguous and pick the lowest trips.

### IN-03: A filesystem at 90% or more is red on the meter but never "critical" in the server's order

**File:** `web/src/routes/host.tsx:450-451`, `internal/host/health.go:196-199`

**Issue:** The meter uses `danger={ceiling*0.9}`. `filesystemFinding` never sets
`critical`. A 97%-full data disk is red on the page and has no ", critical"
suffix, and it sorts after any critical temperature. So the page's colours and
the server's "worst first" disagree.

**Fix:** Mark filesystems at 90% or more as critical in Go (with the same integer
comparison), or drop the red band from the meter.

### IN-04: Sensor series keys collide for identical chip/label pairs

**File:** `internal/host/sample.go:112-117`, `web/src/routes/host.tsx:938-939`, `web/src/components/charts/Sensors.tsx` (`sensorKey`)

**Issue:** Two drives that both report `nvme/Composite` (or two fans with the
same label) overwrite each other in the history map. They also give duplicate
React keys. The node path has the same problem; the host now inherits it.

**Fix:** Make the chip part unique, for example `nvme#1`, or use the hwmon index,
in both Go and TS.

### IN-05: The warning notice's explanation leaves out trip points

**File:** `web/src/routes/host.tsx:251-254`

**Issue:** The notice says "at its chip's own limit, or at a default for its
kind". The server also uses the thermal zone's passive/hot and critical trips
(D-07), and the guide says so.

**Fix:** Add "or its thermal zone's trip points".

### IN-06: A second tab outside /host still shortens /host's rate window

**File:** `web/src/components/Sidebar.tsx:189-195`, `internal/host/collector.go:131`

**Issue:** The sidebar skips its own poll only in the tab that is on `/host`.
Any other open tab still calls `GET /api/v1/host` every 30 s. That advances the
shared page baseline `c.prev`, which is the interference the comment says it
avoids.

**Fix:** Say this limit in the comment. Or serve the sidebar from a light
`health` read that does not advance `c.prev` (for example the sampler's `Latest`
snapshot, as the wall does).

### IN-07: readme-images waits the full 10 s if the daemon has already exited

**File:** `web/scripts/readme-images.mjs:306-309`

**Issue:** `daemon.once('exit')` is attached after the daemon may already have
exited. `layout-audit.mjs` checks `exitCode`/`signalCode` first; this script
does not.

**Fix:** Use the same guard as `layout-audit.mjs:687-690`.

---

_Reviewed: 2026-09-28T22:46:26Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
