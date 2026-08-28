// Package auth owns password verification, the session lifecycle and the
// sudo-mode window. It knows nothing about Talos and nothing about HTTP.
package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/alexedwards/scs/v2"

	"github.com/holzcloud/holzkube/internal/auth/scsstore"
	"github.com/holzcloud/holzkube/internal/model"
	"github.com/holzcloud/holzkube/internal/store"
)

// ErrInvalidCredentials is returned for both an unknown username and a wrong
// password. Callers must not distinguish the two, and neither does this error.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")

// CookieName is the session cookie's name.
const CookieName = "holzkube_session"

const (
	sessionKeyUser      = "user_id"
	sessionKeySudoUntil = "sudo_until"
)

// hashParams targets roughly a quarter second of work on the machines holzkube
// runs on. The cost is the point: it is what turns an offline-quality brute
// force back into a slow one. The parameters are stored inside the encoded hash
// (FOUND-02), so they can be raised later without invalidating any password.
var hashParams = &argon2id.Params{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

// Service is the authentication service.
type Service struct {
	store store.Store
	sm    *scs.SessionManager
}

// New builds the service and its session manager. Sessions are absolute-lifetime
// only (D-07): 24 hours from issue, no sliding idle window.
func New(st store.Store, lifetime time.Duration) (*Service, error) {
	if st == nil {
		return nil, errors.New("auth: nil store")
	}
	if lifetime <= 0 {
		return nil, errors.New("auth: session lifetime must be positive")
	}

	sm := scs.New()
	sm.Store = scsstore.New(st.Sessions())
	sm.Lifetime = lifetime
	sm.IdleTimeout = 0
	sm.Cookie.Name = CookieName
	sm.Cookie.Path = "/"
	sm.Cookie.HttpOnly = true
	sm.Cookie.Secure = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Persist = true

	return &Service{store: st, sm: sm}, nil
}

// Sessions exposes the session manager so the HTTP layer can install its
// load-and-save middleware.
func (s *Service) Sessions() *scs.SessionManager { return s.sm }

// Hash derives an encoded argon2id hash. The encoded form carries the salt and
// the cost parameters alongside the digest.
func Hash(password string) (string, error) {
	if password == "" {
		return "", errors.New("auth: empty password")
	}
	return argon2id.CreateHash(password, hashParams)
}

// Verify checks a password against an encoded hash.
func Verify(password, encodedHash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, encodedHash)
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

// Login verifies the credentials and, on success, rotates the session id before
// attaching the identity to it.
//
// The order matters: RenewToken first, then write the identity. Renewing after
// would leave the authenticated identity briefly reachable under the
// pre-authentication session id, which is the session-fixation hole this is
// here to close (FOUND-02).
func (s *Service) Login(ctx context.Context, username, password string) (model.User, error) {
	u, err := s.FindByUsername(ctx, username)
	switch {
	case errors.Is(err, store.ErrNotFound):
		// Spend the same work as a real verification before failing.
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

	if err := s.sm.RenewToken(ctx); err != nil {
		return model.User{}, fmt.Errorf("auth: rotate session: %w", err)
	}
	s.sm.Put(ctx, sessionKeyUser, string(u.ID))
	return u, nil
}

// StartSession attaches an identity to a freshly rotated session without
// re-checking a password. It exists for the setup wizard, which has just proven
// possession of the credentials by choosing them.
func (s *Service) StartSession(ctx context.Context, u model.User) error {
	if err := s.sm.RenewToken(ctx); err != nil {
		return fmt.Errorf("auth: rotate session: %w", err)
	}
	s.sm.Put(ctx, sessionKeyUser, string(u.ID))
	return nil
}

// Logout destroys the session.
func (s *Service) Logout(ctx context.Context) error {
	return s.sm.Destroy(ctx)
}

// CurrentUser returns the authenticated account, if any.
func (s *Service) CurrentUser(ctx context.Context) (model.User, bool) {
	id := s.sm.GetString(ctx, sessionKeyUser)
	if id == "" {
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

// SessionID returns the current session token, for the audit trail.
func (s *Service) SessionID(ctx context.Context) string {
	return s.sm.Token(ctx)
}

// HasSudo reports whether the session is inside its sudo window.
//
// Nothing in this plan opens the window: there is no re-authentication endpoint
// yet, so the gate is fail-closed by construction and plan 04 is what makes it
// passable. A gate that defaults open until someone remembers to close it is
// not a gate.
func (s *Service) HasSudo(ctx context.Context) bool {
	until := s.sm.GetInt64(ctx, sessionKeySudoUntil)
	if until == 0 {
		return false
	}
	return time.Now().Before(time.Unix(until, 0))
}

// GrantSudo opens the sudo window for the given duration.
func (s *Service) GrantSudo(ctx context.Context, window time.Duration) {
	s.sm.Put(ctx, sessionKeySudoUntil, time.Now().Add(window).Unix())
}

func normalizeUsername(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
