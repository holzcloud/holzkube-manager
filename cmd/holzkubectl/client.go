package main

// The client half of holzkubectl (V2-API-01, Omni parity phase 7).
//
// The single rule this tool is built under: **it is a client of the REST API
// and implements nothing.** Every answer it prints came from the server, and
// every decision it displays was made there. The reason is the decision
// PROJECT.md records -- "one interface to maintain" -- and the only cut in
// which that and "have a CLI" are both true: a second implementation of a
// verdict is a second thing to keep in step, and the two disagreeing is worse
// than not having the CLI.
//
// So there is no domain logic below this line. There is a request, a schema
// this file does not define, and a table.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config is how this tool reaches a server.
//
// Environment variables and flags only, and no config file, for the reason the
// server itself gives for the same choice: nothing to parse means nothing to
// migrate, and a token in a file is a token in a backup.
type Config struct {
	// URL is the base address of a holzkube-manager instance.
	URL string

	// Token is a service-account token. It is the only credential this tool
	// accepts -- deliberately, and see Authenticate below.
	Token string

	// Fingerprint pins the server's TLS certificate, as SHA-256 hex.
	//
	// It exists because holzkube-manager generates its own certificate on
	// first run, which no public authority signed, and because the alternative
	// every CLI reaches for is an --insecure flag. This product refuses that
	// shape for the nodes it manages -- D-03 pins a node's certificate by
	// fingerprint rather than skipping verification -- and a tool that took
	// the shortcut it denies its own transport would be telling the operator
	// two different things about the same risk.
	Fingerprint string

	// Timeout bounds one request.
	Timeout time.Duration
}

// FromEnvironment reads the configuration an operator exported.
func FromEnvironment() Config {
	timeout := 30 * time.Second
	if raw := os.Getenv("HOLZKUBE_TIMEOUT"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			timeout = d
		}
	}
	return Config{
		URL:         strings.TrimRight(os.Getenv("HOLZKUBE_URL"), "/"),
		Token:       os.Getenv("HOLZKUBE_TOKEN"),
		Fingerprint: os.Getenv("HOLZKUBE_FINGERPRINT"),
		Timeout:     timeout,
	}
}

// ErrNotConfigured reports a tool with nowhere to talk to.
var ErrNotConfigured = errors.New("holzkubectl: no server configured")

// Client talks to one instance.
type Client struct {
	cfg  Config
	http *http.Client
}

// NewClient builds a client, or says what is missing.
func NewClient(cfg Config) (*Client, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("%w: set HOLZKUBE_URL to the address of a holzkube-manager instance",
			ErrNotConfigured)
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("%w: set HOLZKUBE_TOKEN to a service-account token. "+
			"Create one under Settings, Accounts; it is shown once",
			ErrNotConfigured)
	}

	transport := &http.Transport{}
	if cfg.Fingerprint != "" {
		transport.TLSClientConfig = pinnedTLS(cfg.Fingerprint)
	}

	return &Client{cfg: cfg, http: &http.Client{Transport: transport, Timeout: cfg.Timeout}}, nil
}

// pinnedTLS verifies the server by certificate fingerprint instead of by chain.
//
// VerifyConnection and not VerifyPeerCertificate, which is the same trap
// internal/talos/pin.go documents: Go skips the latter entirely on a resumed
// TLS session, so a pin written there holds on the first connection of a
// process and on none of the rest.
func pinnedTLS(want string) *tls.Config {
	want = normaliseFingerprint(want)

	return &tls.Config{
		// Verification is not skipped -- it is replaced. The chain cannot be
		// checked because there is no authority to check it against; the
		// identity still is, and by a stronger check than a chain gives.
		InsecureSkipVerify: true, //nolint:gosec // replaced by VerifyConnection below, which is stricter
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("the server presented no certificate")
			}
			got := fingerprintOf(cs.PeerCertificates[0])
			if got != want {
				return fmt.Errorf("the server's certificate is %s and HOLZKUBE_FINGERPRINT is %s",
					got, want)
			}
			return nil
		},
	}
}

func fingerprintOf(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

// normaliseFingerprint accepts the shapes an operator will paste: colon
// separated or not, upper case or lower.
func normaliseFingerprint(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), ":", ""))
}

// request is one call to the API.
//
// A struct rather than five positional arguments, and ContentType is on it
// rather than defaulting to JSON, because the default is what went wrong: the
// cluster-template route takes a YAML document as its body, and a client that
// assumed JSON for everything with a body labelled that document as something
// it is not. Nothing in front of the server looked, which is the only reason it
// worked. Naming the type at each call site means the next route that takes
// something else cannot be got wrong quietly.
type request struct {
	Method string
	Path   string

	// ContentType must be set whenever Body is, and is the type of these
	// bytes as sent.
	ContentType string
	Body        []byte
}

// Do makes one request and decodes the answer into out.
//
// out may be nil for a route that answers no body, and a *[]byte for one that
// answers something that is not JSON.
func (c *Client) Do(ctx context.Context, req request, out any) error {
	var reader io.Reader
	if req.Body != nil {
		reader = bytes.NewReader(req.Body)
	}

	hreq, err := http.NewRequestWithContext(ctx, req.Method, c.cfg.URL+req.Path, reader)
	if err != nil {
		return err
	}
	hreq.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	hreq.Header.Set("Accept", "application/json")
	if req.Body != nil {
		if req.ContentType == "" {
			// A programming error in this package, not a condition an
			// operator can reach or fix.
			return fmt.Errorf("%s %s: a body was sent with no content type", req.Method, req.Path)
		}
		hreq.Header.Set("Content-Type", req.ContentType)
	}

	resp, err := c.http.Do(hreq)
	if err != nil {
		return fmt.Errorf("%s %s: %w", req.Method, req.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading the response: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		return problemFrom(resp.StatusCode, raw)
	}

	switch dst := out.(type) {
	case nil:
		return nil
	case *[]byte:
		*dst = raw
		return nil
	default:
		if len(raw) == 0 {
			return nil
		}
		return json.Unmarshal(raw, out)
	}
}

// problemFrom turns an RFC 9457 problem document into an error.
//
// The server's own sentence is what is printed, unchanged. This tool has no
// better account of any refusal than the one that made it, and rewriting a
// refusal is how two halves of an interface come to describe one condition as
// two different problems.
func problemFrom(status int, raw []byte) error {
	var p struct {
		Detail string `json:"detail"`
		Code   string `json:"code"`
		Errors []struct {
			Field  string `json:"field"`
			Reason string `json:"reason"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.Detail == "" {
		return fmt.Errorf("the server answered %d: %s", status, strings.TrimSpace(string(raw)))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s)", p.Detail, p.Code)
	for _, fe := range p.Errors {
		fmt.Fprintf(&b, "\n  %s: %s", fe.Field, fe.Reason)
	}
	return errors.New(b.String())
}
