package host

import (
	"errors"
	"time"
)

// errUnsupported is what every Sys method answers on a platform that is not
// Linux. It becomes the reason code CodeUnsupported.
var errUnsupported = errors.New("holzkube-manager reads the host on Linux only")

// unsupportedPlatform is what a Sys implements on a platform this package has
// no readings for at all. The collector then reads nothing: /sys, /proc and
// /etc/os-release are Linux's, and on darwin their absence would come out as
// readable empty lists ("no sensors", "no interfaces") and as read failures
// naming files that never existed there -- claims about a machine nobody
// looked at.
type unsupportedPlatform interface{ unsupportedPlatform() }

// platformUnsupported reports whether s is such a Sys.
func platformUnsupported(s Sys) bool {
	_, ok := s.(unsupportedPlatform)
	return ok
}

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
