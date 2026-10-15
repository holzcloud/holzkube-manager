package configdrift_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/configdrift"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The banner (D-06): the card and the metrics show a drift at once, and only
// the banner waits. It appears when a drift has run 24 hours unconfirmed,
// counted from its START, which is a fact of the record and not of the last
// reading. The tests never wait: HealthFor takes `now`, and the clock is moved.

var homelab = model.Cluster{ID: "c1", Name: "homelab"}

// driftOf asks for the view of the cluster and insists the drift is there and
// started at start: every test below that excludes a banner first has the case
// that would have brought one.
func driftOf(t *testing.T, svc *configdrift.Service, now time.Time, wantSince time.Time) *model.ConfigDriftHealth {
	t.Helper()
	h := svc.HealthFor(homelab, now)
	if h == nil || len(h.Items) == 0 {
		t.Fatalf("precondition: the view is %+v, want a pass with a node", h)
	}
	if h.Drifted != 1 {
		t.Fatalf("precondition: %d drifted nodes in %+v, want 1", h.Drifted, h.Items)
	}
	if !h.Items[0].Since.Equal(wantSince) {
		t.Fatalf("precondition: the drift started %v, want %v", h.Items[0].Since, wantSince)
	}
	return h
}

func TestTheBannerWaitsTwentyFourHours(t *testing.T) {
	t.Parallel()

	clk := newClock()
	t0 := clk.Now()
	d := newDrifting(t, withClock(clk))

	for _, tc := range []struct {
		after  time.Duration
		banner bool
	}{
		{0, false},
		{time.Hour, false},
		{configdrift.BannerAfter - time.Minute, false},
		{configdrift.BannerAfter, true},
		{configdrift.BannerAfter + time.Minute, true},
	} {
		h := driftOf(t, d.svc, t0.Add(tc.after), t0)
		if h.Banner != tc.banner {
			t.Errorf("a drift %v old: Banner is %v, want %v", tc.after, h.Banner, tc.banner)
		}
		if tc.banner && h.Warning == "" {
			t.Errorf("a drift %v old brings the banner without a sentence", tc.after)
		}
		if !tc.banner && h.Warning != "" {
			t.Errorf("a drift %v old carries a warning %q without the banner", tc.after, h.Warning)
		}
	}
	if configdrift.BannerAfter != 24*time.Hour {
		t.Errorf("BannerAfter is %v, want 24h", configdrift.BannerAfter)
	}
}

func TestTheBannerCountsFromTheStartOfTheDriftNotTheLastReading(t *testing.T) {
	t.Parallel()

	clk := newClock()
	t0 := clk.Now()
	d := newDrifting(t, withClock(clk))
	ctx := context.Background()

	// Readings every hour for a day; the last one is an hour old at the 24th.
	for range 23 {
		clk.Advance(time.Hour)
		d.svc.Tick(ctx)
	}
	h := driftOf(t, d.svc, t0.Add(23*time.Hour), t0)
	if h.Banner {
		t.Fatal("precondition: 23 hours of drift bring the banner")
	}
	if age := t0.Add(configdrift.BannerAfter).Sub(h.Items[0].ReadAt); age >= configdrift.BannerAfter {
		t.Fatalf("precondition: the last reading is %v old at the 24th hour, so counting from it would show nothing", age)
	}

	h = driftOf(t, d.svc, t0.Add(configdrift.BannerAfter), t0)
	if !h.Banner {
		t.Error("24 hours after the start of the drift there is no banner, though the last reading is younger")
	}

	// A day later, many readings on, the start is still the first one.
	clk.Advance(2 * time.Hour)
	d.svc.Tick(ctx)
	driftOf(t, d.svc, clk.Now(), t0)
}

func TestTheStartOfADriftReachesTheBannerAfterARestart(t *testing.T) {
	t.Parallel()

	clk := newClock()
	t0 := clk.Now()
	d := newDrifting(t, withClock(clk))

	d.n.reopen(t)
	again := d.n.service(t, withRead(d.fake.Read), withClock(clk))
	clk.Advance(25 * time.Hour)
	if h := again.HealthFor(homelab, clk.Now()); h != nil {
		t.Fatalf("precondition: a new process that has read nothing says %+v, want \"not read yet\"", h)
	}

	again.Tick(context.Background())

	h := driftOf(t, again, clk.Now(), t0)
	if !h.Banner {
		t.Error("a restart started the count again: 25 hours after the start there is no banner")
	}
}

func TestAnUnreadableNodeWithADriftOlderThanADayStillBringsTheBanner(t *testing.T) {
	t.Parallel()

	clk := newClock()
	t0 := clk.Now()
	d := newDrifting(t, withClock(clk))
	ctx := context.Background()

	clk.Advance(25 * time.Hour)
	d.fake.set(machineconfig.Snap{}, errors.New("no route to the node"))
	d.svc.Tick(ctx)

	h := driftOf(t, d.svc, clk.Now(), t0)
	if h.Items[0].State != string(configdrift.StateUnreadable) {
		t.Fatalf("precondition: the node is %q, want unreadable", h.Items[0].State)
	}
	if !h.Banner {
		t.Error("a node that stopped answering lost the banner it had earned")
	}

	// The same after a restart: the start is in the record, not in the process.
	d.n.reopen(t)
	again := d.n.service(t, withRead(d.fake.Read), withClock(clk))
	again.Tick(ctx)
	h = driftOf(t, again, clk.Now(), t0)
	if h.Items[0].State != string(configdrift.StateUnreadable) || !h.Banner {
		t.Errorf("after a restart the node is %q with Banner %v, want unreadable with the banner", h.Items[0].State, h.Banner)
	}
}

func TestAnUnreadableNodeIsNotCountedAsRead(t *testing.T) {
	t.Parallel()

	clk := newClock()
	t0 := clk.Now()
	d := newDrifting(t, withClock(clk))

	if h := driftOf(t, d.svc, t0, t0); h.ReadNodes != 1 || h.Nodes != 1 {
		t.Fatalf("precondition: a node that answered is %d of %d read, want 1 of 1", h.ReadNodes, h.Nodes)
	}

	clk.Advance(time.Hour)
	d.fake.set(machineconfig.Snap{}, errors.New("no route to the node"))
	d.svc.Tick(context.Background())

	h := driftOf(t, d.svc, clk.Now(), t0)
	if h.Items[0].State != string(configdrift.StateUnreadable) {
		t.Fatalf("precondition: the node is %q, want unreadable", h.Items[0].State)
	}
	if h.ReadNodes != 0 || h.Nodes != 1 {
		t.Errorf("a node that did not answer is %d of %d read, want 0 of 1", h.ReadNodes, h.Nodes)
	}
}

func TestConfirmingTakesTheBannerAway(t *testing.T) {
	t.Parallel()

	clk := newClock()
	t0 := clk.Now()
	d := newDrifting(t, withClock(clk))
	ctx := context.Background()

	clk.Advance(25 * time.Hour)
	d.svc.Tick(ctx)
	if h := driftOf(t, d.svc, clk.Now(), t0); !h.Banner {
		t.Fatal("precondition: a drift of 25 hours has no banner")
	}

	if err := d.svc.Confirm(ctx, "m1", d.after.Digest); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	h := d.svc.HealthFor(homelab, clk.Now())
	if h == nil || len(h.Items) != 1 {
		t.Fatalf("the view is %+v, want one node", h)
	}
	if h.Banner || h.Warning != "" {
		t.Errorf("after confirming the banner is %v with %q", h.Banner, h.Warning)
	}
	if h.Drifted != 0 || !h.Items[0].Since.IsZero() || h.Items[0].State != string(configdrift.StateOK) {
		t.Errorf("after confirming the item is %+v with %d drifted, want ok, no start, none", h.Items[0], h.Drifted)
	}
}

func TestAnOkOrSettlingNodeNeverBringsABanner(t *testing.T) {
	t.Parallel()

	e := newExpecting(t)
	later := func() time.Time { return e.clk.Now().Add(72 * time.Hour) }

	h := e.svc.HealthFor(homelab, later())
	if h == nil || len(h.Items) != 1 || h.Items[0].State != string(configdrift.StateBaselineCreated) {
		t.Fatalf("precondition: the view is %+v, want the node in baseline-created", h)
	}
	if h.Banner || h.Warning != "" {
		t.Errorf("a node whose baseline was just laid brings the banner %q three days later", h.Warning)
	}

	d := e.read(t, e.base)
	if d.State != configdrift.StateOK {
		t.Fatalf("precondition: the node is %q, want ok", d.State)
	}
	if h := e.svc.HealthFor(homelab, later()); h.Banner || h.Drifted != 0 {
		t.Errorf("an ok node: Banner %v, %d drifted", h.Banner, h.Drifted)
	}

	e.expect(t, e.own, "no-reboot", e.clk.Now())
	e.clk.Advance(time.Minute)
	if d := e.read(t, e.third); d.State != configdrift.StateSettling {
		t.Fatalf("precondition: the node is %q, want settling", d.State)
	}
	h = e.svc.HealthFor(homelab, later())
	if h.Items[0].State != string(configdrift.StateSettling) {
		t.Fatalf("precondition: the item is %q, want settling", h.Items[0].State)
	}
	if h.Banner || h.Drifted != 0 || !h.Items[0].Since.IsZero() {
		t.Errorf("a settling node: Banner %v, %d drifted, start %v", h.Banner, h.Drifted, h.Items[0].Since)
	}
}

// twoNodes is a cluster of two nodes, each with a reader of its own, so that
// the two can start to drift at different moments. cp-2 is the second machine.
type twoNodes struct {
	n      *node
	svc    *configdrift.Service
	clk    *clock
	f1, f2 *fakeReader
	before machineconfig.Snap
	after  machineconfig.Snap
}

func newTwoNodes(t *testing.T) *twoNodes {
	t.Helper()

	n := newNode(t)
	second := model.Machine{ID: "m2", Cluster: "c1", Hostname: "cp-2", Role: model.RoleControlPlane}
	inv := &inventory{
		machines: []model.Machine{n.machine, second},
		clusters: []model.Cluster{homelab},
	}
	w := &twoNodes{n: n, clk: newClock(), f1: &fakeReader{}, f2: &fakeReader{}}
	w.before, w.after = twoSnaps(t, n)
	w.svc = n.service(t, withInventory(inv), withClock(w.clk), withRead(func(ctx context.Context, id model.MachineID) (machineconfig.Snap, error) {
		if id == "m2" {
			return w.f2.Read(ctx, id)
		}
		return w.f1.Read(ctx, id)
	}))
	w.f1.set(w.before, nil)
	w.f2.set(w.before, nil)
	w.svc.Tick(context.Background())
	return w
}

func TestTheBannerNamesTheOldestNodeAndCountsTheRest(t *testing.T) {
	t.Parallel()

	w := newTwoNodes(t)
	ctx := context.Background()
	t0 := w.clk.Now()

	// cp-2 starts to drift first, an hour before cp-1. cp-1 sorts first by
	// name, so naming "the first item" would name the wrong one.
	w.clk.Advance(time.Minute)
	w.f2.set(w.after, nil)
	w.svc.Tick(ctx)
	w.clk.Advance(time.Hour)
	w.f1.set(w.after, nil)
	w.svc.Tick(ctx)

	oldestStart := t0.Add(time.Minute)
	h := w.svc.HealthFor(homelab, oldestStart.Add(configdrift.BannerAfter))
	if h == nil || h.Nodes != 2 || h.Drifted != 2 || len(h.Items) != 2 {
		t.Fatalf("precondition: the view is %+v, want two nodes, both drifting", h)
	}
	if h.Items[0].Node != "cp-1" || h.Items[1].Node != "cp-2" || !h.Items[1].Since.Before(h.Items[0].Since) {
		t.Fatalf("precondition: items %+v, want cp-1 first by name and cp-2 the older drift", h.Items)
	}
	if !h.Banner {
		t.Fatal("the oldest drift is 24 hours old and there is no banner; the younger one does not take it away")
	}
	for _, want := range []string{"cp-2", "homelab", "2026-10-14 09:01 UTC", "1 other node"} {
		if !strings.Contains(h.Warning, want) {
			t.Errorf("the warning %q does not name %q", h.Warning, want)
		}
	}
	if strings.Contains(h.Warning, "cp-1") {
		t.Errorf("the warning %q names the younger node; it names the oldest and counts the rest", h.Warning)
	}

	// Once both are old enough it is still the oldest that is named.
	h = w.svc.HealthFor(homelab, oldestStart.Add(26*time.Hour))
	if !h.Banner || !strings.Contains(h.Warning, "cp-2") || strings.Contains(h.Warning, "cp-1") {
		t.Errorf("26 hours on, with both drifts older than a day: Banner %v, warning %q, want the oldest (cp-2) named and not the other", h.Banner, h.Warning)
	}

	// A second before the oldest drift is 24 hours old there is no banner,
	// however young the other one is.
	h = w.svc.HealthFor(homelab, oldestStart.Add(configdrift.BannerAfter-time.Second))
	if h.Banner {
		t.Error("the banner is there a second before the oldest drift is 24 hours old")
	}
}

func TestTheBannerSentenceNamesNoConfigurationContent(t *testing.T) {
	t.Parallel()

	clk := newClock()
	t0 := clk.Now()
	d := newDrifting(t, withClock(clk))

	h := driftOf(t, d.svc, t0.Add(30*time.Hour), t0)
	if !h.Banner {
		t.Fatal("precondition: no banner after 30 hours")
	}
	for _, leak := range []string{
		"KubeletConfig", "kubelet", "ghcr.io", "image", "v1.34", d.after.Digest, d.before.Digest, "machine:", "config-baselines",
	} {
		if strings.Contains(h.Warning, leak) {
			t.Errorf("the banner sentence %q carries %q", h.Warning, leak)
		}
	}
	for _, want := range []string{"cp-1", "homelab", "2026-10-14 09:00 UTC"} {
		if !strings.Contains(h.Warning, want) {
			t.Errorf("the banner sentence %q does not say %q", h.Warning, want)
		}
	}
}
