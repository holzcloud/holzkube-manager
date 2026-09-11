import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { LogPanel } from '@/components/LogPanel'
import type { StreamLine } from '@/hooks/useStream'

/**
 * The two things this panel must never do quietly.
 *
 * It must not swallow a gap: a log with an invisible hole in it, read by
 * somebody diagnosing an outage, produces a confident wrong conclusion. And it
 * must not render "nothing is arriving" as one condition, because there are
 * four causes and each calls for something different.
 */

const at = '2026-09-11T12:00:00Z'

describe('LogPanel', () => {
  it('draws a gap as a visible break with the count', () => {
    const lines: StreamLine[] = [
      { id: 1, line: 'before', at },
      { id: -1, at, gap: { missed: 212, through: 400 } },
      { id: 401, line: 'after', at },
    ]
    render(<LogPanel title="kubelet" lines={lines} connection="open" />)

    expect(screen.getByText('212 lines dropped')).toBeInTheDocument()
    expect(screen.getByText('before')).toBeInTheDocument()
    expect(screen.getByText('after')).toBeInTheDocument()
  })

  it('distinguishes the four causes of silence', () => {
    const cases: Array<[StreamLine['state'], string]> = [
      ['live', 'live'],
      ['reconnecting', 'reconnecting'],
      ['rebooting', 'node rebooting'],
      ['disconnected', 'disconnected'],
    ]

    for (const [state, label] of cases) {
      const { unmount } = render(
        <LogPanel title="dmesg" lines={[{ id: 1, state, at }]} connection="open" />,
      )
      // Once in the header, once in the body: the header says where the stream
      // stands now, the body says when it got there.
      expect(screen.getAllByText(new RegExp(label)).length).toBeGreaterThan(0)
      unmount()
    }
  })

  it('says so when there is nothing rather than looking broken', () => {
    render(<LogPanel title="etcd" lines={[]} connection="open" />)
    expect(screen.getByText(/Nothing yet/)).toBeInTheDocument()
  })

  it('shows the connection state when it is not open', () => {
    render(<LogPanel title="etcd" lines={[]} connection="connecting" />)
    expect(screen.getByText('(connecting)')).toBeInTheDocument()
  })
})
