package inventory_test

import (
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
)

func ptr(v float64) *float64 { return &v }

// TestTemperatureLimits is the browser's temperatureLimits, case for case,
// now that the rule lives here (D-06). The first two rows are the two cases
// NodeHardware.test.tsx held, under the names they had there.
func TestTemperatureLimits(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name               string
		kind               string
		high, crit         *float64
		wantWarn, wantDang float64
	}{
		{"prefer the chip's own numbers", "cpu", ptr(70), ptr(90), 70, 90},
		{"fall back to the kind when the chip says nothing", "disk", nil, nil, 60, 70},

		{"cpu default", "cpu", nil, nil, 80, 95},
		{"board default", "board", nil, nil, 70, 85},
		{"gpu default", "gpu", nil, nil, 80, 95},
		{"other default", "other", nil, nil, 75, 90},
		{"an empty kind is other", "", nil, nil, 75, 90},
		{"an unknown kind is other", "psu", nil, nil, 75, 90},

		// A high limit at or above danger is no warning line at all.
		{"high 100, crit 90: high ignored", "cpu", ptr(100), ptr(90), 80, 90},
		{"high equal to crit is ignored", "cpu", ptr(90), ptr(90), 80, 90},
		// The warning line never lies above the red one.
		{"no high, crit 70: warn never above danger", "cpu", nil, ptr(70), 70, 70},
		// The Pi 5: its CPU zone says only where it shuts down.
		{"no high, crit 110 (the Pi 5)", "cpu", nil, ptr(110), 80, 110},
		{"high only", "board", ptr(60), nil, 60, 85},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			warn, danger := inventory.TemperatureLimits(tc.kind, tc.high, tc.crit)
			if warn != tc.wantWarn || danger != tc.wantDang {
				t.Errorf("TemperatureLimits(%q, %v, %v) = %v/%v, want %v/%v",
					tc.kind, show(tc.high), show(tc.crit), warn, danger, tc.wantWarn, tc.wantDang)
			}
		})
	}
}

func show(p *float64) any {
	if p == nil {
		return "nil"
	}
	return *p
}
