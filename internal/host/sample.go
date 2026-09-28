package host

import (
	"context"
)

// Sample is the host's contribution to the metrics history: what the sampler
// records under history.HostSubject every fifteen seconds (2026-09-28).
//
// It is not Read, and must not call it. Read walks the data directory for its
// size and advances the rate memo the page's CPU and network rates are taken
// against; a sampler sharing that memo would shorten the page's windows while
// it is open, and have its own shortened by the page. Sample reads only what
// the history keeps.
//
// On a platform with no readings (a darwin build) it is an empty map and no
// error: nothing is recorded, which is not a failure worth a log line on every
// pass.
func (c *Collector) Sample(ctx context.Context) (map[string]float64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if platformUnsupported(c.cfg.Sys) {
		return map[string]float64{}, nil
	}

	var live Live
	if sensors, err := readSensors(c.cfg.FS); err != nil {
		live.Sensors = Hidden[Sensors](reasonFor(sensorsPath(err), err))
	} else {
		live.Sensors = Read(sensors)
	}
	return HistoryValues(live), nil
}

// HistoryValues is what one reading of the host contributes to its history.
//
// The keys are a node's keys (history.HardwareValues), so the charts on /host
// are the node's charts and read the same series names. A value that was not
// read is absent from the map -- never a 0 -- so its slot in the ring stays
// empty and the chart draws a gap where a zero would draw a host gone cold.
//
// It lives here rather than in internal/history so that the history does not
// import the host package; the sampler only sees the map.
func HistoryValues(l Live) map[string]float64 {
	out := map[string]float64{}
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
