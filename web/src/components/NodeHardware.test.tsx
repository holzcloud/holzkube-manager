import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type Hardware, hardwareSchema, type Machine } from '@/api'
import { temperatureLimits } from '@/components/charts/Sensors'
import { cpuTemperature, NodeHardware } from '@/components/NodeHardware'
import { append, forget } from '@/hooks/useLiveSeries'
import { formatBytes, formatCores, formatRate } from '@/lib/format'

/**
 * The live hardware view (2026-09-26).
 *
 * The tests are about the three ways this screen could lie: a sensor the node
 * does not report shown as zero, a hot sensor shown in the same calm way as a
 * cool one, and a stopped fan confused with a fan that is not there.
 */

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  Link: ({ children }: { children: ReactNode }) => <a href="#link">{children}</a>,
}))

function wrap(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const reading: Hardware = hardwareSchema.parse({
  machine: 'm-1',
  hostname: 'wk-02',
  observed_at: '2026-09-26T10:00:00Z',
  uptime_seconds: 3600,
  rates_over_seconds: 3,
  cpu: {
    model: 'Intel Core i5-9600K',
    cores: 6,
    threads: 6,
    usage_percent: 18,
    per_core: [3, 95],
    load1: 1,
    load5: 1,
    load15: 1,
  },
  memory: { total_bytes: 32 * 2 ** 30, used_bytes: 11 * 2 ** 30, cache_bytes: 2 ** 30 },
  disks: [{ name: 'nvme0n1', model: 'Samsung 980', temperature_c: null }],
  temperatures: [
    {
      chip: 'coretemp',
      kind: 'cpu',
      label: 'Package id 0',
      celsius: 57,
      high_c: 80,
      critical_c: 100,
    },
    { chip: 'coretemp', kind: 'cpu', label: 'Core 2', celsius: 100, high_c: 80, critical_c: 100 },
  ],
  fans: [
    { chip: 'nct6798', label: 'CPU_FAN', rpm: 1080 },
    { chip: 'nct6798', label: 'SYS_FAN1', rpm: 0 },
  ],
  sensors_notice: '',
})

const machine = {
  id: 'm-1',
  cluster: '',
  hostname: { value: 'wk-02', level: 'node', available: true, unavailable_reason: '' },
} as unknown as Machine

afterEach(() => {
  vi.restoreAllMocks()
  forget()
})

describe('NodeHardware', () => {
  it('marks a hot sensor with words, not colour alone', async () => {
    vi.spyOn(api.machines, 'hardware').mockResolvedValue(reading)
    wrap(<NodeHardware machine={machine} />)

    const hot = (await screen.findByText('Core 2')).closest('li')
    expect(hot).not.toBeNull()
    expect(hot?.getAttribute('data-severity')).toBe('danger')
    expect(within(hot as HTMLElement).getByText(/critical/)).toBeInTheDocument()

    const calm = screen.getByText('Package id 0').closest('li')
    expect(calm?.getAttribute('data-severity')).toBe('ok')
  })

  it('tells a stopped fan from a missing one, and a missing temperature from zero', async () => {
    vi.spyOn(api.machines, 'hardware').mockResolvedValue(reading)
    wrap(<NodeHardware machine={machine} />)

    expect(await screen.findByText('stopped')).toBeInTheDocument()
    expect(screen.getByText('1080 RPM')).toBeInTheDocument()
    expect(screen.getByText('temperature not reported')).toBeInTheDocument()
    expect(screen.queryByText('0 °C')).toBeNull()
  })

  it('says so when the node reports no sensors, instead of an empty card', async () => {
    vi.spyOn(api.machines, 'hardware').mockResolvedValue({
      ...reading,
      temperatures: [],
      fans: [],
      sensors_notice: 'No sensor driver is loaded in the Talos kernel.',
    })
    wrap(<NodeHardware machine={machine} />)

    expect(await screen.findByText('This node reports no temperature sensors.')).toBeInTheDocument()
    expect(screen.getByText(/This node reports no fans/)).toBeInTheDocument()
    expect(screen.getByText('No sensor driver is loaded in the Talos kernel.')).toBeInTheDocument()
  })
})

describe("the drive figure turns at the server's lines", () => {
  // Through the schema, so a drive's pair that the schema dropped would show.
  function driveFigure(warn: number | null, danger: number | null, celsius: number | null) {
    return hardwareSchema.parse({
      ...reading,
      disks: [
        {
          name: 'sda',
          model: 'WD Red',
          size_bytes: 4e12,
          read_bytes_per_sec: 0,
          write_bytes_per_sec: 0,
          temperature_c: celsius,
          temperature_warn_c: warn,
          temperature_danger_c: danger,
        },
      ],
    })
  }

  async function figureOf(h: Hardware) {
    vi.spyOn(api.machines, 'hardware').mockResolvedValue(h)
    wrap(<NodeHardware machine={machine} />)
    const row = (await screen.findByText('sda')).closest('li') as HTMLElement
    return row
  }

  it('says high past the warning line the server sent', async () => {
    const row = await figureOf(driveFigure(60, 70, 65))
    const figure = row.querySelector('[data-severity]')
    expect(figure?.getAttribute('data-severity')).toBe('warn')
    expect(figure?.textContent).toBe('65 °C — high')
  })

  it('stays calm below a warning line of 70, whatever a drive default would say', async () => {
    const row = await figureOf(driveFigure(70, 80, 65))
    const figure = row.querySelector('[data-severity]')
    expect(figure?.getAttribute('data-severity')).toBe('ok')
    expect(figure?.textContent).toBe('65 °C')
    expect(within(row).queryByText(/high/)).toBeNull()
  })

  it('says a missing temperature in words, never as 0 °C', async () => {
    const row = await figureOf(driveFigure(null, null, null))
    expect(within(row).getByText('temperature not reported')).toBeInTheDocument()
    expect(within(row).queryByText(/0 °C/)).toBeNull()
  })
})

describe('temperature limits', () => {
  it("prefer the chip's own numbers", () => {
    expect(
      temperatureLimits({
        chip: 'x',
        kind: 'cpu',
        label: '',
        celsius: 50,
        high_c: 70,
        critical_c: 90,
      }),
    ).toEqual({ warn: 70, danger: 90 })
  })

  it('fall back to the kind when the chip says nothing', () => {
    expect(
      temperatureLimits({
        chip: 'x',
        kind: 'disk',
        label: '',
        celsius: 50,
        high_c: null,
        critical_c: null,
      }),
    ).toEqual({ warn: 60, danger: 70 })
  })

  it('take the package reading as the processor temperature', () => {
    expect(cpuTemperature(reading)?.label).toBe('Package id 0')
  })
})

describe('the live series', () => {
  it('records each reading once and forgets what fell out of the window', () => {
    append('k', 1_000, { a: 1 }, 10_000)
    // The same reading handed back again, which react-query does on every
    // render: recorded once, not twice.
    expect(append('k', 1_000, { a: 2 }, 10_000).a?.map((p) => p.v)).toEqual([1])
    append('k', 5_000, { a: 3 }, 10_000)
    const series = append('k', 20_000, { a: 4 }, 10_000)
    expect(series.a?.map((p) => p.v)).toEqual([4])
  })
})

describe('units', () => {
  it('prints what the node and Kubernetes mean', () => {
    expect(formatBytes(5.41 * 2 ** 30)).toBe('5.41 GiB')
    expect(formatRate(187_900)).toBe('188 kB/s')
    expect(formatCores(95)).toBe('95m')
    expect(formatCores(2310)).toBe('2.31 cores')
  })
})
