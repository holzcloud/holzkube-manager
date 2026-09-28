//go:build !linux

package host

import (
	"io/fs"
	"time"
)

// osSys is Sys on a platform this package has no readings for. The daemon
// builds for darwin; there every section of the page is "not readable", with
// the reason saying why, and the page still renders.
type osSys struct{}

// OS is the running kernel's Sys.
func OS() Sys { return osSys{} }

func (osSys) Uname() (Uname, error)            { return Uname{}, errUnsupported }
func (osSys) BootTime() (time.Duration, error) { return 0, errUnsupported }
func (osSys) Loads() (Loads, error)            { return Loads{}, errUnsupported }
func (osSys) Statfs(string) (FSStats, error)   { return FSStats{}, errUnsupported }

// unsupportedPlatform makes the collector read nothing here (WR-03). The
// assertion keeps a darwin build from compiling without it.
func (osSys) unsupportedPlatform() {}

var _ unsupportedPlatform = osSys{}

// statBlocks has nothing to read here; the size walk reports errUnsupported.
func statBlocks(fs.FileInfo) (blocks int64, dev, ino uint64, ok bool) { return 0, 0, 0, false }
