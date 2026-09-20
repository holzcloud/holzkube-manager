import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { TidyUp } from '@/components/TidyUp'

/**
 * Clearing out what is finished.
 *
 * This is a deletion, so the tests are about what happens BEFORE anything is
 * removed. The operator asked for a button; the button is the easy half.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const plan = {
  items: [
    {
      kind: 'ReplicaSet',
      namespace: 'default',
      name: 'website-7c9',
      reason: 'an older rollout of website, running nothing',
      age: '6 days',
    },
    {
      kind: 'Pod',
      namespace: 'backup',
      name: 'nightly-29244',
      reason: 'failed, and its log goes with it',
      age: '11 days',
    },
  ],
  notice: 'Images on the nodes are not here: the kubelet removes them itself.',
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('clearing out what is finished', () => {
  it('asks for nothing until somebody asks', async () => {
    const asked = vi.spyOn(api.kubernetes, 'sweepPlan').mockResolvedValue(plan)

    wrap(<TidyUp clusterID="c-1" namespace="" />)

    // A plan is three list calls against the API server, on a screen that
    // refetches itself. Nothing happens until the operator opens it.
    expect(asked).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /Look for things to clear out/ })).toBeInTheDocument()
  })

  it('shows the list with a reason on every row before removing anything', async () => {
    vi.spyOn(api.kubernetes, 'sweepPlan').mockResolvedValue(plan)
    const swept = vi.spyOn(api.kubernetes, 'sweep')

    wrap(<TidyUp clusterID="c-1" namespace="" />)
    await userEvent.click(screen.getByRole('button', { name: /Look for things to clear out/ }))

    // A list of names with no reasons is a list nobody can check.
    expect(
      await screen.findByText('an older rollout of website, running nothing'),
    ).toBeInTheDocument()
    expect(screen.getByText('failed, and its log goes with it')).toBeInTheDocument()
    // The first press is a plan, and the plan removes nothing.
    expect(swept).not.toHaveBeenCalled()
  })

  it('removes exactly the list that was shown', async () => {
    vi.spyOn(api.kubernetes, 'sweepPlan').mockResolvedValue(plan)
    const swept = vi.spyOn(api.kubernetes, 'sweep').mockResolvedValue({ removed: 2, failed: [] })

    wrap(<TidyUp clusterID="c-1" namespace="" />)
    await userEvent.click(screen.getByRole('button', { name: /Look for things to clear out/ }))
    await userEvent.click(await screen.findByRole('button', { name: 'Remove these 2' }))

    // The plan is passed back rather than recomputed: between a plan and an
    // apply somebody's CronJob can run, and a sweep that recomputed would
    // remove things nobody saw in the list they approved.
    await waitFor(() => {
      expect(swept).toHaveBeenCalledWith('c-1', plan.items)
    })
    expect(await screen.findByText('Removed 2.')).toBeInTheDocument()
  })

  it('says what is not in the list, including on an empty plan', async () => {
    vi.spyOn(api.kubernetes, 'sweepPlan').mockResolvedValue({ items: [], notice: plan.notice })

    wrap(<TidyUp clusterID="c-1" namespace="default" />)
    await userEvent.click(screen.getByRole('button', { name: /Look for things to clear out/ }))

    // An empty plan must not read as "there is nothing to clean up anywhere" --
    // images especially, which no button here can delete at all.
    expect(await screen.findByText(/the kubelet removes them itself/)).toBeInTheDocument()
    // And the empty list is a claim about this namespace, not "No data".
    expect(
      screen.getByText(/Nothing has finished and been left behind in default/),
    ).toBeInTheDocument()
  })

  it('offers no removal when there is nothing to remove', async () => {
    vi.spyOn(api.kubernetes, 'sweepPlan').mockResolvedValue({ items: [], notice: plan.notice })

    wrap(<TidyUp clusterID="c-1" namespace="" />)
    await userEvent.click(screen.getByRole('button', { name: /Look for things to clear out/ }))

    await screen.findByText(/the kubelet removes them itself/)
    expect(screen.getByRole('button', { name: /Remove/ })).toBeDisabled()
  })

  it('reports what could not be removed rather than only what was', async () => {
    vi.spyOn(api.kubernetes, 'sweepPlan').mockResolvedValue(plan)
    vi.spyOn(api.kubernetes, 'sweep').mockResolvedValue({
      removed: 1,
      failed: [{ reason: 'still terminating' }],
    })

    wrap(<TidyUp clusterID="c-1" namespace="" />)
    await userEvent.click(screen.getByRole('button', { name: /Look for things to clear out/ }))
    await userEvent.click(await screen.findByRole('button', { name: 'Remove these 2' }))

    expect(await screen.findByText('Removed 1; 1 could not be removed.')).toBeInTheDocument()
  })
})
