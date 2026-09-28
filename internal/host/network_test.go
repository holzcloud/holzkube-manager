package host

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
)

// The Network card (HMON-04): which interfaces are hardware, their state and
// speed, and their throughput computed from the collector's own memo (D-09,
// D-12).

// netFS wraps a fixture so a test can make chosen paths fail the way the
// kernel makes them fail, and see every path that was opened.
type netFS struct {
	fs.FS
	// failOpen: opening the path fails with this error.
	failOpen map[string]error
	// failRead: the path opens, and reading it fails with this error -- a
	// down link's speed, which the kernel refuses at read(2), not at open(2).
	failRead map[string]error
	opened   *[]string
}

func (f netFS) Open(name string) (fs.File, error) {
	if f.opened != nil {
		*f.opened = append(*f.opened, name)
	}
	if err := f.failOpen[name]; err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	file, err := f.FS.Open(name)
	if err != nil {
		return nil, err
	}
	// The open succeeded; the failure is the read's, and comes later.
	if readErr, fails := f.failRead[name]; fails {
		return failingRead{File: file, name: name, err: readErr}, nil
	}
	return file, nil
}

// Lstat and ReadLink pass through, so the wrapper does not change what
// fs.Lstat sees on a symlinked fixture.
func (f netFS) Lstat(name string) (fs.FileInfo, error) { return fs.Lstat(f.FS, name) }
func (f netFS) ReadLink(name string) (string, error)   { return fs.ReadLink(f.FS, name) }

type failingRead struct {
	fs.File
	name string
	err  error
}

func (f failingRead) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: f.name, Err: f.err}
}

func intp(n int) *int           { return &n }
func floatp(f float64) *float64 { return &f }
func boolp(b bool) *bool         { return &b }
func link(name string, up bool, speed *int) Link {
	return Link{Name: name, Up: &up, SpeedMbit: speed}
}

func readNetwork(t *testing.T, c *Collector) (Network, *float64) {
	t.Helper()
	live := c.Read(context.Background()).Live
	return mustRead(t, "network", live.Network), live.RatesOverSeconds
}

func wantNetwork(t *testing.T, step string, got, want Network) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\n got %s\nwant %s", step, showNetwork(got), showNetwork(want))
	}
}

func showNetwork(n Network) string {
	show := func(ls []Link) string {
		var b strings.Builder
		for _, l := range ls {
			fmt.Fprintf(&b, "[%s up=%s speed=%s rx=%s tx=%s]", l.Name, showPtr(l.Up), showPtr(l.SpeedMbit), showPtr(l.RxBytesPerSec), showPtr(l.TxBytesPerSec))
		}
		return b.String()
	}
	return "physical " + show(n.Physical) + " virtual " + show(n.Virtual)
}

func showPtr[T any](p *T) string {
	if p == nil {
		return "null"
	}
	return fmt.Sprint(*p)
}

// pi5Network is the Pi 5's interfaces on a first read: no rates yet.
func pi5Network(docker0Speed *int) Network {
	return Network{
		Physical: []Link{link("eth0", true, intp(1000)), link("wlan0", false, nil)},
		Virtual: []Link{
			link("br-0a1b2c3d4e5f", true, intp(10000)),
			link("docker0", true, docker0Speed),
			// operstate "unknown", flags 0x9: IFF_UP, so up (WR-02).
			link("lo", true, nil),
			link("veth1a2b3c4", true, intp(10000)),
			link("veth5d6e7f8", true, intp(10000)),
		},
	}
}

// TestNetworkPi5 is the Pi 5's /sys/class/net as the kernel lays it out: every
// entry a symlink into /sys/devices, eth0 and wlan0 with a device link, the
// loopback, the docker bridges and the veths without one. (The wlan0 device
// path is the real one with its colons replaced, which a Go module cannot
// carry in a file name.)
func TestNetworkPi5(t *testing.T) {
	t.Parallel()

	var opened []string
	fsys := netFS{FS: fixtureFS(t, "pi5"), opened: &opened}
	c := New(Config{FS: fsys, Sys: fakeSys{loads: sysinfoLoads}, Now: newLiveClock().now})
	v := c.Read(context.Background())

	wantNetwork(t, "first read", mustRead(t, "network", v.Live.Network), pi5Network(intp(10000)))

	// On the wire: every rate is null, not 0, and so is wlan0's speed.
	wire := marshalView(t, v)
	live, _ := wire["live"].(map[string]any)
	netw, _ := live["network"].(map[string]any)
	value, _ := netw["value"].(map[string]any)
	physical, _ := value["physical"].([]any)
	if len(physical) != 2 {
		t.Fatalf("physical on the wire = %v", value["physical"])
	}
	for _, x := range physical {
		l, _ := x.(map[string]any)
		for _, key := range []string{"rx_bytes_per_sec", "tx_bytes_per_sec"} {
			if r, has := l[key]; !has || r != nil {
				t.Errorf("%v: %s = %v (present %v), want null on the first read", l["name"], key, r, has)
			}
		}
	}
	if w, _ := physical[1].(map[string]any); w["speed_mbit"] != nil {
		t.Errorf("wlan0 speed_mbit = %v, want null", w["speed_mbit"])
	}

	// T-11-19: no MAC address is ever read.
	for _, name := range opened {
		if strings.HasSuffix(name, "/address") {
			t.Errorf("read %s: the address file carries the MAC and must never be opened", name)
		}
	}
}

// TestNetworkSpeedEINVAL: a down link answers the speed read with EINVAL
// (measured on the Pi 5's wlan0 and lo). That is "no speed", not a failure of
// the section -- and not the file's content either, as docker0 shows.
func TestNetworkSpeedEINVAL(t *testing.T) {
	t.Parallel()

	fsys := netFS{FS: fixtureFS(t, "pi5"), failRead: map[string]error{
		"sys/class/net/wlan0/speed":   syscall.EINVAL,
		"sys/class/net/lo/speed":      syscall.EINVAL,
		"sys/class/net/docker0/speed": syscall.EINVAL,
	}}
	c := New(Config{FS: fsys, Sys: fakeSys{loads: sysinfoLoads}, Now: newLiveClock().now})
	got, _ := readNetwork(t, c)
	wantNetwork(t, "speed reads failing with EINVAL", got, pi5Network(nil))
}

// TestNetworkLinkState (WR-02): operstate "unknown" is the kernel not
// knowing, not the link being down. The loopback always says it, as do
// tun/WireGuard and some USB NICs; their administrative flag decides then. A
// state neither file gave is not claimed at all.
func TestNetworkLinkState(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name             string
		operstate, flags *string
		want             *bool
	}{
		{"up", strp("up\n"), strp("0x1003\n"), boolp(true)},
		{"down with IFF_UP set (no carrier)", strp("down\n"), strp("0x1003\n"), boolp(false)},
		{"dormant", strp("dormant\n"), strp("0x1003\n"), boolp(false)},
		{"lowerlayerdown", strp("lowerlayerdown\n"), strp("0x1003\n"), boolp(false)},
		{"unknown, IFF_UP set (loopback)", strp("unknown\n"), strp("0x9\n"), boolp(true)},
		{"unknown, IFF_UP set (wireguard)", strp("unknown\n"), strp("0x91\n"), boolp(true)},
		{"unknown, IFF_UP clear", strp("unknown\n"), strp("0x1002\n"), boolp(false)},
		{"unknown, no flags", strp("unknown\n"), nil, nil},
		{"unknown, flags unparsable", strp("unknown\n"), strp("up\n"), nil},
		{"operstate unreadable, IFF_UP set", nil, strp("0x1003\n"), boolp(true)},
		{"operstate empty, IFF_UP set", strp("\n"), strp("0x91\n"), boolp(true)},
		{"neither readable", nil, nil, nil},
	} {
		fsys := fstest.MapFS{"sys/class/net/x0/statistics/rx_bytes": {Data: []byte("0\n")}}
		if tc.operstate != nil {
			fsys["sys/class/net/x0/operstate"] = &fstest.MapFile{Data: []byte(*tc.operstate)}
		}
		if tc.flags != nil {
			fsys["sys/class/net/x0/flags"] = &fstest.MapFile{Data: []byte(*tc.flags)}
		}
		got, _ := readNetwork(t, New(Config{FS: fsys, Sys: fakeSys{}, Now: newLiveClock().now}))
		if len(got.Virtual) != 1 || !reflect.DeepEqual(got.Virtual[0].Up, tc.want) {
			t.Errorf("%s: got %s, want up=%s", tc.name, showNetwork(got), showPtr(tc.want))
		}
	}

	// On the wire an unknown state is null, never false and never absent.
	fsys := fstest.MapFS{"sys/class/net/x0/statistics/rx_bytes": {Data: []byte("0\n")}}
	wire := marshalView(t, New(Config{FS: fsys, Sys: fakeSys{}, Now: newLiveClock().now}).Read(context.Background()))
	live, _ := wire["live"].(map[string]any)
	netw, _ := live["network"].(map[string]any)
	value, _ := netw["value"].(map[string]any)
	virtual, _ := value["virtual"].([]any)
	if len(virtual) != 1 {
		t.Fatalf("virtual on the wire = %v", value["virtual"])
	}
	if l, _ := virtual[0].(map[string]any); l["up"] != nil {
		t.Errorf("up = %v, want null", l["up"])
	} else if _, has := l["up"]; !has {
		t.Errorf("up is absent, want null")
	}
}

func strp(s string) *string { return &s }

func TestNetworkSpeedValues(t *testing.T) {
	t.Parallel()

	for raw, want := range map[string]*int{"1000\n": intp(1000), "2500": intp(2500), "-1\n": nil, "0\n": nil, "fast\n": nil, "": nil} {
		fsys := fstest.MapFS{
			"sys/class/net/eth0/device/vendor": {Data: []byte("0x14e4\n")},
			"sys/class/net/eth0/operstate":     {Data: []byte("up\n")},
			"sys/class/net/eth0/speed":         {Data: []byte(raw)},
		}
		got, _ := readNetwork(t, New(Config{FS: fsys, Sys: fakeSys{}, Now: newLiveClock().now}))
		if len(got.Physical) != 1 || !reflect.DeepEqual(got.Physical[0].SpeedMbit, want) {
			t.Errorf("speed %q: got %s, want speed %s", raw, showNetwork(got), showPtr(want))
		}
	}
}

// netDir writes one interface's files into a MapFS.
func netDir(fsys fstest.MapFS, name string, physical bool, rx, tx uint64) {
	dir := "sys/class/net/" + name + "/"
	if physical {
		fsys[dir+"device/vendor"] = &fstest.MapFile{Data: []byte("0x14e4\n")}
	}
	fsys[dir+"operstate"] = &fstest.MapFile{Data: []byte("up\n")}
	fsys[dir+"speed"] = &fstest.MapFile{Data: []byte("1000\n")}
	fsys[dir+"statistics/rx_bytes"] = &fstest.MapFile{Data: fmt.Appendf(nil, "%d\n", rx)}
	fsys[dir+"statistics/tx_bytes"] = &fstest.MapFile{Data: fmt.Appendf(nil, "%d\n", tx)}
}

func rated(name string, rx, tx *float64) Link {
	return Link{Name: name, Up: boolp(true), SpeedMbit: intp(1000), RxBytesPerSec: rx, TxBytesPerSec: tx}
}

// TestNetworkRates is the memo for links (D-12): the same window and the same
// advance rule as the CPU, and null -- never 0 -- where there is no rate.
func TestNetworkRates(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{}
	clock := newLiveClock()
	c := New(Config{FS: fsys, Sys: fakeSys{loads: sysinfoLoads}, Now: clock.now})

	// 1: no previous reading.
	netDir(fsys, "eth0", true, 1_000_000, 500_000)
	netDir(fsys, "lo", false, 9_000, 9_000)
	got, over := readNetwork(t, c)
	wantNetwork(t, "first read", got, Network{
		Physical: []Link{rated("eth0", nil, nil)},
		Virtual:  []Link{rated("lo", nil, nil)},
	})
	if over != nil {
		t.Errorf("first read: rates_over_seconds = %v, want null", *over)
	}

	// 2: 3 s later. eth0 moved 300000 in and 30000 out; lo's counter went
	// backwards (the interface was recreated); veth0 is new.
	clock.advance(3 * time.Second)
	netDir(fsys, "eth0", true, 1_300_000, 530_000)
	netDir(fsys, "lo", false, 100, 9_500)
	netDir(fsys, "veth0", false, 50, 50)
	got, over = readNetwork(t, c)
	wantNetwork(t, "3 s later", got, Network{
		Physical: []Link{rated("eth0", floatp(100_000), floatp(10_000))},
		Virtual:  []Link{rated("lo", nil, nil), rated("veth0", nil, nil)},
	})
	if over == nil || *over != 3.0 {
		t.Errorf("3 s later: rates_over_seconds = %v, want 3", showPtr(over))
	}

	// 3: 0.1 s after that -- a second tab. No rate, and read 2 stays the
	// baseline.
	clock.advance(100 * time.Millisecond)
	netDir(fsys, "eth0", true, 1_590_000, 560_000)
	got, _ = readNetwork(t, c)
	wantNetwork(t, "0.1 s later", got, Network{
		Physical: []Link{rated("eth0", nil, nil)},
		Virtual:  []Link{rated("lo", nil, nil), rated("veth0", nil, nil)},
	})

	// 4: 3 s after read 2. Against read 2: 300000 in, 30000 out over 3.0 s.
	// Against read 3 it would be 10000 over 2.9 s.
	clock.advance(2900 * time.Millisecond)
	netDir(fsys, "eth0", true, 1_600_000, 560_000)
	netDir(fsys, "lo", false, 400, 9_800)
	netDir(fsys, "veth0", false, 50, 350)
	got, _ = readNetwork(t, c)
	wantNetwork(t, "3 s after read 2", got, Network{
		Physical: []Link{rated("eth0", floatp(100_000), floatp(10_000))},
		Virtual:  []Link{rated("lo", floatp(100), floatp(100)), rated("veth0", floatp(0), floatp(100))},
	})
}

// slowWalkFS makes the data directory's size walk take walk on the test's
// clock: opening the directory moves the clock, as a walk over an SD card
// moves the real one.
type slowWalkFS struct {
	fs.FS
	dir   string
	clock *liveClock
	walk  time.Duration
}

func (f slowWalkFS) Open(name string) (fs.File, error) {
	if name == f.dir {
		f.clock.advance(f.walk)
	}
	return f.FS.Open(name)
}

// TestNetworkRateWindowExcludesTheSizeWalk (WR-01): the counters' window is
// the time between the two counter reads. The data directory walk runs
// before the counters are read, once a minute, for up to 5 s; stamping the
// counters with the request's start put that walk inside the window of one
// poll and outside the next, so a link moving 100000 B/s read as 60000 here.
func TestNetworkRateWindowExcludesTheSizeWalk(t *testing.T) {
	t.Parallel()

	mapFS := fstest.MapFS{"srv/hkm/state.json": &fstest.MapFile{Data: []byte("{}")}}
	clock := newLiveClock()
	fsys := slowWalkFS{FS: mapFS, dir: "srv/hkm", clock: clock, walk: 2 * time.Second}
	c := New(Config{FS: fsys, Sys: fakeSys{loads: sysinfoLoads}, Now: clock.now, DataDir: "/srv/hkm"})

	// 1: this read walks the data directory, which takes 2 s.
	netDir(mapFS, "eth0", true, 1_000_000, 500_000)
	readNetwork(t, c)

	// 2: 3 s after the counters were read, inside the walk's minute: no walk.
	clock.advance(3 * time.Second)
	netDir(mapFS, "eth0", true, 1_300_000, 530_000)
	got, over := readNetwork(t, c)
	wantNetwork(t, "3 s after the counters", got, Network{
		Physical: []Link{rated("eth0", floatp(100_000), floatp(10_000))},
		Virtual:  []Link{},
	})
	if over == nil || *over != 3.0 {
		t.Errorf("rates_over_seconds = %v, want 3 -- the walk is not part of the window", showPtr(over))
	}
}

// TestNetworkUnreadable: a class directory that exists and cannot be listed is
// a reading with a reason -- not an empty list, which would say "no
// interfaces" about a machine nobody could look at.
func TestNetworkUnreadable(t *testing.T) {
	t.Parallel()

	fsys := netFS{FS: fstest.MapFS{}, failOpen: map[string]error{"sys/class/net": fs.ErrPermission}}
	netw := New(Config{FS: fsys, Sys: fakeSys{}, Now: newLiveClock().now}).Read(context.Background()).Live.Network
	assertHidden(t, "network", netw.Readable, netw.Value == nil, netw.Reason, CodeReadFailed)
	if r := netw.Reason; r != nil && !strings.Contains(r.Message, "/sys/class/net") {
		t.Errorf("reason %q does not name /sys/class/net", r.Message)
	}
}

// TestNetworkAbsent: a machine without /sys/class/net has no interfaces, and
// says so with two empty lists.
func TestNetworkAbsent(t *testing.T) {
	t.Parallel()

	v := New(Config{FS: fstest.MapFS{}, Sys: fakeSys{}, Now: newLiveClock().now}).Read(context.Background())
	got := mustRead(t, "network", v.Live.Network)
	if got.Physical == nil || got.Virtual == nil || len(got.Physical)+len(got.Virtual) != 0 {
		t.Errorf("network = %#v, want two empty, non-nil lists", got)
	}
	raw, err := json.Marshal(v.Live.Network)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"readable":true,"value":{"physical":[],"virtual":[]}}`; string(raw) != want {
		t.Errorf("wire = %s, want %s", raw, want)
	}
}

// TestLinkKeysMatchInventory is D-13: the host's link and the node page's link
// are the same JSON object, apart from which of its numbers may be null.
func TestLinkKeysMatchInventory(t *testing.T) {
	t.Parallel()

	keys := func(v any) []string {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		return slices.Sorted(maps.Keys(m))
	}
	host, node := keys(Link{}), keys(inventory.HardwareLink{})
	if !slices.Equal(host, node) {
		t.Errorf("host.Link keys %v, inventory.HardwareLink keys %v", host, node)
	}
}
