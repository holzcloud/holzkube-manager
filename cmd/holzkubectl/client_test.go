package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// serverWithItsOwnCertificate starts a TLS server holding a certificate
// generated for this one test.
//
// It exists because httptest.NewTLSServer hands every server it starts the
// same built-in certificate, so two of them have one fingerprint between them
// -- which made the first version of the mismatch test below pass against a
// pin that was working correctly and would equally have passed against no pin
// at all. A test that cannot fail is not evidence.
func serverWithItsOwnCertificate(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate a key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "holzkubectl test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
		DNSNames:              []string{"localhost"},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create a certificate: %v", err)
	}

	srv := httptest.NewUnstartedServer(h)
	srv.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// fingerprintOfServer is what an operator would paste into HOLZKUBE_FINGERPRINT
// after reading it off the server's startup line.
func fingerprintOfServer(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	sum := sha256.Sum256(srv.Certificate().Raw)
	return hex.EncodeToString(sum[:])
}

func clientFor(t *testing.T, srv *httptest.Server, fingerprint string) *Client {
	t.Helper()
	c, err := NewClient(Config{
		URL:         srv.URL,
		Token:       "hkm_test",
		Fingerprint: fingerprint,
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// TestThePinAcceptsTheCertificateItNames is the ordinary case, and it is here
// because the interesting one below only means something if this one passes:
// a pin that refused everything would also refuse the wrong certificate.
func TestThePinAcceptsTheCertificateItNames(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := clientFor(t, srv, fingerprintOfServer(t, srv))

	var raw []byte
	if err := c.Do(context.Background(), request{Method: http.MethodGet, Path: "/api/v1/machines"}, &raw); err != nil {
		t.Fatalf("the pinned certificate was refused: %v", err)
	}
}

// TestThePinRefusesADifferentCertificate is the whole reason the pin exists.
//
// The fingerprint is a real one -- of a second server -- rather than a made-up
// hex string, because that is the attack: something is answering on the address
// and presenting a certificate that verifies fine on its own terms.
func TestThePinRefusesADifferentCertificate(t *testing.T) {
	other := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer other.Close()

	srv := serverWithItsOwnCertificate(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the handler ran: the wrong certificate was accepted")
	}))

	c := clientFor(t, srv, fingerprintOfServer(t, other))

	var raw []byte
	err := c.Do(context.Background(), request{Method: http.MethodGet, Path: "/api/v1/machines"}, &raw)
	if err == nil {
		t.Fatal("a server presenting a different certificate was accepted")
	}
	if !strings.Contains(err.Error(), "HOLZKUBE_FINGERPRINT") {
		t.Errorf("the refusal does not say which setting disagreed: %v", err)
	}
}

// TestThePinHoldsOnEveryConnectionAndNotJustTheFirst is the trap
// internal/talos/pin.go documents, written down a second time because the CLI
// is a second place it could be got wrong: Go skips VerifyPeerCertificate
// entirely on a resumed TLS session, so a pin written there holds for one
// connection per process and silently for none of the rest.
//
// Two requests over a client that keeps its session cache, against a server
// whose certificate does not match. The second one is the one that matters.
func TestThePinHoldsOnEveryConnectionAndNotJustTheFirst(t *testing.T) {
	other := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer other.Close()

	srv := serverWithItsOwnCertificate(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))

	c := clientFor(t, srv, fingerprintOfServer(t, other))

	for attempt := range 2 {
		var raw []byte
		if err := c.Do(context.Background(), request{Method: http.MethodGet, Path: "/api/v1/machines"}, &raw); err == nil {
			t.Fatalf("request %d was accepted against the wrong certificate", attempt+1)
		}
	}
}

// TestAnUnpinnedClientStillVerifiesTheChain says there is no insecure default.
//
// The httptest certificate is signed by an authority no system trusts, so a
// client with no fingerprint must refuse it. If this ever passes, something
// has grown an InsecureSkipVerify that is not paired with a pin.
func TestAnUnpinnedClientStillVerifiesTheChain(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	c := clientFor(t, srv, "")

	var raw []byte
	if err := c.Do(context.Background(), request{Method: http.MethodGet, Path: "/api/v1/machines"}, &raw); err == nil {
		t.Fatal("an untrusted certificate was accepted by a client with no fingerprint set")
	}
}

// TestAPastedFingerprintIsAcceptedInTheShapeToolsPrintIt: openssl prints colon
// separated upper case, the server's own startup line prints bare lower case,
// and an operator pastes whichever they were looking at.
func TestAPastedFingerprintIsAcceptedInTheShapeToolsPrintIt(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	bare := fingerprintOfServer(t, srv)

	var colonised strings.Builder
	for i := 0; i < len(bare); i += 2 {
		if i > 0 {
			colonised.WriteByte(':')
		}
		colonised.WriteString(strings.ToUpper(bare[i : i+2]))
	}

	c := clientFor(t, srv, "  "+colonised.String()+"  ")

	var raw []byte
	if err := c.Do(context.Background(), request{Method: http.MethodGet, Path: "/api/v1/machines"}, &raw); err != nil {
		t.Fatalf("a fingerprint pasted from openssl was refused: %v", err)
	}
}

// TestTheTokenIsOnEveryRequest. It is the only credential this tool has, and a
// request without it is an anonymous one that the server answers with a 401 the
// operator then has to work out.
func TestTheTokenIsOnEveryRequest(t *testing.T) {
	var got string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := clientFor(t, srv, fingerprintOfServer(t, srv))

	var raw []byte
	if err := c.Do(context.Background(), request{Method: http.MethodGet, Path: "/api/v1/machines"}, &raw); err != nil {
		t.Fatal(err)
	}
	if got != "Bearer hkm_test" {
		t.Errorf("Authorization header is %q", got)
	}
}

// TestAToolWithNoServerSaysWhatToSet. "no server configured" on its own sends
// the operator to the documentation; naming the variable does not.
func TestAToolWithNoServerSaysWhatToSet(t *testing.T) {
	_, err := NewClient(Config{Token: "hkm_x"})
	if err == nil || !strings.Contains(err.Error(), "HOLZKUBE_URL") {
		t.Errorf("a client with no URL: %v", err)
	}

	_, err = NewClient(Config{URL: "https://example"})
	if err == nil || !strings.Contains(err.Error(), "HOLZKUBE_TOKEN") {
		t.Errorf("a client with no token: %v", err)
	}
}

// TestARefusalIsPrintedInTheServersOwnWords.
//
// The detail unchanged, the code so a script can match on it, and the field
// errors because "invalid" without the field is the refusal that costs an
// operator the most time.
func TestARefusalIsPrintedInTheServersOwnWords(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"type":"/problems/validation",
			"title":"Validation failed",
			"status":400,
			"code":"validation",
			"detail":"The labels are not valid.",
			"errors":[{"field":"labels.tier","reason":"must be at most 63 characters"}]
		}`))
	}))
	defer srv.Close()

	c := clientFor(t, srv, fingerprintOfServer(t, srv))

	var raw []byte
	err := c.Do(context.Background(), request{
		Method:      http.MethodPut,
		Path:        "/api/v1/machines/x/labels",
		ContentType: "application/json",
		Body:        []byte(`{}`),
	}, &raw)
	if err == nil {
		t.Fatal("a 400 was not reported as an error")
	}

	for _, want := range []string{
		"The labels are not valid.",
		"validation",
		"labels.tier",
		"must be at most 63 characters",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not carry %q:\n%v", want, err)
		}
	}
}

// TestARefusalThatIsNotAProblemDocumentStillSaysSomething. A reverse proxy in
// front of the server answers 502 in HTML, and a tool that reported "" there
// would be the least useful thing in the operator's terminal.
func TestARefusalThatIsNotAProblemDocumentStillSaysSomething(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>502 Bad Gateway</html>"))
	}))
	defer srv.Close()

	c := clientFor(t, srv, fingerprintOfServer(t, srv))

	var raw []byte
	err := c.Do(context.Background(), request{Method: http.MethodGet, Path: "/api/v1/machines"}, &raw)
	if err == nil {
		t.Fatal("a 502 was not reported as an error")
	}
	if !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "Bad Gateway") {
		t.Errorf("the refusal carries neither the status nor the body: %v", err)
	}
}
