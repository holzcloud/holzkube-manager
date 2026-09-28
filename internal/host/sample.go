package host

import (
	"context"
	"errors"
	"strconv"
	"time"
)

// Sample is the host's contribution to the metrics history: what the sampler
// records under history.HostSubject every fifteen seconds (2026-09-28).
//
// It is not Read, and must not call it. Read walks the data directory for its
// size and reads the update status file, neither of which the history keeps,
// and Read advances the page's rate memo. Sample takes its CPU and network
// rates against a baseline of its own (samplePrev), so its window is the
// sampler's fifteen seconds whether or not /host is open and polling.
//
// On a platform with no readings (a darwin build) it is an empty map and no
// error: nothing is recorded, which is not a failure worth a log line on every
// pass.
//
// One Sample runs at a time. The sampler stops waiting for a read that hangs
// -- a statfs on a share that stopped answering -- but cannot stop the read;
// a Sample started while that one is still stuck answers ErrSampleBusy at
// once rather than hanging beside it, so a host whose disk hangs for an hour
// holds one goroutine, not two hundred and forty.
func (c *Collector) Sample(ctx context.Context) (map[string]float64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !c.sampling.CompareAndSwap(false, true) {
		return nil, ErrSampleBusy
	}
	defer c.sampling.Store(false)

	live := c.sampleLive()

	name := ""
	if u, err := c.cfg.Sys.Uname(); err == nil {
		name = u.Nodename
	}
	snap := Snapshot{At: c.cfg.Now(), Name: name, Health: Assess(live)}
	c.mu.Lock()
	c.latest = &snap
	c.mu.Unlock()

	return HistoryValues(live), nil
}

// ErrSampleBusy is Sample's answer while an earlier Sample has not returned.
var ErrSampleBusy = errors.New("the previous host read has not finished; a filesystem or sysfs read is hanging")

// Snapshot is what the sampler's last pass saw of the host: when, under which
// name, and in what state.
type Snapshot struct {
	// At is the Collector's clock when the sample was taken.
	At time.Time
	// Name is uname(2)'s nodename, or "" when it could not be read.
	Name string
	// Health is Assess of the reading the sample was taken from.
	Health Health
}

// Latest is the sampler's last snapshot, and false before the first one.
//
// The wall reads this rather than reading the host on every request: a screen
// refreshing every ten seconds costs the host nothing, and a sampler that
// stalled shows as an old snapshot -- which the wall's reader turns grey (D-11)
// -- instead of as a fresh green.
func (c *Collector) Latest() (Snapshot, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.latest == nil {
		return Snapshot{}, false
	}
	return *c.latest, true
}

// sampleLive is one Live reading against the sampler's own baseline slot.
func (c *Collector) sampleLive() Live {
	if platformUnsupported(c.cfg.Sys) {
		return c.unsupportedLive()
	}
	return c.readLive(&c.samplePrev)
}

// HistoryValues is what one reading of the host contributes to its history.
//
// The keys are a node's keys (history.HardwareValues), so the charts on /host
// are the node's charts and read the same series names: cpu, core:<n>, memory
// (used over total, in percent), rx and tx (bytes per second, summed over the
// physical interfaces), temp:<chip>/<label> and fan:<chip>/<label>. There is
// no read or write: the host has no disk-throughput reading.
//
// A value that was not read is absent from the map -- never a 0 -- so its
// slot in the ring stays empty and the chart draws a gap where a zero would
// draw a host gone cold (D-03, HMON-06). That includes CPU and memory under
// ProcSubset=pid, and rx/tx whenever a physical interface has no rate this
// round: summed without it, the total would under-report for that slot, and a
// one-slot gap is invisible where a dip is not. A machine with no physical
// interface records no rx/tx at all; the virtual ones (loopback, bridges,
// veths) carry the containers' traffic twice over and are never summed.
//
// It lives here rather than in internal/history so that the history does not
// import the host package; the sampler only sees the map.
func HistoryValues(l Live) map[string]float64 {
	out := map[string]float64{}

	if l.CPU.Usage.Readable && l.CPU.Usage.Value != nil {
		out["cpu"] = *l.CPU.Usage.Value
	}
	if l.CPU.PerCore.Readable && l.CPU.PerCore.Value != nil {
		for i, p := range *l.CPU.PerCore.Value {
			out["core:"+strconv.Itoa(i)] = p
		}
	}
	if l.Memory.Readable && l.Memory.Value != nil && l.Memory.Value.TotalBytes > 0 {
		m := l.Memory.Value
		out["memory"] = 100 * float64(m.UsedBytes) / float64(m.TotalBytes)
	}
	if rx, tx, ok := physicalRates(l); ok {
		out["rx"], out["tx"] = rx, tx
	}

	if l.Sensors.Readable && l.Sensors.Value != nil {
		for _, t := range l.Sensors.Value.Temperatures {
			out["temp:"+t.Chip+"/"+t.Label] = t.Celsius
		}
		for _, f := range l.Sensors.Value.Fans {
			out["fan:"+f.Chip+"/"+f.Label] = float64(f.RPM)
		}
	}
	return out
}

// physicalRates sums the physical interfaces' throughput, and says whether
// there is a sum to record: the network was read, this round has a rate
// window, there is at least one physical interface, and every one of them has
// both rates.
func physicalRates(l Live) (rx, tx float64, ok bool) {
	if !l.Network.Readable || l.Network.Value == nil || l.RatesOverSeconds == nil {
		return 0, 0, false
	}
	links := l.Network.Value.Physical
	if len(links) == 0 {
		return 0, 0, false
	}
	for _, link := range links {
		if link.RxBytesPerSec == nil || link.TxBytesPerSec == nil {
			return 0, 0, false
		}
		rx += *link.RxBytesPerSec
		tx += *link.TxBytesPerSec
	}
	return rx, tx, true
}
