---
phase: 13-host-aktionen
reviewed: 2026-10-02T07:15:00Z
depth: standard
files_reviewed: 25
files_reviewed_list:
  - deploy/holzkube-manager-update.service
  - deploy/holzkube-manager-update.timer
  - deploy/HOST-HELPER.md
  - docs/api-contract.md
  - docs/guide.md
  - .goreleaser.yaml
  - internal/host/actions_test.go
  - internal/host/collector.go
  - internal/host/hostaction/helper.go
  - internal/host/hostaction/helper_test.go
  - internal/host/hostaction/units_test.go
  - internal/host/host.go
  - internal/httpapi/handlers/host.go
  - internal/httpapi/hostactionsapi_test.go
  - internal/httpapi/hosthelperapi_test.go
  - internal/httpapi/problem.go
  - README.md
  - web/fixtures/demo.json
  - web/fixtures/host-helper-installed.json
  - web/src/api.ts
  - web/src/components/HostActions.test.tsx
  - web/src/components/HostActions.tsx
  - web/src/fixtures.test.ts
  - web/src/routes/host.test.tsx
  - web/src/routes/host.tsx
findings:
  critical: 1
  warning: 3
  info: 5
  total: 9
status: issues_found
---

# Phase 13: Code Review Report (round 3: plans 13-16 and 13-17)

**Reviewed:** 2026-10-02T07:15:00Z
**Depth:** standard
**Files Reviewed:** 25
**Status:** issues_found

## Summary

Scope: the per-commit diffs of c317861, 4e48d57, fd811b0, 442a602, d01f1ac, 13ce423, 735a768 and 14073c8. The quick-task 261001-sa9 hunks that touch the same files (README, guide, api.ts) were not reviewed.

Where it ran: the operator's Pi (aarch64), Go from `~/.local/go` without `-race`. `go test ./internal/host/... ./internal/httpapi/... ./internal/publicrepo/...` exited 0. vitest on `HostActions.test.tsx`, `routes/host.test.tsx` and `fixtures.test.ts` exited 0 (349 tests). `TestUnitsVerify` ran and was not skipped (systemd 257 is here). No service on the host was touched.

What holds:
- **Refusal order.** Both routes refuse in this order: container, missing, update script, update unit, outdated, busy. The action route refuses before the body, the token and `Place`. The confirm route refuses after the body (it needs the action name) and before the hostname and `IssueOnce`. This is the order in 13-16-PLAN and 13-REVIEW-2-FIX IN-04, and the page (`actionReason`), the guide and api-contract say the same. The review request gave the order as "container → missing → **outdated → update-script** → …". That is not what was planned or built: for `check-update`, a missing script is named before an older helper. Because update-unit applies only to `update` and outdated only to `check-update`, their relative order cannot be observed.
- **Same reason on page and server.** `UPDATE_UNIT_ACTIONS` matches `NeedsUpdateUnit`, and a test holds the two together.
- **Allow-list.** `wantExactly` rejects every key not on the list. ReadWritePaths are derived from the script's own defaults. The timing contract is computed from the script's numbers: worst case 38m30s, 6m30s under TimeoutStartSec=45min. Start plus 4 x stop is 47 min, under the hour. The update's lock wait (600 s) is longer than the check unit's worst case (2m40s), and the check's lock wait plus its one request fits its own 2 min limit. All of this is consistent with IN-02.
- **Public repo.** No LAN address, host name, MAC, mail address or home directory from the real host appears in the scoped diffs. `internal/publicrepo` passes.

What does not hold: the shipped units and the "Updating itself every hour" docs assume a host laid out like the reference installation, and never say so (CR-01). On a host laid out differently, the script records a status that lies (CR-01, WR-01). The sandbox has never run a real install as root (WR-02). The timer is not detected anywhere, and no document says so (WR-03).

## Critical Issues

### CR-01: The hourly update is advertised to every operator, but the layout it needs is never stated, and on any other layout the page reports a false "current" or "updated"

**File:** `docs/guide.md:85-140`, `README.md:55-60,139-141`, `deploy/HOST-HELPER.md:130-150`; mechanism in `deploy/holzkube-manager-update.sh:69-73,320,442,500-501`
**Issue:**

The script hard-codes these assumptions:
- `SERVICE=holzkube-manager.service`
- `BIN=/usr/local/bin/holzkube-managerd`
- `HEALTH_URL=https://127.0.0.1:8443/...`

The unit makes only `/usr/local/bin` writable. The README's Quick start runs `./holzkube-managerd` out of the unpacked archive and then sends the reader to "Updating itself every hour". No document in the repository (README, guide, HOST-HELPER.md) says how to install the daemon as `holzkube-manager.service` at `/usr/local/bin/holzkube-managerd` listening on loopback. The guide only says "On a machine where holzkube-manager runs as a systemd service". HOST-HELPER.md names the unit, but not the binary path or the health URL.

What happens on a fresh Pi that follows the docs:

1. **No `holzkube-manager.service` (README Quick start).** `LOCAL_VERSION` becomes `keine`. The script downloads, finds no `$BIN` (so no backup), and installs a root-owned binary into `/usr/local/bin`. `systemctl restart holzkube-manager.service` then fails with "Unit not found", and `set -e` ends the run. `on_exit` records `installed=<new>, outcome=failed`. On the next hourly run `LOCAL_VERSION == REMOTE_VERSION`, so it records **`current`**, and keeps doing so every hour. The daemon the operator actually runs is never updated, and the Host page's "Update check" row says it is up to date.
2. **The unit exists but runs a binary at another path** (for example `/opt/...`). The restart succeeds and the old process answers the health check, so the run records **`updated`** to a version that is not running. From then on it records `current`.
3. **The daemon listens on a LAN address only** (`--listen 192.168.1.10:8443`). Every run fails the health check, rolls back and restarts the service twice. The next hour sees the old version again, so this repeats every hour indefinitely.

The daemon never compares its own version with `status.json` `installed` (`internal/host/updatestatus` has no such check), so nothing on the page reveals any of this.

**Fix:** State the preconditions where the feature is offered (guide "Updating itself every hour", HOST-HELPER.md "The hourly update", and one clause in the README bullet), before the install block:

```markdown
The update assumes the layout the release is built for: the daemon installed
as `/usr/local/bin/holzkube-managerd`, run by `holzkube-manager.service`, and
answering on `https://127.0.0.1:8443` (the default `--listen`, or `0.0.0.0:8443`).
On any other layout do not install these units: the update would install a
second binary nobody runs and record it as current.
```

Better still, have the script refuse to install when the service is not loaded, or does not run `$BIN`, before anything is downloaded. That check needs no network:

```bash
systemctl cat "$SERVICE" >/dev/null 2>&1 || fail "$SERVICE ist nicht installiert - nichts zu aktualisieren"
exec_path=$(systemctl show -P ExecStart "$SERVICE" | sed -n 's/.*path=\([^ ;]*\).*/\1/p')
[[ $exec_path == "$BIN" ]] || fail "$SERVICE startet $exec_path, nicht $BIN"
```

Also add a guide section that installs the daemon itself as `holzkube-manager.service`, or at least a pointer to one.

## Warnings

### WR-01: A failing `systemctl restart` after `install` skips the rollback and makes every later run report "current" (cross-file; now runs unattended every hour)

**File:** `deploy/holzkube-manager-update.sh:500-501` (reached through `deploy/holzkube-manager-update.service:ExecStart`)
**Issue:** `install ... "$BIN"` is followed directly by `systemctl restart "$SERVICE"` under `set -euo pipefail`. When the restart returns non-zero, the script exits right there: no health loop, no rollback, and the new binary is left under the old process. Causes include a missing unit, a canceled job, a start failure for a `Type=exec/notify/forking` daemon, or a sandbox denial (WR-02). `on_exit` then records `installed=<new>`, and from the next hour on the run records `current` while the old process keeps running.

`deploy/holzkube-manager-update.service` names this exact hazard in its comment ("ein Abbruch zwischen install und Neustart liesse das neue Binary unter dem alten Prozess liegen, und die naechste Stunde hielte es fuer aktuell") and sizes TimeoutStartSec to avoid it. The script reaches the same state on any restart error. With 13-16 the script runs unattended every hour, so this path is now automated.

**Fix:** Let the health loop and the rollback decide, instead of `set -e`:

```bash
install -o root -g root -m 0755 "$TMP/holzkube-managerd" "$BIN"
if ! systemctl restart "$SERVICE"; then
  log "WARNUNG: systemctl restart $SERVICE scheiterte - pruefe Gesundheit und rolle sonst zurueck."
fi
```

Apply the same treatment to the rollback's restart (line 539), so that `OUTCOME=rolled-back` cannot be skipped. `TestTheUpdateTimingContract` matches `^\s*systemctl restart "\$SERVICE"$` and will need its regex adjusted.

### WR-02: The shipped sandbox has never run a real install as root, and reinstalling over a working hand-made unit has no backup step, no verification step, and no warning about update.conf overrides

**File:** `deploy/holzkube-manager-update.service:57-135`, `deploy/HOST-HELPER.md:151-184`, `docs/guide.md:108-127`
**Issue:** By 13-16-SUMMARY, the operator's working unit is unsandboxed and has no start limit, and nothing was ever started on the host. The unit comment holds the *omitted* lines (CapabilityBoundingSet=, SystemCallFilter=, ProtectProc=) to "erst hinein, wenn sie als root durch ein echtes Installieren und einen echten Neustart gemessen ist". The lines that *were* added have not been measured that way either: ProtectSystem=strict with three ReadWritePaths, PrivateDevices, MemoryDenyWriteExecute, PrivateIPC, ProtectControlGroups and RestrictAddressFamilies. The helper unit, which shares the hardening and runs `systemctl restart`, is not installed on the real host (SUMMARY line 221), so no `systemctl restart` under this sandbox has been observed as root anywhere.

The docs nevertheless tell an operator who has working hand-made units to replace them ("These lines replace units of the same name … Skipping the block keeps them as they are"). They give:
- no `cp` of the existing units first, so there is nothing to go back to;
- no "check that it works" step that runs one real update. `list-timers` and `journalctl` show that a run happened, not that install and restart succeeded inside the sandbox.

The fixed ReadWritePaths also break the overrides that the sourced `/etc/holzkube-manager/update.conf` allows (`HOLZKUBE_MANAGER_BIN`, `HOLZKUBE_MANAGER_PREVIOUS`, `HOLZKUBE_MANAGER_UPDATE_STATUS_DIR`). An operator whose hand-made unit worked with such an override gets EROFS after reinstalling. A STATUS_DIR override fails *silently*: `record_status` and `take_lock` give up without a word, so the page's update row freezes and the lock is gone.

**Fix:** In both guides, before the block:

```sh
# keep what is there, to go back to
sudo cp -a /etc/systemd/system/holzkube-manager-update.service /etc/systemd/system/holzkube-manager-update.timer /root/ 2>/dev/null || true
```

After the block, a check that drives one real run:

```sh
sudo systemctl start holzkube-manager-update.service   # waits for the run
systemctl show -P Result holzkube-manager-update.service   # want: success
cat /var/lib/holzkube-manager-update/status.json
```

Also add a sentence that the unit allows only the default paths, so `update.conf` must not move BIN, PREVIOUS or the status directory. Until one real install and restart has gone through this unit as root, say so in the unit and in HOST-HELPER.md rather than implying the sandbox is known to work.

### WR-03: The timer is not detected, and nothing says so; an installed service with no or a disabled timer looks complete on the page

**File:** `internal/host/hostaction/helper.go:131-135` (`UpdateTimerPath`, used only by tests), `internal/host/hostaction/helper.go:446-460` (`UpdateUnitMissing`), `web/src/components/HostActions.tsx:1393-1400`, `docs/guide.md:362-375`, `docs/api-contract.md:1971-1983`
**Issue:** Detection asks only whether `/etc/systemd/system/holzkube-manager-update.service` is a regular file. In each of these states the page shows no note, the button is on, and **no hourly update ever runs**:
- the timer was never installed;
- the timer was installed but not enabled (`enable` without `--now` before a reboot, or a typo);
- the operator ran only the first line of the removal block (`disable --now`);
- a hand-made timer under another name.

The only trace is the "Update check" row's `checked_at` growing older. No document states this limitation. The notice text even promises "The commands install the unit and its hourly timer; the first hourly run follows within minutes", which suggests the page would notice if they had not. `UpdateTimerPath` is exported and documented as if something reads it, but no production code does.

**Fix:** Either detect it (cheap, from files, like the helper's `paths.target.wants` link): a regular file at `UpdateTimerPath` plus the `timers.target.wants/holzkube-manager-update.timer` link, reported as informational (no refusal, because the button does not need the timer). Or state it in api-contract (`update_unit`), in the guide's "Without the hourly update's unit" paragraph, and in HOST-HELPER.md:

```markdown
holzkube-manager does not look for the timer. With the service installed and
the timer missing or disabled, the button works and nothing runs hourly; the
Host page's "Update check" row then simply stops getting newer.
```

If it stays undetected, add a comment on `UpdateTimerPath` saying it is used only by the tests and the guide.

## Info

### IN-01: The server's refusal detail points elsewhere than the page and its sibling refusals

**File:** `internal/httpapi/handlers/host.go:296-297`
**Issue:** `hostUpdateUnitMissingDetail` ends "deploy/HOST-HELPER.md, "The hourly update", says how to install it." The page's reason says "The note below says how to install it". The sibling `hostUpdateScriptMissingDetail` says "The Host page says how to install it." Since 13-17 the page does show the note, so the reason 13-16-PLAN gave for pointing at the file ("true now and after 13-17") no longer applies. Both texts name the same cause, but the vocabulary also drifts: "hourly update's unit", "update unit", "holzkube-manager-update.service". The busy detail, by contrast, was made to *begin with* the page's sentence.
**Fix:** "The unit holzkube-manager-update.service is not installed, so no order was placed. The Host page says how to install it." Update the pinned string in `hosthelperapi_test.go` and api-contract to match.

### IN-02: "within about 20 seconds" understates the health window

**File:** `docs/guide.md:94`
**Issue:** The loop is 20 x (`curl --max-time 5` + `sleep 1`) plus `is-active`. That is about 20 s only when every connect fails at once, and up to about 120 s when the service accepts but does not answer.
**Fix:** "within about 20 seconds, at most two minutes".

### IN-03: The writable `/usr/local/sbin` covers the helper's root script; this could be enforced by the sandbox instead of by a test

**File:** `deploy/holzkube-manager-update.service:65-72,121`
**Issue:** The unit's comment names this side effect, and `TestTheUpdateScriptDoesNotShipTheHelper` keeps the script from naming the helper. But a `REPO=` override in update.conf, or a compromised release, runs as root with write access to `/usr/local/sbin/holzkube-manager-host`. A read-only bind on that one file makes `install(1)`'s unlink fail with EBUSY, without touching the update script's own self-replacement.
**Fix:** `ReadOnlyPaths=-/usr/local/sbin/holzkube-manager-host` (add it to the allow-list in `TestTheUpdateUnitRunsTheUpdate`). This should be measured with the same real run WR-02 asks for.

### IN-04: Reinstalling over hand-made units can leave stale enablement or shadow a unit elsewhere

**File:** `deploy/HOST-HELPER.md:172-180`, `web/src/components/HostActions.tsx:1393-1400`
**Issue:**
- A hand-made `holzkube-manager-update.service` that had `[Install]` and was enabled keeps its `*.wants/` symlink after the shipped file (no `[Install]`) replaces it. The update then also runs at every boot, contradicting the unit's own "Kein [Install]" rationale.
- Detection looks only in `/etc/systemd/system`. A hand-made unit in `/usr/local/lib/systemd/system` (or a `systemctl link` target under `/home`, which a daemon with `ProtectHome=` cannot stat) shows the page's notice. Its commands then silently shadow the operator's unit, and the notice, unlike the guide, does not say "replaces".
**Fix:** Add `sudo systemctl disable holzkube-manager-update.service 2>/dev/null || true` before the install line in the guide (not in the held block, or update `UpdateUnitInstallCommands` and its test). Add one sentence to the notice's explanation: "They replace a unit of the same name; `systemctl cat holzkube-manager-update.service` shows what is there."

### IN-05: With the timer shipped, a manual check during a real install now routinely ends in a bare "failed"

**File:** `deploy/holzkube-manager-update.service:38-52`, `docs/api-contract.md` (busy row)
**Issue:** An install holds the lock for its whole run, including the daemon restart. A **Check for updates** pressed in that window waits 60 s, exits 1, records nothing, and the helper records `failed`. The page has no busy state for a running update, only for a running check. This is documented in HOST-HELPER.md, and the window is short (a real install, not the hourly no-op), but with the timer installed everywhere it will now be seen.
**Fix:** Optionally treat "update unit active" as busy for `check-update` (the helper could record it), or have the page explain a failed check whose journal names the lock. No change needed for correctness.

---

_Reviewed: 2026-10-02T07:15:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
