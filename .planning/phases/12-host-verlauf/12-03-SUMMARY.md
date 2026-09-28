---
phase: 12-host-verlauf
plan: 03
subsystem: api
tags: [go, history, sampler, rates, health, wall, wall-link]
status: complete

requires:
  - phase: 12-host-verlauf
    provides: "12-01: Collector.Sample, HistoryValues (temp/fan), host/local in the one sampler; 12-02: host.Assess, Health, HealthState"
provides:
  - "Collector baseline slots: prev (page) and samplePrev (sampler); readLive(base), rates(base, ...)"
  - "HistoryValues complete: cpu, core:<n>, memory, rx/tx (physical only, every link rated), temp:, fan:"
  - "View.Health = Assess(Live) on GET /api/v1/host, both paths"
  - "host.Snapshot {At, Name, Health} and Collector.Latest(); Sample stores it"
  - "handlers.wallHost / hostForTheWall / wallHostFrom; named wallAnswer {Wall, Trends, Host} via newWallAnswer"
  - "adoptedClusterWithAPI(t, opts, extra ...harnessOpt)"
affects: [12-04, 12-05, 12-06, 12-07, 12-08]

actuals:
  tokens: 10263
  tasks: 3
  commits: 3
plan_head_before: 63678a9c111d8f3e58094ec46d6ea4f4f7fcf519
plan_head_after: 7dcfe1207929ec0bc6d5e665a8c1aa55bc06621b

tech-stack:
  added: []
  patterns:
    - "One rate memo per reader: the page and the sampler each advance their own baseline slot, both under c.mu"
    - "The wall reads the sampler's snapshot, never the host; staleness (> 3 x FineStep) is decided in the wall's reader"
    - "What a wall link may see is a struct in handlers (wallHost): name, state, a short reason"
    - "overlayFS in host tests: a fixture with chosen files replaced, symlinks passed through"

key-files:
  created:
    - internal/httpapi/handlers/wall_host.go
    - internal/httpapi/handlers/wall_host_test.go
    - internal/httpapi/wallhostapi_test.go
  modified:
    - internal/host/collector.go
    - internal/host/sample.go
    - internal/host/sample_test.go
    - internal/host/host.go
    - internal/history/host_test.go
    - internal/httpapi/hostapi_test.go
    - internal/httpapi/handlers/kubernetes.go
    - internal/httpapi/kubernetesapi_test.go
    - internal/httpapi/walllinkapi_test.go

key-decisions:
  - "rx/tx are written only when the round has a rate window, there is at least one physical interface, and every physical interface has both rates; a host without a physical interface (a container) records no rx/tx rather than a permanent 0"
  - "Sample on an unsupported platform still leaves a snapshot (Assess of the all-unsupported Live, so unknown): the wall of a darwin build shows the host grey, not missing"
  - "A page read never creates a snapshot: before the sampler's first pass the wall has no host tile (host: null)"
  - "Exactly 3 x FineStep old is still current; older is unknown / 'not readable'"

patterns-established:
  - "A test that compares an answer against the input it was built from must not share a map with it: a fault that mutates the input changes both sides (TestTheWallsHost's summary check)"

requirements-completed: [HMON-05, HOST-04, HMON-07]

coverage:
  - id: D1
    description: "HistoryValues writes a node's keys, nothing for an unread value, rx/tx over physical links only and only when every one is rated"
    requirement: HMON-05
    verification:
      - kind: unit
        ref: "internal/host/sample_test.go#TestHistoryValues"
        status: pass
  - id: D2
    description: "The page's rates stay over 3 s and the sampler's over 15 s when both read the same collector"
    requirement: HMON-05
    verification:
      - kind: unit
        ref: "internal/host/sample_test.go#TestSampleHasItsOwnBaseline"
        status: pass
  - id: D3
    description: "A host history with an unreadable-CPU stretch survives a restart; the stretch is a gap (no point, no 0) in 1 h, 6 h and 24 h, the temperature runs through it"
    requirement: HMON-05
    verification:
      - kind: unit
        ref: "internal/history/host_test.go#TestTheHostHistorySurvivesARestartWithItsGap"
        status: pass
  - id: D4
    description: "GET /api/v1/host carries health {state, summary, warnings, unreadable}, lists never null, from the same reading"
    requirement: HOST-04
    verification:
      - kind: unit
        ref: "internal/host/sample_test.go#TestReadCarriesHealth, #TestLatest"
        status: pass
      - kind: integration
        ref: "internal/httpapi/hostapi_test.go#TestHostAPI"
        status: pass
  - id: D5
    description: "The wall's answer carries host {name, state, reason}; stale is unknown; the host is not among the nodes; a wall link sees the same host and cannot open the host routes"
    requirement: HOST-04
    verification:
      - kind: unit
        ref: "internal/httpapi/handlers/wall_host_test.go#TestTheWallsHost"
        status: pass
      - kind: integration
        ref: "internal/httpapi/wallhostapi_test.go#TestTheWallCarriesTheHost, #TestTheWallDoesNotReadTheHost; internal/httpapi/walllinkapi_test.go#TestAWallLinkOpensTheWallAndNothingElse"
        status: pass

duration: ~18 min
completed: 2026-09-28
---

# Phase 12 Plan 03: Host values, own rate window, state on the host route and the wall Summary

**The host now records the same values a node does, over its own 15-second window, and never writes a 0 for a value it could not read. `GET /api/v1/host` includes the host's state. The wall's single answer includes the host as its own tile, built from the sampler's last snapshot. That tile turns grey when the snapshot is more than 45 s old, it is not counted among the cluster's nodes, and a wall link sees the same tile but still cannot open the host routes.**

## Where this ran

Everything ran on the operator's Raspberry Pi 5 (aarch64), with Go 1.26.7 via `GOTOOLCHAIN`. There was no `-race` run, because ThreadSanitizer does not work on the Pi 5 kernel (the race detector only runs in CI). The production service, its data directory and `/usr/local/bin` were not touched. Nothing was pushed, released or tagged.

## Accomplishments

- **Baseline slots.** `rates` and `readLive` now take the baseline slot they read and advance. `Read` passes `&c.prev`, so the page behaves as before. `Sample` passes `&c.samplePrev` through `sampleLive`. `Sample` never calls `Read`, `readService` or `readDevice`.
- **`HistoryValues` is complete.** It writes `cpu`, `core:<i>`, `memory` (100·used/total), `rx`/`tx`, `temp:` and `fan:`. There is no `read`/`write`, because the host has no disk-throughput reading. A value that could not be read writes no key.
- **`View.Health`.** `Read` sets `Assess(v.Live)` on both the Linux and the unsupported path. The Live section of the unsupported path is now `unsupportedLive()`, which `sampleLive` also uses.
- **`Snapshot` and `Latest()`.** `Sample` stores `{At: the collector's clock, Name: uname's nodename or "", Health: Assess(live)}` under `c.mu`.
- **Package doc.** It no longer says "the systemd unit this product ships". ProcSubset=pid is now described as the reference installation's hardening (11-07 deviation 3).
- **`wall_host.go`.** `wallHost{name, state, reason}` comes from `hostForTheWall`/`wallHostFrom`. The staleness rule is `now.Sub(s.At) > 3*history.FineStep`. The tile name falls back to "holzkube-manager host". An unknown state always shows "not readable".
- **`wallAnswer`.** It is now a named type, built by `newWallAnswer`, so the "host appended to the nodes" fault could be tested. `Nodes` and `Summary` pass through unchanged.

## Task Commits

1. **Task 1: every host value, the sampler's own window, the restart with its gap:** `31dd4aa` (feat)
2. **Task 2: health on GET /api/v1/host, Snapshot and Latest:** `5fbbbb0` (feat)
3. **Task 3: the host on the wall's answer:** `7dcfe12` (feat)

## Fault injections (each seen red, then restored)

Before each fault, the production file was copied to the scratchpad. It was copied back afterwards, and `cmp` confirmed the result before the next step. Exit codes were read from the `go test` command itself, with its output written to a file.

| # | Fault | Test | Observed (exit 1 each) |
|---|-------|------|------------------------|
| 1 | `HistoryValues` writes `cpu = 0` when the usage is hidden | TestHistoryValues | `cpu = 0 under ProcSubset=pid; want no key, never a 0`; `first Sample = map[cpu:0]; want nothing`; key sets with `cpu` added |
| 2 | virtual links' rates added to rx/tx | TestHistoryValues | Pi 5: `rx = 1.5101e+07, want 101000`, `tx = 1.50101e+07, want 10100`; veth case: `rx = 3.01e+07, want 100000` |
| 3 | `sampleLive` reads against `&c.prev` (shared baseline) | TestSampleHasItsOwnBaseline | `sampler's first read at 4.5 s: rates over 1.5, want null`; `page at 6 s: rates over 1.5, want 3`; `sampler at 19.5 s: rates over 13.5, want 15` |
| 4 | `encodeLocked` skips `host/local` | TestTheHostHistorySurvivesARestartWithItsGap | `1h/6h/24h after a restart differs from before it`; `no cpu series at all after the restart`; `0 temperature points inside the stretch, want at least 19` |
| 5 | `Read` leaves `Health` zero (both paths) | TestReadCarriesHealth, TestHostAPI | `state = "", want unknown`; `health = {State: …}, want Assess of the same reading`; wire: `health.warnings/unreadable = null, want a list`; HTTP: `health.state = "", want ok, warn or unknown` and `health.warnings = null, want an array` |
| 6 | staleness check skipped (`if false && …`) | TestTheWallsHost | `wallHostFrom = &{… State:ok Reason:healthy}, want {… State:unknown Reason:not readable}` and the same for the stale warning |
| 7 | `newWallAnswer` appends the host to `Wall.Nodes` and counts it in `Summary` | TestTheWallsHost | `nodes = [cp-1, worker-1, example-host], want the cluster's two`; `summary = {"ok":1,"warn":2}, want the cluster's map[ok:1 warn:1]` |
| 8 (extra) | `/api/v1/host` marked `WallLink: true` | TestAWallLinkOpensTheWallAndNothingElse | `/api/v1/host answered 502 to a wall link` (the gate let it through) |

## Verification (all on the Pi)

- `go test ./internal/host/... ./internal/history ./internal/httpapi/... ./cmd/holzkube-managerd -count=1`: exit 0. Run times: httpapi 227 s, handlers 66 s, updatestatus 61 s.
- Each task's `<verify>` commands were run and each exited 0. Their `--- PASS` lines were seen for TestSampleHasItsOwnBaseline, TestTheHostHistorySurvivesARestartWithItsGap, TestLatest, TestTheWallsHost and TestTheWallCarriesTheHost (both subtests).
- `cmd/holzkube-managerd` TestOnlyTheWallAcceptsAWallLink, TestAWallRouteStillRequiresSomething and TestEveryRouteThatReachesUpstreamHasABudgetRow: exit 0.
- `GOOS=darwin GOARCH=arm64 go build ./...` and `GOOS=linux GOARCH=amd64 go build ./...`: exit 0.
- `./bin/task lint:go`: 0 issues. The first run found 1 errcheck issue on a type assertion in `sample_test.go`, which was fixed before the Task 3 commit.
- `go test ./internal/publicrepo/`: ok. `go.mod`/`go.sum` are unchanged and no package was added.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Guard that could not go red] TestTheWallsHost's summary check compared against a map it shared with the answer**
- **Found during:** Task 3, injection 7
- **Issue:** The fault changed `wall.Summary` in place. The test compared the answer's summary against that same `wall.Summary` map, so the summary assertion stayed green. The nodes assertion did go red.
- **Fix:** The test now compares against a literal `{ok:1, warn:1}`. After the fix the fault turns both assertions red.
- **Commit:** 7dcfe12

**2. [Rule 2 - Correctness] rx/tx need a rate window and at least one physical link**
- The plan's rule was "every physical link has a rate". It is vacuously true when there are no physical links. That would record rx/tx = 0 on a container host, where all links are virtual, and on the sampler's first pass whenever no physical link was listed. `physicalRates` now also requires `RatesOverSeconds != nil` and `len(Physical) > 0`. The first-read case is covered by a `TestHistoryValues` subtest.

**3. [Rule 3 - Lint] errcheck on a type assertion in `sample_test.go`**
- `hardenedHost` now returns `overlayFS` instead of `fs.FS`. This change landed in the Task 3 commit.

**4. Extra tests beyond the plan's list**
- `TestTheWallDoesNotReadTheHost`: before any sample, the wall answers `host: null`, and answering the wall creates no snapshot (T-12-11).
- In `TestLatest`: a page `Read` creates no snapshot, and a hot host's snapshot is warn with one warning.
- Injection 8, the wall-link refusal of `/api/v1/host`.

### Notes

- On an unsupported platform, `Sample` still returns an empty map without an error. It also now leaves an unknown snapshot, so a darwin build's wall shows a grey host tile.
- README: this plan is server-only. The README text and the `host.png`/`wall.png` renders belong to 12-07 (its must_haves list them).
- `docs/api-contract.md` does not yet describe `health` or the wall's `host`. That is 12-08's job.

## Known Stubs

None. The browser does not read `health` or the wall's `host` yet. Plans 05 to 07 draw them; the zod schemas are not strict, so the new keys do not break the current UI.

## Threat Flags

None beyond the plan's threat model.
- T-12-08: `wallHost` carries only name, state and reason, and unknown always reads "not readable", which the tests assert.
- T-12-09: injection 8 went red.
- T-12-10: injection 6 went red.
- T-12-11: the wall reads `Latest()` under `c.mu` and never reads the host; `TestTheWallDoesNotReadTheHost` checks this.

## Self-Check: PASSED

- FOUND: internal/httpapi/handlers/wall_host.go, internal/httpapi/handlers/wall_host_test.go, internal/httpapi/wallhostapi_test.go
- FOUND: 31dd4aa, 5fbbbb0, 7dcfe12
