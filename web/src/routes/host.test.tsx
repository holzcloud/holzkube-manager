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
      model: read('Raspberry Pi 5 Model B Rev 1.0'),
      arch: read({ goarch: 'arm64', machine: 'aarch64' }),
      cores: read(4),
      os: read('Debian GNU/Linux 13 (trixie)'),
      kernel: read('6.18.50+rpt-rpi-2712'),
      uptime_seconds: read(26090),
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

const CONTAINER_SENTENCE =
  "holzkube-manager runs in a container. Kernel, CPU, memory and temperatures are the host's; hostname, network and filesystems are the container's."

describe('the Device card', () => {
  it('shows the seven rows of a Pi 5, in order', () => {
    wrap(<HostView host={hostShape()} stale={null} />)

    const labels = screen.getAllByRole('term').map((dt) => dt.textContent)
    expect(labels).toEqual([
      'Hostname',
      'Model',
      'Architecture',
      'Processor cores',
      'Operating system',
      'Kernel',
      'Up since boot',
    ])
    expect(cellOf('Model')).toHaveTextContent('Raspberry Pi 5 Model B Rev 1.0')
    expect(cellOf('Architecture')).toHaveTextContent('arm64 (aarch64)')
    expect(within(cellOf('Architecture')).getByText('arm64 (aarch64)')).toHaveClass('font-mono')
    expect(cellOf('Processor cores')).toHaveTextContent('4 online')
    expect(cellOf('Operating system')).toHaveTextContent('Debian GNU/Linux 13 (trixie)')
    // 26090 s is 7 h 14 min; the boot instant is observed_at minus that.
    const up = cellOf('Up since boot')
    expect(up).toHaveTextContent('7 h 14 min')
    const since = new Date(Date.parse('2026-09-28T10:00:03Z') - 26090 * 1000).toLocaleString()
    expect(up).toHaveTextContent(`since ${since}`)
  })

  it('shows an amd64 desktop the same way', () => {
    wrap(
      <HostView
        host={hostShape(
          {},
          {
            model: read('Example Vendor Example Board X570'),
            arch: read({ goarch: 'amd64', machine: 'x86_64' }),
            cores: read(16),
            os: read('Example Linux 42'),
          },
        )}
        stale={null}
      />,
    )

    expect(cellOf('Model')).toHaveTextContent('Example Vendor Example Board X570')
    expect(cellOf('Architecture')).toHaveTextContent('amd64 (x86_64)')
    expect(cellOf('Processor cores')).toHaveTextContent('16 online')
  })

  it('draws one unreadable row as Not readable while its neighbours render', () => {
    wrap(
      <HostView
        host={hostShape(
          {},
          {
            model: hidden(
              'read-failed',
              'Could not read /sys/firmware/devicetree/base/model: file does not exist',
            ),
          },
        )}
        stale={null}
      />,
    )

    expect(cellOf('Model')).toHaveTextContent('Not readable')
    expect(cellOf('Model')).toHaveTextContent('/sys/firmware/devicetree/base/model')
    expect(cellOf('Hostname')).toHaveTextContent('example-host')
    expect(cellOf('Processor cores')).toHaveTextContent('4 online')
  })

  it('never draws an unread core count or uptime as a zero', () => {
    wrap(
      <HostView
        host={hostShape(
          {},
          {
            cores: hidden('read-failed', 'Could not read /sys/devices/system/cpu/online: gone'),
            uptime_seconds: hidden('unsupported', 'holzkube-manager reads the host on Linux only.'),
          },
        )}
        stale={null}
      />,
    )

    expect(cellOf('Processor cores')).toHaveTextContent('Not readable')
    expect(cellOf('Processor cores')).not.toHaveTextContent('0 online')
    expect(cellOf('Up since boot')).toHaveTextContent('Not readable')
    expect(cellOf('Up since boot')).not.toHaveTextContent('since')
  })

  it('says so when the daemon runs in a container, and marks the hostname', () => {
    wrap(<HostView host={hostShape({ container: true })} stale={null} />)

    expect(screen.getByText(CONTAINER_SENTENCE)).toBeInTheDocument()
    expect(within(cellOf('Hostname')).getByText('container')).toBeInTheDocument()
  })

  it('says nothing about containers when it is not in one', () => {
    wrap(<HostView host={hostShape()} stale={null} />)

    expect(screen.queryByText(CONTAINER_SENTENCE)).toBeNull()
    expect(within(cellOf('Hostname')).queryByText('container')).toBeNull()
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
