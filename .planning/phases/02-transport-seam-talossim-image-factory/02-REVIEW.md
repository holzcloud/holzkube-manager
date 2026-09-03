---
phase: 02-transport-seam-talossim-image-factory
reviewed: 2026-08-29T00:00:00Z
depth: standard
scope: gap-closure round 2 (diff base c1c65a2, plans 02-09 .. 02-13)
files_reviewed: 22
files_reviewed_list:
  - cmd/holzkubed/budget_test.go
  - docs/api-contract.md
  - internal/httpapi/handlers/schematics.go
  - internal/httpapi/handlers/schematics_test.go
  - internal/imagefactory/client.go
  - internal/imagefactory/fake_test.go
  - internal/imagefactory/installer.go
  - internal/imagefactory/installer_export_test.go
  - internal/imagefactory/installer_test.go
  - internal/imagefactory/live_test.go
  - internal/imagefactory/probe.go
  - internal/imagefactory/probe_test.go
  - internal/imagefactory/schematicid.go
  - internal/imagefactory/schematicid_test.go
  - internal/imagefactory/urls.go
  - internal/imagefactory/warnings.go
  - internal/imagefactory/warnings_test.go
  - internal/model/model.go
  - web/src/api.ts
  - web/src/components/SchematicWarnings.tsx
  - web/src/routes/images.test.tsx
  - web/src/routes/images.tsx
findings:
  critical: 1
  warning: 6
  info: 5
  total: 12
status: issues_found
---

# Phase 02: Code Review Report (gap-closure round 2)

**Reviewed:** 2026-08-29
**Depth:** standard
**Diff base:** `c1c65a2`
**Files Reviewed:** 22
**Status:** issues_found

## Summary

This round closed G-02-3 through G-02-8. The taxonomy unification (`registryRefused`
as the single classifier, probe.go:70-93) is genuinely correct and the shared
`registryAnswerTable` consumed by both call sites is the right shape. The
serialiser-refusal split (`NotRepresentableError` → 400 naming a request field) is
sound and the value-echo prohibition holds: `representable` reports a character
class via `%U` and never the scalar, `refusalReason` composes only an index and
that class, and `factoryProblem` never interpolates `err.Error()`. I traced every
path from a kernel arg or META value to a problem body and found no leak of the
value itself.

The one place this round introduced new mutable shared state — the provisional
installer-repo cache entry and its re-question — is not safe under concurrent
resolution, and the failure it produces is precisely the divergence G-02-3 was
opened for, now reachable inside a single process instead of only across a
restart. There is no concurrency test anywhere in `installer_test.go` (no
`go func`, no `t.Parallel`, no `-race`-specific case), so nothing would have
caught it.

Per the scope note I have not filed the probe budget, `DefaultTimeout`, or the
`2 x DefaultTimeout` composition on the assets route: those are WINDOWS entry 20
and `knownOverBudget` rows, deferred to `02-DECISION-probe-budget.md` by design.
`cmd/holzkubed/budget_test.go` correctly declares them and its ratchet works in
both directions.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: Concurrent re-question silently reverts a proven installer repository to the provisional one

**File:** `internal/imagefactory/installer.go:298-350` (write site `346-348`), with the same
class of bug at `installer.go:244-272` (write site `268-270`)

**Issue:** `installerRepo` reads the cache entry under `installerMu`, *releases the
lock*, resolves against the registry (up to `DefaultTimeout`), and then writes the
result back with an unconditional `c.installerRepos[key] = next`. Nothing re-reads
the map under the write lock, so the last writer wins regardless of what it learned
or when it started.

Concrete sequence, all on one key (`metal/v1.13.9/secureboot=false`):

1. Cache holds a stale provisional entry `E{repo:"installer", unresolved:["metal-installer"], at:T0}`.
2. Requests A and B arrive concurrently at `T0 + 6min`. Both read `E` at line 248,
   both see it stale at line 252, both call `requestionInstallerRepo(ctx, r, key, E)`.
3. A's probe of `metal-installer` answers `200`. A takes the `err == nil` branch
   (line 303-310), builds `{repo:"metal-installer", unresolved:nil}` — **proven** —
   and stores it at line 347.
4. B's probe of the same name is silent and times out. B takes the `default` branch
   (line 325-343), where `next` is still the local copy of `E`, sets `next.at = time.Now()`
   at line 343, and stores it at line 347 — **overwriting A's proven entry**.

Consequences, in ascending order of severity:

- A was served `factory.talos.dev/metal-installer/<id>:v1.13.9` and B was served
  `factory.talos.dev/installer/<id>:v1.13.9` for the same schematic at the same
  version, in the same process, at the same moment. That is the exact symptom
  `installer.go:26-29` says G-02-3 was: *"two processes served two different
  repository names for one schematic at one version"*. The re-question mechanism
  reintroduces it without needing a restart.
- The cache is left holding the **fallback** name after the preferred name had
  already answered `2xx`. Every subsequent request for the next five minutes is
  served the wrong repository. `InstallerImage`'s own doc comment
  (`installer.go:120-128`) states what that costs: the reference is consumed by the
  upgrade RPC and a wrong one "produces an upgrade that reports success and
  silently drops every system extension the node was built with (P9(c))".
- The reverted entry is provisional, so it will be re-questioned again in five
  minutes — and can be reverted again by any concurrently in-flight slow probe.
  The state does not converge.

The `errors.Is(err, ErrSchematicNotBuildable)` branch (line 312-323) has the same
hazard in the other direction: it writes `{repo: entry.repo}` from a stale local
read, so it can demote a fresher proven answer for the *preferred* name back to the
fallback name and mark that proven.

The `installerRepo` cold path (line 258-270) is the same pattern: a slow first
resolution can overwrite a proven entry another goroutine wrote while it was in
flight.

**Fix:** Make the write conditional on the entry not having moved, or serialise
resolution per key. The smallest correct change is a compare-and-set that also
refuses to demote a proven entry:

```go
// requestionInstallerRepo, replacing lines 346-349
c.installerMu.Lock()
current, ok := c.installerRepos[key]
// Only write if nobody improved on the entry we started from. A proven entry is
// never demoted, and a re-stamp of a timestamp we did not read is not ours to make.
if !ok || (!current.proven() && current.at.Equal(entry.at)) {
    c.installerRepos[key] = next
} else {
    next = current
}
c.installerMu.Unlock()
return next
```

and the same guard in `installerRepo` before line 269:

```go
c.installerMu.Lock()
if current, ok := c.installerRepos[key]; !ok || !current.proven() {
    c.installerRepos[key] = entry
} else {
    entry = current
}
c.installerMu.Unlock()
```

A `golang.org/x/sync/singleflight` group keyed on `key` around both resolution
paths would fix this *and* WR-02 in one move, and is the shape I would prefer.

Whatever the fix, add the test that is currently absent: N goroutines calling
`InstallerImage` on one key against a fake whose preferred repo is `unreachable`
for the first probe and `200` thereafter, asserting under `-race` that every
returned reference is identical and that the final cache entry is proven.

## Warnings

### WR-01: A cancelled request is recorded as "the registry was silent again" and re-stamps the cache entry

**File:** `internal/imagefactory/installer.go:325-343`, via `internal/imagefactory/probe.go:96-107`

**Issue:** `probeStatus` wraps *every* error from `c.http.Do` as
`ErrUpstreamUnavailable`, including `context.Canceled` and
`context.DeadlineExceeded` from the inbound HTTP request's context. In
`requestionInstallerRepo` this lands in the `default` branch, which re-stamps
`next.at = time.Now()` and stores it.

So an operator closing the asset panel, a browser aborting the fetch, or the
handler returning while the probe is in flight all reset the re-question cadence by
a full five minutes — and the registry was never actually asked. Under a UI that
retries or a user who reopens the dialog repeatedly, a provisional entry can be
kept from ever being promoted, which defeats the mechanism the whole file was
rewritten for. The `default` branch's comment ("Silent again, which is exactly what
a throttling factory.talos.dev produces") is asserting something the code cannot
distinguish.

**Fix:** Treat a caller-side cancellation as "no attempt was made" — leave the
timestamp alone so the next request re-questions immediately:

```go
default:
    if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
        // The caller went away before the registry answered. Nothing was learned,
        // so nothing is re-stamped: the next request asks again rather than
        // inheriting a cadence a cancelled request set.
        return entry
    }
    next.at = time.Now()
```

(Returning early also skips the map write, which removes one of CR-01's write
sites.)

### WR-02: No single-flight: every concurrent request on a cold or stale key issues its own registry resolution

**File:** `internal/imagefactory/installer.go:244-272`, `298-350`

**Issue:** The lock is held only for the map read and the map write. N concurrent
`GET /assets` requests on one uncached key each walk the full candidate list, and N
concurrent requests on a stale provisional key each issue their own re-question.
Against a registry the package's own comments describe as throttling
(`warnings.go:41-44`, `probe.go:84-86`), this is the load pattern most likely to
produce the `429` that makes the entry provisional in the first place — the
mechanism amplifies the condition it exists to describe. It is also the precondition
for CR-01: without concurrent resolvers on one key there is no lost update.

**Fix:** Serialise per key. `singleflight.Group.Do(key, ...)` around the body of
`installerRepo` collapses the herd to one upstream call and makes the cache write a
single-writer operation.

### WR-03: `predictWarnings` does not implement the server's predicate, despite claiming to

**File:** `web/src/components/SchematicWarnings.tsx:63,69`

**Issue:** The doc comment above the function says *"The predicate itself is the
server's, transcribed"*. It is not. The server is
`len(s.Customization.ExtraKernelArgs) > 0` and `len(c.Meta) > 0`
(`internal/imagefactory/warnings.go:105,115`) — presence of any entry. The client is
`kernelArgs.some((arg) => arg.trim() !== '')` and
`meta.some((entry) => entry.value.trim() !== '')` — presence of a *non-blank* entry.

For a record created through this UI the two agree only because `submit`
(`web/src/routes/images.tsx:229-232`) filters blanks before sending. For any record
not created through this form — a record created via the API, or one written by an
earlier build — `SchematicDetailBody` (`images.tsx:1047`) calls `predictWarnings`
directly on the stored record and will render **no warning** for a schematic the
server's own `Warnings()` warns about. The same schematic shows the warning on the
create panel (which uses the server's list) and no warning when reopened, which is
worse than either alone: it teaches the operator the panel is unreliable.

**Fix:** Transcribe the server's predicate as stated, and leave the blank-filtering
to `submit` where it already lives:

```ts
if (kernelArgs.length > 0) { ... }
if (meta.length > 0) { ... }
```

If the live form must not warn on an empty row the operator just added, keep the
`trim` version for `LiveSchematicWarnings` only, and give
`SchematicDetailBody` a predicate that matches the server. Do not leave one function
claiming to be both.

### WR-04: A lone surrogate is silently rewritten rather than refused, contrary to the stated contract

**File:** `web/src/routes/images.tsx:88-105` (comment at `78-82`), `internal/imagefactory/schematicid.go:292-302`

**Issue:** `hasControlCharacter`'s comment says a lone surrogate "is unreachable
from a normal input event and is left to the server's 400". The server does not
answer 400. `JSON.stringify` emits a lone surrogate as a well-formed `\udXXX`
escape; Go's `encoding/json` decodes an unpaired surrogate escape to `U+FFFD`. By
the time `representable` sees the string it is valid UTF-8 with no control
characters, so it is accepted — and the schematic is created, and the id is computed,
over a value the operator did not type.

That is exactly the behaviour T-02-67 is cited to forbid two paragraphs above
(*"An operator who pasted a value from somewhere is better served by being told than
by having their input quietly rewritten into something they did not type"*). The
`utf8.ValidString` branch in `representable` is therefore unreachable from the HTTP
route; it can only fire for a caller inside the process.

**Fix:** Either extend the client check to surrogates and drop the false claim:

```ts
export function hasControlCharacter(value: string): boolean {
  for (const character of value) {
    const code = character.codePointAt(0) ?? 0
    if (code < 0x20 || code === 0x7f || (code >= 0xd800 && code <= 0xdfff)) {
      return true
    }
  }
  return false
}
```

or make the server refuse `U+FFFD` arriving in a field that had none — and in either
case correct the comment, which currently documents a guarantee that does not exist.

### WR-05: `TestWarningsCodesAreNamespaced` still has the defect its own comment says it fixed

**File:** `internal/imagefactory/warnings_test.go:131-141` (comment), `152-155` (the loop)

**Issue:** The rewritten comment states the old test *"is a shape that fails
silently: a third code outside the prefix would not turn it red, it would simply not
be iterated"*, and then asserts *"Every code the package exports must be in this
list"*. The new test still iterates a hand-written `[]string` literal of three
constants. A fourth exported warning code is still simply not iterated, and nothing
enforces the "must be in this list" claim. The only thing that changed is the number
of accepted prefixes, which is the part that was not broken.

**Fix:** Enumerate the exported codes rather than listing them, so the guard cannot
be bypassed by adding a constant. The cheapest honest version reads the package's
own source for `Warning...` constants (`go/ast` over `warnings.go`, in the register
`TestWarningDetailsMatchTheUI` already uses to read a file outside the package). If
that is judged too much machinery, delete the "every code the package exports"
sentence — a comment claiming a guarantee the test does not provide is worse than no
comment.

### WR-06: The provisional-warning branch of `GET /assets` has no handler-level test

**File:** `internal/httpapi/handlers/schematics_test.go:1195-1226`

**Issue:** `TestAssetsCarriesTheWarningsFieldOnTheHappyPath` asserts only the
`warnings: []` case (proven name). Grepping the tree, no test anywhere drives a
*non-empty* `warnings` array through `schematicAssets` to the response body. The
package-level tests in `installer_test.go` cover `InstallerImage` returning the
warning, and `images.test.tsx:764` covers the UI rendering it, but the seam between
them — that `schematicAssets` propagates a non-nil warning list into
`assetReferences.Warnings` rather than dropping it — is untested. That seam is the
whole of G-02-3's user-visible half, and the handler is where it would be lost (the
nil-normalisation at `schematics.go:395-397` is the only code touching it).

**Fix:** Add a handler test that marks the preferred repo unreachable on the fake
(`fakeFactory.unreachable`, already exposed for exactly this) and asserts the raw
response body contains `installer.repo-fallback-unverified`, that `installer` is the
legacy reference, and that the status is `200` and not `502`.

## Info

### IN-01: The raw transport error is echoed verbatim into an operator-facing, long-lived string

**File:** `internal/imagefactory/installer.go:360-371`, via `internal/imagefactory/probe.go:104`

**Issue:** `installerFallbackWarning` interpolates `res.unanswered` with `%v`. That
chain ends in `probeStatus`'s wrap of the `*url.Error` from `http.Client.Do`, so the
`detail` a browser renders contains the full upstream URL and Go's raw dial/TLS
error text. Nothing in it is a kernel argument or a META value — the prohibition
holds — but it is more upstream internals than the sentence needs, in a field the
comment itself says "outlives the form that produced it".

**Fix:** Name the failure class rather than pasting the error: `fmt.Errorf` a short
form at the `resolveInstallerRepo` call site, or render
`errors.Unwrap`-stripped text. Low priority; flagged so the choice is deliberate.

### IN-02: The SecureBoot example in the contract omits the `warnings` field the same section calls mandatory

**File:** `docs/api-contract.md:646-652`

**Issue:** The prose immediately below says *"`warnings` is always present and is
`[]` when there is nothing to say"*, and the ordinary example at line 620-627 shows
it. The SecureBoot example object stops at `installer`. A client author copying the
SecureBoot shape learns the wrong contract.

**Fix:** Add `"warnings": []` to the example at line 651.

### IN-03: The contract describes the provisional outcome more narrowly than the code produces it

**File:** `docs/api-contract.md:679-681`

**Issue:** Outcome 2 reads *"a candidate failed at the transport level and a later
one answered"*. `resolveInstallerRepo`'s `default` branch (`installer.go:428-432`)
also files a candidate as unresolved when it answers `401`, `403`, `429` or any
`5xx` — an HTTP answer, not a transport failure. Those produce the same provisional
entry and the same warning. The table added at lines 708-714 gets this right; the
outcome list contradicts it.

**Fix:** Reword to "a candidate did not answer the question — a transport failure,
or any of the statuses the table below files as `upstream.factory-unavailable` —
and a later one answered".

### IN-04: `WARNING_INSTALLER_REPO_FALLBACK_UNVERIFIED` is exported but unused by application code

**File:** `web/src/api.ts:283`

**Issue:** The constant is referenced only by `web/src/routes/images.test.tsx:9,764`.
No component keys on it: `AssetPanel` (`images.tsx:1123-1132`) passes
`assets.data.warnings` straight through with a hardcoded heading, so the `installer.`
family is distinguished by *where* it is rendered rather than by its code. That is a
workable design, but it means the exported constant is a test fixture wearing an
API's clothes, and the doc comment above it implies a keying that does not happen.

**Fix:** Either key the heading on the code inside `SchematicWarnings` (which would
also make a mixed-family list render correctly), or note in the comment that the
constant exists for tests and contract documentation only.

### IN-05: Two small dead/silent branches in the images route

**File:** `web/src/routes/images.tsx:100`, `web/src/routes/images.tsx:579`

**Issue:**
- Line 100: `character.codePointAt(0) ?? 0` — `for...of` never yields an empty
  string, so the `?? 0` branch is unreachable. Harmless, but it reads as if an
  undefined code point were meaningful, and `0` would be classified as a control
  character.
- Line 579: `key: Number(event.target.value)` — clearing the META key box yields
  `Number('') === 0`, so the row silently becomes key `0` rather than staying
  empty or reporting. The controlled `value={row.key}` then re-renders it as `0`,
  so it is visible, but the operator did not choose it.

**Fix:** Drop the `?? 0` (or assert with a non-null assertion), and guard the key
parse so an empty box does not resolve to a valid META slot:

```ts
const parsed = event.target.value === '' ? Number.NaN : Number(event.target.value)
next[index] = { ...row, key: Number.isNaN(parsed) ? row.key : parsed }
```

---

_Reviewed: 2026-08-29_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
