import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Candidate, CandidateDisk, Found, Schematic } from '@/api'
import { DiskPicker, FoundTable, PlanStep } from '@/routes/provision'

/**
 * The two places this screen can mislead somebody into wiping the wrong thing.
 *
 * Neither is about layout. A scan that reports a running node as "nothing" and
 * a disk picker that cannot tell two identical disks apart both produce the
 * same outcome, which is an operator confidently choosing the wrong machine.
 */

function found(over: Partial<Found>): Found {
  return {
    addr: '10.0.0.5',
    state: 'maintenance',
    hostname: '',
    fingerprint: '',
    known: false,
    detail: '',
    ...over,
  }
}

describe('the scan result', () => {
  it('reports a configured machine as configured, never as nothing (PROV-02)', () => {
    render(
      <FoundTable
        found={[
          found({
            addr: '10.0.0.7',
            state: 'configured',
            detail: 'This machine already has a configuration.',
          }),
        ]}
        onInspect={vi.fn()}
      />,
    )

    expect(screen.getByText('Already configured')).toBeInTheDocument()
    expect(screen.getByText(/already has a configuration/)).toBeInTheDocument()
  })

  it('offers to inspect only a machine that is waiting for a configuration', () => {
    render(
      <FoundTable
        found={[found({ addr: '10.0.0.5' }), found({ addr: '10.0.0.7', state: 'configured' })]}
        onInspect={vi.fn()}
      />,
    )

    // One button, for the one machine the wizard can act on. A second would be
    // an offer to overwrite a running node.
    expect(screen.getAllByRole('button', { name: 'Inspect' })).toHaveLength(1)
  })

  it('shows the fingerprint next to every candidate (PROV-04)', () => {
    render(<FoundTable found={[found({ fingerprint: 'AB:CD:EF' })]} onInspect={vi.fn()} />)
    expect(screen.getByText('AB:CD:EF')).toBeInTheDocument()
  })

  it('says an empty result is about the addresses, not about the machines', () => {
    render(<FoundTable found={[]} onInspect={vi.fn()} />)
    expect(screen.getByText(/without DHCP has no address/)).toBeInTheDocument()
  })
})

describe('the disk picker', () => {
  const disks: CandidateDisk[] = [
    {
      device: '/dev/nvme0n1',
      size: 512_000_000_000,
      pretty_size: '512.0 GB',
      model: 'SAMSUNG MZ',
      serial: 'S1',
      transport: 'nvme',
      system: true,
    },
    {
      device: '/dev/nvme1n1',
      size: 512_000_000_000,
      pretty_size: '512.0 GB',
      model: 'SAMSUNG MZ',
      serial: 'S2',
      transport: 'nvme',
      system: false,
    },
  ]

  it('shows the serial, which is the only thing telling two identical disks apart (PROV-06)', () => {
    render(<DiskPicker disks={disks} chosen="" onChoose={vi.fn()} />)

    expect(screen.getByText('S1')).toBeInTheDocument()
    expect(screen.getByText('S2')).toBeInTheDocument()
    expect(screen.getAllByText('512.0 GB')).toHaveLength(2)
  })

  it('marks the system disk and pre-selects nothing', () => {
    render(<DiskPicker disks={disks} chosen="" onChoose={vi.fn()} />)

    expect(screen.getByText('system disk')).toBeInTheDocument()
    for (const radio of screen.getAllByRole('radio')) {
      expect(radio).not.toBeChecked()
    }
  })
})

/**
 * The schematic control.
 *
 * It was a free-text field for a 64-character hex id, which is a field whose
 * only use is pasting -- and whose only possible wrong answers were an id this
 * installation does not hold, which the server refuses, and an id it does hold
 * that is not the one the ISO was built from, which nothing catches. The server
 * requires the schematic to be in the store, because the stored record is where
 * the architecture to resolve the installer comes from, so the list of accepted
 * values was always exactly the list this installation can show.
 *
 * A picker over that list therefore loses nothing and removes a way to fail.
 * "No schematic" stays reachable: a machine that booted a stock ISO has none,
 * and that is an ordinary answer.
 */
describe('the schematic control on the plan step', () => {
  const HOMELAB = 'c'.repeat(26)
  const LAB2 = 'd'.repeat(26)
  const ID_A = 'a'.repeat(64)
  const ID_B = 'b'.repeat(64)

  function schematic(over: Partial<Schematic>): Schematic {
    return {
      id: ID_A,
      cluster: '',
      name: 'workers',
      talos_version: 'v1.13.9',
      canonical: 'customization: {}\n',
      extensions: [],
      kernel_args: [],
      meta: [],
      usable: true,
      probed_at: '2026-08-29T10:00:00Z',
      probe_reason: '',
      arch: 'amd64',
      created_at: '2026-08-29T10:00:00Z',
      rev: 1,
      ...over,
    }
  }

  function candidate(): Candidate {
    return {
      addr: '10.0.0.5',
      uuid: '00000000-0000-4000-8000-0000000000bb',
      talos_version: 'v1.13.9',
      hostname: '',
      fingerprint: '',
      macs: [],
      disks: [
        {
          device: '/dev/nvme0n1',
          size: 512_000_000_000,
          pretty_size: '512 GB',
          model: 'x',
          serial: 's',
          transport: 'nvme',
          system: false,
        },
      ],
      warnings: [],
    }
  }

  function stub(schematics: Schematic[], clusters: Array<{ id: string; name: string }>) {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string) => {
        const path = new URL(String(input), 'https://127.0.0.1:8443').pathname
        const json = (body: unknown) =>
          new Response(JSON.stringify(body), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          })
        if (path === '/api/v1/schematics') {
          return json(schematics)
        }
        if (path === '/api/v1/clusters') {
          return json({
            clusters: clusters.map((c) => ({
              ...c,
              origin: 'adopted',
              endpoint: 'https://10.0.0.10:6443',
              locked: false,
              created_at: '2026-08-29T10:00:00Z',
              client_cert_not_after: '2027-08-29T10:00:00Z',
              client_cert_days_left: 350,
              certificate_warning: '',
              certificate_urgency: 'none',
              nodes: 3,
              control_plane: 1,
              workers: 2,
              healthy: 3,
              degraded: 0,
              down: 0,
              checking: 0,
            })),
          })
        }
        return json({ type: 'about:blank', title: 'unrouted in this test' })
      }),
    )
  }

  function renderPlan() {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
    })
    return render(
      <QueryClientProvider client={client}>
        <PlanStep candidate={candidate()} onBack={() => undefined} />
      </QueryClientProvider>,
    )
  }

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('offers the schematics this installation holds, by name, and never a text field', async () => {
    stub([schematic({ id: ID_A, name: 'workers' }), schematic({ id: ID_B, name: 'gpu nodes' })], [])
    const user = userEvent.setup()

    renderPlan()

    const control = await screen.findByRole('combobox', { name: 'Image Factory schematic' })
    // The id was the whole content of the old field; a picker that still
    // accepted typing would be the old failure with a menu attached.
    expect(screen.queryByPlaceholderText(/the same id as the ISO/)).not.toBeInTheDocument()

    await user.click(control)
    expect(await screen.findByRole('option', { name: /workers/ })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: /gpu nodes/ })).toBeInTheDocument()
  })

  /**
   * A machine that booted a stock ISO has no schematic. Losing that option
   * while replacing a field that could simply be left empty would make the
   * ordinary case unreachable.
   */
  it('keeps "no schematic" reachable and starts there', async () => {
    stub([schematic({ id: ID_A, name: 'workers' })], [])

    renderPlan()

    const control = await screen.findByRole('combobox', { name: 'Image Factory schematic' })
    expect(control).toHaveTextContent(/Stock Talos/)
  })

  /**
   * Filing is not a constraint -- the image is the same for every cluster --
   * so the schematic stays selectable and is marked instead. The plan says it
   * again on the confirmation screen; this is the earlier of the two, where the
   * choice is still being made.
   */
  it('marks a schematic filed under a different cluster without hiding it', async () => {
    stub(
      [schematic({ id: ID_A, name: 'workers', cluster: LAB2 })],
      [
        { id: HOMELAB, name: 'homelab' },
        { id: LAB2, name: 'lab2' },
      ],
    )
    const user = userEvent.setup()

    renderPlan()

    await user.click(await screen.findByRole('combobox', { name: 'Cluster' }))
    await user.click(await screen.findByRole('option', { name: 'homelab' }))

    await user.click(screen.getByRole('combobox', { name: 'Image Factory schematic' }))
    const option = await screen.findByRole('option', { name: /workers/ })
    expect(option).toHaveTextContent('filed under another cluster')
    await user.click(option)

    expect(await screen.findByText(/filed under a different cluster/)).toBeInTheDocument()
  })

  it('does not mark a schematic filed under the cluster being joined', async () => {
    stub(
      [schematic({ id: ID_A, name: 'workers', cluster: HOMELAB })],
      [{ id: HOMELAB, name: 'homelab' }],
    )
    const user = userEvent.setup()

    renderPlan()

    await user.click(await screen.findByRole('combobox', { name: 'Cluster' }))
    await user.click(await screen.findByRole('option', { name: 'homelab' }))

    await user.click(screen.getByRole('combobox', { name: 'Image Factory schematic' }))
    const option = await screen.findByRole('option', { name: /workers/ })
    expect(option).not.toHaveTextContent('filed under another cluster')
  })
})
