package support_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/support"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// The support bundle, and the two things it has to get right.
//
// It has to be **sendable**, which is what the entropy walk is about: a bundle
// carrying a cluster's certificate authority is a bundle nobody can hand to
// anybody, and the check has to run over every file rather than over the one
// the author was thinking about.
//
// And a node that does not answer has to be the point rather than an error,
// because the bundle somebody actually needs is the one taken while things are
// broken.

const testCluster = model.ClusterID("c1")

// entry is one file out of a bundle.
type entry struct {
	name string
	body []byte
}

// open unpacks a bundle so a test can walk it.
func open(t *testing.T, raw []byte) []entry {
	t.Helper()

	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}

	var out []entry
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("tar: %v", err)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read %s: %v", hdr.Name, err)
		}
		if perm := hdr.FileInfo().Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("%s is mode %04o inside the archive; group and other must have nothing",
				hdr.Name, perm)
		}
		out = append(out, entry{name: hdr.Name, body: body})
	}
}

func find(entries []entry, suffix string) (entry, bool) {
	for _, e := range entries {
		if strings.HasSuffix(e.name, suffix) {
			return e, true
		}
	}
	return entry{}, false
}

// liveCluster is one simulated control-plane node in a simulated cluster,
// bootstrapped so the etcd reads answer.
func liveCluster(t *testing.T) (*talossim.Cluster, support.Deps) {
	t.Helper()

	cl, err := talossim.NewCluster("homelab", "https://10.0.0.10:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	sim, err := talossim.New(talossim.Options{
		Hostname: "cp-1", Cluster: cl, ControlPlane: true, StreamMessages: 4,
	})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	machine := model.Machine{
		ID:       model.MachineID("00000000-0000-4000-8000-000000000001"),
		Cluster:  testCluster,
		Hostname: "cp-1",
		Role:     model.RoleControlPlane,
		Addr:     sim.Host(),
	}

	connect := func(ctx context.Context, _ model.MachineID) (*talos.ClusterClient, error) {
		return talos.NewClusterClient(ctx, sim.Dialer(), talos.Target{
			Cluster: testCluster, Machine: machine.ID, Addr: sim.Host(),
		}, sim.ClientCreds(), talos.Mode{})
	}

	// Bootstrapped, so etcd answers. A node that was never bootstrapped
	// refuses every Etcd* read, which is a different test.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cc, err := connect(ctx, machine.ID)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	bootCtx, cancelBoot, err := talos.WithClassDeadline(ctx, talos.MethodBootstrap)
	if err != nil {
		t.Fatalf("WithClassDeadline: %v", err)
	}
	if err := cc.Bootstrap(bootCtx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	cancelBoot()
	_ = cc.Close()

	return cl, support.Deps{
		Connect: connect,
		Machines: func(context.Context, model.ClusterID) ([]model.Machine, error) {
			return []model.Machine{machine}, nil
		},
		Clusters: func(context.Context) ([]model.Cluster, error) {
			return []model.Cluster{{ID: testCluster, Name: "homelab", Endpoint: "https://10.0.0.10:6443"}}, nil
		},
		AuditTail: func(context.Context, int) ([]byte, error) {
			return []byte(`{"seq":1,"action":"cluster.import","outcome":"success"}` + "\n"), nil
		},
		Instance: func() map[string]any {
			return map[string]any{"version": "test", "dry_run": false}
		},
	}
}

func collect(t *testing.T, deps support.Deps) ([]entry, support.Manifest) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	var buf bytes.Buffer
	man, err := support.New(deps).Write(ctx, testCluster, &buf)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	return open(t, buf.Bytes()), man
}

// TestABundleCarriesWhatSomebodyDebuggingWouldCollectByHand is criterion 1.
func TestABundleCarriesWhatSomebodyDebuggingWouldCollectByHand(t *testing.T) {
	t.Parallel()

	_, deps := liveCluster(t)
	entries, man := collect(t, deps)

	for _, want := range []string{
		"00-MANIFEST.json",
		"cluster.json",
		"nodes/cp-1/inventory.json",
		"nodes/cp-1/facts.json",
		"nodes/cp-1/version.json",
		"nodes/cp-1/services.json",
		"nodes/cp-1/etcd-members.json",
		"nodes/cp-1/etcd-status.json",
		"nodes/cp-1/machineconfig.yaml",
		"nodes/cp-1/dmesg.log",
		"nodes/cp-1/logs/etcd.log",
		"nodes/cp-1/logs/kubelet.log",
		"audit-tail.jsonl",
	} {
		if _, ok := find(entries, want); !ok {
			var names []string
			for _, e := range entries {
				names = append(names, e.name)
			}
			t.Errorf("the bundle has no %s; it has:\n  %s", want, strings.Join(names, "\n  "))
		}
	}

	if len(man.Nodes) != 1 || man.Nodes[0] != "cp-1" {
		t.Errorf("the manifest names %v as its nodes", man.Nodes)
	}
	if man.Redaction == "" {
		t.Error("the manifest makes no statement about redaction; somebody receiving this bundle " +
			"would have to take it on trust")
	}
}

// TestNoFileInTheBundleCarriesAPrivateKey is criterion 3, and it is the one
// that decides whether this feature can exist at all.
//
// The walk is over **every** file rather than over the configuration, and that
// is the point: the configuration is where a key is expected, so it is the one
// place somebody remembers to check. A key that reached facts.json, or a log
// line, or the audit tail would be missed by a check that only looked where it
// was looking.
func TestNoFileInTheBundleCarriesAPrivateKey(t *testing.T) {
	t.Parallel()

	_, deps := liveCluster(t)
	entries, _ := collect(t, deps)

	if len(entries) < 8 {
		t.Fatalf("the walk saw only %d file(s); a bundle that small means the collection failed "+
			"and this check passed for the wrong reason", len(entries))
	}

	for _, e := range entries {
		if bytes.Contains(e.body, []byte("PRIVATE KEY")) {
			t.Errorf("%s contains a PEM private-key marker", e.name)
		}
		// The base64 body of a key survives even when the armour is stripped,
		// so the marker alone is not enough: a redaction that removed the
		// BEGIN/END lines and left the payload would pass the check above.
		if line, ok := highEntropyLine(e.body); ok {
			t.Errorf("%s contains a %d-character high-entropy line, which is what a "+
				"key body looks like with its armour removed: %.40q…", e.name, len(line), line)
		}
	}
}

// TestTheClustersOwnCertificateAuthorityKeyIsNowhereInTheBundle.
//
// This is the check that matters, and it is stronger than looking for the
// string "PRIVATE KEY": the key it searches for is the one that is genuinely
// in the configuration the simulated node serves, generated by machinery from
// a real secrets bundle. Its absence is evidence rather than the absence of a
// marker -- and it is searched for across **every file**, because a key that
// reached facts.json or a log line would be missed by a check that only looked
// at the configuration.
func TestTheClustersOwnCertificateAuthorityKeyIsNowhereInTheBundle(t *testing.T) {
	t.Parallel()

	cl, deps := liveCluster(t)

	key := bytes.TrimSpace(cl.OSKeyPEM())
	if len(key) == 0 {
		t.Fatal("the fixture has no OS certificate-authority key, so this test would pass " +
			"against a bundle that leaked one")
	}
	// The body without its armour, which is what survives a redaction that
	// stripped the BEGIN/END lines and left the payload.
	body := bytes.Join(bytes.Split(key, []byte("\n"))[1:len(bytes.Split(key, []byte("\n")))-1], nil)

	entries, _ := collect(t, deps)

	cfg, ok := find(entries, "machineconfig.yaml")
	if !ok {
		t.Fatal("the bundle carries no machine configuration at all, so this test would pass " +
			"for the wrong reason")
	}
	if len(cfg.body) < 200 {
		t.Fatalf("the configuration in the bundle is %d bytes; that is not a redacted "+
			"configuration, it is an empty one", len(cfg.body))
	}

	for _, e := range entries {
		if bytes.Contains(e.body, key) {
			t.Errorf("%s contains the cluster's OS certificate-authority key verbatim", e.name)
		}
		if len(body) > 40 && bytes.Contains(e.body, body[:40]) {
			t.Errorf("%s contains the start of that key's base64 body with the armour removed", e.name)
		}
	}
}

// TestANodeThatDoesNotAnswerIsTheWholePoint is criterion 2.
//
// The bundle somebody actually needs is the one taken while things are broken.
// A collection that failed on the first unreachable node would produce nothing
// exactly when it matters.
func TestANodeThatDoesNotAnswerIsTheWholePoint(t *testing.T) {
	t.Parallel()

	_, deps := liveCluster(t)

	// Two nodes now: one answering, one that never will.
	live, err := deps.Machines(t.Context(), testCluster)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}
	dead := model.Machine{
		ID:      model.MachineID("00000000-0000-4000-8000-00000000dead"),
		Cluster: testCluster, Hostname: "cp-2", Role: model.RoleControlPlane,
		Addr: "127.0.0.2",
	}
	deps.Machines = func(context.Context, model.ClusterID) ([]model.Machine, error) {
		return append(append([]model.Machine{}, live...), dead), nil
	}
	connectLive := deps.Connect
	deps.Connect = func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
		if id == dead.ID {
			return nil, errors.New("talos: 00000000-0000-4000-8000-00000000dead did not answer")
		}
		return connectLive(ctx, id)
	}

	entries, man := collect(t, deps)

	// The run finished and the healthy node is complete.
	if _, ok := find(entries, "nodes/cp-1/facts.json"); !ok {
		t.Error("the unreachable node cost the healthy node its facts")
	}

	// The dead node contributes what does not need it to be alive, plus a file
	// that says why the rest is missing.
	if _, ok := find(entries, "nodes/cp-2/inventory.json"); !ok {
		t.Error("the unreachable node contributed nothing at all, not even its stored record")
	}
	unreachable, ok := find(entries, "nodes/cp-2/UNREACHABLE.txt")
	if !ok {
		t.Fatal("nothing in the bundle says why cp-2 is empty")
	}
	if !bytes.Contains(unreachable.body, []byte("did not answer")) {
		t.Errorf("the explanation does not carry the transport's own reason: %q", unreachable.body)
	}

	// And the manifest lists it, so somebody opening the bundle sees it in one
	// file rather than by noticing an absence.
	var named bool
	for _, gap := range man.Incomplete {
		if gap.Node == dead.ID {
			named = true
		}
	}
	if !named {
		t.Errorf("the manifest does not list cp-2 among its gaps: %+v", man.Incomplete)
	}
}

// TestAnEmptyBundleIsImpossible.
//
// Every file that could be empty says something instead. A zero-length log is
// read as "nothing was wrong"; a file saying the stream produced no output is
// read as what it is.
func TestAnEmptyBundleIsImpossible(t *testing.T) {
	t.Parallel()

	_, deps := liveCluster(t)
	entries, _ := collect(t, deps)

	for _, e := range entries {
		if len(bytes.TrimSpace(e.body)) == 0 {
			t.Errorf("%s is empty; an empty file in a bundle is read as 'nothing was wrong'", e.name)
		}
	}
}

// TestALogIsTruncatedAtTheStartAndSaysSo is criterion 5.
//
// The lines that explain a failure are the ones next to it, so a bounded log
// keeps the newest -- and it says how much is missing, because a log that is
// quietly shorter than it looks is a log somebody draws conclusions from.
func TestALogIsTruncatedAtTheStartAndSaysSo(t *testing.T) {
	t.Parallel()

	_, deps := liveCluster(t)

	// Enough output to exceed the cap several times over.
	sim, err := talossim.New(talossim.Options{Hostname: "loud", StreamMessages: 12, StreamChunk: 64 << 10})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	machine := model.Machine{
		ID:      model.MachineID("00000000-0000-4000-8000-000000000009"),
		Cluster: testCluster, Hostname: "loud", Addr: sim.Host(),
	}
	deps.Machines = func(context.Context, model.ClusterID) ([]model.Machine, error) {
		return []model.Machine{machine}, nil
	}
	deps.Connect = func(ctx context.Context, _ model.MachineID) (*talos.ClusterClient, error) {
		return talos.NewClusterClient(ctx, sim.Dialer(), talos.Target{
			Cluster: testCluster, Machine: machine.ID, Addr: sim.Host(),
		}, sim.ClientCreds(), talos.Mode{})
	}

	entries, _ := collect(t, deps)

	log, ok := find(entries, "nodes/loud/dmesg.log")
	if !ok {
		t.Fatal("the bundle has no dmesg")
	}
	if len(log.body) > support.MaxLogBytes+4096 {
		t.Errorf("the log is %d bytes against a cap of %d", len(log.body), support.MaxLogBytes)
	}
	if !bytes.Contains(log.body, []byte("were dropped")) {
		t.Errorf("a truncated log does not say how much is missing: %.200q", log.body)
	}
}

// TestANodesOwnHostnameCannotEscapeTheArchive.
//
// The hostname comes off the node and is therefore the node's to choose. A
// hostname of `../../etc` would be a traversal inside an archive somebody
// unpacks -- the same class of problem the restore path refuses, and here it
// is cheaper to make the name safe than to drop the node's contribution.
func TestANodesOwnHostnameCannotEscapeTheArchive(t *testing.T) {
	t.Parallel()

	_, deps := liveCluster(t)
	live, _ := deps.Machines(t.Context(), testCluster)
	live[0].Hostname = "../../../etc/cron.d/evil"
	deps.Machines = func(context.Context, model.ClusterID) ([]model.Machine, error) { return live, nil }

	entries, _ := collect(t, deps)

	// The property is not "the name contains no dots" -- a hostname may
	// legitimately be `node.example.com`, and the sanitiser keeps dots for
	// exactly that reason. What matters is that no path *component* is `.` or
	// `..`, because those are the only two that traverse, and that nothing
	// leaves the root.
	root := ""
	for _, e := range entries {
		if root == "" {
			root = strings.SplitN(e.name, "/", 2)[0]
		}
		if !strings.HasPrefix(e.name, root+"/") && e.name != root {
			t.Errorf("%s is outside the archive root %q", e.name, root)
		}
		for _, part := range strings.Split(e.name, "/") {
			if part == "." || part == ".." {
				t.Errorf("%s has a traversing path component", e.name)
			}
		}
	}

	// And the node still contributed: making the name safe is cheaper than
	// dropping a node's evidence from the bundle somebody is going to read.
	if _, ok := find(entries, "inventory.json"); !ok {
		t.Error("a node with an awkward hostname was left out of the bundle entirely")
	}
}

// highEntropyLine reports a long line whose byte distribution looks like
// base64-encoded key material rather than like text.
//
// The threshold is deliberately loose: this is a smoke detector, not a
// classifier. What it has to catch is the specific shape of a PEM body that
// somebody stripped the armour from, and what it must not do is fire on a
// certificate -- which is why certificates are excluded by their own marker
// before this runs.
func highEntropyLine(body []byte) ([]byte, bool) {
	// A bundle legitimately carries certificates, which are base64 and are not
	// secret. Their armour is what tells them apart from a naked key body.
	if bytes.Contains(body, []byte("BEGIN CERTIFICATE")) {
		body = stripPEMBlocks(body)
	}

	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) < 60 {
			continue
		}
		if _, err := base64.StdEncoding.DecodeString(string(line)); err != nil {
			continue
		}
		if shannon(line) > 4.5 {
			return line, true
		}
	}
	return nil, false
}

// stripPEMBlocks removes armoured blocks, which are the legitimate base64 in a
// bundle.
func stripPEMBlocks(body []byte) []byte {
	var out [][]byte
	inBlock := false
	for _, line := range bytes.Split(body, []byte("\n")) {
		switch {
		case bytes.Contains(line, []byte("-----BEGIN")):
			inBlock = true
		case bytes.Contains(line, []byte("-----END")):
			inBlock = false
		case !inBlock:
			out = append(out, line)
		}
	}
	return bytes.Join(out, []byte("\n"))
}

func shannon(b []byte) float64 {
	var counts [256]int
	for _, c := range b {
		counts[c]++
	}
	var h float64
	n := float64(len(b))
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / n
		h -= p * math.Log2(p)
	}
	return h
}
