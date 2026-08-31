import type { AuditChain } from '@/api'
import { useSystemStatus } from '@/hooks/useSession'

/**
 * The audit hash chain verdict, read from `audit_chain` in
 * `GET /api/v1/system/status` and shown above everything else on every page
 * (D-15).
 *
 * There is deliberately no control that closes, hides or confirms this banner
 * out of the way, and the component holds no state of its own that could be
 * set to "seen". The verdict arrives as a property and nothing else; the only
 * thing that makes the banner go away is a start-up that finds an intact
 * chain. A hash chain nobody looks at is theatre, and a chain break that can be
 * clicked away is worse than no chain at all -- it destroys the evidence that
 * it was ever broken (threat T-01-34).
 *
 * Note what the banner does NOT claim. A break says the file on disk no longer
 * matches its own chain. It does not say who did it, and it does not say the
 * cluster is compromised.
 */
export function ChainBanner({ chain }: { chain: AuditChain }) {
  if (chain.ok) {
    return null
  }

  return (
    <div
      role="alert"
      aria-live="assertive"
      className="border-b border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive"
    >
      <p className="font-semibold">The audit log no longer verifies against its own hash chain.</p>
      <p className="mt-1 text-destructive/90">
        The first record that does not match is line {chain.broken_at_line} of{' '}
        <span className="font-mono text-xs">{chain.file}</span>. That means the file on disk has
        been changed since it was written. It does not by itself say by whom, and it does not mean
        the cluster is compromised. holzkube-manager will not repair the file: the break is the
        evidence.
      </p>
    </div>
  )
}

/**
 * The mounted form: asks the server and renders the verdict. Split from the
 * banner itself so the banner stays a pure function of the verdict, which is
 * what makes "it has no state that could remember having been seen" a property
 * a test can check rather than a promise.
 */
export function ChainBannerContainer() {
  const status = useSystemStatus()

  if (!status.isSuccess) {
    return null
  }

  return <ChainBanner chain={status.data.audit_chain} />
}
