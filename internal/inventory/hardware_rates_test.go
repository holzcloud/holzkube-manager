package inventory

import (
	"math"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// TestRatesAreComputedFromTwoReadings pins the arithmetic behind every
// percentage and every per-second figure on the hardware panel, against
// numbers worked out by hand.
//
// The readings are two seconds apart on a four-CPU machine, so eight seconds of
// CPU time passed. The deltas are chosen so each figure has exactly one right
// answer and each plausible mistake a different wrong one: counting I/O wait as
// busy gives 22.5 instead of 20, dividing by the wall-clock window instead of
// the CPU time gives 80, and reporting the total in every per-core slot gives
// 20 twice instead of 50 and 5.
func TestRatesAreComputedFromTwoReadings(t *testing.T) {
	t.Parallel()

	boot := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	at := boot.Add(time.Hour)

	prev := talos.HardwareCounters{
		At:   at,
		Boot: boot,
		CPUTotal: talos.CPUTimes{
			User: 1000, Nice: 10, System: 300, Idle: 5000, Iowait: 40, Irq: 5, SoftIrq: 5,
		},
		CPUs: []talos.CPUTimes{
			{User: 500, System: 100, Idle: 2400, Iowait: 20},
			{User: 100, System: 50, Idle: 2800, Iowait: 20},
		},
		Disks: map[string]talos.DiskIO{
			"nvme0n1": {ReadBytes: 1_000_000, WrittenBytes: 500_000},
		},
		Links: map[string]talos.LinkIO{
			"eth0": {RxBytes: 10_000_000, TxBytes: 2_000_000},
			// Recreated between the readings: its counters start again.
			"eth1": {RxBytes: 100, TxBytes: 100},
		},
	}

	cur := prev
	cur.At = at.Add(2 * time.Second)
	cur.CPUTotal = talos.CPUTimes{
		// +1.2 user, +0.4 system, +6.2 idle, +0.2 iowait: 8.0 s in all.
		User: 1001.2, Nice: 10, System: 300.4, Idle: 5006.2, Iowait: 40.2, Irq: 5, SoftIrq: 5,
	}
	cur.CPUs = []talos.CPUTimes{
		// +0.9 user, +0.1 system, +1.0 idle: busy half the time.
		{User: 500.9, System: 100.1, Idle: 2401, Iowait: 20},
		// +0.1 user, +1.7 idle, +0.2 iowait: busy five percent.
		{User: 100.1, System: 50, Idle: 2801.7, Iowait: 20.2},
		// A CPU that came online between the readings.
		{User: 3, Idle: 7},
	}
	cur.Disks = map[string]talos.DiskIO{
		"nvme0n1": {ReadBytes: 1_000_000 + 8<<20, WrittenBytes: 500_000 + 2<<20},
		// Appeared between the readings: nothing to subtract from.
		"sdb": {ReadBytes: 1 << 30},
	}
	cur.Links = map[string]talos.LinkIO{
		"eth0": {RxBytes: 12_500_000, TxBytes: 2_500_000},
		"eth1": {RxBytes: 50, TxBytes: 60},
	}

	r := computeRates(prev, cur)

	approx(t, "window", r.window, 2)
	approx(t, "usage", r.usage, 20)
	approx(t, "iowait", r.iowait, 2.5)

	if len(r.perCore) != 3 {
		t.Fatalf("per-core has %d entries, want one per CPU in the current reading (3)", len(r.perCore))
	}
	approx(t, "per-core 0", r.perCore[0], 50)
	approx(t, "per-core 1", r.perCore[1], 5)
	approx(t, "per-core 2 (no previous reading)", r.perCore[2], 0)

	approx(t, "nvme0n1 read", r.disks["nvme0n1"].in, 4<<20)
	approx(t, "nvme0n1 write", r.disks["nvme0n1"].out, 1<<20)
	if _, ok := r.disks["sdb"]; ok {
		t.Error("a disk with no previous reading was given a rate; its whole lifetime's traffic is not a throughput")
	}

	approx(t, "eth0 rx", r.links["eth0"].in, 1_250_000)
	approx(t, "eth0 tx", r.links["eth0"].out, 250_000)
	approx(t, "eth1 rx (counter went backwards)", r.links["eth1"].in, 0)
	approx(t, "eth1 tx (counter went backwards)", r.links["eth1"].out, 0)
}

// TestRatesOfAnIdleWindowAreZeroNotNaN is the division a stalled clock or a
// frozen counter would otherwise make by zero. NaN does not survive JSON
// encoding at all, so it would not be a wrong number on the panel but a
// response that fails to encode.
func TestRatesOfAnIdleWindowAreZeroNotNaN(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 9, 26, 9, 0, 0, 0, time.UTC)
	same := talos.HardwareCounters{
		At:       at,
		CPUTotal: talos.CPUTimes{User: 10, Idle: 90},
		CPUs:     []talos.CPUTimes{{User: 10, Idle: 90}},
		Disks:    map[string]talos.DiskIO{"sda": {ReadBytes: 1}},
	}

	r := computeRates(same, same)
	for name, v := range map[string]float64{
		"usage": r.usage, "iowait": r.iowait, "per-core": r.perCore[0], "sda read": r.disks["sda"].in,
	} {
		if math.IsNaN(v) || v != 0 {
			t.Errorf("%s over an empty window = %v, want 0", name, v)
		}
	}
}

// TestABaselineIsOnlyUsedInsideItsWindow pins when the previous request's
// reading stands in for a second one.
func TestABaselineIsOnlyUsedInsideItsWindow(t *testing.T) {
	t.Parallel()

	boot := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	prev := talos.HardwareCounters{At: boot.Add(time.Hour), Boot: boot}

	for _, tc := range []struct {
		name  string
		after time.Duration
		boot  time.Time
		have  bool
		want  bool
	}{
		{"nothing remembered", 3 * time.Second, boot, false, false},
		{"too close: two tabs on one node", 499 * time.Millisecond, boot, true, false},
		{"the shortest usable window", 500 * time.Millisecond, boot, true, true},
		{"an ordinary refresh", 3 * time.Second, boot, true, true},
		{"the longest usable window", 5 * time.Minute, boot, true, true},
		{"too old: an average, not a reading", 5*time.Minute + time.Second, boot, true, false},
		{"across a reboot", 3 * time.Second, boot.Add(time.Minute), true, false},
		{"from the future: the clock moved back", -3 * time.Second, boot, true, false},
	} {
		cur := talos.HardwareCounters{At: prev.At.Add(tc.after), Boot: tc.boot}
		if got := usableBaseline(prev, tc.have, cur); got != tc.want {
			t.Errorf("%s: usableBaseline = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func approx(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6*math.Max(1, math.Abs(want)) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}
