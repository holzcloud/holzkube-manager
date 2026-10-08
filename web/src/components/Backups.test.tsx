import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type Backups } from '@/api'
import { BackupsPanel } from '@/components/Backups'

/**
 * Scheduled etcd snapshots.
 *
 * The claims worth pinning are the ones that keep the panel honest: an overdue
 * schedule is said so on the panel, the limit of where the files live is on it
 * whatever the state, a skipped run shows its reason, and the download is a
 * plain link to a route that is admin-only and audited.
 */

const base: Backups = {
  available: true,
  schedule: { interval: 'daily', keep: 7 },
  presets: ['off', '6h', 'daily', 'weekly'],
  max_keep: 60,
  notice: '',
  state: {
    snapshots: [
      {
        id: '20261008T060000.000Z.scheduled.snapshot',
        kind: 'scheduled',
        taken_at: '2026-10-08T06:00:00Z',
        bytes: 22_020_096,
        sha256: 'ab'.repeat(32),
      },
      {
        id: '20261007T120000.000Z.snapshot',
        kind: 'upgrade',
        taken_at: '2026-10-07T12:00:00Z',
        bytes: 21_000_000,
        sha256: '',
      },
    ],
    status: {
      last_attempt_at: '2026-10-08T06:00:00Z',
      last_trigger: 'schedule',
      last_result: 'ok',
      last_reason: '',
    },
    health: {
      enabled: true,
      interval: 'daily',
      last_success_at: '2026-10-08T06:00:00Z',
      newest_at: '2026-10-08T06:00:00Z',
      age_seconds: 3600,
      overdue: false,
      last_result: 'ok',
      last_reason: '',
    },
    next_due_at: '2026-10-09T06:00:00Z',
    free_bytes: 8_000_000_000,
  },
}

const baseState = base.state as NonNullable<Backups['state']>
const baseHealth = baseState.health as NonNullable<NonNullable<Backups['state']>['health']>

function renderPanel(data: Backups = base) {
  vi.spyOn(api.backups, 'state').mockResolvedValue(data)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <BackupsPanel cluster="c-1" />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the backups panel', () => {
  it('lists what is stored, with kind, size and checksum, and links the download', async () => {
    renderPanel()

    expect(await screen.findByText('scheduled')).toBeInTheDocument()
    expect(screen.getByText('before an upgrade')).toBeInTheDocument()
    expect(screen.getByText('abababababababab')).toBeInTheDocument()
    expect(screen.getByText('not recorded')).toBeInTheDocument()

    const links = screen.getAllByRole('link', { name: /Download the snapshot/ })
    expect(links).toHaveLength(2)
    expect(links[0]).toHaveAttribute(
      'href',
      '/api/v1/clusters/c-1/backups/20261008T060000.000Z.scheduled.snapshot',
    )
  })

  it('says where the files live and that they do not survive losing the device', async () => {
    renderPanel()
    expect(await screen.findByText(/do not survive losing that device/)).toBeInTheDocument()
  })

  it('says so when the backup is overdue', async () => {
    const overdue: Backups = {
      ...base,
      state: {
        ...baseState,
        health: { ...baseHealth, overdue: true },
      },
    }
    renderPanel(overdue)
    expect(await screen.findByText('backup overdue')).toBeInTheDocument()
    expect(screen.getByText(/more than two intervals ago/)).toBeInTheDocument()
  })

  it('shows no overdue badge for a schedule that is on time', async () => {
    renderPanel()
    await screen.findByText('scheduled')
    expect(screen.queryByText('backup overdue')).not.toBeInTheDocument()
  })

  it('shows why the last run was skipped', async () => {
    renderPanel({
      ...base,
      state: {
        ...baseState,
        status: {
          ...baseState.status,
          last_result: 'skipped',
          last_reason: 'not enough free disk space for a snapshot: 20 MiB free',
        },
      },
    })
    expect(await screen.findByText(/skipped — not enough free disk space/)).toBeInTheDocument()
  })

  it('saves a schedule only once it has been changed, with the preset and the count', async () => {
    const user = userEvent.setup()
    const save = vi.spyOn(api.backups, 'setSchedule').mockResolvedValue({})
    renderPanel()

    const button = await screen.findByRole('button', { name: 'Save the schedule' })
    expect(button).toBeDisabled()

    const keep = screen.getByLabelText('Keep the newest')
    await user.clear(keep)
    await user.type(keep, '3')
    expect(button).toBeEnabled()
    await user.click(button)

    await waitFor(() => expect(save).toHaveBeenCalledWith('c-1', 'daily', 3))
  })

  it('refuses a count outside 1 to the maximum', async () => {
    const user = userEvent.setup()
    const save = vi.spyOn(api.backups, 'setSchedule').mockResolvedValue({})
    renderPanel()

    const keep = await screen.findByLabelText('Keep the newest')
    await user.clear(keep)
    await user.type(keep, '61')

    expect(screen.getByRole('alert')).toHaveTextContent('Keep between 1 and 60')
    expect(screen.getByRole('button', { name: 'Save the schedule' })).toBeDisabled()
    expect(save).not.toHaveBeenCalled()
  })

  it('runs a snapshot now', async () => {
    const user = userEvent.setup()
    const run = vi.spyOn(api.backups, 'run').mockResolvedValue({
      job: {} as never,
      topic: 'job:1',
    })
    renderPanel()

    await user.click(await screen.findByRole('button', { name: 'Run now' }))
    await waitFor(() => expect(run).toHaveBeenCalledWith('c-1'))
  })

  it('says so when the instance has nowhere to keep snapshots', async () => {
    renderPanel({ ...base, available: false, state: null })
    expect(await screen.findByText(/nowhere to keep snapshots/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Run now' })).not.toBeInTheDocument()
  })
})
