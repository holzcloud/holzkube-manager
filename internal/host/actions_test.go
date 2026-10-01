package host

import (
	"context"
	"io/fs"
	"os"
	"reflect"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/host/hostaction"
)

// helperInstalledFS is a root filesystem on which the helper is installed the
// way deploy/HOST-HELPER.md installs it: the shipped script's own bytes, both
// unit files, the path unit enabled, and the check unit -- beside the update
// script both update orders end in, as the reference installation has it.
func helperInstalledFS(t *testing.T) fstest.MapFS {
	t.Helper()
	return fstest.MapFS{
		"usr/local/sbin/holzkube-manager-host": {
			Data: helperScript(t, "../../deploy/holzkube-manager-host.sh"), Mode: 0o755, Sys: &syscall.Stat_t{Uid: 0, Gid: 0},
		},
		"etc/systemd/system/holzkube-manager-host.path":                    {Data: []byte("[Path]\n"), Mode: 0o644},
		"etc/systemd/system/holzkube-manager-host.service":                 {Data: []byte("[Service]\n"), Mode: 0o644},
		"etc/systemd/system/paths.target.wants/holzkube-manager-host.path": {Data: []byte(hostaction.PathUnitPath), Mode: fs.ModeSymlink | 0o777},
		"etc/systemd/system/holzkube-manager-update-check.service":         {Data: []byte("[Service]\n"), Mode: 0o644},
		"usr/local/sbin/holzkube-manager-update": {
			Data: []byte("#!/usr/bin/env bash\n"), Mode: 0o755, Sys: &syscall.Stat_t{Uid: 0, Gid: 0},
		},
	}
}

// helperScript reads a helper script's bytes from the repository.
func helperScript(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// olderHelperFS is helperInstalledFS with the helper as it was before the
// check (frozen at 8b64a06): installed completely, and too old for the check.
func olderHelperFS(t *testing.T) fstest.MapFS {
	t.Helper()
	m := helperInstalledFS(t)
	m["usr/local/sbin/holzkube-manager-host"] = &fstest.MapFile{
		Data: helperScript(t, "hostaction/testdata/holzkube-manager-host-before-check.sh"),
		Mode: 0o755, Sys: &syscall.Stat_t{Uid: 0, Gid: 0},
	}
	return m
}

// TestReadCarriesActions: one answer says whether the page may offer the host
// actions and, if not, exactly what is missing and what to run (D-12, D-14).
func TestReadCarriesActions(t *testing.T) {
	t.Parallel()

	inContainer := helperInstalledFS(t)
	inContainer[".dockerenv"] = &fstest.MapFile{}
	noCheckUnit := helperInstalledFS(t)
	delete(noCheckUnit, "etc/systemd/system/holzkube-manager-update-check.service")
	// The older helper with a piece missing: missing is the whole answer,
	// since the install commands install everything, the newer script too.
	olderNotEnabled := olderHelperFS(t)
	delete(olderNotEnabled, "etc/systemd/system/paths.target.wants/holzkube-manager-host.path")

	noUpdateScript := helperInstalledFS(t)
	delete(noUpdateScript, "usr/local/sbin/holzkube-manager-update")
	updateScriptNotRoots := helperInstalledFS(t)
	updateScriptNotRoots["usr/local/sbin/holzkube-manager-update"] = &fstest.MapFile{
		Data: []byte("#!/usr/bin/env bash\n"), Mode: 0o755, Sys: &syscall.Stat_t{Uid: 1000},
	}
	updateScript := []hostaction.Missing{{Item: hostaction.MissingUpdateScript, Path: hostaction.UpdateScriptPath}}

	scriptOutdated := hostaction.Missing{Item: hostaction.OutdatedScript, Path: hostaction.HelperScriptPath}
	checkUnit := hostaction.Missing{Item: hostaction.OutdatedCheckUnit, Path: hostaction.UpdateCheckUnitPath}

	allMissing := []hostaction.Missing{
		{Item: hostaction.MissingScript, Path: hostaction.HelperScriptPath},
		{Item: hostaction.MissingPathUnit, Path: hostaction.PathUnitPath},
	}

	cases := []struct {
		name          string
		fsys          fstest.MapFS
		noBox         bool
		wantAvailable bool
		wantMissing   []hostaction.Missing
		wantOutdated  []hostaction.Missing
		// wantUpdateScript nil: an empty list.
		wantUpdateScript []hostaction.Missing
		wantContainer    bool
	}{
		{name: "installed, not in a container", fsys: helperInstalledFS(t), wantAvailable: true, wantMissing: []hostaction.Missing{}, wantOutdated: []hostaction.Missing{}},
		// The update script is asked for whatever missing says: the
		// helper's install commands do not install it.
		{name: "nothing installed", fsys: fstest.MapFS{}, wantMissing: allMissing, wantOutdated: []hostaction.Missing{}, wantUpdateScript: updateScript},
		// Only the two update actions need it, so available stays true
		// (13-REVIEW-2 IN-04).
		{name: "no update script", fsys: noUpdateScript, wantAvailable: true, wantMissing: []hostaction.Missing{}, wantOutdated: []hostaction.Missing{}, wantUpdateScript: updateScript},
		{name: "an update script uid 1000 owns", fsys: updateScriptNotRoots, wantAvailable: true, wantMissing: []hostaction.Missing{}, wantOutdated: []hostaction.Missing{}, wantUpdateScript: updateScript},
		{name: "installed, in a container", fsys: inContainer, wantMissing: []hostaction.Missing{}, wantOutdated: []hostaction.Missing{}, wantContainer: true},
		{name: "no Box", fsys: helperInstalledFS(t), noBox: true, wantMissing: []hostaction.Missing{}, wantOutdated: []hostaction.Missing{}},
		// The four older orders work with an older helper: available stays
		// true, and only the check is said to need more (D-12, D-15).
		{name: "the helper before the check", fsys: olderHelperFS(t), wantAvailable: true, wantMissing: []hostaction.Missing{}, wantOutdated: []hostaction.Missing{scriptOutdated}},
		{name: "no check unit", fsys: noCheckUnit, wantAvailable: true, wantMissing: []hostaction.Missing{}, wantOutdated: []hostaction.Missing{checkUnit}},
		{name: "the helper before the check, not enabled", fsys: olderNotEnabled, wantMissing: []hostaction.Missing{
			{Item: hostaction.MissingNotEnabled, Path: hostaction.WantsLinkPath},
		}, wantOutdated: []hostaction.Missing{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := Config{FS: tc.fsys, Sys: tracerSys(), Now: func() time.Time { return fixedNow }}
			if !tc.noBox {
				cfg.Actions = hostaction.NewBox(hostaction.Config{FS: tc.fsys, DataDir: t.TempDir()})
			}
			c := New(cfg)
			v := c.Read(context.Background())
			a := v.Actions

			if a.Available != tc.wantAvailable {
				t.Errorf("actions.available = %v, want %v", a.Available, tc.wantAvailable)
			}
			if a.Missing == nil {
				t.Errorf("actions.missing is nil; on the wire that is null, and the page reads a list")
			}
			if !reflect.DeepEqual(a.Missing, tc.wantMissing) {
				t.Errorf("actions.missing = %+v, want %+v", a.Missing, tc.wantMissing)
			}
			if a.Outdated == nil {
				t.Errorf("actions.outdated is nil; on the wire that is null, and the page reads a list")
			}
			if !reflect.DeepEqual(a.Outdated, tc.wantOutdated) {
				t.Errorf("actions.outdated = %+v, want %+v", a.Outdated, tc.wantOutdated)
			}
			wantUpdateScript := tc.wantUpdateScript
			if wantUpdateScript == nil {
				wantUpdateScript = []hostaction.Missing{}
			}
			if a.UpdateScript == nil {
				t.Errorf("actions.update_script is nil; on the wire that is null, and the page reads a list")
			}
			if !reflect.DeepEqual(a.UpdateScript, wantUpdateScript) {
				t.Errorf("actions.update_script = %+v, want %+v", a.UpdateScript, wantUpdateScript)
			}
			if !reflect.DeepEqual(a.UpdateScriptInstallCommands, hostaction.UpdateScriptInstallCommands) {
				t.Errorf("actions.update_script_install_commands = %q, want hostaction.UpdateScriptInstallCommands %q",
					a.UpdateScriptInstallCommands, hostaction.UpdateScriptInstallCommands)
			}
			if !reflect.DeepEqual(a.InstallCommands, hostaction.InstallCommands) {
				t.Errorf("actions.install_commands = %q, want hostaction.InstallCommands %q", a.InstallCommands, hostaction.InstallCommands)
			}
			if v.Container != tc.wantContainer {
				t.Errorf("container = %v, want %v", v.Container, tc.wantContainer)
			}
			if got := c.InContainer(); got != v.Container {
				t.Errorf("InContainer() = %v, but the answer says container %v", got, v.Container)
			}
			if tc.noBox {
				if a.Result.Readable || a.Result.Reason == nil || a.Result.Reason.Code != CodeNoResult {
					t.Errorf("actions.result without a Box = %+v, want not readable with %s", a.Result, CodeNoResult)
				}
			}

			// And on the wire: the four keys are there, and missing and
			// outdated are lists.
			wire, _ := marshalView(t, v)["actions"].(map[string]any)
			for _, key := range []string{"available", "missing", "outdated", "update_script", "install_commands", "update_script_install_commands"} {
				if _, ok := wire[key]; !ok {
					t.Errorf("actions.%s is not in the answer: %v", key, wire)
				}
			}
			if _, ok := wire["missing"].([]any); !ok {
				t.Errorf("actions.missing on the wire = %#v, want a list", wire["missing"])
			}
			if _, ok := wire["outdated"].([]any); !ok {
				t.Errorf("actions.outdated on the wire = %#v, want a list", wire["outdated"])
			}
			if _, ok := wire["update_script"].([]any); !ok {
				t.Errorf("actions.update_script on the wire = %#v, want a list", wire["update_script"])
			}
		})
	}
}

// TestInstallCommandsAreNotShared: the answer carries a copy, so nothing that
// holds a View can change the one copy of the install commands.
func TestInstallCommandsAreNotShared(t *testing.T) {
	t.Parallel()

	v := New(Config{FS: fstest.MapFS{}, Sys: tracerSys(), Now: func() time.Time { return fixedNow }}).Read(context.Background())
	if len(v.Actions.InstallCommands) == 0 {
		t.Fatal("no install commands in the answer")
	}
	v.Actions.InstallCommands[0] = "changed"
	if hostaction.InstallCommands[0] == "changed" {
		t.Error("the answer's install commands are hostaction.InstallCommands itself, not a copy")
	}
}

// TestUnsupportedPlatformUpdateScript: a platform without systemd still says
// whether the update script is there, as it says what of the helper is
// missing, and the list is never null.
func TestUnsupportedPlatformUpdateScript(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		fsys fstest.MapFS
		want int
	}{
		{"with the update script", helperInstalledFS(t), 0},
		{"without it", fstest.MapFS{}, 1},
	} {
		v := New(Config{
			FS: tc.fsys, Sys: newUnsupportedSys(), Now: func() time.Time { return fixedNow },
			Actions: hostaction.NewBox(hostaction.Config{FS: tc.fsys, DataDir: t.TempDir()}),
		}).Read(context.Background())
		if v.Actions.UpdateScript == nil || len(v.Actions.UpdateScript) != tc.want {
			t.Errorf("%s: actions.update_script on an unsupported platform = %#v, want %d items", tc.name, v.Actions.UpdateScript, tc.want)
		}
		if !reflect.DeepEqual(v.Actions.UpdateScriptInstallCommands, hostaction.UpdateScriptInstallCommands) {
			t.Errorf("%s: actions.update_script_install_commands = %q", tc.name, v.Actions.UpdateScriptInstallCommands)
		}
	}
}

// TestUnsupportedPlatformOutdated: a platform without systemd asks nothing of
// the helper beyond what Missing says, and outdated is an empty list, never
// null -- even over an older helper's files.
func TestUnsupportedPlatformOutdated(t *testing.T) {
	t.Parallel()

	fsys := olderHelperFS(t)
	v := New(Config{
		FS: fsys, Sys: newUnsupportedSys(), Now: func() time.Time { return fixedNow },
		Actions: hostaction.NewBox(hostaction.Config{FS: fsys, DataDir: t.TempDir()}),
	}).Read(context.Background())
	if v.Actions.Outdated == nil || len(v.Actions.Outdated) != 0 {
		t.Errorf("actions.outdated on an unsupported platform = %#v, want an empty list", v.Actions.Outdated)
	}
}
