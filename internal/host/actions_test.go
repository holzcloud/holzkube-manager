package host

import (
	"context"
	"io/fs"
	"reflect"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/host/hostaction"
)

// helperInstalledFS is a root filesystem on which the helper is installed the
// way deploy/HOST-HELPER.md installs it.
func helperInstalledFS() fstest.MapFS {
	return fstest.MapFS{
		"usr/local/sbin/holzkube-manager-host": {
			Data: []byte("#!/usr/bin/env bash\n"), Mode: 0o755, Sys: &syscall.Stat_t{Uid: 0, Gid: 0},
		},
		"etc/systemd/system/holzkube-manager-host.path":                    {Data: []byte("[Path]\n"), Mode: 0o644},
		"etc/systemd/system/holzkube-manager-host.service":                 {Data: []byte("[Service]\n"), Mode: 0o644},
		"etc/systemd/system/paths.target.wants/holzkube-manager-host.path": {Data: []byte(hostaction.PathUnitPath), Mode: fs.ModeSymlink | 0o777},
	}
}

// TestReadCarriesActions: one answer says whether the page may offer the host
// actions and, if not, exactly what is missing and what to run (D-12, D-14).
func TestReadCarriesActions(t *testing.T) {
	t.Parallel()

	inContainer := helperInstalledFS()
	inContainer[".dockerenv"] = &fstest.MapFile{}

	allMissing := []hostaction.Missing{
		{Item: hostaction.MissingScript, Path: hostaction.HelperScriptPath},
		{Item: hostaction.MissingPathUnit, Path: hostaction.PathUnitPath},
		{Item: hostaction.MissingNotEnabled, Path: hostaction.WantsLinkPath},
	}

	cases := []struct {
		name          string
		fsys          fstest.MapFS
		noBox         bool
		wantAvailable bool
		wantMissing   []hostaction.Missing
		wantContainer bool
	}{
		{name: "installed, not in a container", fsys: helperInstalledFS(), wantAvailable: true, wantMissing: []hostaction.Missing{}},
		{name: "nothing installed", fsys: fstest.MapFS{}, wantMissing: allMissing},
		{name: "installed, in a container", fsys: inContainer, wantMissing: []hostaction.Missing{}, wantContainer: true},
		{name: "no Box", fsys: helperInstalledFS(), noBox: true, wantMissing: []hostaction.Missing{}},
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

			// And on the wire: the three keys are there, and missing is a list.
			wire, _ := marshalView(t, v)["actions"].(map[string]any)
			for _, key := range []string{"available", "missing", "install_commands"} {
				if _, ok := wire[key]; !ok {
					t.Errorf("actions.%s is not in the answer: %v", key, wire)
				}
			}
			if _, ok := wire["missing"].([]any); !ok {
				t.Errorf("actions.missing on the wire = %#v, want a list", wire["missing"])
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
