package configdrift_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/configdrift"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// node is one real node (the simulator), a real store on a real disk, and the
// real reader of machineconfig in between. The tests of the later plans of this
// phase build on it, which is why the names are what they are.
type node struct {
	cl      *talossim.Cluster
	sim     *talossim.Server
	dir     string
	st      *fsstore.Store
	machine model.Machine
	mc      *machineconfig.Service
}

func newNode(t *testing.T) *node {
	t.Helper()

	cl, err := talossim.NewCluster("homelab", "https://192.168.1.10:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	sim, err := talossim.New(talossim.Options{Hostname: "cp-1", Cluster: cl, ControlPlane: true})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	n := &node{
		cl:  cl,
		sim: sim,
		dir: t.TempDir(),
		machine: model.Machine{
			ID: "m1", Cluster: "c1", Hostname: "cp-1", Role: model.RoleControlPlane,
		},
	}
	if err := os.Chmod(n.dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	n.openStore(t)
	n.mc = machineconfig.New(machineconfig.Deps{Connect: n.connect})
	return n
}

func (n *node) openStore(t *testing.T) {
	t.Helper()
	st, err := fsstore.Open(n.dir)
	if err != nil {
		t.Fatalf("fsstore.Open: %v", err)
	}
	n.st = st
	t.Cleanup(func() { _ = st.Close() })
}

// reopen is a restart of the daemon: the store is closed and opened again on
// the same directory.
func (n *node) reopen(t *testing.T) {
	t.Helper()
	if err := n.st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	n.openStore(t)
}

func (n *node) connect(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
	return talos.NewClusterClient(ctx, n.sim.Dialer(), talos.Target{
		Cluster: "c1", Machine: id, Addr: n.sim.Host(),
	}, n.sim.ClientCreds(), talos.Mode{})
}

// baselinePath is the file of the node's baseline in the data directory.
func (n *node) baselinePath() string {
	return filepath.Join(n.dir, "config-baselines", string(n.machine.ID)+".json")
}

// clock is a time the test moves.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock { return &clock{t: time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// logSink collects what the service logs.
type logSink struct {
	mu    sync.Mutex
	lines []string
}

func (l *logSink) Log(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, strings.TrimSpace(msg+" "+fmt.Sprint(args...)))
}

func (l *logSink) Lines() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

// serviceOpts shapes a service built by (*node).service.
type serviceOpts struct {
	read    func(context.Context, model.MachineID) (machineconfig.Snap, error)
	clock   *clock
	log     *logSink
	cluster model.Cluster
	machine *model.Machine
	budget  time.Duration
}

type serviceOpt func(*serviceOpts)

func withRead(f func(context.Context, model.MachineID) (machineconfig.Snap, error)) serviceOpt {
	return func(o *serviceOpts) { o.read = f }
}

func withClock(c *clock) serviceOpt { return func(o *serviceOpts) { o.clock = c } }

func withLog(l *logSink) serviceOpt { return func(o *serviceOpts) { o.log = l } }

func withCluster(c model.Cluster) serviceOpt { return func(o *serviceOpts) { o.cluster = c } }

func withMachine(m model.Machine) serviceOpt { return func(o *serviceOpts) { o.machine = &m } }

func withBudget(d time.Duration) serviceOpt { return func(o *serviceOpts) { o.budget = d } }

// service builds the configdrift service over the node: the real reader unless
// withRead says otherwise, the real store, the one machine in one cluster.
func (n *node) service(t *testing.T, opts ...serviceOpt) *configdrift.Service {
	t.Helper()

	o := serviceOpts{cluster: model.Cluster{ID: "c1", Name: "homelab"}}
	for _, f := range opts {
		f(&o)
	}
	machine := n.machine
	if o.machine != nil {
		machine = *o.machine
	}
	read := n.mc.ReadSnapshot
	if o.read != nil {
		read = o.read
	}
	d := configdrift.Deps{
		Clusters:    func(context.Context) ([]model.Cluster, error) { return []model.Cluster{o.cluster}, nil },
		Machines:    func(context.Context, model.ClusterID) ([]model.Machine, error) { return []model.Machine{machine}, nil },
		AllMachines: func(context.Context) ([]model.Machine, error) { return []model.Machine{machine}, nil },
		Read:        read,
		Baselines:   n.st.ConfigBaselines(),
		ReadBudget:  o.budget,
	}
	if o.clock != nil {
		d.Now = o.clock.Now
	}
	if o.log != nil {
		d.Log = o.log.Log
	}
	return configdrift.New(d)
}

// thirdPartyChange is how somebody else changes the node: talosctl, another
// operator. A second client reads the active configuration, patches it and
// applies it with no reboot, under a context that has a deadline.
func (n *node) thirdPartyChange(t *testing.T, patch string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cc, err := n.connect(ctx, n.machine.ID)
	if err != nil {
		t.Fatalf("a third party cannot connect: %v", err)
	}
	defer func() { _ = cc.Close() }()

	active, err := cc.MachineConfigYAML(ctx)
	if err != nil {
		t.Fatalf("a third party cannot read the active configuration: %v", err)
	}
	patched, err := machineconfig.ApplyPatches(active, []string{patch})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	if _, err := cc.ApplyConfigurationWithMode(ctx, patched, "no-reboot"); err != nil {
		t.Fatalf("ApplyConfigurationWithMode: %v", err)
	}
}

// secrets is every value of the generated configuration that must never reach
// the baseline file. The raw configuration is checked to hold at least the
// token and secret values in the clear, so their absence from a file says
// something.
func (n *node) secrets(t *testing.T) map[string]string {
	t.Helper()

	s := n.cl.Secrets
	out := map[string]string{
		"the Talos CA private key":      string(s.Certs.OS.Key),
		"the Kubernetes CA private key": string(s.Certs.K8s.Key),
		"the etcd CA private key":       string(s.Certs.Etcd.Key),
		"the aggregator CA private key": string(s.Certs.K8sAggregator.Key),
		"the service account key":       string(s.Certs.K8sServiceAccount.Key),
		"the cluster secret":            s.Cluster.Secret,
		"the bootstrap token":           s.Secrets.BootstrapToken,
		"the machine (trustd) token":    s.TrustdInfo.Token,
	}
	for name, v := range out {
		if strings.TrimSpace(v) == "" {
			t.Fatalf("%s is empty in the fixture, so a search for it proves nothing", name)
		}
	}
	for _, name := range []string{"the cluster secret", "the bootstrap token", "the machine (trustd) token"} {
		if !strings.Contains(string(n.cl.ControlPlaneConfig), out[name]) {
			t.Fatalf("the raw configuration does not hold %s in the clear, so its absence proves nothing", name)
		}
	}
	return out
}

// forms is a secret as it can stand in a file: as it is and base64 encoded.
func forms(secret string) []string {
	return []string{secret, base64.StdEncoding.EncodeToString([]byte(secret))}
}

// fakeReader is a Read with a fixed answer that records what it was handed.
type fakeReader struct {
	mu        sync.Mutex
	snap      machineconfig.Snap
	err       error
	calls     int
	hadLimit  bool
	remaining time.Duration
}

func (f *fakeReader) set(snap machineconfig.Snap, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snap, f.err = snap, err
}

func (f *fakeReader) Read(ctx context.Context, _ model.MachineID) (machineconfig.Snap, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	d, ok := ctx.Deadline()
	f.hadLimit = ok
	f.remaining = time.Until(d)
	return f.snap, f.err
}

// snapOf makes the snapshot of a raw configuration, as the real reader would.
func snapOf(t *testing.T, raw []byte) machineconfig.Snap {
	t.Helper()
	s, err := machineconfig.Snapshot(raw)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	return s
}
