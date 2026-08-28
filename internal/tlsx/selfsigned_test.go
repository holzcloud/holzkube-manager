package tlsx

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"testing"
	"time"
)

func parseCert(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // test reads the file it just wrote
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		t.Fatalf("%s is not PEM", path)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return cert
}

// D-04: SANs for both loopback addresses, localhost and the machine's hostname,
// long-lived, and no certificate authority -- there is no private CA and nothing
// is imported into a trust store.
func TestGenerateWritesALoopbackCertificate(t *testing.T) {
	dir := t.TempDir()

	certPath, keyPath, fingerprint, err := Generate(dir, "homeserver")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if certPath != filepath.Join(dir, "cert.pem") || keyPath != filepath.Join(dir, "key.pem") {
		t.Fatalf("Generate wrote %s and %s", certPath, keyPath)
	}

	cert := parseCert(t, certPath)

	for _, want := range []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback} {
		if !slices.ContainsFunc(cert.IPAddresses, func(ip net.IP) bool { return ip.Equal(want) }) {
			t.Errorf("certificate has no SAN for %s (has %v)", want, cert.IPAddresses)
		}
	}
	for _, want := range []string{"localhost", "homeserver"} {
		if !slices.Contains(cert.DNSNames, want) {
			t.Errorf("certificate has no DNS SAN for %q (has %v)", want, cert.DNSNames)
		}
	}

	if cert.IsCA {
		t.Error("the certificate is marked as a CA; D-04 says there is no private CA")
	}
	if !cert.BasicConstraintsValid {
		t.Error("basic constraints are not marked valid, so IsCA=false is not asserted")
	}
	if cert.KeyUsage&x509.KeyUsageCertSign != 0 {
		t.Error("the certificate may sign other certificates")
	}
	if !slices.Contains(cert.ExtKeyUsage, x509.ExtKeyUsageServerAuth) {
		t.Errorf("extended key usage %v does not include server authentication", cert.ExtKeyUsage)
	}

	if years := cert.NotAfter.Sub(cert.NotBefore).Hours() / 24 / 365; years < 9 {
		t.Errorf("validity is %.1f years; the operator should not re-accept a fingerprint yearly", years)
	}

	if got := Fingerprint(cert.Raw); got != fingerprint {
		t.Errorf("Generate returned fingerprint %q, the certificate hashes to %q", fingerprint, got)
	}
}

func TestGenerateWritesBothFilesWith0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on windows")
	}
	dir := t.TempDir()

	certPath, keyPath, _, err := Generate(dir, "homeserver")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, path := range []string{certPath, keyPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != filePerm {
			t.Errorf("%s has mode %04o, want %04o", path, got, filePerm)
		}
	}
}

// The private key is written through the same tmp -> fsync -> rename sequence as
// the store, so a crash mid-write cannot leave a truncated key that the next
// start would refuse to load.
func TestGenerateLeavesNoTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	if _, _, _, err := Generate(dir, "homeserver"); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"cert.pem", "key.pem"}) {
		t.Fatalf("directory contains %v, want exactly cert.pem and key.pem", names)
	}
}

// The operator compares this string against what the browser shows in its
// certificate dialog. Browsers print colon-separated upper-case hex pairs, so
// that is what goes in the log -- a comparison that needs a conversion step
// first is a comparison that does not happen (D-04, T-01-39).
func TestFingerprintIsInBrowserFormat(t *testing.T) {
	der := []byte("not really a certificate, but bytes are bytes")

	got := Fingerprint(der)

	if !regexp.MustCompile(`^([0-9A-F]{2}:){31}[0-9A-F]{2}$`).MatchString(got) {
		t.Fatalf("fingerprint %q is not 32 colon-separated upper-case hex pairs", got)
	}

	sum := sha256.Sum256(der)
	for i, b := range sum {
		pair := got[i*3 : i*3+2]
		if want := hexPair(b); pair != want {
			t.Fatalf("byte %d rendered as %q, want %q", i, pair, want)
		}
	}
}

func hexPair(b byte) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{digits[b>>4], digits[b&0x0f]})
}

func TestGenerateIsCalledWithAUsableClock(t *testing.T) {
	dir := t.TempDir()
	certPath, _, _, err := Generate(dir, "homeserver")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	cert := parseCert(t, certPath)

	// Backdated slightly: a server whose clock is a few minutes ahead of the
	// browser's must not hand out a certificate that is not valid yet.
	if !cert.NotBefore.Before(time.Now()) {
		t.Errorf("NotBefore is %s, which is not in the past", cert.NotBefore)
	}
}

// TestGenerateCoversTheBindAddress is the usability half of D-04: an operator
// running --listen 192.168.1.5:8443 used to get a SAN mismatch stacked on top
// of the self-signed warning, which is a second, differently-worded warning the
// rationale exists to avoid.
func TestGenerateCoversTheBindAddress(t *testing.T) {
	for _, tc := range []struct {
		name    string
		bind    string
		wantIP  string
		wantDNS string
	}{
		{name: "a LAN address", bind: "192.168.1.5", wantIP: "192.168.1.5"},
		{name: "a name", bind: "homeserver.lan", wantDNS: "homeserver.lan"},
		{name: "a wildcard is not an address to be reached at", bind: "0.0.0.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			certPath, _, _, err := Generate(dir, "homeserver", tc.bind)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			cert := parseCert(t, certPath)

			// The defaults survive in every case.
			if !slices.Contains(cert.DNSNames, "localhost") {
				t.Errorf("DNSNames = %v, want localhost retained", cert.DNSNames)
			}
			if !slices.ContainsFunc(cert.IPAddresses, func(ip net.IP) bool {
				return ip.Equal(net.IPv4(127, 0, 0, 1))
			}) {
				t.Errorf("IPAddresses = %v, want the loopback retained", cert.IPAddresses)
			}

			if tc.wantIP != "" {
				want := net.ParseIP(tc.wantIP)
				if !slices.ContainsFunc(cert.IPAddresses, func(ip net.IP) bool { return ip.Equal(want) }) {
					t.Errorf("IPAddresses = %v, want %s", cert.IPAddresses, tc.wantIP)
				}
			}
			if tc.wantDNS != "" && !slices.Contains(cert.DNSNames, tc.wantDNS) {
				t.Errorf("DNSNames = %v, want %s", cert.DNSNames, tc.wantDNS)
			}
			if tc.bind == "0.0.0.0" {
				if slices.ContainsFunc(cert.IPAddresses, func(ip net.IP) bool { return ip.IsUnspecified() }) {
					t.Errorf("IPAddresses = %v, want no wildcard SAN", cert.IPAddresses)
				}
			}
		})
	}
}
