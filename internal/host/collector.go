package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/host/updatestatus"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/talos"
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

	// DataDir is the daemon's data directory, absolute. Its filesystem is the
	// second row of the Filesystems card, and its size is walked at most once a
	// minute. Empty: only the root row, and no size.
	DataDir string

	// Version is the running version, the variable --version prints.
	Version string
	// Started is when the process started serving.
	Started time.Time
	// UpdateStatusPath is the absolute path of the update script's status
	// file (--update-status-file). Empty means updatestatus.DefaultPath.
	UpdateStatusPath string
}

// minRateWindow and maxRateWindow bound when the previous reading may serve
// as the baseline for a rate -- the inventory's values for the same question
// about a node. Under half a second the counters have barely moved and the
// percentage is noise; over five minutes it is an average nobody asked for.
const (
	minRateWindow = 500 * time.Millisecond
	maxRateWindow = 5 * time.Minute
)

// noBaseline is what a rate says before there are two readings to take it
// from (D-12). Never a 0: a CPU at 0 % is a reading, and the first answer after
// start has not made one.
var noBaseline = Reason{Code: CodeNoBaseline, Message: "Waiting for a second reading"}

// Collector reads the host.
//
// It remembers the counters of its previous read, because a CPU percentage is
// the difference between two of them (D-12). One Collector serves every
// request, so two open tabs share one memo.
type Collector struct {
	cfg Config

	mu   sync.Mutex
	prev *counters

	sizer *dirSizer
}

// counters is what one read leaves behind for the next one's rates.
type counters struct {
	// at is the Collector's clock as read -- with its monotonic reading, so a
	// wall-clock step between two polls does not bend the window. It is never
	// serialised.
	at time.Time
	// cpu is nil when /proc/stat could not be read; cores then too.
	cpu   *talos.CPUTimes
	cores []talos.CPUTimes
	// links are the byte counters of every interface that had both, by name.
	// Empty when /sys/class/net could not be listed.
	links map[string]talos.LinkIO
}

// New builds a Collector. It reads nothing until Read is called.
func New(cfg Config) *Collector {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.UpdateStatusPath == "" {
		cfg.UpdateStatusPath = updatestatus.DefaultPath
	}
	return &Collector{cfg: cfg, sizer: &dirSizer{fsys: cfg.FS, dir: cfg.DataDir}}
}

// Read takes one reading of the host.
//
// It returns no error, and that is the contract rather than an omission: a
// section that could not be read is a section with a Reason, and the page shows
// every other section beside it. A failure of the whole answer is a failure to
// reach the daemon at all, which the browser sees as exactly that.
func (c *Collector) Read(ctx context.Context) View {
	now := c.cfg.Now()
	v := View{ObservedAt: now.UTC()}

	v.Device = c.readDevice()
	v.Container = detectContainer(c.cfg.FS)
	v.Service = c.readService(ctx, now, v.Container)
	v.Live = c.readLive(now)

	return v
}

// The two sentences an absent status file is explained with (D-16). On a host
// the installed update script predates the status file; in a container there
// is no update timer at all, so the first sentence would send the operator
// looking for a script that does not exist there.
const (
	notRecordedHost = "The update script installed on this machine does not record its checks. " +
		"Versions from this release on do; the next update brings it."
	notRecordedContainer = "holzkube-manager runs in a container, where the host's update timer does not run. " +
		"A container is updated by pulling a new image."
	updateUnreadablePrefix = "The update status file exists but could not be read: "
)

// readService fills the Service card (HOST-02, HOST-03).
func (c *Collector) readService(ctx context.Context, now time.Time, container bool) Service {
	s := Service{
		Version:   c.cfg.Version,
		StartedAt: c.cfg.Started.UTC(),
		DataDir:   DataDir{Path: c.cfg.DataDir, Size: c.sizer.size(ctx, now)},
	}
	if !c.cfg.Started.IsZero() {
		s.UptimeSeconds = int64(now.Sub(c.cfg.Started) / time.Second)
	}

	status, err := updatestatus.Read(c.cfg.FS, c.cfg.UpdateStatusPath)
	switch {
	case errors.Is(err, updatestatus.ErrNotRecorded):
		msg := notRecordedHost
		if container {
			msg = notRecordedContainer
		}
		s.Update = Hidden[updatestatus.Status](Reason{Code: CodeNotRecorded, Message: msg})
	case err != nil:
		s.Update = Hidden[updatestatus.Status](Reason{Code: CodeReadFailed, Message: updateUnreadablePrefix + err.Error()})
	default:
		s.Update = Read(status)
	}
	return s
}

// readLive fills the Live section: CPU, load, memory and swap (HMON-01), the
// filesystems of / and the data directory (HMON-02), the temperatures and
// fans (HMON-03), and the network interfaces (HMON-04).
func (c *Collector) readLive(now time.Time) Live {
	// The proof the hardening is in force (D-03). A mountinfo that cannot be
	// read or parsed proves nothing, so every missing file is then reported as
	// the read failure it is -- and no two paths are claimed to share a disk.
	subsetPid := false
	var mounts []mountEntry
	if raw, err := readBounded(c.cfg.FS, fsPath(pathMountinfo), maxMountinfo); err == nil {
		if ms, err := parseMountinfo(raw); err == nil {
			subsetPid = procSubsetPid(ms)
			mounts = ms
		}
	}

	var live Live
	live.Memory = c.readMemory(subsetPid)
	live.Filesystems = filesystems(mounts, c.cfg.DataDir, c.cfg.Sys)
	live.CPU.Load = c.readLoad(subsetPid)
	if sensors, err := readSensors(c.cfg.FS); err != nil {
		live.Sensors = Hidden[Sensors](reasonFor(sensorsPath(err), err))
	} else {
		live.Sensors = Read(sensors)
	}

	links, linkErr := readLinks(c.cfg.FS)

	cur := counters{at: now, links: linkCounters(links)}
	var statReason *Reason
	if raw, err := readBounded(c.cfg.FS, fsPath(pathProcStat), maxSmallFile); err != nil {
		r := classify(pathProcStat, err, subsetPid)
		statReason = &r
	} else if total, cores, err := parseProcStat(raw); err != nil {
		r := reasonFor(pathProcStat, err)
		statReason = &r
	} else {
		cur.cpu, cur.cores = &total, cores
	}
	c.rates(&live, cur, statReason, links, linkErr)

	return live
}

// rates computes the CPU percentages and the link throughputs against the
// remembered previous read, over one window, and decides whether this read
// becomes the next one's baseline.
func (c *Collector) rates(live *Live, cur counters, statReason *Reason, links []linkSample, linkErr error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	prev := c.prev
	var window time.Duration
	usable := false
	if prev != nil {
		window = cur.at.Sub(prev.at)
		usable = window >= minRateWindow && window <= maxRateWindow
	}

	switch {
	case statReason != nil:
		// Why the file is unreadable outranks "no baseline yet": the second
		// would pass by itself, the first will not.
		live.CPU.Usage = Hidden[float64](*statReason)
		live.CPU.PerCore = Hidden[[]float64](*statReason)
	case usable && prev.cpu != nil:
		busy, _ := inventory.CPUPercent(*prev.cpu, *cur.cpu)
		live.CPU.Usage = Read(busy)
		if len(prev.cores) == len(cur.cores) {
			perCore := make([]float64, len(cur.cores))
			for i := range cur.cores {
				perCore[i], _ = inventory.CPUPercent(prev.cores[i], cur.cores[i])
			}
			live.CPU.PerCore = Read(perCore)
		} else {
			// A core came online or went offline between the two reads: the
			// indices no longer pair up, and a guess would put one core's time
			// on another.
			live.CPU.PerCore = Hidden[[]float64](noBaseline)
		}
	default:
		live.CPU.Usage = Hidden[float64](noBaseline)
		live.CPU.PerCore = Hidden[[]float64](noBaseline)
	}

	if linkErr != nil {
		live.Network = Hidden[Network](reasonFor("/"+netClass, linkErr))
	} else {
		var prevLinks map[string]talos.LinkIO
		if prev != nil {
			prevLinks = prev.links
		}
		live.Network = Read(network(links, prevLinks, window.Seconds(), usable))
	}

	if usable {
		secs := math.Round(window.Seconds()*10) / 10
		live.RatesOverSeconds = &secs
	}

	// Two tabs polling 100 ms apart must not erase each other's baseline, so a
	// read inside the minimum window leaves the memo alone. Boot times are not
	// compared: a process cannot outlive a reboot, and a boot time derived from
	// CLOCK_BOOTTIME jitters enough to make every baseline look foreign.
	if prev == nil || window < 0 || window >= minRateWindow {
		c.prev = &cur
	}
}

// sensorsPath is the class directory a failed sensor walk names: the thermal
// zones when that is where it failed, otherwise hwmon.
func sensorsPath(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) && strings.HasPrefix(pe.Path, thermalClass) {
		return "/" + thermalClass
	}
	return "/" + hwmonClass
}

// readMemory is memory and swap as free(1) counts them.
func (c *Collector) readMemory(subsetPid bool) Reading[inventory.HardwareMemory] {
	raw, err := readBounded(c.cfg.FS, fsPath(pathProcMeminfo), maxSmallFile)
	if err != nil {
		return Hidden[inventory.HardwareMemory](classify(pathProcMeminfo, err, subsetPid))
	}
	info, err := parseMeminfo(raw)
	if err != nil {
		return Hidden[inventory.HardwareMemory](reasonFor(pathProcMeminfo, err))
	}
	return Read(inventory.MemoryView(info))
}

// readLoad is the load average from /proc/loadavg, or -- when that cannot be
// read, as under ProcSubset=pid -- from sysinfo(2), rendered to the same two
// decimals the kernel prints (D-05).
func (c *Collector) readLoad(subsetPid bool) Reading[Load] {
	raw, err := readBounded(c.cfg.FS, fsPath(pathProcLoadavg), maxSmallFile)
	if err == nil {
		l, perr := parseLoadavg(raw)
		if perr == nil {
			return Read(l)
		}
		err = perr
	}
	if loads, serr := c.cfg.Sys.Loads(); serr == nil {
		return Read(Load{
			Load1:  loadFromSysinfo(loads.One),
			Load5:  loadFromSysinfo(loads.Five),
			Load15: loadFromSysinfo(loads.Fifteen),
			Source: loadSourceSysinfo,
		})
	}
	return Hidden[Load](classify(pathProcLoadavg, err, subsetPid))
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
