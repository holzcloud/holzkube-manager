---
phase: 01-foundation-skeleton
reviewed: 2026-08-28T00:00:00Z
depth: standard
files_reviewed: 46
files_reviewed_list:
  - cmd/holzkubed/main.go
  - internal/audit/audit.go
  - internal/audit/chain.go
  - internal/audit/query.go
  - internal/audit/record.go
  - internal/audit/redact.go
  - internal/audit/rotate.go
  - internal/auth/argon.go
  - internal/auth/auth.go
  - internal/auth/ratelimit.go
  - internal/auth/scsstore/scsstore.go
  - internal/auth/session.go
  - internal/auth/sudo.go
  - internal/config/config.go
  - internal/config/datadir.go
  - internal/depguard_test.go
  - internal/httpapi/handlers/account.go
  - internal/httpapi/handlers/audit.go
  - internal/httpapi/handlers/auth.go
  - internal/httpapi/handlers/handlers.go
  - internal/httpapi/handlers/setup.go
  - internal/httpapi/handlers/system.go
  - internal/httpapi/middleware/audit.go
  - internal/httpapi/middleware/authn.go
  - internal/httpapi/middleware/chain.go
  - internal/httpapi/middleware/csrf.go
  - internal/httpapi/middleware/recover.go
  - internal/httpapi/middleware/requestid.go
  - internal/httpapi/middleware/session.go
  - internal/httpapi/middleware/sudo.go
  - internal/httpapi/problem.go
  - internal/httpapi/router.go
  - internal/httpapi/web.go
  - internal/model/model.go
  - internal/store/fsstore/atomic.go
  - internal/store/fsstore/fsstore.go
  - internal/store/fsstore/permissions.go
  - internal/store/lock.go
  - internal/store/migrate/backup/backup.go
  - internal/store/migrate/migrate.go
  - internal/store/store.go
  - internal/tlsx/load.go
  - internal/tlsx/selfsigned.go
  - web/src/api.ts
  - web/src/components/SudoDialog.tsx
  - web/src/routes/audit.tsx
findings:
  critical: 3
  warning: 19
  info: 0
  total: 22
status: issues_found
---

# Phase 1: Code Review Report

**Reviewed:** 2026-08-28
**Depth:** standard
**Files Reviewed:** 46 (hand-written source; generated `web/src/components/ui/*` and test fixtures inspected only for integration errors)
**Status:** issues_found

## Summary

The security-critical surface is unusually well reasoned — the argon2id calibration, the
per-entity lock refcounting, the `heldResponse` buffering that lets the sudo window refresh
before `scs` commits, the allowlist redactor, and the atomic-write sequences are all correct
and defensible. The defects that remain are of the class the scope notes asked for: code that
passes its own tests while being wrong at a seam its tests do not cross.

Three findings block. Two of them sit on the single unauthenticated endpoint
(`GET /api/v1/system/status`), which the design notes claim exposes nothing sensitive and
which in fact both leaks the absolute data-directory path and hands an unauthenticated caller
a lever on the audit writer's global mutex. The third is in `migrate`, whose entire stated
purpose is preventing silent data loss and which unconditionally stamps `VERSION` as current
whether or not any migration ran.

The remaining nineteen warnings cluster around three themes: (1) the audit subsystem's
invariants are stated in comments but not enforced at every path into it (`Outcome` bypasses
the redactor, gate denials produce no record, `pending` leaks); (2) the crash-tolerance claims
in `rotate.go` and `fsstore` are not all true (duplicate day files break the reader; tlsx
temporaries containing private key material are never swept); and (3) the HTTP server is
missing the ordinary hardening a network-facing binary holding cluster PKI should have
(no read/write/idle timeouts, no CSP, no `X-Frame-Options`, no `Host` allowlist).

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: Unauthenticated endpoint re-hashes the audit archive while holding the audit writer's mutex

**File:** `internal/httpapi/handlers/system.go:34-48`, `internal/audit/audit.go:250-262`, `internal/httpapi/router.go:118-123`

**Issue:** `GET /api/v1/system/status` is declared `RequiresSession: false` (`system.go:21`).
When the startup verdict was clean it calls `d.Audit.Verify(r.Context())` on **every request**.
`Logger.Verify` takes `l.mu` — the same mutex `append` takes — and then reads, JSON-decodes and
SHA-256s the current day's file plus the one before it, transparently gunzipping if needed
(`rotate.go:219-257`, `chain.go:52-92`). Three consequences, all reachable by an unauthenticated
caller with no rate limit in front of them:

1. **Cost scales with the archive.** D-16 keeps files forever and today's file grows all day.
   Every anonymous request pays a full re-read plus a per-record `ComputeHash`. `readFile`
   materialises the entire file as `[]Record` in memory first (`rotate.go:239-252`).
2. **Audit-write starvation.** Because `Verify` holds `l.mu`, a flood of anonymous status
   requests serialises against `Logger.Attempt`. The audit middleware is fail-closed
   (`middleware/audit.go:77-83`) — a mutating request whose intent cannot be recorded is
   *refused with 500*. So an unauthenticated caller can stall or fail every authenticated
   mutation in the process.
3. **Cancellation is ignored.** `Verify(_ context.Context)` discards the context
   (`audit.go:250`), so a client that disconnects does not release the work; the mutex stays
   held to completion.

The UI amplifies this on its own: `useSystemStatus` polls at `refetchInterval: 60_000` with
`refetchOnWindowFocus: true` (`web/src/hooks/useSession.ts:20-21`), and `ChainBannerContainer`
plus `Dashboard` both subscribe.

**Fix:** Put the live re-verification behind the session gate, and cache it. Minimum viable
change — gate it and stop holding the writer lock for the read:

```go
// system.go
{
    Method:          http.MethodGet,
    Pattern:         "/api/v1/system/status",
    RequiresSession: false,
    ...
    Handler: handler(func(w http.ResponseWriter, r *http.Request) {
        users, err := d.Store.Users().List(r.Context())
        ...
        // Anonymous callers get the startup snapshot only. Live re-verification
        // is a privileged, throttled operation.
        chain := d.AuditChain
        if d.Auth.IsAuthenticated(r.Context()) && chain.OK && d.Audit != nil {
            chain = d.Audit.CachedVerify(r.Context()) // memoised, min interval e.g. 30s
        }
        writeJSON(w, http.StatusOK, systemStatus{SetupRequired: len(users) == 0, AuditChain: chain})
    }),
}
```

and in `audit`, add a `CachedVerify` that (a) snapshots the file list under `l.mu`, releases
it, and verifies outside the lock, and (b) honours `ctx.Err()` between records the way
`Query` already does at `query.go:74-76`.

---

### CR-02: The absolute data-directory path is disclosed before authentication

**File:** `internal/httpapi/handlers/system.go:45-53`, `cmd/holzkubed/main.go:106-129`, `internal/audit/audit.go:115-119`

**Issue:** `systemStatus.AuditChain.File` is `auditLog.CurrentFile()` =
`filepath.Join(l.dir, ...)` where `l.dir` is `<data-dir>/audit` and `<data-dir>` is the
XDG-resolved **absolute** path (`config/datadir.go:35-51`, `fsstore.Open` calls
`filepath.Abs`). The endpoint is `RequiresSession: false`. An unauthenticated caller therefore
receives, for example:

```json
{"setup_required":false,"audit_chain":{"ok":true,"broken_at_line":0,
 "file":"/home/holz/.local/share/holzkube/audit/audit-2026-08-28.jsonl"}}
```

which discloses the OS username, the home directory layout, and the exact location of the
directory the threat model calls "equivalent to root on every managed node".

This directly contradicts the stated design invariant ("`GET /api/v1/system/status`
deliberately does not expose version, bind address or data directory — it answers before
authentication") and it contradicts the project's own error taxonomy, which goes out of its
way to strip exactly this class of string: `Internal(err)` discards the error precisely
because "a Go error string routinely carries a filesystem path … leaking any of that to a
client is free reconnaissance" (`problem.go:193-210`). `docs/api-contract.md:279` documents
the full path in the example response, so the contract is wrong too and needs the same edit.

**Fix:** Send only the basename before authentication; the operator does not need the
directory they configured, and a break is dealt with by hand on a host they are already
logged into.

```go
// httpapi: ChainStatus.File carries a file name, never a path.
chain.File = filepath.Base(chain.File)
```

Apply it in `main.go` when building `Deps.AuditChain`, and in `system.go` after the live
re-verify. Update `docs/api-contract.md` § System Status Contract to show
`"file": "audit-2026-08-28.jsonl"`. `web/src/components/ChainBanner.tsx:35` and
`web/src/routes/index.tsx:81` render it verbatim and need no change.

---

### CR-03: `migrate.run` stamps VERSION as current even when no migration ran

**File:** `internal/store/migrate/migrate.go:91-104`, `internal/store/migrate/migrate.go:137-141`

**Issue:** The loop tracks `at` but the final write ignores it:

```go
at := from
for _, m := range migs {
    if m.From != at { continue }
    if err := m.Apply(dir); err != nil { ... }
    at = m.To
}
return writeVersion(dir, CurrentVersion)   // <- unconditional
```

If `at != CurrentVersion` after the loop — a gap in the table, an out-of-order entry, or a
`from` value no migration starts at — the directory is nonetheless marked as fully migrated.
Every subsequent start then takes the `from == CurrentVersion` fast path at line 79 and never
looks again. The records are stale in a shape the binary believes is current, which is the
exact "silent data loss" the package header says it exists to prevent, and it is silent by
construction: the pre-migration tarball exists but nothing indicates it should be restored.

This is reachable today, not only in phase 3. `readVersion` rejects only `v < 0`
(`migrate.go:138`), so a `VERSION` file containing `0` yields `from = 0`. `0 < CurrentVersion`,
`migrations` is empty, `at` stays `0`, and the directory is stamped `1`. The doc comment two
functions up asserts the opposite — "version 1 — not version 0, because no release ever wrote
a version 0 layout" — but the parser accepts what the comment says cannot exist.

**Fix:** Refuse to stamp a version the migrations did not actually reach, and reject `0` on
read.

```go
	if at != CurrentVersion {
		return fmt.Errorf(
			"migrate: no migration path from version %d to %d; this data directory cannot be "+
				"advanced by this binary and has been left untouched (backup written)", at, CurrentVersion)
	}
	return writeVersion(dir, CurrentVersion)
```

```go
	v, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || v < 1 {
		return 0, fmt.Errorf("%w: %s contains %q", ErrVersionUnreadable, path, strings.TrimSpace(string(raw)))
	}
```

## Warnings

### WR-01: `Logger.pending` grows without bound when a handler panics

**File:** `internal/audit/audit.go:189-200`, `internal/httpapi/middleware/audit.go:85-91`

**Issue:** `Attempt` stores the record in `l.pending[seq]` and only `Outcome` deletes it. The
audit middleware deliberately does **not** defer the outcome write: "A panic never reaches
this line. That is deliberate." That is a sound forensic choice for the *log*, but the
in-memory `pending` entry is then leaked for the life of the process, and each entry holds a
whole `Record` including its `Params` map. Any panic-triggering input — and the `Recover` link
exists because panics are expected to happen — is a repeatable allocation an attacker with a
session can drive. This is a sibling of the per-entity mutex map that was already found and
fixed in `store.EntityLocks` (`lock.go:106-148`), where the same shape was correctly
refcounted and deleted.

**Fix:** Bound it. Either evict by age under `l.mu` on each `Attempt`, or cap the map and drop
the oldest — the entry is only needed to correlate the outcome, which arrives within one
request:

```go
// in append/Attempt, under l.mu
const pendingTTL = 5 * time.Minute
for seq, rec := range l.pending {
    if now.Sub(rec.TS) > pendingTTL {
        delete(l.pending, seq)
    }
}
```

Dropping the entry does not weaken the design: the un-outcomed intent is already durable on
disk, which is the signal the design wants.

---

### WR-02: `Logger.Outcome` writes an error string into `params` without passing the redactor

**File:** `internal/audit/audit.go:221-224`

**Issue:**

```go
	params := map[string]any{}
	if cause != nil {
		params["error"] = cause.Error()
	}
```

`redact.go:10-12` asserts the package invariant: "There is no list of exclusions in this
package and there is no branch that passes an unrecognised value through; both absences are
asserted by the plan's gate greps." This is that branch. It is safe *today* only because the
one caller passes `errors.New(sniffer.code())` with the value shape-checked by `isCodeToken`
(`middleware/audit.go:98`, `:213-225`) — a validation that lives in a different package from
the invariant it upholds, and that the gate greps do not cover. `Outcome` is exported. The
first wave-2 caller that passes a real `error` writes a filesystem path or a store message
into an archive with no deletion path.

Note also that the value is not length-capped: `capLength` (`redact.go:135-146`) only runs
inside `Params`.

**Fix:** Route it through the same door as everything else, and make the guarantee local:

```go
	params := map[string]any{}
	if cause != nil {
		// Same allowlist door as an intent's params. An outcome names a taxonomy
		// code, never free text, and the redactor is what enforces that here
		// rather than at whichever call site happened to remember.
		params = Params(att.Action+".outcome", map[string]any{"error": cause.Error()})
	}
```

with `"<action>.outcome": {"error"}` entries in `allowlist`, or a dedicated
`redactOutcomeCause(string) string` that applies `isCodeToken`-style validation and
`capLength` inside the audit package.

---

### WR-03: A crash during compression leaves two files for one day; the reader does not tolerate it

**File:** `internal/audit/rotate.go:146-161`, `internal/audit/rotate.go:191-215`, `internal/audit/chain.go:52-92`, `internal/audit/query.go:73-97`

**Issue:** `compressFile` renames the temporary to `X.jsonl.gz`, fsyncs the directory, and
*then* removes `X.jsonl`. The comment says a crash in that window "leaves both files, which
the reader tolerates". The reader does not tolerate it. `allFiles` matches both suffixes
(`rotate.go:191-195`) and `sort.Strings` puts `audit-D.jsonl` immediately before
`audit-D.jsonl.gz` because the former is a byte-prefix of the latter. Both are then read:

- **`Query`** (`query.go:73-97`) walks the file list and emits every matching record from both
  copies. The audit page shows each of that day's records twice, with duplicate `seq` values —
  which in `web/src/routes/audit.tsx:221` are used as React keys.
- **`Verify`** (`chain.go:52-92`) processes the plain file, sets `prev` to that day's last
  hash, then starts the `.gz` whose first record carries the *previous* day's hash as
  `prev_hash`. `rec.PrevHash != prev` at `chain.go:77` → **false chain break reported**, at a
  file the operator will find is perfectly intact. D-15 says a break stays reported until the
  file is dealt with by hand and cannot be cleared, so this is a permanent false alarm on a
  banner the design deliberately made non-dismissible.

`os.Remove(path)` failing for any reason (`rotate.go:157`) makes the state permanent rather
than momentary.

**Fix:** Make the reader authoritative about which copy wins, so the crash window is genuinely
tolerated:

```go
// listFiles: collapse a day that has both forms, preferring the compressed one
// (it is the file the rename made durable; the plain original is the leftover).
func dedupeByDay(paths []string) []string {
	seen := make(map[string]int, len(paths))
	out := paths[:0:0]
	for _, p := range paths {
		day := dayOf(p)
		if i, ok := seen[day]; ok {
			if strings.HasSuffix(p, compressedExt) {
				out[i] = p
			}
			continue
		}
		seen[day] = len(out)
		out = append(out, p)
	}
	return out
}
```

Call it at the end of `allFiles`. Keep `plainFiles` as-is so `CompressOlderThan` still finds
and finishes the orphan on the next rotation.

---

### WR-04: Every gate denial is unaudited — including the one the threat model cares most about

**File:** `internal/httpapi/router.go:108-130`

**Issue:** `wrapRoute` composes `CSRF -> Authn -> Sudo -> Audit`, outermost first, so the audit
link is the innermost. Every refusal produced by the three gates short-circuits before
`middleware.Audit` runs, and therefore produces **no record at all**:

- `403 csrf.precondition-unmet`
- `401 auth.unauthenticated` on a session-required route
- `428 sudo.required` on a destructive route

The third is the problem. `middleware/sudo.go:16-18` names the sudo window as "the only
mechanism that limits the damage a stolen session cookie can do (T-01-25)". A 428 is precisely
the event "somebody holding a session cookie tried a destructive action and could not produce
the password" — the highest-signal forensic event in the phase-1 threat model — and it is
invisible in the log the product exists to keep. The Audit middleware's own doc comment states
the opposing principle: "Recording only successes throws away exactly the forensics that
matter, so both halves are written."

(Failed *logins* are recorded, because `/api/v1/auth/login` is `RequiresSession: false` and
`Destructive: false`, so no gate precedes the audit link. That is the accident of route
configuration, not a property of the chain.)

**Fix:** Either move the audit link outward so it wraps the gates, or give the gates their own
recorder. The first is cleaner and preserves the existing intent/outcome pairing:

```go
	inner := middleware.Chain(
		middleware.Audit(auditAdapter{deps: d}, rt.Action, middleware.IsMutating(rt.Method), ...),
		middleware.CSRF(...),
		middleware.Authn(...),
		middleware.Sudo(...),
	)
```

This does change the documented order in `docs/api-contract.md:96-107` (the audit intent
becomes durable *before* the CSRF check rather than after the sudo check) and shifts the
fail-closed 500 earlier, so the contract needs the matching edit. If that trade is unwanted,
add a narrow `deny` hook on each gate that writes a single terminal record.

---

### WR-05: `CalibrateParams` divides by a measurement that can be zero

**File:** `internal/auth/argon.go:85-92`, `internal/auth/argon.go:108-125`

**Issue:** `measureHash` returns `0` when `argon2id.CreateHash` fails, deliberately, so that
"unusable parameters must not read as instant". But the caller immediately does:

```go
	scaled := float64(p.Iterations) * calibrationMargin * float64(target) / float64(measured)
	next := uint32(math.Ceil(scaled))
```

With `measured == 0` this is `+Inf`, and `uint32(+Inf)` is **implementation-defined** per the
Go spec for out-of-range float-to-integer conversion. Depending on target and toolchain it may
yield `0` (falling into the `next <= p.Iterations` guard and incrementing by one) or
`0xFFFFFFFF` (clamped to `maxCalibrationIterations`). Neither is intended, both are silent,
and this is the code path that decides how expensive a stolen password hash is to attack.

**Fix:** Handle the sentinel explicitly, before it becomes a divisor:

```go
	for range maxCalibrationRounds {
		if measured >= target {
			return &p, measured
		}
		if p.Iterations >= maxCalibrationIterations {
			return &p, measured
		}

		next := p.Iterations + 1
		if measured > 0 {
			// The ratio step only means anything with a real measurement.
			// A zero is a failed probe, not an instant one; step conservatively.
			scaled := float64(p.Iterations) * calibrationMargin * float64(target) / float64(measured)
			if scaled >= float64(maxCalibrationIterations) {
				next = maxCalibrationIterations
			} else if n := uint32(math.Ceil(scaled)); n > next {
				next = n
			}
		}
		...
	}
```

---

### WR-06: `http.Server` has no read, write or idle timeout

**File:** `cmd/holzkubed/main.go:143-147`

**Issue:** Only `ReadHeaderTimeout` is set. There is no `ReadTimeout`, `WriteTimeout`,
`IdleTimeout` or `MaxHeaderBytes`. A caller that sends valid headers and then dribbles a body
one byte per minute holds a connection and a goroutine indefinitely — `MaxBytesReader`
(`handlers/handlers.go:27`) caps the *size* of a body, never the *time* taken to send it. The
same is true of a client that never reads the response. Combined with `throttle` parking
connections for up to 500 ms (`handlers/auth.go:98-103`) and CR-01's unbounded status handler,
this is the resource-exhaustion side of a binary whose own comment at `handlers/auth.go:16-18`
says "a connection parked for half a minute is itself a resource an attacker can spend cheaply
and we cannot".

**Fix:**

```go
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           httpapi.New(deps),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second, // > maxInlineDelay + slowest handler
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}
```

---

### WR-07: No CSP, no `X-Frame-Options`, no `Referrer-Policy`, no HSTS

**File:** `internal/httpapi/web.go:57-67`, `internal/httpapi/problem.go:236-237`, `internal/httpapi/handlers/handlers.go:42-43`

**Issue:** `grep -rn "Content-Security-Policy\|X-Frame-Options\|Referrer-Policy\|Strict-Transport"`
across the repository returns nothing. `X-Content-Type-Options: nosniff` is set on JSON and
problem responses but **not** on `index.html` or on anything `http.FileServer` serves
(`web.go:33`, `:64-66`). `index.html` also carries an inline `<script>` block
(`web/index.html:8-28`), so any CSP added later needs a hash or nonce for it — cheaper to
decide now than after the bundle ships.

The absence of `frame-ancestors 'none'` / `X-Frame-Options: DENY` leaves the console
frameable. `SameSite=Lax` on the session cookie (`auth/session.go:45`) blunts but does not
eliminate this, and it does nothing for an XSS in a page that has no CSP to contain it — in a
binary that holds cluster PKI.

**Fix:** One link at the top of the outer chain, applied to everything:

```go
func SecurityHeaders(https bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Content-Security-Policy",
				"default-src 'self'; script-src 'self' 'sha256-<index.html inline hash>'; "+
					"style-src 'self' 'unsafe-inline'; img-src 'self' data:; "+
					"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
			if https {
				h.Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

Wire it first in `httpapi.New`'s `outer` chain (`router.go:94-101`). Consider also
`Cache-Control: no-store` in `writeJSON` — `/api/v1/auth/me` returns the operator's username
with no cache directive at all.

---

### WR-08: The CSRF origin check is self-referential — no `Host` allowlist

**File:** `internal/httpapi/middleware/csrf.go:114-146`

**Issue:** `checkOrigin` compares the attacker-supplied `Origin` header against the
attacker-supplied `Host` header (`r.Host`). Nothing anywhere validates that `Host` is one this
instance answers to. Under DNS rebinding — the standard attack against a loopback-bound admin
tool — a victim's browser resolving `evil.example` to `127.0.0.1` sends
`Host: evil.example` **and** `Origin: https://evil.example` **and**
`Sec-Fetch-Site: same-origin`, because from the browser's point of view it *is* same-origin.
All three preconditions pass and the request reaches the handler with the victim's cookie.

By default TLS mitigates this: the browser would show a certificate error for `evil.example`
and the generated SAN list is `localhost` + the hostname + the two loopback IPs
(`tlsx/selfsigned.go:127-128`, `:145-157`). Under `--insecure-http` there is no mitigation at
all, and `--insecure-http` on loopback is exactly the configuration DNS rebinding targets.

**Fix:** Add a `Host` allowlist derived from the bind address and the certificate SANs, checked
before the origin comparison:

```go
func checkHost(r *http.Request, allowed map[string]struct{}) error {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if _, ok := allowed[strings.ToLower(strings.Trim(host, "[]"))]; !ok {
		return errors.New("Host " + r.Host + " is not an address this instance serves")
	}
	return nil
}
```

Populate `allowed` in the composition root from `cfg.Listen`, `localhost`, `127.0.0.1`, `::1`
and `os.Hostname()` — the same set `tlsx.dnsNames` already computes.

While here: `tlsx.dnsNames` omits the bind address itself. An operator running
`--listen 192.168.1.5:8443` gets a SAN mismatch on top of the self-signed warning, which is a
second, differently-worded warning the D-04 rationale explicitly wants to avoid. Add
`cfg.Listen`'s host to `IPAddresses`/`DNSNames`.

---

### WR-09: Expired session records are never reaped from disk

**File:** `internal/auth/scsstore/scsstore.go:30-45`, `internal/store/fsstore/fsstore.go:341-420`

**Issue:** `scs` runs no background cleanup for a user-supplied `Store`; the built-in stores
each start their own goroutine and `scsstore` starts none. The only deletion paths are
`Find` (which deletes a session it is asked about after expiry) and `Delete`. A session record
that is never looked up again — the normal case, because the token is gone with the browser —
stays in `<data-dir>/sessions/` forever. Every login adds one.

Nothing breaks, but the count only grows, and it grows the cost of the three places that call
`List`: `InvalidateAllExcept` (`auth/session.go:94-108`, on every password change),
`scsstore.All`, and the permission `Guard` walk at every startup
(`fsstore/permissions.go:35-63`). `EntityLocks` was refcounted specifically so that "a
long-lived process writing millions of short-lived session records does not accumulate a mutex
per record forever" (`store/lock.go:108-111`); the records themselves have no equivalent.

**Fix:** Sweep on open, where the flock guarantees a single writer and nothing is serving yet:

```go
// fsstore.Open, after the entity directories exist
if err := s.sessions.reapExpired(context.Background(), time.Now()); err != nil {
	return nil, err
}
```

`reapExpired` lists, drops anything whose `ExpiresAt` has passed, and logs the count. A
periodic ticker in `main.go` guarded by the shutdown context would cover a process that runs
for months, which is the stated target.

---

### WR-10: `ChangePassword` reports failure after the new password is already persisted

**File:** `internal/auth/auth.go:192-216`

**Issue:** The new hash is written at line 211, and *then* `InvalidateAllExcept` runs. If that
returns an error — `Sessions().List` fails if any single session file is unparsable
(`fsstore.go:365-376` propagates the first `json.Unmarshal` error), or any `Delete` hits an I/O
error — `ChangePassword` returns non-nil, `changePassword` maps it to
`WriteInternal` → 500 (`handlers/account.go:69-71`), and the operator is told the change did
not happen. It did. They will retry with the old current-password and get 401, and conclude
they are locked out of the credential guarding their cluster PKI.

**Fix:** The password write is the operation; the invalidation is best-effort cleanup. Log it
and succeed, the same way `rehashIfOutdated` already reasons about a background failure
(`auth.go:143-148`):

```go
	if err := s.InvalidateAllExcept(ctx, s.sm.Token(ctx)); err != nil {
		// The password is already changed and durable. Reporting failure here
		// would tell the operator to retry a change that succeeded, and the
		// retry fails on the old current-password.
		slog.ErrorContext(ctx, "password changed, but other sessions could not all be invalidated",
			slog.Any("error", err))
	}
	return nil
```

---

### WR-11: The reported "1-based line number" is a record index, not a line

**File:** `internal/audit/chain.go:61-62`, `internal/audit/rotate.go:242-252`

**Issue:** `readFile` skips blank lines (`rotate.go:243-246`) but `Verify` reports `i+1` where
`i` indexes the returned `[]Record`. `readFile`'s own decode error message has the same defect
(`rotate.go:249`: `len(out)+1`). Any blank line in a day file desynchronises every subsequent
number from the actual file line.

This matters more here than it would elsewhere. The package doc opens by justifying the JSONL
format on exactly this ground: "A record that spans several lines has no line number to report,
and reporting the line of the first break is how a finding here becomes actionable"
(`audit.go:4-6`). `docs/api-contract.md:285-287` promises `broken_at_line` is "the **1-based**
line number", `ChainBanner.tsx:34` renders it as "line N of <file>", and the operator is then
expected to open that file and look. A trailing newline from a partial write, or a manual edit
that left a blank line, sends them to the wrong record — while investigating tampering.

**Fix:** Have `readFile` return line numbers alongside records.

```go
type located struct {
	Record
	Line int
}

func readFileLocated(path string) ([]located, error) {
	...
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" { continue }
		var rec Record
		if err := json.Unmarshal([]byte(text), &rec); err != nil {
			return nil, fmt.Errorf("audit: decode %s line %d: %w", filepath.Base(path), line, err)
		}
		out = append(out, located{Record: rec, Line: line})
	}
	...
}
```

`Verify` then reports `rec.Line`; `Query` ignores it.

---

### WR-12: tlsx temporary files hold private key material, are never swept, and land in backups

**File:** `internal/tlsx/selfsigned.go:167-207`, `internal/store/fsstore/permissions.go:80-104`, `internal/store/migrate/backup/backup.go:137-145`

**Issue:** `tlsx.writeAtomic` uses its own prefix, `.tmp-tlsx-*` (line 170), while the store's
sweeper and the backup exclusion both key off `store.TempFilePrefix` = `.holzkube-tmp-`
(`permissions.go:86`, `backup.go:144`). The consequences:

1. A crash or a failed `Rename` between `CreateTemp` and the deferred cleanup leaves
   `.tmp-tlsx-XXXXXX` in the data directory containing a **PEM-encoded EC private key**. It is
   0600, so `Guard` passes it, and nothing ever removes it. It accumulates across restarts.
2. `backup.skip` does not exclude it, so every pre-migration tarball captures orphaned private
   keys alongside the live one.
3. Ordering compounds this: `fsstore.Open` (which sweeps) runs at `main.go:84`, `tlsx.Ensure`
   (which writes) at `main.go:153`. Even if the prefix matched, the sweep has already happened.

**Fix:** Use the shared prefix so both the sweeper and the backup exclusion cover it:

```go
	tmp, err := os.CreateTemp(dir, store.TempFilePrefix+"tlsx-*")
```

`safeKey` already rejects the prefix as an identifier (`fsstore.go:151-167`), so there is no
collision risk with a record. If importing `store` from `tlsx` is unwanted, move the constant
to a leaf package both can import.

---

### WR-13: A second sudo challenge abandons the first caller's promise forever

**File:** `web/src/components/SudoDialog.tsx:38-48`

**Issue:** The `settled` ref is documented as guaranteeing a challenge settles exactly once:
"twice would resolve a promise the pipeline already moved past, never would hang the caller
forever." The registration handler produces the second outcome:

```js
onSudoRequired((next) => {
  settled.current = false      // <- the previous challenge is now unsettleable
  setChallenge(next)           // <- and unreachable
})
```

If a second destructive request 428s while the dialog is open, the first `SudoChallenge`'s
`settle` is dropped on the floor. `askForSudo`'s promise (`api.ts:185-188`) never resolves,
`send` never returns, and the caller's `await` hangs for the life of the page — no error, no
toast, a spinner that never stops.

Only one destructive route exists in phase 1, so a single page cannot trigger this today. It
becomes reachable with phase 6's node actions, which the design explicitly plans to gate
through the same dialog.

**Fix:** Settle the outgoing challenge before replacing it.

```js
useEffect(() => {
  onSudoRequired((next) => {
    setChallenge((current) => {
      // A challenge that is being displaced is a challenge that was refused.
      // Leaving it unsettled hangs its caller forever.
      if (current !== null && !settled.current) {
        current.settle(false)
      }
      settled.current = false
      return next
    })
    setPassword('')
    setMessage('')
  })
  return () => onSudoRequired(null)
}, [])
```

Queueing the second challenge instead of displacing the first would also work and is arguably
the better UX; either way the current behaviour must not survive.

---

### WR-14: The audit view's `queryFn` mutates component state, so a refetch duplicates a page

**File:** `web/src/routes/audit.tsx:102-109`

**Issue:**

```js
const page = useQuery({
  queryKey: ['audit', filters, cursor],
  queryFn: async () => {
    const result = await api.audit(queryFor(filters, cursor))
    setPages((previous) => (cursor === null ? [result.items] : [...previous, result.items]))
    return result
  },
})
```

A React Query `queryFn` is not guaranteed to run once per logical page. The root client sets
`refetchOnWindowFocus: true` with `staleTime: 5_000` (`__root.tsx:20-23`), and `StrictMode` is
on (`main.tsx:12-16`). Whenever this query refetches with `cursor !== null` — switching browser
tabs and back is enough — the `else` branch appends the same page again. `records = pages.flat()`
then contains each of those audit records twice, rendered with `key={record.seq}`
(`audit.tsx:221`): duplicate React keys, and a forensic table that shows the same event twice.

`isOrphanedIntent` reads the same duplicated array, so duplicates also change which rows are
flagged.

**Fix:** Derive the accumulated list instead of writing to state from inside the query, or
merge idempotently by `seq`:

```js
setPages((previous) => {
  if (cursor === null) return [result.items]
  const seen = new Set(previous.flat().map((r) => r.seq))
  const fresh = result.items.filter((r) => !seen.has(r.seq))
  return fresh.length === 0 ? previous : [...previous, fresh]
})
```

`useInfiniteQuery` with `getNextPageParam: (last) => last.next_cursor ?? undefined` removes the
whole hand-rolled accumulator and is the idiomatic fix.

---

### WR-15: `isOrphanedIntent` hides real orphaned intents

**File:** `web/src/routes/audit.tsx:248-253`

**Issue:**

```js
return !all.some((other) => other.seq > record.seq && other.action === record.action)
```

The predicate is "does *any* later record share this action", not "does this attempt have its
outcome". Two failure modes, both false negatives, both in the wrong direction:

- A genuinely orphaned `attempt auth.login` at seq 7 (the process died) is **not** flagged as
  soon as any later `auth.login` appears at seq 8 — which the very next login produces.
- Interleaved requests: `seq1 attempt A`, `seq2 attempt A`, `seq3 success A`. One of the two
  attempts is orphaned; neither is flagged, because each has a later record with action `A`.

An orphaned intent is described by both the package doc and `docs/api-contract.md` as "itself a
finding", and this is the only place the UI surfaces it. Silently not showing findings is worse
than not having the feature.

**Fix:** Pair on the immediate successor, which is how the writer emits them — `Outcome`
appends the outcome record with the next sequence number and repeats the attempt's identifying
fields (`audit.go:226-237`):

```js
function isOrphanedIntent(record: AuditRecord, all: AuditRecord[]): boolean {
  if (record.outcome !== 'attempt') return false
  const successor = all.find((other) => other.seq === record.seq + 1)
  // Not loaded yet is not the same as absent: only claim an orphan when the
  // record that would disprove it is in hand.
  if (successor === undefined) return false
  return successor.action !== record.action || successor.outcome === 'attempt'
}
```

---

### WR-16: The audit action filter offers three tokens that can never appear in the log

**File:** `web/src/routes/audit.tsx:48-57`

**Issue:** `ACTION_TOKENS` lists `auth.me`, `audit.list` and `system.status`. All three are
`GET` routes, and `middleware.Audit` returns `next` unwrapped when `!mutating`
(`middleware/audit.go:71`). No record with any of those actions is ever written. Selecting one
always yields the empty state — "No audit records match these filters. The log itself is
untouched" — which reads as a fact about the operator's activity rather than as a fact about
the filter.

**Fix:** Drop the three read-only tokens from the list, and add a comment tying the list to the
mutating routes so it does not drift back:

```js
/**
 * The mutating action tokens `docs/api-contract.md` § Routes defines. Read-only
 * routes are deliberately absent: the audit middleware skips non-mutating
 * requests, so filtering by one can only ever return nothing.
 */
const ACTION_TOKENS = ['setup.create', 'auth.login', 'auth.logout', 'auth.sudo', 'account.password'] as const
```

---

### WR-17: `shadcn` (a codegen CLI) is declared as a runtime dependency

**File:** `web/package.json:22`

**Issue:** `"shadcn": "^4.19.0"` sits under `dependencies`, not `devDependencies`. shadcn is a
build-time scaffolding CLI that copies components into the tree; nothing under `web/src`
imports it. As a production dependency it is installed by `npm ci` in both the Taskfile
(`Taskfile.yml` `build:web`) and the release pipeline (`.goreleaser.yaml` `before.hooks`), and
it drags a CLI's transitive tree into the dependency graph of a project that took the trouble
to write `internal/depguard_test.go` to keep the *Go* supply chain narrow. The
`^` range is also the only caret in the pinned-version half of the manifest's core deps.

**Fix:** Move it to `devDependencies` (and pin it, to match `@biomejs/biome`, `vite` and the
rest):

```json
  "devDependencies": {
    "shadcn": "4.19.0",
```

Re-run `npm install` to refresh `package-lock.json`. `tw-animate-css` deserves the same look:
it is a CSS package consumed by `index.css`, so it is legitimately a runtime dep, but it is
carrying a caret where its neighbours are pinned.

---

### WR-18: `--sudo-window 0` is silently reinterpreted as five minutes

**File:** `internal/config/config.go:163-175`, `internal/auth/sudo.go:44-47`

**Issue:** The option parser accepts any `time.Duration`, including `0` and negatives. The
consumer then silently substitutes the default:

```go
	if window <= 0 {
		window = DefaultSudoWindow
	}
```

The rationale for that substitution is sound *inside* `IsSudoOpen` (a zero window would make
every destructive route unreachable). It is not sound as configuration handling: an operator
who sets `HOLZKUBE_SUDO_WINDOW=0` intending "always re-ask" gets a five-minute window, and
`LogEffective` dutifully prints `option=sudo-window value=0s origin=environment`
(`config.go:304-312`) — the startup log D-03 exists to make misconfiguration visible actively
reports the value that is *not* in force. `--sudo-window 8760h` is likewise accepted without
comment.

`--session-lifetime` is safe by accident: `auth.New` rejects `lifetime <= 0` (`auth.go:49-51`),
so a bad value is a start failure rather than a silent substitution. `--sudo-window` deserves
the same treatment.

**Fix:** Validate in the option table, where the operator is still in a position to act:

```go
	apply: func(c *Config, raw string) error {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return errors.New("not a duration; durations need a unit, for example 5m")
		}
		if d <= 0 {
			return errors.New("must be positive; there is no value that disables the sudo gate")
		}
		if d > 24*time.Hour {
			return errors.New("must not exceed 24h; a window longer than a session is not a window")
		}
		c.SudoWindow = d
		return nil
	},
```

---

### WR-19: A failed daily compression is retried never, and reported once

**File:** `internal/audit/rotate.go:46-85`

**Issue:** `openDay` assigns `l.file` and `l.day` (lines 70-71) *before* calling
`CompressOlderThan` (line 80). If compression fails, `openDay` returns the error, `append`
fails, and the mutating request that happened to trigger the rotation gets a 500 — the
documented fail-closed stance. But the logger's state has already advanced. The next `append`
hits the early return at line 48 (`l.file != nil && l.day == day`) and never attempts
compression again. So:

- exactly one request fails, at an arbitrary time, for a reason the operator sees only as
  `internal.unexpected`;
- every request after it succeeds;
- the compression silently never happens for that day, and the next attempt is 24 hours later
  for a *different* file.

The comment at lines 74-79 argues the operator "has to see that at once, not discover it
months later next to the gap it caused". One 500 with a redacted body is not that.

**Fix:** Separate the two concerns. Rotation must not fail because housekeeping did:

```go
	if rotating {
		if err := CompressOlderThan(l.dir, keepPlain); err != nil {
			// Rotation succeeded; the archive is intact and appendable. A failed
			// compression is an operator problem, not a reason to refuse the
			// mutation that happened to cross midnight.
			slog.Error("audit: could not compress rotated day files",
				slog.String("dir", l.dir), slog.Any("error", err))
		}
	}
	return nil
```

and retry it on the next `Open`, where a failure *is* a legitimate start refusal.

---

### WR-20: `redactValue` matches a literal dotted key as though it were a nested path

**File:** `internal/audit/redact.go:81-104`

**Issue:** `permitted` is keyed by dotted path, and the top-level lookup is a direct map hit on
the raw JSON key:

```go
func redactValue(path string, permitted map[string]struct{}, v any) any {
	if _, ok := permitted[path]; ok {
```

The first `path` handed in is the literal key from the decoded body (`redact.go:70-71`). So an
allowlist entry `"user.name"`, meant to permit `{"user":{"name":...}}`, also permits a body
sending the flat key `{"user.name": "..."}`. Because JSON object keys may contain dots and
`readBody` decodes whatever arrives (`middleware/audit.go:144-150`), the two namespaces are
conflated.

Not exploitable today: the phase-1 allowlist contains only `"username"`, with no dots
(`redact.go:47-56`). It is a latent hole in the one mechanism the package describes as
"failing the other way" by construction, in an archive with no deletion path.

**Fix:** Distinguish the two lookups. Carry the path as `[]string` and join only for the
prefix test, or mark leaves explicitly:

```go
func redactValue(segments []string, permitted map[string]struct{}, v any) any {
	path := strings.Join(segments, ".")
	if _, ok := permitted[path]; ok && len(segments) == pathDepth(path, permitted) {
		...
	}
```

The `[]string` form is the honest one: it makes a key containing a dot a single segment that
can never match a two-segment allowlist entry.

---

### WR-21: The SPA fallback reads the user list on every request, with a detached context

**File:** `internal/httpapi/router.go:134-160`

**Issue:** `fallback` calls `d.setupRequired()` for every non-API request that is not an asset
path, and `setupRequired` does `d.Store.Users().List(context.Background())` — discarding
`r.Context()`, so client disconnection and any future request deadline cannot cancel the store
read. `listJSON` reads and JSON-decodes every file in `users/` (`fsstore.go:182-203`), on every
navigation, for the life of the process, to answer a question whose answer changes exactly once.

`SPAHandler` is also outside `wrapRoute`, so it is not covered by the CSRF link. That is
harmless today (it only ever serves `GET`-shaped content), but a `POST /` currently returns the
SPA shell with a 200 rather than a 405, which is a slightly odd thing for the API's own
`fallback` to do.

**Fix:** Thread the request context through, and cache the negative:

```go
func (d Deps) setupRequired(ctx context.Context) bool {
	users, err := d.Store.Users().List(ctx)
	if err != nil {
		return true
	}
	return len(users) == 0
}
```

`SPAHandler` takes `func(*http.Request) bool`. Once `len(users) > 0` the answer is permanent
for the process — an `atomic.Bool` latch removes the read entirely after first setup.

---

## Not reported (verified as deliberate or correct)

Checked and found sound, listed so a later reviewer does not re-litigate them: the
`heldResponse` hold-until-touch in `middleware/sudo.go` (correctly ordered against `scs`'s
commit-on-first-write); `EntityLocks` refcounting and deletion (`store/lock.go:122-148`);
`ShortSession` truncation applied inside `append` where no caller can forget
(`audit.go:150`); `IsCA: false` + `KeyUsageDigitalSignature` on the generated leaf
(`tlsx/selfsigned.go:119-126`); `measureHash` taking the fastest of three samples to defeat
cold-cache inflation (`argon.go:99-125`); `backoff` written as a loop to avoid shift overflow
(`ratelimit.go:151-166`); `safeKey`'s character allowlist and `writeAtomic`'s
chmod-before-write ordering; the absence of `nosurf`, the three-precondition CSRF design, the
allowlist-only redactor, the hand-rolled canonical serializer, and login not opening the sudo
window — all confirmed deliberate per the phase's design decisions.

The `writeCanonicalString` handling of `utf8.RuneError` was examined closely as a possible
hash-collision source and is correct: `encoding/json` substitutes one U+FFFD per invalid byte
on write, and the ranged decode substitutes one `RuneError` per invalid byte on hash, so the
write-time hash and the verify-time re-hash agree.

---

_Reviewed: 2026-08-28_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
