import { AlertTriangle } from 'lucide-react'
import type { HostState } from '@/api'

/**
 * The host's state as a mark (HOST-04, D-10, D-13): one component, three
 * shapes, so the state survives greyscale and a reader who cannot tell green
 * from amber -- severity is never colour alone.
 *
 *   ok       a filled dot in --viz-ok
 *   warn     the triangle the Meter already shows for "high", in --viz-warn
 *   unknown  a hollow grey ring: not readable is muted, never amber
 *
 * `unanswered` is not a state the server sends. It is the navigation's name for
 * "the latest poll failed", drawn as the ring: an unanswered question is not an
 * answer, and a green dot kept from an earlier reading would vouch for a host
 * nobody heard from.
 *
 * Every shape sits in the same fixed 16-px box, so a change of state never
 * moves the text beside it. The shapes are aria-hidden; what a screen reader
 * hears is the sr-only word of the sidebar form. The line form carries none:
 * on the /host header the visible word follows the mark, and a screen reader
 * would otherwise say it twice.
 *
 * The mark draws the server's state and nothing else -- it takes a state, not
 * readings, so there is nothing here it could work a state out of.
 */
export type HostMarkState = HostState | 'unanswered'

/** The UI words for the header line (UI-SPEC Copywriting). */
export const STATE_WORD: Record<HostState, string> = {
  ok: 'Healthy',
  warn: 'Warning',
  unknown: 'Not readable',
}

/** The screen-reader words of the sidebar form, after a space: the link reads "Host — warning". */
const SIDEBAR_WORD: Record<HostMarkState, string> = {
  ok: '— healthy',
  warn: '— warning',
  unknown: '— not readable',
  unanswered: '— not readable, holzkube-manager did not answer',
}

export function HostStateMark({
  state,
  summary,
  form,
}: {
  state: HostMarkState
  /** The server's summary sentence, as the hover title. Never the only place it is said. */
  summary?: string
  form: 'sidebar' | 'line'
}) {
  return (
    <>
      <span
        data-host-state={state}
        title={summary}
        className="inline-flex size-4 shrink-0 items-center justify-center"
      >
        {state === 'ok' && (
          <span
            aria-hidden="true"
            className="inline-block size-2 shrink-0 rounded-full"
            style={{ background: 'var(--viz-ok)' }}
          />
        )}
        {state === 'warn' && (
          <AlertTriangle
            aria-hidden="true"
            className="size-3.5 shrink-0"
            style={{ color: 'var(--viz-warn)' }}
          />
        )}
        {(state === 'unknown' || state === 'unanswered') && (
          <span
            aria-hidden="true"
            className="inline-block size-2 shrink-0 rounded-full border border-muted-foreground"
          />
        )}
      </span>
      {/* The space is a text node of its own: an accessible name is put
          together per element and trimmed, so a space inside the sr-only
          span was lost ("Host— warning"). In the link's flex row a
          whitespace-only text node takes no room. */}
      {form === 'sidebar' && (
        <>
          {' '}
          <span className="sr-only">{SIDEBAR_WORD[state]}</span>
        </>
      )}
    </>
  )
}
