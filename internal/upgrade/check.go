package upgrade

import (
	"context"
	"fmt"
	"sort"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// UpdateCheck answers one question for one cluster: is there a newer Talos
// than the cluster runs, and what is the next run?
//
// It starts nothing and reads no node. The versions are the ones the
// inventory last observed, and the releases are the ones this installation
// can install and read -- the Image Factory's stable list, cut to the range
// this build supports -- so "available" never offers a version the product
// cannot upgrade to. A newer upstream release beyond that range is a
// different finding, and the weekly upstream check is what raises it.
type UpdateCheck struct {
	Cluster model.ClusterID `json:"cluster"`
	Name    string          `json:"name"`

	// Current is every distinct Talos version the cluster's nodes report, oldest
	// first. More than one means a rolling upgrade was interrupted or a node
	// was added on another version.
	Current []string `json:"current"`

	// Newest is the newest stable release this installation can upgrade to.
	Newest string `json:"newest,omitempty"`

	// Available is whether at least one node runs something older than Newest.
	Available bool `json:"available"`

	// Next is the version the next run installs. It differs from Newest when
	// the cluster is more than one minor behind: Talos upgrades one minor at a
	// time, and the run after this one is a decision made after this one.
	Next string `json:"next,omitempty"`

	// Runs is how many runs lead to Newest.
	Runs int `json:"runs,omitempty"`

	// NotesURL is the release notes of Next, which Talos asks to be read
	// before an upgrade.
	NotesURL string `json:"notes_url,omitempty"`

	// Reason says why there is nothing to offer when that is not simply "up to
	// date": no version known yet, no release source, no path.
	Reason string `json:"reason,omitempty"`
}

// talosReleaseNotes is where a Talos release's notes live.
const talosReleaseNotes = "https://github.com/siderolabs/talos/releases/tag/"

// CheckTalos compares what a cluster runs with what can be installed.
func (s *Service) CheckTalos(ctx context.Context, cluster model.ClusterID, name string) (UpdateCheck, error) {
	out := UpdateCheck{Cluster: cluster, Name: name, Current: []string{}}

	machines, err := s.deps.Machines(ctx, cluster)
	if err != nil {
		return out, err
	}

	seen := map[Version]bool{}
	var versions []Version
	for _, m := range machines {
		v, perr := ParseVersion(m.Snapshot.TalosVersion)
		if perr != nil || seen[v] {
			continue
		}
		seen[v] = true
		versions = append(versions, v)
	}
	if len(versions) == 0 {
		out.Reason = "No node of this cluster has reported its Talos version yet."
		return out, nil
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].Less(versions[j]) })
	for _, v := range versions {
		out.Current = append(out.Current, v.String())
	}
	lowest := versions[0]

	available, err := s.Releases(ctx)
	if err != nil {
		return out, err
	}
	if len(available) == 0 {
		out.Reason = "The Image Factory listed no stable Talos release this installation supports."
		return out, nil
	}
	newest := available[0]
	out.Newest = newest.String()

	if !lowest.Less(newest) {
		out.Reason = "Every node already runs the newest release this installation supports."
		return out, nil
	}

	chain, err := Chain(lowest, newest, available)
	if err != nil {
		out.Reason = err.Error()
		return out, nil
	}
	if len(chain) == 0 {
		return out, nil
	}
	out.Available = true
	out.Runs = len(chain)
	out.Next = chain[0].To.String()
	out.NotesURL = talosReleaseNotes + out.Next
	return out, nil
}

// String renders the check as one sentence, for logs and audit detail.
func (u UpdateCheck) String() string {
	if !u.Available {
		return fmt.Sprintf("%s: no Talos update (%s)", u.Name, u.Reason)
	}
	return fmt.Sprintf("%s: Talos %s is available (next run %s of %d)", u.Name, u.Newest, u.Next, u.Runs)
}
