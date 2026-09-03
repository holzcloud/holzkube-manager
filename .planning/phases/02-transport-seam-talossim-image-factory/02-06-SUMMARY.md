---
phase: 02-transport-seam-talossim-image-factory
plan: 06
subsystem: api
tags: [image-factory, talos, react, zod, tanstack-query, rfc9457, audit-redaction, vitest]

# Dependency graph
requires:
  - phase: 02-02
    provides: the Image Factory client, the fixtures, and the minted `upstream` problem family
  - phase: 02-04
    provides: asset URL derivation, version filtering and the broken table, the warning surface, the store schema v2 schematics directory, and the `## Schematics` contract
provides:
  - The seven contracted schematic routes, served from a running binary behind the session gate
  - The `Factory *imagefactory.Client` field on `httpapi.Deps`, set inside the composition-root struct literal
  - Audit redaction allowlist entries for `schematic.create` (name, talos_version) and `schematic.delete` (nothing)
  - The Images screen — version-scoped extension catalog with no free-text field, live installer/initramfs warning, saved-schematic table, asset references per architecture, delete through the existing sudo gate
  - `model.Schematic.ProbeReason` — what the Factory said when it refused
  - The third clause of the Route Registration Rule, covering a `Deps` field addition
affects: [02-07, 03-inventory, 06-node-lifecycle, upgrades]

actuals:
  tokens: 36600
  tasks: 3
  commits: 5

tech-stack:
  added: []
  patterns:
    - "A client-side predicate with the server's verbatim text, guarded by a test that reads the other language's source"
    - "Three-state verdicts where a boolean would merge two different repairs"
    - "Named group rows (fieldset + aria-label) for labelled values with actions, since dd is name-prohibited"

key-files:
  created:
    - web/src/routes/images.tsx
    - web/src/routes/images.test.tsx
    - web/src/components/SchematicWarnings.tsx
  modified:
    - internal/httpapi/handlers/schematics.go
    - internal/httpapi/handlers/schematics_test.go
    - internal/httpapi/router.go
    - cmd/holzkubed/main.go
    - internal/audit/redact.go
    - internal/audit/redact_test.go
    - internal/model/model.go
    - internal/imagefactory/warnings_test.go
    - docs/api-contract.md
    - web/src/api.ts
    - web/src/components/Sidebar.tsx
    - web/src/App.tsx
    - web/src/hooks/useTheme.ts

key-decisions:
  - "The schematic wire form normalises nil collections to [] — a null reads as 'the server did not say', which is not 'there are none'; nothing persisted changes"
  - "model.Schematic gains ProbeReason, set only for ErrSchematicNotBuildable — a probe that could not reach the Factory says nothing about the schematic, and a reason recorded for it would read as one"
  - "ProbeReason is additive and unversioned: an older record decodes with an empty reason, which is exactly what it means, so a schema bump would force a backup and a rewrite of every data directory for nothing"
  - "An empty chosenVersion is 'nothing chosen', not a choice — the picker mounts before the version list arrives and reported its own empty starting value back as a selection"
  - "The live warning text is the server's sentence transcribed, and TestWarningDetailsMatchTheUI reads the component from the Go side so 'they cannot disagree' is enforced rather than asserted"
  - "The asset panel's architecture is remembered in localStorage rather than defaulted — a hardcoded default is a bug that only appears on someone else's machine"
  - "The Sidebar's phase-1 claim that the navigation was complete is corrected rather than left standing: a requirement naming a screen wins over a claim that the list was closed"

patterns-established:
  - "Cross-language constant drift guard: the authoring side reads the consuming side's source and fails on divergence, rather than a comment asserting parity"
  - "Not-usable is never one state: usable / refused-with-reason / never-probed are rendered as three, because the last two carry different repairs"

requirements-completed: [FACT-01, FACT-02, FACT-03, FACT-04, FACT-05]

coverage:
  - id: D1
    description: "Seven contracted routes served, session-gated, DELETE destructive, both mutating routes carrying their audit action token"
    requirement: FACT-01
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go (full package)"
        status: pass
      - kind: manual_procedural
        ref: "bin/holzkubed --insecure-http; GET /api/v1/factory/versions, /factory/extensions, /schematics answer 401 (registered, session-gated); DELETE /schematics/x answers 403 (CSRF precondition)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Audit redaction allowlist: schematic.create permits name and talos_version only; schematic.delete permits nothing; a kernel argument is written redacted"
    requirement: FACT-01
    verification:
      - kind: unit
        ref: "internal/audit/redact_test.go#TestRedact (schematic.create kernel-arg case)"
        status: pass
    human_judgment: false
  - id: D3
    description: "Extension selection comes only from the version-scoped catalog; there is no text input through which an unvalidated name could reach the Factory"
    requirement: FACT-01
    verification:
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#offers extensions only from the catalog and has no field to type one into"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#sends only names that came out of the fetched catalog"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go (unknown-extension rejection, zero Factory POSTs)"
        status: pass
    human_judgment: false
  - id: D4
    description: "Changing the Talos version reloads the catalog and reports by name an extension the new version does not list, rather than carrying it over"
    requirement: FACT-01
    verification:
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#reports by name an extension the new version does not have, rather than carrying it over"
        status: pass
    human_judgment: false
  - id: D5
    description: "Usability is three distinguishable states and a refusal shows the reason the probe gave; created and usable are never merged"
    requirement: FACT-02
    verification:
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#renders three distinguishable usability states and the reason for a refusal"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#keeps \"created\" and \"usable\" apart on the create result"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestCreateWithAFailingProbeIsStoredNotUsable, #TestUnreachableProbeRecordsNoReason"
        status: pass
    human_judgment: false
  - id: D6
    description: "ISO, PXE, disk-image, cmdline and installer references are readable and copyable per architecture, with secure boot as a control and the installer adjacent to the ISO"
    requirement: FACT-03
    verification:
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#renders the installer reference on the same panel as the ISO URL"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#changes every asset URL when the architecture control changes"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#adds the secure-boot suffix to the rendered ISO URL when the toggle is on"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#copies a reference to the clipboard"
        status: pass
    human_judgment: false
  - id: D7
    description: "A kernel argument or META entry raises the installer/initramfs warning while authoring, before any create request, in the server's own wording"
    requirement: FACT-04
    verification:
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#warns about kernel arguments while they are being typed, before any create request"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#warns about META values while they are being typed"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/warnings_test.go#TestWarningDetailsMatchTheUI"
        status: pass
    human_judgment: false
  - id: D8
    description: "Prereleases require an explicit opt-in and a broken version renders disabled with its reason; the default is the newest stable, never the last element of the upstream list"
    requirement: FACT-05
    verification:
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#defaults to the newest stable version, not the last element of the upstream list"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#hides prereleases until they are explicitly asked for"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#renders a broken version disabled and says why it is listed"
        status: pass
    human_judgment: false
  - id: D9
    description: "Deleting a schematic goes through the existing sudo dialog and this screen adds no confirmation of its own"
    requirement: FACT-01
    verification:
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#deletes through the existing sudo dialog and adds no confirmation of its own"
        status: pass
    human_judgment: false
  - id: D10
    description: "The Images screen renders and behaves correctly in a real browser against a running holzkubed"
    verification: []
    human_judgment: true
    rationale: "Everything above is jsdom plus a handler-level integration suite. No browser automation exists in this project, and the outstanding phase-1 concern about the embedded UI being unverified in a browser now covers a second screen. A person must open /images once against the live binary."

# Metrics
duration: 33 min
completed: 2026-08-29
status: complete
---

# Phase 02 Plan 06: Image Factory Routes and the Images Screen Summary

**The seven contracted schematic routes with their composition-root wiring and audit allowlist entries, plus an Images screen that assembles a schematic from a version-scoped catalog, warns about the installer/initramfs asymmetry while the operator is still typing, and reports usability as three states rather than two.**

## Performance

- **Duration:** 33 min of execution (first task commit `857f4d3` 08:20 to final task commit `5be5360` 08:53, local time)
- **Started:** 2026-08-29T06:20:08Z
- **Completed:** 2026-08-29T06:53:16Z
- **Tasks:** 3
- **Files modified:** 19

### Executed across two agents

This plan was executed by two agents. The first was terminated mid-run by an
infrastructure failure — the host machine slept — after committing Task 1 and
while partway through Task 2, leaving uncommitted work in the tree and no
Task 2 tests. This summary was written by the continuation agent, which verified
the inherited state before trusting it: it re-ran the committed Task 1 suite,
read every uncommitted file against Task 2's acceptance criteria, and kept, fixed
or rewrote each on the merits. Two real defects in the inherited draft were found
and are documented under Deviations; neither was a test artefact.

## Accomplishments

- Seven routes served from a running binary, all session-gated, `DELETE` alone
  destructive, both mutating routes carrying their contracted audit action token
- The audit redaction allowlist closed for `schematic.create` (name and
  `talos_version`, nothing else) and `schematic.delete` (nothing) — the one
  entry D-16 makes permanent, since the archive has no deletion path
- An authoring screen with no free-text extension field: the picker is
  checkboxes over the catalog fetched for the selected Talos version, and a
  selection the new version's catalog does not list is dropped and reported by
  name rather than sent to the Factory
- The installer/initramfs warning rendered live from the form, in the server's
  own wording, with a cross-language test that fails if the two ever drift
- A saved-schematics table with three usability states, and a detail view
  carrying the full id, the Factory-canonical document, the warnings and the
  asset panel — ISO, installer, PXE, disk image and cmdline, per architecture,
  each copyable
- `model.Schematic.ProbeReason`: the sentence the Factory gave when it refused,
  which is what turns a red badge into something an operator can act on

## Task Commits

1. **Task 1: The seven contracted routes, their wiring, and the allowlist entries**
   — `857f4d3` (test, RED) and `12b65d4` (feat, GREEN) — committed by the first agent
2. **Backend wire-form fix found while starting Task 2** — `93bddfc` (fix)
3. **Task 2: The authoring screen — a catalog, never a free-text field** — `a1b81de` (feat)
4. **Task 3: The saved schematics list, the asset references, and the honest usability verdict** — `5be5360` (feat)

**Plan metadata:** see the `docs(02-06)` commit that carries this file.

## Files Created/Modified

- `internal/httpapi/handlers/schematics.go` — `SchematicRoutes` and the seven handlers; wire-form normalisation; the probe reason
- `internal/httpapi/handlers/schematics_test.go` — handler suite against a fake Factory, including the new outage-while-probing path
- `internal/httpapi/router.go` — one `Deps` field, `Factory *imagefactory.Client`
- `cmd/holzkubed/main.go` — Factory client built and set inside the `httpapi.Deps{…}` literal
- `internal/audit/redact.go` / `redact_test.go` — the two allowlist entries and the argument for them
- `internal/model/model.go` — `Schematic.ProbeReason`
- `internal/imagefactory/warnings_test.go` — `TestWarningDetailsMatchTheUI`, the cross-language drift guard
- `docs/api-contract.md` — Route Registration Rule third clause; `probe_reason` documented
- `web/src/api.ts` — zod schemas and typed calls for all seven routes, the sudo action label for delete
- `web/src/routes/images.tsx` — the whole screen: authoring, saved list, detail, asset panel
- `web/src/routes/images.test.tsx` — 18 cases covering FACT-01 through FACT-05
- `web/src/components/SchematicWarnings.tsx` — pure warning renderer plus the live predicate
- `web/src/components/Sidebar.tsx` — the Images area, and the corrected completeness claim
- `web/src/App.tsx` — the route under `authenticatedRoute`
- `web/src/hooks/useTheme.ts` — the corrected statement about what localStorage holds

## Decisions Made

See `key-decisions` in the frontmatter. The two that reach beyond this plan:

- **`ProbeReason` is additive and unversioned.** A record written before the
  field decodes with an empty reason, which is precisely what an empty reason
  means. Bumping `migrate.CurrentVersion` would force a backup and a rewrite of
  every data directory to add a field whose absence is already correct.
- **The cross-language drift guard runs from the Go side.** vitest is rooted at
  `web/` and refuses to read outside it; allowing it would mean loosening the
  bundler's filesystem allowlist in the shared build config, which is a real
  cost for a test-only convenience. The Go test reads the component instead,
  which also puts the check where the text is authored.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] The version picker pinned the screen to no version at all**

- **Found during:** Task 2
- **Issue:** `version = chosenVersion ?? versions.data?.newest_stable ?? ''`. The
  picker mounts before the versions query resolves, and the Select reported its
  own empty starting value back through `onValueChange`. `''` is not nullish, so
  `??` kept it: the version list rendered, the default never applied, the
  extension catalog was never fetched (`enabled: version !== ''`), and nothing
  anywhere reported a failure. This was not a test artefact — it is what the
  screen did.
- **Fix:** An empty `chosenVersion` is treated as "nothing chosen" rather than
  as a choice.
- **Files modified:** `web/src/routes/images.tsx`
- **Verification:** `images.test.tsx#defaults to the newest stable version…` and
  every catalog-dependent case; all ten Task 2 cases failed before the fix and
  pass after it.
- **Committed in:** `a1b81de`

**2. [Rule 1 - Bug] The live warning claimed to be the server's sentence and was not**

- **Found during:** Task 2
- **Issue:** `SchematicWarnings.tsx` carried its own paraphrase under a comment
  saying it was "deliberately the same sentence the server sends". The plan
  requires the live and server-returned warnings to be unable to disagree in
  wording; two different sentences about one condition read as two problems.
- **Fix:** Both details transcribed verbatim from `imagefactory.Warnings`, and
  `TestWarningDetailsMatchTheUI` added, which reads the component and the API
  module and fails if either sentence or either code drifts.
- **Files modified:** `web/src/components/SchematicWarnings.tsx`, `internal/imagefactory/warnings_test.go`
- **Verification:** `go test ./internal/imagefactory/ -run TestWarning`
- **Committed in:** `a1b81de`

**3. [Rule 2 - Missing Critical] No probe reason was stored, so "not usable" was unactionable**

- **Found during:** Task 3
- **Issue:** Task 3 requires a not-usable row to show the reason the probe gave.
  `model.Schematic` had `Usable` and `ProbedAt` and no reason field, so the
  screen could only have shown a red badge with no stated cause — and a
  schematic naming an extension that does not exist and one asked for at a
  version that never had it are the same badge and two different repairs.
- **Fix:** `ProbeReason` added to the model, set in the create handler only for
  `ErrSchematicNotBuildable` (an unreachable Factory says nothing about the
  schematic), documented in `docs/api-contract.md`, surfaced through the zod
  schema and rendered next to the verdict. The fake Factory gained the
  outage-while-probing path, which had no reproduction before.
- **Files modified:** `internal/model/model.go`, `internal/httpapi/handlers/schematics.go`, `internal/httpapi/handlers/schematics_test.go`, `docs/api-contract.md`, `web/src/api.ts`, `web/src/routes/images.tsx`
- **Verification:** `TestCreateWithAFailingProbeIsStoredNotUsable`, `TestUnreachableProbeRecordsNoReason`, `images.test.tsx#renders three distinguishable usability states…`
- **Committed in:** `5be5360`

**4. [Rule 1 - Bug] Schematic collections encoded as `null`**

- **Found during:** Task 2 (inherited from the interrupted agent, judged correct and kept)
- **Issue:** A nil `Extensions`, `KernelArgs` or `Meta` encoded as JSON `null`,
  which reads to a client as "the server did not say" rather than "there are
  none" — the same distinction the contract already draws for `warnings` and
  the version buckets, and one the zod schemas depend on.
- **Fix:** `schematicOut` normalises the wire form on create, list and get.
  Nothing persisted changes and the id stays the hash of the Factory's own
  canonical document.
- **Files modified:** `internal/httpapi/handlers/schematics.go`, `internal/httpapi/handlers/schematics_test.go`
- **Verification:** `TestSchematicCollectionsAreArraysNeverNull`
- **Committed in:** `93bddfc`

**5. [Rule 2 - Missing Critical] Two doc comments made claims that this plan falsified**

- **Found during:** Tasks 2 and 3
- **Issue:** `Sidebar.tsx` stated "Every area this product will ever have is
  listed here from phase 1 on"; this plan adds one. `useTheme.ts` stated
  "localStorage holds the theme and nothing else (threat T-01-32)"; the asset
  panel remembers the architecture there.
- **Fix:** Both corrected in the commit that falsified them, with the reason
  rather than a silent edit. Neither weakens T-01-32: an architecture is a
  preference, not something a session or a credential can be reconstructed from.
- **Files modified:** `web/src/components/Sidebar.tsx`, `web/src/hooks/useTheme.ts`
- **Verification:** read; no behavioural change
- **Committed in:** `a1b81de`, `5be5360`

**6. [Rule 1 - Bug] A stale dropped-extension report and an orphaned test comment**

- **Found during:** Tasks 2 and 3
- **Issue:** The "removed from the selection" report persisted across a later
  version change where nothing was dropped. Separately, the interrupted agent's
  new Go test had been inserted between an existing doc comment and the function
  it documents.
- **Fix:** The report is cleared on version change; the doc comment was moved
  back onto its function.
- **Files modified:** `web/src/routes/images.tsx`, `internal/httpapi/handlers/schematics_test.go`
- **Verification:** `go test ./internal/httpapi/handlers/`, `npm --prefix web run test`
- **Committed in:** `93bddfc`, `a1b81de`

---

**Total deviations:** 6 auto-fixed (4 bugs, 2 missing critical)
**Impact on plan:** All six were required for the plan's own acceptance criteria
or for claims the code made about itself. The one that widens surface —
`ProbeReason` — is a single additive field the plan's Task 3 criterion cannot be
met without. No scope creep; no new module or npm requirement, and
`go.mod`, `go.sum` and `web/package-lock.json` are untouched.

## Verification Results

| Gate | Result |
|---|---|
| `go test ./... -count=1 -race` | pass |
| `npm --prefix web run test` (82 tests, 6 files) | pass |
| `npm --prefix web run lint` (biome) | pass, zero findings |
| `npm --prefix web run typecheck` (tsc) | pass |
| `go vet ./...`, `gofmt -l` | pass, zero findings |
| `task build` equivalent (`npm run build` + `go build`) | pass, UI embedded |
| Binary starts and the seven routes answer | pass — 401 behind the session gate, 403 on DELETE without the CSRF header; a 404 would have meant unregistered |
| Every contracted route served, and no other | pass — five patterns, seven method/pattern pairs, exactly the contract's table |
| `golangci-lint run` | **not run** — the binary is not installed on this host |

## Issues Encountered

- **`golangci-lint` is not installed on this machine**, so the acceptance
  criterion `golangci-lint run exits 0` could not be executed. `go vet` and
  `gofmt -l` are clean over the whole tree, which is a weaker gate. Recorded in
  `.planning/WINDOWS.md` as an `unrun-verify` entry so it blocks ship rather
  than being forgotten.
- **A `?raw` import of the Go source from vitest is denied** by vite's
  filesystem allowlist (the test root is `web/`). The cross-language drift guard
  was therefore written on the Go side rather than loosening `server.fs.allow`
  in the shared build config for a test-only convenience.

## Known Stubs

None. Every control on the screen is wired to a real route, and no component
receives hardcoded empty data.

## Threat Flags

| Flag | File | Description |
|------|------|-------------|
| threat_flag: information-disclosure | `internal/model/model.go` | `ProbeReason` is a new persisted, operator-visible string sourced from an upstream error. It is composed from the schematic id, the Talos version, the architecture and an HTTP status — no operator input and no credential — but it is a new field flowing from a third party onto an authenticated screen, and a future probe that widened its error text would widen this field with it. |

The register's existing dispositions hold: T-02-34 (kernel args and META in the
audit archive) is mitigated by the two allowlist entries and the fail-closed
default, asserted by `redact_test.go`; T-02-35, T-02-36, T-02-37, T-02-40 and
T-02-41 are each covered by a handler test or the running-binary smoke check.
No npm or Go dependency was added, so T-02-SC did not arise.

## User Setup Required

None — no external service configuration.

## Next Phase Readiness

- Plan 02-07 makes the same kind of `router.go` edit for the dry-run status
  field and relies on the Route Registration Rule's third clause, which is now
  written.
- `docs/api-contract.md` remains single-owner per wave; this plan's edits were
  the Route Registration Rule clause and the `probe_reason` field.
- **Open, for a human:** the Images screen has never been opened in a browser.
  This is coverage item D10 and it extends the standing phase-1 concern that the
  embedded UI is verified only in jsdom.
- **Open:** `golangci-lint` needs to be installed and run over this phase before
  ship; see `.planning/WINDOWS.md`.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-29*

## Self-Check: PASSED

All three created files exist on disk; all five task commits are present in
`git log`. Every task's `<acceptance_criteria>` was re-run and passes, with the
single exception recorded above (`golangci-lint` is not installed on this host).
