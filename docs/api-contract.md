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

They are subcommands of the same binary rather than a separate tool because the
backup format, the permission rules and the chain's hashing all live in this
build. A backup written by one version and refused by another is not a backup.

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
