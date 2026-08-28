package tlsx

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube/internal/config"
)

func mustGenerate(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	certPath, keyPath, _, err := Generate(t.TempDir(), "homeserver")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return certPath, keyPath
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // test reads a file it created
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// The fingerprint the operator accepted in their browser has to survive a
// restart. Regenerating on every start would train them to click through the
// warning without looking, which is the whole defence (T-01-39).
func TestEnsureGeneratesOnceAndReusesAfterwards(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir}

	first, fp1, err := Ensure(cfg)
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	if len(first.Certificates) != 1 {
		t.Fatalf("tls.Config carries %d certificates", len(first.Certificates))
	}
	if first.MinVersion < tls.VersionTLS12 {
		t.Errorf("MinVersion is %x, want at least TLS 1.2", first.MinVersion)
	}

	certBytes := read(t, filepath.Join(dir, "cert.pem"))
	keyBytes := read(t, filepath.Join(dir, "key.pem"))

	_, fp2, err := Ensure(cfg)
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if fp1 != fp2 {
		t.Errorf("fingerprint changed across a restart: %q then %q", fp1, fp2)
	}
	if string(read(t, filepath.Join(dir, "cert.pem"))) != string(certBytes) {
		t.Error("cert.pem was rewritten on the second start")
	}
	if string(read(t, filepath.Join(dir, "key.pem"))) != string(keyBytes) {
		t.Error("key.pem was rewritten on the second start")
	}
}

func TestEnsureUsesASuppliedPairAndGeneratesNothing(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := mustGenerate(t)

	cfg := config.Config{DataDir: dir, TLSCert: certPath, TLSKey: keyPath}
	got, fingerprint, err := Ensure(cfg)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if len(got.Certificates) != 1 {
		t.Fatalf("tls.Config carries %d certificates", len(got.Certificates))
	}
	if _, err := os.Stat(filepath.Join(dir, "cert.pem")); !os.IsNotExist(err) {
		t.Error("a certificate was generated into the data directory although one was supplied")
	}

	supplied := parseCert(t, certPath)
	if want := Fingerprint(supplied.Raw); fingerprint != want {
		t.Errorf("fingerprint %q, want the supplied certificate's %q", fingerprint, want)
	}
}

// A supplied pair that does not work is a hard failure. Falling back to
// self-generation would leave the operator believing their certificate is in
// force while the server presents a different one (T-01-40).
func TestEnsureRefusesABrokenSuppliedPairWithoutFallingBack(t *testing.T) {
	certA, _ := mustGenerate(t)
	_, keyB := mustGenerate(t)

	cases := []struct {
		name     string
		cert     string
		key      string
		contains string
	}{
		{name: "mismatched pair", cert: certA, key: keyB},
		{name: "missing certificate file", cert: filepath.Join(t.TempDir(), "absent.pem"), key: keyB, contains: "absent.pem"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			_, _, err := Ensure(config.Config{DataDir: dir, TLSCert: tc.cert, TLSKey: tc.key})
			if err == nil {
				t.Fatal("Ensure accepted a broken pair")
			}
			if tc.contains != "" && !strings.Contains(err.Error(), tc.contains) {
				t.Errorf("error %q does not name %q", err, tc.contains)
			}
			if _, err := os.Stat(filepath.Join(dir, "cert.pem")); !os.IsNotExist(err) {
				t.Error("Ensure fell back to generating a certificate")
			}
		})
	}
}

func TestEnsureRefusesHalfASuppliedPair(t *testing.T) {
	certPath, keyPath := mustGenerate(t)

	_, _, err := Ensure(config.Config{DataDir: t.TempDir(), TLSCert: certPath})
	if err == nil || !strings.Contains(err.Error(), "tls-key") {
		t.Errorf("--tls-cert alone gave %v, want an error naming --tls-key", err)
	}

	_, _, err = Ensure(config.Config{DataDir: t.TempDir(), TLSKey: keyPath})
	if err == nil || !strings.Contains(err.Error(), "tls-cert") {
		t.Errorf("--tls-key alone gave %v, want an error naming --tls-cert", err)
	}
}

// A session cookie that grants access to cluster PKI must not cross a home
// network in plaintext. The guard is structural rather than advisory: plain HTTP
// is only reachable when the listener cannot leave the machine (D-04, T-01-38).
func TestLoopbackGuard(t *testing.T) {
	allowed := []string{"127.0.0.1:8443", "127.0.0.53:8443", "[::1]:8443", "localhost:8443", "LOCALHOST:8443"}
	refused := []string{"0.0.0.0:8443", ":8443", "[::]:8443", "192.168.1.10:8443", "homeserver.local:8443"}

	for _, listen := range append(append([]string{}, allowed...), refused...) {
		if err := LoopbackGuard(listen, false); err != nil {
			t.Errorf("LoopbackGuard(%q, insecure=false) = %v, want nil: TLS is not restricted to loopback", listen, err)
		}
	}

	for _, listen := range allowed {
		if err := LoopbackGuard(listen, true); err != nil {
			t.Errorf("LoopbackGuard(%q, insecure=true) = %v, want nil", listen, err)
		}
	}

	for _, listen := range refused {
		err := LoopbackGuard(listen, true)
		if err == nil {
			t.Errorf("LoopbackGuard(%q, insecure=true) allowed plaintext off loopback", listen)
			continue
		}
		if !strings.Contains(err.Error(), listen) {
			t.Errorf("error %q does not name the bind address %q", err, listen)
		}
		if !strings.Contains(err.Error(), "insecure-http") {
			t.Errorf("error %q does not name the option that caused it", err)
		}
	}
}
