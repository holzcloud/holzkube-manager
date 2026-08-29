package imagefactory

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// This is an internal test because the property under it cannot be staged from
// outside the package. The scheme downgrade that matters is same host, different
// scheme -- https://factory.talos.dev/... to http://factory.talos.dev/... -- and
// two httptest listeners are never the same host:port, so a redirect between
// them is caught by the host check and proves nothing about the scheme one.
// Calling the CheckRedirect hook with a chain built by hand is the only way to
// put the two cases side by side.

// chain builds the (req, via) pair net/http hands CheckRedirect: via[0] is the
// request the caller made and req is the hop the server is proposing.
func chain(t *testing.T, hops ...string) (*http.Request, []*http.Request) {
	t.Helper()

	if len(hops) < 2 {
		t.Fatalf("a redirect chain needs an origin and at least one hop, got %d", len(hops))
	}
	reqs := make([]*http.Request, 0, len(hops))
	for _, raw := range hops {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		reqs = append(reqs, &http.Request{Method: http.MethodPost, URL: u})
	}
	return reqs[len(reqs)-1], reqs[:len(reqs)-1]
}

// TestRefuseCrossHostRedirectRefusesASchemeDowngrade is CR-03.
//
// Go re-sends the request body on 307 and 308, and the one POST this package
// makes carries the canonical schematic document -- the kernel arguments and
// META values this package's own documentation identifies as capable of holding
// secrets, and the reason the Factory refuses to enumerate schematics at all. A
// single 308 to the same host over http therefore puts them on the wire in
// clear, and New's promise that "TLS verification is never disabled and there
// is no option to disable it" is defeated with no option set.
func TestRefuseCrossHostRedirectRefusesASchemeDowngrade(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		hops    []string
		refused bool
		because string
	}{
		{
			name:    "https to http on the same host",
			hops:    []string{"https://factory.talos.dev/schematics", "http://factory.talos.dev/schematics"},
			refused: true,
			because: "scheme",
		},
		{
			name:    "http to https on the same host",
			hops:    []string{"http://factory.example/schematics", "https://factory.example/schematics"},
			refused: true,
			because: "scheme",
		},
		{
			name:    "another host",
			hops:    []string{"https://factory.talos.dev/schematics", "https://elsewhere.example/schematics"},
			refused: true,
			because: "redirect from",
		},
		{
			name:    "another host over http, which is both faults at once",
			hops:    []string{"https://factory.talos.dev/schematics", "http://elsewhere.example/schematics"},
			refused: true,
		},
		{
			name: "same host, same scheme, different path",
			hops: []string{"https://factory.talos.dev/schematics", "https://factory.talos.dev/v2/schematics"},
		},
		{
			// The port is part of Host, so this is a different host and not a
			// scheme question. Stated as a case so the two are not conflated.
			name:    "same name on another port",
			hops:    []string{"https://factory.talos.dev/schematics", "https://factory.talos.dev:8443/schematics"},
			refused: true,
			because: "redirect from",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, via := chain(t, tc.hops...)
			err := refuseCrossHostRedirect(req, via)

			switch {
			case tc.refused && err == nil:
				t.Fatalf("the redirect %s -> %s was followed", tc.hops[0], tc.hops[len(tc.hops)-1])
			case !tc.refused && err != nil:
				t.Fatalf("the redirect %s -> %s was refused: %v", tc.hops[0], tc.hops[len(tc.hops)-1], err)
			}
			if tc.because != "" && !strings.Contains(err.Error(), tc.because) {
				t.Errorf("the refusal does not say it is about %q: %v", tc.because, err)
			}
		})
	}
}

// TestRefuseCrossHostRedirectStillBoundsTheChain keeps the redirect count
// reachable. It is the check the host and scheme tests would otherwise mask:
// every off-host and off-scheme hop returns before it, so a same-host,
// same-scheme chain is the only thing the bound ever applies to.
func TestRefuseCrossHostRedirectStillBoundsTheChain(t *testing.T) {
	t.Parallel()

	hops := make([]string, 0, maxRedirects+2)
	for i := range maxRedirects + 2 {
		hops = append(hops, "https://factory.talos.dev/hop/"+string(rune('a'+i)))
	}

	if _, via := chain(t, hops[:maxRedirects]...); len(via) != maxRedirects-1 {
		t.Fatalf("the chain builder produced %d prior hops, want %d", len(via), maxRedirects-1)
	}
	req, via := chain(t, hops[:maxRedirects]...)
	if err := refuseCrossHostRedirect(req, via); err != nil {
		t.Errorf("a same-host chain of %d hops was refused: %v", len(via), err)
	}

	req, via = chain(t, hops...)
	err := refuseCrossHostRedirect(req, via)
	if err == nil {
		t.Fatalf("a chain of %d hops was followed; the bound is %d", len(via), maxRedirects)
	}
	if !strings.Contains(err.Error(), "redirects") {
		t.Errorf("the refusal does not say it is about the redirect count: %v", err)
	}
}

// TestEveryClientCarriesTheRedirectPolicy is what keeps the two tests above
// from being about a function nothing calls. Both constructors have to install
// it: WithHTTPClient forces CheckRedirect for exactly this reason, and a client
// built without it would follow a downgrade whatever refuseCrossHostRedirect
// says.
func TestEveryClientCarriesTheRedirectPolicy(t *testing.T) {
	t.Parallel()

	req, via := chain(t,
		"https://factory.talos.dev/schematics",
		"http://factory.talos.dev/schematics")

	for _, tc := range []struct {
		name string
		opts []Option
	}{
		{name: "New"},
		{name: "WithHTTPClient", opts: []Option{WithHTTPClient(&http.Client{})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, err := New("https://factory.talos.dev", tc.opts...)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if c.http.CheckRedirect == nil {
				t.Fatal("the client has no CheckRedirect, so every redirect is followed")
			}
			if err := c.http.CheckRedirect(req, via); err == nil {
				t.Error("the client follows an https -> http redirect on the same host")
			}
		})
	}
}
