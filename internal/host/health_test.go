package host

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
)

// The host's state (D-06, D-08..D-12): one pure function, built here from Live
// values directly -- no fixture tree is needed to say what a reading means.

var (
	hardened = Reason{Code: CodeProcSubset, Message: "Hidden by the unit's ProcSubset=pid."}
	failed   = Reason{Code: CodeReadFailed, Message: "open /sys/class/hwmon: permission denied"}
	noStatfs = Reason{Code: CodeReadFailed, Message: "statfs /srv: permission denied"}
)

// sensorAt is a temperature with its lines, as readSensors hands it over.
func sensorAt(chip, label string, c, warn, danger float64) inventory.HardwareTemperature {
	return inventory.HardwareTemperature{
		Chip: chip, Kind: "cpu", Label: label, Celsius: c, WarnC: warn, DangerC: danger,
	}
}

func sensorsRead(temps ...inventory.HardwareTemperature) Reading[Sensors] {
	if temps == nil {
		temps = []inventory.HardwareTemperature{}
	}
	return Read(Sensors{Temperatures: temps, Fans: []inventory.HardwareFan{}})
}

// fsRow is a filesystem with used and available bytes; size is their sum
// unless a test says otherwise.
func fsRow(mount string, used, available uint64) Filesystem {
	return Filesystem{
		Mount: mount, Roles: []string{"root"},
		Usage: Read(FSUsage{SizeBytes: used + available, UsedBytes: used, AvailableBytes: available}),
	}
}

// fsData is a filesystem that is the data directory's own, not /.
func fsData(mount string, used, available uint64) Filesystem {
	row := fsRow(mount, used, available)
	row.Roles = []string{"data directory"}
	return row
}

func fsHidden(mount string, r Reason) Filesystem {
	return Filesystem{Mount: mount, Roles: []string{"data directory"}, Usage: Hidden[FSUsage](r)}
}

// hostLive is a Live as the shipped unit reads it: CPU and memory hidden by
// ProcSubset=pid, which must never decide the state.
func hostLive(sensors Reading[Sensors], rows ...Filesystem) Live {
	if rows == nil {
		rows = []Filesystem{}
	}
	return Live{
		CPU: CPU{
			Usage:   Hidden[float64](hardened),
			PerCore: Hidden[[]float64](hardened),
			Load:    Read(Load{}),
		},
		Memory:      Hidden[inventory.HardwareMemory](hardened),
		Filesystems: rows,
		Sensors:     sensors,
		Network:     Read(Network{}),
	}
}

var fine = fsRow("/", 20, 80)

// TestAssessWarns: every crossed line is named with its value and its line,
// worst first, at exactly the boundaries the page's marks use.
func TestAssessWarns(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		live     Live
		state    HealthState
		warnings []string
		// public is the wall's form of warnings: the figure first, so a
		// tile that cuts the sentence keeps the reading (13-UI-SPEC checker
		// resolution 4), and a filesystem by its role, never its path.
		public  []string
		summary string
	}{
		{
			name:     "a CPU past its warning line",
			live:     hostLive(sensorsRead(sensorAt("cpu_thermal", "temp1", 82.1, 80, 110)), fine),
			state:    HealthWarn,
			warnings: []string{"cpu_thermal 82.1 °C ≥ 80 °C"},
			public:   []string{"82.1 °C ≥ 80 °C · cpu_thermal"},
			summary:  "1 threshold crossed.",
		},
		{
			name:     "exactly at the warning line warns",
			live:     hostLive(sensorsRead(sensorAt("cpu_thermal", "temp1", 80, 80, 110)), fine),
			state:    HealthWarn,
			warnings: []string{"cpu_thermal 80.0 °C ≥ 80 °C"},
			public:   []string{"80.0 °C ≥ 80 °C · cpu_thermal"},
			summary:  "1 threshold crossed.",
		},
		{
			name:     "a tenth below the line is fine",
			live:     hostLive(sensorsRead(sensorAt("cpu_thermal", "temp1", 79.9, 80, 110)), fine),
			state:    HealthOK,
			warnings: []string{},
			public:   []string{},
			summary:  "Temperatures and filesystems are below their thresholds.",
		},
		{
			name: "past danger is critical, named against the danger line, and listed first",
			live: hostLive(sensorsRead(
				sensorAt("nct6798", "SYSTIN", 99, 70, 85.5),
				sensorAt("cpu_thermal", "temp1", 112, 80, 110),
				sensorAt("k10temp", "Tctl", 90, 80, 95),
			), fine),
			state: HealthWarn,
			warnings: []string{
				// Both critical: SYSTIN is 15.8 % past 85.5, the CPU 1.8 % past 110.
				"nct6798 SYSTIN 99.0 °C ≥ 85.5 °C, critical",
				"cpu_thermal 112.0 °C ≥ 110 °C, critical",
				"k10temp Tctl 90.0 °C ≥ 80 °C",
			},
			public: []string{
				"99.0 °C ≥ 85.5 °C, critical · nct6798 SYSTIN",
				"112.0 °C ≥ 110 °C, critical · cpu_thermal",
				"90.0 °C ≥ 80 °C · k10temp Tctl",
			},
			summary: "3 thresholds crossed.",
		},
		{
			name:     "a filesystem at exactly 80 % of used + available warns",
			live:     hostLive(sensorsRead(), fsRow("/", 80, 20)),
			state:    HealthWarn,
			warnings: []string{"/ 80% used ≥ 80%"},
			public:   []string{"80% used ≥ 80% · /"},
			summary:  "1 threshold crossed.",
		},
		{
			// df prints 79.99 % as 80%; the line is not the rounded figure.
			name:     "79.99 % is below the line although df prints 80%",
			live:     hostLive(sensorsRead(), fsRow("/", 7999, 2001)),
			state:    HealthOK,
			warnings: []string{},
			public:   []string{},
			summary: "Filesystems are below 80%. This machine reports no temperature sensors, " +
				"so it is judged by its filesystems alone.",
		},
		{
			// The blocks ext4 reserves for root are in neither used nor
			// available: 76 of 95 is 80 %, where used/size would say 76 %.
			name: "the denominator is used + available, not size",
			live: hostLive(sensorsRead(), Filesystem{
				Mount: "/", Roles: []string{"root"},
				Usage: Read(FSUsage{SizeBytes: 100, UsedBytes: 76, AvailableBytes: 19}),
			}),
			state:    HealthWarn,
			warnings: []string{"/ 80% used ≥ 80%"},
			public:   []string{"80% used ≥ 80% · /"},
			summary:  "1 threshold crossed.",
		},
		{
			name:     "df's percent is rounded up",
			live:     hostLive(sensorsRead(), fsRow("/", 9001, 999)),
			state:    HealthWarn,
			warnings: []string{"/ 91% used ≥ 80%"},
			public:   []string{"91% used ≥ 80% · /"},
			summary:  "1 threshold crossed.",
		},
		{
			name: "temperatures and filesystems order by relative excess",
			live: hostLive(sensorsRead(
				sensorAt("cpu_thermal", "temp1", 82.1, 80, 110), // 2.6 % past
			), fsRow("/", 95, 5), fsData("/var/lib/holzkube-manager", 85, 15)), // 18.75 % and 6.25 % past
			state: HealthWarn,
			warnings: []string{
				"/ 95% used ≥ 80%",
				"/var/lib/holzkube-manager 85% used ≥ 80%",
				"cpu_thermal 82.1 °C ≥ 80 °C",
			},
			// The data directory's warning names its path on the page and
			// its role on the wall (T-12-08).
			public: []string{
				"95% used ≥ 80% · /",
				"85% used ≥ 80% · data directory",
				"82.1 °C ≥ 80 °C · cpu_thermal",
			},
			summary: "3 thresholds crossed.",
		},
		{
			name: "equal excess keeps reading order",
			live: hostLive(sensorsRead(
				sensorAt("k10temp", "Tccd1", 88, 80, 95),
				sensorAt("k10temp", "Tctl", 88, 80, 95),
			), fine),
			state:    HealthWarn,
			warnings: []string{"k10temp Tccd1 88.0 °C ≥ 80 °C", "k10temp Tctl 88.0 °C ≥ 80 °C"},
			public:   []string{"88.0 °C ≥ 80 °C · k10temp Tccd1", "88.0 °C ≥ 80 °C · k10temp Tctl"},
			summary:  "2 thresholds crossed.",
		},
		{
			// A bare temp<N> is named by its chip alone only when it is the
			// chip's one temperature.
			name: "a bare temp label stays when the chip has several",
			live: hostLive(sensorsRead(
				sensorAt("nct6798", "temp2", 72, 70, 85),
				sensorAt("nct6798", "SYSTIN", 30, 70, 85),
			), fine),
			state:    HealthWarn,
			warnings: []string{"nct6798 temp2 72.0 °C ≥ 70 °C"},
			public:   []string{"72.0 °C ≥ 70 °C · nct6798 temp2"},
			summary:  "1 threshold crossed.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := Assess(tc.live)
			if h.State != tc.state {
				t.Errorf("state = %q, want %q (%+v)", h.State, tc.state, h)
			}
			if !reflect.DeepEqual(h.Warnings, tc.warnings) {
				t.Errorf("warnings:\n got  %q\n want %q", h.Warnings, tc.warnings)
			}
			if !reflect.DeepEqual(h.Public, tc.public) {
				t.Errorf("public:\n got  %q\n want %q", h.Public, tc.public)
			}
			if h.Summary != tc.summary {
				t.Errorf("summary = %q, want %q", h.Summary, tc.summary)
			}
			if len(h.Unreadable) != 0 {
				t.Errorf("unreadable = %q, want none", h.Unreadable)
			}
		})
	}
}

// TestUnreadableIsNeverHealthy (D-12): a host whose temperatures or either
// filesystem's usage could not be read is never ok -- unknown, or warn when a
// line is crossed, with what could not be read still listed.
func TestUnreadableIsNeverHealthy(t *testing.T) {
	t.Parallel()

	sensorStates := map[string]Reading[Sensors]{
		"sensors readable":     sensorsRead(sensorAt("cpu_thermal", "temp1", 50, 80, 110)),
		"sensors not readable": Hidden[Sensors](failed),
		// Read, and every input that is there failed: not a machine without
		// sensors (CR-01).
		"sensors readable, every input failed": Read(Sensors{
			Temperatures: []inventory.HardwareTemperature{},
			Fans:         []inventory.HardwareFan{},
			Unread:       []string{"cpu_thermal temp1"},
		}),
	}
	rootStates := map[string]Filesystem{
		"root readable":     fine,
		"root not readable": fsHidden("/", noStatfs),
	}
	dataStates := map[string]*Filesystem{
		"no data row":       nil,
		"data readable":     {Mount: "/srv", Usage: Read(FSUsage{SizeBytes: 100, UsedBytes: 10, AvailableBytes: 90})},
		"data not readable": {Mount: "/srv", Usage: Hidden[FSUsage](noStatfs)},
	}

	for sn, sensors := range sensorStates {
		for rn, root := range rootStates {
			for dn, data := range dataStates {
				rows := []Filesystem{root}
				if data != nil {
					rows = append(rows, *data)
				}
				unread := !sensors.Readable || (sensors.Value != nil && len(sensors.Value.Unread) > 0) ||
					!root.Usage.Readable || (data != nil && !data.Usage.Readable)

				name := sn + ", " + rn + ", " + dn
				h := Assess(hostLive(sensors, rows...))
				switch {
				case unread && (h.State != HealthUnknown || len(h.Unreadable) == 0):
					t.Errorf("%s: %+v, want unknown with its reasons", name, h)
				case !unread && h.State != HealthOK:
					t.Errorf("%s: %+v, want ok", name, h)
				}
				if unread && h.Summary != strings.Join(h.Unreadable, " ") {
					t.Errorf("%s: summary %q, want the reasons joined", name, h.Summary)
				}

				// The same with a crossed line: warn, and still listing
				// what it could not read.
				hot := append(append([]Filesystem{}, rows...), fsRow("/full", 99, 1))
				h = Assess(hostLive(sensors, hot...))
				if h.State != HealthWarn || len(h.Warnings) != 1 {
					t.Errorf("%s with a full disk: %+v, want warn with one warning", name, h)
				}
				if unread && len(h.Unreadable) == 0 {
					t.Errorf("%s with a full disk: warn lists nothing unreadable: %+v", name, h)
				}
			}
		}
	}

	// 12-SECURITY residual 1, through readSensors as the collector takes it:
	// a machine whose only temperature is a thermal zone that cannot be named
	// is not a machine without sensors.
	t.Run("a fallback zone without a type", func(t *testing.T) {
		t.Parallel()
		s, err := readSensors(fstest.MapFS{
			"sys/class/thermal/thermal_zone0/temp": {Data: []byte("52000\n")},
		})
		if err != nil {
			t.Fatal(err)
		}
		h := Assess(hostLive(Read(s), fine))
		want := []string{"Temperature thermal_zone0 could not be read."}
		if h.State != HealthUnknown || !reflect.DeepEqual(h.Unreadable, want) {
			t.Errorf("= %+v, want unknown with %q", h, want)
		}
	})

	t.Run("the sentences", func(t *testing.T) {
		t.Parallel()
		h := Assess(hostLive(Hidden[Sensors](failed), fine, fsHidden("/srv", Reason{
			Code: CodeReadFailed, Message: "statfs /srv: input/output error.",
		})))
		want := []string{
			"Temperatures could not be read: open /sys/class/hwmon: permission denied.",
			"Usage of /srv could not be read: statfs /srv: input/output error.",
		}
		if h.State != HealthUnknown || !reflect.DeepEqual(h.Unreadable, want) {
			t.Errorf("= %+v, want unknown with %q", h, want)
		}
		if h.Summary != strings.Join(want, " ") {
			t.Errorf("summary = %q", h.Summary)
		}
	})

	// WR-01: a statfs with no blocks, or a sum that wrapped, is no usage
	// figure -- not "below 80%".
	t.Run("a filesystem with no capacity", func(t *testing.T) {
		t.Parallel()
		for _, u := range []FSUsage{{}, {UsedBytes: math.MaxUint64, AvailableBytes: 2}} {
			row := Filesystem{Mount: "/srv", Usage: Read(u)}
			h := Assess(hostLive(sensorsRead(sensorAt("cpu_thermal", "temp1", 50, 80, 110)), fine, row))
			want := []string{"Usage of /srv reports no capacity."}
			if h.State != HealthUnknown || !reflect.DeepEqual(h.Unreadable, want) {
				t.Errorf("%+v: = %+v, want unknown with %q", u, h, want)
			}
		}
	})

	t.Run("no filesystem row at all", func(t *testing.T) {
		t.Parallel()
		h := Assess(hostLive(sensorsRead(sensorAt("cpu_thermal", "temp1", 50, 80, 110))))
		if h.State != HealthUnknown || !reflect.DeepEqual(h.Unreadable, []string{"No filesystem usage could be read."}) {
			t.Errorf("= %+v, want unknown: no filesystem usage", h)
		}
	})

	t.Run("warn with an unreadable value lists it", func(t *testing.T) {
		t.Parallel()
		h := Assess(hostLive(Hidden[Sensors](failed), fsRow("/", 90, 10)))
		if h.State != HealthWarn || !reflect.DeepEqual(h.Warnings, []string{"/ 90% used ≥ 80%"}) ||
			!reflect.DeepEqual(h.Unreadable, []string{"Temperatures could not be read: open /sys/class/hwmon: permission denied."}) {
			t.Errorf("= %+v", h)
		}
	})

	// D-11: CPU and memory hidden by the shipped hardening rate nothing.
	t.Run("hardened but fine is ok", func(t *testing.T) {
		t.Parallel()
		h := Assess(hostLive(sensorsRead(sensorAt("cpu_thermal", "temp1", 64.4, 80, 110)), fine))
		if h.State != HealthOK || h.Summary != "Temperatures and filesystems are below their thresholds." {
			t.Errorf("= %+v, want ok", h)
		}
	})

	t.Run("no temperature sensor is judged by its filesystems and says so", func(t *testing.T) {
		t.Parallel()
		h := Assess(hostLive(sensorsRead(), fine))
		want := "Filesystems are below 80%. This machine reports no temperature sensors, " +
			"so it is judged by its filesystems alone."
		if h.State != HealthOK || h.Summary != want {
			t.Errorf("= %+v, want ok: %q", h, want)
		}
	})

	// Ledger 162: the lists reach the browser as [], never null.
	t.Run("the lists are never null", func(t *testing.T) {
		t.Parallel()
		raw, err := json.Marshal(Assess(hostLive(sensorsRead(), fine)))
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{`"warnings":[]`, `"unreadable":[]`} {
			if !strings.Contains(string(raw), field) {
				t.Errorf("%s has no %s", raw, field)
			}
		}
	})
}
