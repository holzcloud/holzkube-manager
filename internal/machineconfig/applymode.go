package machineconfig

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Which apply mode a change needs, computed rather than guessed.
//
// Talos accepts a configuration in four modes, and choosing wrong is not a
// failure that announces itself:
//
//   - `auto` lets Talos decide, which usually means a reboot.
//   - `no-reboot` refuses outright if the change needs one -- which is the
//     safe direction, and the reason it is not the default here is that a
//     refusal at apply time is worse than a warning before it.
//   - `staged` writes the configuration for the *next* boot and changes
//     nothing now. A second staged apply silently replaces the first.
//   - `try` applies with a timer and rolls back if nobody confirms. It is
//     what stands between a network change and a node nobody can reach.
//
// The table below is a whitelist: a path that is not in it needs a reboot.
// That default is the conservative one -- it over-reports rather than
// under-reports -- and the alternative would be a config change that claims
// to take effect immediately and does not.

// Mode is a Talos apply mode.
type Mode string

const (
	// ModeNoReboot applies immediately without restarting the node.
	ModeNoReboot Mode = "no-reboot"

	// ModeReboot applies and restarts the node.
	ModeReboot Mode = "reboot"

	// ModeStaged writes the configuration for the next boot.
	ModeStaged Mode = "staged"

	// ModeTry applies with a rollback timer.
	ModeTry Mode = "try"
)

// TryTimeout is how long a `try` apply waits before rolling back.
//
// Sixty seconds is Talos's own default and is the number the UI counts down.
// It is a constant here rather than a setting, because the countdown on the
// screen and the timer on the node have to be the same number or the screen is
// lying about how long is left.
const TryTimeout = 60 * time.Second

// noRebootPaths are the configuration paths Talos applies without a restart.
//
// The list is a whitelist and anything not on it is reported as needing a
// reboot. It is curated and shipped in the binary, for the reason the
// compatibility matrix is: there is no machine-readable upstream source, and
// a wrong entry here is a change that reports success and does nothing.
//
// A prefix matches its children: `.machine.network` covers
// `.machine.network.interfaces[0].addresses`.
var noRebootPaths = []string{
	".cluster",
	".machine.time",
	".machine.network",
	".machine.sysctls",
	".machine.sysfs",
	".machine.env",
	".machine.files",
	".machine.logging",
	".machine.kubelet",
	".machine.registries",
	".machine.nodeLabels",
	".machine.nodeAnnotations",
	".machine.nodeTaints",
	".machine.certSANs",
	".machine.features",
	".machine.pods",
	".machine.udev",
	".machine.seccompProfiles",
}

// installPrefix is the path whose changes report success and do nothing until
// the next install or upgrade (CFG-07).
//
// It is singled out because it is the one case where Talos's answer is
// actively misleading: the apply succeeds, the node reports the new
// configuration, and the disk it would install to is unchanged until somebody
// runs an upgrade. An operator who read "applied" and walked away has a node
// that will surprise them months later.
const installPrefix = ".machine.install"

// networkPrefix is the path that can make a node unreachable (CFG-09).
const networkPrefix = ".machine.network"

// Verdict is what holzkube-manager says about a set of changed paths.
type Verdict struct {
	// Mode is the mode holzkube-manager recommends. It is a recommendation and not a
	// decision: the operator chooses, and the screen shows why.
	Mode Mode `json:"mode"`

	// RebootRequired reports that at least one changed path is outside the
	// no-reboot whitelist.
	RebootRequired bool `json:"reboot_required"`

	// RebootPaths are the paths that force it, so the sentence "this needs a
	// reboot" can be followed by "because of this".
	RebootPaths []string `json:"reboot_paths,omitempty"`

	// InstallOnly are `.machine.install` paths: they will apply and do nothing
	// until the next install or upgrade.
	InstallOnly []string `json:"install_only,omitempty"`

	// NetworkPaths are `.machine.network` paths. When any are present, `try`
	// is recommended and TrySeconds is the countdown.
	NetworkPaths []string `json:"network_paths,omitempty"`
	TrySeconds   int      `json:"try_seconds,omitempty"`

	// Sentences are the whole verdict in words, in the order they should be
	// read. The screen renders these rather than re-deriving prose from the
	// flags, so the words and the flags cannot disagree.
	Sentences []string `json:"sentences"`
}

// ModeFor computes the apply mode a set of changed paths needs.
//
// paths are dotted, rooted at the document: `.machine.network.hostname`.
func ModeFor(paths []string) Verdict {
	v := Verdict{Mode: ModeNoReboot}

	for _, p := range paths {
		switch {
		case strings.HasPrefix(p, installPrefix):
			v.InstallOnly = append(v.InstallOnly, p)
		case strings.HasPrefix(p, networkPrefix):
			v.NetworkPaths = append(v.NetworkPaths, p)
		}

		if !noReboot(p) {
			v.RebootRequired = true
			v.RebootPaths = append(v.RebootPaths, p)
		}
	}

	sort.Strings(v.RebootPaths)
	sort.Strings(v.InstallOnly)
	sort.Strings(v.NetworkPaths)

	switch {
	case v.RebootRequired:
		v.Mode = ModeReboot
	case len(v.NetworkPaths) > 0:
		// A network change that is wrong makes the node unreachable, and an
		// unreachable node cannot be told to undo it. `try` is the only mode
		// that survives that mistake.
		v.Mode = ModeTry
		v.TrySeconds = int(TryTimeout / time.Second)
	default:
		v.Mode = ModeNoReboot
	}

	v.Sentences = sentencesFor(v, len(paths))
	return v
}

func noReboot(path string) bool {
	// `.machine.install` is *not* on the no-reboot list even though applying
	// it never restarts anything, because "no reboot needed" would read as
	// "takes effect now" -- and it does not. It is called out separately
	// instead.
	if strings.HasPrefix(path, installPrefix) {
		return false
	}
	for _, prefix := range noRebootPaths {
		if path == prefix || strings.HasPrefix(path, prefix+".") || strings.HasPrefix(path, prefix+"[") {
			return true
		}
	}
	return false
}

func sentencesFor(v Verdict, total int) []string {
	var out []string

	if total == 0 {
		return []string{"Nothing changes."}
	}

	switch v.Mode {
	case ModeNoReboot:
		out = append(out, "This applies immediately without restarting the node.")
	case ModeTry:
		out = append(out, fmt.Sprintf(
			"This changes the node's network. Apply it with a %d-second rollback timer: "+
				"if the node becomes unreachable, it undoes the change by itself. "+
				"Without the timer, a mistake here is a node you cannot reach to fix.",
			v.TrySeconds))
	case ModeReboot:
		out = append(out, fmt.Sprintf(
			"This needs a reboot: %s %s outside what Talos can change while running.",
			joinPaths(v.RebootPaths),
			plural(len(v.RebootPaths), "is", "are")))
	case ModeStaged:
		out = append(out, "This is written for the next boot and changes nothing now.")
	}

	if len(v.InstallOnly) > 0 {
		out = append(out, fmt.Sprintf(
			"%s %s only at the next install or upgrade. The apply will report success and the "+
				"node's install settings will not change until then.",
			joinPaths(v.InstallOnly),
			plural(len(v.InstallOnly), "takes effect", "take effect")))
	}

	if len(v.NetworkPaths) > 0 && v.Mode == ModeReboot {
		out = append(out, "It also changes the network, so check the addresses carefully: "+
			"the reboot is what applies them, and there is no rollback timer on a reboot.")
	}

	return out
}

func joinPaths(paths []string) string {
	if len(paths) == 1 {
		return paths[0]
	}
	if len(paths) <= 3 {
		return strings.Join(paths[:len(paths)-1], ", ") + " and " + paths[len(paths)-1]
	}
	return fmt.Sprintf("%s and %d more", strings.Join(paths[:3], ", "), len(paths)-3)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// NoRebootPaths returns the whitelist, so a test can walk it rather than
// restate it.
func NoRebootPaths() []string { return append([]string(nil), noRebootPaths...) }
