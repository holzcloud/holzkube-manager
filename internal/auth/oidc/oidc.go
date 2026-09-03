// Package oidc speaks OpenID Connect to an external identity provider on behalf
// of holzkube-manager. It knows nothing about HTTP handlers, sessions or the
// store: it turns configuration into an authorisation URL, and an authorisation
// code back into a verified identity.
//
// The flow is the authorization code flow with PKCE (RFC 7636). PKCE is not
// optional here even though holzkube-manager is a confidential client with a
// secret: the redirect carrying the code lands in a browser on an operator's
// machine, and a code intercepted there is worth a session against cluster PKI.
//
// # Why discovery is lazy
//
// The provider's metadata is fetched on first use, not at start, and a failure
// to reach it is not a startup failure. holzkube-manager manages the cluster
// the identity provider is likely to be running on. Refusing to start without
// it would mean the tool for repairing that cluster is unavailable exactly when
// the cluster is broken -- and the local break-glass account, which needs no
// provider at all, would be unreachable behind the same refusal.
package oidc

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// ErrProviderUnreachable is returned when discovery has not succeeded. Callers
// answer it with 503 rather than 500: the request was well formed and the
// operator can retry, or fall back to the local account where one is allowed.
var ErrProviderUnreachable = errors.New("oidc: identity provider is unreachable")

// discoveryRetryAfter throttles rediscovery after a failure. Without it every
// login attempt during an outage becomes another outbound request to a service
// already known to be down.
const discoveryRetryAfter = 15 * time.Second

// maxAuthAge is how recently the provider must have actually authenticated the
// operator for a sudo re-authentication to count. It bounds what the provider's
// own session can silently satisfy: without it, prompt=login can be answered
// from an existing IdP session and the sudo window would open without anybody
// proving anything.
const maxAuthAge = 2 * time.Minute

// Identity is what a completed flow establishes about the operator.
type Identity struct {
	// Subject is the provider's stable identifier for the account. It is the
	// join key, never the username: a username can be reassigned to a different
	// person, and re-binding the local record to whoever holds the name now is
	// exactly the confusion this field exists to avoid.
	Subject string

	Username string
	Email    string
	Groups   []string

	// AuthTime is when the provider last actually authenticated the operator,
	// from the auth_time claim. Zero if the provider did not send one.
	AuthTime time.Time

	// RawIDToken is kept for RP-initiated logout, which passes it back as
	// id_token_hint so the provider knows which session to end.
	RawIDToken string
}

// Provider is the configured identity provider.
type Provider struct {
	issuer       string
	clientID     string
	clientSecret string

	mu          sync.Mutex
	discovered  *gooidc.Provider
	verifier    *gooidc.IDTokenVerifier
	lastAttempt time.Time
	lastErr     error
}

// New builds a Provider. It performs no network I/O: see the package comment.
func New(issuer, clientID, clientSecret string) (*Provider, error) {
	switch {
	case issuer == "":
		return nil, errors.New("oidc: empty issuer")
	case clientID == "":
		return nil, errors.New("oidc: empty client ID")
	case clientSecret == "":
		return nil, errors.New("oidc: empty client secret")
	}
	return &Provider{issuer: issuer, clientID: clientID, clientSecret: clientSecret}, nil
}

// Issuer is the configured issuer URL.
func (p *Provider) Issuer() string { return p.issuer }

// resolve returns the discovered provider, fetching metadata if necessary.
//
// A failure is cached for discoveryRetryAfter and reported as
// ErrProviderUnreachable with the cause attached, so a handler can distinguish
// "the provider is down" from "this token is bad" without inspecting strings.
func (p *Provider) resolve(ctx context.Context) (*gooidc.Provider, *gooidc.IDTokenVerifier, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.discovered != nil {
		return p.discovered, p.verifier, nil
	}
	if p.lastErr != nil && time.Since(p.lastAttempt) < discoveryRetryAfter {
		return nil, nil, fmt.Errorf("%w: %w", ErrProviderUnreachable, p.lastErr)
	}

	p.lastAttempt = time.Now()
	prov, err := gooidc.NewProvider(ctx, p.issuer)
	if err != nil {
		p.lastErr = err
		return nil, nil, fmt.Errorf("%w: %w", ErrProviderUnreachable, err)
	}

	p.discovered = prov
	p.verifier = prov.Verifier(&gooidc.Config{ClientID: p.clientID})
	p.lastErr = nil
	return p.discovered, p.verifier, nil
}

// Available reports whether discovery has already succeeded. It never triggers
// a fetch: it is for status reporting, where blocking on a dead provider would
// turn a health endpoint into a hang.
func (p *Provider) Available() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.discovered != nil
}

// FlowState is what the caller must keep between the redirect out and the
// callback back. It belongs in the server-side session and nowhere else: in a
// cookie it would be readable, and the whole point of state and nonce is that
// the browser cannot choose them.
type FlowState struct {
	State    string
	Nonce    string
	Verifier string

	// Redirect is the redirect_uri sent to the provider. It is stored because
	// the token exchange has to present the identical value (RFC 6749 4.1.3),
	// and it is derived per request from the Host the operator reached us on.
	Redirect string

	// Sudo marks a re-authentication rather than a login, so the callback knows
	// to open the sudo window instead of starting a session.
	Sudo bool
}

// NewFlowState generates the per-request secrets.
func NewFlowState(redirect string, sudo bool) (FlowState, error) {
	state, err := randomToken()
	if err != nil {
		return FlowState{}, err
	}
	nonce, err := randomToken()
	if err != nil {
		return FlowState{}, err
	}
	return FlowState{
		State:    state,
		Nonce:    nonce,
		Verifier: oauth2.GenerateVerifier(),
		Redirect: redirect,
		Sudo:     sudo,
	}, nil
}

// MatchesState compares in constant time. A timing-variable comparison of a
// value the attacker supplies and can retry is a byte-at-a-time oracle.
func (f FlowState) MatchesState(got string) bool {
	return len(got) == len(f.State) &&
		subtle.ConstantTimeCompare([]byte(f.State), []byte(got)) == 1
}

// AuthCodeURL is where the browser is sent to authenticate.
func (p *Provider) AuthCodeURL(ctx context.Context, f FlowState) (string, error) {
	prov, _, err := p.resolve(ctx)
	if err != nil {
		return "", err
	}

	opts := []oauth2.AuthCodeOption{
		gooidc.Nonce(f.Nonce),
		oauth2.S256ChallengeOption(f.Verifier),
	}
	if f.Sudo {
		// Two requirements, because either alone is insufficient. prompt=login
		// asks the provider to authenticate afresh; max_age=0 makes a stale
		// auth_time a protocol violation rather than a preference the provider
		// may decline. The auth_time claim is then checked on the way back.
		opts = append(opts,
			oauth2.SetAuthURLParam("prompt", "login"),
			oauth2.SetAuthURLParam("max_age", "0"))
	}

	return p.oauthConfig(prov, f.Redirect).AuthCodeURL(f.State, opts...), nil
}

// Exchange completes the flow: it trades the code for tokens, verifies the ID
// token's signature, issuer, audience and expiry, and checks the nonce.
func (p *Provider) Exchange(ctx context.Context, f FlowState, code string) (Identity, error) {
	prov, verifier, err := p.resolve(ctx)
	if err != nil {
		return Identity{}, err
	}

	token, err := p.oauthConfig(prov, f.Redirect).Exchange(ctx, code, oauth2.VerifierOption(f.Verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("oidc: code exchange: %w", err)
	}

	rawID, ok := token.Extra("id_token").(string)
	if !ok || rawID == "" {
		// An access token alone says the provider knows somebody; only the ID
		// token says who, in a form this client can verify.
		return Identity{}, errors.New("oidc: the token response carried no id_token")
	}

	idToken, err := verifier.Verify(ctx, rawID)
	if err != nil {
		return Identity{}, fmt.Errorf("oidc: verify id_token: %w", err)
	}

	// The nonce binds this token to the authorisation request this session
	// started. Without the check, a token obtained in any other flow for the
	// same client can be replayed into this one.
	if idToken.Nonce == "" || subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(f.Nonce)) != 1 {
		return Identity{}, errors.New("oidc: id_token nonce does not match this login attempt")
	}

	var claims struct {
		Subject           string   `json:"sub"`
		PreferredUsername string   `json:"preferred_username"`
		Name              string   `json:"name"`
		Email             string   `json:"email"`
		Groups            []string `json:"groups"`
		AuthTime          int64    `json:"auth_time"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("oidc: decode claims: %w", err)
	}
	if claims.Subject == "" {
		return Identity{}, errors.New("oidc: id_token has no sub claim")
	}

	id := Identity{
		Subject:    claims.Subject,
		Username:   firstNonEmpty(claims.PreferredUsername, claims.Name, claims.Email, claims.Subject),
		Email:      claims.Email,
		Groups:     claims.Groups,
		RawIDToken: rawID,
	}
	if claims.AuthTime > 0 {
		id.AuthTime = time.Unix(claims.AuthTime, 0)
	}
	return id, nil
}

// FreshlyAuthenticated reports whether the provider authenticated this identity
// recently enough to satisfy a sudo re-authentication.
//
// A provider that sends no auth_time cannot demonstrate freshness, and the
// answer is no. Treating a missing claim as "recent enough" would make the
// stricter configuration -- max_age=0, which is what asks for the claim -- the
// one that silently accepts anything.
func (id Identity) FreshlyAuthenticated(now time.Time) bool {
	if id.AuthTime.IsZero() {
		return false
	}
	age := now.Sub(id.AuthTime)
	return age >= -clockSkew && age <= maxAuthAge
}

// clockSkew tolerates a provider whose clock leads ours, which would otherwise
// make a just-issued auth_time look like it is from the future.
const clockSkew = time.Minute

// EndSessionURL is the provider's RP-initiated logout endpoint with this
// session's ID token attached, or "" if the provider does not advertise one.
//
// Logging out of holzkube-manager without it ends the local session only: the
// next login round-trips through a provider that still considers the operator
// signed in and returns immediately, which looks exactly like the logout having
// been ignored.
func (p *Provider) EndSessionURL(ctx context.Context, idToken, postLogoutRedirect string) string {
	prov, _, err := p.resolve(ctx)
	if err != nil {
		return ""
	}

	var meta struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := prov.Claims(&meta); err != nil || meta.EndSessionEndpoint == "" {
		return ""
	}
	u, err := url.Parse(meta.EndSessionEndpoint)
	if err != nil {
		return ""
	}

	q := u.Query()
	if idToken != "" {
		q.Set("id_token_hint", idToken)
	}
	if postLogoutRedirect != "" {
		q.Set("post_logout_redirect_uri", postLogoutRedirect)
	}
	q.Set("client_id", p.clientID)
	u.RawQuery = q.Encode()
	return u.String()
}

// oauthConfig builds the exchange configuration for one redirect URI.
//
// The redirect URI is per request rather than configured, because one instance
// answers on more than one name -- a LAN address and a public hostname -- and
// the provider requires the value presented at the token endpoint to be
// byte-identical to the one in the authorisation request. Deriving it from the
// request's Host is safe only because that Host was already checked against the
// allowlist by middleware.AllowHosts; an unchecked Host here would let a caller
// name the redirect target.
func (p *Provider) oauthConfig(prov *gooidc.Provider, redirect string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     p.clientID,
		ClientSecret: p.clientSecret,
		Endpoint:     prov.Endpoint(),
		RedirectURL:  redirect,
		Scopes:       []string{gooidc.ScopeOpenID, "profile", "email"},
	}
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("oidc: read randomness: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
