# Contributing to holzkube-manager

Two facts shape most of what follows. The web UI is compiled into the binary,
so a frontend change is also a Go build and both sets of checks run for either
side of a change. And the data directory is equivalent to root on every managed
node, so the permission guard and the certificate fingerprint below are not
ceremony.

Do not open an issue for anything that looks like a vulnerability. Report it
privately through the repository's Security tab; see [`SECURITY.md`](SECURITY.md).
A public report is a working attack against every running instance.

## Prerequisites

These are the versions the project is developed and tested against on
darwin/arm64. Others may work; these are the ones known to.

- **Go 1.26.7.** `go.mod` declares `go 1.26` with `toolchain go1.26.7`, so a machine
  whose Go predates 1.26.7 fetches the pinned toolchain by itself.
- **Node 22** with **npm 10**.
- **go-task 3.53.1**, **golangci-lint 2.13.1**, **goreleaser 2.18.0**.

```sh
brew install go-task/tap/go-task
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1
go install github.com/goreleaser/goreleaser/v2@v2.18.0
```

`go install` writes to `$(go env GOPATH)/bin`. Put it on your `PATH`, or the
commands below are not found.

The `/v2` in both install paths is load-bearing. `.golangci.yml` is the v2
schema and says `version: "2"` on its first line; a v1 binary does not read it
at all, so you would get a lint run that checks something other than what the
file describes and reports clean. Check what you installed:

```sh
task --version && golangci-lint --version && goreleaser --version
```

## Getting a dev loop running

From a clean clone:

```sh
task build        # npm ci, vite build, then go build into bin/holzkube-managerd
./bin/holzkube-managerd
```

Open <https://127.0.0.1:8443> and complete the setup wizard. The first account
is created in the browser; there is no terminal step and no second account.

The frontend is built **before** the Go compiler runs, and that order is a hard
dependency rather than a convention: `go:embed` reads `internal/httpapi/dist` at
compile time, so a missing or stale bundle is compiled into the binary and
afterwards looks like a frontend bug. `build:go` depends on `build:web` in the
Taskfile for exactly that reason.

Only `internal/httpapi/dist/.gitkeep` is tracked, so `go build ./...` succeeds on
a fresh clone before the web bundle has ever been produced — but the binary it
produces serves an empty page. Run the web build at least once.

`npm ci`, not `npm install`: `ci` installs exactly what `web/package-lock.json`
pins and fails if the lockfile and the manifest disagree. If you change a
dependency, the updated lockfile belongs in the same commit.

### Two modes, and the daemon is needed for both

**The binary alone.** `./bin/holzkube-managerd` serves the embedded bundle on
<https://127.0.0.1:8443>. This is what a user gets, and it is what you should
check a UI change against before opening a PR. Frontend edits are not picked up
until you rebuild with `task build`.

**Vite on :5173.** `npm --prefix web run dev` (equivalently `task dev`) starts
the dev server on <http://localhost:5173> with hot reload and proxies `/api` to
`https://127.0.0.1:8443`, with `secure: false` so the self-signed certificate is
accepted. Vite serves no API of its own — every `/api` call is forwarded — so
**holzkube-managerd must be running either way**. Without it the UI loads and every
request fails.

The normal loop is two terminals:

```sh
./bin/holzkube-managerd                          # terminal 1
npm --prefix web run dev                 # terminal 2 → http://localhost:5173
```

Working on Go only, with the bundle already built once:

```sh
go build -o bin/holzkube-managerd ./cmd/holzkube-managerd
```

That skips `npm ci`, which is most of the build time.

To get a fresh setup wizard, point the daemon at a throwaway directory rather
than deleting your real one:

```sh
./bin/holzkube-managerd --data-dir /tmp/holzkube-manager-dev
```

holzkube-managerd creates that directory with mode `0700` when it does not exist. See
below for what happens when it does exist and is wrong.

## The certificate warning is expected

On first run holzkube-managerd generates a long-lived self-signed leaf certificate into
the data directory and logs its SHA-256 fingerprint:

```
level=INFO msg="TLS certificate ready" sha256_fingerprint=7B:20:0B:F1:…:F7:99 url=https://127.0.0.1:8443
```

Your browser will warn. **Compare the fingerprint the browser shows against the
one in the log before accepting it.** That comparison is the entire security
value of the warning; clicking through without it accepts anything, including
whatever is on the other end when something else has taken the port.

The fingerprint is printed as colon-separated upper-case hex pairs, which is the
format browsers use in their own certificate dialog. The comparison is
character-by-character with nothing to convert — which is the point of printing
it that way.

That line is `INFO`, so `--log-level=warn` or higher suppresses it. The same
string comes out of the file:

```sh
openssl x509 -in ~/.local/share/holzkube-manager/cert.pem -noout -fingerprint -sha256
```

The certificate is generated once and reused on every later start, so you accept
it once per data directory and the fingerprint you accepted stays valid. Deleting
`cert.pem` and `key.pem` regenerates it — and changes the fingerprint, so expect
the warning again.

At <http://localhost:5173> you never see the dialog: the browser talks plain HTTP
to Vite, and the proxy accepts the certificate on your behalf. That is convenient
and it is also why the dialog has to be checked against the binary on :8443, not
against the dev server.

`*.pem` and `*.key` are in `.gitignore`, so TLS material generated inside a
working tree cannot be committed by accident. To use your own certificate,
pass `--tls-cert` and `--tls-key`.

## The data directory must be 0700

Default `~/.local/share/holzkube-manager`, or `$XDG_DATA_HOME/holzkube-manager` when that is set.
Override with `--data-dir` or `HOLZKUBE_MANAGER_DATA_DIR`; the override wins over both.

The directory is `0700` and its contents are `0600`. The store walks the whole
tree at startup and **refuses to start** if anything in it grants group or other
access. Every violation is collected and reported at once, so you repair once and
start once:

```
holzkube-managerd: fsstore: data directory permissions are too permissive:
  /Users/you/.local/share/holzkube-manager is mode 0755, want 0700
  /Users/you/.local/share/holzkube-manager/settings.json is mode 0644, want 0600

Repair with: chmod 0700 /Users/you/.local/share/holzkube-manager && chmod 0600 /Users/you/.local/share/holzkube-manager/*
```

Nothing is repaired automatically, and that is deliberate on both sides.
holzkube-managerd creates a missing directory with `0700`, but it never changes the mode
of one that already exists: a silent `chmod` would repair the condition the guard
exists to report, and the window during which the files were readable by every
account on the host is the thing worth knowing about.

A symlink or device node anywhere in the tree is refused for the same reason
under a different heading — it is not a permission problem but a shape problem,
because it can point anywhere.

The common way to hit this in development is creating the directory yourself:
`mkdir -p` under the usual `umask 022` gives you `0755`. Either let holzkube-managerd
create it, or `chmod 0700` after you do.

## Before you open a pull request

Run all five against the change itself, not against an earlier build:

```sh
task build
golangci-lint run
go test ./...
npm --prefix web test
npm --prefix web run lint
```

All five apply to a change on either side. The binary embeds the UI bundle, so a
frontend change that fails to build is a Go build failure; and a Go change can
break the API contract the frontend tests assert against.

What passing looks like today:

- `golangci-lint run` reports `0 issues.` and exits 0. `.golangci.yml` sets
  `max-issues-per-linter: 0` and `max-same-issues: 0`, so that is a real zero and
  not the default cap of 50 per linter making a noisy run look quiet.
- `npm --prefix web test` reports 64 tests in 5 files.
- `npm --prefix web run build` runs `tsc --noEmit` before Vite, so a type error
  fails `task build`. `npm --prefix web run typecheck` runs that step alone.
- `npm --prefix web run lint` is `biome check`, which is lint **and** format
  check. If it complains about formatting, run `task fmt` — gofmt and Biome, both
  in place — rather than fixing it by hand.

Two more that are not in the five but are worth knowing:

- `task test` runs the same Go suite with `-count=1 -race`. Run it before you
  push anything touching concurrency; `go test ./...` alone will not catch it.
- `task release:snapshot` builds the cross-compiled archives locally without
  tagging or publishing. Run it if you touch `.goreleaser.yaml`, the build flags
  or anything that could pull in cgo — `CGO_ENABLED=0` is what lets one machine
  build every target, and a cgo dependency breaks that everywhere at once.

`go test ./...` does not reach `sandbox/`: it is a separate module and Go's
package patterns stop at a nested `go.mod`. If you change anything there, build
and test it from inside that directory. `internal/depguard_test.go` fails if a
package of the Talos root module ever reaches `cmd/holzkube-managerd`, which is the
boundary `sandbox/` exists to hold.

If your change touches the HTTP API, update
[`docs/api-contract.md`](docs/api-contract.md) in the same PR. That document is
binding, the error taxonomy is closed, and the `code` values never change —
clients branch on them.

[`.github/pull_request_template.md`](.github/pull_request_template.md) carries
this list as a checklist, plus two things worth reading twice: name the
requirement or roadmap phase the change serves, and confirm that no credential,
kubeconfig, talosconfig or private key is in the diff — including in test
fixtures.

## Commit messages

The convention here is what the log actually does, so read some of it before
writing your first one:

```sh
git log --oneline -30
git log -3
```

What you will find:

- **`type(scope): subject`.** The types in use are `feat`, `fix`, `docs`, `test`,
  `chore` and `style`.
- **The scope is the roadmap phase, not a package.** `fix(01): …` for
  phase-wide work, `feat(01-06): …` for plan 6 of phase 1. The earliest commits
  predate the phase numbering and use a bare `docs:` or `chore:`.
- **A fix closing a numbered review finding leads with the id**, before the
  prose: `fix(01): WR-08 check the Host header against an allowlist`.
- **Subjects are lower case after the colon and carry no full stop.** They
  describe the change, not the file it lives in. There is no 50-character rule:
  the median in the tree is 63 characters and the longest is 108. Length follows
  the change.
- **Bodies are used, heavily, and hard-wrapped in the high seventies.** They
  explain the failure mode and why the fix is shaped the way it is, and they
  quote measurements instead of adjectives. Anything not obvious from the subject
  gets a body.
- **Machine-assisted commits carry a `Co-Authored-By:` trailer.**

The type is not decoration at release time: `.goreleaser.yaml` excludes
`^docs\(`, `^test\(` and `^chore\(` from the generated changelog, so a feature
committed as `chore(…)` silently disappears from the release notes.

## Licence and sign-off

holzkube-manager is licensed under **AGPL-3.0-only** (GNU Affero General Public License
version 3; see [`LICENSE`](LICENSE)). **Contributions are accepted under
AGPL-3.0-only.** There is no CLA and no copyright assignment: you keep your
copyright, and your contribution is licensed under the same terms as the rest of
the project.

The Affero clause is the reason for AGPL rather than plain GPL. Section 13 covers
the case that matters for a management UI — a modified holzkube-manager offered to other
people over a network. Contributing here means your work carries that obligation
forward too.

Sign off every commit:

```sh
git commit -s
```

That appends a `Signed-off-by: Your Name <your@email>` line, which is your
statement of the [Developer Certificate of Origin](https://developercertificate.org)
— in short, that you wrote the patch or otherwise have the right to submit it
under this licence. Use your real name and an address that reaches you; the
sign-off is a record, not a formality.

No commit in the history carries the trailer yet and no bot enforces it today,
so if you are one of the first contributors you will be adding the first ones.
Amend with `git commit --amend -s` if you forget, or `git rebase --signoff` for a
branch.

## Map of the codebase

```
cmd/holzkube-managerd/       main.go, the only main package
internal/            the product
web/src/             the React UI, compiled into the binary
sandbox/             separate module, outside the product build
docs/                the binding API contract
```

**`cmd/holzkube-managerd`** is one file. It resolves configuration, builds the logger,
opens the store, the audit log and the session store, assembles the route table
from each handler package's own `Routes` function, sets up TLS and the HTTP
server timeouts, and handles shutdown. New routes add a line here and a file in
`handlers/`; the router itself is not touched.

**`internal/`**

- **`audit/`** — append-only JSONL, one file per day, chained by
  `hash_n = sha256(hash_{n-1} || canonical_json(record_n without its hash field))`. Rotation across the
  file boundary, an allowlist redactor for captured request parameters, and the
  filtered cursor-paginated query the audit view calls.
- **`auth/`** — argon2id hashing with startup calibration, sessions
  (`alexedwards/scs`, with `scsstore/` adapting it to the file store), the login
  delay, and the sudo window that gates destructive routes.
- **`config/`** — `config.go` is the single option table that produces the flags,
  the `HOLZKUBE_MANAGER_*` variables, the `--help` text and the startup log of every
  effective value and its origin. `datadir.go` resolves and creates the data
  directory.
- **`httpapi/`** — `router.go`, `problem.go` (RFC 9457 `application/problem+json`,
  where the rule that an `internal` problem leaks nothing lives) and `web.go`
  (the embedded bundle and the SPA fallback). `handlers/` is one file per route
  group: system, setup, auth, account, audit. `middleware/` is request id, panic
  recovery, security headers, the CSRF preconditions, the Host allowlist that
  closes DNS rebinding, session, the sudo gate and the audit chain.
- **`model/`** — the shared record types.
- **`store/`** — the store interface and the per-record lock. `fsstore/` is the
  filesystem implementation: atomic write via temp-chmod-write-fsync-rename, and
  the permission guard above. `migrate/` is forward-only schema migration with a
  pre-migration backup.
- **`tlsx/`** — self-signed generation, reuse across restarts, loading an
  operator-supplied pair, and `Fingerprint`, which produces the colon-separated
  upper-case hex the log prints.
- **`depguard_test.go`** — the module boundary, as a test that runs rather than a
  note in a document.

**`web/src`** — `main.tsx` and `App.tsx` at the root, `api.ts` the typed client.
`routes/` is one file per screen (setup, login, audit, error, the placeholders)
plus the root route. `components/` holds the shell, sidebar, header, the sudo
dialog and the audit chain banner; `components/ui/` is shadcn output and is
normally not hand-edited. `hooks/` is session and theme, `lib/` is problem
rendering and class utilities, `test/setup.ts` is the vitest setup.

The Vite build writes to `internal/httpapi/dist`, which `go:embed` compiles into
the binary. `task clean` empties it but never removes the tracked `.gitkeep` —
without that file `go:embed` fails outright on a fresh clone.
