# Security policy

holzkube-manager runs outside the cluster and holds the credential that guards it. Its
data directory already holds the operator's password hash, the live sessions and
the TLS key; from phase 2 it holds the cluster CA **private keys**, at which
point reading that directory is equivalent to root on every managed node. Take
reports here seriously, and read the security model below before deploying it
anywhere.

## Supported versions

holzkube-manager is pre-1.0. There is no stable release series, no maintenance branch and
no backporting: a fix lands on `main` and goes out in the next tag. Only the most
recent release is supported.

| Version | Supported |
| --- | --- |
| latest `0.x` release | yes |
| `main` | yes |
| any earlier `0.x` | no — upgrade |

The on-disk format, the HTTP API and the flags may all change between `0.x`
versions. Breaking changes are called out in the release notes.

## Reporting a vulnerability

**Do not open a public issue for a security bug.**

Report it through GitHub private security advisories:

> [github.com/holzcloud/holzkube-manager → Security → Report a vulnerability](https://github.com/holzcloud/holzkube-manager/security/advisories/new)

That channel is private to you and the maintainer, it keeps the discussion, the
patch and the eventual advisory in one place, and it needs no address for either
side to publish or verify.


### What to include

- The version (`holzkube-managerd --version`) and the OS and architecture.
- The bind address, and whether `--insecure-http` was in use.
- Whether the instance had completed setup.
- The steps, in order, and what they achieve.
- **What the attacker already has when they start.** Network reach only? A
  session cookie? A browser the operator is logged into? A local account on the
  host? This matters more than a severity score, because almost everything in
  holzkube-manager's threat model turns on it.

### What not to include

Do not attach the data directory or anything out of it. `key.pem`, `users/` and
`sessions/` are live secrets, and an advisory thread is not the place for them. A
reproduction against a throwaway instance, or a redacted log excerpt, is what is
wanted. The certificate fingerprint is not a secret and is fine to quote.

### Worth reporting

Anything that gets past a gate that is supposed to hold. For example:

- Reaching an authenticated route without a valid session, or forging one.
- Performing a destructive route without an open sudo window.
- A cross-origin or rebound request that the CSRF preconditions or the Host
  allowlist should have refused.
- A change to the audit archive that still verifies, or a way to suppress a
  record for a mutation that happened.
- A secret reaching the audit log, the server log or a client response —
  a password, a session token, a private key, a filesystem path.
- A path that creates or leaves a file in the data directory readable by
  another account on the host.
- Anything that reaches the Talos machine API without an authenticated,
  sudo-authorised operator, once that surface exists.

## What to expect

One maintainer, hobby-scale, no on-call rota. These are numbers one person can
honestly keep, not vendor numbers.

| Stage | Target |
| --- | --- |
| Acknowledgement that the report arrived | 5 days |
| First assessment — is it a bug, and how bad | 14 days |
| Fix released, or a written plan with dates | 90 days |

If ten days pass with no acknowledgement, assume the message went astray rather
than that it was ignored. Open a public issue saying only that a private report
is waiting for a reply — no details — and it will get picked up.

Disclosure is coordinated. The advisory is published when the fix ships or at 90
days, whichever comes first. You are credited by name or handle unless you would
rather not be. If a report is being exploited in the wild, both halves move
faster and the warning goes out before the fix: an operator who cannot upgrade
today still needs to know to unplug today.

A report that turns out to be a bug but not a security bug becomes a normal
public issue, with your agreement.

**Dependency scanner output**: check first whether holzkube-manager actually reaches the
affected code. If it does, report privately. If it does not, a public issue is
fine and gets an answer sooner.

## Security model

One binary, one operator, one data directory. There are no roles, no second
account and no server-side multi-tenancy, so every control below is about
keeping a stranger out rather than about separating insiders.

### What holzkube-manager defends against

**Being on the network at all.** The listener binds `127.0.0.1:8443`. A wider
bind is permitted — it is a legitimate choice — but it is logged as a warning at
every single start, because a management console reachable from every device on a
flat home network is a different security proposition. `--insecure-http` is
refused outright unless the bind address is loopback, and the refusal is a start
failure rather than a request-time surprise.

**A passive network attacker.** HTTPS by default, TLS 1.2 minimum. On first run a
P-256 self-signed certificate is generated into the data directory at `0600` and
its SHA-256 fingerprint is logged in the same colon-separated upper-case hex the
browser shows, so the warning can be compared rather than clicked through. It is
a leaf, not a CA: if its key escaped, it could not sign anything else. Nothing is
installed into a system trust store. Passing `--tls-cert` and `--tls-key` never
falls back to self-generation — a certificate the operator supplied is either in
force or the server does not start.

**Offline attack on the stolen password hash.** argon2id in PHC encoding, so the
parameters travel with the hash. 64 MiB of memory and a parallelism of 4 are a
floor that nothing can lower; the iteration count is calibrated at startup so
that one verification costs at least 250 ms on that host, and the measurement is
logged — including when the host was too slow to reach the target. A hash written
with cheaper parameters is upgraded on the next successful login, which is the
only moment the cleartext exists.

**Online password guessing.** Failures from one source address make the next
attempt wait: 250 ms, doubling, stopping at 30 s. The source is the peer address
and never a forwarded header, because honouring one would let a guesser reset
their own counter on every attempt. There is deliberately no lockout and no
counter that survives the wait — with exactly one account, any state that can
refuse the operator is a state they can be stranded in, and every way out of it
is a second way in for somebody else.

**Account enumeration.** An unknown username is verified against a decoy hash, so
the cost of "no such user" matches the cost of "wrong password". The 401 response
is byte-identical for both, and for a malformed login body as well; the error
constructor takes no arguments precisely so no caller can vary it.

**Session theft and session fixation.** Sessions are server-side records in the
data directory; the cookie carries only an opaque token and is `HttpOnly`,
`Secure` and `SameSite=Lax`. The session id is rotated *before* the identity is
attached, so an authenticated identity is never briefly reachable under a
pre-authentication id. The lifetime is absolute — 24 hours from login, with no
sliding renewal, so traffic cannot extend a stolen session indefinitely.

**What a stolen session can still do.** Destructive routes need a password
re-entered within a sudo window (5 minutes by default), and that window is
per-session state: a second session cannot ride on the first one's
re-authentication. This is the only control that limits a stolen cookie, because
every other check in the chain is satisfied by the cookie itself. Changing the
password destroys every other session but keeps the one making the change.

**Cross-site request forgery.** Every mutating request must satisfy three
conditions at once: `Content-Type: application/json`, an `X-Holzkube-Manager-CSRF: 1`
header, and an `Origin`/`Sec-Fetch-Site` consistent with the request's own
origin. A cross-origin HTML form can satisfy none of the first two, and a
cross-origin `fetch` that sets them is preflighted — with no CORS headers sent by
holzkube-manager, that preflight has nowhere to succeed, so the request never arrives at
all. `SameSite=Lax` is treated as necessary but not sufficient: it is a browser
default that browsers have relaxed before, and it says nothing about a non-browser
caller.

**DNS rebinding.** The `Host` header is checked against an allowlist — the
loopback names, the bind host and the machine's hostname — on reads as well as
writes. This is not redundant with the CSRF checks: under rebinding, `Host`,
`Origin` and `Sec-Fetch-Site` are all attacker-supplied and all agree with each
other, so the three preconditions pass and the request reaches the handler with
the victim's cookie. Nothing else validates that the host named is one of ours.

**Injection into the UI.** A Content-Security-Policy whose `script-src` carries
the hash of the one inline script this build actually contains, computed from the
served bytes rather than written down — no `unsafe-inline` for scripts.
`frame-ancestors 'none'`, `base-uri 'none'`, `form-action 'none'`,
`object-src 'none'`, plus `X-Frame-Options: DENY`, `X-Content-Type-Options:
nosniff` and `Referrer-Policy: no-referrer` on every response including static
assets. HSTS is sent only over an actual TLS connection. The UI loads no
third-party assets and `default-src 'self'` would refuse them if it tried.

**Other accounts on the same host.** The data directory is `0700` and its
contents `0600`. The store walks it at startup and refuses to start if anything
is group- or world-accessible, or if anything in it is not a regular file or
directory. It reports every violation at once and repairs nothing: silently
chmod-ing an operator's files would hide the fact that they were exposed, and the
window in which they were exposed is the thing worth knowing. A `flock` keeps two
instances off one directory.

**Quiet tampering with the record.** Every mutating request writes two records —
the intent before, the outcome after — chained by
`hash_n = sha256(hash_{n-1} || canonical_json(record_n without its hash))` from a
domain-separated genesis anchor. The chain is verified at startup rather than
behind a button, and a break is surfaced through `GET /api/v1/system/status` and
as a banner in the UI. No code path rewrites, repairs or deletes a record. If the
intent cannot be made durable, the request is refused rather than performed
unlogged.

**Secrets in the log that is kept forever.** Audit parameters go through an
allowlist, not a denylist: anything not explicitly named is replaced by
`<redacted>`, so a field added later is redacted by default rather than leaked by
default. Session tokens are truncated to an 8-character correlation handle before
they are written.

**Leaks through error responses.** A 500 carries a request id and nothing else —
no Go error string, no filesystem path, no store-internal message — and the real
error goes to the server log under the same id. The audit-chain status returns a
file name rather than a path, because that endpoint answers before
authentication and the absolute path would name the OS user and their home
directory layout.

**Cheap resource exhaustion.** Request bodies are capped at 64 KiB, headers at
64 KiB, and read, write and idle timeouts bound a connection that dribbles a
request or never reads its response. A login delay longer than 500 ms is answered
with a 429 and a `Retry-After` rather than by parking the connection.

### What it does not defend against

**Anyone who can read the data directory.** This is the headline. It holds the
password hash, the live sessions, the TLS key and — from phase 2 — the cluster CA
private keys, which are enough to mint an admin `talosconfig` and wipe every
machine in the cluster. There is no encryption at rest in this version. The
honest mitigation is full-disk encryption plus host hygiene, and the host running
holzkube-manager is inside the cluster's trust boundary whether or not it is treated that
way.

**A compromised host, root, or anything running as the same OS user.** Such an
attacker can read the files, the process memory and the browser's cookie store.
No control in this repository survives that, and none claims to.

**Forgetting the password.** There is one account, no password reset, no recovery
e-mail and no second operator to ask. The only way back is filesystem access to
the data directory and a fresh setup. That is deliberate and is not a gap to be
filled: any in-band recovery path in a single-operator tool is a second way in.

**A weak password.** The only policy is a 12-character minimum — no composition
rules, no strength meter, no breach-list check. Whatever margin exists comes from
argon2id's calibrated cost and the login delay, not from the policy.

**Tamper-*proofing* of the audit log.** The chain is tamper-*evidence*. Anyone who
can write to the data directory can rewrite every record and recompute the whole
chain, and it will verify. Shipping records off-box is the only real answer and
is not in this version. Startup verification also covers a window — the current
day's file and the one rotated before it — not the entire archive; the archive is
never pruned, so a full verification is possible by hand but is not run for you.

**A compromised browser.** An extension with access to the page, or a hijacked
browser profile, sits inside every control listed above.

**General denial of service.** Body caps, header caps and timeouts exist, and the
login path is delayed, but there is no global rate limit. A caller who can reach
the port can keep it busy.

## Explicitly out of scope

These are closed without a fix. Reporting them is not wasted effort, but it will
get this section as the answer.

1. **Anything that begins with "given a copy of the data directory", "as root on
   the host", or "as the same OS user".** That is the documented blast radius,
   stated plainly in the README, not a finding. The data directory is equivalent
   to root on every managed node by design; there is no partial-credential design
   that avoids it, because generating machine configuration genuinely requires
   the CA key.
2. **The self-signed certificate warning itself.** It is expected, it is
   documented, and the fingerprint is logged in the browser's own format so that
   the warning can be *verified* rather than dismissed. "The certificate is not
   trusted by default", "the browser shows a warning" and "the certificate is
   self-signed" are the design, not bugs. `--tls-cert` and `--tls-key` are there
   for anyone who wants a different answer.
3. **That the console is reachable over the network when the operator binds it
   there.** It is warned about at every start. A genuine flaw that happens to be
   reachable that way is very much in scope; the reachability alone is not.
4. **Missing headers or hardening on responses where they change nothing**, and
   scanner output with no working scenario attached. Attach the scenario and it
   becomes a report.
5. **Weaknesses in what holzkube-manager does not do yet.** Phase 1 is the foundation
   only, with no Talos interaction. Design concerns about later phases are
   welcome, and belong in a public issue.
6. **Self-XSS, social engineering, physical access, and denial of service by the
   one operator against their own instance.**
