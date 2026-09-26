import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type Power, powerActions, powerPath } from '@/api'
import { describe as describeAction, PowerMenu } from '@/components/PowerMenu'

/**
 * The power menu (2026-09-26): the same seven actions for a cluster, a node and
 * an app, every one of them always listed, the impossible ones with the
 * server's reason, and nothing run without asking first.
 */

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  Link: ({ children }: { children: ReactNode }) => <a href="#link">{children}</a>,
}))

function wrap(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const power: Power = {
  state: 'running',
  disabled: false,
  actions: powerActions.map((action) => ({
    action,
    available: action !== 'start' && action !== 'enable',
    reason:
      action === 'start' ? 'It is running.' : action === 'enable' ? 'It is not disabled.' : '',
    sudo: action.startsWith('force-'),
  })),
}

afterEach(() => vi.restoreAllMocks())

describe('PowerMenu', () => {
  it('lists every action, with the reason beside the ones that cannot run', async () => {
    vi.spyOn(api.power, 'get').mockResolvedValue(power)
    wrap(<PowerMenu target={{ kind: 'node', machine: 'm-1', name: 'wk-02' }} />)

    await userEvent.click(await screen.findByRole('button', { name: 'Power: wk-02' }))

    for (const label of [
      'Stop',
      'Force stop',
      'Start',
      'Disable',
      'Enable',
      'Restart',
      'Force restart',
    ]) {
      expect(await screen.findByText(label)).toBeInTheDocument()
    }
    expect(screen.getByText('It is running.')).toBeInTheDocument()
    expect(screen.getAllByText('asks for your password')).toHaveLength(2)
  })

  it('asks before it runs, and runs what was asked', async () => {
    vi.spyOn(api.power, 'get').mockResolvedValue(power)
    const run = vi.spyOn(api.power, 'run').mockResolvedValue({ message: 'Restarted.' })
    wrap(
      <PowerMenu
        target={{
          kind: 'app',
          cluster: 'c',
          namespace: 'media',
          appKind: 'Deployment',
          name: 'jellyfin',
        }}
      />,
    )

    await userEvent.click(await screen.findByRole('button', { name: 'Power: jellyfin' }))
    await userEvent.click(await screen.findByText('Restart'))

    expect(run).not.toHaveBeenCalled()
    expect(await screen.findByText('Restart jellyfin?')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Restart' }))
    expect(run).toHaveBeenCalledWith(
      { kind: 'app', cluster: 'c', namespace: 'media', appKind: 'Deployment', name: 'jellyfin' },
      'restart',
    )
    expect(await screen.findByText('Restarted.')).toBeInTheDocument()
  })
})

describe('what an action says it does', () => {
  it('names the target and warns that a forced stop does not move anything', () => {
    const text = describeAction({ kind: 'node', machine: 'm', name: 'wk-02' }, 'force-stop')
    expect(text).toContain('wk-02')
    expect(text).toMatch(/not moved/)
  })

  it('addresses each kind of target at its own route', () => {
    expect(powerPath({ kind: 'cluster', cluster: 'c1', name: 'x' })).toBe(
      '/api/v1/clusters/c1/power',
    )
    expect(powerPath({ kind: 'node', machine: 'm1', name: 'x' })).toBe('/api/v1/machines/m1/power')
    expect(
      powerPath({
        kind: 'app',
        cluster: 'c1',
        namespace: 'ns',
        appKind: 'StatefulSet',
        name: 'db',
      }),
    ).toBe('/api/v1/clusters/c1/kubernetes/apps/ns/StatefulSet/db/power')
  })
})
