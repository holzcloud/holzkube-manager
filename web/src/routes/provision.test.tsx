import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { CandidateDisk, Found } from '@/api'
import { DiskPicker, FoundTable } from '@/routes/provision'

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
