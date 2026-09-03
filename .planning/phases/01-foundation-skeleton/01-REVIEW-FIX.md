---
phase: 01-foundation-skeleton
fixed_at: 2026-08-28T10:15:00Z
review_path: .planning/phases/01-foundation-skeleton/01-REVIEW.md
iteration: 1
findings_in_scope: 24
fixed: 24
skipped: 0
status: all_fixed
---

# Phase 1: Code Review Fix Report

**Fixed at:** 2026-08-28
**Source review:** `.planning/phases/01-foundation-skeleton/01-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 24 (CR-01..03, WR-01..21)
- Fixed: 24
- Skipped: 0
- Deferred: 0

Every finding was reproduced against the source before being changed. None
turned out to be wrong on inspection, though three were fixed differently from
the review's suggestion for reasons recorded below (WR-02, WR-04, CR-03).

> **Scope note.** The REVIEW.md frontmatter says `warning: 19, total: 22`, but
> the body contains WR-01 through WR-21. The real in-scope count is 24, and all
> 24 were addressed. The frontmatter counters are wrong, not the body.

## Verification

Gates were run **in the main checkout**, not in an isolated worktree:
`.planning/config.json` sets `workflow.use_worktrees: false`, so per #2825 this
run edited and committed on `main` directly. The numbers below are therefore
reproducible from the tree as it stands.

| Gate | Baseline (`9d5cecc`) | After |
|---|---|---|
| `go build ./...` | exit 0 | exit 0 |
| `go vet ./...` | exit 0 | exit 0 |
| `go test ./... -count=1` | 11 packages ok | 11 packages ok |
| `npm --prefix web test -- --run` | 5 files / 60 tests | 5 files / **64 tests** |
| `npm --prefix web run typecheck` | — | exit 0 |
| `npm --prefix web run lint` | — | 41 files, no fixes |
| `npm --prefix web run build` | — | exit 0 |

`go-task`, `golangci-lint` and `goreleaser` are not on PATH and were not run.

**Pre-existing flake, not caused by these fixes.** `TestArgonVerifyCostsAtLeastTheTarget`
(`internal/auth`) failed once during a full-suite run and passed on `-count=5`
in isolation. It measures wall-clock argon2id cost, so concurrent load from the
rest of the suite perturbs it. `internal/auth/argon.go` was untouched at the
time it failed. Worth a separate look; it is a real source of CI noise.

**Regression tests were verified to fail against the old code** for the four
findings where that was the whole claim: WR-04 (no record at all for a 428),
WR-03 (`allFiles` returned four files for three days), WR-11 (reported line 3
instead of 5), and WR-13 (hung for the full 5s timeout). The rest are covered
by tests that pass, plus the existing suite.

## Fixed Issues

### CR-03: `migrate.run` stamps VERSION as current even when no migration ran

**Files:** `internal/store/migrate/migrate.go`, `internal/store/migrate/migrate_test.go`, `internal/store/testdata/README.md`
**Commit:** `bb99e0c`

Confirmed worse than REVIEW.md stated, exactly as the orchestrator described:
`migrations` is empty, so `VERSION=0` created a real backup tarball, matched no
step, and stamped the directory as version 1.

The fix resolves the whole path **before** the backup is taken and refuses if
it has a hole, so an unadvanceable directory is left literally untouched — not
even a tarball implying a migration was attempted. This is stronger than the
review's suggestion, which took the backup first and then refused.
`readVersion` now rejects `0`, matching its own doc comment.

**Deviation from the review, and the reason.** Rejecting `0` breaks the two
tests that drove the whole upgrade path through the `version-previous` fixture
— which is `VERSION=0`. That fixture is *deliberate*: `internal/store/testdata/README.md`
explains it exists so the machinery is proven in phase 1 while only one real
schema version exists. Rather than special-case 0 (which the orchestrator ruled
out) or delete the coverage, `run` now takes the target version as a parameter,
so the tests drive a real **1 → 2** step — the exact shape phase 3 will add —
instead of a fictional 0 → 1 one. `version-previous/` is repurposed as the
fixture for *refusing* version 0, and the README is updated to match.

Net effect on the verifier's FOUND-09 evidence: that `VERSION=0` path now
returns an error naming the version it is stuck at, instead of silently
no-opping the migration half while producing a real backup.

### CR-01 / CR-02: unauthenticated status endpoint

**Files:** `internal/httpapi/handlers/system.go`, `internal/httpapi/router.go`, `internal/audit/audit.go`, `internal/audit/chain.go`, `cmd/holzkubed/main.go`, `internal/httpapi/auditapi_test.go`, `docs/api-contract.md`
**Commit:** `22d0e15`

> **Both findings are in this one commit.** They touch the same handler, the
> same `ChainStatus` type and the same section of `docs/api-contract.md`, and
> separating them would have meant splitting a single edit to `system.go`. The
> commit message names only CR-01; CR-02's path-stripping is in it too. I did
> not amend to correct this because the orchestrator's instructions forbid
> rewriting history — recording it here instead.

**CR-01.** The endpoint still answers before authentication (`setup_required`
is what tells the UI whether to show the setup wizard), but the live
re-verification is now behind `d.Auth.IsAuthenticated`. Anonymous callers get
the startup snapshot. D-15 is unaffected: a break found at startup is served
from that snapshot and was never re-checked here in the first place.
`CachedVerify` memoises for 30s, and `VerifyContext` honours cancellation.

**Deliberate departure from the review's fix.** The review says to snapshot the
file list under `l.mu` and verify *outside* the lock. I did not. `Logger.Query`
documents that it takes the same lock specifically "so a page can never be
assembled from a half-written line" — dropping it for verification would trade
this bug for false chain breaks, which D-15 makes permanent and non-dismissible
until an operator deals with the file by hand. The auth gate plus the cache
removes the unbounded anonymous lever, which was the actual vulnerability.

**CR-02.** `ChainStatus.Public()` reduces `File` to a basename, applied both in
`main.go` (so `Deps.AuditChain` never holds a path) and at the serialization
point in `system.go` (so the live path cannot bypass it). The server-side log
line keeps the full path. `docs/api-contract.md:279` is corrected, and the D-15
test now asserts no path separator reaches an unauthenticated caller.

### WR-04: every gate denial is unaudited

**Files:** `internal/httpapi/router.go`, `internal/httpapi/handlers/account_test.go`, `docs/api-contract.md`
**Commit:** `bf2928d`

Audit moves to sit **between** the authn and sudo gates, so `428 sudo.required`
is now recorded as an attempt plus an error outcome carrying the code.

**Deliberate departure from the review's fix — please read.** The review
suggests moving audit outside all three gates, which would also record the 401
and 403. I stopped short, because that would hand an *unauthenticated* caller a
way to append to an archive D-16 keeps forever with no deletion path, on every
mutating route, with no CSRF check and no rate limit in front of it. That is a
worse problem than the one being fixed. The 428 is only reachable by an
authenticated caller, so recording it costs nothing an attacker can spend.
`csrf.precondition-unmet` and `auth.unauthenticated` therefore remain
unrecorded — now by choice, documented and tested, rather than by accident.

The sudo gate's `heldResponse` still writes to the real `ResponseWriter` only
at flush, so the window refresh continues to happen before `scs` commits.

### WR-12: tlsx temporaries hold private keys, are never swept, land in backups

**Files:** `internal/tlsx/selfsigned.go`, `internal/store/store.go`
**Commit:** `50988a2`

Uses `store.TempFilePrefix`, putting the file under both the sweeper and the
backup exclusion. The `store` package is a light leaf (context, errors, model),
so importing it from `tlsx` costs nothing. Point 3 of the finding (ordering)
needs no change: the sweep at `Open` precedes `tlsx.Ensure` on every boot, so a
crash orphan is removed at the next start.

### WR-02: `Logger.Outcome` bypasses the redactor

**Files:** `internal/audit/redact.go`, `internal/audit/audit.go`, `internal/audit/outcome_redact_test.go`
**Commit:** `d48954a`

**Deviation.** The review's first option — allowlisting `"<action>.outcome": {"error"}` —
would permit *any* error string in clear and defeat the purpose. I took its
second option: `OutcomeCause` accepts only taxonomy-code-shaped values and
redacts everything else. Redacted rather than truncated, because truncating
would keep the first 256 characters of exactly the free text it rejects. No
pass-through branch and no exception were added, per the orchestrator.

### WR-03: duplicate day files break the reader

**Files:** `internal/audit/rotate.go`, `internal/audit/rotate_test.go`
**Commit:** `cea4ed4`

`allFiles` now collapses a day to the compressed copy. `plainFiles` untouched,
so `CompressOlderThan` still finishes the orphan. Reuses the existing `dayOf`
in `query.go` rather than duplicating it.

### WR-11: reported line is a record index, not a file line

**Files:** `internal/audit/rotate.go`, `internal/audit/chain.go`, `internal/audit/chain_test.go`
**Commit:** `4149152`

`readFileLocated` carries the source line. **The record shape and the hash input
are unchanged** — `located` embeds `Record` and `ComputeHash` still receives the
`Record` itself, so the canonical form D-16 makes permanent is untouched.

### WR-01, WR-05, WR-10, WR-19, WR-09, WR-20, WR-21

| ID | Commit | Note |
|---|---|---|
| WR-01 | `94dbdd0` | Stale `pending` entries evicted on the next `Attempt`. |
| WR-05 | `4a8a5d9` | Step arithmetic extracted to `nextIterations` so the `measured == 0` path is directly testable; it is not otherwise reachable without invalid params. |
| WR-10 | `c001730` | Contract now stated on the function: a non-nil error means the password was **not** changed. |
| WR-19 | `fb4a9cb` | Also **added** the retry at `Open`, which did not previously exist — `open` calls `openDay` with `l.file == nil`, so compression was never attempted at start. The review assumed it was. |
| WR-09 | `2efd64f` | Sweep at `Open`. A failure is logged, not fatal: refusing to start over one unparsable stale session file would be worse than the records it leaves. |
| WR-20 | `389b667` | Paths carried as segments. Also guards the recursion's backing array, which would otherwise let siblings overwrite each other's segment. |
| WR-21 | `1129aa1` | Context threaded; negative latched. Latch lives in `fallback`, not on `Deps`, which is copied by value. |

### WR-06, WR-07, WR-08: network hardening

| ID | Commit |
|---|---|
| WR-06 | `732b5cb` |
| WR-07 | `6b9a55b` |
| WR-08 | `d4df687` |

**WR-07 — the CSP hash is computed at runtime, not written down.** A hardcoded
digest drifts the first time anyone edits `index.html`, and the failure is
silent: the browser refuses the pre-paint theme script and nothing in any server
log says so. `ContentSecurityPolicy()` hashes the inline script out of the
embedded bytes this binary actually serves, and a test re-derives the hash from
the served response and fails if they disagree. `script-src` carries the hash
rather than `'unsafe-inline'`. HSTS keys off `r.TLS`, so `--insecure-http` does
not pin an operator's browser to HTTPS for a host they chose to serve without it.

**WR-08 — fixed within the three-precondition design; `nosurf` stays out**, per
the orchestrator. The `Host` check is a fourth condition in the **outer** chain,
not inside the CSRF link: CSRF only covers mutating requests, and a rebound
`GET /api/v1/audit` is a leak of the archive. The bind address is also added to
the generated certificate's SANs, derived from the same value as the allowlist
so the two cannot disagree.

> **One fail-open default to be aware of.** An empty `Deps.AllowedHosts`
> disables the Host check. The composition root always populates it, but the
> escape hatch exists because tests cannot know the port `httptest` will pick.
> Making it fail-closed would have required touching every test that constructs
> `Deps`. This is the one place in these fixes that defaults open, and it is
> worth a second opinion.

### WR-13, WR-14, WR-15, WR-16, WR-17, WR-18

| ID | Commit | Note |
|---|---|---|
| WR-13 | `dfb4785` | Displaced challenge settles false. |
| WR-14 | `feb6bc9` | Pages merged by `seq`, so a repeat `queryFn` run is a no-op. |
| WR-15 | `feb6bc9` | Pairs on the immediate successor. **Behaviour change:** the newest record is no longer flagged as orphaned — its outcome may be in flight, and the record that would disprove it is not in hand. The pre-existing test asserted the opposite and was rewritten, with three tests added covering the two false negatives the review named. |
| WR-16 | `feb6bc9` | Read-only tokens dropped. |
| WR-17 | `75cc432` | See lockfile note below. |
| WR-18 | `a073d37` | Validation in the option table, where the operator can still act. |

> **WR-17 lockfile note.** Refreshing `package-lock.json` also filled in six
> bundled sub-dependencies of `@tailwindcss/oxide-wasm32-wasi` that were missing
> from it, so the diff is +383/-2 rather than a few lines. They are optional,
> platform-specific WASM packages; nothing installed on darwin or linux changes,
> and build, typecheck, lint and tests all pass.

## Follow-ups worth a human decision

1. **`TestArgonVerifyCostsAtLeastTheTarget` is load-sensitive** and fails
   intermittently under full-suite concurrency. Pre-existing; not addressed
   here because it is not in the review.
2. **`Deps.AllowedHosts` defaults open** (see WR-08 above).
3. **The REVIEW.md frontmatter counters** (`warning: 19, total: 22`) disagree
   with its own body (21 warnings, 24 in scope).
4. **CR-01/CR-02 share commit `22d0e15`**, and its message names only CR-01.

---

_Fixed: 2026-08-28_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
