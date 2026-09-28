package host

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
)

// The Live section: CPU, load, memory and swap (HMON-01), and what the page is
// told when the unit's ProcSubset=pid hides their files (HMON-06, D-02, D-03).

// liveClock is a clock a test moves by hand. The collector reads it through a
// closure, so a test decides exactly how far apart two reads are.
type liveClock struct{ t time.Time }

func (c *liveClock) now() time.Time          { return c.t }
func (c *liveClock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newLiveClock() *liveClock               { return &liveClock{t: fixedNow} }

// sysinfoLoads are sysinfo(2)'s raw loads at the measured instant, when
// /proc/loadavg said 7.29 4.90 2.98.
var sysinfoLoads = Loads{One: 477760, Five: 321152, Fifteen: 195328}

func TestHardeningHidesCPUAndMemory(t *testing.T) {
	t.Parallel()

	clock := newLiveClock()
	c := New(Config{FS: fixtureFS(t, "procsubset-pid"), Sys: fakeSys{loads: sysinfoLoads}, Now: clock.now})

	// Twice, 3 s apart: the second read would have a baseline if the file
	// were there, and the hardening must still be the reason given.
	for round := range 2 {
		v := c.Read(context.Background())
		live := v.Live

		assertHidden(t, fmt.Sprintf("round %d usage", round), live.CPU.Usage.Readable, live.CPU.Usage.Value == nil, live.CPU.Usage.Reason, CodeProcSubset)
		assertHidden(t, fmt.Sprintf("round %d per_core", round), live.CPU.PerCore.Readable, live.CPU.PerCore.Value == nil, live.CPU.PerCore.Reason, CodeProcSubset)
		assertHidden(t, fmt.Sprintf("round %d memory", round), live.Memory.Readable, live.Memory.Value == nil, live.Memory.Reason, CodeProcSubset)

		for name, r := range map[string]*Reason{"usage": live.CPU.Usage.Reason, "memory": live.Memory.Reason} {
			if r == nil {
				continue
			}
			for _, must := range []string{"ProcSubset=pid", "ProcSubset=all"} {
				if !strings.Contains(r.Message, must) {
					t.Errorf("round %d %s: message %q does not name %s", round, name, r.Message, must)
				}
			}
		}

		load := mustRead(t, "load", live.CPU.Load)
		if want := (Load{Load1: 7.29, Load5: 4.9, Load15: 2.98, Source: "sysinfo"}); load != want {
			t.Errorf("round %d load = %+v, want %+v", round, load, want)
		}
		clock.advance(3 * time.Second)
	}
}

func assertHidden(t *testing.T, name string, readable, valueNil bool, reason *Reason, code string) {
	t.Helper()
	if readable || !valueNil {
		t.Errorf("%s: readable=%v, value present=%v; want not readable and no value", name, readable, !valueNil)
	}
	if reason == nil || reason.Code != code {
		t.Errorf("%s: reason = %+v, want code %s", name, reason, code)
	}
}

// TestHiddenValuesCarryNoNumber is D-02 on the wire, for the production unit's
// shape: nothing under the hidden readings is a number, and none of them has a
// value key. A 0 put back anywhere in there is exactly what this catches.
func TestHiddenValuesCarryNoNumber(t *testing.T) {
	t.Parallel()

	c := New(Config{FS: fixtureFS(t, "procsubset-pid"), Sys: fakeSys{loads: sysinfoLoads}, Now: newLiveClock().now})
	wire := marshalView(t, c.Read(context.Background()))

	live, _ := wire["live"].(map[string]any)
	cpu, _ := live["cpu"].(map[string]any)
	for path, sub := range map[string]any{
		"live.cpu.usage":    cpu["usage"],
		"live.cpu.per_core": cpu["per_core"],
		"live.memory":       live["memory"],
	} {
		if sub == nil {
			t.Errorf("%s is missing from the answer", path)
			continue
		}
		walkAny(sub, path, func(p string, node any) {
			if _, isNumber := node.(float64); isNumber {
				t.Errorf("%s: a number (%v) under a reading that was not read", p, node)
			}
			if obj, ok := node.(map[string]any); ok {
				if _, has := obj["value"]; has {
					t.Errorf("%s: a value key under a reading that was not read: %v", p, obj)
				}
			}
		})
	}
}

// walkAny visits every node of a decoded JSON tree.
func walkAny(node any, path string, visit func(path string, node any)) {
	visit(path, node)
	switch n := node.(type) {
	case map[string]any:
		for k, child := range n {
			walkAny(child, path+"."+k, visit)
		}
	case []any:
		for i, child := range n {
			walkAny(child, fmt.Sprintf("%s[%d]", path, i), visit)
		}
	}
}

func TestLivePi5FirstRead(t *testing.T) {
	t.Parallel()

	c := New(Config{FS: fixtureFS(t, "pi5"), Sys: fakeSys{loads: Loads{One: 1 << 16}}, Now: newLiveClock().now})
	v := c.Read(context.Background())
	live := v.Live

	assertHidden(t, "usage", live.CPU.Usage.Readable, live.CPU.Usage.Value == nil, live.CPU.Usage.Reason, CodeNoBaseline)
	assertHidden(t, "per_core", live.CPU.PerCore.Readable, live.CPU.PerCore.Value == nil, live.CPU.PerCore.Reason, CodeNoBaseline)
	if r := live.CPU.Usage.Reason; r != nil && r.Message != "Waiting for a second reading" {
		t.Errorf("no-baseline message = %q", r.Message)
	}
	if live.RatesOverSeconds != nil {
		t.Errorf("rates_over_seconds = %v on the first read, want null", *live.RatesOverSeconds)
	}

	mem := mustRead(t, "memory", live.Memory)
	want := inventory.HardwareMemory{
		TotalBytes: 8453947392, UsedBytes: 4093853696, CacheBytes: 3259285504,
		AvailableBytes: 4360093696, SwapTotalBytes: 2147467264, SwapUsedBytes: 1498169344,
	}
	if mem != want {
		t.Errorf("memory = %+v\n     want %+v (free -b on the measured sample)", mem, want)
	}

	// /proc/loadavg is there, so it is the source -- not the fake sysinfo,
	// which says 1.00.
	if load := mustRead(t, "load", live.CPU.Load); load != (Load{Load1: 7.29, Load5: 4.9, Load15: 2.98, Source: "loadavg"}) {
		t.Errorf("load = %+v, want /proc/loadavg's 7.29 4.90 2.98", load)
	}

	wire := marshalView(t, v)
	liveWire, _ := wire["live"].(map[string]any)
	if rate, has := liveWire["rates_over_seconds"]; !has || rate != nil {
		t.Errorf("rates_over_seconds on the wire = %v (present %v), want null", rate, has)
	}
}

// procStat writes a /proc/stat whose total is the sum of its cores. Each core
// is {user, idle} in ticks; every other column is 0.
func procStat(cores ...[2]uint64) *fstest.MapFile {
	var user, idle uint64
	var b strings.Builder
	for i, c := range cores {
		user += c[0]
		idle += c[1]
		fmt.Fprintf(&b, "cpu%d %d 0 0 %d 0 0 0 0 0 0\n", i, c[0], c[1])
	}
	return &fstest.MapFile{Data: []byte(fmt.Sprintf("cpu  %d 0 0 %d 0 0 0 0 0 0\n%sbtime 1790000000\n", user, idle, b.String()))}
}

// liveWire is the live section of one read as the browser receives it.
func liveWire(t *testing.T, c *Collector) map[string]any {
	t.Helper()
	live, _ := marshalView(t, c.Read(context.Background()))["live"].(map[string]any)
	return live
}

// cpuWire is live.cpu.<key> on the wire.
func cpuWire(live map[string]any, key string) map[string]any {
	cpu, _ := live["cpu"].(map[string]any)
	r, _ := cpu[key].(map[string]any)
	return r
}

func wantNoBaseline(t *testing.T, step string, live map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		r := cpuWire(live, key)
		reason, _ := r["reason"].(map[string]any)
		if r["readable"] != false || reason["code"] != CodeNoBaseline {
			t.Errorf("%s: %s = %v, want rate.no-baseline", step, key, r)
		}
		if _, has := r["value"]; has {
			t.Errorf("%s: %s carries a value without a baseline: %v", step, key, r)
		}
	}
	if rate, has := live["rates_over_seconds"]; !has || rate != nil {
		t.Errorf("%s: rates_over_seconds = %v, want null", step, rate)
	}
}

func wantRates(t *testing.T, step string, live map[string]any, usage float64, perCore []float64, over float64) {
	t.Helper()
	u := cpuWire(live, "usage")
	if u["readable"] != true || u["value"] != usage {
		t.Errorf("%s: usage = %v, want readable %v", step, u, usage)
	}
	pc := cpuWire(live, "per_core")
	got, _ := pc["value"].([]any)
	var gotF []float64
	for _, x := range got {
		f, _ := x.(float64)
		gotF = append(gotF, f)
	}
	if pc["readable"] != true || len(got) != len(perCore) || !slices.Equal(gotF, perCore) {
		t.Errorf("%s: per_core = %v, want readable %v with every value present", step, pc, perCore)
	}
	if live["rates_over_seconds"] != over {
		t.Errorf("%s: rates_over_seconds = %v, want %v", step, live["rates_over_seconds"], over)
	}
}

// TestRates is the rate memo (D-12): a baseline is used only inside its window,
// and a read too soon after the last one neither answers nor replaces it.
func TestRates(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{}
	clock := newLiveClock()
	c := New(Config{FS: fsys, Sys: fakeSys{loads: sysinfoLoads}, Now: clock.now})

	// 1: no previous reading.
	fsys["proc/stat"] = procStat([2]uint64{250, 250}, [2]uint64{250, 250}, [2]uint64{250, 250}, [2]uint64{250, 250})
	wantNoBaseline(t, "first read", liveWire(t, c), "usage", "per_core")

	// 2: 3 s later, 300 ticks passed and 150 of them were idle: 50 %. Core 0
	// was busy the whole time, the others idle -- and their 0 % is a reading.
	clock.advance(3 * time.Second)
	fsys["proc/stat"] = procStat([2]uint64{400, 250}, [2]uint64{250, 300}, [2]uint64{250, 300}, [2]uint64{250, 300})
	wantRates(t, "3 s later", liveWire(t, c), 50, []float64{100, 0, 0, 0}, 3.0)

	// 3: 0.1 s after that -- a second tab. No rate for this one, and the
	// baseline must stay the previous read's.
	clock.advance(100 * time.Millisecond)
	fsys["proc/stat"] = procStat([2]uint64{500, 250}, [2]uint64{250, 300}, [2]uint64{250, 300}, [2]uint64{250, 300})
	wantNoBaseline(t, "0.1 s later", liveWire(t, c), "usage", "per_core")

	// 4: 3 s after read 2. Against read 2: 400 ticks, 100 busy on core 0 ->
	// 25 %, core 0 at 100 %, over 3.0 s. Against read 3 it would be 0 % over 2.9 s.
	clock.advance(2900 * time.Millisecond)
	fsys["proc/stat"] = procStat([2]uint64{500, 250}, [2]uint64{250, 400}, [2]uint64{250, 400}, [2]uint64{250, 400})
	wantRates(t, "3 s after read 2", liveWire(t, c), 25, []float64{100, 0, 0, 0}, 3.0)

	// 5: an idle machine. 0 % is a reading and arrives as 0, not as nothing.
	clock.advance(3 * time.Second)
	fsys["proc/stat"] = procStat([2]uint64{500, 350}, [2]uint64{250, 500}, [2]uint64{250, 500}, [2]uint64{250, 500})
	wantRates(t, "idle", liveWire(t, c), 0, []float64{0, 0, 0, 0}, 3.0)

	// 6: six minutes later -- a baseline that old is not one.
	clock.advance(6 * time.Minute)
	fsys["proc/stat"] = procStat([2]uint64{900, 350}, [2]uint64{250, 900}, [2]uint64{250, 900}, [2]uint64{250, 900})
	wantNoBaseline(t, "6 min later", liveWire(t, c), "usage", "per_core")

	// 7: and 3 s after that, read 6 is the baseline again.
	clock.advance(3 * time.Second)
	fsys["proc/stat"] = procStat([2]uint64{1050, 350}, [2]uint64{250, 1050}, [2]uint64{250, 1050}, [2]uint64{250, 1050})
	wantRates(t, "3 s after the stale one", liveWire(t, c), 25, []float64{100, 0, 0, 0}, 3.0)
}

func TestCoreCountChange(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{}
	clock := newLiveClock()
	c := New(Config{FS: fsys, Sys: fakeSys{loads: sysinfoLoads}, Now: clock.now})

	fsys["proc/stat"] = procStat([2]uint64{100, 100}, [2]uint64{100, 100}, [2]uint64{100, 100}, [2]uint64{100, 100})
	c.Read(context.Background())

	// Two cores went offline. The total still pairs up; the cores do not.
	clock.advance(3 * time.Second)
	fsys["proc/stat"] = procStat([2]uint64{250, 250}, [2]uint64{250, 250})
	live := liveWire(t, c)

	u := cpuWire(live, "usage")
	if u["readable"] != true || u["value"] != 50.0 {
		t.Errorf("usage = %v, want readable 50 (400 ticks, 200 idle)", u)
	}
	pc := cpuWire(live, "per_core")
	reason, _ := pc["reason"].(map[string]any)
	if pc["readable"] != false || reason["code"] != CodeNoBaseline {
		t.Errorf("per_core = %v, want rate.no-baseline after the core count changed", pc)
	}
}

// TestLoadUnreadable: with neither /proc/loadavg nor sysinfo(2), load says why
// -- and it is the hardening only with the mount as proof.
func TestLoadUnreadable(t *testing.T) {
	t.Parallel()

	stat := procStat([2]uint64{100, 100})
	for _, tc := range []struct {
		name string
		fsys fstest.MapFS
		code string
	}{
		{"no mountinfo", fstest.MapFS{"proc/stat": stat}, CodeReadFailed},
		{"subset=pid mounted", fstest.MapFS{"proc/self/mountinfo": {Data: []byte(mountSystemd)}}, CodeProcSubset},
		{"plain /proc", fstest.MapFS{"proc/stat": stat, "proc/self/mountinfo": {Data: []byte(mountStackedLower)}}, CodeReadFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := New(Config{FS: tc.fsys, Sys: fakeSys{loadsErr: errors.New("function not implemented")}, Now: newLiveClock().now})
			load := c.Read(context.Background()).Live.CPU.Load
			assertHidden(t, "load", load.Readable, load.Value == nil, load.Reason, tc.code)
		})
	}
}

// TestMissingProcWithoutProofIsReadFailed is the other half of D-03 through the
// collector: /proc/stat and /proc/meminfo absent under a plain /proc mount are
// read failures, not the hardening.
func TestMissingProcWithoutProofIsReadFailed(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{"proc/self/mountinfo": {Data: []byte(mountStackedLower)}}
	live := New(Config{FS: fsys, Sys: fakeSys{loads: sysinfoLoads}, Now: newLiveClock().now}).Read(context.Background()).Live

	assertHidden(t, "usage", live.CPU.Usage.Readable, live.CPU.Usage.Value == nil, live.CPU.Usage.Reason, CodeReadFailed)
	assertHidden(t, "per_core", live.CPU.PerCore.Readable, live.CPU.PerCore.Value == nil, live.CPU.PerCore.Reason, CodeReadFailed)
	assertHidden(t, "memory", live.Memory.Readable, live.Memory.Value == nil, live.Memory.Reason, CodeReadFailed)
	if r := live.Memory.Reason; r != nil && !strings.Contains(r.Message, "/proc/meminfo") {
		t.Errorf("memory reason %q does not name /proc/meminfo", r.Message)
	}
}
