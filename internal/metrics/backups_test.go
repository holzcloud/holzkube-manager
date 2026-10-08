package metrics_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/metrics"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The snapshot age has no series for a cluster that has never had a snapshot --
// a sentinel would graph as a very young one -- while the overdue and schedule
// gauges are written for every cluster, zeros included.
func TestSnapshotAgeIsAbsentWhenThereIsNoneAndOverdueIsNeverAbsent(t *testing.T) {
	t.Parallel()

	e := metrics.New(metrics.Deps{
		Machines: func(context.Context) ([]inventory.MachineView, error) { return nil, nil },
		Clusters: func(context.Context) ([]inventory.ClusterView, error) { return nil, nil },
		Jobs:     func(context.Context) ([]model.Job, error) { return nil, nil },
		Backups: func(context.Context) ([]metrics.BackupStat, error) {
			age := int64(3600)
			return []metrics.BackupStat{
				metrics.BackupStatFrom("with", model.BackupHealth{Enabled: true, AgeSeconds: &age}),
				metrics.BackupStatFrom("without", model.BackupHealth{Enabled: true, Overdue: true}),
				metrics.BackupStatFrom("off", model.BackupHealth{}),
			}, nil
		},
		AuditChainIntact: func() bool { return true },
		Now:              func() time.Time { return now },
	})
	var b strings.Builder
	if err := e.Write(t.Context(), &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()

	for _, want := range []string{
		`holzkube_etcd_snapshot_age_seconds{cluster="with"} 3600`,
		`holzkube_etcd_snapshot_overdue{cluster="with"} 0`,
		`holzkube_etcd_snapshot_overdue{cluster="without"} 1`,
		`holzkube_etcd_snapshot_overdue{cluster="off"} 0`,
		`holzkube_etcd_snapshot_schedule_enabled{cluster="off"} 0`,
		"# TYPE holzkube_etcd_snapshot_age_seconds gauge",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	for _, c := range []string{"without", "off"} {
		if strings.Contains(out, `holzkube_etcd_snapshot_age_seconds{cluster="`+c+`"}`) {
			t.Errorf("an age was written for %q, which has no snapshot", c)
		}
	}
}
