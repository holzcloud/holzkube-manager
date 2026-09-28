package host

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
)

// The Filesystems card (HMON-02, D-07): / and the data directory's
// filesystem, one row when they are one, numbers equal to df's.

// piRootStatfs is statfs("/") as measured on the Pi, beside the line
// `df -B1 /` printed at the same moment:
//
//	125260451840 28882735104 91212472320 25% /
var piRootStatfs = FSStats{FrameSize: 4096, Blocks: 30581165, Free: 23529716, Avail: 22268670}

const piDataDir = "/var/lib/holzkube-manager"

func mountsOf(t *testing.T, raw string) []mountEntry {
	t.Helper()
	ms, err := parseMountinfo([]byte(raw))
	if err != nil {
		t.Fatalf("parse mountinfo: %v", err)
	}
	return ms
}

func fixtureMounts(t *testing.T, name string) []mountEntry {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name, "proc", "self", "mountinfo"))
	if err != nil {
		t.Fatalf("read fixture mountinfo: %v", err)
	}
	return mountsOf(t, string(raw))
}

// TestFilesystemsDedupeByDevice is the production unit's shape: systemd has
// bind-mounted the data directory over itself, so mountinfo has a second line
// for it -- on the same device as /. That is one filesystem and one row.
func TestFilesystemsDedupeByDevice(t *testing.T) {
	t.Parallel()

	sys := fakeSys{statfs: map[string]FSStats{"/": piRootStatfs, piDataDir: piRootStatfs}}
	rows := filesystems(fixtureMounts(t, "procsubset-pid"), piDataDir, sys)

	if len(rows) != 1 {
		t.Fatalf("got %d rows, want exactly 1 for one filesystem: %+v", len(rows), rows)
	}
	r := rows[0]
	if r.Mount != "/" || r.Device != "/dev/mmcblk0p2" || r.FSType != "ext4" {
		t.Errorf("row = mount %q device %q fstype %q, want / /dev/mmcblk0p2 ext4", r.Mount, r.Device, r.FSType)
	}
	if want := []string{"root", "data directory"}; !slices.Equal(r.Roles, want) {
		t.Errorf("roles = %q, want %q", r.Roles, want)
	}
	if u := mustRead(t, "usage", r.Usage); u.UsedBytes != 28882735104 {
		t.Errorf("used = %d, want 28882735104", u.UsedBytes)
	}

	// And through the whole Collector, from the fixture tree.
	v := New(Config{FS: fixtureFS(t, "procsubset-pid"), Sys: sys, Now: newLiveClock().now, DataDir: piDataDir}).Read(context.Background())
	if got := len(v.Live.Filesystems); got != 1 {
		t.Errorf("the answer carries %d filesystem rows, want 1: %+v", got, v.Live.Filesystems)
	}
}

func TestFilesystemsOnTwoDevices(t *testing.T) {
	t.Parallel()

	mountinfo := "22 1 179:2 / / rw,noatime shared:1 - ext4 /dev/mmcblk0p2 rw\n" +
		"41 22 259:1 / /srv rw,noatime shared:41 - ext4 /dev/nvme0n1p1 rw\n"
	fsys := fstest.MapFS{"proc/self/mountinfo": {Data: []byte(mountinfo)}}
	nvme := FSStats{FrameSize: 4096, Blocks: 1000, Free: 400, Avail: 350}
	sys := fakeSys{statfs: map[string]FSStats{"/": piRootStatfs, "/srv/holzkube-manager": nvme}}

	rows := New(Config{FS: fsys, Sys: sys, Now: newLiveClock().now, DataDir: "/srv/holzkube-manager"}).Read(context.Background()).Live.Filesystems

	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(rows), rows)
	}
	if rows[0].Mount != "/" || !slices.Equal(rows[0].Roles, []string{"root"}) {
		t.Errorf("first row = %q %q, want / [root]", rows[0].Mount, rows[0].Roles)
	}
	if rows[1].Mount != "/srv" || rows[1].Device != "/dev/nvme0n1p1" || !slices.Equal(rows[1].Roles, []string{"data directory"}) {
		t.Errorf("second row = %q %q %q, want /srv /dev/nvme0n1p1 [data directory]", rows[1].Mount, rows[1].Device, rows[1].Roles)
	}
	if u := mustRead(t, "data usage", rows[1].Usage); u.SizeBytes != 1000*4096 {
		t.Errorf("data size = %d, want %d", u.SizeBytes, 1000*4096)
	}
}

// TestFilesystemsPrefixBoundary: /var/lib/holzkube is a mount, and
// /var/lib/holzkube-manager is not under it.
func TestFilesystemsPrefixBoundary(t *testing.T) {
	t.Parallel()

	ms := mountsOf(t, "22 1 179:2 / / rw,noatime shared:1 - ext4 /dev/mmcblk0p2 rw\n"+
		"50 22 259:1 / /var/lib/holzkube rw shared:50 - xfs /dev/sda1 rw\n")
	sys := fakeSys{statfs: map[string]FSStats{"/": piRootStatfs, piDataDir: piRootStatfs}}

	rows := filesystems(ms, piDataDir, sys)
	if len(rows) != 1 || !slices.Equal(rows[0].Roles, []string{"root", "data directory"}) {
		t.Errorf("rows = %+v, want one merged row: /var/lib/holzkube does not hold %s", rows, piDataDir)
	}
}

// TestStatfsMatchesDf is df -B1's arithmetic, against the line df printed.
func TestStatfsMatchesDf(t *testing.T) {
	t.Parallel()

	u := mustRead(t, "usage", fsUsage(fakeSys{statfs: map[string]FSStats{"/": piRootStatfs}}, "/"))
	want := FSUsage{SizeBytes: 125260451840, UsedBytes: 28882735104, AvailableBytes: 91212472320}
	if u != want {
		t.Errorf("usage = %+v, want df's %+v", u, want)
	}
}

func TestFilesystemStatfsFails(t *testing.T) {
	t.Parallel()

	ms := mountsOf(t, "22 1 179:2 / / rw shared:1 - ext4 /dev/mmcblk0p2 rw\n"+
		"41 22 259:1 / /srv rw shared:41 - ext4 /dev/nvme0n1p1 rw\n")
	sys := fakeSys{
		statfs:    map[string]FSStats{"/": piRootStatfs},
		statfsErr: map[string]error{"/srv/data": errors.New("permission denied")},
	}

	rows := filesystems(ms, "/srv/data", sys)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	mustRead(t, "root usage", rows[0].Usage)
	if rows[1].Usage.Readable || rows[1].Usage.Value != nil || rows[1].Usage.Reason == nil || rows[1].Usage.Reason.Code != CodeReadFailed {
		t.Errorf("data row usage = %+v, want not readable with read-failed", rows[1].Usage)
	}
	if rows[1].Usage.Reason != nil && !strings.Contains(rows[1].Usage.Reason.Message, "permission denied") {
		t.Errorf("reason %q does not carry the cause", rows[1].Usage.Reason.Message)
	}
}

// TestFilesystemsWithoutMountinfo: nothing proves the two paths share a
// filesystem, so neither a merge nor a device is claimed.
func TestFilesystemsWithoutMountinfo(t *testing.T) {
	t.Parallel()

	sys := fakeSys{statfs: map[string]FSStats{"/": piRootStatfs, piDataDir: piRootStatfs}}
	rows := New(Config{FS: fstest.MapFS{}, Sys: sys, Now: newLiveClock().now, DataDir: piDataDir}).Read(context.Background()).Live.Filesystems

	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 unmerged: %+v", len(rows), rows)
	}
	for i, want := range []struct{ mount, role string }{{"/", "root"}, {piDataDir, "data directory"}} {
		r := rows[i]
		if r.Mount != want.mount || r.Device != "" || r.FSType != "" || !slices.Equal(r.Roles, []string{want.role}) {
			t.Errorf("row %d = %+v, want mount %s, no device, no type, roles [%s]", i, r, want.mount, want.role)
		}
	}

	// And without a data directory, only the root row -- with a roles list,
	// not null.
	only := New(Config{FS: fstest.MapFS{}, Sys: sys, Now: newLiveClock().now}).Read(context.Background())
	if len(only.Live.Filesystems) != 1 || only.Live.Filesystems[0].Roles == nil {
		t.Errorf("without a data dir: %+v, want one root row with roles", only.Live.Filesystems)
	}
}

// TestFilesystemKeysCoverInventory is D-13: a host filesystem row says
// everything a node's does, under the same names.
func TestFilesystemKeysCoverInventory(t *testing.T) {
	t.Parallel()

	have := map[string]bool{}
	for _, typ := range []reflect.Type{reflect.TypeFor[Filesystem](), reflect.TypeFor[FSUsage]()} {
		for i := range typ.NumField() {
			have[jsonKey(typ.Field(i))] = true
		}
	}
	inv := reflect.TypeFor[inventory.HardwareFilesystem]()
	n := inv.NumField()
	for i := range n {
		if k := jsonKey(inv.Field(i)); !have[k] {
			t.Errorf("inventory.HardwareFilesystem has %q; the host row does not", k)
		}
	}
	if n == 0 {
		t.Fatal("inventory.HardwareFilesystem has no fields, so this test proves nothing")
	}
}

func jsonKey(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	return name
}

// TestDataDirSize is du -s -B1 on a real tree: files, a hard link counted
// once, a subdirectory, and a symlink to a large file outside that must not be
// followed.
func TestDataDirSize(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("statBlocks reads syscall.Stat_t on linux only")
	}
	if _, err := exec.LookPath("du"); err != nil {
		t.Skip("no du to compare with")
	}

	outside := t.TempDir()
	big := filepath.Join(outside, "big")
	writeFile(t, big, 1<<20)

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a"), 5000)
	writeFile(t, filepath.Join(dir, "sub", "b"), 70000)
	writeFile(t, filepath.Join(dir, "sub", "deeper", "c"), 300000)
	if err := os.Link(filepath.Join(dir, "sub", "deeper", "c"), filepath.Join(dir, "c-again")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(big, filepath.Join(dir, "link-to-big")); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command("du", "-s", "-B1", dir).Output()
	if err != nil {
		t.Fatalf("du: %v", err)
	}
	field, _, _ := strings.Cut(string(out), "\t")
	want, err := strconv.ParseUint(strings.TrimSpace(field), 10, 64)
	if err != nil {
		t.Fatalf("du printed %q: %v", out, err)
	}

	c := New(Config{FS: os.DirFS("/"), Sys: fakeSys{}, Now: newLiveClock().now, DataDir: dir})
	got := mustRead(t, "size", c.sizer.size(context.Background(), fixedNow))
	if got.Bytes != want {
		t.Errorf("size = %d, du -s -B1 says %d", got.Bytes, want)
	}
	if !got.MeasuredAt.Equal(fixedNow) || got.MeasuredAt.Location() != time.UTC {
		t.Errorf("measured_at = %v, want %v in UTC", got.MeasuredAt, fixedNow.UTC())
	}
}

// TestDataDirSizeIsCached: one walk answers for 60 s, then the next read walks
// again (D-07, T-11-14).
func TestDataDirSizeIsCached(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("statBlocks reads syscall.Stat_t on linux only")
	}

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "state.json"), 1000)

	clock := newLiveClock()
	c := New(Config{FS: os.DirFS("/"), Sys: fakeSys{}, Now: clock.now, DataDir: dir})
	first := mustRead(t, "first", c.sizer.size(context.Background(), clock.now()))
	firstAt := clock.now()

	writeFile(t, filepath.Join(dir, "new"), 1<<20)

	clock.advance(30 * time.Second)
	cached := mustRead(t, "at 30 s", c.sizer.size(context.Background(), clock.now()))
	if cached.Bytes != first.Bytes || !cached.MeasuredAt.Equal(firstAt) {
		t.Errorf("at 30 s: %+v, want the first walk's %+v -- the walk ran again inside 60 s", cached, first)
	}

	clock.advance(31 * time.Second)
	again := mustRead(t, "at 61 s", c.sizer.size(context.Background(), clock.now()))
	if again.Bytes < first.Bytes+1<<20 || !again.MeasuredAt.Equal(clock.now()) {
		t.Errorf("at 61 s: %+v, want a new walk with the new MiB (first was %d bytes)", again, first.Bytes)
	}
}

// TestDataDirSizeFails: a data directory that is not there is not readable,
// with the path in the sentence -- and not a 0.
func TestDataDirSizeFails(t *testing.T) {
	t.Parallel()

	c := New(Config{FS: fstest.MapFS{}, Sys: fakeSys{}, Now: newLiveClock().now, DataDir: piDataDir})
	r := c.sizer.size(context.Background(), fixedNow)
	if r.Readable || r.Value != nil || r.Reason == nil || !strings.Contains(r.Reason.Message, piDataDir) {
		t.Errorf("size = %+v, want not readable naming %s", r, piDataDir)
	}
}

// writeFile writes n non-zero bytes, so the file has blocks and is not sparse.
func writeFile(t *testing.T, p string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Repeat("x", n)), 0o600); err != nil {
		t.Fatal(err)
	}
}
