package hostaction

import (
	"io/fs"
	"strings"
)

// Is the helper installed? (D-12, HACT-07)
//
// An order is only worth placing when something will pick it up. The obvious
// way to ask -- systemd over D-Bus, "is holzkube-manager-host.path active?" --
// is closed to this process on purpose: its unit sets
// RestrictAddressFamilies=AF_INET AF_INET6, so it has no AF_UNIX socket to
// reach PID 1 with, and that hardening stays. What it can do under
// ProtectSystem=strict is read the files systemd itself reads. So the helper
// counts as installed when three things are there, and each one missing is
// reported on its own, in this order:
//
//   - the script, /usr/local/sbin/holzkube-manager-host: a regular file,
//     executable, owned by uid 0 and writable by nobody else. A script root
//     does not own, or that group or other may change, would be somebody
//     else's code run as root, and that is not called installed;
//   - both unit files in /etc/systemd/system;
//   - the path unit's paths.target.wants entry, which `systemctl enable`
//     makes. Without it the units are there but nothing starts them.
//
// The enable entry is only asked about once both unit files are there. A unit
// that is not installed cannot be "installed but not enabled", and the install
// commands enable it in the same run, so while a unit file is missing the
// path-unit item is the whole answer for the units.
//
// What this cannot see is a path unit that is enabled but stopped or failed.
// For that the second net is the Box's pickup timer: an order nobody takes
// within DefaultPickupTimeout is withdrawn, and the warning names
// `systemctl status holzkube-manager-host.path`.

// The helper's files, as the operator installs them (deploy/HOST-HELPER.md).
const (
	// HelperScriptPath is the root helper, installed from
	// deploy/holzkube-manager-host.sh.
	HelperScriptPath = "/usr/local/sbin/holzkube-manager-host"
	// PathUnitPath is the unit that watches for orders.
	PathUnitPath = "/etc/systemd/system/holzkube-manager-host.path"
	// ServiceUnitPath is the unit the path unit starts, which runs the script.
	ServiceUnitPath = "/etc/systemd/system/holzkube-manager-host.service"
	// WantsLinkPath is what `systemctl enable holzkube-manager-host.path`
	// creates: the path unit's WantedBy=paths.target.
	WantsLinkPath = "/etc/systemd/system/paths.target.wants/holzkube-manager-host.path"
	// UpdateCheckUnitPath is the oneshot the helper starts for a
	// "check-update" order, installed from
	// deploy/holzkube-manager-update-check.service. It has no [Install]
	// section: nothing enables it, only the helper starts it.
	UpdateCheckUnitPath = "/etc/systemd/system/holzkube-manager-update-check.service"
	// UpdateScriptPath is the reference installation's update script,
	// installed from deploy/holzkube-manager-update.sh. The hourly timer runs
	// it to update; the check unit runs it with --check, which only looks.
	UpdateScriptPath = "/usr/local/sbin/holzkube-manager-update"
)

// HelperOrdersMarker begins the one line in the helper script that names the
// orders it carries out, separated by single spaces:
//
//	# holzkube-manager-host orders: reboot poweroff restart-service update check-update
//
// The daemon reads it from the installed script to learn which orders that
// helper knows. A helper without the line predates it and knows exactly the
// first four: reboot, poweroff, restart-service and update. A test holds the
// line to the script's pattern, its case arms and Actions().
const HelperOrdersMarker = "# holzkube-manager-host orders: "

// The three things that can be missing, in the order Detect reports them.
const (
	// MissingScript: the helper script is absent, or not something root may
	// run as root (not regular, not executable, not owned by uid 0, or
	// writable by group or other).
	MissingScript = "script"
	// MissingPathUnit: one of the two unit files is absent or not a regular
	// file.
	MissingPathUnit = "path-unit"
	// MissingNotEnabled: the units are there, but the path unit is not
	// enabled, so nothing starts the helper when an order appears.
	MissingNotEnabled = "not-enabled"
)

// Missing is one piece of the helper that is not installed.
type Missing struct {
	// Item is MissingScript, MissingPathUnit or MissingNotEnabled.
	Item string `json:"item"`
	// Path is the absolute path of the file that was found wanting.
	Path string `json:"path"`
}

// InstallCommands installs the helper, run from the root of an unpacked
// release archive (or a checkout).
//
// This is the one copy. The host page shows it when the helper is missing,
// and deploy/HOST-HELPER.md carries the same four lines, held to this slice
// byte for byte by a test -- a test and not an embed, because a package under
// internal/ cannot embed a file from deploy/.
var InstallCommands = []string{
	"sudo install -o root -g root -m 0755 deploy/holzkube-manager-host.sh /usr/local/sbin/holzkube-manager-host",
	"sudo install -o root -g root -m 0644 deploy/holzkube-manager-host.path deploy/holzkube-manager-host.service /etc/systemd/system/",
	"sudo systemctl daemon-reload",
	"sudo systemctl enable --now holzkube-manager-host.path",
}

// Detect reports which pieces of the helper are missing on the machine fsys
// is rooted at ("/"), in the order the script, the path unit, and its being
// enabled. Its being enabled is only looked at when both unit files are
// there. It never returns nil: an installed helper is an empty list.
func Detect(fsys fs.FS) []Missing {
	missing := []Missing{}
	if !scriptInstalled(fsys) {
		missing = append(missing, Missing{Item: MissingScript, Path: HelperScriptPath})
	}
	unitsInstalled := true
	for _, p := range []string{PathUnitPath, ServiceUnitPath} {
		info, err := fs.Stat(fsys, fsName(p))
		if err != nil || !info.Mode().IsRegular() {
			missing = append(missing, Missing{Item: MissingPathUnit, Path: p})
			unitsInstalled = false
			// One item for the pair: the operator installs both with one
			// command.
			break
		}
	}
	// Lstat: any entry under the name enables the unit -- the symlink
	// `systemctl enable` makes, or a file somebody copied there. Whether a
	// link's target exists is the unit file's question, asked above; and
	// while that question has no, "not enabled" would not be true.
	if unitsInstalled {
		if _, err := fs.Lstat(fsys, fsName(WantsLinkPath)); err != nil {
			missing = append(missing, Missing{Item: MissingNotEnabled, Path: WantsLinkPath})
		}
	}
	return missing
}

// scriptInstalled reports whether the helper script is one root may run: a
// regular file, executable, writable by nobody but its owner, and owned by
// uid 0. An owner that cannot be told is no proof, and counts as not
// installed.
func scriptInstalled(fsys fs.FS) bool {
	info, err := fs.Stat(fsys, fsName(HelperScriptPath))
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	perm := info.Mode().Perm()
	if perm&0o111 == 0 || perm&0o022 != 0 {
		return false
	}
	uid, known := ownerUID(info)
	return known && uid == 0
}

// fsName is an absolute path as an fs.FS rooted at "/" names it.
func fsName(path string) string {
	return strings.TrimPrefix(path, "/")
}

// Missing reports which pieces of the helper are missing, read through the
// Box's FS. Empty, never nil, when the helper is installed.
func (b *Box) Missing() []Missing {
	return Detect(b.cfg.FS)
}
