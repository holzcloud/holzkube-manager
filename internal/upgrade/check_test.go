package upgrade_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

func snapshotDir(st *upgrade.SnapshotStore) string { return upgrade.SnapshotDirForTest(st) }

// newCheckService builds a service that reads nothing from a node: the check
// uses only the inventory's last observation and the release list.
func newCheckService(versions []string, releases []string) *upgrade.Service {
	machines := make([]model.Machine, 0, len(versions))
	for _, v := range versions {
		m := model.Machine{}
		m.Snapshot.TalosVersion = v
		machines = append(machines, m)
	}
	return upgrade.NewService(upgrade.Deps{
		Machines: func(context.Context, model.ClusterID) ([]model.Machine, error) { return machines, nil },
		ResolveInstaller: func(context.Context, string, string, bool) (string, error) {
			return "", errors.New("not used by the check")
		},
	}, func(context.Context) ([]string, error) { return releases, nil })
}

func TestCheckTalos(t *testing.T) {
	t.Parallel()

	releases := []string{"v1.12.6", "v1.13.9", "v1.14.0", "v1.14.2", "v1.14.3-rc.1", "v1.15.0"}

	tests := []struct {
		name      string
		versions  []string
		releases  []string
		available bool
		next      string
		runs      int
		reason    string
	}{
		{"a patch behind", []string{"v1.14.0", "v1.14.0"}, releases, true, "v1.14.2", 1, ""},
		{"up to date", []string{"v1.14.2"}, releases, false, "", 0, "newest release"},
		{"one node behind in a mixed cluster", []string{"v1.14.2", "v1.14.0"}, releases, true, "v1.14.2", 1, ""},
		{"a minor behind goes straight to the newest patch", []string{"v1.13.9"}, releases, true, "v1.14.2", 1, ""},
		{"two minors behind is two runs", []string{"v1.12.6"}, releases, true, "v1.13.9", 2, ""},
		{"no version reported", []string{"", "garbage"}, releases, false, "", 0, "has reported"},
		{"no release this build supports", []string{"v1.14.0"}, []string{"v1.15.0"}, false, "", 0, "listed no stable"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := newCheckService(tc.versions, tc.releases).CheckTalos(context.Background(), "c1", "home")
			if err != nil {
				t.Fatalf("CheckTalos: %v", err)
			}
			if got.Available != tc.available || got.Next != tc.next || got.Runs != tc.runs {
				t.Fatalf("available=%v next=%q runs=%d, want %v %q %d (%+v)",
					got.Available, got.Next, got.Runs, tc.available, tc.next, tc.runs, got)
			}
			if tc.reason != "" && !strings.Contains(got.Reason, tc.reason) {
				t.Errorf("reason %q does not mention %q", got.Reason, tc.reason)
			}
			if tc.available && got.NotesURL != "https://github.com/siderolabs/talos/releases/tag/"+tc.next {
				t.Errorf("notes url = %q", got.NotesURL)
			}
		})
	}
}

// The build supports v1.14 at most (talos.MaxSupportedVersion), so a v1.15
// upstream is not offered even though it is stable: this product cannot read
// that cluster's configuration.
func TestCheckTalosNeverOffersWhatTheBuildCannotRead(t *testing.T) {
	t.Parallel()
	got, err := newCheckService([]string{"v1.14.0"}, []string{"v1.14.0", "v1.15.0", "v1.16.2"}).
		CheckTalos(context.Background(), "c1", "home")
	if err != nil {
		t.Fatal(err)
	}
	if got.Available || got.Newest == "v1.15.0" || got.Newest == "v1.16.2" {
		t.Fatalf("offered a release beyond the supported range: %+v", got)
	}
}

func TestCheckTalosListsEveryVersionOldestFirst(t *testing.T) {
	t.Parallel()
	got, err := newCheckService([]string{"v1.14.2", "v1.13.9", "v1.14.2"}, []string{"v1.14.2"}).
		CheckTalos(context.Background(), "c1", "home")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Current) != 2 || got.Current[0] != "v1.13.9" || got.Current[1] != "v1.14.2" {
		t.Fatalf("current = %v", got.Current)
	}
}
