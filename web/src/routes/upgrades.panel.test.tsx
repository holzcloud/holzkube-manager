import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { api, type UpgradePlan } from '@/api'
import { UpgradePanel } from '@/routes/upgrades'

/**
 * A plan describes one target. Once What or To version changes, the plan on
 * screen and the confirmation typed against it describe something the form no
 * longer says, and "Start the rolling upgrade" would launch a version nobody
 * planned.
 */

const plan = {
  cluster: 'c-1',
  to: 'v1.34.1',
  chain: [],
  strand: { blocked: false, sentence: 'Kubernetes stays supported.', remedy: '' },
  gate_preview: {
    ok: true,
    reason: '',
    input: { members: [], voting: 3, statuses: {}, unreachable: [], alarms: [], max_raft_lag: 0 },
  },
  nodes: [],
  blocked: false,
  block_reason: '',
} as unknown as UpgradePlan

function renderPanel() {
  vi.spyOn(api.upgrades, 'releases').mockResolvedValue({ releases: ['v1.14.2'], notice: '' })
  vi.spyOn(api.upgrades, 'talosCheck').mockRejectedValue(new Error('offline'))
  vi.spyOn(api.upgrades, 'snapshotState').mockRejectedValue(new Error('offline'))
  vi.spyOn(api.upgrades, 'planKubernetes').mockResolvedValue(plan)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <UpgradePanel cluster="c-1" name="homelab" />
    </QueryClientProvider>,
  )
}

async function planKubernetes(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('combobox', { name: 'What' }))
  await user.click(await screen.findByRole('option', { name: 'Kubernetes' }))
  await user.type(screen.getByLabelText('To version'), 'v1.34.1')
  await user.click(screen.getByRole('button', { name: /Show me what this would do/ }))
  await screen.findByText('Kubernetes stays supported.')
}

describe('the upgrade panel', () => {
  it('drops the plan and the typed confirmation when the version changes', async () => {
    const user = userEvent.setup()
    renderPanel()
    await planKubernetes(user)
    await user.type(screen.getByLabelText(/Type/), 'homelab')

    await user.type(screen.getByLabelText('To version'), '9')

    expect(screen.queryByText('Kubernetes stays supported.')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Start the rolling upgrade/ })).toBeNull()
  })

  it('drops the plan when What changes', async () => {
    const user = userEvent.setup()
    renderPanel()
    await planKubernetes(user)

    await user.click(screen.getByRole('combobox', { name: 'What' }))
    await user.click(await screen.findByRole('option', { name: 'Talos' }))

    expect(screen.queryByText('Kubernetes stays supported.')).not.toBeInTheDocument()
  })
})
