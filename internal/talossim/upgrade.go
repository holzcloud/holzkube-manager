package talossim

import (
	"context"
	"fmt"
	"strings"

	"github.com/siderolabs/talos/pkg/machinery/api/common"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// The upgrade and etcd-management RPCs, phase 9.
//
// The rule this file keeps is the one the whole simulator keeps: a mutation
// must be observable through a later read. An upgrade that only streamed
// plausible-looking lines and left the node reporting the same version would
// be a simulator that cannot tell a working upgrade from a no-op -- which is
// exactly the thing UPG-07 exists because nobody can tell from the API alone.
//
// So an upgrade here **changes what Version reports**, and it does so only
// after the stream has finished. A test that checks the version too early sees
// the old one, which is what a real node does too.

// lifecycleService serves the streaming install and upgrade.
type lifecycleService struct {
	machine.UnimplementedLifecycleServiceServer
	server *Server
}

// Install is not implemented, and deliberately so: holzkube-manager provisions a
// machine by applying a configuration in maintenance mode and letting the node
// install itself, which is the path Talos documents. A simulator that answered
// this RPC would be offering a second way to install that the product does not
// take.
//
// It is left inherited from the embedded server rather than written as an
// explicit Unimplemented, so that the coverage guard reports it honestly if
// anybody ever calls it.

// Upgrade streams an upgrade and then changes what the node reports.
//
// The version is taken from the installer image reference, because that is
// where a real upgrade's version comes from as well -- the tag on the
// installer is what gets written to disk. A simulator that took the version
// from somewhere else would let a test pass with an image reference nobody
// checked.
func (l *lifecycleService) Upgrade(
	req *machine.LifecycleServiceUpgradeRequest,
	stream machine.LifecycleService_UpgradeServer,
) error {
	l.server.recordCall("Upgrade")

	if err := l.server.node.up(); err != nil {
		return err
	}

	ref := req.GetSource().GetImageName()
	if ref == "" {
		return status.Error(codes.InvalidArgument,
			"talossim: the upgrade request names no installer image")
	}

	// The node refuses an image it has not pulled, which is what the real
	// LifecycleService does and the reason ImagePull is a separate call: a
	// wrong reference fails here, while the node is still running the system
	// it is running.
	if !l.server.node.hasImage(ref) {
		return status.Errorf(codes.FailedPrecondition,
			"talossim: %s has not been pulled onto this node", ref)
	}

	version, err := versionFromInstaller(ref)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	if l.server.upgradeRefused() {
		for _, line := range []string{
			"downloading " + ref,
			"unpacking the installer",
		} {
			if err := sendUpgradeLine(stream, line); err != nil {
				return err
			}
		}
		// A non-zero exit code, which is the installer refusing. The node is
		// still running what it was running, and that is the point: this is a
		// different outcome from the connection dying mid-upgrade, and a
		// caller that conflated them would report a working node as lost.
		return stream.Send(&machine.LifecycleServiceUpgradeResponse{
			Progress: &machine.LifecycleServiceInstallProgress{
				Response: &machine.LifecycleServiceInstallProgress_ExitCode{ExitCode: 1},
			},
		})
	}

	for _, line := range []string{
		"downloading " + ref,
		"unpacking the installer",
		"writing the system partition",
		"installed " + version,
	} {
		if err := sendUpgradeLine(stream, line); err != nil {
			return err
		}
	}

	// Only now. Everything above was the node talking; this is the node having
	// done it.
	l.server.node.upgradeTo(version)

	return stream.Send(&machine.LifecycleServiceUpgradeResponse{
		Progress: &machine.LifecycleServiceInstallProgress{
			Response: &machine.LifecycleServiceInstallProgress_ExitCode{ExitCode: 0},
		},
	})
}

func sendUpgradeLine(stream machine.LifecycleService_UpgradeServer, line string) error {
	return stream.Send(&machine.LifecycleServiceUpgradeResponse{
		Progress: &machine.LifecycleServiceInstallProgress{
			Response: &machine.LifecycleServiceInstallProgress_Message{Message: line},
		},
	})
}

// versionFromInstaller reads the version off an installer reference.
//
// `ghcr.io/siderolabs/installer:v1.14.0` and
// `factory.talos.dev/installer/<schematic>:v1.14.0` both carry it as the tag,
// and a digest carries no version at all -- which is refused rather than
// guessed, for the same reason KubernetesVersion refuses a digest.
func versionFromInstaller(ref string) (string, error) {
	if strings.Contains(ref, "@") {
		return "", fmt.Errorf("talossim: %q names an image by digest, which carries no version", ref)
	}
	i := strings.LastIndex(ref, ":")
	if i < 0 || strings.Contains(ref[i+1:], "/") {
		return "", fmt.Errorf("talossim: %q carries no version tag", ref)
	}
	return strings.TrimPrefix(ref[i+1:], "v"), nil
}

// ImagePull records that an image is on the node.
//
// It is a real state change and not a no-op, because Upgrade refuses an image
// that has not been pulled -- which is what makes the two-call shape in
// internal/talos/upgrade.go testable rather than a claim in a comment.
func (m *machineService) ImagePull(_ context.Context, req *machine.ImagePullRequest) (*machine.ImagePullResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	ref := req.GetReference()
	if ref == "" {
		return nil, status.Error(codes.InvalidArgument, "talossim: the pull request names no image")
	}
	if _, err := versionFromInstaller(ref); err != nil {
		// A reference this node cannot make sense of fails at the pull, which
		// is where a wrong installer reference should fail: before anything
		// has been replaced.
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	m.server.node.pullImage(ref)

	return &machine.ImagePullResponse{
		Messages: []*machine.ImagePull{{Metadata: m.server.node.metadata()}},
	}, nil
}

// EtcdAlarmList reports the alarms etcd has raised.
func (m *machineService) EtcdAlarmList(_ context.Context, _ *emptypb.Empty) (*machine.EtcdAlarmListResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	n := m.server.node.snapshot()
	if !n.Bootstrapped {
		return nil, status.Errorf(codes.FailedPrecondition,
			"talossim: etcd is not running on %s: the node has not been bootstrapped", n.Hostname)
	}

	var alarms []*machine.EtcdMemberAlarm
	if alarm := m.server.etcdAlarm(); alarm != "" {
		alarms = append(alarms, &machine.EtcdMemberAlarm{
			MemberId: memberID(n.Hostname),
			Alarm:    machine.EtcdMemberAlarm_AlarmType(machine.EtcdMemberAlarm_AlarmType_value[alarm]),
		})
	}

	return &machine.EtcdAlarmListResponse{
		Messages: []*machine.EtcdAlarm{{
			Metadata:     m.server.node.metadata(),
			MemberAlarms: alarms,
		}},
	}, nil
}

// EtcdRemoveMemberByID removes a member from the membership.
//
// The simulated cluster has one member, so the only id this can be asked for
// is this node's own -- and removing it is refused, because a member cannot
// remove itself through this RPC on a real node either. That refusal is the
// behaviour worth simulating: it is what sends the caller to EtcdLeaveCluster.
func (m *machineService) EtcdRemoveMemberByID(_ context.Context, req *machine.EtcdRemoveMemberByIDRequest) (*machine.EtcdRemoveMemberByIDResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	n := m.server.node.snapshot()
	if !n.Bootstrapped {
		return nil, status.Errorf(codes.FailedPrecondition,
			"talossim: etcd is not running on %s: the node has not been bootstrapped", n.Hostname)
	}

	if req.GetMemberId() == memberID(n.Hostname) {
		return nil, status.Errorf(codes.InvalidArgument,
			"talossim: %s cannot remove itself from etcd; that is what EtcdLeaveCluster is for",
			n.Hostname)
	}

	m.server.node.removeMember(req.GetMemberId())

	return &machine.EtcdRemoveMemberByIDResponse{
		Messages: []*machine.EtcdRemoveMemberByID{{Metadata: m.server.node.metadata()}},
	}, nil
}

// EtcdForfeitLeadership hands leadership to another member.
//
// A one-member cluster has nobody to hand it to, and the honest answer is an
// empty member name rather than an error: forfeiting leadership you are the
// only candidate for is not a failure, it is a no-op, and a caller that
// treated it as a failure would refuse to upgrade a single-node cluster.
func (m *machineService) EtcdForfeitLeadership(_ context.Context, _ *machine.EtcdForfeitLeadershipRequest) (*machine.EtcdForfeitLeadershipResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	n := m.server.node.snapshot()
	if !n.Bootstrapped {
		return nil, status.Errorf(codes.FailedPrecondition,
			"talossim: etcd is not running on %s: the node has not been bootstrapped", n.Hostname)
	}

	return &machine.EtcdForfeitLeadershipResponse{
		Messages: []*machine.EtcdForfeitLeadership{{Metadata: m.server.node.metadata()}},
	}, nil
}

// EtcdLeaveCluster makes this node leave etcd, and stops etcd.
//
// The node stops reporting itself as bootstrapped, which is what makes the
// mutation observable: EtcdMemberList and EtcdStatus both refuse afterwards,
// exactly as they do on a node that was never bootstrapped. Those two states
// really are indistinguishable from outside on a real node too.
func (m *machineService) EtcdLeaveCluster(_ context.Context, _ *machine.EtcdLeaveClusterRequest) (*machine.EtcdLeaveClusterResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	n := m.server.node.snapshot()
	if !n.Bootstrapped {
		return nil, status.Errorf(codes.FailedPrecondition,
			"talossim: etcd is not running on %s: the node has not been bootstrapped", n.Hostname)
	}

	m.server.node.leaveEtcd()

	return &machine.EtcdLeaveClusterResponse{
		Messages: []*machine.EtcdLeaveCluster{{Metadata: m.server.node.metadata()}},
	}, nil
}

// EtcdSnapshot streams a snapshot of the etcd database.
//
// The bytes are not a real bolt database, and nothing in this product parses
// them: what is being simulated is that a snapshot is a **stream** whose
// length is not known in advance, and that it fails outright when etcd cannot
// confirm it is reading something current. That refusal is UPG-12's whole
// point, so it is the part that has to be real here.
func (m *machineService) EtcdSnapshot(_ *machine.EtcdSnapshotRequest, stream machine.MachineService_EtcdSnapshotServer) error {
	m.server.recordCall("EtcdSnapshot")

	if err := m.server.node.up(); err != nil {
		return err
	}

	n := m.server.node.snapshot()
	if !n.Bootstrapped {
		return status.Errorf(codes.FailedPrecondition,
			"talossim: etcd is not running on %s: the node has not been bootstrapped", n.Hostname)
	}

	if !m.server.etcdHasQuorum() {
		return status.Error(codes.FailedPrecondition,
			"talossim: etcd cannot confirm a quorum, so it cannot take a consistent snapshot")
	}

	for i := range 3 {
		chunk := []byte(fmt.Sprintf("talossim-etcd-snapshot-chunk-%d-of-%s\n", i, n.Hostname))
		if err := stream.Send(&common.Data{Bytes: chunk}); err != nil {
			return err
		}
	}
	return nil
}

// The phase-9 controls.
//
// These are **not** scenarios. The nine in Registry are TRANS-07's own set,
// named by the requirement and closed by a test in both directions, and a
// condition nobody wrote into the requirement does not belong there: an extra
// entry carries an ExpectedClient string nobody agreed to, which is exactly
// what that test refuses.
//
// What these are instead is the node's own state, set directly, the way
// Options sets the hostname. An etcd with a NOSPACE alarm and an etcd that has
// lost quorum are not faults injected into the transport -- they are states a
// real cluster is in, and the health gate's whole job is to read them.

// SetEtcdAlarm makes the node report an alarm against its own member.
//
// name is etcd's own token: "NOSPACE" or "CORRUPT". An empty string clears it.
// Both are conditions under which an upgrade must not start, and they are
// distinguished because the remedies are opposite -- one is a disk, the other
// is a database.
func (s *Server) SetEtcdAlarm(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.etcdAlarmType = name
}

func (s *Server) etcdAlarm() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.etcdAlarmType
}

// SetEtcdQuorum sets whether etcd can confirm a quorum.
//
// A cluster without one still answers a member list -- each member knows who
// it thinks is there -- and cannot take a snapshot, because it cannot confirm
// that what it would read is current. That asymmetry is UPG-12's whole point,
// so it is the part the simulator has to get right.
func (s *Server) SetEtcdQuorum(has bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.etcdNoQuorum = !has
}

func (s *Server) etcdHasQuorum() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.etcdNoQuorum
}

// SetUpgradeRefused makes the installer exit non-zero.
//
// It is the outcome that is easiest to get wrong at the call site: the node is
// still running what it was running, and reporting it the way a dropped
// connection is reported would send somebody to look for a node that is fine.
func (s *Server) SetUpgradeRefused(refused bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upgradeRefuses = refused
}

func (s *Server) upgradeRefused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upgradeRefuses
}
