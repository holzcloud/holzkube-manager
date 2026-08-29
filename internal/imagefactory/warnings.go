package imagefactory

// The warning codes this package emits.
//
// They are exported constants rather than string literals at the call site
// because they are identifiers other components key on: plan 02-06 renders
// them, and the audit allowlist references them. A literal repeated at three
// call sites is a typo waiting to make one of them silently stop matching.
const (
	// WarningInstallerIgnoresKernelArgs reports that a schematic's kernel
	// arguments reach the ISO and the disk image but not the installed system.
	WarningInstallerIgnoresKernelArgs = "schematic.installer-ignores-kernel-args"

	// WarningInstallerIgnoresMeta reports the same asymmetry for META values.
	WarningInstallerIgnoresMeta = "schematic.installer-ignores-meta"
)

// Warning is one thing an operator should know about a schematic before they
// build anything from it.
//
// It is not an error. Every condition warned about here produces a schematic
// the Factory builds happily; the point is that what it builds is not what the
// operator is likely to think it built.
type Warning struct {
	// Code is the stable identifier a UI keys on.
	Code string `json:"code"`

	// Detail is the operator-facing sentence: what is affected, and what to do
	// about it.
	Detail string `json:"detail"`
}

// Warnings inspects a schematic and reports the divergences it will cause.
//
// This is FACT-04, and it exists because of a restriction stated verbatim in
// the Talos v1.13 documentation: "installer and initramfs images only support
// system extensions (kernel args and META are ignored)". So a schematic
// carrying kernel arguments produces an ISO that has them and an installed
// system that does not. The machine boots correctly from the USB stick and then
// installs a subtly different system, and nothing anywhere reports it.
//
// The only moment an operator can act on this is while they are authoring the
// schematic, which is why the warning is produced here rather than discovered
// later by comparing a running node against its declaration.
//
// The returned slice is empty and never nil, so a JSON encoding of "no
// warnings" is [] rather than null. A null reads to a client as "the server did
// not check", which is a different and much less reassuring statement.
func Warnings(s Schematic) []Warning {
	warnings := make([]Warning, 0, 2)

	if len(s.Customization.ExtraKernelArgs) > 0 {
		warnings = append(warnings, Warning{
			Code: WarningInstallerIgnoresKernelArgs,
			Detail: "Kernel arguments apply to the ISO, PXE and disk images only. " +
				"The installer and initramfs images honour system extensions and ignore kernel arguments, " +
				"so a machine installed from this schematic boots without them. " +
				"Set the same arguments in the machine configuration under .machine.install.extraKernelArgs " +
				"so the installed system matches the media it was installed from.",
		})
	}

	if len(s.Customization.Meta) > 0 {
		warnings = append(warnings, Warning{
			Code: WarningInstallerIgnoresMeta,
			Detail: "META values are written when the image is built and apply to the ISO, PXE and disk images only. " +
				"The installer and initramfs images honour system extensions and ignore META, " +
				"so these values are not reapplied when the node is installed or upgraded. " +
				"Set anything that must survive an upgrade in the machine configuration, " +
				"alongside .machine.install.extraKernelArgs, rather than relying on the schematic.",
		})
	}

	return warnings
}
