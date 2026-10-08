package upgrade

import (
	"io"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// WriteForTest exposes the store's write to the external test package.
func WriteForTest(st *SnapshotStore, cluster model.ClusterID, fn func(io.Writer) (int64, error)) (SafetySnapshot, error) {
	return st.write(cluster, fn)
}

// SnapshotDirForTest exposes where the store keeps its files.
func SnapshotDirForTest(st *SnapshotStore) string { return st.dir }

// WriteKindForTest exposes the store's kind-aware write.
func WriteKindForTest(
	st *SnapshotStore, cluster model.ClusterID, kind string, keep int, estimate int64,
	fn func(io.Writer) (int64, error),
) (SafetySnapshot, error) {
	return st.writeKind(cluster, kind, keep, estimate, fn)
}

// SetSubmitForTest replaces how the service starts a job.
func (s *Service) SetSubmitForTest(f SubmitFunc) { s.submit = f }

// BackupStepForTest exposes the snapshot job's step.
func (s *Service) BackupStepForTest() jobs.Step { return s.backupStep() }

// DirForTest is where the store keeps its files.
func (st *SnapshotStore) DirForTest() string { return st.dir }
