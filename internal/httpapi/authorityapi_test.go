package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
)

// authorityPreview is the rotation screen's payload.
type authorityPreviewResponse struct {
	Cluster       string `json:"cluster"`
	Name          string `json:"name"`
	ConfirmPhrase string `json:"confirm_phrase"`
	InProgress    bool   `json:"in_progress"`
	Locked        bool   `json:"locked"`
	Nodes         []struct {
		ID       string `json:"id"`
		Hostname string `json:"hostname"`
		Role     string `json:"role"`
	} `json:"nodes"`
	Passes   []string `json:"passes"`
	Warnings []string `json:"warnings"`
}

// TestTheRotationScreenSaysWhatItIsAboutToDo pins the preview, because this is
// the one operation in the product where the dialog is the last thing standing
// between an operator and a cluster that trusts nobody.
//
// It asserts the four passes are named and that the warnings include the two
// that are true and unwelcome: every node has to answer, and nobody has ever
// run this against real hardware.
func TestTheRotationScreenSaysWhatItIsAboutToDo(t *testing.T) {
	c := newJobHarness(t)
	cluster := c.adopt(t)
	id, _ := cluster["id"].(string)

	resp, raw := c.do(t, http.MethodGet, "/api/v1/clusters/"+id+"/authority", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview: %d (%s)", resp.StatusCode, raw)
	}
	var body authorityPreviewResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v (%s)", err, raw)
	}

	if body.ConfirmPhrase != body.Name || body.Name == "" {
		t.Errorf("confirm phrase = %q, want the cluster's name %q", body.ConfirmPhrase, body.Name)
	}
	if len(body.Nodes) != 1 {
		t.Errorf("the preview names %d nodes, want the one this cluster has", len(body.Nodes))
	}
	if len(body.Passes) != 4 {
		t.Errorf("the preview names %d passes, want 4", len(body.Passes))
	}
	if !body.Locked {
		t.Error("an imported cluster is shown as unlocked; it is adopted read-only (INV-12), and " +
			"a screen that hides that offers a button the route will refuse")
	}
	if body.InProgress {
		t.Error("a cluster with no rotation running is reported as in progress")
	}

	joined := strings.Join(body.Warnings, " ")
	for _, want := range []string{"has to answer", "never been run against real hardware", "Kubernetes"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the warnings do not mention %q: %v", want, body.Warnings)
		}
	}
}

// TestARotationNeedsTheClusterNameTypedAndTheClusterUnlocked walks the two
// gates in front of the operation, in the order an operator meets them.
func TestARotationNeedsTheClusterNameTypedAndTheClusterUnlocked(t *testing.T) {
	c := newJobHarness(t)
	cluster := c.adopt(t)
	id, _ := cluster["id"].(string)

	// Typing something else gets no token at all: the check is on the server,
	// so a client that skipped the box cannot get one.
	resp, raw := c.do(t, http.MethodPost, "/api/v1/clusters/"+id+"/authority/confirm",
		map[string]string{"typed": "homelab-but-not-quite"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("confirm with the wrong phrase: %d (%s), want 400", resp.StatusCode, raw)
	}

	token := c.confirmRotation(t, id)

	// The cluster is still locked, so the rotation is refused -- before
	// anything is written and with the code that names the lock.
	resp, raw = c.do(t, http.MethodPost, "/api/v1/clusters/"+id+"/authority",
		map[string]string{"confirmation": token})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("rotation on a locked cluster: %d (%s), want 403", resp.StatusCode, raw)
	}
	if p := decodeProblem(t, resp, raw); p.Code != httpapi.CodeClusterLocked {
		t.Errorf("code = %q, want %q", p.Code, httpapi.CodeClusterLocked)
	}
}

// TestAConfirmationForSomethingElseCannotRotateTheAuthority is the property
// the token exists for: it describes the action it authorises, and the server
// rebuilds that description from the request rather than reading it out of the
// token.
//
// The token here is a real, valid, unexpired one -- for rebooting a node. If
// the rotation accepted it, then "a confirmation was presented" would be the
// whole check, and any dialog the operator ever confirmed would authorise this
// one.
func TestAConfirmationForSomethingElseCannotRotateTheAuthority(t *testing.T) {
	c := newJobHarness(t)
	cluster := c.adopt(t)
	id, _ := cluster["id"].(string)

	machine := c.onlyMachineID(t)

	// Unlock first, so what refuses the request below is the confirmation and
	// not the read-only lock.
	if resp, raw := c.do(t, http.MethodPost, "/api/v1/clusters/"+id+"/lock",
		map[string]bool{"locked": false}); resp.StatusCode != http.StatusOK {
		t.Fatalf("unlock: %d (%s)", resp.StatusCode, raw)
	}

	resp, raw := c.do(t, http.MethodPost, "/api/v1/machines/"+machine+"/confirm",
		map[string]any{"action": "node.reboot"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirm a reboot: %d (%s)", resp.StatusCode, raw)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode token: %v", err)
	}

	resp, raw = c.do(t, http.MethodPost, "/api/v1/clusters/"+id+"/authority",
		map[string]string{"confirmation": out.Token})
	if resp.StatusCode == http.StatusAccepted {
		t.Fatal("a confirmation issued for rebooting a node started a rotation of the cluster's " +
			"certificate authority")
	}
	if p := decodeProblem(t, resp, raw); p.Code != httpapi.CodeConfirmationInvalid {
		t.Errorf("code = %q, want %q (status %d, body %s)", p.Code,
			httpapi.CodeConfirmationInvalid, resp.StatusCode, raw)
	}
}

// onlyMachineID is the cluster's single node, as a string.
func (h *inventoryHarness) onlyMachineID(t *testing.T) string {
	t.Helper()

	resp, raw := h.do(t, http.MethodGet, "/api/v1/machines", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("machines: %d (%s)", resp.StatusCode, raw)
	}
	var list struct {
		Machines []struct {
			ID string `json:"id"`
		} `json:"machines"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode machines: %v", err)
	}
	if len(list.Machines) != 1 {
		t.Fatalf("machines = %d, want 1", len(list.Machines))
	}
	return list.Machines[0].ID
}

// confirmRotation types the cluster's name and returns the token.
func (h *inventoryHarness) confirmRotation(t *testing.T, id string) string {
	t.Helper()

	resp, raw := h.do(t, http.MethodGet, "/api/v1/clusters/"+id+"/authority", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview: %d (%s)", resp.StatusCode, raw)
	}
	var preview authorityPreviewResponse
	if err := json.Unmarshal(raw, &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}

	resp, raw = h.do(t, http.MethodPost, "/api/v1/clusters/"+id+"/authority/confirm",
		map[string]string{"typed": preview.ConfirmPhrase})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirm: %d (%s)", resp.StatusCode, raw)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode token: %v", err)
	}
	if out.Token == "" {
		t.Fatal("the confirmation carried no token")
	}
	return out.Token
}
