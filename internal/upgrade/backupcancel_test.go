package upgrade_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// TestASnapshotInterruptedInsideAGRPCCallIsNotRecordedAsFailed is the bug
// 6eb348f fixed in the engine, in the snapshot step: it recognised an
// interruption by errors.Is(err, context.Canceled), and a step inside a Talos
// call returns a status error with codes.Canceled, which is not that. The
// interrupted attempt was then written down as failed, with "rpc error: code =
// Canceled" for a reason. The step now asks its context.
func TestASnapshotInterruptedInsideAGRPCCallIsNotRecordedAsFailed(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{}, 1)
	b := newBackupRig(t, func(d *upgrade.Deps) {
		d.Machines = func(ctx context.Context, _ model.ClusterID) ([]model.Machine, error) {
			entered <- struct{}{}
			<-ctx.Done()
			return nil, status.Error(codes.Canceled, "context canceled")
		}
	}, nil)

	j, err := b.svc.SubmitBackup(t.Context(), removeCluster, 3, upgrade.TriggerManual, "")
	if err != nil {
		t.Fatalf("SubmitBackup: %v", err)
	}
	<-entered
	if _, err := b.engine.Cancel(t.Context(), j.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	b.settle()

	if got := b.status().Status.LastResult; got == upgrade.BackupFailed {
		t.Fatalf("an interrupted snapshot was recorded as %q (%s)", got, b.status().Status.LastReason)
	}
}
