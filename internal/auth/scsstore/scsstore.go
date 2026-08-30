// Package scsstore adapts holzkube-manager's session entity onto the scs session
// manager's Store interface.
//
// Its whole reason to exist is FOUND-07: session state is state, so it travels
// through store.Sessions() like everything else instead of reaching around the
// seam to the filesystem.
package scsstore

import (
	"context"
	"errors"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// Store implements scs.Store and scs.IterableStore over a store.SessionStore.
type Store struct {
	sessions store.SessionStore
}

// New wraps a session entity store.
func New(sessions store.SessionStore) *Store {
	return &Store{sessions: sessions}
}

// Find returns the session payload for token, treating an expired session as
// absent and deleting it on the way out.
func (s *Store) Find(token string) ([]byte, bool, error) {
	rec, err := s.sessions.Get(context.Background(), token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrInvalidKey) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !time.Now().Before(rec.ExpiresAt) {
		if err := s.sessions.Delete(context.Background(), token); err != nil {
			return nil, false, err
		}
		return nil, false, nil
	}
	return rec.Data, true, nil
}

// Commit stores the session payload under token.
func (s *Store) Commit(token string, b []byte, expiry time.Time) error {
	_, err := s.sessions.Put(context.Background(), model.Session{
		ID:        token,
		Data:      b,
		ExpiresAt: expiry,
	})
	return err
}

// Delete removes a session. Deleting an absent session is not an error: the
// session manager calls it on logout paths that may race a natural expiry.
func (s *Store) Delete(token string) error {
	err := s.sessions.Delete(context.Background(), token)
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrInvalidKey) {
		return nil
	}
	return err
}

// All returns every live session payload, keyed by token.
func (s *Store) All() (map[string][]byte, error) {
	recs, err := s.sessions.List(context.Background())
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make(map[string][]byte, len(recs))
	for _, rec := range recs {
		if !now.Before(rec.ExpiresAt) {
			continue
		}
		out[rec.ID] = rec.Data
	}
	return out, nil
}
