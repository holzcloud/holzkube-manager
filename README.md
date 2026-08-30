# holzkube-manager

Self-hosted management UI for Kubernetes clusters on Talos Linux. A single Go
binary runs **outside** the cluster, talks to the Talos machine API directly,
and serves an embedded web UI.

> Phase 1 status: the foundation only. There is no Talos interaction yet — this
> milestone delivers the binary, HTTPS, the setup wizard, login, the store and
> the audit log.

## Build

One command produces the binary:

```sh
task build          # builds web, then go, into bin/holzkube-managerd
```

The frontend is built **before** the Go compiler runs, and that ordering is a
hard dependency rather than a convention: `go:embed` reads
`internal/httpapi/dist` at compile time, so a missing or stale bundle is
compiled into the binary and looks like a frontend bug afterwards. `build:go`
depends on `build:web` in the Taskfile for exactly this reason.

Without `task`:

```sh
npm --prefix web ci      # ci, not install: builds what the lockfile pins
npm --prefix web run build
go build -o bin/holzkube-managerd ./cmd/holzkube-managerd
```

Toolchain: Go 1.26.7 (pinned in `go.mod`), Node with npm, and — for the full
task, lint and release chain — [`go-task`](https://taskfile.dev),
`golangci-lint` v2.13.1 and `goreleaser` v2.18.0.

## Run

```sh
./bin/holzkube-managerd
```

Then open <https://127.0.0.1:8443> and complete the setup wizard. The first
account is created in the browser; no terminal step is required, and there is
no second account.

### The certificate warning is expected

On first run holzkube-manager generates a long-lived self-signed certificate into the
data directory and logs its SHA-256 fingerprint:

```
level=INFO msg="TLS certificate ready" sha256_fingerprint=7B:20:0B:F1:…:F7:99 url=https://127.0.0.1:8443
```

Your browser will warn about it. **Compare the fingerprint the browser shows
against the one in the log before accepting it.** That comparison is the entire
security value of the warning; clicking through without it accepts anything.

The fingerprint is printed as colon-separated upper-case hex pairs, which is
what browsers show in their certificate dialog — so the comparison is
character-by-character with nothing to convert.

That line is `INFO` level, so `--log-level=warn` or higher suppresses it. The
same string comes out of:

```sh
openssl x509 -in "$HOLZKUBE_MANAGER_DATA_DIR/cert.pem" -noout -fingerprint -sha256
```

The certificate is generated once and reused on every later start, so the
fingerprint you accepted stays valid. It is a leaf certificate and not a
certificate authority: it cannot sign anything else.

There is no private CA and nothing is installed into your system trust store.
To use your own certificate instead, pass `--tls-cert` and `--tls-key`.

## Configuration

Flags and `HOLZKUBE_MANAGER_*` environment variables only. There is deliberately no
configuration file: nothing to parse means nothing to migrate, and
Docker/Compose speaks environment variables natively. Precedence is
flag > environment > default.

| Flag | Environment | Default |
|---|---|---|
| `--listen` | `HOLZKUBE_MANAGER_LISTEN` | `127.0.0.1:8443` |
| `--data-dir` | `HOLZKUBE_MANAGER_DATA_DIR` | `$XDG_DATA_HOME/holzkube-manager`, else `~/.local/share/holzkube-manager` |
| `--tls-cert` | `HOLZKUBE_MANAGER_TLS_CERT` | generated on first run |
| `--tls-key` | `HOLZKUBE_MANAGER_TLS_KEY` | generated on first run |
| `--insecure-http` | `HOLZKUBE_MANAGER_INSECURE_HTTP` | `false` |
| `--sudo-window` | `HOLZKUBE_MANAGER_SUDO_WINDOW` | `5m` |
| `--session-lifetime` | `HOLZKUBE_MANAGER_SESSION_LIFETIME` | `24h` |
| `--log-level` | `HOLZKUBE_MANAGER_LOG_LEVEL` | `info` |

`--version` and `--help` print and exit; the help output is generated from the
same table as the flags, so it cannot drift from them.

Every option is logged at startup with its effective value **and where that
value came from**:

```
level=INFO msg=configuration option=listen      value=127.0.0.1:8443 origin=default
level=INFO msg=configuration option=sudo-window value=9m0s           origin=environment
```

A value that does not parse — a duration without a unit, an unknown log level —
aborts the start with a message naming the option, its origin and the offending
value. It never falls back to the default: running with a configuration the
operator believes is in force but is not is the worse failure.

The listener binds loopback by default. Exposing it on all interfaces is an
explicit decision, and it is not refused — but it is logged as a warning at
every start: a management tool reachable from every device on a flat home
network is a different security proposition entirely.

`--insecure-http` serves plain HTTP and is **refused unless the bind address is
loopback**. The session cookie grants access to cluster PKI; it does not cross a
home network in the clear.

## Data directory

Everything lives in one directory, `0700`, with `0600` contents:

```
cert.pem  key.pem      TLS material
settings.json          instance settings
users/                 operator accounts
sessions/              server-side sessions
audit/                 append-only JSONL, one file per day
```

It is plain files on purpose: readable, and backed up with `cp`.

The directory is created with `0700` if it does not exist. An existing directory
is left exactly as it is — the store refuses to start on a data directory that is
group- or world-accessible, and quietly fixing it would hide the mistake instead
of reporting it.

### In a container

`HOLZKUBE_MANAGER_DATA_DIR` is the volume path. Mount a named volume or bind mount there,
give it to the non-root user the container runs as, and nothing else needs
configuring — every option is an environment variable:

```yaml
services:
  holzkube-manager:
    image: holzkube-manager
    user: "1000:1000"
    environment:
      HOLZKUBE_MANAGER_DATA_DIR: /data
      HOLZKUBE_MANAGER_LISTEN: 0.0.0.0:8443
    volumes:
      - holzkube-manager-data:/data
    ports:
      - "8443:8443"
```

### Blast radius — stated plainly

From phase 2 this directory holds cluster CA **private keys**. Anyone who can
read it can mint an admin `talosconfig` and an admin `kubeconfig`, and can
therefore wipe every machine in the cluster. **The holzkube-manager data directory is
equivalent to root on every managed node.**

Two consequences, neither of which code can fix:

1. The host running holzkube-manager is inside the cluster's trust boundary. It deserves
   control-plane-grade treatment, not "that Raspberry Pi in the corner".
2. Compromise of the host is compromise of the cluster. There is no partial
   credential design that avoids this — generating machine configuration
   genuinely requires the CA key.

There is no encryption at rest in this version. The honest mitigation is
full-disk encryption (FileVault, LUKS) plus host hygiene, and saying so is
better than implying a defence in depth that does not exist.

## Audit log

Every mutating request writes two records — the intent before the action, the
outcome after — chained by `hash_n = sha256(hash_{n-1} || canonical_json(record_n
without its hash field))`. The chain is verified at startup, not behind a
button, and a break is reported through `GET /api/v1/system/status` and shown
as a banner in the UI.

The chain is tamper-**evidence**, not tamper-proofing: anyone who can write to
the data directory can rewrite the whole chain. Shipping records off-box is the
only real answer and is not in this version.

## Development

```sh
task test              # go test ./... -race
task test:web          # vitest, both projects
task test:web:browser  # only the tests that measure layout in a real browser
task lint              # golangci-lint and Biome
task fmt               # gofmt and Biome, in place
task dev               # Vite dev server, proxying /api to :8443
task clean             # build output, never the tracked dist placeholder
task release:snapshot  # cross-compiled archives locally, without publishing
```

### The frontend tests need a browser

`task test:web` runs two vitest projects. Most tests run under jsdom, which is
fast and lays nothing out. A small second project opens the UI in a headless
Chromium, because two of the things this app has to get right — that an installer
repository name never wraps into a different image's name, and that the schematic
detail dialog is not clamped narrow — are facts about layout, and no assertion
available in jsdom can distinguish them.

Install the browser once:

```sh
npm --prefix web exec -- playwright install chromium
```

Roughly 150MB, cached afterwards. Run through the project's own playwright so the
browser matches the pinned version. If it is missing, the test run says so and
repeats this command. CI installs it the same way, so the gate is not one that
only exists there.

Run `./bin/holzkube-managerd` in one terminal and `task dev` in another; the dev server
proxies `/api` to `https://127.0.0.1:8443` and accepts the self-signed
certificate.

### Module layout

`cmd/holzkube-managerd` depends on the light `pkg/machinery` only. The Docker and QEMU
provisioners live in the Talos **root** module, which pulls in a large part of an
operating system, so they get their own module under
[`sandbox/`](sandbox/README.md) — outside the product build and outside
`go list ./...`. `internal/depguard_test.go` fails the build if a root-module
package ever reaches `cmd/holzkube-managerd`.

## Documentation

- [`docs/api-contract.md`](docs/api-contract.md) — error taxonomy, routes, CSRF
  rules and the audit query contract.

## Licence

holzkube-manager is free software under the **GNU Affero General Public License,
version 3** — see [`LICENSE`](LICENSE).

The Affero clause is the reason for this choice rather than a plain GPL: section
13 covers the case that matters for a management UI, namely a modified holzkube-manager
offered to other people over a network. If you run a changed version and let
anyone else use it, they are entitled to your changes. Running an unmodified
holzkube-manager on your own cluster obliges you to nothing.

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the toolchain and the checks a
change has to pass, and [`SECURITY.md`](SECURITY.md) before reporting anything
that looks like a vulnerability — please do not open a public issue for those.
