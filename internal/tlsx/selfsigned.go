// Package tlsx supplies the server certificate.
//
// HTTPS is the default, not an option: a session cookie that grants access to
// cluster PKI, sent in plaintext across a home LAN, is a real vulnerability, and
// Secure cookies need TLS anyway. On first run a long-lived self-signed
// certificate is generated and its SHA-256 fingerprint is logged, so the
// operator can accept the browser warning once and actually verify what they are
// accepting (D-04).
//
// There is no private certificate authority and nothing is installed into a
// system trust store. The generated certificate is a leaf and says so.
package tlsx

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/holzcloud/holzkube/internal/store"
)

const (
	certFile = "cert.pem"
	keyFile  = "key.pem"

	dirPerm  = 0o700
	filePerm = 0o600

	// validity is ten years. The operator compares this fingerprint by hand
	// once; asking them to redo it annually would teach them to skip the
	// comparison, which is the only thing the browser warning is worth.
	validity = 10 * 365 * 24 * time.Hour

	// clockSkew backdates the certificate, so that a browser whose clock trails
	// the server's does not reject a certificate that is not valid yet.
	clockSkew = time.Hour
)

// Generate writes a fresh self-signed certificate and key into dir and returns
// their paths together with the certificate's fingerprint.
//
// hostname is included as a subject alternative name alongside localhost and
// both loopback addresses, so that reaching the machine by its own name does not
// produce a second, different warning.
//
// extra names the further addresses this instance answers to -- in practice the
// host part of the bind address. An operator running --listen 192.168.1.5:8443
// otherwise got a SAN mismatch stacked on top of the self-signed warning, which
// is the second, differently-worded warning the D-04 rationale exists to avoid.
func Generate(dir, hostname string, extra ...string) (certPath, keyPath, fingerprint string, err error) {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return "", "", "", fmt.Errorf("tlsx: create directory %s: %w", dir, err)
	}

	certPEM, keyPEM, der, err := generate(hostname, extra...)
	if err != nil {
		return "", "", "", err
	}

	certPath = filepath.Join(dir, certFile)
	keyPath = filepath.Join(dir, keyFile)

	// The key goes down first: a cert.pem without its key is a confusing state
	// for the next start to find, and this order cannot produce it.
	if err := writeAtomic(keyPath, keyPEM); err != nil {
		return "", "", "", fmt.Errorf("tlsx: write %s: %w", keyPath, err)
	}
	if err := writeAtomic(certPath, certPEM); err != nil {
		return "", "", "", fmt.Errorf("tlsx: write %s: %w", certPath, err)
	}
	return certPath, keyPath, Fingerprint(der), nil
}

// Fingerprint renders the SHA-256 digest of a DER certificate as colon-separated
// upper-case hex pairs -- the format browsers use in their certificate dialog.
//
// The format matters. The operator is asked to compare this string against what
// the browser shows; a comparison that requires converting one side first is a
// comparison that does not happen (T-01-39).
func Fingerprint(der []byte) string {
	const digits = "0123456789ABCDEF"
	sum := sha256.Sum256(der)

	var b strings.Builder
	b.Grow(len(sum)*3 - 1)
	for i, c := range sum {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteByte(digits[c>>4])
		b.WriteByte(digits[c&0x0f])
	}
	return b.String()
}

func generate(hostname string, extra ...string) (certPEM, keyPEM, der []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("tlsx: generate key: %w", err)
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("tlsx: generate serial: %w", err)
	}

	names := dnsNames(hostname)
	ips := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
	for _, e := range extra {
		names, ips = addSAN(names, ips, e)
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "holzkube", Organization: []string{"holzkube"}},
		NotBefore:    now.Add(-clockSkew),
		NotAfter:     now.Add(validity),

		// A leaf, explicitly. Signing is for its own handshake and nothing else:
		// a self-signed certificate that also claims to be a CA is a certificate
		// that could mint others if its key ever escaped (D-04).
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,

		DNSNames:    names,
		IPAddresses: ips,
	}

	der, err = x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("tlsx: create certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("tlsx: marshal key: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, der, nil
}

// addSAN files one more address under whichever SAN list it belongs in,
// skipping anything already covered. A bind address of 0.0.0.0 or :: is a
// wildcard rather than an address to be reached at, so it is not a SAN.
func addSAN(names []string, ips []net.IP, host string) ([]string, []net.IP) {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" {
		return names, ips
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsUnspecified() {
			return names, ips
		}
		if slices.ContainsFunc(ips, ip.Equal) {
			return names, ips
		}
		return names, append(ips, ip)
	}
	if slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, host) }) {
		return names, ips
	}
	return append(names, host), ips
}

func dnsNames(hostname string) []string {
	names := []string{"localhost"}
	if hostname == "" || hostname == "localhost" {
		return names
	}
	names = append(names, hostname)
	// macOS reports a bare hostname while Bonjour resolves it with a .local
	// suffix; include both so the SAN matches whichever the browser used.
	if !strings.Contains(hostname, ".") {
		names = append(names, hostname+".local")
	}
	return names
}

// writeAtomic writes data so that neither a reader nor a crash observes a
// partial file, using the same sequence as the store:
//
//	tmp in the same directory -> chmod 0600 -> write -> fsync(file)
//	-> rename -> fsync(dir)
//
// A truncated private key would be a start-refusing condition on the next boot
// with no obvious cause, which is worth these six lines.
func writeAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)

	// The store's prefix, not a private one. A crash between CreateTemp and
	// Rename leaves this file behind holding a PEM-encoded EC private key. It
	// is 0600 so the permission Guard passes it, and under a prefix of its own
	// neither the startup sweeper nor the backup exclusion recognised it: the
	// orphans accumulated across restarts and every pre-migration tarball
	// captured them alongside the live key.
	tmp, err := os.CreateTemp(dir, store.TempFilePrefix+"tlsx-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err = tmp.Chmod(filePerm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("fsync temp file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename into place: %w", err)
	}

	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open directory for fsync: %w", err)
	}
	defer d.Close()
	if err = d.Sync(); err != nil {
		return fmt.Errorf("fsync directory: %w", err)
	}
	return nil
}
