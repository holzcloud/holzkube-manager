package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/holzcloud/holzkube-manager/internal/auth/scsstore"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// CookieName is the session cookie's name.
const CookieName = "holzkube-manager_session"

// Session state keys. The values live in the session record, which travels
// through store.Sessions() like every other record (FOUND-07).
const (
	sessionKeyUser = "user_id"

	// sessionKeyAuthenticatedAt is when this session's identity was
	// established, in Unix nanoseconds. It is the anchor of the absolute
	// lifetime: the limit is measured from here and from nothing else, so no
	// amount of later traffic can move it.
	sessionKeyAuthenticatedAt = "authenticated_at"

	// sessionKeyProviderSignIn marks a session that came in through the
	// identity provider. The value predates this package owning it -- the OIDC
	// handlers wrote it -- and keeping it means a session from an earlier
	// release is still recognised for what it is.
	sessionKeyProviderSignIn = "oidc.authenticated"

	// sessionKeyProviderBinding is a fingerprint of the provider identity the
	// session came in through: a hash, so that the session record does not
	// carry somebody's subject at a third party.
	sessionKeyProviderBinding = "oidc.binding"
)

// newSessionManager configures the session manager for D-07: an absolute
// lifetime, server-side state, and a cookie a browser will only return to us.
//
// The manager's inactivity-expiry option is deliberately left at its zero value
// and never assigned. A sliding window would extend a session for as long as
// somebody kept using it, which is exactly the property the operator chose
// against when they picked 24 hours over 30 days.
func newSessionManager(st store.Store, lifetime time.Duration) *scs.SessionManager {
	sm := scs.New()
	sm.Store = scsstore.New(st.Sessions())
	sm.Lifetime = lifetime
	sm.Cookie.Name = CookieName
	sm.Cookie.Path = "/"
	sm.Cookie.HttpOnly = true
	sm.Cookie.Secure = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Persist = true
	return sm
}

// Sessions exposes the session manager so the HTTP layer can install its
// load-and-save middleware.
func (s *Service) Sessions() *scs.SessionManager { return s.sm }

// SessionID returns the current session token, for the audit trail.
//
// The audit package shortens it where the record is sealed, so nothing that
// reaches a log holds a usable credential.
func (s *Service) SessionID(ctx context.Context) string {
	return s.sm.Token(ctx)
}

// markAuthenticated attaches an identity to the session and stamps the moment
// the absolute lifetime is counted from.
//
// Callers must have rotated the token first: writing the identity onto the
// pre-authentication id, even briefly, is the session-fixation hole (FOUND-02).
func (s *Service) markAuthenticated(ctx context.Context, id string) {
	s.sm.Put(ctx, sessionKeyUser, id)
	s.sm.Put(ctx, sessionKeyAuthenticatedAt, s.now().UnixNano())

	// Rotating the token keeps the session's data, so a sign-in in a browser
	// whose previous session came through the provider would otherwise inherit
	// that session's tie to the link. StartProviderSession sets the marks
	// again after this; every other way in is not through the provider.
	s.sm.Remove(ctx, sessionKeyProviderSignIn)
	s.sm.Remove(ctx, sessionKeyProviderBinding)
}

// StartProviderSession is StartSession for a sign-in through the identity
// provider, and ties the session to the identity it came in through.
//
// Such a session lives only as long as that identity is linked to its account
// (see providerLinkHolds). That is what makes an unlink the way to end it: the
// remedy for a wrong link is an unlink, and a remedy that left the wrong
// person's session open until it expired would be no remedy -- the other way to
// end a session, removing the account, is refused for the only admin.
func (s *Service) StartProviderSession(ctx context.Context, u model.User, issuer, subject string) error {
	if err := s.StartSession(ctx, u); err != nil {
		return err
	}
	s.sm.Put(ctx, sessionKeyProviderSignIn, true)
	s.sm.Put(ctx, sessionKeyProviderBinding, bindingFingerprint(issuer, subject))
	return nil
}

// SignedInThroughProvider reports whether this session came in through the
// identity provider.
func (s *Service) SignedInThroughProvider(ctx context.Context) bool {
	return s.sm.GetBool(ctx, sessionKeyProviderSignIn)
}

// providerLinkHolds reports whether a session's way in still exists.
//
// A password session always holds. A session through the provider holds while
// its account is linked to the identity it came in through: an unlink ends it
// at its next request, and so does a relink to a different identity. A
// provider session from before the fingerprint existed carries none, and of it
// only "the account is still linked" can be checked.
func (s *Service) providerLinkHolds(ctx context.Context, u model.User) bool {
	if !s.sm.GetBool(ctx, sessionKeyProviderSignIn) {
		return true
	}
	if !u.HasIdentityBinding() {
		return false
	}
	fp := s.sm.GetString(ctx, sessionKeyProviderBinding)
	if fp == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(fp), []byte(bindingFingerprint(u.Issuer, u.Subject))) == 1
}

// bindingFingerprint is a one-way digest of a provider identity.
func bindingFingerprint(issuer, subject string) string {
	sum := sha256.Sum256([]byte(issuer + "\x00" + subject))
	return hex.EncodeToString(sum[:])
}

// withinAbsoluteLifetime reports whether the session was authenticated less
// than one lifetime ago.
//
// The session manager enforces its own deadline as well; this is the second,
// independent barrier, and it is the one a test can move a clock against. A
// session with no stamp at all -- written by nothing this package does -- is
// treated as expired, because failing towards "log in again" costs a login and
// failing the other way costs the session limit.
func (s *Service) withinAbsoluteLifetime(ctx context.Context) bool {
	stamp := s.sm.GetInt64(ctx, sessionKeyAuthenticatedAt)
	if stamp == 0 {
		return false
	}
	return s.now().Before(time.Unix(0, stamp).Add(s.lifetime))
}

// InvalidateAllExcept deletes every session record but one.
//
// It reaches the sessions through the store rather than through the session
// manager because there is no per-user index: holzkube-manager has exactly one
// operator, so every other live session belongs to them, and the question
// "which of my sessions are these" has no interesting answer.
func (s *Service) InvalidateAllExcept(ctx context.Context, keep string) error {
	records, err := s.store.Sessions().List(ctx)
	if err != nil {
		return fmt.Errorf("auth: list sessions: %w", err)
	}
	for _, rec := range records {
		if rec.ID == keep {
			continue
		}
		if err := s.store.Sessions().Delete(ctx, rec.ID); err != nil {
			return fmt.Errorf("auth: delete session: %w", err)
		}
	}
	return nil
}
