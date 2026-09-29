import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, hardwareSchema, hostSchema, type Machine, type Temperature } from '@/api'
import { FanList, Sensors } from '@/components/charts/Sensors'
import { NodeHardware } from '@/components/NodeHardware'
import { forget } from '@/hooks/useLiveSeries'
import demo from '../../../fixtures/demo.json'

/**
 * The shared sensor list and fan rows (D-13). The node page draws each
 * sensor's recent past beside its figure; the host page has no curves this
 * phase and draws no empty column in their place. Both come from one
 * component, so what decides between them is whether `history` is passed.
 */

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  Link: ({ children }: { children: ReactNode }) => <a href="#link">{children}</a>,
}))

const cpu: Temperature = {
  chip: 'cpu_thermal',
  kind: 'cpu',
  label: 'temp1',
  celsius: 64.4,
  high_c: null,
  critical_c: null,
  warn_c: 80,
  danger_c: 95,
}

afterEach(() => {
  vi.restoreAllMocks()
  forget()
})

describe('Sensors', () => {
  it('draws a sparkline per sensor when there is a history', () => {
    render(<Sensors temperatures={[cpu]} history={{}} emptyText="none" />)
    expect(screen.getByRole('img', { name: /temp1 temperature/ })).toBeInTheDocument()
  })

  it('draws no sparkline column without a history', () => {
    const { container } = render(<Sensors temperatures={[cpu]} emptyText="none" />)
    expect(screen.getByText('64 °C')).toBeInTheDocument()
    expect(container.querySelector('svg')).toBeNull()
  })

  it('says the page’s own sentence when there is no sensor', () => {
    render(<Sensors temperatures={[]} emptyText="This machine reports no temperature sensors." />)
    expect(screen.getByText('This machine reports no temperature sensors.')).toBeInTheDocument()
  })
})

describe("the limits are the server's", () => {
  /** One CPU sensor, its lines as the server sent them. */
  function rowOf(celsius: number, warn_c: number, danger_c: number) {
    render(
      <Sensors
        temperatures={[{ ...cpu, high_c: null, critical_c: null, celsius, warn_c, danger_c }]}
        emptyText="none"
      />,
    )
    return screen.getByText('temp1').closest('li') as HTMLElement
  }

  /**
   * The colour of a row's ▲. It is the severity's, through the same
   * SEVERITY_COLOR the sparkline and the sidebar use: a ▲ that said red for
   * a warning disagreed with the amber notice above it (12-UI-REVIEW,
   * 13-UI-SPEC checker resolution 4).
   */
  function markColor(row: HTMLElement): string {
    return within(row).getByText('▲').style.color
  }

  it('marks a warning with an amber ▲, never the danger red', () => {
    const row = rowOf(85, 80, 95)
    expect(row.getAttribute('data-severity')).toBe('warn')
    expect(markColor(row)).toBe('var(--viz-warn)')
  })

  it('marks a reading past the danger line with a red ▲', () => {
    const row = rowOf(96, 80, 95)
    expect(row.getAttribute('data-severity')).toBe('danger')
    expect(markColor(row)).toBe('var(--viz-danger)')
  })

  it('warns at 60 °C when the server says 55, below any CPU default', () => {
    const row = rowOf(60, 55, 90)
    expect(row.getAttribute('data-severity')).toBe('warn')
    expect(within(row).getByText('high')).toHaveClass('sr-only')
  })

  it('stays calm at 85 °C when the server says 90, above any CPU default', () => {
    const row = rowOf(85, 90, 100)
    expect(row.getAttribute('data-severity')).toBe('ok')
    expect(within(row).queryByText('▲')).toBeNull()
  })

  it('is critical at the danger line and past it', () => {
    for (const celsius of [100, 104]) {
      const row = rowOf(celsius, 90, 100)
      expect(row.getAttribute('data-severity')).toBe('danger')
      expect(within(row).getByText('critical')).toHaveClass('sr-only')
      cleanup()
    }
  })

  it('draws the sparkline up to the danger line', () => {
    render(
      <Sensors
        temperatures={[{ ...cpu, celsius: 30, warn_c: 55, danger_c: 60 }]}
        history={{
          'temp:cpu_thermal/temp1': [
            { t: 0, v: 60 },
            { t: 1, v: 30 },
          ],
        }}
        emptyText="none"
      />,
    )
    // 22 px high with a 2 px margin: the danger line is y 2, half of it y 11.
    const svg = screen.getByRole('img', { name: /temp1 temperature/ })
    expect(svg.querySelectorAll('path')[1]?.getAttribute('d')).toBe('M0.0,2.0 L64.0,11.0')
  })

  // WR-04: a recorded slot with no reading breaks the sparkline too.
  it('breaks the sparkline where a recorded sample is missing', () => {
    render(
      <Sensors
        temperatures={[{ ...cpu, celsius: 30, warn_c: 55, danger_c: 60 }]}
        history={{
          'temp:cpu_thermal/temp1': [
            { t: 0, v: 30 },
            { t: 15_000, v: 31 },
            { t: 45_000, v: 32, gap: true },
            { t: 60_000, v: 33 },
          ],
        }}
        emptyText="none"
      />,
    )
    const svg = screen.getByRole('img', { name: /temp1 temperature/ })
    expect(svg.querySelectorAll('path')[1]?.getAttribute('d')?.match(/M/g)).toHaveLength(2)
  })

  it('refuses a temperature without its lines, on both pages', () => {
    const fixtures = demo as Record<string, unknown>
    const host = structuredClone(fixtures['/api/v1/host']) as {
      live: { sensors: { value: { temperatures: Record<string, unknown>[] } } }
    }
    const hardware = structuredClone(fixtures['/api/v1/machines/m-cp-1/hardware']) as {
      temperatures: Record<string, unknown>[]
    }
    // The control: as the server sends them, both parse.
    expect(hostSchema.safeParse(host).success).toBe(true)
    expect(hardwareSchema.safeParse(hardware).success).toBe(true)

    delete host.live.sensors.value.temperatures[0]?.warn_c
    delete hardware.temperatures[0]?.danger_c
    expect(hostSchema.safeParse(host).success).toBe(false)
    expect(hardwareSchema.safeParse(hardware).success).toBe(false)
  })
})

describe('FanList', () => {
  it('renders the page’s empty node for no fans, and rows otherwise', () => {
    const { rerender } = render(<FanList fans={[]} empty={<p>no fan here</p>} />)
    expect(screen.getByText('no fan here')).toBeInTheDocument()

    rerender(
      <FanList
        fans={[
          { chip: 'nct6798', label: 'fan1', rpm: 1080 },
          { chip: 'nct6798', label: 'fan2', rpm: 0 },
        ]}
        empty={<p>no fan here</p>}
      />,
    )
    expect(screen.getByText('1080 RPM')).toBeInTheDocument()
    expect(screen.getByText('stopped')).toBeInTheDocument()
    expect(screen.queryByText('no fan here')).toBeNull()
  })
})

describe('the fan icon', () => {
  function classesOf(el: Element | null | undefined): string[] {
    return (el?.getAttribute('class') ?? '').split(/\s+/).filter(Boolean)
  }

  it('spins only for a reader who has not asked for reduced motion', () => {
    const { container } = render(
      <FanList
        fans={[
          { chip: 'nct6798', label: 'fan1', rpm: 1080 },
          { chip: 'nct6798', label: 'fan2', rpm: 0 },
        ]}
        empty={null}
      />,
    )
    const [turning, still] = Array.from(container.querySelectorAll('svg'))
    expect(classesOf(turning)).toContain('motion-safe:animate-spin')
    expect(classesOf(turning)).not.toContain('animate-spin')
    expect(classesOf(still).some((c) => c.endsWith('animate-spin'))).toBe(false)
  })
})

describe('the node page after the move', () => {
  it('keeps a sparkline beside every sensor', async () => {
    vi.spyOn(api.machines, 'hardware').mockResolvedValue(
      hardwareSchema.parse({
        machine: 'm-1',
        hostname: 'wk-02',
        observed_at: '2026-09-26T10:00:00Z',
        uptime_seconds: 3600,
        rates_over_seconds: 3,
        cpu: { model: 'x', cores: 1, threads: 1, usage_percent: 1, per_core: [1] },
        memory: { total_bytes: 2 ** 30, used_bytes: 2 ** 29, cache_bytes: 0 },
        temperatures: [{ ...cpu, chip: 'coretemp', label: 'Package id 0' }],
        fans: [],
        sensors_notice: '',
      }),
    )
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const machine = {
      id: 'm-1',
      cluster: '',
      hostname: { value: 'wk-02', level: 'node', available: true, unavailable_reason: '' },
    } as unknown as Machine
    render(
      <QueryClientProvider client={client}>
        <NodeHardware machine={machine} />
      </QueryClientProvider>,
    )

    expect(await screen.findByRole('img', { name: /Package id 0 temperature/ })).toBeInTheDocument()
  })
})
