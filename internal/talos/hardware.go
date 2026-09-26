package talos

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
)

// The live hardware read: what a node's counters and sensors say right now.
//
// It is the one read in this package that is taken on demand and thrown away,
// and that is a deliberate exception to D-19 rather than an oversight of it.
// NodeFacts reads resources because a node's identity, disks and links are
// state Talos already maintains, and re-asking for state is the blind polling
// INV-13 rules out. A CPU's busy time and a fan's speed are not state: Talos
// keeps no resource for them, they are different every second, and the only
// honest answer to "how hot is it now" is to ask now. What keeps this from
// becoming a poller is where the asking comes from -- an operator with the
// hardware panel open -- and that nothing here is remembered beyond the one
// previous reading the rates are computed against (internal/inventory).
//
// Everything below is raw: cumulative counters, bytes, millidegrees turned into
// degrees and nothing more. The arithmetic that turns two readings into a
// percentage, and the choices about which disks and links a person wants to
// see, belong to the view that makes them; this package carries facts.

// sectorBytes is the unit /proc/diskstats counts in. It is 512 whatever the
// device's own sector size is -- the kernel normalises the counters -- so a 4K
// NVMe drive is not four thousand times faster than it looks.
const sectorBytes = 512

// kibibyte is the unit /proc/meminfo counts in, despite spelling it "kB".
const kibibyte = 1024

// HardwareCounters is the cumulative half of a reading: numbers that only mean
// something as a difference between two of them.
type HardwareCounters struct {
	// At is when the counters were read, on this process's clock. The window
	// a rate is computed over is the difference between two of these, which is
	// why both ends of it have to come from the same clock.
	At time.Time

	// Boot is when the node booted. Two readings from different boots cannot be
	// subtracted: every counter restarted from zero in between.
	Boot time.Time

	// CPUTotal is the whole machine; CPUs is one entry per logical CPU in the
	// kernel's order.
	CPUTotal CPUTimes
	CPUs     []CPUTimes

	// Disks and Links are keyed by the kernel's device name. Every device the
	// kernel counts is here -- partitions, loop devices, veths -- because which
	// of them a person wants to see is a question for the view.
	Disks map[string]DiskIO
	Links map[string]LinkIO
}

// CPUTimes is where one CPU's time has gone since boot, in seconds.
//
// Guest and GuestNice are deliberately absent. The kernel already counts guest
// time inside User and Nice, so adding them again would count a virtual
// machine's CPU twice.
type CPUTimes struct {
	User, Nice, System, Idle, Iowait, Irq, SoftIrq, Steal float64
}

// DiskIO is one block device's transferred bytes since boot.
type DiskIO struct {
	ReadBytes, WrittenBytes uint64
}

// LinkIO is one network device's transferred bytes since boot.
type LinkIO struct {
	RxBytes, TxBytes uint64
}

// MemoryInfo is /proc/meminfo in bytes, the fields a memory bar needs.
type MemoryInfo struct {
	TotalBytes, FreeBytes, AvailableBytes        uint64
	BuffersBytes, CachedBytes, SReclaimableBytes uint64
	SwapTotalBytes, SwapFreeBytes                uint64
}

// LoadAverage is the kernel's run-queue average over one, five and fifteen
// minutes.
type LoadAverage struct {
	Load1, Load5, Load15 float64
}

// Mount is one mounted filesystem as the node reports it. Device is the
// kernel's name for what is mounted -- "/dev/nvme0n1p6", "overlay", "tmpfs" --
// and Talos's Mounts RPC carries no filesystem type, so that name is the only
// handle a view has for telling a disk from a pseudo-filesystem.
type Mount struct {
	Device         string
	MountPoint     string
	SizeBytes      uint64
	AvailableBytes uint64
}

// HardwareSample is one live reading of a node.
type HardwareSample struct {
	Counters HardwareCounters

	// Hostname is the node's own, or empty when it has not settled one.
	Hostname string

	// CPUs, BlockDevices and Links are the same resource reads NodeFacts
	// makes, taken again because a disk that was hot-plugged since the last
	// heartbeat is a disk the operator is looking at now.
	CPUs         []CPU
	BlockDevices []BlockDevice
	Links        []Link

	Memory MemoryInfo
	Load   LoadAverage
	Mounts []Mount

	// Sensors is every hwmon chip that carried a temperature or a fan, in the
	// kernel's order. Empty means the kernel exposes none, which is a fact
	// about the node's drivers and is reported as one.
	Sensors []ChipReading

	// Layout is the sensor layout this reading was taken with: the one the
	// caller passed if it still held, a freshly discovered one if it did not.
	// A caller that caches it passes it back next time and skips discovery.
	Layout *SensorLayout
}

// HardwareCounters reads only the cumulative counters.
//
// It is the half of a reading a rate needs a second copy of. A first request
// has no previous reading to subtract from, so it takes one of these, waits,
// and takes a full sample; reading the sensors and the resource state twice
// for that would cost the node work that tells nobody anything.
func (c *ClusterClient) HardwareCounters(ctx context.Context) (HardwareCounters, error) {
	var (
		stat  *machine.SystemStatResponse
		disks *machine.DiskStatsResponse
		nets  *machine.NetworkDeviceStatsResponse
	)

	// Concurrently, so that the three counters describe as nearly the same
	// instant as one connection allows: a CPU percentage and a disk rate
	// computed over windows a round trip apart would still be right, but the
	// window the view reports would only be right for one of them.
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		stat, err = c.conn.c.MachineClient.SystemStat(gctx, &emptypb.Empty{})
		return err
	})
	g.Go(func() error {
		var err error
		disks, err = c.conn.c.MachineClient.DiskStats(gctx, &emptypb.Empty{})
		return err
	})
	g.Go(func() error {
		var err error
		nets, err = c.conn.c.MachineClient.NetworkDeviceStats(gctx, &emptypb.Empty{})
		return err
	})
	if err := g.Wait(); err != nil {
		return HardwareCounters{}, err
	}

	// Stamped once all three have answered, which puts the stamp at most one
	// round trip after the node read them. Against the second-long windows the
	// rates are computed over, that is below the precision anybody reads a
	// throughput figure at.
	at := c.conn.now()

	if len(stat.GetMessages()) == 0 {
		return HardwareCounters{}, fmt.Errorf("talos: %s returned an empty system stat response", c.conn.target.Machine)
	}
	st := stat.GetMessages()[0]
	if st.GetBootTime() == 0 {
		return HardwareCounters{}, fmt.Errorf("talos: %s reports no boot time", c.conn.target.Machine)
	}

	out := HardwareCounters{
		At:       at,
		Boot:     time.Unix(int64(st.GetBootTime()), 0), //nolint:gosec // a boot time cannot overflow int64
		CPUTotal: cpuTimes(st.GetCpuTotal()),
		CPUs:     make([]CPUTimes, 0, len(st.GetCpu())),
		Disks:    map[string]DiskIO{},
		Links:    map[string]LinkIO{},
	}
	for _, cpu := range st.GetCpu() {
		out.CPUs = append(out.CPUs, cpuTimes(cpu))
	}

	for _, msg := range disks.GetMessages() {
		for _, d := range msg.GetDevices() {
			out.Disks[d.GetName()] = DiskIO{
				ReadBytes:    d.GetReadSectors() * sectorBytes,
				WrittenBytes: d.GetWriteSectors() * sectorBytes,
			}
		}
	}
	for _, msg := range nets.GetMessages() {
		for _, n := range msg.GetDevices() {
			out.Links[n.GetName()] = LinkIO{RxBytes: n.GetRxBytes(), TxBytes: n.GetTxBytes()}
		}
	}

	return out, nil
}

// HardwareSample reads everything the hardware view shows.
//
// known is the sensor layout a previous call returned, or nil. With one, only
// the sensors' input files are read; without one -- or when the one passed no
// longer fits the node -- the hwmon tree is discovered first. The layout is
// only ever read here, never written, because a caller caching it may hand the
// same one to several concurrent calls.
//
// Everything is read concurrently on the one connection. The reads are
// independent, and a panel refreshed every few seconds should cost the node one
// round trip's latency rather than the sum of fifteen.
func (c *ClusterClient) HardwareSample(ctx context.Context, known *SensorLayout) (HardwareSample, error) {
	var (
		s      HardwareSample
		layout *SensorLayout
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		s.Counters, err = c.HardwareCounters(gctx)
		return err
	})
	g.Go(func() error {
		var err error
		s.Memory, err = c.memory(gctx)
		return err
	})
	g.Go(func() error {
		var err error
		s.Load, err = c.loadAverage(gctx)
		return err
	})
	g.Go(func() error {
		var err error
		s.Mounts, err = c.mounts(gctx)
		return err
	})

	// The resource reads, through the same readers NodeFacts uses.
	st := c.COSI()
	g.Go(func() error {
		var err error
		s.Hostname, err = readHostname(gctx, st, "")
		return err
	})
	g.Go(func() error {
		var err error
		s.CPUs, err = readCPUs(gctx, st)
		return err
	})
	g.Go(func() error {
		var err error
		s.BlockDevices, err = readBlockDevices(gctx, st)
		return err
	})
	g.Go(func() error {
		var err error
		s.Links, err = readLinks(gctx, st)
		return err
	})

	g.Go(func() error {
		if known != nil {
			readings, stale, err := c.readSensors(gctx, known)
			if err != nil {
				return err
			}
			if !stale {
				s.Sensors, layout = readings, known
				return nil
			}
		}

		fresh, err := c.discoverSensors(gctx)
		if err != nil {
			return err
		}
		readings, _, err := c.readSensors(gctx, fresh)
		if err != nil {
			return err
		}
		s.Sensors, layout = readings, fresh
		return nil
	})

	if err := g.Wait(); err != nil {
		return HardwareSample{}, err
	}

	switch {
	case layout.Boot.IsZero():
		// Discovered in this call, concurrently with the read that says which
		// boot this is. Nothing else holds this layout yet, so it can still
		// be stamped.
		layout.Boot = s.Counters.Boot

	case !layout.Boot.Equal(s.Counters.Boot):
		// The node rebooted since the layout was discovered. Every file in it
		// may still exist and still be the wrong one: hwmon numbers chips in
		// the order their drivers probed, which is not fixed across boots, so
		// last boot's hwmon1 can be this boot's NVMe drive reading out as the
		// CPU. The readings just taken are thrown away rather than shown.
		fresh, err := c.discoverSensors(ctx)
		if err != nil {
			return HardwareSample{}, err
		}
		fresh.Boot = s.Counters.Boot
		readings, _, err := c.readSensors(ctx, fresh)
		if err != nil {
			return HardwareSample{}, err
		}
		s.Sensors, layout = readings, fresh
	}

	s.Layout = layout
	return s, nil
}

func (c *ClusterClient) memory(ctx context.Context) (MemoryInfo, error) {
	resp, err := c.conn.c.MachineClient.Memory(ctx, &emptypb.Empty{})
	if err != nil {
		return MemoryInfo{}, err
	}
	if len(resp.GetMessages()) == 0 {
		return MemoryInfo{}, fmt.Errorf("talos: %s returned an empty memory response", c.conn.target.Machine)
	}

	m := resp.GetMessages()[0].GetMeminfo()
	return MemoryInfo{
		TotalBytes:        m.GetMemtotal() * kibibyte,
		FreeBytes:         m.GetMemfree() * kibibyte,
		AvailableBytes:    m.GetMemavailable() * kibibyte,
		BuffersBytes:      m.GetBuffers() * kibibyte,
		CachedBytes:       m.GetCached() * kibibyte,
		SReclaimableBytes: m.GetSreclaimable() * kibibyte,
		SwapTotalBytes:    m.GetSwaptotal() * kibibyte,
		SwapFreeBytes:     m.GetSwapfree() * kibibyte,
	}, nil
}

func (c *ClusterClient) loadAverage(ctx context.Context) (LoadAverage, error) {
	resp, err := c.conn.c.MachineClient.LoadAvg(ctx, &emptypb.Empty{})
	if err != nil {
		return LoadAverage{}, err
	}
	if len(resp.GetMessages()) == 0 {
		return LoadAverage{}, fmt.Errorf("talos: %s returned an empty load average response", c.conn.target.Machine)
	}

	l := resp.GetMessages()[0]
	return LoadAverage{Load1: l.GetLoad1(), Load5: l.GetLoad5(), Load15: l.GetLoad15()}, nil
}

func (c *ClusterClient) mounts(ctx context.Context) ([]Mount, error) {
	resp, err := c.conn.c.MachineClient.Mounts(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}

	var out []Mount
	for _, msg := range resp.GetMessages() {
		for _, m := range msg.GetStats() {
			out = append(out, Mount{
				Device:         m.GetFilesystem(),
				MountPoint:     m.GetMountedOn(),
				SizeBytes:      m.GetSize(),
				AvailableBytes: m.GetAvailable(),
			})
		}
	}
	return out, nil
}

func cpuTimes(s *machine.CPUStat) CPUTimes {
	return CPUTimes{
		User:    s.GetUser(),
		Nice:    s.GetNice(),
		System:  s.GetSystem(),
		Idle:    s.GetIdle(),
		Iowait:  s.GetIowait(),
		Irq:     s.GetIrq(),
		SoftIrq: s.GetSoftIrq(),
		Steal:   s.GetSteal(),
	}
}
