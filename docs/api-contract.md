# holzkube-manager API contract

**This document is binding.** The five wave-2 plans of phase 1 work against it
in parallel without further coordination, and later phases extend it rather
than reinterpret it. Where this document and an implementation disagree, the
disagreement is a bug in one of them — not a matter of taste.

All API routes live under `/api/v1/`. Requests and responses are JSON. Errors
are always RFC 9457 `application/problem+json`.

## Clients

This API has two clients, and both are in this repository:

- **the web UI** (`web/src/`), which signs in with a session cookie and is
  bound by the CSRF contract below. `web/src/api.ts` is the only place in it
  that calls `fetch`, which is what keeps the contract from being forgotten at
  one call site out of thirty;
- **`holzkubectl`** (`cmd/holzkubectl/`), which authenticates with a
  service-account bearer token and is therefore exempt from the CSRF contract
  and from the sudo window — see *Service accounts*.

Both are clients in the strict sense and neither reimplements a rule. Every
verdict either of them shows was reached here. A second implementation of a
rule is a second thing to keep in step, and the two disagreeing is worse than
one of them not existing.

That is a property to hold rather than a hope: a client decoding a key this API
does not send does not fail, it prints a zero. `cmd/holzkubectl/wire_test.go`
holds every name that tool decodes against the server type that produces it,
because two of its columns were wrong that way on the first attempt and both
looked like answers.

## Error Taxonomy

The taxonomy is **closed and stable**. Every `type` is an absolute URI under
`urn:holzkube-manager:problem:`; `about:blank` is never used. Every response
carries a `code`, a machine token that is finer-grained than `type` and that
clients may branch on. **Codes never change.** They are the contract.

| type suffix | HTTP | code prefix | when |
|---|---|---|---|
| `validation` | 400 | `validation.*` | body or query violates the schema; `errors[]` carries the field path and reason |
| `unauthenticated` | 401 | `auth.*` | no, expired or rejected session; **identical response for an unknown username and a wrong password** |
| `csrf` | 403 | `csrf.*` | the CSRF preconditions are not met |
| `forbidden` | 403 | `forbidden.*` | session is valid, the action is not permitted |
| `not-found` | 404 | `notfound.*` | the resource does not exist |
| `method-not-allowed` | 405 | `method.*` | the path exists, the method does not |
| `conflict` | 409 | `store.conflict`, `setup.already-completed` | `rev` CAS clash, or setup is already complete |
| `unsupported-media-type` | 415 | `media.*` | mutating route without a JSON body |
| `sudo-required` | 428 | `sudo.required` | destructive route, sudo window expired or never opened |
| `rate-limited` | 429 | `ratelimit.*` | login delay is in effect; `Retry-After` is set |
| `internal` | 500 | `internal.*` | unexpected failure; **only `instance`, never a detail** |
| `upstream` | 502 | `upstream.*` | a dependency outside this process did not answer, or answered with a refusal |
| `setup-required` | 503 | `setup.required` | no operator account exists yet |

`upstream.node-*` names a Talos node: `upstream.node-unreachable` for a refused,
dropped or unresolvable connection, `upstream.node-timeout` for a node that
accepted the connection and then did not answer in time.
`upstream.factory-*` names the Image Factory at `factory.talos.dev`:
`upstream.factory-unavailable` when it did not answer or answered 5xx, and
`upstream.factory-rejected` when it answered and the answer was a refusal —
the first is retryable, the second is not.

`upstream` exists because `internal` carries no detail by contract. Without it,
an unreachable node and an unreachable Factory are both anonymous 500s in an
archive that D-16 never deletes, and the operator is shown a request id for a
failure that was never holzkube-manager's.

### What a `type` is, and what it is not

A `type` is an **identifier**. Clients match on it; nothing fetches it. It is
deliberately **not dereferenceable** — there is no page at that address, and
none is promised. That is a property a URN makes obvious and an https URL
actively misrepresented, which is why the taxonomy is rooted at a URN and not at
a vendor domain.

It is also **deployment-independent**. Every installation of holzkube-manager emits the
same thirteen types, and the base is not configurable: not by flag, not by
environment variable, not by build tag. A per-deployment base was considered and
rejected, because two installations emitting different `type` values for the
same error would force a per-install special case into every third-party client
— solving a problem nobody has, since nothing fetches the URI, at the cost of the
one property the field has.

The namespace identifier is not registered with IANA; RFC 9457 asks for a URI,
not a registered URN namespace, and the value is an opaque identifier either way.

### Response shape

```json
{
  "type": "urn:holzkube-manager:problem:conflict",
  "title": "Request conflicts with the current state",
  "status": 409,
  "detail": "An operator account already exists. Setup can only run once.",
  "instance": "/requests/59776aad9c9ec84772e0fa3e8dff2be6",
  "code": "setup.already-completed"
}
```

- `instance` is always `/requests/<request-id>`, and the same id is on the
  response as `X-Request-Id` and in the server log. It is unique per request:
  two concurrent failures of the same kind never share one.
- `errors[]` appears only on `validation`, as `[{"field": "...", "reason": "..."}]`.

### Two rules with teeth

1. **`internal` leaks nothing.** No Go error string, no filesystem path, no
   stack trace, no store-internal message ever reaches the client — the client
   gets `instance` and nothing else, and the real error goes to the log under
   the same id. This binary holds cluster PKI; a passed-through internal string
   is free reconnaissance. Enforced by `TestProblemInternalLeaksNothing`.
2. **401 is indistinguishable.** An unknown username and a wrong password
   produce byte-identical responses. `Unauthenticated()` takes no arguments
   precisely so no caller can vary it.

### Currently unreachable entries

`unsupported-media-type` and `forbidden` are minted but not yet emitted:
non-JSON mutating requests are rejected by the CSRF preconditions at 403 before
any handler inspects the body, and phase 1 has a single operator with no
permission model. They exist now because the taxonomy is a closed contract that
wave 2 codes against; adding an entry later would be a contract change.

`upstream` is minted in phase 2 wave 1 for the same reason and is likewise not
yet emitted: no route reaches a Talos node or the Image Factory until the
transport handlers (plan 02-05) and the schematics handlers (plan 02-06) land.
All four codes are reserved now so those two plans reference the same tokens.

## Routes

`Destructive` is the declarative marking from D-06. `RequiresSession` declares
that a live session is required. Both are read by middleware — nothing pattern
matches on the URL.

| Method | Path | Destructive | RequiresSession | Request | Response |
|---|---|---|---|---|---|
| `GET` | `/api/v1/system/status` | false | false | — | `200` system status |
| `POST` | `/api/v1/setup` | false | false | `{"username","password"}` | `201 {"id","username"}`, sets session cookie |
| `POST` | `/api/v1/auth/login` | false | false | `{"username","password"}` | `204`, rotates the session |
| `POST` | `/api/v1/auth/logout` | false | true | `{}` | `204` |
| `GET` | `/api/v1/auth/me` | false | true | — | `200 {"id","username","dry_run"}` |
| `GET` | `/api/v1/audit` | false | true | query, see below | `200` audit page |

Added by plan 04, listed here so wave 2 can code against them today:

| Method | Path | Destructive | RequiresSession | Request | Response |
|---|---|---|---|---|---|
| `POST` | `/api/v1/auth/sudo` | false | true | `{"password"}` | `204`, opens the sudo window |
| `POST` | `/api/v1/account/password` | **`Destructive: true`** | true | `{"current_password","new_password"}` | `204` |

`POST /api/v1/account/password` is `Destructive: true` because it changes the
credential guarding cluster PKI. It therefore requires an open sudo window and
answers `428` `sudo.required` otherwise.

### Status codes on the mutating path

A mutating request is checked in this order, and the first failure wins:

```
CSRF preconditions   -> 403 csrf.precondition-unmet
session required     -> 401 auth.unauthenticated
audit intent durable -> 500 internal.unexpected   (request is refused, not performed)
sudo window          -> 428 sudo.required
handler validation   -> 400 validation.failed
handler conflict     -> 409 store.conflict
```

The audit step is fail-closed on purpose: a mutation that cannot be recorded is
not performed. An unlogged mutation is the outcome the audit log exists to
prevent.

The audit step sits **after** the session check and **before** the sudo check,
and both halves of that placement are deliberate.

It is after the session check because the archive is append-only and kept
forever (D-16). Recording denials that an anonymous caller can provoke would
let one grow the archive on any mutating route with no session, no CSRF header
and no rate limit in front of it. So `403 csrf.precondition-unmet` and
`401 auth.unauthenticated` are **not** recorded.

It is before the sudo check so that `428 sudo.required` **is** recorded, as an
`attempt` followed by an `error` outcome carrying the `sudo.required` code. That
refusal means somebody holding a session cookie tried a destructive action and
could not produce the password, which is the single highest-signal event in the
threat model (T-01-25); it is only reachable by an authenticated caller, so
recording it costs nothing an attacker can spend.

### The sudo window, as a client sees it

- **Logging in does not open the window.** A fresh session reaching a
  destructive route gets `428` `sudo.required`, exactly as an old one does. The
  window is opened only by `POST /api/v1/auth/sudo`, and it belongs to that one
  session — a second browser logged in as the same operator is unaffected.
- The window is **5 minutes** by default (`--sudo-window`, D-05).
- **Every successful destructive action restarts it**, so a series of them asks
  for the password once rather than once per action.
- A failed destructive action does not restart it.

The client flow is therefore: call the action; on `428`, prompt for the
password; `POST /api/v1/auth/sudo`; on `204`, retry the original call. A wrong
password there is `401`, not `428` — the session is fine, the credential was
not.

### Login and re-authentication are rate limited

`POST /api/v1/auth/login` and `POST /api/v1/auth/sudo` share one delay counter
per source IP (D-08).

- The first failure is not delayed. Each further one doubles the wait, from
  250 ms, stopping at 30 seconds.
- A short wait is served by holding the request; anything longer answers `429`
  `ratelimit.delayed` with `Retry-After` in seconds.
- A successful attempt clears the counter.
- **There is no lock and therefore no unlock.** Waiting is always sufficient;
  no endpoint, flag or recovery path exists to clear the delay, and none may be
  added. Clients should show the `Retry-After` value and let the operator wait.

## CSRF Contract

Every mutating request (anything other than `GET`, `HEAD`, `OPTIONS`, `TRACE`)
must satisfy **all three** conditions simultaneously. Failing any one gives
`403` with code `csrf.precondition-unmet`.

1. `Content-Type: application/json`, optionally with parameters
   (`; charset=utf-8` is accepted).
2. `X-Holzkube-Manager-CSRF: 1` — the value is checked, not just the header's presence.
   Any other value is a refusal, so a client cannot drift to `true` and only
   discover it later.
3. An `Origin` / `Sec-Fetch-Site` consistent with our own origin:
   - if `Sec-Fetch-Site` is present it must be `same-origin` or `none`
     (`same-site` is refused: a sibling subdomain is not us);
   - if `Origin` is present, its host must equal the request `Host` **and** its
     scheme must match the request's. An `http://` origin on an `https://`
     request is a refusal.
   - Absence of both is permitted: non-browser clients send neither, and a
     browser cannot reach conditions 1 and 2 cross-origin anyway.

A cross-origin HTML form can satisfy neither (1) nor (2) — both are outside the
CORS "simple request" envelope, so the browser preflights and the request never
arrives. `SameSite=Lax` on the cookie is necessary but not sufficient by itself.
No token plumbing is required; a double-submit token can be layered on later.

### Host allowlist

Condition (3) compares the `Origin` against the request `Host`, and both come
from the caller, so on its own it is self-referential. Under DNS rebinding — the
standard attack against a loopback-bound admin tool — a victim's browser
resolving `evil.example` to `127.0.0.1` sends `Host: evil.example`,
`Origin: https://evil.example` and `Sec-Fetch-Site: same-origin`, because from
the browser's point of view it *is* same-origin. All three conditions pass.

Every request therefore also has its `Host` checked against the addresses this
instance answers to — the bind address plus `localhost`, `127.0.0.1`, `::1` and
the machine's hostname, which is the same set that goes into the generated
certificate's SANs. A `Host` outside it gives `403` with code `forbidden.host`.

The check covers reads as well as mutations: a rebound `GET /api/v1/audit` is a
leak of the archive, and only the mutating path goes through the three
conditions above.

**Client obligation:** set all three on every mutating call. In the web UI
`web/src/api.ts` is the only place that calls `fetch`, which is what keeps this
from being forgotten at one call site out of thirty.

## Audit Query Contract

```
GET /api/v1/audit?from=<RFC3339>&to=<RFC3339>&action=<token>&limit=<n>&cursor=<seq>
```

All parameters are optional. Sorting is **newest first**, always. The filters
are implemented and applied.

| Parameter | Form | Meaning |
|---|---|---|
| `from` | RFC 3339 | records at or after this instant |
| `to` | RFC 3339 | records at or before this instant |
| `action` | dotted token, `[a-z0-9._-]`, ≤ 64 | **exact** match on the action; a prefix does not match |
| `limit` | positive integer | page size; default `100`, silently capped at `1000` |
| `cursor` | positive integer | the `next_cursor` of the previous page; `0` is rejected |

A malformed parameter is `400` `validation.failed` with the offending field in
`errors[]` — never a silently different answer to a question nobody asked. A
`limit` above the ceiling is served at the ceiling rather than refused; a query
against an archive that is never shortened (D-16) must not be able to pull a
year of records into memory.

The cursor is the `seq` of the last record on the previous page, and the next
page continues strictly below it. A sequence number rather than an offset, so
pagination neither overlaps nor skips while records are being appended.

```json
{
  "items": [ { "seq": 4, "ts": "...", "actor": "holz", "action": "auth.login", "outcome": "success", "...": "..." } ],
  "next_cursor": null
}
```

### The exhaustion rule — both obligations, stated literally

This is pinned here because the producer (plan 03) and the consumer (plan 05)
would otherwise each invent it and agree only by luck.

- **Server obligation.** `next_cursor` is **always present** in the JSON. It
  carries either a number or the value `null`. `null` means exactly "there is
  no further page". The value `0` is **never** a valid cursor and is never
  sent. The field is **never omitted**.
- **Client obligation.** The client checks `next_cursor !== null` to decide
  whether another page exists. It must **not** test the value for truthiness.
  A falsy check would treat `0` as exhaustion and would be correct only by the
  accident of which sentinel was chosen.

### Record shape

The field set is closed (D-14) and is part of the hash chain: `seq`, `ts`,
`actor`, `session`, `src_ip`, `cluster_id`, `machine_id`, `action`, `params`,
`job_id`, `outcome`, `prev_hash`, `hash`. `outcome` is one of `attempt`,
`success`, `error`.

Every mutating request writes two records: the intent before the action and the
outcome after. The outcome repeats the intent's identifying fields; its `params`
carries only `{"error": "..."}` on failure, and is empty on success, because the
input parameters were already recorded by the intent. The `error` value is the
**stable taxonomy `code`**, never a Go error string or a path — this file is
kept forever, and an internal message in it is free reconnaissance.

`params` on the intent is the request body **after allowlist redaction** (D-14).
A field that is not explicitly permitted for that action appears as the literal
`"<redacted>"`; the key survives so the record shows what was sent, the value
does not. An unknown nested object is replaced whole rather than walked. The
allowlist lives in `internal/audit/redact.go`; adding a parameter to a later
action requires adding it there on purpose. There is no list of forbidden
fields, because such a list forgets the next secret.

`params` also carries `request_id`, the same id as `X-Request-Id` and as the
`instance` of any problem response — the record's field set is closed, so
server-side context belongs in `params` rather than in a new column.

`session` is a **truncated** token, not the live one: a log kept forever must
not be a store of every session that ever existed.

### Actor vocabulary

`actor` is normally the signed-in operator's username, or the literal
`anonymous` for a request that carried no session. Those are the only two shapes
any Phase 1–5 record has, because the audit middleware fills the field from the
request context and every writer so far is an HTTP request.

Two further tokens are **reserved now and written by nothing yet**:

| Token | Meaning |
|---|---|
| `system` | a mutation the process itself initiated, with no request and no signed-in operator behind it |
| `job:<id>` | a mutation the jobs engine performed, where `<id>` is the job's identifier |

They are fixed here rather than when the first non-HTTP writer arrives, and the
reason is the hash chain. `actor` is one of the canonical fields, so its value
is hashed into every record that follows it. Changing the vocabulary once a
second writer already disagrees with the first forces either a break in the
chain at the seam or a rewrite of the whole archive — and the archive has
unlimited retention and no deletion path (D-16). Deciding while exactly one
writer exists costs nothing; deciding later costs the archive.

`job:<id>` carries the identifier deliberately. "A job did this" is not enough
for a post-mortem; "which job did this" is, and the prefix is what keeps the
value distinguishable from a username without changing the field's type.

**`system` is refused as an operator username.** It is the one token whose flat
shape could collide with a real account, and a record whose `actor` is ambiguous
between "the process" and "a person" is a repudiation risk in a log that is kept
forever. `job:<id>` needs no such rule: `:` is not a legal username character.

An intent with **no** matching outcome means the process did not survive the
action. It is a finding and is left standing as one; nothing completes the pair
after the fact, and the record remains findable through this query API.

Files rotate daily to `audit/audit-<YYYY-MM-DD>.jsonl`. From the second day on
a rotated file is gzipped in place to `audit-<YYYY-MM-DD>.jsonl.gz`; both forms
read identically through the query API. No file is ever removed, and there is
no option that would remove one (D-16). The chain runs **across** the file
boundary: the first record of a new day carries the last hash of the previous
day. The first record of a fresh data directory carries a defined genesis
anchor, not an empty string.

**Adding, removing or renaming a field is not a compatible change.** Rotated
files are kept indefinitely (D-16), so every record ever written must stay
verifiable. The chain is
`hash_n = sha256(hash_{n-1} || canonical_json(record_n without its hash field))`,
where canonical means keys sorted lexicographically, no whitespace, UTF-8, and
nested `params` normalized explicitly rather than left to Go map ordering.
Verification therefore does not depend on any JSON encoder's output behaviour,
which is what lets the archive survive a Go upgrade or a reordered struct field.

## System Status Contract

```
GET /api/v1/system/status
```

```json
{
  "setup_required": true,
  "audit_chain": { "ok": true, "broken_at_line": 0, "file": "audit-2026-08-28.jsonl" }
}
```

- `setup_required` — no operator account exists yet. While it is true, every UI
  route redirects to `/setup` (D-01).
- `audit_chain.ok` — `false` means the hash chain does not verify.
  `broken_at_line` is the **1-based** line number of the first record that does
  not verify, and is `0` when `ok` is true. `file` names the file checked.
  It is a **file name, never a path**: this endpoint answers before
  authentication, and the audit directory sits under the XDG-resolved absolute
  data directory, so a path would disclose the OS user and their home directory
  layout to an anonymous caller.
- A break found at startup stays reported for the life of the process (D-15).
  While startup was clean the endpoint re-verifies live **for authenticated
  callers**, so damage occurring during the run is not hidden until the next
  restart. An anonymous caller receives the startup snapshot: re-verification
  re-reads and re-hashes the window under the audit writer's mutex, and the
  audit middleware is fail-closed, so serving it unauthenticated would let an
  anonymous caller stall or fail other people's mutations. The live re-verify
  is additionally memoised for 30s, so polling clients do not each pay for a
  full re-read. Startup verification covers the current day's file **and the
  one rotated before it**, so `file` is not necessarily today's.
- There is **no** endpoint and no parameter that acknowledges, clears or
  recomputes the verdict. A break disappears only by dealing with the file by
  hand and restarting. A chain that repairs itself is worse than no chain: it
  destroys the evidence that it was broken.
- The UI renders a break as a persistent banner. It is not dismissible: a hash
  chain nobody looks at is theatre.

## Dry-run

```
GET /api/v1/auth/me
```

```json
{ "id": "...", "username": "holz", "dry_run": false }
```

`dry_run` reports whether this process was started with `--dry-run` (or
`HOLZKUBE_MANAGER_DRY_RUN=true`). It is a statement about the transport, not about the
UI: while it is `true`, every RPC the deadline class table classifies as a
mutation is refused by a gRPC client interceptor on the one connect path both
Talos client types are built on, before the call reaches the wire. Nothing is
hidden and nothing is merely disabled in the interface — no mutation reaches a
node, and a test enumerates the whole mutation class and asserts a zero
server-side call counter for every method in it (FOUND-12, D-03).

Reads and streams are unaffected: the mode disables mutations, not the product.
Maintenance mode is not an exemption — `ApplyConfiguration` on an unconfigured
node is refused like everything else, because it is the most consequential
mutation the product performs.

The scope is deliberately the Talos wire and nothing else. A schematic created
while dry-run is on still writes a record to the store and to the audit archive,
and still contacts the Image Factory. `--dry-run` is about what reaches a
**node**.

A mutation refused by dry-run answers **403 `forbidden.dry-run`**. It is its own
code rather than a generic refusal because the remedy is not on the operator's
side at all: nothing they can type will make it go through, and the instance has
to be restarted without the flag. A client that showed it as an ordinary
permission error would send somebody looking for a role to grant.

The field is on this endpoint and **not** on `GET /api/v1/system/status`, which
answers before authentication. Whether an instance can currently change anything
is not something an anonymous caller is owed, and the operator this field exists
for is signed in by definition. The UI renders it as a banner with no dismiss
control, for the same reason the audit-chain banner has none.

## Route Registration Rule

A wave-2 plan adds a route **without touching `router.go`**:

1. Add the handler and its `httpapi.Route` entry to **your own file** under
   `internal/httpapi/handlers/`, returned from that file's `…Routes(deps)`
   function.
2. If the file is new, add its `…Routes(deps)` call to the `slices.Concat` in
   `cmd/holzkube-managerd/main.go` — one line.
3. If the handler needs a dependency the composition root already builds, add
   **one field** to `Deps` in `router.go`, with a doc comment in the style every
   other field there has. That field is the third and last permitted `router.go`
   edit. A `Route` entry still belongs in the handler file and never in
   `router.go`.

Set the new dependency **inside** the `httpapi.Deps{…}` literal in
`cmd/holzkube-managerd/main.go`, never afterwards. `Deps` is copied by value into every
`…Routes(deps)` call, so a field assigned after the literal is the zero value
inside each handler closure — a nil dependency with no compile error and no
failure until the first request.

`router.go` owns the `Route` type, the mounting and the middleware wiring. After
phase 1 it is edited only to add a `Deps` field: nothing adds a `Route` literal,
a mounting change or a middleware change there. Two plans adding routes never
touch the same lines.

**Every mutating route must set `Destructive` deliberately, and `Action`
always.**

- A mutating route that can destroy something and is not marked
  `Destructive: true` bypasses the sudo gate. The flag existing in one readable
  place is what makes that omission visible in review instead of invisible in
  production (D-06). It is binding for phase 6 (node reboot, shutdown, reset)
  and phase 9 (etcd member removal).
- A mutating route with an empty `Action` is skipped by the audit middleware and
  executes with **no record at all**. Treat a missing `Action` as a defect of
  the same class as a missing `Destructive`.

## Schematics

Contract-first. Every route below is **documented here and implemented in plan
02-06**; the derivation, filtering, warning and persistence machinery they sit
on lands in plan 02-04. Nothing in this section is served yet.

A *schematic* is an Image Factory customisation — system extensions, kernel
arguments, META values — identified by the SHA-256 the Factory assigns to its
own canonical rendering of the document. holzkube-manager persists the Factory's
canonical document verbatim, not the input, because the id is the hash of
exactly those bytes.

### The schematic resource

```json
{
  "id": "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba",
  "cluster": "",
  "name": "workers with intel microcode",
  "talos_version": "v1.13.9",
  "arch": "amd64",
  "canonical": "customization: {}\n",
  "extensions": ["siderolabs/intel-ucode"],
  "kernel_args": ["console=ttyS0"],
  "meta": [{"key": 10, "value": "…"}],
  "usable": false,
  "probed_at": "0001-01-01T00:00:00Z",
  "probe_reason": "",
  "created_at": "2026-08-29T11:00:00Z",
  "rev": 1
}
```

- **`usable` is false until the model-build probe agrees.** A successful
  creation never sets it. The Factory accepts a schematic naming an extension
  that does not exist, assigns it an ordinary id, and refuses only when an image
  is requested (FACT-02), so creation and validation are two events and this
  field records the second. A client that renders "created" as success is
  lying to the operator.
- **`probed_at` zero means never probed**, which is not the same as "probed and
  refused". Those are different states and the UI must not merge them.
- **`probe_reason` is what the Factory said when it refused**, and is empty
  otherwise — including when the probe could not reach the Factory at all, which
  says nothing about the schematic. It carries the version, the architecture and
  the status the Factory answered with, because "not usable" with no stated
  cause is a verdict an operator cannot act on: a schematic naming an extension
  that does not exist and one asked for at a version that never had it are the
  same red badge and two different repairs.
- **`arch` is the architecture the schematic was authored and probed against.**
  It is what `usable` and `probe_reason` are statements about — the probe asks
  the Factory for one architecture's image, so a stored verdict that does not
  name one cannot be read back. An **empty value means a record written before
  the field existed**; its verdict cannot be qualified after the fact, because
  the architecture a past probe used is not recoverable from anything else the
  record holds. It is **not a default for the assets query**: see below.
- `rev` is the CAS revision, as on every stored record. A `PUT`-shaped update
  carrying a stale `rev` answers `409` `store.conflict`.

`POST /api/v1/patches` answers it too, on the write that supersedes a parent.
The window is narrow — the handler reads the parent, marks it superseded and
writes it back — and two operators editing one patch at the same time land in
it. It used to answer `500 internal.unexpected`, which tells an operator
something is broken and to stop; what actually happened is that the chain moved
under them and the request can be made again against its head.

### Routes

| Method | Path | Destructive | RequiresSession | Action | Request | Response |
|---|---|---|---|---|---|---|
| `GET` | `/api/v1/factory/versions` | false | true | — | — | `200` version buckets |
| `GET` | `/api/v1/factory/extensions` | false | true | — | `?version=` | `200` extension catalog |
| `POST` | `/api/v1/schematics` | false | true | `schematic.create` | schematic input | `201` schematic + `warnings` |
| `GET` | `/api/v1/schematics` | false | true | — | — | `200 []` schematic |
| `GET` | `/api/v1/schematics/{id}` | false | true | — | — | `200` schematic |
| `GET` | `/api/v1/schematics/{id}/assets` | false | true | — | `?version=&arch=&platform=&secureboot=` | `200` asset references |
| `DELETE` | `/api/v1/schematics/{id}` | **`Destructive: true`** | true | `schematic.delete` | — | `204` |

`DELETE` is `Destructive: true` and therefore behind the sudo window. The
Factory provides **no way to list schematics** — they may carry secrets in
kernel arguments, so it deliberately will not enumerate them — which means a
schematic id that is neither stored here nor readable from a running node is
gone. Deleting a record is deleting the only copy of a reference an upgrade
needs.

### `GET /api/v1/factory/versions`

```json
{
  "stable": ["v1.12.0", "…", "v1.13.9"],
  "prerelease": ["v1.14.0-rc.1", "v1.14.0-rc.2"],
  "newest_stable": "v1.13.9",
  "broken": {"vX.Y.Z": "why this version is listed"}
}
```

- The split is **structural**: a version is a prerelease because semver says a
  hyphen introduces one, not because it appears on a list. The upstream list is
  served ascending and *ends* in the current alpha, beta and rc tags, so
  `newest_stable` is a comparison and never the last element.
- `prerelease` is served rather than hidden, so a UI can offer opting in
  explicitly instead of pretending the versions do not exist.
- `broken` maps a version to **the reason it is listed**. A version greyed out
  with no stated cause is a control an operator cannot judge. The map is
  frequently empty; empty means nothing is currently known broken, not that the
  server did not check.
- All three collections encode as `[]` / `{}` when empty, never `null`.

### `GET /api/v1/factory/extensions?version=v1.13.9`

The catalog is **version-scoped and there is no fallback**. An extension valid
at one Talos version may not exist at another, so a list fetched for the wrong
version produces a schematic that is un-buildable at the moment it is used. A
failure to fetch it is a failure to validate and is reported as `upstream`
`upstream.factory-unavailable`; it is never answered with a cached or unscoped
list, and never with an empty one.

### `POST /api/v1/schematics`

Extension names are validated against the version-scoped catalog **before** any
request reaches the Factory. An unknown name is `400` `validation.failed` with
every unknown name in `errors[]` — all of them at once, not the first.

**Creating the same schematic twice answers `409` `store.conflict`.** The id is
the SHA-256 of the canonical document, so two authoring attempts that share a
customisation are the same schematic however they are named — a second name, a
second cluster and a second Talos version all collide, and so does a browser
reload that re-submits the form. The operator's own text is not replaced: the
record is the only copy of a reference the Factory will not list back, which is
why `DELETE` is `Destructive` and behind the sudo window, and a `POST` marked
`Destructive: false` does not get to overwrite the label, the cluster and the
version of a record that already exists. Read it back with `GET
/api/v1/schematics/{id}`, or delete it and author it again.

**The probe verdict is the exception, and it is a documented side effect of this
`409`.** A re-POST runs the whole authoring sequence, including a fresh probe
that often answers because the first attempt warmed the Factory. That verdict
used to be discarded. It is now written back onto the existing record —
`usable`, `probed_at` and `probe_reason`, and no other field — under a
compare-and-swap on the record's revision. The status code is still `409` and
the body shape is unchanged; what changed is that a failed create can now change
stored state, so a client should refetch the saved list after one.

The refresh happens only when **all three** conditions hold, and the `409` `detail`
says which of the three outcomes the caller got:

| Condition | Meaning |
|---|---|
| the fresh probe answered | `probed_at` is stamped for a success and for a Factory refusal and for nothing else. A probe that could not reach the Factory says nothing about the schematic, and writing its silence over a stored verdict would replace an answer with an absence. |
| the stored `arch` equals the architecture this submission asked about | the verdict is architecture-scoped, and this record's identity cannot vary by architecture. A record written before `arch` existed carries an empty one and is therefore never refreshed. |
| the stored `talos_version` is the one this submission asked about | the verdict is version-scoped for the same reasons the architecture one is: the probe asks exactly one version's ISO URL, the extension catalog is version-scoped, and `probe_reason` names the version and the architecture in a single sentence. And this record's identity cannot vary by version either, because the canonical document contains no version — so a verdict measured at one version, written onto a record that names another, would be unrecognisable afterwards and would erase the refusal reason that was the only place the disagreement was visible. Unlike `arch` there is no old-record exemption: `talos_version` has been required since the first record. |

| `detail` says | Outcome |
|---|---|
| the probe ran again and this schematic builds | refreshed, `usable` now `true` |
| the probe ran again and the Factory refused it, with the reason | refreshed, `usable` still `false`, `probe_reason` written |
| the verdict was not refreshed, and why | left exactly as it was — the probe did not answer either, or the record holds another architecture's verdict, or the record holds another Talos version's verdict |

A refresh that loses the compare-and-swap, or whose record was deleted between
the read and the write, abandons the refresh and answers the plain conflict. It
never retries and never recreates the record.

This is a recovery and not a re-probe route: it is reached by re-submitting a
form and is answered as an error, and a probe that times out again leaves the
record unchanged. There is no endpoint that asks for a verdict.

**A second architecture collides too, and that is the case worth stating
separately.** The canonical document does not contain the architecture —
upstream leaves it out, because a schematic describes what goes into an image
and the architecture is a path segment on the asset URL. So the same
customisation at `arm64` hashes to the id it already has at `amd64` and answers
`409`. The `arch` field on the stored record names which architecture the
`usable`, `probed_at` and `probe_reason` verdict is about; it does not make room
for a second verdict. **One stored customisation holds exactly one
architecture's verdict, and obtaining the other means deleting the record and
authoring it again.** This is also the case the verdict refresh above declines:
a second-architecture `POST` changes nothing at all, and its `detail` names both
architectures so the caller can tell that decline from the other one. **A second
Talos version has the same structure and is declined for the same reason** — the
canonical document contains no version either, so `v1.12.0` and `v1.13.9` of one
customisation are one record, and that record holds exactly one version's
verdict; the `detail` names both versions there, so an operator can tell the
version decline from the architecture one. This is a
recorded constraint rather than a defect — the
reasoning and the decided direction are in
`.planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-schematic-identity.md`.

The `201` body is the schematic resource plus:

```json
{"warnings": [{"code": "schematic.installer-ignores-kernel-args", "detail": "…"}]}
```

`warnings` is always present and is `[]` when there is nothing to say. A `null`
reads to a client as "the server did not check", which is a different and much
weaker statement.

| Warning code | Raised when |
|---|---|
| `schematic.installer-ignores-kernel-args` | the schematic carries extra kernel arguments |
| `schematic.installer-ignores-meta` | the schematic carries META values |
| `installer.repo-fallback-unverified` | the installer repository name was reached past a candidate that never answered, so the preferred name was unheard rather than ruled out. Raised on `GET .../assets`, not on this route. |
| `installer.secureboot-repo-fallback-unverified` | the same, for a SecureBoot request, where the fallback name may be a *different image* rather than another name for the same one. Raised on `GET .../assets`, not on this route. |

The table has four members and the taxonomy is closed: a client may match on
these codes, and no code is added without a row here.

The two `installer.` codes are separate deliberately. `installer-secureboot` is
not reliably another name for `metal-installer-secureboot` — at the pinned Talos
version the two resolve to two different images, and at the oldest supported
version to the same one — so neither "alias" nor "different image" is true of the
pair in general, and the answer is labelled per resolution instead of settled
once. Both candidates behind the SecureBoot code are still SecureBoot
installers: this is a statement about *which* SecureBoot image, never a fallback
to the ordinary one.

The first two exist because of a restriction stated verbatim upstream: *"`installer` and
`initramfs` images only support system extensions (kernel args and META are
ignored)"*. The ISO therefore has them and the installed system does not, and
the machine boots correctly from the USB stick and then installs a subtly
different system with nothing reporting it. The detail text names both affected
images and the remedy — `.machine.install.extraKernelArgs` in the machine
config. **A client must render these; they are the entire mechanism protecting
against that divergence (FACT-04).**

### `GET /api/v1/schematics/{id}/assets`

```json
{
  "iso": "https://factory.talos.dev/image/<id>/v1.13.9/metal-amd64.iso",
  "pxe": "https://factory.talos.dev/pxe/<id>/v1.13.9/metal-amd64",
  "disk_image": "https://factory.talos.dev/image/<id>/v1.13.9/metal-amd64.raw.zst",
  "cmdline": "https://factory.talos.dev/image/<id>/v1.13.9/cmdline-metal-amd64",
  "installer": "factory.talos.dev/metal-installer/<id>:v1.13.9",
  "warnings": []
}
```

- `arch` is a **required** parameter with no default. holzkube-manager is developed on
  `arm64` and targets `amd64`; a defaulted architecture is a bug that only ever
  appears on someone else's machine (FACT-03). The record carrying an `arch` of
  its own does not change this: the record describes what was *probed*, the
  parameter asks what to *build*, and using the description as the default is
  the same FACT-03 bug wearing a record's clothes.
- `secureboot=true` suffixes the platform-architecture segment of every URL
  **and selects the installer repository**. The SecureBoot installer is a
  different image — same schematic, same version, different digest — chosen by
  repository name alone, and Talos requires it for a SecureBoot install: it
  carries the signed UKI and systemd-boot, and there is no machine-config flag
  that substitutes for it. A SecureBoot request therefore answers with
  `metal-installer-secureboot` (or the legacy `installer-secureboot`), and the
  five references in one response are all SecureBoot or all not:

  ```json
  {
    "iso": "https://factory.talos.dev/image/<id>/v1.13.9/metal-amd64-secureboot.iso",
    "pxe": "https://factory.talos.dev/pxe/<id>/v1.13.9/metal-amd64-secureboot",
    "disk_image": "https://factory.talos.dev/image/<id>/v1.13.9/metal-amd64-secureboot.raw.zst",
    "cmdline": "https://factory.talos.dev/image/<id>/v1.13.9/cmdline-metal-amd64-secureboot",
    "installer": "factory.talos.dev/metal-installer-secureboot/<id>:v1.13.9"
  }
  ```

  If neither SecureBoot name resolves the route answers `200` with `"installer":
  null` and an `installer_error`, exactly as it does when neither ordinary name
  resolves — the four other references are still returned. It does **not** fall
  back to the ordinary installer: a SecureBoot ISO paired with an installer that
  does not produce a SecureBoot node is the drift the resolution exists to
  prevent, and because `secureboot` is a query parameter that never reaches the
  stored record, a substitution here would be undetectable from that point on by
  anything — no log, no audit entry, no later re-derivation.
- **The route is not atomic, and that is deliberate.** Once the request itself is
  valid, the `iso`, `pxe`, `disk_image` and `cmdline` references are **always**
  returned. They are pure string assembly over the request — schematic id,
  version, architecture, platform, SecureBoot flag — and nothing that builds them
  touches the registry, so nothing an upstream does can make them wrong or
  unavailable. `installer` is the one field the registry can withhold, and
  withholding it withholds only itself.

  A request that fails *before* that point is a different thing and answers the
  way it always has, with a problem and no body: an unknown schematic is `404`,
  and a missing or unserved `arch` or platform is `400` `validation.failed`.
  There are no references to return, because there is no valid request to derive
  them from.
- `installer_error` is present **when and only when `installer` is null**, and is
  absent otherwise. It carries the `code` and `detail` of the problem the route
  would previously have answered with:

  ```json
  {
    "installer": null,
    "installer_error": {
      "code": "upstream.factory-unavailable",
      "detail": "The Image Factory did not answer usably: resolving the installer image reference for v1.13.9."
    }
  }
  ```

  Branch on `code`, not on the presence of the member alone: the two reasons have
  opposite remedies. `upstream.factory-unavailable` means the registry did not
  answer — asking again may succeed, and this is the common case, because
  `factory.talos.dev` throttles without producing an HTTP response at all.
  `upstream.factory-rejected` means it did answer and no candidate carries a
  manifest — this version has no installer under the requested name, and no
  number of retries changes that. The same two tokens carry the same two meanings
  here as they do in the Upstream failures table below.
- `warnings` is always present and is `[]` when there is nothing to say, in the
  same shape and under the same name as the `201` body's — a client has one
  warning shape to learn rather than two. Unlike the `201` body's warnings, these
  are about *this resolution* rather than about the schematic: nothing persists
  them, and they cannot be recomputed from the record later.

  What `warnings` means depends on whether there is a reference for it to be
  about, and a client must not read the empty array as proof on its own:

  | `installer` | `installer_error` | `warnings` | Means |
  |---|---|---|---|
  | a reference | absent | `[]` | **Proven** — the preferred repository name answered. |
  | a reference | absent | one entry | **Provisional** — usable, but the preferred name was never ruled out. The detail says which name did not answer. |
  | `null` | present, `upstream.factory-rejected` | `[]` | **Refused** — every candidate answered and none carries a manifest. Nothing to be provisional about. |
  | `null` | present, `upstream.factory-unavailable` | `[]` | **Unresolved** — the registry did not answer. Retryable. |

  So `warnings: []` means "proven" only when `installer` is non-null. When
  `installer` is null it means "there is nothing to warn about", which is why the
  null and not an empty string is what carries the news.
- **`installer` is resolved against the registry, never assembled.** The
  repository name is version-dependent: for part of the supported range only the
  legacy `installer` name answers, for the rest the platform-prefixed
  `metal-installer` does. The reference is consumed by the upgrade RPC, and a
  wrong one produces an upgrade that reports success while silently dropping
  every system extension the node was built with.

  There are four outcomes, not three:

  1. **Proven** — the preferred candidate answered. The reference is returned
     with `warnings: []` and no `installer_error`.
  2. **Provisional** — a candidate failed at the transport level and a later one
     answered, so the preferred name was never actually ruled out. The reference
     is returned *and* carries `installer.repo-fallback-unverified` — or, for a
     SecureBoot request, `installer.secureboot-repo-fallback-unverified`, which
     adds that the name that answered may select a *different image* rather than
     the same one — naming the repository that did not answer, the version and
     the transport error. It is
     usable; asking again once the registry is reachable may produce a different
     reference. The fallback is deliberate — `factory.talos.dev` is known to
     throttle without producing an HTTP response at all, and refusing here would
     leave an operator unable to read their own asset URLs because a third party
     was busy.
  3. **Refused** — every candidate answered and none carries a manifest. `200`
     with `"installer": null` and `installer_error.code`
     `upstream.factory-rejected`. There is no guessed fallback, because a guess
     here is the failure the whole resolution exists to prevent.
  4. **Unresolved** — the registry did not answer at all, or answered something
     that says nothing about this schematic (a 5xx, a rate limit, an
     authentication challenge, a dropped connection). `200` with `"installer":
     null` and `installer_error.code` `upstream.factory-unavailable`. Retryable,
     and reached far more often than 3.

  Outcomes 3 and 4 answered `502` with no body at all until 2026-08-30. That was
  never the intent recorded here — this section described them as returning "no
  installer reference", which is what they now do — and the four references the
  old behaviour discarded had never depended on the registry in the first place.
  A client that treated a `502` from this route as "no assets" should treat a
  `200` with a null `installer` the same way for the installer *alone*.
- **How long the route may take, and who enforces it.** The installer resolution
  is the one part of this route that leaves the process. Every candidate
  repository name is asked at the same time rather than one after the other, and
  the whole resolution runs under a **35-second server-side deadline**. A request
  that does not get an answer inside it is cut by holzkube-manager, not by the
  client and not by the socket: the response is a `200` carrying the four
  locally assembled references, `"installer": null` and an `installer_error` with
  `upstream.factory-unavailable` — outcome 4 above, on the table just above this
  list.

  Read that number as a **budget, not a prediction**. It is what the server will
  spend before it gives up; it is not a claim about how long `factory.talos.dev`
  takes, which is somebody else's build farm and is known to throttle. A typical
  cold resolution is well inside it and a warm one is served from an in-process
  cache without touching the registry at all. What the number guarantees is the
  other end: this route will answer, with a body, rather than holding a
  connection open until something else times out.

### Upstream failures

Every route in this section reaches `factory.talos.dev`. A failure there is
reported with the `upstream` problem type at `502`, using the tokens reserved in
the taxonomy above:

| Code | Meaning |
|---|---|
| `upstream.factory-unavailable` | the Factory did not answer, answered 5xx, or answered something holzkube-manager will not decode. **Retryable.** |
| `upstream.factory-rejected` | the Factory answered and the answer was a refusal. **Not retryable**; the request or the schematic is wrong. |

A schematic the Factory refuses to build is `upstream.factory-rejected`; a
Factory that did not answer while probing is `upstream.factory-unavailable`.
Merging them would send an operator to fix a schematic that is not broken.

**Which status is which is fixed, and it is the same rule everywhere holzkube-manager
reads a Factory or registry answer** — the ISO probe and the installer manifest
resolution included:

| Status | Code | Why |
|---|---|---|
| `400`, `404` | `upstream.factory-rejected` | The Factory answered *about this schematic*. `404` is "no manifest under that name"; `400` is what it returns when an extension the schematic names is not available at the requested version. Both are reproducible and neither changes on a retry. |
| `401`, `403`, `429`, every `5xx`, no answer at all | `upstream.factory-unavailable` | The Factory declined to answer *us*, or failed to. An authentication challenge, a policy refusal, a rate limit and an outage say nothing about the schematic. |

**A rate limit is deliberately on the retryable side.** `factory.talos.dev`
throttles, and has been observed doing so without an HTTP response at all. The
probe verdict is written once, when the schematic is created, and there is no
re-probe path — so a `429` recorded as a refusal becomes a permanent,
unclearable accusation against a schematic nothing ever found fault with. The
cost of the other side of that trade is that more records finish creation with
`probed_at` unset, which is `never probed` and is what the contract already
requires a client not to merge with `probed and refused`.

**A Factory that assigns an id holzkube-manager did not compute is also
`upstream.factory-rejected`, and `POST /api/v1/schematics` still stores the
record before answering it.** The two halves are one rule. The Factory answered
and a retry reproduces the identical mismatch — the canonical serialisations
have drifted, and nothing about asking again changes that — so calling it
retryable would have the operator orphan a second schematic on every attempt.
And the schematic does exist upstream, under an id the Factory chose; since the
Factory will not enumerate schematics, the stored record is the only place that
id can ever be read back from. So the record is written, `usable` is `false` and
`probed_at` is zero because no probe ran, and the response is the `502` rather
than a `201` — drift in the mechanism that lets holzkube-manager know a schematic's id
without a round trip is not something to report as success.

**A value holzkube-manager's own serialiser will not render is a `400`, not a `502`.**
This is the exception the paragraph opening this section swallows. Before any
request is made, `POST /api/v1/schematics` computes the schematic id locally, and
that computation refuses a scalar it cannot render the way the Factory would.
No request reaches `factory.talos.dev`, nothing is known about the Factory, and
no retry can succeed, so the answer is the `validation` problem type at `400`
with code `validation.failed`. The fields that can produce it are `name`,
`cluster`, `kernel_args`, `meta` and `extensions` — every operator-supplied
string the request carries, checked against one predicate rather than five
transcriptions of it. A document path this handler does not recognise still
answers `400`, with no field named, rather than falling back to a `502`.

**`name` and `cluster` were added to that list in plan 02-20, and a client
written before it may see a `400` where it previously saw a `201`.** Both were
accepted unchecked: a `POST` carrying a NUL and a right-to-left override in
`name` answered `201`, stored both verbatim, and rendered the override in the
saved table. `cluster` was not read at all. The two are stored and rendered, so
they are held to the same rule as the fields that reach the document — a value
that cannot survive serialisation cannot survive being shown either. Nothing
about the accepted set changed: the refusal is the table below, and text in any
language, including the bidirectional controls `U+200B` and `U+202E`, is
accepted exactly as it was.

**Every bad field is reported in one answer.** A body with a refused `name`, a
refused `cluster` and a refused `kernel_args` entry produces three field errors,
not the first one — fixing a form should cost one round trip, not three.

**What is refused, stated as rules rather than as a list of characters:**

| Rule | Refused | Why |
|---|---|---|
| not valid UTF-8 | any byte sequence that is not | the document is UTF-8; an unpaired surrogate has no encoding |
| control characters | `U+0000`–`U+001F`, `U+007F`–`U+009F` | the Factory does not write them literally. It escapes most of them, which moves the id; `U+0085` it also *folds*, replacing it with a space inside a quoted scalar and eating it at the end of a plain one |
| line separators | `U+2028`, `U+2029` | both are printable to YAML and both are read as line breaks by the Factory. A plain scalar carrying one becomes a document the Factory answers `400` to; a quoted one comes back with the break folded and spaces inserted |
| byte order mark | `U+FEFF` | inside YAML's printable range and excluded from it by name; the Factory escapes it and every character after it |
| above the printable ceiling | anything above `U+FFFD` | the Factory writes nothing above `U+FFFD` literally. `U+FFFE` and `U+FFFF` make the document unparseable; everything above the BMP, emoji included, comes back escaped as `\U0001F600` |

`U+FFFD` itself is accepted, and so is `U+FDD0` — a non-character below the
ceiling round-trips, so the rule is the ceiling and not "non-characters".

**An unpaired surrogate in the request body is a `400`, and is never repaired.**
This one is checked on the raw bytes, before the JSON decoder runs, and it is the
only check on this route that is. `encoding/json` rewrites an escaped unpaired
surrogate — `"\ud800"` with no low half after it, or a low half standing alone —
to `U+FFFD` while decoding, and does the same to any raw byte sequence that is
not valid UTF-8. A validator handed the decoded string therefore sees a clean
value and has nothing to object to, which is how such a body used to answer
`201` with a schematic id computed over a character the caller never sent.

The answer is deterministic: `400`, `validation.failed`, one field error naming
the member of the body that carried it when the body is an object whose members
can be identified, and naming no field when it is not. The reason names the
class — `contains the unpaired surrogate U+D800, half of a character whose other
half never arrived`, or `contains a byte sequence that is not valid UTF-8` — and
never the value.

**No value is ever silently repaired.** Not this one, not a control character,
not anything else on this route: a refusal reports and refuses. A stored value
the operator did not write, filed under a name they will not recognise, is worse
than the `400` that would have told them. A well-formed surrogate pair is not
affected — it is an ordinary astral character and is judged by the table above,
so a `😀` is refused for being above `U+FFFD` and never for its encoding.

**The refusal set is a floor, not a ceiling, and it is derived from a
measurement rather than from a reading of the upstream emitter.** The
measurement is an opt-in test in the repository —
`HOLZKUBE_MANAGER_FACTORY_LIVE=1 go test ./internal/imagefactory/ -run TestLiveCanonical` —
which builds each candidate scalar into a schematic, POSTs it, and compares
holzkube-manager's canonical document and id against the ones `factory.talos.dev`
returns. The rules above are the classes it observed diverging; every one of
them cites the rows that proved it.

What it reached: `U+0000`, `U+0009`, `U+000A`, `U+000D`, `U+001F`, `U+007F`,
six points in `U+0080`–`U+009F` (`U+0080`, `U+0081`, `U+0085`, `U+008D`,
`U+0094`, `U+009F`), `U+2028`, `U+2029`, `U+FEFF`, `U+FDD0`, `U+FFFE`,
`U+FFFF`, the boundaries `U+D7FF`, `U+E000`, `U+FFFD` and `U+10FFFF`, and the
accepted controls `U+00A0`, `U+00E4`, `U+200B`, `U+202E`, `U+4E2D`, `U+1F600`
— each at up to three positions in a scalar and in both quoting styles, through
both `kernel_args` and `meta`.

What it did not reach: the other twenty-six codepoints of `U+0080`–`U+009F`, the
rest of `U+FDD0`–`U+FDEF`, and the interior of the range above `U+FFFF`, which
is refused on the strength of `U+1F600` and `U+10FFFF` alone. A client should
read this section as *these classes were measured to diverge and are refused*,
not as *every divergent codepoint is refused*. No finite sweep can promise the
second, and a value outside the refused set that nonetheless diverges upstream
surfaces as `upstream.factory-rejected` with an id mismatch, which is the
paragraph above this one.

**The reason names the entry and the character class and never the value.**
`name`, `cluster`, `kernel_args` and `meta` can carry secrets — which is why the Factory offers no
way to enumerate schematics at all — and a problem body is rendered in a browser,
may be logged by a proxy, and outlives the form that produced it. A reason reads
`entry 2 contains the control character U+0007`, one-based, matching the row an
operator counts in the form.

### Wall links

    GET    /api/v1/wall-links
    POST   /api/v1/wall-links      {"label":"The screen in the IT office"}
    DELETE /api/v1/wall-links/{id}

A wall link is what a screen in a corridor is left open on. A session expires,
and a television showing a sign-in page at three in the morning is worse than no
wall at all: it stopped answering the one question it was put up for, and nobody
notices until somebody needs it.

**It opens ONE route.** Not a role, not a session, not a service account. A
reader may read the audit archive, every Secret's key names and every cluster's
configuration; this URL lives on a television and gets bookmarked, photographed
and forwarded. The wall route carries `WallLink: true` and nothing else in the
table does, which makes "what can that credential reach" a question somebody
answers by reading the route table rather than by reasoning about a ladder.

**It is not a fourth role**, because `UserRole`'s own definition says three and
not more: every role beyond them is a policy that has to be kept in step with a
surface that grows every phase. A credential that opens one named route needs no
place in that ladder.

**It satisfies that route's session and role gates and nothing else.** It never
becomes a session and never becomes a user, so it cannot be escalated into
either — and a caller who already has a session cannot present one to skip a role
check on some other route.

**A separate prefix, `hkw_`.** A secret scanner keys on a fixed prefix, and a
token of the wrong kind is refused by shape before anything is hashed or
compared.

    Authorization: Bearer hkw_<43 characters>

**The token is shown once.** Only its hash is stored, and no route returns it
again — a lost link is revoked and replaced. The screen says so before the button
is pressed as well as after, because a warning that arrives once the link is on
screen came too late to act on.

**A label is required.** The whole question a revocation asks is *which screen
was this*, and a list of unnamed credentials is one nobody can act on safely.

**`last_used_at` is what the list exists for**: is this still on a wall? It is
written at most once a minute — a screen polls every ten seconds, and a store
write per poll would be a write amplifier rather than an answer.

**At most eight.** Every request carrying a token is compared against all of
them, and a list nobody prunes is a list of credentials nobody has looked at.

**Both writes are `Destructive` and administrator-only.** Creating one hands out
a credential that will live on a screen for months; revoking one turns a screen
off from across the building, and that is only noticed by whoever walks past it.

**The archive keeps the label in clear and never a token.** "Which screen was
this" is the question a revocation asks afterwards, and an archive that recorded
only that a link was created could not answer it. The token is in no request
body: it is minted on the server and returned once.

### Audit

The two mutating routes carry the action tokens `schematic.create` and
`schematic.delete`. As every mutating route, a missing `Action` means no record
at all, so both are stated here rather than left to the handler.

**The audit allowlist for `schematic.create` permits `name` and
`talos_version`, and nothing else.** `kernel_args`, `meta`, `extensions` and
`canonical` are redacted. The Image Factory itself refuses to enumerate
schematics precisely because kernel arguments may carry secrets, and holzkube-manager's
archive is append-only and kept forever (D-16) with no deletion path — so one
kernel argument written in clear is written in clear permanently. The allowlist
default is redact-everything, which means this entry can only be got wrong by
adding to it.

## Inventory

The inventory is two flat resources — `clusters` and `machines` — and the
machines are deliberately **not nested under the clusters**. A machine that
belongs to no cluster is an ordinary state, not an error: it is every machine in
maintenance mode, and every machine whose cluster was removed. Nesting would
make the ordinary case a special one in the URL as well as in the store.

### `Field<T>`: every fact carries its provenance

Every fact a node reported is served as the same object, and nothing about it is
optional reading:

```json
{
  "value": "v1.13.9",
  "level": "node",
  "available": true,
  "stale_since": null,
  "unavailable_reason": ""
}
```

- `level` is `"none" | "node" | "etcd" | "k8s"` — **a string, always**. The Go
  type is an `iota`; the order of those constants is an implementation detail and
  serialising the number would publish it as a contract.
- `value` is **omitted when it is the type's zero**. A client must treat an
  absent `value` as the zero value, not as an error.
- The three states are distinguishable and **all three are meant to be
  rendered**:

  | `available` | `stale_since` | meaning | how it is shown |
  |---|---|---|---|
  | `true` | `null` | confirmed right now | the value |
  | `false` | set | known, but old | the value, muted, plus its age |
  | `false` | `null` | never read | an em dash plus `unavailable_reason` |

`available` means "this value is confirmed right now", not "this value exists".
**A stale field keeps its `value`.** Dropping it is indistinguishable from `null`
to a client, and that is exactly how the empty dashboard INV-08 forbids comes
about — on the screen that exists for the outage, during the outage.

The **age is computed by the client** from `stale_since`. The server holds no
staleness threshold of its own, deliberately: a server-side threshold would be a
second truth beside the timestamp, and the two would drift.

### `/metrics`: the one route outside the versioned API

`GET /metrics` answers the Prometheus text exposition format
(`text/plain; version=0.0.4`). Three things about it differ from every other
route, each on purpose:

- **It requires no session.** A scraper sends a bare `GET` on a timer and cannot
  log in. A metrics endpoint behind the session cookie is one nobody can scrape,
  and the usual consequence is a second listener with no authentication at all
  — strictly worse. What guards it is what guards everything: the host allowlist
  and a loopback bind address.
- **It is not under `/api/v1`.** `/metrics` is where every scraper looks by
  default. It also means the export is not versioned with the API, which is
  right: a metric name is its own contract, and renaming one breaks dashboards
  whatever the URL says.
- **It carries no audit action.** It changes nothing and is requested every
  fifteen seconds for ever; recording it would fill an archive D-16 keeps for
  ever with the fact that a scraper was scraping.

The export is bounded by two rules. **No machine is ever a label value** — a
per-node label multiplies every series by the fleet, which is the axis that
grows — so cardinality is a function of the number of clusters and not of the
number of nodes. And **a series that drops to zero is still written**, because
Prometheus cannot tell a series that stopped being reported from a target that
went away, and a "nodes down" graph that ends at zero shows its last non-zero
value for as long as anybody looks at it.

`holzkube_cluster_client_certificate_seconds` may be **negative**. An expired
certificate is a named state, not a missing metric: the failure it describes
takes every node in the cluster down in the same second, so a series that
vanished at expiry would go blank exactly when somebody needed it.

An instance started without an inventory answers `503` with a plain-text
comment rather than a problem document — a scraper does not read RFC 9457, and
what it does with a non-200 is mark the target down, which is the correct
reading.

### `watch`: how quickly a change will be noticed

Every machine carries one more object, and it is **not** a `Field<T>` and **not**
part of `stage`:

```json
{
  "live": true,
  "since": "2026-09-12T16:04:11Z",
  "reason": "",
  "restarts": 0
}
```

- `live` is true only once the node's resource subscription has delivered the
  node's current contents. A stream that has been opened has not yet said
  anything, and reporting it as live would claim a freshness nobody established.
- `reason` is why it is not live, and is empty when it is.
- `restarts` counts rebuilds since the server started. A watch that is `live`
  with a large `restarts` is the failure this field exists for: it looks healthy
  at every instant somebody looks and delivers nothing between them.

`live: false` is **not** a node problem and must not be rendered as one. The
heartbeat reads every node on its own timer whether or not a watch is running,
so a node without a live watch is at most one heartbeat behind — which is what
this product did before watches existed. Its `stage` and its fields say what is
true; `watch` says only how soon the next change will show up.

### Routes

| Method | Path | Destructive | Action | Notes |
|---|---|---|---|---|
| `GET` | `/metrics` | no | — | Prometheus text exposition; **no session**, see below |
| `GET` | `/api/v1/clusters` | no | — | `{ "clusters": [...] }`, never `null` |
| `GET` | `/api/v1/clusters/{id}` | no | — | |
| `POST` | `/api/v1/clusters/fingerprint` | no | `cluster.fingerprint` | step one of adoption |
| `POST` | `/api/v1/clusters` | no | `cluster.import` | step two; `201` |
| `POST` | `/api/v1/clusters/{id}/lock` | **yes** | `cluster.lock` | `{ "locked": bool }` |
| `DELETE` | `/api/v1/clusters/{id}` | **yes** | `cluster.forget` | `204`; machines survive, unassigned |
| `GET` | `/api/v1/machines` | no | — | `{ "machines": [...] }` |
| `GET` | `/api/v1/machines/{id}` | no | — | |
| `POST` | `/api/v1/machines` | no | `machine.add` | cluster-scoped; `201` |
| `POST` | `/api/v1/machines/{id}/refresh` | no | `machine.refresh` | one observation pass |
| `DELETE` | `/api/v1/machines/{id}` | **yes** | `machine.forget` | `204`; the machine is not touched |

Importing is mutating and is **not** destructive: it creates and destroys
nothing. Unlocking is destructive because it is what makes every other
destructive route reachable on that cluster. Forgetting a record is destructive
because there is no way to get it back.

### Adoption is two calls, and the split is the contract

`POST /api/v1/clusters/fingerprint` opens one TLS connection to the named
address, reads the certificate, and throws the connection away. It trusts
nothing and stores nothing.

`POST /api/v1/clusters` then carries the `talosconfig`, the address and that
fingerprint together. **The server re-reads the certificate and refuses outright
if it no longer matches**, so the confirmation is a gate rather than a ceremony.

The `talosconfig` travels in the request body — uploaded or pasted. There is no
field for a path on the server, and there will not be one: that would be an
arbitrary file read under the server's uid.

What the server then does, in order, is the whole of the adoption:

1. connect with the supplied credentials,
2. read that node's own machine configuration,
3. derive the cluster's secrets bundle from it,
4. mint itself a client certificate from that bundle,
5. reconnect under it and call `Version`.

**Nothing is written before step 5 succeeds.** A half-adopted cluster is a state
this product refuses to have. An imported cluster comes back `locked: true`.

### The per-cluster read-only lock

A cluster-scoped mutating route is refused with `403` and
`forbidden.cluster-locked` while the cluster is locked. The check is
**server-side and declarative**: a route declares how to find the cluster it acts
on, and middleware asks. Nothing pattern-matches on the URL, and a lock the UI
merely honours is not a lock.

The check sits **inside** the audit link — an attempt to change a cluster
somebody adopted read-only belongs in the archive — and **outside** the sudo
gate, because asking for a password and then refusing anyway teaches an operator
that the prompt means nothing.

### Codes minted here

| code | HTTP | when |
|---|---|---|
| `validation.node-not-controlplane` | 400 | the adoption named a node whose configuration carries no control-plane material |
| `validation.talosconfig-invalid` | 400 | the uploaded file is not a usable talosconfig |
| `validation.fingerprint-mismatch` | 400 | the node presented a certificate other than the confirmed one |
| `forbidden.cluster-locked` | 403 | the cluster was adopted read-only and this request would have changed something |
| `conflict.cluster-already-adopted` | 409 | the adoption named a node that already belongs to a stored cluster. Adopting it again would move the node to the new record and leave the old one observing nothing; the detail names the cluster to forget first. |
| `conflict.no-machines-to-rotate` | 409 | a certificate-authority rotation was asked for on a cluster with no machines recorded. The rotation writes every node's configuration, so an empty inventory has nothing to rotate. |

Each is minted deliberately, in the commit that first emits it. The alternative
is not a missing code: it is every one of these failures arriving as
`internal.unexpected` — which by contract carries no detail — and staying that
way forever in an archive with no deletion path.

## Cluster templates

Two routes, and **neither of them applies anything**. That is the scope and it
is stated rather than implied: applying a template is provisioning, and
provisioning is the one part of this product that has never run against real
hardware. A route that drove it from a YAML file would move that gap somewhere
harder to see.

`POST /api/v1/cluster-templates/plan` takes a template **as the request body, in
YAML** — not wrapped in a JSON envelope. The document is one an operator wrote
in an editor and keeps in a repository, and escaping every newline to post it
produces a file nobody can read in a request log. Clients send it as
`Content-Type: application/yaml`; this is the one route in the API whose request
body is not JSON, and a client that assumes JSON everywhere labels this document
as something it is not.

It answers with what the document would mean for the machines this installation
knows about right now: which machines each side resolves to, what does not add
up, and a sentence saying so. A class is resolved at the moment the template is
read, which is the whole reason a template is worth more than a list of UUIDs.

The document is parsed **strictly**: an unknown field is a refusal. That is the
opposite of what this API's own Image Factory client does with upstream
responses, and the difference is who wrote the document — an unknown field from
a third party is their addition, and an unknown field here is this operator's
typo.

Two refusals are worth naming because they are what the format makes easy:

- a node set that gives both a `machineClass` and a list of `machines`, because
  one of them would be ignored and nothing says which;
- a `count` with no class, because a count picks from a class and a list of
  machines is already the count.

And one problem is found at plan time rather than parse time, because it needs
the fleet: a machine named in both `controlPlane` and `workers`. Two lists of
UUIDs is exactly where that mistake lives.

An **even control plane is a note and not a refusal**. Four voting members
tolerate no more failures than three do, which is worth saying — but somebody
may be mid-change, and refusing there is a product telling an operator they
cannot do what they are doing.

`GET /api/v1/clusters/{id}/template` writes an existing cluster down as one.
**The export lists machines by UUID and never by class**, and that is a
limitation stated rather than hidden: nothing here can know which of an
operator's labels they meant as the *reason* a machine is in this cluster, and
guessing would produce a document that quietly selects a different set later.
Turning the list into a class is the operator's edit, and it is one line. The
machines are sorted, so two exports of one cluster are the same file and a diff
between them means something.

## Provisioning: the installer reference, SecureBoot and disk encryption

### The installer reference is resolved, never assembled

`.machine.install.image` is resolved against the Image Factory when the plan is
made and carried on the request from there, so the reference an operator
confirmed is the reference the job writes.

It used to be assembled by hand as
`factory.talos.dev/installer/<id>:<version>`, and that string was wrong in three
ways at once, each silent:

- **SecureBoot was not in it.** A machine booted from a SecureBoot ISO installed
  a system that is not SecureBoot. Talos requires the SecureBoot installer for
  that install — it carries the signed UKI and systemd-boot, and no
  machine-config flag substitutes for it. `internal/imagefactory` names this
  pairing as "the ISO/installer drift this file's own comments warn about,
  arriving from the one direction nothing checked"; provisioning was that
  direction.
- **The repository name was assumed.** `internal/imagefactory` keeps an ordered
  candidate list (`metal-installer`, then the legacy `installer`, each with the
  SecureBoot suffix when asked for) because which one answers varies by version,
  and warns when it had to fall back. The hand-built string took the legacy name
  unconditionally. Measured against the public Factory on 2026-09-14 at v1.13.9,
  the two ordinary names resolve to the same digest, so this was a latent risk
  rather than an observed wrong image at that version — the SecureBoot one below
  was not.
- **The Factory host was hard-coded**, so an installation pointed at a private
  Factory with `--image-factory` provisioned nodes that pulled their installer
  from the public one.

A schematic named on a plan therefore now requires a configured Factory, and a
failure to resolve is a **refusal with no reference** rather than a fallback —
the same decision `internal/imagefactory` already made for the asset panel.
Substituting the ordinary installer produces a node that installs, joins, and is
not SecureBoot, with nothing afterwards saying so. A machine with no schematic
gets Talos's own published installer; there is nothing to resolve for it.

**`secureboot` is stated by the operator.** A schematic id does not carry it —
one id resolves under `metal-installer` and `metal-installer-secureboot` to two
different images, picked by repository name alone — so the only party who knows
which image was written to the USB stick is whoever wrote it.

The architecture comes from the **stored schematic record**, never from a
constant (FACT-03).

The **upgrade path resolves it the same way**, and it had the same defect with
worse consequences: a fresh provision that dropped SecureBoot produced a node
that never had it, while an upgrade took it away from a node that did.

An upgrade plan now reads each node's `SecurityStates.talos.dev` resource and
puts `secureboot` on the plan beside the installer it resolved — the fact is
shown next to the decision it made, so a surprising installer name can be
explained. It is read **from the node** and never remembered from provisioning:
a machine may have been installed by something else, or reinstalled since, and a
stored flag would be this product's memory of a decision rather than the
machine's account of what it is running.

A node that will not say how it booted is **blocked**, not upgraded on an
assumption — either assumption is wrong for half the fleet. So is a node whose
installer cannot be resolved.

### Disk encryption

`POST /api/v1/provision/plan` and `/apply` accept an `encryption` object:
`{"state": bool, "ephemeral": bool, "kind": "nodeID"|"tpm"}`. It emits Talos
`VolumeConfig` documents for the named system volumes alongside the machine
configuration.

**It applies at install and nowhere else.** Talos encrypts a system volume when
the volume is empty — "before mounting the partition, format and encrypt it;
this occurs only if the partition is empty and has no filesystem". Handing the
same configuration to an installed node does not encrypt what is on it, does not
fail, and does not warn. That is why there is no route that turns encryption on
for a running machine: it would report success and change nothing, which is the
worst failure a security control can have.

Two of Talos's four key kinds are offered and two are refused by name, with
reasons rather than as unrecognised values:

| kind | |
|---|---|
| `nodeID` | offered. Protects a drive that leaves the machine and nothing else; the response sentence says so. |
| `tpm` | offered **only with `secureboot`**. The seal is a statement about which kernel booted, and without SecureBoot that measurement can be produced by a kernel somebody else chose. The combination is refused rather than quietly downgraded — and this product can make that check because it knows which image the node is about to boot. |
| `static` | refused. Talos stores the STATE volume's encryption configuration in META in cleartext, so a passphrase written there is a passphrase on the disk it protects — and it would also be in this installation's store and in every backup of it. |
| `kms` | refused. It is the strongest of the four, and offering it would mean holzkube-manager runs that key server: a node whose key server is down does not boot. This product runs outside the cluster so that it is there in the failure where the cluster is not; a fleet that cannot boot without it would be the opposite arrangement. |

EPHEMERAL is written with `lockToState`, which Talos recommends: wiping or
replacing STATE then leaves EPHEMERAL unreadable rather than leaving workload
data recoverable by whoever supplies a new STATE. STATE is not, because a volume
cannot be locked to itself — and that rule is left to Talos's own validator
rather than restated, since two places saying it is two places to disagree.

The documents are built from machinery's own types and validated by Talos's
parser, not written as YAML text. A hand-written document Talos ignores is the
same outcome as no encryption at all, and it looks like success.

## `seen_at`: answered, versus read

A machine view carries `seen_at` alongside its readings, and the two answer
different questions. The readings say when something was last *read* from the
node; `seen_at` says when the node last **answered anything at all**.

They come apart in one case, and it is the useful one: an observation that
opens a connection and then fails its facts read moves `seen_at` and leaves the
snapshot untouched. So a `seen_at` newer than the readings is a machine that is
present and cannot be read — a different repair from one that is absent, and
one an operator cannot reach from the stage alone.

The snapshot is deliberately **not** re-stamped in that case. Nothing was read,
so there is nothing to write, and giving an old reading a new timestamp would
turn a stale fact into one that looks current — which is what the `Field[T]`
read model exists against.

For two milestones a half-failed observation persisted nothing at all, so
`seen_at` only moved when a complete reading worked and the two failures were
one record. The field's own documentation described the behaviour it now has.

## Renewing this installation's client certificate

`POST /api/v1/clusters/{id}/client-certificate` issues holzkube-manager a fresh
admin certificate for one cluster (V2-OPS-02).

The cluster card has warned about this certificate since D-23 shipped, on a
ladder ending in *"every node in this cluster becomes unreachable at once"*, and
there was nothing to click. A countdown to a door that does not exist is worse
than no countdown: it teaches an operator that the warnings on that screen are
not actionable.

**It touches no node.** The certificate is minted from the cluster's own Talos
certificate authority, which this installation holds, and a node trusts that
authority rather than any particular certificate issued from it — so a fresh one
is accepted the moment it is presented, with nothing rolled and nothing
restarted. This is a different operation from rotating the authority itself,
which changes what every node trusts; that one is below.

**The new certificate is proven before it is kept, and that ordering is the
operation.** Minting is two lines; the way this goes wrong is replacing a
working credential with one that is not, and finding out when the old one
expires — precisely when nobody can get in to fix it. So a connection is opened
with the new certificate and a node has to answer through it before anything is
written. The check is a real connection and not a local signature verification:
verifying against the stored authority would only prove this installation is
consistent with itself, and what has to be true is that the *nodes* still trust
that authority.

Every machine in the cluster is tried, because "this certificate does not work"
and "the node I picked is switched off" are different findings and only the
first is a reason to throw a certificate away.

It is **Destructive** (D-06, so it needs the re-authentication window) for the
reason the password change is: it replaces the credential guarding access to a
cluster's PKI.

It is deliberately **not** under the cluster lock. INV-12 means "nothing may
change this cluster", and this changes nothing on the cluster. Putting it behind
the lock would mean an imported cluster kept read-only on purpose becomes
permanently unreachable the day its certificate expires — the lock turning into
the thing it exists to protect against.

| code | HTTP | when |
|---|---|---|
| `conflict.no-certificate-authority` | 409 | the stored bundle carries an admin certificate and no authority key, so nothing new can be issued from it. The cluster was adopted from a talosconfig that did not carry the authority; the detail says how to get one. |
| `conflict.certificate-rejected` | 409 | the new certificate reached no node, **so the old one was kept and nothing changed**. Its own code because this is the safe outcome of a renewal rather than a failure of one: either every node is unreachable, or the authority in this store is no longer the one the cluster trusts. |

## What Kubernetes knows about a cluster

`GET /api/v1/clusters/{id}/kubernetes` answers the cluster's own view of itself:
the API server's version, its nodes, its pods and its namespaces. `?namespace=`
filters the pods, and the filter is applied by the API SERVER rather than in the
browser -- a cluster with ten thousand pods must not send all of them so that
three can be shown.

It is milestone v1.17's second slice and it is **reads only**. The client was
proven against a cluster before it was allowed to change anything in one. The
routes that change things are the later slices, and each is destructive in the
sense D-06 means:

| route | what it does |
|---|---|
| `POST /api/v1/clusters/{id}/kubernetes/nodes/{node}/cordon` | takes `{"unschedulable": true|false}`: stops or resumes scheduling onto one node |
| `POST /api/v1/clusters/{id}/kubernetes/nodes/{node}/drain` | answers `202` with a job: cordons, then evicts what would move |
| `POST /api/v1/clusters/{id}/kubernetes/pods/{namespace}/{pod}/restart` | deletes one pod so its controller replaces it |
| `POST /api/v1/clusters/{id}/kubernetes/deployments/{namespace}/{deployment}/scale` | takes `{"replicas": n}` through the scale subresource |
| `POST /api/v1/clusters/{id}/kubernetes/deployments/{namespace}/{deployment}/restart` | rolls the pods out under the deployment's own strategy |
| `POST /api/v1/clusters/{id}/kubernetes/manifest/plan` | says what applying a manifest would do, and writes nothing |
| `POST /api/v1/clusters/{id}/kubernetes/manifest/apply` | applies it with server-side apply |
| `POST /api/v1/clusters/{id}/kubernetes/services/{namespace}/{service}/proxy` | takes `{"port": "...", "path": "/healthz"}` and fetches that path from the service |
| `GET /api/v1/clusters/{id}/kubernetes/pods/{namespace}/{pod}/containers` | which containers the pod has, how each is doing, and a one-line explanation |
| `GET /api/v1/clusters/{id}/kubernetes/pods/{namespace}/{pod}/log` | `?container=&previous=&tail=` — one container's output |
| `GET /api/v1/clusters/{id}/kubernetes/events` | `?namespace=` — what the cluster has reported |
| `GET /api/v1/clusters/{id}/kubernetes/pods/{namespace}/{pod}/events` | the same, filtered by the server to one pod |
| `POST /api/v1/clusters/{id}/kubernetes/object` | takes `{"api_version","kind","namespace","name"}` and answers that object as YAML |
| `GET /api/v1/clusters/{id}/kubernetes/identity` | who this product acts as, and what the cluster says that identity may do. `?as=` previews a candidate without storing it |
| `POST /api/v1/clusters/{id}/kubernetes/identity` | takes `{"user": "...", "groups": [...]}`; an empty user goes back to this product's own certificate |
| `GET /api/v1/clusters/{id}/kubernetes/workloads` | everything that runs: Deployments, StatefulSets, DaemonSets, Jobs and CronJobs |
| `POST /api/v1/clusters/{id}/kubernetes/workloads/{kind}/{namespace}/{name}/scale` | takes `{"replicas": n}`, for the kinds that have one |
| `POST /api/v1/clusters/{id}/kubernetes/workloads/{kind}/{namespace}/{name}/restart` | rolls the pods, for the kinds that have a pod template |
| `GET /api/v1/clusters/{id}/kubernetes/resources` | ConfigMaps, Secrets, claims, Ingresses, autoscalers and disruption budgets |
| `POST /api/v1/clusters/{id}/kubernetes/delete` | takes `{"api_version","kind","namespace","name"}` and removes that one object |
| `GET /api/v1/clusters/{id}/kubernetes/nodes/{node}` | one node's conditions, taints, and how much room the scheduler has left |
| `GET /api/v1/clusters/{id}/kubernetes/usage` | what nodes and pods are USING, from metrics-server |
| `POST /api/v1/clusters/{id}/kubernetes/pods/{namespace}/{pod}/exec` | takes `{"container": "...", "command": ["prog","arg"]}` and runs it once |

**Running a command in a container is the most dangerous thing here, and four
decisions make it something this product can offer.**

*It is not a shell.* A command and its arguments go, it runs, the output comes
back. No terminal, no stdin, no session that stays open. `sh -c "..."` is refused
by name (`conflict.exec-refused`) — passing a shell a string is what makes an
archive useless, because it would record `sh -lc` and the interesting half would
sit in an argument nobody reads. `command` is therefore a LIST: a string would
have to be split by something, and whatever split it would be a shell.

*The command is the event.* Every argument is archived in clear, which is only
possible because of the first decision.

*It runs as the operator.* A cluster with no `act_as` set refuses
(`conflict.exec-refused`) rather than quietly using `system:masters` — a fallback
here would be the worst one in the product. Your cluster's audit log therefore
names the person who ran it.

*It is bounded.* One command, a 30-second ceiling, an output cap at 256 KiB. The
output is never archived: it is whatever the workload printed.

A non-zero exit is not an error — `test -f /etc/passwd` returning 1 is
information — so it comes back with whatever was written and the exit reported
alongside.

**There is deliberately no port-forward.** The service proxy already reaches a
workload for reading. A forward means this daemon holding a listener whose
authentication story is nobody's — a second network path into the cluster, owned
by this process. Somebody who needs one has `kubectl` and their own credentials.

| code | HTTP | when |
|---|---|---|
| `conflict.exec-refused` | 409 | a shell with a string, an argument with a line break, or a cluster with no identity to run it as |

**Used and requested are different numbers and both are needed.** A node with no
room left and 5% usage is over-reserved; a node with room left and 95% usage is
about to fall over. Those are opposite repairs, so neither figure substitutes for
the other and this route never mixes them.

**A cluster with no metrics-server answers `200` with `collecting: false`**, not
an error. Talos does not ship one, so that is the ordinary case rather than a
failure — and a `502` would say the cluster is unreachable when it is answering
perfectly well. It is also not zero: "nobody is measuring" and "nothing is being
used" are different facts, the same distinction INV-08 makes about a node that
was not asked.

**A pod's usage is summed across its containers**, the way `kubectl top pod`
shows it. Reading only the first would report a sidecar-heavy pod as idle.

**This is where "why is my pod Pending" is answered from the node's side.** The
pod's own events say `0/2 nodes are available`, which names no node. A taint, a
kubelet condition, or a node with nothing left to reserve does.

**A pressure condition is bad when it is TRUE**, which is the inversion of
`Ready`. `bad` carries that per condition so a client does not have to know
which way each one reads — a screen that treated them alike would paint a
healthy node red and a full disk green. `Ready` itself is left out: it is on the
node list, and one fact in two places can disagree.

**`NoSchedule` and `NoExecute` are different days of work** — one keeps new pods
off, the other evicts the ones already there — so each taint carries that in
words. `explaining_taints` leaves out the ordinary ones: every Talos
control-plane node carries the control-plane taint, and presenting it as a
finding would tell somebody their cluster is misconfigured on their first visit.

**Requested is not used, and the answer says so.** `cpu_requested` is what the
pods on the node ASKED for, which is what the scheduler reserves; a node at 5%
CPU can still have no room. Pods that have finished hold no reservation and are
not counted — otherwise a CronJob that ran a hundred times would report a node as
full. The pod list behind it is filtered by the SERVER on `spec.nodeName`.

**Each of the six answers a question the workload list cannot**: where the
setting lives, whether a URL reaches the cluster, why a claim is Pending, why a
replica count goes back after somebody scales by hand, why a drain refuses.
`healthy: false` marks the ones that are the REASON something else is stuck, so a
client can put them first rather than burying the finding among forty ConfigMaps.

**Secrets are listed and never read.** Their names and key names are the answer
to "does this namespace have the pull secret"; their values appear nowhere, here
or in the object route, because base64 is not encryption.

**Deleting takes one object and nothing else.** There is no label selector and no
delete-all-in-namespace: the operations that remove many things at once are the
ones where a mistake is unbounded, which is the same reason `--prune` was refused
on the manifest path. Deleting a **Namespace** is refused outright
(`conflict.refused-kind`) — it removes everything inside it, asynchronously, and
nothing stops it once it starts. It is a POST rather than a DELETE because the
archive captures bodies, and *what* was removed is the event.

**A cluster is not only its Deployments.** The storage layer is a DaemonSet, the
database a StatefulSet, the backup a CronJob. Listing only Deployments showed a
workload list with the broken thing absent from it — worse than an empty list,
because an empty list does not imply the thing is not there.

**The numbers mean different things per kind, so they are not flattened.** A
DaemonSet's `desired` is how many nodes match — the cluster's shape, not
somebody's intention. A StatefulSet mid-update is normal, not broken. A Job's
numbers are succeeded and failed. A CronJob has no pods at all between runs, and
`0 of 0 ready` would report a schedule as an outage. Each row therefore carries a
`summary` written for its kind, and clients should show that rather than compute
one.

**A PodDisruptionBudget is judged on whether it is MET, not on whether anything
may be disrupted.** `disruptionsAllowed: 0` is the ordinary, correct state of
every single-replica workload: with one copy, taking it down *is* the outage, so
the budget allows nothing. Flagging that marks every single-replica database in a
cluster permanently, and a warning everybody sees is a warning nobody reads. It is
still said in `detail`, because somebody about to drain a node needs to know this
one cannot be moved without downtime — said, not flagged. `healthy` is false when
`currentHealthy < desiredHealthy`: a pod is already missing, the service is
degraded now, and a drain will be refused on top of that.

**`scalable` and `rollable` say what the kind supports**, so a screen offers only
what exists: a DaemonSet gets no replica field, a Job no roll button. Asking
anyway is refused here by name rather than by the API server's own message —
"the server rejected the request" does not tell somebody that the count they
wanted is the number of nodes.

**Acting as the operator is what makes a cluster's own audit log useful.** By
default every Kubernetes request arrives as `holzkube-manager` in
`system:masters`. A cluster that logs faithfully therefore records that *this
product* scaled a deployment — for every operator, for ever — and cannot answer
the one question an audit log exists for. `system:masters` also bypasses RBAC, so
a cluster cannot express "this person may restart pods in `web` and nothing
else".

When `act_as` is set, requests carry `Impersonate-User` (and optionally
`Impersonate-Group`), and the API server records **both** identities: the
impersonator and the impersonated.

**A refusal is never retried as the administrator.** That rule is what keeps this
from being decorative: with a fallback every request would succeed either way,
the cluster's RBAC would decide nothing, and its audit log would show an
administrator action whenever somebody lacked a role. A `403` comes back as a
`403` and names whose it was.

**Every client carries the identity, including the manifest path.** Impersonating
only the typed calls would attribute reads to the person and writes to the
product, which is the wrong half to get right.

**The preview asks the cluster, and it is the reason this is safe to switch on.**
`?as=` runs a `SelfSubjectAccessReview` for each permission the product actually
issues, as that identity, and answers the cluster's own verdicts and reasons.
Nothing is computed here: RBAC is the cluster's arrangement of roles and
bindings, and any answer derived in this process would be a guess about somebody
else's configuration.

**The value is the cluster's idea of the user** — for OIDC usually the email or
subject, carrying whatever `--oidc-username-prefix` prepends. This product cannot
derive it, so it is written down rather than guessed. Setting it is `Destructive`
and under the cluster lock: nothing in the cluster changes, but a wrong string is
a cluster whose every Kubernetes screen is refused.

**The object route is how the thirteenth question gets answered.** The lists show
chosen fields; a toleration, a node selector or somebody's controller annotation
is not among them, and a product that cannot show the object sends its operator
back to `kubectl` for it. The kind is resolved through the cluster's own
discovery, so a `CustomResourceDefinition` this build has never heard of works.

**A Secret is refused** (`conflict.refused-kind`), not redacted. Its `data` is
base64 rather than encryption, so rendering it puts the credential on the screen.
Redaction was the obvious alternative and is worse: it teaches that looking at
Secrets here is safe, and the first field it misses is a credential on a screen
that promised it was not.

**`managedFields` and the last-applied annotation are removed, and the answer
says so** in `notice`. Both are bookkeeping longer than the object itself, and an
object silently missing fields is how somebody concludes a field is not set when
it is. It is a POST for the same reason the proxy is: the archive captures bodies,
and here nothing else names the object.

**Logs answer the question the rest of this screen cannot.** Everything else says
WHAT is wrong; the log of the container that died says why. `previous=true` is
therefore the flag that matters: a pod in CrashLoopBackOff has printed nothing in
its current container, so a log view without it answers every crash loop with an
empty box. `has_previous` on the container says whether asking would answer.

**A log is cut at the START when it is too long** (1 MiB), because the last lines
are the ones that explain a failure, and the cut is reported as `truncated`.

**Nothing of a log's content is archived.** A log line is whatever the workload
printed — tokens, connection strings, personal data — and the audit archive is
kept forever (D-16), so a log in it is a secret in it with no path that removes
it. The archive records that somebody read the log of a named pod. That is also
why this route is a GET with the container in the query while the service proxy
had to be a POST: there the path being fetched *was* the event; here the event is
the pod, and the pod is in the route's own path.

**A pod with several containers is refused rather than guessed at.** Picking the
first would show a sidecar's log and let somebody conclude the application
printed nothing. The refusal names the containers.

**Events carry a notice, and it is part of the contract.** A cluster forgets its
events after about an hour, so an empty list means "nothing reported recently",
never "nothing happened". Both event routes filter on the SERVER — the per-pod
one on `involvedObject` — so a detail screen cannot show another object's
failures under this object's name.

**Container detail carries the previous run**, which is where a diagnosis lives:
exit code 137 is a memory limit, 1 is a throw, `ImagePullBackOff` never started.
`explanation` is computed on the server so that sentence is written once rather
than in the screen, the API and whatever reads the API next.

**A drain evicts rather than deletes**, so a PodDisruptionBudget can refuse it —
and when one does, the answer names the pod and the budget instead of retrying
past it. Pods a DaemonSet owns, mirror pods and pods with `emptyDir` data are
classified before anything is evicted: the first two come straight back, and the
third loses data, so the last needs `delete_local_data` said out loud.

**`restart` on a pod is refused when nothing owns it** (`conflict.nothing-would-recreate-it`).
Kubernetes has no restart verb: it has "delete the pod and let the controller make
another", and those are the same operation only while a controller exists. A
product that deleted a bare pod because somebody clicked restart would have
destroyed something on the strength of a word.

**Scaling and rolling are different operations and the routes say so.** Scaling
changes how many pods there are, through the scale subresource `kubectl scale`
uses. A rollout restart annotates the pod template with
`kubectl.kubernetes.io/restartedAt`, which changes the template hash, which makes
the deployment controller replace the pods under its own surge, `maxUnavailable`
and readiness probes — so the workload stays up. Deleting a deployment's pods one
at a time would take it down, and that is why both exist.

**Kubernetes's view and the inventory's are allowed to disagree, and where they
do, that is the information.** The inventory knows what the machine API says
about a machine. This knows whether the kubelet registered, what the scheduler
will do with the node, and whether somebody cordoned it.

**A pod carries both its phase and its readiness**, because `Running` with
`1/2 ready` is the most misread state in Kubernetes: the phase is the pod's own
claim about its lifecycle, the ready count is whether its containers pass their
probes, and a screen that showed only the first would call a broken workload
healthy. Restarts are summed across containers, the way `kubectl` shows them,
and a container's own waiting reason -- `CrashLoopBackOff`, `ImagePullBackOff` --
is preferred over the pod's usually-empty one.

**An API server that does not answer is not a cluster with no pods.** The route
answers `upstream.*` in that case rather than an empty list, for the reason
INV-08 gives one layer down: an empty screen is a claim.

| code | HTTP | when |
|---|---|---|
| `conflict.no-kubernetes-authority` | 409 | the cluster's stored bundle carries no Kubernetes authority, so no certificate can be minted for its API server. The cluster can still be managed over the Talos API; the repair is to adopt it again with a talosconfig that carries the authority. |
| `upstream.no-kubernetes-endpoint` | 502 | no control-plane node could say where the API server is. The endpoint is read from a node's own machine configuration -- in the `KubeClusterConfig` document since Talos 1.14 -- rather than assembled from the address the cluster was adopted through. |
| `upstream.kubernetes-unreachable` | 502 | the API server did not answer. |

## One screen for the IT office

    GET /api/v1/clusters/{id}/wall[?namespace=]

    {"generated_at":"2026-09-20T09:59:55Z",
     "nodes":[{"kind":"Node","name":"cp-2","state":"unknown","detail":"not reporting"}],
     "workloads":[{"kind":"Deployment","namespace":"default","name":"api",
                   "state":"down","detail":"0 of 2 ready"}],
     "namespaces":[{"name":"default","total":3,"state":"down",
                    "worst":"api · 0 of 2 ready","stopped":1}],
     "cpu":{…},"memory":{…},"pods":{…},
     "warnings":[{"object":"Pod/api-7c9","reason":"FailedScheduling",
                  "message":"0/3 nodes are available…","count":340,
                  "last_seen":"2026-09-20T09:58:00Z"}],
     "summary":{"ok":4,"warn":2,"down":1,"stopped":1,"unknown":1}}

**One route for the whole screen**, and it is the most expensive read in this
product. A wall makes the same call every few seconds for weeks; five routes
would be five chances for one to fail while the other four painted a confident
picture, with nothing on the screen to say which quarter of it was stale. That is
the trade this row in the budget table records.

**The state is decided here, not on the screen.** A tile is a colour, and working
a colour out from four numbers is arithmetic that must not happen in two places —
least of all on a display nobody is standing in front of to notice them
disagreeing.

**Five states, because two would lie three ways.** `ok`, `warn`, `down`,
`stopped`, `unknown`.

- A **CronJob between runs** has no pods. Arithmetic over desired and ready calls
  nought of nought an outage; on a wall that is red every night at three, and by
  the second week nobody looks at the wall. It is `ok`.
- Something **deliberately stopped** is a decision, not a fault. Its own colour,
  so that seeing it is not the same as being alarmed by it.
- A **node nobody is hearing from** is `unknown` and never `ok`. That is the one
  case a wall must never paint green, because it is the case a wall exists for. A
  client must count it with `down` and not with `warn`.

**`generated_at` is what makes the screen honest.** A wall that cannot go stale
lies during exactly the incident it exists for: a daemon that died at two leaves a
confident green screen up all night. A client shows the age at all times — not
only when it is bad, because a number that appears only during trouble is one
nobody has learned to read by then — and stops looking confident past a few
refresh intervals.

**`namespaces` is the same workloads rolled up, and the roll-up is arithmetic,
so it happens here too.** One entry per namespace, carrying the WORST state
inside it and naming whatever decided that — "postgres · 2 of 3 ready" — because
a coloured block with no name sends somebody to go and look, which is the whole
thing a wall exists to save. `worst` is empty when nothing is wrong: an entry
that always carries a sentence is one whose sentence nobody reads.

`stopped` is counted separately and never colours the entry. Something switched
off on purpose is a decision, and a namespace that went amber because somebody
paused a job is a namespace that teaches an operator to ignore amber. But a
namespace where EVERYTHING is stopped is `stopped` and not `ok`, because `ok`
claims it is running and nothing in it is.

**The roll-up is an additional view, never a replacement.** The totals sum to the
length of `workloads`, and every workload that is not running is still in
`workloads` under its own name. A client is free to draw either or both; what it
must not do is show only the entries that are not green, which is how this screen
once came to report "showing 0 of 132" about a cluster running all 132.

**The namespace narrows the workloads and the warnings, and never the nodes.** A
wall that hid a dead node because somebody had left a namespace selected would be
the worst possible failure of this screen.

**Warnings only, newest first, at most six.** The events route answers newest
LAST, because there the sequence is the story; a wall has room for six lines and
they have to be the six most recent, so the order is turned round here rather than
in the thing every other screen shares. `count` is carried because a
FailedScheduling seen 340 times is a different situation from one seen once, and
from four metres that count is the whole message.

**`last_seen` is an instant, not an age**, and the client works the age out. This
field was called `age` and carried an instant anyway, so a wall put
"2026-09-20T09:58:00Z" on a television. The arithmetic belongs in the browser for
once: this screen keeps the last answer up when a refresh fails, and an age baked
in here would freeze at the moment the daemon stopped answering — the one moment
it must not.

**It carries names and states and nothing else.** No addresses, no versions, no
key names. This is the answer most likely to end up on a screen a visitor can
see, and `RoleReader` — whose own definition names "the dashboard left open on a
screen in the hallway" — is what it asks for.

## Namespaces, quotas and the cluster's own kinds

    GET /api/v1/clusters/{id}/kubernetes/inventory

    {"namespaces":[{"name":"old-staging","phase":"Terminating","pods_running":0,
                    "quotas":[],"has_limit_range":false,"healthy":false,
                    "notice":"It has been deleted and something in it will not go…"},
                   {"name":"ci","phase":"Active","pods_running":2,
                    "quotas":[{"quota":"build-quota","resource":"cpu",
                               "used":"4","hard":"4","percent":100}],
                    "has_limit_range":true,"healthy":false,
                    "notice":"Its quota is full on cpu…"}],
     "custom_kinds":[{"group":"longhorn.io","kind":"Volume","versions":["v1beta2"],
                      "stored":"v1beta2","scope":"Namespaced","established":true}],
     "notice":"A quota's used figure is what the namespace has reserved…"}

**No namespace parameter.** The answer *is* the namespaces, and narrowing it to one
would be a screen that cannot show the namespace somebody is looking for.

**A namespace stuck `Terminating` is the first finding.** Something in it has a
finalizer nothing will clear, so it hangs — for weeks, in practice — the name cannot
be reused, and every attempt to recreate it fails with "already exists" while every
attempt to use it fails too. `kubectl get ns` shows the word and nothing about what
is holding it. The API server's own condition message does, and `notice` carries
that; where no condition has been written yet, it says so rather than leaving a
blank, because the first seconds of a deletion are not the same as nothing holding
it.

**A full ResourceQuota is the second, and the refusal lands somewhere else.** It is
why the next pod is refused, and the pod's own event says "exceeded quota" in a
namespace whose quota nothing on any screen showed — the most confusing refusal in
Kubernetes after an unbound claim. Only the resources that are actually full are
named; a quota at 6Gi of 8Gi is ordinary and naming it would bury the one that
matters.

**A compute quota with no LimitRange is the combination nobody expects.** Every pod
that does not set requests is refused — not for being too big, but because the quota
cannot account for a pod that asked for nothing. The error says "must specify
limits", which reads as the pod being wrong; it is the namespace that is
half-configured. Only `cpu` and `memory` quotas do this: a quota on
`count/pods` or `persistentvolumeclaims` does not, and warning about it would warn
on most namespaces that have a quota at all.

**Quotas are read from `status`, not `spec`.** Status is where the API server
reports what is in force and what is used; a client reading spec would report a
quota as empty the moment somebody edited it.

**The quota lines come back sorted.** They are built from a map, whose order Go
randomises, and an unsorted answer makes the screen shuffle on every refresh so
nothing is findable twice.

**`pods_running` excludes finished pods**, so an empty namespace reads as empty
rather than as a name — and a namespace whose CronJob has run two hundred times
does not read as busy.

**The cluster's own kinds are listed because the object route can already read
them.** A CustomResourceDefinition this build has never heard of works through the
cluster's own discovery; what was missing was finding out which ones exist, and a
cluster's operators keep their state in them.

**`established: false` is a kind every manifest naming it is refused for**, and
nothing else in a cluster says so. A definition waits like this while its
conversion webhook is unreachable.

**A version the API server does not serve is left out.** Listing it would send
somebody to a version every request for which is refused.

**Objects are not counted per kind.** That would be one list call per definition on
a cluster that can have two hundred, and a screen's worth of numbers nobody asked
for is not worth a request storm.

**A cluster that does not let this identity read apiextensions answers no kinds
rather than failing.** That is not a failure of the namespace screen, and the
client says both readings of an empty list.

## Who may do what

    GET /api/v1/clusters/{id}/kubernetes/access[?namespace=]

    {"administrators":["ServiceAccount ci/deployer","User holz@holzcloud.ch"],
     "bindings":[{"kind":"RoleBinding","namespace":"web","name":"web-readers",
                  "role_kind":"Role","role_name":"pod-readr","role_exists":false,
                  "subjects":[{"kind":"ServiceAccount","namespace":"web","name":"reader",
                               "checkable":true,"exists":true}],
                  "administrative":false,"healthy":false,
                  "summary":"It names Role pod-readr, which does not exist…"}],
     "roles":[{"kind":"ClusterRole","name":"platform-operator",
               "rules":["everything on every resource"],"administrative":true,
               "bound":1,"built_in":false}],
     "accounts":[{"namespace":"default","name":"default","used_by":["default/bare"],
                  "bindings":1,"administrative":true,"notice":"…can do anything to the cluster"}],
     "notice":"A binding that names a role or a service account which does not exist…"}

**A list of roles is not the answer.** Every question somebody has about RBAC is
about whether an arrangement *works*, and each is answered by two objects
disagreeing — which nothing in the cluster will report, because RBAC has **no
referential integrity, deliberately**, so that a binding may be written before its
role.

**`role_exists: false` is the finding.** A binding naming a role that is not there
grants nothing at all, and looks exactly like one that grants everything it was
written for. `summary` says why RBAC allows it, so the finding does not read as a
bug in the cluster.

**A subject is checked only when it can be.** `checkable` is true for a
ServiceAccount and false for a User or a Group: those live in the identity
provider and no cluster has a list of them. A client must not mark an unchecked
subject as missing — a column saying "does not exist" beside every OIDC user would
teach that the marking is noise, and the real finding would then be invisible.

**`administrators` is not a field anywhere in Kubernetes.** It is every subject
that reaches wildcard verbs on wildcard resources through some binding, and it is
the first thing anybody wants to know. An empty list is a real answer and needs its
caveat: the cluster's own certificate holders are outside RBAC entirely and are not
in it.

**Administrative is decided from the rules, never from the name.** Looking for
`cluster-admin` would miss every hand-written role with `verbs: ["*"]` on
`resources: ["*"]` — the same power under a name nobody recognises, and the usual
way somebody grants it by accident.

**One rule has to carry all three wildcards**, rather than a set of rules carrying
them between them. "`*` verbs on configmaps" and "get on `*`" are both ordinary, and
a check that ORed them would report half the cluster's built-in roles as
administrative — a warning everybody sees is a warning nobody reads. A rule granting
only `nonResourceURLs` is not counted: that is `/healthz` and `/metrics`, not
administration of anything in the cluster.

**`used_by` on a service account is every running pod that runs as it**, and the
`default` account of a namespace is what every pod naming none runs as. A
permission granted there reaches things nobody intended: anything able to run a pod
in the namespace then has it, which is why that combination gets its own sentence.

**`built_in` marks the roles Kubernetes ships.** About seventy, identical on every
cluster, and never what somebody is looking for — so a client can fold them away.
It should fold rather than filter: hiding them for good makes a screen that cannot
answer "does this cluster still have the standard roles".

**`bound: 0` on a role somebody wrote means it permits nothing**, because nothing
grants it. On a built-in role it is ordinary.

**The namespace narrows the namespaced objects only.** ClusterRoles and
ClusterRoleBindings are always in the answer: a cluster-wide grant is not something
a namespace filter should hide, and it is the grant that matters most.

**`RoleOperator`, not `RoleReader`.** This answer names who administers the
cluster, which is a different kind of fact from how many pods are running.

**Nothing here writes.** A wrong RBAC change locks the operator, and this daemon,
out of the cluster. The manifest path exists for anybody who means to change a
binding, with a plan shown first.

## The cluster's storage

    GET /api/v1/clusters/{id}/kubernetes/storage[?namespace=]

    {"volumes":[{"name":"pvc-8f3a…","capacity":"20Gi","phase":"Released","claim":"",
                 "reclaim_policy":"Retain","access_modes":["RWO"],
                 "driver":"driver.longhorn.io","notice":"Its claim is gone and…"}],
     "claims":[{"namespace":"db","name":"postgres-data","phase":"Bound",
                "used_by":["db/postgres-0"],"expandable":true,"notice":""}],
     "classes":[{"name":"longhorn","default":true,"binding_mode":"Immediate",
                 "allows_expansion":true}],
     "notice":"A capacity here is what was provisioned…"}

**The claim list on its own cannot answer the questions storage raises.** It is
one half of a two-sided arrangement, and every field added here is one no claim
carries:

- **Whether deleting it destroys the data.** That is the *volume's*
  `reclaim_policy`: `Delete` destroys, `Retain` keeps. A client should render what
  happens rather than the API's word — it is the most consequential field in the
  answer and the least readable.
- **Who is using it.** `used_by` is every running pod that mounts the claim.
  Nothing but the pods knows this. A `Succeeded` or `Failed` pod is excluded: it
  mounts nothing any more, and counting it is how somebody decides a claim is in
  use when it is not. An empty list on a `Bound` claim is a real and interesting
  answer — storage being paid for and not used.
- **Whether it can be grown.** `expandable` comes from the class's
  `allowVolumeExpansion`, so a screen does not offer an edit the provisioner
  refuses.
- **Why it is Pending.** The claim's own events say "unbound immediate
  PersistentVolumeClaim", which states the thing somebody already knows. `notice`
  names the actual cause, and two of the three are not faults: a class whose
  binding mode is `WaitForFirstConsumer` is *working as configured*, a claim with
  no class is waiting for the default one, and a claim naming a class that does
  not exist will never be provisioned at all.

**`Released` is the row this route exists for.** A volume whose claim is gone and
whose data is still on the disk, held because the reclaim policy said `Retain`. It
is invisible in every namespace view — a PersistentVolume is cluster-scoped — it
counts against nothing, and it is the commonest way a cluster quietly fills its
storage backend. It is also, occasionally, exactly the data somebody needs back,
so `notice` says both.

**The namespace narrows the claims only.** Volumes and classes are cluster-scoped;
hiding them in a namespace view is how a `Released` volume holding somebody's
database stays invisible.

**Two default storage classes, or none, are reported in the notice.** With two the
API server picks one arbitrarily, which is not a choice anybody made; with none a
claim naming no class stays Pending for ever rather than failing, and nothing else
in a cluster will say so.

**A capacity is what was provisioned, never how full the filesystem is.** Nothing
in the Kubernetes API reports the second — only something running inside the pod
can — and a column headed `20Gi` invites exactly that reading, so the notice
refuses it in words.

**Nothing here writes.** Deleting a `Retain` volume is how data goes for good. The
object-delete route already does it for anybody who means it, with the name typed
out; a "clean up volumes" button would be the one in this product whose mistake
cannot be undone at all.

## The cluster's networking

    GET /api/v1/clusters/{id}/kubernetes/network[?namespace=]

    {"services":[{"namespace":"default","name":"api","type":"ClusterIP",
                  "cluster_ip":"10.96.0.11","external":"","ports":["80/TCP → 8080"],
                  "selector":"app=api","endpoints":0,"ready_endpoints":0,
                  "healthy":false,"notice":"Nothing is behind it: no pod matches app=api…"}],
     "policies":[{"namespace":"default","name":"api-lockdown","applies":"app=api",
                  "types":["Ingress"],"selects":0,"healthy":false,"summary":"…protects nothing"}],
     "unprotected":["monitoring","open"],
     "ingress_classes":["nginx"],"default_ingress_class":"nginx",
     "notice":"Endpoints are counted from EndpointSlices…"}

**A Service with no endpoints is the commonest broken thing in Kubernetes, and it
looks completely healthy in every list.** Name, type, ClusterIP, ports — all
present, and every connection to it refused instantly while the workload that
calls it reports a connection error pointing at itself. Counting the endpoints is
the single fact that makes this route worth more than `kubectl get svc`, and
`notice` names the selector that matches nothing: "nothing matches" without saying
*what* does not match leaves somebody where they were.

**`ready_endpoints` is reported beside `endpoints` because an unready endpoint is
excluded from load balancing entirely.** A Service with three endpoints of which
none are ready serves nothing, and it looks better than one with none. `healthy`
is false for both.

**Endpoints come from EndpointSlices**, which is what the kube-proxy on each node
actually load-balances to, in one list for the whole set rather than one call per
Service — the latter turns a screen into a hundred round trips.

**No endpoints is correct for an `ExternalName` Service**, which is a DNS alias and
forwards nothing. A screen that flagged it would be wrong on every cluster that has
one.

**`external` is what reaches it from outside**, and empty for a plain ClusterIP —
which is the answer to "why can I not reach this from my laptop", and one a list of
ports cannot give. A `LoadBalancer` with no address answers "waiting for an
address": Talos ships no load-balancer controller, so that is the ordinary state on
this product's own target.

**The dangerous default in network policies is having none.** A namespace with no
NetworkPolicy accepts traffic from every pod in the cluster. That is Kubernetes's
default and plenty of clusters run that way on purpose, but it is invisible, and
"we have policies" is usually believed about a cluster where two namespaces have
them and eleven do not. `unprotected` lists the namespaces that have pods and no
policy — listed rather than counted, because "eleven namespaces are open" is not
actionable and a list is.

**A policy whose selector matches no pod is worse than no policy**, because
somebody believes it is in force. `selects` is how many pods it currently matches
and `healthy` is false at zero.

**An empty pod selector means every pod in the namespace**, which is the opposite
of how an empty filter reads everywhere else, so `applies` says it in words rather
than as a blank.

**`types` is derived when the policy does not name them** — Ingress, plus Egress
if there are egress rules — because a policy without the field is ordinary and an
empty column would read as a policy that does nothing. A policy with Ingress rules
only leaves outbound traffic entirely alone, which is not what "we have a policy
for that" usually means, and the summary says so.

**An Ingress naming no class in a cluster with no default is one no controller
will ever pick up**, and it looks exactly like a working Ingress. So the classes
and the default are in the answer.

**Nothing here writes.** A NetworkPolicy applied wrongly cuts a cluster off from
itself, including from whatever this daemon needs to reach it. The manifest path
exists for anybody who means to change one, with a plan shown first.

## How full a cluster is

    GET /api/v1/clusters/{id}/kubernetes/capacity

    {"cpu":{"allocatable":"12","requested":"4200m","percent":35},
     "memory":{"allocatable":"46Gi","requested":"18Gi","percent":39},
     "pods":{"allocatable":"330","requested":"74","percent":22},
     "nodes":[{"name":"cp-1","ready":true,"cordoned":false,
               "cpu":{"allocatable":"4","requested":"1400m","percent":35},
               "memory":{...},"pods":{...},"pods_running":24}],
     "notice":"Requested is what the pods asked for…"}

**This is not the usage route and neither substitutes for the other.** Usage needs
a metrics-server and Talos ships none, so on most clusters the usage answer is
correctly "nobody is collecting this" — an honest sentence that reads exactly like
nothing, which is what an operator reported: no resource information anywhere.

This is the figure every cluster has: **allocatable against what the pods
requested**. It is the scheduler's own arithmetic, so it is also what decides
whether the next pod starts, which is the question behind "how full is it" more
often than consumption is. A cluster at 90% requested and 5% used is
over-reserved and will refuse work it could do; one at 20% requested and 95% used
is about to fall over while looking empty. Those are opposite repairs, and a
single number would send somebody the wrong way half the time.

**`allocatable` is not the machine's size.** It is what the kubelet offers the
scheduler: the machine minus what Talos and the system reserve.

**`percent` is `-1`, never 0, when there is nothing to divide by.** A node that
has not reported its allocatable is not a node at 0% full, and a client must show
those differently.

**A cordoned or unready node is left out of the cluster totals, and its pods are
not.** Nothing new will be placed there, so counting its room would report space
that does not exist; the pods already on it are still using it. Both flags are on
every node row so a client can say which nodes were excluded and why.

**Reservations are reproduced here because nothing reports them.** A node does not
publish what has been reserved on it — that is the scheduler's arithmetic — so
this sums the pods. `Succeeded` and `Failed` pods hold no reservation and are
skipped, or a node would read as full because a CronJob ran a hundred times. An
unscheduled pod is attributed to no node, which is usually *because* there is no
room for it.

**A pod's own requests and limits are on the pod row** in the overview, for the
third level of the same question: which pod took the space. Init containers
contribute their **maximum** rather than their sum, because they run one at a
time — summing them overstates every pod that has more than one.

## Stopping and starting a workload

    POST /api/v1/clusters/{id}/kubernetes/workloads/{kind}/{ns}/{name}/stop
    POST /api/v1/clusters/{id}/kubernetes/workloads/{kind}/{ns}/{name}/start

    {"stopped":true,"would_start_with":3}

**"Stop this pod" is not a thing Kubernetes has.** Deleting a pod does not stop
it: its controller makes another within seconds — that is the whole point of a
controller and it is why the restart button works. Stopping means telling the
*controller* to want none, and what an operator means by "stop this service" is
always the controller. A pod that nothing owns therefore has no stop, and that is
said (`conflict.cannot-stop`) rather than the pod being deleted and the deletion
called a stop.

**Starting again needs a number nobody wrote down.** Scaling to zero throws the
replica count away, and "start it again" then has no answer but 1 — which
silently runs a three-replica service at a third at the moment somebody is
restoring it. So the count is written onto the workload as the annotation
`holzkube.holzcloud.ch/replicas-before-stop` **before** it goes to zero: an
annotation that outlives a stop that did not happen is harmless, a stop whose
count went missing is not.

It lives on the object rather than in this product's store, deliberately: the
cluster is the thing that knows, this installation can be reinstalled, and
somebody using `kubectl` can see why their deployment has a strange annotation.
Nothing written down means a start uses 1, and `would_start_with: 0` on the
workload row is how a client says so before anybody presses.

**A CronJob and a Job stop by being suspended**, which for them *is* stopping:
they have no replicas, and a suspended CronJob keeps its schedule and runs
nothing. One verb, the two ways Kubernetes expresses it.

**A DaemonSet has no stop** (`conflict.cannot-stop`), and the refusal says what
to do instead — cordon or drain the nodes. Its size is how many nodes match, so
there is no count to set to zero. `stoppable` on the workload row says this per
kind, for the same reason `scalable` does: a button the API server would refuse
teaches that the buttons are suggestions.

**Starting is `Destructive` too**, and that is not symmetry for its own sake:
starting something somebody stopped deliberately is as much a change to what the
cluster runs as stopping it was.

## Clearing out what is finished

    GET  /api/v1/clusters/{id}/kubernetes/sweep[?namespace=]
    POST /api/v1/clusters/{id}/kubernetes/sweep   {"items":[…]}

    {"items":[{"kind":"ReplicaSet","namespace":"default","name":"web-7c9",
               "reason":"an older rollout of web, running nothing","age":"6 days"}],
     "notice":"Only things that have finished…"}

**The plan comes first and the plan is the feature.** Nothing here is
recoverable, so the GET says what would be removed and removes nothing; the POST
is a separate request. "It removed more than I expected" is the failure being
prevented.

**The POST removes exactly the list it is given**, rather than recomputing one.
Between a plan and an apply somebody's CronJob can run, and a sweep that
recomputed would remove things nobody saw in the list they approved.

**Every row carries a reason**, because a list of names with no reasons is a list
nobody can check before pressing the button.

**What is deliberately not swept:** a pod that is `Pending`, `Running` or
`Unknown` — `Unknown` especially, since it means a node stopped reporting rather
than that the pod stopped; the **newest** ReplicaSet of a Deployment, which is
what a rollback returns to; a Job a CronJob still owns, because that is the
CronJob's own history and its `successfulJobsHistoryLimit` decides when it goes.
Anything younger than an hour is left alone: a Job that finished a minute ago is
one whose logs somebody may be reading right now, and those logs go with the pod.

**At most 200 objects in one sweep** (`conflict.refused-kind`). The route issues
one delete per item, so without a cap its cost is whatever a plan happened to
contain — and a cluster with four hundred leftover pods is exactly the cluster
somebody presses this on. The refusal means "do it in parts and look at each".

**Something already gone counts as removed.** That was the outcome being asked
for, and reporting it as a failure would make an ordinary race look like a fault.

**Images are not in the list, and that is a fact about Talos rather than a
decision.** The machine API can *list* the images on a node and has no delete.
Image removal is the kubelet's own garbage collection, which runs when the disk
crosses a threshold — so `notice` says who removes them. A button that claimed to
delete images would do nothing at all.

| code | HTTP | when |
|---|---|---|
| `conflict.cannot-stop` | 409 | the thing has no stop: a DaemonSet, whose size is how many nodes match, or a pod nothing owns, where removing it would be a deletion rather than a stop. The detail says what to do instead. |


## Rotating a cluster's certificate authority

Three routes, and the split is the operation's safety rather than REST taste:

| route | what it does |
|---|---|
| `GET /api/v1/clusters/{id}/authority` | the plan: the four passes, every node that will be written, whether a rotation is already in progress, whether the cluster is locked, and the warnings |
| `POST /api/v1/clusters/{id}/authority/confirm` | takes `{"typed": "<cluster name>"}` and answers a token bound to this cluster and this action |
| `POST /api/v1/clusters/{id}/authority` | takes `{"confirmation": "<token>"}` and answers `202` with the job |

**The four passes, in the only order that keeps a cluster reachable.** Every
node accepts the new authority as well as the one it uses now; every node starts
issuing from the new one; holzkube-manager mints itself a certificate from the
new authority and proves it against a node before keeping it; every node stops
accepting the old one. Between passes the cluster trusts two authorities, which
is a working Talos configuration — so an interrupted rotation is **continued**
rather than restarted. The authority being moved to is stored on the cluster's
secrets record for exactly that reason; without it a resumed run would generate
a second new authority and leave the cluster trusting three.

**The refusal is checked after pass 1 and before pass 2**, which is the last
moment at which stopping costs nothing. Every node in the cluster has to answer,
and the ones that did not are named. A node that misses pass 1 refuses the
certificate every other node accepts after pass 2, and nothing afterwards can
repair it: reaching it would need the credential it no longer accepts. This is
the opposite of what a rolling upgrade does with a locked node, and for the
opposite reason.

It is **Destructive** and it **is** under the cluster lock, unlike the renewal
above: this one rewrites the configuration of every node, which is precisely
what an adoption kept read-only (INV-12) says must not happen.

**The Kubernetes authority is not rotated.** Talos keeps the two apart, and
rotating that authority through machine configuration alone would leave every
kubelet holding a certificate the API server does not accept. (Since milestone
v1.17 this product does speak to Kubernetes — see the routes above — but it does
so with a certificate it mints from the stored Kubernetes authority for one hour
and never writes down. Reading that authority is not the same as being able to
replace it safely.)

**What the plan's warnings say, and they are part of the contract:** no rotation
has ever been run against real hardware. Every pass is measured against the
simulator, which models what a node accepts and who issued its certificate. The
mechanism and the refusals are proven; your cluster surviving it is not.

The confirmation is checked by re-deriving the intent from the request rather
than reading it out of the token, so a confirmation issued for rebooting a node
— or for another cluster — does not authorise a rotation.

| code | HTTP | when |
|---|---|---|
| `conflict.no-machines-to-rotate` | 409 | the cluster has no machines recorded, so there is nothing to write |

## Applying a manifest

Two routes, and the split is the point:

| route | what it does |
|---|---|
| `POST /api/v1/clusters/{id}/kubernetes/manifest/plan` | takes `{"manifest": "<YAML>"}` and answers `{"objects": [...], "warnings": [...]}`. It writes nothing. |
| `POST /api/v1/clusters/{id}/kubernetes/manifest/apply` | takes the same body and answers `{"applied": [...], "failed": [...], "fully_applied": bool}` |

**The plan is not a courtesy.** An apply that runs without one is the same genus
as a reset without a confirmation: the operator finds out what it did afterwards.
So each object's `action` is `create` or `update`, and it is decided by **asking
the cluster** whether that object exists — not by reading the manifest, which
looks identical either way. The screen has to ask for the plan before it can ask
for the apply.

**A kind's resource and scope come from the cluster's own discovery.** A cluster
with a `CustomResourceDefinition` has kinds this build has never heard of;
refusing them would make this useless for the manifests people actually apply. A
kind the cluster does not have becomes a warning that names the kind rather than
a stack of discovery words.

**A namespaced object that names no namespace is warned about**, with the
namespace it would land in (`default`). Kubernetes would do that silently, and
finding out afterwards is how a manifest meant for one namespace lands in
another. A cluster-scoped object is sent to the cluster-scoped path — sending it
to a namespaced one answers 404.

**Server-side apply, under this product's own field manager**
(`holzkube-manager`). That is what makes `kubectl get -o yaml
--show-managed-fields` able to say who set a field. When another field manager
owns a field an apply sets, the API server answers `409` and **this product
reports it rather than forcing past it**: forcing takes a value away from
whatever is managing it — usually a controller that will set it back — which is a
fight a management product must not start on its own. `force` is never sent.

**The apply answers `200` with the per-object result even when objects failed,
and `fully_applied: false`.** An apply of ten objects where the sixth conflicts
has changed five things; one status code cannot say which five, and a problem
document would replace the list with a sentence. Only a manifest that could not
be parsed at all — nothing attempted — answers a problem.

**It does not delete.** `kubectl apply --prune` decides what to remove by
comparing against a previous apply, and getting that wrong deletes things nobody
asked about. Removing an object is a separate operation and is not in this
milestone.

The body is capped at 1 MiB rather than the 64 KiB every other route gets,
because one rendered chart passes 64 KiB easily.

| code | HTTP | when |
|---|---|---|
| `validation.manifest-invalid` | 400 | the document is empty, is not YAML, or every object in it was unusable |
| `notfound.kubernetes-workload` | 404 | — shared with the workload routes above |

## Reaching a workload

`POST /api/v1/clusters/{id}/kubernetes/services/{namespace}/{service}/proxy` takes
`{"port": "8080", "path": "/healthz"}` and answers
`{"status": 200, "body": "...", "truncated": false}`.

**It fetches; it does not host.** The body comes back as a JSON **string** and the
workload's own content type is not carried at all. That is the decision the route
exists to make: handing back `text/html` from a pod would serve that pod's markup
from this daemon's origin — the origin holding the operator's session cookie — so
any pod in the cluster could script the interface. The screen shows the body as
text in a `<pre>`, never in an iframe and never as HTML.

**A GET is the only method that is ever sent to the workload.** The identity this
product holds is powerful; a proxy that forwarded any method would let anybody
with an operator session drive any in-cluster API — an unauthenticated admin
endpoint on some pod included — with this product's credentials, and the archive
would record "proxy" rather than what was done.

**It goes through the API server's own proxy subresource**, not through a
port-forward tunnel opened by this daemon. So the cluster's authorisation decides
whether this identity may reach that service, and no new listener exists anywhere.
The scheme is `http`: `https` through the proxy would mean deciding what to do
about the workload's certificate, and deciding that quietly is worse than not
offering it.

**It is a POST for a read, and the reason is the archive.** The audit middleware
captures request bodies and not query strings, and on this route the port and the
path *are* the event: "somebody read `/healthz`" and "somebody read
`/admin/users`" must not be the same entry. Both are kept in clear in the archive.
A path in a query string would also sit in the browser's history and the daemon's
access log for no benefit. It is **not** `Destructive`: it cannot write to a
workload, so a confirmation window would be theatre.

**The service is read before it is proxied**, so "there is no such service" is a
different answer from "that path is not served". A non-2xx from the workload comes
back as `status` rather than as an error: a 503 from a health endpoint is the
answer somebody came here for.

**Bounded**: a response is cut at 1 MiB and the cut is reported as `truncated`,
because half a metrics page that looked whole would be read as a complete one. The
call has its own 20-second ceiling rather than the 60 seconds other Kubernetes
calls get — behind it is an arbitrary workload, and a pod that accepts a
connection and never answers is an ordinary broken workload rather than an
exceptional event.

A path containing `..` is refused with `validation.failed`. Measured, so the
reason is stated accurately: such a path does **not** escape into the API server's
own resources — `path.Clean` collapses it and client-go escapes each segment — it
is silently *rewritten*, and the operator would be shown the answer to a question
they did not ask.

## Cluster size

`GET /api/v1/clusters/{id}/scale` answers the question that comes before either
half of scaling: which of this cluster's nodes may be removed, what adding one
would mean, and which machines could join.

**It changes nothing.** Removing a node is that node's own route and adding one
is provisioning; both already existed. What did not exist was the answer, and it
was scattered across etcd's membership, the inventory, and arithmetic somebody
had to do in their head — the part that goes wrong, and in a direction that
feels like caution. Four voting members feel safer than three and tolerate
exactly the same single loss.

Every refusal on this route comes from `upgrade.RefuseIfCannotSpareAVoter`, the
same function `remove-from-cluster` calls, and is carried into the response
**unchanged**. A screen that reached its own verdict would disagree with the
route eventually and would do it quietly: it would look right until somebody
clicked. `reason` is therefore never this route's paraphrase of another route's
refusal.

`members_known` is separate from `voting` because a count of zero that means
"etcd has no voting members" and one that means "etcd was not asked" are
different answers that send an operator to different places. A membership that
cannot be read **does not fail the request**: it is carried into the plan as
`members_problem`, every control-plane node is refused with it as the reason,
and the inventory half of the answer — which machines are in the cluster, which
workers can still go — still comes back. A cluster whose etcd is unreachable is
exactly the cluster somebody is asking this about.

A worker's removal is never gated on the membership. It is not a member.

Machines that cannot join are **listed with a reason** rather than omitted:
"there is nothing to add" and "there are two machines and both are unreachable"
are different situations. Machines belonging to a *different* cluster are left
out entirely — they are not this cluster's business.

## Labels and machine classes

`PUT /api/v1/machines/{id}/labels` **replaces** a machine's labels. Replaces and
not merges: a merge cannot remove anything, so an interface built on one needs a
second operation to delete, and an operator who took a row out of a form would
find it still there afterwards.

A label is the operator's own word about a machine, and that is a contract and
not a description. Everything else on a machine record is something the node
said about itself and is overwritten by the next observation; **nothing observed
ever writes a label**. That is what makes a selector over labels stable — one
over observed facts would silently re-form its set when a node rebooted with a
different disk.

A label set is bounded (32 labels, 63 characters each side) and refused whole
when any entry cannot be read on a screen: a leading or trailing space, or an
unprintable character. Every reason is reported at once in the detail, so a
screen shows them together rather than teaching one per round trip.

`GET /api/v1/machine-classes` lists the named selectors, `PUT
/api/v1/machine-classes/{id}` writes one and `DELETE` removes it.

A selector is an **all-of** over three kinds of condition — `equals`, `present`
and `absent`. There is no "not equal to", deliberately: it reads as a statement
about the machine and quietly includes every machine the label was never written
on, which is how a selector meant to exclude two nodes selects a hundred.

**An empty selector matches nothing, and one is refused rather than stored.**
This is the single most consequential decision in this area. "No conditions"
reads naturally as "everything", which is what a query language usually means by
it — and the thing on the other end of a machine class is a cluster, so a
selector cleared by accident must not quietly become the whole fleet.

Each class is reported with the membership it names **now**, and with a
`sentence` describing its selector in words. The membership is computed on every
read rather than stored: a class is a question and not a group, so a machine
joins by being labelled and leaves by being unlabelled — one place to look when
the membership is not what somebody expected, instead of two that can disagree.

## Accounts and roles

Every route that needs a session also names the least privileged role that may
use it, and a session route that names none **cannot be registered**: the
process refuses to start. That is the same fail-closed shape as the retry
allowlist and the audit redaction allowlist, and it is there for the same
reason — a permission nobody chose is not a permission anybody reviewed.

There are three roles and no more:

| Role | May |
|---|---|
| `reader` | look. Read the fleet, the jobs, the plans and the streams; sign out; change their own password. |
| `operator` | run the fleet: configure, upgrade, provision, reboot, reset, remove a node. |
| `admin` | everything, plus the two things whose blast radius is the instance: manage accounts, and hand out the credentials that make this instance unnecessary. |

The admin-only reads are the ones that hand over a credential or the contents of
a cluster: `talosconfig`, `kubeconfig`, the etcd snapshot and its restore, the
support bundle, the audit archive, and the cluster lock — because unlocking is
what makes every other destructive route reachable.

A request from an account without the role answers **403 `forbidden.role`**, and
the detail names the role it needed. It is a 403 and not a 404: everybody who
can receive it has already authenticated against this installation, and hiding
the route would turn "ask an admin" into "file a bug".

**An account with no stored role is an admin.** Every account created before
roles existed has an empty one, and that account was the only account — reading
it as the least privilege would demote an operator out of their own instance on
the upgrade that introduced this. The empty role is not writable: creating an
account with it is a 400.

### Managing accounts

`GET /api/v1/users` lists them. No password hash is ever reported, and there is
no route that returns one — `auth.Users` strips it before the handler sees it,
so the HTTP layer has nothing to leak. `linked_identity` says whether the
account signs in through the identity provider; the issuer and subject are not
reported, because they are somebody's identity at a third party.

`POST /api/v1/users`, `POST /api/v1/users/{id}/role`,
`POST /api/v1/users/{id}/password` and `DELETE /api/v1/users/{id}` are all
**Destructive** and behind the sudo window. Each changes who can reach cluster
PKI, which is the argument that made the operator's own password change
destructive.

Two refusals are the same rule seen from different sides, and they are separate
codes because the remedy differs:

| Code | Status | Means |
|---|---|---|
| `conflict.self-demotion` | 409 | an account tried to take away its own admin role. Another admin can do it. |
| `conflict.last-admin` | 409 | the change would leave this instance with no admin. Promote another account first. |

An account **may** delete itself, as long as it is not the last admin. Somebody
leaving should not have to ask a colleague to remove them; a rule that refused
it is a rule people work around by sharing an account.

The password reset does not ask for the old password, and the account's own
change does. That is not an oversight in either direction: the account's own
change defends against a stolen session, and the reset exists precisely because
nobody has the old password any more.

**Single sign-on binds on first use and only when there is exactly one
account.** With two, there is no answer to "which account is this identity", and
guessing one is how a new provider subject takes over somebody else's account.

### Service accounts

A service account is an identity that is not a person. It has a role like any
other account and appears in the audit archive under its own name, and it
authenticates by presenting a token on every request rather than by holding a
session.

```
Authorization: Bearer hkm_<43 characters>
```

The asymmetry is enforced in both directions, because an identity reachable two
ways is an identity whose weakest way in is the one that matters. **A password
never signs in a service account** — the refusal is in the password check
itself, not at the sign-in form, and it spends the same work as a wrong password
so the two are not distinguishable by timing. **A token never signs in a
person** — a person has no token hash, and there is no route that would mint one
for them (409 `conflict.not-a-service-account`).

The mirror of that is the two routes that take a password — `POST
/api/v1/account/password` and `POST /api/v1/auth/sudo` — which answer 409
`conflict.not-a-person` to a service account: it authenticates with a token and
has no password to change and none to re-authenticate with — and does not need
one, since its token already satisfies the re-authentication window. Both routes
are genuinely reachable by a service account: each needs only `RoleReader`, and
a bearer token satisfies the CSRF check and the sudo window, so every gate in
front of them hands the request through. Both answered `500 internal.unexpected`
until v1.16, because `Verify` against an empty hash returns an error rather than
false. There was nothing unexpected about it.

The sudo refusal comes **before** the login throttle. The throttle exists to
slow a password guesser and this caller is not guessing — it has proven its
identity with a token — so spending an address's budget on it would let one
misconfigured automation delay a person signing in from the same address.

`internal/httpapi/handlers/password_test.go` holds this over the package's own
source: every function that calls `auth.Verify` must also ask whether the
account has a password. Two routes were separately, identically wrong, and the
third one to take a password is the one that would not have looked.

Unlike the sign-in refusal above, this one **names the reason**. The two are
different questions: there, an anonymous caller is guessing at usernames and
must learn nothing from the answer; here the caller has already proven which
account it is and is being told a fact about its own account.

`POST /api/v1/service-accounts` mints the account and its token together and
returns the token **once**. There is no route that returns it again, because
only its SHA-256 is stored. SHA-256 and not argon2id: stretching improves
nothing on 256 bits from `crypto/rand`, and the stretch would be paid on every
call a machine makes.

`POST /api/v1/service-accounts/{id}/token` rotates. That is the only revocation
there is — the old token stops working at the instant the new one is minted —
and it is why there is one token per account rather than a set: a set needs
names, expiries and a list, and every one of those is somewhere a forgotten
credential keeps working.

**A bearer token satisfies both the CSRF check and the sudo window**, and each
is a statement about the attack rather than a convenience.

CSRF is an attack on *ambient* credentials: a cookie the browser attaches to a
request the operator never made. A page on another origin can neither read a
bearer token nor cause one to be sent, so the header would protect nothing.

The sudo window exists against a *stolen cookie* — somebody who has the session
and not the password — and re-asking for the password is what separates them. A
token has no such gap: it is put on each request deliberately by whatever holds
it, and there is no second secret to ask for. Demanding one would mean either
giving every service account a password, which is a second and weaker way in, or
making every destructive route unreachable by automation, which is most of what
automation is for. What replaces the window is the token itself: per account,
rotatable, and every use of it recorded under that account's name.

An invalid token leaves the request **anonymous** rather than producing its own
refusal, so there is one sentence for "you are not signed in" rather than two
that differ by which credential was tried.

`GET /api/v1/users` lists service accounts alongside people, marked by `kind`,
with `token_issued_at` and `last_used_at`. `last_used_at` is written
best-effort and at most once a minute: writing it per request would put a store
write in front of every call a machine makes and lose a revision race with
whatever that call was about to do. An empty `last_used_at` means never used.

### Audit

`cluster.import` permits `name`, `endpoint` and `fingerprint` in clear.
**`talosconfig` is deliberately absent from the allowlist**: it carries a client
private key, and the fail-closed default writes `<redacted>` for it. That is the
whole reason `internal/audit/redact.go` is an allowlist and not a denylist.

### The two configurations a cluster hands out

`GET /api/v1/clusters/{id}/talosconfig` and `GET /api/v1/clusters/{id}/kubeconfig`
look alike and are not, and the difference decides what a failure means.

The **talosconfig** is rendered here, from the stored secrets bundle, and
reaches no node. It cannot fail for a cluster whose secrets are on disk. The
certificate in it is minted on demand and is deliberately *not* the one
holzkube-manager dials with: two consumers sharing one credential means revoking
either revokes both, and this instance's own access is the one that has to keep
working when everything else has stopped.

The **kubeconfig** is rendered by a control-plane node, from the machine
configuration it is running, and this route is a passthrough. So it can fail for
reasons the talosconfig cannot — the cluster is unreachable, or has no
Kubernetes yet. The route tries control-plane nodes in turn and stops at the
first that answers, because this is a question about the cluster and any member
can answer it. A cluster with no control-plane node on record is refused with a
sentence saying so rather than with a connection error: the two send an operator
to different places.

Both respond `application/yaml` with a `Content-Disposition` filename and
`Cache-Control: no-store`.

**The kubeconfig is audited and the talosconfig is not**, under
`cluster.kubeconfig` with an empty parameter allowlist — the cluster is in the
path and there is no body. The asymmetry is deliberate: what the kubeconfig
hands over is `system:masters` on somebody's cluster, it is not revocable from
here (a Kubernetes CA rotation is what withdraws it), and "who asked for this and
when" is exactly what an archive exists to answer.

## Streaming

### One connection per tab, not per panel

`GET /api/v1/stream?topic=…&topic=…` is the only streaming route, and it
carries every panel a browser tab has open.

A connection per panel would be simpler and wrong in a way that only shows up
in use: browsers cap concurrent connections per origin at six, and a node
detail page with kubelet, etcd, apid and dmesg open is already four of them.
The seventh panel would silently never connect.

At most **12 topics** per connection. Each is a follow stream against a node,
so this is a limit on what one browser tab can ask a cluster to do.

### Topics

| shape | meaning |
|---|---|
| `dmesg:<machine-uuid>` | the node's kernel ring buffer |
| `logs:<machine-uuid>:<service>` | one Talos service's log output |

The spelling is a contract: a client sends these strings back on every
reconnect. An unrecognised topic is `400 validation.*` **before any bytes are
written** — an error inside a `200` the client has already begun parsing is
worse than no answer.

### Frames

```
event: message
id: dmesg:abc=41,logs:abc:kubelet=12
data: {"topic":"dmesg:abc","payload":{"line":"…","at":"2026-09-11T…"}}
```

`event` is `message` for output and `gap` for a dropped range. The payload of a
`gap` is `{"missed":N,"through":M}`.

**`id` is the whole cursor set, not this topic's position.** SSE gives a client
exactly one string to hand back, and this connection carries several topics at
different positions — so the browser's own automatic reconnect replays each
topic from where *that* topic stood. A per-topic id would resume one stream
correctly and silently truncate the rest.

A client may send `Last-Event-ID` itself in the same `topic=id,topic=id` form.
An entry that does not parse is skipped rather than refusing the connection: a
malformed cursor should cost a replay from the start of the buffer, not the
whole stream.

### What a gap means, and why it is not optional

The server holds a bounded ring buffer per topic and never blocks on a
subscriber — a browser tab that stopped reading must not be able to apply
backpressure to a Talos node. A subscriber that falls behind therefore loses
events, and **the newest event wins**: the oldest queued one is discarded, so a
slow panel keeps tailing instead of quietly becoming a replay it can never
catch up from.

Every lost event is counted and reported as a `gap`. A client that does not
render it is showing a log with an invisible hole in it, to somebody using that
log to diagnose an outage. That produces confident wrong conclusions, which is
worse than showing no log at all.

### Connection state is *in* the stream

"Nothing is arriving" has four causes that call for four different reactions:

| state | meaning |
|---|---|
| `live` | the upstream stream is open and delivering |
| `reconnecting` | it broke and is being reopened; the node may be fine |
| `rebooting` | the node ended the stream and is expected back — wait, do not investigate |
| `disconnected` | nothing is running and nothing is trying |

They arrive as ordinary events on the same stream (`payload.state`, with a
`payload.reason`), so they are ordered against the log lines around them. A
state carried out of band would arrive whenever it arrived, and
"reconnecting" would appear above lines that predate it.

### The two things that had to land first

The chain could not stream at all before this phase, and it failed *silently* —
the wrappers buffered, the handler looked fine, and the panel never filled.

1. Every `ResponseWriter` wrapper in the middleware chain implements
   `Unwrap()`, so `http.ResponseController` reaches the real connection. Only
   `Unwrap` — not hand-written `Flush` and `Hijack` — so the capability is
   exactly the connection's and there is no reimplementation to get wrong.
2. The streaming route clears the process-wide write deadline for its own
   connection with `SetWriteDeadline(time.Time{})`. That timeout is sized for
   argon2id and the login rate limiter and has nothing to say about a stream;
   left in place it kills every stream at the same age.

A route declares `Streaming: true`, and `httpapi.New` **panics at composition
time** if a route is both `Streaming` and `Destructive`: the sudo gate holds a
response back until the window has been refreshed, and a held response is not a
stream.

## Jobs and node actions

### Every destructive endpoint answers 202

`POST /api/v1/machines/{id}/reboot`, `/shutdown` and `/reset` all answer
**`202 Accepted`** with a job id, a `Location` header and the stream topic to
watch — never a `200` with a result.

That is not a style choice. A reboot takes a minute and a reset several, and an
endpoint that waits is an endpoint whose progress nobody can watch and whose
failure arrives as a timeout with no record of how far it got.

### Confirmation is server-side, and it is bound to the parameters

Two calls, always:

1. `POST /api/v1/machines/{id}/confirm` with `{action, params, typed}` returns
   `{token, expires, action, params}`.
2. The action endpoint carries that token plus the same `params`.

**The server rebuilds the intent from what was submitted and compares.** A
confirmation for `wipe_mode=user-disks` cannot authorise `wipe_mode=all`,
because the two produce different tokens. A token that carried its own
description would authorise whatever it said while the request did something
else.

The signing key lives in memory and is generated at startup, so a confirmation
does not survive a restart. That is correct: it is a statement about a decision
somebody is making now, and one that outlived its process would be a decision
about a fleet that may have changed.

**The route issues tokens for a fixed set of actions, and `typed` is decided
per action rather than defaulted.**

| action | `typed` required | why |
|---|---|---|
| `node.reboot` | no | asking for a hostname before every reboot is how somebody learns to paste it without reading, and then the typing means nothing on the screen where it matters |
| `node.shutdown` | no | same |
| `node.reset` | **yes**, the hostname | it wipes disks on a real machine |
| `node.remove-from-cluster` | **yes**, the hostname | the node leaves etcd and its record here is forgotten; on a three-member control plane that is a third of the quorum |

Anything else is refused with `validation.*` rather than confirmed. That
refusal is the point: the rule used to be "required for reset, not otherwise",
so `node.remove-from-cluster` inherited "not otherwise" by not being mentioned —
the browser asked for the hostname and the server issued a token to anybody who
asked without one. A default is how a confirmation becomes decoration.

| code | HTTP | when |
|---|---|---|
| `forbidden.confirmation-invalid` | 403 | no token, a forged one, or one for a different action or parameters |
| `forbidden.confirmation-expired` | 403 | the token verified and is too old — read the dialog again |
| `store.cluster-busy` | 409 | another mutating job holds this cluster's lease |
| `store.job-finished` | 409 | a cancel arrived for a job that already ended |

### `GET /api/v1/machines/{id}/reset-preview`

What the machine has, what each scope would do, and what has to be typed. It is
a read, so it is neither destructive nor confirmed — and it is the screen the
confirmation is issued from.

The wipe scopes come back **least destructive first**: a list whose first option
wipes the machine is a list somebody will click through. The pre-selected flags
are `user-disks`, `graceful: true`, `reboot: true` — each the opposite of what
`talosctl reset` does unprompted, and `talos_default_warning` says so on the
screen rather than leaving an operator who knows that tool to assume.

### Job states, and what "parked" means

| state | meaning |
|---|---|
| `pending` | created, waiting for the cluster's lease |
| `running` | executing a step |
| `parked` | **interrupted in a step that cannot be checked; a person has to decide** |
| `succeeded` / `failed` / `cancelled` | terminal |

Parked is not a failure mode of the engine. It is the engine's most important
output.

Each step carries a `verifiable` flag, recorded when the job was submitted. A
verifiable step has a paired read-only "did this already happen?" query, so an
interrupted run of it is resumed by asking. An unverifiable step has none —
nothing a node can be asked distinguishes "wiped five seconds ago and booting"
from "booting for another reason" — so an interrupted run parks the job and
`parked_reason` says what to check.

`POST /api/v1/jobs/{id}/cancel` stops at the **next step boundary**. It is
mutating and deliberately **not** destructive: an operator watching something go
wrong should not have to find their password before they can stop it.

### One mutating job per cluster

The lease is per cluster, not global — one slow node must not stop the fleet —
and it is held in memory rather than in the store. A lease that survived a crash
would block every job on that cluster until somebody cleared it by hand, and the
record a crashed job leaves behind already says what happened.

## Machine configuration

### Nothing leaves this package unredacted

A machine configuration contains the cluster's certificate-authority private
key. "Look at a node's config" is therefore the same feature as "hand the
cluster over" unless every path out goes through one redaction function — and
"every path" is five: the rendered view, the raw tab, the diff, the API
response and the audit log.

The redaction is **in front of the view rather than applied by whoever
remembers**: `machineconfig` hands back redacted views and the handlers have
nothing else to serialise. There is no `?raw=true`, no "show secrets" toggle and
no endpoint behind one.

It is two passes, because they fail in different directions. Machinery's own
`RedactSecrets` knows the schema and removes the fields it names; a sweep for
PEM private-key blocks knows nothing about the schema and catches a key in a
field this build has never heard of. Neither alone is enough — the first misses
what upstream adds after this build, the second misses anything that is a secret
without looking like one. A configuration that does not *parse* still gets the
second pass and is returned with an error alongside, because refusing to show it
would hide the problem and showing it unredacted would hand over the key.

### Routes

| Method | Path | Destructive | Notes |
|---|---|---|---|
| `GET` | `/api/v1/machines/{id}/config` | no | `raw` and `rendered`, both redacted |
| `POST` | `/api/v1/machines/{id}/config/plan` | no | reads, merges locally, changes nothing |
| `POST` | `/api/v1/machines/{id}/config/apply` | **yes** | cluster-scoped; `mode` required |
| `GET` / `POST` | `/api/v1/patches` | no | list, create-or-edit |
| `GET` | `/api/v1/patches/{id}` | no | including superseded versions |

`plan` is a `POST` because it carries the patches, not because it mutates.

### The diff is structural

A text diff of two YAML documents reports reordering and reindentation as
changes, and reports a list that grew from three entries to four as "one line
added" — which is exactly the change that matters, because it is the shape a
strategic merge patch applied twice produces.

So two findings are named rather than left in the noise:

- `list-changed` carries `len_before` and `len_after`;
- `duplicates` carries the values that appear twice **after** the merge.

`idempotent: false` on a plan is the same finding one step earlier: applying the
set twice differs from applying it once, which is nearly always an append.

### The apply mode is computed, not chosen

`verdict.mode` comes from the changed paths against a curated whitelist, and
anything not on the whitelist is reported as needing a reboot — the conservative
direction, because the alternative is a change that claims to take effect and
does not.

Three cases are called out by name:

- **`.machine.install`** applies, reports success, and changes nothing until the
  next install or upgrade. It is deliberately *not* on the no-reboot list, because
  "no reboot needed" would read as "takes effect now".
- **`.machine.network`** gets `try` and `try_seconds`. A network change that is
  wrong makes the node unreachable, and an unreachable node cannot be told to
  undo it. The countdown is the same number the node uses, or the screen is lying
  about how long is left.
- **a second `staged` apply** is refused with `409 store.staged-pending`. Talos
  accepts it and silently replaces the first, so the operator staged two changes
  and exactly one happens with nothing saying which. The guard is per process and
  in memory — a stored flag would be a claim about the node that only the node can
  answer, and it would go stale the moment somebody rebooted outside holzkube-manager.

### Patches are strategic merge only, and append-only

RFC 6902 addresses list entries by index. An index is a claim about a list as it
happened to be when the patch was written; applied to a node whose list is one
longer it edits the wrong entry and reports success. It is refused with its own
code, `validation.patch-not-strategic`, so a client can tell "fix this patch"
from "use the other form".

A patch that is not usable for any other reason — not YAML, or YAML that is not
a mapping — is `validation.patch-invalid`. The two are separate because the
remedies are: one says rewrite the patch in the other form, the other says fix
what you wrote.

Editing a patch writes a **new version** and marks the old one `superseded`. The
old body stays readable, because "what exactly was applied to this node in March"
only has an answer if the thing applied still exists.

### Audit

`config.apply` permits the cluster, the mode and the **patch ids**. Patch
*bodies* are deliberately absent from the allowlist: a body is arbitrary
configuration, configuration is where the secrets are, and the archive has no
deletion path. A stored patch is readable at its own id for as long as it exists
— which is forever — so the record is complete without the archive holding the
bytes.

## Provisioning

### Three reads and one write

| route | method | destructive | what it does |
|---|---|---|---|
| `/api/v1/provision/notices` | GET | no | the sentences the wizard opens with |
| `/api/v1/provision/scan` | POST | no | probes a subnet or a list of addresses |
| `/api/v1/provision/inspect` | POST | no | reads one machine in maintenance mode |
| `/api/v1/provision/plan` | POST | no | validates, warns, and says what would be written |
| `/api/v1/provision/confirm` | POST | no | issues a token bound to exactly this run |
| `/api/v1/provision/apply` | POST | **yes** | submits the job, `202` with a job id |
| `/api/v1/provision/bootstrap-recovery` | GET | no | etcd attempts with no recorded outcome |
| `/api/v1/provision/bootstrap-recovery/{cluster}` | POST | **yes** | records what a person found |

The scan and the inspect are POSTs because they carry a body, not because they
mutate. Nothing on any machine changes until the apply.

`apply` is the only route here marked `Destructive`, so it is the only one
behind the sudo window, and it passes the same per-cluster lock every other
mutation passes.

### A scan never reports "nothing" about a machine

An address that answered is always reported, in one of three states:

- `maintenance` — an unconfigured machine, which is the one the wizard wants.
  It is decided by the certificate being **self-signed**: a node with no cluster
  PKI has nothing to be signed by, and that signature is the only thing a scan
  can read without authenticating.
- `configured` — something presented a certificate a certificate authority
  issued, which is what a machine that already has a configuration looks like.
- `answered` — a connection was accepted and no TLS handshake this could read
  anything from followed.

Only an address where **nothing answered at all** is absent from the result, and
that is a statement about the address rather than about a machine.

The distinction is PROV-02 and it is not pedantry: "nothing found" and "there is
already a node here" lead to opposite actions, and a scan that conflates them
sends the operator to check a cable while a running node sits at the address
they were about to overwrite.

**A scan result carries no UUID.** A probe cannot learn one without connecting,
and a UUID derived from the address the probe was aimed at would be a value that
looks like an identity and is not one. `inspect` is where the identity comes
from. `known` is keyed by address, because the address is what a scan has.

### The identity is read again immediately before the write

`inspect` reads a machine's UUID, MACs, disks and Talos version. The job then
reads the UUID **twice more**: once at the start, and once in the same breath as
the apply. A mismatch is `400 validation.wrong-machine` and nothing is written.

Checking against the identity the wizard read two minutes ago would be checking
a memory. Between an operator reading a screen and clicking apply, a DHCP lease
can move — and applying to the machine that now holds the address wipes it.

### The subnet limit is arithmetic, not taste

`expand` refuses anything larger than a **/23**. 510 addresses at
`ScanConcurrency` with `ScanTimeout` each is 64 seconds of probing, which fits
inside `ScanRouteBudget` — and that budget plus its slack has to fit inside the
server's write timeout, which `cmd/holzkube-managerd/budget_test.go` asserts. A
/22 is twice that and does not fit, so offering it would be offering a scan that
reliably ends as a timeout with no result — which reads, on the screen, as
"nothing is on my network".

IPv6 is refused outright. Scanning an IPv6 subnet is not a scan.

### Confirming a provision is not confirming a node action

`POST /api/v1/provision/confirm` exists rather than reusing
`/api/v1/machines/{id}/confirm`, and the reason is structural: the machine-scoped
route reads the machine out of the inventory to check what was typed against its
hostname, and **a machine being provisioned is not in the inventory**. It has no
UUID recorded, no cluster and no credentials until the run being confirmed has
finished.

What is typed is the **install disk device**. A reset asks for the hostname
because the hostname is what an operator can check against the machine in front
of them; a machine in maintenance mode has no hostname worth checking — it is
whatever the ISO decided — and the thing about to be destroyed is a disk.

### Codes minted here

| code | HTTP | when |
|---|---|---|
| `validation.wrong-machine` | 400 | the machine at the address is not the one the plan names |
| `validation.not-in-maintenance` | 400 | the machine answered and already has a configuration |
| `conflict.bootstrap-unclear` | 409 | a previous etcd bootstrap has no recorded outcome |
| `conflict.bootstrap-in-progress` | 409 | another bootstrap holds the cluster's lease |
| `conflict.already-bootstrapped` | 409 | the cluster already has a running etcd |

The last one is **not a failure**. The cluster is in the state that was wanted,
and reporting it as a failure would send somebody to fix something that works.

`notfound.cluster` is also new here, and it is a correction rather than an
addition: the cluster-lock link used to answer "this cluster is locked
read-only" for every error, including a cluster that does not exist — which sent
an operator looking for an unlock button for something that is not there.

### The bootstrap recovery flow

An etcd bootstrap with no recorded outcome is the one case nothing can decide.
`GET /api/v1/provision/bootstrap-recovery` lists them with the guidance that
says what to look at; the POST records what a person found.

`bootstrapped` has **no default**. A missing field is a `400`, because guessing
there is the operation the whole record exists to prevent. `note` is required
for the same reason: that record is the only account anything will ever have of
that bootstrap.

A `409 conflict.bootstrap-unclear` also comes back from `plan` when the run
would initialise etcd for a cluster with an unresolved attempt. Refusing there
rather than at the job step means the operator learns it on the screen where
they can still go and look, instead of watching a machine install get most of
the way through and stop.

### Audit

The provisioning actions permit the address, the UUID, the cluster, the install
disk, the schematic, the Talos version, the fingerprint and the hostname — every
one of them an identifier or a choice made on a screen. `confirmation` is
deliberately absent from all of them: it is the HMAC that authorises the action,
and an archive with no deletion path is the last place it belongs.

`provision.scan` permits `cidr` and `addrs`. "A scan happened" and "this
installation scanned 10.0.0.0/24" are different events, and the second is the
one somebody reviewing an unexpected connection on their own network is looking
for.

### The allowlist may name a list

An allowlist entry written with a `[]` suffix — `patch_ids[]`, `addrs[]` —
permits a list of scalars. Every element still goes through the same check, an
over-long list is refused, and a list holding an object or another list is
refused whole. A field **not** marked that way still refuses a list, so
`username` is unchanged: a caller sending a list there is sending a shape that
field does not have.

It exists because a record can be incomplete in a way that matters.
`config.apply` names its patches by id, and a run whose ids are `<redacted>` is a
record that says a configuration was applied and cannot say which one.

### Every audited action is in the table

`internal/audit/redact.go` says in three places that it shows the **full set** of
mutations, including the ones listed with nothing permitted. The fail-closed
default makes that claim invisible when it is false — an action nobody listed and
an action deliberately listed as permitting nothing both write `<redacted>` for
everything — and no test inside that package can tell them apart, because only
the route table knows which actions exist.

It shipped false. Phases 6 and 7 added eight audited actions with no entry, so
every parameter of every reboot, reset and configuration apply was a marker while
this document described what those records would contain.
`cmd/holzkube-managerd/allowlist_test.go` now holds both directions: every route
action has an entry, and every entry is emitted by a route.

## Upgrades and etcd

### There is no "latest"

No route accepts it and no screen offers it. "Latest" is a promise this server
cannot keep: the newest Talos release may be two minors away, Talos upgrades one
minor at a time, and an endpoint that quietly did two upgrades in a row would do
the second against a cluster nobody looked at — with the health gate between
them, which is the check that matters most.

`POST /api/v1/clusters/{id}/upgrade/plan` returns the **chain** instead: every
intermediate minor, each with the sentence saying why it exists. A run installs
one of them; the rest are the runs that follow.

`GET /api/v1/upgrade/releases` filters pre-releases out entirely. A chain that
routed a cluster through a release candidate would route it through software its
own project does not call finished.

### The plan is the screen

| field | what it is |
|---|---|
| `chain` | every version to pass through (UPG-05) |
| `strand` | whether this would take Kubernetes out of support (UPG-06) |
| `gate_preview` | the health gate **as it stands now**, with its inputs (UPG-02) |
| `nodes[]` | the walk order, control plane first |
| `nodes[].installer` | the exact installer this node will use (UPG-03) |
| `nodes[].skip_reason` | why a locked node is walked past (UPG-14) |
| `blocked` / `block_reason` | whether this can be submitted at all |

`gate_preview` is named that because it is not the gate that decides. **The gate
runs inside every node's step**, immediately before that node is touched.
Evaluating it once at the start would be evaluating it against a cluster the
previous node has since changed — which is exactly the cluster UPG-02 is about.

A gate that could not be evaluated is a **refusal**, never a blank. A verdict
with neither `ok` nor a `reason` would be the most important check on the screen
silently saying nothing.

### The gate shows its inputs, whether it passes or refuses

`gate_preview.input` carries the membership, the voting count, each member's
raft status, the unreachable members and etcd's own alarms. It is present when
the gate says yes as well: "why did it let that through" deserves the same
answer as "why did it not".

The gate refuses on, in order: an etcd alarm, an unreachable member, fewer than
three voting members, raft lag past `MaxRaftLag`, no leader, and any error a
member reports about itself.

**A learner is not a voter.** A cluster of three members with one learner
reports three members everywhere a member count is shown, and has two votes.
Counting the learner is how a rolling upgrade takes down the second of two
voters while believing it is taking down the first of three.

**A single-node cluster is the deliberate exception.** It has no quorum to lose,
and refusing it would make the smallest homelab unupgradeable.

### Two voters is the number that decides

Three voting members survive losing one. Two survive losing none: the remaining
member holds one vote out of two, which is not a majority, and the cluster stops
accepting writes until the other comes back. There is no command that recovers
from the second loss without a snapshot — which is why this gate refuses rather
than warns.

### A Talos upgrade that would strand Kubernetes is blocked

`409 conflict.would-strand-kubernetes`, with a remedy rather than only a
complaint. The state it prevents has no good way out: the node comes back on the
new Talos, the cluster is then running a Kubernetes version that Talos does not
support, and the Kubernetes upgrade that would fix it is performed by that same
Talos.

An **unknown** Talos version is blocked too. An extrapolated compatibility window
is an upgrade that is allowed to strand a cluster.

### The schematic is read from the node, never guessed

`nodes[].installer` is built from the schematic Talos recorded at install time
and read back off the node on every plan — not from the stored snapshot, not
from a sibling node, not from what the operator last typed. Two nodes in one
cluster can legitimately have been built from different schematics.

The failure this prevents is silent and permanent: an upgrade to a stock
installer on a node built from a Factory image succeeds, the node comes back,
joins, reports healthy — and every system extension it had is gone. Nothing
reports it. A node whose schematic cannot be read blocks the run.

### "The API said OK" is not proof

The upgrade RPC is `LifecycleService.Upgrade`, a **stream**, not the deprecated
unary `MachineService.Upgrade`. The unary one returns as soon as the node has
accepted the request, so everything the installer says afterwards — which is
everything that says what went wrong — is lost.

The stream ending is the node **leaving**, not the upgrade succeeding: a node
rebooting into what it just wrote and a node that fell over look identical from
here. What settles it is a separate read afterwards, against the node, for three
things: the version that was installed, the schematic that was installed, and
its own services running. A node on the right version with the wrong image is
the case that check exists for.

### The image is pulled before anything is replaced

`ImagePull` then `Upgrade`, in that order, because the API requires it — and
keeping the two calls visible rather than hiding them behind one wrapper is
deliberate. A wrong installer reference, an unreachable registry or a schematic
that does not exist fails at the pull, while the node is still running the system
it is running. Failing there costs nothing.

### Confirming an upgrade types the cluster's name

Not a hostname. A rolling upgrade is not about one machine — there is no single
hostname to type — and the thing being put at risk is the cluster.

The confirmation and the submission both rebuild the plan server-side and refuse
if it is blocked. A token issued against one plan and a run built from another
would be a confirmation of something that did not happen.

### etcd members are named by hostname

`GET /api/v1/clusters/{id}/etcd/members` returns each member as
`"cp-1 (id 8e9e05c52164694d)"`. The id is carried because the removal RPC needs
it, and it is never the only thing on offer: an operator asked to confirm the
removal of `8e9e05c52164694d` is confirming a string, not a machine.

The id travels as a **hex string**, not a number. A 64-bit member id does not
survive a JSON number in a browser, and a screen showing a member id that is not
any member's is worse than one showing none.

`tolerates` is derived once here rather than in every client: a majority of *n*
is *n/2+1*, so the number that may be lost is *n − (n/2+1)*.

Removing a voter from a cluster that cannot spare one is `409
conflict.last-voting-member`. A removal is not an upgrade — there is no node
coming back afterwards — so it is the last point at which anything can say no.

The same decision governs `remove-from-cluster` further down, and it is reached
through one function (`upgrade.RefuseIfCannotSpareAVoter`) rather than
two copies. Two doors into the same rule answering differently is the failure
this is written to avoid, and it had already happened: the member route refused
and the node route read no membership at all.

### The snapshot fallback is stated, not hidden

`GET /api/v1/clusters/{id}/etcd/snapshot` streams bytes. It needs a **quorum**:
the member has to confirm with the others that what it is reading is current, and
a cluster that has lost quorum cannot answer that, so the RPC fails there.

What is still available then is the member's own database file, copied off the
node. That is a snapshot of what one member believed — which is what you restore
from when there is nothing better, and it is not the same thing. The refusal says
so, on both the call's error and the stream's, because for a server stream the
refusal arrives on the first read rather than when the stream is opened.

A snapshot that wrote **zero bytes** is refused rather than reported as success.
A zero-length file looks like a backup in a directory listing.

### Restoring etcd from a snapshot

`POST /api/v1/machines/{id}/etcd/restore` replaces one control-plane node's etcd
with an uploaded snapshot. It is the most consequential route in this API and
its shape says so.

**The body is the snapshot**, `application/octet-stream`, and not a field in a
JSON envelope: base64 inside JSON costs a third of a database's size on the wire
and all of it in memory. The two small values that go with it ride in the query
string.

**`?machine=` is a typed confirmation and must equal the `{id}` in the path.**
Every other confirmation in this API asks whether the operator meant to do
something; this one asks whether they meant to do it *here*. A restore aimed at
the wrong control-plane node makes that node's data the cluster's and discards
the rest, and the two nodes are one dropdown apart. A mismatch is **400
`validation.failed`** with `machine` named in `errors`.

**`?skip_hash_check=true`** turns off the snapshot's integrity check on the node.
It exists for one case: a copy of a node's etcd data directory has no hash to
check, and that is what is left on a cluster that had already lost quorum (see
the snapshot fallback above). For a snapshot this API produced, it skips the one
check that would have caught a truncated upload.

The route is **destructive** and therefore behind the sudo window. It is **not**
`Streaming`: that flag is about the response, and the response is a small JSON
object. What it needs instead is the server's *read* deadline cleared for the
request, which the handler does explicitly — an etcd database is as large as it
is, and a read deadline on this route is a bound on how large a cluster may be
before it stops being restorable.

Two refusals happen before anything is sent, because a refusal that had already
uploaded something would be a refusal that changed the cluster: a node that is
not a control-plane node, and a snapshot with no bytes in it.

```json
{ "uploaded_bytes": 4194304, "notice": "Restoring replaces the cluster's etcd …" }
```

`uploaded_bytes` is the whole of the verdict a client can check for itself,
which is the same reason the snapshot download reports a length.

**A failure between the upload and the bootstrap is named rather than
generalised.** The upload leaves the snapshot on the node and changes nothing;
only the bootstrap that follows replaces etcd. When the bootstrap fails, the
detail says the snapshot was uploaded and etcd was not restarted from it, so the
cluster is still running the data it had — which is the better of the two places
to be standing, and an operator told only "restore failed" cannot tell it from
the other one.

**What it does not do:** the other control-plane nodes still hold the etcd that
was just replaced and will not agree with the recovered member. They have to be
reset and rejoined. Nothing in this API does that.

### Removing a node from a cluster

`POST /api/v1/machines/{id}/remove-from-cluster` does three things in an order
where each one is there because skipping it leaves something behind: the node
leaves etcd (from the node itself, so it forfeits leadership and removes itself
rather than being removed by a peer while still running), it is reset to its
system disk, and it is forgotten from the inventory.

**Cordon and drain are not performed**, and the response says so. holzkube-manager
speaks the Talos machine API and not the Kubernetes API. Anything still scheduled
on the node stops when it does.

**A control-plane node's removal is gated on etcd's membership**, read before
anything is changed, and refused with `409 conflict.last-voting-member` when the
cluster does not survive losing a voter. Two cases, and the refusal says which:

- **the only member** — removing it does not make the cluster smaller, it ends
  it. There is no quorum left to rejoin and no member to add one through, and
  the way back is a restore from a snapshot. To take that node out of service,
  reset it.
- **one of two** — the cluster stops accepting writes and the Kubernetes API
  stops with it. Recoverable by adding a control-plane node back, which the
  first case is not.

The gate and the confirmation are not the same thing and neither substitutes for
the other. This route shipped with the hostname confirmation and no membership
read at all, so a one-node cluster was one correctly-typed hostname from a wipe:
a confirmation dialog asks whether somebody meant what they already clicked, and
a gate is the part that knows what it costs. Both are cheap and only one of them
can answer a question about etcd.

A **worker's removal reads no membership**, deliberately: it is not a member,
and a cluster whose etcd cannot be reached must not be a cluster whose workers
cannot be removed.

The upgrade gate's single-node exemption does not reach this route. A cluster of
one is exempt there because it has no quorum to lose and refusing would make the
smallest homelab unupgradeable — and the node comes back. Nothing comes back
from a removal.

### The per-node lock

`POST /api/v1/machines/{id}/lock` marks a node rolling operations skip (UPG-14).
A locked node is walked past, not failed: the lock is somebody saying "not this
one", and stopping a whole upgrade because of it would make the lock a blunt
instrument nobody uses.

It requires a **reason**. A lock nobody can explain is a lock the next person
clears because it is in the way.

It is deliberately **not** behind the cluster's read-only lock and it reaches no
node, so it works on a machine that is down — which is precisely when somebody
wants to set one.

### Codes minted here

| code | HTTP | when |
|---|---|---|
| `conflict.upgrade-blocked` | 409 | the plan this run was built from has something in the way |
| `conflict.would-strand-kubernetes` | 409 | this upgrade would leave Kubernetes unsupported |
| `conflict.last-voting-member` | 409 | removing this member would leave etcd without a quorum |
| `conflict.unknown-schematic` | 409 | the node's Image Factory schematic could not be read |
| `conflict.not-upgraded` | 409 | the node came back running something other than what was installed |
| `conflict.node-refused` | 409 | the node answered and the answer was no |

`conflict.node-refused` exists because `talos.KindRejected` deliberately has no
upstream code — "the node refused" is not an availability problem — and without
one every such refusal arrived as `internal.unexpected`, which by contract
carries no detail. On the etcd routes it almost always means etcd is not running
on the node that was asked.

### Two guards that had holes

`cmd/holzkube-managerd/allowlist_test.go` kept its **own copy** of the route
table, so this phase's routes were added to `main` and not to the test: the guard
walked every route except the new ones and passed. There is now one `routeTable`
function and the test calls it.

`budget_test.go` had no completeness check at all, so a route with upstream calls
and no row composed to whatever it composed to. The new guard found two routes
from phase 3 — cluster create and talosconfig — that had never had a row.

Both are the same failure: a guard with its own copy of the thing it guards goes
quiet exactly when something is added.

## Operations

### The subcommands

| command | what it does |
|---|---|
| `holzkube-managerd` | serves (the default; no subcommand needed) |
| `holzkube-managerd backup [--label X]` | writes a tarball into the data directory |
| `holzkube-managerd backups` | lists what is there, newest first |
| `holzkube-managerd restore FILE` | backs up what is there, then unpacks |
| `holzkube-managerd verify-audit` | checks the audit hash chain; exits non-zero on a break |
| `holzkube-managerd break-glass [--ttl=15m]` | mints a short-lived admin token on this machine |

They are subcommands of the same binary rather than a separate tool because the
backup format, the permission rules and the chain's hashing all live in this
build. A backup written by one version and refused by another is not a backup.

### break-glass, and why it is allowed to exist

`break-glass` prints an admin bearer token without anybody signing in. Stated
that plainly it sounds like a back door, so the reasoning has to be stated too.

**It grants nothing new.** Whoever can open the data directory can already read
every cluster secret in it, every session record and every password hash: the
directory IS the authority. What this adds is not access -- it is a supported
and AUDITED way to use access somebody already has, in place of the unsupported
one, which is hand-editing the store and leaves no record at all.

**That reasoning is also its limit.** It holds only because this is a subcommand
operating on a path. Over the network, or from an account that could not already
read the directory, the same act would be a back door -- so no route carries it,
and `TestBreakGlassIsNotAThingTheServerServes` is what keeps one from appearing.

Four things make it defensible in practice:

- **It expires.** Fifteen minutes by default, a day at the very most. A
  credential handed out with no decision behind it must not outlive the errand;
  this is the only reason `token_expires_at` exists on an account at all, and an
  ordinary service account still never expires, because an expiry nobody is
  awake to renew is an outage rather than a safeguard.
- **It is one account, reused.** Minting again rotates the same identity and
  invalidates the previous token, rather than leaving a trail of admin accounts
  nobody remembers creating.
- **It is named `break-glass`**, deliberately obviously, in every list and in
  the archive. A credential of this kind hiding under an innocuous name would be
  the difference between a tool and a back door.
- **The act is recorded before it happens**, with the local user's name, and the
  outcome afterwards -- so a failed attempt is visible too, which is the one an
  operator would most want to see.

The token goes to stdout alone and the explanation to stderr, so
`TOKEN=$(holzkube-managerd break-glass)` picks up a credential and not a
sentence about expiry. Revoke it early by deleting the account.

All of them take the same `--data-dir` and `HOLZKUBE_MANAGER_DATA_DIR` the
server takes, through the same loader.

### A backup is safe while the server runs; a restore is not

**Backup takes no lock.** Every record in the store is written atomically —
temporary file, fsync, rename — so a tarball taken mid-write captures either the
old record or the new one and never half of one. The alternative, refusing to
back up while the server runs, is a backup subcommand nobody runs.

**Restore refuses a directory another process holds.** It takes the store's own
process lock to find out, because that lock is what makes "one writer" true:
restoring underneath a running instance would replace the files it has open with
different ones carrying the same names.

### A restore backs up what it is about to replace

Before a byte is unpacked, the current contents are written to a
`pre-restore-*.tar.gz`. A restore that went wrong without one would have
replaced a working installation with a broken one and left nothing to go back
to. The path is printed whether or not the restore then succeeds.

### Every tar entry is checked against the destination

A path in a tarball is whatever the person who made it wrote. `../../../etc/shadow`
is a valid tar entry, and so is an absolute path.

The check is on the **resolved** path rather than on the text, because
`a/../../b` contains no leading `..` and still escapes. An entry that resolves
outside the data directory is `ErrOutsideDestination` and nothing is written.

Symlinks, hard links and devices are **refused**, not skipped. A symlink in an
archive is a path the next write follows, and silently dropping one produces a
restored directory that is missing something nobody is told about.

Restored file modes are narrowed to `0700`, never widened. A restore that
produced a `0644` would produce a data directory `fsstore` then refuses to open
— which is a restore that appeared to work.

### Pre-releases are opt-in

A node reporting `v1.14.0-rc.2` is **inside** the supported window and is
refused anyway, with `talos.ErrPreRelease`, unless the instance was started with
`--allow-prerelease` / `HOLZKUBE_MANAGER_ALLOW_PRERELEASE=true`.

The two refusals are separate errors because the remedies are opposite kinds of
thing. "This node is outside the supported range" is a fact about the node and
the answer is to change the node; "this instance does not accept pre-releases"
is a setting, and a client showing them as one refusal would send somebody to
reinstall a node they deliberately put a release candidate on.

`v1.14.0+dirty` is build metadata, not a pre-release: a release built from a
modified tree, which must not be refused as one.

### A node outside the range is marked, not hidden

`MachineView` carries `unsupported_version`, `pre_release` and
`version_notice`. All three are derived from the **last version the node
reported**, not from a failed connection — a node outside the range is refused
at connect time, so a marking that depended on connecting would be blank for
exactly the nodes it exists to mark.

A machine whose snapshot carries no version is left unmarked and gets no notice.
"We have never heard a version from this node" is not "this node is fine", and
inventing a verdict about it would be the marking saying something nobody knows.

`GET /api/v1/system/status` serves `talos_range` and `allow_prerelease` so the
screen and the refusal agree; a copy in the browser bundle would drift from the
constants that enforce it.

### There is no backup button

A backup contains every secret in the data directory verbatim, and the file
lands on the **server's** disk rather than the operator's. A button would
produce a file somebody then has to find over SSH anyway — and one that streamed
the archive to the browser would be an endpoint that hands the cluster over to
whoever has a session.

### The container

Non-root (uid 65532) from `scratch`, with the data directory as a declared
volume. Both properties are asserted by `cmd/holzkube-managerd/container_test.go`
against the files themselves, because no Docker daemon runs in CI here and a
comment asking people to keep two files in step is not a mechanism.

Scratch rather than alpine or distroless: the binary is static, embeds its own
web assets, and verifies Talos endpoints against the cluster PKI it holds rather
than against a system trust store. A shell, a package manager and a CA bundle
would each be a way in this product has no use for.

The Compose file binds to `127.0.0.1` by default. The dashboard shows every
node's state and the API can wipe a machine; publishing that on a LAN address is
a deliberate act.
