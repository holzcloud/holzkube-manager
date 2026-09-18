package talossim

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"
)

// ServerName is the name the simulated node's certificate is issued for, and
// the name a client verifies against.
//
// It is pinned rather than derived from the listen address because the same
// server answers on two listeners -- a real loopback TCP socket and an
// in-process one -- and the point of the exercise is that a client above the
// seam cannot tell which it reached. A ServerName derived from the address
// would differ between the two and make the transports distinguishable for the
// wrong reason.
const ServerName = "talossim"

// certValidity is short on purpose: this material never leaves the process and
// never outlives the test that created it.
const certValidity = time.Hour

// pki is one self-contained certificate authority plus the two leaves the
// simulated node needs. It exists inside talossim rather than in internal/tlsx
// because tlsx says in its package documentation that it has no certificate
// authority and installs nothing into a trust store, and that sentence has to
// stay true.
type pki struct {
	// mu guards everything a CA rotation replaces. The listener reads this on
	// every handshake, so a rotation mid-test is a write while reads are in
	// flight -- which is what the race detector in CI exists to catch.
	mu sync.RWMutex

	caCert *x509.Certificate
	caKey  crypto.Signer

	// accepted are the additional authorities a client certificate may come
	// from: machine.acceptedCAs of the node's active configuration. Talos
	// accepts machine.ca implicitly, which is why caCert is not in here.
	accepted []*x509.Certificate

	// names are what a re-issued server certificate has to carry, kept so a
	// rotation mints the same node rather than a differently named one.
	hostname string
	nodeIP   string

	server tls.Certificate
	client tls.Certificate

	// maintenance records that this node is serving the pre-configuration
	// world: a self-signed certificate and no client certificate asked for.
	// See newPKI.
	maintenance bool
}

// newPKI builds the node's certificate material.
//
// osCA, when supplied, is the cluster's own Talos OS certificate authority --
// the one that appears in the machine configuration this node serves. Using it
// rather than a fresh authority is what makes the adoption path testable end
// to end: holzkube-manager derives the bundle from the configuration it read, mints
// itself a client certificate from that CA, and reconnects. If the node
// verified against some other authority, that second connection would fail for
// a reason that has nothing to do with whether the derivation was correct --
// and the connectivity proof D-04 asks for would be untestable.
//
// maintenance makes the node present what an unconfigured machine presents: a
// **self-signed** server certificate, and no request for a client certificate.
// That is not a detail of the fake. A node in maintenance mode has no cluster
// PKI at all, so it has nothing to be signed by and nothing to verify a client
// against -- and the self-signature is the single observable a scan can read
// without authenticating, which is how Dialer.Probe decides whether a machine
// is waiting for a configuration. A simulator that announced itself as being
// in maintenance mode while serving a CA-issued certificate would let a test of
// the provisioning path pass against a node no real machine resembles.
func newPKI(hostname, nodeIP string, osCA *pemPair, maintenance bool) (*pki, error) {
	caCert, caKey, err := authority(osCA)
	if err != nil {
		return nil, err
	}

	ips := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
	if ip := net.ParseIP(nodeIP); ip != nil {
		ips = append(ips, ip)
	}

	serverKey, serverDER, err := issue(caCert, caKey, &x509.Certificate{
		Subject:     pkix.Name{CommonName: ServerName, Organization: []string{"talossim"}},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    dedupe(ServerName, hostname),
		IPAddresses: ips,
	})
	if err != nil {
		return nil, fmt.Errorf("talossim: node certificate: %w", err)
	}

	clientKey, clientDER, err := issue(caCert, caKey, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "holzkube-manager", Organization: []string{"talossim"}},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		return nil, fmt.Errorf("talossim: client certificate: %w", err)
	}

	p := &pki{
		caCert:      caCert,
		caKey:       caKey,
		hostname:    hostname,
		nodeIP:      nodeIP,
		server:      tls.Certificate{Certificate: [][]byte{serverDER}, PrivateKey: serverKey, Leaf: mustLeaf(serverDER)},
		client:      tls.Certificate{Certificate: [][]byte{clientDER}, PrivateKey: clientKey, Leaf: mustLeaf(clientDER)},
		maintenance: maintenance,
	}
	if !maintenance {
		return p, nil
	}

	// Self-signed, and issued for the same names, so that everything about the
	// node except who vouched for it is unchanged.
	selfKey, selfDER, err := issue(nil, nil, &x509.Certificate{
		Subject:     pkix.Name{CommonName: ServerName, Organization: []string{"talossim"}},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    dedupe(ServerName, hostname),
		IPAddresses: ips,
	})
	if err != nil {
		return nil, fmt.Errorf("talossim: maintenance-mode node certificate: %w", err)
	}
	p.server = tls.Certificate{Certificate: [][]byte{selfDER}, PrivateKey: selfKey, Leaf: mustLeaf(selfDER)}
	return p, nil
}

// pemPair is a PEM-encoded certificate and its private key. It is talossim's
// own two-field shape rather than machinery's x509 type so that this file
// stays a file about certificates and not about Talos configuration.
type pemPair struct {
	Crt []byte
	Key []byte
}

// authority returns the certificate authority the node issues from: the
// cluster's, when one was supplied, and a fresh self-signed one otherwise.
func authority(osCA *pemPair) (*x509.Certificate, crypto.Signer, error) {
	if osCA == nil {
		key, der, err := issue(nil, nil, &x509.Certificate{
			Subject:               pkix.Name{CommonName: "talossim CA", Organization: []string{"talossim"}},
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
			BasicConstraintsValid: true,
			IsCA:                  true,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("talossim: certificate authority: %w", err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, nil, fmt.Errorf("talossim: parse certificate authority: %w", err)
		}
		return cert, key, nil
	}

	pair, err := tls.X509KeyPair(osCA.Crt, osCA.Key)
	if err != nil {
		return nil, nil, fmt.Errorf("talossim: load cluster certificate authority: %w", err)
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, nil, fmt.Errorf("talossim: parse cluster certificate authority: %w", err)
	}
	signer, ok := pair.PrivateKey.(crypto.Signer)
	if !ok {
		return nil, nil, fmt.Errorf("talossim: cluster authority key of type %T cannot sign", pair.PrivateKey)
	}
	return cert, signer, nil
}

// caPEM returns the authority's certificate in PEM form, which is what a
// client needs in order to trust this node.
func (p *pki) caPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: p.caCert.Raw})
}

// issue fills in the parts of a template that are the same for every
// certificate here -- key, serial, validity window -- and signs it. A nil
// parent self-signs, which is how the authority is made.
//
// The key, serial and validity construction follows internal/tlsx/selfsigned.go
// so that there is one shape of certificate generation in this repository.
func issue(parent *x509.Certificate, parentKey crypto.Signer, tmpl *x509.Certificate) (*ecdsa.PrivateKey, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial: %w", err)
	}

	now := time.Now()
	tmpl.SerialNumber = serial
	tmpl.NotBefore = now.Add(-time.Minute)
	tmpl.NotAfter = now.Add(certValidity)

	signer := crypto.Signer(parentKey)
	issuer := parent
	if parentKey == nil {
		signer = key
		issuer = tmpl
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, issuer, &key.PublicKey, signer)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}
	return key, der, nil
}

func mustLeaf(der []byte) *x509.Certificate {
	// The DER was produced by x509.CreateCertificate three lines earlier, so a
	// parse failure here is not an environment problem but a broken toolchain,
	// and there is nothing a caller could do with the error.
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		panic("talossim: a certificate this package just created does not parse: " + err.Error())
	}
	return cert
}

func dedupe(values ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// certificateFromPEM parses one PEM certificate, and refuses anything else.
func certificateFromPEM(raw []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("not a PEM document")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	return cert, nil
}

// pool returns a certificate pool trusting this authority and every authority
// the active configuration additionally accepts.
func (p *pki) pool() *x509.CertPool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.poolLocked()
}

func (p *pki) poolLocked() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(p.caCert)
	for _, c := range p.accepted {
		pool.AddCert(c)
	}
	return pool
}

// rotate makes the node serve, and verify against, the authorities its newly
// applied configuration names.
//
// This is what lets a CA rotation be measured here at all. Without it the
// simulator keeps serving a certificate from the authority it was built with
// and keeps verifying clients against it, so pass 2 would "succeed" and the
// node would go on accepting exactly what it accepted before -- the simulator
// passing a rotation a real node would not (TRANS-06), and the rotation's own
// proof step would fail against a node that never rotated.
//
// issuingKey may be empty: that is a worker, which carries the authority's
// certificate and not its key. Such a node cannot mint itself a new server
// certificate, so it keeps the one it has and only its accepted set moves --
// which is what a real worker does, since trustd on the control plane issues
// its certificates.
func (p *pki) rotate(issuingCrt, issuingKey []byte, accepted [][]byte) error {
	if len(issuingCrt) == 0 {
		return nil
	}

	caCert, err := certificateFromPEM(issuingCrt)
	if err != nil {
		return fmt.Errorf("talossim: the applied configuration's machine.ca: %w", err)
	}

	var extra []*x509.Certificate
	for _, raw := range accepted {
		c, err := certificateFromPEM(raw)
		if err != nil {
			return fmt.Errorf("talossim: the applied configuration's machine.acceptedCAs: %w", err)
		}
		extra = append(extra, c)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.accepted = extra

	if len(issuingKey) == 0 || caCert.Equal(p.caCert) {
		// Nothing to re-issue: either this node cannot (no key) or the issuer
		// has not moved.
		if len(issuingKey) == 0 {
			return nil
		}
	}

	_, caKey, err := authority(&pemPair{Crt: issuingCrt, Key: issuingKey})
	if err != nil {
		return err
	}

	ips := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
	if ip := net.ParseIP(p.nodeIP); ip != nil {
		ips = append(ips, ip)
	}
	serverKey, serverDER, err := issue(caCert, caKey, &x509.Certificate{
		Subject:     pkix.Name{CommonName: ServerName, Organization: []string{"talossim"}},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    dedupe(ServerName, p.hostname),
		IPAddresses: ips,
	})
	if err != nil {
		return fmt.Errorf("talossim: re-issuing the node certificate after a rotation: %w", err)
	}

	p.caCert, p.caKey = caCert, caKey
	p.server = tls.Certificate{
		Certificate: [][]byte{serverDER}, PrivateKey: serverKey, Leaf: mustLeaf(serverDER),
	}
	return nil
}

// serverTLS is the listener configuration: real mTLS.
//
// RequireAndVerifyClientCert with an explicit ClientCAs pool is what makes the
// simulator worth having. A fake that accepted any client certificate would let
// a test claiming to prove mTLS pass against a server that ignores client
// certificates entirely (T-02-06).
// It reads the current material on every handshake rather than once at
// startup, because a CA rotation replaces it while the listener is up: a
// configuration captured at grpc.Creds time would make the node keep
// presenting the authority it was built with, for ever.
func (p *pki) serverTLS() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			p.mu.RLock()
			defer p.mu.RUnlock()
			return p.currentServerTLSLocked(), nil
		},
	}
}

func (p *pki) currentServerTLSLocked() *tls.Config {
	cfg := &tls.Config{
		Certificates: []tls.Certificate{p.server},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    p.poolLocked(),
		MinVersion:   tls.VersionTLS12,
	}
	if p.maintenance {
		// A node with no cluster PKI has no authority to verify a client
		// against, so it asks for nothing. This is the reason maintenance mode
		// serves the method set it does rather than the full one (D-06): the
		// connection is unauthenticated in both directions.
		cfg.ClientAuth = tls.NoClientCert
		cfg.ClientCAs = nil
	}
	return cfg
}

// clientTLS is the configuration a client uses to reach this node: it trusts
// the node's authority and presents a certificate that authority issued.
func (p *pki) clientTLS() *tls.Config {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return &tls.Config{
		Certificates: []tls.Certificate{p.client},
		RootCAs:      p.pool(),
		ServerName:   ServerName,
		MinVersion:   tls.VersionTLS12,
	}
}
