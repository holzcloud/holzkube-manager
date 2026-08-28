# holzkube

Self-hosted management UI for Kubernetes clusters on Talos Linux. A single Go
binary runs **outside** the cluster, talks to the Talos machine API directly,
and serves an embedded web UI.

> Phase 1 status: the foundation only. There is no Talos interaction yet — this
> milestone delivers the binary, HTTPS, the setup wizard, login, the store and
> the audit log.

## Build

The frontend must be built before the Go compiler runs: `go:embed` reads
`internal/httpapi/dist` at compile time, so a missing or stale bundle is
compiled into the binary. The Taskfile encodes that ordering.

```sh
task build          # builds web, then go, into bin/holzkubed
```

Or by hand:

```sh
npm --prefix web install
npm --prefix web run build
go build -o bin/holzkubed ./cmd/holzkubed
```

## Run

```sh
./bin/holzkubed
```

Then open <https://127.0.0.1:8443> and complete the setup wizard. The first
account is created in the browser; no terminal step is required, and there is
no second account.

### The certificate warning is expected

On first run holzkube generates a long-lived self-signed certificate into the
data directory and logs its SHA-256 fingerprint:

```
level=INFO msg="TLS certificate ready" sha256_fingerprint=ab12… url=https://127.0.0.1:8443
```

Your browser will warn about it. **Compare the fingerprint the browser shows
against the one in the log before accepting it.** That comparison is the entire
security value of the warning; clicking through without it accepts anything.

There is no private CA and nothing is installed into your system trust store.
To use your own certificate instead, pass `--tls-cert` and `--tls-key`.

## Configuration

Flags and `HOLZKUBE_*` environment variables only. There is deliberately no
configuration file: nothing to parse means nothing to migrate, and
Docker/Compose speaks environment variables natively. Precedence is
flag > environment > default.

| Flag | Environment | Default |
|---|---|---|
| `--listen` | `HOLZKUBE_LISTEN` | `127.0.0.1:8443` |
| `--data-dir` | `HOLZKUBE_DATA_DIR` | `$XDG_DATA_HOME/holzkube`, else `~/.local/share/holzkube` |
| `--tls-cert` | `HOLZKUBE_TLS_CERT` | generated on first run |
| `--tls-key` | `HOLZKUBE_TLS_KEY` | generated on first run |
| `--insecure-http` | `HOLZKUBE_INSECURE_HTTP` | `false` |
| `--sudo-window` | `HOLZKUBE_SUDO_WINDOW` | `5m` |
| `--session-lifetime` | `HOLZKUBE_SESSION_LIFETIME` | `24h` |

The listener binds loopback by default. Exposing it on all interfaces is an
explicit decision: a management tool reachable from every device on a flat home
network is a different security proposition entirely.

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
go test ./...              # Go tests
npm --prefix web run lint  # Biome
task dev                   # Vite dev server, proxying /api to :8443
```

Run `./bin/holzkubed` in one terminal and `task dev` in another; the dev server
proxies `/api` to `https://127.0.0.1:8443` and accepts the self-signed
certificate.

## Documentation

- [`docs/api-contract.md`](docs/api-contract.md) — error taxonomy, routes, CSRF
  rules and the audit query contract.
