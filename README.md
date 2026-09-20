# holzkube-manager

Self-hosted management UI for Kubernetes clusters on Talos Linux. A single Go
binary runs **outside** the cluster, talks to the Talos machine API directly,
and serves an embedded web UI.

> Phase 1 status: the foundation only. There is no Talos interaction yet — this
> milestone delivers the binary, HTTPS, the setup wizard, login, the store and
> the audit log.

## Build

One command produces both binaries:

```sh
task build          # builds web, then go, into bin/holzkube-managerd and bin/holzkubectl
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

The command-line client is its own binary and embeds nothing, so it needs none
of the above:

```sh
go build -o bin/holzkubectl ./cmd/holzkubectl   # or: task build:cli
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

### Encrypting a node's disks

A machine being provisioned can have its system volumes encrypted with LUKS2 —
**STATE**, which holds the node's own secrets and certificates, and
**EPHEMERAL**, which holds whatever the workloads write.

**It is decided at install or not at all.** Talos encrypts a system volume while
the volume is empty; handing the same configuration to a node that is already
installed does not encrypt what is on it, does not fail, and does not warn. So
this is a checkbox on the provisioning screen and there is deliberately no way
to turn it on later — a switch that reported success and changed nothing would
be worse than no switch.

Two key kinds are offered:

- **derived from the machine (`nodeID`)** — protects a drive that leaves the
  machine: a disposal, a warranty return, a stolen disk. It does not protect
  against anyone who has the machine, and the screen says so next to the choice.
- **sealed by the TPM** — strong, and only with SecureBoot. The seal is a
  statement about which kernel booted, and without SecureBoot that measurement
  can be produced by a kernel somebody else chose. holzkube-manager knows which
  image the node is about to boot, so it refuses the combination rather than
  installing the weaker thing quietly.

Two are refused, with the reason rather than a validation error:

- **a passphrase in the configuration** — Talos stores the STATE volume's
  encryption config in META in cleartext, so the passphrase protecting the disk
  ends up on the disk. It would also be in this installation's store and in
  every backup of it.
- **a network key server (KMS)** — the strongest of the four, and taking it
  would mean this product runs that server. A node whose key server is down does
  not boot. holzkube-manager runs outside the cluster so that it is there in the
  failure where the cluster is not; a fleet that cannot boot without it would be
  the opposite arrangement.

### Which installer a machine gets

The provisioning screen asks whether the machine booted the **SecureBoot** image.
It has to ask: a schematic id builds both variants, and which one was written to
the USB stick is not recoverable from the id. The answer picks the installer, and
Talos requires it to match — the ordinary installer does not produce a SecureBoot
node.

The installer reference is then **resolved against the Image Factory** rather
than assembled, so it names the repository that actually answers, carries
SecureBoot, and points at the Factory this installation was configured with. If
it cannot be resolved the plan is refused rather than falling back: substituting
the ordinary installer gives you a node that installs, joins, and is not
SecureBoot, and nothing afterwards says so.

### Upgrades and SecureBoot

An upgrade plan reads how each node actually booted and resolves that node's
installer accordingly. It has to: the ordinary installer does not produce a
SecureBoot node, so upgrading one with it takes SecureBoot away from a machine
that had it — and the upgrade succeeds, the node rejoins, and nothing says so.

The fact is read from the node rather than remembered from when this
installation provisioned it, because it may not have been the one that did. A
node that will not answer is blocked rather than upgraded on a guess: either
guess is wrong for half a mixed fleet.

### Certificates

holzkube-manager reaches a cluster with an admin certificate it minted for
itself at adoption, from that cluster's own Talos certificate authority. It is
good for a year, and when it expires **every node in that cluster becomes
unreachable at once** — the cluster is fine, and this can no longer get into it.

So the cluster card counts down, and escalates: a badge at 90 days, a banner on
every page at 30, red at 7. Each card carries the button that answers it:
**Renew this cluster's certificate**.

Renewing touches no node and restarts nothing. A node trusts the *authority*,
not any particular certificate issued from it, so a fresh one is accepted the
moment it is presented.

The new certificate is proven before it is kept. Minting one is trivial; the way
this goes wrong is replacing a working credential with one that is not, and
finding out at the moment the old one expires — precisely when nobody can get in
to fix it. So a connection is opened with the new certificate and a node has to
answer through it before anything is written, and every machine in the cluster
is tried, because "this certificate does not work" and "the node I picked is
switched off" are different findings. If none answers, **the old certificate is
kept and nothing changes**; the message says so rather than reporting a broken
cluster.

Two refusals worth knowing:

- **the stored bundle has no authority key.** The cluster was adopted from a
  talosconfig carrying an admin certificate and nothing to issue from. Import it
  again with a talosconfig that carries the authority, or take a fresh one from
  a control-plane node with `talosctl config new`.
- **no node accepted it.** Either the cluster is unreachable, or its authority
  was rotated outside this tool — see *What this product does not do*.

`holzkubectl renew-certificate <cluster>` does the same thing, which is what
makes this a cron entry rather than a banner somebody has to be logged in to
see. A failed run leaves the old certificate in place, which is what makes that
defensible.

### Growing and shrinking a cluster

Each cluster card answers **what can this cluster spare?** — which of its nodes
may be removed, and what adding one would actually buy. Nothing on it changes
anything: removing a node is that node's own action and adding one is
provisioning. What was missing was never a button.

The arithmetic is the reason it exists, and it goes wrong in a direction that
feels like caution. A majority of *n* is *n/2+1*, so **an even number of voting
members tolerates exactly what the odd number below it does**: four control-plane
nodes survive losing one, the same as three, and the fourth is paying for itself
and buying nothing. Two survive losing none — the same as one. The panel says
this in words, with the numbers, because an operator told "add two" and not why
will add one.

A control-plane node that cannot be removed says so with the reason the removal
route itself would give, including what to do instead. It is the same sentence
because it is produced by the same code: a screen with its own account of the
rule looks right until somebody clicks.

Two things this refuses that are worth naming:

- **the only etcd member.** Removing it does not make the cluster smaller, it
  ends it — no quorum left to rejoin, no member to add one through, and a
  restore from a snapshot as the only way back. To take that node out of
  service, reset it.
- **one of two.** The cluster stops accepting writes and the Kubernetes API
  stops with it. That one is recoverable by adding a control-plane node back.

A locked node is not removable either (a lock is somebody saying "not this one",
and a removal cannot walk past anything the way a rolling upgrade can), and the
reason they wrote is on the refusal.

`holzkubectl scale <cluster>` prints the same thing.

### Cluster templates

A cluster described in one file, with its nodes chosen by machine class rather
than listed by UUID, so the description survives a machine being replaced.

**Nothing applies a template.** The clusters screen says what a document would
mean for the machines this installation knows about — which machines each class
resolves to, and what does not add up — and building or changing a cluster is
still the provisioning and upgrade screens, one decision at a time. That is
deliberate: applying is provisioning, and provisioning has not yet run against
real hardware here.

An existing cluster can be exported as a template. The export lists its machines
by UUID rather than by class, because nothing here can know which of your labels
you meant as the *reason* a machine is in that cluster — guessing would give you
a file that quietly selects a different set later. Turning the list into a class
is one line, and it is yours to write.

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

## holzkubectl — the command-line client

A second way to ask the same questions, for the times a terminal is what you
have: over SSH, in a pipeline, or on a machine with no browser.

It is a **client**, and that is the whole design. Every verdict it prints was
reached by the server and every sentence it shows about a refusal is the
server's own. There is no domain logic in it and there never will be: a second
implementation of a rule is a second thing to keep in step, and two that
disagree are worse than one of them not existing.

```
holzkubectl nodes                 every machine, across every cluster
holzkubectl clusters              the clusters this instance manages
holzkubectl jobs                  long-running operations and where they are
holzkubectl classes               the machine classes and what they name now
holzkubectl scale <cluster>       which of a cluster's nodes may be removed
holzkubectl renew-certificate <cluster>
                                  issue this installation a fresh admin
                                  certificate for the cluster
holzkubectl rotate-authority <cluster>
                                  print what rotating the cluster's Talos
                                  certificate authority would do; add
                                  --confirm <cluster-name> to do it
holzkubectl label <id> k=v ...    replace a machine's labels (none clears them)
holzkubectl template plan <file>  what a cluster template would mean
holzkubectl template export <id>  write a cluster down as a template
holzkubectl kubeconfig <cluster>  admin credentials for the cluster's Kubernetes
holzkubectl talosconfig <cluster> an admin talosconfig
holzkubectl version               this tool's version
```

Configuration is environment variables only — nothing to parse means nothing to
migrate, and a token in a config file is a token in a backup:

| Variable | |
| --- | --- |
| `HOLZKUBE_URL` | `https://holzkube.example:8443` |
| `HOLZKUBE_TOKEN` | a service-account token, from **Settings → Accounts**. Shown once. |
| `HOLZKUBE_FINGERPRINT` | the server's TLS certificate as SHA-256 hex. Optional, and how you reach an instance using its own generated certificate. |
| `HOLZKUBE_TIMEOUT` | how long one request may take. Default `30s`. |

```console
$ export HOLZKUBE_URL=https://holzkube.example:8443
$ export HOLZKUBE_TOKEN=hkm_...
$ holzkubectl nodes
HOST   STATE             ROLE          ADDRESS      TALOS    CLUSTER  LABELS
cp-1   running           controlplane  10.0.0.11    v1.13.9  prod     rack=b
w-3    running (locked)  worker        10.0.0.31    v1.13.9  prod     rack=c
w-4    unknown           worker        (the node has not answered since 09:14)
```

A reading that is not available prints its reason in brackets rather than a
blank. That is the API's `Field[T]` read model reaching the terminal intact: a
node that has not answered has an address that is *unknown*, which is not the
same statement as a node with no address.

**A token, and never a password.** A CLI that took a password would be a second
sign-in path holding a session, and sessions are for browsers: they are ambient,
they are what the CSRF checks and the re-authentication window exist against,
and none of that machinery means anything on a terminal. A token is put on each
request deliberately, is per account, is rotatable, and every use of it is in
the audit archive under that account's name. A token account carries a role like
any other, so a reader's token can read and cannot reboot.

**There is deliberately no flag that skips certificate verification.** This
product pins *node* certificates by fingerprint rather than skipping the check
(D-03), and a tool that took the shortcut it denies its own transport would be
telling you two different things about one risk. `HOLZKUBE_FINGERPRINT` is the
supported way to reach a self-signed instance: the chain cannot be checked
because there is no authority to check it against, but the identity still is,
and by a stronger check than a chain gives.

Add `--json` to any read and the server's bytes come through unchanged, so a
script never depends on this tool's formatting.

`holzkubectl template plan` exits non-zero when the plan has problems, and still
prints them — an exit code that swallowed the reasons would make it worse than
silence.

## One screen for the IT office

`/wall` is a single page meant for a large screen: every node and every workload
as a tile, how full the cluster is, and the most recent warnings. No navigation,
no sidebar, nothing to click — it answers one question, and it has to answer it in
the second somebody glances up.

**It never scrolls.** A screen nobody touches cannot show what is below the fold,
so the tiles shrink to fit. Past the point where the names would be unreadable
from across a room it shows how many there are and draws the ones that are not
fine, which is a true answer at any size — unlike a grid that stops silently at
the bottom edge.

**It says how old its answer is, always**, and dims when that answer is more than
a few refreshes old. A wall that cannot go stale lies during exactly the incident
it exists for: a daemon that died at two leaves a confident green screen up all
night. A failed refresh keeps the last answer on screen with its age visible,
rather than blanking — the state a moment ago, labelled, beats nothing.

**Five colours, not two.** A CronJob between runs is not an outage; something you
stopped on purpose is not broken; and a node nobody is hearing from is never
green. Those three are what a green-and-red wall gets wrong, and the third one is
the case the screen exists for.

**Still to come:** the screen currently needs somebody to sign in on it once, and
a session expires — so it is not yet something to leave up for weeks. A kiosk
link, long-lived and read-only and revocable, is the other half.

## Kubernetes itself

Until this milestone the product managed machines and left the cluster running on
them to `kubectl`. It now speaks to the cluster's own API server as well, with
[client-go], and the screen under **Kubernetes** shows what the cluster says
about itself: the API server's version, its nodes, its pods, its deployments and
its namespaces.

**How it authenticates, and what it deliberately does not do.** It mints itself a
client certificate from the Kubernetes authority in the cluster's stored bundle —
common name `holzkube-manager`, one hour, never written to disk. It does not reuse
the admin kubeconfig Talos hands out, and the reason is your cluster's audit log:
that log records the common name, and a product acting as `admin` would make the
log say nothing about who acted. A cluster whose bundle carries no Kubernetes
authority is still fully manageable over the Talos API; the Kubernetes screen
says so instead of showing an empty list.

**Kubernetes's view and the inventory's are allowed to disagree, and where they
do, that is the information.** The inventory knows what the machine API says
about a machine. This knows whether the kubelet registered, what the scheduler
will do with the node, and whether somebody cordoned it.

**An API server that does not answer is not a cluster with no pods.** Every read
on this screen distinguishes "the cluster said" from "the cluster could not be
asked", for the reason the inventory spent two phases learning: an empty screen
is a claim.

### Node scheduling: cordon and drain

Cordoning stops the scheduler placing anything new on a node; uncordoning undoes
it completely. Draining cordons and then moves what would move, and it comes back
as a job, because it waits out each pod's termination grace period and can take
minutes.

**A drain evicts rather than deletes.** Eviction is the request a
PodDisruptionBudget can refuse — and when one does, the answer names the pod and
the budget instead of retrying past it. Deleting the pods directly would ignore
the budget the cluster's owner wrote down.

**Two decisions the drain refuses to take for you**, the same two `kubectl`
refuses:

- a pod no controller owns is **gone** once it is evicted, not moved. It needs
  `force` said out loud.
- a pod with an `emptyDir` loses that data when it moves. It needs
  `delete_local_data`.

Pods a DaemonSet owns and mirror pods are reported and not evicted: they come
straight back on the same node, which is what they are for.

This closes a gap the product had been papering over with a sentence. Removing a
node from a cluster and upgrading one both used to tell you to cordon and drain
by hand first, because there was no Kubernetes client to do it with.

### Pods and deployments

**"Restart this pod"** deletes it so its controller makes another. Kubernetes has
no restart verb, and those two are the same operation only while a controller
exists — so a pod nothing owns is **refused**, by name, rather than deleted
because somebody clicked a word.

**Scaling and rolling are different operations** and the screen keeps them apart.
Scaling changes how many pods there are, through the same scale subresource
`kubectl scale` uses; zero is a real answer and is how a workload is switched off.
A rollout restart annotates the pod template, which makes the deployment
controller replace the pods under its own surge, `maxUnavailable` and readiness
probes — so the workload stays up. Deleting a deployment's pods one at a time
would take it down, and that is why both buttons exist.

### Applying a manifest

Paste a manifest, press **Plan**, read what it would do, then **Apply**. Several
documents separated by `---` are fine.

**The plan is not a courtesy, and Apply stays disabled until there is one for
exactly the text in the box.** Editing the manifest throws the plan away: a plan
for a document you have since changed is worse than no plan, because it is a
reassuring description of something else.

**`create` and `update` come from asking the cluster**, one object at a time. They
cannot be read off a manifest — it looks identical either way, which is exactly
why you cannot tell from the text which of your objects already exist.

**A kind's resource and scope come from the cluster's own discovery**, so a
`CustomResourceDefinition` your cluster has installed works here without this
product having heard of it. A kind the cluster does not have is named in a
warning rather than buried in a discovery error. A namespaced object that names no
namespace is warned about, with the namespace it would land in.

**Server-side apply, under this product's own field manager**
(`holzkube-manager`), so `kubectl get -o yaml --show-managed-fields` can tell you
what this product set. When another field manager owns a field your manifest sets,
the API server answers with a conflict and **this product reports it instead of
forcing past it**: forcing takes the value away from whatever is managing it —
usually a controller that will set it back — which is a fight a management product
must not pick on your behalf. `force` is never sent.

**It does not delete, and there is no `--prune`.** Pruning decides what to remove
by comparing against a previous apply, and getting that wrong deletes things
nobody asked about. Removing an object is a separate operation and is not built.

An apply of several objects **attempts all of them** and reports each one: an
apply of ten where the sixth conflicts has changed five things, and a single
"failed" would leave you guessing which five. At most 256 objects per manifest,
and at most 1 MiB.

### Whose name your cluster sees

By default every Kubernetes request this product makes arrives as
`holzkube-manager`, in `system:masters`. Your cluster's audit log records that
name — so it says *this product* scaled a deployment, for every operator, for
ever, and cannot answer the one question an audit log exists for. And
`system:masters` bypasses RBAC entirely, so your cluster cannot express "this
person may restart pods in `web` and nothing else".

On the Kubernetes screen, **Who this acts as** changes that. Give it the name
your API server knows you by — for OIDC usually your email, with whatever
`--oidc-username-prefix` adds — and requests carry it as an impersonation header.
The API server then records both: this product as the impersonator, you as the
actor, and RBAC decides what you may do.

**Nothing is stored before the cluster has been asked.** The panel runs an access
review for every permission this product issues, as that identity, and shows the
cluster's own verdicts. A name your RBAC has never heard of would otherwise break
every Kubernetes screen at once, and look like the product is broken rather than
like a setting is wrong.

**A refusal is never retried as the administrator.** That is what makes this worth
having rather than decorative: with a fallback, every request would succeed either
way and your RBAC would decide nothing. A refused screen stays refused and says
whose refusal it was. You can always go back to this product's own certificate
with one button.

### What runs, and what is beside it

The Kubernetes screen lists **every** workload kind, not only Deployments: your
storage layer is a DaemonSet, your database a StatefulSet, your backup a CronJob,
and a list without them shows a cluster nobody runs. The numbers mean different
things per kind and are not flattened into one — a DaemonSet's size is how many
nodes match, a CronJob has no pods at all between runs, and "0 of 0 ready" would
report a schedule as an outage. What a kind cannot do is not offered: a DaemonSet
gets no replica field, a Job no roll button.

Beside them: **ConfigMaps, Secrets, persistent volume claims, Ingresses,
autoscalers and disruption budgets**. Each answers a question the workload list
cannot — where the setting lives, whether a URL reaches the cluster, why a claim
is Pending, why a replica count goes back after you scale by hand, why a drain
refuses. The ones that are the *reason* something is stuck come first.

**A disruption budget allowing nothing is not a fault.** With one copy, taking it
down *is* the outage, so the budget allows nothing — that is every single-replica
database in every cluster, and marking them all would be a warning nobody reads.
The screen says what a drain will do instead, and marks only a budget that is not
met: a pod is already missing, and a drain will be refused on top of that.

Secrets are listed and never read: their names and key names answer "does this
namespace have the pull secret", and their values appear nowhere, because base64
is not encryption.

**Removing an object** asks you to type its name. It is the only thing here that
doing again does not undo, and the rows look alike. Deleting a Namespace is
refused outright — it removes everything inside it and nothing stops it once it
starts.

### Why a node will not take a pod

Every node has a **Why?** button. The pod's own events say `0/2 nodes are
available`, which names no node; this says which and why.

**Taints**, with what each does in words — `NoSchedule` keeps new pods off,
`NoExecute` evicts the ones already there, and those are different days. The
control-plane taint is marked ordinary rather than presented as a finding, because
every Talos control-plane node has one.

**Conditions**, with the inversion handled: `Ready` is bad when it is False, a
pressure is bad when it is True.

**Room left**, which is allocatable minus what pods **requested** — what the
scheduler reserves, not what anything uses. A node at 5% CPU can have no room. The
screen says that rather than leaving it to be inferred.

### What is being used

Separately, and only if you have installed metrics-server: Talos does not ship
one. Without it the screen says nobody is collecting this, which is not the same
as usage being zero. A pod's usage is summed across its containers, the way
`kubectl top` shows it.

### Running a command in a container

From a pod's **Why?** panel. It is not a terminal and does not pretend to be one:
a program and its arguments run once and the output comes back.

Four things make this something a management product can offer at all. It is
**not a shell** — `sh -c "…"` is refused, because an archive holding `sh -lc` and
one opaque argument cannot say what happened. **The command is the event**: every
argument is kept in the audit archive, which is only possible because of the
first. It **runs as you**, and is refused outright unless the cluster has an
identity set — never falling back to this product's own certificate. And it is
**bounded**: thirty seconds, 256 KiB, nothing left open.

There is deliberately **no port-forward**. Reading from a service already works
through the service proxy, and a forward would mean this daemon holding a
listener whose authentication story is nobody's. If you need one, you have
`kubectl`.

### Why a pod is broken

Every pod on the Kubernetes screen has a **Why?** button, and it answers in the
order the question is actually asked: which container failed and how it ended,
its log, and what the cluster reported about it.

**The log opens on the run that already crashed.** A pod in `CrashLoopBackOff`
is, at the moment you look at it, waiting to start again — its current container
has printed nothing. Everything that explains the crash belongs to the run that
ended, and Kubernetes keeps exactly one of those. A log view without that answers
every crash loop with an empty box.

**A container's ending is put in words.** Exit code 137 is a memory limit, 1 is a
throw, `ImagePullBackOff` never started; those are different days of work and the
pod's own summary cannot tell them apart. A container that asked for no CPU or
memory is named as one the scheduler places blind and the kubelet evicts first.

**Events are where "Pending" is explained**, and nowhere else: the pod says
nothing, and `0/2 nodes are available: insufficient cpu` says everything. They
are shown per pod and for the cluster, warnings first. An empty list always
carries the reason it is not a claim — a cluster forgets its events after about
an hour.

**Any object can be shown as YAML**, for the question the lists do not cover.
`managedFields` and the last-applied annotation are removed and the answer says
so, because an object silently missing fields is how somebody concludes a field
is not set when it is.

**A Secret is refused, not redacted.** Its `data` is base64 rather than
encryption, so rendering it would put the credential on the screen. Redacting was
the obvious alternative and is worse: it teaches that looking at Secrets here is
safe, and the first field it misses is a credential on a screen that promised it
was not.

**None of a log's content reaches the audit archive.** A log line is whatever the
workload printed — tokens, connection strings, personal data — and the archive is
kept forever, so a secret written there has no path that removes it. The archive
records that somebody read the log of a named pod, and nothing of what they read.

### Reaching a service

Pick a service, pick one of its ports, type a path, press **Fetch**. The answer
appears as text.

**It fetches; it does not host.** A workload's own content type is deliberately
thrown away and the body is shown as text in a code block — never rendered, never
in an iframe. Rendering it would put that pod's markup in this product's own
origin, the origin holding your session, so any pod in the cluster could script
this interface. What this is good for is `/healthz`, `/readyz`, `/metrics` or a
JSON endpoint; what it is not is a way to use a workload's web interface through
here.

**Only GET is ever sent.** The identity this product holds inside your cluster is
powerful, and a proxy that forwarded any method would let anyone with an operator
session drive any in-cluster API with it — an unauthenticated admin endpoint on
some pod included — while the audit log recorded "proxy" rather than what was done.

**It goes through the API server's proxy**, not a tunnel this daemon opens. Your
cluster's own authorisation decides whether this identity may reach that service,
and no new listener is opened anywhere.

**The archive records the port and the path in clear**, because "somebody read
`/healthz`" and "somebody read `/admin/users`" are different events. A response is
cut at 1 MiB, and the cut is reported rather than made quietly.

### Namespaces, quotas and the cluster's own kinds

A namespace has been a filter everywhere in this product. Two things about one are
findings in their own right, and both look like the workload's fault:

**Stuck in `Terminating`.** Something in it has a finalizer nothing will clear, so
the namespace hangs — for weeks — the name cannot be reused, and recreating it
fails with "already exists" while using it fails too. `kubectl get ns` shows the
word and nothing about what is holding it; the API server's own condition does, and
that is what the row carries.

**A full quota.** It is why the next pod is refused, and the refusal appears on the
*pod* as "exceeded quota" in a namespace whose quota nothing showed. Only the
resources actually full are named — a quota at 6Gi of 8Gi is ordinary, and naming
it would bury the one that matters.

**A compute quota with no LimitRange**, which refuses every pod that sets no
requests — not for being too big, but because the quota cannot account for a pod
that asked for nothing. The error says "must specify limits", which reads as the pod
being wrong. A quota that only counts objects does not do this, so it is not
warned about.

**The cluster's own kinds**, because its operators keep their state in them and
"what is a Longhorn Volume called here" is not answerable from a list of pods. A
definition the API server is not serving is marked: every manifest naming it is
refused, and nothing else in a cluster says so. Objects are not counted per kind —
that would be one request per definition, and a cluster can have two hundred.

### Who may do what

`kubectl get rolebindings` gives a list of names. Every question somebody has
about RBAC is whether an arrangement *works*, and RBAC has **no referential
integrity** — deliberately, so that a binding can be written before its role. So
nothing in a cluster will tell them:

**A binding naming a role that is not there grants nothing at all**, and looks
exactly like one that grants everything it was written for. Same for a binding
naming a service account that was never created, or was deleted with the binding
left behind. Both are marked, and the screen says why RBAC permits it — otherwise
the finding reads like a bug in the cluster.

**Who is an administrator is not a field.** It is worked out from the rules: every
subject that reaches wildcard verbs on wildcard resources through some binding.
Looking for the name `cluster-admin` would miss every hand-written role with the
same power under a name nobody recognises, which is the usual way somebody grants
it by accident.

**One rule has to carry all three wildcards.** "`*` verbs on configmaps" and "get
on `*`" are both ordinary, and a check that added them together would report half
the cluster's built-in roles as administrative — a warning everybody sees is a
warning nobody reads.

**A subject is only marked missing when it can be checked.** A User or a Group
lives in the identity provider and no cluster has a list of them; marking those
would teach that the marking is noise.

**What runs as which service account**, because a pod runs as one and finding out
otherwise means reading every binding in the cluster. The `default` account
matters most: every pod naming none runs as it, so a permission granted there
reaches things nobody intended.

**The seventy roles Kubernetes ships fold away** behind a button rather than being
filtered out, so the screen can still answer "does this cluster have the standard
roles".

Nothing on this screen writes. A wrong RBAC change locks the operator, and this
daemon, out of the cluster.

### Storage

The claim list was one half of a two-sided arrangement, and the half that cannot
answer the questions storage raises. Each row here carries something no claim
knows.

**Whether deleting it destroys the data.** That is the *volume's* reclaim policy,
and the screen says what happens — "data kept" or "data destroyed" — rather than
printing `Retain` and `Delete` under a heading nobody reads.

**Who is using it.** Every running pod that mounts the claim. A finished pod is
left out: it mounts nothing any more, and counting it is how somebody decides a
claim is in use when it is not. A bound claim nothing mounts says so, because that
is storage being paid for and not used.

**Why a claim is Pending.** The claim's own events say "unbound immediate
PersistentVolumeClaim", which states what somebody already knows. Two of the three
real causes are not faults at all: a class that waits for a consumer is working as
configured, and a claim with no class is waiting for the default one. The third —
a class name that does not exist in the cluster — will never be provisioned, and
no amount of waiting changes that.

**A volume whose claim is gone and whose data is not.** `Released` is invisible in
every namespace view, counts against nothing, and is the commonest way a cluster
quietly fills its storage backend. It is also, sometimes, exactly the data
somebody needs back, so both readings are on the row.

**A capacity is what was provisioned, never how full the filesystem is.** Nothing
in the Kubernetes API reports the second; only something running inside the pod
can.

Nothing on this screen deletes. Deleting a `Retain` volume is how data goes for
good, and the object-delete route already does it for anybody who means it, with
the name typed out.

### Networking

**A Service with nothing behind it is the commonest broken thing in Kubernetes,
and it looks completely healthy in every list** — name, type, ClusterIP, ports all
present, every connection to it refused instantly, and the workload that calls it
reporting a connection error that looks like its own fault. This screen counts
what is actually behind each Service and puts the broken ones first, naming the
selector that matches nothing.

**Both endpoint numbers are shown**, because an unready endpoint is not in the
load balancer at all: three endpoints of which none are ready serves nothing while
looking better than having none.

**A plain ClusterIP says "inside only"**, which is the answer to "why can I not
reach this from my laptop". A LoadBalancer with no address says it is waiting for
one — Talos ships no load-balancer controller, so that is the ordinary state here
rather than a fault.

**The namespaces with no NetworkPolicy are listed by name.** A namespace without
one accepts traffic from every pod in the cluster. That is Kubernetes's default
and plenty of clusters run that way on purpose, but it is invisible, and "we have
policies" is usually believed about a cluster where two namespaces have them and
eleven do not.

**A policy that matches no pod is marked broken**, because somebody believes it is
protecting something. And an empty pod selector means *every* pod in the
namespace, which is the opposite of how an empty filter reads everywhere else, so
it is written out in words.

Nothing on this screen writes. A NetworkPolicy applied wrongly cuts a cluster off
from itself, including from whatever this daemon needs to reach it.

### How full the cluster is

Three levels of the same question, on one screen: the cluster's totals, each
node's, and each pod's own reservation.

**This is not the same number as usage, and neither replaces the other.** Usage
needs a metrics-server and Talos ships none, so the usage panel usually says —
correctly — that nobody is collecting it. That is a true answer that reads exactly
like nothing, which is how an operator came to report seeing no resource
information at all.

So the figure every cluster has is here too: **allocatable against what the pods
requested**. It is the scheduler's own arithmetic, so it is also what decides
whether the next pod starts. A cluster at 90% requested and 5% used is
over-reserved and will refuse work it could do; one at 20% requested and 95% used
is about to fall over while looking empty. Those are opposite repairs.

**A node that has reported nothing says so**, rather than appearing as a node with
all its room free. **A cordoned or unready node is left out of the cluster
totals** — nothing new will be placed there — **and its pods are not**, because
they are still on it. The screen says which nodes those were.

### Stopping and starting a service

**Stopping a pod is not something Kubernetes has.** Delete one and its controller
makes another within seconds; that is the whole point of a controller, and it is
why the restart button works at all. So **Stop** tells the controller to want
none, and a pod nothing owns is refused with that said rather than deleted and
the deletion called a stop.

**Starting again restores the count it was running.** Scaling to zero throws that
number away, so it is written onto the workload as an annotation before the scale
— and the screen shows what a start would bring back *before* anybody presses it.
A three-replica service therefore comes back as three, instead of silently as one.
Where nothing recorded the count — something else scaled it down — the screen says
that too, and a start runs one.

A **CronJob** or **Job** stops by being suspended, which for them is exactly what
stopping means: the schedule is kept and nothing runs. A **DaemonSet** has no
stop, and the refusal says what to do instead — cordon or drain the nodes.

### Clearing out what is finished

A nightly CronJob leaves a finished Job and its pod behind every night. Each
rollout leaves the previous ReplicaSet at zero replicas so it can be rolled back
to, and the one before that. None of it runs, none of it reserves anything, and
all of it makes every list harder to read.

**The button does not delete.** It asks what *would* be removed and shows the list
with a reason on every row; a second press on that list removes it. The list is
sent back as it was shown rather than recomputed, because between the two a
CronJob can run, and nobody should lose something they never saw in what they
approved.

**What is deliberately left alone:** a pod that is Pending, Running or Unknown —
Unknown especially, since it means a node stopped reporting rather than that the
pod stopped; the newest ReplicaSet of a Deployment, which is the way back; a Job
a CronJob still owns, which is that CronJob's own history; and anything younger
than an hour, because its logs go with it and somebody may be reading them.

**Images are not in that list, and cannot be.** The Talos machine API can list the
images on a node and has no delete — image removal is the kubelet's own garbage
collection, which runs when the disk fills. The screen says so instead of
offering a button that would do nothing.

[client-go]: https://github.com/kubernetes/client-go

## What this product does not do

Operations it performs half of, said here because a product that does the first
half silently is a product whose operator finds out during the incident.

### It rotates a cluster's authority and has never proven it on hardware

**Renewing** this installation's own certificate is the easy half, and it is
built: each cluster card has a button that issues a fresh admin certificate from
the cluster's own Talos certificate authority, which this installation already
holds — see *Certificates* below. It touches no node, because a node trusts the
authority rather than any one certificate issued from it.

**Rotating the authority** changes what every node trusts, and it is now built
too — as a job, in four passes, in this order:

1. every node accepts the new authority as well as the one it uses now,
2. every node starts issuing from the new authority,
3. holzkube-manager takes a certificate from the new authority and proves it
   against a node before keeping it,
4. every node stops accepting the old authority.

Between the passes the cluster trusts two authorities, which is a working state,
so an interrupted rotation is continued rather than restarted: the authority
being moved to is stored, every pass has a read-only check beside it, and the
job says which pass it was on. Getting the order wrong is what leaves a cluster
that trusts nobody, so the order is enforced in two places — the sequence of the
passes, and a refusal in each pass that reads the node's own configuration
first.

**The refusal that matters: every node in the cluster has to answer.** It is
checked after pass 1 and before pass 2, the last moment at which stopping costs
nothing. A node that misses pass 1 refuses the certificate every other node
accepts after pass 2, and nothing afterwards can repair it — reaching it would
need the credential it no longer accepts. So a node that is merely switched off
stops the rotation, which is the opposite of what a rolling upgrade does with a
locked node, and for the opposite reason.

**What is not proven: that a real cluster survives it.** Every pass is measured
against the simulator, which was taught for this to adopt an applied
configuration as its active one and to derive its TLS from it — which client
authorities it accepts, and who issued its own certificate. That makes the
mechanism and every refusal measurable. It does not make this a claim about your
hardware, and the dialog says so before you type the cluster's name. Ledger
entry 103 stays open until a rotation has run on real metal.

**The Kubernetes authority is not rotated.** Talos keeps the two apart.
Rotating the Kubernetes authority through machine configuration alone would
leave every kubelet holding a certificate from an authority the API server no
longer has, so it is refused rather than half done. `talosctl rotate-ca
--kubernetes` is the tool for that one.

If a rotation happened outside this tool, this installation's stored authority
is the old one: adopt the cluster again with a fresh talosconfig. Renewing here
refuses until you do, and the refusal says so: *"the certificate authority in
this installation's store is no longer the one the cluster trusts."*

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

### It applies manifests and has never done so against a real cluster

Every part of the manifest path is measured — discovery, both scopes, the
create-versus-update decision, a conflict reported rather than forced, and four
deliberately reinstated faults that each turned the tests red. All of it against
an in-process API server this repository ships (`internal/kubesim`), which serves
real Kubernetes JSON over real TLS with a real client certificate.

What that fake does not have: the `managedFields` bookkeeping a real API server
keeps, admission webhooks, and kinds from groups beyond `core/v1` and `apps/v1`.
The mechanism and the refusals are proven; your cluster's reaction to your
manifest is not. Ledger entry 148 stays open until a manifest has been applied
through this product against real hardware.

The worst case is bounded by one decision: `force` is never sent. So an apply that
meets something it does not own fails and says so, rather than taking a field away
from the controller that owns it.

## On a phone

The whole interface is meant to be used from a phone, and that is a measured
property rather than a hope: `web/scripts/layout-audit.mjs` drives every screen
at 390px and at 1280px in a real browser, against real rows, and fails the build
on three things — an element past the right edge with nothing to scroll, a
control smaller than 44px, and a table wider than the screen.

**Below 768px a table is not a table.** Rows you act on — nodes, pods,
deployments, clusters, accounts — become cards: the name on top, the figures as
labelled values, and the buttons underneath the name they act on. Rows you scan —
the audit archive, recent activity — become a compact list with the rest folded
away, and tapping one opens its detail.

The reason is not taste. A wide table on a phone scrolls sideways, and the first
column is what scrolls away first: you end up looking at a "Scale" field and a
"Roll pods" button with no way to see which deployment they belong to. An action
detached from its subject is a different and worse defect than an ugly layout,
and it is what this is built to prevent.

**44px is the minimum target below 768px.** WCAG 2.5.8 asks for 24; Apple and
Material name 44 for a finger, and a cluster screen gets read one-handed while
something is broken.

**The guard renders real rows, and that had to be fixed before any of this could
be believed.** It used to start a daemon with an empty data directory, so every
screen showed nothing and passed. The first run with data found eight tables
between 521 and 1027px wide and ten controls under 44px — all of which had been
there, passing, for as long as the guard had existed (ledger 149).

**It also drove eleven screens out of fourteen.** The node detail page was
missing, and so were `/setup` and `/login` — the first two anybody ever sees. Those
two were missing for an ordering reason rather than an oversight: the script
created an account and signed in before measuring anything, and after that
`/setup` no longer exists. It now runs three passes in the only order that works:
`/setup` against a fresh daemon, then `/login` with an account but no session,
then the rest signed in. The first run of the node detail page found fifteen
elements past the right edge with nothing to scroll (ledger 153).

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
