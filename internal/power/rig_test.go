package power_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/power"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// The rig: simulated Talos nodes, a simulated API server that knows them as
// Kubernetes nodes, the real store, the real job engine, and a stand-in for the
// one thing no test can reach -- the network card at the far end of a magic
// packet.

const testCluster = model.ClusterID("c1")

// spec is one node as a test describes it.
type spec struct {
	host string
	cp   bool
	off  bool

	// mac is its recorded network card; empty means none was ever read.
	mac string

	// endpoint marks the control-plane node the daemon reaches the cluster
	// through.
	endpoint bool

	disabled bool
}

type rig struct {
	t      *testing.T
	st     store.Store
	svc    *power.Service
	engine *jobs.Engine
	kube   *kubesim.Server
	waker  *fakeWaker
	logs   *syncBuffer

	// deps is what the service was built from, kept so that a test can build
	// a second one over the same store -- which is what a restart is.
	deps power.Deps
	log  *slog.Logger

	sims map[model.MachineID]*talossim.Server
	ids  map[string]model.MachineID
}

func newRig(t *testing.T, specs []spec, kopts kubesim.Options) *rig {
	t.Helper()

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	st, err := fsstore.Open(dir)
	if err != nil {
		t.Fatalf("fsstore.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	r := &rig{
		t:    t,
		st:   st,
		sims: map[model.MachineID]*talossim.Server{},
		ids:  map[string]model.MachineID{},
		logs: &syncBuffer{},
		waker: &fakeWaker{
			byMAC: map[string]*talossim.Server{},
		},
	}

	ctx := t.Context()
	endpoint := ""
	for i, sp := range specs {
		id := model.MachineID(fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1))
		addr := fmt.Sprintf("10.0.0.%d", i+10)
		if sp.endpoint {
			endpoint = addr
		}

		sim, err := talossim.New(talossim.Options{
			Hostname: sp.host, ControlPlane: sp.cp, Bootstrapped: sp.cp,
			// A boot visibly after the reboot that caused it: see BootTakes.
			BootTakes: 2 * time.Second,
		})
		if err != nil {
			t.Fatalf("talossim.New: %v", err)
		}
		t.Cleanup(func() { _ = sim.Close() })
		r.sims[id] = sim
		r.ids[sp.host] = id

		role := model.RoleWorker
		if sp.cp {
			role = model.RoleControlPlane
		}
		rec := model.Machine{
			ID: id, Cluster: testCluster, Hostname: sp.host, Role: role, Addr: addr,
			Disabled: sp.disabled,
		}
		if sp.mac != "" {
			rec.Snapshot.Interfaces = []model.Interface{
				{Name: "eth0", HardwareAddr: sp.mac, Up: true},
				// A bond and a loopback: addresses no firmware listens on,
				// which a wake must leave out.
				{Name: "bond0", HardwareAddr: "02:00:00:00:00:99", Kind: "bond"},
				{Name: "lo", HardwareAddr: "00:00:00:00:00:00"},
			}
			r.waker.byMAC[sp.mac] = sim
		}
		if _, err := st.Machines().Put(ctx, rec); err != nil {
			t.Fatalf("put machine: %v", err)
		}

		kopts.Nodes = append(kopts.Nodes, kubesim.Node{Name: sp.host, InternalAddress: addr})
	}

	if _, err := st.Clusters().Put(ctx, model.Cluster{
		ID: testCluster, Name: "homelab", Endpoint: endpoint,
	}); err != nil {
		t.Fatalf("put cluster: %v", err)
	}

	ksim, err := kubesim.New(kopts)
	if err != nil {
		t.Fatalf("kubesim.New: %v", err)
	}
	t.Cleanup(ksim.Close)
	r.kube = ksim

	logger := slog.New(slog.NewTextHandler(r.logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	r.engine = jobs.New(jobs.Deps{Store: st, Logger: logger})
	t.Cleanup(func() { _ = r.engine.Close() })

	connect := func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
		sim, ok := r.sims[id]
		if !ok {
			return nil, fmt.Errorf("no such simulated machine %s", id)
		}
		return talos.NewClusterClient(ctx, sim.Dialer(), talos.Target{
			Cluster: testCluster, Machine: id, Addr: sim.Host(),
		}, sim.ClientCreds(), talos.Mode{})
	}
	machines := func(ctx context.Context, id model.ClusterID) ([]model.Machine, error) {
		all, err := st.Machines().List(ctx)
		if err != nil {
			return nil, err
		}
		var out []model.Machine
		for _, m := range all {
			if m.Cluster == id {
				out = append(out, m)
			}
		}
		return out, nil
	}
	controlPlanes := func(ctx context.Context, id model.ClusterID) ([]model.Machine, error) {
		all, err := machines(ctx, id)
		if err != nil {
			return nil, err
		}
		var out []model.Machine
		for _, m := range all {
			if m.Role == model.RoleControlPlane {
				out = append(out, m)
			}
		}
		return out, nil
	}

	r.log = logger
	r.deps = power.Deps{
		Store:  st,
		Logger: logger,
		Jobs:   r.engine,
		Connect: func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
			return connect(ctx, id)
		},
		Kube: func(context.Context, model.ClusterID) (*kube.Client, error) {
			caCrt, caKey := ksim.AuthorityPEM()
			creds, err := kube.MintCreds(ksim.Endpoint(), caCrt, caKey, time.Now())
			if err != nil {
				return nil, err
			}
			return kube.New(creds)
		},
		Machines:     machines,
		Gate:         upgrade.NewGate(connect, controlPlanes),
		Waker:        r.waker,
		PollInterval: 20 * time.Millisecond,
		BootBudget:   4 * time.Second,
		KubeBudget:   2 * time.Second,
	}
	r.deps.Jobs = r.engine
	r.svc = power.New(r.deps)
	r.svc.Register(r.engine)

	for _, sp := range specs {
		if sp.off {
			r.powerOff(sp.host)
		}
	}
	return r
}

// restart is the daemon going away and coming back: the running engine is
// closed -- its jobs are interrupted where they stand, exactly as a SIGTERM
// leaves them -- and a new engine and a new service over the same store
// register the jobs and resume them.
func (r *rig) restart() {
	r.t.Helper()
	if err := r.engine.Close(); err != nil {
		r.t.Fatalf("close the engine: %v", err)
	}
	r.engine = jobs.New(jobs.Deps{Store: r.st, Logger: r.log})
	r.t.Cleanup(func() { _ = r.engine.Close() })
	deps := r.deps
	deps.Jobs = r.engine
	r.svc = power.New(deps)
	r.svc.Register(r.engine)
	if err := r.engine.Resume(r.t.Context()); err != nil {
		r.t.Fatalf("Resume: %v", err)
	}
}

// powerOff switches a simulated node off the way pulling its plug would: it
// stops answering, and nothing the product did caused it.
func (r *rig) powerOff(host string) {
	r.t.Helper()
	sim := r.sim(host)
	cc, err := talos.NewClusterClient(r.t.Context(), sim.Dialer(), talos.Target{
		Cluster: testCluster, Machine: r.ids[host], Addr: sim.Host(),
	}, sim.ClientCreds(), talos.Mode{})
	if err != nil {
		r.t.Fatalf("connect to %s: %v", host, err)
	}
	defer cc.Close() //nolint:errcheck // test
	ctx, cancel, err := talos.WithClassDeadline(r.t.Context(), talos.MethodShutdown)
	if err != nil {
		r.t.Fatalf("deadline: %v", err)
	}
	defer cancel()
	if err := cc.ShutdownForced(ctx); err != nil {
		r.t.Fatalf("power %s off: %v", host, err)
	}
}

func (r *rig) sim(host string) *talossim.Server {
	r.t.Helper()
	sim, ok := r.sims[r.ids[host]]
	if !ok {
		r.t.Fatalf("no node %q", host)
	}
	return sim
}

func (r *rig) machine(host string) model.Machine {
	r.t.Helper()
	rec, err := r.st.Machines().Get(r.t.Context(), r.ids[host])
	if err != nil {
		r.t.Fatalf("get %s: %v", host, err)
	}
	return rec
}

// wait follows a job to its end.
func (r *rig) wait(j model.Job) model.Job {
	r.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		got, err := r.engine.Get(r.t.Context(), j.ID)
		if err != nil {
			r.t.Fatalf("get job: %v", err)
		}
		if got.State.Terminal() || got.State == model.JobParked {
			return got
		}
		if time.Now().After(deadline) {
			r.t.Fatalf("job %s never finished; it is %s at step %d", j.ID, got.State, got.Current)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// succeeded fails the test with the job's own account of where it stopped.
func (r *rig) succeeded(j model.Job) model.Job {
	r.t.Helper()
	j = r.wait(j)
	if j.State != model.JobSucceeded {
		r.t.Fatalf("job %s ended %s:\n%s", j.Kind, j.State, describe(j))
	}
	return j
}

func describe(j model.Job) string {
	var b strings.Builder
	for i, s := range j.Steps {
		fmt.Fprintf(&b, "  %d. %-40s %-8s %s\n", i, s.Name, s.State, s.Detail)
	}
	return b.String()
}

func stepNames(j model.Job) []string {
	out := make([]string, 0, len(j.Steps))
	for _, s := range j.Steps {
		out = append(out, s.Name)
	}
	return out
}

// fakeWaker is the network card at the far end: a magic packet for a MAC it
// knows switches that node on.
type fakeWaker struct {
	mu    sync.Mutex
	byMAC map[string]*talossim.Server
	sent  []string

	// hold makes the card at the far end ignore the packet, so that a start
	// waits -- a firmware that takes its time, or one that was never armed.
	hold bool
}

func (w *fakeWaker) Wake(_ context.Context, macs []net.HardwareAddr) (power.WakeReport, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	report := power.WakeReport{Destinations: []string{"255.255.255.255:9"}}
	for _, mac := range macs {
		w.sent = append(w.sent, mac.String())
		report.Sent = append(report.Sent, mac.String()+" to 255.255.255.255:9")
		if sim, ok := w.byMAC[mac.String()]; ok && !w.hold {
			sim.PowerOn()
		}
	}
	return report, nil
}

func (w *fakeWaker) setHold(v bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.hold = v
}

func (w *fakeWaker) woken() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.sent)
}

// syncBuffer is a log sink the engine's goroutines and the test can share.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func statusOf(t *testing.T, rep power.Report, a power.Action) power.ActionStatus {
	t.Helper()
	for _, s := range rep.Actions {
		if s.Action == a {
			return s
		}
	}
	t.Fatalf("the report has no %s", a)
	return power.ActionStatus{}
}
