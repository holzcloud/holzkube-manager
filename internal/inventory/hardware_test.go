package inventory_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// adoptedMachine adopts the fixture's cluster and returns the one machine in
// it. The hardware view reaches a node with the cluster's credentials, so a
// node nobody adopted has nothing to be read with.
func adoptedMachine(ctx context.Context, t *testing.T, f *fixture) model.MachineID {
	t.Helper()

	f.importCluster(ctx, t)
	machines, err := f.svc.Machines(ctx)
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}
	if len(machines) != 1 {
		t.Fatalf("the adoption recorded %d machines, want 1", len(machines))
	}
	return machines[0].ID
}

func temperatureLabelled(t *testing.T, v inventory.HardwareView, label string) inventory.HardwareTemperature {
	t.Helper()
	for _, tt := range v.Temperatures {
		if tt.Label == label {
			return tt
		}
	}
	t.Fatalf("no temperature labelled %q; the view has %+v", label, v.Temperatures)
	return inventory.HardwareTemperature{}
}

func diskNamed(t *testing.T, v inventory.HardwareView, name string) inventory.HardwareDisk {
	t.Helper()
	for _, d := range v.Disks {
		if d.Name == name {
			return d
		}
	}
	t.Fatalf("no disk %q; the view has %+v", name, v.Disks)
	return inventory.HardwareDisk{}
}

func near(t *testing.T, what string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Errorf("%s = %v, want %v (±%v)", what, got, want, tolerance)
	}
}

func nearPtr(t *testing.T, what string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Errorf("%s is null, want %v", what, want)
		return
	}
	near(t, what, *got, want, 0.051)
}

// TestHardwareDiscoversClassifiesAndAttachesTheSensors is the sensor half of
// the hardware view against a node whose /sys looks like a desktop board's
// (talossim/hardware.go): a coretemp chip, an nvme chip per NVMe drive and an
// nct6798 Super I/O chip.
//
// The two NVMe drives are the same model on purpose. Matching a drive
// temperature to a drive by model alone cannot tell them apart, so the only
// way both come out right is through the controller's serial number -- and
// the simulator makes the second drive three degrees warmer, so a swap shows
// up as a wrong number rather than as the same number twice.
func TestHardwareDiscoversClassifiesAndAttachesTheSensors(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{
		ControlPlane: true,
		Disks: []talossim.DiskFixture{
			{Device: "nvme0n1", Size: 1 << 40, Model: "Samsung SSD 980 1TB", Serial: "S64ANS0T123456", Transport: "nvme"},
			{Device: "nvme1n1", Size: 1 << 40, Model: "Samsung SSD 980 1TB", Serial: "S64ANS0T654321", Transport: "nvme"},
			{Device: "sda", Size: 4 << 40, Model: "WDC WD40EFRX-68N32N0", Serial: "WD-WCC7K1234567", Transport: "sata"},
		},
	})
	id := adoptedMachine(ctx, t, f)

	v, err := f.svc.Hardware(ctx, id)
	if err != nil {
		t.Fatalf("Hardware: %v", err)
	}

	// Classification, by the driver's name.
	for _, tc := range []struct {
		label, chip, kind string
		celsius           float64
	}{
		{"Package id 0", "coretemp", "cpu", 57},
		{"Core 0", "coretemp", "cpu", 55},
		{"nvme0n1 Composite", "nvme", "disk", 41.9},
		{"nvme0n1 Sensor 1", "nvme", "disk", 39.9},
		{"nvme1n1 Composite", "nvme", "disk", 44.9},
		{"SYSTIN", "nct6798", "board", 34},
	} {
		got := temperatureLabelled(t, v, tc.label)
		if got.Chip != tc.chip || got.Kind != tc.kind {
			t.Errorf("%q is chip %q kind %q, want chip %q kind %q", tc.label, got.Chip, got.Kind, tc.chip, tc.kind)
		}
		near(t, tc.label, got.Celsius, tc.celsius, 0.051)
	}

	// Thresholds are the chip's own, and a chip that sets none reads null
	// rather than a red line at zero.
	pkg := temperatureLabelled(t, v, "Package id 0")
	nearPtr(t, "Package id 0 high", pkg.HighC, 80)
	nearPtr(t, "Package id 0 critical", pkg.CriticalC, 100)
	if systin := temperatureLabelled(t, v, "SYSTIN"); systin.HighC != nil || systin.CriticalC != nil {
		t.Errorf("SYSTIN has limits %v/%v; the chip sets none, so both must be null", systin.HighC, systin.CriticalC)
	}

	// The thermal zone the CPU chip already covers is not listed twice.
	for _, tt := range v.Temperatures {
		if tt.Chip == "x86_pkg_temp" {
			t.Errorf("the x86_pkg_temp thermal zone is listed beside coretemp: %+v", tt)
		}
	}
	if len(v.Temperatures) != 7 {
		t.Errorf("the view has %d temperatures, want 7 (2 coretemp, 2 per NVMe drive, 1 nct6798): %+v",
			len(v.Temperatures), v.Temperatures)
	}

	// Attachment: each drive gets its own controller's Composite reading,
	// and the SATA drive, which has no sensor the kernel exposes, gets null.
	nearPtr(t, "nvme0n1 temperature", diskNamed(t, v, "nvme0n1").TemperatureC, 41.9)
	nearPtr(t, "nvme1n1 temperature", diskNamed(t, v, "nvme1n1").TemperatureC, 44.9)
	if c := diskNamed(t, v, "sda").TemperatureC; c != nil {
		t.Errorf("sda has temperature %v; it has no sensor, so it must be null", *c)
	}

	// Fans, the stopped one included.
	if len(v.Fans) != 2 {
		t.Fatalf("the view has %d fans, want 2: %+v", len(v.Fans), v.Fans)
	}
	for i, want := range []inventory.HardwareFan{
		{Chip: "nct6798", Label: "fan1", RPM: 1080},
		{Chip: "nct6798", Label: "fan2", RPM: 0},
	} {
		if v.Fans[i] != want {
			t.Errorf("fan %d = %+v, want %+v", i, v.Fans[i], want)
		}
	}

	if v.SensorsNotice != "" {
		t.Errorf("sensors_notice = %q on a node with temperatures and fans", v.SensorsNotice)
	}
}

// TestHardwareExplainsANodeWithoutSensors is the notice that keeps an empty
// sensor panel from reading as this product failing.
func TestHardwareExplainsANodeWithoutSensors(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true, Sensors: talossim.SensorsNone})
	id := adoptedMachine(ctx, t, f)

	v, err := f.svc.Hardware(ctx, id)
	if err != nil {
		t.Fatalf("Hardware: %v", err)
	}

	if v.Temperatures == nil || v.Fans == nil {
		t.Errorf("temperatures %v / fans %v: an empty list must be empty, not null", v.Temperatures, v.Fans)
	}
	if len(v.Temperatures) != 0 || len(v.Fans) != 0 {
		t.Errorf("a node with no sensors reports %d temperatures and %d fans", len(v.Temperatures), len(v.Fans))
	}
	for _, d := range v.Disks {
		if d.TemperatureC != nil {
			t.Errorf("disk %s has temperature %v on a node with no sensors", d.Name, *d.TemperatureC)
		}
	}

	notice := strings.ToLower(v.SensorsNotice)
	for _, want := range []string{"temperature", "fan", "kernel", "driver", "not a fault"} {
		if !strings.Contains(notice, want) {
			t.Errorf("sensors_notice %q does not say %q; it has to name the kernel's drivers as the cause "+
				"and clear the hardware and this product", v.SensorsNotice, want)
		}
	}
	if strings.Count(v.SensorsNotice, ". ") > 0 || !strings.HasSuffix(v.SensorsNotice, ".") {
		t.Errorf("sensors_notice %q is not one sentence", v.SensorsNotice)
	}
}

// TestHardwareDoesNotBlameTheKernelForARefusal is the other side of that
// notice. A node that refuses to let this product read its files is answering,
// and has said nothing about its sensors; reporting "your kernel has no sensor
// drivers" there would send the operator to the wrong machine for the wrong
// reason. The read fails instead, with the node's refusal in it.
func TestHardwareDoesNotBlameTheKernelForARefusal(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true, RefuseFileReads: true})
	id := adoptedMachine(ctx, t, f)

	v, err := f.svc.Hardware(ctx, id)
	if err == nil {
		t.Fatalf("Hardware succeeded against a node that refuses file reads, with sensors_notice %q",
			v.SensorsNotice)
	}
	if kind, ok := talos.ErrorKindOf(err); !ok || kind != talos.KindRejected {
		t.Errorf("the failure is %v (kind %v, classified %v), want the node's own refusal", err, kind, ok)
	}
}

// TestHardwareFallsBackToAThermalZone is a board whose CPU temperature exists
// only as a thermal zone -- no hwmon chip at all, which is a Raspberry Pi
// built without CONFIG_THERMAL_HWMON.
func TestHardwareFallsBackToAThermalZone(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true, Sensors: talossim.SensorsThermalZoneOnly})
	id := adoptedMachine(ctx, t, f)

	v, err := f.svc.Hardware(ctx, id)
	if err != nil {
		t.Fatalf("Hardware: %v", err)
	}

	if len(v.Temperatures) != 1 {
		t.Fatalf("the view has %d temperatures, want the one thermal zone (the cooling device beside it "+
			"is not one): %+v", len(v.Temperatures), v.Temperatures)
	}
	zone := v.Temperatures[0]
	if zone.Chip != "cpu-thermal" || zone.Kind != "cpu" || zone.Label != "thermal_zone0" {
		t.Errorf("the zone reads %+v, want chip cpu-thermal, kind cpu, label thermal_zone0", zone)
	}
	near(t, "cpu-thermal", zone.Celsius, 48.3, 0.051)

	// A board with a CPU temperature and no fan headers the kernel can see
	// still owes an explanation for the empty half.
	if !strings.Contains(v.SensorsNotice, "fan") || strings.Contains(v.SensorsNotice, "temperature") {
		t.Errorf("sensors_notice = %q, want the sentence about fans alone", v.SensorsNotice)
	}
}

// TestHardwareFindsSensorsItCannotList is the fallback for a node whose file
// listing does not resolve the symlinks /sys/class is made of: the chip
// directories cannot be listed, so the sensors are found by reading the file
// names hwmon uses and keeping the ones that exist.
func TestHardwareFindsSensorsItCannotList(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true, ListHidesSymlinks: true})
	id := adoptedMachine(ctx, t, f)

	v, err := f.svc.Hardware(ctx, id)
	if err != nil {
		t.Fatalf("Hardware: %v", err)
	}

	pkg := temperatureLabelled(t, v, "Package id 0")
	if pkg.Kind != "cpu" {
		t.Errorf("Package id 0 is kind %q, want cpu", pkg.Kind)
	}
	nearPtr(t, "Package id 0 critical", pkg.CriticalC, 100)
	nearPtr(t, "nvme0n1 temperature", diskNamed(t, v, "nvme0n1").TemperatureC, 41.9)
	if len(v.Temperatures) != 5 || len(v.Fans) != 2 {
		t.Errorf("found %d temperatures and %d fans by probing, want the 5 and 2 a listing finds: %+v %+v",
			len(v.Temperatures), len(v.Fans), v.Temperatures, v.Fans)
	}
}

// TestHardwareRatesAreTheNodesOwnCounters is the counter half: what the view
// computes from the node's counters matches the rates the simulator's counters
// advance at, and the devices a person does not want to see are left out.
func TestHardwareRatesAreTheNodesOwnCounters(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	id := adoptedMachine(ctx, t, f)

	v, err := f.svc.Hardware(ctx, id)
	if err != nil {
		t.Fatalf("Hardware: %v", err)
	}

	if v.Machine != id || v.Hostname != "cp-1" {
		t.Errorf("machine %q hostname %q, want %q and cp-1", v.Machine, v.Hostname, id)
	}

	// A first request has nothing remembered and takes its own second
	// reading one second after the first.
	near(t, "rates_over_seconds on a first request", v.RatesOverSeconds, 1, 0.5)

	// CPU: one simulated processor is four cores and eight threads, each
	// thread busy by SimulatedThreadBusy.
	if v.CPU.Model != "Simulated CPU" || v.CPU.Cores != 4 || v.CPU.Threads != 8 {
		t.Errorf("cpu = %q %d cores %d threads, want Simulated CPU, 4, 8", v.CPU.Model, v.CPU.Cores, v.CPU.Threads)
	}
	if len(v.CPU.PerCore) != talossim.SimulatedThreadsPerCPU {
		t.Fatalf("per_core has %d entries, want %d", len(v.CPU.PerCore), talossim.SimulatedThreadsPerCPU)
	}
	var sum float64
	for i, p := range v.CPU.PerCore {
		want := 100 * talossim.SimulatedThreadBusy(i)
		near(t, "per-core busy", p, want, 0.6)
		sum += want
	}
	near(t, "usage_percent", v.CPU.UsagePercent, sum/float64(len(v.CPU.PerCore)), 0.6)
	near(t, "iowait_percent", v.CPU.IowaitPercent, 100*talossim.SimulatedIowait, 0.3)
	near(t, "load1", v.CPU.Load1, 0.52, 0.001)

	// Disks: the one fixture drive, its partitions and the loop device left
	// out, at the rate the simulator's counters advance.
	if len(v.Disks) != 1 || v.Disks[0].Name != "nvme0n1" {
		t.Fatalf("disks = %+v, want nvme0n1 alone", v.Disks)
	}
	near(t, "nvme0n1 read", v.Disks[0].ReadBytesPerSec, talossim.SimulatedDiskReadBytesPerSec,
		0.05*talossim.SimulatedDiskReadBytesPerSec)
	near(t, "nvme0n1 write", v.Disks[0].WriteBytesPerSec, talossim.SimulatedDiskWriteBytesPerSec,
		0.05*talossim.SimulatedDiskWriteBytesPerSec)

	// Network: eth0 alone. Loopback and the pod's veth are in the node's
	// counters and are not a cable anybody plugged in.
	if len(v.Network) != 1 || v.Network[0].Name != "eth0" {
		t.Fatalf("network = %+v, want eth0 alone", v.Network)
	}
	if !v.Network[0].Up || v.Network[0].SpeedMbit != 1000 {
		t.Errorf("eth0 up=%v speed=%d, want up at 1000", v.Network[0].Up, v.Network[0].SpeedMbit)
	}
	near(t, "eth0 rx", v.Network[0].RxBytesPerSec, talossim.SimulatedRxBytesPerSec, 0.05*talossim.SimulatedRxBytesPerSec)
	near(t, "eth0 tx", v.Network[0].TxBytesPerSec, talossim.SimulatedTxBytesPerSec, 0.05*talossim.SimulatedTxBytesPerSec)

	// Filesystems: STATE and EPHEMERAL, once each. The squashfs root, the
	// pseudo-filesystems and EPHEMERAL's bind mounts are not disks.
	if len(v.Filesystems) != 2 {
		t.Fatalf("filesystems = %+v, want /system/state and /var", v.Filesystems)
	}
	if fs := v.Filesystems[1]; fs.Mount != "/var" || fs.Device != "/dev/nvme0n1p6" || fs.UsedBytes == 0 ||
		fs.UsedBytes >= fs.SizeBytes {
		t.Errorf("EPHEMERAL = %+v, want /var on /dev/nvme0n1p6, partly used", fs)
	}
	if v.Filesystems[0].Mount != "/system/state" {
		t.Errorf("first filesystem = %+v, want /system/state", v.Filesystems[0])
	}

	// Memory: used is total minus available, not total minus free.
	if total := uint64(16384) << 20; v.Memory.TotalBytes != total {
		t.Errorf("total_bytes = %d, want %d", v.Memory.TotalBytes, total)
	}
	if v.Memory.UsedBytes+v.Memory.AvailableBytes != v.Memory.TotalBytes || v.Memory.CacheBytes == 0 {
		t.Errorf("memory = %+v: used + available must be total, and the page cache is not zero", v.Memory)
	}
}

// TestHardwareMeasuresBetweenTwoRequests is the property that makes an open
// panel cheap: the second request computes its rates against the first one's
// reading and takes no second reading of its own.
//
// It is observed through the window the view reports. A request that took its
// own pair of readings reports a window of about a second whatever happened
// before it; one measured against the previous request reports the time since
// that request, which here is made twice as long.
func TestHardwareMeasuresBetweenTwoRequests(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	id := adoptedMachine(ctx, t, f)

	if _, err := f.svc.Hardware(ctx, id); err != nil {
		t.Fatalf("first Hardware: %v", err)
	}
	time.Sleep(2 * time.Second)

	// Asserted on the window and not on how long the call took: a loaded
	// runner can make any call slow, but only the remembered baseline can
	// make the window span the two seconds slept above.
	v, err := f.svc.Hardware(ctx, id)
	if err != nil {
		t.Fatalf("second Hardware: %v", err)
	}

	if v.RatesOverSeconds < 1.9 || v.RatesOverSeconds > 10 {
		t.Errorf("the second request's rates are over %.3fs; measured against the first request they "+
			"would be over the two seconds between them", v.RatesOverSeconds)
	}
	near(t, "eth0 rx over the longer window", v.Network[0].RxBytesPerSec, talossim.SimulatedRxBytesPerSec,
		0.05*talossim.SimulatedRxBytesPerSec)
}

// TestHardwarePollsReadOnlyTheSensorInputs pins what the cached sensor layout
// buys and when it is thrown away. A poll with a layout reads one file per
// sensor; discovery reads dozens. After the node reboots the layout is
// discovered again, because hwmon numbers its chips in probe order and last
// boot's hwmon1 is not necessarily this boot's.
func TestHardwarePollsReadOnlyTheSensorInputs(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	id := adoptedMachine(ctx, t, f)

	// The default desktop board: two coretemp inputs, two for the one NVMe
	// drive, the SYSTIN input and two fans.
	const inputs = 7

	if _, err := f.svc.Hardware(ctx, id); err != nil {
		t.Fatalf("first Hardware: %v", err)
	}
	discovery := f.sim.Calls("Read")
	if discovery <= inputs {
		t.Fatalf("discovery made %d Read calls; it has at least the %d inputs plus every name and label "+
			"to read, so the count is not measuring discovery", discovery, inputs)
	}

	time.Sleep(600 * time.Millisecond)
	if _, err := f.svc.Hardware(ctx, id); err != nil {
		t.Fatalf("second Hardware: %v", err)
	}
	if polled := f.sim.Calls("Read") - discovery; polled != inputs {
		t.Errorf("a poll with a cached layout made %d Read calls, want %d: one per sensor input", polled, inputs)
	}

	// Reboot the node, through its own API, the way a job would.
	cc, err := talos.NewClusterClient(ctx, f.sim.Dialer(), talos.Target{
		Machine: id, Addr: f.sim.Host(),
	}, f.sim.ClientCreds(), talos.Mode{})
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}
	defer func() { _ = cc.Close() }()
	rebootCtx, cancel := context.WithTimeout(ctx, talos.MutationDeadline)
	defer cancel()
	if err := cc.Reboot(rebootCtx); err != nil {
		t.Fatalf("Reboot: %v", err)
	}

	before := f.sim.Calls("Read")
	v, err := f.svc.Hardware(ctx, id)
	if err != nil {
		t.Fatalf("Hardware after the reboot: %v", err)
	}
	if after := f.sim.Calls("Read") - before; after <= inputs {
		t.Errorf("the first poll after a reboot made %d Read calls, which is a poll of the old layout; "+
			"it has to be discovered again", after)
	}
	if len(v.Temperatures) != 5 || len(v.Fans) != 2 {
		t.Errorf("after the reboot the view has %d temperatures and %d fans, want 5 and 2",
			len(v.Temperatures), len(v.Fans))
	}
}

// TestHardwareOfAnUnknownMachineIsNotFound is the error the HTTP edge turns
// into a 404.
func TestHardwareOfAnUnknownMachineIsNotFound(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})

	_, err := f.svc.Hardware(ctx, "00000000-0000-0000-0000-00000000dead")
	if !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("Hardware of a machine nobody adopted returned %v, want ErrNotFound", err)
	}
}
