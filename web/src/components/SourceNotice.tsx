/**
 * The AGPL section 13 notice.
 *
 * holzkube-manager is offered to its operator over a network, which is exactly the case
 * the Affero clause covers: anyone interacting with the program remotely has to
 * be offered its source. A licence file in the repository does not discharge
 * that on its own — the running instance has to say so — so this renders in the
 * shell and on both unauthenticated screens, which together are every surface a
 * user can reach.
 *
 * The URL is the upstream repository rather than a self-hosted copy. That is
 * honest for an unmodified build and is what section 13 asks for; an operator
 * running a MODIFIED holzkube-manager must point this at their own source, which is why
 * the constant sits here alone and is not buried in a component.
 */

export const SOURCE_URL = 'https://github.com/holzcloud/holzkube-manager'

export function SourceNotice({ className }: { className?: string }) {
  return (
    <p className={className}>
      <a
        href={SOURCE_URL}
        target="_blank"
        rel="noreferrer noopener"
        className="underline underline-offset-2 hover:text-foreground"
      >
        Source
      </a>
      {' · AGPL-3.0'}
    </p>
  )
}
