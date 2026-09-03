---
phase: 01-foundation-skeleton
plan: 04
subsystem: auth
tags: [go, argon2id, phc, calibration, scs, session-rotation, sudo-mode, rate-limit, exponential-backoff, csrf, rfc9457]

# Dependency graph
requires:
  - phase: 01-01
    provides: "auth.Service over scs with session rotation on login, the fail-closed sudo gate, the Route type with Destructive and RequiresSession, the closed problem taxonomy including sudo-required and rate-limited, and the CSRF middleware with two of its three preconditions"
  - phase: 01-02
    provides: "the 0700/0600 permission contract and the refcounted per-entity locks the session store writes through"
  - phase: 01-03
    provides: "audit.ShortSession at the seal point, the allowlist redactor, and the middleware that records every mutation"
provides:
  - "auth.CalibrateParams / auth.ActiveParams: argon2id calibrated to a measured 250ms verification, floored at DefaultParams and never below it"
  - "auth.NeedsRehash plus rehash-on-login: raising the cost upgrades stored hashes without invalidating a password"
  - "absolute 24h session lifetime anchored on authenticated_at, provably unmoved by activity"
  - "auth.InvalidateAllExcept: the password change drops every session but the calling one"
  - "auth.OpenSudoWindow / IsSudoOpen / TouchSudoWindow: the five-minute window as session state only"
  - "middleware.Sudo in final form: reads Route.Destructive, answers 428, and refreshes the window after a successful action"
  - "auth.Limiter: per-source-IP delay doubling from 250ms to a 30s ceiling, with no lock and therefore no unlock path"
  - "middleware.CSRF in final form: all three preconditions, including the header's value and the Origin's scheme"
  - "POST /api/v1/account/password (Destructive: true) and POST /api/v1/auth/sudo"
affects: [01-05, 01-06, 06-jobs, 09-upgrades]

actuals:
  tokens: 30782
  tasks: 3
  commits: 6

tech-stack:
  added: []
  patterns:
    - "argon2id parameters are calibrated at start, floored at DefaultParams, and carried only in the PHC string"
    - "A cost target is measured as the fastest of several runs, so a cold sample cannot under-calibrate"
    - "Session lifetime is absolute and anchored on an explicit authenticated_at stamp, not on library defaults alone"
    - "Sudo state lives in the session record and nowhere else; there is no process-wide register"
    - "A middleware that must act on the session after the handler holds the response until it has"
    - "Rate limiting is a delay with a ceiling and no terminal state, so no recovery path is needed or possible"
    - "The source address for rate limiting is RemoteAddr only; proxy headers are never consulted"

key-files:
  created:
    - internal/auth/argon.go
    - internal/auth/argon_test.go
    - internal/auth/session.go
    - internal/auth/session_test.go
    - internal/auth/sudo.go
    - internal/auth/sudo_test.go
    - internal/auth/ratelimit.go
    - internal/auth/ratelimit_test.go
    - internal/httpapi/middleware/sudo_test.go
    - internal/httpapi/middleware/csrf_test.go
    - internal/httpapi/handlers/account_test.go
  modified:
    - internal/auth/auth.go
    - internal/httpapi/middleware/sudo.go
    - internal/httpapi/middleware/csrf.go
    - internal/httpapi/handlers/auth.go
    - internal/httpapi/handlers/account.go
    - internal/httpapi/router.go
    - internal/audit/redact.go
    - docs/api-contract.md

key-decisions:
  - "A password change invalidates every session except the calling one (the question CONTEXT.md left to discretion): the change is nearly always a reaction to a suspicion, and logging the operator out of the browser they are fixing things in punishes the right instinct"
  - "Logging in does NOT open the sudo window; only POST /api/v1/auth/sudo does. The plan's own acceptance criteria require a freshly logged-in second session to receive 428, which is only true if login leaves the window shut"
  - "github.com/justinas/nosurf was again NOT added, and the plan sentence claiming it is present was corrected rather than satisfied"
  - "The X-Holzkube-CSRF header's VALUE is now checked, not merely its presence, and an Origin must match the request scheme as well as its host"
  - "The sudo middleware buffers the response until the window has been refreshed, because scs commits the session on the first write and a post-handler mutation is otherwise discarded"
  - "The rate limiter sweeps idle sources inline from Fail instead of running a background reaper, so there is no goroutine lifetime to own"
  - "Calibration measures the fastest of three runs; the first implementation measured once and shipped parameters 20% below the target"
  - "argon2id calibration raises iterations only, never memory, and can never return below DefaultParams"

patterns-established:
  - "A new destructive route sets Destructive: true in its own group in the route literal, so gofmt's column alignment cannot hide the flag from a grep or a reviewer"
  - "Any middleware that needs to write session state after the handler must hold the response; the session manager persists on the first write"
  - "A cost or timing target is asserted by measuring it in a test, never by asserting the parameters that were supposed to produce it"
  - "A new password-checking endpoint shares the one Limiter created in its Routes function; a second counter would be a second door"

requirements-completed: [FOUND-02, FOUND-03, FOUND-04]

coverage:
  - id: D1
    description: "Login verifies with argon2id whose parameters are carried in the stored hash and calibrated to a measured 250ms, so the cost can be raised later without invalidating any password"
    requirement: "FOUND-02"
    verification:
      - kind: unit
        ref: "internal/auth/argon_test.go#TestArgonHashCarriesItsParameters"
        status: pass
      - kind: unit
        ref: "internal/auth/argon_test.go#TestArgonVerifyCostsAtLeastTheTarget"
        status: pass
      - kind: unit
        ref: "internal/auth/argon_test.go#TestArgonCalibrationNeverGoesBelowTheFloor"
        status: pass
      - kind: unit
        ref: "internal/auth/argon_test.go#TestArgonVerifiesAHashMadeWithOlderParameters"
        status: pass
      - kind: unit
        ref: "internal/auth/argon_test.go#TestLoginRehashesAnOutdatedHash"
        status: pass
      - kind: manual_procedural
        ref: "live binary: the stored users record carries m=65536,t=26,p=4 in its PHC string after a real setup"
        status: pass
    human_judgment: false
  - id: D2
    description: "The session id rotates on login, the old id is dead, and a session dies 24 hours after it was issued no matter how much traffic it carried"
    requirement: "FOUND-02"
    verification:
      - kind: unit
        ref: "internal/auth/session_test.go#TestSessionRotatesOnLogin"
        status: pass
      - kind: unit
        ref: "internal/auth/session_test.go#TestSessionAbsoluteLimitIgnoresActivity"
        status: pass
      - kind: unit
        ref: "internal/auth/session_test.go#TestLogoutInvalidatesTheSessionServerSide"
        status: pass
      - kind: unit
        ref: "internal/auth/session_test.go#TestSessionCookieFollowsTheTransportContract"
        status: pass
      - kind: e2e
        ref: "internal/httpapi/endtoend_test.go#TestEndToEndSetupLoginAudit"
        status: pass
    human_judgment: false
  - id: D3
    description: "A destructive route answers 428 with a valid session and no open window, passes after re-authentication, refuses again once the window closes, and cannot borrow a window from another session"
    requirement: "FOUND-03"
    verification:
      - kind: unit
        ref: "internal/auth/sudo_test.go#TestSudoWindowStartsClosed"
        status: pass
      - kind: unit
        ref: "internal/auth/sudo_test.go#TestSudoWindowOpensAndExpires"
        status: pass
      - kind: unit
        ref: "internal/auth/sudo_test.go#TestSudoTouchRestartsTheWindow"
        status: pass
      - kind: unit
        ref: "internal/auth/sudo_test.go#TestSudoWindowIsPerSession"
        status: pass
      - kind: unit
        ref: "internal/httpapi/middleware/sudo_test.go#TestSudoGateRefusesADestructiveRouteWithNoWindow"
        status: pass
      - kind: unit
        ref: "internal/httpapi/middleware/sudo_test.go#TestSudoGateRestartsTheWindowAfterTheHandler"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/account_test.go#TestAccountPasswordWithoutSudoIs428"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/account_test.go#TestSudoOpensTheWindowAndTheChangeGoesThrough"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/account_test.go#TestSudoWindowExpires"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/account_test.go#TestDestructiveActionRestartsTheWindow"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/account_test.go#TestSudoWindowIsNotSharedBetweenSessions"
        status: pass
      - kind: manual_procedural
        ref: "live binary: POST /api/v1/account/password returns 428 sudo.required, POST /api/v1/auth/sudo returns 204, the same call then returns 204"
        status: pass
    human_judgment: false
  - id: D4
    description: "The declarative Destructive marking is proven on a real route and reusable by phases 6 and 9"
    requirement: "FOUND-03"
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/account_test.go#TestAccountPasswordRouteIsMarkedDestructive"
        status: pass
      - kind: unit
        ref: "internal/httpapi/middleware/sudo_test.go#TestSudoGateIgnoresRoutesThatAreNotDestructive"
        status: pass
      - kind: other
        ref: "grep -c 'Destructive: true' internal/httpapi/handlers/account.go returns 1"
        status: pass
    human_judgment: false
  - id: D5
    description: "Repeated wrong passwords are delayed with a doubling wait capped at 30 seconds, and no sequence of failures can produce a state the operator cannot log in from"
    requirement: "FOUND-04"
    verification:
      - kind: unit
        ref: "internal/auth/ratelimit_test.go#TestRateLimitDoesNotDelayTheFirstAttempt"
        status: pass
      - kind: unit
        ref: "internal/auth/ratelimit_test.go#TestRateLimitGrowsExponentiallyAndStopsAt30Seconds"
        status: pass
      - kind: unit
        ref: "internal/auth/ratelimit_test.go#TestRateLimitHasNoStateThatOutlastsTheWait"
        status: pass
      - kind: unit
        ref: "internal/auth/ratelimit_test.go#TestRateLimitForgetsAfterSuccess"
        status: pass
      - kind: unit
        ref: "internal/auth/ratelimit_test.go#TestRateLimitIsPerSourceIP"
        status: pass
      - kind: unit
        ref: "internal/auth/ratelimit_test.go#TestRateLimitForgetsIdleSources"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/account_test.go#TestRateLimitSlowsGuessingAndNeverLocksOut"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/account_test.go#TestRateLimitCoversTheSudoReAuthentication"
        status: pass
      - kind: other
        ref: "grep -icE 'func .*(Unlock|Unban|ClearLockout|IsLocked)' and 'lockout|locked_until|banned' over internal/auth/ratelimit.go both return 0"
        status: pass
      - kind: manual_procedural
        ref: "live binary: four wrong logins give 401, 401, 401, then 429 with Retry-After: 1"
        status: pass
    human_judgment: false
  - id: D6
    description: "A mutating request missing any one of the three CSRF preconditions is refused with 403 csrf.precondition-unmet, while reads pass untouched"
    verification:
      - kind: unit
        ref: "internal/httpapi/middleware/csrf_test.go#TestCSRFPreconditions"
        status: pass
      - kind: unit
        ref: "internal/httpapi/middleware/csrf_test.go#TestCSRFLetsReadsThrough"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/account_test.go#TestMutatingRequestWithoutTheCSRFHeaderIs403"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/account_test.go#TestReadingRequestNeedsNoCSRFHeader"
        status: pass
      - kind: manual_procedural
        ref: "live binary: POST /api/v1/auth/logout without X-Holzkube-CSRF returns 403 csrf.precondition-unmet"
        status: pass
    human_judgment: false
  - id: D7
    description: "No path writes a usable session token or a password into the audit archive"
    verification:
      - kind: e2e
        ref: "internal/httpapi/endtoend_test.go#TestEndToEndSetupLoginAudit (assertAuditFileHoldsNoSecret, inherited from 01-03 and still passing)"
        status: pass
      - kind: manual_procedural
        ref: "live binary: the account.password record carries session=\"R8ZNknAp…\" and params current_password/new_password both <redacted>; the live cookie value is absent from the log"
        status: pass
    human_judgment: false
  - id: D8
    description: "The operator can complete a password change from the browser, including seeing and answering the 428 re-authentication prompt"
    verification: []
    human_judgment: true
    rationale: "The server half is fully proven, but no UI exists for either the password change or the sudo prompt -- web/src/App.tsx is still the single-file tracer. Building and verifying that screen is plan 01-05's scope; nothing here can assert it."

# Metrics
duration: 22 min
completed: 2026-08-28
status: complete
---

# Phase 1 Plan 04: Authentication Policy Summary

**argon2id calibrated to a measured 250 ms with its parameters carried in the PHC hash and upgraded on login, an absolute 24-hour session that activity cannot extend, a five-minute per-session sudo window that gates the project's first `Destructive: true` route with 428, and a login delay that doubles to a 30-second ceiling without ever creating a state the single operator could be stranded in.**

## Performance

- **Duration:** 22 min
- **Started:** 2026-08-28T02:19:00Z
- **Completed:** 2026-08-28T02:41:45Z
- **Tasks:** 3 (each RED then GREEN)
- **Files changed:** 19 (11 created, 8 modified)

## Accomplishments

- **The cost of a password is measured, not asserted.** `CalibrateParams` raises iterations until a verification actually takes 250 ms on this host, and a test times a real `Verify` rather than checking that the parameters look expensive. The first implementation of that calibration shipped parameters 20 % *below* the target and the timing test caught it — see Issues.
- **FOUND-02's "parameters stored with the hash" is now load-bearing rather than incidental.** `NeedsRehash` reads them back out of the PHC string, and a login with an outdated hash silently upgrades it. Raising the cost is a one-line change with no migration and no invalidated password.
- **The 24-hour limit is absolute in a way a test can prove.** Twenty simulated hours of continuous traffic leave the stored expiry byte-identical, and the session is dead at hour 25.
- **The sudo window works, and works for the right reason.** It lives in the session record only, so a second browser logged in as the same operator gets 428 while the first one is inside its window. A successful destructive action restarts it; a failed one does not.
- **`POST /api/v1/account/password` is the project's first `Destructive: true` route**, proven end-to-end against a real binary, which is what phases 6 and 9 inherit.
- **Brute force is slow and the operator cannot be stranded.** Four wrong guesses against the live binary give 401, 401, 401, 429 `Retry-After: 1`; waiting is always sufficient, and there is no unlock function, endpoint or flag — asserted by two greps and by a test that guesses a hundred times and then logs in.
- **The CSRF preconditions are now three, strictly.** The custom header's *value* is checked, and an `Origin` must match the request's scheme as well as its host.

## Task Commits

1. **Task 1 RED — argon2id, session policy, destructive route** — `6de695b` (test)
2. **Task 1 GREEN** — `5037e91` (feat)
3. **Task 2 RED — the sudo window and its gate** — `fbee482` (test)
4. **Task 2 GREEN** — `cf734fa` (feat)
5. **Task 3 RED — login delay and CSRF preconditions** — `45a3e47` (test)
6. **Task 3 GREEN** — `f5ad031` (feat)

## Files Created/Modified

**`internal/auth` — the package in its v1 form, split by concern**

- `argon.go` *(new)* — `DefaultParams` as a floor, `CalibrateParams`, `ActiveParams`, `Hash`, `Verify`, `NeedsRehash`. Calibration raises iterations only and measures fastest-of-three.
- `session.go` *(new)* — the session manager's configuration, the `authenticated_at` anchor, `withinAbsoluteLifetime`, `SessionID`, `InvalidateAllExcept`.
- `sudo.go` *(new)* — `DefaultSudoWindow`, `OpenSudoWindow`, `IsSudoOpen`, `TouchSudoWindow`. Session state only.
- `ratelimit.go` *(new)* — `Limiter` with `Delay`/`Fail`/`Succeed`/`Len`, doubling backoff, ceiling, inline sweep.
- `auth.go` — reduced to the service core: `New` (which calibrates at start), `verifyPassword` with the decoy path, `Login` with rotation-then-identity and rehash, `ChangePassword`, `CurrentUser`.

**`internal/httpapi`**

- `middleware/sudo.go` — final form: reads `Route.Destructive`, denies with 428, holds the response until the window is refreshed.
- `middleware/csrf.go` — final form: three preconditions, header value and origin scheme included, with the nosurf reasoning in the file header.
- `handlers/account.go` — `AccountRoutes` with `POST /api/v1/account/password`, `Destructive: true`.
- `handlers/auth.go` — `POST /api/v1/auth/sudo`, plus the shared `Limiter` and the `throttle` helper both password endpoints call.
- `router.go` — two lines: the sudo callbacks now carry the configured window and the refresh.

**Elsewhere**

- `internal/audit/redact.go` — one line: `auth.sudo` listed with nothing permitted.
- `docs/api-contract.md` — the sudo window and the login delay as a client sees them; the two tightened CSRF clauses.

## Decisions Made

- **A password change keeps the calling session and drops every other one.** `01-CONTEXT.md` § *Claude's Discretion* left this open. In a single-operator tool the change is nearly always a reaction to a suspicion; the sessions that might not be the operator's are exactly the other ones, and ending the operator's own session punishes the correct instinct. `TestChangePasswordInvalidatesEveryOtherSession` pins both halves.
- **Logging in does not open the sudo window.** `ARCHITECTURE.md:820` phrases the gate as "if the session is older than N minutes", which would let a fresh login through. The plan's own acceptance criteria contradict that: a second session that has *just logged in* must receive 428. The stricter reading is implemented, and it is the one that makes the window mean something — otherwise anyone who steals a cookie within five minutes of a login inherits the authorisation too.
- **Calibration may raise the cost and may never lower it.** `CalibrateParams` starts from `DefaultParams` and only increases iterations; memory is left alone because it is what makes argon2id expensive to attack with custom hardware. On a host too slow to reach 250 ms it logs a warning naming the measured cost instead of quietly accepting it.
- **The rate limiter's source is `RemoteAddr` and nothing else.** Honouring `X-Forwarded-For` without a trusted proxy would let a guesser reset their own counter on every attempt, which is worse than having no limit because it looks like having one. `middleware.ClientIP`, which 01-01 wrote for the audit trail with the same reasoning, is reused rather than duplicated.
- **One `Limiter` per `AuthRoutes` call, shared by login and sudo.** Both check the same password against the same account; a second counter would be a second door.
- **A malformed login body is not counted as a failure.** It answers 401 like everything else on that path, but it cannot guess a password, so counting it would only let a broken client slow the operator down.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Calibration measured once and shipped parameters below the target**

- **Found during:** Task 1 (GREEN)
- **Issue:** The first `measureHash` took a single sample. The very first argon2id run in a process is inflated by cold cache and CPU frequency scaling, so calibration stopped at `t=18` believing it had reached 253 ms; a steady-state verification with those parameters took **206 ms**. The security target was missed by 18 % and the mechanism reported success. `TestArgonVerifyCostsAtLeastTheTarget` failed, which is exactly why it measures a real verification instead of inspecting the parameters.
- **Fix:** `measureHash` reports the fastest of three runs, so a noisy sample errs towards *more* iterations. A 1.2 safety margin was added to the scaling step for the same reason.
- **Verification:** The test now measures the fastest of three real verifications and requires ≥ 250 ms; the live binary calibrated to `t=26` at 296 ms.
- **Committed in:** `5037e91`

**2. [Rule 1 - Bug] The sudo window's refresh was being computed and discarded**

- **Found during:** Task 2 (GREEN)
- **Issue:** The plan specifies that the gate calls the handler and refreshes the window **afterwards**. Implemented literally, this does nothing: `scs` v2.9.0 wraps the response in a `sessionResponseWriter` that commits the session on the **first write**, so any session value set after the handler has answered is computed, never persisted and never noticed. The failure is invisible — the request succeeds, and the operator is simply asked for their password again mid-series. `TestDestructiveActionRestartsTheWindow` caught it; without that test the feature would have shipped looking correct.
- **Fix:** The sudo middleware buffers the response (`heldResponse`) and flushes it after the refresh. Buffering is safe in this link and nowhere else: it wraps destructive routes, whose answers are a status code and at most a few hundred bytes of `problem+json`, and nothing that streams is destructive. The reasoning is recorded at the call site so it is not "simplified" away later.
- **Verification:** `TestSudoGateRestartsTheWindowAfterTheHandler` asserts the ordering directly; `TestDestructiveActionRestartsTheWindow` performs a second destructive action after the original window would have closed.
- **Committed in:** `cf734fa`

**3. [Rule 3 - Blocking] `internal/httpapi/router.go` edited to carry the window and the refresh**

- **Issue:** `files_modified` does not list `router.go`, but the sudo middleware's callbacks are constructed there and the old one called `d.Auth.HasSudo(ctx)` — a signature with no window length and no refresh hook. There is no route to a configurable window or to a post-action refresh that does not touch it. This is the same class of conflict 01-03 recorded as its deviation 3.
- **Fix:** Two lines in `wrapRoute`: the check became `IsSudoOpen(r.Context(), d.SudoWindow)` and a refresh callback was added. `d.SudoWindow` was already wired from config by 01-01, so nothing in `cmd/holzkubed/main.go` had to change. Routing, mounting, the chain and the registration rule are untouched.
- **Verification:** `go build ./... && go vet ./...` clean; the live binary honours `--sudo-window`.
- **Committed in:** `5037e91` and `cf734fa`

**4. [Rule 2 - Missing Critical] The CSRF header's value was not checked**

- **Found during:** Task 3
- **Issue:** `docs/api-contract.md` states the precondition as `X-Holzkube-CSRF: 1`, but the implementation accepted any non-empty value. That makes the contract a suggestion: a client sending `true` works today and breaks the day anyone reads the contract literally.
- **Fix:** `CSRFHeaderValue = "1"`, compared exactly. Verified safe against the existing client — `web/src/api.ts` line 95 sends `'1'`.
- **Verification:** `TestCSRFPreconditions/custom_header_with_the_wrong_value`.
- **Committed in:** `f5ad031`

**5. [Rule 2 - Missing Critical] An `Origin` on the wrong scheme was accepted**

- **Found during:** Task 3
- **Issue:** The origin check compared hosts only, so `http://holzkube.test` satisfied it on an HTTPS request. holzkube is HTTPS by default precisely because a plaintext page is one a network attacker can write; accepting such a page as a legitimate origin gives most of that back.
- **Fix:** The scheme is compared against the request's (`r.TLS` decides). `Origin: null` already failed the host check and is now covered by a test.
- **Verification:** `TestCSRFPreconditions/origin_on_the_wrong_scheme`, `/origin_null`, `/same-site_fetch_metadata`.
- **Committed in:** `f5ad031`

**6. [Rule 2 - Missing Critical] `internal/audit/redact.go` given an `auth.sudo` entry**

- **Issue:** A new mutating action token with no allowlist entry is already fail-closed (unlisted actions redact everything), but 01-03 established the pattern that every action is listed even when nothing may pass, so the table shows the full set of mutations rather than leaving some to the default. `internal/audit/` is 01-03's territory; the change is one line and adds no permission.
- **Fix:** `"auth.sudo": {}`.
- **Verification:** `go test ./internal/audit/... -count=1` passes; the live binary shows both password fields on the change record as `<redacted>`.
- **Committed in:** `cf734fa`

**7. [Deliberate divergence] The rate limiter sweeps inline instead of running a background reaper**

- **Issue:** The plan calls for "ein Hintergrund-Reaper". A goroutine needs a lifetime, a shutdown path and somewhere to be stopped in every test that builds a server — and a limiter is created per `AuthRoutes` call, so every test harness would leak one.
- **Fix:** `sweep` runs from `Fail`, at most once a minute. `Fail` is the only thing that adds an entry, so the map cannot grow between sweeps and the stated purpose (T-01-28) is met exactly.
- **Verification:** `TestRateLimitForgetsIdleSources`.
- **Committed in:** `f5ad031`

**8. [Plan text corrected] `github.com/justinas/nosurf` is still not a dependency**

- **Issue:** The plan's Task 3 states that nosurf "ist als Abhängigkeit vorhanden und wird eingesetzt, sobald ein Formular-Post-Pfad entsteht". It is not in `go.mod` and never was; 01-01 recorded this as its own deviation 3.
- **Fix:** Not added, and I did not conclude it is needed. The three preconditions require no token, and an unused module is stripped by `go mod tidy` on the next run — adding it would create a dependency that vanishes in CI. The reasoning is now in the header of `internal/httpapi/middleware/csrf.go` where the next reader of that sentence will find it, phrased as what *is* true rather than what the plan assumed.
- **Committed in:** `f5ad031`

**9. [Rule 3 - Blocking] `internal/auth/sudo.go` landed one task early**

- **Issue:** The plan assigns `sudo.go` to Task 2, but Task 1 removes `HasSudo`/`GrantSudo` from `auth.go` as part of splitting the package by concern. Leaving Task 1's tree with a `router.go` calling a function that no longer exists would have meant committing something that does not compile.
- **Fix:** `sudo.go` was created in Task 1's GREEN commit with the window API; Task 2 added the endpoint, the middleware refresh and all the sudo tests, and its RED commit failed for the right reasons (no `/api/v1/auth/sudo`, no refresh hook). No behaviour moved between tasks.
- **Committed in:** `5037e91`

---

**Total deviations:** 9 (2 Rule 1 bugs, 2 Rule 3 blocking, 3 Rule 2 missing-critical, 1 deliberate divergence, 1 plan-text correction).
**Impact on plan:** No scope creep; every item is required for one of the plan's own success criteria. Deviations 1 and 2 are the important ones: both are mechanisms that would have shipped *looking* correct — a cost target reported as met while being missed by 18 %, and a window refresh computed and thrown away — and both were caught only because the tests measure behaviour instead of configuration. Three files outside `files_modified` were touched: `internal/httpapi/router.go` (two lines), `internal/audit/redact.go` (one line) and `docs/api-contract.md` (documentation of shapes 01-05 consumes). None of them overlaps 01-05's or 01-06's declared files.

## Contract changes 01-05 must read

`docs/api-contract.md` gained two subsections under *Routes* and tightened two clauses under *CSRF Contract*. Nothing already-specified changed shape, but three behaviours are now stated that 01-05 has to build against:

1. **Login does not open the sudo window.** The client flow is: call the action → on `428`, prompt for the password → `POST /api/v1/auth/sudo` → on `204`, retry the original call. A wrong password there is `401`, not `428`.
2. **`429` `ratelimit.delayed` carries `Retry-After` in seconds** on both `/auth/login` and `/auth/sudo`. There is no unlock affordance to build, and none may be added.
3. **`X-Holzkube-CSRF` must be exactly `1`**, and an `Origin`, if sent, must match the request's scheme. `web/src/api.ts` already satisfies both; a rewrite of that file must keep doing so.

## Issues Encountered

- **The `Destructive: true` acceptance grep failed on first run — the same trap 01-01 hit.** `gofmt` column-aligns neighbouring struct fields, so the flag rendered as `Destructive:     true` and the literal string never appeared. Rather than weaken the check, the field was given its own group in the route literal with a comment saying why, which reads better anyway and is now recorded as a pattern for every destructive route phases 6 and 9 add.
- **`grep -c 'IdleTimeout'` on `session.go` must be zero, and the reason the option is unset is worth writing down.** The comment explains that the session manager's inactivity-expiry option is deliberately left at its zero value without naming the identifier, so the gate and the explanation coexist. Same resolution 01-03 used for its two colliding greps.
- **argon2id at the required cost makes the HTTP test suite genuinely slow.** `internal/httpapi/handlers` takes ~17 s (~28 s under `-race`) because every login is a real 300 ms verification. Tests inside `package auth` swap in cheap parameters through a test-file-only hook — production code has no way to lower the cost — but tests outside the package cannot, and should not.
- **The shell's `grep` wrapper made the acceptance greps unreadable.** Run through `/usr/bin/grep` for the gate checks. A verification artefact, not a defect.

## Known Stubs

None introduced by this plan. Two pre-existing items remain and are unchanged:

| Stub | File | Resolved by |
|---|---|---|
| `problem.Forbidden` and `problem.UnsupportedMediaType` minted but not emitted | `internal/httpapi/problem.go` | Reserved by design — the taxonomy is a closed contract |
| No UI for the password change or the sudo prompt | `web/src/App.tsx` | Plan 01-05 (tracked here as coverage item D8) |

## Threat Flags

None. Every threat this plan registered is mitigated and tested:

| Threat | Mitigation | Proven by |
|---|---|---|
| T-01-23 brute force | argon2id ≥ 250 ms measured, plus a doubling per-IP delay capped at 30 s with `429`/`Retry-After` | `TestArgonVerifyCostsAtLeastTheTarget`, `TestRateLimitGrowsExponentiallyAndStopsAt30Seconds`, live binary |
| T-01-24 session fixation | `RenewToken` before the identity is written; the old record is deleted | `TestSessionRotatesOnLogin` |
| T-01-25 stolen cookie on a destructive route | per-session five-minute window, declaratively gated, `428` when shut | `TestSudoWindowIsPerSession`, `TestSudoWindowIsNotSharedBetweenSessions`, live binary |
| T-01-26 cross-site POST | three simultaneous preconditions, now including header value and origin scheme | `TestCSRFPreconditions` (15 cases) |
| T-01-27 operator strands themselves | delay only; no terminal state and therefore no recovery path | `TestRateLimitHasNoStateThatOutlastsTheWait`, two absence greps |
| T-01-28 limiter map growth | inline sweep of sources idle for an hour | `TestRateLimitForgetsIdleSources` |
| T-01-29 forged `X-Forwarded-For` | source is `RemoteAddr` only, via `middleware.ClientIP` | code + `TestRateLimitIsPerSourceIP` |
| T-01-30 user enumeration | decoy verification for an unknown user; one `Unauthenticated()` with no arguments | inherited from 01-01, unchanged |

No file created here introduces network, auth, file-access or trust-boundary surface beyond the register.

## Prohibition compliance

The plan's one prohibition — that rate limiting never becomes a hard lock and that no unlock, recovery or emergency-access path is retrofitted — is held three ways: by construction (the limiter stores only a counter and a timestamp, and `Delay` returns zero once the timestamp passes), by test (`TestRateLimitHasNoStateThatOutlastsTheWait` guesses a hundred times, waits, and logs in), and by the two absence greps, which return 0. The reasoning is recorded at the top of `internal/auth/ratelimit.go` addressed to a future agent, because the risk here is not a bug but a well-meant "fix".

## User Setup Required

None — no external service configuration.

## Next Phase Readiness

**Ready.** `internal/auth` is in its v1 form and both remaining wave-2 plans have what they need:

- **Plan 05 (web UI)** — the three contract behaviours above, all response shapes unchanged, and `web/src/api.ts` already compliant with the tightened CSRF clauses. The password-change and sudo-prompt screens are the visible gap (D8).
- **Plan 06 (config/TLS/build)** — `Config.SudoWindow` and `Config.SessionLifetime` are consumed end to end; `--sudo-window` was exercised against the live binary. `golangci-lint` configuration, when it lands, should be pointed at `internal/auth`: the calibration and limiter code is where a well-meant simplification would do the most damage.

**Concerns to carry forward:**

1. **`FOUND-03` cannot be marked complete yet.** It is also declared by 01-05, which has not run. `FOUND-02` and `FOUND-04` are ready once this SUMMARY exists.
2. **Calibration runs once per process and costs roughly half a second at start.** On the target hardware it produced `t=26` at 64 MiB. An operator on much slower hardware will see the warning line rather than a silent downgrade — but nobody has run this on such hardware, so the warning path itself is untested beyond a unit test of the floor.
3. **The sudo middleware buffers destructive responses.** If a later phase ever marks a streaming route `Destructive: true`, that buffering will hold the stream. The constraint is documented at the type, and the alternative — refreshing the window before the handler — is strictly weaker.
4. **Every test client shares the source IP `127.0.0.1`**, so within one test server the rate limiter is effectively global. Tests that intersperse failed logins with other work must account for it; each test builds its own server, so today none do.

## Self-Check: PASSED

**Files claimed as created — all present on disk:**
`internal/auth/argon.go`, `internal/auth/argon_test.go`, `internal/auth/session.go`, `internal/auth/session_test.go`, `internal/auth/sudo.go`, `internal/auth/sudo_test.go`, `internal/auth/ratelimit.go`, `internal/auth/ratelimit_test.go`, `internal/httpapi/middleware/sudo_test.go`, `internal/httpapi/middleware/csrf_test.go`, `internal/httpapi/handlers/account_test.go` — FOUND.

**Commits — all six present in `git log`:** `6de695b`, `5037e91`, `fbee482`, `cf734fa`, `45a3e47`, `f5ad031` — FOUND.

**Plan-level verification re-run at close-out:**

| Check | Result |
|---|---|
| `go test ./internal/auth/... ./internal/httpapi/... -count=1 -race` | PASS |
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test ./... -count=1` (whole repo, no regression) | PASS |
| `gofmt -l ./internal ./cmd` | empty |
| Live binary: `POST /api/v1/account/password` with a valid session → `428 sudo.required` | PASS |
| Live binary: `POST /api/v1/auth/sudo` with the right password → `204`; same call then → `204` | PASS |

**Task acceptance criteria — all re-run:**

| Criterion | Result |
|---|---|
| `go test ./internal/auth/... -count=1` | PASS |
| `go test ./internal/httpapi/handlers -run TestAccount -count=1` | PASS |
| `internal/httpapi/handlers/account.go` contains `Destructive: true` | PASS (1, after the gofmt-alignment fix) |
| `grep -c 'IdleTimeout' internal/auth/session.go` is 0 | PASS (0) |
| A `Verify` takes ≥ 250 ms, measured | PASS (296 ms live) |
| A hash written with lower parameters still verifies | PASS |
| Session id differs before and after login; the old id gives 401 | PASS |
| A 25-hour-old session is refused | PASS |
| A password change makes a parallel session unusable and keeps the calling one | PASS |
| `go test ./internal/auth -run TestSudo -count=1` | PASS |
| `go test ./internal/httpapi/middleware -run TestSudo -count=1` | PASS |
| `internal/httpapi/middleware/sudo.go` references `Destructive` | PASS (1) |
| `grep -cE 'var .*sudo\|map\[.*\](time\.Time\|bool).*[Ss]udo' internal/auth/sudo.go` is 0 | PASS (0) |
| Destructive route with a session and no window → `428` `sudo.required` | PASS |
| After a correct sudo the same call passes | PASS |
| After the window elapses the same call is `428` again | PASS |
| A parallel second session still gets `428` | PASS |
| A non-destructive mutating route passes with no window | PASS |
| `go test ./internal/auth -run TestRateLimit -count=1` | PASS |
| `go test ./internal/httpapi/middleware -run TestCSRF -count=1` | PASS |
| `grep -icE 'func .*(Unlock\|Unban\|ClearLockout\|IsLocked)' internal/auth/ratelimit.go` is 0 | PASS (0) |
| `grep -icE 'lockout\|locked_until\|banned' internal/auth/ratelimit.go` is 0 | PASS (0) |
| After 100 failures the delay is exactly 30 s and a correct password still works | PASS |
| A successful login resets that IP's counter | PASS |
| Failures from IP A do not delay IP B | PASS |
| Mutating request without `X-Holzkube-CSRF: 1` → `403`; with all three → passes | PASS |
| A `GET` without the CSRF headers passes | PASS |

**Hygiene:** no file deletions in the plan's commit range; `go.mod` and `go.sum` unchanged (no dependency added or removed); `.gsd/` and `.planning/milestone.lock` untracked and gitignored; the temporary debug test used to diagnose deviation 2 was removed before its commit and is absent from the tree.

---
*Phase: 01-foundation-skeleton*
*Completed: 2026-08-28*
