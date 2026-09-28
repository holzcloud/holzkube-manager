package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"runtime"
	"strings"
	"time"
)

// The ceilings on every file this package reads (ASVS V5). Everything it opens
// is a kernel interface whose size the kernel decides, and a read without a
// ceiling is one bad driver away from reading without an end. Mountinfo gets
// more because a container host with many bind mounts legitimately writes a
// long one.
const (
	maxSmallFile = 64 << 10
	maxMountinfo = 1 << 20
)

// Config is what a Collector reads through.
type Config struct {
	// FS is rooted at "/". Production: os.DirFS("/"). Tests: a fixture tree.
	FS fs.FS
	// Sys is the syscall seam. Production: OS().
	Sys Sys
	// Now is the clock ObservedAt comes from. Nil means time.Now.
	Now func() time.Time
}

// Collector reads the host.
type Collector struct {
	cfg Config
}

// New builds a Collector. It reads nothing until Read is called.
func New(cfg Config) *Collector {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Collector{cfg: cfg}
}

// Read takes one reading of the host.
//
// It returns no error, and that is the contract rather than an omission: a
// section that could not be read is a section with a Reason, and the page shows
// every other section beside it. A failure of the whole answer is a failure to
// reach the daemon at all, which the browser sees as exactly that.
func (c *Collector) Read(context.Context) View {
	v := View{ObservedAt: c.cfg.Now().UTC()}

	v.Device = c.readDevice()
	v.Container = detectContainer(c.cfg.FS)

	return v
}

// readDevice fills the Device card.
func (c *Collector) readDevice() Device {
	var d Device

	uname, err := c.cfg.Sys.Uname()
	if err != nil {
		reason := reasonFor("uname(2)", err)
		d.Hostname = Hidden[string](reason)
		d.Kernel = Hidden[string](reason)
		d.Arch = Hidden[Arch](reason)
	} else {
		d.Hostname = Read(uname.Nodename)
		d.Kernel = Read(uname.Release)
		d.Arch = Read(Arch{GOARCH: runtime.GOARCH, Machine: uname.Machine})
	}

	// CLOCK_BOOTTIME, truncated to whole seconds. Never sysinfo(2)'s uptime,
	// which rounds up (see Sys.BootTime).
	if boot, err := c.cfg.Sys.BootTime(); err != nil {
		d.UptimeSeconds = Hidden[int64](reasonFor("clock_gettime(CLOCK_BOOTTIME)", err))
	} else {
		d.UptimeSeconds = Read(int64(boot / time.Second))
	}

	d.Model = readModel(c.cfg.FS)
	d.OS = readOSRelease(c.cfg.FS)
	d.Cores = readCores(c.cfg.FS)
	return d
}

// readBounded reads one file through fsys, refusing anything larger than limit.
//
// Larger is an error rather than a truncation: half of a file parsed as if it
// were the whole is a wrong reading, and a wrong reading is worse than a
// missing one.
func readBounded(fsys fs.FS, name string, limit int64) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("%s is larger than %d bytes", name, limit)
	}
	return raw, nil
}

// fsPath turns an absolute path into the name an fs.FS rooted at "/" expects.
// os.DirFS refuses a leading slash (measured), and the paths this package
// names are the kernel's, which are written with one.
func fsPath(abs string) string {
	return strings.TrimPrefix(abs, "/")
}

// reasonFor is the Reason for a failed read of path.
func reasonFor(path string, err error) Reason {
	if errors.Is(err, errUnsupported) {
		return Reason{Code: CodeUnsupported, Message: errUnsupported.Error() + "."}
	}
	return Reason{Code: CodeReadFailed, Message: fmt.Sprintf("Could not read %s: %v", path, err)}
}
