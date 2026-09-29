import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  api,
  type History,
  type Host,
  historySchema,
  hostSchema,
  type Me,
  type SystemStatus,
} from '@/api'
import { forget } from '@/hooks/useLiveSeries'
import { formatBytes } from '@/lib/format'
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
  const rendered = render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
  return {
    ...rendered,
    client,
    /** Renders the page again with the same query client: the next poll. */
    rerender: (next: ReactNode) =>
      rendered.rerender(<QueryClientProvider client={client}>{next}</QueryClientProvider>),
  }
}

/** Waits until the page's history request has answered. */
async function historySettled(client: QueryClient) {
  await waitFor(() => {
    expect(client.getQueryState(['host', 'history', '1h'])?.status).toBe('success')
  })
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
    filesystems: [mergedRoot()],
    sensors: read(pi5Sensors()),
    network: read(pi5Network()),
    ...overrides,
  }
}

type LinkShape = {
  name: string
  up: boolean | null
  speed_mbit: number | null
  rx_bytes_per_sec: number | null
  tx_bytes_per_sec: number | null
}

const linkOf = (
  name: string,
  up: boolean | null,
  speed: number | null,
  rx: number | null,
  tx: number | null,
): LinkShape => ({ name, up, speed_mbit: speed, rx_bytes_per_sec: rx, tx_bytes_per_sec: tx })

/**
 * The Pi 5's interfaces with a second reading in: eth0 up at 1000 Mbit/s,
 * wlan0 down (no speed, and no rate for a link that moved nothing is still a
 * rate -- here it is 0), and the loopback, docker's bridges and a veth.
 */
function pi5Network(): { physical: LinkShape[]; virtual: LinkShape[] } {
  return {
    physical: [linkOf('eth0', true, 1000, 187900, 42100), linkOf('wlan0', false, null, 0, 0)],
    virtual: [
      linkOf('br-0a1b2c3d4e5f', true, 10000, 410, 260),
      linkOf('docker0', true, 10000, 320, 180),
      linkOf('lo', true, null, 1210, 1210),
      linkOf('veth1a2b3c4', true, 10000, 410, 260),
    ],
  }
}

/** The same interfaces on the first read after start: no rate anywhere. */
function pi5NetworkFirstRead(): { physical: LinkShape[]; virtual: LinkShape[] } {
  const noRate = (l: LinkShape): LinkShape => ({
    ...l,
    rx_bytes_per_sec: null,
    tx_bytes_per_sec: null,
  })
  const n = pi5Network()
  return { physical: n.physical.map(noRate), virtual: n.virtual.map(noRate) }
}

/** The Pi 5's sensors as the daemon reads them: the CPU and the RP1's ADC, no fan. */
function pi5Sensors() {
  return {
    temperatures: [
      {
        chip: 'cpu_thermal',
        kind: 'cpu',
        label: 'temp1',
        celsius: 64.4,
        high_c: null,
        critical_c: 110,
        warn_c: 80,
        danger_c: 110,
      },
      {
        chip: 'rp1_adc',
        kind: 'other',
        label: 'temp1',
        celsius: 55.4,
        high_c: null,
        critical_c: null,
        warn_c: 75,
        danger_c: 90,
      },
    ],
    fans: [],
  }
}

/** statfs("/") on the Pi, beside `df -B1 /`: 125260451840 28882735104 91212472320 25% /. */
const PI_ROOT_USAGE = {
  size_bytes: 125260451840,
  used_bytes: 28882735104,
  available_bytes: 91212472320,
}

/** The production shape: the data directory on the root filesystem, one row. */
function mergedRoot() {
  return {
    mount: '/',
    device: '/dev/mmcblk0p2',
    fstype: 'ext4',
    roles: ['root', 'data directory'],
    usage: read(PI_ROOT_USAGE),
  }
}

const NOT_RECORDED =
  'The update script installed on this machine does not record its checks. Versions from this release on do; the next update brings it.'

function service(overrides: Record<string, unknown> = {}) {
  return {
    version: '0.1.0',
    started_at: '2026-09-28T08:00:00Z',
    uptime_seconds: 7203,
    data_dir: {
      path: '/var/lib/holzkube-manager',
      size: read({ bytes: 432013312, measured_at: '2026-09-28T09:59:40Z' }),
    },
    update: hidden('update.not-recorded', NOT_RECORDED),
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
    filesystems: [mergedRoot()],
    sensors: read(pi5Sensors()),
    network: read(pi5Network()),
  }
}

/**
 * The server's decision, written out by the test -- never worked out from the
 * readings, which would put the browser's own rule back in through a test.
 */
function healthOf(
  state: 'ok' | 'warn' | 'unknown',
  summary: string,
  warnings: string[] = [],
  unreadable: string[] = [],
) {
  return { state, summary, warnings, unreadable }
}

const OK_SUMMARY = 'Temperatures and filesystems are below their thresholds.'

function hostShape(
  overrides: Record<string, unknown> = {},
  device: Record<string, unknown> = {},
): Host {
  return hostSchema.parse({
    live: readableLive(),
    service: service(),
    health: healthOf('ok', OK_SUMMARY),
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

/** The host actions with the helper installed and nothing placed. */
function helperInstalled(overrides: Record<string, unknown> = {}) {
  return {
    order: null,
    result: hidden('host-action.no-result', 'The helper has recorded nothing yet.'),
    available: true,
    missing: [],
    install_commands: [],
    ...overrides,
  }
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

/** A recorded history for the host, as GET /api/v1/host/history answers it. */
function hostHistory(series: Record<string, [number, number][]> = {}): History {
  return historySchema.parse({
    range: '1h',
    step_seconds: 15,
    from: '2026-09-28T09:00:03Z',
    to: '2026-09-28T10:00:03Z',
    series,
  })
}

// HostView asks for the recorded history from Phase 12 on. Every test answers
// it -- empty unless the test says otherwise -- so none reaches the network,
// and the page's live points are forgotten between tests as they would be
// between page loads.
beforeEach(() => {
  vi.spyOn(api, 'hostHistory').mockResolvedValue(hostHistory())
})

afterEach(() => {
  vi.restoreAllMocks()
  forget()
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

const THRESHOLD_RULE =
  "A temperature warns at its chip's own limit, or at its thermal zone's trip point, or at a default for its kind (80 °C for a processor) when neither names one; a filesystem warns at 80% used."

/** The mark on the header's state line. */
function stateMark(): HTMLElement {
  const marks = document.querySelectorAll<HTMLElement>('[data-host-state]')
  expect(marks).toHaveLength(1)
  return marks[0] as HTMLElement
}

/** The amber Warning notice, found by its heading; null when there is none. */
function warningNotice(): HTMLElement | null {
  const heading = screen.queryByText(/^Warning — \d+ thresholds? crossed$/)
  return heading?.closest('div') ?? null
}

describe('the state line and the warning notice', () => {
  it('ok: a filled dot, Healthy and the summary, and no amber notice', () => {
    wrap(<HostView host={hostShape()} stale={null} />)

    const mark = stateMark()
    expect(mark.dataset.hostState).toBe('ok')
    expect(mark.querySelector('.rounded-full:not(.border)')).not.toBeNull()
    expect(mark.querySelector('svg')).toBeNull()
    expect(mark).toHaveAttribute('title', OK_SUMMARY)
    const line = mark.parentElement as HTMLElement
    expect(within(line).getByText('Healthy')).toHaveAttribute('aria-live', 'polite')
    expect(within(line).getByText(OK_SUMMARY)).toBeInTheDocument()
    // The line form carries no sr-only word: the visible one follows it.
    expect(line.querySelector('.sr-only')).toBeNull()
    expect(warningNotice()).toBeNull()
  })

  it('warn with one warning: the triangle, Warning, and a notice with its one sentence', () => {
    const health = healthOf('warn', '1 threshold crossed.', ['cpu_thermal 82.1 °C ≥ 80 °C'])
    wrap(<HostView host={hostShape({ health })} stale={null} />)

    const mark = stateMark()
    expect(mark.dataset.hostState).toBe('warn')
    expect(mark.querySelector('svg')).not.toBeNull()
    const line = mark.parentElement as HTMLElement
    expect(within(line).getByText('Warning')).toHaveAttribute('aria-live', 'polite')
    expect(within(line).getByText('1 threshold crossed.')).toBeInTheDocument()

    const notice = warningNotice()
    if (notice === null) throw new Error('no warning notice')
    expect(within(notice).getByText('Warning — 1 threshold crossed')).toBeInTheDocument()
    expect(
      within(notice)
        .getAllByRole('listitem')
        .map((li) => li.textContent),
    ).toEqual(['cpu_thermal 82.1 °C ≥ 80 °C'])
    expect(notice).toHaveTextContent(THRESHOLD_RULE)
    expect(notice).not.toHaveTextContent('Also not readable')
    // No button, no dismiss, no link.
    expect(within(notice).queryByRole('button')).toBeNull()
    expect(within(notice).queryByRole('link')).toBeNull()
  })

  it("lists several warnings in the server's order, never re-sorted", () => {
    const warnings = [
      'nvme Composite 88.0 °C ≥ 81.85 °C',
      '/srv 97% used ≥ 80%',
      'cpu_thermal 82.1 °C ≥ 80 °C',
    ]
    const health = healthOf('warn', '3 thresholds crossed.', warnings)
    wrap(<HostView host={hostShape({ health })} stale={null} />)

    const notice = warningNotice()
    if (notice === null) throw new Error('no warning notice')
    expect(within(notice).getByText('Warning — 3 thresholds crossed')).toBeInTheDocument()
    expect(
      within(notice)
        .getAllByRole('listitem')
        .map((li) => li.textContent),
    ).toEqual(warnings)
  })

  it('warn plus unreadable values: the warnings, then Also not readable', () => {
    const reason = 'Usage of /srv could not be read: permission denied.'
    const health = healthOf('warn', '1 threshold crossed.', ['/ 91% used ≥ 80%'], [reason])
    wrap(<HostView host={hostShape({ health })} stale={null} />)

    const notice = warningNotice()
    if (notice === null) throw new Error('no warning notice')
    expect(within(notice).getByText(`Also not readable: ${reason}`)).toBeInTheDocument()
  })

  it('unknown: the hollow ring, Not readable, the reasons as summary, and no amber notice', () => {
    const summary =
      'Temperatures could not be read: permission denied. The last reading is older than 45 s.'
    const health = healthOf(
      'unknown',
      summary,
      [],
      [
        'Temperatures could not be read: permission denied.',
        'The last reading is older than 45 s.',
      ],
    )
    wrap(<HostView host={hostShape({ health })} stale={null} />)

    const mark = stateMark()
    expect(mark.dataset.hostState).toBe('unknown')
    expect(mark.querySelector('.rounded-full.border')).not.toBeNull()
    expect(mark.querySelector('svg')).toBeNull()
    const line = mark.parentElement as HTMLElement
    expect(within(line).getByText('Not readable')).toHaveAttribute('aria-live', 'polite')
    expect(within(line).getByText(summary)).toBeInTheDocument()
    // Not readable is never shown as healthy, and never as the amber warning.
    expect(within(line).queryByText('Healthy')).toBeNull()
    expect(warningNotice()).toBeNull()
  })

  it('puts the warning after the stale notice and before the container and hardening notices', () => {
    const health = healthOf('warn', '1 threshold crossed.', ['cpu_thermal 82.1 °C ≥ 80 °C'])
    wrap(
      <HostView
        host={hostShape({ health, container: true, live: procSubsetLive() })}
        stale={new Error('Network down')}
      />,
    )

    const order = [
      screen.getByText(/holzkube-manager did not answer the latest request/),
      warningNotice(),
      screen.getByText(CONTAINER_SENTENCE),
      screen.getByText(HARDENING_HEADLINE).closest('div'),
    ]
    for (let i = 1; i < order.length; i++) {
      const before = order[i - 1] as HTMLElement
      const after = order[i] as HTMLElement
      expect(before.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    }
    // The state line is not dimmed with the stale reading.
    expect(stateMark().closest('.opacity-60')).toBeNull()
  })

  it('agrees with its filesystem meter at the 80 % boundary: amber and the warning at 80 %, neither at 79.99 %', () => {
    const at = (used: number, available: number) =>
      readableLive({
        filesystems: [
          {
            ...mergedRoot(),
            usage: read({ size_bytes: 10_500, used_bytes: used, available_bytes: available }),
          },
        ],
      })
    const warn = healthOf('warn', '1 threshold crossed.', ['/ 80% used ≥ 80%'])
    const first = wrap(
      <HostView host={hostShape({ live: at(80, 20), health: warn })} stale={null} />,
    )

    const meter = () => cardOf('Filesystems').querySelector<HTMLElement>('[data-severity]')
    expect(meter()?.dataset.severity).toBe('warn')
    expect(within(warningNotice() as HTMLElement).getByText('/ 80% used ≥ 80%')).toBeInTheDocument()
    first.unmount()

    wrap(<HostView host={hostShape({ live: at(7999, 2001) })} stale={null} />)
    expect(meter()?.dataset.severity).toBe('ok')
    expect(warningNotice()).toBeNull()
  })

  it('shows the cpu_thermal ▲ exactly when the notice names cpu_thermal', () => {
    const hot = pi5Sensors()
    ;(hot.temperatures[0] as { celsius: number }).celsius = 82.1
    const health = healthOf('warn', '1 threshold crossed.', ['cpu_thermal 82.1 °C ≥ 80 °C'])
    const first = wrap(
      <HostView
        host={hostShape({ live: readableLive({ sensors: read(hot) }), health })}
        stale={null}
      />,
    )
    const row = () =>
      within(cardOf('Sensors')).getByText('cpu_thermal').closest('li') as HTMLElement
    expect(row()).toHaveTextContent('▲')
    expect(warningNotice()).toHaveTextContent('cpu_thermal')
    first.unmount()

    wrap(<HostView host={hostShape()} stale={null} />)
    expect(row()).not.toHaveTextContent('▲')
    expect(warningNotice()).toBeNull()
  })

  it('refuses an answer without health rather than calling it healthy', () => {
    const host = hostShape()
    const { health: _, ...withoutHealth } = host
    expect(hostSchema.safeParse(withoutHealth).success).toBe(false)
    expect(hostSchema.safeParse({ ...host, health: { summary: 'x' } }).success).toBe(false)
  })
})

const CONTAINER_SENTENCE =
  "holzkube-manager runs in a container. Kernel, CPU, memory and temperatures are the host's; hostname, network and filesystems are the container's."

describe('the Device card', () => {
  it('shows the seven rows of a Pi 5, in order', () => {
    wrap(<HostView host={hostShape()} stale={null} />)

    const labels = within(cardOf('Device'))
      .getAllByRole('term')
      .map((dt) => dt.textContent)
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
  const zeros = outsideAxes(within(card).queryAllByText(DRAWN_ZERO)).map((node) => node.textContent)
  expect(zeros).toEqual([])
}

/**
 * The nodes that are not a chart's axis labels. A chart's scale starts at
 * "0%" or "0 B/s" whatever it draws; that label is the axis, not a reading, and
 * the reading guards here are about readings.
 */
function outsideAxes(nodes: HTMLElement[]): HTMLElement[] {
  return nodes.filter((node) => node.closest('svg') === null)
}

const HARDENING_HEADLINE = "Some readings are hidden by the service's hardening."

describe('the Live section', () => {
  it('under ProcSubset=pid says Not readable, names the hidden readings and the fix, never 0', () => {
    wrap(<HostView host={hostShape({ live: procSubsetLive() })} stale={null} />)

    expect(screen.getByRole('heading', { level: 2, name: 'Readings' })).toBeInTheDocument()

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
    // Load survives the hardening, and takes the figure's place.
    expect(within(processor).getByText('load 0.52')).toBeInTheDocument()
    expect(within(processor).getByText('5 min 0.41 · 15 min 0.33')).toBeInTheDocument()

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
    expect(within(processor).getByText('18%', { selector: 'p' })).toBeInTheDocument()
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
    expect(within(processor).getByText('load 7.29')).toBeInTheDocument()
    expect(within(processor).getByText('5 min 4.90 · 15 min 2.98')).toBeInTheDocument()
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

const NO_CURVE = 'Not readable — no history'
const OBSERVED = Date.parse('2026-09-28T10:00:03Z')

/** A series recorded every 15 s from `fromMin` to `toMin` minutes before the reading. */
function every15s(fromMin: number, toMin: number, v: number): [number, number][] {
  const out: [number, number][] = []
  for (let t = OBSERVED - fromMin * 60_000; t <= OBSERVED - toMin * 60_000; t += 15_000) {
    out.push([t, v])
  }
  return out
}

describe('the charts', () => {
  it('draws the processor, memory and network curves and a sparkline per core, as a node does', async () => {
    vi.spyOn(api, 'hostHistory').mockResolvedValue(
      hostHistory({
        cpu: every15s(4, 0.25, 21),
        'core:0': every15s(4, 0.25, 12),
        memory: every15s(4, 0.25, 48),
        rx: every15s(4, 0.25, 150_000),
        tx: every15s(4, 0.25, 40_000),
      }),
    )
    const { client } = wrap(<HostView host={hostShape()} stale={null} />)
    await historySettled(client)

    const processor = cardOf('Processor')
    expect(
      within(processor).getByRole('img', { name: 'Processor load, last 5 min' }),
    ).toBeInTheDocument()
    // Core 0 has a recorded curve; the others start with this page's reading.
    expect(within(processor).getByRole('img', { name: 'CPU 0 load' })).toBeInTheDocument()
    for (const i of [1, 2, 3]) {
      expect(
        within(processor).getByRole('img', { name: `CPU ${i} load: collecting` }),
      ).toBeInTheDocument()
    }
    expect(
      within(cardOf('Memory')).getByRole('img', { name: 'Memory in use, last 5 min' }),
    ).toBeInTheDocument()
    expect(
      within(cardOf('Network')).getByRole('img', { name: 'Network throughput, last 5 min' }),
    ).toBeInTheDocument()
    expect(
      within(cardOf('Sensors')).getAllByRole('img', { name: /^temp1 temperature/ }),
    ).toHaveLength(2)
    expect(screen.queryByText(NO_CURVE)).toBeNull()
  })

  it('under ProcSubset=pid with nothing recorded says Not readable — no history for processor and memory, and draws no axis', async () => {
    vi.spyOn(api, 'hostHistory').mockResolvedValue(
      hostHistory({ 'temp:cpu_thermal/temp1': every15s(4, 0.25, 63) }),
    )
    const { client } = wrap(<HostView host={hostShape({ live: procSubsetLive() })} stale={null} />)
    await historySettled(client)

    for (const [title, chart] of [
      ['Processor', /^Processor load/],
      ['Memory', /^Memory in use/],
    ] as const) {
      const card = cardOf(title)
      const sentence = within(card).getByText(NO_CURVE)
      expect(sentence.tagName).toBe('P')
      expect(sentence).toHaveClass('text-xs', 'text-muted-foreground')
      expect(within(card).queryByRole('img', { name: chart })).toBeNull()
      expectNoDrawnZero(card)
    }
    // The hardening hides neither the network nor the sensors: they chart.
    expect(
      within(cardOf('Network')).getByRole('img', { name: /^Network throughput/ }),
    ).toBeInTheDocument()
    expect(
      within(cardOf('Sensors')).getAllByRole('img', { name: /^temp1 temperature/ }),
    ).toHaveLength(2)
  })

  it('hardened now, draws the processor curve it recorded before rather than the sentence', async () => {
    vi.spyOn(api, 'hostHistory').mockResolvedValue(hostHistory({ cpu: every15s(4, 2, 30) }))
    const { client } = wrap(<HostView host={hostShape({ live: procSubsetLive() })} stale={null} />)
    await historySettled(client)

    const processor = cardOf('Processor')
    expect(within(processor).getByRole('img', { name: /^Processor load/ })).toBeInTheDocument()
    expect(within(processor).queryByText(NO_CURVE)).toBeNull()
    // Memory has no earlier stretch: it still says so.
    expect(within(cardOf('Memory')).getByText(NO_CURVE)).toBeInTheDocument()
  })

  it('on the first poll after a restart draws the processor chart, starting now, never Not readable — no history', async () => {
    const noBaseline = hidden('rate.no-baseline', 'Waiting for a second reading')
    const live = readableLive({
      rates_over_seconds: null,
      cpu: { ...readableLive().cpu, usage: noBaseline, per_core: noBaseline },
      network: read(pi5NetworkFirstRead()),
    })
    const { client } = wrap(<HostView host={hostShape({ live })} stale={null} />)
    await historySettled(client)

    const processor = cardOf('Processor')
    expect(within(processor).getByRole('img', { name: /^Processor load/ })).toBeInTheDocument()
    expect(
      within(processor).getByText('The curve starts now; it fills in as readings arrive.'),
    ).toBeInTheDocument()
    expect(screen.queryByText(NO_CURVE)).toBeNull()
  })

  it('draws a stretch without readings as a gap: two runs, nothing between them', async () => {
    vi.spyOn(api, 'hostHistory').mockResolvedValue(
      hostHistory({ cpu: [...every15s(30, 20, 25), ...every15s(10, 0.25, 35)] }),
    )
    const { client } = wrap(<HostView host={hostShape()} stale={null} />)
    await historySettled(client)
    fireEvent.click(screen.getByRole('button', { name: '1 h' }))

    const chart = await within(cardOf('Processor')).findByRole('img', {
      name: 'Processor load, last 1 h',
    })
    const line = chart.querySelector('path[fill="none"]')
    const d = line?.getAttribute('d') ?? ''
    expect(d.match(/M/g)).toHaveLength(2)
  })

  // WR-04: one missing 15-s sample -- an update restart, one failed read --
  // is a gap as well, not a line drawn across it for being short.
  it('draws a single missing sample as a gap too', async () => {
    vi.spyOn(api, 'hostHistory').mockResolvedValue(
      hostHistory({ cpu: [...every15s(20, 5, 25), ...every15s(4.5, 0.25, 35)] }),
    )
    const { client } = wrap(<HostView host={hostShape()} stale={null} />)
    await historySettled(client)
    fireEvent.click(screen.getByRole('button', { name: '1 h' }))

    const chart = await within(cardOf('Processor')).findByRole('img', {
      name: 'Processor load, last 1 h',
    })
    const d = chart.querySelector('path[fill="none"]')?.getAttribute('d') ?? ''
    expect(d.match(/M/g)).toHaveLength(2)
  })

  it('adds no processor or memory point while the hardening hides them, poll after poll', async () => {
    const first = hostShape({ live: procSubsetLive() })
    const second = hostShape({ live: procSubsetLive(), observed_at: '2026-09-28T10:00:06Z' })
    const page = wrap(<HostView host={first} stale={null} />)
    await historySettled(page.client)
    page.rerender(<HostView host={second} stale={null} />)

    for (const title of ['Processor', 'Memory']) {
      expect(within(cardOf(title)).getByText(NO_CURVE)).toBeInTheDocument()
    }
    expect(screen.queryByRole('img', { name: /^Processor load/ })).toBeNull()
    // The network is read both times: its curve has the two points.
    const network = within(cardOf('Network')).getByRole('img', { name: /^Network throughput/ })
    expect(network.closest('figure')?.querySelectorAll('tbody tr')).toHaveLength(2)
  })

  it('says what is true now: Readings, recorded every 15 s, and that hidden values are not recorded', () => {
    wrap(<HostView host={hostShape({ live: procSubsetLive() })} stale={null} />)

    const heading = screen.getByRole('heading', { level: 2, name: 'Readings' })
    expect(heading).toHaveAttribute('id', 'host-readings')
    expect(heading.closest('section')).toHaveAttribute('aria-labelledby', 'host-readings')
    expect(
      screen.getByText(
        'Read every 3 s while this page is open. holzkube-manager also records them every 15 s and keeps the last 24 hours, through a restart.',
      ),
    ).toBeInTheDocument()
    expect(screen.queryByText(/Nothing on this page is stored/)).toBeNull()
    expect(screen.getByText(/, every 3 s while this page is open\./)).toHaveTextContent(
      /Rates are over the last 3\.0 s\. History is sampled every 15 s\.$/,
    )
    const notice = screen.getByText(HARDENING_HEADLINE).closest('div')
    expect(notice).toHaveTextContent(
      'CPU usage, memory and swap are shown as not readable rather than as zero, and are not recorded.',
    )
  })
})

const GiB = 2 ** 30

describe('the Processor card figure', () => {
  it('with usage read shows usage as the figure and the three loads under it', () => {
    wrap(<HostView host={hostShape()} stale={null} />)

    const figure = within(cardOf('Processor')).getByText('18%', { selector: 'p' })
    expect(figure).toHaveClass('text-xl')
    expect(figure.nextElementSibling).toHaveTextContent('load 7.29 · 4.90 · 2.98')
  })

  it('with usage hidden puts the 1-minute load in the figure slot and says usage once, muted, below', () => {
    wrap(<HostView host={hostShape({ live: procSubsetLive() })} stale={null} />)

    const processor = cardOf('Processor')
    const figure = within(processor).getByText('load 0.52')
    expect(figure).toHaveClass('font-semibold', 'text-xl')
    const loads = figure.nextElementSibling
    expect(loads).toHaveTextContent(/^5 min 0\.41 · 15 min 0\.33$/)
    const usage = loads?.nextElementSibling
    expect(usage).toHaveTextContent(
      "Not readableHidden by the unit's ProcSubset=pid — see the note at the top.",
    )
    expect(usage).toHaveClass('text-right')
    // The one strong figure is the load, never a usage figure.
    expect(processor.querySelectorAll('.text-xl')).toHaveLength(1)
  })

  it('draws an unreadable load as MissingValue with its reason, like every missing value', () => {
    const live = readableLive({
      cpu: {
        ...readableLive().cpu,
        load: hidden('read-failed', 'Could not read /proc/loadavg: gone'),
      },
    })
    wrap(<HostView host={hostShape({ live })} stale={null} />)

    const processor = cardOf('Processor')
    const reason = within(processor).getByText('Could not read /proc/loadavg: gone')
    expect(reason.previousElementSibling).toHaveTextContent('Not readable')
    expect(reason.parentElement).toHaveClass('text-right')
    expect(processor).not.toHaveTextContent('Load not readable')
  })

  it('says 1 core for one and 4 cores for four', () => {
    const { unmount } = wrap(<HostView host={hostShape({}, { cores: read(1) })} stale={null} />)
    expect(within(cardOf('Processor')).getByText('1 core')).toBeInTheDocument()
    expect(within(cardOf('Processor')).queryByText('1 cores')).toBeNull()
    unmount()

    wrap(<HostView host={hostShape()} stale={null} />)
    expect(within(cardOf('Processor')).getByText('4 cores')).toBeInTheDocument()
  })
})

describe('the filesystem caption', () => {
  it('names the denominator its df-style figure divides by', () => {
    const live = readableLive({
      filesystems: [
        {
          ...mergedRoot(),
          usage: read({ size_bytes: 105 * GiB, used_bytes: 80 * GiB, available_bytes: 20 * GiB }),
        },
      ],
    })
    wrap(<HostView host={hostShape({ live })} stale={null} />)

    const card = cardOf('Filesystems')
    expect(within(card).getByText('80%')).toBeInTheDocument()
    expect(card).toHaveTextContent('80 GiB of 100 GiB usable · 20 GiB free · /dev/mmcblk0p2')
    expect(card).not.toHaveTextContent('of 105 GiB')
  })
})

/** A recorded update run, as the status file names it. */
function recorded(
  outcome: string,
  installed: string | null,
  latest: string | null,
  checkedAt = '2026-09-28T09:37:03Z',
) {
  return read({ checked_at: checkedAt, installed, latest, outcome })
}

describe('the Service card', () => {
  it('shows the version, how long the process has run, the data directory and its free space', () => {
    wrap(<HostView host={hostShape()} stale={null} />)

    const labels = within(cardOf('Service'))
      .getAllByRole('term')
      .map((dt) => dt.textContent)
    expect(labels).toEqual([
      'Version',
      'Running for',
      'Data directory',
      'Free space',
      'Update check',
    ])

    expect(within(cellOf('Version')).getByText('0.1.0')).toHaveClass('font-mono')
    const running = cellOf('Running for')
    expect(running).toHaveTextContent('2 h 0 min')
    expect(running).toHaveTextContent(`since ${new Date('2026-09-28T08:00:00Z').toLocaleString()}`)

    const dataDir = cellOf('Data directory')
    expect(within(dataDir).getByText('/var/lib/holzkube-manager')).toHaveClass('font-mono')
    // Measured 23 s before the reading was taken.
    expect(dataDir).toHaveTextContent(`${formatBytes(432013312)} · measured 23s ago`)

    expect(cellOf('Free space')).toHaveTextContent(
      `${formatBytes(91212472320)} free of ${formatBytes(125260451840)} on /`,
    )
    expect(within(cellOf('Free space')).getByText('/')).toHaveClass('font-mono')
  })

  it('draws an unmeasured data directory and an unreadable free space as Not readable', () => {
    const live = readableLive({
      filesystems: [
        {
          ...mergedRoot(),
          usage: hidden('read-failed', 'Could not read statfs(/): permission denied'),
        },
      ],
    })
    const svc = service({
      data_dir: {
        path: '/var/lib/holzkube-manager',
        size: hidden(
          'read-failed',
          'Could not read /var/lib/holzkube-manager: the size walk took longer than 5 s',
        ),
      },
    })
    wrap(<HostView host={hostShape({ live, service: svc })} stale={null} />)

    expect(cellOf('Data directory')).toHaveTextContent('Not readable')
    expect(cellOf('Data directory')).toHaveTextContent('the size walk took longer than 5 s')
    expect(cellOf('Free space')).toHaveTextContent('Not readable')
    expect(cellOf('Free space')).toHaveTextContent('permission denied')
    expectNoDrawnZero(cardOf('Service'))
  })

  it('says Not recorded with the server sentence, and invents no check time', () => {
    wrap(<HostView host={hostShape()} stale={null} />)

    const update = cellOf('Update check')
    expect(update).toHaveTextContent('Not recorded')
    expect(update).toHaveTextContent(NOT_RECORDED)
    expect(update).not.toHaveTextContent('Checked')
    expect(update).not.toHaveTextContent('Installed')
  })

  it('in a container says the container sentence the server sent', () => {
    const sentence =
      "holzkube-manager runs in a container, where the host's update timer does not run. A container is updated by pulling a new image."
    wrap(
      <HostView
        host={hostShape({
          container: true,
          service: service({ update: hidden('update.not-recorded', sentence) }),
        })}
        stale={null}
      />,
    )

    expect(cellOf('Update check')).toHaveTextContent(sentence)
    expect(cellOf('Update check')).not.toHaveTextContent(NOT_RECORDED)
  })

  it('says Not readable with the cause for a broken status file', () => {
    const cause =
      'The update status file exists but could not be read: outcome in the update status file is none of current, available, updated, rolled-back, failed'
    wrap(
      <HostView
        host={hostShape({ service: service({ update: hidden('read-failed', cause) }) })}
        stale={null}
      />,
    )

    const update = cellOf('Update check')
    expect(update).toHaveTextContent('Not readable')
    expect(update).toHaveTextContent(cause)
    expect(update).not.toHaveTextContent('Checked')
  })

  it.each([
    ['current', '0.1.0', '0.1.0', 'Up to date.', false],
    ['available', '0.1.0', '0.2.0', '0.2.0 is available.', false],
    ['updated', '0.1.0', '0.1.0', 'Updated to 0.1.0.', false],
    [
      'rolled-back',
      '0.1.0',
      '0.2.0',
      'The update to 0.2.0 was rolled back; 0.1.0 is installed.',
      true,
    ],
    ['failed', '0.1.0', null, 'The last update run failed; 0.1.0 is still installed.', true],
    ['failed', null, null, 'The last update run failed.', true],
  ] as const)(
    'outcome %s (installed %s, latest %s) reads "%s"',
    (outcome, installed, latest, sentence, amber) => {
      wrap(
        <HostView
          host={hostShape({ service: service({ update: recorded(outcome, installed, latest) }) })}
          stale={null}
        />,
      )

      const update = cellOf('Update check')
      const line = within(update).getByText(sentence)
      if (amber) {
        expect(line).toHaveClass('text-amber-700')
      } else {
        expect(line).not.toHaveClass('text-amber-700')
      }
      // Checked 23 min before the reading; the absolute time beside it.
      expect(update).toHaveTextContent(
        `Checked 23m ago · ${new Date('2026-09-28T09:37:03Z').toLocaleString()}`,
      )
      if (installed !== null) {
        expect(
          within(update).getAllByText(installed, { selector: '.font-mono' }).length,
        ).toBeGreaterThan(0)
        expect(update).toHaveTextContent(`Installed ${installed}`)
      } else {
        expect(update).not.toHaveTextContent('Installed')
      }
      if (latest !== null) {
        expect(update).toHaveTextContent(`latest ${latest}`)
      } else {
        expect(update).not.toHaveTextContent('latest')
      }
    },
  )

  it('names a status that disagrees with the running version, ignoring a leading v', () => {
    const mismatchLine = 'The update status names 0.0.9 as installed; this process runs 0.1.0.'
    const { unmount } = wrap(
      <HostView
        host={hostShape({ service: service({ update: recorded('current', '0.0.9', '0.0.9') }) })}
        stale={null}
      />,
    )
    expect(cellOf('Update check')).toHaveTextContent(mismatchLine)
    unmount()

    wrap(
      <HostView
        host={hostShape({ service: service({ update: recorded('current', 'v0.1.0', 'v0.1.0') }) })}
        stale={null}
      />,
    )
    expect(cellOf('Update check')).not.toHaveTextContent('The update status names')
  })
})

describe('the Filesystems card', () => {
  it("draws one meter for root and data directory on one filesystem, at df's Use%", () => {
    wrap(<HostView host={hostShape()} stale={null} />)

    const card = cardOf('Filesystems')
    expect(within(card).getByText('root · data directory')).toBeInTheDocument()
    expect(within(card).getAllByText('/', { selector: '.font-mono' })).toHaveLength(1)
    // df -B1 / printed 25% for these numbers: used / (used + available),
    // rounded up. used / size would say 23%.
    expect(within(card).getByText('25%')).toBeInTheDocument()
    expect(card).toHaveTextContent(
      `${formatBytes(28882735104)} of ${formatBytes(28882735104 + 91212472320)} usable · ${formatBytes(91212472320)} free · /dev/mmcblk0p2`,
    )
    expect(within(card).getByText('/dev/mmcblk0p2')).toHaveClass('font-mono')
  })

  it('draws two meters when the data directory has its own filesystem', () => {
    const live = readableLive({
      filesystems: [
        { ...mergedRoot(), roles: ['root'] },
        {
          mount: '/srv',
          device: '/dev/nvme0n1p1',
          fstype: 'ext4',
          roles: ['data directory'],
          usage: read({ size_bytes: 4096000, used_bytes: 1638400, available_bytes: 1433600 }),
        },
      ],
    })
    wrap(<HostView host={hostShape({ live })} stale={null} />)

    const card = cardOf('Filesystems')
    expect(within(card).getByText('root')).toBeInTheDocument()
    expect(within(card).getByText('data directory')).toBeInTheDocument()
    expect(within(card).queryByText('root · data directory')).toBeNull()
    expect(within(card).getByText('/srv')).toHaveClass('font-mono')
    // Free space names the data directory's filesystem, not the root's.
    expect(cellOf('Free space')).toHaveTextContent(
      `${formatBytes(1433600)} free of ${formatBytes(4096000)} on /srv`,
    )
  })

  it('shows Not readable for one filesystem while the other meter renders', () => {
    const live = readableLive({
      filesystems: [
        { ...mergedRoot(), roles: ['root'] },
        {
          mount: '/srv',
          device: '/dev/nvme0n1p1',
          fstype: 'ext4',
          roles: ['data directory'],
          usage: hidden('read-failed', 'Could not read statfs(/srv/data): permission denied'),
        },
      ],
    })
    wrap(<HostView host={hostShape({ live })} stale={null} />)

    const card = cardOf('Filesystems')
    expect(within(card).getByText('/srv')).toBeInTheDocument()
    expect(card).toHaveTextContent('Not readable')
    expect(card).toHaveTextContent('permission denied')
    expect(within(card).getByText('25%')).toBeInTheDocument()
    expectNoDrawnZero(card)
  })
})

/** The range picker's note when the recorded history could not be read. */
const NO_HISTORY = 'No recorded history yet — the curves start with this page.'

const NO_FAN =
  'No fan reported. Nothing under /sys/class/hwmon on this machine has a fan speed input — either there is no fan, or its controller has no driver.'

describe('the Sensors card', () => {
  it('on the Pi 5 shows the CPU and the ADC by kind, says no fan is reported, and draws one curve per sensor', () => {
    wrap(<HostView host={hostShape()} stale={null} />)

    const card = cardOf('Sensors')
    const processor = within(card).getByText('Processor').parentElement as HTMLElement
    expect(within(processor).getByText('cpu_thermal')).toBeInTheDocument()
    expect(within(processor).getByText('64 °C')).toBeInTheDocument()
    const other = within(card).getByText('Other').parentElement as HTMLElement
    expect(within(other).getByText('rp1_adc')).toBeInTheDocument()
    expect(within(other).getByText('55 °C')).toBeInTheDocument()

    expect(within(card).getByText(NO_FAN)).toBeInTheDocument()
    // The sparkline column is on /host from Phase 12 on: one curve per sensor.
    expect(within(card).getAllByRole('img', { name: /^temp1 temperature/ })).toHaveLength(2)
  })

  it("draws a sensor's recorded history as its sparkline, under the range picker", async () => {
    const observed = Date.parse('2026-09-28T10:00:03Z')
    vi.spyOn(api, 'hostHistory').mockResolvedValue(
      hostHistory({
        'temp:cpu_thermal/temp1': [
          [observed - 60_000, 61.2],
          [observed - 45_000, 62.8],
          [observed - 30_000, 63.9],
        ],
      }),
    )
    wrap(<HostView host={hostShape()} stale={null} />)

    const card = cardOf('Sensors')
    expect(await within(card).findByRole('img', { name: 'temp1 temperature' })).toBeInTheDocument()
    expect(api.hostHistory).toHaveBeenCalledWith('1h')
    for (const range of ['Live', '1 h', '6 h', '24 h']) {
      expect(screen.getByRole('button', { name: range })).toBeInTheDocument()
    }
    // The history answered: nothing says it is missing.
    expect(screen.queryByText(NO_HISTORY)).toBeNull()
  })

  it('says the history is missing when its request fails, and still draws each sensor from the live points', async () => {
    vi.spyOn(api, 'hostHistory').mockRejectedValue(new Error('The server did not answer.'))
    wrap(<HostView host={hostShape()} stale={null} />)

    expect(await screen.findByText(NO_HISTORY)).toBeInTheDocument()
    const card = cardOf('Sensors')
    // One sparkline per sensor, from this page's own reading: the first point
    // of a curve that continues with the page, not an empty column.
    expect(within(card).getAllByRole('img', { name: /^temp1 temperature/ })).toHaveLength(2)
    expect(within(card).getByText('64 °C')).toBeInTheDocument()
  })

  it('on an amd64 board shows its fans, a stopped one as stopped, and marks a hot CPU critical', () => {
    const live = readableLive({
      sensors: read({
        temperatures: [
          {
            chip: 'k10temp',
            kind: 'cpu',
            label: 'Tctl',
            celsius: 96,
            high_c: null,
            critical_c: null,
            warn_c: 80,
            danger_c: 95,
          },
          {
            chip: 'nct6798',
            kind: 'board',
            label: 'SYSTIN',
            celsius: 36,
            high_c: null,
            critical_c: null,
            warn_c: 70,
            danger_c: 85,
          },
        ],
        fans: [
          { chip: 'nct6798', label: 'fan1', rpm: 1080 },
          { chip: 'nct6798', label: 'fan2', rpm: 0 },
        ],
      }),
    })
    wrap(<HostView host={hostShape({ live })} stale={null} />)

    const card = cardOf('Sensors')
    expect(within(card).getByText('1080 RPM')).toBeInTheDocument()
    expect(within(card).getByText('stopped')).toBeInTheDocument()
    expect(within(card).queryByText(NO_FAN)).toBeNull()

    const hot = within(card).getByText('Tctl').closest('li') as HTMLElement
    expect(hot.getAttribute('data-severity')).toBe('danger')
    expect(hot).toHaveTextContent('▲')
    expect(within(hot).getByText(/critical/)).toHaveClass('sr-only')
    expect(within(card).getByText('Mainboard')).toBeInTheDocument()
  })

  it('says the machine reports no temperature sensors, not an empty card', () => {
    const live = readableLive({ sensors: read({ temperatures: [], fans: [] }) })
    wrap(<HostView host={hostShape({ live })} stale={null} />)

    const card = cardOf('Sensors')
    expect(
      within(card).getByText('This machine reports no temperature sensors.'),
    ).toBeInTheDocument()
    expect(within(card).getByText(NO_FAN)).toBeInTheDocument()
  })

  it('says Not readable with the reason when the sensors could not be listed', () => {
    const live = readableLive({
      sensors: hidden('read-failed', 'Could not read /sys/class/hwmon: permission denied'),
    })
    wrap(<HostView host={hostShape({ live })} stale={null} />)

    const card = cardOf('Sensors')
    expect(within(card).getByText('Not readable')).toBeInTheDocument()
    expect(
      within(card).getByText('Could not read /sys/class/hwmon: permission denied'),
    ).toBeInTheDocument()
    expect(within(card).queryByText(NO_FAN)).toBeNull()
    expect(within(card).queryByText(/°C/)).toBeNull()
  })
})

/** The row of the interface with this name. */
function linkRow(card: HTMLElement, name: string): HTMLElement {
  const row = within(card).getByText(name, { selector: '.font-mono' }).closest('li')
  if (!(row instanceof HTMLElement)) throw new Error(`no row for ${name}`)
  return row
}

describe('the Network card', () => {
  it('lists the physical interfaces with state, speed and throughput', () => {
    wrap(<HostView host={hostShape()} stale={null} />)

    const card = cardOf('Network')
    const eth0 = linkRow(card, 'eth0')
    expect(eth0).toHaveTextContent('eth0 · up · 1000 Mbit/s')
    expect(eth0).toHaveTextContent('in 188 kB/s · out 42.1 kB/s')
    expect(eth0).toHaveClass('flex', 'flex-wrap', 'items-baseline', 'justify-between')
    const wlan0 = linkRow(card, 'wlan0')
    expect(wlan0).toHaveTextContent('wlan0 · down')
    expect(wlan0).not.toHaveTextContent('Mbit/s')
    // A rate the server computed as 0 is a reading of an idle link.
    expect(wlan0).toHaveTextContent('in 0 B/s · out 0 B/s')
    expect(within(card).queryByText('Waiting for a second reading')).toBeNull()
  })

  it('says the state is not readable, not down, when the server could not read it', () => {
    const live = readableLive({
      network: read({ physical: [linkOf('usb0', null, null, 10, 10)], virtual: [] }),
    })
    wrap(<HostView host={hostShape({ live })} stale={null} />)
    const row = linkRow(cardOf('Network'), 'usb0')
    expect(row).toHaveTextContent(/^usb0 · state not readablein/)
    expect(row).not.toHaveTextContent('down')
  })

  it('says up without a speed when an up link reports none', () => {
    const live = readableLive({
      network: read({ physical: [linkOf('usb0', true, null, 10, 10)], virtual: [] }),
    })
    wrap(<HostView host={hostShape({ live })} stale={null} />)
    expect(linkRow(cardOf('Network'), 'usb0')).toHaveTextContent(/^usb0 · upin/)
  })

  it('before a second reading shows — for every rate, never 0 B/s, and says why once', () => {
    const live = readableLive({ rates_over_seconds: null, network: read(pi5NetworkFirstRead()) })
    wrap(<HostView host={hostShape({ live })} stale={null} />)

    const card = cardOf('Network')
    // No zero for a rate nobody measured -- checked per element first, so a
    // "0 B/s" is named by its own text rather than hidden in a glued string.
    expect(outsideAxes(within(card).queryAllByText(/0 B\/s/)).map((n) => n.textContent)).toEqual([])
    for (const name of ['eth0', 'wlan0']) {
      expect(linkRow(card, name)).toHaveTextContent('in — · out —')
    }
    expect(within(card).getAllByText('Waiting for a second reading')).toHaveLength(1)
  })

  it('counts the virtual interfaces behind one disclosure a thumb can open', () => {
    wrap(<HostView host={hostShape()} stale={null} />)

    const card = cardOf('Network')
    const summary = within(card).getByText(/virtual interfaces?$/, { selector: 'summary' })
    expect(summary).toHaveTextContent('4 virtual interfaces')
    expect(summary).toHaveClass('min-h-11')
    expect(summary.closest('details')).not.toHaveAttribute('open')
    for (const name of ['br-0a1b2c3d4e5f', 'docker0', 'lo', 'veth1a2b3c4']) {
      expect(linkRow(card, name).closest('details')).toBe(summary.closest('details'))
    }
    expect(within(card).getAllByText(/virtual interface/)).toHaveLength(1)
  })

  it('says 1 virtual interface in the singular, and shows no disclosure for none', () => {
    const one = readableLive({
      network: read({ ...pi5Network(), virtual: [linkOf('lo', false, null, 5, 5)] }),
    })
    const { unmount } = wrap(<HostView host={hostShape({ live: one })} stale={null} />)
    expect(
      within(cardOf('Network')).getByText(/virtual interface/, { selector: 'summary' }),
    ).toHaveTextContent(/^▶1 virtual interface$/)
    unmount()

    const none = readableLive({ network: read({ ...pi5Network(), virtual: [] }) })
    wrap(<HostView host={hostShape({ live: none })} stale={null} />)
    const card = cardOf('Network')
    expect(within(card).queryByText(/virtual interface/)).toBeNull()
    // The chart's own "Show as table" is a details too; it is not a disclosure
    // of interfaces.
    expect([...card.querySelectorAll('details')].filter((d) => !d.closest('figure'))).toEqual([])
  })

  it('says no physical interface was found, and still counts the virtual ones', () => {
    const live = readableLive({ network: read({ ...pi5Network(), physical: [] }) })
    wrap(<HostView host={hostShape({ live })} stale={null} />)

    const card = cardOf('Network')
    expect(within(card).getByText('No physical network interface found.')).toBeInTheDocument()
    expect(within(card).getByText('4 virtual interfaces', { exact: false })).toBeInTheDocument()
  })

  it('marks the card as the container view when the daemon runs in one', () => {
    wrap(<HostView host={hostShape({ container: true })} stale={null} />)
    expect(within(cardOf('Network')).getByText('container')).toBeInTheDocument()
  })

  it('carries no container badge on a host', () => {
    wrap(<HostView host={hostShape()} stale={null} />)
    expect(within(cardOf('Network')).queryByText('container')).toBeNull()
  })

  it('says Not readable with the reason when the interfaces could not be listed', () => {
    const live = readableLive({
      network: hidden('read-failed', 'Could not read /sys/class/net: permission denied'),
    })
    wrap(<HostView host={hostShape({ live })} stale={null} />)

    const card = cardOf('Network')
    expect(within(card).getByText('Not readable')).toBeInTheDocument()
    expect(
      within(card).getByText('Could not read /sys/class/net: permission denied'),
    ).toBeInTheDocument()
    expect(within(card).queryByText('No physical network interface found.')).toBeNull()
  })
})

/**
 * UI-SPEC "populated": one complete readable Pi 5 answer draws every card with
 * every value -- the page as the operator sees it with ProcSubset=all.
 */
describe('the complete Pi 5 page', () => {
  it('renders every live card from one readable answer', () => {
    wrap(<HostView host={hostShape()} stale={null} />)

    const processor = cardOf('Processor')
    for (const i of [0, 1, 2, 3]) {
      expect(within(processor).getByText(`CPU ${i}`)).toBeInTheDocument()
    }
    expect(processor.querySelectorAll('[data-severity]')).toHaveLength(4)

    const memory = cardOf('Memory')
    expect(within(memory).getByText('Memory', { selector: 'span' })).toBeInTheDocument()
    expect(within(memory).getByText('Swap')).toBeInTheDocument()
    expect(memory.querySelectorAll('[data-severity]')).toHaveLength(2)

    const filesystems = cardOf('Filesystems')
    expect(filesystems.querySelectorAll('[data-severity]')).toHaveLength(1)
    expect(within(filesystems).getByText('root · data directory')).toBeInTheDocument()

    const network = cardOf('Network')
    expect(linkRow(network, 'eth0')).toBeInTheDocument()
    expect(linkRow(network, 'wlan0')).toBeInTheDocument()

    const sensors = cardOf('Sensors')
    expect(within(sensors).getByText('cpu_thermal')).toBeInTheDocument()
    expect(within(sensors).getByText('rp1_adc')).toBeInTheDocument()
    expect(within(sensors).getByText(NO_FAN)).toBeInTheDocument()

    expect(screen.queryByText(HARDENING_HEADLINE)).toBeNull()
    expect(screen.getByText(/Rates are over the last 3\.0 s\./)).toBeInTheDocument()
  })
})

const ORDER_ID = '3f9c2a7b1d4e8f60'
const STALE_SENTENCE = /holzkube-manager did not answer the latest request/
const WAITING = 'Waiting for holzkube-manager to come back. This page keeps asking every 3 s.'
const WAITING_POWEROFF =
  'The host is shut down. holzkube-manager answers again once somebody switches the machine on; this page keeps asking every 3 s.'

/** An order the daemon placed at 09:59:50, read at 10:00:03. */
function orderOf(action: string, state = 'picked-up') {
  return { id: ORDER_ID, action, placed_at: '2026-09-28T09:59:50Z', state }
}

function startedFor(action: string) {
  return read({ id: ORDER_ID, action, outcome: 'started', at: '2026-09-28T09:59:51Z' })
}

describe('the order status and the waiting notice', () => {
  it('puts the status box first, before the stale and the warning notices', () => {
    const health = healthOf('warn', '1 threshold crossed.', ['cpu_thermal 82.1 °C ≥ 80 °C'])
    wrap(
      <HostView
        host={hostShape({
          health,
          actions: helperInstalled({ order: orderOf('restart-service', 'pending') }),
        })}
        stale={new Error('Network down')}
      />,
    )

    const box = screen.getByRole('status')
    expect(box).toHaveTextContent('Restart service — order placed.')
    // A pending order and a failed poll: the host was not told to go away, so
    // this is the amber stale notice, not the waiting one.
    const stale = screen.getByText(STALE_SENTENCE)
    expect(box.compareDocumentPosition(stale) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(
      stale.compareDocumentPosition(warningNotice() as HTMLElement) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
  })

  it.each([
    ['restart-service', WAITING],
    ['update', WAITING],
    ['reboot', WAITING],
    ['poweroff', WAITING_POWEROFF],
  ])(
    'a started %s and a failed poll: the waiting notice instead of the stale one, the body still dimmed',
    (action, sentence) => {
      wrap(
        <HostView
          host={hostShape({
            actions: helperInstalled({ order: orderOf(action), result: startedFor(action) }),
          })}
          stale={new Error('Network down')}
          sessionRole="operator"
        />,
      )

      const waiting = screen.getByText(sentence)
      expect(waiting).toHaveClass('border-slate-500/40')
      expect(screen.queryByText(STALE_SENTENCE)).toBeNull()
      expect(cellOf('Hostname').closest('.opacity-60')).not.toBeNull()
      expect(
        screen.getByText('holzkube-manager is not answering; host actions return when it does.'),
      ).toBeInTheDocument()
    },
  )

  it('a picked-up order and a failed poll: waiting too', () => {
    wrap(
      <HostView
        host={hostShape({ actions: helperInstalled({ order: orderOf('reboot') }) })}
        stale={new Error('Network down')}
      />,
    )
    expect(screen.getByText(WAITING)).toBeInTheDocument()
    expect(screen.queryByText(STALE_SENTENCE)).toBeNull()
  })

  it('a failed poll without an order under way: the stale notice, never the waiting one', () => {
    wrap(
      <HostView
        host={hostShape({
          actions: helperInstalled({
            order: orderOf('reboot'),
            result: read({
              id: ORDER_ID,
              action: 'reboot',
              outcome: 'rejected',
              at: '2026-09-28T09:59:51Z',
            }),
          }),
        })}
        stale={new Error('Network down')}
      />,
    )
    expect(screen.getByText(STALE_SENTENCE)).toBeInTheDocument()
    expect(screen.queryByText(WAITING)).toBeNull()
  })

  it('the page answering again: no waiting notice, the box says the host is back', () => {
    // Read at 10:00:03 after 60 s up: booted at 09:59:03 -- before the order
    // at 09:59:50 it would still be "started", so the order here is older.
    const host = hostShape(
      {
        actions: helperInstalled({
          order: null,
          result: read({
            id: ORDER_ID,
            action: 'reboot',
            outcome: 'started',
            at: '2026-09-28T09:58:00Z',
          }),
        }),
      },
      { uptime_seconds: read(60) },
    )
    wrap(<HostView host={host} stale={null} />)

    expect(screen.queryByText(WAITING)).toBeNull()
    expect(screen.queryByText(STALE_SENTENCE)).toBeNull()
    expect(screen.getByRole('status')).toHaveTextContent(
      'Restart host — done. The host restarted and holzkube-manager is back; up since',
    )
  })

  it("says waiting on a page that did not place the order: the second operator's page", () => {
    // Nothing held here: the order is only in the last reading.
    wrap(
      <HostView
        host={hostShape({
          actions: helperInstalled({ order: orderOf('reboot'), result: startedFor('reboot') }),
        })}
        stale={new Error('Network down')}
        sessionRole="operator"
      />,
    )
    expect(screen.getByRole('status')).toHaveTextContent(
      'Restart host — started. The host is restarting.',
    )
    expect(screen.getByText(WAITING)).toBeInTheDocument()
    expect(screen.queryByText(STALE_SENTENCE)).toBeNull()
    // Not placed here, so the box does not take focus from wherever it is.
    expect(screen.getByRole('status')).not.toHaveFocus()
  })

  it('shows no box for an order read more than 15 minutes after it was placed', () => {
    const old = { ...orderOf('reboot'), placed_at: '2026-09-28T09:44:00Z' }
    wrap(<HostView host={hostShape({ actions: helperInstalled({ order: old }) })} stale={null} />)
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('Dismiss status removes the box for that order for the life of the page', async () => {
    const rejected = read({
      id: ORDER_ID,
      action: 'reboot',
      outcome: 'rejected',
      at: '2026-09-28T09:59:51Z',
    })
    const host = hostShape({
      actions: helperInstalled({ order: orderOf('reboot'), result: rejected }),
    })
    const page = wrap(<HostView host={host} stale={null} />)

    await userEvent.click(screen.getByRole('button', { name: 'Dismiss status' }))
    expect(screen.queryByRole('status')).toBeNull()
    // The next poll says the same: still dismissed.
    page.rerender(
      <HostView
        host={hostShape({
          actions: helperInstalled({ order: orderOf('reboot'), result: rejected }),
        })}
        stale={null}
      />,
    )
    expect(screen.queryByRole('status')).toBeNull()
    // A new order is a new box.
    page.rerender(
      <HostView
        host={hostShape({
          actions: helperInstalled({
            order: { ...orderOf('update', 'pending'), id: '0123456789abcdef' },
            result: rejected,
          }),
        })}
        stale={null}
      />,
    )
    expect(screen.getByRole('status')).toHaveTextContent(
      'Check for updates and install — order placed.',
    )
  })

  it('after an order is placed, the dialog closes and focus lands on the status box', async () => {
    const placed = {
      id: ORDER_ID,
      action: 'reboot' as const,
      placed_at: '2026-09-28T10:00:02Z',
      state: 'pending' as const,
    }
    vi.spyOn(api.hostActions, 'confirm').mockResolvedValue({
      token: 'token-1',
      expires: '2026-09-28T10:10:02Z',
    })
    vi.spyOn(api.hostActions, 'place').mockResolvedValue({ order: placed })
    wrap(
      <HostView
        host={hostShape({ actions: helperInstalled() })}
        stale={null}
        sessionRole="operator"
      />,
    )

    await userEvent.click(screen.getByRole('button', { name: 'Restart host' }))
    await userEvent.type(screen.getByLabelText(/to confirm/), 'example-host')
    await userEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: 'Restart host' }),
    )

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    const box = await screen.findByRole('status')
    expect(box).toHaveTextContent('Restart host — order placed.')
    await waitFor(() => expect(box).toHaveFocus())
    // Settled, and still there: nothing handed focus back to the trigger.
    await new Promise((r) => setTimeout(r, 20))
    expect(box).toHaveFocus()
    expect(screen.getByRole('button', { name: 'Restart host' })).toBeDisabled()
  })
})

const INSTALL_COMMANDS = [
  'sudo install -o root -g root -m 0755 deploy/holzkube-manager-host.sh /usr/local/sbin/holzkube-manager-host',
  'sudo install -o root -g root -m 0644 deploy/holzkube-manager-host.path deploy/holzkube-manager-host.service /etc/systemd/system/',
  'sudo systemctl daemon-reload',
  'sudo systemctl enable --now holzkube-manager-host.path',
]
const HELPER_HEADING = 'Host actions need the helper, which is not installed'
const SCRIPT = { item: 'script', path: '/usr/local/sbin/holzkube-manager-host' }
const PATH_UNIT = { item: 'path-unit', path: '/etc/systemd/system/holzkube-manager-host.path' }
const NOT_ENABLED = {
  item: 'not-enabled',
  path: '/etc/systemd/system/paths.target.wants/holzkube-manager-host.path',
}

function helperMissing(missing: unknown[]) {
  return helperInstalled({ available: false, missing, install_commands: INSTALL_COMMANDS })
}

function helperNotice(): HTMLElement | null {
  return screen.queryByText(HELPER_HEADING)?.closest('div') ?? null
}

describe('the helper notice', () => {
  it.each([
    [1, [SCRIPT]],
    [2, [SCRIPT, PATH_UNIT]],
    [3, [SCRIPT, PATH_UNIT, NOT_ENABLED]],
  ])("lists %i missing piece(s), in the server's order, and the install commands", (n, missing) => {
    wrap(<HostView host={hostShape({ actions: helperMissing(missing) })} stale={null} />)

    const notice = helperNotice()
    if (notice === null) throw new Error('no helper notice')
    expect(notice).toHaveClass('border-slate-500/40')
    expect(notice).toHaveTextContent(
      'holzkube-manager never restarts or shuts down this machine itself. A small root-owned helper does, and it knows exactly four orders. Until it is installed, the four buttons above stay off.',
    )
    const items = within(notice).getAllByRole('listitem')
    expect(items).toHaveLength(n)
    items.forEach((li, k) => {
      const path = (missing[k] as { path: string }).path
      expect(li.textContent?.startsWith(`${path} — `)).toBe(true)
      expect(within(li).getByText(path)).toHaveClass('font-mono', 'break-all')
    })
    const pre = notice.querySelector('pre')
    expect(pre?.textContent).toBe(INSTALL_COMMANDS.join('\n'))
    expect(pre).toHaveClass('overflow-x-auto')
    expect(notice).toHaveTextContent(
      "The files are in deploy/ in the release archive; deploy/HOST-HELPER.md explains each step. The service's own unit keeps every line of its hardening.",
    )
    // Nothing to press here: the note goes when the helper is installed.
    expect(within(notice).queryByRole('button')).toBeNull()
  })

  it('says what each piece is and where it comes from', () => {
    wrap(
      <HostView
        host={hostShape({ actions: helperMissing([SCRIPT, PATH_UNIT, NOT_ENABLED]) })}
        stale={null}
      />,
    )
    const items = within(helperNotice() as HTMLElement).getAllByRole('listitem')
    expect(items.map((li) => li.textContent)).toEqual([
      '/usr/local/sbin/holzkube-manager-host — the helper script, from deploy/holzkube-manager-host.sh',
      '/etc/systemd/system/holzkube-manager-host.path — the unit that watches for orders, from deploy/holzkube-manager-host.path (with holzkube-manager-host.service beside it)',
      '/etc/systemd/system/paths.target.wants/holzkube-manager-host.path — holzkube-manager-host.path is installed but not enabled',
    ])
  })

  it('shows nothing when nothing is missing', () => {
    wrap(<HostView host={hostShape({ actions: helperInstalled() })} stale={null} />)
    expect(helperNotice()).toBeNull()
  })

  it('shows nothing in a container: the container notice already says why', () => {
    wrap(
      <HostView
        host={hostShape({ container: true, actions: helperMissing([SCRIPT]) })}
        stale={null}
      />,
    )
    expect(helperNotice()).toBeNull()
    expect(
      screen.getByText('Host actions are only available with the systemd installation.'),
    ).toBeInTheDocument()
  })

  it('comes last in the notice stack', () => {
    const health = healthOf('warn', '1 threshold crossed.', ['cpu_thermal 82.1 °C ≥ 80 °C'])
    wrap(
      <HostView
        host={hostShape({
          health,
          live: procSubsetLive(),
          actions: {
            ...helperMissing([SCRIPT]),
            order: orderOf('reboot', 'withdrawn'),
          },
        })}
        stale={new Error('Network down')}
      />,
    )
    const order = [
      screen.getByRole('status'),
      screen.getByText(STALE_SENTENCE),
      warningNotice(),
      screen.getByText(HARDENING_HEADLINE).closest('div'),
      helperNotice(),
    ]
    for (let k = 1; k < order.length; k++) {
      const before = order[k - 1] as HTMLElement
      const after = order[k] as HTMLElement
      expect(before.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    }
  })
})

describe('HostPage', () => {
  // The page reads the session's role for the host actions (D-16). Answered
  // here, so no test reaches the network for it.
  beforeEach(() => {
    vi.spyOn(api, 'status').mockResolvedValue({
      setup_required: false,
    } as unknown as SystemStatus)
    vi.spyOn(api, 'me').mockResolvedValue({
      id: 'u1',
      username: 'reader-1',
      dry_run: false,
      role: 'reader',
      sso: false,
    } as unknown as Me)
  })

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

  it("hands the session's role to the host actions: a reader is told why they are off", async () => {
    vi.spyOn(api, 'host').mockResolvedValue(hostShape({ actions: helperInstalled() }))
    wrap(<HostPage />)

    expect(
      await screen.findByText(
        'Host actions need the operator role. You are signed in as a reader.',
      ),
    ).toBeInTheDocument()
    for (const b of within(screen.getByRole('group', { name: 'Host actions' })).getAllByRole(
      'button',
    )) {
      expect(b).toBeDisabled()
    }
  })

  it('offers the host actions once the session says operator', async () => {
    vi.spyOn(api, 'me').mockResolvedValue({
      id: 'u2',
      username: 'operator-1',
      dry_run: false,
      role: 'operator',
      sso: false,
    } as unknown as Me)
    vi.spyOn(api, 'host').mockResolvedValue(hostShape({ actions: helperInstalled() }))
    wrap(<HostPage />)

    await waitFor(() => expect(screen.getByRole('button', { name: 'Restart host' })).toBeEnabled())
    expect(document.getElementById('host-actions-reason')).toBeNull()
  })
})
