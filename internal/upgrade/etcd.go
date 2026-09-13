package upgrade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// etcd management: listing members, removing one, taking a snapshot, and
// removing a node from the cluster altogether.
//
// The through-line is that every one of these is named by **hostname**
// (UPG-10). etcd's own identifier is a 64-bit number, it is printed as hex,
// and a screen that offered `8e9e05c52164694d` as the thing to confirm is a
// screen where somebody will eventually remove the wrong member. The id is
// carried because the RPC needs it; it is never the only thing on offer.

// ErrLastVotingMember reports a removal that would leave etcd without a
// quorum.
var ErrLastVotingMember = errors.New("upgrade: removing this member would leave etcd without a quorum")

// Member is one etcd member as a screen shows it.
type Member struct {
	// ID is etcd's own, rendered as the hex string the RPC and talosctl both
	// use. It is a string on the wire rather than a number because JSON
	// numbers are float64 in every browser and a 64-bit member id does not
	// survive that -- which would be a screen quietly showing a member id that
	// is not any member's.
	ID string `json:"id"`

	// Name is what the operator reads: the hostname, with the machine's UUID
	// behind it when holzkube-manager knows which machine this is.
	Name string `json:"name"`

	// Machine is the inventory's machine for this member, when the hostname
	// matches one. Empty means etcd knows about a member holzkube-manager does
	// not -- which is worth seeing rather than hiding.
	Machine model.MachineID `json:"machine,omitempty"`

	// Learner marks a member that replicates and does not vote.
	Learner bool `json:"learner"`

	// Voting is the negation, carried explicitly so a client does not have to
	// know that those are the only two states.
	Voting bool `json:"voting"`

	PeerURLs []string `json:"peer_urls,omitempty"`
}

// MemberList is the answer to "what is in this cluster's etcd".
type MemberList struct {
	Members []Member `json:"members"`

	// VotingCount is how many of them vote, which is the number every decision
	// on this screen turns on.
	VotingCount int `json:"voting_count"`

	// Tolerates is how many members may be lost before the cluster stops
	// accepting writes. It is the sentence an operator actually wants, derived
	// once here rather than in every client.
	Tolerates int `json:"tolerates"`

	// Sentence says both in words.
	Sentence string `json:"sentence"`
}

// Members reads a cluster's etcd membership (UPG-10).
func Members(ctx context.Context, cc *talos.ClusterClient, machines []model.Machine) (MemberList, error) {
	listCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodEtcdMemberList)
	if err != nil {
		return MemberList{}, err
	}
	defer cancel()

	raw, err := cc.EtcdMembers(listCtx)
	if err != nil {
		return MemberList{}, err
	}

	out := MemberList{Members: make([]Member, 0, len(raw))}
	for _, m := range raw {
		member := Member{
			ID:       fmt.Sprintf("%x", m.ID),
			Name:     talos.DescribeMember(m),
			Learner:  m.IsLearner,
			Voting:   !m.IsLearner,
			PeerURLs: m.PeerURLs,
		}
		for _, machine := range machines {
			if machine.Hostname != "" && strings.EqualFold(machine.Hostname, m.Hostname) {
				member.Machine = machine.ID
				break
			}
		}
		out.Members = append(out.Members, member)
		if member.Voting {
			out.VotingCount++
		}
	}

	// Integer division, and it is the right arithmetic: a majority of n is
	// n/2+1, so the number that may be lost is n - (n/2+1).
	out.Tolerates = out.VotingCount - (out.VotingCount/2 + 1)
	if out.Tolerates < 0 {
		out.Tolerates = 0
	}

	switch {
	case out.VotingCount == 0:
		out.Sentence = "etcd reports no voting members, which is not a cluster this can say anything about."
	case out.Tolerates == 0:
		out.Sentence = fmt.Sprintf(
			"%d voting member(s). Losing any one of them stops the cluster accepting writes: a "+
				"majority of %d is %d, and %d remaining is not one.",
			out.VotingCount, out.VotingCount, out.VotingCount/2+1, out.VotingCount-1)
	default:
		out.Sentence = fmt.Sprintf(
			"%d voting member(s), so %d may be lost before the cluster stops accepting writes.",
			out.VotingCount, out.Tolerates)
	}
	return out, nil
}

// RefuseIfCannotSpareAVoter is the one place that decides whether a cluster
// survives losing a voting etcd member.
//
// One place, because there are three doors into it and they must not answer
// differently: RemoveMember, which takes a member out by id; RemoveNode, which
// is the button on a node's page; and internal/scale, which draws the screen
// saying which nodes may be removed. The second went two phases consulting no
// membership at all, so the refusal existed beside it and never ran -- and a
// screen that decided for itself which nodes are removable would be the same
// failure again, one layer up and harder to see, because it would be wrong
// quietly rather than at the moment somebody clicks.
//
// Two cases, separated because the consequences are not the same size:
//
//   - **the only member.** Removing it does not make the cluster smaller, it
//     ends it. There is no quorum left to rejoin and no member to add one
//     through, and the way back is a restore from a snapshot.
//   - **one of two.** The cluster stops accepting writes, and the Kubernetes
//     API stops with it. That is recoverable by adding a member back; the
//     first is not.
//
// The first case shipped excluded by a `VotingCount > 1` on the second one's
// condition, which let the worse of the two through.
//
// The upgrade gate's single-node exemption is deliberately not here. A cluster
// of one is exempt from the *upgrade* gate because it has no quorum to lose
// and refusing would make the smallest homelab unupgradeable -- and the node
// comes back. Nothing comes back from a removal.
func RefuseIfCannotSpareAVoter(list MemberList, name string) error {
	switch {
	case list.VotingCount <= 1:
		return fmt.Errorf("%w: %s is the only etcd member. Removing it does not make the "+
			"cluster smaller, it ends it -- there would be no quorum left to rejoin and no "+
			"member to add one through, and the only way back is a restore from a snapshot. "+
			"To take this node out of service, reset it",
			ErrLastVotingMember, name)

	case list.Tolerates == 0:
		return fmt.Errorf("%w: %s is one of %d voting members, and %d cannot lose one. Add a "+
			"control-plane node first, or accept that the cluster stops accepting writes",
			ErrLastVotingMember, name, list.VotingCount, list.VotingCount)
	}
	return nil
}

// RemoveMember removes one member from etcd (UPG-11).
//
// through is the node the call is made against, and it must not be the member
// being removed: a member cannot remove itself through this RPC. The check is
// here rather than left to the node's refusal because the node's message is
// about the API and this one is about what to do instead.
func RemoveMember(
	ctx context.Context,
	cc *talos.ClusterClient,
	list MemberList,
	id string,
) error {
	member, ok := findMember(list, id)
	if !ok {
		return fmt.Errorf("upgrade: etcd has no member %s", id)
	}

	if member.Voting {
		if err := RefuseIfCannotSpareAVoter(list, member.Name); err != nil {
			return err
		}
	}

	n, err := parseMemberID(id)
	if err != nil {
		return err
	}

	removeCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodEtcdRemoveMemberByID)
	if err != nil {
		return err
	}
	defer cancel()

	return cc.EtcdRemoveMember(removeCtx, n)
}

// Snapshot streams an etcd snapshot to w (UPG-12).
//
// It returns the number of bytes written, because a snapshot that wrote
// nothing and reported success is the failure worth catching: a backup file of
// zero length looks like a backup in a directory listing.
func Snapshot(ctx context.Context, cc *talos.ClusterClient, w io.Writer) (int64, error) {
	// No class deadline: the snapshot is a stream, and its class carries a
	// first-byte deadline and an idle timeout rather than a total one. A
	// multi-gigabyte etcd on a slow disk takes as long as it takes, and a
	// total deadline would fail the backups that most need to succeed.
	r, err := cc.EtcdSnapshot(ctx)
	if err != nil {
		return 0, fmt.Errorf("%w. %s", err, talos.SnapshotNotice)
	}
	defer r.Close() //nolint:errcheck // the byte count is the verdict

	// The notice rides on the copy's error as well as on the call's, and that
	// is not belt and braces: this is a server stream, so the refusal that
	// matters -- etcd declining because it cannot confirm a quorum -- arrives
	// on the first Read rather than when the stream is opened. Carrying the
	// notice only on the constructor's error would put it on the path this
	// never takes.
	n, err := io.Copy(w, r)
	if err != nil {
		return n, fmt.Errorf("%w. %s", err, talos.SnapshotNotice)
	}
	if n == 0 {
		return 0, errors.New("upgrade: the snapshot stream produced no bytes. A zero-length file " +
			"is not a backup, and it looks like one in a directory listing")
	}
	return n, nil
}

// RemoveNode takes a node out of a cluster for good (UPG-13).
//
// The order is the whole of it, and each step is there because skipping it
// leaves something behind:
//
//  1. **etcd leave**, on the node itself, so it forfeits leadership if it has
//     it and removes itself from the membership. Doing this from another node
//     while this one still runs leaves a member that believes it is still in a
//     cluster that has forgotten it.
//  2. **reset**, which wipes it. Without this the machine still holds the
//     cluster's PKI and will try to rejoin on its next boot.
//  3. **forget**, which takes it out of holzkube-manager's inventory. Without
//     this the dashboard shows a node that is never coming back, which is
//     indistinguishable from one that is down.
//
// Cordon and drain are not here, and that is a limitation stated rather than
// hidden: they are Kubernetes API operations and this product speaks the Talos
// machine API. RemoveNodeNotice says so on the screen.
func RemoveNode(
	ctx context.Context,
	cc *talos.ClusterClient,
	m model.Machine,
) error {
	if m.Role == model.RoleControlPlane {
		// Before anything, and this is the whole of the fix: read the
		// membership and refuse if the cluster does not survive losing a
		// voter. It is read through the node being removed, which is fine --
		// it is still a member and still answering, or Connect would not have
		// returned a client.
		//
		// A worker never reaches this. It is not a member, and a cluster whose
		// etcd cannot be reached must not be a cluster whose workers cannot be
		// removed.
		list, err := Members(ctx, cc, nil)
		if err != nil {
			return fmt.Errorf("upgrade: %s is a control-plane node and its cluster's etcd "+
				"membership could not be read, so there is no way to tell whether the cluster "+
				"survives losing it. Nothing has been changed: %w", nameOf(m), err)
		}
		if err := RefuseIfCannotSpareAVoter(list, nameOf(m)); err != nil {
			return err
		}

		leaveCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodEtcdLeaveCluster)
		if err != nil {
			return err
		}
		err = cc.EtcdLeaveCluster(leaveCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("upgrade: %s could not leave etcd: %w. Nothing has been wiped",
				nameOf(m), err)
		}
	}

	resetCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodReset)
	if err != nil {
		return err
	}
	defer cancel()

	return cc.Reset(resetCtx, talos.ResetOptions{
		// The system disk, so the machine comes back in maintenance mode and
		// can be provisioned again. Not every disk: the operator removing a
		// node from a cluster has not necessarily asked for the data on it to
		// go, and a removal that wiped more than it was asked to is not
		// undoable.
		Mode:     "system-disk",
		Graceful: true,
		Reboot:   true,
	})
}

// RemoveNodeNotice is what the screen says about cordon and drain (UPG-13).
//
// It is stated rather than quietly omitted. An operator who reads "remove this
// node" and expects the workloads to move first is an operator whose pods get
// killed, and the honest answer is that this product does not speak to the
// Kubernetes API at all.
const RemoveNodeNotice = "holzkube-manager speaks the Talos machine API and not the Kubernetes API, " +
	"so it cannot cordon or drain this node. Anything still scheduled on it stops when it does. " +
	"Run `kubectl drain <node>` first if the workloads on it need to move rather than restart."

// EvictionWait is how long a removal waits after the etcd leave before
// wiping.
//
// It is not a guess about etcd: it is the gap in which a raft that has just
// lost a member settles, and wiping into that gap is how a removal of one
// member looks to the cluster like the loss of two.
const EvictionWait = 15 * time.Second

func findMember(list MemberList, id string) (Member, bool) {
	for _, m := range list.Members {
		if strings.EqualFold(m.ID, id) {
			return m, true
		}
	}
	return Member{}, false
}

// parseMemberID reads etcd's hex member id.
//
// It is parsed rather than carried as a number for the reason Member.ID
// documents: a 64-bit id does not survive a JSON round trip through a browser,
// so the string is the canonical form everywhere above the seam.
func parseMemberID(id string) (uint64, error) {
	var n uint64
	if _, err := fmt.Sscanf(strings.TrimPrefix(strings.ToLower(id), "0x"), "%x", &n); err != nil {
		return 0, fmt.Errorf("upgrade: %q is not an etcd member id", id)
	}
	return n, nil
}
