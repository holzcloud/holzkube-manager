package provision_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/provision"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestAConfiguredNodeIsReportedAsConfigured is PROV-02, and it is the finding
// this whole scan exists to get right.
//
// "Nothing found" and "there is already a node here" lead to opposite actions.
// A scan that conflates them sends the operator to check a cable while a
// running node sits at the address they were about to overwrite.
func TestAConfiguredNodeIsReportedAsConfigured(t *testing.T) {
	t.Parallel()

	configured, err := talossim.New(talossim.Options{Hostname: "already-a-node"})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = configured.Close() })

	s := provision.NewScanner(talos.NewDirectDialer(configured.Port()), nil)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	found, err := s.Scan(ctx, provision.ScanRequest{Addrs: []string{configured.Host()}})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("the scan found %d machines at an address where one is running, want 1", len(found))
	}
	if found[0].State != provision.StateConfigured {
		t.Fatalf("a configured node is reported as %q, want %q",
			found[0].State, provision.StateConfigured)
	}
	if found[0].Detail == "" {
		t.Error("the finding carries no explanation of what it means")
	}
	if found[0].Fingerprint == "" {
		t.Error("the finding carries no certificate fingerprint to compare against the console")
	}
}

// TestAMachineInMaintenanceModeIsTheOneTheWizardWants is the other half.
func TestAMachineInMaintenanceModeIsTheOneTheWizardWants(t *testing.T) {
	t.Parallel()

	blank, err := talossim.New(talossim.Options{Hostname: "blank", Maintenance: true})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = blank.Close() })

	s := provision.NewScanner(talos.NewDirectDialer(blank.Port()), nil)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	found, err := s.Scan(ctx, provision.ScanRequest{Addrs: []string{blank.Host()}})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(found) != 1 || found[0].State != provision.StateMaintenance {
		t.Fatalf("a machine in maintenance mode was reported as %+v", found)
	}
}

// TestAnAddressWithNothingAtItIsSimplyAbsent is the one place "nothing" is a
// valid report, and it is about an address rather than about a machine.
func TestAnAddressWithNothingAtItIsSimplyAbsent(t *testing.T) {
	t.Parallel()

	sim, err := talossim.New(talossim.Options{Hostname: "somewhere-else"})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	s := provision.NewScanner(talos.NewDirectDialer(sim.Port()), nil)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// An address nothing is listening on. The simulator is on loopback at a
	// port; this asks a different loopback address, so the dial fails.
	found, err := s.Scan(ctx, provision.ScanRequest{Addrs: []string{"127.0.0.2"}})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("an address with nothing at it produced %+v", found)
	}
}

// TestAKnownMachineIsMarkedAsKnown keeps the wizard from offering to provision
// a machine that is already in the inventory as though it were new.
func TestAKnownMachineIsMarkedAsKnown(t *testing.T) {
	t.Parallel()

	sim, err := talossim.New(talossim.Options{Hostname: "known-one"})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	s := provision.NewScanner(talos.NewDirectDialer(sim.Port()),
		func(context.Context, string) bool { return true })

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	found, err := s.Scan(ctx, provision.ScanRequest{Addrs: []string{sim.Host()}})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(found) != 1 || !found[0].Known {
		t.Fatalf("a machine already in the inventory was not marked as known: %+v", found)
	}
}

// TestAnUnreasonableSubnetIsRefusedWithAReason keeps a typo from becoming a
// scan that never finishes.
func TestAnUnreasonableSubnetIsRefusedWithAReason(t *testing.T) {
	t.Parallel()

	s := provision.NewScanner(talos.NewDirectDialer(talos.ApidPort), nil)

	for _, cidr := range []string{"10.0.0.0/8", "0.0.0.0/0"} {
		_, err := s.Scan(t.Context(), provision.ScanRequest{CIDR: cidr})
		if err == nil {
			t.Fatalf("Scan(%q) was accepted", cidr)
		}
		if !strings.Contains(err.Error(), "addresses") {
			t.Errorf("the refusal for %q reads %q, which does not say how big it is", cidr, err)
		}
	}

	if _, err := s.Scan(t.Context(), provision.ScanRequest{CIDR: "2001:db8::/64"}); err == nil {
		t.Error("an IPv6 subnet scan was accepted")
	}
}

// TestTheInstallImageComesFromTheSchematic is PROV-08.
//
// A machine that boots an ISO with system extensions and then installs a stock
// installer comes up without them. The install succeeds, the node joins, the
// extensions are gone, and nothing reports it.
func TestTheInstallImageComesFromTheSchematic(t *testing.T) {
	t.Parallel()

	const schematic = "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"

	withSchematic := provision.InstallImage(schematic, "v1.13.9")
	if !strings.Contains(withSchematic, schematic) {
		t.Fatalf("the install image %q does not name the schematic the ISO was built from",
			withSchematic)
	}
	if !strings.Contains(withSchematic, "factory.talos.dev") {
		t.Errorf("the install image %q is not a Factory reference", withSchematic)
	}

	// A machine booted from a stock ISO has no schematic, and refusing would
	// make the ordinary case impossible.
	stock := provision.InstallImage("", "v1.13.9")
	if strings.Contains(stock, "factory.talos.dev") {
		t.Errorf("a plan with no schematic produced a Factory reference: %q", stock)
	}
}

// TestAnEvenControlPlaneCountIsWarnedAbout is PROV-07.
func TestAnEvenControlPlaneCountIsWarnedAbout(t *testing.T) {
	t.Parallel()

	base := provision.Request{
		Addr:           "10.0.0.5",
		UUID:           testMachine,
		Cluster:        testCluster,
		InstallDisk:    "/dev/nvme0n1",
		TalosVersion:   "v1.13.9",
		SchematicID:    "abc",
		InstallerImage: "factory.example/metal-installer/abc:v1.13.9",
		ControlPlane:   true,
	}

	// One existing control-plane node, so this would make two.
	warnings, err := base.Validate(1)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	var warned bool
	for _, w := range warnings {
		if strings.Contains(w, "quorum") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("adding a second control-plane node produced no quorum warning: %v", warnings)
	}

	// Two existing, so this would make three, which is what etcd wants.
	warnings, err = base.Validate(2)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	for _, w := range warnings {
		if strings.Contains(w, "quorum") {
			t.Errorf("making a third control-plane node produced a quorum warning: %q", w)
		}
	}
}

// TestAPlanWithoutAnInstallerReferenceIsRefused.
//
// The reference is resolved against the Image Factory when the plan is made
// and carried from there, so a request arriving here without one is a request
// that would install whatever a default produced. It used to be assembled from
// parts at install time, which is how the string that was confirmed and the
// string that was written came to be able to differ.
func TestAPlanWithoutAnInstallerReferenceIsRefused(t *testing.T) {
	t.Parallel()

	req := provision.Request{
		Addr:         "10.0.0.5",
		UUID:         testMachine,
		Cluster:      testCluster,
		InstallDisk:  "/dev/nvme0n1",
		TalosVersion: "v1.13.9",
		SchematicID:  "abc",
	}
	_, err := req.Validate(0)
	if err == nil {
		t.Fatal("a plan with no installer reference was accepted")
	}
	if !strings.Contains(err.Error(), "installer reference") {
		t.Errorf("the refusal does not name what is missing: %v", err)
	}

	req.InstallerImage = "factory.example/metal-installer/abc:v1.13.9"
	if _, err := req.Validate(0); err != nil {
		t.Errorf("a complete plan was refused: %v", err)
	}
}

// TestAPlanWithoutItsIdentityIsRefused pins the field PROV-05's check compares
// against: without it there is nothing to verify and the check is decoration.
func TestAPlanWithoutItsIdentityIsRefused(t *testing.T) {
	t.Parallel()

	incomplete := []provision.Request{
		{Addr: "10.0.0.5", Cluster: testCluster, InstallDisk: "/dev/sda", TalosVersion: "v1.13.9", InstallerImage: "i"},
		{UUID: testMachine, Cluster: testCluster, InstallDisk: "/dev/sda", TalosVersion: "v1.13.9", InstallerImage: "i"},
		{Addr: "10.0.0.5", UUID: testMachine, InstallDisk: "/dev/sda", TalosVersion: "v1.13.9", InstallerImage: "i"},
		{Addr: "10.0.0.5", UUID: testMachine, Cluster: testCluster, TalosVersion: "v1.13.9"},
		{Addr: "10.0.0.5", UUID: testMachine, Cluster: testCluster, InstallDisk: "/dev/sda"},
	}
	for i, req := range incomplete {
		if _, err := req.Validate(0); err == nil {
			t.Errorf("incomplete plan %d was accepted: %+v", i, req)
		}
	}
}

// TestTheReappearanceProbeReportsTimeAgainstABudget is PROV-09: never a
// spinner.
//
// A spinner says "something is happening" and cannot say "this has taken twice
// as long as it should" -- which is the only thing the operator needs from it.
func TestTheReappearanceProbeReportsTimeAgainstABudget(t *testing.T) {
	t.Parallel()

	sim, err := talossim.New(talossim.Options{Hostname: "installing"})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	d := talos.NewDirectDialer(sim.Port())

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// The machine is answering as a configured node, which is the outcome that
	// was wanted.
	r := provision.Probe(ctx, d, sim.Host(), testMachine,
		sim.ClientCreds(), sim.MaintenanceCreds(), time.Now().Add(-30*time.Second))
	if r.State != provision.ReappearBack {
		t.Fatalf("a node that answers under cluster credentials was reported as %q", r.State)
	}
	if r.Sentence == "" {
		t.Error("the probe carries no sentence")
	}

	// An address with nothing at it, started a moment ago: still installing.
	r = provision.Probe(ctx, talos.NewDirectDialer(1), "127.0.0.2", testMachine,
		sim.ClientCreds(), sim.MaintenanceCreds(), time.Now())
	if r.State != provision.ReappearInstalling {
		t.Errorf("a machine that has just started installing was reported as %q", r.State)
	}
	if !strings.Contains(r.Sentence, "elapsed") {
		t.Errorf("the sentence %q does not report elapsed time against the budget", r.Sentence)
	}

	// The same address, long past the budget: overdue, and not failed.
	r = provision.Probe(ctx, talos.NewDirectDialer(1), "127.0.0.2", testMachine,
		sim.ClientCreds(), sim.MaintenanceCreds(), time.Now().Add(-2*provision.ReappearBudget))
	if r.State != provision.ReappearOverdue {
		t.Errorf("a machine long past its budget was reported as %q", r.State)
	}
	if !strings.Contains(r.Sentence, "console") {
		t.Errorf("the overdue sentence %q does not say what to do next", r.Sentence)
	}
}

// TestAMachineStillInMaintenanceNeverResolvesByWaiting is the state that would
// otherwise be a progress bar running forever.
func TestAMachineStillInMaintenanceNeverResolvesByWaiting(t *testing.T) {
	t.Parallel()

	blank, err := talossim.New(talossim.Options{Hostname: "did-not-take", Maintenance: true})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = blank.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// The cluster credentials will not work against a maintenance-mode node,
	// which is exactly the situation: the configuration did not take.
	r := provision.Probe(ctx, talos.NewDirectDialer(blank.Port()), blank.Host(), testMachine,
		blank.MaintenanceCreds(), blank.MaintenanceCreds(), time.Now())

	if r.State != provision.ReappearStillMaintenance {
		t.Fatalf("a machine still in maintenance mode was reported as %q; a progress display "+
			"showing that as 'installing' would wait forever", r.State)
	}
	if !strings.Contains(r.Sentence, "will not change") {
		t.Errorf("the sentence %q does not say that waiting will not help", r.Sentence)
	}
}
