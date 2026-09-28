package host

import (
	"context"
	"encoding/json"
	"io/fs"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
)

// The Sensors card's data (HMON-03): the host's hwmon chips and, where the CPU
// has none, its thermal zones -- under the node path's rules (D-08) and in the
// node page's element types (D-13).

func celsius(c float64) *float64 { return &c }

// kindLines are the per-kind defaults written out, so an expectation states
// the number it expects rather than asking the function under test.
var kindLines = map[string][2]float64{
	"cpu": {80, 95}, "disk": {60, 70}, "board": {70, 85}, "gpu": {80, 95}, "other": {75, 90},
}

// temp is a temperature whose chip sets no limit: its lines are its kind's.
func temp(chip, kind, label string, c float64) inventory.HardwareTemperature {
	lines := kindLines[kind]
	return inventory.HardwareTemperature{
		Chip: chip, Kind: kind, Label: label, Celsius: c, WarnC: lines[0], DangerC: lines[1],
	}
}

// limited gives a temperature limits and the lines they make.
func limited(t inventory.HardwareTemperature, high, crit *float64, warn, danger float64) inventory.HardwareTemperature {
	t.HighC, t.CriticalC, t.WarnC, t.DangerC = high, crit, warn, danger
	return t
}

func mustReadSensors(t *testing.T, fsys fs.FS) Sensors {
	t.Helper()
	s, err := readSensors(fsys)
	if err != nil {
		t.Fatalf("readSensors: %v", err)
	}
	if s.Temperatures == nil || s.Fans == nil {
		t.Fatalf("a list is nil and would reach the browser as null: %+v", s)
	}
	return s
}

func assertTemperatures(t *testing.T, got, want []inventory.HardwareTemperature) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("temperatures:\n got  %s\n want %s", describeTemps(got), describeTemps(want))
	}
}

func describeTemps(ts []inventory.HardwareTemperature) string {
	var parts []string
	for _, t := range ts {
		limits := ""
		if t.HighC != nil {
			limits += " high " + trimFloat(*t.HighC)
		}
		if t.CriticalC != nil {
			limits += " crit " + trimFloat(*t.CriticalC)
		}
		limits += " warn " + trimFloat(t.WarnC) + " danger " + trimFloat(t.DangerC)
		parts = append(parts, t.Chip+"/"+t.Label+" ("+t.Kind+") "+trimFloat(t.Celsius)+limits)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func trimFloat(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

// TestSensorsPi5 is the production machine's layout, symlinks and all: the CPU
// arrives as the thermal zone's hwmon twin, the RP1's ADC reports one
// temperature beside four voltages, and the firmware's voltage monitor reports
// no reading at all.
func TestSensorsPi5(t *testing.T) {
	t.Parallel()

	s := mustReadSensors(t, fixtureFS(t, "pi5"))

	// Exactly these two: no voltage input, no rpi_volt entry, and the thermal
	// zone not listed a second time beside cpu_thermal.
	assertTemperatures(t, s.Temperatures, []inventory.HardwareTemperature{
		// The CPU's chip sets no limit; its twin zone's critical trip is the
		// red line, and its four active trips (fan stages) are none (D-07).
		limited(temp("cpu_thermal", "cpu", "temp1", 64.4), nil, celsius(110), 80, 110),
		temp("rp1_adc", "other", "temp1", 55.4),
	})
	for _, tc := range s.Temperatures {
		if strings.HasPrefix(tc.Label, "in") {
			t.Errorf("a voltage input was listed as a temperature: %+v", tc)
		}
	}
	if len(s.Fans) != 0 {
		t.Errorf("fans = %+v, want none: the Pi 5 reports no fan speed input", s.Fans)
	}
}

// TestSensorsAMD64 is an ordinary board: a CPU chip with two inputs, an NVMe
// drive with its own limits, a Super I/O chip with temperatures, fans and a
// voltage, and ACPI's zone as its hwmon twin.
func TestSensorsAMD64(t *testing.T) {
	t.Parallel()

	s := mustReadSensors(t, fixtureFS(t, "amd64"))

	nvme := limited(temp("nvme", "disk", "Composite", 38.8), celsius(82), celsius(85), 82, 85)
	assertTemperatures(t, s.Temperatures, []inventory.HardwareTemperature{
		temp("k10temp", "cpu", "Tctl", 45.2),
		temp("k10temp", "cpu", "Tccd1", 47),
		nvme,
		temp("nct6798", "board", "SYSTIN", 36),
		temp("nct6798", "board", "CPUTIN", 41),
		temp("acpitz", "board", "temp1", 16.8),
	})
	want := []inventory.HardwareFan{
		{Chip: "nct6798", Label: "fan1", RPM: 1080},
		{Chip: "nct6798", Label: "fan2", RPM: 0},
	}
	if !reflect.DeepEqual(s.Fans, want) {
		t.Errorf("fans = %+v, want %+v", s.Fans, want)
	}
}

// TestThermalTwinRule: with no CPU chip in hwmon the zones are read, and a zone
// whose hwmon twin is already listed is skipped -- the other one is the CPU's
// only temperature and is kept.
func TestThermalTwinRule(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"sys/class/hwmon/hwmon0/name":                       {Data: []byte("acpitz\n")},
		"sys/class/hwmon/hwmon0/temp1_input":                {Data: []byte("16800\n")},
		"sys/class/thermal/thermal_zone0/type":              {Data: []byte("acpitz\n")},
		"sys/class/thermal/thermal_zone0/temp":              {Data: []byte("16800\n")},
		"sys/class/thermal/thermal_zone1/type":              {Data: []byte("x86_pkg_temp\n")},
		"sys/class/thermal/thermal_zone1/temp":              {Data: []byte("52000\n")},
		"sys/class/thermal/thermal_zone1/trip_point_0_temp": {Data: []byte("100000\n")},
		"sys/class/thermal/cooling_device0/type":            {Data: []byte("Processor\n")},
	}
	s := mustReadSensors(t, fsys)
	assertTemperatures(t, s.Temperatures, []inventory.HardwareTemperature{
		temp("acpitz", "board", "temp1", 16.8),
		temp("x86_pkg_temp", "cpu", "thermal_zone1", 52),
	})
}

// TestZonesOnlyWithoutCPUChip: an Intel machine's package zone has no hwmon
// twin (x86_pkg_temp registers without one), so only the "zones only when no
// CPU chip reported" rule keeps its temperature from being listed beside
// coretemp's own package sensor.
func TestZonesOnlyWithoutCPUChip(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"sys/class/hwmon/hwmon0/name":          {Data: []byte("coretemp\n")},
		"sys/class/hwmon/hwmon0/temp1_input":   {Data: []byte("52000\n")},
		"sys/class/hwmon/hwmon0/temp1_label":   {Data: []byte("Package id 0\n")},
		"sys/class/thermal/thermal_zone0/type": {Data: []byte("x86_pkg_temp\n")},
		"sys/class/thermal/thermal_zone0/temp": {Data: []byte("52000\n")},
	}
	s := mustReadSensors(t, fsys)
	assertTemperatures(t, s.Temperatures, []inventory.HardwareTemperature{
		temp("coretemp", "cpu", "Package id 0", 52),
	})
}

// TestSensorsNone: a machine without either class directory has no sensors,
// which is two empty lists -- on the wire too, never null.
func TestSensorsNone(t *testing.T) {
	t.Parallel()

	s := mustReadSensors(t, fstest.MapFS{})
	if len(s.Temperatures) != 0 || len(s.Fans) != 0 {
		t.Errorf("sensors = %+v, want none", s)
	}

	v := newTestCollector(fstest.MapFS{}, tracerSys()).Read(context.Background())
	sensors := liveSensors(t, v)
	if sensors["readable"] != true {
		t.Fatalf("live.sensors = %v, want readable", sensors)
	}
	value, _ := sensors["value"].(map[string]any)
	for _, key := range []string{"temperatures", "fans"} {
		if list, ok := value[key].([]any); !ok || len(list) != 0 {
			t.Errorf("live.sensors.value.%s = %v, want []", key, value[key])
		}
	}
}

// deniedDir is a file system whose listing of one directory is refused, as a
// unit's hardening or a restrictive mode would refuse it.
type deniedDir struct {
	fstest.MapFS
	dir string
}

func (d deniedDir) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == d.dir {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return d.MapFS.ReadDir(name)
}

// TestSensorsListFails: a hwmon directory that is there and cannot be listed
// is not "no sensors". The page is told it could not look.
func TestSensorsListFails(t *testing.T) {
	t.Parallel()

	fsys := deniedDir{MapFS: fstest.MapFS{
		"sys/class/hwmon/hwmon0/name":        {Data: []byte("cpu_thermal\n")},
		"sys/class/hwmon/hwmon0/temp1_input": {Data: []byte("64400\n")},
	}, dir: "sys/class/hwmon"}

	v := New(Config{FS: fsys, Sys: tracerSys(), Now: newLiveClock().now}).Read(context.Background())
	sensors := liveSensors(t, v)
	if sensors["readable"] != false {
		t.Fatalf("live.sensors = %v, want not readable", sensors)
	}
	if _, has := sensors["value"]; has {
		t.Errorf("live.sensors carries a value although it was not read: %v", sensors)
	}
	reason, _ := sensors["reason"].(map[string]any)
	if reason["code"] != CodeReadFailed {
		t.Errorf("reason code = %v, want %s", reason["code"], CodeReadFailed)
	}
	if msg, _ := reason["message"].(string); !strings.Contains(msg, "/sys/class/hwmon") {
		t.Errorf("reason message %q does not name /sys/class/hwmon", msg)
	}
}

// TestSensorsSkipsBadInput: one input that does not parse -- a sleeping
// device answering with nothing -- is skipped, and its chip's other inputs are
// still listed.
func TestSensorsSkipsBadInput(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"sys/class/hwmon/hwmon0/name":        {Data: []byte("nct6798\n")},
		"sys/class/hwmon/hwmon0/temp1_input": {Data: []byte("\n")},
		"sys/class/hwmon/hwmon0/temp2_input": {Data: []byte("41000\n")},
		"sys/class/hwmon/hwmon0/temp3_input": {Data: []byte("not a number\n")},
		"sys/class/hwmon/hwmon0/fan1_input":  {Data: []byte("-\n")},
		"sys/class/hwmon/hwmon0/fan2_input":  {Data: []byte("900\n")},
		"sys/class/hwmon/hwmon0/fan2_label":  {Data: []byte("CPU fan\n")},
	}
	s := mustReadSensors(t, fsys)
	assertTemperatures(t, s.Temperatures, []inventory.HardwareTemperature{
		temp("nct6798", "board", "temp2", 41),
	})
	// Skipped, not forgotten: the two that failed are named (D-12).
	if want := []string{"nct6798 temp1", "nct6798 temp3"}; !reflect.DeepEqual(s.Unread, want) {
		t.Errorf("unread = %q, want %q", s.Unread, want)
	}
	want := []inventory.HardwareFan{{Chip: "nct6798", Label: "CPU fan", RPM: 900}}
	if !reflect.DeepEqual(s.Fans, want) {
		t.Errorf("fans = %+v, want %+v", s.Fans, want)
	}
}

// TestSensorsThatFailAreNotNone (D-12): a temperature that is there and could
// not be read is named in Unread, never dropped -- so a host whose every input
// failed is not "a machine with no temperature sensors", judged healthy by its
// filesystems alone. A fan that fails is only skipped: nothing is rated on it.
func TestSensorsThatFailAreNotNone(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		fsys fs.FS
		want []string
	}{
		{
			// The review's probe: the Pi's CPU chip answering with nothing,
			// and its zone with garbage.
			name: "the CPU chip and its zone both fail",
			fsys: fstest.MapFS{
				"sys/class/hwmon/hwmon0/name":          {Data: []byte("cpu_thermal\n")},
				"sys/class/hwmon/hwmon0/temp1_input":   {Data: []byte("\n")},
				"sys/class/thermal/thermal_zone0/type": {Data: []byte("cpu-thermal\n")},
				"sys/class/thermal/thermal_zone0/temp": {Data: []byte("garbage\n")},
			},
			want: []string{"cpu_thermal temp1", "cpu-thermal"},
		},
		{
			name: "a labelled input that fails beside a fan that reads",
			fsys: fstest.MapFS{
				"sys/class/hwmon/hwmon0/name":        {Data: []byte("nct6798\n")},
				"sys/class/hwmon/hwmon0/temp1_input": {Data: []byte("not a number\n")},
				"sys/class/hwmon/hwmon0/temp1_label": {Data: []byte("SYSTIN\n")},
				"sys/class/hwmon/hwmon0/fan1_input":  {Data: []byte("900\n")},
			},
			want: []string{"nct6798 SYSTIN"},
		},
		{
			name: "a chip whose directory cannot be listed",
			fsys: deniedDir{MapFS: fstest.MapFS{
				"sys/class/hwmon/hwmon0/name":        {Data: []byte("k10temp\n")},
				"sys/class/hwmon/hwmon0/temp1_input": {Data: []byte("48000\n")},
			}, dir: "sys/class/hwmon/hwmon0"},
			want: []string{"k10temp"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := mustReadSensors(t, tc.fsys)
			if !reflect.DeepEqual(s.Unread, tc.want) {
				t.Errorf("unread = %q, want %q", s.Unread, tc.want)
			}
			h := Assess(hostLive(Read(s), fine))
			if h.State != HealthUnknown {
				t.Errorf("health = %+v, want unknown", h)
			}
			for _, name := range tc.want {
				sentence := "Temperature " + name + " could not be read."
				if !slices.Contains(h.Unreadable, sentence) {
					t.Errorf("unreadable = %q, want %q among them", h.Unreadable, sentence)
				}
			}
		})
	}

	// What is there and read leaves Unread empty, and stays off the wire.
	s := mustReadSensors(t, fstest.MapFS{
		"sys/class/hwmon/hwmon0/name":        {Data: []byte("cpu_thermal\n")},
		"sys/class/hwmon/hwmon0/temp1_input": {Data: []byte("49600\n")},
	})
	if len(s.Unread) != 0 {
		t.Errorf("unread = %q, want none", s.Unread)
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(raw)), "unread") {
		t.Errorf("sensors on the wire carry Unread: %s", raw)
	}
}

// liveSensors is live.sensors as the browser receives it.
func liveSensors(t *testing.T, v View) map[string]any {
	t.Helper()
	live, _ := marshalView(t, v)["live"].(map[string]any)
	sensors, ok := live["sensors"].(map[string]any)
	if !ok {
		t.Fatalf("the answer has no live.sensors: %v", live)
	}
	return sensors
}

// zoneFS is a machine with one CPU hwmon chip and the chip's twin thermal zone
// carrying the given trip points (type, millidegrees; an empty type is a trip
// whose type file is missing).
func zoneFS(chipFiles map[string]string, trips [][2]string) fstest.MapFS {
	fsys := fstest.MapFS{
		"sys/class/hwmon/hwmon0/name":          {Data: []byte("cpu_thermal\n")},
		"sys/class/hwmon/hwmon0/temp1_input":   {Data: []byte("64400\n")},
		"sys/class/thermal/thermal_zone0/type": {Data: []byte("cpu-thermal\n")},
		"sys/class/thermal/thermal_zone0/temp": {Data: []byte("64400\n")},
	}
	for name, v := range chipFiles {
		fsys["sys/class/hwmon/hwmon0/"+name] = &fstest.MapFile{Data: []byte(v + "\n")}
	}
	for n, trip := range trips {
		prefix := "sys/class/thermal/thermal_zone0/trip_point_" + strconv.Itoa(n) + "_"
		if trip[0] != "" {
			fsys[prefix+"type"] = &fstest.MapFile{Data: []byte(trip[0] + "\n")}
		}
		fsys[prefix+"temp"] = &fstest.MapFile{Data: []byte(trip[1] + "\n")}
	}
	return fsys
}

// TestTripPoints is where the host's temperatures get their lines when the
// chip sets none (D-07): from the twin thermal zone's passive or hot trip
// (the kernel throttles there) and its critical trip (it shuts down there),
// never from an active trip, which is a fan stage.
func TestTripPoints(t *testing.T) {
	t.Parallel()

	cpu := func(high, crit *float64, warn, danger float64) []inventory.HardwareTemperature {
		return []inventory.HardwareTemperature{
			limited(temp("cpu_thermal", "cpu", "temp1", 64.4), high, crit, warn, danger),
		}
	}
	// Sixteen fan stages, then a passive trip the cap must not reach.
	seventeen := make([][2]string, 0, 17)
	for n := range 16 {
		seventeen = append(seventeen, [2]string{"active", strconv.Itoa(40000 + n*1000)})
	}
	seventeen = append(seventeen, [2]string{"passive", "60000"})

	for _, tc := range []struct {
		name string
		fsys fs.FS
		want []inventory.HardwareTemperature
	}{
		{
			// The measured Pi 5: critical 110, four active trips at 50, 60,
			// 67.5 and 75 °C. The CPU warns at its kind's 80, not at 50.
			name: "the Pi 5",
			fsys: fixtureFS(t, "pi5"),
			want: []inventory.HardwareTemperature{
				limited(temp("cpu_thermal", "cpu", "temp1", 64.4), nil, celsius(110), 80, 110),
				temp("rp1_adc", "other", "temp1", 55.4),
			},
		},
		{
			name: "a chip without limits takes its twin zone's passive and critical trips",
			fsys: zoneFS(nil, [][2]string{{"passive", "85000"}, {"critical", "105000"}}),
			want: cpu(celsius(85), celsius(105), 85, 105),
		},
		{
			name: "the chip's own high wins over the zone's passive trip",
			fsys: zoneFS(map[string]string{"temp1_max": "70000"}, [][2]string{{"passive", "85000"}, {"critical", "105000"}}),
			want: cpu(celsius(70), celsius(105), 70, 105),
		},
		{
			name: "a hot trip counts like passive, and the lowest of each wins",
			fsys: zoneFS(nil, [][2]string{
				{"critical", "110000"}, {"passive", "90000"}, {"hot", "87500"}, {"critical", "105000"},
			}),
			want: cpu(celsius(87.5), celsius(105), 87.5, 105),
		},
		{
			name: "active trips and trips without a type set no line",
			fsys: zoneFS(nil, [][2]string{{"active", "50000"}, {"", "45000"}, {"active", "60000"}}),
			want: cpu(nil, nil, 80, 95),
		},
		{
			name: "a disabled trip at zero sets no line",
			fsys: zoneFS(nil, [][2]string{{"critical", "0"}}),
			want: cpu(nil, nil, 80, 95),
		},
		{
			name: "at most sixteen trip points are read",
			fsys: zoneFS(nil, seventeen),
			want: cpu(nil, nil, 80, 95),
		},
		{
			// No CPU chip in hwmon: the zone is the CPU's temperature, and its
			// own trips are its lines.
			name: "the zone fallback takes its own zone's trips",
			fsys: fstest.MapFS{
				"sys/class/thermal/thermal_zone0/type":              {Data: []byte("x86_pkg_temp\n")},
				"sys/class/thermal/thermal_zone0/temp":              {Data: []byte("52000\n")},
				"sys/class/thermal/thermal_zone0/trip_point_0_type": {Data: []byte("passive\n")},
				"sys/class/thermal/thermal_zone0/trip_point_0_temp": {Data: []byte("85000\n")},
				"sys/class/thermal/thermal_zone0/trip_point_1_type": {Data: []byte("critical\n")},
				"sys/class/thermal/thermal_zone0/trip_point_1_temp": {Data: []byte("100000\n")},
				"sys/class/thermal/thermal_zone0/trip_point_2_type": {Data: []byte("active\n")},
				"sys/class/thermal/thermal_zone0/trip_point_2_temp": {Data: []byte("50000\n")},
			},
			want: []inventory.HardwareTemperature{
				limited(temp("x86_pkg_temp", "cpu", "thermal_zone0", 52), celsius(85), celsius(100), 85, 100),
			},
		},
		{
			// With a CPU chip the zones are read only for their lines; not
			// being able to list them leaves the chip's own and the kind's.
			name: "an unlistable thermal class with a CPU chip keeps the chip's limits",
			fsys: deniedDir{MapFS: zoneFS(
				map[string]string{"temp1_max": "70000", "temp1_crit": "100000"},
				[][2]string{{"passive", "60000"}, {"critical", "90000"}},
			), dir: "sys/class/thermal"},
			want: cpu(celsius(70), celsius(100), 70, 100),
		},
		{
			name: "an unlistable thermal class with a CPU chip without limits gives the kind's",
			fsys: deniedDir{MapFS: zoneFS(nil, [][2]string{{"critical", "90000"}}), dir: "sys/class/thermal"},
			want: cpu(nil, nil, 80, 95),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertTemperatures(t, mustReadSensors(t, tc.fsys).Temperatures, tc.want)
		})
	}

	// Without a CPU chip the zones are the temperature, and a class that
	// cannot be listed is "could not look", as before.
	denied := deniedDir{MapFS: fstest.MapFS{
		"sys/class/thermal/thermal_zone0/type": {Data: []byte("cpu-thermal\n")},
		"sys/class/thermal/thermal_zone0/temp": {Data: []byte("52000\n")},
	}, dir: "sys/class/thermal"}
	if _, err := readSensors(denied); err == nil {
		t.Error("readSensors with no CPU chip and an unlistable thermal class returned no error")
	}
}
