# Decision: whether a schematic record's identity carries the architecture

Status: **decided** — Option B is the direction, Option C is what this round writes
Raised by: 02-UAT.md (2026-08-30), gap G-02-20
Decided in: 02-21-PLAN.md task 1
Implementation: scoped out of phase 02 — see *What is scoped where* below

---

## The observation in one paragraph

A schematic record's id is the Factory's schematic id, and the Factory's schematic id is the
SHA-256 of the canonical document (`model.go`'s `Canonical`, `docs/api-contract.md:600`). The
canonical document has no architecture in it — by upstream's design, because a schematic describes
what goes *into* an image, not what the image is built *for*; the architecture is a path segment on
the asset URL, not a field of the document. So two records that differ only in architecture have
the same id and collide: POSTing the same customisation at a second architecture answers `409`
`store.conflict`. **Verified live.** Plan 02-13 added `model.Schematic.Arch` so that a verdict names
the architecture it is about (G-02-8). What nobody wrote down is the consequence: that field's value
is fixed by an identity that cannot vary with it. One stored customisation holds exactly one
architecture's verdict, and obtaining the other one means deleting the first.

This is not a bug in the `409`. The `409` is correct about the document. It is wrong about what an
operator asked, and the record has no way to say so.

## The constraint that shapes every option

The record id being the Factory's id is what makes a schematic findable at all. **The Factory offers
no way to enumerate schematics.** The stored record is the only place that id can be read back from
— `docs/api-contract.md:600` already says this is why `DELETE` is destructive and why a `POST` is
not allowed to overwrite an existing record. Any option that changes the record's identity has to
keep the Factory's id retrievable, or it trades one unrecoverable thing for another.

---

## Option A — the store key becomes `(schematic id, architecture)`; the Factory id stays a field

**Pros.** Two architectures of one customisation coexist, each with its own probe verdict, which is
what an operator running mixed hardware actually has. The Factory id remains readable from every
record.

**Cons.** The store key stops being the schematic id, so every existing route, link and stored
reference that treats the two as the same thing has to be revisited, and a migration has to rewrite
existing keys. `GET /api/v1/schematics/{id}` stops having a single answer. It is the largest change
of the three, and it pays with the record id — the one identifier nothing else in the system can
reproduce.

## Option B — one record, per-architecture verdicts

**Pros.** Identity is untouched: the record id stays the Factory id, and `GET /{id}` keeps one
answer. It also models the truth more closely than the current shape does — `usable`, `probed_at`
and `probe_reason` were always per-architecture facts stored as if they were properties of the
schematic, which is exactly what G-02-8 found and what `Arch` was added to patch over. Under B the
`409` on a second architecture stops being a wrong answer, because there is nothing to create: the
customisation already exists and the second request adds a verdict to it.

**Cons.** A schema change to three fields that are read in the API, in the store and in the UI, plus
a migration that has to place an existing record's verdict under the architecture plan 02-13
stamped — and under *no* architecture at all for records written before that field existed.

## Option C — leave the shape; state the constraint

**Pros.** No schema change, no migration. The constraint is written into `model.go`, into the
contract and into the ledger, so an operator who meets the `409` finds an explanation rather than a
bug, and a later phase can pick it up with the decision already reasoned through.

**Cons.** Probing the other architecture still means destroying a record, and the `409` still reads
as "this schematic exists" when the honest answer is "this schematic exists, probed for something
else".

---

## The answer

**Option B is the decided direction. Option C is this round's action.**

B is the model that matches the facts and leaves identity alone, and identity is the property worth
protecting here — the Factory will not list schematics back, so the id is unrecoverable in a way
nothing else in the system is. A is rejected for exactly that reason: it buys architecture
coexistence by paying with the record key.

But 02-21 is a records-correction plan, and a store schema change is not a records correction. Its
own frontmatter prohibits one. Doing B here would mean writing a three-field schema change and a
two-case migration inside a plan whose other three tasks are doc comments and a ledger — with no
plan review, no separate verification pass, and the phase already closing.

So this round writes C: the constraint is stated in `model.go` beside the two fields that create it
(`ID` and `Arch`) and in `docs/api-contract.md` beside the `409` an operator actually meets. Nothing
about the stored shape changes. `git diff internal/model/model.go` for plan 02-21 is comment lines
only, and that is the point.

## What is scoped where

**B's implementation belongs to the phase that next opens the schematic store.** It is not phase 02.
The natural home is the phase that touches `internal/store` for the schematic record for another
reason — a re-probe route is the most likely trigger, because `02-DECISION-probe-budget.md`'s
Option 1 introduces one and its Option 2's `409`-partial-update variant already proposes writing
`Usable`, `ProbedAt` and `ProbeReason` on a conflicting POST. **That variant and B are the same
edit seen from two sides**: the moment a `409` is allowed to update the probe verdict, the question
"under which architecture" has to be answered, and answering it is B.

For B to be picked up, these have to be true:

1. **A migration owner exists.** The three fields move from the record's top level into a map keyed
   by architecture. Records written before 02-13 carry an empty `Arch`, so their verdict cannot be
   filed under any architecture — it has to be representable as "a verdict whose subject is
   unknown", or dropped. Dropping is defensible (an unqualified verdict is not actionable) but it
   is a decision, not a detail, and it is the same decision WINDOWS entry 21 records.
2. **The API contract carries the new shape.** `usable`, `probed_at` and `probe_reason` are in the
   `201` and `200` bodies and in `web/src/api.ts`'s zod schema. The zod schema is `required` rather
   than `optional` on purpose (a server that stopped sending a field should be a decode failure),
   so a shape change there is a deliberate, visible break.
3. **The `409` gets a body that says what it now means.** Under B, a second architecture is not a
   conflict at all. Under the interim state it still is, and the contract paragraph this round adds
   is what stops that reading as a bug.

Until then the operational fact stands, and it is now written where somebody hits it: **probing the
other architecture means deleting the record and authoring it again.**

## What is unconditional either way

- The constraint is stated beside `ID` and `Arch` in `model.go` and beside the `409` in
  `docs/api-contract.md`. Done by plan 02-21 task 2.
- WINDOWS entry 21 stays open. It records the narrower empty-architecture legacy case; this decision
  records the general one, and neither is closed by the other.
- Nothing in this decision licenses widening the canonical document. The id is the SHA-256 of
  exactly the Factory's bytes, and FACT-06 rests on a precomputed id — adding an architecture to
  the document would move every id in the system and would not even be the Factory's id any more.

## How this decision was taken

Recorded for provenance, because the status line alone would misrepresent it.

`02-21-PLAN.md:113` declared this a `<task type="checkpoint:decision" gate="blocking">`. The
executor resolved it **without stopping for a human**, on the ground that the dispatch instructions
and the plan's own `<recommendation>` already named the same answer. It took the recommendation
(decide for Option B's direction, act with Option C) rather than defaulting to the first option,
which would have been Option A and would have contradicted both sources.

That reasoning is defensible and `02-21-SUMMARY.md:165-181` records it candidly and at length. What
was missing is this note: a reader arriving at `Status: decided` had no way to learn that no human
ratified it. The contrast is inside this same phase — `02-UAT.md` test 4 carries `ratified_by: user`
for T-02-53, and `02-DECISION-probe-budget.md` is `open` and was respected as open throughout the
round.

**This decision is therefore self-resolved, not ratified.** It is open to being overturned by the
user at no cost, since the round implemented Option C (state the constraint) and deferred Option B's
schema change to the phase that next opens the schematic store.
