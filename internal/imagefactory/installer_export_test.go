package imagefactory

import (
	"context"
	"net/http"
	"time"
)

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

// HTTPClientTimeoutForTest exposes the embedded http.Client's Timeout.
//
// It exists so the removal of the client-wide timeout can be asserted
// behaviourally rather than by grepping for a string that is absent. A grep
// proves the field is not assigned in the file it greps; this proves the value
// the client actually runs with, including through WithHTTPClient, which
// shallow-copies a client a caller may have set a Timeout on.
func (c *Client) HTTPClientTimeoutForTest() time.Duration { return c.http.Timeout }

// ProbeStatusUnbudgetedForTest calls probeStatus with a class the client has no
// budget for, which is the one way to reach a request built on a context with
// no deadline.
//
// Every production path derives its deadline through withBudget, so the refusal
// requireDeadline exists for is unreachable from outside the package -- which
// is the property being asserted, and is also why asserting it needs a door
// like this one. The class value is deliberately not one of the three: it
// stands for the next class somebody adds and forgets to give a budget.
func (c *Client) ProbeStatusUnbudgetedForTest(ctx context.Context, u string) (int, error) {
	return c.probeStatus(ctx, http.MethodHead, u, nil, budgetClass(0))
}

// DoUnbudgetedForTest reaches the other of the two call sites that touch
// http.Client.Do, on a context with no deadline.
func (c *Client) DoUnbudgetedForTest(ctx context.Context, u string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	var dst any
	return c.do(req, &dst)
}

// RegistryReason exposes the mapping from a transport failure to the sentence
// an operator reads.
//
// It is exported to the tests rather than tested through InstallerImage because
// going through the client for each case would need a fake registry that can
// fail in five different transport-level ways -- a test about the fake. The
// full path is still covered: TestInstallerImageFallsBackToTheLegacyRepository
// reads the sentence out of a real warning.
func RegistryReason(err error) string { return registryReason(err) }
