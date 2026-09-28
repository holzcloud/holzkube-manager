---
phase: 12-host-verlauf
plan: 08
subsystem: docs, verification
tags: [api-contract, guide, host, history, health, wall, unshare, proc-subset, gate]
status: complete

requires:
  - phase: 12-host-verlauf
    provides: "12-01..12-07: the host's history route, host.Assess and the limits, the wall's host field, the /host curves, state mark and tile, fixtures and README"
provides:
  - "docs/api-contract.md: GET /api/v1/host/history, the health block of GET /api/v1/host, warn_c/danger_c on every temperature and the drives' pair, trip points as host limits, the wall's host field"
  - "docs/guide.md: what the Host page keeps, 'Not readable — no history' under ProcSubset=pid, the lines (Pi 5 80 / 110 °C), the three states and where they show, the wall's tile"
  - "the Pi runs: history through a restart with the downtime as a gap, nothing recorded for what subset=pid hides, the full gate"
affects: [phase-12-verification]

actuals:
  tokens: 7000
  tasks: 2
  commits: 2
plan_head_before: abd3c80d9d04e17141d9a0020d396d7e64b045d4
plan_head_after: 8f6a640d4f3c82150f499001744036f6f7a76f5a

tech-stack:
  added: []
  patterns:
    - "A Pi check of the real binary: dev daemon on 127.0.0.1 with a temp data dir, scratch scripts that print states and counts only"

key-files:
  created:
    - .planning/phases/12-host-verlauf/12-08-SUMMARY.md
  modified:
    - docs/api-contract.md
    - docs/guide.md
    - .planning/phases/12-host-verlauf/deferred-items.md

key-decisions:
  - "The contract's old line 'Thermal zones never supply one' (high_c/critical_c) was wrong since 12-02 and is replaced by the trip-point rule; the host example now shows cpu_thermal with critical_c 110, warn_c 80, danger_c 110"
  - "The node hardware section also gained warn_c/danger_c and the drive pair, since the rule is one rule for both pages"
  - "The guide names the states by the words the app shows (Healthy, Warning, Not readable), not by the API's ok/warn/unknown"
  - "TestEveryProblemCodeIsInTheContract does not see string-literal codes such as upstream.history-unavailable; recorded in deferred-items.md rather than fixed, because fixing it means documenting 17 other codes"

requirements-completed: [HMON-05, HMON-07, HOST-04]

coverage:
  - id: D1
    description: "The contract and the guide describe the host's history route, health block, temperature lines and the wall's host field"
    requirement: HOST-04
    verification:
      - kind: manual_procedural
        ref: "grep acceptance counts; go test ./internal/httpapi -run TestEveryProblemCode…, ./internal/config, ./internal/publicrepo"
        status: pass
  - id: D2
    description: "On the Pi the real binary records the host every 15 s and keeps every point through a SIGTERM, a 60-s pause and a restart, with no point for the downtime"
    requirement: HMON-05
    verification:
      - kind: manual_procedural
        ref: "dev daemon run 1 (this SUMMARY)"
        status: pass
  - id: D3
    description: "Under a real subset=pid /proc the binary records no cpu, memory or core series, its temperature series fill, and its health is ok"
    requirement: HMON-05
    verification:
      - kind: manual_procedural
        ref: "unshare run (this SUMMARY)"
        status: pass
  - id: D4
    description: "The wall's host tile"
    requirement: HOST-04
    verification:
      - kind: integration
        ref: "internal/httpapi/wallhostapi_test.go#TestTheWallCarriesTheHost (plan 03); not checkable on a dev daemon without an adopted cluster"
        status: pass

duration: ~42 min
completed: 2026-09-29
---

# Phase 12 Plan 08: The contract, the guide, and the Pi runs Summary

**The contract now describes `GET /api/v1/host/history`, the `health` block, the temperature lines and the wall's `host` field. The guide tells the operator what the Host page keeps, when it warns and what its three states mean. On the Pi, the real binary recorded the host every 15 s and kept every point through a stop and restart, with the downtime left as a gap. Under a real `subset=pid` /proc it recorded no CPU or memory series at all and still reported `ok`. `./bin/task ci` exited 0.**

## Where this ran

The operator's Raspberry Pi 5 (aarch64), so arm64 ran natively. Go 1.26.7 via `GOTOOLCHAIN`, Node through nvm. There was no `-race` run: ThreadSanitizer refuses the Pi 5 kernel's 47-bit address space, so the race verdict belongs to CI. The production service `holzkube-manager.service`, its binary in `/usr/local/bin` and `/var/lib/holzkube-manager` were not stopped, read, copied or replaced. Nothing was pushed, released or tagged.

**Production service, `ActiveEnterTimestamp`:** `Mon 2026-09-28 18:06:00 CEST` before anything else, and the same value after the gate. Unchanged.

## Task 1: contract and guide (`8f6a640`)

- **`docs/api-contract.md`**
  - Node hardware view: `warn_c`/`danger_c` on the example temperature, `temperature_warn_c`/`temperature_danger_c` on the drive, and the rule with the per-kind defaults.
  - "The machine holzkube-manager runs on":
    - The intro now separates the route, which stores nothing, from the sampler's history.
    - A table row for `/api/v1/host/history`, and a note that neither host route is open to a wall link.
    - The example gains `health` and the Pi's `cpu_thermal` lines (critical 110, warn 80, danger 110).
    - The `high_c`/`critical_c` row now gives the trip-point rule: passive/hot trip is high, critical trip is crit, active trips never count, disabled trips are ignored, at most 16 per zone. It replaces "Thermal zones never supply one", which has been wrong since 12-02.
  - A **`health`** section:
    - The keys, with the server's sentences as examples.
    - Worst first: critical before warning, then by relative excess.
    - What is rated: temperatures at `warn_c`/`danger_c`, filesystems at 80 % of used + available compared in integers. CPU and memory are not rated.
    - Warn outranks unknown, which outranks ok. A host with no sensor is judged by its filesystems. The thresholds are not configurable.
  - **`GET /api/v1/host/history`**:
    - The shape, with a documentation example. The node's keys without `read`/`write`.
    - A value that could not be read is absent, never 0, so there are no `cpu`/`core`/`memory` series under `ProcSubset=pid`.
    - rx/tx cover physical interfaces only, and are left out of any step where one link has no rate.
    - The sampler has its own 15-s rate window.
    - The history is kept 24 h and through a restart. The file is written every 30 min and on shutdown, and the retention never drops the host.
    - Reader, not audited, not a wall-link route. `200 {}` before the first pass, `400` on `range`, `502 upstream.history-unavailable`.
  - **Wall**:
    - `host` added to the example with documentation values.
    - The field comes from the sampler's snapshot and may trail the host page by one interval. It is null without a reader or before the first sample, and `unknown`/`not readable` once older than 45 s.
    - The reason is `healthy`, the first warning plus " and {n} more", or `not readable`. The name falls back to `holzkube-manager host`.
    - The host is never in `nodes` or `summary`. The field carries name, state and reason only.
- **`docs/guide.md`**:
  - "The Host page" no longer says "without storing anything".
  - New: the day of history (15 s, 24 h, across restarts and updates, the downtime as a gap, physical links only).
  - New: "Not readable — no history" for processor and memory under `ProcSubset=pid`, and that `ProcSubset=all` gives them a history from then on.
  - New: the warning lines, including the Pi 5's 80 °C / 110 °C and that fan stages are never a line.
  - New: the three states (Healthy, Warning, Not readable) and where they appear.
  - In the wall section: the "manager" tile, which trails by up to one interval and turns grey after 45 s.

**Guards:**
- `go test ./internal/httpapi -run 'TestEveryProblemCodeIsInTheContract|TestEveryProblemCodeIsEmitted' -v`: exit 0, both PASS (not `[no tests to run]`).
- `go test ./internal/config ./internal/publicrepo`: exit 0.
- Acceptance counts in `api-contract.md`: `/api/v1/host/history` 3, `warn_c` 9, `"host"` 1, `not readable` 3, `45 s` 1.
- Acceptance counts in `guide.md`: `Not readable — no history` 1, `110 °C` 1.

**Injections** (file copied to the scratchpad first, restored by copy, `cmp` identical afterwards; exit codes read from `go test` itself):

| # | Fault | Guard | Observed |
|---|-------|-------|----------|
| 1a | `upstream.history-unavailable` renamed everywhere in the contract | TestEveryProblemCodeIsInTheContract | **stayed green, exit 0.** The test only reads the `Code*` constants in `problem.go`; this code is a string literal in `handlers/history.go`. The guard does not cover it. The contract names it, as grep confirms, but no test holds that. Recorded in `deferred-items.md`. |
| 1b | `validation.fingerprint-mismatch` (a `problem.go` constant) renamed in the contract | TestEveryProblemCodeIsInTheContract | exit 1: `these problem codes are not mentioned …: CodeFingerprintMismatch (validation.fingerprint-mismatch)` |
| 1c | `--update-status-file` renamed in the guide's option table | internal/config TestTheReadmeDocumentsEveryOption | exit 1: `--update-status-file is an option and has no row…`; `the README documents --update-status-fyle, which is not an option` |

## Task 2: on the Pi (record only)

**Builds:**
- `./bin/task build`: exit 0; `file` reports ELF ARM aarch64.
- `GOOS=linux GOARCH=amd64 go build`: exit 0.
- `GOOS=darwin GOARCH=arm64 go build`: exit 0.

**Run 1: dev daemon on 127.0.0.1:18443 with a temp data dir.** It ran outside any systemd unit, so CPU and memory were readable here.

- setup `201`, login `204`.
- `/api/v1/host` has a `health` block: state `ok`, `warnings` an array of 0, `unreadable` an array of 0. There are two temperatures, both carrying lines: cpu warn 80 / danger 110 (critical_c 110, high_c null), other warn 75 / 90. CPU usage on the first call was `rate.no-baseline`, as expected.
- History (1 h, step 15) at three times:

  | when | cpu | memory | core (4) | rx | tx | temp (2) |
  |---|---|---|---|---|---|---|
  | +21 s | 1 | 2 | 4 | 1 | 1 | 4 |
  | +61 s | 4 | 5 | 16 | 4 | 4 | 10 |
  | +81 s (just before the stop) | 5 | 6 | 20 | 5 | 5 | 12 |

  CPU and rx/tx trail memory and temperature by one point because they need a rate baseline.
- SIGTERM: the daemon exited with status 0, and `history/metrics.bin` existed (461 bytes). After a 60-s pause the daemon restarted on the same directory (63 s from stop to start), and login returned `204`.
- After 35 s:
  - **All 53 points from before the stop were present with identical timestamps and values** (0 missing, 0 changed).
  - **0 points lay between the stop and the restart.**
  - All 10 series span the stop. The spacing across the downtime was 60 s for memory and the two temperatures, and 75 s for cpu, the four cores, rx and tx (these need a second reading after the restart). The normal spacing is 15 s.
  - 1 h then held cpu 7, memory 9, core 28, rx 7, tx 7, temp 18. The 24 h range (step 60) answered with all 10 series.
- `?range=2h` returned `400`, and the route without a session returned `401`.
- The second daemon exited on SIGTERM with status 0, and the data directory was removed.

**Run 2: the same binary under a real `subset=pid` /proc.** It ran through `unshare --user --map-root-user --mount --pid --fork sh -c 'mount -t proc -o subset=pid,hidepid=invisible proc /proc && exec ./bin/holzkube-managerd … --listen 127.0.0.1:18444 …'`. The daemon's own mountinfo showed `subset=pid`. Setup returned `201` and login `204`.

- After ≥ 40 s (at least three passes): **no `cpu` series, no `memory` series, 0 `core:*` series** (absent, not zero). The temperature series had 6 points (2 series) and rx/tx had 2 each.
- `/api/v1/host`: `cpu.usage` and `memory` were `hardening.proc-subset`. **`health.state` was `ok`**, with `unreadable` an array of 0 and `warnings` 0. The hidden CPU and memory did not make it unknown (D-11 on the real kernel).
- Stop: SIGTERM to the namespace's PID 1 (the unshare process's child). It exited without escalation, and unshare exited with status 0. The data directory was removed.

**Afterwards:** nothing listens on 18443 or 18444 (`ss -ltn`: 0 lines), both temp directories are gone, and no dev daemon is running.

**The wall's host tile** cannot be seen on a dev daemon without an adopted cluster. The evidence is `internal/httpapi/wallhostapi_test.go` `TestTheWallCarriesTheHost` from plan 03 (a simulated cluster, a session and a wall link). It passed in the gate below.

**The gate, `./bin/task ci`** (exit code written by the command itself to a file):

- **First run: exit 201**, stopped at `test:web`. Three jsdom tests timed out at 5000 ms, each after 40–47 s:
  - `NodeHardware.test.tsx` › "marks a hot sensor with words, not colour alone"
  - `PodDiagnosis.test.tsx` › "says a container asked for nothing…"
  - `Sidebar.test.tsx` › "has no mark before the first answer"

  At the time the Pi's load average was 13–15 on 4 cores, from another process's headless Chromium and a Next.js server. This plan changed only Markdown. Those three files, rerun alone (`vitest run --project jsdom …`), passed: **exit 0, 21/21 in 8.4 s**.
- **Second full run: exit 0** (22:15:36Z → 22:37:57Z).
  - `lint:web`: 2 warnings and 1 info, no error.
  - `test:web`: 589/589.
  - `build`.
  - `lint:go`: golangci-lint 2.13.1, 0 issues.
  - `go test ./...`: 80 `ok`, 8 without test files, `internal/upgrade` ok in 71.6 s.
  - `test:layout`: every route ok at 390 and 1280 px.
  - `test:next`: `internal/upgrade` ok in 57.7 s.
- The known `TestANodeSaysHowItBooted` timeout did not occur in either run.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Accuracy] The contract said thermal zones never supply a limit**
- **Found during:** Task 1
- **Issue:** The `high_c`/`critical_c` null row said "Thermal zones never supply one". Since 12-02 an hwmon chip without limits takes its twin zone's trips, and the host example still showed `critical_c: null` for the Pi's `cpu_thermal`.
- **Fix:** The row now states the trip-point rule, and the example shows `critical_c: 110`, `warn_c: 80`, `danger_c: 110`.
- **Files modified:** docs/api-contract.md
- **Commit:** 8f6a640

**2. [Rule 2 - Completeness] The node hardware section had no `warn_c`/`danger_c`**
- **Found during:** Task 1
- **Issue:** The plan asked for the lines "on every temperature (host and node)". The node view's example and bullets lacked them and the drive pair.
- **Fix:** The example and bullets now carry the rule and the defaults.
- **Files modified:** docs/api-contract.md
- **Commit:** 8f6a640

**3. [Orchestrator] `deferred-items.md` corrected.** The 12-01 entry said `./bin/task lint:web` is red. It exits 0 with warnings only, as the orchestrator measured and this gate saw again. The entry now says so.

### Found, not fixed

- **The problem-code guard does not see string-literal codes.** The plan's key link ("every problem code the routes emit is named in the contract", via `contract_codes_test.go`) is weaker than it reads. The test collects only `problem.go` constants, and injection 1a left it green. A scan of `httpapi.<Constructor>("…")` literals outside tests finds 17 codes the contract does not name (for example `upstream.inventory-unavailable`, `notfound.user`, `oidc.provider-unreachable`). `upstream.history-unavailable` and `upstream.host-unavailable` are named. Recorded in `deferred-items.md` with a proposed fix.

## Human check that remains

After the operator's next release has installed itself through the update timer, on the production service (the operator's machine, not this session):
- Open /host signed in. Check the state line and, if a threshold is crossed, the amber notice.
- Processor and Memory should say "Not readable — no history" under `ProcSubset=pid`, while Network and Sensors draw curves.
- Switch between 1 h, 6 h and 24 h.
- After the next hourly update restart, the curves from before it should still be there.
- The Host entry in the navigation carries the mark, and the wall shows the "manager" tile first in Nodes.

## Known Stubs

None.

## Self-Check: PASSED

- FOUND: docs/api-contract.md, docs/guide.md, .planning/phases/12-host-verlauf/deferred-items.md, this SUMMARY
- FOUND: commit 8f6a640
