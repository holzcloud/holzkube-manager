package imagefactory

import (
	"fmt"
	"strconv"
	"strings"
)

// IsPrerelease reports whether a Talos version carries a semver prerelease
// component.
//
// The decision is structural and consults no curated data at all, which is
// exactly why D-08 says prerelease filtering needs no table: "v1.14.0-rc.2" is
// a prerelease because semver says a hyphen introduces one, not because someone
// remembered to add it to a list. A list would have to be maintained forever
// and would be wrong the day upstream tags something nobody updated it for.
//
// A version this package cannot parse is reported as a prerelease. That is the
// fail-safe direction: an unrecognised string is not a stable release just
// because it also failed to look like a prerelease, and the cost of the two
// mistakes is not symmetric -- offering a release candidate as the default is
// the trap PITFALLS P9(d) records.
func IsPrerelease(version string) bool {
	v, err := parseTalosVersion(version)
	if err != nil {
		return true
	}
	return v.prerelease != ""
}

// SplitVersions divides a version list into the stable versions and everything
// else, preserving the order of the input.
//
// Both slices are non-nil even when empty, so a JSON encoding of "there are no
// prereleases" is [] and not null. A null reads to a client as "the server did
// not check".
func SplitVersions(versions []string) (stable, prerelease []string) {
	stable = make([]string, 0, len(versions))
	prerelease = make([]string, 0)

	for _, v := range versions {
		if IsPrerelease(v) {
			prerelease = append(prerelease, v)
			continue
		}
		stable = append(stable, v)
	}
	return stable, prerelease
}

// NewestStable returns the highest stable version in the list.
//
// It compares rather than taking the last element. The upstream list is served
// in ascending order and ends in the current alpha, beta and rc tags, so "the
// last element" is a release candidate -- PITFALLS P9(d) records exactly that,
// live, against a 108-entry response. Ordering is also not a property this
// package can check, so relying on it would make correctness depend on
// something nothing asserts.
//
// A list with no stable version is an error rather than the newest prerelease.
// Silently promoting a release candidate is the behaviour FACT-05 exists to
// prevent.
func NewestStable(versions []string) (string, error) {
	var (
		bestRaw string
		best    talosVersion
		found   bool
	)

	for _, raw := range versions {
		v, err := parseTalosVersion(raw)
		if err != nil || v.prerelease != "" {
			continue
		}
		if !found || v.greaterThan(best) {
			bestRaw, best, found = raw, v, true
		}
	}

	if !found {
		return "", fmt.Errorf("imagefactory: no stable Talos version among the %d offered; "+
			"a prerelease is never promoted to fill this in", len(versions))
	}
	return bestRaw, nil
}

// talosVersion is a parsed vMAJOR.MINOR.PATCH[-prerelease][+build].
//
// The comparison is implemented here rather than with golang.org/x/mod/semver.
// That module is reachable in the graph but is not a requirement of this one,
// and this plan adds no module requirement: the comparison needed here is over
// stable versions only, so it is three integers and no prerelease precedence
// rules at all. Importing a module to compare three integers is the trade this
// project's dependency posture (D-01) exists to refuse.
type talosVersion struct {
	major, minor, patch int
	prerelease          string
}

// greaterThan compares two parsed versions numerically. Only stable versions
// reach it: NewestStable filters prereleases out first, so semver's prerelease
// precedence rules are deliberately not implemented rather than implemented
// approximately.
func (v talosVersion) greaterThan(other talosVersion) bool {
	switch {
	case v.major != other.major:
		return v.major > other.major
	case v.minor != other.minor:
		return v.minor > other.minor
	default:
		return v.patch > other.patch
	}
}

// parseTalosVersion reads the shape the Factory serves. Anything else is an
// error, never a best-effort reading: a version this package guessed at is a
// version an operator is offered without anyone having checked it.
func parseTalosVersion(version string) (talosVersion, error) {
	rest, ok := strings.CutPrefix(version, "v")
	if !ok {
		return talosVersion{}, fmt.Errorf("imagefactory: %q does not begin with v", version)
	}

	// Build metadata is stripped before the prerelease is read, so that
	// "v1.13.9+deadbeef" is a release and "v1.14.0-rc.1+abc" is not.
	if plus := strings.IndexByte(rest, '+'); plus >= 0 {
		rest = rest[:plus]
	}

	var prerelease string
	if hyphen := strings.IndexByte(rest, '-'); hyphen >= 0 {
		prerelease = rest[hyphen+1:]
		rest = rest[:hyphen]
		if prerelease == "" {
			return talosVersion{}, fmt.Errorf("imagefactory: %q has an empty prerelease component", version)
		}
	}

	parts := strings.Split(rest, ".")
	if len(parts) != 3 {
		return talosVersion{}, fmt.Errorf("imagefactory: %q is not major.minor.patch", version)
	}

	numbers := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || (len(p) > 1 && p[0] == '0') {
			return talosVersion{}, fmt.Errorf("imagefactory: %q has a non-numeric component %q", version, p)
		}
		numbers[i] = n
	}

	return talosVersion{
		major:      numbers[0],
		minor:      numbers[1],
		patch:      numbers[2],
		prerelease: prerelease,
	}, nil
}
