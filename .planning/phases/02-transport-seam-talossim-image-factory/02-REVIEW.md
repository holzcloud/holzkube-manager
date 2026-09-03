---
phase: 02-transport-seam-talossim-image-factory
reviewed: 2026-09-03T22:30:00Z
depth: standard
scope: gap-closure rounds 3-4 (diff base 575e747, plans 02-14 .. 02-24)
files_reviewed: 42
files_reviewed_list:
  - .github/workflows/ci.yml
  - .gitignore
  - README.md
  - Taskfile.yml
  - cmd/holzkube-managerd/budget_test.go
  - cmd/holzkube-managerd/main.go
  - docs/api-contract.md
  - internal/httpapi/endtoend_test.go
  - internal/httpapi/handlers/account_test.go
  - internal/httpapi/handlers/budget_drift_test.go
  - internal/httpapi/handlers/schematics.go
  - internal/httpapi/handlers/schematics_test.go
  - internal/httpapi/middleware/audit_test.go
  - internal/httpapi/problem.go
  - internal/httpapi/problem_test.go
  - internal/imagefactory/canonical_live_test.go
  - internal/imagefactory/client.go
  - internal/imagefactory/client_test.go
  - internal/imagefactory/fake_test.go
  - internal/imagefactory/guard_drift_test.go
  - internal/imagefactory/installer.go
  - internal/imagefactory/installer_export_test.go
  - internal/imagefactory/installer_test.go
  - internal/imagefactory/live_test.go
  - internal/imagefactory/probe.go
  - internal/imagefactory/schematicid.go
  - internal/imagefactory/schematicid_test.go
  - internal/imagefactory/tracer_test.go
  - internal/imagefactory/warnings.go
  - internal/imagefactory/warnings_test.go
  - internal/model/model.go
  - web/package.json
  - web/src/api.test.ts
  - web/src/api.ts
  - web/src/components/SudoDialog.test.tsx
  - web/src/lib/problem.test.ts
  - web/src/lib/problem.ts
  - web/src/routes/images.browser.test.tsx
  - web/src/routes/images.test.tsx
  - web/src/routes/images.tsx
  - web/src/test/problem-fixtures.ts
  - web/vite.config.ts
findings:
  critical: 1
  warning: 5
  info: 5
  total: 11
status: issues_found
---

# Phase 02: Code Review Report (gap-closure rounds 3-4)

**Reviewed:** 2026-09-03
**Depth:** standard
**Diff base:** `575e747` (plans 02-14 .. 02-24)
**Files Reviewed:** 42
**Status:** issues_found

## Summary

This delta is genuinely strong in the places it was aimed at. The concurrent
candidate fan-out in `resolveInstallerRepo` (installer.go:613-707) is correctly
built: one slot per candidate indexed by declared position, `close(done[i])` as
the happens-before for `answers[i]`, cancel-then-wait so no goroutine outlives
the call, and the short-circuit taken only when no earlier candidate is still
outstanding. I could not construct a data race or an ordering inversion in it,
and `go test ./internal/imagefactory/ -race -count=2` is clean. The raw-body
surrogate refusal (`rawTextReason`, schematics.go:1079-1128) is the other place I
tried hardest to break: I walked escaped backslashes, `\`, high halves at
the end of the buffer, and well-formed pairs, and the scanner classifies every
one of them correctly. The refusal set really is equal on both sides — the
sweep in `guard_drift_test.go` compares behaviour rather than declarations, and
`REFUSED_RANGES` and `NotRepresentableReason` agree codepoint for codepoint.
The whole Go suite for the changed packages passes.

The one place this round introduced a new *write* to an existing record is where
it fails. `refreshTheStoredVerdict` (schematics.go:513-587) guards the
architecture with a page of argument and does not guard the Talos version at
all — and the schematic id is the hash of a document that contains neither. The
handler's own comment forty lines above says so out loud ("any two authoring
attempts sharing a customisation land here regardless of name, cluster **or the
version they were authored against**"), and then the refresh writes a verdict
about one version onto a record that names another. That is G-02-8's lie in a
new place, arriving through the mitigation built to close G-02-9.

Per the scope note I have not re-filed anything on `.planning/WINDOWS.md`. In
particular: the assets-row budget-table limitation (65), process-wide
`writeTimeout` (59), `CreateRouteBudget` clipping (60), the audit outcome of a
refreshing 409 (61), the missing progress indicator (62), `DisallowUnknownFields`
on the catalog read (9), the META key-0 coercion (10), raw decoder errors
reaching the client (16), the two unused warning-code constants (27/51), the
SecureBoot example's missing `warnings` (25/50), the raw transport error in the
fallback detail (24), and the un-run CI browser step (44) are all already
recorded and are deliberately absent below.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: A conflicting POST at a different Talos version overwrites the stored verdict with a verdict about a version the record does not name

**File:** `internal/httpapi/handlers/schematics.go:513-587` (the missing guard belongs beside
line 554), reached from `internal/httpapi/handlers/schematics.go:429-462`

**Issue:** `refreshTheStoredVerdict` writes `Usable`, `ProbedAt` and `ProbeReason` onto the
stored record after exactly two conditions:

```go
if fresh.ProbedAt.IsZero() { ... return }          // the fresh probe answered
if stored.Arch != fresh.Arch { ... return }        // the architecture matches
stored.Usable = fresh.Usable
stored.ProbedAt = fresh.ProbedAt
stored.ProbeReason = fresh.ProbeReason
```

There is no third condition on `TalosVersion`, and there needs to be, because the
record's identity cannot vary by version either. `Schematic.Canonical()`
(`internal/imagefactory/schematicid.go:125-178`) emits `owner`, `overlay` and
`customization` and nothing else — no architecture *and no Talos version*. The id
is the SHA-256 of those bytes, so two authoring attempts that differ only in
`talos_version` are one record and collide on `store.ErrConflict`. The handler
knows this: its own comment at lines 433-436 says the collision happens
"regardless of name, cluster **or the version they were authored against**".

The verdict, however, *is* version-scoped, on exactly the evidence the Arch guard
cites for itself. `ProbeBuildable` takes `talosVersion` and probes that version's
ISO URL, `Extensions` is version-scoped, and the refusal sentence the guard quotes
as proof that the verdict is architecture-scoped —
`<id> at <version>/<arch> answered HTTP <status>` (`probe.go:71`) — carries the
version in the same breath as the architecture. `model.Schematic.TalosVersion`'s
own doc comment already states that a stored verdict that does not name its
version cannot be read back.

Concrete sequence, both directions wrong:

1. Operator authors *"workers with intel microcode"* at `v1.12.0`. The probe
   succeeds. Record: `talos_version: v1.12.0`, `usable: true`, `probe_reason: ""`.
2. Later they author the identical customisation at `v1.13.9` — an ordinary action;
   the form's version selector is right there. `Author` fetches the `v1.13.9`
   catalog (the extension is still in it, so validation passes), POSTs, gets the
   *same* id back, and `ProbeBuildable` at `v1.13.9` is refused because the
   extension is not built for that version yet → `ErrSchematicNotBuildable`.
3. `probedAt` is stamped, `probeReason` is set, `Put` returns `ErrConflict`,
   `refreshTheStoredVerdict` runs. `ProbedAt` is non-zero; `stored.Arch == fresh.Arch`
   (both `amd64`). **The record is written to.**
4. The stored record now reads `talos_version: v1.12.0`, `usable: false`,
   `probe_reason: "<id> at v1.13.9/amd64 answered HTTP 400"`.

`UsabilityVerdict` (`web/src/routes/images.tsx:874-960`) renders
**"Not usable — the Factory refused to build it"** for a schematic that builds
perfectly well at the only version the record names, and the detail dialog above
it says "Authored against v1.12.0" beside a reason naming v1.13.9. An operator
who deletes and re-authors on the strength of that badge has been sent to fix
something that is not broken.

The reverse ordering is the worse half. Author at `v1.13.9` where it is refused
(`usable: false`), then re-POST the identical customisation at `v1.12.0` where it
builds. The refresh writes `usable: true` onto a record whose `talos_version` is
`v1.13.9`, and the screen asserts **"Usable — the build probe confirmed it"**
about a version at which the probe did nothing of the kind. That is precisely the
claim T-02-62 and G-02-1 exist to prevent, and it is now producible through the
supported UI in two clicks.

The refresh also silently rewrites the operator's `probe_reason` into a sentence
about a version that appears nowhere else in the record, which is the only place
the discrepancy is visible at all — and nothing surfaces it, because
`refreshTheStoredVerdict`'s success clause reports "the stored verdict was
refreshed. Nothing else about the record was changed."

**Nothing catches this.** `TestConflictAtAnotherArchitectureDeclinesTheRefresh`
(`schematics_test.go:3088`) covers the architecture guard;
`createBody` (`schematics_test.go:483-494`) hardcodes `catalogVersion`, so no test
anywhere varies the version across a conflict, and
`TestConflictRefreshTouchesNothingButTheThreeProbeFields` deliberately excludes
the three fields at issue.

**Fix:** Add the version condition beside the architecture one and reuse the same
shape, so a reader meets one rule stated twice rather than two rules:

```go
// The third condition: the stored record's Talos version is the one the fresh
// probe asked about.
//
// The verdict is version-scoped for the same reason it is architecture-scoped,
// and on the same evidence: ProbeBuildable probes one version's ISO, the
// extension catalog is version-scoped, and ProbeReason names `<version>/<arch>`
// in one sentence. This record's identity cannot vary by version either --
// Canonical() emits no version -- so a v1.13.9 verdict written onto a v1.12.0
// record would be undetectable afterwards, there being no second record to
// disagree with it.
if stored.TalosVersion != fresh.TalosVersion {
    return " The stored verdict was not refreshed, because this record holds the verdict " +
        "for " + stored.TalosVersion + " and this submission asked about " +
        fresh.TalosVersion + ", and one stored customisation holds exactly one " +
        "version's verdict."
}
```

and add the regression test the architecture guard already has, in the register
beside it:

```go
// TestConflictAtAnotherTalosVersionDeclinesTheRefresh
body := createBody("later-version-attempt", []string{"siderolabs/intel-ucode"}, nil)
body["talos_version"] = otherCatalogVersion
// ... assert marshalRecord(after) == marshalRecord(before) and that the 409
// detail names both versions.
```

If the project instead decides a cross-version refresh is *wanted*, that is a
different change and a larger one: it needs a place on the record to hold a
second version's verdict, which `model.Schematic` does not have — and
`02-DECISION-schematic-identity.md` is where that has to be argued, not here.

## Warnings

### WR-01: A malformed `secureboot` query value is silently read as `false`, on the one route whose own comments call that substitution undetectable

**File:** `internal/httpapi/handlers/schematics.go:942-944`

**Issue:**

```go
// Anything other than an explicit true is false. A parse error here would
// be a 400 for a parameter whose absence is already meaningful.
secureBoot, _ := strconv.ParseBool(q.Get("secureboot"))
```

`strconv.ParseBool` accepts only `1 t T TRUE true True 0 f F FALSE false False`.
`secureboot=yes`, `secureboot=on`, `secureboot=Y`, or the plain typo
`secureboot=ture` all return an error, the error is discarded, and the request is
served as an **ordinary** (non-SecureBoot) request: five ordinary references, no
SecureBoot warning, no indication that the parameter was not understood.

That is the one outcome this route is written to make impossible. Forty lines
above, `schematicAssets` (schematics.go:691-695) says a SecureBoot substitution
"would be undetectable forever: SecureBoot is a query parameter that never
reaches the stored record, so no later code, log or audit entry can re-derive
that a substitution happened", and `docs/api-contract.md:735-741` repeats it. The
resolution machinery goes to considerable lengths to refuse rather than
substitute — and then the *parameter parse* substitutes for free.

The asymmetry is the tell: every other parameter on this route answers `400` on a
value it does not understand. `arch` does (line 916-920), `version` does (926-930),
`platform` does (936-940). `secureboot` is the only one that guesses, and it is the
only one whose wrong guess is unrecoverable.

The comment's justification does not hold either: absence being meaningful is a
reason to accept the *empty* string, not a reason to accept `yes`.

**Fix:** Distinguish absent from malformed and refuse the second, in the register
the other three parameters already use:

```go
secureBoot := false
if raw := q.Get("secureboot"); raw != "" {
    parsed, err := strconv.ParseBool(raw)
    if err != nil {
        return imagefactory.AssetRequest{}, httpapi.Validation(
            "The SecureBoot selection is not a boolean, and it is never guessed: it is what "+
                "selects the installer repository, and an ordinary installer served for a "+
                "SecureBoot request is undetectable from the stored record afterwards.",
            httpapi.FieldError{Field: "secureboot", Reason: "must be true or false"})
    }
    secureBoot = parsed
}
```

Add the two-row table test (`?secureboot=yes` → 400, absent → 200 with ordinary
references) and state the accepted values in `docs/api-contract.md` beside the
`arch` and `version` bullets at line 708-741.

### WR-02: `storeInstallerRepo` can permanently record the fallback repository name as proven while a concurrent resolver observed a 2xx from the preferred name

**File:** `internal/imagefactory/installer.go:378-387`, with the branch that mints the
conflicting entry at `installer.go:420-431`

**Issue:** `storeInstallerRepo`'s rule is "a proven entry is never overwritten", and its
doc comment justifies last-write-wins among *unproven* entries with an invariant:

> "Among unproven entries the last write wins, and that is harmless: on one key they
> carry the same repository name, because the candidate order is fixed and the first
> 2xx wins."

That invariant is true of every entry produced by a 2xx. It is **not** true of the
entry produced by the `ErrSchematicNotBuildable` promotion branch, which mints a
*proven* entry whose `repo` was chosen by an earlier, different walk:

```go
case errors.Is(err, ErrSchematicNotBuildable):
    next = installerRepoEntry{repo: entry.repo, at: time.Now()}   // proven, no warning
```

So two resolvers on one key can hold two different proven answers, and the first
to reach the mutex wins forever — proven entries never expire and are never
re-questioned (`installerRepo`, line 294).

Concretely, on key `metal/v1.13.9/secureboot=false` with a stale provisional entry
`{repo:"installer", unresolved:["metal-installer"]}`:

- Goroutine A re-questions `metal-installer` and gets `404` (a registry mid-publication,
  a CDN miss, a shard that has not caught up). A promotes `{repo:"installer"}` to
  **proven** and stores it.
- Goroutine B re-questions the same name a moment later and gets `200`. B builds
  `{repo:"metal-installer"}` — proven, by the preferred name, by a positive
  observation — calls `storeInstallerRepo`, finds A's proven entry, discards its own
  and is handed `installer`.

The cache is now permanently `installer` for that key, chosen by a single negative
observation, over a name that answered `2xx` in the same second, with the warning
dropped. `InstallerImage`'s own doc comment states the cost of a wrong installer
reference: an upgrade that reports success and silently drops every system
extension the node was built with (P9(c)). Unlike the divergence
`02-REVIEW-FIX.md` disclosed as acceptable, this one carries **no** warning — the
promotion branch clears it — so there is nothing on the panel saying the answer
was not the preferred name's.

It needs contradictory answers from the registry, which is why this is a warning
and not a blocker. It does not need concurrency to be *reachable* in a weaker
form: a single transient `404` on a re-question promotes the fallback to proven
forever with no expiry and no warning, on the strength of one observation the
comment itself calls "exactly as fresh as any proven entry's own single 2xx" —
which is an argument about the *name being usable*, not about the *preference
order* the file spends 60 lines establishing.

**Fix:** Do not let a negative observation mint a proven entry that outranks a
positive one on the preferred name. Either keep the promotion but make it lose to
a repo-preferring comparison:

```go
func (c *Client) storeInstallerRepo(key string, next installerRepoEntry) installerRepoEntry {
    c.installerMu.Lock()
    defer c.installerMu.Unlock()

    current, ok := c.installerRepos[key]
    // A proven entry is not overwritten by an unproven one, and a proven entry
    // is not overwritten by another proven one UNLESS the newcomer names a
    // candidate the declared order prefers. A name proven by its own 2xx beats a
    // name promoted because a preferred candidate answered 404 once.
    if ok && current.proven() && !prefers(next.repo, current.repo) {
        return current
    }
    c.installerRepos[key] = next
    return next
}
```

or — the shape I would prefer, and which closes WINDOWS entry 22 in the same
move — put a `singleflight.Group` keyed on `key` around the whole of
`installerRepo`, so there is only ever one resolver per key and the question
cannot be answered twice at once. Whichever is taken, the promotion branch should
say in its comment that it is minting a proven entry from a *negative*
observation, because that is the fact the current text elides.

### WR-03: `REQUEST_CEILING_MS` transcribes `writeTimeout` across the language seam with no drift guard, while its two siblings have one

**File:** `web/src/api.ts:453-475`

**Issue:** The constant's own doc comment states its derivation as a fact about another
file: *"The number it is above is `writeTimeout` in cmd/holzkube-managerd/main.go,
130 seconds"*, and argues that a ceiling below it "would abort while the server is
still working and replace an honest problem+json … with a generic network
failure". That is a correct argument and nothing enforces it.

This is the third Go→TypeScript transcription in the phase and the only one
without a guard. `ASSETS_WAIT_SECONDS` and `CREATE_WAIT_SECONDS`
(`images.tsx:220-233`) are pinned by
`internal/httpapi/handlers/budget_drift_test.go`, which reads the literals out of
`images.tsx` and fails in both directions. The warning sentences are pinned by
`TestWarningDetailsMatchTheUI`. The installer names are pinned by
`TestBrowserInstallerNamesEqualInstallerCandidates`. `REQUEST_CEILING_MS` is pinned
by nothing.

`TestRouteBudgetTableReadsTheRealConstants` does go red if `writeTimeout` moves,
but its failure message sends the reader to `routeBudgets` in
`cmd/holzkube-managerd/budget_test.go` and names no client-side constant, so the
one file that has to move with it is the one nothing points at. Raising
`writeTimeout` to 160s would leave every long create aborting at 150s with
"The server did not answer within 150 seconds" in place of the problem+json the
comment exists to protect.

**Fix:** Add a third row to the guard that already reads this seam. It reads
`images.tsx`; `api.ts` is beside it:

```go
// budget_drift_test.go
const apiPath = "../../../web/src/api.ts"

// TestTheClientCeilingSitsAboveTheServersResponseBudget reads REQUEST_CEILING_MS
// out of web/src/api.ts and asserts it exceeds writeTimeout. A ceiling at or
// below it aborts while the server is still working and replaces an honest
// problem+json with a network failure.
```

`writeTimeout` is unexported in package `main`, so either export it (as the route
budgets already were, for exactly this reason) or move the assertion into
`cmd/holzkube-managerd/budget_test.go`, which already reads it and already has the
table this row belongs in.

### WR-04: The refusal-set drift guard is not anchored to the declaration it claims to read

**File:** `internal/imagefactory/guard_drift_test.go:49-50`, used at `170-196`

**Issue:**

```go
var browserRefusalRange = regexp.MustCompile(
    `\{\s*from:\s*0x([0-9a-fA-F]+),\s*to:\s*0x([0-9a-fA-F]+)`)
```

`browserRefusalRanges` runs this over the whole of `images.tsx` and treats every
match as an entry of `REFUSED_RANGES`. Nothing ties it to that identifier. Two
failure modes follow, and both are silent:

- Any *other* `{ from: 0x…, to: 0x… }` object literal added to `images.tsx` — a
  second table, a fixture, a codepoint range for some future control — is folded
  into the set this test believes the browser refuses. If it happens to cover a
  codepoint the server accepts, the guard stops reporting a real over-refusal.
- `REFUSED_RANGES` itself can be renamed, moved to another module, or deleted
  without the `t.Fatalf` at line 176 firing, as long as one such literal survives
  anywhere in the file. The Fatalf's own comment says "A guard that silently
  passes when it can no longer find what it guards is worse than no guard" —
  which is the property it does not have.

This is the same anchoring discipline the sibling guard already applies:
`budget_drift_test.go:89-90` anchors on `^\s*(?:export\s+)?const\s+NAME\s*=` and
says why ("so a number that merely appears somewhere in the file cannot satisfy
this"). One of the two guards written in the same round follows that rule and the
other does not.

**Fix:** Cut the declaration out first, then scan only inside it:

```go
// The declaration, not the file: an object literal with from/to elsewhere in
// images.tsx is not part of the refusal set, and REFUSED_RANGES being renamed
// or deleted has to be a failure rather than a shorter list.
var refusedRangesDecl = regexp.MustCompile(
    `(?s)const\s+REFUSED_RANGES\s*:[^=]*=\s*\[(.*?)\]`)

decl := refusedRangesDecl.FindStringSubmatch(source)
if decl == nil {
    t.Fatalf("%s declares no REFUSED_RANGES this guard can read. …", imagesRoutePath)
}
matches := browserRefusalRange.FindAllStringSubmatch(decl[1], -1)
```

While there: the entry regex silently skips any range written with a decimal
literal or a named constant. Failing on an entry it cannot parse (as
`stringArrayLiteral` and `exportedWarningCodes` both already do) is the honest
answer.

### WR-05: `allowedHosts`'s doc comment is attached to `ssoOnly`

**File:** `cmd/holzkube-managerd/main.go:288-306`

**Issue:** The comment block that documents `allowedHosts` — DNS rebinding, the
self-referential CSRF preconditions, the agreement with the certificate's SANs —
runs straight into `ssoOnly`'s own sentence with no blank line and no separating
declaration, so it *is* `ssoOnly`'s doc comment:

```go
// allowedHosts is every Host header value this instance answers to.
//
// It closes DNS rebinding: …
// The set is the bind address plus the loopback names, which is the same set
// tlsx puts in the certificate's SANs — …
// ssoOnly reports, for a Host header, whether the local password is refused
// there. …
func ssoOnly(cfg config.Config) func(string) bool {
```

`godoc` and every editor hover therefore render a DNS-rebinding rationale above a
function that has nothing to do with DNS rebinding, and `allowedHosts` (line 308)
— the function that *does* close it, and the one a security reader will go
looking for — is left undocumented. This is a security-relevant control whose
entire justification now sits on the wrong symbol; the merge happened when the
SSO work inserted `ssoOnly` between the comment and its function.

**Fix:** Move the block back onto `allowedHosts` and leave `ssoOnly` its own two
sentences. `gofmt` will not catch this and `golangci-lint` does not either, so it
is worth a `revive`/`godot`-adjacent rule or, failing that, a reviewer's eye at
every insertion between a comment and its declaration.

## Info

### IN-01: `PRESENTATION_BY_CODE` calls itself the full closed taxonomy and has no `upstream.` entry

**File:** `web/src/lib/problem.ts:89-109`

**Issue:** The comment says *"The full closed taxonomy from `docs/api-contract.md` §
Error Taxonomy, one entry per code prefix"*. The list has thirteen entries and the
`upstream.` family — four codes, and the single most-exercised family this phase
produced — is not among them. Behaviour is fine: `presentationFor` falls through to
`'toast'`, which is the right presentation. The claim is what is wrong, and it
matters because this round *added* `CODE_UPSTREAM_FACTORY_UNAVAILABLE` and
`CODE_UPSTREAM_FACTORY_REJECTED` twenty lines above it, so the file now names the
family in one place and omits it from the list that says it is complete.

**Fix:** Add `['upstream.', 'toast']` before `['internal.', 'toast']`, so the list
is what its comment says it is and a future decision to present an upstream
failure differently has an anchor to change.

### IN-02: Any abort is reported to the operator as the 150-second ceiling firing

**File:** `web/src/api.ts:512-517`, used at `561-566` and `583-589`

**Issue:** `isCeilingAbort` returns true for `AbortError` as well as `TimeoutError`,
and both paths then throw `ceilingError()`, whose sentence is *"The server did not
answer within 150 seconds, so the request was given up on."* `AbortSignal.timeout`
rejects with `TimeoutError`; `AbortError` comes from somewhere else — a page
navigation, an extension, a browser-initiated cancel, or the first caller who
threads an external signal through `send`. Any of those is reported as a
150-second server timeout that did not happen, on a screen whose whole design
principle is that a sentence must be true of what actually occurred.

**Fix:** Match `TimeoutError` only, and let anything else propagate as itself:

```go
function isCeilingAbort(cause: unknown): boolean {
  return cause instanceof DOMException && cause.name === 'TimeoutError'
}
```

If a caller-supplied signal is added later, that abort deserves its own sentence
rather than this one.

### IN-03: The TypeScript problem-type base is a second unguarded transcription of `ProblemBaseURI`

**File:** `web/src/test/problem-fixtures.ts:15`

**Issue:** The file's own comment names the risk — *"the Go constant
`httpapi.ProblemBaseURI` is its one authority; the web suite cannot import that, so
this is the single place the string is spelled on this side of the seam"* — and
nothing checks it. `.planning/WINDOWS.md` entry 46 records the same hazard for
`audit_test.go:126` and does not name this file.

The consequence is quieter than entry 46's and therefore easier to miss: a stale
base here produces **no** red test, because `lib/problem.ts` branches on
`problem.code` and never on `problem.type`. The fixtures simply stop representing
what the server emits, and every assertion built on them keeps passing while
describing a wire format that no longer exists.

**Fix:** Either pin it from Go, in the register `budget_drift_test.go` already
established (a five-line test reading the literal out of `problem-fixtures.ts` and
comparing it to `httpapi.ProblemBaseURI`), or append a line to WINDOWS entry 46's
successor naming this file too, so the next re-rooting has both spellings in one
place.

### IN-04: A whitespace-only schematic name passes both the form's guard and the server's

**File:** `web/src/routes/images.tsx:379-393` and `578`, with the server's check at
`internal/httpapi/handlers/schematics.go:795-799`

**Issue:** `submit` trims kernel arguments and drops blank META rows, and sends `name`
exactly as typed. The submit button is disabled on `name === ''` and the server
refuses on `in.Name == ""`. `"   "` satisfies neither test, so a schematic can be
created whose only human-readable label is three spaces — rendered as an empty
cell in the saved table and as an empty `DialogTitle`. `validate`'s own reason
string says a schematic "needs a label the operator can recognise it by", which a
blank one is not.

**Fix:** Compare on the trimmed value on both sides — `strings.TrimSpace(in.Name) == ""`
on the server (which is the half that matters, since it is the contract), and
`name.trim() === ''` in the disabled expression so the form does not offer a
submission the server will refuse.

### IN-05: `schematicAssets`'s budget comment still describes the serial candidate walk in the present tense

**File:** `internal/httpapi/handlers/schematics.go:713-719`

**Issue:**

> "a cold `resolveInstallerRepo` **walks two candidates in series**, each with its own
> manifest budget, and the sum of those two was the 60.000s worst case…"

It does not, since plan 02-23. It asks every candidate at once
(`installer.go:613-707`), and the constant this comment is justifying —
`AssetsRouteBudget` at `schematics.go:60-83` — says so at length one screen above
("One manifest budget and not two, because `resolveInstallerRepo` asks every
candidate repository name at the same time"). The two comments in one file now
contradict each other about the mechanism, and the stale one is the one a reader
meets at the call site.

This is the class of drift the file elsewhere treats as load-bearing: it is the
same "a fact with an expiry date that nothing in the build checks" that
`installerCandidates` deleted its digest literals over.

**Fix:** Rewrite the clause in the past tense, matching what `AssetsRouteBudget`'s
own comment already says:

```go
// One deadline over the whole resolution, for the same reason createSchematic
// derives one. A cold resolveInstallerRepo used to walk its candidates in
// series, each with its own manifest budget, and the sum of those two was the
// 60.000s worst case that arrived against a 60s writeTimeout as a problem
// document written to an expired socket. It now issues every candidate at once,
// so the worst case is one manifest budget; the ceiling is what the route
// declares and the sum only says whether it clips.
```

---

_Reviewed: 2026-09-03_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
