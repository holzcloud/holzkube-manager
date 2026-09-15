# holzkube-manager

## Where this actually runs

**The operator runs holzkube-manager on a Raspberry Pi, arm64, and that is the
production target.** It manages a Talos cluster on the LAN; the daemon runs
beside the cluster, not in it.

So `linux/arm64` is the architecture that matters. amd64 is still built — the
ROADMAP's OPS-05 is an amd64 hardware run and the Talos nodes themselves are
amd64 — but when only one can be checked, the one to check is arm64.

## What that means for verification, and it is a real limit

**This session's container is x86_64 with no qemu-user and no binfmt_misc. An
arm64 binary cannot be executed here at all.**

Every end-to-end check in this repository's history has therefore run the amd64
artifact: downloaded, checksummed, started, driven in a browser. The arm64
artifact has been built, checksummed and never once run — by anyone, until the
operator started it on their Pi. That is not a small gap and it must not be
described as one. A report that says "the release was verified" means the amd64
build was verified, and should say so.

What can honestly be checked here about arm64: that the archive exists, that its
checksum matches, that it contains `holzkube-managerd`, and that `file` reports
ARM aarch64. Nothing about whether it runs.

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

## The method this repository is built on

A guard is worth nothing until it has gone red against the fault deliberately
put back. Every claim in a commit message about a test covering something means
the fault was reinstated and the test was seen to fail — and a suite that stays
green after an injection that did not actually inject anything is not a result,
it is an unperformed measurement.
