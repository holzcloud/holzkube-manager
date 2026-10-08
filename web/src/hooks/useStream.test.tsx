import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useStream } from '@/hooks/useStream'

/**
 * Toggling a panel changes the topic list, which reopens the connection, and
 * the server answers a new connection with its ring buffer. The lines this tab
 * already shows must not appear a second time.
 */

class FakeSource {
  static all: FakeSource[] = []
  static readonly CLOSED = 2
  readyState = 0
  onopen: (() => void) | null = null
  onmessage: ((ev: MessageEvent<string>) => void) | null = null
  onerror: (() => void) | null = null
  closed = false
  constructor(readonly url: string) {
    FakeSource.all.push(this)
  }
  addEventListener() {}
  close() {
    this.closed = true
  }
  send(topic: string, id: number, line: string) {
    this.onmessage?.({
      data: JSON.stringify({ topic, payload: { line, at: '2026-09-11T12:00:00Z' } }),
      lastEventId: `${topic}=${id}`,
    } as MessageEvent<string>)
  }
}

beforeEach(() => {
  FakeSource.all = []
  vi.stubGlobal('EventSource', FakeSource)
})
afterEach(() => vi.unstubAllGlobals())

describe('useStream', () => {
  it('does not show a line twice when the reopened connection replays the ring buffer', () => {
    const { result, rerender } = renderHook(({ topics }) => useStream(topics), {
      initialProps: { topics: ['kubelet'] },
    })
    const first = FakeSource.all[0] as FakeSource
    act(() => {
      first.send('kubelet', 1, 'one')
      first.send('kubelet', 2, 'two')
    })

    rerender({ topics: ['kubelet', 'dmesg'] })
    const second = FakeSource.all[1] as FakeSource
    expect(first.closed).toBe(true)
    act(() => {
      second.send('kubelet', 1, 'one')
      second.send('kubelet', 2, 'two')
      second.send('kubelet', 3, 'three')
      second.send('dmesg', 1, 'boot')
    })

    expect(result.current.lines.kubelet?.map((l) => l.line)).toEqual(['one', 'two', 'three'])
    expect(result.current.lines.dmesg?.map((l) => l.line)).toEqual(['boot'])
  })

  it('gives frames without a cursor distinct ids, and survives a frame that is not JSON', () => {
    const { result } = renderHook(() => useStream(['kubelet']))
    const source = FakeSource.all[0] as FakeSource
    act(() => {
      source.onmessage?.({ data: '{not json', lastEventId: '' } as MessageEvent<string>)
      source.onmessage?.({
        data: JSON.stringify({ topic: 'kubelet', payload: { line: 'a', at: 'x' } }),
        lastEventId: '',
      } as MessageEvent<string>)
      source.onmessage?.({
        data: JSON.stringify({ topic: 'kubelet', payload: { line: 'b', at: 'x' } }),
        lastEventId: '',
      } as MessageEvent<string>)
    })

    const lines = result.current.lines.kubelet ?? []
    expect(lines.map((l) => l.line)).toEqual(['a', 'b'])
    expect(new Set(lines.map((l) => l.id)).size).toBe(2)
  })
})
