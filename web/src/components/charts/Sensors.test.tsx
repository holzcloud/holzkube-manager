import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, hardwareSchema, type Machine, type Temperature } from '@/api'
import { FanList, Sensors } from '@/components/charts/Sensors'
import { NodeHardware } from '@/components/NodeHardware'
import { forget } from '@/hooks/useLiveSeries'

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
