package host

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// The /proc files the Live section reads. Under the production unit's
// ProcSubset=pid every one of them is gone (measured), and the page says so
// rather than drawing a 0 (D-02, D-03).
const (
	pathProcStat    = "/proc/stat"
	pathProcMeminfo = "/proc/meminfo"
	pathProcLoadavg = "/proc/loadavg"
)

// userHZ is the unit /proc/stat counts CPU time in. The kernel fixes it at 100
// for this file whatever CONFIG_HZ is (it converts before printing), which is
// why no sysconf(_SC_CLK_TCK) is needed.
const userHZ = 100

// parseProcStat reads the CPU lines of /proc/stat, in seconds: the "cpu " line
// is the machine's total and the "cpuN" lines are the cores in order.
//
// Only the first eight columns are taken -- user nice system idle iowait irq
// softirq steal. Guest and guest_nice are already counted inside user and nice,
// so adding them would count a virtual machine's time twice (as talos.CPUTimes
// says).
func parseProcStat(raw []byte) (total talos.CPUTimes, cores []talos.CPUTimes, err error) {
	cores = make([]talos.CPUTimes, 0, 8)
	haveTotal := false
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		t, err := cpuTimesOf(fields)
		if err != nil {
			return talos.CPUTimes{}, nil, err
		}
		if fields[0] == "cpu" {
			total, haveTotal = t, true
			continue
		}
		if _, err := strconv.Atoi(strings.TrimPrefix(fields[0], "cpu")); err != nil {
			return talos.CPUTimes{}, nil, fmt.Errorf("unexpected line %q", fields[0])
		}
		cores = append(cores, t)
	}
	if err := sc.Err(); err != nil {
		return talos.CPUTimes{}, nil, err
	}
	if !haveTotal {
		return talos.CPUTimes{}, nil, errors.New(`no "cpu" line`)
	}
	return total, cores, nil
}

func cpuTimesOf(fields []string) (talos.CPUTimes, error) {
	// Kernels older than 2.6.33 wrote fewer columns; every kernel this product
	// can run on writes at least the eight used here.
	if len(fields) < 9 {
		return talos.CPUTimes{}, fmt.Errorf("%s has %d columns, want at least 8", fields[0], len(fields)-1)
	}
	var v [8]float64
	for i := range v {
		ticks, err := strconv.ParseUint(fields[i+1], 10, 64)
		if err != nil {
			return talos.CPUTimes{}, fmt.Errorf("%s column %d: %w", fields[0], i+1, err)
		}
		v[i] = float64(ticks) / userHZ
	}
	return talos.CPUTimes{
		User: v[0], Nice: v[1], System: v[2], Idle: v[3],
		Iowait: v[4], Irq: v[5], SoftIrq: v[6], Steal: v[7],
	}, nil
}

// parseMeminfo reads /proc/meminfo into bytes.
//
// MemTotal and MemAvailable are required: without either, "used" cannot be
// computed the way free(1) computes it, and a guess would be a wrong reading.
// The others are 0 when absent -- a kernel without swap writes SwapTotal 0
// anyway, and one without SReclaimable is old enough not to have it.
func parseMeminfo(raw []byte) (talos.MemoryInfo, error) {
	var m talos.MemoryInfo
	fields := map[string]*uint64{
		"MemTotal":     &m.TotalBytes,
		"MemFree":      &m.FreeBytes,
		"MemAvailable": &m.AvailableBytes,
		"Buffers":      &m.BuffersBytes,
		"Cached":       &m.CachedBytes,
		"SReclaimable": &m.SReclaimableBytes,
		"SwapTotal":    &m.SwapTotalBytes,
		"SwapFree":     &m.SwapFreeBytes,
	}
	seen := map[string]bool{}

	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		key, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		dst, want := fields[key]
		if !want {
			continue
		}
		parts := strings.Fields(rest)
		if len(parts) != 2 || parts[1] != "kB" {
			return talos.MemoryInfo{}, fmt.Errorf("%s: %q is not a kB value", key, strings.TrimSpace(rest))
		}
		kb, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			return talos.MemoryInfo{}, fmt.Errorf("%s: %w", key, err)
		}
		*dst = kb * 1024
		seen[key] = true
	}
	if err := sc.Err(); err != nil {
		return talos.MemoryInfo{}, err
	}
	for _, need := range []string{"MemTotal", "MemAvailable"} {
		if !seen[need] {
			return talos.MemoryInfo{}, fmt.Errorf("no %s line", need)
		}
	}
	return m, nil
}

// parseLoadavg reads the three averages of /proc/loadavg.
func parseLoadavg(raw []byte) (Load, error) {
	f := strings.Fields(string(raw))
	if len(f) < 3 {
		return Load{}, fmt.Errorf("%q is not a load average", strings.TrimSpace(string(raw)))
	}
	var v [3]float64
	for i := range v {
		x, err := strconv.ParseFloat(f[i], 64)
		if err != nil {
			return Load{}, fmt.Errorf("load average %q: %w", f[i], err)
		}
		v[i] = x
	}
	return Load{Load1: v[0], Load5: v[1], Load15: v[2], Source: loadSourceLoadavg}, nil
}

// loadFromSysinfo turns one of sysinfo(2)'s load averages into the number
// /proc/loadavg prints for the same instant.
//
// sysinfo hands out the kernel's avenrun shifted from FSHIFT (11) up to
// SI_LOAD_SHIFT (16). /proc/loadavg prints avenrun with LOAD_INT/LOAD_FRAC after
// adding FIXED_1/200, which rounds to hundredths the kernel's way
// (fs/proc/loadavg.c). Doing the same here gives the same two decimals; a naive
// float division rounded to two places can differ in the last one.
func loadFromSysinfo(l uint64) float64 {
	x := l >> (16 - 11)
	x += (1 << 11) / 200
	whole := x >> 11
	frac := ((x & (1<<11 - 1)) * 100) >> 11
	// One division of an exact integer, so 729/100 is the float64 nearest to
	// 7.29 -- the same one ParseFloat("7.29") gives for /proc/loadavg.
	return float64(whole*100+frac) / 100
}

// The two places a load average can come from.
const (
	loadSourceLoadavg = "loadavg"
	loadSourceSysinfo = "sysinfo"
)

// classify is the Reason for a failed read of a /proc file.
//
// The hardening is named only with proof: the file is absent AND the /proc
// this process sees is mounted subset=pid. Absence alone is not enough -- a
// container without that file would be misreported, and systemd silently skips
// subset= on a kernel that does not have it, leaving the files present. Any
// other failure, including permission denied under the hardening, is reported
// as the failure it was.
func classify(path string, err error, subsetPid bool) Reason {
	if errors.Is(err, fs.ErrNotExist) && subsetPid {
		return Reason{
			Code: CodeProcSubset,
			Message: "Hidden by the unit's ProcSubset=pid: " + path +
				" is not visible to holzkube-manager. Set ProcSubset=all in the unit's [Service] section to show it.",
		}
	}
	return reasonFor(path, err)
}
