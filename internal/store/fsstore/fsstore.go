// Package fsstore implements store.Store over 0600 files on disk.
//
// It is the only package in holzkube permitted to know filesystem paths for
// state. Everything above it addresses records by entity and identifier.
package fsstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/holzcloud/holzkube/internal/model"
	"github.com/holzcloud/holzkube/internal/store"
)

// Store is the filesystem-backed implementation of store.Store.
type Store struct {
	dir string

	users    *userStore
	settings *settingsStore
	sessions *sessionStore
}

// Open prepares dir as a holzkube data directory and returns a Store over it.
// The directory is created 0700 if missing.
func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("fsstore: empty data directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("fsstore: resolve data directory: %w", err)
	}
	if err := os.MkdirAll(abs, dirPerm); err != nil {
		return nil, fmt.Errorf("fsstore: create data directory: %w", err)
	}

	s := &Store{dir: abs}
	s.users = &userStore{dir: filepath.Join(abs, "users")}
	s.settings = &settingsStore{path: filepath.Join(abs, "settings.json")}
	s.sessions = &sessionStore{dir: filepath.Join(abs, "sessions")}

	for _, sub := range []string{s.users.dir, s.sessions.dir} {
		if err := os.MkdirAll(sub, dirPerm); err != nil {
			return nil, fmt.Errorf("fsstore: create %s: %w", sub, err)
		}
	}
	return s, nil
}

// Dir reports the data directory. It is deliberately not part of store.Store:
// callers above the seam have no business with paths.
func (s *Store) Dir() string { return s.dir }

func (s *Store) Users() store.UserStore        { return s.users }
func (s *Store) Settings() store.SettingsStore { return s.settings }
func (s *Store) Sessions() store.SessionStore  { return s.sessions }

// Close releases the store. Phase 1 holds no long-lived handles; the process
// lock and per-entity lock map arrive in plan 02.
func (s *Store) Close() error { return nil }

// safeKey rejects any identifier that could escape its entity directory or
// collide with the temp-file prefix used by writeAtomic. Deriving a filename
// from user-controlled input without this check is a path traversal.
func safeKey(key string) error {
	if key == "" {
		return fmt.Errorf("%w: empty", store.ErrInvalidKey)
	}
	if len(key) > 128 {
		return fmt.Errorf("%w: too long", store.ErrInvalidKey)
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_':
		default:
			return fmt.Errorf("%w: %q contains %q", store.ErrInvalidKey, key, r)
		}
	}
	return nil
}

// readJSON loads a record. A missing file is store.ErrNotFound.
func readJSON(path string, out any) error {
	raw, err := os.ReadFile(path) //nolint:gosec // fsstore is the only package that may touch state paths
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store.ErrNotFound
		}
		return err
	}
	return json.Unmarshal(raw, out)
}

// listJSON loads every *.json record in dir into out via decode.
func listJSON(dir string, decode func(raw []byte) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // see readJSON
		if err != nil {
			return err
		}
		if err := decode(raw); err != nil {
			return err
		}
	}
	return nil
}

// --- users -----------------------------------------------------------------

type userStore struct {
	mu  sync.Mutex
	dir string
}

func (u *userStore) path(id model.UserID) (string, error) {
	if err := safeKey(string(id)); err != nil {
		return "", err
	}
	return filepath.Join(u.dir, string(id)+".json"), nil
}

func (u *userStore) Get(_ context.Context, id model.UserID) (model.User, error) {
	p, err := u.path(id)
	if err != nil {
		return model.User{}, err
	}
	var rec model.User
	if err := readJSON(p, &rec); err != nil {
		return model.User{}, err
	}
	return rec, nil
}

func (u *userStore) List(_ context.Context) ([]model.User, error) {
	out := make([]model.User, 0, 1)
	err := listJSON(u.dir, func(raw []byte) error {
		var rec model.User
		if err := json.Unmarshal(raw, &rec); err != nil {
			return err
		}
		out = append(out, rec)
		return nil
	})
	return out, err
}

func (u *userStore) Put(_ context.Context, rec model.User) (model.User, error) {
	p, err := u.path(rec.ID)
	if err != nil {
		return model.User{}, err
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	var current model.User
	switch err := readJSON(p, &current); {
	case err == nil:
		if rec.Rev != current.Rev {
			return model.User{}, fmt.Errorf("%w: user %s is at rev %d, put carried %d",
				store.ErrConflict, rec.ID, current.Rev, rec.Rev)
		}
	case errors.Is(err, store.ErrNotFound):
		if rec.Rev != 0 {
			return model.User{}, fmt.Errorf("%w: user %s does not exist but put carried rev %d",
				store.ErrConflict, rec.ID, rec.Rev)
		}
	default:
		return model.User{}, err
	}

	rec.Rev++
	raw, err := json.Marshal(rec)
	if err != nil {
		return model.User{}, err
	}
	if err := writeAtomic(p, raw); err != nil {
		return model.User{}, err
	}
	return rec, nil
}

func (u *userStore) Delete(_ context.Context, id model.UserID) error {
	p, err := u.path(id)
	if err != nil {
		return err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if err := removeAndSync(p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store.ErrNotFound
		}
		return err
	}
	return nil
}

// --- settings --------------------------------------------------------------

type settingsStore struct {
	mu   sync.Mutex
	path string
}

func (s *settingsStore) Get(_ context.Context) (model.Settings, error) {
	var rec model.Settings
	if err := readJSON(s.path, &rec); err != nil {
		return model.Settings{}, err
	}
	return rec, nil
}

func (s *settingsStore) Put(_ context.Context, rec model.Settings) (model.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var current model.Settings
	switch err := readJSON(s.path, &current); {
	case err == nil:
		if rec.Rev != current.Rev {
			return model.Settings{}, fmt.Errorf("%w: settings are at rev %d, put carried %d",
				store.ErrConflict, current.Rev, rec.Rev)
		}
	case errors.Is(err, store.ErrNotFound):
		if rec.Rev != 0 {
			return model.Settings{}, fmt.Errorf("%w: settings do not exist but put carried rev %d",
				store.ErrConflict, rec.Rev)
		}
	default:
		return model.Settings{}, err
	}

	rec.Rev++
	raw, err := json.Marshal(rec)
	if err != nil {
		return model.Settings{}, err
	}
	if err := writeAtomic(s.path, raw); err != nil {
		return model.Settings{}, err
	}
	return rec, nil
}

// --- sessions --------------------------------------------------------------

type sessionStore struct {
	mu  sync.Mutex
	dir string
}

func (s *sessionStore) path(id string) (string, error) {
	if err := safeKey(id); err != nil {
		return "", err
	}
	return filepath.Join(s.dir, id+".json"), nil
}

func (s *sessionStore) Get(_ context.Context, id string) (model.Session, error) {
	p, err := s.path(id)
	if err != nil {
		return model.Session{}, err
	}
	var rec model.Session
	if err := readJSON(p, &rec); err != nil {
		return model.Session{}, err
	}
	return rec, nil
}

func (s *sessionStore) List(_ context.Context) ([]model.Session, error) {
	out := make([]model.Session, 0, 4)
	err := listJSON(s.dir, func(raw []byte) error {
		var rec model.Session
		if err := json.Unmarshal(raw, &rec); err != nil {
			return err
		}
		out = append(out, rec)
		return nil
	})
	return out, err
}

// Put upserts a session. Sessions are rewritten on nearly every request, so
// they carry a revision for consistency with the rest of the store but do not
// enforce CAS: the session manager is the single writer per token, and a
// spurious 409 there would log the operator out for no reason.
func (s *sessionStore) Put(_ context.Context, rec model.Session) (model.Session, error) {
	p, err := s.path(rec.ID)
	if err != nil {
		return model.Session{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var current model.Session
	if err := readJSON(p, &current); err == nil {
		rec.Rev = current.Rev
	} else if !errors.Is(err, store.ErrNotFound) {
		return model.Session{}, err
	}

	rec.Rev++
	raw, err := json.Marshal(rec)
	if err != nil {
		return model.Session{}, err
	}
	if err := writeAtomic(p, raw); err != nil {
		return model.Session{}, err
	}
	return rec, nil
}

func (s *sessionStore) Delete(_ context.Context, id string) error {
	p, err := s.path(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := removeAndSync(p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}
