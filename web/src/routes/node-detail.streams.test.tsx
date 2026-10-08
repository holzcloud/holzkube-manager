import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Machine } from '@/api'
import { NodeStreams } from '@/routes/node-detail'

/** The panel toggles are toggle buttons: their state must be exposed, not only coloured. */

class QuietSource {
  static readonly CLOSED = 2
  readyState = 0
  onopen = null
  onmessage = null
  onerror = null
  addEventListener() {}
  close() {}
}

beforeEach(() => vi.stubGlobal('EventSource', QuietSource))

describe('NodeStreams', () => {
  it('exposes which panels are open through aria-pressed', async () => {
    const machine = {
      id: 'm1',
      services: { value: [{ id: 'kubelet' }] },
    } as unknown as Machine
    const user = userEvent.setup()
    render(<NodeStreams machine={machine} />)

    expect(screen.getByRole('button', { name: 'dmesg' })).toHaveAttribute('aria-pressed', 'true')
    const kubelet = screen.getByRole('button', { name: 'kubelet' })
    expect(kubelet).toHaveAttribute('aria-pressed', 'false')

    await user.click(kubelet)
    expect(kubelet).toHaveAttribute('aria-pressed', 'true')
  })
})
