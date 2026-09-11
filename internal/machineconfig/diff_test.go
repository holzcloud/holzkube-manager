package machineconfig_test

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestAListThatGrewIsReportedAsAListThatGrew is CFG-04's point.
//
// A text diff says "one line added". What the operator needs to know is that a
// list went from three entries to four, because that is the shape a strategic
// merge patch applied twice produces -- and the second application is the one
// nobody meant.
func TestAListThatGrewIsReportedAsAListThatGrew(t *testing.T) {
	t.Parallel()

	_, base := fixture(t)

	once, err := machineconfig.ApplyPatches(base, []string{
		"machine:\n  certSANs:\n    - 10.0.0.9\n",
	})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}

	diff, err := machineconfig.Compare(base, once)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}

	var listChange *machineconfig.Change
	for i, c := range diff.Changes {
		if strings.Contains(c.Path, "certSANs") {
			listChange = &diff.Changes[i]
		}
	}
	if listChange == nil {
		t.Fatalf("the certSANs change is not in the diff: %+v", diff.Changes)
	}
	if listChange.Kind != machineconfig.ChangeListGrew {
		t.Errorf("the change is %q, want %q", listChange.Kind, machineconfig.ChangeListGrew)
	}
	if listChange.LenAfter != listChange.LenBefore+1 {
		t.Errorf("lengths are %d -> %d, want one more", listChange.LenBefore, listChange.LenAfter)
	}
}

// TestADuplicateIsCalledOut is the finding that usually means the operator is
// about to do something they did not intend.
func TestADuplicateIsCalledOut(t *testing.T) {
	t.Parallel()

	before := []byte("machine:\n  certSANs:\n    - 10.0.0.9\n")
	after := []byte("machine:\n  certSANs:\n    - 10.0.0.9\n    - 10.0.0.9\n")

	diff, err := machineconfig.Compare(before, after)
	// The fixtures here are fragments rather than whole configurations, so the
	// parse fails and only the text-level redaction runs -- which is exactly
	// the path this asserts still produces a usable diff.
	if err != nil && !strings.Contains(err.Error(), "did not parse") {
		t.Fatalf("Compare: %v", err)
	}

	if !diff.Duplicates {
		t.Fatalf("a list with the same value twice was not reported as having duplicates: %+v", diff.Changes)
	}
}

// TestReorderingAloneIsNotAChange is what a structural diff buys.
//
// A text diff reports reindentation and key order as differences, and an
// operator who sees noise learns to skim -- on the one screen where skimming
// is expensive.
func TestReorderingAloneIsNotAChange(t *testing.T) {
	t.Parallel()

	a := []byte("machine:\n  type: controlplane\n  token: abc\n")
	b := []byte("machine:\n  token: abc\n  type: controlplane\n")

	diff, err := machineconfig.Compare(a, b)
	if err != nil && !strings.Contains(err.Error(), "did not parse") {
		t.Fatalf("Compare: %v", err)
	}
	if len(diff.Changes) != 0 {
		t.Fatalf("reordering two keys was reported as %d change(s): %+v", len(diff.Changes), diff.Changes)
	}
}

// TestApplyModeIsComputedFromTheChangedPaths is CFG-06, CFG-07 and CFG-09 in
// one table.
func TestApplyModeIsComputedFromTheChangedPaths(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name     string
		paths    []string
		mode     machineconfig.Mode
		contains string
	}{
		{
			name:     "a kubelet change applies live",
			paths:    []string{".machine.kubelet.extraArgs"},
			mode:     machineconfig.ModeNoReboot,
			contains: "immediately",
		},
		{
			name:     "a network change wants the rollback timer",
			paths:    []string{".machine.network.interfaces"},
			mode:     machineconfig.ModeTry,
			contains: "rollback timer",
		},
		{
			name:     "an install change applies and does nothing",
			paths:    []string{".machine.install.disk"},
			mode:     machineconfig.ModeReboot,
			contains: "next install or upgrade",
		},
		{
			name:     "an unknown path is assumed to need a reboot",
			paths:    []string{".machine.somethingNobodyCurated"},
			mode:     machineconfig.ModeReboot,
			contains: "needs a reboot",
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			v := machineconfig.ModeFor(row.paths)
			if v.Mode != row.mode {
				t.Errorf("mode = %q, want %q (%v)", v.Mode, row.mode, v.Sentences)
			}
			joined := strings.Join(v.Sentences, " ")
			if !strings.Contains(joined, row.contains) {
				t.Errorf("the verdict reads %q, which does not say %q", joined, row.contains)
			}
		})
	}
}

// TestTheTryCountdownMatchesTalos pins the one number that appears both on the
// screen and on the node. If they differ, the screen is lying about how long
// is left.
func TestTheTryCountdownMatchesTalos(t *testing.T) {
	t.Parallel()

	v := machineconfig.ModeFor([]string{".machine.network.hostname"})
	if v.TrySeconds != int(machineconfig.TryTimeout.Seconds()) {
		t.Fatalf("the countdown is %ds and the timeout is %s", v.TrySeconds, machineconfig.TryTimeout)
	}
}

// TestJSONPatchesAreRefusedByName is CFG-03's exclusion, with the reason in
// the message.
func TestJSONPatchesAreRefusedByName(t *testing.T) {
	t.Parallel()

	jsonPatch := `- op: replace
  path: /machine/certSANs/2
  value: 10.0.0.9
`
	err := machineconfig.ValidatePatch(jsonPatch)
	if !errors.Is(err, machineconfig.ErrPatchNotStrategic) {
		t.Fatalf("a JSON patch was accepted or refused for the wrong reason: %v", err)
	}
	if !strings.Contains(err.Error(), "index") {
		t.Errorf("the refusal reads %q, which does not say why an index is the problem", err)
	}
}

// TestAnAppendingPatchIsNotIdempotent is CFG-05, and it is the check that
// makes the duplicate above findable before it is applied rather than after.
func TestAnAppendingPatchIsNotIdempotent(t *testing.T) {
	t.Parallel()

	_, base := fixture(t)

	appending := "machine:\n  certSANs:\n    - 10.0.0.9\n"
	idempotent, err := machineconfig.IsIdempotent(base, appending)
	if err != nil {
		t.Fatalf("IsIdempotent: %v", err)
	}
	if idempotent {
		t.Error("a patch that appends to a list was reported as idempotent")
	}

	// A scalar set is idempotent, which is the contrast that makes the check
	// worth having rather than a constant false.
	setting := "machine:\n  network:\n    hostname: node-9\n"
	idempotent, err = machineconfig.IsIdempotent(base, setting)
	if err != nil {
		t.Fatalf("IsIdempotent: %v", err)
	}
	if !idempotent {
		t.Error("setting a scalar twice was reported as changing something the second time")
	}
}

// TestAnEmptyPatchIsRefused keeps a patch that changes nothing from being
// stored as if it did.
func TestAnEmptyPatchIsRefused(t *testing.T) {
	t.Parallel()

	for _, body := range []string{"", "   \n", "{}"} {
		if err := machineconfig.ValidatePatch(body); !errors.Is(err, machineconfig.ErrPatchInvalid) {
			t.Errorf("ValidatePatch(%q) = %v, want ErrPatchInvalid", body, err)
		}
	}
}

// TestGenerateUsesTheStoredBundleAndThePinnedContract is CFG-11.
//
// The two failures it guards are different and both quiet: a generated
// configuration with fresh PKI is internally consistent and no node accepts
// it, and one generated without a pinned contract depends on which build of
// holzkube-manager produced it.
func TestGenerateUsesTheStoredBundleAndThePinnedContract(t *testing.T) {
	t.Parallel()

	cl, _ := fixture(t)
	stored := storedSecrets(cl)

	raw, err := machineconfig.Generate(machineconfig.GenerateInput{
		ClusterName:       "redaction",
		Endpoint:          "https://10.0.0.1:6443",
		Secrets:           stored,
		ControlPlane:      false,
		TalosVersion:      "v1.13",
		KubernetesVersion: "1.34.1",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// The cluster's own authority, not a fresh one. A generated configuration
	// with fresh PKI is the failure D-07 refuses on the import path, arriving
	// by a different door.
	if !strings.Contains(string(raw), strings.TrimSpace(base64OSCA(cl))) {
		t.Error("the generated configuration does not carry the cluster's own Talos CA certificate")
	}

	// A worker's configuration carries the CA certificate without its private
	// key, which is what makes a worker a worker.
	if strings.Contains(string(raw), strings.TrimSpace(base64OSCAKey(cl))) {
		t.Error("a worker's generated configuration carries the CA private key")
	}
}

// TestGenerateRefusesWithoutAContract pins the absence of a default.
func TestGenerateRefusesWithoutAContract(t *testing.T) {
	t.Parallel()

	cl, _ := fixture(t)
	_, err := machineconfig.Generate(machineconfig.GenerateInput{
		ClusterName:       "redaction",
		Endpoint:          "https://10.0.0.1:6443",
		Secrets:           storedSecrets(cl),
		KubernetesVersion: "1.34.1",
	})
	if err == nil {
		t.Fatal("a configuration was generated with no pinned Talos version contract")
	}
	if !strings.Contains(err.Error(), "contract") {
		t.Errorf("the refusal reads %q, which does not say what is missing", err)
	}
}

// TestGenerateRefusesAnEmptyBundle keeps machinery from producing a document
// with an empty certificate authority -- which looks right and opens nothing.
func TestGenerateRefusesAnEmptyBundle(t *testing.T) {
	t.Parallel()

	_, err := machineconfig.Generate(machineconfig.GenerateInput{
		ClusterName:       "redaction",
		Endpoint:          "https://10.0.0.1:6443",
		TalosVersion:      "v1.13",
		KubernetesVersion: "1.34.1",
	})
	if err == nil {
		t.Fatal("a configuration was generated from a bundle with no certificate authority")
	}
}

func storedSecrets(cl *talossim.Cluster) model.ClusterSecrets {
	s := cl.Secrets
	return model.ClusterSecrets{
		OSCACrt:           s.Certs.OS.Crt,
		OSCAKey:           s.Certs.OS.Key,
		K8sCACrt:          s.Certs.K8s.Crt,
		K8sCAKey:          s.Certs.K8s.Key,
		EtcdCACrt:         s.Certs.Etcd.Crt,
		EtcdCAKey:         s.Certs.Etcd.Key,
		AggregatorCACrt:   s.Certs.K8sAggregator.Crt,
		AggregatorCAKey:   s.Certs.K8sAggregator.Key,
		ServiceAccountKey: s.Certs.K8sServiceAccount.Key,
		TalosClusterID:    s.Cluster.ID,
		ClusterSecret:     s.Cluster.Secret,
		BootstrapToken:    s.Secrets.BootstrapToken,
		MachineToken:      s.TrustdInfo.Token,
	}
}

// base64OSCA and base64OSCAKey are how the certificate and key appear inside a
// machine configuration: base64 of the PEM, on one line.
func base64OSCA(cl *talossim.Cluster) string {
	return base64.StdEncoding.EncodeToString(cl.Secrets.Certs.OS.Crt)
}

func base64OSCAKey(cl *talossim.Cluster) string {
	return base64.StdEncoding.EncodeToString(cl.Secrets.Certs.OS.Key)
}
