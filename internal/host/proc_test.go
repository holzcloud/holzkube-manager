package host

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"syscall"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

func TestProcStat(t *testing.T) {
	t.Parallel()

	raw, err := fs.ReadFile(fixtureFS(t, "pi5"), "proc/stat")
	if err != nil {
		t.Fatal(err)
	}
	total, cores, err := parseProcStat(raw)
	if err != nil {
		t.Fatal(err)
	}

	// ticks / 100; the fixture's guest columns are 0 and must not appear
	// anywhere (they are already inside user and nice).
	want := talos.CPUTimes{User: 4000, Nice: 10, System: 1500, Idle: 50000, Iowait: 200, Irq: 0, SoftIrq: 50, Steal: 0}
	if total != want {
		t.Errorf("total = %+v, want %+v", total, want)
	}
	if len(cores) != 4 {
		t.Fatalf("got %d cores, want 4", len(cores))
	}
	wantCore := talos.CPUTimes{User: 1000, Nice: 2.5, System: 375, Idle: 12500, Iowait: 50, SoftIrq: 12.5}
	for i, c := range cores {
		if c != wantCore {
			t.Errorf("cpu%d = %+v, want %+v", i, c, wantCore)
		}
	}

	t.Run("guest columns are ignored", func(t *testing.T) {
		t.Parallel()
		total, _, err := parseProcStat([]byte("cpu  100 0 0 0 0 0 0 0 7000 7000\n"))
		if err != nil {
			t.Fatal(err)
		}
		if total.User != 1 || total.Nice != 0 {
			t.Errorf("total = %+v, want user 1 s and nothing from guest", total)
		}
	})

	t.Run("no cpu line", func(t *testing.T) {
		t.Parallel()
		if _, _, err := parseProcStat([]byte("intr 1 2 3\n")); err == nil {
			t.Error("a /proc/stat without a cpu line parsed")
		}
	})

	t.Run("a short cpu line", func(t *testing.T) {
		t.Parallel()
		if _, _, err := parseProcStat([]byte("cpu  1 2 3 4\n")); err == nil {
			t.Error("a cpu line with four columns parsed")
		}
	})
}

func TestMeminfo(t *testing.T) {
	t.Parallel()

	raw, err := fs.ReadFile(fixtureFS(t, "pi5"), "proc/meminfo")
	if err != nil {
		t.Fatal(err)
	}
	info, err := parseMeminfo(raw)
	if err != nil {
		t.Fatal(err)
	}

	// Every column of `free -b` on the measured sample (research, "Memory
	// (HMON-01)"), through the inventory's own formula.
	got := inventory.MemoryView(info)
	want := inventory.HardwareMemory{
		TotalBytes:     8453947392,
		UsedBytes:      4093853696,
		CacheBytes:     3259285504,
		AvailableBytes: 4360093696,
		SwapTotalBytes: 2147467264,
		SwapUsedBytes:  1498169344,
	}
	if got != want {
		t.Errorf("memory = %+v\n     want %+v", got, want)
	}

	for _, missing := range []string{"MemTotal", "MemAvailable"} {
		t.Run("no "+missing, func(t *testing.T) {
			t.Parallel()
			var kept []string
			for _, line := range strings.Split(string(raw), "\n") {
				if !strings.HasPrefix(line, missing+":") {
					kept = append(kept, line)
				}
			}
			if _, err := parseMeminfo([]byte(strings.Join(kept, "\n"))); err == nil {
				t.Errorf("meminfo without %s parsed", missing)
			}
		})
	}
}

func TestLoadavg(t *testing.T) {
	t.Parallel()

	got, err := parseLoadavg([]byte("7.29 4.90 2.98 3/1234 5678\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := (Load{Load1: 7.29, Load5: 4.90, Load15: 2.98, Source: "loadavg"}); got != want {
		t.Errorf("load = %+v, want %+v", got, want)
	}
	if _, err := parseLoadavg([]byte("7.29\n")); err == nil {
		t.Error("a one-field loadavg parsed")
	}
}

func TestLoadFromSysinfo(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		raw  uint64
		want float64
	}{
		{0, 0.00},
		{65536, 1.00},
		// The measured instant: /proc/loadavg said 7.29 4.90 2.98.
		{477760, 7.29},
		{321152, 4.90},
		{195328, 2.98},
	} {
		if got := loadFromSysinfo(tc.raw); got != tc.want {
			t.Errorf("loadFromSysinfo(%d) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

// TestClassifyNeedsTheMountAsProof is D-03: a missing /proc file is the
// hardening only when the mount says subset=pid. Absence alone proves nothing.
func TestClassifyNeedsTheMountAsProof(t *testing.T) {
	t.Parallel()

	notExist := &fs.PathError{Op: "open", Path: "proc/stat", Err: syscall.ENOENT}
	denied := &fs.PathError{Op: "open", Path: "proc/stat", Err: syscall.EACCES}

	for _, tc := range []struct {
		name      string
		err       error
		subsetPid bool
		code      string
	}{
		{"absent under subset=pid", notExist, true, CodeProcSubset},
		{"absent without subset=pid", notExist, false, CodeReadFailed},
		{"permission denied under subset=pid", denied, true, CodeReadFailed},
		{"permission denied without it", denied, false, CodeReadFailed},
		{"no such source on this platform", errUnsupported, true, CodeUnsupported},
		{"wrapped absence under subset=pid", fmt.Errorf("reading: %w", notExist), true, CodeProcSubset},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := classify("/proc/stat", tc.err, tc.subsetPid)
			if r.Code != tc.code {
				t.Fatalf("code = %q, want %q (message %q)", r.Code, tc.code, r.Message)
			}
			switch tc.code {
			case CodeProcSubset:
				for _, must := range []string{"/proc/stat", "ProcSubset=pid", "ProcSubset=all"} {
					if !strings.Contains(r.Message, must) {
						t.Errorf("message %q does not name %q", r.Message, must)
					}
				}
			case CodeReadFailed:
				if !strings.Contains(r.Message, "/proc/stat") || !strings.Contains(r.Message, errors.Unwrap(tc.err).Error()) {
					t.Errorf("message %q does not name the path and the error", r.Message)
				}
			}
		})
	}
}
