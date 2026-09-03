---
phase: 02-transport-seam-talossim-image-factory
fixed_at: 2026-08-29T20:26:00Z
review_path: .planning/phases/02-transport-seam-talossim-image-factory/02-REVIEW.md
iteration: 1
findings_in_scope: 5
fixed: 5
skipped: 0
status: all_fixed
---

# Phase 02: Code Review Fix Report

**Fixed at:** 2026-08-29
**Source review:** `.planning/phases/02-transport-seam-talossim-image-factory/02-REVIEW.md`
**Iteration:** 1
**Scope as chosen by the user:** CR-01, WR-01, WR-03, WR-04, WR-05 — WR-02, WR-06 and every
INFO finding were left untouched for the ledger.

> **Correction, added after an adversarial review pass.** When this report was written, that
> last clause was a promise rather than a fact: the seven scoped-out findings were not on
> `.planning/WINDOWS.md`, so the ship gate could not see them. They are now, as ledger ids
> 22-28 (`[R2-WR-02]`, `[R2-WR-06]`, `[R2-IN-01]` … `[R2-IN-05]`), tagged `R2-` to keep them
> apart from the round-1 findings that occupy ids 8-18 under the same short names. Id 29
> records a residual risk neither review round filed: a lone surrogate submitted through the
> API rather than the browser is still silently rewritten to U+FFFD before the schematic id is
> computed.
>
> The same pass found that this report's CR-01 section overstates what the fix achieves. See
> `internal/imagefactory/installer.go`'s `storeInstallerRepo` doc comment, corrected in
> `f90de22`: the guard makes the *cache* converge in every ordering, but two concurrent callers
> can still be served two different references in the mirror ordering. That divergence is
> disclosed by the fallback warning rather than silent, which is why it was judged a comment
> defect and not a code defect — but the claim as originally written here was wrong.

**Summary:**
- Findings in scope: 5
- Fixed: 5
- Skipped: 0

**Where verification ran:** the main checkout (`workflow.use_worktrees` is `false` in
`.planning/config.json`, so no worktree was created and every gate ran against the tree the
reader is looking at). All gates were run after each fix and again at the end.

## Fixed Issues

### CR-01: Concurrent re-question silently reverts a proven installer repository to the provisional one

**Files modified:** `internal/imagefactory/installer.go`, `internal/imagefactory/installer_test.go`, `internal/imagefactory/fake_test.go`
**Commit:** `49be02f`

**Applied fix:** Both write sites (`installerRepo`'s cold path and `requestionInstallerRepo`)
now go through a new `storeInstallerRepo(key, next)`, which takes the lock, re-reads the entry
and **refuses to overwrite a proven one**, returning that entry to the caller that lost the race
so two concurrent callers are handed the same reference rather than two.

**Deviation from the review's suggested patch, deliberate.** The review proposed a
compare-and-set on `entry.at` (`!current.proven() && current.at.Equal(entry.at)`). That snippet
has a failure mode of its own: if a concurrent *failed* re-question re-stamps the entry first, the
goroutine that has just **proven** the preferred name finds `current.at != entry.at`, refuses to
write, and adopts the provisional entry — discarding the only observation either goroutine made
that ends the question. The proven-guard alone is sufficient and does not have that hole. Among
unproven entries last-write-wins is harmless: on one key they necessarily carry the same
repository name (candidate order is fixed, first 2xx wins), so all a later write moves is the
re-question cadence, which is what it is meant to move. This reasoning is written into the
function's doc comment so the next reader does not "fix" it back.

**The absent test now exists.** `TestInstallerImageNeverRevertsAProvenNameUnderConcurrentResolution`
covers both write sites as subtests (`cold cache`, `stale provisional entry`). The interleaving is
**forced, not hoped for**: a new fake knob (`setRepoSilentForNext`) hands the silence to whichever
request arrives first and delays it, so the goroutine holding the stale observation always writes
last. Verified by reverting `installer.go` to its pre-fix state — the test then fails with exactly
CR-01's symptom on both subtests (`127.0.0.1/installer/...` vs `127.0.0.1/metal-installer/...`,
cache left holding `installer` as provisional). Passes with the fix under `-race -count=3`.

**Constraints honoured:** no timeout, deadline or budget value changed; the re-question still asks
only the never-ruled-out candidate — the new concurrent case carries its own counter assertion
pinning that (`installer` asked exactly once); the fallback is still a warning and never an error.

### WR-01: A cancelled request is recorded as "the registry was silent again" and re-stamps the cache entry

**Files modified:** `internal/imagefactory/installer.go`, `internal/imagefactory/installer_test.go`
**Commit:** `be75d5e`

**Applied fix:** The `default` branch of `requestionInstallerRepo` returns the entry untouched when
the caller's context is done — nothing learned, nothing re-stamped, and (as the review noted) no
map write, which removes one of CR-01's write sites. The retained answer is still served with its
warning, so a cancelled caller does not become a 502 for anyone.

**One deliberate refinement of the suggested patch:** the discriminator is `ctx.Err() != nil` alone,
not `errors.Is(err, context.Canceled)`. This client's own budget is `http.Client.Timeout`, whose
expiry *also* satisfies `errors.Is(err, context.DeadlineExceeded)` — and that expiry is a genuine
observation of a silent registry which must keep re-stamping. Only the inbound context tells the
two apart, so only the inbound context is consulted.

New test `TestInstallerImageDoesNotReStampAReQuestionItsCallerAbandoned` asserts the timestamp is
unmoved, the registry was not asked at all (request counter unchanged), and the retained reference
and its warning are still served. It fails against the pre-fix code.

### WR-03: `predictWarnings` does not implement the server's predicate, despite claiming to

**Files modified:** `web/src/components/SchematicWarnings.tsx`, `web/src/routes/images.test.tsx`
**Commit:** `2ee117b`

**Applied fix:** `predictWarnings` now states the server's rule and only that —
`kernelArgs.length > 0` / `meta.length > 0`, matching `warnings.go:89,100`. The blank-row filter
moved to `LiveSchematicWarnings`, on exactly the terms `submit` already uses, so the live panel
predicts on what would actually be sent and adding an empty row still warns about nothing. One
predicate, in the place each fact belongs to — the review's "do not leave one function claiming to
be both" is satisfied without forking the component.

Two tests added: a just-added empty row stays quiet, and a stored record carrying blank entries
(the API-authored case) warns in the detail panel. The second fails against the pre-fix code.
The Go drift guard `TestWarningDetailsMatchTheUI` still passes — the three pinned sentences were
not touched.

### WR-04: A lone surrogate is silently rewritten rather than refused, contrary to the stated contract

**Files modified:** `web/src/routes/images.tsx`, `web/src/routes/images.test.tsx`
**Commit:** `ec10e08`

**Applied fix:** Took the review's first option — the client check now carries **both** halves of
`representable`'s rule (`code >= 0xd800 && code <= 0xdfff` alongside the control-character test),
and the comment states the real mechanism instead of a guarantee that does not exist:
`JSON.stringify` emits `\udXXX`, Go's `encoding/json` decodes an unpaired surrogate escape to
U+FFFD, so the server never sees anything to refuse and `representable`'s `utf8.ValidString`
branch is unreachable from the HTTP route.

**The server was deliberately left alone**, per the binding constraint: widening `representable`'s
refused set would change which scalars the canonical serialiser renders, and that decides the
locally precomputed schematic id FACT-06 rests on. The comment now says that explicitly.

The row message gained the new class (`"a control character or an unpaired surrogate — half of a
character whose other half never arrived"`) and still names a class, never the value (T-02-64).
Tests pin both directions: unpaired surrogates refused, a well-formed pair (an emoji) still
accepted, U+0080 still accepted. Fails against the pre-fix code.

### WR-05: `TestWarningsCodesAreNamespaced` still has the defect its own comment says it fixed

**Files modified:** `internal/imagefactory/warnings_test.go`
**Commit:** `37d40b2`

**Applied fix:** Took the review's preferred option rather than deleting the sentence. A new
`exportedWarningCodes` helper parses the package's own non-test source with `go/ast` — the whole
directory, not just `warnings.go`, since a guard that only looks where the constants live today
misses the one added elsewhere — and the test iterates what it found. Parsing rather than grepping
because "Warning" appears in doc comments and in this file's own expectations.

The vacuous-pass hole is closed too: a scan that matched nothing would silently pass, which is the
same defect in a third coat, so the three known constants are asserted present by
compiler-checked reference before any prefix is examined, and a value that is not a string literal
fails rather than being skipped.

**Verified by construction:** declaring a fourth exported `Warning...` constant with no family
prefix **in a different file** (`probe.go`) makes the guard name it and fail; the file was then
restored byte-for-byte.

## Verification

Run in the main checkout after the last commit, with a clean working tree:

- `go test ./... -count=1 -race` — all packages ok
- `golangci-lint run ./...` — 0 issues
- `npm --prefix web run test -- --run` — 108 passed (7 files)
- `npm --prefix web run lint` — clean
- `tsc -p web/tsconfig.json --noEmit` — clean (invoked directly; `npm exec tsc -- --noEmit`
  swallows the flag with the installed tsc 7.0.2 and prints its help instead)
- `go test ./cmd/holzkubed/ -run Budget` — `TestRouteBudgetsComposeAgainstWriteTimeout` and
  `TestRouteBudgetTableReadsTheRealConstants` both pass **without any budget being raised**

Constraint audit over `git diff e2db392..HEAD`:

- `cmd/holzkubed/main.go` — 0 lines of diff, byte-identical
- no `DefaultTimeout`, `writeTimeout`, `installerRepoRetryInterval` or `http.Client.Timeout` value
  changed anywhere (the only matches in the diff are comments and the pre-existing test helper
  `newClientWithRetryInterval(t, url, 0)`)
- `KERNEL_ARGS_DETAIL`, `META_DETAIL` and `DEFAULT_HEADING` in `SchematicWarnings.tsx` unchanged
- no new problem-body or warning text interpolates a kernel-arg or META value

## Flagged for human confirmation

Both Go fixes change concurrency/error-classification semantics, so although each carries a
regression test that fails against the pre-fix code, the *rule* chosen is a judgement call worth a
second pair of eyes:

- **CR-01** — the never-demote-proven rule was chosen over the review's timestamp CAS, for the
  reason given above. If a future change makes two *unproven* entries on one key able to hold
  different repository names, last-write-wins among them would need revisiting.
- **WR-01** — `ctx.Err()` as the sole discriminator means a genuine transport failure that happens
  to coincide with a caller disconnecting is filed as "nothing learned" and re-questioned on the
  next request. That is one extra probe in a rare race, chosen over the alternative of resetting a
  five-minute cadence on a question nobody asked.

Neither is a known defect; both are recorded so the choice is deliberate rather than inherited.

## Left alone, as instructed

WR-02 (no single-flight — noted in `storeInstallerRepo`'s doc comment as a separate question about
load rather than correctness), WR-06, and IN-01 through IN-05.

---

_Fixed: 2026-08-29_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
