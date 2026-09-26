package talossim

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/siderolabs/talos/pkg/machinery/api/common"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
)

// The simulated node's hardware: the counters behind SystemStat, DiskStats and
// NetworkDeviceStats, the Memory, LoadAvg and Mounts answers, and a small
// read-only /sys that List and Read serve.
//
// The counters are not random and not fixed. Each is a constant rate times the
// time since the node's last boot, which gives them the two properties a real
// kernel's have and a test needs: they advance between two reads, so a rate
// computed from them is a real subtraction over a real window, and they start
// again from zero when the node reboots. Because the rates are constants, what
// the view computes from them is known in advance -- a test asserts the
// percentage the simulator was built to produce rather than one it happened to.

// The rates the simulated counters advance at. Exported so a test can assert
// the view computed them rather than restating the numbers.
const (
	// SimulatedThreadsPerCPU is how many logical CPUs each simulated
	// processor contributes to SystemStat. It matches the ThreadCount the
	// processor resource claims (cosi.go), so the SMBIOS answer and the
	// kernel's answer agree about how many threads the machine has.
	SimulatedThreadsPerCPU = 8

	// SimulatedIowait is every thread's share of time spent waiting on I/O.
	SimulatedIowait = 0.01

	// SimulatedDiskReadBytesPerSec and SimulatedDiskWriteBytesPerSec are
	// every fixture disk's throughput.
	SimulatedDiskReadBytesPerSec  = 4 << 20
	SimulatedDiskWriteBytesPerSec = 1 << 20

	// SimulatedRxBytesPerSec and SimulatedTxBytesPerSec are eth0's.
	SimulatedRxBytesPerSec = 1_250_000
	SimulatedTxBytesPerSec = 250_000
)

// SimulatedThreadBusy is logical CPU i's busy share, excluding I/O wait.
//
// It differs between threads on purpose. A view that averaged the per-core
// figures, or reported the total in every per-core slot, would pass against a
// node whose every thread was equally busy.
func SimulatedThreadBusy(i int) float64 { return 0.10 + 0.05*float64(i%4) }

// uptimeSeconds is how long the node has been up by its own clock.
func (s *Server) uptimeSeconds(lastBoot time.Time) float64 {
	return max(s.opts.Now().Sub(lastBoot).Seconds(), 0)
}

// simulatedCPU is the CPU half of SystemStat.
func (s *Server) simulatedCPU(lastBoot time.Time) (*machine.CPUStat, []*machine.CPUStat) {
	up := s.uptimeSeconds(lastBoot)

	threads := s.opts.CPUs * SimulatedThreadsPerCPU
	total := &machine.CPUStat{}
	per := make([]*machine.CPUStat, 0, threads)

	for i := range threads {
		busy := SimulatedThreadBusy(i)
		cpu := &machine.CPUStat{
			// Three quarters of the busy time in user space and a quarter in
			// the kernel, which is roughly what a node running containers
			// shows. The split does not change the percentage; it is there
			// so that a view that counted only User would be visibly wrong.
			User:   up * busy * 0.75,
			System: up * busy * 0.25,
			Iowait: up * SimulatedIowait,
			Idle:   up * (1 - busy - SimulatedIowait),
		}
		per = append(per, cpu)

		total.User += cpu.GetUser()
		total.System += cpu.GetSystem()
		total.Iowait += cpu.GetIowait()
		total.Idle += cpu.GetIdle()
	}

	return total, per
}

// Memory reports /proc/meminfo, in kB as the kernel does.
func (m *machineService) Memory(_ context.Context, _ *emptypb.Empty) (*machine.MemoryResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	total := m.server.opts.MemoryMiB * 1024
	return &machine.MemoryResponse{
		Messages: []*machine.Memory{{
			Metadata: m.server.node.metadata(),
			Meminfo: &machine.MemInfo{
				Memtotal:     total,
				Memfree:      total * 40 / 100,
				Memavailable: total * 60 / 100,
				Buffers:      total / 100,
				Cached:       total * 20 / 100,
				Sreclaimable: total * 2 / 100,
				// No swap, which is what a Kubernetes node has: the kubelet
				// refuses to start on one that swaps unless told otherwise.
				Swaptotal: 0,
				Swapfree:  0,
			},
		}},
	}, nil
}

// LoadAvg reports a lightly loaded node.
func (m *machineService) LoadAvg(_ context.Context, _ *emptypb.Empty) (*machine.LoadAvgResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	return &machine.LoadAvgResponse{
		Messages: []*machine.LoadAvg{{
			Metadata: m.server.node.metadata(),
			Load1:    0.52,
			Load5:    0.41,
			Load15:   0.33,
		}},
	}, nil
}

// DiskStats reports /proc/diskstats: every fixture disk, two partitions on
// each, and the loop device a Talos root filesystem is mounted from.
//
// The partitions and the loop device carry traffic of their own. A view that
// summed every device the kernel counts would report the system disk's
// throughput twice and a squashfs image as a drive, and it can only be caught
// doing that against a node that has them.
func (m *machineService) DiskStats(_ context.Context, _ *emptypb.Empty) (*machine.DiskStatsResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	up := m.server.uptimeSeconds(m.server.node.snapshot().LastBoot)
	sectors := func(bytesPerSec float64) uint64 { return uint64(up * bytesPerSec / 512) }

	var devices []*machine.DiskStat
	for _, d := range m.server.opts.Disks {
		devices = append(devices, &machine.DiskStat{
			Name:         d.Device,
			ReadSectors:  sectors(SimulatedDiskReadBytesPerSec),
			WriteSectors: sectors(SimulatedDiskWriteBytesPerSec),
		})
		for _, n := range []int{5, 6} {
			devices = append(devices, &machine.DiskStat{
				Name:         partition(d.Device, n),
				ReadSectors:  sectors(SimulatedDiskReadBytesPerSec / 2),
				WriteSectors: sectors(SimulatedDiskWriteBytesPerSec / 2),
			})
		}
	}
	devices = append(devices, &machine.DiskStat{Name: "loop0", ReadSectors: sectors(64 << 10)})

	total := &machine.DiskStat{Name: "total"}
	for _, d := range devices {
		total.ReadSectors += d.GetReadSectors()
		total.WriteSectors += d.GetWriteSectors()
	}

	return &machine.DiskStatsResponse{
		Messages: []*machine.DiskStats{{
			Metadata: m.server.node.metadata(),
			Total:    total,
			Devices:  devices,
		}},
	}, nil
}

// NetworkDeviceStats reports /proc/net/dev: loopback, the one physical link
// the node's LinkStatus names, and one pod's veth. The last two are the pair a
// view has to tell apart.
func (m *machineService) NetworkDeviceStats(_ context.Context, _ *emptypb.Empty) (*machine.NetworkDeviceStatsResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	up := m.server.uptimeSeconds(m.server.node.snapshot().LastBoot)
	bytes := func(perSec float64) uint64 { return uint64(up * perSec) }

	devices := []*machine.NetDev{
		{Name: "lo", RxBytes: bytes(50_000), TxBytes: bytes(50_000)},
		{Name: "eth0", RxBytes: bytes(SimulatedRxBytesPerSec), TxBytes: bytes(SimulatedTxBytesPerSec)},
		{Name: "veth3f2a1b9c", RxBytes: bytes(20_000), TxBytes: bytes(30_000)},
	}
	total := &machine.NetDev{Name: "total"}
	for _, d := range devices {
		total.RxBytes += d.GetRxBytes()
		total.TxBytes += d.GetTxBytes()
	}

	return &machine.NetworkDeviceStatsResponse{
		Messages: []*machine.NetworkDeviceStats{{
			Metadata: m.server.node.metadata(),
			Total:    total,
			Devices:  devices,
		}},
	}, nil
}

// Mounts reports what a Talos node has mounted, in the shape Talos reports it:
// the squashfs root on a loop device, pseudo-filesystems, the STATE and
// EPHEMERAL partitions, and EPHEMERAL again at every bind mount the kubelet
// and the CNI make out of it.
func (m *machineService) Mounts(_ context.Context, _ *emptypb.Empty) (*machine.MountsResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	var stats []*machine.MountStat
	if len(m.server.opts.Disks) > 0 {
		system := m.server.opts.Disks[0]
		ephemeral := system.Size * 9 / 10
		ephemeralFree := ephemeral * 3 / 4
		stats = []*machine.MountStat{
			{Filesystem: "/dev/loop0", MountedOn: "/", Size: 80 << 20, Available: 0},
			{Filesystem: "tmpfs", MountedOn: "/run", Size: 8 << 30, Available: 8 << 30},
			{Filesystem: "overlay", MountedOn: "/etc/kubernetes", Size: 1 << 20, Available: 1 << 20},
			{Filesystem: "/dev/" + partition(system.Device, 5), MountedOn: "/system/state", Size: 100 << 20, Available: 99 << 20},
			{Filesystem: "/dev/" + partition(system.Device, 6), MountedOn: "/var", Size: ephemeral, Available: ephemeralFree},
			{Filesystem: "/dev/" + partition(system.Device, 6), MountedOn: "/var/lib/kubelet/pods/0f1e/volume-subpaths/cfg", Size: ephemeral, Available: ephemeralFree},
			{Filesystem: "/dev/" + partition(system.Device, 6), MountedOn: "/etc/cni", Size: ephemeral, Available: ephemeralFree},
		}
	}

	return &machine.MountsResponse{
		Messages: []*machine.Mounts{{
			Metadata: m.server.node.metadata(),
			Stats:    stats,
		}},
	}, nil
}

// partition names a disk's nth partition the way the kernel does: nvme0n1p6,
// but sda6.
func partition(device string, n int) string {
	if device != "" && device[len(device)-1] >= '0' && device[len(device)-1] <= '9' {
		return device + "p" + strconv.Itoa(n)
	}
	return device + strconv.Itoa(n)
}

// List walks the simulated /sys the way Talos's List does: the root first, as
// ".", then its children in lexical order; a symlinked root resolved before
// the walk; symlinks below it reported with their target and not followed;
// and descent only as deep as the request asks.
func (m *machineService) List(req *machine.ListRequest, stream grpc.ServerStreamingServer[machine.FileInfo]) error {
	if err := m.server.node.up(); err != nil {
		return err
	}
	if err := m.server.fileReadsRefused(); err != nil {
		return err
	}

	// The same normalisation Talos applies before it walks: an absolute
	// path, no trailing slash, "/" for nothing at all.
	root := req.GetRoot()
	if !strings.HasPrefix(root, "/") {
		root = "/" + root
	}
	root = strings.TrimSuffix(root, "/")
	if root == "" {
		root = "/"
	}

	maxDepth := 0
	if req.GetRecurse() {
		maxDepth = int(req.GetRecursionDepth())
		if maxDepth == 0 {
			maxDepth = -1
		}
	}

	hide := m.server.opts.ListHidesSymlinks
	items, err := m.server.sysfs.walk(root, maxDepth, !hide)
	if err != nil {
		return err
	}
	metadata := m.server.node.metadata()
	for _, fi := range items {
		fi.Metadata = metadata
		if hide {
			fi.Link = ""
		}
		if err := stream.Send(fi); err != nil {
			return err
		}
	}
	return nil
}

// Read serves one file of the simulated /sys, resolving symlinks anywhere in
// the path the way the kernel's open does. A missing file is refused with the
// error Talos returns, which reaches the client as codes.Unknown: Talos hands
// the os error to gRPC as it is.
func (m *machineService) Read(req *machine.ReadRequest, stream grpc.ServerStreamingServer[common.Data]) error {
	if err := m.server.node.up(); err != nil {
		return err
	}
	if err := m.server.fileReadsRefused(); err != nil {
		return err
	}

	resolved, ok := m.server.sysfs.resolve(req.GetPath())
	if !ok {
		return fmt.Errorf("stat %s: no such file or directory", req.GetPath())
	}
	content, isFile := m.server.sysfs.files[resolved]
	if !isFile {
		return errors.New("path must be a regular file")
	}
	return stream.Send(&common.Data{Bytes: []byte(content)})
}

// fileReadsRefused is the RefuseFileReads option's answer, worded as Talos's
// authorizer words it.
func (s *Server) fileReadsRefused() error {
	if !s.opts.RefuseFileReads {
		return nil
	}
	return status.Error(codes.PermissionDenied, "not authorized")
}

// SensorProfile is which sensors the simulated node's kernel exposes.
type SensorProfile int

const (
	// SensorsDesktop is a desktop board: coretemp with a package and a core
	// temperature, an nvme chip per NVMe disk, and an nct6798 Super I/O chip
	// with a board temperature and two fan headers, one reading 0 RPM.
	SensorsDesktop SensorProfile = iota

	// SensorsNone is a kernel with no sensor driver loaded for this board:
	// /sys/class/hwmon and /sys/class/thermal exist and are empty.
	SensorsNone

	// SensorsThermalZoneOnly is a board whose CPU temperature exists only as
	// a thermal zone, with no hwmon chip at all -- a Raspberry Pi built
	// without CONFIG_THERMAL_HWMON.
	SensorsThermalZoneOnly
)

// sysfs is a read-only simulated filesystem: files with contents, symlinks
// with targets, and the directories they imply.
type sysfs struct {
	files map[string]string
	links map[string]string
	dirs  map[string]bool
}

func newSysfs() *sysfs {
	return &sysfs{
		files: map[string]string{},
		links: map[string]string{},
		dirs:  map[string]bool{"/": true},
	}
}

func (f *sysfs) file(p, content string) {
	p = path.Clean(p)
	f.files[p] = content + "\n"
	f.parents(p)
}

func (f *sysfs) link(p, target string) {
	p = path.Clean(p)
	f.links[p] = target
	f.parents(p)
}

func (f *sysfs) dir(p string) {
	p = path.Clean(p)
	f.dirs[p] = true
	f.parents(p)
}

func (f *sysfs) parents(p string) {
	for d := path.Dir(p); ; d = path.Dir(d) {
		f.dirs[d] = true
		if d == "/" {
			return
		}
	}
}

// resolve follows every symlink in p and returns the real path, or false when
// some component does not exist.
func (f *sysfs) resolve(p string) (string, bool) {
	parts := strings.Split(strings.TrimPrefix(path.Clean("/"+p), "/"), "/")
	cur := "/"
	for hops := 0; len(parts) > 0; {
		part := parts[0]
		parts = parts[1:]
		if part == "" {
			continue
		}

		next := path.Join(cur, part)
		if target, ok := f.links[next]; ok {
			// ELOOP, as the kernel would.
			if hops++; hops > 40 {
				return "", false
			}
			if !path.IsAbs(target) {
				target = path.Join(cur, target)
			}
			parts = append(strings.Split(strings.TrimPrefix(path.Clean(target), "/"), "/"), parts...)
			cur = "/"
			continue
		}
		if _, ok := f.files[next]; !ok && !f.dirs[next] {
			return "", false
		}
		cur = next
	}
	return cur, true
}

// walk is Talos's archiver.Walker over this filesystem.
func (f *sysfs) walk(root string, maxDepth int, followRoot bool) ([]*machine.FileInfo, error) {
	// Lstat: every component but the last is resolved, the last is looked at
	// as it is.
	dir, ok := f.resolve(path.Dir(root))
	if !ok {
		return nil, fmt.Errorf("lstat %s: no such file or directory", root)
	}
	at := path.Join(dir, path.Base(root))
	if root == "/" {
		at = "/"
	}

	if target, isLink := f.links[at]; isLink {
		if !followRoot {
			return []*machine.FileInfo{{
				Name:         root,
				RelativeName: path.Base(root),
				Link:         target,
				Mode:         0o777,
			}}, nil
		}
		resolved, ok := f.resolve(at)
		if !ok {
			return nil, fmt.Errorf("lstat %s: no such file or directory", root)
		}
		at = resolved
	}

	if content, isFile := f.files[at]; isFile {
		return []*machine.FileInfo{{
			Name:         at,
			RelativeName: path.Base(at),
			Size:         int64(len(content)),
			Mode:         0o444,
		}}, nil
	}
	if !f.dirs[at] {
		return nil, fmt.Errorf("lstat %s: no such file or directory", root)
	}

	items := []*machine.FileInfo{{Name: at, RelativeName: ".", IsDir: true, Mode: 0o755}}
	f.walkDir(at, at, maxDepth, &items)
	return items, nil
}

func (f *sysfs) walkDir(root, dir string, maxDepth int, items *[]*machine.FileInfo) {
	for _, name := range f.children(dir) {
		full := path.Join(dir, name)
		rel := strings.TrimPrefix(strings.TrimPrefix(full, root), "/")

		switch {
		case f.links[full] != "":
			*items = append(*items, &machine.FileInfo{Name: full, RelativeName: rel, Link: f.links[full], Mode: 0o777})
		case f.dirs[full]:
			*items = append(*items, &machine.FileInfo{Name: full, RelativeName: rel, IsDir: true, Mode: 0o755})
			if maxDepth == -1 || strings.Count(rel, "/") < maxDepth {
				f.walkDir(root, full, maxDepth, items)
			}
		default:
			*items = append(*items, &machine.FileInfo{
				Name: full, RelativeName: rel, Size: int64(len(f.files[full])), Mode: 0o444,
			})
		}
	}
}

// children lists a directory's immediate entries, sorted as filepath.Walk
// sorts them.
func (f *sysfs) children(dir string) []string {
	seen := map[string]bool{}
	add := func(p string) {
		if p != "/" && path.Dir(p) == dir {
			seen[path.Base(p)] = true
		}
	}
	for p := range f.files {
		add(p)
	}
	for p := range f.links {
		add(p)
	}
	for p := range f.dirs {
		add(p)
	}

	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// newSimulatedSysfs builds the /sys a node with these options would have.
//
// The shape is copied from real machines rather than invented: /sys/class
// entries are symlinks into /sys/devices, a chip's attributes sit in its hwmon
// directory beside a "device" link to its parent, and an NVMe drive's model is
// padded with spaces to forty characters, as the controller reports it. Each
// of those is a way a reader can be wrong that a tidier fake would not catch.
func newSimulatedSysfs(opts Options) *sysfs {
	f := newSysfs()
	f.dir("/sys/class/hwmon")
	f.dir("/sys/class/thermal")

	switch opts.Sensors {
	case SensorsNone:
		return f

	case SensorsThermalZoneOnly:
		zone := "/sys/devices/virtual/thermal/thermal_zone0"
		f.link("/sys/class/thermal/thermal_zone0", "../../devices/virtual/thermal/thermal_zone0")
		f.file(zone+"/type", "cpu-thermal")
		f.file(zone+"/temp", "48312")
		// A cooling device sits beside the zones in the same class
		// directory, and is not one.
		f.link("/sys/class/thermal/cooling_device0", "../../devices/virtual/thermal/cooling_device0")
		f.file("/sys/devices/virtual/thermal/cooling_device0/type", "gpio-fan")
		return f
	}

	chip := 0
	hwmon := func(device string) string {
		dir := fmt.Sprintf("%s/hwmon/hwmon%d", device, chip)
		f.link(fmt.Sprintf("/sys/class/hwmon/hwmon%d", chip), "../../"+strings.TrimPrefix(dir, "/sys/"))
		f.link(dir+"/device", "../../../"+path.Base(device))
		chip++
		return dir
	}

	cpu := hwmon("/sys/devices/platform/coretemp.0")
	f.file(cpu+"/name", "coretemp")
	f.file(cpu+"/temp1_input", "57000")
	f.file(cpu+"/temp1_label", "Package id 0")
	f.file(cpu+"/temp1_max", "80000")
	f.file(cpu+"/temp1_crit", "100000")
	f.file(cpu+"/temp1_crit_alarm", "0")
	f.file(cpu+"/temp2_input", "55000")
	f.file(cpu+"/temp2_label", "Core 0")
	f.file(cpu+"/temp2_max", "80000")
	f.file(cpu+"/temp2_crit", "100000")
	f.file(cpu+"/uevent", "")
	f.dir(cpu + "/power")

	controllers := 0
	for _, d := range opts.Disks {
		if d.Transport != "nvme" {
			// A SATA drive reports a temperature only through drivetemp,
			// which Talos does not load: its disk row carries no reading.
			continue
		}
		controller := fmt.Sprintf("/sys/devices/pci0000:00/0000:00:1d.%d/0000:0%d:00.0/nvme/nvme%d",
			controllers, controllers+3, controllers)
		f.file(controller+"/model", fmt.Sprintf("%-40s", d.Model))
		f.file(controller+"/serial", fmt.Sprintf("%-20s", d.Serial))

		// Each drive three degrees warmer than the one before, so that a
		// reader that attached drive temperatures to the wrong drives would
		// show a different number rather than the same one twice.
		warmer := 3000 * controllers
		controllers++

		nvme := hwmon(controller)
		f.file(nvme+"/name", "nvme")
		f.file(nvme+"/temp1_input", strconv.Itoa(41850+warmer))
		f.file(nvme+"/temp1_label", "Composite")
		f.file(nvme+"/temp1_max", "84850")
		f.file(nvme+"/temp1_crit", "84850")
		f.file(nvme+"/temp2_input", strconv.Itoa(39850+warmer))
		f.file(nvme+"/temp2_label", "Sensor 1")
	}

	board := hwmon("/sys/devices/platform/nct6775.656")
	f.file(board+"/name", "nct6798")
	// No thresholds on the board temperature, as on most Super I/O inputs:
	// the view has to carry a missing limit as null rather than as zero.
	f.file(board+"/temp1_input", "34000")
	f.file(board+"/temp1_label", "SYSTIN")
	// No labels on the fans either -- nct6775 does not provide them.
	f.file(board+"/fan1_input", "1080")
	f.file(board+"/fan2_input", "0")
	f.file(board+"/fan1_min", "0")
	f.file(board+"/pwm1", "128")

	// A thermal zone the CPU chip already covers. It must not be listed a
	// second time.
	f.link("/sys/class/thermal/thermal_zone0", "../../devices/virtual/thermal/thermal_zone0")
	f.file("/sys/devices/virtual/thermal/thermal_zone0/type", "x86_pkg_temp")
	f.file("/sys/devices/virtual/thermal/thermal_zone0/temp", "57000")

	return f
}
