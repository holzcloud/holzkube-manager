import { afterEach, describe, expect, it, vi } from 'vitest'
import { readSeenVersion, shouldAnnounce, splitHeadline, writeSeenVersion } from './releaseNotes'

const KNOWN = ['v0.8.0', 'v0.7.1']

describe('when the notes open by themselves', () => {
  it('opens for an update: seen one release, now running another the changelog knows', () => {
    expect(shouldAnnounce('v0.7.1', 'v0.8.0', KNOWN)).toBe(true)
    // The binary reports without the v.
    expect(shouldAnnounce('0.7.1', '0.8.0', KNOWN)).toBe(true)
  })

  it('does not open twice for one version', () => {
    expect(shouldAnnounce('v0.8.0', 'v0.8.0', KNOWN)).toBe(false)
    expect(shouldAnnounce('v0.8.0', '0.8.0', KNOWN)).toBe(false)
  })

  it('does not open on a browser that has never seen any version', () => {
    // A fresh install has nothing that is new to it.
    expect(shouldAnnounce(null, 'v0.8.0', KNOWN)).toBe(false)
  })

  it('does not open for a development build, which no release describes', () => {
    expect(shouldAnnounce('v0.8.0', 'v0.8.0-4-gabc1234-dirty', KNOWN)).toBe(false)
    expect(shouldAnnounce('v0.8.0', '', KNOWN)).toBe(false)
  })
})

describe('the remembered version', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    window.localStorage.clear()
  })

  it('round-trips', () => {
    expect(readSeenVersion()).toBeNull()
    writeSeenVersion('v0.8.0')
    expect(readSeenVersion()).toBe('v0.8.0')
  })

  it('works without storage: nothing is known, nothing throws', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    expect(readSeenVersion()).toBeNull()
    expect(() => writeSeenVersion('v0.8.0')).not.toThrow()
  })
})

describe('the headline of a change', () => {
  it('takes a short first sentence as the headline', () => {
    expect(splitHeadline('Configuration drift. holzkube-manager now notices it.')).toEqual({
      headline: 'Configuration drift',
      body: 'holzkube-manager now notices it.',
    })
  })

  it('leaves a change without a short first sentence alone', () => {
    const text = 'Removing a node asks etcd first.'
    expect(splitHeadline(text)).toEqual({ headline: '', body: text })
    const long = `${'word '.repeat(30)}ends here. And goes on.`
    expect(splitHeadline(long).headline).toBe('')
  })

  it('does not take the end of an abbreviation for the end of a headline', () => {
    const text = 'Use e.g. a label. More text follows here.'
    expect(splitHeadline(text).headline).not.toBe('Use e.g')
  })
})
