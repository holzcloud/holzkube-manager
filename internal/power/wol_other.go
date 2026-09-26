//go:build !unix

package power

import (
	"context"
	"errors"
	"net"
)

// errNoBroadcast is what a platform without SO_BROADCAST reports.
var errNoBroadcast = errors.New("power: this platform cannot send a broadcast")

// listenBroadcast is unavailable off unix. The daemon ships for Linux and
// macOS only (.goreleaser.yaml); this exists so the package still compiles
// wherever `go vet ./...` is run.
func listenBroadcast(context.Context) (net.PacketConn, error) {
	return nil, errNoBroadcast
}
