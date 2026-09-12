// Package metrics is holzkube-manager's Prometheus export (V2-API-02).
//
// The boundary is the one the backlog drew and it is not negotiable here:
// **export, never ingest**. This product does not become a monitoring pipeline,
// does not keep a time series, and does not alert. It publishes the handful of
// numbers it is the only thing that knows -- how many nodes are in each stage,
// how long a cluster's client certificate has left, how many jobs ended which
// way, how many etcd votes there are, and whether the audit chain still holds --
// and somebody else's Prometheus does the rest.
//
// Three decisions shape the output.
//
// **No node ever becomes a label value.** A per-node label multiplies every
// series by the fleet, and a fleet is the axis that grows; what an operator
// actually graphs is "how many nodes are down", not "is node
// 4c4c4544-0043-4a10 down", and the second question is what the dashboard is
// for. The cardinality of everything here is a function of the number of
// clusters, which for this product is one or a few.
//
// **A series that drops to zero is still written.** Prometheus has no way to
// tell a series that stopped being reported from one whose target went away,
// and a graph of "nodes down" that simply stops when the number reaches zero
// shows the last non-zero value for as long as anybody is looking. So every
// stage is emitted for every cluster, zeros included.
//
// **Nothing here is a gauge standing in for a time series.** There is no
// last_job_duration and no last_scrape_seconds: a single number overwritten on
// each scrape answers "what was it the last time somebody looked", which is the
// question Prometheus exists to stop people asking.
package metrics

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/health"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// ContentType is the Prometheus text exposition format's media type.
//
// The version parameter is part of it rather than decoration: a scraper reads
// it to decide how to parse, and text/plain alone would make it guess.
const ContentType = "text/plain; version=0.0.4; charset=utf-8"

// Deps is what an export needs.
//
// They are functions rather than the services themselves because this package
// has no business holding an inventory: what it needs is three lists and two
// facts, and taking them as functions is what lets a test about cardinality
// build a fleet of two hundred nodes without a store.
type Deps struct {
	Machines func(ctx context.Context) ([]inventory.MachineView, error)
	Clusters func(ctx context.Context) ([]inventory.ClusterView, error)
	Jobs     func(ctx context.Context) ([]model.Job, error)

	// AuditChainIntact is the startup verification verdict (D-15). It is a
	// snapshot of what was found at startup, which is the honest thing to
	// export: a break found then must not stop being reported because a later
	// re-check happened to look at a different file.
	AuditChainIntact func() bool

	// Now is the clock, so that the certificate metric is testable.
	Now func() time.Time
}

// Exporter renders one scrape.
type Exporter struct{ deps Deps }

// New builds an exporter.
func New(d Deps) *Exporter {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Exporter{deps: d}
}

// unassigned is the label value for machines that belong to no cluster.
//
// It is the empty string rather than a word like "none", because a word is a
// cluster id somebody could one day have, and a series that silently merges a
// real cluster with the orphans is worse than an ugly label.
const unassigned = ""

// Write renders the whole exposition.
//
// It never fails partway with half a body written: every read happens first,
// and only then is anything emitted. A scrape that returns a truncated
// exposition is a scrape Prometheus rejects wholesale, so a failure after the
// first byte would lose the metrics that had already been gathered as well as
// the ones that had not.
func (e *Exporter) Write(ctx context.Context, w io.Writer) error {
	machines, err := e.deps.Machines(ctx)
	if err != nil {
		return fmt.Errorf("metrics: read the machines: %w", err)
	}
	clusters, err := e.deps.Clusters(ctx)
	if err != nil {
		return fmt.Errorf("metrics: read the clusters: %w", err)
	}
	jobs, err := e.deps.Jobs(ctx)
	if err != nil {
		return fmt.Errorf("metrics: read the jobs: %w", err)
	}

	var b strings.Builder

	e.writeClusters(&b, clusters)
	e.writeMachines(&b, clusters, machines)
	e.writeJobs(&b, jobs)
	e.writeAudit(&b)

	_, err = io.WriteString(w, b.String())
	return err
}

// writeClusters emits the per-cluster facts that come from the cluster record
// rather than from its nodes.
func (e *Exporter) writeClusters(b *strings.Builder, clusters []inventory.ClusterView) {
	// The info metric. It is the standard way to publish a name without
	// putting it on every other series: the value is always 1 and the labels
	// carry the description, so a query joins against it when it wants the
	// human name and ignores it when it does not. Without this the choice
	// would be between a cluster id nobody recognises and a name repeated on
	// every series here.
	help(b, "holzkube_cluster_info", "gauge",
		"Always 1. Carries a cluster's human name and origin so that no other series has to.")
	for _, c := range clusters {
		metric(b, "holzkube_cluster_info", labels{
			{"cluster", string(c.ID)},
			{"name", c.Name},
			{"origin", string(c.Origin)},
		}, 1)
	}

	// Seconds, not days, and signed. A negative value is an expired
	// certificate, which is a named state and not a missing metric: the
	// failure it describes takes every node in the cluster down in the same
	// second, so a series that vanished at expiry would go blank at exactly
	// the moment somebody needed it. Seconds rather than the read model's
	// whole days because an alert written against a threshold of hours is a
	// reasonable thing to want on the last day.
	help(b, "holzkube_cluster_client_certificate_seconds", "gauge",
		"Seconds until this cluster's client certificate expires. Negative means it already has, "+
			"which takes every node in the cluster down at once.")
	now := e.deps.Now()
	for _, c := range clusters {
		metric(b, "holzkube_cluster_client_certificate_seconds",
			labels{{"cluster", string(c.ID)}},
			c.ClientCertNotAfter.Sub(now).Seconds())
	}

	help(b, "holzkube_cluster_locked", "gauge",
		"1 when this cluster refuses mutation because it was adopted read-only (INV-12).")
	for _, c := range clusters {
		metric(b, "holzkube_cluster_locked", labels{{"cluster", string(c.ID)}}, boolean(c.Locked))
	}
}

// writeMachines emits everything counted over the fleet.
//
// Every count is per cluster and never per machine, and the buckets are written
// even when they are empty -- see the package comment for why a zero has to be
// present rather than absent.
func (e *Exporter) writeMachines(
	b *strings.Builder,
	clusters []inventory.ClusterView,
	machines []inventory.MachineView,
) {
	// The cluster set is taken from the clusters *and* from the machines, so
	// that a machine belonging to no cluster is still counted -- under the
	// empty label. Leaving those out would make the fleet total on this page
	// disagree with the fleet on the screen, and the nodes it dropped are
	// exactly the ones in maintenance mode.
	ids := []model.ClusterID{unassigned}
	seen := map[model.ClusterID]bool{unassigned: true}
	for _, c := range clusters {
		if !seen[c.ID] {
			seen[c.ID] = true
			ids = append(ids, c.ID)
		}
	}
	for _, m := range machines {
		if !seen[m.Cluster] {
			seen[m.Cluster] = true
			ids = append(ids, m.Cluster)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	byStage := map[model.ClusterID]map[health.Stage]int{}
	watchLive := map[model.ClusterID]int{}
	unsupported := map[model.ClusterID]int{}
	etcdVotes := map[model.ClusterID]int{}

	for _, m := range machines {
		if byStage[m.Cluster] == nil {
			byStage[m.Cluster] = map[health.Stage]int{}
		}
		byStage[m.Cluster][m.Stage]++

		if m.Watch.Live {
			watchLive[m.Cluster]++
		}
		if m.UnsupportedVersion {
			unsupported[m.Cluster]++
		}
		// Only a confirmed membership counts. A node whose etcd level is stale
		// is a node whose last known answer was "member", and counting it
		// would report a quorum this product cannot currently see -- which is
		// the one number where a hopeful answer is worse than none.
		if m.EtcdMember.Available && m.EtcdMember.Value {
			etcdVotes[m.Cluster]++
		}
	}

	help(b, "holzkube_machines", "gauge",
		"Machines in each observation stage. Every stage is reported for every cluster, zeros "+
			"included: a series that disappears at zero shows its last non-zero value for as long "+
			"as anybody looks at the graph.")
	for _, id := range ids {
		for _, st := range health.Stages() {
			metric(b, "holzkube_machines",
				labels{{"cluster", string(id)}, {"stage", st.String()}},
				float64(byStage[id][st]))
		}
	}

	help(b, "holzkube_machines_watch_live", "gauge",
		"Machines whose resource watch has delivered a snapshot and is still delivering (INV-13). "+
			"A number below the fleet size is not an outage: those nodes are being read on the "+
			"heartbeat instead, so they are at most one heartbeat behind.")
	for _, id := range ids {
		metric(b, "holzkube_machines_watch_live", labels{{"cluster", string(id)}}, float64(watchLive[id]))
	}

	help(b, "holzkube_machines_unsupported_version", "gauge",
		"Machines running a Talos version outside the range this build was tested against (OPS-03).")
	for _, id := range ids {
		metric(b, "holzkube_machines_unsupported_version", labels{{"cluster", string(id)}}, float64(unsupported[id]))
	}

	help(b, "holzkube_cluster_etcd_members_confirmed", "gauge",
		"Control-plane nodes currently confirmed as etcd members. It counts what this instance "+
			"can see right now and never a stale answer: a quorum reported from memory is the one "+
			"number where a hopeful value is worse than none.")
	for _, id := range ids {
		metric(b, "holzkube_cluster_etcd_members_confirmed", labels{{"cluster", string(id)}}, float64(etcdVotes[id]))
	}
}

// writeJobs emits the job records by kind and state.
//
// A gauge and not a counter, and the distinction is worth the sentence: this is
// how many records the store currently holds, not how many jobs have ever run.
// The two are the same number today because nothing deletes a job record, and
// they would stop being the same after a restore from backup -- at which point a
// counter would have gone backwards, which Prometheus reads as a process
// restart and silently repairs by adding the whole value again.
func (e *Exporter) writeJobs(b *strings.Builder, jobs []model.Job) {
	counts := map[model.JobKind]map[model.JobState]int{}
	for _, j := range jobs {
		if counts[j.Kind] == nil {
			counts[j.Kind] = map[model.JobState]int{}
		}
		counts[j.Kind][j.State]++
	}

	kinds := make([]model.JobKind, 0, len(counts))
	for k := range counts {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })

	help(b, "holzkube_jobs", "gauge",
		"Job records this instance holds, by kind and state. A gauge and not a counter: a restore "+
			"from backup can lower it, and a counter that goes backwards is read as a restart and "+
			"double-counted.")
	for _, k := range kinds {
		for _, st := range model.JobStates() {
			metric(b, "holzkube_jobs",
				labels{{"kind", string(k)}, {"state", string(st)}},
				float64(counts[k][st]))
		}
	}
}

func (e *Exporter) writeAudit(b *strings.Builder) {
	help(b, "holzkube_audit_chain_intact", "gauge",
		"1 when the audit archive's hash chain verified at startup (D-15). A 0 means the archive "+
			"has been altered or truncated and every record after the break is unproven.")

	ok := false
	if e.deps.AuditChainIntact != nil {
		ok = e.deps.AuditChainIntact()
	}
	metric(b, "holzkube_audit_chain_intact", nil, boolean(ok))
}

// label is one name/value pair.
type label struct{ name, value string }

type labels []label

// help writes the two lines every metric owes.
//
// They are not optional and they are not decoration: a metric without a HELP is
// a number whose meaning lives in somebody's head, and a metric without a TYPE
// is one a query engine will average when it should have rated.
func help(b *strings.Builder, name, typ, text string) {
	b.WriteString("# HELP ")
	b.WriteString(name)
	b.WriteByte(' ')
	b.WriteString(escapeHelp(text))
	b.WriteByte('\n')

	b.WriteString("# TYPE ")
	b.WriteString(name)
	b.WriteByte(' ')
	b.WriteString(typ)
	b.WriteByte('\n')
}

// metric writes one sample.
func metric(b *strings.Builder, name string, ls labels, v float64) {
	b.WriteString(name)

	if len(ls) > 0 {
		b.WriteByte('{')
		for i, l := range ls {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(l.name)
			b.WriteString(`="`)
			b.WriteString(escapeLabel(l.value))
			b.WriteByte('"')
		}
		b.WriteByte('}')
	}

	b.WriteByte(' ')
	b.WriteString(number(v))
	b.WriteByte('\n')
}

// number renders a sample value.
//
// 'g' with -1 precision so that a whole number comes out as "3" rather than
// "3.000000" and a fractional one keeps every digit it has. Prometheus accepts
// both; a page full of trailing zeroes is simply unreadable by a human, and this
// page is read by humans during incidents.
func number(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func boolean(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

// escapeLabel escapes a label value.
//
// Backslash, double quote and newline, which is the whole set the exposition
// format defines. Cluster ids and job kinds are this product's own and contain
// none of them; a name is an operator's free text and can contain all three,
// and an unescaped quote in one would not corrupt that series but the rest of
// the scrape after it.
func escapeLabel(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s)
}

// escapeHelp escapes a HELP line, where a quote is ordinary text and only the
// backslash and the newline end the line early.
func escapeHelp(s string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`).Replace(s)
}
