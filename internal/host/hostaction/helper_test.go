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
		{"nothing installed", fstest.MapFS{}, []Missing{script, pathUnit, notEnabled}},

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
	if got := NewBox(Config{FS: fstest.MapFS{}, DataDir: t.TempDir()}).Missing(); len(got) != 3 {
		t.Errorf("Box over an empty FS: Missing = %+v, want three items", got)
	}
}

// TestInstallCommandsAreTheGuidesBlock holds the shape of the one copy of the
// install commands: four lines, each run with sudo, installing to exactly the
// paths Detect looks at.
func TestInstallCommandsAreTheGuidesBlock(t *testing.T) {
	t.Parallel()

	want := []string{
		"sudo install -o root -g root -m 0755 deploy/holzkube-manager-host.sh " + HelperScriptPath,
		"sudo install -o root -g root -m 0644 deploy/holzkube-manager-host.path deploy/holzkube-manager-host.service /etc/systemd/system/",
		"sudo systemctl daemon-reload",
		"sudo systemctl enable --now holzkube-manager-host.path",
	}
	if !reflect.DeepEqual(InstallCommands, want) {
		t.Errorf("InstallCommands =\n%s\nwant\n%s", strings.Join(InstallCommands, "\n"), strings.Join(want, "\n"))
	}
	for _, p := range []string{PathUnitPath, ServiceUnitPath, WantsLinkPath} {
		if !strings.HasPrefix(p, "/etc/systemd/system/") {
			t.Errorf("%s is not where the second command installs the units", p)
		}
	}
}
