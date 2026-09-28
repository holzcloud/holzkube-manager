---
phase: 01-foundation-skeleton
plan: 01
subsystem: infra
tags: [go, react, vite, typescript, tailwind, argon2id, scs, rfc9457, jsonl, hash-chain, embed, tls]

# Dependency graph
requires: []
provides:
  - "Go module github.com/holzcloud/holzkube with toolchain go1.26.7 pinned"
  - "store: entity-shaped persistence interface that never leaks a path, plus the fsstore atomic write sequence"
  - "audit: JSONL record format and hash chain over canonical JSON, intent-before/outcome-after"
  - "auth: argon2id hashing, scs sessions backed by store.Sessions(), session rotation on login, sudo window"
  - "httpapi: closed RFC 9457 taxonomy, Route type with the Destructive marking, per-link middleware chain"
  - "docs/api-contract.md: the binding contract wave 2 codes against"
  - "web/: Vite+React scaffold, single fetch chokepoint in src/api.ts, committed package-lock.json"
  - "Taskfile.yml encoding the hard web-before-go build order"
affects: [01-02, 01-03, 01-04, 01-05, 01-06, 02-talos-transport, 03-inventory, 06-jobs, 09-upgrades]

actuals:
  tokens: 55501
  tasks: 4
  commits: 5

tech-stack:
  added:
    - "github.com/alexedwards/argon2id v1.0.0"
    - "github.com/alexedwards/scs/v2 v2.9.0"
    - "golang.org/x/sync v0.22.0"
    - "golang.org/x/crypto v0.55.0 (indirect, via argon2id)"
    - "react 19.2.8, react-dom 19.2.8, zod 4.4.3"
    - "vite 8.2.2, @vitejs/plugin-react 6.1.0, typescript 7.0.2"
    - "tailwindcss 4.3.3 via @tailwindcss/vite 4.3.3"
    - "@biomejs/biome 2.5.11"
  patterns:
    - "Entity-shaped store access: store.Users().Get(ctx, id), never a path"
    - "Atomic write sequence: tmp -> chmod 0600 -> fsync -> rename -> fsync(dir)"
    - "Audit as an intent/outcome pair, intent fsynced before the action"
    - "Hash chain over canonical JSON of the record without its hash field"
    - "RFC 9457 problem+json as the only error shape, with stable code tokens"
    - "Declarative Destructive marking on the route, read by middleware (D-06)"
    - "One middleware link per file, so parallel plans never touch chain.go"
    - "Single fetch chokepoint in web/src/api.ts carrying the CSRF preconditions"

key-files:
  created:
    - cmd/holzkubed/main.go
    - internal/store/store.go
    - internal/store/fsstore/fsstore.go
    - internal/store/fsstore/atomic.go
    - internal/audit/record.go
    - internal/audit/audit.go
    - internal/auth/auth.go
    - internal/auth/scsstore/scsstore.go
    - internal/httpapi/problem.go
    - internal/httpapi/router.go
    - internal/httpapi/web.go
    - internal/httpapi/endtoend_test.go
    - internal/httpapi/problem_test.go
    - internal/audit/record_test.go
    - docs/api-contract.md
    - web/src/api.ts
    - web/package-lock.json
    - Taskfile.yml
  modified:
    - README.md

key-decisions:
  - "Audit hash chain uses canonical JSON over the record without its hash field (human decision at the task 2 gate)"
  - "@biomejs/biome pinned to 2.5.11 instead of the plan's 2.5.10, on explicit human instruction at the task 1 gate"
  - "The route table is assembled at the composition root, not inside router.go, because the plan's literal shape is an import cycle in Go"
  - "github.com/justinas/nosurf was not added: the chosen CSRF combination needs no token library, and go mod tidy strips an unused module"
  - "Audit params are recorded as an empty object in phase 1, because capturing them before allowlist redaction exists would write the setup and login passwords into a log kept forever"
  - "The audit middleware is fail-closed: a mutation whose intent cannot be made durable is refused, not performed"

patterns-established:
  - "Route registration rule: a plan adds routes in its own handler file plus one line at the composition root; router.go is not touched again"
  - "Every mutating route must set Action, or it executes with no audit record at all"
  - "next_cursor is always present and is number-or-null; 0 is never a cursor and clients compare against null"

requirements-completed: []

coverage:
  - id: D1
    description: "A single built binary serves HTTPS on 127.0.0.1 against a fresh data directory, with the data directory 0700 and key material 0600"
    requirement: "FOUND-01, FOUND-05"
    verification:
      - kind: e2e
        ref: "internal/httpapi/endtoend_test.go#TestEndToEndSetupLoginAudit"
        status: pass
      - kind: manual_procedural
        ref: "./bin/holzkubed --data-dir <fresh> then curl -sk https://127.0.0.1:8443/api/v1/system/status"
        status: pass
    human_judgment: false
  - id: D2
    description: "The embedded React UI renders and its setup and login forms are usable in a real browser"
    requirement: "FOUND-01"
    verification: []
    human_judgment: true
    rationale: "No browser automation exists in this phase. The bundle is proven to build, embed and be served, and the server-side /setup redirect is asserted, but nothing asserts that the page renders or that the forms submit. Playwright coverage is not in phase 1 scope."
  - id: D3
    description: "The setup wizard creates exactly one account and afterwards actively refuses with 409 setup.already-completed rather than merely hiding the route"
    requirement: "FOUND-01"
    verification:
      - kind: e2e
        ref: "internal/httpapi/endtoend_test.go#TestEndToEndSetupLoginAudit"
        status: pass
    human_judgment: false
  - id: D4
    description: "Login authenticates via argon2id, rotates the session id, sets an HttpOnly+Secure+SameSite=Lax cookie, and answers identically for an unknown user and a wrong password"
    requirement: "FOUND-02"
    verification:
      - kind: e2e
        ref: "internal/httpapi/endtoend_test.go#TestEndToEndSetupLoginAudit"
        status: pass
    human_judgment: false
  - id: D5
    description: "Setup and login each write an intent/outcome JSONL pair, the chain over all lines verifies, and a rewritten record is detected at the right line"
    requirement: "FOUND-06"
    verification:
      - kind: e2e
        ref: "internal/httpapi/endtoend_test.go#TestEndToEndSetupLoginAudit"
        status: pass
      - kind: unit
        ref: "internal/audit/record_test.go#TestVerifyDetectsTampering"
        status: pass
      - kind: unit
        ref: "internal/audit/record_test.go#TestCanonicalJSONIsKeySorted"
        status: pass
      - kind: unit
        ref: "internal/audit/record_test.go#TestResumeContinuesTheChain"
        status: pass
    human_judgment: false
  - id: D6
    description: "Every API error is problem+json from the closed taxonomy with an absolute type URI and a stable code; an internal error leaks neither path nor Go error text; concurrent failures get distinct instances"
    requirement: "FOUND-11"
    verification:
      - kind: unit
        ref: "internal/httpapi/problem_test.go#TestProblemTaxonomy"
        status: pass
      - kind: unit
        ref: "internal/httpapi/problem_test.go#TestProblemInternalLeaksNothing"
        status: pass
      - kind: unit
        ref: "internal/httpapi/problem_test.go#TestProblemInstancesAreDistinctUnderConcurrency"
        status: pass
    human_judgment: false
  - id: D7
    description: "No package above internal/store/fsstore reads or writes state records directly; all record access goes through the store entity methods"
    requirement: "FOUND-07"
    verification:
      - kind: other
        ref: "grep -cE 'func .*\\(path string' internal/store/store.go returns 0; os.ReadFile/WriteFile audit over cmd and internal finds no state-record access outside fsstore"
        status: pass
    human_judgment: true
    rationale: "Currently held by review and a grep, not by an enforcing lint rule. internal/audit and internal/tlsx legitimately open their own files (append-only log, TLS material), so a naive rule would need to encode that exemption. The research SUMMARY calls this out as worth a lint rule; golangci-lint configuration belongs to plan 06."
  - id: D8
    description: "docs/api-contract.md is sufficient for the five wave-2 plans to work in parallel without further coordination"
    verification:
      - kind: other
        ref: "all six required headings present; all 12 code taxonomy entries cross-checked against the document table"
        status: pass
    human_judgment: true
    rationale: "Structural completeness is machine-checked, but whether the contract is actually sufficient to prevent two parallel plans from diverging is a judgment that only shows up when wave 2 runs."

# Metrics
duration: 19 min
completed: 2026-08-28
status: complete
---

# Phase 1 Plan 01: Foundation Tracer Summary

**A single 10 MB `holzkubed` binary that serves the embedded React UI over HTTPS on `127.0.0.1`, creates the first operator account through a browser setup wizard, rotates the session on login, and records both operations as intent/outcome JSONL pairs in a verifying SHA-256 hash chain.**

## Performance

- **Duration:** 19 min
- **Started:** 2026-08-28T01:01:58Z
- **Completed:** 2026-08-28T01:21:51Z
- **Tasks:** 4 (2 human gates resolved before this agent resumed, 2 executed)
- **Files changed:** 48 (47 created, 1 modified)

## Accomplishments

- The whole architecture proven end-to-end before any of it was widened: binary → HTTPS → embedded UI → setup → login → audit, in one path with one test.
- All the interfaces wave 2 builds against are fixed: the `store` interface, `audit.Record`, the `problem` taxonomy, `Route` with its `Destructive` marking, the CSRF contract, and the `GET /api/v1/audit` query contract.
- `docs/api-contract.md` written as a binding document, including the `next_cursor` exhaustion rule stated as two explicit obligations so plans 03 and 05 cannot each invent their own.
- The audit hash chain is encoder-independent by construction, which is what makes indefinite retention (D-16) survivable across a Go upgrade.
- Zero Talos. Nothing imports `pkg/machinery`, nothing speaks gRPC, and `go.mod` contains no `siderolabs` line.

## Task Commits

1. **Task 3 RED — failing end-to-end test** — `f5b586a` (test)
2. **Task 3 GREEN, move 1/3 — Go path to a green tracer** — `adef298` (feat)
3. **Task 3, move 2/3 — Vite/React scaffold and embedded bundle** — `35779bf` (feat)
4. **Task 3, move 3/3 — build chain and first-run docs** — `57df87c` (feat)
5. **Task 4 — closed taxonomy and the wave-2 API contract** — `410b304` (feat)

Tasks 1 and 2 were checkpoints; they produced no commits by design.

## Resolved Checkpoints

### Task 1 — Package-Legitimacy Gate (`checkpoint:human-verify`, `gate="blocking-human"`)

**Human answer: approved, with one instruction — bump `@biomejs/biome` from 2.5.10 to 2.5.11.**

All 9 npm packages and 5 Go modules were confirmed present at the pinned versions with the expected upstream maintainers before any install ran. Every installed version was re-verified against the lockfile after installation; all nine npm pins match exactly, including the instructed 2.5.11. `npm audit` reported 0 vulnerabilities.

### Task 2 — Audit JSONL record format (`checkpoint:decision`, `gate="blocking"`)

**Human answer: `canonical-json-over-record-without-hash`.**

Implemented exactly as specified: `hash_n = sha256(hash_{n-1} || canonical_json(record_n without the hash field))`, where canonical means keys sorted lexicographically, no whitespace, UTF-8, and no HTML escaping. Nested `params` are normalized explicitly through a decoder with `UseNumber` rather than being left to Go's randomized map iteration order.

The rationale is carried in the code (`internal/audit/record.go`), in `docs/api-contract.md` and in the README: D-16 mandates unbounded retention, which makes encoder-independence a permanent obligation. A chain anchored to `encoding/json`'s byte output would break retroactively across the entire archive on a Go upgrade or a reordered struct field. `TestCanonicalJSONIsKeySorted` re-canonicalizes the same record 50 times and asserts byte-identical output, which is what pins the property rather than merely claiming it.

## Files Created/Modified

**Go — core**
- `cmd/holzkubed/main.go` — composition root: config → data dir → store → audit → startup chain verification → auth → HTTPS server
- `internal/config/config.go` — flags + `HOLZKUBE_*` only, precedence flag > env > default, XDG data dir (D-02, D-03)
- `internal/tlsx/selfsigned.go` — ECDSA P-256 self-signed cert, 10 years, SANs for `127.0.0.1`/`::1`/`localhost`/hostname, `0600`, fingerprint logged (D-04)
- `internal/model/model.go` — `UserID`/`ClusterID`/`MachineID` as distinct types, records carrying `Rev`
- `internal/store/store.go` — the entity-shaped interface; no method takes or returns a path
- `internal/store/fsstore/{fsstore,atomic}.go` — `0600` file implementation, `rev` CAS, key validation against traversal, full atomic write sequence
- `internal/audit/{record,audit}.go` — closed record format, canonical JSON, hash chain, daily file, `fsync` per record, `Attempt`/`Outcome`/`Verify`/`List`
- `internal/auth/auth.go` — argon2id (64 MiB, t=3, p=4) with parameters in the hash, session rotation on login, decoy verification for unknown users, sudo window
- `internal/auth/scsstore/scsstore.go` — `scs.Store` over `store.Sessions()`, so session state does not bypass FOUND-07

**Go — HTTP**
- `internal/httpapi/problem.go` — 12-entry closed RFC 9457 taxonomy with stable codes
- `internal/httpapi/router.go` — `Route` with `Destructive`, `Deps`, mux mounting, chain wiring, 405 discrimination, SPA fallback
- `internal/httpapi/web.go` — `//go:embed all:dist`, SPA fallback, server-side `/setup` redirect
- `internal/httpapi/middleware/*.go` — eight files, one link each
- `internal/httpapi/handlers/*.go` — setup, auth, account (intentionally empty), audit, system

**Frontend**
- `web/src/api.ts` — the only place calling `fetch`; sets the CSRF preconditions and decodes `problem+json` into a typed `ProblemError`
- `web/src/App.tsx` — setup form, login form, audit table, chain-break banner; English only (D-09)
- `web/package-lock.json` — committed, as plans 05 and 06 require

**Docs and tooling**
- `docs/api-contract.md`, `README.md`, `Taskfile.yml`, `biome.json`, `.gitignore`

## Decisions Made

- **Route assembly lives at the composition root, not in `router.go`.** See deviation 1 — the plan's literal structure does not compile in Go. `router.go` still owns the `Route` type, the `Destructive` semantics, the mounting and the chain, and wave-2 plans still never edit it.
- **`Action` added to `Route`.** The audit middleware needs a stable token per route. Deriving it from the URL pattern would be fragile, and a mutating route without one executes unlogged — so it is documented in the contract as a defect of the same class as a missing `Destructive`.
- **The audit middleware is fail-closed.** If the intent record cannot be made durable, the request is refused with a 500 rather than performed. An unlogged mutation is exactly what the audit log exists to prevent (threat T-01-05).
- **`params` is empty in phase 1 audit records.** Capturing request bodies before the allowlist redactor exists would write the setup and login passwords straight into an append-only log that is never deleted. The plan assigns redaction to plan 03; capture is deferred to land with it.
- **`GET /api/v1/audit` returns `{items, next_cursor}` from day one**, not a bare array, so plan 03 does not have to change a shape plan 05 already consumes.
- **`system/status` reports the startup verdict, and re-verifies live only when startup was clean.** A break found at startup stays visible for the process lifetime as D-15 requires; a break appearing later is not hidden until the next restart.
- **Settings has `Get`/`Put` only**, not the full four-method set. A singleton with a `List` method invites code that assumes otherwise. The shape of the Settings entity is explicitly under Claude's discretion in `01-CONTEXT.md`.

## Deviations from Plan

### 1. [Human-instructed] `@biomejs/biome` pinned to 2.5.11, not 2.5.10

- **Found during:** Task 1 gate (before this agent resumed)
- **Issue:** The plan pinned 2.5.10 from `STACK.md`; the registry's latest had moved to 2.5.11.
- **Fix:** `web/package.json` and `biome.json`'s `$schema` pin 2.5.11. Verified against the lockfile: installed version is exactly 2.5.11.
- **Authority:** Explicit human instruction at the blocking-human gate. Every other version stays exactly as the plan pinned it.
- **Committed in:** `35779bf`

### 2. [Rule 3 - Blocking] Route assembly moved from `router.go` to the composition root

- **Found during:** Task 3, move 1
- **Issue:** The plan says `router.go` aggregates `handlers.SetupRoutes(deps)` and friends, while also placing the problem taxonomy at `internal/httpapi/problem.go` in package `httpapi` and having handlers call the constructors. Those two requirements are jointly impossible in Go: `httpapi` → `handlers` → `httpapi` is an import cycle and does not compile. The same cycle applies to `middleware`.
- **Fix:** Dependencies point one way only. `httpapi` owns `Route`, `Deps` and the taxonomy; `handlers` and `middleware` import `httpapi` and never the reverse; `middleware` links take narrow callbacks and locally declared interfaces. The five `handlers.*Routes(deps)` calls are composed in `cmd/holzkubed/main.go` via `slices.Concat` and passed in as `Deps.Routes`.
- **What is preserved:** `router.go` still owns the `Route` type, the `Destructive` marking, the mux mounting and the chain wiring, and is still never edited by a wave-2 plan. The registration rule is documented in `docs/api-contract.md` § *Route Registration Rule*: add routes in your own handler file, plus one line at the composition root.
- **Verification:** `go build ./...`, `go vet ./...` clean; the acceptance grep for `Destructive bool` in `router.go` passes.
- **Committed in:** `adef298`

### 3. [Rule 3 - Blocking] `github.com/justinas/nosurf` not added

- **Found during:** Task 3, move 1
- **Issue:** The plan lists nosurf v1.2.0 as a pinned dependency, but the CSRF design it also specifies — `Content-Type: application/json` **and** `X-Holzkube-CSRF` **and** an `Origin`/`Sec-Fetch-Site` check — needs no token library. An unused module is stripped by `go mod tidy` on the next run, so adding it would create a dependency that silently vanishes in CI.
- **Fix:** Not added. The CSRF combination is implemented directly in `internal/httpapi/middleware/csrf.go`, ~50 lines, and the three preconditions are asserted by the end-to-end test.
- **Impact:** None on behaviour. nosurf remains an option if plan 04 layers a double-submit token on top, which `docs/api-contract.md` notes explicitly.
- **Committed in:** `adef298`

### 4. [Rule 1 - Bug] `Taskfile.yml` did not parse as YAML

- **Found during:** Task 3, move 3
- **Issue:** `deps: [build:web]` is invalid YAML — inside a flow sequence, `:` is an indicator, so the parser fails on the plain scalar. `go-task` is not installed on this host, so the file would have been committed broken and only failed for the operator later.
- **Fix:** Quoted the dependency names: `deps: ["build:web"]`, `deps: ["build:go"]`.
- **Verification:** The file now parses, and `build:go` is confirmed to depend on `build:web`.
- **Committed in:** `57df87c`

### 5. [Rule 2 - Missing Critical] Added an audit chain test suite

- **Found during:** Task 3, move 1
- **Issue:** Threat T-01-04 is dispositioned `mitigate` with the hash chain as the mitigation, but nothing asserted that the chain actually detects a rewritten record. An untested tamper-evidence claim is a claim.
- **Fix:** Added `internal/audit/record_test.go`: canonical output is key-sorted and stable across 50 re-serializations, the `hash` field cannot influence the hash, `prev_hash` does, a record edited in place is detected at the correct line number, and the chain continues rather than forking across a restart.
- **Committed in:** `adef298`

### 6. [Rule 2 - Missing Critical] Added path-traversal rejection on entity keys

- **Found during:** Task 3, move 1
- **Issue:** `fsstore` derives filenames from identifiers that are, for sessions, externally supplied. The plan lists traversal as canon covered by `gosec`, but `gosec` flags the file read, not a missing key check.
- **Fix:** `safeKey` restricts keys to `[A-Za-z0-9_-]` with a length cap, returning `store.ErrInvalidKey` otherwise. Applied on every user and session path derivation.
- **Committed in:** `adef298`

### 7. [Rule 2 - Missing Critical] Audit params deliberately left empty rather than captured

- **Found during:** Task 3, move 1
- **Issue:** D-14 calls for input parameters in the record, but the allowlist redactor is assigned to plan 03. Capturing bodies now would write the setup and login passwords in cleartext into an append-only log that is never deleted (D-16) — unrecoverable once written.
- **Fix:** The audit middleware records `params: {}`. Capture lands together with redaction in plan 03. Documented in the code, in `docs/api-contract.md` and below under Known Stubs.
- **Committed in:** `adef298`

---

**Total deviations:** 7 (1 human-instructed, 3 Rule 3 blocking, 1 Rule 1 bug, 3 Rule 2 missing-critical — items 5–7 counted once each under Rule 2).
**Impact on plan:** No scope creep. Deviation 2 is the only structural one and was forced by Go's import rules; the property the plan actually cares about — wave-2 plans never touching `router.go` — is preserved and documented. Deviation 4 caught a file that would have shipped broken.

## Known Stubs

All of these are deferrals the plan itself assigns to a named later plan, not accidental gaps. Each is documented at its site.

| Stub | File | Resolved by |
|---|---|---|
| `AccountRoutes` returns nil | `internal/httpapi/handlers/account.go:11` | Plan 04 adds the password change as `Destructive: true` |
| Audit query parameters accepted but not applied; `next_cursor` always `null` | `internal/httpapi/handlers/audit.go:29` | Plan 03 implements filters and the cursor |
| `params: {}` on every audit record | `internal/httpapi/router.go:169` | Plan 03, together with allowlist redaction |
| `Verify` covers only the current day's file | `internal/audit/audit.go:262` | Plan 03 extends it to the last rotated file |
| No rotation, no gzip of older files | `internal/audit/audit.go` | Plan 03 |
| Sudo gate is fail-closed with no way to open the window | `internal/auth/auth.go:236` | Plan 04 adds `POST /api/v1/auth/sudo` |
| No login rate limiting; `RateLimited()` constructor unused | `internal/httpapi/problem.go` | Plan 04 (FOUND-04) |
| No `flock`, no per-entity lock map, no `VERSION`/migrations, no startup permission guard | `internal/store/fsstore/` | Plan 02 (FOUND-08, FOUND-09, FOUND-10) |
| `problem.Forbidden` and `problem.UnsupportedMediaType` minted but not emitted | `internal/httpapi/problem.go` | Reserved by design — the taxonomy is a closed contract |
| No router, no shadcn/ui, no theme toggle, no toasts; the UI is one file | `web/src/App.tsx` | Plan 05 (D-10, D-11, D-12) |
| Lint and release targets absent from `Taskfile.yml` | `Taskfile.yml` | Plan 06 |
| `build:web` uses `npm install`, not `npm ci` | `Taskfile.yml:16` | Plan 06, now that the lockfile is committed |

None of these prevents this plan's goal: the tracer path runs end-to-end on real data.

## Threat Flags

None. No file created here introduces network, auth, file-access or trust-boundary surface beyond what the plan's `<threat_model>` already registers. The mitigations for T-01-01 through T-01-09 and T-01-SC are all implemented; T-01-04, T-01-06 and T-01-09 additionally have tests.

## Issues Encountered

- **The `Destructive bool` acceptance grep initially failed.** `gofmt` column-aligns struct fields, so the field rendered as `Destructive     bool` and the literal string never appeared. Rather than defeat the check, the `Route` struct was restructured so each flag sits in its own documented group — which `gofmt` renders unaligned, and which is better code anyway.
- **Biome's `files.includes` and `vcs.useIgnoreFile` both misfired.** `useIgnoreFile` demanded an ignore file inside `web/`, and `files.includes` combined with a CLI path argument matched nothing. Resolved by scoping through the CLI path instead of config globs. `npm --prefix web run lint` is clean.
- **`go-task` is not installed on this host,** so `Taskfile.yml` could not be executed directly. It was validated as YAML and structurally (`build:go` depends on `build:web`), and each of its commands was run manually to produce the binary. This is what surfaced deviation 4.
- **TypeScript 7.0.2 works with a plain `tsc --noEmit`.** `tsc -b` was avoided since the plan specifies a single `tsconfig.json` with no project references.

## User Setup Required

None — no external service configuration. The operator creates the first account in the browser on first run.

## Next Phase Readiness

**Ready.** All five wave-2 plans have what they need:

- **Plan 02 (store hardening)** — the `store` interface and `fsstore` with the atomic sequence are in place; `flock`, the per-entity mutex map, `VERSION`/migrations and the permission guard slot in beneath an unchanged interface.
- **Plan 03 (audit)** — the record format is fixed and hashed, `Verify` and `List` have their signatures, and the query contract including the `next_cursor` rule is written down.
- **Plan 04 (auth hardening)** — `HasSudo`/`GrantSudo` exist and the gate is fail-closed; `account.go` is the empty file waiting for the password change; the `sudo-required` and `rate-limited` taxonomy entries are minted.
- **Plan 05 (app shell)** — `web/package-lock.json` is committed, `api.ts` is the single fetch chokepoint, and the response shapes are pinned in the contract.
- **Plan 06 (build/release)** — `Taskfile.yml`, `biome.json` and `.gitignore` exist; the lockfile is committed and in sync, so the `npm ci` tightening is a one-line change.

**Concerns to carry forward:**

1. **No requirement was marked complete.** All six IDs this plan declares (`FOUND-01`, `-02`, `-05`, `-06`, `-07`, `-11`) are also declared by sibling plans that have not finished. `requirements.ready-ids` returned `0/6 ready`, which is correct: they flip only when the last declaring plan finishes.
2. **FOUND-07 is held by review and a grep, not by a lint rule.** The research SUMMARY calls the rule worth enforcing; `golangci-lint` configuration belongs to plan 06. A rule would need to exempt `internal/audit` and `internal/tlsx`, which legitimately own their own files.
3. **The UI is unverified in a browser.** It builds, embeds, and is served, and the `/setup` redirect is asserted server-side — but nothing asserts that the page renders or that the forms submit. Flagged as `D2` with `human_judgment: true`.
4. **Plan 02 will find the store thread-unsafe across processes.** Per-entity mutexes exist within one process; there is no `flock`. Two `holzkubed` instances on one data directory can still interleave writes. This is exactly plan 02's scope, noted so it is not rediscovered as a surprise.

## Self-Check: PASSED

**Files claimed as created — all present on disk:**
`cmd/holzkubed/main.go`, `internal/store/store.go`, `internal/store/fsstore/fsstore.go`, `internal/store/fsstore/atomic.go`, `internal/audit/record.go`, `internal/audit/audit.go`, `internal/auth/auth.go`, `internal/auth/scsstore/scsstore.go`, `internal/httpapi/problem.go`, `internal/httpapi/router.go`, `internal/httpapi/web.go`, `internal/httpapi/endtoend_test.go`, `internal/httpapi/problem_test.go`, `internal/audit/record_test.go`, `docs/api-contract.md`, `web/src/api.ts`, `web/package-lock.json`, `Taskfile.yml` — FOUND.

**Commits — all five present in `git log`:** `f5b586a`, `adef298`, `35779bf`, `57df87c`, `410b304` — FOUND.

**Plan-level verification re-run at close-out:**

| Check | Result |
|---|---|
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `npm --prefix web install && npm --prefix web run build` | PASS |
| `go test ./... -count=1` | PASS |
| `go test ./internal/httpapi -run TestEndToEndSetupLoginAudit -count=1` | PASS |
| `go test ./internal/httpapi -run TestProblem -count=1` | PASS |
| Binary against a fresh data dir answers `{"setup_required":true,...}` on `https://127.0.0.1:8443` | PASS |
| `docs/api-contract.md` carries all six required headings | PASS |

**Task acceptance criteria — all re-run:**

| Criterion | Result |
|---|---|
| `go.mod` contains `toolchain go1.26.7` | PASS (host is go1.26.4; the build auto-downloaded and used 1.26.7) |
| `grep -rn 'siderolabs' go.mod \| wc -l` is 0 | PASS |
| `grep -c 'go:embed all:' internal/httpapi/web.go` ≥ 1 | PASS (1) |
| `internal/store/store.go` has `type Store interface`, no path parameter | PASS (1 / 0) |
| `internal/httpapi/router.go` contains `Destructive bool` | PASS (2) |
| `git ls-files --error-unmatch web/package-lock.json` | PASS |
| `grep -c 'https://holzkube.dev/problems/' internal/httpapi/problem.go` ≥ 12 | PASS (13) |
| `docs/api-contract.md` names `POST /api/v1/account/password` as `Destructive: true` | PASS |
| `grep -c 'next_cursor !== null' docs/api-contract.md` ≥ 1 | PASS (1) |
| Second `POST /api/v1/setup` → 409 `problem+json` `setup.already-completed` | PASS (e2e) |
| Session cookie differs before and after login | PASS (e2e) |
| `GET /api/v1/audit` returns 4 records; `audit_chain.ok` true | PASS (e2e) |
| Mutating request without `X-Holzkube-CSRF: 1` → 403 | PASS (e2e) |
| `problem.Internal(err)` leaks neither path nor error text | PASS (unit) |

**Hygiene:** no file deletions in the plan's commit range; `.gsd/` and `.planning/milestone.lock` are untracked and are listed in `.gitignore`; `node_modules/` never staged.

---
*Phase: 01-foundation-skeleton*
*Completed: 2026-08-28*
