package talos

// Pinning a maintenance node's certificate (PROV-04, T-02-27).
//
// A machine in maintenance mode has no cluster PKI. There is no authority to
// verify its certificate against, so the connection is made with
// InsecureSkipVerify and the *only* trust anchor available is the fingerprint
// the operator read off the machine's own console. Creds carries it.
//
// Until this file existed, nothing compared it. The wizard collected the
// fingerprint, the screen showed it, the composition root's comment said the
// seam performed the pin, and the seam's own comment said it did not -- so the
// one connection in this product with no PKI behind it verified nothing at all,
// and the call it exists to make is ApplyConfiguration: handing a machine its
// cluster's secrets. A man in the middle on that path is handed the cluster.
//
// The linter found the fossil rather than the hole: provision/plan.go carried a
// normalise() that trimmed and lower-cased a fingerprint for a comparison
// nobody had written.

import (
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/tlsx"
)

// ErrFingerprintPin reports a peer whose certificate is not the one the
// operator confirmed.
//
// It is separate from ErrFingerprintMismatch, which the talosconfig path uses
// for a value that disagrees before a connection is made. This one is a
// verification failure on a live handshake, and the two want different
// sentences on the screen: one is "you pasted the wrong value", the other is
// "the machine answering is not the machine you looked at".
var ErrFingerprintPin = errors.New("talos: the node presented a certificate the operator did not confirm")

// pin records what a pinned handshake saw.
//
// It exists because the error a rejected handshake produces does not survive
// the journey. gRPC turns a transport failure into a status, and classify then
// reads a status with no response trailers as KindUnreachable -- so a node that
// answered with the wrong certificate arrives at the operator as "the node
// could not be reached", which sends them to check a cable while somebody else
// is on the wire. The verifier therefore writes down what it saw, and the
// constructor reads it rather than trying to recover a sentence from a string.
type pin struct {
	mu        sync.Mutex
	mismatch  bool
	presented string
	expected  string
	machine   model.MachineID
}

// err is the failure this pin observed, or nil.
func (p *pin) err() error {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.mismatch {
		return nil
	}
	// The values are in the error on purpose. This is the one failure where
	// the operator has to compare two strings by eye against a machine's
	// console, and an error that only said "mismatch" would send them back to
	// the machine to read it again.
	return fmt.Errorf("%w: %s presented %s, expected %s",
		ErrFingerprintPin, p.machine, p.presented, p.expected)
}

// pinned returns a TLS configuration that refuses any peer whose leaf
// certificate does not match fingerprint, and the record of what it saw.
//
// It clones rather than mutates. The configuration belongs to the caller --
// the composition root builds one per request -- and installing a callback on
// somebody else's value is how one node's pin ends up applied to another's.
//
// VerifyPeerCertificate runs even when InsecureSkipVerify is set, which is
// exactly the combination this path needs: the chain cannot be verified and the
// leaf still has to be the right leaf.
func pinned(base *tls.Config, fingerprint string, machine model.MachineID) (*tls.Config, *pin, error) {
	want := normaliseFingerprint(fingerprint)
	if want == "" {
		return nil, nil, fmt.Errorf("talos: %s: empty fingerprint", machine)
	}

	p := &pin{machine: machine, expected: fingerprint}

	conf := base.Clone()

	// VerifyConnection and not VerifyPeerCertificate, and that distinction is
	// the difference between a pin and the appearance of one.
	//
	// VerifyPeerCertificate is skipped entirely on a *resumed* TLS session --
	// there is no certificate message to hand it, so Go does not call it. A pin
	// written that way holds on the first handshake and silently stops holding
	// on every resumption afterwards, which is the worst possible shape: it
	// passes every test that connects once. VerifyConnection runs on every
	// handshake, resumed or full, and sees the peer certificates the session
	// was established with.
	//
	// gosec's G123 is what found this, on code written minutes earlier.
	conf.VerifyConnection = func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			// Not reachable over TLS as Go implements it, and not treated as
			// an edge case either: a peer that presented nothing has not been
			// identified, and the only safe reading of "no certificate" on a
			// pinned connection is failure.
			p.record("no certificate")
			return p.err()
		}

		presented := tlsx.Fingerprint(cs.PeerCertificates[0].Raw)
		if normaliseFingerprint(presented) != want {
			p.record(presented)
			return p.err()
		}
		return nil
	}
	return conf, p, nil
}

func (p *pin) record(presented string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.mismatch = true
	p.presented = presented
}

// normaliseFingerprint makes two spellings of the same fingerprint comparable.
//
// Whitespace and case only -- never the separators. A value pasted out of a
// terminal arrives with a trailing newline and sometimes in the other case, and
// refusing it would teach the operator that the field is unreliable. Stripping
// the colons as well would make ab:cd equal to abcd, which is a different
// string and not obviously the same fingerprint; nothing in this product
// produces the second form.
func normaliseFingerprint(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
