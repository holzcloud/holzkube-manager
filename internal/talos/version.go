package talos

// The supported Talos version range.
//
// The range is a single pair of constants referenced by both the check and its
// tests, so widening or narrowing it is one edit rather than a search. It is a
// product decision recorded in the roadmap -- supported range v1.12 to v1.14,
// release candidates opt-in -- and not a machinery capability: the client
// library will happily talk to a node outside the range, which is exactly the
// problem. An untested API surface that answers is worse than one that
// refuses, because the divergence surfaces later, on a cluster.

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	// MinSupportedVersion is the oldest Talos release holzkube-manager is tested
	// against, as a major.minor tag.
	MinSupportedVersion = "v1.12"

	// MaxSupportedVersion is the newest.
	MaxSupportedVersion = "v1.14"
)

// ErrUnsupportedVersion reports a node running a Talos version outside the
// supported range.
//
// It is a refusal and never retryable: the node's version is a property of its
// current boot, so repeating the call changes nothing until it is upgraded or
// downgraded.
var ErrUnsupportedVersion = errors.New("talos: unsupported Talos version")

// CheckSupportedVersion reports whether a node's reported version tag is one
// holzkube-manager supports.
//
// The comparison is major.minor only and is implemented here rather than by
// importing golang.org/x/mod/semver, for the reason plan 02-04 gives for the
// same choice: the whole comparison is two integers against two integers, and
// a module added for that is a module in the graph forever. The prerelease
// component is stripped before comparing, so v1.14.0-rc.2 is inside the range
// -- opting into a release candidate is a separate decision from being inside
// the supported window, and conflating them would refuse a node the operator
// deliberately chose.
func CheckSupportedVersion(version string) error {
	major, minor, err := parseMajorMinor(version)
	if err != nil {
		return fmt.Errorf("%w: cannot read a version from %q; holzkube-manager supports %s to %s",
			ErrUnsupportedVersion, version, MinSupportedVersion, MaxSupportedVersion)
	}

	// The bounds are parsed rather than hard-coded as integers so that the
	// constants above are the single place the range is written down.
	minMajor, minMinor, err := parseMajorMinor(MinSupportedVersion)
	if err != nil {
		return fmt.Errorf("talos: MinSupportedVersion %q is not a version: %w", MinSupportedVersion, err)
	}
	maxMajor, maxMinor, err := parseMajorMinor(MaxSupportedVersion)
	if err != nil {
		return fmt.Errorf("talos: MaxSupportedVersion %q is not a version: %w", MaxSupportedVersion, err)
	}

	if before(major, minor, minMajor, minMinor) || before(maxMajor, maxMinor, major, minor) {
		return fmt.Errorf("%w: the node reports %s and holzkube-manager supports %s to %s",
			ErrUnsupportedVersion, version, MinSupportedVersion, MaxSupportedVersion)
	}
	return nil
}

// before reports whether a.b sorts before c.d.
func before(a, b, c, d int) bool {
	if a != c {
		return a < c
	}
	return b < d
}

// parseMajorMinor reads the major and minor components of a Talos version tag.
func parseMajorMinor(version string) (int, int, error) {
	v := strings.TrimSpace(version)
	v = strings.TrimPrefix(v, "v")

	// Build metadata and prerelease components carry no ordering information
	// this check uses, and a "-rc.2" left in place would make the minor
	// component unparseable.
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}

	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("talos: %q has no major.minor component", version)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("talos: major component of %q: %w", version, err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("talos: minor component of %q: %w", version, err)
	}
	return major, minor, nil
}
