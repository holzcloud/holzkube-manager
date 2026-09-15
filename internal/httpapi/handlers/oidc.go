package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/auth"
	"github.com/holzcloud/holzkube-manager/internal/auth/oidc"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// Session keys for the in-flight authorisation request. They live in the
// server-side session rather than in a cookie or a signed parameter: state,
// nonce and the PKCE verifier are only worth anything if the browser carrying
// them cannot read or choose them.
const (
	sessionKeyState    = "oidc.state"
	sessionKeyNonce    = "oidc.nonce"
	sessionKeyVerifier = "oidc.verifier"
	sessionKeyRedirect = "oidc.redirect"
	sessionKeySudoFlow = "oidc.sudo"
	// gosec G101 flags this because the name contains "token" and the value is
	// a literal. It is a session *key* -- the name under which the token is
	// stored -- and not the token itself, which only ever arrives from the
	// provider at runtime. Renaming it to something without the word would hide
	// the finding by making the constant worse.
	sessionKeyIDToken   = "oidc.id_token" //nolint:gosec // a session key name, not a credential
	sessionKeyIsSSOAuth = "oidc.authenticated"
)

// afterAuth is where the browser lands once the flow completes.
//
// It is a constant, and deliberately not a parameter the caller may supply. A
// return-to parameter on an authentication callback is the standard open
// redirect: the link that carries an operator through a real login and drops
// them on somebody else's page afterwards is indistinguishable, to them, from
// the product working.
const afterAuth = "/"

// OIDCRoutes serves the browser redirects of the authorization code flow.
//
// They are GET routes that answer with 302 rather than JSON, because every step
// is a navigation the browser performs. The CSRF link in the per-route chain
// covers mutating methods only, which is correct here: the callback's defence
// is the state parameter it compares against the session, and that is stronger
// than a token check because it also proves the flow started on this server.
func OIDCRoutes(d httpapi.Deps) []httpapi.Route {
	if d.OIDC == nil {
		// No provider configured: the routes do not exist rather than existing
		// and answering 404. A route table that mirrors the configuration is
		// one an operator can read to find out what this instance accepts.
		return nil
	}

	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/auth/oidc/start",
			RequiresSession: false,
			Action:          "auth.oidc.start",
			Handler:         handler(func(w http.ResponseWriter, r *http.Request) { oidcStart(d, w, r, false) }),
		},
		{
			Method:  http.MethodGet,
			Pattern: "/api/v1/auth/oidc/sudo",

			// Not destructive, for the same reason POST /auth/sudo is not:
			// gating re-authentication behind an open window would be a lock
			// whose key is inside it.
			Destructive:     false,
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Action:          "auth.oidc.sudo",
			Handler:         handler(func(w http.ResponseWriter, r *http.Request) { oidcStart(d, w, r, true) }),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/auth/oidc/callback",
			RequiresSession: false,
			Action:          "auth.oidc.callback",
			Handler:         handler(func(w http.ResponseWriter, r *http.Request) { oidcCallback(d, w, r) }),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/auth/oidc/logout",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Action:          "auth.oidc.logout",
			Handler:         handler(func(w http.ResponseWriter, r *http.Request) { oidcLogout(d, w, r) }),
		},
	}
}

// redirectURI is the callback address as the browser will reach it.
//
// It is derived from the request rather than configured because one instance
// answers on more than one name -- a LAN address and a public hostname -- and
// the provider requires the redirect_uri at the token endpoint to be identical
// to the one in the authorisation request. Deriving it is safe only because
// middleware.AllowHosts has already refused any Host this instance does not
// serve; without that check the caller would be naming the redirect target.
//
// The scheme comes from the connection, never from X-Forwarded-Proto: that
// header is set by whoever spoke to us, and here that would let a caller
// downgrade the redirect it is about to be sent to.
func redirectURI(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/api/v1/auth/oidc/callback"
}

func oidcStart(d httpapi.Deps, w http.ResponseWriter, r *http.Request, sudo bool) {
	if !sudo && refuseUnlinkedOnSSOOnlyHost(d, w, r) {
		return
	}

	flow, err := oidc.NewFlowState(redirectURI(r), sudo)
	if err != nil {
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}

	url, err := d.OIDC.AuthCodeURL(r.Context(), flow)
	if err != nil {
		writeProviderProblem(d, w, r, sudo, err)
		return
	}

	sm := d.Auth.Sessions()
	sm.Put(r.Context(), sessionKeyState, flow.State)
	sm.Put(r.Context(), sessionKeyNonce, flow.Nonce)
	sm.Put(r.Context(), sessionKeyVerifier, flow.Verifier)
	sm.Put(r.Context(), sessionKeyRedirect, flow.Redirect)
	sm.Put(r.Context(), sessionKeySudoFlow, flow.Sudo)

	// gosec G710 sees a redirect target that came out of a function and cannot
	// tell where it came from. This one is the provider's authorization
	// endpoint, taken from the discovery document of --oidc-issuer, with our own
	// parameters appended. The issuer is configuration, it is refused unless it
	// is https (see config.optionTable), and nothing on the request influences
	// it. The genuinely attacker-shaped redirect in this flow is the one back to
	// the application afterwards, and that is the constant afterAuth.
	http.Redirect(w, r, url, http.StatusFound) //nolint:gosec // target derives from the configured issuer, not from the request
}

// refuseUnlinkedOnSSOOnlyHost answers a sign-in that cannot possibly succeed,
// before the browser is sent anywhere, and reports whether it did.
//
// On an SSO-only host the first binding of an identity is refused (see
// bindFirstIdentity). Without this check the operator learns that only at the
// very end: the page offers the button, the provider authenticates them, and
// the callback then says the account is not linked. Every step of that
// round-trip was already doomed when the button was drawn.
//
// The check is here rather than in a field on GET /api/v1/system/status, which
// is what would let the page grey the button out. That endpoint answers before
// authentication, and "this instance has no identity linked yet" is a fact
// about its state that an anonymous caller on the public address has no reason
// to be told. Refusing at the start of a flow the caller deliberately began
// discloses the same thing to somebody who could have discovered it by
// finishing the flow anyway, and to nobody else.
//
// A missing binding is the only case that can be decided in advance. A binding
// to a *different* subject cannot: the subject arrives with the token, so that
// one still surfaces in the callback.
func refuseUnlinkedOnSSOOnlyHost(d httpapi.Deps, w http.ResponseWriter, r *http.Request) bool {
	if !d.SSOOnly(r) {
		return false
	}

	u, err := d.Auth.SingleAccount(r.Context())
	if err != nil {
		// No account yet. Setup is refused here too, so there is nothing this
		// address can offer until the wizard has run on the local network.
		failSignIn(w, r, "setup-required")
		return true
	}
	if u.HasIdentityBinding() {
		return false
	}

	failSignIn(w, r, "bind-host")
	return true
}

// failSignIn ends a failed sign-in the way a browser navigation has to end:
// back on the sign-in page, with a stable code the page can render.
//
// These four routes are navigations, not API calls. A problem document is the
// right answer to fetch() and the wrong one to a redirect chain the operator is
// riding through: it renders as raw JSON in the address bar, which is where
// this flow has actually left people standing. The code is the machine-readable
// half of the problem that was there before; the human half moves into the
// page, which can say it in the operator's own language and offer the next
// step. The detail stays in the server log, where it was already going.
//
// Only the sign-in flow redirects. A failure during sudo re-authentication
// keeps its problem document: the caller is already authenticated, /login is
// the wrong place to send them, and that path is diagnosed far more often than
// it is walked into.
func failSignIn(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, afterAuthFailure+"?sso_error="+url.QueryEscape(code), http.StatusFound)
}

// afterAuthFailure is the sign-in page. Like afterAuth it is a constant and not
// anything the caller may influence.
const afterAuthFailure = "/login"

// failSudo hands a refused re-authentication back to the application instead of
// leaving the operator on a problem document.
//
// The sign-in flow already does this, and the comment above failSignIn says why:
// a problem document reached by NAVIGATION renders as raw JSON in the address
// bar, "which is where this flow has actually left people standing". The sudo
// flow was deliberately excluded from that on the grounds that its caller is
// already authenticated and /login is the wrong destination -- which is true,
// and did not make the JSON any more readable. An operator met exactly that: a
// destructive action refused three times, each ending on a page of JSON they
// had no reason to read as an error.
//
// It goes to afterAuth rather than /login because the session is fine; only the
// window is shut. The code rides in the query string so the page can say which
// of the three refusals this was, each of which has a different remedy and a
// different person to carry it out.
func failSudo(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, afterAuth+"?sudo_error="+url.QueryEscape(code), http.StatusFound)
}

func oidcCallback(d httpapi.Deps, w http.ResponseWriter, r *http.Request) {
	sm := d.Auth.Sessions()
	flow := oidc.FlowState{
		State:    sm.GetString(r.Context(), sessionKeyState),
		Nonce:    sm.GetString(r.Context(), sessionKeyNonce),
		Verifier: sm.GetString(r.Context(), sessionKeyVerifier),
		Redirect: sm.GetString(r.Context(), sessionKeyRedirect),
		Sudo:     sm.GetBool(r.Context(), sessionKeySudoFlow),
	}
	// One flow per session, consumed whether or not it succeeds. Leaving the
	// state behind would let a captured callback URL be replayed.
	clearFlow(d, r)

	if flow.State == "" || flow.Verifier == "" {
		failSignIn(w, r, "no-flow")
		return
	}

	// The provider reports its own refusals here rather than at the token
	// endpoint: an operator who is not assigned to this application in
	// Authentik arrives with error=access_denied and no code at all.
	if e := r.URL.Query().Get("error"); e != "" {
		d.Logger.WarnContext(r.Context(), "the identity provider refused the sign-in", "error", e)
		failSignIn(w, r, "denied")
		return
	}
	if !flow.MatchesState(r.URL.Query().Get("state")) {
		failSignIn(w, r, "state-mismatch")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		failSignIn(w, r, "no-code")
		return
	}

	identity, err := d.OIDC.Exchange(r.Context(), flow, code)
	if err != nil {
		if errors.Is(err, oidc.ErrProviderUnreachable) {
			writeProviderProblem(d, w, r, flow.Sudo, err)
			return
		}
		// The detail stays generic. The error text names token contents and
		// provider internals, and this response is rendered in a browser.
		d.Logger.WarnContext(r.Context(), "oidc callback failed", "error", err)
		failSignIn(w, r, "exchange-failed")
		return
	}

	if flow.Sudo {
		completeSudo(d, w, r, identity)
		return
	}
	completeLogin(d, w, r, identity)
}

// completeLogin turns a verified identity into a session.
func completeLogin(d httpapi.Deps, w http.ResponseWriter, r *http.Request, identity oidc.Identity) {
	issuer := d.OIDC.Issuer()

	u, err := d.Auth.FindByIdentity(r.Context(), issuer, identity.Subject)
	switch {
	case err == nil:
		// Bound already: sign in.

	case errors.Is(err, auth.ErrNoIdentityBinding):
		bound, bindErr := bindFirstIdentity(d, r, issuer, identity)
		if bindErr != nil {
			writeBindProblem(d, w, r, bindErr)
			return
		}
		u = bound

	default:
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}

	if err := d.Auth.StartSession(r.Context(), u); err != nil {
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}
	sm := d.Auth.Sessions()
	sm.Put(r.Context(), sessionKeyIsSSOAuth, true)
	sm.Put(r.Context(), sessionKeyIDToken, identity.RawIDToken)

	http.Redirect(w, r, afterAuth, http.StatusFound)
}

// errBindFromUntrustedHost is returned when the first sign-in through the
// provider arrives on a host where the local password is not accepted.
var errBindFromUntrustedHost = errors.New("oidc: first binding must not happen on an SSO-only host")

// errBindBeforeSetup is returned when no operator account exists yet.
var errBindBeforeSetup = errors.New("oidc: setup has not created an account yet")

// errBindAmbiguous is returned when this instance has more than one account.
//
// Binding on first sign-in is trust on first use, and trust on first use needs
// somebody to trust. With one account there is no question about which account
// an arriving identity belongs to; with two there is no answer, and the
// plausible-looking guesses -- match on username, take the admin, take the
// first -- are each a way for a new subject at the provider to take over
// somebody else's account.
//
// So the binding becomes deliberate rather than automatic. What that costs is
// one step for an operator who has just added a second account; what it buys
// is that adding an account never silently changes who a provider identity
// signs in as.
var errBindAmbiguous = errors.New("oidc: this instance has more than one account, so a first " +
	"binding cannot be inferred")

// bindFirstIdentity links the provider identity to the one operator account.
//
// Two conditions, and both are about the same question: who is allowed to
// become the operator of this instance.
//
// The account must already exist, so that a break-glass credential is always
// there before the provider becomes a way in -- an instance whose only route to
// authentication is the identity provider it manages the cluster for can lock
// its operator out of the repair.
//
// And the binding may not be made from an SSO-only host, which in practice
// means it may not be made from the internet. Binding on first sign-in is trust
// on first use; requiring it to happen on the LAN puts that trust behind the
// same boundary the break-glass account already sits behind, rather than
// offering it to whoever reaches the public name first.
func bindFirstIdentity(d httpapi.Deps, r *http.Request, issuer string, identity oidc.Identity) (model.User, error) {
	if d.SSOOnly(r) {
		return model.User{}, errBindFromUntrustedHost
	}

	u, err := d.Auth.SingleAccount(r.Context())
	if err != nil {
		// Two different conditions arrive here as one error, and they are
		// worth telling apart: no account at all is "run setup", and several
		// accounts is "an admin has to link this one deliberately".
		users, listErr := d.Auth.Users(r.Context())
		if listErr == nil && len(users) > 1 {
			return model.User{}, errBindAmbiguous
		}
		return model.User{}, errBindBeforeSetup
	}

	bound, err := d.Auth.BindIdentity(r.Context(), u, issuer, identity.Subject)
	if err != nil {
		return model.User{}, err
	}
	d.Logger.InfoContext(r.Context(), "operator account bound to a provider identity",
		"username", bound.Username, "issuer", issuer)
	return bound, nil
}

func writeBindProblem(d httpapi.Deps, w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, errBindFromUntrustedHost):
		failSignIn(w, r, "bind-host")
	case errors.Is(err, errBindBeforeSetup):
		failSignIn(w, r, "setup-required")
	case errors.Is(err, errBindAmbiguous):
		failSignIn(w, r, "bind-ambiguous")
	case errors.Is(err, auth.ErrAlreadyBound):
		failSignIn(w, r, "other-identity")
	default:
		// An unexpected failure keeps its problem document and its request id:
		// this one is a bug report, not something the operator can act on from
		// the sign-in page.
		httpapi.WriteInternal(w, r, d.Logger, err)
	}
}

// completeSudo opens the sudo window after a re-authentication.
func completeSudo(d httpapi.Deps, w http.ResponseWriter, r *http.Request, identity oidc.Identity) {
	u, ok := d.Auth.CurrentUser(r.Context())
	if !ok {
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
		return
	}

	// The returning identity must be the one already signed in. Without this,
	// a second operator's completed flow would open the sudo window on the
	// first one's session.
	if !u.HasIdentityBinding() || u.Issuer != d.OIDC.Issuer() || u.Subject != identity.Subject {
		d.Logger.Warn("a sudo re-authentication came back as a different account than the one signed in",
			slog.String("signed-in", u.Username))
		failSudo(w, r, "oidc.other-identity")
		return
	}

	// prompt=login and max_age=0 asked the provider to authenticate afresh;
	// auth_time is where it says whether it did. A provider that reused an
	// existing session answers with an old timestamp, and accepting that would
	// make the sudo gate a redirect with no proof behind it.
	//
	// THE TWO CAUSES ARE TOLD APART because they need different repairs from
	// different people, and this used to answer both with one sentence. A
	// missing claim is the provider's configuration -- auth_time is OPTIONAL in
	// OIDC Core unless it is asked for, so a provider that never sends it makes
	// this gate permanently impassable however often the operator tries. An
	// auth_time that is merely old is the provider declining to re-prompt, which
	// the operator fixes by signing out there first. That distinction was
	// missing when an operator met this on real hardware: three round trips,
	// three identical 403s, nothing in the journal, and no way to tell which of
	// the two they were looking at.
	if identity.AuthTime.IsZero() {
		d.Logger.Warn("the identity provider sent no auth_time, so a re-authentication cannot be proven",
			slog.String("issuer", d.OIDC.Issuer()),
			slog.String("remedy", "the provider must emit the auth_time claim in its ID token; "+
				"it is optional in OIDC Core and this gate cannot work without it"))
		failSudo(w, r, "oidc.no-auth-time")
		return
	}
	if !identity.FreshlyAuthenticated(time.Now()) {
		d.Logger.Warn("the identity provider did not re-authenticate for a sudo round trip",
			slog.String("issuer", d.OIDC.Issuer()),
			slog.Time("auth_time", identity.AuthTime),
			slog.Duration("age", time.Since(identity.AuthTime)),
			slog.String("remedy", "the provider reused an existing session despite prompt=login; "+
				"sign out there first, or allow it to re-prompt"))
		failSudo(w, r, "oidc.not-fresh")
		return
	}

	d.Auth.OpenSudoWindow(r.Context())
	d.Auth.Sessions().Put(r.Context(), sessionKeyIDToken, identity.RawIDToken)
	http.Redirect(w, r, afterAuth, http.StatusFound)
}

// oidcLogout ends the local session and hands the browser to the provider's
// RP-initiated logout, so that the next sign-in is a real one.
//
// Without it, signing out here and back in round-trips through a provider that
// still holds a session and returns immediately -- which is indistinguishable,
// to the operator, from the logout having been ignored.
func oidcLogout(d httpapi.Deps, w http.ResponseWriter, r *http.Request) {
	idToken := d.Auth.Sessions().GetString(r.Context(), sessionKeyIDToken)

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	end := d.OIDC.EndSessionURL(r.Context(), idToken, scheme+"://"+r.Host+"/")

	if err := d.Auth.Logout(r.Context()); err != nil {
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}
	if end == "" {
		// The provider advertises no end_session_endpoint. The local session is
		// gone either way, which is the part this server is responsible for.
		http.Redirect(w, r, afterAuth, http.StatusFound)
		return
	}
	// Same as above: end comes from the provider's end_session_endpoint in the
	// discovery document, not from anything the caller sent.
	http.Redirect(w, r, end, http.StatusFound) //nolint:gosec // target derives from the configured issuer, not from the request
}

func clearFlow(d httpapi.Deps, r *http.Request) {
	sm := d.Auth.Sessions()
	for _, k := range []string{
		sessionKeyState, sessionKeyNonce, sessionKeyVerifier, sessionKeyRedirect, sessionKeySudoFlow,
	} {
		sm.Remove(r.Context(), k)
	}
}

// writeProviderProblem answers a provider outage with 503 rather than 500. The
// distinction is what tells the operator to use the local account instead of
// filing a bug against this server.
func writeProviderProblem(d httpapi.Deps, w http.ResponseWriter, r *http.Request, sudo bool, err error) {
	d.Logger.WarnContext(r.Context(), "identity provider unreachable", "error", err)
	if sudo {
		httpapi.WriteProblem(w, r, httpapi.Upstream("oidc.provider-unreachable",
			"The identity provider could not be reached."))
		return
	}
	failSignIn(w, r, "provider-unreachable")
}
