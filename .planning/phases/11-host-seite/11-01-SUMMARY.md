---
phase: 11-host-seite
plan: 01
subsystem: api, ui
tags: [go, x/sys/unix, fs.FS, uname, clock_gettime, zod, react, tanstack-query, layout-audit]

requires:
  - phase: 10 (v1.17 node hardware page)
    provides: HARDWARE_POLL_INTERVAL_MS, the notice/dl class strings, the layout audit and demo fixture
provides:
  - "internal/host: Reading[T]/Reason and the five reason codes, View/Device/Arch, the Sys seam (uname, CLOCK_BOOTTIME, sysinfo loads, statfs) with a linux and a !linux implementation, Collector/Config/New/Read, readBounded, fsPath, reasonFor"
  - "identity readers: device-tree model (NUL trimmed) or DMI vendor+product, os-release per os-release(5), online CPU list, container detection"
  - "GET /api/v1/host: session, reader, not audited, 502 upstream.host-unavailable without a collector; Deps.Host; composition-root wiring; noUpstream entry; contract section"
  - "web: reasonSchema, reading(), hostSchema, api.host; /host page with loading, first-poll error, stale, container notice, Device card, MissingValue; Host nav entry; demo fixture; /host in the layout audit"
affects: [11-02, 11-03, 11-04, 11-05, 11-06, 11-07, 12]

actuals:
  tokens: 19861
  tasks: 3
  commits: 6
plan_head_before: 243578f01f7712b2b290ecea0f7aab58f9153851
plan_head_after: 744998dc62e9ac6474bff30890098045e8495a0c

tech-stack:
  added: []
  patterns:
    - "Reading[T]: pointer value + omitempty, so a readable 0 is sent and an unread value has no value key"
    - "Host reads go through fs.FS rooted at / (os.DirFS in production, os.OpenRoot fixtures in tests) and a four-method Sys seam; no os.ReadFile/os.Open in internal/host"
    - "Paths written as the kernel names them (absolute) and read via fsPath + readBounded; the reason message names the absolute path"
    - "zod reading() is a discriminated union on readable; no .default(0) inside a reading"

key-files:
  created:
    - internal/host/host.go
    - internal/host/sys.go
    - internal/host/sys_linux.go
    - internal/host/sys_other.go
    - internal/host/collector.go
    - internal/host/identity.go
    - internal/host/helpers_test.go
    - internal/host/collector_test.go
    - internal/host/identity_test.go
    - internal/host/testdata/pi5/...
    - internal/host/testdata/amd64/...
    - internal/httpapi/handlers/host.go
    - internal/httpapi/hostapi_test.go
    - web/src/routes/host.tsx
    - web/src/routes/host.test.tsx
  modified:
    - internal/httpapi/router.go
    - internal/httpapi/endtoend_test.go
    - cmd/holzkube-managerd/main.go
    - cmd/holzkube-managerd/budget_test.go
    - go.mod
    - docs/api-contract.md
    - web/src/api.ts
    - web/src/App.tsx
    - web/src/components/Sidebar.tsx
    - web/fixtures/demo.json
    - web/src/fixtures.test.ts
    - web/scripts/layout-audit.mjs

key-decisions:
  - "os-release escapes (\\\" \\\\ \\$ \\` \\') are unescaped inside either quote style, as Python's platform.freedesktop_os_release does; systemd treats single quotes literally, but the plan's amd64 fixture (single quotes around an escaped double quote) asks for the unescaped reading"
  - "DMI model: when only one of sys_vendor/product_name exists it is used alone; firmware placeholders are shown as read"
  - "The unread-model reason names the device-tree path (the first source tried), since that is the source on the production target"
  - "The host collector is built unconditionally in main; the nil-collector 502 exists only for harnesses and HostRoutes(httpapi.Deps{})"

patterns-established:
  - "MissingValue({reason, align}) is the only way a missing value is drawn on /host: Waiting for a second reading / Not recorded / Not readable + reason (short form for hardening.proc-subset)"
  - "Notice order on /host: stale (amber), container (slate), hardening (slate, plan 03)"

requirements-completed: [HOST-01]

coverage:
  - id: D1
    description: "GET /api/v1/host serves the Device readings: 401 without a session, 200 for admin and reader, no audit record, 502 upstream.host-unavailable without a collector"
    requirement: HOST-01
    verification:
      - kind: integration
        ref: "internal/httpapi/hostapi_test.go#TestHostAPI"
        status: pass
    human_judgment: false
  - id: D2
    description: "Every value is a well-formed reading (value xor reason), no JSON null, unread uname has no value key"
    requirement: HOST-01
    verification:
      - kind: unit
        ref: "internal/host/collector_test.go#TestEveryReadingIsWellFormed, TestNoNulls, TestUnreadableUnameHasNoValue, TestTracerReadsUname"
        status: pass
    human_judgment: false
  - id: D3
    description: "Device identity from the Pi 5 and amd64 fixtures (model NUL-trimmed, DMI, os-release, cores, arch, boot uptime), missing sources as read-failed, container detection without leaking PID 1's environment"
    requirement: HOST-01
    verification:
      - kind: unit
        ref: "internal/host/identity_test.go#TestIdentityPi5, TestIdentityAMD64, TestCountCPUList, TestOSRelease, TestIdentityMissingSources, TestContainer"
        status: pass
      - kind: integration
        ref: "internal/host/identity_test.go#TestLiveHostMatchesTheKernel (ran on the Pi 5, aarch64)"
        status: pass
    human_judgment: false
  - id: D4
    description: "/host renders header, Device card (seven rows in order), Not readable rows, container notice and badge, loading, first-poll error and stale states"
    requirement: HOST-01
    verification:
      - kind: automated_ui
        ref: "web/src/routes/host.test.tsx (12 cases)"
        status: pass
    human_judgment: false
  - id: D5
    description: "The layout guard measures /host at 390 px and 1280 px against a schema-checked Pi-shaped fixture"
    requirement: HOST-01
    verification:
      - kind: automated_ui
        ref: "./bin/task test:layout (/host ok at 390px and 1280px)"
        status: pass
      - kind: unit
        ref: "web/src/fixtures.test.ts#/api/v1/host is something the product would accept"
        status: pass
    human_judgment: false
  - id: D6
    description: "The page on the operator's real Pi, in a browser, behind the production unit's hardening"
    verification: []
    human_judgment: true
    rationale: "The production daemon was not replaced or restarted (operator's call); the page was verified by component tests, the layout audit against the fixture and the live-kernel test, not by opening it on the running service"

duration: 37min
completed: 2026-09-28
status: complete
---

# Phase 11 Plan 01: Host page tracer and Device card Summary

**`GET /api/v1/host` and the `/host` page: hostname, model, architecture, cores, OS, kernel and boot uptime read through an fs.FS and four syscalls, each value a Reading that is either a value or a reason, never a zero.**

## Where this ran

On the operator's Raspberry Pi 5 (aarch64), natively. Go 1.26.7 from `~/.local/go`, Node via nvm. No `go test -race` (ThreadSanitizer refuses the Pi 5 kernel's address space; the race verdict is CI's). The production service, its data directory and `/usr/local/bin` were not touched. `TestLiveHostMatchesTheKernel` **ran and passed** here (not skipped): hostname equals `os.Hostname()`, kernel equals `uname -r`, boot uptime within 2 s of `/proc/uptime`. darwin/arm64 cross-build checked with `GOOS=darwin GOARCH=arm64 go build ./cmd/holzkube-managerd`.

## Performance

- **Duration:** 37 min
- **Started:** 2026-09-28T14:07:55Z
- **Completed:** 2026-09-28T14:45:08Z
- **Tasks:** 3 (Task 1 a tracer)
- **Files modified:** 32

## Accomplishments

- New package `internal/host`: `Reading[T]` (pointer value, `omitempty`), `Reason` and the five codes, `View`/`Device`/`Arch`, the `Sys` seam with linux and `!linux` implementations, `Collector` with bounded reads. No `os.ReadFile`/`os.Open` anywhere in it; `TestNoDirectFileAccessOutsideFsstore` passes with no exemption added.
- Identity from sources that survive `ProcSubset=pid`: device-tree model with its NUL trimmed, or DMI; os-release per os-release(5); `/sys/devices/system/cpu/online`; `uname(2)`; `CLOCK_BOOTTIME` truncated. Container detection returns only a boolean.
- `GET /api/v1/host` behind the session and reader gate, unaudited, wired inside the `Deps` literal, listed in `noUpstream`, documented in `docs/api-contract.md` with `502 upstream.host-unavailable`.
- `/host` page with the Device card, the three readability patterns, loading/error/stale states, the D-17 container notice and badge, a Host navigation entry (Cpu icon) after Nodes, a Pi-shaped demo fixture and `/host` in the layout audit.

## Task Commits

1. **Task 1 (tracer): hostname and kernel from uname(2) end to end** - `f124281` (test, RED), `c8a7bb6` (feat, GREEN)
2. **Task 2: Device card complete** - `899e375` (test, Go RED), `024bf15` (test, web RED), `1f88951` (feat, GREEN)
3. **Task 3: layout guard sees /host** - `744998d` (feat)

Tracer gate: after `c8a7bb6` the whole Task 1 `<verify>` block was re-run (four host tests, TestHostAPI, the budget guard, the darwin build, the host component tests, typecheck), all exit 0, before Task 2 started.

## Fault injections (each guard seen red, then restored and green)

| # | Guard | Fault put back | Observed red (exit 1) |
|---|-------|----------------|-----------------------|
| 1 | `TestEveryReadingIsWellFormed` | `Hidden` returns `Value: &zero` | `collector_test.go:117: unreadable .device.hostname: readable:false carries a value key: map[readable:false reason:map[code:read-failed ...] value:]` |
| 2 | `TestEveryRouteThatReachesUpstreamHasABudgetRow` | `"GET /api/v1/host"` removed from `noUpstream` | `budget_test.go:1821: these routes have no row in the budget table and are not named as reaching nothing: GET /api/v1/host` |
| 3 | `TestIdentityPi5` | NUL removed from `readModel`'s trim set | `identity_test.go:45: model = "Raspberry Pi 5 Model B Rev 1.0\x00", want the device-tree string without its trailing NUL` |
| 4 | `TestContainer` | `detectContainer` ignores `/run/.containerenv` | `--- FAIL: TestContainer/podman_marker ... identity_test.go:208: container = false, want true` |
| 5 | `fixtures.test.ts` `/api/v1/host` case (extra, not in the plan) | `cores` value sent as the string `"4"` | `× /api/v1/host is something the product would accept ... "Invalid input: expected number, received string"` |

After each revert the named test was rerun and exited 0. Each injection was checked with `diff`/`git diff` to have actually changed the file before the run.

## RED evidence

- Go Task 1: stub `Read` returning `View{}`; TestTracerReadsUname, TestUnreadableUnameHasNoValue and TestEveryReadingIsWellFormed failed on their assertions (`hostname = {Readable:false Value:<nil> Reason:<nil>}, want readable example-host`); TestNoNulls passed on the stub, as a guard should.
- Go Task 2: stub readers; all seven new tests failed on assertions (e.g. `countCPUList("0-3") = 0, not implemented; want 4`, `container = false, want true`).
- Web Task 1: stub `HostView`/`HostPage` returning null, 6/6 failed on missing elements. Web Task 2: 5/6 new cases failed; the "not in a container" absence case passed on the stub, as it must.
- `gsd-tools check tdd-red-evidence` parses TAP/Surefire only and rejects Go's `-v` output as `invalid_record`; the plan is `type: execute`, so the evidence above is recorded verbatim instead.

## Verification (all exit codes read from the command itself)

- `go test ./internal/host ./internal/httpapi/... ./cmd/holzkube-managerd ./internal/store/fsstore ./internal/publicrepo ./internal/config -count=1` exit 0
- `GOOS=darwin GOARCH=arm64 go build ./cmd/holzkube-managerd` exit 0
- `./bin/task lint:go` (golangci-lint 2.13.1) exit 0, 0 issues
- `npm --prefix web run test` (53 files, 494 tests, jsdom and browser) exit 0; `typecheck` exit 0; `lint` exit 0 (2 warnings and 1 info remain, all in pre-existing files `DataTable.tsx` and `wall.test.tsx`)
- `./bin/task test:layout` exit 0: `ok 390px /host (3 controls, 1 items)`, `ok 1280px /host`

## Decisions Made

See `key-decisions` in the frontmatter. The two worth reading: os-release escapes are unescaped in single quotes too (the Python reader's behaviour, which the plan's fixture asks for), and the reason for an unread model names the device-tree path because that is the production target's source.

## Deviations from Plan

**1. [Rule 3 - Blocking] Commits on `main` despite the executor's protected-branch assertion**
- The GSD default treats `main` as protected; the dispatch assigned a sequential executor on `main` with normal commits, `git.branching_strategy` is `none`, and all phase work so far lives on main. Committed on main as dispatched, nothing pushed.

**2. [Acceptance-literal] `grep -c 'Value \*T' internal/host/host.go` prints 0**
- gofmt aligns the struct field as `Value    *T`; the pointer field is there (`json:"value,omitempty"` greps 1). Not changeable without breaking gofmt.

**3. [Rule 1 - Lint] Test helper renamed `valueOf` -> `cellOf`** in `host.test.tsx`: biome's `noShadowRestrictedNames` refused shadowing the global.

**4. [Minor] Absolute path constants read through `fsPath`**
- Paths are declared as the kernel names them (`/sys/...`) and read via `fsPath`, so reason messages name the absolute path and `fsPath` has a production caller now. The comment naming Podman's marker was reworded so `run/.containerenv` appears exactly once, as the acceptance grep requires.

**5. [Extra guard] Fault injection 5** on the new fixture case, not in the plan.

**Total deviations:** 5, none changing scope.

## Issues Encountered

- Under load (the Go httpapi suite running concurrently) three unrelated web tests (`audit.test.tsx`, `images.test.tsx`) timed out at 5 s; rerun alone they passed (77/77), and the final full web run passed 494/494.

## Known Stubs

None. `maxMountinfo` is declared for plans 03/04 and not used yet (golangci-lint does not flag it).

## User Setup Required

None.

## Next Phase Readiness

Plans 02-07 build on `Reading[T]`, `Sys.Loads`/`Sys.Statfs`, `readBounded`, `nullableKeys`, `hostSchema`, `MissingValue` and the demo fixture entry. The page has not been opened against the running production service (D6 above); replacing or restarting it is the operator's call.

---
*Phase: 11-host-seite*
*Completed: 2026-09-28*

## Self-Check: PASSED

All 10 key files present; all 6 task commits (f124281, c8a7bb6, 899e375, 024bf15, 1f88951, 744998d) found in the log.
