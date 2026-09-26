package power_test

import (
	"bytes"
	"net"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/power"
)

// Wake-on-LAN, in the two halves a test can reach: the bytes, and where they
// are sent. Whether a real card wakes is the half no test here can reach, and
// the package says so.

// TestTheMagicPacketIsSixFFsAndTheMACSixteenTimes, byte for byte.
//
// A card that sees anything else ignores it without a word, and there is no
// answer to wait for -- a wrong packet would look exactly like a card that was
// never armed in its firmware, which is the first thing an operator would
// spend an evening checking.
func TestTheMagicPacketIsSixFFsAndTheMACSixteenTimes(t *testing.T) {
	t.Parallel()

	mac, err := net.ParseMAC("a8:a1:59:12:34:56")
	if err != nil {
		t.Fatalf("ParseMAC: %v", err)
	}
	packet, err := power.MagicPacket(mac)
	if err != nil {
		t.Fatalf("MagicPacket: %v", err)
	}

	if len(packet) != 102 {
		t.Fatalf("the packet is %d bytes, want 102 (6 + 16 x 6)", len(packet))
	}
	if !bytes.Equal(packet[:6], []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Errorf("the synchronisation stream is % x, want six 0xFF", packet[:6])
	}
	for i := range 16 {
		got := packet[6+i*6 : 6+(i+1)*6]
		if !bytes.Equal(got, mac) {
			t.Fatalf("repetition %d of the address is % x, want % x", i+1, got, []byte(mac))
		}
	}

	// An address that is not six bytes is not something a card compares
	// against, and sending one would be sending nothing while saying it was.
	long, _ := net.ParseMAC("02:00:5e:10:00:00:00:01")
	if _, err := power.MagicPacket(long); err == nil {
		t.Error("an eight-byte address was accepted")
	}
}

// TestPacketsGoToEveryBroadcastThatReachesTheLAN: the limited broadcast first,
// then the directed broadcast of each network this host is on -- and not the
// ones that reach nothing.
func TestPacketsGoToEveryBroadcastThatReachesTheLAN(t *testing.T) {
	t.Parallel()

	network := func(cidr string, up, loopback, broadcast bool) power.LocalNetwork {
		ip, n, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Fatalf("ParseCIDR %s: %v", cidr, err)
		}
		n.IP = ip
		return power.LocalNetwork{Addr: n, Up: up, Loopback: loopback, Broadcast: broadcast}
	}

	got := power.Destinations([]power.LocalNetwork{
		network("192.168.1.10/24", true, false, true), // the Pi's LAN port
		network("10.20.3.4/16", true, false, true),    // a second network on another port
		network("192.168.1.11/24", true, false, true), // a second address on the same LAN: once, not twice
		network("127.0.0.1/8", true, true, false),     // loopback reaches this host and nothing else
		network("172.17.0.1/16", false, false, true),  // an interface that is down
		network("10.99.0.1/24", true, false, false),   // a tunnel: no broadcast at all
		network("fd00::5/64", true, false, true),      // IPv6 has no broadcast and no Wake-on-LAN
		network("192.0.2.7/32", true, false, true),    // a host route: no broadcast address
		network("198.51.100.2/31", true, false, true), // a point-to-point pair: none either
	}, power.WakePort)

	want := []string{"255.255.255.255:9", "192.168.1.255:9", "10.20.255.255:9"}
	if !slices.Equal(got, want) {
		t.Fatalf("destinations = %v, want %v", got, want)
	}
}

// TestAPacketLeavesThisProcessAsABroadcast: the real sender, over a real
// socket, to a listener this test holds.
//
// The destination is the loopback network's broadcast address. It is a
// broadcast like 255.255.255.255 -- Linux refuses to send to it from a socket
// without SO_BROADCAST, with the same EACCES -- and it never leaves this host,
// so the test proves the socket is opened the way a LAN broadcast needs without
// putting a packet on anybody's network. It cannot prove a card woke; nothing
// here can.
func TestAPacketLeavesThisProcessAsABroadcast(t *testing.T) {
	t.Parallel()

	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		t.Skipf("no UDP here: %v", err)
	}
	defer conn.Close() //nolint:errcheck // test
	local, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		t.Fatalf("a UDP listener reports a %T address", conn.LocalAddr())
	}
	port := local.Port

	mac, _ := net.ParseMAC("02:42:ac:11:00:02")
	waker := power.NetWaker{Only: []string{net.JoinHostPort("127.255.255.255", strconv.Itoa(port))}}
	if _, err := waker.Wake(t.Context(), []net.HardwareAddr{mac}); err != nil {
		if strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("the socket may not send a broadcast -- SO_BROADCAST is not set, and every "+
				"magic packet on a real LAN would be refused the same way: %v", err)
		}
		t.Skipf("this host does not route a broadcast on its loopback network: %v", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 512)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		if runtime.GOOS != "linux" {
			t.Skipf("no loopback broadcast arrived on %s: %v", runtime.GOOS, err)
		}
		t.Fatalf("nothing arrived: %v", err)
	}
	want, _ := power.MagicPacket(mac)
	if !bytes.Equal(buf[:n], want) {
		t.Errorf("arrived % x, want the magic packet", buf[:n])
	}
}

// TestOnlyARealNetworkCardWakes: a machine is woken by the addresses of its
// physical links -- not a bond's, not a CNI's, not loopback's all-zero one.
func TestOnlyARealNetworkCardWakes(t *testing.T) {
	t.Parallel()

	got := power.MACsOf([]power.InterfaceAddr{
		{HardwareAddr: "a8:a1:59:12:34:56"},
		{HardwareAddr: "A8:A1:59:12:34:56"}, // the same card, spelled differently
		{HardwareAddr: "a8:a1:59:12:34:57"},
		{HardwareAddr: "02:00:00:00:00:99", Kind: "bond"},
		{HardwareAddr: "ee:ee:ee:ee:ee:ee", Kind: "veth"},
		{HardwareAddr: "00:00:00:00:00:00"},
		{HardwareAddr: ""},
	})
	var names []string
	for _, m := range got {
		names = append(names, m.String())
	}
	want := []string{"a8:a1:59:12:34:56", "a8:a1:59:12:34:57"}
	if !slices.Equal(names, want) {
		t.Fatalf("woken by %v, want %v", names, want)
	}
}
