package inventory

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/compat"
	"github.com/holzcloud/holzkube-manager/internal/health"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// The read model is the API's answer shape, and it is the same shape whether
// it arrives by poll today or by SSE in phase 5 (D-18): every fact is a
// health.Field carrying its level, whether it is confirmed right now, and
// since when it has not been.

// MachineView is one node as the API serves it.
//
// The identity fields at the top are not Fields: holzkube-manager holds them itself
// and they are true whether or not anything is reachable. Everything below is
// something a node said, and every one of those carries its provenance.
type MachineView struct {
	ID      model.MachineID   `json:"id"`
	Cluster model.ClusterID   `json:"cluster,omitempty"`
	Role    model.MachineRole `json:"role"`

	// Stage is the observer's state machine (Pattern 7). It is what the screen
	// colours on, and it is the only thing that gets an alarm colour --
	// staleness is shown but muted (D-15).
	Stage health.Stage `json:"stage"`

	// LostAddr reports that a different machine answered at this one's last
	// known address (D-10). It is the explanation for a node that stopped
	// being found without anything having broken.
	LostAddr bool `json:"lost_addr"`

	// CertificateExpired distinguishes a cluster whose credentials ran out
	// from a node that died. Without it, an expired certificate looks like a
	// dead fleet and sends the operator to the wrong repair (D-23).
	CertificateExpired bool `json:"certificate_expired"`

	// UnsupportedVersion marks a node running a Talos version outside the range
	// this build was tested against (OPS-03).
	//
	// It is derived from the last version the node reported rather than from a
	// failed connection, and that is the point: a node outside the range is
	// *refused* at connect time, so if this depended on connecting it would be
	// blank for exactly the nodes it exists to mark. What it reads is the
	// snapshot, which survives the node being unreachable for any reason.
	UnsupportedVersion bool `json:"unsupported_version"`

	// PreRelease marks a node running an alpha, beta or release candidate. It
	// is separate from UnsupportedVersion because the two mean opposite
	// things about what to do: one is a node to change, the other is a node
	// this instance can accept once somebody says so.
	PreRelease bool `json:"pre_release"`

	// VersionNotice is the sentence for either, or empty. It is here rather
	// than assembled in the browser because it is a statement about what this
	// build supports, and a copy in the bundle is a copy that drifts from the
	// constants it describes.
	VersionNotice string `json:"version_notice,omitempty"`

	// Locked and LockReason are UPG-14: this node is skipped by rolling
	// operations. They are not Fields, because they are holzkube-manager's own
	// note about what it should not do rather than something a node said --
	// and they are true whether or not the node is answering, which is
	// precisely when the note matters most.
	Locked bool `json:"locked"`

	LockReason string `json:"lock_reason,omitempty"`

	// Labels are the operator's own words about this machine. They are a plain
	// map and never a Field: nothing observed them, so there is no level they
	// could be unavailable at, and no observation ever overwrites one.
	Labels map[string]string `json:"labels,omitempty"`

	AdoptedAt time.Time `json:"adopted_at"`

	// SeenAt is when this machine last answered anything at all, which is not
	// the same as when it was last read. A node whose connection succeeds and
	// whose facts read fails moves this and leaves the snapshot alone, so a
	// SeenAt later than the snapshot's reading is the shape of a node that is
	// present and cannot be read -- a different repair from one that is
	// absent, and the reason the field is on the view rather than only in the
	// record.
	SeenAt time.Time `json:"seen_at,omitzero"`

	// Watch is whether this node's resource subscription is delivering
	// (INV-13, D-19). It is not a Field and not part of Stage: it says how
	// quickly a change will be noticed, not whether anything below is true.
	// A node whose watch is down is still confirmed by the heartbeat -- it is
	// just up to a heartbeat behind, which is what this product did before the
	// watch existed.
	Watch WatchStatus `json:"watch"`

	Hostname health.Field[string] `json:"hostname"`
	Addr     health.Field[string] `json:"addr"`

	TalosVersion      health.Field[string] `json:"talos_version"`
	KubernetesVersion health.Field[string] `json:"kubernetes_version"`
	SchematicID       health.Field[string] `json:"schematic_id"`

	Manufacturer health.Field[string] `json:"manufacturer"`
	ProductName  health.Field[string] `json:"product_name"`
	SerialNumber health.Field[string] `json:"serial_number"`

	MemoryMiB  health.Field[uint64]            `json:"memory_mib"`
	CPUs       health.Field[[]model.CPU]       `json:"cpus"`
	Disks      health.Field[[]model.Disk]      `json:"disks"`
	Interfaces health.Field[[]model.Interface] `json:"interfaces"`
	Services   health.Field[[]model.Service]   `json:"services"`

	EtcdMember health.Field[bool] `json:"etcd_member"`

	// Compatibility is what the shipped matrix says about this node's pair of
	// versions (D-24). It is LevelNode because both inputs are read from the
	// node, and it is a Field for the same reason they are: a node that has
	// not answered has a compatibility verdict that is as old as its versions.
	Compatibility health.Field[Compatibility] `json:"compatibility"`
}

// Compatibility is the matrix verdict as the API serves it.
type Compatibility struct {
	Known          bool   `json:"known"`
	Supported      bool   `json:"supported"`
	HeadroomMinors int    `json:"headroom_minors"`
	Sentence       string `json:"sentence"`
}

// ClusterView is one cluster as the API serves it.
//
// Nothing here is a secret, and that is a property of the type rather than of
// the handler that uses it: the secrets live in their own entity and this view
// is built without loading it.
type ClusterView struct {
	ID       model.ClusterID     `json:"id"`
	Name     string              `json:"name"`
	Origin   model.ClusterOrigin `json:"origin"`
	Endpoint string              `json:"endpoint"`
	Locked   bool                `json:"locked"`

	CreatedAt time.Time `json:"created_at"`

	// The certificate ladder (D-23). DaysLeft may be negative, which is the
	// expired state and is not the same thing as a cluster that is down.
	ClientCertNotAfter time.Time `json:"client_cert_not_after"`
	ClientCertDaysLeft int       `json:"client_cert_days_left"`
	CertificateWarning string    `json:"certificate_warning,omitempty"`
	CertificateUrgency string    `json:"certificate_urgency"`

	Nodes        int `json:"nodes"`
	ControlPlane int `json:"control_plane"`
	Workers      int `json:"workers"`

	// Healthy, Degraded, Down and Checking count the nodes in each condition,
	// so the fleet overview does not have to fetch every node to draw a tile.
	//
	// Checking is a node nobody has had an answer from YET: an observer that
	// has not run, or is connecting. It used to be counted as Down, so every
	// import and every daemon restart put "1 not answering" on a card about a
	// node that had never been asked -- the operator saw exactly that moments
	// after re-adopting their cluster. Down is reserved for downgradeAfter
	// consecutive failures, which is a claim; Checking is the absence of one.
	Healthy  int `json:"healthy"`
	Degraded int `json:"degraded"`
	Down     int `json:"down"`
	Checking int `json:"checking"`
}

// Certificate urgency levels, in the order D-23 escalates them.
const (
	// UrgencyNone is more than 90 days left.
	UrgencyNone = "none"

	// UrgencyBadge is 90 days or fewer: a badge on the cluster overview.
	UrgencyBadge = "badge"

	// UrgencyBanner is 30 days or fewer: a persistent banner on every page.
	UrgencyBanner = "banner"

	// UrgencyCritical is 7 days or fewer, or already expired. The failure is
	// total and simultaneous, which is why there is a rung above "banner" that
	// the research's 90/30 did not name.
	UrgencyCritical = "critical"
)

// Machines returns every machine in the inventory, newest adoption last.
func (s *Service) Machines(ctx context.Context) ([]MachineView, error) {
	recs, err := s.deps.Store.Machines().List(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]MachineView, 0, len(recs))
	for _, rec := range recs {
		out = append(out, s.viewOf(rec))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AdoptedAt.Equal(out[j].AdoptedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].AdoptedAt.Before(out[j].AdoptedAt)
	})
	return out, nil
}

// MachineRecord reads the stored record rather than the view.
//
// It exists for the callers that make a decision about a machine rather than
// draw one: a view is assembled for a screen, and a rule reading a machine's
// role out of a `Field[T]`-carrying view is a rule reading a shape built for
// display. The removal path takes this.
func (s *Service) MachineRecord(ctx context.Context, id model.MachineID) (model.Machine, error) {
	rec, err := s.deps.Store.Machines().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return model.Machine{}, ErrNotFound
		}
		return model.Machine{}, err
	}
	return rec, nil
}

// Machine returns one machine.
func (s *Service) Machine(ctx context.Context, id model.MachineID) (MachineView, error) {
	rec, err := s.deps.Store.Machines().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return MachineView{}, ErrNotFound
		}
		return MachineView{}, err
	}
	return s.viewOf(rec), nil
}

// viewOf turns a stored machine into the answer shape.
//
// The rule it implements is D-14's table: a confirmed level's fields are
// available, an unconfirmed level's fields keep their value and gain the
// moment confirmation stopped, and a field nothing ever read carries a reason
// instead of a value. A stale field never loses its value -- dropping it is
// indistinguishable from null, and that is how the empty dashboard INV-08
// forbids comes about.
func (s *Service) viewOf(rec model.Machine) MachineView {
	unsupported, preRelease, notice := versionStanding(rec.Snapshot.TalosVersion)

	stage, levels := s.status(rec.ID, rec.Snapshot.ObservedAt)
	snap := rec.Snapshot

	v := MachineView{
		ID:                 rec.ID,
		Cluster:            rec.Cluster,
		Role:               rec.Role,
		Stage:              stage,
		LostAddr:           !rec.LostAddrAt.IsZero(),
		CertificateExpired: s.expiredCertificate(rec.ID),
		Locked:             rec.Locked,
		LockReason:         rec.LockReason,
		Labels:             rec.Labels,
		UnsupportedVersion: unsupported,
		PreRelease:         preRelease,
		VersionNotice:      notice,
		AdoptedAt:          rec.AdoptedAt,
		SeenAt:             rec.SeenAt,
		Watch:              s.watchStatus(rec.ID),
	}

	// Hostname and address are holzkube-manager's own record of what it last saw, so
	// they are LevelNone and always available: they are true regardless of
	// whether anything answers.
	v.Hostname = health.Known(health.LevelNone, rec.Hostname)
	v.Addr = health.Known(health.LevelNone, rec.Addr)

	node := fieldFactory{level: health.LevelNode, state: levels[health.LevelNode]}
	k8sLevel := fieldFactory{level: health.LevelK8s, state: levels[health.LevelK8s]}
	etcd := fieldFactory{level: health.LevelEtcd, state: levels[health.LevelEtcd]}

	v.TalosVersion = field(node, snap.TalosVersion, "the node has not reported its Talos version")
	v.Manufacturer = field(node, snap.Manufacturer, "the node has not reported its manufacturer")
	v.ProductName = field(node, snap.ProductName, "the node has not reported its product name")
	v.SerialNumber = field(node, snap.SerialNumber, "the node has not reported a serial number")
	v.MemoryMiB = field(node, snap.MemoryMiB, "the node has not reported its memory")
	v.CPUs = field(node, snap.CPUs, "the node has not reported its processors")
	v.Disks = field(node, snap.Disks, "the node has not reported its disks")
	v.Interfaces = field(node, snap.Interfaces, "the node has not reported its network interfaces")
	v.Services = field(node, snap.Services, "the node has not reported its services")

	// The schematic is LevelNode and is honestly empty for a node that was not
	// installed from a Factory image. An empty value with a reason is the
	// truth; a derived one is a phase 9 upgrade that strips extensions and
	// says it worked (D-12).
	v.SchematicID = field(node, snap.SchematicID,
		"this node reports no Image Factory schematic, which is what a node not installed from a Factory image looks like")

	v.KubernetesVersion = field(k8sLevel, snap.KubernetesVersion,
		"the node reports no kubelet, so its Kubernetes version is not known")

	if rec.Role == model.RoleControlPlane {
		v.EtcdMember = field(etcd, snap.EtcdMember, "etcd has not answered on this node")
	} else {
		v.EtcdMember = health.Never[bool](health.LevelEtcd, "a worker is not an etcd member")
	}

	verdict := compat.Check(snap.TalosVersion, snap.KubernetesVersion)
	v.Compatibility = field(node, Compatibility{
		Known:          verdict.Known,
		Supported:      verdict.Supported,
		HeadroomMinors: verdict.HeadroomMinors,
		Sentence:       verdict.Sentence,
	}, "no versions have been read from this node yet")

	return v
}

// fieldFactory carries one level's confirmation state so that field() reads as
// one line per fact.
type fieldFactory struct {
	level health.Level
	state levelState
}

// field builds one answer from a value and its level's state.
//
// zero values are the interesting case. A value that was never read and a
// value that is genuinely empty are indistinguishable in the stored snapshot,
// so the rule is: if the level is confirmed, the value is what the node said,
// empty or not. If it is not confirmed and there is a moment to point at, it
// is stale. Otherwise nothing has ever read it, and the reason says so.
func field[T any](f fieldFactory, value T, neverReason string) health.Field[T] {
	switch {
	case f.state.ok:
		return health.Known(f.level, value)
	case !f.state.confirmedAt.IsZero():
		reason := f.state.reason
		if reason == "" {
			reason = neverReason
		}
		return health.Stale(f.level, value, f.state.confirmedAt, reason)
	default:
		reason := f.state.reason
		if reason == "" {
			reason = neverReason
		}
		return health.Never[T](f.level, reason)
	}
}

// Clusters returns every cluster with its node counts and the certificate
// ladder.
func (s *Service) Clusters(ctx context.Context) ([]ClusterView, error) {
	clusters, err := s.deps.Store.Clusters().List(ctx)
	if err != nil {
		return nil, err
	}
	machines, err := s.Machines(ctx)
	if err != nil {
		return nil, err
	}

	now := s.deps.Now().UTC()
	out := make([]ClusterView, 0, len(clusters))
	for _, c := range clusters {
		out = append(out, clusterView(c, machines, now))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// Cluster returns one cluster.
func (s *Service) Cluster(ctx context.Context, id model.ClusterID) (ClusterView, error) {
	c, err := s.deps.Store.Clusters().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ClusterView{}, ErrNotFound
		}
		return ClusterView{}, err
	}
	machines, err := s.Machines(ctx)
	if err != nil {
		return ClusterView{}, err
	}
	return clusterView(c, machines, s.deps.Now().UTC()), nil
}

func clusterView(c model.Cluster, machines []MachineView, now time.Time) ClusterView {
	v := ClusterView{
		ID:                 c.ID,
		Name:               c.Name,
		Origin:             c.Origin,
		Endpoint:           c.Endpoint,
		Locked:             c.Locked,
		CreatedAt:          c.CreatedAt,
		ClientCertNotAfter: c.ClientCertNotAfter,
	}

	left := certificateExpiry(c, now)
	v.ClientCertDaysLeft = int(left / (24 * time.Hour))
	v.CertificateUrgency, v.CertificateWarning = certificateLadder(c.Name, left)

	for _, m := range machines {
		if m.Cluster != c.ID {
			continue
		}
		v.Nodes++
		if m.Role == model.RoleControlPlane {
			v.ControlPlane++
		} else {
			v.Workers++
		}
		switch m.Stage {
		case health.StageWatching:
			v.Healthy++
		case health.StageDegraded:
			v.Degraded++
		case health.StageDown:
			v.Down++
		case health.StageUnknown, health.StageConnecting:
			v.Checking++
		}
	}
	return v
}

// certificateLadder is D-23: 90 days a badge, 30 days a banner on every page,
// 7 days urgent, and expired a state of its own.
//
// The rungs are days rather than a percentage of the certificate's life
// because what matters is how long the operator has to act, not how far
// through the certificate is.
func certificateLadder(clusterName string, left time.Duration) (urgency, warning string) {
	const day = 24 * time.Hour

	switch {
	case left <= 0:
		return UrgencyCritical, "The client certificate for " + clusterName + " has expired. " +
			"Every node in this cluster is unreachable until it is replaced; the cluster itself is probably fine."
	case left <= 7*day:
		return UrgencyCritical, "The client certificate for " + clusterName + " expires within a week. " +
			"When it does, every node in this cluster becomes unreachable at once."
	case left <= 30*day:
		return UrgencyBanner, "The client certificate for " + clusterName + " expires within 30 days."
	case left <= 90*day:
		return UrgencyBadge, "The client certificate for " + clusterName + " expires within 90 days."
	default:
		return UrgencyNone, ""
	}
}

// versionStanding is OPS-03's marking, computed from what the node last said.
//
// Three answers rather than two, because "we have never heard a version from
// this node" is not "this node is fine": a machine whose snapshot carries no
// version is left unmarked and gets no notice, which is the honest reading --
// it is a node nothing has read yet, and inventing a verdict about it would be
// the marking saying something nobody knows.
func versionStanding(version string) (unsupported, preRelease bool, notice string) {
	if version == "" {
		return false, false, ""
	}

	preRelease = talos.IsPreRelease(version)
	unsupported = !talos.InSupportedRange(version)

	switch {
	case unsupported:
		return true, preRelease, fmt.Sprintf(
			"This node reports Talos %s, and this build of holzkube-manager is tested against %s "+
				"to %s. Every version-dependent action against it is refused: the client library "+
				"would happily talk to it, which is the problem -- an untested API surface that "+
				"answers is worse than one that refuses, because the divergence surfaces later, "+
				"on a cluster.",
			version, talos.MinSupportedVersion, talos.MaxSupportedVersion)
	case preRelease:
		return false, true, fmt.Sprintf(
			"This node reports Talos %s, which is a pre-release. It is inside the supported "+
				"range, and this instance accepts it only when started with --allow-prerelease: "+
				"everything this product guarantees about a node is a claim about released Talos.",
			version)
	}
	return false, false, ""
}
