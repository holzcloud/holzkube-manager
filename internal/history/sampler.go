package history

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The sampler: the one thing in this product that reads the fleet on a timer
// for nobody in particular.
//
// # Why it exists at all
//
// The hardware view is a gauge read when somebody looks, and it stays that
// (inventory/hardware.go). But a gauge has no past, and "what was this node
// doing an hour ago, before I opened the page" is the question a chart exists
// to answer. Something has to have been looking. So every fifteen seconds this
// asks each node the same question the panel asks, and each cluster the same
// question the apps screen asks, and files the answer.
//
// It reuses both reads rather than having its own, and that is a property: the
// numbers on a chart are the numbers the gauge beside it shows, computed by the
// same code, with the same exclusions (no veths, no partitions, no loop
// devices) and the same budgets. A second reader would be a second answer to
// "how busy is this node" that nothing keeps in step with the first.
//
// # What it promises about the fleet
//
// It only reads. It is started in --dry-run as well, for that reason: dry-run
// promises to change nothing, and a CPU counter read changes nothing.
//
// Passes never overlap. One goroutine runs them one after the other off a
// ticker, and a pass that outlasts the interval makes the ticker drop the ticks
// it missed rather than queue them: a fleet slow enough to take longer than
// fifteen seconds is one that should be asked less often, not twice at once.
//
// A node that does not answer records nothing -- a gap, never a zero. A
// failing subject is logged once when it starts failing and once when it
// answers again, and not every fifteen seconds in between: a powered-off node
// is the ordinary state of a homelab, and a journal with 5760 lines a day about
// it is a journal nobody reads.

// appsListAllowance is what the app read may spend beyond the kubelets' shared
// ceiling: the seven list calls before it, against an API server that answers
// them from memory in milliseconds. It is small on purpose -- the apps read is
// bounded by kube.SummaryBudget plus this, fourteen seconds, so a pass against
// a cluster whose API server hangs still ends inside the fifteen-second
// interval.
const appsListAllowance = 4 * time.Second

// AppsBudget bounds one cluster's apps read in a pass. The hardware read
// carries its own bound, inventory.HardwareBudget, inside Service.Hardware.
const AppsBudget = kube.SummaryBudget + appsListAllowance

// hardwareConcurrency is how many nodes are read at once. A homelab's handful
// in one round; a larger fleet in several, all inside HardwareBudget each.
const hardwareConcurrency = 8

// SamplerDeps is what the sampler reads with and writes to.
type SamplerDeps struct {
	History *Store
	Logger  *slog.Logger
	Now     func() time.Time

	// Interval is FineStep unless a test says otherwise.
	Interval time.Duration

	// Inventory is every machine and cluster on record. Only machines in a
	// cluster are sampled; the lists also decide whose history is deleted.
	Inventory func(ctx context.Context) ([]model.Machine, []model.Cluster, error)

	// Hardware is inventory.Service.Hardware.
	Hardware func(ctx context.Context, id model.MachineID) (inventory.HardwareView, error)

	// Apps is one cluster's apps list, over every namespace.
	Apps func(ctx context.Context, cluster model.ClusterID) (kube.Apps, error)
}

// Sampler fills a Store.
type Sampler struct {
	deps SamplerDeps

	mu      sync.Mutex
	failing map[string]bool
}

// NewSampler builds a sampler. It starts nothing; Run does.
func NewSampler(d SamplerDeps) *Sampler {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Interval <= 0 {
		d.Interval = FineStep
	}
	return &Sampler{deps: d, failing: map[string]bool{}}
}

// Run samples until ctx is done, then writes the file one last time.
//
// The first pass is at once rather than an interval in: a daemon restarted by
// the hourly update should not leave a fifteen-second hole of its own making
// in every chart.
func (s *Sampler) Run(ctx context.Context) {
	tick := time.NewTicker(s.deps.Interval)
	defer tick.Stop()

	for {
		s.pass(ctx)
		if err := s.deps.History.FlushIfDue(s.deps.Now()); err != nil {
			s.deps.Logger.Warn("could not write the metrics history", slog.Any("error", err))
		}

		select {
		case <-ctx.Done():
			// Written whatever the minute says: this is a restart, most
			// likely the hourly update, and the minute since the last write
			// would otherwise be the one hole in every chart.
			if err := s.deps.History.Flush(s.deps.Now()); err != nil {
				s.deps.Logger.Warn("could not write the metrics history on shutdown", slog.Any("error", err))
			}
			return
		case <-tick.C:
		}
	}
}

// pass is one round: the inventory, then every node and every cluster at once,
// all stamped with the moment the pass began so that one pass is one slot.
func (s *Sampler) pass(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	at := s.deps.Now()

	machines, clusters, err := s.deps.Inventory(ctx)
	if err != nil {
		s.failed("inventory", "the inventory could not be listed; nothing is sampled", err)
		return
	}
	s.recovered("inventory", "the inventory can be listed again")

	known := make(map[model.MachineID]bool, len(machines))
	for _, m := range machines {
		known[m.ID] = true
	}
	live := make(map[model.ClusterID]bool, len(clusters))
	for _, c := range clusters {
		live[c.ID] = true
	}
	s.deps.History.Retain(known, live)
	s.deps.History.Prune(at)
	s.forgetFailing(known, live)

	var wg sync.WaitGroup
	sem := make(chan struct{}, hardwareConcurrency)
	for _, m := range machines {
		if m.Cluster == "" {
			// A machine in no cluster is in maintenance mode or on its way
			// somewhere, and is not what the charts are for.
			continue
		}
		wg.Add(1)
		go func(id model.MachineID) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			s.sampleMachine(ctx, id, at)
		}(m.ID)
	}
	for _, c := range clusters {
		wg.Add(1)
		go func(id model.ClusterID) {
			defer wg.Done()
			s.sampleCluster(ctx, id, at)
		}(c.ID)
	}
	wg.Wait()
}

func (s *Sampler) sampleMachine(ctx context.Context, id model.MachineID, at time.Time) {
	key := MachineSubject(id)
	view, err := s.deps.Hardware(ctx, id)
	if err != nil {
		s.failed(key, "a node did not answer the hardware read; its charts have a gap until it does", err)
		return
	}
	s.recovered(key, "a node answers the hardware read again")
	s.deps.History.Record(key, at, HardwareValues(view))
}

func (s *Sampler) sampleCluster(ctx context.Context, id model.ClusterID, at time.Time) {
	key := "cluster/" + string(id)

	ctx, cancel := context.WithTimeout(ctx, AppsBudget)
	defer cancel()

	apps, err := s.deps.Apps(ctx, id)
	if err != nil {
		s.failed(key, "a cluster's apps could not be read; their charts have a gap until they can", err)
		return
	}
	s.recovered(key, "a cluster's apps can be read again")

	names := make([]string, 0, len(apps.Apps))
	for _, app := range apps.Apps {
		subject := AppSubject(id, app.Namespace, app.Kind, app.Name)
		names = append(names, subject)
		if !app.UsageKnown {
			// Unknown is not zero: the node it runs on did not answer, or
			// nobody is measuring. Nothing is recorded, so the chart shows a
			// gap rather than an app that stopped using anything.
			continue
		}
		s.deps.History.Record(subject, at, map[string]float64{
			"cpu":    float64(app.CPUMillis),
			"memory": float64(app.MemoryBytes),
		})
	}
	s.deps.History.SetApps(id, names)
}

// HardwareValues is what one hardware reading contributes to a machine's
// history. The keys are the contract's, and they are the ones the panel's own
// live series already used, so a chart can splice the two.
func HardwareValues(v inventory.HardwareView) map[string]float64 {
	out := map[string]float64{
		"cpu": v.CPU.UsagePercent,
	}
	// A node that reported no memory total has not reported its memory, and a
	// percentage of nothing is not zero percent.
	if v.Memory.TotalBytes > 0 {
		out["memory"] = 100 * float64(v.Memory.UsedBytes) / float64(v.Memory.TotalBytes)
	}

	var rx, tx, read, write float64
	for _, l := range v.Network {
		rx += l.RxBytesPerSec
		tx += l.TxBytesPerSec
	}
	for _, d := range v.Disks {
		read += d.ReadBytesPerSec
		write += d.WriteBytesPerSec
	}
	out["rx"], out["tx"], out["read"], out["write"] = rx, tx, read, write

	for i, p := range v.CPU.PerCore {
		out["core:"+strconv.Itoa(i)] = p
	}
	for _, t := range v.Temperatures {
		out["temp:"+t.Chip+"/"+t.Label] = t.Celsius
	}
	for _, f := range v.Fans {
		out["fan:"+f.Chip+"/"+f.Label] = float64(f.RPM)
	}
	return out
}

// failed logs a subject's failure the first time, and stays quiet until it
// recovers.
func (s *Sampler) failed(key, msg string, err error) {
	s.mu.Lock()
	first := !s.failing[key]
	s.failing[key] = true
	s.mu.Unlock()
	if first {
		s.deps.Logger.Warn(msg, slog.String("subject", key), slog.Any("error", err))
	}
}

// recovered logs the end of a failure, once.
func (s *Sampler) recovered(key, msg string) {
	s.mu.Lock()
	was := s.failing[key]
	delete(s.failing, key)
	s.mu.Unlock()
	if was {
		s.deps.Logger.Info(msg, slog.String("subject", key))
	}
}

// forgetFailing drops the failure memory of machines and clusters that were
// forgotten, so a map keyed by things that no longer exist does not grow for
// the life of the process.
func (s *Sampler) forgetFailing(machines map[model.MachineID]bool, clusters map[model.ClusterID]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key := range s.failing {
		if key == "inventory" {
			continue
		}
		if id, ok := strings.CutPrefix(key, "cluster/"); ok {
			if !clusters[model.ClusterID(id)] {
				delete(s.failing, key)
			}
			continue
		}
		if !retained(key, machines, clusters) {
			delete(s.failing, key)
		}
	}
}

// clientTTL is how long one minted Kubernetes client is reused for a cluster's
// apps reads. The certificate it carries lasts an hour (kube.MintCreds); ten
// minutes is well inside that, and it spares a Pi generating a key and signing
// a certificate every fifteen seconds -- and the API server a new TLS
// handshake each time.
const clientTTL = 10 * time.Minute

// KubeApps is the sampler's Apps, built on the inventory's Kubernetes client
// and keeping one per cluster for clientTTL. A client whose read failed is
// dropped, so a cluster whose API server moved is looked up again on the next
// pass rather than asked at the old address for ten minutes.
func KubeApps(
	client func(ctx context.Context, cluster model.ClusterID) (*kube.Client, error),
	now func() time.Time,
) func(ctx context.Context, cluster model.ClusterID) (kube.Apps, error) {
	type cached struct {
		c  *kube.Client
		at time.Time
	}
	var mu sync.Mutex
	clients := map[model.ClusterID]cached{}

	return func(ctx context.Context, cluster model.ClusterID) (kube.Apps, error) {
		mu.Lock()
		entry, ok := clients[cluster]
		mu.Unlock()

		if !ok || now().Sub(entry.at) > clientTTL {
			c, err := client(ctx, cluster)
			if err != nil {
				return kube.Apps{}, err
			}
			entry = cached{c: c, at: now()}
			mu.Lock()
			clients[cluster] = entry
			mu.Unlock()
		}

		apps, err := entry.c.Apps(ctx, kube.AppsQuery{}, now())
		if err != nil {
			mu.Lock()
			delete(clients, cluster)
			mu.Unlock()
		}
		return apps, err
	}
}
