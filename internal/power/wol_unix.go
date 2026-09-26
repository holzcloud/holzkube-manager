//go:build unix

package power

import (
	"context"
	"net"
	"syscall"
)

// listenBroadcast opens a UDP socket that may send to a broadcast address.
//
// SO_BROADCAST is not optional: Linux refuses a send to 255.255.255.255 or to a
// directed broadcast from a socket without it, with EACCES -- which reads as a
// permissions problem and is not one.
func listenBroadcast(ctx context.Context) (net.PacketConn, error) {
	lc := net.ListenConfig{
		Control: func(_, _ string, c syscall.RawConn) error {
			var sockErr error
			if err := c.Control(func(fd uintptr) {
				sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
			}); err != nil {
				return err
			}
			return sockErr
		},
	}
	return lc.ListenPacket(ctx, "udp4", ":0")
}
