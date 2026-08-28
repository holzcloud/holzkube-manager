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
