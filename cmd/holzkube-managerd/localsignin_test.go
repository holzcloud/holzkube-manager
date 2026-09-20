package main

import (
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/config"
)

// Where the local account still works (2026-09-20).
//
// The sign-in page on an SSO-only address said "the local account works on the
// local network" and left the operator to remember which address that was. The
// answer is already written down in the configuration, so it is derived rather
// than added as a setting -- a second place to keep in step would drift, and a
// link to an address this process does not answer on is worse than no link.

func configFor(t *testing.T, listen string, allowed, ssoOnly []string, insecure bool) config.Config {
	t.Helper()

	return config.Config{
		Listen:       listen,
		AllowedHosts: allowed,
		SSOOnlyHosts: ssoOnly,
		InsecureHTTP: insecure,
	}
}

// TestTheLocalAddressIsTheOneNotDeclaredSSOOnly.
func TestTheLocalAddressIsTheOneNotDeclaredSSOOnly(t *testing.T) {
	t.Parallel()

	got := localSignInURL(configFor(t,
		"192.168.0.30:8443",
		[]string{"manager.holzcloud.ch", "srv-rsp-prod01.lan"},
		[]string{"manager.holzcloud.ch"},
		false,
	))()

	if got != "https://srv-rsp-prod01.lan:8443" {
		t.Errorf("local address = %q, want the configured name that is not SSO-only", got)
	}
}

// TestALoopbackNameIsNeverOffered.
//
// A link to localhost on the office PC or a phone points at that device, not at
// this one, and lands on a connection refused -- which reads as the product being
// broken rather than as the link being useless there.
func TestALoopbackNameIsNeverOffered(t *testing.T) {
	t.Parallel()

	got := localSignInURL(configFor(t,
		"127.0.0.1:8443",
		[]string{"manager.holzcloud.ch", "localhost"},
		[]string{"manager.holzcloud.ch"},
		false,
	))()

	if got != "" {
		t.Errorf("local address = %q, want nothing rather than a loopback name", got)
	}
}

// TestTheBindAddressIsOfferedWhenNothingElseIs.
func TestTheBindAddressIsOfferedWhenNothingElseIs(t *testing.T) {
	t.Parallel()

	got := localSignInURL(configFor(t,
		"192.168.0.30:8443",
		[]string{"manager.holzcloud.ch"},
		[]string{"manager.holzcloud.ch"},
		false,
	))()

	if got != "https://192.168.0.30:8443" {
		t.Errorf("local address = %q, want the address this process binds", got)
	}
}

// TestNothingIsOfferedWhenEveryAddressIsSSOOnly, because there is then no honest
// answer and the sentence stands on its own as it did before.
func TestNothingIsOfferedWhenEveryAddressIsSSOOnly(t *testing.T) {
	t.Parallel()

	got := localSignInURL(configFor(t,
		"0.0.0.0:8443",
		[]string{"manager.holzcloud.ch"},
		[]string{"manager.holzcloud.ch", "0.0.0.0"},
		false,
	))()

	if got != "" {
		t.Errorf("local address = %q, want nothing", got)
	}
}

// TestTheSchemeFollowsTheServerRatherThanBeingAssumed.
func TestTheSchemeFollowsTheServerRatherThanBeingAssumed(t *testing.T) {
	t.Parallel()

	got := localSignInURL(configFor(t,
		"192.168.0.30:8080",
		[]string{"manager.holzcloud.ch", "srv.lan"},
		[]string{"manager.holzcloud.ch"},
		true,
	))()

	if got != "http://srv.lan:8080" {
		t.Errorf("local address = %q, want http because this instance serves http", got)
	}
}

// TestTheDefaultPortIsLeftOut, because ":443" in a link is noise an operator has
// to read past.
func TestTheDefaultPortIsLeftOut(t *testing.T) {
	t.Parallel()

	got := localSignInURL(configFor(t,
		"192.168.0.30:443",
		[]string{"manager.holzcloud.ch", "srv.lan"},
		[]string{"manager.holzcloud.ch"},
		false,
	))()

	if got != "https://srv.lan" {
		t.Errorf("local address = %q, want no port for the default one", got)
	}
}
