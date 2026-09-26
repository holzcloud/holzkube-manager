/**
 * The alpha notice (2026-09-26).
 *
 * The operator's words: it has to be visible everywhere that this is alpha
 * software under heavy development, and that breaking changes can come. So the
 * wording lives here once, and every surface an operator can reach -- the shell,
 * both unauthenticated screens and the wall -- shows it: the badge where there
 * is room for one word, the sentence where there is room for a sentence.
 *
 * The version string itself stays plain. `holzkube-managerd --version` is read
 * by the update script with awk's last field, so a suffix there would make every
 * host believe an update is always due.
 */

export const ALPHA_SENTENCE =
  'Alpha — under heavy development. An update can change or break things; keep a backup of the data directory.'

export function AlphaBadge({ className = '' }: { className?: string }) {
  return (
    <span
      title={ALPHA_SENTENCE}
      className={`rounded-full border border-primary/40 bg-primary/10 px-1.5 py-px text-[10px] font-semibold uppercase tracking-wider text-primary ${className}`}
    >
      Alpha
    </span>
  )
}

export function AlphaNotice({ className = '' }: { className?: string }) {
  return <p className={className}>{ALPHA_SENTENCE}</p>
}
