package inventory

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// The three questions the provisioning wizard asks the inventory.
//
// They are here rather than answered by the provisioning package reading the
// store because the store is entity-shaped and this package is what knows what
// the entities mean: "a machine is at this address" is a statement about the
// address hint on a record, and "this cluster has three control-plane nodes"
// is a statement about a role that is derived from an etcd membership list and
// not from anything the operator typed.

// KnownAt reports whether the inventory already has a machine at an address.
//
// It is what makes a scan result say "this is one of yours" rather than only
// "something is here". The two lead to different actions: a configured node
// this installation has never heard of belongs to somebody else and aiming a
// provisioning run at it is a mistake to stop; one it already manages is one
// the operator can recognise.
//
// LostAddrAt is honoured: a record whose address was taken over by a different
// machine is no longer at that address, and saying it is would be repeating
// the stale hint that made it lose the address in the first place.
func (s *Service) KnownAt(ctx context.Context, addr string) bool {
	if addr == "" {
		return false
	}
	recs, err := s.deps.Store.Machines().List(ctx)
	if err != nil {
		// A failed read is not a "no". It is reported as unknown, which is the
		// honest answer, and the scan's own finding is unchanged: something is
		// at that address either way.
		return false
	}
	for _, rec := range recs {
		if rec.Addr == addr && rec.LostAddrAt.IsZero() {
			return true
		}
	}
	return false
}

// ControlPlaneCount is how many control-plane nodes a cluster has.
//
// It is what the even-count warning is computed against (PROV-07), and it
// counts what is in the inventory rather than asking etcd: the question is
// "how many will this cluster have after the machine you are about to add",
// and a node that is down is still one of them.
//
// A machine whose role is unknown is not counted. Counting it would turn a
// warning about quorum into a warning based on a guess, and the operator
// cannot tell the two apart from the sentence.
func (s *Service) ControlPlaneCount(ctx context.Context, id model.ClusterID) (int, error) {
	recs, err := s.presentMachines(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, rec := range recs {
		if rec.Cluster == id && rec.Role == model.RoleControlPlane {
			n++
		}
	}
	return n, nil
}

// ClusterCreds loads the credentials holzkube-manager dials a cluster's nodes with.
//
// It is the exported half of what Connect uses internally, and it exists for
// the one caller that needs the credentials without a machine record to hang
// them on: a machine being provisioned is not in the inventory yet, and the
// node it is about to become has to be reached before it can be.
func (s *Service) ClusterCreds(ctx context.Context, id model.ClusterID) (talos.Creds, error) {
	return s.clusterCreds(ctx, id)
}

// KubernetesVersion is the version a new node should join a cluster at.
//
// It is read off the cluster's existing nodes rather than taken from a
// constant in this build: a node generated at one Kubernetes version and
// joined to a cluster running another is a node that joins and then does not
// work, and the symptom -- pods that never schedule -- looks like a
// networking problem for as long as it takes somebody to compare two version
// strings.
//
// The *lowest* version any node reports wins. A cluster part-way through an
// upgrade has two, and adding a node at the higher one puts a kubelet in front
// of a control plane that has not got there yet; the lower one is the version
// every node in the cluster can already talk to.
//
// A cluster whose nodes have never reported one -- the first node of a new
// cluster, or one nothing has observed yet -- has no answer here, and the
// caller is told that rather than handed a guess.
func (s *Service) KubernetesVersion(ctx context.Context, id model.ClusterID) (string, error) {
	recs, err := s.presentMachines(ctx)
	if err != nil {
		return "", err
	}

	lowest := ""
	for _, rec := range recs {
		v := rec.Snapshot.KubernetesVersion
		if rec.Cluster != id || v == "" {
			continue
		}
		if lowest == "" || compareVersions(v, lowest) < 0 {
			lowest = v
		}
	}
	if lowest == "" {
		return "", fmt.Errorf("inventory: no node in cluster %s has reported a Kubernetes version, "+
			"so there is nothing to match a new node against. Name the version explicitly", id)
	}
	return lowest, nil
}

// compareVersions orders two dotted version strings numerically.
//
// It is here rather than in a library because the only versions it ever sees
// are kubelet versions read off a node, and those are "1.34.1" or "v1.34.1"
// and nothing else. A non-numeric component sorts as zero, which keeps a
// pre-release string from being read as a higher version than the release it
// precedes.
func compareVersions(a, b string) int {
	as := strings.Split(strings.TrimPrefix(a, "v"), ".")
	bs := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		an, bn := 0, 0
		if i < len(as) {
			an, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bn, _ = strconv.Atoi(bs[i])
		}
		if an != bn {
			if an < bn {
				return -1
			}
			return 1
		}
	}
	return 0
}

// MachinesOf lists a cluster's machines.
//
// It returns the stored records rather than the views, because the callers are
// the upgrade domain and the health gate: both need the role, the lock and the
// last-known versions, and none of them needs the provenance a view carries.
func (s *Service) MachinesOf(ctx context.Context, id model.ClusterID) ([]model.Machine, error) {
	recs, err := s.presentMachines(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Machine, 0, len(recs))
	for _, rec := range recs {
		if rec.Cluster == id {
			out = append(out, rec)
		}
	}
	return out, nil
}

// ControlPlanesOf lists a cluster's control-plane machines.
//
// A machine whose role is unknown is not included, and that is the
// conservative reading in the direction that matters: the health gate asks
// every member it is given, and a member it was not given shows up as one it
// could not ask, which refuses. Including a node that turns out not to run
// etcd would make the gate refuse for a reason that is not true; leaving it
// out makes the gate refuse for a reason that is.
func (s *Service) ControlPlanesOf(ctx context.Context, id model.ClusterID) ([]model.Machine, error) {
	machines, err := s.MachinesOf(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]model.Machine, 0, len(machines))
	for _, m := range machines {
		if m.Role == model.RoleControlPlane {
			out = append(out, m)
		}
	}
	return out, nil
}
