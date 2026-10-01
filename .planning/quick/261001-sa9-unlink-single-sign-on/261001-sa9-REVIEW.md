---
phase: quick-261001-sa9-unlink-single-sign-on
reviewed: 2026-10-01T19:33:06Z
depth: deep
files_reviewed: 20
files_reviewed_list:
  - README.md
  - cmd/holzkube-managerd/budget_test.go
  - docs/api-contract.md
  - docs/guide.md
  - internal/audit/redact.go
  - internal/auth/identity.go
  - internal/auth/identity_test.go
  - internal/auth/token.go
  - internal/httpapi/handlers/oidc.go
  - internal/httpapi/handlers/oidc_rules_test.go
  - internal/httpapi/handlers/ssoonly_test.go
  - internal/httpapi/handlers/users.go
  - internal/httpapi/problem.go
  - internal/httpapi/unlinkapi_test.go
  - web/src/api.ts
  - web/src/components/ResumeAfterProvider.test.tsx
  - web/src/components/ResumeAfterProvider.tsx
  - web/src/routes/accounts.test.tsx
  - web/src/routes/accounts.tsx
  - web/src/routes/login.tsx
findings:
  critical: 1
  warning: 7
  info: 5
  total: 13
status: issues_found
---

# Quick 261001-sa9: Code Review Report

**Reviewed:** 2026-10-01T19:33:06Z
**Depth:** deep
**Files Reviewed:** 20
**Status:** issues_found

## Summary

This review covers the unlink route (`DELETE /api/v1/users/{id}/identity`), the switch from `SingleAccount` to `PersonAccounts`/`SinglePersonAccount`, the SSO-only pre-check, the `BindIdentity` re-read, `oidc.not-linked`, and the page, docs and tests for them. Call chains were traced into `fsstore` (rev CAS), the CSRF and audit middleware, `auth.CurrentUser`/`StartSession`, the setup handler, `refuseIfLastAdmin`, `MintBreakGlass`, `SudoDialog` and the `/auth/me` handler.

**Session:** the operator's Pi (aarch64). Go from `~/.local/go`, no `-race`.

**What was run (exit codes read from the commands themselves):**
- `go test ./internal/auth/ ./internal/httpapi/... ./internal/audit/ -count=1` → 0
- `go test ./internal/publicrepo/ -count=1` → 0
- vitest on `accounts.test.tsx` and `ResumeAfterProvider.test.tsx` → 0 (26 tests)
- Probe tests in a scratch copy of the tree (the checkout was not changed):
  - A service account with a binding is returned by `FindByIdentity`, and `UnlinkIdentity` refuses it.
  - `BindIdentity` binds a service account if asked.
  - Two concurrent `BindIdentity` calls: one wins and the other fails with `store.ErrConflict`. There is no double bind, but the loser gets a 500.

**Checked and found sound:**
- **Role, sudo and CSRF.** The route is `MinRole: RoleAdmin` and `Destructive: true`. CSRF covers DELETE because `middleware.IsMutating` treats every method except GET, HEAD, OPTIONS and TRACE as mutating.
- **Concurrent writes.** Lost updates between unlink, bind, role change and password reset are prevented by the store's rev compare-and-swap.
- **`BindIdentity` re-read.** It does not lose or resurrect the password hash, role or sessions: it writes the freshly read record at its own rev.
- **Audit params.** The `user.identity-unlink` allowlist entry is `{}`, so a body carrying issuer or subject is redacted. Neither value reaches an error string or a log line in the new code.
- **Public-repo identifiers.** The diff and the commit messages contain only documentation values (192.168.1.10, idp.example.com, auth.example.com, manager.example.com).

**Main concerns:**
- **Service-account bindings.** The login path still honours a provider binding on a service account, and the new unlink refuses to remove one (CR-01). Code from before this change could create such a binding.
- **First-use linking.** Counting only people makes trust-on-first-use reachable in normal operation, and the unlink reopens it on purpose (WR-01).
- **Audit and sessions.** The audit record does not say whose account was unlinked (WR-02), and sessions established through the removed identity survive (WR-03).

## Critical Issues

### CR-01: A service account's provider binding still signs in, and the new unlink refuses to remove it

**Files:**
- `internal/auth/identity.go:32-49` (FindByIdentity)
- `internal/auth/identity.go:66-87` (BindIdentity)
- `internal/auth/identity.go:110-112` (UnlinkIdentity)
- `internal/httpapi/handlers/oidc.go:340-358` (completeLogin)
- `web/src/routes/accounts.tsx:203-249`
- `docs/api-contract.md:3599-3602`

**Issue:**

`FindByIdentity` matches every bound account regardless of kind. `completeLogin` then calls `StartSession(u)` with whatever came back, and neither `StartSession` (auth.go:184) nor the session branch of `CurrentUser` checks `IsService()`. A service account that carries an issuer/subject therefore gets a full browser session through the provider. For `u-break-glass` that also skips its `TokenExpiresAt`: a session path never reads the token expiry.

The new code calls such a binding "planted" (identity.go:106-109, contract line 3601). That is wrong:
- The `SingleAccount` this change removes required exactly one account of any kind. Fault 10 in the SUMMARY records that the old rule "bound the service account" in the service-only case.
- A service-only instance is reachable: `refuseIfLastAdmin` (users.go:172-184) counts service admins, so the last person can be deleted while a break-glass or service admin remains.

Instances that ran earlier releases can carry such a binding. After this change:
1. Such a binding still signs in (`FindByIdentity` → `StartSession`).
2. The one operation built to remove bindings refuses it with `conflict.not-a-person`.
3. The page hides it: `SignInCell` ignores `linked_identity` for service accounts, and `linked` is false for them (accounts.tsx:249). `accounts.test.tsx` even renders "a service account … that shows a link" and asserts that no remedy is offered.
4. The SSO-only pre-check counts only people, so it answers `setup-required` or `bind-host` while the callback on the LAN would happily sign in as the service account.

Separately, `BindIdentity` has no kind guard of its own. Verified: a probe bound `u-svc2` (KindService) without error. Only the caller's use of `SinglePersonAccount` keeps services out, and the re-read record is never re-checked.

**Fix:** Close it at the domain layer, and make the unlink remove the binding instead of refusing:

```go
// FindByIdentity
for _, u := range users {
    if u.IsService() || !u.HasIdentityBinding() {
        continue // a service account never signs in through the provider
    }
    ...
}

// BindIdentity, after the re-read
if u.IsService() {
    return model.User{}, ErrNotAPerson
}

// UnlinkIdentity: clear a binding on a service account rather than refuse it
if !u.HasIdentityBinding() {
    return model.User{}, ErrNotLinked
}
u.Issuer, u.Subject = "", ""
```

Also:
- Have `completeLogin` refuse a service account defensively before `StartSession`.
- Show a service account's binding on the page with an unlink.
- Correct the contract sentence "can only have been planted".
- Add a test that plants a binding on a service account and asserts that the callback does not start a session.

## Warnings

### WR-01: Trust-on-first-use is now reachable in normal operation, can link a removed person's identity to the remaining account, and leaves no audit record

**Files:**
- `internal/httpapi/handlers/oidc.go:411-430`
- `internal/auth/identity.go:173-186`
- `docs/guide.md:551-576`

**Issue:** `bindFirstIdentity` links whichever provider identity first completes a sign-in on the LAN to the sole person account. That account is usually an admin. Nothing ties the identity to that person: no username or email match, and no signed-in password session.

Before this change, that state existed only when the instance had exactly one account in total, which in practice meant a fresh install. With service accounts no longer counting, it is the normal state of a single operator with automation who has not linked, or who has just unlinked. The unlink makes reopening it a routine action.

Concrete wrong-person path: persons A (linked) and B, plus a CI service account.
1. Promote B, then delete A. The old code refused to bind here, because there were more than one account.
2. Any LAN sign-in through the provider, including A's own identity, which no longer resolves, is now bound to B.

The same applies to anybody with a provider account who wins the race between step 3 and step 4 of "Switching providers". The guide's "Why this order" covers only the old provider.

Also: the callback route is a GET, so the audit middleware skips it (`!mutating`). The first-use bind, which grants an admin account to a provider identity, is therefore only a slog line. The unlink before it is audited; the re-link after it is not.

**Fix:**
- Preferred: make linking an explicit act from an authenticated password session. For example, an admin-only "link my account" that starts the provider flow, with the callback binding only to `CurrentUser`. Keep anonymous first-use binding for the fresh-install case only.
- At minimum: write an audit record (`user.identity-link`) from `bindFirstIdentity`, carrying the account but not the issuer or subject. Also add a sentence to the guide that after an unlink, whoever signs in first through the provider from the LAN becomes this account.

### WR-02: The unlink's audit record does not say which account was unlinked

**Files:**
- `internal/audit/redact.go:132-137`
- `internal/httpapi/router.go:587-603`
- `internal/httpapi/middleware/audit.go:115-120`

**Issue:** The allowlist comment justifies recording nothing with "the account is in the path". But the audit record has no path or target field: `auditAdapter.Attempt` writes Actor, Session, SrcIP, Action and Params, and Params comes only from the body.

So the archive records "admin X did `user.identity-unlink`" without the account it was done to. That is exactly the question a forensic reader has, for example "who unlinked the other admin right before somebody re-linked it". `TestUnlinkSingleSignOnAuditRecordCarriesNoIdentity` asserts only actor and outcome, so it cannot see this. `user.delete`, `user.role` and `user.password-reset` share the gap, but this change's comment asserts the opposite.

**Fix:** Record the target account. Either add a path-parameter capture to the audit middleware (an allowlisted `{id}` → `params.account`), or have the route carry the account's ID or username as a permitted param. Extend the test to assert that the record names the target and still carries neither issuer nor subject. Correct the comment.

### WR-03: Unlinking leaves alive the sessions created through the removed identity, and the sole admin has no way to end them

**Files:**
- `internal/httpapi/handlers/users.go:424-428`
- `docs/guide.md:545-549`
- `docs/api-contract.md` (unlink paragraph)

**Issue:** The unlink ends no session, including sessions that came in through the very binding being removed. Their sudo window stays open too.

When the unlink is the remedy for a wrong link (WR-01), the wrong person keeps an admin session until it expires. The guide's alternative, "To end somebody's sessions, remove the account", is refused by `refuseIfLastAdmin` for the only admin. `SetPassword` (auth/users.go:147-161) does not end sessions either. So for the common single-operator case there is no way through the product to evict that session.

**Fix:** In `UnlinkIdentity`, or in the handler, end the account's other sessions that carry `oidc.authenticated=true`, keeping the caller's own session. Or end all of the account's sessions except the caller's, as `ChangePassword` does. Document whichever is chosen.

### WR-04: A self-unlinked session that came in through the provider can never confirm a destructive action again

**Files:**
- `internal/httpapi/handlers/auth.go:274`
- `web/src/components/SudoDialog.tsx:56,151-195`
- `docs/guide.md:545-548`

**Issue:** `/auth/me` reports `sso: true` for the life of a session that signed in through the provider. `SudoDialog` then offers only "Continue to your provider" and never a password field. After an admin unlinks their own account in such a session:
1. Every destructive action goes through a provider round trip.
2. The round trip ends in `oidc.not-linked`.
3. The dialog never offers the password, even on the LAN where it would be accepted.

The guide says "it can no longer confirm destructive actions through the provider, and says so — the password on the local network still can". In that session, it cannot. The operator has to sign out and sign in again with the password.

**Fix:** Report `sso` only while the account is still linked, for example `SSO: flag && u.HasIdentityBinding()`, so the dialog falls back to the password form. Or give the provider branch of the dialog a "use password instead" option on hosts that accept passwords. Add a test for the self-unlink-then-sudo path.

### WR-05: With only service accounts left, the answer is `setup-required`, but setup refuses to run

**Files:**
- `internal/httpapi/handlers/oidc.go:199-201`
- `internal/httpapi/handlers/oidc.go:418-419`
- `web/src/routes/login.tsx:255-256`
- `docs/guide.md:514`
- `docs/api-contract.md:3570`

**Issue:** When there is no person account but service accounts exist (reachable via `refuseIfLastAdmin`, see CR-01), both the pre-check and `bindFirstIdentity` answer `setup-required`. The page then says "This instance has no operator account yet. It has to be created from the local network." But the setup handler refuses as soon as any account exists (setup.go:118-128, `setup.already-completed`), so the stated remedy cannot work.

The docs also disagree with each other:
- The guide says `setup-required` means "there is no account at all".
- The contract says it means there is no person account.

**Fix:** Either give the service-only case its own code (for example `no-person`) with the real remedy, or keep `setup-required` only when `Users().List` is empty. The real remedy: an admin, possibly using a service-account token, creates a person account. Align the guide and the contract.

### WR-06: The SSO-only pre-check tells an anonymous caller more than finishing the flow would, and the comment says otherwise

**Files:**
- `internal/httpapi/handlers/oidc.go:173-210`
- `docs/api-contract.md:3568-3574`

**Issue:** The new comment and the contract claim that the pre-check "discloses nothing the finished flow would not". On an SSO-only host the finished callback answers every identity that is not linked with `bind-host`: `errBindFromUntrustedHost` is returned before anything is counted. That holds whether there are zero people, people with no link, or another person who is linked. The pre-check splits those cases apart:
- **Zero people:** `setup-required`, which reveals that no person account exists. The finished flow would say `bind-host`.
- **People, none linked:** `bind-host`.
- **Some person linked:** the flow proceeds. The caller learns whether anybody here is linked.

Any anonymous caller on the public address learns this without a provider account, so they could not have finished the flow at all. On instances with several people this is new: before the change, they always got a uniform `setup-required`. An outsider can also watch the answer flip from "proceed" to `bind-host` at the moment an admin unlinks the last linked person, which is the opening of the trust-on-first-use window in WR-01.

**Fix:** At minimum, correct the comment and the contract so they state what is disclosed. Better: answer the public host's pre-check with one indistinguishable outcome. Either always proceed and let the callback say `bind-host`, or say `bind-host` both when nobody is linked and when there are no people, instead of `setup-required`.

### WR-07: Store revision conflicts surface as 500s, as a raw problem document on a browser navigation

**Files:**
- `internal/auth/identity.go:83-86`
- `internal/httpapi/handlers/oidc.go:445-449`
- `internal/httpapi/handlers/users.go:420`
- `internal/httpapi/handlers/users.go:433-460`

**Issue:** Verified with a probe: two concurrent first-use binds are correctly prevented from both binding. The loser's `BindIdentity` returns `store: revision conflict`, which `writeBindProblem` sends to `default` → `WriteInternal`. On a browser navigation that renders raw JSON, the very failure `failSignIn`'s comment exists to avoid.

An unlink that races a role change or password reset on the same account also gets `ErrConflict` → `writeUserError` default → 500 `internal.unexpected`. It is logged as a bug and audited as an error, although it is an ordinary conflict.

**Fix:**
- Map `store.ErrConflict` in `writeUserError` to a 409 (for example `conflict.changed`, "the account changed meanwhile; reload and retry").
- In `BindIdentity`, re-read once on `ErrConflict`. If the account is now bound to the same identity, return it; if to a different one, return `ErrAlreadyBound`. The loser then lands on `other-identity` instead of a 500.

## Info

### IN-01: `BindIdentity` returns the stored record with its password hash

**File:** `internal/auth/identity.go:76,83-87`

**Issue:** Both returns hand back the re-read record with `PasswordHash` set. `UnlinkIdentity` strips the hash, and `PersonAccounts` documents stripping. No current caller leaks it (the hash goes to `StartSession` and a log of the username), but the two sibling operations are inconsistent.

**Fix:** Set `bound.PasswordHash = ""` (and the same on the early return) before returning.

### IN-02: The pre-check's error branch answers a navigation with a problem document

**File:** `internal/httpapi/handlers/oidc.go:194-197`

**Issue:** `WriteInternal` on a GET that the browser navigates to renders JSON in the address bar, which `failSignIn`'s comment calls out as the failure to avoid. The old code answered this case with `setup-required`.

**Fix:** Log the error and use `failSignIn(w, r, "unavailable")` or a similar code. Or deliberately let the flow proceed and leave the decision to the callback.

### IN-03: The sudo prompt does not name the unlink

**File:** `web/src/api.ts:635-678`

**Issue:** `ACTION_LABELS` has no entry for `DELETE /api/v1/users/{id}/identity`, so the sudo dialog and the after-provider banner say "This destructive action". The table's own comment records that this generic sentence confused the operator before.

**Fix:** Add `{ match: /^\/api\/v1\/users\/[^/]+\/identity$/, action: 'Unlink single sign-on for this account' }`.

### IN-04: A failed unlink's error stays in the dialog after it is closed and reopened

**File:** `web/src/routes/accounts.tsx` (`UnlinkSingleSignOn`, the `unlink.error` render and `onClick={() => setOpen(true)}`)

**Issue:** The mutation is never reset. Cancel and reopen still show the previous `Problem`, for example a 409 from a race, above a fresh confirmation.

**Fix:** Call `unlink.reset()` when the dialog opens, or in `onOpenChange` when it closes.

### IN-05: The redirect codes, including the new `oidc.not-linked`, are not in the API contract

**File:** `docs/api-contract.md` (OIDC section)

**Issue:** `sudo_error=oidc.not-linked` is a new client-visible code that only `ResumeAfterProvider.tsx` knows about. The other `sudo_error` and `sso_error` codes are likewise undocumented, a gap that predates this change. `TestEveryProblemCodeIsInTheContract` covers problem codes only, so nothing catches it.

**Fix:** Add a table of the `sso_error` and `sudo_error` codes and their meanings next to the SSO-only paragraph.

---

_Reviewed: 2026-10-01T19:33:06Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
