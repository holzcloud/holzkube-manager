import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type Host, type HostOrder, hostSchema } from '@/api'
import { HostActions, HostOrderStatus, orderPhase } from '@/components/HostActions'
import demo from '../../fixtures/demo.json'

/**
 * The host actions on /host (Phase 13): the update button, its dialog with the
 * typed hostname, and the status box that follows the order it placed through
 * the host answer. The server is the gate (the confirm route compares the typed
 * text with uname(2)); what these hold is that the page asks it the right
 * question and says what came back.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

/** The demo host, renamed to the documentation hostname, with actions as given. */
function hostWith(actions?: unknown): Host {
  const base = structuredClone((demo as Record<string, unknown>)['/api/v1/host']) as Record<
    string,
    unknown
  >
  const device = base.device as Record<string, unknown>
  device.hostname = { readable: true, value: 'example-host' }
  if (actions !== undefined) {
    base.actions = actions
  }
  return hostSchema.parse(base)
}

const ID = '3f9c2a7b1d4e8f60'

const held: HostOrder = {
  id: ID,
  action: 'update',
  placed_at: '2026-09-28T10:00:05Z',
  state: 'pending',
}

const noResult = {
  readable: false,
  reason: { code: 'host-action.no-result', message: 'Nothing recorded.' },
}

afterEach(() => vi.restoreAllMocks())

describe('HostActions', () => {
  it('keeps the confirm button off until the hostname is typed exactly', async () => {
    wrap(<HostActions host={hostWith()} onPlaced={() => undefined} />)
    await userEvent.click(screen.getByRole('button', { name: 'Check for updates and install' }))

    expect(
      screen.getByRole('heading', { name: 'Check for updates and install on example-host?' }),
    ).toBeInTheDocument()
    const confirm = screen.getByRole('button', { name: 'Check for updates and install' })
    const field = screen.getByLabelText(/to confirm/)
    expect(field).toHaveAttribute('autocapitalize', 'none')
    expect(confirm).toBeDisabled()

    // What a phone keyboard makes of it: not the hostname.
    await userEvent.type(field, 'Example-host')
    expect(confirm).toBeDisabled()

    await userEvent.clear(field)
    await userEvent.type(field, 'example-host')
    expect(confirm).toBeEnabled()

    // The way out does not say "Cancel": nothing is cancelled, the host keeps
    // running.
    expect(screen.getByRole('button', { name: 'Keep running' })).toBeInTheDocument()
  })

  it('asks for a token with the typed hostname, places the order with it, and hands it on', async () => {
    const confirm = vi
      .spyOn(api.hostActions, 'confirm')
      .mockResolvedValue({ token: 'token-1', expires: '2026-09-28T10:10:05Z' })
    const place = vi.spyOn(api.hostActions, 'place').mockResolvedValue({ order: held })
    const onPlaced = vi.fn()

    wrap(<HostActions host={hostWith()} onPlaced={onPlaced} />)
    await userEvent.click(screen.getByRole('button', { name: 'Check for updates and install' }))
    await userEvent.type(screen.getByLabelText(/to confirm/), 'example-host')
    await userEvent.click(screen.getByRole('button', { name: 'Check for updates and install' }))

    await vi.waitFor(() => expect(onPlaced).toHaveBeenCalledWith(held))
    expect(confirm).toHaveBeenCalledWith('update', 'example-host')
    expect(place).toHaveBeenCalledWith('update', 'token-1')
  })
})

describe('HostOrderStatus', () => {
  it('says the order waits for the helper while the file is there', () => {
    const host = hostWith({ order: held, result: noResult })
    wrap(<HostOrderStatus host={host} held={held} />)

    const box = screen.getByRole('status')
    expect(box).toHaveTextContent(
      'Check for updates and install — order placed. Waiting for the helper to pick it up.',
    )
    expect(box).toHaveTextContent(`Order ${ID} · placed`)
  })

  it("says what the helper did once its result names this order's id", () => {
    const host = hostWith({
      order: { ...held, state: 'picked-up' },
      result: {
        readable: true,
        value: { id: ID, action: 'update', outcome: 'started', at: '2026-09-28T10:00:06Z' },
      },
    })
    wrap(<HostOrderStatus host={host} held={held} />)

    expect(screen.getByRole('status')).toHaveTextContent(
      'Check for updates and install — started. If a newer release exists it is installed and ' +
        'holzkube-manager restarts; the outcome appears here and under Update check.',
    )
  })

  it("never takes another order's result for this one", () => {
    const other = {
      readable: true,
      value: {
        id: '0000000000000000',
        action: 'update',
        outcome: 'rejected',
        at: '2026-09-28T10:00:06Z',
      },
    }
    expect(
      orderPhase(held, hostWith({ order: { ...held, state: 'picked-up' }, result: other })),
    ).toBe('picked-up')
    // An answer from before the daemon reported the order: as it was placed.
    expect(orderPhase(held, hostWith({ order: null, result: noResult }))).toBe('placed')
  })
})
