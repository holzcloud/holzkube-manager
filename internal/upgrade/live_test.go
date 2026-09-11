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
