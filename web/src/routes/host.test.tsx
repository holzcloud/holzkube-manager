import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type Host, hostSchema } from '@/api'
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
        critical_c: null,
      },
      {
        chip: 'rp1_adc',
        kind: 'other',
        label: 'temp1',
        celsius: 55.4,
        high_c: null,
        critical_c: null,
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

function hostShape(
  overrides: Record<string, unknown> = {},
  device: Record<string, unknown> = {},
): Host {
  return hostSchema.parse({
    live: readableLive(),
    service: service(),
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
      `${formatBytes(28882735104)} of ${formatBytes(125260451840)} · ${formatBytes(91212472320)} free · /dev/mmcblk0p2`,
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

const NO_FAN =
  'No fan reported. Nothing under /sys/class/hwmon on this machine has a fan speed input — either there is no fan, or its controller has no driver.'

describe('the Sensors card', () => {
  it('on the Pi 5 shows the CPU and the ADC by kind, says no fan is reported, and draws no curve', () => {
    wrap(<HostView host={hostShape()} stale={null} />)

    const card = cardOf('Sensors')
    const processor = within(card).getByText('Processor').parentElement as HTMLElement
    expect(within(processor).getByText('cpu_thermal')).toBeInTheDocument()
    expect(within(processor).getByText('64 °C')).toBeInTheDocument()
    const other = within(card).getByText('Other').parentElement as HTMLElement
    expect(within(other).getByText('rp1_adc')).toBeInTheDocument()
    expect(within(other).getByText('55 °C')).toBeInTheDocument()

    expect(within(card).getByText(NO_FAN)).toBeInTheDocument()
    // No history on this page yet: no sparkline column, not a column of empty ones.
    expect(card.querySelector('svg')).toBeNull()
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
          },
          {
            chip: 'nct6798',
            kind: 'board',
            label: 'SYSTIN',
            celsius: 36,
            high_c: null,
            critical_c: null,
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
    expect(
      within(card)
        .queryAllByText(/0 B\/s/)
        .map((n) => n.textContent),
    ).toEqual([])
    expect(card).not.toHaveTextContent('0 B/s')
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
    expect(card.querySelector('details')).toBeNull()
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
