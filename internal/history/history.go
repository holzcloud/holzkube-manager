// Package history is the last twenty-four hours of what the gauges showed: a
// node's CPU, memory, throughput, cores, temperatures and fans, and an app's CPU
// and memory, kept so a chart can open on the past rather than start drawing at
// the moment somebody looked (2026-09-26).
//
// # What it is not
//
// It is not the inventory and not a record. The inventory says what a node IS
// and keeps that for ever (D-16); this says what a node was DOING, and forgets
// it after a day. Nothing here decides anything, nothing here is audited, and a
// lost history file costs a chart its left-hand side and nothing else -- which is
// why a file that will not decode is set aside with a warning rather than
// refusing to start.
//
// # Two tiers, because one would be either too coarse or too big
//
// The last hour at the sampler's own fifteen seconds, and the day before it at
// one minute, each minute the average of the fifteen-second samples in it. The
// operator chose that split: a spike a quarter of a minute long is worth seeing
// while it is recent, and at a day's width a chart has fewer pixels than a day
// has minutes anyway.
//
// Both tiers are rings over ABSOLUTE slots -- the slot of a sample is its Unix
// time divided by the step -- rather than lists of timestamped points. So a
// slot nobody wrote is a gap by construction: a node that did not answer for
// ten minutes leaves ten minutes of nothing, and a chart draws a break, never a
// line through zero (INV-08 again: a node that was not heard from is not a node
// that was idle).
//
// # Why float32
//
// The values are gauges read off a kernel: a percentage to a tenth, a
// temperature to a tenth, a byte rate. Seven significant digits is more than
// any of them carries, and half the bytes matters twice -- in memory on a Pi,
// and in a file rewritten every minute on an SD card. The value is float32 in
// memory as well as on disk, so what a chart shows is the same before a restart
// and after it.
package history

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

const (
	// FineStep is the sampler's interval and the resolution of the last hour.
	FineStep = 15 * time.Second

	// CoarseStep is the resolution of the rest of the day.
	CoarseStep = time.Minute

	// FineSlots is the fine ring's length: an hour of fifteen seconds.
	FineSlots = int(time.Hour / FineStep)

	// CoarseSlots is the coarse ring's length: a day of minutes.
	CoarseSlots = int(24 * time.Hour / CoarseStep)

	// Retention is how long anything is kept: a subject not heard from for
	// this long is deleted, from memory and from the file.
	Retention = 24 * time.Hour

	// finePerCoarse is how many fine slots make one coarse one.
	finePerCoarse = int64(CoarseStep / FineStep)
)

// Range is what a chart asks for.
type Range string

// The three ranges the contract names.
const (
	Range1h  Range = "1h"
	Range6h  Range = "6h"
	Range24h Range = "24h"
)

// ParseRange reads the query parameter. Empty is an hour, which is what a chart
// that has not been told otherwise shows.
func ParseRange(s string) (Range, bool) {
	switch Range(s) {
	case "":
		return Range1h, true
	case Range1h, Range6h, Range24h:
		return Range(s), true
	default:
		return "", false
	}
}

// tier is which ring a range reads and how many of its slots.
//
// An hour is the fine ring whole. Six hours and a day are the coarse ring: the
// fine ring does not reach back that far, and a chart of six hours at fifteen
// seconds would be 1440 points for a line a few hundred pixels wide.
func (r Range) tier() (fine bool, step time.Duration, slots int) {
	switch r {
	case Range6h:
		return false, CoarseStep, int(6 * time.Hour / CoarseStep)
	case Range24h:
		return false, CoarseStep, CoarseSlots
	default:
		return true, FineStep, FineSlots
	}
}

// Point is one sample as the API carries it: [unix milliseconds, value].
type Point [2]float64

// View is the history route's answer. Its shape is the API contract.
type View struct {
	Range       Range  `json:"range"`
	StepSeconds int    `json:"step_seconds"`
	From        string `json:"from"`
	To          string `json:"to"`

	// Series is never null, and a series with nothing in the range is left
	// out rather than sent as an empty list: "no such sensor in this window"
	// and "a sensor with no readings" are the same thing to a chart.
	Series map[string][]Point `json:"series"`
}

// MachineSubject is the key a machine's hardware history is filed under.
func MachineSubject(id model.MachineID) string { return "machine/" + string(id) }

// AppSubject is the key an app's history is filed under. kind is the list's
// spelling (kube.CanonicalAppKind), so "deployment" and "Deployment" in two
// URLs are one app.
func AppSubject(cluster model.ClusterID, namespace, kind, name string) string {
	return appPrefix(cluster) + namespace + "/" + kind + "/" + name
}

func appPrefix(cluster model.ClusterID) string { return "app/" + string(cluster) + "/" }

// ring is one tier of one series: a fixed number of consecutive absolute slots
// ending at head, NaN where nothing was written.
type ring struct {
	vals []float32
	head int64
}

func newRing(n int) *ring {
	r := &ring{vals: make([]float32, n)}
	for i := range r.vals {
		r.vals[i] = nan32
	}
	return r
}

var nan32 = float32(math.NaN())

func (r *ring) index(slot int64) int {
	n := int64(len(r.vals))
	return int(((slot % n) + n) % n)
}

// put writes one slot. A slot newer than head moves the ring forward, and every
// slot it moves over is cleared: those are the samples that never came, and a
// value left over from the previous lap of the ring would be a reading from an
// hour or a day ago drawn as if it were now. A slot older than the ring holds
// is dropped.
func (r *ring) put(slot int64, v float32) {
	n := int64(len(r.vals))
	if slot > r.head {
		from := max(r.head+1, slot-n+1)
		for s := from; s <= slot; s++ {
			r.vals[r.index(s)] = nan32
		}
		r.head = slot
	}
	if slot <= r.head-n {
		return
	}
	r.vals[r.index(slot)] = v
}

// at reads one slot, NaN for anything outside what the ring holds.
func (r *ring) at(slot int64) float32 {
	if slot > r.head || slot <= r.head-int64(len(r.vals)) {
		return nan32
	}
	return r.vals[r.index(slot)]
}

// series is one quantity's two tiers.
type series struct {
	fine, coarse *ring
}

func newSeries() *series {
	return &series{fine: newRing(FineSlots), coarse: newRing(CoarseSlots)}
}

// subject is one machine's or one app's series.
type subject struct {
	// seen is the last time anything was recorded for it. Retention is
	// measured from here and not from the file's age: a node that has been
	// off for a day has nothing left worth drawing.
	seen   time.Time
	series map[string]*series
}

// Store is every subject's history, and the file it is kept in.
type Store struct {
	mu       sync.Mutex
	subjects map[string]*subject

	// apps is, per cluster, the apps the last successful listing named,
	// whether or not their usage was known. It is what lets the app route say
	// 404 for an app that does not exist and 200 with nothing for one that
	// does and has not been measured.
	apps map[model.ClusterID]map[string]bool

	// The file. See persist.go.
	path      string
	write     func(path string, data []byte) error
	dirty     bool
	lastFlush time.Time
	writes    int
}

// NewMemory is a Store with no file behind it, for a test or a caller that does
// not want one. Flush on it does nothing.
func NewMemory() *Store {
	return &Store{
		subjects: map[string]*subject{},
		apps:     map[model.ClusterID]map[string]bool{},
	}
}

// Record files one sample of every value in values for one subject at one
// moment. A value that is not a finite number is not recorded: NaN is how a
// gap is stored, and an infinity is a division by a zero somebody forgot.
func (s *Store) Record(key string, at time.Time, values map[string]float64) {
	if len(values) == 0 {
		return
	}
	fine := floorDiv(at.UnixMilli(), FineStep.Milliseconds())
	minute := floorDiv(fine, finePerCoarse)

	s.mu.Lock()
	defer s.mu.Unlock()

	sub := s.subjects[key]
	if sub == nil {
		sub = &subject{series: map[string]*series{}}
		s.subjects[key] = sub
	}
	if at.After(sub.seen) {
		sub.seen = at
	}

	for name, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		ser := sub.series[name]
		if ser == nil {
			ser = newSeries()
			sub.series[name] = ser
		}
		ser.fine.put(fine, float32(v))
		// NaN only when the sample was too old for the fine ring to take, and
		// then the minute it belongs to keeps whatever it had rather than
		// being overwritten with a gap.
		if avg := ser.minuteAverage(minute); !math.IsNaN(float64(avg)) {
			ser.coarse.put(minute, avg)
		}
	}
	s.dirty = true
}

// minuteAverage is one minute's value in the coarse tier: the mean of the fine
// samples in it that exist.
//
// Recomputed from the fine ring on every sample rather than kept as a running
// sum, so there is one source for it: the minute in progress is always the
// average of what the fine tier holds for that minute, including after a
// restart that reloaded half of it from the file.
func (ser *series) minuteAverage(minute int64) float32 {
	var sum float64
	var n int
	for slot := minute * finePerCoarse; slot < (minute+1)*finePerCoarse; slot++ {
		if v := ser.fine.at(slot); !math.IsNaN(float64(v)) {
			sum += float64(v)
			n++
		}
	}
	if n == 0 {
		return nan32
	}
	return float32(sum / float64(n))
}

// Query is one subject's history over a range, ending now.
func (s *Store) Query(key string, r Range, now time.Time) View {
	fine, step, slots := r.tier()
	stepMS := step.Milliseconds()

	view := View{
		Range:       r,
		StepSeconds: int(step / time.Second),
		From:        now.Add(-time.Duration(slots) * step).UTC().Format(time.RFC3339),
		To:          now.UTC().Format(time.RFC3339),
		Series:      map[string][]Point{},
	}

	last := floorDiv(now.UnixMilli(), stepMS)
	first := last - int64(slots) + 1

	s.mu.Lock()
	defer s.mu.Unlock()

	sub := s.subjects[key]
	if sub == nil {
		return view
	}
	for name, ser := range sub.series {
		rg := ser.coarse
		if fine {
			rg = ser.fine
		}
		var points []Point
		for slot := first; slot <= last; slot++ {
			v := rg.at(slot)
			if math.IsNaN(float64(v)) {
				continue
			}
			points = append(points, Point{float64(slot * stepMS), shortest(v)})
		}
		if len(points) > 0 {
			view.Series[name] = points
		}
	}
	return view
}

// shortest is a float32 as the shortest decimal that is still that float32:
// 18.2 rather than 18.200000762939453, which is what widening it gives and what
// a chart's tooltip would print.
func shortest(v float32) float64 {
	f, err := strconv.ParseFloat(strconv.FormatFloat(float64(v), 'g', -1, 32), 64)
	if err != nil {
		return float64(v)
	}
	return f
}

// Has reports whether anything is kept for a subject.
func (s *Store) Has(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.subjects[key]
	return ok
}

// Subjects is every key held, sorted.
func (s *Store) Subjects() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.subjects))
	for k := range s.subjects {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Retain deletes every subject whose machine or cluster is no longer in the
// inventory, and returns how many it deleted.
//
// Forgetting a machine or a cluster is the operator saying it is gone, and its
// history goes with it; waiting out the day's retention would keep a chart for
// something no screen can reach any more. A machine that is merely unassigned
// is still a machine, and keeps its history until it ages out.
func (s *Store) Retain(machines map[model.MachineID]bool, clusters map[model.ClusterID]bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	for key := range s.subjects {
		if !retained(key, machines, clusters) {
			delete(s.subjects, key)
			n++
		}
	}
	for cluster := range s.apps {
		if !clusters[cluster] {
			delete(s.apps, cluster)
		}
	}
	if n > 0 {
		s.dirty = true
	}
	return n
}

func retained(key string, machines map[model.MachineID]bool, clusters map[model.ClusterID]bool) bool {
	if id, ok := strings.CutPrefix(key, "machine/"); ok {
		return machines[model.MachineID(id)]
	}
	if rest, ok := strings.CutPrefix(key, "app/"); ok {
		cluster, _, _ := strings.Cut(rest, "/")
		return clusters[model.ClusterID(cluster)]
	}
	// A key this version does not know how to read -- one a later version
	// wrote, loaded after a downgrade -- is left to age out rather than
	// deleted on sight.
	return true
}

// Prune deletes every subject not heard from within the retention.
func (s *Store) Prune(now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	for key, sub := range s.subjects {
		if now.Sub(sub.seen) > Retention {
			delete(s.subjects, key)
			n++
		}
	}
	if n > 0 {
		s.dirty = true
	}
	return n
}

// SetApps records the apps one successful listing of a cluster named.
func (s *Store) SetApps(cluster model.ClusterID, keys []string) {
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apps[cluster] = set
}

// AppKnown reports whether an app exists as far as this store can tell.
//
// known is true for an app that has history, or that the last listing of its
// cluster named. certain is false when there has been no successful listing of
// the cluster since this process started -- the cluster is down, or the
// daemon has just come up -- and then an app without history cannot be called
// unknown: nobody has asked.
func (s *Store) AppKnown(cluster model.ClusterID, namespace, kind, name string) (known, certain bool) {
	key := AppSubject(cluster, namespace, kind, name)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.subjects[key]; ok {
		return true, true
	}
	listed, ok := s.apps[cluster]
	if !ok {
		return false, false
	}
	return listed[key], true
}

// floorDiv is division rounding towards minus infinity, so a slot is the same
// function of time on both sides of 1970. Nothing samples before 1970; a test
// clock at zero does.
func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}
