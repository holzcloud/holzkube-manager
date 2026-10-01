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

// ErrNotLinked is returned when an unlink is asked of an account that has no
// provider binding.
var ErrNotLinked = errors.New("auth: the account is not linked to an identity provider")

// FindByIdentity returns the person account bound to (issuer, subject).
//
// A service account is never the answer, whatever the store holds. It signs in
// with a token, and a browser session for one skips everything a token is
// checked for -- for the break-glass account, its expiry. Such a binding is not
// hypothetical: before first-use linking counted only people, an instance whose
// only account was a service account bound the provider's identity to it. It is
// ignored here and removed by UnlinkIdentity.
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
		if u.IsService() || !u.HasIdentityBinding() {
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
// unbinding is a deliberate act with its own operation, UnlinkIdentity, behind
// an admin role and the re-authentication window, not a side effect of somebody
// signing in.
//
// The account is re-read from the store by its ID, and the stored record is
// what gets the binding. The caller's copy usually comes from Users or
// SinglePersonAccount, which strip the password hash -- writing that copy back
// would erase the password at the moment single sign-on first worked, and with
// it the break-glass way in.
//
// The stored record's kind is checked here, not left to the caller: a service
// account is never bound (ErrNotAPerson), and the record that counts is the one
// about to be written.
func (s *Service) BindIdentity(ctx context.Context, account model.User, issuer, subject string) (model.User, error) {
	if issuer == "" || subject == "" {
		return model.User{}, errors.New("auth: refusing to bind an empty identity")
	}
	u, err := s.store.Users().Get(ctx, account.ID)
	if err != nil {
		return model.User{}, fmt.Errorf("auth: read the account to bind: %w", err)
	}
	if u.IsService() {
		return model.User{}, ErrNotAPerson
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

// UnlinkIdentity removes an account's link to a provider identity.
//
// A service account's binding is removed like a person's. It never signs in
// (FindByIdentity ignores it), but it is a binding an earlier release made --
// first-use linking used to count every account, so a service-only instance
// bound the provider's identity to its service account -- and the remedy for a
// binding nobody wants is to remove it, not to refuse to look at it.
//
// Both halves go: an issuer without a subject, or the reverse, is not a
// binding HasIdentityBinding recognises, but it is a half-remembered one, and
// the next first-use bind would sit on top of whatever was left. The account
// itself -- its ID, role and password -- is untouched, so this decides only
// which account the next provider sign-in resolves to.
//
// Neither the issuer nor the subject goes into an error: they are somebody's
// identity at a third party, and an error is a thing that gets logged.
func (s *Service) UnlinkIdentity(ctx context.Context, id model.UserID) (model.User, error) {
	u, err := s.store.Users().Get(ctx, id)
	if err != nil {
		return model.User{}, err
	}

	// An unlink that changes nothing would still write a success record into
	// the audit archive, and that is a record of nothing.
	if !u.HasIdentityBinding() {
		return model.User{}, ErrNotLinked
	}

	u.Issuer = ""
	u.Subject = ""
	saved, err := s.store.Users().Put(ctx, u)
	if err != nil {
		return model.User{}, fmt.Errorf("auth: store unlinked account: %w", err)
	}
	saved.PasswordHash = ""
	return saved, nil
}

// ErrNoPersonAccount is returned by SinglePersonAccount when no account for a
// person exists -- before setup, or with only service accounts.
var ErrNoPersonAccount = errors.New("auth: there is no account for a person")

// ErrSeveralPersonAccounts is returned by SinglePersonAccount when more than
// one account for a person exists.
var ErrSeveralPersonAccounts = errors.New("auth: there is more than one account for a person")

// PersonAccounts returns every account for a person, with password hashes
// stripped as Users strips them.
func (s *Service) PersonAccounts(ctx context.Context) ([]model.User, error) {
	users, err := s.Users(ctx)
	if err != nil {
		return nil, err
	}
	people := make([]model.User, 0, len(users))
	for _, u := range users {
		if !u.IsService() {
			people = append(people, u)
		}
	}
	return people, nil
}

// SinglePersonAccount returns the only account for a person.
//
// The question it answers is who an identity arriving from the provider can
// belong to, and only a person ever signs in through the provider. So service
// accounts never make the answer ambiguous: counting them, as the function
// this replaced did, blocked first-use linking on every instance that had any
// automation (operator decision, 2026-10-01).
//
// "None" and "several" are separate errors because both callers -- the OIDC
// sign-in pre-check and first-use binding -- answer them differently: the
// first is "run setup", the second is "nothing can be linked automatically".
func (s *Service) SinglePersonAccount(ctx context.Context) (model.User, error) {
	people, err := s.PersonAccounts(ctx)
	if err != nil {
		return model.User{}, err
	}
	switch len(people) {
	case 0:
		return model.User{}, ErrNoPersonAccount
	case 1:
		return people[0], nil
	default:
		return model.User{}, ErrSeveralPersonAccounts
	}
}

func constantTimeEqual(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
