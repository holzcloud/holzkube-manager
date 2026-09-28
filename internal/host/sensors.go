package host

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// Where the sensors are, as fs.FS names.
const (
	hwmonClass   = "sys/class/hwmon"
	thermalClass = "sys/class/thermal"
)

// The ceilings on the walk (T-11-17). A real machine has a handful of chips
// with a handful of inputs each; these are far above that and far below what a
// broken driver could make a walk read.
const (
	maxSensorChips  = 64
	maxChipInputs   = 32
	maxThermalZones = 64
	maxTripPoints   = 16
)

// Sensors is the host's temperatures and fans (HMON-03).
//
// The element types are the node page's own, so both pages carry exactly the
// same JSON for a sensor and render it with the same components (D-13). Both
// lists are never null: an empty list is a machine with no such sensor, which
// the page says in words.
type Sensors struct {
	Temperatures []inventory.HardwareTemperature `json:"temperatures"`
	Fans         []inventory.HardwareFan         `json:"fans"`
}

// sensorChip is one hwmon entry found in the class directory.
type sensorChip struct {
	index int
	dir   string
}

// readSensors walks /sys/class/hwmon, and /sys/class/thermal when no CPU chip
// reported a temperature, with the node path's rules (D-08): the chip is
// classified by its driver's name with talos.ClassifyChip, only temp*_input and
// fan*_input are read -- never a voltage -- and a thermal zone whose hwmon twin
// is listed is not listed a second time.
//
// Every temperature carries where it turns amber and red (D-06, D-07): the
// chip's own limits, else its thermal zone's trip points (see zoneTrips), else
// its kind's default -- through inventory.TemperatureLimits, the rule the node
// page uses.
//
// A missing class directory is a machine without that interface, which is an
// empty list. Any other failure to list it is an error: the page cannot tell
// "no sensors" from "could not look", and must not say the first when it is
// the second.
func readSensors(fsys fs.FS) (Sensors, error) {
	out := Sensors{
		Temperatures: make([]inventory.HardwareTemperature, 0),
		Fans:         make([]inventory.HardwareFan, 0),
	}

	chips, err := classEntries(fsys, hwmonClass, "hwmon", maxSensorChips)
	if err != nil {
		return Sensors{}, err
	}

	// The zones are read first, because an hwmon chip that sets no limits of
	// its own takes its twin zone's. Whether a listing error matters is known
	// only once the chips are read: see below.
	zoneList, zoneErr := classEntries(fsys, thermalClass, "thermal_zone", maxThermalZones)
	zones := readZones(fsys, zoneList)
	twins := make(map[string]tripLimits, len(zones))
	for _, z := range zones {
		name := talos.ThermalTwinName(z.typ)
		if _, seen := twins[name]; !seen {
			twins[name] = z.limits
		}
	}

	listed := map[string]bool{}
	cpuTemps := false
	for _, chip := range chips {
		temps, fans, name := readChip(fsys, chip, twins)
		if len(temps) == 0 && len(fans) == 0 {
			// A voltage monitor, a battery, a power sensor: nothing this card
			// shows. Left out rather than listed empty, as on the node page.
			continue
		}
		listed[name] = true
		if talos.ClassifyChip(name) == talos.SensorCPU && len(temps) > 0 {
			cpuTemps = true
		}
		out.Temperatures = append(out.Temperatures, temps...)
		out.Fans = append(out.Fans, fans...)
	}

	// When a CPU chip reported, the zones were read only for their limits,
	// and a zone directory that cannot be listed leaves every chip its own
	// limits or its kind's defaults: "no limit found" is not "the sensors
	// could not be read". When the zones are the CPU's only temperature, not
	// being able to list them is exactly that, and is said.
	if !cpuTemps {
		if zoneErr != nil {
			return Sensors{}, zoneErr
		}
		// The fallback for a machine whose CPU has no hwmon chip, with each
		// zone's own trip points as its limits.
		for _, z := range zones {
			celsius, ok := readMillidegrees(fsys, z.dir+"/temp")
			if !ok {
				continue
			}
			if listed[talos.ThermalTwinName(z.typ)] {
				continue
			}
			kind := string(talos.ClassifyChip(z.typ))
			warn, danger := inventory.TemperatureLimits(kind, z.limits.high, z.limits.crit)
			out.Temperatures = append(out.Temperatures, inventory.HardwareTemperature{
				Chip:      z.typ,
				Kind:      kind,
				Label:     path.Base(z.dir),
				Celsius:   celsius,
				HighC:     z.limits.high,
				CriticalC: z.limits.crit,
				WarnC:     warn,
				DangerC:   danger,
			})
		}
	}

	return out, nil
}

// thermalZone is one /sys/class/thermal zone with a type, and the limits its
// trip points set.
type thermalZone struct {
	dir    string
	typ    string
	limits tripLimits
}

// tripLimits are a zone's warning and critical lines, nil where it sets none.
type tripLimits struct{ high, crit *float64 }

// readZones reads the type and trip points of every listed zone. A zone
// without a type is left out: it cannot be matched to anything, and its
// temperature cannot be named.
func readZones(fsys fs.FS, list []sensorChip) []thermalZone {
	out := make([]thermalZone, 0, len(list))
	for _, z := range list {
		typ, ok := readTrimmed(fsys, z.dir+"/type")
		if !ok || typ == "" {
			continue
		}
		out = append(out, thermalZone{dir: z.dir, typ: typ, limits: zoneTrips(fsys, z.dir)})
	}
	return out
}

// zoneTrips is where a thermal zone's trip points put its lines (D-07).
//
// A passive or hot trip is where the kernel starts throttling the processor,
// which is the warning; a critical trip is where it shuts the machine down,
// which is danger. The lowest of each wins. An active trip is a fan stage and
// never a line -- on a Raspberry Pi 5 the first is at 50 °C, and a warning
// there would be on for good -- and a trip whose type cannot be read is not
// guessed at. At most maxTripPoints trips are read, each through readTrimmed,
// so a driver cannot make the walk read without end (T-12-05).
func zoneTrips(fsys fs.FS, dir string) tripLimits {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return tripLimits{}
	}
	files := make(map[string]bool, len(entries))
	for _, e := range entries {
		files[e.Name()] = true
	}

	var out tripLimits
	for n := range maxTripPoints {
		trip := fmt.Sprintf("trip_point_%d_", n)
		if !files[trip+"type"] || !files[trip+"temp"] {
			continue
		}
		prefix := dir + "/" + trip
		typ, ok := readTrimmed(fsys, prefix+"type")
		if !ok {
			continue
		}
		var into **float64
		switch typ {
		case "passive", "hot":
			into = &out.high
		case "critical":
			into = &out.crit
		default:
			continue
		}
		c, ok := readMillidegrees(fsys, prefix+"temp")
		// A trip at or below zero is a disabled one, as a limit at or below
		// zero is a chip that sets none.
		if !ok || c <= 0 {
			continue
		}
		if *into == nil || c < **into {
			*into = &c
		}
	}
	return out
}

// classEntries lists a /sys/class directory's <prefix>N entries in index
// order, at most limit of them.
//
// The entries are symlinks into /sys/devices, so they are never filtered on
// IsDir: a symlink's entry says it is a link, not a directory, and a filter on
// it would drop every chip on a real machine. Reading through the path lets
// the file system follow the link.
func classEntries(fsys fs.FS, class, prefix string, limit int) ([]sensorChip, error) {
	entries, err := fs.ReadDir(fsys, class)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []sensorChip
	for _, e := range entries {
		index, ok := talos.Numbered(e.Name(), prefix)
		if !ok {
			continue
		}
		out = append(out, sensorChip{index: index, dir: class + "/" + e.Name()})
	}
	// Numerically, so that hwmon10 follows hwmon9 rather than hwmon1.
	sort.Slice(out, func(i, j int) bool { return out[i].index < out[j].index })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// readChip reads one hwmon chip's temperatures and fans. A chip whose
// directory cannot be listed has nothing to show, and one input that does not
// parse is skipped without its neighbours. A limit the chip does not set is
// taken from its twin thermal zone, when it has one.
func readChip(
	fsys fs.FS, chip sensorChip, twins map[string]tripLimits,
) (temps []inventory.HardwareTemperature, fans []inventory.HardwareFan, name string) {
	name, _ = readTrimmed(fsys, chip.dir+"/name")
	if name == "" {
		name = fmt.Sprintf("hwmon%d", chip.index)
	}
	kind := string(talos.ClassifyChip(name))
	twin := twins[name]

	entries, err := fs.ReadDir(fsys, chip.dir)
	if err != nil {
		return nil, nil, name
	}
	files := make(map[string]bool, len(entries))
	for _, e := range entries {
		files[e.Name()] = true
	}

	for _, n := range capInputs(talos.InputsNamed(files, "temp")) {
		prefix := fmt.Sprintf("%s/temp%d_", chip.dir, n)
		celsius, ok := readMillidegrees(fsys, prefix+"input")
		if !ok {
			continue
		}
		label, _ := readTrimmed(fsys, prefix+"label")
		if label == "" {
			label = fmt.Sprintf("temp%d", n)
		}
		// The limits through the node path's own parser, over only the files
		// the chip has: a limit at or below zero is a chip that sets none.
		limits := make(map[string]string, 2)
		for _, suffix := range []string{"max", "crit"} {
			if !files[fmt.Sprintf("temp%d_%s", n, suffix)] {
				continue
			}
			if raw, ok := readTrimmed(fsys, prefix+suffix); ok {
				limits[suffix] = raw
			}
		}
		high := roundTenth(talos.Millidegrees(limits, "max"))
		if high == nil {
			high = twin.high
		}
		crit := roundTenth(talos.Millidegrees(limits, "crit"))
		if crit == nil {
			crit = twin.crit
		}
		warn, danger := inventory.TemperatureLimits(kind, high, crit)
		temps = append(temps, inventory.HardwareTemperature{
			Chip:      name,
			Kind:      kind,
			Label:     label,
			Celsius:   celsius,
			HighC:     high,
			CriticalC: crit,
			WarnC:     warn,
			DangerC:   danger,
		})
	}

	for _, n := range capInputs(talos.InputsNamed(files, "fan")) {
		raw, ok := readTrimmed(fsys, fmt.Sprintf("%s/fan%d_input", chip.dir, n))
		if !ok {
			continue
		}
		rpm, err := strconv.Atoi(raw)
		if err != nil || rpm < 0 {
			continue
		}
		label, _ := readTrimmed(fsys, fmt.Sprintf("%s/fan%d_label", chip.dir, n))
		if label == "" {
			label = fmt.Sprintf("fan%d", n)
		}
		fans = append(fans, inventory.HardwareFan{Chip: name, Label: label, RPM: rpm})
	}
	return temps, fans, name
}

// capInputs keeps the first maxChipInputs of a chip's inputs.
func capInputs(ns []int) []int {
	if len(ns) > maxChipInputs {
		return ns[:maxChipInputs]
	}
	return ns
}

// readTrimmed reads one small sysfs attribute, trimmed.
func readTrimmed(fsys fs.FS, name string) (string, bool) {
	raw, err := readBounded(fsys, name, maxSmallFile)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(raw)), true
}

// readMillidegrees reads a temperature input as degrees Celsius to a tenth, as
// the node path rounds it.
func readMillidegrees(fsys fs.FS, name string) (float64, bool) {
	raw, ok := readTrimmed(fsys, name)
	if !ok {
		return 0, false
	}
	milli, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return math.Round(float64(milli)/100) / 10, true
}

// roundTenth rounds a limit to a tenth, or keeps nil.
func roundTenth(c *float64) *float64 {
	if c == nil {
		return nil
	}
	r := math.Round(*c*10) / 10
	return &r
}
