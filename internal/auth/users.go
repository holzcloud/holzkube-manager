package auth

// Account management (V2-AUTH-02).
//
// The rule the whole file turns on is the lockout guard: an instance must
// always have at least one admin who can sign in with a password. Every refusal
// here is a case where a single legitimate-looking action would leave nobody
// able to manage the instance, and the repair for that is a shell on the host.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

var (
	// ErrLastAdmin reports a change that would leave the instance with no
	// admin who can sign in.
	ErrLastAdmin = errors.New("auth: this is the last account that can manage this instance")

	// ErrUsernameTaken reports a username another account already has.
	ErrUsernameTaken = errors.New("auth: that username is taken")

	// ErrInvalidRole reports a role that is not one of the three.
	ErrInvalidRole = errors.New("auth: that is not a role")

	// ErrSelfDemotion reports an admin removing their own ability to manage
	// accounts. It is separate from ErrLastAdmin because the remedy is
	// different: another admin can do it, and the account itself cannot.
	ErrSelfDemotion = errors.New("auth: an account cannot take away its own admin role")
)

// Users lists the accounts, without password hashes.
//
// The hash is stripped here rather than at the HTTP boundary. A hash is not a
// password and is also not nothing -- it is the input to an offline attack --
// and the place to decide it never leaves this package is the place that reads
// it from the store, not each of the handlers that might forget.
func (s *Service) Users(ctx context.Context) ([]model.User, error) {
	users, err := s.store.Users().List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range users {
		users[i].PasswordHash = ""
		users[i].Role = users[i].Role.OrAdmin()
	}
	return users, nil
}

// CreateUser adds an account.
func (s *Service) CreateUser(ctx context.Context, id, username, password string, role model.UserRole) (model.User, error) {
	username = strings.TrimSpace(username)
	if !role.Valid() {
		return model.User{}, ErrInvalidRole
	}

	if _, err := s.FindByUsername(ctx, username); err == nil {
		return model.User{}, ErrUsernameTaken
	} else if !errors.Is(err, store.ErrNotFound) {
		return model.User{}, err
	}

	hash, err := Hash(password)
	if err != nil {
		return model.User{}, fmt.Errorf("auth: hash the password: %w", err)
	}

	created, err := s.store.Users().Put(ctx, model.User{
		ID:           model.UserID(id),
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		CreatedAt:    s.now().UTC(),
	})
	if err != nil {
		return model.User{}, err
	}
	created.PasswordHash = ""
	return created, nil
}

// SetRole changes one account's role.
//
// Two refusals, and they are different questions. An account may not take its
// own admin role away, because the operator doing it is one click from having
// nobody to undo it -- and the instance may not lose its last admin at all.
// Reporting them separately is what lets the interface say "ask another admin"
// in one case and "promote somebody first" in the other.
func (s *Service) SetRole(ctx context.Context, actor model.UserID, id model.UserID, role model.UserRole) (model.User, error) {
	if !role.Valid() {
		return model.User{}, ErrInvalidRole
	}

	u, err := s.store.Users().Get(ctx, id)
	if err != nil {
		return model.User{}, err
	}

	demoting := u.Role.OrAdmin() == model.RoleAdmin && role != model.RoleAdmin
	if demoting {
		if id == actor {
			return model.User{}, ErrSelfDemotion
		}
		if err := s.refuseIfLastAdmin(ctx, id); err != nil {
			return model.User{}, err
		}
	}

	u.Role = role
	saved, err := s.store.Users().Put(ctx, u)
	if err != nil {
		return model.User{}, err
	}
	saved.PasswordHash = ""
	return saved, nil
}

// DeleteUser removes an account.
//
// An account may delete itself -- that is somebody leaving, and refusing it
// would make an admin ask another admin to do a thing they are entitled to do.
// What it may not do is leave the instance with no admin.
func (s *Service) DeleteUser(ctx context.Context, id model.UserID) error {
	u, err := s.store.Users().Get(ctx, id)
	if err != nil {
		return err
	}
	if u.Role.OrAdmin() == model.RoleAdmin {
		if err := s.refuseIfLastAdmin(ctx, id); err != nil {
			return err
		}
	}
	return s.store.Users().Delete(ctx, id)
}

// SetPassword replaces an account's password without knowing the old one.
//
// It is the admin's reset, and it is deliberately not the same operation as
// the account's own change: that one asks for the current password, because
// what it defends against is a stolen session. This one cannot ask, because
// the point of it is that nobody has the old password any more.
func (s *Service) SetPassword(ctx context.Context, id model.UserID, password string) error {
	u, err := s.store.Users().Get(ctx, id)
	if err != nil {
		return err
	}

	hash, err := Hash(password)
	if err != nil {
		return fmt.Errorf("auth: hash the password: %w", err)
	}
	u.PasswordHash = hash

	_, err = s.store.Users().Put(ctx, u)
	return err
}

// refuseIfLastAdmin fails when id is the only account that can manage this
// instance.
//
// "Can manage" means admin, and it deliberately does not ask whether the
// account has a usable password: an admin bound to an identity provider and no
// password is still an admin, and treating them as absent would delete the
// account of the person who is about to sign in. The case that is genuinely
// lost -- every admin bound to a provider that has gone away -- is a shell on
// the host, and no rule here can prevent it without preventing single sign-on.
func (s *Service) refuseIfLastAdmin(ctx context.Context, id model.UserID) error {
	users, err := s.store.Users().List(ctx)
	if err != nil {
		return err
	}

	for _, u := range users {
		if u.ID != id && u.Role.OrAdmin() == model.RoleAdmin {
			return nil
		}
	}
	return fmt.Errorf("%w: promote another account to admin first", ErrLastAdmin)
}
