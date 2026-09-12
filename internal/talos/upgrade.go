package talos

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/siderolabs/talos/pkg/machinery/api/common"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
)

// The upgrade RPCs.
//
// An upgrade is the only operation in this product that replaces a running
// system while it is running, and the shape of the API says so: it is a
// **stream**, not a call that returns when the work is done. The node writes
// the installer's own output and then stops answering, because it is
// rebooting into what it just wrote.
//
// That is why nothing here waits for a node to come back. The stream ending is
// not the upgrade succeeding -- it is the node leaving. What decides whether
// an upgrade worked is a separate read, afterwards, against the node's own
// report of what it is running (UPG-07).

// LifecycleService is the service the streaming upgrade lives on.
//
// It is separate from MachineService because Talos v1.13 moved install and
// upgrade onto it, and the deprecated MachineService.Upgrade is the one this
// package does *not* call -- see Upgrade.
const lifecycleService = "/machine.LifecycleService/"

// MethodLifecycleUpgrade and MethodImagePull are the two calls an upgrade
// makes, named here so the deadline class table can carry them.
const (
	MethodLifecycleUpgrade = lifecycleService + "Upgrade"
	MethodImagePull        = machineService + "ImagePull"
)

// UpgradeProgress is one line of the node's own account of an upgrade.
type UpgradeProgress struct {
	// Message is a line the installer wrote. It is the node's text, not this
	// package's: an upgrade that fails fails inside the installer, and
	// paraphrasing it would remove the only thing that says why.
	Message string

	// ExitCode is set on the final message, and only there. A non-zero one is
	// the installer refusing, which is a different thing from the stream
	// breaking because the node rebooted.
	ExitCode int32
	Exited   bool
}

// UpgradeStream is a running upgrade.
//
// It carries the same release obligation as LogStream and for the same reason:
// the deadline context the stream interceptor derives is released by Close and,
// absent an error, by nothing else.
type UpgradeStream struct {
	s      machine.LifecycleService_UpgradeClient
	cancel context.CancelFunc
}

// ImagePull pulls an image into the node's containerd.
//
// It is a separate call from Upgrade because the upgrade API requires it: the
// request names an image **that is already on the node**, and the reason is
// worth keeping rather than hiding behind a convenience wrapper. Pulling is
// where a wrong installer reference, an unreachable registry or a schematic
// that does not exist fails -- and it fails while the node is still running
// the system it is running. Failing there costs nothing. Failing after the
// upgrade has started costs the node.
func (c *ClusterClient) ImagePull(ctx context.Context, ref string) error {
	// machinery deprecates this in favour of ImageServiceClient, and the swap
	// is deliberately not made here. ImageService.Pull is a *stream* where this
	// is a unary call: taking it needs a new deadline-class row, a talossim
	// handler, a coverage-guard entry and a change to the wire call the upgrade
	// path makes -- on the one path in this product that has never run against
	// real hardware (window 85). Changing what an unverified path sends, to
	// silence a deprecation on a method Talos v1.13 still serves, would be
	// trading a warning for an unknown. It belongs in the same session that
	// first runs an upgrade on a real machine.
	return c.conn.c.ImagePull(ctx, common.ContainerdNamespace_NS_SYSTEM, ref) //nolint:staticcheck // see above
}

// Upgrade starts an upgrade and returns the node's own output as it arrives.
//
// It uses LifecycleService rather than the deprecated MachineService.Upgrade,
// which is what UPG-01 asks for and is also the only one that streams. The
// difference is not cosmetic: the unary one returns as soon as the node has
// accepted the request, so everything the installer says afterwards -- which
// is everything that says what went wrong -- is lost.
//
// The image must already be on the node. See ImagePull.
//
// The caller must Close the returned stream.
func (c *ClusterClient) Upgrade(ctx context.Context, ref string) (*UpgradeStream, error) {
	if ref == "" {
		return nil, errors.New("talos: an upgrade must name the installer image to upgrade to; " +
			"there is no default, and a default would be this build's opinion about a node's future")
	}

	streamCtx, cancel := context.WithCancel(ctx)

	s, err := c.conn.c.LifecycleClient.Upgrade(streamCtx, &machine.LifecycleServiceUpgradeRequest{
		Source: &machine.InstallArtifactsSource{ImageName: ref},
	})
	if err != nil {
		cancel()
		return nil, err
	}
	return &UpgradeStream{s: s, cancel: cancel}, nil
}

// Recv returns the next line of the upgrade's output.
//
// io.EOF ends the stream. It is **not** proof the upgrade succeeded: a node
// that is rebooting into the system it just wrote stops answering, and that
// looks the same from here. UPG-07 is the answer to that, and it is a separate
// read against the node afterwards.
func (u *UpgradeStream) Recv() (UpgradeProgress, error) {
	msg, err := u.s.Recv()
	if err != nil {
		return UpgradeProgress{}, err
	}

	progress := msg.GetProgress()
	switch r := progress.GetResponse().(type) {
	case *machine.LifecycleServiceInstallProgress_Message:
		return UpgradeProgress{Message: r.Message}, nil
	case *machine.LifecycleServiceInstallProgress_ExitCode:
		return UpgradeProgress{ExitCode: r.ExitCode, Exited: true}, nil
	default:
		// A message shape this build does not know. Reported as an empty line
		// rather than dropped: a stream that silently skips what it cannot
		// read is a stream whose output is not the node's output.
		return UpgradeProgress{}, nil
	}
}

// Close releases the stream.
func (u *UpgradeStream) Close() error {
	u.cancel()
	return nil
}

// Drain reads an upgrade stream to its end, handing each line to onLine.
//
// It exists because every caller wants the same thing and the same three
// endings, and getting the third one wrong is the failure this whole file is
// about:
//
//   - io.EOF, which means the node stopped talking. That is the ordinary end
//     of a successful upgrade and also what a node that fell over looks like.
//   - a non-zero exit code, which is the installer refusing. That is a
//     failure, and the node is still running what it was running.
//   - any other error, which is the connection going away mid-upgrade. It is
//     reported as-is and is deliberately **not** turned into a failure: the
//     upgrade may well be proceeding, and the node is the only thing that can
//     say.
func (u *UpgradeStream) Drain(onLine func(UpgradeProgress)) error {
	for {
		p, err := u.Recv()
		switch {
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			return err
		}

		if onLine != nil {
			onLine(p)
		}
		if p.Exited && p.ExitCode != 0 {
			return fmt.Errorf("talos: the installer exited with status %d; the node is still "+
				"running the system it was running", p.ExitCode)
		}
	}
}
