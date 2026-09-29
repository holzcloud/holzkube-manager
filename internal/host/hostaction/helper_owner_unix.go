//go:build unix

package hostaction

import (
	"io/fs"
	"syscall"
)

// ownerUID is the uid that owns the file info describes, and whether it could
// be told: the stat the kernel filled in carries it.
func ownerUID(info fs.FileInfo) (uint32, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return 0, false
	}
	return st.Uid, true
}
