package auth

// Service accounts and their tokens (Omni parity phase 4).
//
// A service account is an identity that is not a person: it has a role, it
// appears in the audit archive under its own name, and it authenticates by
// presenting a token on every request rather than by holding a cookie. The
// asymmetry with a person's account is enforced in both directions -- a
// password never signs in a service account, and a token never signs in a
// person -- because an identity that can be reached two ways is an identity
// whose weakest way in is the one that matters.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// TokenPrefix marks a holzkube-manager service-account token.
//
// It is there so that a token pasted into a bug report, a log or a public
// repository is recognisable as one. Secret scanners key on exactly this kind
// of fixed prefix, and a token that looks like arbitrary base64 is a token
// nobody notices until it is used.
const TokenPrefix = "hkm_"

// tokenBytes is the entropy behind the prefix. 256 bits, from crypto/rand.
const tokenBytes = 32

// tokenUseThrottle is how stale LastUsedAt is allowed to be.
//
// Writing it on every request would put a store write in front of every API
// call a machine makes and lose a revision race with whatever that call was
// about to do. A minute is enough to answer the question the field exists for
// -- "is this token still in use, and roughly since when" -- and is not enough
// to be a write amplifier.
const tokenUseThrottle = time.Minute

var (
	// ErrNotAServiceAccount reports a token operation aimed at a person.
	ErrNotAServiceAccount = errors.New("auth: that account is a person, not a service account")

	// ErrNotAPerson reports a password operation aimed at a service account.
	ErrNotAPerson = errors.New("auth: that account is a service account and has no password")

	// ErrInvalidToken reports a token that authenticates nothing.
	ErrInvalidToken = errors.New("auth: that token is not valid")
)

// CreateServiceAccount adds a machine identity and returns its one token.
//
// The token is returned once and never again: only its hash is stored, so a
// lost token is replaced rather than recovered. That is the whole reason the
// return signature is shaped this way -- a function that could hand the token
// back later would need to keep it.
func (s *Service) CreateServiceAccount(ctx context.Context, id, username string, role model.UserRole) (model.User, string, error) {
	username = strings.TrimSpace(username)
	if !role.Valid() {
		return model.User{}, "", ErrInvalidRole
	}

	if _, err := s.FindByUsername(ctx, username); err == nil {
		return model.User{}, "", ErrUsernameTaken
	} else if !errors.Is(err, store.ErrNotFound) {
		return model.User{}, "", err
	}

	token, hash, err := newToken()
	if err != nil {
		return model.User{}, "", err
	}

	now := s.now().UTC()
	created, err := s.store.Users().Put(ctx, model.User{
		ID:            model.UserID(id),
		Username:      username,
		Kind:          model.KindService,
		Role:          role,
		TokenHash:     hash,
		TokenIssuedAt: now,
		CreatedAt:     now,
		// No PasswordHash, and that is what makes the account unreachable by
		// the sign-in form: verifyPassword runs Verify against an empty hash,
		// which fails, and the decoy path keeps the timing flat.
	})
	if err != nil {
		return model.User{}, "", err
	}

	created.TokenHash = ""
	return created, token, nil
}

// RotateToken mints a new token for a service account and invalidates the old
// one, which is the same act: there is one token per account.
//
// One and not many, deliberately. A set of tokens per account needs a
// lifecycle of its own -- names, expiries, a list, a revoke -- and every one of
// those is a place for a forgotten credential to keep working. Rotation is the
// operation people actually need, and it leaves nothing behind.
func (s *Service) RotateToken(ctx context.Context, id model.UserID) (string, error) {
	u, err := s.store.Users().Get(ctx, id)
	if err != nil {
		return "", err
	}
	if !u.IsService() {
		return "", ErrNotAServiceAccount
	}

	token, hash, err := newToken()
	if err != nil {
		return "", err
	}

	u.TokenHash = hash
	u.TokenIssuedAt = s.now().UTC()
	if _, err := s.store.Users().Put(ctx, u); err != nil {
		return "", err
	}
	return token, nil
}

// AuthenticateToken resolves a bearer token to the account that holds it.
func (s *Service) AuthenticateToken(ctx context.Context, token string) (model.User, error) {
	want := hashToken(token)
	if want == "" {
		return model.User{}, ErrInvalidToken
	}

	users, err := s.store.Users().List(ctx)
	if err != nil {
		return model.User{}, err
	}

	// Every account is compared, and the comparison is constant-time, so the
	// time this takes does not depend on which account matched or on how far
	// down the list it was. The scan is O(accounts) and accounts are counted
	// in single digits here; an index keyed by the hash would be faster and
	// would put the hash in a second place that has to stay in step.
	var found model.User
	matched := false
	for _, u := range users {
		if !u.IsService() || u.TokenHash == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(u.TokenHash), []byte(want)) == 1 {
			found, matched = u, true
		}
	}
	if !matched {
		return model.User{}, ErrInvalidToken
	}

	s.noteTokenUse(ctx, found)
	return found, nil
}

// noteTokenUse records that a token was used, at most once per throttle window.
//
// Best effort in every direction: a failed write is not a failed request, and a
// revision conflict means something else wrote the record in the same instant,
// which is exactly the case this throttle exists to make rare rather than
// impossible.
func (s *Service) noteTokenUse(ctx context.Context, u model.User) {
	now := s.now().UTC()
	if now.Sub(u.LastUsedAt) < tokenUseThrottle {
		return
	}
	u.LastUsedAt = now
	_, _ = s.store.Users().Put(ctx, u)
}

// newToken mints a token and the hash that is stored in its place.
func newToken() (token, hash string, err error) {
	var b [tokenBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", fmt.Errorf("auth: mint a token: %w", err)
	}
	token = TokenPrefix + base64.RawURLEncoding.EncodeToString(b[:])
	return token, hashToken(token), nil
}

// hashToken is what is stored in place of a token, and the empty string for
// anything that is not one.
func hashToken(token string) string {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, TokenPrefix) || len(token) <= len(TokenPrefix) {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
