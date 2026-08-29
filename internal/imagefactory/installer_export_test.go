package imagefactory

import "time"

// This file exists only in the test binary. It exposes the installer-repository
// cache entry -- an unexported struct with unexported fields -- to the external
// imagefactory_test package, which is where the fake Factory lives.
//
// The alternative was to put the assertions in an internal test file, which
// cannot reach newFakeFactory, or to duplicate the fake inside the package,
// which would give the re-question cases a second fake to drift against. An
// accessor that only the test binary can see is the smaller of the three costs.

// InstallerRepoEntryForTest is a snapshot of one cache entry's provenance.
type InstallerRepoEntryForTest struct {
	// Repo is the repository name the entry serves.
	Repo string

	// WarningCode is empty when the entry is proven.
	WarningCode string

	// Unresolved is the candidate names the entry has never ruled out. Empty
	// means proven, and a proven entry never expires.
	Unresolved []string

	// WrittenAt is when the entry was last written or re-stamped. It is what
	// the re-question cadence is measured from.
	WrittenAt time.Time
}

// InstallerRepoEntryForTest returns the cached entry for r, if there is one.
func (c *Client) InstallerRepoEntryForTest(r AssetRequest) (InstallerRepoEntryForTest, bool) {
	c.installerMu.Lock()
	defer c.installerMu.Unlock()

	entry, ok := c.installerRepos[installerRepoKey(r)]
	if !ok {
		return InstallerRepoEntryForTest{}, false
	}
	return InstallerRepoEntryForTest{
		Repo:        entry.repo,
		WarningCode: entry.warning.Code,
		Unresolved:  entry.unresolved,
		WrittenAt:   entry.at,
	}, true
}
