// Package auth owns password verification, the session lifecycle, the sudo-mode
// window and the login rate limit. It knows nothing about Talos and nothing
// about HTTP.
//
// The file split follows the four concerns: argon.go is the cost of a password,
// session.go is how long an identity lasts, sudo.go is what a valid session is
// still not allowed to do, and ratelimit.go is how slowly guessing goes.
package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/holzcloud/holzkube/internal/model"
	"github.com/holzcloud/holzkube/internal/store"
)

// ErrInvalidCredentials is returned for an unknown username, a wrong password
// and a wrong current password on a change. Callers must not distinguish them,
// and neither does this error.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")

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

// New builds the service and its session manager. Sessions are absolute-lifetime
// only (D-07): 24 hours from issue, with no sliding window.
func New(st store.Store, lifetime time.Duration) (*Service, error) {
	if st == nil {
		return nil, errors.New("auth: nil store")
	}
	if lifetime <= 0 {
		return nil, errors.New("auth: session lifetime must be positive")
	}

	// Calibrate here rather than lazily on the first login, so the cost is
	// paid and reported at start instead of showing up as a slow first
	// request that nobody can explain.
	_ = ActiveParams()

	return &Service{
		store:    st,
		sm:       newSessionManager(st, lifetime),
		lifetime: lifetime,
		now:      time.Now,
	}, nil
}

// decoyHash is verified against when the username is unknown, so that the
// "no such user" path costs the same as the "wrong password" path. Without it
// the response time alone enumerates accounts.
var decoyHash = sync.OnceValue(func() string {
	h, err := Hash("holzkube-decoy-password-never-valid")
	if err != nil {
		return ""
	}
	return h
})

// FindByUsername returns the account with the given username.
func (s *Service) FindByUsername(ctx context.Context, username string) (model.User, error) {
	users, err := s.store.Users().List(ctx)
	if err != nil {
		return model.User{}, err
	}
	want := normalizeUsername(username)
	for _, u := range users {
		if subtle.ConstantTimeCompare([]byte(normalizeUsername(u.Username)), []byte(want)) == 1 {
			return u, nil
		}
	}
	return model.User{}, store.ErrNotFound
}

// verifyPassword checks a password against the one account, spending the same
// work whether the account exists or not.
func (s *Service) verifyPassword(ctx context.Context, username, password string) (model.User, error) {
	u, err := s.FindByUsername(ctx, username)
	switch {
	case errors.Is(err, store.ErrNotFound):
		if h := decoyHash(); h != "" {
			_, _ = Verify(password, h)
		}
		return model.User{}, ErrInvalidCredentials
	case err != nil:
		return model.User{}, err
	}

	ok, err := Verify(password, u.PasswordHash)
	if err != nil {
		return model.User{}, fmt.Errorf("auth: verify password: %w", err)
	}
	if !ok {
		return model.User{}, ErrInvalidCredentials
	}
	return u, nil
}

// Login verifies the credentials and, on success, rotates the session id before
// attaching the identity to it.
//
// The order matters: RenewToken first, then write the identity. Renewing after
// would leave the authenticated identity briefly reachable under the
// pre-authentication session id, which is the session-fixation hole this is
// here to close (FOUND-02).
//
// A successful login is also the only moment the cleartext password exists in
// the process, so it is the only moment a hash written with older, cheaper
// parameters can be upgraded. That is what makes raising the cost a decision
// rather than a migration.
func (s *Service) Login(ctx context.Context, username, password string) (model.User, error) {
	u, err := s.verifyPassword(ctx, username, password)
	if err != nil {
		return model.User{}, err
	}

	if err := s.sm.RenewToken(ctx); err != nil {
		return model.User{}, fmt.Errorf("auth: rotate session: %w", err)
	}
	s.markAuthenticated(ctx, string(u.ID))

	u = s.rehashIfOutdated(ctx, u, password)
	return u, nil
}

// rehashIfOutdated rewrites the stored hash when the active parameters are
// above the ones it was written with.
//
// Failure is logged and swallowed on purpose: the operator has just proved
// their identity, and refusing the login because a background upgrade lost a
// revision race would turn a security improvement into an outage.
func (s *Service) rehashIfOutdated(ctx context.Context, u model.User, password string) model.User {
	if !NeedsRehash(u.PasswordHash) {
		return u
	}
	hash, err := Hash(password)
	if err != nil {
		slog.WarnContext(ctx, "could not rehash the password with the current parameters", slog.Any("error", err))
		return u
	}
	u.PasswordHash = hash
	updated, err := s.store.Users().Put(ctx, u)
	if err != nil {
		slog.WarnContext(ctx, "could not store the rehashed password", slog.Any("error", err))
		return u
	}
	slog.InfoContext(ctx, "password hash upgraded to the current argon2id parameters")
	return updated
}

// StartSession attaches an identity to a freshly rotated session without
// re-checking a password. It exists for the setup wizard, which has just proven
// possession of the credentials by choosing them.
func (s *Service) StartSession(ctx context.Context, u model.User) error {
	if err := s.sm.RenewToken(ctx); err != nil {
		return fmt.Errorf("auth: rotate session: %w", err)
	}
	s.markAuthenticated(ctx, string(u.ID))
	return nil
}

// Logout destroys the session.
func (s *Service) Logout(ctx context.Context) error {
	return s.sm.Destroy(ctx)
}

// ChangePassword replaces the operator's password and invalidates every other
// session.
//
// Keeping the calling session alive is deliberate. A password change in a
// single-operator tool is nearly always a reaction to a suspicion; logging the
// operator out of the browser they are fixing things in punishes exactly the
// right instinct. Every other session goes, because those are the ones that
// might not be theirs.
//
// A non-nil error means the password was not changed. Once the new hash is
// durable this returns nil even if the invalidation sweep failed, because the
// alternative tells the operator to retry a change that already succeeded.
func (s *Service) ChangePassword(ctx context.Context, current, next string) error {
	u, ok := s.CurrentUser(ctx)
	if !ok {
		return ErrInvalidCredentials
	}

	valid, err := Verify(current, u.PasswordHash)
	if err != nil {
		return fmt.Errorf("auth: verify current password: %w", err)
	}
	if !valid {
		return ErrInvalidCredentials
	}

	hash, err := Hash(next)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	if _, err := s.store.Users().Put(ctx, u); err != nil {
		return fmt.Errorf("auth: store new password: %w", err)
	}

	if err := s.InvalidateAllExcept(ctx, s.sm.Token(ctx)); err != nil {
		// The password write above is the operation, and it is already
		// durable. Reporting failure here maps to a 500 and tells the operator
		// the change did not happen -- so they retry with the old current
		// password, get 401, and conclude they are locked out of the
		// credential guarding their cluster PKI. The invalidation is
		// best-effort cleanup, and it fails for reasons as ordinary as one
		// unparsable session file, so it is logged rather than surfaced. Same
		// reasoning as rehashIfOutdated.
		slog.ErrorContext(ctx, "password changed, but other sessions could not all be invalidated",
			slog.Any("error", err))
	}
	return nil
}

// CurrentUser returns the authenticated account, if any.
func (s *Service) CurrentUser(ctx context.Context) (model.User, bool) {
	id := s.sm.GetString(ctx, sessionKeyUser)
	if id == "" {
		return model.User{}, false
	}
	if !s.withinAbsoluteLifetime(ctx) {
		return model.User{}, false
	}
	u, err := s.store.Users().Get(ctx, model.UserID(id))
	if err != nil {
		return model.User{}, false
	}
	return u, true
}

// IsAuthenticated reports whether the request carries a live session.
func (s *Service) IsAuthenticated(ctx context.Context) bool {
	_, ok := s.CurrentUser(ctx)
	return ok
}

func normalizeUsername(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
