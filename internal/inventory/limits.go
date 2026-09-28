package inventory

// Where a temperature turns amber and where it turns red.
//
// This is the one rule for it (D-06). The node page, the host page and the
// host's state all show a temperature against these two numbers, and before
// this file the browser worked them out for itself -- which is fine for a
// colour and useless for a state the wall shows without a browser's help.
// The rule is the browser's, moved here case for case.
//
// The chip's own high and crit win whenever it reports them: Intel's TjMax,
// an NVMe drive's warning composite temperature, are the manufacturer's
// numbers and better than any default. The defaults below are for chips that
// report only a reading, and they are the conventional ones -- a desktop CPU
// at 80 °C is working hard, a drive at 60 °C is past where its life shortens.

// temperatureDefault is one kind's pair of lines.
type temperatureDefault struct{ warn, danger float64 }

// temperatureDefaults are the lines of a chip that reports none, by kind.
// A kind not listed here is "other".
var temperatureDefaults = map[string]temperatureDefault{
	"cpu":   {warn: 80, danger: 95},
	"disk":  {warn: 60, danger: 70},
	"board": {warn: 70, danger: 85},
	"gpu":   {warn: 80, danger: 95},
	"other": {warn: 75, danger: 90},
}

// TemperatureLimits is where a sensor of this kind turns amber (warn) and red
// (danger), given the chip's own high and critical limits, either nil when
// the chip sets none.
//
// danger is the chip's critical limit, or the kind's default. warn is the
// chip's high limit when it lies below danger, otherwise the kind's default --
// never above danger, so a chip whose critical limit is below the default
// warning line is amber no later than it is red.
func TemperatureLimits(kind string, high, crit *float64) (warn, danger float64) {
	fallback, ok := temperatureDefaults[kind]
	if !ok {
		fallback = temperatureDefaults["other"]
	}

	danger = fallback.danger
	if crit != nil {
		danger = *crit
	}
	if high != nil && *high < danger {
		return *high, danger
	}
	return min(fallback.warn, danger), danger
}
