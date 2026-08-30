import { useSession } from '@/hooks/useSession'

/**
 * The dry-run indicator (FOUND-12, D-03), read from `dry_run` in
 * `GET /api/v1/auth/me` and shown above everything else on every page.
 *
 * Like {@link ChainBanner}, it has no control that closes, hides or
 * acknowledges it, and the component holds no state of its own that could be
 * set to "seen" -- nothing is written to `localStorage`, `sessionStorage` or a
 * cookie. The reason is sharper here than for the chain banner: an operator who
 * dismissed this once and then believes they are in dry-run while they are live
 * is worse off than an operator with no indicator at all, because they will
 * reboot a production node expecting nothing to happen (threat T-02-44). The
 * only thing that makes it go away is restarting the process without the flag.
 *
 * It is `role="status"` with `aria-live="polite"`, deliberately not
 * `ChainBanner`'s assertive `alert`. Dry-run is a mode the operator chose, not
 * a fault, and styling a chosen mode as an error is how operators learn to
 * ignore banners -- which would cost the chain banner its meaning too. The
 * colours differ from the chain banner's for the same reason: two banners
 * showing at once must not read as one.
 *
 * What it says is what is actually true. The refusal happens in the transport,
 * at the last layer before the wire, so no mutation reaches a node; it is not a
 * UI-level caveat and the wording does not hedge it into one.
 */
export function DryRunBanner({ dryRun }: { dryRun: boolean }) {
  if (!dryRun) {
    return null
  }

  return (
    <div
      role="status"
      aria-live="polite"
      className="border-b border-amber-500/50 bg-amber-500/10 px-4 py-3 text-sm text-amber-900 dark:text-amber-200"
    >
      <p className="font-semibold">Dry-run mode: this instance changes nothing on any node.</p>
      <p className="mt-1 opacity-90">
        holzkube-managerd was started with <span className="font-mono text-xs">--dry-run</span>. Every
        mutating call is refused in the transport before it reaches a node, so applying a
        configuration, bootstrapping, rebooting and resetting all do nothing at all. Reading is
        unaffected: everything shown here is real. Restart without the flag to make changes.
      </p>
    </div>
  )
}

/**
 * The mounted form: asks the server and renders the mode. Split from the banner
 * itself so the banner stays a pure function of the flag, which is what makes
 * "it has no state that could remember having been seen" a property a test can
 * check rather than a promise.
 */
export function DryRunBannerContainer() {
  const { me } = useSession()

  if (!me) {
    return null
  }

  return <DryRunBanner dryRun={me.dry_run} />
}
