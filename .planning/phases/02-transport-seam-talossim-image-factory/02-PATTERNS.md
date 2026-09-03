# Phase 2: Transport Seam, `talossim` & Image Factory - Pattern Map

**Mapped:** 2026-08-28
**Files analyzed:** 24 (13 new, 11 modified)
**Analogs found:** 20 / 24

Source of the file list: `02-CONTEXT.md` §§ `<decisions>` (D-01…D-10),
`<deadline_policy>`, `<code_context>`, `<specifics>`. No RESEARCH.md exists for
this phase; the research flag was closed inside CONTEXT.md.

---

## File Classification

### Track (a) — transport seam, pool, `talossim`, dry-run

| New/Modified File | Role | Data Flow | Closest Analog | Match |
|---|---|---|---|---|
| `internal/talos/talos.go` (new) — `Dialer`, `DiscoverySource` seams | interface seam | request-response | `internal/store/store.go` | exact |
| `internal/talos/client.go` (new) — `ClusterClient` / `MaintenanceClient` (D-06) | service | request-response | `internal/auth/auth.go` + `internal/store/store.go` | role-match |
| `internal/talos/deadline.go` (new) — deadline classes, no-deadline refusal (D-04) | utility | request-response | `internal/config/config.go` (`optionTable`) | partial |
| `internal/talos/retry.go` (new) — read-only allowlist, backoff + jitter | utility | request-response | `internal/auth/ratelimit.go` (`backoff`) | role-match |
| `internal/talos/breaker.go` (new) — per-node circuit breaker, fan-out | service | event-driven | `internal/auth/ratelimit.go` (`Limiter`) | role-match |
| `internal/talos/dryrun.go` (new) — mutation refusal at the wire (D-03) | middleware | request-response | `internal/tlsx/load.go` (`LoopbackGuard`) | role-match |
| `internal/talos/errors.go` (new) — transport error → problem code | utility | transform | `internal/httpapi/problem.go` | role-match |
| `internal/talossim/*.go` (new) — in-process fake Talos node | test fake / server | request-response + streaming | **none** (see *No Analog Found*); mTLS half → `internal/tlsx/selfsigned.go` | partial |
| `internal/depguard_test.go` (mod) — module-graph + pin guard (D-07) | test | batch | itself (`goList`, `offendingPackages`) | exact |
| `internal/config/config.go` (mod) — `--dry-run` option | config | transform | `insecure-http` entry, lines 155-167 | exact |
| `cmd/holzkubed/main.go` (mod) — transport construction + `Deps` wiring | config / composition | request-response | itself, lines 135-162 | exact |
| `internal/httpapi/router.go` (mod) — new `Deps` field | config | request-response | `Deps` struct, lines 78-99 | exact |
| `internal/httpapi/problem.go` (mod) — mint the transport problem code | utility | transform | `NotFound` / `Forbidden`, lines 102-125 | exact |
| `docs/api-contract.md` (mod) — taxonomy row + actor vocabulary (D-10) | doc | — | existing taxonomy table, lines 20-31 | exact |
| `web/src/components/DryRunBanner.tsx` (new) — visible dry-run mode (D-03) | component | request-response | `web/src/components/ChainBanner.tsx` | exact |

### Track (b) — Image Factory

| New/Modified File | Role | Data Flow | Closest Analog | Match |
|---|---|---|---|---|
| `internal/imagefactory/client.go` (new) — hand-rolled HTTP (D-01) | service | request-response | **none** (see *No Analog Found*) | — |
| `internal/imagefactory/schematic.go` (new) — copied `pkg/schematic` structs | model | transform | `internal/model/model.go` | role-match |
| `internal/imagefactory/urls.go` (new) — ISO/PXE/installer derivation (D-02) | utility | transform | `internal/tlsx/load.go` (`ListenHost`) | partial |
| `internal/imagefactory/schematicid.go` (new) — local ID precomputation | utility | transform | `internal/audit/record.go` (`CanonicalJSON`) | exact |
| `internal/imagefactory/brokenversions.go` (new) — curated list (D-08) | config data | batch | `internal/audit/redact.go` (`allowlist`) | exact |
| `internal/model/model.go` (mod) — `Schematic` record + `SchematicID` | model | CRUD | `model.User`, lines 23-31 | exact |
| `internal/store/store.go` (mod) — `Schematics()` accessor (D-09) | interface seam | CRUD | `UserStore`, lines 47-61 | exact |
| `internal/store/fsstore/fsstore.go` (mod) — `schematicStore` | model impl | CRUD | `userStore`, lines 218-300 | exact |
| `internal/store/migrate/migrate.go` (mod) — bump `CurrentVersion` to 2 | migration | batch | `migrate.go` lines 25, 51-62 | exact |
| `internal/httpapi/handlers/schematics.go` (new) — Routes + handlers | controller | request-response | `internal/httpapi/handlers/system.go` | exact |
| `internal/audit/redact.go` (mod) — allowlist entries for new actions | config data | transform | `allowlist`, lines 47-57 | exact |

---

## Pattern Assignments

### `internal/talos/talos.go` (interface seam, request-response)

**Analog:** `internal/store/store.go` — the project's canonical seam. Copy its
shape: a package doc that states the rule the seam enforces, sentinel errors as
package-level `var`, and entity/verb-shaped interfaces that never leak the
underlying implementation's vocabulary.

Package-doc pattern (`internal/store/store.go:1-8`):

```go
// Package store defines holzkube's persistence seam.
//
// The interface is deliberately entity-shaped and never path-shaped:
// store.Users().Get(ctx, id), never store.ReadFile("users/x.json"). No method
// here accepts or returns a filesystem path. That is what makes the eventual
// fsstore -> sqlitestore swap a single new implementation with zero changes
// above. Any os.ReadFile outside internal/store/fsstore is an architecture bug.
package store
```

The talos equivalent to write: *no package outside `internal/talos` may import
`.../machinery/client`*, which is the sentence the depguard extension (D-07)
then makes testable.

Sentinel errors (`internal/store/store.go:17-33`) — note each carries a
rationale comment saying whether it is retryable, which is exactly what the
retry allowlist needs:

```go
var (
	// ErrNotFound is returned when a record does not exist.
	ErrNotFound = errors.New("store: record not found")
	...
	// ErrAlreadyRunning is returned when the data directory is already held by
	// another process. It is a refusal to start, not a retryable condition:
	// two instances on one directory is a corruption path that no in-process
	// locking can detect.
	ErrAlreadyRunning = errors.New("store: data directory already in use by another process")
)
```

Root-interface + accessor pattern (`internal/store/store.go:47-53`) — the shape
`Dialer` / `DiscoverySource` should follow:

```go
// Store is the root of the persistence seam.
type Store interface {
	Users() UserStore
	Settings() SettingsStore
	Sessions() SessionStore
	Close() error
}
```

Note also the "no method that invites the wrong shape" argument at
`internal/store/store.go:63-66` — it is the same argument D-06 makes for two
non-overlapping client types:

```go
// SettingsStore holds the singleton settings record. It has no List or Delete
// because there is exactly one, always: a singleton with a List method is an
// invitation to write code that assumes otherwise.
```

---

### `internal/talos/client.go` (service, request-response) — D-06

**Analog:** `internal/auth/auth.go` for constructor shape and injected clock;
`internal/store/store.go` for the split-interface argument.

Constructor pattern (`internal/auth/auth.go:31-60`) — validate every dependency,
return `(*T, error)`, never a half-built value:

```go
// Service is the authentication service.
type Service struct {
	store    store.Store
	sm       *scs.SessionManager
	lifetime time.Duration

	// now is the clock. It exists so the absolute session limit and the sudo
	// window can be tested at the durations they actually run at rather than
	// at durations chosen to keep a test fast.
	now func() time.Time
}

func New(st store.Store, lifetime time.Duration) (*Service, error) {
	if st == nil {
		return nil, errors.New("auth: nil store")
	}
	if lifetime <= 0 {
		return nil, errors.New("auth: session lifetime must be positive")
	}
```

Copy the injected-clock field verbatim in spirit: deadline and breaker tests need
`now func() time.Time` for the same reason.

Package doc that names what the package must *not* know
(`internal/auth/auth.go:1-3`) — the `MaintenanceClient` doc should say the
mirror image:

```go
// Package auth owns password verification, the session lifecycle, the sudo-mode
// window and the login rate limit. It knows nothing about Talos and nothing
// about HTTP.
```

**D-05 note for the planner:** the `Version` liveness probe is the analog of
`auth.New`'s eager argon2id calibration (`internal/auth/auth.go:52-55`) — pay
and report the cost at construction rather than as an unexplained slow first
request.

---

### `internal/talos/deadline.go` (utility, request-response) — D-04

**Analog:** `internal/config/config.go`'s `optionTable` — the project's
established way of expressing a policy as a reviewable table of entries with a
per-entry rationale, rather than as scattered constants. Deadline classes should
be one such table keyed by RPC name, so an RPC with no class is a compile-time /
init-time gap, not a silent default.

The refusal-with-a-reason pattern to copy (`internal/config/config.go:180-186`):

```go
				if d <= 0 {
					return errors.New("must be positive; there is no value that disables the sudo gate")
				}
				if d > maxSudoWindow {
					return errors.New("must not exceed 24h; a window longer than a session is not a window")
				}
```

This is the exact wording register for "the wrapper refuses to issue a call
without a deadline; there is no default-less path".

---

### `internal/talos/retry.go` (utility, request-response)

**Analog:** `internal/auth/ratelimit.go:144-165`. Reuse the loop-not-shift
backoff and its overflow rationale verbatim; the phase's policy (200 ms → 800 ms,
2 retries, full jitter) is the same shape with different constants:

```go
// backoff is the delay after n consecutive failures: doubling from
// BaseLoginDelay, stopping at MaxLoginDelay.
//
// It is written as a loop rather than a shift so that a hundred failures is
// arithmetic that cannot overflow into a negative duration -- which would read
// as "no delay" and quietly undo the whole mechanism at exactly the point it
// matters most.
func backoff(failures int) time.Duration {
	if failures <= 0 {
		return 0
	}
	d := BaseLoginDelay
	for range failures - 1 {
		if d >= MaxLoginDelay {
			return MaxLoginDelay
		}
		d *= 2
	}
	if d > MaxLoginDelay {
		return MaxLoginDelay
	}
	return d
}
```

**Allowlist direction:** the retry allowlist must be authored the way
`internal/audit/redact.go:4-18` argues for allowlists — a *denylist of
non-idempotent RPCs* forgets the next mutating RPC added in Phase 8. Copy the
rationale comment structure from that file (see *Shared Patterns*).

---

### `internal/talos/breaker.go` (service, event-driven)

**Analog:** `internal/auth/ratelimit.go`'s `Limiter` — per-key state under one
mutex, with a sweep driven by the only method that adds entries. The per-node
breaker is the same data structure keyed by node address instead of source IP.

```go
// Delay reports how long this source must still wait before its next attempt is
// worth making. ...
func (l *Limiter) Delay(ip string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	s, ok := l.sources[ip]
	if !ok {
		return 0
	}
	remaining := s.nextAllowed.Sub(l.now())
	if remaining <= 0 {
		return 0
	}
	return remaining
}
```

Copy the no-goroutine sweep and its justification (`ratelimit.go:126-131`) — it
is directly load-bearing for a per-node map that would otherwise need a lifetime
and a shutdown path in every test:

```go
// sweep drops sources nobody has heard from in a while. It runs from Fail
// because Fail is the only thing that adds an entry, so the map cannot grow
// between sweeps. A background goroutine would do the same work while also
// needing a lifetime, a shutdown path and somewhere to be stopped in every test
// that builds a server.
```

Also copy `Len()` (`ratelimit.go:113-118`): "It exists so a test can assert the
map does not grow without bound." A per-node breaker needs the same assertion.

**Fan-out warning from CONTEXT:** audit fsyncs per record under one global
mutex. A fan-out that audits per node hits that mutex N times per operation —
the planner should treat contention as *mutation failure*, not latency.

---

### `internal/talos/dryrun.go` (middleware, request-response) — D-03

**Analog:** `internal/tlsx/load.go:83-106` — `LoopbackGuard` is the project's
existing example of a structural refusal placed at the one layer where the
property is actually true, with a comment saying why it is structural rather
than advisory. This is the closest existing thing to "enforced at the last layer
before the wire".

```go
// LoopbackGuard refuses plaintext HTTP anywhere it could leave the machine.
//
// The restriction is structural rather than advisory: --insecure-http exists for
// a developer proxying to a local server, and a session cookie that grants
// access to cluster PKI must not cross a home network in the clear (D-04,
// T-01-38). It runs before the server is set up, so the refusal is a start
// failure and not a request-time surprise.
func LoopbackGuard(listen string, insecure bool) error {
	if !insecure {
		return nil
	}
	loopback, err := config.IsLoopback(listen)
	if err != nil {
		return fmt.Errorf("tlsx: --insecure-http: %w", err)
	}
	if loopback {
		return nil
	}
	return fmt.Errorf(
		"tlsx: refusing --insecure-http while listening on %s: "+
			"the session cookie grants access to cluster PKI and would cross the network in plaintext; "+
			"plain HTTP is available only when the listener cannot leave this machine",
		listen)
}
```

Refusal-message shape to copy: *what was refused*, *why*, *what would have to
change*. The dry-run refusal error should read the same way.

Second analog for the "no other call site can reach the network" proof:
`internal/depguard_test.go` — a boundary asserted by a test that runs. See the
depguard section.

---

### `internal/talos/errors.go` + `internal/httpapi/problem.go` (mod)

**Analog:** `internal/httpapi/problem.go:102-125`. Mint the transport code with
the same three-part structure: a `Type…` constant in the closed taxonomy block,
a constructor, and a row in `docs/api-contract.md` in the same commit.

Taxonomy block (`internal/httpapi/problem.go:20-36`) — append to it, do not
restructure:

```go
// The taxonomy is closed and stable. These URIs and the code tokens below are
// a public contract: clients may match on them, so they never change.
const (
	TypeValidation           = "https://holzkube.dev/problems/validation"
	...
	TypeSetupRequired        = "https://holzkube.dev/problems/setup-required"
)
```

Constructor pattern (`internal/httpapi/problem.go:102-125`):

```go
// Forbidden reports a valid session that is not allowed to do this.
//
// It is distinct from Unauthenticated on purpose: "who are you" and "you may
// not" are different answers, and collapsing them makes a permissions bug look
// like a login bug.
func Forbidden(code, detail string) *Problem {
	return &Problem{
		Type:   TypeForbidden,
		Title:  "Not permitted",
		Status: http.StatusForbidden,
		Detail: detail,
		Code:   code,
	}
}
```

**Critical contrast** (`internal/httpapi/problem.go` `Internal`): `Internal(err)`
discards `err` entirely — `_ = err`, `Detail: ""`. That is why CONTEXT insists a
transport code be minted: an unreachable node falling through to
`internal.unexpected` carries *no detail at all* into the permanent archive.

`Problem` also implements `error` (`problem.go:62`: `func (p *Problem) Error()
string { return p.Code + ": " + p.Title }`), so a transport failure can travel
as an error value from `internal/talos` up to the handler unchanged.

Taxonomy table row format in `docs/api-contract.md:20-31`:

```
| `internal` | 500 | `internal.*` | unexpected failure; **only `instance`, never a detail** |
```

---

### `internal/talossim/*.go` (test fake / in-process server)

**No Go analog for the gRPC server half** — this is the first in-process server
in the repo. Two partial analogs apply:

**mTLS material — `internal/tlsx/selfsigned.go:108-140`.** `talossim` needs a CA
plus a node leaf; copy the key/serial/template construction:

```go
func generate(hostname string, extra ...string) (certPEM, keyPEM, der []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("tlsx: generate key: %w", err)
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	...
	names := dnsNames(hostname)
	ips := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
```

Note `tlsx` deliberately has **no CA** (`selfsigned.go:10-11`: "There is no
private certificate authority and nothing is installed into a system trust
store"). `talossim` needs one, so it must **not** be added to `tlsx` — it belongs
inside `internal/talossim`, or the tlsx doc comment becomes false.

TLS config baseline (`internal/tlsx/load.go:122-127`):

```go
func configFor(cert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
}
```

`talossim` additionally sets `ClientAuth: tls.RequireAndVerifyClientCert` and a
`ClientCAs` pool — that part has no analog.

**Scenario table — `internal/audit/redact.go`'s `allowlist`.** The nine TRANS-07
scenarios plus their expected client behaviour (CONTEXT `<specifics>` says the
behaviour table is owed by planning) should be one reviewable `map`/slice
literal with a per-entry comment, in the style of the allowlist below.

**Method-coverage test — `internal/depguard_test.go`** is the model for the
`<specifics>` requirement that a test enumerate the methods holzkube actually
calls and fail if any is unimplemented: an assertion that runs, not a note.

---

### `internal/depguard_test.go` (mod) — D-07

**Analog:** itself. Two additions, both reusing `goList`:

- module-graph guard: `goList(t, "-m", "all")` must not contain `talosRoot`
- version-pin assertion on the resolved machinery and cosi versions

`goList` already handles the offline case correctly and is directly reusable
(`internal/depguard_test.go:122-148`) — note it **skips loudly** rather than
failing, and the skip message states which command could not run:

```go
	cmd := exec.Command(goBin, append([]string{"list"}, args...)...) //nolint:gosec // args are constants in this file
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("skipping the dependency guard: `go list %s` failed (an incomplete module cache with no network will do this): %v\n%s",
			strings.Join(args, " "), err, out)
	}
```

The existing gap the new guard closes is stated at
`internal/depguard_test.go:36` — it walks only `-deps ./cmd/holzkubed`, which is
package-level.

Failure-message shape to copy (`depguard_test.go:38-43`) — it names the offender,
the rule, and where the offending code belongs instead:

```go
		t.Fatalf("cmd/holzkubed depends on %d package(s) of the Talos root module:\n  %s\n\n"+
			"Only %s may be imported by the product. Anything that needs the root module "+
			"belongs in the separate module under sandbox/ -- see sandbox/README.md.",
			len(offenders), strings.Join(offenders, "\n  "), talosMachinery)
```

Also extend the negative-control table `TestGuardRecognisesTheRootModule`
(`depguard_test.go:66-86`) — it already anticipates Phase 2's imports.

---

### `internal/config/config.go` (mod) — `--dry-run`

**Analog:** the `insecure-http` entry, `internal/config/config.go:155-167`. Copy
it exactly; a boolean option is a five-field literal in `optionTable`:

```go
		{
			name: "insecure-http", env: "INSECURE_HTTP", def: "false", boolean: true,
			usage: "serve plain HTTP; refused unless --listen is loopback (env " + EnvPrefix + "INSECURE_HTTP)",
			apply: func(c *Config, raw string) error {
				v, err := strconv.ParseBool(raw)
				if err != nil {
					return errors.New("not a boolean")
				}
				c.InsecureHTTP = v
				return nil
			},
			render: func(c Config) string { return strconv.FormatBool(c.InsecureHTTP) },
		},
```

Add the field to `Config` (`config.go:62-73`). `LogEffective` needs no change —
it iterates `optionTable` — but consider a `logger.Warn` for the dry-run case in
the style of the wildcard-bind warning (`config.go`, `LogEffective`):

```go
	if loopback, err := IsLoopback(c.Listen); err != nil || !loopback {
		logger.Warn("listening beyond loopback: holzkube is reachable from every device on this network",
			slog.String("listen", c.Listen))
	}
```

---

### `cmd/holzkubed/main.go` (mod) + `internal/httpapi/router.go` (mod)

**Analog:** `cmd/holzkubed/main.go:135-162` — the composition root is additive
exactly as CONTEXT says.

```go
	deps := httpapi.Deps{
		Store:      st,
		Audit:      auditLog,
		Auth:       authSvc,
		Logger:     logger,
		SudoWindow: cfg.SudoWindow,
		...
		AllowedHosts: allowedHosts(cfg.Listen),
	}

	// The route table is assembled here, from each handler package's own Routes
	// function. A wave-2 plan adds its routes in its handler file and adds one
	// line here; router.go stays untouched.
	deps.Routes = slices.Concat(
		handlers.SystemRoutes(deps),
		handlers.SetupRoutes(deps),
		handlers.AuthRoutes(deps),
		handlers.AccountRoutes(deps),
		handlers.AuditRoutes(deps),
	)
```

**Hazard, stated in CONTEXT and confirmed here:** `deps` is copied by value into
each `…Routes(deps)` call at line 156-162. The transport and image-factory
fields must be set **inside the `httpapi.Deps{…}` literal at line 135**, not
assigned after it — a later assignment is the zero value inside every handler
closure with no compile error.

Construction goes between line 133 (`authSvc`) and line 135, following the
`fsstore.Open` / `audit.Open` / `auth.New` shape (`main.go:100-133`): construct,
check error, `defer Close()`.

`Deps` field with a doc comment (`internal/httpapi/router.go:78-99`) — every
field there carries one; match that.

---

### `web/src/components/DryRunBanner.tsx` (component) — D-03 UI banner

**Analog:** `web/src/components/ChainBanner.tsx` — same job, same placement,
same "no dismiss control" argument.

```tsx
export function ChainBanner({ chain }: { chain: AuditChain }) {
  if (chain.ok) {
    return null
  }

  return (
    <div
      role="alert"
      aria-live="assertive"
      className="border-b border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive"
    >
```

Copy the pure-component + container split (`ChainBanner.tsx:43-57`), which is
what makes "it has no state that could remember having been seen" testable:

```tsx
export function ChainBannerContainer() {
  const status = useSystemStatus()

  if (!status.isSuccess) {
    return null
  }

  return <ChainBanner chain={status.data.audit_chain} />
}
```

The dry-run flag should ride on `GET /api/v1/system/status` — add a field to
`systemStatus` (`internal/httpapi/handlers/system.go:9-12`) and to `SystemStatus`
in `web/src/api.ts`; `useSystemStatus` (`web/src/hooks/useSession.ts:15-23`)
already polls it every 60 s and needs no change.

---

### `internal/imagefactory/client.go` (service, request-response) — D-01

**No analog.** There is no outbound HTTP client anywhere in the repo; `go.mod`
has three direct requires and none is an HTTP library. Nearest guidance:

- Error-wrapping convention with the package name as prefix, everywhere:
  `fmt.Errorf("tlsx: load key pair %s + %s: %w", certPath, keyPath, err)`
  (`internal/tlsx/load.go:114`).
- Explicit non-fallback rule (`internal/tlsx/load.go:19-23`) — the same rule
  applies to FACT-02's "factory returns 200 for a bogus extension" trap:

```go
// There are exactly three paths and one of them is deliberately missing: a
// supplied certificate that cannot be used is a hard failure, never a quiet fall
// back to self-generation.
```

- Body-size cap: `internal/httpapi/handlers/handlers.go:19-27` caps inbound
  bodies at `64 << 10` with `http.MaxBytesReader`. The factory client should cap
  the *response* body with `io.LimitReader` for the same stated reason
  ("Unbounded decoding of attacker-controlled input is a denial of service with
  no upside").
- Strict decoding: `dec.DisallowUnknownFields()` plus the trailing-content check
  (`handlers.go:26-38`) — the extension catalog response should be decoded the
  same way so an upstream schema change is loud.

The client needs an explicit `http.Client` with a `Timeout` and no ambient
default; the repo's precedent for naming and justifying every timeout is
`cmd/holzkubed/main.go:27-45`.

---

### `internal/imagefactory/schematicid.go` (utility, transform)

**Analog:** `internal/audit/record.go:66-100`. Local schematic-ID precomputation
is a canonical-serialisation-then-hash problem, which the audit chain already
solves and documents:

```go
// canonicalFields projects a Record onto the map that gets hashed. The hash
// field is excluded by construction -- it is the output, it cannot be an input.
```

```go
// CanonicalJSON renders the record, without its hash field, in canonical form:
// keys sorted lexicographically, no whitespace, UTF-8, no HTML escaping.
//
// This is deliberately not encoding/json's output. The chain must survive a Go
// upgrade, a change in escaping behaviour, and a reordered struct field --
```

The upstream schematic ID is a SHA-256 over the marshalled schematic, so the
same "do not trust `encoding/json`'s incidental behaviour" discipline applies —
and the phase must additionally verify the precomputed ID against the one the
factory returns from `POST /schematics`.

Determinism-of-marshalling precedent, `internal/store/fsstore/fsstore.go:147-159`:

```go
// marshalRecord serializes a record deterministically.
//
// Determinism is not cosmetic here. A repeated Put of an unchanged record must
// produce byte-identical output ...
```

---

### `internal/imagefactory/brokenversions.go` (config data, batch) — D-08

**Analog:** `internal/audit/redact.go:40-57`. A curated in-binary table with a
per-entry rationale, and a doc comment arguing the direction of the list:

```go
// allowlist maps an action token to the parameter paths that may appear in
// clear. Nested paths are dotted; every entry names a leaf, never a branch.
//
// Phase 1 permits exactly two things, both of them identifiers the operator
// chose for themselves and neither of them a credential. ...
var allowlist = map[string][]string{
	"setup.create": {"username"},
	"auth.login":   {"username"},

	// Listed with nothing permitted, so the table shows the full set of
	// phase-1 mutations rather than leaving them to the default.
	"auth.logout":      {},
	"auth.sudo":        {},
	"account.password": {},
}
```

Each broken-version entry needs a comment saying *why* it is listed — the D-08
rationale is "reviewable in git", which only holds if the reason is in the file.
Prerelease filtering is structural (semver prerelease component) and needs no
table entry at all.

---

### `internal/model/model.go` (mod) — `Schematic`

**Analog:** `model.User`, `internal/model/model.go:20-31`.

```go
// User is an operator account. holzkube is a single-operator tool, but the
// record is shaped so that a future OIDC or multi-user layer has somewhere to
// go without a migration.
type User struct {
	ID           UserID    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	CreatedAt    time.Time `json:"created_at"`

	// Rev is the compare-and-swap revision. Every stored record carries one.
	Rev uint64 `json:"rev"`
}
```

Rules this fixes: snake_case JSON tags, a `CreatedAt time.Time`, a trailing
`Rev uint64` on **every** stored record, and a distinct named ID type. The
named-ID rationale is at `internal/model/model.go:3-5`:

```go
// UserID, ClusterID and MachineID are distinct named types on purpose. Making
// them separate types costs nothing today and is the entire multi-cluster
// insurance policy: the compiler finds every place a scope was forgotten.
```

`ClusterID` (line 15) and `MachineID` (line 18) already exist with zero users —
a `Schematic` scoped to a cluster should use `ClusterID`, not a bare string.

---

### `internal/store/store.go` (mod) — `Schematics()` accessor (D-09)

**Analog:** `UserStore`, `internal/store/store.go:47-61`. Two edits, both
mechanical:

```go
// Store is the root of the persistence seam.
type Store interface {
	Users() UserStore
	Settings() SettingsStore
	Sessions() SessionStore
	Close() error
}

// UserStore holds operator accounts.
type UserStore interface {
	Get(ctx context.Context, id model.UserID) (model.User, error)
	List(ctx context.Context) ([]model.User, error)
	Put(ctx context.Context, rec model.User) (model.User, error)
	Delete(ctx context.Context, id model.UserID) error
}
```

Signature set is fixed: `Get/List/Put/Delete`, `ctx` first, `Put` returns the
stored record (with the bumped `Rev`), no method accepts a path.

---

### `internal/store/fsstore/fsstore.go` (mod) — `schematicStore`

**Analog:** `userStore`, `internal/store/fsstore/fsstore.go:218-300`. Copy
verbatim with the types swapped. The load-bearing parts:

Entity-kind constant + lock key (`fsstore.go:136-145`):

```go
// Entity kinds. They are the first half of every per-entity lock key, which is
// what keeps users/a and sessions/a from contending.
const (
	kindUsers    = "users"
	kindSettings = "settings"
	kindSessions = "sessions"
```

Path derivation always through `safeKey` (`fsstore.go:226-232`):

```go
func (u *userStore) path(id model.UserID) (string, error) {
	if err := safeKey(string(id)); err != nil {
		return "", err
	}
	return filepath.Join(u.dir, string(id)+".json"), nil
}
```

The CAS `Put` — this is the pattern that must be copied exactly, including the
"exists but rev 0" branch (`fsstore.go:258-289`):

```go
func (u *userStore) Put(_ context.Context, rec model.User) (model.User, error) {
	p, err := u.path(rec.ID)
	if err != nil {
		return model.User{}, err
	}

	defer u.locks.LockEntity(kindUsers, string(rec.ID))()

	var current model.User
	switch err := readJSON(p, &current); {
	case err == nil:
		if rec.Rev != current.Rev {
			return model.User{}, fmt.Errorf("%w: user %s is at rev %d, put carried %d",
				store.ErrConflict, rec.ID, current.Rev, rec.Rev)
		}
	case errors.Is(err, store.ErrNotFound):
		if rec.Rev != 0 {
			return model.User{}, fmt.Errorf("%w: user %s does not exist but put carried rev %d",
				store.ErrConflict, rec.ID, rec.Rev)
		}
	default:
		return model.User{}, err
	}

	rec.Rev++
	raw, err := marshalRecord(rec)
	if err != nil {
		return model.User{}, err
	}
	if err := writeAtomic(p, raw); err != nil {
		return model.User{}, err
	}
	return rec, nil
}
```

Wiring in `Open` (`fsstore.go:87-93`) — three lines, mirroring `users`:

```go
	s = &Store{dir: abs, release: release, entityMu: store.NewEntityLocks()}
	s.users = &userStore{dir: filepath.Join(abs, "users"), locks: s.entityMu}
	...
	for _, sub := range []string{s.users.dir, s.sessions.dir} {
		if err := os.MkdirAll(sub, dirPerm); err != nil {
			return nil, fmt.Errorf("fsstore: create %s: %w", sub, err)
		}
	}
```

**`safeKey` constraint** (`fsstore.go:162-180`): identifiers are limited to
`[A-Za-z0-9_-]`, ≤ 128 chars. A schematic ID is a 64-char lowercase hex SHA-256,
so it passes unchanged — but a caller-supplied schematic *name* would not.

---

### `internal/store/migrate/migrate.go` (mod) — schema bump

**Analog:** the file itself. `CurrentVersion` goes 1 → 2 (`migrate.go:25-26`) and
the empty list gains its first entry (`migrate.go:51-62`):

```go
// Migration advances a data directory by exactly one version.
type Migration struct {
	From  int
	To    int
	Apply func(dir string) error
}

// migrations is the ordered, forward-only list. Phase 1 defines exactly one
// schema version, so the list is empty ... The surrounding machinery is
// exercised by tests against fixtures anyway, so that phase 3 only appends an
// entry to a path that has already been proven.
var migrations = []Migration{}
```

The machinery is already proven: `run` takes `target` as a parameter precisely so
a 1 → 2 path is testable (`migrate.go:69-76`). Backup, ordered application and
the deferred `VERSION` write need no changes. Update the doc comment, which
currently says "Phase 1 defines exactly one schema version" and "phase 3 only
appends" — Phase 2 is now the first appender.

---

### `internal/httpapi/handlers/schematics.go` (controller, request-response)

**Analog:** `internal/httpapi/handlers/system.go` — the whole file is the
template. Package rule (`handlers/handlers.go:1-9`):

```go
// Handlers are thin by rule: decode, call a service, encode. A loop or a
// conditional about domain state belongs in a service, not here -- that is what
// keeps a future holzkubectl cheap.
//
// Each file exports its own Routes function and owns the URL shapes it serves.
// A wave-2 plan adds a route by adding it to its handler file and registering
// that file's Routes function at the composition root; router.go is not touched.
```

Routes-as-data (`handlers/system.go:16-27`):

```go
func SystemRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/system/status",
			RequiresSession: false,
			Action:          "system.status",
			Handler: handler(func(w http.ResponseWriter, r *http.Request) {
				users, err := d.Store.Users().List(r.Context())
				if err != nil {
					httpapi.WriteInternal(w, r, d.Logger, err)
					return
				}
```

Every mutating route needs `Action` (audit token) and a decision on
`Destructive` — the `Route` doc at `internal/httpapi/router.go:20-41` states the
D-06 rule. Note per D-03 that `Destructive` is **not** the dry-run mechanism;
schematic creation is mutating but not destructive.

Helpers to reuse from `handlers/handlers.go`: `decodeJSON` (lines 26-38),
`writeJSON` (lines 41-54), `handler` (line 57).

---

## Shared Patterns

### Audit actor vocabulary (D-10)

**Source:** `internal/httpapi/router.go:244-269`
**Apply to:** every non-HTTP mutation writer in this phase

```go
// auditAdapter fills in the actor and session from the request context, so the
// audit middleware needs no knowledge of the auth service.
type auditAdapter struct {
	deps Deps
}

func (a auditAdapter) Attempt(ctx context.Context, action, srcIP string, params map[string]any) (uint64, error) {
	if a.deps.Audit == nil {
		return 0, errors.New("httpapi: audit log is not configured")
	}
	actor := "anonymous"
	if u, ok := a.deps.Auth.CurrentUser(ctx); ok {
		actor = u.Username
	}
	return a.deps.Audit.Attempt(ctx, audit.Record{
		Actor:   actor,
		Session: a.deps.Auth.SessionID(ctx),
		SrcIP:   srcIP,
		Action:  action,
		Params:  params,
	})
}
```

This is the only place `Actor` is set in production code (verified: the only
other `Actor:` occurrences in `internal/` are tests). D-10 fixes `system` and
`job:<id>` as the two additional values and keeps the adapter here. The
`Record.Actor` field is part of the hash chain (`internal/audit/record.go:26-43`,
`canonicalFields` at lines 78-89), which is why D-10 calls the format one-way.

`model.ClusterID` / `model.MachineID` map onto `Record.ClusterID` /
`Record.MachineID` (`record.go:36-37`), both currently always empty — Phase 2's
node-scoped mutations are the first records that should fill them.

### Redaction allowlist — the guaranteed merge point

**Source:** `internal/audit/redact.go:40-57` (table) and `redact.go:58-63`
(default)
**Apply to:** every plan in both tracks that adds an action token

```go
// Params returns the parameters as they may be written to the log.
//
// An action with no entry in the table redacts everything. That is the default
// on purpose: forgetting to extend the allowlist costs a useful record, while
// the opposite default would cost a secret.
```

CONTEXT flags this as the one file both parallel tracks must edit. The failure is
silent and permanent (D-16, no deletion path). Every new action token —
`schematic.create`, `talos.apply-config`, `talos.reboot`, … — needs an entry,
even an empty one, following the existing "listed with nothing permitted" style.

### Error wrapping and package prefix

**Source:** `internal/tlsx/load.go`, `internal/store/fsstore/fsstore.go`,
`internal/store/store.go`
**Apply to:** all new packages

Every error string starts with the package name; every wrap uses `%w`; sentinels
are package-level `var` with a rationale comment. Callers branch with
`errors.Is`, never on strings:

```go
	return fmt.Errorf("tlsx: load key pair %s + %s: %w", certPath, keyPath, err)
```

```go
	switch err := readJSON(p, &current); {
	case err == nil:
	case errors.Is(err, store.ErrNotFound):
	default:
```

### Comment register

**Source:** every file read
**Apply to:** all new code

The codebase's dominant convention is that a doc comment states the *decision and
its counterfactual*, not the mechanism — "It takes no arguments on purpose"
(`problem.go`), "written as a loop rather than a shift so that …"
(`ratelimit.go`), "a guard added after the mistake has to be paid for with a
refactor rather than with a red test" (`depguard_test.go`). Every D-01…D-10
decision should appear as such a comment at the code that implements it. Plans
that produce mechanism-only comments will read as foreign.

### Structured logging

**Source:** `cmd/holzkubed/main.go:125-128`, `internal/config/config.go`
**Apply to:** transport and factory client

`slog` with typed attributes, never `fmt.Sprintf` into a message:

```go
		logger.Error("audit hash chain does not verify",
			slog.String("file", chainFile),
			slog.Int("broken_at_line", brokenLine))
```

---

## No Analog Found

| File | Role | Data Flow | Reason |
|---|---|---|---|
| `internal/imagefactory/client.go` | service | request-response | No outbound HTTP client exists anywhere in the repo. `go.mod` has three direct requires (`argon2id`, `scs/v2`, `x/sync`) and none is an HTTP library. Use the wrapping/limit/strict-decode conventions listed above and invent the rest. |
| `internal/talossim/server.go` (gRPC server half) | test fake / server | request-response + streaming | No gRPC server, no gRPC dependency, and no in-process test server of any kind exists. The single `httptest.NewServer` use in the repo (`internal/httpapi/security_test.go`) is a plain HTTP test server and carries nothing transferable. The mTLS half has a partial analog in `internal/tlsx`. |
| `internal/talossim/cosi.go` (in-memory COSI state) | test fake | CRUD | No COSI, no protobuf state server precedent. Follow the wiring named in CONTEXT `<research_flag_resolved>`: `state.WrapCore(namespaced.NewState(inmem.Build))` + `protobuf/server.NewState` + `cosiapi.RegisterStateServer`. |
| `internal/talos/*` streaming handling | service | streaming | No streaming code path exists; the `WriteTimeout = 60s` conflict named in `<deadline_policy>` confirms the HTTP chain has never carried one. None of the three `ResponseWriter` wrappers implements `Flush`/`Unwrap`/`Hijack`. Phase 5 problem; do not paper over it here. |

---

## Metadata

**Analog search scope:** `./cmd`, `./internal` (75 Go files), `./web/src`,
`./docs`, `go.mod`
**Files read in full or in targeted ranges:** `internal/store/store.go`,
`internal/store/fsstore/fsstore.go`, `internal/store/migrate/migrate.go`,
`internal/model/model.go`, `internal/depguard_test.go`,
`internal/httpapi/problem.go`, `internal/httpapi/router.go`,
`internal/httpapi/handlers/system.go`, `internal/httpapi/handlers/handlers.go`,
`internal/tlsx/load.go`, `internal/tlsx/selfsigned.go`,
`internal/config/config.go`, `internal/auth/auth.go`,
`internal/auth/ratelimit.go`, `internal/audit/redact.go`,
`internal/audit/record.go`, `cmd/holzkubed/main.go`,
`web/src/components/ChainBanner.tsx`, `web/src/hooks/useSession.ts`,
`docs/api-contract.md`
**Pattern extraction date:** 2026-08-28
