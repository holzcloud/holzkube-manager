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

## The method this repository is built on

A guard is worth nothing until it has gone red against the fault deliberately
put back. Every claim in a commit message about a test covering something means
the fault was reinstated and the test was seen to fail — and a suite that stays
green after an injection that did not actually inject anything is not a result,
it is an unperformed measurement.
