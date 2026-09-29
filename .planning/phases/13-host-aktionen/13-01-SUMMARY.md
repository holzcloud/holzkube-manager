---
phase: 13-host-aktionen
plan: 01
subsystem: api
tags: [host-actions, root-helper, systemd, fsstore, confirmation, react]
status: complete

requires:
  - phase: 11-host
    provides: internal/host collector, GET /api/v1/host, /host page with the header slot for actions
  - phase: 12-host-wie-ein-knoten
    provides: host history, health and the notice stack on /host
provides:
  - fsstore.PlaceNew (exclusive placement by link(2))
  - internal/host/hostaction (Action, Box, Place, Order, ReadResult)
  - deploy/holzkube-manager-host.sh (the root helper, complete)
  - POST /api/v1/host/confirm and POST /api/v1/host/actions/{reboot,poweroff,restart-service,update}
  - CodeHostOrderPending (409 conflict.host-order-pending)
  - actions {order, result} in GET /api/v1/host; reason code host-action.no-result
  - api.hostActions, HOST_ACTION_PATHS, four sudo labels
  - HostActions / HostOrderStatus on /host (update button, dialog, status box)
  - harness options withHostActions / withHostOver and the installedHelperFS fixture
affects: [13-03, 13-04, 13-05, 13-06, 13-07, 13-08, 13-09]

actuals:
  tokens: 21400
  tasks: 2
  commits: 2
plan_head_before: ef4648a4165144bd7927014bb136d2e68607c1d6
plan_head_after: cbff04115b3a48960fc50c0a79fe4e6ebd3a2c0d

tech-stack:
  added: []
  patterns:
    - "One-slot order file placed through an injected fsstore primitive (link, not rename)"
    - "Host token intent {Action: host.<a>, Machine: @host}, rebuilt from the route"
    - "Root script consumes before it acts; result only in its own state directory"

key-files:
  created:
    - deploy/holzkube-manager-host.sh
    - internal/host/hostaction/hostaction.go
    - internal/host/hostaction/result.go
    - internal/httpapi/hostactionsapi_test.go
    - web/src/components/HostActions.tsx
    - web/src/components/HostActions.test.tsx
  modified:
    - internal/store/fsstore/atomic.go
    - internal/host/host.go
    - internal/host/collector.go
    - internal/host/collector_test.go
    - internal/httpapi/router.go
    - internal/httpapi/problem.go
    - internal/httpapi/handlers/host.go
    - internal/httpapi/handlers/confirm_test.go
    - internal/httpapi/endtoend_test.go
    - internal/audit/redact.go
    - cmd/holzkube-managerd/main.go
    - cmd/holzkube-managerd/budget_test.go
    - docs/api-contract.md
    - web/src/api.ts
    - web/src/routes/host.tsx

key-decisions:
  - "The result reader accepts `- -` with rejected AND failed (only never with started): the script writes `- - failed` when it cannot remove the order, and refusing that line would show a correct file as read-failed"
  - "On an unsupported platform (darwin) actions.result is Hidden unsupported, keeping the every-reading-unsupported invariant; the order stays (the process's own fact)"
  - "The actions group is a <fieldset aria-label=\"Host actions\"> rather than div role=group (biome a11y/useSemanticElements); reset classes keep its min-content width off the phone grid"
  - "withHostOver builds the harness's collector after the Box so GET /api/v1/host reads the same slot the routes place into, as main.go wires it"

patterns-established:
  - "installedHelperFS: MapFS for the installed helper files (fs.ReadLinkFS overlay) + a real temp dir for the helper's state directory"
  - "runHelper: a script copy refused without its three overrides, run as root only under unshare --user --map-root-user with a sealed env and a stub systemctl"

requirements-completed: [HACT-04, HACT-05, HACT-06]

duration: 32min
completed: 2026-09-29
---

# Phase 13 Plan 01: Host-action tracer Summary

**"Check for updates and install" runs end to end: typed hostname -> host-bound token -> gated route -> one exclusive `update <id>` line in the data directory -> root helper (consume, validate, fixed `systemctl start --no-block holzkube-manager-update.service`) -> strict read of its `last` -> status box on /host.**

## Where this ran

The operator's Raspberry Pi 5 (aarch64), Go 1.26.7 via GOTOOLCHAIN, **no -race** (ThreadSanitizer refuses the Pi 5 kernel's address space). The root run of `TestHostActionRoundTrip` **ran and was not skipped** (`unshare --user --map-root-user id -u` printed 0; the test log shows the helper's line `Auftrag <id>: update` and `--- PASS: TestHostActionRoundTrip`). Nothing was installed; the helper exists only as `deploy/holzkube-manager-host.sh` and ran as temp copies with a stub systemctl.

Production service untouched: `systemctl show holzkube-manager.service -p ActiveEnterTimestamp` read `Mon 2026-09-28 18:06:00 CEST` before (00:32Z) and after (01:03Z) the plan.

## Performance

- **Duration:** ~32 min
- **Started:** 2026-09-29T00:32:16Z
- **Completed:** 2026-09-29T01:04Z
- **Tasks:** 2
- **Files:** 21 (6 created, 15 modified)

## Accomplishments

- `fsstore.PlaceNew`: temp with the sweep prefix, 0600 before write, fsync, `os.Link` (EEXIST -> wraps fs.ErrExist), temp always removed, dir fsync.
- `internal/host/hostaction`: Action/Actions/Known, Box with Place (crypto/rand 16-hex id, `<action> <id>\n` only), Order (pending/picked-up by Lstat), Result; `ReadResult` modelled on updatestatus.Read (size capped twice, one line, four fields, errors never quote bytes).
- `deploy/holzkube-manager-host.sh`: the full measured skeleton, German comments, override block, EUID and state-dir (symlink/not-dir/not root-only) checks, dd nofollow/nonblock as the only read, withdrawn-before-pickup exit, `rm` before anything, anchored ERE under LC_ALL=C, 60 s / +5 s age window, `record ... started` before systemctl, fixed argv per action.
- Five routes on the D-06 gates; `hostTypedPhrase` separate from `typedPhrase`; `hostIntentTarget = "@host"`; four audit allowlist entries with empty lists; five `noUpstream` rows; contract "Host actions" subsection.
- `actions` in GET /api/v1/host; web schema with a `.default` so older daemons parse; `api.hostActions`, `HOST_ACTION_PATHS`, sudo labels; update button, dialog ("Keep running", `autoCapitalize="none"`), status box first in the notice stack.

## Task Commits

1. **Task 1: tracer (button -> gates -> order -> root helper -> page)** - `957f237` (feat)
2. **Task 2: the host confirm table cannot drift** - `cbff041` (test)

## Fault injections (each put in, run, seen red from the command's own exit code, restored)

Round trip (`go test ./internal/httpapi -run TestHostActionRoundTrip`):

| Injection | rc | Failing line |
|---|---|---|
| script's `rm -f -- "$ORDER"` replaced by `true` | 1 | `hostactionsapi_test.go:344: the order is still there after the helper ran` and `:361: actions.order after the helper ran = ... State:pending, want ... picked-up` |
| `os.Link` -> `os.Rename` in PlaceNew | 1 | `hostactionsapi_test.go:311: second order: 202, want 409` |
| action route's intent `Machine: hostIntentTarget` -> `""` | 1 | `hostactionsapi_test.go:270: POST /api/v1/host/actions/update: 403, want 202 (... forbidden.confirmation-invalid)` |
| script records time as `%Y-%m-%d_%H:%M:%S` | 1 | `hostactionsapi_test.go:352: actions.result after the helper ran = {Readable:false ...}, want readable` |

Component (`vitest run src/components/HostActions.test.tsx`):

| Injection | rc | Failing test |
|---|---|---|
| confirm button `disabled={run.isPending}` only | 1 | `× keeps the confirm button off until the hostname is typed exactly` |
| `orderPhase` takes any readable result, ignoring the id | 1 | `× never takes another order's result for this one` (`expected 'rejected' to be 'picked-up'`) |

Task 2 (`go test ./internal/httpapi/handlers -run 'TestEveryConfirmableActionDecidesOnTypedPhrase|TestEveryConfirmedActionIsInTheTable|TestEveryHostActionRequiresTyping'`):

| Fault | rc | Failing line |
|---|---|---|
| (a) `"host.reboot": true` added to `typedPhrase` | 1 | `confirm_test.go:98: typedPhrase carries "host.reboot": the node confirm route would issue a token for a host action` (+ `:102` in both tables, + `:57` in the node table guard) -> `--- FAIL: TestEveryHostActionRequiresTyping` |
| (b) `hostTypedPhrase["host.update"]` = false | 1 | `confirm_test.go:87: hostTypedPhrase[host.update] = false: a host action without the typed hostname` -> `--- FAIL: TestEveryHostActionRequiresTyping` |
| (c) `Confirmer.Check` call removed from the host action handler | 1 | `confirm_test.go:190: no Confirmer.Check call site was found in host.go` -> `--- FAIL: TestEveryConfirmedActionIsInTheTable` |

After the last restore `git status --porcelain` listed only `internal/httpapi/handlers/confirm_test.go`.

## Verification (all exit codes read from the command itself)

- `go test ./internal/httpapi -run TestHostActionRoundTrip -v`: PASS, not skipped.
- The six cmd guards (budget, reachable, client-use, allowlist both ways, wall link): 6 PASS.
- `go test ./internal/... ./cmd/... -count=1`: rc 0 (includes TestNoDirectFileAccessOutsideFsstore, TestEveryProblemCodeIsInTheContract/Emitted).
- `./bin/task lint:go`: 0 issues. `npm --prefix web run typecheck`: rc 0. `npm --prefix web run lint`: rc 0 (the 2 warnings are pre-existing, DataTable.tsx and wall.test.tsx).
- `vitest --project jsdom` on HostActions, host, fixtures, api, api.nulls: 128/128. Full `npm --prefix web run test`: 596/599 while the Go suite loaded the Pi; the three failures were 5-s timeouts in audit.test.tsx and images.test.tsx, which pass alone (77/77).
- `bash -n deploy/holzkube-manager-host.sh` and `GOOS=darwin GOARCH=arm64 go build ./...`: rc 0.
- `git diff ef4648a -- go.mod go.sum web/package.json web/package-lock.json`: empty.
- `go test ./internal/publicrepo/`: ok.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] The reader would have refused a line the script writes**
- **Found during:** Task 1 (the tracer's purpose: a seam between two layers)
- **Issue:** The plan has the script record `- - failed` when it cannot remove the order, and the reader accept `-` only with `rejected`. That correct file would appear on the page as `read-failed` ("file could not be read").
- **Fix:** `ReadResult` accepts `-` for id and action together with `rejected` or `failed`, never with `started`. Plan 13-04's reader table ("`-` with started or failed" refused) should refuse `-` only with started. Documented in result.go and the contract.
- **Files:** internal/host/hostaction/result.go, docs/api-contract.md
- **Commit:** 957f237

**2. [Rule 1 - Bug] Two existing host guards and the new `actions` field**
- **Found during:** Task 1 verification (`go test ./internal/host`)
- **Issue:** `TestNoNulls` flagged `actions.order` (null by contract when nothing was placed); `TestUnsupportedPlatformReadsNothing` found the darwin answer's `actions.result` saying `host-action.no-result` instead of `unsupported`.
- **Fix:** `order` added to `nullableKeys` with its reason; `readUnsupported` answers `actions.result` Hidden `unsupported` (the order, the process's own fact, stays); the reading count there is now 18.
- **Files:** internal/host/collector.go, internal/host/collector_test.go
- **Commit:** 957f237

**3. [Rule 3 - Blocking] Actions group element**
- **Issue:** biome's `a11y/useSemanticElements` refuses `div role="group"`.
- **Fix:** `<fieldset aria-label="Host actions">` (implicit group role) with `m-0 min-w-0 border-0 p-0`, so its default min-content width cannot push the phone grid over the edge. Plan 08's 390-px measurement covers it.
- **Commit:** 957f237

**4. [Rule 3 - Blocking] Harness wiring for GET /api/v1/host over the same Box**
- **Issue:** `withHost` takes a collector built before the harness's data directory exists, so it could not hold the Box the routes place into.
- **Fix:** `withHostOver(func(box) *host.Collector)`, applied after `withHostActions` builds the Box; `withHost` is unchanged for plan 03's use.
- **Commit:** 957f237

Also: the host confirm route answers a missing hostname with a Validation without a field error (as the plan says), and the helper script's state-directory check also refuses group/other-writable modes (the plan names only symlink, not-dir and owner; the update script's record_status does the mode check too).

## Issues Encountered

- Running `npx prettier` once on the web files rewrote them in the wrong style; api.ts and host.tsx were restored from HEAD and the edits re-applied, then formatted with the project's biome. Nothing of it reached a commit.

## Known Stubs

- Only the update button is offered on /host; reboot, poweroff and restart-service have routes, labels and sentences but no button yet. Intentional per plan: 13-08 widens the dialog to four actions and adds the reasons line.
- README, guide and demo fixture do not mention host actions yet: 13-09 carries them (CLAUDE.md's README rule is met at the phase's end, not by this tracer).

## Threat Flags

None beyond the plan's threat model (T-13-01..T-13-10): the five new routes, the order file and the result read are exactly its boundaries.

## Next Phase Readiness

- Plans 03 (gates), 04 (script matrix, reader table), 05 (claim, timer, process guard), 06 (helper detection) build on `withHostActions`, `installedHelperFS`, `runHelper` and the Box.
- 13-04 should adjust its reader row for `- - failed` (deviation 1).

## Self-Check: PASSED
