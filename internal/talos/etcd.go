package talos

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
)

// The etcd RPCs beyond EtcdMemberList.
//
// Everything here is about a three-node etcd, which is the smallest cluster
// that tolerates anything at all. One member of three going away is a cluster
// that still works; two is a cluster that is gone, and there is no command
// that brings it back without a snapshot. That asymmetry is why the health
// gate in internal/upgrade refuses rather than warns, and why the numbers it
// refuses on are read here rather than assumed.

// The method names this file calls, for the deadline class table.
const (
	MethodEtcdAlarmList         = machineService + "EtcdAlarmList"
	MethodEtcdRemoveMemberByID  = machineService + "EtcdRemoveMemberByID"
	MethodEtcdForfeitLeadership = machineService + "EtcdForfeitLeadership"
	MethodEtcdLeaveCluster      = machineService + "EtcdLeaveCluster"
)

// EtcdStatus is one member's own account of the raft it is part of.
//
// The three fields the health gate turns on are Leader, RaftIndex and
// IsLearner, and each answers a different question that looks the same from
// outside: whether there is a leader at all, whether this member has caught up
// with it, and whether this member is allowed to vote.
type EtcdStatus struct {
	MemberID uint64

	// Leader is the member id etcd currently considers the leader. Zero means
	// there is none, which is a cluster mid-election and not a cluster that
	// can be upgraded.
	Leader uint64

	RaftIndex        uint64
	RaftAppliedIndex uint64
	RaftTerm         uint64

	// IsLearner marks a member that replicates and does not vote. Counting one
	// as a voter is how a "three-member" cluster turns out to have two votes
	// at exactly the moment one of them is being rebooted.
	IsLearner bool

	DBSize      int64
	DBSizeInUse int64

	// Errors is etcd's own list. It is carried rather than summarised: the gate
	// shows its inputs (UPG-02), and a summarised error is an input nobody can
	// check.
	Errors []string
}

// EtcdStatus reads this node's own etcd member status.
func (c *ClusterClient) EtcdStatus(ctx context.Context) (EtcdStatus, error) {
	resp, err := c.conn.c.EtcdStatus(ctx)
	if err != nil {
		return EtcdStatus{}, err
	}

	for _, msg := range resp.GetMessages() {
		s := msg.GetMemberStatus()
		if s == nil {
			continue
		}
		return EtcdStatus{
			MemberID:         s.GetMemberId(),
			Leader:           s.GetLeader(),
			RaftIndex:        s.GetRaftIndex(),
			RaftAppliedIndex: s.GetRaftAppliedIndex(),
			RaftTerm:         s.GetRaftTerm(),
			IsLearner:        s.GetIsLearner(),
			DBSize:           s.GetDbSize(),
			DBSizeInUse:      s.GetDbSizeInUse(),
			Errors:           s.GetErrors(),
		}, nil
	}
	return EtcdStatus{}, errors.New("talos: the node answered EtcdStatus with no member status")
}

// EtcdAlarm is one alarm etcd has raised against a member.
type EtcdAlarm struct {
	MemberID uint64

	// Type is etcd's own name for it: "NOSPACE" or "CORRUPT". Both are
	// conditions under which an upgrade must not start -- NOSPACE because the
	// member cannot write, CORRUPT because what it would replicate is wrong.
	Type string
}

// EtcdAlarmList reports every alarm the cluster has raised.
func (c *ClusterClient) EtcdAlarmList(ctx context.Context) ([]EtcdAlarm, error) {
	resp, err := c.conn.c.EtcdAlarmList(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]EtcdAlarm, 0, 2)
	for _, msg := range resp.GetMessages() {
		for _, a := range msg.GetMemberAlarms() {
			out = append(out, EtcdAlarm{MemberID: a.GetMemberId(), Type: a.GetAlarm().String()})
		}
	}
	return out, nil
}

// EtcdMemberDetail is a member with everything a screen needs to name it.
//
// It exists next to EtcdMember rather than replacing it because UPG-10 is a
// statement about the screen: a member is listed by **hostname** and never by
// a raw hex id. An operator asked to remove `8e9e05c52164694d` is an operator
// who will eventually remove the wrong one.
type EtcdMemberDetail struct {
	ID       uint64
	Hostname string

	// IsLearner marks a member that does not vote.
	IsLearner bool

	PeerURLs   []string
	ClientURLs []string
}

// EtcdMembers reports the etcd membership with the detail a screen needs.
func (c *ClusterClient) EtcdMembers(ctx context.Context) ([]EtcdMemberDetail, error) {
	resp, err := c.conn.c.EtcdMemberList(ctx, &machine.EtcdMemberListRequest{})
	if err != nil {
		return nil, err
	}

	out := make([]EtcdMemberDetail, 0, 3)
	for _, msg := range resp.GetMessages() {
		for _, m := range msg.GetMembers() {
			out = append(out, EtcdMemberDetail{
				ID:         m.GetId(),
				Hostname:   m.GetHostname(),
				IsLearner:  m.GetIsLearner(),
				PeerURLs:   m.GetPeerUrls(),
				ClientURLs: m.GetClientUrls(),
			})
		}
	}
	return out, nil
}

// EtcdRemoveMember removes a member from the etcd cluster by its id.
//
// It is aimed at a *different* node than the one being removed: a member
// cannot remove itself, and the call is served by whichever control-plane node
// this client is connected to. That is why the id is a parameter rather than
// implied by the connection.
func (c *ClusterClient) EtcdRemoveMember(ctx context.Context, id uint64) error {
	return c.conn.c.EtcdRemoveMemberByID(ctx, &machine.EtcdRemoveMemberByIDRequest{MemberId: id})
}

// EtcdForfeitLeadership hands this node's etcd leadership to somebody else.
//
// It is called before taking a leader down, and skipping it is not a failure
// so much as an avoidable outage: etcd elects a new leader on its own after a
// timeout, and everything that writes to the cluster stalls until it does.
//
// It returns the member that took over, which is worth reporting -- an
// operator watching a rolling upgrade wants to know where the leadership went.
func (c *ClusterClient) EtcdForfeitLeadership(ctx context.Context) (string, error) {
	resp, err := c.conn.c.EtcdForfeitLeadership(ctx, &machine.EtcdForfeitLeadershipRequest{})
	if err != nil {
		return "", err
	}
	for _, msg := range resp.GetMessages() {
		if m := msg.GetMember(); m != "" {
			return m, nil
		}
	}
	return "", nil
}

// EtcdLeaveCluster makes this node leave the etcd cluster.
//
// It is the node's own half of a removal, and it is the polite one: the node
// forfeits leadership if it has it, removes itself from the membership and
// stops etcd. The alternative -- removing it from another node while it is
// still running -- leaves a member that believes it is still in a cluster that
// has forgotten it.
func (c *ClusterClient) EtcdLeaveCluster(ctx context.Context) error {
	return c.conn.c.EtcdLeaveCluster(ctx, &machine.EtcdLeaveClusterRequest{})
}

// EtcdSnapshot streams a point-in-time snapshot of the etcd database.
//
// The caller owns the returned reader and must Close it.
//
// UPG-12 asks for a documented fallback when there is no quorum, and the
// fallback is not a flag on this call: a cluster without quorum does not serve
// this RPC at all, because the member cannot confirm it is reading a current
// database. What is left then is the member's own on-disk snapshot, taken from
// the node's filesystem -- and that is a different operation with a different
// honesty, which is why it is not hidden behind this one. See SnapshotNotice.
func (c *ClusterClient) EtcdSnapshot(ctx context.Context) (io.ReadCloser, error) {
	return c.conn.c.EtcdSnapshot(ctx, &machine.EtcdSnapshotRequest{})
}

// SnapshotNotice is what a screen says about a snapshot taken without quorum
// (UPG-12).
//
// It is a constant rather than UI copy because it is a statement about what
// the data is, and softening it would be softening the difference between a
// backup and a file that looks like one.
const SnapshotNotice = "A snapshot through the etcd API is a consistent point-in-time copy, and " +
	"it needs a quorum: the member has to confirm with the others that what it is reading is " +
	"current. A cluster that has lost quorum cannot answer that, so this call fails there. What " +
	"is still available on such a cluster is the member's own database file, copied off the " +
	"node -- that is a snapshot of what one member believed, which is what you restore from when " +
	"there is nothing better, and it is not the same thing."

// VotingMembers counts the members that can vote.
//
// Learners replicate and do not vote, so a cluster of three members with one
// learner has two votes -- and two votes tolerate zero failures. Counting
// learners is how a rolling upgrade takes down the second of two voters.
func VotingMembers(members []EtcdMemberDetail) int {
	n := 0
	for _, m := range members {
		if !m.IsLearner {
			n++
		}
	}
	return n
}

// DescribeMember names a member the way a screen must (UPG-10).
//
// Hostname first, and the hex id only in brackets behind it. The id is
// necessary -- it is what EtcdRemoveMember takes -- and it is never the only
// thing on offer: an operator asked to confirm the removal of
// "8e9e05c52164694d" is confirming a string, not a machine.
func DescribeMember(m EtcdMemberDetail) string {
	name := m.Hostname
	if name == "" {
		name = "a member that did not report a hostname"
	}
	if m.IsLearner {
		return fmt.Sprintf("%s (learner, id %x)", name, m.ID)
	}
	return fmt.Sprintf("%s (id %x)", name, m.ID)
}

// EtcdRecover uploads a snapshot to a node so that a subsequent
// BootstrapFromSnapshot can start etcd from it (UPG-12's other half).
//
// It is a client stream -- the only one this product makes -- and that is why
// it takes a reader rather than bytes: an etcd database is as large as it is,
// and reading a multi-gigabyte snapshot into memory to hand it over would put
// the size of somebody's cluster into this process's resident set.
//
// Uploading is not restoring. This call leaves the snapshot on the node and
// changes nothing: the node still runs whatever etcd it was running. What acts
// on it is BootstrapFromSnapshot, and keeping them apart is what lets a restore
// fail at the upload without having touched the cluster.
func (c *ClusterClient) EtcdRecover(ctx context.Context, snapshot io.Reader) error {
	_, err := c.conn.c.EtcdRecover(ctx, snapshot)
	return err
}

// RestoreNotice is what a screen says before a restore (UPG-12).
//
// It is a constant for the same reason SnapshotNotice is: it states what the
// operation does to a cluster, and every softer wording of it is a wording that
// gets somebody to click.
const RestoreNotice = "Restoring replaces the cluster's etcd with the contents of the snapshot. " +
	"Everything written since the snapshot was taken is gone -- every object created, every " +
	"change applied, every secret rotated. The cluster is unavailable while it happens, and the " +
	"other control-plane nodes have to be reset and rejoined afterwards, because their etcd is " +
	"the one being replaced and they will not agree with the recovered member. This is what you " +
	"do when the alternative is rebuilding the cluster."
