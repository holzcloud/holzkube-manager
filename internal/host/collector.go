package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/host/hostaction"
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

	// Actions is the host actions' order slot. Nil: this instance runs
	// without host actions, and the answer says so.
	Actions *hostaction.Box
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

	mu sync.Mutex
	// prev is the page's baseline: what the last Read left for the next one.
	prev *counters
	// samplePrev is the sampler's own baseline, what the last Sample left for
	// the next one. With one shared memo, a page polling every 3 s would
	// shorten the sampler's 15-s window to whatever time had passed since its
	// last poll -- the history's averages quietly turning into 3-s samples
	// while /host is open -- and the sampler's reads would shorten the page's
	// windows in turn. Both slots are only touched under mu.
	samplePrev *counters
	// latest is what the last Sample saw, for the wall (see Latest). Nil
	// until the first Sample.
	latest *Snapshot
	// sampling is set while a Sample runs (see Sample).
	sampling atomic.Bool

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

	if platformUnsupported(c.cfg.Sys) {
		return c.readUnsupported(v, now)
	}

	v.Device = c.readDevice()
	v.Container = detectContainer(c.cfg.FS)
	v.Service = c.readService(ctx, now, v.Container)
	v.Live = c.readLive(&c.prev)
	// From the same reading, so the page's markers, its meters and its header
	// cannot disagree.
	v.Health = Assess(v.Live)
	v.Actions = c.readActions(v.Container)

	return v
}

// InContainer reports whether this process runs in a container (D-17), read
// now through the Collector's FS: the host action routes refuse there (D-14).
// False on a platform this package has no readings for, where Read says
// container false as well.
func (c *Collector) InContainer() bool {
	if platformUnsupported(c.cfg.Sys) {
		return false
	}
	return detectContainer(c.cfg.FS)
}

// CheckRunning reports whether the helper is busy with a check right now
// (hostaction.CheckRunning), read now from the helper's result and the update
// status -- the two readings Read puts into actions.result and
// service.update, from which the page draws the same conclusion. The host
// action routes refuse while it is true (13-REVIEW-2 WR-01). False without
// host actions, on a platform with no readings, and when the helper's result
// cannot be read: then there is no check anybody knows of.
func (c *Collector) CheckRunning() bool {
	box := c.cfg.Actions
	if box == nil || platformUnsupported(c.cfg.Sys) {
		return false
	}
	r, err := box.Result()
	if err != nil {
		return false
	}
	var answered time.Time
	if s, err := updatestatus.Read(c.cfg.FS, c.cfg.UpdateStatusPath); err == nil {
		answered = s.CheckedAt
	}
	return hostaction.CheckRunning(r, answered, c.cfg.Now())
}

// readUnsupported is the answer on a platform with no readings (a darwin
// build): every reading says unsupported, and only what the process knows
// about itself -- its version, when it started, where its data is -- is
// stated. The filesystem rows still ask statfs, which answers unsupported
// there too.
func (c *Collector) readUnsupported(v View, now time.Time) View {
	r := reasonFor("", errUnsupported)
	v.Device = Device{
		Hostname:      Hidden[string](r),
		Model:         Hidden[string](r),
		Arch:          Hidden[Arch](r),
		Cores:         Hidden[int](r),
		OS:            Hidden[string](r),
		Kernel:        Hidden[string](r),
		UptimeSeconds: Hidden[int64](r),
	}
	v.Service = Service{
		Version:   c.cfg.Version,
		StartedAt: c.cfg.Started.UTC(),
		DataDir:   DataDir{Path: c.cfg.DataDir, Size: Hidden[DirSize](r)},
		Update:    Hidden[updatestatus.Status](r),
	}
	if !c.cfg.Started.IsZero() {
		v.Service.UptimeSeconds = int64(now.Sub(c.cfg.Started) / time.Second)
	}
	v.Live = c.unsupportedLive()
	v.Health = Assess(v.Live)
	// No helper result is read here either: the helper is a systemd unit on
	// a Linux host, and a darwin build has nothing to ask. The order is this
	// process's own fact and stays.
	// Nor are actions available: there is no systemd to carry them out. What
	// is missing and how to install it are stated all the same.
	v.Actions = Actions{
		Result:          Hidden[hostaction.Result](r),
		Missing:         []hostaction.Missing{},
		Outdated:        []hostaction.Missing{},
		InstallCommands: slices.Clone(hostaction.InstallCommands),
	}
	if c.cfg.Actions != nil {
		v.Actions.Order = c.cfg.Actions.Order()
		v.Actions.Missing = c.cfg.Actions.Missing()
	}
	return v
}

// The sentences the helper's result is explained with when there is none.
const (
	noActionsMessage       = "This instance was started without host actions."
	noResultMessage        = "The holzkube-manager-host helper has not recorded an order on this machine."
	resultUnreadablePrefix = "The helper's result file could not be read: "
)

// readActions fills the host actions' part of the answer: the last order this
// process placed, what the helper last recorded, and whether an action can be
// placed at all -- with what is missing and how to install it. The reader's
// error names the rule a file broke, never its bytes, so it can be shown.
//
// Available is decided here and by the same two questions the routes ask
// (InContainer, the Box's Missing), and Outdated by the Box's Outdated, which
// the routes ask for the check, so the page never offers a button the server
// then refuses.
func (c *Collector) readActions(container bool) Actions {
	box := c.cfg.Actions
	if box == nil {
		return Actions{
			Result:          Hidden[hostaction.Result](Reason{Code: CodeNoResult, Message: noActionsMessage}),
			Missing:         []hostaction.Missing{},
			Outdated:        []hostaction.Missing{},
			InstallCommands: slices.Clone(hostaction.InstallCommands),
		}
	}
	missing := box.Missing()
	// Outdated only once nothing is missing: the script is read only when
	// Detect found it root's, and the install commands install everything.
	outdated := []hostaction.Missing{}
	if len(missing) == 0 {
		outdated = box.Outdated()
	}
	a := Actions{
		Order:           box.Order(),
		Available:       !container && len(missing) == 0,
		Missing:         missing,
		Outdated:        outdated,
		InstallCommands: slices.Clone(hostaction.InstallCommands),
	}
	result, err := box.Result()
	switch {
	case errors.Is(err, hostaction.ErrNoResult):
		a.Result = Hidden[hostaction.Result](Reason{Code: CodeNoResult, Message: noResultMessage})
	case err != nil:
		a.Result = Hidden[hostaction.Result](Reason{Code: CodeReadFailed, Message: resultUnreadablePrefix + err.Error()})
	default:
		a.Result = Read(result)
	}
	return a
}

// Hostname is this machine's name as uname(2) reports it now -- read fresh on
// every call, because the host confirm route compares what the operator typed
// against it (D-08), and a name remembered from an earlier read could be one
// the machine no longer has.
func (c *Collector) Hostname() (string, error) {
	if platformUnsupported(c.cfg.Sys) {
		return "", errUnsupported
	}
	u, err := c.cfg.Sys.Uname()
	if err != nil {
		return "", err
	}
	return u.Nodename, nil
}

// unsupportedLive is the Live section on a platform with no readings: every
// value unsupported, the filesystem rows as statfs answers them there.
func (c *Collector) unsupportedLive() Live {
	r := reasonFor("", errUnsupported)
	return Live{
		CPU: CPU{
			Usage:   Hidden[float64](r),
			PerCore: Hidden[[]float64](r),
			Load:    Hidden[Load](r),
		},
		Memory:      Hidden[inventory.HardwareMemory](r),
		Filesystems: filesystems(nil, c.cfg.DataDir, c.cfg.Sys),
		Sensors:     Hidden[Sensors](r),
		Network:     Hidden[Network](r),
	}
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
//
// base is the baseline slot its rates are taken against and advance: &c.prev
// for the page, &c.samplePrev for the sampler.
func (c *Collector) readLive(base **counters) Live {
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
	// The data directory's filesystem is the one its real path is on. A data
	// directory moved off the SD card by a symlink (/var/lib/holzkube-manager
	// -> /mnt/ssd/hkm) lexically sits under /, and matched lexically it would
	// show the SD card's free space as the SSD's -- a plausible wrong number.
	// When the path cannot be resolved, the lexical one is all there is.
	dataDir := c.cfg.DataDir
	if dataDir != "" {
		if resolved, ok := resolvePath(c.cfg.FS, dataDir); ok {
			dataDir = resolved
		}
	}
	live.Filesystems = filesystems(mounts, dataDir, c.cfg.Sys)
	live.CPU.Load = c.readLoad(subsetPid)
	if sensors, err := readSensors(c.cfg.FS); err != nil {
		live.Sensors = Hidden[Sensors](reasonFor(sensorsPath(err), err))
	} else {
		live.Sensors = Read(sensors)
	}

	// The rate clock is read here, where the counters are, and not taken from
	// the start of the request: readService may have walked the data
	// directory for up to dirSizeDeadline in between, and counters stamped
	// before that walk would be divided by a window the walk is missing from
	// -- too high in the poll that walked, too low in the one after.
	sampledAt := c.cfg.Now()
	links, linkErr := readLinks(c.cfg.FS)

	cur := counters{at: sampledAt, links: linkCounters(links)}
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
	c.rates(base, &live, cur, statReason, links, linkErr)

	return live
}

// rates computes the CPU percentages and the link throughputs against the
// previous read remembered in base, over one window, and decides whether this
// read becomes the next one's baseline there. base is one of the Collector's
// slots and is only read and written under c.mu.
func (c *Collector) rates(base **counters, live *Live, cur counters, statReason *Reason, links []linkSample, linkErr error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	prev := *base
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
		*base = &cur
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
