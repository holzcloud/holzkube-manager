---
phase: 13-host-aktionen
plan: 03
subsystem: api
tags: [host-actions, authz, sudo, confirmation, audit, fault-injection]
status: complete

requires:
  - phase: 13-host-aktionen
    provides: "13-01: the five host routes, hostTypedPhrase, hostIntentTarget, withHostActions and installedHelperFS"
provides:
  - "TestHostActionGates: sudo window, operator role, host-bound token and audit record on all four host action routes"
  - "TestHostConfirmGates: the typed hostname compared by the server; no token without it"
  - "the fault-injection record F1-F6 (plus two redaction injections)"
affects: [13-06, 13-09]

actuals:
  tokens: 4400
  tasks: 2
  commits: 2
plan_head_before: c2a5a0d4ef1906e58d74509a221227fcc42c3848
plan_head_after: e66fe31f479e4b605c501ae179f0c73f5cb83b58

tech-stack:
  added: []
  patterns:
    - "A refused request is checked twice: its status and code, and an empty order slot afterwards"
    - "A lock whose only effect is a record is asserted in the archive by Action, never by status"

key-files:
  created:
    - internal/httpapi/hostgatesapi_test.go
  modified: []

key-decisions:
  - "The success case uses an operator-role account (not the setup admin), so the test also proves operator is enough"
  - "The reader's sudo window is opened and it gets a valid host token, so the role is the only lock left in its way; under F3 the reader placed real orders"
  - "The redactor keeps a non-allowlisted key and writes <redacted> as its value, so the audit test checks the value (and the raw page for the hostname), not whether the key exists"
  - "F6 was injected as the intent's machine taken from a request body field; the test sends that field, and the correct handler refuses it as an unknown field (400)"

patterns-established:
  - "requireNoOrder reports a leaked order and removes it, so one fault is reported where it happened and not again by every later request that meets a taken slot"

requirements-completed: [HACT-01, HACT-02, HACT-03, HACT-04, HACT-05]

coverage:
  - id: D1
    description: "Sudo window, operator role and a host-bound token on each of the four host action routes; 202 for an operator with all three"
    requirement: HACT-05
    verification:
      - kind: integration
        ref: "internal/httpapi/hostgatesapi_test.go#TestHostActionGates"
        status: pass
    human_judgment: false
  - id: D2
    description: "The typed hostname is compared by the server; wrong, empty, differently-cased or unreadable gives no token"
    requirement: HACT-05
    verification:
      - kind: integration
        ref: "internal/httpapi/hostgatesapi_test.go#TestHostConfirmGates"
        status: pass
    human_judgment: false
  - id: D3
    description: "Every host action is in the archive under host.<action>, and neither the token nor the typed hostname is"
    requirement: HACT-05
    verification:
      - kind: integration
        ref: "internal/httpapi/hostgatesapi_test.go#TestHostActionGates/every_action_is_audited"
        status: pass
    human_judgment: false

duration: 14min
completed: 2026-09-29
---

# Phase 13 Plan 03: Host-action gates Summary

**The four locks in front of the host actions (sudo window, typed hostname, operator role, audit record) and the host-bound token are each asserted on all four routes, and each was removed once on purpose and the test seen red.**

## Where this ran

On the operator's Raspberry Pi 5 (aarch64). Go 1.26.7 through GOTOOLCHAIN. **No -race**, because ThreadSanitizer refuses this kernel's address space. The production service, its data directory and /usr/local/bin were not touched. Every exit code below comes from `go test` itself, not from a pipe.

## Performance

- **Started:** 2026-09-29T01:20:58Z
- **Completed:** 2026-09-29T01:34:15Z
- **Tasks:** 2
- **Files:** 1 created

## Accomplishments

- `TestHostActionGates` runs one harness over `installedHelperFS`, with an admin, an operator (sudo open), a second operator session (sudo shut) and a reader (sudo open). Its subtests each loop over `hostaction.Actions()`:
  - **sudo window:** with the window shut, the answer is 428 `sudo.required`.
  - **reader:** 403 `forbidden.role`, both on confirm and on each action.
  - **no confirmation:** missing, empty or made-up tokens get 403.
  - **another action's token:** 403.
  - **node token:** a `node.reboot` token for a UUID gets 403.
  - **host token bound to a machine:** 403 when sent alone, and a refusal when the machine is also in the body.
  - **operator places each action:** 202, and the order file names the action.
  - **every action is audited.**
  - After every refusal, the test also checks that the order slot is empty.
- `TestHostConfirmGates`:
  - Five wrong typings ("one letter more", "nothing", "another case", "only spaces", "a prefix") each get 400 `validation.failed` naming `typed`, and no token.
  - The hostname surrounded by spaces and a tab gets 200 with a token.
  - `node.reboot`, `host.hibernate`, `reboot` and an empty action each get 400 naming `action`.
  - When uname fails, the answer is 400 and carries no token.
- `TestARouteWithNoRoleCannotBeRegistered` ran green beside these tests. It is the **fifth lock** D-10 names: a session route without MinRole stops the process at composition.

## Task Commits

1. **Task 1: sudo window, role and token on each route** - `6edd1a9` (test)
2. **Task 2: typed hostname and the audit record** - `e66fe31` (test)

## Fault injections

Each fault was put into `internal/httpapi/handlers/host.go` (or `internal/audit/redact.go`) by a script that asserts it applied exactly once. The test was run, and the file was then restored from a saved copy and checked with `cmp`. After the last restore, `git status --porcelain` listed only the test file. Line numbers for F1, F3, F5 and F6 refer to the file at `6edd1a9`. The rest refer to `e66fe31`.

| # | Fault | go test rc | Red subtests and failing line |
|---|-------|-----------|-------------------------------|
| F1 | `Destructive: a != hostaction.Reboot` (no sudo window on reboot only) | 1 | only `TestHostActionGates/sudo_window`: `hostgatesapi_test.go:188: reboot with the sudo window shut: 202 , want 428 sudo.required ({"order":{..."action":"reboot"...` and `:191: ... the order slot is not empty` |
| F3 | `MinRole: model.RoleReader` on the action routes | 1 | only `TestHostActionGates/reader`: `:209: reader on reboot: 202 , want 403 forbidden.role` (and the same for poweroff, restart-service and update), plus `:212: ... the order slot is not empty` for each |
| F5 | `Confirmer.Check` replaced by `error(nil)` | 1 | `no_confirmation` (`:225: reboot with no body field: 202 , want 403 forbidden.confirmation-invalid`, `:228` slot not empty), `another_action's_token`, `node_token`, `host_token_bound_to_a_machine`; `sudo_window`, `reader` and the 202 case stayed green |
| F6 | intent `Machine: cmp.Or(body.Machine, hostIntentTarget)` with a `machine` body field | 1 | only `TestHostActionGates/host_token_bound_to_a_machine`: `:281: reboot with a host token bound to a machine and that machine in the body: 202, want a refusal` (all four actions), `:284` slot not empty |
| F2 | typed comparison skipped (`if false && ...`) | 1 | `TestHostConfirmGates/typed_{one_letter_more,nothing,another_case,only_spaces,a_prefix}`: `:409: host.reboot typing "example-hostx": 200  [], want 400 validation.failed naming typed ({..."token":"...`, `:413: ... the answer carries a token`; the surrounding-spaces, not-a-host-action and unreadable subtests stayed green |
| F4 | `Action: ""` on the poweroff route only | 1 | only `TestHostActionGates/every_action_is_audited`: `:336: the archive holds no host.poweroff attempt and success by operator-account (attempt false, success false)`. **`operator_places_each_action` stayed PASS**, so the poweroff route still answered 202 without an Action. No status test could have caught this. The cmd guard `TestEveryAllowlistedActionIsAReachableRoute` also went red on it (`allowlist_test.go:91: ... host.poweroff`). |
| R1 | `"typed"` added to the `action.confirm` allowlist | 1 | `every_action_is_audited`: `:345: the action.confirm records hold the typed hostname "example-host"`, `:350: action.confirm record #123 carries what was typed: map[... typed:example-host]` |
| R2 | `"confirmation"` added to the `host.poweroff` allowlist | 1 | `every_action_is_audited`: `:323: host.poweroff record #117 carries the confirmation token: map[confirmation:1790645976.XBd4...]` |

## Verification

- `go test ./internal/httpapi -run 'TestHostActionGates|TestARouteWithNoRoleCannotBeRegistered' -v`: rc 0. Seven lock subtests PASS at Task 1, eight with the audit subtest.
- `go test ./internal/httpapi -run 'TestHostConfirmGates|TestHostActionGates|TestHostActionRoundTrip' -v`: rc 0. The round trip ran and was not skipped.
- `go test ./cmd/holzkube-managerd -run 'TestEveryAuditedActionIsInTheAllowlist|TestEveryAllowlistedActionIsAReachableRoute' -v`: rc 0, 2 PASS.
- `go test ./internal/httpapi/... ./cmd/holzkube-managerd -count=1`: rc 0. `internal/httpapi` took 216 s under the Pi's load.
- `./bin/task lint:go`: 0 issues. `go test ./internal/publicrepo/`: ok.

## Deviations from Plan

**1. [Rule 1 - Test correctness] The audit parameters are redacted, not dropped.**
- **Found during:** Task 2, first run.
- **Issue:** The plan asks that the `action.confirm` record contain no `typed` and that no host.* record carry `confirmation`. The redactor, by design, keeps a key it may not record and writes `<redacted>` in place of the value. The first run was red on correct code: 37 host.* records had `confirmation:<redacted>` and 13 action.confirm records had `typed:<redacted>`.
- **Fix:** The test now asserts that the value is `audit.RedactedMarker` whenever the key is present. It also checks that the raw action.confirm page does not contain the hostname. R1 and R2 show that this check turns red once the value leaks.
- **Commit:** e66fe31

**2. [Rule 2 - Test strength] More cases than the plan listed.**
- The success case uses an operator-role account rather than the admin, so it also shows the minimum role is enough.
- The reader gets an open sudo window and a valid token, so under F3 it really placed orders.
- `requireNoOrder` removes a leaked order, so one fault is reported only where it happened. Without this, the first F1 run spread red across every later subtest.
- Two more wrong typings were added ("   " and a prefix), and two redaction injections (R1, R2).

**3. F6's shape.** `decodeJSON` refuses unknown fields. So an injection that takes the target from a body field has to add that field, and the test has to send it. The correct handler answers that request with 400, not 403, and the test accepts any refusal (≥ 400) there. The request that carries the token alone still asserts 403 `forbidden.confirmation-invalid`.

**Plan 13-06 not present yet:** there is no helper check in the Box yet. The harness still builds on `installedHelperFS` as planned, so these tests keep testing the locks once 13-06 adds the check. Nothing of 13-06 was built here.

**Total deviations:** 1 test-correctness fix, 2 strengthenings. **Impact:** no production code changed.

## Known Stubs

None.

## Threat Flags

None. The plan adds only a test file.

## Next Phase Readiness

- 13-04 and 13-05 are unaffected.
- When 13-06 adds its helper check, `newHostGates` already passes an installed-helper FS. If 13-06 changes how the Box takes that FS, `newHostGates` must follow.

## Self-Check: PASSED

- `internal/httpapi/hostgatesapi_test.go` exists.
- Commits `6edd1a9` and `e66fe31` are in `git log`.
- `git rev-list --count c2a5a0d..HEAD` = 2 before the SUMMARY commit.
