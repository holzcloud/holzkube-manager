// Package compat holds the Talos-to-Kubernetes compatibility matrix.
//
// It is a curated table compiled into the binary, reviewable in git and
// shipped with the release -- the same construction, and the same reasoning,
// as the known-broken version list in internal/imagefactory: there is no
// machine-readable upstream source for it, and a lookup over the network would
// be unavailable in exactly the situation this tool exists for.
//
// It is deliberately separate from talos.MinSupportedVersion and
// talos.MaxSupportedVersion (D-25). Those say which Talos versions *holzkube-manager*
// has been tested against; this says which Kubernetes versions *Talos*
// supports. Folded into one constant, neither would be comprehensible and
// neither could be maintained on its own.
package compat

import (
	"fmt"
	"strconv"
	"strings"
)

// Window is the Kubernetes version range one Talos minor release supports.
//
// Min and Max are inclusive and are minor versions of Kubernetes 1.x. Default
// is what a fresh install of that Talos version brings up, which is what makes
// "you are two minors behind the default" a sentence an operator can act on.
type Window struct {
	MinMinor     int
	MaxMinor     int
	DefaultMinor int
}

// matrix maps a Talos minor version to its Kubernetes window.
//
// Source: the Talos release notes' "Kubernetes" section for each minor, which
// states the supported range explicitly. Every row is a deliberate edit; a
// Talos version that is not here has no window, and holzkube-manager says so rather
// than extrapolating -- an extrapolated window is an upgrade that is allowed
// to strand a cluster.
var matrix = map[int]Window{
	// Talos 1.7 .. 1.14. The range each release supports moves by one minor
	// per release, with the default tracking the newest Kubernetes at the time
	// the Talos release was cut.
	7:  {MinMinor: 26, MaxMinor: 30, DefaultMinor: 30},
	8:  {MinMinor: 27, MaxMinor: 31, DefaultMinor: 31},
	9:  {MinMinor: 28, MaxMinor: 32, DefaultMinor: 32},
	10: {MinMinor: 29, MaxMinor: 32, DefaultMinor: 32},
	11: {MinMinor: 30, MaxMinor: 33, DefaultMinor: 33},
	12: {MinMinor: 31, MaxMinor: 33, DefaultMinor: 33},
	13: {MinMinor: 32, MaxMinor: 34, DefaultMinor: 34},
	14: {MinMinor: 32, MaxMinor: 35, DefaultMinor: 35},
}

// Verdict is what the matrix says about one node's pair of versions.
//
// Known is false when the Talos version has no row, and every other field is
// then meaningless. That is the honest answer for a version nobody curated,
// and it is the one case in which the screen shows nothing rather than a
// number.
type Verdict struct {
	Known bool

	// Supported reports whether this Kubernetes version is inside the window.
	Supported bool

	Window Window

	// HeadroomMinors is how many Kubernetes minor versions remain between the
	// running one and the top of the window. Zero means the next Kubernetes
	// upgrade needs a Talos upgrade first; a negative number means the cluster
	// is already outside the window.
	HeadroomMinors int

	// Sentence is the same fact in words. Both are shown (D-24): the number is
	// what a dashboard sorts on, and the sentence is what somebody acts on.
	Sentence string
}

// Check reports what the matrix says about a node.
//
// Both versions are the strings the node reported -- "v1.13.9" and "v1.34.1",
// with or without the leading v. An unparsable version is not a guess; it is
// an unknown verdict, for the same reason an uncurated Talos version is.
func Check(talosVersion, kubernetesVersion string) Verdict {
	talosMinor, ok := minor(talosVersion)
	if !ok {
		return Verdict{Sentence: "The Talos version could not be read, so no compatibility statement is possible."}
	}

	win, ok := matrix[talosMinor]
	if !ok {
		return Verdict{Sentence: fmt.Sprintf(
			"Talos 1.%d is not in the compatibility table shipped with this build, so no statement is made about it.",
			talosMinor)}
	}

	k8sMinor, ok := minor(kubernetesVersion)
	if !ok {
		return Verdict{
			Known:    true,
			Window:   win,
			Sentence: fmt.Sprintf("Talos 1.%d supports Kubernetes 1.%d to 1.%d; this node's Kubernetes version is not known.", talosMinor, win.MinMinor, win.MaxMinor),
		}
	}

	v := Verdict{
		Known:          true,
		Window:         win,
		Supported:      k8sMinor >= win.MinMinor && k8sMinor <= win.MaxMinor,
		HeadroomMinors: win.MaxMinor - k8sMinor,
	}

	switch {
	case k8sMinor > win.MaxMinor:
		v.Sentence = fmt.Sprintf(
			"Kubernetes 1.%d is newer than Talos 1.%d supports (up to 1.%d). Upgrade Talos before this node is trusted to behave.",
			k8sMinor, talosMinor, win.MaxMinor)
	case k8sMinor < win.MinMinor:
		v.Sentence = fmt.Sprintf(
			"Kubernetes 1.%d is older than Talos 1.%d supports (from 1.%d). Upgrade Kubernetes.",
			k8sMinor, talosMinor, win.MinMinor)
	case v.HeadroomMinors == 0:
		v.Sentence = fmt.Sprintf(
			"Kubernetes 1.%d is at the top of what Talos 1.%d supports. The next Kubernetes upgrade needs a Talos upgrade first.",
			k8sMinor, talosMinor)
	default:
		v.Sentence = fmt.Sprintf(
			"Kubernetes 1.%d is %d minor version(s) below the top of what Talos 1.%d supports (1.%d).",
			k8sMinor, v.HeadroomMinors, talosMinor, win.MaxMinor)
	}
	return v
}

// TalosMinors returns every Talos minor the table covers, so a test can walk
// the whole matrix rather than restate it.
func TalosMinors() []int {
	out := make([]int, 0, len(matrix))
	for k := range matrix {
		out = append(out, k)
	}
	return out
}

// minor parses the minor version out of a 1.x version string.
//
// Only the 1.x line exists for either project, so a major that is not 1 is an
// unreadable version rather than a version from a future numbering scheme this
// table could say anything about.
func minor(version string) (int, bool) {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 || parts[0] != "1" {
		return 0, false
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
