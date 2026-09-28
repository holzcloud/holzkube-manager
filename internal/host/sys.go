package host

import (
	"errors"
	"time"
)

// errUnsupported is what every Sys method answers on a platform that is not
// Linux. It becomes the reason code CodeUnsupported.
var errUnsupported = errors.New("holzkube-manager reads the host on Linux only")

// Uname is the part of uname(2) this package uses.
type Uname struct {
	Nodename string
	Release  string
	Machine  string
}

// Loads is the load average as sysinfo(2) reports it: fixed point, shifted by
// SI_LOAD_SHIFT (16).
type Loads struct {
	One, Five, Fifteen uint64
}

// FSStats is the part of statfs(2) this package uses. Blocks, Free and Avail
// count FrameSize-byte units.
type FSStats struct {
	FrameSize uint64
	Blocks    uint64
	Free      uint64
	Avail     uint64
}

// Sys is the four syscalls the host readings need.
//
// They are syscalls rather than files for one reason: each of them survives the
// production unit's ProcSubset=pid, which hides /proc/uptime, /proc/loadavg and
// every other /proc file that is not a process directory. None of them needs
// /proc at all.
type Sys interface {
	// Uname wraps uname(2): hostname, kernel release and uname -m.
	Uname() (Uname, error)

	// BootTime wraps clock_gettime(CLOCK_BOOTTIME): the time since boot,
	// including time spent suspended. Not sysinfo(2)'s uptime, which rounds
	// up to the next second (measured on the Pi: 26091 against /proc/uptime's
	// 26090.33).
	BootTime() (time.Duration, error)

	// Loads wraps sysinfo(2)'s load averages -- the same numbers
	// /proc/loadavg shows, from a call ProcSubset=pid does not hide.
	Loads() (Loads, error)

	// Statfs wraps statfs(2) for the filesystem holding path.
	Statfs(path string) (FSStats, error)
}
