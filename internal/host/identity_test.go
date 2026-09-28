package host

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// The Device card's sources (HOST-01, D-04), read from fixture trees shaped
// like the two machines this product is known to run on: the operator's Pi 5
// and an amd64 desktop board. The fixture values are documentation values
// written by hand, never copied from a real /sys.

func identityCollector(fsys fs.FS, sys Sys) *Collector {
	return New(Config{FS: fsys, Sys: sys, Now: func() time.Time { return fixedNow }})
}

func mustRead[T any](t *testing.T, name string, r Reading[T]) T {
	t.Helper()
	if !r.Readable || r.Value == nil {
		t.Fatalf("%s is not readable: %+v (reason %+v)", name, r, r.Reason)
	}
	return *r.Value
}

func TestIdentityPi5(t *testing.T) {
	t.Parallel()

	sys := fakeSys{
		uname: Uname{Nodename: "example-host", Release: "6.18.50+rpt-rpi-2712", Machine: "aarch64"},
		boot:  26090*time.Second + 300*time.Millisecond,
	}
	v := identityCollector(fixtureFS(t, "pi5"), sys).Read(context.Background())
	d := v.Device

	if got := mustRead(t, "model", d.Model); got != "Raspberry Pi 5 Model B Rev 1.0" {
		t.Errorf("model = %q, want the device-tree string without its trailing NUL", got)
	}
	if got := mustRead(t, "os", d.OS); got != "Debian GNU/Linux 13 (trixie)" {
		t.Errorf("os = %q, want the PRETTY_NAME", got)
	}
	if got := mustRead(t, "cores", d.Cores); got != 4 {
		t.Errorf("cores = %d, want 4 from 0-3", got)
	}
	if got := mustRead(t, "arch", d.Arch); got != (Arch{GOARCH: runtime.GOARCH, Machine: "aarch64"}) {
		t.Errorf("arch = %+v", got)
	}
	if got := mustRead(t, "uptime_seconds", d.UptimeSeconds); got != 26090 {
		t.Errorf("uptime_seconds = %d, want 26090 (truncated, never rounded up)", got)
	}
	if v.Container {
		t.Error("container = true on a tree with no container marker")
	}
}

func TestIdentityAMD64(t *testing.T) {
	t.Parallel()

	sys := fakeSys{uname: Uname{Nodename: "example-host", Release: "6.12.0", Machine: "x86_64"}}
	d := identityCollector(fixtureFS(t, "amd64"), sys).Read(context.Background()).Device

	if got := mustRead(t, "model", d.Model); got != "Example Vendor Example Board X570" {
		t.Errorf("model = %q, want DMI vendor and product", got)
	}
	if got := mustRead(t, "os", d.OS); got != `Example Linux 42 "Desk"` {
		t.Errorf("os = %q, want the PRETTY_NAME from usr/lib/os-release, unescaped", got)
	}
	if got := mustRead(t, "cores", d.Cores); got != 16 {
		t.Errorf("cores = %d, want 16 from 0-15", got)
	}
}

func TestCountCPUList(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]int{"0-3": 4, "0-3,5,7-8": 7, "0": 1, "0-15\n": 16} {
		got, err := countCPUList(in)
		if err != nil || got != want {
			t.Errorf("countCPUList(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "3-1", "a", "0-", "0,,1"} {
		if got, err := countCPUList(in); err == nil {
			t.Errorf("countCPUList(%q) = %d with no error; want an error", in, got)
		}
	}
}

func TestOSRelease(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		files fstest.MapFS
		want  string
	}{
		"double quotes": {
			files: fstest.MapFS{"etc/os-release": {Data: []byte("NAME=\"Debian GNU/Linux\"\nPRETTY_NAME=\"Debian GNU/Linux 13 (trixie)\"\n")}},
			want:  "Debian GNU/Linux 13 (trixie)",
		},
		"single quotes": {
			files: fstest.MapFS{"etc/os-release": {Data: []byte("PRETTY_NAME='Example OS 1'\n")}},
			want:  "Example OS 1",
		},
		"backslash escapes": {
			files: fstest.MapFS{"etc/os-release": {Data: []byte(`PRETTY_NAME="A \"quoted\" \$HOME \\ and \` + "`" + `tick\` + "`" + `"` + "\n")}},
			want:  "A \"quoted\" $HOME \\ and `tick`",
		},
		"unquoted, with comments and blank lines": {
			files: fstest.MapFS{"etc/os-release": {Data: []byte("# a comment\n\nPRETTY_NAME=Plain\n")}},
			want:  "Plain",
		},
		"NAME and VERSION when there is no PRETTY_NAME": {
			files: fstest.MapFS{"etc/os-release": {Data: []byte("NAME=\"Example\"\nVERSION=\"7 (seven)\"\n")}},
			want:  "Example 7 (seven)",
		},
		"Linux when it names nothing": {
			files: fstest.MapFS{"etc/os-release": {Data: []byte("ID=example\n")}},
			want:  "Linux",
		},
		"usr/lib when etc has none": {
			files: fstest.MapFS{"usr/lib/os-release": {Data: []byte("PRETTY_NAME=\"From usr/lib\"\n")}},
			want:  "From usr/lib",
		},
		"etc wins over usr/lib": {
			files: fstest.MapFS{
				"etc/os-release":     {Data: []byte("PRETTY_NAME=\"From etc\"\n")},
				"usr/lib/os-release": {Data: []byte("PRETTY_NAME=\"From usr/lib\"\n")},
			},
			want: "From etc",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := readOSRelease(tc.files)
			if !got.Readable || *got.Value != tc.want {
				t.Errorf("os = %+v, want %q", got, tc.want)
			}
		})
	}
}

func TestIdentityMissingSources(t *testing.T) {
	t.Parallel()

	sys := fakeSys{uname: Uname{Nodename: "example-host", Release: "6.12.0", Machine: "x86_64"}, bootErr: errors.New("clock unavailable")}
	v := identityCollector(fstest.MapFS{}, sys).Read(context.Background())
	d := v.Device

	for name, r := range map[string]struct {
		readable bool
		reason   *Reason
	}{
		"model":          {d.Model.Readable, d.Model.Reason},
		"os":             {d.OS.Readable, d.OS.Reason},
		"cores":          {d.Cores.Readable, d.Cores.Reason},
		"uptime_seconds": {d.UptimeSeconds.Readable, d.UptimeSeconds.Reason},
	} {
		if r.readable {
			t.Errorf("%s is readable on an empty tree", name)
			continue
		}
		if r.reason == nil || r.reason.Code != CodeReadFailed || r.reason.Message == "" {
			t.Errorf("%s: reason = %+v, want read-failed with a sentence", name, r.reason)
		}
	}
	if d.Model.Value != nil {
		t.Errorf("model carries a value although nothing was read: %q", *d.Model.Value)
	}
	if !strings.Contains(d.Model.Reason.Message, "sys/firmware/devicetree/base/model") {
		t.Errorf("model reason %q does not name the device-tree path", d.Model.Reason.Message)
	}

	// Arch comes from uname; with uname failing it carries uname's reason.
	failing := identityCollector(fstest.MapFS{}, fakeSys{unameErr: errors.New("nope")}).Read(context.Background())
	if failing.Device.Arch.Readable || failing.Device.Arch.Reason == nil {
		t.Errorf("arch = %+v with uname failing, want not readable", failing.Device.Arch)
	}
}

func TestContainer(t *testing.T) {
	t.Parallel()

	const environ = "PATH=/bin\x00container=podman\x00HOME=/root\x00"
	for name, tc := range map[string]struct {
		files fstest.MapFS
		want  bool
	}{
		"docker marker":              {fstest.MapFS{".dockerenv": {}}, true},
		"podman marker":              {fstest.MapFS{"run/.containerenv": {}}, true},
		"container= in PID 1's env":  {fstest.MapFS{"proc/1/environ": {Data: []byte(environ)}}, true},
		"PID 1's env without it":     {fstest.MapFS{"proc/1/environ": {Data: []byte("PATH=/bin\x00HOME=/root\x00")}}, false},
		"PID 1's env, key mid-entry": {fstest.MapFS{"proc/1/environ": {Data: []byte("NOTcontainer=x\x00")}}, false},
		"PID 1's env unreadable":     {fstest.MapFS{"proc/1/environ": {Mode: fs.ModeDir}}, false},
		"no marker at all":           {fstest.MapFS{}, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			v := identityCollector(tc.files, tracerSys()).Read(context.Background())
			if v.Container != tc.want {
				t.Errorf("container = %v, want %v", v.Container, tc.want)
			}

			// Only the verdict leaves: the environment of PID 1 is never in
			// the answer (T-11-03).
			raw := marshalView(t, v)
			for _, secret := range []string{"podman", "PATH=/bin", "HOME=/root"} {
				if strings.Contains(stringify(raw), secret) {
					t.Errorf("the view contains %q from PID 1's environment", secret)
				}
			}
		})
	}
}

func stringify(v any) string {
	var b strings.Builder
	walkJSON(v, "", func(_ string, obj map[string]any) {
		for k, val := range obj {
			b.WriteString(k)
			b.WriteByte('=')
			if s, ok := val.(string); ok {
				b.WriteString(s)
			}
			b.WriteByte(' ')
		}
	})
	return b.String()
}

// TestLiveHostMatchesTheKernel reads the machine the test runs on, through the
// production seams, and compares with what the kernel says by other routes.
// Fixtures prove the parsing; this proves the fixtures are shaped like the
// real thing.
func TestLiveHostMatchesTheKernel(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the host is read on Linux only; elsewhere every section is unsupported by design")
	}
	t.Parallel()

	v := New(Config{FS: os.DirFS("/"), Sys: OS()}).Read(context.Background())
	d := v.Device

	want, err := os.Hostname()
	if err != nil {
		t.Fatalf("os.Hostname: %v", err)
	}
	if got := mustRead(t, "hostname", d.Hostname); got != want {
		t.Errorf("hostname = %q, os.Hostname says %q", got, want)
	}

	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		t.Fatalf("uname -r: %v", err)
	}
	if got := mustRead(t, "kernel", d.Kernel); got != strings.TrimSpace(string(out)) {
		t.Errorf("kernel = %q, uname -r says %q", got, strings.TrimSpace(string(out)))
	}

	up := mustRead(t, "uptime_seconds", d.UptimeSeconds)
	if raw, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(raw))
		if len(fields) > 0 {
			proc, err := strconv.ParseFloat(fields[0], 64)
			if err != nil {
				t.Fatalf("parse /proc/uptime %q: %v", raw, err)
			}
			if diff := float64(up) - proc; diff > 2 || diff < -2 {
				t.Errorf("uptime_seconds = %d, /proc/uptime says %.2f", up, proc)
			}
		}
	} else {
		t.Logf("/proc/uptime not readable here (%v); boot uptime compared with nothing", err)
	}

	if got := mustRead(t, "arch", d.Arch); got.GOARCH != runtime.GOARCH || got.Machine == "" {
		t.Errorf("arch = %+v", got)
	}
	if got := mustRead(t, "cores", d.Cores); got < 1 {
		t.Errorf("cores = %d", got)
	}
}
