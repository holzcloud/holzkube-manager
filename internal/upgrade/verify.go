package upgrade

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/provision"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"gopkg.in/yaml.v3"
)

// The three checks that are about what a node *is* rather than about what the
// cluster can survive.

// ErrUnknownSchematic reports a node whose Image Factory schematic is not
// known (UPG-03).
//
// It blocks the upgrade. The failure it prevents is silent and permanent: an
// upgrade to a stock installer on a node that was built from a Factory image
// succeeds, the node comes back, joins, reports healthy -- and every system
// extension it had is gone. Nothing reports that. The operator finds out when
// whatever needed the extension stops working, which may be months later and
// will not look like an upgrade.
var ErrUnknownSchematic = errors.New("upgrade: this node's Image Factory schematic is not known")

// ErrKernelArgDrift reports that a node's kernel arguments are not what its
// machine configuration says (UPG-04).
var ErrKernelArgDrift = errors.New("upgrade: this node's kernel arguments differ from its configuration")

// ErrNotUpgraded reports that a node came back running something other than
// what was installed (UPG-07).
var ErrNotUpgraded = errors.New("upgrade: the node is not running the version that was installed")

// RollbackNotice is the remedy every not-upgraded verdict carries.
//
// UPG-07 exists to tell the operator that an upgrade did not take. Saying so
// and stopping there hands somebody a broken node at the moment they are least
// able to go and research what to do about it -- and Talos has an answer that
// this product does not implement, which is the worst combination to leave
// unstated.
//
// It rides on the error rather than living in the interface because the job
// screen renders a step's detail verbatim, so the error *is* the screen here.
// That is the same reason talos.SnapshotNotice is appended to the snapshot's
// error rather than written next to the button.
//
// What it deliberately does not do is give a command line. The two-step shape
// is what machinery's API says (MachineService.Rollback exists and this product
// does not call it); the exact talosctl invocation is Talos' documentation's to
// state, and a flag spelled wrongly here would be worse than no flag on the one
// screen where somebody is going to paste it.
const RollbackNotice = "holzkube-manager does not undo an upgrade. Talos installs to one of two " +
	"boot partitions and keeps the previous installation on the other, and `talosctl rollback` " +
	"against this node boots it -- per node, and only until that node is upgraded again. It " +
	"undoes the Talos version and nothing else: a Kubernetes upgrade, and anything etcd did " +
	"while the node was on the new version, are not affected. See Talos' documentation for the " +
	"exact invocation."

// SchematicVerdict is what is known about a node's schematic.
type SchematicVerdict struct {
	// ID is the schematic, read off the node and never guessed (D-12).
	ID string `json:"id,omitempty"`

	// Known is whether there is one at all. False means one of two things,
	// which the sentence distinguishes: the node was not built from a Factory
	// image, or it was and nothing has read it yet.
	Known bool `json:"known"`

	// Offered is whether holzkube-manager can read it from the node right now.
	// UPG-03 asks that it "offer to read it", which is only meaningful if the
	// node is answering.
	Offered bool `json:"offered"`

	Sentence string `json:"sentence"`
}

// CheckSchematic reads a node's schematic and says whether an upgrade may
// proceed.
//
// The schematic comes from the node's own ExtensionStatus, which is where
// Talos records what image it was installed from. It is never inferred from
// the cluster, from a sibling node or from what the operator last typed: two
// nodes in one cluster can legitimately have been built from different
// schematics, and a guess here is the extensions-disappear failure with an
// extra step.
func CheckSchematic(ctx context.Context, cc *talos.ClusterClient) (SchematicVerdict, error) {
	factsCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodCOSIList)
	if err != nil {
		return SchematicVerdict{}, err
	}
	defer cancel()

	facts, err := cc.NodeFacts(factsCtx)
	if err != nil {
		return SchematicVerdict{}, err
	}

	if facts.SchematicID == "" {
		return SchematicVerdict{
			Offered: true,
			Sentence: "This node reports no Image Factory schematic. That is the truth for a node " +
				"installed from a stock image, and upgrading it to a stock installer changes " +
				"nothing about its extensions -- it has none. If you believe it was built from a " +
				"Factory image, do not upgrade it here: the schematic would be lost and every " +
				"extension with it.",
		}, nil
	}

	return SchematicVerdict{
		ID:      facts.SchematicID,
		Known:   true,
		Offered: true,
		Sentence: "This node was installed from Image Factory schematic " + facts.SchematicID +
			", read from the node itself. The upgrade installs from the same schematic, so its " +
			"system extensions survive.",
	}, nil
}

// InstallerFor builds the installer reference an upgrade uses.
//
// It is provision.InstallImage, deliberately: the reference an upgrade
// installs and the reference a fresh provision installs are the same thing
// built the same way, and two functions producing it would be two places for
// the schematic to go missing.
func InstallerFor(schematicID string, to Version) string {
	return provision.InstallImage(schematicID, to.String())
}

// KernelArgDrift is UPG-04.
//
// The upgrade RPC carries an installer image and nothing else. Kernel
// arguments live in the machine configuration and are written to the
// bootloader at install time, which means there are two sources of truth for
// them and only one of them travels with an upgrade. A node whose bootloader
// has arguments its configuration does not is a node where the one-click path
// silently discards them.
type KernelArgDrift struct {
	// Drifted is whether the one-click path is blocked.
	Drifted bool `json:"drifted"`

	// InConfig and OnNode are the two lists, both shown. A diff an operator
	// cannot see is a diff they have to take on trust.
	InConfig []string `json:"in_config"`
	OnNode   []string `json:"on_node"`

	// OnlyOnNode and OnlyInConfig are the difference, which is what they
	// actually read.
	OnlyOnNode   []string `json:"only_on_node,omitempty"`
	OnlyInConfig []string `json:"only_in_config,omitempty"`

	Sentence string `json:"sentence,omitempty"`
}

// ReadKernelArgDrift reads both sides and compares them (UPG-04).
//
// Both sides come from the node, which is the only place either of them can be
// read honestly: the command line from /proc/cmdline as the node reports it,
// and the configured arguments from the configuration the node is actually
// running rather than from whatever this installation last wrote.
func ReadKernelArgDrift(ctx context.Context, cc *talos.ClusterClient) (KernelArgDrift, error) {
	cmdlineCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodCOSIGet)
	if err != nil {
		return KernelArgDrift{}, err
	}
	cmdline, err := cc.KernelCmdline(cmdlineCtx)
	cancel()
	if err != nil {
		return KernelArgDrift{}, err
	}

	configCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodCOSIGet)
	if err != nil {
		return KernelArgDrift{}, err
	}
	raw, err := cc.MachineConfigYAML(configCtx)
	cancel()
	if err != nil {
		return KernelArgDrift{}, err
	}

	return CheckKernelArgs(configuredKernelArgs(raw), strings.Fields(cmdline)), nil
}

// configuredKernelArgs reads .machine.install.extraKernelArgs out of a machine
// configuration.
//
// It is parsed out of the YAML rather than through machinery's config loader,
// because the loader validates the whole document and a node running a
// configuration this build's loader rejects is exactly the node somebody is
// trying to upgrade. A list this cannot read comes back empty, which reports
// no drift -- the conservative direction: a false "no drift" costs a check, a
// false "drift" blocks an upgrade that is fine.
func configuredKernelArgs(raw []byte) []string {
	var doc struct {
		Machine struct {
			Install struct {
				ExtraKernelArgs []string `yaml:"extraKernelArgs"`
			} `yaml:"install"`
		} `yaml:"machine"`
	}

	// Only the first document: a machine configuration's later documents are
	// the multi-doc extensions, and none of them carries install options.
	first := raw
	if i := bytes.Index(raw, []byte("\n---")); i >= 0 {
		first = raw[:i]
	}
	if err := yaml.Unmarshal(first, &doc); err != nil {
		return nil
	}
	return doc.Machine.Install.ExtraKernelArgs
}

// CheckKernelArgs compares a node's running kernel command line against what
// its machine configuration asks for.
//
// It compares **sets**, not order. The kernel does not care about the order of
// independent arguments, and a comparison that did would block every upgrade
// on a difference that is not one.
//
// Arguments Talos itself adds are excluded, because they are not the
// operator's and are not carried in the configuration: a check that flagged
// them would flag every node.
func CheckKernelArgs(inConfig, onNode []string) KernelArgDrift {
	d := KernelArgDrift{
		InConfig: append([]string(nil), inConfig...),
		OnNode:   append([]string(nil), onNode...),
	}

	config := set(inConfig)
	node := set(onNode)

	for _, a := range onNode {
		if talosOwnedArg(a) || config[a] {
			continue
		}
		d.OnlyOnNode = append(d.OnlyOnNode, a)
	}
	for _, a := range inConfig {
		if node[a] {
			continue
		}
		d.OnlyInConfig = append(d.OnlyInConfig, a)
	}

	if len(d.OnlyOnNode) == 0 && len(d.OnlyInConfig) == 0 {
		return d
	}

	d.Drifted = true
	var parts []string
	if len(d.OnlyOnNode) > 0 {
		parts = append(parts, fmt.Sprintf("running with %s, which its configuration does not ask for",
			strings.Join(d.OnlyOnNode, " ")))
	}
	if len(d.OnlyInConfig) > 0 {
		parts = append(parts, fmt.Sprintf("configured for %s, which it is not running",
			strings.Join(d.OnlyInConfig, " ")))
	}
	d.Sentence = fmt.Sprintf(
		"This node is %s. The upgrade call carries an installer image and nothing else -- kernel "+
			"arguments are written at install time from the machine configuration -- so upgrading "+
			"through this screen would install a system with the configuration's arguments and "+
			"drop the difference without saying so. Reconcile the two first, or upgrade this node "+
			"by hand with the arguments you mean.",
		strings.Join(parts, ", and "))
	return d
}

// talosOwnedArg reports an argument Talos sets for itself.
//
// These are not the operator's and are not carried in the machine
// configuration's extraKernelArgs, so treating their presence as drift would
// flag every node in every cluster -- which is a check that gets switched off.
func talosOwnedArg(arg string) bool {
	name, _, _ := strings.Cut(arg, "=")
	switch name {
	case "talos.platform", "talos.config", "talos.board", "talos.environment",
		"talos.shutdown", "talos.experimental.wipe", "talos.unified_boot",
		"init_on_alloc", "init_on_free", "slab_nomerge", "pti", "consoleblank",
		"nvme_core.io_timeout", "printk.devkmsg", "ima_template", "ima_appraise",
		"ima_hash", "selinux", "lsm", "console", "root", "initrd", "BOOT_IMAGE":
		return true
	}
	return false
}

func set(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, v := range values {
		out[v] = true
	}
	return out
}

// Observed is what a node reports after an upgrade (UPG-07).
type Observed struct {
	TalosVersion      string `json:"talos_version"`
	KubernetesVersion string `json:"kubernetes_version,omitempty"`
	SchematicID       string `json:"schematic_id,omitempty"`

	// Healthy is whether the node's own services are running.
	Healthy bool `json:"healthy"`

	// Unhealthy names the services that are not, so the screen can say which
	// rather than "not healthy".
	Unhealthy []string `json:"unhealthy,omitempty"`
}

// VerifyUpgraded is UPG-07: declared against observed.
//
// "The API said OK" is not proof. The upgrade RPC returns when the node has
// accepted the request; the stream ends when the node stops talking, which is
// what a successful upgrade and a node falling over both look like. What
// settles it is the node, afterwards, reporting the version that was
// installed -- and reporting that its own services are running, because a node
// on the right version with etcd not starting is not an upgraded node.
//
// The schematic is checked too, and that check is the one most likely to catch
// something: an upgrade that silently installed a stock image comes back on
// exactly the right version with its extensions gone.
func VerifyUpgraded(
	ctx context.Context,
	cc *talos.ClusterClient,
	want Version,
	wantSchematic string,
) (Observed, error) {
	factsCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodCOSIList)
	if err != nil {
		return Observed{}, err
	}
	facts, err := cc.NodeFacts(factsCtx)
	cancel()
	if err != nil {
		return Observed{}, err
	}

	versionCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodVersion)
	if err != nil {
		return Observed{}, err
	}
	version, err := cc.Version(versionCtx)
	cancel()
	if err != nil {
		return Observed{}, err
	}

	servicesCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodServiceList)
	if err != nil {
		return Observed{}, err
	}
	services, err := cc.ServiceList(servicesCtx)
	cancel()
	if err != nil {
		return Observed{}, err
	}

	obs := Observed{
		TalosVersion:      version,
		KubernetesVersion: facts.KubernetesVersion(),
		SchematicID:       facts.SchematicID,
		Healthy:           true,
	}
	for _, s := range services {
		if s.State == "Running" && (s.Healthy || s.HealthUnknown) {
			continue
		}
		obs.Healthy = false
		obs.Unhealthy = append(obs.Unhealthy, s.ID)
	}

	return obs, VerdictFor(obs, want, wantSchematic)
}

// VerdictFor is the comparison VerifyUpgraded makes, without the reads.
//
// It is separated for the reason gate.go separates decide(): the reads need a
// node and the judgement does not, so keeping them together would mean every
// test of the judgement had to stage a node that reports the thing being
// judged. The four verdicts below are then testable as what they are -- four
// sentences an operator gets handed at a bad moment.
func VerdictFor(obs Observed, want Version, wantSchematic string) error {
	got, err := ParseVersion(obs.TalosVersion)
	if err != nil {
		return fmt.Errorf("%w: it reports %q, which is not a version. %s",
			ErrNotUpgraded, obs.TalosVersion, RollbackNotice)
	}
	if got.Major != want.Major || got.Minor != want.Minor || got.Patch != want.Patch {
		return fmt.Errorf("%w: %s was installed and the node reports %s. The upgrade call "+
			"succeeded and the node did not end up on that version, which is exactly the case "+
			"'the API said OK' does not cover. %s", ErrNotUpgraded, want, got, RollbackNotice)
	}

	if wantSchematic != "" && obs.SchematicID != wantSchematic {
		return fmt.Errorf("%w: the node is on %s and reports schematic %q instead of %q. "+
			"The version is right and the image is not, which means the system extensions this "+
			"node had are gone. %s",
			ErrUnknownSchematic, got, orNone(obs.SchematicID), wantSchematic, RollbackNotice)
	}

	if !obs.Healthy {
		return fmt.Errorf("%w: the node is on %s and these services are not running: %s. %s",
			ErrNotUpgraded, got, strings.Join(obs.Unhealthy, ", "), RollbackNotice)
	}

	return nil
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// SkipReason says why a node was not upgraded, for the ones that were skipped
// rather than failed (UPG-14).
func SkipReason(m model.Machine) string {
	return fmt.Sprintf("%s is locked, so this run skipped it. A locked node is skipped rather "+
		"than failing the run: the lock is somebody saying 'not this one', and stopping the whole "+
		"upgrade because of it would make the lock a blunt instrument.", nameOf(m))
}
