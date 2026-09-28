//go:build linux

package host

import (
	"io/fs"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// osSys is Sys over the real kernel.
type osSys struct{}

// OS is the running kernel's Sys.
func OS() Sys { return osSys{} }

func (osSys) Uname() (Uname, error) {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return Uname{}, err
	}
	return Uname{
		Nodename: unix.ByteSliceToString(u.Nodename[:]),
		Release:  unix.ByteSliceToString(u.Release[:]),
		Machine:  unix.ByteSliceToString(u.Machine[:]),
	}, nil
}

func (osSys) BootTime() (time.Duration, error) {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_BOOTTIME, &ts); err != nil {
		return 0, err
	}
	return time.Duration(ts.Nano()), nil
}

func (osSys) Loads() (Loads, error) {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return Loads{}, err
	}
	// The field is uint64 on 64-bit kernels and uint32 on 32-bit ones; both
	// widen without loss.
	return Loads{
		One:     uint64(info.Loads[0]), //nolint:unconvert // uint32 on 32-bit linux
		Five:    uint64(info.Loads[1]), //nolint:unconvert // uint32 on 32-bit linux
		Fifteen: uint64(info.Loads[2]), //nolint:unconvert // uint32 on 32-bit linux
	}, nil
}

func (osSys) Statfs(path string) (FSStats, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return FSStats{}, err
	}
	// Frsize is the unit Blocks, Bfree and Bavail count in. Some filesystems
	// leave it 0 and mean Bsize.
	frame := uint64(st.Frsize) //nolint:gosec,unconvert // a block size is positive; int64 or int32 by arch
	if frame == 0 {
		frame = uint64(st.Bsize) //nolint:gosec,unconvert // a block size is positive; int64 or int32 by arch
	}
	return FSStats{
		FrameSize: frame,
		Blocks:    uint64(st.Blocks), //nolint:unconvert // uint32 on some 32-bit arches
		Free:      uint64(st.Bfree),  //nolint:unconvert // uint32 on some 32-bit arches
		Avail:     uint64(st.Bavail), //nolint:unconvert // uint32 on some 32-bit arches
	}, nil
}

// statBlocks is what du(1) counts for one entry: its allocated 512-byte blocks,
// and the device and inode that make a hard link count once.
func statBlocks(fi fs.FileInfo) (blocks int64, dev, ino uint64, ok bool) {
	st, isStat := fi.Sys().(*syscall.Stat_t)
	if !isStat || st == nil {
		return 0, 0, 0, false
	}
	return st.Blocks, uint64(st.Dev), st.Ino, true //nolint:unconvert,gosec // Dev is uint32 on some 32-bit arches
}
