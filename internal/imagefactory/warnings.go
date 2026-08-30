package imagefactory

// The warning codes this package emits.
//
// They are exported constants rather than string literals at the call site
// because they are identifiers other components key on: plan 02-06 renders
// them, and the audit allowlist references them. A literal repeated at three
// call sites is a typo waiting to make one of them silently stop matching.
//
// There are two families and the prefix says which, because the two make
// different kinds of statement:
//
//   - "schematic." is a statement about a stored schematic. Warnings, below,
//     recomputes these from the record at any time -- they are properties of
//     the thing itself and outlive the request that produced them.
//   - "installer." is a statement about one installer-repository resolution
//     attempt: how a name was obtained on this request. No record holds it and
//     nothing can recompute it later, which is exactly why it has to ride on
//     the response that carries the name.
//
// Do not go looking for the second family in Warnings. It is not there and it
// cannot be: Warnings takes a Schematic and this fact is not about one.
// installer.go emits it, alongside the reference the fact is about.
// TestWarningsCodesAreNamespaced holds the prefix set, so a code added later
// has to choose a family rather than inherit one.
const (
	// WarningInstallerIgnoresKernelArgs reports that a schematic's kernel
	// arguments reach the ISO and the disk image but not the installed system.
	WarningInstallerIgnoresKernelArgs = "schematic.installer-ignores-kernel-args"

	// WarningInstallerIgnoresMeta reports the same asymmetry for META values.
	WarningInstallerIgnoresMeta = "schematic.installer-ignores-meta"

	// WarningInstallerRepoFallbackUnverified reports that the installer
	// repository name in a reference was reached past a candidate that never
	// answered, so the preferred name was unheard rather than ruled out.
	//
	// This is a warning and not an error, and the distinction is the whole of
	// G-02-3. The fallback is deliberate: 02-04-PLAN.md:157-161 specifies it for
	// a connection failure, and 02-04-SUMMARY.md:385 records factory.talos.dev
	// throttling without producing an HTTP response at all. Refusing to answer
	// here would leave an operator unable to read their own asset URLs because a
	// third party was busy -- a worse outcome than a labelled answer, and one
	// nothing on screen would explain.
	//
	// What was wrong before was not the fallback. It was that the fully formed
	// transport error was discarded at the moment a later candidate answered,
	// and the resulting name was then cached with no provenance and no expiry.
	// Two processes disagreed about one schematic at one version, split exactly
	// at a restart, with nothing anywhere saying the preferred name had never
	// been ruled out. This code is that missing sentence.
	WarningInstallerRepoFallbackUnverified = "installer.repo-fallback-unverified"

	// WarningInstallerSecureBootRepoFallbackUnverified reports the same
	// provenance for a SecureBoot request, and it is a separate code because
	// the fact an operator needs from it is a different one.
	//
	// Round 1 recorded installer-secureboot as a legacy alias of
	// metal-installer-secureboot, and that recording is why the SecureBoot
	// fallback was treated as harmless when installerCandidates was written. It
	// is wrong at the pinned version: the two names resolve to two different
	// manifest digests (02-UAT.md G-02-13, re-measured 2026-08-30 by
	// TestLiveFactory's installer-name matrix, which is where the numbers live
	// so that they can be re-measured rather than believed). At the oldest
	// supported version the same two names do resolve to one image, so neither
	// "alias" nor "different image" is true of the pair in general -- which is
	// exactly why the answer has to be labelled per resolution rather than
	// settled once in a comment.
	//
	// Nothing about this weakens T-02-53: both candidates are SecureBoot
	// installers and a SecureBoot request is still never answered with an
	// ordinary one. What the operator gains is the ability to tell "the
	// preferred name was unheard" from "and the image you were handed is
	// therefore not the one that name selects", which the generic code above
	// cannot say without claiming it of every fallback.
	WarningInstallerSecureBootRepoFallbackUnverified = "installer.secureboot-repo-fallback-unverified"
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
