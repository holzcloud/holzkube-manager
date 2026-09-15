// Package httpapi is holzkube-manager's HTTP surface: the route table, the RFC 9457
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

	"github.com/holzcloud/holzkube-manager/internal/audit"
	"github.com/holzcloud/holzkube-manager/internal/auth"
	"github.com/holzcloud/holzkube-manager/internal/auth/oidc"
	"github.com/holzcloud/holzkube-manager/internal/httpapi/middleware"
	"github.com/holzcloud/holzkube-manager/internal/imagefactory"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/metrics"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/nodestream"
	"github.com/holzcloud/holzkube-manager/internal/provision"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/streamhub"
	"github.com/holzcloud/holzkube-manager/internal/support"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
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

	// MinRole is the least privileged account that may use this route.
	//
	// The zero value is the empty string, which model.UserRole.AtLeast refuses
	// for everybody -- so a session route that names no role is unreachable
	// rather than open. That is the same fail-closed shape as
	// internal/talos's retry allowlist and internal/audit's redaction
	// allowlist, and for the same reason: a route nobody classified is a route
	// nobody decided about, and the safe reading of an undecided permission is
	// "no".
	//
	// New refuses to register a session route with no MinRole, so the failure
	// is a panic at composition rather than a 403 an operator meets later. It
	// is ignored on a route that needs no session: there is no account to have
	// a role.
	MinRole model.UserRole

	// Action is the stable audit token for this route, e.g. "auth.login".
	// A mutating route without one would execute unlogged.
	Action string

	// Streaming declares that this route writes a response over time rather
	// than at once -- an SSE stream today, and whatever phase 6 hangs job
	// progress on.
	//
	// It is declarative for the same reason Destructive is: what it turns off
	// is the process-wide write deadline, and a route that quietly needed that
	// and did not say so would work in development and die after two minutes
	// in production. New refuses a route that is both Streaming and
	// Destructive, because the sudo gate holds a response back until the
	// window has been refreshed and a held response is not a stream.
	Streaming bool

	// ClusterScope names the cluster a mutating route acts on, so that the
	// per-cluster read-only lock can be enforced at the route rather than
	// inside every handler (D-22, INV-12).
	//
	// It is a function on the route rather than a pattern this file matches,
	// for the same reason Destructive is a flag: the route knows where its
	// cluster id comes from -- a path segment, a body field, the machine the
	// request names -- and nothing else should have to guess. A nil value
	// means the route is not cluster-scoped, which is every read route.
	ClusterScope func(*http.Request) (string, error)
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

	// Version is the release this binary was built from, as the composition
	// root knows it. Empty in a test harness that does not care; the version
	// route then serves an empty string rather than inventing one, because a
	// made-up version is worse than a missing one.
	Version string

	// Factory is the Talos Image Factory client the schematic routes speak
	// through. It is nil in a deployment that serves no schematic routes, and
	// those handlers answer 502 rather than panicking if it ever is.
	Factory *imagefactory.Client

	// TalosMode carries the process's transport-level operating decisions to
	// the handlers -- today only whether it was started with --dry-run.
	//
	// It is the mode value itself rather than a bare boolean because it is the
	// value a handler has to hand to talos.NewClusterClient in order to make a
	// node call at all: one field answers both "may this instance mutate" and
	// "what do I pass to the constructor", and there is no second copy of the
	// answer to disagree with the first.
	TalosMode talos.Mode

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

	// OIDC is the configured identity provider, or nil when this instance
	// authenticates with the local password only. The OIDC routes are not
	// registered at all when it is nil.
	OIDC *oidc.Provider

	// Inventory is the node and cluster inventory the read routes and the
	// adoption route speak through. It is nil in a deployment that serves
	// neither, and those handlers answer 502 rather than panicking if it is.
	Inventory *inventory.Service

	// Hub is the stream fan-out and NodeStreams owns the readers that feed it.
	// Both are nil in a deployment that serves no streams, and the stream
	// route answers 502 rather than panicking if they are.
	Hub         *streamhub.Hub
	NodeStreams *nodestream.Manager

	// Jobs is the engine long-running operations run on, and Confirmer issues
	// and checks the tokens that gate the destructive ones. Both are nil in a
	// deployment that offers no node actions, and those handlers answer 502
	// rather than panicking if they are.
	Jobs      *jobs.Engine
	Confirmer *jobs.Confirmer

	// Config is the machine-configuration domain: viewing, diffing and
	// applying. It is nil in a deployment that serves none of those, and those
	// handlers answer 502 rather than panicking if it is.
	Config *machineconfig.Service

	// Provision is the provisioning wizard's service: scanning, inspecting,
	// planning and the etcd bootstrap lease. It is nil in a deployment that
	// provisions nothing, and those handlers answer 502 rather than panicking
	// if it is.
	Provision *provision.Service

	// Upgrade is the rolling-upgrade and etcd-management service. It is nil in
	// a deployment that upgrades nothing, and those handlers answer 502 rather
	// than panicking if it is.
	Upgrade *upgrade.Service

	// Metrics renders the Prometheus exposition. It is nil in a deployment
	// with no inventory to report on, and /metrics then answers 503 -- which
	// is what a scraper reads as "this target is down", the correct verdict
	// for an instance that cannot say what its fleet looks like.
	Metrics *metrics.Exporter

	// Support collects support bundles. It is nil in a deployment that offers
	// none, and that handler answers 502 rather than panicking if it is.
	Support *support.Collector

	// ClusterLocked reports whether a cluster refuses mutation. It is nil in a
	// deployment with no inventory, and the lock link is then inert -- which
	// is correct, because there are no clusters to protect.
	ClusterLocked func(r *http.Request, cluster string) error

	// IsSSOOnly reports whether a Host header names an address on which the
	// local password is refused. It is nil when no host is SSO-only, which is
	// the default and means "the password is accepted everywhere this instance
	// answers".
	//
	// It is a function rather than a list so that this package does not have to
	// re-derive the normalisation the configuration already did; the same rule
	// decides membership in AllowedHosts and here (config.NormalizeHost).
	IsSSOOnly func(host string) bool

	Routes []Route
}

// SSOOnly reports whether the local password is refused for this request. A nil
// IsSSOOnly means no host is SSO-only.
func (d Deps) SSOOnly(r *http.Request) bool {
	return d.IsSSOOnly != nil && d.IsSSOOnly(r.Host)
}

// New builds the handler: the outer chain, the route table and the SPA fallback.
func New(d Deps) http.Handler {
	mux := http.NewServeMux()

	// A second, method-less mux is the cheapest way to tell "no such path" from
	// "wrong method for a path that exists" once a catch-all is registered.
	known := http.NewServeMux()
	seen := make(map[string]bool, len(d.Routes))

	for _, rt := range d.Routes {
		// A composition-time panic, not a returned error. This runs once, at
		// startup, from the one place that assembles the table; a route table
		// that contradicts itself is a programming error and the right moment
		// to find it is before the listener opens.
		if rt.Streaming && rt.Destructive {
			panic("httpapi: route " + rt.Method + " " + rt.Pattern +
				" is both Streaming and Destructive. The sudo gate buffers a response until the " +
				"window has been refreshed, so a held response cannot be a stream.")
		}

		// The same composition-time refusal, for the same reason. A route that
		// needs a session and names no role is unreachable by every account,
		// which in production is a route that looks broken; finding it here is
		// finding it before the listener opens.
		if rt.RequiresSession && !rt.MinRole.Valid() {
			panic("httpapi: route " + rt.Method + " " + rt.Pattern +
				" requires a session and names no MinRole. Every session route says which " +
				"accounts may use it; the zero value is refused rather than defaulted, because " +
				"a permission nobody chose is not a permission anybody reviewed.")
		}
		if !rt.RequiresSession && rt.MinRole != "" {
			panic("httpapi: route " + rt.Method + " " + rt.Pattern +
				" names a MinRole and needs no session. There is no account to have a role, so " +
				"the marking would read as a restriction that is not enforced.")
		}

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
		// Beside the session loader rather than inside it: a token and a
		// cookie are alternatives, and a request carrying a token gets no
		// session at all.
		middleware.BearerToken(func(r *http.Request) (*http.Request, bool) {
			u, err := d.Auth.AuthenticateToken(r.Context(), middleware.BearerOf(r))
			if err != nil {
				// Not an error here. An unauthenticated request meets the
				// route's own gate, which has one sentence for "you are not
				// signed in" -- better than two that differ by which
				// credential was tried.
				return r, false
			}
			return r.WithContext(auth.WithTokenActor(r.Context(), u)), true
		}),
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
		// Exempt for a bearer token, and for one reason: CSRF is an attack on
		// ambient credentials, and a token is not ambient. See CSRF's own doc.
		middleware.CSRF(middleware.IsTokenRequest,
			func(w http.ResponseWriter, r *http.Request, err error) {
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
		// Inside audit rather than outside it, and that is the opposite
		// placement from authn. The archive exists for exactly this event --
		// somebody holding a valid session reached for something their account
		// may not do -- and it is the same argument that keeps the sudo
		// refusal inside. What kept the 401 out was that anyone can provoke
		// one; a 403 here needs a session first.
		middleware.Authz(rt.RequiresSession,
			func(r *http.Request) bool {
				u, ok := d.Auth.CurrentUser(r.Context())
				return ok && u.Role.AtLeast(rt.MinRole)
			},
			func(w http.ResponseWriter, r *http.Request) {
				WriteProblem(w, r, Forbidden(CodeForbiddenRole,
					"This account does not have the role this action needs. It needs at least "+
						string(rt.MinRole)+"."))
			}),
		// The lock sits between the archive and the password prompt. Inside
		// audit, because an attempt to change a cluster somebody adopted
		// read-only is precisely what the archive is for. Outside sudo,
		// because asking for a password and then refusing anyway teaches an
		// operator that the prompt means nothing.
		middleware.ClusterLock(rt.ClusterScope, d.ClusterLocked,
			func(w http.ResponseWriter, r *http.Request, err error) {
				// A cluster that does not exist is not a locked cluster.
				// Answering "locked read-only" to a request naming a cluster
				// nobody has adopted sends the operator to find an unlock
				// button for something that is not there -- and hides the
				// actual mistake, which is a wrong or stale cluster id.
				if errors.Is(err, inventory.ErrNotFound) {
					WriteProblem(w, r, NotFound("notfound.cluster",
						"No such cluster. The request names a cluster this installation does "+
							"not have a record of."))
					return
				}
				WriteProblem(w, r, ClusterLocked(err.Error()))
			}),
		// A bearer token satisfies the sudo gate, and that is a security
		// argument rather than a convenience.
		//
		// The window exists against a *stolen cookie*: somebody who has the
		// session and not the password. Re-asking for the password is what
		// separates them. A bearer token has no such gap -- it is not ambient,
		// it is put on each request deliberately by whatever holds it, and
		// there is no second secret to ask for. Demanding one anyway would
		// mean either giving every service account a password (a second way
		// in, weaker than the first) or making destructive routes unreachable
		// by automation, which is most of what automation is for.
		//
		// What replaces the window is the token itself: it is per account, it
		// is rotatable, and every use of it is in the audit archive under that
		// account's name.
		middleware.Sudo(rt.Destructive,
			func(r *http.Request) bool {
				if middleware.IsTokenRequest(r) {
					return true
				}
				return d.Auth.IsSudoOpen(r.Context(), d.SudoWindow)
			},
			func(r *http.Request) {
				// Nothing to touch on a token request: there is no session to
				// stamp, and writing one would be the request creating the
				// session it deliberately does not have.
				if middleware.IsTokenRequest(r) {
					return
				}
				d.Auth.TouchSudoWindow(r.Context())
			},
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
