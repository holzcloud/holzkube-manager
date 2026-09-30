package hostaction

import (
	"io"
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
//
// Is it new enough for the check? (D-12, HACT-04)
//
// A helper installed before check-update existed carries out the four older
// orders and refuses the fifth. The daemon cannot ask systemd which helper is
// installed, for the reason above, but it can read the installed script, as it
// reads the helper's files to tell whether it is installed at all. So
// KnownOrders reads the script's one marker line (HelperOrdersMarker), which
// names the orders that script carries out; a script without that line is the
// helper of 13-01, which knows the first four. It reads only once the script
// is one Detect calls installed -- regular, owned by uid 0, writable by nobody
// else -- and at most MaxHelperScriptSize bytes of it.
//
// The marker decides only what the page offers and what the routes refuse
// before placing anything. It is not the lock: the helper's own anchored
// pattern refuses every word it does not know, whatever any line claims, and
// 13-13 proved that on the frozen older helper as root.
//
// The check also needs its unit, holzkube-manager-update-check.service, which
// the helper starts. Outdated reports what of the two is not there. It is
// asked only while Detect reports nothing missing: the install commands
// install everything, the newer script and the check unit with it.

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

// MaxHelperScriptSize is the most of the installed script KnownOrders reads.
// The shipped script is under 10 KiB; a larger file is not the helper this
// daemon knows, and counts as one without the marker line.
const MaxHelperScriptSize = 64 << 10

// originalOrders are the orders of 13-01, which every helper without the
// marker line knows.
var originalOrders = []Action{Reboot, Poweroff, RestartService, Update}

// What the check needs that can be outdated, in the order Outdated reports
// them. Reported as Missing, with these items.
const (
	// OutdatedScript: the installed helper script does not name check-update
	// on its marker line, so it would refuse the order.
	OutdatedScript = "script-outdated"
	// OutdatedCheckUnit: the check unit the helper starts is absent or not a
	// regular file.
	OutdatedCheckUnit = "check-unit"
)

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
	// Item is MissingScript, MissingPathUnit or MissingNotEnabled -- or, in
	// Outdated's list, OutdatedScript or OutdatedCheckUnit.
	Item string `json:"item"`
	// Path is the absolute path of the file that was found wanting.
	Path string `json:"path"`
}

// InstallCommands installs the helper, run from the root of an unpacked
// release archive (or a checkout).
//
// The second line installs three units: the helper's path unit and service,
// and the check unit the helper starts for a check-update order.
// daemon-reload makes systemd see them; only the path unit is enabled. The
// check unit has no [Install] section -- nothing enables it, and only the
// helper starts it.
//
// This is the one copy. The host page shows it when the helper is missing,
// and deploy/HOST-HELPER.md carries the same four lines, held to this slice
// byte for byte by a test -- a test and not an embed, because a package under
// internal/ cannot embed a file from deploy/.
var InstallCommands = []string{
	"sudo install -o root -g root -m 0755 deploy/holzkube-manager-host.sh /usr/local/sbin/holzkube-manager-host",
	"sudo install -o root -g root -m 0644 deploy/holzkube-manager-host.path deploy/holzkube-manager-host.service deploy/holzkube-manager-update-check.service /etc/systemd/system/",
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

// KnownOrders reports which orders the helper script installed on the machine
// fsys is rooted at ("/") carries out, in Actions() order, each once: the
// known words on its one marker line. Words this daemon does not know are
// ignored. A script that is not installed (see Detect), is larger than
// MaxHelperScriptSize, cannot be read, or has no marker line or more than one
// counts as the helper of 13-01 and knows the original four. It returns no
// error and never quotes the file.
func KnownOrders(fsys fs.FS) []Action {
	if !scriptInstalled(fsys) {
		return append([]Action{}, originalOrders...)
	}
	raw, ok := readScript(fsys)
	if !ok {
		return append([]Action{}, originalOrders...)
	}
	var named []string
	markers := 0
	for _, line := range strings.Split(string(raw), "\n") {
		if rest, found := strings.CutPrefix(line, HelperOrdersMarker); found {
			markers++
			named = strings.Fields(rest)
		}
	}
	if markers != 1 {
		return append([]Action{}, originalOrders...)
	}
	known := []Action{}
	for _, a := range Actions() {
		for _, w := range named {
			if w == string(a) {
				known = append(known, a)
				break
			}
		}
	}
	return known
}

// readScript reads the installed helper script, refusing a file that is not
// regular, is larger than MaxHelperScriptSize, or grew past it after the size
// check.
func readScript(fsys fs.FS) ([]byte, bool) {
	name := fsName(HelperScriptPath)
	info, err := fs.Stat(fsys, name)
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxHelperScriptSize {
		return nil, false
	}
	f, err := fsys.Open(name)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, MaxHelperScriptSize+1))
	if err != nil || len(raw) > MaxHelperScriptSize {
		return nil, false
	}
	return raw, true
}

// Outdated reports what check-update needs on the machine fsys is rooted at
// that is not there: a helper script that names the order (OutdatedScript),
// then the check unit it starts (OutdatedCheckUnit). It never returns nil: a
// helper new enough for the check is an empty list. Ask it only when Detect
// reports nothing missing.
func Outdated(fsys fs.FS) []Missing {
	outdated := []Missing{}
	knows := false
	for _, a := range KnownOrders(fsys) {
		if a == CheckUpdate {
			knows = true
		}
	}
	if !knows {
		outdated = append(outdated, Missing{Item: OutdatedScript, Path: HelperScriptPath})
	}
	if info, err := fs.Stat(fsys, fsName(UpdateCheckUnitPath)); err != nil || !info.Mode().IsRegular() {
		outdated = append(outdated, Missing{Item: OutdatedCheckUnit, Path: UpdateCheckUnitPath})
	}
	return outdated
}

// Outdated reports what the check needs that is not there, read through the
// Box's FS. Empty, never nil, when the helper is new enough.
func (b *Box) Outdated() []Missing {
	return Outdated(b.cfg.FS)
}
