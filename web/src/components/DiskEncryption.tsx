import { Label } from '@/components/ui/label'

/**
 * Encrypting a node's system volumes, offered at the one moment it can be.
 *
 * Talos encrypts a system volume when the volume is empty — "before mounting
 * the partition, format and encrypt it; this occurs only if the partition is
 * empty and has no filesystem". So this is a provisioning question and nowhere
 * else: handing the same configuration to a node that is already installed
 * does not encrypt what is on it, does not fail, and does not warn. The
 * operator would believe the opposite of the truth, which is the worst failure
 * a security control can have.
 *
 * The panel says what each choice is *not* worth, on the same screen as the
 * switch, because "the disk is encrypted" is what somebody will remember.
 */
export function DiskEncryption({
  state,
  ephemeral,
  kind,
  secureBoot,
  onState,
  onEphemeral,
  onKind,
}: {
  state: boolean
  ephemeral: boolean
  kind: string
  secureBoot: boolean
  onState: (v: boolean) => void
  onEphemeral: (v: boolean) => void
  onKind: (v: string) => void
}) {
  const enabled = state || ephemeral

  return (
    <section className="space-y-2 rounded-md border border-border px-3 py-2">
      <div>
        <h3 className="text-sm font-medium">Disk encryption</h3>
        <p className="max-w-prose text-xs text-muted-foreground">
          Talos encrypts a system volume only while it is empty, so this is decided now or not at
          all. A node that is already installed keeps its plaintext partitions whatever its
          configuration says, and reports nothing about it.
        </p>
      </div>

      <label className="flex items-start gap-2 text-sm">
        <input
          type="checkbox"
          className="mt-1"
          checked={state}
          onChange={(e) => onState(e.target.checked)}
        />
        <span>
          <strong>STATE</strong> — the node's own secrets and certificates
        </span>
      </label>

      <label className="flex items-start gap-2 text-sm">
        <input
          type="checkbox"
          className="mt-1"
          checked={ephemeral}
          onChange={(e) => onEphemeral(e.target.checked)}
        />
        <span>
          <strong>EPHEMERAL</strong> — whatever the workloads write
        </span>
      </label>

      {enabled && (
        <div className="space-y-1.5">
          <Label htmlFor="encryption-kind">Key</Label>
          <select
            id="encryption-kind"
            value={kind}
            onChange={(e) => onKind(e.target.value)}
            className="h-8 rounded-md border border-border bg-background px-2 text-sm"
          >
            <option value="nodeID">Derived from this machine (nodeID)</option>
            <option value="tpm">Sealed by the TPM</option>
          </select>

          {/* What it is worth, next to the choice rather than in documentation
              somebody reads afterwards. */}
          {kind === 'nodeID' && (
            <p className="max-w-prose text-xs text-muted-foreground">
              The key comes from this machine's identity, so it protects a drive that leaves the
              machine — a disposal, a warranty return, a stolen disk. It does not protect against
              anyone who has the machine.
            </p>
          )}
          {kind === 'tpm' && !secureBoot && (
            <p className="max-w-prose text-xs text-destructive">
              TPM sealing needs SecureBoot, and this machine is not marked as having booted the
              SecureBoot image. The seal is a statement about which kernel booted, and without
              SecureBoot that measurement can be produced by a kernel somebody else chose. The
              server refuses this combination rather than installing the weaker thing quietly.
            </p>
          )}
          {kind === 'tpm' && secureBoot && (
            <p className="max-w-prose text-xs text-muted-foreground">
              The key is sealed by the TPM against a SecureBoot chain, so it protects the disk and
              also refuses to unseal under a kernel that is not the one this node should boot.
            </p>
          )}

          <p className="max-w-prose text-xs text-muted-foreground">
            A passphrase written into the configuration and a network key server are the two other
            things Talos offers, and neither is here. Ask the server for either and it answers with
            the reason.
          </p>
        </div>
      )}
    </section>
  )
}
