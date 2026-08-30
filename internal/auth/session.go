package auth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/holzcloud/holzkube-manager/internal/auth/scsstore"
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
