package auth

// The break-glass token: an admin credential minted on the machine itself,
// without anybody signing in.
//
// # Why this is allowed to exist
//
// It grants nothing new. Whoever can open this data directory can already read
// every cluster secret in it, every session record and every password hash; the
// directory IS the authority. What the subcommand adds is not access, it is a
// SUPPORTED and AUDITED way to use access somebody already has, instead of the
// unsupported one -- hand-editing the store -- which leaves no record and can
// corrupt it.
//
// That reasoning is the whole justification, and it is also the limit of it: if
// this were ever reachable over the network, or from an account that could not
// already read the directory, none of the above would hold and the feature
// would be a back door. It is a subcommand operating on a path, and it must
// stay one.
//
// # What makes it defensible in practice
//
// - It EXPIRES. A credential handed out without a sign-in must not outlive the
//   errand it was minted for, and the default is minutes. This is the reason
//   model.User.TokenExpiresAt exists at all.
// - It is ONE account, reused. Minting again rotates the same identity rather
//   than leaving a trail of admin accounts nobody remembers creating.
// - It is NAMED. It appears in every list and in the audit archive as
//   "break-glass", so an operator who did not mint it can see that somebody did.
// - It is REVOKED like anything else: delete the account.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// BreakGlassUsername is the one account this mints, and it is deliberately
// obvious. A credential of this kind hiding under an innocuous name would be
// the difference between a tool and a back door.
const BreakGlassUsername = "break-glass"

// BreakGlassID is fixed so that minting twice rotates one account rather than
// racing to create a second with the same name.
const BreakGlassID = "u-break-glass"

// DefaultBreakGlassTTL is how long a freshly minted token lasts.
//
// Fifteen minutes is enough to do the thing it was minted for and short enough
// that a token left in a shell history is worthless by the time anybody reads
// it.
const DefaultBreakGlassTTL = 15 * time.Minute

// MaxBreakGlassTTL bounds --ttl.
//
// A day, and not longer, because past that the honest thing to ask for is a
// service account created through the product with somebody signed in -- which
// has a person's decision behind it and this does not.
const MaxBreakGlassTTL = 24 * time.Hour

// ErrTTLTooLong reports a --ttl past MaxBreakGlassTTL.
var ErrTTLTooLong = errors.New("auth: a break-glass token cannot last that long")

// MintBreakGlass creates or rotates the break-glass account and returns its
// token, once.
//
// The role is always admin. A break-glass credential that could not do the
// thing somebody broke the glass for would be an invitation to hand-edit the
// store instead, which is the outcome this exists to prevent.
func (s *Service) MintBreakGlass(ctx context.Context, ttl time.Duration) (model.User, string, error) {
	if ttl <= 0 {
		ttl = DefaultBreakGlassTTL
	}
	if ttl > MaxBreakGlassTTL {
		return model.User{}, "", fmt.Errorf("%w: %s is more than %s", ErrTTLTooLong, ttl, MaxBreakGlassTTL)
	}

	token, hash, err := newToken()
	if err != nil {
		return model.User{}, "", err
	}
	now := s.now().UTC()

	// Read-modify-write rather than create-or-fail, so minting a second time
	// rotates the token of the same identity. The previous one stops working at
	// that moment, which is the behaviour somebody expects from a credential
	// they have just re-issued.
	existing, err := s.store.Users().Get(ctx, model.UserID(BreakGlassID))
	switch {
	case err == nil:
		if !existing.IsService() {
			// Somebody made a person by this name. Refuse rather than convert:
			// turning an account with a password into a token account in place
			// is the kind of surprise that ends with a person locked out.
			return model.User{}, "", ErrNotAServiceAccount
		}
	case errors.Is(err, store.ErrNotFound):
		existing = model.User{
			ID:        model.UserID(BreakGlassID),
			Username:  BreakGlassUsername,
			Kind:      model.KindService,
			CreatedAt: now,
		}
	default:
		return model.User{}, "", err
	}

	existing.Role = model.RoleAdmin
	existing.TokenHash = hash
	existing.TokenIssuedAt = now
	existing.TokenExpiresAt = now.Add(ttl)

	saved, err := s.store.Users().Put(ctx, existing)
	if err != nil {
		return model.User{}, "", err
	}
	saved.TokenHash = ""
	return saved, token, nil
}
