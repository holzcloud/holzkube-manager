---
phase: quick-261001-sa9-unlink-single-sign-on
fixed_at: 2026-10-01T20:14:20Z
review_path: .planning/quick/261001-sa9-unlink-single-sign-on/261001-sa9-REVIEW.md
iteration: 1
findings_in_scope: 13
fixed: 13
skipped: 0
status: all_fixed
---

# Quick 261001-sa9: Code Review Fix Report

**Fixed at:** 2026-10-01T20:14:20Z
**Source review:** .planning/quick/261001-sa9-unlink-single-sign-on/261001-sa9-REVIEW.md
**Iteration:** 1

**Session:** the operator's Pi (aarch64), Go from `~/.local/go`, Node via nvm,
no `-race`. `workflow.use_worktrees` is false, so the fixes were made and every
test ran **in the main checkout**. `holzkube-manager.service` and
`/var/lib/holzkube-manager` were not touched.

**Summary:**
- Findings in scope: 13. That is CR-01 and WR-01..07, plus IN-01 and IN-03,
  which were asked for. IN-02, IN-04 and IN-05 fell out of warning fixes.
- Fixed: 13
- Skipped: 0

**Method.** Every guard was seen red against its fault, reinstated in the
fixed tree. The script that did it has four steps:
1. It takes a byte copy of the file and injects the fault by exact string
   replacement. It refuses if the old string does not occur exactly once, or if
   the file is unchanged afterwards.
2. It runs the test and reads the exit code from the test command itself.
3. It restores the file from the copy and checks it with `cmp` (0 every time).
4. It flags a build failure as "not a result".

Two injections did not compile on the first try (G2, P2). Both were redone with
an injection that compiles, and only those reruns are counted. Every run in the
tables below exited 1.

## Decisions revised

- **DEC-1 ("unlinking ends no session") is revised.** The review shows two
  problems with it:
  - The wrong person keeps an admin session after the unlink meant to remove
    them, and the only admin cannot end it any other way (WR-03).
  - A provider session that unlinked its own account could no longer confirm
    anything, and was never offered the password (WR-04).

  Now a session that came in through the provider lives only as long as its
  account is linked to that same identity. Password sessions go on.
- **DEC-7 (the SSO-only pre-check) is revised, by removing the pre-check.** Each
  of its answers told an anonymous caller something that finishing the flow
  would not have (WR-06).
- **DEC-5's service-account refusal is withdrawn.** A binding on a service
  account was not "planted": an earlier release made it (CR-01).

## Fixed Issues

### CR-01: A service account's provider binding still signs in, and the new unlink refuses to remove it

**Files modified:**
- `internal/auth/identity.go`
- `internal/auth/auth.go`
- `internal/auth/identity_test.go`
- `internal/httpapi/handlers/users.go`
- `internal/httpapi/handlers/oidc_rules_test.go`
- `internal/httpapi/unlinkapi_test.go`
- `web/src/routes/accounts.tsx`
- `web/src/routes/accounts.test.tsx`
- `docs/api-contract.md`
- `docs/guide.md`

**Commit:** eeb3ba2

**Applied fix:**
- `FindByIdentity` skips service accounts.
- `BindIdentity` refuses a service account after its re-read (`ErrNotAPerson`).
- `StartSession` refuses a service account.
- `CurrentUser` does not count a session record that names a service account
  as signed in. Such a record could be left behind by an earlier release.
- `UnlinkIdentity` clears a service account's binding instead of refusing it.
- The page shows the leftover link and offers the unlink.

**Seen red:**

| Fault reinstated | Test that went red |
|---|---|
| `FindByIdentity` without the kind check | TestFindByIdentityNeverAnswersAServiceAccount, TestCallbackNeverSignsInAsAServiceAccount |
| `BindIdentity` without it | TestBindIdentityRefusesAServiceAccount |
| `StartSession` without it | TestASessionNeverCarriesAServiceAccount |
| `CurrentUser` without it | TestASessionNeverCarriesAServiceAccount |
| The unlink refusal put back | TestUnlinkIdentityClearsAServiceAccountsBinding, TestUnlinkSingleSignOnClearsAServiceAccountsBinding |
| The page hides the link | accounts.test.tsx |

### WR-01: Trust-on-first-use is now reachable in normal operation, can link a removed person's identity to the remaining account, and leaves no audit record

**Files modified:**
- `internal/httpapi/handlers/oidc.go`
- `internal/httpapi/handlers/oidc_rules_test.go`
- `internal/audit/redact.go`
- `cmd/holzkube-managerd/allowlist_test.go`
- `web/src/routes/accounts.tsx`
- `web/src/routes/accounts.test.tsx`
- `web/src/routes/login.tsx`
- `docs/guide.md`
- `docs/api-contract.md`

**Commit:** 761acde

**Applied fix:** `bindFirstIdentity` now writes its own `user.identity-link`
record before it binds:
- The actor is `anonymous`.
- `params` carry `account`, `account_username` and `provider` (the issuer's
  host), and `src_ip` carries the caller's address. The subject is never
  recorded.
- The outcome is success, or error with the same `sso_error` code the page was
  given.
- If the intent record cannot be written, the link is not made
  (`link-unrecorded`).

**The window, and what was decided about it:**
- **Verified:** the link only ever happens on an address that accepts the
  password (TestAFirstUseLinkNeverHappensOnAnSSOOnlyAddress).
- **No time limit was added.** The operator's switch has a service restart
  between the unlink and the relink. A window that closed on a timer would need
  a "reopen" operation, which does not exist.
- **Instead, the window is stated where it opens.** The unlink confirmation (one
  person) says the next single sign-on from the local network links the account,
  whoever completes it, and to link again right away and check the result. The
  guide's "Who can link, and when" and its switching steps say the same.

**Seen red:**

| Fault reinstated | Test that went red |
|---|---|
| No intent record | TestAFirstUseLinkIsInTheAuditArchive, TestARefusedFirstUseLinkIsInTheAuditArchive |
| No outcome | the same two |
| The issuer's path recorded | TestAFirstUseLinkIsInTheAuditArchive |
| The refusal's code dropped | TestARefusedFirstUseLinkIsInTheAuditArchive |
| The LAN-only rule removed | TestAFirstUseLinkNeverHappensOnAnSSOOnlyAddress |
| The dialog sentence gone | accounts.test.tsx |

### WR-02: The unlink's audit record does not say which account was unlinked

**Files modified:**
- `internal/audit/redact.go`
- `internal/httpapi/middleware/audit.go`
- `internal/httpapi/router.go`
- `internal/httpapi/unlinkapi_test.go`
- `docs/api-contract.md`
- `docs/guide.md`

**Commit:** 06511ae

**Applied fix:**
- `audit.AccountFromPath` maps five account actions to their path wildcard:
  `user.role`, `user.password-reset`, `user.delete`, `user.identity-unlink` and
  `service-account.rotate`.
- The middleware writes `account` (the id from the path) into the params after
  redaction, so a body that sends the same key does not decide it.
- The audit adapter adds `account_username`. It is read before the handler runs,
  so a deletion is still named.
- The issuer and the subject are still never recorded.

**Seen red:**

| Fault reinstated | Test that went red |
|---|---|
| No path capture | TestUnlinkSingleSignOnAuditRecordNamesTheAccountAndNoIdentity, TestDeletingAnAccountNamesItInTheAuditRecord |
| A body-sent `account` wins | the first |
| No name lookup | both |
| The unlink missing from the table | the first |

### WR-03: Unlinking leaves alive the sessions created through the removed identity, and the sole admin has no way to end them

**Files modified:**
- `internal/auth/session.go`
- `internal/auth/auth.go`
- `internal/auth/providersession_test.go`
- `internal/httpapi/handlers/oidc.go`
- `internal/httpapi/handlers/auth.go`
- `internal/httpapi/handlers/users.go`
- `internal/httpapi/handlers/oidc_rules_test.go`
- `internal/httpapi/unlinkapi_test.go`
- `web/src/routes/accounts.tsx`
- `web/src/routes/accounts.test.tsx`
- `web/src/routes/login.tsx`
- `web/src/components/ResumeAfterProvider.test.tsx`
- `docs/guide.md`
- `docs/api-contract.md`

**Commit:** 6460dfb

**Mechanism chosen:** a session that came in through single sign-on ends with
its link. Password sessions go on.
- `auth.StartProviderSession` stores a SHA-256 fingerprint of the issuer and
  subject in the session. The record never holds the subject itself.
- `CurrentUser` counts such a session as signed in only while the account is
  linked to that identity. An unlink, or a relink to someone else, ends it at its
  next request, wherever it is open. That is how the only admin ends a wrong
  person's session.
- A provider session from before this release has no fingerprint. It is held to
  "the account is still linked".
- A password sign-in clears the provider marks a rotated session would otherwise
  inherit.
- The SSO flag moved into the auth package under the same session key
  (`SignedInThroughProvider`).

**Seen red:**

| Fault reinstated | Test that went red |
|---|---|
| `CurrentUser` without the link check | four tests |
| No fingerprint comparison | TestASessionThroughTheProviderDoesNotOutliveARelink |
| A session from before the fingerprint treated as a password session | TestAProviderSessionFromAnEarlierReleaseEndsWithTheLink |
| The marks not cleared on sign-in | TestAPasswordSignInDropsTheProviderMarks |
| The callback starting an untied session | TestUnlinkingYourOwnLinkEndsTheSessionThatCameThroughIt |
| The caller's own session not destroyed | the same test |

### WR-04: A self-unlinked session that came in through the provider can never confirm a destructive action again

**Files modified:** as for WR-03

**Commit:** 6460dfb

**Applied fix:** that session no longer exists after the unlink:
- The unlink destroys the caller's own session when it is one that ends, and
  still answers 200 with the account (`self` true).
- The confirmation says so beforehand: "this session ends with the link".
- The page then goes to `/login?reason=unlinked`. That text warns that a single
  sign-on there links whichever provider is configured at the time.
- A password session that unlinks its own account is told "you stay signed in".

**Seen red:**

| Fault reinstated | Test that went red |
|---|---|
| The page not recognising its own provider session | accounts.test.tsx (two tests) |
| The caller's session not destroyed | TestUnlinkingYourOwnLinkEndsTheSessionThatCameThroughIt |

### WR-05: With only service accounts left, the answer is `setup-required`, but setup refuses to run

**Files modified:**
- `internal/httpapi/handlers/oidc.go`
- `internal/httpapi/handlers/oidc_rules_test.go`
- `web/src/routes/login.tsx`
- `docs/guide.md`
- `docs/api-contract.md`

**Commit:** eaacbfa

**Applied fix:**
- A new answer, `no-person` (`errBindNoPerson`), for the case where accounts
  exist and none of them is a person. It names the real remedy: an admin creates
  a person account, with an admin service account's token if no person is left.
  Checked: a token does satisfy the sudo window.
- `setup-required` now means no account at all, everywhere.

**Seen red:**

| Fault reinstated | Test that went red |
|---|---|
| The service-only case answered as setup again | TestFirstUseBindCountsOnlyPeople |

### WR-06: The SSO-only pre-check tells an anonymous caller more than finishing the flow would, and the comment says otherwise

**Files modified:**
- `internal/httpapi/handlers/oidc.go`
- `internal/httpapi/handlers/ssoonly_test.go`
- `docs/guide.md`
- `docs/api-contract.md`

**Commit:** eaacbfa

**Applied fix:**
- `refuseUnlinkedOnSSOOnlyHost` is removed. `oidc/start` answers every caller the
  same way, and the callback decides.
- The cost is one round trip to the provider when nothing is linked.
- The comment and the contract now state why.

**Seen red:**

| Fault reinstated | Test that went red |
|---|---|
| The old pre-check put back | TestSignInStartOnSSOOnlyHostDisclosesNothingAboutAccounts, four of six states |

### WR-07: Store revision conflicts surface as 500s, as a raw problem document on a browser navigation

**Files modified:**
- `internal/auth/identity.go`
- `internal/auth/identity_test.go`
- `internal/httpapi/handlers/oidc.go`
- `internal/httpapi/handlers/users.go`
- `internal/httpapi/handlers/oidc_rules_test.go`
- `web/src/routes/login.tsx`
- `docs/api-contract.md`

**Commit:** ee3a831

**Applied fix:**
- `BindIdentity` re-reads once on `store.ErrConflict`:
  - If the same identity was bound meanwhile, that is a success.
  - If a different one was, the answer is `ErrAlreadyBound`
    (`sso_error=other-identity`).
  - An unrelated change (a role) is written around.
- If the record changes under it twice, the answer is `store.ErrConflict`. The
  callback turns that into `sso_error=account-changed`.
- Every account route maps the conflict to the existing 409 `store.conflict`.

The test conflicts are real ones: the test store lets a concurrent write land
between the read and the write.

**Seen red:**

| Fault reinstated | Test that went red |
|---|---|
| No retry | TestBindIdentityLosingARevisionRace, three cases |
| The callback mapping removed | TestABindThatKeepsLosingARaceSaysSo (got 500) |
| The account-route mapping removed | TestAnAccountChangeLosingARaceIsAConflict |

### IN-01: `BindIdentity` returns the stored record with its password hash

**Files modified:**
- `internal/auth/identity.go`
- `internal/auth/identity_test.go`

**Commit:** ee3a831

**Applied fix:** both return paths now strip the password hash.

**Seen red:**

| Fault reinstated | Test that went red |
|---|---|
| The hash returned on the first-bind path | TestBindIdentityReturnsNoPasswordHash |
| The hash returned on the same-identity path | TestBindIdentityReturnsNoPasswordHash |

### IN-02: The pre-check's error branch answers a navigation with a problem document

**Files modified:** `internal/httpapi/handlers/oidc.go`

**Commit:** eaacbfa

**Applied fix:** this fell out of WR-06. The pre-check and its error branch are
gone.

### IN-03: The sudo prompt does not name the unlink

**Files modified:**
- `web/src/api.ts`
- `web/src/api.test.ts`

**Commit:** 0277eff

**Applied fix:** `ACTION_LABELS` gained "Unlink single sign-on for this account".

**Seen red:**

| Fault reinstated | Test that went red |
|---|---|
| The generic label put back | api.test.ts |

### IN-04: A failed unlink's error stays in the dialog after it is closed and reopened

**Files modified:**
- `web/src/routes/accounts.tsx`
- `web/src/routes/accounts.test.tsx`

**Commit:** 6460dfb

**Applied fix:** `unlink.reset()` runs when the dialog opens. This fell out of
the WR-03 dialog rework.

**Seen red:**

| Fault reinstated | Test that went red |
|---|---|
| The reset removed | accounts.test.tsx |

### IN-05: The redirect codes, including the new `oidc.not-linked`, are not in the API contract

**Files modified:**
- `docs/api-contract.md`
- `internal/httpapi/handlers/redirect_codes_test.go` (new)

**Commit:** eaacbfa

**Applied fix:** this fell out of WR-05, WR-06 and WR-07, which minted new codes.
- The contract gains `sso_error` and `sudo_error` tables.
- A new guard parses `oidc.go` for every code it can redirect with. It fails when
  a code has no sentence on the page or no row in the contract.

**Seen red:**

| Fault reinstated | Test that went red |
|---|---|
| A page sentence removed | TestEveryRedirectCodeIsExplainedAndInTheContract |
| A contract row removed | the same |
| A sudo sentence removed | the same |

## Verification

All of this ran in the main checkout on the Pi (aarch64):

- The targeted `go test` runs and vitest runs listed above.
- `go vet` on the touched packages.
- `go test ./internal/publicrepo/`: exit 0.
- `./bin/task ci` was run three times:
  1. **Exit 201.** The pinned golangci-lint 2.13.1 found four issues in this fix
     set: the WR-02 block sat between `audit.Params` and its doc comment, one
     file was not gofmt-clean, and there were two staticcheck QF1008 findings.
     They were fixed in 8cf88e6, which also records `internal.unexpected`
     instead of an empty cause for an unexpected link failure. That last change
     has no test of its own and is not claimed as seen red.
  2. **Exit 201.** Lint was clean. `TestANodeSaysHowItBooted` in
     `internal/upgrade` timed out ("Get … timed out"). That package was not
     touched, and the SUMMARY records the test as flaky. Run alone it passed
     (exit 0).
  3. **Exit 0.** lint:web, 61 vitest files with 848 tests, build, golangci-lint
     with 0 issues, and `go test ./... -count=1` with every package ok.

---

_Fixed: 2026-10-01T20:14:20Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
