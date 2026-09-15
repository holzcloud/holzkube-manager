---
name: release
description: Cut a new holzkube-manager version. Use when the operator asks for a new release, a new version, or a new beta — "baue eine neue Version", "neues Release", "mach eine Beta". Merges everything to main, has GitHub Actions build and publish the tagged release for linux/amd64 and linux/arm64, and verifies what an operator would actually download.
---

# Cutting a release

## The one rule that outranks the rest

**The release is built by GitHub Actions. Never by this session, and never on
any machine of the operator's.**

A binary built here is not a release: it is unsigned by the pipeline, built from
whatever this container happens to have, and reproducible by nobody. It has been
handed over that way three times — v1.14.0-beta.1, v1.15.0-beta.1 and once
more — and each time `deploy/holzkube-manager-update.sh` still had nothing to
find, because that script reads GitHub releases and a file in a chat is not one.

So: no `task build` output, no `go build` output, and no `SendUserFile` with a
binary in it as the answer to "build a new version". The answer is a release URL.

## Two things this session cannot do, so do not try

- **Push a tag.** `git push origin refs/tags/v…` answers 403. Measured, not
  assumed: a branch push succeeds, a tag push fails, and the agent proxy records
  no relay failure, so the refusal is GitHub's. The Actions token creates tags
  fine, which is how we know there is no tag ruleset on the repository — it is
  this session's credential scope.
- **Delete a ref.** Same 403. A tag that goes out wrong stays; the recovery is a
  new tag, never an untag. `tmp-ref-probe` is still on origin as the proof.

Both are why the workflow takes the tag as a dispatch input and creates it
itself.

## The procedure

1. **Everything on main.** `git branch -r --no-merged origin/main`. Merge what is
   left over; there is usually nothing, because work goes straight to main.
   Working tree clean, `git push` done.

2. **Green here first.** `./bin/task ci`. This is the same gate CI runs, and
   finding a failure here costs a minute instead of a round trip.

3. **Green there on the head commit.** Check the CI run for the exact SHA being
   released. Not "main was green yesterday".

4. **Pick the version.** Milestone-aligned, which is what the existing tags mean:
   `v1.14.0-beta.1` was milestone v1.14, and a commit that says "ship
   v1.15.0-beta.1" was milestone v1.15. So a milestone's first beta is
   `v<milestone>.0-beta.1`, and a second beta of the same milestone is
   `-beta.2`. Read `.planning/` for which milestone is actually complete rather
   than guessing from the last tag — the last tag has been behind before.

5. **Write the annotation.** It is the record, so it is worth the ten minutes:
   what this beta covers, what it is *for*, what is new since the previous tag,
   and the known limits with their ledger entry numbers. `git tag -l
   --format='%(contents)' <previous>` shows the shape. Keep it honest about what
   is built-but-unproven; this project has a ledger full of that and hiding it
   in a release note would be the one place it matters most.

6. **Dispatch.** `mcp__github__actions_run_trigger`, method `run_workflow`,
   workflow `ci.yml`, ref `main`, inputs `{tag, notes}`. The job creates the tag
   with its own token, but only after `ci` and `test-macos` are green on that
   commit — the dispatch does not skip the gate, it queues behind it.

7. **Watch it.** The run queues behind any in-flight run on `main`: same
   concurrency group, and `cancel-in-progress` is deliberately false outside
   pull requests so a half-uploaded release cannot happen. Expect roughly
   fifteen minutes end to end, most of it the two gates.

8. **Read the verification step.** `Verify the release an operator would
   download` checks the release the API now serves: both linux architectures,
   exactly one daemon archive each, `checksums.txt`, and that it is neither a
   draft nor a prerelease. If it fails, the release exists and is wrong — say so
   plainly rather than reporting the goreleaser success.

9. **Report the URL**, the tag, and the assets. Not a file.

## Which architecture is the one that matters

The operator runs this on a Raspberry Pi. **linux/arm64 is the production
target** (see CLAUDE.md).

This session's container is x86_64 with no qemu-user and no binfmt_misc, so an
arm64 binary cannot be run here at all. Every end-to-end check ever made in this
repository has therefore been of the amd64 artifact. Say that when reporting a
release: "verified" without naming the architecture reads as a claim about the
one the operator actually starts, and it has never been true of it.

What is honestly checkable here for arm64: the archive is present, its checksum
matches, it contains `holzkube-managerd`, and `file` says ARM aarch64. Check
those, and say what they do not cover.

## Two settings that move together or not at all

`.goreleaser.yaml` has `draft: false` and leaves `prerelease` at false, and both
are load-bearing for the update script rather than matters of taste:
`/releases/latest` skips drafts **and** prereleases. Marking a beta as a
prerelease reads as the careful choice and is the one that breaks every host
running `holzkube-manager-update.sh` — they would go on installing the previous
build and report themselves up to date.

If betas should ever stop being "latest", the script has to learn to ask for
them first. Change one without the other and the failure is silent on every
machine that matters.
