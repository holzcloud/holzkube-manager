package provision_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/provision"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// blankMachine is a simulated machine waiting for a configuration: the state
// the wizard is written for.
func blankMachine(t *testing.T, disks ...talossim.DiskFixture) (*talossim.Server, talos.Dialer) {
	t.Helper()

	if len(disks) == 0 {
		disks = []talossim.DiskFixture{
			{Device: "nvme0n1", Size: 512 << 30, Model: "SIMULATED NVMe", Serial: "SN-0001", Transport: "nvme"},
			{Device: "sda", Size: 1 << 40, Model: "SIMULATED SATA", Serial: "SN-0002", Transport: "sata"},
		}
	}

	sim, err := talossim.New(talossim.Options{
		Hostname:    "blank-1",
		Maintenance: true,
		Disks:       disks,
	})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	return sim, talos.NewDirectDialer(sim.Port())
}

// TestTheWizardShowsWhatIdentifiesTheMachine is PROV-03 and PROV-06.
//
// Two identical mini-PCs on a shelf are told apart by a UUID nobody can read
// and a MAC address printed on a label. A wizard that shows neither is a wizard
// that asks the operator to guess which machine is about to be wiped.
func TestTheWizardShowsWhatIdentifiesTheMachine(t *testing.T) {
	t.Parallel()

	sim, d := blankMachine(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	c, err := provision.Inspect(ctx, d, sim.Host(), sim.MaintenanceCreds())
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if c.UUID == "" {
		t.Error("the candidate carries no UUID, which is the key everything downstream is about")
	}
	if c.TalosVersion == "" {
		t.Error("the candidate carries no Talos version")
	}
	if len(c.MACs) == 0 {
		t.Error("the candidate carries no MAC address, which is the one identifier printed on a label")
	}
	if c.Fingerprint == "" {
		t.Error("the candidate carries no certificate fingerprint to compare against the console (PROV-04)")
	}

	// PROV-06: every column the disk picker shows, because "512 GB" on a
	// machine with two 512 GB disks is not a choice.
	if len(c.Disks) != 2 {
		t.Fatalf("the picker offers %d disks, want 2: %+v", len(c.Disks), c.Disks)
	}
	for _, disk := range c.Disks {
		switch {
		case disk.Device == "":
			t.Errorf("a disk has no device name: %+v", disk)
		case disk.Size == 0:
			t.Errorf("%s has no size", disk.Device)
		case disk.PrettySize == "":
			t.Errorf("%s has no human-readable size", disk.Device)
		case disk.Model == "":
			t.Errorf("%s has no model", disk.Device)
		case disk.Serial == "":
			t.Errorf("%s has no serial number, which is what tells two identical disks apart", disk.Device)
		case disk.Transport == "":
			t.Errorf("%s has no transport", disk.Device)
		}
	}

	// PROV-12: both warnings, at the start, because each produces a failure
	// that looks like something else.
	joined := strings.Join(c.Warnings, "\n")
	if !strings.Contains(joined, "boot from the disk") {
		t.Errorf("the wizard does not warn that an existing installation shadows the ISO: %v", c.Warnings)
	}
	if !strings.Contains(joined, "no DHCP") {
		t.Errorf("the wizard does not warn that there is no zero-touch path without DHCP: %v", c.Warnings)
	}
}

// TestTheMachineIsIdentifiedAgainImmediatelyBeforeTheWrite is PROV-05, and it
// is the check that stops this product from wiping a machine nobody asked it
// to touch.
//
// Between the operator reading the wizard and clicking apply, a DHCP lease can
// move. The identity read during the wizard is then a memory, and the machine
// at the address is somebody else's.
func TestTheMachineIsIdentifiedAgainImmediatelyBeforeTheWrite(t *testing.T) {
	t.Parallel()

	sim, d := blankMachine(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	c, err := provision.Inspect(ctx, d, sim.Host(), sim.MaintenanceCreds())
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if err := provision.VerifyMachine(ctx, d, sim.Host(), c.UUID, sim.MaintenanceCreds()); err != nil {
		t.Fatalf("the machine the plan is for did not verify: %v", err)
	}

	// The same address, a different machine. This is the lease that moved.
	const somebodyElse = model.MachineID("00000000-0000-4000-8000-00000000dead")
	err = provision.VerifyMachine(ctx, d, sim.Host(), somebodyElse, sim.MaintenanceCreds())
	if !errors.Is(err, provision.ErrWrongMachine) {
		t.Fatalf("applying to a different machine than the plan names returned %v, want ErrWrongMachine", err)
	}
	for _, want := range []string{string(somebodyElse), string(c.UUID), "Nothing has been written"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not mention %q", err, want)
		}
	}
}

// TestAConfiguredMachineIsNotInspectedAsACandidate keeps the wizard from
// reading a running node as though it were blank. Maintenance credentials are
// no credentials at all, and a configured node does not answer to them.
func TestAConfiguredMachineIsNotInspectedAsACandidate(t *testing.T) {
	t.Parallel()

	sim, err := talossim.New(talossim.Options{Hostname: "already-a-node"})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	if _, err := provision.Inspect(ctx, talos.NewDirectDialer(sim.Port()),
		sim.Host(), sim.MaintenanceCreds()); err == nil {
		t.Fatal("a configured node was inspected as a provisioning candidate")
	}
}

// TestTheCNINoticeIsNotAFailure is PROV-13, pinned as text because it is the
// single most common reason to think a working provisioning run failed.
func TestTheCNINoticeIsNotAFailure(t *testing.T) {
	t.Parallel()

	for _, want := range []string{"NotReady", "CNI", "not a failure"} {
		if !strings.Contains(provision.CNINotice, want) {
			t.Errorf("the notice does not mention %q: %q", want, provision.CNINotice)
		}
	}
}
