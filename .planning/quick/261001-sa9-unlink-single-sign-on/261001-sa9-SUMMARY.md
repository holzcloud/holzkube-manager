---
phase: quick-261001-sa9
plan: 01
subsystem: auth
tags: [oidc, single-sign-on, accounts, audit, sudo]
status: complete
requires: []
provides:
  - "auth.Service.UnlinkIdentity, ErrNotLinked"
  - "auth.Service.PersonAccounts, SinglePersonAccount, ErrNoPersonAccount, ErrSeveralPersonAccounts (SingleAccount removed)"
  - "DELETE /api/v1/users/{id}/identity (admin, Destructive, audited user.identity-unlink)"
  - "conflict.not-linked problem code; linked_provider in the account view"
  - "sudo_error=oidc.not-linked"
affects: [accounts page, login page, sudo re-authentication, docs/api-contract.md, docs/guide.md, README.md]
tech-stack:
  added: []
  patterns: ["pure decision function extracted for table tests (sudoIdentityRefusal)", "BindIdentity re-reads the stored record before writing"]
key-files:
  created:
    - internal/auth/identity_test.go
    - internal/httpapi/unlinkapi_test.go
    - internal/httpapi/handlers/oidc_rules_test.go
  modified:
    - internal/auth/identity.go
    - internal/auth/token.go
    - internal/audit/redact.go
    - internal/httpapi/problem.go
    - internal/httpapi/handlers/users.go
    - internal/httpapi/handlers/oidc.go
    - internal/httpapi/handlers/ssoonly_test.go
    - cmd/holzkube-managerd/budget_test.go
    - web/src/api.ts
    - web/src/routes/accounts.tsx
    - web/src/routes/accounts.test.tsx
    - web/src/routes/login.tsx
    - web/src/components/ResumeAfterProvider.tsx
    - web/src/components/ResumeAfterProvider.test.tsx
    - docs/api-contract.md
    - docs/guide.md
    - README.md
decisions:
  - "BindIdentity re-reads the account from the store by ID: SinglePersonAccount returns hash-stripped accounts, and writing that copy back would erase the password on first single sign-on"
  - "Service-account unlink refusal uses httpapi.Conflict(CodeNotAPerson, ...) rather than notAPerson(), whose title says 'has no password'"
  - "README 'Safe by default' bullet names the unlink (CLAUDE.md README rule outranks the plan's DEC-6)"
metrics:
  duration: "41m"
  completed: 2026-10-01
actuals:
  tokens: 20200
  tasks: 4
  commits: 4
plan_head_before: 353f3bd86c8e00c335ff0289ceec144223a40719
plan_head_after: 1a083f4c4cc45f21283ba1d8a035fca6bcd9a839
---

# Quick 261001-sa9: Unlink single sign-on Summary

An admin can unlink an account's single sign-on from Settings → Accounts. The route is behind the sudo window, the audit record carries no identity, and the next first sign-in from the LAN links the new provider. First-use linking now counts only person accounts. The SSO-only pre-check no longer answers setup-required just because there are several accounts. An unlinked session's provider re-authentication is refused with oidc.not-linked, and the page explains it.

**Session:** the operator's Pi (aarch64, srv-node-01), Go from `~/.local/go`, no `-race` (the race detector is CI's alone). `holzkube-manager.service` was not touched. ActiveEnterTimestamp was `Tue 2026-09-29 20:25:38 CEST` with NRestarts=0, both before and after. `/var/lib/holzkube-manager` was not read or written.

## Commits

| Task | Commit | What |
|---|---|---|
| 1 (tracer) | b850900 | store → route → audit → page, happy path; linked_provider (host only); budget row |
| 2 | 0f31e64 | refusals (service account, not linked), conflict.not-linked, the kept session, the contract |
| 3 | 25f9fef | SinglePersonAccount/PersonAccounts replace SingleAccount; bindFirstIdentity and the SSO-only pre-check per DEC-2/DEC-7; BindIdentity keeps the password hash |
| 4 | 1a083f4 | sudoIdentityRefusal → oidc.not-linked, page text, login copy, guide "Switching providers", README bullet |

## Fault table

Every exit code below was read from the `go test` / `vitest` command itself, never through a pipe. Each fault was restored from a byte copy (`cmp` identical), and the test was then green again (exit 0). After the last commit, every file the plan touched equals HEAD.

| # | Guard | Fault | Test | Red seen (exit) |
|---|---|---|---|---|
| 1 | role | unlink route MinRole RoleAdmin → RoleOperator | TestUnlinkSingleSignOnNeedsAnAdmin (200 instead of 403) | yes (1) |
| 2 | sudo | Destructive true → false | TestUnlinkSingleSignOnNeedsTheSudoWindow (200 instead of 428) | yes (1) |
| 3 | audit record exists | Action → "user.delete" | TestUnlinkSingleSignOnAuditRecordCarriesNoIdentity ("no record of the unlink") | yes (1) |
| 4 | audit carries no identity | allowlist {} → {"issuer","subject"} | TestUnlinkSingleSignOnAuditRecordCarriesNoIdentity (record carries issuer and subject) | yes (1) |
| 5 | service-account refusal | IsService check deleted | TestUnlinkSingleSignOnRefusesAServiceAccount (200) | yes (1) |
| 5 | service-account refusal | IsService check deleted | TestUnlinkIdentityRefusesAServiceAccount (nil, want ErrNotAPerson) | yes (1) |
| 6 | unbound refusal | HasIdentityBinding check deleted | TestUnlinkSingleSignOnRefusesAnUnlinkedAccount (200) | yes (1) |
| 6 | unbound refusal | HasIdentityBinding check deleted | TestUnlinkIdentityRefusesAnUnlinkedAccount (nil, want ErrNotLinked) | yes (1) |
| 7 | both halves cleared | only Issuer cleared | TestUnlinkIdentityLetsTheNextSignInBindANewProvider ("half a binding") | yes (1) |
| 8 | session kept (DEC-1) | d.Auth.Logout after unlink | TestUnlinkSingleSignOnOfYourOwnAccountKeepsTheSession (/auth/me 401) | yes (1) |
| 9 | host only | viewOfUser reports u.Issuer whole | TestUnlinkSingleSignOnClearsTheBinding (linked_provider + issuer path in body) | yes (1) |
| 10 | people only (auth) | SinglePersonAccount over s.Users (every account) | TestSinglePersonAccountIgnoresServiceAccounts | yes (1) |
| 10 | people only (bind) | same | TestFirstUseBindCountsOnlyPeople (person+service → ambiguous; service-only → bound the service account) | yes (1) |
| 11 | error mapping | ErrNoPersonAccount/ErrSeveralPersonAccounts mappings swapped | TestFirstUseBindCountsOnlyPeople (no-account, two-person, service-only cases) | yes (1) |
| 12 | pre-check defect (current code) | the three new ssoonly tests against the unchanged pre-check | TestSignInProceedsOnSSOOnlyHostWhenALinkedAccountIsOneOfSeveral, TestSignInOnSSOOnlyHostCountsOnlyPeople, TestSignInOnSSOOnlyHostWithSeveralUnlinkedPeopleSaysBindHost (all "setup-required") | yes (1) |
| 13 | pre-check counts people | d.Auth.Users in place of d.Auth.PersonAccounts | TestSignInOnSSOOnlyHostCountsOnlyPeople (proceeded instead of bind-host) | yes (1) |
| 14 | not-linked code | !HasIdentityBinding branch removed from sudoIdentityRefusal | TestSudoRefusalNamesAnUnlinkedAccount (got oidc.other-identity) | yes (1) |
| 15 | page explanation | 'oidc.not-linked' map entry deleted | ResumeAfterProvider.test.tsx "says an unlinked account is not linked…" (title not found) | yes (1) |
| 16 | dialog counts people | personCount = users.length | accounts.test.tsx "does not count a service account…" (warning paragraph present) | yes (1) |
| 17 | bind keeps the password hash (added, see deviations) | BindIdentity writes the caller's hash-stripped copy (`u := account`) | TestFirstUseBindCountsOnlyPeople ("u-person lost its password hash to the bind") | yes (1) |

The table has sixteen plan faults plus one added fault (17). Faults 5, 6 and 10 each have two test rows.

## Suites (exit codes)

- Task 1 verify: `go test ./internal/httpapi/ -run TestUnlinkSingleSignOnClearsTheBinding` → 0, `go test ./cmd/holzkube-managerd/ -run 'TestEveryAuditedActionIsInTheAllowlist|TestEveryRouteThatReachesUpstreamHasABudgetRow'` → 0, vitest accounts.test.tsx → 0. The tracer gate re-ran the chain → 0.
- Task 2 verify: `go test ./internal/auth/ ./internal/httpapi/ -run 'Unlink|TestEveryProblemCodeIsInTheContract|TestEveryProblemCodeIsEmitted'` → 0
- Task 3 verify: the three commands → 0, 0, 0. Also `go test ./internal/auth/... ./internal/httpapi/...` → 0.
- Task 4 verify: the whole chain (handlers tests, the two vitest files, the login grep, the guide heading) → 0
- `go test ./internal/publicrepo/` → 0 before each commit
- **Final gate `./bin/task ci` → 0**: lint:web, 61 vitest files / 843 tests, build, pinned golangci-lint, `go test ./... -count=1`, layout audit. The flaky TestANodeSaysHowItBooted did not fail this time.
- Not run: `npm --prefix web run test:browser`. No browser test covers the accounts, login or sudo-notice screens. The vitest jsdom suite and the layout audit in `task ci` do cover them.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] First single sign-on would have erased the account's password hash**
- **Found during:** Task 3.
- **Issue:** The plan has PersonAccounts go through `s.Users` "so hashes stay stripped". `bindFirstIdentity` passes SinglePersonAccount's result to `BindIdentity`, which `Put`s the value it is given. The old SingleAccount read the store directly, so its result still had the hash. The new path would write a record with no hash, and the first LAN single sign-on would remove the break-glass password.
- **Fix:** `BindIdentity` now re-reads the account by ID from the store and writes the stored record. TestFirstUseBindCountsOnlyPeople plants a hash and asserts it survives the bind. That test went red with the re-read removed (fault 17).
- **Files:** internal/auth/identity.go, internal/httpapi/handlers/oidc_rules_test.go
- **Commit:** 25f9fef

**2. [Rule 1 - Wording] Service-account refusal title**
- **Issue:** The plan says to use `notAPerson(...)`, but that function's title is "That account has no password", which is wrong for an unlink.
- **Fix:** The refusal now uses `httpapi.Conflict(httpapi.CodeNotAPerson, ...)`, with the same code and the detail the plan specified. The ErrNotAPerson doc comment now covers single sign-on as well.
- **Commit:** 0f31e64

**3. [CLAUDE.md] README mentions the unlink**
- **Issue:** CLAUDE.md says every new feature updates the README in the same change, and CLAUDE.md outranks the plan's DEC-6.
- **Fix:** The existing "Safe by default" bullet gained one clause: "single sign-on (OIDC) that an admin can unlink to move to another provider". There is no screenshot, because this is not a screen of its own.
- **Commit:** 1a083f4

**4. Minor**
- The plan's HTTP DELETE tests send `{}` as the body. The CSRF chain requires a JSON Content-Type on every mutating request, the same as the wall-link revoke test, and the web client already sends one.
- In the contract, the pre-check's three answers went into the person-only paragraph, because the contract had no oidc/start refusal section to put them in.
- In the guide, the plan's "(since this release)" became "Service accounts used to count … they no longer do", which stays true without naming a version.
- In budget_test.go, the comment "The four rows below" became "The rows below".

## Known Stubs

None.

## Threat Flags

None. The new surface (the DELETE route, linked_provider, oidc.not-linked) is what the plan's threat model covers. T-Q-sa9-01..06 and 11..12 are mitigated and seen red: faults 1–13, plus fault 17, which protects the break-glass credential.

## Release notes

1. New: Settings → Accounts can unlink an account from single sign-on, so moving to a different identity provider no longer locks you out. The next single sign-on from the local network links the new provider. The guide has the steps under "Switching providers".
2. Service accounts no longer stop single sign-on from linking. Linking on first sign-in now counts only accounts for people.
3. Fixed: on an address that accepts only single sign-on, an instance with more than one account refused every sign-in as "setup required", even for a linked account. It now lets a linked account sign in.

## Self-Check: PASSED

- Files exist: internal/auth/identity_test.go, internal/httpapi/unlinkapi_test.go, internal/httpapi/handlers/oidc_rules_test.go
- Commits exist: b850900, 0f31e64, 25f9fef, 1a083f4 (`git rev-list --count 353f3bd..HEAD` = 4)
- `grep -rn SingleAccount --include=*.go` finds nothing
