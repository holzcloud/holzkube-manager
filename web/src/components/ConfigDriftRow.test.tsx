import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { type Cluster, clusterSchema } from '@/api'
import { ConfigDriftRow } from '@/components/ConfigDriftRow'
import { formatDateTime } from '@/lib/format'
import demo from '../../fixtures/demo.json'

/**
 * The "Configuration" row on a cluster card (15-09, D-05, D-04).
 *
 * The row decides nothing: states, the start of a drift and the banner come from
 * the server. What it must not do is say "fine" about a node nobody could judge,
 * so the tests are about what is counted as matching and what is said instead.
 */

type Health = NonNullable<Cluster['config_drift']>
type Item = Health['items'][number]

function item(node: string, state: Item['state'], since?: string): Item {
  return {
    machine: `m-${node}`,
    node,
    state,
    since: since ?? null,
    read_at: state === 'not-read' ? null : '2026-10-12T09:00:00Z',
    sentence: `${node} is ${state}.`,
  }
}

function health(items: Item[], overrides: Partial<Health> = {}): Health {
  return {
    checked_at: '2026-10-12T09:05:00Z',
    nodes: items.length,
    read_nodes: items.filter((i) => i.state !== 'not-read' && i.state !== 'unreadable').length,
    drifted: items.filter((i) => i.since).length,
    banner: false,
    warning: '',
    items,
    ...overrides,
  }
}

const THREE_OK = [item('cp-1', 'ok'), item('cp-2', 'ok'), item('cp-3', 'ok')]

describe('the configuration row of a cluster card', () => {
  it('says "not read yet" when the server sends no config_drift, never "no drift"', () => {
    for (const missing of [undefined, null]) {
      const { unmount } = render(<ConfigDriftRow health={missing} />)
      expect(screen.getByText('not read yet')).toBeInTheDocument()
      expect(document.body.textContent).not.toMatch(/match|no drift|differ/i)
      unmount()
    }
  })

  it('says that all nodes match their baseline, without a warning colour', () => {
    render(<ConfigDriftRow health={health(THREE_OK)} />)
    const line = screen.getByText('3 nodes match their baseline')
    expect(line.className).not.toMatch(/amber|red/)
    expect(
      screen.getByText(new RegExp(`Last read ${formatDateTime('2026-10-12T09:05:00Z')}`)),
    ).toBeInTheDocument()
  })

  it('names the node that differs, in a warning colour, and since when', () => {
    render(
      <ConfigDriftRow
        health={health([
          item('cp-1', 'ok'),
          item('cp-2', 'drift', '2026-10-11T07:30:00Z'),
          item('cp-3', 'ok'),
        ])}
      />,
    )
    const line = screen.getByText('1 of 3 nodes differs from its baseline: cp-2')
    expect(line.className).toMatch(/amber/)
    expect(document.body.textContent).toContain(formatDateTime('2026-10-11T07:30:00Z'))
    // The others match, and that is said as well: it is a fact about them.
    expect(screen.getByText('2 of 3 nodes match their baseline')).toBeInTheDocument()
  })

  it('still counts a node that stopped answering while it drifts, and says its drift is the last known', () => {
    render(
      <ConfigDriftRow
        health={health([
          item('cp-1', 'ok'),
          item('cp-2', 'unreadable', '2026-10-11T07:30:00Z'),
          item('cp-3', 'ok'),
        ])}
      />,
    )
    expect(
      screen.getByText('1 of 3 nodes differs from its baseline: cp-2 (last known)'),
    ).toBeInTheDocument()
  })

  it('says how many nodes answered, and that the rest show their last reading', () => {
    render(<ConfigDriftRow health={health(THREE_OK, { read_nodes: 2 })} />)
    expect(
      screen.getByText('2 of 3 nodes answered; the rest show their last reading.'),
    ).toBeInTheDocument()
  })

  it('lists a node without a judgement with its state and never counts it as matching', () => {
    render(
      <ConfigDriftRow
        health={health([
          item('cp-1', 'ok'),
          item('cp-2', 'not-read'),
          item('cp-3', 'baseline-damaged'),
          item('w-1', 'unreadable'),
        ])}
      />,
    )
    expect(screen.getByText('1 of 4 nodes match their baseline')).toBeInTheDocument()
    expect(screen.getByText(/3 nodes cannot be judged/)).toBeInTheDocument()

    const list = screen.getByRole('list')
    expect(within(list).getByText('cp-2')).toBeInTheDocument()
    expect(within(list).getByText('not read yet')).toBeInTheDocument()
    expect(within(list).getByText('baseline unusable')).toBeInTheDocument()
    expect(within(list).getByText('cannot be read')).toBeInTheDocument()
  })

  it('says that a baseline was just created instead of that the node matches', () => {
    render(
      <ConfigDriftRow health={health([item('cp-1', 'baseline-created'), item('cp-2', 'ok')])} />,
    )
    expect(screen.getByText('1 of 2 nodes match their baseline')).toBeInTheDocument()
    expect(screen.getByText(/The baseline of 1 node was just created/)).toBeInTheDocument()
  })

  it('puts the per-node list behind a control that is a tap target on a phone', () => {
    render(<ConfigDriftRow health={health(THREE_OK)} />)
    const summary = screen.getByText('All nodes')
    expect(summary.tagName).toBe('SUMMARY')
    expect(summary.className).toContain('max-md:min-h-11')
  })

  it('is read by the cluster schema in the wire format, and its absence is tolerated', () => {
    // The demo cluster carries a reading for the screenshots; this test is about
    // a cluster that has none.
    const base = {
      ...(demo as unknown as { '/api/v1/clusters': { clusters: Record<string, unknown>[] } })[
        '/api/v1/clusters'
      ].clusters[0],
      config_drift: undefined,
    } as Record<string, unknown>

    const without = clusterSchema.safeParse(base)
    expect(without.success ? null : JSON.stringify(without.error.issues)).toBeNull()
    expect(without.success && without.data.config_drift).toBeFalsy()

    const wire = {
      ...base,
      config_drift: {
        checked_at: '2026-10-12T09:05:00Z',
        nodes: 2,
        read_nodes: 2,
        drifted: 1,
        banner: false,
        items: [
          {
            machine: 'm-1',
            node: 'cp-1',
            state: 'ok',
            read_at: '2026-10-12T09:00:00Z',
            sentence: 'fine',
          },
          {
            machine: 'm-2',
            node: 'cp-2',
            state: 'drift',
            since: '2026-10-11T07:30:00Z',
            sentence: 'differs',
          },
        ],
      },
    }
    const parsed = clusterSchema.safeParse(wire)
    expect(parsed.success ? null : JSON.stringify(parsed.error.issues)).toBeNull()
    expect(parsed.success && parsed.data.config_drift?.items).toHaveLength(2)
    expect(parsed.success && parsed.data.config_drift?.warning).toBe('')
  })
})
