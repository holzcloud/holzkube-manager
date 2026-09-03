---
phase: 01-foundation-skeleton
verified: 2026-08-28T07:05:00Z
status: passed
score: 51/51 must-haves verified
behavior_unverified: 0
overrides_applied: 0
gaps: []
deferred:

  - truth: "GET /api/v1/system/status exposes version, bind address and data directory so the dashboard can show them"
    addressed_in: "Deferred — deferred-items.md item 1; owner is a later plan or a deliberate contract extension"
    evidence: "Not a must_haves truth in any of the six plans and not a ROADMAP success criterion. The dashboard honestly omits the fields rather than inventing them (web/src/routes/index.tsx renders only setup_required and audit_chain), so there is no hollow prop. The endpoint answers before authentication and the data-directory path is the most sensitive string in the process."

  - truth: "The frontend bundle is under vite's 500 kB warning threshold"
    addressed_in: "Deferred — deferred-items.md item 2"
    evidence: "572.71 kB / 176.70 kB gzip, everything embedded in one binary by design; a decision, not a defect."
human_verification:

  - test: "Open https://127.0.0.1:8443 in a real browser, click through every sidebar entry, toggle the theme, and complete setup -> login -> change password."
    expected: "The app shell, sidebar, header, toast system and audit table render correctly and legibly in both dark and light themes; placeholder pages read as intentional."
    scope_amended: "2026-08-28 — the clause 'the sudo dialog appears on the password change and the form is not lost' was moved to Phase 10, which builds the Settings screen that owns the password change. Phase 1 ships no password-change UI (api.changePassword has no caller; /settings is a ComingSoon placeholder), so the clause asserted an interaction this phase does not ship. The sudo mechanism itself was verified in a real browser during UAT by injecting a 428 sudo.required: the dialog renders in both themes, the underlying form is byte-identical across open / wrong password / Cancel / Confirm, and Confirm replays the original request. See 01-UAT.md gap G-01-1."
    why_human: "All 60 frontend tests run under jsdom, which asserts structure and behaviour but renders nothing. Visual appearance, layout, contrast and focus handling cannot be verified programmatically."

  - test: "Start holzkubed, read the sha256_fingerprint line from the startup log, then open the URL in a browser and compare the fingerprint shown in the certificate dialog character by character."
    expected: "The two strings are identical, in the same colon-separated upper-case hex format, so the comparison needs no conversion."
    why_human: "Recorded as known gap D6. The verifier confirmed the fingerprint is byte-identical to `openssl x509 -noout -fingerprint -sha256` (D0:91:AF:...:72:C0), which is the same format browsers use, but no browser was opened. The whole value of the format is that a human can compare it in a dialog."

  - test: "Install go-task, golangci-lint v2.13.1 and goreleaser v2.18.0 (commands in deferred-items.md item 4), then run `task build`, `golangci-lint run` and `goreleaser release --snapshot --clean`."
    expected: "`task build` produces bin/holzkubed with the frontend built first; golangci-lint reports zero issues under the v2 config with gosec enabled; the snapshot archives contain the binary, README.md and docs/*."
    why_human: "None of the three tools is on this host (deferred-items.md item 4 — 01-06 installed them into a scratch GOBIN and did not leave them installed). The verifier reproduced the build chain manually (npm run build -> byte-identical bundle -> go build -> working UI) and asserted the Taskfile and goreleaser ordering edges structurally, but the three gates themselves were never executed here."
---


> **Status set to `passed` on 2026-08-28 after UAT.** Two of the three human items were
> discharged: the build/lint/release gates were run for real with the pinned tools, and the
> browser render was verified in headless Chromium across both themes and every route.
>
> One item is **not** discharged and is deliberately not being claimed: comparing the
> certificate fingerprint inside the browser's own certificate dialog. It is recorded as
> `skipped` in `01-UAT.md` with its reason. Every machine-checkable half of it is confirmed
> — the logged fingerprint is byte-identical to `openssl` on cert.pem and to the certificate
> served over the live socket, in the browser's colon-separated upper-case hex format — so
> what remains is a person looking at a native OS dialog, which no automation can reach.
>
> The clause about the sudo dialog on a password change was moved to Phase 10; see the
> scope note on human item 1 and gap G-01-1.

# Phase 1: Foundation Skeleton Verification Report

**Phase Goal:** Der Betreiber startet ein einzelnes Binary, meldet sich an, und jede Mutation ist nachvollziehbar und crash-sicher gespeichert — alles ohne dass Talos existiert.
**Verified:** 2026-08-28T07:05:00Z
**Status:** human_needed
**Re-verification:** No — initial verification
**HEAD:** `a9c3e1b` (working tree clean before and after verification)

## Goal Achievement

Verification was done against the codebase and against a **running binary**, not against
the SUMMARY files. Every claim below that says "live" was produced by building
`cmd/holzkubed` into a scratch directory and exercising it over HTTPS. The repository
was never modified.

### ROADMAP Success Criteria

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| SC1 | Single binary, no runtime deps, embedded UI over HTTPS on 127.0.0.1; RFC-9457 problem+json with a stable taxonomy | ✓ VERIFIED | `go build ./cmd/holzkubed` → 11.5 MB binary; `otool -L` shows only libSystem/libresolv/CoreFoundation/Security. Live: default bind `127.0.0.1:8443`, `GET /` → 302 `/setup`, `GET /setup` → the embedded SPA shell, `/assets/index-CuG0H2d3.js` → 200, 572711 bytes. Live problem responses carry `Content-Type: application/problem+json`, an absolute `type` URI under `https://holzkube.dev/problems/`, a stable `code` and `instance: /requests/<id>`: `notfound.route`, `method.not-allowed`, `auth.unauthenticated`, `csrf.precondition-unmet`, `sudo.required`, `setup.already-completed`, `validation.failed`. All 12 taxonomy entries in `internal/httpapi/problem.go` match the table in `docs/api-contract.md` § Error Taxonomy. |
| SC2 | Login with username + password (argon2id, parameters stored); session rotated at login; repeated failures rate-limited | ✓ VERIFIED | `internal/auth/argon.go` uses the PHC encoded form, so `m=`, `t=`, `p=` and the salt travel with the hash — live user record shows `$argon2id$v=19$m=655...`. Live calibration: `argon2id calibrated target=250ms measured=296.797667ms iterations=26`. `TestSessionRotatesOnLogin` proves the id changes, the old id no longer authenticates, and the old session record is deleted. Live rate limit over 10 wrong logins: `401 401 401 429 429 429 429 429 429 429`, then the correct password returned 204 after the wait — a delay, never a lock. |
| SC3 | A destructive action demands the password again despite a valid session | ✓ VERIFIED | Live, with a valid session cookie: `POST /api/v1/account/password` → **428** `sudo.required`; `POST /api/v1/auth/sudo` with the correct password → 204; the same password change → 204. `Destructive: true` is set on the route (`internal/httpapi/handlers/account.go`) and read by `middleware.Sudo` — nothing pattern matches on the URL. |
| SC4 | Every mutation appears as an intent-before / outcome-after pair; JSONL, daily rotation, one continuous hash chain | ✓ VERIFIED | Live log after a setup/sudo/password-change/logout/login session: 14 records = **7 intent/outcome pairs**, one JSON object per line. Chain **independently recomputed** with a Python re-implementation of the canonical form (`records=14 breaks=0`), anchored at `sha256("holzkube/audit/genesis/v1") = 666cff38…` which I recomputed with `shasum`. Daily rotation, cross-boundary chain continuity and gzip-from-the-second-day are proven by `TestRotateKeepsOneFilePerDay`, `TestRotateCarriesTheChainAcrossTheDayBoundary`, `TestRotateCompressesFromTheSecondDayOn`, `TestRotateNeverRemovesAFile` (all re-run and passing). |
| SC5 | `kill -9` mid-write leaves no corrupt state; all access via `store.Machines().Get(...)`; `0600` files in a `0700` directory; a schema upgrade backs up first | ✓ VERIFIED | **16 real out-of-process `kill -9` rounds** under concurrent write load (see Behavioral Spot-Checks) — every restart came up clean, zero orphan temporaries, zero chain breaks, all 46 session records parsed, the user record intact. Live directory listing: data dir `0700`, every file `0600` including `VERSION`, `cert.pem`, `key.pem` and `holzkube.lock`. `TestNoDirectFileAccessOutsideFsstore` is an AST walk over `internal/` and `cmd/` that fails on any `os.ReadFile`/`WriteFile`/`Open`/… outside the store, audit and tlsx subtrees. Live upgrade from `VERSION=0`: a real `backups/pre-migration-0-to-1-<ts>.tar.gz` was written **before** `VERSION` was rewritten, excluding the lock file and the backups directory. |

### Observable Truths — Plan 01-01 (API surface, taxonomy, embed)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | One built binary serves the embedded web UI over HTTPS on 127.0.0.1 with no runtime dependencies | ✓ VERIFIED | See SC1. `//go:embed all:dist` in `internal/httpapi/web.go` — the `all:` prefix is present, which is what keeps Vite's `_`-prefixed asset names from being silently skipped. |
| 2 | While no operator account exists every UI route redirects to `/setup`; afterwards the setup handler actively refuses with 409 rather than merely being hidden | ✓ VERIFIED | Live before setup: `GET /` → 302 → `/setup`. Live after setup: `GET /` → 200 (no redirect) and `POST /api/v1/setup` → **409** `setup.already-completed`. The redirect is server-side in `SPAHandler`, so it holds even with a stale bundle. |
| 3 | Login returns an HttpOnly+Secure+SameSite=Lax session cookie; the session id after login differs from the one before | ✓ VERIFIED | Live cookie jar: `#HttpOnly_127.0.0.1 … TRUE … holzkube_session` (the `TRUE` column is Secure). `TestSessionCookieFollowsTheTransportContract` asserts HttpOnly, Secure, SameSite=Lax, Path=/, Persist and a 24 h lifetime. Rotation: `TestSessionRotatesOnLogin`. |
| 4 | Setup and login each produce an intent/outcome JSONL pair and the hash chain over all lines verifies | ✓ VERIFIED | Live: `setup.create attempt` → `setup.create success`, `auth.login attempt` → `auth.login success`; independent chain recomputation reports 0 breaks. |
| 5 | Every API error is `application/problem+json` with an absolute `type` URI from the taxonomy and a stable `code` | ✓ VERIFIED | See SC1. |
| 6 | No package above `internal/store/fsstore/` reads or writes state files directly | ✓ VERIFIED | `TestNoDirectFileAccessOutsideFsstore` (re-run, passes). It also guards itself: it fails if it scanned fewer than 15 files, so a broken relative root cannot make it pass vacuously. |
| 7 | Two concurrent requests hitting the same error type produce independent problems with their own instance / request id; no field leaks between requests | ✓ VERIFIED | `TestProblemInstancesAreDistinctUnderConcurrency` — 32 goroutines, 32 distinct `/requests/<id>` values (re-run, passes). |

### Observable Truths — Plan 01-02 (store hardening)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `kill -9` mid-write leaves either the whole old or the whole new record, never a half-written file and never an orphan temporary read as state | ✓ VERIFIED | `TestCrashInject` covers all three interruption points in-process (re-run under `-race`, passes) **and** 16 real out-of-process kills confirmed the same property end to end. `writeAtomic` is tmp-in-same-dir → chmod 0600 → write → fsync(file) → rename → fsync(dir); `sweepTempFiles` runs in `Open` before anything is read. |
| 2 | A second Put of an identical record with the correct rev produces the same persisted bytes and increments rev by exactly one; a stale rev is rejected with ErrConflict rather than overwriting | ✓ VERIFIED | `TestRepeatedPutIsByteStable` (bytes compared after normalising only the rev token; asserts `rev+1`), `TestPutWithStaleRevIsRejectedNotOverwritten`. |
| 3 | Two concurrent Puts on the same entity serialise on the per-entity mutex; exactly one wins, the other gets ErrConflict, and the file afterwards holds exactly one whole record | ✓ VERIFIED | `TestConcurrentPutSameIDYieldsExactlyOneWinner` and `TestConcurrentGetDuringPutNeverSeesAPartialRecord`, both `-race` clean. |
| 4 | A second process on the same data directory refuses to start with a naming error, because the first holds the flock | ✓ VERIFIED | Live: `holzkubed: store: data directory already in use by another process: <dir> is locked by pid 23972`. |
| 5 | State carries a schema version; an upgrade writes a backup before the first change and a VERSION newer than the binary refuses to start | ✓ VERIFIED | Live `VERSION=0` → real backup tarball then `VERSION=1`. Live `VERSION=2` → `migrate: data directory is newer than this binary … upgrade holzkube`. Live `VERSION=banana` → `migrate: VERSION is not a schema version`. |
| 6 | A start against a data directory with too-wide permissions is refused rather than silently accepted | ✓ VERIFIED | Live against a `0755` dir with a `0644` record: all three violations reported at once with a `chmod` repair line, and nothing was auto-repaired. |
| 7 | No package above `internal/store/fsstore/` touches state files; the architecture guard is an executed test, not a README sentence | ✓ VERIFIED | `TestNoDirectFileAccessOutsideFsstore`. **Note:** the known-gaps note says this is "held by review and a depguard rule". That understates it — `.golangci.yml` enables no `depguard` linter at all; the actual enforcement is the AST guard above, which is stronger than the claim. It exempts `internal/store`, `internal/tlsx` and `internal/audit` as subtrees (see Anti-Patterns, W-1). |

### Observable Truths — Plan 01-03 (audit log)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Every mutation appears as an intent record (`outcome=attempt`, fsynced before the action) and an outcome record afterwards | ✓ VERIFIED | Live 7/7 pairs. `Logger.append` calls `l.file.Sync()` per record; the middleware refuses the request if the intent cannot be recorded (`TestAuditRefusesWhenTheIntentCannotBeRecorded`). |
| 2 | An intent without an outcome stays in the log and is findable through the query API — a crashed operation is not tidied away | ✓ VERIFIED | `TestAuditLeavesAnIntentWithoutAnOutcomeOnPanic`; there is no code path that completes an attempt after the fact. Live audit API returned an `attempt` record on its own page. |
| 3 | The log rotates daily and the chain runs across the file boundary, because the first record of a new day carries the previous day's last hash as `prev_hash` | ✓ VERIFIED | `TestRotateCarriesTheChainAcrossTheDayBoundary`, `TestRotateResumesFromACompressedTail`, `TestChainVerifiesAcrossCompressedAndPlainFiles` (re-run, pass). |
| 4 | Rotated files are gzipped from the second day on and are never deleted | ✓ VERIFIED | `TestRotateCompressesFromTheSecondDayOn`, `TestRotateNeverRemovesAFile` (re-run, pass). `keepPlain = 2`; `compressFile` removes the plain original only after the `.gz` is renamed into place and fsynced. |
| 5 | At startup the current and last rotated file are verified; a break shows permanently in `GET /api/v1/system/status` with file and line | ✓ VERIFIED | **Live tamper test:** editing `actor` on line 42 produced `level=ERROR msg="audit hash chain does not verify" file=… broken_at_line=42` at startup and `{"audit_chain":{"ok":false,"broken_at_line":42,…}}` from the endpoint. My independent Python verifier agreed on the exact line. |
| 6 | Input parameters appear only through an allowlist; an unlisted parameter is replaced by a redaction marker rather than passed through | ✓ VERIFIED | Live: `setup.create` and `auth.login` recorded `"password": "<redacted>"` while `username` survived; `account.password` recorded both `current_password` and `new_password` as `<redacted>`. `grep` for the two real passwords across the live log: **0 occurrences**. |

### Observable Truths — Plan 01-04 (authentication policy)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Login verifies with argon2id whose parameters travel inside the stored hash, so they can be raised later without invalidating passwords | ✓ VERIFIED | PHC encoding; `Verify` uses the parameters the hash was written with; `NeedsRehash` + `rehashIfOutdated` only ever upgrade. `TestArgonVerifiesAHashMadeWithOlderParameters`, `TestLoginRehashesAnOutdatedHash`. |
| 2 | A single password verification costs at least 250 ms on the target host | ✓ VERIFIED | Live: `measured=296.797667ms` at `iterations=26`. `TestArgonVerifyCostsAtLeastTheTarget` measures a real verification rather than inspecting parameters. The under-calibration defect is fixed: `measureHash` takes the fastest of three samples and the scaling step carries a 1.2 margin. |
| 3 | The session id before and after a successful login differ, and the old id is then invalid | ✓ VERIFIED | `TestSessionRotatesOnLogin` — the old record is gone from the store and no longer authenticates. Live cross-check: an old token replayed against `/api/v1/auth/me` returned 401. |
| 4 | A session expires at most 24 h after creation, regardless of activity | ✓ VERIFIED | `TestSessionAbsoluteLimitIgnoresActivity` (injected clock). `sm.IdleTimeout` is deliberately never assigned; `withinAbsoluteLifetime` anchors on `authenticated_at` alone and treats a missing stamp as expired. |
| 5 | A route marked destructive answers 428 despite a valid session while the sudo window is shut; after re-authentication the same call goes through | ✓ VERIFIED | Live 428 → sudo 204 → password change 204. |
| 6 | The sudo window lasts 5 minutes and is restarted by each destructive action | ✓ VERIFIED | `TestSudoWindowExpires`, `TestDestructiveActionRestartsTheWindow`, `TestSudoGateRestartsTheWindowAfterTheHandler`, `TestSudoGateDoesNotRefreshOnAFailedAction`. The discarded-refresh defect is fixed: `middleware.Sudo` buffers the response in `heldResponse` and refreshes before flushing, because scs commits the session on the first write. |
| 7 | Repeated failures from the same source IP are delayed exponentially, capped at 30 s — and never produce a state the operator cannot log in from | ✓ VERIFIED | Live: `401 401 401 429×7`, then 204 with the correct password after the wait. `backoff` is a loop, not a shift, so a hundred failures cannot overflow into a negative duration. `Succeed` deletes the source. There is no lock, unlock, recovery or emergency-access path anywhere in `internal/auth`. |
| 8 | A mutating request missing any of the three CSRF preconditions is rejected even with a valid session cookie | ✓ VERIFIED | Live, each in isolation: missing header → 403 `missing X-Holzkube-CSRF header`; `X-Holzkube-CSRF: true` → 403 `must be 1`; `Origin: https://evil.example` → 403 `does not match this host`; `Sec-Fetch-Site: cross-site` → 403 `expected same-origin`. |

### Observable Truths — Plan 01-05 (web interface)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | The permanent app shell stands: sidebar with every later area, header with user and logout, toasts, error page, TanStack router tree | ✓ VERIFIED | `__root.tsx` (createRootRoute + errorComponent + notFoundComponent + QueryClientProvider + SudoDialog + SessionExpiryWatcher + Toaster), `authenticatedRoute` is a pathless layout rendering `AppShell`. `NAV_AREAS` lists all eight areas including Nodes/Clusters/Config/Jobs/Upgrades/Settings. |
| 2 | Areas that do not exist yet show an honest coming-in-phase-N page, not a dead link or an empty frame | ✓ VERIFIED | `placeholders.tsx` derives one route per `NAV_AREAS` entry with a phase number, so a nav entry can never point at an unregistered route. `ComingSoon` renders "Coming in phase N" plus a one-sentence description and explicitly invents no preview data. |
| 3 | The theme is dark-first, follows `prefers-color-scheme` by default, and an explicit choice survives a reload via localStorage | ✓ VERIFIED | `useTheme.ts` resolves `storedTheme() ?? systemTheme()` on every read, deliberately uncached so the reload case is proven by localStorage rather than by a module variable; `useTheme.test.tsx` in the passing suite. Live: the served `index.html` carries `class="dark"`, `meta color-scheme="dark light"` and an inline boot script that sets the class before first paint. |
| 4 | All visible strings are English and there is no i18n layer | ✓ VERIFIED | No i18n/intl/locale/lingui dependency in `web/package.json`; zero German umlauts and zero German stopwords in `web/src`. |
| 5 | While no account exists every route lands on the setup wizard; afterwards the setup route is unreachable | ✓ VERIFIED | Server-side redirect + 409, both live (see 01-01 truth 2). |
| 6 | A 428 from a destructive action opens the password prompt and replays the original action after re-authentication, without losing input | ✓ VERIFIED | `SudoDialog.test.tsx` asserts the replay is the *same* request object: `replay[0] === first[0]`, `replay[1].body === first[1].body`, `replay[1].headers` deep-equal, `credentials === 'same-origin'`, and exactly one `/api/v1/auth/sudo` call. `api.ts` builds `init` once and reuses it for the replay by construction. Cancelling settles the promise without replaying. |
| 7 | A session expiring mid-work leads to the login screen with an explanatory message and no data loss | ✓ VERIFIED | `api.ts` routes a `login-transition` presentation to `sessionExpiredHandler`; `__root.tsx` clears the query cache, raises `notify.info('Your session ended.', …)` and navigates to `/login?reason=expired`. Tests assert the handler fires for `/api/v1/audit` but **not** for the `/auth/me` probe. |
| 8 | Every error response is rendered from problem+json — title as heading, detail as text, code for routing — never a raw status code and never an empty toast | ✓ VERIFIED | `problem.ts` maps the full closed taxonomy to a presentation; `messageFor` always yields a sentence, including for a non-problem+json response (`toProblemError` synthesises a readable title/detail). `problem.test.ts` in the passing suite. |
| 9 | The audit page shows records chronologically, filters by date and action, opens a detail view per entry, and a broken chain appears as a permanent, non-dismissible banner naming file and line | ✓ VERIFIED | `audit.tsx` has from/to inputs, an action `Select` over the eight contract tokens, a detail `Dialog` and cursor paging on `next_cursor`. `ChainBanner.test.tsx` counts interactive elements and asserts **zero** buttons/links/checkboxes/switches/focusables and zero close/dismiss/acknowledge/hide/ok/x accessible names; the component holds no state at all, so "seen" is unrepresentable. Live API confirmed the filters and cursor behaviour. |

### Observable Truths — Plan 01-06 (configuration, TLS, build)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Every option is settable by flag and by a `HOLZKUBE_` env var; precedence is flag > env > default; there is no config-file parser and no search path | ✓ VERIFIED | One `optionTable` generates the flags, the env lookup, the help and the log. `TestEveryOptionIsSettableByFlagAndByEnvironment`, `TestPrecedenceFlagBeatsEnvBeatsDefault`. No file parser exists in `internal/config`. |
| 2 | At startup the effective value of every option is logged, so a wrong value is visible immediately | ✓ VERIFIED | Live: eight `msg=configuration option=… value=… origin=…` lines with correct origins (`flag` for the two I passed, `default` for the rest). Secrets render as `<redacted>`. The silent-env-fallback defect is fixed — live `HOLZKUBE_SUDO_WINDOW=banana` aborted the start naming the option, the variable and the value. |
| 3 | The data directory follows XDG with a fallback and is created 0700; `--data-dir` / `HOLZKUBE_DATA_DIR` override it and are also the Docker volume path | ✓ VERIFIED | `Resolve` is override → `$XDG_DATA_HOME/holzkube` → `~/.local/share/holzkube`; `EnsureDir` creates with `0700` and deliberately never chmods an existing directory. `TestResolvePrecedence`, `TestEnsureDirCreatesWith0700`, `TestEnsureDirLeavesAnExistingDirectoryAlone`. Live directory is `0700`. |
| 4 | The server binds loopback without configuration; binding all interfaces is never a default | ✓ VERIFIED | Default `127.0.0.1:8443`. A non-loopback bind logs an explicit warning (live: `listening beyond loopback: holzkube is reachable from every device on this network`). |
| 5 | Without a configured certificate the first run generates a long-lived self-signed certificate with SANs for loopback, localhost and the hostname, and logs its SHA-256 fingerprint | ✓ VERIFIED | Live cert: `NotAfter Aug 25 2036`, `CA:FALSE`, `KeyUsage: Digital Signature` (critical), `ExtKeyUsage: TLS Web Server Authentication`, `SAN: DNS:localhost, DNS:build-host.home, IP:127.0.0.1, IP:::1`. The logged fingerprint `D0:91:AF:…:72:C0` is **byte-identical** to `openssl x509 -noout -fingerprint -sha256`. The `IsCA: true` defect is fixed. |
| 6 | A supplied certificate is accepted via `--tls-cert`/`--tls-key` and fully replaces self-generation | ✓ VERIFIED | `TestEnsureUsesASuppliedPairAndGeneratesNothing`, `TestEnsureRefusesABrokenSuppliedPairWithoutFallingBack`, `TestEnsureRefusesHalfASuppliedPair` (re-run, pass). There is deliberately no fall-back-to-generation path. |
| 7 | `--insecure-http` is accepted only on a loopback bind; any other bind address refuses to start with a naming message | ✓ VERIFIED | Live with `--listen 0.0.0.0:18445 --insecure-http`: refused before anything was opened or written, with the full reason. `LoopbackGuard` runs first in `run()`. |
| 8 | A single build command produces the binary, and the frontend assets are built before the Go compile | ✓ VERIFIED (with a noted limitation) | `Taskfile.yml`: `build` → `deps: ["build:go"]` → `deps: ["build:web"]`, asserted structurally. `.goreleaser.yaml` repeats the ordering in `before.hooks`. `go-task` is not installed here, so `task build` itself was not executed — the verifier reproduced both halves manually: `npm --prefix web run build` in a `git archive HEAD` tree regenerated a **byte-identical** bundle (`diff -r` reports no difference; same content hashes `index-CuG0H2d3.js` / `index-A7ipRKiN.css`), and the subsequent `go build` produced a binary that serves the UI. See human verification item 3. |
| 9 | `cmd/holzkubed` demonstrably does not depend on the heavy Talos root module; the sandbox is its own Go module under `sandbox/` | ✓ VERIFIED | `TestBinaryDependencyWeight` walks `go list -deps ./cmd/holzkubed`; `TestGuardRecognisesTheRootModule` is an explicit negative control pinning the classification against the imports phase 2 will add; `TestSandboxIsASeparateModule` confirms `go list ./...` never reports `sandbox/`. `sandbox/go.mod` declares `github.com/holzcloud/holzkube/sandbox`. |

**Score:** 51/51 truths verified (0 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `cmd/holzkubed/main.go` | Process entry: config, data dir, store/audit/auth wiring, HTTPS server | ✓ VERIFIED | 194 lines; `func main`; composition root assembles routes via `slices.Concat` |
| `internal/store/store.go` | Entity-shaped Store interface, never file paths | ✓ VERIFIED | `type Store interface { Users(); Settings(); Sessions(); Close() }`; no method accepts or returns a path |
| `internal/store/lock.go` | Process flock + per-entity mutex map | ✓ VERIFIED | `AcquireProcessLock`; `EntityLocks` is refcounted and deletes entries at zero (unbounded-map defect fixed), with `Len()` for the test |
| `internal/store/fsstore/permissions.go` | 0700 dir / 0600 file guard | ✓ VERIFIED | `permMask = 0o077`; collects every violation; repairs nothing |
| `internal/store/fsstore/crashinject_test.go` | Crash injection at every interruption point | ✓ VERIFIED | `TestCrashInject`, three points, plus a second test that the record is untouched before the rename |
| `internal/store/migrate/migrate.go` | Forward-only migrations with a VERSION file | ✓ VERIFIED | `func Run`; `CurrentVersion = 1`; refuses newer and unreadable versions |
| `internal/store/migrate/backup/backup.go` | Pre-migration tarball | ✓ VERIFIED | Plan said `migrate/backup.go`; it is `migrate/backup/backup.go` (package, forced by Go — 01-02 deviation 1). The key_link pattern `backup\.` still matches and the live tarball was produced. |
| `internal/audit/record.go` | Canonical JSONL record format + chain computation | ✓ VERIFIED | `type Record struct` with a closed field set; canonical form independent of `encoding/json` escaping |
| `internal/audit/chain.go` | sha256 chain computation and verification | ✓ VERIFIED | `Genesis` constant recomputed and confirmed; `Verify` opens read-only and has no repairing counterpart |
| `internal/audit/rotate.go` | Daily rotation with chain hand-off and gzip | ✓ VERIFIED | `gzip`; `keepPlain = 2`; no removal path |
| `internal/audit/redact.go` | Allowlist redaction | ✓ VERIFIED | `allowlist` map; no denylist and no pass-through branch anywhere in the file |
| `internal/audit/query.go` | Filtered, paged query across day files | ✓ VERIFIED | `func Query`; live API exercised with `action`, `limit`, `cursor`, `from`, `to` |
| `internal/httpapi/problem.go` | RFC 9457 taxonomy | ✓ VERIFIED | `application/problem+json`; 12 types matching `docs/api-contract.md` |
| `internal/httpapi/router.go` | Route table with declarative Destructive marking | ✓ VERIFIED | `Destructive bool` on `Route`; the only place URL shapes are known |
| `internal/httpapi/middleware/audit.go` | Intent-before/outcome-after on every mutating route | ✓ VERIFIED | `Attempt`; redaction is called inside the middleware so no wiring mistake can disable it |
| `internal/httpapi/middleware/sudo.go` | Gate reading `Route.Destructive`, throwing 428 | ✓ VERIFIED | `Destructive`; `heldResponse` makes the refresh actually persist |
| `internal/httpapi/handlers/account.go` | First destructive route | ✓ VERIFIED | `Destructive: true` |
| `internal/auth/argon.go` | argon2id params, calibration, hash/verify | ✓ VERIFIED | `argon2id`; fastest-of-three calibration |
| `internal/auth/sudo.go` | Sudo window set / check / expire | ✓ VERIFIED | Plan expected `func SudoWindow`; the implementation is `OpenSudoWindow` / `TouchSudoWindow` / `IsSudoOpen` on `*Service`, which is the same contract with better names. State lives in the session, never in a process-wide register. |
| `internal/auth/ratelimit.go` | Capped exponential per-IP delay, no lock state | ✓ VERIFIED | `func Delay`; `sweep` bounds the map |
| `internal/config/config.go` | Flag/ENV precedence + effective-value logging | ✓ VERIFIED | `HOLZKUBE_`; one option table drives everything |
| `internal/config/datadir.go` | XDG resolution with fallback, 0700 creation | ✓ VERIFIED | `XDG_DATA_HOME` |
| `internal/tlsx/selfsigned.go` | Self-signed cert with SANs and fingerprint | ✓ VERIFIED | `DNSNames`; `IsCA: false` |
| `internal/depguard_test.go` | Executed proof the binary is not on the Talos root module | ✓ VERIFIED | `func TestBinaryDependencyWeight` plus a negative control |
| `docs/api-contract.md` | Binding API contract | ✓ VERIFIED | `## Error Taxonomy`, Routes, CSRF Contract, Audit Query Contract, System Status Contract, Route Registration Rule |
| `Taskfile.yml` | Build chain with enforced web-before-go | ✓ VERIFIED | `build:web`; `deps: ["build:web"]` |
| `.golangci.yml` | v2 schema with gosec enabled | ✓ VERIFIED | `version: "2"`, `gosec` enabled, exclusions are per-rule and per-path, never wholesale |
| `sandbox/go.mod` | Separate module | ✓ VERIFIED | `module github.com/holzcloud/holzkube/sandbox` |
| `web/src/api.ts` | Single fetch seam, CSRF header, problem decoding | ✓ VERIFIED | `X-Holzkube-CSRF` with the literal value `1` |
| `web/src/lib/problem.ts` | RFC 9457 decoding and typing | ✓ VERIFIED | `problem+json`; full closed taxonomy → presentation map |
| `web/src/routes/__root.tsx` | Router root with shell, theme, toaster, error boundary | ✓ VERIFIED | `createRootRoute` |
| `web/src/components/Sidebar.tsx` | Navigation covering later phases | ✓ VERIFIED | `Audit` plus seven other areas |
| `web/src/components/SudoDialog.tsx` | 428 prompt with replay | ✓ VERIFIED | Replay proven byte-identical by test |
| `web/src/components/ChainBanner.tsx` | Permanent broken-chain banner | ✓ VERIFIED | `audit_chain`; stateless; zero interactive elements asserted |
| `web/src/routes/audit.tsx` | Audit table with filters and detail view | ✓ VERIFIED | `next_cursor` paging |
| `web/vite.config.ts` | Build and test config in one file | ✓ VERIFIED | `jsdom` |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `cmd/holzkubed/main.go` | `internal/httpapi/router.go` | builds the chain, mounts the route table | ✓ WIRED | `httpapi.New(deps)` |
| `handlers/setup.go` | `internal/store/store.go` | creates the first user through the Users entity | ✓ WIRED | `d.Store.Users().Put(...)`, `.List(...)` — no path anywhere |
| `middleware/chain.go` | `internal/audit/audit.go` | intent before and outcome after every mutating route | ✓ WIRED | `wrapRoute` → `middleware.Audit(auditAdapter{...})` → `Audit.Attempt`/`Outcome`; live pairs confirm it |
| `internal/httpapi/web.go` | `web/dist` | `go:embed all:dist` | ✓ WIRED | The `all:` prefix is present and load-bearing |
| `web/src/api.ts` | `internal/httpapi/problem.go` | decodes problem+json | ✓ WIRED | `PROBLEM_CONTENT_TYPE`; `ProblemError` |
| `fsstore/fsstore.go` | `store/lock.go` | every Put takes the per-entity mutex before the rev check | ✓ WIRED | `defer u.locks.LockEntity(kindUsers, id)()` before the read-modify-write in all three entity stores |
| `fsstore/fsstore.go` | `migrate/migrate.go` | Open migrates before the first entity operation | ✓ WIRED | `migrate.Run(abs)` after flock, guard and temp sweep |
| `migrate/migrate.go` | `migrate/backup` | backup before the first writing migration | ✓ WIRED | `backup.Create(dir, from, CurrentVersion)`; live tarball produced |
| `middleware/audit.go` | `audit/redact.go` | parameters pass the allowlist before entering the record | ✓ WIRED | `audit.Params(action, readBody(r))` — the only return path in `captureParams` |
| `handlers/system.go` | `audit/chain.go` | reports the startup verdict as `audit_chain` | ✓ WIRED | `d.AuditChain` snapshot is authoritative for a break; a clean start re-verifies live |
| `handlers/audit.go` | `audit/query.go` | serves the Audit Query Contract | ✓ WIRED | `d.Audit.Query(r.Context(), filter)` |
| `middleware/sudo.go` | `auth/sudo.go` | reads the window from session state, decides 428 | ✓ WIRED | `d.Auth.IsSudoOpen(r.Context(), d.SudoWindow)` |
| `handlers/auth.go` | `auth/ratelimit.go` | delays the attempt before the password check | ✓ WIRED | `throttle(limiter, w, r)` before `Login`; one shared limiter for login and sudo |
| `auth/session.go` | `auth/scsstore/scsstore.go` | session state lives behind the entity interface | ✓ WIRED | `sm.Store = scsstore.New(st.Sessions())` |
| `handlers/account.go` | `middleware/sudo.go` | the declarative marking activates the gate | ✓ WIRED | `Destructive: true` → live 428 |
| `web/src/api.ts` | `web/src/lib/problem.ts` | every non-ok response becomes a ProblemError | ✓ WIRED | `toProblemError(response)` |
| `SudoDialog.tsx` | `web/src/api.ts` | posts to `/auth/sudo` then replays | ✓ WIRED | `api.sudo(password)`; replay uses the original `init` |
| `routes/audit.tsx` | `web/src/api.ts` | `GET /api/v1/audit` with from/to/action/limit/cursor | ✓ WIRED | `api.audit(queryFor(filters, cursor))` |
| `ChainBanner.tsx` | `web/src/api.ts` | reads `audit_chain` from system status | ✓ WIRED | `useSystemStatus()` → `api.status()` |
| `routes/__root.tsx` | `AppShell.tsx` | the shell wraps every authenticated route | ✓ WIRED | `authenticatedRoute.component = AppShell` |
| `cmd/holzkubed/main.go` | `internal/config/config.go` | resolves flags/env and logs the effective values | ✓ WIRED | `config.Load(args)` then `cfg.LogEffective(logger)` |
| `cmd/holzkubed/main.go` | `internal/tlsx/load.go` | loads or generates the certificate, checks the loopback guard | ✓ WIRED | `tlsx.LoopbackGuard(...)` then `tlsx.Ensure(cfg)` |
| `Taskfile.yml` | `web/dist` | `build:go` depends on `build:web` | ✓ WIRED | Asserted structurally; `task` itself not installed (human item 3) |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `routes/index.tsx` (Dashboard) | `status.data` | `GET /api/v1/system/status` → `Store.Users().List` + `Audit.Verify` | Yes — live JSON confirmed | ✓ FLOWING |
| `routes/index.tsx` (Recent activity) | `recent.data.items` | `api.audit({limit:3})` → `Audit.Query` → real JSONL files | Yes — live records returned | ✓ FLOWING |
| `routes/audit.tsx` | `page.items`, `next_cursor` | `GET /api/v1/audit` with filters → `audit.Query` | Yes — live filter + cursor confirmed (`next_cursor=13`) | ✓ FLOWING |
| `ChainBanner.tsx` | `chain` | `audit_chain` from system status | Yes — live break reported `ok:false, line 42` | ✓ FLOWING |
| `Header.tsx` / `useSession` | `me` | `GET /api/v1/auth/me` → `Auth.CurrentUser` → `Store.Users().Get` | Yes — live `{"id":"cc77…","username":"operator"}` | ✓ FLOWING |
| `routes/index.tsx` | version / bind address / data directory | none — the endpoint does not carry them | N/A — deliberately **not rendered**, no placeholder invented | ✓ NOT HOLLOW (deferred, see `deferred`) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Build | `go build ./...` | exit 0 | ✓ PASS |
| Vet | `go vet ./...` | exit 0 | ✓ PASS |
| Go test suite (run once) | `go test ./... -count=1` | exit 0, 11 packages ok | ✓ PASS |
| Frontend test suite (run once) | `npm --prefix web test -- --run` | 5 files / 60 tests passed | ✓ PASS |
| Single self-contained binary | `otool -L bin` | only libSystem, libresolv, CoreFoundation, Security; 11.5 MB | ✓ PASS |
| Embedded UI over HTTPS on loopback | `curl -sk https://127.0.0.1:18443/` | 302 → `/setup`; `/setup` returns the SPA; `/assets/index-*.js` 200 | ✓ PASS |
| Fingerprint matches a browser's format | `openssl x509 -noout -fingerprint -sha256` vs the startup log | identical string `D0:91:AF:…:72:C0` | ✓ PASS |
| Leaf certificate, not a CA | `openssl x509 -text` | `CA:FALSE`, KeyUsage `Digital Signature`, EKU `TLS Web Server Authentication` | ✓ PASS |
| Setup → 409 on re-run | live POST `/api/v1/setup` twice | 201 then 409 `setup.already-completed` | ✓ PASS |
| Sudo gate | live POST `/api/v1/account/password` | 428 → sudo 204 → 204 | ✓ PASS |
| CSRF, all three preconditions individually | live POSTs | 403 for missing header, wrong header value, foreign Origin, cross-site Sec-Fetch-Site | ✓ PASS |
| Rate limit delays, never locks | 10 wrong logins then the right one | `401 401 401 429×7`, then 204 | ✓ PASS |
| Audit intent/outcome pairing | live log after a full session | 14 records, 7 pairs, one JSON object per line | ✓ PASS |
| Hash chain, **independent oracle** | Python re-implementation of the canonical form | `records=190 breaks=0`; genesis matches `shasum -a 256` | ✓ PASS |
| No secret in the audit log | grep for both passwords and the live session token | 0 occurrences; session appears as `_gmnqZec…` | ✓ PASS |
| Tamper detection + no auto-repair | edit `actor` on line 42, restart, write more records | startup ERROR names file + line 42; status keeps `ok:false` after further writes; the tampered byte is still on disk | ✓ PASS |
| **Real out-of-process `kill -9` × 16** | 16 rounds, server SIGKILLed under 1–16 parallel login loops, restarted each time | every restart clean; 190 audit records, chain verifies; 46 session records, 0 corrupt; **0 orphan temporaries**; user record intact at rev 2; login still works | ✓ PASS |
| Second process refused | start a second binary on the same data dir | `store: data directory already in use by another process … locked by pid 23972` | ✓ PASS |
| Permission guard | start on a `0755` dir with a `0644` record | all three violations reported at once, nothing repaired | ✓ PASS |
| `--insecure-http` off loopback | `--listen 0.0.0.0:18445 --insecure-http` | refused at start with the full reason | ✓ PASS |
| Bad env value aborts | `HOLZKUBE_SUDO_WINDOW=banana` | aborts naming option, variable and value | ✓ PASS |
| Schema upgrade backs up first | set `VERSION=0`, start | real `backups/pre-migration-0-to-1-*.tar.gz` written before `VERSION` became 1; excludes lock file and backups dir | ✓ PASS |
| Version guards | `VERSION=2`, `VERSION=banana` | both refuse to start with distinct, naming errors | ✓ PASS |
| Fresh-clone build reproduces the bundle | `git archive HEAD` → `npm run build` | `diff -r` against the working tree's dist: **identical** (same content hashes) | ✓ PASS |
| Bundle-less binary fails honestly | build from a fresh checkout without the web build | 404 `notfound.ui` "The web UI is not built into this binary. Run `task build:web` and rebuild." | ✓ PASS |
| `task build` / `golangci-lint run` / `goreleaser` | — | tools not installed on this host | ? SKIP → human item 3 |

### Probe Execution

| Probe | Command | Result | Status |
|-------|---------|--------|--------|
| — | — | No `scripts/*/tests/probe-*.sh` exist and no plan or SUMMARY declares a probe. This is not a migration/tooling phase in the probe sense; its runnable checks are the Go and frontend suites plus the live binary above. | N/A |

### Requirements Coverage

All 11 phase requirements are claimed by at least one plan and none is orphaned.
Plan coverage: 01-01 → 01,02,05,06,07,11 · 01-02 → 07,08,09,10 · 01-03 → 06 · 01-04 → 02,03,04 · 01-05 → 01,03,11 · 01-06 → 01,05,10. Union = all 11.

| Requirement | Source Plan(s) | Description | Status | Evidence |
|-------------|----------------|-------------|--------|----------|
| FOUND-01 | 01-01, 01-05, 01-06 | Single binary, embedded web UI, no runtime dependencies | ✓ SATISFIED | 11.5 MB binary, system libs only; `go:embed all:dist`; UI served live; bundle reproduces byte-identically from a fresh checkout |
| FOUND-02 | 01-01, 01-04 | Username + password login (argon2id, params stored); session rotated at login | ✓ SATISFIED | PHC-encoded hash; calibrated 296 ms live; `TestSessionRotatesOnLogin` (old id invalidated and record deleted) |
| FOUND-03 | 01-04, 01-05 | Destructive actions re-ask for the password (sudo-mode) even with a session | ✓ SATISFIED | Live 428 → sudo → success; `Destructive: true` read by middleware; the client-side 428 replay is proven byte-identical |
| FOUND-04 | 01-04 | Login is rate-limited against brute force | ✓ SATISFIED | Live capped exponential backoff, 429 + `Retry-After`; recovery by waiting; no lock state and no unlock path exists |
| FOUND-05 | 01-01, 01-06 | Binds `127.0.0.1` by default; HTTPS with a self-generated certificate when none is configured | ✓ SATISFIED | Default `127.0.0.1:8443`; cert generated on first run, SANs correct, `CA:FALSE`, fingerprint matches openssl; supplied pairs replace generation with no fallback |
| FOUND-06 | 01-01, 01-03 | Every mutation in the audit log (JSONL, daily rotation, hash chain) — intent before, outcome after | ✓ SATISFIED | 7/7 live pairs; chain independently recomputed (0 breaks over 190 records); rotation, cross-boundary chain and gzip proven by re-run tests |
| FOUND-07 | 01-01, 01-02 | All state access through an entity interface; file paths invisible above the store | ✓ SATISFIED | `store.Store` is entity-shaped with no path in any signature; `TestNoDirectFileAccessOutsideFsstore` is an executed AST guard with a self-check against scanning nothing (see W-1) |
| FOUND-08 | 01-02 | Concurrent writes corrupt nothing (atomic writes, flock, per-entity mutex, rev CAS) | ✓ SATISFIED | All four present and tested `-race`; **16 real out-of-process `kill -9` rounds** produced no corruption and no orphan temporaries; second process refused live |
| FOUND-09 | 01-02 | Schema version; forward migrations with a backup first | ✓ SATISFIED | Live `VERSION=0` upgrade wrote a real tarball before changing anything; newer and unreadable versions refuse to start. The migration list is empty because phase 1 defines one version — the machinery itself was exercised live, not only against fixtures |
| FOUND-10 | 01-02, 01-06 | All state files `0600` inside a `0700` directory; wrong permissions flagged at start | ✓ SATISFIED | Live listing shows `0700`/`0600` throughout including `VERSION`, `cert.pem`, `key.pem`, `holzkube.lock`; a `0755`/`0644` directory was refused with all violations named and nothing repaired |
| FOUND-11 | 01-01, 01-05 | API errors as RFC 9457 problem+json with a stable taxonomy | ✓ SATISFIED | Live responses across 7 distinct codes; 12 taxonomy entries match `docs/api-contract.md`; `internal.*` carries no detail (`TestProblemInternalLeaksNothing` re-run); the client renders a sentence for every path |

**Orphaned requirements:** none. REQUIREMENTS.md maps exactly FOUND-01…FOUND-11 to Phase 1 and every one is claimed by a plan.

### Prohibitions (must-NOT)

All four declared prohibitions are `verification: test` and each has **wired, executed enforcement** — none is fail-closed-unverified.

| Prohibition | Plan | Enforcement | Status |
|-------------|------|-------------|--------|
| The taxonomy must not leak internal details (paths, Go errors, store messages) into `detail`; 500s carry only a request id | 01-01 | `TestProblemInternalLeaksNothing` — asserts six forbidden substrings are absent **and** that `detail` is empty. Re-run: PASS. `Internal(err)` discards the error by construction. | ✓ ENFORCED |
| Parameters must not be filtered by denylist and unknown parameters must not pass through | 01-03 | `redact.go` contains no denylist and no pass-through branch; `TestRedactRedactsEverythingForAnUnknownAction`, `TestRedactReplacesAnEntireUnknownBranch`, `TestRedactRefusesAStructureUnderAnAllowlistedPath`, `TestRedactAllowlistCarriesNoCredentialFields`. Live: both passwords absent from the log. | ✓ ENFORCED |
| A detected chain break must not be auto-repaired, recomputed, rewritten or click-away-able | 01-03 | `TestChainVerifyNeverWrites` (re-run: PASS), `TestSystemStatusKeepsAChainBreakVisible` (re-run: PASS), `ChainBanner.test.tsx` asserts zero interactive elements. **Live**: after tampering, further records were written and the break stayed reported and the tampered byte stayed on disk. | ✓ ENFORCED |
| The login rate limit must never become a hard lock, and no unlock/recovery/emergency path may be added | 01-04 | `TestRateLimitHasNoStateThatOutlastsTheWait`, `TestRateLimitSlowsGuessingAndNeverLocksOut`. Code review confirms no lock, unlock, recovery or emergency endpoint anywhere in `internal/auth` or the handlers. **Live**: after 7 consecutive 429s the correct password returned 204. | ✓ ENFORCED |

### Test Quality Audit

| Test area | Linked Req | Active | Skipped | Circular | Assertion Level | Verdict |
|-----------|-----------|--------|---------|----------|-----------------|---------|
| `internal/store/fsstore` (crash, concurrency, permissions) | FOUND-07/08/10 | 17 | 0 | No | Behavioral (state after a simulated crash; byte comparison) | ✓ Sufficient |
| `internal/store`, `internal/store/migrate` | FOUND-08/09 | 16 | 0 | No | Behavioral + value | ✓ Sufficient |
| `internal/audit` | FOUND-06 | 34 | 0 | No | Value (byte-level canonical form, exact line of a break) | ✓ Sufficient |
| `internal/auth` | FOUND-02/03/04 | 22 | 0 | No | Behavioral (clock-driven expiry, measured wall-clock cost) | ✓ Sufficient |
| `internal/httpapi` + middleware + handlers | FOUND-01/03/06/11 | 40 | 0 | No | Behavioral end-to-end through the real chain | ✓ Sufficient |
| `internal/config`, `internal/tlsx` | FOUND-05/10 | 17 | 0 | No | Value | ✓ Sufficient |
| `web/src` (5 files, 60 tests) | FOUND-01/03/11 | 60 | 0 | No | Behavioral (fetch-level replay identity, DOM role queries) | ✓ Sufficient |

**Disabled tests on requirements:** 0. The only three `t.Skip` calls are `runtime.GOOS == "windows"` guards on POSIX permission bits — inert on the target platforms.
**Circular patterns detected:** 0. The audit chain is the one place where circularity was plausible, so the verifier re-implemented the canonical form and the digest **independently in Python** and confirmed the live log against it; the two agree, including on the exact line of an injected break.
**Insufficient assertions:** 0. Notably, `TestArgonVerifyCostsAtLeastTheTarget` measures a real verification rather than inspecting parameters — which is why the 18 % under-calibration was caught during execution rather than shipped.
**Negative controls present:** `TestGuardRecognisesTheRootModule` (dependency classification), the `scanned < 15` self-check in the architecture guard, and `TestRedactAllowlistCarriesNoCredentialFields`.

### Decision Coverage

All 16 CONTEXT.md decisions D-01…D-16 are referenced in shipped source, and each was
checked for substance rather than for the presence of the string:

| Decision | Files | Honoured |
|---|---|---|
| D-01 setup wizard gate | 7 | Yes — server-side redirect + 409 |
| D-02 XDG data dir / volume path | 7 | Yes |
| D-03 flags + ENV only, effective values logged | 3 | Yes — no config-file parser exists |
| D-04 HTTPS default, self-signed, fingerprint | 8 | Yes |
| D-05 5-minute sudo window, refreshed per action | 11 | Yes |
| D-06 declarative Destructive marking | 7 | Yes — middleware reads the flag, nothing matches URLs |
| D-07 24 h absolute sessions, rotated at login | 8 | Yes — idle timeout deliberately unset |
| D-08 capped delay, never a lock | 7 | Yes |
| D-09 English only, no i18n | 3 | Yes |
| D-10 permanent shell + honest placeholders | 10 | Yes |
| D-11 dark-first theme | 6 | Yes |
| D-12 shadcn/ui copied into `src/components/ui/` | 1 | Yes — 13 components copied, Radix primitives used, `components.json` present (see I-1) |
| D-13 audit visible in the UI in phase 1 | 3 | Yes |
| D-14 allowlist redaction, closed field set | 6 | Yes |
| D-15 chain verified at start, break stays visible | 11 | Yes |
| D-16 never delete a rotated file | 9 | Yes |

**Status impact:** none (warning-only gate). No decision was abandoned during execution.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| — | — | `TBD` / `FIXME` / `XXX` anywhere in `internal`, `cmd`, `web/src`, `docs`, build config | none found | Debt-marker gate passes cleanly |
| — | — | `TODO` / `HACK` | none found | — |
| `internal/store/fsstore/permissions_test.go` | 185-190 | W-1: the architecture guard exempts `internal/store`, `internal/tlsx` and `internal/audit` as whole subtrees | ⚠️ Warning | Correct today — those three packages legitimately own file paths — but the exemption is by directory, so a future file added under `internal/audit` or `internal/tlsx` inherits it without review. Worth tightening to a per-file or per-function allowance in a later phase. Does not affect FOUND-07 today. |
| `web/package.json` | 24 | I-1: `shadcn` (a CLI) is listed under `dependencies` rather than `devDependencies` | ℹ️ Info | Nothing imports it, so it does not reach the bundle; cosmetic only. |
| `web/src` (bundle) | — | I-2: 572.71 kB bundle, over vite's 500 kB warning | ℹ️ Info | Already recorded as `deferred-items.md` item 2. Everything is embedded in one binary by design, so there is no round trip to save. |
| `internal/audit/audit.go` | 200-230 | I-3: the outcome record repeats the intent's `actor`, so a successful login's outcome reads `actor=anonymous` | ℹ️ Info | Observed live. Consistent with the closed field set (D-14) and the intent carries the username in `params`, so nothing is lost forensically. Noting it so it is not later mistaken for a bug. |

The `return null` / `return {}` matches in `web/src` are all legitimate conditional renders
or typed absence values (`ChainBanner` returning null on a clean chain, `storedTheme()`
returning null when nothing is stored). No stub, placeholder or hardcoded empty
render-path was found anywhere in the phase.

### Known Gaps — Verifier's Judgement

The six items the SUMMARY files recorded as open were treated as inputs, not rediscovered.
Verdict on each:

1. **`kill -9` only proven in-process (coverage D4).** **Materially closed by this verification.**
   The verifier ran 16 real out-of-process `kill -9` rounds against the built binary under
   1–16 parallel mutating-request loops, restarting after each. Every restart came up clean:
   0 orphan temporaries, 0 chain breaks across 190 records, 46/46 session records parsed,
   the user record intact, and login still working. The criterion as written — *"a `kill -9`
   mid-write leaves no corrupt state"* — is **met**. What remains genuinely unproven is a
   *power-loss* / lying-disk-cache scenario (a torn write below the fsync), which is a
   different and strictly harder failure mode than `kill -9` and is not what the criterion says.
   Acceptable for the phase to be called done.

2. **Certificate fingerprint never compared in a browser (D6).** The verifier confirmed the
   logged string is byte-identical to `openssl x509 -noout -fingerprint -sha256`, in the same
   colon-separated upper-case hex format browsers use. The remaining step is a human reading
   two strings side by side, which is exactly what the format exists for. Routed to human
   verification item 2. Not a blocker.

3. **No real migration exists.** Correct, and unavoidable: phase 1 defines exactly one schema
   version. The verifier nevertheless drove a **real** upgrade live by setting `VERSION=0`, and
   the backup-before-first-change path produced an actual tarball with the right contents and
   exclusions, after which `VERSION` became 1. FOUND-09's substance is proven, not only
   fixture-simulated. Acceptable.

4. **`system/status` carries no version / bind address / data directory.** Not a must-have truth
   in any plan and not a ROADMAP criterion. The dashboard omits the fields honestly instead of
   inventing them, so there is no hollow prop. Recorded under `deferred`. Acceptable.

5. **`grep -c '@radix-ui/' web/package.json` returns 0.** Confirmed. The unified `radix-ui@1.6.7`
   package is a dependency and is imported by seven of the copied shadcn components
   (`import { Dialog as DialogPrimitive } from 'radix-ui'`). D-12's substance — Radix primitives
   plus Tailwind, components copied into `src/components/ui/` — is fully satisfied; only the
   literal grep string in the 01-05 acceptance criterion is stale. Not a gap. If you want it
   recorded formally rather than as a note, an override is suggested below.

6. **FOUND-07 "held by review and a depguard rule".** This understates what exists.
   `.golangci.yml` enables no `depguard` linter at all; the real enforcement is
   `TestNoDirectFileAccessOutsideFsstore`, an AST walk over `internal/` and `cmd/` that fails on
   any forbidden `os.*` call outside three named subtrees and that fails if it scanned fewer than
   15 files. It was verified during execution by planting a canary. That is an exhaustive
   proof over non-test source, subject to the subtree-exemption caveat in W-1. **Stronger than
   claimed.** Acceptable.

### Defects Fixed During Execution — Independently Confirmed

All seven were verified present in the code, not merely claimed:

| # | Defect | Confirmation |
|---|--------|--------------|
| 1 | Full session token written into the permanently-retained audit log | `ShortSession` applied inside `Logger.append` (not at the call site). **Live**: the log shows `_gmnqZec…`; grep for the full cookie value returns 0. |
| 2 | Unbounded per-entity mutex map | `entityLock.refs` counts holders and waiters; the entry is deleted at zero; `Len()` exists for `TestEntityLocksReleaseEmptiesTheMap`. |
| 3 | argon2id 18 % under-calibrated | `measureHash` returns the fastest of three samples; `calibrationMargin = 1.2`. **Live**: 296.8 ms ≥ the 250 ms target. |
| 4 | Sudo refresh computed and discarded | `heldResponse` buffers the response so the session commit happens after the refresh; `TestSudoGateDoesNotRefreshOnAFailedAction` also pins the "only on success" half. |
| 5 | `IsCA: true` on the leaf certificate | `IsCA: false` in `selfsigned.go`. **Live openssl**: `X509v3 Basic Constraints: critical CA:FALSE`. |
| 6 | Env parse errors silently falling back to defaults | `LoadWith` returns an error naming option, variable and value. **Live**: `HOLZKUBE_SUDO_WINDOW=banana` aborts the start. |
| 7 | Release archives shipping without the API contract | `.goreleaser.yaml` `archives[0].files: [README.md, docs/*]`; `before.hooks` build the web bundle before the Go compile. (Archive contents themselves unverified — goreleaser is not installed; human item 3.) |

### Human Verification Required

Three items. This phase ships a user-facing web UI, so the infrastructure-phase
auto-pass does not apply; these are genuine human steps, not invented ones.

#### 1. Render the UI in a real browser

**Test:** Open `https://127.0.0.1:8443`, complete setup, log in, click every sidebar entry,
and toggle the theme.
**Expected:** The shell, sidebar, header, toasts and audit table render correctly and legibly
in both themes; placeholder pages read as intentional.

> **Scope amended 2026-08-28.** The original criterion also required "the sudo dialog appears
> on the password change and the underlying form is not lost". Phase 1 ships no
> password-change UI, so that clause moved to Phase 10 (success criterion 5), which builds the
> Settings screen. The sudo *mechanism* was verified in a real browser during UAT via an
> injected 428. See `01-UAT.md` gap G-01-1.
**Why human:** All 60 frontend tests run under jsdom, which renders nothing. Layout, contrast,
focus rings and visual polish are not programmatically checkable.

#### 2. Compare the certificate fingerprint in the browser's certificate dialog

**Test:** Read `sha256_fingerprint=` from the startup log, then open the certificate dialog in
the browser and compare character by character.
**Expected:** Identical strings in the same format, needing no conversion.
**Why human:** Recorded gap D6. The verifier proved the string matches `openssl` exactly; the
remaining step is the human comparison the format was designed for.

#### 3. Run the three uninstalled gates

**Test:** Install the tools from `deferred-items.md` item 4, then run `task build`,
`golangci-lint run` and `goreleaser release --snapshot --clean`.
**Expected:** `task build` produces `bin/holzkubed` with the frontend built first; golangci-lint
is clean under the v2 config with gosec on; the snapshot archives contain the binary, README.md
and `docs/*`.
**Why human:** `go-task`, `golangci-lint` and `goreleaser` are not on this host. The verifier
reproduced the build chain by hand (fresh `git archive HEAD` → `npm run build` → **byte-identical**
bundle → `go build` → working UI) and asserted the ordering edges structurally, but the three
gates themselves were not executed here.

### Gaps Summary

**No gaps.** Every observable truth, artifact, key link and prohibition that could be checked
programmatically was checked and passed — most of them against a running binary rather than
against source alone. The phase goal is achieved: a single 11.5 MB binary with no runtime
dependencies serves an embedded UI over HTTPS on loopback, authenticates with calibrated
argon2id and a rotated session, gates destructive actions behind a re-entered password, records
every mutation as a fsynced intent/outcome pair in a hash chain that an independent
re-implementation agrees with, and survives repeated real `kill -9` under load without losing or
corrupting a byte.

Two of the six recorded known gaps are, on inspection, stronger than the SUMMARYs claimed
(the `kill -9` proof and the FOUND-07 guard); the rest are honest, correctly scoped and
acceptable. The status is `human_needed` solely because three verifications genuinely require a
human or a tool that is not installed — not because anything is missing from the code.

**Optional override** — if you want the stale acceptance-grep recorded formally rather than as a
note, add to this file's frontmatter:

```yaml
overrides:

  - must_have: "grep -c '@radix-ui/' web/package.json returns a non-zero count"
    reason: "shadcn 4.19 ships Radix as the unified `radix-ui` package; radix-ui@1.6.7 is a dependency and is imported by seven copied ui/ components. D-12's substance (Radix primitives + Tailwind, copied not framework-installed) is fully satisfied; only the literal grep string is stale."
    accepted_by: "holz"
    accepted_at: "2026-08-28T07:05:00Z"
```

---

_Verified: 2026-08-28T07:05:00Z_
_Verifier: Claude (gsd-verifier)_
