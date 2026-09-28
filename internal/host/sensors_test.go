package host

import (
	"context"
	"io/fs"
	"reflect"
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

func temp(chip, kind, label string, c float64) inventory.HardwareTemperature {
	return inventory.HardwareTemperature{Chip: chip, Kind: kind, Label: label, Celsius: c}
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
		temp("cpu_thermal", "cpu", "temp1", 64.4),
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

	nvme := temp("nvme", "disk", "Composite", 38.8)
	nvme.HighC, nvme.CriticalC = celsius(82), celsius(85)
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
	want := []inventory.HardwareFan{{Chip: "nct6798", Label: "CPU fan", RPM: 900}}
	if !reflect.DeepEqual(s.Fans, want) {
		t.Errorf("fans = %+v, want %+v", s.Fans, want)
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
