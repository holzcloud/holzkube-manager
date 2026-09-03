# `.planning/`

The record of how this project was built, kept in the repository rather than
beside it.

It is **not documentation of the product** — that is `README.md` and `docs/`.
This is the working material: what each phase was meant to achieve, what was
decided and why, what was deliberately left undone, and what went wrong on the
way. `docs/adr/` plays the same role in the infrastructure repository.

Read it when you want to know *why* something is the way it is. Several
comments in the source point here for exactly that reason.

## What is in here

| Path | Contents |
|---|---|
| `PROJECT.md`, `ROADMAP.md`, `REQUIREMENTS.md` | what the project is for, and in what order it gets built |
| `STATE.md`, `WINDOWS.md`, `milestone.lock` | where the work currently stands |
| `phases/` | per phase: the plan, the summaries, the verification, the deferred items |
| `research/` | the groundwork — stack, architecture, and pitfalls found before they were hit |

## Two things to know when reading it

**It is dated, not current.** An entry records what was true and what was
decided at the time. Where a later decision reversed an earlier one, both
stand and the newer one says so. Nothing here is rewritten to look consistent
in hindsight, which is the only property that makes a record worth keeping.

**`phases/*/deferred-items.md` lists what is knowingly unfinished.** That is
the file's purpose, and publishing it is a choice rather than an oversight: a
project that states its own gaps is easier to judge than one that leaves them
to be discovered. It is not a list of exploitable holes — those belong in
`SECURITY.md`, reported privately.

## History

This directory was kept out of the published repository until 2026-09-03,
mirrored through a script that stripped it on the way out. The arrangement cost
more than it bought — two names for one thing, and a mirror that went stale
without anyone noticing — so it was abandoned and the two repositories became
one.

Deployment-specific identifiers were removed when this tree came back: machine
names, LAN addresses, the hostnames of one particular installation. The example
addresses that remain name nothing real.
