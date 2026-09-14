package upgrade_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// The half of this package that talks to a node.
//
// Everything here runs against talossim, whose Upgrade actually changes what
// Version reports afterwards -- which is what makes UPG-07 testable at all. A
// simulator that streamed plausible lines and left the version alone would let
// a verification that never looked pass.

func liveNode(t *testing.T) (*talossim.Server, *talos.ClusterClient) {
	t.Helper()

	cl, err := talossim.NewCluster("homelab", "https://10.0.0.10:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	sim, err := talossim.New(talossim.Options{
		Hostname: "cp-1", Cluster: cl, ControlPlane: true,
	})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)

	cc, err := talos.NewClusterClient(ctx, sim.Dialer(), talos.Target{
		Cluster: "c1", Machine: "00000000-0000-4000-8000-000000000001", Addr: sim.Host(),
	}, sim.ClientCreds(), talos.Mode{})
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}
	t.Cleanup(func() { _ = cc.Close() })

	// Bootstrapped, so the etcd reads answer. A node that was never
	// bootstrapped refuses them, which is a different test.
	bootstrapCtx, cancel2, err := talos.WithClassDeadline(ctx, talos.MethodBootstrap)
	if err != nil {
		t.Fatalf("WithClassDeadline: %v", err)
	}
	defer cancel2()
	if err := cc.Bootstrap(bootstrapCtx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	return sim, cc
}

// TestAnUpgradeChangesWhatTheNodeReports is UPG-07 in its simplest form.
//
// The point is not that the stream produced lines. It is that the node,
// afterwards, reports the version that was installed -- which is the only
// thing that distinguishes an upgrade from a stream of text.
func TestAnUpgradeChangesWhatTheNodeReports(t *testing.T) {
	t.Parallel()

	_, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	const installer = "ghcr.io/siderolabs/installer:v1.14.0"

	// The pull comes first. That the node refuses an unpulled image is
	// TestAnUnpulledImageIsRefusedBeforeAnythingIsReplaced's job; here it is
	// simply done in the order the upgrade path does it.
	pullCtx, cancelPull, err := talos.WithClassDeadline(ctx, talos.MethodImagePull)
	if err != nil {
		t.Fatalf("WithClassDeadline: %v", err)
	}
	if err := cc.ImagePull(pullCtx, installer); err != nil {
		t.Fatalf("ImagePull: %v", err)
	}
	cancelPull()

	stream, err := cc.Upgrade(ctx, installer)
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	defer stream.Close() //nolint:errcheck // the test's verdict is its own

	var lines []string
	if err := stream.Drain(func(p talos.UpgradeProgress) {
		if p.Message != "" {
			lines = append(lines, p.Message)
		}
	}); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	if len(lines) == 0 {
		t.Fatal("the upgrade stream produced no output at all; an upgrade that says nothing is " +
			"an upgrade whose failure arrives as a timeout")
	}

	// And the node is on the new version, with its services up.
	obs, err := upgrade.VerifyUpgraded(ctx, cc, v(t, "v1.14.0"), "")
	if err != nil {
		t.Fatalf("the node did not verify as upgraded: %v", err)
	}
	if !strings.Contains(obs.TalosVersion, "1.14.0") {
		t.Errorf("the node reports %q after an upgrade to v1.14.0", obs.TalosVersion)
	}
}

// TestAnUpgradeThatDidNotHappenIsNotReportedAsSuccess is the other direction,
// and it is the one UPG-07 exists for.
//
// "The API said OK" is not proof. Verification against a version the node is
// not running has to fail, or the check is decoration.
func TestAnUpgradeThatDidNotHappenIsNotReportedAsSuccess(t *testing.T) {
	t.Parallel()

	_, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	_, err := upgrade.VerifyUpgraded(ctx, cc, v(t, "v1.14.0"), "")
	if !errors.Is(err, upgrade.ErrNotUpgraded) {
		t.Fatalf("verifying an un-upgraded node returned %v, want ErrNotUpgraded", err)
	}
	if !strings.Contains(err.Error(), "the API said OK") {
		t.Errorf("the failure %q does not say what kind of mistake it is catching", err)
	}
}

// TestAnInstallerThatRefusesIsNotTheNodeDisappearing.
//
// The two endings look alike from the caller's side and mean opposite things:
// a non-zero exit code is the installer refusing, and the node is still
// running what it was running. Reporting it the way a dropped connection is
// reported sends somebody to look for a node that is fine.
func TestAnInstallerThatRefusesIsNotTheNodeDisappearing(t *testing.T) {
	t.Parallel()

	sim, cc := liveNode(t)
	sim.SetUpgradeRefused(true)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	const installer = "ghcr.io/siderolabs/installer:v1.14.0"

	pullCtx, cancelPull, err := talos.WithClassDeadline(ctx, talos.MethodImagePull)
	if err != nil {
		t.Fatalf("WithClassDeadline: %v", err)
	}
	if err := cc.ImagePull(pullCtx, installer); err != nil {
		t.Fatalf("ImagePull: %v", err)
	}
	cancelPull()

	stream, err := cc.Upgrade(ctx, installer)
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	defer stream.Close() //nolint:errcheck // the verdict is Drain's

	err = stream.Drain(nil)
	if err == nil {
		t.Fatal("an installer that exited non-zero was reported as a successful upgrade")
	}
	if !strings.Contains(err.Error(), "still running") {
		t.Errorf("the failure %q does not say that the node is intact", err)
	}

	// And the node really is intact, on its old version.
	obs, verr := upgrade.VerifyUpgraded(ctx, cc, v(t, "v1.13.9"), "")
	if verr != nil {
		t.Fatalf("the node is not on its original version after a refused upgrade: %v", verr)
	}
	if !strings.Contains(obs.TalosVersion, "1.13.9") {
		t.Errorf("the node reports %q", obs.TalosVersion)
	}
}

// TestAnUnpulledImageIsRefusedBeforeAnythingIsReplaced.
//
// A wrong installer reference must fail while the node is still running the
// system it is running. Failing there costs nothing; failing after the upgrade
// has started costs the node.
func TestAnUnpulledImageIsRefusedBeforeAnythingIsReplaced(t *testing.T) {
	t.Parallel()

	_, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	stream, err := cc.Upgrade(ctx, "ghcr.io/siderolabs/installer:v1.14.0")
	if err == nil {
		err = stream.Drain(nil)
		_ = stream.Close()
	}
	if err == nil {
		t.Fatal("an upgrade to an image that was never pulled was accepted")
	}

	// Unchanged.
	if _, verr := upgrade.VerifyUpgraded(ctx, cc, v(t, "v1.13.9"), ""); verr != nil {
		t.Errorf("the node is not on its original version: %v", verr)
	}
}

// TestTheMembershipIsNamedByHostnameAndNeverOnlyByHexID is UPG-10.
//
// An operator asked to confirm the removal of "8e9e05c52164694d" is confirming
// a string, not a machine.
func TestTheMembershipIsNamedByHostnameAndNeverOnlyByHexID(t *testing.T) {
	t.Parallel()

	_, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	list, err := upgrade.Members(ctx, cc, []model.Machine{
		{ID: "00000000-0000-4000-8000-000000000001", Hostname: "cp-1"},
	})
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	if len(list.Members) != 1 {
		t.Fatalf("the membership has %d member(s): %+v", len(list.Members), list.Members)
	}

	m := list.Members[0]
	if !strings.Contains(m.Name, "cp-1") {
		t.Errorf("the member is named %q, which does not carry its hostname", m.Name)
	}
	if m.Machine != "00000000-0000-4000-8000-000000000001" {
		t.Errorf("the member was not matched to the machine in the inventory: %q", m.Machine)
	}
	if m.ID == "" {
		t.Error("the member carries no id; the removal RPC needs one")
	}

	// The number every decision on this screen turns on, derived once.
	if list.VotingCount != 1 || list.Tolerates != 0 {
		t.Errorf("a one-member cluster reports %d voting and tolerates %d",
			list.VotingCount, list.Tolerates)
	}
	if !strings.Contains(list.Sentence, "majority") {
		t.Errorf("the sentence for a cluster that tolerates nothing does not explain why: %q",
			list.Sentence)
	}
}

// TestRemovingTheLastVotingMemberIsRefused is UPG-11's guard.
//
// A removal is not an upgrade: there is no node coming back afterwards, so
// this is the last point at which anything can say no.
func TestRemovingTheLastVotingMemberIsRefused(t *testing.T) {
	t.Parallel()

	_, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	list := upgrade.MemberList{
		Members: []upgrade.Member{
			{ID: "1", Name: "cp-1", Voting: true},
			{ID: "2", Name: "cp-2", Voting: true},
		},
		VotingCount: 2,
		Tolerates:   0,
	}

	err := upgrade.RemoveMember(ctx, cc, list, "2")
	if !errors.Is(err, upgrade.ErrLastVotingMember) {
		t.Fatalf("removing a voter from a two-member etcd returned %v, want ErrLastVotingMember", err)
	}
	if !strings.Contains(err.Error(), "cp-2") {
		t.Errorf("the refusal %q does not name the member", err)
	}

	// The other half of the pair above: this cluster survives as a cluster,
	// so the advice is the one that applies to a cluster.
	if !strings.Contains(err.Error(), "Add a control-plane node first") {
		t.Errorf("the refusal does not say what to do about it: %q", err)
	}
	if strings.Contains(err.Error(), "only etcd member") {
		t.Errorf("a two-member cluster was described as having one member: %q", err)
	}
}

// TestRemovingTheOnlyMemberIsRefused is the case the two-member test above
// does not reach, and the worse one.
//
// A cluster with two voting members tolerates no loss, so removing one stops
// it accepting writes -- bad, and recoverable by adding a member back. A
// cluster with one voting member has nothing after the removal at all: no
// quorum to rejoin, no member to add a member through, and a data directory
// that is now the only copy. There is no repair from here that is not a
// restore.
//
// Somebody in a one-node homelab clicking "remove from cluster" on their one
// control-plane node is the most ordinary way to arrive at it.
func TestRemovingTheOnlyMemberIsRefused(t *testing.T) {
	t.Parallel()

	_, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	list := upgrade.MemberList{
		Members:     []upgrade.Member{{ID: "1", Name: "cp-1", Voting: true}},
		VotingCount: 1,
		Tolerates:   0,
	}

	err := upgrade.RemoveMember(ctx, cc, list, "1")
	if !errors.Is(err, upgrade.ErrLastVotingMember) {
		t.Fatalf("removing the only etcd member returned %v, want ErrLastVotingMember", err)
	}
	if !strings.Contains(err.Error(), "cp-1") {
		t.Errorf("the refusal %q does not name the member", err)
	}

	// And it gives this cluster's advice rather than the other one's. The two
	// refusals share a sentinel and overlap in their conditions -- a cluster
	// of one also tolerates nothing -- so a test that checked only the
	// sentinel would pass while the operator was told to "add a control-plane
	// node first", which is advice about a cluster that still exists. There is
	// nothing to add a node to here.
	if !strings.Contains(err.Error(), "only etcd member") ||
		!strings.Contains(err.Error(), "restore from a snapshot") {
		t.Errorf("the refusal gives the wrong cluster's advice: %q", err)
	}
	if strings.Contains(err.Error(), "Add a control-plane node first") {
		t.Errorf("the refusal tells the operator to add a node to a cluster that would not exist: %q", err)
	}
}

// TestRemovingTheOnlyControlPlaneNodeIsRefused is the same rule as the two
// tests above, asked through the door an operator actually uses.
//
// RemoveMember is "take this etcd member out by id", which is the repair for a
// member that should not be there. node.remove-from-cluster is the button on a
// node's page, and it does something larger: leave etcd, then wipe the system
// disk. It went two phases reading no membership at all -- the refusal existed
// beside it and never ran, so the confirmation dialog was the only thing
// between a one-node cluster and a wipe, and a confirmation dialog is not a
// gate. It is a way of asking somebody whether they meant the thing they
// already clicked.
//
// The upgrade gate exempts a single-node cluster on purpose: it has no quorum
// to lose, and refusing there would make the smallest homelab unupgradeable.
// That exemption must not reach this operation, and this test is the
// difference between the two: an upgraded node comes back, and a removed one
// does not.
func TestRemovingTheOnlyControlPlaneNodeIsRefused(t *testing.T) {
	t.Parallel()

	sim, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	err := upgrade.RemoveNode(ctx, cc, model.Machine{
		ID:       "00000000-0000-4000-8000-000000000001",
		Role:     model.RoleControlPlane,
		Hostname: "cp-1",
	})
	if !errors.Is(err, upgrade.ErrLastVotingMember) {
		t.Fatalf("removing the only control-plane node returned %v, want ErrLastVotingMember", err)
	}
	if !strings.Contains(err.Error(), "only etcd member") {
		t.Errorf("the refusal gives the wrong cluster's advice: %q", err)
	}

	// And nothing happened to the node. A refusal that arrived after the
	// etcd leave, or after the reset, would be a refusal in name only.
	if !sim.Node().Bootstrapped {
		t.Error("the node left etcd before the refusal")
	}
	if sim.Node().Resets != 0 {
		t.Error("the node was reset despite the refusal")
	}
}

// TestRemovingAWorkerAsksEtcdNothing. A worker is not a member, so the
// membership is not its business -- and a cluster whose etcd cannot be reached
// must not be a cluster whose workers cannot be removed.
func TestRemovingAWorkerAsksEtcdNothing(t *testing.T) {
	t.Parallel()

	sim, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	if err := upgrade.RemoveNode(ctx, cc, model.Machine{
		ID:       "00000000-0000-4000-8000-000000000002",
		Role:     model.RoleWorker,
		Hostname: "w-1",
	}); err != nil {
		t.Fatalf("removing a worker: %v", err)
	}
	if sim.Node().Resets != 1 {
		t.Errorf("the worker was reset %d times, want once", sim.Node().Resets)
	}
}

// TestASnapshotIsBytesAndAZeroLengthOneIsNotABackup is UPG-12.
func TestASnapshotIsBytesAndAZeroLengthOneIsNotABackup(t *testing.T) {
	t.Parallel()

	sim, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	var buf bytes.Buffer
	n, err := upgrade.Snapshot(ctx, cc, &buf)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if n == 0 || int64(buf.Len()) != n {
		t.Fatalf("the snapshot reported %d bytes and wrote %d", n, buf.Len())
	}

	// Without quorum the RPC fails outright, and the failure carries the
	// notice explaining what is still possible -- which is the fallback UPG-12
	// asks to be documented.
	sim.SetEtcdQuorum(false)

	buf.Reset()
	_, err = upgrade.Snapshot(ctx, cc, &buf)
	if err == nil {
		t.Fatal("a snapshot succeeded against an etcd that cannot confirm a quorum")
	}
	if !strings.Contains(err.Error(), "database file") {
		t.Errorf("the failure %q does not name the fallback", err)
	}
}

// TestAnEtcdAlarmIsVisibleToTheGate closes the loop between the seam and the
// gate: the alarm the node raises is the alarm the gate refuses on.
func TestAnEtcdAlarmIsVisibleToTheGate(t *testing.T) {
	t.Parallel()

	sim, cc := liveNode(t)
	sim.SetEtcdAlarm("NOSPACE")

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	alarmCtx, cancel2, err := talos.WithClassDeadline(ctx, talos.MethodEtcdAlarmList)
	if err != nil {
		t.Fatalf("WithClassDeadline: %v", err)
	}
	defer cancel2()

	alarms, err := cc.EtcdAlarmList(alarmCtx)
	if err != nil {
		t.Fatalf("EtcdAlarmList: %v", err)
	}
	if len(alarms) != 1 || alarms[0].Type != "NOSPACE" {
		t.Fatalf("the node reports %+v, want one NOSPACE alarm", alarms)
	}
}

// TestANodeCannotRemoveItselfFromEtcd pins the refusal that sends the caller
// to EtcdLeaveCluster.
func TestANodeCannotRemoveItselfFromEtcd(t *testing.T) {
	t.Parallel()

	_, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	list, err := upgrade.Members(ctx, cc, nil)
	if err != nil {
		t.Fatalf("Members: %v", err)
	}

	// Three voters on paper so the quorum guard does not fire first; the
	// refusal being tested is the node's own.
	list.VotingCount = 3
	list.Tolerates = 1

	if err := upgrade.RemoveMember(ctx, cc, list, list.Members[0].ID); err == nil {
		t.Fatal("a node removed itself from etcd through EtcdRemoveMemberByID")
	}
}

// TestLeavingEtcdStopsIt, which is what makes the removal flow's first step
// observable rather than a claim.
func TestLeavingEtcdStopsIt(t *testing.T) {
	t.Parallel()

	_, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	leaveCtx, cancel2, err := talos.WithClassDeadline(ctx, talos.MethodEtcdLeaveCluster)
	if err != nil {
		t.Fatalf("WithClassDeadline: %v", err)
	}
	if err := cc.EtcdLeaveCluster(leaveCtx); err != nil {
		t.Fatalf("EtcdLeaveCluster: %v", err)
	}
	cancel2()

	// Every etcd read now refuses, exactly as on a node that was never
	// bootstrapped -- which is genuinely indistinguishable on a real node too.
	statusCtx, cancel3, err := talos.WithClassDeadline(ctx, talos.MethodEtcdStatus)
	if err != nil {
		t.Fatalf("WithClassDeadline: %v", err)
	}
	defer cancel3()

	if _, err := cc.EtcdStatus(statusCtx); err == nil {
		t.Fatal("etcd still answers after the node left the cluster")
	}
}

// TestTheSchematicIsReadFromTheNodeAndNeverGuessed is UPG-03.
func TestTheSchematicIsReadFromTheNodeAndNeverGuessed(t *testing.T) {
	t.Parallel()

	_, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	verdict, err := upgrade.CheckSchematic(ctx, cc)
	if err != nil {
		t.Fatalf("CheckSchematic: %v", err)
	}
	if !verdict.Offered {
		t.Error("the schematic was not offered to be read from a node that is answering")
	}
	if verdict.Sentence == "" {
		t.Error("the verdict carries no sentence")
	}

	// Whichever way it came out, the sentence has to say what upgrading would
	// do to the extensions -- that is the silent failure UPG-03 is about.
	if !strings.Contains(verdict.Sentence, "extension") {
		t.Errorf("the sentence %q does not mention what happens to the extensions", verdict.Sentence)
	}
}

// TestKernelArgDriftBlocksTheOneClickPath is UPG-04.
//
// The upgrade RPC carries an installer image and nothing else -- kernel
// arguments are written at install time from the machine configuration -- so
// there are two sources of truth for them and only one travels with an
// upgrade. A node whose bootloader has an argument its configuration does not
// is a node where this path silently discards it.
func TestKernelArgDriftBlocksTheOneClickPath(t *testing.T) {
	t.Parallel()

	cl, err := talossim.NewCluster("homelab", "https://10.0.0.10:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	sim, err := talossim.New(talossim.Options{
		Hostname: "cp-1", Cluster: cl, ControlPlane: true,
		// On the bootloader and not in the configuration: the exact shape the
		// check exists for.
		KernelArgs: []string{"nvidia.NVreg_PreserveVideoMemoryAllocations=1"},
	})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cc, err := talos.NewClusterClient(ctx, sim.Dialer(), talos.Target{
		Cluster: "c1", Machine: "00000000-0000-4000-8000-000000000001", Addr: sim.Host(),
	}, sim.ClientCreds(), talos.Mode{})
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}
	defer cc.Close() //nolint:errcheck // the drift is the verdict

	drift, err := upgrade.ReadKernelArgDrift(ctx, cc)
	if err != nil {
		t.Fatalf("ReadKernelArgDrift: %v", err)
	}
	if !drift.Drifted {
		t.Fatalf("an argument on the bootloader and not in the configuration was not reported "+
			"as drift: %+v", drift)
	}
	if len(drift.OnlyOnNode) != 1 || !strings.Contains(drift.OnlyOnNode[0], "nvidia") {
		t.Errorf("the difference is %v, want the one argument that is only on the node",
			drift.OnlyOnNode)
	}

	// Both lists travel, because a diff an operator cannot see is a diff they
	// have to take on trust.
	if len(drift.OnNode) == 0 {
		t.Error("the drift carries no record of what the node is actually running")
	}
	if !strings.Contains(drift.Sentence, "install time") {
		t.Errorf("the sentence %q does not say why an upgrade would lose it", drift.Sentence)
	}
}

// TestTalosOwnArgumentsAreNotDrift is the other half, and without it the check
// above would block every node in every cluster.
//
// Talos sets a dozen arguments for itself and carries none of them in the
// machine configuration. A check that flagged them is a check somebody
// switches off.
func TestTalosOwnArgumentsAreNotDrift(t *testing.T) {
	t.Parallel()

	_, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	drift, err := upgrade.ReadKernelArgDrift(ctx, cc)
	if err != nil {
		t.Fatalf("ReadKernelArgDrift: %v", err)
	}
	if drift.Drifted {
		t.Fatalf("an ordinary node was reported as drifted: only on node %v, only in config %v",
			drift.OnlyOnNode, drift.OnlyInConfig)
	}
}

// The restore path, end to end against the simulator (Omni parity phase 1).
//
// The measurement that makes these worth writing is that the simulator's node
// state actually changes: talossim keeps the uploaded snapshot and what a
// recovery bootstrap started etcd from as two separate fields, so a test can
// tell "uploaded and never recovered from" apart from "restored", which is the
// half-done state this operation is built around.

// TestARestoreReplacesEtcdWithTheSnapshot is the whole path.
func TestARestoreReplacesEtcdWithTheSnapshot(t *testing.T) {
	t.Parallel()

	sim, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// A real snapshot, taken from this node, so the integrity check the node
	// makes is being exercised rather than skipped.
	var taken bytes.Buffer
	if _, err := upgrade.Snapshot(ctx, cc, &taken); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	n, err := upgrade.Restore(ctx, cc, upgrade.RestoreRequest{
		Snapshot: bytes.NewReader(taken.Bytes()),
		Role:     model.RoleControlPlane,
	})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if n != int64(taken.Len()) {
		t.Errorf("the restore reported %d bytes uploaded and the snapshot was %d", n, taken.Len())
	}

	state := sim.Node()
	if state.RecoverCalls != 1 {
		t.Errorf("the node saw %d EtcdRecover uploads, want 1", state.RecoverCalls)
	}
	if !bytes.Equal(state.UploadedSnapshot, taken.Bytes()) {
		t.Errorf("the node holds %d bytes and the snapshot was %d; the upload did not arrive intact",
			len(state.UploadedSnapshot), taken.Len())
	}
	if !bytes.Equal(state.RecoveredFrom, taken.Bytes()) {
		t.Error("the node did not start etcd from the uploaded snapshot. An upload that is never " +
			"recovered from leaves the cluster running its old data while somebody believes it " +
			"was restored")
	}
}

// TestAnUploadThatIsNotFollowedByARecoveryIsNotARestore pins the half-done
// state, because it is the one an operator most needs told apart from success.
func TestAnUploadThatIsNotFollowedByARecoveryIsNotARestore(t *testing.T) {
	t.Parallel()

	sim, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Bytes that are not an etcd API snapshot, so the node's integrity check
	// refuses the recovery after accepting the upload.
	_, err := upgrade.Restore(ctx, cc, upgrade.RestoreRequest{
		Snapshot: bytes.NewReader([]byte("not a snapshot at all")),
		Role:     model.RoleControlPlane,
	})
	if err == nil {
		t.Fatal("a restore from bytes the node refuses reported success")
	}
	if !strings.Contains(err.Error(), "still running the data it had") {
		t.Errorf("the failure %q does not say the cluster was left alone, which is the one thing "+
			"an operator needs to know at that moment", err)
	}

	state := sim.Node()
	if len(state.UploadedSnapshot) == 0 {
		t.Error("the upload did not reach the node, so this is testing the wrong failure")
	}
	if len(state.RecoveredFrom) != 0 {
		t.Error("etcd was started from bytes that failed their integrity check")
	}
}

// TestSkippingTheHashCheckIsHowADataDirectoryCopyIsRestored is the flag's one
// legitimate use, and it is the case an operator is most likely to be in: a
// cluster that had already lost quorum leaves nothing but a copied data
// directory, which carries no hash to check.
func TestSkippingTheHashCheckIsHowADataDirectoryCopyIsRestored(t *testing.T) {
	t.Parallel()

	sim, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	copied := []byte("a copy of somebody's etcd data directory")
	if _, err := upgrade.Restore(ctx, cc, upgrade.RestoreRequest{
		Snapshot:      bytes.NewReader(copied),
		Role:          model.RoleControlPlane,
		SkipHashCheck: true,
	}); err != nil {
		t.Fatalf("Restore with SkipHashCheck: %v", err)
	}

	if !bytes.Equal(sim.Node().RecoveredFrom, copied) {
		t.Error("the node did not recover from the copied data directory")
	}
}

// TestARestoreRefusesAWorkerAndAnEmptySnapshot checks the two refusals that
// happen before anything is sent.
func TestARestoreRefusesAWorkerAndAnEmptySnapshot(t *testing.T) {
	t.Parallel()

	sim, cc := liveNode(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	_, err := upgrade.Restore(ctx, cc, upgrade.RestoreRequest{
		Snapshot: bytes.NewReader([]byte("anything")),
		Role:     model.RoleWorker,
	})
	if !errors.Is(err, upgrade.ErrNotControlPlane) {
		t.Errorf("restoring onto a worker returned %v, want ErrNotControlPlane", err)
	}

	_, err = upgrade.Restore(ctx, cc, upgrade.RestoreRequest{
		Snapshot: bytes.NewReader(nil),
		Role:     model.RoleControlPlane,
	})
	if !errors.Is(err, upgrade.ErrEmptySnapshot) {
		t.Errorf("restoring an empty snapshot returned %v, want ErrEmptySnapshot", err)
	}

	// Neither reached the node. A refusal that had already uploaded something
	// would be a refusal that changed the cluster.
	if state := sim.Node(); state.RecoverCalls != 0 {
		t.Errorf("the node saw %d uploads from two refusals that should have stopped here",
			state.RecoverCalls)
	}
}

// nodeWith starts a simulated node with the given options and a client to it.
func nodeWith(t *testing.T, opts talossim.Options) (*talossim.Server, *talos.ClusterClient) {
	t.Helper()

	cl, err := talossim.NewCluster("homelab", "https://10.0.0.10:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	opts.Cluster = cl
	if opts.Hostname == "" {
		opts.Hostname = "cp-1"
	}
	sim, err := talossim.New(opts)
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)

	cc, err := talos.NewClusterClient(ctx, sim.Dialer(), talos.Target{
		Cluster: "c1", Machine: "00000000-0000-4000-8000-000000000001", Addr: sim.Host(),
	}, sim.ClientCreds(), talos.Mode{})
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}
	t.Cleanup(func() { _ = cc.Close() })

	return sim, cc
}

// TestANodeSaysHowItBooted is the read the upgrade path's installer choice
// hangs on.
//
// It is read from the node and never remembered, because a node may have been
// installed by something other than this installation, or reinstalled since. A
// stored flag would be this product's memory of a decision rather than the
// machine's account of what it is running -- and the decision it feeds is
// which installer to write, where being wrong takes SecureBoot away from a
// machine that has it.
func TestANodeSaysHowItBooted(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	_, ordinary := nodeWith(t, talossim.Options{Hostname: "plain-1", ControlPlane: true})
	state, err := ordinary.SecurityState(ctx)
	if err != nil {
		t.Fatalf("SecurityState on an ordinary node: %v", err)
	}
	if state.SecureBoot {
		t.Error("an ordinary node reports SecureBoot")
	}
	if state.BootedWithUKI {
		t.Error("an ordinary node reports booting a unified kernel image")
	}

	_, secure := nodeWith(t, talossim.Options{
		Hostname: "secure-1", ControlPlane: true, SecureBoot: true,
	})
	state, err = secure.SecurityState(ctx)
	if err != nil {
		t.Fatalf("SecurityState on a SecureBoot node: %v", err)
	}
	if !state.SecureBoot {
		t.Fatal("a SecureBoot node reports no SecureBoot")
	}

	// SecureBoot Talos boots a signed unified kernel image, so the two go
	// together. A node reporting one and not the other is not a state a real
	// machine produces, and the simulator must not be able to claim it.
	if !state.BootedWithUKI {
		t.Error("a SecureBoot node reports not booting a unified kernel image")
	}
	if state.UKISigningKeyFingerprint == "" {
		t.Error("a SecureBoot node names no signing key")
	}
}
