// Package httpapi is holzkube's HTTP surface: the route table, the RFC 9457
// error taxonomy, the middleware wiring and the embedded UI.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
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
type ChainStatus struct {
	OK           bool   `json:"ok"`
	BrokenAtLine int    `json:"broken_at_line"`
	File         string `json:"file"`
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
		middleware.Recover(d.Logger, func(w http.ResponseWriter, r *http.Request, err error) {
			WriteInternal(w, r, d.Logger, err)
		}),
		middleware.RequestID(),
		middleware.Log(d.Logger),
		middleware.Session(d.Auth.Sessions()),
	)

	return outer(mux)
}

// wrapRoute applies the per-route half of the chain: csrf -> authn -> sudo ->
// audit, outermost first.
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
		middleware.Sudo(rt.Destructive,
			func(r *http.Request) bool { return d.Auth.HasSudo(r.Context()) },
			func(w http.ResponseWriter, r *http.Request) {
				WriteProblem(w, r, SudoRequired())
			}),
		middleware.Audit(auditAdapter{deps: d}, rt.Action, middleware.IsMutating(rt.Method),
			func(w http.ResponseWriter, r *http.Request, err error) {
				WriteInternal(w, r, d.Logger, err)
			}),
	)
	return inner(rt.Handler)
}

// fallback serves the SPA for UI paths and a problem response for API paths, so
// an unknown /api path never returns HTML to a client expecting JSON.
func (d Deps) fallback(known *http.ServeMux) http.Handler {
	spa := SPAHandler(d.setupRequired)

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
func (d Deps) setupRequired() bool {
	users, err := d.Store.Users().List(context.Background())
	if err != nil {
		// Fail towards the setup wizard: an instance whose user list cannot be
		// read must not present itself as configured.
		return true
	}
	return len(users) == 0
}

func isAPIPath(p string) bool {
	return p == "/api" || len(p) >= 5 && p[:5] == "/api/"
}

// auditAdapter fills in the actor and session from the request context, so the
// audit middleware needs no knowledge of the auth service.
type auditAdapter struct {
	deps Deps
}

func (a auditAdapter) Attempt(ctx context.Context, action, srcIP string) (uint64, error) {
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
		// Input parameters are deliberately not captured here. Capturing them
		// before allowlist redaction exists would write the setup and login
		// passwords straight into an append-only log that is kept forever
		// (D-16). Plan 03 adds the redactor and the capture together.
		Params: map[string]any{},
	})
}

func (a auditAdapter) Outcome(ctx context.Context, seq uint64, outcome string, cause error) error {
	if a.deps.Audit == nil {
		return errors.New("httpapi: audit log is not configured")
	}
	return a.deps.Audit.Outcome(ctx, seq, outcome, cause)
}
