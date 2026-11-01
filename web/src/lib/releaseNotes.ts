/**
 * Small pure rules behind the What's New screen, kept out of the component so
 * they can be tested without rendering one.
 */

/**
 * Whether two version strings name the same release, ignoring a leading v.
 *
 * goreleaser stamps the binary with the tag minus its v, so a released build
 * reports `1.16.0`, while the changelog spells versions as the git tag does,
 * `v1.16.0`. Neither can adopt the other's spelling (the update script compares
 * the tag with its v stripped against what the binary reports), so the reader
 * normalises. A development build, `v1.16.0-3-g1234-dirty`, matches nothing, on
 * purpose.
 */
export function sameVersion(a: string, b: string): boolean {
  return a.replace(/^v/, '') === b.replace(/^v/, '')
}

/**
 * Splits a change into a headline and the rest, when it has one.
 *
 * The changelog is written as prose: "Configuration drift. holzkube-manager now
 * notices ...". The first sentence is a headline in all but typography, and the
 * screen sets it as one. The text is not edited to make that possible; a first
 * sentence that is long, or that ends in the middle of a word like "e.g.", is
 * not a headline and the change is shown as a plain paragraph.
 */
export function splitHeadline(text: string): { headline: string; body: string } {
  const match = /^(\S[^.!?]{2,60}[.!?])\s+(\S[\s\S]*)$/.exec(text.trim())
  if (match === null) {
    return { headline: '', body: text }
  }
  const head = match[1] ?? ''
  const body = match[2] ?? ''
  // "e.g." / "i.e." / "vs." end a clause, not a headline.
  if (/\b(e\.g|i\.e|vs|etc)\.$/i.test(head)) {
    return { headline: '', body: text }
  }
  return { headline: head.replace(/[.]$/, ''), body }
}

const SEEN_KEY = 'holzkube-manager.whats-new.seen'

/**
 * The version whose notes this browser last showed, or null when it never has.
 *
 * Browser storage can be absent or throw (private windows, blocked site data),
 * and the screen must work without it: a null here means "nothing is known",
 * never an error.
 */
export function readSeenVersion(): string | null {
  try {
    return window.localStorage.getItem(SEEN_KEY)
  } catch {
    return null
  }
}

export function writeSeenVersion(version: string): void {
  try {
    window.localStorage.setItem(SEEN_KEY, version)
  } catch {
    // Without storage the screen simply shows again next time. Not an error.
  }
}

/**
 * Whether the notes should open by themselves.
 *
 * Only for an update: this browser has seen some version before and the running
 * one is a different release that the changelog actually describes. A browser
 * that has never seen any version learns the current one silently -- a fresh
 * install has nothing that is "new" to it -- and a development build, which is
 * no release, never interrupts.
 */
export function shouldAnnounce(
  seen: string | null,
  running: string,
  knownVersions: string[],
): boolean {
  if (seen === null || running === '') {
    return false
  }
  if (sameVersion(seen, running)) {
    return false
  }
  return knownVersions.some((v) => sameVersion(v, running))
}
