package host

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
)

// The host's history values (Phase 12, D-03): the node's keys, and nothing for
// a value that was not read -- the history's gap is an absent key, and a 0
// written in its place would be a chart of a host gone cold.

// sampleSensorsFS is one CPU chip and one fan header, the way hwmon lists them.
func sampleSensorsFS() fstest.MapFS {
	return fstest.MapFS{
		"sys/class/hwmon/hwmon0/name":        {Data: []byte("cpu_thermal\n")},
		"sys/class/hwmon/hwmon0/temp1_input": {Data: []byte("64400\n")},
		"sys/class/hwmon/hwmon1/name":        {Data: []byte("pwmfan\n")},
		"sys/class/hwmon/hwmon1/fan1_input":  {Data: []byte("2100\n")},
	}
}

// TestHistoryValuesSensors: a readable reading gives exactly its temp: and fan:
// keys; an unreadable one gives an empty map, never a key with a 0; and a
// platform with no readings samples nothing, without an error.
//
// Fault injected and seen red: HistoryValues writing the temp: keys as 0 when
// the reading is hidden.
func TestHistoryValuesSensors(t *testing.T) {
	t.Parallel()

	t.Run("readable", func(t *testing.T) {
		t.Parallel()

		got := HistoryValues(Live{Sensors: Read(Sensors{
			Temperatures: []inventory.HardwareTemperature{{Chip: "cpu_thermal", Kind: "cpu", Label: "temp1", Celsius: 64.4}},
			Fans:         []inventory.HardwareFan{{Chip: "pwmfan", Label: "fan1", RPM: 2100}},
		})})
		want := map[string]float64{"temp:cpu_thermal/temp1": 64.4, "fan:pwmfan/fan1": 2100}
		if len(got) != len(want) {
			t.Errorf("HistoryValues = %v, want exactly %v", got, want)
		}
		for k, v := range want {
			if g, ok := got[k]; !ok || g != v {
				t.Errorf("%s = %v (present %v), want %v", k, g, ok, v)
			}
		}
	})

	t.Run("hidden", func(t *testing.T) {
		t.Parallel()

		got := HistoryValues(Live{Sensors: Hidden[Sensors](Reason{Code: CodeReadFailed, Message: "Could not list /sys/class/hwmon"})})
		if got == nil {
			t.Fatal("HistoryValues of an unreadable reading is nil; want an empty map")
		}
		if len(got) != 0 {
			t.Errorf("HistoryValues of an unreadable reading = %v; want no key at all, never a 0", got)
		}
	})

	t.Run("a collector reads the sensors through Sample", func(t *testing.T) {
		t.Parallel()

		got, err := newTestCollector(sampleSensorsFS(), tracerSys()).Sample(context.Background())
		if err != nil {
			t.Fatalf("Sample: %v", err)
		}
		if got["temp:cpu_thermal/temp1"] != 64.4 || got["fan:pwmfan/fan1"] != 2100 || len(got) != 2 {
			t.Errorf("Sample = %v, want the CPU temperature and the fan", got)
		}
	})

	t.Run("a platform with no readings samples nothing", func(t *testing.T) {
		t.Parallel()

		got, err := newTestCollector(sampleSensorsFS(), newUnsupportedSys()).Sample(context.Background())
		if err != nil {
			t.Fatalf("Sample on an unsupported platform: %v; want no error", err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("Sample on an unsupported platform = %v; want an empty map", got)
		}
	})

	t.Run("a cancelled context samples nothing", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if got, err := newTestCollector(sampleSensorsFS(), tracerSys()).Sample(ctx); err == nil {
			t.Errorf("Sample with a cancelled context = %v, nil; want the context's error", got)
		}
	})
}
