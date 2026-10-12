package upgrade_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/k8srelease"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The real client, the service and the chain, joined, against a stand-in for
// dl.k8s.io. The service is handed the client's Newest and nothing else, which
// is exactly what the daemon does.

func TestTheRealSourceFeedsTheChain(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release/stable-1.33.txt":
			_, _ = fmt.Fprint(w, "v1.33.7")
		case "/release/latest-1.33.txt":
			_, _ = fmt.Fprint(w, "v1.33.0-rc.1")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := k8srelease.New(srv.URL + "/release")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	svc := newKubernetesPlanService([]model.Machine{kubernetesMachine("v1.13.9", "v1.32.4")}, client.Newest)

	plan := planKubernetes(t, svc, "v1.34.1")

	if got, want := fmt.Sprint(chainOf(plan)), "[v1.33.7 v1.34.1]"; got != want {
		t.Errorf("the chain is %s, want %s (blocked=%v: %s)", got, want, plan.Blocked, plan.BlockReason)
	}
}

func TestTheRealSourceAnsweringNotFoundBlocksThePlan(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `<?xml version='1.0'?><Error><Code>NoSuchKey</Code></Error>`)
	}))
	t.Cleanup(srv.Close)

	client, err := k8srelease.New(srv.URL + "/release")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	svc := newKubernetesPlanService([]model.Machine{kubernetesMachine("v1.13.9", "v1.32.4")}, client.Newest)

	plan := planKubernetes(t, svc, "v1.34.1")

	if !plan.Blocked {
		t.Fatalf("a plan whose source answered 404 was not blocked (chain %v)", chainOf(plan))
	}
	for _, want := range []string{"stable-1.33.txt", "404"} {
		if !strings.Contains(plan.BlockReason, want) {
			t.Errorf("the block reason %q does not mention %q", plan.BlockReason, want)
		}
	}
	if len(plan.Chain) != 0 {
		t.Errorf("a blocked plan still carries a chain: %v", chainOf(plan))
	}
}
