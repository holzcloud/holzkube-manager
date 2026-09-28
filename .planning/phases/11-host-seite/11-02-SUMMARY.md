---
phase: 11-host-seite
plan: 02
subsystem: infra, api
tags: [bash, update-script, exit-trap, python3-json, user-namespace, go, fs.FS, strict-parser]

requires:
  - phase: 11-01
    provides: "internal/host fs.FS style (fsPath, readBounded) that the reader follows"
provides:
  - "internal/host/updatestatus: Status, ErrNotRecorded, MaxSize (4096), DefaultPath, outcome consts, Read(fsys, path)"
  - "deploy/holzkube-manager-update.sh: record_status + single `trap on_exit EXIT`; status.json in /var/lib/holzkube-manager-update; HOLZKUBE_MANAGER_* path overrides"
  - "script_test.go: writer-to-reader end-to-end tests, non-root and user-namespace root"
affects: [11-04, 11-07]

actuals:
  tokens: 11385
  tasks: 2
  commits: 4
plan_head_before: 65feb1223b697594af64e6448bb1bfebe4139adb
plan_head_after: 76573b2b84acfe99b03fbf68048114567e680cab

tech-stack:
  added: []
  patterns:
    - "Status file written by root into its own root-owned dir, read by the daemon through fs.FS with a size cap and strict validation"
    - "Shell side effects that must not change an exit code run from one EXIT trap with set +e and || true, never exit"
    - "Shell scripts are tested by running a temp copy against PATH stubs, with the root paths under unshare --user --map-root-user"

key-files:
  created:
    - internal/host/updatestatus/status.go
    - internal/host/updatestatus/status_test.go
    - internal/host/updatestatus/script_test.go
  modified:
    - deploy/holzkube-manager-update.sh

key-decisions:
  - "record_status guards itself (every obstacle returns 0) and on_exit guards it again (set +e, || true); each layer alone keeps the exit code, both were measured"
  - "TMP, OUTCOME, INSTALLED, LATEST are emptied at start instead of taken from the environment, because the EXIT trap now runs before TMP= and deletes whatever TMP names"
  - "The script test refuses to run a script copy that lacks the HOLZKUBE_MANAGER_* overrides, so no run can address the host's real binary or configuration"
  - "Reader error messages name the field and the rule but never quote file content (the RFC 3339 parse error is replaced because it would quote the value)"
  - "rolled-back keeps INSTALLED as the version read at start (the previous binary is the one that was installed)"

patterns-established:
  - "Pitfall-9 shape: one EXIT trap, set +e inside, || true on the side effect, no exit in the trap"

requirements-completed: [HOST-03]

coverage:
  - id: D1
    description: "updatestatus.Read turns the status file into a Status, ErrNotRecorded for an absent file, or a refusal naming the field (size, trailing data, JSON, checked_at, outcome, version pattern, null only for failed)"
    requirement: HOST-03
    verification:
      - kind: unit
        ref: "internal/host/updatestatus/status_test.go#TestReadStatus, TestReadStatusPath"
        status: pass
    human_judgment: false
  - id: D2
    description: "The update script records current/available/updated/rolled-back/failed after every deciding run without changing any exit code, and writes nothing for --help, unknown options, the non-root refusal and --rollback"
    requirement: HOST-03
    verification:
      - kind: integration
        ref: "internal/host/updatestatus/script_test.go#TestUpdateScript"
        status: pass
    human_judgment: false
  - id: D3
    description: "As root the script creates the status dir root 0755, refuses a symlinked or foreign-owned status dir, and records updated/rolled-back/failed on the real update paths"
    requirement: HOST-03
    verification:
      - kind: integration
        ref: "internal/host/updatestatus/script_test.go#TestUpdateScriptAsRoot"
        status: pass
    human_judgment: false

duration: 9min
completed: 2026-09-28
status: complete
---

# Phase 11 Plan 02: Update-Status Writer and Reader Summary

**The update script now records every run that reaches a decision in a root-owned `status.json` via python3 `json.dump`, from one EXIT trap that cannot change an exit code, and `updatestatus.Read` parses that file strictly into a status, "not recorded", or a refusal that names the field. A test runs a copy of the script against stubs and reads what it wrote with that parser.**

## Where this ran

On the operator's Raspberry Pi 5 (aarch64), Go via `GOTOOLCHAIN=go1.26.7`, no `-race`. **`TestUpdateScriptAsRoot` ran and passed; it was not skipped.** Unprivileged user namespaces work here. Final run: `go test ./internal/host/updatestatus -count=1 -v` exit 0, 49 `--- PASS`, 0 `--- SKIP`.

**Nothing was installed on the Pi.** `/usr/local/bin/holzkube-managerd` still has its 2026-09-27 mtime, and `/var/lib/holzkube-manager-update` does not exist. No `/tmp/status.json`, `.status.*` or `holzkube-manager-update.*` files are left behind. The installed update script is the old one and records nothing. The new one arrives with the next release, because the script replaces itself after a healthy update. Until then, plan 04 shows "Not recorded".

## Performance

- **Duration:** about 9 min
- **Started:** 2026-09-28T14:48:05Z
- **Completed:** 2026-09-28T14:57Z
- **Tasks:** 2
- **Files:** 3 created, 1 modified

## Accomplishments
- `internal/host/updatestatus`:
  - An absent file is `ErrNotRecorded`.
  - The size limit is checked twice: `fs.Stat` above 4 KiB is refused, and the read goes through `io.LimitReader`.
  - The file must hold exactly one JSON object and nothing after it.
  - `checked_at` must be RFC 3339 and is returned in UTC.
  - `outcome` must be one of the five values.
  - `installed` and `latest` are required keys matching `^v?[0-9A-Za-z.+-]{1,64}$`; `null` is allowed only when the outcome is `failed`. Unknown keys are ignored.
  - Only `fs`/`io` are used, and the file-access guard stays green.
- `deploy/holzkube-manager-update.sh`:
  - New `record_status` and a single `on_exit` trap.
  - Paths can be overridden via `HOLZKUBE_MANAGER_*` variables; the defaults are unchanged production paths.
  - `LOCAL_VERSION` is now read before the first network call.
  - The outcome is set at each decision point.
  - The second EXIT trap is removed.
  - The `--help` header (lines 2–29) is byte-identical: sha256 `b22db80f…ce68`.
- `script_test.go` covers 11 non-root and 7 root cases (listed below), all parsed through `Read`. Exit codes are taken from `exec.ExitError.ExitCode()`.

## Task Commits

1. **Task 1: updatestatus.Read**
   - `b015724` (test, red): 30 `--- FAIL` against a `Read` that returned a zero Status.
   - `25f8c05` (feat): green.
2. **Task 2: script records its outcome**
   - `e31816f` (test, red): at first every case failed at the new override guard. After step 1 (overrides only), a second red run failed exactly the 7 recording cases. Exit codes, stubs and the user namespace already worked at that point.
   - `76573b2` (feat): green.

## Files Created/Modified
- `internal/host/updatestatus/status.go`: the strict reader. The package doc names the writer, explains why the file lives outside the data directory (D-14), and states that the reader is the only thing between the file and the page.
- `internal/host/updatestatus/status_test.go`: `TestReadStatus` (28 cases) and `TestReadStatusPath`.
- `internal/host/updatestatus/script_test.go`: `TestUpdateScript` and `TestUpdateScriptAsRoot`.
  - Stubs: `curl`, `systemctl`, `journalctl`, `sleep`, plus `python3` for one case.
  - The fixture release lists assets for both arm64 and amd64.
- `deploy/holzkube-manager-update.sh`: status recording, overrides, the single trap.

### Cases in `script_test.go`
- **Non-root:**
  - `--check` current, available, and with the release list unreadable.
  - `--check` with the status dir set to 0555.
  - `--check` with python3 failing on the status write.
  - `--help`, `--rollback`, a plain update and `--bogus` all record nothing.
  - `bash -n`.
- **Root (user namespace):**
  - Healthy update (creates the dir 0755 and replaces BIN).
  - Already current.
  - Unhealthy restart leads to rolled-back, with BIN restored.
  - Checksum mismatch leads to failed, with BIN untouched.
  - `--rollback` records nothing.
  - Symlinked status dir.
  - Foreign-owned `/tmp`.

## Fault injections (each put in, run, seen red, then reverted)

| # | Injection | Result |
|---|-----------|--------|
| 1a | `parseVersion` accepts null for every outcome | RED: `TestReadStatus/current_with_latest_null_is_refused` (`err = nil … want an error containing "latest"`), `…/available_with_installed_null_is_refused` |
| 1b | Trailing-data check removed | RED: `TestReadStatus/second_object_after_the_first`, `…/text_after_the_object` (`err = nil … want "after the JSON object"`) |
| 2a | As the plan wrote it: remove `set +e` and `\|\| true` from `on_exit`, python3 fails on the write | **Stayed GREEN, so nothing was actually injected.** `record_status` guards its own python3 call and always returns 0, so the removed guard never came into play. |
| 2a′ control | `record_status` returns 1 when python3 fails, `on_exit` intact | GREEN (exit 0 kept, no file): `on_exit`'s guard holds on its own |
| 2a′ | The same `return 1`, and `set +e` / `\|\| true` removed from `on_exit` | RED: `TestUpdateScript/check_with_python3_failing_on_the_status_write`: `script_test.go:363: exit = 1, want 0 -- recording the status must never change the exit code` |
| 2b | `trap 'rm -rf "$TMP"' EXIT` put back after `TMP=` | RED: `TestUpdateScriptAsRoot/healthy_update` (`status directory not created`), `…/checksum_mismatch` and `…/unhealthy_after_restart_rolls_back` (`the script recorded no status`) |
| 2c | Owner check removed | RED: `TestUpdateScriptAsRoot/status_directory_owned_by_another_uid`: `script_test.go:510: the script wrote /tmp/status.json into a directory root does not own` (the test removed the file) |
| 2d (extra) | Symlink check removed | RED: `TestUpdateScriptAsRoot/status_directory_is_a_symlink`: `…/elsewhere holds status.json; this run must record nothing` |

The other gates also passed:
- `TestNoDirectFileAccessOutsideFsstore`: exit 0.
- `go test ./internal/publicrepo/`: exit 0.
- `./bin/golangci-lint run ./internal/host/updatestatus/...`: 0 issues.
- `bash -n deploy/holzkube-manager-update.sh`: exit 0.

## Decisions Made
See `key-decisions` in the frontmatter. The two with consequences:
- **Two guard layers.** Both `record_status` and `on_exit` protect the exit code. Injection 2a′ shows that each layer holds on its own, and that the script fails only when both are gone.
- **The test refuses a script without overrides.** Without that guard, a red run of the old script as root would have tried to address the real `/usr/local/bin`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Safety] `TMP` (and the other globals) are emptied at start**
- **Found during:** Task 2
- **Issue:** The EXIT trap now runs before `TMP=$(mktemp -d …)`. An inherited `TMP=/tmp` from the environment would have made an early exit run `rm -rf /tmp` as root.
- **Fix:** Added `TMP=""` next to `OUTCOME=""`, `INSTALLED=""` and `LATEST=""`, with a comment explaining why.
- **Commit:** 76573b2

**2. [Rule 2 - Safety] The script test refuses a script without the overrides**
- **Found during:** Task 2 (red run)
- **Issue:** Before the overrides existed, running the script under test (as root in a user namespace) would have read the host's `update.conf` and binary and tried to install to the real paths.
- **Fix:** `newScriptEnv` fails the test when any `HOLZKUBE_MANAGER_*` name is missing from the script. The first red run stopped at that guard. The behavioural red was then measured after step 1.
- **Commit:** e31816f

**3. [Rule 1 - Measurement] Fault injection (a) as worded injects nothing**
- **Issue:** See row 2a above.
- **Fix:** Measured the `on_exit` guard with a `record_status` that fails (2a′ plus a control run). This is recorded as a finding, not hidden.

**4. [Minor] `updated` reads the installed version with `|| INSTALLED=""`**
- Without it, a failing `--version` under `set -e` would have turned a successful update into exit 1.

## Known Stubs
None.

## Next Phase Readiness
- Plan 04 imports `updatestatus.Read`/`DefaultPath` into the collector. Both `ErrNotRecorded` and "not readable" are ready.
- The research mentions a flag/env `HOLZKUBE_MANAGER_UPDATE_STATUS_FILE` and a guide row for it. Neither is part of this plan; they belong to plan 04.

## Self-Check: PASSED
- FOUND: internal/host/updatestatus/status.go, status_test.go, script_test.go, deploy/holzkube-manager-update.sh
- FOUND commits: b015724, 25f8c05, e31816f, 76573b2
