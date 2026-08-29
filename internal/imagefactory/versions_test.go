package imagefactory_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube/internal/imagefactory"
)

// recordedVersions is the 108-entry list captured from factory.talos.dev,
// prerelease tail included. It is the fixture for every filtering test, because
// the trap PITFALLS P9(d) records only exists in a list that ends in
// prereleases -- a hand-written fixture of three stable versions would let
// "latest = last element" pass.
func recordedVersions(t *testing.T) []string {
	t.Helper()
	var versions []string
	if err := json.Unmarshal(readTestdata(t, "versions.json"), &versions); err != nil {
		t.Fatalf("decode recorded versions: %v", err)
	}
	if len(versions) < 100 {
		t.Fatalf("the recorded version list has %d entries; it is not the captured one", len(versions))
	}
	return versions
}

// TestVersionsIsPrereleaseReadsTheSemverComponent pins the structural decision.
// No table, no curated data: the answer is in the version string itself, which
// is why D-08 says prerelease filtering needs no list.
func TestVersionsIsPrereleaseReadsTheSemverComponent(t *testing.T) {
	cases := map[string]bool{
		"v1.13.9":         false,
		"v1.12.0":         false,
		"v1.14.0-alpha.0": true,
		"v1.14.0-beta.1":  true,
		"v1.14.0-rc.2":    true,
		"v1.14.0":         false,

		// Build metadata is not a prerelease marker. A version carrying only
		// build metadata is a release.
		"v1.13.9+deadbeef": false,
		"v1.14.0-rc.1+abc": true,
	}

	for version, want := range cases {
		t.Run(version, func(t *testing.T) {
			if got := imagefactory.IsPrerelease(version); got != want {
				t.Errorf("IsPrerelease(%q) = %v, want %v", version, got, want)
			}
		})
	}
}

// TestVersionsSplitSeparatesTheWholePrereleaseTail checks the split against the
// recorded list: every -alpha, -beta and -rc entry lands in the prerelease
// bucket and none of them in the stable one.
func TestVersionsSplitSeparatesTheWholePrereleaseTail(t *testing.T) {
	all := recordedVersions(t)
	stable, prerelease := imagefactory.SplitVersions(all)

	if len(stable)+len(prerelease) != len(all) {
		t.Fatalf("the split dropped entries: %d + %d != %d", len(stable), len(prerelease), len(all))
	}
	if len(prerelease) == 0 {
		t.Fatal("no prereleases found in a list that ends in six of them")
	}

	for _, v := range stable {
		if strings.Contains(v, "-") {
			t.Errorf("%q is in the stable bucket", v)
		}
	}
	for _, v := range all {
		if !strings.Contains(v, "-") {
			continue
		}
		if !slices.Contains(prerelease, v) {
			t.Errorf("%q is a prerelease and is not in the prerelease bucket", v)
		}
	}
}

// TestVersionsSplitReturnsEmptySlicesNotNil keeps a JSON encoding of "no
// prereleases" as [] rather than null. A null reads to a client as "the server
// did not check".
func TestVersionsSplitReturnsEmptySlicesNotNil(t *testing.T) {
	stable, prerelease := imagefactory.SplitVersions(nil)
	if stable == nil {
		t.Error("the stable bucket is nil")
	}
	if prerelease == nil {
		t.Error("the prerelease bucket is nil")
	}
	if len(stable) != 0 || len(prerelease) != 0 {
		t.Errorf("an empty input produced %d stable and %d prerelease entries", len(stable), len(prerelease))
	}
}

// TestVersionsNewestStableIsNotTheLastElement is the P9(d) trap in one
// assertion. The recorded list ends in v1.14.0-rc.2, so an implementation that
// takes the last element offers an operator a release candidate.
func TestVersionsNewestStableIsNotTheLastElement(t *testing.T) {
	all := recordedVersions(t)

	got, err := imagefactory.NewestStable(all)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v1.13.9" {
		t.Errorf("NewestStable = %q, want v1.13.9", got)
	}
	if got == all[len(all)-1] {
		t.Error("NewestStable returned the last element, which is a prerelease")
	}
}

// TestVersionsNewestStableComparesRatherThanSorts covers a list that is not in
// ascending order. Upstream happens to serve one that is; nothing promises it
// will keep doing so, and "the list was sorted" is not a property this package
// can check.
func TestVersionsNewestStableComparesRatherThanSorts(t *testing.T) {
	shuffled := []string{"v1.9.5", "v1.14.0-rc.2", "v1.13.9", "v1.2.0", "v1.13.10", "v1.12.1"}

	got, err := imagefactory.NewestStable(shuffled)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// v1.13.10 beats v1.13.9 numerically and loses to it lexically, which is
	// the whole reason this is a semver comparison and not a string sort.
	if got != "v1.13.10" {
		t.Errorf("NewestStable = %q, want v1.13.10", got)
	}
}

// TestVersionsNewestStableRefusesWhenThereIsNone returns an error rather than
// the newest prerelease. Silently promoting a release candidate is exactly the
// behaviour FACT-05 exists to prevent.
func TestVersionsNewestStableRefusesWhenThereIsNone(t *testing.T) {
	for _, input := range [][]string{
		nil,
		{},
		{"v1.14.0-rc.1", "v1.14.0-rc.2"},
		{"not-a-version"},
	} {
		got, err := imagefactory.NewestStable(input)
		if err == nil {
			t.Errorf("NewestStable(%v) = %q with no error", input, got)
		}
		if got != "" {
			t.Errorf("NewestStable(%v) refused and still returned %q", input, got)
		}
	}
}

// TestVersionsSplitNeverPromotesAnUnparsableEntry pins the fail-safe direction:
// an entry this package cannot read is not stable just because it also failed
// to look like a prerelease.
func TestVersionsSplitNeverPromotesAnUnparsableEntry(t *testing.T) {
	stable, prerelease := imagefactory.SplitVersions([]string{"v1.13.9", "latest", "", "v1.14.0-rc.2"})

	if slices.Contains(stable, "latest") || slices.Contains(stable, "") {
		t.Errorf("an unparsable entry was offered as stable: %v", stable)
	}
	if !slices.Contains(stable, "v1.13.9") {
		t.Errorf("the one stable version is missing: %v", stable)
	}
	if len(stable)+len(prerelease) != 4 {
		t.Errorf("the split dropped an entry: %v / %v", stable, prerelease)
	}
}

// TestBrokenReasonAnswersFalseForAnUnlistedVersion is the external half of the
// lookup. The table's contents are guarded internally, where the table is
// visible.
func TestBrokenReasonAnswersFalseForAnUnlistedVersion(t *testing.T) {
	reason, broken := imagefactory.BrokenReason("v1.13.9")
	if broken {
		t.Errorf("v1.13.9 is listed as broken: %q", reason)
	}
	if reason != "" {
		t.Errorf("an unlisted version carried the reason %q", reason)
	}
}
