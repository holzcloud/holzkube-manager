---
phase: 01-foundation-skeleton
plan: 06
subsystem: infra
tags: [go, config, xdg, tls, ecdsa, self-signed, taskfile, golangci-lint, gosec, goreleaser, go-modules, npm-ci]

# Dependency graph
requires:
  - phase: 01-01
    provides: "the composition root in cmd/holzkubed, the minimal config and tlsx packages, Taskfile.yml with the hard web-before-go order, and the committed web/package-lock.json"
  - phase: 01-02
    provides: "the 0700/0600 permission contract the data directory is created against, and the store guard that refuses to start on a wrong mode"
  - phase: 01-03
    provides: "the startup audit-chain verdict already wired into main.go, which this plan restructured around rather than reverted"
  - phase: 01-05
    provides: "the committed and in-sync lockfile that makes npm ci possible, and the vite plugin that keeps internal/httpapi/dist/.gitkeep alive"
provides:
  - "config.optionTable: one table from which the flags, the HOLZKUBE_ lookup, the help output and the startup log are all generated"
  - "config.LoadWith(args, env, home): precedence flag > environment > default, with the origin of every effective value recorded"
  - "config.Config.LogEffective: one startup log line per option with value and origin, plus the non-loopback bind warning"
  - "config.Resolve / config.EnsureDir: XDG data directory resolution and 0700 creation that leaves an existing directory untouched"
  - "config.IsLoopback: the single loopback predicate, used by the startup warning and by the plaintext guard"
  - "tlsx.Generate / tlsx.Fingerprint: a self-signed leaf (not a CA) written 0600 through the store's atomic sequence, fingerprinted in the browser's own format"
  - "tlsx.Ensure: supplied pair, existing generated pair, or a new one -- with no silent fall back from a broken supplied pair"
  - "tlsx.LoopbackGuard: --insecure-http refused on any bind address that is not loopback, before anything is opened"
  - "Taskfile targets build, build:web, build:go, test, test:web, lint, lint:go, lint:web, fmt, dev, clean, release:snapshot"
  - ".golangci.yml (v2 schema) with gosec and eight more linters, green at zero findings"
  - ".goreleaser.yaml (v2 schema) producing CGO_ENABLED=0 archives for linux/amd64, linux/arm64 and darwin/arm64 with checksums"
  - "sandbox/ as a separate Go module, with internal/depguard_test.go enforcing that the Talos root module never reaches cmd/holzkubed"
  - "main.version, injected by goreleaser and by task build, surfaced through --version"
affects: [01-verify, 02-talos-transport, 03-inventory, 06-jobs, 09-upgrades]

actuals:
  tokens: 24394
  tasks: 3
  commits: 8

tech-stack:
  added:
    - "golangci-lint v2.13.1 (v2 config schema) as the Go lint gate"
    - "goreleaser v2.18.0 as the release builder"
    - "go-task v3 targets for the full build, test, lint and release chain"
    - "No new Go module: go.mod and go.sum are unchanged by this plan"
  patterns:
    - "One option table generates the flags, the environment lookup, the help output and the startup log, so a new switch cannot be missing from any of them"
    - "Every effective value is logged with its origin (default, environment, flag), because 'wrong value' and 'read from somewhere I forgot' look identical otherwise"
    - "An unparsable value aborts the start naming option, origin and value; it never falls back to the default"
    - "Environment and home directory are injected as parameters, so configuration tests mutate no process state"
    - "Security-relevant guards run before anything is opened or created, so a refusal is a start failure with nothing written"
    - "A self-signed server certificate is a leaf and says so: IsCA false, no KeyUsageCertSign, no trust store import"
    - "Fingerprints are rendered in the format the operator is asked to compare against, not in the format that was convenient to produce"
    - "Every lint exclusion names a rule and a place; no linter is switched off wholesale"
    - "A lint gate lands green: the findings it reports on existing code are resolved in the commit before it"
    - "The module boundary is a running test, not a note in a document"

key-files:
  created:
    - internal/config/config_test.go
    - internal/config/datadir.go
    - internal/config/datadir_test.go
    - internal/tlsx/load.go
    - internal/tlsx/load_test.go
    - internal/tlsx/selfsigned_test.go
    - internal/depguard_test.go
    - .golangci.yml
    - .goreleaser.yaml
    - sandbox/go.mod
    - sandbox/README.md
  modified:
    - internal/config/config.go
    - internal/tlsx/selfsigned.go
    - cmd/holzkubed/main.go
    - Taskfile.yml
    - README.md
    - .gitignore
    - internal/store/fsstore/fsstore.go
    - internal/store/migrate/migrate_test.go
    - internal/httpapi/middleware/recover.go
    - internal/httpapi/middleware/chain.go
    - internal/httpapi/endtoend_test.go
    - internal/httpapi/handlers/account_test.go

key-decisions:
  - "GET /api/v1/system/status was NOT extended with version, bind address and data directory: the gap plan 01-05 recorded is left open and documented, because the endpoint answers before authentication, no consumer remains inside phase 1 to shape or test the addition, and the API contract is the artefact five parallel plans coded against"
  - "The generated certificate is no longer marked IsCA and can no longer sign: the plan says IsCA false and D-04 says there is no private CA; the inherited code from plan 01 contradicted both"
  - "The fingerprint format changed to colon-separated upper-case hex pairs, byte-identical to what a browser and openssl x509 -fingerprint -sha256 print, because the operator is asked to compare it by eye"
  - "The port default is 8443: the common unprivileged HTTPS convention, no collision with 8080-range dev servers, and no root required, which D-02 presupposes"
  - "A non-loopback bind is warned about, never refused: FOUND-05 wants loopback as the default, not as a cage"
  - "A host name that is not localhost counts as not-loopback in the plaintext guard: resolving it would take DNS, and a guard that cannot be sure must be the careful one"
  - "The lint gate landed green rather than red: the eleven findings it reported on existing code were resolved in the commit before it, because a gate that fails on the day it arrives gets bypassed within a week"
  - "gosec exclusions are per rule and per place -- G304 only in the three packages that own files by design, gosec and bodyclose only in _test.go where wrong-permission fixtures are the point, G104 as a proven duplicate of errcheck -- and no linter is disabled wholesale"
  - "golangci-lint's default issue caps (50 per linter, 3 per rule) are lifted, because 'nearly clean' under a cap is a statement about the cap"
  - "slog.SetDefault is called at the composition root, so --log-level governs packages that log through the package-level slog functions too"
  - "--version and --help are queries, not options: they are outside the table because an option without an environment variable would break the table's invariant"

patterns-established:
  - "Adding an option means adding one entry to optionTable in internal/config/config.go; the flag, the HOLZKUBE_ variable, the help line and the startup log line follow, and a test asserts the table and its test-value table agree"
  - "A value that cannot be parsed is a start failure, never a default"
  - "Anything that needs the Talos root module goes into sandbox/, and internal/depguard_test.go fails the build if it does not"
  - "A new lint exclusion states its rule and its place in the config file, next to the reason"

requirements-completed: [FOUND-01, FOUND-05, FOUND-10]

coverage:
  - id: D1
    description: "Every option is settable by a flag and by a HOLZKUBE_ environment variable, with precedence flag > environment > default and the origin of each effective value recorded"
    requirement: "FOUND-01"
    verification:
      - kind: unit
        ref: "internal/config/config_test.go#TestEveryOptionIsSettableByFlagAndByEnvironment"
        status: pass
      - kind: unit
        ref: "internal/config/config_test.go#TestPrecedenceFlagBeatsEnvBeatsDefault"
        status: pass
      - kind: unit
        ref: "internal/config/config_test.go#TestDefaultsWithoutAnyInput"
        status: pass
      - kind: unit
        ref: "internal/config/config_test.go#TestHelpAndVersionAreSentinels"
        status: pass
    human_judgment: false
  - id: D2
    description: "The effective value of every option is logged at startup with where it came from, and an unparsable value aborts the start naming the option, its origin and the value"
    requirement: "FOUND-01"
    verification:
      - kind: unit
        ref: "internal/config/config_test.go#TestLogEffectiveLogsEveryOptionExactlyOnce"
        status: pass
      - kind: unit
        ref: "internal/config/config_test.go#TestUnparsableValueAbortsAndNamesOptionAndOrigin"
        status: pass
      - kind: manual_procedural
        ref: "live binary: eight `msg=configuration option=… value=… origin=…` lines; HOLZKUBE_SUDO_WINDOW=5 aborts with `--sudo-window from environment HOLZKUBE_SUDO_WINDOW: invalid value \"5\"`"
        status: pass
    human_judgment: false
  - id: D3
    description: "The data directory follows XDG with a home fallback, is overridden by HOLZKUBE_DATA_DIR and then by --data-dir, and is created 0700 while an existing directory is left untouched"
    requirement: "FOUND-10"
    verification:
      - kind: unit
        ref: "internal/config/datadir_test.go#TestResolvePrecedence"
        status: pass
      - kind: unit
        ref: "internal/config/datadir_test.go#TestEnsureDirCreatesWith0700"
        status: pass
      - kind: unit
        ref: "internal/config/datadir_test.go#TestEnsureDirLeavesAnExistingDirectoryAlone"
        status: pass
      - kind: unit
        ref: "internal/config/config_test.go#TestDataDirPrecedence"
        status: pass
      - kind: manual_procedural
        ref: "live binary against a fresh nested path: `drwx------` on the data directory, `drwxr-xr-x` on the intermediate parent"
        status: pass
    human_judgment: false
  - id: D4
    description: "The server binds loopback without configuration; a bind beyond loopback is accepted but warned about at every start"
    requirement: "FOUND-05"
    verification:
      - kind: unit
        ref: "internal/config/config_test.go#TestBindBeyondLoopbackIsWarned"
        status: pass
      - kind: unit
        ref: "internal/config/config_test.go#TestIsLoopback"
        status: pass
      - kind: manual_procedural
        ref: "live binary with --listen=0.0.0.0:18445 logs `level=WARN msg=\"listening beyond loopback…\"`"
        status: pass
    human_judgment: false
  - id: D5
    description: "Without a configured certificate the first run generates a long-lived self-signed leaf with SANs for both loopback addresses, localhost and the hostname, written 0600, and a second start reuses it rather than regenerating"
    requirement: "FOUND-05"
    verification:
      - kind: unit
        ref: "internal/tlsx/selfsigned_test.go#TestGenerateWritesALoopbackCertificate"
        status: pass
      - kind: unit
        ref: "internal/tlsx/selfsigned_test.go#TestGenerateWritesBothFilesWith0600"
        status: pass
      - kind: unit
        ref: "internal/tlsx/load_test.go#TestEnsureGeneratesOnceAndReusesAfterwards"
        status: pass
      - kind: manual_procedural
        ref: "live binary: two starts against one data directory log the identical fingerprint; cert.pem and key.pem are -rw-------"
        status: pass
    human_judgment: false
  - id: D6
    description: "The logged SHA-256 fingerprint is the string the operator compares against the browser's certificate dialog"
    requirement: "FOUND-05"
    verification:
      - kind: unit
        ref: "internal/tlsx/selfsigned_test.go#TestFingerprintIsInBrowserFormat"
        status: pass
      - kind: manual_procedural
        ref: "openssl x509 -noout -fingerprint -sha256 on the generated cert.pem prints a string byte-identical to the logged sha256_fingerprint"
        status: pass
    human_judgment: true
    rationale: "The format is machine-verified against openssl, which prints exactly what browsers show in their certificate dialog -- but no browser was opened during execution. That a browser displays this same string, and that the operator can find it, is the part only a human looking at the dialog can confirm. This is the plan's own <human-check> and remains outstanding."
  - id: D7
    description: "A supplied certificate and key replace self-generation entirely; a half-given, unreadable or mismatched pair is a hard start failure and never a silent fall back to generation"
    requirement: "FOUND-05"
    verification:
      - kind: unit
        ref: "internal/tlsx/load_test.go#TestEnsureUsesASuppliedPairAndGeneratesNothing"
        status: pass
      - kind: unit
        ref: "internal/tlsx/load_test.go#TestEnsureRefusesABrokenSuppliedPairWithoutFallingBack"
        status: pass
      - kind: unit
        ref: "internal/tlsx/load_test.go#TestEnsureRefusesHalfASuppliedPair"
        status: pass
      - kind: unit
        ref: "internal/config/config_test.go#TestHalfATLSPairIsRefused"
        status: pass
    human_judgment: false
  - id: D8
    description: "--insecure-http is accepted only with a loopback bind; any other bind address refuses the start with a message naming it"
    requirement: "FOUND-05"
    verification:
      - kind: unit
        ref: "internal/tlsx/load_test.go#TestLoopbackGuard"
        status: pass
      - kind: manual_procedural
        ref: "live binary: --insecure-http --listen=0.0.0.0:18445 exits 1 naming the address; --insecure-http --listen=127.0.0.1:18448 serves HTTP, warns, and writes no key material"
        status: pass
    human_judgment: false
  - id: D9
    description: "One build command produces the binary, with the frontend assets built first as a hard dependency, and build:web now installs from the lockfile with npm ci"
    requirement: "FOUND-01"
    verification:
      - kind: manual_procedural
        ref: "`task clean && task build` from a clean tree: npm ci, vite build, then go build -> bin/holzkubed; the binary serves the embedded UI over HTTPS and reports its injected version"
        status: pass
      - kind: other
        ref: "npm --prefix web ci --dry-run exits 0 against the committed lockfile"
        status: pass
    human_judgment: false
  - id: D10
    description: "The release configuration cross-compiles pure-Go archives with checksums for linux/amd64, linux/arm64 and darwin/arm64, building the frontend first"
    requirement: "FOUND-01"
    verification:
      - kind: manual_procedural
        ref: "goreleaser v2.18.0 check passes; `goreleaser release --snapshot --clean` produces three archives plus checksums.txt, and the extracted darwin binary serves the embedded UI and reports version 0.0.1-snapshot"
        status: pass
    human_judgment: false
  - id: D11
    description: "A Go lint gate with gosec is configured in the v2 schema and reports no findings"
    verification:
      - kind: manual_procedural
        ref: "golangci-lint v2.13.1: `config verify` exits 0 and `run` reports `0 issues.` with issue caps lifted"
        status: pass
    human_judgment: false
  - id: D12
    description: "cmd/holzkubed provably does not depend on the Talos root module, and sandbox/ is a separate module invisible to the product build"
    verification:
      - kind: unit
        ref: "internal/depguard_test.go#TestBinaryDependencyWeight"
        status: pass
      - kind: unit
        ref: "internal/depguard_test.go#TestGuardRecognisesTheRootModule"
        status: pass
      - kind: unit
        ref: "internal/depguard_test.go#TestSandboxIsASeparateModule"
        status: pass
    human_judgment: false
  - id: D13
    description: "The build chain does not delete the tracked internal/httpapi/dist/.gitkeep that go:embed needs on a fresh clone"
    verification:
      - kind: other
        ref: "after task clean, task build and a goreleaser snapshot: the file is present and `git status` reports no deletion; `git ls-files` still lists it"
        status: pass
    human_judgment: false

# Metrics
duration: 28 min
completed: 2026-08-28
status: complete
---

# Phase 1 Plan 06: Configuration, TLS and the Build Chain Summary

**Every option settable by flag and `HOLZKUBE_*` variable from a single table that also generates the help and the startup log; a self-signed leaf certificate whose fingerprint is printed the way a browser shows it; `--insecure-http` structurally confined to loopback; and one `task build` that runs `npm ci` before the Go compiler, behind a green `gosec` gate and a module boundary the Talos root module cannot cross.**

## Performance

- **Duration:** 28 min (22 min between the first and last commit)
- **Started:** 2026-08-28T03:24:00Z
- **Completed:** 2026-08-28T03:52:00Z
- **Tasks:** 3
- **Files changed:** 23 (11 created, 12 modified)

## Accomplishments

- **One table is the configuration surface.** The flags, the `HOLZKUBE_` lookup, the help output and the startup log are all generated from `optionTable`. A new switch cannot be added and then be missing from the help or the log — the two places an operator looks when a value is not what they expected — and a test fails if the table and its test-value table disagree.
- **Origins are logged, not just values.** Eight lines at startup, each with option, effective value and `default`/`environment`/`flag`. "The option is wrong" and "the option is being read from somewhere I forgot" have the same symptom and now have different log lines.
- **A misconfiguration is a start failure.** `HOLZKUBE_SUDO_WINDOW=5` used to be silently discarded — the old code caught the parse error and returned the default. It now aborts naming the option, its origin and the value.
- **The certificate stopped claiming to be a CA.** The inherited generator set `IsCA: true` and `KeyUsageCertSign`, which contradicts both the plan and D-04. It is now a leaf that can sign nothing but its own handshake.
- **The fingerprint is comparable.** Colon-separated upper-case hex pairs, verified byte-identical against `openssl x509 -fingerprint -sha256` — the same string a browser shows. The comparison the README asks for now needs no conversion step.
- **Plaintext HTTP cannot leave the machine.** `LoopbackGuard` runs before the store, the audit log or the certificate is touched, so a refusal writes nothing.
- **`task build` was executed, not merely written.** `go-task`, `golangci-lint` and `goreleaser` are absent from this host; all three were installed into a scratch `GOBIN` so the Taskfile, the lint config and the release config could be run rather than reasoned about. `task clean && task build` from a clean tree, `goreleaser release --snapshot --clean`, and the extracted darwin archive serving the embedded UI are all things that happened.
- **The lint gate is green on the day it lands.** Nine linters including `gosec`, no linter disabled wholesale, and `0 issues.` — because the eleven findings it reported on existing code were resolved in the commit before it.
- **The module boundary exists before there is anything to keep out.** `sandbox/` is its own module and `TestBinaryDependencyWeight` walks the real dependency graph of `./cmd/holzkubed`. Phase 1 has no Talos import, so the guard would pass vacuously — hence a negative control that pins the classification against the imports phase 2 will actually add.

## Task Commits

1. **Task 1 RED — configuration tests** — `de2a623` (test)
2. **Task 1 GREEN — option table, precedence, XDG data directory, startup log** — `742fca6` (feat)
3. **Task 2 RED — certificate and loopback guard tests** — `fdefc81` (test)
4. **Task 2 GREEN — leaf certificate, reuse, fingerprint format, guard** — `01d367e` (feat)
5. **Task 3, move 1/3 — resolve the findings the incoming lint gate reports** — `8f8e39e` (style)
6. **Task 3, move 2/3 — build chain, lint gate, release config, module boundary** — `8340a93` (chore)
7. **Task 3, move 3/3 — README** — `6576f29` (docs)
8. **Follow-up — route package-level slog through the configured logger** — `8abdf71` (fix)

## Files Created/Modified

**Configuration**
- `internal/config/config.go` — `optionTable`, `LoadWith(args, env, home)`, `LogEffective`, `Usage`, `IsLoopback`, the `Origin` type, and `ErrHelp`/`ErrVersion`
- `internal/config/datadir.go` — `Resolve(env, home, override)` and `EnsureDir` with `0700`, parents at `0755`
- `internal/config/config_test.go`, `internal/config/datadir_test.go` — 11 tests, environment and home injected as parameters

**TLS**
- `internal/tlsx/selfsigned.go` — `Generate(dir, hostname)`, `Fingerprint`, and the store's atomic write sequence for key material
- `internal/tlsx/load.go` — `Ensure(cfg)`, `LoopbackGuard(listen, insecure)`, `Load(cert, key)`
- `internal/tlsx/selfsigned_test.go`, `internal/tlsx/load_test.go` — 9 tests

**Composition root**
- `cmd/holzkubed/main.go` — `version` for ldflags injection, the `--help`/`--version` sentinels, a `slog.LevelVar` so `--log-level` can be applied after resolution, `slog.SetDefault`, `cfg.LogEffective`, `config.EnsureDir`, `tlsx.LoopbackGuard` before anything is opened, and `tlsx.Ensure` in place of the old two-branch TLS block. The startup audit-chain verdict that plan 03 wired in is untouched.

**Build, lint, release**
- `Taskfile.yml` — `npm ci`, and `test`, `test:web`, `lint:go`, `lint:web`, `lint`, `fmt`, `clean`, `release:snapshot`
- `.golangci.yml` — v2 schema, nine linters, three rule-and-place exclusions, issue caps lifted
- `.goreleaser.yaml` — v2 schema, `CGO_ENABLED=0`, three targets, checksums, frontend built in the before hook
- `sandbox/go.mod`, `sandbox/README.md` — the separate module and why it exists
- `internal/depguard_test.go` — `TestBinaryDependencyWeight`, its negative control, and `TestSandboxIsASeparateModule`
- `README.md` — build, the full option table, the container volume, and the blast radius
- `.gitignore` — `/.task/`, go-task's staleness cache

**Annotated for the lint gate (no behaviour change)**
- `internal/store/fsstore/fsstore.go`, `internal/store/migrate/migrate_test.go`, `internal/httpapi/middleware/recover.go`, `internal/httpapi/middleware/chain.go`, `internal/httpapi/endtoend_test.go`, `internal/httpapi/handlers/account_test.go`

## Decisions Made

### The `system/status` gap: left open, deliberately

Plan 01-05's dashboard was specified to show version, bind address and data directory from `GET /api/v1/system/status`; that endpoint returns `setup_required` and `audit_chain` and nothing else. 01-05 refused to invent the fields client-side and recorded it as a blocker. The wave context handed the choice to this plan: extend the endpoint, or leave the gap documented.

**Chosen: leave it open, documented** (`deferred-items.md` item 1). Three reasons, in order of weight:

1. **The endpoint answers before authentication** — it has to, because `setup_required` is what the browser asks before there is an account. The data directory path is the most sensitive string this process holds: from phase 2 it names the location of cluster CA private keys. Naming it to an unauthenticated caller, on an installation that a deliberate `--listen 0.0.0.0` makes reachable from a whole home network, is a disclosure with no matching benefit. Gating just those fields on a session would make the response shape conditional, which is a worse contract than either alternative.
2. **No consumer remains inside phase 1.** 01-05 is complete. An extension made now ships unread and untested, with its shape guessed rather than driven by the view that needs it. The dashboard would still not show the fields, so the gap the user actually sees would not close.
3. **`docs/api-contract.md` is the artefact five parallel plans coded against.** Widening it on the last day of the phase, in two files owned by two other plans, trades a cosmetic card for the one document that made the wave work.

What the operator loses is nothing they cannot already get: the bind address and the data directory are printed at every start with their origin, and `--version` prints the version. What is genuinely deferred is putting them *in the UI*, and that is now a written decision with a shape to implement rather than a silent omission.

### The rest

- **`IsCA: false`.** The inherited generator marked the certificate a CA and gave it `KeyUsageCertSign`. The plan says `IsCA: false` and D-04 says there is no private CA. A self-signed leaf that a browser is asked to trust by exception does not need to be one, and one that could mint others if its key escaped is strictly worse.
- **Fingerprint format.** The operator is asked to compare a string by eye against a browser dialog. Lower-case unseparated hex made that comparison harder than the security step deserves. Verified byte-identical against `openssl x509 -fingerprint -sha256`.
- **Port 8443.** The open question CONTEXT.md left to discretion: the common unprivileged HTTPS convention, no collision with 8080-range dev servers, no root required — which D-02's "works on darwin and linux without root" presupposes.
- **A wildcard bind warns, never refuses.** FOUND-05 wants loopback as the default, not as a cage. The flagged planner assumption is upheld: "by default `127.0.0.1`" means the absence of *any* setting, and exposure stays a legal but loud choice.
- **A non-localhost host name counts as not-loopback in the plaintext guard.** Deciding otherwise needs DNS at startup; a guard that cannot be certain has to be the careful one.
- **The lint gate landed green.** See deviation 3. A gate that fails on the day it arrives is bypassed within a week.
- **Issue caps lifted.** golangci-lint defaults to 50 issues per linter and 3 per rule. The first run reported 17 `gosec` findings; with the caps lifted, the same tree had 20. "Nearly clean" under a cap is a statement about the cap.
- **`--version` and `--help` are outside the option table.** Every entry in that table has a flag *and* an environment variable, and the table's tests enforce it. A `HOLZKUBE_VERSION` variable would be nonsense, so they are handled as sentinels returned by `Load` instead.

## Deviations from Plan

### 1. [Rule 1 - Bug] The generated certificate was a certificate authority

- **Found during:** Task 2, reading the inherited `selfsigned.go`
- **Issue:** `IsCA: true` and `KeyUsage: DigitalSignature | CertSign`. The plan specifies `IsCA: false`; D-04 specifies no private CA and no trust import. A key that escapes could sign further certificates rather than only impersonate this one server.
- **Fix:** `IsCA: false`, `KeyUsage: DigitalSignature` only, `BasicConstraintsValid` kept so the absence is asserted rather than merely unstated.
- **Verification:** `TestGenerateWritesALoopbackCertificate` asserts `IsCA` false, `BasicConstraintsValid` true and no `KeyUsageCertSign`; the live binary serves TLS with it and `curl -k` completes the handshake.
- **Committed in:** `01d367e`

### 2. [Rule 1 - Bug] An unparsable environment value was silently replaced by the default

- **Found during:** Task 1, reading the inherited `config.go`
- **Issue:** `envDuration` and `envBool` caught their parse error and returned the fallback. `HOLZKUBE_SUDO_WINDOW=5` therefore produced a five-minute sudo window while the operator believed they had set something else — a security window, silently not what was asked for.
- **Fix:** Every value goes through the option table's parser, and a failure aborts the start naming the option, its origin (`environment HOLZKUBE_SUDO_WINDOW` or `flag`) and the offending value.
- **Verification:** `TestUnparsableValueAbortsAndNamesOptionAndOrigin` over five cases; confirmed live.
- **Committed in:** `742fca6`

### 3. [Rule 3 - Blocking] The lint gate reported eleven findings on existing code

- **Found during:** Task 3
- **Issue:** Enabling nine linters on a codebase written across five plans produced 65 findings. Exclusions broad enough to reach zero would have voided the rule; leaving the gate red on arrival would have made `task lint:go` decoration.
- **Fix:** Two layers. **Exclusions**, each naming a rule and a place: golangci-lint's own `std-error-handling` preset (deferred `Close`, `Fprintf` to a buffer); `gosec` and `bodyclose` off in `_test.go`, where wrong-permission files and traversal-shaped paths are the fixtures that prove the guards reject them; `G304` off only in `internal/store`, `internal/audit` and `internal/tlsx`, the three packages whose entire job is taking a file path in a variable — exactly the exemption 01-01 predicted; `G104` off as a demonstrated duplicate of `errcheck`. `gosec` stays fully on for every other rule in every production file. **Then the residual eleven were resolved**, not excluded: three doc comments on `fsstore` accessors, three `//nolint` with stated reasons at genuine false positives (a recovered panic value compared by identity is not a wrapped error; `contextcheck` cannot see through the recover closure; the response recorder stays unexported on purpose), one dead test helper deleted, one unused parameter renamed, one dead assignment dropped, and one unchecked type assertion in a test made two-valued.
- **Files modified outside `files_modified`:** `internal/store/fsstore/fsstore.go`, `internal/store/migrate/migrate_test.go`, `internal/httpapi/middleware/recover.go`, `internal/httpapi/middleware/chain.go`, `internal/httpapi/endtoend_test.go`, `internal/httpapi/handlers/account_test.go`. All comment-only or dead-code removals; no executable line changed meaning.
- **Verification:** `golangci-lint run` reports `0 issues.` with caps lifted; `go test ./... -race` and the 60 frontend tests pass unchanged.
- **Committed in:** `8f8e39e` (annotations, first) and `8340a93` (the gate, second), in that order so the gate is green at every commit.

### 4. [Rule 2 - Missing Critical] `--log-level` governed only part of the output

- **Found during:** Task 2, watching a live startup log
- **Issue:** `internal/auth/argon.go` logs its calibration through the package-level `slog` functions, which go to `slog.Default()` — a different handler at a different level. `--log-level` was quietly true of some of the output only.
- **Fix:** `slog.SetDefault(logger)` at the composition root, right after the level is applied. One line, in this plan's own file, rather than threading a logger through another plan's API.
- **Verification:** `--log-level=warn` now produces completely silent output on a successful start, the calibration line included.
- **Committed in:** `8abdf71`

### 5. [Rule 3 - Blocking] `go-task` produced an untracked cache directory

- **Found during:** Task 3, after the first real `task build`
- **Issue:** go-task writes `.task/` for its `sources:`/`generates:` staleness checks. `.gitignore` is not in this plan's declared files, and leaving generated state untracked is the failure mode `.gitignore` exists to prevent.
- **Fix:** One line, `/.task/`, next to the other tooling-state entries.
- **Verification:** `git check-ignore -v .task/` matches; `git status` is clean after a build.
- **Committed in:** `8340a93`

### 6. [Rule 1 - Bug] The goreleaser archive glob matched nothing

- **Found during:** Task 3, in the first snapshot run
- **Issue:** `docs/**/*` printed `no files matched` three times. goreleaser's `**` requires an intervening directory, and `docs/` contains `api-contract.md` at its top level, so every archive would have shipped without the API contract.
- **Fix:** `docs/*`. Caught only because the snapshot was actually run rather than only `check`ed.
- **Verification:** `tar tzf` on the archive lists `README.md`, `docs/api-contract.md` and `holzkubed`.
- **Committed in:** `8340a93`

---

**Total deviations:** 6 (2 Rule 1 bugs in inherited code, 1 Rule 1 bug in this plan's own new config, 1 Rule 2 missing-critical, 2 Rule 3 blocking).
**Impact on plan:** No scope creep. Deviations 1 and 2 are security defects in code this plan inherited and was rewriting anyway. Deviation 3 is the only one touching files outside `files_modified` — six files, all comment or dead-code changes, none overlapping an unfinished plan, and it is what makes the lint gate mean something. Deviation 6 would have shipped every release archive without the API contract.

## What could and could not be executed

The plan asks plainly which build commands ran. `go-task`, `golangci-lint` and `goreleaser` were **not** installed on this host. Rather than validate the three configuration files by reading them — which is how plan 01-01's YAML bug survived — all three tools were installed at their pinned versions into a scratch `GOBIN` outside the repository and the configurations were **run**:

| Command | Result |
|---|---|
| `task --list` | 12 targets, all described |
| `task clean` | removes `bin/` and `dist/`, keeps `internal/httpapi/dist/.gitkeep` |
| `task build` (from a clean tree) | `npm ci` → `vite build` → `go build` → `bin/holzkubed`, 7 s |
| `task test:web` | 5 files, 60 tests, all pass |
| `task lint` | golangci-lint `0 issues.`, Biome 41 files clean |
| `task fmt` | no-op on a formatted tree |
| `golangci-lint config verify` | exit 0 against the real v2.13.1 schema |
| `goreleaser check` | config validated by the real v2.18.0 |
| `goreleaser release --snapshot --clean` | 3 archives + `checksums.txt`; the extracted darwin binary runs |
| `task dev` | **not run** — it starts a long-lived Vite dev server |
| `task release:snapshot` | the underlying `goreleaser release --snapshot --clean` was run directly; the target wrapping it was not invoked separately |

**These tools are not on the host afterwards.** They live in a scratch directory outside the repository and will disappear with it. `deferred-items.md` item 4 carries the install commands.

## Issues Encountered

- **The first `gosec` run under-reported itself.** 17 findings became 20 once the default caps were lifted; `errcheck` had been flooding the per-linter cap. The caps are now `0`, which is the only setting under which the number means anything.
- **A `python3 str.replace(old, new, 1)` hit the wrong `filepath.WalkDir` closure** in `migrate_test.go` — two closures in one file share a signature. `go vet` caught it immediately (`undefined: d`); reverted and re-applied against a longer, unique anchor.
- **Deleting the dead `sessionCookie` test helper left `net/url` unused.** The compiler said so; the import went with it.
- **`--log-level=warn` also hides the certificate fingerprint,** since that line is `INFO`. Left as is — the level means what it says — and the README now points at `openssl x509 -fingerprint -sha256`, which reproduces the same string.
- **Backgrounding the server with `&` and `kill %1` hangs a non-interactive shell.** Switched to a subshell writing its PID to a file. No effect on the product.

## User Setup Required

None — no external service configuration. For the full build chain the operator needs `go-task`, `golangci-lint` v2.13.1 and `goreleaser` v2.18.0 installed; `go build` and `go test` need nothing beyond the Go toolchain, and `deferred-items.md` item 4 has the three install commands.

## Next Phase Readiness

**Ready.** Phase 1's first success criterion is met end to end: one binary, one build command, no runtime dependencies, HTTPS on the loopback address with a certificate the operator can actually verify.

For phase 2 specifically:

- **The module boundary is set and guarded before the first Talos import exists.** Adding `pkg/machinery` to `go.mod` is expected and passes; adding anything else from the Talos root module fails `TestBinaryDependencyWeight` with a message naming the offending packages and pointing at `sandbox/`.
- **`--dry-run` (FOUND-12) has a place to land:** one entry in `optionTable`, and the flag, the environment variable, the help line and the startup log line follow.
- **The lint gate is green now,** so the first finding phase 2 introduces is visible as *its* finding rather than as noise in a pile of 65.
- **A release is one command.** `goreleaser release` cross-compiles three targets with checksums; the only missing piece for a real release is a git tag.

**Concerns to carry forward:**

1. **The `system/status` gap is a decision, not an accident** — recorded above and in `deferred-items.md` item 1. If the dashboard should show version, bind address and data directory, that is a deliberate contract extension including the question of whether operational fields are gated on a session.
2. **The certificate fingerprint has not been compared in an actual browser.** It is byte-identical to what `openssl` prints, which is what browsers show, but D6 stays `human_judgment: true` and remains this phase's outstanding manual check.
3. **The build tools are absent from the host.** Everything here was verified with tools installed into a scratch directory; the operator's next `task build` needs a real install.
4. **The frontend bundle is 573 kB,** over vite's warning threshold. Not a defect for an embedded single-binary UI, but worth a decision rather than a warning everyone learns to scroll past (`deferred-items.md` item 2).

## Self-Check: PASSED

**Files claimed as created — all present on disk:**
`internal/config/config_test.go`, `internal/config/datadir.go`, `internal/config/datadir_test.go`, `internal/tlsx/load.go`, `internal/tlsx/load_test.go`, `internal/tlsx/selfsigned_test.go`, `internal/depguard_test.go`, `.golangci.yml`, `.goreleaser.yaml`, `sandbox/go.mod`, `sandbox/README.md` — FOUND.

**Commits — all eight task commits plus this one present in `git log`:** `de2a623`, `742fca6`, `fdefc81`, `01d367e`, `8f8e39e`, `8340a93`, `6576f29`, `8abdf71` — FOUND.

**Plan-level verification re-run at close-out:**

| Check | Result |
|---|---|
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test ./... -count=1 -race` | PASS (11 packages) |
| `golangci-lint run` | PASS (`0 issues.`) |
| `task clean && task build` from a clean tree | PASS |
| `npm --prefix web run test` | PASS (5 files, 60 tests) |
| `npm --prefix web run lint` | PASS (41 files) |
| `npm --prefix web ci --dry-run` | PASS (exit 0) |
| `goreleaser check` and `goreleaser release --snapshot --clean` | PASS (3 archives + checksums) |
| `go list ./...` lists no package under `sandbox/` | PASS |
| `internal/httpapi/dist/.gitkeep` tracked and present after builds | PASS |
| Binary against a fresh data directory answers over HTTPS on loopback | PASS |
| Startup log: 8 option lines with value and origin, 1 fingerprint line | PASS |

**Task acceptance criteria — all re-run:**

| Criterion | Result |
|---|---|
| `go test ./internal/config/... -count=1` | PASS |
| `internal/config/config.go` contains `HOLZKUBE_` | PASS (3) |
| `grep -cF '0.0.0.0' internal/config/config.go` is 0 | PASS |
| `grep -icE 'yaml\|toml\|viper\|configFile\|config\.file' internal/config/config.go` is 0 | PASS |
| Flag beats environment; `--data-dir` beats `HOLZKUBE_DATA_DIR` beats `XDG_DATA_HOME` | PASS |
| Data directory created `0700`; unparsable duration aborts naming option and origin | PASS |
| One startup line per option with value and origin | PASS |
| `go test ./internal/tlsx/... -count=1` | PASS |
| `selfsigned.go` contains `DNSNames` and `IPAddresses` | PASS |
| `load.go` contains `func LoopbackGuard` | PASS |
| First run writes `cert.pem`/`key.pem` at `0600`; second run reuses them | PASS |
| `--tls-cert` without `--tls-key` refuses, naming the counterpart | PASS |
| `--insecure-http` off loopback refuses naming the address; on loopback starts and warns | PASS |
| Fingerprint logged as colon-separated hex, byte-equal to `openssl x509 -fingerprint -sha256` | PASS |
| `go test ./internal -run TestBinaryDependencyWeight -v -count=1` | PASS (no SKIP) |
| `.golangci.yml` contains `version: "2"` and `gosec` | PASS |
| `Taskfile.yml` binds `build:go` to `build:web` via `deps:` | PASS |
| `Taskfile.yml` calls `npm ci`, and it is satisfiable against the lockfile | PASS |
| `sandbox/go.mod` declares a module path different from the root | PASS |
| `go list ./...` lists nothing under `sandbox/` | PASS |
| One build target call from a clean tree produces a running binary with embedded assets | PASS |
| `README.md` has `## Build`, `## Run` and `## Configuration` | PASS |

**Outstanding by design:** the plan's `<human-check>` — comparing the logged fingerprint against the browser's certificate dialog — is recorded as coverage item D6 with `human_judgment: true`. It was verified against `openssl`, not against a browser.

**Hygiene:** no file deletions in this plan's commit range other than the dead `sessionCookie` test helper, which is documented; `.gsd/`, `.planning/milestone.lock`, `/.task/`, `bin/`, `dist/` and `web/node_modules/` are untracked and ignored; `internal/httpapi/dist/.gitkeep` is still tracked.

---
*Phase: 01-foundation-skeleton*
*Completed: 2026-08-28*
