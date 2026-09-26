package inventory

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// The hardware view: one node's CPU, memory, disks, links, temperatures and
// fans, read from the node at the moment somebody asks.
//
// Nothing here is persisted and nothing is read in the background. The
// inventory's other views are the record of what a node *is*, kept honest by a
// heartbeat; this one is a gauge, and a gauge that showed the last heartbeat's
// value would be a gauge that is wrong in exactly the moment somebody looks at
// it because something is wrong. So it asks the node, every time.
//
// (Since 2026-09-26 internal/history's sampler calls Hardware every fifteen
// seconds and keeps the answers for the charts. That is a caller of this read,
// not a cache in front of it: the view still never answers from the past.)
//
// What it does remember is the one thing a gauge cannot do without: the
// previous reading. A CPU percentage and a throughput are differences between
// two counter readings, and the second request of an open panel already has
// the first -- so the panel's own refresh interval becomes the measurement
// window, and only a request with nothing usable behind it pays for a second
// reading of its own.

// HardwareBudget bounds one hardware read: connect, the concurrent sample, and
// on a first request the baseline gap and the second counters read.
//
// It is short on purpose. The panel refreshes every few seconds and each
// refresh is a new request; one that has not answered in ten seconds is
// better abandoned for the next than waited on, because by the time it arrived
// the next would already be due. A healthy node on a LAN answers the whole
// thing in a few hundred milliseconds -- plus the one-second gap on a first
// request.
const HardwareBudget = 10 * time.Second

const (
	// hardwareBaselineGap is how far apart the two readings of a first request
	// are taken. A second is long enough for a percentage to mean something
	// -- the kernel counts CPU time in hundredths of a second -- and short
	// enough that a panel opening does not feel like it is thinking.
	hardwareBaselineGap = time.Second

	// minRateWindow and maxRateWindow bound when a remembered reading may
	// serve as the baseline. Below half a second the counters have barely
	// moved and a percentage computed from them is mostly rounding -- which is
	// what two browser tabs polling the same node would otherwise produce.
	// Above five minutes the "current" CPU load would be a five-minute
	// average, which is a different number with the same label.
	minRateWindow = 500 * time.Millisecond
	maxRateWindow = 5 * time.Minute

	// sensorLayoutTTL is how long a discovered sensor layout is trusted. A
	// chip's files do not change while the node runs, so this is not about
	// correctness -- a reboot or a missing file is caught on every read, see
	// talos.HardwareSample -- but about a driver loaded since, whose sensors
	// would otherwise never appear until the daemon restarted.
	sensorLayoutTTL = 10 * time.Minute
)

// hardwareMemo is one machine's remembered state between hardware reads.
type hardwareMemo struct {
	counters talos.HardwareCounters

	layout   *talos.SensorLayout
	layoutAt time.Time

	// touched is the last read of this machine, on the service's clock. An
	// entry nobody has read for a layout's lifetime is useless on both counts
	// and is dropped, so a forgotten machine does not stay here for ever.
	touched time.Time
}

// HardwareView is the hardware panel's answer. Its shape is the API contract
// (docs/api-contract.md), which is why every list is empty rather than null and
// every number the node did not report is null rather than zero.
type HardwareView struct {
	Machine    model.MachineID `json:"machine"`
	Hostname   string          `json:"hostname"`
	ObservedAt time.Time       `json:"observed_at"`

	UptimeSeconds int64 `json:"uptime_seconds"`

	// RatesOverSeconds is the window every percentage and every per-second
	// figure below was computed over. It is part of the answer because the
	// same "18 %" means something different over one second than over two
	// minutes, and only the server knows which it was.
	RatesOverSeconds float64 `json:"rates_over_seconds"`

	CPU          HardwareCPU           `json:"cpu"`
	Memory       HardwareMemory        `json:"memory"`
	Filesystems  []HardwareFilesystem  `json:"filesystems"`
	Disks        []HardwareDisk        `json:"disks"`
	Network      []HardwareLink        `json:"network"`
	Temperatures []HardwareTemperature `json:"temperatures"`
	Fans         []HardwareFan         `json:"fans"`

	// SensorsNotice is one sentence, empty unless the node reports no
	// temperatures or no fans at all. An empty sensor panel with no
	// explanation reads as this product failing to show something, and the
	// cause is nearly always the node's kernel not carrying the driver.
	SensorsNotice string `json:"sensors_notice"`
}

// HardwareCPU is the processor and how busy it is.
type HardwareCPU struct {
	Model   string `json:"model"`
	Cores   int    `json:"cores"`
	Threads int    `json:"threads"`

	// UsagePercent is time spent doing anything but idling or waiting on
	// I/O. IowaitPercent is the waiting, reported beside it rather than inside
	// it: a CPU stalled on a slow disk is not a busy CPU, and folding the two
	// together sends an operator looking for a runaway process that is not
	// there.
	UsagePercent  float64   `json:"usage_percent"`
	IowaitPercent float64   `json:"iowait_percent"`
	PerCore       []float64 `json:"per_core"`

	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

// HardwareMemory is the node's memory in bytes.
type HardwareMemory struct {
	TotalBytes uint64 `json:"total_bytes"`

	// UsedBytes is total minus MemAvailable: what the node could not hand to a
	// new process without swapping. It is deliberately not total minus free,
	// which counts the page cache as used and makes every healthy Linux
	// machine look full.
	UsedBytes      uint64 `json:"used_bytes"`
	CacheBytes     uint64 `json:"cache_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`

	SwapTotalBytes uint64 `json:"swap_total_bytes"`
	SwapUsedBytes  uint64 `json:"swap_used_bytes"`
}

// HardwareFilesystem is one mounted disk filesystem.
type HardwareFilesystem struct {
	Mount  string `json:"mount"`
	Device string `json:"device"`

	SizeBytes uint64 `json:"size_bytes"`

	// UsedBytes is size minus what is available to an unprivileged writer.
	// The node's Mounts answer carries no free-block count, so blocks reserved
	// for root count as used here; on the XFS Talos formats its data
	// partition with, that reserve is next to nothing.
	UsedBytes uint64 `json:"used_bytes"`
}

// HardwareDisk is one drive.
type HardwareDisk struct {
	Name      string `json:"name"`
	Model     string `json:"model"`
	SizeBytes uint64 `json:"size_bytes"`

	ReadBytesPerSec  float64 `json:"read_bytes_per_sec"`
	WriteBytesPerSec float64 `json:"write_bytes_per_sec"`

	// TemperatureC is the drive's own sensor, null when the kernel exposes
	// none for it -- a SATA drive without the drivetemp module -- or when it
	// cannot be told which of two identical drives a sensor belongs to.
	TemperatureC *float64 `json:"temperature_c"`
}

// HardwareLink is one network interface.
type HardwareLink struct {
	Name string `json:"name"`
	Up   bool   `json:"up"`

	// SpeedMbit is the negotiated speed, and 0 when the link reports none --
	// it is down, or it is a kind of link that has no speed.
	SpeedMbit int `json:"speed_mbit"`

	RxBytesPerSec float64 `json:"rx_bytes_per_sec"`
	TxBytesPerSec float64 `json:"tx_bytes_per_sec"`
}

// HardwareTemperature is one temperature sensor.
type HardwareTemperature struct {
	Chip  string `json:"chip"`
	Kind  string `json:"kind"`
	Label string `json:"label"`

	Celsius float64 `json:"celsius"`

	// HighC and CriticalC are the chip's own limits, null when it sets none.
	HighC     *float64 `json:"high_c"`
	CriticalC *float64 `json:"critical_c"`
}

// HardwareFan is one fan header.
type HardwareFan struct {
	Chip  string `json:"chip"`
	Label string `json:"label"`
	RPM   int    `json:"rpm"`
}

// Hardware reads one machine's hardware live.
//
// A machine that is not in the inventory is ErrNotFound. A node that does not
// answer fails the way every other live read fails -- as the classified
// transport error the connection produced -- because an unreachable node is
// exactly what this view must not paper over with the last numbers it saw.
func (s *Service) Hardware(ctx context.Context, id model.MachineID) (HardwareView, error) {
	ctx, cancel := context.WithTimeout(ctx, HardwareBudget)
	defer cancel()

	cc, err := s.Connect(ctx, id)
	if err != nil {
		return HardwareView{}, err
	}
	defer func() { _ = cc.Close() }()

	prev, havePrev, layout := s.recallHardware(id)

	sample, err := cc.HardwareSample(ctx, layout)
	if err != nil {
		return HardwareView{}, err
	}

	baseline := prev
	if !usableBaseline(prev, havePrev, sample.Counters) {
		// Nothing remembered, or nothing a rate can honestly be computed
		// against: take the reading just made as the baseline and read the
		// counters again. The rest of the sample -- memory, sensors, mounts --
		// is a level rather than a rate, and is not read twice.
		baseline = sample.Counters

		gap := time.NewTimer(hardwareBaselineGap)
		select {
		case <-ctx.Done():
			gap.Stop()
			return HardwareView{}, ctx.Err()
		case <-gap.C:
		}

		if sample.Counters, err = cc.HardwareCounters(ctx); err != nil {
			return HardwareView{}, err
		}
	}

	s.rememberHardware(id, sample, layout)

	hostname := sample.Hostname
	if hostname == "" {
		// A node that has not settled a hostname yet is still the machine the
		// inventory filed under one.
		if rec, err := s.MachineRecord(ctx, id); err == nil {
			hostname = rec.Hostname
		}
	}

	return hardwareView(id, hostname, sample, baseline), nil
}

// recallHardware is what the last read of this machine left behind.
func (s *Service) recallHardware(id model.MachineID) (talos.HardwareCounters, bool, *talos.SensorLayout) {
	s.hwMu.Lock()
	defer s.hwMu.Unlock()

	memo, ok := s.hardware[id]
	if !ok {
		return talos.HardwareCounters{}, false, nil
	}

	var layout *talos.SensorLayout
	if memo.layout != nil && s.deps.Now().Sub(memo.layoutAt) < sensorLayoutTTL {
		layout = memo.layout
	}
	return memo.counters, !memo.counters.At.IsZero(), layout
}

// rememberHardware keeps this read for the next one.
func (s *Service) rememberHardware(id model.MachineID, sample talos.HardwareSample, passed *talos.SensorLayout) {
	now := s.deps.Now()

	s.hwMu.Lock()
	defer s.hwMu.Unlock()

	for other, memo := range s.hardware {
		if now.Sub(memo.touched) > sensorLayoutTTL {
			delete(s.hardware, other)
		}
	}

	memo, ok := s.hardware[id]
	if !ok {
		memo = &hardwareMemo{}
		s.hardware[id] = memo
	}
	memo.touched = now

	// Two tabs on the same node race here, and the later reading wins
	// whichever request finishes last: a baseline that moved backwards would
	// make the next window longer than the one it reports.
	if sample.Counters.At.After(memo.counters.At) {
		memo.counters = sample.Counters
	}
	if sample.Layout != nil && sample.Layout != passed {
		memo.layout = sample.Layout
		memo.layoutAt = now
	}
}

// usableBaseline reports whether a remembered reading can be subtracted from
// the current one.
func usableBaseline(prev talos.HardwareCounters, havePrev bool, cur talos.HardwareCounters) bool {
	if !havePrev || !prev.Boot.Equal(cur.Boot) {
		// Across a reboot every counter restarted from zero, and the
		// difference would be a negative number of seconds of CPU time.
		return false
	}
	window := cur.At.Sub(prev.At)
	return window >= minRateWindow && window <= maxRateWindow
}

// hardwareRates is what two counter readings say about the time between them.
type hardwareRates struct {
	window float64

	usage   float64
	iowait  float64
	perCore []float64

	disks map[string]ioRate
	links map[string]ioRate
}

// ioRate is bytes per second in each direction: read and write for a disk,
// received and transmitted for a link.
type ioRate struct {
	in, out float64
}

// computeRates is the arithmetic between two counter readings, and nothing
// else: no filtering, no rounding, no idea what a screen shows. It is a pure
// function so the arithmetic can be tested against numbers worked out by hand.
func computeRates(prev, cur talos.HardwareCounters) hardwareRates {
	r := hardwareRates{
		window:  cur.At.Sub(prev.At).Seconds(),
		perCore: make([]float64, len(cur.CPUs)),
		disks:   make(map[string]ioRate, len(cur.Disks)),
		links:   make(map[string]ioRate, len(cur.Links)),
	}

	r.usage, r.iowait = cpuPercent(prev.CPUTotal, cur.CPUTotal)

	// By position, because that is the kernel's own identity for a logical
	// CPU. A CPU that came online between the readings has nothing to be
	// compared with, and reads 0 for one refresh rather than a percentage of
	// its whole uptime.
	for i, now := range cur.CPUs {
		if i < len(prev.CPUs) {
			r.perCore[i], _ = cpuPercent(prev.CPUs[i], now)
		}
	}

	for name, now := range cur.Disks {
		if was, ok := prev.Disks[name]; ok {
			r.disks[name] = ioRate{
				in:  perSecond(was.ReadBytes, now.ReadBytes, r.window),
				out: perSecond(was.WrittenBytes, now.WrittenBytes, r.window),
			}
		}
	}
	for name, now := range cur.Links {
		if was, ok := prev.Links[name]; ok {
			r.links[name] = ioRate{
				in:  perSecond(was.RxBytes, now.RxBytes, r.window),
				out: perSecond(was.TxBytes, now.TxBytes, r.window),
			}
		}
	}

	return r
}

// cpuPercent is the share of the CPU time between two readings spent busy,
// and the share spent waiting on I/O.
//
// The denominator is the CPU time that passed, not the wall-clock window: the
// kernel's counters are the authority on how much time there was to spend,
// and dividing by the window instead would report a CPU at 104 % whenever the
// two readings were taken a little further apart than the timestamps say.
func cpuPercent(prev, cur talos.CPUTimes) (busy, iowait float64) {
	total := cpuSum(cur) - cpuSum(prev)
	if total <= 0 {
		return 0, 0
	}

	waiting := cur.Iowait - prev.Iowait
	idle := cur.Idle - prev.Idle + waiting

	return clampPercent(100 * (total - idle) / total), clampPercent(100 * waiting / total)
}

func cpuSum(t talos.CPUTimes) float64 {
	return t.User + t.Nice + t.System + t.Idle + t.Iowait + t.Irq + t.SoftIrq + t.Steal
}

// perSecond is a counter's increase over a window, and 0 for a counter that
// went backwards -- an interface that was recreated, a device that was
// replaced -- because a negative throughput is not a thing that happened.
func perSecond(was, now uint64, window float64) float64 {
	if window <= 0 || now < was {
		return 0
	}
	return float64(now-was) / window
}

func clampPercent(p float64) float64 {
	return math.Min(math.Max(p, 0), 100)
}

// hardwareView assembles the answer.
func hardwareView(id model.MachineID, hostname string, sample talos.HardwareSample, baseline talos.HardwareCounters) HardwareView {
	cur := sample.Counters
	r := computeRates(baseline, cur)

	temps, fans, diskTemps := flattenSensors(sample.Sensors, sample.BlockDevices)

	return HardwareView{
		Machine:          id,
		Hostname:         hostname,
		ObservedAt:       cur.At.UTC(),
		UptimeSeconds:    max(int64(cur.At.Sub(cur.Boot)/time.Second), 0),
		RatesOverSeconds: round(r.window, 3),
		CPU:              cpuView(sample, r),
		Memory:           memoryView(sample.Memory),
		Filesystems:      filesystemsView(sample.Mounts),
		Disks:            disksView(sample.BlockDevices, r, diskTemps),
		Network:          networkView(sample.Links, r),
		Temperatures:     temps,
		Fans:             fans,
		SensorsNotice:    sensorsNotice(len(temps), len(fans)),
	}
}

func cpuView(sample talos.HardwareSample, r hardwareRates) HardwareCPU {
	v := HardwareCPU{
		UsagePercent:  round(r.usage, 1),
		IowaitPercent: round(r.iowait, 1),
		PerCore:       make([]float64, 0, len(r.perCore)),
		Load1:         round(sample.Load.Load1, 2),
		Load5:         round(sample.Load.Load5, 2),
		Load15:        round(sample.Load.Load15, 2),
	}
	for _, p := range r.perCore {
		v.PerCore = append(v.PerCore, round(p, 1))
	}

	for _, cpu := range sample.CPUs {
		if v.Model == "" {
			v.Model = strings.TrimSpace(cpu.ProductName)
		}
		v.Cores += int(cpu.Cores)
		v.Threads += int(cpu.Threads)
	}

	// The kernel's count wins over SMBIOS's, because it is the count the
	// per-core list has and the one the scheduler uses: a board whose BIOS
	// disabled SMT still reports the threads it could have had. SMBIOS is
	// only the fallback for cores, and a board without SMBIOS -- every ARM
	// board -- reports its threads as its cores rather than as nothing.
	if n := len(sample.Counters.CPUs); n > 0 {
		v.Threads = n
	}
	if v.Cores == 0 {
		v.Cores = v.Threads
	}
	return v
}

func memoryView(m talos.MemoryInfo) HardwareMemory {
	return HardwareMemory{
		TotalBytes:     m.TotalBytes,
		UsedBytes:      saturatingSub(m.TotalBytes, m.AvailableBytes),
		CacheBytes:     m.BuffersBytes + m.CachedBytes + m.SReclaimableBytes,
		AvailableBytes: m.AvailableBytes,
		SwapTotalBytes: m.SwapTotalBytes,
		SwapUsedBytes:  saturatingSub(m.SwapTotalBytes, m.SwapFreeBytes),
	}
}

// filesystemsView keeps the filesystems that live on a disk, once each.
//
// A Talos node mounts one partition in many places: EPHEMERAL is /var, and
// then again at every bind mount the kubelet and the CNI make out of it. Each
// of those reports the same device with the same size, and listing them would
// show one partition as a dozen full disks. The first mount of a device is
// kept, because a bind mount can only be made from something already mounted,
// so the first is the original. The root filesystem is a squashfs image on a
// loop device and is not a disk anybody can fill.
func filesystemsView(mounts []talos.Mount) []HardwareFilesystem {
	out := make([]HardwareFilesystem, 0)
	seen := map[string]bool{}
	for _, m := range mounts {
		if !strings.HasPrefix(m.Device, "/dev/") || strings.HasPrefix(m.Device, "/dev/loop") || m.SizeBytes == 0 {
			continue
		}
		if seen[m.Device] {
			continue
		}
		seen[m.Device] = true
		out = append(out, HardwareFilesystem{
			Mount:     m.MountPoint,
			Device:    m.Device,
			SizeBytes: m.SizeBytes,
			UsedBytes: saturatingSub(m.SizeBytes, m.AvailableBytes),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mount < out[j].Mount })
	return out
}

// virtualDiskPrefixes are block devices that are not drives: loop devices
// (the root filesystem and every system extension), RAM disks, device-mapper
// and md volumes whose traffic is already counted on the drives beneath them,
// and network block devices.
var virtualDiskPrefixes = []string{"loop", "ram", "zram", "dm-", "md", "nbd"}

func disksView(devices []talos.BlockDevice, r hardwareRates, temps map[string]float64) []HardwareDisk {
	out := make([]HardwareDisk, 0, len(devices))
	for _, d := range devices {
		if d.CDROM || hasAnyPrefix(d.Device, virtualDiskPrefixes) {
			continue
		}

		// Joined by name with the kernel's counters, which list partitions
		// beside their disks. Only the whole disk is a row: its counters
		// already include its partitions, and adding them would count every
		// byte twice.
		io := r.disks[d.Device]
		disk := HardwareDisk{
			Name:             d.Device,
			Model:            strings.TrimSpace(d.Model),
			SizeBytes:        d.Size,
			ReadBytesPerSec:  math.Round(io.in),
			WriteBytesPerSec: math.Round(io.out),
		}
		if c, ok := temps[d.Device]; ok {
			c = round(c, 1)
			disk.TemperatureC = &c
		}
		out = append(out, disk)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// networkView keeps the links somebody plugged a cable into, and the bonds and
// VLANs made from them.
//
// A Kubernetes node has one veth per pod, a bridge or a tunnel per CNI and a
// loopback, and their counters are in the same list as the NIC's. They are
// left out rather than listed, because the question this panel answers is
// "what is the network doing to this machine", and a pod talking to another
// pod on the same node never touched the network.
func networkView(links []talos.Link, r hardwareRates) []HardwareLink {
	out := make([]HardwareLink, 0, len(links))
	for _, l := range links {
		if !l.Physical && l.Kind != "bond" && l.Kind != "vlan" {
			continue
		}
		io := r.links[l.Name]
		out = append(out, HardwareLink{
			Name:          l.Name,
			Up:            l.Up,
			SpeedMbit:     max(l.SpeedMbit, 0),
			RxBytesPerSec: math.Round(io.in),
			TxBytesPerSec: math.Round(io.out),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// flattenSensors turns chips into the two flat lists the API carries, and
// works out which drive each disk sensor belongs to.
func flattenSensors(chips []talos.ChipReading, devices []talos.BlockDevice) (
	temps []HardwareTemperature, fans []HardwareFan, diskTemps map[string]float64,
) {
	temps = make([]HardwareTemperature, 0)
	fans = make([]HardwareFan, 0)
	diskTemps = map[string]float64{}
	claimed := map[string]bool{}

	for _, chip := range chips {
		drive := ""
		if chip.Kind == talos.SensorDisk {
			drive = matchDrive(chip, devices, claimed)
			if drive != "" {
				claimed[drive] = true
			}
		}
		primary := primaryTemp(chip.Temps)

		for i, t := range chip.Temps {
			label := t.Label
			// Every NVMe drive's chip is called "nvme" and its sensor
			// "Composite"; two of them in one list are indistinguishable
			// without saying which drive each is.
			if chip.Kind == talos.SensorDisk {
				switch {
				case drive != "":
					label = drive + " " + label
				case chip.DeviceModel != "":
					label = chip.DeviceModel + " " + label
				}
			}

			temps = append(temps, HardwareTemperature{
				Chip:      chip.Name,
				Kind:      string(chip.Kind),
				Label:     label,
				Celsius:   round(t.Celsius, 1),
				HighC:     roundPtr(t.High),
				CriticalC: roundPtr(t.Critical),
			})
			if i == primary && drive != "" {
				diskTemps[drive] = t.Celsius
			}
		}
		for _, f := range chip.Fans {
			fans = append(fans, HardwareFan{Chip: chip.Name, Label: f.Label, RPM: f.RPM})
		}
	}
	return temps, fans, diskTemps
}

// matchDrive finds the drive a disk sensor belongs to: by serial number when
// the sensor's parent device has one, otherwise by model when exactly one
// unclaimed drive carries it.
//
// Two identical drives with no serial to tell them apart get no temperature
// at all rather than a guess. A guess is right half the time, and the half it
// is wrong is the drive that is actually overheating.
func matchDrive(chip talos.ChipReading, devices []talos.BlockDevice, claimed map[string]bool) string {
	if serial := normalise(chip.DeviceSerial); serial != "" {
		for _, d := range devices {
			if !claimed[d.Device] && normalise(d.Serial) == serial {
				return d.Device
			}
		}
	}

	model := normalise(chip.DeviceModel)
	if model == "" {
		return ""
	}
	match, n := "", 0
	for _, d := range devices {
		if !claimed[d.Device] && normalise(d.Model) == model {
			match, n = d.Device, n+1
		}
	}
	if n != 1 {
		return ""
	}
	return match
}

// primaryTemp is the sensor that stands for the whole drive: NVMe's
// "Composite", which is the controller's own summary and the figure its
// thresholds apply to, or the first sensor otherwise.
func primaryTemp(temps []talos.TempReading) int {
	for i, t := range temps {
		if strings.EqualFold(t.Label, "Composite") {
			return i
		}
	}
	return 0
}

// sensorsNotice explains an empty sensor panel.
//
// It names the cause because the cause is nearly always the same and is
// nowhere an operator would look: Talos ships a fixed kernel, the sensor chip
// on a given mainboard needs a driver that kernel may not carry, and without
// it the chip is invisible to Linux. The fans are spinning and the CPU is at a
// temperature; nothing here can read it, and nothing here is broken either.
func sensorsNotice(temps, fans int) string {
	const cause = "that is down to which sensor drivers the node's Talos kernel loads, " +
		"not a fault in the hardware or in holzkube-manager."
	switch {
	case temps == 0 && fans == 0:
		return "This node's kernel reports no temperature or fan sensors; " + cause
	case temps == 0:
		return "This node's kernel reports no temperature sensors; " + cause
	case fans == 0:
		return "This node's kernel reports no fan sensors; " + cause
	default:
		return ""
	}
}

func normalise(s string) string { return strings.Join(strings.Fields(s), " ") }

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func saturatingSub(a, b uint64) uint64 {
	if b > a {
		return 0
	}
	return a - b
}

func round(x float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(x*p) / p
}

func roundPtr(x *float64) *float64 {
	if x == nil {
		return nil
	}
	v := round(*x, 1)
	return &v
}
