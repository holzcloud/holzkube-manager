package upgrade

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/compat"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// Choosing what to upgrade to, and refusing the choices that strand a cluster.
//
// There is no "latest" button here, and that is UPG-05 rather than an
// omission. "Latest" is a promise this build cannot keep: the newest Talos
// release may be two minors away, Talos does not support skipping a minor, and
// a button that quietly did two upgrades in a row is a button that does the
// second one against a cluster whose state nobody looked at. What replaces it
// is a **chain** -- every intermediate minor, listed, each one a step the
// operator can see.

// ErrNoPath reports that there is no upgrade path between two versions.
var ErrNoPath = errors.New("upgrade: there is no supported path between these versions")

// ErrWouldStrand reports a Talos upgrade that would take the running
// Kubernetes version out of support (UPG-06).
//
// It is an error rather than a warning, and that is the requirement's word:
// a cluster whose Kubernetes falls out of the window is a cluster whose
// kubelet and control plane are running against an API surface Talos no longer
// tests, and the way out is a Kubernetes upgrade that the same Talos version
// may no longer be able to perform.
var ErrWouldStrand = errors.New("upgrade: this Talos upgrade would leave the running Kubernetes version unsupported")

// Version is a parsed Talos or Kubernetes version.
type Version struct {
	Major, Minor, Patch int

	// Pre is a pre-release suffix: "alpha.0", "beta.1", "rc.2". A version that
	// has one is filtered out of every path this package computes -- see
	// ParseVersion.
	Pre string
}

// String renders the version the way Talos writes it.
func (v Version) String() string {
	s := fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Contract is the major.minor form machinery wants.
func (v Version) Contract() string { return fmt.Sprintf("v%d.%d", v.Major, v.Minor) }

// IsPreRelease reports whether this is an alpha, beta or release candidate.
func (v Version) IsPreRelease() bool { return v.Pre != "" }

// Less orders two versions.
func (v Version) Less(o Version) bool {
	switch {
	case v.Major != o.Major:
		return v.Major < o.Major
	case v.Minor != o.Minor:
		return v.Minor < o.Minor
	case v.Patch != o.Patch:
		return v.Patch < o.Patch
	}
	// A pre-release precedes its own release: v1.14.0-beta.1 is older than
	// v1.14.0. Comparing the suffixes lexically is enough for the three forms
	// Talos uses and is not enough in general, which is why nothing here sorts
	// pre-releases against each other for a decision.
	switch {
	case v.Pre == o.Pre:
		return false
	case v.Pre == "":
		return false
	case o.Pre == "":
		return true
	}
	return v.Pre < o.Pre
}

// ParseVersion reads a Talos or Kubernetes version string.
func ParseVersion(s string) (Version, error) {
	raw := strings.TrimPrefix(strings.TrimSpace(s), "v")

	pre := ""
	if i := strings.IndexAny(raw, "-+"); i >= 0 {
		pre = raw[i+1:]
		raw = raw[:i]
	}

	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return Version{}, fmt.Errorf("upgrade: %q is not a version", s)
	}

	nums := make([]int, 3)
	for i := range 3 {
		if i >= len(parts) {
			break
		}
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("upgrade: %q is not a version", s)
		}
		nums[i] = n
	}

	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2], Pre: pre}, nil
}

// Step is one hop of an upgrade path.
type Step struct {
	// To is the version this step installs.
	To Version `json:"to"`

	// Why says what this step is for, because a chain's middle steps are the
	// ones an operator will otherwise ask about: they are not the version
	// anybody wanted, they are the version Talos requires passing through.
	Why string `json:"why"`
}

// Chain computes the ordered list of versions to pass through (UPG-05).
//
// Talos supports upgrading **one minor at a time**. Going from 1.12 to 1.14 is
// two upgrades, each with a reboot and each with the health gate re-evaluated,
// and a path that jumped straight there would be an unsupported upgrade that
// may well work and is nobody's to promise.
//
// available is the set of releases to choose from. For each intermediate minor
// the newest available patch is taken, because a bug fixed in a patch is fixed
// for a reason and passing through a known-broken release on the way somewhere
// else is a self-inflicted incident.
//
// Pre-releases are filtered out entirely. An upgrade chain that routed a
// cluster through a release candidate would be routing it through software
// whose own project does not call it finished.
func Chain(from, to Version, available []Version) ([]Step, error) {
	switch {
	case from.Major != to.Major:
		return nil, fmt.Errorf("%w: %s and %s are different major versions, and nothing here "+
			"knows what that means", ErrNoPath, from, to)
	case to.Less(from):
		return nil, fmt.Errorf("%w: %s is older than %s. Talos does not support downgrading, and "+
			"a downgrade that appeared to work would be a cluster whose etcd has already written "+
			"a newer storage version", ErrNoPath, to, from)
	case to == from:
		return nil, nil
	}

	// The newest non-pre-release patch per minor.
	newest := map[int]Version{}
	for _, v := range available {
		if v.IsPreRelease() || v.Major != from.Major {
			continue
		}
		if cur, ok := newest[v.Minor]; !ok || cur.Less(v) {
			newest[v.Minor] = v
		}
	}

	var steps []Step
	for m := from.Minor + 1; m < to.Minor; m++ {
		v, ok := newest[m]
		if !ok {
			return nil, fmt.Errorf("%w: getting from %s to %s has to pass through 1.%d, and no "+
				"1.%d release is available to this installation. Talos upgrades one minor at a "+
				"time; there is no way to skip it", ErrNoPath, from, to, m, m)
		}
		steps = append(steps, Step{
			To: v,
			Why: fmt.Sprintf("Talos upgrades one minor version at a time, so %s to %s has to pass "+
				"through 1.%d. This is not the version you asked for; it is the one on the way.",
				from.Contract(), to.Contract(), m),
		})
	}

	why := "The version you chose."
	if len(steps) > 0 {
		why = "The version you chose, reached after the step(s) above."
	}
	steps = append(steps, Step{To: to, Why: why})

	return steps, nil
}

// Releases filters and sorts a list of version strings into upgrade
// candidates.
//
// Pre-releases are dropped, versions outside the range this build has been
// tested against are dropped, and anything unparsable is dropped. What is left
// is sorted newest first, which is the order an operator reads them in.
func Releases(versions []string) []Version {
	minSupported, errMin := ParseVersion(talos.MinSupportedVersion)
	maxSupported, errMax := ParseVersion(talos.MaxSupportedVersion)

	out := make([]Version, 0, len(versions))
	for _, s := range versions {
		v, err := ParseVersion(s)
		if err != nil || v.IsPreRelease() {
			continue
		}
		if errMin == nil && v.Less(minSupported) {
			continue
		}
		if errMax == nil && maxSupported.Less(Version{Major: v.Major, Minor: v.Minor}) {
			continue
		}
		out = append(out, v)
	}

	sort.Slice(out, func(i, j int) bool { return out[j].Less(out[i]) })
	return out
}

// StrandCheck is UPG-06: does this Talos upgrade leave Kubernetes unsupported?
type StrandCheck struct {
	// Blocked is whether the upgrade must not proceed.
	Blocked bool `json:"blocked"`

	// Sentence says what is wrong and what to do about it, naming both
	// versions. It is present whether or not the upgrade is blocked, because
	// "this is fine and here is why" is the answer to the question an operator
	// asks when the button is enabled.
	Sentence string `json:"sentence"`

	// Remedy is the concrete instruction, and it is separate from the sentence
	// because a client shows it as the next action rather than as prose.
	Remedy string `json:"remedy,omitempty"`
}

// CheckStrand asks whether upgrading Talos to `to` would leave the cluster's
// running Kubernetes version outside what that Talos supports.
//
// It blocks rather than warns. The failure it prevents is not an upgrade that
// goes wrong -- the upgrade succeeds, the node comes back, and the cluster is
// then running a Kubernetes version its Talos does not support, which is a
// state with no good way out: the Kubernetes upgrade that would fix it is
// performed by the same Talos that no longer supports the version it is
// upgrading from.
func CheckStrand(to Version, kubernetesVersion string) StrandCheck {
	v := compat.Check(to.String(), kubernetesVersion)

	if !v.Known {
		return StrandCheck{
			Blocked: true,
			Sentence: fmt.Sprintf("Talos %s is not in the compatibility table shipped with this "+
				"build, so nothing here can say which Kubernetes versions it supports.", to.Contract()),
			Remedy: "Upgrade holzkube-manager to a build whose table covers this Talos version, or " +
				"perform this upgrade with talosctl, having checked the release notes yourself.",
		}
	}

	if kubernetesVersion == "" {
		return StrandCheck{
			Blocked: true,
			Sentence: "This cluster's Kubernetes version is not known, so whether Talos " +
				to.Contract() + " supports it cannot be answered.",
			Remedy: "Refresh the cluster so its nodes report their Kubernetes version, then look again.",
		}
	}

	if v.Supported {
		return StrandCheck{Sentence: v.Sentence}
	}

	return StrandCheck{
		Blocked:  true,
		Sentence: v.Sentence,
		Remedy: fmt.Sprintf(
			"Bring Kubernetes into the range Talos %s supports (1.%d to 1.%d) before upgrading "+
				"Talos. Doing it the other way round leaves the cluster on a Kubernetes version "+
				"its own Talos no longer supports, and the upgrade that would fix that is "+
				"performed by that same Talos.",
			to.Contract(), v.Window.MinMinor, v.Window.MaxMinor),
	}
}

// MaxKubernetesIntermediates is the most intermediate minors a Kubernetes
// chain may have.
//
// The longest chain the compatibility matrix can honestly ask for runs from the
// bottom of the lowest window (Kubernetes 1.26 under Talos 1.7) to the top of the
// highest (1.37): ten intermediate minors. Sixteen leaves room. The cap exists
// only so that a typed target such as v1.2000000000.0 neither builds an enormous
// list nor sends a flood of lookups to a source somebody else runs.
const MaxKubernetesIntermediates = 16

// kubernetesChainPrecheck is the question KubernetesChain and PlanKubernetes
// both ask first, kept in one place so that a plan and a chain can never answer
// it differently: is this a downgrade, and does the Talos that is running carry
// the target?
//
// The window is checked for the target only, not for the start and not for the
// intermediate minors; that is a known limit of the chain and not something this
// function decides.
func kubernetesChainPrecheck(from, to Version, talosVersion string) error {
	switch {
	case to.Less(from):
		return fmt.Errorf("%w: Kubernetes does not support downgrading", ErrNoPath)
	case to == from:
		return nil
	}

	v := compat.Check(talosVersion, to.String())
	if v.Known && !v.Supported {
		return fmt.Errorf("%w: %s", ErrWouldStrand, v.Sentence)
	}
	return nil
}

// kubernetesIntermediateMinors lists the minors strictly between the start and
// the target, in ascending order. They are the ones whose newest patch a chain
// has to know.
//
// It counts before it builds, so that neither the list nor the work of making
// it depends on how far away a typed target is.
func kubernetesIntermediateMinors(from, to Version) ([]int, error) {
	n := to.Minor - from.Minor - 1
	if n <= 0 {
		return nil, nil
	}
	if n > MaxKubernetesIntermediates {
		return nil, fmt.Errorf("%w: getting from %s to %s would pass through %d minor versions, and "+
			"nothing here plans more than %d. Kubernetes upgrades one minor at a time; choose a "+
			"nearer target", ErrNoPath, from, to, n, MaxKubernetesIntermediates)
	}
	minors := make([]int, 0, n)
	for m := from.Minor + 1; m < to.Minor; m++ {
		minors = append(minors, m)
	}
	return minors, nil
}

// KubernetesChain computes the Kubernetes versions to pass through.
//
// Kubernetes upgrades one minor at a time as well, but for a different reason
// than Talos: the project's API deprecation policy requires kube-apiserver to
// not skip a minor when it is upgraded, so every minor between the start and
// the target has to be passed through. (The version skew policy would allow a
// kubelet to be several minors behind; it is the API server that decides.) It
// is a separate function from Chain because the two are bounded by different
// things -- a Talos chain is bounded by what is released, a Kubernetes chain by
// what the running Talos supports.
//
// newest maps a minor to the newest stable patch of it, as the caller looked it
// up. Each intermediate step is taken from it, and a minor missing from it is a
// refusal that names the minor: this function never puts a version in by
// itself, because the first release of a minor is the oldest one and is exactly
// the step nobody should be sent through. Targets one minor away or in the same
// minor have nothing on the way and need no newest at all.
func KubernetesChain(from, to Version, talosVersion string, newest map[int]Version) ([]Step, error) {
	if err := kubernetesChainPrecheck(from, to, talosVersion); err != nil {
		return nil, err
	}
	if to == from {
		return nil, nil
	}

	minors, err := kubernetesIntermediateMinors(from, to)
	if err != nil {
		return nil, err
	}

	var steps []Step
	for _, m := range minors {
		v, ok := newest[m]
		if !ok {
			return nil, fmt.Errorf("%w: getting from %s to %s has to pass through 1.%d, and its "+
				"newest patch is not known. Kubernetes upgrades one minor at a time; there is no "+
				"way to skip it, and no version is guessed in its place", ErrNoPath, from, to, m)
		}
		steps = append(steps, Step{
			To: v,
			Why: fmt.Sprintf("kube-apiserver may not skip a minor version when it is upgraded, so "+
				"%s to %s has to pass through 1.%d. This is the newest stable patch of 1.%d, not "+
				"the version you asked for; it is the one on the way.",
				from.Contract(), to.Contract(), m, m),
		})
	}

	// A target in the same minor is a patch jump: nothing on the way, and no
	// step either, as before.
	if to.Minor > from.Minor {
		steps = append(steps, Step{To: to, Why: "The version you chose."})
	}
	return steps, nil
}
