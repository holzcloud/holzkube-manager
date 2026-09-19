import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type Workload } from '@/api'
import { Workloads } from '@/components/Workloads'

/**
 * Everything that runs, whatever kind it is.
 *
 * The tests here are mostly about what is NOT offered. A DaemonSet has no
 * replica count and a Job has no pod template, and a screen that offered those
 * anyway would teach that its buttons are suggestions the API server might
 * refuse.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const base: Workload = {
  kind: 'Deployment',
  namespace: 'default',
  name: 'website',
  desired: 2,
  ready: 2,
  summary: '2 of 2 ready',
  image: 'web:1',
  created_at: '',
  schedule: '',
  suspended: false,
  last_run: '',
  scalable: true,
  rollable: true,
}

const daemon: Workload = {
  ...base,
  kind: 'DaemonSet',
  namespace: 'longhorn-system',
  name: 'longhorn-manager',
  desired: 2,
  ready: 1,
  summary: '1 of 2 nodes ready',
  scalable: false,
  rollable: true,
}

const cron: Workload = {
  ...base,
  kind: 'CronJob',
  name: 'backup',
  desired: 0,
  ready: 0,
  summary: 'last run 2026-09-19T02:00:00Z',
  schedule: '0 2 * * *',
  scalable: false,
  rollable: false,
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('what runs in a cluster', () => {
  it('lists every kind, not only Deployments', async () => {
    vi.spyOn(api.kubernetes, 'workloads').mockResolvedValue([base, daemon, cron])

    wrap(<Workloads clusterID="c-1" namespace="" />)

    expect(await screen.findByText(/longhorn-manager/)).toBeInTheDocument()
    expect(screen.getByText(/backup/)).toBeInTheDocument()
    expect(screen.getByText('DaemonSet')).toBeInTheDocument()
    expect(screen.getByText('CronJob')).toBeInTheDocument()
  })

  it('offers no replica field for a DaemonSet, and says why', async () => {
    vi.spyOn(api.kubernetes, 'workloads').mockResolvedValue([daemon])
    const scale = vi.spyOn(api.kubernetes, 'scaleWorkload')

    wrap(<Workloads clusterID="c-1" namespace="" />)
    await screen.findByText(/longhorn-manager/)

    expect(screen.queryByLabelText(/replicas for longhorn-manager/i)).toBeNull()
    // The reason is on the screen rather than in a tooltip: a phone has no
    // hover.
    expect(screen.getByText(/size is the cluster's shape/i)).toBeInTheDocument()
    // Rolling still applies — a DaemonSet has a pod template.
    expect(screen.getByRole('button', { name: 'Roll pods' })).toBeInTheDocument()
    expect(scale).not.toHaveBeenCalled()
  })

  it('offers nothing at all for a CronJob, and says what it is instead', async () => {
    vi.spyOn(api.kubernetes, 'workloads').mockResolvedValue([cron])

    wrap(<Workloads clusterID="c-1" namespace="" />)
    await screen.findByText(/backup/)

    expect(screen.queryByRole('button', { name: 'Roll pods' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Scale' })).toBeNull()
    expect(screen.getByText(/runs on its schedule/i)).toBeInTheDocument()
  })

  it('shows the server’s sentence rather than doing arithmetic on two numbers', async () => {
    vi.spyOn(api.kubernetes, 'workloads').mockResolvedValue([cron])

    wrap(<Workloads clusterID="c-1" namespace="" />)

    // "0 of 0 ready" is what a CronJob between runs looks like to arithmetic,
    // and it reports a schedule as an outage.
    expect(await screen.findByText(/last run 2026-09-19/)).toBeInTheDocument()
    expect(screen.queryByText(/0 of 0/)).toBeNull()
  })

  it('scales the kind it is looking at, not always a Deployment', async () => {
    vi.spyOn(api.kubernetes, 'workloads').mockResolvedValue([
      { ...base, kind: 'StatefulSet', namespace: 'db', name: 'postgres', desired: 3, ready: 3 },
    ])
    const scale = vi.spyOn(api.kubernetes, 'scaleWorkload').mockResolvedValue(undefined)

    wrap(<Workloads clusterID="c-1" namespace="" />)
    const field = await screen.findByLabelText(/replicas for postgres/i)
    await userEvent.clear(field)
    await userEvent.type(field, '5')
    await userEvent.click(screen.getByRole('button', { name: 'Scale' }))

    await waitFor(() =>
      expect(scale).toHaveBeenCalledWith('c-1', 'StatefulSet', 'db', 'postgres', 5),
    )
  })
})
