// Package certwatch reads the certificates a cluster's control-plane nodes hold
// and says how long they have left.
//
// holzkube-manager already counts down the one client certificate it holds
// itself. This is the other half: the certificates inside the cluster, which
// nobody sees until one runs out. Talos renews the ones it issues, so an
// issued certificate that is nearly out means a rotation is stuck; nothing
// renews an authority, so its expiry is a date that has to be acted on.
//
// The read is periodic and held in memory. A restart starts with nothing and
// the first pass fills it again, and until it does the cluster says "not read
// yet" rather than "fine".
package certwatch

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// Urgency rungs, the same words the client-certificate ladder uses.
const (
	None     = "none"
	Badge    = "badge"
	Banner   = "banner"
	Critical = "critical"
)

const day = 24 * time.Hour

// Ladder is where a certificate stands, given what it is and how long it has
// left.
//
// An authority follows the client certificate's ladder (90, 30 and 7 days):
// nothing renews it, so the operator needs the whole three months. An issued
// certificate is renewed by Talos long before it runs out, so being close at
// all is the fault: 14 days is already a banner and 3 days is critical.
func Ladder(kind talos.CertificateKind, left time.Duration) string {
	if kind == talos.CertificateAuthority {
		switch {
		case left <= 7*day:
			return Critical
		case left <= 30*day:
			return Banner
		case left <= 90*day:
			return Badge
		}
		return None
	}
	switch {
	case left <= 3*day:
		return Critical
	case left <= 14*day:
		return Banner
	}
	return None
}

func rank(urgency string) int {
	switch urgency {
	case Critical:
		return 3
	case Banner:
		return 2
	case Badge:
		return 1
	}
	return 0
}

// Sentence says what a rung means for one certificate.
func Sentence(cluster string, c model.NodeCertificate) string {
	var what string
	if c.Kind == string(talos.CertificateAuthority) {
		what = "The " + c.Name + " of " + cluster
	} else {
		what = "The " + c.Name + " certificate on " + c.Node + " (" + cluster + ")"
	}
	switch {
	case c.DaysLeft <= 0 && c.Kind == string(talos.CertificateAuthority):
		return what + " has expired. Nodes and clients that trust it can no longer be verified; this needs a CA rotation."
	case c.DaysLeft <= 0:
		return what + " has expired. Talos should have renewed it; check that node."
	case c.Kind == string(talos.CertificateAuthority):
		return fmt.Sprintf("%s expires in %d days. Nothing renews an authority; plan a CA rotation.", what, c.DaysLeft)
	default:
		return fmt.Sprintf("%s expires in %d days. Talos renews it on its own, so a rotation on that node is stuck.", what, c.DaysLeft)
	}
}

// DefaultReadBudget bounds one node's connection and read. A control plane that
// is down must not hold up the pass for the others, and a few small resources
// are a fast read.
const DefaultReadBudget = 20 * time.Second

// Deps is what the service needs from the rest of the program.
type Deps struct {
	Clusters      func(ctx context.Context) ([]model.Cluster, error)
	ControlPlanes func(ctx context.Context, id model.ClusterID) ([]model.Machine, error)

	// Read returns the certificates one node holds.
	Read func(ctx context.Context, id model.MachineID) ([]talos.Certificate, error)

	// Every is the period between passes. Zero means six hours.
	Every time.Duration

	// ReadBudget is how long one node may take to answer, connection and read
	// together. Zero means DefaultReadBudget. It is a deadline put on the
	// context handed to Read, because the Talos client refuses a call that
	// carries none (internal/talos/deadline.go) and the context Run is started
	// with has none: in v0.6.0 and v0.7.0 that refusal was the reason nothing was
	// ever read.
	ReadBudget time.Duration

	// Now defaults to time.Now.
	Now func() time.Time

	Log func(msg string, args ...any)
}

type reading struct {
	at           time.Time
	readNodes    int
	controlPlane int
	items        []model.NodeCertificate
}

// Service holds the last reading of each cluster.
type Service struct {
	deps Deps

	mu       sync.Mutex
	readings map[model.ClusterID]reading
}

// New builds the service.
func New(d Deps) *Service {
	if d.Every == 0 {
		d.Every = 6 * time.Hour
	}
	if d.ReadBudget == 0 {
		d.ReadBudget = DefaultReadBudget
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = func(string, ...any) {}
	}
	return &Service{deps: d, readings: map[model.ClusterID]reading{}}
}

// Run reads once shortly after start, so the overview is not empty for six
// hours after a restart, and then every period until ctx ends.
func (s *Service) Run(ctx context.Context, firstAfter time.Duration) {
	timer := time.NewTimer(firstAfter)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.Tick(ctx)
			timer.Reset(s.deps.Every)
		}
	}
}

// Tick makes one pass over every cluster.
func (s *Service) Tick(ctx context.Context) {
	clusters, err := s.deps.Clusters(ctx)
	if err != nil {
		s.deps.Log("certificate watch: list the clusters", "err", err)
		return
	}
	for _, c := range clusters {
		if ctx.Err() != nil {
			return
		}
		s.Refresh(ctx, c.ID)
	}
}

// Refresh reads one cluster's control-plane nodes now.
//
// A node that does not answer is counted and left out. It does not erase what
// an earlier pass learned about it: a node that is down for a reboot would
// otherwise take its certificates off the screen exactly when somebody is
// looking at that node.
func (s *Service) Refresh(ctx context.Context, id model.ClusterID) {
	machines, err := s.deps.ControlPlanes(ctx, id)
	if err != nil {
		s.deps.Log("certificate watch: list the control planes", "cluster", string(id), "err", err)
		return
	}

	s.mu.Lock()
	previous := s.readings[id]
	s.mu.Unlock()

	next := reading{at: s.deps.Now(), controlPlane: len(machines)}
	for _, m := range machines {
		readCtx, cancel := context.WithTimeout(ctx, s.deps.ReadBudget)
		certs, err := s.deps.Read(readCtx, m.ID)
		cancel()
		if err != nil {
			s.deps.Log("certificate watch: read a node", "cluster", string(id), "node", m.Hostname, "err", err)
			for _, old := range previous.items {
				if old.Machine == m.ID {
					next.items = append(next.items, old)
				}
			}
			continue
		}
		next.readNodes++
		for _, c := range certs {
			next.items = append(next.items, model.NodeCertificate{
				Machine: m.ID, Node: m.Hostname,
				Name: c.Name, Kind: string(c.Kind), Subject: c.Subject, NotAfter: c.NotAfter,
			})
		}
	}

	s.mu.Lock()
	s.readings[id] = next
	s.mu.Unlock()
}

// HealthFor is the standing at `now`. It returns nil until a pass has read the
// cluster, which the overview shows as "not read yet".
func (s *Service) HealthFor(c model.Cluster, now time.Time) *model.CertificateHealth {
	s.mu.Lock()
	r, ok := s.readings[c.ID]
	s.mu.Unlock()
	if !ok {
		return nil
	}

	h := &model.CertificateHealth{
		CheckedAt: r.at, ReadNodes: r.readNodes, ControlPlane: r.controlPlane,
		Urgency: None, Items: make([]model.NodeCertificate, 0, len(r.items)),
	}
	worst := -1
	for _, it := range r.items {
		left := it.NotAfter.Sub(now)
		it.DaysLeft = int(left / day)
		it.Urgency = Ladder(talos.CertificateKind(it.Kind), left)
		h.Items = append(h.Items, it)
	}
	sort.SliceStable(h.Items, func(i, j int) bool { return h.Items[i].NotAfter.Before(h.Items[j].NotAfter) })
	for i, it := range h.Items {
		if rank(it.Urgency) > rank(h.Urgency) {
			h.Urgency = it.Urgency
			worst = i
		}
	}
	if worst >= 0 {
		h.Warning = Sentence(c.Name, h.Items[worst])
	}
	return h
}
