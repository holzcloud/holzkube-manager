package model

import "time"

// PatchID identifies a reusable configuration patch.
type PatchID string

// Patch is a reusable configuration change.
//
// It is **strategic merge only**, and that is a decision rather than a
// limitation. RFC 6902 JSON Patch addresses list elements by index -- "replace
// /machine/certSANs/2" -- and an index is a statement about a list as it
// happened to be when the patch was written. Applied to a node whose list is
// one element longer, it edits the wrong entry and reports success. Strategic
// merge addresses by key, which is what makes the same patch mean the same
// thing on two nodes that are not identical.
//
// The store is append-only: applying a patch never rewrites its record, and
// editing one produces a new version rather than changing the old. What that
// buys is the ability to answer "what exactly was applied to this node in
// March", which is a question that only has an answer if the thing applied
// still exists.
type Patch struct {
	ID PatchID `json:"id"`

	// Name is the operator's label. It never reaches a filesystem path.
	Name string `json:"name"`

	// Cluster scopes a patch, empty for one that applies anywhere. A patch
	// authored against one cluster's addressing is rarely right on another.
	Cluster ClusterID `json:"cluster,omitempty"`

	// Version counts from 1 and increases with every edit. The record is never
	// rewritten in place: an edit stores a new Patch whose Parent is this one.
	Version int `json:"version"`

	// Parent is the id of the version this one was edited from, empty for the
	// first.
	Parent PatchID `json:"parent,omitempty"`

	// Superseded marks a version an edit replaced. It stays readable -- that
	// is the point of append-only -- and is not offered for new applications.
	Superseded bool `json:"superseded,omitempty"`

	// Body is the strategic merge patch, as YAML.
	Body string `json:"body"`

	// Description is why this patch exists, in the author's words.
	Description string `json:"description,omitempty"`

	Author    string    `json:"author,omitempty"`
	CreatedAt time.Time `json:"created_at"`

	Rev uint64 `json:"rev"`
}
