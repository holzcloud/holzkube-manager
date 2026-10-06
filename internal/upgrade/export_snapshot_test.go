package upgrade

import (
	"io"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// WriteForTest exposes the store's write to the external test package.
func WriteForTest(st *SnapshotStore, cluster model.ClusterID, fn func(io.Writer) (int64, error)) (SafetySnapshot, error) {
	return st.write(cluster, fn)
}

// SnapshotDirForTest exposes where the store keeps its files.
func SnapshotDirForTest(st *SnapshotStore) string { return st.dir }
