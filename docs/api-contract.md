# holzkube API contract

**This document is binding.** The five wave-2 plans of phase 1 work against it
in parallel without further coordination, and later phases extend it rather
than reinterpret it. Where this document and an implementation disagree, the
disagreement is a bug in one of them — not a matter of taste.

All API routes live under `/api/v1/`. Requests and responses are JSON. Errors
are always RFC 9457 `application/problem+json`.

## Error Taxonomy

The taxonomy is **closed and stable**. Every `type` is an absolute URI under
`https://holzkube.dev/problems/`; `about:blank` is never used. Every response
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
| `setup-required` | 503 | `setup.required` | no operator account exists yet |

### Response shape

```json
{
  "type": "https://holzkube.dev/problems/conflict",
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
| `GET` | `/api/v1/auth/me` | false | true | — | `200 {"id","username"}` |
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
sudo window          -> 428 sudo.required
audit intent durable -> 500 internal.unexpected   (request is refused, not performed)
handler validation   -> 400 validation.failed
handler conflict     -> 409 store.conflict
```

The audit step is fail-closed on purpose: a mutation that cannot be recorded is
not performed. An unlogged mutation is the outcome the audit log exists to
prevent.

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
2. `X-Holzkube-CSRF: 1` — the value is checked, not just the header's presence.
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
  "audit_chain": { "ok": true, "broken_at_line": 0, "file": "/path/audit/audit-2026-08-28.jsonl" }
}
```

- `setup_required` — no operator account exists yet. While it is true, every UI
  route redirects to `/setup` (D-01).
- `audit_chain.ok` — `false` means the hash chain does not verify.
  `broken_at_line` is the **1-based** line number of the first record that does
  not verify, and is `0` when `ok` is true. `file` names the file checked.
- A break found at startup stays reported for the life of the process; while
  startup was clean the endpoint re-verifies live, so damage occurring during
  the run is not hidden until the next restart (D-15). Startup verification
  covers the current day's file **and the one rotated before it**, so `file` is
  not necessarily today's.
- There is **no** endpoint and no parameter that acknowledges, clears or
  recomputes the verdict. A break disappears only by dealing with the file by
  hand and restarting. A chain that repairs itself is worse than no chain: it
  destroys the evidence that it was broken.
- The UI renders a break as a persistent banner. It is not dismissible: a hash
  chain nobody looks at is theatre.

## Route Registration Rule

A wave-2 plan adds a route **without touching `router.go`**:

1. Add the handler and its `httpapi.Route` entry to **your own file** under
   `internal/httpapi/handlers/`, returned from that file's `…Routes(deps)`
   function.
2. If the file is new, add its `…Routes(deps)` call to the `slices.Concat` in
   `cmd/holzkubed/main.go` — one line.

`router.go` owns the `Route` type, the mounting and the middleware wiring, and
is not edited again by phase-1 plans. Two plans adding routes never touch the
same lines.

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
