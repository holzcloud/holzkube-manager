---
name: release
description: Cut a new holzkube-manager version. Use when the operator asks for a new release, a new version, or a new beta — "baue eine neue Version", "neues Release", "mach eine Beta". Merges everything to main, has GitHub Actions build and publish the tagged release for linux/arm64 (Raspberry Pi 5, the only target since 2026-10-02), and verifies what an operator would actually download.
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
  Since 2026-09-26 a tag ruleset also stops the Actions token from deleting a
  tag (measured: the one-off reset workflow got 422 "Cannot delete this tag"),
  while creating one still works.

Both are why the workflow takes the tag as a dispatch input and creates it
itself.

## The procedure

1. **Everything on main.** `git branch -r --no-merged origin/main`. Merge what is
   left over; there is usually nothing, because work goes straight to main.
   Working tree clean, `git push` done.

2. **Green here first.** `./bin/task ci`. This is the same gate CI runs, and
   finding a failure here costs a minute instead of a round trip.

3. **Green there on the head commit.** Since 2026-09-28 CI no longer runs on a
   push to main (the Actions minutes ran out), so there is no push run to wait
   for: the dispatch in step 7 runs the gates on the exact SHA itself, and the
   Release job starts only after they pass. Step 2 is therefore the only check
   before the dispatch, and it is not optional.

4. **Pick the version.** Since 2026-09-26 the product is alpha and numbered
   from `v0.0.1` again; every release before that was deleted. Plain
   `v0.MINOR.PATCH`, no `-alpha` or `-beta` suffix: every release is marked
   prerelease by goreleaser anyway (see the end of this file), and the number
   stays plain. New features raise MINOR (`v0.1.0`), fixes alone raise PATCH
   (`v0.0.2`). The alpha status is said in the notes and the app, not in the
   number.

5. **README and changelog.** Two rules of the operator's, and a release is not
   cut without both (CLAUDE.md):
   - The README describes every feature this release ships. Compare
     `git log <previous tag>..HEAD` against its "What it does" list and its
     screenshots; a new screen gets a picture (`node web/scripts/readme-images.mjs`
     after `task build`). Commit that before tagging.
   - `internal/changelog/changelog.json` has an entry for the new version as its
     **first** element, written for the operator. It becomes the release page
     (`.github/release-notes.py`) and the app's "What's new" panel. The job
     refuses a tag without it, and the release has failed twice for exactly that
     -- write it before dispatching, not after the red run.

6. **Write the annotation.** It is the record, so it is worth the ten minutes:
   what this beta covers, what it is *for*, what is new since the previous tag,
   and the known limits with their ledger entry numbers. `git tag -l
   --format='%(contents)' <previous>` shows the shape. Keep it honest about what
   is built-but-unproven; this project has a ledger full of that and hiding it
   in a release note would be the one place it matters most.

7. **Dispatch.** `mcp__github__actions_run_trigger`, method `run_workflow`,
   workflow `ci.yml`, ref `main`, inputs `{tag, notes}`. The job creates the tag
   with its own token, but only after `ci` is green on that
   commit — the dispatch does not skip the gate, it queues behind it.

8. **Watch it.** The run queues behind any in-flight run on `main`: same
   concurrency group, and `cancel-in-progress` is deliberately false outside
   pull requests so a half-uploaded release cannot happen. Expect roughly
   fifteen minutes end to end, most of it the two gates.

9. **Read the verification step.** `Verify the release an operator would
   download` checks the release the API now serves: the linux/arm64 archive,
   exactly one daemon archive each, `checksums.txt`, that it is a published
   prerelease and not a draft, and that its page carries the changelog entry.
   If it fails, the release exists and is wrong — say so
   plainly rather than reporting the goreleaser success.

10. **Report the URL**, the tag, and the assets. Not a file.

## Which architecture is the one that matters

The operator runs this on a Raspberry Pi. **linux/arm64 is the production
target** (see CLAUDE.md).

Since 2026-10-02 it is the only architecture released. Sessions on the
operator's Pi run arm64 natively, and the Pi's hourly timer installs a new
release within the hour -- its journal (`journalctl -u holzkube-manager-update`)
is the real end-to-end check. A cloud container (x86_64, no qemu-user, no
binfmt_misc) cannot run the arm64 binary at all, and no amd64 artifact exists
any more to run instead. Say where a check ran when reporting a release.

What is honestly checkable from a cloud container for arm64: the archive is present, its checksum
matches, it contains `holzkube-managerd`, and `file` says ARM aarch64. Check
those, and say what they do not cover.

## Two settings that move together or not at all

Every release is a **prerelease** (the operator's decision, 2026-09-26: the
product is alpha). `.goreleaser.yaml` sets `prerelease: "true"` and
`draft: false`, and `verify-release.py` fails a release that is not a
published prerelease.

That only works because `deploy/holzkube-manager-update.sh` reads the release
list and takes the newest release that is not a draft: `/releases/latest`
skips prereleases, and the script used to read nothing else. A host still
running the old script finds no release at all and needs the new script
installed once by hand; after that the script replaces itself from each
release archive once the update is healthy. Change the one without the other
and the failure is silent on every machine that matters.
