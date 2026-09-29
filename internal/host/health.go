package host

import (
	"fmt"
	"math"
	"math/bits"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
)

// HealthState is the host's state as the page, the navigation and the wall
// show it. The strings are kube.StateOK, kube.StateWarn and kube.StateUnknown
// on purpose, so the wall paints the host's tile with the colours it already
// has; the package is not imported for three strings.
type HealthState string

const (
	// HealthOK is a host whose every rated value was read, none past its
	// line.
	HealthOK HealthState = "ok"

	// HealthWarn is a host with at least one rated value past its line.
	HealthWarn HealthState = "warn"

	// HealthUnknown is a host with no line crossed and some rated value that
	// could not be read -- so nobody can say the host is fine.
	HealthUnknown HealthState = "unknown"
)

// Health is the host's state and the sentences that explain it.
//
// Warnings are worst first and Unreadable in reading order; both are never
// null. The sentences are the server's (D-06): the browser prints them as they
// are.
//
// Public is Warnings again, in the same order, fit for a wall link: a
// filesystem is named by its role ("/", "data directory"), never by its path,
// which can be a mount point or, without a mount table, the data directory
// itself (T-12-08). The wall shows these and nothing else of the list; it is
// not sent with the page's answer.
//
// Its sentences put the figure first and the name after it --
// "82.1 °C ≥ 80 °C · cpu_thermal", "91% used ≥ 80% · data directory" --
// where Warnings put the name first. The wall's tile is one line that
// truncates: at 1600 px it cut "manager · cpu_thermal 8…" and lost the only
// fact it had (12-UI-REVIEW). The page shows the whole list and keeps the
// name first.
type Health struct {
	State      HealthState `json:"state"`
	Summary    string      `json:"summary"`
	Warnings   []string    `json:"warnings"`
	Unreadable []string    `json:"unreadable"`
	Public     []string    `json:"-"`
}

// filesystemWarnPercent is where a filesystem warns: 80 % of what df counts,
// used + available. The page's meter turns amber at the same line.
const filesystemWarnPercent = 80

// Assess is the one decision the page, the navigation and the wall show
// (D-06): is this host fine, warning, or not readable.
//
// It rates two things, the temperatures and the usage of every filesystem row
// (/ and, when it is a filesystem of its own, the data directory's). A
// temperature warns at its WarnC and is critical at its DangerC -- the lines
// inventory.TemperatureLimits drew for it. A filesystem warns at 80 % of
// used + available, df's denominator, with >=, exactly where the page's meter
// turns amber (D-08).
//
// CPU usage, per-core usage and memory are not rated. No line hangs on them,
// and the shipped unit's ProcSubset=pid hides them: a host rated on them
// would stay grey for good, and a grey that never goes away teaches the
// operator to overlook grey (D-11).
//
// What it could not read decides the state as much as what it read (D-10,
// D-12): a warning makes it warn, and the unreadable values are still listed;
// without a warning, any rated value that could not be read makes it unknown.
// Only a host whose every rated value was read, and none crossed its line, is
// ok. A machine that reports no temperature sensor at all -- a VM -- rates
// nothing there, is judged by its filesystems and says so.
//
// Assess is pure: no file, no clock. The same Live gives the same Health.
func Assess(l Live) Health {
	var (
		found      []finding
		unreadable = make([]string, 0)
		noSensors  bool
	)

	// Temperatures.
	switch {
	case !l.Sensors.Readable:
		unreadable = append(unreadable, "Temperatures could not be read"+because(l.Sensors.Reason))
	case l.Sensors.Value == nil:
		noSensors = true
	default:
		// A sensor that is there and could not be read is not a machine
		// without one (D-12): only no temperature and nothing unread is.
		for _, name := range l.Sensors.Value.Unread {
			unreadable = append(unreadable, "Temperature "+name+" could not be read.")
		}
		if len(l.Sensors.Value.Temperatures) == 0 && len(l.Sensors.Value.Unread) == 0 {
			noSensors = true
		}
		found = append(found, temperatureFindings(l.Sensors.Value.Temperatures)...)
	}

	// Filesystems.
	if len(l.Filesystems) == 0 {
		unreadable = append(unreadable, "No filesystem usage could be read.")
	}
	for _, row := range l.Filesystems {
		if !row.Usage.Readable || row.Usage.Value == nil {
			unreadable = append(unreadable, "Usage of "+row.Mount+" could not be read"+because(row.Usage.Reason))
			continue
		}
		// A statfs with no blocks -- some FUSE filesystems, a pseudo-fs bound
		// over the data directory -- gives no usage figure at all: "below 80%"
		// would be a green nothing measured (D-12). A sum that wrapped is no
		// figure either.
		if u := *row.Usage.Value; u.UsedBytes+u.AvailableBytes == 0 || u.UsedBytes+u.AvailableBytes < u.UsedBytes {
			unreadable = append(unreadable, "Usage of "+row.Mount+" reports no capacity.")
			continue
		}
		if f, ok := filesystemFinding(row, *row.Usage.Value); ok {
			found = append(found, f)
		}
	}

	// Worst first: critical before warning, then how far past its line,
	// relative to the line, largest first. Ties keep reading order.
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].critical != found[j].critical {
			return found[i].critical
		}
		return found[i].excess > found[j].excess
	})
	warnings := make([]string, 0, len(found))
	public := make([]string, 0, len(found))
	for _, f := range found {
		warnings = append(warnings, f.sentence)
		public = append(public, f.public)
	}

	h := Health{Warnings: warnings, Unreadable: unreadable, Public: public}
	switch {
	case len(warnings) > 0:
		h.State = HealthWarn
		if len(warnings) == 1 {
			h.Summary = "1 threshold crossed."
		} else {
			h.Summary = strconv.Itoa(len(warnings)) + " thresholds crossed."
		}
	case len(unreadable) > 0:
		h.State = HealthUnknown
		h.Summary = strings.Join(unreadable, " ")
	case noSensors:
		h.State = HealthOK
		h.Summary = "Filesystems are below 80%. This machine reports no temperature sensors, " +
			"so it is judged by its filesystems alone."
	default:
		h.State = HealthOK
		h.Summary = "Temperatures and filesystems are below their thresholds."
	}
	return h
}

// finding is one crossed line.
type finding struct {
	sentence string
	// public is sentence for the wall: without a path in it, and with the
	// figure before the name.
	public   string
	critical bool
	// excess is how far past its line the value is, relative to the line:
	// (value - line) / line.
	excess float64
}

// temperatureFindings is every temperature at or past its warning line.
func temperatureFindings(temps []inventory.HardwareTemperature) []finding {
	perChip := make(map[string]int, len(temps))
	for _, t := range temps {
		perChip[t.Chip]++
	}

	var out []finding
	for _, t := range temps {
		if t.Celsius < t.WarnC {
			continue
		}
		f := finding{critical: t.Celsius >= t.DangerC}
		line := t.WarnC
		if f.critical {
			line = t.DangerC
		}
		name := sensorName(t, perChip[t.Chip])
		figure := fmt.Sprintf("%s °C ≥ %s °C", strconv.FormatFloat(t.Celsius, 'f', 1, 64), formatLine(line))
		if f.critical {
			figure += ", critical"
		}
		f.sentence = name + " " + figure
		// Figure first for the wall. A chip's name and label are the
		// driver's, never a path.
		f.public = figure + " · " + name
		f.excess = relative(t.Celsius, line)
		out = append(out, f)
	}
	return out
}

// filesystemFinding is a filesystem at or past 80 % of used + available.
//
// Compared in integers, never on the rounded percent: df prints 79.01 % as
// "80%", and a warning that disagreed with the meter at the line would make
// one of them wrong.
func filesystemFinding(row Filesystem, u FSUsage) (finding, bool) {
	total := u.UsedBytes + u.AvailableBytes
	// Assess has listed a zero or wrapped sum as unreadable already; the
	// guard stays so that bits.Div64 can never panic here.
	if total == 0 || total < u.UsedBytes {
		return finding{}, false
	}
	// used*100 >= total*80, in 128 bits: no filesystem is big enough to
	// overflow 64, and none has to be for this to stay right.
	uh, ul := bits.Mul64(u.UsedBytes, 100)
	th, tl := bits.Mul64(total, filesystemWarnPercent)
	if uh < th || (uh == th && ul < tl) {
		return finding{}, false
	}
	used := fmt.Sprintf("%d%% used ≥ %d%%", dfPercent(u.UsedBytes, total), filesystemWarnPercent)
	return finding{
		sentence: row.Mount + " " + used,
		public:   used + " · " + publicMount(row),
		excess:   relative(float64(u.UsedBytes)/float64(total)*100, filesystemWarnPercent),
	}, true
}

// publicMount names a filesystem row on the wall: by its role, since its
// mount point may be a path that says where the installation keeps its data.
// The root row is "/" -- a path every machine has.
func publicMount(row Filesystem) string {
	switch {
	case slices.Contains(row.Roles, roleRoot):
		return "/"
	case slices.Contains(row.Roles, roleDataDir):
		return roleDataDir
	default:
		return "a filesystem"
	}
}

// dfPercent is df's Use%: used / (used + available), rounded up to a whole
// percent -- the figure the Filesystems meter shows.
func dfPercent(used, total uint64) uint64 {
	hi, lo := bits.Mul64(used, 100)
	q, r := bits.Div64(hi, lo, total) // hi < total because used <= total
	if r != 0 {
		q++
	}
	return q
}

// bareTemp is a label the driver made up rather than one it was given.
var bareTemp = regexp.MustCompile(`^temp[0-9]+$`)

// sensorName is how a warning names a temperature: the chip and its label,
// or the chip alone when the label is a bare temp<N> and the chip has only
// this one -- "cpu_thermal", "k10temp Tctl", "nct6798 temp2".
func sensorName(t inventory.HardwareTemperature, onChip int) string {
	if t.Label == "" || (onChip == 1 && bareTemp.MatchString(t.Label)) {
		return t.Chip
	}
	return t.Chip + " " + t.Label
}

// formatLine is a threshold without decimals when it is whole, else to a
// tenth.
func formatLine(c float64) string {
	if c == math.Trunc(c) {
		return strconv.FormatFloat(c, 'f', 0, 64)
	}
	return strconv.FormatFloat(c, 'f', 1, 64)
}

// relative is how far past line value is, as a share of line.
func relative(value, line float64) float64 {
	if line <= 0 {
		return math.Inf(1)
	}
	return (value - line) / line
}

// because ends an unreadable sentence with the reason's own words, or with a
// full stop when there is none.
func because(r *Reason) string {
	if r == nil {
		return "."
	}
	msg := strings.TrimSuffix(strings.TrimSpace(r.Message), ".")
	if msg == "" {
		return "."
	}
	return ": " + msg + "."
}
