package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// ErrNoIdentityBinding is returned when no account is linked to the given
// provider identity. It is a distinct error rather than ErrInvalidCredentials
// because the caller acts on it: a first sign-in through the provider is
// answered by binding the account, not by refusing the operator.
var ErrNoIdentityBinding = errors.New("auth: no account is bound to this identity")

// ErrAlreadyBound is returned when an account is already linked to a different
// provider identity.
var ErrAlreadyBound = errors.New("auth: the account is already bound to another identity")

// FindByIdentity returns the account bound to (issuer, subject).
//
// Both halves are compared in constant time. They are not secrets, but a
// subject is attacker-supplied on every callback, and a comparison whose timing
// depends on a shared prefix is an oracle for a value that is otherwise only
// obtainable from the provider.
func (s *Service) FindByIdentity(ctx context.Context, issuer, subject string) (model.User, error) {
	if issuer == "" || subject == "" {
		return model.User{}, ErrNoIdentityBinding
	}

	users, err := s.store.Users().List(ctx)
	if err != nil {
		return model.User{}, fmt.Errorf("auth: list users: %w", err)
	}
	for _, u := range users {
		if !u.HasIdentityBinding() {
			continue
		}
		if constantTimeEqual(u.Issuer, issuer) && constantTimeEqual(u.Subject, subject) {
			return u, nil
		}
	}
	return model.User{}, ErrNoIdentityBinding
}

// BindIdentity links an account to a provider identity.
//
// It refuses to move a binding that already exists. Re-pointing an account at a
// different subject is indistinguishable, from the store's side, from an
// attacker who reached this path taking over the only operator account -- and
// unbinding is a deliberate act that belongs in its own operation, not a side
// effect of somebody signing in.
func (s *Service) BindIdentity(ctx context.Context, u model.User, issuer, subject string) (model.User, error) {
	if issuer == "" || subject == "" {
		return model.User{}, errors.New("auth: refusing to bind an empty identity")
	}
	if u.HasIdentityBinding() {
		if constantTimeEqual(u.Issuer, issuer) && constantTimeEqual(u.Subject, subject) {
			return u, nil
		}
		return model.User{}, ErrAlreadyBound
	}

	u.Issuer = issuer
	u.Subject = subject
	bound, err := s.store.Users().Put(ctx, u)
	if err != nil {
		return model.User{}, fmt.Errorf("auth: store identity binding: %w", err)
	}
	return bound, nil
}

// SingleAccount returns the sole operator account.
//
// holzkube-manager is a single-operator tool: setup creates one account and
// refuses to create a second. This returns ErrNoIdentityBinding's sibling
// condition -- no account at all -- as ErrInvalidCredentials, so that a caller
// racing an unfinished setup cannot tell "no accounts yet" from "wrong
// credentials".
func (s *Service) SingleAccount(ctx context.Context) (model.User, error) {
	users, err := s.store.Users().List(ctx)
	if err != nil {
		return model.User{}, fmt.Errorf("auth: list users: %w", err)
	}
	if len(users) != 1 {
		return model.User{}, ErrInvalidCredentials
	}
	return users[0], nil
}

func constantTimeEqual(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
