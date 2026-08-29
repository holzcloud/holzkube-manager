import {
  type SchematicWarning,
  WARNING_INSTALLER_IGNORES_KERNEL_ARGS,
  WARNING_INSTALLER_IGNORES_META,
} from '@/api'

/**
 * The installer/initramfs asymmetry, said out loud (FACT-04).
 *
 * Talos states it verbatim: *"installer and initramfs images only support
 * system extensions (kernel args and META are ignored)"*. So a schematic
 * carrying kernel arguments produces an ISO that has them and an installed
 * system that does not. The machine boots correctly from the USB stick and then
 * installs a subtly different system, and nothing anywhere reports it.
 *
 * The only moment an operator can act on that is while they are still authoring
 * the schematic, which is why this is rendered live from the form rather than
 * only from the server's answer to the create request. FACT-04 says the warning
 * fires *beim Autoren*, not afterwards.
 *
 * The component is pure: it takes the list and renders it. That is what makes
 * "the live warning and the server's warning say the same thing" checkable --
 * both go through here, and the wording below is the server's own text
 * duplicated once, in one place, keyed by the same code constants the server
 * exports.
 */

/**
 * The detail text for a locally predicted warning.
 *
 * These are the server's own sentences, transcribed character for character
 * from `imagefactory.Warnings`, so that the warning an operator reads while
 * typing and the warning they read after submitting cannot disagree in
 * wording -- two different sentences about one condition read as two problems.
 *
 * "Cannot disagree" is enforced rather than asserted: `TestWarningDetailsMatchTheUI`
 * in `internal/imagefactory/warnings_test.go` reads this file and fails if either
 * string drifts from the constant it was copied from. The check lives on the Go
 * side because that is where the text is authored; vitest cannot read outside
 * `web/` without loosening the bundler's filesystem allowlist, which is a real
 * cost for a test-only convenience.
 */
const KERNEL_ARGS_DETAIL =
  'Kernel arguments apply to the ISO, PXE and disk images only. The installer and initramfs images honour system extensions and ignore kernel arguments, so a machine installed from this schematic boots without them. Set the same arguments in the machine configuration under .machine.install.extraKernelArgs so the installed system matches the media it was installed from.'

const META_DETAIL =
  'META values are written when the image is built and apply to the ISO, PXE and disk images only. The installer and initramfs images honour system extensions and ignore META, so these values are not reapplied when the node is installed or upgraded. Set anything that must survive an upgrade in the machine configuration, alongside .machine.install.extraKernelArgs, rather than relying on the schematic.'

/**
 * Predicts the warnings the server will return for what is currently typed.
 *
 * The predicate is client-side on purpose -- there is no request to make while
 * somebody is still filling in a row -- but the *text* is the server's, so the
 * two surfaces cannot drift apart in wording. The predicate itself is the
 * server's, transcribed: a non-empty extra kernel argument, or a META entry
 * with a value.
 */
export function predictWarnings(
  kernelArgs: string[],
  meta: { value: string }[],
): SchematicWarning[] {
  const warnings: SchematicWarning[] = []
  if (kernelArgs.some((arg) => arg.trim() !== '')) {
    warnings.push({
      code: WARNING_INSTALLER_IGNORES_KERNEL_ARGS,
      detail: KERNEL_ARGS_DETAIL,
    })
  }
  if (meta.some((entry) => entry.value.trim() !== '')) {
    warnings.push({
      code: WARNING_INSTALLER_IGNORES_META,
      detail: META_DETAIL,
    })
  }
  return warnings
}

/**
 * Renders a warning list. Nothing to say renders nothing at all -- an empty
 * alert box would train the operator to ignore the box.
 */
export function SchematicWarnings({ warnings }: { warnings: SchematicWarning[] }) {
  if (warnings.length === 0) {
    return null
  }

  return (
    <div
      role="alert"
      aria-label="Schematic warnings"
      className="rounded-md border border-amber-500/50 bg-amber-500/10 px-4 py-3 text-sm"
    >
      <p className="font-semibold">
        This schematic will produce an ISO and an installed system that differ.
      </p>
      <ul className="mt-2 space-y-2">
        {warnings.map((warning) => (
          <li key={warning.code}>
            <span className="font-mono text-xs text-muted-foreground">{warning.code}</span>
            <p className="mt-0.5">{warning.detail}</p>
          </li>
        ))}
      </ul>
    </div>
  )
}

/**
 * The live half: the same component, fed from the form rather than the server.
 */
export function LiveSchematicWarnings({
  kernelArgs,
  meta,
}: {
  kernelArgs: string[]
  meta: { value: string }[]
}) {
  return <SchematicWarnings warnings={predictWarnings(kernelArgs, meta)} />
}
