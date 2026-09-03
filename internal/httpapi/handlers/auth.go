package handlers

import (
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/auth"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/httpapi/middleware"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// maxInlineDelay is the longest wait this package will serve by holding the
// connection open. Beyond it the caller is told to come back instead, because a
// connection parked for half a minute is itself a resource an attacker can
// spend cheaply and we cannot.
const maxInlineDelay = 500 * time.Millisecond

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sudoRequest struct {
	Password string `json:"password"`
}

type meResponse struct {
	ID       model.UserID `json:"id"`
	Username string       `json:"username"`

	// DryRun is the process's transport mode, not a UI preference. It is on
	// this endpoint rather than on GET /api/v1/system/status because that one
	// answers before authentication and Phase 1 declined to extend it for that
	// reason; the operator asking here is signed in by definition.
	DryRun bool `json:"dry_run"`

	// SSO says this session was established through the identity provider.
	//
	// The shell needs it for two decisions that are wrong the other way round:
	// signing out has to go through the provider's RP-initiated logout, or the
	// next sign-in returns instantly from a provider session that never ended;
	// and re-authentication for a destructive action has to be the provider
	// round-trip rather than a password prompt this session has no password for.
	SSO bool `json:"sso"`
}

// AuthRoutes serves login, logout, re-authentication and the identity probe.
//
// The limiter is created here, once, and shared by the two routes that check a
// password. Both are the same oracle, so a guesser must not be able to switch
// endpoints to get a fresh counter.
func AuthRoutes(d httpapi.Deps) []httpapi.Route {
	limiter := auth.NewLimiter()

	return []httpapi.Route{
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/auth/login",
			RequiresSession: false,
			Action:          "auth.login",
			Handler:         handler(func(w http.ResponseWriter, r *http.Request) { login(d, limiter, w, r) }),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/auth/logout",
			RequiresSession: true,
			Action:          "auth.logout",
			Handler:         handler(func(w http.ResponseWriter, r *http.Request) { logout(d, w, r) }),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/auth/me",
			RequiresSession: true,
			Action:          "auth.me",
			Handler:         handler(func(w http.ResponseWriter, r *http.Request) { me(d, w, r) }),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/auth/sudo",

			// Not destructive, deliberately. Gating the re-authentication
			// behind an open window would be a lock whose key is inside it.
			Destructive: false,

			RequiresSession: true,
			Action:          "auth.sudo",
			Handler:         handler(func(w http.ResponseWriter, r *http.Request) { openSudo(d, limiter, w, r) }),
		},
	}
}

// throttle applies the accumulated delay for this source before a password is
// checked, and reports whether it has already answered the request.
//
// The source is the peer address and nothing else (T-01-29). A forwarded header
// would be attacker-controlled, and honouring one would let a guesser reset
// their own counter on every attempt -- which is the same as having no limit
// while looking like having one.
func throttle(limiter *auth.Limiter, w http.ResponseWriter, r *http.Request) bool {
	wait := limiter.Delay(middleware.ClientIP(r))
	switch {
	case wait <= 0:
		return false

	case wait > maxInlineDelay:
		httpapi.WriteProblem(w, r, httpapi.RateLimited(int(math.Ceil(wait.Seconds()))))
		return true

	default:
		select {
		case <-time.After(wait):
			return false
		case <-r.Context().Done():
			return true
		}
	}
}

// refusePasswordOnSSOOnlyHost answers the two password routes when they are
// reached on a host the operator declared SSO-only, and reports whether it did.
//
// The refusal is before the body is read and before the rate limiter is
// consulted, so that a guesser on the public name cannot spend another
// account's limiter budget or learn anything from timing. It is deliberately
// not a 404: the route exists, and pretending otherwise would leave an operator
// who mistyped a hostname debugging a missing endpoint instead of reading that
// this address wants the identity provider.
func refusePasswordOnSSOOnlyHost(d httpapi.Deps, w http.ResponseWriter, r *http.Request) bool {
	if !d.SSOOnly(r) {
		return false
	}
	httpapi.WriteProblem(w, r, httpapi.Forbidden("auth.sso-only",
		"This address accepts the identity provider only. The local account works on the local network."))
	return true
}

func login(d httpapi.Deps, limiter *auth.Limiter, w http.ResponseWriter, r *http.Request) {
	if refusePasswordOnSSOOnlyHost(d, w, r) {
		return
	}

	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		// Even a malformed login body answers with the authentication problem
		// rather than a validation one: a client that can tell "bad shape" from
		// "bad credentials" can probe the account space by shape alone. It is
		// not counted as a failure, because it cannot guess anything.
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
		return
	}

	if throttle(limiter, w, r) {
		return
	}
	ip := middleware.ClientIP(r)

	_, err := d.Auth.Login(r.Context(), req.Username, req.Password)
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		limiter.Fail(ip)
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
		return
	case err != nil:
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}

	limiter.Succeed(ip)
	w.WriteHeader(http.StatusNoContent)
}

func logout(d httpapi.Deps, w http.ResponseWriter, r *http.Request) {
	if err := d.Auth.Logout(r.Context()); err != nil {
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// openSudo re-authenticates an already authenticated session, opening the
// window that destructive routes require (D-05).
//
// It answers 401 rather than 428 for a wrong password: the caller is logged in,
// so the failure is about the credential, not about the window.
func openSudo(d httpapi.Deps, limiter *auth.Limiter, w http.ResponseWriter, r *http.Request) {
	// The same rule as login, and for a stronger reason: this route is what
	// stands between a session cookie and a destructive action. Leaving the
	// password path open here would mean the public address refuses the
	// password to sign in and then accepts it to authorise wiping a node.
	if refusePasswordOnSSOOnlyHost(d, w, r) {
		return
	}

	var req sudoRequest
	if err := decodeJSON(w, r, &req); err != nil {
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
		return
	}

	u, ok := d.Auth.CurrentUser(r.Context())
	if !ok {
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
		return
	}

	if throttle(limiter, w, r) {
		return
	}
	ip := middleware.ClientIP(r)

	valid, err := auth.Verify(req.Password, u.PasswordHash)
	if err != nil {
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}
	if !valid {
		limiter.Fail(ip)
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
		return
	}
	limiter.Succeed(ip)

	d.Auth.OpenSudoWindow(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func me(d httpapi.Deps, w http.ResponseWriter, r *http.Request) {
	u, ok := d.Auth.CurrentUser(r.Context())
	if !ok {
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
		return
	}
	writeJSON(w, http.StatusOK, meResponse{
		ID:       u.ID,
		Username: u.Username,
		DryRun:   d.TalosMode.DryRun,
		SSO:      d.Auth.Sessions().GetBool(r.Context(), sessionKeyIsSSOAuth),
	})
}
