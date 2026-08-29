// Package model holds the record types shared across holzkube.
//
// UserID, ClusterID and MachineID are distinct named types on purpose. Making
// them separate types costs nothing today and is the entire multi-cluster
// insurance policy: the compiler finds every place a scope was forgotten.
package model

import "time"

// UserID identifies an operator account.
type UserID string

// ClusterID identifies a Talos cluster. Nothing in phase 1 uses it yet; it
// exists so that later phases cannot silently omit cluster scoping.
type ClusterID string

// MachineID is a machine's UUID.
type MachineID string

// SchematicID identifies an Image Factory schematic.
//
// The value is the 64-character lowercase hex SHA-256 the Factory assigns, and
// that shape is why it can be a filename unchanged: it passes the store's key
// check without escaping, so nothing has to invent a mapping between an
// identifier and a path. It is a distinct type for the same reason UserID and
// ClusterID are -- the compiler finds the place someone passes the wrong one.
type SchematicID string

// User is an operator account. holzkube is a single-operator tool, but the
// record is shaped so that a future OIDC or multi-user layer has somewhere to
// go without a migration.
type User struct {
	ID           UserID    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	CreatedAt    time.Time `json:"created_at"`

	// Rev is the compare-and-swap revision. Every stored record carries one.
	Rev uint64 `json:"rev"`
}

// Settings is the singleton instance-wide settings record.
type Settings struct {
	SetupCompleted bool      `json:"setup_completed"`
	CreatedAt      time.Time `json:"created_at"`

	Rev uint64 `json:"rev"`
}

// Session is a server-side session record. The session payload is opaque to
// everything above the store: it is whatever the session manager encoded.
type Session struct {
	ID        string    `json:"id"`
	Data      []byte    `json:"data"`
	ExpiresAt time.Time `json:"expires_at"`

	Rev uint64 `json:"rev"`
}

// Schematic is a stored Image Factory schematic: what was asked for, what the
// Factory made of it, and whether it was ever proven to build.
//
// It follows model.User's shape -- snake_case JSON tags, a CreatedAt, and a
// trailing Rev -- because every stored record in holzkube does.
type Schematic struct {
	ID SchematicID `json:"id"`

	// Cluster is the cluster this schematic belongs to, empty when it is not
	// assigned to one. This is the first real user of ClusterID: a schematic
	// authored for one cluster must not silently be offered for another.
	Cluster ClusterID `json:"cluster"`

	// Name is the operator's own label. It never reaches a filesystem path --
	// the ID does.
	Name string `json:"name"`

	// TalosVersion is the version the schematic was authored and probed
	// against. It is part of the record rather than a query parameter because
	// the extension catalog is version-scoped: the same schematic may be
	// un-buildable at a different version.
	TalosVersion string `json:"talos_version"`

	// Canonical is the Factory's own normalised schematic document, stored
	// verbatim. It is the authoritative form (D-01): the id is the SHA-256 of
	// exactly these bytes, so storing the input instead would store something
	// that need not hash to the id it is filed under.
	Canonical string `json:"canonical"`

	// Extensions are the official system extension names baked in. Order is
	// preserved because the Factory preserves it, and it therefore changes the
	// id.
	Extensions []string `json:"extensions"`

	// KernelArgs are the extra kernel arguments. They reach the ISO and the
	// disk image and are ignored by the installer and initramfs, which is what
	// imagefactory.Warnings exists to say out loud.
	KernelArgs []string `json:"kernel_args"`

	// Meta are the META partition values. Same asymmetry as KernelArgs.
	Meta []MetaValue `json:"meta"`

	// Usable is false until the model-build probe confirms the schematic
	// actually builds for its Talos version and architecture.
	//
	// It is deliberately not set from a successful creation. The Factory
	// accepts a schematic naming an extension that does not exist, assigns it
	// an ordinary id, and only refuses when an image is asked for, so FACT-02
	// makes creation and validation two different events and this field
	// records the second one.
	Usable bool `json:"usable"`

	// ProbedAt is when the model-build probe last answered. A zero value means
	// it never has, which is not the same as "it answered no".
	ProbedAt time.Time `json:"probed_at"`

	CreatedAt time.Time `json:"created_at"`

	// Rev is the compare-and-swap revision. Every stored record carries one.
	Rev uint64 `json:"rev"`
}

// MetaValue is one META partition entry. Key is a uint8 because the META
// partition addresses its slots with a single byte.
type MetaValue struct {
	Key   uint8  `json:"key"`
	Value string `json:"value"`
}
