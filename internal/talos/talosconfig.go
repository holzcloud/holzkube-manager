package talos

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"

	"github.com/holzcloud/holzkube-manager/internal/tlsx"
)

// The talosconfig an operator already has is how holzkube-manager first gets into a
// cluster that existed before it did (D-01).
//
// Parsing it lives inside the seam for the reason the seam exists: the file
// format is machinery's, the credentials it yields are a *tls.Config, and
// everything above this package deals in talos.Creds. It is also the one place
// that knows a talosconfig has a context name, endpoints and a client
// certificate with an expiry date -- three facts the adoption path needs and
// nothing else in the product should have to learn.

var (
	// ErrTalosconfigInvalid reports a file that is not a usable talosconfig.
	//
	// It is one error for "not YAML", "no context" and "no client certificate"
	// on purpose: all three mean the operator uploaded the wrong file, and
	// distinguishing them would tell an unauthenticated caller which of their
	// guesses was closer.
	ErrTalosconfigInvalid = errors.New("talos: not a usable talosconfig")

	// ErrFingerprintMismatch reports that the node presented a certificate
	// whose fingerprint is not the one the operator confirmed.
	ErrFingerprintMismatch = errors.New("talos: server certificate fingerprint does not match")
)

// Talosconfig is one context of an operator's talosconfig, in holzkube-manager's own
// shape.
type Talosconfig struct {
	// Context is the context name the credentials came from.
	Context string

	// Endpoints are the addresses the file lists, without ports. They are
	// hints for the first dial, exactly like Target.Addr: the operator names
	// the node to adopt through, and this is what the file suggests.
	Endpoints []string

	// NotAfter is when the client certificate stops working.
	//
	// Talos does not rotate client certificates, and this one typically has
	// about twelve months on it. That is why the date is carried out of the
	// file rather than left inside it: every node goes unreachable in the same
	// second when it passes, and an operator who is not warned reads that as
	// the cluster having died (P1/P5/P16, D-23).
	NotAfter time.Time

	ca  []byte
	crt []byte
	key []byte
}

// ParseTalosconfig reads a talosconfig's current context.
//
// The bytes arrive from an upload or a paste and never from a path on the
// server: a server-side path would be an arbitrary file read under holzkubed's
// uid, and store.go already states that any os.ReadFile outside fsstore is an
// architecture bug (D-06).
func ParseTalosconfig(raw []byte) (*Talosconfig, error) {
	cfg, err := clientconfig.FromBytes(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTalosconfigInvalid, err)
	}

	ctxName := cfg.Context
	if ctxName == "" {
		return nil, fmt.Errorf("%w: the file names no current context", ErrTalosconfigInvalid)
	}
	ctx, ok := cfg.Contexts[ctxName]
	if !ok || ctx == nil {
		return nil, fmt.Errorf("%w: context %q is not in the file", ErrTalosconfigInvalid, ctxName)
	}

	ca, err := decodeField(ctx.CA)
	if err != nil {
		return nil, fmt.Errorf("%w: certificate authority: %w", ErrTalosconfigInvalid, err)
	}
	crt, err := decodeField(ctx.Crt)
	if err != nil {
		return nil, fmt.Errorf("%w: client certificate: %w", ErrTalosconfigInvalid, err)
	}
	key, err := decodeField(ctx.Key)
	if err != nil {
		return nil, fmt.Errorf("%w: client key: %w", ErrTalosconfigInvalid, err)
	}
	if len(ca) == 0 || len(crt) == 0 || len(key) == 0 {
		return nil, fmt.Errorf("%w: context %q carries no client credentials, so it cannot reach a node",
			ErrTalosconfigInvalid, ctxName)
	}

	pair, err := tls.X509KeyPair(crt, key)
	if err != nil {
		return nil, fmt.Errorf("%w: client certificate and key do not match: %w", ErrTalosconfigInvalid, err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("%w: client certificate: %w", ErrTalosconfigInvalid, err)
	}

	return &Talosconfig{
		Context:   ctxName,
		Endpoints: ctx.Endpoints,
		NotAfter:  leaf.NotAfter,
		ca:        ca,
		crt:       crt,
		key:       key,
	}, nil
}

// Creds returns the cluster credentials this context authenticates with.
func (t *Talosconfig) Creds() (Creds, error) {
	return ClusterCreds(t.ca, t.crt, t.key)
}

// ClusterCreds builds full cluster mTLS credentials from PEM material.
//
// It is what both halves of the adoption path use: the talosconfig the
// operator supplied, and the certificate holzkube-manager mints for itself out of the
// derived bundle. One constructor means the second connection is made the same
// way as the first, so the connectivity proof in D-04 is a proof about the
// credentials and not about two different code paths.
//
// Server verification is never disabled here. CredMaintenance is the mode with
// no cluster CA, and it is a different kind for exactly this reason.
func ClusterCreds(caPEM, crtPEM, keyPEM []byte) (Creds, error) {
	pair, err := tls.X509KeyPair(crtPEM, keyPEM)
	if err != nil {
		return Creds{}, fmt.Errorf("talos: client certificate and key do not match: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return Creds{}, errors.New("talos: certificate authority is not a PEM certificate")
	}

	return Creds{
		Kind: CredCluster,
		TLS: &tls.Config{
			Certificates: []tls.Certificate{pair},
			RootCAs:      pool,
			MinVersion:   tls.VersionTLS12,
		},
	}, nil
}

// ServerFingerprint returns the SHA-256 fingerprint of the certificate a
// target presents, in the colon-separated upper-case hex form
// tlsx.Fingerprint produces and Creds.Fingerprint expects.
//
// It verifies nothing, and that is the whole point: it is the value shown to
// the operator *before* the first trusting connection, so that what they
// confirm is the certificate that will actually be used (D-03). Two
// fingerprint formats in one product would be a comparison nobody performs,
// which is why this borrows tlsx's rather than inventing one.
func ServerFingerprint(ctx context.Context, d Dialer, t Target) (string, error) {
	addr, err := d.Resolve(ctx, t)
	if err != nil {
		return "", err
	}

	// The probe class, the same budget D-05 gives the liveness check, and for
	// the same reason: this is one handshake against a node that either
	// answers promptly or is not there. An unbounded dial here would be the
	// one call in the adoption with no ceiling of its own, and the route's
	// budget would be the only thing between it and the response deadline.
	ctx, cancel := context.WithTimeout(ctx, ClassProbe.Deadline())
	defer cancel()

	dialer := &net.Dialer{}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
		// The connection exists to read a certificate nobody trusts yet.
		// Verification here would be circular: there is no authority to verify
		// against until the operator has confirmed this value.
		InsecureSkipVerify: true, //nolint:gosec // see above: this is the pre-trust probe
		MinVersion:         tls.VersionTLS12,
	})
	if err != nil {
		return "", classify(ctx, "Fingerprint", t.Machine, false, err)
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return "", fmt.Errorf("talos: %s presented no certificate", t.Machine)
	}
	return tlsx.Fingerprint(certs[0].Raw), nil
}

// decodeField accepts either the base64 form a talosconfig normally carries or
// raw PEM.
//
// Both appear in the wild: the file format base64-encodes these fields, and a
// config assembled by hand or by another tool sometimes does not. Refusing the
// second would be refusing a file that works perfectly well with talosctl.
func decodeField(v string) ([]byte, error) {
	if v == "" {
		return nil, nil
	}
	if strings.HasPrefix(v, "-----BEGIN ") {
		return []byte(v), nil
	}
	out, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return nil, err
	}
	return out, nil
}
