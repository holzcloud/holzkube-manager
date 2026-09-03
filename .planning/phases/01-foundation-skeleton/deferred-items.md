# Deferred items — phase 01-foundation-skeleton

Found while executing plan 01-06, out of that plan's scope, deliberately not
fixed there. Each entry says who it belongs to.

---

## 1. `GET /api/v1/system/status` carries no version, bind address or data directory

**Found:** plan 01-06, while deciding the gap plan 01-05 recorded.
**Owner:** a later plan, or the phase verifier's judgement.

Plan 01-05's dashboard was specified to show version, bind address and data
directory from `GET /api/v1/system/status`; the endpoint returns `setup_required`
and `audit_chain` only. 01-05 correctly refused to invent the fields client-side.

01-06 decided **not** to extend the endpoint. The reasoning is recorded in
`01-06-SUMMARY.md` under *Decisions Made*; in short: the endpoint answers before
authentication, the data directory path is the most sensitive string in the
process, no consumer exists any more inside phase 1 to shape or test the
addition, and `docs/api-contract.md` is the document five parallel plans coded
against.

If the fields are wanted, that is a deliberate contract extension: decide the
shape, decide whether the operational fields are gated on an authenticated
session, update `docs/api-contract.md`, `internal/httpapi/handlers/system.go`,
`internal/httpapi/router.go` (a `Deps` field for the version) and the dashboard
together.

---

## 2. The frontend bundle is 573 kB, over vite's 500 kB warning threshold

**Found:** plan 01-06, in the output of every `task build`.
**Owner:** a frontend plan (01-05's area).

```
internal/httpapi/dist/assets/index-*.js   572.71 kB │ gzip: 176.70 kB
```

Not a defect — everything is embedded in one binary by design and there is no
network round trip to save. It is worth a decision rather than a permanently
ignored warning: either split the router's routes with `import()`, or raise
`build.chunkSizeWarningLimit` on purpose so the warning means something again.

---

## 3. `--version` on an untagged working tree prints a commit hash

**Found:** plan 01-06.
**Owner:** whoever cuts the first tag.

`task build` injects `git describe --tags --always --dirty`, which with no tags
in the repository yields `01d367e-dirty`. That is honest for a working-tree
build, and goreleaser injects the real version for a release. It resolves itself
at the first `v0.1.0` tag; noted so it is not mistaken for a bug.

---

## 4. The build, lint and release tools are not installed on this host

**Found:** plan 01-06.
**Owner:** the operator's machine, not the repository.

`go-task`, `golangci-lint` and `goreleaser` were absent. Plan 01-06 installed
them into a scratch `GOBIN` outside the repository to verify its own work, so
they are **not** on the host's `PATH` afterwards. Install them properly with the
commands in `.planning/research/STACK.md`:

```sh
brew install go-task/tap/go-task
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1
go install github.com/goreleaser/goreleaser/v2@v2.18.0
```

---

## 5. `conduct@holzcloud.ch` is published but the mailbox may not exist

**Found:** 2026-08-28, while preparing the open-source release.
**Owner:** the operator's mail provider, not the repository.

`CODE_OF_CONDUCT.md` names `conduct@holzcloud.ch` in both places Contributor
Covenant provides for it. `holzcloud.ch` has an MX record on
`mta-gw.infomaniak.ch`, so the domain carries mail, but whether that specific
mailbox or alias exists cannot be checked from outside.

A published reporting address that bounces is worse than none: a person raising
a conduct concern gets silence and no second channel. Create the mailbox or an
alias before the repository goes public. Deferred on the operator's instruction.

---

## 6. Pre-open-source backups still live in a session scratchpad

**Found:** 2026-08-28, before the history rewrite.
**Owner:** the operator.

Two artifacts were taken before `.planning/` was purged from the published
history and the commit identities were rewritten:

- `holzkube-full-history.bundle` (780 KB) — all 91 commits at that point,
  restorable with `git clone <bundle>`
- `planning-working-copy/` (1.0 MB) — a plain copy of `.planning/`

Both sit under the Claude session scratchpad, which is session-scoped and will
not survive indefinitely. The private archive repo `holzcloud/holzkube-manager-planning`
holds the same history and is the durable copy, so this is redundancy rather
than the only line of defence. Move them somewhere permanent if that redundancy
is wanted. Deferred on the operator's instruction.

> **Update 2026-09-03.** That archive repo is archived and read-only now; its
> history was merged into this one. This directory is the durable copy again,
> and it is public. The entry above is kept as it was written.
