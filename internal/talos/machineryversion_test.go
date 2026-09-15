package talos_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/siderolabs/talos/pkg/machinery/gendata"

	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// The supported range and the library that has to understand it, held against
// each other.
//
// They came apart, and the operator paid for it. MaxSupportedVersion said
// "v1.14" while go.mod pinned machinery v1.13.9, so this product advertised a
// Talos release whose configuration documents its own library did not know.
// A real v1.14 cluster then refused adoption with "not registered" for a
// document kind that machinery v1.14 has and v1.13.9 does not.
//
// Nothing could have caught that. The range is two string constants and the pin
// is a line in go.mod; no test compared them, because there was nothing to
// compare them with until gendata.VersionTag — which is the version of the
// machinery actually compiled in, not a second copy of a number.
//
// The claim is deliberately one-directional. The pin must be at least as new as
// the newest release advertised; it may be newer, because machinery v1.14 reads
// a v1.12 configuration perfectly well and nothing is served by forbidding that.
func TestThePinnedMachineryKnowsTheNewestSupportedTalos(t *testing.T) {
	t.Parallel()

	pinned := gendata.VersionTag
	pinnedMajor, pinnedMinor := majorMinor(t, pinned)
	maxMajor, maxMinor := majorMinor(t, talos.MaxSupportedVersion)

	if pinnedMajor < maxMajor || (pinnedMajor == maxMajor && pinnedMinor < maxMinor) {
		t.Errorf("MaxSupportedVersion is %s and the machinery compiled in is %s.\n\n"+
			"This product would advertise support for a Talos whose configuration its own "+
			"library cannot read. That is not hypothetical: it shipped, and a real cluster "+
			"refused adoption because a document kind of the newer Talos was \"not registered\" "+
			"in the older machinery.\n\n"+
			"Either raise the machinery pin in go.mod, or lower MaxSupportedVersion to what "+
			"the pin can actually read.",
			talos.MaxSupportedVersion, pinned)
	}
}

// TestTheSupportedRangeIsNotEmpty keeps the two constants in an order that
// means something, so the test above cannot be satisfied by a range nobody
// could be inside.
func TestTheSupportedRangeIsNotEmpty(t *testing.T) {
	t.Parallel()

	minMajor, minMinor := majorMinor(t, talos.MinSupportedVersion)
	maxMajor, maxMinor := majorMinor(t, talos.MaxSupportedVersion)

	if minMajor > maxMajor || (minMajor == maxMajor && minMinor > maxMinor) {
		t.Errorf("the supported range is %s to %s, which contains nothing",
			talos.MinSupportedVersion, talos.MaxSupportedVersion)
	}
}

// majorMinor reads the first two components of a version, leading v and any
// suffix ignored. gendata.VersionTag is a full tag ("v1.14.0"); the range
// constants are minors ("v1.14"); both reduce to the same pair.
func majorMinor(t *testing.T, version string) (int, int) {
	t.Helper()

	parts := strings.SplitN(strings.TrimPrefix(version, "v"), ".", 3)
	if len(parts) < 2 {
		t.Fatalf("%q is not a version this test can read", version)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		t.Fatalf("major of %q: %v", version, err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("minor of %q: %v", version, err)
	}
	return major, minor
}
