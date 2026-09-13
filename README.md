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
| `--allowed-hosts` | `HOLZKUBE_MANAGER_ALLOWED_HOSTS` | (none) |
| `--sso-only-hosts` | `HOLZKUBE_MANAGER_SSO_ONLY_HOSTS` | (none) |
| `--data-dir` | `HOLZKUBE_MANAGER_DATA_DIR` | `$XDG_DATA_HOME/holzkube-manager`, else `~/.local/share/holzkube-manager` |
| `--tls-cert` | `HOLZKUBE_MANAGER_TLS_CERT` | generated on first run |
| `--tls-key` | `HOLZKUBE_MANAGER_TLS_KEY` | generated on first run |
| `--insecure-http` | `HOLZKUBE_MANAGER_INSECURE_HTTP` | `false` |
| `--dry-run` | `HOLZKUBE_MANAGER_DRY_RUN` | `false` |
| `--allow-prerelease` | `HOLZKUBE_MANAGER_ALLOW_PRERELEASE` | `false` |
| `--image-factory` | `HOLZKUBE_MANAGER_IMAGE_FACTORY_URL` | `https://factory.talos.dev` |
| `--sudo-window` | `HOLZKUBE_MANAGER_SUDO_WINDOW` | `5m` |
| `--session-lifetime` | `HOLZKUBE_MANAGER_SESSION_LIFETIME` | `24h` |
| `--oidc-issuer` | `HOLZKUBE_MANAGER_OIDC_ISSUER` | (none) |
| `--oidc-client-id` | `HOLZKUBE_MANAGER_OIDC_CLIENT_ID` | (none) |
| `--oidc-client-secret` | `HOLZKUBE_MANAGER_OIDC_CLIENT_SECRET` | (none) |
| `--oidc-client-secret-file` | `HOLZKUBE_MANAGER_OIDC_CLIENT_SECRET_FILE` | (none) |
| `--log-level` | `HOLZKUBE_MANAGER_LOG_LEVEL` | `info` |

`--version` and `--help` print and exit; the help output is generated from the
same table as the flags, so it cannot drift from them. The table above is
checked against that same option table by a test, because three options had
already been added without a row here.

`--image-factory` points at a different Image Factory — a private one at an
air-gapped site, for instance. A response from it carrying a field this build
does not know is decoded past and logged as a warning naming the field, rather
than refused: the Factory adds fields without announcing them, and the
extension catalog is read on every visit to the Images screen.

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

## Signing in

Two ways in, and which of them an address offers is configuration rather than a
build-time decision.

**The local account** is created by the setup wizard and authenticates with a
password. It is the break-glass credential.

**Single sign-on** is OpenID Connect against an external provider, using the
authorization code flow with PKCE. PKCE is used even though holzkube-manager is
a confidential client with a secret: the redirect carrying the code lands in a
browser on an operator's machine, and a code intercepted there is worth a
session against cluster PKI.

### Accounts and roles

The setup wizard creates one account and it is an admin, because it is the only
account and any other role would leave an instance nobody can manage. Further
accounts are created from **Settings → Accounts**, which only an admin sees.

| Role | May |
|---|---|
| `reader` | look. The fleet, the jobs, the plans, the log streams; sign out; change their own password. |
| `operator` | run the fleet: configure, upgrade, provision, reboot, reset, remove a node. |
| `admin` | everything, plus the two things whose blast radius is this instance rather than a node — managing accounts, and downloading the credentials that make this instance unnecessary. |

Admin-only reads are the ones that hand over a credential or the contents of a
cluster: the talosconfig, the kubeconfig, the etcd snapshot and its restore, the
support bundle, the audit archive, and the cluster lock — unlocking is what
makes every other destructive route reachable.

Every route names the least privileged role that may use it, and **a route that
names none cannot be registered**: the process refuses to start. A permission
nobody chose is not a permission anybody reviewed.

**Two changes are refused, and for different reasons.** An account cannot take
away its own admin role — another admin can, which is what stops this being one
click from nobody being able to undo it. And nothing can leave the instance with
no admin at all; the repair for that is a shell on the host. An account *may*
delete itself as long as it is not the last admin: somebody leaving should not
have to ask a colleague, and a rule that made them is a rule people work around
by sharing an account.

An admin can reset another account's password without knowing the old one. Their
own change still asks for it, and that asymmetry is deliberate: the account's own
change defends against a stolen session, and a reset exists precisely because
nobody has the old password any more.

**Upgrading from a single-account installation changes nothing.** An account
stored before roles existed has none, and that is read as admin — it was the only
account, so it could do everything, and demoting it on upgrade would be a lockout
dressed as a security improvement.

**Single sign-on links on first use only while there is exactly one account.**
With two, there is no answer to "which account is this identity", and every
plausible guess is a way for a new subject at the provider to take over somebody
else's account.

### Service accounts

An identity for a machine. It has a role like any other account, it appears in
the audit log under its own name, and it signs in by putting a token on every
request:

```
curl -H "Authorization: Bearer hkm_…" https://holzkube.example/api/v1/machines
```

The token is shown **once**, when the account is created or its token is
rotated. Only a hash is stored, so a lost token is replaced rather than
recovered — and rotating is the only revocation there is: the old token stops
working the instant the new one exists.

A service account is never asked to re-authenticate for a destructive action,
and that is a decision rather than a gap. The re-authentication prompt exists
against a stolen session cookie — somebody who has the session and not the
password — and a token has no such gap: it is not something another site can
make a browser send, and there is no second secret to ask for. Requiring one
would mean giving every service account a password, which is a second and weaker
way in, or putting every destructive route out of reach of automation.

A password never signs in a service account and a token never signs in a person.
Both directions are enforced, because an identity that can be reached two ways
is an identity whose weakest way in is the one that matters.

### Why the local account stays

A cluster manager whose only route to authentication runs on the cluster it
manages locks its operator out exactly when they need it. The provider is
almost certainly a workload on that cluster; if it is down, the tool for
repairing the cluster has to still let somebody in.

Two consequences follow, and both are deliberate:

- **Discovery is lazy.** The provider's metadata is fetched on first use, not at
  start, and failing to reach it is not a startup failure. The process starts,
  the local account works, and the sign-in page says single sign-on is
  unavailable.
- **The password is never removed**, only restricted by address.

### Restricting the password by address

`--sso-only-hosts` names the hosts on which the local password is refused --
to sign in, to open the sudo window, and to run setup. Everything else about
those hosts is unchanged.

```sh
holzkube-managerd \
  --listen 0.0.0.0:8443 \
  --allowed-hosts manager.example.com \
  --sso-only-hosts manager.example.com \
  --oidc-issuer https://idp.example.com/application/o/holzkube-manager/ \
  --oidc-client-id holzkube-manager \
  --oidc-client-secret-file /run/credentials/oidc-client-secret
```

That instance answers on its LAN address with both ways in, and on
`manager.example.com` -- the name a reverse proxy publishes -- with single
sign-on only.

**What this rests on.** The policy keys on the `Host` header, so it is exactly
as trustworthy as the network path that decides which Host can reach the
process. It holds when the public name arrives only through a proxy that
publishes that one name, and the port this server binds is not itself forwarded.
It does not hold if the bind port is reachable from the internet directly: a
caller who can open a socket to it chooses its own `Host`. The Host allowlist is
what makes the header meaningful at all -- an unlisted name is refused before
any of this is consulted.

`--sso-only-hosts` without a configured provider is refused at start: that host
would decline the password and have nothing to offer instead.

### Linking the account to a provider identity

The first sign-in through the provider binds the identity -- issuer and `sub` --
to the one operator account. `sub` is the join key rather than the username,
because a username can be reassigned to a different person at the provider while
`sub` is defined to be stable.

Binding is refused on an SSO-only host. Binding on first sign-in is trust on
first use, and requiring it to happen off the public address puts that trust
behind the same boundary the break-glass account already sits behind, instead of
offering it to whoever reaches the public name first. In practice: sign in
through the provider once from the local network, and the public address works
from then on.

### The client secret

Prefer `--oidc-client-secret-file`. A systemd unit file is world-readable, so an
`Environment=` line in one puts the secret in front of every account on the
host; systemd `LoadCredential=` and Docker secrets both present a file instead.
Giving both forms is refused rather than resolved by precedence.

### Re-authentication and signing out

A destructive action needs an open sudo window (D-05). For a provider session
that is a round trip with `prompt=login` and `max_age=0`, and the returning
`auth_time` claim is checked: a provider that answered from an existing session
has not re-authenticated anybody, and accepting it would make the sudo gate a
redirect with no proof behind it. A provider that sends no `auth_time` at all
cannot demonstrate freshness and is refused.

Signing out of a provider session goes through the provider's RP-initiated
logout. Ending only the local session leaves the provider signed in, so the next
sign-in returns immediately -- indistinguishable, to the operator, from the
sign-out having been ignored.

## Data directory

Everything lives in one directory, `0700`, with `0600` contents:

```
cert.pem  key.pem      TLS material
settings.json          instance settings
VERSION                the schema version, for forward-only migrations
users/                 operator accounts
sessions/              server-side sessions
audit/                 append-only JSONL, one file per day
clusters/              the clusters this instance manages
cluster-secrets/       each cluster's PKI, separate from the cluster record
machines/              the node inventory, flat and keyed by UUID
jobs/                  long-running operations, so a restart resumes them
patches/               reusable configuration patches, versioned
bootstrap/             the etcd bootstrap lease and its intent records
backups/               tarballs, from a migration or from `backup`
```

It is plain files on purpose: readable, and backed up with `cp` — or with the
subcommand below, which excludes the things that should not be in a backup.

The directory is created with `0700` if it does not exist. An existing directory
is left exactly as it is — the store refuses to start on a data directory that is
group- or world-accessible, and quietly fixing it would hide the mistake instead
of reporting it.

### Backups

```sh
holzkube-managerd backup            # writes a tarball into the data directory
holzkube-managerd backups           # lists what is there, newest first
holzkube-managerd restore FILE      # backs up what is there, then unpacks
holzkube-managerd verify-audit      # checks the audit hash chain
```

They are subcommands of the same binary because the backup format, the
permission rules and the chain's hashing all live in this build. A backup
written by one version and refused by another is not a backup.

**A backup is safe to take while holzkube-manager is running.** Every record is
written atomically — temporary file, fsync, rename — so a tarball taken
mid-write captures either the old record or the new one and never half of one.

**A restore is not.** It refuses while another instance holds the data
directory, and it backs up what is there before it starts: a restore that went
wrong without one would have replaced a working installation with a broken one
and left nothing to go back to. Every entry in the archive is checked against
the destination before a byte is written, because `../../../etc/shadow` is a
valid tar entry.

A backup contains every secret in the data directory verbatim. It is `0600`
inside the `0700` directory; copying it somewhere else copies those secrets.

### In a container

`compose.yaml` in this repository is the whole setup. The image runs as uid
65532 from `scratch`, the data directory is a declared volume, and the published
port is bound to loopback by default — the dashboard shows every node's state
and the API can wipe a machine, so putting it on a LAN address is a deliberate
act.

```sh
docker compose up -d
```

Scratch rather than alpine or distroless: the binary is static and embeds its
own web assets, so it needs neither a shell nor a package manager, and both
would be a way in that this has no use for. It does carry the public CA bundle
— the Image Factory is a public HTTPS service, and without it every schematic
and version route answers `502` while the container looks perfectly healthy.

The image is built and run: `docker compose up -d` reaches `Up (healthy)`,
survives a restart with its state, and the data directory inside the volume is
`0700` owned by uid 65532. What has *not* been exercised is provisioning a real
machine from it — see the windows in `.planning/WINDOWS.md`.

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

`holzkube-managerd verify-audit` runs the same check on demand and exits
non-zero on a break, so a cron entry or a monitoring check can use it without
parsing output.

Input parameters are redacted through an **allowlist**: a field not explicitly
listed is written as `<redacted>`. The direction is the point — a denylist
forgets the next secret, and this log is kept forever with no deletion path.
`cmd/holzkube-managerd/allowlist_test.go` asserts that every audited route has an
entry, in both directions.

### Supported Talos versions

v1.12 to v1.14. A node outside that range is marked in the node list and every
version-dependent action against it is refused — the client library would
happily talk to it, which is the problem: an untested API surface that answers
is worse than one that refuses, because the divergence surfaces later, on a
cluster.

Pre-releases are inside the range and are refused anyway unless the instance was
started with `--allow-prerelease`. Everything this product guarantees about a
node is a claim about released Talos.

## Support bundle

When something is broken, one archive with what somebody debugging this cluster
would otherwise collect by hand: per node the facts, the service list, the Talos
and Kubernetes versions, disks, links, extensions, etcd status, recent logs and
`dmesg`, plus the machine configuration **with every secret removed** — and the
audit tail and this instance's own metadata.

Two ways in, because they are reached from different places:

```sh
holzkube-managerd support                      # on the host, when the interface is part of what is broken
holzkube-managerd support --cluster=ID --out=FILE
```

An installation with one cluster is not asked which; one with several is told
its names rather than made to guess. The subcommand opens the store directly,
so it cannot run while holzkube-manager is running — and it says so, naming the
route to use instead.

The other way in is a link on the cluster card, because during an incident an
operator is usually in a browser, and telling them to find an SSH session first
is telling them to do the collection by hand after all.

**A node that does not answer is the point, not an error.** It contributes its
stored record plus a file saying which node and why; the run continues, and the
manifest lists every gap. A collection that stopped at the first dead node would
produce nothing exactly when it matters.

The subcommand says in its own manifest that it reached no node: it takes the
store's lock, so holzkube-manager is not running, so the bundle is the stored
half. Claiming otherwise would be a bundle whose gaps are invisible.

Configurations go through the same two redaction passes the config screen uses,
and a file that still contains a PEM private key afterwards is **left out** with
the gap recorded — a bundle that leaks a certificate authority is worse than one
missing a configuration. The acceptance test walks entropy over every file in
the archive, looking for the simulated cluster's real CA key and for its base64
body without the PEM armour.

Logs are capped per stream. The **newest** bytes are kept, cut on a line
boundary, with a first line saying how much was dropped.

## Labels and machine classes

Labels are your words about a machine — `rack=b3`, `storage=nvme`, `owner=ops` —
set on the node's page. Nothing the node reports ever changes them, and that is
the whole point: a set of machines chosen by label stays the same set across a
reboot, and one chosen by observed facts does not.

A **machine class** is a named set, chosen by those labels, listed under the
nodes table. It is a question rather than a group: a machine joins by being
labelled and leaves by being unlabelled, and the membership is worked out
whenever the class is read. There is one place to look when it is not what you
expected.

Every condition has to hold. A class with no conditions is refused rather than
stored — it would match nothing, and the reading that makes it match everything
is the one that costs a cluster.

## Reaching the cluster from the command line

Two files, from the cluster card:

- **talosconfig** — an admin client configuration for `talosctl`. It is rendered
  here from the stored secrets bundle and reaches no node, and the certificate in
  it is minted on demand rather than being the one holzkube-manager dials with:
  losing your copy does not affect this instance's access.
- **kubeconfig** — admin credentials for the cluster's Kubernetes, so `kubectl`
  works. Unlike the talosconfig it is rendered by a control-plane node and this
  is a passthrough, so it can fail when the cluster cannot be reached.

The kubeconfig download is **recorded in the audit log** and the talosconfig is
not. What it hands over is `system:masters`, and nothing here can take it back —
rotating the cluster's Kubernetes CA is what withdraws it.

## What this product does not do

One operation it performs half of, said here because a product that does the
first half silently is a product whose operator finds out during the incident.

### It restores etcd onto one node and does not rebuild the rest

The upgrades screen takes a snapshot through the etcd API — a consistent
point-in-time copy, which needs a quorum, so a cluster that has lost one cannot
produce it. What is left on such a cluster is a copy of a member's own etcd data
directory, taken off the node; that is a different thing and the screen says so.

It restores one too. The restore is deliberately narrow, and the shape is the
argument for why it is a button at all:

- **It runs on one control-plane node**, named in the request, and the typed
  confirmation is that node's own id rather than a word. Every other
  confirmation in this product asks "did you mean to do this"; this one asks
  "did you mean to do it *here*", because a restore aimed at the wrong
  control-plane node makes that node's data the cluster's and discards the rest.
- **It leaves the other control-plane nodes to you.** They still hold the etcd
  that was just replaced and will not agree with the recovered member, so they
  have to be reset and rejoined. Nothing here does that.
- **Uploading and recovering are two calls.** The snapshot is uploaded first and
  changes nothing; the cluster is only touched by the bootstrap that follows. A
  failure at the upload has not touched anything, and the failure message says
  which of the two places you are standing in — "restore failed" does not.
- **The integrity check is on unless you turn it off.** A snapshot taken through
  the etcd API carries a hash. A copy of a data directory does not, which is
  exactly what you have on a cluster that had already lost quorum, so the flag
  exists — and turning it on for an API snapshot skips the one check that would
  have caught a truncated upload.

Everything written since the snapshot was taken is gone. Keep the file
somewhere that survives the cluster.

### It verifies upgrades and does not undo them

UPG-07 verifies every upgrade by asking the node afterwards: the version that
was installed, the schematic that was installed, and whether the node's own
services are running. "The API said OK" is not accepted as proof, and a node
that comes back on the right version with its extensions gone is caught by that
check rather than by somebody noticing weeks later.

What the check hands you is a broken node and a precise sentence about why.
**holzkube-manager does not undo an upgrade.** Talos installs to one of two
boot partitions and keeps the previous installation on the other, and `talosctl
rollback` against that node boots it — per node, and only until that node is
upgraded again. It undoes the Talos version and nothing else: a Kubernetes
upgrade, and anything etcd did while the node was on the new version, are not
affected.

Every failed verification says so in its own message, because the job screen
renders a step's detail verbatim and that message is the screen at the moment
it matters.

## Metrics

```
GET /metrics
```

Prometheus text exposition. **No session** — a scraper has none, and an endpoint
behind the session cookie is one nobody can scrape, whose usual consequence is a
second listener with no authentication at all. What guards it is the host
allowlist that guards everything else here, and a listener that stays on
loopback unless you moved it.

```yaml
scrape_configs:
  - job_name: holzkube-manager
    scheme: https
    static_configs:
      - targets: ['127.0.0.1:8443']
```

What it exports is what this instance knows and nothing else has: nodes per
stage, seconds left on each cluster's client certificate, job records by kind
and state, confirmed etcd members, and whether the audit chain verified at
startup.

**Export, never ingest.** This does not become a monitoring pipeline: it keeps
no history and it does not alert.

Two properties are deliberate and worth knowing before writing a rule against
it. **No node is ever a label value**, so the number of series follows the
number of clusters rather than the size of the fleet — what an operator graphs
is "how many nodes are down", and the dashboard answers "which one". And
`holzkube_cluster_client_certificate_seconds` **goes negative**: an expired
certificate is a named state rather than a missing metric, because the failure
it describes takes every node in the cluster down in the same second and a
series that vanished at expiry would go blank exactly when somebody needed it.

## Development

```sh
task ci                # the gates CI runs, in CI's order
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

### Running the same gates CI runs

`task ci` exists because it did not, and the gap had a cost: local runs used
`task test` and `task lint:web`, CI additionally ran a pinned Go linter and the
frontend suite, and `main` sat red for nine commits while every local run was
green. A local binary built against an older Go than `go.mod` targets refuses to
run at all, which is why `task lint:go` prints the version it is using next to
the one CI pins.

When the local binary refuses, `task lint:go:install` fetches the pinned version
into `./bin` — the same tarball, at the same version, that the CI action
downloads. It is three lines and it ends a sentence this project has said too
often: "the linter does not run here", which costs a round trip through CI for
every finding and turns a misnamed doc comment into an eight-minute wait.

**A green local run is not a green CI run** unless it ran the same things.

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
