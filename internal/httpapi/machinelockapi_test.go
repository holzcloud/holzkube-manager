package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
)

// TestAMachineRouteAsksTheLockOfTheMachinesOwnCluster is the read-only lock on
// the four routes that act on one machine and used to take the cluster to ask
// from the request body.
//
// The lock link checked whatever cluster the body named and the handler then
// acted on the machine in the path, whose cluster nobody compared. A body that
// named no cluster -- or an unlocked one -- walked a node of a read-only
// cluster into a reboot, a shutdown, a wipe or a configuration change. The
// cluster is a fact about the machine, so it is read from the inventory.
func TestAMachineRouteAsksTheLockOfTheMachinesOwnCluster(t *testing.T) {
	h := newJobHarness(t)
	id := h.adoptedMachine(t) // adopted read-only, and left that way

	bodies := map[string]map[string]any{
		"no cluster named":      {"confirmation": "x"},
		"an empty cluster":      {"confirmation": "x", "cluster": ""},
		"another cluster named": {"confirmation": "x", "cluster": "c-some-unlocked-cluster"},
	}

	for _, route := range []string{"reboot", "shutdown", "reset", "config/apply"} {
		for name, body := range bodies {
			t.Run(route+"/"+name, func(t *testing.T) {
				resp, raw := h.do(t, http.MethodPost, "/api/v1/machines/"+string(id)+"/"+route, body)
				if resp.StatusCode != http.StatusForbidden {
					t.Fatalf("%s on a node of a locked cluster: %d (%s), want 403", route, resp.StatusCode, raw)
				}
				if p := decodeProblem(t, resp, raw); p.Code != httpapi.CodeClusterLocked {
					t.Errorf("code = %q, want %q", p.Code, httpapi.CodeClusterLocked)
				}
			})
		}
	}
}

// TestAnEtcdRestoreAsksTheLockOfTheMachinesCluster: the restore replaces a
// cluster's etcd, and it declared no cluster, so a cluster adopted read-only
// could have its whole state replaced.
func TestAnEtcdRestoreAsksTheLockOfTheMachinesCluster(t *testing.T) {
	h := newJobHarness(t)
	id := h.adoptedMachine(t) // adopted read-only, and left that way

	resp, raw := h.do(t, http.MethodPost,
		"/api/v1/machines/"+string(id)+"/etcd/restore?machine="+string(id), map[string]any{})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("an etcd restore onto a node of a locked cluster: %d (%s), want 403", resp.StatusCode, raw)
	}
	if p := decodeProblem(t, resp, raw); p.Code != httpapi.CodeClusterLocked {
		t.Errorf("code = %q, want %q", p.Code, httpapi.CodeClusterLocked)
	}
}
