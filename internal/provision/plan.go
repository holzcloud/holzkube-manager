package provision

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// What the wizard shows before anything is written, and what it refuses.

// ErrWrongMachine reports that the machine at an address is not the one the
// plan was made for (PROV-05).
//
// It is the last check before the configuration is applied, and it is the one
// that matters most: hitting the wrong machine wipes it. Between the operator
// reading a wizard and clicking apply, a DHCP lease can move and a machine can
// reboot into something else.
var ErrWrongMachine = errors.New("provision: the machine at this address is not the one this plan is for")

// ErrNotInMaintenance reports a machine that is not waiting for a
// configuration.
var ErrNotInMaintenance = errors.New("provision: the machine at this address is not in maintenance mode")

// Candidate is everything the wizard shows about a machine before the apply
// (PROV-03).
type Candidate struct {
	Addr string          `json:"addr"`
	UUID model.MachineID `json:"uuid"`

	TalosVersion string `json:"talos_version"`
	Hostname     string `json:"hostname,omitempty"`

	// Fingerprint is what this machine presented. See MaintenanceWarning for
	// what it is and is not worth.
	Fingerprint string `json:"fingerprint,omitempty"`

	// MACs and Disks are how a person tells one identical mini-PC from
	// another. The UUID is the key holzkube-manager uses; the MAC is the one an
	// operator can read off a label.
	MACs  []string `json:"macs,omitempty"`
	Disks []Disk   `json:"disks"`

	// Warnings are the things that are true of this machine right now and will
	// otherwise produce a failure that looks like something else.
	Warnings []string `json:"warnings,omitempty"`
}

// Disk is one candidate install target, with everything the picker shows
// (PROV-06).
type Disk struct {
	Device     string `json:"device"`
	Size       uint64 `json:"size"`
	PrettySize string `json:"pretty_size,omitempty"`
	Model      string `json:"model,omitempty"`
	Serial     string `json:"serial,omitempty"`
	Transport  string `json:"transport,omitempty"`

	// System marks the disk Talos is or would be installed on. It is shown
	// rather than pre-selected: on a machine that has been installed before,
	// choosing it is the ordinary thing and on a fresh one it means nothing.
	System bool `json:"system"`
}

// Inspect reads a machine in maintenance mode.
func Inspect(ctx context.Context, d talos.Dialer, addr string, creds talos.Creds) (Candidate, error) {
	target := talos.Target{Machine: model.MachineID("candidate:" + addr), Addr: addr}

	mc, err := talos.NewMaintenanceClient(ctx, d, target, creds, talos.Mode{})
	if err != nil {
		return Candidate{}, err
	}
	defer mc.Close() //nolint:errcheck // the inspection's verdict is its own

	facts, err := mc.NodeFacts(ctx)
	if err != nil {
		return Candidate{}, err
	}

	c := Candidate{
		Addr:         addr,
		UUID:         facts.UUID,
		TalosVersion: facts.TalosVersion,
		Hostname:     facts.Hostname,
	}
	for _, link := range facts.Links {
		if link.HardwareAddr != "" && link.Kind != "loopback" {
			c.MACs = append(c.MACs, link.HardwareAddr)
		}
	}

	version, err := mc.Version(ctx)
	if err == nil && version != "" {
		c.TalosVersion = version
	}

	disks, err := mc.Disks(ctx)
	if err != nil {
		return Candidate{}, err
	}
	for _, d := range disks {
		c.Disks = append(c.Disks, Disk{
			Device:     d.Device,
			Size:       d.Size,
			PrettySize: prettySize(d.Size),
			Model:      d.Model,
			Serial:     d.Serial,
			Transport:  d.Type,
			System:     d.System,
		})
	}

	if fp, err := talos.ServerFingerprint(ctx, d, target); err == nil {
		c.Fingerprint = fp
	}

	// The two warnings that are about how this machine boots rather than about
	// anything holzkube-manager does, and that each produce a failure that looks like
	// something else (PROV-12).
	c.Warnings = append(c.Warnings, ShadowedISOWarning, NoDHCPWarning)

	for _, d := range c.Disks {
		if d.System {
			c.Warnings = append(c.Warnings, fmt.Sprintf(
				"%s is marked as this machine's system disk, which means Talos has been installed "+
					"on it before. Installing to it replaces that installation.", d.Device))
		}
	}

	return c, nil
}

// Request is a provisioning plan as the operator assembled it.
type Request struct {
	Addr string          `json:"addr"`
	UUID model.MachineID `json:"uuid"`

	Cluster      model.ClusterID `json:"cluster"`
	ControlPlane bool            `json:"control_plane"`

	// InstallDisk is the device Talos installs to, e.g. "/dev/nvme0n1".
	InstallDisk string `json:"install_disk"`

	// SchematicID is the Image Factory schematic. It is the *same* id as the
	// ISO the machine booted, and `.machine.install.image` is built from it --
	// see InstallImage for why that is not optional (PROV-08).
	SchematicID string `json:"schematic_id"`

	// TalosVersion is the version to install, and the version the install
	// image is built for.
	TalosVersion string `json:"talos_version"`

	// Fingerprint, when set, pins the machine's certificate (PROV-04).
	Fingerprint string `json:"fingerprint,omitempty"`

	// Hostname is what the node will call itself.
	Hostname string `json:"hostname,omitempty"`

	// PatchIDs are stored patches applied to the generated configuration.
	PatchIDs []string `json:"patch_ids,omitempty"`
}

// Validate checks a request against the cluster it is for.
//
// controlPlaneCount is how many control-plane nodes the cluster already has,
// so the even-count warning can be about the cluster as it will be rather than
// as it is.
func (r Request) Validate(controlPlaneCount int) (warnings []string, err error) {
	if r.Addr == "" {
		return nil, errors.New("provision: name the machine's address")
	}
	if r.UUID == "" {
		return nil, errors.New("provision: the plan must name the machine's UUID; " +
			"it is what the check immediately before the apply compares against")
	}
	if r.Cluster == "" {
		return nil, errors.New("provision: name the cluster this machine joins")
	}
	if r.InstallDisk == "" {
		return nil, errors.New("provision: choose the disk Talos installs to")
	}
	if r.TalosVersion == "" {
		return nil, errors.New("provision: name the Talos version to install")
	}
	if r.SchematicID == "" {
		// Not fatal: a machine booted from a stock ISO has no schematic, and
		// refusing would make the ordinary case impossible. But it is said out
		// loud, because the consequence arrives later and looks like something
		// else.
		warnings = append(warnings, "This plan names no Image Factory schematic, so the installed "+
			"system will be stock Talos. If the machine booted from an ISO with system extensions, "+
			"those extensions will not be on the installed system.")
	}

	if r.ControlPlane {
		after := controlPlaneCount + 1
		if after%2 == 0 {
			warnings = append(warnings, fmt.Sprintf(
				"This would make %d control-plane nodes. etcd needs an odd number to have a "+
					"quorum -- 1, 3 or 5 -- and an even number gives you the failure tolerance of "+
					"the odd number below it while costing an extra machine.", after))
		}
	}

	return warnings, nil
}

// InstallImage is the installer reference `.machine.install.image` gets
// (PROV-08).
//
// It is derived from the *same* schematic id as the ISO, and that is the whole
// point of the function existing rather than the caller writing the string.
// A machine that boots an ISO with system extensions and then installs a stock
// installer comes up without them -- the install succeeds, the node joins, and
// the extensions are simply gone. Nothing reports it.
func InstallImage(schematicID, talosVersion string) string {
	if schematicID == "" {
		return "ghcr.io/siderolabs/installer:" + talosVersion
	}
	return "factory.talos.dev/installer/" + schematicID + ":" + talosVersion
}

// VerifyMachine is PROV-05: the identity check immediately before the write.
//
// It reconnects, reads the UUID, and refuses if it is not the one the plan
// names. The reconnection is the point -- checking against the identity read
// during the wizard would be checking a memory rather than the machine.
func VerifyMachine(ctx context.Context, d talos.Dialer, addr string, want model.MachineID, creds talos.Creds) error {
	target := talos.Target{Machine: model.MachineID("verify:" + addr), Addr: addr}

	mc, err := talos.NewMaintenanceClient(ctx, d, target, creds, talos.Mode{})
	if err != nil {
		return err
	}
	defer mc.Close() //nolint:errcheck // the verdict is the read's

	facts, err := mc.NodeFacts(ctx)
	if err != nil {
		return err
	}

	if facts.UUID != want {
		return fmt.Errorf("%w: %s answers with UUID %s, and this plan is for %s. "+
			"Nothing has been written. A DHCP lease moving between two machines produces exactly "+
			"this, and applying anyway would wipe the wrong one",
			ErrWrongMachine, addr, facts.UUID, want)
	}
	return nil
}

// ReappearBudget is how long a machine is expected to take between accepting
// its configuration and answering as a cluster node.
//
// It is an expectation and not a timeout: the probe below reports elapsed time
// against it rather than failing at it, because a machine that takes longer
// than expected has not failed, it is slow -- and the operator needs to know
// which.
//
// The number is a placeholder until phase 4's measurement replaces it, and it
// is written here rather than guessed at each call site so that replacing it is
// one edit. See `.planning/phases/04-walking-skeleton/04-SUMMARY.md`.
const ReappearBudget = 8 * time.Minute

// ReappearState is one of the three answers the probe can give (PROV-09).
type ReappearState string

const (
	// ReappearInstalling is the machine still doing what it was told: it does
	// not answer, and it has not been longer than expected.
	ReappearInstalling ReappearState = "installing"

	// ReappearOverdue is the machine still not answering past the budget.
	// It is not a failure -- an install on a slow disk takes longer -- but it
	// is the moment an operator should look at the console.
	ReappearOverdue ReappearState = "overdue"

	// ReappearBack is the machine answering as a configured node.
	ReappearBack ReappearState = "back"

	// ReappearStillMaintenance is the machine answering and still in
	// maintenance mode, which means the configuration did not take.
	//
	// It is its own state because it is the one outcome that will never
	// resolve itself by waiting, and a progress display that showed it as
	// "installing" would wait forever.
	ReappearStillMaintenance ReappearState = "still-in-maintenance"
)

// Reappearance is the probe's answer.
//
// It is a state plus two numbers rather than a spinner, and that is PROV-09
// stated as a type: a spinner says "something is happening" and cannot say
// "this has taken twice as long as it should".
type Reappearance struct {
	State   ReappearState `json:"state"`
	Elapsed time.Duration `json:"elapsed_seconds"`
	Budget  time.Duration `json:"budget_seconds"`

	// Sentence is what the screen says, including the two numbers.
	Sentence string `json:"sentence"`
}

// Probe asks whether a machine has come back, and how long it has been.
func Probe(
	ctx context.Context,
	d talos.Dialer,
	addr string,
	uuid model.MachineID,
	clusterCreds talos.Creds,
	maintenanceCreds talos.Creds,
	since time.Time,
) Reappearance {
	elapsed := time.Since(since)
	r := Reappearance{Elapsed: elapsed, Budget: ReappearBudget}

	target := talos.Target{Cluster: "", Machine: uuid, Addr: addr}

	// The cluster credentials first: answering under them is the outcome that
	// was wanted, and asking about it first means the good case is not behind
	// a probe that is expected to fail.
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	if cc, err := talos.NewClusterClient(probeCtx, d, target, clusterCreds, talos.Mode{}); err == nil {
		defer cc.Close() //nolint:errcheck // the probe's verdict is its own
		if _, err := cc.Probe(probeCtx); err == nil {
			r.State = ReappearBack
			r.Sentence = fmt.Sprintf("The machine is answering as a cluster node, %s after it "+
				"accepted its configuration.", round(elapsed))
			return r
		}
	}

	// Still in maintenance mode? That never resolves itself by waiting.
	if mc, err := talos.NewMaintenanceClient(probeCtx, d, target, maintenanceCreds, talos.Mode{}); err == nil {
		defer mc.Close() //nolint:errcheck // as above
		if _, err := mc.Version(probeCtx); err == nil {
			r.State = ReappearStillMaintenance
			r.Sentence = "The machine is answering, and it is still in maintenance mode. " +
				"The configuration did not take. Waiting will not change that."
			return r
		}
	}

	if elapsed > ReappearBudget {
		r.State = ReappearOverdue
		r.Sentence = fmt.Sprintf(
			"The machine has not answered for %s. An install usually takes about %s. "+
				"It has not failed -- a slow disk takes longer -- but this is the point at which "+
				"the machine's own console is worth looking at.",
			round(elapsed), round(ReappearBudget))
		return r
	}

	r.State = ReappearInstalling
	r.Sentence = fmt.Sprintf("Installing. %s elapsed of about %s expected.",
		round(elapsed), round(ReappearBudget))
	return r
}

// CNINotice is PROV-13.
//
// A node that has installed, joined and is running Talos correctly reports
// Kubernetes as NotReady until a CNI is installed. It is the normal state of a
// freshly provisioned cluster, it lasts until somebody applies a network
// plugin, and shown in red it is the single most common reason to think a
// working provisioning run failed.
const CNINotice = "Talos is healthy and Kubernetes reports NotReady. Before a CNI (a network " +
	"plugin such as Cilium or Flannel) is installed, that is what a correctly provisioned node " +
	"looks like -- the kubelet is running and has no pod network yet. It is not a failure of this " +
	"provisioning run."

func round(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

// prettySize renders a byte count the way Talos does.
func prettySize(n uint64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}
