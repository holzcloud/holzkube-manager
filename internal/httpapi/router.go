// Package httpapi is holzkube's HTTP surface: the route table, the RFC 9457
// error taxonomy, the middleware wiring and the embedded UI.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/holzcloud/holzkube/internal/audit"
	"github.com/holzcloud/holzkube/internal/auth"
	"github.com/holzcloud/holzkube/internal/httpapi/middleware"
	"github.com/holzcloud/holzkube/internal/store"
)

// Route is one entry in the route table.
//
// Destructive is the declarative marking from D-06 and is binding for every
// later phase. It is the single place where "this can break something" is
// readable, which is what turns a mutating route that forgot the flag into a
// visible oversight in review rather than a silent hole in production.
type Route struct {
	Method  string
	Pattern string
	Handler http.Handler

	// Destructive bool is the declarative marking from D-06: it says this route
	// can destroy something. The middleware reads the flag; nothing pattern
	// matches on the URL. Keeping it here makes a new destructive route that
	// forgot the flag a visible oversight in review rather than a silent hole.
	Destructive bool

	// RequiresSession bool declares that the route needs a live session.
	RequiresSession bool

	// Action is the stable audit token for this route, e.g. "auth.login".
	// A mutating route without one would execute unlogged.
	Action string
}

// ChainStatus is the audit hash-chain verdict as the UI consumes it.
//
// File may hold a full path internally; call Public before serialising it.
type ChainStatus struct {
	OK           bool   `json:"ok"`
	BrokenAtLine int    `json:"broken_at_line"`
	File         string `json:"file"`
}

// Public returns the verdict as it may be sent to a client: File carries a
// file name, never a path.
//
// The audit directory lives under the XDG-resolved absolute data directory, so
// a path here names the OS user and their home directory layout. The one
// endpoint that serves this verdict answers before authentication, which makes
// that free reconnaissance about the host the threat model calls equivalent to
// root on every managed node. It is the same class of string Internal(err)
// discards for the same reason.
//
// filepath.Base("") is ".", which would be a worse answer than the empty
// string the caller meant, so an unset File stays unset.
func (c ChainStatus) Public() ChainStatus {
	if c.File != "" {
		c.File = filepath.Base(c.File)
	}
	return c
}

// Deps is everything the HTTP layer needs.
//
// Routes is assembled by the composition root from the handler packages. It
// lives here rather than being built inside New because the handler packages
// import this one for the problem taxonomy and the Route type; having New reach
// back into them would be an import cycle. Wave-2 plans therefore add routes in
// their own handler file and register them at the composition root, and this
// file stays untouched.
type Deps struct {
	Store      store.Store
	Audit      *audit.Logger
	Auth       *auth.Service
	Logger     *slog.Logger
	SudoWindow time.Duration

	// AuditChain is the startup verification verdict (D-15). It is a snapshot
	// on purpose: a break found at startup must stay visible in the UI, not
	// disappear because a later re-check happened to look at a different file.
	AuditChain ChainStatus

	// AllowedHosts is every host this instance answers to, and is what closes
	// DNS rebinding (see middleware.AllowHosts). The composition root fills it
	// from the bind address plus the loopback names; leaving it empty disables
	// the check and is intended only for tests, which cannot know the port
	// httptest will pick.
	AllowedHosts []string

	Routes []Route
}

// New builds the handler: the outer chain, the route table and the SPA fallback.
func New(d Deps) http.Handler {
	mux := http.NewServeMux()

	// A second, method-less mux is the cheapest way to tell "no such path" from
	// "wrong method for a path that exists" once a catch-all is registered.
	known := http.NewServeMux()
	seen := make(map[string]bool, len(d.Routes))

	for _, rt := range d.Routes {
		handler := d.wrapRoute(rt)
		mux.Handle(rt.Method+" "+rt.Pattern, handler)

		if !seen[rt.Pattern] {
			seen[rt.Pattern] = true
			known.Handle(rt.Pattern, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		}
	}

	mux.Handle("/", d.fallback(known))

	outer := middleware.Chain(
		// Outermost, so it covers the file server and the problem responses
		// the links below produce as well as the handlers.
		middleware.SecurityHeaders(ContentSecurityPolicy()),
		middleware.AllowHosts(d.AllowedHosts, func(w http.ResponseWriter, r *http.Request, err error) {
			WriteProblem(w, r, Forbidden("forbidden.host", err.Error()))
		}),
		middleware.Recover(d.Logger, func(w http.ResponseWriter, r *http.Request, err error) {
			WriteInternal(w, r, d.Logger, err)
		}),
		middleware.RequestID(),
		middleware.Log(d.Logger),
		middleware.Session(d.Auth.Sessions()),
	)

	return outer(mux)
}

// wrapRoute applies the per-route half of the chain: csrf -> authn -> audit ->
// sudo, outermost first.
//
// Audit sits inside authn but outside sudo, and the position is load-bearing in
// both directions.
//
// Outside sudo, because a 428 sudo.required is the highest-signal event in the
// phase-1 threat model: somebody holding a session cookie tried a destructive
// action and could not produce the password (T-01-25). With audit innermost
// that refusal short-circuited before the audit link ran and was recorded
// nowhere, in the log the product exists to keep.
//
// Inside authn, because the audit archive is append-only and D-16 keeps it
// forever with no deletion path. Moving this link outside the gates entirely
// would record the 401 and the 403 too, but it would also hand an
// unauthenticated caller a way to append to that archive on every mutating
// route, with no CSRF check and no rate limit in front of it. A denial that
// only an authenticated caller can provoke is worth recording; one that anyone
// can provoke is a disk-exhaustion lever. So csrf.precondition-unmet and
// auth.unauthenticated remain unrecorded here by choice, not by accident.
func (d Deps) wrapRoute(rt Route) http.Handler {
	inner := middleware.Chain(
		middleware.CSRF(func(w http.ResponseWriter, r *http.Request, err error) {
			WriteProblem(w, r, CSRFFailed(err.Error()))
		}),
		middleware.Authn(rt.RequiresSession,
			func(r *http.Request) bool { return d.Auth.IsAuthenticated(r.Context()) },
			func(w http.ResponseWriter, r *http.Request) {
				WriteProblem(w, r, Unauthenticated())
			}),
		middleware.Audit(auditAdapter{deps: d}, rt.Action, middleware.IsMutating(rt.Method),
			func(w http.ResponseWriter, r *http.Request, err error) {
				WriteInternal(w, r, d.Logger, err)
			}),
		middleware.Sudo(rt.Destructive,
			func(r *http.Request) bool { return d.Auth.IsSudoOpen(r.Context(), d.SudoWindow) },
			func(r *http.Request) { d.Auth.TouchSudoWindow(r.Context()) },
			func(w http.ResponseWriter, r *http.Request) {
				WriteProblem(w, r, SudoRequired())
			}),
	)
	return inner(rt.Handler)
}

// fallback serves the SPA for UI paths and a problem response for API paths, so
// an unknown /api path never returns HTML to a client expecting JSON.
func (d Deps) fallback(known *http.ServeMux) http.Handler {
	// One latch per handler, created here rather than as a Deps field: Deps is
	// copied by value throughout, and a latch that copies with it is not a
	// latch. fallback runs exactly once per New.
	var configured atomic.Bool
	spa := SPAHandler(func(r *http.Request) bool {
		return d.setupRequired(r.Context(), &configured)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAPIPath(r.URL.Path) {
			if _, pattern := known.Handler(r); pattern != "" {
				WriteProblem(w, r, MethodNotAllowed(
					"The path exists but does not accept "+r.Method+"."))
				return
			}
			WriteProblem(w, r, NotFound("notfound.route", "No such endpoint."))
			return
		}
		spa.ServeHTTP(w, r)
	})
}

// setupRequired reports whether the instance still has no operator account.
//
// ctx is the request's, so a client that disconnects cancels the store read;
// it used to be context.Background(), which made the read uncancellable by
// anything.
//
// configured latches the negative. listJSON reads and JSON-decodes every file
// in users/, and this ran on every non-asset navigation to answer a question
// whose answer changes exactly once. The latch is only ever set in the
// direction that is permanent: no route deletes a user, and phase 1's store
// exposes Users().Delete with no caller. A phase that adds one has to revisit
// this, because the redirect is D-01's server-side half and is supposed to hold
// even when the client-side check does not.
func (d Deps) setupRequired(ctx context.Context, configured *atomic.Bool) bool {
	if configured.Load() {
		return false
	}
	users, err := d.Store.Users().List(ctx)
	if err != nil {
		// Fail towards the setup wizard: an instance whose user list cannot be
		// read must not present itself as configured.
		return true
	}
	if len(users) == 0 {
		return true
	}
	configured.Store(true)
	return false
}

func isAPIPath(p string) bool {
	return p == "/api" || len(p) >= 5 && p[:5] == "/api/"
}

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
	// params arrives already redacted: the middleware runs every captured body
	// through the allowlist before it gets here, and there is no path that
	// reaches this call with raw input. The session token is shortened inside
	// the audit package, where no caller can forget to do it.
	return a.deps.Audit.Attempt(ctx, audit.Record{
		Actor:   actor,
		Session: a.deps.Auth.SessionID(ctx),
		SrcIP:   srcIP,
		Action:  action,
		Params:  params,
	})
}

func (a auditAdapter) Outcome(ctx context.Context, seq uint64, outcome string, cause error) error {
	if a.deps.Audit == nil {
		return errors.New("httpapi: audit log is not configured")
	}
	return a.deps.Audit.Outcome(ctx, seq, outcome, cause)
}
