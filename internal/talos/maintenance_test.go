package talos_test

import (
	"context"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/siderolabs/talos/pkg/machinery/resources/hardware"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// compileFailTag is the build tag that admits the fixture file which must not
// compile. The package is built with it exactly once, from
// TestMaintenanceClientRejectsClusterOnlyCall, and never anywhere else.
const compileFailTag = "talos_compile_fail"

// TestMaintenanceClientRejectsClusterOnlyCall is D-06, executed.
//
// The requirement is that cluster and maintenance clients are "nicht
// verwechselbar", and the strong reading of that is a compile error rather than
// a runtime rejection: a runtime rejection is a branch somebody has to reach in
// a test, on a node in a state somebody has to arrange, and it fails in
// production for whoever gets there first. A build failure fails for the person
// writing the line.
//
// The assertion cannot be written as ordinary Go, because a file that does not
// compile does not compile for the test binary either. So the offending call
// lives behind a build tag nothing else sets, and the test is a `go build` with
// that tag whose non-zero exit is the evidence.
func TestMaintenanceClientRejectsClusterOnlyCall(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-tags", compileFailTag, ".")
	out, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("building with -tags %s succeeded; the fixture calls a cluster-only method on a "+
			"*MaintenanceClient and the compiler accepted it, so the two types are not distinct in "+
			"the way D-06 requires", compileFailTag)
	}

	text := string(out)
	t.Logf("go build -tags %s said:\n%s", compileFailTag, strings.TrimSpace(text))

	for _, want := range []string{"MaintenanceClient", "Bootstrap"} {
		if !strings.Contains(text, want) {
			t.Errorf("the build failed, but its output does not mention %q; the failure may be an "+
				"unrelated compile error rather than the assertion this test makes:\n%s", want, text)
		}
	}

	// The negative control. The same fixture calls the same method on a
	// *ClusterClient, where it does exist. If the compiler complained about
	// that line too, the fixture is broken (a typo, a renamed method) and the
	// failure above would be evidence of nothing.
	if strings.Contains(text, "ClusterClient has no field or method") {
		t.Errorf("the fixture's cluster-side control also failed to compile, so the maintenance-side "+
			"failure proves nothing:\n%s", text)
	}
}

// TestMaintenanceClientMethodSetIsClosed is the other half of D-06.
//
// The compile-failure test above proves one named cluster method is absent. It
// would keep passing if somebody added Reset, Upgrade and EtcdLeaveCluster to
// the maintenance type, which is how a small type becomes a large one: one
// method at a time, each individually defensible. So the set is pinned whole,
// and growing it is a deliberate edit here rather than a quiet one there.
//
// Maintenance mode realistically serves ApplyConfiguration, Version, Disks and
// COSI reads and nothing else, so the type serves exactly those, plus Close.
func TestMaintenanceClientMethodSetIsClosed(t *testing.T) {
	t.Parallel()

	want := []string{"ApplyConfiguration", "COSI", "Close", "Disks", "Version"}

	typ := reflect.TypeOf(&talos.MaintenanceClient{})
	var got []string
	for i := range typ.NumMethod() {
		got = append(got, typ.Method(i).Name)
	}
	slices.Sort(got)

	if !slices.Equal(got, want) {
		t.Errorf("MaintenanceClient's exported method set is %v, want %v", got, want)
	}
}

// TestConstructorsRefuseTheWrongCredentialKind is T-02-26.
//
// Two types that both accept either credential kind would be two names for one
// type. The refusal names both the kind it was given and the kind it needs,
// because "wrong credentials" is the error message that sends an operator to
// read the source.
func TestConstructorsRefuseTheWrongCredentialKind(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "creds-node"})
	target := simTarget(sim, "00000000-0000-0000-0000-0000000000c1")

	rows := []struct {
		name  string
		build func(context.Context, talos.Creds) error
		creds talos.Creds
	}{
		{
			name: "maintenance constructor, cluster credentials",
			build: func(ctx context.Context, c talos.Creds) error {
				_, err := talos.NewMaintenanceClient(ctx, sim.Dialer(), target, c, talos.Mode{})
				return err
			},
			creds: talos.Creds{Kind: talos.CredCluster, TLS: sim.ClientCreds().TLS},
		},
		{
			name: "cluster constructor, maintenance credentials",
			build: func(ctx context.Context, c talos.Creds) error {
				_, err := talos.NewClusterClient(ctx, sim.Dialer(), target, c, talos.Mode{})
				return err
			},
			creds: talos.Creds{Kind: talos.CredMaintenance, TLS: sim.ClientCreds().TLS},
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()

			err := row.build(ctx, row.creds)
			if err == nil {
				t.Fatal("the constructor accepted the wrong credential kind")
			}

			msg := err.Error()
			for _, want := range []string{string(talos.CredCluster), string(talos.CredMaintenance)} {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q does not name the credential kind %q; it has to say both what "+
						"it got and what it needs", msg, want)
				}
			}
		})
	}
}

// TestMaintenanceClientAnswersItsMethods drives the whole permitted surface
// against a running node, so that "the method set is small" is a statement
// about methods that work rather than about methods that exist.
func TestMaintenanceClientAnswersItsMethods(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "maint-node", TalosVersion: "v1.13.9"})

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	mc, err := talos.NewMaintenanceClient(ctx, sim.Dialer(),
		simTarget(sim, "00000000-0000-0000-0000-0000000000c2"),
		talos.Creds{Kind: talos.CredMaintenance, TLS: sim.ClientCreds().TLS}, talos.Mode{})
	if err != nil {
		t.Fatalf("NewMaintenanceClient: %v", err)
	}
	t.Cleanup(func() {
		if err := mc.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	version, err := mc.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if version != "v1.13.9" {
		t.Errorf("Version() = %q, want %q", version, "v1.13.9")
	}

	disks, err := mc.Disks(ctx)
	if err != nil {
		t.Fatalf("Disks: %v", err)
	}
	if len(disks) == 0 {
		t.Fatal("Disks() returned nothing; a node with no block devices cannot be installed onto, " +
			"which is the one thing maintenance mode exists for")
	}
	if disks[0].Device == "" || disks[0].Size == 0 {
		t.Errorf("Disks()[0] = %+v: the fields an installer picks a disk by are empty", disks[0])
	}

	applied, err := mc.ApplyConfiguration(ctx, []byte("version: v1alpha1\n"))
	if err != nil {
		t.Fatalf("ApplyConfiguration: %v", err)
	}
	if applied.Details == "" {
		t.Error("ApplyConfiguration returned no detail; the node's own account of what it did is the " +
			"only thing distinguishing a real apply from a validated one")
	}

	// The COSI read accessor. hardware.SystemInformation is seeded by the
	// simulator at construction, so a not-found here means the accessor is
	// wired to the wrong connection rather than that the resource is missing.
	sysInfo, err := safe.StateGetByID[*hardware.SystemInformation](ctx, mc.COSI(),
		hardware.SystemInformationID)
	if err != nil {
		t.Fatalf("COSI read of hardware.SystemInformation: %v", err)
	}
	if sysInfo.Metadata().ID() != resource.ID(hardware.SystemInformationID) {
		t.Errorf("COSI returned %q, want %q", sysInfo.Metadata().ID(), hardware.SystemInformationID)
	}
}

// simTarget is the identity of a simulated node, addressed the way the direct
// dialer would address it.
func simTarget(sim *talossim.Server, id string) talos.Target {
	return talos.Target{
		Machine: model.MachineID(id),
		Addr:    sim.Host(),
	}
}

// newClusterClient builds a cluster client against a simulated node and closes
// it when the test ends.
func newClusterClient(t *testing.T, sim *talossim.Server) *talos.ClusterClient {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cc, err := talos.NewClusterClient(ctx, sim.Dialer(),
		simTarget(sim, "00000000-0000-0000-0000-0000000000cc"), sim.ClientCreds(), talos.Mode{})
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}
	t.Cleanup(func() {
		if err := cc.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return cc
}
