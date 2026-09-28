package host

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/host/updatestatus"
)

// The Service card (HOST-02) and its update row (HOST-03, D-16).

const statusName = "var/lib/holzkube-manager-update/status.json"

func serviceCollector(fsys fstest.MapFS, started, now time.Time) *Collector {
	return New(Config{
		FS:      fsys,
		Sys:     tracerSys(),
		Now:     func() time.Time { return now },
		Version: "0.1.0",
		Started: started,
		DataDir: piDataDir,
	})
}

func TestServiceSection(t *testing.T) {
	t.Parallel()

	started := fixedNow
	now := started.Add(2*time.Hour + 3*time.Second)
	s := serviceCollector(fstest.MapFS{}, started, now).Read(context.Background()).Service

	if s.Version != "0.1.0" {
		t.Errorf("version = %q, want 0.1.0", s.Version)
	}
	if !s.StartedAt.Equal(started) || s.StartedAt.Location() != time.UTC {
		t.Errorf("started_at = %v, want %v in UTC", s.StartedAt, started.UTC())
	}
	if s.UptimeSeconds != 7203 {
		t.Errorf("uptime_seconds = %d, want 7203", s.UptimeSeconds)
	}
	if s.DataDir.Path != piDataDir {
		t.Errorf("data_dir.path = %q, want %q", s.DataDir.Path, piDataDir)
	}
	// The fixture has no data directory, so its size is not readable -- and
	// says so, rather than being a 0.
	if s.DataDir.Size.Readable || s.DataDir.Size.Value != nil || s.DataDir.Size.Reason == nil {
		t.Errorf("data_dir.size = %+v, want not readable with a reason", s.DataDir.Size)
	}
}

func TestUpdateStatusReadings(t *testing.T) {
	t.Parallel()

	started := fixedNow
	valid := `{"checked_at":"2026-09-28T09:00:00Z","installed":"0.1.0","latest":"0.2.0","outcome":"available"}`
	failed := `{"checked_at":"2026-09-28T09:00:00Z","installed":"0.1.0","latest":null,"outcome":"failed"}`

	for _, tc := range []struct {
		name    string
		fsys    fstest.MapFS
		code    string
		message string // for a hidden reading: the whole sentence, or its prefix when prefix is set
		prefix  bool
		want    *updatestatus.Status
	}{
		{
			name:    "absent on a host",
			fsys:    fstest.MapFS{},
			code:    CodeNotRecorded,
			message: "The update script installed on this machine does not record its checks. Versions from this release on do; the next update brings it.",
		},
		{
			name:    "absent in a container",
			fsys:    fstest.MapFS{".dockerenv": {}},
			code:    CodeNotRecorded,
			message: "holzkube-manager runs in a container, where the host's update timer does not run. A container is updated by pulling a new image.",
		},
		{
			name:    "an unknown outcome",
			fsys:    fstest.MapFS{statusName: {Data: []byte(`{"checked_at":"2026-09-28T09:00:00Z","installed":"0.1.0","latest":"0.2.0","outcome":"exploded"}`)}},
			code:    CodeReadFailed,
			message: "The update status file exists but could not be read: ",
			prefix:  true,
		},
		{
			name: "recorded",
			fsys: fstest.MapFS{statusName: {Data: []byte(valid)}},
			want: &updatestatus.Status{CheckedAt: time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC), Installed: ptr("0.1.0"), Latest: ptr("0.2.0"), Outcome: "available"},
		},
		{
			name: "recorded failure without a latest",
			fsys: fstest.MapFS{statusName: {Data: []byte(failed)}},
			want: &updatestatus.Status{CheckedAt: time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC), Installed: ptr("0.1.0"), Outcome: "failed"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := serviceCollector(tc.fsys, started, started.Add(time.Minute)).Read(context.Background())
			u := v.Service.Update

			if tc.want != nil {
				got := mustRead(t, "update", u)
				if !got.CheckedAt.Equal(tc.want.CheckedAt) || got.Outcome != tc.want.Outcome ||
					deref(got.Installed) != deref(tc.want.Installed) || deref(got.Latest) != deref(tc.want.Latest) ||
					(got.Latest == nil) != (tc.want.Latest == nil) {
					t.Errorf("update = %+v (installed %s, latest %s), want %+v", got, deref(got.Installed), deref(got.Latest), tc.want)
				}
				if tc.want.Latest == nil {
					service, _ := marshalView(t, v)["service"].(map[string]any)
					update, _ := service["update"].(map[string]any)
					value, _ := update["value"].(map[string]any)
					if latest, has := value["latest"]; !has || latest != nil {
						t.Errorf("latest on the wire = %v (present %v), want null", latest, has)
					}
				}
				return
			}

			// Not recorded or not readable: no value, and above all no time or
			// version made up to fill the row.
			if u.Readable || u.Value != nil || u.Reason == nil {
				t.Fatalf("update = %+v (value %+v), want not readable with a reason and no value", u, u.Value)
			}
			if u.Reason.Code != tc.code {
				t.Errorf("code = %q, want %q", u.Reason.Code, tc.code)
			}
			if tc.prefix {
				if !strings.HasPrefix(u.Reason.Message, tc.message) || len(u.Reason.Message) == len(tc.message) {
					t.Errorf("message = %q, want %q followed by the cause", u.Reason.Message, tc.message)
				}
			} else if u.Reason.Message != tc.message {
				t.Errorf("message = %q\n want %q", u.Reason.Message, tc.message)
			}
		})
	}
}

// TestUpdateStatusPathIsConfigurable: the status is read where the operator's
// --update-status-file says, not only at the default.
func TestUpdateStatusPathIsConfigurable(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{"srv/upd/status.json": {Data: []byte(`{"checked_at":"2026-09-28T09:00:00Z","installed":"0.1.0","latest":"0.1.0","outcome":"current"}`)}}
	c := New(Config{FS: fsys, Sys: tracerSys(), Now: func() time.Time { return fixedNow }, UpdateStatusPath: "/srv/upd/status.json"})
	if got := mustRead(t, "update", c.Read(context.Background()).Service.Update); got.Outcome != "current" {
		t.Errorf("outcome = %q, want current", got.Outcome)
	}
}

func ptr(s string) *string { return &s }

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
