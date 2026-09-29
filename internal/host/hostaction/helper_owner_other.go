//go:build !unix

package hostaction

import "io/fs"

// ownerUID cannot tell an owner on this platform, so no helper counts as
// installed here. The helper is a systemd unit on a Linux host.
func ownerUID(fs.FileInfo) (uint32, bool) {
	return 0, false
}
