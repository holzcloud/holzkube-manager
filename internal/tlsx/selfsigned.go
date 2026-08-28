// Package tlsx supplies the server certificate.
//
// HTTPS is the default, not an option: a session cookie that grants access to
// cluster PKI, sent in plaintext across a home LAN, is a real vulnerability,
// and Secure cookies need TLS anyway. On first run a long-lived self-signed
// certificate is generated and its SHA-256 fingerprint is logged, so the
// operator can accept the browser warning once and actually verify what they
// are accepting (D-04).
package tlsx

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	certFile = "cert.pem"
	keyFile  = "key.pem"

	dirPerm  = 0o700
	filePerm = 0o600

	validity = 10 * 365 * 24 * time.Hour
)

// EnsureCertificate loads <dir>/cert.pem and <dir>/key.pem, generating a
// self-signed pair on first run. It returns the certificate and the SHA-256
// fingerprint of the leaf, formatted as lowercase hex.
func EnsureCertificate(dir string) (tls.Certificate, string, error) {
	certPath := filepath.Join(dir, certFile)
	keyPath := filepath.Join(dir, keyFile)

	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)
	if certErr == nil && keyErr == nil {
		return Load(certPath, keyPath)
	}

	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return tls.Certificate{}, "", fmt.Errorf("tlsx: create directory: %w", err)
	}
	certPEM, keyPEM, err := generate()
	if err != nil {
		return tls.Certificate{}, "", err
	}
	if err := os.WriteFile(certPath, certPEM, filePerm); err != nil {
		return tls.Certificate{}, "", fmt.Errorf("tlsx: write certificate: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, filePerm); err != nil {
		return tls.Certificate{}, "", fmt.Errorf("tlsx: write key: %w", err)
	}
	return Load(certPath, keyPath)
}

// Load reads an operator-supplied certificate and key pair.
func Load(certPath, keyPath string) (tls.Certificate, string, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, "", fmt.Errorf("tlsx: load key pair: %w", err)
	}
	if len(cert.Certificate) == 0 {
		return tls.Certificate{}, "", fmt.Errorf("tlsx: %s contains no certificate", certPath)
	}
	return cert, Fingerprint(cert.Certificate[0]), nil
}

// Fingerprint returns the SHA-256 fingerprint of a DER certificate, which is
// exactly what a browser shows in its certificate viewer.
func Fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

func generate() (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("tlsx: generate key: %w", err)
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("tlsx: generate serial: %w", err)
	}

	dnsNames := []string{"localhost"}
	if host, err := os.Hostname(); err == nil && host != "" && host != "localhost" {
		dnsNames = append(dnsNames, host)
		// macOS reports a hostname without the .local suffix that Bonjour
		// actually resolves; include both so the SAN matches either way.
		if !strings.Contains(host, ".") {
			dnsNames = append(dnsNames, host+".local")
		}
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "holzkube", Organization: []string{"holzkube"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(validity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              dnsNames,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("tlsx: create certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("tlsx: marshal key: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}
