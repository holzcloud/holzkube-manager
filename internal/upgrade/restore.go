package upgrade

// Restoring etcd from a snapshot (UPG-12's second half, Omni parity phase 1).
//
// The README said for a long time that this product "takes etcd snapshots and
// does not restore them", and that sentence was the largest single gap in it: a
// backup that cannot be replayed is a file, not a recovery.
//
// A restore is two calls and they are deliberately not one. EtcdRecover uploads
// the snapshot and changes nothing -- the node goes on running whatever etcd it
// was running. BootstrapFromSnapshot then replaces that etcd with the uploaded
// file. Keeping them apart is what lets an upload fail without having touched
// the cluster, and it is also what makes the half-done state nameable: a
// snapshot uploaded and never recovered from is a cluster running its old data
// while somebody believes it was restored.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// ErrNotControlPlane reports a restore aimed at a node that has no etcd.
var ErrNotControlPlane = errors.New("upgrade: only a control-plane node has an etcd to restore")

// ErrEmptySnapshot reports a snapshot with no bytes in it.
//
// It is checked here rather than left to the node because a zero-length file
// looks like a backup in a directory listing, and an operator who reaches for
// one has already lost the cluster it was meant to save. Failing before the
// upload says so while there is still something to say it about.
var ErrEmptySnapshot = errors.New("upgrade: the snapshot is empty, which is not a snapshot")

// RestoreRequest is one restore.
type RestoreRequest struct {
	// Snapshot is the database to restore. It is a reader and not bytes
	// because an etcd database is as large as it is, and holding somebody's
	// cluster in this process's memory to hand it over is a size limit nobody
	// chose.
	Snapshot io.Reader

	// Role is the target node's role, checked before anything is sent.
	Role model.MachineRole

	// SkipHashCheck turns off the snapshot's own integrity check on the node.
	//
	// It exists for exactly one case and it is the case an operator is most
	// likely to be in: a snapshot copied off a node's data directory rather
	// than taken through the etcd API has no hash to check, and that is what
	// you are left with on a cluster that had already lost quorum (see
	// talos.SnapshotNotice). Passing it for an API-taken snapshot turns off the
	// one check that would have caught a truncated upload.
	SkipHashCheck bool
}

// Restore uploads a snapshot to a control-plane node and starts etcd from it.
//
// It returns the number of bytes uploaded, for the same reason Snapshot does:
// a transfer that moved nothing and reported success is the failure worth
// catching.
func Restore(ctx context.Context, cc *talos.ClusterClient, req RestoreRequest) (int64, error) {
	if req.Role != model.RoleControlPlane {
		return 0, fmt.Errorf("%w, and this node is %s", ErrNotControlPlane, req.Role)
	}
	if req.Snapshot == nil {
		return 0, ErrEmptySnapshot
	}

	// One byte, read before anything is sent, so an empty snapshot is refused
	// here rather than by the node.
	//
	// The difference is what an operator is told. A node refuses an empty
	// upload with a transport error naming an RPC, which reads like the node
	// is broken; this reads like the file is. And a test asserted the refusal
	// happened before the upload while the code was refusing after it, which
	// is how this was found.
	//
	// One byte and not the whole thing: the alternative is buffering the
	// database to find out how big it is, which is the memory cost this
	// signature exists to avoid.
	first := make([]byte, 1)
	switch n, err := io.ReadFull(req.Snapshot, first); {
	case errors.Is(err, io.EOF), n == 0 && err == nil:
		return 0, ErrEmptySnapshot
	case err != nil && !errors.Is(err, io.ErrUnexpectedEOF):
		return 0, fmt.Errorf("reading the snapshot: %w", err)
	}

	// Counted on the way past. The byte already read goes back in front, so
	// what the node receives is the whole file and the count is the whole
	// file's length.
	counted := &countingReader{r: io.MultiReader(bytes.NewReader(first), req.Snapshot)}

	// No class deadline on the upload: ClassUpload carries none, because a
	// total deadline here is a bound on how large somebody's etcd is allowed
	// to be. What bounds it is the acknowledgement at the end.
	if err := cc.EtcdRecover(ctx, counted); err != nil {
		return counted.n, fmt.Errorf("uploading the snapshot: %w", err)
	}
	if counted.n == 0 {
		// Unreachable through the check above, and kept because it is the
		// condition the bootstrap below must never run under: replacing a
		// working etcd with nothing is the one outcome worse than not
		// restoring at all.
		return 0, ErrEmptySnapshot
	}

	bootCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodBootstrap)
	if err != nil {
		return counted.n, err
	}
	defer cancel()

	if err := cc.BootstrapFromSnapshot(bootCtx, req.SkipHashCheck); err != nil {
		// Named, because this is the half-done state. The snapshot is on the
		// node and the cluster is still running its old etcd, which is the
		// better of the two places to stop -- and an operator who is told only
		// "restore failed" cannot tell it from the worse one.
		return counted.n, fmt.Errorf("the snapshot was uploaded and etcd was not restarted from "+
			"it, so the cluster is still running the data it had: %w", err)
	}
	return counted.n, nil
}

// countingReader counts what passes through it.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
