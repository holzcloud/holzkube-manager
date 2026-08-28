package tlsx

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/holzcloud/holzkube/internal/config"
)

// Ensure returns the TLS configuration the server should serve with, and the
// fingerprint to log.
//
// There are exactly three paths and one of them is deliberately missing: a
// supplied certificate that cannot be used is a hard failure, never a quiet fall
// back to self-generation. The operator who passed --tls-cert would otherwise
// believe their certificate is in force while the server presents a different
// one, and nothing in the log would say so (T-01-40).
func Ensure(cfg config.Config) (*tls.Config, string, error) {
	switch {
	case cfg.TLSCert != "" && cfg.TLSKey == "":
		return nil, "", errors.New("tlsx: --tls-cert was given without --tls-key; supply both or neither")
	case cfg.TLSKey != "" && cfg.TLSCert == "":
		return nil, "", errors.New("tlsx: --tls-key was given without --tls-cert; supply both or neither")
	case cfg.TLSCert != "":
		cert, fingerprint, err := Load(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return nil, "", err
		}
		return configFor(cert), fingerprint, nil
	}

	certPath := filepath.Join(cfg.DataDir, certFile)
	keyPath := filepath.Join(cfg.DataDir, keyFile)
	certExists, err := exists(certPath)
	if err != nil {
		return nil, "", err
	}
	keyExists, err := exists(keyPath)
	if err != nil {
		return nil, "", err
	}

	switch {
	case certExists && keyExists:
		// Reuse. The fingerprint the operator accepted in their browser has to
		// survive a restart, or they learn to click the warning away unread.
		cert, fingerprint, err := Load(certPath, keyPath)
		if err != nil {
			return nil, "", err
		}
		return configFor(cert), fingerprint, nil

	case certExists != keyExists:
		// Half a generated pair means something went wrong that regenerating
		// would erase along with the surviving half.
		return nil, "", fmt.Errorf(
			"tlsx: %s and %s: exactly one of the generated pair exists; "+
				"remove both to generate a new certificate, or pass --tls-cert and --tls-key",
			certPath, keyPath)
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}
	certPath, keyPath, _, err = Generate(cfg.DataDir, hostname, ListenHost(cfg.Listen))
	if err != nil {
		return nil, "", err
	}
	cert, fingerprint, err := Load(certPath, keyPath)
	if err != nil {
		return nil, "", err
	}
	return configFor(cert), fingerprint, nil
}

// LoopbackGuard refuses plaintext HTTP anywhere it could leave the machine.
//
// The restriction is structural rather than advisory: --insecure-http exists for
// a developer proxying to a local server, and a session cookie that grants
// access to cluster PKI must not cross a home network in the clear (D-04,
// T-01-38). It runs before the server is set up, so the refusal is a start
// failure and not a request-time surprise.
func LoopbackGuard(listen string, insecure bool) error {
	if !insecure {
		return nil
	}
	loopback, err := config.IsLoopback(listen)
	if err != nil {
		return fmt.Errorf("tlsx: --insecure-http: %w", err)
	}
	if loopback {
		return nil
	}
	return fmt.Errorf(
		"tlsx: refusing --insecure-http while listening on %s: "+
			"the session cookie grants access to cluster PKI and would cross the network in plaintext; "+
			"plain HTTP is available only when the listener cannot leave this machine",
		listen)
}

// Load reads a certificate and key pair. A pair that does not load -- unreadable,
// malformed, or a key that does not belong to the certificate -- is an error and
// never a reason to generate one instead.
func Load(certPath, keyPath string) (tls.Certificate, string, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, "", fmt.Errorf("tlsx: load key pair %s + %s: %w", certPath, keyPath, err)
	}
	if len(cert.Certificate) == 0 {
		return tls.Certificate{}, "", fmt.Errorf("tlsx: %s contains no certificate", certPath)
	}
	return cert, Fingerprint(cert.Certificate[0]), nil
}

func configFor(cert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
}

func exists(path string) (bool, error) {
	switch _, err := os.Stat(path); {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("tlsx: inspect %s: %w", path, err)
	}
}

// ListenHost returns the host part of a bind address, or "" if there is none
// to speak of.
//
// It is exported because the same value is both a certificate SAN and an entry
// in the HTTP Host allowlist, and the two must agree: a host the certificate
// vouches for but the server refuses, or the reverse, is a confusing failure
// either way.
func ListenHost(listen string) string {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		host = listen
	}
	return strings.Trim(strings.TrimSpace(host), "[]")
}
