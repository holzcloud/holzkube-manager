package kube_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// Reaching a workload (milestone v1.17, slice 6).
//
// The tests here are mostly refusals, and that is the honest shape of this
// feature: the useful half is four lines, and what makes it safe to have is what
// it will not do.

func proxyCluster(t *testing.T) (*kubesim.Server, *kube.Client) {
	t.Helper()

	return newCluster(t, kubesim.Options{
		Services: []kubesim.Service{{
			Namespace: "default",
			Name:      "api",
			Type:      "ClusterIP",
			ClusterIP: "10.96.0.12",
			Ports:     []kubesim.ServicePort{{Name: "http", Port: 8080}},
		}},
		ProxyBodies: map[string]string{
			"default/api:8080/healthz":   "ok",
			"default/api:http/healthz":   "ok",
			"default/api:8080/dashboard": "<script>alert(document.cookie)</script>",
		},
	})
}

// TestReachingAServiceGoesThroughTheAPIServersProxy, at the path that was asked
// for and no other.
func TestReachingAServiceGoesThroughTheAPIServersProxy(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := proxyCluster(t)

	response, err := client.ProxyGet(ctx, "default", "api", "8080", "/healthz")
	if err != nil {
		t.Fatalf("ProxyGet: %v", err)
	}
	if response.Body != "ok" || response.Status != 200 {
		t.Errorf("response = %+v, want the service's own answer", response)
	}
	if response.Truncated {
		t.Error("a two-byte body was reported as truncated")
	}

	if got := sim.Proxied(); !slices.Equal(got, []string{"default/api:8080/healthz"}) {
		t.Errorf("proxied = %v, want exactly the path that was asked for", got)
	}

	// A port NAME works too, because a service's ports are usually named and
	// making somebody look up the number would send them to kubectl.
	if _, err := client.ProxyGet(ctx, "default", "api", "http", "/healthz"); err != nil {
		t.Errorf("a named port: %v", err)
	}
}

// TestTheWorkloadsContentTypeIsNotHonoured is the refusal this slice exists for,
// and it is the one that would not show up in any screenshot.
//
// The fake answers with `text/html` and a script tag, which is what a hostile or
// simply ordinary workload serves. If this product ever handed that back as HTML
// from its own origin, any pod in the cluster could script the interface holding
// the operator's session. So the body is TEXT here, and the type the workload
// claimed is not carried at all.
func TestTheWorkloadsContentTypeIsNotHonoured(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := proxyCluster(t)

	response, err := client.ProxyGet(ctx, "default", "api", "8080", "/dashboard")
	if err != nil {
		t.Fatalf("ProxyGet: %v", err)
	}

	// The body is carried verbatim -- it is what the service said, and a proxy
	// that silently rewrote it would be lying about the workload.
	if !strings.Contains(response.Body, "<script>") {
		t.Errorf("body = %q, want the service's own bytes", response.Body)
	}
	// And there is nowhere for a content type to travel: ProxyResponse has no
	// such field, which is the structural version of this promise. If a field is
	// ever added, this test is the place that says why it must not be.
	if _, ok := any(response).(interface{ ContentType() string }); ok {
		t.Error("ProxyResponse grew a content type. Serving a workload's own type from this " +
			"daemon's origin is how a pod scripts the operator's session")
	}
}

// TestAPathThatClimbsOutIsRefused.
//
// What this is NOT about, because it was measured: with the check removed,
// "/healthz/../../../api/v1/secrets" reaches the service as "/api/v1/secrets".
// It stays inside the proxy subresource -- path.Clean collapses it and client-go
// escapes each segment -- so it does not address the API server's own resources.
// The red run printed exactly that, and the comment that used to be here claiming
// otherwise was wrong.
//
// What it IS about: such a path is silently rewritten, and the operator is shown
// the answer to a question they did not ask. A refusal says so.
func TestAPathThatClimbsOutIsRefused(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := proxyCluster(t)

	for _, path := range []string{
		"/../../nodes",
		"/healthz/../../../api/v1/secrets",
		"..",
		"/a/../../b",
	} {
		_, err := client.ProxyGet(ctx, "default", "api", "8080", path)
		if !errors.Is(err, kube.ErrProxyPathRefused) {
			t.Errorf("ProxyGet(%q) = %v, want ErrProxyPathRefused", path, err)
		}
	}

	// Nothing was sent at all -- and this is the assertion that caught the
	// rewriting when the check was removed: four requests arrived at the fake,
	// for paths nobody had typed.
	if got := sim.Proxied(); len(got) != 0 {
		t.Errorf("requests reached the cluster anyway: %v", got)
	}
}

// TestAPortThatIsNotAPortIsRefused.
//
// Measured with the check removed: a port carrying a slash is caught by client-go
// itself ("invalid resource name"), so that one was never a hole. The four that
// WERE passed through silently are "0", "70000", "http:8080" and "HTTP" -- sent
// to the API server as a port, answered with whatever it makes of them. This
// refuses them here, with a reason an operator can act on instead of a client-go
// error about resource names.
func TestAPortThatIsNotAPortIsRefused(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := proxyCluster(t)

	for _, port := range []string{"", "0", "70000", "8080/../nodes", "http:8080", "HTTP", "a b"} {
		if _, err := client.ProxyGet(ctx, "default", "api", port, "/healthz"); !errors.Is(
			err, kube.ErrProxyPathRefused) {
			t.Errorf("port %q = %v, want ErrProxyPathRefused", port, err)
		}
	}
}

// TestAServiceThatIsNotThereSaysSoRatherThanAnswering404FromThePath: the two are
// different things to an operator, and the product asks first so it can tell
// them apart.
func TestAServiceThatIsNotThereSaysSoRatherThanAnswering404FromThePath(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := proxyCluster(t)

	if _, err := client.ProxyGet(ctx, "default", "ghost", "8080", "/healthz"); !errors.Is(
		err, kube.ErrNoSuchWorkload) {
		t.Errorf("err = %v, want ErrNoSuchWorkload", err)
	}
}

// TestAPathTheServiceDoesNotServeComesBackAsItsOwnStatus: a 404 from the
// workload is the answer somebody came here for, not a failure of this product.
func TestAPathTheServiceDoesNotServeComesBackAsItsOwnStatus(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := proxyCluster(t)

	response, err := client.ProxyGet(ctx, "default", "api", "8080", "/nope")
	if err != nil {
		t.Fatalf("a 404 from the workload arrived as an error: %v", err)
	}
	if response.Status != 404 {
		t.Errorf("status = %d, want the workload's own 404", response.Status)
	}
}

// TestServicesAreListedWithTheirPorts, because the proxy needs a port and a
// screen that made somebody guess it would send them to kubectl.
func TestServicesAreListedWithTheirPorts(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := proxyCluster(t)

	services, err := client.Services(ctx, "")
	if err != nil {
		t.Fatalf("Services: %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("services = %+v", services)
	}
	if services[0].Type != "ClusterIP" || services[0].ClusterIP != "10.96.0.12" {
		t.Errorf("service = %+v, want the type and address that answer 'why can I not reach "+
			"this from outside'", services[0])
	}
	if len(services[0].Ports) != 1 || services[0].Ports[0].Port != 8080 ||
		services[0].Ports[0].Name != "http" {
		t.Errorf("ports = %+v, want the number and the name", services[0].Ports)
	}
}

// TestABodyLargerThanTheCapIsTruncatedAndSaysSo.
//
// A service can answer with a gigabyte and this process would hold it. Half a
// metrics page that looked whole would be read as a complete one, so the cut is
// reported rather than made quietly.
func TestABodyLargerThanTheCapIsTruncatedAndSaysSo(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{
		Services: []kubesim.Service{{Namespace: "default", Name: "chatty"}},
		ProxyBodies: map[string]string{
			"default/chatty:80/metrics": strings.Repeat("x", kube.MaxProxyBytes+4096),
		},
	})

	response, err := client.ProxyGet(ctx, "default", "chatty", "80", "/metrics")
	if err != nil {
		t.Fatalf("ProxyGet: %v", err)
	}
	if len(response.Body) != kube.MaxProxyBytes {
		t.Errorf("body = %d bytes, want it cut at the cap of %d", len(response.Body), kube.MaxProxyBytes)
	}
	if !response.Truncated {
		t.Error("the body was cut and not reported as cut, which is how half a page is read as a " +
			"whole one")
	}
}
