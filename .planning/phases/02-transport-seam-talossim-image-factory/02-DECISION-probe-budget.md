# Decision needed: how the build probe gets a budget it can actually meet

Status: open — blocks gap-closure planning for G-02-1, G-02-2, G-02-9
Raised by: 02-UAT.md (2026-08-29), UAT test 1
Decide before: `/gsd-plan-phase --gaps` is run for cluster A

---

## The problem in one paragraph

`ProbeBuildable` HEADs the ISO URL, which makes the Image Factory build a ~335MB image
synchronously. Measured cold against the live Factory, five times across two independent
investigators: 30.50, 30.59, 31.18, 31.52, 32.69 seconds. The budget governing it is
`imagefactory.DefaultTimeout = 30s` (`internal/imagefactory/client.go:28`), a value whose own
doc comment sizes it against "the three JSON endpoints this package calls — the largest answer is
the extension catalog at a few tens of kilobytes". The probe therefore aborts one to three seconds
before the answer arrives, every time, for any schematic the Factory has not built before.

Raising that budget is not a one-line change, because `POST /api/v1/schematics` already makes
three sequential Factory calls under one request — `Extensions`, `CreateSchematic`,
`ProbeBuildable` — for a 90–120s worst case against `writeTimeout = 60s`
(`cmd/holzkubed/main.go:44`). `GET /schematics/{id}/assets` has the same shape at 2 × 30s = exactly
60s. **The per-request budgets and the response budget were never composed against each other**,
and there is no headroom left to spend.

## Why this is a decision and not a bug fix

The two candidate answers differ in what they cost and in what they change about the API contract.
A planner asked to "fix the timeout" would pick one silently. That choice should be recorded.

---

## Option 1 — Split the create route

`POST /api/v1/schematics` answers `201` as soon as the Factory has assigned the id, with
`usable: false` and an explicit "not yet probed" state. The probe becomes a follow-up the UI drives
against a new route.

**What it fixes.** All three gaps at once. Create stops being a 31-second wait with no feedback
(G-02-1's UI half, and the deferred progress-indication item). The 90–120s worst case leaves the
60s response budget entirely (G-02-2). The follow-up route *is* the re-probe path, so a probe that
times out is retryable rather than permanent, and the sudo-gated destructive delete stops being the
only recovery (G-02-9).

**What it costs.**
- A new route in `SchematicRoutes` and in `docs/api-contract.md` §522-526. Its `Destructive`
  classification needs deciding: it mutates a stored record, but only the three probe fields, and it
  reaches no node. `Destructive: false` looks right, and should be argued in the contract rather
  than assumed.
- A third probe state in `model.Schematic`. Today `ProbedAt` zero means "no verdict" and conflates
  "never attempted" with "attempted and timed out" — a conflation the UAT caught as a copy bug: the
  badge reads "the build probe did not run" when the probe ran for a full 30 seconds and gave up
  (`web/src/routes/images.tsx:552-554`). An additive JSON field is backward compatible with existing
  records (missing field decodes to the zero value), so `internal/store/migrate` is not strictly
  required — but stamping a version may still be wanted for clarity.
- UI work: the Images screen has to drive the follow-up, show an in-progress state, and handle the
  probe failing separately from the create failing.
- The 201 body's `warnings` contract (`schematics.go:47`) is unaffected.

**What it risks.** A schematic can now exist in a state no current client understands. Anything
reading `usable` today must be re-read: `usable: false` currently means "created, verdict absent",
and after this change it also means "created, probe pending". The badge copy has to distinguish
three states rather than two.

---

## Option 2 — Keep create synchronous, buy headroom

Give manifest and ISO probes their own timeout constant, distinct from the JSON-endpoint
`DefaultTimeout`, sized just under what the response budget can carry. Compose the budgets
explicitly instead of letting them collide.

**What it fixes.** G-02-1 for the common case, if the probe's budget clears the observed 30.5–32.7s
band with margin. G-02-2 partially: a shared deadline across the two installer candidates
(`context.WithTimeout` in `schematicAssets` before `InstallerImage`, or a per-candidate deadline
derived from the remaining budget) makes the assets route fit. Probing candidates concurrently and
preferring the platform-prefixed 2xx collapses the cold case from 30+13s to ~30s and would also
close G-02-3's silent-fallback window for free.

**What it costs.** Much less. No new route, no contract change, no new probe state — though the
badge copy fix (G-02-1) is still needed regardless of which option is chosen. `writeTimeout` may
have to rise, and its comment currently reasons only about argon2id and the login rate limiter; it
would need to name the upstream budgets it must cover.

**What it does not fix.** G-02-9 stays open: a probe that times out anyway is still permanent, and
the only recovery is still delete-and-reauthor. The partial remedy is to stop *throwing away* the
verdict the 409 path already computes — a re-POST of the same customisation runs the full `Author`
sequence including a fresh, usually successful probe, and then discards it
(`schematics.go:266-287`). The comment there justifies refusing to overwrite the label, cluster and
version, which is sound; that reasoning does not extend to the probe verdict. Updating only
`Usable`, `ProbedAt` and `ProbeReason` on conflict preserves the stated invariant and gives
operators a non-destructive recovery without a new route.

**What it risks.** The create wait stays ~31s with a disabled button as its entire feedback surface,
which `.planning/research/PITFALLS.md:164` already rules out ("Elapsed time, current sub-step,
expected duration") and `:646` names as the anti-pattern. The margin also stays latency-dependent:
the Factory's build time is not ours to bound, and today's 2–3s shortfall could become 20s.

---

## Recommendation

**Option 2 now, Option 1 when phase 3 forces the question anyway.**

Option 2 closes the measured failure with no contract change, and its 409-partial-update variant
gives G-02-9 a real answer for a fraction of the cost. Option 1 is the structurally correct end
state, but it introduces a probe state and a route at the moment the phase is trying to close, and
phase 3 will revisit this surface regardless once the transport seam has a production caller.

If Option 2 is chosen, G-02-9 should be recorded in `WINDOWS.md` as a known window — "a probe that
times out despite the raised budget is still permanent" — rather than treated as closed.

## What is not in question either way

These are unconditional and should be planned regardless of the choice above:

- The badge copy must stop claiming the probe "did not run" when it ran and timed out.
- The `DefaultTimeout` doc comment must stop justifying a value against a workload it does not
  govern.
- A composition guard test — `(max sequential upstream calls per route) × (client timeout) + slack <
  writeTimeout`, as a table over routes — is the assertion whose absence let `2 × 30 == 60` ship.
- The test fakes are zero-latency (`schematics_test.go` `fakeFactory`), which makes every failure
  above structurally unrepresentable. `internal/imagefactory/live_test.go:96` already pays the cold
  cost but asserts only `err == nil` and never bounds elapsed time.
