package hostaction

import (
	"io/fs"
	"reflect"
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
