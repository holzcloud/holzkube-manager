// Package support writes a support bundle: one archive with everything
// somebody debugging this cluster would otherwise collect by hand.
//
// The shape of the problem it solves is the reason it exists. An operator with
// a broken cluster gathers node facts, service lists, logs, dmesg, etcd status
// and the machine configuration across several screens while under pressure,
// and then cannot hand the result to anybody because it is six browser tabs.
// `talosctl support` does exactly this for Talos; this product had every
// building block and no button.
//
// # Two properties shape everything here
//
// **It must be sendable.** A bundle is by definition an archive of everything,
// and everything includes secrets: a machine configuration carries the
// cluster's certificate-authority private key. So every configuration in the
// bundle goes through internal/machineconfig's redaction -- the same two passes
// the config screen uses -- and the acceptance test walks entropy over *every
// file in the archive* rather than over the one the author was thinking about.
// A bundle you cannot send is not a support bundle.
//
// **A node that does not answer is the point, not an error.** The bundle a
// person actually needs is the one taken while things are broken. So an
// unreachable node produces a file that says which node and why, the run
// continues, and nothing anywhere is silently empty.
package support

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// MaxLogBytes is how much of each log stream a bundle carries.
//
// Logs are the largest thing in a bundle by an order of magnitude and the part
// whose value falls off fastest with age: the lines that explain a failure are
// the ones next to it. Truncation is therefore at the *start* -- the newest
// lines are kept -- and the file says how much was dropped, because a log that
// is quietly shorter than it looks is a log somebody draws conclusions from.
const MaxLogBytes = 256 << 10

// PerNodeTimeout bounds one node's collection.
//
// It is per node and not per bundle, so one node that is answering slowly
// cannot eat the budget of the nodes after it. A bundle over a cluster where
// every node is slow takes a while and finishes; that is the right trade for
// something somebody runs once, during an incident.
const PerNodeTimeout = 90 * time.Second

// Deps is what a collector needs.
type Deps struct {
	// Connect opens a client to one machine. It is the inventory's Connect.
	Connect func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error)

	// Machines lists a cluster's machines, and Clusters lists the clusters.
	Machines func(ctx context.Context, id model.ClusterID) ([]model.Machine, error)
	Clusters func(ctx context.Context) ([]model.Cluster, error)

	// AuditTail returns the last n audit records, already redacted by the
	// audit package's own allowlist. It is a function rather than a reader
	// because the redaction belongs to that package and a bundle that read the
	// files directly would be a second path past the allowlist.
	AuditTail func(ctx context.Context, n int) ([]byte, error)

	// Instance describes this holzkube-manager: version, dry-run, the
	// supported Talos range. It is what turns "the config looked fine" into
	// "the config looked fine to this build".
	Instance func() map[string]any
}

// AuditTailRecords is how many audit records a bundle carries.
//
// Enough to cover the operations that led to the incident, not the archive:
// the archive is kept forever on the host and is not a support artefact. The
// records are already redacted by the audit allowlist before they reach here.
const AuditTailRecords = 500

// Collector writes bundles.
type Collector struct{ deps Deps }

// New builds one.
func New(d Deps) *Collector { return &Collector{deps: d} }

// Manifest is the bundle's own account of itself.
//
// It is written first and read first, and it carries the failures as data
// rather than leaving them to be noticed: somebody opening a bundle should be
// able to see in one file which nodes are missing from it and why.
type Manifest struct {
	CreatedAt time.Time      `json:"created_at"`
	Instance  map[string]any `json:"instance"`

	// Clusters and Nodes are what was collected.
	Clusters []string `json:"clusters"`
	Nodes    []string `json:"nodes"`

	// Incomplete names every read that did not produce what it was asked for,
	// with the reason. A bundle with an empty list is a bundle taken from a
	// healthy cluster, which is worth being able to tell apart from a bundle
	// whose collection quietly failed.
	Incomplete []Gap `json:"incomplete"`

	// Redaction says what was removed and by what. It is in the manifest
	// because the person receiving a bundle has to know it was redacted
	// without taking anybody's word for it.
	Redaction string `json:"redaction"`
}

// Gap is one thing a bundle does not have.
type Gap struct {
	Node   model.MachineID `json:"node,omitempty"`
	What   string          `json:"what"`
	Reason string          `json:"reason"`
}

// RedactionNotice is the manifest's statement about what was removed.
const RedactionNotice = "Every machine configuration in this bundle was passed through " +
	"holzkube-manager's redaction before it was written: machinery's own RedactSecrets, which " +
	"knows the schema this build was compiled against, and then a sweep for PEM private-key " +
	"blocks, which knows no schema and catches a key in a field this build has never seen. " +
	"Audit records were redacted by the audit allowlist, which is fail-closed: a parameter not " +
	"explicitly permitted is written as <redacted>. Nothing here has been checked by hand, and " +
	"the entropy walk that checks it runs over every file in this archive rather than over the " +
	"ones somebody thought of."

// Write collects a bundle for one cluster and writes it as a gzipped tar.
//
// It returns the manifest, so a caller can report what was missing without
// unpacking what it just wrote.
func (c *Collector) Write(ctx context.Context, cluster model.ClusterID, w io.Writer) (Manifest, error) {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	man := Manifest{
		CreatedAt:  time.Now().UTC(),
		Redaction:  RedactionNotice,
		Incomplete: []Gap{},
		Clusters:   []string{},
		Nodes:      []string{},
	}
	if c.deps.Instance != nil {
		man.Instance = c.deps.Instance()
	}

	root := "holzkube-manager-support-" + man.CreatedAt.Format("20060102T150405Z")

	machines, err := c.deps.Machines(ctx, cluster)
	if err != nil {
		// The inventory is the one read whose failure ends the run: without it
		// there is no list of nodes to be incomplete about.
		return man, fmt.Errorf("support: read the inventory: %w", err)
	}
	man.Clusters = append(man.Clusters, string(cluster))

	// The cluster record, without its secrets. ClusterSecrets is a separate
	// entity precisely so that something serialising a cluster cannot reach
	// them (D-10), and this is one of the places that separation pays.
	if c.deps.Clusters != nil {
		if clusters, cerr := c.deps.Clusters(ctx); cerr == nil {
			for _, cl := range clusters {
				if cl.ID != cluster {
					continue
				}
				writeJSON(tw, path.Join(root, "cluster.json"), cl, &man)
			}
		} else {
			man.Incomplete = append(man.Incomplete, Gap{
				What: "cluster.json", Reason: cerr.Error(),
			})
		}
	}

	sort.Slice(machines, func(i, j int) bool { return nameOf(machines[i]) < nameOf(machines[j]) })

	for _, m := range machines {
		man.Nodes = append(man.Nodes, nameOf(m))
		c.collectNode(ctx, tw, path.Join(root, "nodes", nameOf(m)), m, &man)
	}

	if c.deps.AuditTail != nil {
		if tail, aerr := c.deps.AuditTail(ctx, AuditTailRecords); aerr == nil {
			writeFile(tw, path.Join(root, "audit-tail.jsonl"), tail, &man)
		} else {
			man.Incomplete = append(man.Incomplete, Gap{
				What: "audit-tail.jsonl", Reason: aerr.Error(),
			})
		}
	}

	// The manifest last, because it names the gaps every read above produced.
	// Written under a name that sorts first so that `tar t` shows it at the
	// top: the person opening this has to find it without being told to.
	writeJSON(tw, path.Join(root, "00-MANIFEST.json"), man, &man)

	if err := tw.Close(); err != nil {
		return man, fmt.Errorf("support: close tar: %w", err)
	}
	if err := gz.Close(); err != nil {
		return man, fmt.Errorf("support: close gzip: %w", err)
	}
	return man, nil
}

// collectNode gathers one node, and never fails the bundle.
//
// Every read is independent and every failure becomes a Gap. A node that is
// down contributes its inventory record and a list of what could not be asked,
// which is exactly the bundle somebody wants when a node is down.
func (c *Collector) collectNode(
	ctx context.Context,
	tw *tar.Writer,
	dir string,
	m model.Machine,
	man *Manifest,
) {
	// The stored record first, because it is the one thing that does not need
	// the node to be alive -- and on a dead node it is the whole file.
	writeJSON(tw, path.Join(dir, "inventory.json"), m, man)

	nodeCtx, cancel := context.WithTimeout(ctx, PerNodeTimeout)
	defer cancel()

	cc, err := c.deps.Connect(nodeCtx, m.ID)
	if err != nil {
		man.Incomplete = append(man.Incomplete, Gap{
			Node: m.ID,
			What: "everything the node itself would report",
			// The transport's own sentence, which already names the machine
			// and never the address, and already distinguishes "nothing
			// answered" from "answered and refused".
			Reason: err.Error(),
		})
		writeFile(tw, path.Join(dir, "UNREACHABLE.txt"), []byte(
			"This node did not answer when the bundle was taken.\n\n"+
				err.Error()+"\n\n"+
				"Everything in this directory other than inventory.json is therefore missing, and\n"+
				"that is a fact about the cluster rather than about the bundle. 00-MANIFEST.json\n"+
				"lists it.\n"), man)
		return
	}
	defer cc.Close() //nolint:errcheck // a bundle's verdict is not a close error's

	// Each read is its own call with its own class deadline, and each failure
	// is its own gap. One subsystem being down -- etcd is the usual one -- must
	// not cost the reads that would explain why.
	c.read(nodeCtx, tw, dir, m, man, "facts.json", talos.MethodCOSIList, func(ctx context.Context) (any, error) {
		return cc.NodeFacts(ctx)
	})
	c.read(nodeCtx, tw, dir, m, man, "version.json", talos.MethodVersion, func(ctx context.Context) (any, error) {
		v, err := cc.Version(ctx)
		return map[string]string{"talos_version": v}, err
	})
	c.read(nodeCtx, tw, dir, m, man, "services.json", talos.MethodServiceList, func(ctx context.Context) (any, error) {
		return cc.ServiceList(ctx)
	})
	c.read(nodeCtx, tw, dir, m, man, "etcd-members.json", talos.MethodEtcdMemberList, func(ctx context.Context) (any, error) {
		return cc.EtcdMembers(ctx)
	})
	c.read(nodeCtx, tw, dir, m, man, "etcd-status.json", talos.MethodEtcdStatus, func(ctx context.Context) (any, error) {
		return cc.EtcdStatus(ctx)
	})
	c.read(nodeCtx, tw, dir, m, man, "etcd-alarms.json", talos.MethodEtcdAlarmList, func(ctx context.Context) (any, error) {
		return cc.EtcdAlarmList(ctx)
	})

	// The configuration, and the one read in this function that must not reach
	// the archive as it came off the node.
	c.collectConfig(nodeCtx, tw, dir, cc, m, man)

	// dmesg, which is where a node explains a disk or a network card. It is
	// bounded by MaxLogBytes like every stream here.
	c.collectStream(nodeCtx, tw, path.Join(dir, "dmesg.log"), m, man, func(ctx context.Context) (*talos.LogStream, error) {
		return cc.Dmesg(ctx, true)
	})

	// The services worth having logs for. Named rather than derived from the
	// service list, because a bundle that fetched every service's log on every
	// node would be an archive nobody opens -- and these four are the ones a
	// Talos failure is in.
	for _, svc := range []string{"etcd", "kubelet", "apid", "machined"} {
		service := svc
		c.collectStream(nodeCtx, tw, path.Join(dir, "logs", service+".log"), m, man,
			func(ctx context.Context) (*talos.LogStream, error) { return cc.Logs(ctx, service) })
	}
}

// collectConfig writes the node's machine configuration, redacted.
//
// The redaction is not optional and it is not conditional: an unparsable
// configuration still gets the PEM sweep and is written *with* the error
// beside it, because refusing it would hide the problem and writing it raw
// would hand over the cluster's certificate authority.
func (c *Collector) collectConfig(
	ctx context.Context,
	tw *tar.Writer,
	dir string,
	cc *talos.ClusterClient,
	m model.Machine,
	man *Manifest,
) {
	callCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodCOSIGet)
	if err != nil {
		man.Incomplete = append(man.Incomplete, Gap{Node: m.ID, What: "machineconfig.yaml", Reason: err.Error()})
		return
	}
	defer cancel()

	raw, err := cc.MachineConfigYAML(callCtx)
	if err != nil {
		man.Incomplete = append(man.Incomplete, Gap{Node: m.ID, What: "machineconfig.yaml", Reason: err.Error()})
		return
	}

	redacted, rerr := machineconfig.Redact(raw)
	if rerr != nil {
		// Redact returns the swept bytes alongside its error, so the
		// configuration is still written -- it is the sweep that matters here,
		// and the error is recorded as a gap so nobody reads the file as
		// authoritative.
		man.Incomplete = append(man.Incomplete, Gap{
			Node: m.ID, What: "machineconfig.yaml is the PEM sweep only",
			Reason: rerr.Error(),
		})
	}

	// The last line of defence, and it is a refusal rather than a warning: if
	// a private key is still in these bytes after both passes, the bundle does
	// not carry the file at all. A bundle that leaks a certificate authority
	// is worse than a bundle that is missing a configuration.
	if machineconfig.ContainsPrivateKey(redacted) {
		man.Incomplete = append(man.Incomplete, Gap{
			Node: m.ID, What: "machineconfig.yaml",
			Reason: "redaction left a PEM private key in the bytes, so the file was not written. " +
				"This is a defect in holzkube-manager and worth reporting; the bundle is still " +
				"usable without it.",
		})
		return
	}

	writeFile(tw, path.Join(dir, "machineconfig.yaml"), redacted, man)
}

// collectStream writes a bounded tail of a log stream.
func (c *Collector) collectStream(
	ctx context.Context,
	tw *tar.Writer,
	name string,
	m model.Machine,
	man *Manifest,
	open func(context.Context) (*talos.LogStream, error),
) {
	stream, err := open(ctx)
	if err != nil {
		man.Incomplete = append(man.Incomplete, Gap{Node: m.ID, What: path.Base(name), Reason: err.Error()})
		return
	}
	defer stream.Close() //nolint:errcheck // the bytes collected are the verdict

	// A ring, because the lines that explain a failure are the ones next to
	// it. Reading the head and stopping would keep a node's boot messages and
	// throw away the crash.
	var (
		buf     []byte
		dropped int
	)
	for {
		chunk, rerr := stream.Recv()
		if rerr != nil {
			if !errors.Is(rerr, io.EOF) && len(buf) == 0 {
				man.Incomplete = append(man.Incomplete, Gap{
					Node: m.ID, What: path.Base(name), Reason: rerr.Error(),
				})
				return
			}
			break
		}
		buf = append(buf, chunk...)
		if len(buf) > MaxLogBytes {
			cut := len(buf) - MaxLogBytes
			// Cut on a line boundary so the first surviving line is a whole
			// one; a half line at the top of a log is a line somebody
			// misreads.
			if i := indexByteFrom(buf, '\n', cut); i >= 0 {
				cut = i + 1
			}
			dropped += cut
			buf = buf[cut:]
		}
	}

	if dropped > 0 {
		header := fmt.Sprintf(
			"[holzkube-manager: %d earlier byte(s) of this stream were dropped. This file holds "+
				"the most recent %d KiB, because the lines that explain a failure are the ones "+
				"next to it.]\n", dropped, MaxLogBytes>>10)
		buf = append([]byte(header), buf...)
	}
	if len(buf) == 0 {
		// An empty stream is a fact and gets a file that says so, rather than a
		// zero-length file somebody reads as "nothing was wrong".
		buf = []byte("[holzkube-manager: this stream produced no output.]\n")
	}

	writeFile(tw, name, buf, man)
}

// read performs one typed read, writes it as JSON, and records a gap if it
// failed.
func (c *Collector) read(
	ctx context.Context,
	tw *tar.Writer,
	dir string,
	m model.Machine,
	man *Manifest,
	name, method string,
	call func(context.Context) (any, error),
) {
	callCtx, cancel, err := talos.WithClassDeadline(ctx, method)
	if err != nil {
		man.Incomplete = append(man.Incomplete, Gap{Node: m.ID, What: name, Reason: err.Error()})
		return
	}
	defer cancel()

	value, err := call(callCtx)
	if err != nil {
		man.Incomplete = append(man.Incomplete, Gap{Node: m.ID, What: name, Reason: err.Error()})
		return
	}
	writeJSON(tw, path.Join(dir, name), value, man)
}

// writeJSON writes a value as indented JSON.
//
// Indented on purpose: a bundle is read by a person, often in an editor with no
// JSON formatter, and the bytes saved by compacting it are nothing next to a
// 256 KiB log.
func writeJSON(tw *tar.Writer, name string, value any, man *Manifest) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		man.Incomplete = append(man.Incomplete, Gap{What: name, Reason: err.Error()})
		return
	}
	writeFile(tw, name, append(raw, '\n'), man)
}

func writeFile(tw *tar.Writer, name string, body []byte, man *Manifest) {
	hdr := &tar.Header{
		Name: name,
		// 0600 inside an archive whose contents are the operator's business
		// and nobody else's, matching everything else this product writes.
		Mode:     0o600,
		Size:     int64(len(body)),
		Typeflag: tar.TypeReg,
		ModTime:  time.Now().UTC(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		man.Incomplete = append(man.Incomplete, Gap{What: name, Reason: err.Error()})
		return
	}
	if _, err := tw.Write(body); err != nil {
		man.Incomplete = append(man.Incomplete, Gap{What: name, Reason: err.Error()})
	}
}

func nameOf(m model.Machine) string {
	if m.Hostname != "" {
		return sanitise(m.Hostname)
	}
	return sanitise(string(m.ID))
}

// sanitise makes a node's own name safe as a path component.
//
// The hostname comes off a node and is therefore the node's to choose. A
// hostname of `../../etc` would be a directory traversal inside an archive
// somebody unpacks, which is the same class of problem the restore path
// refuses -- and here it is cheaper to make the name safe than to refuse a
// node's contribution to a bundle.
func sanitise(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), ".")
	if out == "" {
		return "unnamed"
	}
	return out
}

func indexByteFrom(b []byte, sep byte, from int) int {
	if from >= len(b) {
		return -1
	}
	if i := indexByte(b[from:], sep); i >= 0 {
		return from + i
	}
	return -1
}

func indexByte(b []byte, sep byte) int {
	for i := range b {
		if b[i] == sep {
			return i
		}
	}
	return -1
}
