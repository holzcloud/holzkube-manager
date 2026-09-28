package host

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
)

// The host's history values (Phase 12, D-03): the node's keys, and nothing for
// a value that was not read -- the history's gap is an absent key, and a 0
// written in its place would be a chart of a host gone cold.

// sampleSensorsFS is one CPU chip and one fan header, the way hwmon lists them.
func sampleSensorsFS() fstest.MapFS {
	return fstest.MapFS{
		"sys/class/hwmon/hwmon0/name":        {Data: []byte("cpu_thermal\n")},
		"sys/class/hwmon/hwmon0/temp1_input": {Data: []byte("64400\n")},
		"sys/class/hwmon/hwmon1/name":        {Data: []byte("pwmfan\n")},
		"sys/class/hwmon/hwmon1/fan1_input":  {Data: []byte("2100\n")},
	}
}

// TestHistoryValuesSensors: a readable reading gives exactly its temp: and fan:
// keys; an unreadable one gives an empty map, never a key with a 0; and a
// platform with no readings samples nothing, without an error.
//
// Fault injected and seen red: HistoryValues writing the temp: keys as 0 when
// the reading is hidden.
func TestHistoryValuesSensors(t *testing.T) {
	t.Parallel()

	t.Run("readable", func(t *testing.T) {
		t.Parallel()

		got := HistoryValues(Live{Sensors: Read(Sensors{
			Temperatures: []inventory.HardwareTemperature{{Chip: "cpu_thermal", Kind: "cpu", Label: "temp1", Celsius: 64.4}},
			Fans:         []inventory.HardwareFan{{Chip: "pwmfan", Label: "fan1", RPM: 2100}},
		})})
		want := map[string]float64{"temp:cpu_thermal/temp1": 64.4, "fan:pwmfan/fan1": 2100}
		if len(got) != len(want) {
			t.Errorf("HistoryValues = %v, want exactly %v", got, want)
		}
		for k, v := range want {
			if g, ok := got[k]; !ok || g != v {
				t.Errorf("%s = %v (present %v), want %v", k, g, ok, v)
			}
		}
	})

	t.Run("hidden", func(t *testing.T) {
		t.Parallel()

		got := HistoryValues(Live{Sensors: Hidden[Sensors](Reason{Code: CodeReadFailed, Message: "Could not list /sys/class/hwmon"})})
		if got == nil {
			t.Fatal("HistoryValues of an unreadable reading is nil; want an empty map")
		}
		if len(got) != 0 {
			t.Errorf("HistoryValues of an unreadable reading = %v; want no key at all, never a 0", got)
		}
	})

	t.Run("a collector reads the sensors through Sample", func(t *testing.T) {
		t.Parallel()

		got, err := newTestCollector(sampleSensorsFS(), tracerSys()).Sample(context.Background())
		if err != nil {
			t.Fatalf("Sample: %v", err)
		}
		if got["temp:cpu_thermal/temp1"] != 64.4 || got["fan:pwmfan/fan1"] != 2100 || len(got) != 2 {
			t.Errorf("Sample = %v, want the CPU temperature and the fan", got)
		}
	})

	t.Run("a platform with no readings samples nothing", func(t *testing.T) {
		t.Parallel()

		got, err := newTestCollector(sampleSensorsFS(), newUnsupportedSys()).Sample(context.Background())
		if err != nil {
			t.Fatalf("Sample on an unsupported platform: %v; want no error", err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("Sample on an unsupported platform = %v; want an empty map", got)
		}
	})

	t.Run("a cancelled context samples nothing", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if got, err := newTestCollector(sampleSensorsFS(), tracerSys()).Sample(ctx); err == nil {
			t.Errorf("Sample with a cancelled context = %v, nil; want the context's error", got)
		}
	})
}

// overlayFS is a fixture tree with some of its files replaced: a file named in
// over is read from there, anything else from the fixture, and a directory the
// fixture does not have at all from over -- so a test can move a fixture's
// counters between two reads, or give a fixture sensors it lacks. Lstat and
// ReadLink go to the fixture, whose /sys/class/net entries are symlinks.
type overlayFS struct {
	fs.FS
	over fstest.MapFS
}

func (o overlayFS) Open(name string) (fs.File, error) {
	if _, replaced := o.over[name]; replaced {
		return o.over.Open(name)
	}
	f, err := o.FS.Open(name)
	if errors.Is(err, fs.ErrNotExist) {
		return o.over.Open(name)
	}
	return f, err
}

func (o overlayFS) Lstat(name string) (fs.FileInfo, error) { return fs.Lstat(o.FS, name) }
func (o overlayFS) ReadLink(name string) (string, error)   { return fs.ReadLink(o.FS, name) }

func counterFile(n uint64) *fstest.MapFile { return &fstest.MapFile{Data: fmt.Appendf(nil, "%d\n", n)} }

// pi5Moved is the Pi 5 fixture 3 s of work later: every core 100 ticks busy
// and 100 idle (50 %), eth0 300000 B in and 30000 out, wlan0 3000 and 300 --
// and every virtual interface 9 MB each way, which the history must not see.
func pi5Moved() fstest.MapFS {
	over := fstest.MapFS{
		"proc/stat": {Data: []byte("cpu  400400 1000 150000 5000400 20000 0 5000 0 0 0\n" +
			"cpu0 100100 250 37500 1250100 5000 0 1250 0 0 0\n" +
			"cpu1 100100 250 37500 1250100 5000 0 1250 0 0 0\n" +
			"cpu2 100100 250 37500 1250100 5000 0 1250 0 0 0\n" +
			"cpu3 100100 250 37500 1250100 5000 0 1250 0 0 0\n" +
			"btime 1790000000\n")},
		"sys/class/net/eth0/statistics/rx_bytes":  counterFile(48213377120 + 300_000),
		"sys/class/net/eth0/statistics/tx_bytes":  counterFile(9143021788 + 30_000),
		"sys/class/net/wlan0/statistics/rx_bytes": counterFile(1203311 + 3_000),
		"sys/class/net/wlan0/statistics/tx_bytes": counterFile(88412 + 300),
		"sys/class/net/lo/statistics/rx_bytes":    counterFile(912004421 + 9_000_000),
		"sys/class/net/lo/statistics/tx_bytes":    counterFile(912004421 + 9_000_000),
	}
	for _, v := range []string{"br-0a1b2c3d4e5f", "docker0", "veth1a2b3c4", "veth5d6e7f8"} {
		over["sys/class/net/"+v+"/statistics/rx_bytes"] = counterFile(3301442 + 9_000_000)
		over["sys/class/net/"+v+"/statistics/tx_bytes"] = counterFile(7719003 + 9_000_000)
	}
	return over
}

// twoReads samples a collector over first, then over second 3 s later, and
// returns the second sample's values.
func twoReads(t *testing.T, sys Sys, first fs.FS, then func()) map[string]float64 {
	t.Helper()
	clock := newLiveClock()
	c := New(Config{FS: first, Sys: sys, Now: clock.now})
	if _, err := c.Sample(context.Background()); err != nil {
		t.Fatalf("first Sample: %v", err)
	}
	clock.advance(3 * time.Second)
	then()
	got, err := c.Sample(context.Background())
	if err != nil {
		t.Fatalf("second Sample: %v", err)
	}
	return got
}

func wantValues(t *testing.T, got, want map[string]float64) {
	t.Helper()
	keys := slices.Sorted(maps.Keys(got))
	wantKeys := slices.Sorted(maps.Keys(want))
	if !slices.Equal(keys, wantKeys) {
		t.Errorf("keys = %v\nwant   %v", keys, wantKeys)
	}
	for k, w := range want {
		if g, ok := got[k]; ok && math.Abs(g-w) > 1e-9 {
			t.Errorf("%s = %v, want %v", k, g, w)
		}
	}
}

// TestHistoryValues: the host records what a node records -- cpu, core:<n>,
// memory, rx and tx, temp: and fan: -- and nothing for a value it did not read.
// rx and tx are the physical interfaces only, and only when every one of them
// has a rate this round (RESEARCH A3).
//
// Faults injected and seen red: cpu written as 0 when the usage is hidden (the
// hardened fixture); the virtual interfaces' rates added to rx and tx (the Pi 5
// and the veth case).
func TestHistoryValues(t *testing.T) {
	t.Parallel()

	t.Run("the Pi 5, second read", func(t *testing.T) {
		t.Parallel()

		over := fstest.MapFS{}
		moving := overlayFS{FS: fixtureFS(t, "pi5"), over: over}
		got := twoReads(t, fakeSys{loads: sysinfoLoads}, moving, func() { maps.Copy(over, pi5Moved()) })
		wantValues(t, got, map[string]float64{
			"cpu":    50,
			"core:0": 50, "core:1": 50, "core:2": 50, "core:3": 50,
			// free -b on the measured sample: 4093853696 used of 8453947392.
			"memory": 100 * 4093853696.0 / 8453947392.0,
			// eth0 100000 + wlan0 1000 in, 10000 + 100 out -- and not a byte
			// of the 3 MB/s every virtual interface moved.
			"rx":                     101_000,
			"tx":                     10_100,
			"temp:cpu_thermal/temp1": 64.4,
			"temp:rp1_adc/temp1":     55.4,
		})
	})

	t.Run("ProcSubset=pid hides cpu and memory, and writes no key for them", func(t *testing.T) {
		t.Parallel()

		over := fstest.MapFS{}
		maps.Copy(over, sampleSensorsFS())
		got := twoReads(t, fakeSys{loads: sysinfoLoads}, overlayFS{FS: fixtureFS(t, "procsubset-pid"), over: over}, func() {})
		wantValues(t, got, map[string]float64{"temp:cpu_thermal/temp1": 64.4, "fan:pwmfan/fan1": 2100})
		for _, k := range []string{"cpu", "memory", "core:0"} {
			if v, has := got[k]; has {
				t.Errorf("%s = %v under ProcSubset=pid; want no key, never a 0", k, v)
			}
		}
	})

	t.Run("a virtual link's traffic is not the host's", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{}
		netDir(fsys, "eth0", true, 1_000_000, 500_000)
		netDir(fsys, "veth0", false, 0, 0)
		got := twoReads(t, fakeSys{loads: sysinfoLoads}, fsys, func() {
			netDir(fsys, "eth0", true, 1_300_000, 530_000)
			netDir(fsys, "veth0", false, 90_000_000, 90_000_000)
		})
		wantValues(t, got, map[string]float64{"rx": 100_000, "tx": 10_000})
	})

	t.Run("a physical link without a rate takes rx and tx with it", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{"proc/stat": procStat([2]uint64{100, 100})}
		netDir(fsys, "eth0", true, 1_000_000, 500_000)
		got := twoReads(t, fakeSys{loads: sysinfoLoads}, fsys, func() {
			fsys["proc/stat"] = procStat([2]uint64{250, 250})
			netDir(fsys, "eth0", true, 1_300_000, 530_000)
			// eth1 appeared since the last read: no baseline, no rate. A sum
			// without it would under-report for this slot.
			netDir(fsys, "eth1", true, 5_000, 5_000)
		})
		wantValues(t, got, map[string]float64{"cpu": 50, "core:0": 50})
	})

	t.Run("the first read has no rates and records none", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{"proc/stat": procStat([2]uint64{100, 100})}
		netDir(fsys, "eth0", true, 1_000_000, 500_000)
		got := mustSample(t, fsys)
		if len(got) != 0 {
			t.Errorf("first Sample = %v; want nothing -- no rate has a baseline yet", got)
		}
	})
}

func mustSample(t *testing.T, fsys fs.FS) map[string]float64 {
	t.Helper()
	got, err := New(Config{FS: fsys, Sys: fakeSys{loads: sysinfoLoads}, Now: newLiveClock().now}).Sample(context.Background())
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	return got
}

// TestSampleHasItsOwnBaseline (RESEARCH Pitfall 1): with /host open and polling
// every 3 s, the page's rates stay over 3 s and the sampler's over its 15 s.
// One shared memo would give each the time since whichever read came last.
//
// Fault injected and seen red: sampleLive reading against &c.prev, the page's
// slot -- the page's 6-s read then reported 1.5 s, and the sampler's 13.5 s.
func TestSampleHasItsOwnBaseline(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{"proc/stat": procStat([2]uint64{100, 100})}
	clock := newLiveClock()
	c := New(Config{FS: fsys, Sys: fakeSys{loads: sysinfoLoads}, Now: clock.now})
	tick := func(d time.Duration) {
		clock.advance(d)
		st := procStat([2]uint64{100, 100})
		st.Data = fmt.Appendf(nil, "cpu  %d 0 0 %d 0 0 0 0 0 0\ncpu0 %[1]d 0 0 %[2]d 0 0 0 0 0 0\n",
			100+clock.t.Sub(fixedNow).Milliseconds(), 100+clock.t.Sub(fixedNow).Milliseconds())
		fsys["proc/stat"] = st
	}
	over := func(l Live) string {
		if l.RatesOverSeconds == nil {
			return "null"
		}
		return fmt.Sprint(*l.RatesOverSeconds)
	}

	c.Read(context.Background()) // page, 0 s
	tick(3 * time.Second)
	if got := over(c.Read(context.Background()).Live); got != "3" {
		t.Errorf("page at 3 s: rates over %s, want 3", got)
	}
	tick(1500 * time.Millisecond)
	if got := over(c.sampleLive()); got != "null" {
		t.Errorf("sampler's first read at 4.5 s: rates over %s, want null -- it has no baseline of its own yet", got)
	}
	tick(1500 * time.Millisecond)
	if got := over(c.Read(context.Background()).Live); got != "3" {
		t.Errorf("page at 6 s: rates over %s, want 3 -- the sampler's read at 4.5 s is not the page's baseline", got)
	}
	tick(13500 * time.Millisecond)
	sampled := c.sampleLive()
	if got := over(sampled); got != "15" {
		t.Errorf("sampler at 19.5 s: rates over %s, want 15 -- the page's reads are not the sampler's baseline", got)
	}
	if u := sampled.CPU.Usage; !u.Readable || u.Value == nil || *u.Value != 50 {
		t.Errorf("sampler at 19.5 s: usage = %+v, want 50 over its own window", u)
	}
}
