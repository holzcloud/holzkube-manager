package talos_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// machineConfig is the shape of the file the operator uploaded in place of a
// talosconfig: what `talosctl gen config` writes as controlplane.yaml. The
// values are placeholders; the point is the shape and that none of them leaks.
const machineConfig = `version: v1alpha1
debug: false
persist: true
machine:
  type: controlplane
  token: placeholder-machine-token-value
  ca:
    crt: cGxhY2Vob2xkZXI=
    key: placeholder-ca-key-value
cluster:
  id: placeholder
  secret: placeholder-cluster-secret-value
  controlPlane:
    endpoint: https://192.168.1.110:6443
`

// TestAMachineConfigUploadedAsATalosconfigIsNamed is the guard for the
// operator's import: they uploaded holzkube-holzkube-01.yaml, a machine
// configuration, into the talosconfig field. The refusal said nothing that
// would tell them which file they should have picked -- and the file they did
// pick carries the cluster's CA private key, which is worth saying too.
func TestAMachineConfigUploadedAsATalosconfigIsNamed(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string]string{
		"a single document": machineConfig,
		"the first of several documents, as Talos 1.8 and later write it": machineConfig +
			"---\napiVersion: v1alpha1\nkind: HostnameConfig\nhostname: holzkube-01\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := talos.ParseTalosconfig([]byte(raw))
			if !errors.Is(err, talos.ErrTalosconfigInvalid) {
				t.Fatalf("err = %v, want ErrTalosconfigInvalid", err)
			}
			msg := err.Error()
			if !strings.Contains(msg, "machine configuration") || !strings.Contains(msg, "~/.talos/config") {
				t.Errorf("the refusal does not say this is a machine configuration or where the talosconfig is:\n%s", msg)
			}
			for _, secret := range []string{"placeholder-machine-token-value", "placeholder-ca-key-value", "placeholder-cluster-secret-value"} {
				if strings.Contains(msg, secret) {
					t.Errorf("the refusal repeats a value from the uploaded file: %s", msg)
				}
			}
		})
	}
}
