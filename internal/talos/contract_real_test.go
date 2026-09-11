package talos_test

// The real half of TRANS-08.
//
// The suite in contract_test.go was written backend-parametrised before it was
// known what the second backend would be (D-26), so this file adds a transport
// and no assertions: the same nine scenario bodies run against a Talos node
// that is actually running.
//
// It is driven by two environment variables rather than by bringing the
// cluster up itself, and that is the module boundary doing its job (D-28). The
// provisioner lives in `sandbox/`, a separate Go module, because it depends on
// the Talos *root* module -- containerd, CNI, a large part of an operating
// system -- and `internal/depguard_test.go` asserts none of that ever reaches
// the product binary. What crosses the boundary here is an endpoint and a
// talosconfig: data, not a dependency.
//
//	sandbox/ $ go run ./cmd/talos-sandbox up
//	          # then run the printed command
//
// When the variables are absent the run is **skipped and says so** (D-27). A
// skip that announces itself is more honest than a suite that quietly pretends
// there is no second backend -- and fake drift is the risk every later phase
// carries, so success criterion 5 is met only once this has run green
// somewhere. `.planning/WINDOWS.md` is where an unexecuted verification is
// recorded.

import (
	"os"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

const (
	// envEndpoint is the address of a running Talos node, without a port.
	envEndpoint = "HOLZKUBE_CONTRACT_ENDPOINT"

	// envTalosconfig is the path to a talosconfig that can reach it.
	//
	// A path, not the bytes, and only here: this is a test reading a file the
	// person running it just made. The product itself never accepts a path --
	// store.go's rule about os.ReadFile outside fsstore, and D-06's rule about
	// the adoption body, both stand.
	envTalosconfig = "HOLZKUBE_CONTRACT_TALOSCONFIG"
)

// inducibleOnRealTalos is the honest answer to "which of the nine faults can
// be provoked on a node nobody is simulating".
//
// The two that can are the two whose fault is the node's own documented
// behaviour rather than something the simulator does to it: a second bootstrap
// really is refused with AlreadyExists, and an invalid configuration really is
// rejected with the node unaffected (PITFALLS P11). The other seven need
// somebody to cut a connection, remove a resource or change an address, and a
// test that faked them here would be testing the faking.
//
// They are skipped by name rather than dropped, so the output of a Tier-1 run
// says which of the nine it actually covered. A run that silently covered two
// and reported nine would be worse than no run.
var inducibleOnRealTalos = map[talossim.ScenarioName]bool{
	talossim.ScenarioSecondBootstrapAlreadyExists: true,
	talossim.ScenarioRejectApply:                  true,
}

// realTransport reaches a Talos node that is actually running.
var realTransport = contractTransport{
	Name: "real-talos",
	New: func(t *testing.T, _ talossim.Options) nodeUnderTest {
		t.Helper()

		endpoint := os.Getenv(envEndpoint)
		raw, err := os.ReadFile(os.Getenv(envTalosconfig)) //nolint:gosec // a test reading the file the operator just pointed it at
		if err != nil {
			t.Fatalf("read %s: %v", envTalosconfig, err)
		}

		tc, err := talos.ParseTalosconfig(raw)
		if err != nil {
			t.Fatalf("parse talosconfig: %v", err)
		}
		creds, err := tc.Creds()
		if err != nil {
			t.Fatalf("build credentials: %v", err)
		}

		return nodeUnderTest{
			Dialer: talos.NewDirectDialer(talos.ApidPort),
			Creds:  creds,
			Target: talos.Target{
				// The UUID is not known before the node is asked, and nothing
				// in this suite needs it to be the real one: Target.Machine is
				// identity for holzkube-manager's own bookkeeping, and the dialer reaches
				// the node through Addr.
				Machine: model.MachineID("00000000-0000-0000-0000-0000000000e2"),
				Addr:    endpoint,
			},
			Inject: func(t *testing.T, sc talossim.Scenario) func() {
				t.Helper()

				if !inducibleOnRealTalos[sc.Name] {
					t.Skipf("%s cannot be induced on a real node without simulating it, "+
						"which would be testing the simulation; covered at Tier 0 only", sc.Name)
				}
				// The two that are inducible are inducible by the assertion
				// itself -- it bootstraps twice, or applies an invalid
				// configuration -- so there is nothing to inject and nothing to
				// restore.
				return func() {}
			},
			// Nil on purpose: a real node keeps no arrival counter, and every
			// assertion that uses one checks for this first. That is what lets
			// the suite degrade to what real Talos can prove rather than fail
			// against it.
			Calls: nil,
			Sim:   nil,
		}
	},
}

// TestScenarioContractAgainstRealTalos is success criterion 5's second half.
func TestScenarioContractAgainstRealTalos(t *testing.T) {
	t.Parallel()

	endpoint, hasEndpoint := os.LookupEnv(envEndpoint)
	config, hasConfig := os.LookupEnv(envTalosconfig)

	if !hasEndpoint || !hasConfig || endpoint == "" || config == "" {
		t.Skipf("Tier 1 not available: %s and %s are not both set.\n\n"+
			"Bring a throwaway cluster up with:\n"+
			"  cd sandbox && go run ./cmd/talos-sandbox up\n"+
			"and run the command it prints. A skipped Tier-1 run is recorded in "+
			".planning/WINDOWS.md; it is not the same as a green one.",
			envEndpoint, envTalosconfig)
	}

	runScenarioContract(t, realTransport)
}
