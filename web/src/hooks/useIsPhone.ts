import { useSyncExternalStore } from 'react'

/**
 * Whether this is a phone-width screen (2026-09-19).
 *
 * # Why a media QUERY and not a media-query CLASS
 *
 * The obvious way to show a table on a desk and cards on a phone is
 * `hidden md:block` beside `md:hidden` — pure CSS, no JavaScript, correct at
 * every width without a listener. It was built that way first and it was wrong,
 * for a reason that has nothing to do with tests: both presentations are then
 * in the DOM at once. Two copies of every form control, so two elements
 * labelled "Replicas for api"; two links with the same accessible name; every
 * `getByLabelText` ambiguous and, more importantly, every `document.getElementById`
 * and every label/control pairing suddenly non-unique. Hiding one with
 * `display: none` keeps assistive technology out of it but does not make the
 * document valid.
 *
 * So exactly one is rendered, and this is what decides which.
 *
 * # The breakpoint is Tailwind's `md`, and it has to stay that way
 *
 * 768px is where `md:` turns on. If this number and that breakpoint ever
 * disagree, the layout guard measures one presentation and the stylesheet
 * styles the other — a failure that looks like nothing at all. It is written
 * once, here, and used through this hook rather than repeated per screen.
 *
 * # What a browser without matchMedia gets
 *
 * The desk. A missing matchMedia means jsdom or a very old browser, and the
 * table is the presentation that works without any layout assumptions — the
 * same reasoning useTheme uses for falling back to the product default rather
 * than to a blank screen.
 */

/** Tailwind's `md` breakpoint, minus a pixel: below this, `md:` rules are off. */
export const PHONE_QUERY = '(max-width: 767px)'

function query(): MediaQueryList | null {
  if (typeof globalThis.matchMedia !== 'function') return null
  return globalThis.matchMedia(PHONE_QUERY)
}

function subscribe(onChange: () => void): () => void {
  const mql = query()
  if (mql === null) return () => {}
  // addEventListener is the modern one; addListener is kept for the browsers
  // that still only have it, because falling back to "always desk" on a phone
  // would be the exact failure this hook exists to prevent.
  if (typeof mql.addEventListener === 'function') {
    mql.addEventListener('change', onChange)
    return () => mql.removeEventListener('change', onChange)
  }
  mql.addListener?.(onChange)
  return () => mql.removeListener?.(onChange)
}

function snapshot(): boolean {
  return query()?.matches ?? false
}

export function useIsPhone(): boolean {
  // The server snapshot is the desk for the reason above: this product has no
  // server rendering today, and a hook that differed between the two would be a
  // hydration mismatch waiting for the day it does.
  return useSyncExternalStore(subscribe, snapshot, () => false)
}
