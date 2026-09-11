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

// ErrPreRelease reports a node running an alpha, beta or release candidate
// while this instance has not opted into them (OPS-03).
//
// It is separate from ErrUnsupportedVersion because the remedy is the opposite
// kind of thing. An unsupported version is a fact about the node and the
// answer is to change the node; a pre-release is a fact about *this
// installation's* settings, and the answer is a decision the operator makes
// once -- so a client showing them as the same refusal would send somebody to
// reinstall a node they deliberately put a release candidate on.
var ErrPreRelease = errors.New("talos: this node runs a Talos pre-release and this instance has not opted into pre-releases")

// CheckSupportedVersion reports whether a node's reported version tag is one
// holzkube-manager supports.
//
// The comparison is major.minor only and is implemented here rather than by
// importing golang.org/x/mod/semver, for the reason plan 02-04 gives for the
// same choice: the whole comparison is two integers against two integers, and
// a module added for that is a module in the graph forever.
//
// # Pre-releases are opt-in (OPS-03)
//
// The prerelease component is stripped before the range comparison, so
// v1.14.0-rc.2 is *inside* the window -- and it is then refused separately
// unless allowPreRelease is set. The two checks are separate because the
// answers to them are: "this node is outside the supported range" is a fact
// about the node, and "this instance does not accept pre-releases" is a
// setting, and an operator who deliberately put a release candidate on a node
// needs to be told which of the two they are looking at.
//
// The default is to refuse, and that is the direction the requirement asks
// for. The reason it is the right default rather than a cautious one: a
// pre-release is software its own project does not call finished, and every
// guarantee this product makes about a node -- the apply modes, the
// compatibility window, the resource paths the inventory reads -- is a claim
// about released Talos. Accepting one silently would be extending those
// claims to something nobody tested them against.
func CheckSupportedVersion(version string, allowPreRelease bool) error {
	major, minor, err := parseMajorMinor(version)
	if err != nil {
		return fmt.Errorf("%w: cannot read a version from %q; holzkube-manager supports %s to %s",
			ErrUnsupportedVersion, version, MinSupportedVersion, MaxSupportedVersion)
	}

	if pre := preRelease(version); pre != "" && !allowPreRelease {
		return fmt.Errorf("%w: it reports %s. Start holzkube-manager with --allow-prerelease (or "+
			"HOLZKUBE_MANAGER_ALLOW_PRERELEASE=true) if that is deliberate; everything this "+
			"product guarantees about a node is a claim about released Talos",
			ErrPreRelease, version)
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

// preRelease returns the prerelease component of a version tag, or the empty
// string.
//
// Build metadata after a `+` is not a prerelease: `v1.14.0+dirty` is a release
// built from a modified tree, which is a different thing from `v1.14.0-rc.2`
// and must not be refused as one.
func preRelease(version string) string {
	v := strings.TrimSpace(version)
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[i+1:]
	}
	return ""
}

// IsPreRelease reports whether a version tag names an alpha, beta or release
// candidate.
//
// It is exported because the inventory marks such a node on the screen
// (OPS-03), and that marking has to work on a node this instance refused to
// connect to -- which is precisely the node an operator is looking for.
func IsPreRelease(version string) bool { return preRelease(version) != "" }

// InSupportedRange reports whether a version tag is inside the window,
// ignoring any prerelease component.
//
// It exists alongside CheckSupportedVersion because a *screen* needs the
// answer without needing a refusal: the machine list marks a node outside the
// range whether or not anything has tried to connect to it, and a marking that
// depended on a failed connection would be missing exactly when the node is
// down for an unrelated reason.
func InSupportedRange(version string) bool {
	major, minor, err := parseMajorMinor(version)
	if err != nil {
		return false
	}
	minMajor, minMinor, err := parseMajorMinor(MinSupportedVersion)
	if err != nil {
		return false
	}
	maxMajor, maxMinor, err := parseMajorMinor(MaxSupportedVersion)
	if err != nil {
		return false
	}
	return !before(major, minor, minMajor, minMinor) && !before(maxMajor, maxMinor, major, minor)
}
