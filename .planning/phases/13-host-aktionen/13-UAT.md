---
status: partial
phase: 13-host-aktionen
source: 13-01-SUMMARY.md, 13-02-SUMMARY.md, 13-03-SUMMARY.md, 13-04-SUMMARY.md, 13-05-SUMMARY.md, 13-06-SUMMARY.md, 13-07-SUMMARY.md, 13-08-SUMMARY.md, 13-09-SUMMARY.md
started: 2026-09-29T19:23:00Z
updated: 2026-09-30T00:00:00Z
---

## Where this ran

The operator's Raspberry Pi 5 (aarch64). A throwaway instance built from the
checkout at ec3f292 (`./bin/task build`), `--insecure-http --listen
127.0.0.1:18443` with an empty data directory in the session scratchpad; driven
by curl and by the built-in browser at 390 x 844. The helper is not installed
on this machine, and cannot be without root, so everything past "helper
missing" is blocked here. Production service and its data directory untouched.

## Current Test

[testing complete]

## Tests

### 1. Four host actions on /host, off with one reason while the helper is missing
expected: Check for updates and install, Restart service, Restart host, Shut down host in a 2 x 2 grid below md, all disabled, one sentence saying the helper is not installed.
result: pass

### 2. Helper notice names what is missing and how to install it
expected: The notice lists each missing piece with its path and the four install commands; every sentence is true for the machine it is shown on.
result: issue
reported: "On a machine with nothing installed the third item reads 'holzkube-manager-host.path is installed but not enabled' -- the path unit is listed as missing one line above."
severity: minor

### 3. Button labels fit their buttons at 390 px
expected: No label runs past its button at 390 px.
result: issue
reported: "'Check for updates and install' overflows its button at 390 px: scrollWidth 180 against clientWidth 173; the icon touches the left border."
severity: cosmetic

### 4. With the helper missing an action is refused and leaves nothing behind
expected: After a sudo window, each of reboot, poweroff, restart-service, update answers 409 conflict.host-helper-missing; no order file appears in the data directory; each attempt is in the audit log under host.<action>.
result: pass

### 5. Dialog with typed hostname, sudo, and order status up to "back"
expected: Clicking an action opens its dialog, the typed hostname and a sudo window place the order, the status box follows it until the helper reports it done.
result: blocked
blocked_by: other
reason: "Needs the root helper installed on the host; the buttons stay off without it. Held by HostActions.test.tsx (coverage D3, D4) and the unshare round trip TestHostActionRoundTrip."

### 6. Waiting notice across a real host reboot
expected: While a rebooting host does not answer, /host shows the waiting notice, not the stale one, also in a second session that did not place the order.
result: blocked
blocked_by: physical-device
reason: "Needs the helper installed and a real reboot of the Pi; both are the operator's call."

### 7. Four gates per host action route (13-03 D1)
expected: Sudo window, operator role and a host-bound token on each of the four routes; 202 with all three
result: pass
source: automated
coverage_id: D1

### 8. Typed hostname compared by the server (13-03 D2)
expected: Wrong, empty, differently-cased or unreadable hostname gives no token
result: pass
source: automated
coverage_id: D2

### 9. Audit entries without secrets (13-03 D3)
expected: Every host action is in the archive under host.<action>, and neither the token nor the typed hostname is
result: pass
source: automated
coverage_id: D3

### 10. Buttons, order and grid (13-08 D1)
expected: Four buttons in the header, harmless first, machine-wide pair apart and red; 2 x 2 grid with visible labels below md
result: pass
source: automated
coverage_id: D1

### 11. One reason for disabled buttons (13-08 D2)
expected: The one reason the buttons are off: container, helper missing, reader, no hostname, not answering, update running, order waiting
result: pass
source: automated
coverage_id: D2

### 12. Action dialogs (13-08 D3)
expected: One dialog per action with typed hostname, Keep running, destructive confirm for the machine-wide pair; 428 opens SudoDialog on top and replays; refusals keep the typed text
result: pass
source: automated
coverage_id: D3

### 13. Order status (13-08 D4)
expected: Order status from placed to back / update finished, Dismiss status per id, focus after placement
result: pass
source: automated
coverage_id: D4

### 14. Helper notice layout (13-08 D6)
expected: Missing pieces in the server's order, install commands in a self-scrolling pre, nothing wider than 390 px
result: pass
source: automated
coverage_id: D6

### 15. Long hostname in the dialog (13-08 D7)
expected: A 40-character hostname wraps inside the host action dialog at 390 px
result: pass
source: automated
coverage_id: D7

### 16. Install the helper and the check unit, then probe
expected: Following deploy/HOST-HELPER.md on the Pi, /host shows all five actions on; "Check for updates" answers under Update check without installing anything; then "Check for updates and install" runs the real update.
result: [pending]

### 17. The check unit under its real sandbox (V-26)
expected: After installing, `systemctl start holzkube-manager-update-check.service` reaches GitHub and records current/available in the update status; `journalctl -u holzkube-manager-update-check` shows no sandbox denial (no "Operation not permitted", "Bad system call" or "ohne Sperre"; the IN-03 keys SystemCallFilter=, ProtectProc=, ProcSubset=, PrivateIPC= are in force), and /var/lib/holzkube-manager-update/.lock is -rw------- root root.
result: [pending]

### 18. Restart service and Shut down host, once each
expected: Each runs through the helper exactly once; the page shows the waiting notice and then "back" (restart) or the host is off (poweroff); a check cut off by a reboot shows the "host restarted before the check ended" sentence.
result: [pending]

## Summary

total: 18
passed: 11
issues: 2
pending: 3
skipped: 0
blocked: 2

## Gaps

- gap_id: G-13-2
  resolved_by: 13-10-PLAN.md
  resolved_at: 2026-09-30
  truth: "Every sentence of the helper notice is true for the machine it is shown on"
  status: resolved
  reason: "User reported: On a machine with nothing installed the third item reads 'holzkube-manager-host.path is installed but not enabled' -- the path unit is listed as missing one line above."
  severity: minor
  test: 2
  root_cause: "hostaction.Detect reports MissingNotEnabled whenever the paths.target.wants link is absent, also when the unit files themselves are missing; its own comment says not-enabled means 'the units are there'. HostActions.tsx renders not-enabled as 'is installed but not enabled'."
  artifacts:
    - path: "internal/host/hostaction/helper.go"
      issue: "not-enabled reported alongside path-unit"
    - path: "web/src/components/HostActions.tsx"
      issue: "line 777 asserts the unit is installed"
    - path: "web/fixtures/demo.json"
      issue: "fixture shows all three pieces missing, so host.png carries the false sentence"
  missing:
    - "Report not-enabled only when both unit files are present (or word it so it is true either way), with a test seen red against the old behaviour; fixture, contract and host.png follow"
  debug_session: ""

- gap_id: G-13-3
  resolved_by: 13-11-PLAN.md
  resolved_at: 2026-09-30
  truth: "No host action label runs past its button at 390 px"
  status: resolved
  reason: "User reported: 'Check for updates and install' overflows its button at 390 px: scrollWidth 180 against clientWidth 173; the icon touches the left border."
  severity: cosmetic
  test: 3
  root_cause: "The 2 x 2 grid below md gives each button 173 px; the longest label with its icon needs 180 px and the button neither wraps nor shortens it. The layout audit only checks against the viewport edge, not a child past its own button."
  artifacts:
    - path: "web/src/components/HostActions.tsx"
      issue: "button label does not wrap below md"
  missing:
    - "Let the label wrap (or shorten it below md) so scrollWidth <= clientWidth at 390 px, with a check that goes red against the current markup; phase 14's clipping check is the natural home"
  debug_session: ""
