// Package host reads the machine holzkube-manager itself runs on: what it is,
// and how it is doing right now (HOST-01, HMON-01..06).
//
// # Where the readings come from
//
// Everything is read through an fs.FS rooted at "/" and through four syscalls
// behind the Sys interface -- no subprocess, no D-Bus, no root (D-01). The
// production daemon hands in os.DirFS("/") and OS(); the tests hand in fixture
// trees and a fake, so a test about the Pi 5's device-tree model passes on an
// amd64 laptop and a test about DMI passes on the Pi. It is also why nothing in
// this package calls os.ReadFile or os.Open: the file-access guard in
// internal/store/fsstore forbids them outside the store, and an fs.FS does not
// need them.
//
// # Every value is a reading
//
// A systemd unit with ProcSubset=pid, as the reference installation runs the
// daemon, hides /proc/stat and /proc/meminfo. (The repository ships no unit of
// its own; the hardening is the operator's.) A value that could not be read is
// therefore an ordinary state, and it must never be drawn as a 0: 0 % CPU, 0 B of swap and
// 0 cores are all real readings that mean something else (D-02). So every value
// is a Reading, which either carries a value or carries a Reason, and never
// both.
//
// The health package's Field[T] is not reused for this, deliberately. Its value
// is tagged `omitzero`, which drops a readable zero from the JSON -- a machine
// with no swap would arrive looking exactly like a machine whose swap could not
// be read. Reading keeps its value behind a pointer tagged `omitempty`, which
// omits only nil: a readable 0 is sent as 0, and an unread value has no value
// key at all.
package host

import (
	"time"

	"github.com/holzcloud/holzkube-manager/internal/host/hostaction"
	"github.com/holzcloud/holzkube-manager/internal/host/updatestatus"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
)

// Reason says why a value is not there. Code is for the browser to branch on,
// Message is the server's sentence for the operator.
type Reason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// The reason codes. A client branches on these, so they are a contract: see
// docs/api-contract.md, "The machine holzkube-manager runs on".
const (
	// CodeProcSubset: the unit's ProcSubset=pid hides the file this comes
	// from. A consequence of a hardening the operator chose, not a fault.
	CodeProcSubset = "hardening.proc-subset"

	// CodeReadFailed: the source exists in principle and reading it failed.
	// The message names the path and the error.
	CodeReadFailed = "read-failed"

	// CodeNoBaseline: a rate needs two readings and there has been one.
	CodeNoBaseline = "rate.no-baseline"

	// CodeNotRecorded: the update script on this machine does not record its
	// checks. Not an error: there is nothing to read yet.
	CodeNotRecorded = "update.not-recorded"

	// CodeUnsupported: this platform has no such source at all (the daemon
	// also builds for darwin, where none of the Linux interfaces exist).
	CodeUnsupported = "unsupported"

	// CodeNoResult: the host helper has recorded no order yet -- or this
	// instance runs without host actions. Not an error: there is nothing to
	// read yet.
	CodeNoResult = "host-action.no-result"
)

// Reading is one value that was either read or not.
//
// Readable with a Value, or not readable with a Reason -- never both, never a
// zero value standing in for "could not read". Build one with Read or Hidden
// rather than by hand; the two constructors are what keep that invariant.
type Reading[T any] struct {
	Readable bool    `json:"readable"`
	Value    *T      `json:"value,omitempty"`
	Reason   *Reason `json:"reason,omitempty"`
}

// Read is a value that was read.
func Read[T any](v T) Reading[T] {
	return Reading[T]{Readable: true, Value: &v}
}

// Hidden is a value that was not read, and why.
func Hidden[T any](r Reason) Reading[T] {
	return Reading[T]{Reason: &r}
}

// View is the whole answer of GET /api/v1/host, taken at one instant.
//
// ObservedAt is set once per read, so every section in one answer is from the
// same moment (D-11).
type View struct {
	ObservedAt time.Time `json:"observed_at"`

	// Container reports that the daemon runs in a container (D-17): kernel,
	// CPU, memory and temperatures are then the host's, and hostname, network
	// and filesystems the container's.
	Container bool `json:"container"`

	Device Device `json:"device"`

	Service Service `json:"service"`

	Live Live `json:"live"`

	// Health is Assess of Live: the one decision the page's header, the
	// navigation and the wall show, made here so that none of them decides it
	// again (D-06).
	Health Health `json:"health"`

	// Actions is where the host actions stand: the last order this process
	// placed and what the root helper last recorded (HACT-01..08, D-05).
	Actions Actions `json:"actions"`
}

// Actions is the host actions' part of the answer.
//
// Order is this process's own fact -- what it placed and whether the order
// file is still there -- and is null when it placed none since it started.
// Result comes from the helper's state directory, outside the process, and is
// a reading like every other: host-action.no-result when the helper has
// recorded nothing, read-failed when its file is not what the helper writes.
type Actions struct {
	Order  *hostaction.Order          `json:"order"`
	Result Reading[hostaction.Result] `json:"result"`

	// Available is whether this machine can carry out a host action at all:
	// the daemon does not run in a container (D-14), the helper is installed
	// completely (Missing is empty), and this instance was started with host
	// actions. The routes refuse exactly when it is false.
	Available bool `json:"available"`
	// Missing lists the pieces of the helper that are not installed, in the
	// order hostaction.Detect reports them (script, path-unit, not-enabled).
	// Never null: empty when the helper is installed, and when this instance
	// has no host actions to ask for.
	Missing []hostaction.Missing `json:"missing"`
	// Outdated lists what the check for updates needs that is not there, in
	// the order hostaction.Outdated reports it: script-outdated, the
	// installed helper script names no check-update on its marker line and
	// would refuse the order; check-unit, the unit the helper starts for the
	// check is not installed. The four older orders do not need either, so
	// Available stays as it is; only the check is refused. Never null, and
	// empty while anything is Missing: the install commands install
	// everything, the newer script and the check unit with it.
	Outdated []hostaction.Missing `json:"outdated"`
	// InstallCommands are the commands that install the helper, exactly
	// hostaction.InstallCommands -- the page shows them, it does not keep a
	// copy of its own.
	InstallCommands []string `json:"install_commands"`
}

// Service is holzkube-manager itself on this machine (HOST-02, HOST-03).
//
// Version, StartedAt and UptimeSeconds are this process's own facts and are
// always there. The data directory's size and the update status come from
// outside the process and are readings.
type Service struct {
	// Version is the one --version prints: the release tag in a release
	// build, "dev" in a working-tree build.
	Version string `json:"version"`
	// StartedAt is when this process started serving, in UTC.
	StartedAt time.Time `json:"started_at"`
	// UptimeSeconds is whole seconds since StartedAt -- the process's age, not
	// the machine's (that is Device.UptimeSeconds).
	UptimeSeconds int64 `json:"uptime_seconds"`

	DataDir DataDir `json:"data_dir"`

	// Update is what the update script last recorded, or why there is
	// nothing: update.not-recorded when the file does not exist, read-failed
	// when it exists and is not what the script writes. Never an invented
	// time or version (D-16).
	Update Reading[updatestatus.Status] `json:"update"`
}

// DataDir is the daemon's state directory and its size on disk. Its free space
// is the Filesystems row with the "data directory" role.
type DataDir struct {
	Path string           `json:"path"`
	Size Reading[DirSize] `json:"size"`
}

// Live is how the machine is doing at ObservedAt (HMON-01).
//
// CPU usage, memory and swap come from /proc/stat and /proc/meminfo, which the
// production unit's ProcSubset=pid hides: under that unit they are readings
// with the reason hardening.proc-subset, never a 0 (D-02, D-03). Load survives,
// through sysinfo(2) (D-05).
type Live struct {
	// RatesOverSeconds is the window every rate in this answer was computed
	// over, rounded to a tenth of a second, and null while there is no usable
	// previous reading to compute one against (D-12).
	RatesOverSeconds *float64 `json:"rates_over_seconds"`

	CPU    CPU                               `json:"cpu"`
	Memory Reading[inventory.HardwareMemory] `json:"memory"`

	// Filesystems are / and the data directory's filesystem, one row when they
	// are one filesystem (HMON-02, D-07). Never null.
	Filesystems []Filesystem `json:"filesystems"`

	// Sensors are the temperatures and fans from hwmon and, where the CPU has
	// no hwmon chip, the thermal zones (HMON-03, D-08). Not readable only when
	// /sys/class/hwmon exists and could not be listed; a machine without it
	// reads as empty lists.
	Sensors Reading[Sensors] `json:"sensors"`

	// Network is the interfaces, physical and virtual, with their throughput
	// over RatesOverSeconds (HMON-04, D-09). Not readable only when
	// /sys/class/net exists and could not be listed. In a container these are
	// the container's interfaces, not the host's (D-17).
	Network Reading[Network] `json:"network"`
}

// CPU is the processor's share of busy time and the run-queue average.
//
// Usage and PerCore are percentages over RatesOverSeconds: a CPU counter only
// means something as the difference between two readings, so the first answer
// after the daemon starts has none and says so (rate.no-baseline).
type CPU struct {
	Usage   Reading[float64]   `json:"usage"`
	PerCore Reading[[]float64] `json:"per_core"`
	Load    Reading[Load]      `json:"load"`
}

// Device is what the machine is (HOST-01).
//
// Each value comes from a source that survives the unit's ProcSubset=pid (D-04):
// the device tree or DMI under /sys, os-release, the online CPU list under /sys,
// and uname(2) and clock_gettime(2) for the rest.
type Device struct {
	Hostname      Reading[string] `json:"hostname"`
	Model         Reading[string] `json:"model"`
	Arch          Reading[Arch]   `json:"arch"`
	Cores         Reading[int]    `json:"cores"`
	OS            Reading[string] `json:"os"`
	Kernel        Reading[string] `json:"kernel"`
	UptimeSeconds Reading[int64]  `json:"uptime_seconds"`
}

// Load is the run-queue average over one, five and fifteen minutes, and where
// it came from: "loadavg" when /proc/loadavg was readable, "sysinfo" when the
// unit's ProcSubset=pid hid it and sysinfo(2) answered instead. The numbers are
// the same either way (D-05); the source is there so the page can say why load
// is still shown when CPU usage is not.
type Load struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
	Source string  `json:"source"`
}

// Arch is the architecture twice: as Go names it (what this binary was built
// for) and as the kernel names it (uname -m). They say different things on a
// 32-bit userland over a 64-bit kernel, which is why both are shown.
type Arch struct {
	GOARCH  string `json:"goarch"`
	Machine string `json:"machine"`
}
