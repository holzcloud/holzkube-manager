import { describe, expect, it } from 'vitest'
import { parseConditions, slug } from '@/components/MachineClasses'

/**
 * The condition parser is the one place on this screen where a mistake makes a
 * selector that quietly names the wrong machines rather than an error, so it is
 * tested on its own rather than through a form.
 */
describe('parsing machine class conditions', () => {
  it('reads the three kinds of condition', () => {
    expect(parseConditions('rack=b3, storage=, !decommissioned')).toEqual({
      equals: { rack: 'b3' },
      present: ['storage'],
      absent: ['decommissioned'],
    })
  })

  it('reads a bare key as presence, which is the obvious reading', () => {
    expect(parseConditions('storage')).toEqual({
      equals: {},
      present: ['storage'],
      absent: [],
    })
  })

  it('produces no conditions from nothing, so the server refuses it', () => {
    // Not "everything". An empty selector matches no machines on the server,
    // deliberately, and this side must not invent conditions to avoid the
    // refusal — the refusal is the point.
    expect(parseConditions('   ,  , ')).toEqual({ equals: {}, present: [], absent: [] })
  })

  it('ignores spacing rather than making it part of a key', () => {
    // A key with a leading space is refused by the server because it is
    // invisible on a screen, and trimming here means an operator who typed a
    // space after a comma is not told off for it.
    expect(parseConditions('  rack = b3 ')).toEqual({
      equals: { rack: 'b3' },
      present: [],
      absent: [],
    })
  })

  it('keeps an equals sign inside a value', () => {
    expect(parseConditions('note=a=b')).toEqual({
      equals: { note: 'a=b' },
      present: [],
      absent: [],
    })
  })
})

describe('turning a class name into an id', () => {
  it('produces something that needs no escaping in a path', () => {
    expect(slug('Rack B / spares')).toBe('rack-b-spares')
    expect(slug('  Storage  ')).toBe('storage')
  })

  it('never produces an empty id', () => {
    // An empty id would address the collection rather than a member, which is
    // a PUT to the wrong URL rather than a validation error.
    expect(slug('///')).toBe('class')
    expect(slug('')).toBe('class')
  })
})
