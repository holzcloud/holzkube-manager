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

/** Everything read, a second reading in: the shape of an unhardened host. */
function readableLive(overrides: Record<string, unknown> = {}) {
  return {
    rates_over_seconds: 3,
    cpu: {
      usage: read(18.2),
      per_core: read([12.5, 0, 40, 97]),
      load: read({ load1: 7.29, load5: 4.9, load15: 2.98, source: 'loadavg' }),
    },
    memory: read({
      total_bytes: 8453947392,
      used_bytes: 4093853696,
      cache_bytes: 3259285504,
      available_bytes: 4360093696,
      swap_total_bytes: 2147467264,
      swap_used_bytes: 1498169344,
    }),
    ...overrides,
  }
}

const HARDENING_STAT =
  "Hidden by the unit's ProcSubset=pid: /proc/stat is not visible to holzkube-manager. Set ProcSubset=all in the unit's [Service] section to show it."
const HARDENING_MEMINFO =
  "Hidden by the unit's ProcSubset=pid: /proc/meminfo is not visible to holzkube-manager. Set ProcSubset=all in the unit's [Service] section to show it."

/** The production unit: /proc/stat and /proc/meminfo hidden, load from sysinfo(2). */
function procSubsetLive() {
  return {
    rates_over_seconds: 3,
    cpu: {
      usage: hidden('hardening.proc-subset', HARDENING_STAT),
      per_core: hidden('hardening.proc-subset', HARDENING_STAT),
      load: read({ load1: 0.52, load5: 0.41, load15: 0.33, source: 'sysinfo' }),
    },
    memory: hidden('hardening.proc-subset', HARDENING_MEMINFO),
  }
}

function hostShape(
  overrides: Record<string, unknown> = {},
  device: Record<string, unknown> = {},
): Host {
  return hostSchema.parse({
    live: readableLive(),
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
    expect(screen.getByText(/, every 3 s while this page is open/)).toBeInTheDocument()
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

/** The Card whose title is this text. */
function cardOf(title: string): HTMLElement {
  const card = screen
    .getByText(title, { selector: '[data-slot="card-title"]' })
    .closest('[data-slot="card"]')
  if (!(card instanceof HTMLElement)) throw new Error(`no card titled ${title}`)
  return card
}

/** A zero drawn where a value belongs: "0%", "0.0%", "0 B". */
const DRAWN_ZERO = /(^|\s)0(\.0)?%|(^|\s)0 B(\s|$)/

/**
 * No element inside `card` shows a zero. Per element rather than over the
 * card's textContent: textContent glues neighbours together ("4 cores" and
 * "0.0%" become "cores0.0%"), and the regex's word boundary then misses the
 * very zero it is looking for (seen while injecting one).
 */
function expectNoDrawnZero(card: HTMLElement) {
  const zeros = within(card)
    .queryAllByText(DRAWN_ZERO)
    .map((node) => node.textContent)
  expect(zeros).toEqual([])
}

const HARDENING_HEADLINE = "Some readings are hidden by the service's hardening."

describe('the Live section', () => {
  it('under ProcSubset=pid says Not readable, names the hidden readings and the fix, never 0', () => {
    wrap(<HostView host={hostShape({ live: procSubsetLive() })} stale={null} />)

    expect(screen.getByRole('heading', { level: 2, name: 'Live' })).toBeInTheDocument()

    const processor = cardOf('Processor')
    const memory = cardOf('Memory')
    for (const card of [processor, memory]) {
      expect(card).toHaveTextContent('Not readable')
      expect(card).toHaveTextContent(
        "Hidden by the unit's ProcSubset=pid — see the note at the top.",
      )
      expectNoDrawnZero(card)
    }
    // No per-core meters, no memory meter: nothing to draw a bar for.
    expect(within(processor).queryByText('CPU 0')).toBeNull()
    expect(within(memory).queryByText('Swap')).toBeNull()
    // Load survives the hardening.
    expect(within(processor).getByText('load 0.52 · 0.41 · 0.33')).toBeInTheDocument()

    const notice = screen.getByText(HARDENING_HEADLINE).closest('div')
    if (!(notice instanceof HTMLElement)) throw new Error('no hardening notice')
    expect(notice).toHaveTextContent('CPU usage, memory and swap are shown as not readable')
    expect(notice).toHaveTextContent('Load is still exact; it comes from sysinfo(2).')
    const fix = within(notice).getByText('ProcSubset=all', { selector: 'code' })
    expect(fix).toHaveClass('block', 'overflow-x-auto')
    expect(notice).toHaveTextContent('ProtectProc=invisible can stay')
  })

  it('draws a readable host: usage, four cores including a real 0 %, memory and load', () => {
    wrap(<HostView host={hostShape()} stale={null} />)

    const processor = cardOf('Processor')
    expect(within(processor).getByText('18%')).toBeInTheDocument()
    expect(within(processor).getByText('4 cores')).toBeInTheDocument()
    expect(within(processor).getByText('load 7.29 · 4.90 · 2.98')).toBeInTheDocument()
    for (const i of [0, 1, 2, 3]) {
      expect(within(processor).getByText(`CPU ${i}`)).toBeInTheDocument()
    }
    // A core that did nothing is a reading, and it is drawn.
    expect(within(processor).getByText('0.0%')).toBeInTheDocument()

    const memory = cardOf('Memory')
    expect(memory).toHaveTextContent('3.81 GiB of 7.87 GiB · 3.04 GiB cache · 4.06 GiB available')
    expect(within(memory).getByText('Swap')).toBeInTheDocument()
    expect(memory).toHaveTextContent('1.4 GiB of 2 GiB')

    expect(screen.queryByText(HARDENING_HEADLINE)).toBeNull()
    expect(screen.getByText(/Rates are over the last 3\.0 s\./)).toBeInTheDocument()
  })

  it('says No swap configured. for a readable swap total of 0, and draws no swap meter', () => {
    const live = readableLive({
      memory: read({
        total_bytes: 8453947392,
        used_bytes: 4093853696,
        cache_bytes: 3259285504,
        available_bytes: 4360093696,
        swap_total_bytes: 0,
        swap_used_bytes: 0,
      }),
    })
    wrap(<HostView host={hostShape({ live })} stale={null} />)

    const memory = cardOf('Memory')
    expect(within(memory).getByText('No swap configured.')).toBeInTheDocument()
    expect(within(memory).queryByText('Swap')).toBeNull()
  })

  it('before a second reading says so, with no core meters and no rates clause', () => {
    const noBaseline = hidden('rate.no-baseline', 'Waiting for a second reading')
    const live = readableLive({
      rates_over_seconds: null,
      cpu: { ...readableLive().cpu, usage: noBaseline, per_core: noBaseline },
    })
    wrap(<HostView host={hostShape({ live })} stale={null} />)

    const processor = cardOf('Processor')
    expect(within(processor).getByText('Waiting for a second reading')).toBeInTheDocument()
    expect(within(processor).queryByText('CPU 0')).toBeNull()
    expectNoDrawnZero(processor)
    expect(within(processor).getByText('load 7.29 · 4.90 · 2.98')).toBeInTheDocument()
    expect(screen.queryByText(/Rates are over/)).toBeNull()
    expect(screen.queryByText(HARDENING_HEADLINE)).toBeNull()
  })

  it('shows no hardening notice when nothing carries the hardening code', () => {
    const failed = hidden('read-failed', 'Could not read /proc/stat: permission denied')
    const live = readableLive({ cpu: { ...readableLive().cpu, usage: failed, per_core: failed } })
    wrap(<HostView host={hostShape({ live })} stale={null} />)

    expect(screen.queryByText(HARDENING_HEADLINE)).toBeNull()
    expect(screen.queryByText('ProcSubset=all')).toBeNull()
    expect(cardOf('Processor')).toHaveTextContent('Could not read /proc/stat: permission denied')
  })

  it('lists only what the response hides', () => {
    const live = { ...procSubsetLive(), memory: readableLive().memory }
    wrap(<HostView host={hostShape({ live })} stale={null} />)

    const notice = screen.getByText(HARDENING_HEADLINE).closest('div')
    expect(notice).toHaveTextContent('so CPU usage is shown as not readable')
    expect(notice).not.toHaveTextContent('memory and swap')
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
