---
phase: 02-transport-seam-talossim-image-factory
plan: 04
subsystem: infra
tags: [image-factory, talos, oci-registry, semver, store, schema-migration, api-contract]

requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "plan 02-02's imagefactory client -- its *http.Client, timeout, body cap, strict decoder, path-segment patterns and offline fake -- plus the upstream RFC 9457 family this plan's contract section codes against"
  - phase: 01-foundation
    provides: "the store entity seam with its CAS Put and atomic writer, the migrate package's backup-then-apply machinery, and internal/audit's curated-table-with-a-reason pattern"
provides:
  - "imagefactory URL derivation for ISO, PXE, disk image and cmdline with architecture and platform as parameters (FACT-03)"
  - "Client.InstallerImage: the installer OCI repository name proven against the registry manifest per Talos version, never assumed (FACT-03, D-02)"
  - "Structural prerelease filtering and a semver NewestStable that is not the last element (FACT-05)"
  - "The curated broken-version table and its BrokenReason lookup (FACT-05, D-08)"
  - "imagefactory.Warnings: the installer/initramfs asymmetry stated at authoring time (FACT-04)"
  - "model.Schematic behind store.Schematics(), schema version 2 with the first real migration (D-09, FACT-06 persistence half)"
  - "The docs/api-contract.md Schematics section: seven routes, two audit action tokens, the allowlist rule"
affects: [02-06, 02-07, provisioning wizard, node upgrade, 03-provisioner]

actuals:
  tokens: 26700
  tasks: 3
  commits: 6

tech-stack:
  added: []
  patterns:
    - "A guard the value is checked against rather than escaped: the same refusal helper serves all four URL derivations and the installer resolver"
    - "Resolve-then-cache keyed on what the answer actually depends on, with failures deliberately not cached"
    - "A curated table whose per-entry reason is its whole value, guarded by a test that refuses an entry without one"
    - "Contract-first documentation: the route table is written and reviewed in the plan that builds the machinery, implemented in the next"

key-files:
  created:
    - internal/imagefactory/urls.go
    - internal/imagefactory/urls_test.go
    - internal/imagefactory/installer.go
    - internal/imagefactory/installer_test.go
    - internal/imagefactory/versions.go
    - internal/imagefactory/versions_test.go
    - internal/imagefactory/brokenversions.go
    - internal/imagefactory/brokenversions_internal_test.go
    - internal/imagefactory/warnings.go
    - internal/imagefactory/warnings_test.go
    - internal/store/testdata/version-1/VERSION
    - internal/store/testdata/version-1/users/alice.json
  modified:
    - internal/imagefactory/client.go
    - internal/imagefactory/probe.go
    - internal/imagefactory/fake_test.go
    - internal/model/model.go
    - internal/store/store.go
    - internal/store/fsstore/fsstore.go
    - internal/store/fsstore/fsstore_test.go
    - internal/store/migrate/migrate.go
    - internal/store/migrate/migrate_test.go
    - internal/store/testdata/README.md
    - internal/store/testdata/version-current/VERSION
    - internal/store/testdata/version-future/VERSION
    - docs/api-contract.md

key-decisions:
  - "The curated broken-version table ships EMPTY: research's only candidate (v1.9.0) was re-probed live and metal-installer now answers 200, so listing it would have put an unchecked claim in the table whose value is that every claim is checkable"
  - "The semver comparison is implemented locally rather than importing golang.org/x/mod/semver -- only stable versions are compared, so it is three integers, and this plan adds no module requirement"
  - "The installer resolution cache is keyed on platform AND Talos version, not version alone, because the modern repository name is the platform-prefixed one"
  - "A registry that did not answer is kept apart from a registry that refused: only an all-404 outcome is ErrSchematicNotBuildable, everything else stays retryable"
  - "Arch moved from probe.go to urls.go and ProbeBuildable now builds its URL through ISOURL, so the probe cannot address a different asset than the operator is handed"
  - "A pre-VERSION data directory is one version behind as of phase 2, so it is migrated and backed up -- the phase-1 test asserting the opposite was updated rather than the reading of the fixture changed"

patterns-established:
  - "Refusal register: every URL and installer refusal names the value, what shape was expected, and why, in tlsx.LoopbackGuard's voice"
  - "An empty curated table with a doc comment recording the re-probe that emptied it, rather than a plausible-looking entry"
  - "Ordering proven by content, not sequence: the pre-migration tarball is asserted NOT to contain the directory the migration creates"

requirements-completed: [FACT-03, FACT-04, FACT-05, FACT-06]

coverage:
  - id: D1
    description: "ISO, PXE, disk-image and cmdline URLs are derived from (schematic id, version, architecture, platform); no architecture is a compile-time constant inside a URL"
    requirement: FACT-03
    verification:
      - kind: unit
        ref: "internal/imagefactory/urls_test.go#TestAssetURLs"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/urls_test.go#TestAssetURLsDifferOnlyInTheArchitecture"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/urls_test.go#TestAssetURLsRefuseRatherThanGuess"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/urls_test.go#TestAssetURLsRefuseInEveryDerivation"
        status: pass
    human_judgment: false
  - id: D2
    description: "The installer OCI repository name is resolved per Talos version against the registry manifest and never assumed; a version where only the legacy name answers yields the legacy name, and a version where neither answers yields no reference at all"
    requirement: FACT-03
    verification:
      - kind: integration
        ref: "internal/imagefactory/installer_test.go#TestInstallerImageResolvesThePlatformPrefixedName"
        status: pass
      - kind: integration
        ref: "internal/imagefactory/installer_test.go#TestInstallerImageFallsBackToTheLegacyName"
        status: pass
      - kind: integration
        ref: "internal/imagefactory/installer_test.go#TestInstallerImageRefusesWhenNeitherAnswers"
        status: pass
      - kind: integration
        ref: "internal/imagefactory/installer_test.go#TestInstallerImageCachesPerTalosVersion"
        status: pass
      - kind: integration
        ref: "internal/imagefactory/installer_test.go#TestInstallerImageSeparatesAnUnreachableRegistryFromARefusal"
        status: pass
    human_judgment: false
  - id: D3
    description: "GET /versions output is split on the semver prerelease component structurally, so a prerelease is never the default selection and needs no curated data"
    requirement: FACT-05
    verification:
      - kind: unit
        ref: "internal/imagefactory/versions_test.go#TestVersionsSplitSeparatesTheWholePrereleaseTail"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/versions_test.go#TestVersionsNewestStableIsNotTheLastElement"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/versions_test.go#TestVersionsNewestStableComparesRatherThanSorts"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/versions_test.go#TestVersionsSplitNeverPromotesAnUnparsableEntry"
        status: pass
    human_judgment: false
  - id: D4
    description: "A version listed in the curated broken table carries the reason it is listed, in the file, reviewable in git"
    requirement: FACT-05
    verification:
      - kind: unit
        ref: "internal/imagefactory/brokenversions_internal_test.go#TestBrokenReasonFindsAListedVersion"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/brokenversions_internal_test.go#TestBrokenReasonEveryEntryCarriesAReason"
        status: pass
    human_judgment: true
    rationale: "The mechanism is proven, but the table ships with zero entries because the single research candidate (v1.9.0) was falsified by a live re-probe during execution. Whether an empty table is an acceptable state for FACT-05, or whether the requirement wants a populated one, is a judgment the operator should make -- see Decisions Made."
  - id: D5
    description: "A schematic with a non-empty ExtraKernelArgs or Meta yields a warning naming installer and initramfs; a schematic with neither yields none"
    requirement: FACT-04
    verification:
      - kind: unit
        ref: "internal/imagefactory/warnings_test.go#TestWarningsForKernelArgs"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/warnings_test.go#TestWarningsForMeta"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/warnings_test.go#TestWarningsForBoth"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/warnings_test.go#TestWarningsForNeitherIsEmptyAndNotNil"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/warnings_test.go#TestWarningsNameInstallerAndInitramfs"
        status: pass
    human_judgment: false
  - id: D6
    description: "A schematic record persists through store.Schematics() and survives a process restart with its Rev, its Factory-canonical document and its usable verdict intact"
    requirement: FACT-06
    verification:
      - kind: integration
        ref: "internal/store/fsstore/fsstore_test.go#TestSchematicSurvivesARestart"
        status: pass
      - kind: integration
        ref: "internal/store/fsstore/fsstore_test.go#TestSchematicPutRefusesAStaleRevision"
        status: pass
      - kind: integration
        ref: "internal/store/fsstore/fsstore_test.go#TestSchematicRoundTripsByteIdentically"
        status: pass
      - kind: integration
        ref: "internal/store/fsstore/fsstore_test.go#TestSchematicGetDistinguishesMissingFromUnaddressable"
        status: pass
    human_judgment: false
  - id: D7
    description: "A version-1 data directory is migrated to version 2 with a backup written first, and a version-2 directory is left untouched"
    verification:
      - kind: integration
        ref: "internal/store/migrate/migrate_test.go#TestMigrateVersionOneGainsTheSchematicsDirectory"
        status: pass
      - kind: integration
        ref: "internal/store/migrate/migrate_test.go#TestMigrateCurrentVersionIsANoOp"
        status: pass
      - kind: integration
        ref: "internal/store/migrate/migrate_test.go#TestMigrateLegacyDirectoryWithoutVersionIsTreatedAsVersionOne"
        status: pass
      - kind: manual_procedural
        ref: "fsstore.Open on a copied real version-1 data directory: VERSION 2, backups/pre-migration-1-to-2-2026-08-29T05:26:18Z.tar.gz, schematics/ created, users/alice.json intact"
        status: pass
    human_judgment: false
  - id: D8
    description: "The inbound schematics API is contracted before it is implemented: seven routes, the resource shape, the warning codes, the upstream codes and both audit action tokens"
    verification: []
    human_judgment: true
    rationale: "docs/api-contract.md is a one-way contract that plan 02-06 codes against, and no test asserts that a documented contract is the right contract. A human should read the Schematics section before wave 3 starts, in particular the DELETE-is-destructive call and the schematic.create allowlist (name and talos_version only)."

duration: 32 min
completed: 2026-08-29
status: complete
---

# Phase 02 Plan 04: Asset URLs, Version Honesty & Schematic Persistence Summary

**Every Image Factory artifact an operator can actually use — ISO, PXE, disk image, resolved cmdline and a registry-proven installer reference — derived from parameters rather than constants, behind an honest version list that never defaults to a release candidate, with the kernel-args warning that stops an ISO and the system it installs from diverging, all persisted through `store.Schematics()` at schema version 2.**

## Performance

- **Duration:** 32 min
- **Started:** 2026-08-29T04:55:00Z
- **Completed:** 2026-08-29T05:27:00Z
- **Tasks:** 3 (each RED → GREEN)
- **Files modified:** 25 (12 created, 13 modified)

## Accomplishments

- **The installer repository name is proven, never assumed.** `Client.InstallerImage` issues an OCI manifest request against the platform-prefixed name and falls back to the legacy one, returning an error and *no reference at all* when neither answers. This is the highest-severity item in the plan: the reference is consumed by the upgrade RPC, and a wrong one produces an upgrade that reports success while silently dropping every system extension a node was built with (`PITFALLS.md` §P9c). The resolution is cached per platform and version and a failure is deliberately never cached.
- **The architecture is a parameter end to end.** `Arch` and `Platform` are closed named types, `AssetRequest` carries them, and all four derivation functions plus `ProbeBuildable` go through one validating helper. `TestAssetURLsDifferOnlyInTheArchitecture` asserts that the amd64 and arm64 ISO URLs differ in nothing but the architecture segment — the direct form of FACT-03's "ohne hartkodierte Architektur".
- **Prerelease filtering needs no data at all.** `IsPrerelease` reads the semver prerelease component; `NewestStable` compares numerically instead of taking the last element. Against the recorded 108-entry upstream list — which ends in `v1.14.0-rc.2` — it returns `v1.13.9`. `v1.13.10 > v1.13.9` is asserted explicitly, because that pair is where a string sort silently gives the wrong answer.
- **The FACT-04 warning says the thing upstream says.** Both warning details name `installer` *and* `initramfs`, explain that the ISO carries the kernel arguments and the installed system does not, and point at `.machine.install.extraKernelArgs`. `Warnings` returns `[]` and never `nil`, so "checked, nothing to report" is distinguishable from "did not check".
- **Schematics live behind the entity seam at schema version 2.** `model.Schematic` follows `model.User`'s shape, `fsstore.schematicStore` copies `userStore`'s CAS `Put` unchanged in structure, and the first real entry in the forward-only migration table creates the `schematics/` directory. The 1 → 2 path was verified against a real data directory, not only a fixture.
- **The inbound API is contracted before it is written.** `docs/api-contract.md` gained a `## Schematics` section covering all seven routes plan 02-06 implements, the resource shape, both warning codes, both upstream codes, `DELETE` as destructive, and — closing this plan's first prohibition at the contract level — the `schematic.create` audit allowlist permitting `name` and `talos_version` and nothing else.
- **No new module.** `go.mod` and `go.sum` are byte-identical to their pre-plan state. The semver comparison is 30 lines of local code rather than a dependency.

## Task Commits

1. **Task 1 RED: failing tests for asset URLs and installer resolution** — `7880bf6` (test)
2. **Task 1 GREEN: derive every asset URL, resolve the installer repo per version** — `38f7e05` (feat)
3. **Task 2 RED: failing tests for version filtering, broken versions and warnings** — `bd034e1` (test)
4. **Task 2 GREEN: structural prerelease filtering and the installer asymmetry warning** — `56045d1` (feat)
5. **Task 3 RED: failing tests for the schematic entity and the 1 → 2 migration** — `09afb6b` (test)
6. **Task 3 GREEN: persist schematics behind the store seam at schema version 2** — `1c877a4` (feat)

No REFACTOR commit on any of the three cycles: each GREEN implementation was already the minimal shape (a validating helper plus four one-line derivations; a lookup plus a table; a struct copied from `userStore` with the types swapped).

## Files Created/Modified

- `internal/imagefactory/urls.go` — `Arch`, `Platform`, `AssetRequest`, the four derivation functions, the shared validating helper and the refusal register
- `internal/imagefactory/installer.go` — `InstallerImage`, the manifest probe against both repository names, the per-platform-and-version cache, and the refused/unreachable split
- `internal/imagefactory/versions.go` — `IsPrerelease`, `SplitVersions`, `NewestStable` and the local semver parse and comparison
- `internal/imagefactory/brokenversions.go` — the curated table (empty, with the reason it is empty), `BrokenReason` and the testable `brokenReason`
- `internal/imagefactory/warnings.go` — `Warning`, the two exported code constants and `Warnings`
- `internal/imagefactory/probe.go` — `Arch` removed (moved to `urls.go`); the ISO URL now comes from `ISOURL`
- `internal/imagefactory/client.go` — the installer-resolution cache fields on `Client`
- `internal/imagefactory/fake_test.go` — `/v2/<repo>/<id>/manifests/<version>` routes with a per-version repository availability map, plus `setManifestStatus`
- `internal/model/model.go` — `SchematicID`, `Schematic`, `MetaValue`
- `internal/store/store.go` — `Schematics()` on the root and the `SchematicStore` interface
- `internal/store/fsstore/fsstore.go` — `kindSchematics`, `schematicStore`, the `Open` wiring and the directory creation
- `internal/store/migrate/migrate.go` — `CurrentVersion` 2, `dirPerm`, and the first entry in the forward-only table
- `internal/store/testdata/` — new `version-1/` fixture; `version-current/` bumped to 2, `version-future/` to 3; README table updated
- `docs/api-contract.md` — the `## Schematics` section

## Decisions Made

- **The curated broken-version table ships empty, and that is a finding.** Research recorded exactly one defensible candidate: `v1.9.0`, where the platform-prefixed installer name was observed not to resolve (`PITFALLS.md` §P9d). That observation was re-probed twice against `factory.talos.dev` during execution and **`GET /v2/metal-installer/<id>/manifests/v1.9.0` now answers 200**. Listing it anyway would have put an unchecked claim into the one table whose entire value is that each claim carries checkable evidence, and the plan explicitly forbids inventing entries to make the feature look populated. The table's doc comment records the re-probe, states what would justify a real entry, and the lookup is proven against a synthetic table so the first real entry needs nothing but the entry.
- **The semver comparison is local, not `golang.org/x/mod/semver`.** That module is reachable in the graph but is not a requirement of this one, and the plan's artifact list says this plan adds no module requirement. `NewestStable` filters prereleases out before comparing, so what remains is three integers — no prerelease precedence rules at all. Implementing them approximately would have been worse than not implementing them.
- **The installer cache key is `platform/version`, not `version`.** The plan says "keyed on version because that is what the name depends on". The name also depends on the platform: the modern candidate is `<platform>-installer`. With one platform in this milestone the difference is invisible, which is exactly why it would have been a latent bug rather than a caught one.
- **A registry that did not answer is not a schematic that cannot be built.** Only an outcome where *every* candidate repository answered 404 yields `ErrSchematicNotBuildable`; a transport failure, a 5xx or an authentication challenge yields `ErrUpstreamUnavailable` and stays retryable. This mirrors `ProbeBuildable`'s existing split and keeps an outage from sending an operator to rebuild something that is not broken.
- **`ProbeBuildable` now derives its URL through `ISOURL`.** It previously built `metal-%s.iso` with its own format string. Two URL builders for the same asset drift, and the drift would mean the probe proving one thing while the operator is handed another.
- **A pre-VERSION data directory is migrated as of this phase.** Phase 1's `TestMigrateLegacyDirectoryWithoutVersionIsTreatedAsCurrent` asserted no migration and no backup, which was true only while `CurrentVersion` was 1. The fixture's *reading* is unchanged — such a directory is version 1 — but it is now one version behind, so the test was renamed and its expectations inverted rather than the reading fudged.
- **The `schematic.create` audit allowlist is contracted here, not left to plan 02-06.** It permits `name` and `talos_version`. `kernel_args`, `meta`, `extensions` and `canonical` are redacted, because the Factory itself refuses to enumerate schematics on the grounds that kernel arguments may carry secrets, and holzkube's archive is append-only and never deleted (D-16).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `Arch` already existed in `probe.go`**

- **Found during:** Task 1
- **Issue:** The plan instructs `urls.go` to declare `type Arch string` with `ArchAMD64` and `ArchARM64`. Plan 02-02 already declared all three in `probe.go`; a second declaration does not compile.
- **Fix:** Moved the declaration to `urls.go`, where the plan places it, and removed it from `probe.go`. While there, `ProbeBuildable` was rewired to build its ISO URL through `ISOURL` rather than its own `fmt.Sprintf("metal-%s.iso", arch)`, which also gives the probe the same request validation.
- **Files modified:** `internal/imagefactory/urls.go`, `internal/imagefactory/probe.go`
- **Verification:** the whole pre-existing `imagefactory` suite, including `TestTracerSeparatesAnUnreachableProbeFromABadSchematic`, still passes.
- **Committed in:** `38f7e05`

**2. [Rule 3 - Blocking] The internal/talos seam guard flagged the OCI reference format string**

- **Found during:** Task 1
- **Issue:** `TestNoAddressAboveTheSeam` matches `%s:%s` as an endpoint being built outside `internal/talos`. `fmt.Sprintf("%s/%s/%s:%s", …)` for an OCI reference matched it. The guard's own doc comment says it is deliberately narrow so that people do not switch it off, so widening the regex was the wrong repair.
- **Fix:** The reference is assembled by concatenation instead, with a comment saying why: it is an image reference, not a network endpoint, and the guard should keep looking at this shape rather than learn to ignore it.
- **Files modified:** `internal/imagefactory/installer.go`
- **Verification:** `go test ./internal/talos/ -count=1` green.
- **Committed in:** `38f7e05`

**3. [Rule 2 - Missing Critical] The installer cache key includes the platform**

- **Found during:** Task 1
- **Issue:** The plan specifies a cache keyed on Talos version. The modern repository name is `<platform>-installer`, so two platforms at one version are two different answers sharing one cache slot.
- **Fix:** The key is `platform/version`, documented.
- **Files modified:** `internal/imagefactory/installer.go`
- **Verification:** `TestInstallerImageCachesPerTalosVersion` still asserts one request per version at one platform.
- **Committed in:** `38f7e05`

**4. [Rule 2 - Missing Critical] A failed installer resolution is never cached**

- **Found during:** Task 1
- **Issue:** The plan asks for a cache and does not say what to do with a failure. Caching one would remember a transient registry outage as "no installer exists for this version" and block upgrades long after the registry recovered.
- **Fix:** Only a successful resolution is stored. `TestInstallerImageDoesNotCacheAFailure` pins it.
- **Files modified:** `internal/imagefactory/installer.go`
- **Verification:** the registry is forced to 503, the call fails, the force is lifted, the next call resolves.
- **Committed in:** `38f7e05`, `7880bf6`

**5. [Rule 2 - Missing Critical] Refused and unanswered are separate verdicts in the resolver**

- **Found during:** Task 1
- **Issue:** The plan says only "if neither answers, return an error". A single error kind would report a registry outage as a schematic that cannot be built.
- **Fix:** All-404 is `ErrSchematicNotBuildable`; anything else is `ErrUpstreamUnavailable`. Pinned by `TestInstallerImageSeparatesAnUnreachableRegistryFromARefusal`.
- **Files modified:** `internal/imagefactory/installer.go`
- **Verification:** both branches asserted.
- **Committed in:** `38f7e05`

**6. [Rule 1 - Correctness] `SplitVersions` never promotes an entry it cannot parse**

- **Found during:** Task 2
- **Issue:** The plan's two-bucket signature has no place for an unreadable entry, and the obvious reading — "not a prerelease, therefore stable" — offers an operator a version nothing checked.
- **Fix:** `IsPrerelease` returns true for an unparsable version, documented as the fail-safe direction. `TestVersionsSplitNeverPromotesAnUnparsableEntry` pins it, and no entry is dropped.
- **Files modified:** `internal/imagefactory/versions.go`
- **Verification:** `"latest"` and `""` land outside the stable bucket and the count is preserved.
- **Committed in:** `56045d1`

**7. [Rule 3 - Blocking] Three phase-1 migration fixtures and one test had to move with `CurrentVersion`**

- **Found during:** Task 3
- **Issue:** `version-current/` held `1` and `version-future/` held `2`; both encode a relationship to `CurrentVersion` rather than an absolute version. Bumping to 2 broke four existing tests.
- **Fix:** `version-current/` → `2`, `version-future/` → `3`, a new `version-1/` fixture, the README table updated, and `TestMigrateLegacyDirectoryWithoutVersionIsTreatedAsCurrent` renamed to `…IsTreatedAsVersionOne` with its expectations inverted (it is now migrated and backed up). The `run` doc comment, which claimed tests drive "a 1 → 2 path phase 3 will add", was corrected — that path is now real.
- **Files modified:** `internal/store/testdata/*`, `internal/store/migrate/migrate_test.go`, `internal/store/migrate/migrate.go`
- **Verification:** `go test ./internal/store/... -count=1 -race` green, including the permission, crash-injection and concurrency suites.
- **Committed in:** `1c877a4`

**8. [Rule 2 - Missing Critical] `brokenversions_internal_test.go`, a file the plan does not name**

- **Found during:** Task 2
- **Issue:** With the table empty, `BrokenReason`'s "listed → returns the reason" branch is unreachable from an external test, so the mechanism would ship unproven. The behaviour block requires it to be tested.
- **Fix:** An internal test file exercising `brokenReason` against a synthetic table, plus the guard that every real entry is a valid Talos version with a reason long enough to review.
- **Files modified:** `internal/imagefactory/brokenversions.go` (split into `BrokenReason` and `brokenReason`), `internal/imagefactory/brokenversions_internal_test.go`
- **Verification:** both tests pass; the guard is what makes the first real entry safe to add.
- **Committed in:** `bd034e1`, `56045d1`

---

**Total deviations:** 8 auto-fixed (4 missing critical, 3 blocking, 1 correctness)
**Impact on plan:** All eight sit inside the plan's stated intent. Three are compile-or-test blockers created by the plan building on phase-1 and 02-02 artifacts. Four close gaps between what the plan requires to be *true* and what its file list would have made *checkable*. One is a correctness choice the plan's signature left open. No scope creep: no new module, no new route, no new CLI flag.

## Flagged Assumptions Resolved

- **Assumption 1 (FACT-03/04/05 probe classification miss):** confirmed as a language miss, exactly as in plan 02-02. All three have real, testable edges; each is covered by a coverage item above.
- **Assumption 2 (D-08 vs `STACK.md` on broken versions):** **not resolved, and now weaker in both directions.** `STACK.md` records `BrokenVersions(ctx) ([]string, error)` on the official client, implying an upstream endpoint. This plan followed D-08 as instructed and did not go looking for that endpoint (the live probing budget was spent on the installer manifests, and `factory.talos.dev` began rate-limiting mid-session). Note that even if the endpoint exists it returns *versions without reasons*, which is the thing D-08 argues the table is for. **Input for a later D-08 review:** an upstream list could seed the table, but each entry would still need a reason written by hand.
- **Assumption 3 (the table may end up with one entry or none):** resolved as **none**. See Decisions Made.
- **Assumption 4 (the schema bump is one-way in practice):** confirmed unchanged. A version-2 directory cannot be opened by a pre-phase-2 build; `TestMigrateRefusesAVersionNewerThanTheBinary` still holds with the bumped fixture.
- **Assumption 5 (this plan owns `docs/api-contract.md` in wave 2):** held. The append is a new trailing section; nothing existing was edited, so plan 02-06's amendment of the Route Registration Rule and plan 02-07's dry-run append do not overlap it.

## Prohibitions

Both entered `unresolved`:

- *"A schematic's kernel arguments and META values must never be written in clear into the audit archive or a log line."* **Contracted, not yet enforced.** No code in this plan logs or audits anything — the audit allowlist entry belongs to plan 02-06. What this plan did is remove the room to get it wrong: `docs/api-contract.md` now states the allowlist for `schematic.create` verbatim (`name` and `talos_version`, nothing else), and `internal/audit`'s default is redact-everything, so the entry can only fail by someone adding to it on purpose. Carried forward as a concern below.
- *"The installer repository name must never be a compile-time constant chosen once and reused across the supported version range."* **Held by code and by tests.** There is no code path in `InstallerImage` that returns a reference without a 2xx manifest answer for that exact name, and `TestInstallerImageFallsBackToTheLegacyName` plus `TestInstallerImageRefusesWhenNeitherAnswers` pin both non-obvious branches.

## Threat Flags

| Flag | File | Description |
|------|------|-------------|
| threat_flag: information disclosure | `internal/store/fsstore/fsstore.go`, `internal/model/model.go` | `model.Schematic.KernelArgs` and `.Meta` are persisted in clear, 0600 inside the 0700 data directory — the register's T-02-20 mitigation. Flagged rather than merely mitigated because the pre-migration backup tarball now also contains them, and a tarball is a single file an operator may copy off the machine to somewhere with weaker permissions. |
| threat_flag: tampering | `internal/imagefactory/installer.go` | The OCI reference this function returns is consumed by a later phase's upgrade RPC. Nothing yet enforces that a caller uses the *resolved* reference rather than reconstructing one; the safety is currently that the only constructor is here. Plan 02-06 and the upgrade phase should keep it that way. |

## Known Stubs

None. Every function shipped is wired to a real code path and exercised by a test.

Three deliberate, documented scope limits (gaps, not stubs):

- `brokenVersions` has no entries. See Decisions Made — the mechanism ships proven, the data set is empty because nothing is currently known broken.
- `PlatformMetal` is the only platform. `Platform` is a closed type precisely so adding one is a compile-checked change rather than a string appearing somewhere new.
- The seven routes in `docs/api-contract.md` are documented and not served. That is the plan's intent (plan 02-06 implements them) and the section says so in its first paragraph.

## Issues Encountered

- **`factory.talos.dev` rate-limited or throttled mid-session.** Several probes returned curl exit code 000 (no HTTP response) after roughly a dozen requests, including endpoints that had answered seconds earlier. This is worth knowing for two reasons: it is almost certainly what `PITFALLS.md` recorded as "`metal-installer` at `v1.9.0` → connection failure" (the finding that would have populated the broken-version table), and it means the opt-in `TestLiveFactory` drift guard from plan 02-02 will be flaky if it is ever run in a tight loop. The definite answers this plan relies on (`metal-installer@v1.9.0` → 200, the `-secureboot` suffix, `.raw.zst`) were each confirmed on a separate request.
- **`cmdline-metal-amd64` could not be fetched during the throttled window**; `cmdline-metal-amd64-secureboot` answered `302`, confirming the route shape. The path is taken from `STACK.md`'s verified table.

No authentication gates, no verification failures, no node repairs.

## User Setup Required

None — no external service configuration. Everything in this plan runs offline against the fake; the live probes were exploratory and are not part of any test.

## Next Phase Readiness

**Ready for the wave-3 consumer:**

- Plan 02-06 has the whole surface it needs: `ISOURL`/`PXEURL`/`DiskImageURL`/`CmdlineURL`/`InstallerImage` for the assets route, `SplitVersions`/`NewestStable`/`BrokenReason` for the versions route, `Warnings` for the create response, `store.Schematics()` for persistence, and `docs/api-contract.md` describing exactly what to build — including the two audit action tokens and the allowlist, so neither is invented at implementation time.
- The `upstream` code tokens confirmed by the operator at the wave-1 boundary are referenced by name in the contract section, not re-derived.

**Concerns to carry forward:**

- **The `schematic.create` audit allowlist is a contract, not yet code.** It is the live half of this plan's first prohibition and belongs to plan 02-06. If that entry is written wrong, the mistake is permanent: the archive has no deletion path.
- **The pre-migration tarball now contains schematic records** once any exist, which means it contains kernel arguments and META values in clear in a single portable file. The tarball is 0600, but nothing warns an operator who copies it elsewhere. Worth a line in the operator docs.
- **The broken-version table is empty and nothing will ever populate it by itself.** There is no process that adds an entry when someone discovers a bad version. If FACT-05 is meant to ship with real data, that process — not more code — is what is missing.
- **`TestLiveFactory` remains the only guard between the fake and upstream drift**, it is opt-in, nothing schedules it, and this session's throttling suggests it needs a retry policy before it could run unattended. Unchanged from plan 02-02's carry-forward, now with evidence.
- **`PITFALLS.md` §P9(d)'s `v1.9.0` finding should be marked as superseded.** It was a live observation that no longer reproduces, and it is the sort of note a later phase would otherwise act on.

## Self-Check

- `internal/imagefactory/urls.go` — FOUND
- `internal/imagefactory/urls_test.go` — FOUND
- `internal/imagefactory/installer.go` — FOUND
- `internal/imagefactory/installer_test.go` — FOUND
- `internal/imagefactory/versions.go` — FOUND
- `internal/imagefactory/versions_test.go` — FOUND
- `internal/imagefactory/brokenversions.go` — FOUND
- `internal/imagefactory/brokenversions_internal_test.go` — FOUND
- `internal/imagefactory/warnings.go` — FOUND
- `internal/imagefactory/warnings_test.go` — FOUND
- `internal/store/testdata/version-1/VERSION` — FOUND
- `internal/store/testdata/version-1/users/alice.json` — FOUND
- Commits `7880bf6`, `38f7e05`, `bd034e1`, `56045d1`, `09afb6b`, `1c877a4` — all FOUND
- `go test ./... -count=1 -race` — GREEN
- `golangci-lint run` — GREEN, 0 issues
- `git diff go.mod go.sum` — empty, no new module
- Real version-1 data directory migrated via `fsstore.Open`: VERSION `2`, `backups/pre-migration-1-to-2-2026-08-29T05:26:18Z.tar.gz`, `schematics/` created, `users/alice.json` intact — PASS

## Self-Check: PASSED

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-29*
