package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// Scheduled etcd snapshots: the schedule, the list of what is stored, a
// download, and "run now".
//
// The files are the ones the pre-upgrade snapshot already keeps, in the same
// directory; see internal/upgrade/backups.go. What is new here is only the
// door: and since a snapshot holds every Kubernetes secret of the cluster, the
// download is guarded exactly as the live snapshot is -- admin, and recorded in
// the audit archive even though it is a GET.

func backupRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			// The list carries sizes, times and checksums, no content, so a
			// reader may see it.
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/backups",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Handler:         handler(backupsState(d)),
		},
		{
			// Destructive for its sudo window, as the other settings that
			// decide what this product does on its own: turning a schedule on
			// is how a disk fills with copies of every secret in the cluster.
			// No ClusterScope: it changes nothing on any node, so a cluster
			// adopted read-only can still be backed up.
			Method:          http.MethodPut,
			Pattern:         "/api/v1/clusters/{id}/backups/schedule",
			RequiresSession: true,
			MinRole:         model.RoleAdmin,
			Destructive:     true,
			Action:          "cluster.backup-schedule",
			Handler:         handler(backupSetSchedule(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/clusters/{id}/backups/run",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "etcd.backup-run",
			Handler:         handler(backupRunNow(d)),
		},
		{
			// Bytes, not JSON: Streaming for the reason the live snapshot is.
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/backups/{name}",
			RequiresSession: true,
			MinRole:         model.RoleAdmin,
			AuditRead:       true,
			Streaming:       true,
			Action:          "etcd.backup-download",
			Handler:         handler(backupDownload(d)),
		},
	}
}

// clusterRecord reads the cluster, answering the 404 itself.
func clusterRecord(d httpapi.Deps, w http.ResponseWriter, r *http.Request) (model.Cluster, bool) {
	c, err := d.Store.Clusters().Get(r.Context(), model.ClusterID(r.PathValue("id")))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpapi.WriteProblem(w, r, httpapi.NotFound("notfound.cluster", "No such cluster."))
			return model.Cluster{}, false
		}
		httpapi.WriteInternal(w, r, d.Logger, err)
		return model.Cluster{}, false
	}
	return c, true
}

func backupsState(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := upgradeConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		c, ok := clusterRecord(d, w, r)
		if !ok {
			return
		}
		state, err := d.Upgrade.BackupsOf(c)
		if err != nil && !errors.Is(err, upgrade.ErrNoSnapshotStore) {
			writeUpgradeError(w, r, d, err)
			return
		}
		sched := model.BackupSchedule{Interval: model.BackupOff, Keep: model.DefaultBackupKeep}
		if c.BackupSchedule != nil {
			sched = *c.BackupSchedule
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"available": err == nil,
			"schedule":  sched,
			"presets":   []string{model.BackupOff, model.BackupEvery6, model.BackupDaily, model.BackupWeekly},
			"max_keep":  model.MaxBackupKeep,
			"state":     state,
			"notice": "These snapshots are kept on the device this manager runs on. They survive a bad " +
				"upgrade, not the loss of that device: download the ones that matter and keep them elsewhere. " +
				"Each one holds every Kubernetes secret of the cluster.",
		})
	}
}

func backupSetSchedule(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := upgradeConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		if d.Inventory == nil {
			httpapi.WriteProblem(w, r, httpapi.Upstream("upstream.inventory-unavailable",
				"This instance was started without the inventory."))
			return
		}
		var body struct {
			Interval string `json:"interval"`
			Keep     *int   `json:"keep"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}
		switch body.Interval {
		case model.BackupOff, model.BackupEvery6, model.BackupDaily, model.BackupWeekly:
		default:
			httpapi.WriteProblem(w, r, httpapi.Validation("Choose how often: off, 6h, daily or weekly.",
				httpapi.FieldError{Field: "interval", Reason: "must be off, 6h, daily or weekly"}))
			return
		}
		keep := model.DefaultBackupKeep
		if body.Keep != nil {
			keep = *body.Keep
		}
		if keep < 1 || keep > model.MaxBackupKeep {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"Keep between 1 and "+strconv.Itoa(model.MaxBackupKeep)+" snapshots.",
				httpapi.FieldError{Field: "keep", Reason: "must be between 1 and " + strconv.Itoa(model.MaxBackupKeep)}))
			return
		}
		if _, ok := clusterRecord(d, w, r); !ok {
			return
		}
		c, err := d.Inventory.SetBackupSchedule(r.Context(), model.ClusterID(r.PathValue("id")),
			model.BackupSchedule{Interval: body.Interval, Keep: keep})
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}
		sched := model.BackupSchedule{Interval: model.BackupOff, Keep: keep}
		if c.BackupSchedule != nil {
			sched = *c.BackupSchedule
		}
		writeJSON(w, http.StatusOK, map[string]any{"schedule": sched})
	}
}

func backupRunNow(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := upgradeConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		c, ok := clusterRecord(d, w, r)
		if !ok {
			return
		}
		keep := model.DefaultBackupKeep
		if c.BackupSchedule != nil && c.BackupSchedule.Keep > 0 {
			keep = c.BackupSchedule.Keep
		}
		actor := ""
		if d.Auth != nil {
			if u, ok := d.Auth.CurrentUser(r.Context()); ok {
				actor = u.Username
			}
		}
		j, err := d.Upgrade.SubmitBackup(r.Context(), c.ID, keep, upgrade.TriggerManual, actor)
		if err != nil {
			switch {
			case errors.Is(err, jobs.ErrClusterBusy):
				writeJobError(w, r, d, err)
			case errors.Is(err, upgrade.ErrNoSnapshotStore), errors.Is(err, upgrade.ErrBackupsNotWired):
				httpapi.WriteProblem(w, r, httpapi.Upstream("upstream.upgrade-unavailable", err.Error()))
			default:
				writeJobError(w, r, d, err)
			}
			return
		}
		w.Header().Set("Location", "/api/v1/jobs/"+string(j.ID))
		writeJSON(w, http.StatusAccepted, map[string]any{"job": j, "topic": string(jobs.Topic(j.ID))})
	}
}

func backupDownload(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := upgradeConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		cluster := model.ClusterID(r.PathValue("id"))
		name := r.PathValue("name")

		// Opened before any header goes out, so a snapshot that is not there is
		// a problem document and not a download of an error page.
		f, size, err := d.Upgrade.OpenBackup(cluster, name)
		if err != nil {
			switch {
			case errors.Is(err, upgrade.ErrSnapshotNotFound):
				httpapi.WriteProblem(w, r, httpapi.NotFound("notfound.snapshot", "No such stored snapshot."))
			case errors.Is(err, upgrade.ErrDownloadUnavailable), errors.Is(err, upgrade.ErrNoSnapshotStore):
				httpapi.WriteProblem(w, r, httpapi.Upstream("upstream.upgrade-unavailable", err.Error()))
			default:
				writeUpgradeError(w, r, d, err)
			}
			return
		}
		defer f.Close() //nolint:errcheck // a read-only file

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename=%q", "etcd-"+string(cluster)+"-"+name))
		_, _ = io.Copy(w, f)
	}
}
