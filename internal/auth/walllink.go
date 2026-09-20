package auth

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

// The link a screen in a corridor is left open on (2026-09-20).
//
// # Why this credential exists at all
//
// A session expires. A television showing a sign-in page at three in the morning
// is worse than no wall at all: it is a screen that stopped answering the one
// question it was put up for, and nobody will notice until somebody needs it.
//
// The operator chose this shape from a list: a long-lived, read-only, revocable
// link, bookmarked on the screen once.
//
// # It opens ONE route
//
// Not a role, not a session, not a service account. A reader may read the audit
// archive, every Secret's key names and every cluster's configuration; this URL
// lives on a television, gets bookmarked, photographed and mailed around an
// office. So it authorises exactly the wall route, which is a property somebody
// can check by reading the route table rather than one they have to reason about
// from a role.
//
// # A separate prefix, on purpose
//
// `hkw_` rather than the service account's `hkm_`. Two reasons, and the second
// is the one that matters: a secret scanner keys on a fixed prefix, and a wall
// token pasted where a service token belongs fails as a wall token rather than
// being tried as an account and matching nothing for a reason nobody can read.

// WallTokenPrefix marks a wall link's token.
const WallTokenPrefix = "hkw_"

// MaxWallLinks bounds how many can exist.
//
// Not tidiness: every request carrying a token compares against all of them in
// constant time, and a list nobody prunes is a list of credentials nobody has
// looked at. Eight is more screens than a homelab has and few enough that the
// settings list is readable at a glance.
const MaxWallLinks = 8

var (
	// ErrTooManyWallLinks reports the cap.
	ErrTooManyWallLinks = errors.New("auth: there are already as many wall links as this allows")

	// ErrNoSuchWallLink reports a revocation of something that is not there.
	ErrNoSuchWallLink = errors.New("auth: no such wall link")

	// ErrInvalidWallLink reports a token that opens nothing.
	ErrInvalidWallLink = errors.New("auth: that link is not valid")
)

// CreateWallLink mints one and returns its token once.
//
// The token is returned here and never again: only its hash is stored. A lost
// link is replaced, not recovered, which is the whole reason the signature is
// shaped this way.
func (s *Service) CreateWallLink(ctx context.Context, label, createdBy string) (model.WallLink, string, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		// A list of links with no labels is a list nobody can revoke safely:
		// the whole question at revocation time is WHICH screen this was.
		return model.WallLink{}, "", fmt.Errorf("%w: a wall link needs a label saying which screen it is for", ErrInvalidWallLink)
	}

	settings, err := s.currentSettings(ctx)
	if err != nil {
		return model.WallLink{}, "", err
	}
	if len(settings.WallLinks) >= MaxWallLinks {
		return model.WallLink{}, "", ErrTooManyWallLinks
	}

	token, hash, err := newWallToken()
	if err != nil {
		return model.WallLink{}, "", err
	}
	// The id is NOT derived from the token: an identifier that appears in the
	// settings list, in the archive and in a revoke URL must not be a piece of
	// the secret it identifies.
	id, _, err := newWallToken()
	if err != nil {
		return model.WallLink{}, "", err
	}

	link := model.WallLink{
		ID:        strings.TrimPrefix(id, WallTokenPrefix)[:16],
		Label:     label,
		TokenHash: hash,
		CreatedAt: s.now().UTC(),
		CreatedBy: createdBy,
	}
	settings.WallLinks = append(settings.WallLinks, link)
	if _, err := s.store.Settings().Put(ctx, settings); err != nil {
		return model.WallLink{}, "", err
	}

	link.TokenHash = ""
	return link, token, nil
}

// WallLinks lists them, without their hashes.
func (s *Service) WallLinks(ctx context.Context) ([]model.WallLink, error) {
	settings, err := s.currentSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.WallLink, 0, len(settings.WallLinks))
	for _, link := range settings.WallLinks {
		link.TokenHash = ""
		out = append(out, link)
	}
	return out, nil
}

// RevokeWallLink removes one. The screen showing it stops working at its next
// refresh, which is the point.
func (s *Service) RevokeWallLink(ctx context.Context, id string) error {
	settings, err := s.currentSettings(ctx)
	if err != nil {
		return err
	}

	kept := make([]model.WallLink, 0, len(settings.WallLinks))
	found := false
	for _, link := range settings.WallLinks {
		if link.ID == id {
			found = true
			continue
		}
		kept = append(kept, link)
	}
	if !found {
		return ErrNoSuchWallLink
	}

	settings.WallLinks = kept
	_, err = s.store.Settings().Put(ctx, settings)
	return err
}

// AuthenticateWallLink resolves a token to the link that holds it.
//
// Every link is compared and the comparison is constant-time, so how long this
// takes does not depend on which one matched or how far down the list it was --
// the same reasoning AuthenticateToken carries, and for the same reason it can
// afford a scan: these are counted in single digits.
func (s *Service) AuthenticateWallLink(ctx context.Context, token string) (model.WallLink, error) {
	want := hashWallToken(token)
	if want == "" {
		return model.WallLink{}, ErrInvalidWallLink
	}

	settings, err := s.currentSettings(ctx)
	if err != nil {
		return model.WallLink{}, err
	}

	var found model.WallLink
	matched := false
	for _, link := range settings.WallLinks {
		if link.TokenHash == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(link.TokenHash), []byte(want)) == 1 {
			found, matched = link, true
		}
	}
	if !matched {
		return model.WallLink{}, ErrInvalidWallLink
	}

	s.noteWallLinkUse(ctx, settings, found)
	found.TokenHash = ""
	return found, nil
}

// wallUseThrottle is how stale LastUsedAt may be.
//
// A screen polls every ten seconds. Writing on every poll would put a store
// write in front of every refresh of every wall, for a field whose whole
// question is "is this still in use, and roughly since when". A minute answers
// that and is not a write amplifier -- the same number, for the same reason, as
// the service-account token's.
const wallUseThrottle = time.Minute

func (s *Service) noteWallLinkUse(ctx context.Context, settings model.Settings, link model.WallLink) {
	now := s.now().UTC()
	if now.Sub(link.LastUsedAt) < wallUseThrottle {
		return
	}
	for i := range settings.WallLinks {
		if settings.WallLinks[i].ID != link.ID {
			continue
		}
		settings.WallLinks[i].LastUsedAt = now
		// A failure here is deliberately swallowed: this is bookkeeping about a
		// request that has already been authenticated, and losing a race with
		// another settings write must not turn a working wall into a refusal.
		_, _ = s.store.Settings().Put(ctx, settings)
		return
	}
}

// currentSettings reads the singleton, treating "not there yet" as empty.
//
// The settings record is written by setup, and every caller here runs long
// after that -- but depending on a write somebody else did is how a feature
// breaks on the one installation whose setup predates the field. An absent
// singleton means nothing has been configured, which is exactly a settings
// record with no wall links in it.
func (s *Service) currentSettings(ctx context.Context) (model.Settings, error) {
	settings, err := s.store.Settings().Get(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return model.Settings{}, nil
	}
	if err != nil {
		return model.Settings{}, err
	}
	return settings, nil
}

func newWallToken() (token, hash string, err error) {
	var b [tokenBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", fmt.Errorf("auth: mint a wall link: %w", err)
	}
	token = WallTokenPrefix + base64.RawURLEncoding.EncodeToString(b[:])
	return token, hashWallToken(token), nil
}

// hashWallToken is what is stored in place of a token, and "" for anything that
// is not one.
//
// The prefix is checked rather than assumed: a service-account token arriving
// here must not be hashed and compared, because a match would mean a wall link
// whose hash somebody had set to a service token's -- and refusing it by shape
// costs nothing.
func hashWallToken(token string) string {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, WallTokenPrefix) || len(token) <= len(WallTokenPrefix) {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
