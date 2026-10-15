import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type ConfigDriftDetail, configDriftDetailSchema } from '@/api'
import { NodeConfigDrift } from '@/components/NodeConfigDrift'
import { formatDateTime } from '@/lib/format'
import { ProblemError } from '@/lib/problem'
import { PROBLEM_BASE_URI } from '@/test/problem-fixtures'

/**
 * The configuration section of the node page (15-09, D-05, D-07, D-11).
 *
 * The claims worth pinning are the ones that keep the page from saying something
 * the server did not: a state without a judgement never looks like "fine", the
 * button sends the digest it shows and exists only while there is a drift, the
 * fixed sentence about secret values is there in every state, and there is no
 * way to put the baseline back from here.
 *
 * The examples are shaped like the ones in docs/api-contract.md and hold
 * documentation values only.
 */

const DIGEST = 'ab'.repeat(32)
const NOTICE =
  'A change of nothing but a secret value, such as a rotated token, is not visible here: ' +
  'snapshots hold the redacted configuration.'

const STATES = [
  'not-read',
  'baseline-created',
  'ok',
  'settling',
  'drift',
  'unreadable',
  'baseline-damaged',
] as const

const SENTENCES: Record<(typeof STATES)[number], string> = {
  'not-read': 'Nothing was read from this node since the daemon started.',
  'baseline-created': 'The first reading of this node is its baseline from here on.',
  ok: 'The node has the configuration of its baseline.',
  settling: 'An apply from here is on its way; a difference is expected for up to 15 minutes.',
  drift: 'This node differs from its baseline and nothing this installation did explains it.',
  unreadable: 'The node could not be read just now; this says nothing about drift.',
  'baseline-damaged': 'The stored baseline cannot be trusted and nothing is compared with it.',
}

/** What the server sends: time fields absent when empty, no diff outside drift. */
function wire(state: (typeof STATES)[number]): unknown {
  const base = { machine: 'm-1', state, sentence: SENTENCES[state], notice: NOTICE }
  if (state === 'not-read') return base
  if (state === 'drift') {
    return {
      ...base,
      checked_at: '2026-10-12T09:00:00Z',
      read_at: '2026-10-12T09:00:00Z',
      baseline_taken_at: '2026-10-01T08:00:00Z',
      baseline_source: 'first-sight',
      drift_since: '2026-10-11T07:30:00Z',
      digest: DIGEST,
      diff: {
        changes: [
          {
            path: 'KubeletConfig.image',
            kind: 'modified',
            before: 'registry.example.com/kubelet:v1.34.0',
            after: 'registry.example.com/kubelet:v1.35.0',
          },
          {
            path: 'MachineConfig.install.extraKernelArgs',
            kind: 'list-changed',
            after: 'quiet',
            len_before: 1,
            len_after: 2,
          },
        ],
        paths: ['KubeletConfig.image', 'MachineConfig.install.extraKernelArgs'],
      },
    }
  }
  return {
    ...base,
    checked_at: '2026-10-12T09:00:00Z',
    read_at: '2026-10-12T09:00:00Z',
    baseline_taken_at: '2026-10-01T08:00:00Z',
    baseline_source: 'first-sight',
    digest: DIGEST,
  }
}

function detail(state: (typeof STATES)[number]): ConfigDriftDetail {
  return configDriftDetailSchema.parse(wire(state))
}

function newClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
}

function renderSection(client = newClient()) {
  const view = render(
    <QueryClientProvider client={client}>
      <NodeConfigDrift machine="m-1" />
    </QueryClientProvider>,
  )
  return { client, unmount: view.unmount }
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the configuration section of the node page', () => {
  it('parses what the server sends, with the time fields absent when empty and no diff outside of drift', () => {
    const drift = configDriftDetailSchema.safeParse(wire('drift'))
    expect(drift.success ? null : JSON.stringify(drift.error.issues)).toBeNull()
    expect(drift.success && drift.data.diff?.changes).toHaveLength(2)
    expect(drift.success && drift.data.diff?.duplicates).toBe(false)
    expect(drift.success && drift.data.diff?.changes[0]?.duplicates).toEqual([])

    const notRead = configDriftDetailSchema.safeParse(wire('not-read'))
    expect(notRead.success ? null : JSON.stringify(notRead.error.issues)).toBeNull()
    expect(notRead.success && notRead.data.diff).toBeFalsy()
    expect(notRead.success && notRead.data.read_at).toBeFalsy()
    expect(notRead.success && notRead.data.drift_since).toBeFalsy()
  })

  it('shows a drift with its path, its values and since when', async () => {
    vi.spyOn(api.configDrift, 'get').mockResolvedValue(detail('drift'))
    renderSection()

    expect(await screen.findByText(SENTENCES.drift)).toBeInTheDocument()
    expect(screen.getByText('KubeletConfig.image')).toBeInTheDocument()
    const before = screen.getByText('registry.example.com/kubelet:v1.34.0')
    expect(before).toHaveClass('line-through')
    expect(screen.getByText('registry.example.com/kubelet:v1.35.0')).toBeInTheDocument()
    // A list says how it changed, not only that it did.
    expect(screen.getByText('MachineConfig.install.extraKernelArgs')).toBeInTheDocument()
    expect(screen.getByText(/1 entries become 2/)).toBeInTheDocument()

    const text = document.body.textContent ?? ''
    expect(text).toContain(formatDateTime('2026-10-11T07:30:00Z'))
    expect(text).toContain(formatDateTime('2026-10-01T08:00:00Z'))
    expect(text).toContain(formatDateTime('2026-10-12T09:00:00Z'))
    expect(text).toMatch(/first seen/)
  })

  it('never shows a state without a judgement as if it were fine', async () => {
    for (const state of ['not-read', 'unreadable', 'baseline-damaged'] as const) {
      vi.spyOn(api.configDrift, 'get').mockResolvedValue(detail(state))
      const { unmount } = renderSection()

      expect(await screen.findByText(SENTENCES[state])).toBeInTheDocument()
      expect(document.body.textContent).not.toMatch(/as last known/i)
      expect(screen.queryByRole('button', { name: /confirm/i })).toBeNull()
      if (state === 'not-read') {
        expect(screen.getAllByText(/not read yet/).length).toBeGreaterThan(0)
      }
      unmount()
      vi.restoreAllMocks()
    }
  })

  it('says that a changed secret value is invisible, in every state', async () => {
    for (const state of STATES) {
      vi.spyOn(api.configDrift, 'get').mockResolvedValue(detail(state))
      const { unmount } = renderSection()
      expect(await screen.findByText(NOTICE), state).toBeInTheDocument()
      unmount()
      vi.restoreAllMocks()
    }
  })

  it('confirms with exactly the digest it shows and then shows the new state', async () => {
    const get = vi
      .spyOn(api.configDrift, 'get')
      .mockResolvedValueOnce(detail('drift'))
      .mockResolvedValue(detail('ok'))
    const confirm = vi.spyOn(api.configDrift, 'confirm').mockResolvedValue(detail('ok'))
    const client = newClient()
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    renderSection(client)

    // The button is there only while there is a drift; ok has none (below).
    await userEvent.click(await screen.findByRole('button', { name: 'Confirm as intended' }))

    await waitFor(() => expect(confirm).toHaveBeenCalledTimes(1))
    expect(confirm).toHaveBeenCalledWith('m-1', DIGEST)

    expect(await screen.findByText(SENTENCES.ok)).toBeInTheDocument()
    expect(screen.getByText('As last known')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /confirm/i })).toBeNull()
    expect(get).toHaveBeenCalledTimes(2)
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['clusters'] })
  })

  it('offers to confirm only while there is a drift', async () => {
    for (const state of STATES) {
      vi.spyOn(api.configDrift, 'get').mockResolvedValue(detail(state))
      const { unmount } = renderSection()
      await screen.findByText(SENTENCES[state])
      const button = screen.queryByRole('button', { name: 'Confirm as intended' })
      if (state === 'drift') {
        expect(button).not.toBeNull()
      } else {
        expect(button, state).toBeNull()
      }
      unmount()
      vi.restoreAllMocks()
    }
  })

  it('offers no way to put the baseline back', async () => {
    for (const state of STATES) {
      vi.spyOn(api.configDrift, 'get').mockResolvedValue(detail(state))
      const { unmount } = renderSection()
      await screen.findByText(SENTENCES[state])
      expect(screen.queryByRole('button', { name: /reset|restore|revert/i }), state).toBeNull()
      expect(screen.queryByRole('link', { name: /reset|restore|revert/i }), state).toBeNull()
      unmount()
      vi.restoreAllMocks()
    }
  })

  it('shows a refused confirmation as the problem it is', async () => {
    const get = vi.spyOn(api.configDrift, 'get').mockResolvedValue(detail('drift'))
    vi.spyOn(api.configDrift, 'confirm').mockRejectedValue(
      new ProblemError({
        type: `${PROBLEM_BASE_URI}conflict.drift-changed`,
        title: 'Conflict',
        status: 409,
        detail: 'The node changed again after you looked; look at the new difference first.',
        code: 'conflict.drift-changed',
      }),
    )
    renderSection()

    await userEvent.click(await screen.findByRole('button', { name: 'Confirm as intended' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'The node changed again after you looked; look at the new difference first.',
    )
    // The page shows the current state again rather than the one that was refused.
    await waitFor(() => expect(get.mock.calls.length).toBeGreaterThanOrEqual(2))
  })
})
