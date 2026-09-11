package talos_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// TestCheckSupportedVersion pins the window and its edges.
//
// The bounds are read from the exported constants rather than written out as
// literals, so widening the range stays a single edit and this test cannot
// disagree with the check about what the range is. What it does assert
// literally is the shape of the answer at each edge.
func TestCheckSupportedVersion(t *testing.T) {
	t.Parallel()

	for _, row := range []struct {
		name      string
		version   string
		supported bool
	}{
		{"the oldest supported minor", talos.MinSupportedVersion + ".0", true},
		{"the newest supported minor", talos.MaxSupportedVersion + ".3", true},
		{"the pinned machinery version", "v1.13.9", true},
		{"one minor below the window", "v1.11.9", false},
		{"one minor above the window", "v1.15.0", false},
		{"a major above the window", "v2.0.0", false},
		// Inside the *window*, and refused for the other reason unless opted
		// in -- see TestAPreReleaseIsOptIn, which pins both directions and the
		// separate error. This row runs with the opt-in on, because what it is
		// about is the range comparison ignoring the suffix.
		{"a release candidate inside the window", "v1.14.0-rc.2", true},
		{"a release candidate above the window", "v1.15.0-rc.1", false},
		{"no version at all", "", false},
		{"not a version", "unknown", false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			err := talos.CheckSupportedVersion(row.version, true)
			if row.supported {
				if err != nil {
					t.Fatalf("CheckSupportedVersion(%q) = %v, want nil", row.version, err)
				}
				return
			}

			if err == nil {
				t.Fatalf("CheckSupportedVersion(%q) = nil, want a refusal", row.version)
			}
			if !errors.Is(err, talos.ErrUnsupportedVersion) {
				t.Errorf("error %v does not satisfy errors.Is(err, ErrUnsupportedVersion)", err)
			}
			// Both halves of the reason, every time: an operator holding a node
			// that will not connect has to be able to tell whether to move the
			// node or to move holzkube-manager.
			for _, want := range []string{talos.MinSupportedVersion, talos.MaxSupportedVersion} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name the supported range bound %q", err, want)
				}
			}
			if row.version != "" && !strings.Contains(err.Error(), row.version) {
				t.Errorf("error %q does not name the observed version %q", err, row.version)
			}
		})
	}
}

// TestAPreReleaseIsOptIn is OPS-03's second half.
//
// A pre-release inside the supported window is refused by default and accepted
// when the operator has said so. The two directions matter equally: refusing
// one somebody deliberately installed, with no way to proceed, would be a
// product that cannot be used to run what its own compatibility table calls
// the next release.
func TestAPreReleaseIsOptIn(t *testing.T) {
	t.Parallel()

	for _, version := range []string{"v1.14.0-rc.2", "v1.13.0-beta.1", "v1.14.0-alpha.0"} {
		if err := talos.CheckSupportedVersion(version, false); !errors.Is(err, talos.ErrPreRelease) {
			t.Errorf("CheckSupportedVersion(%q, false) = %v, want ErrPreRelease", version, err)
		}
		if err := talos.CheckSupportedVersion(version, true); err != nil {
			t.Errorf("CheckSupportedVersion(%q, true) = %v, want nil", version, err)
		}
	}
}

// TestAPreReleaseRefusalIsNotAnUnsupportedVersionRefusal.
//
// The remedies are opposite kinds of thing -- change the node, or change a
// setting -- and a client that showed them as the same refusal would send
// somebody to reinstall a node they deliberately put a release candidate on.
func TestAPreReleaseRefusalIsNotAnUnsupportedVersionRefusal(t *testing.T) {
	t.Parallel()

	err := talos.CheckSupportedVersion("v1.14.0-rc.2", false)
	if errors.Is(err, talos.ErrUnsupportedVersion) {
		t.Error("a pre-release inside the window satisfies errors.Is(err, ErrUnsupportedVersion); " +
			"it is inside the window, and the reason it was refused is a setting")
	}
	if !strings.Contains(err.Error(), "--allow-prerelease") {
		t.Errorf("the refusal %q does not say how to proceed", err)
	}
}

// TestBuildMetadataIsNotAPreRelease.
//
// v1.14.0+dirty is a release built from a modified tree. Refusing it as a
// pre-release would refuse a node running a locally built Talos for a reason
// that is not true of it.
func TestBuildMetadataIsNotAPreRelease(t *testing.T) {
	t.Parallel()

	if talos.IsPreRelease("v1.14.0+dirty") {
		t.Error("build metadata was read as a pre-release")
	}
	if err := talos.CheckSupportedVersion("v1.14.0+dirty", false); err != nil {
		t.Errorf("a release with build metadata was refused: %v", err)
	}
}

// TestInSupportedRangeAnswersWithoutRefusing is what the screen reads.
//
// A node outside the range is marked whether or not anything has tried to
// connect to it -- a marking that depended on a failed connection would be
// missing exactly when the node is down for an unrelated reason.
func TestInSupportedRangeAnswersWithoutRefusing(t *testing.T) {
	t.Parallel()

	for version, want := range map[string]bool{
		"v1.13.9":      true,
		"v1.14.0-rc.2": true, // inside the window; the pre-release question is separate
		"v1.11.9":      false,
		"v1.15.0":      false,
		"":             false,
		"unknown":      false,
	} {
		if got := talos.InSupportedRange(version); got != want {
			t.Errorf("InSupportedRange(%q) = %v, want %v", version, got, want)
		}
	}
}
