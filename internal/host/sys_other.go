//go:build !linux

package host

import "time"

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
