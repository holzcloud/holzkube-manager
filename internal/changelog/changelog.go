// Package changelog is what the version number in the sidebar opens.
//
// It is a curated file and not a rendering of git history, and that is the
// whole point of it. The commit log of this repository is written for whoever
// has to change the code next; it says which ordering was wrong and which
// guard went red. An operator opening "what's new" is asking a different
// question -- what is different about the thing I am running -- and answering
// it with a commit list answers neither well.
//
// The file is embedded rather than read from disk at runtime. A release is one
// binary, and a changelog that could be edited beside it is a changelog that
// can disagree with the build it claims to describe.
package changelog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed changelog.json
var raw []byte

// Change is one thing an operator would notice.
//
// Icon is decoration and carries no meaning the text does not: a reader with
// no emoji font, or a screen reader, loses nothing. That is why nothing
// branches on it.
type Change struct {
	Icon string `json:"icon"`
	Text string `json:"text"`
}

// Release is one published version.
//
// Version is spelled exactly as the git tag is, leading v included, because
// that is the string the release pipeline checks this file against before it
// creates the tag. Two spellings of one version is the drift that check exists
// to make impossible.
type Release struct {
	Version string   `json:"version"`
	Date    string   `json:"date"`
	Changes []Change `json:"changes"`
}

// Series is the version's first two components -- "v1.16" for "v1.16.0-beta.2"
// -- which is how the releases are grouped for reading. A patch and a beta of
// one minor belong under one heading; a reader looking for what changed does
// not want eleven tabs.
func (r Release) Series() string {
	parts := strings.SplitN(strings.TrimPrefix(r.Version, "v"), ".", 3)
	if len(parts) < 2 {
		return r.Version
	}
	return "v" + parts[0] + "." + parts[1]
}

var releases []Release

func init() {
	// A malformed changelog is a build that cannot start, on purpose. The
	// alternative is a binary that serves an empty list, and an empty "what's
	// new" reads as "nothing changed" rather than as "this file is broken" --
	// which is the failure this package would be least likely to notice.
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&releases); err != nil {
		panic(fmt.Sprintf("changelog: embedded changelog.json is not readable: %v", err))
	}
}

// Releases returns the published releases, newest first.
//
// A copy, because the slice is package state and a caller that sorted or
// truncated it in place would change what every later caller sees.
func Releases() []Release {
	out := make([]Release, len(releases))
	copy(out, releases)
	return out
}

// Newest is the release this file claims to be current, or the zero value when
// the file is empty.
func Newest() Release {
	if len(releases) == 0 {
		return Release{}
	}
	return releases[0]
}
