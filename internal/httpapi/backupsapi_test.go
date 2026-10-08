package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// Scheduled etcd snapshots at the HTTP edge: who may do what, that the
// download is audited, and that the standing reaches the overview and
// /metrics. The snapshot itself is talossim's etcd, taken by a real job.

type backupsBody struct {
	Available bool `json:"available"`
	Schedule  struct {
		Interval string `json:"interval"`
		Keep     int    `json:"keep"`
	} `json:"schedule"`
	State struct {
		Snapshots []struct {
			ID     string `json:"id"`
			Kind   string `json:"kind"`
			Bytes  int64  `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"snapshots"`
		Status struct {
			LastResult string `json:"last_result"`
		} `json:"status"`
		Health model.BackupHealth `json:"health"`
	} `json:"state"`
}

func (h *upgradeHarness) backups(t *testing.T, cluster model.ClusterID) backupsBody {
	t.Helper()
	resp, raw := h.do(t, http.MethodGet, "/api/v1/clusters/"+string(cluster)+"/backups", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET backups: %d (%s)", resp.StatusCode, raw)
	}
	var b backupsBody
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatalf("decode: %v (%s)", err, raw)
	}
	return b
}

func TestScheduledSnapshotsEndToEnd(t *testing.T) {
	t.Parallel()

	h := newUpgradeHarness(t)
	h.openSudo(t)
	cluster := h.clusterID(t)
	base := "/api/v1/clusters/" + string(cluster) + "/backups"

	for _, role := range []model.UserRole{model.RoleOperator, model.RoleReader} {
		resp, raw := h.do(t, http.MethodPost, "/api/v1/users", map[string]string{
			"username": string(role) + "-account", "password": newAccountPass, "role": string(role),
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("creating the %s account: %d (%s)", role, resp.StatusCode, raw)
		}
	}
	operator := h.asUser(t, "operator-account", newAccountPass)
	reader := h.asUser(t, "reader-account", newAccountPass)

	// Off by default, and a reader may look.
	if b := h.backups(t, cluster); !b.Available || b.Schedule.Interval != "off" || b.Schedule.Keep != 7 {
		t.Fatalf("a fresh cluster: %+v", b)
	}
	if code, _ := reader.status(t, http.MethodGet, base, nil); code != http.StatusOK {
		t.Errorf("a reader reading the list: %d", code)
	}

	// The schedule is an admin's, and behind the sudo window.
	for name, c := range map[string]*asClient{"operator": operator, "reader": reader} {
		if code, _ := c.status(t, http.MethodPut, base+"/schedule",
			map[string]any{"interval": "daily", "keep": 3}); code != http.StatusForbidden {
			t.Errorf("a %s changing the schedule: %d, want 403", name, code)
		}
	}
	for _, bad := range []map[string]any{
		{"interval": "hourly"},
		{"interval": "daily", "keep": 0},
		{"interval": "daily", "keep": 61},
	} {
		if resp, raw := h.do(t, http.MethodPut, base+"/schedule", bad); resp.StatusCode < 400 || resp.StatusCode >= 500 {
			t.Errorf("schedule %v: %d (%s), want a 4xx", bad, resp.StatusCode, raw)
		}
	}
	if resp, raw := h.do(t, http.MethodPut, base+"/schedule", map[string]any{"interval": "daily", "keep": 3}); resp.StatusCode != http.StatusOK {
		t.Fatalf("set the schedule: %d (%s)", resp.StatusCode, raw)
	}
	b := h.backups(t, cluster)
	if b.Schedule.Interval != "daily" || b.Schedule.Keep != 3 || !b.State.Health.Enabled {
		t.Fatalf("after setting it: %+v", b)
	}

	// The overview carries the standing.
	resp, raw := h.do(t, http.MethodGet, "/api/v1/clusters", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), `"backup":{"enabled":true,"interval":"daily"`) {
		t.Fatalf("the cluster overview does not carry the backup standing: %d (%s)", resp.StatusCode, raw)
	}

	// Run now is an operator's, and starts a job.
	if code, _ := reader.status(t, http.MethodPost, base+"/run", map[string]any{}); code != http.StatusForbidden {
		t.Errorf("a reader running a snapshot: %d, want 403", code)
	}
	code, raw2 := operator.status(t, http.MethodPost, base+"/run", map[string]any{})
	if code != http.StatusAccepted || !strings.Contains(string(raw2), `"job"`) {
		t.Fatalf("run now as an operator: %d (%s)", code, raw2)
	}

	deadline := time.Now().Add(30 * time.Second)
	for len(b.State.Snapshots) == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("no snapshot after 30 s: %+v", b)
		}
		time.Sleep(20 * time.Millisecond)
		b = h.backups(t, cluster)
	}
	snap := b.State.Snapshots[0]
	if snap.Kind != "scheduled" || len(snap.SHA256) != 64 || snap.Bytes == 0 {
		t.Fatalf("the stored snapshot: %+v", snap)
	}

	// The download is an admin's and it is recorded, though it is a GET.
	for name, c := range map[string]*asClient{"operator": operator, "reader": reader} {
		if code, _ := c.status(t, http.MethodGet, base+"/"+snap.ID, nil); code != http.StatusForbidden {
			t.Errorf("a %s downloading a snapshot: %d, want 403", name, code)
		}
	}
	resp, body := h.do(t, http.MethodGet, base+"/"+snap.ID, nil)
	if resp.StatusCode != http.StatusOK || int64(len(body)) != snap.Bytes ||
		resp.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("download: %d, %d bytes of %d, %q", resp.StatusCode, len(body), snap.Bytes, resp.Header.Get("Content-Type"))
	}
	if audit := h.auditRaw(t, "?limit=200"); !strings.Contains(audit, "etcd.backup-download") {
		t.Errorf("the download is not in the audit archive:\n%s", audit)
	}

	for _, name := range []string{"nope.snapshot", "..%2F..%2Fetc%2Fpasswd", "20200101T000000.000Z.snapshot"} {
		if resp, raw := h.do(t, http.MethodGet, base+"/"+name, nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("download of %q: %d (%s), want 404", name, resp.StatusCode, raw)
		}
	}

	// And /metrics says how old it is and that nothing is overdue.
	_, raw = h.do(t, http.MethodGet, "/metrics", nil)
	m := string(raw)
	for _, want := range []string{
		`holzkube_etcd_snapshot_age_seconds{cluster="` + string(cluster) + `"}`,
		`holzkube_etcd_snapshot_schedule_enabled{cluster="` + string(cluster) + `"} 1`,
		`holzkube_etcd_snapshot_overdue{cluster="` + string(cluster) + `"} 0`,
	} {
		if !strings.Contains(m, want) {
			t.Errorf("/metrics lacks %s:\n%s", want, m)
		}
	}

	// Turning it off leaves the files and removes the schedule.
	if resp, raw := h.do(t, http.MethodPut, base+"/schedule", map[string]any{"interval": "off"}); resp.StatusCode != http.StatusOK {
		t.Fatalf("switch off: %d (%s)", resp.StatusCode, raw)
	}
	if b = h.backups(t, cluster); b.Schedule.Interval != "off" || b.State.Health.Enabled || len(b.State.Snapshots) != 1 {
		t.Fatalf("after switching off: %+v", b)
	}
}

func TestChangingTheScheduleNeedsTheSudoWindow(t *testing.T) {
	t.Parallel()

	h := newUpgradeHarness(t)
	cluster := h.clusterID(t)
	resp, raw := h.do(t, http.MethodPut, "/api/v1/clusters/"+string(cluster)+"/backups/schedule",
		map[string]any{"interval": "daily", "keep": 3})
	if resp.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("without the sudo window: %d (%s), want 428", resp.StatusCode, raw)
	}
}
