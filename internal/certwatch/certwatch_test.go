package certwatch_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/certwatch"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

const day = 24 * time.Hour

func TestLadder(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind talos.CertificateKind
		left time.Duration
		want string
	}{
		{talos.CertificateAuthority, 200 * day, certwatch.None},
		{talos.CertificateAuthority, 90 * day, certwatch.Badge},
		{talos.CertificateAuthority, 30 * day, certwatch.Banner},
		{talos.CertificateAuthority, 7 * day, certwatch.Critical},
		{talos.CertificateAuthority, -time.Hour, certwatch.Critical},
		// An issued certificate is renewed by Talos; 60 days left is normal.
		{talos.CertificateIssued, 60 * day, certwatch.None},
		{talos.CertificateIssued, 20 * day, certwatch.None},
		{talos.CertificateIssued, 14 * day, certwatch.Banner},
		{talos.CertificateIssued, 3 * day, certwatch.Critical},
		{talos.CertificateIssued, -time.Hour, certwatch.Critical},
	}
	for _, c := range cases {
		if got := certwatch.Ladder(c.kind, c.left); got != c.want {
			t.Errorf("Ladder(%s, %v) = %s, want %s", c.kind, c.left, got, c.want)
		}
	}
}

type fixture struct {
	now   time.Time
	certs map[model.MachineID][]talos.Certificate
	fail  map[model.MachineID]bool
}

func (f *fixture) service() *certwatch.Service {
	return certwatch.New(certwatch.Deps{
		Clusters: func(context.Context) ([]model.Cluster, error) {
			return []model.Cluster{{ID: "c1", Name: "home"}}, nil
		},
		ControlPlanes: func(context.Context, model.ClusterID) ([]model.Machine, error) {
			return []model.Machine{
				{ID: "m1", Hostname: "cp-1", Role: model.RoleControlPlane},
				{ID: "m2", Hostname: "cp-2", Role: model.RoleControlPlane},
			}, nil
		},
		Read: func(_ context.Context, id model.MachineID) ([]talos.Certificate, error) {
			if f.fail[id] {
				return nil, errors.New("node down")
			}
			return f.certs[id], nil
		},
		Now: func() time.Time { return f.now },
	})
}

func cert(name string, kind talos.CertificateKind, notAfter time.Time) talos.Certificate {
	return talos.Certificate{Name: name, Kind: kind, Subject: name, NotAfter: notAfter}
}

func TestNothingIsSaidBeforeTheFirstRead(t *testing.T) {
	t.Parallel()
	f := &fixture{now: time.Now()}
	if h := f.service().HealthFor(model.Cluster{ID: "c1"}, f.now); h != nil {
		t.Fatalf("health before any read = %+v, want nil (not read yet)", h)
	}
}

func TestTheWorstCertificateSetsTheWarning(t *testing.T) {
	t.Parallel()
	now := time.Now()
	f := &fixture{now: now, certs: map[model.MachineID][]talos.Certificate{
		"m1": {
			cert("etcd peer", talos.CertificateIssued, now.Add(300*day)),
			cert("Kubernetes authority", talos.CertificateAuthority, now.Add(20*day)),
		},
		"m2": {cert("etcd peer", talos.CertificateIssued, now.Add(2*day))},
	}}
	s := f.service()
	s.Tick(t.Context())

	h := s.HealthFor(model.Cluster{ID: "c1", Name: "home"}, now)
	if h == nil {
		t.Fatal("no health after a read")
	}
	if h.Urgency != certwatch.Critical {
		t.Errorf("Urgency = %s, want critical (an issued certificate with 2 days left)", h.Urgency)
	}
	if !strings.Contains(h.Warning, "cp-2") || !strings.Contains(h.Warning, "stuck") {
		t.Errorf("Warning = %q, want it to name cp-2 and the stuck rotation", h.Warning)
	}
	if h.ReadNodes != 2 || h.ControlPlane != 2 {
		t.Errorf("read %d of %d nodes, want 2 of 2", h.ReadNodes, h.ControlPlane)
	}
	if len(h.Items) != 3 || h.Items[0].DaysLeft != 2 {
		t.Errorf("items not soonest first: %+v", h.Items)
	}
}

func TestTheReadingCountsDownBetweenPasses(t *testing.T) {
	t.Parallel()
	now := time.Now()
	f := &fixture{now: now, certs: map[model.MachineID][]talos.Certificate{
		"m1": {cert("Talos API authority", talos.CertificateAuthority, now.Add(100*day))},
	}}
	s := f.service()
	s.Tick(t.Context())

	c := model.Cluster{ID: "c1", Name: "home"}
	if h := s.HealthFor(c, now); h.Urgency != certwatch.None {
		t.Fatalf("today = %s, want none", h.Urgency)
	}
	if h := s.HealthFor(c, now.Add(40*day)); h.Urgency != certwatch.Badge {
		t.Errorf("40 days later = %s, want badge without another read", h.Urgency)
	}
}

func TestADownNodeKeepsItsLastReading(t *testing.T) {
	t.Parallel()
	now := time.Now()
	f := &fixture{now: now, certs: map[model.MachineID][]talos.Certificate{
		"m1": {cert("etcd peer", talos.CertificateIssued, now.Add(2*day))},
		"m2": {cert("etcd peer", talos.CertificateIssued, now.Add(300*day))},
	}}
	s := f.service()
	s.Tick(t.Context())

	f.fail = map[model.MachineID]bool{"m1": true}
	s.Tick(t.Context())

	h := s.HealthFor(model.Cluster{ID: "c1", Name: "home"}, now)
	if h.ReadNodes != 1 {
		t.Errorf("ReadNodes = %d, want 1 (cp-1 did not answer)", h.ReadNodes)
	}
	if h.Urgency != certwatch.Critical {
		t.Errorf("Urgency = %s: a node that is down took its expiring certificate off the screen", h.Urgency)
	}
}
