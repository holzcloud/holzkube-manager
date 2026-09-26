package power

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
)

// Wake-on-LAN (2026-09-26).
//
// # Why this daemon, and why broadcast
//
// Start of a node that is off has exactly one way in that needs nothing on the
// node: its network card listening for a magic packet. The daemon is the right
// thing to send it because it is the one part of the installation that is still
// running when the cluster is not -- it runs on the operator's Pi, beside the
// cluster and on the same LAN, which is also the only place a Wake-on-LAN
// packet reaches.
//
// A card that is asleep has no IP address and answers no ARP, so the packet
// cannot be addressed to the machine. It is broadcast, and the card recognises
// its own MAC address inside the payload. Two kinds of broadcast, both sent:
//
//   - 255.255.255.255, the limited broadcast. It leaves by whichever interface
//     the routing table picks for it and stops at the first router.
//   - the directed broadcast of every IPv4 network this host is on -- the
//     192.168.1.255 of a 192.168.1.0/24. It leaves by that network's own
//     interface, which is what reaches a node on the second of two networks a
//     Pi with two ports sits on, where the limited broadcast would only ever
//     have gone out of the first.
//
// UDP port 9, the discard port, by convention: the card does not care which
// port, and 9 is the one a router is least likely to forward anywhere.
//
// # What this cannot verify
//
// That the node woke. A magic packet has no answer; whether the card was armed
// in the firmware, whether the switch forwarded a broadcast to a port whose link
// is down, whether the machine had power at all -- none of that comes back. The
// only evidence is the node answering its API afterwards, which is why a start
// is a wake followed by a wait rather than a wake.
//
// And this: a daemon run in a container on a bridge network broadcasts onto the
// bridge, not onto the LAN. The Pi runs it on the host, which is the deployment
// this was written for.

// WakePort is where a magic packet goes.
const WakePort = 9

// MagicPacket is the payload Wake-on-LAN cards listen for: six bytes of 0xFF,
// then the card's own MAC address sixteen times.
//
// The format is the whole protocol, and a card that sees anything else ignores
// it silently -- which, with no answer to wait for, would look exactly like a
// card that was never armed. So it is built here once and tested byte for byte.
func MagicPacket(mac net.HardwareAddr) ([]byte, error) {
	if len(mac) != 6 {
		return nil, fmt.Errorf("power: %q is not a 6-byte Ethernet address", mac)
	}
	packet := make([]byte, 0, 6+16*6)
	for range 6 {
		packet = append(packet, 0xFF)
	}
	for range 16 {
		packet = append(packet, mac...)
	}
	return packet, nil
}

// LocalNetwork is one IPv4 network this host sits on, as far as a broadcast is
// concerned.
type LocalNetwork struct {
	// Interface is the name, for the sentence that says where packets went.
	Interface string

	// Addr is this host's address and mask on it.
	Addr *net.IPNet

	// Up, Loopback and Broadcast are the interface's flags that decide whether
	// a broadcast out of it means anything.
	Up        bool
	Loopback  bool
	Broadcast bool
}

// Destinations is every address a magic packet is sent to: the limited
// broadcast first, then the directed broadcast of each IPv4 network this host
// is on, each once.
//
// It is a function of the networks rather than a read of the host, so a test
// can hand it a Pi's two ports and a Docker bridge and see which it chooses.
// Left out, each for its own reason: a loopback interface, which reaches this
// host and nothing else; an interface that is down; one that does not do
// broadcast (a WireGuard tunnel, a point-to-point link); IPv6, which has no
// broadcast and no Wake-on-LAN; and a /31 or /32, which has no broadcast
// address to send to.
func Destinations(networks []LocalNetwork, port int) []string {
	out := []string{net.JoinHostPort(net.IPv4bcast.String(), strconv.Itoa(port))}
	seen := map[string]bool{out[0]: true}

	for _, n := range networks {
		if !n.Up || n.Loopback || !n.Broadcast || n.Addr == nil {
			continue
		}
		ip := n.Addr.IP.To4()
		if ip == nil {
			continue
		}
		mask := n.Addr.Mask
		if len(mask) == net.IPv6len {
			mask = mask[12:]
		}
		if ones, bits := mask.Size(); bits != 32 || ones > 30 {
			continue
		}
		broadcast := make(net.IP, 4)
		for i := range 4 {
			broadcast[i] = ip[i] | ^mask[i]
		}
		dest := net.JoinHostPort(broadcast.String(), strconv.Itoa(port))
		if !seen[dest] {
			seen[dest] = true
			out = append(out, dest)
		}
	}
	return out
}

// HostNetworks reads this host's IPv4 networks for Destinations.
func HostNetworks() ([]LocalNetwork, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("power: listing this host's interfaces: %w", err)
	}
	var out []LocalNetwork
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			out = append(out, LocalNetwork{
				Interface: iface.Name,
				Addr:      ipnet,
				Up:        iface.Flags&net.FlagUp != 0,
				Loopback:  iface.Flags&net.FlagLoopback != 0,
				Broadcast: iface.Flags&net.FlagBroadcast != 0,
			})
		}
	}
	return out, nil
}

// Waker sends Wake-on-LAN packets. It is an interface so that a test can
// stand in for the network card at the far end: the real one has no answer to
// observe, and a test that sent real broadcasts would be testing the kernel.
type Waker interface {
	// Wake sends one magic packet per MAC address to every destination, and
	// says where they went. It fails only when not a single packet could be
	// sent; a destination that refused is named in the answer instead.
	Wake(ctx context.Context, macs []net.HardwareAddr) (WakeReport, error)
}

// WakeReport is what a wake did, for the job's sentence.
type WakeReport struct {
	Sent         []string `json:"sent"`
	Failed       []string `json:"failed,omitempty"`
	Destinations []string `json:"destinations"`
}

// NetWaker is the Waker that sends real packets.
type NetWaker struct {
	// Port is the destination port; zero means WakePort.
	Port int

	// Networks lists this host's networks; nil means HostNetworks. A test
	// narrows it to a loopback address it can listen on.
	Networks func() ([]LocalNetwork, error)

	// Only, when set, replaces the computed destinations with these
	// ("host:port"). It exists for the test that proves a packet leaves this
	// process through a broadcast-enabled socket, which sends to the loopback
	// network's broadcast address -- a broadcast that needs SO_BROADCAST like
	// any other and never leaves the host -- and nowhere else.
	Only []string
}

// Wake implements Waker.
func (w NetWaker) Wake(ctx context.Context, macs []net.HardwareAddr) (WakeReport, error) {
	port := w.Port
	if port == 0 {
		port = WakePort
	}
	list := w.Networks
	if list == nil {
		list = HostNetworks
	}
	networks, err := list()
	if err != nil {
		// Without the directed broadcasts the limited one is still worth
		// sending: on a host with one network it is the only one that matters.
		networks = nil
	}
	dests := Destinations(networks, port)
	if len(w.Only) > 0 {
		dests = w.Only
	}

	conn, err := listenBroadcast(ctx)
	if err != nil {
		return WakeReport{Destinations: dests}, fmt.Errorf("power: opening a socket to send Wake-on-LAN from: %w", err)
	}
	defer conn.Close() //nolint:errcheck // a packet that was sent was sent

	report := WakeReport{Destinations: dests}
	for _, mac := range macs {
		packet, err := MagicPacket(mac)
		if err != nil {
			report.Failed = append(report.Failed, mac.String()+": "+err.Error())
			continue
		}
		for _, dest := range dests {
			addr, err := net.ResolveUDPAddr("udp4", dest)
			if err == nil {
				_, err = conn.WriteTo(packet, addr)
			}
			if err != nil {
				report.Failed = append(report.Failed, mac.String()+" to "+dest+": "+err.Error())
				continue
			}
			report.Sent = append(report.Sent, mac.String()+" to "+dest)
		}
	}
	if len(report.Sent) == 0 {
		return report, fmt.Errorf("power: no Wake-on-LAN packet could be sent: %s",
			strings.Join(report.Failed, "; "))
	}
	return report, nil
}

// MACsOf is the set of addresses a machine can be woken by, read from the last
// snapshot -- the only thing that survives while it is off, because a machine
// that is off cannot be asked.
//
// A physical link has an empty Kind in Talos; a bond, a bridge, a VLAN or a
// CNI's veth has one and a MAC nobody's firmware listens on. Loopback's all-
// zero address is not an address. Sorted and de-duplicated, so the same
// machine is woken by the same packets every time.
func MACsOf(ifaces []InterfaceAddr) []net.HardwareAddr {
	seen := map[string]bool{}
	var out []net.HardwareAddr
	for _, iface := range ifaces {
		if iface.Kind != "" {
			continue
		}
		mac, err := net.ParseMAC(iface.HardwareAddr)
		if err != nil || len(mac) != 6 || allBytes(mac, 0) || allBytes(mac, 0xFF) {
			continue
		}
		if seen[mac.String()] {
			continue
		}
		seen[mac.String()] = true
		out = append(out, mac)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// DryRunWaker is the Waker of an instance started with --dry-run: it sends
// nothing and says so.
//
// A magic packet is not a Talos call, so the transport's own dry-run refusal
// never sees it -- and switching a machine on is as much a change to the fleet
// as switching one off. Without this a dry-run instance would refuse every
// shutdown and still power machines up.
type DryRunWaker struct{}

// Wake implements Waker by refusing.
func (DryRunWaker) Wake(context.Context, []net.HardwareAddr) (WakeReport, error) {
	return WakeReport{}, fmt.Errorf("power: this instance runs with --dry-run, so no Wake-on-LAN " +
		"packet is sent and no machine is switched on")
}

// InterfaceAddr is the two fields of a recorded interface MACsOf reads.
type InterfaceAddr struct {
	HardwareAddr string
	Kind         string
}

func allBytes(b []byte, v byte) bool {
	for _, x := range b {
		if x != v {
			return false
		}
	}
	return true
}
