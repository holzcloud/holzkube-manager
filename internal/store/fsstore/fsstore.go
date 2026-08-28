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

	"github.com/holzcloud/holzkube/internal/model"
	"github.com/holzcloud/holzkube/internal/store"
	"github.com/holzcloud/holzkube/internal/store/migrate"
)

// Store is the filesystem-backed implementation of store.Store.
type Store struct {
	dir string

	// release drops the process flock. It is held for the lifetime of the
	// Store and dropped by Close.
	release func() error

	// entityMu is the middle layer of concurrency control: one mutex per
	// record, shared by every entity store so that "users/a" and "sessions/a"
	// are distinct keys.
	entityMu *store.EntityLocks

	users    *userStore
	settings *settingsStore
	sessions *sessionStore
}

// Open prepares dir as a holzkube data directory and returns a Store over it.
// The directory is created 0700 if missing.
//
// The order of the startup steps is load-bearing:
//
//	create dir -> process flock -> permission guard -> sweep temp files
//	-> schema migration -> entity directories
//
// The flock comes first because everything after it writes. The guard runs
// before the migration so that a directory with wrong permissions is refused
// before a backup tarball is written into it. The temp sweep runs before the
// migration because the migration reads state, and a half-written temp file
// left by a crash must never be read as state.
func Open(dir string) (s *Store, err error) {
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

	release, err := store.AcquireProcessLock(abs)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = release()
		}
	}()

	if err := Guard(abs); err != nil {
		return nil, err
	}

	if err := sweepTempFiles(abs); err != nil {
		return nil, err
	}

	if err := migrate.Run(abs); err != nil {
		return nil, err
	}

	s = &Store{dir: abs, release: release, entityMu: store.NewEntityLocks()}
	s.users = &userStore{dir: filepath.Join(abs, "users"), locks: s.entityMu}
	s.settings = &settingsStore{path: filepath.Join(abs, "settings.json"), locks: s.entityMu}
	s.sessions = &sessionStore{dir: filepath.Join(abs, "sessions"), locks: s.entityMu}

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

// Users returns the operator account entity.
func (s *Store) Users() store.UserStore { return s.users }

// Settings returns the instance settings entity.
func (s *Store) Settings() store.SettingsStore { return s.settings }

// Sessions returns the server-side session entity.
func (s *Store) Sessions() store.SessionStore { return s.sessions }

// Close releases the process lock. After Close another instance may open the
// same data directory.
func (s *Store) Close() error {
	if s.release == nil {
		return nil
	}
	release := s.release
	s.release = nil
	return release()
}

// Entity kinds. They are the first half of every per-entity lock key, which is
// what keeps users/a and sessions/a from contending.
const (
	kindUsers    = "users"
	kindSettings = "settings"
	kindSessions = "sessions"

	// settingsKey is the id half of the lock key for the settings singleton.
	settingsKey = "singleton"
)

// marshalRecord serializes a record deterministically.
//
// Determinism is not cosmetic here. A repeated Put of an unchanged record must
// produce byte-identical output, so that "the file changed" means the record
// changed; and a pre-migration backup must diff readably against the migrated
// directory. encoding/json emits struct fields in declaration order and sorts
// map keys, so fixing the indentation is the only thing left to pin.
func marshalRecord(v any) ([]byte, error) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

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
	locks *store.EntityLocks
	dir   string
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

	defer u.locks.LockEntity(kindUsers, string(rec.ID))()

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
	raw, err := marshalRecord(rec)
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
	defer u.locks.LockEntity(kindUsers, string(id))()
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
	locks *store.EntityLocks
	path  string
}

func (s *settingsStore) Get(_ context.Context) (model.Settings, error) {
	var rec model.Settings
	if err := readJSON(s.path, &rec); err != nil {
		return model.Settings{}, err
	}
	return rec, nil
}

func (s *settingsStore) Put(_ context.Context, rec model.Settings) (model.Settings, error) {
	defer s.locks.LockEntity(kindSettings, settingsKey)()

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
	raw, err := marshalRecord(rec)
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
	locks *store.EntityLocks
	dir   string
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
	defer s.locks.LockEntity(kindSessions, rec.ID)()

	var current model.Session
	if err := readJSON(p, &current); err == nil {
		rec.Rev = current.Rev
	} else if !errors.Is(err, store.ErrNotFound) {
		return model.Session{}, err
	}

	rec.Rev++
	raw, err := marshalRecord(rec)
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
	defer s.locks.LockEntity(kindSessions, id)()
	if err := removeAndSync(p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}
