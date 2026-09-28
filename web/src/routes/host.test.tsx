import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type Host, hostSchema } from '@/api'
import { HostPage, HostView } from './host'

/**
 * The host page (Phase 11, HOST-01).
 *
 * The failure these guard against is not a wrong number. It is a value that
 * could not be read, drawn as if it had been: an empty cell, or a zero. So every
 * shape here goes through hostSchema first -- the same parse a real answer
 * takes -- and the assertions are about what the operator reads.
 */

vi.mock('@/routes/__root', () => ({
  authenticatedRoute: { addChildren: () => undefined },
}))

function wrap(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const read = <T,>(value: T) => ({ readable: true as const, value })
const hidden = (code: string, message: string) => ({
  readable: false as const,
  reason: { code, message },
})

function hostShape(
  overrides: Record<string, unknown> = {},
  device: Record<string, unknown> = {},
): Host {
  return hostSchema.parse({
    observed_at: '2026-09-28T10:00:03Z',
    container: false,
    device: {
      hostname: read('example-host'),
      kernel: read('6.18.50+rpt-rpi-2712'),
      ...device,
    },
    ...overrides,
  })
}

/** The dd that follows the dt with this label. */
function cellOf(label: string): HTMLElement {
  const dt = screen.getByText(label, { selector: 'dt' })
  const dd = dt.nextElementSibling
  if (!(dd instanceof HTMLElement) || dd.tagName !== 'DD') {
    throw new Error(`no dd after the ${label} row`)
  }
  return dd
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('HostView', () => {
  it('shows the page, the Device card and the uname readings', () => {
    wrap(<HostView host={hostShape()} stale={null} />)

    expect(screen.getByRole('heading', { level: 1, name: 'Host' })).toBeInTheDocument()
    expect(
      screen.getByText('The machine holzkube-manager runs on, and the service itself.'),
    ).toBeInTheDocument()
    expect(screen.getByText('Device')).toBeInTheDocument()

    const hostname = cellOf('Hostname')
    expect(hostname).toHaveTextContent('example-host')
    expect(within(hostname).getByText('example-host')).toHaveClass('font-mono')
    expect(cellOf('Kernel')).toHaveTextContent('6.18.50+rpt-rpi-2712')
    expect(screen.getByText(/every 3 s while this page is open/)).toBeInTheDocument()
  })

  it('draws an unreadable hostname as Not readable with its reason, never an empty cell', () => {
    wrap(
      <HostView
        host={hostShape({}, { hostname: hidden('read-failed', 'Could not read uname(2): nope') })}
        stale={null}
      />,
    )

    const hostname = cellOf('Hostname')
    expect(hostname).toHaveTextContent('Not readable')
    expect(hostname).toHaveTextContent('Could not read uname(2): nope')
    // Its neighbour is untouched.
    expect(cellOf('Kernel')).toHaveTextContent('6.18.50+rpt-rpi-2712')
  })

  it('keeps an old reading on screen, dimmed, under the amber notice', () => {
    wrap(<HostView host={hostShape()} stale={new Error('Network down')} />)

    const notice = screen.getByText(/holzkube-manager did not answer the latest request/)
    expect(notice).toHaveTextContent('Network down')
    expect(cellOf('Hostname').closest('.opacity-60')).not.toBeNull()
  })
})

describe('HostPage', () => {
  it('says it is reading before the first answer, and shows no cards', () => {
    vi.spyOn(api, 'host').mockReturnValue(new Promise<Host>(() => {}))
    wrap(<HostPage />)

    expect(screen.getByText('Reading the host…')).toBeInTheDocument()
    expect(screen.queryByText('Device')).toBeNull()
  })

  it('says the readings could not be loaded when the first poll fails', async () => {
    vi.spyOn(api, 'host').mockRejectedValue(new Error('The server did not answer.'))
    wrap(<HostPage />)

    expect(await screen.findByText('The host readings could not be loaded.')).toBeInTheDocument()
    expect(screen.getByText('The server did not answer.')).toBeInTheDocument()
    expect(screen.getByText('This page keeps asking every 3 s.')).toBeInTheDocument()
    expect(screen.queryByText('Device')).toBeNull()
  })

  it('renders the answer once it arrives', async () => {
    vi.spyOn(api, 'host').mockResolvedValue(hostShape())
    wrap(<HostPage />)

    expect(await screen.findByText('example-host')).toBeInTheDocument()
  })
})
