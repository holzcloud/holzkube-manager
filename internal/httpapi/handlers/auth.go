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

// newLimiter builds the password limiter AuthRoutes shares between its two
// routes. A variable so a test can give it a clock it moves by hand.
var newLimiter = auth.NewLimiter

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

	// Role is what this session may do. It decides what the interface offers
	// and it is not the enforcement -- every route decides for itself on the
	// way in.
	Role model.UserRole `json:"role"`

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
	limiter := newLimiter()

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
			MinRole:         model.RoleReader,
			Action:          "auth.logout",
			Handler:         handler(func(w http.ResponseWriter, r *http.Request) { logout(d, w, r) }),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/auth/me",
			RequiresSession: true,
			MinRole:         model.RoleReader,
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
			MinRole:         model.RoleReader,
			Action:          "auth.sudo",
			Handler:         handler(func(w http.ResponseWriter, r *http.Request) { openSudo(d, limiter, w, r) }),
		},
	}
}

// SudoFailuresBeforeSignOut is how many wrong passwords in a row the
// re-authentication takes for one account before it ends the session that sent
// the last of them.
//
// The window is what stands between a stolen session cookie and a destructive
// action, and the backoff alone lets a cookie thief keep guessing at thirty
// seconds a try for as long as the session lives. Ending the session costs the
// operator who mistyped ten times one sign-in; it strands nobody, because the
// sign-in is still there and its counter is per address, not per account.
const SudoFailuresBeforeSignOut = 10

// sudoKey is the limiter key that counts re-authentication attempts per
// account, beside the per-address one. Without it a thief holding a cookie
// could spread the guesses over addresses and meet a fresh counter on each.
func sudoKey(u model.User) string { return "sudo-user:" + string(u.ID) }

// throttle reserves one password attempt against keys, waiting out a short
// delay inline, and reports false when it has already answered the request.
//
// The reservation is taken before the password is checked and charged as a
// failure until the outcome says otherwise (see auth.Limiter.Reserve). Reading
// the delay first and recording the failure after the check let every attempt
// sent in parallel through: they all asked before the first one had failed.
//
// The address key is the peer address and nothing else (T-01-29). A forwarded
// header would be attacker-controlled, and honouring one would let a guesser
// reset their own counter on every attempt -- which is the same as having no
// limit while looking like having one.
func throttle(limiter *auth.Limiter, w http.ResponseWriter, r *http.Request, keys ...string) (*auth.Reservation, bool) {
	for {
		wait, res := limiter.Reserve(keys...)
		if res != nil {
			return res, true
		}
		if wait > maxInlineDelay {
			httpapi.WriteProblem(w, r, httpapi.RateLimited(int(math.Ceil(wait.Seconds()))))
			return nil, false
		}
		// Then ask again: another attempt may have taken the slot meanwhile,
		// and the answer to that is a longer wait, not a free pass.
		select {
		case <-time.After(wait):
		case <-r.Context().Done():
			return nil, false
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

	ip := middleware.ClientIP(r)
	res, ok := throttle(limiter, w, r, ip)
	if !ok {
		return
	}

	u, err := d.Auth.Login(r.Context(), req.Username, req.Password)
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		res.Fail()
		d.Logger.Warn("sign-in failed", "route", r.URL.Path, "peer", ip)
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
		return
	case err != nil:
		res.Cancel()
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}

	res.Succeed()
	// Knowing the password ends the account's re-authentication episode too:
	// a session ended for wrong sudo passwords would otherwise sign in again
	// straight into the wait it left behind.
	limiter.Succeed(sudoKey(u))
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

	// A service account has no password, and its token already satisfies the
	// window this route exists to open -- so there is nothing here for it to
	// ask for. Without this it reached Verify with an empty hash, which errors
	// rather than returning false, and the answer was a 500.
	//
	// Before the throttle rather than after. The throttle exists to slow a
	// password guesser, and this caller is not guessing: it has proven its
	// identity with a token and is being told a fact about its own account.
	// Spending an IP's budget on that would let one automation's misconfigured
	// call delay a person's sign-in from the same address.
	if u.IsService() {
		httpapi.WriteProblem(w, r, notAPerson(
			"A service account has no password to re-authenticate with, and does not need to: "+
				"its token already satisfies the re-authentication window."))
		return
	}

	// Counted per address, as the login is, and per account: the address
	// alone let somebody holding a stolen cookie spread the guesses.
	ip := middleware.ClientIP(r)
	res, ok := throttle(limiter, w, r, ip, sudoKey(u))
	if !ok {
		return
	}

	valid, err := auth.Verify(req.Password, u.PasswordHash)
	if err != nil {
		res.Cancel()
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}
	if !valid {
		res.Fail()
		d.Logger.Warn("sign-in failed", "route", r.URL.Path, "peer", ip)
		if limiter.Failures(sudoKey(u)) >= SudoFailuresBeforeSignOut {
			// The session that kept guessing is ended; see
			// SudoFailuresBeforeSignOut. The 401 is then true of the session
			// as well as of the password.
			d.Logger.Warn("session ended after repeated wrong re-authentication",
				"route", r.URL.Path, "peer", ip, "account", u.Username)
			if err := d.Auth.Logout(r.Context()); err != nil {
				d.Logger.Error("could not end the session", "error", err)
			}
		}
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
		return
	}
	res.Succeed()

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
		// The role rides here for the same reason dry_run does: it decides
		// what the interface offers, and an interface that has to ask a second
		// route what this session may do is an interface that renders a
		// screenful of buttons and then takes half of them away.
		//
		// It is not the enforcement. Every route decides for itself on the way
		// in, and a client that ignored this field would meet 403 rather than
		// get anywhere.
		Role:   u.Role.OrAdmin(),
		DryRun: d.TalosMode.DryRun,
		SSO:    d.Auth.SignedInThroughProvider(r.Context()),
	})
}
