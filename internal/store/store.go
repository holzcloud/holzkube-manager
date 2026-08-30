// Package store defines holzkube-manager's persistence seam.
//
// The interface is deliberately entity-shaped and never path-shaped:
// store.Users().Get(ctx, id), never store.ReadFile("users/x.json"). No method
// here accepts or returns a filesystem path. That is what makes the eventual
// fsstore -> sqlitestore swap a single new implementation with zero changes
// above. Any os.ReadFile outside internal/store/fsstore is an architecture bug.
package store

import (
	"context"
	"errors"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

var (
	// ErrNotFound is returned when a record does not exist.
	ErrNotFound = errors.New("store: record not found")

	// ErrConflict is returned when a Put carries a Rev that is not the stored
	// one. Callers map it to 409 Conflict.
	ErrConflict = errors.New("store: revision conflict")

	// ErrInvalidKey is returned for a key that cannot address a record.
	ErrInvalidKey = errors.New("store: invalid key")

	// ErrAlreadyRunning is returned when the data directory is already held by
	// another process. It is a refusal to start, not a retryable condition:
	// two instances on one directory is a corruption path that no in-process
	// locking can detect.
	ErrAlreadyRunning = errors.New("store: data directory already in use by another process")
)

// TempFilePrefix marks a record that is being written and is not yet in place.
//
// The prefix is part of the crash contract, not an implementation detail: a
// process killed between writing the temporary file and renaming it leaves one
// behind, and the next start must recognise and remove it rather than ever
// read it as state. It lives here because everything that creates such a file
// (fsstore, migrate, tlsx) and everything that must recognise one (the startup
// sweeper, the backup exclusion) needs the same string. A package that invents
// its own prefix writes files nothing sweeps -- which for tlsx meant orphaned
// private keys.
const TempFilePrefix = ".holzkube-manager-tmp-"

// Store is the root of the persistence seam.
type Store interface {
	Users() UserStore
	Settings() SettingsStore
	Sessions() SessionStore
	Schematics() SchematicStore
	Close() error
}

// UserStore holds operator accounts.
type UserStore interface {
	Get(ctx context.Context, id model.UserID) (model.User, error)
	List(ctx context.Context) ([]model.User, error)
	Put(ctx context.Context, rec model.User) (model.User, error)
	Delete(ctx context.Context, id model.UserID) error
}

// SettingsStore holds the singleton settings record. It has no List or Delete
// because there is exactly one, always: a singleton with a List method is an
// invitation to write code that assumes otherwise.
type SettingsStore interface {
	Get(ctx context.Context) (model.Settings, error)
	Put(ctx context.Context, rec model.Settings) (model.Settings, error)
}

// SchematicStore holds Image Factory schematics (D-09).
//
// Schematics are persisted through this seam rather than as an ad-hoc file or
// a column bolted onto something else, so that reshaping the record later costs
// a migration and not an edit. It carries the same fixed method set as
// UserStore for the same reason: what it must not become is a query interface.
// The moment a caller can ask for "the schematics for cluster X, usable only,
// newest first", the filter lives in the store and the swap to another backend
// stops being a single new implementation.
type SchematicStore interface {
	Get(ctx context.Context, id model.SchematicID) (model.Schematic, error)
	List(ctx context.Context) ([]model.Schematic, error)
	Put(ctx context.Context, rec model.Schematic) (model.Schematic, error)
	Delete(ctx context.Context, id model.SchematicID) error
}

// SessionStore holds server-side sessions. It backs the session manager, so
// that session state travels through the same seam as everything else rather
// than reaching around it to the filesystem.
type SessionStore interface {
	Get(ctx context.Context, id string) (model.Session, error)
	List(ctx context.Context) ([]model.Session, error)
	Put(ctx context.Context, rec model.Session) (model.Session, error)
	Delete(ctx context.Context, id string) error
}
