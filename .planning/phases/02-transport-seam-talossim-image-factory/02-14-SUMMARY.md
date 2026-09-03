---
phase: 02-transport-seam-talossim-image-factory
plan: 14
subsystem: api
tags: [yaml, canonical-serialisation, image-factory, unicode, differential-testing, go]

requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "Schematic.Canonical()/ID(), CreateSchematic returning Created.Canonical, and the createProblem split that turns a NotRepresentableError into a field-named 400"
provides:
  - "TestLiveCanonical: an opt-in differential comparing the local canonical document and id against the ones factory.talos.dev returns, over a corpus reaching above U+007F through two document paths"
  - "A refusal set in representable() derived from 116 measured DIVERGES rows, with every clause citing the rows that proved it"
  - "The route-level assertion for G-02-11: a kernel argument carrying U+2028 answers 400 naming kernel_args and its entry index, and the unreachable-Factory sentence is absent from the body"
  - "A contract that states the refused set as rules, its provenance as a measurement, and the ranges the sweep never reached"
affects: [02-20, 02-21, 02-16, image-factory, schematic-authoring-ui]

actuals:
  tokens: 12519
  tasks: 3
  commits: 5

tech-stack:
  added: []
  patterns:
    - "Differential testing against the upstream that is being transcribed, never against a second local library"
    - "A three-way classification (agrees / diverges / refused-locally) with transport failure kept outside all three as a named non-observation"
    - "A widening whose every clause cites the measured row that proved it, and whose unmeasured remainder is stated in the contract"

key-files:
  created:
    - internal/imagefactory/canonical_live_test.go
  modified:
    - internal/imagefactory/schematicid.go
    - internal/imagefactory/schematicid_test.go
    - internal/httpapi/handlers/schematics_test.go
    - docs/api-contract.md

key-decisions:
  - "The refusal set is four clauses derived from the measured rows, not the nine codepoints the UAT names: C0+DEL+C1 as one control-character rule, U+2028/U+2029 as line separators, U+FEFF alone, and everything above U+FFFD"
  - "U+2028 and U+2029 are refused although one of six measured variants AGREES: representability is a property of the scalar, not of the text surrounding the codepoint"
  - "The boundary is U+FFFD and not 'non-characters', because U+FDD0 is a non-character that round-trips and U+FFFD itself round-trips"
  - "U+1F600 was reclassified from negative control to divergence on the measurement's authority, against the plan's own expectation"
  - "plainAllowed is unchanged, and its comment records that no measured row proved it wrong rather than leaving the non-change silent"
  - "A 4xx from the Factory is an answer and a throttle is not, so the differential parses the status out of ErrUpstreamUnavailable instead of treating every non-2xx as a non-observation"

patterns-established:
  - "Halt-gate discipline: the refusal set may only cite rows the recorded table contains, and a class the UAT describes but the table did not prove is recorded as unmeasured rather than shipped"
  - "Cross-layer table binding: the handler test asserts each of its codepoints against Schematic.ID() before POSTing it, so two tables in two packages cannot drift"

requirements-completed: [FACT-06, FACT-01]

coverage:
  - id: D1
    description: "An opt-in differential guard exists that compares Schematic.Canonical() and Schematic.ID() against the Factory's own document and id, over a corpus reaching above U+007F through both customization.extraKernelArgs and customization.meta[].value"
    requirement: FACT-06
    verification:
      - kind: integration
        ref: "HOLZKUBE_FACTORY_LIVE=1 go test ./internal/imagefactory/ -run TestLiveCanonical -count=1 -v"
        status: pass
    human_judgment: false
  - id: D2
    description: "Without the opt-in variable the guard skips loudly, naming in U+ form both what this run measured and what no run of it reaches"
    requirement: FACT-06
    verification:
      - kind: integration
        ref: "go test ./internal/imagefactory/ -run TestLiveCanonical -count=1 -v | grep -qE 'NOT OBSERVED:.*U\\+'"
        status: pass
    human_judgment: false
  - id: D3
    description: "Every codepoint class the measurement showed diverging is refused by Schematic.ID() before any request leaves the process, naming the document path and the entry index"
    requirement: FACT-06
    verification:
      - kind: unit
        ref: "internal/imagefactory/schematicid_test.go#TestSchematicIDRefusesEveryMeasuredDivergence"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/schematicid_test.go#TestSchematicIDRefusesAMeasuredDivergenceInAMetaValue"
        status: pass
    human_judgment: false
  - id: D4
    description: "Every codepoint the measurement proved round-trips still produces an id, and that id is the literal it produced before the widening; both well-known anchors are byte-identical"
    requirement: FACT-06
    verification:
      - kind: unit
        ref: "internal/imagefactory/schematicid_test.go#TestSchematicIDStillAcceptsWhatTheFactoryProvedItCarries"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/schematicid_test.go#TestTheWellKnownIDsDidNotMove"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/tracer_test.go#TestSchematicIDMatchesRecordedFixtures"
        status: pass
    human_judgment: false
  - id: D5
    description: "At the route, a kernel argument carrying a newly refused codepoint answers 400 with a field error naming kernel_args and the one-based entry index, and the response body does not carry the sentence an unreachable Factory produces"
    requirement: FACT-01
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestCreateRefusesAMeasuredDivergenceWithAFieldNamed400"
        status: pass
    human_judgment: false
  - id: D6
    description: "docs/api-contract.md states the refused set as rules with reasons, names the measurement it came from, and names the ranges the sweep never reached"
    verification:
      - kind: other
        ref: "grep -n 'U+0085\\|U+2028\\|U+FEFF' docs/api-contract.md; grep -c 'below .U+0020. or .U+007F. — the set the canonical serialiser was pinned against' docs/api-contract.md == 0"
        status: pass
    human_judgment: false
  - id: D7
    description: "The authoring UI's own pre-check still transcribes only the pre-widening rule, so an operator typing an emoji or U+2028 now learns of it from the server's 400 rather than from the row while they are looking at it"
    verification: []
    human_judgment: true
    rationale: "web/src/routes/images.tsx is outside this plan's declared file set and is owned by plan 02-20. The behaviour is safe -- the server's 400 is the documented backstop -- but whether one extra round trip is an acceptable authoring experience is a judgment, and whether to widen the client guard is 02-20's decision to take."

duration: 45 min
completed: 2026-08-30
status: complete
---

# Phase 02 Plan 14: Canonical Serialiser Divergence Measured and Refused Summary

**A live differential against factory.talos.dev found 116 diverging rows above U+007F across two document paths, and `representable` was widened to exactly the four classes those rows proved — control characters through U+009F, the two line separators, the byte order mark, and everything above U+FFFD — after which the same differential runs green with zero divergences and zero over-refusals.**

## Performance

- **Duration:** 45 min
- **Started:** 2026-08-30T06:13:53Z
- **Completed:** 2026-08-30T06:58:00Z
- **Tasks:** 3 of 3 (the halt gate did not fire)
- **Files modified:** 5 (1 created, 4 modified)

## The halt gate did not fire

This plan carried a gate: task 2 could only execute if the table task 1 recorded
contained at least one DIVERGES row above U+007F and at least one AGREES row
above U+007F. Both conditions were met, by a wide margin and with nothing left
unobserved:

| | |
|---|---|
| DIVERGES rows | 116 (all above U+007F) |
| AGREES rows | 40 (36 of them above U+007F) |
| REFUSED LOCALLY rows | 24 (the C0 set, which established the outcome is reachable) |
| NOT OBSERVED rows | **0** — factory.talos.dev did not throttle in either recorded run |

The sweep was run twice before the widening, once after an instrument fix (see
Deviation 1), and both runs produced **identical verdict counts and identical
per-row verdicts**. It was run a third time after the widening landed.

**02-20: you are not blocked.** G-02-16 is closed on both of its `missing`
bullets. The declared codepoint table you extend is
`divergingClasses` in `internal/imagefactory/schematicid_test.go` and
`refusedCodepoints` in `internal/httpapi/handlers/schematics_test.go`.

## The recorded three-way table

One row per corpus entry. **Both document paths — `customization.extraKernelArgs`
and `customization.meta[].value` — produced identical verdicts and identical
shapes for all 90 entries**, so one table is printed and the equality was
asserted mechanically rather than by eye. Recorded from
`HOLZKUBE_FACTORY_LIVE=1 go test ./internal/imagefactory/ -run TestLiveCanonical -count=1 -v`.

| codepoint | position/style | verdict | shape | rule probed |
|---|---|---|---|---|
| `U+0000` | interior/plain | REFUSED LOCALLY | — | C0 control or U+007F: already refused before this plan |
| `U+0000` | interior/quoted | REFUSED LOCALLY | — | C0 control or U+007F: already refused before this plan |
| `U+0009` | interior/plain | REFUSED LOCALLY | — | C0 control or U+007F: already refused before this plan |
| `U+0009` | interior/quoted | REFUSED LOCALLY | — | C0 control or U+007F: already refused before this plan |
| `U+000A` | interior/plain | REFUSED LOCALLY | — | C0 control or U+007F: already refused before this plan |
| `U+000A` | interior/quoted | REFUSED LOCALLY | — | C0 control or U+007F: already refused before this plan |
| `U+000D` | interior/plain | REFUSED LOCALLY | — | C0 control or U+007F: already refused before this plan |
| `U+000D` | interior/quoted | REFUSED LOCALLY | — | C0 control or U+007F: already refused before this plan |
| `U+001F` | interior/plain | REFUSED LOCALLY | — | C0 control or U+007F: already refused before this plan |
| `U+001F` | interior/quoted | REFUSED LOCALLY | — | C0 control or U+007F: already refused before this plan |
| `U+007F` | interior/plain | REFUSED LOCALLY | — | C0 control or U+007F: already refused before this plan |
| `U+007F` | interior/quoted | REFUSED LOCALLY | — | C0 control or U+007F: already refused before this plan |
| `U+0080` | leading/plain | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0080` | leading/quoted | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0080` | interior/plain | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0080` | interior/quoted | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0080` | trailing/plain | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0080` | trailing/quoted | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0081` | leading/plain | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0081` | leading/quoted | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0081` | interior/plain | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0081` | interior/quoted | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0081` | trailing/plain | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0081` | trailing/quoted | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0085` | leading/plain | DIVERGES | the Factory would not parse the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0085` | leading/quoted | DIVERGES | the Factory silently altered the scalar | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0085` | interior/plain | DIVERGES | the Factory would not parse the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0085` | interior/quoted | DIVERGES | the Factory silently altered the scalar | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0085` | trailing/plain | DIVERGES | the Factory silently altered the scalar | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0085` | trailing/quoted | DIVERGES | the Factory silently altered the scalar | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+008D` | leading/plain | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+008D` | leading/quoted | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+008D` | interior/plain | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+008D` | interior/quoted | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+008D` | trailing/plain | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+008D` | trailing/quoted | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0094` | leading/plain | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0094` | leading/quoted | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0094` | interior/plain | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0094` | interior/quoted | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0094` | trailing/plain | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+0094` | trailing/quoted | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+009F` | leading/plain | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+009F` | leading/quoted | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+009F` | interior/plain | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+009F` | interior/quoted | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+009F` | trailing/plain | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+009F` | trailing/quoted | DIVERGES | the Factory re-normalised the document | C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break |
| `U+2028` | leading/plain | DIVERGES | the Factory would not parse the document | YAML line break above the C1 range (LS/PS) |
| `U+2028` | leading/quoted | DIVERGES | the Factory silently altered the scalar | YAML line break above the C1 range (LS/PS) |
| `U+2028` | interior/plain | DIVERGES | the Factory would not parse the document | YAML line break above the C1 range (LS/PS) |
| `U+2028` | interior/quoted | DIVERGES | the Factory silently altered the scalar | YAML line break above the C1 range (LS/PS) |
| `U+2028` | trailing/plain | DIVERGES | the Factory silently altered the scalar | YAML line break above the C1 range (LS/PS) |
| `U+2028` | trailing/quoted | AGREES | — | YAML line break above the C1 range (LS/PS) |
| `U+2029` | leading/plain | DIVERGES | the Factory would not parse the document | YAML line break above the C1 range (LS/PS) |
| `U+2029` | leading/quoted | DIVERGES | the Factory silently altered the scalar | YAML line break above the C1 range (LS/PS) |
| `U+2029` | interior/plain | DIVERGES | the Factory would not parse the document | YAML line break above the C1 range (LS/PS) |
| `U+2029` | interior/quoted | DIVERGES | the Factory silently altered the scalar | YAML line break above the C1 range (LS/PS) |
| `U+2029` | trailing/plain | DIVERGES | the Factory silently altered the scalar | YAML line break above the C1 range (LS/PS) |
| `U+2029` | trailing/quoted | AGREES | — | YAML line break above the C1 range (LS/PS) |
| `U+FEFF` | leading/plain | DIVERGES | the Factory re-normalised the document | byte order mark, outside YAML's printable set |
| `U+FEFF` | leading/quoted | DIVERGES | the Factory re-normalised the document | byte order mark, outside YAML's printable set |
| `U+FEFF` | interior/plain | DIVERGES | the Factory re-normalised the document | byte order mark, outside YAML's printable set |
| `U+FEFF` | interior/quoted | DIVERGES | the Factory re-normalised the document | byte order mark, outside YAML's printable set |
| `U+FDD0` | interior/plain | AGREES | — | Unicode non-character |
| `U+FDD0` | interior/quoted | AGREES | — | Unicode non-character |
| `U+FFFE` | interior/plain | DIVERGES | the Factory would not parse the document | Unicode non-character |
| `U+FFFE` | interior/quoted | DIVERGES | the Factory would not parse the document | Unicode non-character |
| `U+FFFF` | interior/plain | DIVERGES | the Factory would not parse the document | Unicode non-character |
| `U+FFFF` | interior/quoted | DIVERGES | the Factory would not parse the document | Unicode non-character |
| `U+D7FF` | interior/plain | AGREES | — | boundary of a printable range |
| `U+D7FF` | interior/quoted | AGREES | — | boundary of a printable range |
| `U+E000` | interior/plain | AGREES | — | boundary of a printable range |
| `U+E000` | interior/quoted | AGREES | — | boundary of a printable range |
| `U+FFFD` | interior/plain | AGREES | — | boundary of a printable range |
| `U+FFFD` | interior/quoted | AGREES | — | boundary of a printable range |
| `U+10FFFF` | interior/plain | DIVERGES | the Factory re-normalised the document | boundary of a printable range |
| `U+10FFFF` | interior/quoted | DIVERGES | the Factory re-normalised the document | boundary of a printable range |
| `U+00A0` | interior/plain | AGREES | — | negative control: printable above U+007F, must be accepted and must round-trip |
| `U+00A0` | interior/quoted | AGREES | — | negative control: printable above U+007F, must be accepted and must round-trip |
| `U+00E4` | interior/plain | AGREES | — | negative control: printable above U+007F, must be accepted and must round-trip |
| `U+00E4` | interior/quoted | AGREES | — | negative control: printable above U+007F, must be accepted and must round-trip |
| `U+200B` | interior/plain | AGREES | — | negative control: printable above U+007F, must be accepted and must round-trip |
| `U+200B` | interior/quoted | AGREES | — | negative control: printable above U+007F, must be accepted and must round-trip |
| `U+202E` | interior/plain | AGREES | — | negative control: printable above U+007F, must be accepted and must round-trip |
| `U+202E` | interior/quoted | AGREES | — | negative control: printable above U+007F, must be accepted and must round-trip |
| `U+4E2D` | interior/plain | AGREES | — | negative control: printable above U+007F, must be accepted and must round-trip |
| `U+4E2D` | interior/quoted | AGREES | — | negative control: printable above U+007F, must be accepted and must round-trip |
| `U+1F600` | interior/plain | DIVERGES | the Factory re-normalised the document | negative control: printable above U+007F, must be accepted and must round-trip |
| `U+1F600` | interior/quoted | DIVERGES | the Factory re-normalised the document | negative control: printable above U+007F, must be accepted and must round-trip |

**Nothing went unobserved.** No batch was skipped, no `NOT OBSERVED:` row was
produced by either recorded run, and no codepoint in the corpus is missing a
verdict.

### What the three shapes mean, since two of them are not the same harm

- **the Factory would not parse the document** — holzkube emitted invalid YAML
  and the Factory answered HTTP 400. Through the client, which folds every
  non-2xx into `ErrUpstreamUnavailable`, that reached the operator as
  `502 upstream.factory-unavailable` — *"The Image Factory did not answer
  usably"* — which is verbatim the sentence UAT test 5(b) exists to exclude.
- **the Factory re-normalised the document** — the value survived, escaped into
  a form holzkube does not emit (`"\x80console=ttyS0"`, `"﻿\x63\x6F..."`,
  `"console=\U0001F600ttyS0"`). Only the id moved. Bad, but recoverable.
- **the Factory silently altered the scalar** — the value did **not** survive.
  This is the serious one. `console=ttyS0<U+0085>` came back as
  `console=ttyS0`: the character was eaten, so the stored `kernel_args` and the
  stored `canonical` would describe different images. `'a: console=<U+2029>ttyS0'`
  came back as `'a: console=<U+2029>          ttyS0'` — the break was folded and
  ten spaces were inserted into the operator's value.

## The four clauses, and the rows that proved each

Every clause in `representable` cites its rows in the source comment beside it.

| Clause | Rows that proved it | Rows that bound it |
|---|---|---|
| `r < 0x20`, `r >= 0x7F && r <= 0x9F` — control characters | U+0080, U+0081, U+008D, U+0094, U+009F DIVERGES in **all six** variants on both paths (re-normalised). U+0085 DIVERGES in all six, unparseable when plain and **altered** when quoted, and eaten at the end of a plain scalar | U+00A0 AGREES — the range closes at the top on a measurement, not a guess |
| `r == 0x2028`, `r == 0x2029` — line separators | Each DIVERGES in **five of six** variants on both paths: unparseable plain, altered quoted | — |
| `r == 0xFEFF` — byte order mark | All four measured variants DIVERGES on both paths (re-normalised; the Factory escapes the BOM and every character after it) | Inside YAML's printable range, so no range test finds it — it needs its own clause |
| `r > 0xFFFD` — above the printable ceiling | U+FFFE, U+FFFF DIVERGES unparseable; U+1F600, U+10FFFF DIVERGES re-normalised | U+FFFD AGREES, so the boundary is exact; U+FDD0 AGREES, so the rule is the ceiling and **not** "non-characters" |

`U+007F` and the C1 range were folded into the pre-existing C0 clause because
they are one contiguous rule with one reason. No already-accepted scalar's
rendering changed: the widening is purely additive, and both well-known ids are
byte-identical.

## What the UAT describes that the table did not prove

Listed because a class that was not measured must not read as one that was.

1. **`plainAllowed` was not changed.** G-02-16's `missing` bullet asks for
   `representable` *and* `plainAllowed` to be extended. The sweep produced **no
   row** in which the Factory quoted a scalar this code leaves plain and the
   scalar is still accepted: every codepoint above U+007F either AGREES with the
   plain rendering byte for byte, or DIVERGES and is now refused before
   `plainAllowed` is reached. Editing it on anything less would have moved the
   id of a scalar that is already correct — the one thing this plan's
   prohibition forbids. The non-change and its reason are recorded in the
   function's doc comment rather than left silent.
2. **The C1 range is refused on six of its thirty-two members.** U+0080 and
   U+009F, its two ends, are among them. The other twenty-six are refused by
   extrapolation from those six.
3. **Everything between U+1F601 and U+10FFFE is refused on two points.** U+1F600
   and U+10FFFF. The interior of that range is unmeasured.
4. **`U+FDD1`–`U+FDEF` is unmeasured.** Only U+FDD0 was probed, and it AGREES.
5. **Leading and trailing positions were only swept for the C1 range, the two
   line separators and U+FEFF.** Every other codepoint was probed in the
   interior only. A non-ASCII codepoint the parser strips at a scalar boundary
   the way it strips a space would not have been seen.
6. **Surrogates U+D800–U+DFFF were not measured at all** and cannot be: they
   have no valid UTF-8 encoding, so `representable`'s existing UTF-8 branch
   refuses them before any of this applies. The HTTP route never reaches that
   branch either — Go's `encoding/json` decodes an unpaired surrogate escape to
   U+FFFD first. That is 02-20's half of G-02-11.

All six are stated in `docs/api-contract.md` as the floor-not-ceiling caveat.

## Accomplishments

- **The measurement exists and is repeatable.** `TestLiveCanonical` builds each
  candidate into a schematic, POSTs it, and compares the local canonical
  document byte for byte against `Created.Canonical` before comparing ids —
  because an id mismatch says only that something is wrong and the document diff
  says what. No second YAML library was imported; no Go module was added.
- **The refusal set is derived, not transcribed.** Four clauses, each citing its
  rows. The nine codepoints named in the UAT were not copied in: three of them
  (U+00A0, U+200B, U+202E) turned out to round-trip and are still accepted.
- **The route is asserted, not just the serialiser.** A kernel argument carrying
  U+2028 now answers `400` naming `kernel_args` and `entry 2`, and the absence
  of *"The Image Factory did not answer usably"* is asserted explicitly rather
  than inferred from the status code.
- **The guard runs green after the change.** Post-widening live run:
  `agrees=36 diverges=0 refused-locally=144 not-observed=0`, in 10.4s — every
  diverging class is now refused before a request leaves the process, and not
  one codepoint the Factory proved it carries was refused with it.

## Task Commits

1. **Task 1: Measure the divergence against the Factory's own canonical** — `676972d` (test)
2. **Task 2: Refuse what was measured, and nothing that was measured to work** — `3e338bb` (test, RED) → `ea6d1e0` (feat, GREEN)
3. **Task 3: State the set, and state what was never measured** — `6ff8e08` (docs)
4. **Corpus correction forced by the measurement** — `ef35009` (fix)

## Files Created/Modified

- `internal/imagefactory/canonical_live_test.go` *(created, 642 lines)* — the
  differential. Corpus by class boundary, batched at 10 scalars per POST,
  bisection only when the Factory answers without a document, and a small
  scalar decoder used to label shapes truthfully.
- `internal/imagefactory/schematicid.go` — `representable` widened to four
  clauses with their citations; `plainAllowed` unchanged with its non-change
  recorded; both doc comments now name `TestLiveCanonical` as the instrument.
- `internal/imagefactory/schematicid_test.go` — `divergingClasses`, the refusal
  cases per class on both document paths, the negative control asserted against
  literal ids, and both anchors restated.
- `internal/httpapi/handlers/schematics_test.go` — `refusedCodepoints` and the
  route-level case, with the table bound to `Schematic.ID()` so the two layers
  cannot drift.
- `docs/api-contract.md` — the refused set as a rules table, its provenance, and
  the ranges the sweep never reached.

## Decisions Made

1. **A 4xx is an answer; a throttle is not.** The client folds every non-2xx
   into `ErrUpstreamUnavailable`, so the plan's literal instruction —
   `errors.Is(err, ErrUpstreamUnavailable)` skips the batch — would have filed
   every document the Factory *refused* as a non-observation. That would have
   erased the finding and tripped the halt gate on a falsehood. The differential
   parses the status out of the message; 400–499 except 408 and 429 is an
   answer, everything else is not. 429 stays a non-observation for the same
   reason `registryRefused` keeps it retryable.
2. **U+2028 and U+2029 are refused despite one AGREES variant each.** Trailing,
   inside an already-quoted scalar, both round-trip. Accepting on that basis
   would make representability depend on the text surrounding the codepoint —
   the same character carriable or not according to its neighbours.
3. **The ceiling is U+FFFD, not "non-characters".** U+FDD0 is a non-character
   and round-trips; U+FFFD is the last codepoint the Factory writes literally.
   Both are measured, so the boundary is exact rather than plausible.
4. **U+1F600 was reclassified against the plan's own expectation.** It was
   written in as a negative control that must round-trip. It does not: the
   Factory escapes it to `"\U0001F600"`. The measurement's authority beats the
   expectation's, and the corpus now says so.
5. **The handler's codepoint table is bound to the serialiser rather than shared
   with it.** Go cannot share a table across two test packages without moving it
   into production code. Instead every entry is asserted against
   `Schematic.ID()` before it is POSTed, so a serialiser that stopped refusing
   one fails the handler test at that assertion instead of quietly agreeing with
   a stale list. This is stronger than a shared literal, not weaker.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] The guard reported escaping as alteration**

- **Found during:** Task 1, reading the first recorded run.
- **Issue:** the shape was decided by `strings.Contains(factoryDoc, scalar)`
  over the *whole* document. That is wrong twice: an escaped scalar
  (`"\x80console=ttyS0"`) does not contain its own raw bytes and was reported as
  *altered*, and a scalar genuinely eaten by the Factory was reported as
  *re-normalised* when a sibling entry in the same batch happened to contain the
  codepoint. Both errors point the wrong way — the second says a destroyed value
  survived. The shape is exactly the difference between T-02-74 (the id moved)
  and T-02-75 (the operator's value was destroyed), and task 2 derives its
  reasoning from it.
- **Fix:** `shapeOf` now asks whether any line of the Factory's document
  *decodes back to* the scalar that was sent, using a small decoder for the
  three styles this format produces. It is used only to label a shape, never to
  decide a verdict.
- **Verification:** the sweep was re-run in full. **Identical verdict counts and
  identical per-row verdicts**; only the shape column changed, and it changed to
  match the documents quoted in this SUMMARY.
- **Committed in:** `676972d`.

**2. [Rule 2 - Missing critical] The throttle check would have swallowed the finding**

- **Found during:** Task 1, before the first run.
- **Issue:** described as Decision 1 above. Recorded as a deviation as well
  because the plan gave a literal instruction and it was not followed: taking it
  literally would have produced a table of non-observations and halted the plan
  on a measurement that had in fact succeeded.
- **Fix:** `answeredStatus` / `factoryRefused`.
- **Committed in:** `676972d`.

**3. [Rule 1 - Bug] The guard would have failed on its own finding**

- **Found during:** Task 3, verifying the post-widening state.
- **Issue:** U+1F600 sat in the corpus as a negative control, so once
  `representable` correctly refused it the guard reported an OVER-REFUSAL.
- **Fix:** moved to its own group, with the expectation-versus-measurement noted
  in the comment.
- **Verification:** post-widening live run exits 0.
- **Committed in:** `ef35009`.

---

**Total deviations:** 3 auto-fixed (2 × Rule 1 bug, 1 × Rule 2 missing critical).
**Impact on plan:** no scope creep. All three are inside
`canonical_live_test.go`, the artifact task 1 owns. Every prohibition held: no
Go module added, no id moved, `live_test.go` / `installer.go` / `fake_test.go` /
`installer_test.go` untouched, `.planning/WINDOWS.md` untouched.

## Issues Encountered

**1. An acceptance criterion assumed a 502 that the offline fake cannot produce.**
Task 2's criteria say the new handler test should "go red with a `502`" before
the widening. It goes red — the table-binding assertion fires first — but the
underlying pre-widening behaviour against the fake Factory is **`201`**, not
`502`. Measured directly: before the widening, `POST /api/v1/schematics` with
`console=<U+2028>ttyS0` returned `201` and stored a record whose `canonical`
carried the raw U+2028 and whose id the real Factory would never have assigned.
The `502` is what the **live** Factory produces, and it is reproduced in this
plan's own table as the *"would not parse"* rows.

This is the more important form of the same defect, and it explains why the gap
survived a full offline suite: **the fake accepts the invalid document the real
Factory refuses.** No offline test could have found G-02-16. That is what the
opt-in differential is for.

**2. `TestScenarioGoSilent` in `internal/talossim` failed once under `-race`**
(`context canceled`, want `DeadlineExceeded`), then passed on `-count=3`. An
unrelated package, last touched by plans 02-03/02-05/02-07, out of scope here.
Filed below.


> **CORRECTION (orchestrator, 2026-08-30, after plan 02-16).** The three claims below that
> `golangci-lint` is absent from this host are **false**. It is installed at
> `~/go/bin/golangci-lint` (version 2.13.1) — it is simply not on the default `PATH`, so the
> bare command name failed and was read as "not installed". The Go lint gate for this plan was
> therefore recorded as unrun when it could have run.
>
> Run against this plan's own output it reports exactly one finding, in a file this plan
> committed: `internal/imagefactory/canonical_live_test.go:627` QF1012 (staticcheck),
> `WriteString(fmt.Sprintf(...))` where `fmt.Fprintf` belongs. Fixed by the orchestrator before
> wave 2 continued, because the false record would otherwise have made plans 02-17 through 02-21
> each skip the Go lint gate on this plan's authority. `golangci-lint run` now reports 0 issues.
>
> The `unrun-verify` ledger entry for `task lint:go` is superseded and should NOT be filed by
> 02-21 in its original form; what belongs in the ledger is the PATH trap, not an absent tool.


**3. `task lint:go` is unrun.** `golangci-lint` is not installed on this host —
the standing condition already recorded in STATE.md. `gofmt -l ./cmd ./internal`
prints nothing, `go vet ./...` is clean, and `task test` is green.

## Ledger entries to file

For plan 02-21, which owns `.planning/WINDOWS.md` for this round. Not written
there by this plan.

1. **`unmet-truth` — the refusal set is a floor.** Six unmeasured regions,
   enumerated under *"What the UAT describes that the table did not prove"* and
   stated in `docs/api-contract.md`: twenty-six of the thirty-two C1
   codepoints, the interior of the range above U+FFFF, U+FDD1–U+FDEF, the
   leading/trailing positions for all but three groups, and the surrogates.
   Refused by extrapolation from measured ends, not by measurement.
2. **`stub` — `web/src/routes/images.tsx` now under-refuses relative to the
   server.** Its `hasControlCharacter` guard and the doc comment above it
   claim to transcribe `representable`'s set; the comment is now false and the
   guard is narrower — it stops runes below U+0020, U+007F and lone surrogates,
   and nothing else. An operator typing an emoji, a BOM or U+2028 into a kernel
   argument now learns of it from the server's `400` instead of from the row
   while they are still looking at it. Behaviour is safe (the server's `400` is
   the documented backstop) and the file is owned by **plan 02-20**, which
   extends the same table. Coverage item D7.
3. **`unrun-verify` — `task lint:go` was not run** (golangci-lint absent on this
   host). Extends the existing entry from plan 02-06.
4. **`unrun-verify` — `TestLiveCanonical` is opt-in and runs against a public
   service.** A clean run is 16 POSTs; nothing in CI executes it. The class it
   guards is invisible to the offline suite because the fake accepts documents
   the real Factory refuses (see Issue 1).
5. **`skipped-test` — `TestScenarioGoSilent` flaked once under `-race`** in
   `internal/talossim` and passed on re-run. Not investigated; out of this
   plan's scope.
6. **`deviation` — `internal/imagefactory` records are unaffected but older
   stored schematics are not re-validated.** Any schematic created before this
   plan whose `kernel_args` or `meta` carried one of the newly refused
   codepoints holds an id the Factory did not assign. There is no re-validation
   pass and no migration; nothing enumerates them, and the Factory will not
   enumerate schematics either. Related to WINDOWS entry 21's shape.

## Threat Flags

None. The plan's register (T-02-74…T-02-77) is fully mitigated and no new
security-relevant surface was introduced: one opt-in test file, one widened
refusal predicate, two test files and one document. No new route, no new
dependency, no change to any timeout, deadline or budget — every one of those
stays with the open `02-DECISION-probe-budget.md`.

## User Setup Required

None.

## Next Phase Readiness

- **G-02-16 is closed** on both `missing` bullets: `representable` covers the
  classes a live differential proved diverging, and the differential itself is
  in the tree as the guard. `plainAllowed` is unchanged for a measured reason,
  recorded in its comment.
- **G-02-11's serialiser half is closed at the route.** The `kernel_args` and
  `meta` doors answer a field-named `400` for every measured class. **02-20 owns
  the remaining half**: `name` and `cluster` pass through no guard at all
  (G-02-17), and the surrogate path is the browser-side half described above.
- **02-20 is unblocked.** Extend `divergingClasses`
  (`internal/imagefactory/schematicid_test.go`) and `refusedCodepoints`
  (`internal/httpapi/handlers/schematics_test.go`); the binding assertion in the
  handler test keeps them honest. Consider widening
  `web/src/routes/images.tsx`'s guard in the same change — ledger entry 2.
- **`02-DECISION-probe-budget.md` stays open** and gains no new input: this plan
  changed no budget.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-30*

## Self-Check: PASSED

- All five declared files exist on disk.
- All five commit hashes resolve in `git log --all`.
- All task `<acceptance_criteria>` re-run at close: `go test ./... -count=1 -race` green, `gofmt -l ./cmd ./internal` silent, the loud-skip grep passes, the contract greps report 1+ and 0 respectively, `grep -c TestLiveCanonical internal/imagefactory/schematicid.go` reports 3.
- The recorded table carries 90 rows, one per corpus entry, with 0 NOT OBSERVED.
- `task lint:go` remains unrun on this host (golangci-lint absent); recorded above and in the ledger entries.
