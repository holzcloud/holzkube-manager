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
//
// Is the update script there? (13-REVIEW-2 IN-04)
//
// Both update orders end in the reference installation's update script,
// UpdateScriptPath: update starts holzkube-manager-update.service, which runs
// it, and check-update starts the check unit, which runs it with --check. The
// helper's install commands do not install it -- it is the update mechanism's,
// and an installation that never set that up has none -- so a helper
// installed completely can still carry out both orders into a unit that fails.
// UpdateScriptMissing asks for it the way Detect asks for the helper's script:
// a regular file, executable, owned by uid 0 and writable by nobody else. A
// script somebody other than root may change is somebody else's code run as
// root, and is not called installed either. Only the two update orders need
// it; reboot, poweroff and restart-service go through without it. It is asked
// whatever Detect says, because the helper's install commands do not bring it:
// an operator who installs the helper should learn in the same reading that
// the update buttons need one more file.
//
// The hourly update's units (operator decision 2026-10-01)
//
// The update script runs every hour from holzkube-manager-update.service,
// which holzkube-manager-update.timer starts, and the helper's update order
// starts the same service by name, with --no-block. Until 13-16 those two
// units existed only as what an operator wrote by hand; now both ship in
// deploy/ and the release archive, and UpdateUnitInstallCommands installs
// them. As with the helper, nothing installs or replaces them but the
// operator: the update script replaces the daemon's binary and itself, never
// a unit, so a newer unit comes from repeating the install commands with a
// newer archive.
//
// The update order needs the service: without it the helper's
// `systemctl start --no-block` queues a start for a unit that does not exist
// and records failed. UpdateUnitMissing asks for UpdateUnitPath the way
// Outdated asks for the check unit: a regular file, read through fs.Stat, so a
// symlink counts by what it points to -- a unit linked from elsewhere
// (systemctl link) is installed, a mask (a symlink to /dev/null) or a link to
// nothing is not. It looks only in /etc/systemd/system, where the install
// commands put it and where Detect looks for the helper's units; a unit kept
// in another directory of systemd's search path is reported missing. Only
// update needs it: check-update starts the check unit, and the other three no
// update unit at all. Like the update script, it is asked whatever Detect
// says, because the helper's install commands do not bring it.

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
	// UpdateScriptMissing asks for it, and UpdateScriptInstallCommands
	// installs it.
	UpdateScriptPath = "/usr/local/sbin/holzkube-manager-update"
	// UpdateUnitPath is the hourly update's service, installed from
	// deploy/holzkube-manager-update.service by the operator, never by the
	// update script. It runs UpdateScriptPath with no argument, as root. The
	// timer at UpdateTimerPath starts it hourly, and the helper's update
	// order starts it by name with --no-block, so nobody waits for it.
	UpdateUnitPath = "/etc/systemd/system/holzkube-manager-update.service"
	// UpdateTimerPath is the timer that starts UpdateUnitPath's service five
	// minutes after boot and then an hour after its last start, installed
	// from deploy/holzkube-manager-update.timer by the operator, never by the
	// update script. It is the one of the two that is enabled.
	UpdateTimerPath = "/etc/systemd/system/holzkube-manager-update.timer"
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

// UpdateScriptInstallCommands installs the update script, run from the root
// of an unpacked release archive (or a checkout): the archive carries
// deploy/holzkube-manager-update.sh. The host page shows it while the script
// is missing, and deploy/HOST-HELPER.md carries the same line, held to this
// slice byte for byte by a test.
var UpdateScriptInstallCommands = []string{
	"sudo install -o root -g root -m 0755 deploy/holzkube-manager-update.sh /usr/local/sbin/holzkube-manager-update",
}

// UpdateUnitInstallCommands installs the hourly update's two units, run from
// the root of an unpacked release archive (or a checkout): the archive
// carries deploy/holzkube-manager-update.service and
// deploy/holzkube-manager-update.timer.
//
// The first line exists because the service makes
// /usr/local/lib/holzkube-manager writable -- the update script keeps the
// previous binary there -- but under ProtectSystem=strict cannot create it:
// its ReadWritePaths= entry carries "-", so a directory that does not exist
// is skipped, /usr/local/lib stays read-only, and the script's own install -d
// of it fails. Then both units
// go into /etc/systemd/system, daemon-reload makes systemd see them, and only
// the timer is enabled; the service has no [Install] section, and only the
// timer and the helper start it.
//
// The lines replace units of the same name an operator wrote by hand. This
// is the one copy: deploy/HOST-HELPER.md and docs/guide.md carry the same
// lines, held to this slice byte for byte by a test -- a test and not an
// embed, as for InstallCommands, because a package under internal/ cannot
// embed a file from deploy/.
var UpdateUnitInstallCommands = []string{
	"sudo install -d -o root -g root -m 0755 /usr/local/lib/holzkube-manager",
	"sudo install -o root -g root -m 0644 deploy/holzkube-manager-update.service deploy/holzkube-manager-update.timer /etc/systemd/system/",
	"sudo systemctl daemon-reload",
	"sudo systemctl enable --now holzkube-manager-update.timer",
}

// MissingUpdateUnit is the item UpdateUnitMissing reports: the hourly
// update's service is not a regular file at UpdateUnitPath -- absent, a
// directory, masked (a symlink to /dev/null) or a symlink to nothing.
const MissingUpdateUnit = "update-unit"

// NeedsUpdateUnit reports whether the order a starts the hourly update's
// service: update only. check-update starts the check unit, which Outdated
// asks for. The routes refuse exactly update while UpdateUnitMissing reports
// anything.
func NeedsUpdateUnit(a Action) bool {
	return a == Update
}

// MissingUpdateScript is the item UpdateScriptMissing reports: the update
// script is absent, or not something root may run as root (not regular, not
// executable, not owned by uid 0, or writable by group or other).
const MissingUpdateScript = "update-script"

// NeedsUpdateScript reports whether the order a ends in the update script:
// update and check-update. The routes refuse exactly these two while
// UpdateScriptMissing reports anything.
func NeedsUpdateScript(a Action) bool {
	return a == Update || a == CheckUpdate
}

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
	// Outdated's list, OutdatedScript or OutdatedCheckUnit, in
	// UpdateScriptMissing's, MissingUpdateScript, and in UpdateUnitMissing's,
	// MissingUpdateUnit.
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
	return rootExecutable(fsys, HelperScriptPath)
}

// rootExecutable reports whether the file at path is one root may run: a
// regular file, executable, writable by nobody but its owner, and owned by
// uid 0. An owner that cannot be told is no proof, and counts as no.
func rootExecutable(fsys fs.FS, path string) bool {
	info, err := fs.Stat(fsys, fsName(path))
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

// UpdateScriptMissing reports whether the update script both update orders
// end in is missing on the machine fsys is rooted at ("/"): one
// MissingUpdateScript item when UpdateScriptPath is not a regular executable
// file owned by uid 0 and writable by nobody else, else an empty list. It
// never returns nil, and never reads the file.
func UpdateScriptMissing(fsys fs.FS) []Missing {
	if rootExecutable(fsys, UpdateScriptPath) {
		return []Missing{}
	}
	return []Missing{{Item: MissingUpdateScript, Path: UpdateScriptPath}}
}

// UpdateScript reports whether the update script is missing, read through the
// Box's FS. Empty, never nil, when it is installed.
func (b *Box) UpdateScript() []Missing {
	return UpdateScriptMissing(b.cfg.FS)
}

// UpdateUnitMissing reports whether the hourly update's service, which the
// update order starts, is missing on the machine fsys is rooted at ("/"): one
// MissingUpdateUnit item when UpdateUnitPath is not a regular file once
// symlinks are followed, else an empty list. It looks only at
// /etc/systemd/system, never reads the file, and never returns nil.
func UpdateUnitMissing(fsys fs.FS) []Missing {
	// fs.Stat, not fs.Lstat: systemctl link leaves a symlink to a real unit,
	// which is installed; systemctl mask leaves one to /dev/null, which is
	// not a regular file and so not installed.
	if info, err := fs.Stat(fsys, fsName(UpdateUnitPath)); err == nil && info.Mode().IsRegular() {
		return []Missing{}
	}
	return []Missing{{Item: MissingUpdateUnit, Path: UpdateUnitPath}}
}

// UpdateUnit reports whether the hourly update's service is missing, read
// through the Box's FS. Empty, never nil, when it is installed.
func (b *Box) UpdateUnit() []Missing {
	return UpdateUnitMissing(b.cfg.FS)
}
