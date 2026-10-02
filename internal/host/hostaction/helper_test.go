package hostaction

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
)

// installedFS is a root filesystem on which the helper is installed the way
// deploy/HOST-HELPER.md installs it: the script regular, 0755 and owned by uid
// 0, both unit files, and the path unit enabled (its paths.target.wants
// symlink).
func installedFS() fstest.MapFS {
	return fstest.MapFS{
		fsName(HelperScriptPath): {
			Data: []byte("#!/usr/bin/env bash\n"),
			Mode: 0o755,
			Sys:  &syscall.Stat_t{Uid: 0, Gid: 0},
		},
		fsName(PathUnitPath):    {Data: []byte("[Path]\n"), Mode: 0o644},
		fsName(ServiceUnitPath): {Data: []byte("[Service]\n"), Mode: 0o644},
		fsName(WantsLinkPath):   {Data: []byte(PathUnitPath), Mode: fs.ModeSymlink | 0o777},
	}
}

func TestHelper(t *testing.T) {
	t.Parallel()

	script := Missing{Item: MissingScript, Path: HelperScriptPath}
	pathUnit := Missing{Item: MissingPathUnit, Path: PathUnitPath}
	serviceUnit := Missing{Item: MissingPathUnit, Path: ServiceUnitPath}
	notEnabled := Missing{Item: MissingNotEnabled, Path: WantsLinkPath}

	// with returns the installed fixture changed by edit.
	with := func(edit func(m fstest.MapFS)) fstest.MapFS {
		m := installedFS()
		edit(m)
		return m
	}
	scriptAs := func(f *fstest.MapFile) func(fstest.MapFS) {
		return func(m fstest.MapFS) { m[fsName(HelperScriptPath)] = f }
	}
	dir := &fstest.MapFile{Mode: fs.ModeDir | 0o755}

	cases := []struct {
		name string
		fsys fs.FS
		want []Missing
	}{
		{"installed", installedFS(), []Missing{}},
		// A unit that is not installed cannot be "installed but not enabled":
		// while a unit file is missing, path-unit is the whole answer for the
		// pair, and the install commands enable it in the same run (G-13-2).
		{"nothing installed", fstest.MapFS{}, []Missing{script, pathUnit}},
		{"script installed, no unit files, not enabled", with(func(m fstest.MapFS) {
			delete(m, fsName(PathUnitPath))
			delete(m, fsName(ServiceUnitPath))
			delete(m, fsName(WantsLinkPath))
		}), []Missing{pathUnit}},
		{".service absent, not enabled", with(func(m fstest.MapFS) {
			delete(m, fsName(ServiceUnitPath))
			delete(m, fsName(WantsLinkPath))
		}), []Missing{serviceUnit}},
		{"script absent, both units, not enabled", with(func(m fstest.MapFS) {
			delete(m, fsName(HelperScriptPath))
			delete(m, fsName(WantsLinkPath))
		}), []Missing{script, notEnabled}},
		// A dangling wants link does not make a unit installed.
		{"enabled, both unit files absent", with(func(m fstest.MapFS) {
			delete(m, fsName(PathUnitPath))
			delete(m, fsName(ServiceUnitPath))
		}), []Missing{pathUnit}},

		{"script absent", with(func(m fstest.MapFS) { delete(m, fsName(HelperScriptPath)) }), []Missing{script}},
		{"script is a directory", with(scriptAs(&fstest.MapFile{Mode: fs.ModeDir | 0o755, Sys: &syscall.Stat_t{}})), []Missing{script}},
		{"script mode 0644", with(scriptAs(&fstest.MapFile{Mode: 0o644, Sys: &syscall.Stat_t{}})), []Missing{script}},
		{"script mode 0775", with(scriptAs(&fstest.MapFile{Mode: 0o775, Sys: &syscall.Stat_t{}})), []Missing{script}},
		{"script mode 0757", with(scriptAs(&fstest.MapFile{Mode: 0o757, Sys: &syscall.Stat_t{}})), []Missing{script}},
		{"script owned by uid 1000", with(scriptAs(&fstest.MapFile{Mode: 0o755, Sys: &syscall.Stat_t{Uid: 1000}})), []Missing{script}},
		{"script with no owner information", with(scriptAs(&fstest.MapFile{Mode: 0o755})), []Missing{script}},

		{".path present, .service absent", with(func(m fstest.MapFS) { delete(m, fsName(ServiceUnitPath)) }), []Missing{serviceUnit}},
		{".service present, .path absent", with(func(m fstest.MapFS) { delete(m, fsName(PathUnitPath)) }), []Missing{pathUnit}},
		{".path is a directory", with(func(m fstest.MapFS) { m[fsName(PathUnitPath)] = dir }), []Missing{pathUnit}},
		{".service is a directory", with(func(m fstest.MapFS) { m[fsName(ServiceUnitPath)] = dir }), []Missing{serviceUnit}},

		{"both units, not enabled", with(func(m fstest.MapFS) { delete(m, fsName(WantsLinkPath)) }), []Missing{notEnabled}},
		// systemctl enable makes a symlink; a copied file under the same name
		// enables the unit as well, and is not called missing.
		{"enabled by a copied file", with(func(m fstest.MapFS) {
			m[fsName(WantsLinkPath)] = &fstest.MapFile{Data: []byte("[Path]\n"), Mode: 0o644}
		}), []Missing{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Detect(tc.fsys)
			if got == nil {
				t.Fatalf("Detect returned nil; the answer's missing must be a list, never null")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Detect = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestHelperBoxMissing: the Box reads through its own FS.
func TestHelperBoxMissing(t *testing.T) {
	t.Parallel()

	if got := NewBox(Config{FS: installedFS(), DataDir: t.TempDir()}).Missing(); got == nil || len(got) != 0 {
		t.Errorf("Box over the installed fixture: Missing = %#v, want an empty list", got)
	}
	want := []Missing{
		{Item: MissingScript, Path: HelperScriptPath},
		{Item: MissingPathUnit, Path: PathUnitPath},
	}
	if got := NewBox(Config{FS: fstest.MapFS{}, DataDir: t.TempDir()}).Missing(); !reflect.DeepEqual(got, want) {
		t.Errorf("Box over an empty FS: Missing = %+v, want %+v", got, want)
	}
}

// TestHelperEveryCombination walks all 16 presence combinations of the four
// entries Detect reads -- the script, the .path unit, the .service unit and
// the paths.target.wants entry -- and holds three things in each (G-13-2,
// D-12):
//
//  1. path-unit and not-enabled are never listed together: a unit that is not
//     installed is not "installed but not enabled";
//  2. not-enabled is listed exactly when both unit files are there and the
//     wants entry is not;
//  3. the list is empty exactly when all four are there, so actions.available
//     and the routes' 409 are what they were before the fix in every state.
func TestHelperEveryCombination(t *testing.T) {
	t.Parallel()

	entries := []string{
		fsName(HelperScriptPath),
		fsName(PathUnitPath),
		fsName(ServiceUnitPath),
		fsName(WantsLinkPath),
	}
	for mask := 0; mask < 1<<len(entries); mask++ {
		present := func(i int) bool { return mask&(1<<i) != 0 }
		m := installedFS()
		for i, name := range entries {
			if !present(i) {
				delete(m, name)
			}
		}
		script, path, service, wants := present(0), present(1), present(2), present(3)

		got := Detect(m)
		has := func(item string) bool {
			for _, g := range got {
				if g.Item == item {
					return true
				}
			}
			return false
		}
		state := func() string {
			return strings.Join([]string{
				"script=" + yes(script), ".path=" + yes(path), ".service=" + yes(service), "wants=" + yes(wants),
			}, " ")
		}

		if has(MissingPathUnit) && has(MissingNotEnabled) {
			t.Errorf("%s: Detect = %+v lists path-unit and not-enabled together", state(), got)
		}
		if wantNE := path && service && !wants; has(MissingNotEnabled) != wantNE {
			t.Errorf("%s: Detect = %+v; not-enabled listed = %v, want %v", state(), got, has(MissingNotEnabled), wantNE)
		}
		if all := script && path && service && wants; (len(got) == 0) != all {
			t.Errorf("%s: Detect = %+v; empty = %v, want %v (empty exactly when all is installed)", state(), got, len(got) == 0, all)
		}
	}
}

func yes(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// TestInstallCommandsAreTheGuidesBlock holds the shape of the one copy of the
// install commands: four lines, each run with sudo, installing to exactly the
// paths Detect looks at.
func TestInstallCommandsAreTheGuidesBlock(t *testing.T) {
	t.Parallel()

	want := []string{
		"sudo install -o root -g root -m 0755 deploy/holzkube-manager-host.sh " + HelperScriptPath,
		"sudo install -o root -g root -m 0644 deploy/holzkube-manager-host.path deploy/holzkube-manager-host.service deploy/holzkube-manager-update-check.service /etc/systemd/system/",
		"sudo systemctl daemon-reload",
		"sudo systemctl enable --now holzkube-manager-host.path",
	}
	if !reflect.DeepEqual(InstallCommands, want) {
		t.Errorf("InstallCommands =\n%s\nwant\n%s", strings.Join(InstallCommands, "\n"), strings.Join(want, "\n"))
	}
	for _, p := range []string{PathUnitPath, ServiceUnitPath, UpdateCheckUnitPath, WantsLinkPath} {
		if !strings.HasPrefix(p, "/etc/systemd/system/") {
			t.Errorf("%s is not where the second command installs the units", p)
		}
	}
}

// shippedScriptBytes and olderScript are the helper's bytes as this release
// ships them and as they were before check-update existed (frozen at 8b64a06).
func shippedScriptBytes(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(shippedScript)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func olderScript(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(olderHelper)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// rootScript serves data as the helper script Detect calls installed:
// regular, 0755, owned by uid 0.
func rootScript(data []byte) *fstest.MapFile {
	return &fstest.MapFile{Data: data, Mode: 0o755, Sys: &syscall.Stat_t{Uid: 0, Gid: 0}}
}

// checkUnit is the check unit as the install commands put it there.
func checkUnit() *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("[Service]\nType=oneshot\n"), Mode: 0o644}
}

// TestKnownOrders: the daemon learns which orders the installed helper carries
// out from the one marker line, and a helper without it is the helper of
// 13-01, which knows the first four (D-12: read from the file, no D-Bus).
func TestKnownOrders(t *testing.T) {
	t.Parallel()

	five := Actions()
	four := []Action{Reboot, Poweroff, RestartService, Update}
	shipped := shippedScriptBytes(t)

	over := func(data []byte) fs.FS {
		return fstest.MapFS{fsName(HelperScriptPath): rootScript(data)}
	}
	marker := func(words string) []byte {
		return []byte("#!/usr/bin/env bash\n" + HelperOrdersMarker + words + "\nexit 0\n")
	}
	tooLarge := append(slices.Clone(shipped), []byte("# "+strings.Repeat("x", MaxHelperScriptSize)+"\n")...)

	cases := []struct {
		name string
		fsys fs.FS
		want []Action
	}{
		{"the shipped script", over(shipped), five},
		{"the helper before the check, byte for byte", over(olderScript(t)), four},
		{"a word this daemon does not know is ignored", over(marker("reboot halt poweroff restart-service update check-update")), five},
		{"a marker without check-update, in its own order", over(marker("update reboot restart-service poweroff")), four},
		{"two marker lines", over(append(marker("reboot poweroff restart-service update check-update"),
			[]byte(HelperOrdersMarker+"reboot poweroff restart-service update check-update\n")...)), four},
		{"the marker only inside a line", over([]byte("#!/usr/bin/env bash\necho '" + HelperOrdersMarker + "check-update'\n")), four},
		{"larger than MaxHelperScriptSize", over(tooLarge), four},
		{"no script", fstest.MapFS{}, four},
		{"the script is a directory", fstest.MapFS{fsName(HelperScriptPath): {Mode: fs.ModeDir | 0o755, Sys: &syscall.Stat_t{}}}, four},
		// Read only once Detect would call it root's: a script somebody
		// else may change says nothing about what root runs.
		{"the shipped script owned by uid 1000", fstest.MapFS{fsName(HelperScriptPath): {
			Data: shipped, Mode: 0o755, Sys: &syscall.Stat_t{Uid: 1000},
		}}, four},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := KnownOrders(tc.fsys); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("KnownOrders = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestOutdated: what the check needs that is not there -- a helper that knows
// the order, and the unit it starts -- script first, never nil.
func TestOutdated(t *testing.T) {
	t.Parallel()

	script := Missing{Item: OutdatedScript, Path: HelperScriptPath}
	unit := Missing{Item: OutdatedCheckUnit, Path: UpdateCheckUnitPath}
	shipped, older := shippedScriptBytes(t), olderScript(t)

	with := func(data []byte, u *fstest.MapFile) fstest.MapFS {
		m := fstest.MapFS{fsName(HelperScriptPath): rootScript(data)}
		if u != nil {
			m[fsName(UpdateCheckUnitPath)] = u
		}
		return m
	}
	cases := []struct {
		name string
		fsys fstest.MapFS
		want []Missing
	}{
		{"the shipped script and the check unit", with(shipped, checkUnit()), []Missing{}},
		{"the helper before the check", with(older, checkUnit()), []Missing{script}},
		{"the shipped script, no check unit", with(shipped, nil), []Missing{unit}},
		{"the shipped script, a directory for the check unit", with(shipped, &fstest.MapFile{Mode: fs.ModeDir | 0o755}), []Missing{unit}},
		{"the helper before the check, no check unit", with(older, nil), []Missing{script, unit}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Outdated(tc.fsys)
			if got == nil {
				t.Fatal("Outdated returned nil; the answer's outdated must be a list, never null")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Outdated = %+v, want %+v", got, tc.want)
			}
			if box := NewBox(Config{FS: tc.fsys, DataDir: t.TempDir()}).Outdated(); !reflect.DeepEqual(box, tc.want) {
				t.Errorf("Box.Outdated = %+v, want %+v", box, tc.want)
			}
		})
	}
}

// TestUpdateScriptMissing (13-REVIEW-2 IN-04): both update orders end in the
// update script, and the daemon asks for it the way it asks for the helper's
// script -- a regular executable file owned by uid 0 and writable by nobody
// else -- from the file alone. Anything else is one update-script item.
func TestUpdateScriptMissing(t *testing.T) {
	t.Parallel()

	missing := []Missing{{Item: MissingUpdateScript, Path: UpdateScriptPath}}
	at := func(f *fstest.MapFile) fstest.MapFS {
		return fstest.MapFS{fsName(UpdateScriptPath): f}
	}
	root := func(mode fs.FileMode) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte("#!/usr/bin/env bash\n"), Mode: mode, Sys: &syscall.Stat_t{Uid: 0, Gid: 0}}
	}

	cases := []struct {
		name string
		fsys fstest.MapFS
		want []Missing
	}{
		{"installed as the update script installs itself: root, 0755", at(root(0o755)), []Missing{}},
		{"root, 0700", at(root(0o700)), []Missing{}},
		{"absent", fstest.MapFS{}, missing},
		// The helper's files say nothing about it: the install commands do
		// not bring it.
		{"absent, the helper installed", installedFS(), missing},
		{"a directory", at(&fstest.MapFile{Mode: fs.ModeDir | 0o755, Sys: &syscall.Stat_t{}}), missing},
		{"mode 0644", at(root(0o644)), missing},
		{"mode 0775", at(root(0o775)), missing},
		{"mode 0757", at(root(0o757)), missing},
		{"owned by uid 1000", at(&fstest.MapFile{Mode: 0o755, Sys: &syscall.Stat_t{Uid: 1000}}), missing},
		{"no owner information", at(&fstest.MapFile{Mode: 0o755}), missing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := UpdateScriptMissing(tc.fsys)
			if got == nil {
				t.Fatal("UpdateScriptMissing returned nil; the answer's update_script must be a list, never null")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("UpdateScriptMissing = %+v, want %+v", got, tc.want)
			}
			if box := NewBox(Config{FS: tc.fsys, DataDir: t.TempDir()}).UpdateScript(); !reflect.DeepEqual(box, tc.want) {
				t.Errorf("Box.UpdateScript = %+v, want %+v", box, tc.want)
			}
		})
	}
}

// TestNeedsUpdateScript: exactly the two update orders end in the update
// script; reboot, poweroff and restart-service do not.
func TestNeedsUpdateScript(t *testing.T) {
	t.Parallel()

	var need []Action
	for _, a := range Actions() {
		if NeedsUpdateScript(a) {
			need = append(need, a)
		}
	}
	if want := []Action{Update, CheckUpdate}; !reflect.DeepEqual(need, want) {
		t.Errorf("NeedsUpdateScript is true for %v, want exactly %v", need, want)
	}
}

// TestUpdateUnitMissing (13-16): the update order starts
// holzkube-manager-update.service, which the helper's install commands do not
// install. The daemon asks for its unit file where the hourly update's install
// commands put it, /etc/systemd/system, from the file alone: a regular file is
// installed, wherever a symlink to it points (systemctl link); anything else --
// absent, a directory, a mask (a symlink to /dev/null), a symlink to nothing --
// is one update-unit item.
//
// The symlink cases are real symlinks in a temporary directory, read through
// os.DirFS, so fs.Stat follows them as it does on the host. Fault injected and
// seen red: fs.Lstat in place of fs.Stat (the systemctl-link case).
func TestUpdateUnitMissing(t *testing.T) {
	t.Parallel()

	missing := []Missing{{Item: MissingUpdateUnit, Path: UpdateUnitPath}}
	unit := &fstest.MapFile{Data: []byte("[Service]\nType=oneshot\n"), Mode: 0o644}

	// onDisk is a root with the unit's directory and, if put is set, what
	// put puts under the unit's name (given its absolute path on disk).
	onDisk := func(t *testing.T, put func(t *testing.T, root, unitFile string)) fs.FS {
		t.Helper()
		root := t.TempDir()
		dir := filepath.Join(root, filepath.FromSlash(fsName(filepath.Dir(UpdateUnitPath))))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if put != nil {
			put(t, root, filepath.Join(dir, filepath.Base(UpdateUnitPath)))
		}
		return os.DirFS(root)
	}
	symlink := func(target func(root string) string) func(t *testing.T, root, unitFile string) {
		return func(t *testing.T, root, unitFile string) {
			t.Helper()
			if err := os.Symlink(target(root), unitFile); err != nil {
				t.Fatal(err)
			}
		}
	}

	withUnit := installedFS()
	withUnit[fsName(UpdateScriptPath)] = &fstest.MapFile{Data: []byte("#!/usr/bin/env bash\n"), Mode: 0o755, Sys: &syscall.Stat_t{Uid: 0}}
	helperAndScript := installedFS()
	helperAndScript[fsName(UpdateScriptPath)] = withUnit[fsName(UpdateScriptPath)]
	withUnit[fsName(UpdateUnitPath)] = unit

	cases := []struct {
		name string
		fsys func(t *testing.T) fs.FS
		want []Missing
	}{
		{"installed: a regular file", func(*testing.T) fs.FS { return fstest.MapFS{fsName(UpdateUnitPath): unit} }, []Missing{}},
		{"installed beside the helper and the update script", func(*testing.T) fs.FS { return withUnit }, []Missing{}},
		{"a regular file on disk", func(t *testing.T) fs.FS {
			return onDisk(t, func(t *testing.T, _, unitFile string) {
				t.Helper()
				if err := os.WriteFile(unitFile, unit.Data, 0o644); err != nil {
					t.Fatal(err)
				}
			})
		}, []Missing{}},
		// systemctl link: the name in /etc/systemd/system points at a unit
		// file elsewhere, which systemd loads.
		{"a symlink to a regular unit elsewhere", func(t *testing.T) fs.FS {
			return onDisk(t, func(t *testing.T, root, unitFile string) {
				t.Helper()
				elsewhere := filepath.Join(root, "opt", filepath.Base(UpdateUnitPath))
				if err := os.MkdirAll(filepath.Dir(elsewhere), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(elsewhere, unit.Data, 0o644); err != nil {
					t.Fatal(err)
				}
				symlink(func(string) string { return elsewhere })(t, root, unitFile)
			})
		}, []Missing{}},
		{"absent", func(*testing.T) fs.FS { return fstest.MapFS{} }, missing},
		{"absent on disk", func(t *testing.T) fs.FS { return onDisk(t, nil) }, missing},
		// The helper's install commands and the update script's do not bring
		// it.
		{"absent, the helper and the update script installed", func(*testing.T) fs.FS { return helperAndScript }, missing},
		{"a directory", func(t *testing.T) fs.FS {
			return onDisk(t, func(t *testing.T, _, unitFile string) {
				t.Helper()
				if err := os.Mkdir(unitFile, 0o755); err != nil {
					t.Fatal(err)
				}
			})
		}, missing},
		// systemctl mask: a symlink to /dev/null, which systemd refuses to
		// start.
		{"masked: a symlink to /dev/null", func(t *testing.T) fs.FS {
			return onDisk(t, symlink(func(string) string { return "/dev/null" }))
		}, missing},
		{"a symlink to nothing", func(t *testing.T) fs.FS {
			return onDisk(t, symlink(func(root string) string { return filepath.Join(root, "gone") }))
		}, missing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fsys := tc.fsys(t)
			got := UpdateUnitMissing(fsys)
			if got == nil {
				t.Fatal("UpdateUnitMissing returned nil; it must be a list, never null")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("UpdateUnitMissing = %+v, want %+v", got, tc.want)
			}
			if box := NewBox(Config{FS: fsys, DataDir: t.TempDir()}).UpdateUnit(); !reflect.DeepEqual(box, tc.want) {
				t.Errorf("Box.UpdateUnit = %+v, want %+v", box, tc.want)
			}
		})
	}
}

// TestUpdateTimerMissing: the timer that starts the hourly update, read from
// files as the helper's path unit is (13-REVIEW-3 WR-03). With the service
// installed and the timer absent, masked or not enabled, Check for updates
// and install works and nothing runs hourly; the page says so. The timer file
// is read through symlinks (systemctl link), the enablement link is not
// (Detect's paths.target.wants link is read the same way).
//
// Faults injected and seen red (13-REVIEW-3): the wants link not asked;
// fs.Lstat for the timer file; the timer not asked at all.
func TestUpdateTimerMissing(t *testing.T) {
	t.Parallel()

	timer := &fstest.MapFile{Data: []byte("[Timer]\nOnUnitActiveSec=1h\n"), Mode: 0o644}
	link := &fstest.MapFile{Data: []byte(""), Mode: fs.ModeSymlink | 0o777}
	notInstalled := []Missing{{Item: MissingUpdateTimer, Path: UpdateTimerPath}}
	notEnabled := []Missing{{Item: MissingUpdateTimerNotEnabled, Path: UpdateTimerWantsLinkPath}}

	// onDisk is a root with the timer's directory and its wants directory,
	// and whatever put puts there.
	onDisk := func(t *testing.T, put func(t *testing.T, root, timerFile, wantsLink string)) fs.FS {
		t.Helper()
		root := t.TempDir()
		timerFile := filepath.Join(root, filepath.FromSlash(fsName(UpdateTimerPath)))
		wantsLink := filepath.Join(root, filepath.FromSlash(fsName(UpdateTimerWantsLinkPath)))
		for _, d := range []string{filepath.Dir(timerFile), filepath.Dir(wantsLink)} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		put(t, root, timerFile, wantsLink)
		return os.DirFS(root)
	}

	cases := []struct {
		name string
		fsys func(t *testing.T) fs.FS
		want []Missing
	}{
		{"installed and enabled", func(*testing.T) fs.FS {
			return fstest.MapFS{fsName(UpdateTimerPath): timer, fsName(UpdateTimerWantsLinkPath): link}
		}, []Missing{}},
		{"installed and enabled, on disk", func(t *testing.T) fs.FS {
			return onDisk(t, func(t *testing.T, _, timerFile, wantsLink string) {
				t.Helper()
				if err := os.WriteFile(timerFile, timer.Data, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("../holzkube-manager-update.timer", wantsLink); err != nil {
					t.Fatal(err)
				}
			})
		}, []Missing{}},
		{"linked from elsewhere (systemctl link) and enabled", func(t *testing.T) fs.FS {
			return onDisk(t, func(t *testing.T, root, timerFile, wantsLink string) {
				t.Helper()
				elsewhere := filepath.Join(root, "opt", filepath.Base(UpdateTimerPath))
				if err := os.MkdirAll(filepath.Dir(elsewhere), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(elsewhere, timer.Data, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(elsewhere, timerFile); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(elsewhere, wantsLink); err != nil {
					t.Fatal(err)
				}
			})
		}, []Missing{}},
		{"installed, not enabled", func(*testing.T) fs.FS {
			return fstest.MapFS{fsName(UpdateTimerPath): timer}
		}, notEnabled},
		// The first line of the removal block, disable --now, alone.
		{"installed, disabled on disk", func(t *testing.T) fs.FS {
			return onDisk(t, func(t *testing.T, _, timerFile, _ string) {
				t.Helper()
				if err := os.WriteFile(timerFile, timer.Data, 0o644); err != nil {
					t.Fatal(err)
				}
			})
		}, notEnabled},
		{"absent", func(*testing.T) fs.FS { return fstest.MapFS{} }, notInstalled},
		// An enablement link left behind without the file is still no timer.
		{"absent, a wants link left behind", func(*testing.T) fs.FS {
			return fstest.MapFS{fsName(UpdateTimerWantsLinkPath): link}
		}, notInstalled},
		{"masked: a symlink to /dev/null", func(t *testing.T) fs.FS {
			return onDisk(t, func(t *testing.T, _, timerFile, wantsLink string) {
				t.Helper()
				if err := os.Symlink("/dev/null", timerFile); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("../holzkube-manager-update.timer", wantsLink); err != nil {
					t.Fatal(err)
				}
			})
		}, notInstalled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fsys := tc.fsys(t)
			got := UpdateTimerMissing(fsys)
			if got == nil {
				t.Fatal("UpdateTimerMissing returned nil; it must be a list, never null")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("UpdateTimerMissing = %+v, want %+v", got, tc.want)
			}
			if box := NewBox(Config{FS: fsys, DataDir: t.TempDir()}).UpdateTimer(); !reflect.DeepEqual(box, tc.want) {
				t.Errorf("Box.UpdateTimer = %+v, want %+v", box, tc.want)
			}
		})
	}

	// The wants link is the one systemctl enable makes for the shipped
	// timer's WantedBy=.
	unit := readUnitFile(t, shippedUpdateTimer)
	for _, a := range unit.assignments {
		if a.section == "Install" && a.key == "WantedBy" {
			if want := "/etc/systemd/system/" + a.value + ".wants/" + filepath.Base(UpdateTimerPath); want != UpdateTimerWantsLinkPath {
				t.Errorf("UpdateTimerWantsLinkPath = %s, the shipped timer's WantedBy=%s makes %s", UpdateTimerWantsLinkPath, a.value, want)
			}
		}
	}
}

// TestNeedsUpdateUnit: only update starts holzkube-manager-update.service.
// check-update starts the check unit, which Outdated already asks for, and
// the other three start no update unit at all.
func TestNeedsUpdateUnit(t *testing.T) {
	t.Parallel()

	var need []Action
	for _, a := range Actions() {
		if NeedsUpdateUnit(a) {
			need = append(need, a)
		}
	}
	if want := []Action{Update}; !reflect.DeepEqual(need, want) {
		t.Errorf("NeedsUpdateUnit is true for %v, want exactly %v", need, want)
	}
}
