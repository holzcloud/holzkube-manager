---
phase: 02-transport-seam-talossim-image-factory
plan: 11
subsystem: api
tags: [go, react, rfc9457, problem-details, image-factory, yaml, validation]

requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "the canonical serialiser and its ErrSchematicNotRepresentable sentinel (02-02), createProblem's UnknownExtensionsError branch (02-04) and the Images screen's authoring form (02-05)"
provides:
  - "imagefactory.NotRepresentableError: a typed refusal carrying the canonical document path, the sequence position and a reason that names the character class and never the value"
  - "POST /api/v1/schematics answers a locally unrenderable value with a 400 validation problem naming kernel_args, meta or extensions, with zero upstream requests"
  - "docs/api-contract.md records the exception to the every-failure-in-this-section-is-a-502 rule"
  - "hasControlCharacter in web/src/routes/images.tsx: the client half of the server's representable rule, refusing rather than stripping"
  - "the Images form disables Create while any row carries a control character, and clears a create error when the form it describes changes"
affects: [image-factory, provisioning, http-error-taxonomy, images-ui]

actuals:
  tokens: 54000
  tasks: 3
  commits: 6

tech-stack:
  added: []
  patterns:
    - "A typed error beside a sentinel, Unwrap()ing to it, matched with errors.As at the HTTP edge — the shape UnknownExtensionsError established in internal/imagefactory"
    - "A document-path-to-request-field table held explicitly at the handler, so the serialiser keeps the document's vocabulary and the handler owns the request's"
    - "A client-side predicate transcribed from a named server function, with the server's refusal as the backstop rather than the fallback"

key-files:
  created: []
  modified:
    - internal/imagefactory/schematicid.go
    - internal/imagefactory/schematicid_test.go
    - internal/httpapi/handlers/schematics.go
    - internal/httpapi/handlers/schematics_test.go
    - docs/api-contract.md
    - web/src/routes/images.tsx
    - web/src/routes/images.test.tsx

key-decisions:
  - "The refusal's Path is the canonical document's vocabulary (customization.extraKernelArgs), not the request's (kernel_args). The serialiser has no knowledge of the HTTP shape and acquiring one would tie a hash-deciding file to a request struct."
  - "A document path the handler's table does not recognise still answers 400, with no field named, rather than falling back to a 502. An unrecognised path is still not the Factory's fault, and telling the operator to retry is the exact harm G-02-6 recorded."
  - "representable() now returns a reason string rather than a wrapped error; the writer builds the typed refusal where the path is known. The refused set is byte-for-byte unchanged — it decides the locally precomputed id, and the id is what FACT-06 rests on."
  - "Index is zero-based in the struct and rendered one-based in every operator-facing string, because an operator counting rows in a form starts at one."
  - "The client refuses and reports; it does not strip. An operator who pasted a value is better served by being told than by having their input silently rewritten (T-02-67)."
  - "The stale-error reset keys on a JSON fingerprint of exactly the fields the request is built from. The mutation's own state is not among them, so the effect cannot be its own cause."

patterns-established:
  - "Refusal locality is asserted with upstream request counters, not inferred: a 400 that claims 'this never left the process' is only credible when the fake records zero catalog fetches and zero POSTs."
  - "The no-echo constraint is asserted at both layers — in the package test on the error string and in the handler test on the response body — because the second is what actually leaves the process."

requirements-completed: [FACT-01, FACT-06]

coverage:
  - id: D1
    description: "A schematic carrying a value the canonical serialiser refuses produces a typed error naming the document path and the sequence position, and errors.Is against ErrSchematicNotRepresentable still holds"
    requirement: "FACT-06"
    verification:
      - kind: unit
        ref: "internal/imagefactory/schematicid_test.go#TestSchematicRefusalNamesTheFieldAndEntry"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/schematicid_test.go#TestSchematicIDRefusesRatherThanGuesses"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/schematicid_test.go#TestSchematicRefusalReportsTheFirstBadValueInDocumentOrder"
        status: pass
    human_judgment: false
  - id: D2
    description: "Neither the refusal message nor the HTTP response body echoes the offending value (T-02-64)"
    verification:
      - kind: unit
        ref: "internal/imagefactory/schematicid_test.go#TestSchematicRefusalDoesNotEchoTheValue"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestCreateRefusalDoesNotEchoTheOffendingValue"
        status: pass
    human_judgment: false
  - id: D3
    description: "POST /api/v1/schematics answers a locally refused kernel argument, META value or extension name with 400 validation.failed naming kernel_args, meta or extensions, having made zero upstream requests"
    requirement: "FACT-01"
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestCreateRefusesALocallyUnrenderableValueAsAnInputProblem"
        status: pass
    human_judgment: false
  - id: D4
    description: "A genuine Image Factory outage still answers 502 with upstream.factory-unavailable — the split is preserved, not replaced"
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestCreateSplitsALocalRefusalFromAFactoryOutage"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestCreateAgainstAnOutageIsUpstreamAndNotInternal"
        status: pass
    human_judgment: false
  - id: D5
    description: "The Images form raises a row-level alert for a control character, disables Create while one is present, posts nothing, and re-enables Create once it is removed"
    verification:
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#refuses a kernel argument carrying a control character before any request"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#re-enables Create once the control character is removed"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#refuses a META value carrying a control character before any request"
        status: pass
    human_judgment: false
  - id: D6
    description: "A create error on screen disappears when the operator changes a field the request is built from"
    verification:
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#clears a create error as soon as the form it belongs to changes"
        status: pass
    human_judgment: false
  - id: D7
    description: "docs/api-contract.md states the 400-naming-the-field rule for a locally refused schematic while still stating the upstream 502 rule for the Factory's own failures"
    verification:
      - kind: other
        ref: "docs/api-contract.md § Upstream failures — reviewed by reading; no executed test asserts contract prose"
        status: unknown
    human_judgment: true
    rationale: "Whether the new paragraph reads as an exception to the paragraph above it, rather than as a contradiction of it, is a judgment about prose that no test makes. The rule it states is separately pinned by D3 and D4."
  - id: D8
    description: "The Images screen renders these refusals legibly in a real browser"
    verification: []
    human_judgment: true
    rationale: "Only jsdom has seen this screen. The standing Phase-1 concern about the embedded UI applies unchanged: the row-level alert's placement, contrast and reading order next to its input are visual facts jsdom cannot report."

duration: 11 min
completed: 2026-08-29
status: complete
---

# Phase 02 Plan 11: A Locally Refused Input Is an Input Problem Summary

**A control character in a kernel argument now answers 400 naming `kernel_args` with zero upstream requests, instead of the 18ms 502 that blamed factory.talos.dev — and the form refuses the character before Create is enabled.**

## Performance

- **Duration:** 11 min
- **Started:** 2026-08-29T16:59:48Z
- **Completed:** 2026-08-29T17:11:04Z
- **Tasks:** 3 (all TDD: RED then GREEN, no REFACTOR needed)
- **Files modified:** 7

## Accomplishments

- `imagefactory.NotRepresentableError` names the canonical document path and the sequence position of the value the serialiser refused, unwraps to `ErrSchematicNotRepresentable` so every existing `errors.Is` keeps working, and carries a reason that names the character class and never the value.
- `createProblem` maps that refusal onto a `validation` problem at 400 with a single field error, using an explicit path-to-request-field table. An unrecognised path is still a 400, not a 502.
- The tests assert the claim rather than the symptom: the fake Factory records **zero** catalog fetches and **zero** schematic POSTs on the refusal path, which is what makes "this is your input, not their outage" a fact.
- `docs/api-contract.md` records the exception the "every route in this section reaches factory.talos.dev, and a failure there is a 502" paragraph previously swallowed.
- The Images form refuses the character class the server refuses, per row, with `role="alert"` next to the input, and disables Create while any is present. It reports rather than strips.
- A create error is reset when any field the request is built from changes — the second half of G-02-6's artifacts note, where the error line outlived the form through unrelated interactions.

## Task Commits

Each task was committed atomically, RED then GREEN:

1. **Task 1: The serialiser names what it refused and where** — `3600f32` (test), `226bbeb` (feat)
2. **Task 2: A locally refused input is a 400 naming the field** — `953834e` (test), `b3775f9` (feat)
3. **Task 3: The form refuses the character before Create** — `ecd3d78` (test), `445ff87` (feat)

Every RED commit was verified to fail before its GREEN was written. Task 2's RED reproduced the UAT symptom exactly: `502 upstream.factory-unavailable`, `"The Image Factory did not answer usably: creating the schematic."`

## Files Created/Modified

- `internal/imagefactory/schematicid.go` — `NotRepresentableError`; the path threaded to each `Canonical` call site; `representable` returns a reason rather than a wrapped error; `renderScalar` no longer decides representability.
- `internal/imagefactory/schematicid_test.go` — `errors.As` assertions for all six document paths, first-failure-wins pinned, no-echo asserted.
- `internal/httpapi/handlers/schematics.go` — the `NotRepresentableError` branch in `createProblem` ahead of the `factoryProblem` fallback, `requestFieldForPath`, `refusalReason`.
- `internal/httpapi/handlers/schematics_test.go` — the three refusable fields, the zero-upstream-request assertion, the no-echo assertion on the response body, and the 502 half of the split.
- `docs/api-contract.md` — two paragraphs under § Upstream failures stating the 400 rule and the no-echo constraint.
- `web/src/routes/images.tsx` — `hasControlCharacter`, `CONTROL_CHARACTER_MESSAGE`, `RowProblem`, per-row error props on `RepeatableRows` and `MetaRows`, the disabled term, and the fingerprint-driven mutation reset.
- `web/src/routes/images.test.tsx` — five cases, using `fireEvent.change` because `userEvent.type` models keystrokes and there is no key for U+0007.

## Decisions Made

- **The `Path` is the document's vocabulary, not the request's.** `internal/imagefactory` has no knowledge of the HTTP request shape, and giving it one would tie a file whose bytes decide a hash to a handler's struct tags. The translation table lives at the handler, where the request shape is already known.
- **An unrecognised path falls back to a 400 with no field name, never to a 502.** The plan required this and it is the load-bearing half of the fix: a path the table has not caught up with is still a purely local refusal, and a 502 would invite a retry that can never succeed.
- **`representable()` returns a reason string.** The typed refusal is built by the writer, which is the only place that knows the path. The refused set — invalid UTF-8, any rune below U+0020, U+007F — is byte-for-byte what it was; only the reporting changed. The prohibition on widening or narrowing it is honoured, and `TestSchematicIDMatchesRecordedFixtures` (the live-Factory id corpus) still passes.
- **`Index` is zero-based in the struct, one-based in prose.** A slice index is what a caller wants; "entry 2" is what an operator counting rows sees.
- **The client reports rather than strips (T-02-67).** Silently rewriting a pasted value into something the operator did not type is a worse failure than refusing it.
- **The stale-error reset keys on a JSON fingerprint of the request fields.** A dependency array listing the fields without reading them is flagged by biome's exhaustive-dependencies rule as extra dependencies; the fingerprint is read in the effect body, so the dependency is real rather than a note to the linter. A ref guard skips the reset on mount.

## Deviations from Plan

**1. [Rule 3 - Blocking] The stale-error reset was restructured to satisfy the linter**

- **Found during:** Task 3
- **Issue:** The plan's shape — a `useEffect` depending on name, version, arch, extensions, kernel args, META and SecureBoot, whose body only calls `create.reset()` — is rejected by biome's `useExhaustiveDependencies`: none of the seven are read in the body, so all seven are reported as extra dependencies and `npm run lint` exits non-zero. That is an acceptance criterion of this task.
- **Fix:** The seven fields are serialised into a `requestFingerprint`, which the effect reads and depends on. A `useRef` holds the last fingerprint so the reset does not fire on mount, and a second ref holds `create.reset` so the mutation object's changing identity cannot enter the dependency surface.
- **Files modified:** `web/src/routes/images.tsx`
- **Verification:** `npm --prefix web run lint` exits 0; the behavioural test (`clears a create error as soon as the form it belongs to changes`) passes, and failed before the implementation.
- **Committed in:** `445ff87`

---

**Total deviations:** 1 auto-fixed (1 blocking).
**Impact on plan:** None on behaviour. The plan's constraint — "keep the reset out of a path that could re-trigger itself: the mutation's own state is not among those fields" — is preserved and, if anything, made structural rather than conventional.

## Issues Encountered

- **`golangci-lint` is not installed on this host**, so the plan's `task lint` verification is only partly executed: `gofmt -l ./internal` and `go vet ./...` are clean, `npm --prefix web run lint` and `npm --prefix web run typecheck` are clean, and `golangci-lint run` was not run. This is the same standing gap plan 02-06 recorded. Logged to `.planning/WINDOWS.md` as an `unrun-verify` entry (id 19).
- `.planning/WINDOWS.md` is appendable again after the `kind` repair in `e1a5c6c`; `windows status` and `windows append` both worked.

## Prohibitions

Both of the plan's binding prohibitions hold, and both are asserted rather than claimed:

- **The refusal never echoes the offending value.** Asserted in `TestSchematicRefusalDoesNotEchoTheValue` on the error string and in `TestCreateRefusalDoesNotEchoTheOffendingValue` on the HTTP response body — the second is the one that actually leaves the process.
- **The canonical serialiser's refusal set is unchanged.** `representable` refuses exactly what it refused before; `TestSchematicIDMatchesRecordedFixtures`, which pins the local computation against ids the live Factory assigned, still passes, as does every other test in `internal/imagefactory`.

## Threat Flags

None. This plan adds no network endpoint, no auth path, no file access and no schema change. The two upstream request counters that were reachable on this path are now provably not reached. `T-02-SC` holds: no Go module and no npm package was added (`go.mod`, `go.sum` and `web/package-lock.json` are untouched).

## User Setup Required

None — no external service configuration required.

## Verification

| Check | Result |
|---|---|
| `go test ./... -count=1 -race` | exit 0 |
| `gofmt -l ./internal` | prints nothing |
| `go vet ./...` | exit 0 |
| `npm --prefix web run test` | 100 passed / 7 files |
| `npm --prefix web run lint` | exit 0 |
| `npm --prefix web run typecheck` | exit 0 |
| `golangci-lint run` | **not run** — not installed on this host (see Issues) |
| `docs/api-contract.md` states both rules | yes — the 400 exception and the upstream 502 rule stand side by side |

## Next Phase Readiness

- G-02-6 is closed on both sides. Nothing in this plan touches the probe, its budget, its state or the create route's shape, so `02-DECISION-probe-budget.md` remains open and undisturbed: the new 400 is raised by `Schematic.ID()` before the catalog fetch, before the POST and long before any probe, which is strictly earlier than the point at which the two options diverge.
- The Images screen has still never been opened in a browser (D8). The row-level alert is a new visual element on a screen that only jsdom has seen.
- If a future phase adds a scalar to the canonical document, it must supply a path at the `Canonical` call site, and — if the field is operator-authored — add a row to `requestFieldForPath`. Omitting the second yields a 400 with no field name, which is degraded but not wrong.

## Self-Check: PASSED

All seven modified files exist on disk. All six task commits are present in `git log --oneline --all`. The plan-level `<verification>` block was re-run in full; every automated check passes, with the single documented exception of `golangci-lint`, which is not installed on this host.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-29*
