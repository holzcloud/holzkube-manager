package talos_test

import (
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// TestClassifyChipNamesWhatEachDriverMeasures pins the driver table against the
// names real chips report in their hwmon "name" attribute.
//
// The rows that matter most are the ones a plausible table gets wrong: it87's
// chips name themselves by model ("it8686", not "it87"), a thermal zone's hwmon
// twin swaps '-' for '_', and an unknown chip is "other" rather than dropped.
func TestClassifyChipNamesWhatEachDriverMeasures(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]talos.SensorKind{
		"coretemp":         talos.SensorCPU,
		"k10temp":          talos.SensorCPU,
		"zenpower":         talos.SensorCPU,
		"cpu_thermal":      talos.SensorCPU,
		"cpu-thermal":      talos.SensorCPU,
		"x86_pkg_temp":     talos.SensorCPU,
		"nvme":             talos.SensorDisk,
		"drivetemp":        talos.SensorDisk,
		"nct6798":          talos.SensorBoard,
		"nct6775":          talos.SensorBoard,
		"it8686":           talos.SensorBoard,
		"it8728":           talos.SensorBoard,
		"w83627ehf":        talos.SensorBoard,
		"asus_wmi_sensors": talos.SensorBoard,
		"asusec":           talos.SensorBoard,
		"acpitz":           talos.SensorBoard,
		"pch_cannonlake":   talos.SensorBoard,
		"amdgpu":           talos.SensorGPU,
		"nouveau":          talos.SensorGPU,
		"radeon":           talos.SensorGPU,
		"iwlwifi_1":        talos.SensorOther,
		"BAT0":             talos.SensorOther,
		"":                 talos.SensorOther,
	} {
		if got := talos.ClassifyChip(name); got != want {
			t.Errorf("ClassifyChip(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestThermalTwinName pins the name a thermal zone's hwmon twin registers as.
// The node path and the host path both skip a zone whose twin is listed, and
// this is the one rule they share for it.
func TestThermalTwinName(t *testing.T) {
	t.Parallel()

	for zone, want := range map[string]string{
		"cpu-thermal":  "cpu_thermal",
		"acpitz":       "acpitz",
		"x86_pkg_temp": "x86_pkg_temp",
	} {
		if got := talos.ThermalTwinName(zone); got != want {
			t.Errorf("ThermalTwinName(%q) = %q, want %q", zone, got, want)
		}
	}
}
