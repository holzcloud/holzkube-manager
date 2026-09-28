package host

import (
	"errors"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// netClass is where the network interfaces are, as an fs.FS name.
const netClass = "sys/class/net"

// maxLinks bounds the interfaces one read looks at (T-11-20). A host running
// containers has a veth per container, and each interface costs five small
// reads; far above any real machine, far below what a runaway could make it.
// The cut is by name and made before physical and virtual are told apart --
// telling them apart costs a read per interface, which is what the bound is
// there to limit. So on a host over the limit whatever sorts last is not
// looked at, whatever it is: wlan0 sorts after veth*, and would be dropped
// before them.
const maxLinks = 512

// Link is one network interface (HMON-04).
//
// The keys are inventory.HardwareLink's, so the node page and /host speak the
// same JSON for an interface (D-13). The numbers are pointers, not the
// inventory's plain values: a link with no speed and a round with no rate are
// null here, never 0, because 0 B/s is a reading of an idle link (D-02).
type Link struct {
	Name string `json:"name"`
	// Up is whether the interface is up, null when neither its operational
	// state nor its flags could be read -- a state nobody read is not "down".
	Up *bool `json:"up"`
	// SpeedMbit is the negotiated speed, null when the link reports none: it
	// is down (the kernel then answers the read with EINVAL), or it is a kind
	// of link that has no speed.
	SpeedMbit *int `json:"speed_mbit"`
	// RxBytesPerSec and TxBytesPerSec are over Live.RatesOverSeconds, null
	// when this round has no rate for the link: the first read, a link that
	// appeared since the last one, or a counter that went backwards.
	RxBytesPerSec *float64 `json:"rx_bytes_per_sec"`
	TxBytesPerSec *float64 `json:"tx_bytes_per_sec"`
}

// Network is the machine's interfaces, split the way the page shows them
// (D-09). Physical ones -- those with a device behind them -- are listed; the
// virtual ones (loopback, bridges, veths) are collapsed on the page but still
// here, so they are counted rather than silently dropped. Both lists are
// sorted by name and never null.
type Network struct {
	Physical []Link `json:"physical"`
	Virtual  []Link `json:"virtual"`
}

// linkSample is one interface as one read found it: what the page shows, and
// the byte counters the next read's rates are taken against.
type linkSample struct {
	link     Link
	physical bool
	// io is nil when either counter could not be read: such a link has no
	// rate, now or in the next round.
	io *talos.LinkIO
}

// readLinks lists /sys/class/net and reads each interface's state, speed and
// byte counters.
//
// Physical is decided by the device link, never by the name: an interface
// with a device behind it is hardware, whatever it is called, and one without
// is the kernel's own. The entries are symlinks into /sys/devices, so they are
// never filtered on IsDir. The address file is never read: the page has no
// use for a MAC address, and an answer without one cannot leak one.
//
// A missing class directory is a machine without it: no interfaces. Any other
// failure to list it is an error, so the page never says "no interfaces" when
// it could not look.
func readLinks(fsys fs.FS) ([]linkSample, error) {
	entries, err := fs.ReadDir(fsys, netClass)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// fs.ReadDir sorts by name.
	if len(entries) > maxLinks {
		entries = entries[:maxLinks]
	}

	out := make([]linkSample, 0, len(entries))
	for _, e := range entries {
		dir := netClass + "/" + e.Name()
		s := linkSample{link: Link{Name: e.Name()}}

		// fs.Lstat: on a real sysfs device is itself a symlink, and whether
		// the link exists is the question -- not what it points at.
		if _, err := fs.Lstat(fsys, dir+"/device"); err == nil {
			s.physical = true
		}
		s.link.Up = linkUp(fsys, dir)
		// Any failure to read the speed means no speed. A down link answers
		// the read with EINVAL (measured on the Pi 5's wlan0 and lo), and
		// that is the link telling us it has none, not a broken read.
		if raw, ok := readTrimmed(fsys, dir+"/speed"); ok {
			if mbit, err := strconv.Atoi(raw); err == nil && mbit > 0 {
				s.link.SpeedMbit = &mbit
			}
		}
		rx, rxOK := readCounter(fsys, dir+"/statistics/rx_bytes")
		tx, txOK := readCounter(fsys, dir+"/statistics/tx_bytes")
		if rxOK && txOK {
			s.io = &talos.LinkIO{RxBytes: rx, TxBytes: tx}
		}
		out = append(out, s)
	}
	return out, nil
}

// iffUp is IFF_UP from <linux/if.h>: the interface is administratively up.
const iffUp = 0x1

// linkUp is whether the interface at dir is up.
//
// operstate answers it for every driver that reports carrier: "up" is up, and
// "down", "lowerlayerdown", "dormant" and the rest are not. "unknown" is the
// kernel saying it cannot tell -- the loopback always (measured on the Pi 5:
// operstate unknown, flags 0x9), and tun/WireGuard and USB NICs without
// carrier reporting -- and then the administrative flag IFF_UP is the answer,
// as ip(8) shows UP for them. When neither file can be read the state is not
// known, and says so as nil rather than as "down" (D-02).
func linkUp(fsys fs.FS, dir string) *bool {
	if state, ok := readTrimmed(fsys, dir+"/operstate"); ok && state != "" && state != "unknown" {
		up := state == "up"
		return &up
	}
	raw, ok := readTrimmed(fsys, dir+"/flags")
	if !ok {
		return nil
	}
	flags, err := strconv.ParseUint(strings.TrimPrefix(raw, "0x"), 16, 32)
	if err != nil {
		return nil
	}
	up := flags&iffUp != 0
	return &up
}

// readCounter reads one byte counter.
func readCounter(fsys fs.FS, name string) (uint64, bool) {
	raw, ok := readTrimmed(fsys, name)
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// linkCounters is what a read leaves in the memo: the counters of every link
// that had both.
func linkCounters(samples []linkSample) map[string]talos.LinkIO {
	out := make(map[string]talos.LinkIO, len(samples))
	for _, s := range samples {
		if s.io != nil {
			out[s.link.Name] = *s.io
		}
	}
	return out
}

// network builds the answer from this read's samples and, when the window is
// usable, the previous read's counters.
//
// A rate is there only when the link was in the previous read and neither
// counter went backwards. The inventory's perSecond answers 0 for a backwards
// counter -- an interface that was recreated -- which is fine for a sparkline
// but not here: 0 B/s is a reading, and there was none (D-02).
func network(samples []linkSample, prev map[string]talos.LinkIO, secs float64, usable bool) Network {
	n := Network{Physical: make([]Link, 0), Virtual: make([]Link, 0)}
	for _, s := range samples {
		l := s.link
		if was, ok := prev[l.Name]; usable && ok && s.io != nil &&
			s.io.RxBytes >= was.RxBytes && s.io.TxBytes >= was.TxBytes {
			rx := float64(s.io.RxBytes-was.RxBytes) / secs
			tx := float64(s.io.TxBytes-was.TxBytes) / secs
			l.RxBytesPerSec, l.TxBytesPerSec = &rx, &tx
		}
		if s.physical {
			n.Physical = append(n.Physical, l)
		} else {
			n.Virtual = append(n.Virtual, l)
		}
	}
	sort.Slice(n.Physical, func(i, j int) bool { return n.Physical[i].Name < n.Physical[j].Name })
	sort.Slice(n.Virtual, func(i, j int) bool { return n.Virtual[i].Name < n.Virtual[j].Name })
	return n
}
