package provision

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// Bootstrapping etcd twice destroys a cluster, and this file is four
// independent mechanisms stacked so that it cannot happen (PROV-10).
//
// Four, not one, because each covers a case the others do not:
//
//  1. **A pre-flight `EtcdMemberList`.** If etcd is already running, it
//     answers, and there is nothing to bootstrap. This catches the ordinary
//     case -- somebody bootstrapped from talosctl an hour ago -- and catches
//     nothing else, because a node whose etcd has not started yet looks the
//     same as one that was never bootstrapped.
//  2. **An `O_CREAT|O_EXCL` lease file.** Two holzkube-manager processes, or two
//     goroutines in one, cannot both hold it: the kernel decides. This is the
//     only one of the four that is atomic, and it is the only one that covers
//     two operators clicking at the same moment.
//  3. **An fsynced intent record, written before the call.** If the process
//     dies mid-bootstrap, the record is what says a bootstrap was *attempted*
//     -- which is the fact nothing else can reconstruct afterwards.
//  4. **Talos's own `AlreadyExists`.** The node refuses a second bootstrap
//     itself. It is last because it is the only one that runs after the RPC
//     has left, and by then everything else has already had its chance.
//
// The unclear case -- an intent record with no outcome -- goes to a recovery
// flow and never to a retry. A retry there is the exact operation the four
// mechanisms exist to prevent.

var (
	// ErrBootstrapInProgress reports that another bootstrap holds the lease.
	ErrBootstrapInProgress = errors.New("provision: a bootstrap is already running for this cluster")

	// ErrAlreadyBootstrapped reports that etcd is already up.
	ErrAlreadyBootstrapped = errors.New("provision: this cluster already has a running etcd")

	// ErrBootstrapUnclear reports an intent record with no outcome: a previous
	// bootstrap started and this process cannot say whether it took.
	//
	// It is a distinct error because the response to it is distinct. It is not
	// retried, ever -- it goes to a person, with what is known.
	ErrBootstrapUnclear = errors.New("provision: a previous bootstrap attempt has no recorded outcome")
)

// BootstrapIntent is the fsynced record written before the call.
//
// It is deliberately tiny and self-describing: whoever reads it is reading it
// after a crash, possibly from a different build, and every field they need to
// decide has to be in it rather than inferrable from somewhere else.
type BootstrapIntent struct {
	Cluster model.ClusterID `json:"cluster"`
	Machine model.MachineID `json:"machine"`
	Addr    string          `json:"addr"`

	StartedAt time.Time `json:"started_at"`

	// Outcome is empty while the call is in flight, and one of "succeeded",
	// "already-exists" or "failed" afterwards. An empty outcome on a record
	// that is not being held by a running process is the unclear case.
	Outcome string `json:"outcome,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// Bootstrapper owns the lease directory.
type Bootstrapper struct {
	dir string
}

// NewBootstrapper builds one over a directory inside the data directory.
//
// The directory is created 0700 like everything else holzkube-manager writes: the
// intent records name cluster ids and addresses, which is not a secret but is
// not the neighbours' business either.
func NewBootstrapper(dir string) (*Bootstrapper, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("provision: create the bootstrap lease directory: %w", err)
	}
	return &Bootstrapper{dir: dir}, nil
}

// Bootstrap initialises etcd on a control-plane node, or refuses.
//
// All four mechanisms run, in the order they are described above. The function
// is deliberately long and deliberately not decomposed into "acquire" and
// "call": the order is the safety property, and splitting it into pieces a
// caller sequences would make the order somebody else's responsibility.
func (b *Bootstrapper) Bootstrap(
	ctx context.Context,
	cc *talos.ClusterClient,
	cluster model.ClusterID,
	machine model.MachineID,
	addr string,
) error {
	path := b.intentPath(cluster)

	// --- Mechanism 3, read half. An intent from a previous run is checked
	// before anything else, because if it is unclear then nothing below it may
	// run at all.
	if prev, err := b.readIntent(path); err == nil {
		switch {
		case prev.Outcome == "":
			return fmt.Errorf("%w: one started at %s against %s and this process cannot say "+
				"whether it took effect. Look at the cluster -- `talosctl etcd members` against "+
				"%s answers it -- and then clear the attempt",
				ErrBootstrapUnclear, prev.StartedAt.Format(time.RFC3339), prev.Machine, prev.Addr)
		case prev.Outcome == "succeeded", prev.Outcome == "already-exists":
			return fmt.Errorf("%w: bootstrapped at %s", ErrAlreadyBootstrapped, prev.StartedAt.Format(time.RFC3339))
		}
		// A recorded failure is the one case a second attempt is allowed, and
		// the record is replaced below.
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("provision: clear the failed bootstrap record: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Every node call below gets an explicit budget, because a job's context
	// deliberately has none -- a job outlives the request that asked for it --
	// and the deadline gate refuses a call without one. It is applied here,
	// once, rather than at each call site: this function's order is its safety
	// property and a deadline threaded through it in three places would be
	// three places to get it wrong.
	readCtx, cancelRead, err := talos.WithClassDeadline(ctx, talos.MethodEtcdMemberList)
	if err != nil {
		return err
	}

	// --- Mechanism 1: does etcd already answer?
	//
	// A positive answer is decisive. A negative one proves nothing -- a node
	// whose etcd has not started looks the same as one that was never
	// bootstrapped -- which is why it is only the first of four.
	members, memberErr := cc.EtcdMemberList(readCtx)
	cancelRead()
	if memberErr == nil && len(members) > 0 {
		return fmt.Errorf("%w: %d member(s) already answer", ErrAlreadyBootstrapped, len(members))
	}

	// --- Mechanism 2: the lease. O_CREAT|O_EXCL is the only atomic step here,
	// and it is the only one that covers two operators clicking at once.
	intent := BootstrapIntent{
		Cluster:   cluster,
		Machine:   machine,
		Addr:      addr,
		StartedAt: time.Now().UTC(),
	}
	if err := b.writeIntentExclusive(path, intent); err != nil {
		return err
	}

	// --- Mechanism 4: the call, and Talos's own refusal.
	callCtx, cancelCall, err := talos.WithClassDeadline(ctx, talos.MethodBootstrap)
	if err != nil {
		return err
	}
	defer cancelCall()

	err = cc.Bootstrap(callCtx)

	switch {
	case err == nil:
		intent.Outcome = "succeeded"
	case isAlreadyExists(err):
		// The node says it is already bootstrapped. This is not a failure of
		// the operation -- the cluster is in the state that was wanted -- and
		// recording it as one would send somebody to fix something that works.
		intent.Outcome = "already-exists"
		intent.Detail = "the node reported that etcd was already initialised"
	default:
		intent.Outcome = "failed"
		intent.Detail = err.Error()
	}

	if writeErr := b.overwriteIntent(path, intent); writeErr != nil {
		// The call happened and the outcome could not be recorded. That is
		// exactly the unclear state, and saying so is better than returning
		// the call's own error and leaving a record that claims nothing
		// happened.
		return fmt.Errorf("%w: the bootstrap ran and its outcome could not be written (%v). "+
			"Check the cluster before doing anything else", ErrBootstrapUnclear, writeErr)
	}

	if intent.Outcome == "failed" {
		return err
	}
	return nil
}

// PendingIntent returns the unclear record for a cluster, if there is one.
//
// It is what the recovery screen reads: an operator being asked to decide
// needs to know when the attempt was, which machine it was against, and what
// to check.
func (b *Bootstrapper) PendingIntent(cluster model.ClusterID) (BootstrapIntent, bool) {
	intent, err := b.readIntent(b.intentPath(cluster))
	if err != nil || intent.Outcome != "" {
		return BootstrapIntent{}, false
	}
	return intent, true
}

// Pending returns every unclear record, across all clusters.
//
// It is what the recovery screen is built from, and it walks the directory
// rather than taking a cluster id because an operator who has just found a
// half-finished bootstrap does not know which cluster it was for -- that is
// the thing they came here to find out.
//
// A record still being written by a bootstrap that is running right now looks
// exactly like one left by a crash, because there is nothing in the file that
// distinguishes them. Callers say so: the job list is where "still running"
// is known, and an attempt whose job is alive is not a case for recovery.
func (b *Bootstrapper) Pending() ([]BootstrapIntent, error) {
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		return nil, fmt.Errorf("provision: read the bootstrap lease directory: %w", err)
	}

	out := make([]BootstrapIntent, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		intent, err := b.readIntent(filepath.Join(b.dir, e.Name()))
		if err != nil {
			// A record that cannot be read is the loudest case there is:
			// something was written about a bootstrap and nothing can say
			// what. It is reported with the cluster its filename names and an
			// empty everything else, because inventing the missing fields
			// would be inventing facts about a bootstrap.
			out = append(out, BootstrapIntent{
				Cluster: model.ClusterID(strings.TrimSuffix(e.Name(), ".json")),
				Detail:  "this record could not be read: " + err.Error(),
			})
			continue
		}
		if intent.Outcome == "" {
			out = append(out, intent)
		}
	}
	return out, nil
}

// Resolve closes an unclear attempt with what a person found.
//
// It takes the operator's verdict rather than re-probing, deliberately: the
// reason the attempt is unclear is that probing cannot answer it, and a
// function that probed again and then asked would be pretending otherwise.
func (b *Bootstrapper) Resolve(cluster model.ClusterID, bootstrapped bool, note string) error {
	path := b.intentPath(cluster)

	intent, err := b.readIntent(path)
	if err != nil {
		return err
	}
	if intent.Outcome != "" {
		return fmt.Errorf("provision: this attempt is already resolved as %q", intent.Outcome)
	}

	intent.Outcome = "failed"
	if bootstrapped {
		intent.Outcome = "succeeded"
	}
	intent.Detail = "resolved by the operator: " + note

	return b.overwriteIntent(path, intent)
}

// Clear removes a cluster's bootstrap record entirely.
//
// It exists for the case the record is about a cluster that no longer exists.
// It is not part of the recovery flow: resolving an unclear attempt records
// what happened, and deleting the record instead would throw away the one
// piece of evidence there is.
func (b *Bootstrapper) Clear(cluster model.ClusterID) error {
	if err := os.Remove(b.intentPath(cluster)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (b *Bootstrapper) intentPath(cluster model.ClusterID) string {
	return filepath.Join(b.dir, string(cluster)+".json")
}

// writeIntentExclusive is the lease: it fails if the file exists.
//
// O_CREAT|O_EXCL is atomic in the kernel, which is what makes this the one
// mechanism here that two processes cannot both pass. The fsync is what makes
// the record survive the crash it exists to describe: without it, a machine
// that loses power mid-bootstrap may come back with no record at all, which
// reads as "nothing was attempted".
func (b *Bootstrapper) writeIntentExclusive(path string, intent BootstrapIntent) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("%w: %s", ErrBootstrapInProgress, path)
		}
		return fmt.Errorf("provision: take the bootstrap lease: %w", err)
	}
	defer f.Close() //nolint:errcheck // the sync below is what matters

	raw, err := json.MarshalIndent(intent, "", "  ")
	if err != nil {
		return err
	}
	if _, err := f.Write(raw); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("provision: fsync the bootstrap intent: %w", err)
	}
	return syncDir(filepath.Dir(path))
}

// overwriteIntent replaces the record with its outcome.
//
// Written in place rather than through a temporary file and a rename, which is
// the opposite of what the store does and is deliberate: the file *is* the
// lease, and a rename would briefly leave the path free for a second process
// to take.
func (b *Bootstrapper) overwriteIntent(path string, intent BootstrapIntent) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // the sync below is what matters

	raw, err := json.MarshalIndent(intent, "", "  ")
	if err != nil {
		return err
	}
	if _, err := f.Write(raw); err != nil {
		return err
	}
	return f.Sync()
}

func (b *Bootstrapper) readIntent(path string) (BootstrapIntent, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // this package owns this directory
	if err != nil {
		return BootstrapIntent{}, err
	}
	var intent BootstrapIntent
	if err := json.Unmarshal(raw, &intent); err != nil {
		// A record that does not parse is the unclear case by another route:
		// something was written and cannot be read, and guessing would be
		// guessing about a bootstrap.
		return BootstrapIntent{}, fmt.Errorf("%w: the record at %s does not parse: %w",
			ErrBootstrapUnclear, path, err)
	}
	return intent, nil
}

// syncDir makes a newly created file's *existence* durable, which fsyncing the
// file alone does not do.
func syncDir(dir string) error {
	d, err := os.Open(dir) //nolint:gosec // this package owns this directory
	if err != nil {
		return err
	}
	defer d.Close() //nolint:errcheck // the sync below is what matters
	return d.Sync()
}

// isAlreadyExists recognises Talos's refusal of a second bootstrap.
//
// Both the gRPC code and the message are checked. The code is the contract;
// the message is the fallback for a path that wraps the status and loses it,
// which is a thing that happens between a node, apid and a client library.
func isAlreadyExists(err error) bool {
	if st, ok := status.FromError(err); ok && st.Code() == codes.AlreadyExists {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "alreadyexists") ||
		strings.Contains(strings.ToLower(err.Error()), "already exists")
}
