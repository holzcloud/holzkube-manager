// Package provision turns a blank machine into a cluster node.
//
// It is the core value, and it is also the part of this product that can
// destroy something. Two properties therefore shape everything here.
//
// **Nothing is ever reported as "nothing".** A subnet scan that finds a node
// which is already configured says so; it does not report an empty result and
// leave the operator to conclude the machine is not there. The difference
// matters because the two lead to opposite actions: one is "check the cable",
// the other is "this machine is already in use".
//
// **The machine is identified again immediately before anything is written.**
// A wizard that read a machine's identity two minutes ago and applies a
// configuration now is a wizard that can hit a different machine -- DHCP moved
// a lease, somebody rebooted something -- and hitting the wrong machine wipes
// it.
package provision

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// ScanConcurrency is how many addresses are probed at once.
//
// Bounded, and not large. A /24 is 254 addresses; probing them all at once
// opens 254 sockets from one host, which some home routers treat as a scan and
// some NAT tables simply drop. Sixteen finishes a /24 in a few seconds and
// looks like a host doing something ordinary.
const ScanConcurrency = 16

// ScanTimeout is how long one address gets.
//
// Short, because the answer for almost every address is "nothing is there",
// and that answer arrives as a refused connection or not at all. A long
// timeout would make a /24 scan take minutes to tell the operator what it
// already knew about 250 of the addresses.
const ScanTimeout = 2 * time.Second

// Found is one machine a scan turned up.
type Found struct {
	Addr string `json:"addr"`

	// State is what was found there, and it is never "nothing" for an address
	// that answered (PROV-02).
	State FoundState `json:"state"`

	// Hostname is the name on the certificate the machine presented, and it is
	// the only thing the machine said about itself that a scan can learn:
	// nothing here has authenticated to the node, so nothing here has asked it
	// a question. It is unverified in both directions -- see
	// MaintenanceWarning.
	//
	// There is deliberately no UUID field. A scan cannot learn a machine's UUID
	// without connecting to it, and a UUID invented from the address the probe
	// was aimed at would be a value that looks like an identity and is not one.
	// Inspect is where the identity comes from, and VerifyMachine reads it
	// again immediately before anything is written (PROV-05).
	Hostname string `json:"hostname,omitempty"`

	// Fingerprint is the SHA-256 of the certificate the machine presented, in
	// the same colon-separated upper-case hex the adoption flow uses. It is
	// what an operator can compare against the machine's physical console.
	Fingerprint string `json:"fingerprint,omitempty"`

	// Known reports that holzkube-manager already manages a machine at this
	// address, which is a different thing from the machine being configured: a
	// configured node this installation has never heard of is somebody else's,
	// and one it knows about is its own.
	//
	// It is keyed by address rather than by UUID because the address is what a
	// scan has. The inventory's own record of where a machine was last seen is
	// the other half of the comparison.
	Known bool `json:"known"`

	// Detail says what happened, for an address that answered in a way that
	// does not fit the states above.
	Detail string `json:"detail,omitempty"`
}

// FoundState is what is at an address.
type FoundState string

const (
	// StateMaintenance is an unconfigured machine waiting for a configuration.
	// This is the one a provisioning wizard wants.
	StateMaintenance FoundState = "maintenance"

	// StateConfigured is a machine that already has a configuration.
	//
	// It is reported rather than filtered out, and that is PROV-02. "Nothing
	// found" and "there is already a node here" lead to opposite actions, and
	// a scan that conflates them sends the operator to check a cable while a
	// running node sits at the address they were about to overwrite.
	StateConfigured FoundState = "configured"

	// StateAnswered is an address that accepted a connection and then did not
	// complete a TLS handshake this scan could read anything from -- something
	// that is not a Talos node, or one that is still coming up.
	//
	// It is reported rather than dropped, and that is PROV-02 again: an
	// operator aiming a provisioning run at an address needs to know that
	// something is there, even when what is there cannot be identified.
	StateAnswered FoundState = "answered"
)

// ScanRequest is what to scan.
type ScanRequest struct {
	// CIDR is the subnet, e.g. "192.168.1.0/24".
	CIDR string

	// Addrs are individual addresses, the always-available second way in. An
	// operator whose machine is on another subnet, or who simply knows the
	// address, does not have to scan anything.
	Addrs []string
}

// Scanner finds machines.
type Scanner struct {
	dialer talos.Dialer

	// known reports whether the inventory already has a machine at an address.
	// It is a function rather than a store because this package has no business
	// knowing how the inventory is stored.
	known func(ctx context.Context, addr string) bool
}

// NewScanner builds a scanner.
func NewScanner(d talos.Dialer, known func(context.Context, string) bool) *Scanner {
	if known == nil {
		known = func(context.Context, string) bool { return false }
	}
	return &Scanner{dialer: d, known: known}
}

// Scan probes a set of addresses and reports what is at each one that
// answered.
//
// Addresses that do not answer are simply absent from the result. That is the
// one place "nothing" is a valid report, and it is about an address rather
// than about a machine.
func (s *Scanner) Scan(ctx context.Context, req ScanRequest) ([]Found, error) {
	targets, err := expand(req)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("provision: nothing to scan")
	}

	work := make(chan string)
	results := make(chan Found)

	var wg sync.WaitGroup
	for range ScanConcurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for addr := range work {
				if found, ok := s.probe(ctx, addr); ok {
					results <- found
				}
			}
		}()
	}

	go func() {
		defer close(work)
		for _, addr := range targets {
			select {
			case <-ctx.Done():
				return
			case work <- addr:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	out := make([]Found, 0, 8)
	for found := range results {
		out = append(out, found)
	}

	// Sorted by address, numerically where possible, so two scans of the same
	// subnet list the same machines in the same order.
	sortByAddr(out)
	return out, nil
}

// probe asks one address what it is.
func (s *Scanner) probe(ctx context.Context, addr string) (Found, bool) {
	probeCtx, cancel := context.WithTimeout(ctx, ScanTimeout)
	defer cancel()

	target := talos.Target{Machine: model.MachineID("scan:" + addr), Addr: addr}

	identity, err := s.dialer.Probe(probeCtx, target)
	if err != nil {
		if errors.Is(err, talos.ErrNotReachableYet) {
			// Nothing answered. This is the one "nothing" this package
			// reports, and it is about the address rather than about a
			// machine.
			return Found{}, false
		}

		// Something accepted a connection and then did not speak TLS in a way
		// this could read. That is not nothing, and reporting it as nothing is
		// the exact failure PROV-02 is about.
		return Found{
			Addr:   addr,
			State:  StateAnswered,
			Known:  s.known(ctx, addr),
			Detail: "Something answered on the Talos API port and did not identify itself as a Talos machine.",
		}, true
	}

	found := Found{Addr: addr, Hostname: identity.Hostname, Known: s.known(ctx, addr)}

	// The fingerprint is read on the same pass, because it is what an operator
	// compares against the machine's physical console -- and asking for it in
	// a second round trip would mean the value they confirm came from a
	// different connection than the one they are about to trust.
	if fp, err := talos.ServerFingerprint(probeCtx, s.dialer, target); err == nil {
		found.Fingerprint = fp
	}

	// A node that has not been configured yet has no cluster authority to be
	// signed by, so it signs its own certificate. That self-signature is the
	// whole of what this probe can read, and it is enough for the one
	// distinction the wizard turns on.
	if identity.Maintenance {
		found.State = StateMaintenance
		return found, true
	}

	// Anything else answered on the Talos API port with a certificate somebody
	// else vouched for. Whether that is a Talos node in a cluster or something
	// unrelated cannot be told apart without authenticating, and both lead to
	// the same instruction: do not aim a provisioning run at this address
	// without looking first.
	found.State = StateConfigured
	found.Detail = "Something at this address presented a certificate issued by a certificate " +
		"authority, which is what a machine that already has a configuration looks like. " +
		"Provisioning it would replace what is on it."
	return found, true
}

// expand turns a scan request into a list of addresses.
func expand(req ScanRequest) ([]string, error) {
	out := make([]string, 0, 256)
	out = append(out, req.Addrs...)

	if req.CIDR == "" {
		return out, nil
	}

	prefix, err := netip.ParsePrefix(req.CIDR)
	if err != nil {
		return nil, fmt.Errorf("provision: %q is not a subnet: %w", req.CIDR, err)
	}
	if !prefix.Addr().Is4() {
		return nil, fmt.Errorf("provision: %q is IPv6; scanning an IPv6 subnet is not a scan, "+
			"it is a lifetime. Name the addresses instead", req.CIDR)
	}
	// A /23 and not a /22, and the number is not a taste: 510 addresses at
	// ScanConcurrency with ScanTimeout each is 64 seconds of probing, which
	// fits inside the route budget the server can hold a response open for. A
	// /22 is twice that and does not, so offering it would be offering a scan
	// that reliably ends as a timeout with no result -- which reads to the
	// operator as "nothing is on my network".
	if ones := prefix.Bits(); ones < 23 {
		return nil, fmt.Errorf("provision: %q has %d addresses, which is more than one request "+
			"can scan before the server has to answer. Scan a /23 or smaller, or name the "+
			"addresses", req.CIDR, 1<<(32-ones))
	}

	// The network and broadcast addresses are skipped: neither is a host, and
	// probing them is two seconds spent learning nothing.
	addr := prefix.Masked().Addr().Next()
	for prefix.Contains(addr) {
		next := addr.Next()
		if !prefix.Contains(next) {
			break // the broadcast address
		}
		out = append(out, addr.String())
		addr = next
	}
	return out, nil
}

// sortByAddr orders results by IP where they parse, and lexically otherwise.
func sortByAddr(found []Found) {
	for i := 1; i < len(found); i++ {
		for j := i; j > 0 && less(found[j], found[j-1]); j-- {
			found[j], found[j-1] = found[j-1], found[j]
		}
	}
}

func less(a, b Found) bool {
	ai, aok := netip.ParseAddr(a.Addr)
	bi, bok := netip.ParseAddr(b.Addr)
	if aok == nil && bok == nil {
		return ai.Less(bi)
	}
	return a.Addr < b.Addr
}

// MaintenanceWarning is the sentence the wizard shows about trusting a machine
// in maintenance mode (PROV-04).
//
// It is a constant rather than UI copy because it is a statement about the
// protocol rather than about the screen, and softening it would be softening
// the only thing standing between an operator and a man-in-the-middle on their
// own LAN.
const MaintenanceWarning = "A machine in maintenance mode has no cluster PKI, so nothing here " +
	"has verified that it is the machine you think it is. The certificate fingerprint below is " +
	"what that machine presented; the only place it can be checked against is the machine's own " +
	"console. Pinning it means a second machine answering at this address cannot silently take " +
	"the configuration."

// The two warnings PROV-12 asks for at the start of the wizard.
//
// Both describe how the machine boots rather than anything holzkube-manager does, and
// both produce a failure that looks like something else: the first looks like
// "the ISO did not work", the second like "the machine never appeared".
const (
	// ShadowedISOWarning explains that an existing installation wins.
	ShadowedISOWarning = "If this machine already has Talos installed on a disk, it will boot from " +
		"the disk rather than from the ISO unless the boot order says otherwise. The machine then " +
		"comes up configured instead of in maintenance mode, and this wizard will not find it."

	// NoDHCPWarning explains that there is no zero-touch path without DHCP.
	NoDHCPWarning = "A machine that boots the ISO with no DHCP on its network has no address, so " +
		"nothing can reach it and there is no zero-touch path. It has to be given an address at " +
		"its own console first."
)

// hostPort is here so that the seam guard in internal/talos has one obvious
// place to look at rather than a format string somewhere in the middle of the
// scan loop -- and so that this package never builds one itself.
var _ = net.JoinHostPort
