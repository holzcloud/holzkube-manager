# holzkube-manager

## Continuing this work somewhere else

`.planning/HANDOVER.md` is the handover: the state as of 2026-09-17, what is
still open **on the operator's own machine** (their cluster will not delete,
and why), the recipes for driving the daemon here without a browser, and the
three things this environment cannot do at all. It is dated rather than
maintained; where it and a later file disagree, the later file wins.

## Where this actually runs

**The operator runs holzkube-manager on a Raspberry Pi, arm64, and that is the
production target.** It manages a Talos cluster on the LAN; the daemon runs
beside the cluster, not in it.

So `linux/arm64` is the architecture that matters. amd64 is still built — the
ROADMAP's OPS-05 is an amd64 hardware run and the Talos nodes themselves are
amd64 — but when only one can be checked, the one to check is arm64.

## This repository is public

Nothing that identifies the operator's installation goes into it: no LAN
address, host name, public name, mail address, MAC or home directory from the
real setup. Not in tests, not in fixtures, not in the ledger, and not in commit
messages. Use documentation values (192.168.1.10, homeserver, example.com)
instead. That also holds when the value is what the journal or a screenshot
showed.

This happened once already: the identifiers were removed before the repository
went public on 2026-09-03, came back through sessions working against the real
cluster, and on 2026-09-26 had to be cut out of the whole history, with every
tag moved. `internal/publicrepo` now fails the gate on the known values; it
lists them as hashes, so a new one gets added there as a hash too.

## A feature is not done until the README says so; a release is not done without its changelog

The operator's standing rule, 2026-09-26.

**Every new feature updates the README in the same change.** The README is the
short tour a public repository opens on: the "What it does" list, and a
screenshot in "A look around" when the feature is a screen of its own. Depth
goes into `docs/guide.md`, not the README. Screenshots are rendered, never
taken from the real cluster: add the screen's data to `web/fixtures/demo.json`
and run `task build && node web/scripts/readme-images.mjs`.

**Every release carries its changelog.** The entry in
`internal/changelog/changelog.json` -- newest first, written for the operator,
not a commit list -- is what the app's "What's new" panel shows and, through
`.github/release-notes.py`, what the GitHub release page shows. The release job
refuses a tag without one, and `verify-release.py` fails a published release
whose page does not carry it.

## What that means for verification

**Where a session runs decides what it can check, so say which one it was.**

**On the operator's Pi (aarch64, srv-node-01)** — where sessions run since
2026-09-17 — arm64 executes natively, and the production daemon runs beside the
checkout: `holzkube-manager.service`, data in `/var/lib/holzkube-manager`,
`holzkube-manager-update.timer` pulling releases hourly. Its journal
(`journalctl -u holzkube-manager`) is the operator's real traffic against the
real cluster, and the first read of it found a defect no suite had (ledger 137).
Read it before assuming the product works. Measured limits there: no
`go test -race` (ThreadSanitizer refuses the Pi 5 kernel's 47-bit address
space, so the race detector is CI's alone), no Node unless installed, Go only if
installed (`~/.local/go`), and no GitHub credentials for push or dispatch.
Replacing the production binary, restarting the service, or copying its data
directory (it holds cluster secrets) is the operator's call, every time.

**In the cloud container (x86_64, no qemu-user, no binfmt_misc)** an arm64
binary cannot be executed at all. Every end-to-end check before 2026-09-17 ran
the amd64 artifact; a report from there that says "the release was verified"
means the amd64 build, and should say so. What that container can honestly
check about arm64: the archive exists, its checksum matches, it contains
`holzkube-managerd`, and `file` reports ARM aarch64.

## Talos versions

**The operator updates Talos as soon as a release lands, and wants the newest
release supported.** That makes the machinery pin a product decision rather than
housekeeping: a pin that lags means this product advertises support for a Talos
whose configuration documents its own library cannot decode.

That is not hypothetical. v1.16.1 advertised `v1.12 to v1.14` while pinning
machinery v1.13.9, and a real v1.14 cluster refused adoption because
`DiscoveryServiceConfig` — a document kind v1.14 has and v1.13.9 does not — was
"not registered".

Two things hold it now, and they cover different halves:

- `internal/talos/machineryversion_test.go` compares `gendata.VersionTag`, the
  machinery actually compiled in, against `MaxSupportedVersion`. It runs in the
  gate and catches the range claiming more than the library can read.
- `.github/workflows/talos-upstream.yml` asks weekly whether a newer stable
  machinery exists and fails when one does. Nothing in a build looks outside the
  repository, so this is the only thing that can notice.

Raising the pin means: `go get …/machinery@vX.Y.Z`, `go mod tidy`, move
`MaxSupportedVersion` if the minor changed, run `./bin/task ci`, and deal with
what the new machinery deprecates rather than suppressing it wholesale.

## How decisions get put to the operator

**When something needs the operator to decide, it is asked as a CHOICE, never as
an open question.** Named options, each with what it costs and what it buys, and
a recommendation when there is one. "Was soll ich tun?" makes the operator do the
work of inventing the alternatives; a list makes them do the work of picking one,
which is the part only they can do.

This holds for the open decisions this project already has -- cloud providers, a
Kubernetes client, SAML -- and for every one that comes up mid-task.

**It holds hardest for the last two sentences of a long message, which is where
it has actually been broken.** A report that ends "sag Bescheid, ob..." or "dann
brauche ich von dir, welche..." has asked an open question with the choice list
missing, and the length of what came before is no excuse: that is the moment the
operator is being handed the work of inventing the options. If a message would
end in a question, the question is a choice, or it does not go in the message.

## The method this repository is built on

A guard is worth nothing until it has gone red against the fault deliberately
put back. Every claim in a commit message about a test covering something means
the fault was reinstated and the test was seen to fail — and a suite that stays
green after an injection that did not actually inject anything is not a result,
it is an unperformed measurement.
